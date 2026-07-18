package application

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"narou-viewer/services/novel-fetcher/internal/model"
	"narou-viewer/services/novel-fetcher/internal/sites"
	"narou-viewer/services/novel-fetcher/internal/storage"
	"narou-viewer/services/novel-fetcher/internal/taskqueue"
	"narou-viewer/services/novel-fetcher/internal/taskstate"
)

type fakeStore struct {
	work               model.StoredWork
	foundByID          bool
	foundBySite        bool
	episodes           []model.StoredEpisode
	titleMatches       []model.StoredWork
	upsertedStatuses   []string
	savedEpisodes      []string
	markedFailed       []string
	updatedStatuses    []string
	updateFailedID     string
	updateResumeID     string
	updateErrorMessage string
	saveErr            error
	checkpointValid    bool
	checkpointExists   bool
	checkpointCalls    int
	recordCalls        int
	taskSaveCalls      int
	completeCalls      int
	recordErr          error
	preflightErr       error
	preflightCalls     int
}

type fakeFetcher struct {
	work          model.Work
	tocErr        error
	episodeErr    error
	fetched       []string
	reportEpisode bool
}

type cancelAwareFetcher struct {
	work model.Work
}

func newServiceTask(kind string) *taskqueue.Task {
	task := taskqueue.NewTask(kind)
	task.AttemptCount = 1
	return task
}

func (f *cancelAwareFetcher) FetchToc(_ context.Context, target string, _ sites.ProgressReporter) (model.Work, error) {
	work := f.work
	work.SourceURL = target
	return work, nil
}

func (f *cancelAwareFetcher) FetchEpisode(ctx context.Context, _ model.Work, episode model.Episode, _ sites.ProgressReporter) (model.Episode, error) {
	if err := ctx.Err(); err != nil {
		return model.Episode{}, err
	}
	return episode, nil
}

type recordingReporter struct {
	progress        []sites.Progress
	messages        []string
	warnings        []string
	targets         []string
	novelIDs        []int
	savedCounts     []int
	failedEpisodeID string
	resumeEpisodeID string
}

func (s *fakeStore) FindWorkByID(id int) (model.StoredWork, bool, error) {
	if s.foundByID && s.work.ID == id {
		return s.work, true, nil
	}
	return model.StoredWork{}, false, nil
}

func (s *fakeStore) FindWorkBySiteKey(site string, siteWorkID string) (model.StoredWork, bool, error) {
	if s.foundBySite && string(s.work.Site) == site && s.work.SiteWorkID == siteWorkID {
		return s.work, true, nil
	}
	return model.StoredWork{}, false, nil
}

func (s *fakeStore) FindPotentialDuplicateWorks(work model.Work) ([]model.StoredWork, error) {
	matches := []model.StoredWork{}
	for _, match := range s.titleMatches {
		if match.Title == work.Title {
			matches = append(matches, match)
		}
	}
	return matches, nil
}

func (s *fakeStore) ListEpisodes(int) ([]model.StoredEpisode, error) {
	return s.episodes, nil
}

func (s *fakeStore) PreflightWorkMutation(_ model.StoredWork, _ model.Work) error {
	s.preflightCalls++
	return s.preflightErr
}

func (s *fakeStore) UpsertWorkToc(_ context.Context, work model.Work, status string) (model.StoredWork, error) {
	s.upsertedStatuses = append(s.upsertedStatuses, status)
	stored := s.work
	if stored.ID == 0 {
		stored.ID = 10
	}
	stored.Site = work.Site
	stored.SiteWorkID = work.SiteWorkID
	stored.Title = work.Title
	stored.SourceURL = work.SourceURL
	return stored, nil
}

func (s *fakeStore) IsTaskEpisodeCheckpointValid(_ context.Context, _ taskstate.TaskRef, _ model.Work, _ model.StoredWork, _ model.Episode, _ int) (bool, bool, error) {
	s.checkpointCalls++
	return s.checkpointValid, s.checkpointExists || s.checkpointValid, nil
}

func (s *fakeStore) RecordTaskEpisodeCheckpoint(_ context.Context, _ taskstate.TaskRef, _ int, _ string, _ int, _ string) error {
	s.recordCalls++
	return s.recordErr
}

