package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"narou-viewer/apps/viewer-api-go/internal/ai"
	appextraction "narou-viewer/apps/viewer-api-go/internal/application/extraction"
	"narou-viewer/apps/viewer-api-go/internal/application/extractionruntime"
	"narou-viewer/apps/viewer-api-go/internal/characters"
	"narou-viewer/apps/viewer-api-go/internal/extraction"
	extractdomain "narou-viewer/apps/viewer-api-go/internal/extraction"
	"narou-viewer/apps/viewer-api-go/internal/extraction/checkpointstore"
	"narou-viewer/apps/viewer-api-go/internal/fsatomic"
	"narou-viewer/apps/viewer-api-go/internal/library"
	"narou-viewer/apps/viewer-api-go/internal/store"
)

type extractionCheckpoint = checkpointstore.Checkpoint

type extractionRequestOptions = appextraction.RequestOptions

type extractionBatchProgress = appextraction.BatchProgress

type extractionEpisodeInput = extraction.EpisodeInput
type extractionChunk = extraction.Chunk
type extractionBatch = extraction.Batch
type extractionBatchBudget = extraction.BatchBudget
type extractionDelta = extraction.Delta
type extractionUnresolvedMention = extraction.UnresolvedMention
type extractionGenerationState = extraction.GenerationState

type extractionBatchResult struct {
	Delta extractionDelta
	Usage ai.UsageRequest
}

const extractionDefaultMaxTokens = 12000

type extractionInputs = appextraction.Inputs

const extractionMinimumCompletionTokens = 512
const (
	maxProviderOrderItems      = 16
	maxProviderOrderItemLength = 80
)

func extractionTimingLogEnabled() bool {
	value := strings.TrimSpace(os.Getenv("VIEWER_EXTRACTION_TIMING_LOG"))
	if value == "" {
		value = strings.TrimSpace(os.Getenv("VIEWER_CHARACTER_SUMMARY_TIMING_LOG"))
	}
	return value == "1"
}

func logExtractionTiming(stage string, startedAt time.Time, fields ...any) {
	if !extractionTimingLogEnabled() {
		return
	}
	values := []any{
		"stage", stage,
		"elapsedMs", time.Since(startedAt).Milliseconds(),
	}
	values = append(values, fields...)
	log.Printf("viewer-api-go: extraction timing %s", formatTimingFields(values...))
}

func LogExtractionTiming(stage string, startedAt time.Time, fields ...any) {
	logExtractionTiming(stage, startedAt, fields...)
}

func formatTimingFields(fields ...any) string {
	parts := make([]string, 0, len(fields)/2)
	for index := 0; index+1 < len(fields); index += 2 {
		parts = append(parts, fmt.Sprintf("%v=%v", fields[index], fields[index+1]))
	}
	return strings.Join(parts, " ")
}

func (s *Server) handleAISettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		settings, err := s.stateStore.GetAIGenerationSettings()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to access AI generation settings.")
			return
		}
		s.enrichAIGenerationSettingsModelInfo(r.Context(), &settings)
		writeJSON(w, http.StatusOK, settings)
	case http.MethodPut:
		body, ok := decodeObjectOrBadRequest(w, r)
		if !ok {
			return
		}
		if preferredValue, exists := body["preferredMode"]; exists {
			preferred, ok := preferredValue.(string)
			if !ok || (preferred != "llm" && preferred != "heuristic") {
				writeError(w, http.StatusBadRequest, "preferredMode must be 'llm' or 'heuristic'.")
				return
			}
		}
		if selectedProfileID, ok := body["selectedProfileId"]; ok && selectedProfileID != nil {
			if _, ok := selectedProfileID.(string); !ok {
				writeError(w, http.StatusBadRequest, "selectedProfileId must be a string or null.")
				return
			}
		}
		update, errMessage := parseAIGenerationSettingsUpdate(body)
		if errMessage != "" {
			writeError(w, http.StatusBadRequest, errMessage)
			return
		}
		if update.SelectedProfileID != nil && *update.SelectedProfileID != "" {
			if update.ProfilesSet {
				if !profileInputExists(update.Profiles, *update.SelectedProfileID) {
					writeError(w, http.StatusBadRequest, "selectedProfileId must match one of the profiles.")
					return
				}
			} else {
				current, err := s.stateStore.GetAIGenerationSettings()
				if err != nil {
					writeError(w, http.StatusInternalServerError, "Failed to access AI generation settings.")
					return
				}
				if !profileMetadataExists(current.Settings.Profiles, *update.SelectedProfileID) {
					writeError(w, http.StatusBadRequest, "selectedProfileId must match one of the profiles.")
					return
				}
			}
		}
		if update.SelectedProfileID == nil && update.ProfilesSet {
			current, err := s.stateStore.GetAIGenerationSettings()
			if err != nil {
				writeError(w, http.StatusInternalServerError, "Failed to access AI generation settings.")
				return
			}
			if current.Settings.SelectedProfileID != nil && *current.Settings.SelectedProfileID != "" && !profileInputExists(update.Profiles, *current.Settings.SelectedProfileID) {
				writeError(w, http.StatusBadRequest, "selectedProfileId must match one of the profiles.")
				return
			}
		}
		settings, err := s.stateStore.PutAIGenerationSettings(update)
		if err != nil {
			if store.IsAIGenerationSettingsCryptoError(err) {
				writeError(w, http.StatusServiceUnavailable, err.Error())
				return
			}
			writeError(w, http.StatusInternalServerError, "Failed to access AI generation settings.")
			return
		}
		s.enrichAIGenerationSettingsModelInfo(r.Context(), &settings)
		writeJSON(w, http.StatusOK, settings)
	default:
		methodOnly(w, r, http.MethodGet, http.MethodPut)
	}
}

func (s *Server) enrichAIGenerationSettingsModelInfo(ctx context.Context, settings *ai.SettingsResponse) {
	if s == nil || s.stateStore == nil || settings == nil {
		return
	}
	lookupRoot, cancelRoot := context.WithTimeout(ctx, 2*time.Second)
	defer cancelRoot()
	for index := range settings.Settings.Profiles {
		if lookupRoot.Err() != nil {
			return
		}
		profile := &settings.Settings.Profiles[index]
		if profile.ModelID == nil || strings.TrimSpace(*profile.ModelID) == "" {
			continue
		}
		profileID := profile.ID
		config, err := s.stateStore.ResolveAIGenerationConfigOverride(&profileID, nil)
		if err != nil || config == nil {
			continue
		}
		lookupCtx, cancel := context.WithTimeout(lookupRoot, 2*time.Second)
		info, ok := ai.LookupOpenRouterModelInfo(lookupCtx, config.APIKey, config.ModelID, config.ProviderOrder)
		cancel()
		if !ok {
			continue
		}
		profile.ModelInfo = &ai.ModelInfoMetadata{
			ContextLength:       info.ContextLength,
			MaxCompletionTokens: info.MaxCompletionTokens,
			Source:              "openrouter",
		}
	}
}

func (s *Server) handlePreferredMode(w http.ResponseWriter, r *http.Request) {
	if !methodOnly(w, r, http.MethodPut) {
		return
	}
	body, ok := decodeObjectOrBadRequest(w, r)
	if !ok {
		return
	}
	preferred, ok := body["preferredMode"].(string)
	if !ok || (preferred != "llm" && preferred != "heuristic") {
		writeError(w, http.StatusBadRequest, "preferredMode must be 'llm' or 'heuristic'.")
		return
	}

	response, err := s.stateStore.PutAIGenerationPreferredMode(preferred)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to access AI generation settings.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"preferredMode":           response.PreferredMode,
		"effectiveGenerationMode": response.EffectiveGenerationMode,
	})
}

