package readingstate

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepositoryGetPutConflictAndTombstone(t *testing.T) {
	stateDir := t.TempDir()
	repo := NewRepository(stateDir)
	if err := repo.Ensure(); err != nil {
		t.Fatalf("Ensure returned error: %v", err)
	}

	missing, err := repo.Get("novel")
	if err != nil {
		t.Fatalf("Get missing returned error: %v", err)
	}
	if missing.NovelID != "novel" || missing.Position != 0 || missing.StateVersion != 0 {
		t.Fatalf("unexpected missing state: %+v", missing)
	}

	episodeIndex := "3"
	clientID := " client "
	stored, err := repo.Put(PutInput{State: State{
		NovelID:              "novel",
		LastReadEpisodeIndex: &episodeIndex,
		Position:             42,
		Scroll:               &ScrollState{Type: "ratio", Value: 0.75},
		UpdatedByClientID:    &clientID,
	}})
	if err != nil {
		t.Fatalf("Put returned error: %v", err)
	}
	if stored.LastReadEpisodeIndex == nil || *stored.LastReadEpisodeIndex != "3" || stored.Position != 42 || stored.StateVersion != 1 || stored.UpdatedByClientID == nil || *stored.UpdatedByClientID != "client" {
		t.Fatalf("unexpected stored state: %+v", stored)
	}
	raw, err := os.ReadFile(filepath.Join(stateDir, FileName))
	if err != nil {
		t.Fatalf("read stored yaml: %v", err)
	}
	if strings.Contains(string(raw), "line_number:") {
		t.Fatalf("line_number must not be written:\n%s", raw)
	}

	staleVersion := 0
	conflict, err := repo.Put(PutInput{
		State:                State{NovelID: "novel", LastReadEpisodeIndex: &episodeIndex, Position: 99},
		ExpectedStateVersion: &staleVersion,
	})
	if !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("expected version conflict, state=%+v err=%v", conflict, err)
	}
	if conflict.Position != 42 || conflict.StateVersion != stored.StateVersion {
		t.Fatalf("conflict should return current state: %+v", conflict)
	}
	cleared, err := repo.Put(PutInput{State: State{NovelID: "novel", Position: 99}})
	if err != nil {
		t.Fatalf("clear Put returned error: %v", err)
	}
	if cleared.Position != 0 || cleared.LastReadEpisodeIndex != nil || cleared.StateVersion != 2 {
		t.Fatalf("clear should reset position and advance state version: %+v", cleared)
	}

	deleted, err := repo.Prune("novel")
	if err != nil {
		t.Fatalf("Prune returned error: %v", err)
	}
	if !deleted {
		t.Fatal("Prune should report that active state was deleted")
	}
	isDeleted, err := repo.IsDeleted("novel")
	if err != nil {
		t.Fatalf("IsDeleted returned error: %v", err)
	}
	if !isDeleted {
		t.Fatal("IsDeleted should report tombstone")
	}
	tombstone, err := repo.Get("novel")
	if err != nil {
		t.Fatalf("Get tombstone returned error: %v", err)
	}
	if tombstone.Position != 0 || tombstone.LastReadEpisodeIndex != nil || tombstone.StateVersion != 3 {
		t.Fatalf("unexpected tombstone state: %+v", tombstone)
	}
	repeatedDeleted, err := repo.Prune("novel")
	if err != nil {
		t.Fatalf("repeated Prune returned error: %v", err)
	}
	if repeatedDeleted {
		t.Fatal("repeated Prune should not report active state deletion")
	}
	repeatedTombstone, err := repo.Get("novel")
	if err != nil {
		t.Fatalf("Get repeated tombstone returned error: %v", err)
	}
	if repeatedTombstone.StateVersion != 4 {
		t.Fatalf("repeated prune should advance tombstone version: %+v", repeatedTombstone)
	}

	missingDeleted, err := repo.Prune("missing")
	if err != nil {
		t.Fatalf("missing Prune returned error: %v", err)
	}
	if missingDeleted {
		t.Fatal("missing Prune should not report active state deletion")
	}
	missingTombstone, err := repo.Get("missing")
	if err != nil {
		t.Fatalf("Get missing tombstone returned error: %v", err)
	}
	if missingTombstone.StateVersion != 1 || missingTombstone.Position != 0 || missingTombstone.LastReadEpisodeIndex != nil {
		t.Fatalf("missing prune should create a versioned tombstone: %+v", missingTombstone)
	}
	notDeleted, err := repo.IsDeleted("unknown")
	if err != nil {
		t.Fatalf("IsDeleted unknown returned error: %v", err)
	}
	if notDeleted {
		t.Fatal("unknown novel should not be deleted")
	}
}

