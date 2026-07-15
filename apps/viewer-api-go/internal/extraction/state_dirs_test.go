package extraction

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureStateDirsCreatesJobDirectories(t *testing.T) {
	stateDir := t.TempDir()
	if err := EnsureStateDirs(stateDir); err != nil {
		t.Fatalf("EnsureStateDirs returned error: %v", err)
	}
	path := filepath.Join(stateDir, "extraction_jobs", "index")
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		t.Fatalf("job index directory was not created: info=%+v err=%v", info, err)
	}
}

func TestEnsureStateDirsReportsBlockedJobDirectory(t *testing.T) {
	stateDir := t.TempDir()
	blocked := filepath.Join(stateDir, "extraction_jobs")
	if err := os.WriteFile(blocked, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("write blocked path: %v", err)
	}
	if err := EnsureStateDirs(stateDir); err == nil {
		t.Fatal("EnsureStateDirs should report a blocked job directory")
	}
}

func TestEnsureStateDirsRemovesObsoleteJobArtifacts(t *testing.T) {
	stateDir := t.TempDir()
	legacyIndex := filepath.Join(stateDir, "character_jobs", "index")
	if err := os.MkdirAll(legacyIndex, 0o755); err != nil {
		t.Fatalf("mkdir legacy index: %v", err)
	}
	legacyJob := filepath.Join(stateDir, "character_jobs", "job.yaml")
	if err := os.WriteFile(legacyJob, []byte("job_id: legacy"), 0o644); err != nil {
		t.Fatalf("write legacy job: %v", err)
	}
	jobsDir := filepath.Join(stateDir, "extraction_jobs")
	if err := os.MkdirAll(filepath.Join(jobsDir, "legacy_conflicts", "checkpoints"), 0o755); err != nil {
		t.Fatalf("mkdir obsolete conflict directory: %v", err)
	}
	canonicalJob := filepath.Join(jobsDir, "job.yaml")
	if err := os.WriteFile(canonicalJob, []byte("job_id: canonical"), 0o644); err != nil {
		t.Fatalf("write canonical job: %v", err)
	}
	conflictCheckpoint := filepath.Join(jobsDir, "legacy_conflicts", "checkpoints", "legacy.json")
	if err := os.WriteFile(conflictCheckpoint, []byte(`{"novelId":"legacy"}`), 0o600); err != nil {
		t.Fatalf("write obsolete conflict checkpoint: %v", err)
	}
	if err := EnsureStateDirs(stateDir); err != nil {
		t.Fatalf("EnsureStateDirs returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(jobsDir, "index")); err != nil {
		t.Fatalf("canonical job index directory was not created: %v", err)
	}
	for _, obsoletePath := range []string{filepath.Join(stateDir, "character_jobs"), filepath.Join(jobsDir, "legacy_conflicts")} {
		if _, err := os.Stat(obsoletePath); !os.IsNotExist(err) {
			t.Fatalf("obsolete job artifacts should be removed: path=%s err=%v", obsoletePath, err)
		}
	}
	if canonical, err := os.ReadFile(canonicalJob); err != nil || string(canonical) != "job_id: canonical" {
		t.Fatalf("canonical job should be preserved: %q err=%v", canonical, err)
	}
}

func TestEnsureStateDirsContinuesWhenObsoleteArtifactsCannotBeRemoved(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission failure cannot be reproduced as root")
	}

	stateDir := t.TempDir()
	legacyDir := filepath.Join(stateDir, "character_jobs")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatalf("mkdir legacy job directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "job.yaml"), []byte("job_id: legacy"), 0o644); err != nil {
		t.Fatalf("write legacy job: %v", err)
	}
	indexDir := filepath.Join(stateDir, "extraction_jobs", "index")
	if err := os.MkdirAll(indexDir, 0o755); err != nil {
		t.Fatalf("mkdir canonical job index: %v", err)
	}
	if err := os.Chmod(stateDir, 0o500); err != nil {
		t.Fatalf("make state directory read-only: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(stateDir, 0o700)
	})

	if err := EnsureStateDirs(stateDir); err != nil {
		t.Fatalf("obsolete cleanup failure should not prevent startup: %v", err)
	}
	if info, err := os.Stat(legacyDir); err != nil || !info.IsDir() {
		t.Fatalf("failed obsolete cleanup should leave the undeletable directory: info=%+v err=%v", info, err)
	}
}
