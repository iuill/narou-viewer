package statebackup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/sys/unix"

	"narou-viewer/apps/viewer-api-go/internal/statebarrier"
)

const restoreTransactionVersion = 1
const maxRestoreJournalBytes = 1 << 20

const (
	restorePhaseStaging     = "staging"
	restorePhasePublishing  = "publishing"
	restorePhaseVerifying   = "verifying"
	restorePhaseRollingBack = "rolling_back"
	restorePhaseRolledBack  = "rolled_back"
	restorePhaseCommitting  = "committing"
)

type restoreTransaction struct {
	Version           int                        `json:"version"`
	GenerationID      string                     `json:"generation_id"`
	StageDirectory    string                     `json:"stage_directory"`
	RollbackDirectory string                     `json:"rollback_directory"`
	Phase             string                     `json:"phase"`
	NextAction        int                        `json:"next_action"`
	Actions           []restoreTransactionAction `json:"actions"`
}

type restoreTransactionAction struct {
	Relative string `json:"relative"`
	HadOld   bool   `json:"had_old"`
	HasNew   bool   `json:"has_new"`
}

type RecoveryOutcome string

const (
	RecoveryNone             RecoveryOutcome = "none"
	RecoveryStagingCleanup   RecoveryOutcome = "staging_cleanup"
	RecoveryRolledBack       RecoveryOutcome = "rolled_back"
	RecoveryCommittedCleanup RecoveryOutcome = "committed_cleanup"
)

func newRestoreTransaction(generationID string) restoreTransaction {
	return restoreTransaction{
		Version:           restoreTransactionVersion,
		GenerationID:      generationID,
		StageDirectory:    ".restore-staging-" + generationID,
		RollbackDirectory: ".restore-rollback-" + generationID,
		Phase:             restorePhaseStaging,
		Actions:           []restoreTransactionAction{},
	}
}

func Recover(ctx context.Context, dataDir string) (RecoveryOutcome, error) {
	dataDir = filepath.Clean(strings.TrimSpace(dataDir))
	if dataDir == "." {
		return RecoveryNone, errors.New("data directory is required")
	}
	if err := rejectFilesystemRoot(dataDir, "data directory"); err != nil {
		return RecoveryNone, err
	}
	info, err := os.Lstat(dataDir)
	if err != nil {
		return RecoveryNone, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return RecoveryNone, fmt.Errorf("restore recovery data path must be a non-symlink directory: %s", dataDir)
	}
	locks, err := statebarrier.AcquireWriters(dataDir)
	if err != nil {
		return RecoveryNone, fmt.Errorf("restore recovery requires viewer-api and novel-fetcher to be stopped: %w", err)
	}
	defer locks.Close()
	if err := ensureRestoreRoots(dataDir); err != nil {
		return RecoveryNone, err
	}
	return recoverRestoreTransactionLocked(ctx, dataDir)
}

