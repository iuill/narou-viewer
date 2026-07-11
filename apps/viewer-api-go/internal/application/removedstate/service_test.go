package removedstate

import (
	"os"
	"path/filepath"
	"testing"

	"narou-viewer/apps/viewer-api-go/internal/ai"
	"narou-viewer/apps/viewer-api-go/internal/application/readertextcache"
	"narou-viewer/apps/viewer-api-go/internal/characters"
	extractdomain "narou-viewer/apps/viewer-api-go/internal/extraction"
	"narou-viewer/apps/viewer-api-go/internal/publications"
	"narou-viewer/apps/viewer-api-go/internal/store"
)

func TestServicePrunesReaderBookmarksAndUsage(t *testing.T) {
	dataDir := t.TempDir()
	stateStore := store.New(dataDir)
	if err := stateStore.Initialize(); err != nil {
		t.Fatalf("Initialize returned error: %v", err)
	}

	novelID := "novel-remove"
	episodeIndex := "1"
	if _, err := stateStore.PutReadingState(store.ReadingStatePutInput{
		ReadingState: store.ReadingState{
			NovelID:              novelID,
			LastReadEpisodeIndex: &episodeIndex,
			Position:             42,
		},
	}); err != nil {
		t.Fatalf("PutReadingState returned error: %v", err)
	}
	if _, err := stateStore.CreateBookmark(store.Bookmark{
		NovelID:      novelID,
		EpisodeIndex: episodeIndex,
		Position:     12,
	}); err != nil {
		t.Fatalf("CreateBookmark returned error: %v", err)
	}

	usageNovelID := novelID
	usageDBPath := filepath.Join(dataDir, "state", "ai_usage.sqlite")
	if err := ai.SaveUsageRun(usageDBPath, ai.UsageRun{
		RunID:          "run-remove",
		Feature:        "character-summary",
		WorkflowName:   "Character summary",
		Status:         "completed",
		StartedAt:      "2026-01-01T00:00:00Z",
		FinishedAt:     "2026-01-01T00:00:01Z",
		ElapsedMs:      1000,
		NovelID:        &usageNovelID,
		GenerationMode: "heuristic",
		RequestCount:   1,
		InputTokens:    1,
		OutputTokens:   2,
		TotalTokens:    3,
		Requests: []ai.UsageRequest{{
			RequestIndex: 1,
			Kind:         "chat",
			InputTokens:  1,
			OutputTokens: 2,
			TotalTokens:  3,
		}},
	}); err != nil {
		t.Fatalf("SaveUsageRun returned error: %v", err)
	}
	stateDir := filepath.Join(dataDir, "state")
	if err := characters.EnsureStateDirs(stateDir); err != nil {
		t.Fatalf("EnsureStateDirs returned error: %v", err)
	}
	if err := extractdomain.EnsureStateDirs(stateDir); err != nil {
		t.Fatalf("extraction EnsureStateDirs returned error: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(stateDir, "term_profiles"), 0o755); err != nil {
		t.Fatalf("mkdir term profiles: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "term_profiles", novelID+".yaml"), []byte("novel_id: "+novelID+"\nterms: []\n"), 0o644); err != nil {
		t.Fatalf("write term profile fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "character_profiles", novelID+".yaml"), []byte("novel_id: "+novelID+"\n"), 0o644); err != nil {
		t.Fatalf("write character profile fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "character_events", novelID+".yaml"), []byte("novel_id: "+novelID+"\n"), 0o644); err != nil {
		t.Fatalf("write character events fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "extraction_jobs", "index", novelID+".yaml"), []byte("job_ids:\n  - job-remove\n"), 0o644); err != nil {
		t.Fatalf("write character job index fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "extraction_jobs", "job-remove.yaml"), []byte("novel_id: "+novelID+"\n"), 0o644); err != nil {
		t.Fatalf("write character job fixture: %v", err)
	}
	checkpointDir := filepath.Join(stateDir, "extraction_jobs", "checkpoints")
	if err := os.MkdirAll(checkpointDir, 0o755); err != nil {
		t.Fatalf("mkdir checkpoint fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(checkpointDir, "checkpoint-remove.json"), []byte(`{"novelId":"`+novelID+`"}`), 0o644); err != nil {
		t.Fatalf("write checkpoint fixture: %v", err)
	}
	if _, err := publications.NewRepository(stateDir).PutEntry(novelID, publications.Entry{
		Kind:      publications.KindNovel,
		Status:    publications.EntryStatusManual,
		Override:  publications.OverrideModeISBN,
		ISBN13:    "9784040000008",
		UpdatedAt: "2026-06-28T12:00:00Z",
	}); err != nil {
		t.Fatalf("write publication fixture: %v", err)
	}
	readerSearchCache := readertextcache.New(stateDir)
	if err := readerSearchCache.Save(t.Context(), novelID, "1", "etag-1", "本文"); err != nil {
		t.Fatalf("write reader search cache fixture: %v", err)
	}
	if err := readerSearchCache.Save(t.Context(), "novel-keep", "1", "etag-1", "残す本文"); err != nil {
		t.Fatalf("write retained reader search cache fixture: %v", err)
	}

	service := NewService(stateStore, stateDir, usageDBPath)
	result, err := service.PruneRemovedNovelState([]string{novelID})
	if err != nil {
		t.Fatalf("PruneRemovedNovelState returned error: %v", err)
	}
	if result.ReadingStatesDeleted != 1 || result.BookmarksDeleted != 1 || result.AIUsageRunsDeleted != 1 {
		t.Fatalf("unexpected cleanup result: %+v", result)
	}
	if result.CharacterProfilesDeleted != 1 ||
		result.CharacterEventsDeleted != 1 ||
		result.TermProfilesDeleted != 1 ||
		result.ExtractionJobsDeleted != 1 ||
		result.ExtractionJobIndexesDeleted != 1 ||
		result.ExtractionCheckpointsDeleted != 1 {
		t.Fatalf("unexpected character cleanup result: %+v", result)
	}
	if result.PublicationEntriesDeleted != 1 {
		t.Fatalf("unexpected publication cleanup result: %+v", result)
	}
	if result.ReaderSearchCacheRowsDeleted != 1 {
		t.Fatalf("unexpected reader search cache cleanup result: %+v", result)
	}
	if state, err := stateStore.GetReadingState(novelID); err != nil || state.Position != 0 || state.LastReadEpisodeIndex != nil || state.StateVersion != 2 {
		t.Fatalf("reader state should be tombstoned: state=%+v err=%v", state, err)
	}
	if bookmarks, err := stateStore.ListBookmarks(novelID); err != nil || len(bookmarks) != 0 {
		t.Fatalf("bookmarks should be pruned: bookmarks=%+v err=%v", bookmarks, err)
	}
	if _, ok, err := ai.LoadUsageRun(usageDBPath, "run-remove"); err != nil || ok {
		t.Fatalf("usage run should be pruned: ok=%v err=%v", ok, err)
	}
	if publicationsState, err := publications.NewRepository(stateDir).Get(novelID); err != nil || len(publicationsState.Entries) != 0 {
		t.Fatalf("publication state should be pruned: state=%+v err=%v", publicationsState, err)
	}
	if entry, ok, err := readerSearchCache.Get(t.Context(), novelID, "1", "etag-1"); err != nil || ok || entry.Text != "" {
		t.Fatalf("reader search cache should be pruned: entry=%+v ok=%v err=%v", entry, ok, err)
	}
	if entry, ok, err := readerSearchCache.Get(t.Context(), "novel-keep", "1", "etag-1"); err != nil || !ok || entry.Text != "残す本文" {
		t.Fatalf("other reader search cache rows should remain: entry=%+v ok=%v err=%v", entry, ok, err)
	}
}

func TestNilServiceReturnsEmptyCleanup(t *testing.T) {
	result, err := (*Service)(nil).PruneRemovedNovelState([]string{"novel"})
	if err != nil {
		t.Fatalf("PruneRemovedNovelState returned error: %v", err)
	}
	if result != (CleanupResult{}) {
		t.Fatalf("nil service should return empty cleanup: %+v", result)
	}
}
