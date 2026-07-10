package aisettings

import (
	"errors"
	"log"
	"narou-viewer/apps/viewer-api-go/internal/ai"
	"narou-viewer/apps/viewer-api-go/internal/state/yamlfile"
	"os"
	"path/filepath"
	"strings"
)

type aiGenerationSettingsDocument struct {
	SchemaVersion                 int                                `yaml:"schema_version"`
	Revision                      int                                `yaml:"revision"`
	PreferredMode                 string                             `yaml:"preferred_mode"`
	SelectedProfileID             *string                            `yaml:"selected_profile_id"`
	SharedProviders               aiSharedProvidersDocument          `yaml:"shared_providers"`
	Profiles                      []aiGenerationProfileRecord        `yaml:"profiles"`
	ExtractionStrategyModels      aiExtractionStrategyModelsDocument `yaml:"extraction_strategy_models,omitempty"`
	LegacyCharacterStrategyModels aiExtractionStrategyModelsDocument `yaml:"character_summary_strategy_models,omitempty"`
}

type aiSharedProvidersDocument struct {
	OpenRouter  aiAPIKeyDocument `yaml:"openrouter"`
	GoogleBooks aiAPIKeyDocument `yaml:"google_books"`
}

type aiAPIKeyDocument struct {
	APIKey          *string `yaml:"api_key"`
	APIKeyEncrypted string  `yaml:"api_key_encrypted,omitempty"`
	APIKeySalt      string  `yaml:"api_key_salt,omitempty"`
	APIKeyIV        string  `yaml:"api_key_iv,omitempty"`
	APIKeyTag       string  `yaml:"api_key_tag,omitempty"`
	APIKeyVersion   int     `yaml:"api_key_version,omitempty"`
	UpdatedAt       *string `yaml:"updated_at"`
}

type aiExtractionStrategyModelsDocument struct {
	NameDiscoveryModelID *string `yaml:"name_discovery_model_id,omitempty"`
}

type aiGenerationProfileRecord struct {
	ID                string                       `yaml:"id"`
	Label             string                       `yaml:"label"`
	Provider          string                       `yaml:"provider"`
	Credentials       aiProfileCredentialsDocument `yaml:"credentials"`
	ModelID           *string                      `yaml:"model_id"`
	ProviderOrder     []string                     `yaml:"provider_order"`
	AllowFallbacks    bool                         `yaml:"allow_fallbacks"`
	RequireParameters bool                         `yaml:"require_parameters"`
	UpdatedAt         *string                      `yaml:"updated_at"`
}

type aiProfileCredentialsDocument struct {
	aiAPIKeyDocument `yaml:",inline"`
	Source           string `yaml:"source"`
}

func (r *Repository) readAIGenerationSettingsDocument() (aiGenerationSettingsDocument, error) {
	var raw aiGenerationSettingsDocument
	path := filepath.Join(r.stateDir, FileName)
	if err := yamlfile.Read(path, &raw); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return emptyAiGenerationSettingsDocument(), nil
		}
		return aiGenerationSettingsDocument{}, err
	}
	doc := normalizeAIGenerationSettingsDocument(raw)
	migrated, changed, err := migratePlaintextAIGenerationAPIKeys(doc)
	if err != nil {
		return aiGenerationSettingsDocument{}, err
	}
	if changed {
		if err := yamlfile.WriteAtomicMode(path, migrated, 0o600); err != nil {
			log.Printf("AI generation settings plaintext key migration could not be persisted: %v", err)
			return doc, nil
		}
		doc = migrated
	}
	return doc, nil
}

func emptyAiGenerationSettingsDocument() aiGenerationSettingsDocument {
	profileID := defaultAIProfileID
	return aiGenerationSettingsDocument{
		SchemaVersion:     2,
		Revision:          0,
		PreferredMode:     "heuristic",
		SelectedProfileID: &profileID,
		SharedProviders: aiSharedProvidersDocument{
			OpenRouter:  aiAPIKeyDocument{APIKey: nil, UpdatedAt: nil},
			GoogleBooks: aiAPIKeyDocument{APIKey: nil, UpdatedAt: nil},
		},
		Profiles: []aiGenerationProfileRecord{
			{
				ID:       defaultAIProfileID,
				Label:    "Default",
				Provider: "openrouter",
				Credentials: aiProfileCredentialsDocument{
					aiAPIKeyDocument: aiAPIKeyDocument{APIKey: nil, UpdatedAt: nil},
					Source:           "shared",
				},
				ModelID:           nil,
				ProviderOrder:     []string{},
				AllowFallbacks:    false,
				RequireParameters: true,
				UpdatedAt:         nil,
			},
		},
	}
}

