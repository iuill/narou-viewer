package ai

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const maxOpenRouterResponseBytes int64 = 2 << 20
const openRouterModelInfoCacheTTL = 6 * time.Hour
const openRouterModelInfoNegativeCacheTTL = 1 * time.Minute

type OpenRouterConfig struct {
	APIKey            string
	ModelID           string
	ProviderOrder     []string
	AllowFallbacks    bool
	RequireParameters bool
	ReasoningEffort   string
	Temperature       *float64
	MaxTokens         int
	ResponseFormat    any
}

type ChatMessage struct {
	Role             string          `json:"role"`
	Content          any             `json:"content,omitempty"`
	ToolCallID       string          `json:"tool_call_id,omitempty"`
	Name             string          `json:"name,omitempty"`
	ToolCalls        []ToolCall      `json:"tool_calls,omitempty"`
	Reasoning        json.RawMessage `json:"reasoning,omitempty"`
	ReasoningDetails json.RawMessage `json:"reasoning_details,omitempty"`
}

type ToolDefinition struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters,omitempty"`
}

type ToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function ToolCallFunction `json:"function"`
}

type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ChatResult struct {
	Answer           string
	InputTokens      int
	OutputTokens     int
	TotalTokens      int
	FinishReason     string
	ToolCalls        []ToolCall
	Reasoning        json.RawMessage
	ReasoningDetails json.RawMessage
}

type OpenRouterReasoningRequest struct {
	RequestedEffort   *string `json:"requestedEffort"`
	Source            string  `json:"source"`
	RequireParameters bool    `json:"requireParameters"`
}

type OpenRouterModelInfo struct {
	ID                  string
	ContextLength       int
	MaxCompletionTokens int
}

type openRouterModelInfoCacheEntry struct {
	info      OpenRouterModelInfo
	found     bool
	expiresAt time.Time
}

type openRouterModelsHTTPError struct {
	statusCode int
}

func (e openRouterModelsHTTPError) Error() string {
	return fmt.Sprintf("OpenRouter models endpoint responded with %d", e.statusCode)
}

var openRouterModelInfoCache sync.Map

var (
	ErrOpenRouterEmptyResponse     = errors.New("OpenRouter returned an empty response")
	ErrOpenRouterTruncatedResponse = errors.New("OpenRouter response was truncated")
)

func IsOpenRouterOutputError(err error) bool {
	return errors.Is(err, ErrOpenRouterEmptyResponse) || errors.Is(err, ErrOpenRouterTruncatedResponse)
}

