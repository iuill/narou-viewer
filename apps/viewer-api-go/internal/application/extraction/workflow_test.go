package extraction

import (
	"context"
	"errors"
	"testing"

	"narou-viewer/apps/viewer-api-go/internal/ai"
	"narou-viewer/apps/viewer-api-go/internal/characters"
	core "narou-viewer/apps/viewer-api-go/internal/extraction"
	"narou-viewer/apps/viewer-api-go/internal/extraction/checkpointstore"
	"narou-viewer/apps/viewer-api-go/internal/store"
)

type workflowFakePorts struct {
	settings       ai.SettingsResponse
	config         *store.ResolvedAIGenerationConfig
	inputs         Inputs
	rebatchOutput  *Inputs
	loadErr        error
	seed           []characters.GeneratedCharacter
	processedIndex *string
	hasExisting    bool
	reprocessFrom  string

	locked            bool
	materialized      bool
	saveHeuristic     bool
	saveGenerated     bool
	removedCheckpoint bool
	builtGenerated    bool
	loadedPreview     bool
	recordedUsage     []ai.UsageRun
	generateErr       error
	generateErrAfter  int
	parallelCalls     int
	planErr           error
	allocatorErr      error
	generateCalls     int
	rebatchCalls      int
	heuristicEpisodes []characters.HeuristicEpisode
	generatedEpisodes []characters.HeuristicEpisode
	checkpoint        checkpointstore.Checkpoint
	savedCheckpoint   bool
}

func (p *workflowFakePorts) LockTarget(string, string) func() {
	p.locked = true
	return func() {}
}

func (p *workflowFakePorts) GetAIGenerationSettings() (ai.SettingsResponse, error) {
	return p.settings, nil
}

func (p *workflowFakePorts) ResolveActiveAIGenerationConfig() (*store.ResolvedAIGenerationConfig, error) {
	return p.config, nil
}

func (p *workflowFakePorts) NovelTitle(context.Context, string) *string {
	title := "作品A"
	return &title
}

func (p *workflowFakePorts) RecordUsage(run ai.UsageRun) error {
	p.recordedUsage = append(p.recordedUsage, run)
	return nil
}

func (p *workflowFakePorts) Limits() (int, int) {
	return 4000, 12000
}

func (p *workflowFakePorts) LoadInputs(context.Context, string, string, int, int, string) (Inputs, error) {
	if p.loadErr != nil {
		return Inputs{}, p.loadErr
	}
	return p.inputs, nil
}

func (p *workflowFakePorts) LoadGenerationSeed(string, string) ([]characters.GeneratedCharacter, *string, bool, error) {
	return p.seed, p.processedIndex, p.hasExisting, nil
}

func (p *workflowFakePorts) LoadGeneratedCharactersBeforeEpisode(string, string) ([]characters.GeneratedCharacter, *string, bool, error) {
	return p.seed, p.processedIndex, p.hasExisting, nil
}

func (p *workflowFakePorts) ReprocessFromEpisode(context.Context, string, *string, string) (string, error) {
	return p.reprocessFrom, nil
}

func (p *workflowFakePorts) MaterializeGeneratedSummary(string) error {
	p.materialized = true
	return nil
}

func (p *workflowFakePorts) LoadPendingUnresolved(string, string) ([]characters.GeneratedUnresolvedMention, error) {
	return nil, nil
}

func (p *workflowFakePorts) RebatchInputs(_ context.Context, inputs Inputs, _ *store.ResolvedAIGenerationConfig, _ int) Inputs {
	p.rebatchCalls++
	if p.rebatchOutput != nil {
		return *p.rebatchOutput
	}
	return inputs
}

func (p *workflowFakePorts) LoadIDAllocator(novelID string, seed []characters.GeneratedCharacter) (*characters.GeneratedCharacterIDAllocator, error) {
	if p.allocatorErr != nil {
		return nil, p.allocatorErr
	}
	return characters.NewGeneratedCharacterIDAllocator(novelID, seed), nil
}

func (p *workflowFakePorts) PlanRuntimeBatch(_ context.Context, _ *store.ResolvedAIGenerationConfig, _ string, _ string, _ []characters.GeneratedCharacter, template core.Batch, chunks []core.Chunk, _ []characters.GeneratedUnresolvedMention) (core.Batch, []core.Chunk, error) {
	if p.planErr != nil {
		return core.Batch{}, nil, p.planErr
	}
	return core.RuntimeBatch(template, chunks), nil, nil
}

