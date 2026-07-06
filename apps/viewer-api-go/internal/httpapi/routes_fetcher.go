package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"narou-viewer/apps/viewer-api-go/internal/ai"
	"narou-viewer/apps/viewer-api-go/internal/application/fetchercommands"
	"narou-viewer/apps/viewer-api-go/internal/fetcher"
)

type fetcherDownloadRequest struct {
	Targets              []string `json:"targets"`
	Force                bool     `json:"force"`
	ConvertAfterDownload bool     `json:"convertAfterDownload"`
	Mail                 bool     `json:"mail"`
}

type fetcherNovelIDsRequest struct {
	NovelIDs           []string `json:"novelIds"`
	ForceRedownload    *bool    `json:"forceRedownload"`
	IncludeFrozen      *bool    `json:"includeFrozen"`
	ConvertAfterUpdate *bool    `json:"convertAfterUpdate"`
	SkipUnchanged      *bool    `json:"skipUnchanged"`
	WithFiles          *bool    `json:"withFiles"`
}

func (s *Server) handleFetcherStatus(w http.ResponseWriter, r *http.Request) {
	if !methodOnly(w, r, http.MethodGet) {
		return
	}
	if status, err := s.fetcherClient.Status(r.Context()); err == nil {
		writeJSON(w, http.StatusOK, status)
		return
	}
	writeJSON(w, http.StatusOK, fallbackFetcherStatus())
}

func (s *Server) handleFetcherQueue(w http.ResponseWriter, r *http.Request) {
	if !methodOnly(w, r, http.MethodGet) {
		return
	}
	if queue, err := s.fetcherClient.Queue(r.Context()); err == nil {
		writeJSON(w, http.StatusOK, queue)
		return
	}
	writeJSON(w, http.StatusOK, queueShape())
}

func (s *Server) handleFetcherTaskSummary(w http.ResponseWriter, r *http.Request) {
	if !methodOnly(w, r, http.MethodGet) {
		return
	}
	if summary, err := s.fetcherClient.TasksSummary(r.Context()); err == nil {
		writeJSON(w, http.StatusOK, summary)
		return
	}
	writeJSON(w, http.StatusOK, statusTaskSummaryShape())
}

func (s *Server) handleFetcherDownload(w http.ResponseWriter, r *http.Request) {
	if !methodOnly(w, r, http.MethodPost) {
		return
	}
	body, ok := decodeJSONOrBadRequest[fetcherDownloadRequest](w, r)
	if !ok {
		return
	}
	targetStrings, ok := normalizeStringList(body.Targets)
	if !ok {
		writeError(w, http.StatusBadRequest, "targets must be a non-empty string array.")
		return
	}
	result, err := s.fetcherCommands.Download(
		r.Context(),
		targetStrings,
		fetchercommands.DownloadOptions{
			Force:                body.Force,
			ConvertAfterDownload: body.ConvertAfterDownload,
			Mail:                 body.Mail,
		},
	)
	if err != nil {
		writeFetcherError(w, err, "Failed to queue fetcher download.")
		return
	}
	writeJSON(w, http.StatusAccepted, result)
}

func (s *Server) handleFetcherNovelIDsAction(w http.ResponseWriter, r *http.Request) {
	if !methodOnly(w, r, http.MethodPost) {
		return
	}
	body, ok := decodeJSONOrBadRequest[fetcherNovelIDsRequest](w, r)
	if !ok {
		return
	}
	normalizedNovelIDs, ok := normalizeStringList(body.NovelIDs)
	if !ok {
		writeError(w, http.StatusBadRequest, "novelIds must be a non-empty string array.")
		return
	}
	action := strings.TrimPrefix(r.URL.Path, "/api/fetcher/works/")
	var result any
	var err error
	switch action {
	case "update":
		result, err = s.fetcherCommands.Update(
			r.Context(),
			normalizedNovelIDs,
			fetchercommands.UpdateOptions{
				ForceRedownload:    boolPointerValue(body.ForceRedownload, false),
				IncludeFrozen:      boolPointerValue(body.IncludeFrozen, false),
				ConvertAfterUpdate: boolPointerValue(body.ConvertAfterUpdate, false),
				SkipUnchanged:      boolPointerValue(body.SkipUnchanged, true),
			},
		)
	case "resume":
		result, err = s.fetcherCommands.Resume(r.Context(), normalizedNovelIDs)
	case "remove":
		withFiles := boolPointerValue(body.WithFiles, true)
		result, err = s.fetcherCommands.Remove(
			r.Context(),
			normalizedNovelIDs,
			withFiles,
		)
	default:
		writeError(w, http.StatusNotFound, "Not found.")
		return
	}
	if err != nil {
		writeFetcherCommandError(w, err, "Failed to queue fetcher request.")
		return
	}
	writeJSON(w, http.StatusAccepted, result)
}

