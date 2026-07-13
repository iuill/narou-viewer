package ai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestGenerateOpenRouterChatRejectsMissingConfig(t *testing.T) {
	if _, err := GenerateOpenRouterChat(context.Background(), OpenRouterConfig{
		ModelID: "openrouter/auto",
	}, []ChatMessage{{Role: "user", Content: "hello"}}); err == nil || !strings.Contains(err.Error(), "API key") {
		t.Fatalf("expected missing API key error, got %v", err)
	}
	if _, err := GenerateOpenRouterChat(context.Background(), OpenRouterConfig{
		APIKey: "sk-test",
	}, []ChatMessage{{Role: "user", Content: "hello"}}); err == nil || !strings.Contains(err.Error(), "model") {
		t.Fatalf("expected missing model error, got %v", err)
	}
}

func TestGenerateOpenRouterChatUsesOpenAICompatibleEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("authorization") != "Bearer sk-test" {
			t.Fatalf("unexpected authorization header: %q", r.Header.Get("authorization"))
		}
		if r.Header.Get("http-referer") != "https://viewer.example.test" || r.Header.Get("x-openrouter-title") != "narou-viewer-test" {
			t.Fatalf("unexpected OpenRouter metadata headers: referer=%q title=%q", r.Header.Get("http-referer"), r.Header.Get("x-openrouter-title"))
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body["model"] != "openrouter/auto" {
			t.Fatalf("unexpected model: %+v", body)
		}
		provider := body["provider"].(map[string]any)
		if provider["allow_fallbacks"] != false || provider["require_parameters"] != true {
			t.Fatalf("unexpected provider options: %+v", provider)
		}
		_, _ = w.Write([]byte(`{
			"choices": [{"message": {"content": " テスト応答 ", "reasoning": null, "reasoning_details": null}}],
			"usage": {"prompt_tokens": 3, "completion_tokens": 4, "total_tokens": 7}
		}`))
	}))
	defer server.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", server.URL)
	t.Setenv("OPENROUTER_HTTP_REFERER", "https://viewer.example.test")
	t.Setenv("OPENROUTER_APP_TITLE", "narou-viewer-test")

	result, err := GenerateOpenRouterChat(context.Background(), OpenRouterConfig{
		APIKey:            "sk-test",
		ModelID:           "openrouter/auto",
		ProviderOrder:     []string{"OpenAI"},
		AllowFallbacks:    false,
		RequireParameters: true,
	}, []ChatMessage{{Role: "user", Content: "hello"}})
	if err != nil {
		t.Fatalf("GenerateOpenRouterChat returned error: %v", err)
	}
	if result.Answer != "テスト応答" || result.InputTokens != 3 || result.OutputTokens != 4 || result.TotalTokens != 7 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if result.Reasoning != nil || result.ReasoningDetails != nil {
		t.Fatalf("explicit null reasoning fields should be normalized away: %+v", result)
	}
}

