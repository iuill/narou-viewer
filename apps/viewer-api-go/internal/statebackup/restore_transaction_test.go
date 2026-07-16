package statebackup

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"narou-viewer/apps/viewer-api-go/internal/statebarrier"
)

const restoreCrashHelperEnvironment = "NAROU_VIEWER_RESTORE_CRASH_HELPER"
const restoreCrashDataEnvironment = "NAROU_VIEWER_RESTORE_CRASH_DATA_DIR"

func TestInterruptedPublishRecoversPreviousGenerationOnNextProcess(t *testing.T) {
	dataDir := t.TempDir()
	if err := ensureRestoreRoots(dataDir); err != nil {
		t.Fatalf("ensureRestoreRoots: %v", err)
	}
	transaction := newRestoreTransaction("crash-recovery-test")
	stageRoot := filepath.Join(dataDir, transaction.StageDirectory)
	rollbackRoot := filepath.Join(dataDir, transaction.RollbackDirectory)
	if err := beginRestoreTransaction(dataDir, &transaction); err != nil {
		t.Fatalf("beginRestoreTransaction: %v", err)
	}
	if err := createEmptyPrivateDirectory(stageRoot); err != nil {
		t.Fatalf("create stage: %v", err)
	}
	if err := prepareStagingLayout(stageRoot); err != nil {
		t.Fatalf("prepare stage: %v", err)
	}
	if err := createEmptyPrivateDirectory(rollbackRoot); err != nil {
		t.Fatalf("create rollback: %v", err)
	}
	oldLibrary := filepath.Join(dataDir, "novel-fetcher", "library.sqlite")
	newLibrary := filepath.Join(stageRoot, "novel-fetcher", "library.sqlite")
	oldWork := filepath.Join(dataDir, "novel-fetcher", "works", "old.txt")
	newWork := filepath.Join(stageRoot, "novel-fetcher", "works", "new.txt")
	for path, raw := range map[string]string{
		oldLibrary: "old library",
		newLibrary: "new library",
		oldWork:    "old work",
		newWork:    "new work",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatalf("mkdir fixture %s: %v", path, err)
		}
		if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
			t.Fatalf("write fixture %s: %v", path, err)
		}
	}
	if err := buildRestoreTransactionPlan(dataDir, &transaction); err != nil {
		t.Fatalf("buildRestoreTransactionPlan: %v", err)
	}

	command := exec.Command(os.Args[0], "-test.run=^TestRestoreTransactionCrashHelper$")
	command.Env = append(os.Environ(), restoreCrashHelperEnvironment+"=1", restoreCrashDataEnvironment+"="+dataDir)
	if output, err := command.CombinedOutput(); err == nil {
		t.Fatalf("crash helper unexpectedly exited successfully: %s", output)
	} else if exit := new(exec.ExitError); !errors.As(err, &exit) || exit.ExitCode() != 91 {
		t.Fatalf("crash helper exit=%v output=%s", err, output)
	}
	if raw, err := os.ReadFile(oldLibrary); err != nil || string(raw) != "new library" {
		t.Fatalf("expected mixed live library before recovery: raw=%q err=%v", raw, err)
	}
	if raw, err := os.ReadFile(oldWork); err != nil || string(raw) != "old work" {
		t.Fatalf("expected old works generation before recovery: raw=%q err=%v", raw, err)
	}

	outcome, err := Recover(context.Background(), dataDir)
	if err != nil || outcome != RecoveryRolledBack {
		t.Fatalf("Recover: outcome=%v err=%v", outcome, err)
	}
	if raw, err := os.ReadFile(oldLibrary); err != nil || string(raw) != "old library" {
		t.Fatalf("library did not roll back: raw=%q err=%v", raw, err)
	}
	if raw, err := os.ReadFile(oldWork); err != nil || string(raw) != "old work" {
		t.Fatalf("works did not remain on old generation: raw=%q err=%v", raw, err)
	}
	for _, path := range []string{restoreJournalPath(dataDir), stageRoot, rollbackRoot} {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("recovery left transaction path %s: %v", path, err)
		}
	}
	if outcome, err := Recover(context.Background(), dataDir); err != nil || outcome != RecoveryNone {
		t.Fatalf("second Recover: outcome=%v err=%v", outcome, err)
	}
}

