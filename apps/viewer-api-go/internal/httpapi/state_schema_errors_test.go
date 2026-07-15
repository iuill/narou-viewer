package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"narou-viewer/apps/viewer-api-go/internal/state/schemaguard"
)

func TestWriteStateSchemaError(t *testing.T) {
	_, err := schemaguard.CheckYAML(
		[]byte("schema_version: 99\n"),
		schemaguard.Contract{ID: "VA-TEST", Path: "state/test.yaml", Current: 3},
	)
	recorder := httptest.NewRecorder()
	if !writeStateSchemaError(recorder, err) {
		t.Fatal("writeStateSchemaError returned false")
	}
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusConflict)
	}
	var response apiErrorResponse
	if decodeErr := json.Unmarshal(recorder.Body.Bytes(), &response); decodeErr != nil {
		t.Fatalf("decode response: %v", decodeErr)
	}
	if response.Code != "STATE_SCHEMA_UNSUPPORTED" {
		t.Fatalf("code = %q", response.Code)
	}
	if response.Details["schemaId"] != "VA-TEST" || response.Details["status"] != "future_unknown" {
		t.Fatalf("details = %#v", response.Details)
	}
	if response.Details["observedVersion"] != float64(99) || response.Details["supportedVersion"] != float64(3) {
		t.Fatalf("version details = %#v", response.Details)
	}
}

func TestWriteStateSchemaErrorIgnoresOrdinaryErrors(t *testing.T) {
	recorder := httptest.NewRecorder()
	if writeStateSchemaError(recorder, errors.New("ordinary")) {
		t.Fatal("writeStateSchemaError returned true")
	}
	if recorder.Body.Len() != 0 {
		t.Fatalf("unexpected response body: %q", recorder.Body.String())
	}
}
