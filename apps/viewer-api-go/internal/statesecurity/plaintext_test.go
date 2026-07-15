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

func TestAPIKeyVersionsUsesExactRecursiveKeys(t *testing.T) {
	versions, err := APIKeyVersions([]byte(`
shared_providers:
  openrouter:
    api_key_version: 1
profiles:
  - credentials:
      api_key_version: 99
metadata:
  not_api_key_version: 77
`))
	if err != nil || len(versions) != 2 || versions[0] != 1 || versions[1] != 99 {
		t.Fatalf("APIKeyVersions = %v err=%v", versions, err)
	}
	if _, err := APIKeyVersions([]byte("api_key_version: invalid\n")); err == nil {
		t.Fatal("APIKeyVersions should reject a non-integer version")
	}
}

func TestSecurityScansHandleMissingMalformedAndNonScalarValues(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.yaml")
	if found, exists, err := HasLegacyPlaintextAPIKeyIfExists(missing); err != nil || exists || found {
		t.Fatalf("missing plaintext scan: found=%v exists=%v err=%v", found, exists, err)
	}
	if versions, exists, err := APIKeyVersionsIfExists(missing); err != nil || exists || len(versions) != 0 {
		t.Fatalf("missing version scan: versions=%v exists=%v err=%v", versions, exists, err)
	}
	path := filepath.Join(t.TempDir(), "settings.yaml")
	if err := os.WriteFile(path, []byte("api_key: [\n"), 0o600); err != nil {
		t.Fatalf("write malformed fixture: %v", err)
	}
	if _, err := HasLegacyPlaintextAPIKey(path); err == nil {
		t.Fatal("malformed plaintext scan should fail")
	}
	for _, raw := range []string{"api_key: null\n", "api_key:\n  nested: value\n", "items:\n  - api_key: ''\n"} {
		if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
			t.Fatalf("write non-scalar fixture: %v", err)
		}
		if found, err := HasLegacyPlaintextAPIKey(path); err != nil || found {
			t.Fatalf("non-scalar scan: raw=%q found=%v err=%v", raw, found, err)
		}
	}
	if err := collectAPIKeyVersions(nil, map[int]bool{}); err != nil {
		t.Fatalf("nil version node: %v", err)
	}
	if _, err := APIKeyVersions([]byte("[malformed")); err == nil {
		t.Fatal("malformed version YAML should fail")
	}
	if containsNonEmptyAPIKey(nil) {
		t.Fatal("nil YAML node should not contain an API key")
	}
	if _, err := APIKeyVersions([]byte("outer:\n  inner:\n    api_key_version: invalid\n")); err == nil {
		t.Fatal("nested invalid API key version should propagate")
	}
}
