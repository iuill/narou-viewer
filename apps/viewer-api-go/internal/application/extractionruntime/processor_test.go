package extractionruntime

import (
	"context"
	"errors"
	"testing"

	"narou-viewer/apps/viewer-api-go/internal/ai"
	appextraction "narou-viewer/apps/viewer-api-go/internal/application/extraction"
	core "narou-viewer/apps/viewer-api-go/internal/extraction"
	"narou-viewer/apps/viewer-api-go/internal/store"
)

type fakeExecutionSettings struct {
	response ai.SettingsResponse
	config   *store.ResolvedAIGenerationConfig
	err      error
}

func (s fakeExecutionSettings) GetAIGenerationSettings() (ai.SettingsResponse, error) {
	return s.response, s.err
}

func (s fakeExecutionSettings) ResolveActiveAIGenerationConfig() (*store.ResolvedAIGenerationConfig, error) {
	return s.config, s.err
}

type captureWorkflow struct {
	resolvedOverride *store.ResolvedAIGenerationConfig
}

func (w *captureWorkflow) GenerateAndSave(_ context.Context, _ string, _ string, resolvedOverride *store.ResolvedAIGenerationConfig, _ string, _ func(appextraction.BatchProgress)) (appextraction.FinalCounts, error) {
	w.resolvedOverride = resolvedOverride
	return appextraction.FinalCounts{}, nil
}

type fakeWorkflow struct {
	err error
}

func (w fakeWorkflow) GenerateAndSave(ctx context.Context, novelID string, upToEpisodeIndex string, resolvedOverride *store.ResolvedAIGenerationConfig, strategy string, progressSink func(appextraction.BatchProgress)) (appextraction.FinalCounts, error) {
	progressSink(appextraction.BatchProgress{
		Phase: "start",
		Batch: core.Batch{BatchIndex: 1, BatchCount: 2},
	})
	progressSink(appextraction.BatchProgress{
		Phase:                "complete",
		Batch:                core.Batch{BatchIndex: 1, BatchCount: 2},
		CompletedBatchCount:  1,
		MergedCharacterCount: 3,
		MergedTermCount:      2,
	})
	return appextraction.FinalCounts{CharacterCount: 3, TermCount: 2}, w.err
}

type fakeJobStore struct {
	jobs []core.Job
	err  error
}

func (s *fakeJobStore) Save(_ string, job core.Job) error {
	s.jobs = append(s.jobs, job)
	return s.err
}

func TestProcessorMarksCompletedJob(t *testing.T) {
	store := &fakeJobStore{}
	processor := NewProcessor(Dependencies{Workflow: fakeWorkflow{}, JobStore: store})
	if !processor.Process(t.Context(), "novel", core.Job{Status: "queued", RequestedUpToEpisodeIndex: "2"}) {
		t.Fatal("processor should report success")
	}
	last := store.jobs[len(store.jobs)-1]
	if last.Status != "completed" || last.Progress == nil || *last.Progress != 100 || last.ProgressStage == nil || *last.ProgressStage != "completed" {
		t.Fatalf("job should be completed with progress: %+v", last)
	}
}

func TestProcessorMarksFailedJob(t *testing.T) {
	store := &fakeJobStore{}
	processor := NewProcessor(Dependencies{Workflow: fakeWorkflow{err: errors.New("generation failed")}, JobStore: store})
	if !processor.Process(t.Context(), "novel", core.Job{Status: "queued", RequestedUpToEpisodeIndex: "2"}) {
		t.Fatal("processor should save failed job state")
	}
	last := store.jobs[len(store.jobs)-1]
	if last.Status != "failed" || last.ErrorMessage == nil || *last.ErrorMessage != "generation failed" {
		t.Fatalf("job should be failed with error message: %+v", last)
	}
}

func TestProcessorUpdatesJobMetadataToResolvedExecutionProfile(t *testing.T) {
	workflow := &captureWorkflow{}
	jobStore := &fakeJobStore{}
	config := &store.ResolvedAIGenerationConfig{ProfileID: "profile-b", ProfileLabel: "Profile B", ModelID: "model-b"}
	processor := NewProcessor(Dependencies{
		Workflow: workflow,
		JobStore: jobStore,
		Settings: fakeExecutionSettings{
			response: ai.SettingsResponse{EffectiveGenerationMode: "openrouter"},
			config:   config,
		},
	})
	profileA := "profile-a"
	modelA := "model-a"
	if !processor.Process(t.Context(), "novel", core.Job{Status: "queued", RequestedUpToEpisodeIndex: "2", ProfileID: &profileA, ModelID: &modelA}) {
		t.Fatal("processor should complete with the resolved execution profile")
	}
	last := jobStore.jobs[len(jobStore.jobs)-1]
	if last.ProfileID == nil || *last.ProfileID != "profile-b" || last.ModelID == nil || *last.ModelID != "model-b" || last.GenerationMode != "openrouter" {
		t.Fatalf("job metadata must reflect the executed profile: %+v", last)
	}
	if workflow.resolvedOverride != config {
		t.Fatalf("workflow should execute the exact resolved config: %+v", workflow.resolvedOverride)
	}
}

