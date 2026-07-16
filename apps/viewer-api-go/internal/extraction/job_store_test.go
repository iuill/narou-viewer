package extraction

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"narou-viewer/apps/viewer-api-go/internal/extraction/checkpointstore"
	"narou-viewer/apps/viewer-api-go/internal/state/schemaguard"
	"narou-viewer/apps/viewer-api-go/internal/state/schemaguardtest"
)

func TestLoadJobsReadsCharacterJobDocuments(t *testing.T) {
	stateDir := t.TempDir()
	jobDir := filepath.Join(stateDir, "extraction_jobs")
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		t.Fatalf("mkdir job dir: %v", err)
	}
	writeFile(t, filepath.Join(jobDir, "job-1.yaml"), `
schema_version: 2
revision: 1
job_id: job-1
novel_id: novel-1
requested_up_to_episode_index: "3"
profile_id: default
profile_label: Default
generation_mode: heuristic
model_id: null
status: completed
progress: 100
progress_stage: completed
current_batch_index: 2
batch_count: 2
completed_batch_count: 2
generated_character_count: 4
created_at: 2026-01-01T00:00:00Z
started_at: 2026-01-01T00:00:01Z
finished_at: 2026-01-01T00:00:02Z
error_message: null
`)
	writeFile(t, filepath.Join(jobDir, "other.yaml"), `
job_id: other
novel_id: other-novel
requested_up_to_episode_index: "1"
generation_mode: heuristic
status: queued
created_at: 2026-01-02T00:00:00Z
`)

	jobs, ok, err := LoadJobs(stateDir, "novel-1")
	if err != nil {
		t.Fatalf("LoadJobs returned error: %v", err)
	}
	if !ok || len(jobs) != 1 {
		t.Fatalf("unexpected jobs: ok=%v jobs=%+v", ok, jobs)
	}
	if jobs[0].JobID != "job-1" || jobs[0].ProfileID == nil || *jobs[0].ProfileID != "default" {
		t.Fatalf("unexpected job: %+v", jobs[0])
	}
	if jobs[0].Progress == nil || *jobs[0].Progress != 100 || jobs[0].ProgressStage == nil || *jobs[0].ProgressStage != "completed" ||
		jobs[0].CurrentBatchIndex == nil || *jobs[0].CurrentBatchIndex != 2 || jobs[0].BatchCount == nil || *jobs[0].BatchCount != 2 ||
		jobs[0].CompletedBatchCount == nil || *jobs[0].CompletedBatchCount != 2 ||
		jobs[0].GeneratedCharacterCount == nil || *jobs[0].GeneratedCharacterCount != 4 {
		t.Fatalf("job progress metadata should round-trip from yaml: %+v", jobs[0])
	}

	createdAt := "2026-01-03T00:00:00Z"
	progress := 45
	progressStage := "batch"
	currentBatchIndex := 1
	batchCount := 2
	completedBatchCount := 1
	generatedCharacterCount := 3
	activeWorkers := []ActiveWorker{{
		WorkerIndex:       1,
		BatchIndex:        2,
		StartEpisodeIndex: "3",
		EndEpisodeIndex:   "4",
		Phase:             "extraction",
	}}
	if err := SaveJob(stateDir, "novel-1", Job{
		JobID:                     "go-job-new",
		RequestedUpToEpisodeIndex: "4",
		GenerationMode:            "heuristic",
		Status:                    "queued",
		Progress:                  &progress,
		ProgressStage:             &progressStage,
		CurrentBatchIndex:         &currentBatchIndex,
		BatchCount:                &batchCount,
		CompletedBatchCount:       &completedBatchCount,
		GeneratedCharacterCount:   &generatedCharacterCount,
		ActiveWorkers:             activeWorkers,
		CreatedAt:                 createdAt,
	}); err != nil {
		t.Fatalf("SaveJob returned error: %v", err)
	}
	jobs, ok, err = LoadJobs(stateDir, "novel-1")
	if err != nil || !ok {
		t.Fatalf("LoadJobs after SaveJob failed: ok=%v err=%v", ok, err)
	}
	if len(jobs) != 2 || jobs[0].JobID != "go-job-new" || jobs[0].CreatedAt != createdAt {
		t.Fatalf("saved job should be loaded first: %+v", jobs)
	}
	if jobs[0].Progress == nil || *jobs[0].Progress != progress || jobs[0].ProgressStage == nil || *jobs[0].ProgressStage != progressStage ||
		jobs[0].CurrentBatchIndex == nil || *jobs[0].CurrentBatchIndex != currentBatchIndex || jobs[0].BatchCount == nil || *jobs[0].BatchCount != batchCount ||
		jobs[0].CompletedBatchCount == nil || *jobs[0].CompletedBatchCount != completedBatchCount ||
		jobs[0].GeneratedCharacterCount == nil || *jobs[0].GeneratedCharacterCount != generatedCharacterCount ||
		len(jobs[0].ActiveWorkers) != 1 || jobs[0].ActiveWorkers[0] != activeWorkers[0] {
		t.Fatalf("saved job progress metadata should be loaded: %+v", jobs[0])
	}
	allJobs, err := LoadAllJobs(stateDir)
	if err != nil {
		t.Fatalf("LoadAllJobs returned error: %v", err)
	}
	if len(allJobs) != 3 || allJobs[0].NovelID != "novel-1" || allJobs[0].Job.JobID != "go-job-new" {
		t.Fatalf("unexpected all jobs: %+v", allJobs)
	}
	indexRaw, err := os.ReadFile(filepath.Join(jobDir, "index", "novel-1.yaml"))
	if err != nil {
		t.Fatalf("read job index: %v", err)
	}
	indexText := string(indexRaw)
	if !strings.Contains(indexText, "active_job_id: go-job-new") || !strings.Contains(indexText, "- go-job-new") {
		t.Fatalf("saved job should update TS-compatible index: %s", indexText)
	}
}

