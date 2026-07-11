package extraction

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"time"

	"narou-viewer/apps/viewer-api-go/internal/ai"
	"narou-viewer/apps/viewer-api-go/internal/characters"
	core "narou-viewer/apps/viewer-api-go/internal/extraction"
	"narou-viewer/apps/viewer-api-go/internal/extraction/checkpointstore"
	"narou-viewer/apps/viewer-api-go/internal/store"
	"narou-viewer/apps/viewer-api-go/internal/terms"
)

const (
	extractionContractVersion = 2
	CheckpointSchemaVersion   = 4
)

type generationRunner struct {
	ports WorkflowPorts
}

func (r generationRunner) GenerateWithCheckpoint(ctx context.Context, config *store.ResolvedAIGenerationConfig, novelID string, upToEpisodeIndex string, seed []characters.GeneratedCharacter, seedIdentityMergeEvents []characters.GeneratedIdentityMergeEvent, seedTerms []terms.GeneratedTerm, batches []core.Batch, progressSink func(BatchProgress), initialUnresolved []characters.GeneratedUnresolvedMention) ([]characters.GeneratedCharacter, core.GenerationState, []ai.UsageRequest, error) {
	allocator, err := r.ports.LoadIDAllocator(novelID, seed)
	if err != nil {
		return nil, core.GenerationState{}, nil, err
	}
	checkpointFingerprint := CheckpointFingerprint(config, CheckpointGenerationInputs(seed, seedTerms, batches, initialUnresolved, allocator))
	checkpoint := r.loadCheckpointForGeneration(novelID, upToEpisodeIndex, checkpointFingerprint)
	processedBatches := map[int]bool{}
	for _, batchIndex := range checkpoint.ProcessedBatchIndexes {
		processedBatches[batchIndex] = true
	}
	hasCheckpointSnapshot := CheckpointHasSnapshot(checkpoint)
	rawGenerated := append([]characters.GeneratedCharacter{}, seed...)
	identityMergeEvents := core.NormalizeGeneratedIdentityMergeEvents(seedIdentityMergeEvents)
	generated := core.ApplyIdentityMergeEvents(rawGenerated, identityMergeEvents, upToEpisodeIndex)
	generatedTerms := append([]terms.GeneratedTerm{}, seedTerms...)
	if hasCheckpointSnapshot {
		rawGenerated = append([]characters.GeneratedCharacter{}, checkpoint.Characters...)
		identityMergeEvents = append([]characters.GeneratedIdentityMergeEvent{}, checkpoint.IdentityMergeEvents...)
		generated = core.ApplyIdentityMergeEvents(rawGenerated, identityMergeEvents, upToEpisodeIndex)
		generatedTerms = append([]terms.GeneratedTerm{}, checkpoint.Terms...)
	}
	if hasCheckpointSnapshot {
		allocator.ApplyState(checkpoint.NextCharacterOrdinal, checkpoint.IssuedCharacterIDs, checkpoint.RetiredCharacterIDs)
	}
	pendingUnresolved := append([]characters.GeneratedUnresolvedMention{}, initialUnresolved...)
	if hasCheckpointSnapshot {
		pendingUnresolved = append([]characters.GeneratedUnresolvedMention{}, checkpoint.PendingUnresolvedMentions...)
	}
	usageRequests := []ai.UsageRequest{}
	for _, batch := range batches {
		if processedBatches[batch.BatchIndex] || (len(checkpoint.ProcessedBatchIndexes) == 0 && allEpisodeIndexesProcessed(batch.EpisodeIndexes, checkpoint.ProcessedEpisodeIndexes)) {
			continue
		}
		remainingChunks := append([]core.Chunk{}, batch.Chunks...)
		logicalBatchStartedAt := time.Now()
		batchGeneratedCharacters := 0
		batchGeneratedTerms := 0
		for len(remainingChunks) > 0 {
			runtimeBatch, nextRemaining, err := r.ports.PlanRuntimeBatch(ctx, config, novelID, upToEpisodeIndex, generated, generatedTerms, batch, remainingChunks, pendingUnresolved, identityMergeEvents)
			if err != nil {
				return nil, generationStateFromAllocator(pendingUnresolved, allocator), usageRequests, err
			}
			if progressSink != nil {
				progressSink(BatchProgress{Phase: "start", Batch: runtimeBatch})
			}
			promptUnresolved := core.ApplyIdentityMergeEventsToUnresolvedMentions(pendingUnresolved, identityMergeEvents, runtimeBatchBoundary(runtimeBatch))
			result, err := r.ports.GenerateBatch(ctx, config, novelID, upToEpisodeIndex, generated, generatedTerms, runtimeBatch, promptUnresolved)
			if result.Usage.Kind != "" {
				usageRequests = append(usageRequests, result.Usage)
			}
			if err != nil {
				return nil, generationStateFromAllocator(pendingUnresolved, allocator), usageRequests, err
			}
			var changed int
			rawGenerated, changed = core.ApplyDeltaWithoutMergeProposals(novelID, rawGenerated, result.Delta, allocator)
			identityMergeEvents = core.NormalizeGeneratedIdentityMergeEvents(append(identityMergeEvents, core.IdentityMergeEventsFromProposals(result.Delta.MergeProposals, runtimeBatchBoundary(runtimeBatch), rawGenerated)...))
			generated = core.ApplyIdentityMergeEvents(rawGenerated, identityMergeEvents, upToEpisodeIndex)
			generatedTerms = core.FilterAndMergeTermDeltas(generatedTerms, result.Delta.Terms, generated)
			pendingUnresolved = core.FilterResolvedGeneratedUnresolvedMentionsWithIdentityEvents(core.MergeGeneratedUnresolvedMentions(pendingUnresolved, result.Delta.UnresolvedMentions), identityMergeEvents, upToEpisodeIndex, generated)
			remainingChunks = nextRemaining
			batchGeneratedCharacters += changed
			batchGeneratedTerms += len(result.Delta.Terms)
		}
		if progressSink != nil {
			progressSink(BatchProgress{
				Phase:                   "complete",
				Batch:                   batch,
				ElapsedMs:               time.Since(logicalBatchStartedAt).Milliseconds(),
				GeneratedCharacterCount: batchGeneratedCharacters,
				MergedCharacterCount:    len(generated),
				GeneratedTermCount:      batchGeneratedTerms,
				MergedTermCount:         len(generatedTerms),
			})
		}
		checkpoint.ProcessedEpisodeIndexes = mergeStringSets(checkpoint.ProcessedEpisodeIndexes, batch.EpisodeIndexes)
		checkpoint.ProcessedBatchIndexes = appendUniqueInt(checkpoint.ProcessedBatchIndexes, batch.BatchIndex)
		checkpoint.Characters = rawGenerated
		checkpoint.Terms = generatedTerms
		checkpoint.PendingUnresolvedMentions = pendingUnresolved
		checkpoint.IssuedCharacterIDs = allocator.IssuedCharacterIDs()
		checkpoint.RetiredCharacterIDs = allocator.RetiredCharacterIDs()
		checkpoint.IdentityMergeEvents = identityMergeEvents
		checkpoint.NextCharacterOrdinal = allocator.NextCharacterOrdinal()
		checkpoint.SchemaVersion = CheckpointSchemaVersion
		checkpoint.NovelID = novelID
		checkpoint.UpToEpisodeIndex = upToEpisodeIndex
		checkpoint.GenerationFingerprint = checkpointFingerprint
		checkpoint.UpdatedAt = ai.NowISO()
		if err := r.ports.SaveCheckpoint(novelID, upToEpisodeIndex, checkpoint); err != nil {
			return nil, generationStateFromAllocator(pendingUnresolved, allocator), usageRequests, err
		}
	}
	state := generationStateFromAllocator(pendingUnresolved, allocator)
	state.Terms = generatedTerms
	state.IdentityMergeEvents = identityMergeEvents
	state.PersistenceCharacters = rawGenerated
	return generated, state, usageRequests, nil
}

