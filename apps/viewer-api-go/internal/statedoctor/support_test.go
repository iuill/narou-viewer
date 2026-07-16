package statedoctor

import "testing"

func TestSupportsSchemaVersionUsesExplicitAndMigrationContracts(t *testing.T) {
	for _, test := range []struct {
		id      string
		version int
		want    bool
	}{
		{id: "VA-READING", version: 3, want: true},
		{id: "VA-READING", version: 2, want: false},
		{id: "VA-CHAR-EVENTS", version: 0, want: true},
		{id: "VA-AI-USAGE", version: 0, want: true},
		{id: "VA-AI-USAGE", version: 2, want: false},
		{id: "NF-LIBRARY", version: 2, want: true},
		{id: "UNKNOWN", version: 1, want: false},
	} {
		if got := SupportsSchemaVersion(test.id, test.version); got != test.want {
			t.Fatalf("SupportsSchemaVersion(%q, %d) = %v, want %v", test.id, test.version, got, test.want)
		}
	}
}
