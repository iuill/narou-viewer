package libraryview

import (
	"context"
	"errors"
	"testing"

	"narou-viewer/apps/viewer-api-go/internal/library"
	"narou-viewer/apps/viewer-api-go/internal/publications"
	"narou-viewer/apps/viewer-api-go/internal/store"
)

type fakeLibrary struct {
	list library.NovelListResult
	tocs map[string]*library.TocResponse
}

func (f fakeLibrary) ListNovels(context.Context) (library.NovelListResult, error) {
	return f.list, nil
}

func (f fakeLibrary) GetToc(_ context.Context, novelID string) (*library.TocResponse, error) {
	return f.tocs[novelID], nil
}

type batchLibrary struct {
	list       library.NovelListResult
	tocs       map[string]*library.TocResponse
	batchTocs  map[string][]library.TocEpisodeSummary
	batchErr   error
	tocCalls   int
	batchCalls int
}

func (f *batchLibrary) ListNovels(context.Context) (library.NovelListResult, error) {
	return f.list, nil
}

func (f *batchLibrary) GetToc(context.Context, string) (*library.TocResponse, error) {
	f.tocCalls++
	return f.tocs["novel-a"], nil
}

func (f *batchLibrary) GetTocsByFetcherWorkIDs(_ context.Context, workIDs []string) (map[string][]library.TocEpisodeSummary, error) {
	f.batchCalls++
	if f.batchErr != nil {
		return nil, f.batchErr
	}
	result := make(map[string][]library.TocEpisodeSummary, len(workIDs))
	for _, workID := range workIDs {
		if episodes, ok := f.batchTocs[workID]; ok {
			result[workID] = episodes
		}
	}
	return result, nil
}

type fakeState struct {
	readingStates map[string]store.ReadingState
	bookmarks     map[string][]store.Bookmark
}

func (f fakeState) GetReadingState(novelID string) (store.ReadingState, error) {
	return f.readingStates[novelID], nil
}

func (f fakeState) ListBookmarks(novelID string) ([]store.Bookmark, error) {
	return f.bookmarks[novelID], nil
}

type fakePublications struct {
	states map[string]publications.NovelPublications
}

func (f fakePublications) Get(novelID string) (publications.NovelPublications, error) {
	return f.states[novelID], nil
}

func (f fakePublications) ListByNovelIDs(novelIDs []string) (map[string]publications.NovelPublications, error) {
	result := make(map[string]publications.NovelPublications, len(novelIDs))
	for _, novelID := range novelIDs {
		result[novelID] = f.states[novelID]
	}
	return result, nil
}

func TestListNovelsEnrichesAndSortsByActivity(t *testing.T) {
	activityAt := "2026-06-02T00:00:00.000Z"
	lastRead := "2"
	service := NewService(
		fakeLibrary{
			list: library.NovelListResult{Novels: []library.NovelSummary{
				{NovelID: "novel-b", Title: "A title", UpdatedAt: &activityAt},
				{NovelID: "novel-a", Title: "Z title", UpdatedAt: &activityAt},
			}},
			tocs: map[string]*library.TocResponse{
				"novel-a": {Episodes: []library.TocEpisodeSummary{{EpisodeIndex: "2", Title: "Last read title"}}},
			},
		},
		fakeState{
			readingStates: map[string]store.ReadingState{
				"novel-a": {NovelID: "novel-a", LastReadEpisodeIndex: &lastRead, UpdatedAt: &activityAt},
				"novel-b": {NovelID: "novel-b", UpdatedAt: &activityAt},
			},
			bookmarks: map[string][]store.Bookmark{
				"novel-a": {{NovelID: "novel-a", EpisodeIndex: "3"}},
			},
		},
		fakePublications{states: map[string]publications.NovelPublications{
			"novel-a": {
				NovelID: "novel-a",
				Entries: []publications.Entry{
					{ID: "comic", Kind: publications.KindComic, Status: publications.EntryStatusManual, ImageURL: "https://example.test/comic.jpg"},
					{ID: "novel", Kind: publications.KindNovel, Status: publications.EntryStatusManual, ImageURL: "https://example.test/novel.jpg", CoverSource: "Google Books"},
				},
			},
		}},
	)

	result, err := service.ListNovels(context.Background())
	if err != nil {
		t.Fatalf("ListNovels returned error: %v", err)
	}
	if result.Novels[0].NovelID != "novel-a" || result.Novels[1].NovelID != "novel-b" {
		t.Fatalf("same activity timestamp should be sorted by novelId: %+v", result.Novels)
	}
	novel := result.Novels[0]
	if novel.BookmarkCount != 1 || novel.LatestBookmarkEpisodeIndex == nil || *novel.LatestBookmarkEpisodeIndex != "3" {
		t.Fatalf("bookmark summary should be populated: %+v", novel)
	}
	if novel.LastReadEpisodeTitle == nil || *novel.LastReadEpisodeTitle != "Last read title" {
		t.Fatalf("last read episode title should be populated: %+v", novel)
	}
	if novel.PublicationCoverImageURL != "https://example.test/novel.jpg" || novel.PublicationCoverKind != "novel" || novel.PublicationCoverSource != "Google Books" {
		t.Fatalf("novel publication cover should be preferred: %+v", novel)
	}
}

