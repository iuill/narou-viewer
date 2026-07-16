package fetcher

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var ErrUnavailable = errors.New("novel-fetcher unavailable")

type HTTPError struct {
	StatusCode int
	Message    string
}

func (e *HTTPError) Error() string {
	if strings.TrimSpace(e.Message) != "" {
		return fmt.Sprintf("novel-fetcher request failed: HTTP %d: %s", e.StatusCode, e.Message)
	}
	return fmt.Sprintf("novel-fetcher request failed: HTTP %d", e.StatusCode)
}

type Client struct {
	baseURL    string
	httpClient *http.Client
}

type StatusResponse struct {
	CheckedAt string              `json:"checkedAt"`
	Version   VersionResponse     `json:"version"`
	Queue     QueueResponse       `json:"queue"`
	Tasks     TaskSummaryResponse `json:"tasks"`
}

type VersionResponse struct {
	Current *string `json:"current"`
	Latest  *string `json:"latest"`
}

type QueueResponse struct {
	Total     int  `json:"total"`
	WebWorker int  `json:"webWorker"`
	Worker    int  `json:"worker"`
	Running   bool `json:"running"`
}

type TaskPayload map[string]json.RawMessage

func (payload *TaskPayload) UnmarshalJSON(raw []byte) error {
	if string(raw) == "null" {
		*payload = nil
		return nil
	}
	var record map[string]json.RawMessage
	if err := json.Unmarshal(raw, &record); err != nil {
		*payload = nil
		return nil
	}
	*payload = TaskPayload(record)
	return nil
}

type TaskSummaryResponse struct {
	Current          *TaskPayload  `json:"current"`
	Queued           []TaskPayload `json:"queued"`
	Paused           []TaskPayload `json:"paused"`
	Interrupted      []TaskPayload `json:"interrupted"`
	RecentCompleted  []TaskPayload `json:"recentCompleted"`
	RecentFailed     []TaskPayload `json:"recentFailed"`
	CompletedCount   int           `json:"completedCount"`
	FailedCount      int           `json:"failedCount"`
	CanceledCount    int           `json:"canceledCount"`
	PausedCount      int           `json:"pausedCount"`
	InterruptedCount int           `json:"interruptedCount"`
	ConvertCurrent   *TaskPayload  `json:"convertCurrent"`
	ConvertQueued    []TaskPayload `json:"convertQueued"`
}

var taskPayloadCanonicalKeys = map[string]string{
	"task_id":             "taskId",
	"novel_ids":           "novelIds",
	"novel_id":            "novelId",
	"novel_title":         "novelTitle",
	"novel_author":        "novelAuthor",
	"source_url":          "sourceUrl",
	"created_at":          "createdAt",
	"started_at":          "startedAt",
	"completed_at":        "completedAt",
	"finished_at":         "finishedAt",
	"error_message":       "errorMessage",
	"elapsed_time":        "elapsedTime",
	"total_steps":         "totalSteps",
	"current_step":        "currentStep",
	"saved_episode_count": "savedEpisodeCount",
	"failed_episode_id":   "failedEpisodeId",
	"resume_episode_id":   "resumeEpisodeId",
	"force_redownload":    "forceRedownload",
	"skip_unchanged":      "skipUnchanged",
	"requested_action":    "requestedAction",
	"attempt_count":       "attemptCount",
	"queue_position":      "queuePosition",
	"can_pause":           "canPause",
	"can_resume":          "canResume",
	"can_cancel":          "canCancel",
	"paused_at":           "pausedAt",
	"interrupted_at":      "interruptedAt",
}

type DownloadResponse struct {
	Targets              []string `json:"targets"`
	Force                bool     `json:"force"`
	ConvertAfterDownload bool     `json:"convertAfterDownload"`
	Mail                 bool     `json:"mail"`
	TaskIDs              []string `json:"taskIds"`
	Message              string   `json:"message"`
}

