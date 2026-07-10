package readerassistant

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"narou-viewer/apps/viewer-api-go/internal/ai"
	"narou-viewer/apps/viewer-api-go/internal/characters"
	"narou-viewer/apps/viewer-api-go/internal/library"
	"narou-viewer/apps/viewer-api-go/internal/terms"
)

func TestNormalizeHistoryKeepsLatestValidTurns(t *testing.T) {
	raw := []any{}
	for i := 0; i < 14; i++ {
		raw = append(raw, map[string]any{"role": "user", "text": " message "})
	}
	raw = append(raw,
		map[string]any{"role": "system", "text": "ignored"},
		map[string]any{"role": "assistant", "text": 10},
		map[string]any{"role": "assistant", "text": " answer "},
	)

	history := NormalizeHistory(raw)
	if len(history) != 8 {
		t.Fatalf("history should keep latest 8 valid messages, got %d", len(history))
	}
	if history[len(history)-1]["role"] != "assistant" || history[len(history)-1]["text"] != "answer" {
		t.Fatalf("history should trim and keep the latest assistant turn: %+v", history[len(history)-1])
	}
}

func TestUsageRequestsIncludeToolCallsAndFinalAnswer(t *testing.T) {
	requests := UsageRequests([]map[string]any{{"name": "search_episodes"}}, 3, 4)
	if len(requests) != 2 {
		t.Fatalf("usage requests length = %d", len(requests))
	}
	if requests[0].Kind != "tool_call" || requests[0].ToolNames[0] != "search_episodes" {
		t.Fatalf("tool request was not recorded: %+v", requests[0])
	}
	if requests[1].Kind != "final_answer" || requests[1].InputTokens != 3 || requests[1].OutputTokens != 4 || requests[1].TotalTokens != 7 {
		t.Fatalf("final answer token usage was not recorded: %+v", requests[1])
	}
}

func TestNilServiceRespondsUnavailable(t *testing.T) {
	var service *Service
	_, err := service.Respond(t.Context(), Request{}, nil)
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("nil service should be unavailable, got %v", err)
	}
}

func TestEstimateOpenRouterChatRequestTokensIncludesToolSchema(t *testing.T) {
	messages := []ai.ChatMessage{{Role: "user", Content: "本文"}}
	messageOnly := EstimateOpenRouterChatRequestTokens(messages, nil, nil)
	withToolSchema := EstimateOpenRouterChatRequestTokens(messages, ToolDefinitions(), nil)
	if withToolSchema <= messageOnly {
		t.Fatalf("tool schema should increase the prompt estimate: messageOnly=%d withToolSchema=%d", messageOnly, withToolSchema)
	}
}

func TestCharacterSnapshotReturnsPartialDataAtProcessedFrontier(t *testing.T) {
	stateDir := t.TempDir()
	const novelID = "novel-characters"
	if err := characters.SaveGeneratedSummary(stateDir, novelID, "2", []characters.GeneratedCharacter{{
		CanonicalName:               "アリス",
		CanonicalEpisodeIndex:       "1",
		FirstAppearanceEpisodeIndex: "1",
	}}); err != nil {
		t.Fatalf("save character snapshot: %v", err)
	}
	tocEpisodes := []library.TocEpisodeSummary{
		{EpisodeIndex: "1", Title: "第一話"},
		{EpisodeIndex: "2", Title: "第二話"},
		{EpisodeIndex: "3", Title: "第三話"},
	}
	result := NewService(Dependencies{StateDir: stateDir}).characterSnapshotResult(novelID, "3", tocEpisodes)
	if result["status"] != "partial" || result["characterCount"] != 1 || result["fallbackTool"] != "search_full_text" {
		t.Fatalf("character snapshot beyond the processed frontier should return partial data: %+v", result)
	}
}

