package httpapi

import (
	"narou-viewer/apps/viewer-api-go/internal/characters"
	"narou-viewer/apps/viewer-api-go/internal/charactersummary"
	"narou-viewer/apps/viewer-api-go/internal/library"
)

type characterSummaryMergeProposal = charactersummary.MergeProposal

const characterSummaryMergeAutoApplyConfidence = charactersummary.MergeAutoApplyConfidence

func characterSummaryLimits() (int, int) {
	return charactersummary.Limits()
}

func createCharacterSummaryChunks(episodes []characterSummaryEpisodeInput, maxChunkChars int) []characterSummaryChunk {
	return charactersummary.CreateChunks(episodes, maxChunkChars)
}

func extractCharacterSummaryEpisodeText(episode characterSummaryEpisodeInput) string {
	return charactersummary.ExtractEpisodeText(episode)
}

func createCharacterSummaryBatches(chunks []characterSummaryChunk, maxBatchChars int) []characterSummaryBatch {
	return charactersummary.CreateBatches(chunks, maxBatchChars)
}

func createCharacterSummaryBatchesWithBudget(chunks []characterSummaryChunk, budget characterSummaryBatchBudget) []characterSummaryBatch {
	return charactersummary.CreateBatchesWithBudget(chunks, budget)
}

func characterSummaryBudgetExceeded(chars int, tokens int, budget characterSummaryBatchBudget) bool {
	return charactersummary.BudgetExceeded(chars, tokens, budget)
}

func characterSummaryRuntimeBatch(template characterSummaryBatch, chunks []characterSummaryChunk) characterSummaryBatch {
	return charactersummary.RuntimeBatch(template, chunks)
}

func characterSummaryCandidateCards(values []characters.GeneratedCharacter, batch characterSummaryBatch) []map[string]any {
	return charactersummary.CandidateCards(values, batch)
}

func buildCharacterSummaryPrompt(novelID string, upToEpisodeIndex string, knownCharacters []characters.GeneratedCharacter, batch characterSummaryBatch, systemPromptOverride *string) (string, string) {
	return charactersummary.BuildPrompt(novelID, upToEpisodeIndex, knownCharacters, batch, systemPromptOverride)
}

func buildCharacterSummaryPromptWithUnresolved(novelID string, upToEpisodeIndex string, knownCharacters []characters.GeneratedCharacter, batch characterSummaryBatch, unresolvedMentions []characters.GeneratedUnresolvedMention, systemPromptOverride *string) (string, string) {
	return charactersummary.BuildPromptWithUnresolved(novelID, upToEpisodeIndex, knownCharacters, batch, unresolvedMentions, systemPromptOverride)
}

func normalizeCharacterSummaryOpenRouterResponse(raw []byte, novelID string, fallbackEpisodeIndex string) (characterSummaryDelta, error) {
	return charactersummary.NormalizeOpenRouterResponse(raw, novelID, fallbackEpisodeIndex)
}

func mergeGeneratedUnresolvedMentions(existing []characters.GeneratedUnresolvedMention, incoming []characterSummaryUnresolvedMention) []characters.GeneratedUnresolvedMention {
	return charactersummary.MergeGeneratedUnresolvedMentions(existing, incoming)
}

func filterResolvedGeneratedUnresolvedMentions(values []characters.GeneratedUnresolvedMention, generated []characters.GeneratedCharacter) []characters.GeneratedUnresolvedMention {
	return charactersummary.FilterResolvedGeneratedUnresolvedMentions(values, generated)
}

func applyCharacterSummaryDelta(novelID string, existing []characters.GeneratedCharacter, delta characterSummaryDelta, allocator *characters.GeneratedCharacterIDAllocator) ([]characters.GeneratedCharacter, int) {
	return charactersummary.ApplyDelta(novelID, existing, delta, allocator)
}

func applyCharacterSummaryMergeProposals(generated []characters.GeneratedCharacter, proposals []characterSummaryMergeProposal, changed int, allocator *characters.GeneratedCharacterIDAllocator) ([]characters.GeneratedCharacter, int) {
	return charactersummary.ApplyMergeProposals(generated, proposals, changed, allocator)
}

func reuseGeneratedCharacterIDsFromRegistry(generated []characters.GeneratedCharacter, registry []characters.GeneratedCharacter, state characterSummaryGenerationState, upToEpisodeIndex string) ([]characters.GeneratedCharacter, characterSummaryGenerationState) {
	return charactersummary.ReuseGeneratedCharacterIDsFromRegistry(generated, registry, state, upToEpisodeIndex)
}

func mergeGeneratedCharacters(existing []characters.GeneratedCharacter, incoming []characters.GeneratedCharacter) []characters.GeneratedCharacter {
	return charactersummary.MergeGeneratedCharacters(existing, incoming)
}

func latestGeneratedHistoryText(values []characters.GeneratedHistoryVersion) string {
	return charactersummary.LatestGeneratedHistoryText(values)
}

func summarizeGeneratedHistory(values []characters.GeneratedHistoryVersion) string {
	return charactersummary.SummarizeGeneratedHistory(values)
}

func firstNonEmptySummaryString(values ...string) string {
	return charactersummary.FirstNonEmptyString(values...)
}

func characterSummaryTokensFromChars(chars int) int {
	return charactersummary.TokensFromChars(chars)
}

func generatedCharacterIndexByID(values []characters.GeneratedCharacter, characterID string) int {
	return charactersummary.GeneratedCharacterIndexByID(values, characterID)
}

func generatedCharacterID(novelID string, canonicalName string) string {
	return charactersummary.GeneratedCharacterID(novelID, canonicalName)
}

func renderSummaryBlock(block library.ReaderBlock) string {
	return charactersummary.RenderBlock(block)
}
