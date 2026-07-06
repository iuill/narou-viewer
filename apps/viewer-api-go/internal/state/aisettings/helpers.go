package aisettings

import (
	"strconv"
	"strings"
	"time"
)

func normalizeStringPtr(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func normalizeStringList(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func stringPtrOrNil(value *string) *string {
	if value == nil {
		return nil
	}
	return value
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func updatedAtOrNow(value *string, now string) *string {
	if normalized := stringPtrOrNil(value); normalized != nil {
		return normalized
	}
	return &now
}

func isoNow() string {
	return time.Now().UTC().Format(timestampFormat)
}

func intString(value int) string {
	return strconv.Itoa(value)
}
