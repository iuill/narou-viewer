package extraction

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"narou-viewer/apps/viewer-api-go/internal/characters"
	"narou-viewer/apps/viewer-api-go/internal/extraction/checkpointstore"
	"narou-viewer/apps/viewer-api-go/internal/novelstate"
	"narou-viewer/apps/viewer-api-go/internal/state/filequarantine"
	"narou-viewer/apps/viewer-api-go/internal/state/schemaguard"
	"narou-viewer/apps/viewer-api-go/internal/state/yamlfile"
	"narou-viewer/apps/viewer-api-go/internal/terms"

	"gopkg.in/yaml.v3"
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
		if err := preflightPruneNovelStateUnlocked(stateDir, novelID); err != nil {
			return err
		}
		var err error
		result, err = pruneNovelStateUnlocked(stateDir, novelID)
		return err
	})
	return result, err
}

func PreflightPruneNovelState(stateDir string, novelID string) error {
	jobsMu.Lock()
	defer jobsMu.Unlock()

	return novelstate.WithLock(novelID, func() error {
		return preflightPruneNovelStateUnlocked(stateDir, novelID)
	})
}

func PruneNovelStateIfNoActive(stateDir string, novelID string) (NovelStatePruneResult, bool, error) {
	jobsMu.Lock()
	defer jobsMu.Unlock()

	novelID = strings.TrimSpace(novelID)
	if novelID == "" {
		return NovelStatePruneResult{}, false, nil
	}
	if err := preflightPruneNovelStateUnlocked(stateDir, novelID); err != nil {
		return NovelStatePruneResult{}, false, err
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
		if err := preflightPruneNovelStateUnlocked(stateDir, novelID); err != nil {
			return err
		}
		var err error
		result, err = pruneNovelStateUnlocked(stateDir, novelID)
		return err
	})
	return result, false, err
}

func preflightPruneNovelStateUnlocked(stateDir string, novelID string) error {
	novelID = strings.TrimSpace(novelID)
	if novelID == "" {
		return nil
	}
	if err := characters.PreflightPruneNovelState(stateDir, novelID); err != nil {
		return err
	}
	if err := terms.PreflightPruneNovelState(stateDir, novelID); err != nil {
		return err
	}
	if err := preflightJobDocumentsUnlocked(stateDir, novelID); err != nil {
		return err
	}

	jobsDir := filepath.Join(stateDir, "extraction_jobs")
	checkpointPaths, err := filepath.Glob(filepath.Join(jobsDir, "checkpoints", "*.json"))
	if err != nil {
		return err
	}
	for _, path := range checkpointPaths {
		raw, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		_, guardErr := schemaguard.CheckJSON(raw, checkpointstore.SchemaContract)
		var checkpoint checkpointstore.Checkpoint
		if err := json.Unmarshal(raw, &checkpoint); err != nil {
			_, malformedErr := schemaguard.Malformed(checkpointstore.SchemaContract, err)
			return fmt.Errorf("preflight extraction checkpoint %s: %w", path, malformedErr)
		}
		if checkpoint.NovelID == novelID && guardErr != nil {
			return guardErr
		}
	}
	return nil
}

func preflightJobDocumentsUnlocked(stateDir string, novelID string) error {
	jobsDir := filepath.Join(stateDir, "extraction_jobs")
	jobPaths, err := filepath.Glob(filepath.Join(jobsDir, "*.yaml"))
	if err != nil {
		return err
	}
	for _, path := range jobPaths {
		read := readJobDocument(path)
		if read.err != nil {
			return fmt.Errorf("preflight extraction job %s: %w", path, read.err)
		}
		if read.exists && read.incompatible && read.document.NovelID == novelID {
			return read.guardError
		}
	}
	return nil
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

	termProfilePath := filepath.Join(stateDir, "term_profiles", novelID+".yaml")
	if deleted, err := removeIfExists(termProfilePath); err != nil {
		return NovelStatePruneResult{}, err
	} else {
		result.TermProfileDeleted = deleted
	}

	jobsDir := filepath.Join(stateDir, "extraction_jobs")
	indexPath := filepath.Join(jobsDir, "index", novelID+".yaml")
	if deleted, err := removeIfExists(indexPath); err != nil {
		return NovelStatePruneResult{}, err
	} else {
		result.JobIndexDeleted = deleted
	}

	jobPaths, err := filepath.Glob(filepath.Join(jobsDir, "*.yaml"))
	if err != nil {
		return NovelStatePruneResult{}, err
	}
	for _, path := range jobPaths {
		read := readJobDocument(path)
		if read.err != nil {
			log.Printf("extraction: skipping unreadable extraction job during prune %s: %v", path, read.err)
			continue
		} else if !read.exists || read.incompatible || read.document.NovelID != novelID {
			continue
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return NovelStatePruneResult{}, err
		}
		result.JobsDeleted++
	}

	checkpointPaths, err := filepath.Glob(filepath.Join(jobsDir, "checkpoints", "*.json"))
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
	return result, nil
}

