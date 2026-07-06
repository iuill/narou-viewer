package taskqueue

import (
	"context"
	"errors"
	"testing"

	"narou-viewer/services/novel-fetcher/internal/sites"
)

func TestQueueTracksSummaryAndHistory(t *testing.T) {
	queue := NewQueue()
	task := NewTask("download")
	task.Targets = []string{"https://example.com/work"}
	queue.Enqueue(task)
	if len(TaskIDs([]*Task{task})) != 1 {
		t.Fatal("TaskIDs did not return the task id")
	}
	select {
	case <-queue.Wake():
	default:
		t.Fatal("enqueue did not notify wake channel")
	}
	if !queue.HasQueuedTasks() || queue.IsIdle() {
		t.Fatal("queue should report pending work")
	}

	counts := queue.StatusCounts()
	if counts.Total != 1 || counts.Running {
		t.Fatalf("counts = %#v, want one queued task", counts)
	}

	next := queue.PopNext()
	if next == nil || next.ID != task.ID || next.Status != StatusRunning {
		t.Fatalf("next = %#v", next)
	}
	queue.SetTaskProgress(task.ID, sites.Progress{Phase: "episode", CurrentStep: 1, TotalSteps: 2, Message: "half"})
	queue.SetTaskTarget(task.ID, "作品")
	queue.SetTaskMessage(task.ID, "saved")
	queue.AddTaskNovelID(task.ID, 10)
	queue.AddTaskNovelID(task.ID, 10)
	queue.SetTaskSavedEpisodeCount(task.ID, 1)
	queue.FinishTask(task, nil, nil)

	summary := queue.Summary()
	if summary.Current != nil || summary.CompletedCount != 1 || summary.FailedCount != 0 {
		t.Fatalf("summary = %#v", summary)
	}
	if len(summary.RecentCompleted) != 1 {
		t.Fatalf("recent completed = %#v", summary.RecentCompleted)
	}
	payload := summary.RecentCompleted[0]
	if payload["status"] != StatusCompleted || payload["phase"] != "episode" || payload["progress"] != float64(50) {
		t.Fatalf("payload = %#v", payload)
	}
	if ids := payload["novel_ids"].([]string); len(ids) != 1 || ids[0] != "10" {
		t.Fatalf("novel ids = %#v", payload["novel_ids"])
	}
	if !queue.IsIdle() {
		t.Fatal("queue should be idle after finishing task")
	}
}

func TestQueueCancelsQueuedTasks(t *testing.T) {
	queue := NewQueue()
	queued := NewTask("download")
	queued.ID = "queued"
	queue.Enqueue(queued)

	if !queue.CancelQueued("queued") {
		t.Fatal("queued task was not cancelled")
	}
	summary := queue.Summary()
	if summary.FailedCount != 1 || len(summary.RecentFailed) != 1 {
		t.Fatalf("summary after queued cancel = %#v", summary)
	}
}

func TestQueueRecordsFailedTaskError(t *testing.T) {
	queue := NewQueue()
	task := NewTask("update")
	queue.Enqueue(task)
	next := queue.PopNext()
	queue.SetTaskFailureEpisode(next.ID, "2", "2")
	queue.FinishTask(next, errors.New("boom"), nil)

	summary := queue.Summary()
	if summary.FailedCount != 1 || len(summary.RecentFailed) != 1 {
		t.Fatalf("summary = %#v", summary)
	}
	if summary.RecentFailed[0]["error"] != "boom" {
		t.Fatalf("failed payload = %#v", summary.RecentFailed[0])
	}
	if summary.RecentFailed[0]["failed_episode_id"] != "2" || summary.RecentFailed[0]["resume_episode_id"] != "2" {
		t.Fatalf("failed episode payload = %#v", summary.RecentFailed[0])
	}
}

func TestQueueRecordsCanceledRunningTask(t *testing.T) {
	queue := NewQueue()
	task := NewTask("download")
	queue.Enqueue(task)
	next := queue.PopNext()
	queue.FinishTask(next, context.Canceled, nil)

	summary := queue.Summary()
	if summary.FailedCount != 1 || len(summary.RecentFailed) != 1 {
		t.Fatalf("summary = %#v", summary)
	}
	if summary.RecentFailed[0]["status"] != StatusCanceled || summary.RecentFailed[0]["message"] != "Task cancelled" {
		t.Fatalf("canceled payload = %#v", summary.RecentFailed[0])
	}
}

func TestQueueAddsCurrentTaskWarningsOnce(t *testing.T) {
	queue := NewQueue()
	task := NewTask("resume")
	task.ID = "current"
	queue.Enqueue(task)
	queue.PopNext()

	queue.AddTaskWarning("missing", "ignored")
	queue.AddTaskWarning("current", "  ")
	queue.AddTaskWarning("current", "episode 2 failed")
	queue.AddTaskWarning("current", "episode 2 failed")

	summary := queue.Summary()
	warnings := summary.Current["warnings"].([]string)
	if len(warnings) != 1 || warnings[0] != "episode 2 failed" {
		t.Fatalf("warnings = %#v", warnings)
	}
}

func TestQueueIsCurrent(t *testing.T) {
	queue := NewQueue()
	task := NewTask("download")
	task.ID = "current"
	queue.Enqueue(task)
	if queue.IsCurrent("current") {
		t.Fatal("queued task should not be current before PopNext")
	}
	queue.PopNext()
	if !queue.IsCurrent("current") {
		t.Fatal("running task should be current")
	}
	if queue.IsCurrent("other") {
		t.Fatal("different task id should not be current")
	}
}
