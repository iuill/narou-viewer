package library

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"narou-viewer/apps/viewer-api-go/internal/fetcher"
)

type fakeFetcherLibraryReader struct {
	works      []fetcher.LibraryWork
	episodes   map[int][]fetcher.LibraryEpisode
	payloads   map[string]fetcher.LibraryEpisodeResponse
	batchCalls int
}

func (f fakeFetcherLibraryReader) ListLibraryWorks(context.Context) ([]fetcher.LibraryWork, error) {
	return f.works, nil
}

func (f fakeFetcherLibraryReader) GetLibraryToc(_ context.Context, workID int) (fetcher.LibraryWork, []fetcher.LibraryEpisode, error) {
	for _, work := range f.works {
		if work.ID == workID {
			return work, f.episodes[workID], nil
		}
	}
	return fetcher.LibraryWork{}, nil, &fetcher.HTTPError{StatusCode: 404}
}

func (f fakeFetcherLibraryReader) GetLibraryEpisode(_ context.Context, workID int, episodeID string) (fetcher.LibraryEpisodeResponse, error) {
	payload, ok := f.payloads[strings.Join([]string{fmt.Sprint(workID), episodeID}, ":")]
	if !ok {
		return fetcher.LibraryEpisodeResponse{}, &fetcher.HTTPError{StatusCode: 404}
	}
	return payload, nil
}

func (f *fakeFetcherLibraryReader) ListLibraryTocs(_ context.Context, workIDs []int) (map[int][]fetcher.LibraryEpisode, error) {
	f.batchCalls++
	result := make(map[int][]fetcher.LibraryEpisode, len(workIDs))
	for _, workID := range workIDs {
		if episodes, ok := f.episodes[workID]; ok {
			result[workID] = episodes
		}
	}
	return result, nil
}

func TestServiceReadsNovelFetcherLibraryViaAPI(t *testing.T) {
	rootDir := t.TempDir()
	assetPath := filepath.Join(rootDir, "works/syosetu/n1234ab/assets/episodes/1/pic.jpg")
	if err := os.MkdirAll(filepath.Dir(assetPath), 0o755); err != nil {
		t.Fatalf("mkdir asset dir: %v", err)
	}
	if err := os.WriteFile(assetPath, []byte("jpg"), 0o644); err != nil {
		t.Fatalf("write asset: %v", err)
	}
	work := fetcher.LibraryWork{
		ID:                 1,
		Site:               "syosetu",
		SiteName:           "小説家になろう",
		SiteWorkID:         "n1234ab",
		SourceURL:          "https://ncode.syosetu.com/n1234ab",
		Title:              "API作品",
		Author:             "作者",
		Story:              "あらすじ",
		Directory:          "works/syosetu/n1234ab",
		FetchedAt:          "2026-06-01T00:00:00Z",
		EpisodeLen:         1,
		SavedEpisodeLen:    1,
		FetchStatus:        "complete",
		ExpectedEpisodeLen: 1,
	}
	episode := fetcher.LibraryEpisode{
		EpisodeID:    "1",
		DisplayIndex: "1",
		Title:        "第一話",
		SourceURL:    "https://ncode.syosetu.com/n1234ab/1/",
		PublishedAt:  "2026-06-01T00:00:00Z",
		ContentHash:  "sha256:episode",
		BodyStatus:   "complete",
	}
	reader := fakeFetcherLibraryReader{
		works:    []fetcher.LibraryWork{work},
		episodes: map[int][]fetcher.LibraryEpisode{1: []fetcher.LibraryEpisode{episode}},
		payloads: map[string]fetcher.LibraryEpisodeResponse{
			"1:1": {
				Work:    work,
				Episode: episode,
				Canonical: []byte(`{
					"schema_version": 1,
					"episode_id": "1",
					"display_index": "1",
					"title": "第一話",
					"blocks": [
						{"type": "html", "section": "body", "html": "<p>本文<img src=\"assets/episodes/1/pic.jpg\"></p>"}
					]
				}`),
			},
		},
	}
	service := NewServiceWithFetcher(rootDir, reader)
	novelID := NovelID(Work{ID: 1, Site: "syosetu", SiteWorkID: "n1234ab"})

	list, err := service.ListNovels(context.Background())
	if err != nil {
		t.Fatalf("ListNovels() error = %v", err)
	}
	if len(list.Novels) != 1 || list.Novels[0].NovelID != novelID || list.Novels[0].Title != "API作品" {
		t.Fatalf("unexpected list: %+v", list)
	}
	toc, err := service.GetToc(context.Background(), novelID)
	if err != nil {
		t.Fatalf("GetToc() error = %v", err)
	}
	if toc == nil || len(toc.Episodes) != 1 || toc.Episodes[0].EpisodeIndex != "1" {
		t.Fatalf("unexpected toc: %+v", toc)
	}
	response, err := service.GetEpisode(context.Background(), novelID, "1")
	if err != nil {
		t.Fatalf("GetEpisode() error = %v", err)
	}
	if response == nil || response.ContentEtag != "sha256:episode" || !strings.Contains(response.HTML, "/api/library/novels/"+novelID+"/assets/assets/episodes/1/pic.jpg") {
		t.Fatalf("unexpected episode response: %+v", response)
	}
	asset, err := service.GetAsset(context.Background(), novelID, "assets/episodes/1/pic.jpg")
	if err != nil {
		t.Fatalf("GetAsset() error = %v", err)
	}
	if asset == nil || asset.FilePath != assetPath {
		t.Fatalf("unexpected asset: %+v", asset)
	}
}