func TestGenerateOpenRouterChatForwardsOptionalGenerationParameters(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body["temperature"] != float64(0.2) || body["max_tokens"] != float64(4096) {
			t.Fatalf("optional generation parameters should be forwarded: %+v", body)
		}
		reasoning := body["reasoning"].(map[string]any)
		if reasoning["effort"] != "xhigh" {
			t.Fatalf("reasoning effort should be forwarded: %+v", body)
		}
		responseFormat := body["response_format"].(map[string]any)
		if responseFormat["type"] != "json_schema" {
			t.Fatalf("response_format should be forwarded: %+v", body)
		}
		provider := body["provider"].(map[string]any)
		if provider["require_parameters"] != true {
			t.Fatalf("reasoning requests should require provider parameter support: %+v", body)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer server.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", server.URL)
	temperature := 0.2

	result, err := GenerateOpenRouterChat(context.Background(), OpenRouterConfig{
		APIKey:            "sk-test",
		ModelID:           "openrouter/auto",
		AllowFallbacks:    true,
		RequireParameters: false,
		ReasoningEffort:   " xHIGH ",
		Temperature:       &temperature,
		MaxTokens:         4096,
		ResponseFormat:    map[string]any{"type": "json_schema"},
	}, []ChatMessage{{Role: "user", Content: "hello"}})
	if err != nil {
		t.Fatalf("GenerateOpenRouterChat returned error: %v", err)
	}
	if result.Answer != "ok" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestApplyOpenRouterReasoningUsesEnvironmentFallback(t *testing.T) {
	t.Setenv("OPENROUTER_REASONING_EFFORT", "high")
	body := map[string]any{}
	request, err := applyOpenRouterReasoning(body, OpenRouterConfig{})
	if err != nil {
		t.Fatalf("applyOpenRouterReasoning returned error: %v", err)
	}
	reasoning := body["reasoning"].(map[string]any)
	if reasoning["effort"] != "high" || request.RequestedEffort == nil || *request.RequestedEffort != "high" || request.Source != "environment" || !request.RequireParameters {
		t.Fatalf("environment reasoning effort should be forwarded: %+v", body)
	}
	if _, err := applyOpenRouterReasoning(map[string]any{}, OpenRouterConfig{ReasoningEffort: "extreme"}); err == nil {
		t.Fatal("invalid reasoning effort should be rejected")
	}
	t.Setenv("OPENROUTER_REASONING_EFFORT", "extreme")
	if _, err := ResolveOpenRouterReasoningRequest(OpenRouterConfig{}); err == nil {
		t.Fatal("invalid environment reasoning effort should be rejected during startup validation")
	}
}

func TestNormalizeOpenRouterReasoningEffortAcceptsGatewayValues(t *testing.T) {
	for _, effort := range []string{"none", "minimal", "low", "medium", "high", "xhigh", "max"} {
		if normalized, ok := NormalizeOpenRouterReasoningEffort(effort); !ok || normalized != effort {
			t.Fatalf("gateway effort %q should be accepted, got %q ok=%v", effort, normalized, ok)
		}
	}
}

func TestGenerateOpenRouterToolChatForwardsToolsAndReturnsToolCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		tools := body["tools"].([]any)
		firstTool := tools[0].(map[string]any)
		function := firstTool["function"].(map[string]any)
		if firstTool["type"] != "function" || function["name"] != "load_episode_range" {
			t.Fatalf("tool definition should be forwarded: %+v", body)
		}
		messages := body["messages"].([]any)
		if messages[0].(map[string]any)["role"] != "user" {
			t.Fatalf("messages should be forwarded: %+v", body)
		}
		reasoning := body["reasoning"].(map[string]any)
		if reasoning["effort"] != "high" {
			t.Fatalf("reasoning effort should be forwarded to tool chat: %+v", body)
		}
		_, _ = w.Write([]byte(`{
			"choices": [{
				"finish_reason": "tool_calls",
				"message": {
					"tool_calls": [{
						"id": "call_1",
						"type": "function",
						"function": {
							"name": "load_episode_range",
							"arguments": "{\"startEpisodeNumber\":1,\"endEpisodeNumber\":5}"
						}
					}]
				}
			}],
			"usage": {"prompt_tokens": 5, "completion_tokens": 2, "total_tokens": 7}
		}`))
	}))
	defer server.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", server.URL)

	result, err := GenerateOpenRouterToolChat(context.Background(), OpenRouterConfig{
		APIKey:          "sk-test",
		ModelID:         "openrouter/auto",
		ReasoningEffort: "high",
	}, []ChatMessage{{Role: "user", Content: "1〜5話を見たい"}}, []ToolDefinition{
		{Type: "function", Function: ToolFunction{Name: "load_episode_range", Parameters: map[string]any{"type": "object"}}},
	})
	if err != nil {
		t.Fatalf("GenerateOpenRouterToolChat returned error: %v", err)
	}
	if len(result.ToolCalls) != 1 || result.ToolCalls[0].Function.Name != "load_episode_range" || result.InputTokens != 5 {
		t.Fatalf("unexpected tool chat result: %+v", result)
	}
}

