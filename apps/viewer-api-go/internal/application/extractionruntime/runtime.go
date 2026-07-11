package extractionruntime

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"narou-viewer/apps/viewer-api-go/internal/ai"
	appextraction "narou-viewer/apps/viewer-api-go/internal/application/extraction"
	"narou-viewer/apps/viewer-api-go/internal/characters"
	core "narou-viewer/apps/viewer-api-go/internal/extraction"
	extractdomain "narou-viewer/apps/viewer-api-go/internal/extraction"
	"narou-viewer/apps/viewer-api-go/internal/extraction/checkpointstore"
	"narou-viewer/apps/viewer-api-go/internal/fsatomic"
	"narou-viewer/apps/viewer-api-go/internal/library"
	"narou-viewer/apps/viewer-api-go/internal/store"
	"narou-viewer/apps/viewer-api-go/internal/terms"
)

type Library interface {
	GetToc(ctx context.Context, novelID string) (*library.TocResponse, error)
	GetEpisode(ctx context.Context, novelID string, episodeIndex string) (*library.EpisodeResponse, error)
	FindWork(novelID string) (library.Work, bool, error)
}

type SettingsProvider interface {
	GetAIGenerationSettings() (ai.SettingsResponse, error)
	ResolveActiveAIGenerationConfig() (*store.ResolvedAIGenerationConfig, error)
}

type RuntimeDependencies struct {
	StateDir    string
	UsageDBPath string
	Library     Library
	Settings    SettingsProvider
	Logger      Logger
}

type Runtime struct {
	stateDir    string
	usageDBPath string
	library     Library
	settings    SettingsProvider
	logger      Logger

	generationMu      sync.Mutex
	generationTargets map[string]*generationTargetLock
}

type generationTargetLock struct {
	mu   sync.Mutex
	refs int
}

func NewRuntime(deps RuntimeDependencies) *Runtime {
	return &Runtime{
		stateDir:    deps.StateDir,
		usageDBPath: deps.UsageDBPath,
		library:     normalizeLibrary(deps.Library),
		settings:    normalizeSettingsProvider(deps.Settings),
		logger:      deps.Logger,
	}
}

func normalizeLibrary(library Library) Library {
	if isNilInterface(library) {
		return nil
	}
	return library
}

func normalizeSettingsProvider(settings SettingsProvider) SettingsProvider {
	if isNilInterface(settings) {
		return nil
	}
	return settings
}