func (s *Server) handleFetcherTaskAction(w http.ResponseWriter, r *http.Request) {
	if !methodOnly(w, r, http.MethodPost) {
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/fetcher/tasks/")
	if !strings.HasSuffix(rest, "/cancel") {
		writeError(w, http.StatusNotFound, "Not found.")
		return
	}
	taskID := trimPathValue(strings.TrimSuffix(rest, "/cancel"))
	if taskID == "" {
		writeError(w, http.StatusBadRequest, "taskId is required.")
		return
	}
	result, err := s.fetcherCommands.CancelTask(r.Context(), taskID)
	if err != nil {
		writeFetcherError(w, err, "Failed to cancel novel-fetcher task.")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func queueShape() map[string]any {
	return map[string]any{
		"total":     0,
		"webWorker": 0,
		"worker":    0,
		"running":   false,
		"available": false,
		"degraded":  true,
	}
}

func normalizeStringList(items []string) ([]string, bool) {
	if len(items) == 0 {
		return nil, false
	}
	result := make([]string, 0, len(items))
	seen := map[string]bool{}
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			return nil, false
		}
		if seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		result = append(result, trimmed)
	}
	return result, true
}

func nowString() string {
	return ai.NowISO()
}

func fallbackFetcherStatus() map[string]any {
	return map[string]any{
		"version":   map[string]any{"current": nil, "latest": nil},
		"queue":     queueShape(),
		"tasks":     statusTaskSummaryShape(),
		"checkedAt": nowString(),
		"available": false,
		"degraded":  true,
	}
}

func statusTaskSummaryShape() map[string]any {
	return map[string]any{
		"current":         nil,
		"queued":          []any{},
		"recentCompleted": []any{},
		"recentFailed":    []any{},
		"completedCount":  0,
		"failedCount":     0,
		"convertCurrent":  nil,
		"convertQueued":   []any{},
		"available":       false,
		"degraded":        true,
	}
}

func boolPointerValue(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

func writeFetcherCommandError(w http.ResponseWriter, err error, fallback string) {
	var missing fetchercommands.MissingNovelsError
	if errors.As(err, &missing) {
		details := map[string]any{
			"missingNovelIds": missing.NovelIDs,
		}
		writeAPIErrorWithFields(w, http.StatusNotFound, "NOVELS_NOT_FOUND", "Some novelIds were not found in the local library.", details, details)
		return
	}
	writeFetcherError(w, err, fallback)
}

func writeFetcherError(w http.ResponseWriter, err error, fallback string) {
	status := fetcherErrorStatus(err)
	message := fetcherRouteErrorMessage(err)
	if message == "" {
		message = fallback
	}
	writeError(w, status, message)
}

func fetcherRouteErrorMessage(err error) string {
	var httpError *fetcher.HTTPError
	if errors.As(err, &httpError) {
		if message := strings.TrimSpace(httpError.Message); message != "" {
			return message
		}
		return fmt.Sprintf("Fetcher API request failed with HTTP %d.", httpError.StatusCode)
	}
	return strings.TrimSpace(err.Error())
}

func fetcherErrorStatus(err error) int {
	var httpError *fetcher.HTTPError
	if errors.As(err, &httpError) {
		switch httpError.StatusCode {
		case http.StatusBadRequest, http.StatusNotFound, http.StatusConflict, http.StatusNotImplemented:
			return httpError.StatusCode
		}
	}
	return http.StatusBadGateway
}
