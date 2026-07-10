package extractionruntime

import (
	"context"
	"errors"
	"time"

	"narou-viewer/apps/viewer-api-go/internal/ai"
	appextraction "narou-viewer/apps/viewer-api-go/internal/application/extraction"
	extractdomain "narou-viewer/apps/viewer-api-go/internal/extraction"
	"narou-viewer/apps/viewer-api-go/internal/store"
)

type Workflow interface {
	GenerateAndSave(ctx context.Context, novelID string, upToEpisodeIndex string, resolvedOverride *store.ResolvedAIGenerationConfig, strategy string, progressSink func(appextraction.BatchProgress)) (appextraction.FinalCounts, error)
}

type JobStore interface {
	Save(novelID string, job extractdomain.Job) error
}

type ExecutionSettings interface {
	GetAIGenerationSettings() (ai.SettingsResponse, error)
	ResolveActiveAIGenerationConfig() (*store.ResolvedAIGenerationConfig, error)
}

type Logger func(stage string, startedAt time.Time, fields ...any)

type Dependencies struct {
	StateDir string
	Workflow Workflow
	JobStore JobStore
	Settings ExecutionSettings
	Logger   Logger
}

type Processor struct {
	workflow Workflow
	store    JobStore
	settings ExecutionSettings
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
		settings: deps.Settings,
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
	resolvedOverride, resolveErr := p.resolveExecutionContext(&job)
	if resolveErr != nil {
		finishedAt := ai.NowISO()
		message := resolveErr.Error()
		job.Status = "failed"
		job.StartedAt = &finishedAt
		job.FinishedAt = &finishedAt
		job.ErrorMessage = &message
		SetExtractionJobProgress(&job, 0, "failed", nil, nil, nil, nil)
		return p.store.Save(novelID, job) == nil
	}
	now := ai.NowISO()
	if job.StartedAt == nil || job.Status == "queued" {
		job.Status = "running"
		job.StartedAt = &now
		job.FinishedAt = nil
		job.ErrorMessage = nil
		SetExtractionJobProgress(&job, 0, "preparing", nil, nil, nil, nil)
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
			SetExtractionJobProgress(&job, ExtractionJobBatchProgressPercent(progress.Batch.BatchIndex-1, progress.Batch.BatchCount), "batch", &progress.Batch.BatchIndex, &progress.Batch.BatchCount, nil, nil)
		case "complete":
			completedBatches := progress.Batch.BatchIndex
			if progress.CompletedBatchCount > 0 {
				completedBatches = progress.CompletedBatchCount
			}
			SetExtractionJobProgress(&job, ExtractionJobBatchProgressPercent(completedBatches, progress.Batch.BatchCount), "batchComplete", &progress.Batch.BatchIndex, &progress.Batch.BatchCount, &progress.MergedCharacterCount, &progress.MergedTermCount)
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
	counts, generationErr := p.workflow.GenerateAndSave(ctx, novelID, job.RequestedUpToEpisodeIndex, resolvedOverride, job.GenerationStrategy, progressSink)
	if generationErr != nil {
		if ctx.Err() != nil {
			return false
		}
		finishedAt := ai.NowISO()
		message := generationErr.Error()
		job.Status = "failed"
		job.FinishedAt = &finishedAt
		job.ErrorMessage = &message
		SetExtractionJobProgress(&job, ValueOrDefaultInt(job.Progress, 0), "failed", job.CurrentBatchIndex, job.BatchCount, job.GeneratedCharacterCount, job.GeneratedTermCount)
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
	SetExtractionJobProgress(&job, 100, "completed", job.CurrentBatchIndex, job.BatchCount, &counts.CharacterCount, &counts.TermCount)
	saveStartedAt := time.Now()
	err := p.store.Save(novelID, job)
	status := "ok"
	if err != nil {
		status = "error"
	}
	p.log("job_save", saveStartedAt, "status", status, "novelId", novelID, "jobId", job.JobID, "jobStatus", job.Status, "stage", ValueOrDefaultString(job.ProgressStage, ""))
	return err == nil
}

func (p *Processor) resolveExecutionContext(job *extractdomain.Job) (*store.ResolvedAIGenerationConfig, error) {
	if p == nil || p.settings == nil {
		return nil, nil
	}
	settings, err := p.settings.GetAIGenerationSettings()
	if err != nil {
		return nil, err
	}
	job.GenerationMode = settings.EffectiveGenerationMode
	if settings.EffectiveGenerationMode != "openrouter" {
		job.ProfileID = nil
		job.ProfileLabel = nil
		job.ModelID = nil
		return nil, nil
	}
	config, err := p.settings.ResolveActiveAIGenerationConfig()
	if err != nil {
		return nil, err
	}
	if config == nil {
		return nil, errors.New("AI generation profile was not found.")
	}
	job.ProfileID = stringPointer(config.ProfileID)
	job.ProfileLabel = stringPointer(config.ProfileLabel)
	job.ModelID = stringPointer(config.ModelID)
	return config, nil
}

func stringPointer(value string) *string {
	return &value
}

func (p *Processor) log(stage string, startedAt time.Time, fields ...any) {
	if p != nil && p.logger != nil {
		p.logger(stage, startedAt, fields...)
	}
}

func SetExtractionJobProgress(job *extractdomain.Job, progress int, stage string, currentBatchIndex *int, batchCount *int, generatedCharacterCount *int, generatedTermCount *int) {
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
	job.GeneratedTermCount = generatedTermCount
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
