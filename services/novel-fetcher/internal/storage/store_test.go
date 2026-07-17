package storage

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"narou-viewer/services/novel-fetcher/internal/fetcher"
	"narou-viewer/services/novel-fetcher/internal/model"
	"narou-viewer/services/novel-fetcher/internal/storage/migration"
	"narou-viewer/services/novel-fetcher/internal/taskstate"
)

type fakeAssetFetcher struct {
	bytes       []byte
	contentType string
	err         error
	urls        []string
}

func TestUnsupportedEpisodeSchemaErrorIncludesRecoveryDetails(t *testing.T) {
	observed := 99
	err := ErrUnsupportedEpisodeSchema{Path: "episodes/1.json", Observed: &observed, Supported: 1}
	message := err.Error()
	if !strings.Contains(message, "episodes/1.json") || !strings.Contains(message, "observed 99") || !strings.Contains(message, "supported 1") {
		t.Fatalf("error message = %q", message)
	}
}

func (f *fakeAssetFetcher) FetchBytes(_ context.Context, rawURL string, _ fetcher.FetchPolicy) (fetcher.BinaryResponse, error) {
	f.urls = append(f.urls, rawURL)
	if f.err != nil {
		return fetcher.BinaryResponse{}, f.err
	}
	return fetcher.BinaryResponse{
		Bytes:       f.bytes,
		ContentType: f.contentType,
	}, nil
}

func saveWorkFully(t *testing.T, store *Store, work model.Work) (model.StoredWork, error) {
	t.Helper()

	if existing, ok, err := store.FindWorkBySiteKey(string(work.Site), work.SiteWorkID); err != nil {
		return model.StoredWork{}, err
	} else if ok {
		if _, err := store.db.Exec(`DELETE FROM assets WHERE work_id = ?`, existing.ID); err != nil {
			return model.StoredWork{}, err
		}
		if _, err := store.db.Exec(`DELETE FROM episodes WHERE work_id = ?`, existing.ID); err != nil {
			return model.StoredWork{}, err
		}
		if existing.Directory != "" {
			if err := os.RemoveAll(filepath.Join(store.rootDir, existing.Directory)); err != nil {
				return model.StoredWork{}, err
			}
		}
	}

	ctx := context.Background()
	stored, err := store.UpsertWorkToc(ctx, work, FetchStatusPartial)
	if err != nil {
		return model.StoredWork{}, err
	}
	for index, episode := range work.Episodes {
		if _, err := store.SaveEpisodeBody(ctx, work, stored, episode, index); err != nil {
			return model.StoredWork{}, err
		}
	}
	if err := store.UpdateWorkFetchStatus(ctx, stored.ID, FetchStatusComplete, "", "", nil); err != nil {
		return model.StoredWork{}, err
	}
	stored, ok, err := store.FindWorkByID(stored.ID)
	if err != nil {
		return model.StoredWork{}, err
	}
	if !ok {
		return model.StoredWork{}, os.ErrNotExist
	}
	return stored, nil
}