func (r generationRunner) GeneratePreview(ctx context.Context, config *store.ResolvedAIGenerationConfig, novelID string, upToEpisodeIndex string, seed []characters.GeneratedCharacter, seedIdentityMergeEvents []characters.GeneratedIdentityMergeEvent, seedTerms []terms.GeneratedTerm, batches []core.Batch, progressSink func(BatchProgress), initialUnresolved []characters.GeneratedUnresolvedMention) ([]characters.GeneratedCharacter, core.GenerationState, []ai.UsageRequest, error) {
	rawGenerated := append([]characters.GeneratedCharacter{}, seed...)
	identityMergeEvents := core.NormalizeGeneratedIdentityMergeEvents(seedIdentityMergeEvents)
	generated := core.ApplyIdentityMergeEvents(rawGenerated, identityMergeEvents, upToEpisodeIndex)
	generatedTerms := append([]terms.GeneratedTerm{}, seedTerms...)
	allocator, err := r.ports.LoadIDAllocator(novelID, generated)
	if err != nil {
		return nil, core.GenerationState{}, nil, err
	}
	pendingUnresolved := append([]characters.GeneratedUnresolvedMention{}, initialUnresolved...)
	usageRequests := []ai.UsageRequest{}
	for _, batch := range batches {
		remainingChunks := append([]core.Chunk{}, batch.Chunks...)
		logicalBatchStartedAt := time.Now()
		batchGeneratedCharacters := 0
		batchGeneratedTerms := 0
		for len(remainingChunks) > 0 {
			runtimeBatch, nextRemaining, err := r.ports.PlanRuntimeBatch(ctx, config, novelID, upToEpisodeIndex, generated, generatedTerms, batch, remainingChunks, pendingUnresolved, identityMergeEvents)
			if err != nil {
				return nil, generationStateFromAllocator(pendingUnresolved, allocator), usageRequests, err
			}
			if progressSink != nil {
				progressSink(BatchProgress{Phase: "start", Batch: runtimeBatch})
			}
			promptUnresolved := core.ApplyIdentityMergeEventsToUnresolvedMentions(pendingUnresolved, identityMergeEvents, runtimeBatchBoundary(runtimeBatch))
			result, err := r.ports.GenerateBatch(ctx, config, novelID, upToEpisodeIndex, generated, generatedTerms, runtimeBatch, promptUnresolved)
			if result.Usage.Kind != "" {
				usageRequests = append(usageRequests, result.Usage)
			}
			if err != nil {
				return nil, generationStateFromAllocator(pendingUnresolved, allocator), usageRequests, err
			}
			var changed int
			rawGenerated, changed = core.ApplyDeltaWithoutMergeProposals(novelID, rawGenerated, result.Delta, allocator)
			identityMergeEvents = core.NormalizeGeneratedIdentityMergeEvents(append(identityMergeEvents, core.IdentityMergeEventsFromProposals(result.Delta.MergeProposals, runtimeBatchBoundary(runtimeBatch), rawGenerated)...))
			generated = core.ApplyIdentityMergeEvents(rawGenerated, identityMergeEvents, upToEpisodeIndex)
			generatedTerms = core.FilterAndMergeTermDeltas(generatedTerms, result.Delta.Terms, generated)
			pendingUnresolved = core.FilterResolvedGeneratedUnresolvedMentionsWithIdentityEvents(core.MergeGeneratedUnresolvedMentions(pendingUnresolved, result.Delta.UnresolvedMentions), identityMergeEvents, upToEpisodeIndex, generated)
			remainingChunks = nextRemaining
			batchGeneratedCharacters += changed
			batchGeneratedTerms += len(result.Delta.Terms)
		}
		if progressSink != nil {
			progressSink(BatchProgress{
				Phase:                   "complete",
				Batch:                   batch,
				ElapsedMs:               time.Since(logicalBatchStartedAt).Milliseconds(),
				GeneratedCharacterCount: batchGeneratedCharacters,
				MergedCharacterCount:    len(generated),
				GeneratedTermCount:      batchGeneratedTerms,
				MergedTermCount:         len(generatedTerms),
			})
		}
	}
	state := generationStateFromAllocator(pendingUnresolved, allocator)
	state.Terms = generatedTerms
	state.IdentityMergeEvents = identityMergeEvents
	state.PersistenceCharacters = rawGenerated
	return generated, state, usageRequests, nil
}

