package statebackup

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"filippo.io/age"
	"golang.org/x/sys/unix"

	"narou-viewer/apps/viewer-api-go/internal/application/readertextcache"
	"narou-viewer/apps/viewer-api-go/internal/publications"
	"narou-viewer/apps/viewer-api-go/internal/state/aisettings"
	"narou-viewer/apps/viewer-api-go/internal/state/bookmarks"
	"narou-viewer/apps/viewer-api-go/internal/state/novelsettings"
	"narou-viewer/apps/viewer-api-go/internal/state/preferences"
	"narou-viewer/apps/viewer-api-go/internal/state/readingstate"
	"narou-viewer/apps/viewer-api-go/internal/statebarrier"
	"narou-viewer/apps/viewer-api-go/internal/statedoctor"
)

const maxManifestBytes = 4 << 20
const maxArchiveFiles = 1_000_000
const maxUncompressedArchiveBytes int64 = 1 << 40

type scannedArchive struct {
	manifest Manifest
	files    map[string]FileRecord
}

type payloadDestination func(record FileRecord) (io.WriteCloser, error)

func Restore(ctx context.Context, options RestoreOptions) (result RestoreResult, resultErr error) {
	dataDir := filepath.Clean(strings.TrimSpace(options.DataDir))
	archivePath := filepath.Clean(strings.TrimSpace(options.ArchivePath))
	if dataDir == "." || archivePath == "." {
		return RestoreResult{}, errors.New("data directory and archive path are required")
	}
	if len(options.Identities) == 0 {
		return RestoreResult{}, errors.New("at least one age identity is required")
	}
	if !referencePattern.MatchString(options.KeyReference) {
		return RestoreResult{}, errors.New("key reference must be a non-secret identifier using safe characters")
	}
	if err := validateArchiveFile(archivePath, options.AllowInsecureArchive); err != nil {
		return RestoreResult{}, err
	}
	preflight, err := scanEncryptedArchive(ctx, archivePath, options.Identities, nil)
	if err != nil {
		return RestoreResult{}, fmt.Errorf("restore archive preflight: %w", err)
	}
	if err := validateManifest(preflight.manifest, preflight.files, options.KeyReference); err != nil {
		return RestoreResult{}, fmt.Errorf("restore manifest preflight: %w", err)
	}
	if pathWithin(dataDir, archivePath) {
		return RestoreResult{}, errors.New("restore archive must be stored outside the data tree")
	}
	locks, err := statebarrier.AcquireWriters(dataDir)
	if err != nil {
		return RestoreResult{}, fmt.Errorf("cold restore requires viewer-api and novel-fetcher to be stopped: %w", err)
	}
	defer locks.Close()
	if err := ensureRestoreRoots(dataDir); err != nil {
		return RestoreResult{}, err
	}
	if _, err := recoverRestoreTransactionLocked(ctx, dataDir); err != nil {
		return RestoreResult{}, fmt.Errorf("recover interrupted restore transaction: %w", err)
	}

	transaction := newRestoreTransaction(preflight.manifest.GenerationID)
	stageRoot := filepath.Join(dataDir, transaction.StageDirectory)
	rollbackRoot := filepath.Join(dataDir, transaction.RollbackDirectory)
	for _, path := range []string{stageRoot, rollbackRoot} {
		if _, err := os.Lstat(path); err == nil {
			return RestoreResult{}, fmt.Errorf("restore temporary path already exists without a transaction journal: %s", path)
		} else if !errors.Is(err, os.ErrNotExist) {
			return RestoreResult{}, err
		}
	}
	if err := beginRestoreTransaction(dataDir, &transaction); err != nil {
		return RestoreResult{}, err
	}
	transactionActive := true
	defer func() {
		if transactionActive {
			_, recoveryErr := recoverRestoreTransactionLocked(context.Background(), dataDir)
			resultErr = errors.Join(resultErr, recoveryErr)
		}
	}()
	if err := createEmptyPrivateDirectory(stageRoot); err != nil {
		return RestoreResult{}, err
	}
	if err := prepareStagingLayout(stageRoot); err != nil {
		return RestoreResult{}, err
	}
	staged, err := scanEncryptedArchive(ctx, archivePath, options.Identities, stagingDestination(stageRoot))
	if err != nil {
		return RestoreResult{}, fmt.Errorf("decrypt restore staging: %w", err)
	}
	if err := validateManifest(staged.manifest, staged.files, options.KeyReference); err != nil {
		return RestoreResult{}, fmt.Errorf("staged restore manifest: %w", err)
	}
	if staged.manifest.GenerationID != preflight.manifest.GenerationID {
		return RestoreResult{}, errors.New("archive manifest changed between preflight and staging")
	}
	if err := syncRestoreTree(stageRoot); err != nil {
		return RestoreResult{}, fmt.Errorf("sync restore staging tree: %w", err)
	}
	stagedReport, err := statedoctor.Scan(ctx, stageRoot)
	if err != nil {
		return RestoreResult{}, fmt.Errorf("staged restore state doctor: %w", err)
	}
	if finding, blocked := firstDoctorError(stagedReport); blocked {
		return RestoreResult{}, fmt.Errorf("staged restore state doctor rejected payload: %s %s %s", finding.SchemaID, finding.Path, finding.Kind)
	}
	if err := createEmptyPrivateDirectory(rollbackRoot); err != nil {
		return RestoreResult{}, err
	}
	if err := buildRestoreTransactionPlan(dataDir, &transaction); err != nil {
		return RestoreResult{}, err
	}
	if err := publishRestoreTransaction(ctx, dataDir, &transaction); err != nil {
		return RestoreResult{}, err
	}
	report, scanErr := statedoctor.Scan(ctx, dataDir)
	if scanErr != nil {
		return RestoreResult{}, fmt.Errorf("post-restore state doctor: %w", scanErr)
	}
	if finding, blocked := firstDoctorError(report); blocked {
		return RestoreResult{}, fmt.Errorf("post-restore state doctor rejected restored generation: %s %s %s", finding.SchemaID, finding.Path, finding.Kind)
	}
	if err := commitRestoreTransaction(dataDir, &transaction); err != nil {
		return RestoreResult{}, err
	}
	transactionActive = false
	return RestoreResult{Manifest: staged.manifest, Report: report}, nil
}

