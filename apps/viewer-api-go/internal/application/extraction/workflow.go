package extraction

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"narou-viewer/apps/viewer-api-go/internal/ai"
	"narou-viewer/apps/viewer-api-go/internal/characters"
	core "narou-viewer/apps/viewer-api-go/internal/extraction"
	"narou-viewer/apps/viewer-api-go/internal/store"
)

type Workflow struct {
	ports WorkflowPorts
}

const (
	GenerationStrategySerial                      = "serial"
	GenerationStrategyParallelIdentity            = "parallel_identity"
	GenerationStrategyDiscoveryParallelCorrection = "discovery_parallel_correction"
)

func NewWorkflow(ports WorkflowPorts) *Workflow {
	return &Workflow{ports: ports}
}

func NormalizeGenerationStrategy(value string) string {
	switch strings.TrimSpace(value) {
	case GenerationStrategyParallelIdentity:
		return GenerationStrategyParallelIdentity
	case GenerationStrategyDiscoveryParallelCorrection:
		return GenerationStrategyDiscoveryParallelCorrection
	default:
		return GenerationStrategySerial
	}
}

func (w *Workflow) PrepareInputs(ctx context.Context, novelID string, upToEpisodeIndex string, resolvedConfig *store.ResolvedAIGenerationConfig) (Inputs, error) {
	if w == nil || w.ports == nil {
		return Inputs{}, nil
	}
	return w.prepareInputs(ctx, novelID, upToEpisodeIndex, resolvedConfig)
}

func (w *Workflow) PreparePreview(ctx context.Context, novelID string, upToEpisodeIndex string, resolvedConfig *store.ResolvedAIGenerationConfig) (PreparedPreview, error) {
	if w == nil || w.ports == nil {
		return PreparedPreview{}, nil
	}
	inputs, err := w.prepareInputs(ctx, novelID, upToEpisodeIndex, resolvedConfig)
	if err != nil {
		return PreparedPreview{}, err
	}
	return PreparedPreview{
		Inputs:  inputs,
		Preview: buildPromptPreview(inputs.Batches, resolvedConfig),
	}, nil
}

func (w *Workflow) RunOpenRouterWithCheckpoint(ctx context.Context, config *store.ResolvedAIGenerationConfig, novelID string, upToEpisodeIndex string, seed []characters.GeneratedCharacter, batches []core.Batch, progressSink func(BatchProgress), pendingUnresolved []characters.GeneratedUnresolvedMention) ([]characters.GeneratedCharacter, core.GenerationState, []ai.UsageRequest, error) {
	if w == nil || w.ports == nil {
		return nil, core.GenerationState{}, nil, nil
	}
	return generationRunner{ports: w.ports}.GenerateWithCheckpoint(ctx, config, novelID, upToEpisodeIndex, seed, batches, progressSink, pendingUnresolved)
}

func (w *Workflow) RunOpenRouterPreview(ctx context.Context, config *store.ResolvedAIGenerationConfig, novelID string, upToEpisodeIndex string, seed []characters.GeneratedCharacter, batches []core.Batch, progressSink func(BatchProgress), pendingUnresolved []characters.GeneratedUnresolvedMention) ([]characters.GeneratedCharacter, core.GenerationState, []ai.UsageRequest, error) {
	if w == nil || w.ports == nil {
		return nil, core.GenerationState{}, nil, nil
	}
	return generationRunner{ports: w.ports}.GeneratePreview(ctx, config, novelID, upToEpisodeIndex, seed, batches, progressSink, pendingUnresolved)
}

func (w *Workflow) prepareInputs(ctx context.Context, novelID string, upToEpisodeIndex string, resolvedConfig *store.ResolvedAIGenerationConfig) (Inputs, error) {
	maxChunkChars, maxBatchChars := w.ports.Limits()
	inputs, err := w.ports.LoadInputs(ctx, novelID, upToEpisodeIndex, maxChunkChars, maxBatchChars, "")
	if err != nil {
		return Inputs{}, err
	}
	return w.ports.RebatchInputs(ctx, inputs, resolvedConfig, maxBatchChars), nil
}

