package taskqueue

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"narou-viewer/services/novel-fetcher/internal/sites"
	"narou-viewer/services/novel-fetcher/internal/taskstate"
)

type Queue struct {
	repository taskstate.Repository
	wake       chan struct{}

	mu                sync.Mutex
	queue             []*Task
	current           *Task
	recentComplete    []*Task
	recentFailed      []*Task
	recentPaused      []*Task
	recentInterrupted []*Task
	completedCount    int
	failedCount       int
	canceledCount     int
	pausedCount       int
	interruptedCount  int
}

func NewQueue() *Queue { return &Queue{wake: make(chan struct{}, 1)} }

func NewPersistentQueue(repository taskstate.Repository) *Queue {
	return &Queue{repository: repository, wake: make(chan struct{}, 1)}
}

func (q *Queue) Wake() <-chan struct{} { return q.wake }

func (q *Queue) Notify() {
	select {
	case q.wake <- struct{}{}:
	default:
	}
}

func (q *Queue) Enqueue(tasks ...*Task) error {
	if q.repository != nil {
		if _, err := q.repository.Enqueue(context.Background(), tasks); err != nil {
			return err
		}
		q.Notify()
		return nil
	}
	q.mu.Lock()
	q.queue = append(q.queue, tasks...)
	q.mu.Unlock()
	q.Notify()
	return nil
}

func (q *Queue) StatusCounts() StatusCounts {
	counts, _ := q.StatusCountsWithError()
	return counts
}

