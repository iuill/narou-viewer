package fetchercommands

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"narou-viewer/apps/viewer-api-go/internal/application/removedstate"
	"narou-viewer/apps/viewer-api-go/internal/fetcher"
)

var ErrWorkIDResolverUnavailable = errors.New("fetcher work ID resolver unavailable")

type InvalidFetcherWorkIDError struct {
	WorkID string
}

func (e InvalidFetcherWorkIDError) Error() string {
	return "resolved fetcher work ID must be a positive integer"
}

type MissingNovelsError struct {
	NovelIDs []string
}

func (e MissingNovelsError) Error() string {
	return "some novelIds were not found in the local library"
}

type Client interface {
	Download(ctx context.Context, targets []string, force bool, convertAfterDownload bool, mail bool) (fetcher.DownloadResponse, error)
	Update(ctx context.Context, ids []int, forceRedownload bool, includeFrozen bool, convertAfterUpdate bool, skipUnchanged bool) (fetcher.UpdateResponse, error)
	Resume(ctx context.Context, ids []int) (fetcher.ResumeResponse, error)
	Remove(ctx context.Context, ids []string, withFiles bool) (fetcher.RemoveResponse, error)
	CancelTask(ctx context.Context, taskID string) (fetcher.CancelTaskResponse, error)
}

type WorkIDResolver interface {
	FetcherWorkID(novelID string) (string, bool, error)
}

type RemovedNovelStateCleaner interface {
	PruneRemovedNovelState(novelIDs []string) (removedstate.CleanupResult, error)
}

type Service struct {
	client   Client
	resolver WorkIDResolver
	cleaner  RemovedNovelStateCleaner
}

type DownloadOptions struct {
	Force                bool
	ConvertAfterDownload bool
	Mail                 bool
}

type UpdateOptions struct {
	ForceRedownload    bool
	IncludeFrozen      bool
	ConvertAfterUpdate bool
	SkipUnchanged      bool
}

type DownloadResult struct {
	Targets              []string `json:"targets"`
	Force                bool     `json:"force"`
	ConvertAfterDownload bool     `json:"convertAfterDownload"`
	Mail                 bool     `json:"mail"`
	TaskIDs              []string `json:"taskIds"`
	Message              string   `json:"message"`
}

type UpdateResult struct {
	IDs                []string `json:"ids"`
	NovelIDs           []string `json:"novelIds"`
	FetcherWorkIDs     []string `json:"fetcherWorkIds"`
	ForceRedownload    bool     `json:"forceRedownload"`
	IncludeFrozen      bool     `json:"includeFrozen"`
	ConvertAfterUpdate bool     `json:"convertAfterUpdate"`
	SkipUnchanged      bool     `json:"skipUnchanged"`
	TaskIDs            []string `json:"taskIds"`
	Message            string   `json:"message"`
}

type ResumeResult struct {
	IDs            []string `json:"ids"`
	NovelIDs       []string `json:"novelIds"`
	FetcherWorkIDs []string `json:"fetcherWorkIds"`
	TaskIDs        []string `json:"taskIds"`
	Message        string   `json:"message"`
}

type RemoveResult struct {
	IDs                      []string                    `json:"ids"`
	NovelIDs                 []string                    `json:"novelIds"`
	FetcherWorkIDs           []string                    `json:"fetcherWorkIds"`
	WithFiles                bool                        `json:"withFiles"`
	ViewerStateCleanup       *removedstate.CleanupResult `json:"viewerStateCleanup,omitempty"`
	ViewerStateCleanupStatus string                      `json:"viewerStateCleanupStatus,omitempty"`
	ViewerStateCleanupError  string                      `json:"viewerStateCleanupError,omitempty"`
	Message                  string                      `json:"message"`
}

type CancelTaskResult struct {
	TaskID    string `json:"taskId"`
	Cancelled bool   `json:"cancelled"`
	Message   string `json:"message"`
}

func NewService(client Client, resolver WorkIDResolver) *Service {
	return &Service{client: client, resolver: resolver}
}

func (s *Service) WithRemovedNovelStateCleaner(cleaner RemovedNovelStateCleaner) *Service {
	s.cleaner = cleaner
	return s
}

func (s *Service) Download(ctx context.Context, targets []string, options DownloadOptions) (DownloadResult, error) {
	result, err := s.client.Download(ctx, targets, options.Force, options.ConvertAfterDownload, options.Mail)
	if err != nil {
		return DownloadResult{}, err
	}
	return DownloadResult{
		Targets:              result.Targets,
		Force:                result.Force,
		ConvertAfterDownload: result.ConvertAfterDownload,
		Mail:                 result.Mail,
		TaskIDs:              result.TaskIDs,
		Message:              messageOrFallback(result.Message, "Download started"),
	}, nil
}