func loadJobRecords(stateDir string) ([]JobWithNovel, error) {
	paths, err := filepath.Glob(filepath.Join(stateDir, "extraction_jobs", "*.yaml"))
	if err != nil {
		return nil, err
	}
	records := []JobWithNovel{}
	for _, path := range paths {
		read := readJobDocument(path)
		if read.err != nil {
			log.Printf("extraction: skipping unreadable extraction job %s: %v", path, read.err)
			continue
		} else if !read.exists {
			continue
		}
		records = append(records, JobWithNovel{NovelID: read.document.NovelID, Job: read.document.toJob()})
	}
	sort.SliceStable(records, func(i, j int) bool {
		return records[i].Job.CreatedAt > records[j].Job.CreatedAt
	})
	return records, nil
}

type jobDocumentRead struct {
	document     jobDocument
	exists       bool
	incompatible bool
	guardError   error
	err          error
}

func readJobDocument(path string) jobDocumentRead {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return jobDocumentRead{}
	}
	if err != nil {
		return jobDocumentRead{err: err}
	}
	_, guardErr := schemaguard.CheckYAML(raw, JobSchemaContract)
	var doc jobDocument
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		_, malformedErr := schemaguard.Malformed(JobSchemaContract, err)
		return jobDocumentRead{exists: true, err: malformedErr}
	}
	if guardErr == nil {
		return jobDocumentRead{document: doc, exists: true}
	}
	guardError, ok := schemaguard.AsGuardError(guardErr)
	if !ok || guardError.Result.Status == schemaguard.StatusMalformed {
		return jobDocumentRead{exists: true, err: guardErr}
	}
	if strings.TrimSpace(doc.JobID) == "" {
		doc.JobID = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	doc.Status = "incompatible"
	message := "この抽出 job は現在の build と互換性がないため、変更せず保持されています。"
	doc.ErrorMessage = &message
	return jobDocumentRead{document: doc, exists: true, incompatible: true, guardError: guardErr}
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
		CompletedBatchCount:       d.CompletedBatchCount,
		GeneratedCharacterCount:   d.GeneratedCharacterCount,
		GeneratedTermCount:        d.GeneratedTermCount,
		ActiveWorkers:             append([]ActiveWorker(nil), d.ActiveWorkers...),
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
		checkpointStore := checkpointstore.NewFileStore(stateDir)
		if checkpointStore.Exists(record.NovelID, job.RequestedUpToEpisodeIndex) {
			if _, checkpointErr := checkpointStore.Load(record.NovelID, job.RequestedUpToEpisodeIndex); checkpointErr != nil {
				if _, ok := schemaguard.AsGuardError(checkpointErr); !ok {
					return recovered, checkpointErr
				}
				incompatibleErr := checkpointStore.Quarantine(record.NovelID, job.RequestedUpToEpisodeIndex, "schema or payload validation failed", checkpointErr)
				if !checkpointstore.IsIncompatible(incompatibleErr) {
					return recovered, incompatibleErr
				}
				job.Status = "incompatible"
				message := incompatibleErr.Error()
				job.ErrorMessage = &message
				job.ActiveWorkers = nil
				if err := saveJobUnlocked(stateDir, record.NovelID, job); err != nil {
					return recovered, err
				}
				continue
			}
		}
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
		job.CompletedBatchCount = nil
		job.GeneratedCharacterCount = nil
		job.GeneratedTermCount = nil
		job.ActiveWorkers = nil
		if err := saveJobUnlocked(stateDir, record.NovelID, job); err != nil {
			return recovered, err
		}
		recovered++
	}
	return recovered, nil
}