func buildPromptPreview(chunkBatches []core.Batch, resolvedConfig *store.ResolvedAIGenerationConfig) PromptPreview {
	batches := make([]PromptPreviewBatch, 0, len(chunkBatches))
	for _, batch := range chunkBatches {
		chunks := make([]PromptPreviewChunk, 0, len(batch.Chunks))
		for _, chunk := range batch.Chunks {
			chunks = append(chunks, PromptPreviewChunk{
				EpisodeIndex: chunk.EpisodeIndex,
				Title:        chunk.Title,
				Chapter:      chunk.Chapter,
				Subchapter:   chunk.Subchapter,
				ChunkIndex:   chunk.ChunkIndex,
				ChunkCount:   chunk.ChunkCount,
				Text:         chunk.Text,
			})
		}
		batches = append(batches, PromptPreviewBatch{
			BatchIndex:     batch.BatchIndex,
			BatchCount:     batch.BatchCount,
			EpisodeIndexes: append([]string{}, batch.EpisodeIndexes...),
			ChunkCount:     len(batch.Chunks),
			Chunks:         chunks,
		})
	}
	var systemPromptOverride *string
	if resolvedConfig != nil {
		systemPromptOverride = resolvedConfig.SystemPrompt
	}
	return PromptPreview{
		SystemPrompt: core.ResolveSystemPrompt(systemPromptOverride),
		Batches:      batches,
	}
}

func (w *Workflow) GenerateAndSave(ctx context.Context, novelID string, upToEpisodeIndex string, resolvedOverride *store.ResolvedAIGenerationConfig, strategy string, progressSink func(BatchProgress)) (err error) {
	if w == nil || w.ports == nil {
		return nil
	}
	unlock := w.ports.LockTarget(novelID, upToEpisodeIndex)
	defer unlock()

	recorder := newUsageRecorder(ctx, w.ports, "character-summary", novelID, upToEpisodeIndex)
	defer func() {
		recorder.Finish(err)
	}()
	maxChunkChars, maxBatchChars := w.ports.Limits()
	settings, err := w.ports.GetAIGenerationSettings()
	if err != nil {
		return err
	}
	generationMode := resolveWorkflowGenerationMode(settings, resolvedOverride)
	generationStrategy := NormalizeGenerationStrategy(strategy)
	recorder.GenerationMode = generationMode
	recorder.GenerationStrategy = generationStrategy

	switch generationMode {
	case "openrouter":
		recorder.Enabled = true
		config, err := w.resolveConfig(resolvedOverride)
		if err != nil {
			return err
		}
		recorder.ActiveProfile = config
		return w.generateOpenRouterAndSave(ctx, recorder, novelID, upToEpisodeIndex, config, generationStrategy, maxChunkChars, maxBatchChars, progressSink)
	case "disabled":
		return errors.New("Character summary generation via LLM is unavailable because OpenRouter is not configured.")
	default:
		inputs, err := w.ports.LoadInputs(ctx, novelID, upToEpisodeIndex, maxChunkChars, maxBatchChars, "")
		if err != nil {
			return err
		}
		recorder.Episodes = inputs.Episodes
		return w.ports.SaveHeuristicSummary(novelID, upToEpisodeIndex, inputs.Episodes)
	}
}

