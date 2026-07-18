package statedoctor

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"narou-viewer/apps/viewer-api-go/internal/application/readertextcache"
	"narou-viewer/apps/viewer-api-go/internal/state/schemaguard"
)

func TestScanRejectsInvalidRootsAndReportsInventoryIOFailures(t *testing.T) {
	if _, err := Scan(context.Background(), " "); err == nil {
		t.Fatal("empty data directory should fail")
	}
	if _, err := Scan(context.Background(), filepath.Join(t.TempDir(), "missing")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing data directory error = %v", err)
	}
	fileRoot := filepath.Join(t.TempDir(), "data-file")
	if err := os.WriteFile(fileRoot, []byte("synthetic"), 0o600); err != nil {
		t.Fatalf("write data file: %v", err)
	}
	if _, err := Scan(context.Background(), fileRoot); err == nil {
		t.Fatal("regular-file data root should fail")
	}

	badPatternRoot := filepath.Join(t.TempDir(), "[invalid-glob")
	if err := os.MkdirAll(badPatternRoot, 0o700); err != nil {
		t.Fatalf("mkdir invalid glob root: %v", err)
	}
	report, err := Scan(context.Background(), badPatternRoot)
	if err != nil {
		t.Fatalf("Scan invalid glob root: %v", err)
	}
	if !reportHasKind(report, "inventory_error") {
		t.Fatalf("invalid glob root should report inventory errors: %+v", report.Findings)
	}

	ioRoot := t.TempDir()
	stateDir := filepath.Join(ioRoot, "state")
	for _, path := range []string{
		filepath.Join(stateDir, "character_events", "directory.yaml"),
		filepath.Join(stateDir, "extraction_jobs", "checkpoints", "directory.json"),
	} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatalf("mkdir unreadable inventory entry: %v", err)
		}
	}
	report, err = Scan(context.Background(), ioRoot)
	if err != nil {
		t.Fatalf("Scan directory entries: %v", err)
	}
	if !reportHasKind(report, "read_error") {
		t.Fatalf("directory entries should report read errors: %+v", report.Findings)
	}

	blockedStateRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(blockedStateRoot, "state"), []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("write blocked state path: %v", err)
	}
	report, err = Scan(context.Background(), blockedStateRoot)
	if err != nil {
		t.Fatalf("Scan blocked state path: %v", err)
	}
	for _, kind := range []string{"read_error", "plaintext_credential_scan_error", "crypto_version_scan_error", "sqlite_open_error"} {
		if !reportHasKind(report, kind) {
			t.Fatalf("blocked state path should report %s: %+v", kind, report.Findings)
		}
	}
}

