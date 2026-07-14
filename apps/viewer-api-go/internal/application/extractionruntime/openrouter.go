package extractionruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
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

func (r *Runtime) nextRuntimeBatch(ctx context.Context, config *store.ResolvedAIGenerationConfig, novelID string, upToEpisodeIndex string, knownCharacters []characters.GeneratedCharacter, knownTerms []terms.GeneratedTerm, template extractionBatch, chunks []extractionChunk, unresolvedMentions []characters.GeneratedUnresolvedMention, identityMergeEventSets ...[]characters.GeneratedIdentityMergeEvent) (extractionBatch, []extractionChunk, error) {
	return r.nextRuntimeBatchForFocus(ctx, config, novelID, upToEpisodeIndex, knownCharacters, knownTerms, template, chunks, "", unresolvedMentions, identityMergeEventSets...)
}

func (r *Runtime) nextRuntimeBatchForFocus(ctx context.Context, config *store.ResolvedAIGenerationConfig, novelID string, upToEpisodeIndex string, knownCharacters []characters.GeneratedCharacter, knownTerms []terms.GeneratedTerm, template extractionBatch, chunks []extractionChunk, focus string, unresolvedMentions []characters.GeneratedUnresolvedMention, identityMergeEventSets ...[]characters.GeneratedIdentityMergeEvent) (extractionBatch, []extractionChunk, error) {
	identityMergeEvents := []characters.GeneratedIdentityMergeEvent(nil)
	if len(identityMergeEventSets) > 0 {
		identityMergeEvents = identityMergeEventSets[0]
	}
	return core.PlanRuntimeBatch(template, chunks, func(batch extractionBatch) (bool, error) {
		projectedUnresolved := filterGeneratedUnresolvedAtBoundary(unresolvedMentions, extractionBatchBoundary(batch))
		projectedUnresolved = core.ApplyIdentityMergeEventsToUnresolvedMentions(projectedUnresolved, identityMergeEvents, extractionBatchBoundary(batch))
		return r.batchFitsContextForFocus(ctx, config, novelID, upToEpisodeIndex, knownCharacters, knownTerms, batch, focus, projectedUnresolved)
	})
}

