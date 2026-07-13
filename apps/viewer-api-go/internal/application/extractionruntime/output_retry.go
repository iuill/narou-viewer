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

func parallelIdentityArrayResponseFormat(name string, field string, itemSchema map[string]any) map[string]any {
	return map[string]any{
		"type": "json_schema",
		"json_schema": map[string]any{
			"name":   name,
			"strict": true,
			"schema": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"required":             []any{field},
				"properties": map[string]any{
					field: map[string]any{
						"type":  "array",
						"items": itemSchema,
					},
				},
			},
		},
	}
}

func parallelIdentityClusterResponseFormat() map[string]any {
	return parallelIdentityArrayResponseFormat("parallel_identity_clusters", "clusters", map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []any{"localIds", "canonicalName", "confidence", "reason"},
		"properties": map[string]any{
			"localIds": map[string]any{
				"type":     "array",
				"minItems": 2,
				"items":    map[string]any{"type": "string"},
			},
			"canonicalName": map[string]any{"type": "string"},
			"confidence":    map[string]any{"type": "number", "minimum": 0, "maximum": 1},
			"reason":        map[string]any{"type": "string"},
		},
	})
}

func parallelIdentityDiscoveryResponseFormat(allowedEpisodeIndexes ...string) map[string]any {
	episodeIndexSchema := map[string]any{"type": "string", "pattern": "^\\d+$"}
	episodeIndexValues := make([]any, 0, len(allowedEpisodeIndexes))
	seenEpisodeIndexes := map[string]bool{}
	for _, value := range allowedEpisodeIndexes {
		value = strings.TrimSpace(value)
		if value == "" || seenEpisodeIndexes[value] {
			continue
		}
		seenEpisodeIndexes[value] = true
		episodeIndexValues = append(episodeIndexValues, value)
	}
	if len(episodeIndexValues) > 0 {
		episodeIndexSchema = map[string]any{"type": "string", "enum": episodeIndexValues}
	}
	return parallelIdentityArrayResponseFormat("parallel_identity_discovery", "characters", map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []any{"name", "aliases", "episodeIndex", "reason"},
		"properties": map[string]any{
			"name": map[string]any{"type": "string"},
			"aliases": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"episodeIndex": episodeIndexSchema,
			"reason":       map[string]any{"type": "string"},
		},
	})
}

func parallelIdentityCorrectionResponseFormat() map[string]any {
	return parallelIdentityArrayResponseFormat("parallel_identity_correction", "characters", map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []any{"characterId", "canonicalName", "aliases", "keep", "reason"},
		"properties": map[string]any{
			"characterId":   map[string]any{"type": "string"},
			"canonicalName": map[string]any{"type": "string"},
			"aliases": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"keep":   map[string]any{"type": "boolean"},
			"reason": map[string]any{"type": "string"},
		},
	})
}
