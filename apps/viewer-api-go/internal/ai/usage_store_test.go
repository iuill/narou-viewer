package ai

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"narou-viewer/apps/viewer-api-go/internal/ai/usagemigration"

	_ "modernc.org/sqlite"
)

func TestLoadUsageReadsSQLiteRunsAndRequests(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "ai_usage.sqlite")
	db := openTestUsageDB(t, dbPath)
	defer db.Close()
	insertUsageRun(t, db)

	usage, ok, err := LoadUsage(dbPath)
	if err != nil {
		t.Fatalf("LoadUsage returned error: %v", err)
	}
	if !ok {
		t.Fatal("LoadUsage did not find seeded usage")
	}
	if usage.Summary.RunCount != 1 || usage.Summary.RequestCount != 1 || usage.Summary.TotalTokens != 30 || usage.Summary.CachedInputTokens != 3 || usage.Summary.ReasoningOutputTokens != 4 || usage.Summary.AverageTotalTokens != 30 {
		t.Fatalf("unexpected summary: %+v", usage.Summary)
	}
	if len(usage.Runs) != 1 {
		t.Fatalf("expected one run, got %d", len(usage.Runs))
	}
	run := usage.Runs[0]
	if run.RunID != "run-1" || run.Feature != "reader-assistant" || len(run.Requests) != 1 {
		t.Fatalf("unexpected run: %+v", run)
	}
	if run.StartedAt != "2026-01-01T00:00:00Z" || run.FinishedAt != "2026-01-01T00:00:02Z" || run.ElapsedMs != 2000 || run.GenerationMode != "openrouter" || !run.HasSnapshot {
		t.Fatalf("unexpected run metadata: %+v", run)
	}
	if run.NovelID == nil || *run.NovelID != "novel-1" || run.CurrentEpisodeIndex == nil || *run.CurrentEpisodeIndex != "1" || run.ModelID == nil || *run.ModelID != "openrouter/auto" {
		t.Fatalf("unexpected nullable run metadata: %+v", run)
	}
	if got := run.Requests[0].ToolNames; len(got) != 1 || got[0] != "search_episode" {
		t.Fatalf("unexpected tool names: %+v", got)
	}
	if got := run.Requests[0].ToolSummaries; len(got) != 1 || got[0] != "search_episode" {
		t.Fatalf("unexpected tool summaries: %+v", got)
	}
	if run.Requests[0].RequestIndex != 1 || run.Requests[0].ParentRequestIndex != nil || run.Requests[0].CachedInputTokens != 3 || run.Requests[0].ReasoningOutputTokens != 4 {
		t.Fatalf("unexpected request metadata: %+v", run.Requests[0])
	}

	detail, ok, err := LoadUsageRun(dbPath, "run-1")
	if err != nil {
		t.Fatalf("LoadUsageRun returned error: %v", err)
	}
	if !ok || detail.RunID != "run-1" || len(detail.Requests) != 1 {
		t.Fatalf("unexpected detail: ok=%v detail=%+v", ok, detail)
	}
	snapshot, ok := detail.Snapshot.(map[string]any)
	if !ok || snapshot["runId"] != "run-1" {
		t.Fatalf("unexpected snapshot: %#v", detail.Snapshot)
	}
}

func TestUsageSQLiteDSNIncludesBusyTimeout(t *testing.T) {
	dsn := usageSQLiteDSN(filepath.Join("tmp", "ai_usage.sqlite"))
	if dsn != "file:tmp/ai_usage.sqlite?_pragma=busy_timeout(5000)" {
		t.Fatalf("unexpected usage sqlite dsn: %s", dsn)
	}
}

