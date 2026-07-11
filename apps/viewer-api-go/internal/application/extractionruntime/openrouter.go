package extractionruntime

import (
	"context"
	"errors"
	"fmt"
	"time"

	"narou-viewer/apps/viewer-api-go/internal/ai"
	"narou-viewer/apps/viewer-api-go/internal/characters"
	core "narou-viewer/apps/viewer-api-go/internal/extraction"
	"narou-viewer/apps/viewer-api-go/internal/store"
	"narou-viewer/apps/viewer-api-go/internal/terms"
)

type extractionBatchResult struct {
	Delta extractionDelta
	Usage ai.UsageRequest
}

func (r *Runtime) nextRuntimeBatch(ctx context.Context, config *store.ResolvedAIGenerationConfig, novelID string, upToEpisodeIndex string, knownCharacters []characters.GeneratedCharacter, knownTerms []terms.GeneratedTerm, template extractionBatch, chunks []extractionChunk, unresolvedMentions ...[]characters.GeneratedUnresolvedMention) (extractionBatch, []extractionChunk, error) {
	return r.nextRuntimeBatchForFocus(ctx, config, novelID, upToEpisodeIndex, knownCharacters, knownTerms, template, chunks, "", unresolvedMentions...)
}

func (r *Runtime) nextRuntimeBatchForFocus(ctx context.Context, config *store.ResolvedAIGenerationConfig, novelID string, upToEpisodeIndex string, knownCharacters []characters.GeneratedCharacter, knownTerms []terms.GeneratedTerm, template extractionBatch, chunks []extractionChunk, focus string, unresolvedMentions ...[]characters.GeneratedUnresolvedMention) (extractionBatch, []extractionChunk, error) {
	return core.PlanRuntimeBatch(template, chunks, func(batch extractionBatch) (bool, error) {
		return r.batchFitsContextForFocus(ctx, config, novelID, upToEpisodeIndex, knownCharacters, knownTerms, batch, focus, unresolvedMentions...)
	})
}

func (r *Runtime) batchFitsContext(ctx context.Context, config *store.ResolvedAIGenerationConfig, novelID string, upToEpisodeIndex string, knownCharacters []characters.GeneratedCharacter, knownTerms []terms.GeneratedTerm, batch extractionBatch, unresolvedMentions ...[]characters.GeneratedUnresolvedMention) (bool, error) {
	return r.batchFitsContextForFocus(ctx, config, novelID, upToEpisodeIndex, knownCharacters, knownTerms, batch, "", unresolvedMentions...)
}

func (r *Runtime) batchFitsContextForFocus(ctx context.Context, config *store.ResolvedAIGenerationConfig, novelID string, upToEpisodeIndex string, knownCharacters []characters.GeneratedCharacter, knownTerms []terms.GeneratedTerm, batch extractionBatch, focus string, unresolvedMentions ...[]characters.GeneratedUnresolvedMention) (bool, error) {
	if config == nil {
		return true, nil
	}
	stageStartedAt := time.Now()
	pending := []characters.GeneratedUnresolvedMention(nil)
	if len(unresolvedMentions) > 0 {
		pending = unresolvedMentions[0]
	}
	prepared := prepareExtractionRequest(config, novelID, upToEpisodeIndex, knownCharacters, knownTerms, batch, pending, focus)
	r.log("context_fit_prompt", stageStartedAt, "novelId", novelID, "upToEpisodeIndex", upToEpisodeIndex, "batch", batch.BatchIndex, "chunks", len(batch.Chunks), "knownCharacters", len(knownCharacters), "knownTerms", len(knownTerms), "unresolvedMentions", len(pending))
	stageStartedAt = time.Now()
	promptTokens := prepared.PromptTokens
	r.log("context_fit_token_estimate", stageStartedAt, "novelId", novelID, "upToEpisodeIndex", upToEpisodeIndex, "batch", batch.BatchIndex, "promptTokens", promptTokens)
	stageStartedAt = time.Now()
	maxTokens, err := resolveOpenRouterMaxOutputTokens(ctx, config.APIKey, config.ModelID, config.ProviderOrder, extractionDefaultMaxTokens, promptTokens)
	status := "ok"
	if err != nil {
		status = "error"
	}
	r.log("context_fit_max_tokens", stageStartedAt, "status", status, "novelId", novelID, "upToEpisodeIndex", upToEpisodeIndex, "batch", batch.BatchIndex, "promptTokens", promptTokens, "maxTokens", maxTokens)
	if err == nil {
		return maxTokens >= extractionMinimumCompletionTokens, nil
	}
	if errors.Is(err, errOpenRouterContextTooLarge) {
		return false, nil
	}
	return false, err
}

