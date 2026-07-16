package safefile

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestReadRegularReadsBoundedRegularFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.yaml")
	if err := os.WriteFile(path, []byte("schema_version: 1\n"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	raw, err := ReadRegular(path, 1024)
	if err != nil || string(raw) != "schema_version: 1\n" {
		t.Fatalf("ReadRegular = %q, %v", raw, err)
	}
	if _, err := ReadRegular(path, 4); err == nil {
		t.Fatal("ReadRegular accepted a file over the configured limit")
	}
	if _, err := ReadRegular(filepath.Join(t.TempDir(), "missing"), 1024); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing error = %v", err)
	}
	if _, err := ReadRegular(path, 0); err == nil {
		t.Fatal("ReadRegular accepted a non-positive limit")
	}
	if _, err := ReadRegular(t.TempDir(), 1024); err == nil {
		t.Fatal("ReadRegular accepted a directory")
	}
}

func TestReadRegularRejectsFIFOWithoutBlocking(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.yaml")
	if err := unix.Mkfifo(path, 0o600); err != nil {
		t.Fatalf("mkfifo: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := ReadRegular(path, 1024)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("ReadRegular accepted a FIFO")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ReadRegular blocked while opening a FIFO without a writer")
	}
}