func (r *Runtime) batchFitsContextForFocus(ctx context.Context, config *store.ResolvedAIGenerationConfig, novelID string, upToEpisodeIndex string, knownCharacters []characters.GeneratedCharacter, knownTerms []terms.GeneratedTerm, batch extractionBatch, focus string, unresolvedMentions ...[]characters.GeneratedUnresolvedMention) (bool, error) {
	if config == nil {
		return true, nil
	}
	if !core.FitsStructuredOutputEpisodeIndexEnum(batch.EpisodeIndexes) {
		return false, nil
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
	var delta extractionDelta
	fallbackEpisodeIndex := upToEpisodeIndex
	if len(batch.EpisodeIndexes) > 0 {
		fallbackEpisodeIndex = batch.EpisodeIndexes[len(batch.EpisodeIndexes)-1]
	}
	result, outputAttempts, err := generateOpenRouterChatWithOutputRetry(ctx, ai.OpenRouterConfig{
		APIKey:            config.APIKey,
		ModelID:           config.ModelID,
		ProviderOrder:     config.ProviderOrder,
		AllowFallbacks:    config.AllowFallbacks,
		RequireParameters: config.RequireParameters,
		ReasoningEffort:   config.ReasoningEffort,
		Temperature:       &temperature,
		MaxTokens:         maxTokens,
		ResponseFormat:    prepared.ResponseFormat,
	}, prepared.Messages, func(result ai.ChatResult) error {
		normalizedAnswer, scalarErr := normalizeExtractionEpisodeIndexScalars([]byte(result.Answer), fallbackEpisodeIndex, prepared.ModelToActualEpisodeIndex)
		if scalarErr != nil {
			return scalarErr
		}
		if contractErr := validateExtractionOutputContract(normalizedAnswer); contractErr != nil {
			return contractErr
		}
		normalized, normalizeErr := normalizeExtractionOpenRouterResponseForEpisodes(normalizedAnswer, novelID, fallbackEpisodeIndex, batch.EpisodeIndexes)
		if normalizeErr == nil {
			delta = normalized
		}
		return normalizeErr
	})
	usage := extractionUsageRequestForBatch(batch.BatchIndex-1, batch)
	usedInputFallback := result.InputTokens <= 0
	if result.InputTokens > 0 {
		usage.InputTokens = result.InputTokens
	} else {
		usage.InputTokens = promptTokens * outputAttempts
	}
	usage.OutputTokens = result.OutputTokens
	if result.TotalTokens > 0 && !usedInputFallback {
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
		if outputAttempts <= 0 {
			return extractionBatchResult{}, err
		}
		return extractionBatchResult{Usage: usage}, err
	}
	stageStartedAt = time.Now()
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

func normalizeExtractionEpisodeIndexScalars(raw []byte, processedUpToEpisodeIndex string, modelToActualEpisodeIndex ...map[string]string) ([]byte, error) {
	if !json.Valid(raw) {
		return nil, errors.New("OpenRouter response was not valid JSON.")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return nil, err
	}
	normalizeExtractionEpisodeIndexValue(decoded)
	if len(modelToActualEpisodeIndex) > 0 {
		expandExtractionEpisodeIndexReferences(decoded, modelToActualEpisodeIndex[0])
	}
	if root, ok := decoded.(map[string]any); ok && isDigitsEpisodeIndex(processedUpToEpisodeIndex) {
		// The processed frontier is batch metadata already known by the server,
		// not an extracted fact. In json_object fallback mode some models omit it
		// or render it as prose, so do not make successful entity extraction
		// depend on the model echoing this value correctly.
		processedUpToEpisodeIndex = strings.TrimSpace(processedUpToEpisodeIndex)
		root["processedUpToEpisodeIndex"] = processedUpToEpisodeIndex
	}
	return json.Marshal(decoded)
}

func expandExtractionEpisodeIndexReferences(value any, modelToActual map[string]string) {
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			if key == "episodeIndex" || strings.HasSuffix(key, "EpisodeIndex") {
				if modelEpisodeIndex, ok := item.(string); ok {
					if actualEpisodeIndex, found := modelToActual[strings.TrimSpace(modelEpisodeIndex)]; found {
						typed[key] = actualEpisodeIndex
						continue
					}
				}
			}
			expandExtractionEpisodeIndexReferences(item, modelToActual)
		}
	case []any:
		for _, item := range typed {
			expandExtractionEpisodeIndexReferences(item, modelToActual)
		}
	}
}

func normalizeExtractionEpisodeIndexValue(value any) {
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			if number, ok := item.(json.Number); ok && (key == "episodeIndex" || strings.HasSuffix(key, "EpisodeIndex")) && isDigitsEpisodeIndex(string(number)) {
				typed[key] = string(number)
				continue
			}
			normalizeExtractionEpisodeIndexValue(item)
		}
	case []any:
		for _, item := range typed {
			normalizeExtractionEpisodeIndexValue(item)
		}
	}
}