func (s *fakeStore) SaveEpisodeBodyForTask(_ context.Context, _ taskstate.TaskRef, _ model.Work, _ model.StoredWork, episode model.Episode, _ int, _ string) (model.StoredEpisode, error) {
	s.taskSaveCalls++
	if s.saveErr != nil {
		return model.StoredEpisode{}, s.saveErr
	}
	s.savedEpisodes = append(s.savedEpisodes, episode.Index)
	return model.StoredEpisode{EpisodeID: episode.Index, BodyStatus: storage.BodyStatusComplete}, nil
}

func (s *fakeStore) CompleteWorkForTask(_ context.Context, _ taskstate.TaskRef, _ int) error {
	s.completeCalls++
	s.updatedStatuses = append(s.updatedStatuses, storage.FetchStatusComplete)
	return nil
}

func (s *fakeStore) MarkEpisodeFailed(_ context.Context, _ int, episodeID string, _ error) error {
	s.markedFailed = append(s.markedFailed, episodeID)
	return nil
}

func (s *fakeStore) UpdateWorkFetchStatus(_ context.Context, _ int, status string, failedEpisodeID string, resumeEpisodeID string, fetchError error) error {
	s.updatedStatuses = append(s.updatedStatuses, status)
	s.updateFailedID = failedEpisodeID
	s.updateResumeID = resumeEpisodeID
	if fetchError != nil {
		s.updateErrorMessage = fetchError.Error()
	}
	return nil
}

func (f *fakeFetcher) FetchToc(_ context.Context, target string, report sites.ProgressReporter) (model.Work, error) {
	if f.tocErr != nil {
		return model.Work{}, f.tocErr
	}
	if report != nil {
		report(sites.Progress{Phase: "toc", Message: "toc ok"})
	}
	work := f.work
	work.SourceURL = target
	return work, nil
}

func (f *fakeFetcher) FetchEpisode(_ context.Context, _ model.Work, episode model.Episode, report sites.ProgressReporter) (model.Episode, error) {
	if f.episodeErr != nil {
		return model.Episode{}, f.episodeErr
	}
	f.fetched = append(f.fetched, episode.Index)
	if f.reportEpisode && report != nil {
		report(sites.Progress{Phase: "episode", Message: "custom", CurrentStep: 1, TotalSteps: 3})
	}
	episode.FetchedAt = time.Now()
	episode.Element = model.EpisodeElement{DataType: "html", Body: "<p>本文</p>"}
	return episode, nil
}

func (r *recordingReporter) SetTaskProgress(_ string, progress sites.Progress) {
	r.progress = append(r.progress, progress)
}

func (r *recordingReporter) SetTaskMessage(_ string, message string) {
	r.messages = append(r.messages, message)
}

func (r *recordingReporter) AddTaskWarning(_ string, warning string) {
	r.warnings = append(r.warnings, warning)
}

func (r *recordingReporter) SetTaskTarget(_ string, target string) {
	r.targets = append(r.targets, target)
}

func (r *recordingReporter) SetTaskWorkID(_ string, novelID int) error {
	r.novelIDs = append(r.novelIDs, novelID)
	return nil
}

func (r *recordingReporter) SetTaskSavedEpisodeCount(_ string, count int) {
	r.savedCounts = append(r.savedCounts, count)
}

func (r *recordingReporter) SetTaskFailureEpisode(_ string, failedEpisodeID string, resumeEpisodeID string) {
	r.failedEpisodeID = failedEpisodeID
	r.resumeEpisodeID = resumeEpisodeID
}

func TestEpisodeCanBeSkippedMatchesCompleteTimestamp(t *testing.T) {
	work := model.Work{Episodes: []model.Episode{{
		Index:       "1",
		ModifiedAt:  "2026/05/09 12:00",
		PublishedAt: "2026/05/09 11:00",
	}}}
	stored := model.StoredEpisode{
		EpisodeID:  "1",
		BodyStatus: storage.BodyStatusComplete,
		UpdatedAt:  "2026/05/09 12:00",
	}

	if !EpisodeCanBeSkipped(stored, work) {
		t.Fatal("complete episode with matching timestamp should be skippable")
	}
}