func scanEncryptedArchive(ctx context.Context, archivePath string, identities []age.Identity, destination payloadDestination) (scannedArchive, error) {
	archive, _, err := openSourceFile(archivePath)
	if err != nil {
		return scannedArchive{}, err
	}
	defer archive.Close()
	decrypted, err := age.Decrypt(archive, identities...)
	if err != nil {
		return scannedArchive{}, errors.New("age decryption failed")
	}
	bufferedDecrypted := bufio.NewReader(contextAwareReader(ctx, decrypted))
	compressed, err := gzip.NewReader(bufferedDecrypted)
	if err != nil {
		return scannedArchive{}, errors.New("encrypted payload is not a gzip stream")
	}
	compressed.Multistream(false)
	tarReader := tar.NewReader(compressed)
	result := scannedArchive{files: map[string]FileRecord{}}
	seenEntries := map[string]bool{}
	manifestSeen := false
	entryCount := 0
	var uncompressedBytes int64
	for {
		if err := ctx.Err(); err != nil {
			_ = compressed.Close()
			return scannedArchive{}, err
		}
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			_ = compressed.Close()
			return scannedArchive{}, err
		}
		entryCount++
		if entryCount > maxArchiveFiles {
			_ = compressed.Close()
			return scannedArchive{}, errors.New("archive contains too many entries")
		}
		if header.Typeflag != tar.TypeReg || header.Size < 0 || !safeArchiveEntryName(header.Name) {
			_ = compressed.Close()
			return scannedArchive{}, fmt.Errorf("unsupported archive entry: %s", header.Name)
		}
		if header.Size > maxUncompressedArchiveBytes-uncompressedBytes {
			_ = compressed.Close()
			return scannedArchive{}, errors.New("archive uncompressed payload exceeds size limit")
		}
		uncompressedBytes += header.Size
		if seenEntries[header.Name] {
			_ = compressed.Close()
			return scannedArchive{}, fmt.Errorf("duplicate archive entry: %s", header.Name)
		}
		seenEntries[header.Name] = true
		if header.Name == ManifestName {
			if header.Size > maxManifestBytes {
				_ = compressed.Close()
				return scannedArchive{}, errors.New("manifest exceeds size limit")
			}
			raw, err := io.ReadAll(io.LimitReader(tarReader, maxManifestBytes+1))
			if err != nil || int64(len(raw)) != header.Size {
				_ = compressed.Close()
				return scannedArchive{}, errors.Join(err, errors.New("manifest size mismatch"))
			}
			if err := json.Unmarshal(raw, &result.manifest); err != nil {
				_ = compressed.Close()
				return scannedArchive{}, errors.New("manifest is not valid JSON")
			}
			manifestSeen = true
			continue
		}
		relative := strings.TrimPrefix(header.Name, "payload/")
		group, ok := groupForPayloadPath(relative)
		if !strings.HasPrefix(header.Name, "payload/") || !ok {
			_ = compressed.Close()
			return scannedArchive{}, fmt.Errorf("payload path is outside supported consistency groups: %s", header.Name)
		}
		record := FileRecord{Path: relative, Group: group, Size: header.Size, Mode: uint32(header.Mode) & 0o777}
		hash := sha256.New()
		writer := io.Writer(hash)
		var output io.WriteCloser
		if destination != nil {
			output, err = destination(record)
			if err != nil {
				_ = compressed.Close()
				return scannedArchive{}, err
			}
			writer = io.MultiWriter(hash, output)
		}
		written, copyErr := io.CopyN(writer, contextAwareReader(ctx, tarReader), header.Size)
		var closeErr error
		if output != nil {
			closeErr = output.Close()
		}
		if copyErr != nil || closeErr != nil || written != header.Size {
			_ = compressed.Close()
			return scannedArchive{}, errors.Join(copyErr, closeErr, fmt.Errorf("archive payload size mismatch: %s", relative))
		}
		record.SHA256 = "sha256:" + hex.EncodeToString(hash.Sum(nil))
		result.files[relative] = record
	}
	if _, err := io.Copy(io.Discard, compressed); err != nil {
		_ = compressed.Close()
		return scannedArchive{}, err
	}
	if err := compressed.Close(); err != nil {
		return scannedArchive{}, err
	}
	if trailing, err := io.Copy(io.Discard, bufferedDecrypted); err != nil || trailing != 0 {
		return scannedArchive{}, errors.New("archive has trailing or unauthenticated encrypted data")
	}
	if !manifestSeen {
		return scannedArchive{}, errors.New("archive manifest is missing")
	}
	return result, nil
}

