package httpapi

import (
	"net/http"

	"narou-viewer/apps/viewer-api-go/internal/state/schemaguard"
)

func writeStateSchemaError(w http.ResponseWriter, err error) bool {
	guardError, ok := schemaguard.AsGuardError(err)
	if !ok {
		return false
	}
	details := map[string]any{
		"schemaId":         guardError.Result.Contract.ID,
		"path":             guardError.Result.Contract.Path,
		"status":           guardError.Result.Status.String(),
		"supportedVersion": guardError.Result.Contract.Current,
	}
	if guardError.Result.Observed != nil {
		details["observedVersion"] = *guardError.Result.Observed
	}
	code := "STATE_SCHEMA_UNSUPPORTED"
	message := "Stored state schema is not supported by this build."
	if guardError.Result.Status == schemaguard.StatusMalformed {
		code = "STATE_SCHEMA_INVALID"
		message = "Stored state could not be validated."
	}
	writeAPIError(w, http.StatusConflict, code, message, details)
	return true
}
