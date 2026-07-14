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
		normalizedAnswer, scalarErr := normalizeExtractionEpisodeIndexScalars([]byte(result.Answer), fallbackEpisodeIndex)
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

func normalizeExtractionEpisodeIndexScalars(raw []byte, processedUpToEpisodeIndex string) ([]byte, error) {
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
	if root, ok := decoded.(map[string]any); ok && isDigitsEpisodeIndex(processedUpToEpisodeIndex) {
		// The processed frontier is batch metadata already known by the server,
		// not an extracted fact. In json_object fallback mode some models omit it
		// or render it as prose, so do not make successful entity extraction
		// depend on the model echoing this value correctly.
		processedUpToEpisodeIndex = strings.TrimSpace(processedUpToEpisodeIndex)
		root["processedUpToEpisodeIndex"] = processedUpToEpisodeIndex
		normalizeExtractionCharacterObjects(root, processedUpToEpisodeIndex)
		normalizeExtractionTermObjects(root, processedUpToEpisodeIndex)
	}
	return json.Marshal(decoded)
}

func normalizeExtractionTermObjects(root map[string]any, episodeIndex string) {
	items, ok := root["terms"].([]any)
	if !ok {
		return
	}
	normalizedItems := make([]any, 0, len(items))
	for _, rawItem := range items {
		item, ok := rawItem.(map[string]any)
		if !ok {
			continue
		}
		if _, exists := item["term"]; !exists {
			item["term"] = item["name"]
		}
		term, _ := item["term"].(string)
		if strings.TrimSpace(term) == "" {
			continue
		}
		if _, exists := item["reading"]; !exists {
			item["reading"] = nil
		}
		if item["reading"] != nil {
			if reading, valid := extractionTermVersionFallback(item["reading"], "text", episodeIndex); valid {
				item["reading"] = reading
			}
		}
		category, validCategory := extractionTermVersionFallback(item["category"], "value", episodeIndex)
		if !validCategory {
			category = map[string]any{"value": string(terms.CategoryOther), "episodeIndex": episodeIndex}
		}
		category["value"] = string(terms.NormalizeCategory(strings.TrimSpace(category["value"].(string))))
		item["category"] = category

		descriptions, _ := item["descriptionHistory"].([]any)
		if len(descriptions) == 0 {
			if description, exists := item["description"]; exists {
				descriptions = []any{description}
			}
		}
		normalizedDescriptions := make([]any, 0, len(descriptions))
		for _, description := range descriptions {
			if normalized, valid := extractionTermVersionFallback(description, "text", episodeIndex); valid {
				normalizedDescriptions = append(normalizedDescriptions, normalized)
			}
		}
		if len(normalizedDescriptions) == 0 {
			continue
		}
		item["descriptionHistory"] = normalizedDescriptions
		for field := range item {
			switch field {
			case "term", "reading", "category", "descriptionHistory":
			default:
				delete(item, field)
			}
		}
		normalizedItems = append(normalizedItems, item)
	}
	root["terms"] = normalizedItems
}

func extractionTermVersionFallback(value any, valueKey string, episodeIndex string) (map[string]any, bool) {
	switch typed := value.(type) {
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil, false
		}
		return map[string]any{valueKey: typed, "episodeIndex": episodeIndex}, true
	case map[string]any:
		text, _ := typed[valueKey].(string)
		if strings.TrimSpace(text) == "" {
			return nil, false
		}
		versionEpisodeIndex, _ := typed["episodeIndex"].(string)
		if strings.TrimSpace(versionEpisodeIndex) == "" {
			versionEpisodeIndex = episodeIndex
		}
		return map[string]any{valueKey: text, "episodeIndex": versionEpisodeIndex}, true
	default:
		return nil, false
	}
}

