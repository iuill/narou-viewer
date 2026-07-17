package taskqueue

import "narou-viewer/services/novel-fetcher/internal/taskstate"

type Status = taskstate.Status

const (
	StatusQueued      = taskstate.StatusQueued
	StatusRunning     = taskstate.StatusRunning
	StatusPaused      = taskstate.StatusPaused
	StatusInterrupted = taskstate.StatusInterrupted
	StatusFailed      = taskstate.StatusFailed
	StatusCanceled    = taskstate.StatusCanceled
	StatusSucceeded   = taskstate.StatusSucceeded
)

type Task = taskstate.Task

func NewTask(kind string) *Task          { return taskstate.NewTask(kind) }
func TaskIDs(tasks []*Task) []string     { return taskstate.TaskIDs(tasks) }
func IntIDsToStrings(ids []int) []string { return taskstate.IntIDsToStrings(ids) }