func beginRestoreTransaction(dataDir string, transaction *restoreTransaction) error {
	path := restoreJournalPath(dataDir)
	if _, err := os.Lstat(path); err == nil {
		return fmt.Errorf("restore transaction journal already exists: %s", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := cleanupRestoreJournalPartials(dataDir); err != nil {
		return err
	}
	return writeRestoreTransaction(dataDir, transaction)
}

func buildRestoreTransactionPlan(dataDir string, transaction *restoreTransaction) error {
	actions := make([]restoreTransactionAction, 0, len(restoreTargets()))
	stageRoot, _, err := restoreTransactionRoots(dataDir, transaction)
	if err != nil {
		return err
	}
	for _, relative := range restoreTargets() {
		destination := filepath.Join(dataDir, filepath.FromSlash(relative))
		staged := filepath.Join(stageRoot, filepath.FromSlash(relative))
		hadOld, err := pathExists(destination)
		if err != nil {
			return err
		}
		hasNew, err := pathExists(staged)
		if err != nil {
			return err
		}
		actions = append(actions, restoreTransactionAction{Relative: relative, HadOld: hadOld, HasNew: hasNew})
	}
	transaction.Actions = actions
	transaction.NextAction = 0
	transaction.Phase = restorePhasePublishing
	return writeRestoreTransaction(dataDir, transaction)
}

func publishRestoreTransaction(ctx context.Context, dataDir string, transaction *restoreTransaction) error {
	stageRoot, rollbackRoot, err := restoreTransactionRoots(dataDir, transaction)
	if err != nil {
		return err
	}
	for index, action := range transaction.Actions {
		if err := ctx.Err(); err != nil {
			return err
		}
		transaction.NextAction = index
		if err := writeRestoreTransaction(dataDir, transaction); err != nil {
			return err
		}
		destination := filepath.Join(dataDir, filepath.FromSlash(action.Relative))
		staged := filepath.Join(stageRoot, filepath.FromSlash(action.Relative))
		rollback := filepath.Join(rollbackRoot, filepath.FromSlash(action.Relative))
		if action.HadOld {
			if err := ensureDurableDirectory(filepath.Dir(rollback)); err != nil {
				return err
			}
			if err := durableRename(destination, rollback); err != nil {
				return fmt.Errorf("move current restore target to rollback %s: %w", action.Relative, err)
			}
		}
		if action.HasNew {
			if err := ensureDurableDirectory(filepath.Dir(destination)); err != nil {
				return err
			}
			if err := durableRename(staged, destination); err != nil {
				return fmt.Errorf("publish staged restore target %s: %w", action.Relative, err)
			}
		}
		transaction.NextAction = index + 1
		if err := writeRestoreTransaction(dataDir, transaction); err != nil {
			return err
		}
	}
	transaction.Phase = restorePhaseVerifying
	return writeRestoreTransaction(dataDir, transaction)
}

func commitRestoreTransaction(dataDir string, transaction *restoreTransaction) error {
	transaction.Phase = restorePhaseCommitting
	if err := writeRestoreTransaction(dataDir, transaction); err != nil {
		return err
	}
	return cleanupRestoreTransaction(dataDir, transaction)
}

func recoverRestoreTransactionLocked(ctx context.Context, dataDir string) (RecoveryOutcome, error) {
	transaction, err := readRestoreTransaction(dataDir)
	if errors.Is(err, os.ErrNotExist) {
		return RecoveryNone, nil
	}
	if err != nil {
		return RecoveryNone, fmt.Errorf("read restore transaction journal: %w", err)
	}
	outcome := RecoveryStagingCleanup
	switch transaction.Phase {
	case restorePhaseStaging:
	case restorePhaseRolledBack:
		outcome = RecoveryRolledBack
	case restorePhaseCommitting:
		outcome = RecoveryCommittedCleanup
		// No published target exists during staging. Rolled-back and committed
		// transactions only need their durable temporary data cleaned up.
	case restorePhasePublishing, restorePhaseVerifying, restorePhaseRollingBack:
		outcome = RecoveryRolledBack
		if transaction.Phase != restorePhaseRollingBack {
			if err := prepareRestoreRollback(dataDir, &transaction); err != nil {
				return RecoveryNone, fmt.Errorf("validate interrupted restore rollback prerequisites: %w", err)
			}
		}
		if err := rollbackRestoreTransaction(ctx, dataDir, &transaction); err != nil {
			return RecoveryNone, fmt.Errorf("roll back interrupted restore transaction: %w", err)
		}
		transaction.Phase = restorePhaseRolledBack
		if err := writeRestoreTransaction(dataDir, &transaction); err != nil {
			return RecoveryNone, err
		}
	default:
		return RecoveryNone, fmt.Errorf("restore transaction has unsupported phase %q", transaction.Phase)
	}
	if err := cleanupRestoreTransaction(dataDir, &transaction); err != nil {
		return RecoveryNone, err
	}
	return outcome, nil
}

func rollbackRestoreTransaction(ctx context.Context, dataDir string, transaction *restoreTransaction) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if transaction.Phase != restorePhaseRollingBack {
		if err := prepareRestoreRollback(dataDir, transaction); err != nil {
			return err
		}
	}
	stageRoot, rollbackRoot, err := restoreTransactionRoots(dataDir, transaction)
	if err != nil {
		return err
	}
	for index := transaction.NextAction; index >= 0; index-- {
		if err := ctx.Err(); err != nil {
			return err
		}
		action := transaction.Actions[index]
		if err := rollbackRestoreAction(dataDir, stageRoot, rollbackRoot, action); err != nil {
			return err
		}
		transaction.NextAction = index - 1
		if err := writeRestoreTransaction(dataDir, transaction); err != nil {
			return err
		}
	}
	return nil
}

func rollbackRestoreAction(dataDir string, stageRoot string, rollbackRoot string, action restoreTransactionAction) error {
	destination := filepath.Join(dataDir, filepath.FromSlash(action.Relative))
	staged := filepath.Join(stageRoot, filepath.FromSlash(action.Relative))
	rollback := filepath.Join(rollbackRoot, filepath.FromSlash(action.Relative))
	rollbackExists, err := pathExists(rollback)
	if err != nil {
		return err
	}
	destinationExists, err := pathExists(destination)
	if err != nil {
		return err
	}
	if action.HadOld {
		if rollbackExists {
			if destinationExists {
				if err := durableRemoveAll(destination); err != nil {
					return err
				}
			}
			if err := ensureDurableDirectory(filepath.Dir(destination)); err != nil {
				return err
			}
			if err := durableRename(rollback, destination); err != nil {
				return fmt.Errorf("restore rollback target %s: %w", action.Relative, err)
			}
		} else if !destinationExists {
			return fmt.Errorf("restore rollback target is missing from both live and rollback locations: %s", action.Relative)
		}
	} else if !action.HasNew {
		if destinationExists {
			return fmt.Errorf("unexpected live restore target without an old or staged value: %s", action.Relative)
		}
	} else {
		stagedExists, err := pathExists(staged)
		if err != nil {
			return err
		}
		if !stagedExists && destinationExists {
			if err := durableRemoveAll(destination); err != nil {
				return err
			}
		}
	}
	return nil
}

func prepareRestoreRollback(dataDir string, transaction *restoreTransaction) error {
	if err := validateRollbackPrerequisites(dataDir, transaction); err != nil {
		return err
	}
	transaction.Phase = restorePhaseRollingBack
	transaction.NextAction = len(transaction.Actions) - 1
	return writeRestoreTransaction(dataDir, transaction)
}

func validateRollbackPrerequisites(dataDir string, transaction *restoreTransaction) error {
	_, rollbackRoot, err := restoreTransactionRoots(dataDir, transaction)
	if err != nil {
		return err
	}
	requiredBefore := 0
	switch transaction.Phase {
	case restorePhasePublishing:
		requiredBefore = transaction.NextAction
	case restorePhaseVerifying:
		requiredBefore = len(transaction.Actions)
	default:
		return fmt.Errorf("restore phase %q cannot begin rollback", transaction.Phase)
	}
	for index := 0; index < requiredBefore; index++ {
		action := transaction.Actions[index]
		if !action.HadOld {
			continue
		}
		rollback := filepath.Join(rollbackRoot, filepath.FromSlash(action.Relative))
		exists, err := pathExists(rollback)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("completed restore action is missing rollback data: %s", action.Relative)
		}
	}
	return nil
}

