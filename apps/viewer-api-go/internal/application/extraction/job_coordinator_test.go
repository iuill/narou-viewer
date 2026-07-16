package extraction

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	extractdomain "narou-viewer/apps/viewer-api-go/internal/extraction"
)

func TestJobCoordinatorProcessJobsConsumesQueuedAndRunningJobs(t *testing.T) {
	stateDir := t.TempDir()
	jobs := []extractdomain.Job{
		{JobID: "job-queued", RequestedUpToEpisodeIndex: "1", GenerationMode: "heuristic", Status: "queued", CreatedAt: "2026-01-01T00:00:00Z"},
		{JobID: "job-running", RequestedUpToEpisodeIndex: "2", GenerationMode: "heuristic", Status: "running", CreatedAt: "2026-01-01T00:00:01Z"},
	}
	for _, job := range jobs {
		if err := extractdomain.SaveJob(stateDir, "novel-1", job); err != nil {
			t.Fatalf("SaveJob(%s) returned error: %v", job.JobID, err)
		}
	}

	processed := []string{}
	coordinator := NewJobCoordinator(stateDir, func(ctx context.Context, novelID string, job extractdomain.Job) bool {
		processed = append(processed, novelID+":"+job.JobID)
		job.Status = "completed"
		if err := extractdomain.SaveJob(stateDir, novelID, job); err != nil {
			t.Fatalf("SaveJob completed returned error: %v", err)
		}
		return true
	})
	coordinator.Recover()
	coordinator.processJobs(context.Background())

	want := []string{"novel-1:job-queued", "novel-1:job-running"}
	if !reflect.DeepEqual(processed, want) {
		t.Fatalf("unexpected processed jobs: got %+v want %+v", processed, want)
	}
}

func TestJobCoordinatorNoopsWithoutProcessor(t *testing.T) {
	NewJobCoordinator(t.TempDir(), nil).Kick(nil)
	(*JobCoordinator)(nil).Recover()
	(*JobCoordinator)(nil).Kick(context.Background())
}

func TestJobCoordinatorDoesNotProcessCurrentJobWhenSameNovelHasFutureJob(t *testing.T) {
	stateDir := t.TempDir()
	if err := extractdomain.SaveJob(stateDir, "novel-mixed", extractdomain.Job{JobID: "current-job", Status: "queued"}); err != nil {
		t.Fatalf("SaveJob current: %v", err)
	}
	future := []byte("schema_version: 99\njob_id: future-job\nnovel_id: novel-mixed\nstatus: running\n")
	if err := os.WriteFile(filepath.Join(stateDir, "extraction_jobs", "future-job.yaml"), future, 0o600); err != nil {
		t.Fatalf("write future job: %v", err)
	}
	called := false
	coordinator := NewJobCoordinator(stateDir, func(context.Context, string, extractdomain.Job) bool {
		called = true
		return true
	})
	coordinator.Recover()
	coordinator.processJobs(context.Background())
	if called {
		t.Fatal("processor must not be called for a novel with an incompatible canonical job")
	}
}

func TestJobCoordinatorKickProcessesWithBackgroundContext(t *testing.T) {
	stateDir := t.TempDir()
	job := extractdomain.Job{JobID: "job-1", RequestedUpToEpisodeIndex: "1", GenerationMode: "heuristic", Status: "queued", CreatedAt: "2026-01-01T00:00:00Z"}
	if err := extractdomain.SaveJob(stateDir, "novel-1", job); err != nil {
		t.Fatalf("SaveJob returned error: %v", err)
	}

	processed := make(chan string, 1)
	coordinator := NewJobCoordinator(stateDir, func(_ context.Context, novelID string, job extractdomain.Job) bool {
		processed <- novelID + ":" + job.JobID
		return false
	})
	coordinator.Kick(nil)

	select {
	case got := <-processed:
		if got != "novel-1:job-1" {
			t.Fatalf("unexpected processed job: %s", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Kick did not process queued job")
	}
}

func TestJobCoordinatorProcessJobsStopsWhenContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	NewJobCoordinator(t.TempDir(), func(context.Context, string, extractdomain.Job) bool {
		called = true
		return false
	}).processJobs(ctx)
	if called {
		t.Fatal("processor should not be called for a canceled context")
	}
}
