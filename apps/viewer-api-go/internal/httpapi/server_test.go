package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"narou-viewer/apps/viewer-api-go/internal/ai"
	appextraction "narou-viewer/apps/viewer-api-go/internal/application/extraction"
	"narou-viewer/apps/viewer-api-go/internal/application/fetchercommands"
	"narou-viewer/apps/viewer-api-go/internal/characters"
	extractdomain "narou-viewer/apps/viewer-api-go/internal/extraction"
	"narou-viewer/apps/viewer-api-go/internal/extraction/checkpointstore"
	"narou-viewer/apps/viewer-api-go/internal/fetcher"
	"narou-viewer/apps/viewer-api-go/internal/library"
	"narou-viewer/apps/viewer-api-go/internal/publications"
	"narou-viewer/apps/viewer-api-go/internal/storageusage"
	"narou-viewer/apps/viewer-api-go/internal/store"
	"narou-viewer/apps/viewer-api-go/internal/terms"

	_ "modernc.org/sqlite"
)

func TestHTTPAPIHelperBranches(t *testing.T) {
	if !profileMetadataExists([]ai.Profile{{ID: "profile-1"}}, "profile-1") {
		t.Fatal("profileMetadataExists should find an existing profile")
	}
	if profileMetadataExists([]ai.Profile{{ID: "profile-1"}}, "missing") {
		t.Fatal("profileMetadataExists should reject a missing profile")
	}
	if got, ok := normalizeStringList([]string{" first ", "first", "second"}); !ok || len(got) != 2 || got[0] != "first" || got[1] != "second" {
		t.Fatalf("normalizeStringList should trim and dedupe string values: got=%+v ok=%v", got, ok)
	}
	if got, ok := normalizeInteger(float64(3)); !ok || got != 3 {
		t.Fatalf("normalizeInteger should accept integer floats, got=%d ok=%v", got, ok)
	}
	if got, ok := normalizeInteger(4); !ok || got != 4 {
		t.Fatalf("normalizeInteger should accept ints, got=%d ok=%v", got, ok)
	}
	if _, ok := normalizeInteger(float64(3.5)); ok {
		t.Fatal("normalizeInteger should reject fractional floats")
	}
	if _, ok := normalizeInteger("3"); ok {
		t.Fatal("normalizeInteger should reject strings")
	}
	falseValue := false
	if !boolPointerValue(nil, true) || boolPointerValue(&falseValue, true) {
		t.Fatal("boolPointerValue should honor explicit booleans and fallback values")
	}
	methodResponse := httptest.NewRecorder()
	if methodOnly(methodResponse, httptest.NewRequest(http.MethodPost, "/", nil), http.MethodGet, http.MethodPut) || methodResponse.Header().Get("allow") != "GET, PUT" {
		t.Fatalf("methodOnly should include all allowed methods, allow=%q", methodResponse.Header().Get("allow"))
	}
	emptyErrorResponse := httptest.NewRecorder()
	writeFetcherError(emptyErrorResponse, errors.New(""), "fallback message")
	if emptyErrorResponse.Code != http.StatusBadGateway || !strings.Contains(emptyErrorResponse.Body.String(), "fallback message") {
		t.Fatalf("writeFetcherError should use fallback for empty messages: code=%d body=%s", emptyErrorResponse.Code, emptyErrorResponse.Body.String())
	}
	preservedErrorResponse := httptest.NewRecorder()
	writeFetcherError(preservedErrorResponse, &fetcher.HTTPError{StatusCode: http.StatusNotImplemented, Message: "unsupported"}, "fallback message")
	var preservedErrorBody map[string]string
	if err := json.Unmarshal(preservedErrorResponse.Body.Bytes(), &preservedErrorBody); err != nil {
		t.Fatalf("decode preserved error response: %v", err)
	}
	if preservedErrorResponse.Code != http.StatusNotImplemented || preservedErrorBody["error"] != "unsupported" {
		t.Fatalf("writeFetcherError should preserve actionable fetcher statuses: code=%d body=%s", preservedErrorResponse.Code, preservedErrorResponse.Body.String())
	}
	commandService := fetchercommands.NewService(nil, nil)
	if _, err := commandService.Update(context.Background(), []string{"novel"}, fetchercommands.UpdateOptions{}); !errors.Is(err, fetchercommands.ErrWorkIDResolverUnavailable) {
		t.Fatalf("fetcher command service should reject unavailable resolver, got %v", err)
	}
	serverCtx, serverCancel := context.WithCancel(context.Background())
	serverWithCancel := &Server{ctx: serverCtx, cancel: serverCancel}
	if err := serverWithCancel.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown returned error: %v", err)
	}
	if err := serverCtx.Err(); !errors.Is(err, context.Canceled) {
		t.Fatalf("Shutdown should cancel server context, got %v", err)
	}
	if err := (*Server)(nil).Shutdown(context.Background()); err != nil {
		t.Fatalf("nil Shutdown returned error: %v", err)
	}
	workerCtx, workerCancel := context.WithCancel(context.Background())
	workerCancel()
	workerServer := &Server{ctx: workerCtx, dataDir: t.TempDir()}
	if workerServer.processCharacterJob(workerCtx, "novel", extractdomain.Job{Status: "queued"}) {
		t.Fatal("processCharacterJob should stop before starting work when context is canceled")
	}
	options, message := (&Server{}).parseExtractionRequestOptions(map[string]any{
		"profileId":            nil,
		"modelId":              nil,
		"providerOrder":        []any{"ProviderA", " ProviderB "},
		"allowFallbacks":       false,
		"requireParameters":    true,
		"reasoningEffort":      " xHIGH ",
		"systemPromptOverride": nil,
	})
	if message != "" || !options.ProfileResolution || options.Transient == nil || options.ProfileID != nil {
		t.Fatalf("transient null overrides should be accepted: options=%+v message=%q", options, message)
	}
	if strategyOptions, message := (&Server{}).parseExtractionRequestOptions(map[string]any{"generationStrategy": "parallel_identity"}); message != "" || strategyOptions.GenerationStrategy != "parallel_identity" {
		t.Fatalf("generation strategy should be accepted: options=%+v message=%q", strategyOptions, message)
	}
	if strategyOptions, message := (&Server{}).parseExtractionRequestOptions(map[string]any{"generationStrategy": "discovery_parallel_correction"}); message != "" || strategyOptions.GenerationStrategy != "discovery_parallel_correction" {
		t.Fatalf("discovery generation strategy should be accepted: options=%+v message=%q", strategyOptions, message)
	}
	if options.Transient.ModelID != nil || len(options.Transient.ProviderOrder) != 2 || options.Transient.AllowFallbacks == nil || *options.Transient.AllowFallbacks || options.Transient.RequireParameters == nil || !*options.Transient.RequireParameters || options.Transient.ReasoningEffort == nil || *options.Transient.ReasoningEffort != "xhigh" {
		t.Fatalf("transient overrides should be normalized: %+v", options.Transient)
	}
	if message, status := (&Server{}).resolveExtractionRequestOptions(&extractionRequestOptions{ProfileResolution: true}); message != "AI生成プロファイルが見つかりません。" || status != http.StatusBadRequest {
		t.Fatalf("nil store profile resolution should fail, got %q", message)
	}
	if message, status := (&Server{}).resolveExtractionRequestOptions(nil); message != "" || status != http.StatusOK {
		t.Fatalf("nil request options should be ignored, got %q", message)
	}
	for _, body := range []map[string]any{
		{"modelId": 10},
		{"systemPromptOverride": 10},
		{"reasoningEffort": "extreme"},
		{"generationStrategy": "unknown"},
	} {
		if _, message := (&Server{}).parseExtractionRequestOptions(body); message == "" {
			t.Fatalf("invalid character summary options should be rejected: body=%+v", body)
		}
	}
	nilUnlock := (*Server)(nil).extractionRuntime().LockTarget("novel-1", "1")
	nilUnlock()
	if got := appendUniqueInt([]int{1}, 1); len(got) != 1 || got[0] != 1 {
		t.Fatalf("appendUniqueInt should not duplicate existing values: %+v", got)
	}
	if got := mergeStringSets([]string{"1"}, []string{"", "1", "2"}); len(got) != 2 || got[1] != "2" {
		t.Fatalf("mergeStringSets should skip blanks and duplicates: %+v", got)
	}
	if fingerprint := extractionCheckpointFingerprint(nil, func() {}); fingerprint == "" {
		t.Fatal("extractionCheckpointFingerprint should return a stable fallback hash")
	}
}

func TestExtractionUsageTokenHelpers(t *testing.T) {
	episodes := []characters.HeuristicEpisode{
		{EpisodeIndex: "1", Text: "alpha beta"},
		{EpisodeIndex: "2", Text: "gamma"},
	}
	if got := heuristicEpisodeIndexes(episodes); len(got) != 2 || got[0] != "1" || got[1] != "2" {
		t.Fatalf("unexpected heuristic episode indexes: %+v", got)
	}
	expectedInputTokens := estimateTokenCount(episodes[0].Text) + estimateTokenCount(episodes[1].Text)
	if got := extractionInputTokens(episodes); got != expectedInputTokens {
		t.Fatalf("unexpected input token estimate: %d", got)
	}

	batches := []extractionBatch{
		{Chunks: []extractionChunk{{Text: "one two"}, {Text: "three"}}},
		{Chunks: []extractionChunk{{Text: "four"}}},
	}
	requests := extractionBatchUsageRequests(batches)
	if len(requests) != 2 || requests[0].RequestIndex != 0 || requests[1].RequestIndex != 1 {
		t.Fatalf("unexpected usage requests: %+v", requests)
	}
	if got := usageRequestsInputTokens(requests); got <= 0 {
		t.Fatalf("usage input tokens should be estimated: %+v", requests)
	}

	requests = append(requests, ai.UsageRequest{InputTokens: 3, OutputTokens: 4}, ai.UsageRequest{InputTokens: 1, OutputTokens: 2, TotalTokens: 20})
	if got := usageRequestsOutputTokens(requests); got != 6 {
		t.Fatalf("unexpected usage output token total: %d", got)
	}
	if got := usageRequestsTotalTokens(requests); got <= 20 {
		t.Fatalf("usage total tokens should include estimated and explicit totals: %d", got)
	}
}

func TestExtractionPlaygroundParallelStartEmitsBatchStatus(t *testing.T) {
	event := extractionPlaygroundProgressEvent(extractionBatchProgress{
		Phase: "parallelStart",
		Batch: extractionBatch{BatchIndex: 2, BatchCount: 4, EpisodeIndexes: []string{"3"}},
	})
	if event == nil || event["type"] != "status" || event["stage"] != "generating" || event["batchIndex"] != 2 || event["batchCount"] != 4 {
		t.Fatalf("parallelStart event = %+v, want public batch status", event)
	}
	if event := extractionPlaygroundProgressEvent(extractionBatchProgress{Phase: "error"}); event != nil {
		t.Fatalf("error phase should not emit playground progress event: %+v", event)
	}
}

func TestServerConstructorDoesNotStartCharacterJobLifecycle(t *testing.T) {
	dataDir := t.TempDir()
	stateStore := store.New(dataDir)
	if err := stateStore.Initialize(); err != nil {
		t.Fatalf("Initialize returned error: %v", err)
	}
	stateDir := filepath.Join(dataDir, "state")
	if err := characters.EnsureStateDirs(stateDir); err != nil {
		t.Fatalf("EnsureStateDirs returned error: %v", err)
	}
	startedAt := "2026-01-01T00:00:00Z"
	job := extractdomain.Job{
		JobID:                     "job-running",
		RequestedUpToEpisodeIndex: "1",
		GenerationMode:            "heuristic",
		Status:                    "running",
		CreatedAt:                 "2026-01-01T00:00:00Z",
		StartedAt:                 &startedAt,
	}
	if err := extractdomain.SaveJob(stateDir, "novel-1", job); err != nil {
		t.Fatalf("SaveJob returned error: %v", err)
	}

	handler := NewServerWithDependencies(ServerDependencies{
		DataDir:       dataDir,
		Library:       library.NewService(filepath.Join(dataDir, "novel-fetcher")),
		StateStore:    stateStore,
		FetcherClient: fetcher.NewClient("http://127.0.0.1:1"),
	})
	server := handler.(*Server)
	t.Cleanup(func() {
		_ = server.Shutdown(context.Background())
	})

	jobs, ok, err := extractdomain.LoadJobs(stateDir, "novel-1")
	if err != nil || !ok || len(jobs) != 1 {
		t.Fatalf("LoadJobs after constructor: jobs=%+v ok=%v err=%v", jobs, ok, err)
	}
	if jobs[0].Status != "running" {
		t.Fatalf("constructor should not recover running jobs, got status %q", jobs[0].Status)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	server.StartBackground(ctx)
	jobs, ok, err = extractdomain.LoadJobs(stateDir, "novel-1")
	if err != nil || !ok || len(jobs) != 1 {
		t.Fatalf("LoadJobs after StartBackground: jobs=%+v ok=%v err=%v", jobs, ok, err)
	}
	if jobs[0].Status != "queued" {
		t.Fatalf("StartBackground should recover running jobs, got status %q", jobs[0].Status)
	}
}

func TestExtractionProcessedEpisodesChangedDetectsContentEtagDiff(t *testing.T) {
	dataDir := newHTTPAPITestData(t)
	server := newTestServerWithLibraryAndStore(dataDir, library.NewService(filepath.Join(dataDir, "novel-fetcher")), store.New(dataDir)).(*Server)
	novelID := library.NovelID(library.Work{ID: 1, Site: "syosetu", SiteWorkID: "n1234"})
	toc, err := server.library.GetToc(context.Background(), novelID)
	if err != nil || toc == nil || len(toc.Episodes) != 1 {
		t.Fatalf("fixture toc should load: toc=%+v err=%v", toc, err)
	}
	eventsDir := filepath.Join(dataDir, "state", "character_events")
	if err := os.MkdirAll(eventsDir, 0o755); err != nil {
		t.Fatalf("mkdir events dir: %v", err)
	}
	writeHTTPFixtureFile(t, filepath.Join(eventsDir, novelID+".yaml"), `
schema_version: 1
novel_id: `+novelID+`
processed_up_to_episode_index: "1"
next_character_ordinal: 1
episode_etags:
  - episode_index: "1"
    content_etag: "`+toc.Episodes[0].ContentEtag+`"
characters: []
`)
	processed := "1"
	reprocessFrom, err := server.extractionReprocessFromEpisode(context.Background(), novelID, &processed, processed)
	if err != nil || reprocessFrom != "" {
		t.Fatalf("matching content etag should not trigger reprocess: reprocessFrom=%q err=%v", reprocessFrom, err)
	}
	writeHTTPFixtureFile(t, filepath.Join(eventsDir, novelID+".yaml"), `
schema_version: 1
novel_id: `+novelID+`
processed_up_to_episode_index: "1"
next_character_ordinal: 1
episode_etags:
  - episode_index: "1"
    content_etag: "stale-etag"
characters: []
`)
	reprocessFrom, err = server.extractionReprocessFromEpisode(context.Background(), novelID, &processed, processed)
	if err != nil || reprocessFrom != "1" {
		t.Fatalf("stale content etag should trigger reprocess: reprocessFrom=%q err=%v", reprocessFrom, err)
	}
	writeHTTPFixtureFile(t, filepath.Join(eventsDir, novelID+".yaml"), `
schema_version: 1
novel_id: `+novelID+`
processed_up_to_episode_index: "1"
next_character_ordinal: 1
characters: []
`)
	reprocessFrom, err = server.extractionReprocessFromEpisode(context.Background(), novelID, &processed, processed)
	if err != nil || reprocessFrom != "1" {
		t.Fatalf("missing legacy etags should trigger bootstrap reprocess: reprocessFrom=%q err=%v", reprocessFrom, err)
	}
	processed = "2"
	writeHTTPFixtureFile(t, filepath.Join(eventsDir, novelID+".yaml"), `
schema_version: 1
novel_id: `+novelID+`
processed_up_to_episode_index: "2"
next_character_ordinal: 1
episode_etags:
  - episode_index: "1"
    content_etag: "`+toc.Episodes[0].ContentEtag+`"
  - episode_index: "2"
    content_etag: "deleted-etag"
characters: []
`)
	reprocessFrom, err = server.extractionReprocessFromEpisode(context.Background(), novelID, &processed, "1")
	if err != nil || reprocessFrom != "" {
		t.Fatalf("changes after requested episode should not trigger reprocess: reprocessFrom=%q err=%v", reprocessFrom, err)
	}
	reprocessFrom, err = server.extractionReprocessFromEpisode(context.Background(), novelID, &processed, processed)
	if err != nil || reprocessFrom != "2" {
		t.Fatalf("deleted processed episode should trigger reprocess from the deleted episode: reprocessFrom=%q err=%v", reprocessFrom, err)
	}
}

func TestNonStreamingLLMContextUsesShorterDeadlineThanServerWriteTimeout(t *testing.T) {
	ctx, cancel := nonStreamingLLMContext(context.Background())
	defer cancel()
	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("non-streaming LLM context should have a deadline")
	}
	remaining := time.Until(deadline)
	if remaining <= 0 || remaining > nonStreamingLLMTimeout || remaining > streamingWriteDeadline {
		t.Fatalf("unexpected non-streaming LLM timeout: remaining=%s configured=%s", remaining, nonStreamingLLMTimeout)
	}
	if nonStreamingLLMTimeout >= streamingWriteDeadline {
		t.Fatalf("non-streaming LLM timeout should stay shorter than streaming write deadline: nonStreaming=%s streaming=%s", nonStreamingLLMTimeout, streamingWriteDeadline)
	}
}

func TestExtractionTargetLockSerializesSameTarget(t *testing.T) {
	server := &Server{}
	unlockFirst := server.extractionRuntime().LockTarget("novel-1", "1")

	acquiredSameTarget := make(chan struct{})
	releaseSameTarget := make(chan struct{})
	go func() {
		unlock := server.extractionRuntime().LockTarget("novel-1", "1")
		close(acquiredSameTarget)
		<-releaseSameTarget
		unlock()
	}()

	select {
	case <-acquiredSameTarget:
		t.Fatal("same target lock should wait for the first holder")
	case <-time.After(20 * time.Millisecond):
	}

	unlockOther := server.extractionRuntime().LockTarget("novel-1", "2")
	unlockOther()

	unlockFirst()
	select {
	case <-acquiredSameTarget:
	case <-time.After(time.Second):
		t.Fatal("same target lock did not unblock after release")
	}
	close(releaseSameTarget)
}

func TestAIGenerationPreviewHelperBranches(t *testing.T) {
	if got := extractionPlaygroundErrorMessage(nil); got != "Character profiles could not be read." {
		t.Fatalf("nil playground error should use default message, got %q", got)
	}
	if got := extractionPlaygroundErrorMessage(errors.New("   ")); got != "Character profiles could not be read." {
		t.Fatalf("blank playground error should use default message, got %q", got)
	}
	config := &store.ResolvedAIGenerationConfig{
		ProfileID:    "profile-1",
		ProfileLabel: "Profile One",
		ModelID:      "openrouter/model-1",
	}
	if got := resolvedProfileID(config); got == nil || *got != "profile-1" {
		t.Fatalf("resolvedProfileID should expose configured profile id: %+v", got)
	}
	if got := resolvedProfileLabel(config); got == nil || *got != "Profile One" {
		t.Fatalf("resolvedProfileLabel should expose configured profile label: %+v", got)
	}
	if got := resolvedModelID(config); got == nil || *got != "openrouter/model-1" {
		t.Fatalf("resolvedModelID should expose configured model id: %+v", got)
	}
	emptyConfig := &store.ResolvedAIGenerationConfig{ProfileID: " ", ProfileLabel: " ", ModelID: " "}
	if resolvedProfileID(nil) != nil || resolvedProfileID(emptyConfig) != nil {
		t.Fatal("resolvedProfileID should omit missing profile ids")
	}
	if resolvedProfileLabel(nil) != nil || resolvedProfileLabel(emptyConfig) != nil {
		t.Fatal("resolvedProfileLabel should omit missing profile labels")
	}
	if resolvedModelID(nil) != nil || resolvedModelID(emptyConfig) != nil {
		t.Fatal("resolvedModelID should omit missing model ids")
	}

	document := library.ReaderDocument{
		Blocks: []library.ReaderBlock{
			{Section: "body", PlainText: " first "},
			{Section: "body", Text: " second "},
			{Section: "postscript", PlainText: "ignored"},
		},
	}
	if got := readerDocumentBodyText(document); got != "first\nsecond" {
		t.Fatalf("readerDocumentBodyText should join body plain text and fallback text, got %q", got)
	}

	preview := map[string]any{
		"batches": []map[string]any{{
			"episodeIndexes": []string{"2", "10"},
			"chunkCount":     3,
		}},
	}
	indexes := previewEpisodeIndexes(preview)
	if len(indexes) != 2 || indexes[0] != "2" || indexes[1] != "10" {
		t.Fatalf("unexpected preview indexes: %+v", indexes)
	}
	if count := previewChunkCount(preview); count != 3 {
		t.Fatalf("unexpected preview chunk count: %d", count)
	}
	if indexes := previewEpisodeIndexes(map[string]any{"batches": []any{}}); len(indexes) != 0 {
		t.Fatalf("malformed preview indexes should be empty: %+v", indexes)
	}
	if count := previewChunkCount(map[string]any{"batches": []map[string]any{{"chunkCount": "bad"}}}); count != 0 {
		t.Fatalf("malformed preview chunk count should be zero: %d", count)
	}
	typedPreview := appextraction.PromptPreview{
		Batches: []appextraction.PromptPreviewBatch{
			{EpisodeIndexes: []string{"1", "2"}, ChunkCount: 2},
			{EpisodeIndexes: []string{"2", "3"}, ChunkCount: 4},
		},
	}
	if indexes := promptPreviewEpisodeIndexes(typedPreview); len(indexes) != 3 || indexes[0] != "1" || indexes[1] != "2" || indexes[2] != "3" {
		t.Fatalf("typed prompt preview indexes should dedupe across batches: %+v", indexes)
	}
	if count := promptPreviewChunkCount(typedPreview); count != 6 {
		t.Fatalf("typed prompt preview chunk count should sum all batches: %d", count)
	}
	if got := truncateRunes("abcdef", 3); got != "abc" {
		t.Fatalf("truncateRunes should cut by rune count, got %q", got)
	}
	if got := truncateRunes("abcdef", 0); got != "" {
		t.Fatalf("truncateRunes with non-positive limit should return empty text, got %q", got)
	}
	if compareEpisodeString("10", "2") <= 0 || compareEpisodeString("same", "same") != 0 || compareEpisodeString("alpha", "beta") >= 0 {
		t.Fatal("compareEpisodeString should compare numeric values first and fallback to lexical order")
	}

	history := normalizeReaderAssistantHistory([]any{
		map[string]any{"role": "system", "text": "ignored"},
		map[string]any{"role": "user", "text": " 質問 "},
		map[string]any{"role": "assistant", "text": strings.Repeat("a", 700)},
		"bad",
	})
	if len(history) != 2 || history[0]["text"] != "質問" || len([]rune(history[1]["text"])) != 600 {
		t.Fatalf("reader assistant history should trim, filter, and truncate: %+v", history)
	}
	longHistory := []any{}
	for index := 0; index < 10; index++ {
		longHistory = append(longHistory, map[string]any{"role": "user", "text": fmt.Sprintf("質問%d", index)})
	}
	latestHistory := normalizeReaderAssistantHistory(longHistory)
	if len(latestHistory) != 8 || latestHistory[0]["text"] != "質問2" || latestHistory[7]["text"] != "質問9" {
		t.Fatalf("reader assistant history should keep the latest 8 messages: %+v", latestHistory)
	}
	if len(normalizeReaderAssistantHistory("bad")) != 0 {
		t.Fatal("malformed history should be ignored")
	}
	if got := readerAssistantSearchQuery("アリス ボブについて"); got != "ボブについて" {
		t.Fatalf("readerAssistantSearchQuery should pick the longest bounded term, got %q", got)
	}
	if snippet := snippetAround("0123456789abcdef", 8, 2, 6); snippet != "56789a" {
		t.Fatalf("unexpected snippet: %q", snippet)
	}
	if snippet := snippetAround("short", 99, 10, 4); snippet != "rt" {
		t.Fatalf("snippetAround should clamp oversized positions, got %q", snippet)
	}
	japaneseText := strings.Repeat("あ", 100) + "ボブ"
	bytePosition := strings.Index(strings.ToLower(japaneseText), "ボブ")
	runePosition := runeOffsetForByteIndex(strings.ToLower(japaneseText), bytePosition)
	japaneseSnippet := snippetAround(japaneseText, runePosition, len([]rune("ボブ")), 20)
	if !strings.Contains(japaneseSnippet, "ボブ") {
		t.Fatalf("multi-byte snippet should include the match without panicking: %q", japaneseSnippet)
	}
	if offset := runeOffsetForByteIndex("abc", -1); offset != 0 {
		t.Fatalf("runeOffsetForByteIndex should clamp negative byte indexes, got %d", offset)
	}
	if firstNonEmptyString("", " value ") != " value " {
		t.Fatal("firstNonEmptyString should return first non-empty value")
	}
	if firstNonEmptyString("", "") != "" {
		t.Fatal("firstNonEmptyString should return empty when no values are present")
	}
	if elapsedMsBetweenISO("bad", ai.NowISO()) != 0 || maxInt(1, 2) != 2 || maxInt(3, 2) != 3 || estimateTokenCount("") != 0 {
		t.Fatal("reader assistant helper fallbacks should be stable")
	}
	usageRequests := readerAssistantUsageRequests([]map[string]any{{"name": "search_episodes"}}, 3, 4)
	if len(usageRequests) != 2 || usageRequests[0].Kind != "tool_call" || usageRequests[1].Kind != "final_answer" || usageRequests[1].TotalTokens != 7 {
		t.Fatalf("unexpected usage request summary: %+v", usageRequests)
	}
}

func TestGenerateAndSaveExtractionDirectBranches(t *testing.T) {
	dataDir := newHTTPAPITestData(t)
	stateStore := store.New(dataDir)
	if err := stateStore.Initialize(); err != nil {
		t.Fatalf("initialize store: %v", err)
	}
	libraryService := library.NewService(filepath.Join(dataDir, "novel-fetcher"))
	handler := newTestServerWithLibraryAndStore(dataDir, libraryService, stateStore)
	server := handler.(*Server)
	novels := requestJSON(t, handler, http.MethodGet, "/api/library/novels", nil, http.StatusOK)
	novelID := novels["novels"].([]any)[0].(map[string]any)["novelId"].(string)

	preview, err := server.extractionPromptPreview(context.Background(), novelID, "1", nil)
	if err != nil {
		t.Fatalf("extractionPromptPreview returned error: %v", err)
	}
	if indexes := previewEpisodeIndexes(preview); len(indexes) != 1 || indexes[0] != "1" || previewChunkCount(preview) != 1 {
		t.Fatalf("unexpected prompt preview: %+v", preview)
	}
	emptyPreview, err := (&Server{}).extractionPromptPreview(context.Background(), novelID, "1", nil)
	if err != nil {
		t.Fatalf("nil-library extractionPromptPreview returned error: %v", err)
	}
	if len(previewEpisodeIndexes(emptyPreview)) != 0 || previewChunkCount(emptyPreview) != 0 {
		t.Fatalf("nil-library prompt preview should be empty: %+v", emptyPreview)
	}

	if err := server.generateAndSaveExtraction(context.Background(), novelID, "1", nil, nil); err != nil {
		t.Fatalf("heuristic generateAndSaveExtraction returned error: %v", err)
	}
	heuristic, ok, err := characters.LoadSummary(server.stateDir(), novelID, "1")
	if err != nil || !ok || heuristic.Status != "ready" {
		t.Fatalf("heuristic summary should be readable: ok=%v summary=%+v err=%v", ok, heuristic, err)
	}
	heuristicPreview, err := server.generateExtractionPreview(context.Background(), novelID, "1", nil, nil, []string{"1"}, nil)
	if err != nil || heuristicPreview.Status != "ready" || heuristicPreview.ProcessedUpToEpisodeIndex == nil {
		t.Fatalf("heuristic preview should be built without touching state profiles: preview=%+v err=%v", heuristicPreview, err)
	}
	if _, err := loadRequiredExtractionPreview(t.TempDir(), "novel-1", "1", nil); err == nil {
		t.Fatal("loadRequiredExtractionPreview should fail when the temporary summary is missing")
	}

	if _, err := stateStore.PutAIGenerationPreferredMode("llm"); err != nil {
		t.Fatalf("PutAIGenerationPreferredMode returned error: %v", err)
	}
	if err := server.generateAndSaveExtraction(context.Background(), novelID, "1", nil, nil); err == nil {
		t.Fatal("disabled generateAndSaveExtraction should fail")
	}
	if _, err := server.generateExtractionPreview(context.Background(), novelID, "1", nil, nil, []string{"1"}, nil); err == nil {
		t.Fatal("disabled generateExtractionPreview should fail")
	}
	afterDisabled, ok, err := characters.LoadSummary(server.stateDir(), novelID, "1")
	if err != nil || !ok || afterDisabled.Status != heuristic.Status || len(afterDisabled.Characters) != len(heuristic.Characters) {
		t.Fatalf("disabled summary should leave existing summary unchanged: ok=%v summary=%+v err=%v", ok, afterDisabled, err)
	}
}

