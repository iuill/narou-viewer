package application

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"narou-viewer/services/novel-fetcher/internal/model"
	"narou-viewer/services/novel-fetcher/internal/sites"
	"narou-viewer/services/novel-fetcher/internal/storage"
	"narou-viewer/services/novel-fetcher/internal/taskqueue"
	"narou-viewer/services/novel-fetcher/internal/taskstate"
)

type LibraryStore interface {
	FindWorkByID(id int) (model.StoredWork, bool, error)
	FindWorkBySiteKey(site string, siteWorkID string) (model.StoredWork, bool, error)
	FindPotentialDuplicateWorks(work model.Work) ([]model.StoredWork, error)
	ListEpisodes(workID int) ([]model.StoredEpisode, error)
	PreflightWorkMutation(storedWork model.StoredWork, incomingWork model.Work) error
	UpsertWorkToc(ctx context.Context, work model.Work, status string) (model.StoredWork, error)
	SaveEpisodeBody(ctx context.Context, work model.Work, stored model.StoredWork, episode model.Episode, sortOrder int) (model.StoredEpisode, error)
	MarkEpisodeFailed(ctx context.Context, workID int, episodeID string, fetchError error) error
	UpdateWorkFetchStatus(ctx context.Context, workID int, status string, failedEpisodeID string, resumeEpisodeID string, fetchError error) error
}

// TaskCheckpointStore is implemented by the durable library store. Keeping
// it optional preserves the small fake stores used by application tests and
// callers that only need the pre-persistence service contract.
type TaskCheckpointStore interface {
	IsTaskEpisodeCheckpointValid(ctx context.Context, ref taskstate.TaskRef, work model.Work, stored model.StoredWork, episode model.Episode, sortOrder int) (valid bool, found bool, err error)
	RecordTaskEpisodeCheckpoint(ctx context.Context, ref taskstate.TaskRef, workID int, episodeID string, sortOrder int, nextEpisodeID string) error
	SaveEpisodeBodyForTask(ctx context.Context, ref taskstate.TaskRef, work model.Work, stored model.StoredWork, episode model.Episode, sortOrder int, nextEpisodeID string) (model.StoredEpisode, error)
}

type TaskCompletionStore interface {
	CompleteWorkForTask(ctx context.Context, ref taskstate.TaskRef, workID int) error
}

type TaskReporter interface {
	SetTaskProgress(taskID string, progress sites.Progress)
	SetTaskMessage(taskID string, message string)
	AddTaskWarning(taskID string, warning string)
	SetTaskTarget(taskID string, target string)
	AddTaskNovelID(taskID string, novelID int) error
	SetTaskSavedEpisodeCount(taskID string, count int)
	SetTaskFailureEpisode(taskID string, failedEpisodeID string, resumeEpisodeID string)
}

type Service struct {
	store    LibraryStore
	fetcher  sites.WorkFetcher
	reporter TaskReporter
}

type Options struct {
	Store    LibraryStore
	Fetcher  sites.WorkFetcher
	Reporter TaskReporter
}

func NewService(options Options) *Service {
	return &Service{
		store:    options.Store,
		fetcher:  options.Fetcher,
		reporter: options.Reporter,
	}
}

func (s *Service) RunTask(ctx context.Context, next *taskqueue.Task) error {
	switch next.Kind {
	case "download":
		return s.runDownload(ctx, next)
	case "update":
		return s.runUpdate(ctx, next)
	case "resume":
		return s.runResume(ctx, next)
	default:
		return fmt.Errorf("unknown task kind: %s", next.Kind)
	}
}

