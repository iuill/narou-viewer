package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"narou-viewer/services/novel-fetcher/internal/model"
	"narou-viewer/services/novel-fetcher/internal/sites"
	"narou-viewer/services/novel-fetcher/internal/storage"
	"narou-viewer/services/novel-fetcher/internal/taskqueue"
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
}

type fakeFetcher struct {
	work          model.Work
	tocErr        error
	episodeErr    error
	fetched       []string
	reportEpisode bool
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

func (s *fakeStore) SaveEpisodeBody(_ context.Context, _ model.Work, _ model.StoredWork, episode model.Episode, _ int) (model.StoredEpisode, error) {
	if s.saveErr != nil {
		return model.StoredEpisode{}, s.saveErr
	}
	s.savedEpisodes = append(s.savedEpisodes, episode.Index)
	return model.StoredEpisode{EpisodeID: episode.Index, BodyStatus: storage.BodyStatusComplete}, nil
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

func (r *recordingReporter) AddTaskNovelID(_ string, novelID int) {
	r.novelIDs = append(r.novelIDs, novelID)
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
	task := taskqueue.NewTask("download")
	task.Targets = []string{"https://ncode.syosetu.com/n1234ab/"}
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
	task := taskqueue.NewTask("download")
	task.Targets = []string{"https://kakuyomu.jp/works/0000000000000000000"}

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
	task := taskqueue.NewTask("update")
	task.NovelIDs = []int{30}
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

func TestRunTaskResumePropagatesMissingWork(t *testing.T) {
	service := NewService(Options{
		Store:    &fakeStore{},
		Fetcher:  &fakeFetcher{},
		Reporter: &recordingReporter{},
	})
	task := taskqueue.NewTask("resume")
	task.NovelIDs = []int{99}

	err := service.RunTask(context.Background(), task)
	if err == nil || err.Error() != "novel id 99 was not found" {
		t.Fatalf("err = %v", err)
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
	task := taskqueue.NewTask("resume")
	task.NovelIDs = []int{40}

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
	task := taskqueue.NewTask("update")
	task.NovelIDs = []int{50}

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
