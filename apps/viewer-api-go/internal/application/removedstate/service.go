package removedstate

import (
	"context"
	"strings"
	"sync"

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
	TermProfilesDeleted          int `json:"termProfilesDeleted"`
	ExtractionJobsDeleted        int `json:"extractionJobsDeleted"`
	ExtractionJobIndexesDeleted  int `json:"extractionJobIndexesDeleted"`
	ExtractionCheckpointsDeleted int `json:"extractionCheckpointsDeleted"`
	PublicationEntriesDeleted    int `json:"publicationEntriesDeleted"`
	AIUsageRunsDeleted           int `json:"aiUsageRunsDeleted"`
	ReaderSearchCacheRowsDeleted int `json:"readerSearchCacheRowsDeleted"`
}

type Service struct {
	stateStore        *store.Store
	stateDir          string
	aiUsageDBPath     string
	readerSearchCache *readertextcache.Store
	mu                sync.Mutex
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
	s.mu.Lock()
	defer s.mu.Unlock()

	novelIDs = normalizeNovelIDs(novelIDs)
	if err := s.preflightPruneRemovedNovelState(novelIDs); err != nil {
		return cleanup, err
	}

	for _, novelID := range novelIDs {
		readerSearchRowsDeleted, err := s.readerSearchCache.PruneByNovelID(context.Background(), novelID)
		if err != nil {
			return cleanup, err
		}
		cleanup.ReaderSearchCacheRowsDeleted += readerSearchRowsDeleted
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
		if characterResult.TermProfileDeleted {
			cleanup.TermProfilesDeleted += 1
		}
		if characterResult.JobIndexDeleted {
			cleanup.ExtractionJobIndexesDeleted += 1
		}
		cleanup.ExtractionJobsDeleted += characterResult.JobsDeleted
		cleanup.ExtractionCheckpointsDeleted += characterResult.CheckpointsDeleted

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

	}
	return cleanup, nil
}

func (s *Service) preflightPruneRemovedNovelState(novelIDs []string) error {
	if err := ai.PreflightUsagePrune(s.aiUsageDBPath); err != nil {
		return err
	}
	publicationRepository := publications.NewRepository(s.stateDir)
	for _, novelID := range novelIDs {
		if s.stateStore != nil {
			if err := s.stateStore.PreflightPruneNovelState(novelID); err != nil {
				return err
			}
		}
		if err := extractdomain.PreflightPruneNovelState(s.stateDir, novelID); err != nil {
			return err
		}
		if err := publicationRepository.PreflightPruneNovel(novelID); err != nil {
			return err
		}
	}
	return nil
}

func normalizeNovelIDs(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		novelID := strings.TrimSpace(value)
		if novelID == "" || seen[novelID] {
			continue
		}
		seen[novelID] = true
		result = append(result, novelID)
	}
	return result
}
