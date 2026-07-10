package store

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"narou-viewer/apps/viewer-api-go/internal/characters"
	extractdomain "narou-viewer/apps/viewer-api-go/internal/extraction"
)

func TestStorePersistsReaderStatePreferencesAndBookmarks(t *testing.T) {
	dataDir := t.TempDir()
	store := New(dataDir)
	if err := store.Initialize(); err != nil {
		t.Fatalf("Initialize returned error: %v", err)
	}

	missingState, err := store.GetReadingState("novel-1")
	if err != nil {
		t.Fatalf("GetReadingState returned error: %v", err)
	}
	if missingState.NovelID != "novel-1" || missingState.Position != 0 || missingState.StateVersion != 0 {
		t.Fatalf("unexpected missing state: %+v", missingState)
	}

	episodeIndex := "3"
	clientID := "client-1"
	state, err := store.PutReadingState(ReadingStatePutInput{
		ReadingState: ReadingState{
			NovelID:              "novel-1",
			LastReadEpisodeIndex: &episodeIndex,
			Position:             42,
			Scroll:               &ScrollState{Type: "ratio", Value: 0.75},
			UpdatedByClientID:    &clientID,
		},
	})
	if err != nil {
		t.Fatalf("PutReadingState returned error: %v", err)
	}
	if state.LastReadEpisodeIndex == nil || *state.LastReadEpisodeIndex != episodeIndex || state.Position != 42 || state.StateVersion != 1 {
		t.Fatalf("unexpected stored state: %+v", state)
	}
	staleVersion := state.StateVersion - 1
	conflictState, err := store.PutReadingState(ReadingStatePutInput{
		ReadingState: ReadingState{
			NovelID:              "novel-1",
			LastReadEpisodeIndex: &episodeIndex,
			Position:             99,
		},
		ExpectedStateVersion: &staleVersion,
	})
	if !errors.Is(err, ErrReadingStateVersionConflict) {
		t.Fatalf("expected reading state version conflict, got state=%+v err=%v", conflictState, err)
	}
	if conflictState.Position != 42 || conflictState.StateVersion != state.StateVersion {
		t.Fatalf("conflict should return current state without writing: %+v", conflictState)
	}
	clearedState, err := store.PutReadingState(ReadingStatePutInput{ReadingState: ReadingState{NovelID: "novel-1", LastReadEpisodeIndex: nil, Position: 99}})
	if err != nil {
		t.Fatalf("PutReadingState clear returned error: %v", err)
	}
	if clearedState.Position != 0 || clearedState.StateVersion != 2 {
		t.Fatalf("cleared state should reset position and increment version: %+v", clearedState)
	}
	reloadedState, err := New(dataDir).GetReadingState("novel-1")
	if err != nil {
		t.Fatalf("reloaded GetReadingState returned error: %v", err)
	}
	if reloadedState.Position != 0 || reloadedState.StateVersion != 2 {
		t.Fatalf("state was not persisted: %+v", reloadedState)
	}

	preferences, err := store.GetReaderPreferences()
	if err != nil {
		t.Fatalf("GetReaderPreferences returned error: %v", err)
	}
	if preferences.ReadingMode != DefaultReadingMode || preferences.FontFamily != DefaultReaderFontFamily || preferences.Theme != DefaultReaderTheme {
		t.Fatalf("unexpected default preferences: %+v", preferences)
	}
	updatedPreferences, err := store.PutReaderPreferences(ReaderPreferences{
		ReadingMode: "horizontal",
		FontFamily:  "gothic",
		Theme:       "midnight",
	})
	if err != nil {
		t.Fatalf("PutReaderPreferences returned error: %v", err)
	}
	if updatedPreferences.ReadingMode != "horizontal" || updatedPreferences.FontFamily != "gothic" || updatedPreferences.Theme != "midnight" || updatedPreferences.UpdatedAt == nil {
		t.Fatalf("unexpected updated preferences: %+v", updatedPreferences)
	}
	defaultNovelSettings, err := store.GetNovelReaderSettings("novel-1")
	if err != nil {
		t.Fatalf("GetNovelReaderSettings returned error: %v", err)
	}
	if defaultNovelSettings.NovelID != "novel-1" || !defaultNovelSettings.Correction.QuoteNormalization || !defaultNovelSettings.Correction.HyphenDashNormalization || !defaultNovelSettings.Correction.ParenthesisNormalization || !defaultNovelSettings.Correction.HalfwidthAlnumPunctuationNormalization || defaultNovelSettings.UpdatedAt != nil {
		t.Fatalf("unexpected default novel reader settings: %+v", defaultNovelSettings)
	}
	updatedNovelSettings, err := store.PutNovelReaderSettings(NovelReaderSettings{
		NovelID: "novel-1",
		Correction: NovelReaderCorrection{
			QuoteNormalization:                     true,
			HyphenDashNormalization:                true,
			ParenthesisNormalization:               true,
			HalfwidthAlnumPunctuationNormalization: true,
		},
	})
	if err != nil {
		t.Fatalf("PutNovelReaderSettings returned error: %v", err)
	}
	if !updatedNovelSettings.Correction.QuoteNormalization || !updatedNovelSettings.Correction.HyphenDashNormalization || !updatedNovelSettings.Correction.ParenthesisNormalization || !updatedNovelSettings.Correction.HalfwidthAlnumPunctuationNormalization || updatedNovelSettings.UpdatedAt == nil {
		t.Fatalf("unexpected updated novel reader settings: %+v", updatedNovelSettings)
	}
	reloadedNovelSettings, err := New(dataDir).GetNovelReaderSettings("novel-1")
	if err != nil {
		t.Fatalf("reloaded GetNovelReaderSettings returned error: %v", err)
	}
	if !reloadedNovelSettings.Correction.QuoteNormalization || !reloadedNovelSettings.Correction.HyphenDashNormalization || !reloadedNovelSettings.Correction.ParenthesisNormalization || !reloadedNovelSettings.Correction.HalfwidthAlnumPunctuationNormalization {
		t.Fatalf("novel reader settings were not persisted: %+v", reloadedNovelSettings)
	}
	falseValue := false
	patchedNovelSettings, err := store.PatchNovelReaderSettings("novel-1", NovelReaderCorrectionPatch{
		QuoteNormalization: &falseValue,
	})
	if err != nil {
		t.Fatalf("PatchNovelReaderSettings returned error: %v", err)
	}
	if patchedNovelSettings.Correction.QuoteNormalization || !patchedNovelSettings.Correction.HyphenDashNormalization || !patchedNovelSettings.Correction.ParenthesisNormalization || !patchedNovelSettings.Correction.HalfwidthAlnumPunctuationNormalization {
		t.Fatalf("partial patch should update one field and preserve omitted fields: %+v", patchedNovelSettings)
	}
	reloadedPatchedNovelSettings, err := New(dataDir).GetNovelReaderSettings("novel-1")
	if err != nil {
		t.Fatalf("reloaded patched GetNovelReaderSettings returned error: %v", err)
	}
	if reloadedPatchedNovelSettings.Correction.QuoteNormalization || !reloadedPatchedNovelSettings.Correction.HyphenDashNormalization || !reloadedPatchedNovelSettings.Correction.ParenthesisNormalization || !reloadedPatchedNovelSettings.Correction.HalfwidthAlnumPunctuationNormalization {
		t.Fatalf("explicit false should persist across store reload: %+v", reloadedPatchedNovelSettings)
	}
	patchedUpdatedAt := reloadedPatchedNovelSettings.UpdatedAt
	emptyPatchedNovelSettings, err := store.PatchNovelReaderSettings("novel-1", NovelReaderCorrectionPatch{})
	if err != nil {
		t.Fatalf("empty PatchNovelReaderSettings returned error: %v", err)
	}
	if emptyPatchedNovelSettings.UpdatedAt == nil || patchedUpdatedAt == nil || *emptyPatchedNovelSettings.UpdatedAt != *patchedUpdatedAt {
		t.Fatalf("empty patch should preserve timestamp without writing: before=%v after=%v", patchedUpdatedAt, emptyPatchedNovelSettings.UpdatedAt)
	}
	patchedHalfwidthSettings, err := store.PatchNovelReaderSettings("novel-1", NovelReaderCorrectionPatch{
		HalfwidthAlnumPunctuationNormalization: &falseValue,
	})
	if err != nil {
		t.Fatalf("PatchNovelReaderSettings halfwidth returned error: %v", err)
	}
	if patchedHalfwidthSettings.Correction.HalfwidthAlnumPunctuationNormalization {
		t.Fatalf("halfwidth explicit false should be returned: %+v", patchedHalfwidthSettings)
	}
	reloadedHalfwidthSettings, err := New(dataDir).GetNovelReaderSettings("novel-1")
	if err != nil {
		t.Fatalf("reloaded halfwidth GetNovelReaderSettings returned error: %v", err)
	}
	if reloadedHalfwidthSettings.Correction.HalfwidthAlnumPunctuationNormalization {
		t.Fatalf("halfwidth explicit false should persist across store reload: %+v", reloadedHalfwidthSettings)
	}

	label := "bookmark"
	bookmark, err := store.CreateBookmark(Bookmark{
		NovelID:      "novel-1",
		EpisodeIndex: "3",
		Position:     7,
		Label:        &label,
	})
	if err != nil {
		t.Fatalf("CreateBookmark returned error: %v", err)
	}
	if bookmark.ID == "" || bookmark.Label == nil || *bookmark.Label != label {
		t.Fatalf("unexpected bookmark: %+v", bookmark)
	}
	bookmarks, err := store.ListBookmarks("novel-1")
	if err != nil {
		t.Fatalf("ListBookmarks returned error: %v", err)
	}
	if len(bookmarks) != 1 || bookmarks[0].ID != bookmark.ID {
		t.Fatalf("unexpected bookmark list: %+v", bookmarks)
	}
	if err := store.DeleteBookmark(bookmark.ID); err != nil {
		t.Fatalf("DeleteBookmark returned error: %v", err)
	}
	if err := store.DeleteBookmark(bookmark.ID); err != ErrBookmarkNotFound {
		t.Fatalf("expected ErrBookmarkNotFound, got %v", err)
	}
	sentinelErr := errors.New("sentinel")
	cryptoErr := &AIGenerationSettingsCryptoError{Message: "crypto failed", Err: sentinelErr}
	if !errors.Is(cryptoErr, sentinelErr) || !strings.Contains(cryptoErr.Error(), "crypto failed") {
		t.Fatalf("crypto error should unwrap and include message: %v", cryptoErr)
	}
}

