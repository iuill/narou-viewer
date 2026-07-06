package fetcher

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientReadsStatusQueueSummaryAndMutations(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		switch r.URL.Path {
		case "/api/v2/system/version":
			writeEnvelope(t, w, map[string]any{"current": "novel-fetcher/v1.0.0", "latest": ""})
		case "/api/v2/system/queue":
			writeEnvelope(t, w, map[string]any{"total": "2", "web_worker": 1, "worker": 1})
		case "/api/v2/tasks/summary":
			writeEnvelope(t, w, map[string]any{
				"current": map[string]any{
					"task_id":             "current",
					"type":                "download",
					"status":              "running",
					"novel_title":         "作品タイトル",
					"created_at":          "2026-06-01T00:00:00Z",
					"started_at":          "2026-06-01T00:00:01Z",
					"finished_at":         "2026-06-01T00:00:02Z",
					"total_steps":         10,
					"current_step":        4,
					"saved_episode_count": 3,
					"failed_episode_id":   "5",
					"resume_episode_id":   "6",
				},
				"queued":           []any{map[string]any{"id": "queued"}, "bad"},
				"recent_completed": []any{},
				"recent_failed":    []any{},
				"completed_count":  "3",
				"failed_count":     -1,
				"convert_current":  nil,
				"convert_queued":   []any{},
			})
		case "/api/v2/novels/download":
			writeEnvelope(t, w, map[string]any{"targets": []any{}, "task_ids": []any{"task-1", 2}}, "Download queued")
		case "/api/v2/novels/update":
			writeEnvelope(t, w, map[string]any{"ids": []any{1}, "task_ids": []any{"task-2"}, "skip_unchanged": false}, "Update queued")
		case "/api/v2/novels/resume":
			writeEnvelope(t, w, map[string]any{"ids": []any{1}, "task_ids": []any{"task-3"}}, "Resume queued")
		case "/api/v2/novels/remove":
			writeEnvelope(t, w, map[string]any{"ids": []any{"1"}}, "Novel removed")
		case "/api/v2/tasks/task-1/cancel":
			writeEnvelope(t, w, map[string]any{"task_id": "task-1", "cancelled": true}, "Task cancelled")
		default:
			w.WriteHeader(http.StatusNotFound)
			writeEnvelope(t, w, map[string]any{})
		}
	}))
	defer server.Close()

	client := NewClient(server.URL)
	status, err := client.Status(context.Background())
	if err != nil {
		t.Fatalf("Status returned error: %v", err)
	}
	if status.Version.Current == nil || *status.Version.Current != "novel-fetcher/v1.0.0" || !status.Queue.Running {
		t.Fatalf("unexpected status: %+v", status)
	}
	summary, err := client.TasksSummary(context.Background())
	if err != nil {
		t.Fatalf("TasksSummary returned error: %v", err)
	}
	if len(summary.Queued) != 1 || summary.FailedCount != 0 {
		t.Fatalf("unexpected summary normalization: %+v", summary)
	}
	if summary.Current == nil {
		t.Fatal("current task should be present")
	}
	current := map[string]json.RawMessage(*summary.Current)
	for _, key := range []string{"id", "taskId", "novelTitle", "createdAt", "startedAt", "finishedAt", "totalSteps", "currentStep", "savedEpisodeCount", "failedEpisodeId", "resumeEpisodeId"} {
		if _, ok := current[key]; !ok {
			t.Fatalf("current task should expose canonical key %q: %+v", key, current)
		}
	}
	for _, key := range []string{"task_id", "novel_title", "created_at", "started_at", "finished_at", "total_steps", "current_step", "saved_episode_count", "failed_episode_id", "resume_episode_id"} {
		if _, ok := current[key]; ok {
			t.Fatalf("current task should not expose sidecar snake_case key %q: %+v", key, current)
		}
	}
	download, err := client.Download(context.Background(), []string{"target"}, false, false, false)
	if err != nil {
		t.Fatalf("Download returned error: %v", err)
	}
	if download.Targets[0] != "target" || len(download.TaskIDs) != 2 || download.Message != "Download queued" {
		t.Fatalf("unexpected download response: %+v", download)
	}
	update, err := client.Update(context.Background(), []int{1}, true, false, false, true)
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if update.Message != "Update queued" {
		t.Fatalf("unexpected update response: %+v", update)
	}
	resume, err := client.Resume(context.Background(), []int{1})
	if err != nil {
		t.Fatalf("Resume returned error: %v", err)
	}
	if resume.Message != "Resume queued" {
		t.Fatalf("unexpected resume response: %+v", resume)
	}
	remove, err := client.Remove(context.Background(), []string{"1"}, true)
	if err != nil {
		t.Fatalf("Remove returned error: %v", err)
	}
	if remove.Message != "Novel removed" || len(remove.IDs) != 1 {
		t.Fatalf("unexpected remove response: %+v", remove)
	}
	cancel, err := client.CancelTask(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("CancelTask returned error: %v", err)
	}
	if cancel.Message != "Task cancelled" || !cancel.Cancelled || cancel.TaskID != "task-1" {
		t.Fatalf("unexpected cancel response: %+v", cancel)
	}
}