type UpdateResponse struct {
	IDs                []string `json:"ids"`
	ForceRedownload    bool     `json:"forceRedownload"`
	IncludeFrozen      bool     `json:"includeFrozen"`
	ConvertAfterUpdate bool     `json:"convertAfterUpdate"`
	SkipUnchanged      bool     `json:"skipUnchanged"`
	TaskIDs            []string `json:"taskIds"`
	Message            string   `json:"message"`
}

type ResumeResponse struct {
	IDs     []string `json:"ids"`
	TaskIDs []string `json:"taskIds"`
	Message string   `json:"message"`
}

type RemoveResponse struct {
	IDs     []string `json:"ids"`
	Message string   `json:"message"`
}

type TaskControlResponse struct {
	TaskID          string `json:"taskId"`
	Status          string `json:"status"`
	RequestedAction string `json:"requestedAction"`
	Changed         bool   `json:"changed"`
	Cancelled       bool   `json:"cancelled"`
	Message         string `json:"message"`
}

type CancelTaskResponse = TaskControlResponse

type LibraryWork struct {
	ID                  int    `json:"id"`
	Site                string `json:"site"`
	SiteName            string `json:"site_name"`
	SiteWorkID          string `json:"site_work_id"`
	SourceURL           string `json:"source_url"`
	Title               string `json:"title"`
	Author              string `json:"author"`
	Story               string `json:"story"`
	Directory           string `json:"directory"`
	FetchedAt           string `json:"fetched_at"`
	EpisodeLen          int    `json:"episode_count"`
	SavedEpisodeLen     int    `json:"saved_episode_count"`
	FetchStatus         string `json:"fetch_status"`
	LastFetchError      string `json:"last_fetch_error"`
	LastFailedEpisodeID string `json:"failed_episode_id"`
	ResumeEpisodeID     string `json:"resume_episode_id"`
	ExpectedEpisodeLen  int    `json:"expected_episode_count"`
}

type LibraryEpisode struct {
	EpisodeID      string `json:"episode_id"`
	SiteEpisodeID  string `json:"site_episode_id"`
	SourceURL      string `json:"source_url"`
	SortOrder      int    `json:"sort_order"`
	DisplayIndex   string `json:"display_index"`
	Title          string `json:"title"`
	Chapter        string `json:"chapter"`
	Subchapter     string `json:"subchapter"`
	PublishedAt    string `json:"published_at"`
	UpdatedAt      string `json:"updated_at"`
	ContentHash    string `json:"content_hash"`
	FetchedAt      string `json:"fetched_at"`
	BodyStatus     string `json:"body_status"`
	LastFetchError string `json:"last_fetch_error"`
}

type LibraryEpisodeResponse struct {
	Work      LibraryWork     `json:"work"`
	Episode   LibraryEpisode  `json:"episode"`
	Canonical json.RawMessage `json:"canonical"`
}

type libraryWorksResponse struct {
	Works []LibraryWork `json:"works"`
}

type libraryTocResponse struct {
	LibraryWork
	Episodes []LibraryEpisode `json:"episodes"`
}

type libraryTocsResponse struct {
	Works []libraryTocResponse `json:"works"`
}

type envelope[T any] struct {
	Success bool   `json:"success"`
	Data    *T     `json:"data"`
	Message string `json:"message"`
}

type versionData struct {
	Current *string `json:"current"`
	Latest  *string `json:"latest"`
}

type queueData struct {
	Total     IntValue `json:"total"`
	WebWorker IntValue `json:"web_worker"`
	Worker    IntValue `json:"worker"`
	Running   *bool    `json:"running"`
}

type taskSummaryData struct {
	Current          *TaskPayload  `json:"current"`
	Queued           []TaskPayload `json:"queued"`
	Paused           []TaskPayload `json:"paused"`
	Interrupted      []TaskPayload `json:"interrupted"`
	RecentCompleted  []TaskPayload `json:"recent_completed"`
	RecentFailed     []TaskPayload `json:"recent_failed"`
	CompletedCount   IntValue      `json:"completed_count"`
	FailedCount      IntValue      `json:"failed_count"`
	CanceledCount    IntValue      `json:"canceled_count"`
	PausedCount      IntValue      `json:"paused_count"`
	InterruptedCount IntValue      `json:"interrupted_count"`
	ConvertCurrent   *TaskPayload  `json:"convert_current"`
	ConvertQueued    []TaskPayload `json:"convert_queued"`
}

