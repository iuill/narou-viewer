package statebarrier

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestAcquireViewerAPIRejectsActiveWriterAndReleases(t *testing.T) {
	dataDir := t.TempDir()
	lock, err := AcquireViewerAPI(dataDir)
	if err != nil {
		t.Fatalf("AcquireViewerAPI: %v", err)
	}
	if _, err := AcquireViewerAPI(dataDir); !errors.Is(err, ErrWriterActive) {
		t.Fatalf("second viewer lock error = %v", err)
	}
	if err := lock.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := lock.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if err := (*Lock)(nil).Close(); err != nil {
		t.Fatalf("nil lock Close: %v", err)
	}
	reacquired, err := AcquireViewerAPI(dataDir)
	if err != nil {
		t.Fatalf("reacquire viewer-api: %v", err)
	}
	defer reacquired.Close()
	info, err := os.Stat(filepath.Join(dataDir, filepath.FromSlash(ViewerAPILockRelativePath)))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("lock mode: info=%v err=%v", info, err)
	}
}

func TestAcquireRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.WriteFile(target, []byte("synthetic"), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}
	link := filepath.Join(root, "writer.lock")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	if _, err := Acquire(link); err == nil {
		t.Fatal("Acquire should refuse a symlink")
	}
}

func TestAcquireRejectsBlockedParent(t *testing.T) {
	root := t.TempDir()
	blocked := filepath.Join(root, "blocked")
	if err := os.WriteFile(blocked, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("write blocked parent: %v", err)
	}
	if _, err := Acquire(filepath.Join(blocked, "writer.lock")); err == nil {
		t.Fatal("Acquire should reject a non-directory parent")
	}
}

func TestAcquireRejectsSymlinkParentWithoutCreatingExternalLock(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(root, "outside")
	if err := os.Mkdir(outside, 0o700); err != nil {
		t.Fatalf("mkdir outside: %v", err)
	}
	linkedParent := filepath.Join(root, "linked-state")
	if err := os.Symlink(outside, linkedParent); err != nil {
		t.Fatalf("symlink lock parent: %v", err)
	}
	if _, err := Acquire(filepath.Join(linkedParent, ".writer.lock")); err == nil {
		t.Fatal("Acquire should reject a symlink parent")
	}
	if _, err := os.Stat(filepath.Join(outside, ".writer.lock")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("external lock should not be created: %v", err)
	}
}