func (r generationRunner) loadCheckpointForGeneration(novelID string, upToEpisodeIndex string, expectedFingerprint string) checkpointstore.Checkpoint {
	checkpoint, err := r.ports.LoadCheckpoint(novelID, upToEpisodeIndex)
	if err != nil ||
		checkpoint.SchemaVersion != CheckpointSchemaVersion ||
		checkpoint.NovelID != novelID ||
		checkpoint.UpToEpisodeIndex != upToEpisodeIndex ||
		(expectedFingerprint != "" && checkpoint.GenerationFingerprint != expectedFingerprint) {
		return EmptyCheckpoint(novelID, upToEpisodeIndex, expectedFingerprint)
	}
	return NormalizeCheckpoint(checkpoint)
}

func EmptyCheckpoint(novelID string, upToEpisodeIndex string, fingerprint string) checkpointstore.Checkpoint {
	return checkpointstore.Checkpoint{
		SchemaVersion:         CheckpointSchemaVersion,
		NovelID:               novelID,
		UpToEpisodeIndex:      upToEpisodeIndex,
		GenerationFingerprint: fingerprint,
		Characters:            []characters.GeneratedCharacter{},
		Terms:                 []terms.GeneratedTerm{},
	}
}

func NormalizeCheckpoint(checkpoint checkpointstore.Checkpoint) checkpointstore.Checkpoint {
	if checkpoint.Characters == nil {
		checkpoint.Characters = []characters.GeneratedCharacter{}
	}
	if checkpoint.Terms == nil {
		checkpoint.Terms = []terms.GeneratedTerm{}
	}
	if checkpoint.PendingUnresolvedMentions == nil {
		checkpoint.PendingUnresolvedMentions = []characters.GeneratedUnresolvedMention{}
	}
	if checkpoint.IssuedCharacterIDs == nil {
		checkpoint.IssuedCharacterIDs = []string{}
	}
	if checkpoint.RetiredCharacterIDs == nil {
		checkpoint.RetiredCharacterIDs = []characters.GeneratedRetiredCharacterID{}
	}
	if checkpoint.IdentityMergeEvents == nil {
		checkpoint.IdentityMergeEvents = []characters.GeneratedIdentityMergeEvent{}
	}
	return checkpoint
}