type downloadData struct {
	Targets              StringList `json:"targets"`
	Force                *bool      `json:"force"`
	ConvertAfterDownload *bool      `json:"convert_after_download"`
	Mail                 *bool      `json:"mail"`
	TaskIDs              StringList `json:"task_ids"`
}

type updateData struct {
	IDs                StringList `json:"ids"`
	ForceRedownload    *bool      `json:"force_redownload"`
	IncludeFrozen      *bool      `json:"include_frozen"`
	ConvertAfterUpdate *bool      `json:"convert_after_update"`
	SkipUnchanged      *bool      `json:"skip_unchanged"`
	TaskIDs            StringList `json:"task_ids"`
}

type resumeData struct {
	IDs     StringList `json:"ids"`
	TaskIDs StringList `json:"task_ids"`
}

type removeData struct {
	IDs StringList `json:"ids"`
}

type cancelTaskData struct {
	TaskID          string `json:"task_id"`
	CanonicalTaskID string `json:"taskId"`
	Status          string `json:"status"`
	RequestedAction string `json:"requested_action"`
	CanonicalAction string `json:"requestedAction"`
	Changed         bool   `json:"changed"`
	Cancelled       bool   `json:"cancelled"`
}

type downloadRequest struct {
	Targets              []string `json:"targets"`
	Force                bool     `json:"force"`
	ConvertAfterDownload bool     `json:"convert_after_download"`
	Mail                 bool     `json:"mail"`
}

type updateRequest struct {
	IDs                []int `json:"ids"`
	ForceRedownload    bool  `json:"force_redownload"`
	IncludeFrozen      bool  `json:"include_frozen"`
	ConvertAfterUpdate bool  `json:"convert_after_update"`
	SkipUnchanged      bool  `json:"skip_unchanged"`
}

type resumeRequest struct {
	IDs []int `json:"ids"`
}

type removeRequest struct {
	IDs       []string `json:"ids"`
	WithFiles bool     `json:"with_files"`
}

type IntValue int

func (v *IntValue) UnmarshalJSON(raw []byte) error {
	var number int
	if err := json.Unmarshal(raw, &number); err == nil && number >= 0 {
		*v = IntValue(number)
		return nil
	}
	var floatNumber float64
	if err := json.Unmarshal(raw, &floatNumber); err == nil && floatNumber >= 0 {
		*v = IntValue(floatNumber)
		return nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		var parsed int
		if _, scanErr := fmt.Sscanf(text, "%d", &parsed); scanErr == nil && parsed >= 0 {
			*v = IntValue(parsed)
		}
		return nil
	}
	return nil
}

type StringList []string

func (list *StringList) UnmarshalJSON(raw []byte) error {
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return err
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		var text string
		if err := json.Unmarshal(item, &text); err == nil {
			if trimmed := strings.TrimSpace(text); trimmed != "" {
				result = append(result, trimmed)
			}
			continue
		}
		var number float64
		if err := json.Unmarshal(item, &number); err == nil && number >= 0 && number == float64(int(number)) {
			result = append(result, fmt.Sprintf("%d", int(number)))
		}
	}
	*list = result
	return nil
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *Client) Status(ctx context.Context) (StatusResponse, error) {
	version, _, err := getData[versionData](c, ctx, "/api/v2/system/version")
	if err != nil {
		return StatusResponse{}, err
	}
	queue, err := c.Queue(ctx)
	if err != nil {
		return StatusResponse{}, err
	}
	tasks, err := c.TasksSummary(ctx)
	if err != nil {
		return StatusResponse{}, err
	}
	return StatusResponse{
		CheckedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Version: VersionResponse{
			Current: cleanStringPointer(version.Current),
			Latest:  cleanStringPointer(version.Latest),
		},
		Queue: queue,
		Tasks: tasks,
	}, nil
}