func TestClientReadsLibraryAPI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		switch r.URL.Path {
		case "/api/v1/works":
			_ = json.NewEncoder(w).Encode(map[string]any{"works": []any{map[string]any{
				"id":                  12,
				"site":                "syosetu",
				"site_work_id":        "n1234ab",
				"title":               "作品",
				"directory":           "works/syosetu/n1234ab",
				"episode_count":       2,
				"saved_episode_count": 1,
			}}})
		case "/api/v1/works/12/toc":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":                  12,
				"site":                "syosetu",
				"site_work_id":        "n1234ab",
				"title":               "作品",
				"directory":           "works/syosetu/n1234ab",
				"episode_count":       2,
				"saved_episode_count": 1,
				"episodes": []any{map[string]any{
					"episode_id":    "1",
					"display_index": "1",
					"title":         "第一話",
					"body_status":   "complete",
				}},
			})
		case "/api/v1/works/tocs":
			if r.URL.Query().Get("ids") != "12,13" {
				t.Fatalf("ids query = %q", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"works": []any{map[string]any{
				"id":                  12,
				"site":                "syosetu",
				"site_work_id":        "n1234ab",
				"title":               "作品",
				"directory":           "works/syosetu/n1234ab",
				"episode_count":       2,
				"saved_episode_count": 1,
				"episodes": []any{map[string]any{
					"episode_id":    "1",
					"display_index": "1",
					"title":         "第一話",
					"body_status":   "complete",
				}},
			}}})
		case "/api/v1/works/12/episodes/1":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"work": map[string]any{"id": 12, "site": "syosetu", "site_work_id": "n1234ab", "title": "作品"},
				"episode": map[string]any{
					"episode_id":    "1",
					"display_index": "1",
					"title":         "第一話",
					"body_status":   "complete",
					"content_hash":  "sha256:test",
				},
				"canonical": map[string]any{
					"schema_version": 1,
					"episode_id":     "1",
					"display_index":  "1",
					"title":          "第一話",
					"blocks":         []any{map[string]any{"type": "html", "section": "body", "html": "<p>本文</p>"}},
				},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL)
	works, err := client.ListLibraryWorks(context.Background())
	if err != nil {
		t.Fatalf("ListLibraryWorks returned error: %v", err)
	}
	if len(works) != 1 || works[0].ID != 12 || works[0].EpisodeLen != 2 {
		t.Fatalf("unexpected works: %+v", works)
	}
	work, episodes, err := client.GetLibraryToc(context.Background(), 12)
	if err != nil {
		t.Fatalf("GetLibraryToc returned error: %v", err)
	}
	if work.ID != 12 || len(episodes) != 1 || episodes[0].EpisodeID != "1" {
		t.Fatalf("unexpected toc: work=%+v episodes=%+v", work, episodes)
	}
	tocs, err := client.ListLibraryTocs(context.Background(), []int{12, 13, 12})
	if err != nil {
		t.Fatalf("ListLibraryTocs returned error: %v", err)
	}
	if len(tocs) != 1 || len(tocs[12]) != 1 || tocs[12][0].EpisodeID != "1" {
		t.Fatalf("unexpected batch tocs: %+v", tocs)
	}
	episode, err := client.GetLibraryEpisode(context.Background(), 12, "1")
	if err != nil {
		t.Fatalf("GetLibraryEpisode returned error: %v", err)
	}
	if episode.Work.ID != 12 || episode.Episode.ContentHash != "sha256:test" || len(episode.Canonical) == 0 {
		t.Fatalf("unexpected episode: %+v", episode)
	}
}

