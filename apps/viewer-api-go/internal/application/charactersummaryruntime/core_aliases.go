package charactersummaryruntime

import (
	"narou-viewer/apps/viewer-api-go/internal/characters"
	core "narou-viewer/apps/viewer-api-go/internal/charactersummary"
	"narou-viewer/apps/viewer-api-go/internal/library"
)

type characterSummaryChunk = core.Chunk
type characterSummaryBatch = core.Batch
type characterSummaryBatchBudget = core.BatchBudget
type characterSummaryDelta = core.Delta
type characterSummaryUnresolvedMention = core.UnresolvedMention
type characterSummaryGenerationState = core.GenerationState

const (
	characterSummaryDefaultMaxTokens        = 12000
	characterSummaryMinimumCompletionTokens = 512
)

func characterSummaryLimits() (int, int) {
	return core.Limits()
}

func extractCharacterSummaryEpisodeText(episode core.EpisodeInput) string {
	return core.ExtractEpisodeText(episode)
}

func createCharacterSummaryChunksFromText(episode core.EpisodeInput, text string, maxChunkChars int) []characterSummaryChunk {
	return core.CreateChunksFromText(episode, text, maxChunkChars)
}

func createCharacterSummaryBatches(chunks []characterSummaryChunk, maxBatchChars int) []characterSummaryBatch {
	return core.CreateBatches(chunks, maxBatchChars)
}

func createCharacterSummaryBatchesWithBudget(chunks []characterSummaryChunk, budget characterSummaryBatchBudget) []characterSummaryBatch {
	return core.CreateBatchesWithBudget(chunks, budget)
}

func buildCharacterSummaryPromptWithUnresolved(novelID string, upToEpisodeIndex string, knownCharacters []characters.GeneratedCharacter, batch characterSummaryBatch, unresolvedMentions []characters.GeneratedUnresolvedMention, systemPromptOverride *string) (string, string) {
	return core.BuildPromptWithUnresolved(novelID, upToEpisodeIndex, knownCharacters, batch, unresolvedMentions, systemPromptOverride)
}

func normalizeCharacterSummaryOpenRouterResponse(raw []byte, novelID string, fallbackEpisodeIndex string) (characterSummaryDelta, error) {
	return core.NormalizeOpenRouterResponse(raw, novelID, fallbackEpisodeIndex)
}

func applyCharacterSummaryDelta(novelID string, existing []characters.GeneratedCharacter, delta characterSummaryDelta, allocator *characters.GeneratedCharacterIDAllocator) ([]characters.GeneratedCharacter, int) {
	return core.ApplyDelta(novelID, existing, delta, allocator)
}

func mergeGeneratedUnresolvedMentions(existing []characters.GeneratedUnresolvedMention, incoming []characterSummaryUnresolvedMention) []characters.GeneratedUnresolvedMention {
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