func (w *Workflow) GeneratePreview(ctx context.Context, novelID string, upToEpisodeIndex string, resolvedOverride *store.ResolvedAIGenerationConfig, strategy string, progressSink func(BatchProgress), episodeIndexes []string, preloaded *Inputs) (summary characters.SummaryResponse, err error) {
	if w == nil || w.ports == nil {
		return characters.SummaryResponse{}, nil
	}
	recorder := newUsageRecorder(ctx, w.ports, "character-summary-playground", novelID, upToEpisodeIndex)
	defer func() {
		recorder.Finish(err)
	}()
	recorder.PreviewOnly = true
	maxChunkChars, maxBatchChars := w.ports.Limits()
	settings, err := w.ports.GetAIGenerationSettings()
	if err != nil {
		return characters.SummaryResponse{}, err
	}
	generationMode := resolveWorkflowGenerationMode(settings, resolvedOverride)
	generationStrategy := NormalizeGenerationStrategy(strategy)
	recorder.GenerationMode = generationMode
	recorder.GenerationStrategy = generationStrategy

	switch generationMode {
	case "openrouter":
		recorder.Enabled = true
		config, err := w.resolveConfig(resolvedOverride)
		if err != nil {
			return characters.SummaryResponse{}, err
		}
		recorder.ActiveProfile = config
		return w.generateOpenRouterPreview(ctx, recorder, novelID, upToEpisodeIndex, config, generationStrategy, maxChunkChars, maxBatchChars, progressSink, episodeIndexes, preloaded)
	case "disabled":
		return characters.SummaryResponse{}, errors.New("Character summary generation via LLM is unavailable because OpenRouter is not configured.")
	default:
		inputs, err := w.previewInputs(ctx, novelID, upToEpisodeIndex, maxChunkChars, maxBatchChars, "", nil, preloaded)
		if err != nil {
			return characters.SummaryResponse{}, err
		}
		recorder.Episodes = inputs.Episodes
		return w.ports.BuildHeuristicPreview(novelID, upToEpisodeIndex, inputs.Episodes, episodeIndexes)
	}
}

func (w *Workflow) resolveConfig(resolvedOverride *store.ResolvedAIGenerationConfig) (*store.ResolvedAIGenerationConfig, error) {
	config, err := w.ports.ResolveActiveAIGenerationConfig()
	if err != nil {
		return nil, err
	}
	if resolvedOverride != nil {
		config = resolvedOverride
	}
	if config == nil {
		return nil, errors.New("AI generation profile was not found.")
	}
	return config, nil
}

func (w *Workflow) generateOpenRouterAndSave(ctx context.Context, recorder *usageRecorder, novelID string, upToEpisodeIndex string, config *store.ResolvedAIGenerationConfig, strategy string, maxChunkChars int, maxBatchChars int, progressSink func(BatchProgress)) error {
	seedGenerated, processedIndex, hasExisting, err := w.ports.LoadGenerationSeed(novelID, upToEpisodeIndex)
	if err != nil {
		return err
	}
	identityRegistry := append([]characters.GeneratedCharacter{}, seedGenerated...)
	reprocessFromEpisodeIndex, err := w.ports.ReprocessFromEpisode(ctx, novelID, processedIndex, upToEpisodeIndex)
	if err != nil {
		return err
	}
	if hasExisting && processedIndex != nil {
		if reprocessFromEpisodeIndex == "" && episodeProcessedCovers(*processedIndex, upToEpisodeIndex) {
			if err := w.ports.MaterializeGeneratedSummary(novelID); err != nil {
				return err
			}
			recorder.Enabled = false
			return nil
		}
	}

	afterEpisodeIndex := ""
	if processedIndex != nil && reprocessFromEpisodeIndex == "" {
		afterEpisodeIndex = *processedIndex
	}
	inputs, err := w.ports.LoadInputs(ctx, novelID, upToEpisodeIndex, maxChunkChars, maxBatchChars, afterEpisodeIndex)
	if err != nil {
		return err
	}
	if reprocessFromEpisodeIndex != "" {
		inputs = filterInputsFrom(inputs, reprocessFromEpisodeIndex)
		seedGenerated, _, _, err = w.ports.LoadGeneratedCharactersBeforeEpisode(novelID, reprocessFromEpisodeIndex)
		if err != nil {
			return err
		}
	}
	pendingUnresolved, err := w.ports.LoadPendingUnresolved(novelID, reprocessFromEpisodeIndex)
	if err != nil {
		return err
	}
	inputs = w.ports.RebatchInputs(ctx, inputs, config, maxBatchChars)
	recorder.SetPlannedInputs(inputs)
	var generated []characters.GeneratedCharacter
	var generationState core.GenerationState
	var actualUsageRequests []ai.UsageRequest
	switch strategy {
	case GenerationStrategyParallelIdentity:
		generated, generationState, actualUsageRequests, err = w.ports.GenerateParallelIdentity(ctx, config, novelID, upToEpisodeIndex, seedGenerated, inputs.Batches, progressSink, pendingUnresolved)
	case GenerationStrategyDiscoveryParallelCorrection:
		generated, generationState, actualUsageRequests, err = w.ports.GenerateDiscoveryParallelCorrection(ctx, config, novelID, upToEpisodeIndex, seedGenerated, inputs.Batches, progressSink, pendingUnresolved)
	default:
		runner := generationRunner{ports: w.ports}
		generated, generationState, actualUsageRequests, err = runner.GenerateWithCheckpoint(ctx, config, novelID, upToEpisodeIndex, seedGenerated, inputs.Batches, progressSink, pendingUnresolved)
	}
	recorder.UseActualRequests(actualUsageRequests)
	if err != nil {
		return err
	}
	if reprocessFromEpisodeIndex != "" {
		generated, generationState = core.ReuseGeneratedCharacterIDsFromRegistry(generated, identityRegistry, generationState, upToEpisodeIndex)
	}
	err = w.ports.SaveGeneratedSummary(novelID, upToEpisodeIndex, generated, inputs.Episodes, characters.SaveGeneratedSummaryOptions{
		ReplaceFromEpisodeIndex: reprocessFromEpisodeIndex,
		UnresolvedMentions:      generationState.UnresolvedMentions,
		SetUnresolvedMentions:   true,
		IssuedCharacterIDs:      generationState.IssuedCharacterIDs,
		RetiredCharacterIDs:     generationState.RetiredCharacterIDs,
		NextCharacterOrdinal:    generationState.NextOrdinal,
	})
	if err == nil {
		_ = w.ports.DeleteCheckpoint(novelID, upToEpisodeIndex)
	}
	return err
}

