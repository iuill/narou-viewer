package extractionruntime

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"narou-viewer/apps/viewer-api-go/internal/ai"
	"narou-viewer/apps/viewer-api-go/internal/store"
)

func TestParallelIdentityResponseFormatsUseStrictJSONSchema(t *testing.T) {
	tests := []struct {
		name            string
		format          map[string]any
		field           string
		requiredItemKey string
	}{
		{name: "clusters", format: parallelIdentityClusterResponseFormat(), field: "clusters", requiredItemKey: "localIds"},
		{name: "discovery", format: parallelIdentityDiscoveryResponseFormat("16818093084122790426"), field: "characters", requiredItemKey: "episodeIndex"},
		{name: "correction", format: parallelIdentityCorrectionResponseFormat(), field: "characters", requiredItemKey: "keep"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.format["type"] != "json_schema" {
				t.Fatalf("response format must use json_schema: %+v", test.format)
			}
			jsonSchema := test.format["json_schema"].(map[string]any)
			if jsonSchema["strict"] != true {
				t.Fatalf("response schema must be strict: %+v", jsonSchema)
			}
			root := jsonSchema["schema"].(map[string]any)
			if root["additionalProperties"] != false {
				t.Fatalf("root schema must reject extra fields: %+v", root)
			}
			items := root["properties"].(map[string]any)[test.field].(map[string]any)["items"].(map[string]any)
			if items["additionalProperties"] != false {
				t.Fatalf("item schema must reject extra fields: %+v", items)
			}
			required := items["required"].([]any)
			found := false
			for _, key := range required {
				if key == test.requiredItemKey {
					found = true
				}
			}
			if !found {
				t.Fatalf("item schema must require %s: %+v", test.requiredItemKey, required)
			}
			if test.name == "discovery" {
				episodeIndex := items["properties"].(map[string]any)["episodeIndex"].(map[string]any)
				values := episodeIndex["enum"].([]any)
				if len(values) != 1 || values[0] != "16818093084122790426" {
					t.Fatalf("discovery schema must restrict episodeIndex to the current batch: %+v", episodeIndex)
				}
			}
		})
	}
}

func TestGenerateOpenRouterBatchRetriesInvalidModelOutput(t *testing.T) {
	openrouter := newExtractionOpenRouterTestServer(
		t,
		`{"processedUpToEpisodeIndex":"1","newCharacters":[{"canonicalName":{"text":"アリス","episodeIndex":"1"},"fullName":null,"fullNameHistory":[],"gender":null,"genderHistory":[],"firstAppearanceEpisodeIndex":"1","aliases":"invalid","appearanceHistory":[],"personalityHistory":[],"summaryHistory":[]}],"characterUpdates":[],"mergeProposals":[],"unresolvedMentions":[],"terms":[]}`,
		`{"processedUpToEpisodeIndex":"1","newCharacters":[],"characterUpdates":[],"mergeProposals":[],"unresolvedMentions":[],"terms":[]}`,
	)
	defer openrouter.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", openrouter.URL)
	runtime := NewRuntime(RuntimeDependencies{StateDir: t.TempDir()})
	batch := extractionBatch{BatchIndex: 1, BatchCount: 1, EpisodeIndexes: []string{"1"}, Chunks: []extractionChunk{{EpisodeIndex: "1", Text: "本文"}}}

	result, err := runtime.generateOpenRouterBatch(context.Background(), &store.ResolvedAIGenerationConfig{APIKey: "sk-test", ModelID: "model"}, "novel-1", "1", nil, nil, batch)
	if err != nil {
		t.Fatalf("generateOpenRouterBatch should recover from the first invalid response: %v", err)
	}
	if result.Usage.InputTokens != 22 || result.Usage.OutputTokens != 14 || result.Usage.TotalTokens != 36 {
		t.Fatalf("retry usage should include both provider responses: %+v", result.Usage)
	}
}

