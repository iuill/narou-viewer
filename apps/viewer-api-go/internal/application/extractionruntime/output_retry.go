package extractionruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"narou-viewer/apps/viewer-api-go/internal/ai"
)

const (
	extractionOutputAttempts  = 2
	extractionOutputRetryWait = 250 * time.Millisecond
)

// generateOpenRouterChatWithOutputRetry retries only responses that reached the
// provider but failed the extraction output contract. Transport/status retries
// remain the responsibility of ai.GenerateOpenRouterChat.
func generateOpenRouterChatWithOutputRetry(
	ctx context.Context,
	config ai.OpenRouterConfig,
	messages []ai.ChatMessage,
	validate func(ai.ChatResult) error,
) (ai.ChatResult, int, error) {
	accumulated := ai.ChatResult{}
	var lastValidationErr error
	reachedAttempts := 0
	for attempt := 1; attempt <= extractionOutputAttempts; attempt++ {
		result, err := ai.GenerateOpenRouterChat(ctx, config, messages)
		reachedAttempts++
		accumulated.Answer = result.Answer
		accumulated.FinishReason = result.FinishReason
		accumulated.ToolCalls = result.ToolCalls
		accumulated.InputTokens += result.InputTokens
		accumulated.OutputTokens += result.OutputTokens
		accumulated.TotalTokens += result.TotalTokens
		if err != nil {
			if !ai.IsOpenRouterOutputError(err) {
				if result.InputTokens == 0 && result.OutputTokens == 0 && result.TotalTokens == 0 && result.Answer == "" {
					reachedAttempts--
				}
				return accumulated, reachedAttempts, err
			}
			lastValidationErr = err
		} else if validate == nil {
			return accumulated, reachedAttempts, nil
		} else if validationErr := validate(result); validationErr == nil {
			return accumulated, reachedAttempts, nil
		} else {
			lastValidationErr = validationErr
		}
		if attempt < extractionOutputAttempts {
			timer := time.NewTimer(extractionOutputRetryWait)
			select {
			case <-ctx.Done():
				if !timer.Stop() {
					<-timer.C
				}
				return accumulated, reachedAttempts, ctx.Err()
			case <-timer.C:
			}
		}
	}
	return accumulated, reachedAttempts, fmt.Errorf("モデル出力が%d回連続で抽出契約に適合しませんでした: %w", extractionOutputAttempts, lastValidationErr)
}

func decodeRequiredJSONArray(answer string, field string, target any) error {
	var object map[string]json.RawMessage
	if err := json.Unmarshal([]byte(answer), &object); err != nil {
		return err
	}
	raw, ok := object[field]
	if !ok || len(raw) == 0 || string(raw) == "null" || !strings.HasPrefix(strings.TrimSpace(string(raw)), "[") {
		return fmt.Errorf("モデル出力の %s は必須の配列です", field)
	}
	return json.Unmarshal(raw, target)
}

func validateRequiredJSONArrayItems(answer string, field string, requiredFields []string) error {
	var items []json.RawMessage
	if err := decodeRequiredJSONArray(answer, field, &items); err != nil {
		return err
	}
	for _, raw := range items {
		var item map[string]json.RawMessage
		if json.Unmarshal(raw, &item) != nil || len(item) != len(requiredFields) {
			return fmt.Errorf("モデル出力の %s に抽出契約外の項目があります", field)
		}
		for _, required := range requiredFields {
			if item[required] == nil || string(item[required]) == "null" {
				return fmt.Errorf("モデル出力の %s に必須field %s がありません", field, required)
			}
		}
	}
	return nil
}