func (p *workflowFakePorts) GenerateBatch(context.Context, *store.ResolvedAIGenerationConfig, string, string, []characters.GeneratedCharacter, core.Batch, []characters.GeneratedUnresolvedMention) (BatchResult, error) {
	if p.generateErr != nil && (p.generateErrAfter == 0 || p.generateCalls >= p.generateErrAfter) {
		return BatchResult{Usage: ai.UsageRequest{RequestIndex: 0, Kind: "character_summary_batch", InputTokens: 10, OutputTokens: 3, TotalTokens: 13}}, p.generateErr
	}
	p.generateCalls++
	return BatchResult{
		Delta: core.Delta{NewCharacters: []characters.GeneratedCharacter{{CanonicalName: "Alice", CanonicalEpisodeIndex: "1", SummaryHistory: []characters.GeneratedHistoryVersion{{EpisodeIndex: "1", Text: "本文"}}}}},
		Usage: ai.UsageRequest{RequestIndex: 0, Kind: "character_summary_batch", InputTokens: 10, OutputTokens: 3, TotalTokens: 13},
	}, nil
}

func (p *workflowFakePorts) GenerateParallelIdentity(_ context.Context, _ *store.ResolvedAIGenerationConfig, _ string, _ string, seed []characters.GeneratedCharacter, _ []core.Batch, _ func(BatchProgress), _ []characters.GeneratedUnresolvedMention) ([]characters.GeneratedCharacter, core.GenerationState, []ai.UsageRequest, error) {
	p.parallelCalls++
	if p.generateErr != nil {
		return nil, core.GenerationState{}, nil, p.generateErr
	}
	generated := append([]characters.GeneratedCharacter{}, seed...)
	generated = append(generated, characters.GeneratedCharacter{CharacterID: "char_parallel", CanonicalName: "Parallel", CanonicalEpisodeIndex: "1", FirstAppearanceEpisodeIndex: "1"})
	return generated, core.GenerationState{}, []ai.UsageRequest{{RequestIndex: 0, Kind: "character_summary_parallel_identity", InputTokens: 12, OutputTokens: 4, TotalTokens: 16}}, nil
}

func (p *workflowFakePorts) GenerateDiscoveryParallelCorrection(_ context.Context, _ *store.ResolvedAIGenerationConfig, _ string, _ string, seed []characters.GeneratedCharacter, _ []core.Batch, _ func(BatchProgress), _ []characters.GeneratedUnresolvedMention) ([]characters.GeneratedCharacter, core.GenerationState, []ai.UsageRequest, error) {
	p.parallelCalls++
	if p.generateErr != nil {
		return nil, core.GenerationState{}, nil, p.generateErr
	}
	generated := append([]characters.GeneratedCharacter{}, seed...)
	generated = append(generated, characters.GeneratedCharacter{CharacterID: "char_discovery", CanonicalName: "Discovery", CanonicalEpisodeIndex: "1", FirstAppearanceEpisodeIndex: "1"})
	return generated, core.GenerationState{}, []ai.UsageRequest{{RequestIndex: 0, Kind: "character_summary_discovery_parallel_correction", InputTokens: 16, OutputTokens: 5, TotalTokens: 21}}, nil
}

func (p *workflowFakePorts) LoadCheckpoint(string, string) (checkpointstore.Checkpoint, error) {
	if p.checkpoint.NovelID == "" {
		return checkpointstore.Checkpoint{}, errors.New("missing checkpoint")
	}
	return p.checkpoint, nil
}

func (p *workflowFakePorts) SaveCheckpoint(_ string, _ string, checkpoint checkpointstore.Checkpoint) error {
	p.savedCheckpoint = true
	p.checkpoint = checkpoint
	return nil
}

func (p *workflowFakePorts) DeleteCheckpoint(string, string) error {
	p.removedCheckpoint = true
	return nil
}

func (p *workflowFakePorts) SaveGeneratedSummary(_ string, _ string, _ []characters.GeneratedCharacter, episodes []characters.HeuristicEpisode, _ characters.SaveGeneratedSummaryOptions) error {
	p.saveGenerated = true
	p.generatedEpisodes = append([]characters.HeuristicEpisode{}, episodes...)
	return nil
}

