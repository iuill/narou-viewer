package extractionjobs

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"narou-viewer/apps/viewer-api-go/internal/ai"
	appextraction "narou-viewer/apps/viewer-api-go/internal/application/extraction"
	extractdomain "narou-viewer/apps/viewer-api-go/internal/extraction"
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
	ErrExtractionClear           = errors.New("extraction state could not be cleared")
	ErrExtractionActive          = errors.New("extraction is still running")
)

type LibraryPort interface {
	FindWork(string) (library.Work, bool, error)
	FindEpisode(int, string) (library.Episode, bool, error)
}

type SettingsPort interface {
	GetAIGenerationSettings() (ai.SettingsResponse, error)
}

type JobsResponse struct {
	Jobs []extractdomain.Job `json:"jobs"`
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
	Message                      string `json:"message"`
	CharacterProfileDeleted      bool   `json:"characterProfileDeleted"`
	CharacterEventsDeleted       bool   `json:"characterEventsDeleted"`
	TermProfileDeleted           bool   `json:"termProfileDeleted"`
	ExtractionJobsDeleted        int    `json:"extractionJobsDeleted"`
	ExtractionJobIndexDeleted    bool   `json:"extractionJobIndexDeleted"`
	ExtractionCheckpointsDeleted int    `json:"extractionCheckpointsDeleted"`
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
	loadedJobs, ok, err := extractdomain.LoadJobs(s.stateDir, novelID)
	if err != nil {
		return JobsResponse{}, ErrJobsRead
	}
	if !ok {
		loadedJobs = []extractdomain.Job{}
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
	job := extractdomain.Job{
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
	savedJob, created, err := extractdomain.SaveJobIfNoActive(s.stateDir, novelID, job)
	if err != nil {
		return EnqueueResponse{}, false, ErrJobSave
	}
	return enqueueResponse(savedJob, created), created, nil
}

func (s *Service) Clear(ctx context.Context, novelID string) (ClearResponse, error) {
	if err := s.ensureNovelExists(ctx, novelID); err != nil {
		return ClearResponse{}, err
	}
	result, active, err := extractdomain.PruneNovelStateIfNoActive(s.stateDir, novelID)
	if err != nil {
		return ClearResponse{}, ErrExtractionClear
	}
	if active {
		return ClearResponse{}, ErrExtractionActive
	}
	return ClearResponse{
		Message:                      "抽出データをクリアしました。",
		CharacterProfileDeleted:      result.ProfileDeleted,
		CharacterEventsDeleted:       result.EventsDeleted,
		TermProfileDeleted:           result.TermProfileDeleted,
		ExtractionJobsDeleted:        result.JobsDeleted,
		ExtractionJobIndexDeleted:    result.JobIndexDeleted,
		ExtractionCheckpointsDeleted: result.CheckpointsDeleted,
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
	generationStrategy := appextraction.GenerationStrategySerial
	if raw == nil {
		return generationStrategy, nil
	}
	strategy := *raw
	normalized := appextraction.NormalizeGenerationStrategy(strategy)
	if strings.TrimSpace(strategy) != "" && normalized != strings.TrimSpace(strategy) {
		return "", ErrInvalidGenerationStrategy
	}
	return normalized, nil
}

func enqueueResponse(job extractdomain.Job, created bool) EnqueueResponse {
	message := "人物と用語の抽出を依頼しました。"
	if !created {
		message = "この作品では既に人物と用語の抽出が進行中です。進行中の処理を表示します。"
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