func TestGenerateOpenRouterChatHandlesNonRetryableErrors(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"bad request"}}`))
	}))
	defer server.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", server.URL)

	if _, err := GenerateOpenRouterChat(context.Background(), OpenRouterConfig{
		APIKey:  "sk-test",
		ModelID: "openrouter/auto",
	}, []ChatMessage{{Role: "user", Content: "hello"}}); err == nil || !strings.Contains(err.Error(), "bad request") {
		t.Fatalf("expected non-retryable OpenRouter error, got %v", err)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("non-retryable errors should not retry, calls=%d", calls)
	}
}

func TestGenerateOpenRouterChatRetriesRetryableErrors(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"message":"rate limited"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer server.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", server.URL)

	result, err := GenerateOpenRouterChat(context.Background(), OpenRouterConfig{
		APIKey:  "sk-test",
		ModelID: "openrouter/auto",
	}, []ChatMessage{{Role: "user", Content: "hello"}})
	if err != nil {
		t.Fatalf("GenerateOpenRouterChat returned error: %v", err)
	}
	if result.Answer != "ok" || atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("unexpected retry result: result=%+v calls=%d", result, calls)
	}
}

func TestGenerateOpenRouterChatDoesNotRetryClientTimeout(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		time.Sleep(2 * time.Second)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"late"}}]}`))
	}))
	defer server.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", server.URL)
	t.Setenv("OPENROUTER_REQUEST_TIMEOUT_SECONDS", "1")

	if _, err := GenerateOpenRouterChat(context.Background(), OpenRouterConfig{
		APIKey:  "sk-test",
		ModelID: "openrouter/auto",
	}, []ChatMessage{{Role: "user", Content: "hello"}}); err == nil {
		t.Fatal("expected client timeout error")
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("client timeouts should not retry, calls=%d", calls)
	}
}

func TestGenerateOpenRouterChatStopsRetryDelayOnContextCancel(t *testing.T) {
	var cancel context.CancelFunc
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cancel()
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"rate limited"}}`))
	}))
	defer server.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", server.URL)

	ctx, cancelFunc := context.WithCancel(context.Background())
	cancel = cancelFunc
	_, err := GenerateOpenRouterChat(ctx, OpenRouterConfig{
		APIKey:  "sk-test",
		ModelID: "openrouter/auto",
	}, []ChatMessage{{Role: "user", Content: "hello"}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation during retry delay, got %v", err)
	}
}

func TestGenerateOpenRouterChatRetriesNonJSONRetryableErrors(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`upstream failed`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer server.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", server.URL)

	result, err := GenerateOpenRouterChat(context.Background(), OpenRouterConfig{
		APIKey:  "sk-test",
		ModelID: "openrouter/auto",
	}, []ChatMessage{{Role: "user", Content: "hello"}})
	if err != nil {
		t.Fatalf("GenerateOpenRouterChat returned error: %v", err)
	}
	if result.Answer != "ok" || atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("unexpected non-json retry result: result=%+v calls=%d", result, calls)
	}
}

func TestGenerateOpenRouterChatRejectsEmptyChoices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[]}`))
	}))
	defer server.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", server.URL)

	if _, err := GenerateOpenRouterChat(context.Background(), OpenRouterConfig{
		APIKey:  "sk-test",
		ModelID: "openrouter/auto",
	}, []ChatMessage{{Role: "user", Content: "hello"}}); err == nil || !strings.Contains(err.Error(), "empty response") {
		t.Fatalf("expected empty choices error, got %v", err)
	}
}

func TestGenerateOpenRouterChatRejectsBlankContentAndMalformedJSON(t *testing.T) {
	for name, body := range map[string]string{
		"blank content":  `{"choices":[{"message":{"content":"   "}}]}`,
		"malformed json": `{`,
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(body))
			}))
			defer server.Close()
			t.Setenv("OPENROUTER_API_BASE_URL", server.URL)

			if _, err := GenerateOpenRouterChat(context.Background(), OpenRouterConfig{
				APIKey:  "sk-test",
				ModelID: "openrouter/auto",
			}, []ChatMessage{{Role: "user", Content: "hello"}}); err == nil {
				t.Fatal("expected OpenRouter response error")
			}
		})
	}
}

func TestGenerateOpenRouterChatRejectsOversizedResponseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"`))
		_, _ = w.Write([]byte(strings.Repeat("x", int(maxOpenRouterResponseBytes)+1)))
		_, _ = w.Write([]byte(`"}}]}`))
	}))
	defer server.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", server.URL)

	if _, err := GenerateOpenRouterChat(context.Background(), OpenRouterConfig{
		APIKey:  "sk-test",
		ModelID: "openrouter/auto",
	}, []ChatMessage{{Role: "user", Content: "hello"}}); err == nil || !strings.Contains(err.Error(), "exceeded") {
		t.Fatalf("expected oversized response body error, got %v", err)
	}
}

