package fsatomic

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteFileWritesAtomicallyWithMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := WriteFile(path, []byte(`{"ok":true}`), 0o600); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	if string(raw) != `{"ok":true}` {
		t.Fatalf("written content = %q", raw)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat written file: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("written mode = %#o, want 0600", info.Mode().Perm())
	}
}

func TestWriteFileReportsMissingParentAndCleansTemp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing", "state.json")
	if err := WriteFile(path, []byte("{}"), 0o600); err == nil {
		t.Fatal("WriteFile should fail when the parent directory is missing")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read temp root: %v", err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), "state.json") {
			t.Fatalf("temporary file should be cleaned up: %s", entry.Name())
		}
	}
}

func TestSyncParentDirReportsMissingParent(t *testing.T) {
	err := SyncParentDir(filepath.Join(t.TempDir(), "missing", "state.json"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("SyncParentDir error = %v, want os.ErrNotExist", err)
	}
}