func TestClientReportsFetcherErrors(t *testing.T) {
	if _, err := (*Client)(nil).Queue(context.Background()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("nil client should be unavailable, err=%v", err)
	}
	emptyClient := NewClient("")
	if _, err := emptyClient.Status(context.Background()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("empty base URL status should be unavailable, err=%v", err)
	}
	if _, err := emptyClient.TasksSummary(context.Background()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("empty base URL summary should be unavailable, err=%v", err)
	}
	if _, err := emptyClient.Download(context.Background(), []string{"target"}, false, false, false); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("empty base URL download should be unavailable, err=%v", err)
	}
	if _, err := emptyClient.Update(context.Background(), []int{1}, false, false, false, true); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("empty base URL update should be unavailable, err=%v", err)
	}
	if _, err := emptyClient.Resume(context.Background(), []int{1}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("empty base URL resume should be unavailable, err=%v", err)
	}
	if _, err := emptyClient.Remove(context.Background(), []string{"1"}, false); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("empty base URL remove should be unavailable, err=%v", err)
	}
	if _, err := emptyClient.CancelTask(context.Background(), "task"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("empty base URL cancel should be unavailable, err=%v", err)
	}
	if _, err := NewClient("http://[::1").Queue(context.Background()); err == nil {
		t.Fatal("invalid base URL should fail before sending request")
	}

	badJSON := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte("{"))
	}))
	defer badJSON.Close()
	if _, err := NewClient(badJSON.URL).Queue(context.Background()); err == nil {
		t.Fatal("invalid JSON should fail")
	}

	badEnvelope := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false})
	}))
	defer badEnvelope.Close()
	if _, err := NewClient(badEnvelope.URL).Queue(context.Background()); err == nil {
		t.Fatal("bad envelope should fail")
	}

	badData := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": []any{}})
	}))
	defer badData.Close()
	if _, err := NewClient(badData.URL).Queue(context.Background()); err == nil {
		t.Fatal("bad data payload should fail")
	}

	missingData := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
	}))
	defer missingData.Close()
	if _, err := NewClient(missingData.URL).Queue(context.Background()); err == nil {
		t.Fatal("missing data payload should fail")
	}

	notFound := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false})
	}))
	defer notFound.Close()
	if _, err := NewClient(notFound.URL).Queue(context.Background()); err == nil {
		t.Fatal("HTTP error should fail")
	} else {
		var httpError *HTTPError
		if !errors.As(err, &httpError) || httpError.StatusCode != http.StatusNotFound {
			t.Fatalf("HTTP error should expose status code: err=%v", err)
		}
	}

	textError := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "text/plain")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("fetcher exploded"))
	}))
	defer textError.Close()
	if _, err := NewClient(textError.URL).Queue(context.Background()); err == nil || !strings.Contains(err.Error(), "HTTP 500") || !strings.Contains(err.Error(), "fetcher exploded") {
		t.Fatalf("non-JSON HTTP error should preserve status and snippet, err=%v", err)
	}

	jsonError := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": "upstream down"}})
	}))
	defer jsonError.Close()
	if _, err := NewClient(jsonError.URL).Queue(context.Background()); err == nil || !strings.Contains(err.Error(), "HTTP 502") || !strings.Contains(err.Error(), "upstream down") {
		t.Fatalf("JSON HTTP error should preserve status and message, err=%v", err)
	}

	emptySuccess := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer emptySuccess.Close()
	if _, err := NewClient(emptySuccess.URL).Queue(context.Background()); err == nil {
		t.Fatal("empty successful response should fail JSON decoding")
	}
}

