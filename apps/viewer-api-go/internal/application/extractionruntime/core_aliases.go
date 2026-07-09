package extractionruntime

import (
	"narou-viewer/apps/viewer-api-go/internal/characters"
	core "narou-viewer/apps/viewer-api-go/internal/extraction"
	"narou-viewer/apps/viewer-api-go/internal/library"
)

type extractionChunk = core.Chunk
type extractionBatch = core.Batch
type extractionBatchBudget = core.BatchBudget
type extractionDelta = core.Delta
type extractionUnresolvedMention = core.UnresolvedMention
type extractionGenerationState = core.GenerationState

const (
	extractionDefaultMaxTokens        = 12000
	extractionMinimumCompletionTokens = 512
)

func extractionLimits() (int, int) {
	return core.Limits()
}

func extractExtractionEpisodeText(episode core.EpisodeInput) string {
	return core.ExtractEpisodeText(episode)
}

func createExtractionChunksFromText(episode core.EpisodeInput, text string, maxChunkChars int) []extractionChunk {
	return core.CreateChunksFromText(episode, text, maxChunkChars)
}

func createExtractionBatches(chunks []extractionChunk, maxBatchChars int) []extractionBatch {
	return core.CreateBatches(chunks, maxBatchChars)
}

func createExtractionBatchesWithBudget(chunks []extractionChunk, budget extractionBatchBudget) []extractionBatch {
	return core.CreateBatchesWithBudget(chunks, budget)
}

func buildExtractionPromptWithUnresolved(novelID string, upToEpisodeIndex string, knownCharacters []characters.GeneratedCharacter, batch extractionBatch, unresolvedMentions []characters.GeneratedUnresolvedMention, systemPromptOverride *string) (string, string) {
	return core.BuildPromptWithUnresolved(novelID, upToEpisodeIndex, knownCharacters, batch, unresolvedMentions, systemPromptOverride)
}

func normalizeExtractionOpenRouterResponse(raw []byte, novelID string, fallbackEpisodeIndex string) (extractionDelta, error) {
	return core.NormalizeOpenRouterResponse(raw, novelID, fallbackEpisodeIndex)
}

func applyExtractionDelta(novelID string, existing []characters.GeneratedCharacter, delta extractionDelta, allocator *characters.GeneratedCharacterIDAllocator) ([]characters.GeneratedCharacter, int) {
	return core.ApplyDelta(novelID, existing, delta, allocator)
}

func mergeGeneratedUnresolvedMentions(existing []characters.GeneratedUnresolvedMention, incoming []extractionUnresolvedMention) []characters.GeneratedUnresolvedMention {
	return core.MergeGeneratedUnresolvedMentions(existing, incoming)
}

func filterResolvedGeneratedUnresolvedMentions(values []characters.GeneratedUnresolvedMention, generated []characters.GeneratedCharacter) []characters.GeneratedUnresolvedMention {
	return core.FilterResolvedGeneratedUnresolvedMentions(values, generated)
}

func mergeGeneratedCharacters(existing []characters.GeneratedCharacter, incoming []characters.GeneratedCharacter) []characters.GeneratedCharacter {
	return core.MergeGeneratedCharacters(existing, incoming)
}

func sortGeneratedCharacters(values []characters.GeneratedCharacter) {
	core.SortGeneratedCharacters(values)
}

func mergeGeneratedTextVersionLists(lists ...[]characters.GeneratedTextVersion) []characters.GeneratedTextVersion {
	return core.MergeGeneratedTextVersionLists(lists...)
}

func mergeGeneratedHistoryVersionLists(lists ...[]characters.GeneratedHistoryVersion) []characters.GeneratedHistoryVersion {
	return core.MergeGeneratedHistoryVersionLists(lists...)
}

func mergeGeneratedCharacter(left characters.GeneratedCharacter, right characters.GeneratedCharacter) characters.GeneratedCharacter {
	return core.MergeGeneratedCharacter(left, right)
}

func firstNonEmptySummaryString(values ...string) string {
	return core.FirstNonEmptyString(values...)
}

func latestGeneratedHistoryText(values []characters.GeneratedHistoryVersion) string {
	return core.LatestGeneratedHistoryText(values)
}

func renderSummaryInlineTokens(tokens []library.ReaderInline) string {
	return core.RenderInlineTokens(tokens)
}