func cleanupRestoreTransaction(dataDir string, transaction *restoreTransaction) error {
	// Revalidate immediately before recursive deletion. Journal validation is
	// deliberately not treated as permanent authorization for a later cleanup.
	stageRoot, rollbackRoot, err := restoreTransactionRoots(dataDir, transaction)
	if err != nil {
		return err
	}
	if err := durableRemoveAll(rollbackRoot); err != nil {
		return fmt.Errorf("remove restore rollback data: %w", err)
	}
	if err := durableRemoveAll(stageRoot); err != nil {
		return fmt.Errorf("remove restore staging data: %w", err)
	}
	if err := cleanupRestoreJournalPartials(dataDir); err != nil {
		return err
	}
	journalPath := restoreJournalPath(dataDir)
	if err := os.Remove(journalPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove restore transaction journal: %w", err)
	}
	if err := syncDirectory(dataDir); err != nil {
		return fmt.Errorf("sync restore transaction directory: %w", err)
	}
	return nil
}

func cleanupRestoreJournalPartials(dataDir string) error {
	paths, err := filepath.Glob(filepath.Join(dataDir, ".state-restore-transaction-*.partial"))
	if err != nil {
		return err
	}
	removed := false
	for _, path := range paths {
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("restore journal partial is not a regular file: %s", path)
		}
		if err := os.Remove(path); err != nil {
			return err
		}
		removed = true
	}
	if removed {
		return syncDirectory(dataDir)
	}
	return nil
}

