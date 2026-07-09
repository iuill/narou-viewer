package checkpointstore

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"narou-viewer/apps/viewer-api-go/internal/characters"
)

func TestFileStoreSavesLoadsExistsAndDeletesCheckpoint(t *testing.T) {
	store := NewFileStore(t.TempDir())
	checkpoint := Checkpoint{
		SchemaVersion:           1,
		NovelID:                 "novel-a",
		UpToEpisodeIndex:        "2",
		GenerationFingerprint:   "fingerprint-a",
		ProcessedEpisodeIndexes: []string{"1"},
		ProcessedBatchIndexes:   []int{1},
		Characters:              []characters.GeneratedCharacter{{CharacterID: "char-a", CanonicalName: "Alice"}},
		UpdatedAt:               "2026-06-26T00:00:00Z",
	}

	if store.Exists("novel-a", "2") {
		t.Fatal("checkpoint should not exist before save")
	}
	if err := store.Save("novel-a", "2", checkpoint); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	if !store.Exists("novel-a", "2") {
		t.Fatal("checkpoint should exist after save")
	}
	path := store.Path("novel-a", "2")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat checkpoint: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("checkpoint mode = %#o, want 0600", info.Mode().Perm())
	}
	loaded, err := store.Load("novel-a", "2")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if loaded.NovelID != "novel-a" || loaded.GenerationFingerprint != "fingerprint-a" || len(loaded.Characters) != 1 {
		t.Fatalf("loaded checkpoint = %+v", loaded)
	}
	if err := store.Delete("novel-a", "2"); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	if store.Exists("novel-a", "2") {
		t.Fatal("checkpoint should not exist after delete")
	}
}

func TestFileStoreLoadReportsMissingAndInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)
	if _, err := store.Load("missing", "1"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Load missing error = %v, want os.ErrNotExist", err)
	}

	path := store.Path("broken", "1")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir checkpoint dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(`{`), 0o600); err != nil {
		t.Fatalf("write invalid checkpoint: %v", err)
	}
	if _, err := store.Load("broken", "1"); err == nil {
		t.Fatal("Load should fail for invalid JSON")
	}
}

func TestFileStoreSaveReportsPathErrors(t *testing.T) {
	dir := t.TempDir()
	fileBackedStateDir := filepath.Join(dir, "state-as-file")
	if err := os.WriteFile(fileBackedStateDir, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("write state dir blocker: %v", err)
	}
	if err := NewFileStore(fileBackedStateDir).Save("novel-a", "1", Checkpoint{}); err == nil {
		t.Fatal("Save should fail when stateDir is a file")
	}

	store := NewFileStore(filepath.Join(dir, "state"))
	blockedPath := store.Path("novel-a", "1")
	if err := os.MkdirAll(filepath.Dir(blockedPath), 0o755); err != nil {
		t.Fatalf("mkdir checkpoint dir: %v", err)
	}
	if err := os.Mkdir(blockedPath, 0o755); err != nil {
		t.Fatalf("mkdir checkpoint path blocker: %v", err)
	}
	if err := store.Save("novel-a", "1", Checkpoint{}); err == nil {
		t.Fatal("Save should fail when target path is a directory")
	}
}
