package characterjobs

import (
	"context"
	"errors"
	"testing"
	"time"

	"narou-viewer/apps/viewer-api-go/internal/ai"
	"narou-viewer/apps/viewer-api-go/internal/characters"
	"narou-viewer/apps/viewer-api-go/internal/library"
)

type fakeLibrary struct {
	work       library.Work
	episode    library.Episode
	workFound  bool
	episodeOK  bool
	findWorkID string
}

func (f fakeLibrary) FindWork(novelID string) (library.Work, bool, error) {
	if !f.workFound {
		return library.Work{}, false, nil
	}
	work := f.work
	if work.ID == 0 {
		work.ID = 7
	}
	return work, true, nil
}

func (f fakeLibrary) FindEpisode(int, string) (library.Episode, bool, error) {
	return f.episode, f.episodeOK, nil
}

type fakeSettings struct {
	response ai.SettingsResponse
}

func (f fakeSettings) GetAIGenerationSettings() (ai.SettingsResponse, error) {
	return f.response, nil
}

func TestEnqueueCreatesJobAndReturnsActiveJobOnSecondRequest(t *testing.T) {
	stateDir := t.TempDir()
	if err := characters.EnsureStateDirs(stateDir); err != nil {
		t.Fatalf("EnsureStateDirs returned error: %v", err)
	}
	profileID := "profile-1"
	modelID := "model-1"
	service := NewService(
		stateDir,
		fakeLibrary{workFound: true, episodeOK: true},
		fakeSettings{response: ai.SettingsResponse{
			EffectiveGenerationMode: "llm",
			Settings: ai.SettingsMetadata{
				SelectedProfileID: &profileID,
				Profiles:          []ai.Profile{{ID: profileID, Label: "Profile 1", ModelID: &modelID}},
			},
		}},
	)
	service.now = func() time.Time { return time.Unix(123, 456).UTC() }
	service.nowISO = func() string { return "2026-06-30T00:00:00.000Z" }

	createdResponse, created, err := service.Enqueue(context.Background(), "novel-1", EnqueueInput{UpToEpisodeIndex: "1"})
	if err != nil || !created {
		t.Fatalf("first Enqueue result=%+v created=%v err=%v", createdResponse, created, err)
	}
	if createdResponse.Status != "queued" || createdResponse.GenerationStrategy != "serial" || createdResponse.Message == "" {
		t.Fatalf("created response should expose queued job DTO: %+v", createdResponse)
	}

	activeResponse, created, err := service.Enqueue(context.Background(), "novel-1", EnqueueInput{UpToEpisodeIndex: "1"})
	if err != nil || created {
		t.Fatalf("second Enqueue result=%+v created=%v err=%v", activeResponse, created, err)
	}
	if activeResponse.JobID != createdResponse.JobID || activeResponse.Message == createdResponse.Message {
		t.Fatalf("active response should return existing active job with conflict message: %+v", activeResponse)
	}
}

func TestListAndClearJobs(t *testing.T) {
	stateDir := t.TempDir()
	if err := characters.EnsureStateDirs(stateDir); err != nil {
		t.Fatalf("EnsureStateDirs returned error: %v", err)
	}
	service := NewService(stateDir, fakeLibrary{workFound: true, episodeOK: true}, fakeSettings{})

	empty, err := service.List(context.Background(), "novel-1")
	if err != nil {
		t.Fatalf("List empty returned error: %v", err)
	}
	if len(empty.Jobs) != 0 || empty.Jobs == nil {
		t.Fatalf("List should return an empty jobs array: %+v", empty)
	}
	if err := characters.SaveJob(stateDir, "novel-1", characters.Job{JobID: "job-done", RequestedUpToEpisodeIndex: "1", Status: "completed", CreatedAt: "2026-01-01T00:00:00Z"}); err != nil {
		t.Fatalf("SaveJob returned error: %v", err)
	}
	listed, err := service.List(context.Background(), "novel-1")
	if err != nil || len(listed.Jobs) != 1 {
		t.Fatalf("List with job = %+v err=%v", listed, err)
	}
	cleared, err := service.Clear(context.Background(), "novel-1")
	if err != nil {
		t.Fatalf("Clear returned error: %v", err)
	}
	if cleared.Message == "" || cleared.JobsDeleted != 1 {
		t.Fatalf("Clear response should expose deletion counts: %+v", cleared)
	}
}