func validateExtractionOutputContract(raw []byte) error {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(raw, &root); err != nil {
		return err
	}
	if root["terms"] == nil || string(root["terms"]) == "null" {
		return errors.New("OpenRouter response did not match the expected extraction schema.")
	}
	if root["newCharacters"] == nil {
		var legacyCharacters []json.RawMessage
		var legacyTerms []json.RawMessage
		if root["characters"] == nil || json.Unmarshal(root["characters"], &legacyCharacters) != nil || legacyCharacters == nil || root["terms"] == nil || json.Unmarshal(root["terms"], &legacyTerms) != nil || legacyTerms == nil {
			return errors.New("モデル出力が抽出契約のroot fieldsと一致しません")
		}
		return nil
	}
	var processed string
	if json.Unmarshal(root["processedUpToEpisodeIndex"], &processed) != nil || !isDigitsEpisodeIndex(processed) {
		return errors.New("モデル出力の processedUpToEpisodeIndex が不正です")
	}
	for _, field := range []string{"newCharacters", "characterUpdates", "mergeProposals", "unresolvedMentions", "terms"} {
		var items []json.RawMessage
		if root[field] == nil || json.Unmarshal(root[field], &items) != nil || items == nil {
			return fmt.Errorf("モデル出力の %s は必須の配列です", field)
		}
		switch field {
		case "newCharacters":
			for _, item := range items {
				if err := validateExtractionCharacterItem(item, false); err != nil {
					return err
				}
			}
		case "characterUpdates":
			for _, item := range items {
				if err := validateExtractionCharacterItem(item, true); err != nil {
					return err
				}
			}
		case "mergeProposals":
			for _, item := range items {
				if err := validateExtractionSimpleObject(item, []string{"sourceCharacterId", "targetCharacterId", "confidence", "reason"}); err != nil {
					return errors.New("モデル出力の mergeProposals に不正な項目があります")
				}
			}
		case "unresolvedMentions":
			for _, item := range items {
				if err := validateExtractionSimpleObject(item, []string{"mention", "episodeIndex", "reason"}); err != nil {
					return errors.New("モデル出力の unresolvedMentions に不正な項目があります")
				}
			}
		}
	}
	return nil
}