func TestRestoreTransactionCrashHelper(t *testing.T) {
	if os.Getenv(restoreCrashHelperEnvironment) != "1" {
		return
	}
	dataDir := os.Getenv(restoreCrashDataEnvironment)
	transaction, err := readRestoreTransaction(dataDir)
	if err != nil {
		t.Fatalf("readRestoreTransaction: %v", err)
	}
	action := transaction.Actions[0]
	destination := filepath.Join(dataDir, filepath.FromSlash(action.Relative))
	staged := filepath.Join(dataDir, transaction.StageDirectory, filepath.FromSlash(action.Relative))
	rollback := filepath.Join(dataDir, transaction.RollbackDirectory, filepath.FromSlash(action.Relative))
	if err := ensureDurableDirectory(filepath.Dir(rollback)); err != nil {
		t.Fatalf("ensure rollback parent: %v", err)
	}
	if err := durableRename(destination, rollback); err != nil {
		t.Fatalf("move old target: %v", err)
	}
	if err := durableRename(staged, destination); err != nil {
		t.Fatalf("publish new target: %v", err)
	}
	os.Exit(91)
}

func TestRecoverCleansInterruptedStagingTransaction(t *testing.T) {
	dataDir := t.TempDir()
	if err := ensureRestoreRoots(dataDir); err != nil {
		t.Fatalf("ensureRestoreRoots: %v", err)
	}
	transaction := newRestoreTransaction("staging-recovery-test")
	if err := beginRestoreTransaction(dataDir, &transaction); err != nil {
		t.Fatalf("beginRestoreTransaction: %v", err)
	}
	stageRoot := filepath.Join(dataDir, transaction.StageDirectory)
	if err := os.Mkdir(stageRoot, 0o700); err != nil {
		t.Fatalf("mkdir stage: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stageRoot, "partial"), []byte("synthetic partial payload"), 0o600); err != nil {
		t.Fatalf("write partial stage: %v", err)
	}
	journalPartial := filepath.Join(dataDir, ".state-restore-transaction-stale.partial")
	if err := os.WriteFile(journalPartial, []byte("synthetic journal metadata"), 0o600); err != nil {
		t.Fatalf("write journal partial: %v", err)
	}
	if outcome, err := Recover(context.Background(), dataDir); err != nil || outcome != RecoveryStagingCleanup {
		t.Fatalf("Recover staging: outcome=%v err=%v", outcome, err)
	}
	if _, err := os.Lstat(stageRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("staging recovery left stage root: %v", err)
	}
	if _, err := os.Lstat(journalPartial); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("staging recovery left journal partial: %v", err)
	}
}

func TestRestoreTransactionRemovesAndCanRollbackStaleLibrarySHM(t *testing.T) {
	dataDir := t.TempDir()
	if err := ensureRestoreRoots(dataDir); err != nil {
		t.Fatalf("ensureRestoreRoots: %v", err)
	}
	transaction := newRestoreTransaction("library-shm-rollback-test")
	stageRoot := filepath.Join(dataDir, transaction.StageDirectory)
	rollbackRoot := filepath.Join(dataDir, transaction.RollbackDirectory)
	if err := beginRestoreTransaction(dataDir, &transaction); err != nil {
		t.Fatalf("beginRestoreTransaction: %v", err)
	}
	if err := createEmptyPrivateDirectory(stageRoot); err != nil {
		t.Fatalf("create stage: %v", err)
	}
	if err := prepareStagingLayout(stageRoot); err != nil {
		t.Fatalf("prepare stage: %v", err)
	}
	if err := createEmptyPrivateDirectory(rollbackRoot); err != nil {
		t.Fatalf("create rollback: %v", err)
	}
	shmPath := filepath.Join(dataDir, "novel-fetcher", "library.sqlite-shm")
	if err := os.WriteFile(shmPath, []byte("old shared memory"), 0o600); err != nil {
		t.Fatalf("write stale shm: %v", err)
	}
	if err := buildRestoreTransactionPlan(dataDir, &transaction); err != nil {
		t.Fatalf("buildRestoreTransactionPlan: %v", err)
	}
	if err := publishRestoreTransaction(context.Background(), dataDir, &transaction); err != nil {
		t.Fatalf("publishRestoreTransaction: %v", err)
	}
	if _, err := os.Lstat(shmPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("publish should remove stale shm: %v", err)
	}
	if err := rollbackRestoreTransaction(context.Background(), dataDir, &transaction); err != nil {
		t.Fatalf("rollbackRestoreTransaction: %v", err)
	}
	if raw, err := os.ReadFile(shmPath); err != nil || string(raw) != "old shared memory" {
		t.Fatalf("rollback should restore stale shm: raw=%q err=%v", raw, err)
	}
}

