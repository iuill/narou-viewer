package store

import (
	"narou-viewer/apps/viewer-api-go/internal/ai"
	"narou-viewer/apps/viewer-api-go/internal/state/aisettings"
)

var ErrAIGenerationProfileNotFound = aisettings.ErrAIGenerationProfileNotFound

type AIGenerationSettingsCryptoError = aisettings.AIGenerationSettingsCryptoError
type AIGenerationSettingsUpdate = aisettings.AIGenerationSettingsUpdate
type AISharedProvidersInput = aisettings.AISharedProvidersInput
type AIProviderCredentialInput = aisettings.AIProviderCredentialInput
type AIExtractionStrategyModelsInput = aisettings.AIExtractionStrategyModelsInput
type AIExtractionRuntimeInput = aisettings.AIExtractionRuntimeInput
type AIProfileInput = aisettings.AIProfileInput
type AIProfileCredentialsInput = aisettings.AIProfileCredentialsInput
type ResolvedAIGenerationConfig = aisettings.ResolvedAIGenerationConfig
type AIGenerationTransientConfig = aisettings.AIGenerationTransientConfig

func IsAIGenerationSettingsCryptoError(err error) bool {
	return aisettings.IsAIGenerationSettingsCryptoError(err)
}

func (s *Store) GetAIGenerationSettings() (ai.SettingsResponse, error) {
	return s.aiSettings.GetAIGenerationSettings()
}

func (s *Store) PutAIGenerationSettings(input AIGenerationSettingsUpdate) (ai.SettingsResponse, error) {
	return s.aiSettings.PutAIGenerationSettings(input)
}

func (s *Store) PutAIGenerationPreferredMode(preferredMode string) (ai.SettingsResponse, error) {
	return s.aiSettings.PutAIGenerationPreferredMode(preferredMode)
}

func (s *Store) ResolveActiveAIGenerationConfig() (*ResolvedAIGenerationConfig, error) {
	return s.aiSettings.ResolveActiveAIGenerationConfig()
}

func (s *Store) ResolveAIGenerationConfigOverride(profileID *string, transient *AIGenerationTransientConfig) (*ResolvedAIGenerationConfig, error) {
	return s.aiSettings.ResolveAIGenerationConfigOverride(profileID, transient)
}

func (s *Store) ResolveGoogleBooksAPIKey() (string, error) {
	return s.aiSettings.ResolveGoogleBooksAPIKey()
}

func (s *Store) GoogleBooksAPIKeyConfigured() bool {
	return s.aiSettings.GoogleBooksAPIKeyConfigured()
}
