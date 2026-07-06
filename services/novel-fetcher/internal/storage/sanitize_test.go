package storage

import "testing"

func TestSanitizeFilenameAndTruncateRunes(t *testing.T) {
	if got := sanitizeFilename("  bad/name:\t title  "); got != "bad_name_ title" {
		t.Fatalf("sanitizeFilename returned %q", got)
	}
	if got := sanitizeFilename(" \n\t "); got != "untitled" {
		t.Fatalf("sanitizeFilename blank = %q", got)
	}
	if got := truncateRunes("あいうえお", 4); got != "あいうえ" {
		t.Fatalf("truncateRunes returned %q", got)
	}
}
