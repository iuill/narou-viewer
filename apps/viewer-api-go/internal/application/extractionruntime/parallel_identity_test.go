package extractionruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"narou-viewer/apps/viewer-api-go/internal/ai"
	appextraction "narou-viewer/apps/viewer-api-go/internal/application/extraction"
	"narou-viewer/apps/viewer-api-go/internal/characters"
	core "narou-viewer/apps/viewer-api-go/internal/extraction"
	"narou-viewer/apps/viewer-api-go/internal/store"
	"narou-viewer/apps/viewer-api-go/internal/terms"
)

func newExtractionOpenRouterTestServer(t *testing.T, responses ...string) *httptest.Server {
	t.Helper()
	index := 0
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if index >= len(responses) {
			t.Fatalf("unexpected OpenRouter request #%d", index+1)
		}
		content := responses[index]
		index++
		if err := json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": content}}},
			"usage":   map[string]any{"prompt_tokens": 11, "completion_tokens": 7, "total_tokens": 18},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
}

func TestGenerateOpenRouterExtractionParallelIdentityRejectsMissingConfig(t *testing.T) {
	runtime := NewRuntime(RuntimeDependencies{StateDir: t.TempDir()})
	generated, state, usage, err := runtime.generateOpenRouterExtractionParallelIdentity(context.Background(), nil, "novel-1", "1", nil, nil, nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "AI生成プロファイル") {
		t.Fatalf("err = %v", err)
	}
	if len(generated) != 0 || len(usage) != 0 || len(state.UnresolvedMentions) != 0 {
		t.Fatalf("generated=%+v state=%+v usage=%+v", generated, state, usage)
	}
}

func TestGenerateOpenRouterExtractionParallelIdentityEmptyInput(t *testing.T) {
	runtime := NewRuntime(RuntimeDependencies{StateDir: t.TempDir()})
	generated, state, usage, err := runtime.generateOpenRouterExtractionParallelIdentity(context.Background(), &store.ResolvedAIGenerationConfig{}, "novel-empty", "1", nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("generateOpenRouterExtractionParallelIdentity returned error: %v", err)
	}
	if len(generated) != 0 || len(usage) != 0 || len(state.UnresolvedMentions) != 0 || len(state.RetiredCharacterIDs) != 0 {
		t.Fatalf("generated=%+v state=%+v usage=%+v", generated, state, usage)
	}
}

func TestGenerateOpenRouterExtractionDiscoveryParallelCorrectionWithSeed(t *testing.T) {
	openrouter := newExtractionOpenRouterTestServer(t, `{"terms":[],"characters":[{"characterId":"char_seed","canonicalName":"アリス姫","aliases":["姫様"],"keep":true,"reason":"代表名を補正"}]}`)
	defer openrouter.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", openrouter.URL)
	runtime := NewRuntime(RuntimeDependencies{StateDir: t.TempDir()})
	seed := []characters.GeneratedCharacter{{
		CharacterID:                 "char_seed",
		CanonicalName:               "アリス",
		CanonicalEpisodeIndex:       "1",
		FirstAppearanceEpisodeIndex: "1",
	}}
	generated, state, usage, err := runtime.generateOpenRouterExtractionDiscoveryParallelCorrection(context.Background(), &store.ResolvedAIGenerationConfig{APIKey: "sk-test", ModelID: "base-model", AllowFallbacks: true}, "novel-d", "1", seed, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("generateOpenRouterExtractionDiscoveryParallelCorrection returned error: %v", err)
	}
	if len(generated) != 1 || generated[0].CanonicalName != "アリス姫" || len(usage) != 1 || usage[0].Kind != "extraction_correction" {
		t.Fatalf("generated=%+v usage=%+v", generated, usage)
	}
	if len(state.UnresolvedMentions) != 0 || len(state.RetiredCharacterIDs) != 0 {
		t.Fatalf("state = %+v", state)
	}
}

func TestExtractionWorkflowPortsGenerateParallelIdentityWrapsServer(t *testing.T) {
	runtime := NewRuntime(RuntimeDependencies{StateDir: t.TempDir()})
	generated, state, usage, err := runtime.GenerateParallelIdentity(context.Background(), nil, "novel-1", "1", nil, nil, nil, nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "AI生成プロファイル") {
		t.Fatalf("err = %v", err)
	}
	if len(generated) != 0 || len(usage) != 0 || len(state.UnresolvedMentions) != 0 {
		t.Fatalf("generated=%+v state=%+v usage=%+v", generated, state, usage)
	}
}

func TestExtractionWorkflowPortsGenerateDiscoveryParallelCorrectionWrapsServer(t *testing.T) {
	runtime := NewRuntime(RuntimeDependencies{StateDir: t.TempDir()})
	generated, state, usage, err := runtime.GenerateDiscoveryParallelCorrection(context.Background(), nil, "novel-1", "1", nil, nil, nil, nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "AI生成プロファイル") {
		t.Fatalf("err = %v", err)
	}
	if len(generated) != 0 || len(usage) != 0 || len(state.UnresolvedMentions) != 0 {
		t.Fatalf("generated=%+v state=%+v usage=%+v", generated, state, usage)
	}
}

func TestParallelIdentityRuntimeAndExtractionEmptyInput(t *testing.T) {
	runtime := NewRuntime(RuntimeDependencies{StateDir: t.TempDir()})
	config := &store.ResolvedAIGenerationConfig{}
	pending := []characters.GeneratedUnresolvedMention{{Mention: "謎の男", EpisodeIndex: "1"}}
	batches, err := runtime.parallelIdentityRuntimeBatches(context.Background(), config, "novel-1", "1", nil, nil, nil, pending)
	if err != nil {
		t.Fatalf("parallelIdentityRuntimeBatches returned error: %v", err)
	}
	if len(batches) != 0 {
		t.Fatalf("batches = %+v", batches)
	}

	candidates, rawTerms, proposals, usage, unresolved, err := runtime.extractParallelIdentityCandidates(context.Background(), config, "novel-1", "1", nil, nil, func(appextraction.BatchProgress) {
		t.Fatal("progress sink should not be called for empty batches")
	}, pending)
	if err != nil {
		t.Fatalf("extractParallelIdentityCandidates returned error: %v", err)
	}
	if len(candidates) != 0 || len(rawTerms) != 0 || len(proposals) != 0 || len(usage) != 0 || len(unresolved) != 1 || unresolved[0].Mention != "謎の男" {
		t.Fatalf("candidates=%+v usage=%+v unresolved=%+v", candidates, usage, unresolved)
	}
}

