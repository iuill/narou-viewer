package httpapi

import (
	"context"

	"narou-viewer/apps/viewer-api-go/internal/ai"
	"narou-viewer/apps/viewer-api-go/internal/application/extractionruntime"
	"narou-viewer/apps/viewer-api-go/internal/application/readerassistant"
	extractdomain "narou-viewer/apps/viewer-api-go/internal/extraction"
	"narou-viewer/apps/viewer-api-go/internal/library"
	"narou-viewer/apps/viewer-api-go/internal/store"
)

type readerAssistantContext = readerassistant.Context
type readerAssistantSearchHit = readerassistant.SearchHit
type readerAssistantHitRegistry = readerassistant.HitRegistry
type readerAssistantFullTextCandidate = readerassistant.FullTextCandidate
type readerAssistantUsageInput = readerassistant.UsageInput
type readerAssistantToolResult = readerassistant.ToolResult

const (
	readerAssistantMaxEpisodeRangeCount   = readerassistant.MaxEpisodeRangeCount
	readerAssistantMaxFullTextQueryRunes  = readerassistant.MaxFullTextQueryRunes
	readerAssistantMaxFullTextTerms       = readerassistant.MaxFullTextTerms
	readerAssistantDefaultFullTextResults = readerassistant.DefaultFullTextResults
)

func readerAssistantServiceForTest(s *Server) *readerassistant.Service {
	if s != nil && s.readerAssistant != nil {
		return s.readerAssistant
	}
	if s == nil {
		return readerassistant.NewService(readerassistant.Dependencies{})
	}
	return readerassistant.NewService(readerassistant.Dependencies{
		Library:     s.library,
		Settings:    s.stateStore,
		StateDir:    s.stateDir(),
		UsageDBPath: s.aiUsageDBPath(),
	})
}

func normalizeReaderAssistantHistory(value any) []map[string]string {
	return readerassistant.NormalizeHistory(value)
}

func readerAssistantSearchQuery(message string) string {
	return readerassistant.SearchQuery(message)
}

func snippetAround(text string, position int, matchLength int, contextChars int) string {
	return readerassistant.SnippetAround(text, position, matchLength, contextChars)
}

func runeOffsetForByteIndex(text string, byteIndex int) int {
	return readerassistant.RuneOffsetForByteIndex(text, byteIndex)
}

func readerAssistantUsageRequests(toolRequests []map[string]any, inputTokens int, outputTokens int) []ai.UsageRequest {
	return readerassistant.UsageRequests(toolRequests, inputTokens, outputTokens)
}

func readerAssistantUsageRecentPreviousRange(currentNumber int, count int) map[string]any {
	return readerassistant.UsageRecentPreviousRange(currentNumber, count)
}

func sanitizeReaderAssistantSnapshotValue(value any) any {
	return readerassistant.SanitizeSnapshotValue(value)
}

func newReaderAssistantHitRegistry() *readerAssistantHitRegistry {
	return readerassistant.NewHitRegistry()
}

func readerAssistantRangeAround(tocEpisodes []library.TocEpisodeSummary, episodeIndex string) (int, int) {
	return readerassistant.RangeAround(tocEpisodes, episodeIndex)
}

func resolveReaderAssistantEpisodeRange(contextInfo readerAssistantContext, args map[string]any) (int, int, error) {
	return readerassistant.ResolveEpisodeRange(contextInfo, args)
}

func readerAssistantEpisodeNumberArg(value any, minNumber int, maxNumber int) (int, error) {
	return readerassistant.EpisodeNumberArg(value, minNumber, maxNumber)
}

func readerAssistantEpisodeVisible(contextInfo readerAssistantContext, episodeIndex string) bool {
	return readerassistant.EpisodeVisible(contextInfo, episodeIndex)
}

func readerAssistantToolResultMessage(name string) string {
	return readerassistant.ToolResultMessage(name)
}

func readerAssistantRecentPreviousScopeNote(contextInfo readerAssistantContext) string {
	return readerassistant.RecentPreviousScopeNote(contextInfo)
}

func readerAssistantToolRecovery(name string, err error) readerAssistantToolResult {
	return readerassistant.ToolRecovery(name, err)
}

func fmtEpisodeRangeError(maxCount int) error {
	return readerassistant.FmtEpisodeRangeError(maxCount)
}

func readerAssistantRecentPreviousEpisodeCount(message string) int {
	return readerassistant.RecentPreviousEpisodeCount(message)
}