func TestGenerateOpenRouterBatchNormalizesNumericEpisodeIndexesWithoutPrecisionLoss(t *testing.T) {
	const episodeIndex = "16818093084191348892"
	openrouter := newExtractionOpenRouterTestServer(t,
		`{"processedUpToEpisodeIndex":16818093084191348892,"newCharacters":[],"characterUpdates":[],"mergeProposals":[],"unresolvedMentions":[],"terms":[{"term":"合成用語","reading":null,"category":{"value":"other","episodeIndex":16818093084191348892},"descriptionHistory":[{"text":"合成説明。","episodeIndex":16818093084191348892}]}]}`,
	)
	defer openrouter.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", openrouter.URL)
	runtime := NewRuntime(RuntimeDependencies{StateDir: t.TempDir()})
	batch := extractionBatch{BatchIndex: 1, BatchCount: 1, EpisodeIndexes: []string{episodeIndex}, Chunks: []extractionChunk{{EpisodeIndex: episodeIndex, Text: "合成本文"}}}

	result, err := runtime.generateOpenRouterBatch(context.Background(), &store.ResolvedAIGenerationConfig{APIKey: "sk-test", ModelID: "model"}, "novel-1", episodeIndex, nil, nil, batch)
	if err != nil {
		t.Fatalf("numeric episode indexes should be normalized: %v", err)
	}
	if len(result.Delta.Terms) != 1 || result.Delta.Terms[0].CategoryHistory[0].EpisodeIndex != episodeIndex || result.Delta.Terms[0].DescriptionHistory[0].EpisodeIndex != episodeIndex {
		t.Fatalf("numeric episode index precision was lost: %+v", result.Delta.Terms)
	}
}

func TestGenerateOpenRouterBatchUsesServerBatchFrontierForProcessedEpisodeIndex(t *testing.T) {
	openrouter := newExtractionOpenRouterTestServer(t,
		`{"processedUpToEpisodeIndex":"episode 4","newCharacters":[],"characterUpdates":[],"mergeProposals":[],"unresolvedMentions":[],"terms":[]}`,
	)
	defer openrouter.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", openrouter.URL)
	runtime := NewRuntime(RuntimeDependencies{StateDir: t.TempDir()})
	batch := extractionBatch{BatchIndex: 1, BatchCount: 1, EpisodeIndexes: []string{"4"}, Chunks: []extractionChunk{{EpisodeIndex: "4", Text: "合成本文"}}}

	if _, err := runtime.generateOpenRouterBatch(context.Background(), &store.ResolvedAIGenerationConfig{APIKey: "sk-test", ModelID: "model"}, "novel-1", "4", nil, nil, batch); err != nil {
		t.Fatalf("processed frontier should come from the server batch: %v", err)
	}
}

func TestGenerateOpenRouterBatchNormalizesFallbackCharacterShape(t *testing.T) {
	openrouter := newExtractionOpenRouterTestServer(t,
		`{"processedUpToEpisodeIndex":"episode 4","newCharacters":[{"temporaryId":"person-1","fullName":null,"fullNameHistory":[],"gender":null,"genderHistory":[],"firstAppearanceEpisodeIndex":null,"aliases":["合成人物"],"appearanceHistory":[],"personalityHistory":[],"summaryHistory":["合成説明"]},{"temporaryId":"unnamed-person","fullName":null,"fullNameHistory":[],"gender":null,"genderHistory":[],"firstAppearanceEpisodeIndex":null,"aliases":[],"appearanceHistory":[],"personalityHistory":[],"summaryHistory":["名前のない人物"]}],"characterUpdates":[],"mergeProposals":[],"unresolvedMentions":[],"terms":[{"name":"合成用語","reading":"ごうせいようご","category":"organization","description":"合成説明"}]}`,
	)
	defer openrouter.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", openrouter.URL)
	runtime := NewRuntime(RuntimeDependencies{StateDir: t.TempDir()})
	batch := extractionBatch{BatchIndex: 1, BatchCount: 1, EpisodeIndexes: []string{"4"}, Chunks: []extractionChunk{{EpisodeIndex: "4", Text: "合成本文"}}}

	result, err := runtime.generateOpenRouterBatch(context.Background(), &store.ResolvedAIGenerationConfig{APIKey: "sk-test", ModelID: "model"}, "novel-1", "4", nil, nil, batch)
	if err != nil {
		t.Fatalf("fallback character shape should be normalized: %v", err)
	}
	if len(result.Delta.NewCharacters) != 1 || result.Delta.NewCharacters[0].CanonicalName != "合成人物" || result.Delta.NewCharacters[0].FirstAppearanceEpisodeIndex != "4" || len(result.Delta.NewCharacters[0].SummaryHistory) != 1 {
		t.Fatalf("unexpected normalized character: %+v", result.Delta.NewCharacters)
	}
	if len(result.Delta.Terms) != 1 || result.Delta.Terms[0].Term != "合成用語" || result.Delta.Terms[0].DescriptionHistory[0].EpisodeIndex != "4" {
		t.Fatalf("unexpected normalized term: %+v", result.Delta.Terms)
	}
}

