package worker

import (
	"context"
	"sync"
	"testing"
	"time"

	"narou-viewer/services/novel-fetcher/internal/taskqueue"
)

type blockingExecutor struct {
	entered chan struct{}
	once    sync.Once
	err     error
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
	if summary.Current != nil || summary.FailedCount != 1 {
		t.Fatalf("summary = %#v", summary)
	}
	if summary.RecentFailed[0]["status"] != taskqueue.StatusCanceled {
		t.Fatalf("failed payload = %#v", summary.RecentFailed[0])
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner.setRunningTask(next.ID, cancel)

	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("pending cancel was not applied when running task was set")
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
