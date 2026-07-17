package server

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"narou-viewer/services/novel-fetcher/internal/config"
	"narou-viewer/services/novel-fetcher/internal/fetcher"
	"narou-viewer/services/novel-fetcher/internal/model"
	"narou-viewer/services/novel-fetcher/internal/sites"
	"narou-viewer/services/novel-fetcher/internal/storage"
	"narou-viewer/services/novel-fetcher/internal/taskqueue"
	"narou-viewer/services/novel-fetcher/internal/taskstate"
)

type progressFetcher struct {
	reported   chan struct{}
	release    chan struct{}
	reportOnce sync.Once
}

type blockingTocFetcher struct {
	entered   chan struct{}
	release   chan struct{}
	enterOnce sync.Once
	work      model.Work
}

type staticFetcher struct {
	work model.Work
	err  error
}

type partialFailFetcher struct{}

type countingFetcher struct {
	mu             sync.Mutex
	episodeHits    []string
	body           string
	publishedAt    string
	modifiedAt     string
	omitModifiedAt bool
}

type cancelingAssetFetcher struct{}

type blockingAssetFetcher struct {
	entered chan struct{}
	once    sync.Once
}

func (f *blockingTocFetcher) FetchToc(ctx context.Context, target string, report sites.ProgressReporter) (model.Work, error) {
	f.enterOnce.Do(func() {
		close(f.entered)
	})
	if report != nil {
		report(sites.Progress{Phase: "toc", Message: "waiting"})
	}
	select {
	case <-ctx.Done():
		return model.Work{}, ctx.Err()
	case <-f.release:
		work := f.work
		work.SourceURL = target
		return work, nil
	}
}

func (f *blockingTocFetcher) FetchEpisode(_ context.Context, _ model.Work, episode model.Episode, _ sites.ProgressReporter) (model.Episode, error) {
	if episode.Element.Body == "" {
		episode.Element = model.EpisodeElement{DataType: "html", Body: "<p>本文</p>"}
	}
	episode.RawHTML = "<p>本文</p>"
	episode.FetchedAt = time.Now()
	return episode, nil
}

func (f staticFetcher) FetchToc(_ context.Context, target string, report sites.ProgressReporter) (model.Work, error) {
	if f.err != nil {
		return model.Work{}, f.err
	}
	if report != nil {
		report(sites.Progress{Phase: "toc", Message: "ok"})
	}
	work := f.work
	work.SourceURL = target
	return work, nil
}

func (f staticFetcher) FetchEpisode(_ context.Context, _ model.Work, episode model.Episode, _ sites.ProgressReporter) (model.Episode, error) {
	if f.err != nil {
		return model.Episode{}, f.err
	}
	if episode.Element.Body == "" {
		episode.Element = model.EpisodeElement{DataType: "html", Body: "<p>本文</p>"}
	}
	episode.RawHTML = "<p>本文</p>"
	episode.FetchedAt = time.Now()
	return episode, nil
}

func (f partialFailFetcher) FetchToc(_ context.Context, target string, _ sites.ProgressReporter) (model.Work, error) {
	return model.Work{
		Site:       model.SiteSyosetu,
		SiteName:   "小説家になろう",
		SiteWorkID: "n9999aa",
		SourceURL:  target,
		Title:      "途中保存作品",
		Author:     "作者",
		FetchedAt:  time.Now(),
		Episodes: []model.Episode{
			{Index: "1", Href: "/n9999aa/1/", Title: "第一話", Element: model.EpisodeElement{DataType: "html"}},
			{Index: "2", Href: "/n9999aa/2/", Title: "第二話", Element: model.EpisodeElement{DataType: "html"}},
		},
	}, nil
}

func (f partialFailFetcher) FetchEpisode(_ context.Context, _ model.Work, episode model.Episode, _ sites.ProgressReporter) (model.Episode, error) {
	if episode.Index == "2" {
		return model.Episode{}, errors.New("episode 2 failed")
	}
	episode.RawHTML = "<p>一本文</p>"
	episode.FetchedAt = time.Now()
	episode.Element = model.EpisodeElement{DataType: "html", Body: "<p>一本文</p>"}
	return episode, nil
}

func (f *countingFetcher) FetchToc(_ context.Context, target string, _ sites.ProgressReporter) (model.Work, error) {
	episode := model.Episode{
		Index:       "1",
		Href:        "/n1234ab/1/",
		Title:       "第一話",
		PublishedAt: firstNonEmpty(f.publishedAt, "2026/05/09 12:00"),
		Element: model.EpisodeElement{
			DataType: "html",
		},
	}
	if !f.omitModifiedAt {
		episode.ModifiedAt = firstNonEmpty(f.modifiedAt, "2026/05/09 12:00")
	}
	return model.Work{
		Site:       model.SiteSyosetu,
		SiteName:   "小説家になろう",
		SiteWorkID: "n1234ab",
		SourceURL:  target,
		Title:      "保存作品",
		Author:     "作者",
		FetchedAt:  time.Now(),
		Episodes:   []model.Episode{episode},
	}, nil
}

func (f *countingFetcher) FetchEpisode(_ context.Context, _ model.Work, episode model.Episode, _ sites.ProgressReporter) (model.Episode, error) {
	f.mu.Lock()
	f.episodeHits = append(f.episodeHits, episode.Index)
	f.mu.Unlock()

	body := f.body
	if body == "" {
		body = "<p>更新本文</p>"
	}
	episode.RawHTML = body
	episode.FetchedAt = time.Now()
	episode.Element = model.EpisodeElement{DataType: "html", Body: body}
	return episode, nil
}

func (f *countingFetcher) hitCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.episodeHits)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func (f cancelingAssetFetcher) FetchBytes(_ context.Context, _ string, _ fetcher.FetchPolicy) (fetcher.BinaryResponse, error) {
	return fetcher.BinaryResponse{}, context.Canceled
}

func (f *blockingAssetFetcher) FetchBytes(ctx context.Context, _ string, _ fetcher.FetchPolicy) (fetcher.BinaryResponse, error) {
	f.once.Do(func() { close(f.entered) })
	<-ctx.Done()
	return fetcher.BinaryResponse{}, ctx.Err()
}

