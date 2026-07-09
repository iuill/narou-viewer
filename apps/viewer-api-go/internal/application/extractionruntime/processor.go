package extractionruntime

import (
	"context"
	"time"

	"narou-viewer/apps/viewer-api-go/internal/ai"
	appextraction "narou-viewer/apps/viewer-api-go/internal/application/extraction"
	extractdomain "narou-viewer/apps/viewer-api-go/internal/extraction"
	"narou-viewer/apps/viewer-api-go/internal/store"
)

type Workflow interface {
	GenerateAndSave(ctx context.Context, novelID string, upToEpisodeIndex string, resolvedOverride *store.ResolvedAIGenerationConfig, strategy string, progressSink func(appextraction.BatchProgress)) error
}

type JobStore interface {
	Save(novelID string, job extractdomain.Job) error
}

type Logger func(stage string, startedAt time.Time, fields ...any)

type Dependencies struct {
	StateDir string
	Workflow Workflow
	JobStore JobStore
	Logger   Logger
}

type Processor struct {
	workflow Workflow
	store    JobStore
	logger   Logger
}

type filesystemJobStore struct {
	stateDir string
}

func (s filesystemJobStore) Save(novelID string, job extractdomain.Job) error {
	return extractdomain.SaveJob(s.stateDir, novelID, job)
}

func NewProcessor(deps Dependencies) *Processor {
	jobStore := deps.JobStore
	if jobStore == nil {
		jobStore = filesystemJobStore{stateDir: deps.StateDir}
	}
	return &Processor{
		workflow: deps.Workflow,
		store:    jobStore,
		logger:   deps.Logger,
	}
}

func (p *Processor) Process(ctx context.Context, novelID string, job extractdomain.Job) bool {
	if ctx == nil {
		ctx = context.Background()
	}
	jobStartedAt := time.Now()
	defer p.log("job_process", jobStartedAt, "novelId", novelID, "jobId", job.JobID, "upToEpisodeIndex", job.RequestedUpToEpisodeIndex, "status", job.Status)
	if ctx.Err() != nil || p == nil || p.workflow == nil || p.store == nil {
		return false
	}
	now := ai.NowISO()
	if job.StartedAt == nil || job.Status == "queued" {
		job.Status = "running"
		job.StartedAt = &now
		job.FinishedAt = nil
		job.ErrorMessage = nil
		SetExtractionJobProgress(&job, 0, "preparing", nil, nil, nil)
		saveStartedAt := time.Now()
		if err := p.store.Save(novelID, job); err != nil {
			p.log("job_save", saveStartedAt, "status", "error", "novelId", novelID, "jobId", job.JobID, "jobStatus", job.Status, "stage", ValueOrDefaultString(job.ProgressStage, ""))
			return false
		}
		p.log("job_save", saveStartedAt, "status", "ok", "novelId", novelID, "jobId", job.JobID, "jobStatus", job.Status, "stage", ValueOrDefaultString(job.ProgressStage, ""))
	}
	if ctx.Err() != nil {
		return false
	}

	progressSink := func(progress appextraction.BatchProgress) {
		if ctx.Err() != nil {
			return
		}
		switch progress.Phase {
		case "start":
			SetExtractionJobProgress(&job, ExtractionJobBatchProgressPercent(progress.Batch.BatchIndex-1, progress.Batch.BatchCount), "batch", &progress.Batch.BatchIndex, &progress.Batch.BatchCount, nil)
		case "complete":
			completedBatches := progress.Batch.BatchIndex
			if progress.CompletedBatchCount > 0 {
				completedBatches = progress.CompletedBatchCount
			}
			SetExtractionJobProgress(&job, ExtractionJobBatchProgressPercent(completedBatches, progress.Batch.BatchCount), "batchComplete", &progress.Batch.BatchIndex, &progress.Batch.BatchCount, &progress.MergedCharacterCount)
		default:
			return
		}
		saveStartedAt := time.Now()
		err := p.store.Save(novelID, job)
		status := "ok"
		if err != nil {
			status = "error"
		}
		p.log("job_save", saveStartedAt, "status", status, "novelId", novelID, "jobId", job.JobID, "jobStatus", job.Status, "stage", ValueOrDefaultString(job.ProgressStage, ""))
	}
	if err := p.workflow.GenerateAndSave(ctx, novelID, job.RequestedUpToEpisodeIndex, nil, job.GenerationStrategy, progressSink); err != nil {
		if ctx.Err() != nil {
			return false
		}
		finishedAt := ai.NowISO()
		message := err.Error()
		job.Status = "failed"
		job.FinishedAt = &finishedAt
		job.ErrorMessage = &message
		SetExtractionJobProgress(&job, ValueOrDefaultInt(job.Progress, 0), "failed", job.CurrentBatchIndex, job.BatchCount, job.GeneratedCharacterCount)
		saveStartedAt := time.Now()
		err := p.store.Save(novelID, job)
		status := "ok"
		if err != nil {
			status = "error"
		}
		p.log("job_save", saveStartedAt, "status", status, "novelId", novelID, "jobId", job.JobID, "jobStatus", job.Status, "stage", ValueOrDefaultString(job.ProgressStage, ""))
		return err == nil
	}

	finishedAt := ai.NowISO()
	job.Status = "completed"
	job.FinishedAt = &finishedAt
	job.ErrorMessage = nil
	SetExtractionJobProgress(&job, 100, "completed", job.CurrentBatchIndex, job.BatchCount, job.GeneratedCharacterCount)
	saveStartedAt := time.Now()
	err := p.store.Save(novelID, job)
	status := "ok"
	if err != nil {
		status = "error"
	}
	p.log("job_save", saveStartedAt, "status", status, "novelId", novelID, "jobId", job.JobID, "jobStatus", job.Status, "stage", ValueOrDefaultString(job.ProgressStage, ""))
	return err == nil
}

func (p *Processor) log(stage string, startedAt time.Time, fields ...any) {
	if p != nil && p.logger != nil {
		p.logger(stage, startedAt, fields...)
	}
}

func SetExtractionJobProgress(job *extractdomain.Job, progress int, stage string, currentBatchIndex *int, batchCount *int, generatedCharacterCount *int) {
	if progress < 0 {
		progress = 0
	}
	if progress > 100 {
		progress = 100
	}
	if stage != "failed" && job.Progress != nil && progress < *job.Progress {
		progress = *job.Progress
	}
	job.Progress = &progress
	job.ProgressStage = &stage
	job.CurrentBatchIndex = currentBatchIndex
	job.BatchCount = batchCount
	job.GeneratedCharacterCount = generatedCharacterCount
}

func ExtractionJobBatchProgressPercent(completedBatches int, batchCount int) int {
	if batchCount <= 0 {
		return 70
	}
	return 35 + completedBatches*55/batchCount
}

func ValueOrDefaultInt(value *int, fallback int) int {
	if value == nil {
		return fallback
	}
	return *value
}

func ValueOrDefaultString(value *string, fallback string) string {
	if value == nil {
		return fallback
	}
	return *value
}