func TestLibraryNovelsSortByLastActivityAt(t *testing.T) {
	dataDir := newHTTPAPITestData(t)
	db, err := sql.Open("sqlite", filepath.Join(dataDir, "novel-fetcher", "library.sqlite"))
	if err != nil {
		t.Fatalf("open library sqlite: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO works (
			id, site, site_name, site_work_id, source_url, title, author, story, directory, fetched_at,
			fetch_status, last_fetch_error, last_failed_episode_id, resume_episode_id, expected_episode_count
		) VALUES (
			2, 'syosetu', '小説家になろう', 'n5678', 'https://ncode.syosetu.com/n5678/',
			'New Download', 'Author', 'Story', 'works/syosetu/n5678', '2026-06-01T00:00:00Z',
			'complete', '', '', '', 0
		)
	`); err != nil {
		t.Fatalf("insert second work: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close library sqlite: %v", err)
	}

	stateStore := store.New(dataDir)
	if err := stateStore.Initialize(); err != nil {
		t.Fatalf("initialize store: %v", err)
	}
	readNovelID := library.NovelID(library.Work{ID: 1, Site: "syosetu", SiteWorkID: "n1234"})
	writeHTTPFixtureFile(t, filepath.Join(dataDir, "state", "reading_state.yaml"), `
schema_version: 3
revision: 1
novels:
  `+strconv.Quote(readNovelID)+`:
    last_read_episode_index: "1"
    position: 0
    updated_at: "2026-06-02T00:00:00.000Z"
    state_version: 1
`)

	handler := newTestServerWithLibraryAndStore(dataDir, library.NewService(filepath.Join(dataDir, "novel-fetcher")), stateStore)
	response := requestJSON(t, handler, http.MethodGet, "/api/library/novels", nil, http.StatusOK)
	novels := response["novels"].([]any)
	if len(novels) != 2 {
		t.Fatalf("expected two novels, got %+v", response)
	}
	first := novels[0].(map[string]any)
	second := novels[1].(map[string]any)
	if first["novelId"] != readNovelID || first["lastActivityAt"] != "2026-06-02T00:00:00.000Z" {
		t.Fatalf("read novel should sort first by reading activity: first=%+v second=%+v", first, second)
	}
	if second["title"] != "New Download" || second["lastActivityAt"] != "2026-06-01T00:00:00Z" {
		t.Fatalf("download activity should remain as fallback: first=%+v second=%+v", first, second)
	}

	toc := requestJSON(t, handler, http.MethodGet, "/api/library/novels/"+readNovelID+"/toc", nil, http.StatusOK)
	if toc["lastActivityAt"] != "2026-06-02T00:00:00.000Z" || toc["lastReadEpisodeIndex"] != "1" || toc["lastReadEpisodeTitle"] != "Episode 1" {
		t.Fatalf("toc should expose the same reader activity summary: %+v", toc)
	}
}

func TestLibraryNovelsExposePublicationCover(t *testing.T) {
	dataDir := newHTTPAPITestData(t)
	stateStore := store.New(dataDir)
	if err := stateStore.Initialize(); err != nil {
		t.Fatalf("initialize store: %v", err)
	}
	novelID := library.NovelID(library.Work{ID: 1, Site: "syosetu", SiteWorkID: "n1234"})
	repository := publications.NewRepository(filepath.Join(dataDir, "state"))
	if _, err := repository.PutEntry(novelID, publications.Entry{
		Kind:     publications.KindComic,
		Status:   publications.EntryStatusManual,
		Override: publications.OverrideModeISBN,
		ISBN13:   "9784040000008",
		ImageURL: "https://example.test/comic-cover.jpg",
	}); err != nil {
		t.Fatalf("write comic publication fixture: %v", err)
	}
	if _, err := repository.PutEntry(novelID, publications.Entry{
		Kind:           publications.KindNovel,
		Status:         publications.EntryStatusManual,
		Override:       publications.OverrideModeISBN,
		ISBN13:         "9784040000009",
		ImageURL:       "https://example.test/novel-cover.jpg",
		CoverSource:    "Google Books",
		CoverSourceURL: "https://books.google.test/novel",
	}); err != nil {
		t.Fatalf("write novel publication fixture: %v", err)
	}

	handler := newTestServerWithLibraryAndStore(dataDir, library.NewService(filepath.Join(dataDir, "novel-fetcher")), stateStore)
	response := requestJSON(t, handler, http.MethodGet, "/api/library/novels", nil, http.StatusOK)
	novels := response["novels"].([]any)
	if len(novels) != 1 {
		t.Fatalf("expected one novel, got %+v", response)
	}
	novel := novels[0].(map[string]any)
	if novel["publicationCoverImageUrl"] != "https://example.test/novel-cover.jpg" || novel["publicationCoverKind"] != "novel" {
		t.Fatalf("novel publication cover should be exposed and prefer novel edition: %+v", novel)
	}
	if novel["publicationCoverSource"] != "Google Books" || novel["publicationCoverSourceUrl"] != "https://books.google.test/novel" {
		t.Fatalf("novel publication cover source should be exposed: %+v", novel)
	}

	if _, err := repository.PutDisplayCoverEntryID(novelID, "comic"); err != nil {
		t.Fatalf("write selected publication cover fixture: %v", err)
	}
	selectedResponse := requestJSON(t, handler, http.MethodGet, "/api/library/novels", nil, http.StatusOK)
	selectedNovel := selectedResponse["novels"].([]any)[0].(map[string]any)
	if selectedNovel["publicationCoverImageUrl"] != "https://example.test/comic-cover.jpg" || selectedNovel["publicationCoverKind"] != "comic" {
		t.Fatalf("selected publication cover should be exposed before fallback: %+v", selectedNovel)
	}
	if _, ok := selectedNovel["publicationCoverSource"]; ok {
		t.Fatalf("selected comic cover without source should not expose stale cover source: %+v", selectedNovel)
	}
}

func TestProcessCharacterJobMarksFailedOnGenerationError(t *testing.T) {
	dataDir := newHTTPAPITestData(t)
	stateStore := store.New(dataDir)
	if err := stateStore.Initialize(); err != nil {
		t.Fatalf("initialize store: %v", err)
	}
	if err := characters.EnsureStateDirs(filepath.Join(dataDir, "state")); err != nil {
		t.Fatalf("initialize character state dirs: %v", err)
	}
	handler := newTestServerWithLibraryAndStore(dataDir, library.NewService(filepath.Join(dataDir, "novel-fetcher")), stateStore)
	server := handler.(*Server)
	novels := requestJSON(t, handler, http.MethodGet, "/api/library/novels", nil, http.StatusOK)
	novelID := novels["novels"].([]any)[0].(map[string]any)["novelId"].(string)
	job := extractdomain.Job{
		JobID:                     "job-failure",
		RequestedUpToEpisodeIndex: "1",
		GenerationMode:            "heuristic",
		Status:                    "queued",
		CreatedAt:                 ai.NowISO(),
	}
	savedJob, _, err := extractdomain.SaveJobIfNoActive(server.stateDir(), novelID, job)
	if err != nil {
		t.Fatalf("SaveJobIfNoActive returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "state", "ai_generation_settings.yaml"), []byte("profiles: ["), 0o644); err != nil {
		t.Fatalf("corrupt AI settings: %v", err)
	}
	server.processCharacterJob(context.Background(), novelID, savedJob)
	jobs, ok, err := extractdomain.LoadJobs(server.stateDir(), novelID)
	if err != nil || !ok {
		t.Fatalf("load failed job: ok=%v jobs=%+v err=%v", ok, jobs, err)
	}
	var failedJob *extractdomain.Job
	for index := range jobs {
		if jobs[index].JobID == savedJob.JobID {
			failedJob = &jobs[index]
			break
		}
	}
	if failedJob == nil || failedJob.Status != "failed" || failedJob.FinishedAt == nil || failedJob.ErrorMessage == nil {
		t.Fatalf("job should be marked failed: %+v", jobs)
	}
}

func TestCharacterJobSubmitStoresGenerationStrategy(t *testing.T) {
	dataDir := newHTTPAPITestData(t)
	stateStore := store.New(dataDir)
	if err := stateStore.Initialize(); err != nil {
		t.Fatalf("initialize store: %v", err)
	}
	handler := newTestServerWithLibraryAndStore(dataDir, library.NewService(filepath.Join(dataDir, "novel-fetcher")), stateStore)
	server := handler.(*Server)
	server.cancel()
	novels := requestJSON(t, handler, http.MethodGet, "/api/library/novels", nil, http.StatusOK)
	novelID := novels["novels"].([]any)[0].(map[string]any)["novelId"].(string)

	response := requestJSON(t, handler, http.MethodPost, "/api/library/novels/"+novelID+"/extraction-jobs", map[string]any{
		"upToEpisodeIndex":   "1",
		"generationStrategy": "parallel_identity",
	}, http.StatusAccepted)
	if response["generationStrategy"] != "parallel_identity" {
		t.Fatalf("response should include generation strategy: %+v", response)
	}
	jobs, ok, err := extractdomain.LoadJobs(server.stateDir(), novelID)
	if err != nil || !ok || len(jobs) == 0 {
		t.Fatalf("jobs not saved: ok=%v jobs=%+v err=%v", ok, jobs, err)
	}
	if jobs[0].GenerationStrategy != "parallel_identity" {
		t.Fatalf("job strategy = %q, want parallel_identity", jobs[0].GenerationStrategy)
	}
	aiJobs := requestJSON(t, handler, http.MethodGet, "/api/ai-generation/jobs", nil, http.StatusOK)
	if firstJob := aiJobs["jobs"].([]any)[0].(map[string]any); firstJob["generationStrategy"] != "parallel_identity" {
		t.Fatalf("AI jobs response should include generationStrategy: %+v", firstJob)
	}
}

func TestReaderAssistantToolContextUsesBoundaryTools(t *testing.T) {
	dataDir := newHTTPAPITestData(t)
	stateStore := store.New(dataDir)
	if err := stateStore.Initialize(); err != nil {
		t.Fatalf("initialize store: %v", err)
	}
	if err := characters.EnsureStateDirs(filepath.Join(dataDir, "state")); err != nil {
		t.Fatalf("initialize character state dirs: %v", err)
	}
	handler := newTestServerWithLibraryAndStore(dataDir, library.NewService(filepath.Join(dataDir, "novel-fetcher")), stateStore)
	server := handler.(*Server)
	novels := requestJSON(t, handler, http.MethodGet, "/api/library/novels", nil, http.StatusOK)
	novelID := novels["novels"].([]any)[0].(map[string]any)["novelId"].(string)
	toc, err := server.library.GetToc(context.Background(), novelID)
	if err != nil || toc == nil || len(toc.Episodes) < 1 {
		t.Fatalf("fixture should include episodes: toc=%+v err=%v", toc, err)
	}
	current, err := server.library.GetEpisode(context.Background(), novelID, toc.Episodes[0].EpisodeIndex)
	if err != nil || current == nil {
		t.Fatalf("load current fixture episode: episode=%+v err=%v", current, err)
	}
	streamEvents := []map[string]any{}
	requests, results, ok := server.readerAssistantToolContext(
		context.Background(),
		novelID,
		toc.Title,
		current.EpisodeIndex,
		episodeReference(current),
		readerDocumentBodyText(current.ReaderDocument),
		"本文",
		toc.Episodes,
		func(event map[string]any) bool {
			streamEvents = append(streamEvents, event)
			return true
		},
	)
	if !ok {
		t.Fatal("reader assistant tool context stream sink should complete")
	}
	if len(requests) < 6 || len(results) < 6 {
		t.Fatalf("reader assistant should execute multiple boundary tools: requests=%+v results=%+v", requests, results)
	}
	if len(streamEvents) != len(requests)*2 || streamEvents[0]["type"] != "tool_call" || streamEvents[1]["type"] != "tool_result" {
		t.Fatalf("reader assistant should stream each tool boundary during context construction: %+v", streamEvents)
	}
	previous := results[1]["result"].(map[string]any)
	if previous["status"] != "not_available" {
		t.Fatalf("first episode should have a not_available previous episode result: %+v", previous)
	}
	var searchResult map[string]any
	for _, result := range results {
		if result["name"] == "search_episodes" {
			searchResult = result["result"].(map[string]any)
		}
	}
	if searchResult == nil || len(searchResult["matches"].([]map[string]any)) == 0 {
		t.Fatalf("search tool should find fixture body text: %+v", results)
	}
	guardedSearchResult := server.searchEpisodesResult(context.Background(), novelID, "本文", toc.Episodes, "missing")
	if guardedSearchResult["spoilerGuard"] != "closed" || len(guardedSearchResult["matches"].([]map[string]any)) != 0 {
		t.Fatalf("search tool should fail closed when the current episode is missing from TOC: %+v", guardedSearchResult)
	}
	start, end := readerAssistantRangeAround(nil, "1")
	if episodeNumberByIndex(toc.Episodes, "missing") != 0 || start != 1 || end != 1 {
		t.Fatal("reader assistant range helpers should handle missing indexes")
	}
	fakeSixEpisodeToc := []library.TocEpisodeSummary{
		{EpisodeIndex: "1", Title: "Episode 1"},
		{EpisodeIndex: "2", Title: "Episode 2"},
		{EpisodeIndex: "3", Title: "Episode 3"},
		{EpisodeIndex: "4", Title: "Episode 4"},
		{EpisodeIndex: "5", Title: "Episode 5"},
		{EpisodeIndex: "6", Title: "Episode 6"},
	}
	recentStart, recentEnd, err := resolveReaderAssistantEpisodeRange(readerAssistantContext{
		CurrentEpisodeIndex:        "6",
		CurrentEpisodeNumber:       6,
		TocEpisodes:                fakeSixEpisodeToc,
		RecentPreviousEpisodeCount: readerAssistantRecentPreviousEpisodeCount("直近5話の流れを要約して"),
	}, map[string]any{})
	if err != nil || recentStart != 1 || recentEnd != 5 {
		t.Fatalf("recent previous range should exclude current episode: start=%d end=%d err=%v", recentStart, recentEnd, err)
	}
	explicitStart, explicitEnd, err := resolveReaderAssistantEpisodeRange(readerAssistantContext{
		CurrentEpisodeIndex:        "6",
		CurrentEpisodeNumber:       6,
		TocEpisodes:                fakeSixEpisodeToc,
		RecentPreviousEpisodeCount: readerAssistantRecentPreviousEpisodeCount("直近5話の流れを要約して"),
	}, map[string]any{"startEpisodeNumber": 1, "endEpisodeNumber": 6})
	if err != nil || explicitStart != 1 || explicitEnd != 6 {
		t.Fatalf("explicit tool range should not be overridden by recent previous intent: start=%d end=%d err=%v", explicitStart, explicitEnd, err)
	}
	parsedEpisodeNumber, err := readerAssistantEpisodeNumberArg(" 4 ", 1, 6)
	if err != nil || parsedEpisodeNumber != 4 {
		t.Fatalf("episode number string should parse: number=%d err=%v", parsedEpisodeNumber, err)
	}
	floatEpisodeNumber, err := readerAssistantEpisodeNumberArg(float64(5), 1, 6)
	if err != nil || floatEpisodeNumber != 5 {
		t.Fatalf("episode number float should parse when integral: number=%d err=%v", floatEpisodeNumber, err)
	}
	if _, err := readerAssistantEpisodeNumberArg(float64(5.5), 1, 6); err == nil {
		t.Fatal("fractional episode number should be rejected")
	}
	if !readerAssistantEpisodeVisible(readerAssistantContext{CurrentEpisodeIndex: "6", TocEpisodes: fakeSixEpisodeToc}, "5") {
		t.Fatal("episode before current index should be visible")
	}
	if readerAssistantEpisodeVisible(readerAssistantContext{CurrentEpisodeIndex: "6", TocEpisodes: fakeSixEpisodeToc}, "missing") {
		t.Fatal("missing episode should not be visible")
	}
	if message := readerAssistantToolResultMessage("load_episode_range"); message != "既読範囲の本文を確認しました。" {
		t.Fatalf("tool result message should be formal completion text: %s", message)
	}
	if note := readerAssistantRecentPreviousScopeNote(readerAssistantContext{
		CurrentEpisodeIndex:        "6",
		CurrentEpisodeNumber:       6,
		TocEpisodes:                fakeSixEpisodeToc,
		RecentPreviousEpisodeCount: 5,
	}); !strings.Contains(note, "第1話〜第5話") || !strings.Contains(note, "第6話は要約対象に含めない") {
		t.Fatalf("recent previous scope note should give concrete boundaries: %s", note)
	}

	fakeTwoEpisodeToc := []library.TocEpisodeSummary{
		{EpisodeIndex: toc.Episodes[0].EpisodeIndex, Title: "Episode 1"},
		{EpisodeIndex: "2", Title: "Episode 2"},
	}
	previousReady := server.previousEpisodeResult(context.Background(), novelID, toc.Title, "2", fakeTwoEpisodeToc)
	if previousReady["status"] != "ready" {
		t.Fatalf("synthetic second episode should load first episode as previous: %+v", previousReady)
	}
	if (&Server{}).loadEpisodeRangeResult(context.Background(), novelID, toc.Title, fakeTwoEpisodeToc, 1, 2) != nil {
		t.Fatal("nil-library range load should be unavailable")
	}
	if episodeReference(nil) != nil {
		t.Fatal("nil episode reference should stay nil")
	}
	if episode, err := (&Server{}).readerAssistantEpisode(context.Background(), novelID, "1"); err != nil || episode != nil {
		t.Fatalf("nil-library episode loader should return nil without error: episode=%+v err=%v", episode, err)
	}
	emptySnapshot := (&Server{dataDir: t.TempDir()}).characterSnapshotResult("missing", "1", nil)
	if emptySnapshot["status"] != "not_generated" {
		t.Fatalf("missing character snapshot should be not_generated: %+v", emptySnapshot)
	}
	if config, err := (&Server{}).resolveReaderAssistantConfig(); err != nil || config != nil {
		t.Fatalf("nil-store reader config should be empty: config=%+v err=%v", config, err)
	}
}

func TestReaderAssistantUsageRecorderBranches(t *testing.T) {
	server := &Server{dataDir: t.TempDir()}
	if err := server.recordReaderAssistantUsage(readerAssistantUsageInput{
		RunID:                      "failed-reader-run",
		Status:                     "failed",
		NovelID:                    "novel-1",
		NovelTitle:                 "",
		CurrentEpisodeIndex:        "1",
		CurrentEpisodeNumber:       6,
		CurrentPosition:            42,
		Message:                    "hello",
		GenerationMode:             "remote",
		ToolRequests:               []map[string]any{{"name": "load_episode_range", "arguments": map[string]any{"startEpisodeNumber": 1, "endEpisodeNumber": 5}}},
		ToolResults:                []map[string]any{{"name": "load_episode_range", "result": map[string]any{"summary": strings.Repeat("長", 1200), "startEpisodeNumber": 1, "endEpisodeNumber": 5}}},
		RecentPreviousEpisodeCount: 5,
		ErrorMessage:               "provider failed",
	}); err != nil {
		t.Fatalf("recordReaderAssistantUsage returned error: %v", err)
	}
	usage, ok, err := ai.LoadUsageRun(server.aiUsageDBPath(), "failed-reader-run")
	if err != nil || !ok {
		t.Fatalf("failed reader usage should be readable: ok=%v usage=%+v err=%v", ok, usage, err)
	}
	if usage.Status != "failed" || usage.ErrorMessage == nil || usage.ToolCallCount != 1 || usage.ToolResultCount != 1 || len(usage.Requests) != 2 {
		t.Fatalf("unexpected failed reader usage: %+v", usage)
	}
	snapshot, ok := usage.Snapshot.(map[string]any)
	if !ok {
		t.Fatalf("reader usage snapshot should be a metadata object: %#v", usage.Snapshot)
	}
	readingContext, ok := snapshot["readingContext"].(map[string]any)
	if !ok || readingContext["currentPosition"] != float64(42) || readingContext["recentPreviousEpisodeCount"] != float64(5) {
		t.Fatalf("reader usage snapshot should persist debug reading context: %#v", snapshot)
	}
	recentRange, ok := readingContext["recentPreviousRange"].(map[string]any)
	if !ok || recentRange["startEpisodeNumber"] != float64(1) || recentRange["endEpisodeNumber"] != float64(5) {
		t.Fatalf("reader usage snapshot should persist recent previous range: %#v", readingContext)
	}
	toolRequests, ok := snapshot["toolRequests"].([]any)
	if !ok || len(toolRequests) != 1 {
		t.Fatalf("reader usage snapshot should persist sanitized tool requests: %#v", snapshot)
	}
	toolResults, ok := snapshot["toolResults"].([]any)
	if !ok || len(toolResults) != 1 {
		t.Fatalf("reader usage snapshot should persist sanitized tool results: %#v", snapshot)
	}
	firstToolResult := toolResults[0].(map[string]any)["result"].(map[string]any)
	if len([]rune(firstToolResult["summary"].(string))) != 1000 || firstToolResult["startEpisodeNumber"] != float64(1) {
		t.Fatalf("reader usage snapshot should truncate large tool result text while preserving range metadata: %#v", firstToolResult)
	}
	if readerAssistantUsageRecentPreviousRange(1, 5) != nil || readerAssistantUsageRecentPreviousRange(6, 0) != nil {
		t.Fatal("recent previous range should be empty without a usable current episode and count")
	}
	sanitizedList := sanitizeReaderAssistantSnapshotValue([]any{"ok", strings.Repeat("あ", 1200)}).([]any)
	if sanitizedList[0] != "ok" || len([]rune(sanitizedList[1].(string))) != 1000 {
		t.Fatalf("snapshot sanitizer should handle generic arrays and truncate strings: %#v", sanitizedList)
	}
	conversation, ok := snapshot["conversation"].(map[string]any)
	if !ok || conversation["message"] != nil || conversation["answer"] != nil || conversation["messageChars"] == nil {
		t.Fatalf("reader usage snapshot should persist only conversation metadata: %#v", snapshot)
	}
}

func TestServerAddsCORSHeadersAndHandlesPreflight(t *testing.T) {
	t.Setenv("VIEWER_API_DEV_CORS", "1")
	handler := newTestServerWithStore(store.New(t.TempDir()))
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodOptions, "/api/reader/state", nil)
	request.Header.Set("Origin", "http://localhost:5173")
	request.Header.Set("Access-Control-Request-Method", http.MethodPut)
	request.Header.Set("Access-Control-Request-Headers", "content-type, x-request-id")
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent ||
		response.Header().Get("Access-Control-Allow-Origin") != "http://localhost:5173" ||
		!strings.Contains(response.Header().Get("Access-Control-Allow-Methods"), http.MethodPut) {
		t.Fatalf("unexpected CORS preflight response: code=%d headers=%v", response.Code, response.Header())
	}
	allowHeaders := response.Header().Get("Access-Control-Allow-Headers")
	for _, headerName := range []string{apiContractVersionHeader, apiClientBuildHeader, apiRequestIDHeader} {
		if !strings.Contains(allowHeaders, headerName) {
			t.Fatalf("CORS allow headers should include %s: %q", headerName, allowHeaders)
		}
	}

	getResponse := httptest.NewRecorder()
	getRequest := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	getRequest.Header.Set("Origin", "http://localhost:5173")
	handler.ServeHTTP(getResponse, getRequest)
	if getResponse.Code != http.StatusOK || getResponse.Header().Get("Access-Control-Allow-Origin") != "http://localhost:5173" {
		t.Fatalf("CORS headers should be included on normal responses: code=%d headers=%v", getResponse.Code, getResponse.Header())
	}
	exposedHeaders := getResponse.Header().Get("Access-Control-Expose-Headers")
	for _, headerName := range []string{apiContractVersionHeader, apiContractMinVersionHeader, apiReloadRequiredHeader, apiRequestIDHeader} {
		if !strings.Contains(exposedHeaders, headerName) {
			t.Fatalf("CORS expose headers should include %s: %q", headerName, exposedHeaders)
		}
	}
	if !isAllowedCORSOrigin(nil, "https://127.0.0.1:5173") ||
		!isAllowedCORSOrigin(nil, "http://192.168.1.20:5173") ||
		isAllowedCORSOrigin(nil, "file:///tmp/app") {
		t.Fatal("default CORS origin filter should allow local/LAN HTTP(S) origins and reject non-HTTP origins")
	}
	if isAllowedCORSOrigin(nil, "://bad-origin") {
		t.Fatal("malformed CORS origins should be rejected")
	}
	sameOriginRequest := httptest.NewRequest(http.MethodPut, "/api/reader/state", nil)
	sameOriginRequest.Host = "viewer.example.test"
	if !isAllowedCORSOrigin(sameOriginRequest, "https://viewer.example.test") {
		t.Fatal("same-origin production requests should be allowed")
	}

	blockedResponse := httptest.NewRecorder()
	blockedRequest := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	blockedRequest.Host = "viewer.example.test"
	blockedRequest.Header.Set("Origin", "https://blocked.example.test")
	handler.ServeHTTP(blockedResponse, blockedRequest)
	if blockedResponse.Code != http.StatusOK || blockedResponse.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("unexpected CORS reflection for disallowed origin: code=%d headers=%v", blockedResponse.Code, blockedResponse.Header())
	}

	blockedMutation := httptest.NewRecorder()
	blockedMutationRequest := httptest.NewRequest(http.MethodPut, "/api/ai-generation/settings/preferred-mode", strings.NewReader(`{"preferredMode":"llm"}`))
	blockedMutationRequest.Host = "viewer.example.test"
	blockedMutationRequest.Header.Set("Origin", "https://blocked.example.test")
	blockedMutationRequest.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(blockedMutation, blockedMutationRequest)
	if blockedMutation.Code != http.StatusForbidden {
		t.Fatalf("unsafe CORS mutation should be rejected before routing: code=%d body=%s", blockedMutation.Code, blockedMutation.Body.String())
	}

	t.Setenv("VIEWER_API_ALLOWED_ORIGINS", "https://viewer.example.test")
	allowlistHandler := newTestServerWithStore(store.New(t.TempDir()))
	allowlistResponse := httptest.NewRecorder()
	allowlistRequest := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	allowlistRequest.Header.Set("Origin", "https://viewer.example.test")
	allowlistHandler.ServeHTTP(allowlistResponse, allowlistRequest)
	if allowlistResponse.Code != http.StatusOK || allowlistResponse.Header().Get("Access-Control-Allow-Origin") != "https://viewer.example.test" {
		t.Fatalf("configured CORS origin should be allowed: code=%d headers=%v", allowlistResponse.Code, allowlistResponse.Header())
	}
	if !isAllowedCORSOrigin(nil, "http://localhost:5173") {
		t.Fatal("configured CORS allowlist should preserve implicit localhost origins")
	}

	blockedPreflight := httptest.NewRecorder()
	blockedPreflightRequest := httptest.NewRequest(http.MethodOptions, "/api/health", nil)
	blockedPreflightRequest.Header.Set("Origin", "https://blocked.example.test")
	allowlistHandler.ServeHTTP(blockedPreflight, blockedPreflightRequest)
	if blockedPreflight.Code != http.StatusNoContent || blockedPreflight.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("blocked CORS preflight should not reflect origin: code=%d headers=%v", blockedPreflight.Code, blockedPreflight.Header())
	}
}

func TestServerCORSProductionRejectsDevelopmentFallback(t *testing.T) {
	handler := newTestServerWithStore(store.New(t.TempDir()))
	request := httptest.NewRequest(http.MethodPut, "/api/ai-generation/settings/preferred-mode", strings.NewReader(`{"preferredMode":"llm"}`))
	request.Host = "viewer.example.test"
	request.Header.Set("Origin", "http://192.168.1.20:5173")
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("private LAN origin should be rejected without VIEWER_API_DEV_CORS: code=%d body=%s", response.Code, response.Body.String())
	}
	if isAllowedCORSOrigin(nil, "http://192.168.1.20:5173") {
		t.Fatal("private LAN origin should not be an implicit production CORS origin")
	}
	sameOriginRequest := httptest.NewRequest(http.MethodPut, "/api/reader/state", nil)
	sameOriginRequest.Host = "viewer.example.test"
	if !isAllowedCORSOrigin(sameOriginRequest, "https://viewer.example.test") {
		t.Fatal("same-origin requests should still be allowed in production")
	}
	t.Setenv("VIEWER_API_ALLOWED_ORIGINS", "https://allowed.example.test")
	if !isAllowedCORSOrigin(nil, "https://allowed.example.test") {
		t.Fatal("explicit CORS allowlist should be honored in production")
	}
}

func TestServerRejectsUnsafeAPIRequestsWithoutCurrentContract(t *testing.T) {
	handler := newTestServerWithStore(store.New(t.TempDir()))
	request := httptest.NewRequest(http.MethodPut, "/api/reader/preferences", strings.NewReader(`{"theme":"paper"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusUpgradeRequired || response.Header().Get(apiReloadRequiredHeader) != "1" {
		t.Fatalf("legacy unsafe API request should ask the client to update: code=%d headers=%v body=%s", response.Code, response.Header(), response.Body.String())
	}
	requestID := response.Header().Get(apiRequestIDHeader)
	if requestID == "" {
		t.Fatalf("legacy unsafe API response should include a request id header: headers=%v body=%s", response.Header(), response.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode legacy client response: %v body=%s", err, response.Body.String())
	}
	if payload["error"] != "Client update required." || payload["code"] != "CLIENT_UPDATE_REQUIRED" || payload["message"] != "Client update required." {
		t.Fatalf("unexpected legacy client response payload: %+v", payload)
	}
	if payload["requestId"] != requestID {
		t.Fatalf("legacy client response should include matching requestId: header=%q payload=%+v", requestID, payload)
	}
}

func TestServerReturnsStructuredJSONForMissingAPIRoutes(t *testing.T) {
	handler := newTestServerWithStore(store.New(t.TempDir()))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/__missing__", nil)
	request.Header.Set(apiRequestIDHeader, "test-request-123")
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("missing API route should return 404: code=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get(apiRequestIDHeader) != "test-request-123" {
		t.Fatalf("missing API route should preserve request id header: headers=%v", recorder.Header())
	}
	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode missing API route response: %v body=%s", err, recorder.Body.String())
	}

	if response["error"] != "Not found." || response["code"] != "NOT_FOUND" || response["message"] != "Not found." {
		t.Fatalf("missing API route should return structured error: %+v", response)
	}
	if response["requestId"] != "test-request-123" {
		t.Fatalf("missing API route should include requestId: %+v", response)
	}

	exactAPIRecorder := httptest.NewRecorder()
	exactAPIRequest := httptest.NewRequest(http.MethodGet, "/api", nil)
	exactAPIRequest.Header.Set(apiRequestIDHeader, "exact-api-request")
	handler.ServeHTTP(exactAPIRecorder, exactAPIRequest)
	if exactAPIRecorder.Code != http.StatusNotFound {
		t.Fatalf("exact /api route should return 404: code=%d body=%s", exactAPIRecorder.Code, exactAPIRecorder.Body.String())
	}
	if exactAPIRecorder.Header().Get(apiRequestIDHeader) != "exact-api-request" {
		t.Fatalf("exact /api route should preserve request id header: headers=%v", exactAPIRecorder.Header())
	}
	var exactAPIResponse map[string]any
	if err := json.Unmarshal(exactAPIRecorder.Body.Bytes(), &exactAPIResponse); err != nil {
		t.Fatalf("decode exact /api response: %v body=%s", err, exactAPIRecorder.Body.String())
	}
	if exactAPIResponse["requestId"] != "exact-api-request" {
		t.Fatalf("exact /api route should include requestId: %+v", exactAPIResponse)
	}
}

func TestAPIRequestIDHelpers(t *testing.T) {
	for _, value := range []string{"request-1", "abc.DEF_123:xyz"} {
		if !isValidAPIRequestID(value) {
			t.Fatalf("request id should be valid: %q", value)
		}
	}
	for _, value := range []string{"", "contains space", "contains/slash", strings.Repeat("a", 129)} {
		if isValidAPIRequestID(value) {
			t.Fatalf("request id should be invalid: %q", value)
		}
	}

	generated := generateAPIRequestID()
	if !isValidAPIRequestID(generated) {
		t.Fatalf("generated request id should be valid: %q", generated)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	request.Header.Set(apiRequestIDHeader, "client-request-1")
	if resolved := resolveAPIRequestID(request); resolved != "client-request-1" {
		t.Fatalf("valid client request id should be preserved: %q", resolved)
	}
	request.Header.Set(apiRequestIDHeader, "bad request id")
	if resolved := resolveAPIRequestID(request); resolved == "bad request id" || !isValidAPIRequestID(resolved) {
		t.Fatalf("invalid client request id should be replaced: %q", resolved)
	}

	recorder := httptest.NewRecorder()
	request.Header.Set(apiRequestIDHeader, "client-request-2")
	ensureAPIRequestID(recorder, request)
	if recorder.Header().Get(apiRequestIDHeader) != "client-request-2" {
		t.Fatalf("ensureAPIRequestID should set preserved request id: headers=%v", recorder.Header())
	}
	ensureAPIRequestID(recorder, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if recorder.Header().Get(apiRequestIDHeader) != "client-request-2" {
		t.Fatalf("ensureAPIRequestID should not overwrite an existing response id: headers=%v", recorder.Header())
	}
}

func TestWriteJSONNormalizesLegacyErrorPayloadsWithRequestID(t *testing.T) {
	stringPayload := httptest.NewRecorder()
	stringPayload.Header().Set(apiRequestIDHeader, "legacy-string-error")
	writeJSON(stringPayload, http.StatusBadRequest, map[string]string{"error": "bad input"})
	var stringResponse map[string]any
	if err := json.Unmarshal(stringPayload.Body.Bytes(), &stringResponse); err != nil {
		t.Fatalf("decode string payload response: %v body=%s", err, stringPayload.Body.String())
	}
	if stringResponse["code"] != "BAD_REQUEST" || stringResponse["message"] != "bad input" || stringResponse["requestId"] != "legacy-string-error" {
		t.Fatalf("legacy string error payload should be normalized: %+v", stringResponse)
	}

	objectPayload := httptest.NewRecorder()
	objectPayload.Header().Set(apiRequestIDHeader, "legacy-object-error")
	writeJSON(objectPayload, http.StatusConflict, map[string]any{"error": "conflict", "resource": "reader-state"})
	var objectResponse map[string]any
	if err := json.Unmarshal(objectPayload.Body.Bytes(), &objectResponse); err != nil {
		t.Fatalf("decode object payload response: %v body=%s", err, objectPayload.Body.String())
	}
	details, _ := objectResponse["details"].(map[string]any)
	if objectResponse["code"] != "CONFLICT" ||
		objectResponse["message"] != "conflict" ||
		objectResponse["requestId"] != "legacy-object-error" ||
		details["resource"] != "reader-state" {
		t.Fatalf("legacy object error payload should be normalized with details: %+v", objectResponse)
	}
}

func TestExtractionTargetLockCleansUp(t *testing.T) {
	server := &Server{}
	runtime := server.extractionRuntime()
	unlock := runtime.LockTarget("novel-1", "1")
	if runtime.ActiveLockCount() != 1 {
		t.Fatalf("target lock should be tracked while held: %d", runtime.ActiveLockCount())
	}
	unlock()
	if runtime.ActiveLockCount() != 0 {
		t.Fatalf("target lock should be removed after release: %d", runtime.ActiveLockCount())
	}
}

func TestServerRoutesCoverContractLikePaths(t *testing.T) {
	fetcherServer := newHTTPAPIFetcherServer(t)
	t.Setenv("NOVEL_FETCHER_API_BASE_URL", fetcherServer.URL)
	dataDir := newHTTPAPITestData(t)
	stateStore := store.New(dataDir)
	if err := stateStore.Initialize(); err != nil {
		t.Fatalf("initialize store: %v", err)
	}
	if err := characters.EnsureStateDirs(filepath.Join(dataDir, "state")); err != nil {
		t.Fatalf("initialize character state dirs: %v", err)
	}
	handler := newTestServerWithLibraryAndStore(dataDir, library.NewService(filepath.Join(dataDir, "novel-fetcher")), stateStore)

	health := requestJSON(t, handler, http.MethodGet, "/api/health", nil, http.StatusOK)
	if health["service"] != "viewer-api" {
		t.Fatalf("unexpected health response: %+v", health)
	}
	systemStatus := requestJSON(t, handler, http.MethodGet, "/api/system/status", nil, http.StatusOK)
	if services, ok := systemStatus["services"].([]any); !ok || len(services) == 0 {
		t.Fatalf("unexpected system status: %+v", systemStatus)
	}
	if service := findRuntimeService(systemStatus, "go-internal-ai"); service == nil || service["summary"] != "未使用" {
		t.Fatalf("system status should include AI settings readiness: %+v", systemStatus)
	}
	storageUsage := requestJSON(t, handler, http.MethodGet, "/api/system/storage", nil, http.StatusOK)
	if storageUsage["totalBytes"].(float64) <= 0 {
		t.Fatalf("storage usage should report file bytes: %+v", storageUsage)
	}
	if novels, ok := storageUsage["novels"].([]any); !ok || len(novels) == 0 {
		t.Fatalf("storage usage should include novel breakdown: %+v", storageUsage)
	}
	storageProgress := requestJSON(t, handler, http.MethodGet, "/api/system/storage/progress", nil, http.StatusOK)
	if storageProgress["state"] != "completed" || storageProgress["phase"] != "completed" {
		t.Fatalf("storage progress should reflect completed scan: %+v", storageProgress)
	}
	if storageProgress["checkedNovels"].(float64) < 1 || storageProgress["totalNovels"].(float64) < 1 {
		t.Fatalf("storage progress should include rough novel counts: %+v", storageProgress)
	}

	novels := requestJSON(t, handler, http.MethodGet, "/api/library/novels", nil, http.StatusOK)
	novelItems := novels["novels"].([]any)
	if len(novelItems) != 1 {
		t.Fatalf("expected one novel, got %+v", novels)
	}
	novelID := novelItems[0].(map[string]any)["novelId"].(string)
	toc := requestJSON(t, handler, http.MethodGet, "/api/library/novels/"+novelID+"/toc", nil, http.StatusOK)
	episodes := toc["episodes"].([]any)
	episodeIndex := episodes[0].(map[string]any)["episodeIndex"].(string)
	episode := requestJSON(t, handler, http.MethodGet, "/api/library/novels/"+novelID+"/episodes/"+episodeIndex, nil, http.StatusOK)
	if episode["title"] != "Episode 1" {
		t.Fatalf("unexpected episode: %+v", episode)
	}
	if episode["contentEtag"] != "hash-1-reader-corrections-q1h1p1a1" {
		t.Fatalf("episode response etag should include reader correction settings: %+v", episode)
	}
	etagResponse := httptest.NewRecorder()
	etagRequest := httptest.NewRequest(http.MethodGet, "/api/library/novels/"+novelID+"/episodes/"+episodeIndex, nil)
	etagRequest.Header.Set("If-None-Match", `"hash-1-reader-corrections-q1h1p1a1"`)
	handler.ServeHTTP(etagResponse, etagRequest)
	if etagResponse.Code != http.StatusNotModified {
		t.Fatalf("expected 304 for matching etag, got %d body=%s", etagResponse.Code, etagResponse.Body.String())
	}
	if etagResponse.Header().Get("ETag") != `"hash-1-reader-corrections-q1h1p1a1"` {
		t.Fatalf("304 response should include etag, got %q", etagResponse.Header().Get("ETag"))
	}
	staleEtagResponse := httptest.NewRecorder()
	staleEtagRequest := httptest.NewRequest(http.MethodGet, "/api/library/novels/"+novelID+"/episodes/"+episodeIndex, nil)
	staleEtagRequest.Header.Set("If-None-Match", `"hash-1"`)
	handler.ServeHTTP(staleEtagResponse, staleEtagRequest)
	if staleEtagResponse.Code != http.StatusOK {
		t.Fatalf("raw content etag should be stale when reader settings are part of the response etag, got %d body=%s", staleEtagResponse.Code, staleEtagResponse.Body.String())
	}
	defaultReaderSettings := requestJSON(t, handler, http.MethodGet, "/api/library/novels/"+novelID+"/reader-settings", nil, http.StatusOK)
	defaultCorrection := defaultReaderSettings["correction"].(map[string]any)
	if defaultReaderSettings["novelId"] != novelID || defaultCorrection["quoteNormalization"] != true || defaultCorrection["hyphenDashNormalization"] != true || defaultCorrection["parenthesisNormalization"] != true || defaultCorrection["halfwidthAlnumPunctuationNormalization"] != true {
		t.Fatalf("unexpected default novel reader settings: %+v", defaultReaderSettings)
	}
	partialUpdatedReaderSettings := requestJSON(t, handler, http.MethodPut, "/api/library/novels/"+novelID+"/reader-settings", map[string]any{
		"correction": map[string]any{"quoteNormalization": false},
	}, http.StatusOK)
	partialUpdatedCorrection := partialUpdatedReaderSettings["correction"].(map[string]any)
	if partialUpdatedCorrection["quoteNormalization"] != false || partialUpdatedCorrection["hyphenDashNormalization"] != true || partialUpdatedCorrection["parenthesisNormalization"] != true || partialUpdatedCorrection["halfwidthAlnumPunctuationNormalization"] != true {
		t.Fatalf("partial novel reader settings update should preserve omitted fields: %+v", partialUpdatedReaderSettings)
	}
	correctedEpisode := requestJSON(t, handler, http.MethodGet, "/api/library/novels/"+novelID+"/episodes/"+episodeIndex, nil, http.StatusOK)
	if correctedEpisode["contentEtag"] != "hash-1-reader-corrections-q0h1p1a1" {
		t.Fatalf("corrected episode should include correction etag key: %+v", correctedEpisode)
	}
	halfwidthUpdatedReaderSettings := requestJSON(t, handler, http.MethodPut, "/api/library/novels/"+novelID+"/reader-settings", map[string]any{
		"correction": map[string]any{"halfwidthAlnumPunctuationNormalization": false},
	}, http.StatusOK)
	halfwidthUpdatedCorrection := halfwidthUpdatedReaderSettings["correction"].(map[string]any)
	if halfwidthUpdatedCorrection["quoteNormalization"] != false || halfwidthUpdatedCorrection["hyphenDashNormalization"] != true || halfwidthUpdatedCorrection["parenthesisNormalization"] != true || halfwidthUpdatedCorrection["halfwidthAlnumPunctuationNormalization"] != false {
		t.Fatalf("halfwidth partial update should preserve omitted fields: %+v", halfwidthUpdatedReaderSettings)
	}
	halfwidthDisabledEpisode := requestJSON(t, handler, http.MethodGet, "/api/library/novels/"+novelID+"/episodes/"+episodeIndex, nil, http.StatusOK)
	if halfwidthDisabledEpisode["contentEtag"] != "hash-1-reader-corrections-q0h1p1a0" {
		t.Fatalf("halfwidth disabled episode should include a0 correction etag key: %+v", halfwidthDisabledEpisode)
	}
	requestRaw(t, handler, http.MethodGet, "/api/library/novels/"+novelID+"/assets/assets/episodes/1/pic.jpg", nil, http.StatusOK)
	requestJSON(t, handler, http.MethodGet, "/api/library/novels/"+novelID+"/assets/missing.png", nil, http.StatusNotFound)

	requestJSON(t, handler, http.MethodGet, "/api/reader/state", nil, http.StatusBadRequest)
	readerState := requestJSON(t, handler, http.MethodPut, "/api/reader/state", map[string]any{
		"novelId":              novelID,
		"lastReadEpisodeIndex": episodeIndex,
		"position":             4,
		"scroll":               map[string]any{"type": "ratio", "value": 0.5},
		"clientId":             "test",
		"expectedStateVersion": 0,
	}, http.StatusOK)
	if readerState["position"].(float64) != 4 {
		t.Fatalf("unexpected reader state: %+v", readerState)
	}
	conflictBody, err := json.Marshal(map[string]any{
		"novelId":              novelID,
		"lastReadEpisodeIndex": episodeIndex,
		"position":             99,
		"scroll":               nil,
		"clientId":             "stale-client",
		"expectedStateVersion": 0,
	})
	if err != nil {
		t.Fatalf("marshal conflict reader state body: %v", err)
	}
	conflictRequest := httptest.NewRequest(http.MethodPut, "/api/reader/state", bytes.NewReader(conflictBody))
	conflictRequest.Header.Set("Content-Type", "application/json")
	conflictRequest.Header.Set(apiRequestIDHeader, "reader-conflict-1")
	setTestAPIContractHeaders(conflictRequest)
	conflictResponse := httptest.NewRecorder()
	handler.ServeHTTP(conflictResponse, conflictRequest)
	if conflictResponse.Code != http.StatusConflict {
		t.Fatalf("conflict reader state status=%d want=%d body=%s", conflictResponse.Code, http.StatusConflict, conflictResponse.Body.String())
	}
	var conflictReaderState map[string]any
	if err := json.Unmarshal(conflictResponse.Body.Bytes(), &conflictReaderState); err != nil {
		t.Fatalf("decode conflict reader state: %v body=%s", err, conflictResponse.Body.String())
	}
	if conflictReaderState["position"].(float64) != 4 || conflictReaderState["stateVersion"].(float64) != readerState["stateVersion"].(float64) {
		t.Fatalf("conflict response should return current reader state without writing: %+v", conflictReaderState)
	}
	if conflictResponse.Header().Get(apiRequestIDHeader) != "reader-conflict-1" || conflictReaderState["requestId"] != "reader-conflict-1" {
		t.Fatalf("conflict response should include matching requestId: header=%q payload=%+v", conflictResponse.Header().Get(apiRequestIDHeader), conflictReaderState)
	}
	requestJSON(t, handler, http.MethodPut, "/api/reader/state", map[string]any{
		"novelId":              "missing",
		"lastReadEpisodeIndex": episodeIndex,
		"position":             1,
		"scroll":               nil,
		"clientId":             "missing-client",
		"expectedStateVersion": 0,
	}, http.StatusNotFound)
	preferences := requestJSON(t, handler, http.MethodPut, "/api/reader/preferences", map[string]any{
		"readingMode": "horizontal",
		"fontFamily":  "gothic",
		"theme":       "forest",
	}, http.StatusOK)
	if preferences["theme"] != "forest" {
		t.Fatalf("unexpected preferences: %+v", preferences)
	}
	bookmark := requestJSON(t, handler, http.MethodPost, "/api/bookmarks", map[string]any{
		"novelId":      novelID,
		"episodeIndex": episodeIndex,
		"position":     1,
		"label":        "mark",
	}, http.StatusCreated)
	requestJSON(t, handler, http.MethodGet, "/api/bookmarks?novelId="+novelID, nil, http.StatusOK)
	mergedNovels := requestJSON(t, handler, http.MethodGet, "/api/library/novels", nil, http.StatusOK)
	mergedNovel := mergedNovels["novels"].([]any)[0].(map[string]any)
	if mergedNovel["lastReadEpisodeIndex"] != episodeIndex || mergedNovel["lastReadEpisodeTitle"] != "Episode 1" || mergedNovel["latestBookmarkEpisodeIndex"] != episodeIndex || mergedNovel["bookmarkCount"].(float64) != 1 {
		t.Fatalf("library list should include reader state and bookmark summaries: %+v", mergedNovel)
	}
	requestJSON(t, handler, http.MethodDelete, "/api/bookmarks/"+bookmark["id"].(string), nil, http.StatusOK)

	characters := requestJSON(t, handler, http.MethodGet, "/api/library/novels/"+novelID+"/characters?upToEpisodeIndex="+episodeIndex, nil, http.StatusOK)
	if characters["status"] != "ready" {
		t.Fatalf("unexpected characters response: %+v", characters)
	}
	createdJob := requestJSON(t, handler, http.MethodPost, "/api/library/novels/"+novelID+"/extraction-jobs", map[string]any{
		"upToEpisodeIndex": episodeIndex,
	}, http.StatusAccepted)
	if createdJob["status"] != "queued" {
		t.Fatalf("Go character job should be queued before the worker processes it: %+v", createdJob)
	}
	completedJob := waitForCharacterJobStatus(t, handler, novelID, createdJob["jobId"].(string), "completed")
	if completedJob["startedAt"] == nil || completedJob["finishedAt"] == nil || completedJob["errorMessage"] != nil {
		t.Fatalf("completed character job should include lifecycle metadata: %+v", completedJob)
	}
	if completedJob["progress"] != float64(100) || completedJob["progressStage"] != "completed" {
		t.Fatalf("completed character job should expose worker progress metadata: %+v", completedJob)
	}
	requestJSON(t, handler, http.MethodGet, "/api/library/novels/"+novelID+"/extraction-jobs", nil, http.StatusOK)

	requestJSON(t, handler, http.MethodGet, "/api/ai-generation/settings", nil, http.StatusOK)
	requestJSON(t, handler, http.MethodPut, "/api/ai-generation/settings/preferred-mode", map[string]any{"preferredMode": "llm"}, http.StatusOK)
	jobs := requestJSON(t, handler, http.MethodGet, "/api/ai-generation/jobs", nil, http.StatusOK)
	if firstJob := jobs["jobs"].([]any)[0].(map[string]any); firstJob["novelTitle"] != "Fixture Novel" || firstJob["novelAuthor"] != "Author" {
		t.Fatalf("AI job list should include library metadata: %+v", firstJob)
	}
	requestJSON(t, handler, http.MethodGet, "/api/ai-generation/usage", nil, http.StatusOK)
	requestJSON(t, handler, http.MethodGet, "/api/ai-generation/usage/run-http", nil, http.StatusOK)
	requestJSON(t, handler, http.MethodGet, "/api/ai-generation/usage/go-fixture-usage-run", nil, http.StatusNotFound)
	stream := requestRaw(t, handler, http.MethodPost, "/api/ai-generation/playground/extraction/stream", map[string]any{
		"novelId":          novelID,
		"upToEpisodeIndex": 1,
	}, http.StatusOK)
	if !strings.Contains(stream, `"type":"result"`) ||
		!strings.Contains(stream, `"type":"promptPreview"`) ||
		!strings.Contains(stream, `"type":"batchTiming"`) ||
		!strings.Contains(stream, `"stage":"buildingResponse"`) ||
		!strings.Contains(stream, `"message":"入力を確認しました。"`) ||
		!strings.Contains(stream, `"progress":10`) {
		t.Fatalf("unexpected stream body: %s", stream)
	}

	fetcherStatus := requestJSON(t, handler, http.MethodGet, "/api/fetcher/status", nil, http.StatusOK)
	if tasks := fetcherStatus["tasks"].(map[string]any); tasks["completedCount"] != float64(1) || len(tasks["recentCompleted"].([]any)) != 1 {
		t.Fatalf("unexpected canonical fetcher status tasks: %+v", tasks)
	}
	fetcherSummary := requestJSON(t, handler, http.MethodGet, "/api/fetcher/tasks/summary", nil, http.StatusOK)
	if fetcherSummary["completedCount"] != float64(1) || fetcherSummary["completed_count"] != nil {
		t.Fatalf("unexpected canonical fetcher task summary: %+v", fetcherSummary)
	}
	update := requestJSON(t, handler, http.MethodPost, "/api/fetcher/works/update", map[string]any{"novelIds": []string{novelID}}, http.StatusAccepted)
	if update["message"] != "Update started" ||
		len(update["novelIds"].([]any)) != 1 ||
		update["novelIds"].([]any)[0] != novelID ||
		len(update["fetcherWorkIds"].([]any)) != 1 ||
		update["fetcherWorkIds"].([]any)[0] != "1" {
		t.Fatalf("unexpected update response: %+v", update)
	}
	resume := requestJSON(t, handler, http.MethodPost, "/api/fetcher/works/resume", map[string]any{"novelIds": []string{novelID}}, http.StatusAccepted)
	if resume["message"] != "Resume started" ||
		len(resume["novelIds"].([]any)) != 1 ||
		resume["novelIds"].([]any)[0] != novelID ||
		len(resume["fetcherWorkIds"].([]any)) != 1 ||
		resume["fetcherWorkIds"].([]any)[0] != "1" {
		t.Fatalf("unexpected resume response: %+v", resume)
	}
	remove := requestJSON(t, handler, http.MethodPost, "/api/fetcher/works/remove", map[string]any{"novelIds": []string{novelID}}, http.StatusAccepted)
	if remove["message"] != "Novel removed" || len(remove["fetcherWorkIds"].([]any)) != 1 {
		t.Fatalf("unexpected remove response: %+v", remove)
	}
	cancel := requestJSON(t, handler, http.MethodPost, "/api/fetcher/tasks/task-1/cancel", nil, http.StatusOK)
	if cancel["message"] != "Task cancelled" || cancel["taskId"] != "task-1" || cancel["cancelled"] != true {
		t.Fatalf("unexpected cancel response: %+v", cancel)
	}
}

func TestServerFetcherRemoveDefaultsWithFilesToTrue(t *testing.T) {
	var capturedWithFiles any
	var capturedIDs []any
	fetcherServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v2/novels/remove" {
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			capturedWithFiles = body["with_files"]
			capturedIDs = body["ids"].([]any)
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{}})
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": map[string]any{"message": "missing"}})
	}))
	t.Cleanup(fetcherServer.Close)
	t.Setenv("NOVEL_FETCHER_API_BASE_URL", fetcherServer.URL)

	dataDir := newHTTPAPITestData(t)
	stateStore := store.New(dataDir)
	if err := stateStore.Initialize(); err != nil {
		t.Fatalf("initialize store: %v", err)
	}
	if err := characters.EnsureStateDirs(filepath.Join(dataDir, "state")); err != nil {
		t.Fatalf("initialize character state dirs: %v", err)
	}
	handler := newTestServerWithLibraryAndStore(dataDir, library.NewService(filepath.Join(dataDir, "novel-fetcher")), stateStore)
	novels := requestJSON(t, handler, http.MethodGet, "/api/library/novels", nil, http.StatusOK)
	novelID := novels["novels"].([]any)[0].(map[string]any)["novelId"].(string)
	remove := requestJSON(t, handler, http.MethodPost, "/api/fetcher/works/remove", map[string]any{"novelIds": []string{novelID}}, http.StatusAccepted)
	if capturedWithFiles != true {
		t.Fatalf("remove should forward with_files=true by default, got %#v", capturedWithFiles)
	}
	if remove["message"] != "Novel removal started" || remove["withFiles"] != true || len(remove["novelIds"].([]any)) != 1 || len(remove["fetcherWorkIds"].([]any)) != 1 {
		t.Fatalf("unexpected remove response: %+v", remove)
	}
	capturedWithFiles = nil
	requestJSON(t, handler, http.MethodPost, "/api/fetcher/works/remove", map[string]any{"novelIds": []string{novelID}, "withFiles": false}, http.StatusAccepted)
	if capturedWithFiles != false {
		t.Fatalf("remove should preserve explicit withFiles=false, got %#v", capturedWithFiles)
	}
	duplicated := requestJSON(t, handler, http.MethodPost, "/api/fetcher/works/remove", map[string]any{"novelIds": []string{novelID, " " + novelID + " "}}, http.StatusAccepted)
	if len(capturedIDs) != 1 || len(duplicated["novelIds"].([]any)) != 1 {
		t.Fatalf("remove should trim and dedupe novelIds before forwarding: ids=%+v response=%+v", capturedIDs, duplicated)
	}
}