func (s *Service) Update(ctx context.Context, novelIDs []string, options UpdateOptions) (UpdateResult, error) {
	workIDs, err := s.fetcherWorkIDs(novelIDs)
	if err != nil {
		return UpdateResult{}, err
	}
	updateIDs, err := workIDs.asInts()
	if err != nil {
		return UpdateResult{}, err
	}
	result, err := s.client.Update(
		ctx,
		updateIDs,
		options.ForceRedownload,
		options.IncludeFrozen,
		options.ConvertAfterUpdate,
		options.SkipUnchanged,
	)
	if err != nil {
		return UpdateResult{}, err
	}
	return UpdateResult{
		IDs:                result.IDs,
		NovelIDs:           append([]string{}, novelIDs...),
		FetcherWorkIDs:     workIDs.asStrings(),
		ForceRedownload:    result.ForceRedownload,
		IncludeFrozen:      result.IncludeFrozen,
		ConvertAfterUpdate: result.ConvertAfterUpdate,
		SkipUnchanged:      result.SkipUnchanged,
		TaskIDs:            result.TaskIDs,
		Message:            messageOrFallback(result.Message, "Update started"),
	}, nil
}

func (s *Service) Resume(ctx context.Context, novelIDs []string) (ResumeResult, error) {
	workIDs, err := s.fetcherWorkIDs(novelIDs)
	if err != nil {
		return ResumeResult{}, err
	}
	resumeIDs, err := workIDs.asInts()
	if err != nil {
		return ResumeResult{}, err
	}
	result, err := s.client.Resume(ctx, resumeIDs)
	if err != nil {
		return ResumeResult{}, err
	}
	fetcherWorkIDs := result.IDs
	if len(fetcherWorkIDs) == 0 {
		fetcherWorkIDs = workIDs.asStrings()
	}
	return ResumeResult{
		IDs:            result.IDs,
		NovelIDs:       append([]string{}, novelIDs...),
		FetcherWorkIDs: fetcherWorkIDs,
		TaskIDs:        result.TaskIDs,
		Message:        messageOrFallback(result.Message, "Resume started"),
	}, nil
}

func (s *Service) Remove(ctx context.Context, novelIDs []string, withFiles bool) (RemoveResult, error) {
	workIDs, err := s.fetcherWorkIDs(novelIDs)
	if err != nil {
		return RemoveResult{}, err
	}
	result, err := s.client.Remove(ctx, workIDs.asStrings(), withFiles)
	if err != nil {
		return RemoveResult{}, err
	}
	response := RemoveResult{
		IDs:            result.IDs,
		NovelIDs:       append([]string{}, novelIDs...),
		FetcherWorkIDs: workIDs.asStrings(),
		WithFiles:      withFiles,
		Message:        messageOrFallback(result.Message, "Novel removal started"),
	}
	if s.cleaner != nil {
		cleanup, cleanupErr := s.cleaner.PruneRemovedNovelState(novelIDs)
		response.ViewerStateCleanup = &cleanup
		if cleanupErr != nil {
			response.ViewerStateCleanupStatus = "partial"
			response.ViewerStateCleanupError = "Failed to clean up removed novel state."
		} else {
			response.ViewerStateCleanupStatus = "ok"
		}
	}
	return response, nil
}

func (s *Service) CancelTask(ctx context.Context, taskID string) (CancelTaskResult, error) {
	result, err := s.client.CancelTask(ctx, taskID)
	if err != nil {
		return CancelTaskResult{}, err
	}
	responseTaskID := strings.TrimSpace(result.TaskID)
	if responseTaskID == "" {
		responseTaskID = taskID
	}
	return CancelTaskResult{
		TaskID:    responseTaskID,
		Cancelled: result.Cancelled,
		Message:   messageOrFallback(result.Message, "Task cancelled"),
	}, nil
}

type fetcherWorkIDs []string

func (ids fetcherWorkIDs) asStrings() []string {
	return append([]string{}, ids...)
}

func (ids fetcherWorkIDs) asInts() ([]int, error) {
	result := make([]int, 0, len(ids))
	for _, id := range ids {
		parsed, err := strconv.Atoi(id)
		if err != nil || parsed <= 0 {
			return nil, InvalidFetcherWorkIDError{WorkID: id}
		}
		result = append(result, parsed)
	}
	return result, nil
}

func (s *Service) fetcherWorkIDs(novelIDs []string) (fetcherWorkIDs, error) {
	if s == nil || s.resolver == nil {
		return nil, ErrWorkIDResolverUnavailable
	}
	result := make(fetcherWorkIDs, 0, len(novelIDs))
	missing := make([]string, 0)
	for _, novelID := range novelIDs {
		workID, ok, err := s.resolver.FetcherWorkID(novelID)
		if err != nil {
			return nil, err
		}
		if !ok {
			missing = append(missing, novelID)
			continue
		}
		normalizedWorkID, err := normalizeFetcherWorkID(workID)
		if err != nil {
			return nil, err
		}
		result = append(result, normalizedWorkID)
	}
	if len(missing) > 0 {
		return nil, MissingNovelsError{NovelIDs: missing}
	}
	return result, nil
}

func normalizeFetcherWorkID(workID string) (string, error) {
	normalized := strings.TrimSpace(workID)
	parsed, err := strconv.Atoi(normalized)
	if err != nil || parsed <= 0 {
		return "", InvalidFetcherWorkIDError{WorkID: workID}
	}
	return strconv.Itoa(parsed), nil
}

func messageOrFallback(value string, fallback string) string {
	if text := strings.TrimSpace(value); text != "" {
		return text
	}
	return fallback
}
