package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"narou-viewer/apps/viewer-api-go/internal/statebackup"
)

func TestRunRejectsUnknownAndIncompleteCommands(t *testing.T) {
	for _, args := range [][]string{
		nil,
		{"unknown"},
		{"backup"},
		{"restore"},
		{"prune"},
	} {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		if code := run(context.Background(), args, &stdout, &stderr); code == 0 {
			t.Fatalf("run(%v) unexpectedly succeeded: stdout=%s", args, stdout.String())
		}
	}
}

func TestPrivateSecretReadUsesValidatedFileDescriptor(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "secret")
	replacement := filepath.Join(root, "replacement")
	if err := os.WriteFile(path, []byte("validated fixture"), 0o600); err != nil {
		t.Fatalf("write validated fixture: %v", err)
	}
	if err := os.WriteFile(replacement, []byte("replacement fixture"), 0o600); err != nil {
		t.Fatalf("write replacement fixture: %v", err)
	}
	file, err := openPrivateRegularFile(path)
	if err != nil {
		t.Fatalf("openPrivateRegularFile: %v", err)
	}
	defer file.Close()
	if err := os.Rename(replacement, path); err != nil {
		t.Fatalf("replace path after open: %v", err)
	}
	raw, err := io.ReadAll(file)
	if err != nil || string(raw) != "validated fixture" {
		t.Fatalf("validated descriptor content=%q err=%v", raw, err)
	}
}

func TestRunRecoverReportsCleanDataTree(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := run(context.Background(), []string{"recover", "--data-dir", t.TempDir()}, &stdout, &stderr); code != 0 {
		t.Fatalf("recover code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte("no interrupted restore")) {
		t.Fatalf("recover output=%q", stdout.String())
	}
}

func TestRecoveryMessagesDistinguishRollbackFromCommittedCleanup(t *testing.T) {
	rolledBack, err := recoveryMessage(statebackup.RecoveryRolledBack)
	if err != nil || !bytes.Contains([]byte(rolledBack), []byte("rolled back")) {
		t.Fatalf("rollback message=%q err=%v", rolledBack, err)
	}
	committed, err := recoveryMessage(statebackup.RecoveryCommittedCleanup)
	if err != nil || !bytes.Contains([]byte(committed), []byte("preserved")) || bytes.Contains([]byte(committed), []byte("rolled back")) {
		t.Fatalf("committed cleanup message=%q err=%v", committed, err)
	}
	if _, err := recoveryMessage(statebackup.RecoveryOutcome("unsupported")); err == nil {
		t.Fatal("unsupported recovery outcome should fail")
	}
}

func TestCredentialInputsRequirePrivateRegularFiles(t *testing.T) {
	root := t.TempDir()
	passphrasePath := filepath.Join(root, "backup-passphrase")
	if err := os.WriteFile(passphrasePath, []byte("synthetic passphrase\n"), 0o600); err != nil {
		t.Fatalf("write passphrase: %v", err)
	}
	if recipient, err := backupRecipient("", passphrasePath); err != nil || recipient == nil {
		t.Fatalf("backupRecipient: recipient=%v err=%v", recipient, err)
	}
	if identities, err := restoreIdentities("", passphrasePath); err != nil || len(identities) != 1 {
		t.Fatalf("restoreIdentities: identities=%v err=%v", identities, err)
	}
	if _, err := backupRecipient("", ""); err == nil {
		t.Fatal("backupRecipient should require one credential source")
	}
	if err := os.Chmod(passphrasePath, 0o644); err != nil {
		t.Fatalf("chmod passphrase: %v", err)
	}
	if _, err := readSecretFile(passphrasePath); err == nil {
		t.Fatal("readSecretFile should reject group-readable files")
	}
	if err := os.Chmod(passphrasePath, 0o700); err != nil {
		t.Fatalf("chmod passphrase executable: %v", err)
	}
	if _, err := readSecretFile(passphrasePath); err == nil {
		t.Fatal("readSecretFile should reject executable files")
	}
	link := filepath.Join(root, "passphrase-link")
	if err := os.Symlink(passphrasePath, link); err != nil {
		t.Fatalf("symlink passphrase: %v", err)
	}
	if _, err := readSecretFile(link); err == nil {
		t.Fatal("readSecretFile should reject symlinks")
	}
}