func TestEpisodeCanBeSkippedRejectsMissingBodyOrChangedTimestamp(t *testing.T) {
	work := model.Work{Episodes: []model.Episode{{Index: "1", ModifiedAt: "2026/05/10 12:00"}}}
	stored := model.StoredEpisode{
		EpisodeID:  "1",
		BodyStatus: storage.BodyStatusComplete,
		UpdatedAt:  "2026/05/09 12:00",
	}

	if EpisodeCanBeSkipped(stored, work) {
		t.Fatal("changed timestamp should not be skippable")
	}
	stored.UpdatedAt = "2026/05/10 12:00"
	stored.BodyStatus = storage.BodyStatusFailed
	if EpisodeCanBeSkipped(stored, work) {
		t.Fatal("failed body should not be skippable")
	}
}

func TestCanonicalTaskEpisodeIDUsesIndexOrOneBasedFallback(t *testing.T) {
	if got := CanonicalTaskEpisodeID(model.Episode{Index: " 2 "}, 9); got != " 2 " {
		t.Fatalf("CanonicalTaskEpisodeID with index = %q", got)
	}
	if got := CanonicalTaskEpisodeID(model.Episode{}, 0); got != "1" {
		t.Fatalf("CanonicalTaskEpisodeID fallback = %q", got)
	}
}

func TestRunTaskRequiresClaimedTask(t *testing.T) {
	service := NewService(Options{Store: &fakeStore{}, Fetcher: &fakeFetcher{}, Reporter: &recordingReporter{}})
	unclaimed := taskqueue.NewTask("download")
	unclaimed.Target = "https://example.invalid/unclaimed"
	if err := service.RunTask(context.Background(), unclaimed); err == nil {
		t.Fatal("unclaimed task was executed")
	}
}

func TestRunTaskDownloadsAndSavesEpisodes(t *testing.T) {
	work := model.Work{
		Site:       model.SiteSyosetu,
		SiteName:   "小説家になろう",
		SiteWorkID: "n1234ab",
		Title:      "取得作品",
		Author:     "作者",
		Episodes: []model.Episode{{
			Index: "1",
			Title: "第一話",
		}},
	}
	store := &fakeStore{
		work:        model.StoredWork{ID: 20, Site: model.SiteSyosetu, SiteWorkID: "n1234ab"},
		foundBySite: true,
	}
	fetcher := &fakeFetcher{work: work, reportEpisode: true}
	reporter := &recordingReporter{}
	service := NewService(Options{Store: store, Fetcher: fetcher, Reporter: reporter})
	task := newServiceTask("download")
	task.Target = "https://ncode.syosetu.com/n1234ab/"
	task.Force = true

	if err := service.RunTask(context.Background(), task); err != nil {
		t.Fatalf("RunTask returned error: %v", err)
	}
	if len(store.savedEpisodes) != 1 || store.savedEpisodes[0] != "1" {
		t.Fatalf("saved episodes = %#v", store.savedEpisodes)
	}
	if len(store.updatedStatuses) != 1 || store.updatedStatuses[0] != storage.FetchStatusComplete {
		t.Fatalf("updated statuses = %#v", store.updatedStatuses)
	}
	if len(reporter.novelIDs) != 1 || reporter.novelIDs[0] != 20 {
		t.Fatalf("reported novel ids = %#v", reporter.novelIDs)
	}
	if reporter.targets[0] != "取得作品" || reporter.messages[0] != "saved 取得作品" {
		t.Fatalf("reporter = %#v", reporter)
	}
	if len(reporter.progress) == 0 || reporter.progress[len(reporter.progress)-1].Phase != "episode" {
		t.Fatalf("progress = %#v", reporter.progress)
	}
}

func TestRunTaskRejectsDuplicateTitleAcrossSites(t *testing.T) {
	work := model.Work{
		Site:       model.SiteKakuyomu,
		SiteName:   "カクヨム",
		SiteWorkID: "0000000000000000000",
		Title:      "同名作品",
		Author:     "作者",
		Episodes:   []model.Episode{{Index: "1", Title: "第一話"}},
	}
	store := &fakeStore{
		titleMatches: []model.StoredWork{
			{ID: 20, Site: model.SiteSyosetu, SiteName: "小説家になろう", SiteWorkID: "n1234ab", Title: "同名作品"},
			{ID: 21, Site: model.SiteKakuyomu, SiteName: "カクヨム", SiteWorkID: "0000000000000000000", Title: "同名作品"},
		},
	}
	reporter := &recordingReporter{}
	service := NewService(Options{Store: store, Fetcher: &fakeFetcher{work: work}, Reporter: reporter})
	task := newServiceTask("download")
	task.Target = "https://kakuyomu.jp/works/0000000000000000000"

	err := service.RunTask(context.Background(), task)
	if err == nil || err.Error() != "同名または近いタイトルの作品が別サイトにあるため、ダウンロードを取りやめました" {
		t.Fatalf("RunTask error = %v", err)
	}
	if len(reporter.warnings) != 1 || reporter.warnings[0] != "同名または近いタイトルの作品が別サイトにあります: 同名作品（小説家になろう）" {
		t.Fatalf("warnings = %#v", reporter.warnings)
	}
	if len(store.upsertedStatuses) != 0 || len(store.savedEpisodes) != 0 {
		t.Fatalf("duplicate download should stop before save: upserts=%#v saved=%#v", store.upsertedStatuses, store.savedEpisodes)
	}
}