func TestGenerateOpenRouterBatchCompletesOmittedDeltaArrays(t *testing.T) {
	openrouter := newExtractionOpenRouterTestServer(t,
		`{"newCharacters":[{"canonicalName":"アリス","aliases":[],"summaryHistory":["主人公"]}],"characterUpdates":null,"terms":[]}`,
	)
	defer openrouter.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", openrouter.URL)
	runtime := NewRuntime(RuntimeDependencies{StateDir: t.TempDir()})
	batch := extractionBatch{BatchIndex: 1, BatchCount: 1, EpisodeIndexes: []string{"4"}, Chunks: []extractionChunk{{EpisodeIndex: "4", Text: "合成本文"}}}

	result, err := runtime.generateOpenRouterBatch(context.Background(), &store.ResolvedAIGenerationConfig{APIKey: "sk-test", ModelID: "model"}, "novel-1", "4", nil, nil, batch)
	if err != nil {
		t.Fatalf("omitted empty delta arrays should be completed: %v", err)
	}
	if len(result.Delta.NewCharacters) != 1 || result.Delta.NewCharacters[0].CanonicalName != "アリス" {
		t.Fatalf("unexpected normalized delta: %+v", result.Delta)
	}
}

func TestNormalizeExtractionRootDeltaArraysPreservesInvalidTypes(t *testing.T) {
	normalized, err := normalizeExtractionEpisodeIndexScalars([]byte(`{"newCharacters":"invalid","terms":[]}`), "4")
	if err != nil {
		t.Fatalf("normalize JSON object: %v", err)
	}
	if err := validateExtractionOutputContract(normalized); err == nil {
		t.Fatal("an explicitly invalid root delta type should still be rejected")
	}
}

func TestNormalizeExtractionRootDeltaArraysPreservesLegacyResponse(t *testing.T) {
	normalized, err := normalizeExtractionEpisodeIndexScalars([]byte(`{"characters":[],"terms":[]}`), "4")
	if err != nil {
		t.Fatalf("normalize legacy response: %v", err)
	}
	var root map[string]any
	if err := json.Unmarshal(normalized, &root); err != nil {
		t.Fatalf("decode legacy response: %v", err)
	}
	if _, exists := root["newCharacters"]; exists {
		t.Fatalf("legacy response must not be converted into an empty delta response: %+v", root)
	}
	if err := validateExtractionOutputContract(normalized); err != nil {
		t.Fatalf("legacy response should remain valid: %v", err)
	}
}

