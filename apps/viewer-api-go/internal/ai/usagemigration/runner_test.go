package usagemigration

import (
	"bytes"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestRunCreatesCurrentSchemaAndIsIdempotent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "ai_usage.sqlite")
	db := openTestDB(t, dbPath)
	defer db.Close()

	if err := Run(db, dbPath); err != nil {
		t.Fatalf("first Run returned error: %v", err)
	}
	if err := Run(db, dbPath); err != nil {
		t.Fatalf("second Run returned error: %v", err)
	}

	var migrationCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = 1`).Scan(&migrationCount); err != nil {
		t.Fatalf("read migration version: %v", err)
	}
	if migrationCount != 1 {
		t.Fatalf("migration 1 count = %d, want 1", migrationCount)
	}
	for _, table := range []string{"ai_usage_runs", "ai_usage_requests", "ai_usage_run_snapshots"} {
		var exists int
		if err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?)`, table).Scan(&exists); err != nil {
			t.Fatalf("inspect %s: %v", table, err)
		}
		if exists != 1 {
			t.Fatalf("table %s was not created", table)
		}
	}
	var journalMode string
	if err := db.QueryRow(`PRAGMA journal_mode`).Scan(&journalMode); err != nil {
		t.Fatalf("read journal mode: %v", err)
	}
	if journalMode != "delete" {
		t.Fatalf("journal_mode = %q, want delete", journalMode)
	}
}

func TestRunAdoptsExistingCurrentTablesWithoutLosingRows(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "ai_usage.sqlite")
	db := openTestDB(t, dbPath)
	defer db.Close()
	createBaselineTables(t, db)
	if _, err := db.Exec(`
		INSERT INTO ai_usage_runs (
			run_id, feature, workflow_name, status, started_at, finished_at, elapsed_ms,
			generation_mode, answer_chars, request_count, input_tokens, output_tokens, total_tokens,
			cached_input_tokens, reasoning_output_tokens, total_cost, tool_call_count, tool_result_count
		) VALUES ('legacy-run', 'extraction', 'extraction', 'completed', '2026-01-01T00:00:00Z',
			'2026-01-01T00:00:01Z', 1000, 'heuristic', 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)
	`); err != nil {
		t.Fatalf("seed existing run: %v", err)
	}

	if err := Run(db, dbPath); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	var runID string
	if err := db.QueryRow(`SELECT run_id FROM ai_usage_runs`).Scan(&runID); err != nil {
		t.Fatalf("read adopted run: %v", err)
	}
	if runID != "legacy-run" {
		t.Fatalf("adopted run_id = %q", runID)
	}
}

func TestRunAddsLegacyRequestMetadataColumns(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "ai_usage.sqlite")
	db := openTestDB(t, dbPath)
	defer db.Close()
	createBaselineTables(t, db)
	for _, column := range []string{"kind", "parent_request_index", "tool_names", "tool_summaries"} {
		if _, err := db.Exec(`ALTER TABLE ai_usage_requests DROP COLUMN ` + column); err != nil {
			t.Fatalf("drop legacy column %s: %v", column, err)
		}
	}
	if _, err := db.Exec(`
		INSERT INTO ai_usage_requests (
			run_id, request_index, input_tokens, output_tokens, total_tokens,
			cached_input_tokens, reasoning_output_tokens, cost
		) VALUES ('legacy-run', 0, 1, 2, 3, 0, 0, 0)
	`); err != nil {
		t.Fatalf("seed legacy request: %v", err)
	}

	if err := Run(db, dbPath); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	var kind string
	var parent sql.NullInt64
	var toolNames string
	var toolSummaries string
	if err := db.QueryRow(`
		SELECT kind, parent_request_index, tool_names, tool_summaries
		FROM ai_usage_requests WHERE run_id = 'legacy-run'
	`).Scan(&kind, &parent, &toolNames, &toolSummaries); err != nil {
		t.Fatalf("read migrated request: %v", err)
	}
	if kind != "other" || parent.Valid || toolNames != "[]" || toolSummaries != "[]" {
		t.Fatalf("unexpected migrated metadata: kind=%q parent=%v names=%q summaries=%q", kind, parent, toolNames, toolSummaries)
	}
}

func TestRunRejectsFutureSchemaBeforeChangingDatabase(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "ai_usage.sqlite")
	db := openTestDB(t, dbPath)
	if _, err := db.Exec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY); INSERT INTO schema_migrations(version) VALUES (99)`); err != nil {
		t.Fatalf("seed future schema: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close future fixture: %v", err)
	}
	before, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("read future fixture: %v", err)
	}

	db = openTestDB(t, dbPath)
	err = Run(db, dbPath)
	var future *FutureSchemaError
	if !errors.As(err, &future) || future.Observed != 99 || future.Supported != SupportedLatestVersion {
		t.Fatalf("Run error = %v, want future schema error", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close rejected fixture: %v", err)
	}
	after, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("read rejected fixture: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("future schema database bytes changed during rejected migration")
	}
}

func TestRunRejectsPartialBaselineWithoutRecordingMigration(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "ai_usage.sqlite")
	db := openTestDB(t, dbPath)
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE ai_usage_runs (run_id TEXT PRIMARY KEY)`); err != nil {
		t.Fatalf("create partial baseline: %v", err)
	}
	if err := Run(db, dbPath); err == nil {
		t.Fatal("Run should reject a partial baseline")
	}
	var exists int
	if err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = 'schema_migrations')`).Scan(&exists); err != nil {
		t.Fatalf("inspect migration table: %v", err)
	}
	if exists != 0 {
		t.Fatal("failed baseline adoption should roll back schema_migrations")
	}
}

func openTestDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	return db
}

func createBaselineTables(t *testing.T, db *sql.DB) {
	t.Helper()
	for _, statement := range baselineTables {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("create baseline table: %v", err)
		}
	}
}
