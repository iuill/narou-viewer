package server

import (
	"net/http"
	"strconv"
	"strings"

	"narou-viewer/services/novel-fetcher/internal/config"
	"narou-viewer/services/novel-fetcher/internal/model"
	"narou-viewer/services/novel-fetcher/internal/storage"
	"narou-viewer/services/novel-fetcher/internal/taskqueue"
)

func (a *App) handleHealth(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, map[string]any{
		"status":  "ok",
		"version": config.Version,
	})
}

func (a *App) handleVersion(writer http.ResponseWriter, _ *http.Request) {
	version := "novel-fetcher/" + config.Version
	writeEnvelope(writer, http.StatusOK, map[string]any{
		"current": version,
		"latest":  version,
	}, nil)
}

func (a *App) handleQueue(writer http.ResponseWriter, _ *http.Request) {
	counts := a.queue.StatusCounts()

	writeEnvelope(writer, http.StatusOK, map[string]any{
		"total":      counts.Total,
		"web_worker": 0,
		"worker":     counts.Total,
		"running":    counts.Running,
	}, nil)
}

func (a *App) handleTasksSummary(writer http.ResponseWriter, _ *http.Request) {
	summary := a.queue.Summary()

	writeEnvelope(writer, http.StatusOK, map[string]any{
		"current":           summary.Current,
		"queued":            summary.Queued,
		"recent_completed":  summary.RecentCompleted,
		"recent_failed":     summary.RecentFailed,
		"paused":            summary.Paused,
		"interrupted":       summary.Interrupted,
		"completed_count":   summary.CompletedCount,
		"failed_count":      summary.FailedCount,
		"canceled_count":    summary.CanceledCount,
		"paused_count":      summary.PausedCount,
		"interrupted_count": summary.InterruptedCount,
		"convert_current":   nil,
		"convert_queued":    []any{},
	}, nil)
}

func (a *App) handleTask(writer http.ResponseWriter, request *http.Request) {
	taskID := strings.TrimSpace(request.PathValue("taskID"))
	if taskID == "" {
		writeError(writer, http.StatusBadRequest, "task id is required")
		return
	}
	task, found, err := a.queue.GetTask(taskID)
	if err != nil {
		writeTaskStateError(writer, err)
		return
	}
	if !found {
		writeError(writer, http.StatusNotFound, "task was not found")
		return
	}
	writeEnvelope(writer, http.StatusOK, taskqueue.Payload(task), nil)
}

func (a *App) handleListNovels(writer http.ResponseWriter, _ *http.Request) {
	works, err := a.store.ListWorks()
	if err != nil {
		writeError(writer, http.StatusInternalServerError, err.Error())
		return
	}

	novels := make([]map[string]any, 0, len(works))
	for _, work := range works {
		novels = append(novels, map[string]any{
			"id":           work.ID,
			"title":        work.Title,
			"title_plain":  work.Title,
			"author":       work.Author,
			"author_plain": work.Author,
			"sitename":     work.SiteName,
			"toc_url":      work.SourceURL,
		})
	}

	writeEnvelope(writer, http.StatusOK, map[string]any{"novels": novels}, nil)
}

func (a *App) handleListWorks(writer http.ResponseWriter, _ *http.Request) {
	works, err := a.store.ListWorks()
	if err != nil {
		writeError(writer, http.StatusInternalServerError, err.Error())
		return
	}

	payloads := make([]map[string]any, 0, len(works))
	for _, work := range works {
		payloads = append(payloads, workPayload(work))
	}
	writeJSON(writer, http.StatusOK, map[string]any{"works": payloads})
}

func (a *App) handleGetWork(writer http.ResponseWriter, request *http.Request) {
	work, ok := a.resolveWork(writer, request)
	if !ok {
		return
	}
	writeJSON(writer, http.StatusOK, workPayload(work))
}

