package taskstate

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"narou-viewer/services/novel-fetcher/internal/storage/migration"

	_ "modernc.org/sqlite"
)

type scanError struct{ err error }

func (s scanError) Scan(...any) error { return s.err }

type resultWithRows int64

func (r resultWithRows) LastInsertId() (int64, error) { return 0, nil }
func (r resultWithRows) RowsAffected() (int64, error) { return int64(r), nil }

type resultError struct{}

func (resultError) LastInsertId() (int64, error) { return 0, nil }
func (resultError) RowsAffected() (int64, error) { return 0, errors.New("rows unavailable") }

func newRepository(t *testing.T) (*sql.DB, *SQLiteRepository) {
	t.Helper()
	store, err := openTestDB(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store, NewSQLiteRepository(store)
}

func openTestDB(root string) (*sql.DB, error) {
	databasePath := filepath.Join(root, "library.sqlite")
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", databasePath)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := migration.Run(db, databasePath); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func TestSQLiteRepositoryPersistsQueueOrderAcrossStoreReopen(t *testing.T) {
	root := t.TempDir()
	store, err := openTestDB(root)
	if err != nil {
		t.Fatal(err)
	}
	first := NewTask("download")
	first.Targets = []string{"https://example.com/first"}
	second := NewTask("download")
	second.Targets = []string{"https://example.com/second"}
	repository := NewSQLiteRepository(store)
	if _, err := repository.Enqueue(context.Background(), []*Task{first, second}); err != nil {
		t.Fatal(err)
	}
	_ = store.Close()

	store, err = openTestDB(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	repository = NewSQLiteRepository(store)
	summary, err := repository.Summary(context.Background(), 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(summary.Queued) != 2 || summary.Queued[0].ID != first.ID || summary.Queued[1].ID != second.ID {
		t.Fatalf("queued = %#v", summary.Queued)
	}
	if summary.Queued[0].QueuePosition == nil || *summary.Queued[0].QueuePosition != 1 {
		t.Fatalf("queue position = %#v", summary.Queued[0].QueuePosition)
	}
	claimed, err := repository.ClaimNext(context.Background(), time.Now())
	if err != nil || claimed == nil || claimed.ID != first.ID || claimed.AttemptCount != 1 {
		t.Fatalf("claimed = %#v, err = %v", claimed, err)
	}
}

func TestSQLiteRepositoryDeduplicatesExactRequestAndRejectsDifferentOptions(t *testing.T) {
	_, repository := newRepository(t)
	first := NewTask("update")
	first.NovelIDs = []int{42}
	first.SkipUnchanged = true
	second := NewTask("update")
	second.NovelIDs = []int{42}
	second.SkipUnchanged = true
	result, err := repository.Enqueue(context.Background(), []*Task{first})
	if err != nil || len(result.Tasks) != 1 || result.Tasks[0].ID != first.ID {
		t.Fatalf("first enqueue = %#v, err = %v", result, err)
	}
	result, err = repository.Enqueue(context.Background(), []*Task{second})
	if err != nil || len(result.DeduplicatedIDs) != 1 || second.ID != first.ID {
		t.Fatalf("duplicate enqueue = %#v, err = %v", result, err)
	}
	conflict := NewTask("update")
	conflict.NovelIDs = []int{42}
	conflict.ForceRedownload = true
	if _, err := repository.Enqueue(context.Background(), []*Task{conflict}); !errors.Is(err, ErrTaskAlreadyActive) {
		t.Fatalf("conflicting enqueue error = %v", err)
	}
}

func TestSQLiteRepositoryRecoveryDoesNotAutoResumeRunningTask(t *testing.T) {
	_, repository := newRepository(t)
	task := NewTask("download")
	task.Targets = []string{"https://example.com/work"}
	if _, err := repository.Enqueue(context.Background(), []*Task{task}); err != nil {
		t.Fatal(err)
	}
	claimed, err := repository.ClaimNext(context.Background(), time.Now())
	if err != nil || claimed == nil {
		t.Fatalf("claim = %#v, err = %v", claimed, err)
	}
	if err := repository.RecoverOnStartup(context.Background(), time.Now()); err != nil {
		t.Fatal(err)
	}
	recovered, found, err := repository.Get(context.Background(), task.ID)
	if err != nil || !found {
		t.Fatalf("recovered = %#v, found = %v, err = %v", recovered, found, err)
	}
	if recovered.Status != StatusInterrupted {
		t.Fatalf("status = %s", recovered.Status)
	}
	queued, err := repository.HasQueuedTasks(context.Background())
	if err != nil || queued {
		t.Fatalf("queued = %v, err = %v", queued, err)
	}
	if _, err := repository.ClaimNext(context.Background(), time.Now()); err != nil {
		t.Fatal(err)
	}
}

func TestSQLiteRepositoryControlTransitionsArePersistent(t *testing.T) {
	_, repository := newRepository(t)
	task := NewTask("download")
	task.Targets = []string{"https://example.com/work"}
	if _, err := repository.Enqueue(context.Background(), []*Task{task}); err != nil {
		t.Fatal(err)
	}
	paused, err := repository.RequestPause(context.Background(), task.ID)
	if err != nil || paused.Task.Status != StatusPaused || !paused.Changed {
		t.Fatalf("pause = %#v, err = %v", paused, err)
	}
	resumed, err := repository.RequestResume(context.Background(), task.ID)
	if err != nil || resumed.Task.Status != StatusQueued || !resumed.Changed {
		t.Fatalf("resume = %#v, err = %v", resumed, err)
	}
	claimed, err := repository.ClaimNext(context.Background(), time.Now())
	if err != nil || claimed == nil {
		t.Fatalf("claim = %#v, err = %v", claimed, err)
	}
	canceled, err := repository.RequestCancel(context.Background(), task.ID)
	if err != nil || canceled.Task.RequestedAction != RequestedActionCancel || !canceled.Changed {
		t.Fatalf("cancel = %#v, err = %v", canceled, err)
	}
	if action, err := repository.ReadRequestedAction(context.Background(), TaskRef{TaskID: task.ID, Attempt: claimed.AttemptCount}); err != nil || action != RequestedActionCancel {
		t.Fatalf("action = %s, err = %v", action, err)
	}
}

func TestSQLiteRepositoryPersistsProgressAndFinalOutcome(t *testing.T) {
	store, repository := newRepository(t)
	task := NewTask("update")
	task.NovelIDs = []int{7}
	if _, err := repository.Enqueue(context.Background(), []*Task{task}); err != nil {
		t.Fatal(err)
	}
	counts, err := repository.QueueCounts(context.Background())
	if err != nil || counts.Total != 1 || counts.Running {
		t.Fatalf("counts = %#v, err = %v", counts, err)
	}
	claimed, err := repository.ClaimNext(context.Background(), time.Now())
	if err != nil || claimed == nil {
		t.Fatalf("claim = %#v, err = %v", claimed, err)
	}
	ref := TaskRef{TaskID: task.ID, Attempt: claimed.AttemptCount}
	if action, err := repository.ReadRequestedAction(context.Background(), ref); err != nil || action != RequestedActionNone {
		t.Fatalf("action = %s, err = %v", action, err)
	}
	if err := repository.UpdateProgress(context.Background(), ref, Progress{Phase: "episode", CurrentStep: 1, TotalSteps: 2, Message: "half"}); err != nil {
		t.Fatal(err)
	}
	if err := repository.UpdateMessage(context.Background(), ref, "saved"); err != nil {
		t.Fatal(err)
	}
	if err := repository.AddNovelID(context.Background(), ref, 0); err != nil {
		t.Fatal(err)
	}
	if err := repository.AddWarning(context.Background(), ref, "warning"); err != nil {
		t.Fatal(err)
	}
	if err := repository.AddWarning(context.Background(), ref, "warning"); err != nil {
		t.Fatal(err)
	}
	if err := repository.SetTarget(context.Background(), ref, "作品"); err != nil {
		t.Fatal(err)
	}
	if err := repository.AddNovelID(context.Background(), ref, 8); err != nil {
		t.Fatal(err)
	}
	if err := repository.AddNovelID(context.Background(), ref, 8); err != nil {
		t.Fatal(err)
	}
	if err := repository.SetSavedEpisodeCount(context.Background(), ref, 1); err != nil {
		t.Fatal(err)
	}
	if err := repository.SetFailureEpisode(context.Background(), ref, "2", "2"); err != nil {
		t.Fatal(err)
	}
	updated, found, err := repository.Get(context.Background(), task.ID)
	if err != nil || !found {
		t.Fatalf("updated = %#v, found = %v, err = %v", updated, found, err)
	}
	if updated.Message != "saved" || updated.Phase != "episode" || updated.CurrentStep != 1 || updated.TotalSteps != 2 || updated.SavedEpisodeCount != 1 || len(updated.Warnings) != 1 || updated.TargetLabel != "作品" || updated.FailedEpisodeID != "2" || len(updated.NovelIDs) != 2 {
		t.Fatalf("updated = %#v", updated)
	}
	if err := repository.Finalize(context.Background(), ref, Outcome{Status: StatusFailed, Error: errors.New("boom")}); err != nil {
		t.Fatal(err)
	}
	finished, found, err := repository.Get(context.Background(), task.ID)
	if err != nil || !found || finished.Status != StatusFailed || finished.ErrorMessage != "boom" {
		t.Fatalf("finished = %#v, found = %v, err = %v", finished, found, err)
	}
	if err := repository.UpdateMessage(context.Background(), ref, "stale"); !errors.Is(err, ErrStaleTaskAttempt) {
		t.Fatalf("stale update error = %v", err)
	}
	_ = store
}

func TestSQLiteRepositoryResumeFailedTaskAndCancelTerminalCandidates(t *testing.T) {
	_, repository := newRepository(t)
	task := NewTask("download")
	task.Targets = []string{"https://example.com/work"}
	if _, err := repository.Enqueue(context.Background(), []*Task{task}); err != nil {
		t.Fatal(err)
	}
	claimed, err := repository.ClaimNext(context.Background(), time.Now())
	if err != nil || claimed == nil {
		t.Fatal(err)
	}
	ref := TaskRef{TaskID: task.ID, Attempt: claimed.AttemptCount}
	if err := repository.Finalize(context.Background(), ref, Outcome{Status: StatusFailed, Error: errors.New("temporary")}); err != nil {
		t.Fatal(err)
	}
	resumed, err := repository.RequestResume(context.Background(), task.ID)
	if err != nil || !resumed.Changed || resumed.Task.Status != StatusQueued {
		t.Fatalf("resume = %#v, err = %v", resumed, err)
	}
	claimed, err = repository.ClaimNext(context.Background(), time.Now())
	if err != nil || claimed == nil || claimed.AttemptCount != 2 {
		t.Fatalf("second claim = %#v, err = %v", claimed, err)
	}
	canceled, err := repository.RequestCancel(context.Background(), task.ID)
	if err != nil || !canceled.Changed {
		t.Fatalf("running cancel = %#v, err = %v", canceled, err)
	}
	if err := repository.Finalize(context.Background(), TaskRef{TaskID: task.ID, Attempt: 2}, Outcome{Status: StatusCanceled}); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.RequestResume(context.Background(), task.ID); !errors.Is(err, ErrTaskStateConflict) {
		t.Fatalf("terminal resume error = %v", err)
	}
}

func TestSQLiteRepositoryRejectsCorruptQueueInvariantAndMalformedRequest(t *testing.T) {
	store, repository := newRepository(t)
	if _, err := repository.Enqueue(context.Background(), []*Task{func() *Task {
		task := NewTask("download")
		task.Targets = []string{"https://example.com/work"}
		return task
	}()}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Exec(`DELETE FROM fetch_task_queue`); err != nil {
		t.Fatal(err)
	}
	if err := repository.RecoverOnStartup(context.Background(), time.Now()); !errors.Is(err, ErrTaskStateConflict) {
		t.Fatalf("queue invariant error = %v", err)
	}
	if _, err := store.Exec(`UPDATE fetch_tasks SET request_json = ?`, "{}"); err != nil {
		t.Fatal(err)
	}
	if err := repository.RecoverOnStartup(context.Background(), time.Now()); err == nil {
		t.Fatal("malformed request was accepted")
	}
}

func TestRequestHelpersNormalizeAndValidate(t *testing.T) {
	task := NewTask("download")
	task.Targets = []string{"HTTPS://Example.com/work/?page=1#fragment"}
	_, key, fingerprint, err := RequestForTask(task)
	if err != nil || key != "https://example.com/work" || fingerprint == "" {
		t.Fatalf("request = key %q, fingerprint %q, err %v", key, fingerprint, err)
	}
	if _, err := DecodeRequest("{}"); err == nil {
		t.Fatal("invalid request was accepted")
	}
	if got := IntIDsToStrings([]int{1, 2}); len(got) != 2 || got[1] != "2" {
		t.Fatalf("ids = %#v", got)
	}
	if got := TaskIDs([]*Task{task}); len(got) != 1 || got[0] != task.ID {
		t.Fatalf("task ids = %#v", got)
	}
}

func TestSQLiteRepositoryControlIdempotencyAndRecoveryOutcomes(t *testing.T) {
	for _, test := range []struct {
		name       string
		request    RequestedAction
		committed  bool
		wantStatus Status
	}{
		{name: "committed", committed: true, wantStatus: StatusSucceeded},
		{name: "cancel", request: RequestedActionCancel, wantStatus: StatusCanceled},
		{name: "pause", request: RequestedActionPause, wantStatus: StatusInterrupted},
	} {
		t.Run(test.name, func(t *testing.T) {
			store, repository := newRepository(t)
			task := NewTask("download")
			task.Targets = []string{"https://example.com/" + test.name}
			if _, err := repository.Enqueue(context.Background(), []*Task{task}); err != nil {
				t.Fatal(err)
			}
			claimed, err := repository.ClaimNext(context.Background(), time.Now())
			if err != nil || claimed == nil {
				t.Fatalf("claim = %#v, err = %v", claimed, err)
			}
			if test.committed {
				if _, err := store.Exec(`UPDATE fetch_tasks SET execution_committed = 1 WHERE task_id = ?`, task.ID); err != nil {
					t.Fatal(err)
				}
			} else {
				var result ControlResult
				if test.request == RequestedActionCancel {
					result, err = repository.RequestCancel(context.Background(), task.ID)
				} else {
					result, err = repository.RequestPause(context.Background(), task.ID)
				}
				if err != nil || !result.Changed {
					t.Fatalf("control = %#v, err = %v", result, err)
				}
				if test.request == RequestedActionPause {
					second, secondErr := repository.RequestPause(context.Background(), task.ID)
					if secondErr != nil || second.Changed {
						t.Fatalf("idempotent pause = %#v, err = %v", second, secondErr)
					}
				}
			}
			if err := repository.RecoverOnStartup(context.Background(), time.Now()); err != nil {
				t.Fatal(err)
			}
			recovered, found, err := repository.Get(context.Background(), task.ID)
			if err != nil || !found || recovered.Status != test.wantStatus {
				t.Fatalf("recovered = %#v, found = %v, err = %v", recovered, found, err)
			}
		})
	}
}

func TestSQLiteRepositoryRejectsStaleAttemptForAllCriticalWrites(t *testing.T) {
	_, repository := newRepository(t)
	task := NewTask("download")
	task.Targets = []string{"https://example.com/stale"}
	if _, err := repository.Enqueue(context.Background(), []*Task{task}); err != nil {
		t.Fatal(err)
	}
	claimed, err := repository.ClaimNext(context.Background(), time.Now())
	if err != nil || claimed == nil {
		t.Fatal(err)
	}
	stale := TaskRef{TaskID: task.ID, Attempt: claimed.AttemptCount + 1}
	checks := []struct {
		name string
		run  func() error
	}{
		{"read", func() error { _, err := repository.ReadRequestedAction(context.Background(), stale); return err }},
		{"progress", func() error { return repository.UpdateProgress(context.Background(), stale, Progress{}) }},
		{"message", func() error { return repository.UpdateMessage(context.Background(), stale, "x") }},
		{"warning", func() error { return repository.AddWarning(context.Background(), stale, "x") }},
		{"target", func() error { return repository.SetTarget(context.Background(), stale, "x") }},
		{"novel", func() error { return repository.AddNovelID(context.Background(), stale, 8) }},
		{"count", func() error { return repository.SetSavedEpisodeCount(context.Background(), stale, 1) }},
		{"failure", func() error { return repository.SetFailureEpisode(context.Background(), stale, "1", "1") }},
		{"finalize", func() error { return repository.Finalize(context.Background(), stale, Outcome{Status: StatusFailed}) }},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			err := check.run()
			if !errors.Is(err, ErrStaleTaskAttempt) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestSQLiteRepositoryCoversStateMatrixAndTerminalFields(t *testing.T) {
	store, repository := newRepository(t)
	missing := NewTask("download")
	if _, err := repository.Enqueue(context.Background(), []*Task{missing}); err == nil {
		t.Fatal("missing target was accepted")
	}

	queued := NewTask("download")
	queued.Targets = []string{"https://example.com/matrix"}
	if _, err := repository.Enqueue(context.Background(), []*Task{queued}); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.RequestResume(context.Background(), queued.ID); !errors.Is(err, ErrTaskStateConflict) {
		t.Fatalf("queued resume error = %v", err)
	}
	if _, err := repository.RequestPause(context.Background(), queued.ID); err != nil {
		t.Fatal(err)
	}
	if result, err := repository.RequestPause(context.Background(), queued.ID); err != nil || result.Changed {
		t.Fatalf("paused pause = %#v, err = %v", result, err)
	}
	if result, err := repository.RequestCancel(context.Background(), queued.ID); err != nil || !result.Changed || result.Task.Status != StatusCanceled {
		t.Fatalf("paused cancel = %#v, err = %v", result, err)
	}
	if _, err := repository.RequestCancel(context.Background(), queued.ID); !errors.Is(err, ErrTaskStateConflict) {
		t.Fatalf("canceled cancel error = %v", err)
	}

	second := NewTask("download")
	second.Targets = []string{"https://example.com/matrix-2"}
	if _, err := repository.Enqueue(context.Background(), []*Task{second}); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ClaimNext(context.Background(), time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.RequestResume(context.Background(), second.ID); !errors.Is(err, ErrTaskStateConflict) {
		t.Fatalf("running resume error = %v", err)
	}
	if _, err := repository.RequestPause(context.Background(), second.ID); err != nil {
		t.Fatal(err)
	}
	if result, err := repository.RequestPause(context.Background(), second.ID); err != nil || result.Changed {
		t.Fatalf("running pause no-op = %#v, err = %v", result, err)
	}

	third := NewTask("download")
	third.Targets = []string{"https://example.com/matrix-3"}
	if _, err := repository.Enqueue(context.Background(), []*Task{third}); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ClaimNext(context.Background(), time.Now()); err == nil {
		t.Fatal("second running claim unexpectedly succeeded")
	}
	if err := repository.Finalize(context.Background(), TaskRef{TaskID: second.ID, Attempt: 1}, Outcome{Status: StatusSucceeded, ExecutionCommitted: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ClaimNext(context.Background(), time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := repository.Finalize(context.Background(), TaskRef{TaskID: third.ID, Attempt: 1}, Outcome{Status: StatusPaused}); err != nil {
		t.Fatal(err)
	}
	paused, _, err := repository.Get(context.Background(), third.ID)
	if err != nil || paused.PausedAt == nil {
		t.Fatalf("paused = %#v, err = %v", paused, err)
	}
	if result, err := repository.RequestPause(context.Background(), third.ID); err != nil || result.Changed {
		t.Fatalf("paused pause = %#v, err = %v", result, err)
	}

	if _, err := repository.ClaimNext(context.Background(), time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ClaimNext(context.Background(), time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := repository.Finalize(context.Background(), TaskRef{TaskID: third.ID, Attempt: 1}, Outcome{Status: StatusSucceeded}); err == nil {
		t.Fatal("finalize with stale attempt unexpectedly succeeded")
	}
	if err := repository.Finalize(context.Background(), TaskRef{TaskID: third.ID, Attempt: 2}, Outcome{Status: StatusFailed}); err == nil {
		t.Fatal("finalize invalid running state unexpectedly succeeded")
	}

	if _, err := store.Exec(`INSERT INTO fetch_task_queue(task_id, enqueued_at) VALUES (?, ?)`, third.ID, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if err := repository.RecoverOnStartup(context.Background(), time.Now()); !errors.Is(err, ErrTaskStateConflict) {
		t.Fatalf("nonqueued queue row error = %v", err)
	}
}

func TestSQLiteRepositorySummaryIncludesPersistentHistoryBuckets(t *testing.T) {
	store, repository := newRepository(t)
	makeTask := func(name string) *Task {
		task := NewTask("download")
		task.Targets = []string{"https://example.com/summary/" + name}
		if _, err := repository.Enqueue(context.Background(), []*Task{task}); err != nil {
			t.Fatal(err)
		}
		return task
	}
	paused := makeTask("paused")
	if _, err := repository.RequestPause(context.Background(), paused.ID); err != nil {
		t.Fatal(err)
	}
	interrupted := makeTask("interrupted")
	claimed, err := repository.ClaimNext(context.Background(), time.Now())
	if err != nil || claimed == nil {
		t.Fatal(err)
	}
	if err := repository.RecoverOnStartup(context.Background(), time.Now()); err != nil {
		t.Fatal(err)
	}
	if recovered, _, err := repository.Get(context.Background(), interrupted.ID); err != nil || recovered.Status != StatusInterrupted {
		t.Fatalf("interrupted = %#v, err = %v", recovered, err)
	}
	succeeded := makeTask("succeeded")
	claimed, err = repository.ClaimNext(context.Background(), time.Now())
	if err != nil || claimed == nil {
		t.Fatal(err)
	}
	if err := repository.Finalize(context.Background(), TaskRef{TaskID: succeeded.ID, Attempt: 1}, Outcome{Status: StatusSucceeded, ExecutionCommitted: true}); err != nil {
		t.Fatal(err)
	}
	failed := makeTask("failed")
	claimed, err = repository.ClaimNext(context.Background(), time.Now())
	if err != nil || claimed == nil {
		t.Fatal(err)
	}
	if err := repository.Finalize(context.Background(), TaskRef{TaskID: failed.ID, Attempt: 1}, Outcome{Status: StatusFailed, Error: errors.New("failed")}); err != nil {
		t.Fatal(err)
	}
	canceled := makeTask("canceled")
	claimed, err = repository.ClaimNext(context.Background(), time.Now())
	if err != nil || claimed == nil {
		t.Fatal(err)
	}
	if _, err := repository.RequestCancel(context.Background(), canceled.ID); err != nil {
		t.Fatal(err)
	}
	if err := repository.Finalize(context.Background(), TaskRef{TaskID: canceled.ID, Attempt: 1}, Outcome{Status: StatusCanceled}); err != nil {
		t.Fatal(err)
	}
	queued := makeTask("queued")
	counts, err := repository.QueueCounts(context.Background())
	if err != nil || counts.Total != 1 || counts.Running {
		t.Fatalf("counts = %#v, err = %v", counts, err)
	}
	summary, err := repository.Summary(context.Background(), 20)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Current != nil || len(summary.Queued) != 1 || len(summary.Paused) != 1 || len(summary.Interrupted) != 1 || len(summary.RecentCompleted) != 1 || len(summary.RecentFailed) != 2 {
		t.Fatalf("summary = %#v", summary)
	}
	if summary.Queued[0].ID != queued.ID {
		t.Fatalf("queued summary = %#v", summary.Queued)
	}
	if summary.CompletedCount != 1 || summary.FailedCount != 2 || summary.CanceledCount != 1 || summary.PausedCount != 1 || summary.InterruptedCount != 1 {
		t.Fatalf("summary counts = %#v", summary)
	}
	if _, found, err := repository.Get(context.Background(), "missing"); err != nil || found {
		t.Fatalf("missing task = found %v, err %v", found, err)
	}
	if _, err := repository.ClaimNext(context.Background(), time.Now()); err != nil {
		t.Fatal(err)
	}
	_ = store
}

func TestSQLiteRepositoryRejectsMalformedStoredTaskAndInvalidClaimState(t *testing.T) {
	store, repository := newRepository(t)
	task := NewTask("download")
	task.Targets = []string{"https://example.com/corrupt"}
	if _, err := repository.Enqueue(context.Background(), []*Task{task}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Exec(`UPDATE fetch_tasks SET warnings_json = ? WHERE task_id = ?`, "{", task.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := repository.Get(context.Background(), task.ID); err == nil {
		t.Fatal("malformed warnings were accepted")
	}
	if _, err := store.Exec(`UPDATE fetch_tasks SET warnings_json = '[]', status = 'paused' WHERE task_id = ?`, task.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ClaimNext(context.Background(), time.Now()); !errors.Is(err, ErrTaskStateConflict) {
		t.Fatalf("invalid claim state error = %v", err)
	}
	if err := repository.RecoverOnStartup(context.Background(), time.Now()); !errors.Is(err, ErrTaskStateConflict) {
		t.Fatalf("invariant error = %v", err)
	}
}

func TestSQLiteRepositoryTerminalControlMatrixAndResumeConflict(t *testing.T) {
	store, repository := newRepository(t)
	newTask := func(name string) *Task {
		task := NewTask("download")
		task.Targets = []string{"https://example.com/control/" + name}
		if _, err := repository.Enqueue(context.Background(), []*Task{task}); err != nil {
			t.Fatal(err)
		}
		if _, err := repository.RequestPause(context.Background(), task.ID); err != nil {
			t.Fatal(err)
		}
		return task
	}
	failed := newTask("failed")
	if _, err := store.Exec(`UPDATE fetch_tasks SET status = 'failed' WHERE task_id = ?`, failed.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.RequestPause(context.Background(), failed.ID); !errors.Is(err, ErrTaskStateConflict) {
		t.Fatalf("failed pause error = %v", err)
	}
	if result, err := repository.RequestCancel(context.Background(), failed.ID); err != nil || !result.Changed || result.Task.Status != StatusCanceled {
		t.Fatalf("failed cancel = %#v, err = %v", result, err)
	}
	interrupted := newTask("interrupted")
	if _, err := store.Exec(`UPDATE fetch_tasks SET status = 'interrupted' WHERE task_id = ?`, interrupted.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.RequestPause(context.Background(), interrupted.ID); !errors.Is(err, ErrTaskStateConflict) {
		t.Fatalf("interrupted pause error = %v", err)
	}
	if result, err := repository.RequestCancel(context.Background(), interrupted.ID); err != nil || !result.Changed || result.Task.Status != StatusCanceled {
		t.Fatalf("interrupted cancel = %#v, err = %v", result, err)
	}

	succeeded := newTask("succeeded")
	if _, err := store.Exec(`UPDATE fetch_tasks SET status = 'succeeded' WHERE task_id = ?`, succeeded.ID); err != nil {
		t.Fatal(err)
	}
	canceled := newTask("canceled")
	if _, err := store.Exec(`UPDATE fetch_tasks SET status = 'canceled' WHERE task_id = ?`, canceled.ID); err != nil {
		t.Fatal(err)
	}
	for _, task := range []*Task{succeeded, canceled} {
		if _, err := repository.RequestPause(context.Background(), task.ID); !errors.Is(err, ErrTaskStateConflict) {
			t.Fatalf("terminal pause for %s = %v", task.ID, err)
		}
		if _, err := repository.RequestCancel(context.Background(), task.ID); !errors.Is(err, ErrTaskStateConflict) {
			t.Fatalf("terminal cancel for %s = %v", task.ID, err)
		}
		if _, err := repository.RequestResume(context.Background(), task.ID); !errors.Is(err, ErrTaskStateConflict) {
			t.Fatalf("terminal resume for %s = %v", task.ID, err)
		}
	}

	running := NewTask("download")
	running.Targets = []string{"https://example.com/control/running"}
	if _, err := repository.Enqueue(context.Background(), []*Task{running}); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ClaimNext(context.Background(), time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.RequestResume(context.Background(), running.ID); !errors.Is(err, ErrTaskStateConflict) {
		t.Fatalf("running resume error = %v", err)
	}
	if err := repository.Finalize(context.Background(), TaskRef{TaskID: running.ID, Attempt: 1}, Outcome{Status: StatusInterrupted}); err != nil {
		t.Fatal(err)
	}

	reserved := newTask("reserved")
	conflict := newTask("conflict")
	if _, err := store.Exec(`UPDATE fetch_tasks SET primary_work_id = 500 WHERE task_id = ?`, reserved.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Exec(`DELETE FROM fetch_task_queue WHERE task_id = ?`, conflict.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Exec(`UPDATE fetch_tasks SET status = 'failed', primary_work_id = 500 WHERE task_id = ?`, conflict.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.RequestResume(context.Background(), conflict.ID); !errors.Is(err, ErrTaskAlreadyActive) {
		t.Fatalf("resume conflict error = %v", err)
	}
}

func TestSQLiteRepositoryRejectsUnknownRequestVersionAndInvalidFinalization(t *testing.T) {
	store, repository := newRepository(t)
	if _, err := repository.ClaimNext(context.Background(), time.Now()); err != nil {
		t.Fatal(err)
	}
	task := NewTask("download")
	task.Targets = []string{"https://example.com/version"}
	if _, err := repository.Enqueue(context.Background(), []*Task{task}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Exec(`UPDATE fetch_tasks SET request_version = 99 WHERE task_id = ?`, task.ID); err != nil {
		t.Fatal(err)
	}
	if err := repository.RecoverOnStartup(context.Background(), time.Now()); err == nil {
		t.Fatal("unknown request version was accepted")
	}
	for _, status := range []Status{"", StatusQueued, StatusRunning} {
		if err := repository.Finalize(context.Background(), TaskRef{TaskID: task.ID, Attempt: 1}, Outcome{Status: status}); err == nil {
			t.Fatalf("invalid status %q was accepted", status)
		}
	}
	if _, err := DecodeRequest("{"); err == nil {
		t.Fatal("malformed JSON was accepted")
	}
}

func TestSQLiteRepositoryNoopAndMissingControlPaths(t *testing.T) {
	store, repository := newRepository(t)
	if _, err := repository.RequestPause(context.Background(), "missing"); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("missing pause = %v", err)
	}
	if _, err := repository.RequestResume(context.Background(), "missing"); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("missing resume = %v", err)
	}
	if _, err := repository.RequestCancel(context.Background(), "missing"); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("missing cancel = %v", err)
	}
	if _, err := repository.Summary(context.Background(), 0); err != nil {
		t.Fatal(err)
	}
	task := NewTask("download")
	task.Targets = []string{"https://example.com/noop"}
	if _, err := repository.Enqueue(context.Background(), []*Task{task}); err != nil {
		t.Fatal(err)
	}
	claimed, err := repository.ClaimNext(context.Background(), time.Now())
	if err != nil || claimed == nil {
		t.Fatal(err)
	}
	counts, err := repository.QueueCounts(context.Background())
	if err != nil || !counts.Running || counts.Total != 1 {
		t.Fatalf("running counts = %#v, err = %v", counts, err)
	}
	ref := TaskRef{TaskID: task.ID, Attempt: claimed.AttemptCount}
	if err := repository.AddWarning(context.Background(), ref, ""); err != nil {
		t.Fatal(err)
	}
	if err := repository.Finalize(context.Background(), ref, Outcome{Status: StatusSucceeded, Message: "done"}); err != nil {
		t.Fatal(err)
	}
	paused := NewTask("download")
	paused.Targets = []string{"https://example.com/resume-queue-error"}
	if _, err := repository.Enqueue(context.Background(), []*Task{paused}); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.RequestPause(context.Background(), paused.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Exec(`INSERT INTO fetch_task_queue(task_id, enqueued_at) VALUES (?, ?)`, paused.ID, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.RequestResume(context.Background(), paused.ID); err == nil {
		t.Fatal("duplicate resume queue row was accepted")
	}
}

func TestTaskStateSmallValidationHelpers(t *testing.T) {
	originalRandomReader := taskIDRandomReader
	defer func() { taskIDRandomReader = originalRandomReader }()
	taskIDRandomReader = func([]byte) (int, error) { return 0, errors.New("random unavailable") }
	if got := NewTaskID("fallback"); !strings.HasPrefix(got, "fallback-") {
		t.Fatalf("fallback task id = %q", got)
	}
	if task, found, err := scanTask(scanError{err: sql.ErrNoRows}); err != nil || found || task != nil {
		t.Fatalf("no rows scan = %#v, %v, %v", task, found, err)
	}
	if _, _, err := scanTask(scanError{err: errors.New("scan failed")}); err == nil {
		t.Fatal("scan error was ignored")
	}
	if err := requireAttemptUpdate(resultWithRows(0)); !errors.Is(err, ErrStaleTaskAttempt) {
		t.Fatalf("zero rows = %v", err)
	}
	if err := requireAttemptUpdate(resultWithRows(1)); err != nil {
		t.Fatal(err)
	}
	if err := requireAttemptUpdate(resultError{}); err == nil {
		t.Fatal("rows error was ignored")
	}
	if _, err := optionalTime("bad-time"); err == nil {
		t.Fatal("invalid optional time was accepted")
	}
	if got := canonicalizeTarget(" n1234ab/ "); got != "n1234ab" {
		t.Fatalf("canonical target = %q", got)
	}
}

func TestSQLiteRepositoryPropagatesClosedDatabaseErrors(t *testing.T) {
	store, repository := newRepository(t)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	task := NewTask("download")
	task.Targets = []string{"https://example.com/closed"}
	if _, err := repository.Enqueue(context.Background(), []*Task{task}); err == nil {
		t.Fatal("enqueue accepted closed database")
	}
	if _, err := repository.ClaimNext(context.Background(), time.Now()); err == nil {
		t.Fatal("claim accepted closed database")
	}
	if _, err := repository.Summary(context.Background(), 20); err == nil {
		t.Fatal("summary accepted closed database")
	}
	if _, err := repository.QueueCounts(context.Background()); err == nil {
		t.Fatal("counts accepted closed database")
	}
	if _, err := repository.HasQueuedTasks(context.Background()); err == nil {
		t.Fatal("has queued accepted closed database")
	}
	if _, _, err := repository.Get(context.Background(), task.ID); err == nil {
		t.Fatal("get accepted closed database")
	}
	if _, err := repository.RequestPause(context.Background(), task.ID); err == nil {
		t.Fatal("pause accepted closed database")
	}
	if _, err := repository.RequestResume(context.Background(), task.ID); err == nil {
		t.Fatal("resume accepted closed database")
	}
	if _, err := repository.RequestCancel(context.Background(), task.ID); err == nil {
		t.Fatal("cancel accepted closed database")
	}
	ref := TaskRef{TaskID: task.ID, Attempt: 1}
	if _, err := repository.ReadRequestedAction(context.Background(), ref); err == nil {
		t.Fatal("read action accepted closed database")
	}
	if err := repository.UpdateProgress(context.Background(), ref, Progress{Message: "x"}); err == nil {
		t.Fatal("progress accepted closed database")
	}
	if err := repository.UpdateMessage(context.Background(), ref, "x"); err == nil {
		t.Fatal("message accepted closed database")
	}
	if err := repository.AddWarning(context.Background(), ref, "x"); err == nil {
		t.Fatal("warning accepted closed database")
	}
	if err := repository.SetTarget(context.Background(), ref, "x"); err == nil {
		t.Fatal("target accepted closed database")
	}
	if err := repository.AddNovelID(context.Background(), ref, 1); err == nil {
		t.Fatal("novel id accepted closed database")
	}
	if err := repository.SetSavedEpisodeCount(context.Background(), ref, 1); err == nil {
		t.Fatal("count accepted closed database")
	}
	if err := repository.SetFailureEpisode(context.Background(), ref, "1", "1"); err == nil {
		t.Fatal("failure accepted closed database")
	}
	if err := repository.Finalize(context.Background(), ref, Outcome{Status: StatusSucceeded}); err == nil {
		t.Fatal("finalize accepted closed database")
	}
	if err := repository.RecoverOnStartup(context.Background(), time.Now()); err == nil {
		t.Fatal("recovery accepted closed database")
	}
}

func TestSQLiteRepositoryRejectsInvalidStoredTimestampsAndSummaryRows(t *testing.T) {
	for _, column := range []string{"created_at", "updated_at", "started_at", "paused_at", "interrupted_at", "finished_at"} {
		t.Run(column, func(t *testing.T) {
			store, repository := newRepository(t)
			task := NewTask("download")
			task.Targets = []string{"https://example.com/time/" + column}
			if _, err := repository.Enqueue(context.Background(), []*Task{task}); err != nil {
				t.Fatal(err)
			}
			if _, err := store.Exec(`UPDATE fetch_tasks SET `+column+` = ? WHERE task_id = ?`, "not-a-time", task.ID); err != nil {
				t.Fatal(err)
			}
			if _, _, err := repository.Get(context.Background(), task.ID); err == nil {
				t.Fatalf("invalid %s was accepted", column)
			}
		})
	}
	store, repository := newRepository(t)
	task := NewTask("download")
	task.Targets = []string{"https://example.com/summary-corrupt"}
	if _, err := repository.Enqueue(context.Background(), []*Task{task}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Exec(`UPDATE fetch_tasks SET request_json = ? WHERE task_id = ?`, "{}", task.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Summary(context.Background(), 20); err == nil {
		t.Fatal("summary accepted malformed queued task")
	}
}