func TestGenerateOpenRouterChatRejectsTruncatedFinishReason(t *testing.T) {
	for _, finishReason := range []string{"length", "max_tokens", "content_filter"} {
		t.Run(finishReason, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(`{"choices":[{"finish_reason":"` + finishReason + `","message":{"content":"途中までの応答"}}],"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}}`))
			}))
			defer server.Close()
			t.Setenv("OPENROUTER_API_BASE_URL", server.URL)

			result, err := GenerateOpenRouterChat(context.Background(), OpenRouterConfig{
				APIKey:  "sk-test",
				ModelID: "openrouter/auto",
			}, []ChatMessage{{Role: "user", Content: "hello"}})
			if err == nil || !errors.Is(err, ErrOpenRouterTruncatedResponse) || !IsOpenRouterOutputError(err) || !strings.Contains(err.Error(), "finish_reason="+finishReason) || result.TotalTokens != 18 {
				t.Fatalf("expected truncated finish reason error, got %v", err)
			}
		})
	}
}

func TestGenerateOpenRouterChatReturnsLastRetryError(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":{"message":"upstream unavailable"}}`))
	}))
	defer server.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", server.URL)

	if _, err := GenerateOpenRouterChat(context.Background(), OpenRouterConfig{
		APIKey:  "sk-test",
		ModelID: "openrouter/auto",
	}, []ChatMessage{{Role: "user", Content: "hello"}}); err == nil || !strings.Contains(err.Error(), "upstream unavailable") {
		t.Fatalf("expected exhausted retry error, got %v", err)
	}
	if atomic.LoadInt32(&calls) != 3 {
		t.Fatalf("retryable errors should be attempted three times, calls=%d", calls)
	}
}

func TestOpenRouterMetadataDefaults(t *testing.T) {
	t.Setenv("OPENROUTER_API_BASE_URL", " ")
	t.Setenv("OPENROUTER_HTTP_REFERER", " ")
	t.Setenv("OPENROUTER_APP_TITLE", " ")
	if got := openRouterChatCompletionsURL(); got != "https://openrouter.ai/api/v1/chat/completions" {
		t.Fatalf("unexpected default OpenRouter URL: %q", got)
	}
	if got := openRouterHTTPReferer(); got != "http://localhost" {
		t.Fatalf("unexpected default referer: %q", got)
	}
	if got := openRouterAppTitle(); got != "narou-viewer" {
		t.Fatalf("unexpected default app title: %q", got)
	}
}

func TestLookupOpenRouterModelInfoUsesModelsEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("q") != "openai/gpt-4.1-mini" {
			t.Fatalf("unexpected model query: %s", r.URL.RawQuery)
		}
		if r.Header.Get("authorization") != "Bearer dummy-openrouter-key-realish" {
			t.Fatalf("unexpected authorization header: %q", r.Header.Get("authorization"))
		}
		_, _ = w.Write([]byte(`{
			"data": [{
				"id": "openai/gpt-4.1-mini",
				"canonical_slug": "openai/gpt-4.1-mini",
				"context_length": 128000,
				"top_provider": {
					"context_length": 64000,
					"max_completion_tokens": 16384
				}
			}]
		}`))
	}))
	defer server.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", server.URL)

	info, ok := LookupOpenRouterModelInfo(context.Background(), "dummy-openrouter-key-realish", "openai/gpt-4.1-mini")
	if !ok {
		t.Fatal("expected model metadata to be found")
	}
	if info.ContextLength != 64000 || info.MaxCompletionTokens != 16384 {
		t.Fatalf("unexpected model metadata: %+v", info)
	}
}