func TestRecoverFailsClosedOnMalformedJournal(t *testing.T) {
	dataDir := t.TempDir()
	if err := ensureRestoreRoots(dataDir); err != nil {
		t.Fatalf("ensureRestoreRoots: %v", err)
	}
	journal := restoreJournalPath(dataDir)
	if err := os.WriteFile(journal, []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("write malformed journal: %v", err)
	}
	if outcome, err := Recover(context.Background(), dataDir); err == nil || outcome != RecoveryNone {
		t.Fatalf("Recover malformed journal: outcome=%v err=%v", outcome, err)
	}
	if _, err := os.Lstat(journal); err != nil {
		t.Fatalf("malformed journal should remain for explicit inspection: %v", err)
	}
}

func TestRecoverFinishesCommittedGenerationCleanup(t *testing.T) {
	dataDir := t.TempDir()
	if err := ensureRestoreRoots(dataDir); err != nil {
		t.Fatalf("ensureRestoreRoots: %v", err)
	}
	transaction := newRestoreTransaction("commit-recovery-test")
	stageRoot := filepath.Join(dataDir, transaction.StageDirectory)
	rollbackRoot := filepath.Join(dataDir, transaction.RollbackDirectory)
	if err := beginRestoreTransaction(dataDir, &transaction); err != nil {
		t.Fatalf("beginRestoreTransaction: %v", err)
	}
	if err := createEmptyPrivateDirectory(stageRoot); err != nil {
		t.Fatalf("create stage: %v", err)
	}
	if err := prepareStagingLayout(stageRoot); err != nil {
		t.Fatalf("prepare stage: %v", err)
	}
	if err := createEmptyPrivateDirectory(rollbackRoot); err != nil {
		t.Fatalf("create rollback: %v", err)
	}
	library := filepath.Join(dataDir, "novel-fetcher", "library.sqlite")
	stagedLibrary := filepath.Join(stageRoot, "novel-fetcher", "library.sqlite")
	if err := os.WriteFile(library, []byte("old library"), 0o600); err != nil {
		t.Fatalf("write old library: %v", err)
	}
	if err := os.WriteFile(stagedLibrary, []byte("new library"), 0o600); err != nil {
		t.Fatalf("write staged library: %v", err)
	}
	if err := buildRestoreTransactionPlan(dataDir, &transaction); err != nil {
		t.Fatalf("buildRestoreTransactionPlan: %v", err)
	}
	if err := publishRestoreTransaction(context.Background(), dataDir, &transaction); err != nil {
		t.Fatalf("publishRestoreTransaction: %v", err)
	}
	transaction.Phase = restorePhaseCommitting
	if err := writeRestoreTransaction(dataDir, &transaction); err != nil {
		t.Fatalf("write committing transaction: %v", err)
	}
	if outcome, err := Recover(context.Background(), dataDir); err != nil || outcome != RecoveryCommittedCleanup {
		t.Fatalf("Recover committing transaction: outcome=%v err=%v", outcome, err)
	}
	if raw, err := os.ReadFile(library); err != nil || string(raw) != "new library" {
		t.Fatalf("committed library changed during cleanup: raw=%q err=%v", raw, err)
	}
	for _, path := range []string{restoreJournalPath(dataDir), stageRoot, rollbackRoot} {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("commit recovery left transaction path %s: %v", path, err)
		}
	}
}

func TestRecoverRejectsMissingBlankAndSymlinkDataRoots(t *testing.T) {
	if outcome, err := Recover(context.Background(), " "); err == nil || outcome != RecoveryNone {
		t.Fatalf("blank Recover: outcome=%v err=%v", outcome, err)
	}
	root := t.TempDir()
	missing := filepath.Join(root, "missing")
	if outcome, err := Recover(context.Background(), missing); !errors.Is(err, os.ErrNotExist) || outcome != RecoveryNone {
		t.Fatalf("missing Recover: outcome=%v err=%v", outcome, err)
	}
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink target: %v", err)
	}
	if outcome, err := Recover(context.Background(), link); err == nil || outcome != RecoveryNone {
		t.Fatalf("symlink Recover: outcome=%v err=%v", outcome, err)
	}
}