func (c *Client) Queue(ctx context.Context) (QueueResponse, error) {
	payload, _, err := getData[queueData](c, ctx, "/api/v2/system/queue")
	if err != nil {
		return QueueResponse{}, err
	}
	total := int(payload.Total)
	running := total > 0
	if payload.Running != nil {
		running = *payload.Running
	} else {
		running = total > 0
	}
	return QueueResponse{
		Total:     total,
		WebWorker: int(payload.WebWorker),
		Worker:    int(payload.Worker),
		Running:   running,
	}, nil
}

func (c *Client) TasksSummary(ctx context.Context) (TaskSummaryResponse, error) {
	payload, _, err := getData[taskSummaryData](c, ctx, "/api/v2/tasks/summary")
	if err != nil {
		return TaskSummaryResponse{}, err
	}
	return TaskSummaryResponse{
		Current:          taskPointer(payload.Current),
		Queued:           taskList(payload.Queued),
		Paused:           taskList(payload.Paused),
		Interrupted:      taskList(payload.Interrupted),
		RecentCompleted:  taskList(payload.RecentCompleted),
		RecentFailed:     taskList(payload.RecentFailed),
		CompletedCount:   int(payload.CompletedCount),
		FailedCount:      int(payload.FailedCount),
		CanceledCount:    int(payload.CanceledCount),
		PausedCount:      int(payload.PausedCount),
		InterruptedCount: int(payload.InterruptedCount),
		ConvertCurrent:   taskPointer(payload.ConvertCurrent),
		ConvertQueued:    taskList(payload.ConvertQueued),
	}, nil
}

func (c *Client) Download(ctx context.Context, targets []string, force bool, convertAfterDownload bool, mail bool) (DownloadResponse, error) {
	data, message, err := postData[downloadData](c, ctx, "/api/v2/novels/download", downloadRequest{
		Targets:              targets,
		Force:                force,
		ConvertAfterDownload: convertAfterDownload,
		Mail:                 mail,
	})
	if err != nil {
		return DownloadResponse{}, err
	}
	responseTargets := []string(data.Targets)
	if len(responseTargets) == 0 {
		responseTargets = targets
	}
	return DownloadResponse{
		Targets:              responseTargets,
		Force:                boolPointerValue(data.Force, force),
		ConvertAfterDownload: boolPointerValue(data.ConvertAfterDownload, convertAfterDownload),
		Mail:                 boolPointerValue(data.Mail, mail),
		TaskIDs:              []string(data.TaskIDs),
		Message:              message,
	}, nil
}

func (c *Client) Update(ctx context.Context, ids []int, forceRedownload bool, includeFrozen bool, convertAfterUpdate bool, skipUnchanged bool) (UpdateResponse, error) {
	data, message, err := postData[updateData](c, ctx, "/api/v2/novels/update", updateRequest{
		IDs:                ids,
		ForceRedownload:    forceRedownload,
		IncludeFrozen:      includeFrozen,
		ConvertAfterUpdate: convertAfterUpdate,
		SkipUnchanged:      skipUnchanged,
	})
	if err != nil {
		return UpdateResponse{}, err
	}
	return UpdateResponse{
		IDs:                []string(data.IDs),
		ForceRedownload:    boolPointerValue(data.ForceRedownload, forceRedownload),
		IncludeFrozen:      boolPointerValue(data.IncludeFrozen, includeFrozen),
		ConvertAfterUpdate: boolPointerValue(data.ConvertAfterUpdate, convertAfterUpdate),
		SkipUnchanged:      boolPointerValue(data.SkipUnchanged, skipUnchanged),
		TaskIDs:            []string(data.TaskIDs),
		Message:            message,
	}, nil
}