func TestListNovelsUsesBatchTocSummariesForLastReadTitles(t *testing.T) {
	lastRead := "2"
	libraryPort := &batchLibrary{
		list: library.NovelListResult{Novels: []library.NovelSummary{
			{NovelID: "novel-a", FetcherWorkID: "101", Title: "A"},
			{NovelID: "novel-b", FetcherWorkID: "102", Title: "B"},
		}},
		batchTocs: map[string][]library.TocEpisodeSummary{
			"101": {{EpisodeIndex: "2", Title: "Batch title A"}},
			"102": {{EpisodeIndex: "2", Title: "Batch title B"}},
		},
	}
	service := NewService(
		libraryPort,
		fakeState{readingStates: map[string]store.ReadingState{
			"novel-a": {NovelID: "novel-a", LastReadEpisodeIndex: &lastRead},
			"novel-b": {NovelID: "novel-b", LastReadEpisodeIndex: &lastRead},
		}},
		nil,
	)

	result, err := service.ListNovels(context.Background())
	if err != nil {
		t.Fatalf("ListNovels returned error: %v", err)
	}
	if libraryPort.batchCalls != 1 || libraryPort.tocCalls != 0 {
		t.Fatalf("ListNovels should batch TOC lookups, batch=%d toc=%d", libraryPort.batchCalls, libraryPort.tocCalls)
	}
	titles := map[string]string{}
	for _, novel := range result.Novels {
		if novel.LastReadEpisodeTitle != nil {
			titles[novel.NovelID] = *novel.LastReadEpisodeTitle
		}
	}
	if titles["novel-a"] != "Batch title A" || titles["novel-b"] != "Batch title B" {
		t.Fatalf("last read titles should come from batch TOC summaries: %+v", result.Novels)
	}
}

func TestListNovelsFallsBackWhenBatchTocIsMissingOrFails(t *testing.T) {
	lastRead := "2"
	for _, test := range []struct {
		name      string
		batchErr  error
		batchTocs map[string][]library.TocEpisodeSummary
	}{
		{
			name:      "missing work in batch response",
			batchTocs: map[string][]library.TocEpisodeSummary{},
		},
		{
			name:      "batch request failed",
			batchErr:  errors.New("batch failed"),
			batchTocs: map[string][]library.TocEpisodeSummary{},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			libraryPort := &batchLibrary{
				list: library.NovelListResult{Novels: []library.NovelSummary{
					{NovelID: "novel-a", FetcherWorkID: "101", Title: "A"},
				}},
				tocs: map[string]*library.TocResponse{
					"novel-a": {Episodes: []library.TocEpisodeSummary{{EpisodeIndex: "2", Title: "Fallback title"}}},
				},
				batchTocs: test.batchTocs,
				batchErr:  test.batchErr,
			}
			service := NewService(
				libraryPort,
				fakeState{readingStates: map[string]store.ReadingState{
					"novel-a": {NovelID: "novel-a", LastReadEpisodeIndex: &lastRead},
				}},
				nil,
			)

			result, err := service.ListNovels(context.Background())
			if err != nil {
				t.Fatalf("ListNovels should degrade when batch TOC is incomplete: %v", err)
			}
			if libraryPort.batchCalls != 1 || libraryPort.tocCalls != 1 {
				t.Fatalf("ListNovels should fall back to single TOC lookup, batch=%d toc=%d", libraryPort.batchCalls, libraryPort.tocCalls)
			}
			title := result.Novels[0].LastReadEpisodeTitle
			if title == nil || *title != "Fallback title" {
				t.Fatalf("fallback title should be used: %+v", result.Novels[0])
			}
		})
	}
}

func TestGetTocEnrichesSummaryWithSelectedPublicationCover(t *testing.T) {
	activityAt := "2026-06-02T00:00:00.000Z"
	service := NewService(
		fakeLibrary{tocs: map[string]*library.TocResponse{
			"novel-1": {
				NovelSummary: library.NovelSummary{NovelID: "novel-1", UpdatedAt: &activityAt},
				Episodes:     []library.TocEpisodeSummary{{EpisodeIndex: "1", Title: "Episode 1"}},
			},
		}},
		fakeState{readingStates: map[string]store.ReadingState{
			"novel-1": {NovelID: "novel-1"},
		}},
		fakePublications{states: map[string]publications.NovelPublications{
			"novel-1": {
				NovelID:             "novel-1",
				DisplayCoverEntryID: "comic",
				Entries: []publications.Entry{
					{ID: "novel", Kind: publications.KindNovel, Status: publications.EntryStatusManual, ImageURL: "https://example.test/novel.jpg"},
					{ID: "comic", Kind: publications.KindComic, Status: publications.EntryStatusManual, ImageURL: "https://example.test/comic.jpg"},
				},
			},
		}},
	)

	toc, err := service.GetToc(context.Background(), "novel-1")
	if err != nil {
		t.Fatalf("GetToc returned error: %v", err)
	}
	if toc.PublicationCoverImageURL != "https://example.test/comic.jpg" || toc.PublicationCoverKind != "comic" {
		t.Fatalf("selected publication cover should be used: %+v", toc.NovelSummary)
	}
}

func TestListNovelsAllowsMissingStateService(t *testing.T) {
	service := NewService(fakeLibrary{list: library.NovelListResult{Novels: []library.NovelSummary{{NovelID: "novel-1"}}}}, nil, nil)

	result, err := service.ListNovels(context.Background())
	if err != nil {
		t.Fatalf("ListNovels returned error: %v", err)
	}
	if len(result.Novels) != 1 || result.Novels[0].NovelID != "novel-1" {
		t.Fatalf("ListNovels should pass through library result without state: %+v", result)
	}
}

func TestNilServiceReturnsEmptyViews(t *testing.T) {
	var service *Service

	result, err := service.ListNovels(context.Background())
	if err != nil || len(result.Novels) != 0 {
		t.Fatalf("nil ListNovels = %+v err=%v", result, err)
	}
	toc, err := service.GetToc(context.Background(), "novel-1")
	if err != nil || toc != nil {
		t.Fatalf("nil GetToc = %+v err=%v", toc, err)
	}
}