func TestGenerateOpenRouterBatchRecoversNullableNamesAndLegacyCharacterFields(t *testing.T) {
	openrouter := newExtractionOpenRouterTestServer(t,
		`{"newCharacters":[{"canonicalName":null,"fullName":null,"aliases":["アリス"],"summary":"主人公","appearance":"銀髪","personality":"慎重"},{"displayName":"ボブ","summary":"王国騎士"}],"terms":[{"term":null,"name":"魔導院","reading":"","category":"organization","description":"魔術を研究する組織"}]}`,
	)
	defer openrouter.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", openrouter.URL)
	runtime := NewRuntime(RuntimeDependencies{StateDir: t.TempDir()})
	batch := extractionBatch{BatchIndex: 1, BatchCount: 1, EpisodeIndexes: []string{"4"}, Chunks: []extractionChunk{{EpisodeIndex: "4", Text: "合成本文"}}}

	result, err := runtime.generateOpenRouterBatch(context.Background(), &store.ResolvedAIGenerationConfig{APIKey: "sk-test", ModelID: "model"}, "novel-1", "4", nil, nil, batch)
	if err != nil {
		t.Fatalf("recoverable fallback values should be normalized: %v", err)
	}
	if len(result.Delta.NewCharacters) != 2 || result.Delta.NewCharacters[0].CanonicalName != "アリス" || len(result.Delta.NewCharacters[0].SummaryHistory) != 1 || len(result.Delta.NewCharacters[0].AppearanceHistory) != 1 || len(result.Delta.NewCharacters[0].PersonalityHistory) != 1 {
		t.Fatalf("legacy character fields were not preserved: %+v", result.Delta.NewCharacters)
	}
	if result.Delta.NewCharacters[1].CanonicalName != "ボブ" || len(result.Delta.NewCharacters[1].SummaryHistory) != 1 {
		t.Fatalf("displayName fallback was not preserved: %+v", result.Delta.NewCharacters[1])
	}
	if len(result.Delta.Terms) != 1 || result.Delta.Terms[0].Term != "魔導院" || len(result.Delta.Terms[0].ReadingHistory) != 0 {
		t.Fatalf("nullable term fields were not recovered: %+v", result.Delta.Terms)
	}
}

func TestNormalizeExtractionEpisodeIndexScalarsCoversFallbackVariants(t *testing.T) {
	raw := []byte(`{
		"processedUpToEpisodeIndex":null,
		"newCharacters":[null,{"canonicalName":"合成人物","firstAppearanceEpisodeIndex":"3","extra":"ignored"}],
		"characterUpdates":[{"characterId":"char-1","extra":"ignored"}],
		"terms":[null,{"term":"","description":"ignored"},{"term":"既存形式","category":{"value":"place","episodeIndex":"3"},"descriptionHistory":[{"text":"説明","episodeIndex":"3"}]},{"term":"説明なし","reading":{"text":"せつめいなし"}}]
	}`)

	normalized, err := normalizeExtractionEpisodeIndexScalars(raw, "4")
	if err != nil {
		t.Fatalf("normalize fallback variants: %v", err)
	}
	var root map[string]any
	if err := json.Unmarshal(normalized, &root); err != nil {
		t.Fatalf("decode normalized fallback variants: %v", err)
	}
	if root["processedUpToEpisodeIndex"] != "4" {
		t.Fatalf("unexpected processed frontier: %+v", root)
	}
	newCharacters := root["newCharacters"].([]any)
	if len(newCharacters) != 1 {
		t.Fatalf("non-object character should be removed: %+v", newCharacters)
	}
	newCharacter := newCharacters[0].(map[string]any)
	if newCharacter["firstAppearanceEpisodeIndex"] != "3" || newCharacter["extra"] != nil {
		t.Fatalf("unexpected normalized new character: %+v", newCharacter)
	}
	updates := root["characterUpdates"].([]any)
	update := updates[0].(map[string]any)
	if update["canonicalName"] != nil || update["firstAppearanceEpisodeIndex"] != nil || update["extra"] != nil {
		t.Fatalf("unexpected normalized character update: %+v", update)
	}
	normalizedTerms := root["terms"].([]any)
	if len(normalizedTerms) != 1 {
		t.Fatalf("invalid terms should be removed: %+v", normalizedTerms)
	}
	category := normalizedTerms[0].(map[string]any)["category"].(map[string]any)
	if category["value"] != "place" || category["episodeIndex"] != "3" {
		t.Fatalf("existing term version should be preserved: %+v", normalizedTerms[0])
	}

	if _, err := normalizeExtractionEpisodeIndexScalars([]byte(`[]`), "4"); err != nil {
		t.Fatalf("non-object JSON should remain valid: %v", err)
	}
}