func TestScannerReconciliationCoversInvalidAndConflictingDerivedState(t *testing.T) {
	dataDir := t.TempDir()
	stateDir := filepath.Join(dataDir, "state")
	s := &scanner{
		dataDir:         dataDir,
		stateDir:        stateDir,
		novelFetcherDir: filepath.Join(dataDir, "novel-fetcher"),
		report:          Report{DataDir: dataDir, Findings: []Finding{}},
		yamlFiles:       map[string]scannedFile{},
		jsonFiles:       map[string]scannedFile{},
		libraryNovelIDs: map[string]bool{"known": true},
		libraryReadable: true,
	}
	addYAML := func(relative string, raw string, accepted bool) string {
		t.Helper()
		path := filepath.Join(stateDir, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatalf("mkdir fixture: %v", err)
		}
		if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
		s.yamlFiles[path] = scannedFile{path: path, raw: []byte(raw), exists: true, accepted: accepted}
		return path
	}

	addYAML("extraction_jobs/invalid.yaml", "schema_version: 2\nrevision: 1\njob_id: other\nnovel_id: novel-invalid\nrequested_up_to_episode_index: '1'\nstatus: completed\n", true)
	addYAML("extraction_jobs/skipped.yaml", "job_id: skipped\nnovel_id: skipped\n", false)
	addYAML("extraction_jobs/active-a.yaml", "schema_version: 2\nrevision: 1\njob_id: active-a\nnovel_id: novel-active\nrequested_up_to_episode_index: '1'\nstatus: queued\n", true)
	addYAML("extraction_jobs/active-b.yaml", "schema_version: 2\nrevision: 1\njob_id: active-b\nnovel_id: novel-active\nrequested_up_to_episode_index: '1'\nstatus: running\n", true)
	addYAML("extraction_jobs/single.yaml", "schema_version: 2\nrevision: 1\njob_id: single\nnovel_id: novel-single\nrequested_up_to_episode_index: '1'\nstatus: queued\n", true)
	addYAML("extraction_jobs/index/novel-active.yaml", "novel_id: novel-active\nactive_job_id: missing\njob_ids: [active-a, active-b]\n", true)
	addYAML("extraction_jobs/index/invalid-index.yaml", ":\n", true)
	addYAML("extraction_jobs/index/skipped-index.yaml", "novel_id: skipped-index\n", false)
	addYAML("extraction_jobs/unsafe.yaml", "schema_version: 2\nrevision: 1\njob_id: unsafe\nnovel_id: ../unsafe\nrequested_up_to_episode_index: '1'\nstatus: completed\n", true)
	s.scanJobIndexConsistency()
	for _, kind := range []string{"typed_payload_invalid", "multiple_active_jobs", "job_index_mismatch"} {
		if !reportHasKind(s.report, kind) {
			t.Fatalf("job reconciliation should report %s: %+v", kind, s.report.Findings)
		}
	}

	addYAML("character_events/with-events.yaml", "novel_id: with-events\nprocessed_up_to_episode_index: '5'\n", true)
	addYAML("character_profiles/with-events.yaml", "schema_version: 999\n", false)
	addYAML("character_profiles/profile-only.yaml", "schema_version: 1\n", true)
	addYAML("term_profiles/no-events.yaml", "novel_id: no-events\nprocessed_up_to_episode_index: '2'\n", true)
	addYAML("term_profiles/with-events.yaml", "novel_id: with-events\nprocessed_up_to_episode_index: '10'\n", true)
	addYAML("term_profiles/no-frontier.yaml", "novel_id: no-frontier\n", true)
	addYAML("term_profiles/skipped.yaml", "novel_id: skipped\nprocessed_up_to_episode_index: '1'\n", false)
	s.scanCharacterTermConsistency()
	for _, kind := range []string{"profiles_without_events", "character_profile_rebuildable", "term_frontier_without_character_frontier", "frontier_inversion"} {
		if !reportHasKind(s.report, kind) {
			t.Fatalf("frontier reconciliation should report %s: %+v", kind, s.report.Findings)
		}
	}

	addYAML("reading_state.yaml", "novels:\n  known: {}\n  orphan-reading: {}\n", true)
	addYAML("bookmarks.yaml", "bookmarks:\n  - novel_id: orphan-bookmark\n  - novel_id: orphan-bookmark\n  - novel_id: known\n  - novel_id: ''\n", true)
	s.scanLibraryOrphans()
	if !reportHasKind(s.report, "orphan_novel_state") {
		t.Fatalf("library reconciliation should report orphan state: %+v", s.report.Findings)
	}

	if sameStringSet([]string{"a"}, []string{"b"}) || sameStringSet([]string{"a"}, []string{"a", "b"}) {
		t.Fatal("sameStringSet should reject unequal sets")
	}
	if !containsString([]string{"a", "b"}, "b") || containsString([]string{"a"}, "b") {
		t.Fatal("containsString result mismatch")
	}
	if safeFileComponent("../unsafe") || !safeFileComponent("safe") {
		t.Fatal("safeFileComponent result mismatch")
	}
	if compareEpisodeIndex("10", "2") <= 0 || compareEpisodeIndex("episode-b", "episode-a") <= 0 {
		t.Fatal("compareEpisodeIndex result mismatch")
	}
}

func TestScannerFormattingAndSchemaResultBranches(t *testing.T) {
	s := &scanner{dataDir: t.TempDir(), report: Report{Findings: []Finding{}}}
	legacy := 1
	current := 2
	contract := schemaguard.Contract{ID: "TEST", Current: 2, ReadableLegacy: []int{1}}
	s.addSchemaResult(filepath.Join(s.dataDir, "legacy.yaml"), schemaguard.Result{Contract: contract, Observed: &legacy, Status: schemaguard.StatusLegacy}, nil)
	s.addSchemaResult(filepath.Join(s.dataDir, "current.yaml"), schemaguard.Result{Contract: contract, Observed: &current, Status: schemaguard.StatusCurrent}, nil)
	s.addSchemaResult(filepath.Join(s.dataDir, "malformed.yaml"), schemaguard.Result{Contract: contract, Status: schemaguard.StatusMalformed}, errors.New("synthetic malformed schema"))
	for _, kind := range []string{"schema_legacy", "schema_current", "schema_malformed"} {
		if !reportHasKind(s.report, kind) {
			t.Fatalf("schema result should report %s: %+v", kind, s.report.Findings)
		}
	}
	if got := observedVersion(schemaguard.Result{Status: schemaguard.StatusMalformed}); got != "malformed" {
		t.Fatalf("malformed observed version = %q", got)
	}
	if got := observedVersion(schemaguard.Result{Status: schemaguard.StatusCurrent}); got != "missing" {
		t.Fatalf("missing observed version = %q", got)
	}
	if got := emptyDash(" "); got != "-" {
		t.Fatalf("emptyDash = %q", got)
	}

	report := Report{Findings: []Finding{
		{ID: "z", Path: "same", SchemaID: "B", Kind: "kind", Severity: SeverityWarning, RepairKind: "repair"},
		{ID: "a", Path: "same", SchemaID: "A", Kind: "kind", Severity: SeverityError},
		{ID: "b", Path: "same", SchemaID: "A", Kind: "kind", Severity: SeverityInfo},
	}}
	report.finalize()
	if report.Findings[0].ID != "a" || report.Summary != (Summary{Inventory: 1, Warnings: 1, Errors: 1, Repairable: 1}) {
		t.Fatalf("finalized report = %+v", report)
	}
}

