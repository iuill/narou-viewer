package pathutil

import (
	"strings"
	"unicode/utf8"
)

func Filename(value string) string {
	replacer := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		":", "_",
		"*", "_",
		"?", "_",
		"\"", "_",
		"<", "_",
		">", "_",
		"|", "_",
		"\n", " ",
		"\r", " ",
		"\t", " ",
	)
	sanitized := strings.Trim(replacer.Replace(value), " .")
	for strings.Contains(sanitized, "  ") {
		sanitized = strings.ReplaceAll(sanitized, "  ", " ")
	}
	if sanitized == "" {
		return "untitled"
	}
	return TruncateRunes(sanitized, 80)
}

func Segment(value string) string {
	sanitized := strings.ReplaceAll(Filename(value), " ", "_")
	if sanitized == "." || sanitized == ".." || sanitized == "" {
		return "untitled"
	}
	return sanitized
}

func TruncateRunes(value string, limit int) string {
	if utf8.RuneCountInString(value) <= limit {
		return value
	}

	var builder strings.Builder
	for _, char := range value {
		if utf8.RuneCountInString(builder.String()) >= limit {
			break
		}
		builder.WriteRune(char)
	}
	return strings.TrimSpace(builder.String())
}
