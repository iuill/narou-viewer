package charactersummaryruntime

import (
	"context"
	"errors"
	"fmt"
	"time"

	"narou-viewer/apps/viewer-api-go/internal/ai"
	"narou-viewer/apps/viewer-api-go/internal/characters"
	core "narou-viewer/apps/viewer-api-go/internal/charactersummary"
	"narou-viewer/apps/viewer-api-go/internal/store"
)

type characterSummaryBatchResult struct {
	Delta characterSummaryDelta
	Usage ai.UsageRequest
}

func (r *Runtime) nextRuntimeBatch(ctx context.Context, config *store.ResolvedAIGenerationConfig, novelID string, upToEpisodeIndex string, knownCharacters []characters.GeneratedCharacter, template characterSummaryBatch, chunks []characterSummaryChunk, unresolvedMentions ...[]characters.GeneratedUnresolvedMention) (characterSummaryBatch, []characterSummaryChunk, error) {
	return core.PlanRuntimeBatch(template, chunks, func(batch characterSummaryBatch) (bool, error) {
		return r.batchFitsContext(ctx, config, novelID, upToEpisodeIndex, knownCharacters, batch, unresolvedMentions...)
	})
}

func (r *Runtime) batchFitsContext(ctx context.Context, config *store.ResolvedAIGenerationConfig, novelID string, upToEpisodeIndex string, knownCharacters []characters.GeneratedCharacter, batch characterSummaryBatch, unresolvedMentions ...[]characters.GeneratedUnresolvedMention) (bool, error) {
	if config == nil {
		return true, nil
	}
	stageStartedAt := time.Now()
	pending := []characters.GeneratedUnresolvedMention(nil)
	if len(unresolvedMentions) > 0 {
		pending = unresolvedMentions[0]
	}
	systemPrompt, userPrompt := buildCharacterSummaryPromptWithUnresolved(novelID, upToEpisodeIndex, knownCharacters, batch, pending, config.SystemPrompt)
	r.log("context_fit_prompt", stageStartedAt, "novelId", novelID, "upToEpisodeIndex", upToEpisodeIndex, "batch", batch.BatchIndex, "chunks", len(batch.Chunks), "knownCharacters", len(knownCharacters), "unresolvedMentions", len(pending))
	stageStartedAt = time.Now()
	responseFormat := characterSummaryOpenRouterResponseFormat()
	promptTokens := estimateOpenRouterChatRequestTokens([]ai.ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}, nil, responseFormat)
	r.log("context_fit_token_estimate", stageStartedAt, "novelId", novelID, "upToEpisodeIndex", upToEpisodeIndex, "batch", batch.BatchIndex, "promptTokens", promptTokens)
	stageStartedAt = time.Now()
	maxTokens, err := resolveOpenRouterMaxOutputTokens(ctx, config.APIKey, config.ModelID, config.ProviderOrder, characterSummaryDefaultMaxTokens, promptTokens)
	status := "ok"
	if err != nil {
		status = "error"
	}
	r.log("context_fit_max_tokens", stageStartedAt, "status", status, "novelId", novelID, "upToEpisodeIndex", upToEpisodeIndex, "batch", batch.BatchIndex, "promptTokens", promptTokens, "maxTokens", maxTokens)
	if err == nil {
		return maxTokens >= characterSummaryMinimumCompletionTokens, nil
	}
	if errors.Is(err, errOpenRouterContextTooLarge) {
		return false, nil
	}
	return false, err
}

