package worker

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"narou-viewer/services/novel-fetcher/internal/taskqueue"
)

type Executor interface {
	RunTask(ctx context.Context, task *taskqueue.Task) error
}

type Runner struct {
	queue        *taskqueue.Queue
	executor     Executor
	workInterval time.Duration
	logger       *slog.Logger

	mu            sync.Mutex
	cancel        context.CancelFunc
	taskID        string
	taskCancel    context.CancelFunc
	pendingCancel map[string]struct{}
	done          chan struct{}
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
	runCtx, cancel := context.WithCancel(ctx)
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
		cancel()
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
	r.mu.Lock()
	if r.taskID == taskID && r.taskCancel != nil {
		r.taskCancel()
		r.mu.Unlock()
		return true
	}
	r.mu.Unlock()

	if r.queue.CancelQueued(taskID) {
		return true
	}

	if r.queue.IsCurrent(taskID) {
		r.markPendingCancel(taskID)
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

		taskCtx, cancel := context.WithCancel(ctx)
		r.setRunningTask(next.ID, cancel)
		err := r.executor.RunTask(taskCtx, next)
		cancel()
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

func (r *Runner) setRunningTask(taskID string, cancel context.CancelFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.taskID = taskID
	r.taskCancel = cancel
	if _, ok := r.pendingCancel[taskID]; ok {
		delete(r.pendingCancel, taskID)
		cancel()
	}
}

func (r *Runner) clearRunningTask(taskID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.taskID == taskID {
		r.taskID = ""
		r.taskCancel = nil
	}
}

func (r *Runner) markPendingCancel(taskID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.pendingCancel == nil {
		r.pendingCancel = map[string]struct{}{}
	}
	r.pendingCancel[taskID] = struct{}{}
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