func TestStoreSavesSQLiteAndCanonicalEpisode(t *testing.T) {
	rootDir := t.TempDir()
	store, err := NewStore(rootDir)
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	defer store.Close()

	fetchedAt := time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)
	stored, err := saveWorkFully(t, store, model.Work{
		Site:       model.SiteSyosetu,
		SiteName:   "小説家になろう",
		SiteWorkID: "n1234ab",
		SourceURL:  "https://ncode.syosetu.com/n1234ab/",
		Title:      "テスト/作品",
		Author:     "作者",
		Story:      "あらすじ",
		FetchedAt:  fetchedAt,
		Episodes: []model.Episode{
			{
				Index:        "1",
				Href:         "/n1234ab/1/",
				Title:        "第一話",
				FileSubtitle: "第一話",
				Chapter:      "第一章",
				PublishedAt:  "2026/05/09 12:00",
				FetchedAt:    time.Date(2026, 5, 9, 12, 1, 0, 0, time.UTC),
				Element: model.EpisodeElement{
					DataType: "html",
					Body:     "<p>本文</p>",
				},
				RawHTML: "<html>raw</html>",
			},
		},
	})
	if err != nil {
		t.Fatalf("逐次保存 helper returned error: %v", err)
	}
	if stored.ID != 1 {
		t.Fatalf("stored.ID = %d, want 1", stored.ID)
	}
	if stored.Directory != "works/syosetu/n1234ab" {
		t.Fatalf("stored.Directory = %q", stored.Directory)
	}

	works, err := store.ListWorks()
	if err != nil {
		t.Fatalf("ListWorks returned error: %v", err)
	}
	if len(works) != 1 || works[0].Title != "テスト/作品" || works[0].EpisodeLen != 1 {
		t.Fatalf("unexpected works: %#v", works)
	}

	episodes, err := store.ListEpisodes(stored.ID)
	if err != nil {
		t.Fatalf("ListEpisodes returned error: %v", err)
	}
	if len(episodes) != 1 || episodes[0].EpisodeID != "1" || episodes[0].Title != "第一話" {
		t.Fatalf("unexpected episodes: %#v", episodes)
	}
	if episodes[0].BodyPath != "works/syosetu/n1234ab/episodes/1.json" {
		t.Fatalf("BodyPath = %q", episodes[0].BodyPath)
	}
	if episodes[0].RawPath != "works/syosetu/n1234ab/raw/episodes/1.html" {
		t.Fatalf("RawPath = %q", episodes[0].RawPath)
	}
	if episodes[0].SourceURL != "https://ncode.syosetu.com/n1234ab/1/" {
		t.Fatalf("SourceURL = %q", episodes[0].SourceURL)
	}

	canonical, err := store.ReadCanonicalEpisode(episodes[0])
	if err != nil {
		t.Fatalf("ReadCanonicalEpisode returned error: %v", err)
	}
	if canonical.Title != "第一話" || canonical.Chapter != "第一章" || canonical.Blocks[len(canonical.Blocks)-1].HTML != "<p>本文</p>" {
		t.Fatalf("unexpected canonical episode: %#v", canonical)
	}
	if canonical.SourceURL != "https://ncode.syosetu.com/n1234ab/1/" {
		t.Fatalf("canonical.SourceURL = %q", canonical.SourceURL)
	}

	canonicalBytes, err := os.ReadFile(filepath.Join(rootDir, episodes[0].BodyPath))
	if err != nil {
		t.Fatalf("canonical episode read failed: %v", err)
	}
	if !strings.Contains(string(canonicalBytes), `"<p>本文</p>"`) {
		t.Fatalf("canonical episode should keep HTML readable: %s", canonicalBytes)
	}
	if strings.Contains(string(canonicalBytes), `\u003c`) || strings.Contains(string(canonicalBytes), `\u003e`) {
		t.Fatalf("canonical episode should not HTML-escape angle brackets: %s", canonicalBytes)
	}

	if _, err := os.Stat(filepath.Join(rootDir, "library.sqlite")); err != nil {
		t.Fatalf("library.sqlite was not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(rootDir, episodes[0].BodyPath)); err != nil {
		t.Fatalf("canonical episode was not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(rootDir, episodes[0].RawPath)); err != nil {
		t.Fatalf("raw html was not written: %v", err)
	}
}

func TestCompleteWorkForTaskDoesNotCrossAcceptedControl(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	work, err := store.UpsertWorkToc(context.Background(), model.Work{
		Site:       model.SiteSyosetu,
		SiteName:   "小説家になろう",
		SiteWorkID: "n4321ab",
		SourceURL:  "https://example.com/control-fence",
		Title:      "制御境界テスト",
		Author:     "作者",
		FetchedAt:  time.Now(),
	}, FetchStatusPartial)
	if err != nil {
		t.Fatal(err)
	}
	repository := taskstate.NewSQLiteRepository(store.DB())
	task := taskstate.NewTask("update")
	task.NovelIDs = []int{work.ID}
	if _, err := repository.Enqueue(context.Background(), []*taskstate.Task{task}); err != nil {
		t.Fatal(err)
	}
	claimed, err := repository.ClaimNext(context.Background(), time.Now())
	if err != nil || claimed == nil {
		t.Fatalf("claim = %#v, err = %v", claimed, err)
	}
	if result, err := repository.RequestPause(context.Background(), task.ID); err != nil || !result.Changed {
		t.Fatalf("pause = %#v, err = %v", result, err)
	}
	ref := taskstate.TaskRef{TaskID: task.ID, Attempt: claimed.AttemptCount}
	if err := store.CompleteWorkForTask(context.Background(), ref, work.ID); !errors.Is(err, taskstate.ErrStaleTaskAttempt) {
		t.Fatalf("completion after control error = %v", err)
	}
	storedTask, found, err := repository.Get(context.Background(), task.ID)
	if err != nil || !found || storedTask.ExecutionCommitted || storedTask.RequestedAction != taskstate.RequestedActionPause {
		t.Fatalf("task after rejected completion = %#v, found = %v, err = %v", storedTask, found, err)
	}
	storedWork, found, err := store.FindWorkByID(work.ID)
	if err != nil || !found || storedWork.FetchStatus != FetchStatusPartial {
		t.Fatalf("work after rejected completion = %#v, found = %v, err = %v", storedWork, found, err)
	}
}

func TestTaskEpisodeCheckpointRequiresMatchingCanonicalBody(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	work := model.Work{
		Site:       model.SiteSyosetu,
		SiteName:   "小説家になろう",
		SiteWorkID: "ncheckpoint",
		SourceURL:  "https://ncode.syosetu.com/ncheckpoint/",
		Title:      "チェックポイント作品",
		Author:     "作者",
		Story:      "あらすじ",
		Episodes: []model.Episode{{
			Index:       "1",
			Title:       "第一話",
			PublishedAt: "2026/05/09 12:00",
			ModifiedAt:  "2026/05/09 12:00",
			FetchedAt:   time.Date(2026, 5, 9, 12, 1, 0, 0, time.UTC),
			Element:     model.EpisodeElement{DataType: "html", Body: "<p>本文</p>"},
		}},
	}
	stored, err := store.UpsertWorkToc(context.Background(), work, FetchStatusPartial)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveEpisodeBody(context.Background(), work, stored, work.Episodes[0], 0); err != nil {
		t.Fatal(err)
	}
	repository := taskstate.NewSQLiteRepository(store.DB())
	task := taskstate.NewTask("update")
	task.NovelIDs = []int{stored.ID}
	if _, err := repository.Enqueue(context.Background(), []*taskstate.Task{task}); err != nil {
		t.Fatal(err)
	}
	claimed, err := repository.ClaimNext(context.Background(), time.Now().UTC())
	if err != nil || claimed == nil {
		t.Fatalf("ClaimNext() = %#v, err = %v", claimed, err)
	}
	ref := taskstate.TaskRef{TaskID: task.ID, Attempt: claimed.AttemptCount}
	if err := store.RecordTaskEpisodeCheckpoint(context.Background(), ref, stored.ID, "missing", 0, ""); err == nil {
		t.Fatal("missing episode checkpoint unexpectedly succeeded")
	}
	valid, _, err := store.IsTaskEpisodeCheckpointValid(context.Background(), ref, work, stored, work.Episodes[0], 0)
	if err != nil || valid {
		t.Fatalf("checkpoint before record = %v, err = %v", valid, err)
	}
	if err := store.RecordTaskEpisodeCheckpoint(context.Background(), ref, stored.ID, "1", 0, ""); err != nil {
		t.Fatal(err)
	}
	valid, _, err = store.IsTaskEpisodeCheckpointValid(context.Background(), ref, work, stored, work.Episodes[0], 0)
	if err != nil || !valid {
		t.Fatalf("checkpoint after record = %v, err = %v", valid, err)
	}
	newRevision := work.Episodes[0]
	newRevision.ModifiedAt = "2026/05/10 12:00"
	valid, _, err = store.IsTaskEpisodeCheckpointValid(context.Background(), ref, work, stored, newRevision, 0)
	if err != nil || valid {
		t.Fatalf("checkpoint for older source revision = %v, err = %v", valid, err)
	}
	updatedEpisode := work.Episodes[0]
	updatedEpisode.Element.Body = "<p>更新本文</p>"
	if _, err := store.SaveEpisodeBodyForTask(context.Background(), ref, work, stored, updatedEpisode, 0, ""); err != nil {
		t.Fatalf("SaveEpisodeBodyForTask() error = %v", err)
	}
	valid, _, err = store.IsTaskEpisodeCheckpointValid(context.Background(), ref, work, stored, updatedEpisode, 0)
	if err != nil || !valid {
		t.Fatalf("checkpoint after atomic save = %v, err = %v", valid, err)
	}
	if err := store.CompleteWorkForTask(context.Background(), ref, stored.ID); err != nil {
		t.Fatalf("CompleteWorkForTask() error = %v", err)
	}
	completedTask, found, err := repository.Get(context.Background(), task.ID)
	if err != nil || !found || !completedTask.ExecutionCommitted {
		t.Fatalf("completed task = %#v, found = %v, err = %v", completedTask, found, err)
	}
	if err := repository.Finalize(context.Background(), ref, taskstate.Outcome{Status: taskstate.StatusSucceeded, ExecutionCommitted: true}); err != nil {
		t.Fatalf("Finalize() error = %v", err)
	}
	if err := store.CompleteWorkForTask(context.Background(), ref, stored.ID); !errors.Is(err, taskstate.ErrStaleTaskAttempt) {
		t.Fatalf("stale CompleteWorkForTask() error = %v", err)
	}

	episode, found, err := store.FindEpisode(stored.ID, "1")
	if err != nil || !found {
		t.Fatalf("FindEpisode() = %#v/%v/%v", episode, found, err)
	}
	if err := os.WriteFile(filepath.Join(store.rootDir, episode.BodyPath), []byte(`{"schema_version":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	valid, _, err = store.IsTaskEpisodeCheckpointValid(context.Background(), ref, work, stored, updatedEpisode, 0)
	if err != nil || valid {
		t.Fatalf("checkpoint after body mutation = %v, err = %v", valid, err)
	}
	if err := os.Remove(filepath.Join(store.rootDir, episode.BodyPath)); err != nil {
		t.Fatal(err)
	}
	valid, _, err = store.IsTaskEpisodeCheckpointValid(context.Background(), ref, work, stored, updatedEpisode, 0)
	if err != nil || valid {
		t.Fatalf("checkpoint after body removal = %v, err = %v", valid, err)
	}
	if err := os.WriteFile(filepath.Join(store.rootDir, episode.BodyPath), []byte(`{"schema_version":99}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.IsTaskEpisodeCheckpointValid(context.Background(), ref, work, stored, updatedEpisode, 0); err == nil {
		t.Fatal("future episode schema was accepted")
	}
}

func TestReadCanonicalEpisodeValidatesSchemaFixtureBeforeTypedDecode(t *testing.T) {
	tests := []struct {
		name         string
		fixture      string
		wantObserved *int
		wantTitle    string
	}{
		{
			name:      "current v1",
			fixture:   "canonical_episode_v1.json",
			wantTitle: "Synthetic current episode",
		},
		{
			name:         "future v99",
			fixture:      "canonical_episode_v99.json",
			wantObserved: intPointer(99),
		},
		{
			name:    "missing version",
			fixture: "canonical_episode_missing_version.json",
		},
	}

	store := &Store{rootDir: "."}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			document, err := store.ReadCanonicalEpisode(model.StoredEpisode{
				BodyPath: filepath.Join("testdata", test.fixture),
			})
			if test.wantTitle != "" {
				if err != nil {
					t.Fatalf("ReadCanonicalEpisode returned error: %v", err)
				}
				if document.Title != test.wantTitle || document.SchemaVersion != canonicalEpisodeSchemaVersion {
					t.Fatalf("document = %#v", document)
				}
				return
			}

			var unsupported ErrUnsupportedEpisodeSchema
			if !errors.As(err, &unsupported) {
				t.Fatalf("ReadCanonicalEpisode error = %v, want ErrUnsupportedEpisodeSchema", err)
			}
			if unsupported.Supported != canonicalEpisodeSchemaVersion {
				t.Fatalf("supported version = %d", unsupported.Supported)
			}
			if test.wantObserved == nil {
				if unsupported.Observed != nil {
					t.Fatalf("observed version = %v, want missing", *unsupported.Observed)
				}
				return
			}
			if unsupported.Observed == nil || *unsupported.Observed != *test.wantObserved {
				t.Fatalf("observed version = %v, want %d", unsupported.Observed, *test.wantObserved)
			}
		})
	}
}