func parseAIGenerationSettingsUpdate(body map[string]any) (store.AIGenerationSettingsUpdate, string) {
	update := store.AIGenerationSettingsUpdate{}
	if preferred, ok := body["preferredMode"].(string); ok {
		update.PreferredMode = &preferred
	}
	if selected, ok := body["selectedProfileId"]; ok {
		if selected == nil {
			empty := ""
			update.SelectedProfileID = &empty
		} else if value, ok := selected.(string); ok {
			trimmed := strings.TrimSpace(value)
			update.SelectedProfileID = &trimmed
		}
	}
	if rawShared, ok := body["sharedProviders"]; ok {
		shared, ok := parseAISharedProviders(rawShared)
		if !ok {
			return update, "sharedProviders is invalid."
		}
		update.SharedProviders = &shared
	}
	if rawStrategyModels, ok := body["extractionStrategyModels"]; ok {
		strategyModels, ok := parseAIExtractionStrategyModels(rawStrategyModels)
		if !ok {
			return update, "extractionStrategyModels is invalid."
		}
		update.ExtractionStrategyModels = &strategyModels
	}
	if rawProfiles, ok := body["profiles"]; ok {
		profiles, ok := rawProfiles.([]any)
		if !ok {
			return update, "profiles must be an array."
		}
		if len(profiles) == 0 {
			return update, "profiles must contain at least one profile."
		}
		update.ProfilesSet = true
		update.Profiles = make([]store.AIProfileInput, 0, len(profiles))
		for index, rawProfile := range profiles {
			profile, ok := parseAIProfile(rawProfile, index)
			if !ok {
				return update, "profiles contains an invalid entry."
			}
			update.Profiles = append(update.Profiles, profile)
		}
	}
	return update, ""
}

func parseAIExtractionStrategyModels(value any) (store.AIExtractionStrategyModelsInput, bool) {
	if value == nil {
		return store.AIExtractionStrategyModelsInput{}, true
	}
	record, ok := value.(map[string]any)
	if !ok {
		return store.AIExtractionStrategyModelsInput{}, false
	}
	nameDiscoveryModelID, ok := nullableStringField(record, "nameDiscoveryModelId")
	if !ok {
		return store.AIExtractionStrategyModelsInput{}, false
	}
	return store.AIExtractionStrategyModelsInput{NameDiscoveryModelID: nameDiscoveryModelID}, true
}

func parseAISharedProviders(value any) (store.AISharedProvidersInput, bool) {
	record, ok := value.(map[string]any)
	if !ok {
		return store.AISharedProvidersInput{}, false
	}
	openrouter, ok := record["openrouter"].(map[string]any)
	if !ok {
		if _, exists := record["openrouter"]; exists {
			return store.AISharedProvidersInput{}, false
		}
	}
	googleBooks, googleBooksOK := record["googleBooks"].(map[string]any)
	if !googleBooksOK {
		if _, exists := record["googleBooks"]; exists {
			return store.AISharedProvidersInput{}, false
		}
	}
	openRouterCredential, ok := parseAIProviderCredential(openrouter)
	if !ok {
		return store.AISharedProvidersInput{}, false
	}
	googleBooksCredential, ok := parseAIProviderCredential(googleBooks)
	if !ok {
		return store.AISharedProvidersInput{}, false
	}
	return store.AISharedProvidersInput{OpenRouter: openRouterCredential, GoogleBooks: googleBooksCredential}, true
}

func parseAIProviderCredential(record map[string]any) (store.AIProviderCredentialInput, bool) {
	credential := store.AIProviderCredentialInput{}
	if record == nil {
		return credential, true
	}
	if apiKey, ok := record["apiKey"]; ok {
		credential.APIKeySet = true
		if apiKey == nil {
			credential.APIKey = nil
		} else if value, ok := apiKey.(string); ok {
			credential.APIKey = &value
		} else {
			return store.AIProviderCredentialInput{}, false
		}
	}
	return credential, true
}

func parseAIProfile(value any, index int) (store.AIProfileInput, bool) {
	record, ok := value.(map[string]any)
	if !ok {
		return store.AIProfileInput{}, false
	}
	id := stringField(record, "id")
	if id == "" {
		if index == 0 {
			id = "default"
		} else {
			id = "profile-" + strconv.Itoa(index+1)
		}
	}
	label := stringField(record, "label")
	if label == "" {
		return store.AIProfileInput{}, false
	}
	provider, ok := optionalStringField(record, "provider")
	if !ok {
		return store.AIProfileInput{}, false
	}
	if provider != "" && provider != "openrouter" {
		return store.AIProfileInput{}, false
	}
	credentials, ok := parseAIProfileCredentials(record["credentials"])
	if !ok {
		return store.AIProfileInput{}, false
	}
	if apiKey, ok := record["apiKey"]; ok {
		if credentials.Source == "" {
			credentials.Source = "custom"
		}
		credentials.APIKeySet = true
		if apiKey == nil {
			credentials.APIKey = nil
		} else if value, ok := apiKey.(string); ok {
			credentials.APIKey = &value
		} else {
			return store.AIProfileInput{}, false
		}
	}
	providerOrder, ok := parseStringArray(record["providerOrder"])
	if !ok {
		return store.AIProfileInput{}, false
	}
	modelID, ok := nullableStringField(record, "modelId")
	if !ok {
		return store.AIProfileInput{}, false
	}
	allowFallbacks, ok := boolField(record, "allowFallbacks", false)
	if !ok {
		return store.AIProfileInput{}, false
	}
	requireParameters, ok := boolField(record, "requireParameters", true)
	if !ok {
		return store.AIProfileInput{}, false
	}
	return store.AIProfileInput{
		ID:                id,
		Label:             label,
		Provider:          provider,
		Credentials:       credentials,
		ModelID:           modelID,
		ProviderOrder:     providerOrder,
		AllowFallbacks:    allowFallbacks,
		RequireParameters: requireParameters,
	}, true
}

func parseAIProfileCredentials(value any) (store.AIProfileCredentialsInput, bool) {
	if value == nil {
		return store.AIProfileCredentialsInput{}, true
	}
	record, ok := value.(map[string]any)
	if !ok {
		return store.AIProfileCredentialsInput{}, false
	}
	source, ok := optionalStringField(record, "source")
	if !ok {
		return store.AIProfileCredentialsInput{}, false
	}
	if source != "shared" && source != "custom" {
		if source != "" {
			return store.AIProfileCredentialsInput{}, false
		}
	}
	credentials := store.AIProfileCredentialsInput{Source: source}
	if apiKey, ok := record["apiKey"]; ok {
		credentials.APIKeySet = true
		if apiKey == nil {
			credentials.APIKey = nil
		} else if value, ok := apiKey.(string); ok {
			credentials.APIKey = &value
		} else {
			return store.AIProfileCredentialsInput{}, false
		}
	}
	return credentials, true
}

func profileInputExists(profiles []store.AIProfileInput, id string) bool {
	for _, profile := range profiles {
		if profile.ID == id {
			return true
		}
	}
	return false
}

