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
	path := filepath.Join(stateDir, "character_jobs", "index")
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		t.Fatalf("job index directory was not created: info=%+v err=%v", info, err)
	}
}

func TestEnsureStateDirsReportsBlockedJobDirectory(t *testing.T) {
	stateDir := t.TempDir()
	blocked := filepath.Join(stateDir, "character_jobs")
	if err := os.WriteFile(blocked, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("write blocked path: %v", err)
	}
	if err := EnsureStateDirs(stateDir); err == nil {
		t.Fatal("EnsureStateDirs should report a blocked job directory")
	}
}