func TestCharacterJobValidationErrors(t *testing.T) {
	service := NewService(t.TempDir(), fakeLibrary{workFound: true, episodeOK: false}, fakeSettings{})

	if _, _, err := service.Enqueue(context.Background(), "novel-1", EnqueueInput{UpToEpisodeIndex: "bad"}); !errors.Is(err, ErrInvalidUpToEpisodeIndex) {
		t.Fatalf("invalid index error = %v", err)
	}
	if _, _, err := service.Enqueue(context.Background(), "novel-1", EnqueueInput{UpToEpisodeIndex: "1"}); !errors.Is(err, ErrEpisodeOutOfRange) {
		t.Fatalf("missing episode error = %v", err)
	}
	invalidStrategy := "parallel"
	service = NewService(t.TempDir(), fakeLibrary{workFound: true, episodeOK: true}, fakeSettings{})
	if _, _, err := service.Enqueue(context.Background(), "novel-1", EnqueueInput{UpToEpisodeIndex: "1", GenerationStrategy: &invalidStrategy}); !errors.Is(err, ErrInvalidGenerationStrategy) {
		t.Fatalf("invalid strategy error = %v", err)
	}
	missing := NewService(t.TempDir(), fakeLibrary{}, fakeSettings{})
	if _, err := missing.List(context.Background(), "missing"); !errors.Is(err, ErrNovelNotFound) {
		t.Fatalf("missing novel error = %v", err)
	}
}

func TestClearRejectsActiveJob(t *testing.T) {
	stateDir := t.TempDir()
	if err := characters.EnsureStateDirs(stateDir); err != nil {
		t.Fatalf("EnsureStateDirs returned error: %v", err)
	}
	if err := characters.SaveJob(stateDir, "novel-1", characters.Job{JobID: "job-running", RequestedUpToEpisodeIndex: "1", Status: "running", CreatedAt: "2026-01-01T00:00:00Z"}); err != nil {
		t.Fatalf("SaveJob returned error: %v", err)
	}
	service := NewService(stateDir, fakeLibrary{workFound: true, episodeOK: true}, fakeSettings{})

	if _, err := service.Clear(context.Background(), "novel-1"); !errors.Is(err, ErrSummaryActive) {
		t.Fatalf("Clear active error = %v", err)
	}
}

func TestEnqueueAllowsEmptyProfilesAndExplicitStrategy(t *testing.T) {
	stateDir := t.TempDir()
	if err := characters.EnsureStateDirs(stateDir); err != nil {
		t.Fatalf("EnsureStateDirs returned error: %v", err)
	}
	strategy := "parallel_identity"
	service := NewService(
		stateDir,
		fakeLibrary{workFound: true, episodeOK: true},
		fakeSettings{response: ai.SettingsResponse{EffectiveGenerationMode: "heuristic"}},
	)
	service.now = nil
	service.nowISO = nil

	response, created, err := service.Enqueue(context.Background(), "novel-1", EnqueueInput{
		UpToEpisodeIndex:   "1",
		GenerationStrategy: &strategy,
	})
	if err != nil || !created {
		t.Fatalf("Enqueue with explicit strategy result=%+v created=%v err=%v", response, created, err)
	}
	if response.GenerationStrategy != strategy {
		t.Fatalf("explicit generation strategy should be normalized and saved: %+v", response)
	}
	jobs, ok, err := characters.LoadJobs(stateDir, "novel-1")
	if err != nil || !ok || len(jobs) != 1 {
		t.Fatalf("LoadJobs after Enqueue failed: jobs=%+v ok=%v err=%v", jobs, ok, err)
	}
	if jobs[0].ProfileID != nil || jobs[0].ProfileLabel != nil || jobs[0].ModelID != nil {
		t.Fatalf("empty profile settings should keep profile metadata nil: %+v", jobs[0])
	}
}
