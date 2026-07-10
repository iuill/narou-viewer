package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"narou-viewer/apps/viewer-api-go/internal/ai"
	"narou-viewer/apps/viewer-api-go/internal/application/extractionjobs"
	"narou-viewer/apps/viewer-api-go/internal/application/readerassistant"
	"narou-viewer/apps/viewer-api-go/internal/application/readerview"
	"narou-viewer/apps/viewer-api-go/internal/characters"
	"narou-viewer/apps/viewer-api-go/internal/extraction"
	"narou-viewer/apps/viewer-api-go/internal/library"
	"narou-viewer/apps/viewer-api-go/internal/publications"
	"narou-viewer/apps/viewer-api-go/internal/store"
	"narou-viewer/apps/viewer-api-go/internal/terms"
)

func (s *Server) handleNovelSubroute(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/library/novels/")
	parts := strings.Split(rest, "/")
	if len(parts) == 0 || parts[0] == "" {
		writeError(w, http.StatusNotFound, "Novel not found.")
		return
	}
	novelID := trimPathValue(parts[0])
	suffix := strings.Join(parts[1:], "/")

	switch suffix {
	case "toc":
		s.handleToc(w, r, novelID)
	case "publications":
		s.handlePublications(w, r, novelID)
	case "publications/entries":
		s.handlePublicationEntries(w, r, novelID)
	case "publications/display-cover":
		s.handlePublicationDisplayCover(w, r, novelID)
	default:
		if strings.HasPrefix(suffix, "publications/entries/") {
			s.handlePublicationEntry(w, r, novelID, trimPathValue(strings.TrimPrefix(suffix, "publications/entries/")))
			return
		}
		if strings.HasPrefix(suffix, "episodes/") {
			s.handleEpisode(w, r, novelID, trimPathValue(strings.TrimPrefix(suffix, "episodes/")))
			return
		}
		if strings.HasPrefix(suffix, "assets/") {
			s.handleAsset(w, r, novelID, strings.TrimPrefix(suffix, "assets/"))
			return
		}
		s.handleReaderSubroute(w, r, novelID, suffix)
	}
}

func (s *Server) handleReaderSubroute(w http.ResponseWriter, r *http.Request, novelID string, suffix string) {
	switch suffix {
	case "characters":
		s.handleCharacters(w, r, novelID)
	case "terms":
		s.handleTerms(w, r, novelID)
	case "extraction":
		s.handleExtractionClear(w, r, novelID)
	case "extraction-jobs":
		s.handleExtractionJobs(w, r, novelID)
	case "reader-settings":
		s.handleNovelReaderSettings(w, r, novelID)
	case "reader-assistant/chat":
		s.handleReaderAssistantChat(w, r, novelID, false)
	case "reader-assistant/chat/stream":
		s.handleReaderAssistantChat(w, r, novelID, true)
	default:
		writeError(w, http.StatusNotFound, "Not found.")
	}
}

func (s *Server) handleNovels(w http.ResponseWriter, r *http.Request) {
	if !methodOnly(w, r, http.MethodGet) {
		return
	}
	if s.libraryView == nil {
		writeJSON(w, http.StatusOK, map[string]any{"novels": []any{}})
		return
	}
	result, err := s.libraryView.ListNovels(r.Context())
	writeResult(w, result, err)
}

func (s *Server) handleToc(w http.ResponseWriter, r *http.Request, novelID string) {
	if !methodOnly(w, r, http.MethodGet) {
		return
	}
	if s.libraryView == nil {
		writeError(w, http.StatusNotFound, "Novel not found.")
		return
	}
	toc, err := s.libraryView.GetToc(r.Context(), novelID)
	if err != nil {
		writeResult(w, nil, err)
		return
	}
	if toc == nil {
		writeError(w, http.StatusNotFound, "Novel not found.")
		return
	}
	writeJSON(w, http.StatusOK, toc)
}

