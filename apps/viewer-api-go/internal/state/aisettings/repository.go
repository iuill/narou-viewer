package aisettings

import (
	"narou-viewer/apps/viewer-api-go/internal/ai"
	"narou-viewer/apps/viewer-api-go/internal/state/yamlfile"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	FileName           = "ai_generation_settings.yaml"
	defaultAIProfileID = "default"
	timestampFormat    = "2006-01-02T15:04:05.000Z07:00"
)

type Repository struct {
	stateDir string
	mu       sync.Mutex
}

func NewRepository(stateDir string) *Repository {
	return &Repository{stateDir: stateDir}
}

func (r *Repository) Ensure() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	path := filepath.Join(r.stateDir, FileName)
	if err := yamlfile.EnsureMode(path, emptyAiGenerationSettingsDocument(), 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

type AIGenerationSettingsUpdate struct {
	PreferredMode            *string
	SelectedProfileID        *string
	SharedProviders          *AISharedProvidersInput
	Profiles                 []AIProfileInput
	ProfilesSet              bool
	ExtractionStrategyModels *AIExtractionStrategyModelsInput
}

type AISharedProvidersInput struct {
	OpenRouter  AIProviderCredentialInput
	GoogleBooks AIProviderCredentialInput
}

type AIProviderCredentialInput struct {
	APIKey       *string
	APIKeySet    bool
	UpdatedAt    *string
	UpdatedAtSet bool
}

type AIExtractionStrategyModelsInput struct {
	NameDiscoveryModelID *string
}

type AIProfileInput struct {
	ID                string
	Label             string
	Provider          string
	Credentials       AIProfileCredentialsInput
	ModelID           *string
	ProviderOrder     []string
	AllowFallbacks    bool
	RequireParameters bool
	UpdatedAt         *string
}

type AIProfileCredentialsInput struct {
	Source       string
	APIKey       *string
	APIKeySet    bool
	UpdatedAt    *string
	UpdatedAtSet bool
}

func (r *Repository) GetAIGenerationSettings() (ai.SettingsResponse, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	doc, err := r.readAIGenerationSettingsDocument()
	if err != nil {
		return ai.SettingsResponse{}, err
	}
	return toAIGenerationSettingsResponse(doc), nil
}

func (r *Repository) PutAIGenerationSettings(input AIGenerationSettingsUpdate) (ai.SettingsResponse, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	doc, err := r.readAIGenerationSettingsDocument()
	if err != nil {
		return ai.SettingsResponse{}, err
	}
	now := isoNow()
	doc.Revision++
	if input.PreferredMode != nil {
		doc.PreferredMode = *input.PreferredMode
	}
	if input.SharedProviders != nil {
		doc.SharedProviders.OpenRouter, err = toAIAPIKeyDocument(input.SharedProviders.OpenRouter, doc.SharedProviders.OpenRouter, now)
		if err != nil {
			return ai.SettingsResponse{}, err
		}
		doc.SharedProviders.GoogleBooks, err = toAIAPIKeyDocument(input.SharedProviders.GoogleBooks, doc.SharedProviders.GoogleBooks, now)
		if err != nil {
			return ai.SettingsResponse{}, err
		}
	}
	if input.ExtractionStrategyModels != nil {
		doc.ExtractionStrategyModels.NameDiscoveryModelID = normalizeStringPtr(input.ExtractionStrategyModels.NameDiscoveryModelID)
	}
	if input.ProfilesSet {
		existingProfiles := make(map[string]aiGenerationProfileRecord, len(doc.Profiles))
		for _, profile := range doc.Profiles {
			existingProfiles[profile.ID] = profile
		}
		profiles := make([]aiGenerationProfileRecord, 0, len(input.Profiles))
		for index, profile := range input.Profiles {
			id := strings.TrimSpace(profile.ID)
			if id == "" {
				if index == 0 {
					id = defaultAIProfileID
				} else {
					id = "profile-" + intString(index+1)
				}
			}
			credentials, err := toAIProfileCredentialsDocument(profile.Credentials, existingProfiles[id].Credentials, now)
			if err != nil {
				return ai.SettingsResponse{}, err
			}
			profiles = append(profiles, aiGenerationProfileRecord{
				ID:                id,
				Label:             strings.TrimSpace(profile.Label),
				Provider:          normalizedAIProvider(profile.Provider),
				Credentials:       credentials,
				ModelID:           normalizeStringPtr(profile.ModelID),
				ProviderOrder:     normalizeStringList(profile.ProviderOrder),
				AllowFallbacks:    profile.AllowFallbacks,
				RequireParameters: profile.RequireParameters,
				UpdatedAt:         updatedAtOrNow(profile.UpdatedAt, now),
			})
		}
		doc.Profiles = profiles
	}
	if input.SelectedProfileID != nil {
		selected := strings.TrimSpace(*input.SelectedProfileID)
		if selected == "" {
			doc.SelectedProfileID = nil
		} else {
			doc.SelectedProfileID = &selected
		}
	}
	doc = normalizeAIGenerationSettingsDocument(doc)
	if err := yamlfile.WriteAtomicMode(filepath.Join(r.stateDir, FileName), doc, 0o600); err != nil {
		return ai.SettingsResponse{}, err
	}
	return toAIGenerationSettingsResponse(doc), nil
}

func (r *Repository) PutAIGenerationPreferredMode(preferredMode string) (ai.SettingsResponse, error) {
	return r.PutAIGenerationSettings(AIGenerationSettingsUpdate{PreferredMode: &preferredMode})
}
