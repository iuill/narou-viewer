package taskqueue

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"narou-viewer/services/novel-fetcher/internal/sites"
	"narou-viewer/services/novel-fetcher/internal/storage"
	"narou-viewer/services/novel-fetcher/internal/taskstate"
)

func TestQueueTracksSummaryAndHistory(t *testing.T) {
	if NewTaskID("test") == "" {
		t.Fatal("NewTaskID returned an empty id")
	}
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

func TestPersistentQueueUsesRepositoryForLifecycleAndControls(t *testing.T) {
	store, err := storage.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	queue := NewPersistentQueue(taskstate.NewSQLiteRepository(store.DB()))
	task := NewTask("update")
	task.NovelIDs = []int{99}
	if err := queue.Enqueue(task); err != nil {
		t.Fatal(err)
	}
	if counts := queue.StatusCounts(); counts.Total != 1 || counts.Running {
		t.Fatalf("counts = %#v", counts)
	}
	claimed := queue.PopNext()
	if claimed == nil || !queue.IsCurrent(task.ID) {
		t.Fatal("persistent task was not claimed")
	}
	queue.SetTaskProgress(task.ID, sites.Progress{Phase: "episode", CurrentStep: 1, TotalSteps: 2, Message: "half"})
	queue.SetTaskMessage(task.ID, "saved")
	queue.AddTaskWarning(task.ID, "warning")
	queue.AddTaskWarning(task.ID, "warning")
	queue.SetTaskTarget(task.ID, "作品")
	queue.AddTaskNovelID(task.ID, 100)
	queue.SetTaskSavedEpisodeCount(task.ID, 1)
	queue.SetTaskFailureEpisode(task.ID, "2", "2")
	if _, err := queue.RequestCancel(task.ID); err != nil {
		t.Fatal(err)
	}
	queue.FinishTask(claimed, context.Canceled, slog.Default())
	summary := queue.Summary()
	if summary.Current != nil || len(summary.RecentFailed) != 1 || summary.RecentFailed[0]["status"] != StatusCanceled {
		t.Fatalf("summary = %#v", summary)
	}

	queued := NewTask("download")
	queued.Targets = []string{"https://example.com/queued"}
	if err := queue.Enqueue(queued); err != nil {
		t.Fatal(err)
	}
	if result, err := queue.RequestPause(queued.ID); err != nil || !result.Changed {
		t.Fatalf("pause = %#v, err = %v", result, err)
	}
	if result, err := queue.RequestResume(queued.ID); err != nil || !result.Changed {
		t.Fatalf("resume = %#v, err = %v", result, err)
	}
	if _, found, err := queue.GetTask(queued.ID); err != nil || !found {
		t.Fatalf("GetTask found=%v err=%v", found, err)
	}
	claimed = queue.PopNext()
	if claimed == nil {
		t.Fatal("resumed task was not claimed")
	}
	if action := queue.RequestedAction(taskstate.TaskRef{TaskID: claimed.ID, Attempt: claimed.AttemptCount}); action != taskstate.RequestedActionNone {
		t.Fatalf("requested action = %q", action)
	}
	queue.FinishTask(claimed, taskstate.ErrTaskPauseRequested, slog.Default())
	if summary := queue.Summary(); len(summary.Paused) != 1 || summary.Paused[0]["status"] != StatusPaused {
		t.Fatalf("paused summary = %#v", summary)
	}
	if result, err := queue.RequestResume(queued.ID); err != nil || !result.Changed {
		t.Fatalf("resume paused task = %#v, err = %v", result, err)
	}
	claimed = queue.PopNext()
	if claimed == nil {
		t.Fatal("resumed paused task was not claimed")
	}
	queue.FinishTask(claimed, taskstate.ErrRunnerShutdown, slog.Default())
	if summary := queue.Summary(); len(summary.Interrupted) != 1 || summary.Interrupted[0]["status"] != StatusInterrupted {
		t.Fatalf("interrupted summary = %#v", summary)
	}
}

func TestMemoryQueueSupportsQueuedPauseAndCurrentControls(t *testing.T) {
	queue := NewQueue()
	paused := NewTask("download")
	if err := queue.Enqueue(paused); err != nil {
		t.Fatal(err)
	}
	result, err := queue.RequestPause(paused.ID)
	if err != nil || !result.Changed || result.Task.Status != StatusPaused {
		t.Fatalf("queued pause = %#v, err = %v", result, err)
	}
	canceled := NewTask("download")
	if err := queue.Enqueue(canceled); err != nil {
		t.Fatal(err)
	}
	if result, err := queue.RequestCancel(canceled.ID); err != nil || !result.Changed || result.Task.Status != StatusCanceled {
		t.Fatalf("queued cancel = %#v, err = %v", result, err)
	}
	current := NewTask("download")
	if err := queue.Enqueue(current); err != nil {
		t.Fatal(err)
	}
	if queue.PopNext() == nil {
		t.Fatal("current task was not popped")
	}
	if result, err := queue.RequestCancel(current.ID); err != nil || !result.Changed {
		t.Fatalf("current cancel = %#v, err = %v", result, err)
	}
}

func TestMemoryQueueRejectsDurableControls(t *testing.T) {
	queue := NewQueue()
	if _, err := queue.RequestPause("task"); err == nil {
		t.Fatal("memory pause unexpectedly succeeded")
	}
	if _, err := queue.RequestResume("task"); err == nil {
		t.Fatal("memory resume unexpectedly succeeded")
	}
	if _, err := queue.RequestCancel("task"); err == nil {
		t.Fatal("memory cancel unexpectedly succeeded")
	}
	if _, _, err := queue.GetTask("task"); err == nil {
		t.Fatal("memory get unexpectedly succeeded")
	}
}

func TestPersistentQueueErrorAwareReadsDoNotReportClosedStoreAsEmpty(t *testing.T) {
	store, err := storage.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	queue := NewPersistentQueue(taskstate.NewSQLiteRepository(store.DB()))
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := queue.StatusCountsWithError(); err == nil {
		t.Fatal("closed store was reported as empty status counts")
	}
	if _, err := queue.SummaryWithError(); err == nil {
		t.Fatal("closed store was reported as an empty summary")
	}
	if _, err := queue.PopNextWithError(); err == nil {
		t.Fatal("closed store was reported as no next task")
	}
	if _, err := queue.HasQueuedTasksWithError(); err == nil {
		t.Fatal("closed store was reported as no queued tasks")
	}
	if err := queue.AddTaskNovelID("missing", 1); err == nil {
		t.Fatal("closed store accepted a task identity update")
	}
}

func TestPersistentQueueRejectsIdentityUpdateForNonRunningTask(t *testing.T) {
	store, err := storage.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	queue := NewPersistentQueue(taskstate.NewSQLiteRepository(store.DB()))
	task := NewTask("download")
	task.Targets = []string{"https://example.invalid/queued"}
	if err := queue.Enqueue(task); err != nil {
		t.Fatal(err)
	}
	if err := queue.AddTaskNovelID(task.ID, 1); !errors.Is(err, taskstate.ErrStaleTaskAttempt) {
		t.Fatalf("queued identity update error = %v", err)
	}
	if err := queue.AddTaskNovelID(task.ID, 0); err != nil {
		t.Fatalf("zero identity update error = %v", err)
	}
}