func profileMetadataExists(profiles []ai.Profile, id string) bool {
	for _, profile := range profiles {
		if profile.ID == id {
			return true
		}
	}
	return false
}

func stringPointer(value string) *string {
	return &value
}

func stringField(record map[string]any, key string) string {
	value, ok := record[key].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}

func optionalStringField(record map[string]any, key string) (string, bool) {
	value, ok := record[key]
	if !ok || value == nil {
		return "", true
	}
	text, ok := value.(string)
	if !ok {
		return "", false
	}
	return strings.TrimSpace(text), true
}

func nullableStringField(record map[string]any, key string) (*string, bool) {
	value, ok := record[key]
	if !ok || value == nil {
		return nil, true
	}
	text, ok := value.(string)
	if !ok {
		return nil, false
	}
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil, true
	}
	return &trimmed, true
}

func boolField(record map[string]any, key string, fallback bool) (bool, bool) {
	value, exists := record[key]
	if !exists {
		return fallback, true
	}
	typed, ok := value.(bool)
	if !ok {
		return false, false
	}
	return typed, true
}

func parseStringArray(value any) ([]string, bool) {
	if value == nil {
		return []string{}, true
	}
	if text, ok := value.(string); ok {
		result := []string{}
		for _, item := range strings.Split(text, ",") {
			trimmed := strings.TrimSpace(item)
			if trimmed != "" {
				result = append(result, trimmed)
			}
		}
		if len(result) == 0 {
			return []string{}, true
		}
		return validateProviderOrder(result)
	}
	items, ok := value.([]any)
	if !ok {
		return nil, false
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		text, ok := item.(string)
		if !ok {
			return nil, false
		}
		trimmed := strings.TrimSpace(text)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return validateProviderOrder(result)
}

func validateProviderOrder(values []string) ([]string, bool) {
	if len(values) > maxProviderOrderItems {
		return nil, false
	}
	seen := map[string]bool{}
	for _, value := range values {
		if !isValidProviderOrderItem(value) {
			return nil, false
		}
		normalized := strings.ToLower(value)
		if seen[normalized] {
			return nil, false
		}
		seen[normalized] = true
	}
	return values, true
}

func isValidProviderOrderItem(value string) bool {
	if value == "" || len(value) > maxProviderOrderItemLength {
		return false
	}
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-' || r == '_' || r == '.' || r == '/' || r == ':':
		default:
			return false
		}
	}
	return true
}

