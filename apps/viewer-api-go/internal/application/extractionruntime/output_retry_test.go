package extractionruntime

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
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
			if test.name == "clusters" {
				confidence := items["properties"].(map[string]any)["confidence"].(map[string]any)
				if confidence["minimum"] != 0 || confidence["maximum"] != 1 {
					t.Fatalf("cluster confidence must be constrained to a probability: %+v", confidence)
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

func TestGenerateOpenRouterBatchRetriesLegacyRoot(t *testing.T) {
	openrouter := newExtractionOpenRouterTestServer(
		t,
		`{"processedUpToEpisodeIndex":"ep1","characters":[],"terms":[]}`,
		`{"processedUpToEpisodeIndex":"ep1","newCharacters":[],"characterUpdates":[],"mergeProposals":[],"unresolvedMentions":[],"terms":[]}`,
	)
	defer openrouter.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", openrouter.URL)
	runtime := NewRuntime(RuntimeDependencies{StateDir: t.TempDir()})
	batch := extractionBatch{BatchIndex: 1, BatchCount: 1, EpisodeIndexes: []string{"20"}, Chunks: []extractionChunk{{EpisodeIndex: "20", Text: "合成本文"}}}

	result, err := runtime.generateOpenRouterBatch(context.Background(), &store.ResolvedAIGenerationConfig{APIKey: "sk-test", ModelID: "model"}, "novel-1", "20", nil, nil, batch)
	if err != nil {
		t.Fatalf("legacy root should be rejected and recovered by retry: %v", err)
	}
	if result.Usage.InputTokens != 22 || result.Usage.OutputTokens != 14 || result.Usage.TotalTokens != 36 {
		t.Fatalf("legacy root should consume two attempts: %+v", result.Usage)
	}
}

func TestGenerateOpenRouterBatchRetriesOutOfRangeMergeConfidence(t *testing.T) {
	openrouter := newExtractionOpenRouterTestServer(
		t,
		`{"processedUpToEpisodeIndex":"ep1","newCharacters":[],"characterUpdates":[],"mergeProposals":[{"sourceCharacterId":"char-a","targetCharacterId":"char-b","confidence":95,"reason":"百分率"}],"unresolvedMentions":[],"terms":[]}`,
		`{"processedUpToEpisodeIndex":"ep1","newCharacters":[],"characterUpdates":[],"mergeProposals":[{"sourceCharacterId":"char-a","targetCharacterId":"char-b","confidence":0.95,"reason":"確率"}],"unresolvedMentions":[],"terms":[]}`,
	)
	defer openrouter.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", openrouter.URL)
	runtime := NewRuntime(RuntimeDependencies{StateDir: t.TempDir()})
	batch := extractionBatch{BatchIndex: 1, BatchCount: 1, EpisodeIndexes: []string{"20"}, Chunks: []extractionChunk{{EpisodeIndex: "20", Text: "合成本文"}}}

	result, err := runtime.generateOpenRouterBatch(context.Background(), &store.ResolvedAIGenerationConfig{APIKey: "sk-test", ModelID: "model"}, "novel-1", "20", nil, nil, batch)
	if err != nil || len(result.Delta.MergeProposals) != 1 || result.Delta.MergeProposals[0].Confidence != 0.95 {
		t.Fatalf("out-of-range confidence should be rejected and recovered by retry: result=%+v err=%v", result, err)
	}
	if result.Usage.InputTokens != 22 || result.Usage.OutputTokens != 14 || result.Usage.TotalTokens != 36 {
		t.Fatalf("out-of-range confidence should consume two attempts: %+v", result.Usage)
	}
}

func TestGenerateOpenRouterBatchRetriesNullRequiredScalars(t *testing.T) {
	invalidResponses := map[string]string{
		"merge confidence":   `{"processedUpToEpisodeIndex":"ep1","newCharacters":[],"characterUpdates":[],"mergeProposals":[{"sourceCharacterId":"char-a","targetCharacterId":"char-b","confidence":null,"reason":"同一人物"}],"unresolvedMentions":[],"terms":[]}`,
		"character history":  `{"processedUpToEpisodeIndex":"ep1","newCharacters":[{"canonicalName":{"text":"アリス","episodeIndex":"ep1"},"fullName":null,"fullNameHistory":[],"gender":null,"genderHistory":[],"firstAppearanceEpisodeIndex":"ep1","aliases":[],"appearanceHistory":[],"personalityHistory":[],"summaryHistory":[{"text":null,"episodeIndex":"ep1"}]}],"characterUpdates":[],"mergeProposals":[],"unresolvedMentions":[],"terms":[]}`,
		"unresolved mention": `{"processedUpToEpisodeIndex":"ep1","newCharacters":[],"characterUpdates":[],"mergeProposals":[],"unresolvedMentions":[{"mention":null,"episodeIndex":"ep1","reason":"候補不明"}],"terms":[]}`,
		"term description":   `{"processedUpToEpisodeIndex":"ep1","newCharacters":[],"characterUpdates":[],"mergeProposals":[],"unresolvedMentions":[],"terms":[{"term":"帝国評議会","reading":null,"category":{"value":"organization","episodeIndex":"ep1"},"descriptionHistory":[{"text":null,"episodeIndex":"ep1"}]}]}`,
	}
	valid := `{"processedUpToEpisodeIndex":"ep1","newCharacters":[],"characterUpdates":[],"mergeProposals":[],"unresolvedMentions":[],"terms":[]}`

	for name, invalid := range invalidResponses {
		t.Run(name, func(t *testing.T) {
			openrouter := newExtractionOpenRouterTestServer(t, invalid, valid)
			defer openrouter.Close()
			t.Setenv("OPENROUTER_API_BASE_URL", openrouter.URL)
			runtime := NewRuntime(RuntimeDependencies{StateDir: t.TempDir()})
			batch := extractionBatch{BatchIndex: 1, BatchCount: 1, EpisodeIndexes: []string{"20"}, Chunks: []extractionChunk{{EpisodeIndex: "20", Text: "合成本文"}}}

			result, err := runtime.generateOpenRouterBatch(context.Background(), &store.ResolvedAIGenerationConfig{APIKey: "sk-test", ModelID: "model"}, "novel-1", "20", nil, nil, batch)
			if err != nil {
				t.Fatalf("null scalar should be rejected and recovered by retry: %v", err)
			}
			if result.Usage.InputTokens != 22 || result.Usage.OutputTokens != 14 || result.Usage.TotalTokens != 36 {
				t.Fatalf("null scalar should consume two attempts: %+v", result.Usage)
			}
		})
	}
}

func TestGenerateOpenRouterBatchAddsContractCorrectionOnRetry(t *testing.T) {
	requests := 0
	openrouter := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Messages []ai.ChatMessage `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode OpenRouter request: %v", err)
		}
		requests++
		content := `{"processedUpToEpisodeIndex":"20","newCharacters":[],"characterUpdates":[],"mergeProposals":[],"unresolvedMentions":[{"mention":"合成人物","episodeIndex":"1","reason":"候補不明"}],"terms":[]}`
		if requests == 2 {
			if len(request.Messages) < 2 {
				t.Fatalf("retry should append a correction message: %+v", request.Messages)
			}
			correction, ok := request.Messages[len(request.Messages)-1].Content.(string)
			if !ok || !strings.Contains(correction, "必須fieldと空配列") || !strings.Contains(correction, "現在の抽出バッチ") {
				t.Fatalf("retry correction should identify the contract failure: %#v", request.Messages[len(request.Messages)-1].Content)
			}
			content = `{"processedUpToEpisodeIndex":"20","newCharacters":[],"characterUpdates":[],"mergeProposals":[],"unresolvedMentions":[{"mention":"合成人物","episodeIndex":"20","reason":"候補不明"}],"terms":[]}`
		}
		if err := json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": content}}},
			"usage":   map[string]any{"prompt_tokens": 11, "completion_tokens": 7, "total_tokens": 18},
		}); err != nil {
			t.Fatalf("encode OpenRouter response: %v", err)
		}
	}))
	defer openrouter.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", openrouter.URL)
	runtime := NewRuntime(RuntimeDependencies{StateDir: t.TempDir()})
	batch := extractionBatch{BatchIndex: 1, BatchCount: 1, EpisodeIndexes: []string{"20"}, Chunks: []extractionChunk{{EpisodeIndex: "20", Text: "合成本文"}}}

	result, err := runtime.generateOpenRouterBatch(context.Background(), &store.ResolvedAIGenerationConfig{APIKey: "sk-test", ModelID: "model"}, "novel-1", "20", nil, nil, batch)
	if err != nil || len(result.Delta.UnresolvedMentions) != 1 {
		t.Fatalf("corrected retry should succeed: result=%+v err=%v", result, err)
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

func TestGenerateOpenRouterBatchUsesShortModelEpisodeReferences(t *testing.T) {
	const episodeIndex = "16818093083280979446"
	openrouter := newExtractionOpenRouterTestServer(t, `{"processedUpToEpisodeIndex":"ep2","newCharacters":[],"characterUpdates":[],"mergeProposals":[],"unresolvedMentions":[{"mention":"合成人物","episodeIndex":"ep2","reason":"候補不明"}],"terms":[]}`)
	defer openrouter.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", openrouter.URL)
	runtime := NewRuntime(RuntimeDependencies{StateDir: t.TempDir()})
	batch := extractionBatch{BatchIndex: 1, BatchCount: 1, EpisodeIndexes: []string{
		"16818093082855575914",
		episodeIndex,
	}, Chunks: []extractionChunk{{EpisodeIndex: episodeIndex, Text: "合成本文"}}}

	result, err := runtime.generateOpenRouterBatch(context.Background(), &store.ResolvedAIGenerationConfig{APIKey: "sk-test", ModelID: "model"}, "novel-1", episodeIndex, nil, nil, batch)
	if err != nil || len(result.Delta.UnresolvedMentions) != 1 || result.Delta.UnresolvedMentions[0].EpisodeIndex != episodeIndex {
		t.Fatalf("short model reference should be restored to the real episode index: result=%+v err=%v", result, err)
	}
}

func TestPrepareExtractionRequestHidesLongEpisodeIndexesFromOutputContract(t *testing.T) {
	const firstEpisodeIndex = "16818093082855575914"
	const secondEpisodeIndex = "16818093083280979446"
	prepared := prepareExtractionRequest(
		&store.ResolvedAIGenerationConfig{},
		"novel-1",
		secondEpisodeIndex,
		nil,
		nil,
		extractionBatch{
			EpisodeIndexes: []string{firstEpisodeIndex, secondEpisodeIndex},
			Chunks: []extractionChunk{
				{EpisodeIndex: firstEpisodeIndex, Text: "合成本文1"},
				{EpisodeIndex: secondEpisodeIndex, Text: "合成本文2"},
			},
		},
		nil,
	)
	userPrompt, _ := prepared.Messages[1].Content.(string)
	if strings.Contains(userPrompt, firstEpisodeIndex) || strings.Contains(userPrompt, secondEpisodeIndex) || !strings.Contains(userPrompt, `"episodeIndex": "ep1"`) || !strings.Contains(userPrompt, `"episodeIndex": "ep2"`) {
		t.Fatalf("episode payload should use only short model references: %s", userPrompt)
	}
	definitions := prepared.ResponseFormat["json_schema"].(map[string]any)["schema"].(map[string]any)["$defs"].(map[string]any)
	values := definitions["episodeIndex"].(map[string]any)["enum"].([]any)
	if !reflect.DeepEqual(values, []any{"ep1", "ep2"}) {
		t.Fatalf("response schema should enumerate short model references: %+v", values)
	}
	if prepared.ModelToActualEpisodeIndex["ep1"] != firstEpisodeIndex || prepared.ModelToActualEpisodeIndex["ep2"] != secondEpisodeIndex {
		t.Fatalf("model references should map back to real IDs: %+v", prepared.ModelToActualEpisodeIndex)
	}
}

func TestRemoveOpaqueEpisodeMetadataFromExtractionPrompt(t *testing.T) {
	payload := map[string]any{
		"candidateCharacters": []any{map[string]any{"characterId": "char-1", "firstAppearance": "16818093082855575914"}},
		"knownTerms": []any{
			map[string]any{"term": "合成用語", "latestEpisodeIndex": "16818093082855575914"},
			map[string]any{"term": "短い話数", "latestEpisodeIndex": "12"},
		},
		"unresolvedMentions": []any{map[string]any{"mention": "先生", "episodeIndex": "16818093082855575914"}},
		"episodes":           []any{map[string]any{"episodeIndex": "ep1"}},
	}

	removeOpaqueEpisodeMetadataFromExtractionPrompt(payload)
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("encode prompt payload: %v", err)
	}
	if strings.Contains(string(encoded), "16818093082855575914") || !strings.Contains(string(encoded), `"episodeIndex":"ep1"`) || !strings.Contains(string(encoded), `"latestEpisodeIndex":"12"`) {
		t.Fatalf("prompt should retain only current-batch short references: %s", encoded)
	}
}

func TestNormalizeExtractionEpisodeIndexesPreservesInvalidRootTypes(t *testing.T) {
	normalized, err := normalizeExtractionEpisodeIndexScalars([]byte(`{"newCharacters":"invalid","terms":[]}`), "4")
	if err != nil {
		t.Fatalf("normalize JSON object: %v", err)
	}
	if err := validateExtractionOutputContract(normalized); err == nil {
		t.Fatal("an explicitly invalid root delta type should still be rejected")
	}
}

func TestNormalizeExtractionEpisodeIndexesRejectsLegacyResponse(t *testing.T) {
	for name, raw := range map[string]string{
		"legacy": `{"characters":[],"terms":[]}`,
		"mixed":  `{"processedUpToEpisodeIndex":"4","characters":[],"newCharacters":[],"characterUpdates":[],"mergeProposals":[],"unresolvedMentions":[],"terms":[]}`,
	} {
		t.Run(name, func(t *testing.T) {
			normalized, err := normalizeExtractionEpisodeIndexScalars([]byte(raw), "4")
			if err != nil {
				t.Fatalf("normalize response: %v", err)
			}
			if err := validateExtractionOutputContract(normalized); err == nil {
				t.Fatalf("%s response should be rejected", name)
			}
		})
	}
}

func TestExtractionResponseValidationRejectsUnusableVariants(t *testing.T) {
	if _, err := normalizeExtractionEpisodeIndexScalars([]byte(`not-json`), "4"); err == nil {
		t.Fatal("invalid JSON should be rejected")
	}
	if err := validateExtractionCharacterItem(json.RawMessage(`{}`), false); err == nil {
		t.Fatal("missing character fields should be rejected")
	}
	for _, raw := range []string{`[]`, `{"text":"説明","extra":true}`, `{"text":1,"episodeIndex":"4"}`, `{"text":null,"episodeIndex":"4"}`} {
		if err := validateExtractionVersionObject(json.RawMessage(raw)); err == nil {
			t.Fatalf("invalid version should be rejected: %s", raw)
		}
	}
	for _, field := range []string{"mention", "episodeIndex", "reason"} {
		item := map[string]json.RawMessage{
			"mention":      json.RawMessage(`"影"`),
			"episodeIndex": json.RawMessage(`"4"`),
			"reason":       json.RawMessage(`"候補不明"`),
		}
		item[field] = json.RawMessage("null")
		encoded, err := json.Marshal(item)
		if err != nil {
			t.Fatalf("encode null scalar fixture: %v", err)
		}
		if err := validateExtractionSimpleObject(encoded, []string{"mention", "episodeIndex", "reason"}); err == nil {
			t.Fatalf("null %s should be rejected", field)
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
		`{"characters":[{"name":"王女セリア","aliases":[],"episodeIndex":"ep1","reason":"第20話で登場"}]}`,
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
