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
