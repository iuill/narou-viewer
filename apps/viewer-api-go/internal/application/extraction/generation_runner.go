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
)

type generationRunner struct {
	ports WorkflowPorts
}

func (r generationRunner) GenerateWithCheckpoint(ctx context.Context, config *store.ResolvedAIGenerationConfig, novelID string, upToEpisodeIndex string, seed []characters.GeneratedCharacter, batches []core.Batch, progressSink func(BatchProgress), initialUnresolved []characters.GeneratedUnresolvedMention) ([]characters.GeneratedCharacter, core.GenerationState, []ai.UsageRequest, error) {
	checkpointFingerprint := CheckpointFingerprint(config, CheckpointBatchInputs(batches))
	checkpoint := r.loadCheckpointForGeneration(novelID, upToEpisodeIndex, checkpointFingerprint)
	processedBatches := map[int]bool{}
	for _, batchIndex := range checkpoint.ProcessedBatchIndexes {
		processedBatches[batchIndex] = true
	}
	hasCheckpointSnapshot := CheckpointHasSnapshot(checkpoint)
	generated := append([]characters.GeneratedCharacter{}, seed...)
	if hasCheckpointSnapshot {
		generated = append([]characters.GeneratedCharacter{}, checkpoint.Characters...)
	}
	allocator, err := r.ports.LoadIDAllocator(novelID, generated)
	if err != nil {
		return nil, core.GenerationState{}, nil, err
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
		for len(remainingChunks) > 0 {
			runtimeBatch, nextRemaining, err := r.ports.PlanRuntimeBatch(ctx, config, novelID, upToEpisodeIndex, generated, batch, remainingChunks, pendingUnresolved)
			if err != nil {
				return nil, generationStateFromAllocator(pendingUnresolved, allocator), usageRequests, err
			}
			batchStartedAt := time.Now()
			if progressSink != nil {
				progressSink(BatchProgress{Phase: "start", Batch: runtimeBatch})
			}
			result, err := r.ports.GenerateBatch(ctx, config, novelID, upToEpisodeIndex, generated, runtimeBatch, pendingUnresolved)
			if err != nil {
				return nil, generationStateFromAllocator(pendingUnresolved, allocator), usageRequests, err
			}
			usageRequests = append(usageRequests, result.Usage)
			var changed int
			generated, changed = core.ApplyDelta(novelID, generated, result.Delta, allocator)
			pendingUnresolved = core.FilterResolvedGeneratedUnresolvedMentions(core.MergeGeneratedUnresolvedMentions(pendingUnresolved, result.Delta.UnresolvedMentions), generated)
			remainingChunks = nextRemaining
			if progressSink != nil {
				progressSink(BatchProgress{
					Phase:                   "complete",
					Batch:                   runtimeBatch,
					ElapsedMs:               time.Since(batchStartedAt).Milliseconds(),
					GeneratedCharacterCount: changed,
					MergedCharacterCount:    len(generated),
				})
			}
		}
		checkpoint.ProcessedEpisodeIndexes = mergeStringSets(checkpoint.ProcessedEpisodeIndexes, batch.EpisodeIndexes)
		checkpoint.ProcessedBatchIndexes = appendUniqueInt(checkpoint.ProcessedBatchIndexes, batch.BatchIndex)
		checkpoint.Characters = generated
		checkpoint.PendingUnresolvedMentions = pendingUnresolved
		checkpoint.IssuedCharacterIDs = allocator.IssuedCharacterIDs()
		checkpoint.RetiredCharacterIDs = allocator.RetiredCharacterIDs()
		checkpoint.NextCharacterOrdinal = allocator.NextCharacterOrdinal()
		checkpoint.SchemaVersion = 1
		checkpoint.NovelID = novelID
		checkpoint.UpToEpisodeIndex = upToEpisodeIndex
		checkpoint.GenerationFingerprint = checkpointFingerprint
		checkpoint.UpdatedAt = ai.NowISO()
		if err := r.ports.SaveCheckpoint(novelID, upToEpisodeIndex, checkpoint); err != nil {
			return nil, generationStateFromAllocator(pendingUnresolved, allocator), usageRequests, err
		}
	}
	return generated, generationStateFromAllocator(pendingUnresolved, allocator), usageRequests, nil
}

