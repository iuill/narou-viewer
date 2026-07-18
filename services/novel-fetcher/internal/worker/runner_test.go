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

type postCommitFinalizeRepository struct {
	taskstate.Repository
	committed chan struct{}
	release   chan struct{}
	once      sync.Once
}

type flakyControlRepository struct {
	taskstate.Repository
	mu             sync.Mutex
	pauseFailures  int
	cancelFailures int
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

func (r *postCommitFinalizeRepository) Finalize(ctx context.Context, ref taskstate.TaskRef, outcome taskstate.Outcome) error {
	err := r.Repository.Finalize(ctx, ref, outcome)
	r.once.Do(func() { close(r.committed) })
	<-r.release
	return err
}

func (r *flakyControlRepository) RequestPause(ctx context.Context, taskID string) (taskstate.ControlResult, error) {
	r.mu.Lock()
	if r.pauseFailures > 0 {
		r.pauseFailures--
		r.mu.Unlock()
		return taskstate.ControlResult{}, errors.New("synthetic pause persistence failure")
	}
	r.mu.Unlock()
	return r.Repository.RequestPause(ctx, taskID)
}

func (r *flakyControlRepository) RequestCancel(ctx context.Context, taskID string) (taskstate.ControlResult, error) {
	r.mu.Lock()
	if r.cancelFailures > 0 {
		r.cancelFailures--
		r.mu.Unlock()
		return taskstate.ControlResult{}, errors.New("synthetic cancel persistence failure")
	}
	r.mu.Unlock()
	return r.Repository.RequestCancel(ctx, taskID)
}

func newRunnerTestQueue(t *testing.T) (*taskqueue.Queue, taskstate.Repository) {
	t.Helper()
	store, err := storage.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	repository := taskstate.NewSQLiteRepositoryWithReader(store.DB(), store.ReadDB())
	return taskqueue.NewQueue(repository), repository
}

func runnerQueueSummary(t *testing.T, queue *taskqueue.Queue) taskqueue.Summary {
	t.Helper()
	summary, err := queue.Summary()
	if err != nil {
		t.Fatal(err)
	}
	return summary
}

func TestRunnerStartAndStopAreIdempotent(t *testing.T) {
	queue, _ := newRunnerTestQueue(t)
	runner := NewRunner(Options{Queue: queue})
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
	queue, _ := newRunnerTestQueue(t)
	executor := &blockingExecutor{entered: make(chan struct{})}
	runner := NewRunner(Options{Queue: queue, Executor: executor})
	runner.Start(context.Background())
	defer runner.Stop(context.Background())

	task := taskqueue.NewTask("download")
	task.Target = "https://example.invalid/synthetic/stop"
	if err := queue.Enqueue(task); err != nil {
		t.Fatal(err)
	}
	select {
	case <-executor.entered:
	case <-time.After(time.Second):
		t.Fatal("executor was not called")
	}

	stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	runner.Stop(stopCtx)

	summary := runnerQueueSummary(t, queue)
	if summary.Current != nil || summary.InterruptedCount != 1 {
		t.Fatalf("summary = %#v", summary)
	}
	if summary.Interrupted[0]["status"] != taskqueue.StatusInterrupted {
		t.Fatalf("interrupted payload = %#v", summary.Interrupted[0])
	}
}

func TestRunnerCancelCancelsCurrentTask(t *testing.T) {
	queue, _ := newRunnerTestQueue(t)
	executor := &blockingExecutor{entered: make(chan struct{})}
	runner := NewRunner(Options{Queue: queue, Executor: executor})
	runner.Start(context.Background())
	defer runner.Stop(context.Background())

	task := taskqueue.NewTask("download")
	task.Target = "https://example.invalid/synthetic/cancel"
	if err := queue.Enqueue(task); err != nil {
		t.Fatal(err)
	}
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
		summary := runnerQueueSummary(t, queue)
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
	queue, _ := newRunnerTestQueue(t)
	executor := &blockingExecutor{entered: make(chan struct{})}
	runner := NewRunner(Options{Queue: queue, Executor: executor})
	runner.Start(context.Background())
	defer runner.Stop(context.Background())

	task := taskqueue.NewTask("download")
	task.Target = "https://example.invalid/synthetic/pause"
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
		summary := runnerQueueSummary(t, queue)
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
	queue, _ := newRunnerTestQueue(t)
	runner := NewRunner(Options{Queue: queue, Executor: &blockingExecutor{entered: make(chan struct{})}})
	paused := taskqueue.NewTask("download")
	paused.Target = "https://example.invalid/synthetic/queued-pause"
	if err := queue.Enqueue(paused); err != nil {
		t.Fatal(err)
	}
	if result, err := runner.RequestPause(paused.ID); err != nil || !result.Changed {
		t.Fatalf("queued pause = %#v, err = %v", result, err)
	}
	canceled := taskqueue.NewTask("download")
	canceled.Target = "https://example.invalid/synthetic/queued-cancel"
	if err := queue.Enqueue(canceled); err != nil {
		t.Fatal(err)
	}
	if result, err := runner.RequestCancel(canceled.ID); err != nil || !result.Changed {
		t.Fatalf("queued cancel = %#v, err = %v", result, err)
	}
	if summary := runnerQueueSummary(t, queue); summary.FailedCount != 1 || len(summary.RecentFailed) != 1 || summary.PausedCount != 1 {
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
			queue := taskqueue.NewQueue(repository)
			task := taskqueue.NewTask("download")
			task.Target = "https://example.invalid/handoff/" + test.name
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
				firstResult, err = runner.RequestPause(task.ID)
			} else {
				firstResult, err = runner.RequestCancel(task.ID)
			}
			if err != nil || !firstResult.Changed || firstResult.Task.RequestedAction != test.first {
				t.Fatalf("first durable control = %#v, err = %v", firstResult, err)
			}

			if test.second == taskstate.RequestedActionPause {
				_, err = runner.RequestPause(task.ID)
			} else {
				_, err = runner.RequestCancel(task.ID)
			}
			if !errors.Is(err, taskstate.ErrTaskStateConflict) {
				t.Fatalf("second control error = %v", err)
			}
			close(repository.release)
			released = true
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
	queue := taskqueue.NewQueue(repository)
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
	task.Target = "https://example.invalid/idempotent-control"
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

func TestRunnerRetriesSameControlAfterPersistenceFailure(t *testing.T) {
	store, err := storage.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	base := taskstate.NewSQLiteRepository(store.DB())
	repository := &flakyControlRepository{Repository: base, pauseFailures: 1}
	queue := taskqueue.NewQueue(repository)
	executor := &heldControlExecutor{entered: make(chan struct{}), observed: make(chan struct{}), release: make(chan struct{})}
	runner := NewRunner(Options{Queue: queue, Executor: executor, WorkInterval: time.Millisecond})
	runner.Start(context.Background())
	defer runner.Stop(context.Background())
	released := false
	defer func() {
		if !released {
			close(executor.release)
		}
	}()
	task := taskqueue.NewTask("download")
	task.Target = "https://example.invalid/synthetic/retry-control"
	if err := queue.Enqueue(task); err != nil {
		t.Fatal(err)
	}
	select {
	case <-executor.entered:
	case <-time.After(time.Second):
		t.Fatal("executor did not start")
	}
	if _, err := runner.RequestPause(task.ID); err == nil {
		t.Fatal("first pause persistence unexpectedly succeeded")
	}
	select {
	case <-executor.observed:
	case <-time.After(time.Second):
		t.Fatal("executor did not observe pause after persistence failure")
	}
	retried, err := runner.RequestPause(task.ID)
	if err != nil || !retried.Changed || retried.Task == nil || retried.Task.RequestedAction != taskstate.RequestedActionPause {
		t.Fatalf("retried pause = %#v, err = %v", retried, err)
	}
	close(executor.release)
	released = true
	waitForTaskStatus(t, base, task.ID, taskstate.StatusPaused)
}

func TestRunnerPersistsAcceptedControlDuringFinalizationRetry(t *testing.T) {
	store, err := storage.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	base := taskstate.NewSQLiteRepository(store.DB())
	repository := &flakyControlRepository{Repository: base, cancelFailures: 2}
	queue := taskqueue.NewQueue(repository)
	executor := &blockingExecutor{entered: make(chan struct{})}
	runner := NewRunner(Options{Queue: queue, Executor: executor, WorkInterval: time.Millisecond, Logger: slog.Default()})
	runner.Start(context.Background())
	defer runner.Stop(context.Background())
	task := taskqueue.NewTask("download")
	task.Target = "https://example.invalid/synthetic/finalize-control-retry"
	if err := queue.Enqueue(task); err != nil {
		t.Fatal(err)
	}
	select {
	case <-executor.entered:
	case <-time.After(time.Second):
		t.Fatal("executor did not start")
	}
	if _, err := runner.RequestCancel(task.ID); err == nil {
		t.Fatal("first cancel persistence unexpectedly succeeded")
	}
	waitForTaskStatus(t, base, task.ID, taskstate.StatusCanceled)
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
	queue := taskqueue.NewQueue(repository)
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
	task.Target = "https://example.invalid/finalizing-control"
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
	queue := taskqueue.NewQueue(repository)
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
	task.Target = "https://example.invalid/finalizing-idempotent"
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

func TestRunnerRejectsResumeUntilFinalizedAttemptIsCleared(t *testing.T) {
	store, err := storage.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	base := taskstate.NewSQLiteRepository(store.DB())
	repository := &postCommitFinalizeRepository{
		Repository: base,
		committed:  make(chan struct{}),
		release:    make(chan struct{}),
	}
	released := false
	queue := taskqueue.NewQueue(repository)
	executor := &blockingExecutor{entered: make(chan struct{})}
	runner := NewRunner(Options{Queue: queue, Executor: executor})
	runner.Start(context.Background())
	defer runner.Stop(context.Background())
	defer func() {
		if !released {
			close(repository.release)
		}
	}()
	task := taskqueue.NewTask("download")
	task.Target = "https://example.invalid/finalize-resume-fence"
	if err := queue.Enqueue(task); err != nil {
		t.Fatal(err)
	}
	select {
	case <-executor.entered:
	case <-time.After(time.Second):
		t.Fatal("executor did not start")
	}
	if result, err := runner.RequestPause(task.ID); err != nil || !result.Changed {
		t.Fatalf("pause = %#v, err = %v", result, err)
	}
	select {
	case <-repository.committed:
	case <-time.After(time.Second):
		t.Fatal("paused finalization was not committed")
	}
	stored, found, err := base.Get(context.Background(), task.ID)
	if err != nil || !found || stored.Status != taskstate.StatusPaused {
		t.Fatalf("committed task = %#v, found = %v, err = %v", stored, found, err)
	}
	if _, err := runner.RequestResume(task.ID); !errors.Is(err, taskstate.ErrTaskStateConflict) {
		t.Fatalf("resume before runner clear error = %v", err)
	}

	close(repository.release)
	released = true
	waitForRunnerIdle(t, runner)
	resumed, err := runner.RequestResume(task.ID)
	if err != nil || !resumed.Changed || resumed.Task == nil || resumed.Task.Status != taskstate.StatusQueued {
		t.Fatalf("resume after runner clear = %#v, err = %v", resumed, err)
	}
}

func TestRunnerRepeatedResumeIsIdempotentAfterTaskStarts(t *testing.T) {
	store, err := storage.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	repository := taskstate.NewSQLiteRepositoryWithReader(store.DB(), store.ReadDB())
	queue := taskqueue.NewQueue(repository)
	executor := &blockingExecutor{entered: make(chan struct{})}
	runner := NewRunner(Options{Queue: queue, Executor: executor})
	runner.Start(context.Background())
	defer runner.Stop(context.Background())
	task := taskqueue.NewTask("download")
	task.Target = "https://example.invalid/synthetic/repeated-resume"
	if err := queue.Enqueue(task); err != nil {
		t.Fatal(err)
	}
	select {
	case <-executor.entered:
	case <-time.After(time.Second):
		t.Fatal("executor did not start")
	}
	result, err := runner.RequestResume(task.ID)
	if err != nil || result.Changed || result.Task == nil || result.Task.Status != taskstate.StatusRunning {
		t.Fatalf("repeated resume = %#v, err = %v", result, err)
	}
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

func waitForRunnerIdle(t *testing.T, runner *Runner) {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		runner.mu.Lock()
		idle := runner.taskRef.TaskID == ""
		runner.mu.Unlock()
		if idle {
			return
		}
		select {
		case <-deadline:
			t.Fatal("runner did not release the finalized task")
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

func TestRetryableControlPersistence(t *testing.T) {
	if !retryableControlPersistence(errors.New("temporary storage failure")) {
		t.Fatal("temporary storage failure should be retryable")
	}
	for _, err := range []error{
		nil,
		taskstate.ErrTaskNotFound,
		taskstate.ErrTaskStateConflict,
		taskstate.ErrTaskAlreadyActive,
		taskstate.ErrStaleTaskAttempt,
	} {
		if retryableControlPersistence(err) {
			t.Fatalf("semantic error should not be retryable: %v", err)
		}
	}
}

func TestRunnerStopsRetryingWhenTaskStoreCannotBeRead(t *testing.T) {
	store, err := storage.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	queue := taskqueue.NewQueue(taskstate.NewSQLiteRepository(store.DB()))
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
	queue := taskqueue.NewQueue(taskstate.NewSQLiteRepository(store.DB()))
	executor := &closingStoreExecutor{store: store, closed: make(chan struct{})}
	runner := NewRunner(Options{Queue: queue, Executor: executor, WorkInterval: time.Hour, Logger: slog.Default()})
	runner.Start(context.Background())
	task := taskqueue.NewTask("download")
	task.Target = "https://example.invalid/finalize-error"
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