func TestSaveEpisodeBodyDoesNotOverwriteFutureCanonicalEpisode(t *testing.T) {
	rootDir := t.TempDir()
	store, err := NewStore(rootDir)
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	defer store.Close()

	work := model.Work{
		Site:       model.SiteVerification,
		SiteName:   "Verification",
		SiteWorkID: "future-schema-work",
		SourceURL:  "https://example.invalid/future-schema-work/",
		Title:      "Synthetic schema guard work",
		Author:     "Synthetic author",
		FetchedAt:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Episodes: []model.Episode{{
			Index:     "1",
			Title:     "Synthetic episode",
			FetchedAt: time.Date(2026, 1, 1, 0, 1, 0, 0, time.UTC),
			Element: model.EpisodeElement{
				DataType: "html",
				Body:     "<p>Synthetic current body.</p>",
			},
		}},
	}
	stored, err := saveWorkFully(t, store, work)
	if err != nil {
		t.Fatalf("saveWorkFully returned error: %v", err)
	}
	episodes, err := store.ListEpisodes(stored.ID)
	if err != nil || len(episodes) != 1 {
		t.Fatalf("ListEpisodes = %#v, %v", episodes, err)
	}

	futureBytes, err := os.ReadFile(filepath.Join("testdata", "canonical_episode_v99.json"))
	if err != nil {
		t.Fatalf("read future fixture: %v", err)
	}
	canonicalPath := filepath.Join(rootDir, episodes[0].BodyPath)
	if err := os.WriteFile(canonicalPath, futureBytes, 0o644); err != nil {
		t.Fatalf("seed future canonical episode: %v", err)
	}
	contentHashBefore := episodes[0].ContentHash

	work.Episodes[0].Element.Body = "<p>This update must be rejected.</p>"
	_, err = store.SaveEpisodeBody(context.Background(), work, stored, work.Episodes[0], 0)
	var unsupported ErrUnsupportedEpisodeSchema
	if !errors.As(err, &unsupported) {
		t.Fatalf("SaveEpisodeBody error = %v, want ErrUnsupportedEpisodeSchema", err)
	}

	afterBytes, err := os.ReadFile(canonicalPath)
	if err != nil {
		t.Fatalf("read canonical episode after rejected save: %v", err)
	}
	if !bytes.Equal(afterBytes, futureBytes) {
		t.Fatal("future canonical episode bytes changed after rejected save")
	}
	updatedEpisodes, err := store.ListEpisodes(stored.ID)
	if err != nil || len(updatedEpisodes) != 1 {
		t.Fatalf("ListEpisodes after rejected save = %#v, %v", updatedEpisodes, err)
	}
	if updatedEpisodes[0].ContentHash != contentHashBefore {
		t.Fatalf("content hash changed from %q to %q", contentHashBefore, updatedEpisodes[0].ContentHash)
	}
}

func TestUpsertWorkTocRejectsFutureCanonicalEpisodeBeforeMetadataOrPrune(t *testing.T) {
	rootDir := t.TempDir()
	store, err := NewStore(rootDir)
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	defer store.Close()

	work := model.Work{
		Site:       model.SiteVerification,
		SiteName:   "Verification",
		SiteWorkID: "future-upsert-work",
		SourceURL:  "https://example.invalid/future-upsert-work/",
		Title:      "Synthetic original work",
		Author:     "Synthetic author",
		FetchedAt:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Episodes: []model.Episode{
			{
				Index:     "1",
				Title:     "Synthetic retained episode",
				FetchedAt: time.Date(2026, 1, 1, 0, 1, 0, 0, time.UTC),
				Element:   model.EpisodeElement{DataType: "html", Body: "<p>Synthetic retained body.</p>"},
			},
			{
				Index:     "2",
				Title:     "Synthetic pruned episode",
				FetchedAt: time.Date(2026, 1, 1, 0, 2, 0, 0, time.UTC),
				Element:   model.EpisodeElement{DataType: "html", Body: "<p>Synthetic pruned body.</p>"},
				RawHTML:   "<html><body>Synthetic raw fixture.</body></html>",
			},
		},
	}
	stored, err := saveWorkFully(t, store, work)
	if err != nil {
		t.Fatalf("saveWorkFully returned error: %v", err)
	}
	beforeWork, found, err := store.FindWorkByID(stored.ID)
	if err != nil || !found {
		t.Fatalf("FindWorkByID before upsert = %#v/%v/%v", beforeWork, found, err)
	}
	beforeEpisodes, err := store.ListEpisodes(stored.ID)
	if err != nil || len(beforeEpisodes) != 2 {
		t.Fatalf("ListEpisodes before upsert = %#v/%v", beforeEpisodes, err)
	}

	futureBytes, err := os.ReadFile(filepath.Join("testdata", "canonical_episode_v99.json"))
	if err != nil {
		t.Fatalf("read future fixture: %v", err)
	}
	futurePath := filepath.Join(rootDir, beforeEpisodes[1].BodyPath)
	if err := os.WriteFile(futurePath, futureBytes, 0o644); err != nil {
		t.Fatalf("seed future canonical episode: %v", err)
	}

	incoming := work
	incoming.Title = "Synthetic updated work"
	incoming.FetchedAt = time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	incoming.Episodes = append([]model.Episode(nil), work.Episodes[:1]...)
	_, err = store.UpsertWorkToc(context.Background(), incoming, FetchStatusPartial)
	var unsupported ErrUnsupportedEpisodeSchema
	if !errors.As(err, &unsupported) {
		t.Fatalf("UpsertWorkToc error = %v, want ErrUnsupportedEpisodeSchema", err)
	}

	afterWork, found, err := store.FindWorkByID(stored.ID)
	if err != nil || !found {
		t.Fatalf("FindWorkByID after upsert = %#v/%v/%v", afterWork, found, err)
	}
	afterEpisodes, err := store.ListEpisodes(stored.ID)
	if err != nil {
		t.Fatalf("ListEpisodes after upsert returned error: %v", err)
	}
	if !reflect.DeepEqual(afterWork, beforeWork) || !reflect.DeepEqual(afterEpisodes, beforeEpisodes) {
		t.Fatalf("storage metadata changed:\nwork before=%#v\nwork after=%#v\nepisodes before=%#v\nepisodes after=%#v", beforeWork, afterWork, beforeEpisodes, afterEpisodes)
	}
	afterBytes, err := os.ReadFile(futurePath)
	if err != nil {
		t.Fatalf("read future episode after rejected upsert: %v", err)
	}
	if !bytes.Equal(afterBytes, futureBytes) {
		t.Fatal("future canonical bytes changed during rejected upsert")
	}
	if _, err := os.Stat(filepath.Join(rootDir, beforeEpisodes[1].RawPath)); err != nil {
		t.Fatalf("raw episode was pruned during rejected upsert: %v", err)
	}
}

func TestUpsertWorkTocRejectsFutureCanonicalTargetBeforeNewWorkInsert(t *testing.T) {
	rootDir := t.TempDir()
	store, err := NewStore(rootDir)
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	defer store.Close()

	work := model.Work{
		Site:       model.SiteVerification,
		SiteName:   "Verification",
		SiteWorkID: "orphan-target-work",
		SourceURL:  "https://example.invalid/orphan-target-work/",
		Title:      "Synthetic new work",
		Episodes: []model.Episode{{
			Index: "1",
			Title: "Synthetic episode",
		}},
	}
	futureBytes, err := os.ReadFile(filepath.Join("testdata", "canonical_episode_v99.json"))
	if err != nil {
		t.Fatalf("read future fixture: %v", err)
	}
	targetPath := filepath.Join(rootDir, "works", "verification", "orphan-target-work", "episodes", "1.json")
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		t.Fatalf("create orphan target directory: %v", err)
	}
	if err := os.WriteFile(targetPath, futureBytes, 0o644); err != nil {
		t.Fatalf("seed future target: %v", err)
	}

	_, err = store.UpsertWorkToc(context.Background(), work, FetchStatusPartial)
	var unsupported ErrUnsupportedEpisodeSchema
	if !errors.As(err, &unsupported) {
		t.Fatalf("UpsertWorkToc error = %v, want ErrUnsupportedEpisodeSchema", err)
	}
	works, err := store.ListWorks()
	if err != nil {
		t.Fatalf("ListWorks returned error: %v", err)
	}
	if len(works) != 0 {
		t.Fatalf("new work metadata was inserted before target preflight: %#v", works)
	}
	afterBytes, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("read future target after rejected upsert: %v", err)
	}
	if !bytes.Equal(afterBytes, futureBytes) {
		t.Fatal("future target bytes changed during rejected upsert")
	}
}

