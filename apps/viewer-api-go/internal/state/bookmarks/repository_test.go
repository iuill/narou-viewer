package bookmarks

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"narou-viewer/apps/viewer-api-go/internal/state/schemaguard"
	"narou-viewer/apps/viewer-api-go/internal/state/schemaguardtest"
)

func TestRepositoryCreatesListsDeletesAndPrunes(t *testing.T) {
	stateDir := t.TempDir()
	repo := NewRepository(stateDir)
	if err := repo.Ensure(); err != nil {
		t.Fatalf("Ensure returned error: %v", err)
	}
	label := " label "
	first, err := repo.Create(Bookmark{NovelID: "novel-1", EpisodeIndex: "1", Position: 10, Label: &label})
	if err != nil {
		t.Fatalf("Create first returned error: %v", err)
	}
	second, err := repo.Create(Bookmark{NovelID: "novel-1", EpisodeIndex: "2", Position: 20})
	if err != nil {
		t.Fatalf("Create second returned error: %v", err)
	}
	if first.ID == "" || second.ID == "" || first.ID == second.ID {
		t.Fatalf("bookmarks should have unique ids: first=%+v second=%+v", first, second)
	}
	if first.Label == nil || *first.Label != "label" {
		t.Fatalf("label should be normalized: %+v", first)
	}
	negative, err := repo.Create(Bookmark{NovelID: "novel-2", EpisodeIndex: "1", Position: -10})
	if err != nil {
		t.Fatalf("Create negative position returned error: %v", err)
	}
	if negative.Position != 0 {
		t.Fatalf("negative position should be normalized: %+v", negative)
	}
	bookmarks, err := repo.List("novel-1")
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(bookmarks) != 2 || bookmarks[0].ID != second.ID || bookmarks[1].ID != first.ID {
		t.Fatalf("bookmarks should be newest first: %+v", bookmarks)
	}
	if err := repo.Delete(first.ID); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	if err := repo.Delete(first.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	deleted, err := repo.PruneNovel("novel-1")
	if err != nil {
		t.Fatalf("PruneNovel returned error: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("PruneNovel should delete remaining bookmark, got %d", deleted)
	}
	deleted, err = repo.PruneNovel("novel-1")
	if err != nil || deleted != 0 {
		t.Fatalf("second PruneNovel should be no-op, deleted=%d err=%v", deleted, err)
	}
}

func TestRepositoryRejectsInvalidCreateInput(t *testing.T) {
	repo := NewRepository(t.TempDir())
	if _, err := repo.Create(Bookmark{NovelID: "novel", EpisodeIndex: "bad"}); !errors.Is(err, ErrInvalidBookmark) {
		t.Fatalf("invalid episode index should be rejected, got %v", err)
	}
	if _, err := repo.Create(Bookmark{NovelID: " ", EpisodeIndex: "1"}); !errors.Is(err, ErrInvalidBookmark) {
		t.Fatalf("blank novel id should be rejected, got %v", err)
	}
	bookmarks, err := repo.List("")
	if err != nil || len(bookmarks) != 0 {
		t.Fatalf("invalid create should not write bookmarks: bookmarks=%+v err=%v", bookmarks, err)
	}
}

func TestRepositoryNormalizesAndHandlesMissingAndCorruptDocuments(t *testing.T) {
	stateDir := t.TempDir()
	repo := NewRepository(stateDir)
	bookmarks, err := repo.List("")
	if err != nil || len(bookmarks) != 0 {
		t.Fatalf("missing file should return empty list: bookmarks=%+v err=%v", bookmarks, err)
	}
	normalized := normalizeDocument(document{
		Revision: -1,
		Bookmarks: []record{
			{ID: "", NovelID: "novel", EpisodeIndex: "1", CreatedAt: "2026-01-01T00:00:00Z"},
			{ID: "bm", NovelID: "novel", EpisodeIndex: "1", Position: -1, Label: strPtr(" label "), CreatedAt: "2026-01-01T00:00:00Z"},
		},
	})
	if len(normalized.Bookmarks) != 1 || normalized.Bookmarks[0].Position != 0 || normalized.Bookmarks[0].Label == nil || *normalized.Bookmarks[0].Label != "label" {
		t.Fatalf("unexpected normalized document: %+v", normalized)
	}
	if err := os.WriteFile(filepath.Join(stateDir, FileName), []byte("bookmarks: ["), 0o644); err != nil {
		t.Fatalf("write corrupt bookmarks: %v", err)
	}
	if _, err := repo.List(""); err == nil {
		t.Fatal("corrupt bookmarks should return error")
	}
}

func TestRepositoryRejectsUnsupportedSchemasWithoutMutation(t *testing.T) {
	tests := []struct {
		name       string
		document   string
		wantStatus schemaguard.Status
	}{
		{name: "future", document: "schema_version: 999\nrevision: 1\nbookmarks: []\n", wantStatus: schemaguard.StatusFutureUnknown},
		{name: "missing version", document: "revision: 1\nbookmarks: []\n", wantStatus: schemaguard.StatusUnsupportedLegacy},
		{name: "malformed", document: "schema_version: [\n", wantStatus: schemaguard.StatusMalformed},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stateDir := t.TempDir()
			path := filepath.Join(stateDir, FileName)
			if err := os.WriteFile(path, []byte(test.document), 0o644); err != nil {
				t.Fatalf("write guarded fixture: %v", err)
			}
			repo := NewRepository(stateDir)
			err := schemaguardtest.AssertFileUntouched(t, path, func() error {
				_, err := repo.Create(Bookmark{NovelID: "novel", EpisodeIndex: "1"})
				return err
			})
			assertGuardStatus(t, err, test.wantStatus)
			err = schemaguardtest.AssertFileUntouched(t, path, func() error {
				_, err := repo.PruneNovel("novel")
				return err
			})
			assertGuardStatus(t, err, test.wantStatus)
		})
	}
}

func assertGuardStatus(t *testing.T, err error, want schemaguard.Status) {
	t.Helper()
	guardError, ok := schemaguard.AsGuardError(err)
	if !ok || guardError.Result.Status != want {
		t.Fatalf("guard error = %#v, want status %s", err, want)
	}
}

func strPtr(value string) *string {
	return &value
}