func isNilInterface(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

func (r *Runtime) Workflow() *appextraction.Workflow {
	return appextraction.NewWorkflow(r)
}

func (r *Runtime) Result(ctx context.Context, novelID string, upToEpisodeIndex string, options appextraction.RequestOptions) (appextraction.Result, error) {
	if r == nil {
		return appextraction.Result{}, nil
	}
	return appextraction.NewService(r.stateDir, r.library, r.settings, r).Result(ctx, novelID, upToEpisodeIndex, options)
}

func (r *Runtime) PreparePreview(ctx context.Context, novelID string, upToEpisodeIndex string, resolvedConfig *store.ResolvedAIGenerationConfig) (appextraction.PreparedPreview, error) {
	if r == nil {
		return appextraction.PreparedPreview{}, nil
	}
	return r.Workflow().PreparePreview(ctx, novelID, upToEpisodeIndex, resolvedConfig)
}

func (r *Runtime) ProcessJob(ctx context.Context, novelID string, job extractdomain.Job) bool {
	if r == nil {
		return false
	}
	processor := NewProcessor(Dependencies{
		StateDir: r.stateDir,
		Workflow: r.Workflow(),
		Settings: r.settings,
		Logger:   r.logger,
	})
	return processor.Process(ctx, novelID, job)
}

func (r *Runtime) LockTarget(novelID string, upToEpisodeIndex string) func() {
	if r == nil {
		return func() {}
	}
	key := novelID + "\x00" + upToEpisodeIndex
	r.generationMu.Lock()
	if r.generationTargets == nil {
		r.generationTargets = map[string]*generationTargetLock{}
	}
	target := r.generationTargets[key]
	if target == nil {
		target = &generationTargetLock{}
		r.generationTargets[key] = target
	}
	target.refs++
	r.generationMu.Unlock()

	target.mu.Lock()
	return func() {
		target.mu.Unlock()
		r.generationMu.Lock()
		target.refs--
		if target.refs == 0 && r.generationTargets[key] == target {
			delete(r.generationTargets, key)
		}
		r.generationMu.Unlock()
	}
}

func (r *Runtime) ActiveLockCount() int {
	if r == nil {
		return 0
	}
	r.generationMu.Lock()
	defer r.generationMu.Unlock()
	return len(r.generationTargets)
}

func (r *Runtime) GenerateAndSave(ctx context.Context, novelID string, upToEpisodeIndex string, resolvedOverride *store.ResolvedAIGenerationConfig, strategy string, progressSink func(appextraction.BatchProgress)) (appextraction.FinalCounts, error) {
	if r == nil {
		return appextraction.FinalCounts{}, nil
	}
	return r.Workflow().GenerateAndSave(ctx, novelID, upToEpisodeIndex, resolvedOverride, strategy, progressSink)
}

func (r *Runtime) GeneratePreview(ctx context.Context, novelID string, upToEpisodeIndex string, resolvedOverride *store.ResolvedAIGenerationConfig, strategy string, progressSink func(appextraction.BatchProgress), episodeIndexes []string, preloaded *appextraction.Inputs) (appextraction.Result, error) {
	if r == nil {
		return appextraction.Result{}, nil
	}
	return r.Workflow().GeneratePreview(ctx, novelID, upToEpisodeIndex, resolvedOverride, strategy, progressSink, episodeIndexes, preloaded)
}

func (r *Runtime) GetAIGenerationSettings() (ai.SettingsResponse, error) {
	if r == nil || r.settings == nil {
		return ai.SettingsResponse{}, nil
	}
	return r.settings.GetAIGenerationSettings()
}

func (r *Runtime) ResolveActiveAIGenerationConfig() (*store.ResolvedAIGenerationConfig, error) {
	if r == nil || r.settings == nil {
		return nil, nil
	}
	return r.settings.ResolveActiveAIGenerationConfig()
}

func (r *Runtime) NovelTitle(ctx context.Context, novelID string) *string {
	if r == nil || r.library == nil {
		return nil
	}
	work, ok, err := r.library.FindWork(novelID)
	if err != nil || !ok || strings.TrimSpace(work.Title) == "" {
		return nil
	}
	return &work.Title
}

func (r *Runtime) RecordUsage(run ai.UsageRun) error {
	if r == nil || strings.TrimSpace(r.usageDBPath) == "" {
		return nil
	}
	return ai.SaveUsageRun(r.usageDBPath, run)
}

func (r *Runtime) Limits() (int, int) {
	return extractionLimits()
}

func (r *Runtime) LoadInputs(ctx context.Context, novelID string, upToEpisodeIndex string, maxChunkChars int, maxBatchChars int, afterEpisodeIndex string) (appextraction.Inputs, error) {
	startedAt := time.Now()
	defer r.log("load_inputs", startedAt, "novelId", novelID, "upToEpisodeIndex", upToEpisodeIndex, "afterEpisodeIndex", afterEpisodeIndex)
	if r == nil || r.library == nil {
		return appextraction.Inputs{}, nil
	}
	toc, err := r.library.GetToc(ctx, novelID)
	if err != nil {
		return appextraction.Inputs{}, err
	}
	if toc == nil {
		return appextraction.Inputs{}, errors.New("extraction target novel was not found")
	}
	afterEpisodeIndex = strings.TrimSpace(afterEpisodeIndex)
	episodes := []characters.HeuristicEpisode{}
	chunks := []extractionChunk{}
	for _, episodeSummary := range toc.Episodes {
		if compareEpisodeString(episodeSummary.EpisodeIndex, upToEpisodeIndex) > 0 {
			break
		}
		if afterEpisodeIndex != "" && compareEpisodeString(episodeSummary.EpisodeIndex, afterEpisodeIndex) <= 0 {
			continue
		}
		episode, err := r.library.GetEpisode(ctx, novelID, episodeSummary.EpisodeIndex)
		if err != nil {
			return appextraction.Inputs{}, err
		}
		if episode == nil {
			return appextraction.Inputs{}, errors.New("extraction could not load episode bodies")
		}
		input := core.EpisodeInput{
			EpisodeIndex:   episode.EpisodeIndex,
			Title:          episode.Title,
			Chapter:        episode.Chapter,
			Subchapter:     episode.Subchapter,
			HTML:           episode.HTML,
			ReaderDocument: episode.ReaderDocument,
		}
		text := extractExtractionEpisodeText(input)
		episodes = append(episodes, characters.HeuristicEpisode{
			EpisodeIndex: episode.EpisodeIndex,
			Text:         text,
			ContentEtag:  episode.ContentEtag,
		})
		chunks = append(chunks, createExtractionChunksFromText(input, text, maxChunkChars)...)
	}
	return appextraction.Inputs{
		Episodes: episodes,
		Batches:  createExtractionBatches(chunks, maxBatchChars),
	}, nil
}

func (r *Runtime) LoadGenerationSeed(novelID string, upToEpisodeIndex string) ([]characters.GeneratedCharacter, *string, bool, error) {
	existing, processed, ok, err := characters.LoadGeneratedCharacters(r.stateDir, novelID)
	if err != nil || !ok || processed == nil {
		return nil, processed, ok, err
	}
	return existing, processed, true, nil
}

func (r *Runtime) LoadGeneratedCharactersBeforeEpisode(novelID string, episodeIndex string) ([]characters.GeneratedCharacter, *string, bool, error) {
	return characters.LoadGeneratedCharactersBeforeEpisode(r.stateDir, novelID, episodeIndex)
}

func (r *Runtime) LoadGeneratedTermsAtOrBefore(novelID string, committedFrontier string) ([]terms.GeneratedTerm, *string, bool, error) {
	return terms.LoadGeneratedTermsAtOrBefore(r.stateDir, novelID, committedFrontier)
}

func (r *Runtime) LoadGeneratedTermsBeforeEpisode(novelID string, episodeIndex string) ([]terms.GeneratedTerm, *string, bool, error) {
	return terms.LoadGeneratedTermsBeforeEpisode(r.stateDir, novelID, episodeIndex)
}

func (r *Runtime) ReprocessFromEpisode(ctx context.Context, novelID string, processedEpisodeIndex *string, requestedUpToEpisodeIndex string) (string, error) {
	if r == nil || r.library == nil || processedEpisodeIndex == nil || strings.TrimSpace(*processedEpisodeIndex) == "" {
		return "", nil
	}
	scanUpToEpisodeIndex := strings.TrimSpace(*processedEpisodeIndex)
	requestedUpToEpisodeIndex = strings.TrimSpace(requestedUpToEpisodeIndex)
	if requestedUpToEpisodeIndex != "" && compareEpisodeString(requestedUpToEpisodeIndex, scanUpToEpisodeIndex) < 0 {
		scanUpToEpisodeIndex = requestedUpToEpisodeIndex
	}
	digests, err := characters.LoadGeneratedEpisodeDigests(r.stateDir, novelID)
	if err != nil {
		return "", err
	}
	etagByEpisode := map[string]string{}
	for _, digest := range digests {
		if strings.TrimSpace(digest.EpisodeIndex) != "" && strings.TrimSpace(digest.ContentEtag) != "" {
			etagByEpisode[digest.EpisodeIndex] = digest.ContentEtag
		}
	}
	toc, err := r.library.GetToc(ctx, novelID)
	if err != nil || toc == nil {
		return "", err
	}
	currentEpisodeIndexes := []string{}
	currentEtags := map[string]string{}
	for _, episode := range toc.Episodes {
		if compareEpisodeString(episode.EpisodeIndex, scanUpToEpisodeIndex) > 0 {
			break
		}
		currentEpisodeIndexes = append(currentEpisodeIndexes, episode.EpisodeIndex)
		currentEtags[episode.EpisodeIndex] = strings.TrimSpace(episode.ContentEtag)
	}
	if len(currentEpisodeIndexes) == 0 {
		return earliestGeneratedEpisodeDigest(digests, scanUpToEpisodeIndex), nil
	}
	if len(etagByEpisode) == 0 {
		return currentEpisodeIndexes[0], nil
	}
	currentSeen := map[string]bool{}
	for _, episodeIndex := range currentEpisodeIndexes {
		currentSeen[episodeIndex] = true
		savedEtag := strings.TrimSpace(etagByEpisode[episodeIndex])
		currentEtag := currentEtags[episodeIndex]
		if savedEtag == "" || currentEtag == "" || savedEtag != currentEtag {
			return episodeIndex, nil
		}
	}
	for _, digest := range digests {
		episodeIndex := strings.TrimSpace(digest.EpisodeIndex)
		if episodeIndex != "" && compareEpisodeString(episodeIndex, scanUpToEpisodeIndex) <= 0 && !currentSeen[episodeIndex] {
			return episodeIndex, nil
		}
	}
	return "", nil
}

func (r *Runtime) MaterializeGeneratedSummary(novelID string) error {
	startedAt := time.Now()
	_, err := characters.MaterializeGeneratedSummary(r.stateDir, novelID)
	status := "ok"
	if err != nil {
		status = "error"
	}
	r.log("materialize_generated_summary", startedAt, "status", status, "novelId", novelID)
	return err
}

func (r *Runtime) LoadPendingUnresolved(novelID string, reprocessFromEpisodeIndex string) ([]characters.GeneratedUnresolvedMention, error) {
	pending, err := characters.LoadGeneratedUnresolvedMentions(r.stateDir, novelID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(reprocessFromEpisodeIndex) == "" {
		return pending, nil
	}
	return filterGeneratedUnresolvedMentionsBeforeEpisode(pending, reprocessFromEpisodeIndex), nil
}

func (r *Runtime) RebatchInputs(ctx context.Context, inputs appextraction.Inputs, config *store.ResolvedAIGenerationConfig, fallbackMaxBatchChars int) appextraction.Inputs {
	startedAt := time.Now()
	result := rebatchExtractionInputs(ctx, inputs, config, fallbackMaxBatchChars)
	r.log("rebatch_inputs", startedAt, "episodeCount", len(result.Episodes), "batchCount", len(result.Batches), "chunkCount", countExtractionChunks(result.Batches))
	return result
}

func (r *Runtime) LoadIDAllocator(novelID string, seed []characters.GeneratedCharacter) (*characters.GeneratedCharacterIDAllocator, error) {
	return characters.LoadGeneratedCharacterIDAllocator(r.stateDir, novelID, seed)
}

func (r *Runtime) PlanRuntimeBatch(ctx context.Context, config *store.ResolvedAIGenerationConfig, novelID string, upToEpisodeIndex string, knownCharacters []characters.GeneratedCharacter, knownTerms []terms.GeneratedTerm, template extractionBatch, chunks []extractionChunk, unresolvedMentions []characters.GeneratedUnresolvedMention) (extractionBatch, []extractionChunk, error) {
	startedAt := time.Now()
	runtimeBatch, remaining, err := r.nextRuntimeBatch(ctx, config, novelID, upToEpisodeIndex, knownCharacters, knownTerms, template, chunks, unresolvedMentions)
	status := "ok"
	if err != nil {
		status = "error"
	}
	r.log("plan_runtime_batch", startedAt, "status", status, "novelId", novelID, "upToEpisodeIndex", upToEpisodeIndex, "templateBatch", template.BatchIndex, "runtimeBatch", runtimeBatch.BatchIndex, "inputChunks", len(chunks), "runtimeChunks", len(runtimeBatch.Chunks), "remainingChunks", len(remaining), "knownCharacters", len(knownCharacters), "unresolvedMentions", len(unresolvedMentions))
	return runtimeBatch, remaining, err
}

func (r *Runtime) GenerateBatch(ctx context.Context, config *store.ResolvedAIGenerationConfig, novelID string, upToEpisodeIndex string, knownCharacters []characters.GeneratedCharacter, knownTerms []terms.GeneratedTerm, batch extractionBatch, unresolvedMentions []characters.GeneratedUnresolvedMention) (appextraction.BatchResult, error) {
	startedAt := time.Now()
	result, err := r.generateOpenRouterBatch(ctx, config, novelID, upToEpisodeIndex, knownCharacters, knownTerms, batch, unresolvedMentions)
	status := "ok"
	if err != nil {
		status = "error"
	}
	r.log("generate_batch", startedAt, "status", status, "novelId", novelID, "upToEpisodeIndex", upToEpisodeIndex, "batch", batch.BatchIndex, "batchCount", batch.BatchCount, "chunks", len(batch.Chunks), "knownCharacters", len(knownCharacters), "unresolvedMentions", len(unresolvedMentions), "inputTokens", result.Usage.InputTokens, "outputTokens", result.Usage.OutputTokens, "totalTokens", result.Usage.TotalTokens)
	return appextraction.BatchResult{Delta: result.Delta, Usage: result.Usage}, err
}

func (r *Runtime) SaveGeneratedTerms(novelID string, upToEpisodeIndex string, generated []terms.GeneratedTerm, replaceFromEpisodeIndex string) error {
	var replaceFrom *string
	if strings.TrimSpace(replaceFromEpisodeIndex) != "" {
		replaceFrom = &replaceFromEpisodeIndex
	}
	return terms.SaveGeneratedTerms(r.stateDir, novelID, upToEpisodeIndex, generated, replaceFrom)
}

func (r *Runtime) LoadCheckpoint(novelID string, upToEpisodeIndex string) (checkpointstore.Checkpoint, error) {
	return checkpointstore.NewFileStore(r.stateDir).Load(novelID, upToEpisodeIndex)
}

func (r *Runtime) SaveCheckpoint(novelID string, upToEpisodeIndex string, checkpoint checkpointstore.Checkpoint) error {
	startedAt := time.Now()
	err := checkpointstore.NewFileStore(r.stateDir).Save(novelID, upToEpisodeIndex, checkpoint)
	status := "ok"
	if err != nil {
		status = "error"
	}
	r.log("save_checkpoint", startedAt, "status", status, "novelId", novelID, "upToEpisodeIndex", upToEpisodeIndex, "characterCount", len(checkpoint.Characters), "processedBatchCount", len(checkpoint.ProcessedBatchIndexes))
	return err
}

func (r *Runtime) DeleteCheckpoint(novelID string, upToEpisodeIndex string) error {
	return checkpointstore.NewFileStore(r.stateDir).Delete(novelID, upToEpisodeIndex)
}

func (r *Runtime) SaveGeneratedSummary(novelID string, upToEpisodeIndex string, generated []characters.GeneratedCharacter, episodes []characters.HeuristicEpisode, options characters.SaveGeneratedSummaryOptions) error {
	startedAt := time.Now()
	err := characters.SaveGeneratedSummaryWithOptions(r.stateDir, novelID, upToEpisodeIndex, generated, episodes, options)
	status := "ok"
	if err != nil {
		status = "error"
	}
	r.log("save_generated_summary", startedAt, "status", status, "novelId", novelID, "upToEpisodeIndex", upToEpisodeIndex, "characterCount", len(generated), "episodeCount", len(episodes))
	return err
}

func (r *Runtime) BuildGeneratedPreview(novelID string, upToEpisodeIndex string, generated []characters.GeneratedCharacter, episodes []characters.HeuristicEpisode, episodeIndexes []string, options characters.SaveGeneratedSummaryOptions) (characters.SummaryResponse, error) {
	startedAt := time.Now()
	result, err := buildGeneratedExtractionPreview(r.stateDir, novelID, upToEpisodeIndex, generated, episodes, episodeIndexes, options)
	status := "ok"
	if err != nil {
		status = "error"
	}
	r.log("build_generated_preview", startedAt, "status", status, "novelId", novelID, "upToEpisodeIndex", upToEpisodeIndex, "characterCount", len(generated), "episodeCount", len(episodes))
	return result, err
}

func (r *Runtime) SaveHeuristicSummary(novelID string, upToEpisodeIndex string, episodes []characters.HeuristicEpisode) error {
	return characters.SaveHeuristicSummary(r.stateDir, novelID, upToEpisodeIndex, episodes)
}

func (r *Runtime) BuildHeuristicPreview(novelID string, upToEpisodeIndex string, episodes []characters.HeuristicEpisode, episodeIndexes []string) (characters.SummaryResponse, error) {
	return buildHeuristicExtractionPreview(novelID, upToEpisodeIndex, episodes, episodeIndexes)
}

func (r *Runtime) LoadRequiredPreview(novelID string, upToEpisodeIndex string, episodeIndexes []string) (characters.SummaryResponse, error) {
	return loadRequiredExtractionPreview(r.stateDir, novelID, upToEpisodeIndex, episodeIndexes)
}

func (r *Runtime) CheckpointExists(novelID string, upToEpisodeIndex string) bool {
	return checkpointstore.NewFileStore(r.stateDir).Exists(novelID, upToEpisodeIndex)
}

func (r *Runtime) CheckpointPath(novelID string, upToEpisodeIndex string) string {
	return checkpointstore.NewFileStore(r.stateDir).Path(novelID, upToEpisodeIndex)
}

func (r *Runtime) log(stage string, startedAt time.Time, fields ...any) {
	if r != nil && r.logger != nil {
		r.logger(stage, startedAt, fields...)
		return
	}
	if !extractionTimingLogEnabled() {
		return
	}
	values := []any{"stage", stage, "elapsedMs", time.Since(startedAt).Milliseconds()}
	values = append(values, fields...)
	log.Printf("viewer-api-go: extraction timing %s", formatTimingFields(values...))
}

func extractionTimingLogEnabled() bool {
	return strings.TrimSpace(os.Getenv("VIEWER_EXTRACTION_TIMING_LOG")) == "1"
}

func FormatTimingFields(fields ...any) string {
	return formatTimingFields(fields...)
}

func formatTimingFields(fields ...any) string {
	parts := make([]string, 0, len(fields)/2)
	for index := 0; index+1 < len(fields); index += 2 {
		parts = append(parts, fmt.Sprintf("%v=%v", fields[index], fields[index+1]))
	}
	return strings.Join(parts, " ")
}

func countExtractionChunks(batches []extractionBatch) int {
	count := 0
	for _, batch := range batches {
		count += len(batch.Chunks)
	}
	return count
}

func rebatchExtractionInputs(ctx context.Context, inputs appextraction.Inputs, config *store.ResolvedAIGenerationConfig, fallbackMaxBatchChars int) appextraction.Inputs {
	if config == nil || len(inputs.Batches) == 0 {
		return inputs
	}
	chunks := make([]extractionChunk, 0)
	for _, batch := range inputs.Batches {
		chunks = append(chunks, batch.Chunks...)
	}
	inputs.Batches = createExtractionBatchesWithBudget(chunks, resolveExtractionBatchBudget(ctx, config, fallbackMaxBatchChars))
	return inputs
}

func buildGeneratedExtractionPreview(stateDir string, novelID string, upToEpisodeIndex string, generated []characters.GeneratedCharacter, episodes []characters.HeuristicEpisode, episodeIndexes []string, options characters.SaveGeneratedSummaryOptions) (characters.SummaryResponse, error) {
	return buildExtractionPreview(novelID, upToEpisodeIndex, episodeIndexes, func(tempDir string) error {
		if err := copyExtractionPreviewEvents(stateDir, tempDir, novelID); err != nil {
			return err
		}
		return characters.SaveGeneratedSummaryWithOptions(tempDir, novelID, upToEpisodeIndex, generated, episodes, options)
	})
}

func buildHeuristicExtractionPreview(novelID string, upToEpisodeIndex string, episodes []characters.HeuristicEpisode, episodeIndexes []string) (characters.SummaryResponse, error) {
	return buildExtractionPreview(novelID, upToEpisodeIndex, episodeIndexes, func(tempDir string) error {
		return characters.SaveHeuristicSummary(tempDir, novelID, upToEpisodeIndex, episodes)
	})
}

func buildExtractionPreview(novelID string, upToEpisodeIndex string, episodeIndexes []string, writeSummary func(string) error) (characters.SummaryResponse, error) {
	tempDir, err := os.MkdirTemp("", "narou-viewer-extraction-preview-*")
	if err != nil {
		return characters.SummaryResponse{}, err
	}
	defer os.RemoveAll(tempDir)
	if err := writeSummary(tempDir); err != nil {
		return characters.SummaryResponse{}, err
	}
	return loadRequiredExtractionPreview(tempDir, novelID, upToEpisodeIndex, episodeIndexes)
}

func copyExtractionPreviewEvents(sourceStateDir string, targetStateDir string, novelID string) error {
	if strings.TrimSpace(sourceStateDir) == "" || strings.TrimSpace(targetStateDir) == "" || strings.TrimSpace(novelID) == "" {
		return nil
	}
	sourcePath := filepath.Join(sourceStateDir, "character_events", novelID+".yaml")
	raw, err := os.ReadFile(sourcePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	targetPath := filepath.Join(targetStateDir, "character_events", novelID+".yaml")
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}
	return fsatomic.WriteFile(targetPath, raw, 0o600)
}

func loadRequiredExtractionPreview(stateDir string, novelID string, upToEpisodeIndex string, episodeIndexes []string) (characters.SummaryResponse, error) {
	summary, ok, err := characters.LoadSummaryForEpisodes(stateDir, novelID, upToEpisodeIndex, episodeIndexes)
	if err != nil {
		return characters.SummaryResponse{}, err
	}
	if !ok {
		return characters.SummaryResponse{}, errors.New("extraction preview could not be built")
	}
	return summary, nil
}

func filterGeneratedUnresolvedMentionsBeforeEpisode(values []characters.GeneratedUnresolvedMention, fromEpisodeIndex string) []characters.GeneratedUnresolvedMention {
	fromEpisodeIndex = strings.TrimSpace(fromEpisodeIndex)
	if fromEpisodeIndex == "" {
		return append([]characters.GeneratedUnresolvedMention{}, values...)
	}
	result := make([]characters.GeneratedUnresolvedMention, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value.EpisodeIndex) != "" && compareEpisodeString(value.EpisodeIndex, fromEpisodeIndex) < 0 {
			result = append(result, value)
		}
	}
	return result
}

func earliestGeneratedEpisodeDigest(digests []characters.GeneratedEpisodeDigest, processedEpisodeIndex string) string {
	earliest := ""
	for _, digest := range digests {
		episodeIndex := strings.TrimSpace(digest.EpisodeIndex)
		if episodeIndex == "" || compareEpisodeString(episodeIndex, processedEpisodeIndex) > 0 {
			continue
		}
		if earliest == "" || compareEpisodeString(episodeIndex, earliest) < 0 {
			earliest = episodeIndex
		}
	}
	return earliest
}

func extractionStateFromAllocator(unresolved []characters.GeneratedUnresolvedMention, allocator *characters.GeneratedCharacterIDAllocator) extractionGenerationState {
	state := extractionGenerationState{
		UnresolvedMentions: append([]characters.GeneratedUnresolvedMention{}, unresolved...),
	}
	if allocator != nil {
		state.IssuedCharacterIDs = allocator.IssuedCharacterIDs()
		state.RetiredCharacterIDs = allocator.RetiredCharacterIDs()
		state.NextOrdinal = allocator.NextCharacterOrdinal()
	}
	return state
}

func compareEpisodeString(left string, right string) int {
	leftNumber, leftErr := strconv.Atoi(left)
	rightNumber, rightErr := strconv.Atoi(right)
	if leftErr == nil && rightErr == nil {
		if leftNumber < rightNumber {
			return -1
		}
		if leftNumber > rightNumber {
			return 1
		}
		return 0
	}
	return strings.Compare(left, right)
}