func TestReadEndpointsReturnStoredWorkAndEpisodes(t *testing.T) {
	store, stored := newTestStoreWithWork(t)
	app := New(Options{
		Config:  config.Config{},
		Store:   store,
		Fetcher: staticFetcher{},
		Logger:  slog.Default(),
	})
	startApp(t, app)
	defer store.Close()
	defer app.Shutdown(context.Background())

	tests := []struct {
		method string
		path   string
		status int
		check  func(t *testing.T, payload map[string]any)
	}{
		{
			method: http.MethodGet,
			path:   "/health",
			status: http.StatusOK,
			check: func(t *testing.T, payload map[string]any) {
				t.Helper()
				if payload["status"] != "ok" {
					t.Fatalf("health payload = %#v", payload)
				}
			},
		},
		{
			method: http.MethodGet,
			path:   "/api/v2/system/version",
			status: http.StatusOK,
			check: func(t *testing.T, payload map[string]any) {
				t.Helper()
				data := payload["data"].(map[string]any)
				if data["current"] == "" {
					t.Fatalf("version payload = %#v", payload)
				}
				if data["latest"] == "" {
					t.Fatalf("version payload = %#v", payload)
				}
			},
		},
		{
			method: http.MethodGet,
			path:   "/api/v2/system/queue",
			status: http.StatusOK,
			check: func(t *testing.T, payload map[string]any) {
				t.Helper()
				data := payload["data"].(map[string]any)
				if data["total"] != float64(0) || data["running"] != false {
					t.Fatalf("queue payload = %#v", payload)
				}
			},
		},
		{
			method: http.MethodGet,
			path:   "/api/v2/novels",
			status: http.StatusOK,
			check: func(t *testing.T, payload map[string]any) {
				t.Helper()
				data := payload["data"].(map[string]any)
				novels := data["novels"].([]any)
				if len(novels) != 1 || novels[0].(map[string]any)["title"] != "保存作品" {
					t.Fatalf("novels payload = %#v", payload)
				}
			},
		},
		{
			method: http.MethodGet,
			path:   "/api/v1/works",
			status: http.StatusOK,
			check: func(t *testing.T, payload map[string]any) {
				t.Helper()
				works := payload["works"].([]any)
				if len(works) != 1 || works[0].(map[string]any)["episode_count"] != float64(1) {
					t.Fatalf("works payload = %#v", payload)
				}
			},
		},
		{
			method: http.MethodGet,
			path:   "/api/v1/works/" + storedID(stored.ID),
			status: http.StatusOK,
			check: func(t *testing.T, payload map[string]any) {
				t.Helper()
				if payload["title"] != "保存作品" {
					t.Fatalf("work payload = %#v", payload)
				}
			},
		},
		{
			method: http.MethodGet,
			path:   "/api/v1/works/" + storedID(stored.ID) + "/toc",
			status: http.StatusOK,
			check: func(t *testing.T, payload map[string]any) {
				t.Helper()
				episodes := payload["episodes"].([]any)
				if len(episodes) != 1 || episodes[0].(map[string]any)["episode_id"] != "1" {
					t.Fatalf("toc payload = %#v", payload)
				}
				if episodes[0].(map[string]any)["source_url"] != "https://ncode.syosetu.com/n1234ab/1/" {
					t.Fatalf("episode source_url = %#v", episodes[0])
				}
			},
		},
		{
			method: http.MethodGet,
			path:   "/api/v1/works/tocs?ids=" + storedID(stored.ID) + ",999," + storedID(stored.ID),
			status: http.StatusOK,
			check: func(t *testing.T, payload map[string]any) {
				t.Helper()
				works := payload["works"].([]any)
				if len(works) != 1 {
					t.Fatalf("batch toc payload should omit missing and duplicate works: %#v", payload)
				}
				work := works[0].(map[string]any)
				episodes := work["episodes"].([]any)
				if work["id"] != float64(stored.ID) || len(episodes) != 1 || episodes[0].(map[string]any)["episode_id"] != "1" {
					t.Fatalf("batch toc payload = %#v", payload)
				}
			},
		},
		{
			method: http.MethodGet,
			path:   "/api/v1/works/" + storedID(stored.ID) + "/episodes/1",
			status: http.StatusOK,
			check: func(t *testing.T, payload map[string]any) {
				t.Helper()
				canonical := payload["canonical"].(map[string]any)
				if canonical["title"] != "第一話" {
					t.Fatalf("episode payload = %#v", payload)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			recorder := performRequest(app, test.method, test.path, "")
			if recorder.Code != test.status {
				t.Fatalf("status = %d, want %d: %s", recorder.Code, test.status, recorder.Body.String())
			}
			payload := decodeObject(t, recorder)
			test.check(t, payload)
		})
	}
}

func TestReadEndpointsReturnValidationErrors(t *testing.T) {
	store, _ := newTestStoreWithWork(t)
	app := New(Options{Config: config.Config{}, Store: store, Fetcher: staticFetcher{}, Logger: slog.Default()})
	startApp(t, app)
	defer store.Close()
	defer app.Shutdown(context.Background())

	for _, test := range []struct {
		path   string
		status int
	}{
		{"/api/v1/works/not-number", http.StatusBadRequest},
		{"/api/v1/works/999", http.StatusNotFound},
		{"/api/v1/works/tocs?ids=bad", http.StatusBadRequest},
		{"/api/v1/works/1/episodes/999", http.StatusNotFound},
	} {
		recorder := performRequest(app, http.MethodGet, test.path, "")
		if recorder.Code != test.status {
			t.Fatalf("%s status = %d, want %d: %s", test.path, recorder.Code, test.status, recorder.Body.String())
		}
		payload := decodeObject(t, recorder)
		if payload["success"] != false {
			t.Fatalf("error payload = %#v", payload)
		}
	}
}

func TestEpisodeReadDoesNotReturnFutureCanonicalSchema(t *testing.T) {
	rootDir := t.TempDir()
	store, err := storage.NewStore(rootDir)
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	stored, err := saveWorkFully(t, store, model.Work{
		Site:       model.SiteVerification,
		SiteName:   "Verification",
		SiteWorkID: "future-schema-api",
		SourceURL:  "https://example.invalid/future-schema-api/",
		Title:      "Synthetic future schema API work",
		Author:     "Synthetic author",
		FetchedAt:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Episodes: []model.Episode{{
			Index:     "1",
			Title:     "Synthetic episode",
			FetchedAt: time.Date(2026, 1, 1, 0, 1, 0, 0, time.UTC),
			Element:   model.EpisodeElement{DataType: "html", Body: "<p>Synthetic body.</p>"},
		}},
	})
	if err != nil {
		_ = store.Close()
		t.Fatalf("saveWorkFully returned error: %v", err)
	}
	episode, found, err := store.FindEpisode(stored.ID, "1")
	if err != nil || !found {
		_ = store.Close()
		t.Fatalf("FindEpisode = %#v/%v/%v", episode, found, err)
	}
	const futureDocument = `{"schema_version":99,"episode_id":"synthetic-future","future_body":"must-not-be-returned"}`
	if err := os.WriteFile(filepath.Join(rootDir, episode.BodyPath), []byte(futureDocument), 0o644); err != nil {
		_ = store.Close()
		t.Fatalf("seed future canonical episode: %v", err)
	}

	app := New(Options{Config: config.Config{}, Store: store, Fetcher: staticFetcher{}, Logger: slog.Default()})
	startApp(t, app)
	defer store.Close()
	defer app.Shutdown(context.Background())

	recorder := performRequest(app, http.MethodGet, "/api/v1/works/"+storedID(stored.ID)+"/episodes/1", "")
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusInternalServerError, recorder.Body.String())
	}
	payload := decodeObject(t, recorder)
	if payload["success"] != false || payload["canonical"] != nil {
		t.Fatalf("error payload = %#v", payload)
	}
	if strings.Contains(recorder.Body.String(), "must-not-be-returned") {
		t.Fatalf("future canonical content leaked in response: %s", recorder.Body.String())
	}
}

