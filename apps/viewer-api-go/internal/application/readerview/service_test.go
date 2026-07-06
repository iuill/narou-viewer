package readerview

import (
	"context"
	"errors"
	"testing"

	"narou-viewer/apps/viewer-api-go/internal/application/readertextcache"
	"narou-viewer/apps/viewer-api-go/internal/library"
	"narou-viewer/apps/viewer-api-go/internal/store"
)

type fakeLibrary struct {
	episode *library.EpisodeResponse
	exists  bool
}

func (f fakeLibrary) GetEpisode(context.Context, string, string) (*library.EpisodeResponse, error) {
	if f.episode == nil {
		return nil, nil
	}
	episode := *f.episode
	return &episode, nil
}

func (f fakeLibrary) NovelExists(string) (bool, error) {
	return f.exists, nil
}

type fakeState struct {
	settings store.NovelReaderSettings
	patch    store.NovelReaderCorrectionPatch
}

func (f *fakeState) GetNovelReaderSettings(string) (store.NovelReaderSettings, error) {
	return f.settings, nil
}

func (f *fakeState) PatchNovelReaderSettings(novelID string, patch store.NovelReaderCorrectionPatch) (store.NovelReaderSettings, error) {
	f.patch = patch
	f.settings.NovelID = novelID
	return f.settings, nil
}

func TestGetEpisodeAppliesReaderCorrectionsAndResponseETag(t *testing.T) {
	state := &fakeState{settings: store.NovelReaderSettings{
		Correction: store.NovelReaderCorrection{
			QuoteNormalization:                     true,
			HyphenDashNormalization:                true,
			ParenthesisNormalization:               true,
			HalfwidthAlnumPunctuationNormalization: true,
		},
	}}
	service := NewService(fakeLibrary{episode: &library.EpisodeResponse{
		NovelID:      "novel-1",
		EpisodeIndex: "1",
		ContentEtag:  "content-etag",
		ReaderDocument: library.ReaderDocument{Blocks: []library.ReaderBlock{{
			Type:    "paragraph",
			Section: "body",
			Inlines: []library.ReaderInline{{Type: "text", Text: "「quote」"}},
		}}},
	}}, state)

	view, err := service.GetEpisode(context.Background(), "novel-1", "1")
	if err != nil {
		t.Fatalf("GetEpisode returned error: %v", err)
	}
	if view.ETag != "content-etag-reader-corrections-q1h1p1a1" || view.Episode.ContentEtag != view.ETag {
		t.Fatalf("reader correction ETag should be reflected: %+v", view)
	}
}

func TestGetEpisodeWritesSearchTextCache(t *testing.T) {
	cache := readertextcache.New(t.TempDir())
	service := NewServiceWithTextCache(fakeLibrary{episode: &library.EpisodeResponse{
		NovelID:      "novel-1",
		EpisodeIndex: "1",
		ContentEtag:  "content-etag",
		ReaderDocument: library.ReaderDocument{Blocks: []library.ReaderBlock{{
			Type:    "paragraph",
			Section: "body",
			Inlines: []library.ReaderInline{{Type: "text", Text: "本文 needle"}},
		}}},
	}}, &fakeState{}, cache)

	view, err := service.GetEpisode(context.Background(), "novel-1", "1")
	if err != nil {
		t.Fatalf("GetEpisode returned error: %v", err)
	}
	baseEntry, ok, err := cache.Get(context.Background(), "novel-1", "1", "content-etag")
	if err != nil || !ok || baseEntry.Text != "本文 needle" {
		t.Fatalf("search text cache should be written with base ETag, entry=%+v ok=%v err=%v", baseEntry, ok, err)
	}
	if entry, ok, err := cache.Get(context.Background(), "novel-1", "1", view.ETag); err != nil || ok || entry.Text != "" {
		t.Fatalf("response ETag should not create a search cache row, entry=%+v ok=%v err=%v", entry, ok, err)
	}
}