func (w *Workflow) generateOpenRouterPreview(ctx context.Context, recorder *usageRecorder, novelID string, upToEpisodeIndex string, config *store.ResolvedAIGenerationConfig, strategy string, maxChunkChars int, maxBatchChars int, progressSink func(BatchProgress), episodeIndexes []string, preloaded *Inputs) (characters.SummaryResponse, error) {
	seedGenerated, processedIndex, hasExisting, err := w.ports.LoadGenerationSeed(novelID, upToEpisodeIndex)
	if err != nil {
		return characters.SummaryResponse{}, err
	}
	identityRegistry := append([]characters.GeneratedCharacter{}, seedGenerated...)
	reprocessFromEpisodeIndex, err := w.ports.ReprocessFromEpisode(ctx, novelID, processedIndex, upToEpisodeIndex)
	if err != nil {
		return characters.SummaryResponse{}, err
	}
	if hasExisting && processedIndex != nil {
		if reprocessFromEpisodeIndex == "" && episodeProcessedCovers(*processedIndex, upToEpisodeIndex) {
			if err := w.ports.MaterializeGeneratedSummary(novelID); err != nil {
				return characters.SummaryResponse{}, err
			}
			recorder.Enabled = false
			return w.ports.LoadRequiredPreview(novelID, upToEpisodeIndex, episodeIndexes)
		}
	}

	inputs, err := w.previewInputs(ctx, novelID, upToEpisodeIndex, maxChunkChars, maxBatchChars, reprocessFromEpisodeIndex, processedIndex, preloaded)
	if err != nil {
		return characters.SummaryResponse{}, err
	}
	if reprocessFromEpisodeIndex != "" {
		seedGenerated, _, _, err = w.ports.LoadGeneratedCharactersBeforeEpisode(novelID, reprocessFromEpisodeIndex)
		if err != nil {
			return characters.SummaryResponse{}, err
		}
	}
	pendingUnresolved, err := w.ports.LoadPendingUnresolved(novelID, reprocessFromEpisodeIndex)
	if err != nil {
		return characters.SummaryResponse{}, err
	}
	inputs = w.ports.RebatchInputs(ctx, inputs, config, maxBatchChars)
	recorder.SetPlannedInputs(inputs)
	var generated []characters.GeneratedCharacter
	var generationState core.GenerationState
	var actualUsageRequests []ai.UsageRequest
	switch strategy {
	case GenerationStrategyParallelIdentity:
		generated, generationState, actualUsageRequests, err = w.ports.GenerateParallelIdentity(ctx, config, novelID, upToEpisodeIndex, seedGenerated, inputs.Batches, progressSink, pendingUnresolved)
	case GenerationStrategyDiscoveryParallelCorrection:
		generated, generationState, actualUsageRequests, err = w.ports.GenerateDiscoveryParallelCorrection(ctx, config, novelID, upToEpisodeIndex, seedGenerated, inputs.Batches, progressSink, pendingUnresolved)
	default:
		runner := generationRunner{ports: w.ports}
		generated, generationState, actualUsageRequests, err = runner.GeneratePreview(ctx, config, novelID, upToEpisodeIndex, seedGenerated, inputs.Batches, progressSink, pendingUnresolved)
	}
	recorder.UseActualRequests(actualUsageRequests)
	if err != nil {
		return characters.SummaryResponse{}, err
	}
	if reprocessFromEpisodeIndex != "" {
		generated, generationState = core.ReuseGeneratedCharacterIDsFromRegistry(generated, identityRegistry, generationState, upToEpisodeIndex)
	}
	return w.ports.BuildGeneratedPreview(novelID, upToEpisodeIndex, generated, inputs.Episodes, episodeIndexes, characters.SaveGeneratedSummaryOptions{
		ReplaceFromEpisodeIndex: reprocessFromEpisodeIndex,
		UnresolvedMentions:      generationState.UnresolvedMentions,
		SetUnresolvedMentions:   true,
		IssuedCharacterIDs:      generationState.IssuedCharacterIDs,
		RetiredCharacterIDs:     generationState.RetiredCharacterIDs,
		NextCharacterOrdinal:    generationState.NextOrdinal,
	})
}

