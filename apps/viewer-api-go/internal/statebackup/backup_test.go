package statebackup

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"filippo.io/age"

	"narou-viewer/apps/viewer-api-go/internal/ai"
	"narou-viewer/apps/viewer-api-go/internal/characters"
	"narou-viewer/apps/viewer-api-go/internal/extraction"
	"narou-viewer/apps/viewer-api-go/internal/library"
	"narou-viewer/apps/viewer-api-go/internal/publications"
	"narou-viewer/apps/viewer-api-go/internal/state/aisettings"
	"narou-viewer/apps/viewer-api-go/internal/statebarrier"
	"narou-viewer/apps/viewer-api-go/internal/statedoctor"
	"narou-viewer/apps/viewer-api-go/internal/store"
	"narou-viewer/apps/viewer-api-go/internal/terms"

	_ "modernc.org/sqlite"
)

const testPassphrase = "synthetic backup passphrase"

func TestBackupRestoreEncryptedColdGeneration(t *testing.T) {
	dataDir, novelID := seedCleanBackupData(t)
	outputDir := filepath.Join(t.TempDir(), "private-backups")
	recipient, identity := testScryptPair(t)
	createdAt := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	result, err := Backup(context.Background(), BackupOptions{
		DataDir:          dataDir,
		OutputDir:        outputDir,
		ApplicationBuild: "test-build",
		KeyReference:     "local-test-key",
		Recipient:        recipient,
		Now:              func() time.Time { return createdAt },
		GenerationID:     func() (string, error) { return "generation-test", nil },
	})
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}
	archiveInfo, err := os.Stat(result.ArchivePath)
	if err != nil || archiveInfo.Mode().Perm() != 0o600 {
		t.Fatalf("archive mode: info=%v err=%v", archiveInfo, err)
	}
	outputInfo, err := os.Stat(outputDir)
	if err != nil || outputInfo.Mode().Perm() != 0o700 {
		t.Fatalf("output directory mode: info=%v err=%v", outputInfo, err)
	}
	encrypted, err := os.ReadFile(result.ArchivePath)
	if err != nil {
		t.Fatalf("read archive: %v", err)
	}
	for _, forbidden := range []string{testPassphrase, "synthetic reader marker", "local-test-key"} {
		if bytes.Contains(encrypted, []byte(forbidden)) {
			t.Fatalf("encrypted archive exposed %q", forbidden)
		}
	}
	if result.Manifest.FormatVersion != ManifestFormatVersion || result.Manifest.SnapshotMethod != "cold-stop+writer-lock-v1" || result.Manifest.KeyReference != "local-test-key" {
		t.Fatalf("manifest contract: %+v", result.Manifest)
	}
	manifestRaw, err := json.Marshal(result.Manifest)
	if err != nil || bytes.Contains(manifestRaw, []byte(testPassphrase)) {
		t.Fatalf("manifest exposed backup secret: err=%v", err)
	}
	if manifestHasPayloadPath(result.Manifest, "state/character_profiles/") {
		t.Fatalf("derived character profiles must be excluded: %+v", result.Manifest.Files)
	}
	if !manifestHasPayloadPath(result.Manifest, "state/reading_state.yaml") || !manifestHasPayloadPath(result.Manifest, "novel-fetcher/library.sqlite") {
		t.Fatalf("required groups missing files: %+v", result.Manifest.Files)
	}

	readingPath := filepath.Join(dataDir, "state", "reading_state.yaml")
	originalReading, err := os.ReadFile(readingPath)
	if err != nil {
		t.Fatalf("read original reading state: %v", err)
	}
	if err := os.WriteFile(readingPath, []byte("schema_version: 99\n"), 0o600); err != nil {
		t.Fatalf("mutate reading state: %v", err)
	}
	derivedPath := filepath.Join(dataDir, "state", "character_profiles", novelID+".yaml")
	if err := os.WriteFile(derivedPath, []byte("schema_version: 99\n"), 0o600); err != nil {
		t.Fatalf("mutate derived cache: %v", err)
	}
	staleLibrarySHM := filepath.Join(dataDir, "novel-fetcher", "library.sqlite-shm")
	if err := os.WriteFile(staleLibrarySHM, []byte("stale shared memory"), 0o600); err != nil {
		t.Fatalf("write stale library shm: %v", err)
	}
	restored, err := Restore(context.Background(), RestoreOptions{
		DataDir:      dataDir,
		ArchivePath:  result.ArchivePath,
		KeyReference: "local-test-key",
		Identities:   []age.Identity{identity},
	})
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if restored.Manifest.GenerationID != "generation-test" || restored.Report.Summary.Errors != 0 {
		t.Fatalf("restore result: manifest=%+v summary=%+v", restored.Manifest, restored.Report.Summary)
	}
	actualReading, err := os.ReadFile(readingPath)
	if err != nil || !bytes.Equal(actualReading, originalReading) {
		t.Fatalf("reading state not restored: err=%v", err)
	}
	if _, err := os.Stat(derivedPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("derived cache should be removed: %v", err)
	}
	if _, err := os.Stat(staleLibrarySHM); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale library shm should be removed: %v", err)
	}
	if leftovers, _ := filepath.Glob(filepath.Join(dataDir, ".restore-*")); len(leftovers) != 0 {
		t.Fatalf("restore temporary directories remain: %v", leftovers)
	}
	if report, err := statedoctor.Scan(context.Background(), dataDir); err != nil || report.Summary.Errors != 0 {
		t.Fatalf("post-restore doctor: summary=%+v err=%v", report.Summary, err)
	} else {
		returnedJSON, _ := json.Marshal(restored.Report)
		cleanJSON, _ := json.Marshal(report)
		if !bytes.Equal(returnedJSON, cleanJSON) {
			t.Fatalf("returned doctor report differs from cleanup-state scan\nreturned=%s\nclean=%s", returnedJSON, cleanJSON)
		}
	}
}