func TestServerFetcherRemovePrunesViewerState(t *testing.T) {
	fetcherServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v2/novels/remove" {
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{}})
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": map[string]any{"message": "missing"}})
	}))
	t.Cleanup(fetcherServer.Close)
	t.Setenv("NOVEL_FETCHER_API_BASE_URL", fetcherServer.URL)

	dataDir := newHTTPAPITestData(t)
	stateStore := store.New(dataDir)
	if err := stateStore.Initialize(); err != nil {
		t.Fatalf("initialize store: %v", err)
	}
	if err := characters.EnsureStateDirs(filepath.Join(dataDir, "state")); err != nil {
		t.Fatalf("initialize character state dirs: %v", err)
	}
	handler := newTestServerWithLibraryAndStore(dataDir, library.NewService(filepath.Join(dataDir, "novel-fetcher")), stateStore)
	server := handler.(*Server)
	novels := requestJSON(t, handler, http.MethodGet, "/api/library/novels", nil, http.StatusOK)
	novelID := novels["novels"].([]any)[0].(map[string]any)["novelId"].(string)
	writeHTTPFixtureFile(t, filepath.Join(server.stateDir(), "character_events", novelID+".yaml"), `
schema_version: 1
novel_id: `+novelID+`
processed_up_to_episode_index: "1"
next_character_ordinal: 1
characters: []
`)
	characterJobIndexDir := filepath.Join(server.stateDir(), "extraction_jobs", "index")
	if err := os.MkdirAll(characterJobIndexDir, 0o755); err != nil {
		t.Fatalf("mkdir character job index fixture: %v", err)
	}
	writeHTTPFixtureFile(t, filepath.Join(characterJobIndexDir, novelID+".yaml"), `
schema_version: 2
revision: 1
novel_id: `+novelID+`
active_job_id: null
job_ids:
  - job-1
`)

	episodeIndex := "1"
	if _, err := stateStore.PutReadingState(store.ReadingStatePutInput{ReadingState: store.ReadingState{NovelID: novelID, LastReadEpisodeIndex: &episodeIndex, Position: 42}}); err != nil {
		t.Fatalf("PutReadingState returned error: %v", err)
	}
	if _, err := stateStore.CreateBookmark(store.Bookmark{NovelID: novelID, EpisodeIndex: "1", Position: 12}); err != nil {
		t.Fatalf("CreateBookmark returned error: %v", err)
	}
	if err := server.saveExtractionCheckpoint(novelID, "1", extractionCheckpoint{
		SchemaVersion:           1,
		NovelID:                 novelID,
		UpToEpisodeIndex:        "1",
		ProcessedEpisodeIndexes: []string{"1"},
		Characters:              []characters.GeneratedCharacter{},
		UpdatedAt:               "2026-01-01T00:00:00Z",
	}); err != nil {
		t.Fatalf("save checkpoint: %v", err)
	}
	targetUsageNovelID := novelID
	if err := ai.SaveUsageRun(server.aiUsageDBPath(), ai.UsageRun{
		RunID:          "run-target-remove",
		Feature:        "extraction",
		WorkflowName:   "Extraction",
		Status:         "completed",
		StartedAt:      "2026-01-01T00:00:00Z",
		FinishedAt:     "2026-01-01T00:00:01Z",
		ElapsedMs:      1000,
		NovelID:        &targetUsageNovelID,
		GenerationMode: "heuristic",
		RequestCount:   1,
		InputTokens:    1,
		OutputTokens:   2,
		TotalTokens:    3,
		Requests: []ai.UsageRequest{{
			RequestIndex:  1,
			Kind:          "chat",
			ToolNames:     []string{},
			ToolSummaries: []string{},
			InputTokens:   1,
			OutputTokens:  2,
			TotalTokens:   3,
		}},
		Snapshot: map[string]any{"runId": "run-target-remove"},
	}); err != nil {
		t.Fatalf("SaveUsageRun target returned error: %v", err)
	}

	response := requestJSON(t, handler, http.MethodPost, "/api/fetcher/works/remove", map[string]any{"novelIds": []string{novelID}}, http.StatusAccepted)
	if response["viewerStateCleanupStatus"] != "ok" {
		t.Fatalf("cleanup status should be ok: %+v", response)
	}
	cleanup := response["viewerStateCleanup"].(map[string]any)
	expectedCounts := map[string]float64{
		"readingStatesDeleted":         1,
		"bookmarksDeleted":             1,
		"characterEventsDeleted":       1,
		"characterProfilesDeleted":     1,
		"extractionJobsDeleted":        1,
		"extractionJobIndexesDeleted":  1,
		"extractionCheckpointsDeleted": 1,
		"aiUsageRunsDeleted":           1,
	}
	for key, expected := range expectedCounts {
		if cleanup[key] != expected {
			t.Fatalf("unexpected cleanup count %s: got=%#v cleanup=%+v", key, cleanup[key], cleanup)
		}
	}
	if state, err := stateStore.GetReadingState(novelID); err != nil || state.Position != 0 || state.LastReadEpisodeIndex != nil || state.StateVersion != 2 {
		t.Fatalf("reader state should be tombstoned: state=%+v err=%v", state, err)
	}
	staleReaderStateBody := bytes.NewBufferString(`{"novelId":"` + novelID + `","lastReadEpisodeIndex":"1","position":99,"expectedStateVersion":0}`)
	staleReaderStateRequest := httptest.NewRequest(http.MethodPut, "/api/reader/state", staleReaderStateBody)
	staleReaderStateRequest.Header.Set("Content-Type", "application/json")
	setTestAPIContractHeaders(staleReaderStateRequest)
	staleReaderStateResponse := httptest.NewRecorder()
	handler.ServeHTTP(staleReaderStateResponse, staleReaderStateRequest)
	if staleReaderStateResponse.Code != http.StatusConflict && staleReaderStateResponse.Code != http.StatusNotFound {
		t.Fatalf("stale reader state write after remove should be rejected, got %d body=%s", staleReaderStateResponse.Code, staleReaderStateResponse.Body.String())
	}
	if state, err := stateStore.GetReadingState(novelID); err != nil || state.Position != 0 || state.LastReadEpisodeIndex != nil || state.StateVersion != 2 {
		t.Fatalf("stale write should not revive tombstoned reader state: state=%+v err=%v", state, err)
	}
	if bookmarks, err := stateStore.ListBookmarks(novelID); err != nil || len(bookmarks) != 0 {
		t.Fatalf("bookmarks should be pruned: bookmarks=%+v err=%v", bookmarks, err)
	}
	requestJSON(t, handler, http.MethodPut, "/api/library/novels/"+novelID+"/reader-settings", map[string]any{
		"correction": map[string]any{"quoteNormalization": false},
	}, http.StatusGone)
	requestJSON(t, handler, http.MethodPost, "/api/bookmarks", map[string]any{
		"novelId":      novelID,
		"episodeIndex": "1",
		"position":     12,
	}, http.StatusGone)
	if _, err := os.Stat(filepath.Join(server.stateDir(), "character_profiles", novelID+".yaml")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("character profile should be pruned: %v", err)
	}
	if jobs, ok, err := extractdomain.LoadJobs(server.stateDir(), novelID); err != nil || ok || len(jobs) != 0 {
		t.Fatalf("character jobs should be pruned: ok=%v jobs=%+v err=%v", ok, jobs, err)
	}
	if server.extractionCheckpointExists(novelID, "1") {
		t.Fatal("character checkpoint should be pruned")
	}
	if _, ok, err := ai.LoadUsageRun(server.aiUsageDBPath(), "run-target-remove"); err != nil || ok {
		t.Fatalf("target usage run should be pruned: ok=%v err=%v", ok, err)
	}
	if _, ok, err := ai.LoadUsageRun(server.aiUsageDBPath(), "run-http"); err != nil || !ok {
		t.Fatalf("unrelated usage run should remain: ok=%v err=%v", ok, err)
	}
}

func TestServerFetcherRemoveKeepsAcceptedAndDoesNotPartiallyPruneWhenViewerStatePreflightFails(t *testing.T) {
	var removeCalled bool
	fetcherServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v2/novels/remove" {
			removeCalled = true
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{}})
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": map[string]any{"message": "missing"}})
	}))
	t.Cleanup(fetcherServer.Close)
	t.Setenv("NOVEL_FETCHER_API_BASE_URL", fetcherServer.URL)

	dataDir := newHTTPAPITestData(t)
	stateStore := store.New(dataDir)
	if err := stateStore.Initialize(); err != nil {
		t.Fatalf("initialize store: %v", err)
	}
	handler := newTestServerWithLibraryAndStore(dataDir, library.NewService(filepath.Join(dataDir, "novel-fetcher")), stateStore)
	server := handler.(*Server)
	novels := requestJSON(t, handler, http.MethodGet, "/api/library/novels", nil, http.StatusOK)
	novelID := novels["novels"].([]any)[0].(map[string]any)["novelId"].(string)
	if err := os.WriteFile(server.aiUsageDBPath(), []byte("not sqlite"), 0o600); err != nil {
		t.Fatalf("corrupt ai usage db: %v", err)
	}

	response := requestJSON(t, handler, http.MethodPost, "/api/fetcher/works/remove", map[string]any{"novelIds": []string{novelID}}, http.StatusAccepted)
	if !removeCalled {
		t.Fatal("fetcher remove should be called before cleanup")
	}
	if response["viewerStateCleanupStatus"] != "partial" || response["viewerStateCleanupError"] != "Failed to clean up removed novel state." {
		t.Fatalf("cleanup failure should be reported without failing remove: %+v", response)
	}
	cleanup := response["viewerStateCleanup"].(map[string]any)
	if cleanup["characterProfilesDeleted"] != float64(0) || cleanup["extractionJobsDeleted"] != float64(0) {
		t.Fatalf("operation-wide preflight failure should report zero mutations: %+v", cleanup)
	}
}

func TestServerFetcherDownloadNormalizesTargets(t *testing.T) {
	var capturedTargets []any
	fetcherServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v2/novels/download" {
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			capturedTargets = body["targets"].([]any)
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{}})
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": map[string]any{"message": "missing"}})
	}))
	t.Cleanup(fetcherServer.Close)
	t.Setenv("NOVEL_FETCHER_API_BASE_URL", fetcherServer.URL)

	dataDir := newHTTPAPITestData(t)
	stateStore := store.New(dataDir)
	if err := stateStore.Initialize(); err != nil {
		t.Fatalf("initialize store: %v", err)
	}
	handler := newTestServerWithLibraryAndStore(dataDir, library.NewService(filepath.Join(dataDir, "novel-fetcher")), stateStore)
	target := "https://example.test/novel"
	response := requestJSON(t, handler, http.MethodPost, "/api/fetcher/works/download", map[string]any{"targets": []string{target, " " + target + " "}}, http.StatusAccepted)
	if len(capturedTargets) != 1 || capturedTargets[0] != target || len(response["targets"].([]any)) != 1 {
		t.Fatalf("download should trim and dedupe targets before forwarding: targets=%+v response=%+v", capturedTargets, response)
	}
}

func TestServerFetcherPreservesFetcherHTTPStatuses(t *testing.T) {
	fetcherServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v2/novels/update":
			w.WriteHeader(http.StatusNotImplemented)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": "update options unsupported"}})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v2/tasks/missing-task/cancel":
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": "task not found"}})
		default:
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": map[string]any{"message": "missing"}})
		}
	}))
	t.Cleanup(fetcherServer.Close)
	t.Setenv("NOVEL_FETCHER_API_BASE_URL", fetcherServer.URL)

	dataDir := newHTTPAPITestData(t)
	stateStore := store.New(dataDir)
	if err := stateStore.Initialize(); err != nil {
		t.Fatalf("initialize store: %v", err)
	}
	handler := newTestServerWithLibraryAndStore(dataDir, library.NewService(filepath.Join(dataDir, "novel-fetcher")), stateStore)
	novels := requestJSON(t, handler, http.MethodGet, "/api/library/novels", nil, http.StatusOK)
	novelID := novels["novels"].([]any)[0].(map[string]any)["novelId"].(string)
	update := requestJSON(t, handler, http.MethodPost, "/api/fetcher/works/update", map[string]any{"novelIds": []string{novelID}}, http.StatusNotImplemented)
	if update["error"] != "update options unsupported" {
		t.Fatalf("update should preserve fetcher 501 message: %+v", update)
	}
	cancel := requestJSON(t, handler, http.MethodPost, "/api/fetcher/tasks/missing-task/cancel", nil, http.StatusNotFound)
	if cancel["error"] != "task not found" {
		t.Fatalf("cancel should preserve fetcher 404 message: %+v", cancel)
	}
}

func TestMethodAllowHeadersForMultiMethodRoutes(t *testing.T) {
	dataDir := newHTTPAPITestData(t)
	stateStore := store.New(dataDir)
	if err := stateStore.Initialize(); err != nil {
		t.Fatalf("initialize store: %v", err)
	}
	handler := newTestServerWithLibraryAndStore(dataDir, library.NewService(filepath.Join(dataDir, "novel-fetcher")), stateStore)
	novels := requestJSON(t, handler, http.MethodGet, "/api/library/novels", nil, http.StatusOK)
	novelID := novels["novels"].([]any)[0].(map[string]any)["novelId"].(string)
	cases := []struct {
		method string
		path   string
		allow  string
	}{
		{http.MethodPost, "/api/reader/state", "GET, PUT"},
		{http.MethodPost, "/api/reader/preferences", "GET, PUT"},
		{http.MethodPut, "/api/bookmarks", "GET, POST"},
		{http.MethodPost, "/api/library/novels/" + novelID + "/characters?upToEpisodeIndex=1", "GET"},
		{http.MethodPost, "/api/library/novels/" + novelID + "/terms?upToEpisodeIndex=1", "GET"},
		{http.MethodPut, "/api/library/novels/" + novelID + "/extraction-jobs", "GET, POST"},
	}
	for _, tc := range cases {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(tc.method, tc.path, nil)
		setTestAPIContractHeaders(request)
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != tc.allow {
			t.Fatalf("%s %s code=%d allow=%q want code=%d allow=%q", tc.method, tc.path, response.Code, response.Header().Get("Allow"), http.StatusMethodNotAllowed, tc.allow)
		}
	}
}