func (w *Workflow) previewInputs(ctx context.Context, novelID string, upToEpisodeIndex string, maxChunkChars int, maxBatchChars int, reprocessFromEpisodeIndex string, processedIndex *string, preloaded *Inputs) (Inputs, error) {
	if preloaded != nil {
		inputs := *preloaded
		if reprocessFromEpisodeIndex != "" {
			return filterInputsFrom(inputs, reprocessFromEpisodeIndex), nil
		}
		if processedIndex != nil {
			return filterInputsAfter(inputs, *processedIndex), nil
		}
		return inputs, nil
	}
	afterEpisodeIndex := ""
	if processedIndex != nil && reprocessFromEpisodeIndex == "" {
		afterEpisodeIndex = *processedIndex
	}
	inputs, err := w.ports.LoadInputs(ctx, novelID, upToEpisodeIndex, maxChunkChars, maxBatchChars, afterEpisodeIndex)
	if err != nil {
		return Inputs{}, err
	}
	if reprocessFromEpisodeIndex != "" {
		inputs = filterInputsFrom(inputs, reprocessFromEpisodeIndex)
	}
	return inputs, nil
}

type usageRecorder struct {
	ctx                context.Context
	ports              WorkflowPorts
	runPrefix          string
	started            time.Time
	startedAt          string
	NovelID            string
	UpToEpisodeIndex   string
	GenerationMode     string
	GenerationStrategy string
	ActiveProfile      *store.ResolvedAIGenerationConfig
	Episodes           []characters.HeuristicEpisode
	Requests           []ai.UsageRequest
	Enabled            bool
	PreviewOnly        bool
}

func newUsageRecorder(ctx context.Context, ports WorkflowPorts, runPrefix string, novelID string, upToEpisodeIndex string) *usageRecorder {
	started := time.Now()
	return &usageRecorder{
		ctx:              ctx,
		ports:            ports,
		runPrefix:        runPrefix,
		started:          started,
		startedAt:        started.UTC().Format(time.RFC3339Nano),
		NovelID:          novelID,
		UpToEpisodeIndex: upToEpisodeIndex,
		Episodes:         []characters.HeuristicEpisode{},
		Requests:         []ai.UsageRequest{},
	}
}

func (r *usageRecorder) SetPlannedInputs(inputs Inputs) {
	if r == nil {
		return
	}
	r.Episodes = inputs.Episodes
	r.Requests = batchUsageRequests(inputs.Batches)
}

func (r *usageRecorder) UseActualRequests(requests []ai.UsageRequest) {
	if r == nil || len(requests) == 0 {
		return
	}
	r.Requests = requests
}