func TestPruneNovelStateDeletesProfilesJobsIndexesAndCheckpoints(t *testing.T) {
	stateDir := t.TempDir()
	profileDir := filepath.Join(stateDir, "character_profiles")
	eventsDir := filepath.Join(stateDir, "character_events")
	termDir := filepath.Join(stateDir, "term_profiles")
	checkpointDir := filepath.Join(stateDir, "extraction_jobs", "checkpoints")
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		t.Fatalf("mkdir profile dir: %v", err)
	}
	if err := os.MkdirAll(eventsDir, 0o755); err != nil {
		t.Fatalf("mkdir events dir: %v", err)
	}
	if err := os.MkdirAll(termDir, 0o755); err != nil {
		t.Fatalf("mkdir term dir: %v", err)
	}
	if err := os.MkdirAll(checkpointDir, 0o755); err != nil {
		t.Fatalf("mkdir checkpoint dir: %v", err)
	}
	writeFile(t, filepath.Join(profileDir, "novel-1.yaml"), `novel_id: novel-1`)
	writeFile(t, filepath.Join(eventsDir, "novel-1.yaml"), `novel_id: novel-1`)
	writeFile(t, filepath.Join(termDir, "novel-1.yaml"), "schema_version: 1\nnovel_id: novel-1")
	if err := SaveJob(stateDir, "novel-1", Job{JobID: "job-target", RequestedUpToEpisodeIndex: "1", GenerationMode: "heuristic", Status: "completed", CreatedAt: "2026-01-01T00:00:00Z"}); err != nil {
		t.Fatalf("SaveJob target returned error: %v", err)
	}
	if err := SaveJob(stateDir, "novel-2", Job{JobID: "job-other", RequestedUpToEpisodeIndex: "1", GenerationMode: "heuristic", Status: "completed", CreatedAt: "2026-01-02T00:00:00Z"}); err != nil {
		t.Fatalf("SaveJob other returned error: %v", err)
	}
	writeFile(t, filepath.Join(checkpointDir, "target.json"), `{"schemaVersion":4,"novelId":"novel-1"}`)
	writeFile(t, filepath.Join(checkpointDir, "other.json"), `{"schemaVersion":4,"novelId":"novel-2"}`)

	result, err := PruneNovelState(stateDir, "novel-1")
	if err != nil {
		t.Fatalf("PruneNovelState returned error: %v", err)
	}
	if !result.ProfileDeleted || !result.EventsDeleted || !result.TermProfileDeleted || result.JobsDeleted != 1 || !result.JobIndexDeleted || result.CheckpointsDeleted != 1 {
		t.Fatalf("unexpected prune result: %+v", result)
	}
	for _, path := range []string{
		filepath.Join(profileDir, "novel-1.yaml"),
		filepath.Join(eventsDir, "novel-1.yaml"),
		filepath.Join(termDir, "novel-1.yaml"),
		filepath.Join(stateDir, "extraction_jobs", "job-target.yaml"),
		filepath.Join(stateDir, "extraction_jobs", "index", "novel-1.yaml"),
		filepath.Join(checkpointDir, "target.json"),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("target file should be removed: path=%s err=%v", path, err)
		}
	}
	if jobs, ok, err := LoadJobs(stateDir, "novel-2"); err != nil || !ok || len(jobs) != 1 || jobs[0].JobID != "job-other" {
		t.Fatalf("other novel jobs should remain: ok=%v jobs=%+v err=%v", ok, jobs, err)
	}
	for _, path := range []string{filepath.Join(checkpointDir, "other.json")} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("non-target checkpoint should remain: path=%s err=%v", path, err)
		}
	}

	blank, err := PruneNovelState(stateDir, " ")
	if err != nil || blank.ProfileDeleted || blank.EventsDeleted || blank.JobsDeleted != 0 || blank.JobIndexDeleted || blank.CheckpointsDeleted != 0 {
		t.Fatalf("blank prune should be a no-op: result=%+v err=%v", blank, err)
	}
	missing, err := PruneNovelState(stateDir, "missing")
	if err != nil || missing.ProfileDeleted || missing.EventsDeleted || missing.JobsDeleted != 0 || missing.JobIndexDeleted || missing.CheckpointsDeleted != 0 {
		t.Fatalf("missing prune should be a no-op: result=%+v err=%v", missing, err)
	}
}