func TestServerValidationAndErrorPaths(t *testing.T) {
	fetcherServer := newHTTPAPIFetcherServer(t)
	t.Setenv("NOVEL_FETCHER_API_BASE_URL", fetcherServer.URL)
	dataDir := newHTTPAPITestData(t)
	stateStore := store.New(dataDir)
	if err := stateStore.Initialize(); err != nil {
		t.Fatalf("initialize store: %v", err)
	}
	handler := newTestServerWithLibraryAndStore(dataDir, library.NewService(filepath.Join(dataDir, "novel-fetcher")), stateStore)
	novels := requestJSON(t, handler, http.MethodGet, "/api/library/novels", nil, http.StatusOK)
	novelID := novels["novels"].([]any)[0].(map[string]any)["novelId"].(string)

	requestJSON(t, handler, http.MethodPost, "/api/health", nil, http.StatusMethodNotAllowed)
	requestJSON(t, handler, http.MethodPost, "/api/system/storage", nil, http.StatusMethodNotAllowed)
	requestJSON(t, handler, http.MethodPost, "/api/system/storage/progress", nil, http.StatusMethodNotAllowed)
	requestJSON(t, handler, http.MethodGet, "/api/library/novels/missing/toc", nil, http.StatusNotFound)
	requestJSON(t, handler, http.MethodPost, "/api/library/novels", nil, http.StatusMethodNotAllowed)
	requestJSON(t, handler, http.MethodPost, "/api/library/novels/"+novelID+"/toc", nil, http.StatusMethodNotAllowed)
	requestJSON(t, handler, http.MethodPost, "/api/library/novels/"+novelID+"/episodes/1", nil, http.StatusMethodNotAllowed)
	requestJSON(t, handler, http.MethodPost, "/api/library/novels/"+novelID+"/assets/assets/episodes/1/pic.jpg", nil, http.StatusMethodNotAllowed)
	requestJSON(t, handler, http.MethodGet, "/api/library/novels/"+novelID+"/episodes/not-number", nil, http.StatusBadRequest)
	requestJSON(t, handler, http.MethodGet, "/api/library/novels/"+novelID+"/episodes/999", nil, http.StatusNotFound)
	requestJSON(t, handler, http.MethodGet, "/api/library/novels/"+novelID+"/characters", nil, http.StatusBadRequest)
	requestJSON(t, handler, http.MethodPost, "/api/library/novels/"+novelID+"/characters?upToEpisodeIndex=1", nil, http.StatusMethodNotAllowed)
	requestJSON(t, handler, http.MethodGet, "/api/library/novels/"+novelID+"/characters?upToEpisodeIndex=999", nil, http.StatusBadRequest)
	requestJSON(t, handler, http.MethodGet, "/api/library/novels/"+novelID+"/characters?upToEpisodeIndex=0", nil, http.StatusBadRequest)
	requestJSON(t, handler, http.MethodGet, "/api/library/novels/"+novelID+"/terms", nil, http.StatusBadRequest)
	requestJSON(t, handler, http.MethodPost, "/api/library/novels/"+novelID+"/terms?upToEpisodeIndex=1", nil, http.StatusMethodNotAllowed)
	requestJSON(t, handler, http.MethodGet, "/api/library/novels/"+novelID+"/terms?upToEpisodeIndex=999", nil, http.StatusBadRequest)
	requestJSON(t, handler, http.MethodGet, "/api/library/novels/missing/terms?upToEpisodeIndex=1", nil, http.StatusNotFound)
	requestJSON(t, handler, http.MethodGet, "/api/library/novels/missing/extraction-jobs", nil, http.StatusNotFound)
	requestJSON(t, handler, http.MethodPost, "/api/library/novels/"+novelID+"/extraction-jobs", map[string]any{}, http.StatusBadRequest)
	requestJSON(t, handler, http.MethodPost, "/api/library/novels/"+novelID+"/extraction-jobs", map[string]any{
		"upToEpisodeIndex": "999",
	}, http.StatusBadRequest)
	requestJSON(t, handler, http.MethodPost, "/api/library/novels/"+novelID+"/extraction-jobs", map[string]any{
		"upToEpisodeIndex": 0,
	}, http.StatusBadRequest)

	requestJSON(t, handler, http.MethodPost, "/api/library/novels/"+novelID+"/reader-assistant/chat", map[string]any{}, http.StatusBadRequest)
	requestJSON(t, handler, http.MethodPost, "/api/library/novels/"+novelID+"/reader-assistant/chat", map[string]any{
		"message":             "hello",
		"currentEpisodeIndex": "999",
		"position":            0,
	}, http.StatusBadRequest)
	requestJSON(t, handler, http.MethodPost, "/api/library/novels/"+novelID+"/reader-assistant/chat", map[string]any{
		"message":             "hello",
		"currentEpisodeIndex": "1",
		"position":            -1,
	}, http.StatusBadRequest)
	assistant := requestJSON(t, handler, http.MethodPost, "/api/library/novels/"+novelID+"/reader-assistant/chat", map[string]any{
		"message":             "hello",
		"currentEpisodeIndex": "1",
		"position":            0,
	}, http.StatusServiceUnavailable)
	if !strings.Contains(assistant["error"].(string), "読書AIはLLM連携が未設定") {
		t.Fatalf("reader assistant should reject missing LLM settings: %+v", assistant)
	}
	assistantStream := requestRaw(t, handler, http.MethodPost, "/api/library/novels/"+novelID+"/reader-assistant/chat/stream", map[string]any{
		"message":             "hello",
		"currentEpisodeIndex": "1",
		"position":            0,
	}, http.StatusOK)
	if !strings.Contains(assistantStream, `"type":"error"`) ||
		!strings.Contains(assistantStream, `読書AIはLLM連携が未設定`) ||
		strings.Contains(assistantStream, `"type":"result"`) ||
		strings.Contains(assistantStream, `"type":"tool_call"`) {
		t.Fatalf("unexpected assistant stream: %s", assistantStream)
	}
	assistantEvents := decodeNDJSONEvents(t, assistantStream)
	assistantErrorIndex := eventIndex(assistantEvents, "error", "")
	if len(assistantEvents) != 3 ||
		assistantEvents[0]["type"] != "status" ||
		assistantEvents[1]["type"] != "status" ||
		assistantErrorIndex != 2 {
		t.Fatalf("unexpected assistant stream event order: %+v", assistantEvents)
	}

	requestJSON(t, handler, http.MethodPut, "/api/ai-generation/settings", map[string]any{"profiles": "bad"}, http.StatusBadRequest)
	requestJSON(t, handler, http.MethodPut, "/api/ai-generation/settings", map[string]any{"profiles": []any{}}, http.StatusBadRequest)
	requestJSON(t, handler, http.MethodPut, "/api/ai-generation/settings", map[string]any{"preferredMode": "bad"}, http.StatusBadRequest)
	requestJSON(t, handler, http.MethodPut, "/api/ai-generation/settings", map[string]any{"preferredMode": true}, http.StatusBadRequest)
	requestJSON(t, handler, http.MethodPut, "/api/ai-generation/settings", map[string]any{"selectedProfileId": 1}, http.StatusBadRequest)
	requestJSON(t, handler, http.MethodPut, "/api/ai-generation/settings", map[string]any{"selectedProfileId": "missing"}, http.StatusBadRequest)
	requestJSON(t, handler, http.MethodPut, "/api/ai-generation/settings", map[string]any{"sharedProviders": "bad"}, http.StatusBadRequest)
	requestJSON(t, handler, http.MethodPut, "/api/ai-generation/settings", map[string]any{"profiles": []any{map[string]any{"label": ""}}}, http.StatusBadRequest)
	requestJSON(t, handler, http.MethodPut, "/api/ai-generation/settings", map[string]any{
		"selectedProfileId": "missing",
		"profiles":          []any{map[string]any{"id": "default", "label": "Default"}},
	}, http.StatusBadRequest)
	requestJSON(t, handler, http.MethodPut, "/api/ai-generation/settings", map[string]any{
		"preferredMode": "heuristic",
		"profiles":      []any{map[string]any{"id": "default", "label": "Default"}},
	}, http.StatusOK)
	requestJSON(t, handler, http.MethodPut, "/api/ai-generation/settings/preferred-mode", map[string]any{"preferredMode": "bad"}, http.StatusBadRequest)
	requestJSON(t, handler, http.MethodPost, "/api/ai-generation/playground/extraction", map[string]any{}, http.StatusBadRequest)
	requestJSON(t, handler, http.MethodGet, "/api/ai-generation/playground/extraction", nil, http.StatusMethodNotAllowed)
	requestJSON(t, handler, http.MethodPost, "/api/ai-generation/playground/extraction", map[string]any{
		"novelId":          "missing",
		"upToEpisodeIndex": "1",
	}, http.StatusNotFound)
	playground := requestJSON(t, handler, http.MethodPost, "/api/ai-generation/playground/extraction", map[string]any{
		"novelId":          novelID,
		"upToEpisodeIndex": "1",
	}, http.StatusOK)
	if playground["novelId"] != novelID || playground["novelTitle"] != "Fixture Novel" || playground["profileLabel"] != "Default" || playground["generationMode"] != "heuristic" {
		t.Fatalf("unexpected playground result: %+v", playground)
	}
	requestJSON(t, handler, http.MethodPut, "/api/ai-generation/settings/preferred-mode", map[string]any{"preferredMode": "llm"}, http.StatusOK)
	llmPlayground := requestJSON(t, handler, http.MethodPost, "/api/ai-generation/playground/extraction", map[string]any{
		"novelId":          novelID,
		"upToEpisodeIndex": "1",
	}, http.StatusOK)
	if llmPlayground["generationMode"] != "disabled" {
		t.Fatalf("playground should report effective AI generation mode: %+v", llmPlayground)
	}
	requestJSON(t, handler, http.MethodPost, "/api/ai-generation/playground/extraction", map[string]any{
		"novelId":          novelID,
		"upToEpisodeIndex": "999",
	}, http.StatusBadRequest)
	requestJSON(t, handler, http.MethodPost, "/api/ai-generation/playground/extraction", map[string]any{
		"novelId":          novelID,
		"upToEpisodeIndex": 0,
	}, http.StatusBadRequest)
	requestJSON(t, handler, http.MethodPost, "/api/ai-generation/playground/extraction/stream", map[string]any{
		"novelId": "missing",
	}, http.StatusBadRequest)
	requestJSON(t, handler, http.MethodPost, "/api/ai-generation/playground/extraction/stream", map[string]any{
		"novelId":          "missing",
		"upToEpisodeIndex": "1",
	}, http.StatusNotFound)
	requestJSON(t, handler, http.MethodPost, "/api/ai-generation/playground/extraction/stream", map[string]any{
		"novelId":          novelID,
		"upToEpisodeIndex": "999",
	}, http.StatusBadRequest)
	requestJSON(t, handler, http.MethodGet, "/api/ai-generation/usage/missing", nil, http.StatusNotFound)
	requestJSON(t, handler, http.MethodPost, "/api/ai-generation/jobs", nil, http.StatusMethodNotAllowed)
	requestJSON(t, handler, http.MethodPost, "/api/ai-generation/usage", nil, http.StatusMethodNotAllowed)
	requestJSON(t, handler, http.MethodPost, "/api/ai-generation/usage/run-http", nil, http.StatusMethodNotAllowed)

	requestJSON(t, handler, http.MethodPut, "/api/reader/state", map[string]any{"novelId": novelID, "position": -1}, http.StatusBadRequest)
	requestJSON(t, handler, http.MethodPut, "/api/reader/state", map[string]any{"novelId": novelID, "scroll": map[string]any{"type": "bad", "value": 1}}, http.StatusBadRequest)
	requestJSON(t, handler, http.MethodPut, "/api/reader/state", map[string]any{"novelId": novelID, "clientId": ""}, http.StatusBadRequest)
	requestJSON(t, handler, http.MethodPut, "/api/reader/state", map[string]any{"novelId": novelID, "expectedStateVersion": -1}, http.StatusBadRequest)
	requestJSON(t, handler, http.MethodPut, "/api/reader/state", map[string]any{"novelId": novelID, "expectedStateVersion": nil}, http.StatusBadRequest)
	requestJSON(t, handler, http.MethodGet, "/api/reader/state?novelId="+novelID, nil, http.StatusOK)
	requestJSON(t, handler, http.MethodPost, "/api/reader/state", nil, http.StatusMethodNotAllowed)
	requestJSON(t, handler, http.MethodPost, "/api/reader/preferences", nil, http.StatusMethodNotAllowed)
	requestJSON(t, handler, http.MethodPut, "/api/reader/preferences", map[string]any{}, http.StatusBadRequest)
	requestJSON(t, handler, http.MethodPut, "/api/reader/preferences", map[string]any{"readingMode": "bad"}, http.StatusBadRequest)
	requestJSON(t, handler, http.MethodPut, "/api/reader/preferences", map[string]any{"fontFamily": "bad"}, http.StatusBadRequest)
	requestJSON(t, handler, http.MethodPut, "/api/reader/preferences", map[string]any{"theme": "bad"}, http.StatusBadRequest)
	requestJSON(t, handler, http.MethodPut, "/api/library/novels/"+novelID+"/reader-settings", map[string]any{}, http.StatusBadRequest)
	requestJSON(t, handler, http.MethodPut, "/api/library/novels/"+novelID+"/reader-settings", map[string]any{
		"correction": map[string]any{},
	}, http.StatusBadRequest)
	requestJSON(t, handler, http.MethodPut, "/api/library/novels/"+novelID+"/reader-settings", map[string]any{
		"correction": map[string]any{"halfwidthAlnumPunctuationNormalizaton": false},
	}, http.StatusBadRequest)
	requestJSON(t, handler, http.MethodPut, "/api/library/novels/"+novelID+"/reader-settings", map[string]any{
		"correction": map[string]any{"quoteNormalization": "bad"},
	}, http.StatusBadRequest)
	requestJSON(t, handler, http.MethodPut, "/api/library/novels/"+novelID+"/reader-settings", map[string]any{
		"correction": map[string]any{"quoteNormalization": false, "hyphenDashNormalization": "bad"},
	}, http.StatusBadRequest)
	requestJSON(t, handler, http.MethodPut, "/api/library/novels/"+novelID+"/reader-settings", map[string]any{
		"correction": map[string]any{"quoteNormalization": false, "parenthesisNormalization": "bad"},
	}, http.StatusBadRequest)
	requestJSON(t, handler, http.MethodPut, "/api/library/novels/"+novelID+"/reader-settings", map[string]any{
		"correction": map[string]any{"quoteNormalization": false, "halfwidthAlnumPunctuationNormalization": "bad"},
	}, http.StatusBadRequest)
	requestJSON(t, handler, http.MethodPut, "/api/library/novels/missing/reader-settings", map[string]any{
		"correction": map[string]any{"quoteNormalization": false},
	}, http.StatusNotFound)
	requestJSON(t, handler, http.MethodPost, "/api/library/novels/"+novelID+"/reader-settings", nil, http.StatusMethodNotAllowed)
	requestJSON(t, handler, http.MethodPost, "/api/bookmarks", map[string]any{}, http.StatusBadRequest)
	requestJSON(t, handler, http.MethodPost, "/api/bookmarks", map[string]any{"novelId": novelID, "episodeIndex": "1", "position": -1}, http.StatusBadRequest)
	requestJSON(t, handler, http.MethodPost, "/api/bookmarks", map[string]any{"novelId": "missing", "episodeIndex": "1", "position": 1}, http.StatusNotFound)
	requestJSON(t, handler, http.MethodDelete, "/api/bookmarks/missing", nil, http.StatusNotFound)
	requestJSON(t, handler, http.MethodPost, "/api/bookmarks/missing", nil, http.StatusMethodNotAllowed)

	requestJSON(t, handler, http.MethodGet, "/api/fetcher/queue", nil, http.StatusOK)
	requestJSON(t, handler, http.MethodGet, "/api/fetcher/tasks/summary", nil, http.StatusOK)
	requestJSON(t, handler, http.MethodPost, "/api/fetcher/status", nil, http.StatusMethodNotAllowed)
	requestJSON(t, handler, http.MethodPost, "/api/fetcher/queue", nil, http.StatusMethodNotAllowed)
	requestJSON(t, handler, http.MethodPost, "/api/fetcher/tasks/summary", nil, http.StatusMethodNotAllowed)
	requestJSON(t, handler, http.MethodPost, "/api/fetcher/works/download", map[string]any{}, http.StatusBadRequest)
	requestJSON(t, handler, http.MethodPost, "/api/fetcher/works/download", map[string]any{"targets": []string{""}}, http.StatusBadRequest)
	fetcherDownload := requestJSON(t, handler, http.MethodPost, "/api/fetcher/works/download", map[string]any{"targets": []string{"https://example.test/novel"}}, http.StatusAccepted)
	if fetcherDownload["message"] != "Download started" {
		t.Fatalf("unexpected canonical download response: %+v", fetcherDownload)
	}
	requestJSON(t, handler, http.MethodPost, "/api/fetcher/works/update", map[string]any{}, http.StatusBadRequest)
	missingNovels := requestJSON(t, handler, http.MethodPost, "/api/fetcher/works/update", map[string]any{"novelIds": []string{"missing"}}, http.StatusNotFound)
	if missingNovels["code"] != "NOVELS_NOT_FOUND" ||
		missingNovels["missingNovelIds"].([]any)[0] != "missing" ||
		missingNovels["details"].(map[string]any)["missingNovelIds"].([]any)[0] != "missing" {
		t.Fatalf("missing novel IDs should be available during structured error migration: %+v", missingNovels)
	}
	requestJSON(t, handler, http.MethodPost, "/api/fetcher/tasks/task-1/unknown", nil, http.StatusNotFound)
	requestJSON(t, handler, http.MethodPost, "/api/fetcher/tasks/%20/cancel", nil, http.StatusBadRequest)
	for _, endpoint := range []struct {
		method string
		path   string
	}{
		{http.MethodPut, "/api/reader/state"},
		{http.MethodPut, "/api/reader/preferences"},
		{http.MethodPost, "/api/bookmarks"},
		{http.MethodPut, "/api/ai-generation/settings"},
		{http.MethodPut, "/api/ai-generation/settings/preferred-mode"},
		{http.MethodPost, "/api/ai-generation/playground/extraction"},
		{http.MethodPost, "/api/ai-generation/playground/extraction/stream"},
		{http.MethodPost, "/api/library/novels/" + novelID + "/extraction-jobs"},
		{http.MethodPost, "/api/library/novels/" + novelID + "/reader-assistant/chat"},
		{http.MethodPost, "/api/library/novels/" + novelID + "/reader-assistant/chat/stream"},
		{http.MethodPost, "/api/fetcher/works/download"},
		{http.MethodPost, "/api/fetcher/works/update"},
	} {
		response := requestJSONRaw(t, handler, endpoint.method, endpoint.path, "{", http.StatusBadRequest)
		if response["error"] != "Malformed JSON body." {
			t.Fatalf("unexpected malformed json response for %s %s: %+v", endpoint.method, endpoint.path, response)
		}
	}

	badJSONRequest := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{"))
	if decoded, ok := decodeObject(httptest.NewRecorder(), badJSONRequest); ok || len(decoded) != 0 {
		t.Fatalf("invalid json should be reported explicitly: ok=%v decoded=%+v", ok, decoded)
	}
	emptyJSONRequest := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(""))
	if decoded, ok := decodeObject(httptest.NewRecorder(), emptyJSONRequest); !ok || len(decoded) != 0 {
		t.Fatalf("empty json body should decode to empty object: ok=%v decoded=%+v", ok, decoded)
	}
	nullJSONRequest := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("null"))
	if decoded, ok := decodeObject(httptest.NewRecorder(), nullJSONRequest); !ok || len(decoded) != 0 {
		t.Fatalf("null json body should decode to empty object: ok=%v decoded=%+v", ok, decoded)
	}
	validJSONRequest := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"ok":true}`))
	if decoded, ok := decodeObject(httptest.NewRecorder(), validJSONRequest); !ok || decoded["ok"] != true {
		t.Fatalf("valid json object should decode: ok=%v decoded=%+v", ok, decoded)
	}

	trailingJSONRequest := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"ok":true}{"extra":true}`))
	if decoded, ok := decodeObject(httptest.NewRecorder(), trailingJSONRequest); ok || len(decoded) != 0 {
		t.Fatalf("trailing json should be rejected: ok=%v decoded=%+v", ok, decoded)
	}

	oversizedJSONRequest := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"text":"`+strings.Repeat("x", int(maxJSONBodyBytes))+`"}`))
	if decoded, ok := decodeObject(httptest.NewRecorder(), oversizedJSONRequest); ok || len(decoded) != 0 {
		t.Fatalf("oversized json body should be rejected: ok=%v decoded=%+v", ok, decoded)
	}

	textJSONRequest := httptest.NewRequest(http.MethodPut, "/api/ai-generation/settings/preferred-mode", strings.NewReader(`{"preferredMode":"llm"}`))
	textJSONRequest.Header.Set("Content-Type", "text/plain")
	setTestAPIContractHeaders(textJSONRequest)
	textJSONResponse := httptest.NewRecorder()
	handler.ServeHTTP(textJSONResponse, textJSONRequest)
	if textJSONResponse.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("text/plain JSON mutation should be rejected: code=%d body=%s", textJSONResponse.Code, textJSONResponse.Body.String())
	}
}

type failingFetcherLibraryReader struct {
	err error
}

func (f failingFetcherLibraryReader) ListLibraryWorks(context.Context) ([]fetcher.LibraryWork, error) {
	return nil, f.err
}

func (f failingFetcherLibraryReader) GetLibraryToc(context.Context, int) (fetcher.LibraryWork, []fetcher.LibraryEpisode, error) {
	return fetcher.LibraryWork{}, nil, f.err
}

func (f failingFetcherLibraryReader) GetLibraryEpisode(context.Context, int, string) (fetcher.LibraryEpisodeResponse, error) {
	return fetcher.LibraryEpisodeResponse{}, f.err
}

func TestExtractionLookupErrorReturnsBadGateway(t *testing.T) {
	service := library.NewServiceWithFetcher(t.TempDir(), failingFetcherLibraryReader{err: errors.New("sidecar unavailable")})
	handler := NewServerWithDependencies(ServerDependencies{
		DataDir: t.TempDir(),
		Library: service,
	})
	novelID := library.NovelID(library.Work{ID: 1, Site: "syosetu", SiteWorkID: "n1234ab"})

	response := requestJSON(t, handler, http.MethodGet, "/api/library/novels/"+novelID+"/characters?upToEpisodeIndex=1", nil, http.StatusBadGateway)
	if response["message"] != "Library data could not be read." {
		t.Fatalf("lookup failure should not be reported as not found: %+v", response)
	}
}

func TestTermsEndpointUsesCharacterCommitFrontierAndDistinguishesEmptyState(t *testing.T) {
	dataDir := newHTTPAPITestData(t)
	stateStore := store.New(dataDir)
	if err := stateStore.Initialize(); err != nil {
		t.Fatalf("initialize store: %v", err)
	}
	handler := newTestServerWithLibraryAndStore(dataDir, library.NewService(filepath.Join(dataDir, "novel-fetcher")), stateStore)
	server := handler.(*Server)
	novels := requestJSON(t, handler, http.MethodGet, "/api/library/novels", nil, http.StatusOK)
	novelID := novels["novels"].([]any)[0].(map[string]any)["novelId"].(string)
	path := "/api/library/novels/" + novelID + "/terms?upToEpisodeIndex=1"

	legacy := requestJSON(t, handler, http.MethodGet, path, nil, http.StatusOK)
	if legacy["status"] != "not_generated" || legacy["processedUpToEpisodeIndex"] != nil || len(legacy["terms"].([]any)) != 0 {
		t.Fatalf("character-only state should expose an actionable not_generated terms response: %+v", legacy)
	}
	if err := terms.SaveGeneratedTerms(server.stateDir(), novelID, "1", []terms.GeneratedTerm{}, nil); err != nil {
		t.Fatalf("save empty generated terms: %v", err)
	}
	empty := requestJSON(t, handler, http.MethodGet, path, nil, http.StatusOK)
	if empty["status"] != "ready" || empty["processedUpToEpisodeIndex"] != "1" || len(empty["terms"].([]any)) != 0 {
		t.Fatalf("generated empty terms should be ready: %+v", empty)
	}
	if err := terms.SaveGeneratedTerms(server.stateDir(), novelID, "1", []terms.GeneratedTerm{{
		Term:               "聖剣",
		CategoryHistory:    []terms.CategoryVersion{{Category: terms.CategoryItem, EpisodeIndex: "1"}},
		DescriptionHistory: []terms.HistoryVersion{{Text: "王家に伝わる剣。", EpisodeIndex: "1"}},
	}}, nil); err != nil {
		t.Fatalf("save generated term: %v", err)
	}
	ready := requestJSON(t, handler, http.MethodGet, path, nil, http.StatusOK)
	items := ready["terms"].([]any)
	if len(items) != 1 {
		t.Fatalf("term projection should contain the committed term: %+v", ready)
	}
	item := items[0].(map[string]any)
	if item["term"] != "聖剣" || item["reading"] != nil || item["category"] != terms.CategoryItem || item["description"] != "王家に伝わる剣。" {
		t.Fatalf("unexpected term projection: %+v", item)
	}
}

func TestExtractionClearEndpointDeletesGeneratedState(t *testing.T) {
	dataDir := newHTTPAPITestData(t)
	stateStore := store.New(dataDir)
	if err := stateStore.Initialize(); err != nil {
		t.Fatalf("initialize store: %v", err)
	}
	handler := newTestServerWithLibraryAndStore(dataDir, library.NewService(filepath.Join(dataDir, "novel-fetcher")), stateStore)
	server := handler.(*Server)
	novels := requestJSON(t, handler, http.MethodGet, "/api/library/novels", nil, http.StatusOK)
	novelID := novels["novels"].([]any)[0].(map[string]any)["novelId"].(string)
	episodeIndex := "1"

	if err := characters.SaveGeneratedSummary(server.stateDir(), novelID, episodeIndex, []characters.GeneratedCharacter{{
		CharacterID:                 "char_alice",
		CanonicalName:               "アリス",
		CanonicalEpisodeIndex:       episodeIndex,
		FirstAppearanceEpisodeIndex: episodeIndex,
		SummaryHistory:              []characters.GeneratedHistoryVersion{{EpisodeIndex: episodeIndex, Text: "生成済み。"}},
	}}); err != nil {
		t.Fatalf("save generated summary: %v", err)
	}
	if err := terms.SaveGeneratedTerms(server.stateDir(), novelID, episodeIndex, []terms.GeneratedTerm{{
		Term:               "聖剣",
		DescriptionHistory: []terms.HistoryVersion{{EpisodeIndex: episodeIndex, Text: "王家に伝わる剣。"}},
	}}, nil); err != nil {
		t.Fatalf("save generated terms: %v", err)
	}
	if err := extractdomain.SaveJob(server.stateDir(), novelID, extractdomain.Job{
		JobID:                     "completed-clear-test",
		RequestedUpToEpisodeIndex: episodeIndex,
		Status:                    "completed",
		CreatedAt:                 "2026-01-01T00:00:00Z",
	}); err != nil {
		t.Fatalf("save completed job: %v", err)
	}
	checkpointPath := server.extractionCheckpointPath(novelID, episodeIndex)
	if err := os.MkdirAll(filepath.Dir(checkpointPath), 0o755); err != nil {
		t.Fatalf("mkdir checkpoint dir: %v", err)
	}
	if err := os.WriteFile(checkpointPath, []byte(`{"schemaVersion":4,"novelId":"`+novelID+`","upToEpisodeIndex":"`+episodeIndex+`","characters":[]}`), 0o644); err != nil {
		t.Fatalf("write checkpoint: %v", err)
	}

	before := requestJSON(t, handler, http.MethodGet, "/api/library/novels/"+novelID+"/characters?upToEpisodeIndex="+episodeIndex, nil, http.StatusOK)
	if before["status"] != "ready" {
		t.Fatalf("precondition generated summary should be ready: %+v", before)
	}
	cleared := requestJSON(t, handler, http.MethodDelete, "/api/library/novels/"+novelID+"/extraction", nil, http.StatusOK)
	if cleared["characterProfileDeleted"] != true || cleared["characterEventsDeleted"] != true || cleared["termProfileDeleted"] != true || cleared["extractionJobsDeleted"].(float64) < 1 || cleared["extractionCheckpointsDeleted"] != float64(1) {
		t.Fatalf("clear response should report deleted generated state: %+v", cleared)
	}
	after := requestJSON(t, handler, http.MethodGet, "/api/library/novels/"+novelID+"/characters?upToEpisodeIndex="+episodeIndex, nil, http.StatusOK)
	if after["status"] != "not_generated" || len(after["characters"].([]any)) != 0 {
		t.Fatalf("cleared character summary should become not_generated: %+v", after)
	}
	afterTerms := requestJSON(t, handler, http.MethodGet, "/api/library/novels/"+novelID+"/terms?upToEpisodeIndex="+episodeIndex, nil, http.StatusOK)
	if afterTerms["status"] != "not_generated" || len(afterTerms["terms"].([]any)) != 0 {
		t.Fatalf("cleared terms should become not_generated: %+v", afterTerms)
	}
	jobs := requestJSON(t, handler, http.MethodGet, "/api/library/novels/"+novelID+"/extraction-jobs", nil, http.StatusOK)
	if len(jobs["jobs"].([]any)) != 0 {
		t.Fatalf("clear should delete character jobs: %+v", jobs)
	}
	if _, err := os.Stat(checkpointPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("clear should delete checkpoints, err=%v", err)
	}
}

func TestExtractionClearEndpointRejectsActiveJob(t *testing.T) {
	dataDir := newHTTPAPITestData(t)
	stateStore := store.New(dataDir)
	if err := stateStore.Initialize(); err != nil {
		t.Fatalf("initialize store: %v", err)
	}
	handler := newTestServerWithLibraryAndStore(dataDir, library.NewService(filepath.Join(dataDir, "novel-fetcher")), stateStore)
	server := handler.(*Server)
	novels := requestJSON(t, handler, http.MethodGet, "/api/library/novels", nil, http.StatusOK)
	novelID := novels["novels"].([]any)[0].(map[string]any)["novelId"].(string)
	if err := characters.SaveGeneratedSummary(server.stateDir(), novelID, "1", []characters.GeneratedCharacter{{
		CharacterID:                 "char_alice",
		CanonicalName:               "アリス",
		CanonicalEpisodeIndex:       "1",
		FirstAppearanceEpisodeIndex: "1",
	}}); err != nil {
		t.Fatalf("save generated summary: %v", err)
	}
	if err := extractdomain.SaveJob(server.stateDir(), novelID, extractdomain.Job{
		JobID:                     "running-clear-test",
		RequestedUpToEpisodeIndex: "1",
		Status:                    "running",
		CreatedAt:                 "2026-01-01T00:00:00Z",
	}); err != nil {
		t.Fatalf("save running job: %v", err)
	}

	requestJSON(t, handler, http.MethodDelete, "/api/library/novels/"+novelID+"/extraction", nil, http.StatusConflict)
	summary := requestJSON(t, handler, http.MethodGet, "/api/library/novels/"+novelID+"/characters?upToEpisodeIndex=1", nil, http.StatusOK)
	if summary["status"] != "ready" {
		t.Fatalf("active job rejection should keep generated summary: %+v", summary)
	}
}

func TestServerDegradedDependencies(t *testing.T) {
	dataDir := t.TempDir()
	stateStore := store.New(dataDir)
	if err := stateStore.Initialize(); err != nil {
		t.Fatalf("initialize store: %v", err)
	}
	handler := newTestServerWithLibraryAndStore(dataDir, nil, stateStore)
	requestJSON(t, handler, http.MethodGet, "/api/system/status", nil, http.StatusOK)
	requestJSON(t, handler, http.MethodGet, "/api/library/novels", nil, http.StatusOK)
	requestJSON(t, handler, http.MethodGet, "/api/library/novels/missing/toc", nil, http.StatusNotFound)
	requestJSON(t, handler, http.MethodGet, "/api/library/novels/missing/characters?upToEpisodeIndex=1", nil, http.StatusNotFound)
	requestJSON(t, handler, http.MethodPost, "/api/ai-generation/playground/extraction/stream", map[string]any{
		"novelId":          "missing",
		"upToEpisodeIndex": "1",
	}, http.StatusNotFound)
}

func TestServerFallbackStateBranches(t *testing.T) {
	dataDir := newHTTPAPITestData(t)
	if err := os.RemoveAll(filepath.Join(dataDir, "state", "character_profiles")); err != nil {
		t.Fatalf("remove character profiles: %v", err)
	}
	if err := os.RemoveAll(filepath.Join(dataDir, "state", "extraction_jobs")); err != nil {
		t.Fatalf("remove character jobs: %v", err)
	}
	if err := os.Remove(filepath.Join(dataDir, "state", "ai_usage.sqlite")); err != nil {
		t.Fatalf("remove ai usage: %v", err)
	}
	stateStore := store.New(dataDir)
	if err := stateStore.Initialize(); err != nil {
		t.Fatalf("initialize store: %v", err)
	}
	handler := newTestServerWithLibraryAndStore(dataDir, library.NewService(filepath.Join(dataDir, "novel-fetcher")), stateStore)
	novels := requestJSON(t, handler, http.MethodGet, "/api/library/novels", nil, http.StatusOK)
	novelID := novels["novels"].([]any)[0].(map[string]any)["novelId"].(string)

	characters := requestJSON(t, handler, http.MethodGet, "/api/library/novels/"+novelID+"/characters?upToEpisodeIndex=1", nil, http.StatusOK)
	if characters["status"] != "not_generated" || len(characters["characters"].([]any)) != 0 {
		t.Fatalf("fallback characters should be not_generated without fake characters: %+v", characters)
	}
	jobs := requestJSON(t, handler, http.MethodGet, "/api/library/novels/"+novelID+"/extraction-jobs", nil, http.StatusOK)
	if len(jobs["jobs"].([]any)) != 0 {
		t.Fatalf("expected no initial memory jobs: %+v", jobs)
	}
	created := requestJSON(t, handler, http.MethodPost, "/api/library/novels/"+novelID+"/extraction-jobs", map[string]any{
		"upToEpisodeIndex": "1",
	}, http.StatusAccepted)
	if created["jobId"] == "" {
		t.Fatalf("unexpected created job: %+v", created)
	}
	if created["status"] != "queued" {
		t.Fatalf("created job should start queued before the worker processes it: %+v", created)
	}
	completed := waitForCharacterJobStatus(t, handler, novelID, created["jobId"].(string), "completed")
	jobs = requestJSON(t, handler, http.MethodGet, "/api/library/novels/"+novelID+"/extraction-jobs", nil, http.StatusOK)
	if len(jobs["jobs"].([]any)) != 1 {
		t.Fatalf("expected one memory job: %+v", jobs)
	}
	if completed["startedAt"] == nil || completed["finishedAt"] == nil {
		t.Fatalf("persisted job should include worker timestamps: %+v", completed)
	}
	generatedCharacters := requestJSON(t, handler, http.MethodGet, "/api/library/novels/"+novelID+"/characters?upToEpisodeIndex=1", nil, http.StatusOK)
	if generatedCharacters["status"] != "ready" || len(generatedCharacters["characters"].([]any)) != 0 {
		t.Fatalf("completed empty worker output should be ready with no characters: %+v", generatedCharacters)
	}
	usage := requestJSON(t, handler, http.MethodGet, "/api/ai-generation/usage", nil, http.StatusOK)
	if usage["runs"] == nil {
		t.Fatalf("fallback usage should include runs key: %+v", usage)
	}
	playground := requestJSON(t, handler, http.MethodPost, "/api/ai-generation/playground/extraction", map[string]any{
		"novelId":          novelID,
		"upToEpisodeIndex": "1",
	}, http.StatusOK)
	if characters := playground["characters"].([]any); len(characters) != 0 {
		t.Fatalf("missing profiles should produce empty playground characters: %+v", playground)
	}
	requestJSON(t, handler, http.MethodGet, "/api/ai-generation/usage/go-fixture-usage-run", nil, http.StatusNotFound)
}

func TestReaderAssistantUsesConfiguredOpenRouterProvider(t *testing.T) {
	t.Setenv("AI_GENERATION_SETTINGS_MASTER_PASSPHRASE", "test-passphrase")
	t.Setenv("OPENROUTER_REASONING_EFFORT", "high")
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("authorization") != "Bearer sk-reader-secret" {
			t.Fatalf("unexpected authorization: %q", r.Header.Get("authorization"))
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode reader OpenRouter request: %v", err)
		}
		if body["reasoning"].(map[string]any)["effort"] != "high" || body["provider"].(map[string]any)["require_parameters"] != true {
			t.Fatalf("environment reasoning should require provider support: %+v", body)
		}
		_, _ = w.Write([]byte(`{
			"choices": [{"message": {"content": "OpenRouterからの応答です。"}}],
			"usage": {"prompt_tokens": 11, "completion_tokens": 7, "total_tokens": 18}
		}`))
	}))
	defer provider.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", provider.URL)

	dataDir := newHTTPAPITestData(t)
	stateStore := store.New(dataDir)
	if err := stateStore.Initialize(); err != nil {
		t.Fatalf("initialize store: %v", err)
	}
	modelID := "openrouter/auto"
	if _, err := stateStore.PutAIGenerationSettings(store.AIGenerationSettingsUpdate{
		PreferredMode: testStringPtr("llm"),
		SharedProviders: &store.AISharedProvidersInput{
			OpenRouter: store.AIProviderCredentialInput{APIKey: testStringPtr("sk-reader-secret"), APIKeySet: true},
		},
		ProfilesSet: true,
		Profiles: []store.AIProfileInput{
			{
				ID:                "default",
				Label:             "Default",
				Provider:          "openrouter",
				Credentials:       store.AIProfileCredentialsInput{Source: "shared"},
				ModelID:           &modelID,
				RequireParameters: true,
			},
		},
	}); err != nil {
		t.Fatalf("put AI settings: %v", err)
	}
	handler := newTestServerWithLibraryAndStore(dataDir, library.NewService(filepath.Join(dataDir, "novel-fetcher")), stateStore)
	novels := requestJSON(t, handler, http.MethodGet, "/api/library/novels", nil, http.StatusOK)
	novelID := novels["novels"].([]any)[0].(map[string]any)["novelId"].(string)
	response := requestJSON(t, handler, http.MethodPost, "/api/library/novels/"+novelID+"/reader-assistant/chat", map[string]any{
		"message":             "hello",
		"currentEpisodeIndex": "1",
		"position":            0,
	}, http.StatusOK)
	if response["generationMode"] != "remote" || response["answer"] != "OpenRouterからの応答です。" || response["runId"] == nil {
		t.Fatalf("reader assistant should use fake OpenRouter provider: %+v", response)
	}
	reasoningMetadata := response["reasoning"].(map[string]any)
	if reasoningMetadata["requestedEffort"] != "high" || reasoningMetadata["source"] != "environment" || reasoningMetadata["requireParameters"] != true {
		t.Fatalf("reader response should report resolved reasoning request: %+v", response)
	}
	run, ok, err := ai.LoadUsageRun(handler.(*Server).aiUsageDBPath(), response["runId"].(string))
	if err != nil || !ok {
		t.Fatalf("reader usage should be readable: ok=%v run=%+v err=%v", ok, run, err)
	}
	snapshot := run.Snapshot.(map[string]any)
	snapshotReasoning := snapshot["reasoning"].(map[string]any)
	if snapshotReasoning["requestedEffort"] != "high" || snapshotReasoning["source"] != "environment" || snapshotReasoning["requireParameters"] != true {
		t.Fatalf("reader usage should persist resolved reasoning request: %+v", snapshot)
	}
	usage := requestJSON(t, handler, http.MethodGet, "/api/ai-generation/usage", nil, http.StatusOK)
	runs := usage["runs"].([]any)
	if len(runs) == 0 || runs[0].(map[string]any)["generationMode"] != "remote" {
		t.Fatalf("usage should include remote reader assistant run: %+v", usage)
	}
}

func TestReaderAssistantRunsOpenRouterToolLoop(t *testing.T) {
	t.Setenv("AI_GENERATION_SETTINGS_MASTER_PASSPHRASE", "test-passphrase")
	providerCalls := 0
	reasoning := json.RawMessage(`"tool reasoning"`)
	reasoningDetails := json.RawMessage(`[{"type":"reasoning.encrypted","data":"signed-first","id":"first"},{"type":"reasoning.summary","summary":"second block","id":"second"}]`)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		providerCalls++
		rawBody, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read OpenRouter request: %v", err)
		}
		var body map[string]any
		if err := json.Unmarshal(rawBody, &body); err != nil {
			t.Fatalf("decode OpenRouter request: %v", err)
		}
		if _, ok := body["tools"].([]any); !ok {
			t.Fatalf("reader assistant should send tool definitions: %+v", body)
		}
		if providerCalls == 1 {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []any{map[string]any{
					"finish_reason": "tool_calls",
					"message": map[string]any{
						"reasoning":         reasoning,
						"reasoning_details": reasoningDetails,
						"tool_calls": []any{map[string]any{
							"id":   "call_range",
							"type": "function",
							"function": map[string]any{
								"name":      "load_episode_range",
								"arguments": `{"startEpisodeNumber":1,"endEpisodeNumber":1}`,
							},
						}},
					},
				}},
				"usage": map[string]any{"prompt_tokens": 10, "completion_tokens": 3, "total_tokens": 13},
			})
			return
		}
		messages := body["messages"].([]any)
		last := messages[len(messages)-1].(map[string]any)
		if last["role"] != "tool" || last["tool_call_id"] != "call_range" || !strings.Contains(last["content"].(string), "episodes") {
			t.Fatalf("second request should include tool result message: %+v", body)
		}
		var request struct {
			Messages []struct {
				Role             string          `json:"role"`
				Reasoning        json.RawMessage `json:"reasoning"`
				ReasoningDetails json.RawMessage `json:"reasoning_details"`
			} `json:"messages"`
		}
		if err := json.Unmarshal(rawBody, &request); err != nil {
			t.Fatalf("decode typed OpenRouter request: %v", err)
		}
		assistant := request.Messages[len(request.Messages)-2]
		if assistant.Role != "assistant" || !bytes.Equal(assistant.Reasoning, reasoning) || !bytes.Equal(assistant.ReasoningDetails, reasoningDetails) {
			t.Fatalf("second request should preserve reasoning blocks unchanged and in order: reasoning=%s details=%s", assistant.Reasoning, assistant.ReasoningDetails)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{
				"finish_reason": "stop",
				"message":       map[string]any{"content": "1話を確認しました。"},
			}},
			"usage": map[string]any{"prompt_tokens": 8, "completion_tokens": 4, "total_tokens": 12},
		})
	}))
	defer provider.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", provider.URL)

	dataDir := newHTTPAPITestData(t)
	stateStore := store.New(dataDir)
	if err := stateStore.Initialize(); err != nil {
		t.Fatalf("initialize store: %v", err)
	}
	modelID := "openrouter/auto"
	if _, err := stateStore.PutAIGenerationSettings(store.AIGenerationSettingsUpdate{
		PreferredMode: testStringPtr("llm"),
		SharedProviders: &store.AISharedProvidersInput{
			OpenRouter: store.AIProviderCredentialInput{APIKey: testStringPtr("sk-reader-secret"), APIKeySet: true},
		},
		ProfilesSet: true,
		Profiles: []store.AIProfileInput{{
			ID:          "default",
			Label:       "Default",
			Provider:    "openrouter",
			Credentials: store.AIProfileCredentialsInput{Source: "shared"},
			ModelID:     &modelID,
		}},
	}); err != nil {
		t.Fatalf("put AI settings: %v", err)
	}
	handler := newTestServerWithLibraryAndStore(dataDir, library.NewService(filepath.Join(dataDir, "novel-fetcher")), stateStore)
	novels := requestJSON(t, handler, http.MethodGet, "/api/library/novels", nil, http.StatusOK)
	novelID := novels["novels"].([]any)[0].(map[string]any)["novelId"].(string)
	response := requestJSON(t, handler, http.MethodPost, "/api/library/novels/"+novelID+"/reader-assistant/chat", map[string]any{
		"message":             "1話を見たい",
		"currentEpisodeIndex": "1",
		"position":            0,
	}, http.StatusOK)
	if response["answer"] != "1話を確認しました。" || providerCalls != 2 {
		t.Fatalf("reader assistant should complete after a tool loop: response=%+v providerCalls=%d", response, providerCalls)
	}
	toolRequests := response["toolRequests"].([]any)
	if len(toolRequests) != 1 || toolRequests[0].(map[string]any)["name"] != "load_episode_range" {
		t.Fatalf("response should expose executed tool request: %+v", response)
	}
}

func TestReaderAssistantToolExecutionBranches(t *testing.T) {
	dataDir := newHTTPAPITestData(t)
	stateStore := store.New(dataDir)
	if err := stateStore.Initialize(); err != nil {
		t.Fatalf("initialize store: %v", err)
	}
	handler := newTestServerWithLibraryAndStore(dataDir, library.NewService(filepath.Join(dataDir, "novel-fetcher")), stateStore)
	server := handler.(*Server)
	novels := requestJSON(t, handler, http.MethodGet, "/api/library/novels", nil, http.StatusOK)
	novelID := novels["novels"].([]any)[0].(map[string]any)["novelId"].(string)
	toc, err := server.library.GetToc(context.Background(), novelID)
	if err != nil || toc == nil || len(toc.Episodes) == 0 {
		t.Fatalf("load toc: toc=%+v err=%v", toc, err)
	}
	current, err := server.library.GetEpisode(context.Background(), novelID, toc.Episodes[0].EpisodeIndex)
	if err != nil || current == nil {
		t.Fatalf("load current episode: episode=%+v err=%v", current, err)
	}
	contextInfo := readerAssistantContext{
		NovelID:                    novelID,
		NovelTitle:                 toc.Title,
		CurrentEpisodeIndex:        current.EpisodeIndex,
		CurrentEpisodeNumber:       1,
		CurrentEpisodeRef:          episodeReference(current),
		CurrentExcerpt:             readerDocumentBodyText(current.ReaderDocument),
		CurrentPosition:            0,
		Message:                    "直近1話",
		TocEpisodes:                toc.Episodes,
		RecentPreviousEpisodeCount: 1,
		HitRegistry:                newReaderAssistantHitRegistry(),
	}

	currentResult := server.executeReaderAssistantTool(context.Background(), contextInfo, "get_current_episode", `{}`)
	if currentResult.Result["excerpt"] != "" {
		t.Fatalf("recent previous requests should hide current excerpt: %+v", currentResult)
	}
	rangeResult := server.executeReaderAssistantTool(context.Background(), contextInfo, "load_episode_range", `{"startEpisodeNumber":1,"endEpisodeNumber":1}`)
	if rangeResult.Name != "load_episode_range" || rangeResult.Result["episodeCount"].(int) == 0 {
		t.Fatalf("load_episode_range should load fixture episode: %+v", rangeResult)
	}
	summaryResult := server.executeReaderAssistantTool(context.Background(), contextInfo, "load_episode_range", `{"startEpisodeNumber":1,"endEpisodeNumber":1,"output":"summary","summaryPurpose":"plot","summaryFocus":"流れ"}`)
	if summaryResult.Name != "load_episode_range" || summaryResult.Result["generatedBy"] != "local" {
		t.Fatalf("summary range should use local summarizer result: %+v", summaryResult)
	}
	if summaryResult.Result["summaryPurpose"] != "plot" || summaryResult.Result["summaryFocus"] != "流れ" {
		t.Fatalf("summary range should preserve requested log fields: %+v", summaryResult)
	}
	rangeRecovery := readerAssistantToolRecovery("load_episode_range", fmtEpisodeRangeError(readerAssistantMaxEpisodeRangeCount))
	if !strings.Contains(rangeRecovery.Result["guidance"].(string), "第1〜20話") || !strings.Contains(rangeRecovery.Result["guidance"].(string), "第21〜30話") {
		t.Fatalf("range recovery guidance should suggest concrete split ranges: %+v", rangeRecovery)
	}
	searchResult := server.executeReaderAssistantTool(context.Background(), contextInfo, "search_episodes", `{"query":"本文","startEpisodeNumber":1,"endEpisodeNumber":1}`)
	if searchResult.Name != "search_episodes" || len(searchResult.Result["matches"].([]map[string]any)) == 0 {
		t.Fatalf("search_episodes should find fixture text: %+v", searchResult)
	}
	fullTextResult := server.executeReaderAssistantTool(context.Background(), contextInfo, "search_full_text", `{"query":"本文","maxResults":10}`)
	fullTextMatches := fullTextResult.Result["matches"].([]map[string]any)
	if fullTextResult.Name != "search_full_text" || len(fullTextMatches) == 0 || fullTextMatches[0]["hitId"] == "" {
		t.Fatalf("search_full_text should return registered hits: %+v", fullTextResult)
	}
	if fullTextResult.Result["returnedCount"] != len(fullTextMatches) || fullTextResult.Result["candidateCount"].(int) < len(fullTextMatches) {
		t.Fatalf("search_full_text should expose result counts: %+v", fullTextResult)
	}
	if len(fullTextResult.Result["topMatches"].([]map[string]any)) == 0 {
		t.Fatalf("search_full_text should expose top matches: %+v", fullTextResult)
	}
	if metadata, ok := fullTextResult.Result["metadata"].(map[string]any); !ok || metadata["candidateCount"] != fullTextResult.Result["candidateCount"] {
		t.Fatalf("search_full_text should expose metadata counts: %+v", fullTextResult)
	}
	truncatedResult := server.executeReaderAssistantTool(context.Background(), contextInfo, "search_full_text", `{"query":"本文","maxResults":1}`)
	truncatedMatches := truncatedResult.Result["matches"].([]map[string]any)
	if truncatedResult.Result["returnedCount"] != len(truncatedMatches) || truncatedResult.Result["candidateCount"].(int) < len(truncatedMatches) {
		t.Fatalf("truncated search should expose consistent counts: %+v", truncatedResult)
	}
	if truncatedResult.Result["candidateCount"].(int) > 1 && truncatedResult.Result["truncated"] != true {
		t.Fatalf("truncated search should flag truncation: %+v", truncatedResult)
	}
	if truncatedResult.Result["candidateCount"].(int) > 1 && truncatedResult.Result["topMatchesTruncated"] != true {
		t.Fatalf("truncated search should flag top match truncation: %+v", truncatedResult)
	}
	passageResult := server.executeReaderAssistantTool(context.Background(), contextInfo, "load_passages", fmt.Sprintf(`{"hitIds":[%q],"contextChars":400}`, fullTextMatches[0]["hitId"]))
	if passageResult.Name != "load_passages" || len(passageResult.Result["passages"].([]map[string]any)) != 1 {
		t.Fatalf("load_passages should load hit context: %+v", passageResult)
	}
	passage := passageResult.Result["passages"].([]map[string]any)[0]
	passageRange := passage["range"].(map[string]any)
	passageStart := passageRange["start"].(int)
	passageEnd := passageRange["end"].(int)
	passagePosition := passage["position"].(int)
	passageText := passage["text"].(string)
	if passageStart > passagePosition || passageEnd <= passagePosition || !strings.Contains(passageText, "本文") {
		t.Fatalf("load_passages should return Japanese hit context with a covering range: %+v", passage)
	}
	missingPassage := server.executeReaderAssistantTool(context.Background(), contextInfo, "load_passages", `{"hitIds":["missing"],"contextChars":400}`)
	if missingPassage.Name != "tool_recovery" || missingPassage.Result["toolName"] != "load_passages" {
		t.Fatalf("missing hit should return tool recovery: %+v", missingPassage)
	}
	badFullText := server.executeReaderAssistantTool(context.Background(), contextInfo, "search_full_text", `{"query":"本文","maxResults":51}`)
	if badFullText.Name != "tool_recovery" || badFullText.Result["toolName"] != "search_full_text" {
		t.Fatalf("bad full text arguments should return tool recovery: %+v", badFullText)
	}
	longQuery := strings.Repeat("あ", readerAssistantMaxFullTextQueryRunes+1)
	badFullTextQuery := server.executeReaderAssistantTool(context.Background(), contextInfo, "search_full_text", fmt.Sprintf(`{"query":%q}`, longQuery))
	if badFullTextQuery.Name != "tool_recovery" || badFullTextQuery.Result["toolName"] != "search_full_text" {
		t.Fatalf("long full text query should return tool recovery: %+v", badFullTextQuery)
	}
	manyTermQuery := "one two three four five six seven"
	badFullTextTerms := server.executeReaderAssistantTool(context.Background(), contextInfo, "search_full_text", fmt.Sprintf(`{"query":%q}`, manyTermQuery))
	if badFullTextTerms.Name != "tool_recovery" || badFullTextTerms.Result["toolName"] != "search_full_text" {
		t.Fatalf("full text query with too many terms should return tool recovery: %+v", badFullTextTerms)
	}
	badPassages := server.executeReaderAssistantTool(context.Background(), contextInfo, "load_passages", `{"hitIds":[]}`)
	if badPassages.Name != "tool_recovery" || badPassages.Result["toolName"] != "load_passages" {
		t.Fatalf("bad passage arguments should return tool recovery: %+v", badPassages)
	}
	recovery := server.executeReaderAssistantTool(context.Background(), contextInfo, "load_episode", `{"episodeIndex":"999"}`)
	if recovery.Name != "tool_recovery" || recovery.Result["toolName"] != "load_episode" {
		t.Fatalf("out-of-bound load should return tool recovery: %+v", recovery)
	}
	badSearch := server.executeReaderAssistantTool(context.Background(), contextInfo, "search_episodes", `{}`)
	if badSearch.Name != "tool_recovery" {
		t.Fatalf("blank search should return tool recovery: %+v", badSearch)
	}
	if _, _, err := resolveReaderAssistantSearchRange(contextInfo, map[string]any{"startEpisodeNumber": 1, "endEpisodeNumber": 99}); err == nil {
		t.Fatal("search range beyond current episode should return an error")
	}
	if _, _, err := resolveReaderAssistantEpisodeRange(contextInfo, map[string]any{"startEpisodeNumber": 1.5}); err == nil {
		t.Fatal("fractional episode number should return an error")
	}
	if readerAssistantRecentPreviousEpisodeCount("現在話も含めて直近5話") != 0 {
		t.Fatal("current-inclusive recent request should not become previous-only")
	}
	contextInfo.History = []map[string]string{{"role": "user", "text": "前の質問"}}
	if input := buildReaderAssistantInput(contextInfo); !strings.Contains(input, "直近の会話履歴") {
		t.Fatalf("agent input should include history: %s", input)
	}
	contextInfo.CurrentEpisodeNumber = 0
	instructions := buildReaderAssistantInstructions(contextInfo)
	if !strings.Contains(instructions, "現在位置: 第1話まで") {
		t.Fatalf("instructions should fall back to episode 1 label: %s", instructions)
	}
	if !strings.Contains(instructions, "get_character_snapshot が未生成または情報不足なら") || !strings.Contains(instructions, "get_term_snapshot が未生成または情報不足なら") || !strings.Contains(instructions, "search_full_text") {
		t.Fatalf("instructions should guide concrete term fallback search: %s", instructions)
	}
	if args := decodeToolArguments(`bad json`); len(args) != 0 {
		t.Fatalf("invalid tool arguments should decode to empty map: %+v", args)
	}
	if jsonText := mustJSON(make(chan int)); jsonText != "{}" {
		t.Fatalf("unmarshalable tool result should fall back to empty object: %s", jsonText)
	}
	previous := server.executeReaderAssistantTool(context.Background(), contextInfo, "get_previous_episode", `{}`)
	if previous.Name != "get_previous_episode" || previous.Result["status"] != "not_available" {
		t.Fatalf("first episode previous tool should be not_available: %+v", previous)
	}
	snapshot := server.executeReaderAssistantTool(context.Background(), contextInfo, "get_character_snapshot", `{}`)
	if snapshot.Name != "get_character_snapshot" || snapshot.Result["status"] == "" {
		t.Fatalf("character snapshot tool should return a status: %+v", snapshot)
	}
	if snapshot.Result["status"] == "not_generated" && snapshot.Result["fallbackTool"] != "search_full_text" {
		t.Fatalf("not_generated character snapshot should suggest full text fallback: %+v", snapshot)
	}
	termSnapshot := server.executeReaderAssistantTool(context.Background(), contextInfo, "get_term_snapshot", `{}`)
	if termSnapshot.Name != "get_term_snapshot" || termSnapshot.Result["status"] == "" {
		t.Fatalf("term snapshot tool should return a status: %+v", termSnapshot)
	}
	if termSnapshot.Result["status"] == "not_generated" && termSnapshot.Result["fallbackTool"] != "search_full_text" {
		t.Fatalf("not_generated term snapshot should suggest full text fallback: %+v", termSnapshot)
	}
	unsupported := server.executeReaderAssistantTool(context.Background(), contextInfo, "missing_tool", `{}`)
	if unsupported.Name != "tool_recovery" {
		t.Fatalf("unsupported tool should return recovery: %+v", unsupported)
	}
}

func TestReaderAssistantFullTextToolValidationBranches(t *testing.T) {
	contextInfo := readerAssistantContext{
		NovelID:              "novel-a",
		CurrentEpisodeIndex:  "2",
		CurrentEpisodeNumber: 2,
		TocEpisodes: []library.TocEpisodeSummary{
			{EpisodeIndex: "1", Title: "第一話"},
			{EpisodeIndex: "2", Title: "第二話"},
		},
		HitRegistry: newReaderAssistantHitRegistry(),
	}
	if start, end, err := resolveReaderAssistantFullTextSearchRange(contextInfo, map[string]any{}); err != nil || start != 1 || end != 2 {
		t.Fatalf("default full text range = %d..%d err=%v", start, end, err)
	}
	if _, _, err := resolveReaderAssistantFullTextSearchRange(contextInfo, map[string]any{"endEpisodeNumber": 3}); err == nil {
		t.Fatal("full text range should reject spoiler boundary overflow")
	}
	if maxResults, err := readerAssistantMaxResultsArg(nil); err != nil || maxResults != readerAssistantDefaultFullTextResults {
		t.Fatalf("default maxResults = %d err=%v", maxResults, err)
	}
	if _, err := readerAssistantMaxResultsArg(float64(51)); err == nil {
		t.Fatal("maxResults should reject values above the limit")
	}
	if query, err := readerAssistantFullTextQueryArg("  alpha beta  "); err != nil || query != "alpha beta" {
		t.Fatalf("full text query normalization = %q err=%v", query, err)
	}
	if _, err := readerAssistantFullTextQueryArg(strings.Repeat("x", readerAssistantMaxFullTextQueryRunes+1)); err == nil {
		t.Fatal("full text query should reject long values")
	}
	if _, err := readerAssistantFullTextQueryArg("one two three four five six seven"); err == nil {
		t.Fatal("full text query should reject too many terms")
	}
	if contextChars, err := readerAssistantContextCharsArg("1200"); err != nil || contextChars != 1200 {
		t.Fatalf("contextChars string arg = %d err=%v", contextChars, err)
	}
	if _, err := readerAssistantContextCharsArg(float64(1.5)); err == nil {
		t.Fatal("contextChars should reject fractional numbers")
	}
	hitIDs, err := readerAssistantHitIDsArg([]any{"hit-1", "hit-1", "hit-2"})
	if err != nil || len(hitIDs) != 2 {
		t.Fatalf("hitIds should deduplicate non-empty values: ids=%+v err=%v", hitIDs, err)
	}
	if _, err := readerAssistantHitIDsArg([]any{"a", "b", "c", "d", "e", "f"}); err == nil {
		t.Fatal("hitIds should reject more than five ids")
	}
	terms := readerAssistantSearchTerms("alpha beta alpha")
	if len(terms) != 2 || terms[0] != "alpha" || terms[1] != "beta" {
		t.Fatalf("unexpected terms: %+v", terms)
	}
	limitedTerms := readerAssistantSearchTerms("one two three four five six seven")
	if len(limitedTerms) != readerAssistantMaxFullTextTerms {
		t.Fatalf("search terms should be capped: %+v", limitedTerms)
	}
	positions := readerAssistantFindQueryPositions("alpha beta alpha gamma", "alpha beta", terms, 3)
	if len(positions) == 0 || positions[0] != 0 {
		t.Fatalf("unexpected query positions: %+v", positions)
	}
	cooccurrencePositions := readerAssistantFindQueryPositions("alpha "+strings.Repeat("x", 300)+" beta alpha together.", "missing phrase", terms, 3)
	if len(cooccurrencePositions) != 3 || cooccurrencePositions[1] <= 0 {
		t.Fatalf("multi-term fallback should collect term candidates across the episode: %+v", cooccurrencePositions)
	}
	if isolatedPositions := readerAssistantFindQueryPositions("alpha "+strings.Repeat("x", 600)+" beta only there.", "missing phrase", terms, 3); len(isolatedPositions) != 2 {
		t.Fatalf("multi-term fallback should still keep recall for isolated term hits: %+v", isolatedPositions)
	}
	if score := readerAssistantTitleScore("alpha title", "alpha", terms); score <= 0 {
		t.Fatalf("title score should be positive: %f", score)
	}
	if score := readerAssistantFullTextScore("alpha beta gamma", 0, "alpha", terms, 0); score <= 1 {
		t.Fatalf("full text score should include exact match bonuses: %f", score)
	}
	if cooccurrenceScore, isolatedScore := readerAssistantFullTextScore("alpha beta gamma", 0, "missing", terms, 0), readerAssistantFullTextScore("alpha "+strings.Repeat("x", 600)+" beta", 0, "missing", terms, 0); cooccurrenceScore <= isolatedScore {
		t.Fatalf("co-occurring terms should score above isolated term hits: cooccurrence=%f isolated=%f", cooccurrenceScore, isolatedScore)
	}
	coverageCandidates := readerAssistantCoverageFullTextCandidates([]readerAssistantFullTextCandidate{
		{Episode: &library.EpisodeResponse{EpisodeIndex: "1"}, Number: 1, Position: 0, Score: 10},
		{Episode: &library.EpisodeResponse{EpisodeIndex: "2"}, Number: 2, Position: 0, Score: 9},
		{Episode: &library.EpisodeResponse{EpisodeIndex: "50"}, Number: 50, Position: 0, Score: 8},
		{Episode: &library.EpisodeResponse{EpisodeIndex: "100"}, Number: 100, Position: 0, Score: 7},
	}, []readerAssistantFullTextCandidate{
		{Episode: &library.EpisodeResponse{EpisodeIndex: "1"}, Number: 1, Position: 0, Score: 10},
	}, 4)
	if len(coverageCandidates) < 2 || coverageCandidates[0].Number == 1 {
		t.Fatalf("coverage candidates should add non-top hits across the range: %+v", coverageCandidates)
	}
	matchedEpisodeCount, firstMatchedEpisodeNumber, lastMatchedEpisodeNumber := readerAssistantFullTextDistribution(coverageCandidates)
	if matchedEpisodeCount == 0 || firstMatchedEpisodeNumber == nil || lastMatchedEpisodeNumber == nil {
		t.Fatalf("full text distribution should expose matched episode span")
	}
	if length := readerAssistantMatchLengthAt("alpha beta", 0, "alpha", terms); length != len([]rune("alpha")) {
		t.Fatalf("match length = %d", length)
	}
	if length := readerAssistantMatchLengthAt("beta alpha", 0, "missing", terms); length != len([]rune("beta")) {
		t.Fatalf("term fallback match length = %d", length)
	}
	if length := readerAssistantMatchLengthAt("gamma", 0, "missing", terms); length != 0 {
		t.Fatalf("missing match length = %d", length)
	}
	start, end := readerAssistantPassageRange("0123456789", 1, 2, 6)
	if start != 0 || end <= start {
		t.Fatalf("unexpected passage range near start: %d..%d", start, end)
	}
	start, end = readerAssistantPassageRange("0123456789", 20, 1, 6)
	if end != len([]rune("0123456789")) || start >= end {
		t.Fatalf("unexpected passage range past end: %d..%d", start, end)
	}
	if text := substringRunes("あいうえお", 1, 3); text != "いう" {
		t.Fatalf("substringRunes returned %q", text)
	}
	if text := substringRunes("abc", 5, 2); text != "" {
		t.Fatalf("out-of-range substringRunes returned %q", text)
	}
	if index := byteIndexForRuneOffset("あa", 1); index != len("あ") {
		t.Fatalf("byteIndexForRuneOffset returned %d", index)
	}
	if index := byteIndexForRuneOffset("abc", 99); index != -1 {
		t.Fatalf("missing byteIndexForRuneOffset returned %d", index)
	}
	if !intSliceContains([]int{1, 2}, 2) || intSliceContains([]int{1, 2}, 3) {
		t.Fatal("intSliceContains returned unexpected result")
	}
	if value := stringValue(testStringPtr("ok")); value != "ok" {
		t.Fatalf("stringValue returned %q", value)
	}
	if value := stringValue(nil); value != "" {
		t.Fatalf("nil stringValue returned %q", value)
	}
}

func TestReaderAssistantLoadPassagesRecoveryBranches(t *testing.T) {
	dataDir := newHTTPAPITestData(t)
	stateStore := store.New(dataDir)
	if err := stateStore.Initialize(); err != nil {
		t.Fatalf("initialize store: %v", err)
	}
	handler := newTestServerWithLibraryAndStore(dataDir, library.NewService(filepath.Join(dataDir, "novel-fetcher")), stateStore)
	server := handler.(*Server)
	novels := requestJSON(t, handler, http.MethodGet, "/api/library/novels", nil, http.StatusOK)
	novelID := novels["novels"].([]any)[0].(map[string]any)["novelId"].(string)
	toc, err := server.library.GetToc(context.Background(), novelID)
	if err != nil || toc == nil || len(toc.Episodes) == 0 {
		t.Fatalf("load toc: toc=%+v err=%v", toc, err)
	}
	current, err := server.library.GetEpisode(context.Background(), novelID, toc.Episodes[0].EpisodeIndex)
	if err != nil || current == nil {
		t.Fatalf("load current episode: episode=%+v err=%v", current, err)
	}
	contextInfo := readerAssistantContext{
		NovelID:              novelID,
		NovelTitle:           toc.Title,
		CurrentEpisodeIndex:  current.EpisodeIndex,
		CurrentEpisodeNumber: 1,
		TocEpisodes:          toc.Episodes,
	}
	noSearch := server.loadPassagesResult(context.Background(), contextInfo, []string{"hit"}, 400)
	if noSearch.Name != "tool_recovery" {
		t.Fatalf("load_passages without registry should recover: %+v", noSearch)
	}

	contextInfo.HitRegistry = newReaderAssistantHitRegistry()
	contextInfo.HitRegistry.Hits["wrong-novel"] = readerAssistantSearchHit{
		NovelID:         "other",
		MaxEpisodeIndex: current.EpisodeIndex,
		EpisodeIndex:    current.EpisodeIndex,
		EpisodeNumber:   1,
		Title:           current.Title,
		Position:        0,
		ContentEtag:     current.ContentEtag,
	}
	wrongNovel := server.loadPassagesResult(context.Background(), contextInfo, []string{"wrong-novel"}, 400)
	if wrongNovel.Name != "tool_recovery" {
		t.Fatalf("wrong novel hit should recover: %+v", wrongNovel)
	}

	contextInfo.HitRegistry.Hits["changed"] = readerAssistantSearchHit{
		NovelID:         novelID,
		MaxEpisodeIndex: current.EpisodeIndex,
		EpisodeIndex:    current.EpisodeIndex,
		EpisodeNumber:   1,
		Title:           current.Title,
		Position:        0,
		ContentEtag:     "old-etag",
	}
	contentChanged := server.loadPassagesResult(context.Background(), contextInfo, []string{"changed"}, 400)
	if contentChanged.Name != "tool_recovery" {
		t.Fatalf("changed content hit should recover: %+v", contentChanged)
	}

	nilEpisode := (&Server{}).readerAssistantEpisodeText(context.Background(), "missing", "1", newReaderAssistantHitRegistry())
	if nilEpisode != nil {
		t.Fatalf("missing episode text should be nil: %+v", nilEpisode)
	}
	noMatches := server.searchFullTextResult(context.Background(), readerAssistantContext{
		NovelID:              novelID,
		CurrentEpisodeIndex:  current.EpisodeIndex,
		CurrentEpisodeNumber: 1,
		TocEpisodes:          toc.Episodes,
	}, "zzzzzz-no-match", 1, 1, 5)
	if len(noMatches["matches"].([]map[string]any)) != 0 {
		t.Fatalf("unexpected full text matches: %+v", noMatches)
	}
}

func TestAIGenerationEnabledRoutesUseSettingsEndpointAndFakeOpenRouter(t *testing.T) {
	t.Setenv("AI_GENERATION_SETTINGS_MASTER_PASSPHRASE", "test-passphrase")
	providerCalls := 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		providerCalls++
		if r.Header.Get("authorization") != "Bearer sk-contract-dummy" {
			t.Fatalf("unexpected authorization: %q", r.Header.Get("authorization"))
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read OpenRouter request: %v", err)
		}
		content := "Reader assistant fake answer"
		promptTokens := 13
		completionTokens := 5
		if strings.Contains(string(raw), `"response_format"`) {
			content = `{"processedUpToEpisodeIndex":"ep1","newCharacters":[{"canonicalName":{"text":"ミラ","episodeIndex":"ep1"},"fullName":null,"fullNameHistory":[],"gender":null,"genderHistory":[],"firstAppearanceEpisodeIndex":"ep1","aliases":[],"appearanceHistory":[],"personalityHistory":[],"summaryHistory":[{"text":"mock OpenRouter summary","episodeIndex":"ep1"}]}],"characterUpdates":[],"mergeProposals":[],"unresolvedMentions":[],"terms":[]}`
			promptTokens = 29
			completionTokens = 11
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{
				"finish_reason": "stop",
				"message": map[string]any{
					"content": content,
				},
			}},
			"usage": map[string]any{
				"prompt_tokens":     promptTokens,
				"completion_tokens": completionTokens,
				"total_tokens":      promptTokens + completionTokens,
			},
		})
	}))
	defer provider.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", provider.URL)

	dataDir := newHTTPAPITestData(t)
	if err := os.RemoveAll(filepath.Join(dataDir, "state", "character_profiles")); err != nil {
		t.Fatalf("remove character profiles: %v", err)
	}
	if err := os.Remove(filepath.Join(dataDir, "state", "ai_usage.sqlite")); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("remove ai usage fixture: %v", err)
	}
	stateStore := store.New(dataDir)
	if err := stateStore.Initialize(); err != nil {
		t.Fatalf("initialize store: %v", err)
	}
	handler := newTestServerWithLibraryAndStore(dataDir, library.NewService(filepath.Join(dataDir, "novel-fetcher")), stateStore)
	requestJSON(t, handler, http.MethodPut, "/api/ai-generation/settings", map[string]any{
		"preferredMode":     "llm",
		"selectedProfileId": "default",
		"sharedProviders": map[string]any{
			"openrouter": map[string]any{"apiKey": "sk-contract-dummy"},
		},
		"profiles": []any{map[string]any{
			"id":                "default",
			"label":             "Default",
			"provider":          "openrouter",
			"credentials":       map[string]any{"source": "shared"},
			"modelId":           "openrouter/mock",
			"requireParameters": true,
		}},
	}, http.StatusOK)

	novels := requestJSON(t, handler, http.MethodGet, "/api/library/novels", nil, http.StatusOK)
	novelID := novels["novels"].([]any)[0].(map[string]any)["novelId"].(string)
	readerStream := requestRaw(t, handler, http.MethodPost, "/api/library/novels/"+novelID+"/reader-assistant/chat/stream", map[string]any{
		"message":             "現在話を確認して",
		"currentEpisodeIndex": "1",
		"position":            0,
	}, http.StatusOK)
	readerEvents := decodeNDJSONEvents(t, readerStream)
	readerResultIndex := eventIndex(readerEvents, "result", "")
	if readerResultIndex < 0 {
		t.Fatalf("reader assistant stream should return result event: %+v", readerEvents)
	}
	readerResponse := readerEvents[readerResultIndex]["response"].(map[string]any)
	if readerResponse["generationMode"] != "remote" || readerResponse["answer"] != "Reader assistant fake answer" {
		t.Fatalf("reader assistant should use fake OpenRouter result: %+v", readerResponse)
	}

	summaryStream := requestRaw(t, handler, http.MethodPost, "/api/ai-generation/playground/extraction/stream", map[string]any{
		"novelId":          novelID,
		"upToEpisodeIndex": "1",
	}, http.StatusOK)
	summaryEvents := decodeNDJSONEvents(t, summaryStream)
	if eventIndex(summaryEvents, "promptPreview", "") < 0 || eventIndex(summaryEvents, "batchTiming", "") < 0 {
		t.Fatalf("character summary stream should include prompt preview and batch timing: %+v", summaryEvents)
	}
	summaryResultIndex := eventIndex(summaryEvents, "result", "")
	if summaryResultIndex < 0 {
		t.Fatalf("character summary stream should return result event: %+v", summaryEvents)
	}
	summaryResult := summaryEvents[summaryResultIndex]["result"].(map[string]any)
	if summaryResult["generationMode"] != "openrouter" {
		t.Fatalf("character summary should use OpenRouter generation mode: %+v", summaryResult)
	}
	summaryCharacters := summaryResult["characters"].([]any)
	if len(summaryCharacters) != 1 || summaryCharacters[0].(map[string]any)["canonicalName"] != "ミラ" {
		t.Fatalf("character summary should return fake OpenRouter character: %+v", summaryResult)
	}

	usage := requestJSON(t, handler, http.MethodGet, "/api/ai-generation/usage", nil, http.StatusOK)
	runs := usage["runs"].([]any)
	features := map[string]bool{}
	for _, rawRun := range runs {
		run := rawRun.(map[string]any)
		features[run["feature"].(string)] = true
	}
	if !features["reader-assistant"] || !features["extraction"] {
		t.Fatalf("usage should include reader assistant and character summary runs: %+v", usage)
	}
	if providerCalls < 2 {
		t.Fatalf("fake OpenRouter should be called by both enabled routes, calls=%d", providerCalls)
	}
}

func TestReaderAssistantReportsConfiguredOpenRouterFailure(t *testing.T) {
	t.Setenv("AI_GENERATION_SETTINGS_MASTER_PASSPHRASE", "test-passphrase")
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"bad request"}}`))
	}))
	defer provider.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", provider.URL)

	dataDir := newHTTPAPITestData(t)
	stateStore := store.New(dataDir)
	if err := stateStore.Initialize(); err != nil {
		t.Fatalf("initialize store: %v", err)
	}
	modelID := "openrouter/auto"
	if _, err := stateStore.PutAIGenerationSettings(store.AIGenerationSettingsUpdate{
		PreferredMode: testStringPtr("llm"),
		SharedProviders: &store.AISharedProvidersInput{
			OpenRouter: store.AIProviderCredentialInput{APIKey: testStringPtr("sk-reader-secret"), APIKeySet: true},
		},
		ProfilesSet: true,
		Profiles: []store.AIProfileInput{
			{
				ID:                "default",
				Label:             "Default",
				Provider:          "openrouter",
				Credentials:       store.AIProfileCredentialsInput{Source: "shared"},
				ModelID:           &modelID,
				RequireParameters: true,
			},
		},
	}); err != nil {
		t.Fatalf("put AI settings: %v", err)
	}
	handler := newTestServerWithLibraryAndStore(dataDir, library.NewService(filepath.Join(dataDir, "novel-fetcher")), stateStore)
	novels := requestJSON(t, handler, http.MethodGet, "/api/library/novels", nil, http.StatusOK)
	novelID := novels["novels"].([]any)[0].(map[string]any)["novelId"].(string)
	response := requestJSON(t, handler, http.MethodPost, "/api/library/novels/"+novelID+"/reader-assistant/chat", map[string]any{
		"message":             "hello",
		"currentEpisodeIndex": "1",
		"position":            0,
	}, http.StatusBadGateway)
	if !strings.Contains(response["error"].(string), "OpenRouter responded with 400") {
		t.Fatalf("unexpected reader assistant OpenRouter failure: %+v", response)
	}
	stream := requestRaw(t, handler, http.MethodPost, "/api/library/novels/"+novelID+"/reader-assistant/chat/stream", map[string]any{
		"message":             "hello",
		"currentEpisodeIndex": "1",
		"position":            0,
	}, http.StatusOK)
	if !strings.Contains(stream, `"type":"error"`) || !strings.Contains(stream, `OpenRouter responded with 400`) {
		t.Fatalf("unexpected reader assistant failure stream: %s", stream)
	}
}