func readerAssistantMaxResultsArg(value any) (int, error) {
	return readerassistant.MaxResultsArg(value)
}

func readerAssistantFullTextQueryArg(value string) (string, error) {
	return readerassistant.FullTextQueryArg(value)
}

func readerAssistantContextCharsArg(value any) (int, error) {
	return readerassistant.ContextCharsArg(value)
}

func readerAssistantHitIDsArg(value any) ([]string, error) {
	return readerassistant.HitIDsArg(value)
}

func readerAssistantSearchTerms(query string) []string {
	return readerassistant.SearchTerms(query)
}

func readerAssistantFindQueryPositions(text string, query string, terms []string, maxResults int) []int {
	return readerassistant.FindQueryPositions(text, query, terms, maxResults)
}

func readerAssistantTitleScore(title string, query string, terms []string) float64 {
	return readerassistant.TitleScore(title, query, terms)
}

func readerAssistantFullTextScore(text string, position int, query string, terms []string, titleScore float64) float64 {
	return readerassistant.FullTextScore(text, position, query, terms, titleScore)
}

func readerAssistantCoverageFullTextCandidates(primary []readerAssistantFullTextCandidate, fallback []readerAssistantFullTextCandidate, bucketCount int) []readerAssistantFullTextCandidate {
	return readerassistant.CoverageFullTextCandidates(primary, fallback, bucketCount)
}

func readerAssistantFullTextDistribution(candidates []readerAssistantFullTextCandidate) (int, any, any) {
	return readerassistant.FullTextDistribution(candidates)
}

func readerAssistantMatchLengthAt(text string, position int, query string, terms []string) int {
	return readerassistant.MatchLengthAt(text, position, query, terms)
}

func readerAssistantPassageRange(text string, position int, matchLength int, contextChars int) (int, int) {
	return readerassistant.PassageRange(text, position, matchLength, contextChars)
}

func substringRunes(text string, start int, end int) string {
	return readerassistant.SubstringRunes(text, start, end)
}

func byteIndexForRuneOffset(text string, offset int) int {
	return readerassistant.ByteIndexForRuneOffset(text, offset)
}

func intSliceContains(values []int, target int) bool {
	return readerassistant.IntSliceContains(values, target)
}

func stringValue(value *string) string {
	return readerassistant.StringValue(value)
}

func setCharacterJobProgress(job *extractdomain.Job, progress int, stage string, currentBatchIndex *int, batchCount *int, generatedCharacterCount *int) {
	extractionruntime.SetExtractionJobProgress(job, progress, stage, currentBatchIndex, batchCount, generatedCharacterCount, nil)
}

func characterJobBatchProgressPercent(completedBatches int, batchCount int) int {
	return extractionruntime.ExtractionJobBatchProgressPercent(completedBatches, batchCount)
}

func valueOrDefaultInt(value *int, fallback int) int {
	return extractionruntime.ValueOrDefaultInt(value, fallback)
}

func valueOrDefaultString(value *string, fallback string) string {
	return extractionruntime.ValueOrDefaultString(value, fallback)
}

func readerAssistantToolDefinitions() []ai.ToolDefinition {
	return readerassistant.ToolDefinitions()
}

func episodeReference(episode *library.EpisodeResponse) map[string]any {
	return readerassistant.EpisodeReference(episode)
}

func episodeNumberByIndex(tocEpisodes []library.TocEpisodeSummary, episodeIndex string) int {
	return readerassistant.EpisodeNumberByIndex(tocEpisodes, episodeIndex)
}

func resolveReaderAssistantSearchRange(contextInfo readerAssistantContext, args map[string]any) (int, int, error) {
	return readerassistant.ResolveSearchRange(contextInfo, args)
}

func resolveReaderAssistantFullTextSearchRange(contextInfo readerAssistantContext, args map[string]any) (int, int, error) {
	return readerassistant.ResolveFullTextSearchRange(contextInfo, args)
}

func buildReaderAssistantInput(contextInfo readerAssistantContext) string {
	return readerassistant.BuildInput(contextInfo)
}

func buildReaderAssistantInstructions(contextInfo readerAssistantContext) string {
	return readerassistant.BuildInstructions(contextInfo)
}

func decodeToolArguments(raw string) map[string]any {
	return readerassistant.DecodeToolArguments(raw)
}

func mustJSON(value any) string {
	return readerassistant.MustJSON(value)
}

func readerAssistantSummaryPurpose(value any) string {
	return readerassistant.SummaryPurpose(value)
}

