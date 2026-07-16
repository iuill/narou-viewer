package novelsettings

import (
	"os"
	"path/filepath"
	"testing"

	"narou-viewer/apps/viewer-api-go/internal/state/schemaguard"
	"narou-viewer/apps/viewer-api-go/internal/state/schemaguardtest"
)

func TestRepositoryGetsPutsPatchesAndPrunes(t *testing.T) {
	stateDir := t.TempDir()
	repo := NewRepository(stateDir)
	if err := repo.Ensure(); err != nil {
		t.Fatalf("Ensure returned error: %v", err)
	}
	defaults, err := repo.Get("novel")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if defaults.NovelID != "novel" || !defaults.Correction.QuoteNormalization || defaults.UpdatedAt != nil {
		t.Fatalf("unexpected defaults: %+v", defaults)
	}
	updated, err := repo.Put(Settings{NovelID: "novel", Correction: Correction{
		QuoteNormalization:                     true,
		HyphenDashNormalization:                true,
		ParenthesisNormalization:               true,
		HalfwidthAlnumPunctuationNormalization: true,
	}})
	if err != nil {
		t.Fatalf("Put returned error: %v", err)
	}
	if updated.UpdatedAt == nil {
		t.Fatalf("Put should set UpdatedAt: %+v", updated)
	}
	falseValue := false
	patched, err := repo.Patch("novel", Patch{QuoteNormalization: &falseValue})
	if err != nil {
		t.Fatalf("Patch returned error: %v", err)
	}
	if patched.Correction.QuoteNormalization || !patched.Correction.HyphenDashNormalization {
		t.Fatalf("Patch should update one field and preserve omitted fields: %+v", patched)
	}
	patchedUpdatedAt := patched.UpdatedAt
	emptyPatched, err := repo.Patch("novel", Patch{})
	if err != nil {
		t.Fatalf("empty Patch returned error: %v", err)
	}
	if emptyPatched.UpdatedAt == nil || patchedUpdatedAt == nil || *emptyPatched.UpdatedAt != *patchedUpdatedAt {
		t.Fatalf("empty patch should not write: before=%v after=%v", patchedUpdatedAt, emptyPatched.UpdatedAt)
	}
	deleted, err := repo.PruneNovel("novel")
	if err != nil {
		t.Fatalf("PruneNovel returned error: %v", err)
	}
	if !deleted {
		t.Fatal("PruneNovel should report deletion")
	}
	deleted, err = repo.PruneNovel("novel")
	if err != nil || deleted {
		t.Fatalf("second PruneNovel should be no-op, deleted=%v err=%v", deleted, err)
	}
}

func TestRepositoryNormalizesAndHandlesCorruptDocuments(t *testing.T) {
	updatedAt := "2026-01-02T03:04:05.000Z"
	normalized := normalizeDocument(document{
		Revision: -1,
		Novels: map[string]record{
			" ": {
				Correction: correctionRecord{QuoteNormalization: boolPtr(true)},
				UpdatedAt:  &updatedAt,
			},
			"novel": {
				Correction: correctionRecord{
					QuoteNormalization:                     boolPtr(true),
					HyphenDashNormalization:                boolPtr(true),
					ParenthesisNormalization:               boolPtr(true),
					HalfwidthAlnumPunctuationNormalization: boolPtr(true),
				},
				UpdatedAt: &updatedAt,
			},
		},
	})
	if normalized.Revision != 0 || len(normalized.Novels) != 1 {
		t.Fatalf("unexpected normalized document: %+v", normalized)
	}
	if settings := toSettings("novel", normalized.Novels["novel"]); !settings.Correction.QuoteNormalization || settings.UpdatedAt == nil {
		t.Fatalf("unexpected normalized settings: %+v", settings)
	}

	stateDir := t.TempDir()
	repo := NewRepository(stateDir)
	if err := os.WriteFile(filepath.Join(stateDir, FileName), []byte("novels: ["), 0o644); err != nil {
		t.Fatalf("write corrupt settings: %v", err)
	}
	if _, err := repo.Get("novel"); err == nil {
		t.Fatal("Get should fail for corrupt yaml")
	}
	if _, err := repo.Put(Settings{NovelID: "novel"}); err == nil {
		t.Fatal("Put should fail for corrupt yaml")
	}
	if _, err := repo.Patch("novel", Patch{}); err == nil {
		t.Fatal("Patch should fail for corrupt yaml")
	}
}

func TestRepositoryRejectsUnsupportedSchemasWithoutMutation(t *testing.T) {
	tests := []struct {
		name       string
		document   string
		wantStatus schemaguard.Status
	}{
		{name: "future", document: "schema_version: 999\nrevision: 1\nnovels: {}\n", wantStatus: schemaguard.StatusFutureUnknown},
		{name: "missing version", document: "revision: 1\nnovels: {}\n", wantStatus: schemaguard.StatusUnsupportedLegacy},
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
			value := false
			err := schemaguardtest.AssertFileUntouched(t, path, func() error {
				_, err := repo.Patch("novel", Patch{QuoteNormalization: &value})
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