func TestRestoreRejectsInsecureSensitiveFileMode(t *testing.T) {
	sourceData, _ := seedCleanBackupData(t)
	sourcePath := filepath.Join(sourceData, "state", "ai_usage.sqlite")
	info, err := os.Stat(sourcePath)
	if err != nil {
		t.Fatalf("stat AI usage source: %v", err)
	}
	manifest := validEmptyManifest("insecure-mode-test")
	for index := range manifest.Schemas {
		if manifest.Schemas[index].SchemaID == "VA-AI-USAGE" {
			manifest.Schemas[index] = SchemaRecord{SchemaID: "VA-AI-USAGE", Path: "state/ai_usage.sqlite", Observed: "1", Supported: "1", Status: "schema_current", Group: GroupVAHistory}
		}
	}
	recipient, identity := testScryptPair(t)
	archivePath := filepath.Join(t.TempDir(), "insecure-mode"+ArchiveSuffix)
	if err := writeEncryptedArchive(context.Background(), archivePath, recipient, []sourceFile{{
		absolute: sourcePath,
		record:   FileRecord{Path: "state/ai_usage.sqlite", Group: GroupVAHistory, Size: info.Size(), Mode: 0o644},
	}}, &manifest, time.Now().UTC()); err != nil {
		t.Fatalf("write insecure-mode archive: %v", err)
	}
	targetData, _ := seedCleanBackupData(t)
	if _, err := Restore(context.Background(), RestoreOptions{DataDir: targetData, ArchivePath: archivePath, KeyReference: "local-test-key", Identities: []age.Identity{identity}}); err == nil || !strings.Contains(err.Error(), "insecure_file_mode") {
		t.Fatalf("restore should reject insecure sensitive file mode: %v", err)
	}
}