func TestReaderAssistantUsesRequestedPositionForCurrentExcerpt(t *testing.T) {
	t.Setenv("AI_GENERATION_SETTINGS_MASTER_PASSPHRASE", "test-passphrase")
	providerCalls := 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		providerCalls++
		if providerCalls == 1 {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []any{map[string]any{
					"finish_reason": "tool_calls",
					"message": map[string]any{
						"tool_calls": []any{map[string]any{
							"id":   "call_current",
							"type": "function",
							"function": map[string]any{
								"name":      "get_current_episode",
								"arguments": `{}`,
							},
						}},
					},
				}},
				"usage": map[string]any{"prompt_tokens": 10, "completion_tokens": 2, "total_tokens": 12},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{
				"message": map[string]any{"content": "現在位置を確認しました。"},
			}},
			"usage": map[string]any{"prompt_tokens": 10, "completion_tokens": 4, "total_tokens": 14},
		})
	}))
	defer provider.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", provider.URL)

	dataDir := newHTTPAPITestData(t)
	prefix := strings.Repeat("先頭", 500)
	marker := "現在位置の合図"
	longText := prefix + marker + strings.Repeat("末尾", 500)
	rawEpisode, err := json.Marshal(map[string]any{
		"schema_version":  1,
		"episode_id":      "1",
		"site_episode_id": "1",
		"source_url":      "https://ncode.syosetu.com/n1234/1/",
		"sort_order":      0,
		"display_index":   "1",
		"title":           "Episode 1",
		"chapter":         "Chapter",
		"subchapter":      "",
		"published_at":    "2026-01-01T00:00:00Z",
		"updated_at":      "2026-01-02T00:00:00Z",
		"blocks":          []map[string]any{{"type": "paragraph", "section": "body", "text": longText}},
		"fetched_at":      "2026-01-02T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("marshal long episode: %v", err)
	}
	writeHTTPFixtureFile(t, filepath.Join(dataDir, "novel-fetcher", "works", "syosetu", "n1234", "episodes", "1.json"), string(rawEpisode))

	stateStore := store.New(dataDir)
	if err := stateStore.Initialize(); err != nil {
		t.Fatalf("initialize store: %v", err)
	}
	modelID := "openrouter/auto"
	if _, err := stateStore.PutAIGenerationSettings(store.AIGenerationSettingsUpdate{
		PreferredMode: testStringPtr("llm"),
		SharedProviders: &store.AISharedProvidersInput{
			OpenRouter: store.AIProviderCredentialInput{APIKey: testStringPtr("sk-reader-secret"), APIKeySet: true},
		},
		ProfilesSet: true,
		Profiles: []store.AIProfileInput{{
			ID:          "default",
			Label:       "Default",
			Provider:    "openrouter",
			Credentials: store.AIProfileCredentialsInput{Source: "shared"},
			ModelID:     &modelID,
		}},
	}); err != nil {
		t.Fatalf("put AI settings: %v", err)
	}
	handler := newTestServerWithLibraryAndStore(dataDir, library.NewService(filepath.Join(dataDir, "novel-fetcher")), stateStore)
	novels := requestJSON(t, handler, http.MethodGet, "/api/library/novels", nil, http.StatusOK)
	novelID := novels["novels"].([]any)[0].(map[string]any)["novelId"].(string)
	response := requestJSON(t, handler, http.MethodPost, "/api/library/novels/"+novelID+"/reader-assistant/chat", map[string]any{
		"message":             "現在位置について",
		"currentEpisodeIndex": "1",
		"position":            len([]rune(prefix)),
	}, http.StatusOK)
	currentTool := response["toolResults"].([]any)[0].(map[string]any)
	currentResult := currentTool["result"].(map[string]any)
	excerpt := currentResult["excerpt"].(string)
	if !strings.Contains(excerpt, marker) {
		t.Fatalf("current excerpt should be centered around requested position: %q", excerpt)
	}
}

func TestPlaygroundExtractionUsesConfiguredOpenRouterProvider(t *testing.T) {
	t.Setenv("AI_GENERATION_SETTINGS_MASTER_PASSPHRASE", "test-passphrase")
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("authorization") != "Bearer sk-summary-secret" {
			t.Fatalf("unexpected authorization: %q", r.Header.Get("authorization"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{"content": testExtractionResponseContent("アリス", "OpenRouter summary")}}},
			"usage":   map[string]any{"prompt_tokens": 11, "completion_tokens": 7, "total_tokens": 18},
		})
	}))
	defer provider.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", provider.URL)

	dataDir := newHTTPAPITestData(t)
	if err := os.RemoveAll(filepath.Join(dataDir, "state", "character_profiles")); err != nil {
		t.Fatalf("remove character profiles: %v", err)
	}
	if err := os.Remove(filepath.Join(dataDir, "state", "ai_usage.sqlite")); err != nil {
		t.Fatalf("remove ai usage fixture: %v", err)
	}
	stateStore := store.New(dataDir)
	if err := stateStore.Initialize(); err != nil {
		t.Fatalf("initialize store: %v", err)
	}
	modelID := "openrouter/auto"
	if _, err := stateStore.PutAIGenerationSettings(store.AIGenerationSettingsUpdate{
		PreferredMode: testStringPtr("llm"),
		SharedProviders: &store.AISharedProvidersInput{
			OpenRouter: store.AIProviderCredentialInput{APIKey: testStringPtr("sk-summary-secret"), APIKeySet: true},
		},
		ProfilesSet: true,
		Profiles: []store.AIProfileInput{
			{
				ID:                "default",
				Label:             "Default",
				Provider:          "openrouter",
				Credentials:       store.AIProfileCredentialsInput{Source: "shared"},
				ModelID:           &modelID,
				RequireParameters: true,
			},
		},
	}); err != nil {
		t.Fatalf("put AI settings: %v", err)
	}
	handler := newTestServerWithLibraryAndStore(dataDir, library.NewService(filepath.Join(dataDir, "novel-fetcher")), stateStore)
	server := handler.(*Server)
	novels := requestJSON(t, handler, http.MethodGet, "/api/library/novels", nil, http.StatusOK)
	novelID := novels["novels"].([]any)[0].(map[string]any)["novelId"].(string)
	response := requestJSON(t, handler, http.MethodPost, "/api/ai-generation/playground/extraction", map[string]any{
		"novelId":          novelID,
		"upToEpisodeIndex": "1",
	}, http.StatusOK)
	if response["generationMode"] != "openrouter" {
		t.Fatalf("playground should report OpenRouter generation mode: %+v", response)
	}
	characters := response["characters"].([]any)
	if len(characters) != 1 || characters[0].(map[string]any)["canonicalName"] != "アリス" {
		t.Fatalf("playground should return generated character profile: %+v", response)
	}
	if response["terms"] == nil || len(response["terms"].([]any)) != 0 {
		t.Fatalf("playground should always return the combined terms result: %+v", response)
	}
	if server.extractionCheckpointExists(novelID, "1") {
		t.Fatal("playground preview should not create a checkpoint")
	}
	if _, err := os.Stat(filepath.Join(server.stateDir(), "character_profiles", novelID+".yaml")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("playground preview should not persist reader-facing profiles, stat err=%v", err)
	}
	usage, ok, err := ai.LoadUsage(server.aiUsageDBPath())
	if err != nil || !ok || len(usage.Runs) != 1 {
		t.Fatalf("playground preview usage run should be saved: ok=%v usage=%+v err=%v", ok, usage, err)
	}
	run := usage.Runs[0]
	if run.Status != "completed" || run.InputTokens != 11 || run.OutputTokens != 7 || run.TotalTokens != 18 {
		t.Fatalf("playground preview usage should preserve provider token counts: %+v", run)
	}
	if run.Feature != "extraction" || run.WorkflowName != "extraction" || run.HasSnapshot != true {
		t.Fatalf("playground preview usage should keep character summary metadata: %+v", run)
	}
}

func TestGenerateAndSaveExtractionPreservesOpenRouterTokenUsage(t *testing.T) {
	t.Setenv("AI_GENERATION_SETTINGS_MASTER_PASSPHRASE", "test-passphrase")
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{"content": testExtractionResponseContent("アリス", "OpenRouter summary")}}},
			"usage":   map[string]any{"prompt_tokens": 21, "completion_tokens": 8, "total_tokens": 29},
		})
	}))
	defer provider.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", provider.URL)

	dataDir := newHTTPAPITestData(t)
	if err := os.Remove(filepath.Join(dataDir, "state", "ai_usage.sqlite")); err != nil {
		t.Fatalf("remove ai usage fixture: %v", err)
	}
	stateStore := store.New(dataDir)
	if err := stateStore.Initialize(); err != nil {
		t.Fatalf("initialize store: %v", err)
	}
	modelID := "openrouter/auto"
	if _, err := stateStore.PutAIGenerationSettings(store.AIGenerationSettingsUpdate{
		PreferredMode: testStringPtr("llm"),
		SharedProviders: &store.AISharedProvidersInput{
			OpenRouter: store.AIProviderCredentialInput{APIKey: testStringPtr("sk-summary-secret"), APIKeySet: true},
		},
		ProfilesSet: true,
		Profiles: []store.AIProfileInput{{
			ID:          "default",
			Label:       "Default",
			Provider:    "openrouter",
			Credentials: store.AIProfileCredentialsInput{Source: "shared"},
			ModelID:     &modelID,
		}},
	}); err != nil {
		t.Fatalf("put AI settings: %v", err)
	}
	handler := newTestServerWithLibraryAndStore(dataDir, library.NewService(filepath.Join(dataDir, "novel-fetcher")), stateStore)
	server := handler.(*Server)
	novels := requestJSON(t, handler, http.MethodGet, "/api/library/novels", nil, http.StatusOK)
	novelID := novels["novels"].([]any)[0].(map[string]any)["novelId"].(string)
	_ = os.Remove(filepath.Join(dataDir, "state", "character_profiles", novelID+".yaml"))
	_ = os.Remove(filepath.Join(dataDir, "state", "character_events", novelID+".yaml"))

	if err := server.generateAndSaveExtraction(context.Background(), novelID, "1", nil, nil); err != nil {
		t.Fatalf("generateAndSaveExtraction returned error: %v", err)
	}
	usage, ok, err := ai.LoadUsage(server.aiUsageDBPath())
	if err != nil || !ok || len(usage.Runs) != 1 {
		t.Fatalf("usage run should be saved: ok=%v usage=%+v err=%v", ok, usage, err)
	}
	run := usage.Runs[0]
	if run.InputTokens != 21 || run.OutputTokens != 8 || run.TotalTokens != 29 {
		t.Fatalf("usage run should preserve provider token counts: %+v", run)
	}
	detail, ok, err := ai.LoadUsageRun(server.aiUsageDBPath(), run.RunID)
	if err != nil || !ok {
		t.Fatalf("usage run detail should be readable: ok=%v run=%+v err=%v", ok, run, err)
	}
	if detail.RequestCount != 1 || len(detail.Requests) != 1 || detail.Requests[0].InputTokens != 21 || detail.Requests[0].OutputTokens != 8 || detail.Requests[0].TotalTokens != 29 {
		t.Fatalf("usage request should preserve provider token counts: run=%+v detail=%+v", run, detail)
	}
}

