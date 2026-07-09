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

	mu sync.Mutex
}

func NewJobCoordinator(stateDir string, process JobProcessor) *JobCoordinator {
	return &JobCoordinator{
		stateDir: stateDir,
		process:  process,
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

func (c *JobCoordinator) processJobs(ctx context.Context) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for {
		if ctx.Err() != nil {
			return
		}
		records, err := extractdomain.LoadAllJobs(c.stateDir)
		if err != nil {
			return
		}
		var next *extractdomain.JobWithNovel
		for i := range records {
			if records[i].Job.Status == "queued" || records[i].Job.Status == "running" {
				record := records[i]
				next = &record
			}
		}
		if next == nil {
			return
		}
		if !c.process(ctx, next.NovelID, next.Job) {
			return
		}
	}
}