func TestRepositoryNormalizesLegacyAndInvalidDocument(t *testing.T) {
	reading := normalizeDocument(document{
		Revision: -1,
		Novels: map[string]record{
			"": {},
			"novel": {
				LastReadEpisodeIndex: strPtr("bad"),
				Position:             -10,
				LineNumber:           12,
				Scroll:               &ScrollState{Type: "ratio", Value: 2},
				UpdatedByClientID:    strPtr(" client "),
			},
			"deleted": {
				LastReadEpisodeIndex: strPtr("1"),
				Position:             99,
				StateVersion:         0,
				Deleted:              true,
				UpdatedAt:            strPtr("2026-01-01T00:00:00.000Z"),
			},
		},
	})
	if len(reading.Novels) != 2 || reading.Novels["novel"].Position != 0 || reading.Novels["novel"].Scroll.Value != 1 {
		t.Fatalf("unexpected normalized document: %+v", reading)
	}
	if reading.Novels["novel"].LastReadEpisodeIndex != nil || reading.Novels["novel"].UpdatedByClientID == nil || *reading.Novels["novel"].UpdatedByClientID != "client" {
		t.Fatalf("unexpected normalized active record: %+v", reading.Novels["novel"])
	}
	if reading.Novels["deleted"].Position != 0 || reading.Novels["deleted"].LastReadEpisodeIndex != nil || reading.Novels["deleted"].UpdatedAt != nil || reading.Novels["deleted"].StateVersion != 1 {
		t.Fatalf("unexpected normalized tombstone: %+v", reading.Novels["deleted"])
	}
	if normalizeScrollStatePtr(&ScrollState{Type: "bad", Value: 1}) != nil {
		t.Fatal("invalid scroll ptr should normalize to nil")
	}
	if normalizeClientIDPtr(nil) != nil || normalizeClientIDPtr(strPtr(" ")) != nil {
		t.Fatal("nil and blank client id should normalize to nil")
	}
	if isEpisodeIndex("") || isEpisodeIndex("1a") {
		t.Fatal("invalid episode indexes should be rejected")
	}
}

func TestRepositoryNormalizesWriteInput(t *testing.T) {
	stateDir := t.TempDir()
	repo := NewRepository(stateDir)
	badEpisodeIndex := "bad"
	clientID := " client "
	stored, err := repo.Put(PutInput{State: State{
		NovelID:              "novel",
		LastReadEpisodeIndex: &badEpisodeIndex,
		Position:             -10,
		Scroll:               &ScrollState{Type: "bad", Value: 2},
		UpdatedByClientID:    &clientID,
	}})
	if err != nil {
		t.Fatalf("Put returned error: %v", err)
	}
	if stored.LastReadEpisodeIndex != nil || stored.Position != 0 || stored.Scroll != nil || stored.UpdatedByClientID == nil || *stored.UpdatedByClientID != "client" {
		t.Fatalf("Put should return normalized state: %+v", stored)
	}
	reloaded, err := repo.Get("novel")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if reloaded.LastReadEpisodeIndex != nil || reloaded.Position != 0 || reloaded.Scroll != nil || reloaded.UpdatedByClientID == nil || *reloaded.UpdatedByClientID != "client" {
		t.Fatalf("Get should preserve normalized state: %+v", reloaded)
	}

	episodeIndex := "2"
	stored, err = repo.Put(PutInput{State: State{
		NovelID:              "novel",
		LastReadEpisodeIndex: &episodeIndex,
		Position:             -1,
		Scroll:               &ScrollState{Type: "ratio", Value: 2},
	}})
	if err != nil {
		t.Fatalf("Put valid episode returned error: %v", err)
	}
	if stored.LastReadEpisodeIndex == nil || *stored.LastReadEpisodeIndex != "2" || stored.Position != 0 || stored.Scroll == nil || stored.Scroll.Value != 1 {
		t.Fatalf("Put should normalize position and scroll: %+v", stored)
	}
}

func TestRepositoryHandlesMissingAndCorruptDocuments(t *testing.T) {
	stateDir := t.TempDir()
	repo := NewRepository(stateDir)
	if state, err := repo.Get("novel"); err != nil || state.NovelID != "novel" {
		t.Fatalf("missing reading state should return default, state=%+v err=%v", state, err)
	}
	if err := repo.Ensure(); err != nil {
		t.Fatalf("Ensure returned error: %v", err)
	}
	if err := repo.Ensure(); err != nil {
		t.Fatalf("second Ensure returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, FileName), []byte("novels: ["), 0o644); err != nil {
		t.Fatalf("write corrupt reading state: %v", err)
	}
	if _, err := repo.Get("novel"); err == nil {
		t.Fatal("corrupt reading state should return error")
	}
	if _, err := repo.IsDeleted("novel"); err == nil {
		t.Fatal("corrupt reading state should make IsDeleted return error")
	}
}

func TestRepositoryReturnsWriteErrors(t *testing.T) {
	baseDir := t.TempDir()
	blockedParent := filepath.Join(baseDir, "blocked")
	if err := os.WriteFile(blockedParent, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("write blocked parent: %v", err)
	}
	repo := &Repository{path: filepath.Join(blockedParent, FileName)}
	episodeIndex := "1"
	if err := repo.Ensure(); err == nil {
		t.Fatal("Ensure should return error when parent path is blocked")
	}
	if _, err := repo.Put(PutInput{State: State{NovelID: "novel", LastReadEpisodeIndex: &episodeIndex, Position: 1}}); err == nil {
		t.Fatal("Put should return write error when parent path is blocked")
	}
	if _, err := repo.Prune("novel"); err == nil {
		t.Fatal("Prune should return write error when parent path is blocked")
	}
}

func strPtr(value string) *string {
	return &value
}
