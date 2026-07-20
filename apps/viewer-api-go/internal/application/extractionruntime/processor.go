package extractionruntime

import (
	"context"
	"errors"
	"sort"
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
	SaveIfCurrentStatus(novelID string, job extractdomain.Job, expectedStatuses ...string) (bool, error)
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

func (s filesystemJobStore) SaveIfCurrentStatus(novelID string, job extractdomain.Job, expectedStatuses ...string) (bool, error) {
	return extractdomain.SaveJobIfCurrentStatus(s.stateDir, novelID, job, expectedStatuses...)
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
		saved, err := p.store.SaveIfCurrentStatus(novelID, job, extractdomain.JobStatusQueued, extractdomain.JobStatusRunning)
		return err == nil && saved
	}
	now := ai.NowISO()
	if job.StartedAt == nil || job.Status == "queued" {
		job.Status = "running"
		job.StartedAt = &now
		job.FinishedAt = nil
		job.ErrorMessage = nil
		SetExtractionJobProgress(&job, 0, "preparing", nil, nil, nil, nil)
		saveStartedAt := time.Now()
		saved, err := p.store.SaveIfCurrentStatus(novelID, job, extractdomain.JobStatusQueued, extractdomain.JobStatusRunning)
		if err != nil || !saved {
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
		case "discoveryStart":
			SetExtractionJobProgress(&job, ExtractionJobDiscoveryProgressPercent(progress.CompletedBatchCount, progress.Batch.BatchCount), "discovery", nil, nil, job.GeneratedCharacterCount, job.GeneratedTermCount)
			SetExtractionJobActiveWorker(&job, progress, "discovery")
		case "discoveryComplete":
			RemoveExtractionJobActiveWorker(&job, progress.WorkerIndex)
			SetExtractionJobProgress(&job, ExtractionJobDiscoveryProgressPercent(progress.CompletedBatchCount, progress.Batch.BatchCount), "discovery", nil, nil, job.GeneratedCharacterCount, job.GeneratedTermCount)
		case "discoveryError":
			RemoveExtractionJobActiveWorker(&job, progress.WorkerIndex)
		case "parallelStart":
			SetExtractionJobProgress(&job, ExtractionJobBatchProgressPercent(progress.CompletedBatchCount, progress.Batch.BatchCount), "batch", &progress.Batch.BatchIndex, &progress.Batch.BatchCount, job.GeneratedCharacterCount, job.GeneratedTermCount)
			SetExtractionJobCompletedBatchCount(&job, progress.CompletedBatchCount)
			SetExtractionJobActiveWorker(&job, progress, "extraction")
		case "start":
			completedBatches := progress.Batch.BatchIndex - 1
			SetExtractionJobProgress(&job, ExtractionJobBatchProgressPercent(completedBatches, progress.Batch.BatchCount), "batch", &progress.Batch.BatchIndex, &progress.Batch.BatchCount, job.GeneratedCharacterCount, job.GeneratedTermCount)
			SetExtractionJobCompletedBatchCount(&job, completedBatches)
		case "complete":
			RemoveExtractionJobActiveWorker(&job, progress.WorkerIndex)
			completedBatches := progress.Batch.BatchIndex
			if progress.CompletedBatchCount > 0 {
				completedBatches = progress.CompletedBatchCount
			}
			stage := "batchComplete"
			if len(job.ActiveWorkers) > 0 {
				stage = "batch"
			}
			SetExtractionJobProgress(&job, ExtractionJobBatchProgressPercent(completedBatches, progress.Batch.BatchCount), stage, &progress.Batch.BatchIndex, &progress.Batch.BatchCount, &progress.MergedCharacterCount, &progress.MergedTermCount)
			SetExtractionJobCompletedBatchCount(&job, completedBatches)
		case "error":
			RemoveExtractionJobActiveWorker(&job, progress.WorkerIndex)
		default:
			return
		}
		saveStartedAt := time.Now()
		_, err := p.store.SaveIfCurrentStatus(novelID, job, extractdomain.JobStatusRunning)
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
		saved, err := p.store.SaveIfCurrentStatus(novelID, job, extractdomain.JobStatusRunning)
		status := "ok"
		if err != nil {
			status = "error"
		}
		p.log("job_save", saveStartedAt, "status", status, "novelId", novelID, "jobId", job.JobID, "jobStatus", job.Status, "stage", ValueOrDefaultString(job.ProgressStage, ""))
		return err == nil && saved
	}

	finishedAt := ai.NowISO()
	job.Status = "completed"
	job.FinishedAt = &finishedAt
	job.ErrorMessage = nil
	SetExtractionJobProgress(&job, 100, "completed", job.CurrentBatchIndex, job.BatchCount, &counts.CharacterCount, &counts.TermCount)
	if job.BatchCount != nil {
		SetExtractionJobCompletedBatchCount(&job, *job.BatchCount)
	}
	saveStartedAt := time.Now()
	saved, err := p.store.SaveIfCurrentStatus(novelID, job, extractdomain.JobStatusRunning)
	status := "ok"
	if err != nil {
		status = "error"
	}
	p.log("job_save", saveStartedAt, "status", status, "novelId", novelID, "jobId", job.JobID, "jobStatus", job.Status, "stage", ValueOrDefaultString(job.ProgressStage, ""))
	return err == nil && saved
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
		return nil, errors.New("AI生成プロファイルが見つかりません。")
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
	if stage == "preparing" || stage == "completed" || stage == "failed" || stage == "recovered" {
		job.ActiveWorkers = nil
	}
}

