package server

import (
	"encoding/json"
	"net/http"
)

func writeEnvelope(writer http.ResponseWriter, status int, data any, message any) {
	writeJSON(writer, status, map[string]any{
		"success": true,
		"data":    data,
		"message": message,
	})
}

func writeError(writer http.ResponseWriter, status int, message string) {
	writeJSON(writer, status, map[string]any{
		"success": false,
		"error": map[string]any{
			"message": message,
		},
	})
}

func writeJSON(writer http.ResponseWriter, status int, payload any) {
	writer.Header().Set("content-type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(payload)
}
