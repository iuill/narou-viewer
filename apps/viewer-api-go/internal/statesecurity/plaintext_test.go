package statesecurity

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHasLegacyPlaintextAPIKeyFindsNestedNonEmptyValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.yaml")
	for _, testCase := range []struct {
		name string
		raw  string
		want bool
	}{
		{name: "nested", raw: "profiles:\n  - credential:\n      api_key: synthetic-value\n", want: true},
		{name: "empty", raw: "api_key: '  '\n", want: false},
		{name: "encrypted", raw: "api_key_encrypted: ciphertext\n", want: false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if err := os.WriteFile(path, []byte(testCase.raw), 0o600); err != nil {
				t.Fatalf("write fixture: %v", err)
			}
			got, err := HasLegacyPlaintextAPIKey(path)
			if err != nil || got != testCase.want {
				t.Fatalf("HasLegacyPlaintextAPIKey = %v err=%v, want %v", got, err, testCase.want)
			}
		})
	}
}
