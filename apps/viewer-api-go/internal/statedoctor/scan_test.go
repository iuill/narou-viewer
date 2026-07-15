package statedoctor

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"narou-viewer/apps/viewer-api-go/internal/ai/usagemigration"
	"narou-viewer/apps/viewer-api-go/internal/application/readertextcache"
	"narou-viewer/apps/viewer-api-go/internal/characters"
	"narou-viewer/apps/viewer-api-go/internal/extraction"
	"narou-viewer/apps/viewer-api-go/internal/store"
	"narou-viewer/apps/viewer-api-go/internal/terms"

	"gopkg.in/yaml.v3"
	_ "modernc.org/sqlite"
)

func TestScanReportsCrossStoreProblemsWithoutChangingBytes(t *testing.T) {
	dataDir := t.TempDir()
	stateDir := filepath.Join(dataDir, "state")
	stateStore := store.New(dataDir)
	if err := stateStore.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	orphanEpisode := "1"
	if _, err := stateStore.PutReadingState(store.ReadingStatePutInput{ReadingState: store.ReadingState{
		NovelID:              "orphan-novel",
		LastReadEpisodeIndex: &orphanEpisode,
		Position:             1,
	}}); err != nil {
		t.Fatalf("seed orphan reading state: %v", err)
	}
	aiSettingsPath := filepath.Join(stateDir, "ai_generation_settings.yaml")
	if err := os.WriteFile(aiSettingsPath, []byte("schema_version: 2\nshared_providers:\n  openrouter:\n    api_key: synthetic-value\n"), 0o644); err != nil {
		t.Fatalf("write AI settings fixture: %v", err)
	}
	if err := os.Chmod(aiSettingsPath, 0o644); err != nil {
		t.Fatalf("chmod AI settings fixture: %v", err)
	}

	if err := characters.SaveGeneratedSummary(stateDir, "novel-frontier", "1", []characters.GeneratedCharacter{{CanonicalName: "合成人物"}}); err != nil {
		t.Fatalf("seed character frontier: %v", err)
	}
	if err := terms.SaveGeneratedTerms(stateDir, "novel-frontier", "2", []terms.GeneratedTerm{{Term: "合成用語"}}, nil); err != nil {
		t.Fatalf("seed term frontier: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "character_events", "novel-future.yaml"), []byte("schema_version: 99\nnovel_id: novel-future\n"), 0o644); err != nil {
		t.Fatalf("write future character event: %v", err)
	}

	job := extraction.Job{JobID: "job-one", RequestedUpToEpisodeIndex: "1", Status: "completed", CreatedAt: "2026-01-01T00:00:00Z"}
	if err := extraction.SaveJob(stateDir, "novel-job", job); err != nil {
		t.Fatalf("seed job: %v", err)
	}
	indexPath := filepath.Join(stateDir, "extraction_jobs", "index", "novel-job.yaml")
	if err := os.WriteFile(indexPath, []byte("schema_version: 2\nrevision: 1\nnovel_id: novel-job\njob_ids: []\n"), 0o644); err != nil {
		t.Fatalf("write mismatched index: %v", err)
	}

	readerSearchPath := filepath.Join(stateDir, readertextcache.FileName)
	seedSQLiteVersion(t, readerSearchPath, 99)
	seedNovelFetcherDoctorDB(t, dataDir)

	paths := []string{
		filepath.Join(stateDir, "reading_state.yaml"),
		aiSettingsPath,
		filepath.Join(stateDir, "character_events", "novel-future.yaml"),
		indexPath,
		readerSearchPath,
		filepath.Join(dataDir, "novel-fetcher", "library.sqlite"),
	}
	before := readFiles(t, paths)
	report, err := Scan(context.Background(), dataDir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	for _, kind := range []string{
		"legacy_plaintext_api_key",
		"insecure_file_mode",
		"schema_future_unknown",
		"job_index_mismatch",
		"frontier_inversion",
		"orphan_novel_state",
		"cache_version_mismatch",
		"missing_body_file",
		"content_hash_mismatch",
	} {
		if !reportHasKind(report, kind) {
			t.Fatalf("report missing %q: %+v", kind, report.Findings)
		}
	}
	if !report.HasIssues() || report.Summary.Errors == 0 || report.Summary.Warnings == 0 {
		t.Fatalf("unexpected report summary: %+v", report.Summary)
	}
	after := readFiles(t, paths)
	for path, raw := range before {
		if !bytes.Equal(raw, after[path]) {
			t.Fatalf("dry-run changed %s", path)
		}
	}
	if quarantined, _ := filepath.Glob(readerSearchPath + ".unsupported-*"); len(quarantined) != 0 {
		t.Fatalf("dry-run must not quarantine cache: %v", quarantined)
	}
}

func TestApplyOnlyRepairsExplicitDerivedStateFindings(t *testing.T) {
	dataDir := t.TempDir()
	stateDir := filepath.Join(dataDir, "state")
	if err := store.New(dataDir).Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	const novelID = "novel-repair"
	if err := characters.SaveGeneratedSummary(stateDir, novelID, "1", []characters.GeneratedCharacter{{CanonicalName: "合成人物"}}); err != nil {
		t.Fatalf("seed characters: %v", err)
	}
	eventsPath := filepath.Join(stateDir, "character_events", novelID+".yaml")
	profilePath := filepath.Join(stateDir, "character_profiles", novelID+".yaml")
	if err := os.WriteFile(profilePath, []byte("schema_version: 99\nnovel_id: "+novelID+"\n"), 0o644); err != nil {
		t.Fatalf("write future profile: %v", err)
	}
	if err := extraction.SaveJob(stateDir, novelID, extraction.Job{JobID: "job-repair", RequestedUpToEpisodeIndex: "1", Status: "completed", CreatedAt: "2026-01-01T00:00:00Z"}); err != nil {
		t.Fatalf("seed job: %v", err)
	}
	jobPath := filepath.Join(stateDir, "extraction_jobs", "job-repair.yaml")
	indexPath := filepath.Join(stateDir, "extraction_jobs", "index", novelID+".yaml")
	if err := os.WriteFile(indexPath, []byte("schema_version: 2\nrevision: 1\nnovel_id: "+novelID+"\njob_ids: []\n"), 0o644); err != nil {
		t.Fatalf("write mismatched index: %v", err)
	}
	readerSearchPath := filepath.Join(stateDir, readertextcache.FileName)
	seedSQLiteVersion(t, readerSearchPath, 99)

	sourceBefore := readFiles(t, []string{eventsPath, jobPath})
	report, err := Scan(context.Background(), dataDir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	ids := []string{}
	for _, finding := range report.Findings {
		if finding.RepairKind != "" {
			ids = append(ids, finding.ID)
		}
	}
	if len(ids) != 3 {
		t.Fatalf("repairable findings = %d, want 3: %+v", len(ids), report.Findings)
	}
	repaired, err := Apply(context.Background(), dataDir, ids)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(repaired.Applied) != 3 {
		t.Fatalf("applied findings = %+v", repaired.Applied)
	}
	for _, finding := range repaired.Findings {
		if finding.RepairKind != "" {
			t.Fatalf("repairable finding remained: %+v", finding)
		}
	}
	sourceAfter := readFiles(t, []string{eventsPath, jobPath})
	for path, raw := range sourceBefore {
		if !bytes.Equal(raw, sourceAfter[path]) {
			t.Fatalf("derived repair changed source state %s", path)
		}
	}
	for pattern, want := range map[string]int{
		profilePath + ".unsupported-*":  1,
		indexPath + ".rebuild-*":        1,
		readerSearchPath + ".rebuild-*": 1,
	} {
		paths, err := filepath.Glob(pattern)
		if err != nil || len(paths) != want {
			t.Fatalf("quarantine %s = %v err=%v", pattern, paths, err)
		}
	}
	var profileHeader struct {
		SchemaVersion int `yaml:"schema_version"`
	}
	raw, _ := os.ReadFile(profilePath)
	if err := yaml.Unmarshal(raw, &profileHeader); err != nil || profileHeader.SchemaVersion != 1 {
		t.Fatalf("rebuilt profile header = %+v err=%v", profileHeader, err)
	}
}

func TestApplyRejectsDiagnosticOnlyAndStaleFindingIDs(t *testing.T) {
	dataDir := t.TempDir()
	if _, err := Apply(context.Background(), dataDir, nil); err == nil {
		t.Fatal("Apply should require explicit finding IDs")
	}
	report, err := Scan(context.Background(), dataDir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(report.Findings) == 0 {
		t.Fatal("expected inventory findings")
	}
	if _, err := Apply(context.Background(), dataDir, []string{report.Findings[0].ID}); err == nil {
		t.Fatal("Apply should reject diagnostic-only findings")
	}
	if _, err := Apply(context.Background(), dataDir, []string{"finding-does-not-exist"}); err == nil {
		t.Fatal("Apply should reject stale IDs")
	}
}

func TestScanReportsSQLiteIntegrityFailure(t *testing.T) {
	dataDir := t.TempDir()
	stateDir := filepath.Join(dataDir, "state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("mkdir state: %v", err)
	}
	path := filepath.Join(stateDir, "ai_usage.sqlite")
	before := []byte("synthetic invalid sqlite")
	if err := os.WriteFile(path, before, 0o600); err != nil {
		t.Fatalf("write invalid sqlite: %v", err)
	}
	report, err := Scan(context.Background(), dataDir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if !reportHasKind(report, "sqlite_integrity_error") {
		t.Fatalf("integrity finding missing: %+v", report.Findings)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("integrity scan changed bytes: err=%v", err)
	}
}

func TestScanReportsCheckpointSchemaAndSymlinkProblems(t *testing.T) {
	dataDir := t.TempDir()
	checkpointDir := filepath.Join(dataDir, "state", "extraction_jobs", "checkpoints")
	if err := os.MkdirAll(checkpointDir, 0o755); err != nil {
		t.Fatalf("mkdir checkpoints: %v", err)
	}
	for name, raw := range map[string]string{
		"current.json":   `{"schemaVersion":4}`,
		"future.json":    `{"schemaVersion":99}`,
		"malformed.json": `{"schemaVersion":`,
	} {
		if err := os.WriteFile(filepath.Join(checkpointDir, name), []byte(raw), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	if err := os.Symlink("current.json", filepath.Join(checkpointDir, "linked.json")); err != nil {
		t.Fatalf("symlink checkpoint: %v", err)
	}

	report, err := Scan(context.Background(), dataDir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	for _, kind := range []string{"schema_current", "schema_future_unknown", "schema_malformed", "symlink_not_scanned"} {
		if !reportHasKind(report, kind) {
			t.Fatalf("report missing %q: %+v", kind, report.Findings)
		}
	}
}

func TestScanReportsAIUsageCurrentLegacyAndFutureSchemas(t *testing.T) {
	for _, test := range []struct {
		name     string
		version  int
		wantKind string
	}{
		{name: "current", version: 1, wantKind: "schema_current"},
		{name: "legacy_without_ledger", version: 0, wantKind: "schema_legacy"},
		{name: "future", version: 99, wantKind: "schema_future_unknown"},
	} {
		t.Run(test.name, func(t *testing.T) {
			dataDir := t.TempDir()
			path := filepath.Join(dataDir, "state", "ai_usage.sqlite")
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatalf("mkdir state: %v", err)
			}
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatalf("open usage db: %v", err)
			}
			if err := usagemigration.Run(db, path); err != nil {
				t.Fatalf("seed usage schema: %v", err)
			}
			if test.version == 0 {
				if _, err := db.Exec(`DROP TABLE schema_migrations`); err != nil {
					t.Fatalf("drop migration ledger: %v", err)
				}
			} else if test.version > usagemigration.SupportedLatestVersion {
				if _, err := db.Exec(`INSERT INTO schema_migrations(version) VALUES (?)`, test.version); err != nil {
					t.Fatalf("seed future migration: %v", err)
				}
			}
			if err := db.Close(); err != nil {
				t.Fatalf("close usage db: %v", err)
			}
			if err := os.Chmod(path, 0o600); err != nil {
				t.Fatalf("chmod usage db: %v", err)
			}

			report, err := Scan(context.Background(), dataDir)
			if err != nil {
				t.Fatalf("Scan: %v", err)
			}
			if !reportHasSchemaKind(report, "VA-AI-USAGE", test.wantKind) {
				t.Fatalf("report missing %s/%s: %+v", "VA-AI-USAGE", test.wantKind, report.Findings)
			}
		})
	}
}

func TestScanReportsUnknownAISettingsCryptoVersionWithoutReadingCredential(t *testing.T) {
	dataDir := t.TempDir()
	stateDir := filepath.Join(dataDir, "state")
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatalf("mkdir state: %v", err)
	}
	path := filepath.Join(stateDir, "ai_generation_settings.yaml")
	raw := []byte("schema_version: 2\nshared_providers:\n  openrouter:\n    api_key_encrypted: synthetic-ciphertext\n    api_key_version: 99\n")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write settings: %v", err)
	}
	report, err := Scan(context.Background(), dataDir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if !reportHasSchemaKind(report, "VA-AI-SETTINGS-CRYPTO", "crypto_future_unknown") {
		t.Fatalf("crypto future finding missing: %+v", report.Findings)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(raw, after) {
		t.Fatalf("crypto scan changed source bytes: err=%v", err)
	}
}

func TestHumanReportIncludesObservedRecoveryAndRepair(t *testing.T) {
	report := Report{Findings: []Finding{
		{ID: "finding-test", SchemaID: "TEST", Path: "state/test.yaml", Kind: "mismatch", Severity: SeverityWarning, Observed: "2", Supported: "1", RecoveryHint: "synthetic recovery", RepairKind: "synthetic_rebuild"},
		{ID: "finding-clean", SchemaID: "TEST", Path: "state/clean.yaml", Kind: "current", Severity: SeverityInfo},
	}}
	report.finalize()
	output := Human(report)
	for _, fragment := range []string{"observed=2 supported=1", "recovery: synthetic recovery", "--apply --finding finding-test", "summary: inventory=1 warnings=1 errors=0 repairable=1"} {
		if !strings.Contains(output, fragment) {
			t.Fatalf("Human output missing %q: %s", fragment, output)
		}
	}
}

func TestScanReportsNovelFetcherLegacyAndFutureMigrations(t *testing.T) {
	for _, test := range []struct {
		name       string
		withLedger bool
		version    int
		wantKind   string
	}{
		{name: "legacy", wantKind: "schema_legacy"},
		{name: "future", withLedger: true, version: 99, wantKind: "schema_future_unknown"},
	} {
		t.Run(test.name, func(t *testing.T) {
			dataDir := t.TempDir()
			root := filepath.Join(dataDir, "novel-fetcher")
			if err := os.MkdirAll(root, 0o755); err != nil {
				t.Fatalf("mkdir novel-fetcher: %v", err)
			}
			db, err := sql.Open("sqlite", filepath.Join(root, "library.sqlite"))
			if err != nil {
				t.Fatalf("open library: %v", err)
			}
			if _, err := db.Exec(`
				CREATE TABLE works (id INTEGER PRIMARY KEY, site TEXT NOT NULL, site_work_id TEXT NOT NULL);
				CREATE TABLE episodes (body_path TEXT NOT NULL, content_hash TEXT NOT NULL);
			`); err != nil {
				t.Fatalf("seed library: %v", err)
			}
			if test.withLedger {
				if _, err := db.Exec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY); INSERT INTO schema_migrations(version) VALUES (?)`, test.version); err != nil {
					t.Fatalf("seed migration ledger: %v", err)
				}
			}
			if err := db.Close(); err != nil {
				t.Fatalf("close library: %v", err)
			}

			report, err := Scan(context.Background(), dataDir)
			if err != nil {
				t.Fatalf("Scan: %v", err)
			}
			if !reportHasSchemaKind(report, "NF-LIBRARY", test.wantKind) {
				t.Fatalf("report missing NF-LIBRARY/%s: %+v", test.wantKind, report.Findings)
			}
		})
	}
}

func TestScanReportsCanonicalPathSchemaAndReferenceProblems(t *testing.T) {
	dataDir := t.TempDir()
	root := filepath.Join(dataDir, "novel-fetcher")
	episodesDir := filepath.Join(root, "works", "synthetic", "episodes")
	if err := os.MkdirAll(episodesDir, 0o700); err != nil {
		t.Fatalf("mkdir episodes: %v", err)
	}
	for name, raw := range map[string]string{
		"future.json":    `{"schema_version":99}`,
		"malformed.json": `{"schema_version":`,
		"orphan.json":    `{"schema_version":1,"blocks":[]}`,
	} {
		if err := os.WriteFile(filepath.Join(episodesDir, name), []byte(raw), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	outside := filepath.Join(dataDir, "outside.json")
	if err := os.WriteFile(outside, []byte(`{"schema_version":1}`), 0o600); err != nil {
		t.Fatalf("write outside fixture: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(episodesDir, "linked.json")); err != nil {
		t.Fatalf("symlink canonical fixture: %v", err)
	}
	db, err := sql.Open("sqlite", filepath.Join(root, "library.sqlite"))
	if err != nil {
		t.Fatalf("open library: %v", err)
	}
	_, err = db.Exec(`
		CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY);
		INSERT INTO schema_migrations(version) VALUES (1), (2), (3);
		CREATE TABLE works (id INTEGER PRIMARY KEY, site TEXT NOT NULL, site_work_id TEXT NOT NULL);
		CREATE TABLE episodes (body_path TEXT NOT NULL, content_hash TEXT NOT NULL);
		INSERT INTO episodes(body_path, content_hash) VALUES
			('../outside.json', ''),
			('works/synthetic/episodes/future.json', ''),
			('works/synthetic/episodes/malformed.json', ''),
			('works/synthetic/episodes/linked.json', '');
	`)
	if err != nil {
		t.Fatalf("seed library: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close library: %v", err)
	}

	report, err := Scan(context.Background(), dataDir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	for _, kind := range []string{"unsafe_body_path", "schema_unsupported", "schema_malformed", "symlink_not_scanned", "orphan_body_file"} {
		if !reportHasKind(report, kind) {
			t.Fatalf("report missing %q: %+v", kind, report.Findings)
		}
	}
}

func TestScanRefusesSQLiteSymlinkWithoutFollowingIt(t *testing.T) {
	dataDir := t.TempDir()
	stateDir := filepath.Join(dataDir, "state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("mkdir state: %v", err)
	}
	target := filepath.Join(dataDir, "outside.sqlite")
	if err := os.WriteFile(target, []byte("synthetic outside bytes"), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}
	if err := os.Symlink(target, filepath.Join(stateDir, "ai_usage.sqlite")); err != nil {
		t.Fatalf("symlink usage db: %v", err)
	}
	report, err := Scan(context.Background(), dataDir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if !reportHasSchemaKind(report, "VA-AI-USAGE", "sqlite_open_error") || !reportHasKind(report, "sensitive_symlink") {
		t.Fatalf("symlink findings missing: %+v", report.Findings)
	}
	if raw, err := os.ReadFile(target); err != nil || string(raw) != "synthetic outside bytes" {
		t.Fatalf("symlink target changed: raw=%q err=%v", raw, err)
	}
}

func TestSQLiteAndPathHelpersRejectUnsafeInputs(t *testing.T) {
	emptyPath := filepath.Join(t.TempDir(), "empty.sqlite")
	if err := os.WriteFile(emptyPath, nil, 0o600); err != nil {
		t.Fatalf("write empty sqlite: %v", err)
	}
	if db, exists, err := openReadOnlySQLite(emptyPath); err == nil || !exists || db != nil {
		t.Fatalf("empty sqlite should fail closed: db=%v exists=%v err=%v", db, exists, err)
	}
	for _, path := range []string{"", "../outside.json", "/absolute.json"} {
		if clean, ok := safeRelativeStoragePath(path); ok || clean != "" {
			t.Fatalf("unsafe path accepted: input=%q clean=%q", path, clean)
		}
	}
	if clean, ok := safeRelativeStoragePath(" works/synthetic/episode.json "); !ok || clean != filepath.Join("works", "synthetic", "episode.json") {
		t.Fatalf("safe path rejected: clean=%q ok=%v", clean, ok)
	}
	if containsString([]string{"a", "b"}, "missing") {
		t.Fatal("containsString should report a miss")
	}
}

func seedSQLiteVersion(t *testing.T, path string, version int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir sqlite fixture: %v", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open sqlite fixture: %v", err)
	}
	if _, err := db.Exec("CREATE TABLE synthetic_cache (value TEXT); PRAGMA user_version = " + strconv.Itoa(version)); err != nil {
		t.Fatalf("seed sqlite version: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close sqlite fixture: %v", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatalf("chmod sqlite fixture: %v", err)
	}
}

func seedNovelFetcherDoctorDB(t *testing.T, dataDir string) {
	t.Helper()
	root := filepath.Join(dataDir, "novel-fetcher")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir novel-fetcher fixture: %v", err)
	}
	bodyDir := filepath.Join(root, "works", "synthetic", "episodes")
	if err := os.MkdirAll(bodyDir, 0o755); err != nil {
		t.Fatalf("mkdir canonical fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(bodyDir, "2.json"), []byte(`{"schema_version":1,"episode_id":"2","blocks":[]}`), 0o644); err != nil {
		t.Fatalf("write canonical fixture: %v", err)
	}
	db, err := sql.Open("sqlite", filepath.Join(root, "library.sqlite"))
	if err != nil {
		t.Fatalf("open novel-fetcher fixture: %v", err)
	}
	_, err = db.Exec(`
		CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY);
		INSERT INTO schema_migrations(version) VALUES (1), (2), (3);
		CREATE TABLE works (id INTEGER PRIMARY KEY, site TEXT NOT NULL, site_work_id TEXT NOT NULL);
		CREATE TABLE episodes (body_path TEXT NOT NULL, content_hash TEXT NOT NULL);
		INSERT INTO works(id, site, site_work_id) VALUES (1, 'synthetic', 'work-1');
		INSERT INTO episodes(body_path, content_hash) VALUES
			('works/synthetic/episodes/1.json', 'sha256:missing'),
			('works/synthetic/episodes/2.json', 'sha256:does-not-match');
	`)
	if err != nil {
		t.Fatalf("seed novel-fetcher fixture: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close novel-fetcher fixture: %v", err)
	}
}

func readFiles(t *testing.T, paths []string) map[string][]byte {
	t.Helper()
	result := map[string][]byte{}
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		result[path] = raw
	}
	return result
}

func reportHasKind(report Report, kind string) bool {
	for _, finding := range report.Findings {
		if finding.Kind == kind {
			return true
		}
	}
	return false
}

func reportHasSchemaKind(report Report, schemaID string, kind string) bool {
	for _, finding := range report.Findings {
		if finding.SchemaID == schemaID && finding.Kind == kind {
			return true
		}
	}
	return false
}
