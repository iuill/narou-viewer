package fetchercommands

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"narou-viewer/apps/viewer-api-go/internal/application/removedstate"
	"narou-viewer/apps/viewer-api-go/internal/fetcher"
)

func TestServiceUpdateResolvesNovelIDsAndDecoratesResult(t *testing.T) {
	client := &fakeClient{updateResult: fetcher.UpdateResponse{TaskIDs: []string{"task-update"}}}
	service := NewService(client, fakeResolver{
		"novel-a": "12",
		"novel-b": "34",
	})

	result, err := service.Update(context.Background(), []string{"novel-a", "novel-b"}, UpdateOptions{
		ForceRedownload:    true,
		IncludeFrozen:      true,
		ConvertAfterUpdate: true,
		SkipUnchanged:      true,
	})
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if !reflect.DeepEqual(client.updateIDs, []int{12, 34}) {
		t.Fatalf("Update should pass fetcher work IDs: %+v", client.updateIDs)
	}
	if !client.updateForceRedownload ||
		!client.updateIncludeFrozen ||
		!client.updateConvertAfterUpdate ||
		!client.updateSkipUnchanged {
		t.Fatalf("Update should pass all options: %+v", client)
	}
	if result.Message != "Update started" {
		t.Fatalf("Update should add fallback message: %+v", result)
	}
	if !reflect.DeepEqual(result.NovelIDs, []string{"novel-a", "novel-b"}) {
		t.Fatalf("Update should preserve viewer novel IDs: %+v", result)
	}
	if !reflect.DeepEqual(result.FetcherWorkIDs, []string{"12", "34"}) {
		t.Fatalf("Update should expose fetcher work IDs: %+v", result)
	}
}

func TestServiceDownloadPassesOptionsAndPreservesMessage(t *testing.T) {
	client := &fakeClient{downloadResult: fetcher.DownloadResponse{Message: "queued"}}
	service := NewService(client, nil)

	result, err := service.Download(context.Background(), []string{"https://example.test/novel"}, DownloadOptions{
		Force:                true,
		ConvertAfterDownload: true,
		Mail:                 true,
	})
	if err != nil {
		t.Fatalf("Download returned error: %v", err)
	}
	if !reflect.DeepEqual(client.downloadTargets, []string{"https://example.test/novel"}) ||
		!client.downloadForce ||
		!client.downloadConvertAfterDownload ||
		!client.downloadMail {
		t.Fatalf("Download should pass targets and options: %+v", client)
	}
	if result.Message != "queued" {
		t.Fatalf("Download should preserve fetcher message: %+v", result)
	}
}

func TestServiceUpdateReportsMissingNovels(t *testing.T) {
	service := NewService(&fakeClient{}, fakeResolver{"novel-a": "12"})

	_, err := service.Update(context.Background(), []string{"novel-a", "missing"}, UpdateOptions{})
	var missing MissingNovelsError
	if !errors.As(err, &missing) {
		t.Fatalf("Update should return MissingNovelsError, got %v", err)
	}
	if !reflect.DeepEqual(missing.NovelIDs, []string{"missing"}) {
		t.Fatalf("unexpected missing novel IDs: %+v", missing.NovelIDs)
	}
	if missing.Error() == "" {
		t.Fatal("MissingNovelsError should have an error message")
	}
	if (InvalidFetcherWorkIDError{WorkID: "bad"}).Error() == "" {
		t.Fatal("InvalidFetcherWorkIDError should have an error message")
	}
}

func TestServiceUpdateReportsUnavailableResolver(t *testing.T) {
	service := NewService(&fakeClient{}, nil)

	_, err := service.Update(context.Background(), []string{"novel-a"}, UpdateOptions{})
	if !errors.Is(err, ErrWorkIDResolverUnavailable) {
		t.Fatalf("Update should reject unavailable resolver, got %v", err)
	}
	if ErrWorkIDResolverUnavailable.Error() == "" {
		t.Fatal("ErrWorkIDResolverUnavailable should have an error message")
	}
}

