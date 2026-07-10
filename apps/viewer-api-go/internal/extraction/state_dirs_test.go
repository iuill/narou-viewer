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

func TestEnsureStateDirsMigratesLegacyJobDirectoryBeforeCreatingDestination(t *testing.T) {
	stateDir := t.TempDir()
	legacyIndex := filepath.Join(stateDir, "character_jobs", "index")
	if err := os.MkdirAll(legacyIndex, 0o755); err != nil {
		t.Fatalf("mkdir legacy index: %v", err)
	}
	legacyJob := filepath.Join(stateDir, "character_jobs", "job.yaml")
	if err := os.WriteFile(legacyJob, []byte("job_id: legacy"), 0o644); err != nil {
		t.Fatalf("write legacy job: %v", err)
	}
	if err := EnsureStateDirs(stateDir); err != nil {
		t.Fatalf("EnsureStateDirs returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(stateDir, "extraction_jobs", "job.yaml")); err != nil {
		t.Fatalf("legacy job was not migrated: %v", err)
	}
	if _, err := os.Stat(filepath.Join(stateDir, "character_jobs")); !os.IsNotExist(err) {
		t.Fatalf("legacy directory should be renamed, err=%v", err)
	}
}

func TestEnsureStateDirsPrefersExistingExtractionDirectory(t *testing.T) {
	stateDir := t.TempDir()
	for _, dir := range []string{"character_jobs", "extraction_jobs"} {
		if err := os.MkdirAll(filepath.Join(stateDir, dir), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	if err := EnsureStateDirs(stateDir); err != nil {
		t.Fatalf("EnsureStateDirs returned error: %v", err)
	}
	for _, dir := range []string{"character_jobs", "extraction_jobs"} {
		if _, err := os.Stat(filepath.Join(stateDir, dir)); err != nil {
			t.Fatalf("%s should remain when both directories exist: %v", dir, err)
		}
	}
}
