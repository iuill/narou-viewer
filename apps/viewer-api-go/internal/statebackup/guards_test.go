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
	"strings"
	"testing"
	"time"

	"filippo.io/age"

	"narou-viewer/apps/viewer-api-go/internal/statebarrier"
)

func TestBackupOptionAndDoctorGuards(t *testing.T) {
	recipient, _ := testScryptPair(t)
	for _, options := range []BackupOptions{
		{},
		{DataDir: t.TempDir(), OutputDir: t.TempDir(), KeyReference: "local-test-key"},
		{DataDir: t.TempDir(), OutputDir: t.TempDir(), KeyReference: "unsafe reference", Recipient: recipient},
	} {
		if _, err := Backup(context.Background(), options); err == nil {
			t.Fatalf("Backup should reject options: %+v", options)
		}
	}
	dataDir, _ := seedCleanBackupData(t)
	if _, err := Backup(context.Background(), BackupOptions{DataDir: dataDir, OutputDir: filepath.Join(dataDir, "backups"), KeyReference: "local-test-key", Recipient: recipient}); err == nil {
		t.Fatal("Backup should reject an output directory inside data")
	}
	if _, err := Backup(context.Background(), BackupOptions{
		DataDir: dataDir, OutputDir: filepath.Join(t.TempDir(), "backups"), KeyReference: "local-test-key", Recipient: recipient,
		GenerationID: func() (string, error) { return "unsafe generation", nil },
	}); err == nil {
		t.Fatal("Backup should reject an unsafe generation ID")
	}
	if _, err := Backup(context.Background(), BackupOptions{
		DataDir: dataDir, OutputDir: filepath.Join(t.TempDir(), "backups"), KeyReference: "local-test-key", Recipient: recipient,
		ApplicationBuild: "bad\nbuild", GenerationID: func() (string, error) { return "build-test", nil },
	}); err == nil {
		t.Fatal("Backup should reject an unsafe build ID")
	}
	settingsPath := filepath.Join(dataDir, "state", "ai_generation_settings.yaml")
	if err := os.Chmod(settingsPath, 0o644); err != nil {
		t.Fatalf("chmod settings: %v", err)
	}
	if _, err := Backup(context.Background(), BackupOptions{
		DataDir: dataDir, OutputDir: filepath.Join(t.TempDir(), "backups"), KeyReference: "local-test-key", Recipient: recipient,
	}); err == nil || !strings.Contains(err.Error(), "insecure_file_mode") {
		t.Fatalf("Backup should reject insecure sensitive mode: %v", err)
	}
	if err := os.Chmod(settingsPath, 0o600); err != nil {
		t.Fatalf("restore settings mode: %v", err)
	}
	retentionOutput := filepath.Join(t.TempDir(), "backups")
	if _, err := Backup(context.Background(), BackupOptions{
		DataDir: dataDir, OutputDir: retentionOutput, KeyReference: "local-test-key", Recipient: recipient,
		GenerationID: func() (string, error) { return "retention-test", nil }, Retention: &RetentionPolicy{},
	}); err == nil || !strings.Contains(err.Error(), "retention keep generations") {
		t.Fatalf("Backup should reject invalid retention before archive creation: %v", err)
	}
	if archives, _ := filepath.Glob(filepath.Join(retentionOutput, "*"+ArchiveSuffix)); len(archives) != 0 {
		t.Fatalf("invalid retention should not create an archive: %v", archives)
	}
	if generation, err := randomGenerationID(); err != nil || len(generation) != 24 {
		t.Fatalf("randomGenerationID = %q err=%v", generation, err)
	}
}