func normalizeAIGenerationSettingsDocument(raw aiGenerationSettingsDocument) aiGenerationSettingsDocument {
	doc := emptyAiGenerationSettingsDocument()
	if raw.Revision >= 0 {
		doc.Revision = raw.Revision
	}
	if raw.PreferredMode == "llm" || raw.PreferredMode == "heuristic" {
		doc.PreferredMode = raw.PreferredMode
	}
	doc.SharedProviders.OpenRouter = normalizeAIAPIKeyDocument(raw.SharedProviders.OpenRouter)
	doc.SharedProviders.GoogleBooks = normalizeAIAPIKeyDocument(raw.SharedProviders.GoogleBooks)
	strategyModels := raw.ExtractionStrategyModels
	if normalizeStringPtr(strategyModels.NameDiscoveryModelID) == nil {
		strategyModels = raw.LegacyCharacterStrategyModels
	}
	doc.ExtractionStrategyModels = aiExtractionStrategyModelsDocument{
		NameDiscoveryModelID: normalizeStringPtr(strategyModels.NameDiscoveryModelID),
	}
	profiles := make([]aiGenerationProfileRecord, 0, len(raw.Profiles))
	for _, profile := range raw.Profiles {
		id := strings.TrimSpace(profile.ID)
		label := strings.TrimSpace(profile.Label)
		if id == "" || label == "" {
			continue
		}
		profiles = append(profiles, aiGenerationProfileRecord{
			ID:                id,
			Label:             label,
			Provider:          normalizedAIProvider(profile.Provider),
			Credentials:       normalizeAIProfileCredentialsDocument(profile.Credentials),
			ModelID:           normalizeStringPtr(profile.ModelID),
			ProviderOrder:     normalizeStringList(profile.ProviderOrder),
			AllowFallbacks:    profile.AllowFallbacks,
			RequireParameters: profile.RequireParameters,
			UpdatedAt:         stringPtrOrNil(profile.UpdatedAt),
		})
	}
	if len(profiles) > 0 {
		doc.Profiles = profiles
	}
	selected := normalizeStringPtr(raw.SelectedProfileID)
	if selected != nil && hasAIProfile(doc.Profiles, *selected) {
		doc.SelectedProfileID = selected
	} else if len(doc.Profiles) > 0 {
		doc.SelectedProfileID = &doc.Profiles[0].ID
	} else {
		doc.SelectedProfileID = nil
	}
	return doc
}

func toAIGenerationSettingsResponse(doc aiGenerationSettingsDocument) ai.SettingsResponse {
	shared := normalizeAIAPIKeyDocument(doc.SharedProviders.OpenRouter)
	googleBooks := normalizeAIAPIKeyDocument(doc.SharedProviders.GoogleBooks)
	profiles := make([]ai.Profile, 0, len(doc.Profiles))
	for _, profile := range doc.Profiles {
		credentials := normalizeAIProfileCredentialsDocument(profile.Credentials)
		hasKey := hasStoredAIKey(shared)
		updatedAt := shared.UpdatedAt
		masked := maskAIKeyDocument(shared)
		if credentials.Source == "custom" {
			hasKey = hasStoredAIKey(credentials.aiAPIKeyDocument)
			updatedAt = credentials.UpdatedAt
			masked = maskAIKeyDocument(credentials.aiAPIKeyDocument)
		}
		profiles = append(profiles, ai.Profile{
			ID:       profile.ID,
			Label:    profile.Label,
			Provider: profile.Provider,
			Credentials: ai.ProfileCredentials{
				Source:       credentials.Source,
				HasAPIKey:    hasKey,
				APIKeyMasked: masked,
				UpdatedAt:    updatedAt,
			},
			ModelID:           profile.ModelID,
			ProviderOrder:     normalizeStringList(profile.ProviderOrder),
			AllowFallbacks:    profile.AllowFallbacks,
			RequireParameters: profile.RequireParameters,
			UpdatedAt:         profile.UpdatedAt,
		})
	}
	return ai.SettingsResponse{
		APIBaseURLConfigured:       internalAIGenerationConfigured(doc),
		MasterPassphraseConfigured: aiSettingsMasterPassphrase() != "",
		PreferredMode:              doc.PreferredMode,
		EffectiveGenerationMode:    effectiveAIGenerationMode(doc),
		Settings: ai.SettingsMetadata{
			SelectedProfileID: doc.SelectedProfileID,
			SharedProviders: ai.SharedProviders{
				OpenRouter: ai.ProviderMetadata{
					HasAPIKey:    hasStoredAIKey(shared),
					APIKeyMasked: maskAIKeyDocument(shared),
					UpdatedAt:    shared.UpdatedAt,
				},
				GoogleBooks: ai.ProviderMetadata{
					HasAPIKey:    hasStoredAIKey(googleBooks),
					APIKeyMasked: maskAIKeyDocument(googleBooks),
					UpdatedAt:    googleBooks.UpdatedAt,
				},
			},
			Profiles: profiles,
			ExtractionStrategyModels: ai.ExtractionStrategyModels{
				NameDiscoveryModelID: normalizeStringPtr(doc.ExtractionStrategyModels.NameDiscoveryModelID),
			},
		},
	}
}