func TestExtractParallelIdentityCandidatesReturnsFirstBatchError(t *testing.T) {
	runtime := NewRuntime(RuntimeDependencies{StateDir: t.TempDir()})
	progress := []string{}
	batch := extractionBatch{
		BatchIndex: 1,
		Chunks:     []extractionChunk{{EpisodeIndex: "1", Title: "第一話", Text: "アリスが廊下に立っていた。"}},
	}
	candidates, rawTerms, proposals, usage, unresolved, err := runtime.extractParallelIdentityCandidates(context.Background(), &store.ResolvedAIGenerationConfig{}, "novel-1", "1", nil, []extractionBatch{batch}, func(item appextraction.BatchProgress) {
		progress = append(progress, item.Phase)
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "OpenRouter API key and modelId are required") {
		t.Fatalf("err = %v", err)
	}
	if len(candidates) != 0 || len(rawTerms) != 0 || len(proposals) != 0 || len(usage) != 0 || len(unresolved) != 0 {
		t.Fatalf("candidates=%+v usage=%+v unresolved=%+v", candidates, usage, unresolved)
	}
	if len(progress) != 2 || progress[0] != "parallelStart" || progress[1] != "error" {
		t.Fatalf("progress = %+v", progress)
	}
}

func TestExtractParallelIdentityCandidatesCollectsSuccessfulBatches(t *testing.T) {
	openrouter := newExtractionOpenRouterTestServer(
		t,
		`{"processedUpToEpisodeIndex":"1","terms":[{"term":"魔導院","reading":null,"category":{"value":"organization","episodeIndex":"1"},"descriptionHistory":[{"text":"王都にある。","episodeIndex":"1"}]}],"characters":[{"canonicalName":"アリス","summary":"廊下に立っていた人物。"}]}`,
		`{"processedUpToEpisodeIndex":"2","terms":[{"term":"白銀騎士団","reading":null,"category":{"value":"organization","episodeIndex":"2"},"descriptionHistory":[{"text":"村へ派遣された。","episodeIndex":"2"}]}],"characters":[{"canonicalName":"ボブ","summary":"庭にいた人物。"}]}`,
	)
	defer openrouter.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", openrouter.URL)
	t.Setenv("EXTRACTION_LLM_START_INTERVAL_MS", "0")

	runtime := NewRuntime(RuntimeDependencies{StateDir: t.TempDir()})
	progressEvents := []appextraction.BatchProgress{}
	batches := []extractionBatch{
		{
			BatchIndex:     1,
			BatchCount:     2,
			EpisodeIndexes: []string{"1"},
			Chunks:         []extractionChunk{{EpisodeIndex: "1", Title: "第一話", Text: "アリスが廊下に立っていた。"}},
		},
		{
			BatchIndex:     2,
			BatchCount:     2,
			EpisodeIndexes: []string{"2"},
			Chunks:         []extractionChunk{{EpisodeIndex: "2", Title: "第二話", Text: "ボブが庭にいた。"}},
		},
	}
	candidates, rawTerms, proposals, usage, unresolved, err := runtime.extractParallelIdentityCandidates(
		context.Background(),
		&store.ResolvedAIGenerationConfig{APIKey: "sk-test", ModelID: "openai/gpt-5.4-mini", AllowFallbacks: true, ExtractionParallelConcurrency: 1},
		"novel-1",
		"2",
		nil,
		batches,
		func(progress appextraction.BatchProgress) {
			progressEvents = append(progressEvents, progress)
		},
		nil,
	)
	if err != nil {
		t.Fatalf("extractParallelIdentityCandidates returned error: %v", err)
	}
	if len(candidates) != 2 || candidates[0].Character.CanonicalName != "アリス" || candidates[1].Character.CanonicalName != "ボブ" {
		t.Fatalf("candidates = %+v", candidates)
	}
	if len(proposals) != 0 {
		t.Fatalf("merge proposals = %+v, want none", proposals)
	}
	if len(usage) != 2 || usage[0].RequestIndex != 0 || usage[1].RequestIndex != 1 {
		t.Fatalf("usage = %+v", usage)
	}
	if len(unresolved) != 0 {
		t.Fatalf("unresolved = %+v", unresolved)
	}
	if len(rawTerms) != 2 || rawTerms[0].Term != "魔導院" || rawTerms[1].Term != "白銀騎士団" {
		t.Fatalf("rawTerms = %+v", rawTerms)
	}
	completedCounts := []int{}
	mergedCharacterCounts := []int{}
	mergedTermCounts := []int{}
	for _, progress := range progressEvents {
		if progress.WorkerIndex != 1 {
			t.Fatalf("single-concurrency progress should use worker 1: %+v", progressEvents)
		}
		if progress.Phase == "complete" {
			completedCounts = append(completedCounts, progress.CompletedBatchCount)
			mergedCharacterCounts = append(mergedCharacterCounts, progress.MergedCharacterCount)
			mergedTermCounts = append(mergedTermCounts, progress.MergedTermCount)
		}
	}
	if len(completedCounts) != 2 || completedCounts[0] != 1 || completedCounts[1] != 2 {
		t.Fatalf("completedCounts = %+v events=%+v", completedCounts, progressEvents)
	}
	if len(mergedCharacterCounts) != 2 || mergedCharacterCounts[0] != 1 || mergedCharacterCounts[1] != 2 {
		t.Fatalf("mergedCharacterCounts = %+v events=%+v", mergedCharacterCounts, progressEvents)
	}
	if len(mergedTermCounts) != 2 || mergedTermCounts[0] != 1 || mergedTermCounts[1] != 2 {
		t.Fatalf("mergedTermCounts = %+v events=%+v", mergedTermCounts, progressEvents)
	}
}

func TestExtractParallelIdentityCandidatesReturnsUsageFromSuccessfulAndNormalizationFailedRequests(t *testing.T) {
	openrouter := newExtractionOpenRouterTestServer(
		t,
		`{"processedUpToEpisodeIndex":"1","newCharacters":[],"characterUpdates":[],"mergeProposals":[],"unresolvedMentions":[],"terms":[]}`,
		`not-json`,
		`not-json`,
	)
	defer openrouter.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", openrouter.URL)
	t.Setenv("EXTRACTION_LLM_START_INTERVAL_MS", "0")
	batches := []extractionBatch{
		{BatchIndex: 1, BatchCount: 2, EpisodeIndexes: []string{"1"}, Chunks: []extractionChunk{{EpisodeIndex: "1", Text: "本文1"}}},
		{BatchIndex: 2, BatchCount: 2, EpisodeIndexes: []string{"2"}, Chunks: []extractionChunk{{EpisodeIndex: "2", Text: "本文2"}}},
	}
	runtime := NewRuntime(RuntimeDependencies{StateDir: t.TempDir()})
	_, _, _, usage, _, err := runtime.extractParallelIdentityCandidates(context.Background(), &store.ResolvedAIGenerationConfig{APIKey: "sk-test", ModelID: "model", ExtractionParallelConcurrency: 1}, "novel-1", "2", nil, batches, nil, nil)
	if err == nil {
		t.Fatal("normalization failure should be returned")
	}
	if len(usage) != 2 || usage[0].TotalTokens == 0 || usage[1].TotalTokens == 0 {
		t.Fatalf("provider usage was lost on partial failure: %+v", usage)
	}
}

func TestExtractParallelIdentityCandidatesDoesNotSendFutureCandidatesToEarlierBatch(t *testing.T) {
	requestBodies := make([]string, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request: %v", err)
		}
		requestBodies = append(requestBodies, string(body))
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": `{"processedUpToEpisodeIndex":"1","newCharacters":[],"characterUpdates":[],"mergeProposals":[],"unresolvedMentions":[],"terms":[]}`}}},
			"usage":   map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
		})
	}))
	defer server.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", server.URL)
	t.Setenv("EXTRACTION_LLM_START_INTERVAL_MS", "0")

	known := []characters.GeneratedCharacter{
		{CharacterID: "char_early", CanonicalName: "アリス", CanonicalEpisodeIndex: "1", FirstAppearanceEpisodeIndex: "1", NameHistory: []characters.GeneratedTextVersion{{Text: "アリス", EpisodeIndex: "1"}}, SummaryHistory: []characters.GeneratedHistoryVersion{{Text: "旅人。", EpisodeIndex: "1"}}},
		{CharacterID: "char_future", CanonicalName: "王女セリア", CanonicalEpisodeIndex: "20", FirstAppearanceEpisodeIndex: "20", NameHistory: []characters.GeneratedTextVersion{{Text: "王女セリア", EpisodeIndex: "20"}}, SummaryHistory: []characters.GeneratedHistoryVersion{{Text: "正体を明かした王女。", EpisodeIndex: "20"}}},
	}
	batches := []extractionBatch{
		{BatchIndex: 1, BatchCount: 2, EpisodeIndexes: []string{"1"}, Chunks: []extractionChunk{{EpisodeIndex: "1", Text: "アリスが歩いた。"}}},
		{BatchIndex: 2, BatchCount: 2, EpisodeIndexes: []string{"20"}, Chunks: []extractionChunk{{EpisodeIndex: "20", Text: "王女セリアが名乗った。"}}},
	}
	runtime := NewRuntime(RuntimeDependencies{StateDir: t.TempDir()})
	_, _, _, _, _, err := runtime.extractParallelIdentityCandidatesWithKnown(context.Background(), &store.ResolvedAIGenerationConfig{APIKey: "sk-test", ModelID: "test-model", ExtractionParallelConcurrency: 1}, "novel-1", "20", known, nil, batches, nil, nil)
	if err != nil {
		t.Fatalf("extract candidates: %v", err)
	}
	if len(requestBodies) != 2 {
		t.Fatalf("request count = %d", len(requestBodies))
	}
	if strings.Contains(requestBodies[0], "王女セリア") || strings.Contains(requestBodies[0], "正体を明かした王女") {
		t.Fatalf("future candidate leaked into episode 1 request: %s", requestBodies[0])
	}
	if !strings.Contains(requestBodies[1], "王女セリア") {
		t.Fatalf("episode 20 candidate missing from its request: %s", requestBodies[1])
	}
}

func TestParallelEntitiesContextFitUsesFocusedPreparedPrompt(t *testing.T) {
	config := &store.ResolvedAIGenerationConfig{}
	batch := extractionBatch{EpisodeIndexes: []string{"1"}, Chunks: []extractionChunk{{EpisodeIndex: "1", Text: "本文"}}}
	plain := prepareExtractionRequest(config, "novel-1", "1", nil, nil, batch, nil)
	focused := prepareExtractionRequest(config, "novel-1", "1", nil, nil, batch, nil, "parallel_entities")
	if focused.PromptTokens <= plain.PromptTokens || !strings.Contains(fmt.Sprint(focused.Messages[0].Content), "人物と用語を同じレスポンス") {
		t.Fatalf("focused planner prompt must match parallel request: plain=%d focused=%d prompt=%q", plain.PromptTokens, focused.PromptTokens, focused.Messages[0].Content)
	}
}

func TestParallelIdentityExtractsCharactersAndTermsInOnePass(t *testing.T) {
	requestIndex := 0
	openrouter := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		raw, _ := json.Marshal(request["messages"])
		if !strings.Contains(string(raw), "人物と用語を同じレスポンス") {
			t.Fatalf("parallel request must extract both entity types: %s", raw)
		}
		responses := []string{
			`{"processedUpToEpisodeIndex":"1","characters":[],"terms":[{"term":"白銀騎士団","reading":null,"category":{"value":"organization","episodeIndex":"1"},"descriptionHistory":[{"text":"王都直属の騎士団。","episodeIndex":"1"}]}]}`,
			`{"processedUpToEpisodeIndex":"2","characters":[],"terms":[{"term":"白銀騎士団","reading":null,"category":{"value":"organization","episodeIndex":"2"},"descriptionHistory":[{"text":"辺境の村へ派遣された。","episodeIndex":"2"}]}]}`,
		}
		content := responses[requestIndex]
		requestIndex++
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": content}}},
			"usage":   map[string]any{"prompt_tokens": 11, "completion_tokens": 7, "total_tokens": 18},
		})
	}))
	defer openrouter.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", openrouter.URL)
	t.Setenv("EXTRACTION_LLM_START_INTERVAL_MS", "0")

	runtime := NewRuntime(RuntimeDependencies{StateDir: t.TempDir()})
	batches := []extractionBatch{
		{BatchIndex: 1, BatchCount: 2, EpisodeIndexes: []string{"1"}, Chunks: []extractionChunk{{EpisodeIndex: "1", Text: "白銀騎士団は王都直属の騎士団。"}}},
		{BatchIndex: 2, BatchCount: 2, EpisodeIndexes: []string{"2"}, Chunks: []extractionChunk{{EpisodeIndex: "2", Text: "白銀騎士団が辺境の村へ派遣された。"}}},
	}
	_, state, usage, err := runtime.generateOpenRouterExtractionParallelIdentity(
		context.Background(),
		&store.ResolvedAIGenerationConfig{APIKey: "sk-test", ModelID: "openai/gpt-5.4-mini", AllowFallbacks: true, ExtractionParallelConcurrency: 1},
		"novel-1",
		"2",
		nil,
		nil,
		batches,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("generateOpenRouterExtractionParallelIdentity returned error: %v", err)
	}
	if requestIndex != 2 || len(usage) != 2 {
		t.Fatalf("requests=%d usage=%+v", requestIndex, usage)
	}
	projected := terms.ProjectTerms(state.Terms, "2")
	if len(state.Terms) != 1 || len(state.Terms[0].DescriptionFacts) != 2 || len(state.Terms[0].DescriptionHistory) != 0 || len(projected) != 1 || projected[0].Description != "王都直属の騎士団。 辺境の村へ派遣された。" {
		t.Fatalf("parallel term facts must stay compact and project cumulatively: state=%+v projected=%+v", state.Terms, projected)
	}
}