func TestStorePruneNovelStateDeletesReaderStateAndBookmarks(t *testing.T) {
	dataDir := t.TempDir()
	store := New(dataDir)
	if err := store.Initialize(); err != nil {
		t.Fatalf("Initialize returned error: %v", err)
	}
	episodeIndex := "1"
	if _, err := store.PutReadingState(ReadingStatePutInput{ReadingState: ReadingState{NovelID: "novel-1", LastReadEpisodeIndex: &episodeIndex, Position: 10}}); err != nil {
		t.Fatalf("PutReadingState target returned error: %v", err)
	}
	if _, err := store.PutReadingState(ReadingStatePutInput{ReadingState: ReadingState{NovelID: "novel-2", LastReadEpisodeIndex: &episodeIndex, Position: 20}}); err != nil {
		t.Fatalf("PutReadingState other returned error: %v", err)
	}
	if _, err := store.CreateBookmark(Bookmark{NovelID: "novel-1", EpisodeIndex: "1", Position: 1}); err != nil {
		t.Fatalf("CreateBookmark target returned error: %v", err)
	}
	if _, err := store.CreateBookmark(Bookmark{NovelID: "novel-1", EpisodeIndex: "2", Position: 2}); err != nil {
		t.Fatalf("CreateBookmark second target returned error: %v", err)
	}
	if _, err := store.CreateBookmark(Bookmark{NovelID: "novel-2", EpisodeIndex: "1", Position: 3}); err != nil {
		t.Fatalf("CreateBookmark other returned error: %v", err)
	}
	if _, err := store.PutNovelReaderSettings(NovelReaderSettings{
		NovelID: "novel-1",
		Correction: NovelReaderCorrection{
			QuoteNormalization:                     true,
			HyphenDashNormalization:                true,
			ParenthesisNormalization:               true,
			HalfwidthAlnumPunctuationNormalization: true,
		},
	}); err != nil {
		t.Fatalf("PutNovelReaderSettings target returned error: %v", err)
	}
	if _, err := store.PutNovelReaderSettings(NovelReaderSettings{
		NovelID: "novel-2",
		Correction: NovelReaderCorrection{
			QuoteNormalization:                     true,
			HyphenDashNormalization:                true,
			ParenthesisNormalization:               true,
			HalfwidthAlnumPunctuationNormalization: true,
		},
	}); err != nil {
		t.Fatalf("PutNovelReaderSettings other returned error: %v", err)
	}

	result, err := store.PruneNovelState(" novel-1 ")
	if err != nil {
		t.Fatalf("PruneNovelState returned error: %v", err)
	}
	if !result.ReadingStateDeleted || result.BookmarksDeleted != 2 {
		t.Fatalf("unexpected prune result: %+v", result)
	}
	state, err := store.GetReadingState("novel-1")
	if err != nil {
		t.Fatalf("GetReadingState target returned error: %v", err)
	}
	if state.Position != 0 || state.LastReadEpisodeIndex != nil || state.StateVersion != 2 {
		t.Fatalf("target reader state should be tombstoned with a bumped version: %+v", state)
	}
	staleVersion := 1
	conflictState, err := store.PutReadingState(ReadingStatePutInput{
		ReadingState:         ReadingState{NovelID: "novel-1", LastReadEpisodeIndex: &episodeIndex, Position: 99},
		ExpectedStateVersion: &staleVersion,
	})
	if !errors.Is(err, ErrReadingStateVersionConflict) {
		t.Fatalf("expected tombstone version conflict, got state=%+v err=%v", conflictState, err)
	}
	if conflictState.Position != 0 || conflictState.LastReadEpisodeIndex != nil || conflictState.StateVersion != 2 {
		t.Fatalf("stale write should not revive pruned reader state: %+v", conflictState)
	}
	result, err = store.PruneNovelState("novel-1")
	if err != nil || result.ReadingStateDeleted || result.BookmarksDeleted != 0 {
		t.Fatalf("repeated prune should only advance the reader state tombstone: result=%+v err=%v", result, err)
	}
	repeatedState, err := store.GetReadingState("novel-1")
	if err != nil {
		t.Fatalf("GetReadingState repeated tombstone returned error: %v", err)
	}
	if repeatedState.Position != 0 || repeatedState.LastReadEpisodeIndex != nil || repeatedState.StateVersion != 3 {
		t.Fatalf("repeated prune should advance tombstone version: %+v", repeatedState)
	}
	reacquiredVersion := 2
	reacquiredConflictState, err := store.PutReadingState(ReadingStatePutInput{
		ReadingState:         ReadingState{NovelID: "novel-1", LastReadEpisodeIndex: &episodeIndex, Position: 123},
		ExpectedStateVersion: &reacquiredVersion,
	})
	if !errors.Is(err, ErrReadingStateVersionConflict) {
		t.Fatalf("expected stale reacquired write to conflict, got state=%+v err=%v", reacquiredConflictState, err)
	}
	if reacquiredConflictState.Position != 0 || reacquiredConflictState.LastReadEpisodeIndex != nil || reacquiredConflictState.StateVersion != 3 {
		t.Fatalf("stale reacquired write should not revive repeated tombstone: %+v", reacquiredConflictState)
	}
	otherState, err := store.GetReadingState("novel-2")
	if err != nil {
		t.Fatalf("GetReadingState other returned error: %v", err)
	}
	if otherState.Position != 20 {
		t.Fatalf("other reader state should remain: %+v", otherState)
	}
	if bookmarks, err := store.ListBookmarks("novel-1"); err != nil || len(bookmarks) != 0 {
		t.Fatalf("target bookmarks should be removed: bookmarks=%+v err=%v", bookmarks, err)
	}
	if bookmarks, err := store.ListBookmarks("novel-2"); err != nil || len(bookmarks) != 1 {
		t.Fatalf("other bookmarks should remain: bookmarks=%+v err=%v", bookmarks, err)
	}
	targetSettings, err := store.GetNovelReaderSettings("novel-1")
	if err != nil {
		t.Fatalf("GetNovelReaderSettings target returned error: %v", err)
	}
	if !targetSettings.Correction.QuoteNormalization || !targetSettings.Correction.HyphenDashNormalization || !targetSettings.Correction.ParenthesisNormalization || !targetSettings.Correction.HalfwidthAlnumPunctuationNormalization || targetSettings.UpdatedAt != nil {
		t.Fatalf("target reader settings should be reset: %+v", targetSettings)
	}
	if _, err := store.PatchNovelReaderSettings("novel-1", NovelReaderCorrectionPatch{QuoteNormalization: boolPtr(false)}); !errors.Is(err, ErrNovelStateDeleted) {
		t.Fatalf("pruned novel reader settings patch should be rejected, got %v", err)
	}
	if _, err := store.CreateBookmark(Bookmark{NovelID: "novel-1", EpisodeIndex: "1", Position: 4}); !errors.Is(err, ErrNovelStateDeleted) {
		t.Fatalf("pruned novel bookmark creation should be rejected, got %v", err)
	}
	otherSettings, err := store.GetNovelReaderSettings("novel-2")
	if err != nil {
		t.Fatalf("GetNovelReaderSettings other returned error: %v", err)
	}
	if !otherSettings.Correction.QuoteNormalization || !otherSettings.Correction.HyphenDashNormalization || !otherSettings.Correction.ParenthesisNormalization || !otherSettings.Correction.HalfwidthAlnumPunctuationNormalization || otherSettings.UpdatedAt == nil {
		t.Fatalf("other reader settings should remain: %+v", otherSettings)
	}

	noop, err := store.PruneNovelState(" ")
	if err != nil || noop.ReadingStateDeleted || noop.BookmarksDeleted != 0 {
		t.Fatalf("blank prune should be a no-op: result=%+v err=%v", noop, err)
	}
	missing, err := store.PruneNovelState("missing")
	if err != nil || missing.ReadingStateDeleted || missing.BookmarksDeleted != 0 {
		t.Fatalf("missing prune should only leave a reader state tombstone: result=%+v err=%v", missing, err)
	}
	missingState, err := store.GetReadingState("missing")
	if err != nil {
		t.Fatalf("GetReadingState missing tombstone returned error: %v", err)
	}
	if missingState.StateVersion != 1 || missingState.Position != 0 || missingState.LastReadEpisodeIndex != nil {
		t.Fatalf("missing prune should create a versioned tombstone: %+v", missingState)
	}
	initialVersion := 0
	revivedState, err := store.PutReadingState(ReadingStatePutInput{
		ReadingState:         ReadingState{NovelID: "missing", LastReadEpisodeIndex: &episodeIndex, Position: 10},
		ExpectedStateVersion: &initialVersion,
	})
	if !errors.Is(err, ErrReadingStateVersionConflict) {
		t.Fatalf("expected stale version 0 write to conflict against tombstone, state=%+v err=%v", revivedState, err)
	}
	if revivedState.StateVersion != 1 || revivedState.Position != 0 || revivedState.LastReadEpisodeIndex != nil {
		t.Fatalf("stale version 0 write should not revive missing tombstone: %+v", revivedState)
	}
}

