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

	taskLocksMu    sync.Mutex
	taskLocks      map[string]*runnerTaskLock
	mu             sync.Mutex
	cancel         context.CancelCauseFunc
	taskRef        taskstate.TaskRef
	taskFinalizing bool
	taskAction     taskstate.RequestedAction
	taskCancel     context.CancelCauseFunc
	done           chan struct{}
}

type runnerTaskLock struct {
	mu   sync.Mutex
	refs int
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

func (r *Runner) RequestCancel(taskID string) (taskstate.ControlResult, error) {
	return r.requestControl(taskID, taskstate.RequestedActionCancel, taskstate.ErrTaskCancelRequested)
}

func (r *Runner) RequestPause(taskID string) (taskstate.ControlResult, error) {
	return r.requestControl(taskID, taskstate.RequestedActionPause, taskstate.ErrTaskPauseRequested)
}

func (r *Runner) RequestResume(taskID string) (taskstate.ControlResult, error) {
	unlockTask := r.lockTask(taskID)
	defer unlockTask()

	r.mu.Lock()
	if r.taskRef.TaskID == taskID && r.taskFinalizing {
		r.mu.Unlock()
		return taskstate.ControlResult{}, fmt.Errorf("%w: task %s is finalizing", taskstate.ErrTaskStateConflict, taskID)
	}
	r.mu.Unlock()
	return r.queue.RequestResume(taskID)
}

// requestControl serializes the cancellation signal and durable action per
// task. A SQLite wait for one task therefore cannot block another task's
// cancellation signal. Claim handoff uses the same task lock.
func (r *Runner) requestControl(taskID string, action taskstate.RequestedAction, cause error) (taskstate.ControlResult, error) {
	unlockTask := r.lockTask(taskID)
	defer unlockTask()

	r.mu.Lock()
	if r.taskRef.TaskID == taskID {
		if r.taskAction != taskstate.RequestedActionNone && r.taskAction != action {
			current := r.taskAction
			r.mu.Unlock()
			return taskstate.ControlResult{}, fmt.Errorf("%w: task %s already has requested action %s", taskstate.ErrTaskStateConflict, taskID, current)
		}
		if r.taskFinalizing && r.taskAction == taskstate.RequestedActionNone {
			r.mu.Unlock()
			return taskstate.ControlResult{}, fmt.Errorf("%w: task %s is finalizing", taskstate.ErrTaskStateConflict, taskID)
		}

		newSignal := r.taskAction == taskstate.RequestedActionNone
		r.taskAction = action
		if newSignal && r.taskCancel != nil {
			// Signal before the SQLite write. Asset localization can own the
			// single writer connection, and cancellation is what releases it.
			r.taskCancel(cause)
		}
		r.mu.Unlock()
		return r.persistControl(taskID, action)
	}
	r.mu.Unlock()
	return r.persistControl(taskID, action)
}

func (r *Runner) lockTask(taskID string) func() {
	r.taskLocksMu.Lock()
	if r.taskLocks == nil {
		r.taskLocks = map[string]*runnerTaskLock{}
	}
	lock := r.taskLocks[taskID]
	if lock == nil {
		lock = &runnerTaskLock{}
		r.taskLocks[taskID] = lock
	}
	lock.refs++
	r.taskLocksMu.Unlock()

	lock.mu.Lock()
	return func() {
		lock.mu.Unlock()
		r.taskLocksMu.Lock()
		lock.refs--
		if lock.refs == 0 {
			delete(r.taskLocks, taskID)
		}
		r.taskLocksMu.Unlock()
	}
}

func (r *Runner) persistControl(taskID string, action taskstate.RequestedAction) (taskstate.ControlResult, error) {
	if action == taskstate.RequestedActionCancel {
		return r.queue.RequestCancel(taskID)
	}
	return r.queue.RequestPause(taskID)
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
		next, taskCtx, cancel, err := r.claimNext(ctx)
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

		ref := taskstate.TaskRef{TaskID: next.ID, Attempt: next.AttemptCount}
		err = r.executor.RunTask(taskCtx, next)
		err = r.beginFinalizing(ctx, ref, taskCtx, err)
		cancel(nil)
		for {
			finalizeErr := r.queue.FinishTask(next, err)
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
		r.clearTask(ref)

		hasQueued, queueErr := r.queue.HasQueuedTasks()
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

func (r *Runner) claimNext(ctx context.Context) (*taskqueue.Task, context.Context, context.CancelCauseFunc, error) {
	next, err := r.queue.ClaimNext()
	if err != nil || next == nil {
		return next, nil, nil, err
	}
	ref := taskstate.TaskRef{TaskID: next.ID, Attempt: next.AttemptCount}
	unlockTask := r.lockTask(ref.TaskID)
	defer unlockTask()

	taskCtx, cancel := context.WithCancelCause(ctx)
	for {
		action, readErr := r.queue.RequestedAction(ref)
		if readErr == nil {
			r.mu.Lock()
			r.taskRef = ref
			r.taskAction = action
			r.taskFinalizing = false
			r.taskCancel = cancel
			if action == taskstate.RequestedActionPause {
				cancel(taskstate.ErrTaskPauseRequested)
			} else if action == taskstate.RequestedActionCancel {
				cancel(taskstate.ErrTaskCancelRequested)
			}
			r.mu.Unlock()
			return next, taskCtx, cancel, nil
		}
		if r.logger != nil {
			r.logger.Error("failed to read requested task action", "taskID", ref.TaskID, "error", readErr)
		}
		if !r.waitForRetry(ctx) {
			cancel(nil)
			return nil, nil, nil, readErr
		}
	}
}

func (r *Runner) beginFinalizing(ctx context.Context, ref taskstate.TaskRef, taskCtx context.Context, executorErr error) error {
	// Finalization and same-task controls share the task lock. Once the
	// executor has stopped, any signaled action must be durable before the
	// terminal outcome is chosen.
	unlockTask := r.lockTask(ref.TaskID)
	defer unlockTask()

	r.mu.Lock()
	if r.taskRef != ref {
		r.mu.Unlock()
		return executorErr
	}
	r.taskFinalizing = true
	action := r.taskAction
	r.mu.Unlock()

	for action != taskstate.RequestedActionNone {
		_, controlErr := r.persistControl(ref.TaskID, action)
		if controlErr == nil {
			break
		}
		if !retryableControlPersistence(controlErr) {
			return fmt.Errorf("persist requested task action: %w", controlErr)
		}
		if r.logger != nil {
			r.logger.Error("failed to persist requested task action; retrying", "taskID", ref.TaskID, "action", action, "error", controlErr)
		}
		if !r.waitForRetry(ctx) {
			if cause := context.Cause(ctx); cause != nil {
				return cause
			}
			return ctx.Err()
		}
	}

	switch action {
	case taskstate.RequestedActionPause:
		return taskstate.ErrTaskPauseRequested
	case taskstate.RequestedActionCancel:
		return taskstate.ErrTaskCancelRequested
	}
	if errors.Is(executorErr, context.Canceled) {
		if cause := context.Cause(taskCtx); cause != nil {
			return cause
		}
	}
	return executorErr
}

func retryableControlPersistence(err error) bool {
	return err != nil &&
		!errors.Is(err, taskstate.ErrTaskNotFound) &&
		!errors.Is(err, taskstate.ErrTaskStateConflict) &&
		!errors.Is(err, taskstate.ErrTaskAlreadyActive) &&
		!errors.Is(err, taskstate.ErrStaleTaskAttempt)
}

func (r *Runner) clearTask(ref taskstate.TaskRef) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.taskRef != ref {
		return
	}
	r.taskRef = taskstate.TaskRef{}
	r.taskFinalizing = false
	r.taskAction = taskstate.RequestedActionNone
	r.taskCancel = nil
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