func TestParallelIdentityAppliesTrustedMergeProposalsAfterResolver(t *testing.T) {
	for _, testCase := range []struct {
		name           string
		maxReduceItems string
		wantCalls      int
	}{
		{name: "one shot", maxReduceItems: "100", wantCalls: 2},
		{name: "name group fallback", maxReduceItems: "1", wantCalls: 1},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			call := 0
			detailPrompt := ""
			openrouter := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				call++
				var request map[string]any
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Fatalf("decode OpenRouter request: %v", err)
				}
				content := `{"clusters":[]}`
				if call == 1 {
					rawMessages, _ := json.Marshal(request["messages"])
					detailPrompt = string(rawMessages)
					content = `{"processedUpToEpisodeIndex":"2","newCharacters":[],"characterUpdates":[],"mergeProposals":[{"sourceCharacterId":"char_black_knight","targetCharacterId":"char_alice","confidence":1,"reason":"本文で同一人物と明示された"}],"unresolvedMentions":[],"terms":[]}`
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"choices": []map[string]any{{"message": map[string]any{"content": content}}},
					"usage":   map[string]any{"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15},
				})
			}))
			defer openrouter.Close()
			t.Setenv("OPENROUTER_API_BASE_URL", openrouter.URL)
			t.Setenv("EXTRACTION_LLM_START_INTERVAL_MS", "0")
			t.Setenv("EXTRACTION_PARALLEL_MAX_REDUCE_ITEMS", testCase.maxReduceItems)

			seed := []characters.GeneratedCharacter{
				{CharacterID: "char_black_knight", CanonicalName: "黒騎士", CanonicalEpisodeIndex: "1", FirstAppearanceEpisodeIndex: "1"},
				{CharacterID: "char_alice", CanonicalName: "アリス", CanonicalEpisodeIndex: "2", FirstAppearanceEpisodeIndex: "2"},
			}
			batch := extractionBatch{BatchIndex: 1, BatchCount: 1, EpisodeIndexes: []string{"2"}, Chunks: []extractionChunk{{EpisodeIndex: "2", Text: "黒騎士の正体はアリスだった。"}}}
			runtime := NewRuntime(RuntimeDependencies{StateDir: t.TempDir()})
			generated, state, _, err := runtime.generateOpenRouterExtractionParallelIdentity(
				context.Background(),
				&store.ResolvedAIGenerationConfig{APIKey: "sk-test", ModelID: "model", ExtractionParallelConcurrency: 1},
				"novel-1",
				"2",
				seed,
				nil,
				[]extractionBatch{batch},
				nil,
				nil,
			)
			if err != nil {
				t.Fatalf("generateOpenRouterExtractionParallelIdentity returned error: %v", err)
			}
			if call != testCase.wantCalls {
				t.Fatalf("OpenRouter calls = %d, want %d", call, testCase.wantCalls)
			}
			if !strings.Contains(detailPrompt, "char_black_knight") || !strings.Contains(detailPrompt, "char_alice") {
				t.Fatalf("detail prompt did not include stable candidate IDs: %s", detailPrompt)
			}
			if len(generated) != 1 || len(state.PersistenceCharacters) != 2 || len(state.IdentityMergeEvents) != 1 || state.IdentityMergeEvents[0].EffectiveEpisodeIndex != "2" || len(state.RetiredCharacterIDs) != 0 {
				t.Fatalf("trusted proposal should merge unrelated names: generated=%+v state=%+v", generated, state)
			}
		})
	}
}

func TestParallelIdentityResolverMergeIsEffectiveAtGenerationBoundary(t *testing.T) {
	candidates := []parallelIdentityCandidate{
		{LocalID: "seed:black", Character: characters.GeneratedCharacter{CharacterID: "char_black", CanonicalName: "黒騎士", CanonicalEpisodeIndex: "1", FirstAppearanceEpisodeIndex: "1"}},
		{LocalID: "seed:alice", Character: characters.GeneratedCharacter{CharacterID: "char_alice", CanonicalName: "アリス", CanonicalEpisodeIndex: "2", FirstAppearanceEpisodeIndex: "2"}},
	}
	clusters := []parallelIdentityCluster{{LocalIDs: []string{"seed:black", "seed:alice"}, Confidence: 1}}
	generated, persistence, events := buildParallelIdentityGeneratedState(candidates, clusters, nil, characters.NewGeneratedCharacterIDAllocator("novel-1", nil), "20")
	if len(generated) != 1 || len(persistence) != 2 || len(events) != 1 || events[0].EffectiveEpisodeIndex != "20" {
		t.Fatalf("resolver identity state = generated:%+v persistence:%+v events:%+v", generated, persistence, events)
	}
	if before := core.ApplyIdentityMergeEvents(persistence, events, "2"); len(before) != 2 {
		t.Fatalf("resolver identity merged before effective episode: %+v", before)
	}
	if after := core.ApplyIdentityMergeEvents(persistence, events, "20"); len(after) != 1 {
		t.Fatalf("resolver identity did not merge at effective episode: %+v", after)
	}
}

func TestRunParallelIdentityLLMJobsLimitsConcurrencyAndStaggersStarts(t *testing.T) {
	t.Setenv("EXTRACTION_LLM_START_INTERVAL_MS", "20")

	var mu sync.Mutex
	active := 0
	maxActive := 0
	starts := []time.Time{}
	err := runParallelIdentityLLMJobs(context.Background(), 4, 2, func(ctx context.Context, _ int) error {
		mu.Lock()
		active++
		if active > maxActive {
			maxActive = active
		}
		starts = append(starts, time.Now())
		mu.Unlock()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(35 * time.Millisecond):
		}

		mu.Lock()
		active--
		mu.Unlock()
		return nil
	})
	if err != nil {
		t.Fatalf("runParallelIdentityLLMJobs returned error: %v", err)
	}
	if maxActive > 2 {
		t.Fatalf("max active jobs = %d, want <= 2", maxActive)
	}
	if len(starts) != 4 {
		t.Fatalf("starts = %d, want 4", len(starts))
	}
	sort.Slice(starts, func(i, j int) bool { return starts[i].Before(starts[j]) })
	for index := 1; index < len(starts); index++ {
		if delta := starts[index].Sub(starts[index-1]); delta < 15*time.Millisecond {
			t.Fatalf("job starts were not staggered: starts=%+v delta=%s", starts, delta)
		}
	}
}

func TestParallelIdentityLLMConfig(t *testing.T) {
	if got := parallelIdentityLLMConcurrency(&store.ResolvedAIGenerationConfig{ExtractionParallelConcurrency: 99}); got != maxParallelIdentityLLMConcurrency {
		t.Fatalf("concurrency should be capped: got %d", got)
	}
	if got := parallelIdentityLLMConcurrency(nil); got != defaultParallelIdentityLLMConcurrency {
		t.Fatalf("missing concurrency should use default: got %d", got)
	}

	t.Setenv("EXTRACTION_LLM_START_INTERVAL_MS", "0")
	if got := parallelIdentityLLMStartInterval(); got != 0 {
		t.Fatalf("start interval should allow zero: got %s", got)
	}
	t.Setenv("EXTRACTION_LLM_START_INTERVAL_MS", "-1")
	if got := parallelIdentityLLMStartInterval(); got != time.Duration(defaultParallelIdentityLLMStartIntervalMS)*time.Millisecond {
		t.Fatalf("negative start interval should use default: got %s", got)
	}
	t.Setenv("EXTRACTION_PARALLEL_MAX_REDUCE_ITEMS", "12")
	if got := parallelIdentityMaxReduceItems(); got != 12 {
		t.Fatalf("max reduce items = %d, want 12", got)
	}
	t.Setenv("EXTRACTION_PARALLEL_MAX_REDUCE_TOKENS", "bad")
	if got := parallelIdentityMaxReduceTokens(); got != defaultParallelIdentityMaxReduceTokens {
		t.Fatalf("invalid max reduce tokens should use default: got %d", got)
	}
}

func TestParallelIdentityLLMStartLimiterStopsWaitingOnContextCancel(t *testing.T) {
	limiter := newParallelIdentityLLMStartLimiter(time.Hour)
	if err := limiter.Wait(context.Background()); err != nil {
		t.Fatalf("first wait returned error: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := limiter.Wait(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled wait error = %v, want context.Canceled", err)
	}
}

func TestRunParallelIdentityLLMJobsCancelsAfterFirstError(t *testing.T) {
	t.Setenv("EXTRACTION_LLM_START_INTERVAL_MS", "0")

	expected := errors.New("first request failed")
	var started int32
	err := runParallelIdentityLLMJobs(context.Background(), 3, 1, func(_ context.Context, index int) error {
		atomic.AddInt32(&started, 1)
		if index == 0 {
			return expected
		}
		return nil
	})
	if !errors.Is(err, expected) {
		t.Fatalf("err = %v, want %v", err, expected)
	}
	if got := atomic.LoadInt32(&started); got != 1 {
		t.Fatalf("started jobs = %d, want 1", got)
	}
}

func TestParallelIdentityCandidatesFromDeltaWithKnownKeepsUpdatedKnownID(t *testing.T) {
	known := []characters.GeneratedCharacter{{
		CharacterID:                 "char_known",
		CanonicalName:               "アリス",
		CanonicalEpisodeIndex:       "1",
		FirstAppearanceEpisodeIndex: "1",
	}}
	batch := extractionBatch{BatchIndex: 1, Chunks: []extractionChunk{{EpisodeIndex: "2", Text: "アリスが笑った。"}}}
	candidates := parallelIdentityCandidatesFromDeltaWithKnown("novel-1", 0, batch, extractionDelta{
		CharacterUpdates: []characters.GeneratedCharacter{{
			CharacterID:    "char_known",
			CanonicalName:  "アリス",
			SummaryHistory: []characters.GeneratedHistoryVersion{{EpisodeIndex: "2", Text: "笑顔を見せた。"}},
		}},
	}, known)
	if len(candidates) != 1 || candidates[0].Character.CharacterID != "char_known" {
		t.Fatalf("candidates = %+v", candidates)
	}
}

func TestParallelIdentityCandidateHelpers(t *testing.T) {
	fullName := "山田太郎"
	gender := "male"
	batch := extractionBatch{BatchIndex: 2, Chunks: []extractionChunk{{EpisodeIndex: "3", Text: "太郎が来た。"}}}
	candidates := parallelIdentityCandidatesFromDeltaWithKnown("novel-1", 1, batch, extractionDelta{
		NewCharacters: []characters.GeneratedCharacter{{
			CanonicalName:               "太郎",
			CanonicalEpisodeIndex:       "3",
			FirstAppearanceEpisodeIndex: "3",
			FullName:                    &fullName,
			FullNameEpisodeIndex:        "3",
			Gender:                      &gender,
			GenderEpisodeIndex:          "3",
			Aliases:                     []characters.GeneratedTextVersion{{Text: "タロウ", EpisodeIndex: "3"}},
			SummaryHistory:              []characters.GeneratedHistoryVersion{{EpisodeIndex: "3", Text: "主人公の友人。"}},
			AppearanceHistory:           []characters.GeneratedHistoryVersion{{EpisodeIndex: "3", Text: "背が高い。"}},
			PersonalityHistory:          []characters.GeneratedHistoryVersion{{EpisodeIndex: "3", Text: "穏やか。"}},
		}},
	}, nil)
	if len(candidates) != 1 || candidates[0].LocalID != "b2-c1" || candidates[0].Character.CharacterID != "" {
		t.Fatalf("candidates = %+v", candidates)
	}
	cards := parallelIdentityCandidateCards(candidates)
	if len(cards) != 1 || cards[0]["localId"] != "b2-c1" || cards[0]["batchIndex"] != 2 || cards[0]["fullName"] != fullName || cards[0]["gender"] != gender {
		t.Fatalf("cards = %+v", cards)
	}
	aliases, ok := cards[0]["aliases"].([]string)
	if !ok || len(aliases) != 1 || aliases[0] != "タロウ" {
		t.Fatalf("aliases = %#v", cards[0]["aliases"])
	}
}

