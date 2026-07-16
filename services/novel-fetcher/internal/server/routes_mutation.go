package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"narou-viewer/services/novel-fetcher/internal/taskqueue"
	"narou-viewer/services/novel-fetcher/internal/taskstate"
)

func (a *App) handleDownloadNovels(writer http.ResponseWriter, request *http.Request) {
	var body struct {
		Targets              []string `json:"targets"`
		Force                bool     `json:"force"`
		ConvertAfterDownload bool     `json:"convert_after_download"`
		Mail                 bool     `json:"mail"`
	}
	if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
		writeError(writer, http.StatusBadRequest, "invalid JSON body")
		return
	}

	targets := normalizeStrings(body.Targets)
	if len(targets) == 0 {
		writeError(writer, http.StatusBadRequest, "targets must be a non-empty string array")
		return
	}
	if body.ConvertAfterDownload {
		writeError(writer, http.StatusNotImplemented, "convert_after_download is not supported by novel-fetcher")
		return
	}
	if body.Mail {
		writeError(writer, http.StatusNotImplemented, "mail is not supported by novel-fetcher")
		return
	}

	existingNovelIDs, err := a.existingDownloadNovelIDsByTarget(targets)
	if err != nil {
		writeError(writer, http.StatusInternalServerError, err.Error())
		return
	}

	tasks := make([]*taskqueue.Task, 0, len(targets))
	for _, target := range targets {
		task := taskqueue.NewTask("download")
		task.Targets = []string{target}
		task.NovelIDs = existingNovelIDs[normalizeDownloadTargetKey(target)]
		task.Force = body.Force
		tasks = append(tasks, task)
	}
	if err := a.queue.Enqueue(tasks...); err != nil {
		writeTaskStateError(writer, err)
		return
	}

	writeEnvelope(writer, http.StatusAccepted, map[string]any{
		"targets":                targets,
		"force":                  body.Force,
		"convert_after_download": body.ConvertAfterDownload,
		"mail":                   body.Mail,
		"task_ids":               taskqueue.TaskIDs(tasks),
	}, "Download queued")
}

