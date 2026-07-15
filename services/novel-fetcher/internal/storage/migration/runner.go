package migration

import (
	"database/sql"
	"errors"
	"fmt"
)

const SupportedLatestVersion = 3

type ErrFutureSchema struct {
	Path      string
	Observed  int
	Supported int
}

func (e ErrFutureSchema) Error() string {
	return fmt.Sprintf(
		"unsupported future NF-LIBRARY schema at %q: observed migration %d, supported through %d; use a compatible newer build or restore a supported backup",
		e.Path,
		e.Observed,
		e.Supported,
	)
}

type Migration struct {
	Version int
	Name    string
	Up      func(dbtx) error
}

type dbtx interface {
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

func Run(db *sql.DB, databasePath string) error {
	if err := rejectFutureSchema(db, databasePath); err != nil {
		return err
	}

	statements := []string{
		`PRAGMA auto_vacuum = INCREMENTAL`,
		`PRAGMA journal_mode = WAL`,
		`PRAGMA foreign_keys = ON`,
		`PRAGMA wal_autocheckpoint = 1000`,
		`CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY
		)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			return err
		}
	}
	if err := ensureIncrementalAutoVacuum(db); err != nil {
		return err
	}

	for _, migration := range migrations {
		if err := runMigration(db, migration); err != nil {
			return err
		}
	}
	return nil
}

func rejectFutureSchema(db *sql.DB, databasePath string) error {
	var migrationTableExists int
	if err := db.QueryRow(`
		SELECT EXISTS(
			SELECT 1
			FROM sqlite_master
			WHERE type = 'table' AND name = 'schema_migrations'
		)
	`).Scan(&migrationTableExists); err != nil {
		return err
	}
	if migrationTableExists == 0 {
		return nil
	}

	var observed int
	if err := db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&observed); err != nil {
		return err
	}
	if observed > SupportedLatestVersion {
		return ErrFutureSchema{
			Path:      databasePath,
			Observed:  observed,
			Supported: SupportedLatestVersion,
		}
	}
	return nil
}

var migrations = []Migration{
	{
		Version: 1,
		Name:    "initial_library_schema",
		Up: func(db dbtx) error {
			statements := []string{
				`CREATE TABLE IF NOT EXISTS works (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					site TEXT NOT NULL,
					site_name TEXT NOT NULL,
					site_work_id TEXT NOT NULL,
					source_url TEXT NOT NULL,
					title TEXT NOT NULL,
					author TEXT NOT NULL,
					story TEXT NOT NULL,
					directory TEXT NOT NULL,
					fetched_at TEXT NOT NULL,
					created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
					UNIQUE(site, site_work_id)
				)`,
				`CREATE TABLE IF NOT EXISTS episodes (
					work_id INTEGER NOT NULL REFERENCES works(id) ON DELETE CASCADE,
					episode_id TEXT NOT NULL,
					site_episode_id TEXT NOT NULL,
					source_url TEXT NOT NULL DEFAULT '',
					sort_order INTEGER NOT NULL,
					display_index TEXT NOT NULL,
					title TEXT NOT NULL,
					chapter TEXT NOT NULL,
					subchapter TEXT NOT NULL,
					published_at TEXT NOT NULL,
					updated_at TEXT NOT NULL,
					body_path TEXT NOT NULL,
					raw_path TEXT NOT NULL,
					content_hash TEXT NOT NULL,
					fetched_at TEXT NOT NULL,
					PRIMARY KEY(work_id, episode_id)
				)`,
				`CREATE INDEX IF NOT EXISTS episodes_work_sort_idx ON episodes(work_id, sort_order)`,
				`CREATE TABLE IF NOT EXISTS assets (
					asset_id TEXT PRIMARY KEY,
					work_id INTEGER NOT NULL REFERENCES works(id) ON DELETE CASCADE,
					episode_id TEXT,
					source_url TEXT NOT NULL,
					storage_path TEXT NOT NULL,
					media_type TEXT NOT NULL,
					byte_length INTEGER NOT NULL DEFAULT 0,
					width INTEGER NOT NULL DEFAULT 0,
					height INTEGER NOT NULL DEFAULT 0,
					content_hash TEXT NOT NULL,
					fetched_at TEXT NOT NULL
				)`,
			}
			for _, statement := range statements {
				if _, err := db.Exec(statement); err != nil {
					return err
				}
			}
			return nil
		},
	},
	{
		Version: 2,
		Name:    "fetch_status_columns",
		Up: func(db dbtx) error {
			columns := []struct {
				table      string
				column     string
				definition string
			}{
				{"works", "fetch_status", "TEXT NOT NULL DEFAULT 'complete'"},
				{"works", "last_fetch_error", "TEXT NOT NULL DEFAULT ''"},
				{"works", "last_failed_episode_id", "TEXT NOT NULL DEFAULT ''"},
				{"works", "resume_episode_id", "TEXT NOT NULL DEFAULT ''"},
				{"works", "expected_episode_count", "INTEGER NOT NULL DEFAULT 0"},
				{"episodes", "body_status", "TEXT NOT NULL DEFAULT 'complete'"},
				{"episodes", "source_url", "TEXT NOT NULL DEFAULT ''"},
				{"episodes", "last_fetch_error", "TEXT NOT NULL DEFAULT ''"},
				{"episodes", "last_attempted_at", "TEXT NOT NULL DEFAULT ''"},
			}
			for _, column := range columns {
				if err := ensureColumn(db, column.table, column.column, column.definition); err != nil {
					return err
				}
			}
			return nil
		},
	},
	{
		Version: 3,
		Name:    "fetch_runs",
		Up: func(db dbtx) error {
			// Reserved for future fetch audit/history views. Current fetch flow persists
			// resumable work and episode state directly through works/episodes.
			_, err := db.Exec(`
				CREATE TABLE IF NOT EXISTS fetch_runs (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					work_id INTEGER REFERENCES works(id) ON DELETE CASCADE,
					task_id TEXT NOT NULL,
					target TEXT NOT NULL,
					mode TEXT NOT NULL,
					status TEXT NOT NULL,
					started_at TEXT NOT NULL,
					finished_at TEXT NOT NULL DEFAULT '',
					total_episode_count INTEGER NOT NULL DEFAULT 0,
					saved_episode_count INTEGER NOT NULL DEFAULT 0,
					failed_episode_id TEXT NOT NULL DEFAULT '',
					error_message TEXT NOT NULL DEFAULT ''
				)
			`)
			return err
		},
	},
}

func runMigration(db *sql.DB, migration Migration) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	applied, err := isApplied(tx, migration.Version)
	if err != nil {
		return err
	}
	if applied {
		return nil
	}

	if err := migration.Up(tx); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO schema_migrations(version) VALUES (?)`, migration.Version); err != nil {
		return err
	}
	return tx.Commit()
}

func isApplied(tx *sql.Tx, version int) (bool, error) {
	var appliedVersion int
	err := tx.QueryRow(`SELECT version FROM schema_migrations WHERE version = ?`, version).Scan(&appliedVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func ensureIncrementalAutoVacuum(db *sql.DB) error {
	var mode int
	if err := db.QueryRow(`PRAGMA auto_vacuum`).Scan(&mode); err != nil {
		return err
	}
	if mode == 2 {
		return nil
	}
	if _, err := db.Exec(`PRAGMA auto_vacuum = INCREMENTAL`); err != nil {
		return err
	}
	if mode == 0 {
		_, err := db.Exec(`VACUUM`)
		return err
	}
	return nil
}

func ensureColumn(db dbtx, table string, column string, definition string) error {
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name string
		var columnType string
		var notNull int
		var defaultValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &pk); err != nil {
			return err
		}
		if name == column {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	_, err = db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + column + ` ` + definition)
	return err
}