func (s *Service) runDownload(ctx context.Context, next *taskqueue.Task) error {
	for _, target := range next.Targets {
		work, err := s.fetcher.FetchToc(ctx, target, s.progressReporter(next.ID))
		if err != nil {
			return err
		}
		s.reporter.SetTaskTarget(next.ID, work.Title)
		if err := s.rejectDuplicateTitleAcrossSites(next.ID, work); err != nil {
			return err
		}
		existingWork, previousEpisodes, err := s.existingStateForWork(work)
		if err != nil {
			return err
		}
		if err := s.store.PreflightWorkMutation(existingWork, work); err != nil {
			return err
		}
		stored, err := s.store.UpsertWorkToc(ctx, work, storage.FetchStatusPartial)
		if err != nil {
			return err
		}
		if err := s.reporter.AddTaskNovelID(next.ID, stored.ID); err != nil {
			return err
		}
		if err := s.fetchAndSaveEpisodes(ctx, next, work, stored, 0, !next.Force, previousEpisodes); err != nil {
			return err
		}
		if err := s.completeWork(ctx, next, stored.ID); err != nil {
			return err
		}
		s.reporter.SetTaskMessage(next.ID, fmt.Sprintf("saved %s", stored.Title))
	}
	return nil
}

func (s *Service) rejectDuplicateTitleAcrossSites(taskID string, work model.Work) error {
	matches, err := s.store.FindPotentialDuplicateWorks(work)
	if err != nil {
		return err
	}

	warnings := []string{}
	for _, match := range matches {
		if match.Site == work.Site && match.SiteWorkID == work.SiteWorkID {
			continue
		}
		if match.Site == work.Site {
			continue
		}
		warnings = append(warnings, fmt.Sprintf(
			"同名または近いタイトルの作品が別サイトにあります: %s（%s）",
			match.Title,
			match.SiteName,
		))
	}
	for _, warning := range warnings {
		s.reporter.AddTaskWarning(taskID, warning)
	}
	if len(warnings) > 0 {
		return fmt.Errorf("同名または近いタイトルの作品が別サイトにあるため、ダウンロードを取りやめました")
	}
	return nil
}

func (s *Service) runUpdate(ctx context.Context, next *taskqueue.Task) error {
	for _, id := range next.NovelIDs {
		work, ok, err := s.store.FindWorkByID(id)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("novel id %d was not found", id)
		}
		fetched, err := s.fetcher.FetchToc(ctx, work.SourceURL, s.progressReporter(next.ID))
		if err != nil {
			return err
		}
		s.reporter.SetTaskTarget(next.ID, fetched.Title)
		previousEpisodes, err := s.store.ListEpisodes(work.ID)
		if err != nil {
			return err
		}
		if err := s.store.PreflightWorkMutation(work, fetched); err != nil {
			return err
		}
		stored, err := s.store.UpsertWorkToc(ctx, fetched, storage.FetchStatusPartial)
		if err != nil {
			return err
		}
		if err := s.fetchAndSaveEpisodes(ctx, next, fetched, stored, 0, next.SkipUnchanged && !next.ForceRedownload, previousEpisodes); err != nil {
			return err
		}
		if err := s.completeWork(ctx, next, stored.ID); err != nil {
			return err
		}
		s.reporter.SetTaskMessage(next.ID, fmt.Sprintf("updated %s", stored.Title))
	}
	return nil
}

func (s *Service) runResume(ctx context.Context, next *taskqueue.Task) error {
	for _, id := range next.NovelIDs {
		work, ok, err := s.store.FindWorkByID(id)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("novel id %d was not found", id)
		}
		fetched, err := s.fetcher.FetchToc(ctx, work.SourceURL, s.progressReporter(next.ID))
		if err != nil {
			return err
		}
		s.reporter.SetTaskTarget(next.ID, fetched.Title)
		if err := s.store.PreflightWorkMutation(work, fetched); err != nil {
			return err
		}
		previousEpisodes, err := s.store.ListEpisodes(work.ID)
		if err != nil {
			return err
		}
		stored, err := s.store.UpsertWorkToc(ctx, fetched, storage.FetchStatusPartial)
		if err != nil {
			return err
		}
		if err := s.fetchAndSaveEpisodes(ctx, next, fetched, stored, 0, true, previousEpisodes); err != nil {
			return err
		}
		if err := s.completeWork(ctx, next, stored.ID); err != nil {
			return err
		}
		s.reporter.SetTaskMessage(next.ID, fmt.Sprintf("resumed %s", stored.Title))
	}
	return nil
}