func (a *App) handleUpdateNovels(writer http.ResponseWriter, request *http.Request) {
	var body struct {
		IDs                []int `json:"ids"`
		ForceRedownload    bool  `json:"force_redownload"`
		IncludeFrozen      bool  `json:"include_frozen"`
		ConvertAfterUpdate bool  `json:"convert_after_update"`
		SkipUnchanged      *bool `json:"skip_unchanged"`
	}
	if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
		writeError(writer, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if len(body.IDs) == 0 {
		writeError(writer, http.StatusBadRequest, "ids must be a non-empty array")
		return
	}
	if body.IncludeFrozen {
		writeError(writer, http.StatusNotImplemented, "include_frozen is not supported by novel-fetcher")
		return
	}
	if body.ConvertAfterUpdate {
		writeError(writer, http.StatusNotImplemented, "convert_after_update is not supported by novel-fetcher")
		return
	}

	skipUnchanged := body.SkipUnchanged != nil && *body.SkipUnchanged
	tasks := make([]*taskqueue.Task, 0, len(body.IDs))
	for _, id := range body.IDs {
		if _, ok, err := a.store.FindWorkByID(id); err != nil {
			writeError(writer, http.StatusInternalServerError, err.Error())
			return
		} else if !ok {
			writeError(writer, http.StatusNotFound, fmt.Sprintf("novel id %d was not found", id))
			return
		}

		task := taskqueue.NewTask("update")
		task.NovelIDs = []int{id}
		task.ForceRedownload = body.ForceRedownload
		task.SkipUnchanged = skipUnchanged
		tasks = append(tasks, task)
	}
	if err := a.queue.Enqueue(tasks...); err != nil {
		writeTaskStateError(writer, err)
		return
	}

	writeEnvelope(writer, http.StatusAccepted, map[string]any{
		"ids":                  taskqueue.IntIDsToStrings(body.IDs),
		"force_redownload":     body.ForceRedownload,
		"include_frozen":       body.IncludeFrozen,
		"convert_after_update": body.ConvertAfterUpdate,
		"skip_unchanged":       skipUnchanged,
		"task_ids":             taskqueue.TaskIDs(tasks),
	}, "Update queued")
}

func (a *App) handleResumeNovels(writer http.ResponseWriter, request *http.Request) {
	var body struct {
		IDs []int `json:"ids"`
	}
	if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
		writeError(writer, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if len(body.IDs) == 0 {
		writeError(writer, http.StatusBadRequest, "ids must be a non-empty array")
		return
	}

	tasks := make([]*taskqueue.Task, 0, len(body.IDs))
	for _, id := range body.IDs {
		if _, ok, err := a.store.FindWorkByID(id); err != nil {
			writeError(writer, http.StatusInternalServerError, err.Error())
			return
		} else if !ok {
			writeError(writer, http.StatusNotFound, fmt.Sprintf("novel id %d was not found", id))
			return
		}

		task := taskqueue.NewTask("resume")
		task.NovelIDs = []int{id}
		tasks = append(tasks, task)
	}
	if err := a.queue.Enqueue(tasks...); err != nil {
		writeTaskStateError(writer, err)
		return
	}

	writeEnvelope(writer, http.StatusAccepted, map[string]any{
		"ids":      taskqueue.IntIDsToStrings(body.IDs),
		"task_ids": taskqueue.TaskIDs(tasks),
	}, "Resume queued")
}

func (a *App) handleRemoveNovels(writer http.ResponseWriter, request *http.Request) {
	var body struct {
		IDs       []string `json:"ids"`
		WithFiles bool     `json:"with_files"`
	}
	if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
		writeError(writer, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if len(body.IDs) == 0 {
		writeError(writer, http.StatusBadRequest, "ids must be a non-empty array")
		return
	}

	for _, rawID := range body.IDs {
		id, err := strconv.Atoi(rawID)
		if err != nil {
			writeError(writer, http.StatusBadRequest, "ids must contain numeric strings")
			return
		}
		if err := a.store.RemoveWork(id, body.WithFiles); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				writeError(writer, http.StatusNotFound, fmt.Sprintf("novel id %d was not found", id))
				return
			}
			writeError(writer, http.StatusInternalServerError, err.Error())
			return
		}
	}

	writeEnvelope(writer, http.StatusAccepted, map[string]any{"ids": body.IDs}, "Novel removed")
}

func (a *App) handleCancelTask(writer http.ResponseWriter, request *http.Request) {
	taskID := strings.TrimSpace(request.PathValue("taskID"))
	if taskID == "" {
		writeError(writer, http.StatusBadRequest, "task id is required")
		return
	}

	result, err := a.queue.RequestCancel(taskID)
	if err != nil {
		writeTaskStateError(writer, err)
		return
	}
	if result.Task != nil && result.Task.Status == taskqueue.StatusRunning {
		a.runner.Cancel(taskID)
	}
	status := http.StatusOK
	if result.Task != nil && result.Task.Status == taskqueue.StatusRunning && result.Changed {
		status = http.StatusAccepted
	}
	writeEnvelope(writer, status, taskqueue.Payload(result.Task), "Task cancelled")
}

func (a *App) handlePauseTask(writer http.ResponseWriter, request *http.Request) {
	a.handleTaskControl(writer, request, "pause")
}

func (a *App) handleResumeTask(writer http.ResponseWriter, request *http.Request) {
	a.handleTaskControl(writer, request, "resume")
}

func (a *App) handleTaskControl(writer http.ResponseWriter, request *http.Request, action string) {
	taskID := strings.TrimSpace(request.PathValue("taskID"))
	if taskID == "" {
		writeError(writer, http.StatusBadRequest, "task id is required")
		return
	}
	var result taskstate.ControlResult
	var err error
	switch action {
	case "pause":
		result, err = a.queue.RequestPause(taskID)
	case "resume":
		result, err = a.queue.RequestResume(taskID)
	default:
		err = errors.New("unknown task action")
	}
	if err != nil {
		writeTaskStateError(writer, err)
		return
	}
	if action == "pause" && result.Task != nil && result.Task.Status == taskqueue.StatusRunning {
		a.runner.Pause(taskID)
	}
	status := http.StatusOK
	if result.Changed && result.Task != nil && (result.Task.Status == taskqueue.StatusRunning || result.Task.Status == taskqueue.StatusQueued) {
		status = http.StatusAccepted
	}
	message := "Task " + action
	writeEnvelope(writer, status, taskqueue.Payload(result.Task), message)
}

func writeTaskStateError(writer http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	message := err.Error()
	if errors.Is(err, taskstate.ErrTaskNotFound) {
		status = http.StatusNotFound
	}
	if errors.Is(err, taskstate.ErrTaskAlreadyActive) || errors.Is(err, taskstate.ErrTaskStateConflict) {
		status = http.StatusConflict
	}
	writeError(writer, status, message)
}