func TestMutationEndpointsValidateBodiesAndQueueTasks(t *testing.T) {
	store, stored := newTestStoreWithWork(t)
	app := New(Options{
		Config: config.Config{},
		Store:  store,
		Fetcher: staticFetcher{work: model.Work{
			Site:       model.SiteSyosetu,
			SiteName:   "小説家になろう",
			SiteWorkID: "n9999aa",
			Title:      "取得作品",
			Author:     "作者",
			FetchedAt:  time.Now(),
			Episodes: []model.Episode{{
				Index:     "1",
				Title:     "第一話",
				FetchedAt: time.Now(),
				Element:   model.EpisodeElement{DataType: "html", Body: "<p>本文</p>"},
			}},
		}},
		Logger: slog.Default(),
	})
	startApp(t, app)
	defer store.Close()
	defer app.Shutdown(context.Background())

	badDownload := performRequest(app, http.MethodPost, "/api/v2/novels/download", `{"targets":["  "]}`)
	if badDownload.Code != http.StatusBadRequest {
		t.Fatalf("bad download status = %d", badDownload.Code)
	}
	invalidDownload := performRequest(app, http.MethodPost, "/api/v2/novels/download", `{`)
	if invalidDownload.Code != http.StatusBadRequest {
		t.Fatalf("invalid download status = %d", invalidDownload.Code)
	}

	unsupportedDownloadConvert := performRequest(app, http.MethodPost, "/api/v2/novels/download", `{"targets":["https://ncode.syosetu.com/n9999aa/"],"convert_after_download":true}`)
	if unsupportedDownloadConvert.Code != http.StatusNotImplemented {
		t.Fatalf("unsupported download convert status = %d: %s", unsupportedDownloadConvert.Code, unsupportedDownloadConvert.Body.String())
	}
	unsupportedDownloadMail := performRequest(app, http.MethodPost, "/api/v2/novels/download", `{"targets":["https://ncode.syosetu.com/n9999aa/"],"mail":true}`)
	if unsupportedDownloadMail.Code != http.StatusNotImplemented {
		t.Fatalf("unsupported download mail status = %d: %s", unsupportedDownloadMail.Code, unsupportedDownloadMail.Body.String())
	}

	download := performRequest(app, http.MethodPost, "/api/v2/novels/download", `{"targets":[" https://ncode.syosetu.com/n9999aa/ "],"force":true}`)
	if download.Code != http.StatusAccepted {
		t.Fatalf("download status = %d: %s", download.Code, download.Body.String())
	}
	downloadPayload := decodeObject(t, download)
	downloadData := downloadPayload["data"].(map[string]any)
	if len(downloadData["task_ids"].([]any)) != 1 {
		t.Fatalf("download payload = %#v", downloadPayload)
	}
	if downloadData["force"] != true || downloadData["convert_after_download"] != false || downloadData["mail"] != false {
		t.Fatalf("download options were not reported: %#v", downloadData)
	}
	waitForIdleApp(t, app)

	badUpdate := performRequest(app, http.MethodPost, "/api/v2/novels/update", `{"ids":[]}`)
	if badUpdate.Code != http.StatusBadRequest {
		t.Fatalf("bad update status = %d", badUpdate.Code)
	}
	invalidUpdate := performRequest(app, http.MethodPost, "/api/v2/novels/update", `{`)
	if invalidUpdate.Code != http.StatusBadRequest {
		t.Fatalf("invalid update status = %d", invalidUpdate.Code)
	}
	missingUpdate := performRequest(app, http.MethodPost, "/api/v2/novels/update", `{"ids":[999]}`)
	if missingUpdate.Code != http.StatusNotFound {
		t.Fatalf("missing update status = %d", missingUpdate.Code)
	}
	unsupportedUpdateFrozen := performRequest(app, http.MethodPost, "/api/v2/novels/update", `{"ids":[`+storedID(stored.ID)+`],"include_frozen":true}`)
	if unsupportedUpdateFrozen.Code != http.StatusNotImplemented {
		t.Fatalf("unsupported update frozen status = %d: %s", unsupportedUpdateFrozen.Code, unsupportedUpdateFrozen.Body.String())
	}
	unsupportedUpdateConvert := performRequest(app, http.MethodPost, "/api/v2/novels/update", `{"ids":[`+storedID(stored.ID)+`],"convert_after_update":true}`)
	if unsupportedUpdateConvert.Code != http.StatusNotImplemented {
		t.Fatalf("unsupported update convert status = %d: %s", unsupportedUpdateConvert.Code, unsupportedUpdateConvert.Body.String())
	}

	update := performRequest(app, http.MethodPost, "/api/v2/novels/update", `{"ids":[`+storedID(stored.ID)+`],"force_redownload":true,"skip_unchanged":false}`)
	if update.Code != http.StatusAccepted {
		t.Fatalf("update status = %d: %s", update.Code, update.Body.String())
	}
	updatePayload := decodeObject(t, update)
	updateData := updatePayload["data"].(map[string]any)
	if updateData["force_redownload"] != true || updateData["include_frozen"] != false || updateData["convert_after_update"] != false || updateData["skip_unchanged"] != false {
		t.Fatalf("update options were not reported: %#v", updateData)
	}
	waitForIdleApp(t, app)

	invalidResume := performRequest(app, http.MethodPost, "/api/v2/novels/resume", `{`)
	if invalidResume.Code != http.StatusBadRequest {
		t.Fatalf("invalid resume status = %d", invalidResume.Code)
	}
	emptyResume := performRequest(app, http.MethodPost, "/api/v2/novels/resume", `{"ids":[]}`)
	if emptyResume.Code != http.StatusBadRequest {
		t.Fatalf("empty resume status = %d", emptyResume.Code)
	}
	missingResume := performRequest(app, http.MethodPost, "/api/v2/novels/resume", `{"ids":[999]}`)
	if missingResume.Code != http.StatusNotFound {
		t.Fatalf("missing resume status = %d", missingResume.Code)
	}
	resume := performRequest(app, http.MethodPost, "/api/v2/novels/resume", `{"ids":[`+storedID(stored.ID)+`]}`)
	if resume.Code != http.StatusAccepted {
		t.Fatalf("resume status = %d: %s", resume.Code, resume.Body.String())
	}
	waitForIdleApp(t, app)

	invalidRemove := performRequest(app, http.MethodPost, "/api/v2/novels/remove", `{`)
	if invalidRemove.Code != http.StatusBadRequest {
		t.Fatalf("invalid remove status = %d", invalidRemove.Code)
	}
	removeEmpty := performRequest(app, http.MethodPost, "/api/v2/novels/remove", `{"ids":[],"with_files":false}`)
	if removeEmpty.Code != http.StatusBadRequest {
		t.Fatalf("remove empty status = %d", removeEmpty.Code)
	}
	removeBadID := performRequest(app, http.MethodPost, "/api/v2/novels/remove", `{"ids":["abc"],"with_files":false}`)
	if removeBadID.Code != http.StatusBadRequest {
		t.Fatalf("remove bad id status = %d", removeBadID.Code)
	}
	removeMissing := performRequest(app, http.MethodPost, "/api/v2/novels/remove", `{"ids":["999"],"with_files":false}`)
	if removeMissing.Code != http.StatusNotFound {
		t.Fatalf("remove missing status = %d", removeMissing.Code)
	}
}