func TestServiceRejectsInvalidFetcherWorkIDs(t *testing.T) {
	cases := []string{"", "not-number", "0", "-1"}
	for _, workID := range cases {
		client := &fakeClient{}
		service := NewService(client, fakeResolver{"novel-a": workID})

		_, err := service.Update(context.Background(), []string{"novel-a"}, UpdateOptions{})
		var invalid InvalidFetcherWorkIDError
		if !errors.As(err, &invalid) {
			t.Fatalf("Update should reject invalid fetcher work ID %q, got %v", workID, err)
		}
		if client.updateIDs != nil {
			t.Fatalf("Update should not call client with invalid fetcher work ID %q: %+v", workID, client.updateIDs)
		}
	}
}

func TestServiceNormalizesFetcherWorkIDs(t *testing.T) {
	client := &fakeClient{}
	service := NewService(client, fakeResolver{"novel-a": " 012 "})

	result, err := service.Remove(context.Background(), []string{"novel-a"}, true)
	if err != nil {
		t.Fatalf("Remove returned error: %v", err)
	}
	if !reflect.DeepEqual(client.removeIDs, []string{"12"}) {
		t.Fatalf("Remove should pass normalized fetcher work IDs: %+v", client.removeIDs)
	}
	if !reflect.DeepEqual(result.FetcherWorkIDs, []string{"12"}) {
		t.Fatalf("Remove should expose normalized fetcher work IDs: %+v", result)
	}
}

func TestServiceResumeUsesFetcherIDsFromResponse(t *testing.T) {
	client := &fakeClient{resumeResult: fetcher.ResumeResponse{IDs: []string{"12"}}}
	service := NewService(client, fakeResolver{"novel-a": "12"})

	result, err := service.Resume(context.Background(), []string{"novel-a"})
	if err != nil {
		t.Fatalf("Resume returned error: %v", err)
	}
	if !reflect.DeepEqual(client.resumeIDs, []int{12}) {
		t.Fatalf("Resume should pass fetcher work IDs: %+v", client.resumeIDs)
	}
	if result.Message != "Resume started" ||
		!reflect.DeepEqual(result.NovelIDs, []string{"novel-a"}) ||
		!reflect.DeepEqual(result.FetcherWorkIDs, []string{"12"}) {
		t.Fatalf("Resume should decorate response: %+v", result)
	}
}

func TestServiceResumeFallsBackToResolvedFetcherIDs(t *testing.T) {
	client := &fakeClient{}
	service := NewService(client, fakeResolver{"novel-a": "12"})

	result, err := service.Resume(context.Background(), []string{"novel-a"})
	if err != nil {
		t.Fatalf("Resume returned error: %v", err)
	}
	if !reflect.DeepEqual(result.FetcherWorkIDs, []string{"12"}) {
		t.Fatalf("Resume should fall back to resolved fetcher work IDs: %+v", result)
	}
}

func TestServiceRemovePassesFetcherIDsAndWithFiles(t *testing.T) {
	client := &fakeClient{}
	service := NewService(client, fakeResolver{"novel-a": "12"})

	result, err := service.Remove(context.Background(), []string{"novel-a"}, false)
	if err != nil {
		t.Fatalf("Remove returned error: %v", err)
	}
	if !reflect.DeepEqual(client.removeIDs, []string{"12"}) || client.removeWithFiles {
		t.Fatalf("Remove should pass fetcher IDs and withFiles: %+v", client)
	}
	if result.Message != "Novel removal started" ||
		result.WithFiles != false ||
		!reflect.DeepEqual(result.FetcherWorkIDs, []string{"12"}) {
		t.Fatalf("Remove should decorate response: %+v", result)
	}
}

func TestServiceRemovePrunesViewerStateAfterFetcherSuccess(t *testing.T) {
	client := &fakeClient{}
	cleaner := &fakeRemovedNovelStateCleaner{result: removedstate.CleanupResult{BookmarksDeleted: 2}}
	service := NewService(client, fakeResolver{
		"novel-a": "12",
		"novel-b": "34",
	}).WithRemovedNovelStateCleaner(cleaner)

	result, err := service.Remove(context.Background(), []string{"novel-a", "novel-b"}, true)
	if err != nil {
		t.Fatalf("Remove returned error: %v", err)
	}
	if !reflect.DeepEqual(cleaner.novelIDs, []string{"novel-a", "novel-b"}) {
		t.Fatalf("Remove should prune viewer state for viewer novel IDs: %+v", cleaner.novelIDs)
	}
	if result.ViewerStateCleanupStatus != "ok" || result.ViewerStateCleanup == nil || result.ViewerStateCleanup.BookmarksDeleted != 2 {
		t.Fatalf("Remove should expose cleanup result: %+v", result)
	}
}

