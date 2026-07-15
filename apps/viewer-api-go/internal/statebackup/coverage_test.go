package statebackup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"filippo.io/age"

	"narou-viewer/apps/viewer-api-go/internal/statedoctor"
)

func TestBackupCoversOuterDirectoryGenerationAndRetentionBoundaries(t *testing.T) {
	dataDir, _ := seedCleanBackupData(t)
	recipient, _ := testScryptPair(t)
	if _, err := Backup(context.Background(), BackupOptions{
		DataDir: dataDir, OutputDir: filepath.Dir(dataDir), KeyReference: "local-test-key", Recipient: recipient,
	}); err == nil {
		t.Fatal("backup output containing the data tree should fail")
	}
	blockedOutput := filepath.Join(t.TempDir(), "blocked-output")
	if err := os.WriteFile(blockedOutput, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("write blocked output: %v", err)
	}
	if _, err := Backup(context.Background(), BackupOptions{
		DataDir: dataDir, OutputDir: blockedOutput, KeyReference: "local-test-key", Recipient: recipient,
	}); err == nil {
		t.Fatal("backup should reject a file output directory")
	}
	generationErr := errors.New("synthetic generation failure")
	if _, err := Backup(context.Background(), BackupOptions{
		DataDir: dataDir, OutputDir: filepath.Join(t.TempDir(), "backups"), KeyReference: "local-test-key", Recipient: recipient,
		GenerationID: func() (string, error) { return "", generationErr },
	}); !errors.Is(err, generationErr) {
		t.Fatalf("generation error = %v", err)
	}

	outputDir := filepath.Join(t.TempDir(), "backups")
	if err := os.MkdirAll(outputDir, 0o700); err != nil {
		t.Fatalf("mkdir retention output: %v", err)
	}
	oldArchive := filepath.Join(outputDir, "narou-viewer-old"+ArchiveSuffix)
	if err := os.WriteFile(oldArchive, []byte("synthetic encrypted archive"), 0o600); err != nil {
		t.Fatalf("write old archive: %v", err)
	}
	oldTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(oldArchive, oldTime, oldTime); err != nil {
		t.Fatalf("age old archive: %v", err)
	}
	result, err := Backup(context.Background(), BackupOptions{
		DataDir: dataDir, OutputDir: outputDir, KeyReference: "local-test-key", Recipient: recipient,
		Now: func() time.Time { return oldTime.Add(48 * time.Hour) }, GenerationID: func() (string, error) { return "retention-success", nil },
		Retention: &RetentionPolicy{KeepGenerations: 1, MaxAge: time.Hour},
	})
	if err != nil || len(result.Pruned) != 1 || result.Pruned[0] != oldArchive {
		t.Fatalf("retained backup result=%+v err=%v", result, err)
	}
}

func TestWriteEncryptedArchiveCoversSizeHeaderAndMidCopyFailures(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "source")
	if err := os.WriteFile(path, bytes.Repeat([]byte("x"), 4096), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat source: %v", err)
	}
	recipient, _ := testScryptPair(t)
	file := sourceFile{absolute: path, record: FileRecord{Path: "state/reading_state.yaml", Group: GroupVACore, Size: info.Size() + 1, Mode: 0o600}}
	manifest := validEmptyManifest("size-changed")
	if err := writeEncryptedArchive(context.Background(), filepath.Join(root, "size"+ArchiveSuffix), recipient, []sourceFile{file}, &manifest, time.Now()); err == nil {
		t.Fatal("changed source size should fail")
	}

	file.record.Size = info.Size()
	file.record.Path = "state/reading\x00state.yaml"
	manifest = validEmptyManifest("invalid-header")
	if err := writeEncryptedArchive(context.Background(), filepath.Join(root, "header"+ArchiveSuffix), recipient, []sourceFile{file}, &manifest, time.Now()); err == nil {
		t.Fatal("invalid tar header name should fail")
	}

	file.record.Path = "state/reading_state.yaml"
	manifest = validEmptyManifest("mid-copy-cancel")
	ctx := &cancelOnReadContext{Context: context.Background()}
	if err := writeEncryptedArchive(ctx, filepath.Join(root, "cancel"+ArchiveSuffix), recipient, []sourceFile{file}, &manifest, time.Now()); !errors.Is(err, context.Canceled) {
		t.Fatalf("mid-copy cancellation error = %v", err)
	}
}