func TestParallelIdentityCompletesSingletonsAndNormalizesIDs(t *testing.T) {
	candidates := []parallelIdentityCandidate{
		{LocalID: "b1-c1", Character: characters.GeneratedCharacter{CanonicalName: "アリス"}},
		{LocalID: "b2-c1", Character: characters.GeneratedCharacter{CanonicalName: "アリス"}},
		{LocalID: "b3-c1", Character: characters.GeneratedCharacter{CanonicalName: "ボブ"}},
	}
	clusters := completeParallelIdentitySingletons([]parallelIdentityCluster{{
		LocalIDs:      []string{"b2-c1", "b1-c1", "b1-c1", ""},
		CanonicalName: "アリス",
		Confidence:    0.9,
	}, {
		LocalIDs:      []string{"b1-c1", "b3-c1"},
		CanonicalName: "ボブ",
	}}, candidates)
	if len(clusters) != 2 {
		t.Fatalf("clusters = %+v", clusters)
	}
	if got := clusters[0].LocalIDs; len(got) != 2 || got[0] != "b1-c1" || got[1] != "b2-c1" {
		t.Fatalf("normalized ids = %+v", got)
	}
	if clusters[1].LocalIDs[0] != "b3-c1" || clusters[1].CanonicalName != "ボブ" {
		t.Fatalf("singleton cluster = %+v", clusters[1])
	}
}

func TestParallelIdentityBuildGeneratedStatePreservesTimedSources(t *testing.T) {
	seed := []characters.GeneratedCharacter{{
		CharacterID:                 "char_old",
		CanonicalName:               "アリス",
		CanonicalEpisodeIndex:       "1",
		FirstAppearanceEpisodeIndex: "1",
		SummaryHistory:              []characters.GeneratedHistoryVersion{{EpisodeIndex: "1", Text: "古い情報。"}},
	}, {
		CharacterID:                 "char_newer",
		CanonicalName:               "アリス姫",
		CanonicalEpisodeIndex:       "2",
		FirstAppearanceEpisodeIndex: "2",
		SummaryHistory:              []characters.GeneratedHistoryVersion{{EpisodeIndex: "2", Text: "姫だと判明。"}},
	}}
	allocator := characters.NewGeneratedCharacterIDAllocator("novel-1", seed)
	candidates := seedParallelIdentityCandidates(seed)
	candidates = append(candidates, parallelIdentityCandidate{
		LocalID: "b1-c1",
		Character: characters.GeneratedCharacter{
			CanonicalName:               "アリス",
			CanonicalEpisodeIndex:       "3",
			FirstAppearanceEpisodeIndex: "3",
			Aliases:                     []characters.GeneratedTextVersion{{Text: "姫様", EpisodeIndex: "3"}},
		},
	})
	generated, persistence, events := buildParallelIdentityGeneratedState(candidates, []parallelIdentityCluster{{
		LocalIDs:      []string{"seed:char_old", "seed:char_newer", "b1-c1"},
		CanonicalName: "アリス",
		Confidence:    0.96,
	}}, nil, allocator, "3")
	if len(generated) != 1 || generated[0].CharacterID != "char_old" {
		t.Fatalf("generated = %+v", generated)
	}
	if len(generated[0].SummaryHistory) != 2 {
		t.Fatalf("summary histories should merge: %+v", generated[0].SummaryHistory)
	}
	if len(persistence) != 3 || len(events) != 2 || len(allocator.RetiredCharacterIDs()) != 0 {
		t.Fatalf("timed identity state = persistence:%+v events:%+v retired:%+v", persistence, events, allocator.RetiredCharacterIDs())
	}
}

func TestParallelIdentityBuildGeneratedStateKeepsRawSeedBehindExistingMergeEvent(t *testing.T) {
	rawSeed := []characters.GeneratedCharacter{
		{CharacterID: "char_black_knight", CanonicalName: "黒騎士", CanonicalEpisodeIndex: "1", FirstAppearanceEpisodeIndex: "1", Aliases: []characters.GeneratedTextVersion{{Text: "黒騎士", EpisodeIndex: "1"}}},
		{CharacterID: "char_alice", CanonicalName: "アリス", CanonicalEpisodeIndex: "2", FirstAppearanceEpisodeIndex: "2", Aliases: []characters.GeneratedTextVersion{{Text: "アリス", EpisodeIndex: "2"}}},
	}
	events := []characters.GeneratedIdentityMergeEvent{{SourceCharacterID: "char_black_knight", TargetCharacterID: "char_alice", EffectiveEpisodeIndex: "20"}}
	visibleSeed := core.ApplyIdentityMergeEvents(rawSeed, events, "21")
	generated, persistence, persistedEvents := buildParallelIdentityGeneratedState(
		seedParallelIdentityCandidates(visibleSeed),
		nil,
		nil,
		characters.NewGeneratedCharacterIDAllocator("novel-1", rawSeed),
		"21",
		parallelIdentityInitialState{PersistenceCharacters: rawSeed, IdentityMergeEvents: events},
	)
	if len(generated) != 1 || len(persistence) != 2 || len(persistedEvents) != 1 {
		t.Fatalf("generated=%+v persistence=%+v events=%+v", generated, persistence, persistedEvents)
	}
	for _, character := range persistence {
		if len(character.Aliases) != 1 {
			t.Fatalf("visible merged seed leaked into raw persistence: %+v", persistence)
		}
	}
}

func TestParallelCorrectionPersistsAliasPromotedToCanonicalName(t *testing.T) {
	raw := []characters.GeneratedCharacter{{
		CharacterID:                 "char_alice",
		CanonicalName:               "謎の少女",
		CanonicalEpisodeIndex:       "1",
		FirstAppearanceEpisodeIndex: "1",
		NameHistory:                 []characters.GeneratedTextVersion{{Text: "謎の少女", EpisodeIndex: "1"}},
		Aliases:                     []characters.GeneratedTextVersion{{Text: "アリス", EpisodeIndex: "20"}},
	}}
	current := append([]characters.GeneratedCharacter{}, raw...)
	current[0].CanonicalName = "アリス"
	current[0].CanonicalEpisodeIndex = "20"
	persisted := applyParallelIdentityCurrentProjectionToPersistence(raw, raw, current, nil, "20")
	if len(persisted) != 1 || persisted[0].CanonicalName != "アリス" || persisted[0].CanonicalEpisodeIndex != "20" || generatedCharacterNameEpisodeIndex(persisted[0], "アリス") != "20" {
		t.Fatalf("promoted alias was not persisted as canonical: %+v", persisted)
	}
	atOne := projectGeneratedCharactersAtBoundary(persisted, "1")
	if len(atOne) != 1 || atOne[0].CanonicalName != "謎の少女" {
		t.Fatalf("future canonical promotion leaked into episode 1: %+v", atOne)
	}
	atTwenty := projectGeneratedCharactersAtBoundary(persisted, "20")
	if len(atTwenty) != 1 || atTwenty[0].CanonicalName != "アリス" {
		t.Fatalf("canonical promotion was not reproducible at episode 20: %+v", atTwenty)
	}
	stateDir := t.TempDir()
	if err := characters.SaveGeneratedSummary(stateDir, "novel-1", "20", persisted); err != nil {
		t.Fatalf("save promoted canonical: %v", err)
	}
	reloaded, _, ok, err := characters.LoadGeneratedCharacters(stateDir, "novel-1")
	if err != nil || !ok {
		t.Fatalf("reload promoted canonical: ok=%v err=%v", ok, err)
	}
	reloadedAtOne := projectGeneratedCharactersAtBoundary(reloaded, "1")
	reloadedAtTwenty := projectGeneratedCharactersAtBoundary(reloaded, "20")
	if len(reloadedAtOne) != 1 || reloadedAtOne[0].CanonicalName != "謎の少女" || len(reloadedAtTwenty) != 1 || reloadedAtTwenty[0].CanonicalName != "アリス" {
		t.Fatalf("canonical promotion changed across save/load: at1=%+v at20=%+v", reloadedAtOne, reloadedAtTwenty)
	}
}

func TestParallelIdentityUpdateDoesNotWriteVisibleMergedHistoryIntoRawSeed(t *testing.T) {
	rawSeed := []characters.GeneratedCharacter{
		{CharacterID: "char_black_knight", CanonicalName: "黒騎士", CanonicalEpisodeIndex: "1", FirstAppearanceEpisodeIndex: "1", Aliases: []characters.GeneratedTextVersion{{Text: "黒騎士", EpisodeIndex: "1"}}, SummaryHistory: []characters.GeneratedHistoryVersion{{EpisodeIndex: "1", Text: "正体不明。"}}},
		{CharacterID: "char_alice", CanonicalName: "アリス", CanonicalEpisodeIndex: "2", FirstAppearanceEpisodeIndex: "2", Aliases: []characters.GeneratedTextVersion{{Text: "アリス", EpisodeIndex: "2"}}},
	}
	events := []characters.GeneratedIdentityMergeEvent{{SourceCharacterID: "char_black_knight", TargetCharacterID: "char_alice", EffectiveEpisodeIndex: "20"}}
	visibleSeed := core.ApplyIdentityMergeEvents(rawSeed, events, "21")
	updateCandidates := parallelIdentityCandidatesFromDeltaWithKnown("novel-1", 0, extractionBatch{BatchIndex: 1}, extractionDelta{
		CharacterUpdates: []characters.GeneratedCharacter{{CharacterID: "char_alice", SummaryHistory: []characters.GeneratedHistoryVersion{{EpisodeIndex: "21", Text: "王都へ戻った。"}}}},
	}, visibleSeed)
	generated, persistence, persistedEvents := buildParallelIdentityGeneratedState(
		append(seedParallelIdentityCandidates(visibleSeed), updateCandidates...),
		nil,
		nil,
		characters.NewGeneratedCharacterIDAllocator("novel-1", rawSeed),
		"21",
		parallelIdentityInitialState{PersistenceCharacters: rawSeed, IdentityMergeEvents: events},
	)
	if len(generated) != 1 || len(persistence) != 2 || len(persistedEvents) != 1 {
		t.Fatalf("generated=%+v persistence=%+v events=%+v", generated, persistence, persistedEvents)
	}
	persistenceByID := generatedCharacterMapByID(persistence)
	alice := persistenceByID["char_alice"]
	if len(alice.Aliases) != 1 || alice.Aliases[0].Text != "アリス" || len(alice.SummaryHistory) != 1 || alice.SummaryHistory[0].EpisodeIndex != "21" {
		t.Fatalf("visible merged source history leaked into raw update: %+v", alice)
	}
}