func TestLoadUsageHandlesMissingDatabase(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "missing", "ai_usage.sqlite")
	usage, ok, err := LoadUsage(dbPath)
	if err != nil || ok || len(usage.Runs) != 0 || usage.Summary.RunCount != 0 {
		t.Fatalf("missing usage database should return empty usage: ok=%v usage=%+v err=%v", ok, usage, err)
	}
	detail, ok, err := LoadUsageRun(dbPath, "missing")
	if err != nil || ok || detail.RunID != "" {
		t.Fatalf("missing usage detail database should return not found: ok=%v detail=%+v err=%v", ok, detail, err)
	}
	emptyDBPath := filepath.Join(t.TempDir(), "ai_usage.sqlite")
	if err := os.WriteFile(emptyDBPath, nil, 0o600); err != nil {
		t.Fatalf("write empty usage db: %v", err)
	}
	if usage, ok, err := LoadUsage(emptyDBPath); err != nil || ok || usage.Summary.RunCount != 0 {
		t.Fatalf("empty usage database should return empty usage: ok=%v usage=%+v err=%v", ok, usage, err)
	}
}

func TestSaveUsageRunCreatesReadableSQLiteUsage(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "state", "ai_usage.sqlite")
	novelID := "novel-1"
	novelTitle := "Novel 1"
	currentEpisodeIndex := "1"
	run := UsageRun{
		RunID:               "run-written",
		Feature:             "reader-assistant",
		WorkflowName:        "reader-ai-assistant",
		Status:              "completed",
		StartedAt:           "2026-01-01T00:00:00Z",
		FinishedAt:          "2026-01-01T00:00:01Z",
		ElapsedMs:           1000,
		NovelID:             &novelID,
		NovelTitle:          &novelTitle,
		CurrentEpisodeIndex: &currentEpisodeIndex,
		GenerationMode:      "local",
		AnswerChars:         12,
		RequestCount:        1,
		InputTokens:         3,
		OutputTokens:        4,
		TotalTokens:         7,
		Requests: []UsageRequest{
			{
				RequestIndex: 0,
				Kind:         "final_answer",
				InputTokens:  3,
				OutputTokens: 4,
				TotalTokens:  7,
			},
		},
		Snapshot: map[string]any{"mode": "local"},
	}
	if err := SaveUsageRun(dbPath, run); err != nil {
		t.Fatalf("SaveUsageRun returned error: %v", err)
	}
	info, err := os.Stat(dbPath)
	if err != nil {
		t.Fatalf("stat usage database: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("usage database should be owner-only: mode=%o", info.Mode().Perm())
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open migrated usage database: %v", err)
	}
	var migrationVersion int
	if err := db.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&migrationVersion); err != nil {
		t.Fatalf("read usage migration version: %v", err)
	}
	var journalMode string
	if err := db.QueryRow(`PRAGMA journal_mode`).Scan(&journalMode); err != nil {
		t.Fatalf("read usage journal mode: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close migrated usage database: %v", err)
	}
	if migrationVersion != usagemigration.SupportedLatestVersion || journalMode != "delete" {
		t.Fatalf("usage database contract: migration=%d journal=%q", migrationVersion, journalMode)
	}
	usage, ok, err := LoadUsage(dbPath)
	if err != nil {
		t.Fatalf("LoadUsage returned error: %v", err)
	}
	if !ok || usage.Summary.RunCount != 1 || usage.Summary.TotalTokens != 7 || len(usage.Runs) != 1 {
		t.Fatalf("unexpected saved usage: ok=%v usage=%+v", ok, usage)
	}
	detail, ok, err := LoadUsageRun(dbPath, "run-written")
	if err != nil {
		t.Fatalf("LoadUsageRun returned error: %v", err)
	}
	if !ok || detail.GenerationMode != "local" || !detail.HasSnapshot || len(detail.Requests) != 1 {
		t.Fatalf("unexpected saved detail: ok=%v detail=%+v", ok, detail)
	}
	if detail.Requests[0].ToolNames == nil || detail.Requests[0].ToolSummaries == nil {
		t.Fatalf("nil tool lists should load as empty arrays: %+v", detail.Requests[0])
	}
	run.Snapshot = nil
	if err := SaveUsageRun(dbPath, run); err != nil {
		t.Fatalf("SaveUsageRun without snapshot returned error: %v", err)
	}
	detail, ok, err = LoadUsageRun(dbPath, "run-written")
	if err != nil {
		t.Fatalf("LoadUsageRun after snapshot clear returned error: %v", err)
	}
	if !ok || detail.HasSnapshot || detail.Snapshot != nil {
		t.Fatalf("snapshot should be cleared on overwrite without snapshot: ok=%v detail=%+v", ok, detail)
	}
}