func runtimeBatchBoundary(batch core.Batch) string {
	boundary := ""
	for _, episodeIndex := range batch.EpisodeIndexes {
		if boundary == "" || compareEpisodeString(episodeIndex, boundary) > 0 {
			boundary = episodeIndex
		}
	}
	for _, chunk := range batch.Chunks {
		if boundary == "" || compareEpisodeString(chunk.EpisodeIndex, boundary) > 0 {
			boundary = chunk.EpisodeIndex
		}
	}
	return boundary
}

func CheckpointHasSnapshot(checkpoint checkpointstore.Checkpoint) bool {
	return len(checkpoint.ProcessedBatchIndexes) > 0 ||
		len(checkpoint.ProcessedEpisodeIndexes) > 0 ||
		len(checkpoint.Characters) > 0 ||
		len(checkpoint.IdentityMergeEvents) > 0 ||
		len(checkpoint.Terms) > 0 ||
		len(checkpoint.PendingUnresolvedMentions) > 0
}

func CheckpointFingerprint(config *store.ResolvedAIGenerationConfig, extra any) string {
	input := struct {
		ProfileID         string   `json:"profileId"`
		ModelID           string   `json:"modelId"`
		ProviderOrder     []string `json:"providerOrder"`
		AllowFallbacks    bool     `json:"allowFallbacks"`
		RequireParameters bool     `json:"requireParameters"`
		SystemPrompt      string   `json:"systemPrompt"`
		ContractVersion   int      `json:"contractVersion"`
		Extra             any      `json:"extra"`
	}{
		Extra: extra, ContractVersion: extractionContractVersion,
	}
	if config != nil {
		input.ProfileID = config.ProfileID
		input.ModelID = config.ModelID
		input.ProviderOrder = append([]string{}, config.ProviderOrder...)
		input.AllowFallbacks = config.AllowFallbacks
		input.RequireParameters = config.RequireParameters
		if config.SystemPrompt != nil {
			input.SystemPrompt = *config.SystemPrompt
		}
	}
	raw, err := json.Marshal(input)
	if err != nil {
		raw = []byte("")
	}
	sum := sha1.Sum(raw)
	return hex.EncodeToString(sum[:])
}