func TestApplyPropagatesInitialScanFailure(t *testing.T) {
	if _, err := Apply(context.Background(), filepath.Join(t.TempDir(), "missing"), []string{"finding-synthetic"}); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Apply initial scan error = %v", err)
	}
}

func TestSQLiteDiagnosticsCoverCancellationLedgerAndStorageFailures(t *testing.T) {
	dataDir := t.TempDir()
	stateDir := filepath.Join(dataDir, "state")
	novelFetcherDir := filepath.Join(dataDir, "novel-fetcher")
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatalf("mkdir state: %v", err)
	}
	s := &scanner{
		dataDir:         dataDir,
		stateDir:        stateDir,
		novelFetcherDir: novelFetcherDir,
		report:          Report{DataDir: dataDir, Findings: []Finding{}},
		yamlFiles:       map[string]scannedFile{},
		jsonFiles:       map[string]scannedFile{},
		libraryNovelIDs: map[string]bool{},
	}

	readerPath := filepath.Join(stateDir, "reader_search.sqlite")
	readerDB, err := sql.Open("sqlite", readerPath)
	if err != nil {
		t.Fatalf("open reader fixture: %v", err)
	}
	if _, err := readerDB.Exec(`CREATE TABLE reader_search_texts (
		novel_id TEXT, episode_index TEXT, content_etag TEXT, text TEXT,
		plain_text_length INTEGER, updated_at TEXT
	); PRAGMA user_version = 1;`); err != nil {
		t.Fatalf("seed reader fixture: %v", err)
	}
	if err := readerDB.Close(); err != nil {
		t.Fatalf("close reader fixture: %v", err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	s.scanReaderSearchSQLite(cancelled)
	if !reportHasKind(s.report, "cache_version_error") || !reportHasKind(s.report, "sqlite_integrity_error") {
		t.Fatalf("cancelled reader scan findings: %+v", s.report.Findings)
	}

	ledgerPath := filepath.Join(stateDir, "ledger.sqlite")
	ledgerDB, err := sql.Open("sqlite", ledgerPath)
	if err != nil {
		t.Fatalf("open ledger fixture: %v", err)
	}
	if _, err := ledgerDB.Exec(`CREATE TABLE schema_migrations (version TEXT); INSERT INTO schema_migrations(version) VALUES ('not-a-number')`); err != nil {
		t.Fatalf("seed ledger fixture: %v", err)
	}
	if exists, _, err := sqliteMigrationVersion(context.Background(), ledgerDB); err == nil || !exists {
		t.Fatalf("invalid migration ledger should fail after table discovery: exists=%v err=%v", exists, err)
	}
	if err := ledgerDB.Close(); err != nil {
		t.Fatalf("close ledger fixture: %v", err)
	}
	if _, _, err := sqliteMigrationVersion(context.Background(), ledgerDB); err == nil {
		t.Fatal("closed migration database should fail table discovery")
	}
	if readertextcache.ValidateSchema(context.Background(), ledgerDB) == nil {
		t.Fatal("closed database should not have a valid reader schema")
	}
	if s.scanQuickCheck(context.Background(), "VA-READER-SEARCH", ledgerPath, ledgerDB) {
		t.Fatal("closed database quick_check should fail")
	}
	if got := sqliteRecoveryHint("VA-READER-SEARCH"); got == sqliteRecoveryHint("NF-LIBRARY") {
		t.Fatalf("cache recovery hint should differ from canonical state hint: %q", got)
	}

	if err := os.MkdirAll(novelFetcherDir, 0o700); err != nil {
		t.Fatalf("mkdir novel-fetcher: %v", err)
	}
	libraryPath := filepath.Join(novelFetcherDir, "library.sqlite")
	libraryDB, err := sql.Open("sqlite", libraryPath)
	if err != nil {
		t.Fatalf("open library fixture: %v", err)
	}
	if _, err := libraryDB.Exec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY); INSERT INTO schema_migrations(version) VALUES (3)`); err != nil {
		t.Fatalf("seed library fixture: %v", err)
	}
	if err := libraryDB.Close(); err != nil {
		t.Fatalf("close library fixture: %v", err)
	}
	s.scanNovelFetcherSQLite(context.Background())
	if !reportHasKind(s.report, "storage_contract_error") {
		t.Fatalf("missing canonical tables should report storage contract error: %+v", s.report.Findings)
	}
}

func TestCanonicalEpisodeDiagnosticsCoverUnreadableAndResolvedOutsidePaths(t *testing.T) {
	dataDir := t.TempDir()
	novelFetcherDir := filepath.Join(dataDir, "novel-fetcher")
	if err := os.MkdirAll(filepath.Join(novelFetcherDir, "works"), 0o700); err != nil {
		t.Fatalf("mkdir canonical root: %v", err)
	}
	s := &scanner{
		dataDir:         dataDir,
		novelFetcherDir: novelFetcherDir,
		report:          Report{DataDir: dataDir, Findings: []Finding{}},
	}
	unreadable := filepath.Join(novelFetcherDir, "works", "episodes", "directory.json")
	if err := os.MkdirAll(unreadable, 0o700); err != nil {
		t.Fatalf("mkdir unreadable canonical fixture: %v", err)
	}
	s.scanCanonicalEpisode(unreadable, "")
	if !reportHasKind(s.report, "read_error") {
		t.Fatalf("directory canonical body should report read error: %+v", s.report.Findings)
	}

	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "episode.json"), []byte(`{"schema_version":1}`), 0o600); err != nil {
		t.Fatalf("write outside episode: %v", err)
	}
	linkRoot := filepath.Join(novelFetcherDir, "linked-works")
	if err := os.Symlink(outside, linkRoot); err != nil {
		t.Fatalf("symlink outside root: %v", err)
	}
	s.scanCanonicalEpisode(filepath.Join(linkRoot, "episode.json"), "")
	if !reportHasKind(s.report, "resolved_path_outside_storage") {
		t.Fatalf("resolved outside body should be rejected: %+v", s.report.Findings)
	}

	blockedRoot := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(blockedRoot, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("write blocked works parent: %v", err)
	}
	s.novelFetcherDir = blockedRoot
	if err := s.scanUnreferencedCanonicalEpisodes(map[string]bool{}); err == nil {
		t.Fatal("blocked canonical works root should fail")
	}
}

func TestSQLiteDiagnosticsCoverInvalidCurrentSchemasAndRowShapes(t *testing.T) {
	newScanner := func(dataDir string) *scanner {
		return &scanner{
			dataDir:         dataDir,
			stateDir:        filepath.Join(dataDir, "state"),
			novelFetcherDir: filepath.Join(dataDir, "novel-fetcher"),
			report:          Report{DataDir: dataDir, Findings: []Finding{}},
			yamlFiles:       map[string]scannedFile{},
			jsonFiles:       map[string]scannedFile{},
			libraryNovelIDs: map[string]bool{},
		}
	}

	aiRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(aiRoot, "state"), 0o700); err != nil {
		t.Fatalf("mkdir AI state: %v", err)
	}
	aiDB, err := sql.Open("sqlite", filepath.Join(aiRoot, "state", "ai_usage.sqlite"))
	if err != nil {
		t.Fatalf("open invalid current AI DB: %v", err)
	}
	if _, err := aiDB.Exec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY); INSERT INTO schema_migrations(version) VALUES (1)`); err != nil {
		t.Fatalf("seed invalid current AI DB: %v", err)
	}
	if err := aiDB.Close(); err != nil {
		t.Fatalf("close invalid current AI DB: %v", err)
	}
	aiScanner := newScanner(aiRoot)
	aiScanner.scanAIUsageSQLite(context.Background())
	if !reportHasKind(aiScanner.report, "schema_invalid") {
		t.Fatalf("partial current AI schema should be invalid: %+v", aiScanner.report.Findings)
	}

	readerRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(readerRoot, "state"), 0o700); err != nil {
		t.Fatalf("mkdir reader state: %v", err)
	}
	readerDB, err := sql.Open("sqlite", filepath.Join(readerRoot, "state", "reader_search.sqlite"))
	if err != nil {
		t.Fatalf("open reader schema fixture: %v", err)
	}
	if _, err := readerDB.Exec(`CREATE TABLE reader_search_texts (
		novel_id TEXT NOT NULL,
		episode_index TEXT NOT NULL,
		content_etag TEXT NOT NULL,
		text TEXT NOT NULL,
		plain_text_length INTEGER NOT NULL,
		updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
	); PRAGMA user_version = 1`); err != nil {
		t.Fatalf("seed reader schema fixture: %v", err)
	}
	if err := readerDB.Close(); err != nil {
		t.Fatalf("close reader schema fixture: %v", err)
	}
	readerScanner := newScanner(readerRoot)
	readerScanner.scanReaderSearchSQLite(context.Background())
	if !reportHasKind(readerScanner.report, "cache_schema_mismatch") {
		t.Fatalf("current version without the conflict key should mismatch: %+v", readerScanner.report.Findings)
	}

	emptyNFRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(emptyNFRoot, "novel-fetcher"), 0o700); err != nil {
		t.Fatalf("mkdir empty novel-fetcher: %v", err)
	}
	if err := os.WriteFile(filepath.Join(emptyNFRoot, "novel-fetcher", "library.sqlite"), nil, 0o600); err != nil {
		t.Fatalf("write empty library: %v", err)
	}
	emptyScanner := newScanner(emptyNFRoot)
	emptyScanner.scanNovelFetcherSQLite(context.Background())
	if !reportHasKind(emptyScanner.report, "sqlite_open_error") {
		t.Fatalf("empty library should fail open: %+v", emptyScanner.report.Findings)
	}

	ledgerNFRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(ledgerNFRoot, "novel-fetcher"), 0o700); err != nil {
		t.Fatalf("mkdir ledger novel-fetcher: %v", err)
	}
	ledgerNFDB, err := sql.Open("sqlite", filepath.Join(ledgerNFRoot, "novel-fetcher", "library.sqlite"))
	if err != nil {
		t.Fatalf("open invalid library ledger: %v", err)
	}
	if _, err := ledgerNFDB.Exec(`CREATE TABLE schema_migrations (version TEXT); INSERT INTO schema_migrations(version) VALUES ('invalid')`); err != nil {
		t.Fatalf("seed invalid library ledger: %v", err)
	}
	if err := ledgerNFDB.Close(); err != nil {
		t.Fatalf("close invalid library ledger: %v", err)
	}
	ledgerNFScanner := newScanner(ledgerNFRoot)
	ledgerNFScanner.scanNovelFetcherSQLite(context.Background())
	if !reportHasKind(ledgerNFScanner.report, "migration_ledger_error") {
		t.Fatalf("invalid library ledger should be reported: %+v", ledgerNFScanner.report.Findings)
	}

	rowRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(rowRoot, "novel-fetcher"), 0o700); err != nil {
		t.Fatalf("mkdir row novel-fetcher: %v", err)
	}
	rowScanner := newScanner(rowRoot)
	rowDB, err := sql.Open("sqlite", filepath.Join(rowRoot, "rows.sqlite"))
	if err != nil {
		t.Fatalf("open row fixture: %v", err)
	}
	if _, err := rowDB.Exec(`CREATE TABLE works (id INTEGER, site TEXT, site_work_id TEXT); INSERT INTO works VALUES (1, NULL, 'work')`); err != nil {
		t.Fatalf("seed invalid work row: %v", err)
	}
	if err := rowScanner.scanNovelFetcherRows(context.Background(), rowDB); err == nil {
		t.Fatal("NULL work row should fail canonical scan")
	}
	if _, err := rowDB.Exec(`DELETE FROM works; INSERT INTO works VALUES (1, 'synthetic', 'work')`); err != nil {
		t.Fatalf("replace work row: %v", err)
	}
	if err := rowScanner.scanNovelFetcherRows(context.Background(), rowDB); err == nil {
		t.Fatal("missing episodes table should fail canonical scan")
	}
	if _, err := rowDB.Exec(`CREATE TABLE episodes (body_path TEXT, content_hash TEXT); INSERT INTO episodes VALUES ('works/episode.json', NULL)`); err != nil {
		t.Fatalf("seed invalid episode row: %v", err)
	}
	if err := rowScanner.scanNovelFetcherRows(context.Background(), rowDB); err == nil {
		t.Fatal("NULL episode row should fail canonical scan")
	}
	if err := rowDB.Close(); err != nil {
		t.Fatalf("close row fixture: %v", err)
	}
}
