package libraryview

import (
	"context"
	"sort"
	"strings"
	"time"

	"narou-viewer/apps/viewer-api-go/internal/library"
	"narou-viewer/apps/viewer-api-go/internal/publications"
	"narou-viewer/apps/viewer-api-go/internal/store"
)

type LibraryPort interface {
	ListNovels(context.Context) (library.NovelListResult, error)
	GetToc(context.Context, string) (*library.TocResponse, error)
}

type TocBatchPort interface {
	GetTocsByFetcherWorkIDs(context.Context, []string) (map[string][]library.TocEpisodeSummary, error)
}

type StatePort interface {
	GetReadingState(string) (store.ReadingState, error)
	ListBookmarks(string) ([]store.Bookmark, error)
}

type PublicationPort interface {
	Get(string) (publications.NovelPublications, error)
	ListByNovelIDs([]string) (map[string]publications.NovelPublications, error)
}

type Service struct {
	library     LibraryPort
	state       StatePort
	publication PublicationPort
}

func NewService(library LibraryPort, state StatePort, publication PublicationPort) *Service {
	return &Service{library: library, state: state, publication: publication}
}

func (s *Service) ListNovels(ctx context.Context) (library.NovelListResult, error) {
	if s == nil || s.library == nil {
		return library.NovelListResult{}, nil
	}
	result, err := s.library.ListNovels(ctx)
	if err != nil {
		return library.NovelListResult{}, err
	}
	if err := s.populateNovelStateSummaries(ctx, &result); err != nil {
		return library.NovelListResult{}, err
	}
	return result, nil
}

func (s *Service) GetToc(ctx context.Context, novelID string) (*library.TocResponse, error) {
	if s == nil || s.library == nil {
		return nil, nil
	}
	toc, err := s.library.GetToc(ctx, novelID)
	if err != nil || toc == nil {
		return toc, err
	}
	if err := s.populateNovelStateSummary(ctx, &toc.NovelSummary, toc.Episodes, nil); err != nil {
		return nil, err
	}
	return toc, nil
}

func (s *Service) populateNovelStateSummaries(ctx context.Context, result *library.NovelListResult) error {
	if s == nil || s.state == nil || result == nil {
		return nil
	}
	var publicationStates map[string]publications.NovelPublications
	if s.publication != nil {
		novelIDs := make([]string, 0, len(result.Novels))
		for _, novel := range result.Novels {
			novelIDs = append(novelIDs, novel.NovelID)
		}
		states, err := s.publication.ListByNovelIDs(novelIDs)
		if err != nil {
			return err
		}
		publicationStates = states
	}
	readingStates := make(map[string]store.ReadingState, len(result.Novels))
	lastReadWorkIDs := []string{}
	for _, novel := range result.Novels {
		readingState, err := s.state.GetReadingState(novel.NovelID)
		if err != nil {
			return err
		}
		readingStates[novel.NovelID] = readingState
		if readingState.LastReadEpisodeIndex != nil && strings.TrimSpace(novel.FetcherWorkID) != "" {
			lastReadWorkIDs = append(lastReadWorkIDs, novel.FetcherWorkID)
		}
	}
	tocSummaries, err := s.tocSummariesForFetcherWorkIDs(ctx, lastReadWorkIDs)
	if err != nil {
		tocSummaries = map[string][]library.TocEpisodeSummary{}
	}
	for index := range result.Novels {
		var publicationState *publications.NovelPublications
		if publicationStates != nil {
			state := publicationStates[result.Novels[index].NovelID]
			publicationState = &state
		}
		episodes := tocSummaries[result.Novels[index].FetcherWorkID]
		readingState := readingStates[result.Novels[index].NovelID]
		if err := s.populateNovelStateSummaryWithState(ctx, &result.Novels[index], episodes, publicationState, readingState); err != nil {
			return err
		}
	}
	sortNovelSummariesByActivity(result.Novels)
	return nil
}

func (s *Service) tocSummariesForFetcherWorkIDs(ctx context.Context, workIDs []string) (map[string][]library.TocEpisodeSummary, error) {
	result := map[string][]library.TocEpisodeSummary{}
	batchLibrary, ok := s.library.(TocBatchPort)
	if !ok {
		return result, nil
	}
	if len(workIDs) == 0 {
		return result, nil
	}
	return batchLibrary.GetTocsByFetcherWorkIDs(ctx, workIDs)
}

func (s *Service) populateNovelStateSummary(ctx context.Context, novel *library.NovelSummary, episodes []library.TocEpisodeSummary, publicationState *publications.NovelPublications) error {
	if s == nil || s.state == nil || novel == nil {
		return nil
	}
	readingState, err := s.state.GetReadingState(novel.NovelID)
	if err != nil {
		return err
	}
	return s.populateNovelStateSummaryWithState(ctx, novel, episodes, publicationState, readingState)
}