func TestPruneUsageByNovelIDDeletesRunsRequestsAndSnapshots(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "state", "ai_usage.sqlite")
	targetNovelID := "novel-1"
	otherNovelID := "novel-2"
	for _, run := range []UsageRun{
		{
			RunID:          "run-target",
			Feature:        "extraction",
			WorkflowName:   "Extraction",
			Status:         "completed",
			StartedAt:      "2026-01-01T00:00:00Z",
			FinishedAt:     "2026-01-01T00:00:01Z",
			ElapsedMs:      1000,
			NovelID:        &targetNovelID,
			GenerationMode: "heuristic",
			RequestCount:   1,
			InputTokens:    1,
			OutputTokens:   2,
			TotalTokens:    3,
			Requests: []UsageRequest{{
				RequestIndex:  1,
				Kind:          "chat",
				ToolNames:     []string{},
				ToolSummaries: []string{},
				InputTokens:   1,
				OutputTokens:  2,
				TotalTokens:   3,
			}},
			Snapshot: map[string]any{"runId": "run-target"},
		},
		{
			RunID:          "run-other",
			Feature:        "extraction",
			WorkflowName:   "Extraction",
			Status:         "completed",
			StartedAt:      "2026-01-02T00:00:00Z",
			FinishedAt:     "2026-01-02T00:00:01Z",
			ElapsedMs:      1000,
			NovelID:        &otherNovelID,
			GenerationMode: "heuristic",
		},
	} {
		if err := SaveUsageRun(dbPath, run); err != nil {
			t.Fatalf("SaveUsageRun(%s) returned error: %v", run.RunID, err)
		}
	}

	deleted, err := PruneUsageByNovelID(dbPath, " novel-1 ")
	if err != nil {
		t.Fatalf("PruneUsageByNovelID returned error: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("expected one deleted run, got %d", deleted)
	}
	if detail, ok, err := LoadUsageRun(dbPath, "run-target"); err != nil || ok || detail.RunID != "" {
		t.Fatalf("target run should be removed: ok=%v detail=%+v err=%v", ok, detail, err)
	}
	other, ok, err := LoadUsageRun(dbPath, "run-other")
	if err != nil || !ok || other.RunID != "run-other" {
		t.Fatalf("other run should remain: ok=%v other=%+v err=%v", ok, other, err)
	}
	usage, ok, err := LoadUsage(dbPath)
	if err != nil || !ok || usage.Summary.RunCount != 1 || len(usage.Runs) != 1 || usage.Runs[0].RunID != "run-other" {
		t.Fatalf("usage should only include other run: ok=%v usage=%+v err=%v", ok, usage, err)
	}

	if deleted, err := PruneUsageByNovelID(dbPath, " "); err != nil || deleted != 0 {
		t.Fatalf("blank prune should be a no-op: deleted=%d err=%v", deleted, err)
	}
	if deleted, err := PruneUsageByNovelID(filepath.Join(t.TempDir(), "missing.sqlite"), "novel-1"); err != nil || deleted != 0 {
		t.Fatalf("missing database prune should be a no-op: deleted=%d err=%v", deleted, err)
	}
	emptyDBPath := filepath.Join(t.TempDir(), "empty.sqlite")
	if err := os.WriteFile(emptyDBPath, nil, 0o600); err != nil {
		t.Fatalf("write empty db: %v", err)
	}
	if deleted, err := PruneUsageByNovelID(emptyDBPath, "novel-1"); err != nil || deleted != 0 {
		t.Fatalf("empty database prune should be a no-op: deleted=%d err=%v", deleted, err)
	}
}