func TestRestoreTransactionRejectsInvalidJournalsAndReservedPaths(t *testing.T) {
	valid := newRestoreTransaction("validation-test")
	valid.Phase = restorePhasePublishing
	valid.Actions = make([]restoreTransactionAction, 0, len(restoreTargets()))
	for _, relative := range restoreTargets() {
		valid.Actions = append(valid.Actions, restoreTransactionAction{Relative: relative})
	}
	for _, transaction := range []*restoreTransaction{
		nil,
		func() *restoreTransaction { value := valid; value.Version = 99; return &value }(),
		func() *restoreTransaction { value := valid; value.StageDirectory = "outside"; return &value }(),
		func() *restoreTransaction { value := valid; value.Actions = value.Actions[:1]; return &value }(),
		func() *restoreTransaction {
			value := valid
			value.Actions = append([]restoreTransactionAction(nil), value.Actions...)
			value.Actions[0].Relative = "../outside"
			return &value
		}(),
		func() *restoreTransaction { value := valid; value.Phase = "unknown"; return &value }(),
		func() *restoreTransaction {
			value := newRestoreTransaction("staging-actions")
			value.Actions = []restoreTransactionAction{{Relative: "unexpected"}}
			return &value
		}(),
	} {
		if err := validateRestoreTransaction(transaction); err == nil {
			t.Fatalf("invalid transaction accepted: %+v", transaction)
		}
	}

	dataDir := t.TempDir()
	if err := ensureRestoreRoots(dataDir); err != nil {
		t.Fatalf("ensureRestoreRoots: %v", err)
	}
	staging := newRestoreTransaction("duplicate-journal")
	if err := beginRestoreTransaction(dataDir, &staging); err != nil {
		t.Fatalf("begin first transaction: %v", err)
	}
	if err := beginRestoreTransaction(dataDir, &staging); err == nil {
		t.Fatal("beginRestoreTransaction accepted an existing journal")
	}
	raw, err := json.Marshal(staging)
	if err != nil {
		t.Fatalf("marshal journal: %v", err)
	}
	raw = append(append(raw, '\n'), []byte("{}\n")...)
	if err := os.WriteFile(restoreJournalPath(dataDir), raw, 0o600); err != nil {
		t.Fatalf("write trailing journal: %v", err)
	}
	if _, err := readRestoreTransaction(dataDir); err == nil {
		t.Fatal("readRestoreTransaction accepted trailing JSON")
	}
	if err := os.Chmod(restoreJournalPath(dataDir), 0o644); err != nil {
		t.Fatalf("chmod journal: %v", err)
	}
	if _, err := readRestoreTransaction(dataDir); err == nil {
		t.Fatal("readRestoreTransaction accepted an insecure mode")
	}
	if err := writeRestoreTransaction(dataDir, nil); err == nil {
		t.Fatal("writeRestoreTransaction accepted a nil transaction")
	}
}

