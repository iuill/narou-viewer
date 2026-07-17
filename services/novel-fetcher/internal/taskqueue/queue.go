package taskqueue

import (
	"context"
	"errors"
	"strings"
	"time"

	"narou-viewer/services/novel-fetcher/internal/sites"
	"narou-viewer/services/novel-fetcher/internal/taskstate"
)

// Queue adds wake-up notification and task reporting to the durable task
// repository. Task state itself always lives in the repository.
type Queue struct {
	repository taskstate.Repository
	wake       chan struct{}
}

func NewQueue(repository taskstate.Repository) *Queue {
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
	if _, err := q.repository.Enqueue(context.Background(), tasks); err != nil {
		return err
	}
	q.Notify()
	return nil
}

func (q *Queue) StatusCounts() (StatusCounts, error) {
	counts, err := q.repository.QueueCounts(context.Background())
	if err != nil {
		return StatusCounts{}, err
	}
	return StatusCounts{
		Total:       counts.Total,
		Queued:      counts.Queued,
		Running:     counts.Running,
		Paused:      counts.Paused,
		Interrupted: counts.Interrupted,
	}, nil
}

func (q *Queue) Summary() (Summary, error) {
	state, err := q.repository.Summary(context.Background(), 20)
	if err != nil {
		return Summary{}, err
	}
	return Summary{
		Current:          Payload(state.Current),
		Queued:           Payloads(state.Queued),
		Paused:           Payloads(state.Paused),
		Interrupted:      Payloads(state.Interrupted),
		RecentCompleted:  Payloads(state.RecentCompleted),
		RecentFailed:     Payloads(state.RecentFailed),
		CompletedCount:   state.CompletedCount,
		FailedCount:      state.FailedCount,
		CanceledCount:    state.CanceledCount,
		PausedCount:      state.PausedCount,
		InterruptedCount: state.InterruptedCount,
	}, nil
}

func (q *Queue) ClaimNext() (*Task, error) {
	return q.repository.ClaimNext(context.Background(), time.Now().UTC())
}

func (q *Queue) HasQueuedTasks() (bool, error) {
	return q.repository.HasQueuedTasks(context.Background())
}

func (q *Queue) SetTaskProgress(taskID string, progress sites.Progress) {
	if ref, err := q.runningRef(taskID); err == nil {
		_ = q.repository.UpdateProgress(context.Background(), ref, taskstate.Progress{
			Phase: progress.Phase, CurrentStep: progress.CurrentStep,
			TotalSteps: progress.TotalSteps, Message: progress.Message,
		})
	}
}

func (q *Queue) SetTaskMessage(taskID string, message string) {
	if ref, err := q.runningRef(taskID); err == nil {
		_ = q.repository.UpdateMessage(context.Background(), ref, message)
	}
}

func (q *Queue) AddTaskWarning(taskID string, warning string) {
	if strings.TrimSpace(warning) == "" {
		return
	}
	if ref, err := q.runningRef(taskID); err == nil {
		_ = q.repository.AddWarning(context.Background(), ref, warning)
	}
}

func (q *Queue) SetTaskTarget(taskID string, target string) {
	if ref, err := q.runningRef(taskID); err == nil {
		_ = q.repository.SetTarget(context.Background(), ref, target)
	}
}

func (q *Queue) AddTaskNovelID(taskID string, novelID int) error {
	if novelID == 0 {
		return nil
	}
	ref, err := q.runningRef(taskID)
	if err != nil {
		return err
	}
	return q.repository.AddNovelID(context.Background(), ref, novelID)
}

func (q *Queue) SetTaskSavedEpisodeCount(taskID string, count int) {
	if ref, err := q.runningRef(taskID); err == nil {
		_ = q.repository.SetSavedEpisodeCount(context.Background(), ref, count)
	}
}

func (q *Queue) SetTaskFailureEpisode(taskID string, failedEpisodeID string, resumeEpisodeID string) {
	if ref, err := q.runningRef(taskID); err == nil {
		_ = q.repository.SetFailureEpisode(context.Background(), ref, failedEpisodeID, resumeEpisodeID)
	}
}

func (q *Queue) FinishTask(done *Task, err error) error {
	outcome := taskstate.Outcome{Status: StatusSucceeded, Error: err, ExecutionCommitted: done.ExecutionCommitted}
	setOutcomeFromError(&outcome, err)
	return q.repository.Finalize(context.Background(), taskstate.TaskRef{TaskID: done.ID, Attempt: done.AttemptCount}, outcome)
}

func setOutcomeFromError(outcome *taskstate.Outcome, err error) {
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

func (q *Queue) RequestPause(taskID string) (taskstate.ControlResult, error) {
	result, err := q.repository.RequestPause(context.Background(), taskID)
	if err == nil && result.Changed {
		q.Notify()
	}
	return result, err
}

func (q *Queue) RequestResume(taskID string) (taskstate.ControlResult, error) {
	result, err := q.repository.RequestResume(context.Background(), taskID)
	if err == nil && result.Changed {
		q.Notify()
	}
	return result, err
}

func (q *Queue) RequestCancel(taskID string) (taskstate.ControlResult, error) {
	result, err := q.repository.RequestCancel(context.Background(), taskID)
	if err == nil && result.Changed {
		q.Notify()
	}
	return result, err
}

func (q *Queue) GetTask(taskID string) (*Task, bool, error) {
	return q.repository.Get(context.Background(), taskID)
}

func (q *Queue) RequestedAction(ref taskstate.TaskRef) (taskstate.RequestedAction, error) {
	return q.repository.ReadRequestedAction(context.Background(), ref)
}

func (q *Queue) runningRef(taskID string) (taskstate.TaskRef, error) {
	task, found, err := q.repository.Get(context.Background(), taskID)
	if err != nil {
		return taskstate.TaskRef{}, err
	}
	if !found {
		return taskstate.TaskRef{}, taskstate.ErrTaskNotFound
	}
	if task.Status != StatusRunning {
		return taskstate.TaskRef{}, taskstate.ErrStaleTaskAttempt
	}
	return taskstate.TaskRef{TaskID: taskID, Attempt: task.AttemptCount}, nil
}
