package characterjobs

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"narou-viewer/apps/viewer-api-go/internal/ai"
	appcharactersummary "narou-viewer/apps/viewer-api-go/internal/application/charactersummary"
	"narou-viewer/apps/viewer-api-go/internal/characters"
	"narou-viewer/apps/viewer-api-go/internal/library"
)

var (
	ErrNovelNotFound             = errors.New("novel not found")
	ErrEpisodeOutOfRange         = errors.New("episode out of range")
	ErrInvalidUpToEpisodeIndex   = errors.New("invalid upToEpisodeIndex")
	ErrInvalidGenerationStrategy = errors.New("invalid generationStrategy")
	ErrJobsRead                  = errors.New("character jobs could not be read")
	ErrJobSave                   = errors.New("character job could not be saved")
	ErrSettingsRead              = errors.New("AI generation settings could not be read")
	ErrSummaryClear              = errors.New("character summary state could not be cleared")
	ErrSummaryActive             = errors.New("character summary generation is still running")
)

type LibraryPort interface {
	FindWork(string) (library.Work, bool, error)
	FindEpisode(int, string) (library.Episode, bool, error)
}

type SettingsPort interface {
	GetAIGenerationSettings() (ai.SettingsResponse, error)
}

type JobsResponse struct {
	Jobs []characters.Job `json:"jobs"`
}

type EnqueueInput struct {
	UpToEpisodeIndex   string
	GenerationStrategy *string
}

type EnqueueResponse struct {
	JobID                     string `json:"jobId"`
	RequestedUpToEpisodeIndex string `json:"requestedUpToEpisodeIndex"`
	Status                    string `json:"status"`
	GenerationStrategy        string `json:"generationStrategy"`
	Message                   string `json:"message"`
}

type ClearResponse struct {
	Message            string `json:"message"`
	ProfileDeleted     bool   `json:"profileDeleted"`
	EventsDeleted      bool   `json:"eventsDeleted"`
	JobsDeleted        int    `json:"jobsDeleted"`
	JobIndexDeleted    bool   `json:"jobIndexDeleted"`
	CheckpointsDeleted int    `json:"checkpointsDeleted"`
}

type Service struct {
	stateDir string
	library  LibraryPort
	settings SettingsPort
	now      func() time.Time
	nowISO   func() string
}

func NewService(stateDir string, library LibraryPort, settings SettingsPort) *Service {
	return &Service{
		stateDir: stateDir,
		library:  library,
		settings: settings,
		now:      time.Now,
		nowISO:   ai.NowISO,
	}
}

func (s *Service) List(ctx context.Context, novelID string) (JobsResponse, error) {
	if err := s.ensureNovelExists(ctx, novelID); err != nil {
		return JobsResponse{}, err
	}
	loadedJobs, ok, err := characters.LoadJobs(s.stateDir, novelID)
	if err != nil {
		return JobsResponse{}, ErrJobsRead
	}
	if !ok {
		loadedJobs = []characters.Job{}
	}
	return JobsResponse{Jobs: loadedJobs}, nil
}

func (s *Service) Enqueue(ctx context.Context, novelID string, input EnqueueInput) (EnqueueResponse, bool, error) {
	upToEpisodeIndex := strings.TrimSpace(input.UpToEpisodeIndex)
	if !isDigits(upToEpisodeIndex) {
		return EnqueueResponse{}, false, ErrInvalidUpToEpisodeIndex
	}
	if err := s.ensureNovelExists(ctx, novelID); err != nil {
		return EnqueueResponse{}, false, err
	}
	if err := s.ensureEpisodeExists(ctx, novelID, upToEpisodeIndex); err != nil {
		return EnqueueResponse{}, false, err
	}
	generationStrategy, err := normalizeGenerationStrategy(input.GenerationStrategy)
	if err != nil {
		return EnqueueResponse{}, false, err
	}
	if s == nil || s.settings == nil {
		return EnqueueResponse{}, false, ErrSettingsRead
	}
	settings, err := s.settings.GetAIGenerationSettings()
	if err != nil {
		return EnqueueResponse{}, false, ErrSettingsRead
	}
	activeProfile := resolveActiveAIProfile(settings)
	now := s.nowISOValue()
	job := characters.Job{
		JobID:                     "go-job-" + strconv.FormatInt(s.nowValue().UnixNano(), 10),
		RequestedUpToEpisodeIndex: upToEpisodeIndex,
		ProfileID:                 profileID(activeProfile),
		ProfileLabel:              profileLabel(activeProfile),
		GenerationMode:            settings.EffectiveGenerationMode,
		GenerationStrategy:        generationStrategy,
		ModelID:                   profileModelID(activeProfile),
		Status:                    "queued",
		CreatedAt:                 now,
		StartedAt:                 nil,
		FinishedAt:                nil,
		ErrorMessage:              nil,
	}
	savedJob, created, err := characters.SaveJobIfNoActive(s.stateDir, novelID, job)
	if err != nil {
		return EnqueueResponse{}, false, ErrJobSave
	}
	return enqueueResponse(savedJob, created), created, nil
}