func TestResolveParallelIdentityClustersSingleCandidateAvoidsOpenRouter(t *testing.T) {
	runtime := NewRuntime(RuntimeDependencies{StateDir: t.TempDir()})
	candidate := parallelIdentityCandidate{LocalID: "b1-c1", Character: characters.GeneratedCharacter{CanonicalName: "ソロ"}}
	clusters, usage, err := runtime.resolveParallelIdentityClusters(context.Background(), &store.ResolvedAIGenerationConfig{}, "novel-1", "1", []parallelIdentityCandidate{candidate})
	if err != nil {
		t.Fatalf("resolveParallelIdentityClusters returned error: %v", err)
	}
	if len(clusters) != 1 || clusters[0].LocalIDs[0] != "b1-c1" || clusters[0].CanonicalName != "ソロ" {
		t.Fatalf("clusters = %+v", clusters)
	}
	if usage.Kind != "" || usage.InputTokens != 0 || usage.OutputTokens != 0 || usage.TotalTokens != 0 {
		t.Fatalf("single candidate should not record resolver usage: %+v", usage)
	}
}

func TestResolveParallelIdentityClustersEmptyAndMissingOpenRouterConfig(t *testing.T) {
	runtime := NewRuntime(RuntimeDependencies{StateDir: t.TempDir()})
	clusters, usage, err := runtime.resolveParallelIdentityClusters(context.Background(), &store.ResolvedAIGenerationConfig{}, "novel-1", "1", nil)
	if err != nil {
		t.Fatalf("empty candidates returned error: %v", err)
	}
	if len(clusters) != 0 || usage.Kind != "" {
		t.Fatalf("clusters=%+v usage=%+v", clusters, usage)
	}

	candidates := []parallelIdentityCandidate{
		{LocalID: "b1-c1", Character: characters.GeneratedCharacter{CanonicalName: "アリス"}},
		{LocalID: "b2-c1", Character: characters.GeneratedCharacter{CanonicalName: "アリス姫"}},
	}
	clusters, usage, err = runtime.resolveParallelIdentityClusters(context.Background(), &store.ResolvedAIGenerationConfig{}, "novel-1", "2", candidates)
	if err == nil || !strings.Contains(err.Error(), "OpenRouter API key and modelId are required") {
		t.Fatalf("err = %v", err)
	}
	if len(clusters) != 0 || usage.Kind != "" {
		t.Fatalf("clusters=%+v usage=%+v", clusters, usage)
	}
}

func TestParallelIdentityReduceGuardsRejectLargeOneShotPayloads(t *testing.T) {
	runtime := NewRuntime(RuntimeDependencies{StateDir: t.TempDir()})
	t.Setenv("EXTRACTION_PARALLEL_MAX_REDUCE_ITEMS", "1")
	candidates := []parallelIdentityCandidate{
		{LocalID: "b1-c1", Character: characters.GeneratedCharacter{CanonicalName: "アリス"}},
		{LocalID: "b2-c1", Character: characters.GeneratedCharacter{CanonicalName: "ボブ"}},
	}
	clusters, usage, err := runtime.resolveParallelIdentityClusters(context.Background(), &store.ResolvedAIGenerationConfig{APIKey: "sk-test", ModelID: "model-a"}, "novel-1", "2", candidates)
	if err != nil {
		t.Fatalf("resolve should fall back to name-grouped singletons: %v", err)
	}
	if len(clusters) != 2 || usage.Kind != "" {
		t.Fatalf("clusters=%+v usage=%+v", clusters, usage)
	}
	generated := []characters.GeneratedCharacter{
		{CharacterID: "char_a", CanonicalName: "アリス"},
		{CharacterID: "char_b", CanonicalName: "ボブ"},
	}
	if _, _, err := runtime.correctParallelIdentityCharactersOneShot(context.Background(), &store.ResolvedAIGenerationConfig{APIKey: "sk-test", ModelID: "model-a"}, "novel-1", "2", generated); err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("correction guard err = %v", err)
	}

	t.Setenv("EXTRACTION_PARALLEL_MAX_REDUCE_ITEMS", "100")
	t.Setenv("EXTRACTION_PARALLEL_MAX_REDUCE_TOKENS", "1")
	if _, _, err := runtime.resolveParallelIdentityClustersOneShot(context.Background(), &store.ResolvedAIGenerationConfig{APIKey: "sk-test", ModelID: "model-a"}, "novel-1", "2", candidates); err == nil || !strings.Contains(err.Error(), "estimated prompt tokens") {
		t.Fatalf("resolve token guard err = %v", err)
	}
}

func TestResolveParallelIdentityClustersFallsBackToNameGroups(t *testing.T) {
	openrouter := newExtractionOpenRouterTestServer(t, `{"clusters":[{"localIds":["b1-c1","b2-c1"],"canonicalName":"アリス","confidence":0.95,"reason":"同じ名前"}]}`)
	defer openrouter.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", openrouter.URL)
	t.Setenv("EXTRACTION_PARALLEL_MAX_REDUCE_ITEMS", "2")

	runtime := NewRuntime(RuntimeDependencies{StateDir: t.TempDir()})
	candidates := []parallelIdentityCandidate{
		{LocalID: "b1-c1", Character: characters.GeneratedCharacter{CanonicalName: "アリス"}},
		{LocalID: "b2-c1", Character: characters.GeneratedCharacter{CanonicalName: "アリス", Aliases: []characters.GeneratedTextVersion{{Text: "姫様"}}}},
		{LocalID: "b3-c1", Character: characters.GeneratedCharacter{CanonicalName: "ボブ"}},
	}
	clusters, usage, err := runtime.resolveParallelIdentityClusters(context.Background(), &store.ResolvedAIGenerationConfig{APIKey: "sk-test", ModelID: "openai/gpt-5.4-mini", AllowFallbacks: true}, "novel-1", "3", candidates)
	if err != nil {
		t.Fatalf("resolveParallelIdentityClusters returned error: %v", err)
	}
	if len(clusters) != 2 {
		t.Fatalf("clusters = %+v", clusters)
	}
	if got := clusters[0].LocalIDs; len(got) != 2 || got[0] != "b1-c1" || got[1] != "b2-c1" {
		t.Fatalf("group cluster ids = %+v", got)
	}
	if clusters[1].LocalIDs[0] != "b3-c1" || usage.Kind != "extraction_identity_resolution" {
		t.Fatalf("clusters=%+v usage=%+v", clusters, usage)
	}
}

func TestResolveParallelIdentityClustersRetriesInvalidJSON(t *testing.T) {
	openrouter := newExtractionOpenRouterTestServer(t, `{"clusters":[{"localIds":["b1-c1","b2-c1"],"canonicalName":"アリス","reason":"同一人物"}]}`, `{"clusters":[]}`)
	defer openrouter.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", openrouter.URL)
	runtime := NewRuntime(RuntimeDependencies{StateDir: t.TempDir()})
	candidates := []parallelIdentityCandidate{
		{LocalID: "b1-c1", Character: characters.GeneratedCharacter{CanonicalName: "アリス"}},
		{LocalID: "b2-c1", Character: characters.GeneratedCharacter{CanonicalName: "ボブ"}},
	}
	clusters, usage, err := runtime.resolveParallelIdentityClustersOneShot(context.Background(), &store.ResolvedAIGenerationConfig{APIKey: "sk-test", ModelID: "model"}, "novel-1", "1", candidates)
	if err != nil || len(clusters) != 2 {
		t.Fatalf("identity resolution should recover from invalid JSON: clusters=%+v err=%v", clusters, err)
	}
	if usage.InputTokens != 22 || usage.OutputTokens != 14 || usage.TotalTokens != 36 {
		t.Fatalf("identity retry usage should include both responses: %+v", usage)
	}
}

func TestResolveParallelIdentityClustersFallbackKeepsSeedIdentityAcrossSplitNameGroup(t *testing.T) {
	openrouter := newExtractionOpenRouterTestServer(
		t,
		`{"clusters":[{"localIds":["seed:char_alice","b1-c1"],"canonicalName":"アリス","confidence":0.96,"reason":"同一人物"}]}`,
		`{"clusters":[{"localIds":["seed:char_alice","b2-c1"],"canonicalName":"アリス","confidence":0.96,"reason":"同一人物"}]}`,
	)
	defer openrouter.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", openrouter.URL)
	t.Setenv("EXTRACTION_PARALLEL_MAX_REDUCE_ITEMS", "2")

	runtime := NewRuntime(RuntimeDependencies{StateDir: t.TempDir()})
	candidates := []parallelIdentityCandidate{
		{LocalID: "seed:char_alice", Character: characters.GeneratedCharacter{CharacterID: "char_alice", CanonicalName: "アリス", CanonicalEpisodeIndex: "1", FirstAppearanceEpisodeIndex: "1"}},
		{LocalID: "b1-c1", Character: characters.GeneratedCharacter{CanonicalName: "アリス", CanonicalEpisodeIndex: "2", FirstAppearanceEpisodeIndex: "2"}},
		{LocalID: "b2-c1", Character: characters.GeneratedCharacter{CanonicalName: "アリス", CanonicalEpisodeIndex: "3", FirstAppearanceEpisodeIndex: "3"}},
	}
	clusters, usage, err := runtime.resolveParallelIdentityClusters(context.Background(), &store.ResolvedAIGenerationConfig{APIKey: "sk-test", ModelID: "openai/gpt-5.4-mini", AllowFallbacks: true}, "novel-1", "3", candidates)
	if err != nil {
		t.Fatalf("resolveParallelIdentityClusters returned error: %v", err)
	}
	if len(clusters) != 1 {
		t.Fatalf("clusters = %+v", clusters)
	}
	if got := clusters[0].LocalIDs; len(got) != 3 || got[0] != "b1-c1" || got[1] != "b2-c1" || got[2] != "seed:char_alice" {
		t.Fatalf("merged cluster ids = %+v", got)
	}
	if usage.Kind != "extraction_identity_resolution" || usage.InputTokens != 22 || usage.OutputTokens != 14 || usage.TotalTokens != 36 {
		t.Fatalf("usage = %+v", usage)
	}

	allocator := characters.NewGeneratedCharacterIDAllocator("novel-1", []characters.GeneratedCharacter{candidates[0].Character})
	generated, _, events := buildParallelIdentityGeneratedState(candidates, clusters, nil, allocator, "3")
	if len(generated) != 1 || generated[0].CharacterID != "char_alice" || generated[0].CanonicalName != "アリス" {
		t.Fatalf("generated = %+v", generated)
	}
	if len(events) != 2 {
		t.Fatalf("identity events = %+v, want two timed sources", events)
	}
}

