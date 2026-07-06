package yamlfile

import (
	"os"
	"path/filepath"
	"testing"
)

type testDocument struct {
	SchemaVersion int               `yaml:"schema_version"`
	Novels        map[string]string `yaml:"novels"`
}

func TestEnsureReadAndWriteAtomic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.yaml")
	initial := testDocument{SchemaVersion: 3, Novels: map[string]string{}}
	if err := Ensure(path, initial); err != nil {
		t.Fatalf("Ensure returned error: %v", err)
	}
	if err := Ensure(path, testDocument{SchemaVersion: 99}); err != nil {
		t.Fatalf("second Ensure returned error: %v", err)
	}
	var ensured testDocument
	if err := Read(path, &ensured); err != nil {
		t.Fatalf("Read ensured returned error: %v", err)
	}
	if ensured.SchemaVersion != 3 {
		t.Fatalf("Ensure should not overwrite existing file: %+v", ensured)
	}

	updated := testDocument{SchemaVersion: 3, Novels: map[string]string{"novel": "read"}}
	if err := WriteAtomic(path, updated); err != nil {
		t.Fatalf("WriteAtomic returned error: %v", err)
	}
	var reloaded testDocument
	if err := Read(path, &reloaded); err != nil {
		t.Fatalf("Read updated returned error: %v", err)
	}
	if reloaded.Novels["novel"] != "read" {
		t.Fatalf("unexpected reloaded document: %+v", reloaded)
	}
}

func TestBlockedParentReturnsError(t *testing.T) {
	baseDir := t.TempDir()
	blockedParent := filepath.Join(baseDir, "blocked")
	if err := os.WriteFile(blockedParent, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("write blocked parent: %v", err)
	}
	path := filepath.Join(blockedParent, "state.yaml")
	if err := Ensure(path, testDocument{}); err == nil {
		t.Fatal("Ensure should return error when parent path is blocked")
	}
	if err := WriteAtomic(path, testDocument{}); err == nil {
		t.Fatal("WriteAtomic should return error when parent path is blocked")
	}
	if err := Read(path, &testDocument{}); err == nil {
		t.Fatal("Read should return error when parent path is blocked")
	}
	if err := WriteAtomic(filepath.Join(t.TempDir(), "bad.yaml"), map[string]any{"bad": make(chan int)}); err == nil {
		t.Fatal("WriteAtomic should report marshal errors")
	}
}