type cancelOnReadContext struct {
	context.Context
	calls int
}

func (c *cancelOnReadContext) Err() error {
	c.calls++
	if c.calls > 1 {
		return context.Canceled
	}
	return nil
}

func TestArchiveScannerCoversTarSizeDestinationAndTrailingFailures(t *testing.T) {
	recipient, identity := testScryptPair(t)

	malformedTar := filepath.Join(t.TempDir(), "malformed-tar"+ArchiveSuffix)
	var malformedCompressed bytes.Buffer
	gzipWriter := gzip.NewWriter(&malformedCompressed)
	if _, err := gzipWriter.Write([]byte("not a tar stream")); err != nil {
		t.Fatalf("write malformed tar: %v", err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatalf("close malformed gzip: %v", err)
	}
	writeEncryptedBytes(t, malformedTar, recipient, malformedCompressed.Bytes())
	if _, err := scanEncryptedArchive(context.Background(), malformedTar, []age.Identity{identity}, nil); err == nil {
		t.Fatal("malformed tar stream should fail")
	}

	oversized := filepath.Join(t.TempDir(), "oversized"+ArchiveSuffix)
	writeOversizedHeaderArchive(t, oversized, recipient)
	if _, err := scanEncryptedArchive(context.Background(), oversized, []age.Identity{identity}, nil); err == nil {
		t.Fatal("oversized uncompressed archive should fail")
	}

	manifest := validEmptyManifest("close-error")
	payload := []byte("schema_version: 3\n")
	hashManifestForTest(t, &manifest, "state/reading_state.yaml", GroupVACore, payload, 0o600)
	manifestRaw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal close-error manifest: %v", err)
	}
	payloadArchive := filepath.Join(t.TempDir(), "close-error"+ArchiveSuffix)
	writeTestEncryptedArchive(t, payloadArchive, recipient, []testArchiveEntry{
		{name: "payload/state/reading_state.yaml", raw: payload},
		{name: ManifestName, raw: manifestRaw},
	})
	closeFailure := errors.New("synthetic destination close failure")
	if _, err := scanEncryptedArchive(context.Background(), payloadArchive, []age.Identity{identity}, func(FileRecord) (io.WriteCloser, error) {
		return &closeErrorWriter{closeErr: closeFailure}, nil
	}); !errors.Is(err, closeFailure) {
		t.Fatalf("destination close error = %v", err)
	}

	validManifest, _ := json.Marshal(validEmptyManifest("trailing-data"))
	var trailingPlaintext bytes.Buffer
	gzipStream := gzip.NewWriter(&trailingPlaintext)
	tarStream := tar.NewWriter(gzipStream)
	if err := tarStream.WriteHeader(&tar.Header{Name: ManifestName, Mode: 0o600, Size: int64(len(validManifest)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatalf("write trailing manifest header: %v", err)
	}
	if _, err := tarStream.Write(validManifest); err != nil {
		t.Fatalf("write trailing manifest: %v", err)
	}
	if err := tarStream.Close(); err != nil {
		t.Fatalf("close trailing tar: %v", err)
	}
	if err := gzipStream.Close(); err != nil {
		t.Fatalf("close trailing gzip: %v", err)
	}
	trailingPlaintext.WriteString("unauthenticated-to-gzip-member")
	trailingArchive := filepath.Join(t.TempDir(), "trailing"+ArchiveSuffix)
	writeEncryptedBytes(t, trailingArchive, recipient, trailingPlaintext.Bytes())
	if _, err := scanEncryptedArchive(context.Background(), trailingArchive, []age.Identity{identity}, nil); err == nil {
		t.Fatal("trailing plaintext inside authenticated age payload should fail archive framing")
	}
}

type closeErrorWriter struct {
	bytes.Buffer
	closeErr error
}

func (w *closeErrorWriter) Close() error { return w.closeErr }

func writeOversizedHeaderArchive(t *testing.T, path string, recipient age.Recipient) {
	t.Helper()
	output, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		t.Fatalf("open oversized archive: %v", err)
	}
	encrypted, err := age.Encrypt(output, recipient)
	if err != nil {
		t.Fatalf("encrypt oversized archive: %v", err)
	}
	compressed := gzip.NewWriter(encrypted)
	tarWriter := tar.NewWriter(compressed)
	if err := tarWriter.WriteHeader(&tar.Header{Name: "payload/state/reading_state.yaml", Mode: 0o600, Size: maxUncompressedArchiveBytes + 1, Typeflag: tar.TypeReg}); err != nil {
		t.Fatalf("write oversized header: %v", err)
	}
	_ = tarWriter.Close()
	if err := compressed.Close(); err != nil {
		t.Fatalf("close oversized gzip: %v", err)
	}
	if err := encrypted.Close(); err != nil {
		t.Fatalf("close oversized encryption: %v", err)
	}
	if err := output.Close(); err != nil {
		t.Fatalf("close oversized archive: %v", err)
	}
}

func TestRestoreDetectsArchiveSwapAndExistingRollbackGeneration(t *testing.T) {
	recipient, identity := testScryptPair(t)
	root := t.TempDir()
	archive := filepath.Join(root, "archive"+ArchiveSuffix)
	replacement := filepath.Join(root, "replacement"+ArchiveSuffix)
	manifestA, _ := json.Marshal(validEmptyManifest("generation-a"))
	manifestB, _ := json.Marshal(validEmptyManifest("generation-b"))
	writeTestEncryptedArchive(t, archive, recipient, []testArchiveEntry{{name: ManifestName, raw: manifestA}})
	writeTestEncryptedArchive(t, replacement, recipient, []testArchiveEntry{{name: ManifestName, raw: manifestB}})
	swapping := &swapAfterUnwrapIdentity{Identity: identity, target: archive, replacement: replacement}
	dataDir := filepath.Join(t.TempDir(), "data")
	if _, err := Restore(context.Background(), RestoreOptions{DataDir: dataDir, ArchivePath: archive, KeyReference: "local-test-key", Identities: []age.Identity{swapping}}); err == nil {
		t.Fatal("archive generation swap should fail")
	}

	stableArchive := filepath.Join(root, "stable"+ArchiveSuffix)
	writeTestEncryptedArchive(t, stableArchive, recipient, []testArchiveEntry{{name: ManifestName, raw: manifestA}})
	stableData := filepath.Join(t.TempDir(), "data")
	rollback := filepath.Join(stableData, ".restore-rollback-generation-a")
	if err := os.MkdirAll(rollback, 0o700); err != nil {
		t.Fatalf("mkdir stale rollback: %v", err)
	}
	if _, err := Restore(context.Background(), RestoreOptions{DataDir: stableData, ArchivePath: stableArchive, KeyReference: "local-test-key", Identities: []age.Identity{identity}}); err == nil {
		t.Fatal("existing rollback generation should fail")
	}
}

type swapAfterUnwrapIdentity struct {
	age.Identity
	once        sync.Once
	target      string
	replacement string
}

func (i *swapAfterUnwrapIdentity) Unwrap(stanzas []*age.Stanza) ([]byte, error) {
	key, err := i.Identity.Unwrap(stanzas)
	if err == nil {
		i.once.Do(func() {
			_ = os.Rename(i.replacement, i.target)
		})
	}
	return key, err
}

func TestRestoreFilesystemHelpersCoverBlockedParentsAndRollbackErrors(t *testing.T) {
	root := t.TempDir()
	blocked := filepath.Join(root, "blocked")
	if err := os.WriteFile(blocked, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("write blocked parent: %v", err)
	}
	if err := ensureRestoreRoots(filepath.Join(blocked, "data")); err == nil {
		t.Fatal("ensureRestoreRoots should fail below a file")
	}
	if err := createEmptyPrivateDirectory(filepath.Join(blocked, "temporary")); err == nil {
		t.Fatal("createEmptyPrivateDirectory should fail below a file")
	}
	if err := prepareStagingLayout(blocked); err == nil {
		t.Fatal("prepareStagingLayout should fail with a file stage root")
	}
	if _, err := stagingDestination(blocked)(FileRecord{Path: "state/reading_state.yaml", Mode: 0o600}); err == nil {
		t.Fatal("staging destination should fail with a file root")
	}

	dataDir := filepath.Join(root, "rollback-data")
	rollbackRoot := filepath.Join(root, "rollback")
	if err := os.WriteFile(filepath.Join(root, "rollback-data"), []byte("blocked data root"), 0o600); err != nil {
		t.Fatalf("write blocked rollback data: %v", err)
	}
	err := rollbackRestoreTransaction(context.Background(), dataDir, &restoreTransaction{
		StageDirectory:    "stage",
		RollbackDirectory: filepath.Base(rollbackRoot),
		Actions:           []restoreTransactionAction{{Relative: "novel-fetcher/library.sqlite", HadOld: true}},
	})
	if err == nil {
		t.Fatal("rollback should report a blocked destination parent")
	}

	publishData := filepath.Join(root, "publish-data")
	publishStage := filepath.Join(root, "publish-stage")
	publishRollback := filepath.Join(root, "publish-rollback")
	if err := os.WriteFile(filepath.Join(publishData, "novel-fetcher"), []byte("blocked"), 0o600); err != nil {
		if err := os.MkdirAll(publishData, 0o700); err != nil {
			t.Fatalf("mkdir publish data: %v", err)
		}
		if err := os.WriteFile(filepath.Join(publishData, "novel-fetcher"), []byte("blocked"), 0o600); err != nil {
			t.Fatalf("write blocked publish parent: %v", err)
		}
	}
	transaction := newRestoreTransaction("blocked-parent")
	transaction.StageDirectory = filepath.Base(publishStage)
	transaction.RollbackDirectory = filepath.Base(publishRollback)
	if err := buildRestoreTransactionPlan(publishData, &transaction); err == nil {
		t.Fatal("publish planning should fail when destination parent is a file")
	}
}

func TestPayloadSelectionRejectsInvalidRootsAndExactStateFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "novel-fetcher"), []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("write blocked novel-fetcher: %v", err)
	}
	if _, err := collectPayloadFiles(root); err == nil {
		t.Fatal("payload selection should reject an unreadable exact canonical path")
	}

	directoryLibraryRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(directoryLibraryRoot, "novel-fetcher", "library.sqlite"), 0o700); err != nil {
		t.Fatalf("mkdir directory library: %v", err)
	}
	if _, err := collectPayloadFiles(directoryLibraryRoot); err == nil {
		t.Fatal("payload selection should reject a directory exact file")
	}

	fileRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(fileRoot, "novel-fetcher"), 0o700); err != nil {
		t.Fatalf("mkdir novel-fetcher: %v", err)
	}
	if err := os.WriteFile(filepath.Join(fileRoot, "novel-fetcher", "works"), []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("write works root: %v", err)
	}
	if _, err := collectPayloadFiles(fileRoot); err == nil {
		t.Fatal("payload selection should reject a regular-file works root")
	}

	coreRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(coreRoot, "state", "reading_state.yaml"), 0o700); err != nil {
		t.Fatalf("mkdir directory state file: %v", err)
	}
	if _, err := collectPayloadFiles(coreRoot); err == nil {
		t.Fatal("payload selection should reject a directory exact state file")
	}

	if _, err := scanEncryptedArchive(context.Background(), filepath.Join(t.TempDir(), "missing"), nil, nil); err == nil {
		t.Fatal("archive scanner should reject a missing source")
	}
	if finding, blocked := blockingDoctorFinding(statedoctor.Report{Findings: []statedoctor.Finding{{Severity: statedoctor.SeverityError, Kind: "synthetic"}}}); !blocked || finding.Kind != "synthetic" {
		t.Fatalf("error doctor finding should block backup: finding=%+v blocked=%v", finding, blocked)
	}
	if err := validateManifest(validEmptyManifest("count-mismatch"), map[string]FileRecord{"state/reading_state.yaml": {}}, "local-test-key"); err == nil {
		t.Fatal("payload count mismatch should fail")
	}
}