func GenerateOpenRouterChat(ctx context.Context, config OpenRouterConfig, messages []ChatMessage) (ChatResult, error) {
	if strings.TrimSpace(config.APIKey) == "" || strings.TrimSpace(config.ModelID) == "" {
		return ChatResult{}, errors.New("OpenRouter API key and modelId are required")
	}
	body := map[string]any{
		"model":    strings.TrimSpace(config.ModelID),
		"messages": messages,
	}
	if config.Temperature != nil {
		body["temperature"] = *config.Temperature
	}
	if config.MaxTokens > 0 {
		body["max_tokens"] = config.MaxTokens
	}
	if config.ResponseFormat != nil {
		body["response_format"] = config.ResponseFormat
	}
	reasoningRequest, err := applyOpenRouterReasoning(body, config)
	if err != nil {
		return ChatResult{}, err
	}
	if len(config.ProviderOrder) > 0 || !config.AllowFallbacks || reasoningRequest.RequireParameters {
		body["provider"] = map[string]any{
			"order":              config.ProviderOrder,
			"allow_fallbacks":    config.AllowFallbacks,
			"require_parameters": reasoningRequest.RequireParameters,
		}
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return ChatResult{}, err
	}
	client := &http.Client{Timeout: openRouterRequestTimeout()}
	var lastErr error
	var accumulated ChatResult
	for attempt := 0; attempt < 3; attempt++ {
		result, retry, err := doOpenRouterChatRequest(ctx, client, config, raw)
		accumulated.InputTokens += result.InputTokens
		accumulated.OutputTokens += result.OutputTokens
		accumulated.TotalTokens += result.TotalTokens
		if !retry || err == nil {
			result.InputTokens = accumulated.InputTokens
			result.OutputTokens = accumulated.OutputTokens
			result.TotalTokens = accumulated.TotalTokens
			return result, err
		}
		lastErr = err
		if attempt == 2 {
			break
		}
		select {
		case <-ctx.Done():
			return accumulated, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return accumulated, lastErr
}

func GenerateOpenRouterToolChat(ctx context.Context, config OpenRouterConfig, messages []ChatMessage, tools []ToolDefinition) (ChatResult, error) {
	if strings.TrimSpace(config.APIKey) == "" || strings.TrimSpace(config.ModelID) == "" {
		return ChatResult{}, errors.New("OpenRouter API key and modelId are required")
	}
	body := map[string]any{
		"model":    strings.TrimSpace(config.ModelID),
		"messages": messages,
		"tools":    tools,
	}
	if config.Temperature != nil {
		body["temperature"] = *config.Temperature
	}
	if config.MaxTokens > 0 {
		body["max_tokens"] = config.MaxTokens
	}
	reasoningRequest, err := applyOpenRouterReasoning(body, config)
	if err != nil {
		return ChatResult{}, err
	}
	if len(config.ProviderOrder) > 0 || !config.AllowFallbacks || reasoningRequest.RequireParameters {
		body["provider"] = map[string]any{
			"order":              config.ProviderOrder,
			"allow_fallbacks":    config.AllowFallbacks,
			"require_parameters": reasoningRequest.RequireParameters,
		}
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return ChatResult{}, err
	}
	client := &http.Client{Timeout: openRouterRequestTimeout()}
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		result, retry, err := doOpenRouterChatRequest(ctx, client, config, raw)
		if !retry || err == nil {
			return result, err
		}
		lastErr = err
		if attempt == 2 {
			break
		}
		select {
		case <-ctx.Done():
			return ChatResult{}, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return ChatResult{}, lastErr
}

func NormalizeOpenRouterReasoningEffort(value string) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	// OpenRouter gateway values: https://openrouter.ai/docs/api/reference/parameters#reasoning-effort
	switch normalized {
	case "", "none", "minimal", "low", "medium", "high", "xhigh", "max":
		return normalized, true
	default:
		return "", false
	}
}

func ResolveOpenRouterReasoningRequest(config OpenRouterConfig) (OpenRouterReasoningRequest, error) {
	value := strings.TrimSpace(config.ReasoningEffort)
	source := "request"
	if value == "" {
		value = strings.TrimSpace(os.Getenv("OPENROUTER_REASONING_EFFORT"))
		source = "environment"
	}
	effort, ok := NormalizeOpenRouterReasoningEffort(value)
	if !ok {
		return OpenRouterReasoningRequest{}, fmt.Errorf("OpenRouter reasoning effort %q is invalid", value)
	}
	request := OpenRouterReasoningRequest{
		Source:            source,
		RequireParameters: config.RequireParameters || effort != "",
	}
	if effort == "" {
		request.Source = "provider-default"
		return request, nil
	}
	request.RequestedEffort = &effort
	return request, nil
}

func applyOpenRouterReasoning(body map[string]any, config OpenRouterConfig) (OpenRouterReasoningRequest, error) {
	request, err := ResolveOpenRouterReasoningRequest(config)
	if err != nil {
		return OpenRouterReasoningRequest{}, err
	}
	if request.RequestedEffort != nil {
		body["reasoning"] = map[string]any{"effort": *request.RequestedEffort}
	}
	return request, nil
}

func doOpenRouterChatRequest(ctx context.Context, client *http.Client, config OpenRouterConfig, raw []byte) (ChatResult, bool, error) {
	endpoint, err := openRouterChatCompletionsEndpoint()
	if err != nil {
		return ChatResult{}, false, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return ChatResult{}, false, err
	}
	request.Header.Set("content-type", "application/json")
	request.Header.Set("authorization", "Bearer "+strings.TrimSpace(config.APIKey))
	request.Header.Set("http-referer", openRouterHTTPReferer())
	request.Header.Set("x-openrouter-title", openRouterAppTitle())
	response, err := client.Do(request)
	if err != nil {
		return ChatResult{}, isRetryableOpenRouterTransportError(err), err
	}
	defer response.Body.Close()
	responseBody, err := readLimitedOpenRouterResponseBody(response)
	if err != nil {
		return ChatResult{}, isRetryableOpenRouterStatus(response.StatusCode), err
	}
	var decoded struct {
		Choices []struct {
			Message struct {
				Content          string          `json:"content"`
				ToolCalls        []ToolCall      `json:"tool_calls"`
				Reasoning        json.RawMessage `json:"reasoning"`
				ReasoningDetails json.RawMessage `json:"reasoning_details"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(responseBody, &decoded); err != nil {
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return ChatResult{}, isRetryableOpenRouterStatus(response.StatusCode), fmt.Errorf("OpenRouter responded with %d", response.StatusCode)
		}
		return ChatResult{}, false, err
	}
	result := ChatResult{
		InputTokens:  decoded.Usage.PromptTokens,
		OutputTokens: decoded.Usage.CompletionTokens,
		TotalTokens:  totalTokens(decoded.Usage.TotalTokens, decoded.Usage.PromptTokens, decoded.Usage.CompletionTokens),
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		if decoded.Error != nil && strings.TrimSpace(decoded.Error.Message) != "" {
			return result, isRetryableOpenRouterStatus(response.StatusCode), fmt.Errorf("OpenRouter responded with %d: %s", response.StatusCode, decoded.Error.Message)
		}
		return result, isRetryableOpenRouterStatus(response.StatusCode), fmt.Errorf("OpenRouter responded with %d", response.StatusCode)
	}
	if len(decoded.Choices) == 0 || (strings.TrimSpace(decoded.Choices[0].Message.Content) == "" && len(decoded.Choices[0].Message.ToolCalls) == 0) {
		return result, false, ErrOpenRouterEmptyResponse
	}
	finishReason := strings.TrimSpace(decoded.Choices[0].FinishReason)
	result.Answer = strings.TrimSpace(decoded.Choices[0].Message.Content)
	result.FinishReason = finishReason
	result.ToolCalls = decoded.Choices[0].Message.ToolCalls
	result.Reasoning = normalizeOptionalOpenRouterJSON(decoded.Choices[0].Message.Reasoning)
	result.ReasoningDetails = normalizeOptionalOpenRouterJSON(decoded.Choices[0].Message.ReasoningDetails)
	if isTruncatedOpenRouterFinishReason(finishReason) {
		return result, false, fmt.Errorf("%w: finish_reason=%s", ErrOpenRouterTruncatedResponse, finishReason)
	}
	return result, false, nil
}

func normalizeOptionalOpenRouterJSON(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil
	}
	return raw
}

func readLimitedOpenRouterResponseBody(response *http.Response) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxOpenRouterResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > maxOpenRouterResponseBytes {
		return nil, fmt.Errorf("OpenRouter response body exceeded %d bytes", maxOpenRouterResponseBytes)
	}
	return raw, nil
}

func isTruncatedOpenRouterFinishReason(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "length", "max_tokens", "content_filter":
		return true
	default:
		return false
	}
}

func isRetryableOpenRouterStatus(status int) bool {
	return status == http.StatusRequestTimeout || status == http.StatusTooManyRequests || status >= 500
}

func isRetryableOpenRouterTransportError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return false
	}
	return true
}

func totalTokens(total int, input int, output int) int {
	if total > 0 {
		return total
	}
	return input + output
}

func LookupOpenRouterModelInfo(ctx context.Context, apiKey string, modelID string, providerOrder ...[]string) (OpenRouterModelInfo, bool) {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" || modelID == "openrouter/auto" || isPlaceholderOpenRouterAPIKey(apiKey) {
		return OpenRouterModelInfo{}, false
	}
	providers := normalizeOpenRouterProviderFilter(providerOrder...)
	cacheKey := openRouterModelsURL() + "\x00" + modelID + "\x00" + strings.Join(providers, ",") + "\x00" + openRouterCredentialFingerprint(apiKey)
	now := time.Now()
	if cached, ok := openRouterModelInfoCache.Load(cacheKey); ok {
		entry := cached.(openRouterModelInfoCacheEntry)
		if now.Before(entry.expiresAt) {
			return entry.info, entry.found
		}
		openRouterModelInfoCache.Delete(cacheKey)
	}
	info, found, err := fetchOpenRouterModelInfo(ctx, apiKey, modelID, providers)
	if err != nil {
		if shouldNegativeCacheOpenRouterModelInfoError(err) {
			openRouterModelInfoCache.Store(cacheKey, openRouterModelInfoCacheEntry{
				found:     false,
				expiresAt: now.Add(openRouterModelInfoNegativeCacheTTL),
			})
		}
		return OpenRouterModelInfo{}, false
	}
	openRouterModelInfoCache.Store(cacheKey, openRouterModelInfoCacheEntry{
		info:      info,
		found:     found,
		expiresAt: now.Add(openRouterModelInfoCacheTTL),
	})
	return info, found
}

func openRouterCredentialFingerprint(apiKey string) string {
	trimmed := strings.TrimSpace(apiKey)
	if trimmed == "" {
		return "empty"
	}
	sum := sha1.Sum([]byte(trimmed))
	return hex.EncodeToString(sum[:])[:12]
}

func shouldNegativeCacheOpenRouterModelInfoError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return false
	}
	var httpErr openRouterModelsHTTPError
	if errors.As(err, &httpErr) {
		return httpErr.statusCode != http.StatusUnauthorized && httpErr.statusCode != http.StatusForbidden
	}
	return true
}

func isPlaceholderOpenRouterAPIKey(apiKey string) bool {
	trimmed := strings.TrimSpace(apiKey)
	return strings.HasPrefix(trimmed, "sk-test") ||
		strings.HasPrefix(trimmed, "sk-contract") ||
		strings.HasPrefix(trimmed, "sk-summary-secret")
}

func normalizeOpenRouterProviderFilter(providerOrder ...[]string) []string {
	if len(providerOrder) == 0 {
		return nil
	}
	seen := map[string]bool{}
	result := []string{}
	for _, values := range providerOrder {
		for _, value := range values {
			trimmed := strings.TrimSpace(value)
			if trimmed == "" || seen[trimmed] {
				continue
			}
			seen[trimmed] = true
			result = append(result, trimmed)
		}
	}
	return result
}

func fetchOpenRouterModelInfo(ctx context.Context, apiKey string, modelID string, providers []string) (OpenRouterModelInfo, bool, error) {
	endpoint, err := openRouterModelsEndpoint()
	if err != nil {
		return OpenRouterModelInfo{}, false, err
	}
	requestURL, err := url.Parse(endpoint)
	if err != nil {
		return OpenRouterModelInfo{}, false, err
	}
	query := requestURL.Query()
	query.Set("q", modelID)
	if len(providers) > 0 {
		query.Set("providers", strings.Join(providers, ","))
	}
	requestURL.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL.String(), nil)
	if err != nil {
		return OpenRouterModelInfo{}, false, err
	}
	if strings.TrimSpace(apiKey) != "" {
		request.Header.Set("authorization", "Bearer "+strings.TrimSpace(apiKey))
	}
	request.Header.Set("http-referer", openRouterHTTPReferer())
	request.Header.Set("x-openrouter-title", openRouterAppTitle())
	response, err := (&http.Client{Timeout: openRouterRequestTimeout()}).Do(request)
	if err != nil {
		return OpenRouterModelInfo{}, false, err
	}
	defer response.Body.Close()
	responseBody, err := readLimitedOpenRouterResponseBody(response)
	if err != nil {
		return OpenRouterModelInfo{}, false, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return OpenRouterModelInfo{}, false, openRouterModelsHTTPError{statusCode: response.StatusCode}
	}
	var decoded struct {
		Data []struct {
			ID            string `json:"id"`
			CanonicalSlug string `json:"canonical_slug"`
			ContextLength int    `json:"context_length"`
			TopProvider   struct {
				ContextLength       int `json:"context_length"`
				MaxCompletionTokens int `json:"max_completion_tokens"`
			} `json:"top_provider"`
			PerRequestLimits map[string]any `json:"per_request_limits"`
		} `json:"data"`
	}
	if err := json.Unmarshal(responseBody, &decoded); err != nil {
		return OpenRouterModelInfo{}, false, err
	}
	for _, model := range decoded.Data {
		if model.ID != modelID && model.CanonicalSlug != modelID {
			continue
		}
		contextLength := model.TopProvider.ContextLength
		if contextLength <= 0 {
			contextLength = model.ContextLength
		}
		if limit := positiveOpenRouterNumberLimit(model.PerRequestLimits, "context_length", "prompt_tokens", "max_prompt_tokens"); limit > 0 && (contextLength <= 0 || limit < contextLength) {
			contextLength = limit
		}
		maxCompletionTokens := model.TopProvider.MaxCompletionTokens
		if limit := positiveOpenRouterNumberLimit(model.PerRequestLimits, "max_completion_tokens", "completion_tokens", "max_tokens"); limit > 0 && (maxCompletionTokens <= 0 || limit < maxCompletionTokens) {
			maxCompletionTokens = limit
		}
		return OpenRouterModelInfo{
			ID:                  model.ID,
			ContextLength:       contextLength,
			MaxCompletionTokens: maxCompletionTokens,
		}, true, nil
	}
	return OpenRouterModelInfo{}, false, nil
}

func positiveOpenRouterNumberLimit(values map[string]any, keys ...string) int {
	if len(values) == 0 {
		return 0
	}
	for _, key := range keys {
		value, ok := values[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case float64:
			if typed > 0 {
				return int(typed)
			}
		case int:
			if typed > 0 {
				return typed
			}
		case json.Number:
			if parsed, err := strconv.Atoi(string(typed)); err == nil && parsed > 0 {
				return parsed
			}
		case string:
			if parsed, err := strconv.Atoi(strings.TrimSpace(typed)); err == nil && parsed > 0 {
				return parsed
			}
		}
	}
	return 0
}

func openRouterChatCompletionsURL() string {
	endpoint, err := openRouterChatCompletionsEndpoint()
	if err != nil {
		return ""
	}
	return endpoint
}

func openRouterModelsURL() string {
	endpoint, err := openRouterModelsEndpoint()
	if err != nil {
		return ""
	}
	return endpoint
}

func openRouterChatCompletionsEndpoint() (string, error) {
	baseURL, err := openRouterAPIBaseURL()
	if err != nil {
		return "", err
	}
	return baseURL + "/chat/completions", nil
}

func openRouterModelsEndpoint() (string, error) {
	baseURL, err := openRouterAPIBaseURL()
	if err != nil {
		return "", err
	}
	return baseURL + "/models", nil
}

func openRouterAPIBaseURL() (string, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("OPENROUTER_API_BASE_URL")), "/")
	if baseURL == "" {
		baseURL = "https://openrouter.ai/api/v1"
	}
	if err := validateOpenRouterBaseURL(baseURL); err != nil {
		return "", err
	}
	return baseURL, nil
}

func validateOpenRouterBaseURL(baseURL string) error {
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("OPENROUTER_API_BASE_URL must be an absolute HTTPS URL or local HTTP URL")
	}
	if parsed.Scheme == "https" {
		return nil
	}
	if parsed.Scheme == "http" && isLocalOpenRouterHost(parsed.Hostname()) {
		return nil
	}
	return fmt.Errorf("OPENROUTER_API_BASE_URL must use https unless it points to localhost")
}

func isLocalOpenRouterHost(host string) bool {
	switch strings.ToLower(strings.TrimSpace(host)) {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}

func openRouterRequestTimeout() time.Duration {
	for _, envName := range []string{"OPENROUTER_REQUEST_TIMEOUT_SECONDS", "AI_GENERATION_SERVICE_REQUEST_TIMEOUT_SECONDS"} {
		value := strings.TrimSpace(os.Getenv(envName))
		if value == "" {
			continue
		}
		seconds, err := strconv.Atoi(value)
		if err == nil && seconds > 0 {
			return time.Duration(seconds) * time.Second
		}
	}
	return 120 * time.Second
}

func openRouterHTTPReferer() string {
	value := strings.TrimSpace(os.Getenv("OPENROUTER_HTTP_REFERER"))
	if value != "" {
		return value
	}
	return "http://localhost"
}

func openRouterAppTitle() string {
	value := strings.TrimSpace(os.Getenv("OPENROUTER_APP_TITLE"))
	if value != "" {
		return value
	}
	return "narou-viewer"
}
