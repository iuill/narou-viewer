package extraction

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"narou-viewer/apps/viewer-api-go/internal/ai"
	"narou-viewer/apps/viewer-api-go/internal/ai/snapshotcontracttest"
	"narou-viewer/apps/viewer-api-go/internal/characters"
	core "narou-viewer/apps/viewer-api-go/internal/extraction"
	"narou-viewer/apps/viewer-api-go/internal/extraction/checkpointstore"
	"narou-viewer/apps/viewer-api-go/internal/store"
	"narou-viewer/apps/viewer-api-go/internal/terms"
)

type workflowFakePorts struct {
	settings       ai.SettingsResponse
	config         *store.ResolvedAIGenerationConfig
	inputs         Inputs
	rebatchOutput  *Inputs
	loadErr        error
	seed           []characters.GeneratedCharacter
	seedBefore     []characters.GeneratedCharacter
	seedBeforeSet  bool
	seedEvents     []characters.GeneratedIdentityMergeEvent
	seedTerms      []terms.GeneratedTerm
	processedIndex *string
	hasExisting    bool
	reprocessFrom  string

	locked                 bool
	materialized           bool
	saveHeuristic          bool
	saveGenerated          bool
	saveTerms              bool
	removedCheckpoint      bool
	builtGenerated         bool
	builtPreviewCharacters []characters.GeneratedCharacter
	loadedPreview          bool
	recordedUsage          []ai.UsageRun
	generateErr            error
	saveGeneratedErr       error
	saveTermsErr           error
	generateErrAfter       int
	parallelCalls          int
	planErr                error
	allocatorErr           error
	generateCalls          int
	runtimeBatchChunkLimit int
	generateBatchResults   []BatchResult
	rebatchCalls           int
	heuristicEpisodes      []characters.HeuristicEpisode
	generatedEpisodes      []characters.HeuristicEpisode
	checkpoint             checkpointstore.Checkpoint
	checkpointQuarantined  bool
	checkpointReason       string
	savedCheckpoint        bool
	savedCharacters        []characters.GeneratedCharacter
	savedSummaryOptions    characters.SaveGeneratedSummaryOptions
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

func (p *workflowFakePorts) LoadGenerationSeed(string, string) ([]characters.GeneratedCharacter, []characters.GeneratedIdentityMergeEvent, *string, bool, error) {
	return p.seed, p.seedEvents, p.processedIndex, p.hasExisting, nil
}

func (p *workflowFakePorts) LoadGeneratedCharactersBeforeEpisode(string, string) ([]characters.GeneratedCharacter, []characters.GeneratedIdentityMergeEvent, *string, bool, error) {
	if p.seedBeforeSet {
		return p.seedBefore, nil, nil, true, nil
	}
	return p.seed, p.seedEvents, p.processedIndex, p.hasExisting, nil
}

func (p *workflowFakePorts) LoadGeneratedTermsAtOrBefore(string, string) ([]terms.GeneratedTerm, *string, bool, error) {
	return p.seedTerms, p.processedIndex, p.hasExisting || p.processedIndex != nil, nil
}

func (p *workflowFakePorts) LoadGeneratedTermsBeforeEpisode(string, string) ([]terms.GeneratedTerm, *string, bool, error) {
	return p.seedTerms, p.processedIndex, p.hasExisting || p.processedIndex != nil, nil
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

func (p *workflowFakePorts) PlanRuntimeBatch(_ context.Context, _ *store.ResolvedAIGenerationConfig, _ string, _ string, _ []characters.GeneratedCharacter, _ []terms.GeneratedTerm, template core.Batch, chunks []core.Chunk, _ []characters.GeneratedUnresolvedMention, _ []characters.GeneratedIdentityMergeEvent) (core.Batch, []core.Chunk, error) {
	if p.planErr != nil {
		return core.Batch{}, nil, p.planErr
	}
	if p.runtimeBatchChunkLimit > 0 && len(chunks) > p.runtimeBatchChunkLimit {
		return core.RuntimeBatch(template, chunks[:p.runtimeBatchChunkLimit]), append([]core.Chunk{}, chunks[p.runtimeBatchChunkLimit:]...), nil
	}
	return core.RuntimeBatch(template, chunks), nil, nil
}

func (p *workflowFakePorts) GenerateBatch(context.Context, *store.ResolvedAIGenerationConfig, string, string, []characters.GeneratedCharacter, []terms.GeneratedTerm, core.Batch, []characters.GeneratedUnresolvedMention) (BatchResult, error) {
	if p.generateErr != nil && (p.generateErrAfter == 0 || p.generateCalls >= p.generateErrAfter) {
		return BatchResult{Usage: ai.UsageRequest{RequestIndex: 0, Kind: "extraction_batch", InputTokens: 10, OutputTokens: 3, TotalTokens: 13}}, p.generateErr
	}
	if p.generateCalls < len(p.generateBatchResults) {
		result := p.generateBatchResults[p.generateCalls]
		p.generateCalls++
		return result, nil
	}
	p.generateCalls++
	return BatchResult{
		Delta: core.Delta{NewCharacters: []characters.GeneratedCharacter{{CanonicalName: "Alice", CanonicalEpisodeIndex: "1", SummaryHistory: []characters.GeneratedHistoryVersion{{EpisodeIndex: "1", Text: "本文"}}}}},
		Usage: ai.UsageRequest{RequestIndex: 0, Kind: "extraction_batch", InputTokens: 10, OutputTokens: 3, TotalTokens: 13},
	}, nil
}

func (p *workflowFakePorts) GenerateParallelIdentity(_ context.Context, _ *store.ResolvedAIGenerationConfig, _ string, _ string, seed []characters.GeneratedCharacter, _ []characters.GeneratedIdentityMergeEvent, seedTerms []terms.GeneratedTerm, _ []core.Batch, _ func(BatchProgress), _ []characters.GeneratedUnresolvedMention) ([]characters.GeneratedCharacter, core.GenerationState, []ai.UsageRequest, error) {
	p.parallelCalls++
	if p.generateErr != nil {
		return nil, core.GenerationState{}, nil, p.generateErr
	}
	generated := append([]characters.GeneratedCharacter{}, seed...)
	generated = append(generated, characters.GeneratedCharacter{CharacterID: "char_parallel", CanonicalName: "Parallel", CanonicalEpisodeIndex: "1", FirstAppearanceEpisodeIndex: "1"})
	return generated, core.GenerationState{Terms: seedTerms}, []ai.UsageRequest{{RequestIndex: 0, Kind: "extraction_parallel_identity", InputTokens: 12, OutputTokens: 4, TotalTokens: 16}}, nil
}

func (p *workflowFakePorts) GenerateDiscoveryParallelCorrection(_ context.Context, _ *store.ResolvedAIGenerationConfig, _ string, _ string, seed []characters.GeneratedCharacter, _ []characters.GeneratedIdentityMergeEvent, seedTerms []terms.GeneratedTerm, _ []core.Batch, _ func(BatchProgress), _ []characters.GeneratedUnresolvedMention) ([]characters.GeneratedCharacter, core.GenerationState, []ai.UsageRequest, error) {
	p.parallelCalls++
	if p.generateErr != nil {
		return nil, core.GenerationState{}, nil, p.generateErr
	}
	generated := append([]characters.GeneratedCharacter{}, seed...)
	generated = append(generated, characters.GeneratedCharacter{CharacterID: "char_discovery", CanonicalName: "Discovery", CanonicalEpisodeIndex: "1", FirstAppearanceEpisodeIndex: "1"})
	return generated, core.GenerationState{Terms: seedTerms}, []ai.UsageRequest{{RequestIndex: 0, Kind: "extraction_discovery_parallel_correction", InputTokens: 16, OutputTokens: 5, TotalTokens: 21}}, nil
}

func (p *workflowFakePorts) LoadCheckpoint(string, string) (checkpointstore.Checkpoint, error) {
	if p.checkpoint.NovelID == "" {
		return checkpointstore.Checkpoint{}, os.ErrNotExist
	}
	return p.checkpoint, nil
}

func (p *workflowFakePorts) QuarantineCheckpoint(_ string, _ string, reason string, cause error) error {
	p.checkpointQuarantined = true
	p.checkpointReason = reason
	return &checkpointstore.IncompatibleError{Path: "checkpoint", QuarantinedPath: "checkpoint.unsupported", Reason: reason, Err: cause}
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

func (p *workflowFakePorts) SaveGeneratedSummary(_ string, _ string, generated []characters.GeneratedCharacter, episodes []characters.HeuristicEpisode, options characters.SaveGeneratedSummaryOptions) error {
	p.saveGenerated = true
	p.savedCharacters = append([]characters.GeneratedCharacter{}, generated...)
	p.savedSummaryOptions = options
	p.generatedEpisodes = append([]characters.HeuristicEpisode{}, episodes...)
	return p.saveGeneratedErr
}

func (p *workflowFakePorts) SaveGeneratedTerms(_ string, _ string, generated []terms.GeneratedTerm, _ string) error {
	p.saveTerms = true
	p.seedTerms = append([]terms.GeneratedTerm{}, generated...)
	return p.saveTermsErr
}

func (p *workflowFakePorts) BuildGeneratedPreview(_ string, _ string, generated []characters.GeneratedCharacter, episodes []characters.HeuristicEpisode, _ []string, _ characters.SaveGeneratedSummaryOptions) (characters.SummaryResponse, error) {
	p.builtGenerated = true
	p.builtPreviewCharacters = append([]characters.GeneratedCharacter{}, generated...)
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

	_, err := NewWorkflow(ports).GenerateAndSave(context.Background(), "novel-a", "1", nil, "", nil)
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

func TestUsageRecorderReindexesRuntimeRequestsBeforePersistence(t *testing.T) {
	recorder := newUsageRecorder(context.Background(), nil, "test", "novel-a", "2")
	recorder.UseActualRequests([]ai.UsageRequest{
		{RequestIndex: 0, Kind: "extraction_batch", TotalTokens: 10},
		{RequestIndex: 0, Kind: "extraction_batch", TotalTokens: 12},
	})
	if len(recorder.Requests) != 2 || recorder.Requests[0].RequestIndex != 0 || recorder.Requests[1].RequestIndex != 1 {
		t.Fatalf("runtime requests were not reindexed: %+v", recorder.Requests)
	}
	dbPath := filepath.Join(t.TempDir(), "ai_usage.sqlite")
	if err := ai.SaveUsageRun(dbPath, ai.UsageRun{
		RunID:        "run-split",
		Feature:      "extraction",
		WorkflowName: "extraction",
		Status:       "failed",
		StartedAt:    "2026-01-01T00:00:00Z",
		FinishedAt:   "2026-01-01T00:00:01Z",
		RequestCount: len(recorder.Requests),
		TotalTokens:  22,
		Requests:     recorder.Requests,
	}); err != nil {
		t.Fatalf("SaveUsageRun returned error: %v", err)
	}
	loaded, ok, err := ai.LoadUsageRun(dbPath, "run-split")
	if err != nil || !ok {
		t.Fatalf("LoadUsageRun failed: ok=%v err=%v", ok, err)
	}
	if len(loaded.Requests) != 2 || loaded.Requests[0].RequestIndex != 0 || loaded.Requests[1].RequestIndex != 1 {
		t.Fatalf("persisted requests = %+v, want indexes 0,1", loaded.Requests)
	}
}

func TestWorkflowGenerateAndSaveParallelIdentityUsesStrategy(t *testing.T) {
	ports := &workflowFakePorts{
		settings: ai.SettingsResponse{EffectiveGenerationMode: "openrouter"},
		config:   &store.ResolvedAIGenerationConfig{ProfileID: "profile-a", ProfileLabel: "Profile A", ModelID: "model-a"},
		inputs:   workflowInputs(),
	}

	_, err := NewWorkflow(ports).GenerateAndSave(context.Background(), "novel-a", "1", nil, GenerationStrategyParallelIdentity, nil)
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
	snapshotcontracttest.AssertSafeProducerSnapshot(t, snapshot, 1000)
	if ports.recordedUsage[0].ErrorMessage != nil {
		t.Fatal("completed extraction usage unexpectedly recorded an error message")
	}
}

func TestWorkflowGenerateAndSaveHeuristicSavesWithoutUsage(t *testing.T) {
	ports := &workflowFakePorts{
		settings: ai.SettingsResponse{EffectiveGenerationMode: "heuristic"},
		inputs:   workflowInputs(),
	}

	_, err := NewWorkflow(ports).GenerateAndSave(context.Background(), "novel-a", "1", nil, "", nil)
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
	if summary.GenerationMode != "openrouter" || !ports.builtGenerated {
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
	if summary.GenerationMode != "openrouter" || !ports.materialized || !ports.loadedPreview {
		t.Fatalf("summary=%+v materialized=%v loadedPreview=%v, want loaded preview", summary, ports.materialized, ports.loadedPreview)
	}
	if len(ports.recordedUsage) != 0 {
		t.Fatalf("recordedUsage length = %d, want 0", len(ports.recordedUsage))
	}
}

func TestWorkflowPreviewReusesIDFromMergedIdentityRegistry(t *testing.T) {
	processed := "20"
	ports := &workflowFakePorts{
		settings:       ai.SettingsResponse{EffectiveGenerationMode: "openrouter"},
		config:         &store.ResolvedAIGenerationConfig{ProfileID: "profile-a", ModelID: "model-a"},
		processedIndex: &processed,
		hasExisting:    true,
		reprocessFrom:  "21",
		seedBeforeSet:  true,
		inputs:         workflowInputsWithEpisodes("21"),
		seed: []characters.GeneratedCharacter{
			{CharacterID: "char_black_knight", CanonicalName: "Alice", CanonicalEpisodeIndex: "1", FirstAppearanceEpisodeIndex: "1", NameHistory: []characters.GeneratedTextVersion{{Text: "Alice", EpisodeIndex: "1"}}},
			{CharacterID: "char_alice", CanonicalName: "Alice", CanonicalEpisodeIndex: "2", FirstAppearanceEpisodeIndex: "2", NameHistory: []characters.GeneratedTextVersion{{Text: "Alice", EpisodeIndex: "2"}}},
		},
		seedEvents: []characters.GeneratedIdentityMergeEvent{{SourceCharacterID: "char_black_knight", TargetCharacterID: "char_alice", EffectiveEpisodeIndex: "20"}},
	}
	if _, err := NewWorkflow(ports).GeneratePreview(context.Background(), "novel-a", "21", nil, "serial", nil, []string{"21"}, nil); err != nil {
		t.Fatalf("GeneratePreview returned error: %v", err)
	}
	found := false
	for _, character := range ports.builtPreviewCharacters {
		if character.CanonicalName == "Alice" {
			found = true
			if character.CharacterID != "char_alice" {
				t.Fatalf("preview should reuse merged target ID: %+v", character)
			}
		}
	}
	if !found {
		t.Fatalf("generated preview character was not captured: %+v", ports.builtPreviewCharacters)
	}
}

func TestWorkflowGeneratePreviewHeuristicUsesPreloadedInputs(t *testing.T) {
	ports := &workflowFakePorts{settings: ai.SettingsResponse{EffectiveGenerationMode: "heuristic"}}
	preloaded := workflowInputsWithEpisodes("1", "2")

	summary, err := NewWorkflow(ports).GeneratePreview(context.Background(), "novel-a", "2", nil, "", nil, []string{"1", "2"}, &preloaded)
	if err != nil {
		t.Fatalf("GeneratePreview returned error: %v", err)
	}
	if summary.GenerationMode != "heuristic" {
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

func TestWorkflowReprocessRemapsPersistenceToReusedRegistryID(t *testing.T) {
	processed := "3"
	ports := &workflowFakePorts{
		settings:       ai.SettingsResponse{EffectiveGenerationMode: "openrouter"},
		config:         &store.ResolvedAIGenerationConfig{ProfileID: "profile-a", ProfileLabel: "Profile A", ModelID: "model-a"},
		seed:           []characters.GeneratedCharacter{{CharacterID: "char_old", CanonicalName: "アリス", CanonicalEpisodeIndex: "2", FirstAppearanceEpisodeIndex: "2"}},
		seedBeforeSet:  true,
		processedIndex: &processed,
		hasExisting:    true,
		reprocessFrom:  "2",
		inputs:         workflowInputsWithEpisodes("2"),
		generateBatchResults: []BatchResult{{
			Delta: core.Delta{NewCharacters: []characters.GeneratedCharacter{{CharacterID: "char_new", CanonicalName: "アリス", CanonicalEpisodeIndex: "2", FirstAppearanceEpisodeIndex: "2"}}},
			Usage: ai.UsageRequest{Kind: "extraction_batch", InputTokens: 10, OutputTokens: 3, TotalTokens: 13},
		}},
	}

	counts, err := NewWorkflow(ports).GenerateAndSave(context.Background(), "novel-a", "3", nil, GenerationStrategySerial, nil)
	if err != nil {
		t.Fatalf("GenerateAndSave returned error: %v", err)
	}
	if counts.CharacterCount != 1 || len(ports.savedCharacters) != 1 || ports.savedCharacters[0].CharacterID != "char_old" {
		t.Fatalf("saved reprocess result did not reuse registry ID: counts=%+v saved=%+v", counts, ports.savedCharacters)
	}
	retired := ports.savedSummaryOptions.RetiredCharacterIDs
	if len(retired) != 1 || retired[0].CharacterID != "char_new" || retired[0].MergedInto != "char_old" {
		t.Fatalf("reprocess allocator state did not preserve remap: %+v", ports.savedSummaryOptions)
	}
}

func TestWorkflowOpenRouterMissingProfileRecordsFailedUsage(t *testing.T) {
	ports := &workflowFakePorts{
		settings: ai.SettingsResponse{EffectiveGenerationMode: "openrouter"},
		inputs:   workflowInputs(),
	}

	_, err := NewWorkflow(ports).GenerateAndSave(context.Background(), "novel-a", "1", nil, "", nil)
	if err == nil {
		t.Fatal("GenerateAndSave should fail when profile is missing")
	}
	if len(ports.recordedUsage) != 1 || ports.recordedUsage[0].Status != "failed" {
		t.Fatalf("recordedUsage = %+v, want failed usage", ports.recordedUsage)
	}
}

func TestWorkflowDisabledModeReturnsUnavailableError(t *testing.T) {
	ports := &workflowFakePorts{settings: ai.SettingsResponse{EffectiveGenerationMode: "disabled"}}

	if _, err := NewWorkflow(ports).GenerateAndSave(context.Background(), "novel-a", "1", nil, "", nil); err == nil {
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
		ModelID:                        "openai/gpt-5-mini",
		ExtractionNameDiscoveryModelID: "openai/gpt-5-nano",
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

	_, err := NewWorkflow(ports).GenerateAndSave(context.Background(), "novel-a", "1", nil, "", nil)
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

	_, err := NewWorkflow(ports).GenerateAndSave(context.Background(), "novel-a", "1", nil, "", nil)
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

func TestLegacyExtractionStateErrorPromptsUserToClearData(t *testing.T) {
	want := "旧生成データには用語が含まれないため、抽出データをクリアして再生成してください。"
	if ErrLegacyExtractionStateIncomplete.Error() != want {
		t.Fatalf("legacy extraction error = %q, want %q", ErrLegacyExtractionStateIncomplete, want)
	}
}

func TestWorkflowTermsSaveFailureStopsBeforeCharacterSaveAndKeepsCheckpoint(t *testing.T) {
	ports := &workflowFakePorts{
		settings:     ai.SettingsResponse{EffectiveGenerationMode: "openrouter"},
		config:       &store.ResolvedAIGenerationConfig{ProfileID: "profile-a", ModelID: "model-a"},
		inputs:       workflowInputs(),
		saveTermsErr: errors.New("term save failed"),
	}

	_, err := NewWorkflow(ports).GenerateAndSave(context.Background(), "novel-a", "1", nil, "", nil)
	if !errors.Is(err, ports.saveTermsErr) {
		t.Fatalf("GenerateAndSave error = %v, want term save failure", err)
	}
	if !ports.saveTerms || ports.saveGenerated || ports.removedCheckpoint {
		t.Fatalf("term failure must stop character save and retain checkpoint: %+v", ports)
	}
}

func TestWorkflowCharacterSaveFailureKeepsCheckpointAfterTermsSave(t *testing.T) {
	ports := &workflowFakePorts{
		settings:         ai.SettingsResponse{EffectiveGenerationMode: "openrouter"},
		config:           &store.ResolvedAIGenerationConfig{ProfileID: "profile-a", ModelID: "model-a"},
		inputs:           workflowInputs(),
		saveGeneratedErr: errors.New("character save failed"),
	}

	_, err := NewWorkflow(ports).GenerateAndSave(context.Background(), "novel-a", "1", nil, "", nil)
	if !errors.Is(err, ports.saveGeneratedErr) {
		t.Fatalf("GenerateAndSave error = %v, want character save failure", err)
	}
	if !ports.saveTerms || !ports.saveGenerated || ports.removedCheckpoint {
		t.Fatalf("character failure must occur after terms save and retain checkpoint: %+v", ports)
	}
}
