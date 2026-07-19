package extraction

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	extractdomain "narou-viewer/apps/viewer-api-go/internal/extraction"
)

func TestJobCoordinatorDoesNotProcessMultipleActiveJobsForOneNovel(t *testing.T) {
	stateDir := t.TempDir()
	jobDir := filepath.Join(stateDir, "extraction_jobs")
	if err := os.MkdirAll(jobDir, 0o700); err != nil {
		t.Fatalf("mkdir jobs: %v", err)
	}
	for name, status := range map[string]string{"job-queued": "queued", "job-running": "running"} {
		raw := "schema_version: 2\nrevision: 1\njob_id: " + name + "\nnovel_id: novel-1\nrequested_up_to_episode_index: \"1\"\nstatus: " + status + "\ncreated_at: 2026-01-01T00:00:00Z\n"
		if err := os.WriteFile(filepath.Join(jobDir, name+".yaml"), []byte(raw), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	called := false
	coordinator := NewJobCoordinator(stateDir, func(ctx context.Context, novelID string, job extractdomain.Job) bool {
		called = true
		return true
	})
	coordinator.Recover()
	coordinator.processJobs(context.Background())

	if called {
		t.Fatal("processor must not run when a novel has multiple active canonical jobs")
	}
}

func TestJobCoordinatorNoopsWithoutProcessor(t *testing.T) {
	NewJobCoordinator(t.TempDir(), nil).Kick(nil)
	(*JobCoordinator)(nil).Recover()
	(*JobCoordinator)(nil).Kick(context.Background())
}

func TestJobCoordinatorDoesNotProcessCurrentJobWhenSameNovelHasFutureJob(t *testing.T) {
	stateDir := t.TempDir()
	if err := extractdomain.SaveJob(stateDir, "novel-mixed", extractdomain.Job{JobID: "current-job", RequestedUpToEpisodeIndex: "1", Status: "queued"}); err != nil {
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

func TestJobCoordinatorCancelStopsRunningJobAndFinalizesPause(t *testing.T) {
	stateDir := t.TempDir()
	job := extractdomain.Job{JobID: "job-pause", RequestedUpToEpisodeIndex: "1", Status: extractdomain.JobStatusQueued, CreatedAt: "2026-01-01T00:00:00Z"}
	if err := extractdomain.SaveJob(stateDir, "novel-1", job); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	stopped := make(chan struct{})
	coordinator := NewJobCoordinator(stateDir, func(ctx context.Context, _ string, _ extractdomain.Job) bool {
		close(started)
		<-ctx.Done()
		close(stopped)
		return false
	})
	coordinator.Kick(context.Background())
	<-started
	if _, err := extractdomain.ControlJob(stateDir, "novel-1", job.JobID, "pause"); err != nil {
		t.Fatal(err)
	}
	coordinator.Cancel(job.JobID)
	<-stopped
	for index := 0; index < 100; index++ {
		jobs, _, err := extractdomain.LoadJobs(stateDir, "novel-1")
		if err != nil {
			t.Fatal(err)
		}
		if len(jobs) == 1 && jobs[0].Status == extractdomain.JobStatusPaused {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("job was not finalized as paused")
}
