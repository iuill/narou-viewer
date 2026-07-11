package extraction

import (
	"context"
	"errors"
	"testing"

	"narou-viewer/apps/viewer-api-go/internal/ai"
	"narou-viewer/apps/viewer-api-go/internal/characters"
	"narou-viewer/apps/viewer-api-go/internal/library"
	"narou-viewer/apps/viewer-api-go/internal/store"
	"narou-viewer/apps/viewer-api-go/internal/terms"
)

type fakeLibrary struct {
	toc *library.TocResponse
}

func (f fakeLibrary) GetToc(context.Context, string) (*library.TocResponse, error) {
	return f.toc, nil
}

type fakeSettings struct {
	response ai.SettingsResponse
	err      error
}

func (f fakeSettings) GetAIGenerationSettings() (ai.SettingsResponse, error) {
	return f.response, f.err
}

type nilableSettings struct{}

func (*nilableSettings) GetAIGenerationSettings() (ai.SettingsResponse, error) {
	return ai.SettingsResponse{}, nil
}

type fakeRepository struct {
	loads     []fakeRepositoryLoad
	loadCalls []fakeRepositoryCall
}

type fakeRepositoryLoad struct {
	summary characters.SummaryResponse
	ok      bool
	err     error
}

type fakeRepositoryCall struct {
	stateDir         string
	novelID          string
	upToEpisodeIndex string
	episodeIndexes   []string
}

func (f *fakeRepository) LoadForEpisodes(stateDir string, novelID string, upToEpisodeIndex string, episodeIndexes []string) (characters.SummaryResponse, bool, error) {
	f.loadCalls = append(f.loadCalls, fakeRepositoryCall{
		stateDir:         stateDir,
		novelID:          novelID,
		upToEpisodeIndex: upToEpisodeIndex,
		episodeIndexes:   append([]string{}, episodeIndexes...),
	})
	if len(f.loads) == 0 {
		return characters.SummaryResponse{}, false, nil
	}
	next := f.loads[0]
	f.loads = f.loads[1:]
	return next.summary, next.ok, next.err
}

type fakeGenerator struct {
	lockCalls         []fakeGeneratorLockCall
	generateSaveCount int
	previewCount      int
	preview           characters.SummaryResponse
	generateSaveCall  fakeGeneratorGenerateSaveCall
	previewCall       fakeGeneratorPreviewCall
}

type fakeGeneratorLockCall struct {
	novelID          string
	upToEpisodeIndex string
}

type fakeGeneratorGenerateSaveCall struct {
	novelID          string
	upToEpisodeIndex string
	resolvedConfig   *store.ResolvedAIGenerationConfig
	strategy         string
	hasProgressSink  bool
}

type fakeGeneratorPreviewCall struct {
	novelID          string
	upToEpisodeIndex string
	resolvedConfig   *store.ResolvedAIGenerationConfig
	strategy         string
	hasProgressSink  bool
	episodeIndexes   []string
	inputs           *Inputs
}

func (f *fakeGenerator) LockTarget(novelID string, upToEpisodeIndex string) func() {
	f.lockCalls = append(f.lockCalls, fakeGeneratorLockCall{novelID: novelID, upToEpisodeIndex: upToEpisodeIndex})
	return func() {}
}

func (f *fakeGenerator) GenerateAndSave(_ context.Context, novelID string, upToEpisodeIndex string, resolvedConfig *store.ResolvedAIGenerationConfig, strategy string, progressSink func(BatchProgress)) (FinalCounts, error) {
	f.generateSaveCount++
	f.generateSaveCall = fakeGeneratorGenerateSaveCall{
		novelID:          novelID,
		upToEpisodeIndex: upToEpisodeIndex,
		resolvedConfig:   resolvedConfig,
		strategy:         strategy,
		hasProgressSink:  progressSink != nil,
	}
	return FinalCounts{}, nil
}

func (f *fakeGenerator) GeneratePreview(_ context.Context, novelID string, upToEpisodeIndex string, resolvedConfig *store.ResolvedAIGenerationConfig, strategy string, progressSink func(BatchProgress), episodeIndexes []string, inputs *Inputs) (Result, error) {
	f.previewCount++
	f.previewCall = fakeGeneratorPreviewCall{
		novelID:          novelID,
		upToEpisodeIndex: upToEpisodeIndex,
		resolvedConfig:   resolvedConfig,
		strategy:         strategy,
		hasProgressSink:  progressSink != nil,
		episodeIndexes:   append([]string{}, episodeIndexes...),
		inputs:           inputs,
	}
	return Result{NovelID: f.preview.NovelID, UpToEpisodeIndex: f.preview.UpToEpisodeIndex, ProcessedUpToEpisodeIndex: f.preview.ProcessedUpToEpisodeIndex, Characters: f.preview.Characters, Terms: []terms.Term{}}, nil
}