func validateManifest(manifest Manifest, payload map[string]FileRecord, expectedKeyReference string) error {
	if manifest.FormatVersion != ManifestFormatVersion {
		return fmt.Errorf("unsupported manifest format %d", manifest.FormatVersion)
	}
	if !referencePattern.MatchString(manifest.GenerationID) || manifest.SnapshotMethod != "cold-stop+writer-lock-v1" || manifest.Encryption != "age-v1" {
		return errors.New("manifest snapshot or encryption contract is unsupported")
	}
	if manifest.KeyReference != expectedKeyReference {
		return errors.New("manifest key reference does not match the requested backup key")
	}
	groups := map[string]bool{}
	for _, group := range manifest.Groups {
		if _, duplicate := groups[group.ID]; duplicate {
			return fmt.Errorf("duplicate manifest group: %s", group.ID)
		}
		groups[group.ID] = group.Included
	}
	for _, required := range requiredGroups {
		if !groups[required] {
			return fmt.Errorf("partial restore is not supported; required group is missing: %s", required)
		}
	}
	if groups[GroupVACache] || groups[GroupSecrets] {
		return errors.New("manifest must exclude VA-CACHE and SECRETS")
	}
	manifestFiles := map[string]FileRecord{}
	for _, file := range manifest.Files {
		group, ok := groupForPayloadPath(file.Path)
		if !ok || group != file.Group || file.Size < 0 || file.SHA256 == "" || file.Mode > 0o777 {
			return fmt.Errorf("invalid manifest file record: %s", file.Path)
		}
		if _, duplicate := manifestFiles[file.Path]; duplicate {
			return fmt.Errorf("duplicate manifest file record: %s", file.Path)
		}
		manifestFiles[file.Path] = file
	}
	if len(manifestFiles) != len(payload) {
		return errors.New("manifest and payload file counts differ")
	}
	for path, expected := range manifestFiles {
		actual, ok := payload[path]
		if !ok || actual.Group != expected.Group || actual.Size != expected.Size || actual.Mode != expected.Mode || actual.SHA256 != expected.SHA256 {
			return fmt.Errorf("manifest hash or metadata mismatch: %s", path)
		}
	}
	for _, schema := range manifest.Schemas {
		group, ok := groupForSchema(schema.SchemaID)
		if !ok || group != schema.Group || group == GroupVACache {
			return fmt.Errorf("manifest contains unsupported schema group: %s", schema.SchemaID)
		}
		switch schema.Status {
		case "missing":
			if schema.Observed != "" && schema.Observed != "missing" {
				return fmt.Errorf("missing schema record has an observed version: %s", schema.SchemaID)
			}
		case "schema_current", "schema_legacy", "crypto_current":
		default:
			return fmt.Errorf("manifest contains unsupported schema status: %s %s", schema.SchemaID, schema.Status)
		}
		versions, err := observedSchemaVersions(schema)
		if err != nil {
			return err
		}
		for _, version := range versions {
			if !statedoctor.SupportsSchemaVersion(schema.SchemaID, version) {
				return fmt.Errorf("restore build does not support %s schema version %d", schema.SchemaID, version)
			}
		}
	}
	seenSchemas := map[string]bool{}
	for _, schema := range manifest.Schemas {
		seenSchemas[schema.SchemaID] = true
	}
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
		if !seenSchemas[schemaID] {
			return fmt.Errorf("manifest schema inventory is incomplete: %s", schemaID)
		}
	}
	return nil
}