func TestRestoreTransactionFilesystemAndRollbackEdges(t *testing.T) {
	root := t.TempDir()
	blocked := filepath.Join(root, "blocked")
	if err := os.WriteFile(blocked, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("write blocked path: %v", err)
	}
	if err := ensureDurableDirectory(filepath.Join(blocked, "child")); err == nil {
		t.Fatal("ensureDurableDirectory accepted a file parent")
	}
	tree := filepath.Join(root, "tree")
	if err := os.Mkdir(tree, 0o700); err != nil {
		t.Fatalf("mkdir tree: %v", err)
	}
	if err := os.Symlink(blocked, filepath.Join(tree, "link")); err != nil {
		t.Fatalf("symlink tree entry: %v", err)
	}
	if err := syncRestoreTree(tree); err == nil {
		t.Fatal("syncRestoreTree accepted a symlink")
	}
	source := filepath.Join(root, "source")
	destination := filepath.Join(root, "destination")
	if err := os.WriteFile(source, []byte("fixture"), 0o600); err != nil {
		t.Fatalf("write rename source: %v", err)
	}
	if err := durableRename(source, destination); err != nil {
		t.Fatalf("same-directory durableRename: %v", err)
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := rollbackRestoreTransaction(cancelled, root, &restoreTransaction{Actions: []restoreTransactionAction{{Relative: "target"}}}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled rollback error = %v", err)
	}
	if err := rollbackRestoreTransaction(context.Background(), root, &restoreTransaction{
		StageDirectory: "stage", RollbackDirectory: "rollback",
		Actions: []restoreTransactionAction{{Relative: "missing-old", HadOld: true}},
	}); err == nil {
		t.Fatal("rollback accepted a missing old target")
	}
	unexpected := filepath.Join(root, "unexpected")
	if err := os.WriteFile(unexpected, []byte("fixture"), 0o600); err != nil {
		t.Fatalf("write unexpected target: %v", err)
	}
	if err := rollbackRestoreTransaction(context.Background(), root, &restoreTransaction{
		StageDirectory: "stage", RollbackDirectory: "rollback",
		Actions: []restoreTransactionAction{{Relative: "unexpected"}},
	}); err == nil {
		t.Fatal("rollback accepted an unexplained live target")
	}
	placed := filepath.Join(root, "placed")
	if err := os.WriteFile(placed, []byte("new target"), 0o600); err != nil {
		t.Fatalf("write placed target: %v", err)
	}
	if err := rollbackRestoreTransaction(context.Background(), root, &restoreTransaction{
		StageDirectory: "stage", RollbackDirectory: "rollback",
		Actions: []restoreTransactionAction{{Relative: "placed", HasNew: true}},
	}); err != nil {
		t.Fatalf("rollback new-only target: %v", err)
	}
	if _, err := os.Lstat(placed); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("new-only target was not removed: %v", err)
	}
}