func readySummary(novelID string, upToEpisodeIndex string, names ...string) characters.SummaryResponse {
	values := make([]characters.Character, 0, len(names))
	for _, name := range names {
		values = append(values, characters.Character{CanonicalName: name})
	}
	return characters.SummaryResponse{
		Status:                    "ready",
		NovelID:                   novelID,
		UpToEpisodeIndex:          upToEpisodeIndex,
		ProcessedUpToEpisodeIndex: &upToEpisodeIndex,
		Characters:                values,
	}
}

func TestServiceSmallHelpers(t *testing.T) {
	empty := emptyReadySummary("novel-empty", "5")
	if empty.Status != "ready" || empty.ProcessedUpToEpisodeIndex == nil || *empty.ProcessedUpToEpisodeIndex != "5" || len(empty.Characters) != 0 {
		t.Fatalf("emptyReadySummary = %+v", empty)
	}

	selected := "profile-b"
	settings := ai.SettingsResponse{Settings: ai.SettingsMetadata{
		SelectedProfileID: &selected,
		Profiles: []ai.Profile{
			{ID: "profile-a", Label: "A"},
			{ID: "profile-b", Label: "B"},
		},
	}}
	if profile := resolveActiveAIProfile(settings); profile == nil || profile.ID != "profile-b" {
		t.Fatalf("resolveActiveAIProfile selected = %+v, want profile-b", profile)
	}
	if profile := resolveActiveAIProfile(ai.SettingsResponse{}); profile != nil {
		t.Fatalf("resolveActiveAIProfile empty = %+v, want nil", profile)
	}
	var nilSettings *nilableSettings
	if provider := normalizeSettingsProvider(nilSettings); provider != nil {
		t.Fatalf("normalizeSettingsProvider typed nil = %+v, want nil", provider)
	}
	if provider := normalizeSettingsProvider(fakeSettings{}); provider == nil {
		t.Fatal("normalizeSettingsProvider should keep non-nil providers")
	}
	if got := NormalizeGenerationStrategy(""); got != GenerationStrategySerial {
		t.Fatalf("NormalizeGenerationStrategy blank = %q, want serial", got)
	}
	if got := NormalizeGenerationStrategy(GenerationStrategyParallelIdentity); got != GenerationStrategyParallelIdentity {
		t.Fatalf("NormalizeGenerationStrategy parallel = %q", got)
	}
	if got := NormalizeGenerationStrategy(GenerationStrategyDiscoveryParallelCorrection); got != GenerationStrategyDiscoveryParallelCorrection {
		t.Fatalf("NormalizeGenerationStrategy discovery = %q", got)
	}
}

func TestFilesystemSummaryRepositoryLoadsSavedSummary(t *testing.T) {
	stateDir := t.TempDir()
	if err := characters.SaveHeuristicSummary(stateDir, "novel-a", "1", []characters.HeuristicEpisode{{EpisodeIndex: "1", Text: "Alice appears."}}); err != nil {
		t.Fatalf("SaveHeuristicSummary returned error: %v", err)
	}
	summary, ok, err := filesystemSummaryRepository{}.LoadForEpisodes(stateDir, "novel-a", "1", []string{"1"})
	if err != nil || !ok {
		t.Fatalf("LoadForEpisodes ok=%v err=%v", ok, err)
	}
	if summary.NovelID != "novel-a" || summary.Status != "ready" {
		t.Fatalf("summary = %+v", summary)
	}
}