func (p *workflowFakePorts) BuildGeneratedPreview(_ string, _ string, _ []characters.GeneratedCharacter, episodes []characters.HeuristicEpisode, _ []string, _ characters.SaveGeneratedSummaryOptions) (characters.SummaryResponse, error) {
	p.builtGenerated = true
	p.generatedEpisodes = append([]characters.HeuristicEpisode{}, episodes...)
	return characters.SummaryResponse{Status: "ready"}, nil
}

func (p *workflowFakePorts) SaveHeuristicSummary(_ string, _ string, episodes []characters.HeuristicEpisode) error {
	p.saveHeuristic = true
	p.heuristicEpisodes = append([]characters.HeuristicEpisode{}, episodes...)
	return nil
}

func (p *workflowFakePorts) BuildHeuristicPreview(_ string, _ string, episodes []characters.HeuristicEpisode, _ []string) (characters.SummaryResponse, error) {
	p.heuristicEpisodes = append([]characters.HeuristicEpisode{}, episodes...)
	return characters.SummaryResponse{Status: "ready"}, nil
}

func (p *workflowFakePorts) LoadRequiredPreview(string, string, []string) (characters.SummaryResponse, error) {
	p.loadedPreview = true
	return characters.SummaryResponse{Status: "ready"}, nil
}

func (p *workflowFakePorts) CheckpointExists(string, string) bool {
	return true
}

func workflowInputs() Inputs {
	return Inputs{
		Episodes: []characters.HeuristicEpisode{{EpisodeIndex: "1", Text: "本文"}},
		Batches: []core.Batch{{
			BatchIndex:     1,
			BatchCount:     1,
			EpisodeIndexes: []string{"1"},
			Chunks:         []core.Chunk{{EpisodeIndex: "1", Text: "本文"}},
		}},
	}
}

func workflowInputsWithEpisodes(indexes ...string) Inputs {
	episodes := make([]characters.HeuristicEpisode, 0, len(indexes))
	chunks := make([]core.Chunk, 0, len(indexes))
	for _, index := range indexes {
		episodes = append(episodes, characters.HeuristicEpisode{EpisodeIndex: index, Text: "本文" + index})
		chunks = append(chunks, core.Chunk{EpisodeIndex: index, Text: "本文" + index})
	}
	return Inputs{
		Episodes: episodes,
		Batches: []core.Batch{{
			BatchIndex:     1,
			BatchCount:     1,
			EpisodeIndexes: append([]string{}, indexes...),
			Chunks:         chunks,
		}},
	}
}

func TestWorkflowGenerateAndSaveOpenRouterRecordsUsageAndSaves(t *testing.T) {
	ports := &workflowFakePorts{
		settings: ai.SettingsResponse{EffectiveGenerationMode: "openrouter"},
		config:   &store.ResolvedAIGenerationConfig{ProfileID: "profile-a", ProfileLabel: "Profile A", ModelID: "model-a"},
		inputs:   workflowInputs(),
	}

	err := NewWorkflow(ports).GenerateAndSave(context.Background(), "novel-a", "1", nil, "", nil)
	if err != nil {
		t.Fatalf("GenerateAndSave returned error: %v", err)
	}
	if !ports.locked || !ports.saveGenerated || !ports.removedCheckpoint {
		t.Fatalf("workflow did not lock/save/remove checkpoint: %+v", ports)
	}
	if len(ports.recordedUsage) != 1 {
		t.Fatalf("recordedUsage length = %d, want 1", len(ports.recordedUsage))
	}
	run := ports.recordedUsage[0]
	if run.Status != "completed" || run.InputTokens != 10 || run.OutputTokens != 3 || run.TotalTokens != 13 {
		t.Fatalf("usage run = %+v, want completed provider token counts", run)
	}
	if run.ProfileID == nil || *run.ProfileID != "profile-a" {
		t.Fatalf("usage profile = %#v, want profile-a", run.ProfileID)
	}
}