func validateExtractionCharacterItem(raw json.RawMessage, update bool) error {
	var item map[string]json.RawMessage
	if json.Unmarshal(raw, &item) != nil {
		return errors.New("モデル出力の人物項目がobjectではありません")
	}
	required := []string{"canonicalName", "fullName", "fullNameHistory", "gender", "genderHistory", "firstAppearanceEpisodeIndex", "aliases", "appearanceHistory", "personalityHistory", "summaryHistory"}
	if update {
		required = append(required, "characterId")
	}
	if len(item) != len(required) {
		requiredSet := make(map[string]bool, len(required))
		for _, field := range required {
			requiredSet[field] = true
		}
		missing := make([]string, 0)
		for _, field := range required {
			if item[field] == nil {
				missing = append(missing, field)
			}
		}
		extra := make([]string, 0)
		for field := range item {
			if !requiredSet[field] {
				extra = append(extra, field)
			}
		}
		sort.Strings(extra)
		return fmt.Errorf("モデル出力の人物項目が抽出契約と一致しません (不足: %s, 契約外: %s)", strings.Join(missing, ","), strings.Join(extra, ","))
	}
	for _, field := range required {
		if item[field] == nil {
			return fmt.Errorf("モデル出力の人物項目に %s がありません", field)
		}
	}
	if update {
		var characterID string
		if json.Unmarshal(item["characterId"], &characterID) != nil || strings.TrimSpace(characterID) == "" || string(item["firstAppearanceEpisodeIndex"]) != "null" {
			return errors.New("モデル出力の人物更新IDまたは初登場話が不正です")
		}
	}
	if !update || string(item["canonicalName"]) != "null" {
		if err := validateExtractionVersionObject(item["canonicalName"]); err != nil {
			return err
		}
	}
	for _, field := range []string{"fullName", "gender"} {
		if string(item[field]) != "null" {
			if err := validateExtractionVersionObject(item[field]); err != nil {
				return err
			}
		}
	}
	if !update {
		var firstAppearance string
		if json.Unmarshal(item["firstAppearanceEpisodeIndex"], &firstAppearance) != nil || !isDigitsEpisodeIndex(firstAppearance) {
			return errors.New("モデル出力の人物初登場話が不正です")
		}
	}
	for _, field := range []string{"fullNameHistory", "genderHistory", "aliases", "appearanceHistory", "personalityHistory", "summaryHistory"} {
		var versions []json.RawMessage
		if json.Unmarshal(item[field], &versions) != nil || versions == nil {
			return fmt.Errorf("モデル出力の人物項目 %s は配列ではありません", field)
		}
		for _, version := range versions {
			if err := validateExtractionVersionObject(version); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateExtractionVersionObject(raw json.RawMessage) error {
	var version map[string]json.RawMessage
	if json.Unmarshal(raw, &version) != nil {
		return errors.New("モデル出力の人物履歴項目がobjectではありません")
	}
	if len(version) != 2 || version["text"] == nil || version["episodeIndex"] == nil {
		missing := make([]string, 0, 2)
		for _, field := range []string{"text", "episodeIndex"} {
			if version[field] == nil {
				missing = append(missing, field)
			}
		}
		extra := make([]string, 0)
		for field := range version {
			if field != "text" && field != "episodeIndex" {
				extra = append(extra, field)
			}
		}
		sort.Strings(extra)
		return fmt.Errorf("モデル出力の人物履歴項目が不正です (不足: %s, 契約外: %s)", strings.Join(missing, ","), strings.Join(extra, ","))
	}
	var text string
	var episodeIndex string
	if json.Unmarshal(version["text"], &text) != nil || json.Unmarshal(version["episodeIndex"], &episodeIndex) != nil || !isDigitsEpisodeIndex(episodeIndex) {
		return errors.New("モデル出力の人物履歴値が不正です")
	}
	return nil
}

func validateExtractionSimpleObject(raw json.RawMessage, required []string) error {
	var item map[string]json.RawMessage
	if json.Unmarshal(raw, &item) != nil || len(item) != len(required) {
		return errors.New("object contract mismatch")
	}
	for _, field := range required {
		if item[field] == nil {
			return errors.New("object field missing")
		}
		if field == "confidence" {
			var value float64
			if json.Unmarshal(item[field], &value) != nil {
				return errors.New("object number field invalid")
			}
			continue
		}
		var value string
		if json.Unmarshal(item[field], &value) != nil || (field == "episodeIndex" && !isDigitsEpisodeIndex(value)) {
			return errors.New("object string field invalid")
		}
	}
	return nil
}

type preparedExtractionRequest struct {
	Messages                  []ai.ChatMessage
	ResponseFormat            map[string]any
	PromptTokens              int
	ModelToActualEpisodeIndex map[string]string
}

func prepareExtractionRequest(config *store.ResolvedAIGenerationConfig, novelID string, upToEpisodeIndex string, knownCharacters []characters.GeneratedCharacter, knownTerms []terms.GeneratedTerm, batch extractionBatch, unresolved []characters.GeneratedUnresolvedMention, focus ...string) preparedExtractionRequest {
	var systemPromptOverride *string
	if config != nil {
		systemPromptOverride = config.SystemPrompt
	}
	modelBatch, modelUpToEpisodeIndex, modelToActualEpisodeIndex := extractionBatchWithModelEpisodeReferences(batch, upToEpisodeIndex)
	systemPrompt, userPrompt := buildExtractionPromptWithUnresolved(novelID, modelUpToEpisodeIndex, knownCharacters, knownTerms, modelBatch, unresolved, systemPromptOverride)
	var promptPayload map[string]any
	if json.Unmarshal([]byte(userPrompt), &promptPayload) == nil {
		removeOpaqueEpisodeMetadataFromExtractionPrompt(promptPayload)
		promptPayload["episodeIndexContract"] = "episodeIndexにはepisodes内の短い参照値（ep1、ep2など）だけを使用してください。元の長いIDや推測した値は出力しないでください。"
		if encoded, err := json.MarshalIndent(promptPayload, "", "  "); err == nil {
			userPrompt = string(encoded)
		}
	}
	if len(focus) > 0 && focus[0] == "parallel_entities" {
		systemPrompt += "\n並列抽出では人物と用語を同じレスポンスで必ず抽出してください。この場合だけ、term の descriptionHistory は累積 snapshot ではなく、今回の episodes で新しく明示された事実だけを書いてください。過去や未来の本文から補完しないでください。"
	}
	messages := []ai.ChatMessage{{Role: "system", Content: systemPrompt}, {Role: "user", Content: userPrompt}}
	responseFormat := extractionOpenRouterResponseFormat(modelBatch.EpisodeIndexes...)
	return preparedExtractionRequest{
		Messages:                  messages,
		ResponseFormat:            responseFormat,
		PromptTokens:              estimateOpenRouterChatRequestTokens(messages, nil, responseFormat),
		ModelToActualEpisodeIndex: modelToActualEpisodeIndex,
	}
}

func removeOpaqueEpisodeMetadataFromExtractionPrompt(payload map[string]any) {
	for _, field := range []string{"candidateCharacters", "knownTerms", "unresolvedMentions"} {
		items, _ := payload[field].([]any)
		for _, rawItem := range items {
			item, _ := rawItem.(map[string]any)
			switch field {
			case "candidateCharacters":
				removeOpaqueEpisodeMetadataField(item, "firstAppearance")
			case "knownTerms":
				removeOpaqueEpisodeMetadataField(item, "latestEpisodeIndex")
			case "unresolvedMentions":
				removeOpaqueEpisodeMetadataField(item, "episodeIndex")
			}
		}
	}
}

func removeOpaqueEpisodeMetadataField(item map[string]any, field string) {
	value, _ := item[field].(string)
	value = strings.TrimSpace(value)
	if len(value) >= 16 && isDigitsEpisodeIndex(value) {
		delete(item, field)
	}
}

func extractionBatchWithModelEpisodeReferences(batch extractionBatch, upToEpisodeIndex string) (extractionBatch, string, map[string]string) {
	actualToModel := map[string]string{}
	modelToActual := map[string]string{}
	add := func(actual string) string {
		actual = strings.TrimSpace(actual)
		if actual == "" {
			return ""
		}
		if model, ok := actualToModel[actual]; ok {
			return model
		}
		model := fmt.Sprintf("ep%d", len(actualToModel)+1)
		actualToModel[actual] = model
		modelToActual[model] = actual
		return model
	}
	modelBatch := batch
	modelBatch.EpisodeIndexes = make([]string, 0, len(batch.EpisodeIndexes))
	for _, actual := range batch.EpisodeIndexes {
		if strings.TrimSpace(actual) != "" {
			modelBatch.EpisodeIndexes = append(modelBatch.EpisodeIndexes, add(actual))
		}
	}
	modelBatch.Chunks = append([]extractionChunk{}, batch.Chunks...)
	for index := range modelBatch.Chunks {
		modelBatch.Chunks[index].EpisodeIndex = add(modelBatch.Chunks[index].EpisodeIndex)
	}
	modelUpToEpisodeIndex := actualToModel[strings.TrimSpace(upToEpisodeIndex)]
	if modelUpToEpisodeIndex == "" && len(modelBatch.EpisodeIndexes) > 0 {
		modelUpToEpisodeIndex = modelBatch.EpisodeIndexes[len(modelBatch.EpisodeIndexes)-1]
	}
	return modelBatch, modelUpToEpisodeIndex, modelToActual
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

func extractionOpenRouterResponseFormat(allowedEpisodeIndexes ...string) map[string]any {
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
	episodeIndexSchema := map[string]any{"type": "string", "pattern": "^\\d+$"}
	if len(episodeIndexValues) > 0 {
		episodeIndexSchema = map[string]any{"type": "string", "enum": episodeIndexValues}
	}
	episodeIndexRef := map[string]any{"$ref": "#/$defs/episodeIndex"}
	textVersionSchema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []any{"text", "episodeIndex"},
		"properties": map[string]any{
			"text":         map[string]any{"type": "string"},
			"episodeIndex": episodeIndexRef,
		},
	}
	historyVersionSchema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []any{"episodeIndex", "text"},
		"properties": map[string]any{
			"episodeIndex": episodeIndexRef,
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
			"firstAppearanceEpisodeIndex": episodeIndexRef,
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
			"episodeIndex": episodeIndexRef,
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
			"episodeIndex": episodeIndexRef,
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
				"$defs": map[string]any{
					"episodeIndex": episodeIndexSchema,
				},
				"required": []any{"processedUpToEpisodeIndex", "newCharacters", "characterUpdates", "mergeProposals", "unresolvedMentions", "terms"},
				"properties": map[string]any{
					"processedUpToEpisodeIndex": episodeIndexRef,
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
								"episodeIndex": episodeIndexRef,
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

func ExtractionOpenRouterResponseFormat(allowedEpisodeIndexes ...string) map[string]any {
	return extractionOpenRouterResponseFormat(allowedEpisodeIndexes...)
}