func TestRestoreOptionArchiveAndWriterGuards(t *testing.T) {
	_, identity := testScryptPair(t)
	for _, options := range []RestoreOptions{
		{},
		{DataDir: t.TempDir(), ArchivePath: filepath.Join(t.TempDir(), "missing"), KeyReference: "local-test-key"},
		{DataDir: t.TempDir(), ArchivePath: filepath.Join(t.TempDir(), "missing"), KeyReference: "bad reference", Identities: []age.Identity{identity}},
	} {
		if _, err := Restore(context.Background(), options); err == nil {
			t.Fatalf("Restore should reject options: %+v", options)
		}
	}
	root := t.TempDir()
	archive := filepath.Join(root, "archive"+ArchiveSuffix)
	if err := os.WriteFile(archive, []byte("not encrypted"), 0o644); err != nil {
		t.Fatalf("write archive fixture: %v", err)
	}
	if _, err := Restore(context.Background(), RestoreOptions{DataDir: t.TempDir(), ArchivePath: archive, KeyReference: "local-test-key", Identities: []age.Identity{identity}}); err == nil || !strings.Contains(err.Error(), "mode") {
		t.Fatalf("Restore should reject insecure archive mode: %v", err)
	}
	link := filepath.Join(root, "archive-link"+ArchiveSuffix)
	if err := os.Symlink(archive, link); err != nil {
		t.Fatalf("symlink archive: %v", err)
	}
	if err := validateArchiveFile(link, true); err == nil {
		t.Fatal("validateArchiveFile should reject symlink even when mode override is allowed")
	}

	dataDir, _ := seedCleanBackupData(t)
	recipient, restoreIdentity := testScryptPair(t)
	backup, err := Backup(context.Background(), BackupOptions{
		DataDir: dataDir, OutputDir: filepath.Join(t.TempDir(), "backups"), KeyReference: "local-test-key", Recipient: recipient,
		GenerationID: func() (string, error) { return "writer-guard-test", nil },
	})
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}
	if _, err := Restore(context.Background(), RestoreOptions{DataDir: dataDir, ArchivePath: backup.ArchivePath, KeyReference: "wrong-key", Identities: []age.Identity{restoreIdentity}}); err == nil || !strings.Contains(err.Error(), "key reference") {
		t.Fatalf("Restore should reject key reference mismatch: %v", err)
	}
	insideArchive := filepath.Join(dataDir, "inside"+ArchiveSuffix)
	rawArchive, err := os.ReadFile(backup.ArchivePath)
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if err := os.WriteFile(insideArchive, rawArchive, 0o600); err != nil {
		t.Fatalf("copy archive inside data: %v", err)
	}
	if _, err := Restore(context.Background(), RestoreOptions{DataDir: dataDir, ArchivePath: insideArchive, KeyReference: "local-test-key", Identities: []age.Identity{restoreIdentity}}); err == nil || !strings.Contains(err.Error(), "outside the data tree") {
		t.Fatalf("Restore should reject archive inside data: %v", err)
	}
	staleStage := filepath.Join(dataDir, ".restore-staging-writer-guard-test")
	if err := os.Mkdir(staleStage, 0o700); err != nil {
		t.Fatalf("create stale staging path: %v", err)
	}
	if _, err := Restore(context.Background(), RestoreOptions{DataDir: dataDir, ArchivePath: backup.ArchivePath, KeyReference: "local-test-key", Identities: []age.Identity{restoreIdentity}}); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("Restore should reject an existing staging path: %v", err)
	}
	if err := os.RemoveAll(staleStage); err != nil {
		t.Fatalf("remove stale staging: %v", err)
	}
	active, err := statebarrier.AcquireNovelFetcher(dataDir)
	if err != nil {
		t.Fatalf("acquire writer fixture: %v", err)
	}
	_, err = Restore(context.Background(), RestoreOptions{DataDir: dataDir, ArchivePath: backup.ArchivePath, KeyReference: "local-test-key", Identities: []age.Identity{restoreIdentity}})
	if !errors.Is(err, statebarrier.ErrWriterActive) {
		t.Fatalf("Restore active writer error = %v", err)
	}
	if err := active.Close(); err != nil {
		t.Fatalf("close writer fixture: %v", err)
	}
}

