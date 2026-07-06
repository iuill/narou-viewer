package taskqueue

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"narou-viewer/services/novel-fetcher/internal/sites"
)

type Queue struct {
	mu             sync.Mutex
	queue          []*Task
	current        *Task
	recentComplete []*Task
	recentFailed   []*Task
	completedCount int
	failedCount    int
	wake           chan struct{}
}

func NewQueue() *Queue {
	return &Queue{wake: make(chan struct{}, 1)}
}

func (q *Queue) Wake() <-chan struct{} {
	return q.wake
}

func (q *Queue) Notify() {
	select {
	case q.wake <- struct{}{}:
	default:
	}
}

func (q *Queue) Enqueue(tasks ...*Task) {
	q.mu.Lock()
	q.queue = append(q.queue, tasks...)
	q.mu.Unlock()
	q.Notify()
}

func (q *Queue) StatusCounts() StatusCounts {
	q.mu.Lock()
	defer q.mu.Unlock()

	total := len(q.queue)
	running := q.current != nil
	if running {
		total++
	}
	return StatusCounts{Total: total, Running: running}
}

func (q *Queue) Summary() Summary {
	q.mu.Lock()
	defer q.mu.Unlock()

	return Summary{
		Current:         Payload(q.current),
		Queued:          Payloads(q.queue),
		RecentCompleted: Payloads(q.recentComplete),
		RecentFailed:    Payloads(q.recentFailed),
		CompletedCount:  q.completedCount,
		FailedCount:     q.failedCount,
	}
}

func (q *Queue) PopNext() *Task {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.queue) == 0 {
		return nil
	}

	next := q.queue[0]
	q.queue = q.queue[1:]
	now := time.Now()
	next.StartedAt = &now
	next.Status = StatusRunning
	q.current = next
	return next
}

func (q *Queue) HasQueuedTasks() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.queue) > 0
}

func (q *Queue) IsIdle() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.current == nil && len(q.queue) == 0
}

func (q *Queue) SetTaskProgress(taskID string, progress sites.Progress) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.current == nil || q.current.ID != taskID {
		return
	}

	q.current.Phase = progress.Phase
	q.current.CurrentStep = progress.CurrentStep
	q.current.TotalSteps = progress.TotalSteps
	if progress.Message != "" {
		q.current.Message = progress.Message
	}
}

func (q *Queue) SetTaskMessage(taskID string, message string) {
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
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.current != nil && q.current.ID == taskID {
		q.current.TargetLabel = target
	}
}

func (q *Queue) AddTaskNovelID(taskID string, novelID int) {
	if novelID == 0 {
		return
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	if q.current == nil || q.current.ID != taskID {
		return
	}
	for _, existingID := range q.current.NovelIDs {
		if existingID == novelID {
			return
		}
	}
	q.current.NovelIDs = append(q.current.NovelIDs, novelID)
}

func (q *Queue) SetTaskSavedEpisodeCount(taskID string, count int) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.current != nil && q.current.ID == taskID {
		q.current.SavedEpisodeCount = count
	}
}

func (q *Queue) SetTaskFailureEpisode(taskID string, failedEpisodeID string, resumeEpisodeID string) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.current != nil && q.current.ID == taskID {
		q.current.FailedEpisodeID = failedEpisodeID
		q.current.ResumeEpisodeID = resumeEpisodeID
	}
}

func (q *Queue) FinishTask(done *Task, err error, logger *slog.Logger) {
	q.mu.Lock()
	defer q.mu.Unlock()

	now := time.Now()
	done.FinishedAt = &now
	if errors.Is(err, context.Canceled) {
		done.Status = StatusCanceled
		done.Message = "Task cancelled"
		q.failedCount++
		q.recentFailed = appendRecent(q.recentFailed, done)
	} else if err != nil {
		done.Status = StatusFailed
		done.ErrorMessage = err.Error()
		q.failedCount++
		q.recentFailed = appendRecent(q.recentFailed, done)
		if logger != nil {
			logger.Warn("task failed", "taskID", done.ID, "error", err)
		}
	} else {
		done.Status = StatusCompleted
		if done.Message == "" {
			done.Message = "Task completed"
		}
		q.completedCount++
		q.recentComplete = appendRecent(q.recentComplete, done)
	}
	q.current = nil
}

func (q *Queue) IsCurrent(taskID string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	return q.current != nil && q.current.ID == taskID
}

func (q *Queue) CancelQueued(taskID string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	for index, queued := range q.queue {
		if queued.ID == taskID {
			now := time.Now()
			queued.Status = StatusCanceled
			queued.FinishedAt = &now
			queued.Message = "Task cancelled"
			q.queue = append(q.queue[:index], q.queue[index+1:]...)
			q.failedCount++
			q.recentFailed = appendRecent(q.recentFailed, queued)
			return true
		}
	}

	return false
}

func appendRecent(tasks []*Task, entry *Task) []*Task {
	tasks = append([]*Task{entry}, tasks...)
	if len(tasks) > 20 {
		return tasks[:20]
	}
	return tasks
}