func TestRestoreRejectsPayloadWhenStagedDoctorFindsManifestMismatch(t *testing.T) {
	dataDir, _ := seedCleanBackupData(t)
	readingPath := filepath.Join(dataDir, "state", "reading_state.yaml")
	liveBefore, err := os.ReadFile(readingPath)
	if err != nil {
		t.Fatalf("read live state: %v", err)
	}
	sourcePath := filepath.Join(t.TempDir(), "future-reading.yaml")
	if err := os.WriteFile(sourcePath, []byte("schema_version: 999\nrevision: 1\nnovels: {}\n"), 0o600); err != nil {
		t.Fatalf("write future source: %v", err)
	}
	info, err := os.Stat(sourcePath)
	if err != nil {
		t.Fatalf("stat future source: %v", err)
	}
	manifest := validEmptyManifest("rollback-test")
	for index := range manifest.Schemas {
		if manifest.Schemas[index].SchemaID == "VA-READING" {
			manifest.Schemas[index] = SchemaRecord{SchemaID: "VA-READING", Path: "state/reading_state.yaml", Observed: "3", Supported: "3", Status: "schema_current", Group: GroupVACore}
		}
	}
	recipient, identity := testScryptPair(t)
	archivePath := filepath.Join(t.TempDir(), "rollback"+ArchiveSuffix)
	if err := writeEncryptedArchive(context.Background(), archivePath, recipient, []sourceFile{{absolute: sourcePath, record: FileRecord{Path: "state/reading_state.yaml", Group: GroupVACore, Size: info.Size(), Mode: 0o600}}}, &manifest, time.Now().UTC()); err != nil {
		t.Fatalf("write synthetic archive: %v", err)
	}
	if _, err := Restore(context.Background(), RestoreOptions{DataDir: dataDir, ArchivePath: archivePath, KeyReference: "local-test-key", Identities: []age.Identity{identity}}); err == nil || !strings.Contains(err.Error(), "staged restore state doctor") {
		t.Fatalf("restore should fail staged state doctor: %v", err)
	}
	liveAfter, err := os.ReadFile(readingPath)
	if err != nil || !bytes.Equal(liveBefore, liveAfter) {
		t.Fatalf("rejected staged restore changed live state: err=%v", err)
	}
	if leftovers, _ := filepath.Glob(filepath.Join(dataDir, ".restore-*")); len(leftovers) != 0 {
		t.Fatalf("failed restore left temporary data: %v", leftovers)
	}
}

func TestBackupRefusesActiveWriterAndLegacyPlaintextCredential(t *testing.T) {
	dataDir, _ := seedCleanBackupData(t)
	outputDir := filepath.Join(t.TempDir(), "backups")
	recipient, _ := testScryptPair(t)
	active, err := statebarrier.AcquireViewerAPI(dataDir)
	if err != nil {
		t.Fatalf("acquire active writer fixture: %v", err)
	}
	_, err = Backup(context.Background(), BackupOptions{DataDir: dataDir, OutputDir: outputDir, KeyReference: "local-test-key", Recipient: recipient})
	if !errors.Is(err, statebarrier.ErrWriterActive) {
		t.Fatalf("active writer backup error = %v", err)
	}
	if err := active.Close(); err != nil {
		t.Fatalf("release active writer: %v", err)
	}

	settingsPath := filepath.Join(dataDir, "state", aisettings.FileName)
	if err := os.WriteFile(settingsPath, []byte("schema_version: 2\nshared_providers:\n  openrouter:\n    api_key: synthetic-value\n"), 0o600); err != nil {
		t.Fatalf("write plaintext credential fixture: %v", err)
	}
	_, err = Backup(context.Background(), BackupOptions{DataDir: dataDir, OutputDir: outputDir, KeyReference: "local-test-key", Recipient: recipient})
	if err == nil || !strings.Contains(err.Error(), "legacy plaintext") {
		t.Fatalf("plaintext credential backup error = %v", err)
	}
	archives, globErr := filepath.Glob(filepath.Join(outputDir, "*"+ArchiveSuffix))
	if globErr != nil || len(archives) != 0 {
		t.Fatalf("refused backup left archive: %v err=%v", archives, globErr)
	}
}

func TestRestoreRejectsTamperingBeforeChangingLiveState(t *testing.T) {
	dataDir, _ := seedCleanBackupData(t)
	recipient, identity := testScryptPair(t)
	result, err := Backup(context.Background(), BackupOptions{
		DataDir: dataDir, OutputDir: filepath.Join(t.TempDir(), "backups"), KeyReference: "local-test-key", Recipient: recipient,
		GenerationID: func() (string, error) { return "tamper-test", nil },
	})
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}
	readingPath := filepath.Join(dataDir, "state", "reading_state.yaml")
	liveBefore, _ := os.ReadFile(readingPath)
	raw, err := os.ReadFile(result.ArchivePath)
	if err != nil {
		t.Fatalf("read archive: %v", err)
	}
	raw[len(raw)-1] ^= 0xff
	if err := os.WriteFile(result.ArchivePath, raw, 0o600); err != nil {
		t.Fatalf("tamper archive: %v", err)
	}
	_, err = Restore(context.Background(), RestoreOptions{DataDir: dataDir, ArchivePath: result.ArchivePath, KeyReference: "local-test-key", Identities: []age.Identity{identity}})
	if err == nil {
		t.Fatal("tampered archive restore should fail")
	}
	liveAfter, _ := os.ReadFile(readingPath)
	if !bytes.Equal(liveBefore, liveAfter) {
		t.Fatal("tampered archive changed live state before preflight completed")
	}
}