func TestSaveUsageRunMigratesExistingDatabaseMode(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "ai_usage.sqlite")
	file, err := os.OpenFile(dbPath, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		t.Fatalf("create legacy usage database: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close legacy usage database: %v", err)
	}
	if err := SaveUsageRun(dbPath, UsageRun{
		RunID:          "run-mode-migration",
		Feature:        "reader-assistant",
		WorkflowName:   "reader-ai-assistant",
		Status:         "completed",
		StartedAt:      "2026-01-01T00:00:00Z",
		FinishedAt:     "2026-01-01T00:00:01Z",
		GenerationMode: "local",
	}); err != nil {
		t.Fatalf("SaveUsageRun returned error: %v", err)
	}
	info, err := os.Stat(dbPath)
	if err != nil {
		t.Fatalf("stat usage database: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("legacy usage database mode should be migrated to 0600, got %o", info.Mode().Perm())
	}
}

func TestLoadUsageRunIgnoresMalformedSnapshotJSON(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "ai_usage.sqlite")
	db := openTestUsageDB(t, dbPath)
	insertUsageRun(t, db)
	if _, err := db.Exec(`UPDATE ai_usage_run_snapshots SET snapshot_json = ? WHERE run_id = ?`, "{", "run-1"); err != nil {
		t.Fatalf("update malformed snapshot: %v", err)
	}
	db.Close()

	detail, ok, err := LoadUsageRun(dbPath, "run-1")
	if err != nil {
		t.Fatalf("LoadUsageRun returned error: %v", err)
	}
	if !ok || !detail.HasSnapshot || detail.Snapshot != nil {
		t.Fatalf("malformed snapshot should be ignored but metadata preserved: ok=%v detail=%+v", ok, detail)
	}
}

func TestSaveUsageRunRejectsUnserializableSnapshot(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "state", "ai_usage.sqlite")
	run := UsageRun{
		RunID:          "run-bad-snapshot",
		Feature:        "reader-assistant",
		WorkflowName:   "reader-ai-assistant",
		Status:         "completed",
		StartedAt:      "2026-01-01T00:00:00Z",
		FinishedAt:     "2026-01-01T00:00:01Z",
		GenerationMode: "local",
		Snapshot:       map[string]any{"bad": func() {}},
	}
	if err := SaveUsageRun(dbPath, run); err == nil {
		t.Fatal("SaveUsageRun should reject snapshots that cannot be encoded as JSON")
	}
	if usage, ok, err := LoadUsage(dbPath); err != nil || ok || len(usage.Runs) != 0 {
		t.Fatalf("failed snapshot writes should not commit partial usage: ok=%v usage=%+v err=%v", ok, usage, err)
	}
}

func TestSaveUsageRunReturnsFilesystemErrors(t *testing.T) {
	blockedParent := filepath.Join(t.TempDir(), "state")
	if err := os.WriteFile(blockedParent, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("write blocked parent: %v", err)
	}
	err := SaveUsageRun(filepath.Join(blockedParent, "ai_usage.sqlite"), UsageRun{
		RunID:          "run-fs-error",
		Feature:        "reader-assistant",
		WorkflowName:   "reader-ai-assistant",
		Status:         "failed",
		StartedAt:      "2026-01-01T00:00:00Z",
		FinishedAt:     "2026-01-01T00:00:01Z",
		GenerationMode: "local",
	})
	if err == nil {
		t.Fatal("SaveUsageRun should return mkdir errors")
	}
}