func normalizeAIAPIKeyDocument(value aiAPIKeyDocument) aiAPIKeyDocument {
	return aiAPIKeyDocument{
		APIKey:          normalizeStringPtr(value.APIKey),
		APIKeyEncrypted: strings.TrimSpace(value.APIKeyEncrypted),
		APIKeySalt:      strings.TrimSpace(value.APIKeySalt),
		APIKeyIV:        strings.TrimSpace(value.APIKeyIV),
		APIKeyTag:       strings.TrimSpace(value.APIKeyTag),
		APIKeyVersion:   value.APIKeyVersion,
		UpdatedAt:       stringPtrOrNil(value.UpdatedAt),
	}
}

func normalizeAIProfileCredentialsDocument(value aiProfileCredentialsDocument) aiProfileCredentialsDocument {
	source := "shared"
	if value.Source == "custom" {
		source = "custom"
	}
	return aiProfileCredentialsDocument{
		aiAPIKeyDocument: normalizeAIAPIKeyDocument(value.aiAPIKeyDocument),
		Source:           source,
	}
}

func toAIAPIKeyDocument(input AIProviderCredentialInput, existing aiAPIKeyDocument, now string) (aiAPIKeyDocument, error) {
	if !input.APIKeySet {
		preserved := normalizeAIAPIKeyDocument(existing)
		if input.UpdatedAtSet {
			preserved.UpdatedAt = updatedAtOrNow(input.UpdatedAt, now)
		}
		return preserved, nil
	}
	key := normalizeStringPtr(input.APIKey)
	if key == nil {
		return aiAPIKeyDocument{APIKey: nil, UpdatedAt: updatedAtOrNow(input.UpdatedAt, now)}, nil
	}
	encrypted, err := encryptAIAPIKey(*key)
	if err != nil {
		return aiAPIKeyDocument{}, err
	}
	encrypted.UpdatedAt = updatedAtOrNow(input.UpdatedAt, now)
	return encrypted, nil
}

func toAIProfileCredentialsDocument(input AIProfileCredentialsInput, existing aiProfileCredentialsDocument, now string) (aiProfileCredentialsDocument, error) {
	source := "shared"
	if input.Source == "" {
		source = normalizeAIProfileCredentialsDocument(existing).Source
	} else if input.Source == "custom" {
		source = "custom"
	}
	if source != "custom" {
		return aiProfileCredentialsDocument{
			aiAPIKeyDocument: aiAPIKeyDocument{APIKey: nil, UpdatedAt: updatedAtOrNow(input.UpdatedAt, now)},
			Source:           source,
		}, nil
	}
	keyDocument, err := toAIAPIKeyDocument(
		AIProviderCredentialInput{
			APIKey:       input.APIKey,
			APIKeySet:    input.APIKeySet,
			UpdatedAt:    input.UpdatedAt,
			UpdatedAtSet: input.UpdatedAtSet,
		},
		existing.aiAPIKeyDocument,
		now,
	)
	if err != nil {
		return aiProfileCredentialsDocument{}, err
	}
	return aiProfileCredentialsDocument{aiAPIKeyDocument: keyDocument, Source: source}, nil
}

func hasStoredAIKey(value aiAPIKeyDocument) bool {
	return normalizeStringPtr(value.APIKey) != nil || strings.TrimSpace(value.APIKeyEncrypted) != ""
}

func maskAIKeyDocument(value aiAPIKeyDocument) *string {
	key := normalizeStringPtr(value.APIKey)
	if key == nil {
		if strings.TrimSpace(value.APIKeyEncrypted) == "" {
			return nil
		}
		masked := "********"
		return &masked
	}
	if len(*key) <= 8 {
		masked := "********"
		return &masked
	}
	masked := (*key)[:4] + "..." + (*key)[len(*key)-4:]
	return &masked
}

func hasAIProfile(profiles []aiGenerationProfileRecord, id string) bool {
	for _, profile := range profiles {
		if profile.ID == id {
			return true
		}
	}
	return false
}

func normalizedAIProvider(provider string) string {
	if provider == "openrouter" {
		return "openrouter"
	}
	return "openrouter"
}
