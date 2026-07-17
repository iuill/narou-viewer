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

	taskLocksMu   sync.Mutex
	taskLocks     map[string]*runnerTaskLock
	mu            sync.Mutex
	cancel        context.CancelCauseFunc
	taskRef       taskstate.TaskRef
	taskPhase     runnerPhase
	taskAction    taskstate.RequestedAction
	taskCancel    context.CancelCauseFunc
	controlDone   chan struct{}
	controlResult taskstate.ControlResult
	controlErr    error
	done          chan struct{}
}

type runnerTaskLock struct {
	mu   sync.Mutex
	refs int
}

type runnerPhase uint8

const (
	runnerPhaseIdle runnerPhase = iota
	runnerPhaseStarting
	runnerPhaseRunning
	runnerPhaseFinalizing
)

func (p runnerPhase) String() string {
	switch p {
	case runnerPhaseStarting:
		return "starting"
	case runnerPhaseRunning:
		return "running"
	case runnerPhaseFinalizing:
		return "finalizing"
	default:
		return "idle"
	}
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
	if r.taskRef.TaskID == taskID && r.taskPhase != runnerPhaseIdle {
		phase := r.taskPhase
		r.mu.Unlock()
		return taskstate.ControlResult{}, fmt.Errorf("%w: task %s cannot resume while runner phase is %s", taskstate.ErrTaskStateConflict, taskID, phase)
	}
	r.mu.Unlock()
	return r.queue.RequestResume(taskID)
}

// requestControl owns both the in-memory cancellation and the durable state
// change for the current task. Operations are serialized per task so a
// control waiting on SQLite for one task cannot block another task's context
// signal. Claim handoff uses the same task lock.
func (r *Runner) requestControl(taskID string, action taskstate.RequestedAction, cause error) (taskstate.ControlResult, error) {
	unlockTask := r.lockTask(taskID)
	defer unlockTask()

	r.mu.Lock()
	if r.taskRef.TaskID == taskID {
		switch r.taskPhase {
		case runnerPhaseFinalizing:
			if r.taskAction == action && r.controlDone != nil {
				result, err := r.controlResult, r.controlErr
				result.Changed = false
				r.mu.Unlock()
				return result, err
			}
			r.mu.Unlock()
			return taskstate.ControlResult{}, fmt.Errorf("%w: task %s is finalizing", taskstate.ErrTaskStateConflict, taskID)
		case runnerPhaseStarting:
			if r.taskAction != taskstate.RequestedActionNone && r.taskAction != action {
				current := r.taskAction
				r.mu.Unlock()
				return taskstate.ControlResult{}, fmt.Errorf("%w: task %s already has requested action %s", taskstate.ErrTaskStateConflict, taskID, current)
			}
			if r.taskAction == action && r.controlDone != nil {
				result, err := r.controlResult, r.controlErr
				result.Changed = false
				r.mu.Unlock()
				return result, err
			}
			r.mu.Unlock()
			result, err := r.persistControl(taskID, action)
			r.mu.Lock()
			if err == nil && result.Task != nil && result.Task.Status == taskqueue.StatusRunning {
				r.taskAction = action
				r.controlResult = result
				r.controlErr = nil
				r.controlDone = make(chan struct{})
				close(r.controlDone)
				if r.taskCancel != nil {
					r.taskCancel(cause)
				}
			}
			r.mu.Unlock()
			return result, err
		case runnerPhaseRunning:
			if r.taskAction != taskstate.RequestedActionNone && r.taskAction != action {
				current := r.taskAction
				r.mu.Unlock()
				return taskstate.ControlResult{}, fmt.Errorf("%w: task %s already has requested action %s", taskstate.ErrTaskStateConflict, taskID, current)
			}
			if r.taskAction == action && r.controlDone != nil {
				result, err := r.controlResult, r.controlErr
				result.Changed = false
				r.mu.Unlock()
				return result, err
			}

			newSignal := r.taskAction == taskstate.RequestedActionNone
			r.taskAction = action
			done := make(chan struct{})
			r.controlDone = done
			r.controlResult = taskstate.ControlResult{}
			r.controlErr = nil
			if newSignal && r.taskCancel != nil {
				// Signal before the SQLite write. Asset localization can own the
				// single writer connection, and cancellation is what releases it.
				r.taskCancel(cause)
			}
			r.mu.Unlock()

			result, err := r.persistControl(taskID, action)
			r.mu.Lock()
			if r.taskRef.TaskID == taskID && r.controlDone == done {
				r.controlResult = result
				r.controlErr = err
				close(done)
			}
			r.mu.Unlock()
			return result, err
		}
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
		err = r.beginFinalizing(ref, taskCtx, err)
		cancel(nil)
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
		r.clearTask(ref)

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

func (r *Runner) claimNext(ctx context.Context) (*taskqueue.Task, context.Context, context.CancelCauseFunc, error) {
	next, err := r.queue.PopNextWithError()
	if err != nil || next == nil {
		return next, nil, nil, err
	}
	ref := taskstate.TaskRef{TaskID: next.ID, Attempt: next.AttemptCount}
	unlockTask := r.lockTask(ref.TaskID)
	defer unlockTask()

	taskCtx, cancel := context.WithCancelCause(ctx)
	r.mu.Lock()
	r.taskRef = ref
	r.taskPhase = runnerPhaseStarting
	r.taskAction = taskstate.RequestedActionNone
	r.taskCancel = cancel
	r.controlDone = nil
	r.controlResult = taskstate.ControlResult{}
	r.controlErr = nil
	r.mu.Unlock()

	for {
		action, readErr := r.queue.RequestedActionWithError(ref)
		if readErr == nil {
			r.mu.Lock()
			if action == taskstate.RequestedActionNone {
				// The in-memory queue has no durable requested_action column. A
				// control accepted while starting is nevertheless authoritative.
				action = r.taskAction
			}
			r.taskAction = action
			r.taskPhase = runnerPhaseRunning
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

func (r *Runner) beginFinalizing(ref taskstate.TaskRef, taskCtx context.Context, executorErr error) error {
	r.mu.Lock()
	if r.taskRef != ref {
		r.mu.Unlock()
		return executorErr
	}
	r.taskPhase = runnerPhaseFinalizing
	done := r.controlDone
	r.mu.Unlock()

	if done != nil {
		<-done
	}

	r.mu.Lock()
	action, controlErr := r.taskAction, r.controlErr
	r.mu.Unlock()
	if controlErr != nil {
		return fmt.Errorf("persist requested task action: %w", controlErr)
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

func (r *Runner) clearTask(ref taskstate.TaskRef) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.taskRef != ref {
		return
	}
	r.taskRef = taskstate.TaskRef{}
	r.taskPhase = runnerPhaseIdle
	r.taskAction = taskstate.RequestedActionNone
	r.taskCancel = nil
	r.controlDone = nil
	r.controlResult = taskstate.ControlResult{}
	r.controlErr = nil
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
