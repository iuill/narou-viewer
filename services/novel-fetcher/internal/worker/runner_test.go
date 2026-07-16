package worker

import (
	"context"
	"errors"
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

	if !runner.Cancel(task.ID) {
		t.Fatal("running task was not cancelled")
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
	if !runner.Pause(task.ID) {
		t.Fatal("running task was not paused")
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
	if !runner.Pause(paused.ID) {
		t.Fatal("queued task was not paused")
	}
	canceled := taskqueue.NewTask("download")
	if err := queue.Enqueue(canceled); err != nil {
		t.Fatal(err)
	}
	if !runner.Cancel(canceled.ID) {
		t.Fatal("queued task was not canceled")
	}
	if summary := queue.Summary(); summary.FailedCount != 1 || len(summary.RecentFailed) != 1 || summary.PausedCount != 1 {
		t.Fatalf("queued control summary = %#v", summary)
	}
}

func TestRunnerCancelDuringCurrentTaskHandoff(t *testing.T) {
	queue := taskqueue.NewQueue()
	runner := NewRunner(Options{Queue: queue, Executor: &blockingExecutor{entered: make(chan struct{})}})
	task := taskqueue.NewTask("download")
	queue.Enqueue(task)

	next := queue.PopNext()
	if next == nil {
		t.Fatal("task was not popped")
	}

	if !runner.Cancel(next.ID) {
		t.Fatal("current task handoff cancel returned false")
	}

	ctx, cancel := context.WithCancelCause(context.Background())
	defer func() { cancel(nil) }()
	runner.setRunningTask(next.ID, cancel)

	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("pending cancel was not applied when running task was set")
	}
}

func TestRunnerAppliesPersistedControlDuringHandoff(t *testing.T) {
	store, err := storage.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	queue := taskqueue.NewPersistentQueue(taskstate.NewSQLiteRepository(store.DB()))
	task := taskqueue.NewTask("download")
	task.Targets = []string{"https://example.invalid/work"}
	if err := queue.Enqueue(task); err != nil {
		t.Fatal(err)
	}
	next := queue.PopNext()
	if next == nil {
		t.Fatal("task was not popped")
	}
	if result, err := queue.RequestCancel(task.ID); err != nil || !result.Changed {
		t.Fatalf("persisted cancel = %#v, err = %v", result, err)
	}
	ctx, cancel := context.WithCancelCause(context.Background())
	defer func() { cancel(nil) }()
	runner := NewRunner(Options{Queue: queue})
	runner.setRunningTask(next.ID, cancel)
	if !errors.Is(context.Cause(ctx), taskstate.ErrTaskCancelRequested) {
		t.Fatalf("handoff cause = %v", context.Cause(ctx))
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