func (c *Client) Resume(ctx context.Context, ids []int) (ResumeResponse, error) {
	data, message, err := postData[resumeData](c, ctx, "/api/v2/novels/resume", resumeRequest{IDs: ids})
	if err != nil {
		return ResumeResponse{}, err
	}
	return ResumeResponse{
		IDs:     []string(data.IDs),
		TaskIDs: []string(data.TaskIDs),
		Message: message,
	}, nil
}

func (c *Client) Remove(ctx context.Context, ids []string, withFiles bool) (RemoveResponse, error) {
	data, message, err := postData[removeData](c, ctx, "/api/v2/novels/remove", removeRequest{
		IDs:       ids,
		WithFiles: withFiles,
	})
	if err != nil {
		return RemoveResponse{}, err
	}
	responseIDs := []string(data.IDs)
	if len(responseIDs) == 0 {
		responseIDs = ids
	}
	return RemoveResponse{
		IDs:     responseIDs,
		Message: message,
	}, nil
}

func (c *Client) CancelTask(ctx context.Context, taskID string) (CancelTaskResponse, error) {
	return c.taskControl(ctx, taskID, "cancel")
}

func (c *Client) PauseTask(ctx context.Context, taskID string) (TaskControlResponse, error) {
	return c.taskControl(ctx, taskID, "pause")
}

func (c *Client) ResumeTask(ctx context.Context, taskID string) (TaskControlResponse, error) {
	return c.taskControl(ctx, taskID, "resume")
}

func (c *Client) taskControl(ctx context.Context, taskID string, action string) (TaskControlResponse, error) {
	data, message, err := postData[cancelTaskData](c, ctx, "/api/v2/tasks/"+url.PathEscape(taskID)+"/"+action, nil)
	if err != nil {
		return TaskControlResponse{}, err
	}
	responseTaskID := strings.TrimSpace(data.TaskID)
	if responseTaskID == "" {
		responseTaskID = strings.TrimSpace(data.CanonicalTaskID)
	}
	if responseTaskID == "" {
		responseTaskID = taskID
	}
	requestedAction := strings.TrimSpace(data.RequestedAction)
	if requestedAction == "" {
		requestedAction = strings.TrimSpace(data.CanonicalAction)
	}
	cancelled := data.Cancelled || (action == "cancel" && data.Status == "canceled")
	return TaskControlResponse{TaskID: responseTaskID, Status: data.Status, RequestedAction: requestedAction, Changed: data.Changed, Cancelled: cancelled, Message: message}, nil
}

func (c *Client) ListLibraryWorks(ctx context.Context) ([]LibraryWork, error) {
	response, err := getJSON[libraryWorksResponse](c, ctx, "/api/v1/works")
	if err != nil {
		return nil, err
	}
	return response.Works, nil
}

func (c *Client) GetLibraryToc(ctx context.Context, workID int) (LibraryWork, []LibraryEpisode, error) {
	response, err := getJSON[libraryTocResponse](c, ctx, "/api/v1/works/"+fmt.Sprint(workID)+"/toc")
	if err != nil {
		return LibraryWork{}, nil, err
	}
	work := response.LibraryWork
	return work, response.Episodes, nil
}

func (c *Client) ListLibraryTocs(ctx context.Context, workIDs []int) (map[int][]LibraryEpisode, error) {
	if len(workIDs) == 0 {
		return map[int][]LibraryEpisode{}, nil
	}
	ids := make([]string, 0, len(workIDs))
	seen := map[int]struct{}{}
	for _, workID := range workIDs {
		if workID <= 0 {
			continue
		}
		if _, ok := seen[workID]; ok {
			continue
		}
		seen[workID] = struct{}{}
		ids = append(ids, fmt.Sprint(workID))
	}
	if len(ids) == 0 {
		return map[int][]LibraryEpisode{}, nil
	}
	response, err := getJSON[libraryTocsResponse](c, ctx, "/api/v1/works/tocs?ids="+url.QueryEscape(strings.Join(ids, ",")))
	if err != nil {
		return nil, err
	}
	result := make(map[int][]LibraryEpisode, len(response.Works))
	for _, work := range response.Works {
		result[work.ID] = work.Episodes
	}
	return result, nil
}