func TestNewStoreDoesNotModifyFutureLibraryDatabase(t *testing.T) {
	rootDir := t.TempDir()
	store, err := NewStore(rootDir)
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	if _, err := store.db.Exec(`INSERT INTO schema_migrations(version) VALUES (99)`); err != nil {
		t.Fatalf("seed future migration: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close seeded store: %v", err)
	}

	databasePath := filepath.Join(rootDir, "library.sqlite")
	beforeBytes, err := os.ReadFile(databasePath)
	if err != nil {
		t.Fatalf("read database before guarded open: %v", err)
	}

	_, err = NewStore(rootDir)
	var futureSchema migration.ErrFutureSchema
	if !errors.As(err, &futureSchema) {
		t.Fatalf("NewStore error = %v, want migration.ErrFutureSchema", err)
	}
	if futureSchema.Path != databasePath || futureSchema.Observed != 99 || futureSchema.Supported != migration.SupportedLatestVersion {
		t.Fatalf("future schema error = %#v", futureSchema)
	}

	afterBytes, err := os.ReadFile(databasePath)
	if err != nil {
		t.Fatalf("read database after guarded open: %v", err)
	}
	if !bytes.Equal(afterBytes, beforeBytes) {
		t.Fatal("future library database bytes changed during guarded open")
	}
}

func TestNewStoreRejectsFutureMigrationStoredInUncheckpointedWAL(t *testing.T) {
	rootDir := t.TempDir()
	databasePath := filepath.Join(rootDir, "library.sqlite")
	seedDB, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatalf("open seed database: %v", err)
	}
	defer seedDB.Close()

	if _, err := seedDB.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		t.Fatalf("enable WAL: %v", err)
	}
	if _, err := seedDB.Exec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY)`); err != nil {
		t.Fatalf("create schema_migrations: %v", err)
	}
	if _, err := seedDB.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		t.Fatalf("checkpoint base schema: %v", err)
	}
	if _, err := seedDB.Exec(`PRAGMA wal_autocheckpoint=0`); err != nil {
		t.Fatalf("disable automatic checkpoint: %v", err)
	}
	if _, err := seedDB.Exec(`INSERT INTO schema_migrations(version) VALUES (99)`); err != nil {
		t.Fatalf("seed future migration in WAL: %v", err)
	}
	walInfo, err := os.Stat(databasePath + "-wal")
	if err != nil || walInfo.Size() == 0 {
		t.Fatalf("future migration was not retained in WAL: info=%v err=%v", walInfo, err)
	}

	_, err = NewStore(rootDir)
	var futureSchema migration.ErrFutureSchema
	if !errors.As(err, &futureSchema) {
		t.Fatalf("NewStore error = %v, want migration.ErrFutureSchema", err)
	}
	if futureSchema.Observed != 99 || futureSchema.Supported != migration.SupportedLatestVersion {
		t.Fatalf("future schema error = %#v", futureSchema)
	}

	var observed int
	if err := seedDB.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&observed); err != nil {
		t.Fatalf("query future migration after rejected open: %v", err)
	}
	if observed != 99 {
		t.Fatalf("future migration version = %d, want 99", observed)
	}
	var worksTableCount int
	if err := seedDB.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'works'`).Scan(&worksTableCount); err != nil {
		t.Fatalf("query works table: %v", err)
	}
	if worksTableCount != 0 {
		t.Fatal("known migrations ran after the read-only WAL preflight")
	}
}

func intPointer(value int) *int {
	return &value
}

