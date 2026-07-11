package httpapi

import (
	"narou-viewer/apps/viewer-api-go/internal/characters"
	"narou-viewer/apps/viewer-api-go/internal/extraction"
	"narou-viewer/apps/viewer-api-go/internal/library"
)

type extractionMergeProposal = extraction.MergeProposal

const extractionMergeAutoApplyConfidence = extraction.MergeAutoApplyConfidence

func extractionLimits() (int, int) {
	return extraction.Limits()
}

func createExtractionChunks(episodes []extractionEpisodeInput, maxChunkChars int) []extractionChunk {
	return extraction.CreateChunks(episodes, maxChunkChars)
}

func extractExtractionEpisodeText(episode extractionEpisodeInput) string {
	return extraction.ExtractEpisodeText(episode)
}

func createExtractionBatches(chunks []extractionChunk, maxBatchChars int) []extractionBatch {
	return extraction.CreateBatches(chunks, maxBatchChars)
}

func createExtractionBatchesWithBudget(chunks []extractionChunk, budget extractionBatchBudget) []extractionBatch {
	return extraction.CreateBatchesWithBudget(chunks, budget)
}

func extractionBudgetExceeded(chars int, tokens int, budget extractionBatchBudget) bool {
	return extraction.BudgetExceeded(chars, tokens, budget)
}

func extractionRuntimeBatch(template extractionBatch, chunks []extractionChunk) extractionBatch {
	return extraction.RuntimeBatch(template, chunks)
}

func extractionCandidateCards(values []characters.GeneratedCharacter, batch extractionBatch) []map[string]any {
	return extraction.CandidateCards(values, batch)
}

func buildExtractionPrompt(novelID string, upToEpisodeIndex string, knownCharacters []characters.GeneratedCharacter, batch extractionBatch, systemPromptOverride *string) (string, string) {
	return extraction.BuildPrompt(novelID, upToEpisodeIndex, knownCharacters, batch, systemPromptOverride)
}

func buildExtractionPromptWithUnresolved(novelID string, upToEpisodeIndex string, knownCharacters []characters.GeneratedCharacter, batch extractionBatch, unresolvedMentions []characters.GeneratedUnresolvedMention, systemPromptOverride *string) (string, string) {
	return extraction.BuildPromptWithUnresolved(novelID, upToEpisodeIndex, knownCharacters, batch, unresolvedMentions, systemPromptOverride)
}

func normalizeExtractionOpenRouterResponse(raw []byte, novelID string, fallbackEpisodeIndex string) (extractionDelta, error) {
	return extraction.NormalizeOpenRouterResponse(raw, novelID, fallbackEpisodeIndex)
}

func mergeGeneratedUnresolvedMentions(existing []characters.GeneratedUnresolvedMention, incoming []extractionUnresolvedMention) []characters.GeneratedUnresolvedMention {
	return extraction.MergeGeneratedUnresolvedMentions(existing, incoming)
}

func filterResolvedGeneratedUnresolvedMentions(values []characters.GeneratedUnresolvedMention, generated []characters.GeneratedCharacter) []characters.GeneratedUnresolvedMention {
	return extraction.FilterResolvedGeneratedUnresolvedMentions(values, generated)
}

func applyExtractionDelta(novelID string, existing []characters.GeneratedCharacter, delta extractionDelta, allocator *characters.GeneratedCharacterIDAllocator) ([]characters.GeneratedCharacter, int) {
	return extraction.ApplyDelta(novelID, existing, delta, allocator)
}

func applyExtractionMergeProposals(generated []characters.GeneratedCharacter, proposals []extractionMergeProposal, changed int, allocator *characters.GeneratedCharacterIDAllocator) ([]characters.GeneratedCharacter, int) {
	return extraction.ApplyMergeProposals(generated, proposals, changed, allocator)
}

func reuseGeneratedCharacterIDsFromRegistry(generated []characters.GeneratedCharacter, registry []characters.GeneratedCharacter, state extractionGenerationState, upToEpisodeIndex string) ([]characters.GeneratedCharacter, extractionGenerationState) {
	return extraction.ReuseGeneratedCharacterIDsFromRegistry(generated, registry, state, upToEpisodeIndex)
}

func mergeGeneratedCharacters(existing []characters.GeneratedCharacter, incoming []characters.GeneratedCharacter) []characters.GeneratedCharacter {
	return extraction.MergeGeneratedCharacters(existing, incoming)
}

func latestGeneratedHistoryText(values []characters.GeneratedHistoryVersion) string {
	return extraction.LatestGeneratedHistoryText(values)
}

func summarizeGeneratedHistory(values []characters.GeneratedHistoryVersion) string {
	return extraction.SummarizeGeneratedHistory(values)
}

func firstNonEmptySummaryString(values ...string) string {
	return extraction.FirstNonEmptyString(values...)
}

func extractionTokensFromChars(chars int) int {
	return extraction.TokensFromChars(chars)
}

func generatedCharacterIndexByID(values []characters.GeneratedCharacter, characterID string) int {
	return extraction.GeneratedCharacterIndexByID(values, characterID)
}

func generatedCharacterID(novelID string, canonicalName string) string {
	return extraction.GeneratedCharacterID(novelID, canonicalName)
}

func renderSummaryBlock(block library.ReaderBlock) string {
	return extraction.RenderBlock(block)
}
