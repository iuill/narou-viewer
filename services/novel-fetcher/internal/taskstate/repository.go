package taskstate

import (
	"context"
	"time"
)

type Repository interface {
	Enqueue(ctx context.Context, tasks []*Task) (EnqueueResult, error)
	ClaimNext(ctx context.Context, now time.Time) (*Task, error)
	Summary(ctx context.Context, recentLimit int) (Summary, error)
	QueueCounts(ctx context.Context) (QueueCounts, error)
	HasQueuedTasks(ctx context.Context) (bool, error)
	Get(ctx context.Context, taskID string) (*Task, bool, error)
	RequestPause(ctx context.Context, taskID string) (ControlResult, error)
	RequestResume(ctx context.Context, taskID string) (ControlResult, error)
	RequestCancel(ctx context.Context, taskID string) (ControlResult, error)
	ReadRequestedAction(ctx context.Context, ref TaskRef) (RequestedAction, error)
	UpdateProgress(ctx context.Context, ref TaskRef, progress Progress) error
	UpdateMessage(ctx context.Context, ref TaskRef, message string) error
	AddWarning(ctx context.Context, ref TaskRef, warning string) error
	SetTarget(ctx context.Context, ref TaskRef, target string) error
	SetWorkID(ctx context.Context, ref TaskRef, workID int) error
	SetSavedEpisodeCount(ctx context.Context, ref TaskRef, count int) error
	SetFailureEpisode(ctx context.Context, ref TaskRef, failedEpisodeID string, resumeEpisodeID string) error
	Finalize(ctx context.Context, ref TaskRef, outcome Outcome) error
	RecoverOnStartup(ctx context.Context, now time.Time) error
}