func TestWorkflowGenerateAndSaveParallelIdentityUsesStrategy(t *testing.T) {
	ports := &workflowFakePorts{
		settings: ai.SettingsResponse{EffectiveGenerationMode: "openrouter"},
		config:   &store.ResolvedAIGenerationConfig{ProfileID: "profile-a", ProfileLabel: "Profile A", ModelID: "model-a"},
		inputs:   workflowInputs(),
	}

	err := NewWorkflow(ports).GenerateAndSave(context.Background(), "novel-a", "1", nil, GenerationStrategyParallelIdentity, nil)
	if err != nil {
		t.Fatalf("GenerateAndSave returned error: %v", err)
	}
	if ports.parallelCalls != 1 || ports.generateCalls != 0 {
		t.Fatalf("parallelCalls=%d generateCalls=%d, want parallel only", ports.parallelCalls, ports.generateCalls)
	}
	if !ports.saveGenerated || !ports.removedCheckpoint {
		t.Fatalf("parallel workflow should save and delete stale serial checkpoint: %+v", ports)
	}
	if len(ports.recordedUsage) != 1 || ports.recordedUsage[0].Snapshot == nil {
		t.Fatalf("recordedUsage = %+v, want snapshot", ports.recordedUsage)
	}
	snapshot, ok := ports.recordedUsage[0].Snapshot.(map[string]any)
	if !ok || snapshot["generationStrategy"] != GenerationStrategyParallelIdentity {
		t.Fatalf("snapshot = %#v, want generationStrategy", ports.recordedUsage[0].Snapshot)
	}
}

func TestWorkflowGenerateAndSaveHeuristicSavesWithoutUsage(t *testing.T) {
	ports := &workflowFakePorts{
		settings: ai.SettingsResponse{EffectiveGenerationMode: "heuristic"},
		inputs:   workflowInputs(),
	}

	err := NewWorkflow(ports).GenerateAndSave(context.Background(), "novel-a", "1", nil, "", nil)
	if err != nil {
		t.Fatalf("GenerateAndSave returned error: %v", err)
	}
	if !ports.locked || !ports.saveHeuristic {
		t.Fatalf("heuristic workflow should lock and save: %+v", ports)
	}
	if len(ports.heuristicEpisodes) != 1 || ports.heuristicEpisodes[0].EpisodeIndex != "1" {
		t.Fatalf("heuristic episodes = %+v, want episode 1", ports.heuristicEpisodes)
	}
	if len(ports.recordedUsage) != 0 {
		t.Fatalf("recordedUsage length = %d, want 0", len(ports.recordedUsage))
	}
}

func TestWorkflowPrepareInputsLoadsAndRebatches(t *testing.T) {
	ports := &workflowFakePorts{inputs: workflowInputs()}

	inputs, err := NewWorkflow(ports).PrepareInputs(context.Background(), "novel-a", "1", &store.ResolvedAIGenerationConfig{ModelID: "model-a"})
	if err != nil {
		t.Fatalf("PrepareInputs returned error: %v", err)
	}
	if len(inputs.Episodes) != 1 || inputs.Episodes[0].EpisodeIndex != "1" {
		t.Fatalf("inputs = %+v, want episode 1", inputs)
	}
	if ports.rebatchCalls != 1 {
		t.Fatalf("rebatchCalls = %d, want 1", ports.rebatchCalls)
	}
}

func TestWorkflowPreparePreviewUsesRebatchedInputsForPromptPreview(t *testing.T) {
	systemPrompt := "preview prompt"
	rebatchOutput := workflowInputsWithEpisodes("2")
	rebatchOutput.Batches[0].BatchIndex = 7
	rebatchOutput.Batches[0].BatchCount = 9
	rebatchOutput.Batches[0].Chunks[0].Title = "rebatch title"
	ports := &workflowFakePorts{
		inputs:        workflowInputsWithEpisodes("1"),
		rebatchOutput: &rebatchOutput,
	}

	prepared, err := NewWorkflow(ports).PreparePreview(context.Background(), "novel-a", "2", &store.ResolvedAIGenerationConfig{SystemPrompt: &systemPrompt})
	if err != nil {
		t.Fatalf("PreparePreview returned error: %v", err)
	}
	if ports.rebatchCalls != 1 {
		t.Fatalf("rebatchCalls = %d, want 1", ports.rebatchCalls)
	}
	if len(prepared.Inputs.Episodes) != 1 || prepared.Inputs.Episodes[0].EpisodeIndex != "2" {
		t.Fatalf("prepared inputs = %+v, want rebatch output", prepared.Inputs)
	}
	if prepared.Preview.SystemPrompt != systemPrompt {
		t.Fatalf("SystemPrompt = %q, want override", prepared.Preview.SystemPrompt)
	}
	if len(prepared.Preview.Batches) != 1 || prepared.Preview.Batches[0].BatchIndex != 7 || prepared.Preview.Batches[0].ChunkCount != 1 {
		t.Fatalf("preview batches = %+v, want rebatched batch metadata", prepared.Preview.Batches)
	}
	if prepared.Preview.Batches[0].Chunks[0].Title != "rebatch title" || prepared.Preview.Batches[0].Chunks[0].EpisodeIndex != "2" {
		t.Fatalf("preview chunks = %+v, want rebatched chunk", prepared.Preview.Batches[0].Chunks)
	}
}