func (s *Service) existingStateForWork(work model.Work) (model.StoredWork, []model.StoredEpisode, error) {
	stored, ok, err := s.store.FindWorkBySiteKey(string(work.Site), work.SiteWorkID)
	if err != nil || !ok {
		return model.StoredWork{}, nil, err
	}
	episodes, err := s.store.ListEpisodes(stored.ID)
	return stored, episodes, err
}

func (s *Service) fetchAndSaveEpisodes(ctx context.Context, next *taskqueue.Task, work model.Work, stored model.StoredWork, startIndex int, skipComplete bool, skipReferenceEpisodes []model.StoredEpisode) error {
	completeEpisodes := map[string]bool{}
	checkpointStore, hasCheckpoints := s.store.(TaskCheckpointStore)
	if skipComplete {
		if skipReferenceEpisodes == nil {
			existingEpisodes, err := s.store.ListEpisodes(stored.ID)
			if err != nil {
				return err
			}
			skipReferenceEpisodes = existingEpisodes
		}
		for _, episode := range skipReferenceEpisodes {
			if EpisodeCanBeSkipped(episode, work) {
				completeEpisodes[episode.EpisodeID] = true
			}
		}
	}

	totalEpisodes := len(work.Episodes)
	savedEpisodeCount := 0
	for index := startIndex; index < totalEpisodes; index++ {
		episodeRef := work.Episodes[index]
		episodeID := CanonicalTaskEpisodeID(episodeRef, index)
		nextEpisodeID := ""
		if index+1 < totalEpisodes {
			nextEpisodeID = CanonicalTaskEpisodeID(work.Episodes[index+1], index+1)
		}
		if hasCheckpoints && next.AttemptCount > 0 {
			valid, found, err := checkpointStore.IsTaskEpisodeCheckpointValid(ctx, taskstate.TaskRef{TaskID: next.ID, Attempt: next.AttemptCount}, work, stored, episodeRef, index)
			if err != nil {
				return err
			}
			if valid {
				savedEpisodeCount++
				s.reporter.SetTaskSavedEpisodeCount(next.ID, savedEpisodeCount)
				continue
			}
			// A checkpoint for an older source revision must force a refetch. Do
			// not fall back to the timestamp-based local skip path after the
			// stronger durable revision check has rejected it.
			if found {
				delete(completeEpisodes, episodeID)
			}
		}
		if completeEpisodes[episodeID] {
			if hasCheckpoints && next.AttemptCount > 0 {
				if err := checkpointStore.RecordTaskEpisodeCheckpoint(ctx, taskstate.TaskRef{TaskID: next.ID, Attempt: next.AttemptCount}, stored.ID, episodeID, index, nextEpisodeID); err != nil {
					if !errors.Is(err, storage.ErrInvalidTaskEpisodeCheckpoint) {
						return err
					}
					delete(completeEpisodes, episodeID)
				} else {
					savedEpisodeCount++
					s.reporter.SetTaskSavedEpisodeCount(next.ID, savedEpisodeCount)
					continue
				}
			} else {
				savedEpisodeCount++
				s.reporter.SetTaskSavedEpisodeCount(next.ID, savedEpisodeCount)
				continue
			}
		}

		fetched, err := s.fetcher.FetchEpisode(ctx, work, episodeRef, func(progress sites.Progress) {
			if progress.TotalSteps <= 1 {
				progress.CurrentStep = index + 1
				progress.TotalSteps = totalEpisodes
				progress.Message = fmt.Sprintf("%d / %d 話を取得中: %s", index+1, totalEpisodes, episodeRef.Title)
			}
			s.reporter.SetTaskProgress(next.ID, progress)
		})
		if err != nil {
			if controlErr := taskControlCause(ctx, err); controlErr != nil {
				s.markTaskControl(stored.ID, episodeID, controlErr)
				return controlErr
			}
			s.markEpisodeFailed(stored.ID, episodeID, err)
			s.reporter.SetTaskFailureEpisode(next.ID, episodeID, episodeID)
			return err
		}
		var saveErr error
		if hasCheckpoints && next.AttemptCount > 0 {
			_, saveErr = checkpointStore.SaveEpisodeBodyForTask(ctx, taskstate.TaskRef{TaskID: next.ID, Attempt: next.AttemptCount}, work, stored, fetched, index, nextEpisodeID)
		} else {
			_, saveErr = s.store.SaveEpisodeBody(ctx, work, stored, fetched, index)
		}
		if err := saveErr; err != nil {
			if controlErr := taskControlCause(ctx, err); controlErr != nil {
				s.markTaskControl(stored.ID, episodeID, controlErr)
				return controlErr
			}
			var unsupportedSchema storage.ErrUnsupportedEpisodeSchema
			if errors.As(err, &unsupportedSchema) {
				return err
			}
			s.markEpisodeFailed(stored.ID, episodeID, err)
			s.reporter.SetTaskFailureEpisode(next.ID, episodeID, episodeID)
			return err
		}
		savedEpisodeCount++
		s.reporter.SetTaskSavedEpisodeCount(next.ID, savedEpisodeCount)
	}
	return nil
}

