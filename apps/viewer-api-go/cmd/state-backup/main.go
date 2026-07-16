package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"filippo.io/age"
	"golang.org/x/sys/unix"

	"narou-viewer/apps/viewer-api-go/internal/statebackup"
)

func main() {
	runCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(runCtx, os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: state-backup <backup|restore|recover|prune> [options]")
		return 2
	}
	var err error
	switch args[0] {
	case "backup":
		err = runBackup(ctx, args[1:], stdout, stderr)
	case "restore":
		err = runRestore(ctx, args[1:], stdout, stderr)
	case "recover":
		err = runRecover(ctx, args[1:], stdout, stderr)
	case "prune":
		err = runPrune(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown state-backup command: %s\n", args[0])
		return 2
	}
	if err != nil {
		fmt.Fprintf(stderr, "state-backup: %v\n", err)
		return 1
	}
	return 0
}

func runRecover(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) error {
	flags := flag.NewFlagSet("state-backup recover", flag.ContinueOnError)
	flags.SetOutput(stderr)
	dataDir := flags.String("data-dir", defaultDataDir(), "viewer data directory")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("recover does not accept positional arguments")
	}
	outcome, err := statebackup.Recover(ctx, *dataDir)
	if err != nil {
		return err
	}
	message, err := recoveryMessage(outcome)
	if err != nil {
		return err
	}
	fmt.Fprintln(stdout, message)
	return nil
}

func recoveryMessage(outcome statebackup.RecoveryOutcome) (string, error) {
	switch outcome {
	case statebackup.RecoveryNone:
		return "no interrupted restore transaction found", nil
	case statebackup.RecoveryStagingCleanup:
		return "interrupted restore staging removed; live generation was unchanged", nil
	case statebackup.RecoveryRolledBack:
		return "interrupted restore rolled back to the previous generation and temporary data removed", nil
	case statebackup.RecoveryCommittedCleanup:
		return "committed restore generation preserved and temporary data removed", nil
	default:
		return "", fmt.Errorf("unsupported restore recovery outcome: %s", outcome)
	}
}

func runBackup(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) error {
	flags := flag.NewFlagSet("state-backup backup", flag.ContinueOnError)
	flags.SetOutput(stderr)
	dataDir := flags.String("data-dir", defaultDataDir(), "viewer data directory")
	outputDir := flags.String("output-dir", "", "private local backup directory")
	recipientValue := flags.String("recipient", "", "age X25519 or hybrid public recipient")
	passphraseFile := flags.String("passphrase-file", "", "0600 file containing a backup-only passphrase")
	keyReference := flags.String("key-reference", "", "non-secret backup key identifier")
	build := flags.String("build", detectedBuild(), "application build identifier recorded in the manifest")
	keep := flags.Int("keep", 7, "minimum number of newest local archives to retain")
	maxAge := flags.Duration("max-age", 30*24*time.Hour, "delete archives older than this after preserving --keep")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("backup does not accept positional arguments")
	}
	if strings.TrimSpace(*outputDir) == "" || strings.TrimSpace(*keyReference) == "" {
		return errors.New("--output-dir and --key-reference are required")
	}
	recipient, err := backupRecipient(*recipientValue, *passphraseFile)
	if err != nil {
		return err
	}
	result, err := statebackup.Backup(ctx, statebackup.BackupOptions{
		DataDir:          *dataDir,
		OutputDir:        *outputDir,
		ApplicationBuild: *build,
		KeyReference:     *keyReference,
		Recipient:        recipient,
		Retention:        &statebackup.RetentionPolicy{KeepGenerations: *keep, MaxAge: *maxAge},
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "backup created: %s\ngeneration: %s\n", result.ArchivePath, result.Manifest.GenerationID)
	if len(result.Pruned) > 0 {
		fmt.Fprintf(stdout, "retention pruned: %d archive(s)\n", len(result.Pruned))
	}
	return nil
}

func runRestore(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) error {
	flags := flag.NewFlagSet("state-backup restore", flag.ContinueOnError)
	flags.SetOutput(stderr)
	dataDir := flags.String("data-dir", defaultDataDir(), "viewer data directory")
	archivePath := flags.String("archive", "", "encrypted archive path")
	identityFile := flags.String("identity-file", "", "0600 file containing an age identity")
	passphraseFile := flags.String("passphrase-file", "", "0600 file containing the backup passphrase")
	keyReference := flags.String("key-reference", "", "expected non-secret backup key identifier")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("restore does not accept positional arguments")
	}
	if strings.TrimSpace(*archivePath) == "" || strings.TrimSpace(*keyReference) == "" {
		return errors.New("--archive and --key-reference are required")
	}
	identities, err := restoreIdentities(*identityFile, *passphraseFile)
	if err != nil {
		return err
	}
	result, err := statebackup.Restore(ctx, statebackup.RestoreOptions{
		DataDir:      *dataDir,
		ArchivePath:  *archivePath,
		KeyReference: *keyReference,
		Identities:   identities,
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "restore completed: generation=%s\n", result.Manifest.GenerationID)
	fmt.Fprintf(stdout, "post-restore: inventory=%d warnings=%d errors=%d repairable=%d\n", result.Report.Summary.Inventory, result.Report.Summary.Warnings, result.Report.Summary.Errors, result.Report.Summary.Repairable)
	return nil
}

