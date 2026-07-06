package taskqueue

import (
	"fmt"
	"strconv"
	"sync/atomic"
	"time"
)

type Status string

const (
	StatusQueued    Status = "queued"
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
	StatusCanceled  Status = "canceled"
)

type Task struct {
	ID                string
	Kind              string
	Targets           []string
	NovelIDs          []int
	Force             bool
	ForceRedownload   bool
	SkipUnchanged     bool
	Status            Status
	Message           string
	Warnings          []string
	ErrorMessage      string
	TargetLabel       string
	FailedEpisodeID   string
	ResumeEpisodeID   string
	SavedEpisodeCount int
	Phase             string
	CurrentStep       int
	TotalSteps        int
	CreatedAt         time.Time
	StartedAt         *time.Time
	FinishedAt        *time.Time
}

func NewTaskID(prefix string) string {
	return fmt.Sprintf("%s-%d-%d", prefix, time.Now().UnixNano(), taskIDSequence.Add(1))
}

func NewTask(kind string) *Task {
	return &Task{
		ID:        NewTaskID(kind),
		Kind:      kind,
		Status:    StatusQueued,
		CreatedAt: time.Now(),
	}
}

func TaskIDs(tasks []*Task) []string {
	ids := make([]string, 0, len(tasks))
	for _, task := range tasks {
		ids = append(ids, task.ID)
	}
	return ids
}

func IntIDsToStrings(ids []int) []string {
	values := make([]string, 0, len(ids))
	for _, id := range ids {
		values = append(values, strconv.Itoa(id))
	}
	return values
}

var taskIDSequence atomic.Uint64