func TestPlaygroundExtractionReportsOpenRouterSchemaMismatch(t *testing.T) {
	t.Setenv("AI_GENERATION_SETTINGS_MASTER_PASSPHRASE", "test-passphrase")
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"choices": [{"message": {"content": "{\"notCharacters\":[]}"}}]
		}`))
	}))
	defer provider.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", provider.URL)

	dataDir := newHTTPAPITestData(t)
	if err := os.RemoveAll(filepath.Join(dataDir, "state", "character_profiles")); err != nil {
		t.Fatalf("remove character profiles: %v", err)
	}
	if err := os.Remove(filepath.Join(dataDir, "state", "ai_usage.sqlite")); err != nil {
		t.Fatalf("remove ai usage fixture: %v", err)
	}
	stateStore := store.New(dataDir)
	if err := stateStore.Initialize(); err != nil {
		t.Fatalf("initialize store: %v", err)
	}
	modelID := "openrouter/auto"
	if _, err := stateStore.PutAIGenerationSettings(store.AIGenerationSettingsUpdate{
		PreferredMode: testStringPtr("llm"),
		SharedProviders: &store.AISharedProvidersInput{
			OpenRouter: store.AIProviderCredentialInput{APIKey: testStringPtr("sk-summary-secret"), APIKeySet: true},
		},
		ProfilesSet: true,
		Profiles: []store.AIProfileInput{
			{
				ID:                "default",
				Label:             "Default",
				Provider:          "openrouter",
				Credentials:       store.AIProfileCredentialsInput{Source: "shared"},
				ModelID:           &modelID,
				RequireParameters: true,
			},
		},
	}); err != nil {
		t.Fatalf("put AI settings: %v", err)
	}
	handler := newTestServerWithLibraryAndStore(dataDir, library.NewService(filepath.Join(dataDir, "novel-fetcher")), stateStore)
	novels := requestJSON(t, handler, http.MethodGet, "/api/library/novels", nil, http.StatusOK)
	novelID := novels["novels"].([]any)[0].(map[string]any)["novelId"].(string)
	response := requestJSON(t, handler, http.MethodPost, "/api/ai-generation/playground/extraction", map[string]any{
		"novelId":          novelID,
		"upToEpisodeIndex": "1",
	}, http.StatusServiceUnavailable)
	if !strings.Contains(response["error"].(string), "モデル出力のrootに契約外field notCharacters があります") {
		t.Fatalf("unexpected schema mismatch response: %+v", response)
	}
	server := handler.(*Server)
	usage, ok, err := ai.LoadUsage(server.aiUsageDBPath())
	if err != nil || !ok || len(usage.Runs) != 1 {
		t.Fatalf("failed playground preview usage run should be saved: ok=%v usage=%+v err=%v", ok, usage, err)
	}
	run := usage.Runs[0]
	if run.Status != "failed" || run.ErrorMessage == nil || !strings.Contains(*run.ErrorMessage, "モデル出力のrootに契約外field notCharacters があります") {
		t.Fatalf("failed playground preview usage should preserve the generation error: %+v", run)
	}
}

func TestPlaygroundExtractionUsesTransientOverrides(t *testing.T) {
	t.Setenv("AI_GENERATION_SETTINGS_MASTER_PASSPHRASE", "test-passphrase")
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode OpenRouter request: %v", err)
		}
		if body["model"] != "openrouter/transient" {
			t.Fatalf("transient modelId should override profile model: %+v", body)
		}
		if body["temperature"] != float64(0.2) || body["max_tokens"] != float64(12000) {
			t.Fatalf("character summary should set generation parameters: %+v", body)
		}
		if _, ok := body["response_format"].(map[string]any); !ok {
			t.Fatalf("character summary should request json_schema response: %+v", body)
		}
		reasoning := body["reasoning"].(map[string]any)
		if reasoning["effort"] != "xhigh" {
			t.Fatalf("transient reasoning effort should be forwarded: %+v", body)
		}
		providerOptions := body["provider"].(map[string]any)
		order := providerOptions["order"].([]any)
		if len(order) != 2 || order[0] != "ProviderA" || order[1] != "ProviderB" || providerOptions["allow_fallbacks"] != true || providerOptions["require_parameters"] != true {
			t.Fatalf("transient provider options should be forwarded: %+v", providerOptions)
		}
		messages := body["messages"].([]any)
		if messages[0].(map[string]any)["content"] != "一時システムプロンプト" {
			t.Fatalf("systemPromptOverride should be forwarded: %+v", messages)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{"content": testExtractionResponseContent("セシル", "一時設定で生成")}}},
		})
	}))
	defer provider.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", provider.URL)

	dataDir := newHTTPAPITestData(t)
	if err := os.RemoveAll(filepath.Join(dataDir, "state", "character_profiles")); err != nil {
		t.Fatalf("remove character profiles: %v", err)
	}
	stateStore := store.New(dataDir)
	if err := stateStore.Initialize(); err != nil {
		t.Fatalf("initialize store: %v", err)
	}
	modelID := "openrouter/base"
	if _, err := stateStore.PutAIGenerationSettings(store.AIGenerationSettingsUpdate{
		PreferredMode: testStringPtr("heuristic"),
		SharedProviders: &store.AISharedProvidersInput{
			OpenRouter: store.AIProviderCredentialInput{APIKey: testStringPtr("sk-summary-secret"), APIKeySet: true},
		},
		ProfilesSet: true,
		Profiles: []store.AIProfileInput{
			{
				ID:                "default",
				Label:             "Default",
				Provider:          "openrouter",
				Credentials:       store.AIProfileCredentialsInput{Source: "shared"},
				ModelID:           &modelID,
				RequireParameters: true,
			},
		},
	}); err != nil {
		t.Fatalf("put AI settings: %v", err)
	}
	handler := newTestServerWithLibraryAndStore(dataDir, library.NewService(filepath.Join(dataDir, "novel-fetcher")), stateStore)
	novels := requestJSON(t, handler, http.MethodGet, "/api/library/novels", nil, http.StatusOK)
	novelID := novels["novels"].([]any)[0].(map[string]any)["novelId"].(string)
	response := requestJSON(t, handler, http.MethodPost, "/api/ai-generation/playground/extraction", map[string]any{
		"novelId":              novelID,
		"upToEpisodeIndex":     "1",
		"modelId":              "openrouter/transient",
		"providerOrder":        "ProviderA, ProviderB",
		"allowFallbacks":       true,
		"requireParameters":    false,
		"reasoningEffort":      "xhigh",
		"systemPromptOverride": "一時システムプロンプト",
	}, http.StatusOK)
	if response["generationMode"] != "openrouter" || response["modelId"] != "openrouter/transient" {
		t.Fatalf("playground should report transient generation metadata: %+v", response)
	}
	reasoningMetadata := response["reasoning"].(map[string]any)
	if reasoningMetadata["requestedEffort"] != "xhigh" || reasoningMetadata["source"] != "request" || reasoningMetadata["requireParameters"] != true {
		t.Fatalf("playground should report requested reasoning metadata: %+v", response)
	}
}

func TestPlaygroundExtractionStreamUsesTransientPromptPreviewAndBatchProgress(t *testing.T) {
	t.Setenv("AI_GENERATION_SETTINGS_MASTER_PASSPHRASE", "test-passphrase")
	t.Setenv("OPENROUTER_REASONING_EFFORT", "high")
	providerCalls := 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		providerCalls++
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode OpenRouter request: %v", err)
		}
		if body["reasoning"].(map[string]any)["effort"] != "high" || body["provider"].(map[string]any)["require_parameters"] != true {
			t.Fatalf("environment reasoning should require provider support: %+v", body)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{"content": testExtractionResponseContent("セシル", "stream生成")}}},
			"usage":   map[string]any{"prompt_tokens": 11, "completion_tokens": 7, "total_tokens": 18},
		})
	}))
	defer provider.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", provider.URL)

	dataDir := newHTTPAPITestData(t)
	if err := os.RemoveAll(filepath.Join(dataDir, "state", "character_profiles")); err != nil {
		t.Fatalf("remove character profiles: %v", err)
	}
	stateStore := store.New(dataDir)
	if err := stateStore.Initialize(); err != nil {
		t.Fatalf("initialize store: %v", err)
	}
	modelID := "openrouter/base"
	if _, err := stateStore.PutAIGenerationSettings(store.AIGenerationSettingsUpdate{
		PreferredMode: testStringPtr("heuristic"),
		SharedProviders: &store.AISharedProvidersInput{
			OpenRouter: store.AIProviderCredentialInput{APIKey: testStringPtr("sk-summary-secret"), APIKeySet: true},
		},
		ProfilesSet: true,
		Profiles: []store.AIProfileInput{
			{
				ID:          "default",
				Label:       "Default",
				Provider:    "openrouter",
				Credentials: store.AIProfileCredentialsInput{Source: "shared"},
				ModelID:     &modelID,
			},
		},
	}); err != nil {
		t.Fatalf("put AI settings: %v", err)
	}
	handler := newTestServerWithLibraryAndStore(dataDir, library.NewService(filepath.Join(dataDir, "novel-fetcher")), stateStore)
	novels := requestJSON(t, handler, http.MethodGet, "/api/library/novels", nil, http.StatusOK)
	novelID := novels["novels"].([]any)[0].(map[string]any)["novelId"].(string)
	stream := requestRaw(t, handler, http.MethodPost, "/api/ai-generation/playground/extraction/stream", map[string]any{
		"novelId":              novelID,
		"upToEpisodeIndex":     "1",
		"modelId":              "openrouter/transient",
		"systemPromptOverride": "stream用プロンプト",
	}, http.StatusOK)
	events := decodeNDJSONEvents(t, stream)
	if providerCalls != 1 {
		t.Fatalf("stream should call OpenRouter once for the fixture batch, calls=%d stream=%s", providerCalls, stream)
	}
	promptIndex := eventIndex(events, "promptPreview", "")
	generatingIndex := eventIndex(events, "status", "generating")
	batchIndex := -1
	for index, event := range events {
		if event["type"] == "status" && event["stage"] == "generating" && event["batchIndex"] != nil {
			batchIndex = index
			break
		}
	}
	timingIndex := eventIndex(events, "batchTiming", "")
	buildingIndex := eventIndex(events, "status", "buildingResponse")
	resultIndex := eventIndex(events, "result", "")
	if promptIndex < 0 || generatingIndex < 0 || batchIndex < 0 || timingIndex < 0 || buildingIndex < 0 || resultIndex < 0 ||
		!(promptIndex < generatingIndex && generatingIndex < batchIndex && batchIndex < timingIndex && timingIndex < buildingIndex && buildingIndex < resultIndex) {
		t.Fatalf("unexpected stream event order: %+v", events)
	}
	preview := events[promptIndex]["preview"].(map[string]any)
	if preview["systemPrompt"] != "stream用プロンプト" {
		t.Fatalf("promptPreview should reflect transient systemPromptOverride: %+v", preview)
	}
	if events[timingIndex]["batchIndex"] != float64(1) || events[timingIndex]["generatedCharacterCount"] != float64(1) {
		t.Fatalf("batchTiming should come from real batch progress: %+v", events[timingIndex])
	}
	if events[batchIndex]["batchIndex"] != float64(1) || events[batchIndex]["batchCount"] != float64(1) {
		t.Fatalf("batch status should include batch metadata: %+v", events[batchIndex])
	}
	result := events[resultIndex]["result"].(map[string]any)
	if result["terms"] == nil || len(result["terms"].([]any)) != 0 {
		t.Fatalf("stream final result should always include terms: %+v", result)
	}
	reasoningMetadata := result["reasoning"].(map[string]any)
	if reasoningMetadata["requestedEffort"] != "high" || reasoningMetadata["source"] != "environment" || reasoningMetadata["requireParameters"] != true {
		t.Fatalf("stream result should report resolved reasoning request: %+v", result)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "state", "character_profiles", novelID+".yaml")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stream playground preview should not persist reader-facing profiles, stat err=%v", err)
	}
}

func TestPlaygroundExtractionRejectsInvalidTransientOverrides(t *testing.T) {
	dataDir := newHTTPAPITestData(t)
	stateStore := store.New(dataDir)
	if err := stateStore.Initialize(); err != nil {
		t.Fatalf("initialize store: %v", err)
	}
	handler := newTestServerWithLibraryAndStore(dataDir, library.NewService(filepath.Join(dataDir, "novel-fetcher")), stateStore)
	novels := requestJSON(t, handler, http.MethodGet, "/api/library/novels", nil, http.StatusOK)
	novelID := novels["novels"].([]any)[0].(map[string]any)["novelId"].(string)
	for _, body := range []map[string]any{
		{"novelId": novelID, "upToEpisodeIndex": "1", "profileId": 10},
		{"novelId": novelID, "upToEpisodeIndex": "1", "providerOrder": 10},
		{"novelId": novelID, "upToEpisodeIndex": "1", "allowFallbacks": "true"},
		{"novelId": novelID, "upToEpisodeIndex": "1", "requireParameters": "false"},
		{"novelId": novelID, "upToEpisodeIndex": "1", "reasoningEffort": "extreme"},
		{"novelId": novelID, "upToEpisodeIndex": "1", "reasoningEffort": "  "},
		{"novelId": novelID, "upToEpisodeIndex": "1", "systemPromptOverride": "  "},
	} {
		response := requestJSON(t, handler, http.MethodPost, "/api/ai-generation/playground/extraction", body, http.StatusBadRequest)
		if response["error"] != "一時 AI 生成設定が不正です。" {
			t.Fatalf("unexpected transient validation response: body=%+v response=%+v", body, response)
		}
	}
}

func TestExtractionBatchUsageRequestsUseBatchCount(t *testing.T) {
	requests := extractionBatchUsageRequests([]extractionBatch{
		{BatchIndex: 1, Chunks: []extractionChunk{{Text: "あいうえ"}, {Text: "かき"}}},
		{BatchIndex: 2, Chunks: []extractionChunk{{Text: "さしすせそ"}}},
	})
	if len(requests) != 2 || requests[0].RequestIndex != 0 || requests[1].RequestIndex != 1 {
		t.Fatalf("usage requests should follow batches, got %+v", requests)
	}
	if requests[0].InputTokens != estimateTokenCount("あいうえ")+estimateTokenCount("かき") || requests[1].InputTokens != estimateTokenCount("さしすせそ") {
		t.Fatalf("usage request tokens should be summed by batch chunks: %+v", requests)
	}
}

func TestExtractionBatchUsageRequestsMatchMaterializedBatches(t *testing.T) {
	dataDir := newHTTPAPITestData(t)
	libraryService := library.NewService(filepath.Join(dataDir, "novel-fetcher"))
	handler := newTestServerWithLibraryAndStore(dataDir, libraryService, store.New(dataDir))
	server := handler.(*Server)
	novels := requestJSON(t, handler, http.MethodGet, "/api/library/novels", nil, http.StatusOK)
	novelID := novels["novels"].([]any)[0].(map[string]any)["novelId"].(string)
	maxChunkChars, maxBatchChars := extractionLimits()
	inputs, err := server.loadExtractionInputs(context.Background(), novelID, "1", maxChunkChars, maxBatchChars)
	if err != nil {
		t.Fatalf("loadExtractionInputs returned error: %v", err)
	}
	materialized := inputs.Batches
	plannedRequests := extractionBatchUsageRequests(materialized)

	if len(inputs.Episodes) == 0 || len(materialized) == 0 {
		t.Fatalf("summary inputs should include fixture episodes and batches: inputs=%+v", inputs)
	}
	for index, batch := range materialized {
		if batch.BatchIndex != index+1 {
			t.Fatalf("batch index should be sequential: %+v", materialized)
		}
		if batch.BatchCount != len(materialized) {
			t.Fatalf("batch count should be the materialized total: %+v", materialized)
		}
	}
	if len(plannedRequests) != len(materialized) {
		t.Fatalf("usage requests should match materialized batches: requests=%+v materialized=%+v", plannedRequests, materialized)
	}
	for index, request := range plannedRequests {
		if request.RequestIndex != index || request.Kind != "extraction_batch" {
			t.Fatalf("usage request should identify the materialized batch: %+v", plannedRequests)
		}
		expectedInputTokens := 0
		for _, chunk := range materialized[index].Chunks {
			expectedInputTokens += estimateTokenCount(chunk.Text)
		}
		if request.InputTokens != expectedInputTokens || request.TotalTokens != expectedInputTokens {
			t.Fatalf("usage request tokens should be derived from batch chunks: request=%+v batch=%+v", request, materialized[index])
		}
	}
	if total := usageRequestsTotalTokens([]ai.UsageRequest{{InputTokens: 2, OutputTokens: 3}}); total != 5 {
		t.Fatalf("usageRequestsTotalTokens should fall back to input + output, got %d", total)
	}
}

func TestExtractionCheckpointFingerprintCompatibility(t *testing.T) {
	config := &store.ResolvedAIGenerationConfig{
		ProfileID:         "default",
		ModelID:           "openrouter/test-model",
		ProviderOrder:     []string{"anthropic", "openai"},
		AllowFallbacks:    true,
		RequireParameters: true,
		SystemPrompt:      testStringPtr("抽出ルール"),
	}
	batches := []extractionBatch{
		{
			BatchIndex:     1,
			BatchCount:     2,
			EpisodeIndexes: []string{"1"},
			Chunks: []extractionChunk{
				{EpisodeIndex: "1", Title: "第一話", Text: "アリスが村を出る。"},
				{EpisodeIndex: "1", Title: "第一話", Text: "ボブが見送る。"},
			},
		},
		{
			BatchIndex:     2,
			BatchCount:     2,
			EpisodeIndexes: []string{"2"},
			Chunks: []extractionChunk{
				{EpisodeIndex: "2", Title: "第二話", Text: "アリスが森でボブと再会する。"},
			},
		},
	}
	fingerprint := extractionCheckpointFingerprint(config, extractionCheckpointBatchInputs(batches))
	if fingerprint != "76b40f87eebc8e6cc6e09eaaf412f18ba24007b3" {
		t.Fatalf("checkpoint fingerprint should remain compatible, got %s", fingerprint)
	}
	batches[0].BatchCount = 99
	if changed := extractionCheckpointFingerprint(config, extractionCheckpointBatchInputs(batches)); changed != fingerprint {
		t.Fatalf("checkpoint fingerprint should ignore batch count: before=%s after=%s", fingerprint, changed)
	}
}

func extractionGenerationFingerprint(config *store.ResolvedAIGenerationConfig, novelID string, seed []characters.GeneratedCharacter, seedTerms []terms.GeneratedTerm, batches []extractionBatch, unresolved []characters.GeneratedUnresolvedMention) string {
	allocator := characters.NewGeneratedCharacterIDAllocator(novelID, seed)
	inputs := appextraction.CheckpointGenerationInputs(seed, seedTerms, batches, unresolved, allocator)
	return extractionCheckpointFingerprint(config, inputs)
}

func TestCharacterJobProgressHelpers(t *testing.T) {
	job := extractdomain.Job{}
	currentBatchIndex := 2
	batchCount := 4
	generatedCharacterCount := 5
	setCharacterJobProgress(&job, 120, "batchComplete", &currentBatchIndex, &batchCount, &generatedCharacterCount)
	if job.Progress == nil || *job.Progress != 100 || job.ProgressStage == nil || *job.ProgressStage != "batchComplete" {
		t.Fatalf("setCharacterJobProgress should clamp progress and set stage: %+v", job)
	}
	setCharacterJobProgress(&job, 50, "batchComplete", &currentBatchIndex, &batchCount, &generatedCharacterCount)
	if job.Progress == nil || *job.Progress != 100 {
		t.Fatalf("setCharacterJobProgress should not move progress backwards: %+v", job)
	}
	if job.CurrentBatchIndex == nil || *job.CurrentBatchIndex != 2 || job.BatchCount == nil || *job.BatchCount != 4 || job.GeneratedCharacterCount == nil || *job.GeneratedCharacterCount != 5 {
		t.Fatalf("setCharacterJobProgress should preserve batch metadata: %+v", job)
	}
	setCharacterJobProgress(&job, -10, "failed", nil, nil, nil)
	if job.Progress == nil || *job.Progress != 0 || valueOrDefaultInt(nil, 7) != 7 || valueOrDefaultInt(job.Progress, 7) != 0 {
		t.Fatalf("progress helpers should clamp low values and handle nil defaults: %+v", job)
	}
	stage := "batch"
	if valueOrDefaultString(nil, "fallback") != "fallback" || valueOrDefaultString(&stage, "fallback") != "batch" {
		t.Fatal("valueOrDefaultString should handle nil and non-nil values")
	}
	if got := formatTimingFields("stage", "load_inputs", "elapsedMs", 12); got != "stage=load_inputs elapsedMs=12" {
		t.Fatalf("formatTimingFields returned %q", got)
	}
	t.Setenv("VIEWER_EXTRACTION_TIMING_LOG", "")
	if extractionTimingLogEnabled() {
		t.Fatal("character summary timing log should be disabled by default")
	}
	t.Setenv("VIEWER_EXTRACTION_TIMING_LOG", "1")
	if !extractionTimingLogEnabled() {
		t.Fatal("legacy extraction timing log should be enabled by env")
	}
	t.Setenv("VIEWER_EXTRACTION_TIMING_LOG", "0")
	if extractionTimingLogEnabled() {
		t.Fatal("new extraction timing setting should take precedence")
	}
	t.Setenv("VIEWER_EXTRACTION_TIMING_LOG", "1")
	if !extractionTimingLogEnabled() {
		t.Fatal("new extraction timing log should be enabled")
	}
	logExtractionTiming("test", time.Now())
	if characterJobBatchProgressPercent(0, 0) != 70 || characterJobBatchProgressPercent(2, 4) != 62 {
		t.Fatal("characterJobBatchProgressPercent returned an unexpected value")
	}
	if allEpisodeIndexesProcessed(nil, []string{"1"}) || allEpisodeIndexesProcessed([]string{"1"}, nil) || allEpisodeIndexesProcessed([]string{"1", "2"}, []string{"1"}) {
		t.Fatal("allEpisodeIndexesProcessed should reject empty or incomplete processed sets")
	}
	if !allEpisodeIndexesProcessed([]string{"1", "2"}, []string{"2", "1"}) {
		t.Fatal("allEpisodeIndexesProcessed should accept complete processed sets regardless of order")
	}
	if inputs, err := (&Server{}).loadExtractionInputs(context.Background(), "novel-1", "1", 10, 10); err != nil || len(inputs.Batches) != 0 {
		t.Fatalf("nil-library summary input loader should be empty without error: inputs=%+v err=%v", inputs, err)
	}
}

func TestExtractionBatchBudgetUsesModelContext(t *testing.T) {
	if got := extractionTokensFromChars(0); got != 0 {
		t.Fatalf("zero chars should map to zero tokens, got %d", got)
	}
	if got := extractionTokensFromChars(5); got != 5 {
		t.Fatalf("character token estimate should be conservative for Japanese text, got %d", got)
	}
	nilBudget := resolveExtractionBatchBudget(context.Background(), nil, 12001)
	if nilBudget.MaxTextChars != 12001 || nilBudget.MaxTextTokens != 12001 {
		t.Fatalf("nil config should use fallback char and token budget: %+v", nilBudget)
	}
	placeholderBudget := resolveExtractionBatchBudget(context.Background(), &store.ResolvedAIGenerationConfig{
		APIKey:  "sk-test-budget",
		ModelID: "openrouter/budget-model",
	}, 12000)
	if placeholderBudget.MaxTextChars != 12000 || placeholderBudget.MaxTextTokens != 12000 {
		t.Fatalf("placeholder credentials should keep fallback budget: %+v", placeholderBudget)
	}

	modelID := "openrouter/budget-model"
	smallModelID := "openrouter/small-budget-model"
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Fatalf("unexpected model metadata request: path=%s query=%s", r.URL.Path, r.URL.RawQuery)
		}
		switch r.URL.Query().Get("q") {
		case modelID:
			_, _ = w.Write([]byte(`{
				"data": [{
					"id": "openrouter/budget-model",
					"context_length": 128000,
					"top_provider": {
						"context_length": 128000,
						"max_completion_tokens": 16000
					}
				}]
			}`))
		case smallModelID:
			_, _ = w.Write([]byte(`{
				"data": [{
					"id": "openrouter/small-budget-model",
					"context_length": 2600,
					"top_provider": {
						"context_length": 2600
					}
				}]
			}`))
		default:
			t.Fatalf("unexpected model metadata query: %s", r.URL.RawQuery)
		}
	}))
	defer provider.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", provider.URL)

	config := &store.ResolvedAIGenerationConfig{APIKey: "dummy-openrouter-key-realish-budget", ModelID: modelID}
	budget := resolveExtractionBatchBudget(context.Background(), config, 12000)
	if budget.MaxTextTokens <= extractionTokensFromChars(12000) || budget.MaxTextChars != 0 {
		t.Fatalf("model context should expand token batch budget: %+v", budget)
	}

	smallBudget := resolveExtractionBatchBudget(context.Background(), &store.ResolvedAIGenerationConfig{
		APIKey:  "dummy-openrouter-key-realish-small-budget",
		ModelID: smallModelID,
	}, 12000)
	if smallBudget.MaxTextChars != 12000 || smallBudget.MaxTextTokens != 12000 {
		t.Fatalf("too-small model context should keep fallback budget: %+v", smallBudget)
	}

	t.Setenv("EXTRACTION_MAX_BATCH_TOKENS", "1234")
	budget = resolveExtractionBatchBudget(context.Background(), config, 12000)
	if budget.MaxTextTokens != 1234 || budget.MaxTextChars != 0 {
		t.Fatalf("explicit token budget should win: %+v", budget)
	}
}

func TestResolveOpenRouterMaxOutputTokensRejectsContextOverflow(t *testing.T) {
	modelID := "openrouter/context-overflow-model"
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" || r.URL.Query().Get("q") != modelID {
			t.Fatalf("unexpected model metadata request: path=%s query=%s", r.URL.Path, r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{
			"data": [{
				"id": "openrouter/context-overflow-model",
				"context_length": 1000,
				"top_provider": {
					"context_length": 1000,
					"max_completion_tokens": 500
				}
			}]
		}`))
	}))
	defer provider.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", provider.URL)

	if _, err := resolveOpenRouterMaxOutputTokens(context.Background(), "dummy-openrouter-key-realish-overflow", modelID, nil, 12000, 800); !errors.Is(err, errOpenRouterContextTooLarge) {
		t.Fatalf("context overflow should be reported as a hard error, got %v", err)
	}
}

func TestReaderAssistantTokenEstimateIncludesTools(t *testing.T) {
	assistantContext := readerAssistantContext{
		NovelID:             "novel-1",
		NovelTitle:          "長い物語",
		CurrentEpisodeIndex: "1",
		CurrentEpisodeRef:   map[string]any{"episodeIndex": "1", "title": "第一話"},
		Message:             "ここまでの人物関係を確認したい。" + strings.Repeat("あ", 160),
	}
	messages := []ai.ChatMessage{
		{Role: "system", Content: buildReaderAssistantInstructions(assistantContext)},
		{Role: "user", Content: buildReaderAssistantInput(assistantContext)},
	}
	tools := readerAssistantToolDefinitions()
	messagesOnly := estimateChatMessagesTokenCount(messages)
	withTools := estimateOpenRouterChatRequestTokens(messages, tools, nil)
	if withTools <= messagesOnly {
		t.Fatalf("tools schema should increase prompt estimate: messages=%d withTools=%d", messagesOnly, withTools)
	}

	modelID := "openrouter/reader-tools-context-model"
	contextLength := messagesOnly + 320
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/models":
			_, _ = w.Write([]byte(fmt.Sprintf(`{
				"data": [{
					"id": %q,
					"context_length": %d,
					"top_provider": {
						"context_length": %d,
						"max_completion_tokens": 1024
					}
				}]
			}`, modelID, contextLength, contextLength)))
		case "/chat/completions":
			t.Fatal("chat request should not be sent when tools push the prompt over context")
		default:
			t.Fatalf("unexpected OpenRouter path: %s", r.URL.Path)
		}
	}))
	defer provider.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", provider.URL)

	_, _, _, err := (&Server{}).runReaderAssistantAgentLoop(context.Background(), assistantContext, ai.OpenRouterConfig{
		APIKey:  "dummy-openrouter-key-realish-reader-tools",
		ModelID: modelID,
	}, nil)
	if !errors.Is(err, errOpenRouterContextTooLarge) {
		t.Fatalf("reader assistant should reject prompts that overflow after adding tools, got %v", err)
	}
}

func TestOpenRouterTokenEstimateHelpersCoverStructuredPayloads(t *testing.T) {
	messages := []ai.ChatMessage{{
		Role:    "assistant",
		Content: map[string]any{"answer": "本文"},
		ToolCalls: []ai.ToolCall{{
			Function: ai.ToolCallFunction{Name: "load_episode", Arguments: `{"episodeIndex":"1"}`},
		}},
	}}
	messageTokens := estimateChatMessagesTokenCount(messages)
	if messageTokens <= 0 {
		t.Fatalf("structured messages should produce a positive estimate: %d", messageTokens)
	}
	withToolSchema := estimateOpenRouterChatRequestTokens(messages, readerAssistantToolDefinitions(), nil)
	if withToolSchema <= messageTokens {
		t.Fatalf("serialized tool schema should increase estimate: messages=%d withTools=%d", messageTokens, withToolSchema)
	}
	fallbackTokens := estimateOpenRouterChatRequestTokens(messages, nil, map[string]any{"bad": func() {}})
	if fallbackTokens != messageTokens {
		t.Fatalf("unserializable request metadata should fall back to messages estimate: got=%d want=%d", fallbackTokens, messageTokens)
	}
}

func TestExtractionRuntimeBatchesAccountForKnownCharacters(t *testing.T) {
	knownCharacters := []characters.GeneratedCharacter{{
		CanonicalName: "既知人物",
		Summary:       testStringPtr(strings.Repeat("既知情報", 220)),
	}}
	batch := extractionBatch{
		BatchIndex:     1,
		BatchCount:     1,
		EpisodeIndexes: []string{"1", "2"},
		Chunks: []extractionChunk{
			{EpisodeIndex: "1", Title: "第一話", Text: strings.Repeat("前半", 40)},
			{EpisodeIndex: "2", Title: "第二話", Text: strings.Repeat("後半", 40)},
		},
	}
	config := &store.ResolvedAIGenerationConfig{APIKey: "dummy-openrouter-key-realish-known", ModelID: "openrouter/known-character-context"}
	single := extractionRuntimeBatch(batch, []extractionChunk{batch.Chunks[0]})
	systemPrompt, userPrompt := buildExtractionPrompt("novel-1", "2", knownCharacters, single, nil)
	singleTokens := estimateOpenRouterChatRequestTokens([]ai.ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}, nil, extractionOpenRouterResponseFormat())
	contextLength := singleTokens + extractionMinimumCompletionTokens + 320
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(fmt.Sprintf(`{
			"data": [{
				"id": "openrouter/known-character-context",
				"context_length": %d,
				"top_provider": {
					"context_length": %d,
					"max_completion_tokens": 2048
				}
			}]
		}`, contextLength, contextLength)))
	}))
	defer provider.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", provider.URL)

	runtimeBatches, err := (&Server{}).extractionRuntimeBatches(context.Background(), config, "novel-1", "2", knownCharacters, batch)
	if err != nil {
		t.Fatalf("extractionRuntimeBatches returned error: %v", err)
	}
	if len(runtimeBatches) != 2 || len(runtimeBatches[0].Chunks) != 1 || len(runtimeBatches[1].Chunks) != 1 {
		t.Fatalf("known characters should force a too-large later batch to split: %+v", runtimeBatches)
	}
}

func TestNextExtractionRuntimeBatchReturnsRemainingWhenCandidateOverflows(t *testing.T) {
	batch := extractionBatch{
		BatchIndex:     1,
		BatchCount:     1,
		EpisodeIndexes: []string{"1", "2"},
		Chunks: []extractionChunk{
			{EpisodeIndex: "1", Title: "第一話", Text: strings.Repeat("前半", 80)},
			{EpisodeIndex: "2", Title: "第二話", Text: strings.Repeat("後半", 1000)},
		},
	}
	single := extractionRuntimeBatch(batch, []extractionChunk{batch.Chunks[0]})
	systemPrompt, userPrompt := buildExtractionPrompt("novel-1", "2", nil, single, nil)
	singleTokens := estimateOpenRouterChatRequestTokens([]ai.ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}, nil, extractionOpenRouterResponseFormat())
	modelID := "openrouter/next-runtime-context"
	contextLength := singleTokens + extractionMinimumCompletionTokens + 1000
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(fmt.Sprintf(`{
			"data": [{
				"id": %q,
				"context_length": %d,
				"top_provider": {
					"context_length": %d,
					"max_completion_tokens": 2048
				}
			}]
		}`, modelID, contextLength, contextLength)))
	}))
	defer provider.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", provider.URL)

	runtimeBatch, remaining, err := (&Server{}).nextExtractionRuntimeBatch(context.Background(), &store.ResolvedAIGenerationConfig{
		APIKey:  "dummy-openrouter-key-realish-next-runtime",
		ModelID: modelID,
	}, "novel-1", "2", nil, batch, batch.Chunks)
	if err != nil {
		t.Fatalf("nextExtractionRuntimeBatch returned error: %v", err)
	}
	if len(runtimeBatch.Chunks) != 1 || runtimeBatch.Chunks[0].EpisodeIndex != "1" {
		t.Fatalf("first fitting chunk should be returned: %+v", runtimeBatch)
	}
	if len(remaining) != 1 || remaining[0].EpisodeIndex != "2" {
		t.Fatalf("overflowing chunk should remain for the next request: %+v", remaining)
	}
}

func TestNextExtractionRuntimeBatchSplitsOversizedFirstChunk(t *testing.T) {
	chunk := extractionChunk{EpisodeIndex: "1", Title: "第一話", Text: strings.Repeat("長文", 220)}
	trailing := extractionChunk{EpisodeIndex: "2", Title: "第二話", Text: "続き"}
	batch := extractionBatch{
		BatchIndex:     1,
		BatchCount:     1,
		EpisodeIndexes: []string{"1", "2"},
		Chunks:         []extractionChunk{chunk, trailing},
	}
	half := chunk
	half.Text = string([]rune(chunk.Text)[:len([]rune(chunk.Text))/2])
	halfBatch := extractionRuntimeBatch(batch, []extractionChunk{half})
	systemPrompt, userPrompt := buildExtractionPrompt("novel-1", "2", nil, halfBatch, nil)
	halfTokens := estimateOpenRouterChatRequestTokens([]ai.ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}, nil, extractionOpenRouterResponseFormat())
	modelID := "openrouter/next-runtime-split-context"
	contextLength := halfTokens + extractionMinimumCompletionTokens + 320
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(fmt.Sprintf(`{
			"data": [{
				"id": %q,
				"context_length": %d,
				"top_provider": {
					"context_length": %d,
					"max_completion_tokens": 2048
				}
			}]
		}`, modelID, contextLength, contextLength)))
	}))
	defer provider.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", provider.URL)

	runtimeBatch, remaining, err := (&Server{}).nextExtractionRuntimeBatch(context.Background(), &store.ResolvedAIGenerationConfig{
		APIKey:  "dummy-openrouter-key-realish-next-runtime-split",
		ModelID: modelID,
	}, "novel-1", "2", nil, batch, batch.Chunks)
	if err != nil {
		t.Fatalf("nextExtractionRuntimeBatch returned error: %v", err)
	}
	if len(runtimeBatch.Chunks) != 1 || runtimeBatch.Chunks[0].EpisodeIndex != "1" || runtimeBatch.Chunks[0].Text == chunk.Text {
		t.Fatalf("oversized first chunk should be split before sending: %+v", runtimeBatch)
	}
	if len(remaining) < 2 || remaining[len(remaining)-1].EpisodeIndex != "2" {
		t.Fatalf("split remainder and trailing chunks should remain: %+v", remaining)
	}
}

func TestExtractionRuntimeBatchesSmallBranches(t *testing.T) {
	emptyBatch := extractionBatch{BatchIndex: 1, BatchCount: 1}
	emptyResult, err := (&Server{}).extractionRuntimeBatches(context.Background(), nil, "novel-1", "1", nil, emptyBatch)
	if err != nil || len(emptyResult) != 1 {
		t.Fatalf("empty runtime batch should pass through: result=%+v err=%v", emptyResult, err)
	}
	batch := extractionBatch{
		BatchIndex:     1,
		BatchCount:     1,
		EpisodeIndexes: []string{"1"},
		Chunks:         []extractionChunk{{EpisodeIndex: "1", Title: "第一話", Text: "あ"}},
	}
	nilConfigResult, err := (&Server{}).extractionRuntimeBatches(context.Background(), nil, "novel-1", "1", nil, batch)
	if err != nil || len(nilConfigResult) != 1 {
		t.Fatalf("nil config runtime batch should pass through: result=%+v err=%v", nilConfigResult, err)
	}

	modelID := "openrouter/tiny-single-chunk-context"
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(fmt.Sprintf(`{
			"data": [{
				"id": %q,
				"context_length": 32,
				"top_provider": {
					"context_length": 32,
					"max_completion_tokens": 16
				}
			}]
		}`, modelID)))
	}))
	defer provider.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", provider.URL)
	_, err = (&Server{}).extractionRuntimeBatches(context.Background(), &store.ResolvedAIGenerationConfig{
		APIKey:  "dummy-openrouter-key-realish-tiny-single",
		ModelID: modelID,
	}, "novel-1", "1", nil, batch)
	if err == nil || !strings.Contains(err.Error(), "cannot fit in model context") {
		t.Fatalf("unsplittable single-rune chunk should return a clear error, got %v", err)
	}
}

func TestExtractionRuntimeBatchesSplitOversizedSingleChunk(t *testing.T) {
	chunk := extractionChunk{EpisodeIndex: "1", Title: "第一話", Text: strings.Repeat("長文", 220)}
	batch := extractionBatch{
		BatchIndex:     1,
		BatchCount:     1,
		EpisodeIndexes: []string{"1"},
		Chunks:         []extractionChunk{chunk},
	}
	half := chunk
	half.Text = string([]rune(chunk.Text)[:len([]rune(chunk.Text))/2])
	halfBatch := extractionRuntimeBatch(batch, []extractionChunk{half})
	systemPrompt, userPrompt := buildExtractionPrompt("novel-1", "1", nil, halfBatch, nil)
	halfTokens := estimateOpenRouterChatRequestTokens([]ai.ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}, nil, extractionOpenRouterResponseFormat())
	contextLength := halfTokens + extractionMinimumCompletionTokens + 320
	modelID := "openrouter/single-chunk-context"
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(fmt.Sprintf(`{
			"data": [{
				"id": %q,
				"context_length": %d,
				"top_provider": {
					"context_length": %d,
					"max_completion_tokens": 2048
				}
			}]
		}`, modelID, contextLength, contextLength)))
	}))
	defer provider.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", provider.URL)

	runtimeBatches, err := (&Server{}).extractionRuntimeBatches(context.Background(), &store.ResolvedAIGenerationConfig{
		APIKey:  "dummy-openrouter-key-realish-single-chunk",
		ModelID: modelID,
	}, "novel-1", "1", nil, batch)
	if err != nil {
		t.Fatalf("extractionRuntimeBatches returned error: %v", err)
	}
	if len(runtimeBatches) < 2 {
		t.Fatalf("oversized single chunk should be split into smaller runtime batches: %+v", runtimeBatches)
	}
}

func TestRebatchExtractionInputsUsesResolvedBudget(t *testing.T) {
	inputs := extractionInputs{
		Batches: []extractionBatch{{
			BatchIndex:     1,
			BatchCount:     1,
			EpisodeIndexes: []string{"1", "2", "3"},
			Chunks: []extractionChunk{
				{EpisodeIndex: "1", Title: "第一話", Text: "１２３４５６７８"},
				{EpisodeIndex: "2", Title: "第二話", Text: "１２３４５６７８"},
				{EpisodeIndex: "3", Title: "第三話", Text: "１２３４５６７８"},
			},
		}},
	}
	t.Setenv("EXTRACTION_MAX_BATCH_TOKENS", "90")
	rebatched := rebatchExtractionInputs(context.Background(), inputs, &store.ResolvedAIGenerationConfig{
		APIKey:  "dummy-openrouter-key-realish-rebatch",
		ModelID: "openrouter/rebatch-model",
	}, 12000)
	if len(rebatched.Batches) != 2 || len(rebatched.Batches[0].Chunks) != 2 || rebatched.Batches[0].BatchCount != 2 {
		t.Fatalf("rebatch should rebuild batch indexes from token budget: %+v", rebatched.Batches)
	}
}

func TestOpenRouterExtractionUsesModelMaxCompletionTokens(t *testing.T) {
	chatRequests := 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/models":
			_, _ = w.Write([]byte(`{
				"data": [{
					"id": "openrouter/test-model",
					"context_length": 128000,
					"top_provider": {
						"context_length": 128000,
						"max_completion_tokens": 16384
					}
				}]
			}`))
		case "/chat/completions":
			chatRequests++
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode OpenRouter request: %v", err)
			}
			if body["max_tokens"] != float64(16384) {
				t.Fatalf("character summary should use model max completion tokens: %+v", body)
			}
			_, _ = w.Write([]byte(`{
				"choices": [{"message": {"content": "{\"processedUpToEpisodeIndex\":\"1\",\"newCharacters\":[{\"canonicalName\":{\"text\":\"アリス\",\"episodeIndex\":\"1\"},\"fullName\":null,\"fullNameHistory\":[],\"gender\":null,\"genderHistory\":[],\"firstAppearanceEpisodeIndex\":\"1\",\"aliases\":[],\"appearanceHistory\":[],\"personalityHistory\":[],\"summaryHistory\":[{\"text\":\"人物\",\"episodeIndex\":\"1\"}]}],\"characterUpdates\":[],\"mergeProposals\":[],\"unresolvedMentions\":[],\"terms\":[]}"}}],
				"usage": {"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15}
			}`))
		default:
			t.Fatalf("unexpected OpenRouter path: %s", r.URL.Path)
		}
	}))
	defer provider.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", provider.URL)

	server := &Server{dataDir: t.TempDir()}
	config := &store.ResolvedAIGenerationConfig{APIKey: "dummy-openrouter-key-realish-summary", ModelID: "openrouter/test-model"}
	batch := extractionBatch{
		BatchIndex:     1,
		BatchCount:     1,
		EpisodeIndexes: []string{"1"},
		Chunks:         []extractionChunk{{EpisodeIndex: "1", Title: "一話", Text: "アリスが現れた。"}},
	}
	if _, err := server.generateOpenRouterExtractionBatch(context.Background(), config, "novel-1", "1", nil, batch); err != nil {
		t.Fatalf("generateOpenRouterExtractionBatch returned error: %v", err)
	}
	if chatRequests != 1 {
		t.Fatalf("expected one chat request, got %d", chatRequests)
	}
}

func TestOpenRouterExtractionUsesDeltaCandidatesAndStableIDMerges(t *testing.T) {
	modelID := "openrouter/character-delta-candidates"
	chatRequests := 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/models":
			_, _ = w.Write([]byte(fmt.Sprintf(`{
				"data": [{
					"id": %q,
					"context_length": 128000,
					"top_provider": {
						"context_length": 128000,
						"max_completion_tokens": 4096
					}
				}]
			}`, modelID)))
		case "/chat/completions":
			chatRequests++
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode OpenRouter request: %v", err)
			}
			messages := body["messages"].([]any)
			userContent := messages[1].(map[string]any)["content"].(string)
			var prompt map[string]any
			if err := json.Unmarshal([]byte(userContent), &prompt); err != nil {
				t.Fatalf("decode prompt JSON: %v", err)
			}
			if _, exists := prompt["knownCharacters"]; exists {
				t.Fatalf("delta prompt should not send full knownCharacters payload: %+v", prompt)
			}
			episodes := prompt["episodes"].([]any)
			if len(episodes) != 1 || episodes[0].(map[string]any)["episodeIndex"] != "ep1" {
				t.Fatalf("prompt should contain only the requested runtime episode: %+v", episodes)
			}
			candidates := prompt["candidateCharacters"].([]any)
			if len(candidates) != 1 {
				t.Fatalf("prompt should send compact relevant candidates: %+v", prompt)
			}
			candidate := candidates[0].(map[string]any)
			if candidate["characterId"] != "char_seed" || candidate["displayName"] != "アリス" {
				t.Fatalf("candidate card should preserve stable id and display name: %+v", candidate)
			}
			_, _ = w.Write([]byte(`{
				"choices": [{"message": {"content": "{\"processedUpToEpisodeIndex\":\"2\",\"newCharacters\":[{\"canonicalName\":{\"text\":\"クレア\",\"episodeIndex\":\"2\"},\"fullName\":null,\"fullNameHistory\":[],\"gender\":null,\"genderHistory\":[],\"firstAppearanceEpisodeIndex\":\"2\",\"aliases\":[{\"text\":\"クレア\",\"episodeIndex\":\"2\"}],\"appearanceHistory\":[],\"personalityHistory\":[],\"summaryHistory\":[{\"episodeIndex\":\"2\",\"text\":\"新たに同行する。\"}]}],\"characterUpdates\":[{\"characterId\":\"char_seed\",\"canonicalName\":null,\"fullName\":null,\"fullNameHistory\":[],\"gender\":null,\"genderHistory\":[],\"firstAppearanceEpisodeIndex\":null,\"aliases\":[{\"text\":\"アリス\",\"episodeIndex\":\"2\"}],\"appearanceHistory\":[],\"personalityHistory\":[],\"summaryHistory\":[{\"episodeIndex\":\"2\",\"text\":\"クレアを案内する。\"}]}],\"mergeProposals\":[],\"unresolvedMentions\":[],\"terms\":[]}"}}],
				"usage": {"prompt_tokens": 20, "completion_tokens": 6, "total_tokens": 26}
			}`))
		default:
			t.Fatalf("unexpected OpenRouter path: %s", r.URL.Path)
		}
	}))
	defer provider.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", provider.URL)

	server := &Server{dataDir: t.TempDir()}
	config := &store.ResolvedAIGenerationConfig{APIKey: "dummy-openrouter-key-realish-delta", ModelID: modelID}
	seed := []characters.GeneratedCharacter{{
		CharacterID:                 "char_seed",
		CanonicalName:               "アリス",
		CanonicalEpisodeIndex:       "1",
		FirstAppearanceEpisodeIndex: "1",
		Aliases:                     []characters.GeneratedTextVersion{{Text: "アリス", EpisodeIndex: "1"}},
		SummaryHistory:              []characters.GeneratedHistoryVersion{{EpisodeIndex: "1", Text: "旅の案内役。"}},
	}}
	batches := []extractionBatch{{
		BatchIndex:     1,
		BatchCount:     1,
		EpisodeIndexes: []string{"2"},
		Chunks:         []extractionChunk{{EpisodeIndex: "2", Title: "二話", Text: "アリスはクレアを案内した。"}},
	}}
	generated, _, usageRequests, err := server.generateOpenRouterExtraction(context.Background(), config, "novel-1", "2", seed, batches, nil)
	if err != nil {
		t.Fatalf("generateOpenRouterExtraction returned error: %v", err)
	}
	if chatRequests != 1 || len(usageRequests) != 1 || usageRequests[0].InputTokens != 20 || usageRequests[0].OutputTokens != 6 {
		t.Fatalf("delta generation should make one tracked request: calls=%d usage=%+v", chatRequests, usageRequests)
	}
	if len(generated) != 2 {
		t.Fatalf("delta generation should merge seed and new characters: %+v", generated)
	}
	if generated[0].CharacterID != "char_seed" || generated[0].CanonicalName != "アリス" || len(generated[0].SummaryHistory) != 2 {
		t.Fatalf("character update should merge into the stable seed id: %+v", generated[0])
	}
	if generated[1].CanonicalName != "クレア" || generated[1].CharacterID == "" || generated[1].CharacterID == "char_seed" {
		t.Fatalf("new character should receive a separate stable id: %+v", generated[1])
	}
}

func TestExtractionGenerationSeedFiltersAlreadyProcessedEpisodes(t *testing.T) {
	dataDir := t.TempDir()
	server := &Server{dataDir: dataDir}
	if err := characters.SaveGeneratedSummary(server.stateDir(), "novel-1", "2", []characters.GeneratedCharacter{{
		CharacterID:                 "char_existing",
		CanonicalName:               "アリス",
		CanonicalEpisodeIndex:       "1",
		FirstAppearanceEpisodeIndex: "1",
		SummaryHistory:              []characters.GeneratedHistoryVersion{{EpisodeIndex: "2", Text: "既存生成済み。"}},
	}}); err != nil {
		t.Fatalf("save existing generated summary: %v", err)
	}
	if err := terms.SaveGeneratedTerms(server.stateDir(), "novel-1", "2", []terms.GeneratedTerm{}, nil); err != nil {
		t.Fatalf("save existing generated terms: %v", err)
	}
	seed, processed, ok, err := server.loadExtractionGenerationSeed("novel-1", "4")
	if err != nil || !ok || processed == nil || *processed != "2" || len(seed) != 1 || seed[0].CharacterID != "char_existing" {
		t.Fatalf("loadExtractionGenerationSeed should return reusable seed: ok=%v processed=%v seed=%+v err=%v", ok, processed, seed, err)
	}
	coveredSeed, processed, ok, err := server.loadExtractionGenerationSeed("novel-1", "1")
	if err != nil || !ok || processed == nil || *processed != "2" || len(coveredSeed) != 1 || !extractionProcessedCovers(*processed, "1") {
		t.Fatalf("generation seed beyond request should cover earlier requests: ok=%v processed=%v seed=%+v err=%v", ok, processed, coveredSeed, err)
	}

	inputs := extractionInputs{
		Episodes: []characters.HeuristicEpisode{
			{EpisodeIndex: "1", Text: "一話"},
			{EpisodeIndex: "2", Text: "二話"},
			{EpisodeIndex: "3", Text: "三話"},
		},
		Batches: []extractionBatch{{
			BatchIndex:     1,
			BatchCount:     1,
			EpisodeIndexes: []string{"1", "2", "3"},
			Chunks: []extractionChunk{
				{EpisodeIndex: "1", Title: "一話", Text: "一話"},
				{EpisodeIndex: "2", Title: "二話", Text: "二話"},
				{EpisodeIndex: "3", Title: "三話", Text: "三話"},
			},
		}},
	}
	filtered := filterExtractionInputsAfter(inputs, "2")
	if len(filtered.Episodes) != 1 || filtered.Episodes[0].EpisodeIndex != "3" || len(filtered.Batches) != 1 || len(filtered.Batches[0].Chunks) != 1 || filtered.Batches[0].Chunks[0].EpisodeIndex != "3" {
		t.Fatalf("filter should keep only episodes after processed index: %+v", filtered)
	}
}