func (s *Server) handleAIJobs(w http.ResponseWriter, r *http.Request) {
	if !methodOnly(w, r, http.MethodGet) {
		return
	}
	records, err := extractdomain.LoadAllJobs(s.stateDir())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Character jobs could not be read.")
		return
	}
	novelsByID := map[string]library.NovelSummary{}
	if s.library != nil {
		if result, err := s.library.ListNovels(r.Context()); err == nil {
			for _, novel := range result.Novels {
				novelsByID[novel.NovelID] = novel
			}
		}
	}
	jobs := make([]ai.Job, 0, len(records))
	for _, record := range records {
		var novelTitle *string
		var novelAuthor *string
		if novel, ok := novelsByID[record.NovelID]; ok {
			novelTitle = stringPointer(novel.Title)
			novelAuthor = stringPointer(novel.Author)
		}
		jobs = append(jobs, ai.Job{
			JobID:                     record.Job.JobID,
			NovelID:                   record.NovelID,
			NovelTitle:                novelTitle,
			NovelAuthor:               novelAuthor,
			RequestedUpToEpisodeIndex: record.Job.RequestedUpToEpisodeIndex,
			ProfileID:                 record.Job.ProfileID,
			ProfileLabel:              record.Job.ProfileLabel,
			GenerationMode:            record.Job.GenerationMode,
			GenerationStrategy:        record.Job.GenerationStrategy,
			ModelID:                   record.Job.ModelID,
			Status:                    record.Job.Status,
			Progress:                  record.Job.Progress,
			ProgressStage:             record.Job.ProgressStage,
			CurrentBatchIndex:         record.Job.CurrentBatchIndex,
			BatchCount:                record.Job.BatchCount,
			GeneratedCharacterCount:   record.Job.GeneratedCharacterCount,
			CreatedAt:                 record.Job.CreatedAt,
			StartedAt:                 record.Job.StartedAt,
			FinishedAt:                record.Job.FinishedAt,
			ErrorMessage:              record.Job.ErrorMessage,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": jobs})
}

func (s *Server) handleUsage(w http.ResponseWriter, r *http.Request) {
	if !methodOnly(w, r, http.MethodGet) {
		return
	}
	if usage, ok, err := ai.LoadUsage(s.aiUsageDBPath()); err != nil {
		writeError(w, http.StatusInternalServerError, "AI usage store could not be read.")
		return
	} else if ok {
		writeJSON(w, http.StatusOK, usage)
		return
	}
	writeJSON(w, http.StatusOK, ai.EmptyUsage())
}

func (s *Server) handleUsageDetail(w http.ResponseWriter, r *http.Request) {
	if !methodOnly(w, r, http.MethodGet) {
		return
	}
	runID := strings.TrimPrefix(r.URL.Path, "/api/ai-generation/usage/")
	if run, ok, err := ai.LoadUsageRun(s.aiUsageDBPath(), runID); err != nil {
		writeError(w, http.StatusInternalServerError, "AI usage store could not be read.")
		return
	} else if ok {
		writeJSON(w, http.StatusOK, run)
		return
	}
	writeError(w, http.StatusNotFound, "AI usage run not found.")
}

func (s *Server) aiUsageDBPath() string {
	return filepath.Join(s.dataDir, "state", "ai_usage.sqlite")
}

func (s *Server) parseExtractionRequestOptions(body map[string]any) (extractionRequestOptions, string) {
	options := extractionRequestOptions{}
	if rawStrategy, exists := body["generationStrategy"]; exists {
		strategy, ok := rawStrategy.(string)
		if !ok {
			return options, "キャラクター一覧生成方式が不正です。"
		}
		normalized := appextraction.NormalizeGenerationStrategy(strategy)
		if strings.TrimSpace(strategy) != "" && normalized != strings.TrimSpace(strategy) {
			return options, "キャラクター一覧生成方式が不正です。"
		}
		options.GenerationStrategy = normalized
	}
	if rawProfileID, exists := body["profileId"]; exists {
		if rawProfileID == nil {
			options.ProfileResolution = true
		} else if profileID, ok := rawProfileID.(string); ok {
			trimmed := strings.TrimSpace(profileID)
			if trimmed != "" {
				options.ProfileID = &trimmed
			}
			options.ProfileResolution = true
		} else {
			return options, "一時 AI 生成設定が不正です。"
		}
	}

	transient := store.AIGenerationTransientConfig{}
	hasTransient := false
	if raw, exists := body["modelId"]; exists {
		modelID, ok := nullableBodyString(raw)
		if !ok {
			return options, "一時 AI 生成設定が不正です。"
		}
		transient.ModelID = modelID
		hasTransient = true
	}
	if raw, exists := body["providerOrder"]; exists {
		providerOrder, ok := parseStringArray(raw)
		if !ok {
			return options, "一時 AI 生成設定が不正です。"
		}
		transient.ProviderOrder = providerOrder
		transient.ProviderOrderSet = true
		hasTransient = true
	}
	if raw, exists := body["allowFallbacks"]; exists {
		value, ok := raw.(bool)
		if !ok {
			return options, "一時 AI 生成設定が不正です。"
		}
		transient.AllowFallbacks = &value
		hasTransient = true
	}
	if raw, exists := body["requireParameters"]; exists {
		value, ok := raw.(bool)
		if !ok {
			return options, "一時 AI 生成設定が不正です。"
		}
		transient.RequireParameters = &value
		hasTransient = true
	}
	if raw, exists := body["systemPromptOverride"]; exists {
		if raw == nil {
			hasTransient = true
		} else if text, ok := raw.(string); ok {
			trimmed := strings.TrimSpace(text)
			if trimmed == "" {
				return options, "一時 AI 生成設定が不正です。"
			}
			transient.SystemPromptOverride = &trimmed
			hasTransient = true
		} else {
			return options, "一時 AI 生成設定が不正です。"
		}
	}
	if hasTransient {
		options.Transient = &transient
		options.ProfileResolution = true
	}
	return options, ""
}

func nullableBodyString(value any) (*string, bool) {
	if value == nil {
		return nil, true
	}
	text, ok := value.(string)
	if !ok {
		return nil, false
	}
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil, true
	}
	return &trimmed, true
}

func (s *Server) resolveExtractionRequestOptions(options *extractionRequestOptions) (string, int) {
	if options == nil || !options.ProfileResolution {
		return "", http.StatusOK
	}
	if s.stateStore == nil {
		return "AI generation profile was not found.", http.StatusBadRequest
	}
	config, err := s.stateStore.ResolveAIGenerationConfigOverride(options.ProfileID, options.Transient)
	if err != nil {
		if store.IsAIGenerationSettingsCryptoError(err) {
			return "AI generation settings could not be decrypted.", http.StatusServiceUnavailable
		}
		return "AI generation profile was not found.", http.StatusBadRequest
	}
	if config == nil {
		return "AI generation profile was not found.", http.StatusBadRequest
	}
	options.ResolvedConfig = config
	options.GenerationMode = "openrouter"
	return "", http.StatusOK
}

func (s *Server) handlePlayground(w http.ResponseWriter, r *http.Request) {
	if !methodOnly(w, r, http.MethodPost) {
		return
	}
	body, ok := decodeObjectOrBadRequest(w, r)
	if !ok {
		return
	}
	novelID, ok := body["novelId"].(string)
	if !ok || strings.TrimSpace(novelID) == "" {
		writeError(w, http.StatusBadRequest, "novelId is required.")
		return
	}
	if _, ok := isNonNegativeIntegerString(body["upToEpisodeIndex"]); !ok {
		writeError(w, http.StatusBadRequest, "upToEpisodeIndex is required and must be a non-negative integer string.")
		return
	}
	upToEpisodeIndex, _ := isNonNegativeIntegerString(body["upToEpisodeIndex"])
	novelFound, episodeFound, lookupErr := s.validateNovelEpisode(r.Context(), novelID, upToEpisodeIndex)
	if lookupErr != nil {
		writeLibraryLookupError(w, r, novelID, upToEpisodeIndex, lookupErr)
		return
	}
	if !novelFound {
		writeError(w, http.StatusNotFound, "Novel not found.")
		return
	}
	if !episodeFound {
		writeError(w, http.StatusBadRequest, "upToEpisodeIndex is out of range.")
		return
	}
	options, errMessage := s.parseExtractionRequestOptions(body)
	if errMessage != "" {
		writeError(w, http.StatusBadRequest, errMessage)
		return
	}
	if errMessage, status := s.resolveExtractionRequestOptions(&options); errMessage != "" {
		writeError(w, status, errMessage)
		return
	}
	options.PreviewOnly = true
	ctx, cancel := nonStreamingLLMContext(r.Context())
	defer cancel()
	result, err := s.extractionResult(ctx, novelID, upToEpisodeIndex, options)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, extractionPlaygroundErrorMessage(err))
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handlePlaygroundStream(w http.ResponseWriter, r *http.Request) {
	if !methodOnly(w, r, http.MethodPost) {
		return
	}
	body, ok := decodeObjectOrBadRequest(w, r)
	if !ok {
		return
	}
	upToEpisodeIndex, ok := isNonNegativeIntegerString(body["upToEpisodeIndex"])
	if !ok {
		writeError(w, http.StatusBadRequest, "upToEpisodeIndex is required and must be a non-negative integer string.")
		return
	}
	novelID, ok := body["novelId"].(string)
	if !ok || strings.TrimSpace(novelID) == "" {
		writeError(w, http.StatusNotFound, "Novel not found.")
		return
	}
	novelFound, episodeFound, lookupErr := s.validateNovelEpisode(r.Context(), novelID, upToEpisodeIndex)
	if lookupErr != nil {
		writeLibraryLookupError(w, r, novelID, upToEpisodeIndex, lookupErr)
		return
	}
	if !novelFound {
		writeError(w, http.StatusNotFound, "Novel not found.")
		return
	}
	if !episodeFound {
		writeError(w, http.StatusBadRequest, "upToEpisodeIndex is out of range.")
		return
	}
	options, errMessage := s.parseExtractionRequestOptions(body)
	if errMessage != "" {
		writeError(w, http.StatusBadRequest, errMessage)
		return
	}
	if errMessage, status := s.resolveExtractionRequestOptions(&options); errMessage != "" {
		writeError(w, status, errMessage)
		return
	}
	options.PreviewOnly = true

	w.Header().Set("content-type", "application/x-ndjson; charset=utf-8")
	w.Header().Set("cache-control", "no-cache, no-transform")
	w.Header().Set("x-content-type-options", "nosniff")
	w.Header().Set("x-accel-buffering", "no")
	extendStreamingWriteDeadline(w)
	w.WriteHeader(http.StatusOK)
	encoder := json.NewEncoder(w)
	flusher, _ := w.(http.Flusher)
	writeStreamEvent := func(event map[string]any) bool {
		if r.Context().Err() != nil {
			return false
		}
		extendStreamingWriteDeadline(w)
		if err := encoder.Encode(event); err != nil {
			return false
		}
		if flusher != nil {
			flusher.Flush()
		}
		return r.Context().Err() == nil
	}
	_ = writeStreamEvent(playgroundStatusEvent("preparing", "入力を確認しました。", 10, 1))
	_ = writeStreamEvent(playgroundStatusEvent("loadingEpisodes", "本文データを読み込んでいます。", 35, 2))
	var preview appextraction.PromptPreview
	preparedPreview, err := s.extractionRuntime().PreparePreview(r.Context(), novelID, upToEpisodeIndex, options.ResolvedConfig)
	if err == nil {
		options.SummaryInputs = &preparedPreview.Inputs
		preview = preparedPreview.Preview
		_ = writeStreamEvent(map[string]any{
			"type":    "promptPreview",
			"preview": preview,
		})
	}
	_ = writeStreamEvent(playgroundStatusEvent("generating", "キャラクター一覧を生成しています。", 70, 3))
	startedAt := time.Now()
	emittedBatchTiming := false
	options.BatchProgressSink = func(progress extractionBatchProgress) {
		switch progress.Phase {
		case "start":
			_ = writeStreamEvent(playgroundBatchStatusEvent(progress.Batch))
		case "complete":
			emittedBatchTiming = true
			_ = writeStreamEvent(extractionBatchTimingEvent(progress))
		}
	}
	result, err := s.extractionResult(r.Context(), novelID, upToEpisodeIndex, options)
	if err != nil {
		_ = writeStreamEvent(map[string]any{"type": "error", "error": extractionPlaygroundErrorMessage(err)})
		return
	}
	generatedCount := len(result.Characters)
	if !emittedBatchTiming {
		_ = writeStreamEvent(map[string]any{
			"type":                    "batchTiming",
			"batchIndex":              1,
			"batchCount":              1,
			"episodeIndexes":          promptPreviewEpisodeIndexes(preview),
			"chunkCount":              promptPreviewChunkCount(preview),
			"elapsedMs":               time.Since(startedAt).Milliseconds(),
			"generatedCharacterCount": generatedCount,
			"mergedCharacterCount":    generatedCount,
			"message":                 "キャラクター一覧生成を完了しました。",
		})
	}
	_ = writeStreamEvent(playgroundStatusEvent("buildingResponse", "レスポンスを組み立てています。", 90, 4))
	_ = writeStreamEvent(map[string]any{
		"type":   "result",
		"result": result,
	})
}

func (s *Server) extractionResult(ctx context.Context, novelID string, upToEpisodeIndex string, options extractionRequestOptions) (appextraction.Result, error) {
	return s.extractionRuntime().Result(ctx, novelID, upToEpisodeIndex, options)
}

func (s *Server) extractionWorkflow() *appextraction.Workflow {
	return s.extractionRuntime().Workflow()
}

func (s *Server) generateAndSaveExtraction(ctx context.Context, novelID string, upToEpisodeIndex string, resolvedOverride *store.ResolvedAIGenerationConfig, progressSink func(extractionBatchProgress)) error {
	_, err := s.extractionRuntime().GenerateAndSave(ctx, novelID, upToEpisodeIndex, resolvedOverride, "", progressSink)
	return err
}

func (s *Server) generateExtractionPreview(ctx context.Context, novelID string, upToEpisodeIndex string, resolvedOverride *store.ResolvedAIGenerationConfig, progressSink func(extractionBatchProgress), episodeIndexes []string, preloaded *extractionInputs) (summary characters.SummaryResponse, err error) {
	result, err := s.extractionRuntime().GeneratePreview(ctx, novelID, upToEpisodeIndex, resolvedOverride, "", progressSink, episodeIndexes, preloaded)
	return characters.SummaryResponse{Status: "ready", NovelID: result.NovelID, UpToEpisodeIndex: result.UpToEpisodeIndex, ProcessedUpToEpisodeIndex: result.ProcessedUpToEpisodeIndex, Characters: result.Characters}, err
}

func extractionPlaygroundErrorMessage(err error) string {
	if err == nil {
		return "Character profiles could not be read."
	}
	message := strings.TrimSpace(err.Error())
	if message == "" {
		return "Character profiles could not be read."
	}
	return "Character profiles could not be read: " + message
}

func rebatchExtractionInputs(ctx context.Context, inputs extractionInputs, config *store.ResolvedAIGenerationConfig, fallbackMaxBatchChars int) extractionInputs {
	if config == nil || len(inputs.Batches) == 0 {
		return inputs
	}
	chunks := make([]extractionChunk, 0)
	for _, batch := range inputs.Batches {
		chunks = append(chunks, batch.Chunks...)
	}
	inputs.Batches = extraction.CreateBatchesWithBudget(chunks, resolveExtractionBatchBudget(ctx, config, fallbackMaxBatchChars))
	return inputs
}

func buildGeneratedExtractionPreview(stateDir string, novelID string, upToEpisodeIndex string, generated []characters.GeneratedCharacter, episodes []characters.HeuristicEpisode, episodeIndexes []string, options characters.SaveGeneratedSummaryOptions) (characters.SummaryResponse, error) {
	return buildExtractionPreview(novelID, upToEpisodeIndex, episodeIndexes, func(tempDir string) error {
		if err := copyExtractionPreviewEvents(stateDir, tempDir, novelID); err != nil {
			return err
		}
		return characters.SaveGeneratedSummaryWithOptions(tempDir, novelID, upToEpisodeIndex, generated, episodes, options)
	})
}

func buildHeuristicExtractionPreview(novelID string, upToEpisodeIndex string, episodes []characters.HeuristicEpisode, episodeIndexes []string) (characters.SummaryResponse, error) {
	return buildExtractionPreview(novelID, upToEpisodeIndex, episodeIndexes, func(tempDir string) error {
		return characters.SaveHeuristicSummary(tempDir, novelID, upToEpisodeIndex, episodes)
	})
}

func buildExtractionPreview(novelID string, upToEpisodeIndex string, episodeIndexes []string, writeSummary func(string) error) (characters.SummaryResponse, error) {
	tempDir, err := os.MkdirTemp("", "narou-viewer-extraction-preview-*")
	if err != nil {
		return characters.SummaryResponse{}, err
	}
	defer os.RemoveAll(tempDir)
	if err := writeSummary(tempDir); err != nil {
		return characters.SummaryResponse{}, err
	}
	return loadRequiredExtractionPreview(tempDir, novelID, upToEpisodeIndex, episodeIndexes)
}

func (s *Server) loadExtractionPendingUnresolved(novelID string, reprocessFromEpisodeIndex string) ([]characters.GeneratedUnresolvedMention, error) {
	pending, err := characters.LoadGeneratedUnresolvedMentions(s.stateDir(), novelID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(reprocessFromEpisodeIndex) == "" {
		return pending, nil
	}
	return filterGeneratedUnresolvedMentionsBeforeEpisode(pending, reprocessFromEpisodeIndex), nil
}

func filterGeneratedUnresolvedMentionsBeforeEpisode(values []characters.GeneratedUnresolvedMention, fromEpisodeIndex string) []characters.GeneratedUnresolvedMention {
	fromEpisodeIndex = strings.TrimSpace(fromEpisodeIndex)
	if fromEpisodeIndex == "" {
		return append([]characters.GeneratedUnresolvedMention{}, values...)
	}
	result := make([]characters.GeneratedUnresolvedMention, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value.EpisodeIndex) != "" && compareEpisodeString(value.EpisodeIndex, fromEpisodeIndex) < 0 {
			result = append(result, value)
		}
	}
	return result
}

func copyExtractionPreviewEvents(sourceStateDir string, targetStateDir string, novelID string) error {
	if strings.TrimSpace(sourceStateDir) == "" || strings.TrimSpace(targetStateDir) == "" || strings.TrimSpace(novelID) == "" {
		return nil
	}
	sourcePath := filepath.Join(sourceStateDir, "character_events", novelID+".yaml")
	raw, err := os.ReadFile(sourcePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	targetPath := filepath.Join(targetStateDir, "character_events", novelID+".yaml")
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}
	return fsatomic.WriteFile(targetPath, raw, 0o600)
}

func loadRequiredExtractionPreview(stateDir string, novelID string, upToEpisodeIndex string, episodeIndexes []string) (characters.SummaryResponse, error) {
	summary, ok, err := characters.LoadSummaryForEpisodes(stateDir, novelID, upToEpisodeIndex, episodeIndexes)
	if err != nil {
		return characters.SummaryResponse{}, err
	}
	if !ok {
		return characters.SummaryResponse{}, errors.New("character summary preview could not be built")
	}
	return summary, nil
}

func (s *Server) generateOpenRouterExtractionWithCheckpoint(ctx context.Context, config *store.ResolvedAIGenerationConfig, novelID string, upToEpisodeIndex string, seed []characters.GeneratedCharacter, batches []extractionBatch, progressSink func(extractionBatchProgress), initialUnresolved ...[]characters.GeneratedUnresolvedMention) ([]characters.GeneratedCharacter, extractionGenerationState, []ai.UsageRequest, error) {
	pendingUnresolved, err := s.extractionInitialUnresolved(novelID, initialUnresolved...)
	if err != nil {
		return nil, extractionGenerationState{}, nil, err
	}
	return s.extractionRuntime().Workflow().RunOpenRouterWithCheckpoint(ctx, config, novelID, upToEpisodeIndex, seed, nil, batches, progressSink, pendingUnresolved)
}

func (s *Server) generateOpenRouterExtraction(ctx context.Context, config *store.ResolvedAIGenerationConfig, novelID string, upToEpisodeIndex string, seed []characters.GeneratedCharacter, batches []extractionBatch, progressSink func(extractionBatchProgress), initialUnresolved ...[]characters.GeneratedUnresolvedMention) ([]characters.GeneratedCharacter, extractionGenerationState, []ai.UsageRequest, error) {
	pendingUnresolved, err := s.extractionInitialUnresolved(novelID, initialUnresolved...)
	if err != nil {
		return nil, extractionGenerationState{}, nil, err
	}
	return s.extractionRuntime().Workflow().RunOpenRouterPreview(ctx, config, novelID, upToEpisodeIndex, seed, nil, batches, progressSink, pendingUnresolved)
}

func (s *Server) extractionInitialUnresolved(novelID string, initialUnresolved ...[]characters.GeneratedUnresolvedMention) ([]characters.GeneratedUnresolvedMention, error) {
	if len(initialUnresolved) > 0 {
		return append([]characters.GeneratedUnresolvedMention{}, initialUnresolved[0]...), nil
	}
	return characters.LoadGeneratedUnresolvedMentions(s.stateDir(), novelID)
}

func extractionCheckpointHasSnapshot(checkpoint extractionCheckpoint) bool {
	return appextraction.CheckpointHasSnapshot(checkpoint)
}

func extractionStateFromAllocator(unresolved []characters.GeneratedUnresolvedMention, allocator *characters.GeneratedCharacterIDAllocator) extractionGenerationState {
	state := extractionGenerationState{
		UnresolvedMentions: append([]characters.GeneratedUnresolvedMention{}, unresolved...),
	}
	if allocator != nil {
		state.IssuedCharacterIDs = allocator.IssuedCharacterIDs()
		state.RetiredCharacterIDs = allocator.RetiredCharacterIDs()
		state.NextOrdinal = allocator.NextCharacterOrdinal()
	}
	return state
}

func allEpisodeIndexesProcessed(episodeIndexes []string, processed []string) bool {
	if len(episodeIndexes) == 0 || len(processed) == 0 {
		return false
	}
	seen := map[string]bool{}
	for _, value := range processed {
		seen[value] = true
	}
	for _, value := range episodeIndexes {
		if !seen[value] {
			return false
		}
	}
	return true
}

func mergeStringSets(existing []string, incoming []string) []string {
	result := append([]string{}, existing...)
	seen := map[string]bool{}
	for _, value := range result {
		seen[value] = true
	}
	for _, value := range incoming {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func appendUniqueInt(existing []int, value int) []int {
	for _, current := range existing {
		if current == value {
			return existing
		}
	}
	return append(existing, value)
}

func extractionCheckpointFingerprint(config *store.ResolvedAIGenerationConfig, extra any) string {
	return appextraction.CheckpointFingerprint(config, extra)
}

func extractionCheckpointBatchInputs(batches []extractionBatch) []map[string]any {
	return appextraction.CheckpointBatchInputs(batches)
}

func extractionCheckpointBatchInput(batch extractionBatch) map[string]any {
	return appextraction.CheckpointBatchInput(batch)
}

func (s *Server) extractionCheckpointPath(novelID string, upToEpisodeIndex string) string {
	return checkpointstore.NewFileStore(s.stateDir()).Path(novelID, upToEpisodeIndex)
}

func (s *Server) loadExtractionCheckpoint(novelID string, upToEpisodeIndex string) extractionCheckpoint {
	return s.loadExtractionCheckpointForGeneration(novelID, upToEpisodeIndex, "")
}

func (s *Server) loadExtractionCheckpointForGeneration(novelID string, upToEpisodeIndex string, expectedFingerprint string) extractionCheckpoint {
	checkpoint, err := s.extractionRuntime().LoadCheckpoint(novelID, upToEpisodeIndex)
	if err != nil ||
		checkpoint.SchemaVersion != 2 ||
		checkpoint.NovelID != novelID ||
		checkpoint.UpToEpisodeIndex != upToEpisodeIndex ||
		(expectedFingerprint != "" && checkpoint.GenerationFingerprint != expectedFingerprint) {
		return appextraction.EmptyCheckpoint(novelID, upToEpisodeIndex, expectedFingerprint)
	}
	return appextraction.NormalizeCheckpoint(checkpoint)
}

func (s *Server) saveExtractionCheckpoint(novelID string, upToEpisodeIndex string, checkpoint extractionCheckpoint) error {
	return s.extractionRuntime().SaveCheckpoint(novelID, upToEpisodeIndex, checkpoint)
}

func (s *Server) extractionCheckpointExists(novelID string, upToEpisodeIndex string) bool {
	return s.extractionRuntime().CheckpointExists(novelID, upToEpisodeIndex)
}

func (s *Server) recordExtractionUsage(run ai.UsageRun) error {
	return s.extractionRuntime().RecordUsage(run)
}

func (s *Server) novelTitle(ctx context.Context, novelID string) *string {
	return s.extractionRuntime().NovelTitle(ctx, novelID)
}

func resolvedProfileID(config *store.ResolvedAIGenerationConfig) *string {
	if config == nil || strings.TrimSpace(config.ProfileID) == "" {
		return nil
	}
	return &config.ProfileID
}

func resolvedProfileLabel(config *store.ResolvedAIGenerationConfig) *string {
	if config == nil || strings.TrimSpace(config.ProfileLabel) == "" {
		return nil
	}
	return &config.ProfileLabel
}

func resolvedModelID(config *store.ResolvedAIGenerationConfig) *string {
	if config == nil || strings.TrimSpace(config.ModelID) == "" {
		return nil
	}
	return &config.ModelID
}

func heuristicEpisodeIndexes(episodes []characters.HeuristicEpisode) []string {
	indexes := make([]string, 0, len(episodes))
	for _, episode := range episodes {
		indexes = append(indexes, episode.EpisodeIndex)
	}
	return indexes
}

func (s *Server) loadExtractionInputs(ctx context.Context, novelID string, upToEpisodeIndex string, maxChunkChars int, maxBatchChars int, afterEpisodeIndexes ...string) (extractionInputs, error) {
	afterEpisodeIndex := ""
	if len(afterEpisodeIndexes) > 0 {
		afterEpisodeIndex = strings.TrimSpace(afterEpisodeIndexes[0])
	}
	return s.extractionRuntime().LoadInputs(ctx, novelID, upToEpisodeIndex, maxChunkChars, maxBatchChars, afterEpisodeIndex)
}

func (s *Server) loadExtractionGenerationSeed(novelID string, upToEpisodeIndex string) ([]characters.GeneratedCharacter, *string, bool, error) {
	return s.extractionRuntime().LoadGenerationSeed(novelID, upToEpisodeIndex)
}

func extractionProcessedCovers(processedEpisodeIndex string, requestedEpisodeIndex string) bool {
	processedEpisodeIndex = strings.TrimSpace(processedEpisodeIndex)
	requestedEpisodeIndex = strings.TrimSpace(requestedEpisodeIndex)
	if processedEpisodeIndex == "" || requestedEpisodeIndex == "" {
		return false
	}
	return compareEpisodeString(processedEpisodeIndex, requestedEpisodeIndex) >= 0
}

func (s *Server) extractionReprocessFromEpisode(ctx context.Context, novelID string, processedEpisodeIndex *string, requestedUpToEpisodeIndex string) (string, error) {
	return s.extractionRuntime().ReprocessFromEpisode(ctx, novelID, processedEpisodeIndex, requestedUpToEpisodeIndex)
}

func earliestGeneratedEpisodeDigest(digests []characters.GeneratedEpisodeDigest, processedEpisodeIndex string) string {
	earliest := ""
	for _, digest := range digests {
		episodeIndex := strings.TrimSpace(digest.EpisodeIndex)
		if episodeIndex == "" || compareEpisodeString(episodeIndex, processedEpisodeIndex) > 0 {
			continue
		}
		if earliest == "" || compareEpisodeString(episodeIndex, earliest) < 0 {
			earliest = episodeIndex
		}
	}
	return earliest
}

func filterExtractionInputsAfter(inputs extractionInputs, processedEpisodeIndex string) extractionInputs {
	if strings.TrimSpace(processedEpisodeIndex) == "" {
		return inputs
	}
	filteredEpisodes := make([]characters.HeuristicEpisode, 0, len(inputs.Episodes))
	for _, episode := range inputs.Episodes {
		if compareEpisodeString(episode.EpisodeIndex, processedEpisodeIndex) > 0 {
			filteredEpisodes = append(filteredEpisodes, episode)
		}
	}
	chunks := []extractionChunk{}
	for _, batch := range inputs.Batches {
		for _, chunk := range batch.Chunks {
			if compareEpisodeString(chunk.EpisodeIndex, processedEpisodeIndex) > 0 {
				chunks = append(chunks, chunk)
			}
		}
	}
	return extractionInputs{
		Episodes: filteredEpisodes,
		Batches:  extraction.CreateBatchesWithBudget(chunks, extractionBatchBudget{}),
	}
}

func filterExtractionInputsFrom(inputs extractionInputs, fromEpisodeIndex string) extractionInputs {
	if strings.TrimSpace(fromEpisodeIndex) == "" {
		return inputs
	}
	filteredEpisodes := make([]characters.HeuristicEpisode, 0, len(inputs.Episodes))
	for _, episode := range inputs.Episodes {
		if compareEpisodeString(episode.EpisodeIndex, fromEpisodeIndex) >= 0 {
			filteredEpisodes = append(filteredEpisodes, episode)
		}
	}
	chunks := []extractionChunk{}
	for _, batch := range inputs.Batches {
		for _, chunk := range batch.Chunks {
			if compareEpisodeString(chunk.EpisodeIndex, fromEpisodeIndex) >= 0 {
				chunks = append(chunks, chunk)
			}
		}
	}
	return extractionInputs{
		Episodes: filteredEpisodes,
		Batches:  extraction.CreateBatchesWithBudget(chunks, extractionBatchBudget{}),
	}
}

func extractionInputTokens(episodes []characters.HeuristicEpisode) int {
	total := 0
	for _, episode := range episodes {
		total += estimateTokenCount(episode.Text)
	}
	return total
}

func extractionBatchUsageRequests(batches []extractionBatch) []ai.UsageRequest {
	requests := make([]ai.UsageRequest, 0, len(batches))
	for index, batch := range batches {
		requests = append(requests, extractionUsageRequestForBatch(index, batch))
	}
	return requests
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

func usageRequestsInputTokens(requests []ai.UsageRequest) int {
	total := 0
	for _, request := range requests {
		total += request.InputTokens
	}
	return total
}

func usageRequestsOutputTokens(requests []ai.UsageRequest) int {
	total := 0
	for _, request := range requests {
		total += request.OutputTokens
	}
	return total
}

func usageRequestsTotalTokens(requests []ai.UsageRequest) int {
	total := 0
	for _, request := range requests {
		if request.TotalTokens > 0 {
			total += request.TotalTokens
			continue
		}
		total += request.InputTokens + request.OutputTokens
	}
	return total
}

func (s *Server) nextExtractionRuntimeBatch(ctx context.Context, config *store.ResolvedAIGenerationConfig, novelID string, upToEpisodeIndex string, knownCharacters []characters.GeneratedCharacter, template extractionBatch, chunks []extractionChunk, unresolvedMentions ...[]characters.GeneratedUnresolvedMention) (extractionBatch, []extractionChunk, error) {
	pending := []characters.GeneratedUnresolvedMention(nil)
	if len(unresolvedMentions) > 0 {
		pending = unresolvedMentions[0]
	}
	return s.extractionRuntime().PlanRuntimeBatch(ctx, config, novelID, upToEpisodeIndex, knownCharacters, nil, template, chunks, pending)
}

func (s *Server) extractionRuntimeBatches(ctx context.Context, config *store.ResolvedAIGenerationConfig, novelID string, upToEpisodeIndex string, knownCharacters []characters.GeneratedCharacter, batch extractionBatch) ([]extractionBatch, error) {
	return extraction.PlanRuntimeBatches(batch, func(candidate extractionBatch) (bool, error) {
		return extractionBatchFitsContext(ctx, config, novelID, upToEpisodeIndex, knownCharacters, candidate)
	})
}

func splitOversizedExtractionChunkBatch(ctx context.Context, config *store.ResolvedAIGenerationConfig, novelID string, upToEpisodeIndex string, knownCharacters []characters.GeneratedCharacter, batch extractionBatch, unresolvedMentions ...[]characters.GeneratedUnresolvedMention) ([]extractionBatch, error) {
	return extraction.SplitOversizedChunkBatch(batch, func(candidate extractionBatch) (bool, error) {
		return extractionBatchFitsContext(ctx, config, novelID, upToEpisodeIndex, knownCharacters, candidate, unresolvedMentions...)
	})
}

func extractionBatchFitsContext(ctx context.Context, config *store.ResolvedAIGenerationConfig, novelID string, upToEpisodeIndex string, knownCharacters []characters.GeneratedCharacter, batch extractionBatch, unresolvedMentions ...[]characters.GeneratedUnresolvedMention) (bool, error) {
	if config == nil {
		return true, nil
	}
	stageStartedAt := time.Now()
	pending := []characters.GeneratedUnresolvedMention(nil)
	if len(unresolvedMentions) > 0 {
		pending = unresolvedMentions[0]
	}
	systemPrompt, userPrompt := extraction.BuildPromptWithUnresolved(novelID, upToEpisodeIndex, knownCharacters, batch, pending, config.SystemPrompt)
	logExtractionTiming("context_fit_prompt", stageStartedAt, "novelId", novelID, "upToEpisodeIndex", upToEpisodeIndex, "batch", batch.BatchIndex, "chunks", len(batch.Chunks), "knownCharacters", len(knownCharacters), "unresolvedMentions", len(pending))
	stageStartedAt = time.Now()
	responseFormat := extractionOpenRouterResponseFormat()
	promptTokens := estimateOpenRouterChatRequestTokens([]ai.ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}, nil, responseFormat)
	logExtractionTiming("context_fit_token_estimate", stageStartedAt, "novelId", novelID, "upToEpisodeIndex", upToEpisodeIndex, "batch", batch.BatchIndex, "promptTokens", promptTokens)
	stageStartedAt = time.Now()
	maxTokens, err := resolveOpenRouterMaxOutputTokens(ctx, config.APIKey, config.ModelID, config.ProviderOrder, extractionDefaultMaxTokens, promptTokens)
	status := "ok"
	if err != nil {
		status = "error"
	}
	logExtractionTiming("context_fit_max_tokens", stageStartedAt, "status", status, "novelId", novelID, "upToEpisodeIndex", upToEpisodeIndex, "batch", batch.BatchIndex, "promptTokens", promptTokens, "maxTokens", maxTokens)
	if err == nil {
		return maxTokens >= extractionMinimumCompletionTokens, nil
	}
	if errors.Is(err, errOpenRouterContextTooLarge) {
		return false, nil
	}
	return false, err
}

func (s *Server) generateOpenRouterExtractionBatch(ctx context.Context, config *store.ResolvedAIGenerationConfig, novelID string, upToEpisodeIndex string, knownCharacters []characters.GeneratedCharacter, batch extractionBatch, unresolvedMentions ...[]characters.GeneratedUnresolvedMention) (extractionBatchResult, error) {
	pending := []characters.GeneratedUnresolvedMention(nil)
	if len(unresolvedMentions) > 0 {
		pending = unresolvedMentions[0]
	}
	result, err := s.extractionRuntime().GenerateBatch(ctx, config, novelID, upToEpisodeIndex, knownCharacters, nil, batch, pending)
	if err != nil {
		return extractionBatchResult{}, err
	}
	return extractionBatchResult{Delta: result.Delta, Usage: result.Usage}, nil
}

func extractionOpenRouterResponseFormat() map[string]any {
	return extractionruntime.ExtractionOpenRouterResponseFormat()
}

func extractionBatchTimingEvent(progress extractionBatchProgress) map[string]any {
	return map[string]any{
		"type":                    "batchTiming",
		"batchIndex":              progress.Batch.BatchIndex,
		"batchCount":              progress.Batch.BatchCount,
		"episodeIndexes":          progress.Batch.EpisodeIndexes,
		"chunkCount":              len(progress.Batch.Chunks),
		"elapsedMs":               progress.ElapsedMs,
		"generatedCharacterCount": progress.GeneratedCharacterCount,
		"mergedCharacterCount":    progress.MergedCharacterCount,
		"message":                 "batch " + strconv.Itoa(progress.Batch.BatchIndex) + "/" + strconv.Itoa(progress.Batch.BatchCount) + " の生成を完了しました。",
	}
}

func playgroundStatusEvent(stage string, message string, progress int, step int) map[string]any {
	return map[string]any{
		"type":      "status",
		"stage":     stage,
		"message":   message,
		"progress":  progress,
		"step":      step,
		"stepCount": 4,
	}
}

func playgroundBatchStatusEvent(batch extractionBatch) map[string]any {
	event := playgroundStatusEvent("batch", "batch "+strconv.Itoa(batch.BatchIndex)+"/"+strconv.Itoa(batch.BatchCount)+" を生成しています。", 70, 3)
	event["batchIndex"] = batch.BatchIndex
	event["batchCount"] = batch.BatchCount
	return event
}

func (s *Server) extractionPromptPreview(ctx context.Context, novelID string, upToEpisodeIndex string, resolvedConfig *store.ResolvedAIGenerationConfig) (map[string]any, error) {
	preparedPreview, err := s.extractionRuntime().PreparePreview(ctx, novelID, upToEpisodeIndex, resolvedConfig)
	if err != nil {
		return nil, err
	}
	return promptPreviewToMap(preparedPreview.Preview), nil
}

func promptPreviewToMap(preview appextraction.PromptPreview) map[string]any {
	batches := make([]map[string]any, 0, len(preview.Batches))
	for _, batch := range preview.Batches {
		chunks := make([]map[string]any, 0, len(batch.Chunks))
		for _, chunk := range batch.Chunks {
			chunks = append(chunks, map[string]any{
				"episodeIndex": chunk.EpisodeIndex,
				"title":        chunk.Title,
				"chapter":      chunk.Chapter,
				"subchapter":   chunk.Subchapter,
				"chunkIndex":   chunk.ChunkIndex,
				"chunkCount":   chunk.ChunkCount,
				"text":         chunk.Text,
			})
		}
		batches = append(batches, map[string]any{
			"batchIndex":     batch.BatchIndex,
			"batchCount":     batch.BatchCount,
			"episodeIndexes": batch.EpisodeIndexes,
			"chunkCount":     len(batch.Chunks),
			"chunks":         chunks,
		})
	}
	return map[string]any{
		"systemPrompt": preview.SystemPrompt,
		"batches":      batches,
	}
}

func promptPreviewEpisodeIndexes(preview appextraction.PromptPreview) []string {
	values := []string{}
	seen := map[string]bool{}
	for _, batch := range preview.Batches {
		for _, episodeIndex := range batch.EpisodeIndexes {
			if episodeIndex == "" || seen[episodeIndex] {
				continue
			}
			seen[episodeIndex] = true
			values = append(values, episodeIndex)
		}
	}
	return values
}

func promptPreviewChunkCount(preview appextraction.PromptPreview) int {
	total := 0
	for _, batch := range preview.Batches {
		total += batch.ChunkCount
	}
	return total
}

func readerDocumentBodyText(document library.ReaderDocument) string {
	parts := []string{}
	for _, block := range document.Blocks {
		if block.Section != "body" {
			continue
		}
		if block.Type == "paragraph" {
			if text := strings.TrimSpace(extraction.RenderInlineTokens(block.Inlines)); text != "" {
				parts = append(parts, text)
			}
			continue
		}
		if strings.TrimSpace(block.PlainText) != "" {
			parts = append(parts, strings.TrimSpace(block.PlainText))
			continue
		}
		if strings.TrimSpace(block.Text) != "" {
			parts = append(parts, strings.TrimSpace(block.Text))
		}
	}
	return strings.Join(parts, "\n")
}

func previewEpisodeIndexes(preview map[string]any) []string {
	result := []string{}
	batches, ok := preview["batches"].([]map[string]any)
	if !ok {
		return result
	}
	seen := map[string]bool{}
	for _, batch := range batches {
		indexes, ok := batch["episodeIndexes"].([]string)
		if !ok {
			continue
		}
		for _, index := range indexes {
			if !seen[index] {
				seen[index] = true
				result = append(result, index)
			}
		}
	}
	return result
}

func previewChunkCount(preview map[string]any) int {
	batches, ok := preview["batches"].([]map[string]any)
	if !ok {
		return 0
	}
	total := 0
	for _, batch := range batches {
		count, ok := batch["chunkCount"].(int)
		if ok {
			total += count
		}
	}
	return total
}

func compareEpisodeString(left string, right string) int {
	leftNumber, leftErr := strconv.Atoi(left)
	rightNumber, rightErr := strconv.Atoi(right)
	if leftErr == nil && rightErr == nil {
		if leftNumber < rightNumber {
			return -1
		}
		if leftNumber > rightNumber {
			return 1
		}
		return 0
	}
	return strings.Compare(left, right)
}