func TestLoadUsageLimitsRunListButAggregatesAllRuns(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "ai_usage.sqlite")
	db := openTestUsageDB(t, dbPath)
	defer db.Close()
	for index := 0; index < usageRunListLimit+1; index++ {
		runID := fmt.Sprintf("run-%02d", index)
		startedAt := fmt.Sprintf("2026-02-%02dT00:00:00Z", index+1)
		_, err := db.Exec(`
			INSERT INTO ai_usage_runs (
				run_id, feature, workflow_name, status, started_at, finished_at, elapsed_ms,
				novel_id, novel_title, current_episode_index, model_id, profile_id, profile_label,
				generation_mode, answer_chars, request_count, input_tokens, output_tokens, total_tokens,
				cached_input_tokens, reasoning_output_tokens, total_cost, tool_call_count, tool_result_count,
				error_message
			) VALUES (?, 'reader-assistant', 'Reader assistant', 'completed', ?, ?, 1, NULL, NULL, NULL, NULL, NULL, NULL, 'openrouter', 0, 1, 1, 1, 2, 0, 0, 0.01, 0, 0, NULL);
		`, runID, startedAt, startedAt)
		if err != nil {
			t.Fatalf("insert run %d: %v", index, err)
		}
	}

	usage, ok, err := LoadUsage(dbPath)
	if err != nil {
		t.Fatalf("LoadUsage returned error: %v", err)
	}
	if !ok || len(usage.Runs) != usageRunListLimit {
		t.Fatalf("expected limited run list, ok=%v runs=%d", ok, len(usage.Runs))
	}
	if usage.Summary.RunCount != usageRunListLimit+1 || usage.Summary.TotalTokens != (usageRunListLimit+1)*2 {
		t.Fatalf("summary should aggregate all runs: %+v", usage.Summary)
	}
	if usage.Runs[0].RunID != "run-50" || usage.Runs[len(usage.Runs)-1].RunID != "run-01" {
		t.Fatalf("unexpected limited ordering: first=%s last=%s", usage.Runs[0].RunID, usage.Runs[len(usage.Runs)-1].RunID)
	}
}

func TestLoadUsageHandlesOlderRequestMetadataSchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "old-requests.sqlite")
	db := openTestUsageDB(t, dbPath)
	insertUsageRun(t, db)
	for _, column := range []string{"kind", "parent_request_index", "tool_names", "tool_summaries"} {
		if _, err := db.Exec(`ALTER TABLE ai_usage_requests DROP COLUMN ` + column); err != nil {
			t.Fatalf("drop %s: %v", column, err)
		}
	}
	db.Close()

	usage, ok, err := LoadUsage(dbPath)
	if err != nil {
		t.Fatalf("LoadUsage returned error: %v", err)
	}
	if !ok || len(usage.Runs) != 1 || len(usage.Runs[0].Requests) != 1 {
		t.Fatalf("unexpected usage: ok=%v usage=%+v", ok, usage)
	}
	request := usage.Runs[0].Requests[0]
	if request.Kind != "other" || request.ParentRequestIndex != nil || len(request.ToolNames) != 0 || len(request.ToolSummaries) != 0 {
		t.Fatalf("old request metadata should use defaults: %+v", request)
	}
	db, err = sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open migrated request database: %v", err)
	}
	defer db.Close()
	for _, column := range []string{"kind", "parent_request_index", "tool_names", "tool_summaries"} {
		var found int
		if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('ai_usage_requests') WHERE name = ?`, column).Scan(&found); err != nil {
			t.Fatalf("inspect migrated column %s: %v", column, err)
		}
		if found != 1 {
			t.Fatalf("legacy request column %s was not migrated", column)
		}
	}

	detail, ok, err := LoadUsageRun(dbPath, "run-1")
	if err != nil {
		t.Fatalf("LoadUsageRun returned error: %v", err)
	}
	if !ok || len(detail.Requests) != 1 || detail.Requests[0].Kind != "other" {
		t.Fatalf("unexpected detail defaults: ok=%v detail=%+v", ok, detail)
	}
}

func TestFutureUsageSchemaStopsReadWriteAndPruneWithoutChangingFile(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "ai_usage.sqlite")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open future fixture: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY); INSERT INTO schema_migrations(version) VALUES (99)`); err != nil {
		t.Fatalf("seed future fixture: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close future fixture: %v", err)
	}
	if err := os.Chmod(dbPath, 0o640); err != nil {
		t.Fatalf("set future fixture mode: %v", err)
	}
	before, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("read future fixture: %v", err)
	}

	actions := []struct {
		name string
		run  func() error
	}{
		{name: "list", run: func() error { _, _, err := LoadUsage(dbPath); return err }},
		{name: "detail", run: func() error { _, _, err := LoadUsageRun(dbPath, "run"); return err }},
		{name: "write", run: func() error { return SaveUsageRun(dbPath, UsageRun{RunID: "run"}) }},
		{name: "prune", run: func() error { _, err := PruneUsageByNovelID(dbPath, "novel"); return err }},
	}
	for _, action := range actions {
		t.Run(action.name, func(t *testing.T) {
			err := action.run()
			var future *usagemigration.FutureSchemaError
			if !errors.As(err, &future) || future.Observed != 99 {
				t.Fatalf("error = %v, want future schema error", err)
			}
			after, err := os.ReadFile(dbPath)
			if err != nil {
				t.Fatalf("read rejected future fixture: %v", err)
			}
			if !bytes.Equal(before, after) {
				t.Fatal("future usage database bytes changed during rejected action")
			}
			info, err := os.Stat(dbPath)
			if err != nil {
				t.Fatalf("stat rejected future fixture: %v", err)
			}
			if info.Mode().Perm() != 0o640 {
				t.Fatalf("future usage database mode changed to %o", info.Mode().Perm())
			}
		})
	}
}