func TestRunTaskUpdateSkipsUnchangedEpisode(t *testing.T) {
	work := model.Work{
		Site:       model.SiteSyosetu,
		SiteWorkID: "n1234ab",
		Title:      "更新作品",
		Episodes: []model.Episode{{
			Index:      "1",
			ModifiedAt: "2026/05/09 12:00",
		}},
	}
	store := &fakeStore{
		work:      model.StoredWork{ID: 30, Site: model.SiteSyosetu, SiteWorkID: "n1234ab", SourceURL: "https://ncode.syosetu.com/n1234ab/"},
		foundByID: true,
		episodes: []model.StoredEpisode{{
			EpisodeID:  "1",
			BodyStatus: storage.BodyStatusComplete,
			UpdatedAt:  "2026/05/09 12:00",
		}},
	}
	fetcher := &fakeFetcher{work: work}
	reporter := &recordingReporter{}
	service := NewService(Options{Store: store, Fetcher: fetcher, Reporter: reporter})
	task := newServiceTask("update")
	task.WorkID = 30
	task.SkipUnchanged = true

	if err := service.RunTask(context.Background(), task); err != nil {
		t.Fatalf("RunTask returned error: %v", err)
	}
	if len(fetcher.fetched) != 0 || len(store.savedEpisodes) != 0 {
		t.Fatalf("fetched/saved = %#v/%#v", fetcher.fetched, store.savedEpisodes)
	}
	if len(reporter.savedCounts) != 1 || reporter.savedCounts[0] != 1 {
		t.Fatalf("saved counts = %#v", reporter.savedCounts)
	}
	if reporter.messages[0] != "updated 更新作品" {
		t.Fatalf("messages = %#v", reporter.messages)
	}
}