func TestBackupPropagatesCredentialInspectionAndArchiveCreationFailures(t *testing.T) {
	recipient, _ := testScryptPair(t)
	malformedData, _ := seedCleanBackupData(t)
	settingsPath := filepath.Join(malformedData, "state", "ai_generation_settings.yaml")
	if err := os.WriteFile(settingsPath, []byte("[malformed"), 0o600); err != nil {
		t.Fatalf("write malformed settings: %v", err)
	}
	if _, err := Backup(context.Background(), BackupOptions{
		DataDir: malformedData, OutputDir: filepath.Join(t.TempDir(), "backups"), KeyReference: "local-test-key", Recipient: recipient,
	}); err == nil {
		t.Fatal("backup should fail when credential state cannot be inspected")
	}

	dataDir, _ := seedCleanBackupData(t)
	outputDir := filepath.Join(t.TempDir(), "backups")
	if err := os.MkdirAll(outputDir, 0o700); err != nil {
		t.Fatalf("mkdir output: %v", err)
	}
	createdAt := time.Date(2026, 7, 15, 1, 2, 3, 0, time.UTC)
	generation := "preexisting-partial"
	archive := filepath.Join(outputDir, "narou-viewer-"+createdAt.Format("20060102T150405Z")+"-"+generation+ArchiveSuffix)
	if err := os.WriteFile(archive+".partial", []byte("existing partial"), 0o600); err != nil {
		t.Fatalf("write existing partial: %v", err)
	}
	if _, err := Backup(context.Background(), BackupOptions{
		DataDir: dataDir, OutputDir: outputDir, KeyReference: "local-test-key", Recipient: recipient,
		Now: func() time.Time { return createdAt }, GenerationID: func() (string, error) { return generation, nil },
	}); err == nil {
		t.Fatal("backup should propagate archive creation failure")
	}
}

func TestRetentionCoversMissingDirectoryEqualTimesAndFreshCandidates(t *testing.T) {
	if _, err := PruneArchives(filepath.Join(t.TempDir(), "missing"), RetentionPolicy{KeepGenerations: 1}); err == nil {
		t.Fatal("retention should reject a missing directory")
	}
	directory := t.TempDir()
	now := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)
	for _, name := range []string{"narou-viewer-a" + ArchiveSuffix, "narou-viewer-b" + ArchiveSuffix} {
		path := filepath.Join(directory, name)
		if err := os.WriteFile(path, []byte("synthetic encrypted archive"), 0o600); err != nil {
			t.Fatalf("write archive: %v", err)
		}
		if err := os.Chtimes(path, now, now); err != nil {
			t.Fatalf("set equal archive time: %v", err)
		}
	}
	removed, err := PruneArchives(directory, RetentionPolicy{KeepGenerations: 1, MaxAge: time.Hour, Now: func() time.Time { return now }})
	if err != nil || len(removed) != 0 {
		t.Fatalf("fresh equal-time archives should be retained: removed=%v err=%v", removed, err)
	}
	if err := syncDirectory(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("syncDirectory should reject a missing directory")
	}
}