func TestGetEpisodeSkipsDocumentFallbackETagSearchCache(t *testing.T) {
	cache := readertextcache.New(t.TempDir())
	fallbackETag := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	service := NewServiceWithTextCache(fakeLibrary{episode: &library.EpisodeResponse{
		NovelID:      "novel-1",
		EpisodeIndex: "1",
		ContentEtag:  fallbackETag,
		ReaderDocument: library.ReaderDocument{Blocks: []library.ReaderBlock{{
			Type:    "paragraph",
			Section: "body",
			Inlines: []library.ReaderInline{{Type: "text", Text: "本文 needle"}},
		}}},
	}}, &fakeState{}, cache)

	if _, err := service.GetEpisode(context.Background(), "novel-1", "1"); err != nil {
		t.Fatalf("GetEpisode returned error: %v", err)
	}
	if entry, ok, err := cache.Get(context.Background(), "novel-1", "1", fallbackETag); err != nil || ok || entry.Text != "" {
		t.Fatalf("document fallback ETag should not create a search cache row, entry=%+v ok=%v err=%v", entry, ok, err)
	}
}

func TestReaderSearchCacheableETag(t *testing.T) {
	cases := []struct {
		name string
		etag string
		want bool
	}{
		{name: "empty", etag: " ", want: false},
		{name: "content hash", etag: "sha256:abc", want: true},
		{name: "test etag", etag: "content-etag", want: true},
		{name: "bare lowercase hex", etag: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", want: false},
		{name: "bare uppercase hex", etag: "0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF", want: false},
		{name: "non hex sixty four chars", etag: "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz", want: true},
	}
	for _, tc := range cases {
		if got := readerSearchCacheableETag(tc.etag); got != tc.want {
			t.Fatalf("%s: readerSearchCacheableETag(%q) = %v, want %v", tc.name, tc.etag, got, tc.want)
		}
	}
}

func TestGetAndPatchSettingsUseStatePort(t *testing.T) {
	state := &fakeState{settings: store.NovelReaderSettings{NovelID: "novel-1"}}
	service := NewService(fakeLibrary{exists: true}, state)

	settings, err := service.GetSettings("novel-1")
	if err != nil || settings.NovelID != "novel-1" {
		t.Fatalf("GetSettings = %+v err=%v", settings, err)
	}
	patch := store.NovelReaderCorrectionPatch{QuoteNormalization: boolPtr(true)}
	patched, err := service.PatchSettings("novel-1", patch)
	if err != nil || patched.NovelID != "novel-1" {
		t.Fatalf("PatchSettings = %+v err=%v", patched, err)
	}
	if state.patch.QuoteNormalization == nil || !*state.patch.QuoteNormalization {
		t.Fatalf("PatchSettings should forward patch: %+v", state.patch)
	}
}

func TestPatchSettingsReturnsNovelNotFound(t *testing.T) {
	service := NewService(fakeLibrary{exists: false}, &fakeState{})

	_, err := service.PatchSettings("missing", store.NovelReaderCorrectionPatch{QuoteNormalization: boolPtr(true)})
	if !errors.Is(err, ErrNovelNotFound) {
		t.Fatalf("PatchSettings error = %v, want ErrNovelNotFound", err)
	}
}

func TestNilServiceReturnsZeroValues(t *testing.T) {
	var service *Service

	if view, err := service.GetEpisode(context.Background(), "novel-1", "1"); err != nil || view.Episode != nil || view.ETag != "" {
		t.Fatalf("nil GetEpisode = %+v err=%v", view, err)
	}
	if settings, err := service.GetSettings("novel-1"); err != nil || settings.NovelID != "" {
		t.Fatalf("nil GetSettings = %+v err=%v", settings, err)
	}
	if settings, err := service.PatchSettings("novel-1", store.NovelReaderCorrectionPatch{}); err != nil || settings.NovelID != "" {
		t.Fatalf("nil PatchSettings = %+v err=%v", settings, err)
	}
}

func boolPtr(value bool) *bool {
	return &value
}