func observedSchemaVersions(schema SchemaRecord) ([]int, error) {
	observed := strings.TrimSpace(schema.Observed)
	if schema.Status == "missing" || observed == "" || observed == "missing" {
		return nil, nil
	}
	if observed == "no ledger" {
		return []int{0}, nil
	}
	parts := strings.Split(observed, ",")
	versions := make([]int, 0, len(parts))
	for _, part := range parts {
		version, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil {
			return nil, fmt.Errorf("manifest schema version is not numeric: %s %q", schema.SchemaID, schema.Observed)
		}
		versions = append(versions, version)
	}
	return versions, nil
}

func stagingDestination(stageRoot string) payloadDestination {
	return func(record FileRecord) (io.WriteCloser, error) {
		path := filepath.Join(stageRoot, filepath.FromSlash(record.Path))
		if !pathWithin(stageRoot, path) {
			return nil, errors.New("unsafe restore staging path")
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, err
		}
		fd, err := unix.Open(path, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, record.Mode)
		if err != nil {
			return nil, err
		}
		file := os.NewFile(uintptr(fd), path)
		if file == nil {
			_ = unix.Close(fd)
			return nil, errors.New("invalid restore staging file descriptor")
		}
		return &syncedFile{File: file, mode: os.FileMode(record.Mode)}, nil
	}
}

type syncedFile struct {
	*os.File
	mode os.FileMode
}

func (file *syncedFile) Close() error {
	return errors.Join(file.File.Chmod(file.mode), file.File.Sync(), file.File.Close())
}

func validateArchiveFile(path string, allowInsecure bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errors.New("archive must be a regular non-symlink file")
	}
	if !allowInsecure && info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("archive mode must be 0600 or stricter, got %04o", info.Mode().Perm())
	}
	return nil
}

func safeArchiveEntryName(name string) bool {
	if name == "" || filepath.IsAbs(name) || strings.Contains(name, "\\") {
		return false
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(name)))
	return clean == name && clean != "." && clean != ".." && !strings.HasPrefix(clean, "../")
}

func ensureRestoreRoots(dataDir string) error {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return err
	}
	for _, path := range []string{dataDir, filepath.Join(dataDir, "state"), filepath.Join(dataDir, "novel-fetcher")} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return errors.Join(err, fmt.Errorf("restore root must be a non-symlink directory: %s", path))
		}
	}
	return nil
}

func createEmptyPrivateDirectory(path string) error {
	if _, err := os.Lstat(path); err == nil {
		return fmt.Errorf("restore temporary path already exists: %s", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		return err
	}
	return errors.Join(syncDirectory(path), syncDirectory(filepath.Dir(path)))
}

func prepareStagingLayout(stageRoot string) error {
	for _, relative := range []string{
		"novel-fetcher/works",
		"state/character_events",
		"state/term_profiles",
		"state/extraction_jobs",
	} {
		if err := os.MkdirAll(filepath.Join(stageRoot, filepath.FromSlash(relative)), 0o700); err != nil {
			return err
		}
	}
	return nil
}

func restoreTargets() []string {
	targets := []string{
		"novel-fetcher/library.sqlite",
		"novel-fetcher/library.sqlite-wal",
		"novel-fetcher/library.sqlite-shm",
		"novel-fetcher/works",
	}
	for _, name := range []string{readingstate.FileName, bookmarks.FileName, preferences.FileName, novelsettings.FileName, aisettings.FileName, publications.FileName} {
		targets = append(targets, filepath.ToSlash(filepath.Join("state", name)))
	}
	targets = append(targets,
		"state/character_events",
		"state/term_profiles",
		"state/extraction_jobs",
		"state/ai_usage.sqlite",
		"state/ai_usage.sqlite-journal",
		"state/character_profiles",
		"state/"+readertextcache.FileName,
		"state/"+readertextcache.FileName+"-journal",
		"state/"+readertextcache.FileName+"-wal",
		"state/"+readertextcache.FileName+"-shm",
	)
	return targets
}

func firstDoctorError(report statedoctor.Report) (statedoctor.Finding, bool) {
	for _, finding := range report.Findings {
		if finding.Severity == statedoctor.SeverityError {
			return finding, true
		}
	}
	return statedoctor.Finding{}, false
}