func (c *Client) GetLibraryEpisode(ctx context.Context, workID int, episodeID string) (LibraryEpisodeResponse, error) {
	return getJSON[LibraryEpisodeResponse](c, ctx, "/api/v1/works/"+fmt.Sprint(workID)+"/episodes/"+url.PathEscape(episodeID))
}

func getData[T any](c *Client, ctx context.Context, path string) (T, string, error) {
	return requestData[T](c, ctx, http.MethodGet, path, nil)
}

func postData[T any](c *Client, ctx context.Context, path string, body any) (T, string, error) {
	return requestData[T](c, ctx, http.MethodPost, path, body)
}

func requestData[T any](c *Client, ctx context.Context, method string, path string, body any) (T, string, error) {
	var zero T
	raw, err := requestRaw(c, ctx, method, path, body)
	if err != nil {
		return zero, "", err
	}
	var decoded envelope[T]
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return zero, "", err
	}
	if !decoded.Success {
		return zero, "", errors.New("novel-fetcher returned an unexpected response")
	}
	if decoded.Data == nil {
		return zero, "", errors.New("novel-fetcher returned a successful response without data")
	}
	return *decoded.Data, strings.TrimSpace(decoded.Message), nil
}

func getJSON[T any](c *Client, ctx context.Context, path string) (T, error) {
	var zero T
	raw, err := requestRaw(c, ctx, http.MethodGet, path, nil)
	if err != nil {
		return zero, err
	}
	var decoded T
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return zero, err
	}
	return decoded, nil
}

func requestRaw(c *Client, ctx context.Context, method string, path string, body any) ([]byte, error) {
	if c == nil || c.baseURL == "" {
		return nil, ErrUnavailable
	}
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(raw)
	}
	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return nil, err
	}
	request.Header.Set("accept", "application/json")
	if body != nil {
		request.Header.Set("content-type", "application/json")
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, &HTTPError{StatusCode: response.StatusCode, Message: fetcherErrorMessage(raw)}
	}
	return raw, nil
}

func fetcherErrorMessage(raw []byte) string {
	var payload struct {
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(raw, &payload); err == nil {
		if payload.Error != nil {
			if message := cleanString(payload.Error.Message); message != "" {
				return message
			}
		}
		if message := cleanString(payload.Message); message != "" {
			return message
		}
	}
	message := strings.TrimSpace(string(raw))
	if message == "" {
		return ""
	}
	if len(message) > 160 {
		return message[:160] + "..."
	}
	return message
}

func cleanString(value string) string {
	return strings.TrimSpace(value)
}

func cleanStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func boolPointerValue(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

func taskList(items []TaskPayload) []TaskPayload {
	if items == nil {
		return []TaskPayload{}
	}
	result := make([]TaskPayload, 0, len(items))
	for _, item := range items {
		if normalized := normalizeTaskPayload(item); len(normalized) > 0 {
			result = append(result, normalized)
		}
	}
	return result
}

func taskPointer(payload *TaskPayload) *TaskPayload {
	if payload == nil || len(*payload) == 0 {
		return nil
	}
	normalized := normalizeTaskPayload(*payload)
	if len(normalized) == 0 {
		return nil
	}
	return &normalized
}

func normalizeTaskPayload(payload TaskPayload) TaskPayload {
	if len(payload) == 0 {
		return nil
	}
	normalized := make(TaskPayload, len(payload)+1)
	for key, value := range payload {
		if _, isAlias := taskPayloadCanonicalKeys[key]; !isAlias {
			normalized[key] = value
		}
	}
	for key, value := range payload {
		if canonical, ok := taskPayloadCanonicalKeys[key]; ok {
			if _, exists := normalized[canonical]; !exists {
				normalized[canonical] = value
			}
		}
	}
	if _, ok := normalized["id"]; !ok {
		if taskID, ok := normalized["taskId"]; ok {
			normalized["id"] = taskID
		}
	}
	return normalized
}