func (s *Service) Clear(ctx context.Context, novelID string) (ClearResponse, error) {
	if err := s.ensureNovelExists(ctx, novelID); err != nil {
		return ClearResponse{}, err
	}
	result, active, err := characters.PruneNovelStateIfNoActive(s.stateDir, novelID)
	if err != nil {
		return ClearResponse{}, ErrSummaryClear
	}
	if active {
		return ClearResponse{}, ErrSummaryActive
	}
	return ClearResponse{
		Message:            "キャラクター一覧生成データをクリアしました。",
		ProfileDeleted:     result.ProfileDeleted,
		EventsDeleted:      result.EventsDeleted,
		JobsDeleted:        result.JobsDeleted,
		JobIndexDeleted:    result.JobIndexDeleted,
		CheckpointsDeleted: result.CheckpointsDeleted,
	}, nil
}

func (s *Service) ensureNovelExists(_ context.Context, novelID string) error {
	if s == nil || s.library == nil {
		return ErrNovelNotFound
	}
	_, found, err := s.library.FindWork(novelID)
	if err != nil {
		return err
	}
	if !found {
		return ErrNovelNotFound
	}
	return nil
}

func (s *Service) ensureEpisodeExists(_ context.Context, novelID string, episodeIndex string) error {
	if s == nil || s.library == nil {
		return ErrEpisodeOutOfRange
	}
	work, found, err := s.library.FindWork(novelID)
	if err != nil {
		return err
	}
	if !found {
		return ErrNovelNotFound
	}
	_, found, err = s.library.FindEpisode(work.ID, episodeIndex)
	if err != nil {
		return err
	}
	if !found {
		return ErrEpisodeOutOfRange
	}
	return nil
}

func normalizeGenerationStrategy(raw *string) (string, error) {
	generationStrategy := appcharactersummary.GenerationStrategySerial
	if raw == nil {
		return generationStrategy, nil
	}
	strategy := *raw
	normalized := appcharactersummary.NormalizeGenerationStrategy(strategy)
	if strings.TrimSpace(strategy) != "" && normalized != strings.TrimSpace(strategy) {
		return "", ErrInvalidGenerationStrategy
	}
	return normalized, nil
}

func enqueueResponse(job characters.Job, created bool) EnqueueResponse {
	message := "キャラクター一覧生成を依頼しました。"
	if !created {
		message = "この作品では既にキャラクター一覧生成が進行中です。進行中の生成を表示します。"
	}
	return EnqueueResponse{
		JobID:                     job.JobID,
		RequestedUpToEpisodeIndex: job.RequestedUpToEpisodeIndex,
		Status:                    job.Status,
		GenerationStrategy:        job.GenerationStrategy,
		Message:                   message,
	}
}

func (s *Service) nowValue() time.Time {
	if s != nil && s.now != nil {
		return s.now()
	}
	return time.Now()
}

func (s *Service) nowISOValue() string {
	if s != nil && s.nowISO != nil {
		return s.nowISO()
	}
	return ai.NowISO()
}

func isDigits(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func resolveActiveAIProfile(settings ai.SettingsResponse) *ai.Profile {
	profiles := settings.Settings.Profiles
	if settings.Settings.SelectedProfileID != nil {
		for i := range profiles {
			if profiles[i].ID == *settings.Settings.SelectedProfileID {
				return &profiles[i]
			}
		}
	}
	if len(profiles) == 0 {
		return nil
	}
	return &profiles[0]
}

func profileID(profile *ai.Profile) *string {
	if profile == nil {
		return nil
	}
	return &profile.ID
}

func profileLabel(profile *ai.Profile) *string {
	if profile == nil {
		return nil
	}
	return &profile.Label
}

func profileModelID(profile *ai.Profile) *string {
	if profile == nil {
		return nil
	}
	return profile.ModelID
}