func (r *Runtime) generateOpenRouterBatch(ctx context.Context, config *store.ResolvedAIGenerationConfig, novelID string, upToEpisodeIndex string, knownCharacters []characters.GeneratedCharacter, batch characterSummaryBatch, unresolvedMentions ...[]characters.GeneratedUnresolvedMention) (characterSummaryBatchResult, error) {
	temperature := 0.2
	stageStartedAt := time.Now()
	pending := []characters.GeneratedUnresolvedMention(nil)
	if len(unresolvedMentions) > 0 {
		pending = unresolvedMentions[0]
	}
	systemPrompt, userPrompt := buildCharacterSummaryPromptWithUnresolved(novelID, upToEpisodeIndex, knownCharacters, batch, pending, config.SystemPrompt)
	r.log("batch_prompt", stageStartedAt, "novelId", novelID, "upToEpisodeIndex", upToEpisodeIndex, "batch", batch.BatchIndex, "chunks", len(batch.Chunks), "knownCharacters", len(knownCharacters), "unresolvedMentions", len(pending))
	stageStartedAt = time.Now()
	responseFormat := characterSummaryOpenRouterResponseFormat()
	promptTokens := estimateOpenRouterChatRequestTokens([]ai.ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}, nil, responseFormat)
	r.log("batch_token_estimate", stageStartedAt, "novelId", novelID, "upToEpisodeIndex", upToEpisodeIndex, "batch", batch.BatchIndex, "promptTokens", promptTokens)
	stageStartedAt = time.Now()
	maxTokens, err := resolveOpenRouterMaxOutputTokens(ctx, config.APIKey, config.ModelID, config.ProviderOrder, characterSummaryDefaultMaxTokens, promptTokens)
	status := "ok"
	if err != nil {
		status = "error"
	}
	r.log("batch_max_tokens", stageStartedAt, "status", status, "novelId", novelID, "upToEpisodeIndex", upToEpisodeIndex, "batch", batch.BatchIndex, "promptTokens", promptTokens, "maxTokens", maxTokens)
	if err != nil {
		return characterSummaryBatchResult{}, err
	}
	if maxTokens < characterSummaryMinimumCompletionTokens {
		return characterSummaryBatchResult{}, fmt.Errorf("OpenRouter request has only %d output tokens available for character summary; at least %d are required.", maxTokens, characterSummaryMinimumCompletionTokens)
	}
	stageStartedAt = time.Now()
	result, err := ai.GenerateOpenRouterChat(ctx, ai.OpenRouterConfig{
		APIKey:            config.APIKey,
		ModelID:           config.ModelID,
		ProviderOrder:     config.ProviderOrder,
		AllowFallbacks:    config.AllowFallbacks,
		RequireParameters: config.RequireParameters,
		Temperature:       &temperature,
		MaxTokens:         maxTokens,
		ResponseFormat:    responseFormat,
	}, []ai.ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	})
	status = "ok"
	if err != nil {
		status = "error"
	}
	r.log("batch_openrouter", stageStartedAt, "status", status, "novelId", novelID, "upToEpisodeIndex", upToEpisodeIndex, "batch", batch.BatchIndex, "inputTokens", result.InputTokens, "outputTokens", result.OutputTokens, "totalTokens", result.TotalTokens)
	if err != nil {
		return characterSummaryBatchResult{}, err
	}
	stageStartedAt = time.Now()
	delta, err := normalizeCharacterSummaryOpenRouterResponse([]byte(result.Answer), novelID, upToEpisodeIndex)
	status = "ok"
	if err != nil {
		status = "error"
	}
	r.log("batch_normalize", stageStartedAt, "status", status, "novelId", novelID, "upToEpisodeIndex", upToEpisodeIndex, "batch", batch.BatchIndex, "answerChars", len([]rune(result.Answer)))
	if err != nil {
		return characterSummaryBatchResult{}, err
	}
	usage := characterSummaryUsageRequestForBatch(batch.BatchIndex-1, batch)
	if result.InputTokens > 0 {
		usage.InputTokens = result.InputTokens
	}
	usage.OutputTokens = result.OutputTokens
	if result.TotalTokens > 0 {
		usage.TotalTokens = result.TotalTokens
	} else {
		usage.TotalTokens = usage.InputTokens + usage.OutputTokens
	}
	return characterSummaryBatchResult{Delta: delta, Usage: usage}, nil
}

func characterSummaryUsageRequestForBatch(index int, batch characterSummaryBatch) ai.UsageRequest {
	inputTokens := 0
	for _, chunk := range batch.Chunks {
		inputTokens += estimateTokenCount(chunk.Text)
	}
	return ai.UsageRequest{
		RequestIndex: index,
		Kind:         "character_summary_batch",
		InputTokens:  inputTokens,
		TotalTokens:  inputTokens,
	}
}