func TestServiceRemoveKeepsAcceptedWhenViewerStateCleanupFails(t *testing.T) {
	client := &fakeClient{}
	cleaner := &fakeRemovedNovelStateCleaner{
		result: removedstate.CleanupResult{BookmarksDeleted: 1},
		err:    errors.New("cleanup failed"),
	}
	service := NewService(client, fakeResolver{"novel-a": "12"}).WithRemovedNovelStateCleaner(cleaner)

	result, err := service.Remove(context.Background(), []string{"novel-a"}, true)
	if err != nil {
		t.Fatalf("Remove should not fail when cleanup fails after fetcher success: %v", err)
	}
	if result.ViewerStateCleanupStatus != "partial" ||
		result.ViewerStateCleanupError != "Failed to clean up removed novel state." ||
		result.ViewerStateCleanup == nil ||
		result.ViewerStateCleanup.BookmarksDeleted != 1 {
		t.Fatalf("Remove should expose partial cleanup status: %+v", result)
	}
}

func TestServiceRemoveDoesNotPruneViewerStateWhenFetcherFails(t *testing.T) {
	expected := errors.New("client failed")
	client := &fakeClient{removeErr: expected}
	cleaner := &fakeRemovedNovelStateCleaner{}
	service := NewService(client, fakeResolver{"novel-a": "12"}).WithRemovedNovelStateCleaner(cleaner)

	_, err := service.Remove(context.Background(), []string{"novel-a"}, true)
	if !errors.Is(err, expected) {
		t.Fatalf("Remove should propagate client error, got %v", err)
	}
	if cleaner.called {
		t.Fatal("Remove should not prune viewer state when fetcher remove fails")
	}
}

func TestServicePropagatesResolverErrors(t *testing.T) {
	expected := errors.New("resolver failed")
	service := NewService(&fakeClient{}, failingResolver{err: expected})

	_, err := service.Update(context.Background(), []string{"novel-a"}, UpdateOptions{})
	if !errors.Is(err, expected) {
		t.Fatalf("Update should propagate library error, got %v", err)
	}
}

func TestServicePropagatesClientErrors(t *testing.T) {
	expected := errors.New("client failed")
	resolver := fakeResolver{"novel-a": "12"}

	cases := []struct {
		name string
		run  func(*fakeClient) error
	}{
		{
			name: "download",
			run: func(client *fakeClient) error {
				client.downloadErr = expected
				_, err := NewService(client, nil).Download(context.Background(), []string{"target"}, DownloadOptions{})
				return err
			},
		},
		{
			name: "update",
			run: func(client *fakeClient) error {
				client.updateErr = expected
				_, err := NewService(client, resolver).Update(context.Background(), []string{"novel-a"}, UpdateOptions{})
				return err
			},
		},
		{
			name: "resume",
			run: func(client *fakeClient) error {
				client.resumeErr = expected
				_, err := NewService(client, resolver).Resume(context.Background(), []string{"novel-a"})
				return err
			},
		},
		{
			name: "remove",
			run: func(client *fakeClient) error {
				client.removeErr = expected
				_, err := NewService(client, resolver).Remove(context.Background(), []string{"novel-a"}, true)
				return err
			},
		},
		{
			name: "cancel",
			run: func(client *fakeClient) error {
				client.cancelErr = expected
				_, err := NewService(client, nil).CancelTask(context.Background(), "task-1")
				return err
			},
		},
	}
	for _, tc := range cases {
		if err := tc.run(&fakeClient{}); !errors.Is(err, expected) {
			t.Fatalf("%s should propagate client error, got %v", tc.name, err)
		}
	}
}