func TestPruneNovelStateRejectsMalformedCheckpointBeforeDeletingAnything(t *testing.T) {
	stateDir := t.TempDir()
	profilePath := filepath.Join(stateDir, "character_profiles", "novel-1.yaml")
	checkpointPath := filepath.Join(stateDir, "extraction_jobs", "checkpoints", "broken.json")
	for _, dir := range []string{filepath.Dir(profilePath), filepath.Dir(checkpointPath)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir prune fixture: %v", err)
		}
	}
	writeFile(t, profilePath, `novel_id: novel-1`)
	writeFile(t, checkpointPath, `{`)

	if result, err := PruneNovelState(stateDir, "novel-1"); err == nil || result.ProfileDeleted {
		t.Fatalf("malformed checkpoint should stop prune preflight: result=%+v err=%v", result, err)
	}
	for _, path := range []string{profilePath, checkpointPath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("preflight rejection should preserve %s: %v", path, err)
		}
	}
}

func TestPruneNovelStateIfNoActiveRejectsActiveJobs(t *testing.T) {
	stateDir := t.TempDir()
	profileDir := filepath.Join(stateDir, "character_profiles")
	eventsDir := filepath.Join(stateDir, "character_events")
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		t.Fatalf("mkdir profile dir: %v", err)
	}
	if err := os.MkdirAll(eventsDir, 0o755); err != nil {
		t.Fatalf("mkdir events dir: %v", err)
	}
	writeFile(t, filepath.Join(profileDir, "active.yaml"), `novel_id: active`)
	writeFile(t, filepath.Join(eventsDir, "active.yaml"), `novel_id: active`)
	if err := SaveJob(stateDir, "active", Job{JobID: "job-active", RequestedUpToEpisodeIndex: "1", Status: "running", CreatedAt: "2026-01-01T00:00:00Z"}); err != nil {
		t.Fatalf("SaveJob active returned error: %v", err)
	}
	if result, active, err := PruneNovelStateIfNoActive(stateDir, "active"); err != nil || !active || result.ProfileDeleted || result.EventsDeleted {
		t.Fatalf("active job should block prune: result=%+v active=%v err=%v", result, active, err)
	}
	if _, err := os.Stat(filepath.Join(profileDir, "active.yaml")); err != nil {
		t.Fatalf("blocked prune should keep profile: %v", err)
	}

	writeFile(t, filepath.Join(profileDir, "done.yaml"), `novel_id: done`)
	writeFile(t, filepath.Join(eventsDir, "done.yaml"), `novel_id: done`)
	if err := SaveJob(stateDir, "done", Job{JobID: "job-done", RequestedUpToEpisodeIndex: "1", Status: "completed", CreatedAt: "2026-01-01T00:00:00Z"}); err != nil {
		t.Fatalf("SaveJob done returned error: %v", err)
	}
	result, active, err := PruneNovelStateIfNoActive(stateDir, "done")
	if err != nil || active || !result.ProfileDeleted || !result.EventsDeleted || result.JobsDeleted != 1 {
		t.Fatalf("completed job should allow prune: result=%+v active=%v err=%v", result, active, err)
	}
	blank, active, err := PruneNovelStateIfNoActive(stateDir, " ")
	if err != nil || active || blank.ProfileDeleted || blank.EventsDeleted {
		t.Fatalf("blank prune should be a no-op: result=%+v active=%v err=%v", blank, active, err)
	}
}