func TestResultReturnsCachedReadySummary(t *testing.T) {
	upToEpisodeIndex := "2"
	repository := &fakeRepository{
		loads: []fakeRepositoryLoad{{summary: readySummary("novel-a", upToEpisodeIndex, "Alice"), ok: true}},
	}
	generator := &fakeGenerator{}
	service := NewService(
		"/tmp/state",
		fakeLibrary{toc: &library.TocResponse{
			NovelSummary: library.NovelSummary{NovelID: "novel-a", Title: "作品A"},
			Episodes: []library.TocEpisodeSummary{
				{EpisodeIndex: "1"},
				{EpisodeIndex: "2"},
			},
		}},
		fakeSettings{response: ai.SettingsResponse{EffectiveGenerationMode: "heuristic"}},
		generator,
	).WithSummaryRepository(repository)

	result, err := service.Result(context.Background(), "novel-a", upToEpisodeIndex, RequestOptions{})
	if err != nil {
		t.Fatalf("Result returned error: %v", err)
	}
	if result.NovelTitle != "作品A" {
		t.Fatalf("NovelTitle = %q, want %q", result.NovelTitle, "作品A")
	}
	if result.GenerationMode != "heuristic" {
		t.Fatalf("GenerationMode = %q, want heuristic", result.GenerationMode)
	}
	if len(result.Characters) != 1 || result.Characters[0].CanonicalName != "Alice" {
		t.Fatalf("Characters = %#v, want Alice", result.Characters)
	}
	if generator.generateSaveCount != 0 || generator.previewCount != 0 {
		t.Fatalf("generator was called: save=%d preview=%d", generator.generateSaveCount, generator.previewCount)
	}
	if len(repository.loadCalls) != 1 {
		t.Fatalf("loadCalls length = %d, want 1", len(repository.loadCalls))
	}
	assertRepositoryCall(t, repository.loadCalls[0], "/tmp/state", "novel-a", upToEpisodeIndex, []string{"1", "2"})
}

func TestResultGeneratesAndReloadsMissingSummary(t *testing.T) {
	upToEpisodeIndex := "3"
	repository := &fakeRepository{
		loads: []fakeRepositoryLoad{
			{ok: false},
			{summary: readySummary("novel-a", upToEpisodeIndex, "Bob"), ok: true},
		},
	}
	generator := &fakeGenerator{}
	service := NewService(
		"/tmp/state",
		nil,
		fakeSettings{response: ai.SettingsResponse{EffectiveGenerationMode: "heuristic"}},
		generator,
	).WithSummaryRepository(repository)

	result, err := service.Result(context.Background(), "novel-a", upToEpisodeIndex, RequestOptions{})
	if err != nil {
		t.Fatalf("Result returned error: %v", err)
	}
	if generator.generateSaveCount != 1 {
		t.Fatalf("generateSaveCount = %d, want 1", generator.generateSaveCount)
	}
	if len(result.Characters) != 1 || result.Characters[0].CanonicalName != "Bob" {
		t.Fatalf("Characters = %#v, want Bob", result.Characters)
	}
	if len(repository.loadCalls) != 2 {
		t.Fatalf("loadCalls length = %d, want 2", len(repository.loadCalls))
	}
	assertRepositoryCall(t, repository.loadCalls[0], "/tmp/state", "novel-a", upToEpisodeIndex, nil)
	assertRepositoryCall(t, repository.loadCalls[1], "/tmp/state", "novel-a", upToEpisodeIndex, nil)
	if generator.generateSaveCall.novelID != "novel-a" || generator.generateSaveCall.upToEpisodeIndex != upToEpisodeIndex {
		t.Fatalf("generateSaveCall = %#v", generator.generateSaveCall)
	}
	if generator.generateSaveCall.resolvedConfig != nil {
		t.Fatalf("generateSaveCall.resolvedConfig = %#v, want nil", generator.generateSaveCall.resolvedConfig)
	}
}