func (r *Runtime) generateOpenRouterBatch(ctx context.Context, config *store.ResolvedAIGenerationConfig, novelID string, upToEpisodeIndex string, knownCharacters []characters.GeneratedCharacter, knownTerms []terms.GeneratedTerm, batch extractionBatch, unresolvedMentions ...[]characters.GeneratedUnresolvedMention) (extractionBatchResult, error) {
	return r.generateOpenRouterBatchForFocus(ctx, config, novelID, upToEpisodeIndex, knownCharacters, knownTerms, batch, "", unresolvedMentions...)
}

func (r *Runtime) generateOpenRouterBatchForFocus(ctx context.Context, config *store.ResolvedAIGenerationConfig, novelID string, upToEpisodeIndex string, knownCharacters []characters.GeneratedCharacter, knownTerms []terms.GeneratedTerm, batch extractionBatch, focus string, unresolvedMentions ...[]characters.GeneratedUnresolvedMention) (extractionBatchResult, error) {
	temperature := 0.2
	stageStartedAt := time.Now()
	pending := []characters.GeneratedUnresolvedMention(nil)
	if len(unresolvedMentions) > 0 {
		pending = unresolvedMentions[0]
	}
	prepared := prepareExtractionRequest(config, novelID, upToEpisodeIndex, knownCharacters, knownTerms, batch, pending, focus)
	r.log("batch_prompt", stageStartedAt, "novelId", novelID, "upToEpisodeIndex", upToEpisodeIndex, "batch", batch.BatchIndex, "chunks", len(batch.Chunks), "knownCharacters", len(knownCharacters), "knownTerms", len(knownTerms), "unresolvedMentions", len(pending))
	stageStartedAt = time.Now()
	promptTokens := prepared.PromptTokens
	r.log("batch_token_estimate", stageStartedAt, "novelId", novelID, "upToEpisodeIndex", upToEpisodeIndex, "batch", batch.BatchIndex, "promptTokens", promptTokens)
	stageStartedAt = time.Now()
	maxTokens, err := resolveOpenRouterMaxOutputTokens(ctx, config.APIKey, config.ModelID, config.ProviderOrder, extractionDefaultMaxTokens, promptTokens)
	status := "ok"
	if err != nil {
		status = "error"
	}
	r.log("batch_max_tokens", stageStartedAt, "status", status, "novelId", novelID, "upToEpisodeIndex", upToEpisodeIndex, "batch", batch.BatchIndex, "promptTokens", promptTokens, "maxTokens", maxTokens)
	if err != nil {
		return extractionBatchResult{}, err
	}
	if maxTokens < extractionMinimumCompletionTokens {
		return extractionBatchResult{}, fmt.Errorf("OpenRouter request has only %d output tokens available for extraction; at least %d are required.", maxTokens, extractionMinimumCompletionTokens)
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
		ResponseFormat:    prepared.ResponseFormat,
	}, prepared.Messages)
	usage := extractionUsageRequestForBatch(batch.BatchIndex-1, batch)
	if result.InputTokens > 0 {
		usage.InputTokens = result.InputTokens
	}
	usage.OutputTokens = result.OutputTokens
	if result.TotalTokens > 0 {
		usage.TotalTokens = result.TotalTokens
	} else {
		usage.TotalTokens = usage.InputTokens + usage.OutputTokens
	}
	status = "ok"
	if err != nil {
		status = "error"
	}
	r.log("batch_openrouter", stageStartedAt, "status", status, "novelId", novelID, "upToEpisodeIndex", upToEpisodeIndex, "batch", batch.BatchIndex, "inputTokens", result.InputTokens, "outputTokens", result.OutputTokens, "totalTokens", result.TotalTokens)
	if err != nil {
		if result.InputTokens == 0 && result.OutputTokens == 0 && result.TotalTokens == 0 {
			return extractionBatchResult{}, err
		}
		return extractionBatchResult{Usage: usage}, err
	}
	stageStartedAt = time.Now()
	fallbackEpisodeIndex := upToEpisodeIndex
	if len(batch.EpisodeIndexes) > 0 {
		fallbackEpisodeIndex = batch.EpisodeIndexes[len(batch.EpisodeIndexes)-1]
	}
	delta, err := normalizeExtractionOpenRouterResponseForEpisodes([]byte(result.Answer), novelID, fallbackEpisodeIndex, batch.EpisodeIndexes)
	status = "ok"
	if err != nil {
		status = "error"
	}
	r.log("batch_normalize", stageStartedAt, "status", status, "novelId", novelID, "upToEpisodeIndex", upToEpisodeIndex, "batch", batch.BatchIndex, "answerChars", len([]rune(result.Answer)))
	if err != nil {
		return extractionBatchResult{Usage: usage}, err
	}
	return extractionBatchResult{Delta: delta, Usage: usage}, nil
}