func restoreJournalPath(dataDir string) string {
	return filepath.Join(filepath.Clean(dataDir), statebarrier.RestoreJournalRelativePath)
}

func readRestoreTransaction(dataDir string) (restoreTransaction, error) {
	path := restoreJournalPath(dataDir)
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NONBLOCK|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return restoreTransaction{}, err
	}
	file := os.NewFile(uintptr(fd), path)
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return restoreTransaction{}, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 || info.Size() > maxRestoreJournalBytes {
		return restoreTransaction{}, errors.New("restore transaction journal must be a private regular file within the size limit")
	}
	decoder := json.NewDecoder(io.LimitReader(file, maxRestoreJournalBytes+1))
	decoder.DisallowUnknownFields()
	var transaction restoreTransaction
	if err := decoder.Decode(&transaction); err != nil {
		return restoreTransaction{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return restoreTransaction{}, errors.New("restore transaction journal contains trailing data")
	}
	if err := validateRestoreTransactionForDataDir(dataDir, &transaction); err != nil {
		return restoreTransaction{}, err
	}
	return transaction, nil
}

func writeRestoreTransaction(dataDir string, transaction *restoreTransaction) (resultErr error) {
	if err := validateRestoreTransactionForDataDir(dataDir, transaction); err != nil {
		return err
	}
	if err := ensureDurableDirectory(dataDir); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(dataDir, ".state-restore-transaction-*.partial")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() {
		if resultErr != nil {
			_ = temporary.Close()
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return err
	}
	encoder := json.NewEncoder(temporary)
	if err := encoder.Encode(transaction); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, restoreJournalPath(dataDir)); err != nil {
		return err
	}
	return syncDirectory(dataDir)
}

func validateRestoreTransaction(transaction *restoreTransaction) error {
	if transaction == nil || transaction.Version != restoreTransactionVersion {
		return errors.New("restore transaction journal header is invalid")
	}
	if err := validateRestoreTransactionPathFields(transaction); err != nil {
		return err
	}
	switch transaction.Phase {
	case restorePhaseStaging:
		if len(transaction.Actions) != 0 || transaction.NextAction != 0 {
			return errors.New("staging restore transaction must not contain publish actions")
		}
	case restorePhasePublishing, restorePhaseVerifying, restorePhaseRollingBack, restorePhaseRolledBack, restorePhaseCommitting:
		targets := restoreTargets()
		if len(transaction.Actions) != len(targets) {
			return errors.New("restore transaction publish plan is incomplete")
		}
		for index, action := range transaction.Actions {
			if action.Relative != targets[index] {
				return errors.New("restore transaction publish plan contains an unsafe target")
			}
		}
		switch transaction.Phase {
		case restorePhasePublishing:
			if transaction.NextAction < 0 || transaction.NextAction > len(targets) {
				return errors.New("restore transaction publish progress is invalid")
			}
		case restorePhaseVerifying, restorePhaseCommitting:
			if transaction.NextAction != len(targets) {
				return errors.New("completed restore publish progress is invalid")
			}
		case restorePhaseRollingBack:
			if transaction.NextAction < -1 || transaction.NextAction >= len(targets) {
				return errors.New("restore transaction rollback progress is invalid")
			}
		case restorePhaseRolledBack:
			if transaction.NextAction != -1 {
				return errors.New("completed restore rollback progress is invalid")
			}
		}
	default:
		return fmt.Errorf("restore transaction has unsupported phase %q", transaction.Phase)
	}
	return nil
}

