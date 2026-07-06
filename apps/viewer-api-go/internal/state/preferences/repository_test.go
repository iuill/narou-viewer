package preferences

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRepositoryGetsPutsAndNormalizes(t *testing.T) {
	stateDir := t.TempDir()
	repo := NewRepository(stateDir)
	missing, err := repo.Get()
	if err != nil {
		t.Fatalf("Get missing returned error: %v", err)
	}
	if missing.ReadingMode != DefaultReadingMode || missing.FontFamily != DefaultFontFamily || missing.Theme != DefaultTheme {
		t.Fatalf("unexpected defaults: %+v", missing)
	}
	if err := repo.Ensure(); err != nil {
		t.Fatalf("Ensure returned error: %v", err)
	}
	updated, err := repo.Put(Preferences{ReadingMode: "horizontal", FontFamily: "gothic", Theme: "midnight"})
	if err != nil {
		t.Fatalf("Put returned error: %v", err)
	}
	if updated.ReadingMode != "horizontal" || updated.FontFamily != "gothic" || updated.Theme != "midnight" || updated.UpdatedAt == nil {
		t.Fatalf("unexpected updated preferences: %+v", updated)
	}
	ignoredInvalid, err := repo.Put(Preferences{ReadingMode: "bad", FontFamily: "bad", Theme: "bad"})
	if err != nil {
		t.Fatalf("Put invalid values returned error: %v", err)
	}
	if ignoredInvalid.ReadingMode != "horizontal" || ignoredInvalid.FontFamily != "gothic" || ignoredInvalid.Theme != "midnight" {
		t.Fatalf("invalid enum values should not be persisted: %+v", ignoredInvalid)
	}
	reloaded, err := repo.Get()
	if err != nil {
		t.Fatalf("Get after invalid values returned error: %v", err)
	}
	if reloaded.ReadingMode != "horizontal" || reloaded.FontFamily != "gothic" || reloaded.Theme != "midnight" {
		t.Fatalf("invalid enum values should not survive reload: %+v", reloaded)
	}
	normalized := normalizeDocument(document{
		Revision: -1,
		Reader:   record{ReadingMode: "bad", FontFamily: "bad", Theme: "paper", UpdatedAt: strPtr("time")},
	})
	if normalized.Reader.ReadingMode != DefaultReadingMode || normalized.Reader.FontFamily != DefaultFontFamily || normalized.Reader.Theme != "paper" || normalized.Reader.UpdatedAt == nil {
		t.Fatalf("unexpected normalized document: %+v", normalized)
	}
}

func TestRepositoryReturnsCorruptDocumentErrors(t *testing.T) {
	stateDir := t.TempDir()
	repo := NewRepository(stateDir)
	if err := os.WriteFile(filepath.Join(stateDir, FileName), []byte("reader: ["), 0o644); err != nil {
		t.Fatalf("write corrupt preferences: %v", err)
	}
	if _, err := repo.Get(); err == nil {
		t.Fatal("Get should fail for corrupt yaml")
	}
	if _, err := repo.Put(Preferences{Theme: "forest"}); err == nil {
		t.Fatal("Put should fail for corrupt yaml")
	}
}

func TestEnums(t *testing.T) {
	if !IsReadingMode("vertical") || !IsFontFamily("mincho") || !IsTheme("forest") {
		t.Fatal("expected default enum values to be accepted")
	}
	if IsReadingMode("bad") || IsFontFamily("bad") || IsTheme("bad") {
		t.Fatal("invalid enum values should be rejected")
	}
}

func strPtr(value string) *string {
	return &value
}