func (s *Service) markEpisodeFailed(workID int, episodeID string, err error) {
	_ = s.store.MarkEpisodeFailed(context.Background(), workID, episodeID, err)
	status := storage.FetchStatusFailed
	if errors.Is(err, taskstate.ErrTaskCancelRequested) || errors.Is(err, context.Canceled) {
		status = storage.FetchStatusCanceled
	} else if errors.Is(err, taskstate.ErrTaskPauseRequested) {
		status = storage.FetchStatusPaused
	} else if errors.Is(err, taskstate.ErrRunnerShutdown) {
		status = storage.FetchStatusInterrupted
	}
	_ = s.store.UpdateWorkFetchStatus(context.Background(), workID, status, episodeID, episodeID, err)
}

func (s *Service) markTaskControl(workID int, episodeID string, err error) {
	status := storage.FetchStatusCanceled
	if errors.Is(err, taskstate.ErrTaskPauseRequested) {
		status = storage.FetchStatusPaused
	} else if errors.Is(err, taskstate.ErrRunnerShutdown) {
		status = storage.FetchStatusInterrupted
	}
	_ = s.store.UpdateWorkFetchStatus(context.Background(), workID, status, episodeID, episodeID, err)
}

func taskControlCause(ctx context.Context, err error) error {
	if ctx.Err() == nil {
		return nil
	}
	if cause := context.Cause(ctx); cause != nil {
		return cause
	}
	if err != nil {
		return err
	}
	return ctx.Err()
}

func (s *Service) completeWork(ctx context.Context, next *taskqueue.Task, workID int) error {
	if cause := taskControlCause(ctx, nil); cause != nil {
		s.markTaskControl(workID, "", cause)
		return cause
	}
	if store, ok := s.store.(TaskCompletionStore); ok && next.AttemptCount > 0 {
		return store.CompleteWorkForTask(ctx, taskstate.TaskRef{TaskID: next.ID, Attempt: next.AttemptCount}, workID)
	}
	return s.store.UpdateWorkFetchStatus(ctx, workID, storage.FetchStatusComplete, "", "", nil)
}

func EpisodeCanBeSkipped(stored model.StoredEpisode, work model.Work) bool {
	if stored.BodyStatus != storage.BodyStatusComplete {
		return false
	}

	for index, episode := range work.Episodes {
		if CanonicalTaskEpisodeID(episode, index) != stored.EpisodeID {
			continue
		}
		storedTimestamp := firstNonEmptyString(stored.UpdatedAt, stored.PublishedAt)
		fetchedTimestamp := firstNonEmptyString(episode.ModifiedAt, episode.PublishedAt)
		if storedTimestamp == "" || fetchedTimestamp == "" {
			return false
		}
		return storedTimestamp == fetchedTimestamp
	}

	return false
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func (s *Service) progressReporter(taskID string) sites.ProgressReporter {
	return func(progress sites.Progress) {
		s.reporter.SetTaskProgress(taskID, progress)
	}
}

func CanonicalTaskEpisodeID(episode model.Episode, fallback int) string {
	if episode.Index != "" {
		return episode.Index
	}
	return strconv.Itoa(fallback + 1)
}