func TestSaveJobIfNoActiveKeepsActiveJobAtomic(t *testing.T) {
	stateDir := t.TempDir()
	completed := Job{
		JobID:                     "job-complete",
		RequestedUpToEpisodeIndex: "1",
		GenerationMode:            "heuristic",
		Status:                    "completed",
		CreatedAt:                 "2026-01-01T00:00:00Z",
	}
	if saved, created, err := SaveJobIfNoActive(stateDir, "novel-1", completed); err != nil || !created || saved.JobID != completed.JobID {
		t.Fatalf("completed job should be saved first: saved=%+v created=%v err=%v", saved, created, err)
	}
	active := Job{
		JobID:                     "job-active",
		RequestedUpToEpisodeIndex: "2",
		GenerationMode:            "heuristic",
		Status:                    "queued",
		CreatedAt:                 "2026-01-02T00:00:00Z",
	}
	if saved, created, err := SaveJobIfNoActive(stateDir, "novel-1", active); err != nil || !created || saved.JobID != active.JobID {
		t.Fatalf("active job should be saved when no active exists: saved=%+v created=%v err=%v", saved, created, err)
	}
	next := Job{
		JobID:                     "job-next",
		RequestedUpToEpisodeIndex: "3",
		GenerationMode:            "heuristic",
		Status:                    "queued",
		CreatedAt:                 "2026-01-03T00:00:00Z",
	}
	if saved, created, err := SaveJobIfNoActive(stateDir, "novel-1", next); err != nil || created || saved.JobID != active.JobID {
		t.Fatalf("existing active job should be returned: saved=%+v created=%v err=%v", saved, created, err)
	}
	jobs, ok, err := LoadJobs(stateDir, "novel-1")
	if err != nil || !ok {
		t.Fatalf("LoadJobs after SaveJobIfNoActive failed: ok=%v err=%v", ok, err)
	}
	if len(jobs) != 2 {
		t.Fatalf("next job should not be saved while an active job exists: %+v", jobs)
	}
}

