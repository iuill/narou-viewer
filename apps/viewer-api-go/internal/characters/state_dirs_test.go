package characters

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureStateDirsCreatesManagedDirectories(t *testing.T) {
	stateDir := t.TempDir()
	if err := EnsureStateDirs(stateDir); err != nil {
		t.Fatalf("EnsureStateDirs returned error: %v", err)
	}
	for _, dir := range []string{
		filepath.Join(stateDir, "character_profiles"),
		filepath.Join(stateDir, "character_events"),
	} {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			t.Fatalf("managed directory was not created: dir=%s info=%+v err=%v", dir, info, err)
		}
	}
}

func TestEnsureStateDirsReportsBlockedManagedDirectory(t *testing.T) {
	stateDir := t.TempDir()
	blocked := filepath.Join(stateDir, "character_profiles")
	if err := os.WriteFile(blocked, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("write blocked path: %v", err)
	}
	if err := EnsureStateDirs(stateDir); err == nil {
		t.Fatal("EnsureStateDirs should report a blocked managed directory")
	}
}