func characterSummaryOpenRouterResponseFormat() map[string]any {
	textVersionSchema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []any{"text", "episodeIndex"},
		"properties": map[string]any{
			"text":         map[string]any{"type": "string"},
			"episodeIndex": map[string]any{"type": "string", "pattern": "^\\d+$"},
		},
	}
	historyVersionSchema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []any{"episodeIndex", "text"},
		"properties": map[string]any{
			"episodeIndex": map[string]any{"type": "string", "pattern": "^\\d+$"},
			"text":         map[string]any{"type": "string"},
		},
	}
	characterDeltaSchema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required": []any{
			"canonicalName",
			"fullName",
			"fullNameHistory",
			"gender",
			"genderHistory",
			"firstAppearanceEpisodeIndex",
			"aliases",
			"appearanceHistory",
			"personalityHistory",
			"summaryHistory",
		},
		"properties": map[string]any{
			"canonicalName":               textVersionSchema,
			"fullName":                    map[string]any{"anyOf": []any{map[string]any{"type": "null"}, textVersionSchema}},
			"fullNameHistory":             map[string]any{"type": "array", "items": textVersionSchema},
			"gender":                      map[string]any{"anyOf": []any{map[string]any{"type": "null"}, textVersionSchema}},
			"genderHistory":               map[string]any{"type": "array", "items": textVersionSchema},
			"firstAppearanceEpisodeIndex": map[string]any{"type": "string", "pattern": "^\\d+$"},
			"aliases":                     map[string]any{"type": "array", "items": textVersionSchema},
			"appearanceHistory":           map[string]any{"type": "array", "items": historyVersionSchema},
			"personalityHistory":          map[string]any{"type": "array", "items": historyVersionSchema},
			"summaryHistory":              map[string]any{"type": "array", "items": historyVersionSchema},
		},
	}
	characterUpdateSchema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required": []any{
			"characterId",
			"canonicalName",
			"fullName",
			"fullNameHistory",
			"gender",
			"genderHistory",
			"firstAppearanceEpisodeIndex",
			"aliases",
			"appearanceHistory",
			"personalityHistory",
			"summaryHistory",
		},
		"properties": map[string]any{
			"characterId":                 map[string]any{"type": "string"},
			"canonicalName":               map[string]any{"anyOf": []any{map[string]any{"type": "null"}, textVersionSchema}},
			"fullName":                    map[string]any{"anyOf": []any{map[string]any{"type": "null"}, textVersionSchema}},
			"fullNameHistory":             map[string]any{"type": "array", "items": textVersionSchema},
			"gender":                      map[string]any{"anyOf": []any{map[string]any{"type": "null"}, textVersionSchema}},
			"genderHistory":               map[string]any{"type": "array", "items": textVersionSchema},
			"firstAppearanceEpisodeIndex": map[string]any{"type": "string", "pattern": "^\\d+$"},
			"aliases":                     map[string]any{"type": "array", "items": textVersionSchema},
			"appearanceHistory":           map[string]any{"type": "array", "items": historyVersionSchema},
			"personalityHistory":          map[string]any{"type": "array", "items": historyVersionSchema},
			"summaryHistory":              map[string]any{"type": "array", "items": historyVersionSchema},
		},
	}
	return map[string]any{
		"type": "json_schema",
		"json_schema": map[string]any{
			"name":   "character_summary_delta_result",
			"strict": true,
			"schema": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"required":             []any{"processedUpToEpisodeIndex", "newCharacters", "characterUpdates", "mergeProposals", "unresolvedMentions"},
				"properties": map[string]any{
					"processedUpToEpisodeIndex": map[string]any{"type": "string", "pattern": "^\\d+$"},
					"newCharacters": map[string]any{
						"type":  "array",
						"items": characterDeltaSchema,
					},
					"characterUpdates": map[string]any{
						"type":  "array",
						"items": characterUpdateSchema,
					},
					"mergeProposals": map[string]any{
						"type": "array",
						"items": map[string]any{
							"type":                 "object",
							"additionalProperties": false,
							"required":             []any{"sourceCharacterId", "targetCharacterId", "confidence", "reason"},
							"properties": map[string]any{
								"sourceCharacterId": map[string]any{"type": "string"},
								"targetCharacterId": map[string]any{"type": "string"},
								"confidence":        map[string]any{"type": "number"},
								"reason":            map[string]any{"type": "string"},
							},
						},
					},
					"unresolvedMentions": map[string]any{
						"type": "array",
						"items": map[string]any{
							"type":                 "object",
							"additionalProperties": false,
							"required":             []any{"mention", "episodeIndex", "reason"},
							"properties": map[string]any{
								"mention":      map[string]any{"type": "string"},
								"episodeIndex": map[string]any{"type": "string", "pattern": "^\\d+$"},
								"reason":       map[string]any{"type": "string"},
							},
						},
					},
				},
			},
		},
	}
}
