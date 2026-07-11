package httpapi

import "net/http"

func (s *Server) routes() {
	s.registerSystemRoutes()
	s.registerAIGenerationRoutes()
	s.registerReaderRoutes()
	s.registerFetcherRoutes()
	s.registerLibraryRoutes()
	s.registerAPIFallbackRoutes()
}

func (s *Server) registerSystemRoutes() {
	s.mux.HandleFunc("/api/health", s.handleHealth)
	s.mux.HandleFunc("/api/system/status", s.handleSystemStatus)
	s.mux.HandleFunc("/api/system/storage/progress", s.handleStorageUsageProgress)
	s.mux.HandleFunc("/api/system/storage", s.handleStorageUsage)
}

func (s *Server) registerAIGenerationRoutes() {
	s.mux.HandleFunc("/api/ai-generation/settings/preferred-mode", s.handlePreferredMode)
	s.mux.HandleFunc("/api/ai-generation/settings", s.handleAISettings)
	s.mux.HandleFunc("/api/ai-generation/jobs", s.handleAIJobs)
	s.mux.HandleFunc("/api/ai-generation/usage/", s.handleUsageDetail)
	s.mux.HandleFunc("/api/ai-generation/usage", s.handleUsage)
	s.mux.HandleFunc("/api/ai-generation/playground/extraction/stream", s.handlePlaygroundStream)
	s.mux.HandleFunc("/api/ai-generation/playground/extraction", s.handlePlayground)
}

func (s *Server) registerReaderRoutes() {
	s.mux.HandleFunc("/api/reader/state", s.handleReaderState)
	s.mux.HandleFunc("/api/reader/preferences", s.handleReaderPreferences)
	s.mux.HandleFunc("/api/bookmarks/", s.handleBookmarkByID)
	s.mux.HandleFunc("/api/bookmarks", s.handleBookmarks)
}

func (s *Server) registerFetcherRoutes() {
	s.mux.HandleFunc("/api/fetcher/status", s.handleFetcherStatus)
	s.mux.HandleFunc("/api/fetcher/queue", s.handleFetcherQueue)
	s.mux.HandleFunc("/api/fetcher/tasks/summary", s.handleFetcherTaskSummary)
	s.mux.HandleFunc("/api/fetcher/works/download", s.handleFetcherDownload)
	s.mux.HandleFunc("/api/fetcher/works/update", s.handleFetcherNovelIDsAction)
	s.mux.HandleFunc("/api/fetcher/works/resume", s.handleFetcherNovelIDsAction)
	s.mux.HandleFunc("/api/fetcher/works/remove", s.handleFetcherNovelIDsAction)
	s.mux.HandleFunc("/api/fetcher/tasks/", s.handleFetcherTaskAction)
}

func (s *Server) registerLibraryRoutes() {
	s.mux.HandleFunc("/api/library/novels/", s.handleNovelSubroute)
	s.mux.HandleFunc("/api/library/novels", s.handleNovels)
}

func (s *Server) registerAPIFallbackRoutes() {
	s.mux.HandleFunc("/api/", s.handleAPINotFound)
	s.mux.HandleFunc("/api", s.handleAPINotFound)
}

func (s *Server) handleAPINotFound(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotFound, "Not found.")
}
