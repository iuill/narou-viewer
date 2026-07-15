package schemaguardtest

import (
	"bytes"
	"os"
	"testing"
)

func AssertFileUntouched(t *testing.T, path string, action func() error) error {
	t.Helper()
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture before action: %v", err)
	}
	actionErr := action()
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture after action: %v", err)
	}
	if !bytes.Equal(after, before) {
		t.Fatalf("file bytes changed during rejected action\nbefore=%q\nafter=%q", before, after)
	}
	return actionErr
}