func TestNewStoreReturnsErrorWhenRootIsFile(t *testing.T) {
	rootPath := filepath.Join(t.TempDir(), "library-root-file")
	if err := os.WriteFile(rootPath, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	if store, err := NewStore(rootPath); err == nil {
		_ = store.Close()
		t.Fatal("NewStore returned nil error for file root")
	}
}

func TestSnapshotAndRestoreFileRollbacks(t *testing.T) {
	rootDir := t.TempDir()
	existingPath := filepath.Join(rootDir, "existing.txt")
	missingPath := filepath.Join(rootDir, "missing.txt")
	if err := os.WriteFile(existingPath, []byte("before"), 0o644); err != nil {
		t.Fatalf("WriteFile existing returned error: %v", err)
	}

	existingSnapshot, err := snapshotFile(existingPath)
	if err != nil {
		t.Fatalf("snapshotFile existing returned error: %v", err)
	}
	missingSnapshot, err := snapshotFile(missingPath)
	if err != nil {
		t.Fatalf("snapshotFile missing returned error: %v", err)
	}
	if !existingSnapshot.exists || missingSnapshot.exists {
		t.Fatalf("snapshot existence = %v/%v", existingSnapshot.exists, missingSnapshot.exists)
	}

	if err := os.WriteFile(existingPath, []byte("after"), 0o644); err != nil {
		t.Fatalf("WriteFile updated returned error: %v", err)
	}
	if err := os.WriteFile(missingPath, []byte("created"), 0o644); err != nil {
		t.Fatalf("WriteFile missing returned error: %v", err)
	}

	restoreFileRollbacks([]fileRollbackEntry{existingSnapshot, missingSnapshot})
	existingContent, err := os.ReadFile(existingPath)
	if err != nil {
		t.Fatalf("ReadFile existing returned error: %v", err)
	}
	if string(existingContent) != "before" {
		t.Fatalf("existing content = %q, want before", existingContent)
	}
	if _, err := os.Stat(missingPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing file stat error = %v, want not exist", err)
	}
}

func TestRemoveRelativeFilesSkipsBlankDuplicateAndMissingPaths(t *testing.T) {
	rootDir := t.TempDir()
	store, err := NewStore(rootDir)
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	defer store.Close()

	if err := os.WriteFile(filepath.Join(rootDir, "stale.txt"), []byte("stale"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	if err := store.removeRelativeFiles([]string{"", "stale.txt", "stale.txt", "missing.txt"}); err != nil {
		t.Fatalf("removeRelativeFiles returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(rootDir, "stale.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale file stat error = %v, want not exist", err)
	}
}

func TestUpsertWorkTocDeletesEpisodesMissingFromReplacementToc(t *testing.T) {
	rootDir := t.TempDir()
	store, err := NewStore(rootDir)
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	defer store.Close()

	work := model.Work{
		Site:       model.SiteSyosetu,
		SiteName:   "小説家になろう",
		SiteWorkID: "n1234ab",
		SourceURL:  "https://ncode.syosetu.com/n1234ab/",
		Title:      "差し替え作品",
		Episodes: []model.Episode{
			{Index: "1", Href: "/n1234ab/1/", Title: "第一話", Element: model.EpisodeElement{DataType: "html", Body: "<p>一</p>"}},
			{Index: "2", Href: "/n1234ab/2/", Title: "第二話", RawHTML: "<p>raw</p>", Element: model.EpisodeElement{DataType: "html", Body: "<p>二</p>"}},
		},
	}
	stored, err := saveWorkFully(t, store, work)
	if err != nil {
		t.Fatalf("saveWorkFully returned error: %v", err)
	}
	episodeTwo, ok, err := store.FindEpisode(stored.ID, "2")
	if err != nil || !ok {
		t.Fatalf("FindEpisode(2) = %#v/%v/%v", episodeTwo, ok, err)
	}
	if _, err := os.Stat(filepath.Join(rootDir, episodeTwo.BodyPath)); err != nil {
		t.Fatalf("episode 2 body should exist before replacement: %v", err)
	}

	work.Episodes = work.Episodes[:1]
	if _, err := store.UpsertWorkToc(context.Background(), work, FetchStatusPartial); err != nil {
		t.Fatalf("UpsertWorkToc replacement returned error: %v", err)
	}
	episodes, err := store.ListEpisodes(stored.ID)
	if err != nil {
		t.Fatalf("ListEpisodes returned error: %v", err)
	}
	if len(episodes) != 1 || episodes[0].EpisodeID != "1" {
		t.Fatalf("episodes after replacement = %#v", episodes)
	}
	if _, err := os.Stat(filepath.Join(rootDir, episodeTwo.BodyPath)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("episode 2 body should be removed, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(rootDir, episodeTwo.RawPath)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("episode 2 raw should be removed, stat err = %v", err)
	}
}

func TestStoreFindsWorksByNormalizedTitle(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	defer store.Close()

	for _, work := range []model.Work{
		{
			Site:       model.SiteSyosetu,
			SiteName:   "小説家になろう",
			SiteWorkID: "n1234ab",
			SourceURL:  "https://ncode.syosetu.com/n1234ab/",
			Title:      "同名 作品",
			Episodes:   []model.Episode{{Index: "1", Title: "第一話"}},
		},
		{
			Site:       model.SiteKakuyomu,
			SiteName:   "カクヨム",
			SiteWorkID: "0000000000000000000",
			SourceURL:  "https://kakuyomu.jp/works/0000000000000000000",
			Title:      "別作品",
			Episodes:   []model.Episode{{Index: "1", Title: "第一話"}},
		},
	} {
		if _, err := store.UpsertWorkToc(context.Background(), work, FetchStatusPartial); err != nil {
			t.Fatalf("UpsertWorkToc returned error: %v", err)
		}
	}

	matches, err := store.FindPotentialDuplicateWorks(model.Work{Title: " 同名\t作品 "})
	if err != nil {
		t.Fatalf("FindPotentialDuplicateWorks returned error: %v", err)
	}
	if len(matches) != 1 || matches[0].Site != model.SiteSyosetu || matches[0].Title != "同名 作品" {
		t.Fatalf("matches = %#v", matches)
	}
}

func TestStoreFindsWorksBySimilarCrossSiteTitle(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	defer store.Close()

	if _, err := store.UpsertWorkToc(context.Background(), model.Work{
		Site:       model.SiteKakuyomu,
		SiteName:   "カクヨム",
		SiteWorkID: "16818093082596316543",
		SourceURL:  "https://kakuyomu.jp/works/16818093082596316543",
		Title:      "配信スローライフをしてたら、相方のゴーレムがアップをはじめたようです",
		Episodes:   []model.Episode{{Index: "1", Title: "第一話"}},
	}, FetchStatusPartial); err != nil {
		t.Fatalf("UpsertWorkToc returned error: %v", err)
	}

	matches, err := store.FindPotentialDuplicateWorks(model.Work{Title: "スローライフ配信をしてたら、相方のゴーレムがアップをはじめたようです"})
	if err != nil {
		t.Fatalf("FindPotentialDuplicateWorks returned error: %v", err)
	}
	if len(matches) != 1 || matches[0].Site != model.SiteKakuyomu {
		t.Fatalf("matches = %#v", matches)
	}
}

func TestStoreUsesLeadingEpisodeTitlesForWeakSimilarTitle(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	defer store.Close()

	if _, err := saveWorkFully(t, store, model.Work{
		Site:       model.SiteKakuyomu,
		SiteName:   "カクヨム",
		SiteWorkID: "16818093080000000001",
		SourceURL:  "https://kakuyomu.jp/works/16818093080000000001",
		Title:      "魔導配信者の迷宮スローライフ記録",
		Episodes: []model.Episode{
			{Index: "1", Title: "第一話 迷宮で配信を始めました", Element: model.EpisodeElement{Body: "<p>1</p>"}},
			{Index: "2", Title: "第二話 相方ができました", Element: model.EpisodeElement{Body: "<p>2</p>"}},
			{Index: "3", Title: "第三話 初めてのコメント欄", Element: model.EpisodeElement{Body: "<p>3</p>"}},
			{Index: "4", Title: "第四話 休憩のスープ", Element: model.EpisodeElement{Body: "<p>4</p>"}},
			{Index: "5", Title: "第五話 朝の支度", Element: model.EpisodeElement{Body: "<p>5</p>"}},
		},
	}); err != nil {
		t.Fatalf("saveWorkFully returned error: %v", err)
	}

	matches, err := store.FindPotentialDuplicateWorks(model.Work{
		Title: "魔導配信者の迷宮ライフ記録",
		Episodes: []model.Episode{
			{Title: "第一話 迷宮で配信を始めました"},
			{Title: "第二話 相方ができました"},
			{Title: "第三話 初めてのコメント欄"},
			{Title: "第四話 別の休憩"},
			{Title: "第五話 別の朝"},
		},
	})
	if err != nil {
		t.Fatalf("FindPotentialDuplicateWorks returned error: %v", err)
	}
	if len(matches) != 1 || matches[0].Site != model.SiteKakuyomu {
		t.Fatalf("matches = %#v", matches)
	}
}

func TestStoreDoesNotUseTooFewLeadingEpisodeTitlesForWeakSimilarTitle(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	defer store.Close()

	if _, err := saveWorkFully(t, store, model.Work{
		Site:       model.SiteKakuyomu,
		SiteName:   "カクヨム",
		SiteWorkID: "16818093080000000002",
		SourceURL:  "https://kakuyomu.jp/works/16818093080000000002",
		Title:      "魔導配信者の迷宮スローライフ記録",
		Episodes: []model.Episode{
			{Index: "1", Title: "第一話", Element: model.EpisodeElement{Body: "<p>1</p>"}},
		},
	}); err != nil {
		t.Fatalf("saveWorkFully returned error: %v", err)
	}

	matches, err := store.FindPotentialDuplicateWorks(model.Work{
		Title:    "魔導配信者の迷宮ライフ記録",
		Episodes: []model.Episode{{Title: "第一話"}},
	})
	if err != nil {
		t.Fatalf("FindPotentialDuplicateWorks returned error: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("weak title with one generic episode title should not match: %#v", matches)
	}
}

func TestStoreLocalizesRemoteImagesWithDimensions(t *testing.T) {
	rootDir := t.TempDir()
	store, err := NewStore(rootDir)
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	defer store.Close()

	assetFetcher := &fakeAssetFetcher{
		bytes:       testPNG(t, 3, 2),
		contentType: "image/png",
	}
	store.SetAssetFetcher(assetFetcher, fetcher.FetchPolicy{})

	stored, err := saveWorkFully(t, store, model.Work{
		Site:       model.SiteSyosetu,
		SiteName:   "小説家になろう",
		SiteWorkID: "n1234ab",
		SourceURL:  "https://ncode.syosetu.com/n1234ab/",
		Title:      "画像作品",
		Author:     "作者",
		Story:      "あらすじ",
		FetchedAt:  time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC),
		Episodes: []model.Episode{
			{
				Index: "1",
				Href:  "/n1234ab/1/",
				Title: "第一話",
				Element: model.EpisodeElement{
					DataType: "html",
					Body:     `<p>前</p><p><a href="//29644.mitemin.net/i422674/"><img src="//29644.mitemin.net/userpageimage/viewimagebig/icode/i422674/" alt="挿絵" /></a></p>`,
				},
				FetchedAt: time.Date(2026, 5, 9, 12, 1, 0, 0, time.UTC),
			},
		},
	})
	if err != nil {
		t.Fatalf("逐次保存 helper returned error: %v", err)
	}

	if len(assetFetcher.urls) != 1 {
		t.Fatalf("asset fetch count = %d, want 1", len(assetFetcher.urls))
	}
	if assetFetcher.urls[0] != "https://29644.mitemin.net/userpageimage/viewimage/icode/i422674/" {
		t.Fatalf("asset URL = %q", assetFetcher.urls[0])
	}

	episodes, err := store.ListEpisodes(stored.ID)
	if err != nil {
		t.Fatalf("ListEpisodes returned error: %v", err)
	}
	canonical, err := store.ReadCanonicalEpisode(episodes[0])
	if err != nil {
		t.Fatalf("ReadCanonicalEpisode returned error: %v", err)
	}
	body := canonical.Blocks[len(canonical.Blocks)-1].HTML
	if !strings.Contains(body, `src="../assets/episodes/1/0-`) {
		t.Fatalf("localized image src was not written: %s", body)
	}
	if !strings.Contains(body, `width="3"`) || !strings.Contains(body, `height="2"`) {
		t.Fatalf("image dimensions were not injected: %s", body)
	}

	var assetCount int
	var storagePath string
	var width int
	var height int
	if err := store.db.QueryRow(`SELECT COUNT(*), storage_path, width, height FROM assets WHERE work_id = ?`, stored.ID).Scan(&assetCount, &storagePath, &width, &height); err != nil {
		t.Fatalf("asset query failed: %v", err)
	}
	if assetCount != 1 || width != 3 || height != 2 {
		t.Fatalf("unexpected asset row: count=%d width=%d height=%d path=%q", assetCount, width, height, storagePath)
	}
	if _, err := os.Stat(filepath.Join(rootDir, storagePath)); err != nil {
		t.Fatalf("asset file was not written: %v", err)
	}
}

func TestStoreFindsAndRemovesWorksAndEpisodes(t *testing.T) {
	rootDir := t.TempDir()
	store, err := NewStore(rootDir)
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	defer store.Close()

	stored, err := saveWorkFully(t, store, model.Work{
		Site:       model.SiteKakuyomu,
		SiteName:   "カクヨム",
		SiteWorkID: "0000000000000000000",
		SourceURL:  "https://kakuyomu.jp/works/0000000000000000000",
		Title:      "検索作品",
		Author:     "作者",
		FetchedAt:  time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC),
		Episodes: []model.Episode{{
			Index:      "20",
			Title:      "第1話",
			Subchapter: "一幕",
			FetchedAt:  time.Date(2026, 5, 9, 12, 1, 0, 0, time.UTC),
			Element:    model.EpisodeElement{DataType: "html", Body: "<p>本文</p>"},
		}},
	})
	if err != nil {
		t.Fatalf("逐次保存 helper returned error: %v", err)
	}

	foundWork, ok, err := store.FindWorkByID(stored.ID)
	if err != nil || !ok {
		t.Fatalf("FindWorkByID = %#v/%v/%v", foundWork, ok, err)
	}
	if foundWork.Title != "検索作品" || foundWork.EpisodeLen != 1 {
		t.Fatalf("unexpected work: %#v", foundWork)
	}

	if _, ok, err := store.FindWorkByID(999); err != nil || ok {
		t.Fatalf("missing FindWorkByID ok/err = %v/%v", ok, err)
	}

	episode, ok, err := store.FindEpisode(stored.ID, "20")
	if err != nil || !ok {
		t.Fatalf("FindEpisode = %#v/%v/%v", episode, ok, err)
	}
	if episode.Title != "第1話" || episode.Subchapter != "一幕" {
		t.Fatalf("unexpected episode: %#v", episode)
	}

	if _, ok, err := store.FindEpisode(stored.ID, "999"); err != nil || ok {
		t.Fatalf("missing FindEpisode ok/err = %v/%v", ok, err)
	}

	if err := store.RemoveWork(stored.ID, false); err != nil {
		t.Fatalf("RemoveWork returned error: %v", err)
	}
	if _, ok, err := store.FindWorkByID(stored.ID); err != nil || ok {
		t.Fatalf("removed FindWorkByID ok/err = %v/%v", ok, err)
	}
	if _, err := os.Stat(filepath.Join(rootDir, stored.Directory)); err != nil {
		t.Fatalf("directory should remain when withFiles=false: %v", err)
	}
	if err := store.RemoveWork(stored.ID, false); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("RemoveWork missing error = %v", err)
	}
}

func TestStoreRemoveWorkWithFilesDeletesDirectory(t *testing.T) {
	rootDir := t.TempDir()
	store, err := NewStore(rootDir)
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	defer store.Close()

	stored, err := saveWorkFully(t, store, model.Work{
		Site:       model.SiteSyosetu,
		SiteName:   "小説家になろう",
		SiteWorkID: "n2222aa",
		SourceURL:  "https://ncode.syosetu.com/n2222aa/",
		Title:      "削除作品",
		Author:     "作者",
		FetchedAt:  time.Now(),
		Episodes: []model.Episode{{
			Index:     "1",
			Title:     "第一話",
			FetchedAt: time.Now(),
			Element:   model.EpisodeElement{DataType: "html", Body: "<p>本文</p>"},
		}},
	})
	if err != nil {
		t.Fatalf("逐次保存 helper returned error: %v", err)
	}

	if err := store.RemoveWork(stored.ID, true); err != nil {
		t.Fatalf("RemoveWork returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(rootDir, stored.Directory)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("directory stat error = %v, want not exist", err)
	}
}

func TestStoreUpdatesExistingWorkAndReplacesEpisodes(t *testing.T) {
	rootDir := t.TempDir()
	store, err := NewStore(rootDir)
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	defer store.Close()

	work := model.Work{
		Site:       model.SiteSyosetu,
		SiteName:   "小説家になろう",
		SiteWorkID: "n3333aa",
		SourceURL:  "https://ncode.syosetu.com/n3333aa/",
		Title:      "旧タイトル",
		Author:     "旧作者",
		FetchedAt:  time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC),
		Episodes: []model.Episode{{
			Index:     "1",
			Title:     "旧第一話",
			FetchedAt: time.Date(2026, 5, 9, 12, 1, 0, 0, time.UTC),
			Element:   model.EpisodeElement{DataType: "html", Body: "<p>旧本文</p>"},
		}},
	}
	stored, err := saveWorkFully(t, store, work)
	if err != nil {
		t.Fatalf("initial 逐次保存 helper returned error: %v", err)
	}
	oldEpisode, ok, err := store.FindEpisode(stored.ID, "1")
	if err != nil || !ok {
		t.Fatalf("old FindEpisode = %#v/%v/%v", oldEpisode, ok, err)
	}
	oldBodyPath := filepath.Join(rootDir, oldEpisode.BodyPath)

	work.Title = "新タイトル"
	work.Author = "新作者"
	work.Story = "新あらすじ"
	work.FetchedAt = time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	work.Episodes = []model.Episode{
		{
			Index:     "2",
			Title:     "新第二話",
			FetchedAt: time.Date(2026, 5, 10, 12, 1, 0, 0, time.UTC),
			Element:   model.EpisodeElement{DataType: "html", Body: "<p>新本文</p>"},
		},
	}
	updated, err := saveWorkFully(t, store, work)
	if err != nil {
		t.Fatalf("updated 逐次保存 helper returned error: %v", err)
	}
	if updated.ID != stored.ID {
		t.Fatalf("updated.ID = %d, want %d", updated.ID, stored.ID)
	}

	found, ok, err := store.FindWorkByID(stored.ID)
	if err != nil || !ok {
		t.Fatalf("FindWorkByID = %#v/%v/%v", found, ok, err)
	}
	if found.Title != "新タイトル" || found.Author != "新作者" || found.Story != "新あらすじ" || found.EpisodeLen != 1 {
		t.Fatalf("updated work = %#v", found)
	}

	if _, ok, err := store.FindEpisode(stored.ID, "1"); err != nil || ok {
		t.Fatalf("old episode ok/err = %v/%v", ok, err)
	}
	if _, err := os.Stat(oldBodyPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old body file stat error = %v, want not exist", err)
	}
	episode, ok, err := store.FindEpisode(stored.ID, "2")
	if err != nil || !ok {
		t.Fatalf("new episode = %#v/%v/%v", episode, ok, err)
	}
	canonical, err := store.ReadCanonicalEpisode(episode)
	if err != nil {
		t.Fatalf("ReadCanonicalEpisode returned error: %v", err)
	}
	if canonical.Title != "新第二話" || canonical.Blocks[len(canonical.Blocks)-1].HTML != "<p>新本文</p>" {
		t.Fatalf("canonical = %#v", canonical)
	}
}

func TestStoreRemovesStaleAssetsWhenWorkIsUpdated(t *testing.T) {
	rootDir := t.TempDir()
	store, err := NewStore(rootDir)
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	defer store.Close()

	assetFetcher := &fakeAssetFetcher{
		bytes:       testPNG(t, 3, 2),
		contentType: "image/png",
	}
	store.SetAssetFetcher(assetFetcher, fetcher.FetchPolicy{})

	work := model.Work{
		Site:       model.SiteSyosetu,
		SiteName:   "小説家になろう",
		SiteWorkID: "n5555aa",
		SourceURL:  "https://ncode.syosetu.com/n5555aa/",
		Title:      "画像更新作品",
		Author:     "作者",
		FetchedAt:  time.Now(),
		Episodes: []model.Episode{{
			Index: "1",
			Href:  "/n5555aa/1/",
			Title: "第一話",
			Element: model.EpisodeElement{
				DataType: "html",
				Body:     `<p><img src="https://example.com/old.png" alt="旧挿絵"></p>`,
			},
			FetchedAt: time.Now(),
		}},
	}
	stored, err := saveWorkFully(t, store, work)
	if err != nil {
		t.Fatalf("initial 逐次保存 helper returned error: %v", err)
	}

	var oldStoragePath string
	if err := store.db.QueryRow(`SELECT storage_path FROM assets WHERE work_id = ?`, stored.ID).Scan(&oldStoragePath); err != nil {
		t.Fatalf("old asset query failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(rootDir, oldStoragePath)); err != nil {
		t.Fatalf("old asset file was not written: %v", err)
	}

	work.Episodes = []model.Episode{{
		Index: "1",
		Href:  "/n5555aa/1/",
		Title: "第一話",
		Element: model.EpisodeElement{
			DataType: "html",
			Body:     "<p>画像なし本文</p>",
		},
		FetchedAt: time.Now(),
	}}
	if _, err := saveWorkFully(t, store, work); err != nil {
		t.Fatalf("updated 逐次保存 helper returned error: %v", err)
	}

	var assetCount int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM assets WHERE work_id = ?`, stored.ID).Scan(&assetCount); err != nil {
		t.Fatalf("asset count query failed: %v", err)
	}
	if assetCount != 0 {
		t.Fatalf("assetCount = %d, want 0", assetCount)
	}
	if _, err := os.Stat(filepath.Join(rootDir, oldStoragePath)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old asset stat error = %v, want not exist", err)
	}
}

func TestUpsertWorkTocRemovesEpisodesMissingFromUpdatedToc(t *testing.T) {
	rootDir := t.TempDir()
	store, err := NewStore(rootDir)
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	work := model.Work{
		Site:       model.SiteSyosetu,
		SiteName:   "小説家になろう",
		SiteWorkID: "n7777aa",
		SourceURL:  "https://ncode.syosetu.com/n7777aa/",
		Title:      "削除検証作品",
		Author:     "作者",
		FetchedAt:  time.Now(),
		Episodes: []model.Episode{
			{Index: "1", Href: "/n7777aa/1/", Title: "第一話", Element: model.EpisodeElement{DataType: "html", Body: "<p>一</p>"}, FetchedAt: time.Now()},
			{Index: "2", Href: "/n7777aa/2/", Title: "第二話", Element: model.EpisodeElement{DataType: "html", Body: "<p>二</p>"}, FetchedAt: time.Now()},
		},
	}
	stored, err := store.UpsertWorkToc(ctx, work, FetchStatusPartial)
	if err != nil {
		t.Fatalf("initial UpsertWorkToc returned error: %v", err)
	}
	for index, episode := range work.Episodes {
		if _, err := store.SaveEpisodeBody(ctx, work, stored, episode, index); err != nil {
			t.Fatalf("SaveEpisodeBody(%d) returned error: %v", index, err)
		}
	}
	staleEpisode, ok, err := store.FindEpisode(stored.ID, "2")
	if err != nil || !ok {
		t.Fatalf("stale candidate = %#v/%v/%v", staleEpisode, ok, err)
	}
	staleBodyPath := filepath.Join(rootDir, staleEpisode.BodyPath)
	if _, err := os.Stat(staleBodyPath); err != nil {
		t.Fatalf("stale candidate body file was not written: %v", err)
	}

	work.Episodes = work.Episodes[:1]
	if _, err := store.UpsertWorkToc(ctx, work, FetchStatusPartial); err != nil {
		t.Fatalf("updated UpsertWorkToc returned error: %v", err)
	}
	if _, ok, err := store.FindEpisode(stored.ID, "2"); err != nil || ok {
		t.Fatalf("stale episode ok/err = %v/%v", ok, err)
	}
	if _, err := os.Stat(staleBodyPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale body stat error = %v, want not exist", err)
	}
}

func TestUpsertWorkTocRejectsDuplicateEpisodeIDs(t *testing.T) {
	rootDir := t.TempDir()
	store, err := NewStore(rootDir)
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	defer store.Close()

	_, err = store.UpsertWorkToc(context.Background(), model.Work{
		Site:       model.SiteSyosetu,
		SiteName:   "小説家になろう",
		SiteWorkID: "n9999aa",
		SourceURL:  "https://ncode.syosetu.com/n9999aa/",
		Title:      "重複検証作品",
		Author:     "作者",
		FetchedAt:  time.Now(),
		Episodes: []model.Episode{
			{Index: "1", Href: "/n9999aa/1/", Title: "第一話"},
			{Index: "1", Href: "/n9999aa/duplicate/", Title: "重複話"},
		},
	}, FetchStatusPartial)
	if err == nil || !strings.Contains(err.Error(), "duplicate episode id") {
		t.Fatalf("UpsertWorkToc duplicate error = %v", err)
	}
}

func TestSaveEpisodeBodyRemovesReplacedAssetFiles(t *testing.T) {
	rootDir := t.TempDir()
	store, err := NewStore(rootDir)
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	defer store.Close()

	assetFetcher := &fakeAssetFetcher{
		bytes:       testPNG(t, 3, 2),
		contentType: "image/png",
	}
	store.SetAssetFetcher(assetFetcher, fetcher.FetchPolicy{})

	ctx := context.Background()
	work := model.Work{
		Site:       model.SiteSyosetu,
		SiteName:   "小説家になろう",
		SiteWorkID: "n8888aa",
		SourceURL:  "https://ncode.syosetu.com/n8888aa/",
		Title:      "asset掃除作品",
		Author:     "作者",
		FetchedAt:  time.Now(),
		Episodes: []model.Episode{{
			Index:     "1",
			Href:      "/n8888aa/1/",
			Title:     "第一話",
			Element:   model.EpisodeElement{DataType: "html", Body: `<p><img src="https://example.com/old.png"></p>`},
			FetchedAt: time.Now(),
		}},
	}
	stored, err := store.UpsertWorkToc(ctx, work, FetchStatusPartial)
	if err != nil {
		t.Fatalf("UpsertWorkToc returned error: %v", err)
	}
	if _, err := store.SaveEpisodeBody(ctx, work, stored, work.Episodes[0], 0); err != nil {
		t.Fatalf("initial SaveEpisodeBody returned error: %v", err)
	}
	var oldStoragePath string
	if err := store.db.QueryRow(`SELECT storage_path FROM assets WHERE work_id = ?`, stored.ID).Scan(&oldStoragePath); err != nil {
		t.Fatalf("old asset query failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(rootDir, oldStoragePath)); err != nil {
		t.Fatalf("old asset file was not written: %v", err)
	}

	work.Episodes[0].Element.Body = "<p>画像なし</p>"
	if _, err := store.SaveEpisodeBody(ctx, work, stored, work.Episodes[0], 0); err != nil {
		t.Fatalf("updated SaveEpisodeBody returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(rootDir, oldStoragePath)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old asset stat error = %v, want not exist", err)
	}
}

func TestSaveEpisodeBodyKeepsReusedAssetFiles(t *testing.T) {
	rootDir := t.TempDir()
	store, err := NewStore(rootDir)
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	defer store.Close()

	assetFetcher := &fakeAssetFetcher{
		bytes:       testPNG(t, 3, 2),
		contentType: "image/png",
	}
	store.SetAssetFetcher(assetFetcher, fetcher.FetchPolicy{})

	ctx := context.Background()
	work := model.Work{
		Site:       model.SiteSyosetu,
		SiteName:   "小説家になろう",
		SiteWorkID: "n8889aa",
		SourceURL:  "https://ncode.syosetu.com/n8889aa/",
		Title:      "asset再利用作品",
		Author:     "作者",
		FetchedAt:  time.Now(),
		Episodes: []model.Episode{{
			Index:     "1",
			Href:      "/n8889aa/1/",
			Title:     "第一話",
			Element:   model.EpisodeElement{DataType: "html", Body: `<p><img src="https://example.com/same.png"></p>`},
			FetchedAt: time.Now(),
		}},
	}
	stored, err := store.UpsertWorkToc(ctx, work, FetchStatusPartial)
	if err != nil {
		t.Fatalf("UpsertWorkToc returned error: %v", err)
	}
	if _, err := store.SaveEpisodeBody(ctx, work, stored, work.Episodes[0], 0); err != nil {
		t.Fatalf("initial SaveEpisodeBody returned error: %v", err)
	}
	var storagePath string
	if err := store.db.QueryRow(`SELECT storage_path FROM assets WHERE work_id = ?`, stored.ID).Scan(&storagePath); err != nil {
		t.Fatalf("asset query failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(rootDir, storagePath)); err != nil {
		t.Fatalf("asset file was not written: %v", err)
	}

	if _, err := store.SaveEpisodeBody(ctx, work, stored, work.Episodes[0], 0); err != nil {
		t.Fatalf("second SaveEpisodeBody returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(rootDir, storagePath)); err != nil {
		t.Fatalf("reused asset file stat error = %v, want it to remain", err)
	}

	var assetCount int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM assets WHERE work_id = ?`, stored.ID).Scan(&assetCount); err != nil {
		t.Fatalf("asset count query failed: %v", err)
	}
	if assetCount != 1 {
		t.Fatalf("assetCount = %d, want 1", assetCount)
	}
}

func TestMarkEpisodeFailedKeepsCompleteBodyReadable(t *testing.T) {
	rootDir := t.TempDir()
	store, err := NewStore(rootDir)
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	defer store.Close()

	stored, err := saveWorkFully(t, store, model.Work{
		Site:       model.SiteSyosetu,
		SiteName:   "小説家になろう",
		SiteWorkID: "n5555aa",
		SourceURL:  "https://ncode.syosetu.com/n5555aa/",
		Title:      "失敗保持作品",
		Author:     "作者",
		FetchedAt:  time.Now(),
		Episodes: []model.Episode{{
			Index:     "1",
			Href:      "/n5555aa/1/",
			Title:     "第一話",
			Element:   model.EpisodeElement{DataType: "html", Body: "<p>本文</p>"},
			FetchedAt: time.Now(),
		}},
	})
	if err != nil {
		t.Fatalf("逐次保存 helper returned error: %v", err)
	}

	if err := store.MarkEpisodeFailed(context.Background(), stored.ID, "1", errors.New("temporary failure")); err != nil {
		t.Fatalf("MarkEpisodeFailed returned error: %v", err)
	}
	episode, ok, err := store.FindEpisode(stored.ID, "1")
	if err != nil || !ok {
		t.Fatalf("FindEpisode returned ok/err = %v/%v", ok, err)
	}
	if episode.BodyStatus != BodyStatusComplete || episode.LastFetchError != "temporary failure" {
		t.Fatalf("episode after failure = %#v", episode)
	}
	if _, err := store.ReadCanonicalEpisode(episode); err != nil {
		t.Fatalf("complete episode became unreadable: %v", err)
	}
}

func TestSaveEpisodeBodyPropagatesAssetContextCancellation(t *testing.T) {
	rootDir := t.TempDir()
	store, err := NewStore(rootDir)
	if err != nil {
		t.Fatalf("NewStore returned error: %v", err)
	}
	defer store.Close()
	store.SetAssetFetcher(&fakeAssetFetcher{err: context.Canceled}, fetcher.FetchPolicy{})

	ctx := context.Background()
	work := model.Work{
		Site:       model.SiteSyosetu,
		SiteName:   "小説家になろう",
		SiteWorkID: "n6666aa",
		SourceURL:  "https://ncode.syosetu.com/n6666aa/",
		Title:      "キャンセル検証作品",
		Author:     "作者",
		FetchedAt:  time.Now(),
		Episodes: []model.Episode{{
			Index:     "1",
			Href:      "/n6666aa/1/",
			Title:     "第一話",
			Element:   model.EpisodeElement{DataType: "html", Body: `<p><img src="https://example.com/image.png"></p>`},
			FetchedAt: time.Now(),
		}},
	}
	stored, err := store.UpsertWorkToc(ctx, work, FetchStatusPartial)
	if err != nil {
		t.Fatalf("UpsertWorkToc returned error: %v", err)
	}
	if _, err := store.SaveEpisodeBody(ctx, work, stored, work.Episodes[0], 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("SaveEpisodeBody error = %v, want context.Canceled", err)
	}
	episode, ok, err := store.FindEpisode(stored.ID, "1")
	if err != nil || !ok {
		t.Fatalf("FindEpisode returned ok/err = %v/%v", ok, err)
	}
	if episode.BodyStatus != BodyStatusPending || episode.BodyPath != "" {
		t.Fatalf("episode after canceled asset save = %#v", episode)
	}
}

func TestStoreKeepsOriginalImageWhenAssetFetchFails(t *testing.T) {
	for _, test := range []struct {
		name         string
		assetFetcher *fakeAssetFetcher
	}{
		{
			name:         "fetch error",
			assetFetcher: &fakeAssetFetcher{err: errors.New("asset fetch failed")},
		},
		{
			name:         "unsupported content type",
			assetFetcher: &fakeAssetFetcher{bytes: []byte("plain"), contentType: "text/plain"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			rootDir := t.TempDir()
			store, err := NewStore(rootDir)
			if err != nil {
				t.Fatalf("NewStore returned error: %v", err)
			}
			defer store.Close()
			store.SetAssetFetcher(test.assetFetcher, fetcher.FetchPolicy{})

			stored, err := saveWorkFully(t, store, model.Work{
				Site:       model.SiteSyosetu,
				SiteName:   "小説家になろう",
				SiteWorkID: "n4444aa",
				SourceURL:  "https://ncode.syosetu.com/n4444aa/",
				Title:      "画像失敗作品",
				Author:     "作者",
				FetchedAt:  time.Now(),
				Episodes: []model.Episode{{
					Index: "1",
					Href:  "/n4444aa/1/",
					Title: "第一話",
					Element: model.EpisodeElement{
						DataType: "html",
						Body:     `<p><img src="https://example.com/image.png" alt="挿絵"></p>`,
					},
					FetchedAt: time.Now(),
				}},
			})
			if err != nil {
				t.Fatalf("逐次保存 helper returned error: %v", err)
			}
			if len(test.assetFetcher.urls) != 1 {
				t.Fatalf("asset fetch count = %d, want 1", len(test.assetFetcher.urls))
			}

			var assetCount int
			if err := store.db.QueryRow(`SELECT COUNT(*) FROM assets WHERE work_id = ?`, stored.ID).Scan(&assetCount); err != nil {
				t.Fatalf("asset count query failed: %v", err)
			}
			if assetCount != 0 {
				t.Fatalf("assetCount = %d, want 0", assetCount)
			}

			episodes, err := store.ListEpisodes(stored.ID)
			if err != nil {
				t.Fatalf("ListEpisodes returned error: %v", err)
			}
			canonical, err := store.ReadCanonicalEpisode(episodes[0])
			if err != nil {
				t.Fatalf("ReadCanonicalEpisode returned error: %v", err)
			}
			body := canonical.Blocks[len(canonical.Blocks)-1].HTML
			if !strings.Contains(body, `src="https://example.com/image.png"`) || strings.Contains(body, `../assets/`) {
				t.Fatalf("body = %s", body)
			}
		})
	}
}

func TestStorageHelpers(t *testing.T) {
	if sanitizePathSegment("..") != "untitled" {
		t.Fatalf("sanitizePathSegment for dotdot failed")
	}
	if got := sanitizePathSegment("a b/c:*?"); got != "a_b_c___" {
		t.Fatalf("sanitizePathSegment = %q", got)
	}
	if got := truncateRunes("あいうえお", 3); got != "あいう" {
		t.Fatalf("truncateRunes = %q", got)
	}
	if got := truncateRunes("abc", 10); got != "abc" {
		t.Fatalf("truncateRunes short = %q", got)
	}
	if got := formatTime(time.Time{}); got == "" {
		t.Fatal("formatTime zero returned empty")
	}
	if got := parseTime("not a time"); !got.IsZero() {
		t.Fatalf("parseTime invalid = %s", got)
	}
}

func testPNG(t *testing.T, width int, height int) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, img); err != nil {
		t.Fatalf("png encode failed: %v", err)
	}
	return buffer.Bytes()
}