func validateRestoreTransactionForDataDir(dataDir string, transaction *restoreTransaction) error {
	if err := validateRestoreTransaction(transaction); err != nil {
		return err
	}
	_, _, err := restoreTransactionRootsUnchecked(dataDir, transaction)
	return err
}

func restoreTransactionRoots(dataDir string, transaction *restoreTransaction) (string, string, error) {
	if err := validateRestoreTransactionPathFields(transaction); err != nil {
		return "", "", err
	}
	return restoreTransactionRootsUnchecked(dataDir, transaction)
}

func validateRestoreTransactionPathFields(transaction *restoreTransaction) error {
	if transaction == nil || validateGenerationID(transaction.GenerationID) != nil {
		return errors.New("restore transaction journal header is invalid")
	}
	if transaction.StageDirectory != ".restore-staging-"+transaction.GenerationID || transaction.RollbackDirectory != ".restore-rollback-"+transaction.GenerationID {
		return errors.New("restore transaction journal paths are invalid")
	}
	return nil
}

func restoreTransactionRootsUnchecked(dataDir string, transaction *restoreTransaction) (string, string, error) {
	dataRoot, err := filepath.Abs(filepath.Clean(dataDir))
	if err != nil {
		return "", "", err
	}
	resolve := func(name string) (string, error) {
		if filepath.Base(name) != name || name == "." || name == ".." {
			return "", errors.New("restore transaction journal path is not a top-level directory")
		}
		path := filepath.Clean(filepath.Join(dataRoot, name))
		if filepath.Dir(path) != dataRoot || !pathWithin(dataRoot, path) {
			return "", errors.New("restore transaction journal path escapes the data directory")
		}
		return path, nil
	}
	stageRoot, err := resolve(transaction.StageDirectory)
	if err != nil {
		return "", "", err
	}
	rollbackRoot, err := resolve(transaction.RollbackDirectory)
	if err != nil {
		return "", "", err
	}
	return stageRoot, rollbackRoot, nil
}

func pathExists(path string) (bool, error) {
	_, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

func ensureDurableDirectory(path string) error {
	path = filepath.Clean(path)
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.Join(err, fmt.Errorf("restore directory must be a non-symlink directory: %s", path))
	}
	parent := filepath.Dir(path)
	if parent == path {
		return syncDirectory(path)
	}
	return errors.Join(syncDirectory(path), syncDirectory(parent))
}

func syncRestoreTree(root string) error {
	directories := []string{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("restore staging tree contains a symlink: %s", path)
		}
		if entry.IsDir() {
			directories = append(directories, path)
		}
		return nil
	})
	if err != nil {
		return err
	}
	sort.Slice(directories, func(left int, right int) bool {
		return len(directories[left]) > len(directories[right])
	})
	for _, directory := range directories {
		if err := syncDirectory(directory); err != nil {
			return err
		}
	}
	return syncDirectory(filepath.Dir(root))
}

func durableRename(source string, destination string) error {
	if err := os.Rename(source, destination); err != nil {
		return err
	}
	sourceParent := filepath.Dir(source)
	destinationParent := filepath.Dir(destination)
	if err := syncDirectory(sourceParent); err != nil {
		return err
	}
	if destinationParent != sourceParent {
		if err := syncDirectory(destinationParent); err != nil {
			return err
		}
	}
	return nil
}

func durableRemoveAll(path string) error {
	exists, err := pathExists(path)
	if err != nil || !exists {
		return err
	}
	if err := os.RemoveAll(path); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(path))
}