func TestClientStatusReportsComponentErrors(t *testing.T) {
	versionFailure := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		writeEnvelope(t, w, map[string]any{})
	}))
	defer versionFailure.Close()
	if _, err := NewClient(versionFailure.URL).Status(context.Background()); err == nil {
		t.Fatal("Status should fail when version endpoint fails")
	}

	queueFailure := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		if r.URL.Path == "/api/v2/system/version" {
			writeEnvelope(t, w, map[string]any{"current": "v1"})
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		writeEnvelope(t, w, map[string]any{})
	}))
	defer queueFailure.Close()
	if _, err := NewClient(queueFailure.URL).Status(context.Background()); err == nil {
		t.Fatal("Status should fail when queue endpoint fails")
	}

	tasksFailure := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		switch r.URL.Path {
		case "/api/v2/system/version":
			writeEnvelope(t, w, map[string]any{"current": "v1"})
		case "/api/v2/system/queue":
			writeEnvelope(t, w, map[string]any{})
		default:
			w.WriteHeader(http.StatusInternalServerError)
			writeEnvelope(t, w, map[string]any{})
		}
	}))
	defer tasksFailure.Close()
	if _, err := NewClient(tasksFailure.URL).Status(context.Background()); err == nil {
		t.Fatal("Status should fail when tasks endpoint fails")
	}
}

func TestClientNormalizationHelpers(t *testing.T) {
	if cleanString(" value ") != "value" || cleanString(" ") != "" {
		t.Fatal("cleanString returned unexpected values")
	}
	if fetcherErrorMessage([]byte(`{"message":" top level "}`)) != "top level" {
		t.Fatal("fetcherErrorMessage should read a top-level message")
	}
	if fetcherErrorMessage([]byte(`{"error":{"message":" nested "}}`)) != "nested" {
		t.Fatal("fetcherErrorMessage should prefer nested error messages")
	}
	if got := fetcherErrorMessage([]byte(strings.Repeat("x", 200))); len(got) != 163 || !strings.HasSuffix(got, "...") {
		t.Fatalf("fetcherErrorMessage should truncate long text payloads: %q", got)
	}
	if fetcherErrorMessage([]byte(" ")) != "" {
		t.Fatal("fetcherErrorMessage should return an empty string for blank payloads")
	}
	var intValue IntValue
	if err := intValue.UnmarshalJSON([]byte(`"42"`)); err != nil || intValue != 42 {
		t.Fatalf("IntValue should parse numeric strings: value=%d err=%v", intValue, err)
	}
	if got := boolPointerValue(nil, true); !got {
		t.Fatal("boolPointerValue should use fallback")
	}
	var payload TaskPayload
	if err := payload.UnmarshalJSON([]byte(`"bad"`)); err != nil || payload != nil {
		t.Fatalf("TaskPayload should ignore non-object payloads: payload=%+v err=%v", payload, err)
	}
	var list StringList
	if err := list.UnmarshalJSON([]byte(`[" a ",2,-1," "]`)); err != nil || len(list) != 2 || list[0] != "a" || list[1] != "2" {
		t.Fatalf("unexpected StringList result: list=%+v err=%v", list, err)
	}
	if err := list.UnmarshalJSON([]byte(`{}`)); err == nil {
		t.Fatal("StringList should reject non-array payloads")
	}
	var nilPayload *TaskPayload
	if taskPointer(nilPayload) != nil {
		t.Fatal("taskPointer should ignore nil payloads")
	}
	emptyPayload := TaskPayload{}
	if taskPointer(&emptyPayload) != nil {
		t.Fatal("taskPointer should ignore empty payloads")
	}
	nonEmptyPayload := TaskPayload{"id": json.RawMessage(`"task"`)}
	if taskPointer(&nonEmptyPayload) == nil {
		t.Fatal("taskPointer should keep non-empty payloads")
	}
	collidingPayload := normalizeTaskPayload(TaskPayload{
		"task_id": json.RawMessage(`"snake-task"`),
		"taskId":  json.RawMessage(`"canonical-task"`),
	})
	if string(collidingPayload["taskId"]) != `"canonical-task"` || string(collidingPayload["id"]) != `"canonical-task"` {
		t.Fatalf("canonical task payload keys should win over snake_case aliases: %+v", collidingPayload)
	}
}

func writeEnvelope(t *testing.T, w http.ResponseWriter, data map[string]any, message ...string) {
	t.Helper()
	envelope := map[string]any{"success": true, "data": data}
	if len(message) > 0 {
		envelope["message"] = message[0]
	}
	if err := json.NewEncoder(w).Encode(envelope); err != nil {
		t.Fatalf("write envelope: %v", err)
	}
}