func TestExtractionFallbackNormalizerRejectsUnusableVariants(t *testing.T) {
	if _, err := normalizeExtractionEpisodeIndexScalars([]byte(`not-json`), "4"); err == nil {
		t.Fatal("invalid JSON should be rejected")
	}
	if err := validateExtractionCharacterItem(json.RawMessage(`{}`), false); err == nil {
		t.Fatal("missing character fields should be rejected")
	}
	for _, raw := range []string{`[]`, `{"text":"説明","extra":true}`, `{"text":1,"episodeIndex":"4"}`} {
		if err := validateExtractionVersionObject(json.RawMessage(raw)); err == nil {
			t.Fatalf("invalid version should be rejected: %s", raw)
		}
	}
	for _, value := range []any{"", map[string]any{"text": ""}, true} {
		if _, ok := extractionTextVersionFallback(value, "4"); ok {
			t.Fatalf("unusable text version should be rejected: %+v", value)
		}
	}
	for _, value := range []any{"", map[string]any{"value": ""}, true} {
		if _, ok := extractionTermVersionFallback(value, "value", "4"); ok {
			t.Fatalf("unusable term version should be rejected: %+v", value)
		}
	}
}

func TestGenerateOpenRouterBatchRetriesInvalidTermEpisodeIndex(t *testing.T) {
	invalid := `{"processedUpToEpisodeIndex":"20","newCharacters":[],"characterUpdates":[],"mergeProposals":[],"unresolvedMentions":[],"terms":[{"term":"帝国評議会","reading":null,"category":{"value":"organization","episodeIndex":"20"},"descriptionHistory":[{"text":"評議会。","episodeIndex":"unknown"}]}]}`
	valid := `{"processedUpToEpisodeIndex":"20","newCharacters":[],"characterUpdates":[],"mergeProposals":[],"unresolvedMentions":[],"terms":[{"term":"帝国評議会","reading":null,"category":{"value":"organization","episodeIndex":"20"},"descriptionHistory":[{"text":"評議会。","episodeIndex":"20"}]}]}`
	openrouter := newExtractionOpenRouterTestServer(t, invalid, valid)
	defer openrouter.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", openrouter.URL)
	runtime := NewRuntime(RuntimeDependencies{StateDir: t.TempDir()})
	batch := extractionBatch{BatchIndex: 1, BatchCount: 1, EpisodeIndexes: []string{"20"}, Chunks: []extractionChunk{{EpisodeIndex: "20", Text: "本文"}}}

	result, err := runtime.generateOpenRouterBatch(context.Background(), &store.ResolvedAIGenerationConfig{APIKey: "sk-test", ModelID: "model"}, "novel-1", "20", nil, nil, batch)
	if err != nil || len(result.Delta.Terms) != 1 || result.Delta.Terms[0].DescriptionHistory[0].EpisodeIndex != "20" {
		t.Fatalf("term contract retry should recover with valid episode: result=%+v err=%v", result, err)
	}
	if result.Usage.InputTokens != 22 || result.Usage.OutputTokens != 14 || result.Usage.TotalTokens != 36 {
		t.Fatalf("term retry usage should include both responses: %+v", result.Usage)
	}

	failedServer := newExtractionOpenRouterTestServer(t, invalid, invalid)
	defer failedServer.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", failedServer.URL)
	failed, err := runtime.generateOpenRouterBatch(context.Background(), &store.ResolvedAIGenerationConfig{APIKey: "sk-test", ModelID: "model"}, "novel-1", "20", nil, nil, batch)
	if err == nil || failed.Usage.TotalTokens != 36 {
		t.Fatalf("repeated invalid term output should fail with both usage records: result=%+v err=%v", failed, err)
	}
}