func TestDownloadEquivalentTargetsAreDeduplicatedBeforeFetch(t *testing.T) {
	store, stored := newTestStoreWithWork(t)
	fetcher := &blockingTocFetcher{
		entered: make(chan struct{}),
		release: make(chan struct{}),
		work: model.Work{
			Site:       model.SiteSyosetu,
			SiteName:   "小説家になろう",
			SiteWorkID: "n1234ab",
			Title:      "保存作品",
			Author:     "作者",
			FetchedAt:  time.Now(),
			Episodes: []model.Episode{{
				Index:     "1",
				Href:      "/n1234ab/1/",
				Title:     "第一話",
				FetchedAt: time.Now(),
				Element:   model.EpisodeElement{DataType: "html", Body: "<p>本文</p>"},
			}},
		},
	}
	app := New(Options{Config: config.Config{}, Store: store, Fetcher: fetcher, Logger: slog.Default()})
	startApp(t, app)
	releaseOnce := sync.Once{}
	t.Cleanup(func() {
		releaseOnce.Do(func() {
			close(fetcher.release)
		})
		app.Shutdown(context.Background())
		store.Close()
	})

	download := performRequest(app, http.MethodPost, "/api/v2/novels/download", `{"targets":[" n1234ab ","https://ncode.syosetu.com/n1234ab/1/"],"force":true}`)
	if download.Code != http.StatusAccepted {
		t.Fatalf("download status = %d: %s", download.Code, download.Body.String())
	}
	downloadData := decodeObject(t, download)["data"].(map[string]any)
	if targets := downloadData["targets"].([]any); len(targets) != 1 {
		t.Fatalf("download targets = %#v, want one canonical work request", targets)
	}
	if taskIDs := downloadData["task_ids"].([]any); len(taskIDs) != 1 {
		t.Fatalf("download task_ids = %#v, want one task", taskIDs)
	}

	select {
	case <-fetcher.entered:
	case <-time.After(time.Second):
		t.Fatal("download task did not start")
	}

	summary := performRequest(app, http.MethodGet, "/api/v2/tasks/summary", "")
	if summary.Code != http.StatusOK {
		t.Fatalf("summary status = %d: %s", summary.Code, summary.Body.String())
	}
	summaryData := decodeObject(t, summary)["data"].(map[string]any)
	current := summaryData["current"].(map[string]any)
	novelIDs := current["novel_ids"].([]any)
	if !containsStringValue(novelIDs, storedID(stored.ID)) {
		t.Fatalf("download task novel_ids = %#v, want %s", novelIDs, storedID(stored.ID))
	}
	queued := summaryData["queued"].([]any)
	if len(queued) != 0 {
		t.Fatalf("queued tasks = %#v, want equivalent target deduplication", queued)
	}

	releaseOnce.Do(func() {
		close(fetcher.release)
	})
	waitForIdleApp(t, app)
}

func TestExistingDownloadNovelIDsByTargetNormalizesTargets(t *testing.T) {
	emptyApp := &App{}
	emptyMatches, err := emptyApp.existingDownloadNovelIDsByTarget([]string{"  "})
	if err != nil {
		t.Fatalf("empty existingDownloadNovelIDsByTarget returned error: %v", err)
	}
	if len(emptyMatches) != 0 {
		t.Fatalf("empty matches = %#v", emptyMatches)
	}

	store, stored := newTestStoreWithWork(t)
	defer store.Close()
	app := &App{store: store}

	matches, err := app.existingDownloadNovelIDsByTarget([]string{
		"HTTPS://NCODE.SYOSETU.COM/N1234AB",
		"n1234ab",
		"https://ncode.syosetu.com/n1234ab/1/",
		"https://ncode.syosetu.com/n1234ab/",
		"https://ncode.syosetu.com/missing/",
	})
	if err != nil {
		t.Fatalf("existingDownloadNovelIDsByTarget returned error: %v", err)
	}
	ids := matches["site:syosetu:n1234ab"]
	if len(ids) != 1 || ids[0] != stored.ID {
		t.Fatalf("matched ids = %#v, want [%d]", ids, stored.ID)
	}
	if _, ok := matches["url:https://ncode.syosetu.com/missing"]; ok {
		t.Fatalf("missing target unexpectedly matched: %#v", matches)
	}
}

func TestNewDoesNotStartWorkerUntilStart(t *testing.T) {
	store, _ := newTestStoreWithWork(t)
	fetcher := &blockingTocFetcher{
		entered: make(chan struct{}),
		release: make(chan struct{}),
		work: model.Work{
			Site:       model.SiteSyosetu,
			SiteName:   "小説家になろう",
			SiteWorkID: "n1234ab",
			Title:      "保存作品",
			Author:     "作者",
			FetchedAt:  time.Now(),
			Episodes: []model.Episode{{
				Index:   "1",
				Href:    "/n1234ab/1/",
				Title:   "第一話",
				Element: model.EpisodeElement{DataType: "html", Body: "<p>本文</p>"},
			}},
		},
	}
	app := New(Options{Config: config.Config{}, Store: store, Fetcher: fetcher, Logger: slog.Default()})
	t.Cleanup(func() {
		close(fetcher.release)
		app.Shutdown(context.Background())
		store.Close()
	})

	download := performRequest(app, http.MethodPost, "/api/v2/novels/download", `{"targets":["https://ncode.syosetu.com/n1234ab/"]}`)
	if download.Code != http.StatusAccepted {
		t.Fatalf("download status = %d: %s", download.Code, download.Body.String())
	}

	select {
	case <-fetcher.entered:
		t.Fatal("worker started before Start was called")
	case <-time.After(50 * time.Millisecond):
	}

	app.Start(context.Background())
	select {
	case <-fetcher.entered:
	case <-time.After(time.Second):
		t.Fatal("worker did not start after Start was called")
	}
}

func TestCancelQueuedTask(t *testing.T) {
	store, _ := newTestStoreWithWork(t)
	app := New(Options{Config: config.Config{WorkInterval: time.Hour}, Store: store, Fetcher: staticFetcher{}, Logger: slog.Default()})
	defer store.Close()
	defer app.Shutdown(context.Background())

	task := taskqueue.NewTask("download")
	task.ID = "queued-1"
	task.Targets = []string{"https://example.com/work"}
	app.queue.Enqueue(task)

	recorder := performRequest(app, http.MethodPost, "/api/v2/tasks/queued-1/cancel", "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("cancel status = %d: %s", recorder.Code, recorder.Body.String())
	}
	payload := decodeObject(t, recorder)
	if payload["success"] != true {
		t.Fatalf("cancel payload = %#v", payload)
	}
	data := payload["data"].(map[string]any)
	if data["changed"] != true || data["cancelled"] != true || data["status"] != string(taskqueue.StatusCanceled) {
		t.Fatalf("cancel data = %#v", data)
	}

	missing := performRequest(app, http.MethodPost, "/api/v2/tasks/missing/cancel", "")
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing cancel status = %d", missing.Code)
	}
}

