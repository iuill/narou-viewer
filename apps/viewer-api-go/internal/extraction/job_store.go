package extraction

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"narou-viewer/apps/viewer-api-go/internal/novelstate"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

var jobsMu sync.Mutex

func LoadJobs(stateDir string, novelID string) ([]Job, bool, error) {
	jobsMu.Lock()
	defer jobsMu.Unlock()

	return loadJobsUnlocked(stateDir, novelID)
}

func loadJobsUnlocked(stateDir string, novelID string) ([]Job, bool, error) {
	records, err := loadJobRecords(stateDir)
	if err != nil {
		return nil, false, err
	}
	jobs := []Job{}
	for _, record := range records {
		if record.NovelID == novelID {
			jobs = append(jobs, record.Job)
		}
	}
	return jobs, len(jobs) > 0, nil
}

func LoadAllJobs(stateDir string) ([]JobWithNovel, error) {
	jobsMu.Lock()
	defer jobsMu.Unlock()

	return loadJobRecords(stateDir)
}

func PruneNovelState(stateDir string, novelID string) (NovelStatePruneResult, error) {
	jobsMu.Lock()
	defer jobsMu.Unlock()

	var result NovelStatePruneResult
	err := novelstate.WithLock(novelID, func() error {
		var err error
		result, err = pruneNovelStateUnlocked(stateDir, novelID)
		return err
	})
	return result, err
}

func PruneNovelStateIfNoActive(stateDir string, novelID string) (NovelStatePruneResult, bool, error) {
	jobsMu.Lock()
	defer jobsMu.Unlock()

	novelID = strings.TrimSpace(novelID)
	if novelID == "" {
		return NovelStatePruneResult{}, false, nil
	}
	jobs, _, err := loadJobsUnlocked(stateDir, novelID)
	if err != nil {
		return NovelStatePruneResult{}, false, err
	}
	for _, job := range jobs {
		if job.Status == "queued" || job.Status == "running" {
			return NovelStatePruneResult{}, true, nil
		}
	}
	var result NovelStatePruneResult
	err = novelstate.WithLock(novelID, func() error {
		var err error
		result, err = pruneNovelStateUnlocked(stateDir, novelID)
		return err
	})
	return result, false, err
}

func pruneNovelStateUnlocked(stateDir string, novelID string) (NovelStatePruneResult, error) {
	novelID = strings.TrimSpace(novelID)
	if novelID == "" {
		return NovelStatePruneResult{}, nil
	}
	result := NovelStatePruneResult{}

	profilePath := filepath.Join(stateDir, "character_profiles", novelID+".yaml")
	if deleted, err := removeIfExists(profilePath); err != nil {
		return NovelStatePruneResult{}, err
	} else {
		result.ProfileDeleted = deleted
	}

	eventsPath := filepath.Join(stateDir, "character_events", novelID+".yaml")
	if deleted, err := removeIfExists(eventsPath); err != nil {
		return NovelStatePruneResult{}, err
	} else {
		result.EventsDeleted = deleted
	}

	for _, jobsDirName := range []string{"extraction_jobs", "character_jobs"} {
		indexPath := filepath.Join(stateDir, jobsDirName, "index", novelID+".yaml")
		if deleted, err := removeIfExists(indexPath); err != nil {
			return NovelStatePruneResult{}, err
		} else {
			result.JobIndexDeleted = result.JobIndexDeleted || deleted
		}

		jobPaths, err := filepath.Glob(filepath.Join(stateDir, jobsDirName, "*.yaml"))
		if err != nil {
			return NovelStatePruneResult{}, err
		}
		for _, path := range jobPaths {
			var doc jobDocument
			if ok, err := readYAMLIfExists(path, &doc); err != nil {
				log.Printf("extraction: skipping unreadable extraction job during prune %s: %v", path, err)
				continue
			} else if !ok || doc.NovelID != novelID {
				continue
			}
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return NovelStatePruneResult{}, err
			}
			result.JobsDeleted++
		}

		checkpointPaths, err := filepath.Glob(filepath.Join(stateDir, jobsDirName, "checkpoints", "*.json"))
		if err != nil {
			return NovelStatePruneResult{}, err
		}
		for _, path := range checkpointPaths {
			raw, err := os.ReadFile(path)
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return NovelStatePruneResult{}, err
			}
			var checkpoint struct {
				NovelID string `json:"novelId"`
			}
			if err := json.Unmarshal(raw, &checkpoint); err != nil || checkpoint.NovelID != novelID {
				continue
			}
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return NovelStatePruneResult{}, err
			}
			result.CheckpointsDeleted++
		}
	}
	return result, nil
}

func loadJobRecords(stateDir string) ([]JobWithNovel, error) {
	paths, err := filepath.Glob(filepath.Join(stateDir, "extraction_jobs", "*.yaml"))
	if err != nil {
		return nil, err
	}
	records := []JobWithNovel{}
	for _, path := range paths {
		var doc jobDocument
		if ok, err := readYAMLIfExists(path, &doc); err != nil {
			log.Printf("extraction: skipping unreadable extraction job %s: %v", path, err)
			continue
		} else if !ok {
			continue
		}
		records = append(records, JobWithNovel{NovelID: doc.NovelID, Job: doc.toJob()})
	}
	sort.SliceStable(records, func(i, j int) bool {
		return records[i].Job.CreatedAt > records[j].Job.CreatedAt
	})
	return records, nil
}