func TestGenerateOpenRouterBatchRetriesTruncatedModelOutput(t *testing.T) {
	call := 0
	openrouter := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		call++
		content := `{"processedUpToEpisodeIndex":"1"`
		finishReason := "length"
		if call == 2 {
			content = `{"processedUpToEpisodeIndex":"1","newCharacters":[],"characterUpdates":[],"mergeProposals":[],"unresolvedMentions":[],"terms":[]}`
			finishReason = "stop"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"finish_reason": finishReason, "message": map[string]any{"content": content}}},
			"usage":   map[string]any{"prompt_tokens": 0, "completion_tokens": 7, "total_tokens": 0},
		})
	}))
	defer openrouter.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", openrouter.URL)
	runtime := NewRuntime(RuntimeDependencies{StateDir: t.TempDir()})
	batch := extractionBatch{BatchIndex: 1, BatchCount: 1, EpisodeIndexes: []string{"1"}, Chunks: []extractionChunk{{EpisodeIndex: "1", Text: "本文"}}}

	result, err := runtime.generateOpenRouterBatch(context.Background(), &store.ResolvedAIGenerationConfig{APIKey: "sk-test", ModelID: "model"}, "novel-1", "1", nil, nil, batch)
	if err != nil || call != 2 {
		t.Fatalf("generateOpenRouterBatch should retry a truncated response: calls=%d err=%v", call, err)
	}
	if result.Usage.InputTokens <= 0 || result.Usage.OutputTokens != 14 || result.Usage.TotalTokens != result.Usage.InputTokens+14 {
		t.Fatalf("truncated retry usage should include both responses: %+v", result.Usage)
	}
}

func TestDiscoverParallelIdentityNamesRetriesBoundaryViolation(t *testing.T) {
	openrouter := newExtractionOpenRouterTestServer(
		t,
		`{"characters":[{"name":"王女セリア","aliases":[],"episodeIndex":"1","reason":"誤った話数"}]}`,
		`{"characters":[{"name":"王女セリア","aliases":[],"episodeIndex":"20","reason":"第20話で登場"}]}`,
	)
	defer openrouter.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", openrouter.URL)
	runtime := NewRuntime(RuntimeDependencies{StateDir: t.TempDir()})
	batch := extractionBatch{EpisodeIndexes: []string{"20"}, Chunks: []extractionChunk{{EpisodeIndex: "20", Text: "王女セリアが名乗った。"}}}

	names, usage, err := runtime.discoverParallelIdentityNamesForBatch(context.Background(), &store.ResolvedAIGenerationConfig{APIKey: "sk-test", ModelID: "model"}, "novel-1", "20", 0, batch)
	if err != nil || len(names) != 1 || names[0].EpisodeIndex != "20" {
		t.Fatalf("discovery should recover from a boundary-invalid response: names=%+v err=%v", names, err)
	}
	if usage.InputTokens != 22 || usage.OutputTokens != 14 || usage.TotalTokens != 36 {
		t.Fatalf("retry usage should include both discovery responses: %+v", usage)
	}
}

func TestGenerateOpenRouterChatWithOutputRetryStopsDuringWait(t *testing.T) {
	openrouter := newExtractionOpenRouterTestServer(t, `not-json`)
	defer openrouter.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", openrouter.URL)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	validationStarted := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, _, err := generateOpenRouterChatWithOutputRetry(ctx, ai.OpenRouterConfig{APIKey: "sk-test", ModelID: "model"}, []ai.ChatMessage{{Role: "user", Content: "JSON"}}, func(result ai.ChatResult) error {
			close(validationStarted)
			var decoded map[string]any
			return json.Unmarshal([]byte(result.Answer), &decoded)
		})
		done <- err
	}()
	<-validationStarted
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("retry wait should return context cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("retry wait did not stop after context cancellation")
	}
}