func TestPersistentTaskControlEndpointsExposeDurableState(t *testing.T) {
	store, _ := newTestStoreWithWork(t)
	app := New(Options{Config: config.Config{}, Store: store, Fetcher: staticFetcher{}, Logger: slog.Default()})
	defer store.Close()
	defer app.Shutdown(context.Background())

	created := performRequest(app, http.MethodPost, "/api/v2/novels/download", `{"targets":["https://example.com/control"]}`)
	if created.Code != http.StatusAccepted {
		t.Fatalf("create status = %d: %s", created.Code, created.Body.String())
	}
	createdData := decodeObject(t, created)["data"].(map[string]any)
	taskID := createdData["task_ids"].([]any)[0].(string)
	conflicting := performRequest(app, http.MethodPost, "/api/v2/novels/download", `{"targets":["https://example.com/control"],"force":true}`)
	if conflicting.Code != http.StatusConflict {
		t.Fatalf("conflicting create status = %d: %s", conflicting.Code, conflicting.Body.String())
	}
	detail := performRequest(app, http.MethodGet, "/api/v2/tasks/"+taskID, "")
	if detail.Code != http.StatusOK {
		t.Fatalf("detail status = %d: %s", detail.Code, detail.Body.String())
	}
	paused := performRequest(app, http.MethodPost, "/api/v2/tasks/"+taskID+"/pause", "")
	if paused.Code != http.StatusOK {
		t.Fatalf("pause status = %d: %s", paused.Code, paused.Body.String())
	}
	pausedData := decodeObject(t, paused)["data"].(map[string]any)
	if pausedData["status"] != string(taskqueue.StatusPaused) || pausedData["can_resume"] != true || pausedData["changed"] != true {
		t.Fatalf("pause data = %#v", pausedData)
	}
	pausedAgain := performRequest(app, http.MethodPost, "/api/v2/tasks/"+taskID+"/pause", "")
	if pausedAgain.Code != http.StatusOK {
		t.Fatalf("idempotent pause status = %d: %s", pausedAgain.Code, pausedAgain.Body.String())
	}
	resumed := performRequest(app, http.MethodPost, "/api/v2/tasks/"+taskID+"/resume", "")
	if resumed.Code != http.StatusAccepted {
		t.Fatalf("resume status = %d: %s", resumed.Code, resumed.Body.String())
	}
	resumedData := decodeObject(t, resumed)["data"].(map[string]any)
	if resumedData["status"] != string(taskqueue.StatusQueued) || resumedData["changed"] != true {
		t.Fatalf("resume data = %#v", resumedData)
	}
	canceled := performRequest(app, http.MethodPost, "/api/v2/tasks/"+taskID+"/cancel", "")
	if canceled.Code != http.StatusOK {
		t.Fatalf("cancel status = %d: %s", canceled.Code, canceled.Body.String())
	}
	canceledData := decodeObject(t, canceled)["data"].(map[string]any)
	if canceledData["status"] != string(taskqueue.StatusCanceled) || canceledData["changed"] != true || canceledData["cancelled"] != true {
		t.Fatalf("cancel data = %#v", canceledData)
	}
	canceledAgain := performRequest(app, http.MethodPost, "/api/v2/tasks/"+taskID+"/cancel", "")
	if canceledAgain.Code != http.StatusConflict {
		t.Fatalf("repeated cancel status = %d: %s", canceledAgain.Code, canceledAgain.Body.String())
	}
	missing := performRequest(app, http.MethodGet, "/api/v2/tasks/missing", "")
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing detail status = %d", missing.Code)
	}
	for _, action := range []string{"pause", "resume", "cancel"} {
		missingAction := performRequest(app, http.MethodPost, "/api/v2/tasks/missing/"+action, "")
		if missingAction.Code != http.StatusNotFound {
			t.Fatalf("missing %s status = %d", action, missingAction.Code)
		}
	}
}

func TestNewWithErrorRequiresStorage(t *testing.T) {
	app := New(Options{Logger: slog.Default()})
	app.Start(context.Background())
	app.Shutdown(context.Background())
	if app == nil || app.initErr == nil {
		t.Fatal("missing storage did not produce initialization error")
	}
}

func TestNewRecordsTaskStateInitializationError(t *testing.T) {
	store, _ := newTestStoreWithWork(t)
	defer store.Close()
	repositoryTask := taskqueue.NewTask("download")
	repositoryTask.Targets = []string{"https://example.com/corrupt-startup"}
	if _, err := taskstate.NewSQLiteRepository(store.DB()).Enqueue(context.Background(), []*taskqueue.Task{repositoryTask}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB().Exec(`DELETE FROM fetch_task_queue`); err != nil {
		t.Fatal(err)
	}
	app := New(Options{Store: store, Logger: slog.Default()})
	if app == nil || app.initErr == nil {
		t.Fatal("corrupt task state did not produce initialization error")
	}
}

func TestTaskStateReadEndpointsSurfaceStorageErrors(t *testing.T) {
	store, _ := newTestStoreWithWork(t)
	app := New(Options{Store: store, Logger: slog.Default()})
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/v2/system/queue", "/api/v2/tasks/summary"} {
		response := performRequest(app, http.MethodGet, path, "")
		if response.Code != http.StatusInternalServerError {
			t.Fatalf("%s status = %d: %s", path, response.Code, response.Body.String())
		}
	}
}

func TestRunningTaskControlAcknowledgesCancellationRequest(t *testing.T) {
	store, _ := newTestStoreWithWork(t)
	fetcher := &blockingTocFetcher{
		entered: make(chan struct{}),
		release: make(chan struct{}),
		work: model.Work{
			Site: model.SiteSyosetu, SiteName: "小説家になろう", SiteWorkID: "n9999ab", Title: "制御作品", Author: "作者",
			Episodes: []model.Episode{{Index: "1", Title: "第一話"}},
		},
	}
	app := New(Options{Config: config.Config{}, Store: store, Fetcher: fetcher, Logger: slog.Default()})
	startApp(t, app)
	defer store.Close()
	defer app.Shutdown(context.Background())
	defer func() { close(fetcher.release) }()
	created := performRequest(app, http.MethodPost, "/api/v2/novels/download", `{"targets":["https://example.com/running-control"]}`)
	if created.Code != http.StatusAccepted {
		t.Fatalf("create status = %d: %s", created.Code, created.Body.String())
	}
	select {
	case <-fetcher.entered:
	case <-time.After(time.Second):
		t.Fatal("task did not start")
	}
	summary := decodeObject(t, performRequest(app, http.MethodGet, "/api/v2/tasks/summary", ""))["data"].(map[string]any)
	current := summary["current"].(map[string]any)
	taskID := current["id"].(string)
	paused := performRequest(app, http.MethodPost, "/api/v2/tasks/"+taskID+"/pause", "")
	if paused.Code != http.StatusAccepted {
		t.Fatalf("running pause status = %d: %s", paused.Code, paused.Body.String())
	}
	pausedData := decodeObject(t, paused)["data"].(map[string]any)
	if pausedData["changed"] != true {
		t.Fatalf("running pause data = %#v", pausedData)
	}
	waitForIdleApp(t, app)
}

func TestResumeTaskReturnsConflictWhenDownloadTargetIsAlreadyActive(t *testing.T) {
	store, _ := newTestStoreWithWork(t)
	defer store.Close()
	app := New(Options{Config: config.Config{}, Store: store, Fetcher: staticFetcher{}, Logger: slog.Default()})

	failed := taskqueue.NewTask("download")
	failed.Targets = []string{"https://example.com/resume-target-conflict"}
	if err := app.queue.Enqueue(failed); err != nil {
		t.Fatal(err)
	}
	repository := taskstate.NewSQLiteRepository(store.DB())
	claimed, err := repository.ClaimNext(context.Background(), time.Now())
	if err != nil || claimed == nil {
		t.Fatalf("ClaimNext() = %#v, err = %v", claimed, err)
	}
	if err := repository.Finalize(context.Background(), taskstate.TaskRef{TaskID: claimed.ID, Attempt: claimed.AttemptCount}, taskstate.Outcome{Status: taskstate.StatusFailed, Error: errors.New("temporary")}); err != nil {
		t.Fatal(err)
	}
	active := taskqueue.NewTask("download")
	active.Targets = append([]string(nil), failed.Targets...)
	if err := app.queue.Enqueue(active); err != nil {
		t.Fatal(err)
	}

	response := performRequest(app, http.MethodPost, "/api/v2/tasks/"+failed.ID+"/resume", "")
	if response.Code != http.StatusConflict {
		t.Fatalf("resume conflict status = %d: %s", response.Code, response.Body.String())
	}
}