func readerAssistantSummaryFocus(value any) any {
	return readerassistant.SummaryFocus(value)
}

func (s *Server) readerAssistantToolContext(ctx context.Context, novelID string, novelTitle string, currentEpisodeIndex string, currentEpisodeRef map[string]any, currentExcerpt string, message string, tocEpisodes []library.TocEpisodeSummary, streamSink readerassistant.StreamSink) ([]map[string]any, []map[string]any, bool) {
	return readerAssistantServiceForTest(s).ToolContext(ctx, novelID, novelTitle, currentEpisodeIndex, currentEpisodeRef, currentExcerpt, message, tocEpisodes, streamSink)
}

func (s *Server) previousEpisodeResult(ctx context.Context, novelID string, novelTitle string, currentEpisodeIndex string, tocEpisodes []library.TocEpisodeSummary) map[string]any {
	return readerAssistantServiceForTest(s).PreviousEpisodeResult(ctx, novelID, novelTitle, currentEpisodeIndex, tocEpisodes)
}

func (s *Server) loadEpisodeRangeResult(ctx context.Context, novelID string, novelTitle string, tocEpisodes []library.TocEpisodeSummary, startNumber int, endNumber int) map[string]any {
	return readerAssistantServiceForTest(s).LoadEpisodeRangeResult(ctx, novelID, novelTitle, tocEpisodes, startNumber, endNumber)
}

func (s *Server) readerAssistantEpisode(ctx context.Context, novelID string, episodeIndex string) (*library.EpisodeResponse, error) {
	return readerAssistantServiceForTest(s).Episode(ctx, novelID, episodeIndex)
}

func (s *Server) characterSnapshotResult(novelID string, currentEpisodeIndex string, tocEpisodes []library.TocEpisodeSummary) map[string]any {
	return readerAssistantServiceForTest(s).CharacterSnapshotResult(novelID, currentEpisodeIndex, tocEpisodes)
}

func (s *Server) resolveReaderAssistantConfig() (*store.ResolvedAIGenerationConfig, error) {
	return readerAssistantServiceForTest(s).ResolveConfig()
}

func (s *Server) recordReaderAssistantUsage(input readerAssistantUsageInput) error {
	return readerAssistantServiceForTest(s).RecordUsage(input)
}

func (s *Server) searchFullTextResult(ctx context.Context, contextInfo readerAssistantContext, query string, startNumber int, endNumber int, maxResults int) map[string]any {
	return readerAssistantServiceForTest(s).SearchFullTextResult(ctx, contextInfo, query, startNumber, endNumber, maxResults)
}

func (s *Server) loadPassagesResult(ctx context.Context, contextInfo readerAssistantContext, hitIDs []string, contextChars int) readerAssistantToolResult {
	return readerAssistantServiceForTest(s).LoadPassagesResult(ctx, contextInfo, hitIDs, contextChars)
}

func (s *Server) searchEpisodesResult(ctx context.Context, novelID string, query string, tocEpisodes []library.TocEpisodeSummary, maxEpisodeIndex string) map[string]any {
	return readerAssistantServiceForTest(s).SearchEpisodesResult(ctx, novelID, query, tocEpisodes, maxEpisodeIndex)
}

func (s *Server) executeReaderAssistantTool(ctx context.Context, contextInfo readerAssistantContext, name string, rawArguments string) readerAssistantToolResult {
	return readerAssistantServiceForTest(s).ExecuteTool(ctx, contextInfo, name, rawArguments)
}

func (s *Server) readerAssistantEpisodeText(ctx context.Context, novelID string, episodeIndex string, registry *readerAssistantHitRegistry) *readerassistant.EpisodeText {
	return readerAssistantServiceForTest(s).EpisodeText(ctx, novelID, episodeIndex, registry)
}

func (s *Server) runReaderAssistantAgentLoop(ctx context.Context, assistantContext readerAssistantContext, config ai.OpenRouterConfig, streamSink readerassistant.StreamSink) (ai.ChatResult, []map[string]any, []map[string]any, error) {
	return readerAssistantServiceForTest(s).RunAgentLoop(ctx, assistantContext, config, streamSink)
}

func (s *Server) processCharacterJob(ctx context.Context, novelID string, job extractdomain.Job) bool {
	processor := extractionruntime.NewProcessor(extractionruntime.Dependencies{
		StateDir: s.stateDir(),
		Workflow: s.extractionWorkflow(),
		Logger:   logExtractionTiming,
	})
	return processor.Process(ctx, novelID, job)
}
