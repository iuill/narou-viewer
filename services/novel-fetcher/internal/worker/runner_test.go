package worker

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"narou-viewer/services/novel-fetcher/internal/storage"
	"narou-viewer/services/novel-fetcher/internal/taskqueue"
	"narou-viewer/services/novel-fetcher/internal/taskstate"
)

type blockingExecutor struct {
	entered chan struct{}
	once    sync.Once
	err     error
}

type closingStoreExecutor struct {
	store  *storage.Store
	closed chan struct{}
}

type heldControlExecutor struct {
	entered  chan struct{}
	observed chan struct{}
	release  chan struct{}
	once     sync.Once
}

type immediateExecutor struct {
	entered chan struct{}
	once    sync.Once
}

type claimHandoffRepository struct {
	taskstate.Repository
	claimed chan struct{}
	release chan struct{}
	once    sync.Once
}

type blockingFinalizeRepository struct {
	taskstate.Repository
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (e *closingStoreExecutor) RunTask(_ context.Context, _ *taskqueue.Task) error {
	_ = e.store.Close()
	close(e.closed)
	return nil
}

func (e *heldControlExecutor) RunTask(ctx context.Context, _ *taskqueue.Task) error {
	e.once.Do(func() { close(e.entered) })
	<-ctx.Done()
	close(e.observed)
	<-e.release
	return ctx.Err()
}

func (e *immediateExecutor) RunTask(_ context.Context, _ *taskqueue.Task) error {
	e.once.Do(func() { close(e.entered) })
	return nil
}

func (r *claimHandoffRepository) ClaimNext(ctx context.Context, now time.Time) (*taskstate.Task, error) {
	task, err := r.Repository.ClaimNext(ctx, now)
	if err == nil && task != nil {
		r.once.Do(func() { close(r.claimed) })
		<-r.release
	}
	return task, err
}

func (r *blockingFinalizeRepository) Finalize(ctx context.Context, ref taskstate.TaskRef, outcome taskstate.Outcome) error {
	r.once.Do(func() { close(r.entered) })
	<-r.release
	return r.Repository.Finalize(ctx, ref, outcome)
}

func TestRunnerStartAndStopAreIdempotent(t *testing.T) {
	runner := NewRunner(Options{Queue: taskqueue.NewQueue()})
	runner.Stop(context.Background())
	runner.Start(context.Background())
	runner.Start(context.Background())
	runner.Stop(context.Background())
}

func (e *blockingExecutor) RunTask(ctx context.Context, _ *taskqueue.Task) error {
	e.once.Do(func() {
		close(e.entered)
	})
	<-ctx.Done()
	if e.err != nil {
		return e.err
	}
	return ctx.Err()
}

func TestRunnerStartAndStopCancelsCurrentTask(t *testing.T) {
	queue := taskqueue.NewQueue()
	executor := &blockingExecutor{entered: make(chan struct{})}
	runner := NewRunner(Options{Queue: queue, Executor: executor})
	runner.Start(context.Background())
	defer runner.Stop(context.Background())

	queue.Enqueue(taskqueue.NewTask("download"))
	select {
	case <-executor.entered:
	case <-time.After(time.Second):
		t.Fatal("executor was not called")
	}

	stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	runner.Stop(stopCtx)

	summary := queue.Summary()
	if summary.Current != nil || summary.InterruptedCount != 1 {
		t.Fatalf("summary = %#v", summary)
	}
	if summary.Interrupted[0]["status"] != taskqueue.StatusInterrupted {
		t.Fatalf("interrupted payload = %#v", summary.Interrupted[0])
	}
}

func TestRunnerCancelCancelsCurrentTask(t *testing.T) {
	queue := taskqueue.NewQueue()
	executor := &blockingExecutor{entered: make(chan struct{})}
	runner := NewRunner(Options{Queue: queue, Executor: executor})
	runner.Start(context.Background())
	defer runner.Stop(context.Background())

	task := taskqueue.NewTask("download")
	queue.Enqueue(task)
	select {
	case <-executor.entered:
	case <-time.After(time.Second):
		t.Fatal("executor was not called")
	}

	if result, err := runner.RequestCancel(task.ID); err != nil || !result.Changed {
		t.Fatalf("running cancel = %#v, err = %v", result, err)
	}

	deadline := time.After(time.Second)
	for {
		summary := queue.Summary()
		if summary.Current == nil && summary.FailedCount == 1 {
			if summary.RecentFailed[0]["status"] != taskqueue.StatusCanceled {
				t.Fatalf("failed payload = %#v", summary.RecentFailed[0])
			}
			return
		}

		select {
		case <-deadline:
			t.Fatalf("summary after cancel = %#v", summary)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestRunnerPausePreservesResumableTaskState(t *testing.T) {
	queue := taskqueue.NewQueue()
	executor := &blockingExecutor{entered: make(chan struct{})}
	runner := NewRunner(Options{Queue: queue, Executor: executor})
	runner.Start(context.Background())
	defer runner.Stop(context.Background())

	task := taskqueue.NewTask("download")
	if err := queue.Enqueue(task); err != nil {
		t.Fatal(err)
	}
	select {
	case <-executor.entered:
	case <-time.After(time.Second):
		t.Fatal("executor was not called")
	}
	if result, err := runner.RequestPause(task.ID); err != nil || !result.Changed {
		t.Fatalf("running pause = %#v, err = %v", result, err)
	}

	deadline := time.After(time.Second)
	for {
		summary := queue.Summary()
		if summary.Current == nil && summary.PausedCount == 1 {
			if summary.Paused[0]["status"] != taskqueue.StatusPaused {
				t.Fatalf("paused payload = %#v", summary.Paused[0])
			}
			return
		}
		select {
		case <-deadline:
			t.Fatalf("summary after pause = %#v", summary)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestRunnerControlsQueuedTasksBeforeExecution(t *testing.T) {
	queue := taskqueue.NewQueue()
	runner := NewRunner(Options{Queue: queue, Executor: &blockingExecutor{entered: make(chan struct{})}})
	paused := taskqueue.NewTask("download")
	if err := queue.Enqueue(paused); err != nil {
		t.Fatal(err)
	}
	if result, err := runner.RequestPause(paused.ID); err != nil || !result.Changed {
		t.Fatalf("queued pause = %#v, err = %v", result, err)
	}
	canceled := taskqueue.NewTask("download")
	if err := queue.Enqueue(canceled); err != nil {
		t.Fatal(err)
	}
	if result, err := runner.RequestCancel(canceled.ID); err != nil || !result.Changed {
		t.Fatalf("queued cancel = %#v, err = %v", result, err)
	}
	if summary := queue.Summary(); summary.FailedCount != 1 || len(summary.RecentFailed) != 1 || summary.PausedCount != 1 {
		t.Fatalf("queued control summary = %#v", summary)
	}
}

func TestRunnerStartingHandoffKeepsDurableControlAsWinner(t *testing.T) {
	for _, test := range []struct {
		name       string
		first      taskstate.RequestedAction
		second     taskstate.RequestedAction
		wantStatus taskstate.Status
	}{
		{name: "pause then cancel", first: taskstate.RequestedActionPause, second: taskstate.RequestedActionCancel, wantStatus: taskstate.StatusPaused},
		{name: "cancel then pause", first: taskstate.RequestedActionCancel, second: taskstate.RequestedActionPause, wantStatus: taskstate.StatusCanceled},
	} {
		t.Run(test.name, func(t *testing.T) {
			store, err := storage.NewStore(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			base := taskstate.NewSQLiteRepository(store.DB())
			repository := &claimHandoffRepository{
				Repository: base,
				claimed:    make(chan struct{}),
				release:    make(chan struct{}),
			}
			released := false
			queue := taskqueue.NewPersistentQueue(repository)
			task := taskqueue.NewTask("download")
			task.Targets = []string{"https://example.invalid/handoff/" + test.name}
			if err := queue.Enqueue(task); err != nil {
				t.Fatal(err)
			}
			runner := NewRunner(Options{Queue: queue, Executor: &blockingExecutor{entered: make(chan struct{})}})
			runner.Start(context.Background())
			defer runner.Stop(context.Background())
			defer func() {
				if !released {
					close(repository.release)
				}
			}()

			select {
			case <-repository.claimed:
			case <-time.After(time.Second):
				t.Fatal("task was not claimed")
			}
			var firstResult taskstate.ControlResult
			if test.first == taskstate.RequestedActionPause {
				firstResult, err = base.RequestPause(context.Background(), task.ID)
			} else {
				firstResult, err = base.RequestCancel(context.Background(), task.ID)
			}
			if err != nil || !firstResult.Changed || firstResult.Task.RequestedAction != test.first {
				t.Fatalf("first durable control = %#v, err = %v", firstResult, err)
			}

			secondErr := make(chan error, 1)
			go func() {
				var requestErr error
				if test.second == taskstate.RequestedActionPause {
					_, requestErr = runner.RequestPause(task.ID)
				} else {
					_, requestErr = runner.RequestCancel(task.ID)
				}
				secondErr <- requestErr
			}()
			close(repository.release)
			released = true
			select {
			case err := <-secondErr:
				if !errors.Is(err, taskstate.ErrTaskStateConflict) {
					t.Fatalf("second control error = %v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("second control did not return")
			}
			waitForTaskStatus(t, base, task.ID, test.wantStatus)
		})
	}
}

func TestRunnerWaitForNextWorkHonorsIntervalAndCancellation(t *testing.T) {
	runner := NewRunner(Options{WorkInterval: time.Millisecond})
	if !runner.waitForNextWork(context.Background()) {
		t.Fatal("waitForNextWork returned false after interval")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runner = NewRunner(Options{WorkInterval: time.Hour})
	if runner.waitForNextWork(ctx) {
		t.Fatal("waitForNextWork returned true after context cancellation")
	}
}

func TestRunnerRepeatedRunningControlIsIdempotent(t *testing.T) {
	store, err := storage.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	repository := taskstate.NewSQLiteRepository(store.DB())
	queue := taskqueue.NewPersistentQueue(repository)
	executor := &heldControlExecutor{entered: make(chan struct{}), observed: make(chan struct{}), release: make(chan struct{})}
	released := false
	runner := NewRunner(Options{Queue: queue, Executor: executor})
	runner.Start(context.Background())
	defer runner.Stop(context.Background())
	defer func() {
		if !released {
			close(executor.release)
		}
	}()
	task := taskqueue.NewTask("download")
	task.Targets = []string{"https://example.invalid/idempotent-control"}
	if err := queue.Enqueue(task); err != nil {
		t.Fatal(err)
	}
	select {
	case <-executor.entered:
	case <-time.After(time.Second):
		t.Fatal("executor did not start")
	}
	first, err := runner.RequestPause(task.ID)
	if err != nil || !first.Changed {
		t.Fatalf("first pause = %#v, err = %v", first, err)
	}
	select {
	case <-executor.observed:
	case <-time.After(time.Second):
		t.Fatal("executor did not observe pause")
	}
	second, err := runner.RequestPause(task.ID)
	if err != nil || second.Changed {
		t.Fatalf("repeated pause = %#v, err = %v", second, err)
	}
	if _, err := runner.RequestCancel(task.ID); !errors.Is(err, taskstate.ErrTaskStateConflict) {
		t.Fatalf("conflicting cancel error = %v", err)
	}
	close(executor.release)
	released = true
	waitForTaskStatus(t, repository, task.ID, taskstate.StatusPaused)
}

func TestRunnerRejectsNewControlsWhileFinalizing(t *testing.T) {
	store, err := storage.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	base := taskstate.NewSQLiteRepository(store.DB())
	repository := &blockingFinalizeRepository{Repository: base, entered: make(chan struct{}), release: make(chan struct{})}
	released := false
	queue := taskqueue.NewPersistentQueue(repository)
	executor := &immediateExecutor{entered: make(chan struct{})}
	runner := NewRunner(Options{Queue: queue, Executor: executor})
	runner.Start(context.Background())
	defer runner.Stop(context.Background())
	defer func() {
		if !released {
			close(repository.release)
		}
	}()
	task := taskqueue.NewTask("download")
	task.Targets = []string{"https://example.invalid/finalizing-control"}
	if err := queue.Enqueue(task); err != nil {
		t.Fatal(err)
	}
	select {
	case <-repository.entered:
	case <-time.After(time.Second):
		t.Fatal("finalization did not start")
	}
	for _, request := range []func(string) (taskstate.ControlResult, error){runner.RequestPause, runner.RequestCancel} {
		if _, err := request(task.ID); !errors.Is(err, taskstate.ErrTaskStateConflict) {
			t.Fatalf("finalizing control error = %v", err)
		}
	}
	stored, found, err := base.Get(context.Background(), task.ID)
	if err != nil || !found || stored.Status != taskstate.StatusRunning || stored.RequestedAction != taskstate.RequestedActionNone {
		t.Fatalf("task before finalize release = %#v, found = %v, err = %v", stored, found, err)
	}
	close(repository.release)
	released = true
	waitForTaskStatus(t, base, task.ID, taskstate.StatusSucceeded)
}

func TestRunnerKeepsAcceptedControlIdempotentWhileFinalizing(t *testing.T) {
	store, err := storage.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	base := taskstate.NewSQLiteRepository(store.DB())
	repository := &blockingFinalizeRepository{Repository: base, entered: make(chan struct{}), release: make(chan struct{})}
	queue := taskqueue.NewPersistentQueue(repository)
	executor := &blockingExecutor{entered: make(chan struct{})}
	runner := NewRunner(Options{Queue: queue, Executor: executor})
	runner.Start(context.Background())
	defer runner.Stop(context.Background())
	released := false
	defer func() {
		if !released {
			close(repository.release)
		}
	}()
	task := taskqueue.NewTask("download")
	task.Targets = []string{"https://example.invalid/finalizing-idempotent"}
	if err := queue.Enqueue(task); err != nil {
		t.Fatal(err)
	}
	select {
	case <-executor.entered:
	case <-time.After(time.Second):
		t.Fatal("executor did not start")
	}
	first, err := runner.RequestPause(task.ID)
	if err != nil || !first.Changed {
		t.Fatalf("first pause = %#v, err = %v", first, err)
	}
	select {
	case <-repository.entered:
	case <-time.After(time.Second):
		t.Fatal("finalization did not start")
	}
	repeated, err := runner.RequestPause(task.ID)
	if err != nil || repeated.Changed {
		t.Fatalf("finalizing repeated pause = %#v, err = %v", repeated, err)
	}
	if _, err := runner.RequestCancel(task.ID); !errors.Is(err, taskstate.ErrTaskStateConflict) {
		t.Fatalf("finalizing conflicting cancel error = %v", err)
	}
	close(repository.release)
	released = true
	waitForTaskStatus(t, base, task.ID, taskstate.StatusPaused)
}

func waitForTaskStatus(t *testing.T, repository taskstate.Repository, taskID string, want taskstate.Status) {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		task, found, err := repository.Get(context.Background(), taskID)
		if err == nil && found && task.Status == want {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("task %s status = %#v, found = %v, err = %v; want %s", taskID, task, found, err, want)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestRunnerRetryWaitUsesBackoffAndHonorsCancellation(t *testing.T) {
	runner := NewRunner(Options{WorkInterval: time.Millisecond})
	if !runner.waitForRetry(context.Background()) {
		t.Fatal("retry wait ended before its configured delay")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if NewRunner(Options{}).waitForRetry(canceled) {
		t.Fatal("default retry wait ignored cancellation")
	}
}

func TestRunnerStopsRetryingWhenTaskStoreCannotBeRead(t *testing.T) {
	store, err := storage.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	queue := taskqueue.NewPersistentQueue(taskstate.NewSQLiteRepository(store.DB()))
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	runner := NewRunner(Options{Queue: queue, WorkInterval: time.Hour, Logger: slog.Default()})
	runner.Start(context.Background())
	time.Sleep(20 * time.Millisecond)
	stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	runner.Stop(stopCtx)
}

func TestRunnerRetriesFailedTaskFinalizationUntilShutdown(t *testing.T) {
	store, err := storage.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	queue := taskqueue.NewPersistentQueue(taskstate.NewSQLiteRepository(store.DB()))
	executor := &closingStoreExecutor{store: store, closed: make(chan struct{})}
	runner := NewRunner(Options{Queue: queue, Executor: executor, WorkInterval: time.Hour, Logger: slog.Default()})
	runner.Start(context.Background())
	task := taskqueue.NewTask("download")
	task.Targets = []string{"https://example.invalid/finalize-error"}
	if err := queue.Enqueue(task); err != nil {
		t.Fatal(err)
	}
	select {
	case <-executor.closed:
	case <-time.After(time.Second):
		t.Fatal("executor did not close the task store")
	}
	time.Sleep(20 * time.Millisecond)
	stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	runner.Stop(stopCtx)
}