func TestLoadUsageAggregatesMultipleRunsInStartedOrder(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "ai_usage.sqlite")
	db := openTestUsageDB(t, dbPath)
	defer db.Close()
	insertUsageRun(t, db)
	_, err := db.Exec(`
		INSERT INTO ai_usage_runs (
			run_id, feature, workflow_name, status, started_at, finished_at, elapsed_ms,
			novel_id, novel_title, current_episode_index, model_id, profile_id, profile_label,
			generation_mode, answer_chars, request_count, input_tokens, output_tokens, total_tokens,
			cached_input_tokens, reasoning_output_tokens, total_cost, tool_call_count, tool_result_count,
			error_message
		) VALUES ('run-2', 'custom-feature', 'Custom workflow', 'failed', '2026-01-02T00:00:00Z', '2026-01-02T00:00:03Z', 3000, NULL, NULL, NULL, NULL, NULL, NULL, 'openrouter', 0, 2, 5, 7, 12, 1, 2, 0.5, 2, 0, 'failed');
		INSERT INTO ai_usage_requests (
			run_id, request_index, kind, parent_request_index, tool_names, tool_summaries, input_tokens, output_tokens, total_tokens, cached_input_tokens, reasoning_output_tokens, cost
		) VALUES
			('run-2', 2, 'retry', 1, '[]', '[]', 2, 3, 5, 0, 0, 0.2),
			('run-2', 1, 'prompt', NULL, '["tool_a","tool_b"]', '["tool_a","tool_b"]', 3, 4, 7, 1, 2, 0.3);
	`)
	if err != nil {
		t.Fatalf("insert second run: %v", err)
	}

	usage, ok, err := LoadUsage(dbPath)
	if err != nil {
		t.Fatalf("LoadUsage returned error: %v", err)
	}
	if !ok || len(usage.Runs) != 2 {
		t.Fatalf("unexpected usage runs: ok=%v usage=%+v", ok, usage)
	}
	if usage.Runs[0].RunID != "run-2" || usage.Runs[0].Feature != "custom-feature" || usage.Runs[1].RunID != "run-1" {
		t.Fatalf("runs were not sorted by started_at desc: %+v", usage.Runs)
	}
	if usage.Summary.RunCount != 2 || usage.Summary.RequestCount != 3 || usage.Summary.InputTokens != 15 || usage.Summary.CachedInputTokens != 4 || usage.Summary.ReasoningOutputTokens != 6 || usage.Summary.TotalCost != 0.75 || usage.Summary.AverageTotalTokens != 21 {
		t.Fatalf("unexpected aggregate summary: %+v", usage.Summary)
	}
	if got := usage.Runs[0].Requests[0].ToolNames; len(got) != 2 || got[0] != "tool_a" || got[1] != "tool_b" {
		t.Fatalf("requests were not sorted by request_index: %+v", usage.Runs[0].Requests)
	}
	detail, ok, err := LoadUsageRun(dbPath, "run-2")
	if err != nil || !ok || detail.Feature != "custom-feature" {
		t.Fatalf("custom feature detail was not loaded: ok=%v detail=%+v err=%v", ok, detail, err)
	}
}

