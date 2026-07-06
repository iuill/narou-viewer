package pathutil

import "testing"

func TestFilenameSanitizesAndTruncates(t *testing.T) {
	if got := Filename("  bad/name:?  "); got != "bad_name__" {
		t.Fatalf("Filename returned %q", got)
	}
	if got := Filename("\t"); got != "untitled" {
		t.Fatalf("Filename blank = %q", got)
	}
	if got := TruncateRunes("あいうえお", 3); got != "あいう" {
		t.Fatalf("TruncateRunes = %q", got)
	}
}

func TestSegmentNormalizesUnsafePathSegments(t *testing.T) {
	tests := map[string]string{
		"..":          "untitled",
		" ..bad/name": "bad_name",
		"a b/c:*?":    "a_b_c___",
	}
	for input, want := range tests {
		if got := Segment(input); got != want {
			t.Fatalf("Segment(%q) = %q, want %q", input, got, want)
		}
	}
}
