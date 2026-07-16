package usagemigration

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

const SupportedLatestVersion = 1

type FutureSchemaError struct {
	Path      string
	Observed  int
	Supported int
}

func (e *FutureSchemaError) Error() string {
	return fmt.Sprintf(
		"unsupported future VA-AI-USAGE schema at %q: observed migration %d, supported through %d; use a compatible newer build or restore a supported backup",
		e.Path,
		e.Observed,
		e.Supported,
	)
}

func IsFutureSchema(err error) bool {
	var future *FutureSchemaError
	return errors.As(err, &future)
}

func Guard(db *sql.DB, databasePath string) error {
	exists, observed, err := migrationVersion(db)
	if err != nil {
		return err
	}
	return guardObservedVersion(databasePath, exists, observed)
}

func guardObservedVersion(databasePath string, exists bool, observed int) error {
	if !exists {
		return nil
	}
	if observed > SupportedLatestVersion {
		return &FutureSchemaError{Path: databasePath, Observed: observed, Supported: SupportedLatestVersion}
	}
	return nil
}

func Preflight(db *sql.DB, databasePath string) error {
	exists, observed, err := migrationVersion(db)
	if err != nil {
		return err
	}
	if err := guardObservedVersion(databasePath, exists, observed); err != nil {
		return err
	}
	present, err := baselineTablePresence(db)
	if err != nil {
		return err
	}
	if present == 0 && observed < 1 {
		return nil
	}
	if present != len(baselineTables) {
		return fmt.Errorf("VA-AI-USAGE baseline is partial: found %d of %d tables", present, len(baselineTables))
	}
	if err := validateRequiredColumns(db); err != nil {
		return err
	}
	if observed >= 1 {
		columns, err := tableColumns(db, "ai_usage_requests")
		if err != nil {
			return err
		}
		for _, column := range requestMetadataColumns {
			if !columns[column.name] {
				return fmt.Errorf("ai_usage_requests.%s is required after VA-AI-USAGE migration 1", column.name)
			}
		}
	}
	return nil
}

func migrationVersion(db *sql.DB) (bool, int, error) {
	var exists int
	if err := db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM sqlite_master
			WHERE type = 'table' AND name = 'schema_migrations'
		)
	`).Scan(&exists); err != nil {
		return false, 0, err
	}
	if exists == 0 {
		return false, 0, nil
	}
	var observed int
	if err := db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&observed); err != nil {
		return true, 0, err
	}
	return true, observed, nil
}

func Run(db *sql.DB, databasePath string) error {
	if err := Preflight(db, databasePath); err != nil {
		return err
	}
	if _, err := db.Exec(`PRAGMA journal_mode = DELETE`); err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY)`); err != nil {
		return err
	}
	var applied int
	err = tx.QueryRow(`SELECT version FROM schema_migrations WHERE version = 1`).Scan(&applied)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if errors.Is(err, sql.ErrNoRows) {
		if err := applyBaseline(tx); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations(version) VALUES (1)`); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func applyBaseline(tx *sql.Tx) error {
	present, err := baselineTablePresence(tx)
	if err != nil {
		return err
	}
	if present != 0 && present != len(baselineTables) {
		return fmt.Errorf("VA-AI-USAGE baseline is partial: found %d of %d tables", present, len(baselineTables))
	}
	if present == 0 {
		for _, statement := range baselineTables {
			if _, err := tx.Exec(strings.TrimSpace(statement)); err != nil {
				return err
			}
		}
	}
	if err := validateRequiredColumns(tx); err != nil {
		return err
	}
	for _, column := range requestMetadataColumns {
		if err := ensureColumn(tx, "ai_usage_requests", column.name, column.definition); err != nil {
			return err
		}
	}
	return nil
}

func validateRequiredColumns(db queryer) error {
	for _, table := range requiredColumns {
		columns, err := tableColumns(db, table.name)
		if err != nil {
			return err
		}
		for _, column := range table.columns {
			if !columns[column] {
				return fmt.Errorf("%s.%s is required for VA-AI-USAGE baseline adoption", table.name, column)
			}
		}
	}
	return nil
}

func baselineTablePresence(db queryer) (int, error) {
	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type = 'table' AND name IN ('ai_usage_runs', 'ai_usage_requests', 'ai_usage_run_snapshots')`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return 0, err
		}
		count++
	}
	return count, rows.Err()
}