func TestLoadUsageHandlesMissingAndEmptyDatabase(t *testing.T) {
	missingPath := filepath.Join(t.TempDir(), "missing.sqlite")
	if _, ok, err := LoadUsage(missingPath); err != nil || ok {
		t.Fatalf("missing db should return ok=false without error, ok=%v err=%v", ok, err)
	}

	dbPath := filepath.Join(t.TempDir(), "empty.sqlite")
	db := openTestUsageDB(t, dbPath)
	db.Close()
	if usage, ok, err := LoadUsage(dbPath); err != nil || ok || len(usage.Runs) != 0 {
		t.Fatalf("empty db should not be treated as populated, ok=%v err=%v usage=%+v", ok, err, usage)
	}
	if _, ok, err := LoadUsageRun(dbPath, "missing"); err != nil || ok {
		t.Fatalf("missing run should return ok=false without error, ok=%v err=%v", ok, err)
	}
}

func TestLoadUsageReportsSchemaErrors(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "bad.sqlite")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE ai_usage_runs (run_id TEXT PRIMARY KEY);`); err != nil {
		t.Fatalf("create partial schema: %v", err)
	}
	db.Close()
	if _, ok, err := LoadUsage(dbPath); err == nil || ok {
		t.Fatalf("partial runs schema should fail, ok=%v err=%v", ok, err)
	}
	if _, ok, err := LoadUsageRun(dbPath, "run-1"); err == nil || ok {
		t.Fatalf("partial run detail schema should fail, ok=%v err=%v", ok, err)
	}
}

func TestLoadUsageReportsRequestSchemaErrors(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "bad-requests.sqlite")
	db := openTestUsageDB(t, dbPath)
	insertUsageRun(t, db)
	if _, err := db.Exec(`DROP TABLE ai_usage_requests;`); err != nil {
		t.Fatalf("drop requests table: %v", err)
	}
	db.Close()
	if _, ok, err := LoadUsage(dbPath); err == nil || ok {
		t.Fatalf("missing requests table should fail usage list, ok=%v err=%v", ok, err)
	}
	if _, ok, err := LoadUsageRun(dbPath, "run-1"); err == nil || ok {
		t.Fatalf("missing requests table should fail usage detail, ok=%v err=%v", ok, err)
	}
}

func TestDefaultFallbacks(t *testing.T) {
	settings := DefaultSettings("llm")
	if settings.PreferredMode != "llm" || settings.EffectiveGenerationMode != "disabled" {
		t.Fatalf("unexpected settings: %+v", settings)
	}
	heuristic := DefaultSettings("heuristic")
	if heuristic.EffectiveGenerationMode != "heuristic" {
		t.Fatalf("unexpected heuristic mode: %+v", heuristic)
	}
	usage := EmptyUsage()
	if usage.Summary.RunCount != 0 || len(usage.Runs) != 0 {
		t.Fatalf("unexpected fallback usage: %+v", usage)
	}
	if NowISO() == "" {
		t.Fatal("NowISO returned empty string")
	}
	if decoded := decodeStringList("not-json"); len(decoded) != 0 {
		t.Fatalf("invalid JSON should decode to empty list: %+v", decoded)
	}
	if decoded := decodeStringList("null"); len(decoded) != 0 || decoded == nil {
		t.Fatalf("null JSON should decode to non-nil empty list: %#v", decoded)
	}
}

func openTestUsageDB(t *testing.T, dbPath string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	_, err = db.Exec(`
		CREATE TABLE ai_usage_runs (
			run_id TEXT PRIMARY KEY,
			feature TEXT NOT NULL,
			workflow_name TEXT NOT NULL,
			status TEXT NOT NULL,
			started_at TEXT NOT NULL,
			finished_at TEXT NOT NULL,
			elapsed_ms INTEGER NOT NULL,
			novel_id TEXT,
			novel_title TEXT,
			current_episode_index TEXT,
			model_id TEXT,
			profile_id TEXT,
			profile_label TEXT,
			generation_mode TEXT NOT NULL,
			answer_chars INTEGER NOT NULL,
			request_count INTEGER NOT NULL,
			input_tokens INTEGER NOT NULL,
			output_tokens INTEGER NOT NULL,
			total_tokens INTEGER NOT NULL,
			cached_input_tokens INTEGER NOT NULL,
			reasoning_output_tokens INTEGER NOT NULL,
			total_cost REAL NOT NULL,
			tool_call_count INTEGER NOT NULL,
			tool_result_count INTEGER NOT NULL,
			error_message TEXT
		);
		CREATE TABLE ai_usage_requests (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			run_id TEXT NOT NULL,
			request_index INTEGER NOT NULL,
			kind TEXT NOT NULL,
			parent_request_index INTEGER,
			tool_names TEXT NOT NULL,
			tool_summaries TEXT NOT NULL,
			input_tokens INTEGER NOT NULL,
			output_tokens INTEGER NOT NULL,
			total_tokens INTEGER NOT NULL,
			cached_input_tokens INTEGER NOT NULL,
			reasoning_output_tokens INTEGER NOT NULL,
			cost REAL NOT NULL
		);
		CREATE TABLE ai_usage_run_snapshots (
			run_id TEXT PRIMARY KEY,
			snapshot_json TEXT NOT NULL
		);
	`)
	if err != nil {
		t.Fatalf("create schema: %v", err)
	}
	return db
}

func insertUsageRun(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.Exec(`
		INSERT INTO ai_usage_runs (
			run_id, feature, workflow_name, status, started_at, finished_at, elapsed_ms,
			novel_id, novel_title, current_episode_index, model_id, profile_id, profile_label,
			generation_mode, answer_chars, request_count, input_tokens, output_tokens, total_tokens,
			cached_input_tokens, reasoning_output_tokens, total_cost, tool_call_count, tool_result_count,
			error_message
		) VALUES ('run-1', 'reader-assistant', 'Reader assistant', 'completed', '2026-01-01T00:00:00Z', '2026-01-01T00:00:02Z', 2000, 'novel-1', 'Novel 1', '1', 'openrouter/auto', 'default', 'Default', 'openrouter', 123, 1, 10, 20, 30, 3, 4, 0.25, 1, 1, NULL);
		INSERT INTO ai_usage_requests (
			run_id, request_index, kind, parent_request_index, tool_names, tool_summaries, input_tokens, output_tokens, total_tokens, cached_input_tokens, reasoning_output_tokens, cost
		) VALUES ('run-1', 1, 'chat', NULL, '["search_episode"]', '["search_episode"]', 10, 20, 30, 3, 4, 0.25);
		INSERT INTO ai_usage_run_snapshots (run_id, snapshot_json)
		VALUES ('run-1', '{"runId":"run-1","schemaVersion":1}');
	`)
	if err != nil {
		t.Fatalf("insert usage rows: %v", err)
	}
}