func normalizeExtractionCharacterObjects(root map[string]any, processedUpToEpisodeIndex string) {
	fields := []string{"canonicalName", "fullName", "fullNameHistory", "gender", "genderHistory", "firstAppearanceEpisodeIndex", "aliases", "appearanceHistory", "personalityHistory", "summaryHistory"}
	for _, collection := range []string{"newCharacters", "characterUpdates"} {
		items, ok := root[collection].([]any)
		if !ok {
			continue
		}
		update := collection == "characterUpdates"
		allowed := make(map[string]bool, len(fields)+1)
		for _, field := range fields {
			allowed[field] = true
		}
		if update {
			allowed["characterId"] = true
		}
		normalizedItems := make([]any, 0, len(items))
		for _, rawItem := range items {
			item, ok := rawItem.(map[string]any)
			if !ok {
				continue
			}
			if !update {
				if _, exists := item["canonicalName"]; !exists {
					if canonicalName, found := extractionCanonicalNameFallback(item, processedUpToEpisodeIndex); found {
						item["canonicalName"] = canonicalName
					}
				}
			} else {
				if _, exists := item["canonicalName"]; !exists {
					item["canonicalName"] = nil
				}
				if _, exists := item["firstAppearanceEpisodeIndex"]; !exists {
					item["firstAppearanceEpisodeIndex"] = nil
				}
			}
			for _, field := range []string{"fullName", "gender"} {
				if _, exists := item[field]; !exists {
					item[field] = nil
				}
			}
			for _, field := range []string{"canonicalName", "fullName", "gender"} {
				if normalized, ok := extractionTextVersionFallback(item[field], processedUpToEpisodeIndex); ok {
					item[field] = normalized
				}
			}
			if !update {
				firstAppearance, _ := item["firstAppearanceEpisodeIndex"].(string)
				if !isDigitsEpisodeIndex(firstAppearance) {
					firstAppearance = extractionTextVersionEpisodeIndex(item["canonicalName"])
					if !isDigitsEpisodeIndex(firstAppearance) {
						firstAppearance = processedUpToEpisodeIndex
					}
					item["firstAppearanceEpisodeIndex"] = firstAppearance
				}
			}
			for _, field := range []string{"fullNameHistory", "genderHistory", "aliases", "appearanceHistory", "personalityHistory", "summaryHistory"} {
				if _, exists := item[field]; !exists {
					item[field] = []any{}
				}
				if versions, ok := item[field].([]any); ok {
					normalized := make([]any, 0, len(versions))
					for _, version := range versions {
						if normalizedVersion, valid := extractionTextVersionFallback(version, processedUpToEpisodeIndex); valid {
							normalized = append(normalized, normalizedVersion)
						} else {
							normalized = append(normalized, version)
						}
					}
					item[field] = normalized
				}
			}
			for field := range item {
				if !allowed[field] {
					delete(item, field)
				}
			}
			if !update {
				if _, valid := extractionTextVersionFallback(item["canonicalName"], processedUpToEpisodeIndex); !valid {
					continue
				}
			}
			normalizedItems = append(normalizedItems, item)
		}
		root[collection] = normalizedItems
	}
}

func extractionCanonicalNameFallback(item map[string]any, episodeIndex string) (map[string]any, bool) {
	if version, ok := extractionTextVersionFallback(item["fullName"], episodeIndex); ok {
		return version, true
	}
	aliases, _ := item["aliases"].([]any)
	for _, alias := range aliases {
		if version, ok := extractionTextVersionFallback(alias, episodeIndex); ok {
			return version, true
		}
	}
	return nil, false
}

func extractionTextVersionFallback(value any, episodeIndex string) (map[string]any, bool) {
	switch typed := value.(type) {
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil, false
		}
		return map[string]any{"text": typed, "episodeIndex": episodeIndex}, true
	case map[string]any:
		text, _ := typed["text"].(string)
		if strings.TrimSpace(text) == "" {
			return nil, false
		}
		versionEpisodeIndex, _ := typed["episodeIndex"].(string)
		if strings.TrimSpace(versionEpisodeIndex) == "" {
			versionEpisodeIndex = episodeIndex
		}
		return map[string]any{"text": text, "episodeIndex": versionEpisodeIndex}, true
	default:
		return nil, false
	}
}

func extractionTextVersionEpisodeIndex(value any) string {
	version, _ := value.(map[string]any)
	episodeIndex, _ := version["episodeIndex"].(string)
	return strings.TrimSpace(episodeIndex)
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
	responseFormat := extractionOpenRouterResponseFormat(batch.EpisodeIndexes...)
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
