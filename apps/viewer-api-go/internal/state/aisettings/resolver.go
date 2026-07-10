package aisettings

import (
	"errors"
	"os"
	"strings"
)

var ErrAIGenerationProfileNotFound = errors.New("AI generation profile was not found")

type ResolvedAIGenerationConfig struct {
	ProfileID                      string
	ProfileLabel                   string
	APIKey                         string
	ModelID                        string
	ProviderOrder                  []string
	AllowFallbacks                 bool
	RequireParameters              bool
	SystemPrompt                   *string
	ExtractionNameDiscoveryModelID string
}

type AIGenerationTransientConfig struct {
	ModelID              *string
	ProviderOrder        []string
	ProviderOrderSet     bool
	AllowFallbacks       *bool
	RequireParameters    *bool
	SystemPromptOverride *string
}

func (r *Repository) ResolveGoogleBooksAPIKey() (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	doc, err := r.readAIGenerationSettingsDocument()
	if err != nil {
		return "", err
	}
	apiKey, err := decryptAIAPIKey(normalizeAIAPIKeyDocument(doc.SharedProviders.GoogleBooks))
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(apiKey) != "" {
		return strings.TrimSpace(apiKey), nil
	}
	return strings.TrimSpace(os.Getenv("GOOGLE_BOOKS_API_KEY")), nil
}

func (r *Repository) GoogleBooksAPIKeyConfigured() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	doc, err := r.readAIGenerationSettingsDocument()
	if err != nil {
		return strings.TrimSpace(os.Getenv("GOOGLE_BOOKS_API_KEY")) != ""
	}
	return hasStoredAIKey(normalizeAIAPIKeyDocument(doc.SharedProviders.GoogleBooks)) ||
		strings.TrimSpace(os.Getenv("GOOGLE_BOOKS_API_KEY")) != ""
}

func (r *Repository) ResolveActiveAIGenerationConfig() (*ResolvedAIGenerationConfig, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	doc, err := r.readAIGenerationSettingsDocument()
	if err != nil {
		return nil, err
	}
	if doc.PreferredMode != "llm" || !activeOpenRouterProfileReady(doc) {
		return nil, nil
	}
	profile := resolveAIProfile(doc, doc.SelectedProfileID)
	return resolveAIGenerationConfigFromDocument(doc, profile, nil)
}

func (r *Repository) ResolveAIGenerationConfigOverride(profileID *string, transient *AIGenerationTransientConfig) (*ResolvedAIGenerationConfig, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	doc, err := r.readAIGenerationSettingsDocument()
	if err != nil {
		return nil, err
	}
	if len(doc.Profiles) == 0 {
		return nil, nil
	}
	selectedProfileID := doc.SelectedProfileID
	if profileID != nil {
		trimmed := strings.TrimSpace(*profileID)
		if trimmed != "" {
			selectedProfileID = &trimmed
		}
	}
	profile, found := resolveAIProfileForOverride(doc, selectedProfileID, profileID != nil)
	if !found {
		return nil, ErrAIGenerationProfileNotFound
	}
	return resolveAIGenerationConfigFromDocument(doc, profile, transient)
}

func resolveAIGenerationConfigFromDocument(doc aiGenerationSettingsDocument, profile aiGenerationProfileRecord, transient *AIGenerationTransientConfig) (*ResolvedAIGenerationConfig, error) {
	credentials := normalizeAIProfileCredentialsDocument(profile.Credentials)
	source := normalizeAIAPIKeyDocument(doc.SharedProviders.OpenRouter)
	if credentials.Source == "custom" {
		source = credentials.aiAPIKeyDocument
	}
	apiKey, err := decryptAIAPIKey(source)
	if err != nil {
		return nil, err
	}
	modelID := normalizeStringPtr(profile.ModelID)
	providerOrder := normalizeStringList(profile.ProviderOrder)
	allowFallbacks := profile.AllowFallbacks
	requireParameters := profile.RequireParameters
	var systemPrompt *string
	if transient != nil {
		if transient.ModelID != nil {
			modelID = normalizeStringPtr(transient.ModelID)
		}
		if transient.ProviderOrderSet {
			providerOrder = normalizeStringList(transient.ProviderOrder)
		}
		if transient.AllowFallbacks != nil {
			allowFallbacks = *transient.AllowFallbacks
		}
		if transient.RequireParameters != nil {
			requireParameters = *transient.RequireParameters
		}
		systemPrompt = normalizeStringPtr(transient.SystemPromptOverride)
	}
	if strings.TrimSpace(apiKey) == "" || modelID == nil || strings.TrimSpace(*modelID) == "" {
		return nil, nil
	}
	return &ResolvedAIGenerationConfig{
		ProfileID:                      profile.ID,
		ProfileLabel:                   profile.Label,
		APIKey:                         strings.TrimSpace(apiKey),
		ModelID:                        strings.TrimSpace(*modelID),
		ProviderOrder:                  providerOrder,
		AllowFallbacks:                 allowFallbacks,
		RequireParameters:              requireParameters,
		SystemPrompt:                   systemPrompt,
		ExtractionNameDiscoveryModelID: stringValue(doc.ExtractionStrategyModels.NameDiscoveryModelID),
	}, nil
}

func resolveAIProfile(doc aiGenerationSettingsDocument, profileID *string) aiGenerationProfileRecord {
	profile := doc.Profiles[0]
	if profileID == nil {
		return profile
	}
	selected := strings.TrimSpace(*profileID)
	for _, candidate := range doc.Profiles {
		if candidate.ID == selected {
			return candidate
		}
	}
	return profile
}

func resolveAIProfileForOverride(doc aiGenerationSettingsDocument, profileID *string, explicit bool) (aiGenerationProfileRecord, bool) {
	if !explicit {
		return resolveAIProfile(doc, profileID), true
	}
	selected := ""
	if profileID != nil {
		selected = strings.TrimSpace(*profileID)
	}
	if selected == "" {
		return resolveAIProfile(doc, nil), true
	}
	for _, candidate := range doc.Profiles {
		if candidate.ID == selected {
			return candidate, true
		}
	}
	return aiGenerationProfileRecord{}, false
}

func aiGenerationServiceConfigured() bool {
	value := strings.TrimSpace(os.Getenv("AI_GENERATION_SERVICE_API_BASE_URL"))
	if value == "disabled" {
		return false
	}
	if value != "" {
		return true
	}
	return strings.TrimSpace(os.Getenv("NODE_ENV")) != "test"
}

func internalAIGenerationConfigured(doc aiGenerationSettingsDocument) bool {
	return activeOpenRouterProfileReady(doc)
}

func effectiveAIGenerationMode(doc aiGenerationSettingsDocument) string {
	if doc.PreferredMode == "heuristic" {
		return "heuristic"
	}
	if internalAIGenerationConfigured(doc) {
		return "openrouter"
	}
	return "disabled"
}

func activeOpenRouterProfileReady(doc aiGenerationSettingsDocument) bool {
	if len(doc.Profiles) == 0 {
		return false
	}
	active := doc.Profiles[0]
	if doc.SelectedProfileID != nil {
		for _, profile := range doc.Profiles {
			if profile.ID == *doc.SelectedProfileID {
				active = profile
				break
			}
		}
	}
	if active.ModelID == nil || strings.TrimSpace(*active.ModelID) == "" {
		return false
	}
	credentials := normalizeAIProfileCredentialsDocument(active.Credentials)
	if credentials.Source == "custom" {
		return hasStoredAIKey(credentials.aiAPIKeyDocument)
	}
	return hasStoredAIKey(normalizeAIAPIKeyDocument(doc.SharedProviders.OpenRouter))
}