func TestStoreNovelReaderSettingsDocumentFallbacks(t *testing.T) {
	dataDir := t.TempDir()
	store := New(dataDir)

	settings, err := store.GetNovelReaderSettings("novel-missing-file")
	if err != nil {
		t.Fatalf("GetNovelReaderSettings without state file returned error: %v", err)
	}
	if settings.NovelID != "novel-missing-file" || !settings.Correction.QuoteNormalization || !settings.Correction.HyphenDashNormalization || !settings.Correction.ParenthesisNormalization || !settings.Correction.HalfwidthAlnumPunctuationNormalization || settings.UpdatedAt != nil {
		t.Fatalf("missing settings file should return defaults: %+v", settings)
	}

	stateDir := filepath.Join(dataDir, "state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, novelReaderSettingsFile), []byte("novels: ["), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	if _, err := store.GetNovelReaderSettings("novel-corrupt"); err == nil {
		t.Fatal("GetNovelReaderSettings should fail for corrupt yaml")
	}
	if _, err := store.PutNovelReaderSettings(NovelReaderSettings{NovelID: "novel-corrupt"}); err == nil {
		t.Fatal("PutNovelReaderSettings should fail for corrupt yaml")
	}
	if _, err := store.PatchNovelReaderSettings("novel-corrupt", NovelReaderCorrectionPatch{}); err == nil {
		t.Fatal("PatchNovelReaderSettings should fail for corrupt yaml")
	}

}

func TestStoreReadsStateCompatibilityFixtures(t *testing.T) {
	t.Setenv("AI_GENERATION_SETTINGS_MASTER_PASSPHRASE", "phase1-state-fixture-passphrase")
	dataDir := copyStateCompatibilityFixture(t, "normal-reader-state")
	store := New(dataDir)
	if err := store.Initialize(); err != nil {
		t.Fatalf("Initialize returned error: %v", err)
	}

	state, err := store.GetReadingState("novel-normal")
	if err != nil {
		t.Fatalf("GetReadingState returned error: %v", err)
	}
	if state.LastReadEpisodeIndex == nil || *state.LastReadEpisodeIndex != "12" || state.Position != 345 {
		t.Fatalf("unexpected reader state: %+v", state)
	}
	if state.Scroll == nil || state.Scroll.Type != "ratio" || state.Scroll.Value != 0.42 {
		t.Fatalf("unexpected scroll state: %+v", state.Scroll)
	}
	if state.StateVersion != 7 || state.UpdatedByClientID == nil || *state.UpdatedByClientID != "phase1-fixture-client" {
		t.Fatalf("unexpected reader state metadata: %+v", state)
	}

	missingOptionals, err := store.GetReadingState("novel-missing-optionals")
	if err != nil {
		t.Fatalf("GetReadingState missing optionals returned error: %v", err)
	}
	if missingOptionals.LastReadEpisodeIndex != nil || missingOptionals.Position != 0 || missingOptionals.StateVersion != 0 {
		t.Fatalf("missing optionals were not normalized to defaults: %+v", missingOptionals)
	}

	bookmarks, err := store.ListBookmarks("novel-normal")
	if err != nil {
		t.Fatalf("ListBookmarks returned error: %v", err)
	}
	if len(bookmarks) != 2 {
		t.Fatalf("expected 2 bookmarks, got %+v", bookmarks)
	}
	if bookmarks[0].ID != "bm_phase1_first" || bookmarks[0].EpisodeIndex != "12" || bookmarks[0].Position != 345 || bookmarks[0].Label == nil || *bookmarks[0].Label != "重要な場面" {
		t.Fatalf("unexpected first bookmark: %+v", bookmarks[0])
	}
	if bookmarks[1].ID != "bm_phase1_second" || bookmarks[1].EpisodeIndex != "9" || bookmarks[1].Position != 10 || bookmarks[1].Label != nil {
		t.Fatalf("unexpected second bookmark: %+v", bookmarks[1])
	}

	preferences, err := store.GetReaderPreferences()
	if err != nil {
		t.Fatalf("GetReaderPreferences returned error: %v", err)
	}
	if preferences.ReadingMode != "horizontal" || preferences.FontFamily != "gothic" || preferences.Theme != "ocean" || preferences.UpdatedAt == nil || *preferences.UpdatedAt != "2026-01-02T03:06:00.000Z" {
		t.Fatalf("unexpected reader preferences: %+v", preferences)
	}

	settings, err := store.GetAIGenerationSettings()
	if err != nil {
		t.Fatalf("GetAIGenerationSettings returned error: %v", err)
	}
	if settings.PreferredMode != "llm" || settings.Settings.SelectedProfileID == nil || *settings.Settings.SelectedProfileID != "custom-profile" {
		t.Fatalf("unexpected AI settings selection: %+v", settings)
	}
	if !settings.Settings.SharedProviders.OpenRouter.HasAPIKey || settings.Settings.SharedProviders.OpenRouter.APIKeyMasked == nil {
		t.Fatalf("shared OpenRouter key was not recognized: %+v", settings.Settings.SharedProviders.OpenRouter)
	}
	var customProfileFound bool
	for _, profile := range settings.Settings.Profiles {
		if profile.ID != "custom-profile" {
			continue
		}
		customProfileFound = true
		if profile.Credentials.Source != "custom" || !profile.Credentials.HasAPIKey || profile.ModelID == nil || *profile.ModelID != "anthropic/claude-sonnet-4" || !profile.AllowFallbacks || profile.RequireParameters {
			t.Fatalf("unexpected custom profile: %+v", profile)
		}
	}
	if !customProfileFound {
		t.Fatalf("custom profile was not loaded: %+v", settings.Settings.Profiles)
	}
	resolved, err := store.ResolveActiveAIGenerationConfig()
	if err != nil {
		t.Fatalf("ResolveActiveAIGenerationConfig returned error: %v", err)
	}
	if resolved == nil || resolved.ProfileID != "custom-profile" || resolved.APIKey != "dummy-openrouter-phase1-custom" || resolved.ModelID != "anthropic/claude-sonnet-4" {
		t.Fatalf("unexpected resolved AI config: %+v", resolved)
	}

	if _, err := store.PutReaderPreferences(ReaderPreferences{Theme: "forest"}); err != nil {
		t.Fatalf("PutReaderPreferences returned error: %v", err)
	}
	reloadedState, err := store.GetReadingState("novel-normal")
	if err != nil {
		t.Fatalf("GetReadingState after write returned error: %v", err)
	}
	if reloadedState.LastReadEpisodeIndex == nil || *reloadedState.LastReadEpisodeIndex != "12" || reloadedState.Position != 345 {
		t.Fatalf("reader state was not preserved after preferences write: %+v", reloadedState)
	}
	reloadedBookmarks, err := store.ListBookmarks("novel-normal")
	if err != nil {
		t.Fatalf("ListBookmarks after write returned error: %v", err)
	}
	if len(reloadedBookmarks) != 2 {
		t.Fatalf("bookmarks were not preserved after preferences write: %+v", reloadedBookmarks)
	}
}

func TestStoreReadsCharacterCompatibilityFixtures(t *testing.T) {
	dataDir := copyStateCompatibilityFixture(t, "character-profiles-generated")
	store := New(dataDir)
	if err := store.Initialize(); err != nil {
		t.Fatalf("Initialize returned error: %v", err)
	}
	stateDir := filepath.Join(dataDir, "state")

	summary, ok, err := characters.LoadSummary(stateDir, "novel-character", "3")
	if err != nil {
		t.Fatalf("LoadSummary returned error: %v", err)
	}
	if !ok || summary.ProcessedUpToEpisodeIndex == nil || *summary.ProcessedUpToEpisodeIndex != "3" || len(summary.Characters) != 1 {
		t.Fatalf("unexpected character summary: ok=%v summary=%+v", ok, summary)
	}
	character := summary.Characters[0]
	if character.CharacterID != "char_alice" || character.CanonicalName != "アリス" || character.FullName == nil || *character.FullName != "アリス・スミス" || character.Summary == nil || *character.Summary != "アリス・スミスは主人公の仲間だ。" {
		t.Fatalf("unexpected character profile: %+v", character)
	}

	jobs, ok, err := extractdomain.LoadJobs(stateDir, "novel-character")
	if err != nil {
		t.Fatalf("LoadJobs returned error: %v", err)
	}
	if !ok || len(jobs) != 2 {
		t.Fatalf("unexpected character jobs: ok=%v jobs=%+v", ok, jobs)
	}
	byID := map[string]extractdomain.Job{}
	for _, job := range jobs {
		byID[job.JobID] = job
	}
	completed := byID["charjob_completed"]
	if completed.Status != "completed" || completed.RequestedUpToEpisodeIndex != "3" {
		t.Fatalf("unexpected completed job: %+v", completed)
	}
	failed := byID["charjob_failed"]
	if failed.Status != "failed" || failed.ErrorMessage == nil || *failed.ErrorMessage != "provider failed" {
		t.Fatalf("unexpected failed job: %+v", failed)
	}

	if err := characters.SaveGeneratedSummary(stateDir, "novel-character", "4", nil); err != nil {
		t.Fatalf("SaveGeneratedSummary returned error: %v", err)
	}
	reloadedJobs, ok, err := extractdomain.LoadJobs(stateDir, "novel-character")
	if err != nil {
		t.Fatalf("LoadJobs after summary write returned error: %v", err)
	}
	if !ok || len(reloadedJobs) != 2 {
		t.Fatalf("character jobs were not preserved after summary write: ok=%v jobs=%+v", ok, reloadedJobs)
	}
}

func copyStateCompatibilityFixture(t *testing.T, name string) string {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", "..", ".."))
	sourceDir := filepath.Join(repoRoot, "tests", "fixtures", "state", name)
	targetDir := t.TempDir()
	if err := copyDir(sourceDir, targetDir); err != nil {
		t.Fatalf("copy fixture %s: %v", name, err)
	}
	return targetDir
}

func copyDir(sourceDir string, targetDir string) error {
	return filepath.WalkDir(sourceDir, func(sourcePath string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relativePath, err := filepath.Rel(sourceDir, sourcePath)
		if err != nil {
			return err
		}
		targetPath := filepath.Join(targetDir, relativePath)
		if entry.IsDir() {
			return os.MkdirAll(targetPath, 0o755)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		content, err := os.ReadFile(sourcePath)
		if err != nil {
			return err
		}
		return os.WriteFile(targetPath, content, info.Mode().Perm())
	})
}

func TestNormalizers(t *testing.T) {
	if IsEpisodeIndex("") {
		t.Fatal("blank episode index should be rejected")
	}
	if value, ok := NormalizeEpisodeIndex(float64(12)); !ok || value != "12" {
		t.Fatalf("unexpected episode index normalization: %q %v", value, ok)
	}
	if value, ok := NormalizeEpisodeIndex("12"); !ok || value != "12" {
		t.Fatalf("string episode index should be accepted: %q %v", value, ok)
	}
	if _, ok := NormalizeEpisodeIndex("bad"); ok {
		t.Fatal("invalid episode index should be rejected")
	}
	if _, ok := NormalizeEpisodeIndex(nil); ok {
		t.Fatal("nil episode index should be rejected")
	}
	if value, ok := NormalizePosition("9"); !ok || value != 9 {
		t.Fatalf("unexpected position normalization: %d %v", value, ok)
	}
	if value, ok := NormalizePosition(float64(9)); !ok || value != 9 {
		t.Fatalf("float position should be accepted: %d %v", value, ok)
	}
	if _, ok := NormalizePosition(float64(-1)); ok {
		t.Fatal("negative position should be rejected")
	}
	if _, ok := NormalizePosition("bad"); ok {
		t.Fatal("bad string position should be rejected")
	}
	if clientID, ok := NormalizeClientID(" client "); !ok || clientID == nil || *clientID != "client" {
		t.Fatalf("unexpected client id normalization: %v %v", clientID, ok)
	}
	if clientID, ok := NormalizeClientID(nil); !ok || clientID != nil {
		t.Fatalf("nil client id should be accepted as nil: %v %v", clientID, ok)
	}
	if scroll, ok := NormalizeScrollState(map[string]any{"type": "ratio", "value": 2.0}); !ok || scroll == nil || scroll.Value != 1 {
		t.Fatalf("unexpected scroll normalization: %+v %v", scroll, ok)
	}
	if scroll, ok := NormalizeScrollState(map[string]any{"type": "ratio", "value": -1.0}); !ok || scroll == nil || scroll.Value != 0 {
		t.Fatalf("negative scroll should clamp to zero: %+v %v", scroll, ok)
	}
	if scroll, ok := NormalizeScrollState(nil); !ok || scroll != nil {
		t.Fatalf("nil scroll should be accepted as nil: %+v %v", scroll, ok)
	}
	if _, ok := NormalizeScrollState("bad"); ok {
		t.Fatal("non-map scroll should be rejected")
	}
	if _, ok := NormalizeScrollState(map[string]any{"type": "line", "value": 0.5}); ok {
		t.Fatal("invalid scroll type should be rejected")
	}
	if _, ok := NormalizeClientID(" "); ok {
		t.Fatal("blank client id should be rejected")
	}
	if _, ok := NormalizeClientID(float64(1)); ok {
		t.Fatal("non-string client id should be rejected")
	}
	if _, ok := NormalizeScrollState(map[string]any{"type": "ratio", "value": "bad"}); ok {
		t.Fatal("non-number scroll value should be rejected")
	}
	if !IsReadingMode("vertical") || !IsReaderFontFamily("mincho") || !IsReaderTheme("forest") {
		t.Fatal("expected default enum values to be accepted")
	}
	if IsReadingMode("bad") || IsReaderFontFamily("bad") || IsReaderTheme("bad") {
		t.Fatal("invalid enum values should be rejected")
	}
	if normalizeLabelPtr(strPtr(" ")) != nil || stringPtrOrNil(nil) != nil {
		t.Fatal("nil and blank pointer helpers should return nil")
	}
	if value := normalizeLabelPtr(strPtr(" label ")); value == nil || *value != "label" {
		t.Fatalf("label ptr should trim text: %v", value)
	}
	if value := stringPtrOrNil(strPtr(" value ")); value == nil || *value != " value " {
		t.Fatalf("stringPtrOrNil should preserve non-empty value: %v", value)
	}
}

func TestCharacterStateDirInitializationFailsForBlockedManagedDirectories(t *testing.T) {
	for _, blockedName := range []string{"character_profiles", "character_events"} {
		t.Run(blockedName, func(t *testing.T) {
			dataDir := t.TempDir()
			stateDir := filepath.Join(dataDir, "state")
			blockedPath := filepath.Join(stateDir, blockedName)
			if err := os.MkdirAll(filepath.Dir(blockedPath), 0o755); err != nil {
				t.Fatalf("mkdir blocked path parent: %v", err)
			}
			if err := os.WriteFile(blockedPath, []byte("not a directory"), 0o644); err != nil {
				t.Fatalf("write blocked path: %v", err)
			}
			if err := characters.EnsureStateDirs(stateDir); err == nil {
				t.Fatal("EnsureStateDirs should fail when a managed directory is blocked by a file")
			}
		})
	}
	t.Run("extraction_jobs/index", func(t *testing.T) {
		stateDir := t.TempDir()
		blockedPath := filepath.Join(stateDir, "extraction_jobs", "index")
		if err := os.MkdirAll(filepath.Dir(blockedPath), 0o755); err != nil {
			t.Fatalf("mkdir blocked path parent: %v", err)
		}
		if err := os.WriteFile(blockedPath, []byte("not a directory"), 0o644); err != nil {
			t.Fatalf("write blocked path: %v", err)
		}
		if err := extractdomain.EnsureStateDirs(stateDir); err == nil {
			t.Fatal("extraction EnsureStateDirs should fail when a managed directory is blocked")
		}
	})
}

func TestStoreHandlesMissingAndCorruptDocuments(t *testing.T) {
	dataDir := t.TempDir()
	store := New(dataDir)

	if state, err := store.GetReadingState("novel"); err != nil || state.NovelID != "novel" {
		t.Fatalf("missing reading state should return default, state=%+v err=%v", state, err)
	}
	if bookmarks, err := store.ListBookmarks(""); err != nil || len(bookmarks) != 0 {
		t.Fatalf("missing bookmarks should return empty list, bookmarks=%+v err=%v", bookmarks, err)
	}
	if preferences, err := store.GetReaderPreferences(); err != nil || preferences.ReadingMode != DefaultReadingMode {
		t.Fatalf("missing preferences should return defaults, preferences=%+v err=%v", preferences, err)
	}

	if err := store.Initialize(); err != nil {
		t.Fatalf("Initialize returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "state", readingStateFile), []byte("novels: ["), 0o644); err != nil {
		t.Fatalf("write corrupt reading state: %v", err)
	}
	if _, err := store.GetReadingState("novel"); err == nil {
		t.Fatal("corrupt reading state should return error")
	}

	if err := store.Initialize(); err != nil {
		t.Fatalf("reinitialize after corrupt reading state returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "state", bookmarksFile), []byte("bookmarks: ["), 0o644); err != nil {
		t.Fatalf("write corrupt bookmarks: %v", err)
	}
	if _, err := store.ListBookmarks(""); err == nil {
		t.Fatal("corrupt bookmarks should return error")
	}

	if err := store.Initialize(); err != nil {
		t.Fatalf("reinitialize after corrupt bookmarks returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "state", readerPreferencesFile), []byte("reader: ["), 0o644); err != nil {
		t.Fatalf("write corrupt preferences: %v", err)
	}
	if _, err := store.GetReaderPreferences(); err == nil {
		t.Fatal("corrupt preferences should return error")
	}

	if err := os.WriteFile(filepath.Join(dataDir, "state", "ai_generation_settings.yaml"), []byte("profiles: ["), 0o644); err != nil {
		t.Fatalf("write corrupt AI settings: %v", err)
	}
	if _, err := store.GetAIGenerationSettings(); err == nil {
		t.Fatal("corrupt AI settings should return error")
	}
}

func strPtr(value string) *string {
	return &value
}