func (r generationRunner) GeneratePreview(ctx context.Context, config *store.ResolvedAIGenerationConfig, novelID string, upToEpisodeIndex string, seed []characters.GeneratedCharacter, batches []core.Batch, progressSink func(BatchProgress), initialUnresolved []characters.GeneratedUnresolvedMention) ([]characters.GeneratedCharacter, core.GenerationState, []ai.UsageRequest, error) {
	generated := append([]characters.GeneratedCharacter{}, seed...)
	allocator, err := r.ports.LoadIDAllocator(novelID, generated)
	if err != nil {
		return nil, core.GenerationState{}, nil, err
	}
	pendingUnresolved := append([]characters.GeneratedUnresolvedMention{}, initialUnresolved...)
	usageRequests := []ai.UsageRequest{}
	for _, batch := range batches {
		remainingChunks := append([]core.Chunk{}, batch.Chunks...)
		for len(remainingChunks) > 0 {
			runtimeBatch, nextRemaining, err := r.ports.PlanRuntimeBatch(ctx, config, novelID, upToEpisodeIndex, generated, batch, remainingChunks, pendingUnresolved)
			if err != nil {
				return nil, generationStateFromAllocator(pendingUnresolved, allocator), usageRequests, err
			}
			batchStartedAt := time.Now()
			if progressSink != nil {
				progressSink(BatchProgress{Phase: "start", Batch: runtimeBatch})
			}
			result, err := r.ports.GenerateBatch(ctx, config, novelID, upToEpisodeIndex, generated, runtimeBatch, pendingUnresolved)
			if err != nil {
				return nil, generationStateFromAllocator(pendingUnresolved, allocator), usageRequests, err
			}
			usageRequests = append(usageRequests, result.Usage)
			var changed int
			generated, changed = core.ApplyDelta(novelID, generated, result.Delta, allocator)
			pendingUnresolved = core.FilterResolvedGeneratedUnresolvedMentions(core.MergeGeneratedUnresolvedMentions(pendingUnresolved, result.Delta.UnresolvedMentions), generated)
			remainingChunks = nextRemaining
			if progressSink != nil {
				progressSink(BatchProgress{
					Phase:                   "complete",
					Batch:                   runtimeBatch,
					ElapsedMs:               time.Since(batchStartedAt).Milliseconds(),
					GeneratedCharacterCount: changed,
					MergedCharacterCount:    len(generated),
				})
			}
		}
	}
	return generated, generationStateFromAllocator(pendingUnresolved, allocator), usageRequests, nil
}

func (r generationRunner) loadCheckpointForGeneration(novelID string, upToEpisodeIndex string, expectedFingerprint string) checkpointstore.Checkpoint {
	checkpoint, err := r.ports.LoadCheckpoint(novelID, upToEpisodeIndex)
	if err != nil ||
		checkpoint.SchemaVersion != 1 ||
		checkpoint.NovelID != novelID ||
		checkpoint.UpToEpisodeIndex != upToEpisodeIndex ||
		(expectedFingerprint != "" && checkpoint.GenerationFingerprint != expectedFingerprint) {
		return EmptyCheckpoint(novelID, upToEpisodeIndex, expectedFingerprint)
	}
	return NormalizeCheckpoint(checkpoint)
}

func EmptyCheckpoint(novelID string, upToEpisodeIndex string, fingerprint string) checkpointstore.Checkpoint {
	return checkpointstore.Checkpoint{
		SchemaVersion:         1,
		NovelID:               novelID,
		UpToEpisodeIndex:      upToEpisodeIndex,
		GenerationFingerprint: fingerprint,
		Characters:            []characters.GeneratedCharacter{},
	}
}

func NormalizeCheckpoint(checkpoint checkpointstore.Checkpoint) checkpointstore.Checkpoint {
	if checkpoint.Characters == nil {
		checkpoint.Characters = []characters.GeneratedCharacter{}
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
	return checkpoint
}

func CheckpointHasSnapshot(checkpoint checkpointstore.Checkpoint) bool {
	return len(checkpoint.ProcessedBatchIndexes) > 0 ||
		len(checkpoint.ProcessedEpisodeIndexes) > 0 ||
		len(checkpoint.Characters) > 0 ||
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
		Extra             any      `json:"extra"`
	}{
		Extra: extra,
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