func TestRecoverRunningJobsRequeuesInterruptedJobs(t *testing.T) {
	stateDir := t.TempDir()
	progress := 40
	stage := "batch"
	startedAt := "2026-01-01T00:00:01Z"
	if err := SaveJob(stateDir, "novel-1", Job{
		JobID:                     "job-running",
		RequestedUpToEpisodeIndex: "2",
		GenerationMode:            "heuristic",
		Status:                    "running",
		Progress:                  &progress,
		ProgressStage:             &stage,
		CreatedAt:                 "2026-01-01T00:00:00Z",
		StartedAt:                 &startedAt,
	}); err != nil {
		t.Fatalf("SaveJob running returned error: %v", err)
	}
	if err := SaveJob(stateDir, "novel-1", Job{
		JobID:                     "job-completed",
		RequestedUpToEpisodeIndex: "1",
		GenerationMode:            "heuristic",
		Status:                    "completed",
		CreatedAt:                 "2026-01-02T00:00:00Z",
	}); err != nil {
		t.Fatalf("SaveJob completed returned error: %v", err)
	}

	recovered, err := RecoverRunningJobs(stateDir)
	if err != nil {
		t.Fatalf("RecoverRunningJobs returned error: %v", err)
	}
	if recovered != 1 {
		t.Fatalf("RecoverRunningJobs recovered %d jobs, want 1", recovered)
	}
	jobs, ok, err := LoadJobs(stateDir, "novel-1")
	if err != nil || !ok {
		t.Fatalf("LoadJobs after recovery failed: ok=%v err=%v", ok, err)
	}
	byID := map[string]Job{}
	for _, job := range jobs {
		byID[job.JobID] = job
	}
	requeued := byID["job-running"]
	if requeued.Status != "queued" || requeued.StartedAt != nil || requeued.FinishedAt != nil || requeued.ErrorMessage != nil {
		t.Fatalf("running job should be reset to queued: %+v", requeued)
	}
	if requeued.Progress == nil || *requeued.Progress != 0 || requeued.ProgressStage == nil || *requeued.ProgressStage != "recovered" {
		t.Fatalf("recovered job should expose reset progress metadata: %+v", requeued)
	}
	if byID["job-completed"].Status != "completed" {
		t.Fatalf("completed job should not be changed: %+v", byID["job-completed"])
	}
}

func TestRecoverRunningJobsDoesNotRequeueUnsupportedCheckpoint(t *testing.T) {
	stateDir := t.TempDir()
	if err := SaveJob(stateDir, "novel-checkpoint", Job{
		JobID:                     "job-running-incompatible",
		RequestedUpToEpisodeIndex: "2",
		Status:                    "running",
		CreatedAt:                 "2026-01-01T00:00:00Z",
	}); err != nil {
		t.Fatalf("SaveJob: %v", err)
	}
	checkpointPath := checkpointstore.NewFileStore(stateDir).Path("novel-checkpoint", "2")
	if err := os.MkdirAll(filepath.Dir(checkpointPath), 0o755); err != nil {
		t.Fatalf("mkdir checkpoint dir: %v", err)
	}
	if err := os.WriteFile(checkpointPath, []byte(`{"schemaVersion":99,"novelId":"novel-checkpoint","upToEpisodeIndex":"2"}`), 0o600); err != nil {
		t.Fatalf("write checkpoint: %v", err)
	}
	recovered, err := RecoverRunningJobs(stateDir)
	if err != nil || recovered != 0 {
		t.Fatalf("RecoverRunningJobs = %d, err=%v", recovered, err)
	}
	jobs, ok, err := LoadJobs(stateDir, "novel-checkpoint")
	if err != nil || !ok || len(jobs) != 1 || jobs[0].Status != "incompatible" || jobs[0].ErrorMessage == nil {
		t.Fatalf("jobs = %+v, ok=%v err=%v", jobs, ok, err)
	}
	if strings.Contains(*jobs[0].ErrorMessage, stateDir) || !strings.Contains(*jobs[0].ErrorMessage, "互換性") {
		t.Fatalf("job error message should be stable and path-free: %q", *jobs[0].ErrorMessage)
	}
	quarantined, err := filepath.Glob(checkpointPath + ".unsupported-*")
	if err != nil || len(quarantined) != 1 {
		t.Fatalf("quarantined checkpoints = %v, err=%v", quarantined, err)
	}
}

