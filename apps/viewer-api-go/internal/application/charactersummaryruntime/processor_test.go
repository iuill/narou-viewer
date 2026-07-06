package charactersummaryruntime

import (
	"context"
	"errors"
	"testing"

	appcharactersummary "narou-viewer/apps/viewer-api-go/internal/application/charactersummary"
	"narou-viewer/apps/viewer-api-go/internal/characters"
	core "narou-viewer/apps/viewer-api-go/internal/charactersummary"
	"narou-viewer/apps/viewer-api-go/internal/store"
)

type fakeWorkflow struct {
	err error
}

func (w fakeWorkflow) GenerateAndSave(ctx context.Context, novelID string, upToEpisodeIndex string, resolvedOverride *store.ResolvedAIGenerationConfig, strategy string, progressSink func(appcharactersummary.BatchProgress)) error {
	progressSink(appcharactersummary.BatchProgress{
		Phase: "start",
		Batch: core.Batch{BatchIndex: 1, BatchCount: 2},
	})
	progressSink(appcharactersummary.BatchProgress{
		Phase:                "complete",
		Batch:                core.Batch{BatchIndex: 1, BatchCount: 2},
		CompletedBatchCount:  1,
		MergedCharacterCount: 3,
	})
	return w.err
}

type fakeJobStore struct {
	jobs []characters.Job
	err  error
}

func (s *fakeJobStore) Save(_ string, job characters.Job) error {
	s.jobs = append(s.jobs, job)
	return s.err
}

func TestProcessorMarksCompletedJob(t *testing.T) {
	store := &fakeJobStore{}
	processor := NewProcessor(Dependencies{Workflow: fakeWorkflow{}, JobStore: store})
	if !processor.Process(t.Context(), "novel", characters.Job{Status: "queued", RequestedUpToEpisodeIndex: "2"}) {
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
	if !processor.Process(t.Context(), "novel", characters.Job{Status: "queued", RequestedUpToEpisodeIndex: "2"}) {
		t.Fatal("processor should save failed job state")
	}
	last := store.jobs[len(store.jobs)-1]
	if last.Status != "failed" || last.ErrorMessage == nil || *last.ErrorMessage != "generation failed" {
		t.Fatalf("job should be failed with error message: %+v", last)
	}
}

func TestProcessorStopsWhenContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	store := &fakeJobStore{}
	processor := NewProcessor(Dependencies{Workflow: fakeWorkflow{}, JobStore: store})
	if processor.Process(ctx, "novel", characters.Job{Status: "queued"}) {
		t.Fatal("canceled context should stop processing")
	}
	if len(store.jobs) != 0 {
		t.Fatalf("canceled context should not save jobs: %+v", store.jobs)
	}
}

func TestProcessorAcceptsNilContext(t *testing.T) {
	store := &fakeJobStore{}
	processor := NewProcessor(Dependencies{Workflow: fakeWorkflow{}, JobStore: store})
	if !processor.Process(nil, "novel", characters.Job{Status: "queued", RequestedUpToEpisodeIndex: "2"}) {
		t.Fatal("nil context should fall back to background context")
	}
	if len(store.jobs) == 0 {
		t.Fatal("nil context should not prevent processing")
	}
}

func TestProgressHelpers(t *testing.T) {
	job := characters.Job{}
	currentBatchIndex := 1
	batchCount := 2
	generatedCharacterCount := 3
	SetCharacterJobProgress(&job, 120, "batchComplete", &currentBatchIndex, &batchCount, &generatedCharacterCount)
	if job.Progress == nil || *job.Progress != 100 || job.ProgressStage == nil || *job.ProgressStage != "batchComplete" {
		t.Fatalf("progress should be clamped and staged: %+v", job)
	}
	SetCharacterJobProgress(&job, 50, "batchComplete", &currentBatchIndex, &batchCount, &generatedCharacterCount)
	if job.Progress == nil || *job.Progress != 100 {
		t.Fatalf("progress should not move backwards: %+v", job)
	}
	if CharacterJobBatchProgressPercent(0, 0) != 70 || CharacterJobBatchProgressPercent(2, 4) != 62 {
		t.Fatal("batch progress percent returned an unexpected value")
	}
	stage := "batch"
	if ValueOrDefaultString(nil, "fallback") != "fallback" || ValueOrDefaultString(&stage, "fallback") != "batch" {
		t.Fatal("string fallback helper returned an unexpected value")
	}
}