func TestResolveParallelIdentityClustersFallbackIgnoresHallucinatedLocalIDsBeforeOverlapMerge(t *testing.T) {
	openrouter := newExtractionOpenRouterTestServer(
		t,
		`{"clusters":[{"localIds":["b1-c1","b2-c1"],"canonicalName":"アリス","confidence":0.4,"reason":"b2-c1 is not in this request"}]}`,
		`{"clusters":[{"localIds":["seed:char_alice","b2-c1"],"canonicalName":"アリス","confidence":0.96,"reason":"同一人物"}]}`,
	)
	defer openrouter.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", openrouter.URL)
	t.Setenv("EXTRACTION_PARALLEL_MAX_REDUCE_ITEMS", "2")

	runtime := NewRuntime(RuntimeDependencies{StateDir: t.TempDir()})
	candidates := []parallelIdentityCandidate{
		{LocalID: "seed:char_alice", Character: characters.GeneratedCharacter{CharacterID: "char_alice", CanonicalName: "アリス", CanonicalEpisodeIndex: "1", FirstAppearanceEpisodeIndex: "1"}},
		{LocalID: "b1-c1", Character: characters.GeneratedCharacter{CanonicalName: "アリス", CanonicalEpisodeIndex: "2", FirstAppearanceEpisodeIndex: "2"}},
		{LocalID: "b2-c1", Character: characters.GeneratedCharacter{CanonicalName: "アリス", CanonicalEpisodeIndex: "3", FirstAppearanceEpisodeIndex: "3"}},
	}
	clusters, _, err := runtime.resolveParallelIdentityClusters(context.Background(), &store.ResolvedAIGenerationConfig{APIKey: "sk-test", ModelID: "openai/gpt-5.4-mini", AllowFallbacks: true}, "novel-1", "3", candidates)
	if err != nil {
		t.Fatalf("resolveParallelIdentityClusters returned error: %v", err)
	}
	if len(clusters) != 2 {
		t.Fatalf("clusters = %+v", clusters)
	}
	clusterIDs := []string{strings.Join(clusters[0].LocalIDs, ","), strings.Join(clusters[1].LocalIDs, ",")}
	sort.Strings(clusterIDs)
	if !reflect.DeepEqual(clusterIDs, []string{"b1-c1", "b2-c1,seed:char_alice"}) {
		t.Fatalf("cluster ids = %+v", clusterIDs)
	}
}

func TestFilterAutoApplicableParallelIdentityClustersConfidenceBoundaryAndBridge(t *testing.T) {
	filtered := filterAutoApplicableParallelIdentityClusters([]parallelIdentityCluster{
		{LocalIDs: []string{"a", "b"}, Confidence: 0},
		{LocalIDs: []string{"b", "c"}, Confidence: 0.74},
		{LocalIDs: []string{"a", "d"}, Confidence: 0.75},
		{LocalIDs: []string{"c", "e"}, Confidence: 1.2},
		{LocalIDs: []string{"singleton"}, Confidence: 1},
	})
	if len(filtered) != 2 || filtered[0].Confidence != 0.75 || filtered[1].Confidence != 1 {
		t.Fatalf("filtered = %+v", filtered)
	}
	merged := mergeOverlappingParallelIdentityClusters(filtered)
	if len(merged) != 2 {
		t.Fatalf("low-confidence bridge merged trusted clusters: %+v", merged)
	}
}

func TestLowConfidenceSameNameCandidatesRemainSeparateCharacters(t *testing.T) {
	candidates := []parallelIdentityCandidate{
		{LocalID: "b1-c1", Character: characters.GeneratedCharacter{CanonicalName: "アリス", CanonicalEpisodeIndex: "1", FirstAppearanceEpisodeIndex: "1", SummaryHistory: []characters.GeneratedHistoryVersion{{EpisodeIndex: "1", Text: "王都の商人"}}}},
		{LocalID: "b2-c1", Character: characters.GeneratedCharacter{CanonicalName: "アリス", CanonicalEpisodeIndex: "2", FirstAppearanceEpisodeIndex: "2", SummaryHistory: []characters.GeneratedHistoryVersion{{EpisodeIndex: "2", Text: "辺境の騎士"}}}},
	}
	clusters := completeParallelIdentitySingletons(filterAutoApplicableParallelIdentityClusters([]parallelIdentityCluster{{LocalIDs: []string{"b1-c1", "b2-c1"}, Confidence: 0.5}}), candidates)
	generated, _, events := buildParallelIdentityGeneratedState(candidates, clusters, nil, characters.NewGeneratedCharacterIDAllocator("novel-1", nil), "2")
	if len(generated) != 2 || generated[0].CharacterID == generated[1].CharacterID {
		t.Fatalf("same-name singletons were implicitly merged: %+v", generated)
	}
	if len(events) != 0 {
		t.Fatalf("low-confidence candidates created identity events: %+v", events)
	}
}

func TestParallelIdentityCandidatesKeepSameNameNewCharactersSeparate(t *testing.T) {
	delta := extractionDelta{NewCharacters: []characters.GeneratedCharacter{
		{CanonicalName: "アリス", CanonicalEpisodeIndex: "1", FirstAppearanceEpisodeIndex: "1", SummaryHistory: []characters.GeneratedHistoryVersion{{EpisodeIndex: "1", Text: "商人"}}},
		{CanonicalName: "アリス", CanonicalEpisodeIndex: "1", FirstAppearanceEpisodeIndex: "1", SummaryHistory: []characters.GeneratedHistoryVersion{{EpisodeIndex: "1", Text: "騎士"}}},
	}}
	candidates := parallelIdentityCandidatesFromDeltaWithKnown("novel-1", 0, extractionBatch{BatchIndex: 1}, delta, nil)
	if len(candidates) != 2 {
		t.Fatalf("same-name new characters were collapsed before identity resolution: %+v", candidates)
	}
}

func TestRenumberParallelIdentityRuntimeBatches(t *testing.T) {
	batches := renumberParallelIdentityRuntimeBatches([]extractionBatch{{BatchIndex: 1, BatchCount: 1}, {BatchIndex: 1, BatchCount: 1}})
	if batches[0].BatchIndex != 1 || batches[1].BatchIndex != 2 || batches[0].BatchCount != 2 || batches[1].BatchCount != 2 {
		t.Fatalf("batches = %+v", batches)
	}
	if got := ExtractionJobBatchProgressPercent(1, batches[0].BatchCount); got >= 100 {
		t.Fatalf("intermediate progress = %d", got)
	}
}

func TestParallelIdentityNameGroupingAndUsageHelpers(t *testing.T) {
	fullName := "  アリス  "
	candidates := []parallelIdentityCandidate{
		{LocalID: "a", Character: characters.GeneratedCharacter{CanonicalName: "ア リス", FullName: &fullName}},
		{LocalID: "b", Character: characters.GeneratedCharacter{CanonicalName: "アリス", Aliases: []characters.GeneratedTextVersion{{Text: "姫様"}}}},
		{LocalID: "c", Character: characters.GeneratedCharacter{CanonicalName: "姫様"}},
		{LocalID: "d", Character: characters.GeneratedCharacter{CanonicalName: "ボブ"}},
	}
	groups := parallelIdentityCandidateNameGroups(candidates, 2)
	if len(groups) != 3 || len(groups[0]) != 2 || len(groups[1]) != 1 || len(groups[2]) != 1 {
		t.Fatalf("groups = %+v", groups)
	}
	if key := normalizeParallelIdentityNameKey(" A　B "); key != "ab" {
		t.Fatalf("normalized key = %q", key)
	}
	if key := normalizeParallelIdentityNameKey("A"); key != "a" {
		t.Fatalf("single-rune key should be kept: %q", key)
	}
	if key := normalizeParallelIdentityNameKey(" 　 "); key != "" {
		t.Fatalf("whitespace-only key should be empty: %q", key)
	}
	usage := aggregateParallelIdentityUsage("extraction_identity_resolution", []ai.UsageRequest{
		{Kind: "extraction_identity_resolution", InputTokens: 3, OutputTokens: 4},
		{},
		{Kind: "extraction_identity_resolution", TotalTokens: 10, Cost: 0.2},
	})
	if usage.Kind != "extraction_identity_resolution" || usage.InputTokens != 3 || usage.OutputTokens != 4 || usage.TotalTokens != 10 || usage.Cost != 0.2 {
		t.Fatalf("usage = %+v", usage)
	}
	if empty := aggregateParallelIdentityUsage("extraction_identity_resolution", nil); empty.Kind != "" {
		t.Fatalf("empty usage = %+v", empty)
	}
}

func TestParallelIdentityNameGroupsUseSingleRuneNamesAndFullNameHistory(t *testing.T) {
	candidates := []parallelIdentityCandidate{
		{LocalID: "a", Character: characters.GeneratedCharacter{CanonicalName: "楓"}},
		{LocalID: "b", Character: characters.GeneratedCharacter{CanonicalName: "かえで", Aliases: []characters.GeneratedTextVersion{{Text: "楓"}}}},
		{LocalID: "c", Character: characters.GeneratedCharacter{CanonicalName: "アリス・グレイ", FullNameHistory: []characters.GeneratedTextVersion{{Text: "アリス・ノーブル"}}}},
		{LocalID: "d", Character: characters.GeneratedCharacter{CanonicalName: "アリス・ノーブル"}},
	}
	groups := parallelIdentityCandidateNameGroups(candidates, 4)
	if len(groups) != 2 || len(groups[0]) != 2 || len(groups[1]) != 2 {
		t.Fatalf("groups = %+v", groups)
	}
	if groups[0][0].LocalID != "a" || groups[0][1].LocalID != "b" || groups[1][0].LocalID != "c" || groups[1][1].LocalID != "d" {
		t.Fatalf("groups = %+v", groups)
	}
}