func TestRunningTaskCancelEndpointAcknowledgesCancellationRequest(t *testing.T) {
	store, _ := newTestStoreWithWork(t)
	fetcher := &blockingTocFetcher{entered: make(chan struct{}), release: make(chan struct{}), work: model.Work{
		Site: model.SiteSyosetu, SiteName: "小説家になろう", SiteWorkID: "n9999ac", Title: "中止作品", Author: "作者",
		Episodes: []model.Episode{{Index: "1", Title: "第一話"}},
	}}
	app := New(Options{Config: config.Config{}, Store: store, Fetcher: fetcher, Logger: slog.Default()})
	startApp(t, app)
	defer store.Close()
	defer app.Shutdown(context.Background())
	defer func() { close(fetcher.release) }()
	created := performRequest(app, http.MethodPost, "/api/v2/novels/download", `{"targets":["https://example.com/running-cancel"]}`)
	if created.Code != http.StatusAccepted {
		t.Fatalf("create status = %d: %s", created.Code, created.Body.String())
	}
	select {
	case <-fetcher.entered:
	case <-time.After(time.Second):
		t.Fatal("task did not start")
	}
	summary := decodeObject(t, performRequest(app, http.MethodGet, "/api/v2/tasks/summary", ""))["data"].(map[string]any)
	taskID := summary["current"].(map[string]any)["id"].(string)
	canceled := performRequest(app, http.MethodPost, "/api/v2/tasks/"+taskID+"/cancel", "")
	if canceled.Code != http.StatusAccepted {
		t.Fatalf("running cancel status = %d: %s", canceled.Code, canceled.Body.String())
	}
	waitForIdleApp(t, app)
}

func TestRunningTaskCancelInterruptsAssetHTTPBeforeDurableControlWrite(t *testing.T) {
	store, err := storage.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	assetFetcher := &blockingAssetFetcher{entered: make(chan struct{})}
	store.SetAssetFetcher(assetFetcher, fetcher.FetchPolicy{})
	app := New(Options{
		Config: config.Config{},
		Store:  store,
		Fetcher: staticFetcher{work: model.Work{
			Site: model.SiteSyosetu, SiteName: "小説家になろう", SiteWorkID: "n7777ab", Title: "asset制御作品", Author: "作者",
			Episodes: []model.Episode{{
				Index: "1", Href: "/n7777ab/1/", Title: "第一話", FetchedAt: time.Now(),
				Element: model.EpisodeElement{DataType: "html", Body: `<p><img src="https://example.com/image.png"></p>`},
			}},
		}},
		Logger: slog.Default(),
	})
	startApp(t, app)
	t.Cleanup(func() {
		app.Shutdown(context.Background())
		_ = store.Close()
	})

	created := performRequest(app, http.MethodPost, "/api/v2/novels/download", `{"targets":["https://ncode.syosetu.com/n7777ab/"]}`)
	if created.Code != http.StatusAccepted {
		t.Fatalf("create status = %d: %s", created.Code, created.Body.String())
	}
	taskID := decodeObject(t, created)["data"].(map[string]any)["task_ids"].([]any)[0].(string)
	select {
	case <-assetFetcher.entered:
	case <-time.After(time.Second):
		t.Fatal("asset fetch did not start")
	}

	response := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		response <- performRequest(app, http.MethodPost, "/api/v2/tasks/"+taskID+"/cancel", "")
	}()
	select {
	case canceled := <-response:
		if canceled.Code != http.StatusOK && canceled.Code != http.StatusAccepted {
			t.Fatalf("cancel status = %d: %s", canceled.Code, canceled.Body.String())
		}
	case <-time.After(time.Second):
		t.Fatal("cancel waited for the blocked asset HTTP request")
	}
	waitForIdleApp(t, app)
}

func (f *progressFetcher) FetchToc(_ context.Context, target string, _ sites.ProgressReporter) (model.Work, error) {
	return model.Work{
		Site:       model.SiteSyosetu,
		SiteName:   "小説家になろう",
		SiteWorkID: "n0001aa",
		SourceURL:  target,
		Title:      "テスト作品",
		Author:     "作者",
		FetchedAt:  time.Now(),
		Episodes: []model.Episode{
			{
				Index:        "1",
				Href:         "/n0001aa/1/",
				Title:        "第一話",
				FileSubtitle: "第一話",
				Element:      model.EpisodeElement{DataType: "html"},
			},
			{
				Index:        "2",
				Href:         "/n0001aa/2/",
				Title:        "第二話",
				FileSubtitle: "第二話",
				Element:      model.EpisodeElement{DataType: "html"},
			},
		},
	}, nil
}

func (f *progressFetcher) FetchEpisode(ctx context.Context, _ model.Work, episode model.Episode, report sites.ProgressReporter) (model.Episode, error) {
	report(sites.Progress{
		Phase:       "episode",
		CurrentStep: 1,
		TotalSteps:  2,
		Message:     "1 / 2 話を取得中: 第一話",
	})
	f.reportOnce.Do(func() {
		close(f.reported)
	})

	select {
	case <-ctx.Done():
		return model.Episode{}, ctx.Err()
	case <-f.release:
	}

	episode.FetchedAt = time.Now()
	episode.RawHTML = "<p>本文</p>"
	episode.Element = model.EpisodeElement{DataType: "html", Body: "<p>本文</p>"}
	return episode, nil
}

