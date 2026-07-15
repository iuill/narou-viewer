package migration

import (
	"database/sql"
	"errors"
	"testing"

	_ "modernc.org/sqlite"
)

func TestRunCreatesSchemaAndRecordsMigrations(t *testing.T) {
	if latest := migrations[len(migrations)-1].Version; latest != SupportedLatestVersion {
		t.Fatalf("latest migration = %d, SupportedLatestVersion = %d", latest, SupportedLatestVersion)
	}

	db := openTestDB(t)
	defer db.Close()

	if err := Run(db, ":memory:"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if err := Run(db, ":memory:"); err != nil {
		t.Fatalf("second Run() error = %v", err)
	}

	assertColumnExists(t, db, "works", "fetch_status")
	assertColumnExists(t, db, "works", "resume_episode_id")
	assertColumnExists(t, db, "episodes", "body_status")
	assertTableExists(t, db, "fetch_runs")

	var migrationCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&migrationCount); err != nil {
		t.Fatalf("query schema_migrations: %v", err)
	}
	if migrationCount != len(migrations) {
		t.Fatalf("migration count = %d, want %d", migrationCount, len(migrations))
	}
}

func TestRunUpgradesPartiallyInitializedSchema(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	if _, err := db.Exec(`
		CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY);
		CREATE TABLE works (
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
		);
		CREATE TABLE episodes (
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
		);
	`); err != nil {
		t.Fatalf("seed partial schema: %v", err)
	}

	if err := Run(db, ":memory:"); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	assertColumnExists(t, db, "works", "last_fetch_error")
	assertColumnExists(t, db, "episodes", "last_attempted_at")
	assertTableExists(t, db, "assets")
	assertTableExists(t, db, "fetch_runs")
}

func TestRunMigrationSkipsAlreadyAppliedVersion(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	if _, err := db.Exec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY); INSERT INTO schema_migrations(version) VALUES (99);`); err != nil {
		t.Fatalf("seed schema_migrations: %v", err)
	}

	called := false
	err := runMigration(db, Migration{
		Version: 99,
		Name:    "already_applied",
		Up: func(dbtx) error {
			called = true
			return nil
		},
	})
	if err != nil {
		t.Fatalf("runMigration() error = %v", err)
	}
	if called {
		t.Fatal("already-applied migration was executed")
	}
}

func TestRunRejectsFutureSchemaBeforeKnownMigrations(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	if _, err := db.Exec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY); INSERT INTO schema_migrations(version) VALUES (99);`); err != nil {
		t.Fatalf("seed schema_migrations: %v", err)
	}

	err := Run(db, "future-library.sqlite")
	var futureSchema ErrFutureSchema
	if !errors.As(err, &futureSchema) {
		t.Fatalf("Run() error = %v, want ErrFutureSchema", err)
	}
	if futureSchema.Path != "future-library.sqlite" || futureSchema.Observed != 99 || futureSchema.Supported != SupportedLatestVersion {
		t.Fatalf("ErrFutureSchema = %#v", futureSchema)
	}

	var worksTableCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'works'`).Scan(&worksTableCount); err != nil {
		t.Fatalf("query works table: %v", err)
	}
	if worksTableCount != 0 {
		t.Fatal("known migrations ran before the future schema guard")
	}
}

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	return db
}

func assertColumnExists(t *testing.T, db *sql.DB, table string, column string) {
	t.Helper()
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		t.Fatalf("table_info(%s): %v", table, err)
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
			t.Fatalf("scan column: %v", err)
		}
		if name == column {
			return
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("scan columns: %v", err)
	}
	t.Fatalf("column %s.%s was not found", table, column)
}

func assertTableExists(t *testing.T, db *sql.DB, table string) {
	t.Helper()
	var name string
	err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&name)
	if err != nil {
		t.Fatalf("table %s was not found: %v", table, err)
	}
}