func TestExtractionPreviewReusesCoveredGeneratedSummaryWithoutOpenRouter(t *testing.T) {
	t.Setenv("AI_GENERATION_SETTINGS_MASTER_PASSPHRASE", "test-passphrase")
	providerCalls := 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		providerCalls++
		t.Fatalf("OpenRouter should not be called for an already covered character summary: %s", r.URL.Path)
	}))
	defer provider.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", provider.URL)

	dataDir := t.TempDir()
	stateStore := store.New(dataDir)
	if err := stateStore.Initialize(); err != nil {
		t.Fatalf("initialize store: %v", err)
	}
	modelID := "openrouter/covered-summary"
	if _, err := stateStore.PutAIGenerationSettings(store.AIGenerationSettingsUpdate{
		PreferredMode: testStringPtr("llm"),
		SharedProviders: &store.AISharedProvidersInput{
			OpenRouter: store.AIProviderCredentialInput{APIKey: testStringPtr("sk-summary-secret"), APIKeySet: true},
		},
		ProfilesSet: true,
		Profiles: []store.AIProfileInput{{
			ID:          "default",
			Label:       "Default",
			Provider:    "openrouter",
			Credentials: store.AIProfileCredentialsInput{Source: "shared"},
			ModelID:     &modelID,
		}},
	}); err != nil {
		t.Fatalf("put AI settings: %v", err)
	}
	server := newTestServerWithLibraryAndStore(dataDir, library.NewService(filepath.Join(dataDir, "novel-fetcher")), stateStore).(*Server)
	if err := characters.SaveGeneratedSummary(server.stateDir(), "novel-1", "2", []characters.GeneratedCharacter{{
		CharacterID:                 "char_existing",
		CanonicalName:               "アリス",
		CanonicalEpisodeIndex:       "1",
		FirstAppearanceEpisodeIndex: "1",
		SummaryHistory:              []characters.GeneratedHistoryVersion{{EpisodeIndex: "1", Text: "既存生成済み。"}},
	}}); err != nil {
		t.Fatalf("save existing generated summary: %v", err)
	}
	if err := terms.SaveGeneratedTerms(server.stateDir(), "novel-1", "2", []terms.GeneratedTerm{}, nil); err != nil {
		t.Fatalf("save existing generated terms: %v", err)
	}
	preloaded := extractionInputs{
		Episodes: []characters.HeuristicEpisode{{EpisodeIndex: "1", Text: "一話"}},
		Batches: []extractionBatch{{
			BatchIndex:     1,
			BatchCount:     1,
			EpisodeIndexes: []string{"1"},
			Chunks:         []extractionChunk{{EpisodeIndex: "1", Title: "一話", Text: "アリスがいた。"}},
		}},
	}
	summary, err := server.generateExtractionPreview(context.Background(), "novel-1", "1", nil, nil, []string{"1"}, &preloaded)
	if err != nil {
		t.Fatalf("covered preview should load existing summary: %v", err)
	}
	if providerCalls != 0 || summary.Status != "ready" || len(summary.Characters) != 1 || summary.Characters[0].CharacterID != "char_existing" {
		t.Fatalf("covered preview should reuse stored summary without OpenRouter: calls=%d summary=%+v", providerCalls, summary)
	}
}

func TestOpenRouterExtractionCheckpointResume(t *testing.T) {
	failEpisodeTwo := true
	requestedEpisodes := []string{}
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode OpenRouter request: %v", err)
		}
		messages := body["messages"].([]any)
		userContent := messages[1].(map[string]any)["content"].(string)
		var prompt map[string]any
		if err := json.Unmarshal([]byte(userContent), &prompt); err != nil {
			t.Fatalf("decode prompt JSON: %v", err)
		}
		promptEpisodes := prompt["episodes"].([]any)
		episodeTitle := promptEpisodes[0].(map[string]any)["title"].(string)
		if episodeTitle == "一話" {
			requestedEpisodes = append(requestedEpisodes, "1")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []any{map[string]any{"message": map[string]any{"content": testExtractionResponseContent("アリス", "一話の人物")}}},
				"usage":   map[string]any{"prompt_tokens": 10, "completion_tokens": 3, "total_tokens": 13},
			})
			return
		}
		requestedEpisodes = append(requestedEpisodes, "2")
		if failEpisodeTwo {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":{"message":"temporary failure"}}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{"content": testExtractionResponseContent("ボブ", "二話の人物")}}},
			"usage":   map[string]any{"prompt_tokens": 12, "completion_tokens": 4, "total_tokens": 16},
		})
	}))
	defer provider.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", provider.URL)
	t.Setenv("EXTRACTION_MAX_BATCH_CHARS", "1")

	server := &Server{dataDir: t.TempDir()}
	config := &store.ResolvedAIGenerationConfig{APIKey: "sk-summary-secret", ModelID: "openrouter/auto"}
	episodes := []extractionEpisodeInput{
		{EpisodeIndex: "1", Title: "一話", ReaderDocument: library.ReaderDocument{Blocks: []library.ReaderBlock{{Type: "paragraph", Inlines: []library.ReaderInline{{Type: "text", Text: "アリスは出発した。"}}}}}},
		{EpisodeIndex: "2", Title: "二話", ReaderDocument: library.ReaderDocument{Blocks: []library.ReaderBlock{{Type: "paragraph", Inlines: []library.ReaderInline{{Type: "text", Text: "ボブが合流した。"}}}}}},
	}
	maxChunkChars, maxBatchChars := extractionLimits()
	batches := createExtractionBatches(createExtractionChunks(episodes, maxChunkChars), maxBatchChars)
	progressEvents := []extractionBatchProgress{}
	progressSink := func(progress extractionBatchProgress) {
		progressEvents = append(progressEvents, progress)
	}
	if _, _, usageRequests, err := server.generateOpenRouterExtractionWithCheckpoint(context.Background(), config, "novel-1", "2", nil, batches, progressSink); err == nil {
		t.Fatal("first checkpointed generation should fail on the second episode")
	} else if len(usageRequests) != 1 {
		t.Fatalf("failed checkpointed generation should preserve completed batch usage: %+v", usageRequests)
	}
	if len(progressEvents) != 3 || progressEvents[0].Phase != "start" || progressEvents[1].Phase != "complete" || progressEvents[2].Phase != "start" {
		t.Fatalf("checkpointed generation should emit per-batch start/complete progress: %+v", progressEvents)
	}
	checkpoint, err := server.extractionRuntime().LoadCheckpoint("novel-1", "2")
	if err != nil {
		t.Fatalf("load checkpoint: %v", err)
	}
	if len(checkpoint.ProcessedEpisodeIndexes) != 1 || checkpoint.ProcessedEpisodeIndexes[0] != "1" || len(checkpoint.Characters) != 1 {
		t.Fatalf("checkpoint should preserve completed first episode: %+v", checkpoint)
	}

	failEpisodeTwo = false
	requestedEpisodes = []string{}
	progressEvents = []extractionBatchProgress{}
	generated, _, usageRequests, err := server.generateOpenRouterExtractionWithCheckpoint(context.Background(), config, "novel-1", "2", nil, batches, progressSink)
	if err != nil {
		t.Fatalf("resumed checkpointed generation returned error: %v", err)
	}
	if len(generated) != 2 || generated[0].CanonicalName != "アリス" || generated[1].CanonicalName != "ボブ" {
		t.Fatalf("resumed generation should merge checkpointed and new characters: %+v", generated)
	}
	if len(requestedEpisodes) != 1 || requestedEpisodes[0] != "2" {
		t.Fatalf("resumed generation should only request remaining episodes: %+v", requestedEpisodes)
	}
	if len(usageRequests) != 1 || usageRequests[0].OutputTokens == 0 || usageRequests[0].TotalTokens == 0 {
		t.Fatalf("resumed generation should retain provider token usage for requested batches: %+v", usageRequests)
	}
	if !server.extractionCheckpointExists("novel-1", "2") {
		t.Fatal("checkpoint should remain until generated profiles are durably saved")
	}
	info, err := os.Stat(server.extractionCheckpointPath("novel-1", "2"))
	if err != nil {
		t.Fatalf("stat checkpoint: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("checkpoint should be owner-only, mode=%#o", info.Mode().Perm())
	}
}

func TestOpenRouterExtractionCheckpointSnapshotIsAuthoritative(t *testing.T) {
	server := &Server{dataDir: t.TempDir()}
	config := &store.ResolvedAIGenerationConfig{APIKey: "sk-summary-secret", ModelID: "openrouter/auto"}
	seed := []characters.GeneratedCharacter{
		{CharacterID: "char_a", CanonicalName: "アリス", CanonicalEpisodeIndex: "1", FirstAppearanceEpisodeIndex: "1"},
		{CharacterID: "char_b", CanonicalName: "ボブ", CanonicalEpisodeIndex: "1", FirstAppearanceEpisodeIndex: "1"},
	}
	batches := []extractionBatch{{
		BatchIndex:     1,
		BatchCount:     1,
		EpisodeIndexes: []string{"1"},
		Chunks:         []extractionChunk{{EpisodeIndex: "1", Title: "一話", Text: "アリスだけが残る。"}},
	}}
	if err := server.saveExtractionCheckpoint("novel-1", "1", extractionCheckpoint{
		SchemaVersion:             appextraction.CheckpointSchemaVersion,
		NovelID:                   "novel-1",
		UpToEpisodeIndex:          "1",
		GenerationFingerprint:     extractionGenerationFingerprint(config, "novel-1", seed, nil, batches, nil),
		ProcessedEpisodeIndexes:   []string{"1"},
		ProcessedBatchIndexes:     []int{1},
		Characters:                []characters.GeneratedCharacter{{CharacterID: "char_a", CanonicalName: "アリス", CanonicalEpisodeIndex: "1", FirstAppearanceEpisodeIndex: "1"}},
		PendingUnresolvedMentions: []characters.GeneratedUnresolvedMention{{Mention: "黒衣の男", EpisodeIndex: "1"}},
	}); err != nil {
		t.Fatalf("save checkpoint: %v", err)
	}
	generated, generationState, usageRequests, err := server.generateOpenRouterExtractionWithCheckpoint(context.Background(), config, "novel-1", "1", seed, batches, nil)
	if err != nil {
		t.Fatalf("checkpoint generation returned error: %v", err)
	}
	if len(usageRequests) != 0 || len(generated) != 1 || generated[0].CharacterID != "char_a" {
		t.Fatalf("checkpoint snapshot should be authoritative and skip processed batch: generated=%+v usage=%+v", generated, usageRequests)
	}
	if len(generationState.UnresolvedMentions) != 1 || generationState.UnresolvedMentions[0].Mention != "黒衣の男" {
		t.Fatalf("checkpoint should preserve pending unresolved mentions: %+v", generationState.UnresolvedMentions)
	}
}

func TestOpenRouterExtractionCheckpointRejectsGenerationInputMismatch(t *testing.T) {
	requests := 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{"content": testExtractionResponseContent("新モデル", "新しい設定で生成")}}},
			"usage":   map[string]any{"prompt_tokens": 9, "completion_tokens": 5, "total_tokens": 14},
		})
	}))
	defer provider.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", provider.URL)

	server := &Server{dataDir: t.TempDir()}
	oldConfig := &store.ResolvedAIGenerationConfig{APIKey: "sk-summary-secret", ModelID: "openrouter/old", SystemPrompt: testStringPtr("old prompt")}
	newConfig := &store.ResolvedAIGenerationConfig{APIKey: "sk-summary-secret", ModelID: "openrouter/new", SystemPrompt: testStringPtr("new prompt")}
	batches := []extractionBatch{{
		BatchIndex:     1,
		BatchCount:     1,
		EpisodeIndexes: []string{"1"},
		Chunks:         []extractionChunk{{EpisodeIndex: "1", Title: "一話", Text: "新モデルが登場した。"}},
	}}
	if err := server.saveExtractionCheckpoint("novel-1", "1", extractionCheckpoint{
		SchemaVersion:           appextraction.CheckpointSchemaVersion,
		NovelID:                 "novel-1",
		UpToEpisodeIndex:        "1",
		GenerationFingerprint:   extractionGenerationFingerprint(oldConfig, "novel-1", nil, nil, batches, nil),
		ProcessedEpisodeIndexes: []string{"1"},
		ProcessedBatchIndexes:   []int{1},
		Characters:              []characters.GeneratedCharacter{{CanonicalName: "旧モデル"}},
	}); err != nil {
		t.Fatalf("save stale checkpoint: %v", err)
	}

	generated, _, usageRequests, err := server.generateOpenRouterExtractionWithCheckpoint(context.Background(), newConfig, "novel-1", "1", nil, batches, nil)
	if !checkpointstore.IsIncompatible(err) {
		t.Fatalf("generation with mismatched checkpoint error = %v, want incompatible", err)
	}
	if requests != 0 || len(generated) != 0 || len(usageRequests) != 0 {
		t.Fatalf("mismatched checkpoint should stop before provider request: requests=%d generated=%+v usage=%+v", requests, generated, usageRequests)
	}
	quarantined, globErr := filepath.Glob(server.extractionCheckpointPath("novel-1", "1") + ".unsupported-*")
	if globErr != nil || len(quarantined) != 1 {
		t.Fatalf("quarantined checkpoints = %v, err=%v", quarantined, globErr)
	}
}

func TestOpenRouterExtractionLibraryCheckpointRejectsBatchInputMismatch(t *testing.T) {
	providerCalls := 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		providerCalls++
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{"content": testExtractionResponseContent("新本文", "現在の本文で生成")}}},
			"usage":   map[string]any{"prompt_tokens": 9, "completion_tokens": 5, "total_tokens": 14},
		})
	}))
	defer provider.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", provider.URL)

	dataDir := newHTTPAPITestData(t)
	libraryService := library.NewService(filepath.Join(dataDir, "novel-fetcher"))
	server := &Server{dataDir: dataDir, library: libraryService}
	novels, err := libraryService.ListNovels(context.Background())
	if err != nil || len(novels.Novels) != 1 {
		t.Fatalf("list fixture novels: novels=%+v err=%v", novels, err)
	}
	novelID := novels.Novels[0].NovelID
	config := &store.ResolvedAIGenerationConfig{APIKey: "sk-summary-secret", ModelID: "openrouter/current"}
	maxChunkChars, maxBatchChars := extractionLimits()
	inputs, err := server.loadExtractionInputs(context.Background(), novelID, "1", maxChunkChars, maxBatchChars)
	if err != nil {
		t.Fatalf("load fixture character summary inputs: %v", err)
	}
	if err := server.saveExtractionCheckpoint(novelID, "1", extractionCheckpoint{
		SchemaVersion:           appextraction.CheckpointSchemaVersion,
		NovelID:                 novelID,
		UpToEpisodeIndex:        "1",
		GenerationFingerprint:   extractionCheckpointFingerprint(config, map[string]int{"maxChunkChars": maxChunkChars, "maxBatchChars": maxBatchChars}),
		ProcessedEpisodeIndexes: []string{"1"},
		ProcessedBatchIndexes:   []int{1},
		Characters:              []characters.GeneratedCharacter{{CanonicalName: "旧本文"}},
	}); err != nil {
		t.Fatalf("save stale library checkpoint: %v", err)
	}

	generated, _, usageRequests, err := server.generateOpenRouterExtractionWithCheckpoint(context.Background(), config, novelID, "1", nil, inputs.Batches, nil)
	if !checkpointstore.IsIncompatible(err) {
		t.Fatalf("library checkpoint generation error = %v, want incompatible", err)
	}
	if providerCalls != 0 || len(generated) != 0 || len(usageRequests) != 0 {
		t.Fatalf("library checkpoint mismatch should stop before provider call: calls=%d generated=%+v usage=%+v", providerCalls, generated, usageRequests)
	}
}

func TestExtractionInternalHelperErrorBranches(t *testing.T) {
	server := &Server{dataDir: t.TempDir()}
	if episodes := server.extractionHeuristicEpisodes(context.Background(), "novel-1", "1"); len(episodes) != 0 {
		t.Fatalf("nil library should return no heuristic episodes: %+v", episodes)
	}
	if err := server.recordExtractionUsage(ai.UsageRun{}); err != nil {
		t.Fatalf("nil store usage recorder should be a no-op: %v", err)
	}

	blockedServer := &Server{dataDir: t.TempDir()}
	blockedPath := blockedServer.extractionCheckpointPath("novel-1", "1")
	if err := os.MkdirAll(filepath.Dir(blockedPath), 0o755); err != nil {
		t.Fatalf("mkdir blocked checkpoint dir: %v", err)
	}
	if err := os.Mkdir(blockedPath, 0o755); err != nil {
		t.Fatalf("mkdir checkpoint path blocker: %v", err)
	}
	if err := blockedServer.saveExtractionCheckpoint("novel-1", "1", extractionCheckpoint{
		SchemaVersion:    appextraction.CheckpointSchemaVersion,
		NovelID:          "novel-1",
		UpToEpisodeIndex: "1",
	}); err == nil {
		t.Fatal("saveExtractionCheckpoint should fail when target path is a directory")
	}

	dataDir := newHTTPAPITestData(t)
	libraryService := library.NewService(filepath.Join(dataDir, "novel-fetcher"))
	withLibrary := &Server{dataDir: dataDir, library: libraryService}
	novels, err := libraryService.ListNovels(context.Background())
	if err != nil || len(novels.Novels) != 1 {
		t.Fatalf("list fixture novels: novels=%+v err=%v", novels, err)
	}
	episodes := withLibrary.extractionHeuristicEpisodes(context.Background(), novels.Novels[0].NovelID, "1")
	if len(episodes) != 1 || episodes[0].EpisodeIndex != "1" || !strings.Contains(episodes[0].Text, "本文") {
		t.Fatalf("library-backed heuristic episodes should include body text: %+v", episodes)
	}
	if title := withLibrary.novelTitle(context.Background(), novels.Novels[0].NovelID); title == nil || *title != "Fixture Novel" {
		t.Fatalf("novelTitle should resolve fixture title: %v", title)
	}
	if title := withLibrary.novelTitle(context.Background(), "missing"); title != nil {
		t.Fatalf("novelTitle should return nil for missing novels: %v", title)
	}
	generationInputs, err := withLibrary.loadExtractionInputs(context.Background(), novels.Novels[0].NovelID, "1", 10, 10)
	if err != nil || len(generationInputs.Episodes) != 1 || generationInputs.Episodes[0].EpisodeIndex != "1" || len(generationInputs.Batches) == 0 {
		t.Fatalf("strict summary inputs should load fixture body: inputs=%+v err=%v", generationInputs, err)
	}
	if _, err := withLibrary.loadExtractionInputs(context.Background(), "missing", "1", 10, 10); err == nil {
		t.Fatal("strict summary inputs should reject missing novels")
	}
	if inputs, err := (&Server{}).loadExtractionInputs(context.Background(), "novel-1", "1", 10, 10); err != nil || len(inputs.Episodes) != 0 || len(inputs.Batches) != 0 {
		t.Fatalf("nil-library strict summary inputs should be empty without error: inputs=%+v err=%v", inputs, err)
	}

	if value, ok := nullableBodyString(nil); !ok || value != nil {
		t.Fatalf("nullableBodyString should accept nil: value=%v ok=%v", value, ok)
	}
	if value, ok := nullableBodyString("  model "); !ok || value == nil || *value != "model" {
		t.Fatalf("nullableBodyString should trim strings: value=%v ok=%v", value, ok)
	}
	if value, ok := nullableBodyString("  "); !ok || value != nil {
		t.Fatalf("nullableBodyString should normalize blank strings to nil: value=%v ok=%v", value, ok)
	}
	if _, ok := nullableBodyString(123); ok {
		t.Fatal("nullableBodyString should reject non-string values")
	}
}

func TestServerConstructorsAndHelpers(t *testing.T) {
	dataDir := newHTTPAPITestData(t)
	t.Setenv("VIEWER_API_DATA_DIR", dataDir)
	handler, err := newTestServerWithDataDir(dataDir)
	if err != nil {
		t.Fatalf("newTestServerWithDataDir returned error: %v", err)
	}
	requestJSON(t, handler, http.MethodGet, "/api/health", nil, http.StatusOK)

	blockedDataDir := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(blockedDataDir, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("write blocked data dir: %v", err)
	}
	degradedHandler, err := newTestServerWithDataDir(blockedDataDir)
	if err == nil {
		t.Fatal("newTestServerWithDataDir should report initialization errors")
	}
	health := requestJSON(t, degradedHandler, http.MethodGet, "/api/health", nil, http.StatusOK)
	if health["status"] != "warn" {
		t.Fatalf("degraded health should report warn: %+v", health)
	}
	degradedStatus := requestJSON(t, degradedHandler, http.MethodGet, "/api/system/status", nil, http.StatusOK)
	if service := findRuntimeService(degradedStatus, "state"); service == nil || service["status"] != "warn" {
		t.Fatalf("degraded system status should report state readiness: %+v", degradedStatus)
	}

	stateStore := store.New(dataDir)
	if err := stateStore.Initialize(); err != nil {
		t.Fatalf("initialize store: %v", err)
	}
	requestJSON(t, newTestServerWithStore(stateStore), http.MethodGet, "/api/health", nil, http.StatusOK)

	if !matchesContentEtag([]string{`"abc"`}, "abc") || !matchesContentEtag([]string{"W/\"abc\""}, "abc") || !matchesContentEtag([]string{"*"}, "abc") {
		t.Fatal("expected content etag matcher to accept quoted, weak, and wildcard values")
	}
	if matchesContentEtag([]string{`"other"`}, "abc") {
		t.Fatal("unexpected etag match")
	}
	if _, ok := isPositiveIntegerString(float64(2)); !ok {
		t.Fatal("expected positive float integer to be accepted")
	}
	if _, ok := isNonNegativeIntegerString(float64(0)); !ok {
		t.Fatal("expected zero float integer to be accepted")
	}
	if isDigits("12a") {
		t.Fatal("non-digit string should be rejected")
	}
	if _, ok := isPositiveIntegerString("0"); ok {
		t.Fatal("zero positive integer string should be rejected")
	}
	if _, ok := isPositiveIntegerString(float64(1.5)); ok {
		t.Fatal("fractional positive integer should be rejected")
	}
	if _, ok := isNonNegativeIntegerString("10"); !ok {
		t.Fatal("non-negative integer string should be accepted")
	}
	if _, ok := isNonNegativeIntegerString(float64(-1)); ok {
		t.Fatal("negative integer should be rejected")
	}
	if trimPathValue("%zz") != "%zz" {
		t.Fatal("invalid path escape should return original value")
	}

	response := httptest.NewRecorder()
	writeResult(response, nil, os.ErrInvalid)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("writeResult error status=%d", response.Code)
	}
}

func TestServerAIUsageStoreErrors(t *testing.T) {
	dataDir := newHTTPAPITestData(t)
	if err := os.WriteFile(filepath.Join(dataDir, "state", "ai_usage.sqlite"), []byte("not sqlite"), 0o644); err != nil {
		t.Fatalf("overwrite ai usage sqlite: %v", err)
	}
	stateStore := store.New(dataDir)
	if err := stateStore.Initialize(); err != nil {
		t.Fatalf("initialize store: %v", err)
	}
	handler := newTestServerWithLibraryAndStore(dataDir, library.NewService(filepath.Join(dataDir, "novel-fetcher")), stateStore)
	requestJSON(t, handler, http.MethodGet, "/api/ai-generation/usage", nil, http.StatusInternalServerError)
	requestJSON(t, handler, http.MethodGet, "/api/ai-generation/usage/run-http", nil, http.StatusInternalServerError)
}

func TestServerAIGenerationSettingsStoreErrors(t *testing.T) {
	dataDir := newHTTPAPITestData(t)
	if err := os.WriteFile(filepath.Join(dataDir, "state", "ai_generation_settings.yaml"), []byte("profiles: ["), 0o644); err != nil {
		t.Fatalf("overwrite AI settings: %v", err)
	}
	stateStore := store.New(dataDir)
	handler := newTestServerWithLibraryAndStore(dataDir, library.NewService(filepath.Join(dataDir, "novel-fetcher")), stateStore)
	requestJSON(t, handler, http.MethodGet, "/api/ai-generation/settings", nil, http.StatusConflict)
	requestJSON(t, handler, http.MethodPut, "/api/ai-generation/settings/preferred-mode", map[string]any{
		"preferredMode": "heuristic",
	}, http.StatusConflict)

	dataDir = newHTTPAPITestData(t)
	stateStore = store.New(dataDir)
	if err := stateStore.Initialize(); err != nil {
		t.Fatalf("initialize store: %v", err)
	}
	handler = newTestServerWithLibraryAndStore(dataDir, library.NewService(filepath.Join(dataDir, "novel-fetcher")), stateStore)
	requestJSON(t, handler, http.MethodPut, "/api/ai-generation/settings", map[string]any{
		"sharedProviders": map[string]any{
			"openrouter": map[string]any{"apiKey": "sk-test-secret-value"},
		},
	}, http.StatusServiceUnavailable)
}

func TestServerAIGenerationRuntimeStatusServiceBranches(t *testing.T) {
	t.Setenv("NODE_ENV", "test")
	assertAIServiceIdentity := func(t *testing.T, service library.RuntimeStatusService) {
		t.Helper()
		if service.ID != "go-internal-ai" || service.Label != "Go internal AI" {
			t.Fatalf("AI runtime status identity = %+v, want go-internal-ai / Go internal AI", service)
		}
	}
	newServer := func(t *testing.T) *Server {
		t.Helper()
		dataDir := newHTTPAPITestData(t)
		stateStore := store.New(dataDir)
		if err := stateStore.Initialize(); err != nil {
			t.Fatalf("initialize store: %v", err)
		}
		return newTestServerWithLibraryAndStore(dataDir, library.NewService(filepath.Join(dataDir, "novel-fetcher")), stateStore).(*Server)
	}

	heuristicServer := newServer(t)
	if service := heuristicServer.aiGenerationRuntimeStatusService(); service.Status != library.RuntimeStatusOK || service.Summary != "未使用" {
		t.Fatalf("heuristic mode should not use Go internal AI LLM path: %+v", service)
	} else {
		assertAIServiceIdentity(t, service)
	}

	missingPassphraseServer := newServer(t)
	if _, err := missingPassphraseServer.stateStore.PutAIGenerationPreferredMode("llm"); err != nil {
		t.Fatalf("put preferred mode: %v", err)
	}
	if service := missingPassphraseServer.aiGenerationRuntimeStatusService(); service.Status != library.RuntimeStatusError || service.Summary != "要設定" {
		t.Fatalf("llm mode without AI settings master passphrase should require configuration: %+v", service)
	} else {
		assertAIServiceIdentity(t, service)
	}

	t.Setenv("AI_GENERATION_SETTINGS_MASTER_PASSPHRASE", "test-passphrase")
	disabledServer := newServer(t)
	if _, err := disabledServer.stateStore.PutAIGenerationPreferredMode("llm"); err != nil {
		t.Fatalf("put preferred mode: %v", err)
	}
	if service := disabledServer.aiGenerationRuntimeStatusService(); service.Status != library.RuntimeStatusError || service.Summary != "利用不可" {
		t.Fatalf("llm mode without OpenRouter readiness should be unavailable: %+v", service)
	} else {
		assertAIServiceIdentity(t, service)
	}

	warnServer := newServer(t)
	if _, err := warnServer.stateStore.PutAIGenerationPreferredMode("llm"); err != nil {
		t.Fatalf("put preferred mode: %v", err)
	}
	if service := warnServer.aiGenerationRuntimeStatusService(); service.Status != library.RuntimeStatusError || service.Summary != "利用不可" {
		t.Fatalf("llm mode without internal OpenRouter readiness should be unavailable: %+v", service)
	} else {
		assertAIServiceIdentity(t, service)
	}

	okServer := newServer(t)
	secret := "sk-test-secret-value"
	modelID := "openrouter/auto"
	if _, err := okServer.stateStore.PutAIGenerationSettings(store.AIGenerationSettingsUpdate{
		PreferredMode: stringPtr("llm"),
		SharedProviders: &store.AISharedProvidersInput{
			OpenRouter: store.AIProviderCredentialInput{APIKey: &secret, APIKeySet: true},
		},
		Profiles: []store.AIProfileInput{
			{
				ID:                "default",
				Label:             "Default",
				Credentials:       store.AIProfileCredentialsInput{Source: "shared"},
				ModelID:           &modelID,
				RequireParameters: true,
			},
		},
		ProfilesSet: true,
	}); err != nil {
		t.Fatalf("put AI settings: %v", err)
	}
	if service := okServer.aiGenerationRuntimeStatusService(); service.Status != library.RuntimeStatusOK || service.Summary != "利用中" {
		t.Fatalf("llm mode with API key should be ready: %+v", service)
	} else {
		assertAIServiceIdentity(t, service)
	}

	errorServer := newServer(t)
	if err := os.WriteFile(filepath.Join(errorServer.dataDir, "state", "ai_generation_settings.yaml"), []byte("profiles: ["), 0o644); err != nil {
		t.Fatalf("overwrite AI settings: %v", err)
	}
	if service := errorServer.aiGenerationRuntimeStatusService(); service.Status != library.RuntimeStatusError || service.Summary != "設定エラー" {
		t.Fatalf("invalid AI settings should be reported: %+v", service)
	} else {
		assertAIServiceIdentity(t, service)
	}

	noProfile := resolveActiveAIProfile(ai.SettingsResponse{})
	if noProfile != nil || profileID(noProfile) != nil || profileLabel(noProfile) != nil || profileModelID(noProfile) != nil {
		t.Fatalf("empty settings should not resolve an active profile: %v", noProfile)
	}
}

func TestServerGoogleBooksRuntimeStatusService(t *testing.T) {
	t.Setenv("AI_GENERATION_SETTINGS_MASTER_PASSPHRASE", "test-passphrase")
	t.Setenv("PUBLICATION_PROVIDER_GOOGLE_BOOKS_ENABLED", "")
	t.Setenv("GOOGLE_BOOKS_API_KEY", "")
	dataDir := newHTTPAPITestData(t)
	stateStore := store.New(dataDir)
	if err := stateStore.Initialize(); err != nil {
		t.Fatalf("initialize store: %v", err)
	}
	server := newTestServerWithLibraryAndStore(dataDir, library.NewService(filepath.Join(dataDir, "novel-fetcher")), stateStore).(*Server)
	if service := server.googleBooksRuntimeStatusService(); service.Status != library.RuntimeStatusWarn || service.Summary != "API key未設定" {
		t.Fatalf("missing Google Books API key should warn: %+v", service)
	}

	googleBooksKey := "test-google-books-yaml-key"
	if _, err := stateStore.PutAIGenerationSettings(store.AIGenerationSettingsUpdate{
		SharedProviders: &store.AISharedProvidersInput{
			GoogleBooks: store.AIProviderCredentialInput{APIKey: &googleBooksKey, APIKeySet: true},
		},
	}); err != nil {
		t.Fatalf("store Google Books key: %v", err)
	}
	if service := server.googleBooksRuntimeStatusService(); service.Status != library.RuntimeStatusOK || service.Summary != "設定済み" {
		t.Fatalf("YAML configured Google Books API key should be ok: %+v", service)
	}

	if _, err := stateStore.PutAIGenerationSettings(store.AIGenerationSettingsUpdate{
		SharedProviders: &store.AISharedProvidersInput{
			GoogleBooks: store.AIProviderCredentialInput{APIKey: nil, APIKeySet: true},
		},
	}); err != nil {
		t.Fatalf("clear Google Books key: %v", err)
	}
	t.Setenv("GOOGLE_BOOKS_API_KEY", "test-google-books-key")
	if service := server.googleBooksRuntimeStatusService(); service.Status != library.RuntimeStatusOK || service.Summary != "設定済み" {
		t.Fatalf("configured Google Books API key should be ok: %+v", service)
	}

	t.Setenv("PUBLICATION_PROVIDER_GOOGLE_BOOKS_ENABLED", "0")
	if service := server.googleBooksRuntimeStatusService(); service.Status != library.RuntimeStatusOK || service.Summary != "無効" {
		t.Fatalf("disabled Google Books provider should be ok: %+v", service)
	}
}

func TestServerAIGenerationQuarantinesCorruptDerivedProfile(t *testing.T) {
	dataDir := newHTTPAPITestData(t)
	novels := requestJSON(t, newTestServerWithLibraryAndStore(dataDir, library.NewService(filepath.Join(dataDir, "novel-fetcher")), store.New(dataDir)), http.MethodGet, "/api/library/novels", nil, http.StatusOK)
	novelID := novels["novels"].([]any)[0].(map[string]any)["novelId"].(string)
	if err := os.WriteFile(filepath.Join(dataDir, "state", "character_profiles", novelID+".yaml"), []byte("characters: ["), 0o644); err != nil {
		t.Fatalf("overwrite character profile: %v", err)
	}
	stateStore := store.New(dataDir)
	if err := stateStore.Initialize(); err != nil {
		t.Fatalf("initialize store: %v", err)
	}
	handler := newTestServerWithLibraryAndStore(dataDir, library.NewService(filepath.Join(dataDir, "novel-fetcher")), stateStore)
	requestJSON(t, handler, http.MethodPost, "/api/ai-generation/playground/extraction", map[string]any{
		"novelId":          novelID,
		"upToEpisodeIndex": "1",
	}, http.StatusOK)
	stream := requestRaw(t, handler, http.MethodPost, "/api/ai-generation/playground/extraction/stream", map[string]any{
		"novelId":          novelID,
		"upToEpisodeIndex": "1",
	}, http.StatusOK)
	if !strings.Contains(stream, `"type":"result"`) {
		t.Fatalf("expected recovered stream result, got %s", stream)
	}
	quarantined, err := filepath.Glob(filepath.Join(dataDir, "state", "character_profiles", novelID+".yaml.unsupported-*"))
	if err != nil || len(quarantined) != 1 {
		t.Fatalf("quarantined profiles = %v, err=%v", quarantined, err)
	}
}

func TestServerFetcherStatusFallbacksWhenFetcherUnavailable(t *testing.T) {
	t.Setenv("NOVEL_FETCHER_API_BASE_URL", "http://127.0.0.1:1")
	dataDir := newHTTPAPITestData(t)
	stateStore := store.New(dataDir)
	if err := stateStore.Initialize(); err != nil {
		t.Fatalf("initialize store: %v", err)
	}
	handler := newTestServerWithLibraryAndStore(dataDir, library.NewService(filepath.Join(dataDir, "novel-fetcher")), stateStore)

	assertFetcherUnavailableFallback(t, handler)
}

func TestServerFetcherErrors(t *testing.T) {
	t.Setenv("NOVEL_FETCHER_API_BASE_URL", "http://127.0.0.1:1")
	dataDir := newHTTPAPITestData(t)
	stateStore := store.New(dataDir)
	if err := stateStore.Initialize(); err != nil {
		t.Fatalf("initialize store: %v", err)
	}
	handler := newTestServerWithLibraryAndStore(dataDir, library.NewService(filepath.Join(dataDir, "novel-fetcher")), stateStore)
	novels := requestJSON(t, handler, http.MethodGet, "/api/library/novels", nil, http.StatusOK)
	novelID := novels["novels"].([]any)[0].(map[string]any)["novelId"].(string)

	requestJSON(t, handler, http.MethodPost, "/api/fetcher/works/download", map[string]any{"targets": []string{"https://example.test/novel"}}, http.StatusBadGateway)
	requestJSON(t, handler, http.MethodPost, "/api/fetcher/works/update", map[string]any{"novelIds": []string{novelID}}, http.StatusBadGateway)
	requestJSON(t, handler, http.MethodPost, "/api/fetcher/tasks/task-1/cancel", nil, http.StatusBadGateway)
}

func assertFetcherUnavailableFallback(t *testing.T, handler http.Handler) {
	t.Helper()

	status := requestJSON(t, handler, http.MethodGet, "/api/fetcher/status", nil, http.StatusOK)
	if status["queue"] == nil || status["tasks"] == nil {
		t.Fatalf("fallback status should keep frontend shape: %+v", status)
	}
	if status["version"] == nil || status["checkedAt"] == "" {
		t.Fatalf("fallback status should include version and checkedAt: %+v", status)
	}
	if status["available"] != false || status["degraded"] != true {
		t.Fatalf("fallback status should distinguish unavailable fetcher from empty queue: %+v", status)
	}
	statusTasks := status["tasks"].(map[string]any)
	if statusTasks["current"] != nil || statusTasks["convertCurrent"] != nil {
		t.Fatalf("fallback status tasks should not expose active tasks: %+v", statusTasks)
	}
	for _, key := range []string{"recentCompleted", "recentFailed", "completedCount", "failedCount", "convertCurrent", "convertQueued"} {
		if _, ok := statusTasks[key]; !ok {
			t.Fatalf("fallback status tasks should expose camelCase key %q: %+v", key, statusTasks)
		}
	}
	if statusTasks["completedCount"] != float64(0) || statusTasks["failedCount"] != float64(0) {
		t.Fatalf("fallback status tasks should expose zero counts: %+v", statusTasks)
	}
	if len(statusTasks["queued"].([]any)) != 0 || len(statusTasks["recentCompleted"].([]any)) != 0 || len(statusTasks["recentFailed"].([]any)) != 0 || len(statusTasks["convertQueued"].([]any)) != 0 {
		t.Fatalf("fallback status tasks should expose empty task arrays: %+v", statusTasks)
	}
	for _, key := range []string{"recent_completed", "recent_failed", "completed_count", "failed_count", "convert_current", "convert_queued"} {
		if _, ok := statusTasks[key]; ok {
			t.Fatalf("fallback status tasks should not expose snake_case key %q: %+v", key, statusTasks)
		}
	}
	queue := requestJSON(t, handler, http.MethodGet, "/api/fetcher/queue", nil, http.StatusOK)
	if queue["running"] != false || queue["total"] != float64(0) || queue["webWorker"] != float64(0) || queue["worker"] != float64(0) {
		t.Fatalf("fallback queue should be idle: %+v", queue)
	}
	if queue["available"] != false || queue["degraded"] != true {
		t.Fatalf("fallback queue should distinguish unavailable fetcher from empty queue: %+v", queue)
	}
	summary := requestJSON(t, handler, http.MethodGet, "/api/fetcher/tasks/summary", nil, http.StatusOK)
	if summary["current"] != nil || summary["convertCurrent"] != nil {
		t.Fatalf("fallback task summary should not expose active tasks: %+v", summary)
	}
	for _, key := range []string{"recentCompleted", "recentFailed", "completedCount", "failedCount", "convertCurrent", "convertQueued"} {
		if _, ok := summary[key]; !ok {
			t.Fatalf("fallback task summary should expose camelCase key %q: %+v", key, summary)
		}
	}
	if summary["completedCount"] != float64(0) || summary["failedCount"] != float64(0) {
		t.Fatalf("fallback task summary should expose zero counts: %+v", summary)
	}
	if summary["available"] != false || summary["degraded"] != true {
		t.Fatalf("fallback task summary should distinguish unavailable fetcher from empty tasks: %+v", summary)
	}
	if len(summary["queued"].([]any)) != 0 || len(summary["recentCompleted"].([]any)) != 0 || len(summary["recentFailed"].([]any)) != 0 || len(summary["convertQueued"].([]any)) != 0 {
		t.Fatalf("fallback task summary should expose empty task arrays: %+v", summary)
	}
	for _, key := range []string{"recent_completed", "recent_failed", "completed_count", "failed_count", "convert_current", "convert_queued"} {
		if _, ok := summary[key]; ok {
			t.Fatalf("fallback task summary should not expose snake_case key %q: %+v", key, summary)
		}
	}
}