func TestValidateManifestRejectsPartialFutureAndMetadataMismatch(t *testing.T) {
	base := validEmptyManifest("validation-test")
	if err := validateManifest(base, map[string]FileRecord{}, "local-test-key"); err != nil {
		t.Fatalf("valid empty manifest: %v", err)
	}
	partial := base
	partial.Groups = append([]GroupRecord{}, base.Groups...)
	partial.Groups[0].Included = false
	if err := validateManifest(partial, map[string]FileRecord{}, "local-test-key"); err == nil {
		t.Fatal("partial manifest should be rejected")
	}
	future := base
	future.Schemas = append([]SchemaRecord{}, base.Schemas...)
	for index := range future.Schemas {
		if future.Schemas[index].SchemaID == "VA-READING" {
			future.Schemas[index] = SchemaRecord{SchemaID: "VA-READING", Path: "state/reading_state.yaml", Observed: "999", Supported: "999", Status: "schema_current", Group: GroupVACore}
		}
	}
	if err := validateManifest(future, map[string]FileRecord{}, "local-test-key"); err == nil {
		t.Fatal("future schema should be rejected")
	}
	if err := validateManifest(base, map[string]FileRecord{}, "different-key"); err == nil {
		t.Fatal("key reference mismatch should be rejected")
	}
	badFormat := base
	badFormat.FormatVersion = 99
	if err := validateManifest(badFormat, map[string]FileRecord{}, "local-test-key"); err == nil {
		t.Fatal("future manifest format should be rejected")
	}
}

func TestWriteEncryptedArchiveCleansPartialAndNeverOverwrites(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "source.yaml")
	if err := os.WriteFile(sourcePath, []byte("schema_version: 3\n"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	info, _ := os.Stat(sourcePath)
	file := sourceFile{absolute: sourcePath, record: FileRecord{Path: "state/reading_state.yaml", Group: GroupVACore, Size: info.Size(), Mode: 0o600}}
	recipient, _ := testScryptPair(t)
	archivePath := filepath.Join(root, "archive"+ArchiveSuffix)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	manifest := validEmptyManifest("cancelled-test")
	if err := writeEncryptedArchive(cancelled, archivePath, recipient, []sourceFile{file}, &manifest, time.Now().UTC()); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled archive error = %v", err)
	}
	if matches, _ := filepath.Glob(archivePath + "*"); len(matches) != 0 {
		t.Fatalf("cancelled archive left files: %v", matches)
	}

	manifest = validEmptyManifest("first-test")
	if err := writeEncryptedArchive(context.Background(), archivePath, recipient, []sourceFile{file}, &manifest, time.Now().UTC()); err != nil {
		t.Fatalf("write first archive: %v", err)
	}
	first, _ := os.ReadFile(archivePath)
	manifest = validEmptyManifest("second-test")
	if err := writeEncryptedArchive(context.Background(), archivePath, recipient, []sourceFile{file}, &manifest, time.Now().UTC()); err == nil {
		t.Fatal("archive writer should not overwrite an existing generation")
	}
	second, _ := os.ReadFile(archivePath)
	if !bytes.Equal(first, second) {
		t.Fatal("existing archive bytes changed")
	}
	if _, err := os.Stat(archivePath + ".partial"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed overwrite left partial file: %v", err)
	}
}

func TestPruneArchivesKeepsNewestAndHonorsAge(t *testing.T) {
	directory := t.TempDir()
	now := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)
	for index, age := range []time.Duration{time.Hour, 48 * time.Hour, 72 * time.Hour} {
		path := filepath.Join(directory, "narou-viewer-test-"+string(rune('a'+index))+ArchiveSuffix)
		if err := os.WriteFile(path, []byte("encrypted fixture"), 0o600); err != nil {
			t.Fatalf("write archive: %v", err)
		}
		if err := os.Chtimes(path, now.Add(-age), now.Add(-age)); err != nil {
			t.Fatalf("chtimes archive: %v", err)
		}
	}
	if err := os.WriteFile(filepath.Join(directory, "unrelated.txt"), []byte("keep"), 0o600); err != nil {
		t.Fatalf("write unrelated file: %v", err)
	}
	removed, err := PruneArchives(directory, RetentionPolicy{KeepGenerations: 1, MaxAge: 24 * time.Hour, Now: func() time.Time { return now }})
	if err != nil || len(removed) != 2 {
		t.Fatalf("PruneArchives removed=%v err=%v", removed, err)
	}
	if _, err := os.Stat(filepath.Join(directory, "unrelated.txt")); err != nil {
		t.Fatalf("unrelated file removed: %v", err)
	}
	if _, err := PruneArchives(directory, RetentionPolicy{}); err == nil {
		t.Fatal("invalid retention policy should fail")
	}
}

