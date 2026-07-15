package schemaguard

import (
	"errors"
	"testing"
)

func TestCheckYAMLClassifiesSchemaVersions(t *testing.T) {
	contract := Contract{
		ID:                   "TEST-YAML",
		Path:                 "state.yaml",
		Current:              3,
		ReadableLegacy:       []int{1},
		MissingPolicy:        MissingTreatAsLegacy,
		MissingLegacyVersion: 1,
	}
	tests := []struct {
		name       string
		document   string
		wantStatus Status
		wantError  bool
	}{
		{name: "current", document: "schema_version: 3\n", wantStatus: StatusCurrent},
		{name: "legacy", document: "schema_version: 1\n", wantStatus: StatusLegacy},
		{name: "missing as legacy", document: "value: true\n", wantStatus: StatusLegacy},
		{name: "future", document: "schema_version: 99\n", wantStatus: StatusFutureUnknown, wantError: true},
		{name: "unsupported legacy", document: "schema_version: 2\n", wantStatus: StatusUnsupportedLegacy, wantError: true},
		{name: "malformed", document: "schema_version: [\n", wantStatus: StatusMalformed, wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := CheckYAML([]byte(test.document), contract)
			if result.Status != test.wantStatus || (err != nil) != test.wantError {
				t.Fatalf("CheckYAML status/error = %s/%v, want %s/error=%v", result.Status, err, test.wantStatus, test.wantError)
			}
			if test.wantError {
				var guardError *GuardError
				if !errors.As(err, &guardError) || guardError.Result.Status != test.wantStatus {
					t.Fatalf("error = %#v, want GuardError with status %s", err, test.wantStatus)
				}
			}
		})
	}
}

func TestCheckYAMLMissingRejectsWithoutTreatingZeroAsMissing(t *testing.T) {
	contract := Contract{ID: "TEST-YAML", Current: 3, MissingPolicy: MissingReject}
	missing, missingErr := CheckYAML([]byte("value: true\n"), contract)
	zero, zeroErr := CheckYAML([]byte("schema_version: 0\n"), contract)
	if missingErr == nil || zeroErr == nil {
		t.Fatalf("missing/zero errors = %v/%v", missingErr, zeroErr)
	}
	if missing.Observed != nil || zero.Observed == nil || *zero.Observed != 0 {
		t.Fatalf("missing/zero observed = %v/%v", missing.Observed, zero.Observed)
	}
}

func TestCheckJSONUsesCamelCaseVersionHeader(t *testing.T) {
	contract := Contract{ID: "TEST-JSON", Current: 4}
	current, err := CheckJSON([]byte(`{"schemaVersion":4}`), contract)
	if err != nil || current.Status != StatusCurrent {
		t.Fatalf("current JSON = %#v/%v", current, err)
	}
	future, err := CheckJSON([]byte(`{"schemaVersion":99}`), contract)
	if err == nil || future.Status != StatusFutureUnknown {
		t.Fatalf("future JSON = %#v/%v", future, err)
	}
}