func (q *Queue) StatusCountsWithError() (StatusCounts, error) {
	if q.repository != nil {
		counts, err := q.repository.QueueCounts(context.Background())
		if err != nil {
			return StatusCounts{}, err
		}
		return StatusCounts{Total: counts.Total, Queued: counts.Queued, Running: counts.Running, Paused: counts.Paused, Interrupted: counts.Interrupted}, nil
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	total := len(q.queue)
	running := q.current != nil
	if running {
		total++
	}
	return StatusCounts{Total: total, Queued: len(q.queue), Running: running, Paused: len(q.recentPaused), Interrupted: len(q.recentInterrupted)}, nil
}

func (q *Queue) Summary() Summary {
	summary, _ := q.SummaryWithError()
	return summary
}

func (q *Queue) SummaryWithError() (Summary, error) {
	if q.repository != nil {
		state, err := q.repository.Summary(context.Background(), 20)
		if err != nil {
			return Summary{}, err
		}
		return Summary{
			Current: Payload(state.Current), Queued: Payloads(state.Queued), Paused: Payloads(state.Paused),
			Interrupted: Payloads(state.Interrupted), RecentCompleted: Payloads(state.RecentCompleted),
			RecentFailed: Payloads(state.RecentFailed), CompletedCount: state.CompletedCount, FailedCount: state.FailedCount,
			CanceledCount: state.CanceledCount, PausedCount: state.PausedCount, InterruptedCount: state.InterruptedCount,
		}, nil
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	return Summary{
		Current: Payload(q.current), Queued: Payloads(q.queue), Paused: Payloads(q.recentPaused), Interrupted: Payloads(q.recentInterrupted),
		RecentCompleted: Payloads(q.recentComplete), RecentFailed: Payloads(q.recentFailed), CompletedCount: q.completedCount,
		FailedCount: q.failedCount, CanceledCount: q.canceledCount, PausedCount: q.pausedCount, InterruptedCount: q.interruptedCount,
	}, nil
}

func (q *Queue) PopNext() *Task {
	task, _ := q.PopNextWithError()
	return task
}

func (q *Queue) PopNextWithError() (*Task, error) {
	if q.repository != nil {
		task, err := q.repository.ClaimNext(context.Background(), time.Now().UTC())
		if err != nil {
			return nil, err
		}
		return task, nil
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.queue) == 0 {
		return nil, nil
	}
	next := q.queue[0]
	q.queue = q.queue[1:]
	now := time.Now()
	next.StartedAt = &now
	next.Status = StatusRunning
	q.current = next
	return next, nil
}

func (q *Queue) HasQueuedTasks() bool {
	queued, _ := q.HasQueuedTasksWithError()
	return queued
}

func (q *Queue) HasQueuedTasksWithError() (bool, error) {
	if q.repository != nil {
		return q.repository.HasQueuedTasks(context.Background())
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.queue) > 0, nil
}

func (q *Queue) IsIdle() bool {
	counts := q.StatusCounts()
	return counts.Total == 0
}

func (q *Queue) SetTaskProgress(taskID string, progress sites.Progress) {
	if q.repository != nil {
		if task, found, err := q.repository.Get(context.Background(), taskID); err == nil && found && task.Status == StatusRunning {
			_ = q.repository.UpdateProgress(context.Background(), taskstate.TaskRef{TaskID: taskID, Attempt: task.AttemptCount}, taskstate.Progress{Phase: progress.Phase, CurrentStep: progress.CurrentStep, TotalSteps: progress.TotalSteps, Message: progress.Message})
		}
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.current == nil || q.current.ID != taskID {
		return
	}
	q.current.Phase, q.current.CurrentStep, q.current.TotalSteps = progress.Phase, progress.CurrentStep, progress.TotalSteps
	if progress.Message != "" {
		q.current.Message = progress.Message
	}
}

func (q *Queue) SetTaskMessage(taskID string, message string) {
	if q.repository != nil {
		if task, found, err := q.repository.Get(context.Background(), taskID); err == nil && found && task.Status == StatusRunning {
			_ = q.repository.UpdateMessage(context.Background(), taskstate.TaskRef{TaskID: taskID, Attempt: task.AttemptCount}, message)
		}
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.current != nil && q.current.ID == taskID {
		q.current.Message = message
	}
}

func (q *Queue) AddTaskWarning(taskID string, warning string) {
	if strings.TrimSpace(warning) == "" {
		return
	}
	if q.repository != nil {
		if task, found, err := q.repository.Get(context.Background(), taskID); err == nil && found && task.Status == StatusRunning {
			_ = q.repository.AddWarning(context.Background(), taskstate.TaskRef{TaskID: taskID, Attempt: task.AttemptCount}, warning)
		}
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.current == nil || q.current.ID != taskID {
		return
	}
	for _, existing := range q.current.Warnings {
		if existing == warning {
			return
		}
	}
	q.current.Warnings = append(q.current.Warnings, warning)
}

func (q *Queue) SetTaskTarget(taskID string, target string) {
	if q.repository != nil {
		if task, found, err := q.repository.Get(context.Background(), taskID); err == nil && found && task.Status == StatusRunning {
			_ = q.repository.SetTarget(context.Background(), taskstate.TaskRef{TaskID: taskID, Attempt: task.AttemptCount}, target)
		}
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.current != nil && q.current.ID == taskID {
		q.current.TargetLabel = target
	}
}

func (q *Queue) AddTaskNovelID(taskID string, novelID int) error {
	if novelID == 0 {
		return nil
	}
	if q.repository != nil {
		task, found, err := q.repository.Get(context.Background(), taskID)
		if err != nil {
			return err
		}
		if !found {
			return taskstate.ErrTaskNotFound
		}
		if task.Status != StatusRunning {
			return taskstate.ErrStaleTaskAttempt
		}
		return q.repository.AddNovelID(context.Background(), taskstate.TaskRef{TaskID: taskID, Attempt: task.AttemptCount}, novelID)
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.current == nil || q.current.ID != taskID {
		return nil
	}
	for _, existingID := range q.current.NovelIDs {
		if existingID == novelID {
			return nil
		}
	}
	q.current.NovelIDs = append(q.current.NovelIDs, novelID)
	return nil
}

func (q *Queue) SetTaskSavedEpisodeCount(taskID string, count int) {
	if q.repository != nil {
		if task, found, err := q.repository.Get(context.Background(), taskID); err == nil && found && task.Status == StatusRunning {
			_ = q.repository.SetSavedEpisodeCount(context.Background(), taskstate.TaskRef{TaskID: taskID, Attempt: task.AttemptCount}, count)
		}
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.current != nil && q.current.ID == taskID {
		q.current.SavedEpisodeCount = count
	}
}

func (q *Queue) SetTaskFailureEpisode(taskID string, failedEpisodeID string, resumeEpisodeID string) {
	if q.repository != nil {
		if task, found, err := q.repository.Get(context.Background(), taskID); err == nil && found && task.Status == StatusRunning {
			_ = q.repository.SetFailureEpisode(context.Background(), taskstate.TaskRef{TaskID: taskID, Attempt: task.AttemptCount}, failedEpisodeID, resumeEpisodeID)
		}
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.current != nil && q.current.ID == taskID {
		q.current.FailedEpisodeID, q.current.ResumeEpisodeID = failedEpisodeID, resumeEpisodeID
	}
}

func (q *Queue) FinishTask(done *Task, err error, logger *slog.Logger) error {
	if q.repository != nil {
		outcome := taskstate.Outcome{Status: StatusSucceeded, Error: err, ExecutionCommitted: done.ExecutionCommitted}
		setPersistentOutcomeFromError(&outcome, err)
		return q.repository.Finalize(context.Background(), taskstate.TaskRef{TaskID: done.ID, Attempt: done.AttemptCount}, outcome)
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	now := time.Now()
	setMemoryOutcomeFromError(done, err)
	done.FinishedAt = &now
	if done.Status == StatusCanceled || done.Status == StatusInterrupted || done.Status == StatusPaused {
		if done.Status == StatusCanceled {
			q.canceledCount++
			q.failedCount++
			q.recentFailed = appendRecent(q.recentFailed, done)
		} else if done.Status == StatusInterrupted {
			q.interruptedCount++
			q.recentInterrupted = appendRecent(q.recentInterrupted, done)
		} else {
			q.pausedCount++
			q.recentPaused = appendRecent(q.recentPaused, done)
		}
	} else if done.Status == StatusFailed {
		q.failedCount++
		q.recentFailed = appendRecent(q.recentFailed, done)
		if logger != nil {
			logger.Warn("task failed", "taskID", done.ID, "error", err)
		}
	} else {
		done.Status = StatusSucceeded
		if done.Message == "" {
			done.Message = "Task completed"
		}
		q.completedCount++
		q.recentComplete = appendRecent(q.recentComplete, done)
	}
	q.current = nil
	return nil
}

func setPersistentOutcomeFromError(outcome *taskstate.Outcome, err error) {
	outcome.Status = StatusSucceeded
	outcome.Message = "Task completed"
	switch {
	case errors.Is(err, taskstate.ErrTaskPauseRequested):
		outcome.Status, outcome.Message = StatusPaused, "Task paused"
	case errors.Is(err, taskstate.ErrTaskCancelRequested), errors.Is(err, context.Canceled):
		outcome.Status, outcome.Message = StatusCanceled, "Task cancelled"
	case errors.Is(err, taskstate.ErrRunnerShutdown):
		outcome.Status, outcome.Message = StatusInterrupted, "Task interrupted during process shutdown"
	case err != nil:
		outcome.Status, outcome.Message = StatusFailed, err.Error()
	}
}

func setMemoryOutcomeFromError(task *Task, err error) {
	task.Status = StatusSucceeded
	task.Message = "Task completed"
	switch {
	case errors.Is(err, taskstate.ErrTaskPauseRequested):
		task.Status, task.Message = StatusPaused, "Task paused"
	case errors.Is(err, taskstate.ErrTaskCancelRequested), errors.Is(err, context.Canceled):
		task.Status, task.Message = StatusCanceled, "Task cancelled"
	case errors.Is(err, taskstate.ErrRunnerShutdown):
		task.Status, task.Message = StatusInterrupted, "Task interrupted during process shutdown"
	case err != nil:
		task.Status, task.ErrorMessage, task.Message = StatusFailed, err.Error(), err.Error()
	}
}

func (q *Queue) IsCurrent(taskID string) bool {
	current, _ := q.IsCurrentWithError(taskID)
	return current
}

func (q *Queue) IsCurrentWithError(taskID string) (bool, error) {
	if q.repository != nil {
		task, found, err := q.repository.Get(context.Background(), taskID)
		return err == nil && found && task.Status == StatusRunning, err
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.current != nil && q.current.ID == taskID, nil
}

func (q *Queue) CancelQueued(taskID string) bool {
	if q.repository != nil {
		result, err := q.repository.RequestCancel(context.Background(), taskID)
		return err == nil && result.Changed
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	for index, queued := range q.queue {
		if queued.ID == taskID {
			now := time.Now()
			queued.Status, queued.FinishedAt, queued.Message = StatusCanceled, &now, "Task cancelled"
			q.queue = append(q.queue[:index], q.queue[index+1:]...)
			q.failedCount++
			q.canceledCount++
			q.recentFailed = appendRecent(q.recentFailed, queued)
			return true
		}
	}
	return false
}

func (q *Queue) RequestPause(taskID string) (taskstate.ControlResult, error) {
	if q.repository == nil {
		q.mu.Lock()
		defer q.mu.Unlock()
		for index, queued := range q.queue {
			if queued.ID != taskID {
				continue
			}
			now := time.Now()
			queued.Status, queued.PausedAt, queued.FinishedAt, queued.Message = StatusPaused, &now, nil, "Task paused"
			q.queue = append(q.queue[:index], q.queue[index+1:]...)
			q.pausedCount++
			q.recentPaused = appendRecent(q.recentPaused, queued)
			return taskstate.ControlResult{Task: queued, Changed: true}, nil
		}
		if q.current != nil && q.current.ID == taskID {
			return taskstate.ControlResult{Task: q.current, Changed: true}, nil
		}
		return taskstate.ControlResult{}, errors.New("task not found")
	}
	result, err := q.repository.RequestPause(context.Background(), taskID)
	if err == nil && result.Changed {
		q.Notify()
	}
	return result, err
}

func (q *Queue) RequestResume(taskID string) (taskstate.ControlResult, error) {
	if q.repository == nil {
		return taskstate.ControlResult{}, errors.New("resume is not supported by the in-memory task queue")
	}
	result, err := q.repository.RequestResume(context.Background(), taskID)
	if err == nil && result.Changed {
		q.Notify()
	}
	return result, err
}

func (q *Queue) RequestCancel(taskID string) (taskstate.ControlResult, error) {
	if q.repository == nil {
		q.mu.Lock()
		defer q.mu.Unlock()
		if q.current != nil && q.current.ID == taskID {
			return taskstate.ControlResult{Task: q.current, Changed: true}, nil
		}
		for _, queued := range q.queue {
			if queued.ID == taskID {
				return taskstate.ControlResult{Task: queued, Changed: true}, nil
			}
		}
		return taskstate.ControlResult{}, errors.New("task not found")
	}
	result, err := q.repository.RequestCancel(context.Background(), taskID)
	if err == nil && result.Changed {
		q.Notify()
	}
	return result, err
}

func (q *Queue) GetTask(taskID string) (*Task, bool, error) {
	if q.repository == nil {
		return nil, false, errors.New("persistent task queue is not configured")
	}
	return q.repository.Get(context.Background(), taskID)
}

func (q *Queue) RequestedAction(ref taskstate.TaskRef) taskstate.RequestedAction {
	action, _ := q.RequestedActionWithError(ref)
	return action
}

func (q *Queue) RequestedActionWithError(ref taskstate.TaskRef) (taskstate.RequestedAction, error) {
	if q.repository == nil {
		return taskstate.RequestedActionNone, nil
	}
	return q.repository.ReadRequestedAction(context.Background(), ref)
}

func appendRecent(tasks []*Task, entry *Task) []*Task {
	tasks = append([]*Task{entry}, tasks...)
	if len(tasks) > 20 {
		return tasks[:20]
	}
	return tasks
}