type preparedExtractionRequest struct {
	Messages       []ai.ChatMessage
	ResponseFormat map[string]any
	PromptTokens   int
}

func prepareExtractionRequest(config *store.ResolvedAIGenerationConfig, novelID string, upToEpisodeIndex string, knownCharacters []characters.GeneratedCharacter, knownTerms []terms.GeneratedTerm, batch extractionBatch, unresolved []characters.GeneratedUnresolvedMention, focus ...string) preparedExtractionRequest {
	var systemPromptOverride *string
	if config != nil {
		systemPromptOverride = config.SystemPrompt
	}
	systemPrompt, userPrompt := buildExtractionPromptWithUnresolved(novelID, upToEpisodeIndex, knownCharacters, knownTerms, batch, unresolved, systemPromptOverride)
	if len(focus) > 0 && focus[0] == "parallel_entities" {
		systemPrompt += "\n並列抽出では人物と用語を同じレスポンスで必ず抽出してください。この場合だけ、term の descriptionHistory は累積 snapshot ではなく、今回の episodes で新しく明示された事実だけを書いてください。過去や未来の本文から補完しないでください。"
	}
	messages := []ai.ChatMessage{{Role: "system", Content: systemPrompt}, {Role: "user", Content: userPrompt}}
	responseFormat := extractionOpenRouterResponseFormat()
	return preparedExtractionRequest{
		Messages:       messages,
		ResponseFormat: responseFormat,
		PromptTokens:   estimateOpenRouterChatRequestTokens(messages, nil, responseFormat),
	}
}

func extractionUsageRequestForBatch(index int, batch extractionBatch) ai.UsageRequest {
	inputTokens := 0
	for _, chunk := range batch.Chunks {
		inputTokens += estimateTokenCount(chunk.Text)
	}
	return ai.UsageRequest{
		RequestIndex: index,
		Kind:         "extraction_batch",
		InputTokens:  inputTokens,
		TotalTokens:  inputTokens,
	}
}

func extractionOpenRouterResponseFormat() map[string]any {
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
			"firstAppearanceEpisodeIndex": map[string]any{"type": "null"},
			"aliases":                     map[string]any{"type": "array", "items": textVersionSchema},
			"appearanceHistory":           map[string]any{"type": "array", "items": historyVersionSchema},
			"personalityHistory":          map[string]any{"type": "array", "items": historyVersionSchema},
			"summaryHistory":              map[string]any{"type": "array", "items": historyVersionSchema},
		},
	}
	termVersionEpisode := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []any{"text", "episodeIndex"},
		"properties": map[string]any{
			"text":         map[string]any{"type": "string"},
			"episodeIndex": map[string]any{"type": "string", "pattern": "^\\d+$"},
		},
	}
	termCategory := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []any{"value", "episodeIndex"},
		"properties": map[string]any{
			"value": map[string]any{
				"type": "string",
				"enum": []any{"organization", "place", "item", "skill", "race", "event", "other"},
			},
			"episodeIndex": map[string]any{"type": "string", "pattern": "^\\d+$"},
		},
	}
	termSchema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []any{"term", "reading", "category", "descriptionHistory"},
		"properties": map[string]any{
			"term":     map[string]any{"type": "string"},
			"reading":  map[string]any{"anyOf": []any{map[string]any{"type": "null"}, termVersionEpisode}},
			"category": termCategory,
			"descriptionHistory": map[string]any{
				"type":     "array",
				"minItems": 1,
				"items":    historyVersionSchema,
			},
		},
	}
	return map[string]any{
		"type": "json_schema",
		"json_schema": map[string]any{
			"name":   "extraction_delta_result",
			"strict": true,
			"schema": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"required":             []any{"processedUpToEpisodeIndex", "newCharacters", "characterUpdates", "mergeProposals", "unresolvedMentions", "terms"},
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
					"terms": map[string]any{
						"type":  "array",
						"items": termSchema,
					},
				},
			},
		},
	}
}

func ExtractionOpenRouterResponseFormat() map[string]any {
	return extractionOpenRouterResponseFormat()
}