func TestServerAIGenerationSettingsCryptoErrorsAreServiceUnavailable(t *testing.T) {
	dataDir := newHTTPAPITestData(t)
	stateStore := store.New(dataDir)
	if err := stateStore.Initialize(); err != nil {
		t.Fatalf("initialize store: %v", err)
	}
	handler := newTestServerWithLibraryAndStore(dataDir, library.NewService(filepath.Join(dataDir, "novel-fetcher")), stateStore)
	response := requestJSON(t, handler, http.MethodPut, "/api/ai-generation/settings", map[string]any{
		"sharedProviders": map[string]any{
			"openrouter": map[string]any{"apiKey": "sk-test-secret-value"},
		},
	}, http.StatusServiceUnavailable)
	if !strings.Contains(response["error"].(string), "AI_GENERATION_SETTINGS_MASTER_PASSPHRASE") {
		t.Fatalf("crypto error should mention the missing passphrase: %+v", response)
	}
}

func TestReaderAssistantSettingsCryptoErrorReturnsServiceUnavailable(t *testing.T) {
	t.Setenv("AI_GENERATION_SETTINGS_MASTER_PASSPHRASE", "test-passphrase")
	dataDir := newHTTPAPITestData(t)
	stateStore := store.New(dataDir)
	if err := stateStore.Initialize(); err != nil {
		t.Fatalf("initialize store: %v", err)
	}
	modelID := "openrouter/auto"
	if _, err := stateStore.PutAIGenerationSettings(store.AIGenerationSettingsUpdate{
		PreferredMode: testStringPtr("llm"),
		SharedProviders: &store.AISharedProvidersInput{
			OpenRouter: store.AIProviderCredentialInput{APIKey: testStringPtr("sk-reader-secret"), APIKeySet: true},
		},
		ProfilesSet: true,
		Profiles: []store.AIProfileInput{{
			ID:          "default",
			Label:       "Default",
			Provider:    "openrouter",
			Credentials: store.AIProfileCredentialsInput{Source: "shared"},
			ModelID:     &modelID,
		}},
	}); err != nil {
		t.Fatalf("put AI settings: %v", err)
	}
	t.Setenv("AI_GENERATION_SETTINGS_MASTER_PASSPHRASE", "wrong-passphrase")
	handler := newTestServerWithLibraryAndStore(dataDir, library.NewService(filepath.Join(dataDir, "novel-fetcher")), stateStore)
	novels := requestJSON(t, handler, http.MethodGet, "/api/library/novels", nil, http.StatusOK)
	novelID := novels["novels"].([]any)[0].(map[string]any)["novelId"].(string)

	response := requestJSON(t, handler, http.MethodPost, "/api/library/novels/"+novelID+"/reader-assistant/chat", map[string]any{
		"message":             "hello",
		"currentEpisodeIndex": "1",
	}, http.StatusServiceUnavailable)
	if response["error"] != "AI generation settings could not be decrypted." {
		t.Fatalf("reader assistant crypto error should be generic: %+v", response)
	}
}

func TestServerAIGenerationSettingsPersistenceAndMasking(t *testing.T) {
	t.Setenv("AI_GENERATION_SETTINGS_MASTER_PASSPHRASE", "test-passphrase")
	dataDir := newHTTPAPITestData(t)
	stateStore := store.New(dataDir)
	if err := stateStore.Initialize(); err != nil {
		t.Fatalf("initialize store: %v", err)
	}
	handler := newTestServerWithLibraryAndStore(dataDir, library.NewService(filepath.Join(dataDir, "novel-fetcher")), stateStore)

	secret := "sk-test-secret-value"
	response := requestJSON(t, handler, http.MethodPut, "/api/ai-generation/settings", map[string]any{
		"preferredMode":     "llm",
		"selectedProfileId": "custom",
		"sharedProviders": map[string]any{
			"openrouter": map[string]any{"apiKey": secret},
		},
		"profiles": []any{
			map[string]any{
				"id":                "default",
				"label":             "Default",
				"credentials":       map[string]any{"source": "shared"},
				"providerOrder":     "OpenAI",
				"requireParameters": true,
			},
			map[string]any{
				"id":                "custom",
				"label":             "Custom",
				"credentials":       map[string]any{"source": "custom", "apiKey": secret},
				"modelId":           "anthropic/claude-sonnet-4",
				"providerOrder":     []any{"Anthropic", ""},
				"allowFallbacks":    true,
				"requireParameters": false,
			},
		},
	}, http.StatusOK)
	raw, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	if strings.Contains(string(raw), secret) || strings.Contains(string(raw), "apiKey\"") {
		t.Fatalf("AI settings response exposed raw credential: %s", raw)
	}
	settings := response["settings"].(map[string]any)
	if settings["selectedProfileId"] != "custom" {
		t.Fatalf("unexpected selected profile: %+v", settings)
	}
	profiles := settings["profiles"].([]any)
	custom := profiles[1].(map[string]any)
	credentials := custom["credentials"].(map[string]any)
	if credentials["hasApiKey"] != true || credentials["apiKeyMasked"] == nil {
		t.Fatalf("custom credential metadata missing: %+v", credentials)
	}
	legacySecret := "dummy-openrouter-legacy-secret-value"
	legacyResponse := requestJSON(t, handler, http.MethodPut, "/api/ai-generation/settings", map[string]any{
		"selectedProfileId": "custom",
		"profiles": []any{
			map[string]any{
				"id":                "default",
				"label":             "Default",
				"credentials":       map[string]any{"source": "shared"},
				"providerOrder":     []any{},
				"requireParameters": true,
			},
			map[string]any{
				"id":                "custom",
				"label":             "Custom",
				"apiKey":            legacySecret,
				"modelId":           "anthropic/claude-sonnet-4",
				"providerOrder":     []any{"Anthropic"},
				"allowFallbacks":    true,
				"requireParameters": false,
			},
		},
	}, http.StatusOK)
	legacyRaw, err := json.Marshal(legacyResponse)
	if err != nil {
		t.Fatalf("marshal legacy response: %v", err)
	}
	if strings.Contains(string(legacyRaw), legacySecret) {
		t.Fatalf("legacy apiKey response exposed raw credential: %s", legacyRaw)
	}
	legacyProfiles := legacyResponse["settings"].(map[string]any)["profiles"].([]any)
	legacyCredentials := legacyProfiles[1].(map[string]any)["credentials"].(map[string]any)
	if legacyCredentials["source"] != "custom" || legacyCredentials["hasApiKey"] != true {
		t.Fatalf("legacy top-level apiKey should be stored as custom credentials: %+v", legacyCredentials)
	}

	requestJSON(t, handler, http.MethodPut, "/api/ai-generation/settings", map[string]any{
		"profiles": []any{
			map[string]any{
				"id":                "default",
				"label":             "Default",
				"credentials":       map[string]any{"source": "shared"},
				"providerOrder":     []any{},
				"requireParameters": true,
			},
		},
	}, http.StatusBadRequest)

	requestJSON(t, handler, http.MethodPut, "/api/ai-generation/settings", map[string]any{
		"profiles": []any{
			map[string]any{
				"id":                "default",
				"label":             "Default",
				"credentials":       map[string]any{"source": "shared"},
				"providerOrder":     []any{},
				"requireParameters": true,
			},
			map[string]any{
				"id":                "custom",
				"label":             "Custom",
				"credentials":       map[string]any{"source": "custom"},
				"modelId":           "anthropic/claude-sonnet-4",
				"providerOrder":     []any{"Anthropic"},
				"allowFallbacks":    true,
				"requireParameters": false,
			},
		},
	}, http.StatusOK)

	requestJSON(t, handler, http.MethodPut, "/api/ai-generation/settings", map[string]any{
		"profiles": []any{
			map[string]any{"label": "Bad Provider Type", "provider": 1},
		},
	}, http.StatusBadRequest)
	requestJSON(t, handler, http.MethodPut, "/api/ai-generation/settings", map[string]any{
		"profiles": []any{
			map[string]any{"label": "Bad Model Type", "modelId": 1},
		},
	}, http.StatusBadRequest)
	requestJSON(t, handler, http.MethodPut, "/api/ai-generation/settings", map[string]any{
		"profiles": []any{
			map[string]any{"label": "Bad Boolean Type", "allowFallbacks": "yes"},
		},
	}, http.StatusBadRequest)

	preserved := requestJSON(t, handler, http.MethodPut, "/api/ai-generation/settings", map[string]any{
		"preferredMode":     "llm",
		"selectedProfileId": "custom",
		"sharedProviders": map[string]any{
			"openrouter": map[string]any{},
		},
		"profiles": []any{
			map[string]any{
				"id":                "default",
				"label":             "Default",
				"credentials":       map[string]any{"source": "shared"},
				"providerOrder":     []any{},
				"requireParameters": true,
			},
			map[string]any{
				"id":                "custom",
				"label":             "Custom Renamed",
				"modelId":           "anthropic/claude-sonnet-4",
				"providerOrder":     "Anthropic, OpenAI",
				"allowFallbacks":    true,
				"requireParameters": true,
			},
		},
	}, http.StatusOK)
	preservedSettings := preserved["settings"].(map[string]any)
	preservedShared := preservedSettings["sharedProviders"].(map[string]any)["openrouter"].(map[string]any)
	preservedProfiles := preservedSettings["profiles"].([]any)
	preservedCustom := preservedProfiles[1].(map[string]any)
	preservedCredentials := preservedCustom["credentials"].(map[string]any)
	if preservedShared["hasApiKey"] != true || preservedCredentials["hasApiKey"] != true || preservedCustom["label"] != "Custom Renamed" {
		t.Fatalf("metadata-only update should preserve credentials: %+v", preserved)
	}
	preservedProviderOrder := preservedCustom["providerOrder"].([]any)
	if len(preservedProviderOrder) != 2 || preservedProviderOrder[0] != "Anthropic" || preservedProviderOrder[1] != "OpenAI" {
		t.Fatalf("comma-separated providerOrder should be split: %+v", preservedCustom)
	}

	noOpSharedProviders := requestJSON(t, handler, http.MethodPut, "/api/ai-generation/settings", map[string]any{
		"sharedProviders": map[string]any{},
	}, http.StatusOK)
	noOpShared := noOpSharedProviders["settings"].(map[string]any)["sharedProviders"].(map[string]any)["openrouter"].(map[string]any)
	if noOpShared["hasApiKey"] != true {
		t.Fatalf("missing openrouter shared provider should be treated as no-op: %+v", noOpSharedProviders)
	}

	preservedRaw, err := json.Marshal(preserved)
	if err != nil {
		t.Fatalf("marshal preserved response: %v", err)
	}
	if strings.Contains(string(preservedRaw), secret) {
		t.Fatalf("preserved settings response exposed raw credential: %s", preservedRaw)
	}

	reloaded := requestJSON(t, handler, http.MethodGet, "/api/ai-generation/settings", nil, http.StatusOK)
	reloadedRaw, err := json.Marshal(reloaded)
	if err != nil {
		t.Fatalf("marshal reloaded response: %v", err)
	}
	if strings.Contains(string(reloadedRaw), secret) {
		t.Fatalf("reloaded settings response exposed raw credential: %s", reloadedRaw)
	}
	if reloaded["preferredMode"] != "llm" {
		t.Fatalf("preferred mode was not persisted: %+v", reloaded)
	}
}

func TestServerAIGenerationSettingsIncludesOpenRouterModelInfo(t *testing.T) {
	modelID := "openrouter/settings-model"
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" || r.URL.Query().Get("q") != modelID {
			t.Fatalf("unexpected model metadata request: path=%s query=%s", r.URL.Path, r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{
			"data": [{
				"id": "openrouter/settings-model",
				"context_length": 64000,
				"top_provider": {
					"context_length": 96000,
					"max_completion_tokens": 24000
				}
			}]
		}`))
	}))
	defer provider.Close()
	t.Setenv("AI_GENERATION_SETTINGS_MASTER_PASSPHRASE", "test-passphrase")
	t.Setenv("OPENROUTER_API_BASE_URL", provider.URL)

	dataDir := newHTTPAPITestData(t)
	stateStore := store.New(dataDir)
	if err := stateStore.Initialize(); err != nil {
		t.Fatalf("initialize store: %v", err)
	}
	handler := newTestServerWithLibraryAndStore(dataDir, library.NewService(filepath.Join(dataDir, "novel-fetcher")), stateStore)

	response := requestJSON(t, handler, http.MethodPut, "/api/ai-generation/settings", map[string]any{
		"preferredMode":     "llm",
		"selectedProfileId": "default",
		"sharedProviders": map[string]any{
			"openrouter": map[string]any{"apiKey": "dummy-openrouter-key-realish-settings"},
		},
		"profiles": []any{
			map[string]any{
				"id":          "default",
				"label":       "Default",
				"credentials": map[string]any{"source": "shared"},
				"modelId":     modelID,
			},
		},
	}, http.StatusOK)
	assertSettingsModelInfo(t, response, 96000, 24000)

	reloaded := requestJSON(t, handler, http.MethodGet, "/api/ai-generation/settings", nil, http.StatusOK)
	assertSettingsModelInfo(t, reloaded, 96000, 24000)
}

func TestAIGenerationSettingsParserHelpers(t *testing.T) {
	if runtime, ok := parseAIExtractionRuntime(map[string]any{"parallelRequestConcurrency": float64(4)}); !ok || runtime.ParallelRequestConcurrency != 4 {
		t.Fatalf("unexpected extraction runtime parse: %+v %v", runtime, ok)
	}
	for _, invalid := range []any{nil, map[string]any{}, map[string]any{"parallelRequestConcurrency": 0.0}, map[string]any{"parallelRequestConcurrency": 21.0}, map[string]any{"parallelRequestConcurrency": 1.5}} {
		if _, ok := parseAIExtractionRuntime(invalid); ok {
			t.Fatalf("invalid extraction runtime should be rejected: %+v", invalid)
		}
	}
	if shared, ok := parseAISharedProviders(map[string]any{"openrouter": map[string]any{"apiKey": nil}}); !ok || shared.OpenRouter.APIKey != nil {
		t.Fatalf("unexpected shared providers parse: %+v %v", shared, ok)
	}
	googleBooksKey := "google-books-key"
	if shared, ok := parseAISharedProviders(map[string]any{"googleBooks": map[string]any{"apiKey": googleBooksKey}}); !ok ||
		shared.GoogleBooks.APIKey == nil ||
		*shared.GoogleBooks.APIKey != googleBooksKey ||
		!shared.GoogleBooks.APIKeySet {
		t.Fatalf("unexpected Google Books provider parse: %+v %v", shared, ok)
	}
	if _, ok := parseAISharedProviders(map[string]any{"openrouter": map[string]any{"apiKey": 1}}); ok {
		t.Fatal("numeric shared provider apiKey should be rejected")
	}
	if _, ok := parseAISharedProviders(map[string]any{"googleBooks": map[string]any{"apiKey": 1}}); ok {
		t.Fatal("numeric Google Books apiKey should be rejected")
	}
	if _, ok := parseAISharedProviders(map[string]any{}); !ok {
		t.Fatal("missing openrouter shared provider should be accepted as no-op")
	}
	if _, ok := parseAISharedProviders(map[string]any{"openrouter": 1}); ok {
		t.Fatal("non-object openrouter shared provider should be rejected")
	}
	if _, ok := parseAISharedProviders(map[string]any{"googleBooks": 1}); ok {
		t.Fatal("non-object Google Books provider should be rejected")
	}
	if credentials, ok := parseAIProfileCredentials(nil); !ok || credentials.Source != "" {
		t.Fatalf("nil profile credentials should remain unspecified: %+v %v", credentials, ok)
	}
	if _, ok := parseAIProfileCredentials(map[string]any{"source": 1}); ok {
		t.Fatal("numeric credential source should be rejected")
	}
	if _, ok := parseAIProfileCredentials(map[string]any{"source": "bad"}); ok {
		t.Fatal("invalid credential source should be rejected")
	}
	if _, ok := parseAIProfile(map[string]any{"label": "Profile", "provider": "bad"}, 0); ok {
		t.Fatal("invalid provider should be rejected")
	}
	if _, ok := parseAIProfile(map[string]any{"label": "Profile", "provider": 1}, 0); ok {
		t.Fatal("numeric provider should be rejected")
	}
	if _, ok := parseAIProfile(map[string]any{"label": "Profile", "modelId": 1}, 0); ok {
		t.Fatal("numeric modelId should be rejected")
	}
	if _, ok := parseAIProfile(map[string]any{"label": "Profile", "allowFallbacks": "yes"}, 0); ok {
		t.Fatal("non-boolean allowFallbacks should be rejected")
	}
	if _, ok := parseAIProfile(map[string]any{"label": "Profile", "providerOrder": []any{1, " Valid ", ""}}, 0); ok {
		t.Fatal("provider order array with non-string entries should be rejected")
	}
	if _, ok := parseAIProfile(map[string]any{"label": "Profile", "providerOrder": []any{"OpenAI", "openai"}}, 0); ok {
		t.Fatal("duplicate provider order entries should be rejected")
	}
	if _, ok := parseAIProfile(map[string]any{"label": "Profile", "providerOrder": []any{"bad provider"}}, 0); ok {
		t.Fatal("provider order entries with unsafe characters should be rejected")
	}
	profile, ok := parseAIProfile(map[string]any{
		"label":             " Profile ",
		"modelId":           " model ",
		"credentials":       map[string]any{"source": "custom", "apiKey": nil},
		"providerOrder":     " A, B ,, ",
		"allowFallbacks":    true,
		"requireParameters": false,
	}, 1)
	if !ok || profile.ID != "profile-2" || profile.Label != "Profile" || profile.ModelID == nil || *profile.ModelID != "model" || len(profile.ProviderOrder) != 2 || profile.ProviderOrder[0] != "A" || profile.ProviderOrder[1] != "B" {
		t.Fatalf("unexpected profile parse: %+v %v", profile, ok)
	}
	if !profileInputExists([]store.AIProfileInput{profile}, "profile-2") || profileInputExists([]store.AIProfileInput{profile}, "missing") {
		t.Fatal("profileInputExists returned unexpected result")
	}
	if value, ok := nullableStringField(map[string]any{"value": 1}, "value"); ok || value != nil {
		t.Fatalf("non-string nullable field should be rejected: %v %v", value, ok)
	}
	if values, ok := parseStringArray(" Solo "); !ok || len(values) != 1 || values[0] != "Solo" {
		t.Fatalf("string provider order should parse as single entry: %+v %v", values, ok)
	}
	if values, ok := parseStringArray(nil); !ok || len(values) != 0 {
		t.Fatalf("nil provider order should parse empty: %+v %v", values, ok)
	}
	if _, ok := parseStringArray(1); ok {
		t.Fatal("numeric provider order should be rejected")
	}
	if _, ok := parseStringArray([]any{"a", 123, "b"}); ok {
		t.Fatal("mixed provider order array should be rejected")
	}
	if _, ok := parseStringArray(strings.Repeat("a", maxProviderOrderItemLength+1)); ok {
		t.Fatal("oversized provider order item should be rejected")
	}
	if _, ok := parseStringArray("OpenAI, openai"); ok {
		t.Fatal("duplicate provider order items should be rejected")
	}
	strategyModels, ok := parseAIExtractionStrategyModels(map[string]any{"nameDiscoveryModelId": " openai/gpt-5-nano "})
	if !ok || strategyModels.NameDiscoveryModelID == nil || *strategyModels.NameDiscoveryModelID != "openai/gpt-5-nano" {
		t.Fatalf("strategy model settings should parse: %+v %v", strategyModels, ok)
	}
	strategyModels, ok = parseAIExtractionStrategyModels(map[string]any{"nameDiscoveryModelId": " "})
	if !ok || strategyModels.NameDiscoveryModelID != nil {
		t.Fatalf("blank strategy model should normalize to nil: %+v %v", strategyModels, ok)
	}
	if _, ok := parseAIExtractionStrategyModels(map[string]any{"nameDiscoveryModelId": 123}); ok {
		t.Fatal("numeric strategy model should be rejected")
	}
	if _, ok := parseAIExtractionStrategyModels("bad"); ok {
		t.Fatal("non-object strategy model settings should be rejected")
	}
	if value, ok := boolField(map[string]any{"value": true}, "value", false); !ok || !value {
		t.Fatal("boolField returned unexpected values")
	}
	if value, ok := boolField(map[string]any{}, "value", true); !ok || !value {
		t.Fatal("boolField fallback returned unexpected values")
	}
	if _, ok := boolField(map[string]any{"value": 1}, "value", false); ok {
		t.Fatal("non-boolean boolField value should be rejected")
	}
}

func requestJSON(t *testing.T, handler http.Handler, method string, path string, body any, wantStatus int) map[string]any {
	t.Helper()
	raw := requestRaw(t, handler, method, path, body, wantStatus)
	var decoded map[string]any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		t.Fatalf("decode json %s: %v body=%s", path, err, raw)
	}
	return decoded
}

func assertSettingsModelInfo(t *testing.T, response map[string]any, contextLength float64, maxCompletionTokens float64) {
	t.Helper()
	settings := response["settings"].(map[string]any)
	profiles := settings["profiles"].([]any)
	profile := profiles[0].(map[string]any)
	modelInfo := profile["modelInfo"].(map[string]any)
	if modelInfo["contextLength"] != contextLength || modelInfo["maxCompletionTokens"] != maxCompletionTokens || modelInfo["source"] != "openrouter" {
		t.Fatalf("unexpected modelInfo: %+v", modelInfo)
	}
}

func waitForCharacterJobStatus(t *testing.T, handler http.Handler, novelID string, jobID string, status string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		response := requestJSON(t, handler, http.MethodGet, "/api/library/novels/"+novelID+"/extraction-jobs", nil, http.StatusOK)
		for _, rawJob := range response["jobs"].([]any) {
			job := rawJob.(map[string]any)
			if job["jobId"] == jobID {
				if job["status"] == status {
					return job
				}
				if job["status"] == "failed" {
					t.Fatalf("character job failed while waiting for %q: %+v", status, job)
				}
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for character job %s to become %s", jobID, status)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func testExtractionResponseContent(name string, summary string) string {
	payload := map[string]any{
		"processedUpToEpisodeIndex": "ep1",
		"newCharacters": []any{map[string]any{
			"canonicalName":               map[string]any{"text": name, "episodeIndex": "ep1"},
			"fullName":                    nil,
			"fullNameHistory":             []any{},
			"gender":                      nil,
			"genderHistory":               []any{},
			"firstAppearanceEpisodeIndex": "ep1",
			"aliases":                     []any{},
			"appearanceHistory":           []any{},
			"personalityHistory":          []any{},
			"summaryHistory":              []any{map[string]any{"text": summary, "episodeIndex": "ep1"}},
		}},
		"characterUpdates":   []any{},
		"mergeProposals":     []any{},
		"unresolvedMentions": []any{},
		"terms":              []any{},
	}
	raw, _ := json.Marshal(payload)
	return string(raw)
}

func testStringPtr(value string) *string {
	return &value
}

func requestJSONRaw(t *testing.T, handler http.Handler, method string, path string, body string, wantStatus int) map[string]any {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	setTestAPIContractHeaders(request)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != wantStatus {
		t.Fatalf("%s %s status=%d want=%d body=%s", method, path, response.Code, wantStatus, response.Body.String())
	}
	var decoded map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("decode raw json response %s: %v body=%s", path, err, response.Body.String())
	}
	return decoded
}

func requestRaw(t *testing.T, handler http.Handler, method string, path string, body any, wantStatus int) string {
	t.Helper()
	response := requestHTTP(t, handler, method, path, body)
	if response.Code != wantStatus {
		t.Fatalf("%s %s status=%d want=%d body=%s", method, path, response.Code, wantStatus, response.Body.String())
	}
	return response.Body.String()
}

func requestHTTP(t *testing.T, handler http.Handler, method string, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(raw)
	}
	request := httptest.NewRequest(method, path, reader)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	setTestAPIContractHeaders(request)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func setTestAPIContractHeaders(request *http.Request) {
	if !isAPIRequest(request) {
		return
	}
	request.Header.Set(apiContractVersionHeader, apiContractVersion)
	request.Header.Set(apiClientBuildHeader, "httpapi-test")
}

func decodeNDJSONEvents(t *testing.T, raw string) []map[string]any {
	t.Helper()
	events := []map[string]any{}
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("decode ndjson event: %v line=%s raw=%s", err, line, raw)
		}
		events = append(events, event)
	}
	return events
}

func eventIndex(events []map[string]any, eventType string, stage string) int {
	for index, event := range events {
		if event["type"] != eventType {
			continue
		}
		if stage != "" && event["stage"] != stage {
			continue
		}
		return index
	}
	return -1
}

func TestStorageUsageProgressStoreIgnoresStaleRunsAndResetsCancellation(t *testing.T) {
	store := newStorageUsageProgressStore()
	staleRun := store.start("scan-a")
	activeRun := store.start("scan-b")

	store.update(staleRun, storageusage.Progress{
		Phase:         storageusage.ProgressPhaseScanning,
		CheckedNovels: 1,
		TotalNovels:   3,
	})
	if snapshot := store.snapshot(activeRun); snapshot.CheckedNovels != 0 || snapshot.TotalNovels != 0 {
		t.Fatalf("different request progress should not bleed into active request: %+v", snapshot)
	}
	if snapshot := store.snapshot(staleRun); snapshot.CheckedNovels != 1 || snapshot.TotalNovels != 3 {
		t.Fatalf("request-specific progress should still be recorded: %+v", snapshot)
	}

	store.update(activeRun, storageusage.Progress{
		Phase:         storageusage.ProgressPhaseScanning,
		CheckedNovels: 1,
		TotalNovels:   3,
	})
	if snapshot := store.snapshot(""); snapshot.RequestID != activeRun || snapshot.State != storageUsageProgressRunning || snapshot.CheckedNovels != 1 || snapshot.TotalNovels != 3 {
		t.Fatalf("active progress update should be recorded: %+v", snapshot)
	}

	store.reset(activeRun)
	if snapshot := store.snapshot(activeRun); snapshot.State != storageUsageProgressIdle || snapshot.Phase != storageusage.ProgressPhasePreparing || snapshot.CheckedNovels != 0 || snapshot.TotalNovels != 0 || snapshot.Error != "" {
		t.Fatalf("canceled progress should reset to idle: %+v", snapshot)
	}
}

func TestStorageUsageProgressStoreSnapshotsFailuresAndPruning(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/system/storage/progress?requestId=%20scan-1%20", nil)
	if got := storageUsageProgressRequestID(request); got != "scan-1" {
		t.Fatalf("request id should be trimmed, got %q", got)
	}

	store := newStorageUsageProgressStore()
	generatedRun := store.start("")
	if !strings.HasPrefix(generatedRun, "storage-scan-") {
		t.Fatalf("blank request id should generate a stable run id, got %q", generatedRun)
	}
	store.fail(generatedRun, "disk unavailable")
	if snapshot := store.snapshot(generatedRun); snapshot.State != storageUsageProgressError || snapshot.Error != "disk unavailable" {
		t.Fatalf("failed progress should be retained by request id: %+v", snapshot)
	}
	if snapshot := store.snapshot("missing"); snapshot.RequestID != "missing" || snapshot.State != storageUsageProgressIdle || snapshot.Phase != storageusage.ProgressPhasePreparing {
		t.Fatalf("unknown request-specific progress should be idle: %+v", snapshot)
	}

	for i := 0; i < maxStorageUsageProgressRuns+2; i++ {
		store.start(fmt.Sprintf("scan-%d", i))
	}
	if snapshot := store.snapshot(generatedRun); snapshot.State != storageUsageProgressIdle || snapshot.Error != "" {
		t.Fatalf("old progress entries should be pruned: %+v", snapshot)
	}
	latest := fmt.Sprintf("scan-%d", maxStorageUsageProgressRuns+1)
	if snapshot := store.snapshot(""); snapshot.RequestID != latest || snapshot.State != storageUsageProgressRunning {
		t.Fatalf("latest progress should remain the default snapshot: %+v", snapshot)
	}
}

func findRuntimeService(status map[string]any, id string) map[string]any {
	services, ok := status["services"].([]any)
	if !ok {
		return nil
	}
	for _, entry := range services {
		service, ok := entry.(map[string]any)
		if ok && service["id"] == id {
			return service
		}
	}
	return nil
}

func stringPtr(value string) *string {
	return &value
}

func newHTTPAPITestData(t *testing.T) string {
	t.Helper()
	dataDir := t.TempDir()
	libraryRoot := filepath.Join(dataDir, "novel-fetcher")
	if err := os.MkdirAll(filepath.Join(libraryRoot, "works", "syosetu", "n1234", "episodes"), 0o755); err != nil {
		t.Fatalf("mkdir library fixture: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(libraryRoot, "works", "syosetu", "n1234", "assets", "episodes", "1"), 0o755); err != nil {
		t.Fatalf("mkdir asset fixture: %v", err)
	}
	stateDir := filepath.Join(dataDir, "state")
	if err := os.MkdirAll(filepath.Join(stateDir, "character_profiles"), 0o755); err != nil {
		t.Fatalf("mkdir character profile fixture: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(stateDir, "extraction_jobs"), 0o755); err != nil {
		t.Fatalf("mkdir character job fixture: %v", err)
	}
	db, err := sql.Open("sqlite", filepath.Join(libraryRoot, "library.sqlite"))
	if err != nil {
		t.Fatalf("open library sqlite: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`
		CREATE TABLE works (
			id INTEGER PRIMARY KEY,
			site TEXT NOT NULL,
			site_name TEXT NOT NULL,
			site_work_id TEXT NOT NULL,
			source_url TEXT NOT NULL,
			title TEXT NOT NULL,
			author TEXT NOT NULL,
			story TEXT NOT NULL,
			directory TEXT NOT NULL,
			fetched_at TEXT NOT NULL,
			fetch_status TEXT NOT NULL,
			last_fetch_error TEXT NOT NULL,
			last_failed_episode_id TEXT NOT NULL,
			resume_episode_id TEXT NOT NULL,
			expected_episode_count INTEGER NOT NULL
		);
		CREATE TABLE episodes (
			work_id INTEGER NOT NULL,
			episode_id TEXT NOT NULL,
			site_episode_id TEXT NOT NULL,
			source_url TEXT NOT NULL,
			sort_order INTEGER NOT NULL,
			display_index TEXT NOT NULL,
			title TEXT NOT NULL,
			chapter TEXT NOT NULL,
			subchapter TEXT NOT NULL,
			published_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			body_path TEXT NOT NULL,
			raw_path TEXT NOT NULL,
			content_hash TEXT NOT NULL,
			fetched_at TEXT NOT NULL,
			body_status TEXT NOT NULL,
			last_fetch_error TEXT NOT NULL
		);
		INSERT INTO works VALUES (1, 'syosetu', '小説家になろう', 'n1234', 'https://ncode.syosetu.com/n1234/', 'Fixture Novel', 'Author', 'Story', 'works/syosetu/n1234', '2026-01-01T00:00:00Z', 'complete', '', '', '', 1);
		INSERT INTO episodes VALUES (1, '1', '1', 'https://ncode.syosetu.com/n1234/1/', 0, '1', 'Episode 1', 'Chapter', '', '2026-01-01T00:00:00Z', '2026-01-02T00:00:00Z', 'works/syosetu/n1234/episodes/1.json', '', 'hash-1', '2026-01-02T00:00:00Z', 'complete', '');
	`); err != nil {
		t.Fatalf("seed library sqlite: %v", err)
	}
	writeHTTPFixtureFile(t, filepath.Join(libraryRoot, "works", "syosetu", "n1234", "episodes", "1.json"), `{
		"schema_version": 1,
		"episode_id": "1",
		"site_episode_id": "1",
		"source_url": "https://ncode.syosetu.com/n1234/1/",
		"sort_order": 0,
		"display_index": "1",
		"title": "Episode 1",
		"chapter": "Chapter",
		"subchapter": "",
		"published_at": "2026-01-01T00:00:00Z",
		"updated_at": "2026-01-02T00:00:00Z",
		"blocks": [{"type":"paragraph","section":"body","text":"本文です。"}],
		"fetched_at": "2026-01-02T00:00:00Z"
	}`)
	writeHTTPFixtureFile(t, filepath.Join(libraryRoot, "works", "syosetu", "n1234", "assets", "episodes", "1", "pic.jpg"), "jpg")
	novelID := library.NovelID(library.Work{ID: 1, Site: "syosetu", SiteWorkID: "n1234"})
	writeHTTPFixtureFile(t, filepath.Join(stateDir, "character_profiles", novelID+".yaml"), `
novel_id: `+novelID+`
processed_up_to_episode_index: "1"
characters:
  - character_id: alice
    canonical_name:
      text: アリス
      episode_index: "1"
    full_name: null
    gender: null
    first_appearance_episode_index: "1"
    aliases: []
    appearance_history: []
    personality_history: []
    summary_history:
      - episode_index: "1"
        text: テスト人物。
`)
	writeHTTPFixtureFile(t, filepath.Join(stateDir, "extraction_jobs", "job-1.yaml"), `
schema_version: 2
revision: 1
job_id: job-1
novel_id: `+novelID+`
requested_up_to_episode_index: "1"
profile_id: default
profile_label: Default
generation_mode: heuristic
model_id: null
status: completed
created_at: 2026-01-01T00:00:00Z
started_at: 2026-01-01T00:00:01Z
finished_at: 2026-01-01T00:00:02Z
error_message: null
`)
	seedHTTPAIUsage(t, filepath.Join(stateDir, "ai_usage.sqlite"))
	return dataDir
}

func writeHTTPFixtureFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture file: %v", err)
	}
}

func seedHTTPAIUsage(t *testing.T, dbPath string) {
	t.Helper()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open ai usage sqlite: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`
		CREATE TABLE ai_usage_runs (
			run_id TEXT PRIMARY KEY,
			feature TEXT NOT NULL,
			workflow_name TEXT NOT NULL,
			status TEXT NOT NULL,
			started_at TEXT NOT NULL,
			finished_at TEXT NOT NULL,
			elapsed_ms INTEGER NOT NULL,
			novel_id TEXT,
			novel_title TEXT,
			current_episode_index TEXT,
			model_id TEXT,
			profile_id TEXT,
			profile_label TEXT,
			generation_mode TEXT NOT NULL,
			answer_chars INTEGER NOT NULL,
			request_count INTEGER NOT NULL,
			input_tokens INTEGER NOT NULL,
			output_tokens INTEGER NOT NULL,
			total_tokens INTEGER NOT NULL,
			cached_input_tokens INTEGER NOT NULL,
			reasoning_output_tokens INTEGER NOT NULL,
			total_cost REAL NOT NULL,
			tool_call_count INTEGER NOT NULL,
			tool_result_count INTEGER NOT NULL,
			error_message TEXT
		);
		CREATE TABLE ai_usage_requests (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			run_id TEXT NOT NULL,
			request_index INTEGER NOT NULL,
			kind TEXT NOT NULL,
			parent_request_index INTEGER,
			tool_names TEXT NOT NULL,
			tool_summaries TEXT NOT NULL,
			input_tokens INTEGER NOT NULL,
			output_tokens INTEGER NOT NULL,
			total_tokens INTEGER NOT NULL,
			cached_input_tokens INTEGER NOT NULL,
			reasoning_output_tokens INTEGER NOT NULL,
			cost REAL NOT NULL
		);
		CREATE TABLE ai_usage_run_snapshots (
			run_id TEXT PRIMARY KEY,
			snapshot_json TEXT NOT NULL
		);
		INSERT INTO ai_usage_runs (
			run_id, feature, workflow_name, status, started_at, finished_at, elapsed_ms,
			novel_id, novel_title, current_episode_index, model_id, profile_id, profile_label,
			generation_mode, answer_chars, request_count, input_tokens, output_tokens, total_tokens,
			cached_input_tokens, reasoning_output_tokens, total_cost, tool_call_count, tool_result_count,
			error_message
		) VALUES ('run-http', 'extraction', 'Extraction', 'completed', '2026-01-01T00:00:00Z', '2026-01-01T00:00:01Z', 1000, 'n0000aa', 'Novel', '1', 'openrouter/auto', 'default', 'Default', 'openrouter', 10, 1, 1, 2, 3, 0, 0, 0.01, 0, 0, NULL);
		INSERT INTO ai_usage_requests (
			run_id, request_index, kind, parent_request_index, tool_names, tool_summaries,
			input_tokens, output_tokens, total_tokens, cached_input_tokens, reasoning_output_tokens, cost
		) VALUES ('run-http', 1, 'chat', NULL, '[]', '[]', 1, 2, 3, 0, 0, 0.01);
		INSERT INTO ai_usage_run_snapshots (run_id, snapshot_json)
		VALUES ('run-http', '{"runId":"run-http","schemaVersion":1}');
	`); err != nil {
		t.Fatalf("seed ai usage sqlite: %v", err)
	}
}

func newHTTPAPIFetcherServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v2/system/version":
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"current": "novel-fetcher/v1.2.3", "latest": "v1.2.4"}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v2/system/queue":
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"total": 1, "web_worker": 0, "worker": 1, "running": true}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v2/tasks/summary":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data": map[string]any{
					"current":          nil,
					"queued":           []any{},
					"recent_completed": []any{map[string]any{"id": "done"}},
					"recent_failed":    []any{},
					"completed_count":  1,
					"failed_count":     0,
					"convert_current":  nil,
					"convert_queued":   []any{},
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v2/novels/download":
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"targets": []string{"https://example.test/novel"}, "task_ids": []string{"task-download"}}})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v2/novels/update":
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"ids": []int{1}, "task_ids": []string{"task-update"}, "skip_unchanged": true}})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v2/novels/resume":
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"ids": []int{1}, "task_ids": []string{"task-resume"}}})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v2/novels/remove":
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"ids": []string{"1"}}, "message": "Novel removed"})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v2/tasks/task-1/cancel":
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"task_id": "task-1", "cancelled": true}})
		default:
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": map[string]any{"message": "missing"}})
		}
	}))
	t.Cleanup(server.Close)
	return server
}