func (d jobDocument) toJob() Job {
	return Job{
		JobID:                     d.JobID,
		RequestedUpToEpisodeIndex: d.RequestedUpToEpisodeIndex,
		ProfileID:                 d.ProfileID,
		ProfileLabel:              d.ProfileLabel,
		GenerationMode:            d.GenerationMode,
		GenerationStrategy:        d.GenerationStrategy,
		ModelID:                   d.ModelID,
		Status:                    d.Status,
		Progress:                  d.Progress,
		ProgressStage:             d.ProgressStage,
		CurrentBatchIndex:         d.CurrentBatchIndex,
		BatchCount:                d.BatchCount,
		GeneratedCharacterCount:   d.GeneratedCharacterCount,
		CreatedAt:                 d.CreatedAt,
		StartedAt:                 d.StartedAt,
		FinishedAt:                d.FinishedAt,
		ErrorMessage:              d.ErrorMessage,
	}
}

func SaveJob(stateDir string, novelID string, job Job) error {
	jobsMu.Lock()
	defer jobsMu.Unlock()

	return saveJobUnlocked(stateDir, novelID, job)
}

func SaveJobIfNoActive(stateDir string, novelID string, job Job) (Job, bool, error) {
	jobsMu.Lock()
	defer jobsMu.Unlock()

	jobs, _, err := loadJobsUnlocked(stateDir, novelID)
	if err != nil {
		return Job{}, false, err
	}
	for _, existing := range jobs {
		if existing.Status == "queued" || existing.Status == "running" {
			return existing, false, nil
		}
	}
	if err := saveJobUnlocked(stateDir, novelID, job); err != nil {
		return Job{}, false, err
	}
	return job, true, nil
}

func RecoverRunningJobs(stateDir string) (int, error) {
	jobsMu.Lock()
	defer jobsMu.Unlock()

	records, err := loadJobRecords(stateDir)
	if err != nil {
		return 0, err
	}
	recovered := 0
	for _, record := range records {
		if record.Job.Status != "running" {
			continue
		}
		job := record.Job
		job.Status = "queued"
		job.StartedAt = nil
		job.FinishedAt = nil
		job.ErrorMessage = nil
		progress := 0
		stage := "recovered"
		job.Progress = &progress
		job.ProgressStage = &stage
		job.CurrentBatchIndex = nil
		job.BatchCount = nil
		job.GeneratedCharacterCount = nil
		if err := saveJobUnlocked(stateDir, record.NovelID, job); err != nil {
			return recovered, err
		}
		recovered++
	}
	return recovered, nil
}

func saveJobUnlocked(stateDir string, novelID string, job Job) error {
	doc := jobDocument{
		SchemaVersion:             2,
		Revision:                  1,
		JobID:                     job.JobID,
		NovelID:                   novelID,
		RequestedUpToEpisodeIndex: job.RequestedUpToEpisodeIndex,
		ProfileID:                 job.ProfileID,
		ProfileLabel:              job.ProfileLabel,
		GenerationMode:            job.GenerationMode,
		GenerationStrategy:        job.GenerationStrategy,
		ModelID:                   job.ModelID,
		Status:                    job.Status,
		Progress:                  job.Progress,
		ProgressStage:             job.ProgressStage,
		CurrentBatchIndex:         job.CurrentBatchIndex,
		BatchCount:                job.BatchCount,
		GeneratedCharacterCount:   job.GeneratedCharacterCount,
		CreatedAt:                 job.CreatedAt,
		StartedAt:                 job.StartedAt,
		FinishedAt:                job.FinishedAt,
		ErrorMessage:              job.ErrorMessage,
	}
	fileName, err := safeJobFileName(job.JobID)
	if err != nil {
		return err
	}
	if err := writeYAMLAtomic(filepath.Join(stateDir, "extraction_jobs", fileName+".yaml"), doc); err != nil {
		return err
	}
	return saveJobIndex(stateDir, novelID, job)
}

func saveJobIndex(stateDir string, novelID string, job Job) error {
	path := filepath.Join(stateDir, "extraction_jobs", "index", novelID+".yaml")
	doc := jobsIndexDocument{SchemaVersion: 2, NovelID: novelID, JobIDs: []string{}}
	if ok, err := readYAMLIfExists(path, &doc); err != nil {
		return err
	} else if !ok {
		doc = jobsIndexDocument{SchemaVersion: 2, NovelID: novelID, JobIDs: []string{}}
	}
	doc.Revision++
	doc.SchemaVersion = 2
	doc.NovelID = novelID
	if job.Status == "queued" || job.Status == "running" {
		doc.ActiveJobID = &job.JobID
	} else if doc.ActiveJobID != nil && *doc.ActiveJobID == job.JobID {
		doc.ActiveJobID = nil
	}
	doc.JobIDs = prependUniqueJobID(doc.JobIDs, job.JobID)
	return writeYAMLAtomic(path, doc)
}

func prependUniqueJobID(jobIDs []string, jobID string) []string {
	result := []string{jobID}
	for _, existing := range jobIDs {
		if existing != "" && existing != jobID {
			result = append(result, existing)
		}
	}
	return result
}

func safeJobFileName(jobID string) (string, error) {
	trimmed := strings.TrimSpace(jobID)
	if !isRawJobFileNameSafe(trimmed) || isWindowsReservedFileName(trimmed) {
		return "", fmt.Errorf("extraction job id must match [A-Za-z0-9_-]+: %q", jobID)
	}
	return trimmed, nil
}

func isRawJobFileNameSafe(value string) bool {
	for _, r := range value {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return false
	}
	return value != ""
}

func isWindowsReservedFileName(value string) bool {
	lower := strings.ToLower(value)
	switch lower {
	case "con", "prn", "aux", "nul":
		return true
	}
	if len(lower) == 4 && (strings.HasPrefix(lower, "com") || strings.HasPrefix(lower, "lpt")) {
		return lower[3] >= '1' && lower[3] <= '9'
	}
	return false
}
