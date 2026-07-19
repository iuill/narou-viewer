package extraction

import (
	"context"
	"log"
	"sync"

	extractdomain "narou-viewer/apps/viewer-api-go/internal/extraction"
)

type JobProcessor func(ctx context.Context, novelID string, job extractdomain.Job) bool

type JobCoordinator struct {
	stateDir string
	process  JobProcessor

	mu       sync.Mutex
	activeMu sync.Mutex
	active   map[string]context.CancelFunc
}

func NewJobCoordinator(stateDir string, process JobProcessor) *JobCoordinator {
	return &JobCoordinator{
		stateDir: stateDir,
		process:  process,
		active:   map[string]context.CancelFunc{},
	}
}

func (c *JobCoordinator) Recover() {
	if c == nil {
		return
	}
	if _, err := extractdomain.RecoverRunningJobs(c.stateDir); err != nil {
		log.Printf("viewer-api-go: failed to recover character jobs: %v", err)
	}
}

func (c *JobCoordinator) Kick(ctx context.Context) {
	if c == nil || c.process == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	go c.processJobs(ctx)
}

func (c *JobCoordinator) Cancel(jobID string) {
	if c == nil {
		return
	}
	c.activeMu.Lock()
	cancel := c.active[jobID]
	c.activeMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (c *JobCoordinator) processJobs(ctx context.Context) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for {
		if ctx.Err() != nil {
			return
		}
		records, err := extractdomain.LoadJobsForExecution(c.stateDir)
		if err != nil {
			return
		}
		var next *extractdomain.JobWithNovel
		for i := range records {
			if records[i].Job.Status == extractdomain.JobStatusQueued || records[i].Job.Status == extractdomain.JobStatusRunning {
				record := records[i]
				next = &record
			}
		}
		if next == nil {
			return
		}
		jobCtx, cancel := context.WithCancel(ctx)
		c.activeMu.Lock()
		c.active[next.Job.JobID] = cancel
		c.activeMu.Unlock()
		jobs, _, err := extractdomain.LoadJobs(c.stateDir, next.NovelID)
		if err != nil || !jobStillExecutable(jobs, next.Job.JobID) {
			cancel()
			c.activeMu.Lock()
			delete(c.active, next.Job.JobID)
			c.activeMu.Unlock()
			continue
		}
		processed := c.process(jobCtx, next.NovelID, next.Job)
		cancel()
		c.activeMu.Lock()
		delete(c.active, next.Job.JobID)
		c.activeMu.Unlock()
		if err := extractdomain.FinalizePausingJob(c.stateDir, next.NovelID, next.Job.JobID); err != nil {
			return
		}
		if !processed && ctx.Err() != nil {
			return
		}
	}
}

func jobStillExecutable(jobs []extractdomain.Job, jobID string) bool {
	for _, job := range jobs {
		if job.JobID == jobID {
			return job.Status == extractdomain.JobStatusQueued || job.Status == extractdomain.JobStatusRunning
		}
	}
	return false
}
