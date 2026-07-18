package writerlock

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestAcquireRejectsConcurrentWriterAndSymlink(t *testing.T) {
	dataDir := t.TempDir()
	lock, err := Acquire(dataDir)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if _, err := Acquire(dataDir); !errors.Is(err, ErrWriterActive) {
		t.Fatalf("concurrent Acquire error = %v", err)
	}
	if err := lock.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := lock.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	info, err := os.Stat(filepath.Join(dataDir, FileName))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("lock mode: info=%v err=%v", info, err)
	}

	symlinkDir := t.TempDir()
	target := filepath.Join(symlinkDir, "target")
	if err := os.WriteFile(target, []byte("synthetic"), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}
	if err := os.Symlink(target, filepath.Join(symlinkDir, FileName)); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	if _, err := Acquire(symlinkDir); err == nil {
		t.Fatal("Acquire should refuse a symlink")
	}
}

func TestAcquireRejectsSymlinkDataDirectory(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(root, "outside")
	if err := os.Mkdir(outside, 0o700); err != nil {
		t.Fatalf("mkdir outside: %v", err)
	}
	linked := filepath.Join(root, "linked")
	if err := os.Symlink(outside, linked); err != nil {
		t.Fatalf("symlink data directory: %v", err)
	}
	if _, err := Acquire(linked); err == nil {
		t.Fatal("Acquire should reject a symlink data directory")
	}
	if _, err := os.Stat(filepath.Join(outside, FileName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("external lock should not be created: %v", err)
	}
}