func TestWorkflowPreparePreviewPropagatesLoadError(t *testing.T) {
	ports := &workflowFakePorts{loadErr: errors.New("load failed")}

	if _, err := NewWorkflow(ports).PrepareInputs(context.Background(), "novel-a", "1", nil); err == nil {
		t.Fatal("PrepareInputs should return load error")
	}
	if _, err := NewWorkflow(ports).PreparePreview(context.Background(), "novel-a", "1", nil); err == nil {
		t.Fatal("PreparePreview should return load error")
	}
}

func TestWorkflowGeneratePreviewOpenRouterFiltersPreloadedInputs(t *testing.T) {
	processed := "1"
	ports := &workflowFakePorts{
		settings:       ai.SettingsResponse{EffectiveGenerationMode: "openrouter"},
		config:         &store.ResolvedAIGenerationConfig{ProfileID: "profile-a", ProfileLabel: "Profile A", ModelID: "model-a"},
		processedIndex: &processed,
		inputs:         workflowInputsWithEpisodes("1", "2"),
	}
	preloaded := workflowInputsWithEpisodes("1", "2")

	summary, err := NewWorkflow(ports).GeneratePreview(context.Background(), "novel-a", "2", nil, "", nil, []string{"1", "2"}, &preloaded)
	if err != nil {
		t.Fatalf("GeneratePreview returned error: %v", err)
	}
	if summary.Status != "ready" || !ports.builtGenerated {
		t.Fatalf("summary = %+v builtGenerated=%v, want ready generated preview", summary, ports.builtGenerated)
	}
	if len(ports.generatedEpisodes) != 1 || ports.generatedEpisodes[0].EpisodeIndex != "2" {
		t.Fatalf("generatedEpisodes = %+v, want only episode 2", ports.generatedEpisodes)
	}
	if len(ports.recordedUsage) != 1 {
		t.Fatalf("preview usage = %+v, want one preview usage run", ports.recordedUsage)
	}
	snapshot, _ := ports.recordedUsage[0].Snapshot.(map[string]any)
	if snapshot["previewOnly"] != true {
		t.Fatalf("preview usage snapshot = %+v, want previewOnly", snapshot)
	}
}

func TestWorkflowGeneratePreviewExistingSummaryLoadsPreviewWithoutUsage(t *testing.T) {
	processed := "2"
	ports := &workflowFakePorts{
		settings:       ai.SettingsResponse{EffectiveGenerationMode: "openrouter"},
		config:         &store.ResolvedAIGenerationConfig{ProfileID: "profile-a", ModelID: "model-a"},
		processedIndex: &processed,
		hasExisting:    true,
	}

	summary, err := NewWorkflow(ports).GeneratePreview(context.Background(), "novel-a", "1", nil, "", nil, []string{"1"}, nil)
	if err != nil {
		t.Fatalf("GeneratePreview returned error: %v", err)
	}
	if summary.Status != "ready" || !ports.materialized || !ports.loadedPreview {
		t.Fatalf("summary=%+v materialized=%v loadedPreview=%v, want loaded preview", summary, ports.materialized, ports.loadedPreview)
	}
	if len(ports.recordedUsage) != 0 {
		t.Fatalf("recordedUsage length = %d, want 0", len(ports.recordedUsage))
	}
}