func TestServiceBatchReadsFetcherTocs(t *testing.T) {
	reader := &fakeFetcherLibraryReader{
		episodes: map[int][]fetcher.LibraryEpisode{
			1: {{EpisodeID: "1", DisplayIndex: "1", Title: "第一話"}},
			2: {{EpisodeID: "2", DisplayIndex: "2", Title: "第二話"}},
		},
	}
	service := NewServiceWithFetcher(t.TempDir(), reader)

	tocs, err := service.GetTocsByFetcherWorkIDs(context.Background(), []string{"1", "2", "3", "1", "bad"})
	if err != nil {
		t.Fatalf("GetTocsByFetcherWorkIDs() error = %v", err)
	}
	if reader.batchCalls != 1 {
		t.Fatalf("batch reader should be called once, got %d", reader.batchCalls)
	}
	if _, ok := tocs["3"]; ok {
		t.Fatalf("missing work should be omitted from batch toc result: %+v", tocs)
	}
	if len(tocs) != 2 || len(tocs["1"]) != 1 || tocs["1"][0].Title != "第一話" || len(tocs["2"]) != 1 {
		t.Fatalf("unexpected batch toc summaries: %+v", tocs)
	}
}

func TestServiceEpisodeExistsMatchesEpisodeIDDisplayIndexOrSiteEpisodeID(t *testing.T) {
	work := fetcher.LibraryWork{ID: 1, Site: "syosetu", SiteWorkID: "n1234ab", Title: "API作品"}
	reader := fakeFetcherLibraryReader{
		works: []fetcher.LibraryWork{work},
		episodes: map[int][]fetcher.LibraryEpisode{1: {
			{EpisodeID: "5", DisplayIndex: "", SiteEpisodeID: "", Title: "補完ID"},
			{EpisodeID: "episode-raw", DisplayIndex: "6", SiteEpisodeID: "site-6", Title: "サイトID"},
		}},
	}
	service := NewServiceWithFetcher(t.TempDir(), reader)
	novelID := NovelID(Work{ID: 1, Site: "syosetu", SiteWorkID: "n1234ab"})

	for _, episodeIndex := range []string{"5", "6", "site-6"} {
		novelFound, episodeFound, err := service.EpisodeExists(context.Background(), novelID, episodeIndex)
		if err != nil {
			t.Fatalf("EpisodeExists(%q) error = %v", episodeIndex, err)
		}
		if !novelFound || !episodeFound {
			t.Fatalf("EpisodeExists(%q) = novel:%v episode:%v, want true true", episodeIndex, novelFound, episodeFound)
		}
	}
}
