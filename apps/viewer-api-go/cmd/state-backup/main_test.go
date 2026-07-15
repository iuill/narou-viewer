package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
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
	link := filepath.Join(root, "passphrase-link")
	if err := os.Symlink(passphrasePath, link); err != nil {
		t.Fatalf("symlink passphrase: %v", err)
	}
	if _, err := readSecretFile(link); err == nil {
		t.Fatal("readSecretFile should reject symlinks")
	}
}