func TestWorkflowGeneratePreviewHeuristicUsesPreloadedInputs(t *testing.T) {
	ports := &workflowFakePorts{settings: ai.SettingsResponse{EffectiveGenerationMode: "heuristic"}}
	preloaded := workflowInputsWithEpisodes("1", "2")

	summary, err := NewWorkflow(ports).GeneratePreview(context.Background(), "novel-a", "2", nil, "", nil, []string{"1", "2"}, &preloaded)
	if err != nil {
		t.Fatalf("GeneratePreview returned error: %v", err)
	}
	if summary.Status != "ready" {
		t.Fatalf("summary = %+v, want ready", summary)
	}
	if len(ports.heuristicEpisodes) != 2 {
		t.Fatalf("heuristicEpisodes = %+v, want preloaded episodes", ports.heuristicEpisodes)
	}
	if len(ports.recordedUsage) != 0 {
		t.Fatalf("recordedUsage length = %d, want 0", len(ports.recordedUsage))
	}
}

func TestWorkflowGeneratePreviewOpenRouterLoadsAndFiltersReprocessInputs(t *testing.T) {
	processed := "3"
	ports := &workflowFakePorts{
		settings:       ai.SettingsResponse{EffectiveGenerationMode: "heuristic"},
		config:         &store.ResolvedAIGenerationConfig{ProfileID: "profile-a", ProfileLabel: "Profile A", ModelID: "model-a"},
		processedIndex: &processed,
		reprocessFrom:  "2",
		inputs:         workflowInputsWithEpisodes("1", "2", "3"),
	}
	override := &store.ResolvedAIGenerationConfig{ProfileID: "override", ProfileLabel: "Override", ModelID: "model-b"}

	_, err := NewWorkflow(ports).GeneratePreview(context.Background(), "novel-a", "3", override, "", nil, []string{"1", "2", "3"}, nil)
	if err != nil {
		t.Fatalf("GeneratePreview returned error: %v", err)
	}
	if len(ports.generatedEpisodes) != 2 || ports.generatedEpisodes[0].EpisodeIndex != "2" {
		t.Fatalf("generatedEpisodes = %+v, want episodes 2 and 3", ports.generatedEpisodes)
	}
	if len(ports.recordedUsage) != 1 || ports.recordedUsage[0].ProfileID == nil || *ports.recordedUsage[0].ProfileID != "override" {
		t.Fatalf("usage = %+v, want override profile usage", ports.recordedUsage)
	}
}

func TestWorkflowOpenRouterMissingProfileRecordsFailedUsage(t *testing.T) {
	ports := &workflowFakePorts{
		settings: ai.SettingsResponse{EffectiveGenerationMode: "openrouter"},
		inputs:   workflowInputs(),
	}

	err := NewWorkflow(ports).GenerateAndSave(context.Background(), "novel-a", "1", nil, "", nil)
	if err == nil {
		t.Fatal("GenerateAndSave should fail when profile is missing")
	}
	if len(ports.recordedUsage) != 1 || ports.recordedUsage[0].Status != "failed" {
		t.Fatalf("recordedUsage = %+v, want failed usage", ports.recordedUsage)
	}
}

func TestWorkflowDisabledModeReturnsUnavailableError(t *testing.T) {
	ports := &workflowFakePorts{settings: ai.SettingsResponse{EffectiveGenerationMode: "disabled"}}

	if err := NewWorkflow(ports).GenerateAndSave(context.Background(), "novel-a", "1", nil, "", nil); err == nil {
		t.Fatal("GenerateAndSave should fail in disabled mode")
	}
	if _, err := NewWorkflow(ports).GeneratePreview(context.Background(), "novel-a", "1", nil, "", nil, []string{"1"}, nil); err == nil {
		t.Fatal("GeneratePreview should fail in disabled mode")
	}
}