func (r *usageRecorder) Finish(err error) {
	if r == nil || !r.Enabled {
		return
	}
	finishedAt := time.Now().UTC().Format(time.RFC3339Nano)
	status := "completed"
	var errorMessage *string
	if err != nil {
		status = "failed"
		message := err.Error()
		errorMessage = &message
	}
	snapshot := map[string]any{
		"novelId":            r.NovelID,
		"upToEpisodeIndex":   r.UpToEpisodeIndex,
		"episodeIndexes":     heuristicEpisodeIndexes(r.Episodes),
		"episodeCount":       len(r.Episodes),
		"generationMode":     r.GenerationMode,
		"generationStrategy": r.GenerationStrategy,
	}
	if strategyModels := resolvedStrategyModels(r.GenerationStrategy, r.ActiveProfile); strategyModels != nil {
		snapshot["strategyModels"] = strategyModels
	}
	if r.PreviewOnly {
		snapshot["previewOnly"] = true
	} else {
		snapshot["checkpointRemainingExists"] = r.ports.CheckpointExists(r.NovelID, r.UpToEpisodeIndex)
	}
	_ = r.ports.RecordUsage(ai.UsageRun{
		RunID:               r.runPrefix + "-" + strconv.FormatInt(time.Now().UnixNano(), 36),
		Feature:             "character-summary",
		WorkflowName:        "character-summary",
		Status:              status,
		StartedAt:           r.startedAt,
		FinishedAt:          finishedAt,
		ElapsedMs:           int(time.Since(r.started).Milliseconds()),
		NovelID:             &r.NovelID,
		NovelTitle:          r.ports.NovelTitle(r.ctx, r.NovelID),
		CurrentEpisodeIndex: &r.UpToEpisodeIndex,
		ModelID:             resolvedModelID(r.ActiveProfile),
		ProfileID:           resolvedProfileID(r.ActiveProfile),
		ProfileLabel:        resolvedProfileLabel(r.ActiveProfile),
		GenerationMode:      r.GenerationMode,
		RequestCount:        len(r.Requests),
		InputTokens:         usageRequestsInputTokens(r.Requests),
		OutputTokens:        usageRequestsOutputTokens(r.Requests),
		TotalTokens:         usageRequestsTotalTokens(r.Requests),
		ErrorMessage:        errorMessage,
		Requests:            r.Requests,
		Snapshot:            snapshot,
	})
}

func resolveWorkflowGenerationMode(settings ai.SettingsResponse, resolvedOverride *store.ResolvedAIGenerationConfig) string {
	if resolvedOverride != nil {
		return "openrouter"
	}
	if strings.TrimSpace(settings.EffectiveGenerationMode) != "" {
		return settings.EffectiveGenerationMode
	}
	return "heuristic"
}

func resolvedProfileID(config *store.ResolvedAIGenerationConfig) *string {
	if config == nil || strings.TrimSpace(config.ProfileID) == "" {
		return nil
	}
	return &config.ProfileID
}

func resolvedProfileLabel(config *store.ResolvedAIGenerationConfig) *string {
	if config == nil || strings.TrimSpace(config.ProfileLabel) == "" {
		return nil
	}
	return &config.ProfileLabel
}

func resolvedModelID(config *store.ResolvedAIGenerationConfig) *string {
	if config == nil || strings.TrimSpace(config.ModelID) == "" {
		return nil
	}
	return &config.ModelID
}

func resolvedStrategyModels(strategy string, config *store.ResolvedAIGenerationConfig) map[string]string {
	if strategy != GenerationStrategyDiscoveryParallelCorrection || config == nil || strings.TrimSpace(config.ModelID) == "" {
		return nil
	}
	detailModelID := strings.TrimSpace(config.ModelID)
	discoveryModelID := strings.TrimSpace(config.CharacterSummaryNameDiscoveryModelID)
	if discoveryModelID == "" {
		discoveryModelID = detailModelID
	}
	return map[string]string{
		"discovery":  discoveryModelID,
		"detail":     detailModelID,
		"correction": detailModelID,
	}
}