func (s *Service) populateNovelStateSummaryWithState(ctx context.Context, novel *library.NovelSummary, episodes []library.TocEpisodeSummary, publicationState *publications.NovelPublications, readingState store.ReadingState) error {
	if s == nil || s.state == nil || novel == nil {
		return nil
	}
	novel.LastReadEpisodeIndex = readingState.LastReadEpisodeIndex
	novel.LastActivityAt = latestActivityTimestamp(novel.UpdatedAt, readingState.UpdatedAt)
	if readingState.LastReadEpisodeIndex != nil {
		if episodes == nil && s.library != nil {
			if toc, err := s.library.GetToc(ctx, novel.NovelID); err == nil && toc != nil {
				episodes = toc.Episodes
			}
		}
		novel.LastReadEpisodeTitle = episodeTitleByIndex(episodes, *readingState.LastReadEpisodeIndex)
	}
	bookmarks, err := s.state.ListBookmarks(novel.NovelID)
	if err != nil {
		return err
	}
	novel.BookmarkCount = len(bookmarks)
	if len(bookmarks) > 0 {
		novel.LatestBookmarkEpisodeIndex = &bookmarks[0].EpisodeIndex
	}
	return s.populateNovelPublicationCover(novel, publicationState)
}

func (s *Service) populateNovelPublicationCover(novel *library.NovelSummary, publicationState *publications.NovelPublications) error {
	if s == nil || s.publication == nil || novel == nil {
		return nil
	}
	var state publications.NovelPublications
	if publicationState == nil {
		nextState, err := s.publication.Get(novel.NovelID)
		if err != nil {
			return err
		}
		state = nextState
		publicationState = &state
	}
	if publicationState.DisplayCoverEntryID != "" {
		for _, entry := range publicationState.Entries {
			if entry.ID != publicationState.DisplayCoverEntryID || entry.Status == publications.EntryStatusDisabled || entry.ImageURL == "" {
				continue
			}
			setPublicationCover(novel, entry)
			return nil
		}
	}
	for _, preferredKind := range []publications.Kind{publications.KindNovel, publications.KindComic} {
		for _, entry := range publicationState.Entries {
			if entry.Kind != preferredKind || entry.Status == publications.EntryStatusDisabled || entry.ImageURL == "" {
				continue
			}
			setPublicationCover(novel, entry)
			return nil
		}
	}
	return nil
}

func setPublicationCover(novel *library.NovelSummary, entry publications.Entry) {
	novel.PublicationCoverImageURL = entry.ImageURL
	novel.PublicationCoverKind = string(entry.Kind)
	novel.PublicationCoverSource = entry.CoverSource
	novel.PublicationCoverSourceURL = entry.CoverSourceURL
}

func sortNovelSummariesByActivity(novels []library.NovelSummary) {
	sort.SliceStable(novels, func(i, j int) bool {
		leftActivityAt := novels[i].LastActivityAt
		rightActivityAt := novels[j].LastActivityAt
		if !sameTimestamp(leftActivityAt, rightActivityAt) {
			return timestampAfter(leftActivityAt, rightActivityAt)
		}
		return novels[i].NovelID < novels[j].NovelID
	})
}

func latestActivityTimestamp(values ...*string) *string {
	var latest *string
	for _, value := range values {
		if timestampAfter(value, latest) {
			latest = value
		}
	}
	return latest
}

func sameTimestamp(left *string, right *string) bool {
	leftValue := derefTimestamp(left)
	rightValue := derefTimestamp(right)
	if leftValue == rightValue {
		return true
	}
	leftTime, leftErr := time.Parse(time.RFC3339Nano, leftValue)
	rightTime, rightErr := time.Parse(time.RFC3339Nano, rightValue)
	return leftErr == nil && rightErr == nil && leftTime.Equal(rightTime)
}

func timestampAfter(left *string, right *string) bool {
	leftValue := derefTimestamp(left)
	rightValue := derefTimestamp(right)
	if leftValue == "" {
		return false
	}
	if rightValue == "" {
		return true
	}
	leftTime, leftErr := time.Parse(time.RFC3339Nano, leftValue)
	rightTime, rightErr := time.Parse(time.RFC3339Nano, rightValue)
	if leftErr == nil && rightErr == nil {
		return leftTime.After(rightTime)
	}
	return leftValue > rightValue
}

func derefTimestamp(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func episodeTitleByIndex(episodes []library.TocEpisodeSummary, episodeIndex string) *string {
	for _, episode := range episodes {
		if episode.EpisodeIndex == episodeIndex {
			title := episode.Title
			return &title
		}
	}
	return nil
}