func (a *App) handleListTocs(writer http.ResponseWriter, request *http.Request) {
	workIDs, ok := parsePositiveIntList(request.URL.Query().Get("ids"))
	if !ok {
		writeError(writer, http.StatusBadRequest, "ids must be a comma-separated positive integer list")
		return
	}
	if len(workIDs) == 0 {
		writeJSON(writer, http.StatusOK, map[string]any{"works": []any{}})
		return
	}
	works, err := a.store.ListWorks()
	if err != nil {
		writeError(writer, http.StatusInternalServerError, err.Error())
		return
	}
	worksByID := make(map[int]model.StoredWork, len(works))
	for _, work := range works {
		worksByID[work.ID] = work
	}
	payloads := make([]map[string]any, 0, len(workIDs))
	for _, workID := range workIDs {
		work, found := worksByID[workID]
		if !found {
			continue
		}
		episodes, err := a.store.ListEpisodes(work.ID)
		if err != nil {
			writeError(writer, http.StatusInternalServerError, err.Error())
			return
		}
		payloads = append(payloads, tocPayload(work, episodes))
	}
	writeJSON(writer, http.StatusOK, map[string]any{"works": payloads})
}

func (a *App) handleGetToc(writer http.ResponseWriter, request *http.Request) {
	work, ok := a.resolveWork(writer, request)
	if !ok {
		return
	}
	episodes, err := a.store.ListEpisodes(work.ID)
	if err != nil {
		writeError(writer, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(writer, http.StatusOK, tocPayload(work, episodes))
}

func tocPayload(work model.StoredWork, episodes []model.StoredEpisode) map[string]any {
	episodePayloads := make([]map[string]any, 0, len(episodes))
	for _, episode := range episodes {
		episodePayloads = append(episodePayloads, episodeSummaryPayload(episode))
	}
	payload := workPayload(work)
	payload["episodes"] = episodePayloads
	return payload
}

func parsePositiveIntList(raw string) ([]int, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []int{}, true
	}
	seen := map[int]struct{}{}
	values := []int{}
	for _, part := range strings.Split(raw, ",") {
		value, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || value <= 0 {
			return nil, false
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		values = append(values, value)
	}
	return values, true
}

func (a *App) handleGetEpisode(writer http.ResponseWriter, request *http.Request) {
	work, ok := a.resolveWork(writer, request)
	if !ok {
		return
	}

	episodeID := strings.TrimSpace(request.PathValue("episodeID"))
	if episodeID == "" {
		writeError(writer, http.StatusBadRequest, "episode id is required")
		return
	}

	episode, found, err := a.store.FindEpisode(work.ID, episodeID)
	if err != nil {
		writeError(writer, http.StatusInternalServerError, err.Error())
		return
	}
	if !found {
		writeError(writer, http.StatusNotFound, "episode was not found")
		return
	}
	if episode.BodyStatus != storage.BodyStatusComplete || episode.BodyPath == "" {
		writeError(writer, http.StatusNotFound, "episode body was not fetched")
		return
	}

	canonical, err := a.store.ReadCanonicalEpisode(episode)
	if err != nil {
		writeError(writer, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(writer, http.StatusOK, map[string]any{
		"work":      workPayload(work),
		"episode":   episodeSummaryPayload(episode),
		"canonical": canonical,
	})
}

func (a *App) resolveWork(writer http.ResponseWriter, request *http.Request) (model.StoredWork, bool) {
	workID, err := strconv.Atoi(strings.TrimSpace(request.PathValue("workID")))
	if err != nil || workID <= 0 {
		writeError(writer, http.StatusBadRequest, "work id must be a positive integer")
		return model.StoredWork{}, false
	}

	work, found, err := a.store.FindWorkByID(workID)
	if err != nil {
		writeError(writer, http.StatusInternalServerError, err.Error())
		return model.StoredWork{}, false
	}
	if !found {
		writeError(writer, http.StatusNotFound, "work was not found")
		return model.StoredWork{}, false
	}
	return work, true
}
