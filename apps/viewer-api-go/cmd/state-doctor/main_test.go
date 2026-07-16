package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"narou-viewer/apps/viewer-api-go/internal/statedoctor"
)

func TestRunReturnsDocumentedExitCodesAndJSON(t *testing.T) {
	dataDir := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := run(context.Background(), []string{"--data-dir", dataDir, "--format", "json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("clean run code=%d stderr=%s", code, stderr.String())
	}
	var report statedoctor.Report
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil || report.DataDir != dataDir {
		t.Fatalf("decode JSON report: report=%+v err=%v", report, err)
	}

	stateDir := filepath.Join(dataDir, "state")
	if err := os.MkdirAll(filepath.Join(stateDir, "character_events"), 0o755); err != nil {
		t.Fatalf("mkdir fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "character_events", "future.yaml"), []byte("schema_version: 99\nnovel_id: future\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := run(context.Background(), []string{"--data-dir", dataDir}, &stdout, &stderr); code != 1 {
		t.Fatalf("finding run code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte("schema_future_unknown")) {
		t.Fatalf("human report missing finding: %s", stdout.String())
	}

	if code := run(context.Background(), []string{"--data-dir", dataDir, "--apply"}, &stdout, &stderr); code != 2 {
		t.Fatalf("invalid apply code=%d", code)
	}
}

func TestStringListCollectsRepeatedFindingFlags(t *testing.T) {
	var values stringList
	if err := values.Set("finding-a"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := values.Set("finding-b"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if values.String() != "finding-a,finding-b" {
		t.Fatalf("String = %q", values.String())
	}
}