func TestLookupOpenRouterModelInfoUsesProviderFilterAndRequestLimits(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("q") != "openai/provider-fixed-model" || r.URL.Query().Get("providers") != "ProviderB,ProviderC" {
			t.Fatalf("unexpected model metadata query: %s", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{
			"data": [{
				"id": "openai/provider-fixed-model",
				"context_length": 128000,
				"top_provider": {
					"context_length": 64000,
					"max_completion_tokens": 32768
				},
				"per_request_limits": {
					"context_length": 32000,
					"max_completion_tokens": 4096
				}
			}]
		}`))
	}))
	defer server.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", server.URL)

	info, ok := LookupOpenRouterModelInfo(context.Background(), "dummy-openrouter-key-realish-provider", "openai/provider-fixed-model", []string{"ProviderB", "ProviderC", "ProviderB"})
	if !ok {
		t.Fatal("expected provider-filtered model metadata to be found")
	}
	if info.ContextLength != 32000 || info.MaxCompletionTokens != 4096 {
		t.Fatalf("provider-filtered metadata should honor per-request limits: %+v", info)
	}
}

func TestLookupOpenRouterModelInfoCachesEndpointFailuresBriefly(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		http.Error(w, "temporary failure", http.StatusBadGateway)
	}))
	defer server.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", server.URL)

	for i := 0; i < 2; i++ {
		if _, ok := LookupOpenRouterModelInfo(context.Background(), "dummy-openrouter-key-realish-failure-cache", "openai/failure-cache-model"); ok {
			t.Fatal("failed model metadata lookups should not return model info")
		}
	}
	if calls != 1 {
		t.Fatalf("failed model metadata lookup should be briefly cached, calls=%d", calls)
	}
}

func TestLookupOpenRouterModelInfoDoesNotShareFailuresAcrossCredentials(t *testing.T) {
	calls := 0
	modelID := "openai/credential-cache-model"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("authorization") == "Bearer dummy-openrouter-key-bad-key" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"message":"invalid key"}}`))
			return
		}
		_, _ = w.Write([]byte(`{
			"data": [{
				"id": "openai/credential-cache-model",
				"context_length": 128000,
				"top_provider": {
					"context_length": 128000,
					"max_completion_tokens": 8192
				}
			}]
		}`))
	}))
	defer server.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", server.URL)

	if _, ok := LookupOpenRouterModelInfo(context.Background(), "dummy-openrouter-key-bad-key", modelID); ok {
		t.Fatal("unauthorized metadata lookup should fail")
	}
	info, ok := LookupOpenRouterModelInfo(context.Background(), "dummy-openrouter-key-good-key", modelID)
	if !ok || info.MaxCompletionTokens != 8192 {
		t.Fatalf("a different valid credential should not be blocked by an auth failure: ok=%v info=%+v", ok, info)
	}
	if calls != 2 {
		t.Fatalf("auth failures should not be negative-cached across credentials, calls=%d", calls)
	}
}

func TestLookupOpenRouterModelInfoDoesNotNegativeCacheContextCancellation(t *testing.T) {
	calls := 0
	modelID := "openai/cancel-cache-model"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			time.Sleep(50 * time.Millisecond)
			return
		}
		_, _ = w.Write([]byte(`{
			"data": [{
				"id": "openai/cancel-cache-model",
				"context_length": 64000,
				"top_provider": {
					"context_length": 64000,
					"max_completion_tokens": 4096
				}
			}]
		}`))
	}))
	defer server.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", server.URL)

	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	if _, ok := LookupOpenRouterModelInfo(ctx, "dummy-openrouter-key-cancel-cache", modelID); ok {
		t.Fatal("timed out metadata lookup should fail")
	}
	info, ok := LookupOpenRouterModelInfo(context.Background(), "dummy-openrouter-key-cancel-cache", modelID)
	if !ok || info.ContextLength != 64000 {
		t.Fatalf("context timeout should not be negative-cached: ok=%v info=%+v", ok, info)
	}
	if calls != 2 {
		t.Fatalf("timed out lookup and successful retry should both reach the server, calls=%d", calls)
	}
}

func TestOpenRouterModelInfoLimitHelpers(t *testing.T) {
	if got := positiveOpenRouterNumberLimit(map[string]any{"max_tokens": "2048"}, "max_tokens"); got != 2048 {
		t.Fatalf("string request limit should be parsed, got %d", got)
	}
	if got := positiveOpenRouterNumberLimit(map[string]any{"max_tokens": json.Number("4096")}, "max_tokens"); got != 4096 {
		t.Fatalf("json.Number request limit should be parsed, got %d", got)
	}
	if got := positiveOpenRouterNumberLimit(map[string]any{"max_tokens": -1}, "max_tokens"); got != 0 {
		t.Fatalf("non-positive request limits should be ignored, got %d", got)
	}
	if got := normalizeOpenRouterProviderFilter([]string{" ProviderA ", "", "ProviderA", "ProviderB"}); len(got) != 2 || got[0] != "ProviderA" || got[1] != "ProviderB" {
		t.Fatalf("provider filter should trim and dedupe values: %+v", got)
	}
}