func TestTasksSummaryIncludesEpisodeProgress(t *testing.T) {
	fetcher := &progressFetcher{
		reported: make(chan struct{}),
		release:  make(chan struct{}),
	}
	store, err := storage.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	defer store.Close()
	app := New(Options{
		Config:  config.Config{},
		Store:   store,
		Fetcher: fetcher,
		Logger:  slog.Default(),
	})
	startApp(t, app)
	defer app.Shutdown(context.Background())

	server := httptest.NewServer(app.Handler())
	defer server.Close()

	response, err := http.Post(
		server.URL+"/api/v2/novels/download",
		"application/json",
		strings.NewReader(`{"targets":["https://ncode.syosetu.com/n0001aa/"]}`),
	)
	if err != nil {
		t.Fatalf("download request failed: %v", err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("download status = %d, want %d", response.StatusCode, http.StatusAccepted)
	}

	select {
	case <-fetcher.reported:
	case <-time.After(2 * time.Second):
		t.Fatal("fetcher did not report progress")
	}

	summaryResponse, err := http.Get(server.URL + "/api/v2/tasks/summary")
	if err != nil {
		t.Fatalf("summary request failed: %v", err)
	}
	defer summaryResponse.Body.Close()

	var summary struct {
		Data struct {
			Current map[string]any `json:"current"`
		} `json:"data"`
	}
	if err := json.NewDecoder(summaryResponse.Body).Decode(&summary); err != nil {
		t.Fatalf("summary decode failed: %v", err)
	}

	current := summary.Data.Current
	if current == nil {
		t.Fatal("current task was nil")
	}
	if current["phase"] != "episode" {
		t.Fatalf("phase = %#v, want episode", current["phase"])
	}
	if current["current_step"] != float64(1) || current["total_steps"] != float64(2) {
		t.Fatalf("steps = %#v / %#v, want 1 / 2", current["current_step"], current["total_steps"])
	}
	if current["progress"] != float64(50) {
		t.Fatalf("progress = %#v, want 50", current["progress"])
	}
	if current["message"] != "1 / 2 話を取得中: 第一話" {
		t.Fatalf("message = %#v", current["message"])
	}

	close(fetcher.release)
	waitForNoCurrentTask(t, server.URL)
}

func TestDownloadKeepsPartialWorkAfterEpisodeFailure(t *testing.T) {
	store, err := storage.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	defer store.Close()
	app := New(Options{
		Config:  config.Config{},
		Store:   store,
		Fetcher: partialFailFetcher{},
		Logger:  slog.Default(),
	})
	startApp(t, app)
	defer app.Shutdown(context.Background())

	server := httptest.NewServer(app.Handler())
	defer server.Close()

	response, err := http.Post(
		server.URL+"/api/v2/novels/download",
		"application/json",
		strings.NewReader(`{"targets":["https://ncode.syosetu.com/n9999aa/"]}`),
	)
	if err != nil {
		t.Fatalf("download request failed: %v", err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("download status = %d, want %d", response.StatusCode, http.StatusAccepted)
	}

	waitForIdleApp(t, app)

	works, err := store.ListWorks()
	if err != nil {
		t.Fatalf("ListWorks returned error: %v", err)
	}
	if len(works) != 1 {
		t.Fatalf("works = %#v", works)
	}
	if works[0].FetchStatus != storage.FetchStatusFailed || works[0].SavedEpisodeLen != 1 || works[0].EpisodeLen != 2 {
		t.Fatalf("partial work = %#v", works[0])
	}
	if works[0].ResumeEpisodeID != "2" || works[0].LastFailedEpisodeID != "2" {
		t.Fatalf("resume fields = %#v", works[0])
	}

	episodes, err := store.ListEpisodes(works[0].ID)
	if err != nil {
		t.Fatalf("ListEpisodes returned error: %v", err)
	}
	if len(episodes) != 2 {
		t.Fatalf("episodes = %#v", episodes)
	}
	if episodes[0].BodyStatus != storage.BodyStatusComplete || episodes[1].BodyStatus != storage.BodyStatusFailed {
		t.Fatalf("episode statuses = %#v", episodes)
	}
}

func TestDownloadMarksWorkCanceledWhenEpisodeSaveIsCanceled(t *testing.T) {
	store, err := storage.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	defer store.Close()
	store.SetAssetFetcher(cancelingAssetFetcher{}, fetcher.FetchPolicy{})

	app := New(Options{
		Config: config.Config{},
		Store:  store,
		Fetcher: staticFetcher{work: model.Work{
			Site:       model.SiteSyosetu,
			SiteName:   "小説家になろう",
			SiteWorkID: "n7777aa",
			Title:      "asset中止作品",
			Author:     "作者",
			FetchedAt:  time.Now(),
			Episodes: []model.Episode{{
				Index: "1",
				Href:  "/n7777aa/1/",
				Title: "第一話",
				Element: model.EpisodeElement{
					DataType: "html",
					Body:     `<p><img src="https://example.com/image.png"></p>`,
				},
				FetchedAt: time.Now(),
			}},
		}},
		Logger: slog.Default(),
	})
	startApp(t, app)
	defer app.Shutdown(context.Background())

	download := performRequest(app, http.MethodPost, "/api/v2/novels/download", `{"targets":["https://ncode.syosetu.com/n7777aa/"]}`)
	if download.Code != http.StatusAccepted {
		t.Fatalf("download status = %d: %s", download.Code, download.Body.String())
	}
	waitForIdleApp(t, app)

	works, err := store.ListWorks()
	if err != nil {
		t.Fatalf("ListWorks returned error: %v", err)
	}
	if len(works) != 1 {
		t.Fatalf("works = %#v", works)
	}
	if works[0].FetchStatus != storage.FetchStatusCanceled || works[0].ResumeEpisodeID != "1" {
		t.Fatalf("canceled work = %#v", works[0])
	}
}

func TestUpdateRefetchesCompleteEpisodes(t *testing.T) {
	store, stored := newTestStoreWithWork(t)
	fetcher := &countingFetcher{body: "<p>更新後本文</p>"}
	app := New(Options{
		Config:  config.Config{},
		Store:   store,
		Fetcher: fetcher,
		Logger:  slog.Default(),
	})
	startApp(t, app)
	defer store.Close()
	defer app.Shutdown(context.Background())

	update := performRequest(app, http.MethodPost, "/api/v2/novels/update", `{"ids":[`+storedID(stored.ID)+`]}`)
	if update.Code != http.StatusAccepted {
		t.Fatalf("update status = %d: %s", update.Code, update.Body.String())
	}
	waitForIdleApp(t, app)
	if fetcher.hitCount() != 1 {
		t.Fatalf("FetchEpisode calls = %d, want 1", fetcher.hitCount())
	}
	episode, ok, err := store.FindEpisode(stored.ID, "1")
	if err != nil || !ok {
		t.Fatalf("FindEpisode = %#v/%v/%v", episode, ok, err)
	}
	canonical, err := store.ReadCanonicalEpisode(episode)
	if err != nil {
		t.Fatalf("ReadCanonicalEpisode returned error: %v", err)
	}
	if canonical.Blocks[len(canonical.Blocks)-1].HTML != "<p>更新後本文</p>" {
		t.Fatalf("canonical = %#v", canonical)
	}
}

func TestUpdateSkipsCompleteEpisodesWhenSkipUnchanged(t *testing.T) {
	store, stored := newTestStoreWithWork(t)
	fetcher := &countingFetcher{body: "<p>更新後本文</p>"}
	app := New(Options{
		Config:  config.Config{},
		Store:   store,
		Fetcher: fetcher,
		Logger:  slog.Default(),
	})
	startApp(t, app)
	defer store.Close()
	defer app.Shutdown(context.Background())

	update := performRequest(app, http.MethodPost, "/api/v2/novels/update", `{"ids":[`+storedID(stored.ID)+`],"skip_unchanged":true}`)
	if update.Code != http.StatusAccepted {
		t.Fatalf("update status = %d: %s", update.Code, update.Body.String())
	}
	waitForIdleApp(t, app)
	if fetcher.hitCount() != 0 {
		t.Fatalf("FetchEpisode calls = %d, want 0", fetcher.hitCount())
	}
}

func TestUpdateSkipsNeverRevisedCompleteEpisodesWhenSkipUnchanged(t *testing.T) {
	store, err := storage.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	work := model.Work{
		Site:       model.SiteSyosetu,
		SiteName:   "小説家になろう",
		SiteWorkID: "n1234ab",
		SourceURL:  "https://ncode.syosetu.com/n1234ab/",
		Title:      "保存作品",
		Author:     "作者",
		FetchedAt:  time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC),
		Episodes: []model.Episode{{
			Index:       "1",
			Href:        "/n1234ab/1/",
			Title:       "第一話",
			PublishedAt: "2026/05/09 12:00",
			FetchedAt:   time.Date(2026, 5, 9, 12, 1, 0, 0, time.UTC),
			Element:     model.EpisodeElement{DataType: "html", Body: "<p>本文</p>"},
		}},
	}
	stored, err := saveWorkFully(t, store, work)
	if err != nil {
		_ = store.Close()
		t.Fatalf("saveWorkFully returned error: %v", err)
	}
	fetcher := &countingFetcher{body: "<p>更新後本文</p>", publishedAt: "2026/05/09 12:00", omitModifiedAt: true}
	app := New(Options{
		Config:  config.Config{},
		Store:   store,
		Fetcher: fetcher,
		Logger:  slog.Default(),
	})
	startApp(t, app)
	defer store.Close()
	defer app.Shutdown(context.Background())

	update := performRequest(app, http.MethodPost, "/api/v2/novels/update", `{"ids":[`+storedID(stored.ID)+`],"skip_unchanged":true}`)
	if update.Code != http.StatusAccepted {
		t.Fatalf("update status = %d: %s", update.Code, update.Body.String())
	}
	waitForIdleApp(t, app)
	if fetcher.hitCount() != 0 {
		t.Fatalf("FetchEpisode calls = %d, want 0", fetcher.hitCount())
	}
}

func TestUpdateRefetchesChangedCompleteEpisodesWhenSkipUnchanged(t *testing.T) {
	store, stored := newTestStoreWithWork(t)
	fetcher := &countingFetcher{body: "<p>変更後本文</p>", modifiedAt: "2026/05/10 12:00"}
	app := New(Options{
		Config:  config.Config{},
		Store:   store,
		Fetcher: fetcher,
		Logger:  slog.Default(),
	})
	startApp(t, app)
	defer store.Close()
	defer app.Shutdown(context.Background())

	update := performRequest(app, http.MethodPost, "/api/v2/novels/update", `{"ids":[`+storedID(stored.ID)+`],"skip_unchanged":true}`)
	if update.Code != http.StatusAccepted {
		t.Fatalf("update status = %d: %s", update.Code, update.Body.String())
	}
	waitForIdleApp(t, app)
	if fetcher.hitCount() != 1 {
		t.Fatalf("FetchEpisode calls = %d, want 1", fetcher.hitCount())
	}
	episode, ok, err := store.FindEpisode(stored.ID, "1")
	if err != nil || !ok {
		t.Fatalf("FindEpisode = %#v/%v/%v", episode, ok, err)
	}
	if episode.UpdatedAt != "2026/05/10 12:00" {
		t.Fatalf("episode.UpdatedAt = %q", episode.UpdatedAt)
	}
	canonical, err := store.ReadCanonicalEpisode(episode)
	if err != nil {
		t.Fatalf("ReadCanonicalEpisode returned error: %v", err)
	}
	if canonical.Blocks[len(canonical.Blocks)-1].HTML != "<p>変更後本文</p>" {
		t.Fatalf("canonical = %#v", canonical)
	}
}

func TestUpdateForceRedownloadOverridesSkipUnchanged(t *testing.T) {
	store, stored := newTestStoreWithWork(t)
	fetcher := &countingFetcher{body: "<p>強制更新本文</p>"}
	app := New(Options{
		Config:  config.Config{},
		Store:   store,
		Fetcher: fetcher,
		Logger:  slog.Default(),
	})
	startApp(t, app)
	defer store.Close()
	defer app.Shutdown(context.Background())

	update := performRequest(app, http.MethodPost, "/api/v2/novels/update", `{"ids":[`+storedID(stored.ID)+`],"force_redownload":true,"skip_unchanged":true}`)
	if update.Code != http.StatusAccepted {
		t.Fatalf("update status = %d: %s", update.Code, update.Body.String())
	}
	waitForIdleApp(t, app)
	if fetcher.hitCount() != 1 {
		t.Fatalf("FetchEpisode calls = %d, want 1", fetcher.hitCount())
	}
}

func TestResumeSkipsCompleteEpisodes(t *testing.T) {
	store, stored := newTestStoreWithWork(t)
	fetcher := &countingFetcher{body: "<p>再開本文</p>"}
	app := New(Options{
		Config:  config.Config{},
		Store:   store,
		Fetcher: fetcher,
		Logger:  slog.Default(),
	})
	startApp(t, app)
	defer store.Close()
	defer app.Shutdown(context.Background())

	resume := performRequest(app, http.MethodPost, "/api/v2/novels/resume", `{"ids":[`+storedID(stored.ID)+`]}`)
	if resume.Code != http.StatusAccepted {
		t.Fatalf("resume status = %d: %s", resume.Code, resume.Body.String())
	}
	waitForIdleApp(t, app)
	if fetcher.hitCount() != 0 {
		t.Fatalf("FetchEpisode calls = %d, want 0", fetcher.hitCount())
	}
}

func waitForNoCurrentTask(t *testing.T, serverURL string) {
	t.Helper()

	deadline := time.After(2 * time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-deadline:
			t.Fatal("task did not finish")
		case <-ticker.C:
			response, err := http.Get(serverURL + "/api/v2/tasks/summary")
			if err != nil {
				t.Fatalf("summary request failed: %v", err)
			}

			var summary struct {
				Data struct {
					Current map[string]any `json:"current"`
				} `json:"data"`
			}
			if err := json.NewDecoder(response.Body).Decode(&summary); err != nil {
				_ = response.Body.Close()
				t.Fatalf("summary decode failed: %v", err)
			}
			_ = response.Body.Close()

			if summary.Data.Current == nil {
				return
			}
		}
	}
}

