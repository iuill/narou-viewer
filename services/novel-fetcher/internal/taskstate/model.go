package taskstate

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"time"
)

var (
	// ErrTaskPauseRequested is the cancellation cause used when a running task
	// should persist its checkpoint and become resumable.
	ErrTaskPauseRequested = errors.New("task pause requested")
	// ErrTaskCancelRequested is the cancellation cause used for an explicit
	// user cancellation.
	ErrTaskCancelRequested = errors.New("task cancel requested")
	// ErrRunnerShutdown distinguishes process shutdown from user cancellation.
	ErrRunnerShutdown = errors.New("novel-fetcher runner shutdown")
)

type Status string

const (
	StatusQueued      Status = "queued"
	StatusRunning     Status = "running"
	StatusPaused      Status = "paused"
	StatusInterrupted Status = "interrupted"
	StatusFailed      Status = "failed"
	StatusCanceled    Status = "canceled"
	StatusSucceeded   Status = "succeeded"
)

const CurrentRequestVersion = 1

type RequestedAction string

const (
	RequestedActionNone   RequestedAction = ""
	RequestedActionPause  RequestedAction = "pause"
	RequestedActionCancel RequestedAction = "cancel"
)

type Task struct {
	ID                 string
	Kind               string
	Target             string
	WorkID             int
	Force              bool
	ForceRedownload    bool
	SkipUnchanged      bool
	Status             Status
	RequestedAction    RequestedAction
	AttemptCount       int
	QueuePosition      *int
	Message            string
	Warnings           []string
	ErrorMessage       string
	TargetLabel        string
	FailedEpisodeID    string
	ResumeEpisodeID    string
	SavedEpisodeCount  int
	Phase              string
	CurrentStep        int
	TotalSteps         int
	DedupeKey          string
	RequestFingerprint string
	CreatedAt          time.Time
	StartedAt          *time.Time
	PausedAt           *time.Time
	InterruptedAt      *time.Time
	FinishedAt         *time.Time
	UpdatedAt          time.Time
	ExecutionCommitted bool
}

type TaskRef struct {
	TaskID  string
	Attempt int
}

type Progress struct {
	Phase       string
	CurrentStep int
	TotalSteps  int
	Message     string
}

type Outcome struct {
	Status             Status
	Error              error
	Message            string
	ExecutionCommitted bool
}

type Summary struct {
	Current          *Task
	Queued           []*Task
	Paused           []*Task
	Interrupted      []*Task
	RecentCompleted  []*Task
	RecentFailed     []*Task
	CompletedCount   int
	FailedCount      int
	CanceledCount    int
	PausedCount      int
	InterruptedCount int
}

type QueueCounts struct {
	Total       int
	Queued      int
	Running     bool
	Paused      int
	Interrupted int
}

type EnqueueResult struct {
	Tasks           []*Task
	DeduplicatedIDs []string
}

type ControlResult struct {
	Task    *Task
	Changed bool
}

func NewTaskID(prefix string) string {
	bytes := make([]byte, 16)
	if _, err := taskIDRandomReader(bytes); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return fmt.Sprintf("%s-%d-%s", prefix, time.Now().UnixMilli(), hex.EncodeToString(bytes))
}

var taskIDRandomReader = rand.Read

func NewTask(kind string) *Task {
	now := time.Now().UTC()
	return &Task{ID: NewTaskID(kind), Kind: kind, Status: StatusQueued, CreatedAt: now, UpdatedAt: now}
}

func IntIDsToStrings(ids []int) []string {
	values := make([]string, 0, len(ids))
	for _, id := range ids {
		values = append(values, strconv.Itoa(id))
	}
	return values
}

func TaskIDs(tasks []*Task) []string {
	ids := make([]string, 0, len(tasks))
	for _, task := range tasks {
		ids = append(ids, task.ID)
	}
	return ids
}
