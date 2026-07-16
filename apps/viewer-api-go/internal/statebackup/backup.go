package statebackup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"filippo.io/age"
	"golang.org/x/sys/unix"

	"narou-viewer/apps/viewer-api-go/internal/state/aisettings"
	"narou-viewer/apps/viewer-api-go/internal/statebarrier"
	"narou-viewer/apps/viewer-api-go/internal/statedoctor"
	"narou-viewer/apps/viewer-api-go/internal/statesecurity"
)

var referencePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/@-]{0,127}$`)

func Backup(ctx context.Context, options BackupOptions) (BackupResult, error) {
	dataDir := filepath.Clean(strings.TrimSpace(options.DataDir))
	outputDir := filepath.Clean(strings.TrimSpace(options.OutputDir))
	if dataDir == "." || outputDir == "." {
		return BackupResult{}, errors.New("data directory and output directory are required")
	}
	if options.Recipient == nil {
		return BackupResult{}, errors.New("age recipient is required")
	}
	if !referencePattern.MatchString(options.KeyReference) {
		return BackupResult{}, errors.New("key reference must be a non-secret identifier using safe characters")
	}
	overlap, err := pathsOverlapPhysically(dataDir, outputDir)
	if err != nil {
		return BackupResult{}, fmt.Errorf("resolve backup output location: %w", err)
	}
	if overlap {
		return BackupResult{}, errors.New("backup output directory must be separate from the data tree")
	}
	if options.Retention != nil {
		if err := validateRetentionPolicy(*options.Retention); err != nil {
			return BackupResult{}, err
		}
	}
	if err := ensurePrivateDirectory(outputDir); err != nil {
		return BackupResult{}, err
	}
	locks, err := statebarrier.AcquireWriters(dataDir)
	if err != nil {
		return BackupResult{}, fmt.Errorf("cold snapshot requires viewer-api and novel-fetcher to be stopped: %w", err)
	}
	defer locks.Close()
	if err := statebarrier.EnsureNoRestoreInProgress(dataDir); err != nil {
		return BackupResult{}, err
	}

	settingsPath := filepath.Join(dataDir, "state", aisettings.FileName)
	if found, _, err := statesecurity.HasLegacyPlaintextAPIKeyIfExists(settingsPath); err != nil {
		return BackupResult{}, fmt.Errorf("inspect AI settings legacy credential: %w", err)
	} else if found {
		return BackupResult{}, errors.New("backup refused: AI settings contains a non-empty legacy plaintext api_key; complete encrypted credential migration and retry")
	}
	report, err := statedoctor.Scan(ctx, dataDir)
	if err != nil {
		return BackupResult{}, fmt.Errorf("state doctor preflight: %w", err)
	}
	if finding, blocked := blockingDoctorFinding(report); blocked {
		return BackupResult{}, fmt.Errorf("state doctor preflight blocks backup: %s %s %s", finding.SchemaID, finding.Path, finding.Kind)
	}
	files, err := collectPayloadFiles(dataDir)
	if err != nil {
		return BackupResult{}, err
	}
	now := time.Now
	if options.Now != nil {
		now = options.Now
	}
	generationID := randomGenerationID
	if options.GenerationID != nil {
		generationID = options.GenerationID
	}
	generation, err := generationID()
	if err != nil {
		return BackupResult{}, fmt.Errorf("generate snapshot generation ID: %w", err)
	}
	if !referencePattern.MatchString(generation) {
		return BackupResult{}, errors.New("snapshot generation ID contains unsafe characters")
	}
	createdAt := now().UTC()
	build := strings.TrimSpace(options.ApplicationBuild)
	if build == "" {
		build = "development"
	}
	if len(build) > 128 || strings.ContainsAny(build, "\r\n\x00") {
		return BackupResult{}, errors.New("application build identifier is invalid")
	}
	manifest := newManifest(generation, createdAt, build, options.KeyReference, report)
	archiveName := "narou-viewer-" + createdAt.Format("20060102T150405Z") + "-" + generation + ArchiveSuffix
	archivePath := filepath.Join(outputDir, archiveName)
	if err := writeEncryptedArchive(ctx, archivePath, options.Recipient, files, &manifest, createdAt); err != nil {
		return BackupResult{}, err
	}
	result := BackupResult{ArchivePath: archivePath, Manifest: manifest}
	if options.Retention != nil {
		policy := *options.Retention
		if policy.Now == nil {
			policy.Now = now
		}
		pruned, err := PruneArchives(outputDir, policy)
		if err != nil {
			return result, fmt.Errorf("backup completed but retention cleanup failed: %w", err)
		}
		result.Pruned = pruned
	}
	return result, nil
}

func newManifest(generation string, createdAt time.Time, build string, keyReference string, report statedoctor.Report) Manifest {
	manifest := Manifest{
		FormatVersion:    ManifestFormatVersion,
		GenerationID:     generation,
		CreatedAt:        createdAt.Format(time.RFC3339Nano),
		ApplicationBuild: build,
		SnapshotMethod:   "cold-stop+writer-lock-v1",
		Encryption:       "age-v1",
		KeyReference:     keyReference,
		SecretReferences: []string{"backup-key:" + keyReference, "ai-settings-master-passphrase:external"},
		Groups: []GroupRecord{
			{ID: GroupNFCanonical, Included: true},
			{ID: GroupVACore, Included: true},
			{ID: GroupVAExtraction, Included: true},
			{ID: GroupVAHistory, Included: true},
			{ID: GroupVACache, Included: false, Reason: "rebuildable derived state is recreated after restore"},
			{ID: GroupSecrets, Included: false, Reason: "backup key and AI settings master passphrase are managed separately"},
		},
		Schemas:       []SchemaRecord{},
		Files:         []FileRecord{},
		DoctorSummary: report.Summary,
	}
	for _, finding := range report.Findings {
		group, known := groupForSchema(finding.SchemaID)
		if !known || group == GroupVACache {
			continue
		}
		switch finding.Kind {
		case "schema_current", "schema_legacy", "missing", "crypto_current":
			manifest.Schemas = append(manifest.Schemas, SchemaRecord{
				SchemaID:  finding.SchemaID,
				Path:      finding.Path,
				Observed:  finding.Observed,
				Supported: finding.Supported,
				Status:    finding.Kind,
				Group:     group,
			})
		}
	}
	return manifest
}

func blockingDoctorFinding(report statedoctor.Report) (statedoctor.Finding, bool) {
	for _, finding := range report.Findings {
		switch finding.Kind {
		case "insecure_file_mode", "sensitive_symlink", "sensitive_file_outside_state", "frontier_inversion", "term_frontier_without_character_frontier", "multiple_active_jobs":
			return finding, true
		}
		if finding.Severity != statedoctor.SeverityError {
			continue
		}
		if group, known := groupForSchema(finding.SchemaID); known && group == GroupVACache {
			continue
		}
		return finding, true
	}
	return statedoctor.Finding{}, false
}

func writeEncryptedArchive(ctx context.Context, archivePath string, recipient age.Recipient, files []sourceFile, manifest *Manifest, createdAt time.Time) (resultErr error) {
	partialPath := archivePath + ".partial"
	output, err := os.OpenFile(partialPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	complete := false
	defer func() {
		if !complete {
			_ = output.Close()
			_ = os.Remove(partialPath)
		}
	}()
	encrypted, err := age.Encrypt(output, recipient)
	if err != nil {
		return err
	}
	compressed := gzip.NewWriter(encrypted)
	tarWriter := tar.NewWriter(compressed)
	closeWriters := func() error {
		return errors.Join(tarWriter.Close(), compressed.Close(), encrypted.Close())
	}
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			_ = closeWriters()
			return err
		}
		source, info, err := openSourceFile(file.absolute)
		if err != nil {
			_ = closeWriters()
			return err
		}
		if info.Size() != file.record.Size {
			_ = source.Close()
			_ = closeWriters()
			return fmt.Errorf("backup source changed during preflight: %s", file.record.Path)
		}
		header := &tar.Header{Name: "payload/" + file.record.Path, Mode: int64(file.record.Mode), Size: file.record.Size, ModTime: createdAt, Typeflag: tar.TypeReg, Format: tar.FormatPAX}
		if err := tarWriter.WriteHeader(header); err != nil {
			_ = source.Close()
			_ = closeWriters()
			return err
		}
		hash := sha256.New()
		written, copyErr := io.CopyN(io.MultiWriter(tarWriter, hash), contextAwareReader(ctx, source), file.record.Size)
		closeErr := source.Close()
		if copyErr != nil || closeErr != nil || written != file.record.Size {
			_ = closeWriters()
			return errors.Join(copyErr, closeErr, fmt.Errorf("copy backup payload %s: wrote %d of %d bytes", file.record.Path, written, file.record.Size))
		}
		file.record.SHA256 = "sha256:" + hex.EncodeToString(hash.Sum(nil))
		manifest.Files = append(manifest.Files, file.record)
	}
	manifestRaw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		_ = closeWriters()
		return err
	}
	manifestRaw = append(manifestRaw, '\n')
	if err := tarWriter.WriteHeader(&tar.Header{Name: ManifestName, Mode: 0o600, Size: int64(len(manifestRaw)), ModTime: createdAt, Typeflag: tar.TypeReg, Format: tar.FormatPAX}); err != nil {
		_ = closeWriters()
		return err
	}
	if _, err := tarWriter.Write(manifestRaw); err != nil {
		_ = closeWriters()
		return err
	}
	if err := closeWriters(); err != nil {
		return err
	}
	if err := output.Sync(); err != nil {
		return err
	}
	if err := output.Close(); err != nil {
		return err
	}
	if err := publishArchiveNoReplace(partialPath, archivePath); err != nil {
		return err
	}
	if err := os.Chmod(archivePath, 0o600); err != nil {
		return err
	}
	if err := syncDirectory(filepath.Dir(archivePath)); err != nil {
		return err
	}
	complete = true
	return nil
}

func publishArchiveNoReplace(partialPath string, archivePath string) error {
	return unix.Renameat2(unix.AT_FDCWD, partialPath, unix.AT_FDCWD, archivePath, unix.RENAME_NOREPLACE)
}

func openSourceFile(path string) (*os.File, os.FileInfo, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = unix.Close(fd)
		return nil, nil, errors.New("invalid backup source file descriptor")
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, nil, err
	}
	if !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, nil, fmt.Errorf("backup source is not a regular file: %s", path)
	}
	return file, info, nil
}

func randomGenerationID() (string, error) {
	raw := make([]byte, 12)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func ensurePrivateDirectory(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("backup directory must be a non-symlink directory: %s", path)
	}
	return os.Chmod(path, 0o700)
}

func pathWithin(root string, path string) bool {
	root, errRoot := filepath.Abs(filepath.Clean(root))
	path, errPath := filepath.Abs(filepath.Clean(path))
	if errRoot != nil || errPath != nil {
		return false
	}
	return path == root || strings.HasPrefix(path, root+string(filepath.Separator))
}

func pathsOverlapPhysically(left string, right string) (bool, error) {
	physicalLeft, err := physicalPath(left)
	if err != nil {
		return false, err
	}
	physicalRight, err := physicalPath(right)
	if err != nil {
		return false, err
	}
	return pathWithin(physicalLeft, physicalRight) || pathWithin(physicalRight, physicalLeft), nil
}

func pathWithinPhysical(root string, path string) (bool, error) {
	physicalRoot, err := physicalPath(root)
	if err != nil {
		return false, err
	}
	physicalTarget, err := physicalPath(path)
	if err != nil {
		return false, err
	}
	return pathWithin(physicalRoot, physicalTarget), nil
}

// physicalPath resolves every existing path component and then appends any
// not-yet-created suffix. This keeps containment checks meaningful for backup
// output directories that do not exist yet without following a textual alias.
func physicalPath(path string) (string, error) {
	absolute, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	ancestor := absolute
	suffix := []string{}
	for {
		_, statErr := os.Lstat(ancestor)
		if statErr == nil {
			break
		}
		if !errors.Is(statErr, os.ErrNotExist) {
			return "", statErr
		}
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			return "", statErr
		}
		suffix = append(suffix, filepath.Base(ancestor))
		ancestor = parent
	}
	resolved, err := filepath.EvalSymlinks(ancestor)
	if err != nil {
		return "", err
	}
	for index := len(suffix) - 1; index >= 0; index-- {
		resolved = filepath.Join(resolved, suffix[index])
	}
	return filepath.Clean(resolved), nil
}