func runPrune(args []string, stdout io.Writer, stderr io.Writer) error {
	flags := flag.NewFlagSet("state-backup prune", flag.ContinueOnError)
	flags.SetOutput(stderr)
	outputDir := flags.String("output-dir", "", "private local backup directory")
	keep := flags.Int("keep", 7, "minimum number of newest local archives to retain")
	maxAge := flags.Duration("max-age", 30*24*time.Hour, "delete archives older than this after preserving --keep")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || strings.TrimSpace(*outputDir) == "" {
		return errors.New("prune requires --output-dir and does not accept positional arguments")
	}
	removed, err := statebackup.PruneArchives(*outputDir, statebackup.RetentionPolicy{KeepGenerations: *keep, MaxAge: *maxAge})
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "retention pruned: %d archive(s)\n", len(removed))
	return nil
}

func backupRecipient(recipientValue string, passphraseFile string) (age.Recipient, error) {
	recipientValue = strings.TrimSpace(recipientValue)
	passphraseFile = strings.TrimSpace(passphraseFile)
	if (recipientValue == "") == (passphraseFile == "") {
		return nil, errors.New("specify exactly one of --recipient or --passphrase-file")
	}
	if recipientValue != "" {
		recipients, err := age.ParseRecipients(strings.NewReader(recipientValue + "\n"))
		if err != nil || len(recipients) != 1 {
			return nil, errors.New("--recipient must contain exactly one valid age public recipient")
		}
		switch recipients[0].(type) {
		case *age.X25519Recipient, *age.HybridRecipient:
		default:
			return nil, errors.New("--recipient must use a native age X25519 or hybrid recipient")
		}
		return recipients[0], nil
	}
	passphrase, err := readSecretFile(passphraseFile)
	if err != nil {
		return nil, err
	}
	return age.NewScryptRecipient(passphrase)
}

func restoreIdentities(identityFile string, passphraseFile string) ([]age.Identity, error) {
	identityFile = strings.TrimSpace(identityFile)
	passphraseFile = strings.TrimSpace(passphraseFile)
	if (identityFile == "") == (passphraseFile == "") {
		return nil, errors.New("specify exactly one of --identity-file or --passphrase-file")
	}
	if passphraseFile != "" {
		passphrase, err := readSecretFile(passphraseFile)
		if err != nil {
			return nil, err
		}
		identity, err := age.NewScryptIdentity(passphrase)
		if err != nil {
			return nil, err
		}
		return []age.Identity{identity}, nil
	}
	file, err := openPrivateRegularFile(identityFile)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, (1<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > 1<<20 {
		return nil, errors.New("identity file exceeds size limit")
	}
	identities, err := age.ParseIdentities(bytes.NewReader(raw))
	if err != nil || len(identities) == 0 {
		return nil, errors.New("identity file does not contain a valid age identity")
	}
	for _, identity := range identities {
		switch identity.(type) {
		case *age.X25519Identity, *age.HybridIdentity:
		default:
			return nil, errors.New("identity file must contain only native age X25519 or hybrid identities")
		}
	}
	return identities, nil
}

func readSecretFile(path string) (string, error) {
	file, err := openPrivateRegularFile(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, (64<<10)+1))
	if err != nil {
		return "", err
	}
	if len(raw) > 64<<10 {
		return "", errors.New("secret file exceeds size limit")
	}
	value := strings.TrimRight(string(raw), "\r\n")
	if value == "" {
		return "", errors.New("secret file is empty")
	}
	return value, nil
}

func openPrivateRegularFile(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = unix.Close(fd)
		return nil, errors.New("invalid secret or identity file descriptor")
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, errors.New("secret or identity file must be a regular non-symlink file")
	}
	if info.Mode().Perm()&0o177 != 0 {
		_ = file.Close()
		return nil, fmt.Errorf("secret or identity file mode must be 0600 or stricter, got %04o", info.Mode().Perm())
	}
	return file, nil
}

func defaultDataDir() string {
	if value := strings.TrimSpace(os.Getenv("VIEWER_API_DATA_DIR")); value != "" {
		return value
	}
	if value := strings.TrimSpace(os.Getenv("DATA_DIR")); value != "" {
		return value
	}
	return filepath.Clean("../../data")
}

func detectedBuild() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "development"
	}
	revision := ""
	modified := false
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			modified = setting.Value == "true"
		}
	}
	if revision == "" {
		return "development"
	}
	if len(revision) > 12 {
		revision = revision[:12]
	}
	if modified {
		revision += "-dirty"
	}
	return revision
}
