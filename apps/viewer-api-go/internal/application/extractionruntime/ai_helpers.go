package extractionruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"narou-viewer/apps/viewer-api-go/internal/ai"
	core "narou-viewer/apps/viewer-api-go/internal/extraction"
	"narou-viewer/apps/viewer-api-go/internal/store"
)

var errOpenRouterContextTooLarge = errors.New("OpenRouter context length is too small")

func maxInt(left int, right int) int {
	if left > right {
		return left
	}
	return right
}

func estimateTokenCount(value string) int {
	trimmed := strings.TrimSpace(value)
	runes := len([]rune(trimmed))
	if runes == 0 {
		return 0
	}
	return maxInt(runes, (len(trimmed)+3)/4)
}

func estimateChatMessagesTokenCount(messages []ai.ChatMessage) int {
	total := 0
	for _, message := range messages {
		total += estimateTokenCount(message.Role)
		switch content := message.Content.(type) {
		case string:
			total += estimateTokenCount(content)
		case nil:
		default:
			raw, err := json.Marshal(content)
			if err == nil {
				total += estimateTokenCount(string(raw))
			}
		}
		for _, toolCall := range message.ToolCalls {
			total += estimateTokenCount(toolCall.Function.Name)
			total += estimateTokenCount(toolCall.Function.Arguments)
		}
	}
	return total
}

func estimateOpenRouterChatRequestTokens(messages []ai.ChatMessage, tools []ai.ToolDefinition, responseFormat any) int {
	payload := map[string]any{
		"messages": messages,
	}
	if len(tools) > 0 {
		payload["tools"] = tools
	}
	if responseFormat != nil {
		payload["response_format"] = responseFormat
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return estimateChatMessagesTokenCount(messages)
	}
	return estimateTokenCount(string(raw))
}

func resolveOpenRouterMaxOutputTokens(ctx context.Context, apiKey string, modelID string, providerOrder []string, fallback int, promptTokens int) (int, error) {
	if fallback <= 0 {
		fallback = 4096
	}
	maxTokens := fallback
	lookupCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	info, ok := ai.LookupOpenRouterModelInfo(lookupCtx, apiKey, modelID, providerOrder)
	cancel()
	if ok {
		if info.MaxCompletionTokens > 0 {
			maxTokens = info.MaxCompletionTokens
		}
		if info.ContextLength > 0 && promptTokens > 0 {
			available := info.ContextLength - promptTokens - 256
			if available <= 0 {
				return 0, fmt.Errorf("%w: prompt estimate %d tokens is too large for context length %d. Reduce conversation history, tool results, or target text.", errOpenRouterContextTooLarge, promptTokens, info.ContextLength)
			}
			if available < maxTokens {
				maxTokens = available
			}
		}
	}
	if maxTokens < 1 {
		return 0, errors.New("OpenRouter max_tokens could not be resolved.")
	}
	return maxTokens, nil
}

func positiveEnvInt(name string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func truncateRunes(value string, limit int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit])
}

func resolveExtractionBatchBudget(ctx context.Context, config *store.ResolvedAIGenerationConfig, fallbackMaxBatchChars int) extractionBatchBudget {
	fallbackTokens := core.TokensFromChars(fallbackMaxBatchChars)
	if configuredTokens := positiveEnvInt("CHARACTER_SUMMARY_MAX_BATCH_TOKENS", 0); configuredTokens > 0 {
		return extractionBatchBudget{MaxTextTokens: configuredTokens}
	}
	budget := extractionBatchBudget{MaxTextChars: fallbackMaxBatchChars, MaxTextTokens: fallbackTokens}
	if config == nil {
		return budget
	}
	lookupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	info, ok := ai.LookupOpenRouterModelInfo(lookupCtx, config.APIKey, config.ModelID, config.ProviderOrder)
	cancel()
	if !ok || info.ContextLength <= 0 {
		return budget
	}
	return core.ResolveBatchBudget(fallbackMaxBatchChars, info.ContextLength, info.MaxCompletionTokens)
}