func TestRunTaskUpdateRejectsFutureCanonicalSchemaBeforeAnyMutation(t *testing.T) {
	tests := []struct {
		name          string
		removeEpisode bool
		skipUnchanged bool
	}{
		{name: "regular update"},
		{name: "skip unchanged", skipUnchanged: true},
		{name: "episode prune", removeEpisode: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rootDir := t.TempDir()
			store, err := storage.NewStore(rootDir)
			if err != nil {
				t.Fatalf("NewStore returned error: %v", err)
			}
			defer store.Close()

			originalWork := model.Work{
				Site:       model.SiteVerification,
				SiteName:   "Verification",
				SiteWorkID: "future-update-work",
				SourceURL:  "https://example.invalid/future-update-work/",
				Title:      "Synthetic original work",
				Author:     "Synthetic author",
				FetchedAt:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				Episodes: []model.Episode{{
					Index:      "1",
					Title:      "Synthetic original episode",
					ModifiedAt: "2026-01-01T00:00:00Z",
					FetchedAt:  time.Date(2026, 1, 1, 0, 1, 0, 0, time.UTC),
					Element:    model.EpisodeElement{DataType: "html", Body: "<p>Synthetic original body.</p>"},
				}},
			}
			stored, err := store.UpsertWorkToc(context.Background(), originalWork, storage.FetchStatusPartial)
			if err != nil {
				t.Fatalf("UpsertWorkToc returned error: %v", err)
			}
			if _, err := store.SaveEpisodeBody(context.Background(), originalWork, stored, originalWork.Episodes[0], 0); err != nil {
				t.Fatalf("SaveEpisodeBody returned error: %v", err)
			}
			if err := store.UpdateWorkFetchStatus(context.Background(), stored.ID, storage.FetchStatusComplete, "", "", nil); err != nil {
				t.Fatalf("UpdateWorkFetchStatus returned error: %v", err)
			}

			beforeWork, found, err := store.FindWorkByID(stored.ID)
			if err != nil || !found {
				t.Fatalf("FindWorkByID before update = %#v/%v/%v", beforeWork, found, err)
			}
			beforeEpisodes, err := store.ListEpisodes(stored.ID)
			if err != nil || len(beforeEpisodes) != 1 {
				t.Fatalf("ListEpisodes before update = %#v/%v", beforeEpisodes, err)
			}
			futureBytes, err := os.ReadFile(filepath.Join("..", "storage", "testdata", "canonical_episode_v99.json"))
			if err != nil {
				t.Fatalf("read future fixture: %v", err)
			}
			canonicalPath := filepath.Join(rootDir, beforeEpisodes[0].BodyPath)
			if err := os.WriteFile(canonicalPath, futureBytes, 0o644); err != nil {
				t.Fatalf("seed future canonical episode: %v", err)
			}

			incomingWork := originalWork
			incomingWork.Title = "Synthetic updated work"
			incomingWork.FetchedAt = time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
			incomingWork.Episodes = append([]model.Episode(nil), originalWork.Episodes...)
			if test.removeEpisode {
				incomingWork.Episodes = nil
			} else {
				incomingWork.Episodes[0].Title = "Synthetic updated episode"
			}

			fetcher := &fakeFetcher{work: incomingWork}
			reporter := &recordingReporter{}
			service := NewService(Options{Store: store, Fetcher: fetcher, Reporter: reporter})
			task := newServiceTask("update")
			task.WorkID = stored.ID
			task.SkipUnchanged = test.skipUnchanged

			err = service.RunTask(context.Background(), task)
			var unsupported storage.ErrUnsupportedEpisodeSchema
			if !errors.As(err, &unsupported) {
				t.Fatalf("RunTask error = %v, want ErrUnsupportedEpisodeSchema", err)
			}
			if len(fetcher.fetched) != 0 || len(reporter.messages) != 0 {
				t.Fatalf("update continued after preflight: fetched=%#v messages=%#v", fetcher.fetched, reporter.messages)
			}

			afterWork, found, err := store.FindWorkByID(stored.ID)
			if err != nil || !found {
				t.Fatalf("FindWorkByID after update = %#v/%v/%v", afterWork, found, err)
			}
			afterEpisodes, err := store.ListEpisodes(stored.ID)
			if err != nil {
				t.Fatalf("ListEpisodes after update returned error: %v", err)
			}
			if !reflect.DeepEqual(afterWork, beforeWork) || !reflect.DeepEqual(afterEpisodes, beforeEpisodes) {
				t.Fatalf("storage metadata changed:\nwork before=%#v\nwork after=%#v\nepisodes before=%#v\nepisodes after=%#v", beforeWork, afterWork, beforeEpisodes, afterEpisodes)
			}
			afterBytes, err := os.ReadFile(canonicalPath)
			if err != nil {
				t.Fatalf("read canonical after update: %v", err)
			}
			if !bytes.Equal(afterBytes, futureBytes) {
				t.Fatal("future canonical bytes changed during rejected update")
			}
		})
	}
}

func TestRunTaskResumePropagatesMissingWork(t *testing.T) {
	service := NewService(Options{
		Store:    &fakeStore{},
		Fetcher:  &fakeFetcher{},
		Reporter: &recordingReporter{},
	})
	task := newServiceTask("resume")
	task.WorkID = 99

	err := service.RunTask(context.Background(), task)
	if err == nil || err.Error() != "novel id 99 was not found" {
		t.Fatalf("err = %v", err)
	}
}

