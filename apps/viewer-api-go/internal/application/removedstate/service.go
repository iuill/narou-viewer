package removedstate

import (
	"context"

	"narou-viewer/apps/viewer-api-go/internal/ai"
	"narou-viewer/apps/viewer-api-go/internal/application/readertextcache"
	extractdomain "narou-viewer/apps/viewer-api-go/internal/extraction"
	"narou-viewer/apps/viewer-api-go/internal/publications"
	"narou-viewer/apps/viewer-api-go/internal/store"
)

type CleanupResult struct {
	ReadingStatesDeleted         int `json:"readingStatesDeleted"`
	BookmarksDeleted             int `json:"bookmarksDeleted"`
	CharacterEventsDeleted       int `json:"characterEventsDeleted"`
	CharacterProfilesDeleted     int `json:"characterProfilesDeleted"`
	CharacterJobsDeleted         int `json:"characterJobsDeleted"`
	CharacterJobIndexesDeleted   int `json:"characterJobIndexesDeleted"`
	CharacterCheckpointsDeleted  int `json:"characterCheckpointsDeleted"`
	PublicationEntriesDeleted    int `json:"publicationEntriesDeleted"`
	AIUsageRunsDeleted           int `json:"aiUsageRunsDeleted"`
	ReaderSearchCacheRowsDeleted int `json:"readerSearchCacheRowsDeleted"`
}

type Service struct {
	stateStore        *store.Store
	stateDir          string
	aiUsageDBPath     string
	readerSearchCache *readertextcache.Store
}

func NewService(stateStore *store.Store, stateDir string, aiUsageDBPath string) *Service {
	return NewServiceWithReaderSearchCache(stateStore, stateDir, aiUsageDBPath, readertextcache.New(stateDir))
}

func NewServiceWithReaderSearchCache(stateStore *store.Store, stateDir string, aiUsageDBPath string, readerSearchCache *readertextcache.Store) *Service {
	return &Service{
		stateStore:        stateStore,
		stateDir:          stateDir,
		aiUsageDBPath:     aiUsageDBPath,
		readerSearchCache: readerSearchCache,
	}
}

func (s *Service) PruneRemovedNovelState(novelIDs []string) (CleanupResult, error) {
	cleanup := CleanupResult{}
	if s == nil {
		return cleanup, nil
	}

	for _, novelID := range novelIDs {
		if s.stateStore != nil {
			result, err := s.stateStore.PruneNovelState(novelID)
			if err != nil {
				return cleanup, err
			}
			if result.ReadingStateDeleted {
				cleanup.ReadingStatesDeleted += 1
			}
			cleanup.BookmarksDeleted += result.BookmarksDeleted
		}

		characterResult, err := extractdomain.PruneNovelState(s.stateDir, novelID)
		if err != nil {
			return cleanup, err
		}
		if characterResult.ProfileDeleted {
			cleanup.CharacterProfilesDeleted += 1
		}
		if characterResult.EventsDeleted {
			cleanup.CharacterEventsDeleted += 1
		}
		if characterResult.JobIndexDeleted {
			cleanup.CharacterJobIndexesDeleted += 1
		}
		cleanup.CharacterJobsDeleted += characterResult.JobsDeleted
		cleanup.CharacterCheckpointsDeleted += characterResult.CheckpointsDeleted

		publicationsDeleted, err := publications.NewRepository(s.stateDir).PruneNovel(novelID)
		if err != nil {
			return cleanup, err
		}
		cleanup.PublicationEntriesDeleted += publicationsDeleted

		usageDeleted, err := ai.PruneUsageByNovelID(s.aiUsageDBPath, novelID)
		if err != nil {
			return cleanup, err
		}
		cleanup.AIUsageRunsDeleted += usageDeleted

		readerSearchRowsDeleted, err := s.readerSearchCache.PruneByNovelID(context.Background(), novelID)
		if err != nil {
			return cleanup, err
		}
		cleanup.ReaderSearchCacheRowsDeleted += readerSearchRowsDeleted
	}
	return cleanup, nil
}