var baselineTables = []string{
	`CREATE TABLE IF NOT EXISTS ai_usage_runs (
		run_id TEXT PRIMARY KEY,
		feature TEXT NOT NULL,
		workflow_name TEXT NOT NULL,
		status TEXT NOT NULL,
		started_at TEXT NOT NULL,
		finished_at TEXT NOT NULL,
		elapsed_ms INTEGER NOT NULL,
		novel_id TEXT NULL,
		novel_title TEXT NULL,
		current_episode_index TEXT NULL,
		model_id TEXT NULL,
		profile_id TEXT NULL,
		profile_label TEXT NULL,
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
		error_message TEXT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS ai_usage_requests (
		run_id TEXT NOT NULL,
		request_index INTEGER NOT NULL,
		kind TEXT NOT NULL,
		parent_request_index INTEGER NULL,
		tool_names TEXT NOT NULL,
		tool_summaries TEXT NOT NULL,
		input_tokens INTEGER NOT NULL,
		output_tokens INTEGER NOT NULL,
		total_tokens INTEGER NOT NULL,
		cached_input_tokens INTEGER NOT NULL,
		reasoning_output_tokens INTEGER NOT NULL,
		cost REAL NOT NULL,
		PRIMARY KEY (run_id, request_index)
	)`,
	`CREATE TABLE IF NOT EXISTS ai_usage_run_snapshots (
		run_id TEXT PRIMARY KEY,
		snapshot_json TEXT NOT NULL
	)`,
}

var requiredColumns = []struct {
	name    string
	columns []string
}{
	{name: "ai_usage_runs", columns: []string{
		"run_id", "feature", "workflow_name", "status", "started_at", "finished_at", "elapsed_ms",
		"novel_id", "novel_title", "current_episode_index", "model_id", "profile_id", "profile_label",
		"generation_mode", "answer_chars", "request_count", "input_tokens", "output_tokens", "total_tokens",
		"cached_input_tokens", "reasoning_output_tokens", "total_cost", "tool_call_count", "tool_result_count", "error_message",
	}},
	{name: "ai_usage_requests", columns: []string{
		"run_id", "request_index", "input_tokens", "output_tokens", "total_tokens", "cached_input_tokens", "reasoning_output_tokens", "cost",
	}},
	{name: "ai_usage_run_snapshots", columns: []string{"run_id", "snapshot_json"}},
}

var requestMetadataColumns = []struct {
	name       string
	definition string
}{
	{name: "kind", definition: "TEXT NOT NULL DEFAULT 'other'"},
	{name: "parent_request_index", definition: "INTEGER NULL"},
	{name: "tool_names", definition: "TEXT NOT NULL DEFAULT '[]'"},
	{name: "tool_summaries", definition: "TEXT NOT NULL DEFAULT '[]'"},
}

func ensureColumn(tx *sql.Tx, table string, column string, definition string) error {
	columns, err := tableColumns(tx, table)
	if err != nil {
		return err
	}
	if columns[column] {
		return nil
	}
	_, err = tx.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + column + ` ` + definition)
	return err
}

type queryer interface {
	Query(query string, args ...any) (*sql.Rows, error)
}

func tableColumns(db queryer, table string) (map[string]bool, error) {
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	columns := map[string]bool{}
	for rows.Next() {
		var cid int
		var name string
		var columnType string
		var notNull int
		var defaultValue sql.NullString
		var primaryKey int
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return nil, err
		}
		columns[name] = true
	}
	return columns, rows.Err()
}