func TestTermSnapshotRespectsCurrentEpisodeAndCharacterCommitFrontier(t *testing.T) {
	stateDir := t.TempDir()
	const novelID = "novel-terms"
	if err := characters.SaveGeneratedSummary(stateDir, novelID, "2", nil); err != nil {
		t.Fatalf("save character commit frontier: %v", err)
	}
	if err := terms.SaveGeneratedTerms(stateDir, novelID, "3", []terms.GeneratedTerm{
		{
			Term:            "聖剣",
			ReadingHistory:  []terms.TextVersion{{Text: "せいけん", EpisodeIndex: "1"}},
			CategoryHistory: []terms.CategoryVersion{{Category: terms.CategoryItem, EpisodeIndex: "1"}},
			DescriptionHistory: []terms.HistoryVersion{
				{Text: "古い剣。", EpisodeIndex: "1"},
				{Text: "王家の聖剣。", EpisodeIndex: "3"},
			},
		},
		{
			Term:               "王都",
			CategoryHistory:    []terms.CategoryVersion{{Category: terms.CategoryPlace, EpisodeIndex: "3"}},
			DescriptionHistory: []terms.HistoryVersion{{Text: "王国の首都。", EpisodeIndex: "3"}},
		},
	}, nil); err != nil {
		t.Fatalf("save generated terms: %v", err)
	}
	tocEpisodes := []library.TocEpisodeSummary{
		{EpisodeIndex: "1", Title: "第一話"},
		{EpisodeIndex: "2", Title: "第二話"},
		{EpisodeIndex: "3", Title: "第三話"},
	}
	service := NewService(Dependencies{StateDir: stateDir})

	ready := service.termSnapshotResult(novelID, "2", tocEpisodes)
	if ready["status"] != "ready" || ready["termCount"] != 1 {
		t.Fatalf("term snapshot at committed frontier should be ready: %+v", ready)
	}
	items := ready["terms"].([]map[string]any)
	if len(items) != 1 || items[0]["term"] != "聖剣" || items[0]["description"] != "古い剣。" || items[0]["category"] != terms.CategoryItem {
		t.Fatalf("term snapshot should project only visible versions: %+v", items)
	}

	partial := service.termSnapshotResult(novelID, "3", tocEpisodes)
	if partial["status"] != "partial" || partial["termCount"] != 1 || partial["fallbackTool"] != "search_full_text" {
		t.Fatalf("term snapshot beyond character frontier should hide uncommitted terms and suggest fallback: %+v", partial)
	}

	missing := service.termSnapshotResult("missing", "2", tocEpisodes)
	if missing["status"] != "not_generated" || missing["termCount"] != 0 || missing["fallbackTool"] != "search_full_text" {
		t.Fatalf("missing term snapshot should be recoverable: %+v", missing)
	}
}

func TestReaderAssistantExposesTermSnapshotTool(t *testing.T) {
	found := false
	for _, tool := range ToolDefinitions() {
		if tool.Function.Name == "get_term_snapshot" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("get_term_snapshot tool definition is missing")
	}
	if message := ToolResultMessage("get_term_snapshot"); message != "用語情報を確認しました。" {
		t.Fatalf("unexpected term snapshot result message: %s", message)
	}
}

func TestSearchFullTextResultUsesPersistentTextCache(t *testing.T) {
	fakeLibrary := newReaderAssistantCountingLibrary(3, 0)
	service := NewService(Dependencies{Library: fakeLibrary, StateDir: t.TempDir()})
	contextInfo := readerAssistantBenchmarkContext(3)

	first := service.searchFullTextResult(t.Context(), contextInfo, "needle", 1, 3, 10)
	if fakeLibrary.callCount() != 3 {
		t.Fatalf("first search should fetch each episode, calls=%d", fakeLibrary.callCount())
	}
	firstMetadata := first["metadata"].(map[string]any)
	if firstMetadata["cacheMissCount"] != 3 || firstMetadata["cacheHitCount"] != 0 || firstMetadata["failedEpisodeCount"] != 0 {
		t.Fatalf("first search metadata should expose cold-cache misses: %+v", firstMetadata)
	}

	contextInfo.HitRegistry = NewHitRegistry()
	second := service.searchFullTextResult(t.Context(), contextInfo, "needle", 1, 3, 10)
	if fakeLibrary.callCount() != 3 {
		t.Fatalf("second search should use persistent cache without fetching again, calls=%d", fakeLibrary.callCount())
	}
	secondMetadata := second["metadata"].(map[string]any)
	if secondMetadata["cacheHitCount"] != 3 || secondMetadata["cacheMissCount"] != 0 || secondMetadata["loadedEpisodeCount"] != 3 {
		t.Fatalf("second search metadata should expose warm-cache hits: %+v", secondMetadata)
	}
	if second["candidateCount"].(int) == 0 {
		t.Fatalf("warm-cache search should keep matching behavior: %+v", second)
	}
}

func TestSearchFullTextResultRefetchesChangedEtagAndReportsFailures(t *testing.T) {
	fakeLibrary := newReaderAssistantCountingLibrary(3, 0)
	service := NewService(Dependencies{Library: fakeLibrary, StateDir: t.TempDir()})
	contextInfo := readerAssistantBenchmarkContext(3)

	service.searchFullTextResult(t.Context(), contextInfo, "needle", 1, 3, 10)
	fakeLibrary.episodes["2"] = readerAssistantTestEpisode("2", "etag-2b", "updated needle body")
	delete(fakeLibrary.episodes, "3")
	contextInfo = readerAssistantBenchmarkContext(3)
	contextInfo.TocEpisodes[1].ContentEtag = "etag-2b"
	contextInfo.TocEpisodes[2].ContentEtag = "etag-3b"

	result := service.searchFullTextResult(t.Context(), contextInfo, "needle", 1, 3, 10)
	if fakeLibrary.calls["2"] != 2 {
		t.Fatalf("changed ETag episode should be fetched again, calls=%v", fakeLibrary.calls)
	}
	metadata := result["metadata"].(map[string]any)
	if metadata["cacheHitCount"] != 1 || metadata["cacheMissCount"] != 2 || metadata["failedEpisodeCount"] != 1 {
		t.Fatalf("metadata should expose partial cache hits and failed episodes: %+v", metadata)
	}
}

