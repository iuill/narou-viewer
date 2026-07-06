package charactersummary

import (
	"context"
	"reflect"

	"narou-viewer/apps/viewer-api-go/internal/ai"
	"narou-viewer/apps/viewer-api-go/internal/characters"
	"narou-viewer/apps/viewer-api-go/internal/library"
	"narou-viewer/apps/viewer-api-go/internal/store"
)

type Library interface {
	GetToc(ctx context.Context, novelID string) (*library.TocResponse, error)
}

type SummaryRepository interface {
	LoadForEpisodes(stateDir string, novelID string, upToEpisodeIndex string, episodeIndexes []string) (characters.SummaryResponse, bool, error)
}

type filesystemSummaryRepository struct{}

func (filesystemSummaryRepository) LoadForEpisodes(stateDir string, novelID string, upToEpisodeIndex string, episodeIndexes []string) (characters.SummaryResponse, bool, error) {
	return characters.LoadSummaryForEpisodes(stateDir, novelID, upToEpisodeIndex, episodeIndexes)
}

type Service struct {
	stateDir   string
	library    Library
	settings   SettingsProvider
	generator  Generator
	repository SummaryRepository
}

func NewService(stateDir string, library Library, settings SettingsProvider, generator Generator) *Service {
	return &Service{
		stateDir:   stateDir,
		library:    library,
		settings:   normalizeSettingsProvider(settings),
		generator:  generator,
		repository: filesystemSummaryRepository{},
	}
}

func normalizeSettingsProvider(settings SettingsProvider) SettingsProvider {
	if settings == nil {
		return nil
	}
	value := reflect.ValueOf(settings)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		if value.IsNil() {
			return nil
		}
	}
	return settings
}

func (s *Service) WithSummaryRepository(repository SummaryRepository) *Service {
	if repository != nil {
		s.repository = repository
	}
	return s
}

func (s *Service) Result(ctx context.Context, novelID string, upToEpisodeIndex string, options RequestOptions) (Result, error) {
	novelTitle, episodeIndexes := s.tocContext(ctx, novelID)
	profile, generationMode, resolvedProfile, err := s.resolveGenerationContext(options)
	if err != nil {
		return Result{}, err
	}

	summary, err := s.resolveSummary(ctx, novelID, upToEpisodeIndex, episodeIndexes, resolvedProfile, options)
	if err != nil {
		return Result{}, err
	}

	return Result{
		NovelID:                   summary.NovelID,
		NovelTitle:                novelTitle,
		UpToEpisodeIndex:          summary.UpToEpisodeIndex,
		ProcessedUpToEpisodeIndex: summary.ProcessedUpToEpisodeIndex,
		ProfileID:                 profileID(profile),
		ProfileLabel:              profileLabel(profile),
		GenerationMode:            generationMode,
		GenerationStrategy:        NormalizeGenerationStrategy(options.GenerationStrategy),
		ModelID:                   profileModelID(profile),
		Characters:                summary.Characters,
	}, nil
}

func (s *Service) tocContext(ctx context.Context, novelID string) (string, []string) {
	if s == nil || s.library == nil {
		return "", nil
	}
	toc, err := s.library.GetToc(ctx, novelID)
	if err != nil || toc == nil {
		return "", nil
	}
	return toc.Title, episodeIndexesFromToc(toc.Episodes)
}

func (s *Service) resolveGenerationContext(options RequestOptions) (*ai.Profile, string, *store.ResolvedAIGenerationConfig, error) {
	var profile *ai.Profile
	generationMode := "heuristic"
	if s != nil && s.settings != nil {
		settings, err := s.settings.GetAIGenerationSettings()
		if err != nil {
			return nil, "", nil, err
		}
		profile = resolveActiveAIProfile(settings)
		if settings.EffectiveGenerationMode != "" {
			generationMode = settings.EffectiveGenerationMode
		}
	}
	var resolvedProfile *store.ResolvedAIGenerationConfig
	if options.ProfileResolution {
		resolvedProfile = options.ResolvedConfig
		if resolvedProfile != nil {
			generationMode = "openrouter"
			profile = &ai.Profile{
				ID:       resolvedProfile.ProfileID,
				Label:    resolvedProfile.ProfileLabel,
				Provider: "openrouter",
				ModelID:  &resolvedProfile.ModelID,
			}
		}
	}
	return profile, generationMode, resolvedProfile, nil
}

func (s *Service) resolveSummary(ctx context.Context, novelID string, upToEpisodeIndex string, episodeIndexes []string, resolvedProfile *store.ResolvedAIGenerationConfig, options RequestOptions) (characters.SummaryResponse, error) {
	if s == nil || s.repository == nil {
		return emptyReadySummary(novelID, upToEpisodeIndex), nil
	}
	if options.PreviewOnly {
		loaded, ok, err := s.repository.LoadForEpisodes(s.stateDir, novelID, upToEpisodeIndex, episodeIndexes)
		if err != nil {
			return characters.SummaryResponse{}, err
		}
		if ok && loaded.Status == "ready" && !options.ProfileResolution {
			return loaded, nil
		}
		if s.generator == nil {
			return emptyReadySummary(novelID, upToEpisodeIndex), nil
		}
		unlock := s.generator.LockTarget(novelID, upToEpisodeIndex)
		defer unlock()
		return s.generator.GeneratePreview(ctx, novelID, upToEpisodeIndex, resolvedProfile, options.GenerationStrategy, options.BatchProgressSink, episodeIndexes, options.SummaryInputs)
	}

	loaded, ok, err := s.repository.LoadForEpisodes(s.stateDir, novelID, upToEpisodeIndex, episodeIndexes)
	if err != nil {
		return characters.SummaryResponse{}, err
	}
	if ok && loaded.Status == "ready" && !options.ProfileResolution {
		return loaded, nil
	}
	if s.generator != nil {
		if err := s.generator.GenerateAndSave(ctx, novelID, upToEpisodeIndex, resolvedProfile, options.GenerationStrategy, options.BatchProgressSink); err != nil {
			return characters.SummaryResponse{}, err
		}
	}
	loaded, ok, err = s.repository.LoadForEpisodes(s.stateDir, novelID, upToEpisodeIndex, episodeIndexes)
	if err != nil {
		return characters.SummaryResponse{}, err
	}
	if ok {
		return loaded, nil
	}
	return emptyReadySummary(novelID, upToEpisodeIndex), nil
}

func emptyReadySummary(novelID string, upToEpisodeIndex string) characters.SummaryResponse {
	return characters.SummaryResponse{
		Status:                    "ready",
		NovelID:                   novelID,
		UpToEpisodeIndex:          upToEpisodeIndex,
		ProcessedUpToEpisodeIndex: &upToEpisodeIndex,
		Characters:                []characters.Character{},
	}
}

func episodeIndexesFromToc(episodes []library.TocEpisodeSummary) []string {
	values := make([]string, 0, len(episodes))
	for _, episode := range episodes {
		values = append(values, episode.EpisodeIndex)
	}
	return values
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
