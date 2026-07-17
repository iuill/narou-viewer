package worker

import (
	"context"
	"errors"
	"fmt"
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
	taskRef    taskstate.TaskRef
	taskAction taskstate.RequestedAction
	taskCancel context.CancelCauseFunc
	pending    map[taskstate.TaskRef]error
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

// SignalCancel and SignalPause cancel only the in-memory execution context.
// The HTTP control path uses them before the durable state write so that a
// task blocked in storage-owned HTTP work can release the single SQLite
// writer connection needed to persist the requested action.
func (r *Runner) SignalCancel(taskID string) (bool, error) {
	return r.signalRunning(taskID, taskstate.RequestedActionCancel, taskstate.ErrTaskCancelRequested)
}

func (r *Runner) SignalPause(taskID string) (bool, error) {
	return r.signalRunning(taskID, taskstate.RequestedActionPause, taskstate.ErrTaskPauseRequested)
}

func (r *Runner) signalRunning(taskID string, action taskstate.RequestedAction, cause error) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.taskRef.TaskID != taskID || r.taskCancel == nil {
		return false, nil
	}
	if r.taskAction != taskstate.RequestedActionNone {
		if r.taskAction != action {
			return false, fmt.Errorf("%w: task %s already has requested action %s", taskstate.ErrTaskStateConflict, taskID, r.taskAction)
		}
		return true, nil
	}
	r.taskAction = action
	r.taskCancel(cause)
	return true, nil
}

func (r *Runner) control(taskID string, cause error) bool {
	action := taskstate.RequestedActionPause
	if cause == taskstate.ErrTaskCancelRequested {
		action = taskstate.RequestedActionCancel
	}
	if signaled, err := r.signalRunning(taskID, action, cause); err != nil {
		return false
	} else if signaled {
		return true
	}

	if cause == taskstate.ErrTaskCancelRequested {
		if r.queue.CancelQueued(taskID) {
			return true
		}
	} else if cause == taskstate.ErrTaskPauseRequested {
		if result, err := r.queue.RequestPause(taskID); err == nil && result.Changed {
			return true
		}
	}

	current, err := r.queue.IsCurrentWithError(taskID)
	if err != nil {
		if r.logger != nil {
			r.logger.Error("failed to read current task state", "taskID", taskID, "error", err)
		}
		return false
	}
	if current {
		var result taskstate.ControlResult
		if cause == taskstate.ErrTaskCancelRequested {
			result, err = r.queue.RequestCancel(taskID)
		} else {
			result, err = r.queue.RequestPause(taskID)
		}
		if err != nil || result.Task == nil || result.Task.Status != taskqueue.StatusRunning {
			return false
		}
		r.markPending(taskstate.TaskRef{TaskID: taskID, Attempt: result.Task.AttemptCount}, cause)
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
		next, err := r.queue.PopNextWithError()
		if err != nil {
			if r.logger != nil {
				r.logger.Error("failed to claim next task", "error", err)
			}
			if !r.waitForRetry(ctx) {
				return false
			}
			continue
		}
		if next == nil {
			return true
		}

		taskCtx, cancel := context.WithCancelCause(ctx)
		ref := taskstate.TaskRef{TaskID: next.ID, Attempt: next.AttemptCount}
		r.setRunningTask(ref, cancel)
		err = r.executor.RunTask(taskCtx, next)
		if errors.Is(err, context.Canceled) {
			if cause := context.Cause(taskCtx); cause != nil {
				err = cause
			}
		}
		cancel(nil)
		r.clearRunningTask(ref)
		for {
			finalizeErr := r.queue.FinishTask(next, err, r.logger)
			if finalizeErr == nil {
				break
			}
			if r.logger != nil {
				r.logger.Error("task finalization failed", "taskID", next.ID, "error", finalizeErr)
			}
			if !r.waitForRetry(ctx) {
				return false
			}
		}

		hasQueued, queueErr := r.queue.HasQueuedTasksWithError()
		if queueErr != nil {
			if r.logger != nil {
				r.logger.Error("failed to read queued task state", "error", queueErr)
			}
			if !r.waitForRetry(ctx) {
				return false
			}
			continue
		}
		if !hasQueued {
			return true
		}
		if !r.waitForNextWork(ctx) {
			return false
		}
	}
}

func (r *Runner) setRunningTask(ref taskstate.TaskRef, cancel context.CancelCauseFunc) {
	r.mu.Lock()
	r.taskRef = ref
	r.taskAction = taskstate.RequestedActionNone
	r.taskCancel = cancel
	for pendingRef := range r.pending {
		if pendingRef.TaskID == ref.TaskID && pendingRef != ref {
			delete(r.pending, pendingRef)
		}
	}
	pending := r.pending[ref]
	delete(r.pending, ref)
	if pending != nil {
		r.taskAction = requestedActionForCause(pending)
	}
	r.mu.Unlock()
	if pending != nil {
		cancel(pending)
		return
	}
	action, err := r.queue.RequestedActionWithError(ref)
	if err != nil {
		if r.logger != nil {
			r.logger.Error("failed to read requested task action", "taskID", ref.TaskID, "error", err)
		}
		return
	}
	r.mu.Lock()
	if r.taskRef != ref {
		r.mu.Unlock()
		return
	}
	if r.taskAction == taskstate.RequestedActionNone {
		r.taskAction = action
	}
	effectiveAction := r.taskAction
	r.mu.Unlock()
	if effectiveAction == taskstate.RequestedActionPause {
		cancel(taskstate.ErrTaskPauseRequested)
	} else if effectiveAction == taskstate.RequestedActionCancel {
		cancel(taskstate.ErrTaskCancelRequested)
	}
}

func (r *Runner) clearRunningTask(ref taskstate.TaskRef) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.taskRef == ref {
		r.taskRef = taskstate.TaskRef{}
		r.taskAction = taskstate.RequestedActionNone
		r.taskCancel = nil
	}
}

func (r *Runner) markPending(ref taskstate.TaskRef, cause error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.pending == nil {
		r.pending = map[taskstate.TaskRef]error{}
	}
	r.pending[ref] = cause
}

func requestedActionForCause(cause error) taskstate.RequestedAction {
	if errors.Is(cause, taskstate.ErrTaskCancelRequested) {
		return taskstate.RequestedActionCancel
	}
	return taskstate.RequestedActionPause
}

func (r *Runner) waitForNextWork(ctx context.Context) bool {
	if r.workInterval <= 0 {
		return true
	}
	return waitForDuration(ctx, r.workInterval)
}

func (r *Runner) waitForRetry(ctx context.Context) bool {
	delay := r.workInterval
	if delay <= 0 {
		delay = 100 * time.Millisecond
	}
	return waitForDuration(ctx, delay)
}

func waitForDuration(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