func heuristicEpisodeIndexes(episodes []characters.HeuristicEpisode) []string {
	indexes := make([]string, 0, len(episodes))
	for _, episode := range episodes {
		indexes = append(indexes, episode.EpisodeIndex)
	}
	return indexes
}

func batchUsageRequests(batches []core.Batch) []ai.UsageRequest {
	requests := make([]ai.UsageRequest, 0, len(batches))
	for index, batch := range batches {
		inputTokens := 0
		for _, chunk := range batch.Chunks {
			inputTokens += core.TokensFromChars(len([]rune(strings.TrimSpace(chunk.Text))))
		}
		requests = append(requests, ai.UsageRequest{
			RequestIndex: index,
			Kind:         "character_summary_batch",
			InputTokens:  inputTokens,
			TotalTokens:  inputTokens,
		})
	}
	return requests
}

func usageRequestsInputTokens(requests []ai.UsageRequest) int {
	total := 0
	for _, request := range requests {
		total += request.InputTokens
	}
	return total
}

func usageRequestsOutputTokens(requests []ai.UsageRequest) int {
	total := 0
	for _, request := range requests {
		total += request.OutputTokens
	}
	return total
}

func usageRequestsTotalTokens(requests []ai.UsageRequest) int {
	total := 0
	for _, request := range requests {
		if request.TotalTokens > 0 {
			total += request.TotalTokens
			continue
		}
		total += request.InputTokens + request.OutputTokens
	}
	return total
}

func filterInputsAfter(inputs Inputs, processedEpisodeIndex string) Inputs {
	if strings.TrimSpace(processedEpisodeIndex) == "" {
		return inputs
	}
	filteredEpisodes := make([]characters.HeuristicEpisode, 0, len(inputs.Episodes))
	for _, episode := range inputs.Episodes {
		if compareEpisodeString(episode.EpisodeIndex, processedEpisodeIndex) > 0 {
			filteredEpisodes = append(filteredEpisodes, episode)
		}
	}
	chunks := []core.Chunk{}
	for _, batch := range inputs.Batches {
		for _, chunk := range batch.Chunks {
			if compareEpisodeString(chunk.EpisodeIndex, processedEpisodeIndex) > 0 {
				chunks = append(chunks, chunk)
			}
		}
	}
	return Inputs{
		Episodes: filteredEpisodes,
		Batches:  core.CreateBatchesWithBudget(chunks, core.BatchBudget{}),
	}
}

func filterInputsFrom(inputs Inputs, fromEpisodeIndex string) Inputs {
	if strings.TrimSpace(fromEpisodeIndex) == "" {
		return inputs
	}
	filteredEpisodes := make([]characters.HeuristicEpisode, 0, len(inputs.Episodes))
	for _, episode := range inputs.Episodes {
		if compareEpisodeString(episode.EpisodeIndex, fromEpisodeIndex) >= 0 {
			filteredEpisodes = append(filteredEpisodes, episode)
		}
	}
	chunks := []core.Chunk{}
	for _, batch := range inputs.Batches {
		for _, chunk := range batch.Chunks {
			if compareEpisodeString(chunk.EpisodeIndex, fromEpisodeIndex) >= 0 {
				chunks = append(chunks, chunk)
			}
		}
	}
	return Inputs{
		Episodes: filteredEpisodes,
		Batches:  core.CreateBatchesWithBudget(chunks, core.BatchBudget{}),
	}
}

func episodeProcessedCovers(processedEpisodeIndex string, requestedEpisodeIndex string) bool {
	processedEpisodeIndex = strings.TrimSpace(processedEpisodeIndex)
	requestedEpisodeIndex = strings.TrimSpace(requestedEpisodeIndex)
	if processedEpisodeIndex == "" || requestedEpisodeIndex == "" {
		return false
	}
	return compareEpisodeString(processedEpisodeIndex, requestedEpisodeIndex) >= 0
}

func compareEpisodeString(left string, right string) int {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	leftValue, leftErr := strconv.Atoi(left)
	rightValue, rightErr := strconv.Atoi(right)
	if leftErr == nil && rightErr == nil {
		switch {
		case leftValue < rightValue:
			return -1
		case leftValue > rightValue:
			return 1
		default:
			return 0
		}
	}
	return strings.Compare(left, right)
}