func SetExtractionJobActiveWorker(job *extractdomain.Job, progress appextraction.BatchProgress, phase string) {
	if job == nil || progress.WorkerIndex < 1 || len(progress.Batch.EpisodeIndexes) == 0 {
		return
	}
	worker := extractdomain.ActiveWorker{
		WorkerIndex:       progress.WorkerIndex,
		BatchIndex:        progress.Batch.BatchIndex,
		StartEpisodeIndex: progress.Batch.EpisodeIndexes[0],
		EndEpisodeIndex:   progress.Batch.EpisodeIndexes[len(progress.Batch.EpisodeIndexes)-1],
		Phase:             phase,
	}
	for index := range job.ActiveWorkers {
		if job.ActiveWorkers[index].WorkerIndex == worker.WorkerIndex {
			job.ActiveWorkers[index] = worker
			return
		}
	}
	job.ActiveWorkers = append(job.ActiveWorkers, worker)
	sort.Slice(job.ActiveWorkers, func(i, j int) bool {
		return job.ActiveWorkers[i].WorkerIndex < job.ActiveWorkers[j].WorkerIndex
	})
}

func RemoveExtractionJobActiveWorker(job *extractdomain.Job, workerIndex int) {
	if job == nil || workerIndex < 1 || len(job.ActiveWorkers) == 0 {
		return
	}
	workers := job.ActiveWorkers[:0]
	for _, worker := range job.ActiveWorkers {
		if worker.WorkerIndex != workerIndex {
			workers = append(workers, worker)
		}
	}
	job.ActiveWorkers = workers
}

func SetExtractionJobCompletedBatchCount(job *extractdomain.Job, completedBatchCount int) {
	if completedBatchCount < 0 {
		completedBatchCount = 0
	}
	if job.BatchCount != nil && completedBatchCount > *job.BatchCount {
		completedBatchCount = *job.BatchCount
	}
	job.CompletedBatchCount = &completedBatchCount
}

func ExtractionJobBatchProgressPercent(completedBatches int, batchCount int) int {
	if batchCount <= 0 {
		return 70
	}
	return 35 + completedBatches*55/batchCount
}

func ExtractionJobDiscoveryProgressPercent(completedBatches int, batchCount int) int {
	if batchCount <= 0 {
		return 5
	}
	if completedBatches < 0 {
		completedBatches = 0
	}
	if completedBatches > batchCount {
		completedBatches = batchCount
	}
	return 5 + completedBatches*25/batchCount
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
