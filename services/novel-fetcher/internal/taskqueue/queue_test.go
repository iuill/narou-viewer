package taskqueue

import (
	"context"
	"errors"
	"testing"

	"narou-viewer/services/novel-fetcher/internal/sites"
	"narou-viewer/services/novel-fetcher/internal/storage"
	"narou-viewer/services/novel-fetcher/internal/taskstate"
)

func TestQueueUsesRepositoryForLifecycleAndControls(t *testing.T) {
	store, err := storage.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	queue := NewQueue(taskstate.NewSQLiteRepository(store.DB()))
	task := NewTask("update")
	task.WorkID = 99
	if err := queue.Enqueue(task); err != nil {
		t.Fatal(err)
	}
	if ids := TaskIDs([]*Task{task}); len(ids) != 1 || ids[0] != task.ID {
		t.Fatalf("task ids = %#v", ids)
	}
	select {
	case <-queue.Wake():
	default:
		t.Fatal("enqueue did not notify the runner")
	}
	counts, err := queue.StatusCounts()
	if err != nil {
		t.Fatal(err)
	}
	if counts.Total != 1 || counts.Running {
		t.Fatalf("counts = %#v", counts)
	}
	claimed := claimTask(t, queue)
	if claimed.ID != task.ID || claimed.Status != StatusRunning {
		t.Fatal("persistent task was not claimed")
	}
	queue.SetTaskProgress(task.ID, sites.Progress{Phase: "episode", CurrentStep: 1, TotalSteps: 2, Message: "half"})
	queue.SetTaskMessage(task.ID, "saved")
	queue.AddTaskWarning(task.ID, "warning")
	queue.AddTaskWarning(task.ID, "warning")
	queue.AddTaskWarning(task.ID, " ")
	queue.SetTaskTarget(task.ID, "作品")
	queue.SetTaskWorkID(task.ID, 100)
	queue.SetTaskSavedEpisodeCount(task.ID, 1)
	queue.SetTaskFailureEpisode(task.ID, "2", "2")
	if _, err := queue.RequestCancel(task.ID); err != nil {
		t.Fatal(err)
	}
	if err := queue.FinishTask(claimed, context.Canceled); err != nil {
		t.Fatal(err)
	}
	summary := queueSummary(t, queue)
	if summary.Current != nil || len(summary.RecentFailed) != 1 || summary.RecentFailed[0]["status"] != StatusCanceled {
		t.Fatalf("summary = %#v", summary)
	}

	queued := NewTask("download")
	queued.Target = "https://example.com/queued"
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
	claimed = claimTask(t, queue)
	action, err := queue.RequestedAction(taskstate.TaskRef{TaskID: claimed.ID, Attempt: claimed.AttemptCount})
	if err != nil {
		t.Fatal(err)
	}
	if action != taskstate.RequestedActionNone {
		t.Fatalf("requested action = %q", action)
	}
	if err := queue.FinishTask(claimed, taskstate.ErrTaskPauseRequested); err != nil {
		t.Fatal(err)
	}
	if summary := queueSummary(t, queue); len(summary.Paused) != 1 || summary.Paused[0]["status"] != StatusPaused {
		t.Fatalf("paused summary = %#v", summary)
	}
	if result, err := queue.RequestResume(queued.ID); err != nil || !result.Changed {
		t.Fatalf("resume paused task = %#v, err = %v", result, err)
	}
	claimed = claimTask(t, queue)
	if err := queue.FinishTask(claimed, taskstate.ErrRunnerShutdown); err != nil {
		t.Fatal(err)
	}
	if summary := queueSummary(t, queue); len(summary.Interrupted) != 1 || summary.Interrupted[0]["status"] != StatusInterrupted {
		t.Fatalf("interrupted summary = %#v", summary)
	}

	failed := NewTask("download")
	failed.Target = "https://example.invalid/failed"
	if err := queue.Enqueue(failed); err != nil {
		t.Fatal(err)
	}
	claimed = claimTask(t, queue)
	if err := queue.FinishTask(claimed, errors.New("synthetic failure")); err != nil {
		t.Fatal(err)
	}
	if summary := queueSummary(t, queue); summary.FailedCount != 2 || summary.RecentFailed[0]["status"] != StatusFailed {
		t.Fatalf("failed summary = %#v", summary)
	}

	succeeded := NewTask("download")
	succeeded.Target = "https://example.invalid/succeeded"
	if err := queue.Enqueue(succeeded); err != nil {
		t.Fatal(err)
	}
	claimed = claimTask(t, queue)
	if err := queue.FinishTask(claimed, nil); err != nil {
		t.Fatal(err)
	}
	if summary := queueSummary(t, queue); summary.CompletedCount != 1 || summary.RecentCompleted[0]["status"] != StatusSucceeded {
		t.Fatalf("succeeded summary = %#v", summary)
	}
}

func TestQueueReadsDoNotReportClosedStoreAsEmpty(t *testing.T) {
	store, err := storage.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	queue := NewQueue(taskstate.NewSQLiteRepository(store.DB()))
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := queue.StatusCounts(); err == nil {
		t.Fatal("closed store was reported as empty status counts")
	}
	if err := queue.Enqueue(NewTask("download")); err == nil {
		t.Fatal("closed store accepted a new task")
	}
	if _, err := queue.Summary(); err == nil {
		t.Fatal("closed store was reported as an empty summary")
	}
	if _, err := queue.ClaimNext(); err == nil {
		t.Fatal("closed store was reported as no next task")
	}
	if _, err := queue.HasQueuedTasks(); err == nil {
		t.Fatal("closed store was reported as no queued tasks")
	}
	if err := queue.SetTaskWorkID("missing", 1); err == nil {
		t.Fatal("closed store accepted a task identity update")
	}
}

func TestQueueRejectsIdentityUpdateForNonRunningTask(t *testing.T) {
	store, err := storage.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	queue := NewQueue(taskstate.NewSQLiteRepository(store.DB()))
	task := NewTask("download")
	task.Target = "https://example.invalid/queued"
	if err := queue.Enqueue(task); err != nil {
		t.Fatal(err)
	}
	if err := queue.SetTaskWorkID(task.ID, 1); !errors.Is(err, taskstate.ErrStaleTaskAttempt) {
		t.Fatalf("queued identity update error = %v", err)
	}
	if err := queue.SetTaskWorkID("missing", 1); !errors.Is(err, taskstate.ErrTaskNotFound) {
		t.Fatalf("missing identity update error = %v", err)
	}
	if err := queue.SetTaskWorkID(task.ID, 0); err == nil {
		t.Fatal("zero work id was accepted")
	}
}

func claimTask(t *testing.T, queue *Queue) *Task {
	t.Helper()
	task, err := queue.ClaimNext()
	if err != nil {
		t.Fatal(err)
	}
	if task == nil {
		t.Fatal("task was not claimed")
	}
	return task
}

func queueSummary(t *testing.T, queue *Queue) Summary {
	t.Helper()
	summary, err := queue.Summary()
	if err != nil {
		t.Fatal(err)
	}
	return summary
}
