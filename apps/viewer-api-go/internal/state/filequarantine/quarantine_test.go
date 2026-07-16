package filequarantine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMovePreservesBytesAndAvoidsNameCollisions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.yaml")
	want := []byte("schema_version: 99\n")
	for iteration := 0; iteration < 2; iteration++ {
		if err := os.WriteFile(path, want, 0o644); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
		quarantined, err := Move(path, "unsupported")
		if err != nil {
			t.Fatalf("Move: %v", err)
		}
		if !strings.Contains(filepath.Base(quarantined), "state.yaml.unsupported-") {
			t.Fatalf("quarantine path = %q", quarantined)
		}
		raw, err := os.ReadFile(quarantined)
		if err != nil || string(raw) != string(want) {
			t.Fatalf("quarantined bytes = %q, err=%v", raw, err)
		}
	}
}