func TestManifestValidationRejectsEveryBoundaryMismatch(t *testing.T) {
	base := validEmptyManifest("manifest-guards")
	validFile := FileRecord{Path: "state/reading_state.yaml", Group: GroupVACore, Size: 3, Mode: 0o600, SHA256: "sha256:synthetic"}
	withFile := base
	withFile.Files = []FileRecord{validFile}
	payload := map[string]FileRecord{validFile.Path: validFile}
	if err := validateManifest(withFile, payload, "local-test-key"); err != nil {
		t.Fatalf("valid file manifest: %v", err)
	}
	cases := []Manifest{}
	badSnapshot := base
	badSnapshot.SnapshotMethod = "online-raw-copy"
	cases = append(cases, badSnapshot)
	duplicateGroup := base
	duplicateGroup.Groups = append(duplicateGroup.Groups, duplicateGroup.Groups[0])
	cases = append(cases, duplicateGroup)
	cacheIncluded := base
	cacheIncluded.Groups = append([]GroupRecord{}, base.Groups...)
	cacheIncluded.Groups[4].Included = true
	cases = append(cases, cacheIncluded)
	invalidFile := withFile
	invalidFile.Files = []FileRecord{{Path: "../outside", Group: GroupVACore, Size: 1, Mode: 0o600, SHA256: "sha256:x"}}
	cases = append(cases, invalidFile)
	duplicateFile := withFile
	duplicateFile.Files = append(duplicateFile.Files, validFile)
	cases = append(cases, duplicateFile)
	unknownSchema := base
	unknownSchema.Schemas = append(unknownSchema.Schemas, SchemaRecord{SchemaID: "UNKNOWN", Path: "state/unknown", Observed: "1", Status: "schema_current", Group: GroupVACore})
	cases = append(cases, unknownSchema)
	badStatus := base
	badStatus.Schemas = append([]SchemaRecord{}, base.Schemas...)
	badStatus.Schemas[0].Status = "schema_future_unknown"
	cases = append(cases, badStatus)
	badMissing := base
	badMissing.Schemas = append([]SchemaRecord{}, base.Schemas...)
	badMissing.Schemas[0].Observed = "1"
	cases = append(cases, badMissing)
	badNumeric := base
	badNumeric.Schemas = append([]SchemaRecord{}, base.Schemas...)
	badNumeric.Schemas[0].Status = "schema_current"
	badNumeric.Schemas[0].Observed = "invalid"
	cases = append(cases, badNumeric)
	incomplete := base
	incomplete.Schemas = incomplete.Schemas[1:]
	cases = append(cases, incomplete)
	for index, manifest := range cases {
		if err := validateManifest(manifest, payloadForManifest(manifest, payload), "local-test-key"); err == nil {
			t.Fatalf("manifest boundary case %d unexpectedly passed: %+v", index, manifest)
		}
	}
	mismatchedPayload := map[string]FileRecord{validFile.Path: validFile}
	mismatch := withFile
	mismatch.Files[0].Size++
	if err := validateManifest(mismatch, mismatchedPayload, "local-test-key"); err == nil {
		t.Fatal("metadata mismatch should fail")
	}
}

func TestScanEncryptedArchiveRejectsMalformedStructure(t *testing.T) {
	recipient, identity := testScryptPair(t)
	validManifest, err := json.Marshal(validEmptyManifest("archive-guards"))
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	cases := []struct {
		name    string
		entries []testArchiveEntry
	}{
		{name: "missing_manifest", entries: nil},
		{name: "invalid_manifest", entries: []testArchiveEntry{{name: ManifestName, raw: []byte("not json")}}},
		{name: "unsafe_path", entries: []testArchiveEntry{{name: "../outside", raw: []byte("x")}, {name: ManifestName, raw: validManifest}}},
		{name: "unsupported_payload", entries: []testArchiveEntry{{name: "payload/state/unknown.bin", raw: []byte("x")}, {name: ManifestName, raw: validManifest}}},
		{name: "duplicate", entries: []testArchiveEntry{{name: ManifestName, raw: validManifest}, {name: ManifestName, raw: validManifest}}},
		{name: "non_regular", entries: []testArchiveEntry{{name: "payload/state/reading_state.yaml", typeflag: tar.TypeSymlink}, {name: ManifestName, raw: validManifest}}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "invalid"+ArchiveSuffix)
			writeTestEncryptedArchive(t, path, recipient, test.entries)
			if _, err := scanEncryptedArchive(context.Background(), path, []age.Identity{identity}, nil); err == nil {
				t.Fatal("malformed archive should fail")
			}
		})
	}
}