func TestOpenRouterModelsHTTPErrorNegativeCachePolicy(t *testing.T) {
	if got := (openRouterModelsHTTPError{statusCode: http.StatusInternalServerError}).Error(); !strings.Contains(got, "500") {
		t.Fatalf("HTTP status should be included in the error message, got %q", got)
	}
	if shouldNegativeCacheOpenRouterModelInfoError(openRouterModelsHTTPError{statusCode: http.StatusUnauthorized}) {
		t.Fatal("auth failures should not be negative-cached")
	}
	if !shouldNegativeCacheOpenRouterModelInfoError(openRouterModelsHTTPError{statusCode: http.StatusInternalServerError}) {
		t.Fatal("server failures should be negative-cached per credential")
	}
	if shouldNegativeCacheOpenRouterModelInfoError(context.DeadlineExceeded) {
		t.Fatal("context deadline failures should not be negative-cached")
	}
}

func TestLookupOpenRouterModelInfoSkipsAutoAndPlaceholderKeys(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
	}))
	defer server.Close()
	t.Setenv("OPENROUTER_API_BASE_URL", server.URL)

	if _, ok := LookupOpenRouterModelInfo(context.Background(), "dummy-openrouter-key-realish", "openrouter/auto"); ok {
		t.Fatal("openrouter/auto should not resolve to a concrete model")
	}
	if _, ok := LookupOpenRouterModelInfo(context.Background(), "sk-test-secret", "openai/gpt-4.1-mini"); ok {
		t.Fatal("placeholder API keys should skip model lookup")
	}
	if calls != 0 {
		t.Fatalf("skipped lookups should not call the endpoint, calls=%d", calls)
	}
}

func TestOpenRouterRequestTimeoutCanBeConfigured(t *testing.T) {
	if got := openRouterRequestTimeout(); got.Seconds() != 120 {
		t.Fatalf("unexpected default timeout: %s", got)
	}
	t.Setenv("OPENROUTER_REQUEST_TIMEOUT_SECONDS", "75")
	if got := openRouterRequestTimeout(); got.Seconds() != 75 {
		t.Fatalf("unexpected configured timeout: %s", got)
	}
	t.Setenv("OPENROUTER_REQUEST_TIMEOUT_SECONDS", "")
	t.Setenv("AI_GENERATION_SERVICE_REQUEST_TIMEOUT_SECONDS", "90")
	if got := openRouterRequestTimeout(); got.Seconds() != 90 {
		t.Fatalf("unexpected compatibility timeout: %s", got)
	}
}

func TestOpenRouterBaseURLGuard(t *testing.T) {
	for _, baseURL := range []string{
		"https://openrouter.example.test/api/v1",
		"http://localhost:1234",
		"http://127.0.0.1:1234",
		"http://[::1]:1234",
	} {
		if err := validateOpenRouterBaseURL(baseURL); err != nil {
			t.Fatalf("expected %s to be accepted: %v", baseURL, err)
		}
	}
	for _, baseURL := range []string{
		"http://openrouter.example.test/api/v1",
		"ftp://openrouter.example.test/api/v1",
		"not a url",
	} {
		if err := validateOpenRouterBaseURL(baseURL); err == nil {
			t.Fatalf("expected %s to be rejected", baseURL)
		}
	}
	t.Setenv("OPENROUTER_API_BASE_URL", "http://openrouter.example.test/api/v1")
	if got := openRouterChatCompletionsURL(); got != "" {
		t.Fatalf("invalid OpenRouter base URL should not produce an endpoint: %q", got)
	}
	if _, err := GenerateOpenRouterChat(context.Background(), OpenRouterConfig{
		APIKey:  "sk-test",
		ModelID: "openrouter/auto",
	}, []ChatMessage{{Role: "user", Content: "hello"}}); err == nil || !strings.Contains(err.Error(), "https") {
		t.Fatalf("expected invalid base URL error, got %v", err)
	}
}
