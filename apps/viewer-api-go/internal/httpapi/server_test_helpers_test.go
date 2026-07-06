package httpapi

import (
	"errors"
	"net/http"
	"path/filepath"

	"narou-viewer/apps/viewer-api-go/internal/application/fetchercommands"
	"narou-viewer/apps/viewer-api-go/internal/application/readertextcache"
	"narou-viewer/apps/viewer-api-go/internal/application/removedstate"
	"narou-viewer/apps/viewer-api-go/internal/characters"
	"narou-viewer/apps/viewer-api-go/internal/config"
	"narou-viewer/apps/viewer-api-go/internal/fetcher"
	"narou-viewer/apps/viewer-api-go/internal/library"
	"narou-viewer/apps/viewer-api-go/internal/publications"
	"narou-viewer/apps/viewer-api-go/internal/store"
)

func newTestServerWithDataDir(dataDir string) (http.Handler, error) {
	stateStore := store.New(dataDir)
	initErr := stateStore.Initialize()
	publicationService := publications.NewService(filepath.Join(dataDir, "state"))
	publicationInitErr := publicationService.Ensure()
	characterInitErr := characters.EnsureStateDirs(filepath.Join(dataDir, "state"))
	libraryService := library.NewService(filepath.Join(dataDir, "novel-fetcher"))
	joinedErr := errors.Join(initErr, publicationInitErr, characterInitErr)
	handler := newTestServerWithDependencies(dataDir, libraryService, publicationService, stateStore, joinedErr)
	return handler, joinedErr
}

func newTestServerWithStore(stateStore *store.Store) http.Handler {
	dataDir := filepath.Clean("../../data")
	return newTestServerWithLibraryAndStore(dataDir, library.NewService(filepath.Join(dataDir, "novel-fetcher")), stateStore)
}

func newTestServerWithLibraryAndStore(dataDir string, libraryService *library.Service, stateStore *store.Store) http.Handler {
	return newTestServerWithDependencies(dataDir, libraryService, nil, stateStore, nil)
}

func newTestServerWithDependencies(dataDir string, libraryService *library.Service, publicationService *publications.Service, stateStore *store.Store, initErr error) http.Handler {
	fetcherClient := fetcher.NewClient(config.FetcherAPIBaseURL())
	stateDir := filepath.Join(dataDir, "state")
	textCache := readertextcache.New(stateDir)
	fetcherCommands := fetchercommands.NewService(
		fetcherClient,
		fetchercommands.NewLibraryWorkIDResolver(libraryService),
	).WithRemovedNovelStateCleaner(removedstate.NewServiceWithReaderSearchCache(stateStore, stateDir, filepath.Join(stateDir, "ai_usage.sqlite"), textCache))
	return NewServerWithDependencies(ServerDependencies{
		DataDir:        dataDir,
		Library:        libraryService,
		Publications:   publicationService,
		StateStore:     stateStore,
		FetcherClient:  fetcherClient,
		FetcherCommand: fetcherCommands,
		StateInitErr:   initErr,
	})
}