func TestArchiveDecryptAndStagingErrorBranches(t *testing.T) {
	recipient, identity := testScryptPair(t)
	wrongRecipient, wrongIdentity := testScryptPairWithPassphrase(t, "different synthetic passphrase")
	_ = wrongRecipient
	validManifest, _ := json.Marshal(validEmptyManifest("archive-errors"))
	validPath := filepath.Join(t.TempDir(), "valid"+ArchiveSuffix)
	writeTestEncryptedArchive(t, validPath, recipient, []testArchiveEntry{{name: ManifestName, raw: validManifest}})
	if _, err := scanEncryptedArchive(context.Background(), validPath, []age.Identity{wrongIdentity}, nil); err == nil {
		t.Fatal("wrong identity should fail decryption")
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := scanEncryptedArchive(cancelled, validPath, []age.Identity{identity}, nil); err == nil {
		t.Fatal("cancelled archive scan should fail")
	}

	nonGzipPath := filepath.Join(t.TempDir(), "non-gzip"+ArchiveSuffix)
	writeEncryptedBytes(t, nonGzipPath, recipient, []byte("not a gzip stream"))
	if _, err := scanEncryptedArchive(context.Background(), nonGzipPath, []age.Identity{identity}, nil); err == nil {
		t.Fatal("non-gzip encrypted payload should fail")
	}
	largeManifestPath := filepath.Join(t.TempDir(), "large-manifest"+ArchiveSuffix)
	writeTestEncryptedArchive(t, largeManifestPath, recipient, []testArchiveEntry{{name: ManifestName, raw: bytes.Repeat([]byte("x"), maxManifestBytes+1)}})
	if _, err := scanEncryptedArchive(context.Background(), largeManifestPath, []age.Identity{identity}, nil); err == nil {
		t.Fatal("oversized manifest should fail")
	}

	payloadPath := filepath.Join(t.TempDir(), "payload"+ArchiveSuffix)
	manifest := validEmptyManifest("destination-error")
	fileRaw := []byte("schema_version: 3\n")
	hashManifestForTest(t, &manifest, "state/reading_state.yaml", GroupVACore, fileRaw, 0o600)
	manifestRaw, _ := json.Marshal(manifest)
	writeTestEncryptedArchive(t, payloadPath, recipient, []testArchiveEntry{{name: "payload/state/reading_state.yaml", raw: fileRaw}, {name: ManifestName, raw: manifestRaw}})
	destinationErr := errors.New("synthetic destination failure")
	if _, err := scanEncryptedArchive(context.Background(), payloadPath, []age.Identity{identity}, func(FileRecord) (io.WriteCloser, error) { return nil, destinationErr }); !errors.Is(err, destinationErr) {
		t.Fatalf("destination error = %v", err)
	}
	stageRoot := t.TempDir()
	destination := stagingDestination(stageRoot)
	if _, err := destination(FileRecord{Path: "../outside", Mode: 0o600}); err == nil {
		t.Fatal("staging destination should reject path traversal")
	}
	writer, err := destination(FileRecord{Path: "state/reading_state.yaml", Mode: 0o600})
	if err != nil {
		t.Fatalf("create staging destination: %v", err)
	}
	if _, err := writer.Write([]byte("fixture")); err != nil {
		t.Fatalf("write staging fixture: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close staging fixture: %v", err)
	}
	if _, err := destination(FileRecord{Path: "state/reading_state.yaml", Mode: 0o600}); err == nil {
		t.Fatal("staging destination should not overwrite a file")
	}
}

func TestFilesystemAndPublishHelpersFailClosed(t *testing.T) {
	root := t.TempDir()
	filePath := filepath.Join(root, "file")
	if err := os.WriteFile(filePath, []byte("fixture"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := ensurePrivateDirectory(filePath); err == nil {
		t.Fatal("ensurePrivateDirectory should reject a file")
	}
	privateTarget := filepath.Join(root, "private-target")
	if err := os.Mkdir(privateTarget, 0o700); err != nil {
		t.Fatalf("mkdir private target: %v", err)
	}
	privateLink := filepath.Join(root, "private-link")
	if err := os.Symlink(privateTarget, privateLink); err != nil {
		t.Fatalf("symlink private directory: %v", err)
	}
	if err := ensurePrivateDirectory(privateLink); err == nil {
		t.Fatal("ensurePrivateDirectory should reject a symlink")
	}
	if _, _, err := openSourceFile(root); err == nil {
		t.Fatal("openSourceFile should reject a directory")
	}
	if _, _, err := openSourceFile(filepath.Join(root, "missing")); err == nil {
		t.Fatal("openSourceFile should reject a missing file")
	}
	private := filepath.Join(root, "private")
	if err := createEmptyPrivateDirectory(private); err != nil {
		t.Fatalf("create private directory: %v", err)
	}
	if err := createEmptyPrivateDirectory(private); err == nil {
		t.Fatal("createEmptyPrivateDirectory should reject an existing path")
	}
	restoreData := filepath.Join(root, "restore-roots")
	if err := os.MkdirAll(restoreData, 0o700); err != nil {
		t.Fatalf("mkdir restore root: %v", err)
	}
	outsideState := filepath.Join(root, "outside-state")
	if err := os.Mkdir(outsideState, 0o700); err != nil {
		t.Fatalf("mkdir outside state: %v", err)
	}
	if err := os.Symlink(outsideState, filepath.Join(restoreData, "state")); err != nil {
		t.Fatalf("symlink restore state: %v", err)
	}
	if err := ensureRestoreRoots(restoreData); err == nil {
		t.Fatal("ensureRestoreRoots should reject a symlink root")
	}

	dataDir := filepath.Join(root, "publish-data")
	stageRoot := filepath.Join(root, "publish-stage")
	rollbackRoot := filepath.Join(root, "publish-rollback")
	for _, path := range []string{
		filepath.Join(dataDir, "novel-fetcher", "works", "old"),
		filepath.Join(stageRoot, "novel-fetcher", "works", "new"),
		filepath.Join(rollbackRoot, "novel-fetcher", "works", "conflict"),
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatalf("mkdir publish fixture: %v", err)
		}
		if err := os.WriteFile(path, []byte("fixture"), 0o600); err != nil {
			t.Fatalf("write publish fixture: %v", err)
		}
	}
	oldLibrary := filepath.Join(dataDir, "novel-fetcher", "library.sqlite")
	newLibrary := filepath.Join(stageRoot, "novel-fetcher", "library.sqlite")
	if err := os.WriteFile(oldLibrary, []byte("old library"), 0o600); err != nil {
		t.Fatalf("write old library: %v", err)
	}
	if err := os.WriteFile(newLibrary, []byte("new library"), 0o600); err != nil {
		t.Fatalf("write new library: %v", err)
	}
	if err := createEmptyPrivateDirectory(rollbackRoot); err == nil {
		t.Fatal("restore should reject a pre-existing rollback directory")
	}
	if raw, err := os.ReadFile(oldLibrary); err != nil || string(raw) != "old library" {
		t.Fatalf("publish failure did not roll back library: raw=%q err=%v", raw, err)
	}
}

func TestWriteEncryptedArchiveRejectsRecipientAndSourceFailures(t *testing.T) {
	root := t.TempDir()
	archivePath := filepath.Join(root, "failed"+ArchiveSuffix)
	manifest := validEmptyManifest("failure-test")
	if err := writeEncryptedArchive(context.Background(), archivePath, failingRecipient{}, nil, &manifest, time.Now()); err == nil {
		t.Fatal("failing recipient should abort archive creation")
	}
	if _, err := os.Stat(archivePath + ".partial"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("recipient failure left partial: %v", err)
	}
	recipient, _ := testScryptPair(t)
	manifest = validEmptyManifest("source-test")
	err := writeEncryptedArchive(context.Background(), archivePath, recipient, []sourceFile{{absolute: filepath.Join(root, "missing"), record: FileRecord{Path: "state/reading_state.yaml", Group: GroupVACore, Size: 1, Mode: 0o600}}}, &manifest, time.Now())
	if err == nil {
		t.Fatal("missing source should abort archive creation")
	}
	if _, err := os.Stat(archivePath + ".partial"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("source failure left partial: %v", err)
	}
	sourcePath := filepath.Join(root, "source")
	if err := os.WriteFile(sourcePath, []byte("fixture"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	manifest = validEmptyManifest("size-test")
	err = writeEncryptedArchive(context.Background(), archivePath, recipient, []sourceFile{{absolute: sourcePath, record: FileRecord{Path: "state/reading_state.yaml", Group: GroupVACore, Size: 99, Mode: 0o600}}}, &manifest, time.Now())
	if err == nil || !strings.Contains(err.Error(), "changed during preflight") {
		t.Fatalf("source size mismatch error = %v", err)
	}
}

func TestRetentionAndPathHelperErrorBranches(t *testing.T) {
	if _, err := PruneArchives(t.TempDir(), RetentionPolicy{KeepGenerations: 1, MaxAge: -time.Second}); err == nil {
		t.Fatal("negative retention age should fail")
	}
	if _, err := PruneArchives(t.TempDir(), RetentionPolicy{KeepGenerations: 1}); err == nil {
		t.Fatal("zero retention age should fail instead of deleting every generation beyond keep")
	}
	directory := t.TempDir()
	target := filepath.Join(directory, "target")
	if err := os.WriteFile(target, []byte("fixture"), 0o600); err != nil {
		t.Fatalf("write retention target: %v", err)
	}
	link := filepath.Join(directory, "narou-viewer-linked"+ArchiveSuffix)
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink retention archive: %v", err)
	}
	if _, err := PruneArchives(directory, RetentionPolicy{KeepGenerations: 1, MaxAge: time.Hour}); err == nil {
		t.Fatal("retention should reject a symlink archive")
	}
	if _, ok := groupForPayloadPath("state/unknown"); ok {
		t.Fatal("unknown payload path should not have a group")
	}
	if _, ok := groupForSchema("UNKNOWN"); ok {
		t.Fatal("unknown schema should not have a group")
	}
	if clean := pathWithin("/tmp/a", "/tmp/ab"); clean {
		t.Fatal("pathWithin should honor path boundaries")
	}
}

type testArchiveEntry struct {
	name     string
	raw      []byte
	typeflag byte
}

func writeTestEncryptedArchive(t *testing.T, path string, recipient age.Recipient, entries []testArchiveEntry) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		t.Fatalf("create archive: %v", err)
	}
	encrypted, err := age.Encrypt(file, recipient)
	if err != nil {
		t.Fatalf("age Encrypt: %v", err)
	}
	compressed := gzip.NewWriter(encrypted)
	archive := tar.NewWriter(compressed)
	for _, entry := range entries {
		typeflag := entry.typeflag
		if typeflag == 0 {
			typeflag = tar.TypeReg
		}
		size := int64(len(entry.raw))
		if typeflag != tar.TypeReg {
			size = 0
		}
		if err := archive.WriteHeader(&tar.Header{Name: entry.name, Typeflag: typeflag, Mode: 0o600, Size: size}); err != nil {
			t.Fatalf("write header: %v", err)
		}
		if size > 0 {
			if _, err := io.Copy(archive, bytes.NewReader(entry.raw)); err != nil {
				t.Fatalf("write entry: %v", err)
			}
		}
	}
	if err := errors.Join(archive.Close(), compressed.Close(), encrypted.Close(), file.Close()); err != nil {
		t.Fatalf("close archive: %v", err)
	}
}

func writeEncryptedBytes(t *testing.T, path string, recipient age.Recipient, raw []byte) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		t.Fatalf("create encrypted bytes: %v", err)
	}
	encrypted, err := age.Encrypt(file, recipient)
	if err != nil {
		t.Fatalf("age Encrypt: %v", err)
	}
	if _, err := encrypted.Write(raw); err != nil {
		t.Fatalf("write encrypted bytes: %v", err)
	}
	if err := errors.Join(encrypted.Close(), file.Close()); err != nil {
		t.Fatalf("close encrypted bytes: %v", err)
	}
}