func TestWorkflowSmallHelpers(t *testing.T) {
	settings := ai.SettingsResponse{}
	if mode := resolveWorkflowGenerationMode(settings, nil); mode != "heuristic" {
		t.Fatalf("default generation mode = %q, want heuristic", mode)
	}
	override := &store.ResolvedAIGenerationConfig{ModelID: "model-a"}
	if mode := resolveWorkflowGenerationMode(settings, override); mode != "openrouter" {
		t.Fatalf("override generation mode = %q, want openrouter", mode)
	}
	if resolvedProfileID(nil) != nil || resolvedModelID(&store.ResolvedAIGenerationConfig{}) != nil {
		t.Fatal("empty profile helpers should return nil")
	}
	strategyModels := resolvedStrategyModels(GenerationStrategyDiscoveryParallelCorrection, &store.ResolvedAIGenerationConfig{
		ModelID:                              "openai/gpt-5-mini",
		CharacterSummaryNameDiscoveryModelID: "openai/gpt-5-nano",
	})
	if strategyModels["discovery"] != "openai/gpt-5-nano" || strategyModels["detail"] != "openai/gpt-5-mini" || strategyModels["correction"] != "openai/gpt-5-mini" {
		t.Fatalf("unexpected strategy model snapshot: %+v", strategyModels)
	}
	strategyModels = resolvedStrategyModels(GenerationStrategyDiscoveryParallelCorrection, &store.ResolvedAIGenerationConfig{ModelID: "openai/gpt-5-mini"})
	if strategyModels["discovery"] != "openai/gpt-5-mini" {
		t.Fatalf("blank discovery model should fall back to detail model: %+v", strategyModels)
	}
	if resolvedStrategyModels(GenerationStrategySerial, &store.ResolvedAIGenerationConfig{ModelID: "openai/gpt-5-mini"}) != nil {
		t.Fatal("serial strategy should not emit strategy model snapshot")
	}
	if total := usageRequestsTotalTokens([]ai.UsageRequest{{InputTokens: 2, OutputTokens: 3}}); total != 5 {
		t.Fatalf("usageRequestsTotalTokens = %d, want fallback total 5", total)
	}
}

func TestWorkflowInputFilters(t *testing.T) {
	inputs := workflowInputsWithEpisodes("1", "2", "3")
	after := filterInputsAfter(inputs, "1")
	if len(after.Episodes) != 2 || after.Episodes[0].EpisodeIndex != "2" {
		t.Fatalf("filterInputsAfter = %+v, want episodes 2 and 3", after.Episodes)
	}
	from := filterInputsFrom(inputs, "2")
	if len(from.Episodes) != 2 || from.Episodes[0].EpisodeIndex != "2" {
		t.Fatalf("filterInputsFrom = %+v, want episodes 2 and 3", from.Episodes)
	}
	if compareEpisodeString("10", "2") <= 0 {
		t.Fatal("compareEpisodeString should compare numeric episode strings numerically")
	}
}

func TestWorkflowGenerateAndSaveSkipsUsageWhenExistingSummaryCoversRequest(t *testing.T) {
	processed := "2"
	ports := &workflowFakePorts{
		settings:       ai.SettingsResponse{EffectiveGenerationMode: "openrouter"},
		config:         &store.ResolvedAIGenerationConfig{ProfileID: "profile-a", ModelID: "model-a"},
		processedIndex: &processed,
		hasExisting:    true,
	}

	err := NewWorkflow(ports).GenerateAndSave(context.Background(), "novel-a", "1", nil, "", nil)
	if err != nil {
		t.Fatalf("GenerateAndSave returned error: %v", err)
	}
	if !ports.materialized {
		t.Fatal("existing covered summary should be materialized")
	}
	if len(ports.recordedUsage) != 0 {
		t.Fatalf("recordedUsage length = %d, want 0", len(ports.recordedUsage))
	}
}

func TestWorkflowGenerateAndSaveRecordsFailedUsage(t *testing.T) {
	ports := &workflowFakePorts{
		settings:    ai.SettingsResponse{EffectiveGenerationMode: "openrouter"},
		config:      &store.ResolvedAIGenerationConfig{ProfileID: "profile-a", ModelID: "model-a"},
		inputs:      workflowInputs(),
		generateErr: errors.New("provider failed"),
	}

	err := NewWorkflow(ports).GenerateAndSave(context.Background(), "novel-a", "1", nil, "", nil)
	if err == nil {
		t.Fatal("GenerateAndSave should return provider error")
	}
	if len(ports.recordedUsage) != 1 {
		t.Fatalf("recordedUsage length = %d, want 1", len(ports.recordedUsage))
	}
	run := ports.recordedUsage[0]
	if run.Status != "failed" || run.ErrorMessage == nil || *run.ErrorMessage != "provider failed" {
		t.Fatalf("failed usage run = %+v", run)
	}
}
