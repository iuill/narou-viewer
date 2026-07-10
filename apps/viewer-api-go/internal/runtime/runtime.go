package runtime

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"

	appextraction "narou-viewer/apps/viewer-api-go/internal/application/extraction"
	"narou-viewer/apps/viewer-api-go/internal/application/extractionjobs"
	"narou-viewer/apps/viewer-api-go/internal/application/extractionruntime"
	"narou-viewer/apps/viewer-api-go/internal/application/fetchercommands"
	"narou-viewer/apps/viewer-api-go/internal/application/libraryview"
	"narou-viewer/apps/viewer-api-go/internal/application/readerassistant"
	"narou-viewer/apps/viewer-api-go/internal/application/readertextcache"
	"narou-viewer/apps/viewer-api-go/internal/application/readerview"
	"narou-viewer/apps/viewer-api-go/internal/application/removedstate"
	"narou-viewer/apps/viewer-api-go/internal/characters"
	"narou-viewer/apps/viewer-api-go/internal/config"
	extractdomain "narou-viewer/apps/viewer-api-go/internal/extraction"
	"narou-viewer/apps/viewer-api-go/internal/fetcher"
	"narou-viewer/apps/viewer-api-go/internal/httpapi"
	"narou-viewer/apps/viewer-api-go/internal/library"
	"narou-viewer/apps/viewer-api-go/internal/publications"
	"narou-viewer/apps/viewer-api-go/internal/store"
	"narou-viewer/apps/viewer-api-go/internal/terms"
)

type HandlerResult struct {
	Handler         http.Handler
	InitErr         error
	startBackground func(context.Context)
}

func (r HandlerResult) StartBackground(ctx context.Context) error {
	if r.InitErr != nil {
		return r.InitErr
	}
	if r.startBackground != nil {
		r.startBackground(ctx)
	}
	return nil
}

func NewHandler(dataDir string) HandlerResult {
	stateDir := filepath.Join(dataDir, "state")
	usageDBPath := filepath.Join(stateDir, "ai_usage.sqlite")
	stateStore := store.New(dataDir)
	initErr := stateStore.Initialize()
	publicationService := publications.NewService(stateDir)
	publicationInitErr := publicationService.Ensure()
	characterInitErr := characters.EnsureStateDirs(stateDir)
	extractionInitErr := extractdomain.EnsureStateDirs(stateDir)
	termInitErr := terms.EnsureStateDirs(stateDir)
	fetcherClient := fetcher.NewClient(config.FetcherAPIBaseURL())
	libraryService := library.NewServiceWithFetcher(filepath.Join(dataDir, "novel-fetcher"), fetcherClient)
	textCache := readertextcache.New(stateDir)
	fetcherCommands := fetchercommands.NewService(
		fetcherClient,
		fetchercommands.NewLibraryWorkIDResolver(libraryService),
	).WithRemovedNovelStateCleaner(removedstate.NewServiceWithReaderSearchCache(stateStore, stateDir, usageDBPath, textCache))
	libraryViewService := libraryview.NewService(libraryService, stateStore, publicationService)
	readerViewService := readerview.NewServiceWithTextCache(libraryService, stateStore, textCache)
	readerAssistantService := readerassistant.NewService(readerassistant.Dependencies{
		Library:     libraryService,
		Settings:    stateStore,
		StateDir:    stateDir,
		UsageDBPath: usageDBPath,
		TextCache:   textCache,
	})
	extractionRuntime := extractionruntime.NewRuntime(extractionruntime.RuntimeDependencies{
		StateDir:    stateDir,
		UsageDBPath: usageDBPath,
		Library:     libraryService,
		Settings:    stateStore,
		Logger:      httpapi.LogExtractionTiming,
	})
	extractionJobsService := extractionjobs.NewService(stateDir, libraryService, stateStore)
	characterJobCoordinator := appextraction.NewJobCoordinator(stateDir, extractionRuntime.ProcessJob)
	joinedInitErr := errors.Join(initErr, publicationInitErr, characterInitErr, extractionInitErr, termInitErr)
	handler := httpapi.NewServerWithDependencies(httpapi.ServerDependencies{
		DataDir:                  dataDir,
		Library:                  libraryService,
		Publications:             publicationService,
		StateStore:               stateStore,
		FetcherClient:            fetcherClient,
		FetcherCommand:           fetcherCommands,
		LibraryView:              libraryViewService,
		ReaderAssistant:          readerAssistantService,
		ReaderView:               readerViewService,
		Extraction:               extractionRuntime,
		CharacterJobs:            extractionJobsService,
		ExtractionJobCoordinator: characterJobCoordinator,
		StateInitErr:             joinedInitErr,
	})

	result := HandlerResult{
		Handler: handler,
		InitErr: joinedInitErr,
	}
	if background, ok := handler.(interface{ StartBackground(context.Context) }); ok {
		result.startBackground = background.StartBackground
	}
	return result
}