func TestSearchFullTextResultDoesNotSaveWhenTocEtagIsEmpty(t *testing.T) {
	fakeLibrary := newReaderAssistantCountingLibrary(1, 0)
	service := NewService(Dependencies{Library: fakeLibrary, StateDir: t.TempDir()})
	contextInfo := readerAssistantBenchmarkContext(1)
	contextInfo.TocEpisodes[0].ContentEtag = ""

	service.searchFullTextResult(t.Context(), contextInfo, "needle", 1, 1, 10)
	contextInfo.HitRegistry = NewHitRegistry()
	result := service.searchFullTextResult(t.Context(), contextInfo, "needle", 1, 1, 10)
	if fakeLibrary.callCount() != 2 {
		t.Fatalf("empty TOC ETag should not create a reusable persistent cache row, calls=%d", fakeLibrary.callCount())
	}
	metadata := result["metadata"].(map[string]any)
	if metadata["cacheDisabledCount"] != 1 || metadata["cacheHitCount"] != 0 {
		t.Fatalf("empty TOC ETag should be reported as cache disabled: %+v", metadata)
	}
}

func BenchmarkSearchFullTextResultCache(b *testing.B) {
	const episodeCount = 80
	for _, tc := range []struct {
		name     string
		stateDir string
		prewarm  bool
	}{
		{name: "cache_disabled_fetch_each_episode"},
		{name: "warm_persistent_cache", stateDir: b.TempDir(), prewarm: true},
	} {
		b.Run(tc.name, func(b *testing.B) {
			fakeLibrary := newReaderAssistantCountingLibrary(episodeCount, 250*time.Microsecond)
			service := NewService(Dependencies{Library: fakeLibrary, StateDir: tc.stateDir})
			if tc.prewarm {
				service.searchFullTextResult(context.Background(), readerAssistantBenchmarkContext(episodeCount), "needle", 1, episodeCount, 20)
				fakeLibrary.resetCalls()
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				result := service.searchFullTextResult(context.Background(), readerAssistantBenchmarkContext(episodeCount), "needle", 1, episodeCount, 20)
				if result["candidateCount"].(int) == 0 {
					b.Fatal("expected full-text candidates")
				}
			}
			b.ReportMetric(float64(fakeLibrary.callCount())/float64(b.N), "fetches/op")
		})
	}
}

type readerAssistantCountingLibrary struct {
	episodes map[string]*library.EpisodeResponse
	calls    map[string]int
	delay    time.Duration
}

func newReaderAssistantCountingLibrary(episodeCount int, delay time.Duration) *readerAssistantCountingLibrary {
	episodes := map[string]*library.EpisodeResponse{}
	for i := 1; i <= episodeCount; i++ {
		index := strconv.Itoa(i)
		text := fmt.Sprintf("episode %03d needle %s", i, strings.Repeat("body ", 16))
		episodes[index] = readerAssistantTestEpisode(index, "etag-"+index, text)
	}
	return &readerAssistantCountingLibrary{episodes: episodes, calls: map[string]int{}, delay: delay}
}

func (l *readerAssistantCountingLibrary) GetToc(context.Context, string) (*library.TocResponse, error) {
	return nil, nil
}

func (l *readerAssistantCountingLibrary) GetEpisode(_ context.Context, _ string, episodeIndex string) (*library.EpisodeResponse, error) {
	if l.delay > 0 {
		time.Sleep(l.delay)
	}
	l.calls[episodeIndex]++
	episode := l.episodes[episodeIndex]
	if episode == nil {
		return nil, nil
	}
	copied := *episode
	return &copied, nil
}

func (l *readerAssistantCountingLibrary) callCount() int {
	total := 0
	for _, count := range l.calls {
		total += count
	}
	return total
}

func (l *readerAssistantCountingLibrary) resetCalls() {
	l.calls = map[string]int{}
}

func readerAssistantBenchmarkContext(episodeCount int) Context {
	episodes := make([]library.TocEpisodeSummary, 0, episodeCount)
	for i := 1; i <= episodeCount; i++ {
		index := strconv.Itoa(i)
		episodes = append(episodes, library.TocEpisodeSummary{
			EpisodeIndex: index,
			Title:        "Episode " + index,
			ContentEtag:  "etag-" + index,
		})
	}
	return Context{
		NovelID:              "novel-1",
		CurrentEpisodeIndex:  strconv.Itoa(episodeCount),
		CurrentEpisodeNumber: episodeCount,
		TocEpisodes:          episodes,
		HitRegistry:          NewHitRegistry(),
	}
}

func readerAssistantTestEpisode(index string, contentEtag string, text string) *library.EpisodeResponse {
	return &library.EpisodeResponse{
		NovelID:      "novel-1",
		EpisodeIndex: index,
		Title:        "Episode " + index,
		ContentEtag:  contentEtag,
		ReaderDocument: library.ReaderDocument{Blocks: []library.ReaderBlock{{
			Type:    "paragraph",
			Section: "body",
			Inlines: []library.ReaderInline{{Type: "text", Text: text}},
		}}},
	}
}
