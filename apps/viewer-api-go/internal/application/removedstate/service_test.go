package removedstate

import (
	"bytes"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"narou-viewer/apps/viewer-api-go/internal/ai"
	"narou-viewer/apps/viewer-api-go/internal/ai/usagemigration"
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
		Feature:        "extraction",
		WorkflowName:   "Extraction",
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
	if err := os.WriteFile(filepath.Join(stateDir, "term_profiles", novelID+".yaml"), []byte("schema_version: 1\nnovel_id: "+novelID+"\nterms: []\n"), 0o644); err != nil {
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
	if err := os.WriteFile(filepath.Join(stateDir, "extraction_jobs", "job-remove.yaml"), []byte("schema_version: 2\nnovel_id: "+novelID+"\n"), 0o644); err != nil {
		t.Fatalf("write character job fixture: %v", err)
	}
	checkpointDir := filepath.Join(stateDir, "extraction_jobs", "checkpoints")
	if err := os.MkdirAll(checkpointDir, 0o755); err != nil {
		t.Fatalf("mkdir checkpoint fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(checkpointDir, "checkpoint-remove.json"), []byte(`{"schemaVersion":4,"novelId":"`+novelID+`"}`), 0o644); err != nil {
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

func TestServicePreflightsEveryNovelBeforeFirstMutation(t *testing.T) {
	dataDir := t.TempDir()
	stateDir := filepath.Join(dataDir, "state")
	stateStore := store.New(dataDir)
	if err := stateStore.Initialize(); err != nil {
		t.Fatalf("Initialize returned error: %v", err)
	}
	episodeIndex := "1"
	for _, novelID := range []string{"novel-first", "novel-future"} {
		if _, err := stateStore.PutReadingState(store.ReadingStatePutInput{ReadingState: store.ReadingState{
			NovelID:              novelID,
			LastReadEpisodeIndex: &episodeIndex,
			Position:             10,
		}}); err != nil {
			t.Fatalf("PutReadingState(%s): %v", novelID, err)
		}
	}
	if err := os.MkdirAll(filepath.Join(stateDir, "term_profiles"), 0o755); err != nil {
		t.Fatalf("mkdir term profiles: %v", err)
	}
	futurePath := filepath.Join(stateDir, "term_profiles", "novel-future.yaml")
	futureBytes := []byte("schema_version: 99\nnovel_id: novel-future\nterms: []\n")
	if err := os.WriteFile(futurePath, futureBytes, 0o644); err != nil {
		t.Fatalf("write future term profile: %v", err)
	}
	readingPath := filepath.Join(stateDir, "reading_state.yaml")
	readingBefore, err := os.ReadFile(readingPath)
	if err != nil {
		t.Fatalf("read reading state before prune: %v", err)
	}

	service := NewService(stateStore, stateDir, filepath.Join(stateDir, "ai_usage.sqlite"))
	if result, err := service.PruneRemovedNovelState([]string{"novel-first", "novel-future"}); err == nil || result != (CleanupResult{}) {
		t.Fatalf("future state should reject the operation-wide preflight: result=%+v err=%v", result, err)
	}
	readingAfter, err := os.ReadFile(readingPath)
	if err != nil {
		t.Fatalf("read reading state after prune: %v", err)
	}
	if !bytes.Equal(readingBefore, readingAfter) {
		t.Fatal("the first novel was mutated before the second novel failed preflight")
	}
	if after, err := os.ReadFile(futurePath); err != nil || !bytes.Equal(after, futureBytes) {
		t.Fatalf("future term profile changed: err=%v bytes=%q", err, after)
	}
}

func TestServiceFutureUsagePreflightPreservesAllFileState(t *testing.T) {
	dataDir := t.TempDir()
	stateDir := filepath.Join(dataDir, "state")
	stateStore := store.New(dataDir)
	if err := stateStore.Initialize(); err != nil {
		t.Fatalf("Initialize returned error: %v", err)
	}
	episodeIndex := "1"
	if _, err := stateStore.PutReadingState(store.ReadingStatePutInput{ReadingState: store.ReadingState{
		NovelID:              "novel-1",
		LastReadEpisodeIndex: &episodeIndex,
		Position:             10,
	}}); err != nil {
		t.Fatalf("PutReadingState returned error: %v", err)
	}
	usagePath := filepath.Join(stateDir, "ai_usage.sqlite")
	db, err := sql.Open("sqlite", usagePath)
	if err != nil {
		t.Fatalf("open future usage fixture: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY); INSERT INTO schema_migrations(version) VALUES (99)`); err != nil {
		t.Fatalf("seed future usage fixture: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close future usage fixture: %v", err)
	}
	readingPath := filepath.Join(stateDir, "reading_state.yaml")
	readingBefore, err := os.ReadFile(readingPath)
	if err != nil {
		t.Fatalf("read reading state before usage preflight: %v", err)
	}
	usageBefore, err := os.ReadFile(usagePath)
	if err != nil {
		t.Fatalf("read usage before preflight: %v", err)
	}

	service := NewService(stateStore, stateDir, usagePath)
	result, err := service.PruneRemovedNovelState([]string{"novel-1"})
	if !usagemigration.IsFutureSchema(err) || result != (CleanupResult{}) {
		t.Fatalf("future usage should stop prune: result=%+v err=%v", result, err)
	}
	readingAfter, _ := os.ReadFile(readingPath)
	usageAfter, _ := os.ReadFile(usagePath)
	if !bytes.Equal(readingBefore, readingAfter) || !bytes.Equal(usageBefore, usageAfter) {
		t.Fatal("future usage preflight changed file state")
	}
}

func TestServicePartialLegacyUsageSchemaFailsBeforeCorePrune(t *testing.T) {
	dataDir := t.TempDir()
	stateDir := filepath.Join(dataDir, "state")
	stateStore := store.New(dataDir)
	if err := stateStore.Initialize(); err != nil {
		t.Fatalf("Initialize returned error: %v", err)
	}
	episodeIndex := "1"
	if _, err := stateStore.PutReadingState(store.ReadingStatePutInput{ReadingState: store.ReadingState{
		NovelID:              "novel-1",
		LastReadEpisodeIndex: &episodeIndex,
		Position:             10,
	}}); err != nil {
		t.Fatalf("PutReadingState returned error: %v", err)
	}
	usagePath := filepath.Join(stateDir, "ai_usage.sqlite")
	db, err := sql.Open("sqlite", usagePath)
	if err != nil {
		t.Fatalf("open partial usage fixture: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE ai_usage_runs (run_id TEXT PRIMARY KEY)`); err != nil {
		t.Fatalf("seed partial usage fixture: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close partial usage fixture: %v", err)
	}
	readingPath := filepath.Join(stateDir, "reading_state.yaml")
	before, err := os.ReadFile(readingPath)
	if err != nil {
		t.Fatalf("read reading state before partial usage preflight: %v", err)
	}

	if result, err := NewService(stateStore, stateDir, usagePath).PruneRemovedNovelState([]string{"novel-1"}); err == nil || result != (CleanupResult{}) {
		t.Fatalf("partial usage schema should stop prune: result=%+v err=%v", result, err)
	}
	after, err := os.ReadFile(readingPath)
	if err != nil {
		t.Fatalf("read reading state after partial usage preflight: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("partial usage schema was detected after core state changed")
	}
}