func CheckpointBatchInputs(batches []core.Batch) []map[string]any {
	inputs := make([]map[string]any, 0, len(batches))
	for _, batch := range batches {
		inputs = append(inputs, CheckpointBatchInput(batch))
	}
	return inputs
}

func CheckpointGenerationInputs(seed []characters.GeneratedCharacter, seedTerms []terms.GeneratedTerm, batches []core.Batch, initialUnresolved []characters.GeneratedUnresolvedMention, allocator *characters.GeneratedCharacterIDAllocator) map[string]any {
	input := map[string]any{
		"batches":            CheckpointBatchInputs(batches),
		"seedCharacters":     seed,
		"seedTerms":          seedTerms,
		"unresolvedMentions": initialUnresolved,
	}
	if allocator != nil {
		input["nextCharacterOrdinal"] = allocator.NextCharacterOrdinal()
		input["issuedCharacterIds"] = allocator.IssuedCharacterIDs()
		input["retiredCharacterIds"] = allocator.RetiredCharacterIDs()
	}
	return input
}

func CheckpointBatchInput(batch core.Batch) map[string]any {
	chunkHashes := make([]string, 0, len(batch.Chunks))
	for _, chunk := range batch.Chunks {
		sum := sha1.Sum([]byte(chunk.EpisodeIndex + "\x00" + chunk.Title + "\x00" + chunk.Text))
		chunkHashes = append(chunkHashes, hex.EncodeToString(sum[:]))
	}
	return map[string]any{
		"batchIndex":     batch.BatchIndex,
		"episodeIndexes": append([]string{}, batch.EpisodeIndexes...),
		"chunkHashes":    chunkHashes,
	}
}

func generationStateFromAllocator(unresolved []characters.GeneratedUnresolvedMention, allocator *characters.GeneratedCharacterIDAllocator) core.GenerationState {
	state := core.GenerationState{
		UnresolvedMentions: append([]characters.GeneratedUnresolvedMention{}, unresolved...),
	}
	if allocator != nil {
		state.IssuedCharacterIDs = allocator.IssuedCharacterIDs()
		state.RetiredCharacterIDs = allocator.RetiredCharacterIDs()
		state.NextOrdinal = allocator.NextCharacterOrdinal()
	}
	return state
}

func allEpisodeIndexesProcessed(episodeIndexes []string, processed []string) bool {
	if len(episodeIndexes) == 0 || len(processed) == 0 {
		return false
	}
	seen := map[string]bool{}
	for _, value := range processed {
		seen[value] = true
	}
	for _, value := range episodeIndexes {
		if !seen[value] {
			return false
		}
	}
	return true
}

func mergeStringSets(existing []string, incoming []string) []string {
	result := append([]string{}, existing...)
	seen := map[string]bool{}
	for _, value := range result {
		seen[value] = true
	}
	for _, value := range incoming {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func appendUniqueInt(existing []int, value int) []int {
	for _, current := range existing {
		if current == value {
			return existing
		}
	}
	return append(existing, value)
}
