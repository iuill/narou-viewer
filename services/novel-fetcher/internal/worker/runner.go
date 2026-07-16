package worker

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"narou-viewer/services/novel-fetcher/internal/taskqueue"
	"narou-viewer/services/novel-fetcher/internal/taskstate"
)

type Executor interface {
	RunTask(ctx context.Context, task *taskqueue.Task) error
}

type Runner struct {
	queue        *taskqueue.Queue
	executor     Executor
	workInterval time.Duration
	logger       *slog.Logger

	mu         sync.Mutex
	cancel     context.CancelCauseFunc
	taskID     string
	taskCancel context.CancelCauseFunc
	pending    map[string]error
	done       chan struct{}
}

type Options struct {
	Queue        *taskqueue.Queue
	Executor     Executor
	WorkInterval time.Duration
	Logger       *slog.Logger
}

func NewRunner(options Options) *Runner {
	return &Runner{
		queue:        options.Queue,
		executor:     options.Executor,
		workInterval: options.WorkInterval,
		logger:       options.Logger,
	}
}

func (r *Runner) Start(ctx context.Context) {
	r.mu.Lock()
	if r.done != nil {
		r.mu.Unlock()
		return
	}
	runCtx, cancel := context.WithCancelCause(ctx)
	r.cancel = cancel
	r.done = make(chan struct{})
	done := r.done
	r.mu.Unlock()

	go func() {
		defer close(done)
		r.loop(runCtx)
	}()
	r.queue.Notify()
}

func (r *Runner) Stop(ctx context.Context) {
	r.mu.Lock()
	cancel := r.cancel
	done := r.done
	r.mu.Unlock()

	if cancel != nil {
		cancel(taskstate.ErrRunnerShutdown)
	}
	if done == nil {
		return
	}

	select {
	case <-done:
	case <-ctx.Done():
	}
}

func (r *Runner) Cancel(taskID string) bool {
	return r.control(taskID, taskstate.ErrTaskCancelRequested)
}

func (r *Runner) Pause(taskID string) bool {
	return r.control(taskID, taskstate.ErrTaskPauseRequested)
}

func (r *Runner) control(taskID string, cause error) bool {
	r.mu.Lock()
	if r.taskID == taskID && r.taskCancel != nil {
		r.taskCancel(cause)
		r.mu.Unlock()
		return true
	}
	r.mu.Unlock()

	if cause == taskstate.ErrTaskCancelRequested {
		if r.queue.CancelQueued(taskID) {
			return true
		}
	} else if cause == taskstate.ErrTaskPauseRequested {
		if result, err := r.queue.RequestPause(taskID); err == nil && result.Changed {
			return true
		}
	}

	if r.queue.IsCurrent(taskID) {
		if cause == taskstate.ErrTaskCancelRequested {
			_, _ = r.queue.RequestCancel(taskID)
		} else {
			_, _ = r.queue.RequestPause(taskID)
		}
		r.markPending(taskID, cause)
		return true
	}

	return false
}

func (r *Runner) loop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-r.queue.Wake():
			if !r.drain(ctx) {
				return
			}
		}
	}
}

func (r *Runner) drain(ctx context.Context) bool {
	for {
		next := r.queue.PopNext()
		if next == nil {
			return true
		}

		taskCtx, cancel := context.WithCancelCause(ctx)
		r.setRunningTask(next.ID, cancel)
		err := r.executor.RunTask(taskCtx, next)
		if errors.Is(err, context.Canceled) {
			if cause := context.Cause(taskCtx); cause != nil {
				err = cause
			}
		}
		cancel(nil)
		r.clearRunningTask(next.ID)
		r.queue.FinishTask(next, err, r.logger)

		if !r.queue.HasQueuedTasks() {
			return true
		}
		if !r.waitForNextWork(ctx) {
			return false
		}
	}
}

func (r *Runner) setRunningTask(taskID string, cancel context.CancelCauseFunc) {
	r.mu.Lock()
	r.taskID = taskID
	r.taskCancel = cancel
	pending := r.pending[taskID]
	delete(r.pending, taskID)
	r.mu.Unlock()
	if pending != nil {
		cancel(pending)
		return
	}
	if action := r.queue.RequestedAction(taskstate.TaskRef{TaskID: taskID, Attempt: r.currentAttempt(taskID)}); action == taskstate.RequestedActionPause {
		cancel(taskstate.ErrTaskPauseRequested)
	} else if action == taskstate.RequestedActionCancel {
		cancel(taskstate.ErrTaskCancelRequested)
	}
}

func (r *Runner) currentAttempt(taskID string) int {
	if task, found, err := r.queue.GetTask(taskID); err == nil && found {
		return task.AttemptCount
	}
	return 0
}

func (r *Runner) clearRunningTask(taskID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.taskID == taskID {
		r.taskID = ""
		r.taskCancel = nil
	}
}

func (r *Runner) markPending(taskID string, cause error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.pending == nil {
		r.pending = map[string]error{}
	}
	r.pending[taskID] = cause
}

func (r *Runner) waitForNextWork(ctx context.Context) bool {
	if r.workInterval <= 0 {
		return true
	}

	timer := time.NewTimer(r.workInterval)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