func TestProcessorUpdatesJobMetadataToHeuristicExecution(t *testing.T) {
	workflow := &captureWorkflow{}
	jobStore := &fakeJobStore{}
	processor := NewProcessor(Dependencies{
		Workflow: workflow,
		JobStore: jobStore,
		Settings: fakeExecutionSettings{response: ai.SettingsResponse{EffectiveGenerationMode: "heuristic"}},
	})
	profileID := "stale-profile"
	profileLabel := "Stale Profile"
	modelID := "stale-model"
	job := core.Job{
		Status:                    "queued",
		RequestedUpToEpisodeIndex: "2",
		GenerationMode:            "openrouter",
		ProfileID:                 &profileID,
		ProfileLabel:              &profileLabel,
		ModelID:                   &modelID,
	}
	if !processor.Process(t.Context(), "novel", job) {
		t.Fatal("processor should complete with heuristic execution settings")
	}
	last := jobStore.jobs[len(jobStore.jobs)-1]
	if last.GenerationMode != "heuristic" || last.ProfileID != nil || last.ProfileLabel != nil || last.ModelID != nil {
		t.Fatalf("heuristic metadata should clear stale OpenRouter fields: %+v", last)
	}
	if workflow.resolvedOverride != nil {
		t.Fatalf("heuristic execution should not pin an OpenRouter config: %+v", workflow.resolvedOverride)
	}
}

func TestProcessorMarksJobFailedWhenExecutionSettingsCannotBeResolved(t *testing.T) {
	jobStore := &fakeJobStore{}
	processor := NewProcessor(Dependencies{
		Workflow: &captureWorkflow{},
		JobStore: jobStore,
		Settings: fakeExecutionSettings{err: errors.New("settings unavailable")},
	})
	if !processor.Process(t.Context(), "novel", core.Job{Status: "queued", RequestedUpToEpisodeIndex: "2"}) {
		t.Fatal("processor should persist the settings resolution failure")
	}
	last := jobStore.jobs[len(jobStore.jobs)-1]
	if last.Status != "failed" || last.ErrorMessage == nil || *last.ErrorMessage != "settings unavailable" || last.StartedAt == nil || last.FinishedAt == nil {
		t.Fatalf("settings resolution failure should be recorded on the job: %+v", last)
	}
}

func TestProcessorRejectsMissingOpenRouterExecutionProfile(t *testing.T) {
	jobStore := &fakeJobStore{}
	processor := NewProcessor(Dependencies{
		Workflow: &captureWorkflow{},
		JobStore: jobStore,
		Settings: fakeExecutionSettings{response: ai.SettingsResponse{EffectiveGenerationMode: "openrouter"}},
	})
	if !processor.Process(t.Context(), "novel", core.Job{Status: "queued", RequestedUpToEpisodeIndex: "2"}) {
		t.Fatal("processor should persist the missing profile failure")
	}
	last := jobStore.jobs[len(jobStore.jobs)-1]
	if last.Status != "failed" || last.ErrorMessage == nil || *last.ErrorMessage != "AI生成プロファイルが見つかりません。" {
		t.Fatalf("missing OpenRouter profile should fail before generation: %+v", last)
	}
}

func TestProcessorStopsWhenContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	store := &fakeJobStore{}
	processor := NewProcessor(Dependencies{Workflow: fakeWorkflow{}, JobStore: store})
	if processor.Process(ctx, "novel", core.Job{Status: "queued"}) {
		t.Fatal("canceled context should stop processing")
	}
	if len(store.jobs) != 0 {
		t.Fatalf("canceled context should not save jobs: %+v", store.jobs)
	}
}

func TestProcessorAcceptsNilContext(t *testing.T) {
	store := &fakeJobStore{}
	processor := NewProcessor(Dependencies{Workflow: fakeWorkflow{}, JobStore: store})
	if !processor.Process(nil, "novel", core.Job{Status: "queued", RequestedUpToEpisodeIndex: "2"}) {
		t.Fatal("nil context should fall back to background context")
	}
	if len(store.jobs) == 0 {
		t.Fatal("nil context should not prevent processing")
	}
}

func TestProgressHelpers(t *testing.T) {
	job := core.Job{}
	currentBatchIndex := 1
	batchCount := 2
	generatedCharacterCount := 3
	SetExtractionJobProgress(&job, 120, "batchComplete", &currentBatchIndex, &batchCount, &generatedCharacterCount, nil)
	if job.Progress == nil || *job.Progress != 100 || job.ProgressStage == nil || *job.ProgressStage != "batchComplete" {
		t.Fatalf("progress should be clamped and staged: %+v", job)
	}
	SetExtractionJobProgress(&job, 50, "batchComplete", &currentBatchIndex, &batchCount, &generatedCharacterCount, nil)
	if job.Progress == nil || *job.Progress != 100 {
		t.Fatalf("progress should not move backwards: %+v", job)
	}
	if ExtractionJobBatchProgressPercent(0, 0) != 70 || ExtractionJobBatchProgressPercent(2, 4) != 62 {
		t.Fatal("batch progress percent returned an unexpected value")
	}
	stage := "batch"
	if ValueOrDefaultString(nil, "fallback") != "fallback" || ValueOrDefaultString(&stage, "fallback") != "batch" {
		t.Fatal("string fallback helper returned an unexpected value")
	}
}