func TestLoadSummaryAndJobsHandleMissingFiles(t *testing.T) {
	stateDir := t.TempDir()
	if jobs, ok, err := LoadJobs(stateDir, "missing"); err != nil || ok || len(jobs) != 0 {
		t.Fatalf("missing jobs should return empty ok=false, ok=%v err=%v jobs=%+v", ok, err, jobs)
	}
}

func TestJobFileNameValidation(t *testing.T) {
	if fileName, err := safeJobFileName(" go-job-123 "); err != nil || fileName != "go-job-123" {
		t.Fatalf("safe job filename should preserve TS-compatible IDs: fileName=%q err=%v", fileName, err)
	}
	for _, jobID := range []string{"job/new", "a:b", " ", "CON", "lpt1"} {
		if _, err := safeJobFileName(jobID); err == nil {
			t.Fatalf("safe job filename should reject unsafe ID %q", jobID)
		}
	}
}

func TestSaveJobUsesTSCompatibleFileNamesAndRejectsUnsafeIDs(t *testing.T) {
	stateDir := t.TempDir()
	for _, jobID := range []string{"go-job-1", "job_new"} {
		if err := SaveJob(stateDir, "novel-1", Job{
			JobID:                     jobID,
			RequestedUpToEpisodeIndex: "1",
			GenerationMode:            "heuristic",
			Status:                    "completed",
			CreatedAt:                 "2026-01-01T00:00:00Z",
		}); err != nil {
			t.Fatalf("SaveJob(%q) returned error: %v", jobID, err)
		}
	}
	if _, err := os.Stat(filepath.Join(stateDir, "extraction_jobs", "go-job-1.yaml")); err != nil {
		t.Fatalf("TS-compatible job ID should be saved under its raw filename: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(stateDir, "extraction_jobs", "go-job-1.yaml"))
	if err != nil {
		t.Fatalf("read saved job yaml: %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, "schema_version: 2") || !strings.Contains(text, "revision: 1") || !strings.Contains(text, "job_id: go-job-1") {
		t.Fatalf("saved job yaml should include TS-compatible metadata: %s", text)
	}
	if err := SaveJob(stateDir, "novel-1", Job{
		JobID:                     "job/new",
		RequestedUpToEpisodeIndex: "1",
		GenerationMode:            "heuristic",
		Status:                    "completed",
		CreatedAt:                 "2026-01-01T00:00:00Z",
	}); err == nil {
		t.Fatal("unsafe job ID should be rejected before writing an incompatible index entry")
	}
	jobs, ok, err := LoadJobs(stateDir, "novel-1")
	if err != nil || !ok || len(jobs) != 2 {
		t.Fatalf("safe job IDs should both be stored: ok=%v jobs=%+v err=%v", ok, jobs, err)
	}
}

func TestMalformedJobStopsReadsAndNewJobMutation(t *testing.T) {
	stateDir := t.TempDir()
	jobDir := filepath.Join(stateDir, "extraction_jobs")
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		t.Fatalf("mkdir job dir: %v", err)
	}
	writeFile(t, filepath.Join(jobDir, "bad.yaml"), "job_id: [")
	if jobs, ok, err := LoadJobs(stateDir, "novel-1"); err == nil || ok || len(jobs) != 0 {
		t.Fatalf("invalid job yaml should stop reads, jobs=%+v ok=%v err=%v", jobs, ok, err)
	}
	if _, created, err := SaveJobIfNoActive(stateDir, "novel-1", Job{JobID: "new-job", Status: "queued"}); err == nil || created {
		t.Fatalf("invalid job yaml should stop new job mutation: created=%v err=%v", created, err)
	}
}

func TestFutureJobIsListedAsIncompatibleAndNeverRequeuedOrOverwritten(t *testing.T) {
	stateDir := t.TempDir()
	jobDir := filepath.Join(stateDir, "extraction_jobs")
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		t.Fatalf("mkdir job dir: %v", err)
	}
	path := filepath.Join(jobDir, "future-job.yaml")
	future := []byte(`schema_version: 99
revision: 8
job_id: future-job
novel_id: novel-future
requested_up_to_episode_index: "2"
status: running
created_at: 2026-01-01T00:00:00Z
`)
	if err := os.WriteFile(path, future, 0o644); err != nil {
		t.Fatalf("write future job: %v", err)
	}
	jobs, ok, err := LoadJobs(stateDir, "novel-future")
	if err != nil || !ok || len(jobs) != 1 || jobs[0].Status != "incompatible" || jobs[0].ErrorMessage == nil {
		t.Fatalf("future jobs = %+v, ok=%v err=%v", jobs, ok, err)
	}
	if recovered, err := RecoverRunningJobs(stateDir); err != nil || recovered != 0 {
		t.Fatalf("RecoverRunningJobs = %d, err=%v", recovered, err)
	}
	err = schemaguardtest.AssertFileUntouched(t, path, func() error {
		return SaveJob(stateDir, "novel-future", Job{JobID: "future-job", Status: "queued"})
	})
	var guardError *schemaguard.GuardError
	if !errors.As(err, &guardError) || guardError.Result.Status != schemaguard.StatusFutureUnknown {
		t.Fatalf("SaveJob error = %#v, want future GuardError", err)
	}
	err = schemaguardtest.AssertFileUntouched(t, path, func() error {
		_, created, enqueueErr := SaveJobIfNoActive(stateDir, "novel-future", Job{JobID: "different-job", Status: "queued"})
		if created {
			t.Fatal("future job must block enqueue of a different job ID for the same novel")
		}
		return enqueueErr
	})
	if !errors.As(err, &guardError) || guardError.Result.Status != schemaguard.StatusFutureUnknown {
		t.Fatalf("SaveJobIfNoActive error = %#v, want future GuardError", err)
	}
	if _, statErr := os.Stat(filepath.Join(jobDir, "different-job.yaml")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("blocked enqueue created a new job file: %v", statErr)
	}
}

func TestSaveJobRebuildsUnsupportedDerivedIndex(t *testing.T) {
	stateDir := t.TempDir()
	if err := SaveJob(stateDir, "novel-index", Job{JobID: "job-1", Status: "completed", CreatedAt: "2026-01-01T00:00:00Z"}); err != nil {
		t.Fatalf("SaveJob job-1: %v", err)
	}
	path := filepath.Join(stateDir, "extraction_jobs", "index", "novel-index.yaml")
	future := []byte("schema_version: 99\nnovel_id: novel-index\njob_ids: [job-1]\n")
	if err := os.WriteFile(path, future, 0o644); err != nil {
		t.Fatalf("write future index: %v", err)
	}
	if err := SaveJob(stateDir, "novel-index", Job{JobID: "job-2", Status: "queued", CreatedAt: "2026-01-02T00:00:00Z"}); err != nil {
		t.Fatalf("SaveJob job-2: %v", err)
	}
	quarantined, err := filepath.Glob(path + ".unsupported-*")
	if err != nil || len(quarantined) != 1 {
		t.Fatalf("quarantined indexes = %v, err=%v", quarantined, err)
	}
	raw, err := os.ReadFile(quarantined[0])
	if err != nil || string(raw) != string(future) {
		t.Fatalf("quarantined index bytes = %q, err=%v", raw, err)
	}
	rebuilt, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(rebuilt), "schema_version: 2") || !strings.Contains(string(rebuilt), "job-1") || !strings.Contains(string(rebuilt), "job-2") {
		t.Fatalf("rebuilt index = %q, err=%v", rebuilt, err)
	}
}

func TestExtractionYAMLWriteErrors(t *testing.T) {
	blockedParent := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(blockedParent, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("write blocked parent: %v", err)
	}
	if err := SaveJob(blockedParent, "novel-1", Job{JobID: "job-1"}); err == nil {
		t.Fatal("SaveJob should report parent directory errors")
	}
	if err := writeYAMLAtomic(filepath.Join(t.TempDir(), "bad.yaml"), map[string]any{"bad": make(chan int)}); err == nil {
		t.Fatal("writeYAMLAtomic should report marshal errors")
	}
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