func TestServiceCancelTaskDecoratesResult(t *testing.T) {
	client := &fakeClient{cancelResult: fetcher.CancelTaskResponse{TaskID: "sidecar-task-1", Cancelled: false}}
	service := NewService(client, nil)

	result, err := service.CancelTask(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("CancelTask returned error: %v", err)
	}
	if client.cancelTaskID != "task-1" {
		t.Fatalf("CancelTask should pass task ID, got %q", client.cancelTaskID)
	}
	if result.Message != "Task cancelled" || result.TaskID != "sidecar-task-1" || result.Cancelled != false {
		t.Fatalf("CancelTask should decorate response: %+v", result)
	}
}

type fakeResolver map[string]string

func (r fakeResolver) FetcherWorkID(novelID string) (string, bool, error) {
	workID, ok := r[novelID]
	return workID, ok, nil
}

type failingResolver struct {
	err error
}

func (r failingResolver) FetcherWorkID(string) (string, bool, error) {
	return "", false, r.err
}

type fakeRemovedNovelStateCleaner struct {
	called   bool
	novelIDs []string
	result   removedstate.CleanupResult
	err      error
}

func (c *fakeRemovedNovelStateCleaner) PruneRemovedNovelState(novelIDs []string) (removedstate.CleanupResult, error) {
	c.called = true
	c.novelIDs = append([]string{}, novelIDs...)
	return c.result, c.err
}

type fakeClient struct {
	downloadTargets              []string
	downloadForce                bool
	downloadConvertAfterDownload bool
	downloadMail                 bool
	downloadResult               fetcher.DownloadResponse
	downloadErr                  error

	updateIDs                []int
	updateResult             fetcher.UpdateResponse
	updateErr                error
	updateForceRedownload    bool
	updateIncludeFrozen      bool
	updateConvertAfterUpdate bool
	updateSkipUnchanged      bool

	resumeIDs       []int
	resumeResult    fetcher.ResumeResponse
	resumeErr       error
	removeIDs       []string
	removeWithFiles bool
	removeResult    fetcher.RemoveResponse
	removeErr       error

	cancelTaskID string
	cancelResult fetcher.CancelTaskResponse
	cancelErr    error
}

func (c *fakeClient) Download(_ context.Context, targets []string, force bool, convertAfterDownload bool, mail bool) (fetcher.DownloadResponse, error) {
	if c.downloadErr != nil {
		return fetcher.DownloadResponse{}, c.downloadErr
	}
	c.downloadTargets = append([]string{}, targets...)
	c.downloadForce = force
	c.downloadConvertAfterDownload = convertAfterDownload
	c.downloadMail = mail
	return c.downloadResult, nil
}

func (c *fakeClient) Update(_ context.Context, ids []int, forceRedownload bool, includeFrozen bool, convertAfterUpdate bool, skipUnchanged bool) (fetcher.UpdateResponse, error) {
	if c.updateErr != nil {
		return fetcher.UpdateResponse{}, c.updateErr
	}
	c.updateIDs = append([]int{}, ids...)
	c.updateForceRedownload = forceRedownload
	c.updateIncludeFrozen = includeFrozen
	c.updateConvertAfterUpdate = convertAfterUpdate
	c.updateSkipUnchanged = skipUnchanged
	return c.updateResult, nil
}

func (c *fakeClient) Resume(_ context.Context, ids []int) (fetcher.ResumeResponse, error) {
	if c.resumeErr != nil {
		return fetcher.ResumeResponse{}, c.resumeErr
	}
	c.resumeIDs = append([]int{}, ids...)
	return c.resumeResult, nil
}

func (c *fakeClient) Remove(_ context.Context, ids []string, withFiles bool) (fetcher.RemoveResponse, error) {
	if c.removeErr != nil {
		return fetcher.RemoveResponse{}, c.removeErr
	}
	c.removeIDs = append([]string{}, ids...)
	c.removeWithFiles = withFiles
	return c.removeResult, nil
}

func (c *fakeClient) CancelTask(_ context.Context, taskID string) (fetcher.CancelTaskResponse, error) {
	if c.cancelErr != nil {
		return fetcher.CancelTaskResponse{}, c.cancelErr
	}
	c.cancelTaskID = taskID
	return c.cancelResult, nil
}