func waitForIdleApp(t *testing.T, app *App) {
	t.Helper()

	deadline := time.After(2 * time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-deadline:
			t.Fatal("app did not become idle")
		case <-ticker.C:
			if app.queue.IsIdle() {
				return
			}
		}
	}
}

func startApp(t *testing.T, app *App) {
	t.Helper()

	app.Start(context.Background())
}

func newTestStoreWithWork(t *testing.T) (*storage.Store, model.StoredWork) {
	t.Helper()

	store, err := storage.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	stored, err := saveWorkFully(t, store, model.Work{
		Site:       model.SiteSyosetu,
		SiteName:   "小説家になろう",
		SiteWorkID: "n1234ab",
		SourceURL:  "https://ncode.syosetu.com/n1234ab/",
		Title:      "保存作品",
		Author:     "作者",
		Story:      "あらすじ",
		FetchedAt:  time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC),
		Episodes: []model.Episode{{
			Index:       "1",
			Href:        "/n1234ab/1/",
			Title:       "第一話",
			Chapter:     "第一章",
			PublishedAt: "2026/05/09 12:00",
			ModifiedAt:  "2026/05/09 12:00",
			FetchedAt:   time.Date(2026, 5, 9, 12, 1, 0, 0, time.UTC),
			Element:     model.EpisodeElement{DataType: "html", Body: "<p>本文</p>"},
		}},
	})
	if err != nil {
		_ = store.Close()
		t.Fatalf("逐次保存 helper returned error: %v", err)
	}
	return store, stored
}

func saveWorkFully(t *testing.T, store *storage.Store, work model.Work) (model.StoredWork, error) {
	t.Helper()

	ctx := context.Background()
	stored, err := store.UpsertWorkToc(ctx, work, storage.FetchStatusPartial)
	if err != nil {
		return model.StoredWork{}, err
	}
	for index, episode := range work.Episodes {
		if _, err := store.SaveEpisodeBody(ctx, work, stored, episode, index); err != nil {
			return model.StoredWork{}, err
		}
	}
	if err := store.UpdateWorkFetchStatus(ctx, stored.ID, storage.FetchStatusComplete, "", "", nil); err != nil {
		return model.StoredWork{}, err
	}
	stored, ok, err := store.FindWorkByID(stored.ID)
	if err != nil {
		return model.StoredWork{}, err
	}
	if !ok {
		return model.StoredWork{}, errors.New("stored work was not found")
	}
	return stored, nil
}

func performRequest(app *App, method string, path string, body string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("content-type", "application/json")
	}
	app.Handler().ServeHTTP(recorder, request)
	return recorder
}

func decodeObject(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()

	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response failed: %v\n%s", err, recorder.Body.String())
	}
	return payload
}

func storedID(id int) string {
	return strconv.Itoa(id)
}

func containsStringValue(values []any, expected string) bool {
	for _, value := range values {
		if text, ok := value.(string); ok && text == expected {
			return true
		}
	}
	return false
}