func saveJobUnlocked(stateDir string, novelID string, job Job) error {
	doc := jobDocument{
		SchemaVersion:             jobSchemaVersion,
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
		CompletedBatchCount:       job.CompletedBatchCount,
		GeneratedCharacterCount:   job.GeneratedCharacterCount,
		GeneratedTermCount:        job.GeneratedTermCount,
		ActiveWorkers:             append([]ActiveWorker(nil), job.ActiveWorkers...),
		CreatedAt:                 job.CreatedAt,
		StartedAt:                 job.StartedAt,
		FinishedAt:                job.FinishedAt,
		ErrorMessage:              job.ErrorMessage,
	}
	fileName, err := safeJobFileName(job.JobID)
	if err != nil {
		return err
	}
	path := filepath.Join(stateDir, "extraction_jobs", fileName+".yaml")
	read := readJobDocument(path)
	if read.err != nil {
		return read.err
	}
	if read.incompatible {
		return read.guardError
	}
	if err := writeYAMLAtomic(path, doc); err != nil {
		return err
	}
	return saveJobIndex(stateDir, novelID, job)
}

func saveJobIndex(stateDir string, novelID string, job Job) error {
	path := filepath.Join(stateDir, "extraction_jobs", "index", novelID+".yaml")
	doc, err := loadOrRebuildJobIndex(stateDir, novelID, path)
	if err != nil {
		return err
	}
	doc.Revision++
	doc.SchemaVersion = jobIndexSchemaVersion
	doc.NovelID = novelID
	if job.Status == "queued" || job.Status == "running" {
		doc.ActiveJobID = &job.JobID
	} else if doc.ActiveJobID != nil && *doc.ActiveJobID == job.JobID {
		doc.ActiveJobID = nil
	}
	doc.JobIDs = prependUniqueJobID(doc.JobIDs, job.JobID)
	return writeYAMLAtomic(path, doc)
}

func loadOrRebuildJobIndex(stateDir string, novelID string, path string) (jobsIndexDocument, error) {
	var doc jobsIndexDocument
	_, err := yamlfile.ReadGuarded(path, JobIndexSchemaContract, &doc)
	if err == nil && doc.NovelID == novelID {
		if doc.JobIDs == nil {
			doc.JobIDs = []string{}
		}
		return doc, nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		if _, ok := schemaguard.AsGuardError(err); !ok {
			return jobsIndexDocument{}, err
		}
		if _, moveErr := filequarantine.Move(path, "unsupported"); moveErr != nil {
			return jobsIndexDocument{}, moveErr
		}
	} else if err == nil {
		if _, moveErr := filequarantine.Move(path, "unsupported"); moveErr != nil {
			return jobsIndexDocument{}, moveErr
		}
	}
	records, loadErr := loadJobRecords(stateDir)
	if loadErr != nil {
		return jobsIndexDocument{}, loadErr
	}
	rebuilt := jobsIndexDocument{
		SchemaVersion: jobIndexSchemaVersion,
		Revision:      0,
		NovelID:       novelID,
		JobIDs:        []string{},
	}
	for _, record := range records {
		if record.NovelID != novelID || strings.TrimSpace(record.Job.JobID) == "" {
			continue
		}
		rebuilt.JobIDs = append(rebuilt.JobIDs, record.Job.JobID)
		if rebuilt.ActiveJobID == nil && (record.Job.Status == "queued" || record.Job.Status == "running") {
			jobID := record.Job.JobID
			rebuilt.ActiveJobID = &jobID
		}
	}
	return rebuilt, nil
}

func RebuildJobIndex(stateDir string, novelID string) error {
	jobsMu.Lock()
	defer jobsMu.Unlock()

	novelID = strings.TrimSpace(novelID)
	if novelID == "" {
		return nil
	}
	return novelstate.WithLock(novelID, func() error {
		if err := preflightJobDocumentsUnlocked(stateDir, novelID); err != nil {
			return err
		}
		path := filepath.Join(stateDir, "extraction_jobs", "index", novelID+".yaml")
		if _, err := os.Stat(path); err == nil {
			if _, err := filequarantine.Move(path, "rebuild"); err != nil {
				return err
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		doc, err := loadOrRebuildJobIndex(stateDir, novelID, path)
		if err != nil {
			return err
		}
		doc.Revision++
		return writeYAMLAtomic(path, doc)
	})
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