func (s *Server) handleEpisode(w http.ResponseWriter, r *http.Request, novelID string, episodeIndex string) {
	if !methodOnly(w, r, http.MethodGet) {
		return
	}
	if s.readerView == nil {
		writeError(w, http.StatusNotFound, "Episode not found.")
		return
	}
	if !isDigits(episodeIndex) {
		writeError(w, http.StatusBadRequest, "episodeIndex must be a non-negative integer string.")
		return
	}
	episodeView, err := s.readerView.GetEpisode(r.Context(), novelID, episodeIndex)
	if err != nil {
		writeResult(w, nil, err)
		return
	}
	episode := episodeView.Episode
	if episode == nil {
		writeError(w, http.StatusNotFound, "Episode not found.")
		return
	}
	responseEtag := episodeView.ETag
	w.Header().Set("ETag", `"`+responseEtag+`"`)
	if matchesContentEtag(r.Header.Values("If-None-Match"), responseEtag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	writeJSON(w, http.StatusOK, episode)
}

func (s *Server) handleNovelReaderSettings(w http.ResponseWriter, r *http.Request, novelID string) {
	if s.readerView == nil {
		writeError(w, http.StatusNotFound, "Novel not found.")
		return
	}
	switch r.Method {
	case http.MethodGet:
		settings, err := s.readerView.GetSettings(novelID)
		writeResult(w, settings, err)
	case http.MethodPut:
		body, ok := decodeObjectOrBadRequest(w, r)
		if !ok {
			return
		}
		correctionValue, ok := body["correction"].(map[string]any)
		if !ok {
			writeError(w, http.StatusBadRequest, "correction is required.")
			return
		}
		var patch store.NovelReaderCorrectionPatch
		if rawQuoteNormalization, exists := correctionValue["quoteNormalization"]; exists {
			quoteNormalization, ok := rawQuoteNormalization.(bool)
			if !ok {
				writeError(w, http.StatusBadRequest, "correction.quoteNormalization must be a boolean.")
				return
			}
			patch.QuoteNormalization = &quoteNormalization
		}
		if rawHyphenDashNormalization, exists := correctionValue["hyphenDashNormalization"]; exists {
			hyphenDashNormalization, ok := rawHyphenDashNormalization.(bool)
			if !ok {
				writeError(w, http.StatusBadRequest, "correction.hyphenDashNormalization must be a boolean.")
				return
			}
			patch.HyphenDashNormalization = &hyphenDashNormalization
		}
		if rawParenthesisNormalization, exists := correctionValue["parenthesisNormalization"]; exists {
			parenthesisNormalization, ok := rawParenthesisNormalization.(bool)
			if !ok {
				writeError(w, http.StatusBadRequest, "correction.parenthesisNormalization must be a boolean.")
				return
			}
			patch.ParenthesisNormalization = &parenthesisNormalization
		}
		if rawHalfwidthAlnumPunctuationNormalization, exists := correctionValue["halfwidthAlnumPunctuationNormalization"]; exists {
			halfwidthAlnumPunctuationNormalization, ok := rawHalfwidthAlnumPunctuationNormalization.(bool)
			if !ok {
				writeError(w, http.StatusBadRequest, "correction.halfwidthAlnumPunctuationNormalization must be a boolean.")
				return
			}
			patch.HalfwidthAlnumPunctuationNormalization = &halfwidthAlnumPunctuationNormalization
		}
		if patch.IsEmpty() {
			writeError(w, http.StatusBadRequest, "At least one correction field is required.")
			return
		}
		settings, err := s.readerView.PatchSettings(novelID, patch)
		if errors.Is(err, readerview.ErrNovelNotFound) {
			writeError(w, http.StatusNotFound, "Novel not found.")
			return
		}
		if errors.Is(err, store.ErrNovelStateDeleted) {
			writeError(w, http.StatusGone, "Novel has been deleted.")
			return
		}
		writeResult(w, settings, err)
	default:
		methodOnly(w, r, http.MethodGet, http.MethodPut)
	}
}

func (s *Server) handleAsset(w http.ResponseWriter, r *http.Request, novelID string, assetPath string) {
	if !methodOnly(w, r, http.MethodGet) {
		return
	}
	if s.library == nil {
		writeError(w, http.StatusNotFound, "Asset not found.")
		return
	}
	assetPath, _ = url.PathUnescape(assetPath)
	asset, err := s.library.GetAsset(r.Context(), novelID, assetPath)
	if err != nil {
		writeResult(w, nil, err)
		return
	}
	if asset == nil {
		writeError(w, http.StatusNotFound, "Asset not found.")
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("Content-Type", asset.MediaType)
	http.ServeFile(w, r, asset.FilePath)
}

func (s *Server) handleCharacters(w http.ResponseWriter, r *http.Request, novelID string) {
	if !methodOnly(w, r, http.MethodGet) {
		return
	}
	s.handleExtraction(w, r, novelID)
}

func (s *Server) handleTerms(w http.ResponseWriter, r *http.Request, novelID string) {
	if !methodOnly(w, r, http.MethodGet) {
		return
	}
	upToEpisodeIndex := r.URL.Query().Get("upToEpisodeIndex")
	if !isDigits(upToEpisodeIndex) {
		writeError(w, http.StatusBadRequest, "upToEpisodeIndex is required and must be a non-negative integer string.")
		return
	}
	novelFound, episodeFound, err := s.validateNovelEpisode(r.Context(), novelID, upToEpisodeIndex)
	if err != nil {
		writeLibraryLookupError(w, r, novelID, upToEpisodeIndex, err)
		return
	}
	if !novelFound {
		writeError(w, http.StatusNotFound, "Novel not found.")
		return
	}
	if !episodeFound {
		writeError(w, http.StatusBadRequest, "upToEpisodeIndex is out of range.")
		return
	}

	committedFrontier := ""
	characterSummary, _, err := characters.LoadSummaryForEpisodes(s.stateDir(), novelID, upToEpisodeIndex, s.episodeIndexes(r.Context(), novelID))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Character profiles could not be read.")
		return
	}
	if characterSummary.ProcessedUpToEpisodeIndex != nil {
		committedFrontier = *characterSummary.ProcessedUpToEpisodeIndex
	}
	response, err := terms.BuildResponse(s.stateDir(), novelID, upToEpisodeIndex, committedFrontier)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Term profiles could not be read.")
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleExtraction(w http.ResponseWriter, r *http.Request, novelID string) {
	upToEpisodeIndex := r.URL.Query().Get("upToEpisodeIndex")
	if !isDigits(upToEpisodeIndex) {
		writeError(w, http.StatusBadRequest, "upToEpisodeIndex is required and must be a non-negative integer string.")
		return
	}
	novelFound, episodeFound, err := s.validateNovelEpisode(r.Context(), novelID, upToEpisodeIndex)
	if err != nil {
		writeLibraryLookupError(w, r, novelID, upToEpisodeIndex, err)
		return
	}
	if !novelFound {
		writeError(w, http.StatusNotFound, "Novel not found.")
		return
	}
	if !episodeFound {
		writeError(w, http.StatusBadRequest, "upToEpisodeIndex is out of range.")
		return
	}
	if summary, ok, err := characters.LoadSummaryForEpisodes(s.stateDir(), novelID, upToEpisodeIndex, s.episodeIndexes(r.Context(), novelID)); err != nil {
		writeError(w, http.StatusInternalServerError, "Character profiles could not be read.")
		return
	} else if ok {
		writeJSON(w, http.StatusOK, summary)
		return
	}
	writeJSON(w, http.StatusOK, characters.SummaryResponse{
		Status:                    "not_generated",
		NovelID:                   novelID,
		UpToEpisodeIndex:          upToEpisodeIndex,
		ProcessedUpToEpisodeIndex: nil,
		Characters:                []characters.Character{},
	})
}

func (s *Server) handleExtractionClear(w http.ResponseWriter, r *http.Request, novelID string) {
	if !methodOnly(w, r, http.MethodDelete) {
		return
	}
	if s.characterJobQueue == nil {
		writeError(w, http.StatusNotFound, "Novel not found.")
		return
	}
	result, err := s.characterJobQueue.Clear(r.Context(), novelID)
	if errors.Is(err, extractionjobs.ErrNovelNotFound) {
		writeError(w, http.StatusNotFound, "Novel not found.")
		return
	}
	if errors.Is(err, extractionjobs.ErrExtractionClear) {
		writeError(w, http.StatusInternalServerError, "Extraction state could not be cleared.")
		return
	}
	if errors.Is(err, extractionjobs.ErrExtractionActive) {
		writeError(w, http.StatusConflict, "Extraction is still running.")
		return
	}
	writeResult(w, result, err)
}

func (s *Server) handleExtractionJobs(w http.ResponseWriter, r *http.Request, novelID string) {
	if s.characterJobQueue == nil {
		writeError(w, http.StatusNotFound, "Novel not found.")
		return
	}
	switch r.Method {
	case http.MethodGet:
		result, err := s.characterJobQueue.List(r.Context(), novelID)
		s.writeCharacterJobsResult(w, result, err)
	case http.MethodPost:
		body, ok := decodeObjectOrBadRequest(w, r)
		if !ok {
			return
		}
		upToEpisodeIndex, ok := isNonNegativeIntegerString(body["upToEpisodeIndex"])
		if !ok {
			writeError(w, http.StatusBadRequest, "upToEpisodeIndex is required and must be a non-negative integer string.")
			return
		}
		var generationStrategy *string
		if rawStrategy, exists := body["generationStrategy"]; exists {
			strategy, ok := rawStrategy.(string)
			if !ok {
				writeError(w, http.StatusBadRequest, "generationStrategy is invalid.")
				return
			}
			generationStrategy = &strategy
		}
		result, created, err := s.characterJobQueue.Enqueue(r.Context(), novelID, extractionjobs.EnqueueInput{
			UpToEpisodeIndex:   upToEpisodeIndex,
			GenerationStrategy: generationStrategy,
		})
		if !s.writeCharacterJobEnqueueResult(w, result, created, err) {
			return
		}
		if created {
			s.extractionJobs.Kick(s.ctx)
		}
	default:
		methodOnly(w, r, http.MethodGet, http.MethodPost)
	}
}

func (s *Server) writeCharacterJobsResult(w http.ResponseWriter, result extractionjobs.JobsResponse, err error) {
	if errors.Is(err, extractionjobs.ErrNovelNotFound) {
		writeError(w, http.StatusNotFound, "Novel not found.")
		return
	}
	if errors.Is(err, extractionjobs.ErrJobsRead) {
		writeError(w, http.StatusInternalServerError, "Character jobs could not be read.")
		return
	}
	writeResult(w, result, err)
}

func (s *Server) writeCharacterJobEnqueueResult(w http.ResponseWriter, result extractionjobs.EnqueueResponse, created bool, err error) bool {
	switch {
	case errors.Is(err, extractionjobs.ErrInvalidUpToEpisodeIndex):
		writeError(w, http.StatusBadRequest, "upToEpisodeIndex is required and must be a non-negative integer string.")
		return false
	case errors.Is(err, extractionjobs.ErrNovelNotFound):
		writeError(w, http.StatusNotFound, "Novel not found.")
		return false
	case errors.Is(err, extractionjobs.ErrEpisodeOutOfRange):
		writeError(w, http.StatusBadRequest, "upToEpisodeIndex is out of range.")
		return false
	case errors.Is(err, extractionjobs.ErrInvalidGenerationStrategy):
		writeError(w, http.StatusBadRequest, "generationStrategy is invalid.")
		return false
	case errors.Is(err, extractionjobs.ErrSettingsRead):
		writeError(w, http.StatusInternalServerError, "AI generation settings could not be read.")
		return false
	case errors.Is(err, extractionjobs.ErrJobSave):
		writeError(w, http.StatusInternalServerError, "Character job could not be saved.")
		return false
	case err != nil:
		writeResult(w, nil, err)
		return false
	}
	if created {
		writeJSON(w, http.StatusAccepted, result)
		return true
	}
	writeJSON(w, http.StatusOK, result)
	return true
}

func (s *Server) extractionHeuristicEpisodes(ctx context.Context, novelID string, upToEpisodeIndex string) []characters.HeuristicEpisode {
	if s.library == nil {
		return []characters.HeuristicEpisode{}
	}
	toc, err := s.library.GetToc(ctx, novelID)
	if err != nil || toc == nil {
		return []characters.HeuristicEpisode{}
	}
	episodes := []characters.HeuristicEpisode{}
	for _, episodeSummary := range toc.Episodes {
		if compareEpisodeString(episodeSummary.EpisodeIndex, upToEpisodeIndex) > 0 {
			break
		}
		episode, err := s.library.GetEpisode(ctx, novelID, episodeSummary.EpisodeIndex)
		if err != nil || episode == nil {
			continue
		}
		text := extraction.ExtractEpisodeText(extraction.EpisodeInput{
			EpisodeIndex:   episode.EpisodeIndex,
			Title:          episode.Title,
			Chapter:        episode.Chapter,
			Subchapter:     episode.Subchapter,
			HTML:           episode.HTML,
			ReaderDocument: episode.ReaderDocument,
		})
		episodes = append(episodes, characters.HeuristicEpisode{
			EpisodeIndex: episode.EpisodeIndex,
			Text:         text,
		})
	}
	return episodes
}

func (s *Server) handleReaderAssistantChat(w http.ResponseWriter, r *http.Request, novelID string, stream bool) {
	if !methodOnly(w, r, http.MethodPost) {
		return
	}
	body, ok := decodeObjectOrBadRequest(w, r)
	if !ok {
		return
	}
	message, ok := body["message"].(string)
	if !ok || strings.TrimSpace(message) == "" {
		writeError(w, http.StatusBadRequest, "message is required.")
		return
	}
	message = strings.TrimSpace(message)
	if len([]rune(message)) > 1000 {
		writeError(w, http.StatusBadRequest, "message must be at most 1000 characters.")
		return
	}
	if _, ok := isNonNegativeIntegerString(body["currentEpisodeIndex"]); !ok {
		writeError(w, http.StatusBadRequest, "currentEpisodeIndex must be a non-negative integer string.")
		return
	}
	currentEpisodeIndex, _ := isNonNegativeIntegerString(body["currentEpisodeIndex"])
	readerPosition := 0
	if position, exists := body["position"]; exists && position != nil {
		number, ok := position.(float64)
		if !ok || number < 0 || number != float64(int64(number)) {
			writeError(w, http.StatusBadRequest, "position must be a non-negative integer or null.")
			return
		}
		readerPosition = int(number)
	}
	novelFound, episodeFound, err := s.validateNovelEpisode(r.Context(), novelID, currentEpisodeIndex)
	if err != nil {
		writeLibraryLookupError(w, r, novelID, currentEpisodeIndex, err)
		return
	}
	if !novelFound {
		writeError(w, http.StatusNotFound, "Novel not found.")
		return
	}
	if !episodeFound {
		writeError(w, http.StatusBadRequest, "currentEpisodeIndex is out of range.")
		return
	}
	history := readerassistant.NormalizeHistory(body["history"])
	if !stream {
		ctx, cancel := nonStreamingLLMContext(r.Context())
		defer cancel()
		response, err := s.readerAssistant.Respond(ctx, readerassistant.Request{
			NovelID:             novelID,
			CurrentEpisodeIndex: currentEpisodeIndex,
			ReaderPosition:      readerPosition,
			Message:             message,
			History:             history,
		}, nil)
		if err != nil {
			writeError(w, readerAssistantErrorStatus(err), readerAssistantErrorMessage(err))
			return
		}
		writeJSON(w, http.StatusOK, response)
		return
	}

	w.Header().Set("content-type", "application/x-ndjson; charset=utf-8")
	w.Header().Set("cache-control", "no-cache, no-transform")
	w.Header().Set("x-content-type-options", "nosniff")
	w.Header().Set("x-accel-buffering", "no")
	extendStreamingWriteDeadline(w)
	w.WriteHeader(http.StatusOK)
	encoder := json.NewEncoder(w)
	flusher, _ := w.(http.Flusher)
	writeStreamEvent := func(event map[string]any) bool {
		if r.Context().Err() != nil {
			return false
		}
		extendStreamingWriteDeadline(w)
		if err := encoder.Encode(event); err != nil {
			return false
		}
		if flusher != nil {
			flusher.Flush()
		}
		return r.Context().Err() == nil
	}
	if !writeStreamEvent(map[string]any{"type": "status", "message": "AI機能の設定を確認しています。"}) {
		return
	}
	if !writeStreamEvent(map[string]any{"type": "status", "message": "必要な確認項目を判断しています。"}) {
		return
	}
	response, err := s.readerAssistant.Respond(r.Context(), readerassistant.Request{
		NovelID:             novelID,
		CurrentEpisodeIndex: currentEpisodeIndex,
		ReaderPosition:      readerPosition,
		Message:             message,
		History:             history,
	}, readerassistant.StreamSink(writeStreamEvent))
	if err != nil {
		if r.Context().Err() != nil {
			return
		}
		_ = writeStreamEvent(map[string]any{"type": "error", "error": readerAssistantErrorMessage(err)})
		return
	}
	_ = writeStreamEvent(map[string]any{"type": "result", "response": response})
}

func readerAssistantErrorStatus(err error) int {
	if store.IsAIGenerationSettingsCryptoError(err) {
		return http.StatusServiceUnavailable
	}
	if errors.Is(err, readerassistant.ErrUnavailable) {
		return http.StatusServiceUnavailable
	}
	return http.StatusBadGateway
}

func readerAssistantErrorMessage(err error) string {
	if store.IsAIGenerationSettingsCryptoError(err) {
		return "AI generation settings could not be decrypted."
	}
	return err.Error()
}

func resolveExtractionBatchBudget(ctx context.Context, config *store.ResolvedAIGenerationConfig, fallbackMaxBatchChars int) extractionBatchBudget {
	fallbackTokens := extraction.TokensFromChars(fallbackMaxBatchChars)
	if configuredTokens := extraction.PositiveEnvIntWithFallback("EXTRACTION_MAX_BATCH_TOKENS", "CHARACTER_SUMMARY_MAX_BATCH_TOKENS", 0); configuredTokens > 0 {
		return extractionBatchBudget{MaxTextTokens: configuredTokens}
	}
	budget := extractionBatchBudget{MaxTextChars: fallbackMaxBatchChars, MaxTextTokens: fallbackTokens}
	if config == nil {
		return budget
	}
	lookupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	info, ok := ai.LookupOpenRouterModelInfo(lookupCtx, config.APIKey, config.ModelID, config.ProviderOrder)
	cancel()
	if !ok || info.ContextLength <= 0 {
		return budget
	}
	return extraction.ResolveBatchBudget(fallbackMaxBatchChars, info.ContextLength, info.MaxCompletionTokens)
}

func (s *Server) validateNovelEpisode(ctx context.Context, novelID string, episodeIndex string) (bool, bool, error) {
	if s.library == nil {
		return false, false, nil
	}
	return s.library.EpisodeExists(ctx, novelID, episodeIndex)
}

func writeLibraryLookupError(w http.ResponseWriter, r *http.Request, novelID string, episodeIndex string, err error) {
	path := ""
	if r != nil && r.URL != nil {
		path = r.URL.Path
	}
	log.Printf("viewer-api-go: library lookup failed path=%q novelId=%q episodeIndex=%q error=%v", path, novelID, episodeIndex, err)
	writeError(w, http.StatusBadGateway, "Library data could not be read.")
}

func (s *Server) episodeIndexes(ctx context.Context, novelID string) []string {
	if s.library == nil {
		return nil
	}
	toc, err := s.library.GetToc(ctx, novelID)
	if err != nil || toc == nil {
		return nil
	}
	return episodeIndexesFromToc(toc.Episodes)
}

func episodeIndexesFromToc(episodes []library.TocEpisodeSummary) []string {
	indexes := make([]string, 0, len(episodes))
	for _, episode := range episodes {
		indexes = append(indexes, episode.EpisodeIndex)
	}
	return indexes
}

func (s *Server) stateDir() string {
	return filepath.Join(s.dataDir, "state")
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if !methodOnly(w, r, http.MethodGet) {
		return
	}
	stateReady := s.stateInitErr == nil
	status := "ok"
	if !stateReady {
		status = "warn"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  status,
		"service": "viewer-api",
		"runtime": map[string]any{
			"viewerDataDirConfigured": true,
			"stateDirReady":           stateReady,
		},
	})
}

func (s *Server) handleSystemStatus(w http.ResponseWriter, r *http.Request) {
	if !methodOnly(w, r, http.MethodGet) {
		return
	}
	viewerService := library.RuntimeStatusService{
		ID:      "viewer-api",
		Label:   "viewer-api",
		Status:  library.RuntimeStatusOK,
		Summary: "応答中",
		Detail:  "Go viewer-api は起動しており、ステータス API に応答しています。",
	}
	libraryService := library.RuntimeStatusService{
		ID:      "library",
		Label:   "ローカルライブラリ",
		Status:  library.RuntimeStatusWarn,
		Summary: "未接続",
		Detail:  "novel-fetcher のローカルライブラリを読み取れませんでした。",
	}
	if s.library != nil {
		libraryStatus := s.library.RuntimeStatus(r.Context())
		for _, service := range libraryStatus.Services {
			switch service.ID {
			case "viewer-api":
				viewerService.Status = service.Status
				viewerService.Summary = service.Summary
			case "library":
				libraryService = service
			}
		}
	}
	services := []library.RuntimeStatusService{
		viewerService,
		s.fetcherRuntimeStatusService(r.Context()),
		s.aiGenerationRuntimeStatusService(),
		s.googleBooksRuntimeStatusService(),
		libraryService,
	}
	if s.stateInitErr != nil {
		services = append(services, s.stateRuntimeStatusService())
	}
	status := library.RuntimeStatusResponse{
		Status:    aggregateRuntimeStatus(services),
		CheckedAt: ai.NowISO(),
		Services:  services,
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) googleBooksRuntimeStatusService() library.RuntimeStatusService {
	const serviceID = "google-books"
	const serviceLabel = "Google Books"
	if !publications.GoogleBooksEnabled() {
		return library.RuntimeStatusService{
			ID:      serviceID,
			Label:   serviceLabel,
			Status:  library.RuntimeStatusOK,
			Summary: "無効",
			Detail:  "Google Books による表紙画像と補助書誌の取得は無効化されています。",
		}
	}
	apiKey := ""
	var settingsErr error
	if s != nil && s.stateStore != nil {
		apiKey, settingsErr = s.stateStore.ResolveGoogleBooksAPIKey()
	} else if publications.GoogleBooksAPIKeyConfigured() {
		apiKey = "configured"
	}
	if settingsErr != nil {
		return library.RuntimeStatusService{
			ID:      serviceID,
			Label:   serviceLabel,
			Status:  library.RuntimeStatusError,
			Summary: "設定エラー",
			Detail:  "Google Books API key の読み込みに失敗しました。WebUI の保存値または AI_GENERATION_SETTINGS_MASTER_PASSPHRASE を確認してください。",
		}
	}
	if strings.TrimSpace(apiKey) == "" {
		return library.RuntimeStatusService{
			ID:      serviceID,
			Label:   serviceLabel,
			Status:  library.RuntimeStatusWarn,
			Summary: "API key未設定",
			Detail:  "Google Books API key が未設定です。WebUI の AI機能設定または GOOGLE_BOOKS_API_KEY で設定してください。",
		}
	}
	return library.RuntimeStatusService{
		ID:      serviceID,
		Label:   serviceLabel,
		Status:  library.RuntimeStatusOK,
		Summary: "設定済み",
		Detail:  "Google Books API key が設定されています。ISBN 登録時に表紙画像と補助書誌を取得します。",
	}
}

func (s *Server) fetcherRuntimeStatusService(ctx context.Context) library.RuntimeStatusService {
	status, err := s.fetcherClient.Status(ctx)
	if err != nil {
		return library.RuntimeStatusService{
			ID:      "novel-fetcher",
			Label:   "novel-fetcher",
			Status:  library.RuntimeStatusWarn,
			Summary: "未接続",
			Detail:  "取得 sidecar に接続できませんでした。ローカル保存済みデータのみで動作している可能性があります。",
		}
	}
	version := ""
	if status.Version.Current != nil {
		version = *status.Version.Current
	}
	queueTotal := status.Queue.Total
	summary := "接続済み"
	if version != "" {
		summary = version
	}
	return library.RuntimeStatusService{
		ID:      "novel-fetcher",
		Label:   "novel-fetcher",
		Status:  library.RuntimeStatusOK,
		Summary: summary,
		Detail:  "取得 sidecar に接続済みです。queue total: " + strconv.Itoa(queueTotal) + "。",
	}
}

func (s *Server) stateRuntimeStatusService() library.RuntimeStatusService {
	if s.stateInitErr == nil {
		return library.RuntimeStatusService{
			ID:      "state",
			Label:   "state",
			Status:  library.RuntimeStatusOK,
			Summary: "利用可能",
			Detail:  "viewer-api-go の state ディレクトリは初期化済みです。",
		}
	}
	return library.RuntimeStatusService{
		ID:      "state",
		Label:   "state",
		Status:  library.RuntimeStatusWarn,
		Summary: "初期化エラー",
		Detail:  s.stateInitErr.Error(),
	}
}

func (s *Server) aiGenerationRuntimeStatusService() library.RuntimeStatusService {
	const aiGenerationServiceID = "go-internal-ai"
	const aiGenerationServiceLabel = "Go internal AI"

	settings, err := s.stateStore.GetAIGenerationSettings()
	if err != nil {
		return library.RuntimeStatusService{
			ID:      aiGenerationServiceID,
			Label:   aiGenerationServiceLabel,
			Status:  library.RuntimeStatusError,
			Summary: "設定エラー",
			Detail:  err.Error(),
		}
	}
	activeProfile := resolveActiveAIProfile(settings)
	profileLabelValue := "Default"
	modelLabel := "model未設定"
	hasAPIKey := false
	if activeProfile != nil {
		profileLabelValue = activeProfile.Label
		if activeProfile.ModelID != nil {
			modelLabel = *activeProfile.ModelID
		}
		hasAPIKey = activeProfile.Credentials.HasAPIKey
	}
	if settings.PreferredMode == "llm" && !settings.MasterPassphraseConfigured {
		return library.RuntimeStatusService{
			ID:      aiGenerationServiceID,
			Label:   aiGenerationServiceLabel,
			Status:  library.RuntimeStatusError,
			Summary: "要設定",
			Detail:  "現在の生成方法: LLM連携。`AI_GENERATION_SETTINGS_MASTER_PASSPHRASE` が未設定のため、保存済み APIキーを利用できません。",
		}
	}
	if settings.EffectiveGenerationMode == "heuristic" {
		return library.RuntimeStatusService{
			ID:      aiGenerationServiceID,
			Label:   aiGenerationServiceLabel,
			Status:  library.RuntimeStatusOK,
			Summary: "未使用",
			Detail:  "現在の生成方法: ヒューリスティック。Go internal AI module はローカル生成で動作します。",
		}
	}
	if settings.EffectiveGenerationMode == "disabled" {
		return library.RuntimeStatusService{
			ID:      aiGenerationServiceID,
			Label:   aiGenerationServiceLabel,
			Status:  library.RuntimeStatusError,
			Summary: "利用不可",
			Detail:  "現在の生成方法: LLM連携。OpenRouter APIキーと modelId が未設定のため Go internal AI module の LLM 経路は利用できません。",
		}
	}
	serviceStatus := library.RuntimeStatusOK
	if !hasAPIKey || activeProfile == nil || activeProfile.ModelID == nil {
		serviceStatus = library.RuntimeStatusWarn
	}
	apiKeyStatus := "APIキー未設定。"
	if hasAPIKey {
		apiKeyStatus = "APIキー設定済み。"
	}
	return library.RuntimeStatusService{
		ID:      aiGenerationServiceID,
		Label:   aiGenerationServiceLabel,
		Status:  serviceStatus,
		Summary: "利用中",
		Detail:  "現在の生成方法: LLM連携。Go internal AI module が使用するプロファイル: " + profileLabelValue + " / " + modelLabel + "。" + apiKeyStatus,
	}
}

func aggregateRuntimeStatus(services []library.RuntimeStatusService) library.RuntimeServiceStatus {
	status := library.RuntimeStatusOK
	for _, service := range services {
		if service.Status == library.RuntimeStatusError {
			return library.RuntimeStatusError
		}
		if service.Status == library.RuntimeStatusWarn {
			status = library.RuntimeStatusWarn
		}
	}
	return status
}

func resolveActiveAIProfile(settings ai.SettingsResponse) *ai.Profile {
	profiles := settings.Settings.Profiles
	if settings.Settings.SelectedProfileID != nil {
		for i := range profiles {
			if profiles[i].ID == *settings.Settings.SelectedProfileID {
				return &profiles[i]
			}
		}
	}
	if len(profiles) == 0 {
		return nil
	}
	return &profiles[0]
}

func profileID(profile *ai.Profile) *string {
	if profile == nil {
		return nil
	}
	return &profile.ID
}

func profileLabel(profile *ai.Profile) *string {
	if profile == nil {
		return nil
	}
	return &profile.Label
}

func profileModelID(profile *ai.Profile) *string {
	if profile == nil {
		return nil
	}
	return profile.ModelID
}

func matchesContentEtag(values []string, contentEtag string) bool {
	for _, value := range values {
		for _, candidate := range strings.Split(value, ",") {
			candidate = strings.TrimSpace(candidate)
			if candidate == "*" || candidate == contentEtag || candidate == `"`+contentEtag+`"` || candidate == `W/"`+contentEtag+`"` {
				return true
			}
		}
	}
	return false
}

func writeResult(w http.ResponseWriter, value any, err error) {
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Internal server error.")
		return
	}
	writeJSON(w, http.StatusOK, value)
}
