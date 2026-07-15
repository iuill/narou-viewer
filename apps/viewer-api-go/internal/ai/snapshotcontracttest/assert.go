package snapshotcontracttest

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"unicode"
)

// AssertSafeProducerSnapshot pins the producer-side contract documented in
// docs/state-schema-policy.md section 4.4. The usage store intentionally does
// not perform generic redaction, so known producers must only emit bounded
// strings and must not emit credential-shaped fields or values.
func AssertSafeProducerSnapshot(t testing.TB, snapshot any, maxStringRunes int) {
	t.Helper()
	raw, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("encode producer snapshot: %v", err)
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("decode producer snapshot: %v", err)
	}
	visit(t, "$", decoded, maxStringRunes)
}

func visit(t testing.TB, path string, value any, maxStringRunes int) {
	t.Helper()
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			if isCredentialKey(key) {
				t.Fatalf("producer snapshot contains credential field %s.%s", path, key)
			}
			visit(t, path+"."+key, item, maxStringRunes)
		}
	case []any:
		for index, item := range typed {
			visit(t, fmt.Sprintf("%s[%d]", path, index), item, maxStringRunes)
		}
	case string:
		if maxStringRunes > 0 && len([]rune(typed)) > maxStringRunes {
			t.Fatalf("producer snapshot string at %s has %d runes, limit is %d", path, len([]rune(typed)), maxStringRunes)
		}
		if isProviderSecretValue(typed) {
			t.Fatalf("producer snapshot contains a provider-secret-shaped value at %s", path)
		}
	}
}

func isCredentialKey(key string) bool {
	normalized := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, key)
	switch normalized {
	case "apikey", "authorization", "credential", "credentials", "password", "passphrase", "providersecret", "secret":
		return true
	default:
		return false
	}
}

func isProviderSecretValue(value string) bool {
	trimmed := strings.TrimSpace(value)
	lower := strings.ToLower(trimmed)
	return strings.HasPrefix(lower, "bearer ") ||
		strings.HasPrefix(lower, "sk-") ||
		strings.HasPrefix(lower, "sk_live_") ||
		strings.HasPrefix(lower, "sk_test_") ||
		strings.HasPrefix(trimmed, "AIza")
}