func TestPreviewUsesResolvedProfileAndGenerator(t *testing.T) {
	upToEpisodeIndex := "4"
	modelID := "openrouter/model"
	repository := &fakeRepository{loads: []fakeRepositoryLoad{{ok: false}}}
	generator := &fakeGenerator{preview: readySummary("novel-a", upToEpisodeIndex, "Carol")}
	preloadedInputs := &Inputs{}
	progressSink := func(BatchProgress) {}
	resolvedConfig := &store.ResolvedAIGenerationConfig{
		ProfileID:    "profile-a",
		ProfileLabel: "Profile A",
		ModelID:      modelID,
	}
	service := NewService(
		"/tmp/state",
		fakeLibrary{toc: &library.TocResponse{
			NovelSummary: library.NovelSummary{NovelID: "novel-a", Title: "作品A"},
			Episodes: []library.TocEpisodeSummary{
				{EpisodeIndex: "1"},
				{EpisodeIndex: "4"},
			},
		}},
		fakeSettings{response: ai.SettingsResponse{EffectiveGenerationMode: "heuristic"}},
		generator,
	).WithSummaryRepository(repository)

	result, err := service.Result(context.Background(), "novel-a", upToEpisodeIndex, RequestOptions{
		PreviewOnly:       true,
		ProfileResolution: true,
		ResolvedConfig:    resolvedConfig,
		BatchProgressSink: progressSink,
		SummaryInputs:     preloadedInputs,
	})
	if err != nil {
		t.Fatalf("Result returned error: %v", err)
	}
	if len(generator.lockCalls) != 1 || generator.previewCount != 1 {
		t.Fatalf("generator calls = lock:%d preview:%d, want 1/1", len(generator.lockCalls), generator.previewCount)
	}
	if result.GenerationMode != "openrouter" {
		t.Fatalf("GenerationMode = %q, want openrouter", result.GenerationMode)
	}
	if result.ProfileID == nil || *result.ProfileID != "profile-a" {
		t.Fatalf("ProfileID = %#v, want profile-a", result.ProfileID)
	}
	if result.ModelID == nil || *result.ModelID != modelID {
		t.Fatalf("ModelID = %#v, want %s", result.ModelID, modelID)
	}
	assertRepositoryCall(t, repository.loadCalls[0], "/tmp/state", "novel-a", upToEpisodeIndex, []string{"1", "4"})
	if generator.lockCalls[0].novelID != "novel-a" || generator.lockCalls[0].upToEpisodeIndex != upToEpisodeIndex {
		t.Fatalf("lockCalls[0] = %#v", generator.lockCalls[0])
	}
	if generator.previewCall.novelID != "novel-a" || generator.previewCall.upToEpisodeIndex != upToEpisodeIndex {
		t.Fatalf("previewCall = %#v", generator.previewCall)
	}
	if generator.previewCall.resolvedConfig != resolvedConfig {
		t.Fatalf("previewCall.resolvedConfig = %#v, want %#v", generator.previewCall.resolvedConfig, resolvedConfig)
	}
	if !generator.previewCall.hasProgressSink {
		t.Fatal("previewCall.hasProgressSink = false, want true")
	}
	assertStrings(t, generator.previewCall.episodeIndexes, []string{"1", "4"})
	if generator.previewCall.inputs != preloadedInputs {
		t.Fatalf("previewCall.inputs = %#v, want %#v", generator.previewCall.inputs, preloadedInputs)
	}
}

func TestResultReturnsSettingsError(t *testing.T) {
	service := NewService(
		"/tmp/state",
		nil,
		fakeSettings{err: errors.New("settings failed")},
		&fakeGenerator{},
	).WithSummaryRepository(&fakeRepository{})

	_, err := service.Result(context.Background(), "novel-a", "1", RequestOptions{})
	if err == nil {
		t.Fatal("Result returned nil error")
	}
}

func TestTypedNilSettingsProviderReturnsCachedSummary(t *testing.T) {
	upToEpisodeIndex := "5"
	var settings *store.Store
	repository := &fakeRepository{
		loads: []fakeRepositoryLoad{{summary: readySummary("novel-a", upToEpisodeIndex, "Dana"), ok: true}},
	}
	service := NewService(
		"/tmp/state",
		nil,
		settings,
		&fakeGenerator{},
	).WithSummaryRepository(repository)

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("Result panicked with typed nil settings provider: %v", recovered)
		}
	}()
	result, err := service.Result(context.Background(), "novel-a", upToEpisodeIndex, RequestOptions{})
	if err != nil {
		t.Fatalf("Result returned error: %v", err)
	}
	if result.GenerationMode != "heuristic" {
		t.Fatalf("GenerationMode = %q, want heuristic", result.GenerationMode)
	}
	if len(result.Characters) != 1 || result.Characters[0].CanonicalName != "Dana" {
		t.Fatalf("Characters = %#v, want Dana", result.Characters)
	}
}

func assertRepositoryCall(t *testing.T, call fakeRepositoryCall, stateDir string, novelID string, upToEpisodeIndex string, episodeIndexes []string) {
	t.Helper()
	if call.stateDir != stateDir || call.novelID != novelID || call.upToEpisodeIndex != upToEpisodeIndex {
		t.Fatalf("repository call = %#v, want stateDir=%q novelID=%q upToEpisodeIndex=%q", call, stateDir, novelID, upToEpisodeIndex)
	}
	assertStrings(t, call.episodeIndexes, episodeIndexes)
}

func assertStrings(t *testing.T, got []string, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("strings = %#v, want %#v", got, want)
	}
	for index := range got {
		if got[index] != want[index] {
			t.Fatalf("strings = %#v, want %#v", got, want)
		}
	}
}