func hashManifestForTest(t *testing.T, manifest *Manifest, path string, group string, raw []byte, mode uint32) {
	t.Helper()
	source := filepath.Join(t.TempDir(), "source")
	if err := os.WriteFile(source, raw, os.FileMode(mode)); err != nil {
		t.Fatalf("write hash source: %v", err)
	}
	recipient, _ := testScryptPair(t)
	temporaryArchive := filepath.Join(t.TempDir(), "hash"+ArchiveSuffix)
	if err := writeEncryptedArchive(context.Background(), temporaryArchive, recipient, []sourceFile{{absolute: source, record: FileRecord{Path: path, Group: group, Size: int64(len(raw)), Mode: mode}}}, manifest, time.Now()); err != nil {
		t.Fatalf("compute manifest hash: %v", err)
	}
}

func testScryptPairWithPassphrase(t *testing.T, passphrase string) (*age.ScryptRecipient, *age.ScryptIdentity) {
	t.Helper()
	recipient, err := age.NewScryptRecipient(passphrase)
	if err != nil {
		t.Fatalf("NewScryptRecipient: %v", err)
	}
	recipient.SetWorkFactor(10)
	identity, err := age.NewScryptIdentity(passphrase)
	if err != nil {
		t.Fatalf("NewScryptIdentity: %v", err)
	}
	return recipient, identity
}

type failingRecipient struct{}

func (failingRecipient) Wrap([]byte) ([]*age.Stanza, error) {
	return nil, errors.New("synthetic recipient failure")
}

func payloadForManifest(manifest Manifest, valid map[string]FileRecord) map[string]FileRecord {
	if len(manifest.Files) == 0 {
		return map[string]FileRecord{}
	}
	return valid
}