func TestIsParallelIdentityOneShotTooLargeMatchesWrappedSentinel(t *testing.T) {
	guardErr := fmt.Errorf("%w for one-shot correction: 2 characters exceeds limit 1; use serial or reduce target episodes.", errParallelIdentityOneShotTooLarge)
	if !isParallelIdentityOneShotTooLarge(guardErr) {
		t.Fatal("guard error should match sentinel")
	}
	if !isParallelIdentityOneShotTooLarge(fmt.Errorf("wrapped: %w", guardErr)) {
		t.Fatal("wrapped guard error should match sentinel")
	}
	if isParallelIdentityOneShotTooLarge(errors.New("parallel_identity target is too large for one-shot correction")) {
		t.Fatal("text-only error should not match sentinel")
	}
	if isParallelIdentityOneShotTooLarge(nil) {
		t.Fatal("nil error should not match sentinel")
	}
}

func TestCorrectParallelIdentityCharactersKeepsOversizedCharacterUncorrected(t *testing.T) {
	t.Setenv("EXTRACTION_PARALLEL_MAX_REDUCE_TOKENS", "1")

	runtime := NewRuntime(RuntimeDependencies{StateDir: t.TempDir()})
	generated := []characters.GeneratedCharacter{
		{CharacterID: "char_a", CanonicalName: "アリス", CanonicalEpisodeIndex: "1"},
		{CharacterID: "char_b", CanonicalName: "ボブ", CanonicalEpisodeIndex: "2"},
	}
	corrected, usage, err := runtime.correctParallelIdentityCharacters(context.Background(), &store.ResolvedAIGenerationConfig{APIKey: "sk-test", ModelID: "model-a"}, "novel-1", "2", generated)
	if err != nil {
		t.Fatalf("oversized characters should pass through uncorrected: %v", err)
	}
	if len(corrected) != 2 || corrected[0].CharacterID != "char_a" || corrected[1].CharacterID != "char_b" {
		t.Fatalf("corrected = %+v", corrected)
	}
	if usage.Kind != "" {
		t.Fatalf("usage = %+v", usage)
	}
}

func TestCorrectParallelIdentityCharactersFallsBackToChunks(t *testing.T) {
	openrouter := newExtractionOpenRouterTestServer(
		t,
		`{"terms":[],"characters":[{"characterId":"char_a","canonicalName":"アリス姫","aliases":["姫様"],"keep":true,"reason":"代表名を補正"}]}`,
		`{"terms":[],"characters":[{"characterId":"char_b","canonicalName":"ボブ","aliases":["庭師"],"keep":true,"reason":"別名を補足"}]}`,
	)
	defer openrouter.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", openrouter.URL)
	t.Setenv("EXTRACTION_PARALLEL_MAX_REDUCE_ITEMS", "1")

	runtime := NewRuntime(RuntimeDependencies{StateDir: t.TempDir()})
	generated := []characters.GeneratedCharacter{
		{CharacterID: "char_a", CanonicalName: "アリス", CanonicalEpisodeIndex: "1"},
		{CharacterID: "char_b", CanonicalName: "ボブ", CanonicalEpisodeIndex: "2"},
	}
	corrected, usage, err := runtime.correctParallelIdentityCharacters(context.Background(), &store.ResolvedAIGenerationConfig{APIKey: "sk-test", ModelID: "openai/gpt-5.4-mini", AllowFallbacks: true}, "novel-1", "2", generated)
	if err != nil {
		t.Fatalf("correctParallelIdentityCharacters returned error: %v", err)
	}
	if len(corrected) != 2 || corrected[0].CanonicalName != "アリス姫" || len(corrected[1].Aliases) != 1 {
		t.Fatalf("corrected = %+v", corrected)
	}
	if usage.Kind != "extraction_correction" || usage.InputTokens == 0 || usage.OutputTokens == 0 {
		t.Fatalf("usage = %+v", usage)
	}
}

func TestDiscoverParallelIdentityNamesUsesOpenRouter(t *testing.T) {
	openrouter := newExtractionOpenRouterTestServer(t, `{"characters":[{"name":"アリス","aliases":[],"reason":"登場"}]}`, `{"terms":[],"characters":[{"name":"アリス","aliases":["姫"],"episodeIndex":"1","reason":"会話に登場"}]}`)
	defer openrouter.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", openrouter.URL)
	runtime := NewRuntime(RuntimeDependencies{StateDir: t.TempDir()})
	batches := []extractionBatch{{
		BatchIndex:     1,
		BatchCount:     1,
		EpisodeIndexes: []string{"1"},
		Chunks:         []extractionChunk{{EpisodeIndex: "1", Title: "第一話", Text: "アリスが来た。"}},
	}}
	progressEvents := []appextraction.BatchProgress{}
	generated, usage, err := runtime.discoverParallelIdentityNames(context.Background(), &store.ResolvedAIGenerationConfig{APIKey: "sk-test", ModelID: "openai/gpt-5.4-nano", AllowFallbacks: true}, "novel-1", "1", batches, func(progress appextraction.BatchProgress) {
		progressEvents = append(progressEvents, progress)
	})
	if err != nil {
		t.Fatalf("discoverParallelIdentityNames returned error: %v", err)
	}
	if len(generated) != 1 || generated[0].CanonicalName != "アリス" || len(usage) != 1 || usage[0].Kind != "extraction_name_discovery" || usage[0].TotalTokens != 36 {
		t.Fatalf("generated=%+v usage=%+v", generated, usage)
	}
	if len(progressEvents) != 2 || progressEvents[0].Phase != "discoveryStart" || progressEvents[1].Phase != "discoveryComplete" || progressEvents[0].WorkerIndex != 1 || progressEvents[1].CompletedBatchCount != 1 {
		t.Fatalf("unexpected discovery progress: %+v", progressEvents)
	}
}

func TestDiscoverParallelIdentityNamesRejectsEpisodeOutsideBatch(t *testing.T) {
	invalid := `{"characters":[{"name":"王女セリア","aliases":[],"episodeIndex":"1","reason":"第20話で正体を明かした王女"}]}`
	openrouter := newExtractionOpenRouterTestServer(t, invalid, invalid)
	defer openrouter.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", openrouter.URL)
	runtime := NewRuntime(RuntimeDependencies{StateDir: t.TempDir()})
	batch := extractionBatch{EpisodeIndexes: []string{"20"}, Chunks: []extractionChunk{{EpisodeIndex: "20", Text: "王女セリアが名乗った。"}}}
	if _, _, err := runtime.discoverParallelIdentityNamesForBatch(context.Background(), &store.ResolvedAIGenerationConfig{APIKey: "sk-test", ModelID: "model"}, "novel-1", "20", 0, batch); err == nil || !strings.Contains(err.Error(), "outside the current discovery batch") {
		t.Fatalf("out-of-batch discovery episode should fail: %v", err)
	}
}

func TestDiscoverParallelIdentityNamesDefaultsEmptyEpisodeToBatchBoundary(t *testing.T) {
	openrouter := newExtractionOpenRouterTestServer(t, `{"characters":[{"name":"アリス","aliases":[],"episodeIndex":"","reason":"登場"}]}`)
	defer openrouter.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", openrouter.URL)
	runtime := NewRuntime(RuntimeDependencies{StateDir: t.TempDir()})
	batch := extractionBatch{EpisodeIndexes: []string{"1"}, Chunks: []extractionChunk{{EpisodeIndex: "1", Text: "アリスが来た。"}}}
	names, _, err := runtime.discoverParallelIdentityNamesForBatch(context.Background(), &store.ResolvedAIGenerationConfig{APIKey: "sk-test", ModelID: "model"}, "novel-1", "20", 0, batch)
	if err != nil {
		t.Fatalf("empty discovery episode should use batch boundary: %v", err)
	}
	if len(names) != 1 || names[0].EpisodeIndex != "1" {
		t.Fatalf("names = %+v", names)
	}
}

func TestDiscoverParallelIdentityNamesReturnsPartialUsageOnFailure(t *testing.T) {
	openrouter := newExtractionOpenRouterTestServer(t,
		`{"characters":[{"name":"アリス","aliases":[],"episodeIndex":"1","reason":"登場"}]}`,
		`{"characters":[{"name":"セリア","aliases":[],"episodeIndex":"1","reason":"誤った話数"}]}`,
		`{"characters":[{"name":"セリア","aliases":[],"episodeIndex":"1","reason":"誤った話数"}]}`,
	)
	defer openrouter.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", openrouter.URL)
	t.Setenv("EXTRACTION_LLM_START_INTERVAL_MS", "0")
	runtime := NewRuntime(RuntimeDependencies{StateDir: t.TempDir()})
	batches := []extractionBatch{
		{EpisodeIndexes: []string{"1"}, Chunks: []extractionChunk{{EpisodeIndex: "1", Text: "本文1"}}},
		{EpisodeIndexes: []string{"2"}, Chunks: []extractionChunk{{EpisodeIndex: "2", Text: "本文2"}}},
	}
	_, usage, err := runtime.discoverParallelIdentityNames(context.Background(), &store.ResolvedAIGenerationConfig{APIKey: "sk-test", ModelID: "model", ExtractionParallelConcurrency: 1}, "novel-1", "2", batches, nil)
	if err == nil || len(usage) != 2 || usage[0].TotalTokens == 0 || usage[1].TotalTokens == 0 {
		t.Fatalf("discovery usage lost: usage=%+v err=%v", usage, err)
	}
}

func TestNormalizeDiscoveredNamesRejectsNonNumericEpisode(t *testing.T) {
	batch := extractionBatch{EpisodeIndexes: []string{"20"}, Chunks: []extractionChunk{{EpisodeIndex: "20"}}}
	if _, err := normalizeDiscoveredNamesForBatch([]parallelIdentityDiscoveredName{{Name: "王女セリア", EpisodeIndex: "twenty"}}, batch, "20"); err == nil || !strings.Contains(err.Error(), "outside the current discovery batch") {
		t.Fatalf("non-numeric discovery episode should fail: %v", err)
	}
}

