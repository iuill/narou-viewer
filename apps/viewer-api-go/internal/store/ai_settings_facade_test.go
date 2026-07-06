package store

import (
	"errors"
	"testing"
)

func TestAIGenerationSettingsFacadeDelegatesToRepository(t *testing.T) {
	t.Setenv("AI_GENERATION_SETTINGS_MASTER_PASSPHRASE", "test-passphrase")
	store := New(t.TempDir())
	if err := store.Initialize(); err != nil {
		t.Fatalf("Initialize returned error: %v", err)
	}

	preferred, err := store.PutAIGenerationPreferredMode("llm")
	if err != nil {
		t.Fatalf("PutAIGenerationPreferredMode returned error: %v", err)
	}
	if preferred.PreferredMode != "llm" {
		t.Fatalf("preferred mode was not persisted: %+v", preferred)
	}

	modelID := "openrouter/auto"
	apiKey := "sk-test-secret-value"
	googleBooksAPIKey := "google-books-secret-value"
	if _, err := store.PutAIGenerationSettings(AIGenerationSettingsUpdate{
		SharedProviders: &AISharedProvidersInput{
			OpenRouter:  AIProviderCredentialInput{APIKey: &apiKey, APIKeySet: true},
			GoogleBooks: AIProviderCredentialInput{APIKey: &googleBooksAPIKey, APIKeySet: true},
		},
		ProfilesSet: true,
		Profiles: []AIProfileInput{{
			ID:                "default",
			Label:             "Default",
			Credentials:       AIProfileCredentialsInput{Source: "shared"},
			ModelID:           &modelID,
			RequireParameters: true,
		}},
	}); err != nil {
		t.Fatalf("PutAIGenerationSettings returned error: %v", err)
	}

	active, err := store.ResolveActiveAIGenerationConfig()
	if err != nil {
		t.Fatalf("ResolveActiveAIGenerationConfig returned error: %v", err)
	}
	if active == nil || active.APIKey != apiKey || active.ModelID != modelID {
		t.Fatalf("unexpected active config: %+v", active)
	}
	override, err := store.ResolveAIGenerationConfigOverride(nil, &AIGenerationTransientConfig{})
	if err != nil || override == nil || override.ProfileID != "default" {
		t.Fatalf("unexpected override config: %+v err=%v", override, err)
	}
	resolvedGoogleBooksAPIKey, err := store.ResolveGoogleBooksAPIKey()
	if err != nil || resolvedGoogleBooksAPIKey != googleBooksAPIKey {
		t.Fatalf("unexpected Google Books API key: %q err=%v", resolvedGoogleBooksAPIKey, err)
	}
	if !store.GoogleBooksAPIKeyConfigured() {
		t.Fatal("GoogleBooksAPIKeyConfigured should report YAML key")
	}
	if !errors.Is(ErrAIGenerationProfileNotFound, ErrAIGenerationProfileNotFound) {
		t.Fatal("ErrAIGenerationProfileNotFound alias should be comparable")
	}
	if IsAIGenerationSettingsCryptoError(errors.New("other")) {
		t.Fatal("IsAIGenerationSettingsCryptoError should reject unrelated errors")
	}
}