func TestArchiveWriterRejectsManifestOverRestoreLimitBeforePublishing(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "oversized"+ArchiveSuffix)
	files := make([]sourceFile, 0, 30_000)
	missingSource := filepath.Join(t.TempDir(), "missing")
	for index := 0; index < cap(files); index++ {
		relative := "state/extraction_jobs/job-" + strconv.Itoa(index) + ".yaml"
		files = append(files, sourceFile{
			absolute: missingSource,
			record: FileRecord{
				Path:  relative,
				Group: GroupVAExtraction,
				Size:  0,
				Mode:  0o600,
			},
		})
	}
	recipient, _ := testScryptPair(t)
	manifest := validEmptyManifest("oversized-manifest")
	err := writeEncryptedArchive(context.Background(), archivePath, recipient, files, &manifest, time.Now().UTC())
	if err == nil || !strings.Contains(err.Error(), "manifest exceeds size limit") {
		t.Fatalf("oversized manifest error = %v", err)
	}
	for _, path := range []string{archivePath, archivePath + ".partial"} {
		if _, statErr := os.Lstat(path); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("oversized archive left %s: %v", path, statErr)
		}
	}
}

func TestSelectionAndArchivePathGuards(t *testing.T) {
	dataDir := t.TempDir()
	worksDir := filepath.Join(dataDir, "novel-fetcher", "works")
	if err := os.MkdirAll(worksDir, 0o700); err != nil {
		t.Fatalf("mkdir works: %v", err)
	}
	target := filepath.Join(dataDir, "outside")
	if err := os.WriteFile(target, []byte("synthetic"), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}
	if err := os.Symlink(target, filepath.Join(worksDir, "linked.json")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	if _, err := collectPayloadFiles(dataDir); err == nil {
		t.Fatal("payload selection should reject a symlink")
	}
	for name, want := range map[string]bool{
		ManifestName:                       true,
		"payload/state/reading_state.yaml": true,
		"payload/../outside":               false,
		"/absolute":                        false,
		"payload\\windows":                 false,
	} {
		if got := safeArchiveEntryName(name); got != want {
			t.Fatalf("safeArchiveEntryName(%q) = %v, want %v", name, got, want)
		}
	}
}

func seedCleanBackupData(t *testing.T) (string, string) {
	t.Helper()
	dataDir := filepath.Join(t.TempDir(), "data")
	stateDir := filepath.Join(dataDir, "state")
	stateStore := store.New(dataDir)
	if err := stateStore.Initialize(); err != nil {
		t.Fatalf("initialize state: %v", err)
	}
	if err := publications.NewService(stateDir).Ensure(); err != nil {
		t.Fatalf("ensure publications: %v", err)
	}
	if err := characters.EnsureStateDirs(stateDir); err != nil {
		t.Fatalf("ensure character dirs: %v", err)
	}
	if err := terms.EnsureStateDirs(stateDir); err != nil {
		t.Fatalf("ensure term dirs: %v", err)
	}
	if err := extraction.EnsureStateDirs(stateDir); err != nil {
		t.Fatalf("ensure extraction dirs: %v", err)
	}
	work := library.Work{Site: "synthetic", SiteWorkID: "work-1"}
	novelID := library.NovelID(work)
	novelFetcherDir := filepath.Join(dataDir, "novel-fetcher")
	if err := os.MkdirAll(filepath.Join(novelFetcherDir, "works"), 0o700); err != nil {
		t.Fatalf("mkdir works: %v", err)
	}
	db, err := sql.Open("sqlite", filepath.Join(novelFetcherDir, "library.sqlite"))
	if err != nil {
		t.Fatalf("open library fixture: %v", err)
	}
	_, err = db.Exec(`
		CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY);
		INSERT INTO schema_migrations(version) VALUES (1), (2), (3);
		CREATE TABLE works (id INTEGER PRIMARY KEY, site TEXT NOT NULL, site_work_id TEXT NOT NULL);
		CREATE TABLE episodes (body_path TEXT NOT NULL, content_hash TEXT NOT NULL);
		INSERT INTO works(id, site, site_work_id) VALUES (1, 'synthetic', 'work-1');
	`)
	if err != nil {
		t.Fatalf("seed library fixture: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close library fixture: %v", err)
	}
	episode := "1"
	if _, err := stateStore.PutReadingState(store.ReadingStatePutInput{ReadingState: store.ReadingState{NovelID: novelID, LastReadEpisodeIndex: &episode, Position: 7}}); err != nil {
		t.Fatalf("seed reading state: %v", err)
	}
	if err := characters.SaveGeneratedSummary(stateDir, novelID, "1", []characters.GeneratedCharacter{{CanonicalName: "合成人物"}}); err != nil {
		t.Fatalf("seed character state: %v", err)
	}
	if err := terms.SaveGeneratedTerms(stateDir, novelID, "1", []terms.GeneratedTerm{{Term: "合成用語"}}, nil); err != nil {
		t.Fatalf("seed term state: %v", err)
	}
	if err := extraction.SaveJob(stateDir, novelID, extraction.Job{JobID: "job-synthetic", RequestedUpToEpisodeIndex: "1", Status: "completed", CreatedAt: "2026-01-01T00:00:00Z"}); err != nil {
		t.Fatalf("seed extraction job: %v", err)
	}
	if err := ai.SaveUsageRun(filepath.Join(stateDir, "ai_usage.sqlite"), ai.UsageRun{
		RunID: "run-synthetic", Feature: "synthetic", WorkflowName: "synthetic", Status: "completed", StartedAt: "2026-01-01T00:00:00Z", FinishedAt: "2026-01-01T00:00:01Z", GenerationMode: "local",
	}); err != nil {
		t.Fatalf("seed AI usage: %v", err)
	}
	return dataDir, novelID
}

func testScryptPair(t *testing.T) (*age.ScryptRecipient, *age.ScryptIdentity) {
	t.Helper()
	recipient, err := age.NewScryptRecipient(testPassphrase)
	if err != nil {
		t.Fatalf("NewScryptRecipient: %v", err)
	}
	recipient.SetWorkFactor(10)
	identity, err := age.NewScryptIdentity(testPassphrase)
	if err != nil {
		t.Fatalf("NewScryptIdentity: %v", err)
	}
	return recipient, identity
}

func manifestHasPayloadPath(manifest Manifest, prefix string) bool {
	for _, file := range manifest.Files {
		if strings.HasPrefix(file.Path, prefix) {
			return true
		}
	}
	return false
}

func requiredMissingSchemaRecords() []SchemaRecord {
	records := []SchemaRecord{}
	for _, schemaID := range []string{
		"NF-LIBRARY",
		"VA-READING",
		"VA-BOOKMARKS",
		"VA-PREFERENCES",
		"VA-NOVEL-SETTINGS",
		"VA-AI-SETTINGS",
		"VA-PUBLICATIONS",
		"VA-CHAR-EVENTS",
		"VA-TERM-PROFILES",
		"VA-EXTRACTION-JOBS",
		"VA-EXTRACTION-CHECKPOINT",
		"VA-AI-USAGE",
	} {
		group, _ := groupForSchema(schemaID)
		records = append(records, SchemaRecord{SchemaID: schemaID, Path: "missing", Observed: "missing", Status: "missing", Group: group})
	}
	return records
}

func validEmptyManifest(generation string) Manifest {
	return Manifest{
		FormatVersion:  ManifestFormatVersion,
		GenerationID:   generation,
		SnapshotMethod: "cold-stop+writer-lock-v1",
		Encryption:     "age-v1",
		KeyReference:   "local-test-key",
		Groups: []GroupRecord{
			{ID: GroupNFCanonical, Included: true},
			{ID: GroupVACore, Included: true},
			{ID: GroupVAExtraction, Included: true},
			{ID: GroupVAHistory, Included: true},
			{ID: GroupVACache, Included: false},
			{ID: GroupSecrets, Included: false},
		},
		Schemas: requiredMissingSchemaRecords(),
		Files:   []FileRecord{},
	}
}