func TestCorrectParallelIdentityCharactersUsesOpenRouter(t *testing.T) {
	openrouter := newExtractionOpenRouterTestServer(t, `{"characters":[{"characterId":"char_a","canonicalName":"アリス姫","aliases":[],"keep":true}]}`, `{"terms":[],"characters":[{"characterId":"char_a","canonicalName":"アリス姫","aliases":["姫様"],"keep":true,"reason":"代表名を補正"}]}`)
	defer openrouter.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", openrouter.URL)
	runtime := NewRuntime(RuntimeDependencies{StateDir: t.TempDir()})
	generated := []characters.GeneratedCharacter{{
		CharacterID:                 "char_a",
		CanonicalName:               "アリス",
		CanonicalEpisodeIndex:       "1",
		FirstAppearanceEpisodeIndex: "1",
	}}
	corrected, usage, err := runtime.correctParallelIdentityCharacters(context.Background(), &store.ResolvedAIGenerationConfig{APIKey: "sk-test", ModelID: "openai/gpt-5.4-mini", AllowFallbacks: true}, "novel-1", "1", generated)
	if err != nil {
		t.Fatalf("correctParallelIdentityCharacters returned error: %v", err)
	}
	if len(corrected) != 1 || corrected[0].CanonicalName != "アリス姫" || usage.Kind != "extraction_correction" || usage.TotalTokens != 36 {
		t.Fatalf("corrected=%+v usage=%+v", corrected, usage)
	}
}

func TestIdentityAndCorrectionReturnUsageOnInvalidJSON(t *testing.T) {
	t.Run("identity", func(t *testing.T) {
		openrouter := newExtractionOpenRouterTestServer(t, `not-json`, `not-json`)
		defer openrouter.Close()
		t.Setenv("OPENROUTER_API_BASE_URL", openrouter.URL)
		runtime := NewRuntime(RuntimeDependencies{StateDir: t.TempDir()})
		candidates := []parallelIdentityCandidate{{LocalID: "a", Character: characters.GeneratedCharacter{CanonicalName: "アリス"}}, {LocalID: "b", Character: characters.GeneratedCharacter{CanonicalName: "アリス"}}}
		_, usage, err := runtime.resolveParallelIdentityClustersOneShot(context.Background(), &store.ResolvedAIGenerationConfig{APIKey: "sk-test", ModelID: "model"}, "novel-1", "1", candidates)
		if err == nil || usage.TotalTokens == 0 {
			t.Fatalf("identity usage lost: usage=%+v err=%v", usage, err)
		}
	})
	t.Run("correction", func(t *testing.T) {
		openrouter := newExtractionOpenRouterTestServer(t, `not-json`, `not-json`)
		defer openrouter.Close()
		t.Setenv("OPENROUTER_API_BASE_URL", openrouter.URL)
		runtime := NewRuntime(RuntimeDependencies{StateDir: t.TempDir()})
		_, usage, err := runtime.correctParallelIdentityCharactersOneShot(context.Background(), &store.ResolvedAIGenerationConfig{APIKey: "sk-test", ModelID: "model"}, "novel-1", "1", []characters.GeneratedCharacter{{CharacterID: "char_a", CanonicalName: "アリス"}})
		if err == nil || usage.TotalTokens == 0 {
			t.Fatalf("correction usage lost: usage=%+v err=%v", usage, err)
		}
	})
}

func TestParallelIdentityUsageFallsBackPerOutputAttempt(t *testing.T) {
	usage := parallelIdentityUsageFromChatResult("extraction_identity_resolution", 120, 2, ai.ChatResult{})
	if usage.Kind != "extraction_identity_resolution" || usage.InputTokens != 240 || usage.TotalTokens != 240 {
		t.Fatalf("usage fallback must include every provider attempt: %+v", usage)
	}
}

func TestParallelIdentityRepresentativeIDPrefersEarliestAppearanceThenID(t *testing.T) {
	values := []characters.GeneratedCharacter{
		{CharacterID: "char_b", FirstAppearanceEpisodeIndex: "2"},
		{CharacterID: "char_c", CanonicalEpisodeIndex: "1"},
		{CharacterID: "char_a", FirstAppearanceEpisodeIndex: "1"},
	}
	if got := parallelIdentityGeneratedRepresentativeID(values, []string{"char_b", "char_c", "char_a"}); got != "char_a" {
		t.Fatalf("representative = %q", got)
	}
	if got := parallelIdentityGeneratedRepresentativeID(nil, nil); got != "" {
		t.Fatalf("representative without IDs = %q", got)
	}
}

func TestDiscoveryParallelCorrectionHelpers(t *testing.T) {
	config := &store.ResolvedAIGenerationConfig{ModelID: "base-model", APIKey: "key", ExtractionNameDiscoveryModelID: "name-model"}
	copied := extractionNameDiscoveryConfig(config)
	if copied == config || copied.ModelID != "name-model" || config.ModelID != "base-model" {
		t.Fatalf("copied config = %+v base=%+v", copied, config)
	}
	fallback := extractionNameDiscoveryConfig(&store.ResolvedAIGenerationConfig{ModelID: "base-model", APIKey: "key"})
	if fallback.ModelID != "base-model" {
		t.Fatalf("fallback config = %+v", fallback)
	}
	if extractionNameDiscoveryConfig(nil) != nil {
		t.Fatal("nil config should stay nil")
	}

	discovered := discoveredNamesToGeneratedCharacters([]parallelIdentityDiscoveredName{{
		Name:         " アリス ",
		Aliases:      []string{"アリス", "姫"},
		EpisodeIndex: "2",
		Reason:       "会話に登場",
	}, {
		Name: "アリス",
	}}, "1")
	if len(discovered) != 2 || discovered[0].CanonicalName != "アリス" || len(discovered[0].Aliases) < 2 || len(discovered[0].SummaryHistory) != 1 || discovered[1].CanonicalName != "アリス" {
		t.Fatalf("discovered = %+v", discovered)
	}
	candidates := discoveryParallelIdentityCandidates([]characters.GeneratedCharacter{{CharacterID: "char_d", CanonicalName: "アリス"}})
	if len(candidates) != 1 || candidates[0].LocalID != "d1" || candidates[0].Source != "discovery" {
		t.Fatalf("discovery candidates = %+v", candidates)
	}
	payload := extractionBatchPromptPayload(extractionBatch{BatchIndex: 1, BatchCount: 1, EpisodeIndexes: []string{"1"}, Chunks: []extractionChunk{{EpisodeIndex: "1", Title: "第一話", Text: strings.Repeat("あ", 7000)}}})
	chunks, ok := payload["chunks"].([]map[string]any)
	if !ok || len(chunks) != 1 || len([]rune(chunks[0]["text"].(string))) != 6000 {
		t.Fatalf("payload = %+v", payload)
	}
}

func TestDiscoveredNamesKeepSameRoleCandidatesSeparate(t *testing.T) {
	discovered := discoveredNamesToGeneratedCharacters([]parallelIdentityDiscoveredName{
		{Name: "先生", EpisodeIndex: "3", Reason: "王都の教師"},
		{Name: "先生", EpisodeIndex: "80", Reason: "辺境の教師"},
	}, "80")
	if len(discovered) != 2 || discovered[0].FirstAppearanceEpisodeIndex != "3" || discovered[1].FirstAppearanceEpisodeIndex != "80" {
		t.Fatalf("same-role candidates were merged before identity resolution: %+v", discovered)
	}
}

func TestApplyParallelIdentityCorrections(t *testing.T) {
	remove := false
	generated := []characters.GeneratedCharacter{{
		CharacterID:           "char_a",
		CanonicalName:         "アリス",
		CanonicalEpisodeIndex: "1",
		Aliases:               []characters.GeneratedTextVersion{{Text: "アリス", EpisodeIndex: "1"}},
	}, {
		CharacterID:           "char_b",
		CanonicalName:         "誤検出",
		CanonicalEpisodeIndex: "2",
	}}
	corrected := applyParallelIdentityCorrections(generated, []parallelIdentityCorrection{{
		CharacterID:   "char_a",
		CanonicalName: "アリス姫",
		Aliases:       []string{"姫様"},
		Reason:        "代表名を補正",
	}, {
		CharacterID: "char_b",
		Keep:        &remove,
	}}, "3")
	if len(corrected) != 1 || corrected[0].CanonicalName != "アリス姫" || corrected[0].CanonicalEpisodeIndex != "3" || len(corrected[0].Aliases) != 2 || len(corrected[0].SummaryHistory) != 0 {
		t.Fatalf("corrected = %+v", corrected)
	}
	cards := correctionCharacterCards(corrected)
	if len(cards) != 1 || cards[0]["characterId"] != "char_a" || cards[0]["canonicalName"] != "アリス姫" {
		t.Fatalf("cards = %+v", cards)
	}
}

func TestApplyParallelIdentityCorrectionsPreservesNameTimeline(t *testing.T) {
	generated := []characters.GeneratedCharacter{{
		CharacterID:                 "char_a",
		CanonicalName:               "謎の少女",
		CanonicalEpisodeIndex:       "1",
		FirstAppearanceEpisodeIndex: "1",
		NameHistory: []characters.GeneratedTextVersion{
			{Text: "謎の少女", EpisodeIndex: "1"},
			{Text: "アリス", EpisodeIndex: "20"},
		},
		Aliases: []characters.GeneratedTextVersion{
			{Text: "謎の少女", EpisodeIndex: "1"},
			{Text: "アリス", EpisodeIndex: "20"},
		},
		SummaryHistory: []characters.GeneratedHistoryVersion{{EpisodeIndex: "1", Text: "正体不明の少女。"}},
	}}
	corrected := applyParallelIdentityCorrections(generated, []parallelIdentityCorrection{{
		CharacterID:   "char_a",
		CanonicalName: "アリス",
		Aliases:       []string{"謎の少女", "アリス", "姫"},
		Reason:        "第20話の情報から代表名を補正",
	}}, "20")
	if len(corrected) != 1 || corrected[0].CanonicalEpisodeIndex != "20" || len(corrected[0].SummaryHistory) != 1 {
		t.Fatalf("correction rewrote timeline or persisted reason: %+v", corrected)
	}
	projected := projectGeneratedCharactersAtBoundary(corrected, "1")
	if len(projected) != 1 || projected[0].CanonicalName != "謎の少女" {
		t.Fatalf("episode 1 canonical name leaked: %+v", projected)
	}
	for _, alias := range projected[0].Aliases {
		if alias.Text == "アリス" || alias.Text == "姫" {
			t.Fatalf("future correction alias leaked into episode 1: %+v", projected[0].Aliases)
		}
	}
}

func TestGeneratedTextVersionTextsDeduplicatesAndLimits(t *testing.T) {
	values := []characters.GeneratedTextVersion{{Text: " A "}, {Text: "A"}, {Text: ""}}
	for index := 0; index < 20; index++ {
		values = append(values, characters.GeneratedTextVersion{Text: string(rune('B' + index))})
	}
	texts := generatedTextVersionTexts(values)
	if len(texts) != 12 || texts[0] != "A" {
		t.Fatalf("texts = %+v", texts)
	}
}