func TestPublishRestoreTransactionCancellationAndBlockedDestination(t *testing.T) {
	dataDir := t.TempDir()
	if err := ensureRestoreRoots(dataDir); err != nil {
		t.Fatalf("ensureRestoreRoots: %v", err)
	}
	transaction := newRestoreTransaction("publish-failure-test")
	stageRoot := filepath.Join(dataDir, transaction.StageDirectory)
	rollbackRoot := filepath.Join(dataDir, transaction.RollbackDirectory)
	if err := beginRestoreTransaction(dataDir, &transaction); err != nil {
		t.Fatalf("beginRestoreTransaction: %v", err)
	}
	if err := createEmptyPrivateDirectory(stageRoot); err != nil {
		t.Fatalf("create stage: %v", err)
	}
	if err := prepareStagingLayout(stageRoot); err != nil {
		t.Fatalf("prepare stage: %v", err)
	}
	if err := createEmptyPrivateDirectory(rollbackRoot); err != nil {
		t.Fatalf("create rollback: %v", err)
	}
	stagedLibrary := filepath.Join(stageRoot, "novel-fetcher", "library.sqlite")
	if err := os.WriteFile(stagedLibrary, []byte("new library"), 0o600); err != nil {
		t.Fatalf("write staged library: %v", err)
	}
	if err := buildRestoreTransactionPlan(dataDir, &transaction); err != nil {
		t.Fatalf("buildRestoreTransactionPlan: %v", err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := publishRestoreTransaction(cancelled, dataDir, &transaction); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled publish error = %v", err)
	}
	if err := os.RemoveAll(filepath.Join(dataDir, "novel-fetcher")); err != nil {
		t.Fatalf("remove destination directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "novel-fetcher"), []byte("blocked"), 0o600); err != nil {
		t.Fatalf("write blocked destination parent: %v", err)
	}
	if err := publishRestoreTransaction(context.Background(), dataDir, &transaction); err == nil {
		t.Fatal("publish accepted a blocked destination parent")
	}
}

func TestRestoreJournalRejectsSymlinkAndInvalidPartial(t *testing.T) {
	dataDir := t.TempDir()
	if err := ensureRestoreRoots(dataDir); err != nil {
		t.Fatalf("ensureRestoreRoots: %v", err)
	}
	target := filepath.Join(dataDir, "journal-target")
	if err := os.WriteFile(target, []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("write journal target: %v", err)
	}
	if err := os.Symlink(target, restoreJournalPath(dataDir)); err != nil {
		t.Fatalf("symlink journal: %v", err)
	}
	if _, err := readRestoreTransaction(dataDir); err == nil {
		t.Fatal("readRestoreTransaction followed a symlink")
	}
	if err := os.Remove(restoreJournalPath(dataDir)); err != nil {
		t.Fatalf("remove journal symlink: %v", err)
	}
	invalidPartial := filepath.Join(dataDir, ".state-restore-transaction-invalid.partial")
	if err := os.Mkdir(invalidPartial, 0o700); err != nil {
		t.Fatalf("mkdir invalid partial: %v", err)
	}
	if err := cleanupRestoreJournalPartials(dataDir); err == nil {
		t.Fatal("cleanupRestoreJournalPartials accepted a directory")
	}
}

func TestRestoreTransactionAdditionalFailureBoundaries(t *testing.T) {
	activeRoot := t.TempDir()
	active, err := statebarrier.AcquireViewerAPI(activeRoot)
	if err != nil {
		t.Fatalf("AcquireViewerAPI: %v", err)
	}
	if outcome, err := Recover(context.Background(), activeRoot); !errors.Is(err, statebarrier.ErrWriterActive) || outcome != RecoveryNone {
		t.Fatalf("Recover with active writer: outcome=%v err=%v", outcome, err)
	}
	if err := active.Close(); err != nil {
		t.Fatalf("close active writer: %v", err)
	}

	partialRoot := t.TempDir()
	invalidPartial := filepath.Join(partialRoot, ".state-restore-transaction-invalid.partial")
	if err := os.Mkdir(invalidPartial, 0o700); err != nil {
		t.Fatalf("mkdir invalid partial: %v", err)
	}
	if err := beginRestoreTransaction(partialRoot, pointerToTransaction(newRestoreTransaction("partial-failure"))); err == nil {
		t.Fatal("beginRestoreTransaction accepted an invalid journal partial")
	}

	planRoot := t.TempDir()
	if err := ensureRestoreRoots(planRoot); err != nil {
		t.Fatalf("ensure plan roots: %v", err)
	}
	plan := newRestoreTransaction("plan-failure")
	if err := os.Mkdir(filepath.Join(planRoot, plan.StageDirectory), 0o700); err != nil {
		t.Fatalf("mkdir plan stage: %v", err)
	}
	if err := os.WriteFile(filepath.Join(planRoot, plan.StageDirectory, "novel-fetcher"), []byte("blocked"), 0o600); err != nil {
		t.Fatalf("write blocked stage parent: %v", err)
	}
	if err := buildRestoreTransactionPlan(planRoot, &plan); err == nil {
		t.Fatal("buildRestoreTransactionPlan accepted a blocked stage path")
	}

	for _, testCase := range []struct {
		name   string
		id     string
		mutate func(*restoreTransaction)
	}{
		{name: "invalid journal update", id: "invalid-update", mutate: func(transaction *restoreTransaction) { transaction.Actions[0].Relative = "../invalid" }},
		{name: "missing old source", id: "missing-old", mutate: func(transaction *restoreTransaction) { transaction.Actions[0].HadOld = true }},
		{name: "missing staged source", id: "missing-staged", mutate: func(transaction *restoreTransaction) { transaction.Actions[0].HasNew = true }},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			dataDir := t.TempDir()
			if err := ensureRestoreRoots(dataDir); err != nil {
				t.Fatalf("ensureRestoreRoots: %v", err)
			}
			transaction := plannedRestoreTransaction(testCase.id)
			testCase.mutate(&transaction)
			if err := publishRestoreTransaction(context.Background(), dataDir, &transaction); err == nil {
				t.Fatal("publishRestoreTransaction unexpectedly succeeded")
			}
		})
	}

	invalidCommit := plannedRestoreTransaction("invalid-commit")
	invalidCommit.Actions[0].Relative = "../invalid"
	if err := commitRestoreTransaction(t.TempDir(), &invalidCommit); err == nil {
		t.Fatal("commitRestoreTransaction accepted an invalid transaction")
	}

	recoveryRoot := t.TempDir()
	if err := ensureRestoreRoots(recoveryRoot); err != nil {
		t.Fatalf("ensure recovery roots: %v", err)
	}
	brokenRecovery := plannedRestoreTransaction("broken-recovery")
	brokenRecovery.Actions[0].HadOld = true
	if err := writeRestoreTransaction(recoveryRoot, &brokenRecovery); err != nil {
		t.Fatalf("write broken recovery journal: %v", err)
	}
	if outcome, err := Recover(context.Background(), recoveryRoot); err == nil || outcome != RecoveryNone {
		t.Fatalf("Recover missing old target: outcome=%v err=%v", outcome, err)
	}

	cleanupRoot := t.TempDir()
	if err := ensureRestoreRoots(cleanupRoot); err != nil {
		t.Fatalf("ensure cleanup roots: %v", err)
	}
	cleanup := newRestoreTransaction("cleanup-failure")
	if err := writeRestoreTransaction(cleanupRoot, &cleanup); err != nil {
		t.Fatalf("write cleanup journal: %v", err)
	}
	if err := os.Mkdir(filepath.Join(cleanupRoot, ".state-restore-transaction-blocked.partial"), 0o700); err != nil {
		t.Fatalf("mkdir blocked cleanup partial: %v", err)
	}
	if outcome, err := Recover(context.Background(), cleanupRoot); err == nil || outcome != RecoveryNone {
		t.Fatalf("Recover cleanup failure: outcome=%v err=%v", outcome, err)
	}

	blockedRoot := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(blockedRoot, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("write blocked root: %v", err)
	}
	transaction := newRestoreTransaction("write-failure")
	if err := writeRestoreTransaction(blockedRoot, &transaction); err == nil {
		t.Fatal("writeRestoreTransaction accepted a file data root")
	}
	if err := beginRestoreTransaction(blockedRoot, &transaction); err == nil {
		t.Fatal("beginRestoreTransaction accepted a file data root")
	}

	symlinkRoot := t.TempDir()
	target := filepath.Join(symlinkRoot, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatalf("mkdir durable directory target: %v", err)
	}
	link := filepath.Join(symlinkRoot, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink durable directory: %v", err)
	}
	if err := ensureDurableDirectory(link); err == nil {
		t.Fatal("ensureDurableDirectory accepted a symlink")
	}
	if err := syncRestoreTree(filepath.Join(symlinkRoot, "missing")); err == nil {
		t.Fatal("syncRestoreTree accepted a missing root")
	}
	if err := durableRename(filepath.Join(symlinkRoot, "missing-source"), filepath.Join(symlinkRoot, "destination")); err == nil {
		t.Fatal("durableRename accepted a missing source")
	}
	if err := ensureDurableDirectory(string(filepath.Separator)); err != nil {
		t.Fatalf("ensureDurableDirectory filesystem root: %v", err)
	}

	blockedRollbackRoot := t.TempDir()
	if err := ensureRestoreRoots(blockedRollbackRoot); err != nil {
		t.Fatalf("ensure blocked rollback roots: %v", err)
	}
	blockedRollback := plannedRestoreTransaction("blocked-rollback")
	blockedRollback.Actions[0].HadOld = true
	if err := os.WriteFile(filepath.Join(blockedRollbackRoot, "novel-fetcher", "library.sqlite"), []byte("old"), 0o600); err != nil {
		t.Fatalf("write blocked rollback old target: %v", err)
	}
	rollbackRoot := filepath.Join(blockedRollbackRoot, blockedRollback.RollbackDirectory)
	if err := os.Mkdir(rollbackRoot, 0o700); err != nil {
		t.Fatalf("mkdir blocked rollback root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(rollbackRoot, "novel-fetcher"), []byte("blocked"), 0o600); err != nil {
		t.Fatalf("write blocked rollback parent: %v", err)
	}
	if err := publishRestoreTransaction(context.Background(), blockedRollbackRoot, &blockedRollback); err == nil {
		t.Fatal("publish accepted a blocked rollback parent")
	}

	pathErrorRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(pathErrorRoot, "blocked"), []byte("file"), 0o600); err != nil {
		t.Fatalf("write rollback path blocker: %v", err)
	}
	if err := rollbackRestoreTransaction(context.Background(), pathErrorRoot, &restoreTransaction{
		RollbackDirectory: "rollback",
		Actions:           []restoreTransactionAction{{Relative: "blocked/child"}},
	}); err == nil {
		t.Fatal("rollback accepted an unreadable destination path")
	}
	if err := rollbackRestoreTransaction(context.Background(), pathErrorRoot, &restoreTransaction{
		StageDirectory:    "blocked",
		RollbackDirectory: "rollback",
		Actions:           []restoreTransactionAction{{Relative: "staged-child", HasNew: true}},
	}); err == nil {
		t.Fatal("rollback accepted an unreadable staged path")
	}
	if err := cleanupRestoreTransaction(pathErrorRoot, &restoreTransaction{RollbackDirectory: "blocked/child", StageDirectory: "stage"}); err == nil {
		t.Fatal("cleanup accepted an unreadable rollback path")
	}
	if err := cleanupRestoreJournalPartials("[invalid-glob"); err == nil {
		t.Fatal("cleanupRestoreJournalPartials accepted an invalid glob root")
	}

	invalidJSONRoot := t.TempDir()
	if err := os.WriteFile(restoreJournalPath(invalidJSONRoot), []byte("{"), 0o600); err != nil {
		t.Fatalf("write invalid JSON journal: %v", err)
	}
	if _, err := readRestoreTransaction(invalidJSONRoot); err == nil {
		t.Fatal("readRestoreTransaction accepted invalid JSON")
	}

	journalDirectoryRoot := t.TempDir()
	journalDirectory := restoreJournalPath(journalDirectoryRoot)
	if err := os.Mkdir(journalDirectory, 0o700); err != nil {
		t.Fatalf("mkdir journal directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(journalDirectory, "child"), []byte("fixture"), 0o600); err != nil {
		t.Fatalf("write journal directory child: %v", err)
	}
	if err := cleanupRestoreTransaction(journalDirectoryRoot, &restoreTransaction{RollbackDirectory: "rollback", StageDirectory: "stage"}); err == nil {
		t.Fatal("cleanup accepted a non-empty journal directory")
	}
	if err := cleanupRestoreTransaction(pathErrorRoot, &restoreTransaction{RollbackDirectory: "rollback", StageDirectory: "blocked/child"}); err == nil {
		t.Fatal("cleanup accepted an unreadable staging path")
	}

	unwritableRoot := t.TempDir()
	if err := os.Chmod(unwritableRoot, 0o500); err != nil {
		t.Fatalf("chmod unwritable root: %v", err)
	}
	if err := writeRestoreTransaction(unwritableRoot, pointerToTransaction(newRestoreTransaction("unwritable-root"))); err == nil {
		t.Fatal("writeRestoreTransaction accepted an unwritable data root")
	}
	if err := os.Chmod(unwritableRoot, 0o700); err != nil {
		t.Fatalf("restore unwritable root mode: %v", err)
	}

	removeFailureRoot := t.TempDir()
	lockedParent := filepath.Join(removeFailureRoot, "locked")
	destinationToRemove := filepath.Join(lockedParent, "destination")
	rollbackToRestore := filepath.Join(removeFailureRoot, "rollback", "locked", "destination")
	for _, path := range []string{destinationToRemove, rollbackToRestore} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatalf("mkdir rollback removal fixture: %v", err)
		}
		if err := os.WriteFile(path, []byte("fixture"), 0o600); err != nil {
			t.Fatalf("write rollback removal fixture: %v", err)
		}
	}
	if err := os.Chmod(lockedParent, 0o500); err != nil {
		t.Fatalf("chmod locked destination parent: %v", err)
	}
	if err := rollbackRestoreTransaction(context.Background(), removeFailureRoot, &restoreTransaction{
		RollbackDirectory: "rollback",
		Actions:           []restoreTransactionAction{{Relative: "locked/destination", HadOld: true}},
	}); err == nil {
		t.Fatal("rollback ignored a destination removal failure")
	}
	if err := os.Chmod(lockedParent, 0o700); err != nil {
		t.Fatalf("restore locked destination parent mode: %v", err)
	}
}

func plannedRestoreTransaction(generationID string) restoreTransaction {
	transaction := newRestoreTransaction(generationID)
	transaction.Phase = restorePhasePublishing
	for _, relative := range restoreTargets() {
		transaction.Actions = append(transaction.Actions, restoreTransactionAction{Relative: relative})
	}
	return transaction
}

func pointerToTransaction(transaction restoreTransaction) *restoreTransaction {
	return &transaction
}