func TestRunTaskResumeFetchesAndCompletesWork(t *testing.T) {
	work := model.Work{
		Site:       model.SiteSyosetu,
		SiteWorkID: "resume-work",
		Title:      "再開作品",
		Episodes:   []model.Episode{{Index: "1", Title: "第一話"}},
	}
	store := &fakeStore{
		work:      model.StoredWork{ID: 63, Site: model.SiteSyosetu, SiteWorkID: "resume-work", SourceURL: "https://example.invalid/resume/"},
		foundByID: true,
	}
	service := NewService(Options{Store: store, Fetcher: &fakeFetcher{work: work}, Reporter: &recordingReporter{}})
	task := newServiceTask("resume")
	task.WorkID = 63
	if err := service.RunTask(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if len(store.savedEpisodes) != 1 || store.savedEpisodes[0] != "1" {
		t.Fatalf("saved episodes = %#v", store.savedEpisodes)
	}
	if len(store.updatedStatuses) != 1 || store.updatedStatuses[0] != storage.FetchStatusComplete {
		t.Fatalf("completion statuses = %#v", store.updatedStatuses)
	}
}

func TestRunTaskMarksEpisodeFailed(t *testing.T) {
	saveErr := errors.New("save failed")
	work := model.Work{
		Site:       model.SiteSyosetu,
		SiteWorkID: "n1234ab",
		Title:      "失敗作品",
		Episodes:   []model.Episode{{Index: "1", Title: "第一話"}},
	}
	store := &fakeStore{
		work:      model.StoredWork{ID: 40, Site: model.SiteSyosetu, SiteWorkID: "n1234ab", SourceURL: "https://ncode.syosetu.com/n1234ab/"},
		foundByID: true,
		saveErr:   saveErr,
	}
	reporter := &recordingReporter{}
	service := NewService(Options{Store: store, Fetcher: &fakeFetcher{work: work}, Reporter: reporter})
	task := newServiceTask("resume")
	task.WorkID = 40

	err := service.RunTask(context.Background(), task)
	if !errors.Is(err, saveErr) {
		t.Fatalf("err = %v", err)
	}
	if len(store.markedFailed) != 1 || store.markedFailed[0] != "1" {
		t.Fatalf("marked failed = %#v", store.markedFailed)
	}
	if store.updatedStatuses[0] != storage.FetchStatusFailed || store.updateFailedID != "1" || store.updateResumeID != "1" {
		t.Fatalf("failure status = %#v %q %q", store.updatedStatuses, store.updateFailedID, store.updateResumeID)
	}
	if reporter.failedEpisodeID != "1" || reporter.resumeEpisodeID != "1" {
		t.Fatalf("reporter failure = %#v", reporter)
	}
}

func TestRunTaskDoesNotMarkUnsupportedCanonicalSchemaAsFetchFailure(t *testing.T) {
	observed := 99
	unsupported := storage.ErrUnsupportedEpisodeSchema{Path: "future.json", Observed: &observed, Supported: 1}
	work := model.Work{
		Site:       model.SiteVerification,
		SiteWorkID: "future-save-work",
		Title:      "Synthetic future save work",
		Episodes:   []model.Episode{{Index: "1", Title: "Synthetic episode"}},
	}
	store := &fakeStore{
		work:      model.StoredWork{ID: 41, Site: model.SiteVerification, SiteWorkID: "future-save-work", SourceURL: "https://example.invalid/future-save-work/"},
		foundByID: true,
		saveErr:   unsupported,
	}
	reporter := &recordingReporter{}
	service := NewService(Options{Store: store, Fetcher: &fakeFetcher{work: work}, Reporter: reporter})
	task := newServiceTask("resume")
	task.WorkID = 41

	err := service.RunTask(context.Background(), task)
	var got storage.ErrUnsupportedEpisodeSchema
	if !errors.As(err, &got) {
		t.Fatalf("RunTask error = %v, want ErrUnsupportedEpisodeSchema", err)
	}
	if len(store.markedFailed) != 0 || len(store.updatedStatuses) != 0 {
		t.Fatalf("unsupported schema was recorded as fetch failure: marked=%#v statuses=%#v", store.markedFailed, store.updatedStatuses)
	}
	if reporter.failedEpisodeID != "" || reporter.resumeEpisodeID != "" {
		t.Fatalf("unsupported schema was reported as resumable fetch failure: %#v", reporter)
	}
}

func TestRunTaskUsesTaskCheckpointBeforeLocalSkipAndSave(t *testing.T) {
	work := model.Work{
		Site:       model.SiteSyosetu,
		SiteWorkID: "checkpoint-work",
		Title:      "チェックポイント作品",
		Episodes:   []model.Episode{{Index: "1", Title: "第一話"}},
	}

	t.Run("valid checkpoint skips fetch", func(t *testing.T) {
		store := &fakeStore{
			work:            model.StoredWork{ID: 60, Site: model.SiteSyosetu, SiteWorkID: "checkpoint-work", SourceURL: "https://example.invalid/checkpoint/"},
			foundByID:       true,
			checkpointValid: true,
		}
		fetcher := &fakeFetcher{work: work}
		service := NewService(Options{Store: store, Fetcher: fetcher, Reporter: &recordingReporter{}})
		task := newServiceTask("update")
		task.WorkID = 60
		task.SkipUnchanged = true
		task.AttemptCount = 1
		if err := service.RunTask(context.Background(), task); err != nil {
			t.Fatal(err)
		}
		if store.checkpointCalls != 1 || store.taskSaveCalls != 0 || len(fetcher.fetched) != 0 {
			t.Fatalf("checkpoint path = calls:%d saves:%d fetched:%#v", store.checkpointCalls, store.taskSaveCalls, fetcher.fetched)
		}
	})

	t.Run("missing checkpoint saves atomically", func(t *testing.T) {
		store := &fakeStore{
			work:      model.StoredWork{ID: 61, Site: model.SiteSyosetu, SiteWorkID: "checkpoint-work", SourceURL: "https://example.invalid/checkpoint/"},
			foundByID: true,
		}
		fetcher := &fakeFetcher{work: work}
		service := NewService(Options{Store: store, Fetcher: fetcher, Reporter: &recordingReporter{}})
		task := newServiceTask("update")
		task.WorkID = 61
		task.SkipUnchanged = true
		task.AttemptCount = 1
		if err := service.RunTask(context.Background(), task); err != nil {
			t.Fatal(err)
		}
		if store.checkpointCalls != 1 || store.taskSaveCalls != 1 || len(fetcher.fetched) != 1 {
			t.Fatalf("save path = calls:%d saves:%d fetched:%#v", store.checkpointCalls, store.taskSaveCalls, fetcher.fetched)
		}
	})

	t.Run("stale checkpoint cannot fall back to local timestamp skip", func(t *testing.T) {
		store := &fakeStore{
			work:             model.StoredWork{ID: 65, Site: model.SiteSyosetu, SiteWorkID: "checkpoint-work", SourceURL: "https://example.invalid/checkpoint/"},
			foundByID:        true,
			checkpointExists: true,
			episodes: []model.StoredEpisode{{
				EpisodeID:  "1",
				BodyStatus: storage.BodyStatusComplete,
				UpdatedAt:  "2026/05/09 12:00",
			}},
		}
		workWithRevision := work
		workWithRevision.Episodes = []model.Episode{{Index: "1", Title: "第一話", ModifiedAt: "2026/05/09 12:00"}}
		fetcher := &fakeFetcher{work: workWithRevision}
		service := NewService(Options{Store: store, Fetcher: fetcher, Reporter: &recordingReporter{}})
		task := newServiceTask("update")
		task.WorkID = 65
		task.SkipUnchanged = true
		task.AttemptCount = 2
		if err := service.RunTask(context.Background(), task); err != nil {
			t.Fatal(err)
		}
		if store.checkpointCalls != 1 || store.recordCalls != 0 || store.taskSaveCalls != 1 || len(fetcher.fetched) != 1 {
			t.Fatalf("stale checkpoint path = checks:%d records:%d saves:%d fetched:%#v", store.checkpointCalls, store.recordCalls, store.taskSaveCalls, fetcher.fetched)
		}
	})
}

func TestRunTaskRefetchesWhenLocalEpisodeCannotCreateCheckpoint(t *testing.T) {
	work := model.Work{
		Site:       model.SiteSyosetu,
		SiteWorkID: "invalid-checkpoint-work",
		Title:      "壊れたチェックポイント作品",
		Episodes:   []model.Episode{{Index: "1", Title: "第一話", PublishedAt: "2026/05/09 12:00"}},
	}
	store := &fakeStore{
		work:      model.StoredWork{ID: 64, Site: model.SiteSyosetu, SiteWorkID: "invalid-checkpoint-work", SourceURL: "https://example.invalid/invalid-checkpoint/"},
		foundByID: true,
		episodes:  []model.StoredEpisode{{EpisodeID: "1", BodyStatus: storage.BodyStatusComplete, UpdatedAt: "2026/05/09 12:00"}},
		recordErr: storage.ErrInvalidTaskEpisodeCheckpoint,
	}
	fetcher := &fakeFetcher{work: work}
	service := NewService(Options{Store: store, Fetcher: fetcher, Reporter: &recordingReporter{}})
	task := newServiceTask("update")
	task.WorkID = 64
	task.SkipUnchanged = true
	task.AttemptCount = 1
	if err := service.RunTask(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if store.recordCalls != 1 || store.taskSaveCalls != 1 || len(fetcher.fetched) != 1 {
		t.Fatalf("invalid checkpoint recovery = records:%d saves:%d fetched:%#v", store.recordCalls, store.taskSaveCalls, fetcher.fetched)
	}
	checkpointErr := errors.New("checkpoint write failed")
	store.recordErr = checkpointErr
	if err := service.RunTask(context.Background(), task); !errors.Is(err, checkpointErr) {
		t.Fatalf("checkpoint write error = %v", err)
	}
}

func TestRunTaskControlCauseDoesNotMarkEpisodeAsFetchFailure(t *testing.T) {
	work := model.Work{
		Site:       model.SiteSyosetu,
		SiteWorkID: "paused-work",
		Title:      "一時停止作品",
		Episodes:   []model.Episode{{Index: "1", Title: "第一話"}},
	}
	store := &fakeStore{
		work:      model.StoredWork{ID: 62, Site: model.SiteSyosetu, SiteWorkID: "paused-work", SourceURL: "https://example.invalid/paused/"},
		foundByID: true,
	}
	reporter := &recordingReporter{}
	service := NewService(Options{Store: store, Fetcher: &cancelAwareFetcher{work: work}, Reporter: reporter})
	task := newServiceTask("resume")
	task.WorkID = 62
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(taskstate.ErrTaskPauseRequested)

	err := service.RunTask(ctx, task)
	if !errors.Is(err, taskstate.ErrTaskPauseRequested) {
		t.Fatalf("RunTask error = %v", err)
	}
	if len(store.markedFailed) != 0 || len(reporter.failedEpisodeID) != 0 {
		t.Fatalf("control cause was recorded as episode failure: marked=%#v reporter=%#v", store.markedFailed, reporter)
	}
	if len(store.updatedStatuses) != 1 || store.updatedStatuses[0] != storage.FetchStatusPaused {
		t.Fatalf("paused status = %#v", store.updatedStatuses)
	}
}

func TestMarkTaskControlMapsAllRunnerCauses(t *testing.T) {
	store := &fakeStore{}
	service := NewService(Options{Store: store, Fetcher: &fakeFetcher{}, Reporter: &recordingReporter{}})
	causes := []struct {
		cause  error
		status string
	}{
		{taskstate.ErrTaskCancelRequested, storage.FetchStatusCanceled},
		{taskstate.ErrTaskPauseRequested, storage.FetchStatusPaused},
		{taskstate.ErrRunnerShutdown, storage.FetchStatusInterrupted},
	}
	for _, test := range causes {
		service.markTaskControl(70, "1", test.cause)
		if got := store.updatedStatuses[len(store.updatedStatuses)-1]; got != test.status {
			t.Fatalf("cause %v status = %q, want %q", test.cause, got, test.status)
		}
	}
}

func TestRunTaskMarksCanceledEpisodeFailure(t *testing.T) {
	work := model.Work{
		Site:       model.SiteSyosetu,
		SiteWorkID: "n1234ab",
		Title:      "中止作品",
		Episodes:   []model.Episode{{Index: "1", Title: "第一話"}},
	}
	store := &fakeStore{
		work:      model.StoredWork{ID: 50, Site: model.SiteSyosetu, SiteWorkID: "n1234ab", SourceURL: "https://ncode.syosetu.com/n1234ab/"},
		foundByID: true,
	}
	reporter := &recordingReporter{}
	service := NewService(Options{Store: store, Fetcher: &fakeFetcher{work: work, episodeErr: context.Canceled}, Reporter: reporter})
	task := newServiceTask("update")
	task.WorkID = 50

	err := service.RunTask(context.Background(), task)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if store.updatedStatuses[0] != storage.FetchStatusCanceled || store.updateFailedID != "1" || store.updateResumeID != "1" {
		t.Fatalf("canceled status = %#v %q %q", store.updatedStatuses, store.updateFailedID, store.updateResumeID)
	}
	if reporter.failedEpisodeID != "1" || reporter.resumeEpisodeID != "1" {
		t.Fatalf("reporter failure = %#v", reporter)
	}
}
