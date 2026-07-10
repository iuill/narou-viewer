package aisettings

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAIGenerationSettingsReadAndPersist(t *testing.T) {
	t.Setenv("AI_GENERATION_SETTINGS_MASTER_PASSPHRASE", "test-passphrase")
	dataDir := t.TempDir()
	store := NewRepository(filepath.Join(dataDir, "state"))
	if err := store.Ensure(); err != nil {
		t.Fatalf("Initialize returned error: %v", err)
	}

	customKey := "sk-custom-secret-value"
	sharedKey := "sk-shared-secret-value"
	googleBooksKey := "google-books-secret-value"
	modelID := "anthropic/claude-sonnet-4"
	nameDiscoveryModelID := "openai/gpt-5-nano"
	selectedProfileID := "custom"
	settings, err := store.PutAIGenerationSettings(AIGenerationSettingsUpdate{
		PreferredMode:     strPtr("llm"),
		SelectedProfileID: &selectedProfileID,
		ExtractionStrategyModels: &AIExtractionStrategyModelsInput{
			NameDiscoveryModelID: &nameDiscoveryModelID,
		},
		SharedProviders: &AISharedProvidersInput{
			OpenRouter:  AIProviderCredentialInput{APIKey: &sharedKey, APIKeySet: true},
			GoogleBooks: AIProviderCredentialInput{APIKey: &googleBooksKey, APIKeySet: true},
		},
		ProfilesSet: true,
		Profiles: []AIProfileInput{
			{
				ID:                "default",
				Label:             "Default",
				Provider:          "openrouter",
				Credentials:       AIProfileCredentialsInput{Source: "shared"},
				ModelID:           nil,
				ProviderOrder:     []string{"OpenAI", ""},
				AllowFallbacks:    false,
				RequireParameters: true,
			},
			{
				ID:                selectedProfileID,
				Label:             "Custom",
				Provider:          "openrouter",
				Credentials:       AIProfileCredentialsInput{Source: "custom", APIKey: &customKey, APIKeySet: true},
				ModelID:           &modelID,
				ProviderOrder:     []string{"Anthropic"},
				AllowFallbacks:    true,
				RequireParameters: false,
			},
			{
				Label:             "Generated",
				Provider:          "openrouter",
				Credentials:       AIProfileCredentialsInput{Source: "shared"},
				RequireParameters: true,
			},
		},
	})
	if err != nil {
		t.Fatalf("PutAIGenerationSettings returned error: %v", err)
	}
	if settings.PreferredMode != "llm" || settings.Settings.SelectedProfileID == nil || *settings.Settings.SelectedProfileID != selectedProfileID {
		t.Fatalf("unexpected AI settings: %+v", settings)
	}
	if settings.Settings.ExtractionStrategyModels.NameDiscoveryModelID == nil || *settings.Settings.ExtractionStrategyModels.NameDiscoveryModelID != nameDiscoveryModelID {
		t.Fatalf("unexpected character summary strategy models: %+v", settings.Settings.ExtractionStrategyModels)
	}
	if !settings.Settings.SharedProviders.OpenRouter.HasAPIKey || settings.Settings.SharedProviders.OpenRouter.APIKeyMasked == nil {
		t.Fatalf("shared provider key metadata was not exposed safely: %+v", settings.Settings.SharedProviders.OpenRouter)
	}
	if !settings.Settings.SharedProviders.GoogleBooks.HasAPIKey || settings.Settings.SharedProviders.GoogleBooks.APIKeyMasked == nil {
		t.Fatalf("Google Books key metadata was not exposed safely: %+v", settings.Settings.SharedProviders.GoogleBooks)
	}
	if len(settings.Settings.Profiles) != 3 || !settings.Settings.Profiles[1].Credentials.HasAPIKey || settings.Settings.Profiles[1].Credentials.APIKeyMasked == nil {
		t.Fatalf("custom profile key metadata was not exposed safely: %+v", settings.Settings.Profiles)
	}
	if settings.Settings.Profiles[2].ID != "profile-3" {
		t.Fatalf("blank profile id should be generated from index: %+v", settings.Settings.Profiles[2])
	}

	raw, err := os.ReadFile(filepath.Join(dataDir, "state", FileName))
	if err != nil {
		t.Fatalf("read AI settings yaml: %v", err)
	}
	rawText := string(raw)
	if strings.Contains(rawText, sharedKey) || strings.Contains(rawText, customKey) || strings.Contains(rawText, googleBooksKey) || strings.Contains(rawText, "api_key: dummy-openrouter-") {
		t.Fatalf("AI settings yaml exposed plaintext credentials: %s", raw)
	}
	if !strings.Contains(rawText, "api_key_encrypted:") || !strings.Contains(rawText, "api_key_version: 1") {
		t.Fatalf("AI settings yaml did not persist encrypted credentials: %s", raw)
	}
	if !strings.Contains(rawText, "extraction_strategy_models:") || strings.Contains(rawText, "character_summary_strategy_models:") {
		t.Fatalf("AI settings should write only the extraction strategy key: %s", rawText)
	}
	if !strings.Contains(rawText, "name_discovery_model_id: openai/gpt-5-nano") {
		t.Fatalf("AI settings yaml did not persist character summary strategy model: %s", raw)
	}
	info, err := os.Stat(filepath.Join(dataDir, "state", FileName))
	if err != nil {
		t.Fatalf("stat AI settings yaml: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("AI settings yaml should be owner-only, mode=%#o", info.Mode().Perm())
	}
	reloaded, err := NewRepository(filepath.Join(dataDir, "state")).GetAIGenerationSettings()
	if err != nil {
		t.Fatalf("reloaded GetAIGenerationSettings returned error: %v", err)
	}
	if reloaded.Settings.Profiles[1].Credentials.APIKeyMasked == nil || strings.Contains(*reloaded.Settings.Profiles[1].Credentials.APIKeyMasked, "secret") {
		t.Fatalf("reloaded response should mask custom key: %+v", reloaded.Settings.Profiles[1].Credentials)
	}
	doc, err := store.readAIGenerationSettingsDocument()
	if err != nil {
		t.Fatalf("read encrypted document: %v", err)
	}
	if decrypted, err := decryptAIAPIKey(doc.SharedProviders.OpenRouter); err != nil || decrypted != sharedKey {
		t.Fatalf("encrypted shared key should round-trip, decrypted=%q err=%v", decrypted, err)
	}
	if decrypted, err := decryptAIAPIKey(doc.SharedProviders.GoogleBooks); err != nil || decrypted != googleBooksKey {
		t.Fatalf("encrypted Google Books key should round-trip, decrypted=%q err=%v", decrypted, err)
	}
	if resolvedGoogleBooksKey, err := store.ResolveGoogleBooksAPIKey(); err != nil || resolvedGoogleBooksKey != googleBooksKey {
		t.Fatalf("Google Books key should resolve from settings, resolved=%q err=%v", resolvedGoogleBooksKey, err)
	}
	if decrypted, err := decryptAIAPIKey(doc.Profiles[1].Credentials.aiAPIKeyDocument); err != nil || decrypted != customKey {
		t.Fatalf("encrypted custom key should round-trip, decrypted=%q err=%v", decrypted, err)
	}
	resolvedConfig, err := store.ResolveActiveAIGenerationConfig()
	if err != nil {
		t.Fatalf("ResolveActiveAIGenerationConfig returned error: %v", err)
	}
	if resolvedConfig == nil ||
		resolvedConfig.ProfileID != selectedProfileID ||
		resolvedConfig.APIKey != customKey ||
		resolvedConfig.ModelID != modelID ||
		len(resolvedConfig.ProviderOrder) != 1 ||
		resolvedConfig.ProviderOrder[0] != "Anthropic" ||
		!resolvedConfig.AllowFallbacks ||
		resolvedConfig.RequireParameters {
		t.Fatalf("unexpected resolved active config: %+v", resolvedConfig)
	}

	renamedLabel := "Custom Renamed"
	updatedWithoutKeys, err := store.PutAIGenerationSettings(AIGenerationSettingsUpdate{
		ProfilesSet: true,
		Profiles: []AIProfileInput{
			{
				ID:                "default",
				Label:             "Default",
				Credentials:       AIProfileCredentialsInput{Source: "shared"},
				RequireParameters: true,
			},
			{
				ID:                selectedProfileID,
				Label:             renamedLabel,
				Credentials:       AIProfileCredentialsInput{Source: "custom"},
				ModelID:           &modelID,
				RequireParameters: true,
			},
		},
	})
	if err != nil {
		t.Fatalf("PutAIGenerationSettings without apiKey fields returned error: %v", err)
	}
	if updatedWithoutKeys.Settings.SharedProviders.OpenRouter.APIKeyMasked == nil ||
		updatedWithoutKeys.Settings.SharedProviders.GoogleBooks.APIKeyMasked == nil ||
		updatedWithoutKeys.Settings.Profiles[1].Credentials.APIKeyMasked == nil ||
		updatedWithoutKeys.Settings.Profiles[1].Label != renamedLabel {
		t.Fatalf("metadata updates should preserve existing credentials: %+v", updatedWithoutKeys)
	}
	doc, err = store.readAIGenerationSettingsDocument()
	if err != nil {
		t.Fatalf("read preserved encrypted document: %v", err)
	}
	if decrypted, err := decryptAIAPIKey(doc.SharedProviders.OpenRouter); err != nil || decrypted != sharedKey {
		t.Fatalf("shared key should be preserved, decrypted=%q err=%v", decrypted, err)
	}
	if decrypted, err := decryptAIAPIKey(doc.SharedProviders.GoogleBooks); err != nil || decrypted != googleBooksKey {
		t.Fatalf("Google Books key should be preserved, decrypted=%q err=%v", decrypted, err)
	}
	if decrypted, err := decryptAIAPIKey(doc.Profiles[1].Credentials.aiAPIKeyDocument); err != nil || decrypted != customKey {
		t.Fatalf("custom key should be preserved, decrypted=%q err=%v", decrypted, err)
	}
	switchedToShared, err := store.PutAIGenerationSettings(AIGenerationSettingsUpdate{
		ProfilesSet: true,
		Profiles: []AIProfileInput{
			{
				ID:                selectedProfileID,
				Label:             renamedLabel,
				Credentials:       AIProfileCredentialsInput{Source: "shared"},
				ModelID:           &modelID,
				RequireParameters: true,
			},
		},
	})
	if err != nil {
		t.Fatalf("PutAIGenerationSettings switching custom profile to shared returned error: %v", err)
	}
	if switchedToShared.Settings.Profiles[0].Credentials.Source != "shared" {
		t.Fatalf("shared profile credentials should not retain custom key metadata: %+v", switchedToShared.Settings.Profiles[0].Credentials)
	}
	doc, err = store.readAIGenerationSettingsDocument()
	if err != nil {
		t.Fatalf("read switched credentials document: %v", err)
	}
	if decrypted, err := decryptAIAPIKey(doc.Profiles[0].Credentials.aiAPIKeyDocument); err != nil || decrypted != "" {
		t.Fatalf("custom key should be dropped when switching to shared, decrypted=%q err=%v", decrypted, err)
	}
	switchedBackToCustom, err := store.PutAIGenerationSettings(AIGenerationSettingsUpdate{
		ProfilesSet: true,
		Profiles: []AIProfileInput{
			{
				ID:                selectedProfileID,
				Label:             renamedLabel,
				Credentials:       AIProfileCredentialsInput{Source: "custom"},
				ModelID:           &modelID,
				RequireParameters: true,
			},
		},
	})
	if err != nil {
		t.Fatalf("PutAIGenerationSettings switching shared profile back to custom returned error: %v", err)
	}
	if switchedBackToCustom.Settings.Profiles[0].Credentials.Source != "custom" || switchedBackToCustom.Settings.Profiles[0].Credentials.HasAPIKey {
		t.Fatalf("stale custom key should not resurrect after switching back to custom: %+v", switchedBackToCustom.Settings.Profiles[0].Credentials)
	}

	updated, err := store.PutAIGenerationPreferredMode("heuristic")
	if err != nil {
		t.Fatalf("PutAIGenerationPreferredMode returned error: %v", err)
	}
	if updated.PreferredMode != "heuristic" || updated.EffectiveGenerationMode != "heuristic" {
		t.Fatalf("preferred mode was not updated: %+v", updated)
	}
}

func TestAIGenerationSettingsReadsLegacyCharacterStrategyModels(t *testing.T) {
	stateDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(stateDir, FileName), []byte(`
schema_version: 2
revision: 1
preferred_mode: heuristic
selected_profile_id: default
shared_providers: {}
profiles:
  - id: default
    label: Default
    provider: openrouter
    credentials:
      source: shared
character_summary_strategy_models:
  name_discovery_model_id: openai/legacy-model
`), 0o600); err != nil {
		t.Fatalf("write legacy settings: %v", err)
	}
	settings, err := NewRepository(stateDir).GetAIGenerationSettings()
	if err != nil {
		t.Fatalf("GetAIGenerationSettings returned error: %v", err)
	}
	modelID := settings.Settings.ExtractionStrategyModels.NameDiscoveryModelID
	if modelID == nil || *modelID != "openai/legacy-model" {
		t.Fatalf("legacy strategy model was not read: %+v", settings.Settings.ExtractionStrategyModels)
	}
}

func TestGoogleBooksAPIKeyFallsBackToEnv(t *testing.T) {
	t.Setenv("GOOGLE_BOOKS_API_KEY", "google-books-env-key")
	store := NewRepository(filepath.Join(t.TempDir(), "state"))
	if err := store.Ensure(); err != nil {
		t.Fatalf("Ensure returned error: %v", err)
	}

	resolved, err := store.ResolveGoogleBooksAPIKey()
	if err != nil {
		t.Fatalf("ResolveGoogleBooksAPIKey returned error: %v", err)
	}
	if resolved != "google-books-env-key" {
		t.Fatalf("Google Books key should fall back to env, resolved=%q", resolved)
	}
	if !store.GoogleBooksAPIKeyConfigured() {
		t.Fatal("GoogleBooksAPIKeyConfigured should honor env fallback")
	}
}

func TestAIGenerationSettingsHelperBranches(t *testing.T) {
	blank := " "
	if normalizeStringPtr(&blank) != nil {
		t.Fatal("blank string pointer should normalize to nil")
	}
	value := " value "
	if got := normalizeStringPtr(&value); got == nil || *got != "value" {
		t.Fatalf("unexpected normalized string pointer: %v", got)
	}
	if got := normalizeStringList([]string{" a ", "", "b"}); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("unexpected normalized string list: %+v", got)
	}
	profiles := []aiGenerationProfileRecord{{ID: "default"}}
	if !hasAIProfile(profiles, "default") || hasAIProfile(profiles, "missing") {
		t.Fatalf("unexpected profile lookup result")
	}
	now := "2026-01-01T00:00:00Z"
	if got := updatedAtOrNow(nil, now); got == nil || *got != now {
		t.Fatalf("nil updatedAt should use now: %v", got)
	}
	updatedAt := "2026-01-02T00:00:00Z"
	if got := updatedAtOrNow(&updatedAt, now); got == nil || *got != updatedAt {
		t.Fatalf("explicit updatedAt should be preserved: %v", got)
	}
}

func TestAIGenerationSettingsRejectsPlaintextPersistenceWithoutPassphrase(t *testing.T) {
	dataDir := t.TempDir()
	store := NewRepository(filepath.Join(dataDir, "state"))
	if err := store.Ensure(); err != nil {
		t.Fatalf("Initialize returned error: %v", err)
	}
	secret := "sk-test-secret-value"
	if _, err := store.PutAIGenerationSettings(AIGenerationSettingsUpdate{
		SharedProviders: &AISharedProvidersInput{
			OpenRouter: AIProviderCredentialInput{APIKey: &secret, APIKeySet: true},
		},
	}); err == nil {
		t.Fatal("PutAIGenerationSettings should reject credential storage without master passphrase")
	}
	raw, err := os.ReadFile(filepath.Join(dataDir, "state", FileName))
	if err != nil {
		t.Fatalf("read AI settings yaml: %v", err)
	}
	if strings.Contains(string(raw), secret) {
		t.Fatalf("AI settings yaml exposed plaintext credential: %s", raw)
	}
}

func TestAIGenerationSettingsMigratesPlaintextCredentialsAndFileMode(t *testing.T) {
	t.Setenv("AI_GENERATION_SETTINGS_MASTER_PASSPHRASE", "test-passphrase")
	dataDir := t.TempDir()
	stateDir := filepath.Join(dataDir, "state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}
	settingsPath := filepath.Join(stateDir, FileName)
	if err := os.WriteFile(settingsPath, []byte(`
schema_version: 2
revision: 7
preferred_mode: llm
selected_profile_id: default
shared_providers:
  openrouter:
    api_key: dummy-openrouter-legacy-shared
profiles:
  - id: default
    label: Default
    provider: openrouter
    credentials:
      source: shared
    model_id: openrouter/auto
    provider_order: []
    allow_fallbacks: false
    require_parameters: true
  - id: custom
    label: Custom
    provider: openrouter
    credentials:
      source: custom
      api_key: dummy-openrouter-legacy-custom
    model_id: openrouter/custom
    provider_order: []
    allow_fallbacks: false
    require_parameters: true
`), 0o644); err != nil {
		t.Fatalf("write legacy AI settings: %v", err)
	}
	store := NewRepository(filepath.Join(dataDir, "state"))
	if err := store.Ensure(); err != nil {
		t.Fatalf("Initialize returned error: %v", err)
	}
	if info, err := os.Stat(settingsPath); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("Initialize should chmod existing AI settings file: info=%+v err=%v", info, err)
	}
	if _, err := store.GetAIGenerationSettings(); err != nil {
		t.Fatalf("GetAIGenerationSettings returned error: %v", err)
	}
	raw, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read migrated AI settings: %v", err)
	}
	if strings.Contains(string(raw), "dummy-openrouter-legacy") || strings.Contains(string(raw), "api_key: dummy-openrouter-") {
		t.Fatalf("plaintext credentials should be migrated: %s", raw)
	}
	doc, err := store.readAIGenerationSettingsDocument()
	if err != nil {
		t.Fatalf("read migrated document: %v", err)
	}
	if decrypted, err := decryptAIAPIKey(doc.SharedProviders.OpenRouter); err != nil || decrypted != "dummy-openrouter-legacy-shared" {
		t.Fatalf("shared plaintext key should migrate encrypted: decrypted=%q err=%v", decrypted, err)
	}
	if decrypted, err := decryptAIAPIKey(doc.Profiles[1].Credentials.aiAPIKeyDocument); err != nil || decrypted != "dummy-openrouter-legacy-custom" {
		t.Fatalf("custom plaintext key should migrate encrypted: decrypted=%q err=%v", decrypted, err)
	}
}

func TestAIGenerationSettingsCryptoHelpersAndEnvFlags(t *testing.T) {
	t.Setenv("AI_GENERATION_SETTINGS_MASTER_PASSPHRASE", "test-passphrase")
	if !IsAIGenerationSettingsCryptoError(&AIGenerationSettingsCryptoError{Message: "crypto"}) {
		t.Fatal("IsAIGenerationSettingsCryptoError should detect crypto errors")
	}
	if IsAIGenerationSettingsCryptoError(os.ErrInvalid) {
		t.Fatal("IsAIGenerationSettingsCryptoError should reject unrelated errors")
	}
	if (&AIGenerationSettingsCryptoError{Message: "crypto"}).Error() != "crypto" {
		t.Fatal("AIGenerationSettingsCryptoError should expose its message")
	}
	dataDir := t.TempDir()
	store := NewRepository(filepath.Join(dataDir, "state"))
	if err := store.Ensure(); err != nil {
		t.Fatalf("Initialize returned error: %v", err)
	}
	settings, err := store.PutAIGenerationPreferredMode("llm")
	if err != nil {
		t.Fatalf("PutAIGenerationPreferredMode returned error: %v", err)
	}
	if settings.APIBaseURLConfigured || !settings.MasterPassphraseConfigured {
		t.Fatalf("expected internal readiness flags in settings response: %+v", settings)
	}
	if settings.EffectiveGenerationMode != "disabled" {
		t.Fatalf("llm mode should be disabled until active profile has OpenRouter settings: %+v", settings)
	}

	encrypted, err := encryptAIAPIKey("sk-test-secret-value")
	if err != nil {
		t.Fatalf("encryptAIAPIKey returned error: %v", err)
	}
	if encrypted.APIKey != nil || encrypted.APIKeyVersion != aiAPIKeyCryptoVersion {
		t.Fatalf("unexpected encrypted document: %+v", encrypted)
	}
	if decrypted, err := decryptAIAPIKey(encrypted); err != nil || decrypted != "sk-test-secret-value" {
		t.Fatalf("decryptAIAPIKey did not round-trip: decrypted=%q err=%v", decrypted, err)
	}
	legacyVersionless := encrypted
	legacyVersionless.APIKeyVersion = 0
	if decrypted, err := decryptAIAPIKey(legacyVersionless); err != nil || decrypted != "sk-test-secret-value" {
		t.Fatalf("versionless encrypted key should be treated as v1: decrypted=%q err=%v", decrypted, err)
	}
	if decrypted, err := decryptAIAPIKey(aiAPIKeyDocument{APIKey: strPtr(" sk-raw-key ")}); err != nil || decrypted != "sk-raw-key" {
		t.Fatalf("raw API key should be trimmed and returned: decrypted=%q err=%v", decrypted, err)
	}
	t.Setenv("AI_GENERATION_SERVICE_API_BASE_URL", "")
	t.Setenv("NODE_ENV", "development")
	if !aiGenerationServiceConfigured() {
		t.Fatal("legacy AI generation service helper should default to configured outside test env")
	}
	t.Setenv("NODE_ENV", "test")
	if aiGenerationServiceConfigured() {
		t.Fatal("legacy AI generation service helper should be disabled by default in test env")
	}
	t.Setenv("AI_GENERATION_SERVICE_API_BASE_URL", "disabled")
	t.Setenv("NODE_ENV", "development")
	if aiGenerationServiceConfigured() {
		t.Fatal("AI generation service should respect explicit disabled setting")
	}
	readyDoc := emptyAiGenerationSettingsDocument()
	readyDoc.PreferredMode = "llm"
	modelID := "openrouter/auto"
	readyDoc.Profiles[0].ModelID = &modelID
	readyDoc.SharedProviders.OpenRouter.APIKey = strPtr("sk-test")
	if effectiveAIGenerationMode(readyDoc) != "openrouter" || !internalAIGenerationConfigured(readyDoc) {
		t.Fatal("internal AI generation should be ready when active profile has model and API key")
	}
	readyDoc.SharedProviders.OpenRouter.APIKey = nil
	if effectiveAIGenerationMode(readyDoc) != "disabled" || internalAIGenerationConfigured(readyDoc) {
		t.Fatal("internal AI generation should be disabled without an API key")
	}
	if decrypted, err := decryptAIAPIKey(aiAPIKeyDocument{}); err != nil || decrypted != "" {
		t.Fatalf("empty API key document should be accepted: decrypted=%q err=%v", decrypted, err)
	}

	t.Setenv("AI_GENERATION_SETTINGS_MASTER_PASSPHRASE", "wrong-passphrase")
	if _, err := decryptAIAPIKey(encrypted); err == nil {
		t.Fatal("decryptAIAPIKey should reject a wrong passphrase")
	} else if !IsAIGenerationSettingsCryptoError(err) {
		t.Fatalf("wrong passphrase should be reported as a crypto error: %v", err)
	}
	invalidVersion := encrypted
	invalidVersion.APIKeyVersion = 99
	if _, err := decryptAIAPIKey(invalidVersion); err == nil {
		t.Fatal("decryptAIAPIKey should reject unsupported versions")
	} else if !IsAIGenerationSettingsCryptoError(err) {
		t.Fatalf("unsupported versions should be reported as crypto errors: %v", err)
	}
	invalidBase64 := encrypted
	invalidBase64.APIKeyVersion = aiAPIKeyCryptoVersion
	invalidBase64.APIKeyEncrypted = "not base64"
	if _, err := decryptAIAPIKey(invalidBase64); err == nil {
		t.Fatal("decryptAIAPIKey should reject malformed ciphertext")
	} else if !IsAIGenerationSettingsCryptoError(err) {
		t.Fatalf("malformed ciphertext should be reported as a crypto error: %v", err)
	}
	invalidIV := encrypted
	invalidIV.APIKeyIV = "AA=="
	if _, err := decryptAIAPIKey(invalidIV); err == nil {
		t.Fatal("decryptAIAPIKey should reject malformed IV metadata")
	} else if !IsAIGenerationSettingsCryptoError(err) {
		t.Fatalf("malformed IV metadata should be reported as a crypto error: %v", err)
	}
	if mask := maskAIKeyDocument(aiAPIKeyDocument{APIKey: strPtr("sk-short")}); mask == nil || *mask != "********" {
		t.Fatalf("short raw key should use full mask: %v", mask)
	}
	if mask := maskAIKeyDocument(aiAPIKeyDocument{APIKey: strPtr("sk-very-long-secret")}); mask == nil || *mask != "sk-v...cret" {
		t.Fatalf("long raw key should be partially masked: %v", mask)
	}
}

func TestAIGenerationSettingsNormalization(t *testing.T) {
	blank := " "
	doc := normalizeAIGenerationSettingsDocument(aiGenerationSettingsDocument{
		Revision:          -1,
		PreferredMode:     "bad",
		SelectedProfileID: &blank,
		Profiles: []aiGenerationProfileRecord{
			{ID: "", Label: "bad"},
			{ID: "profile", Label: "Profile", Provider: "bad", Credentials: aiProfileCredentialsDocument{Source: "bad"}, ProviderOrder: []string{" A ", ""}},
		},
	})
	if doc.PreferredMode != "heuristic" || doc.Revision != 0 || doc.SelectedProfileID == nil || *doc.SelectedProfileID != "profile" {
		t.Fatalf("unexpected normalized AI settings: %+v", doc)
	}
	if len(doc.Profiles) != 1 || doc.Profiles[0].Provider != "openrouter" || doc.Profiles[0].Credentials.Source != "shared" || len(doc.Profiles[0].ProviderOrder) != 1 {
		t.Fatalf("unexpected normalized AI profile: %+v", doc.Profiles)
	}
	encrypted := aiAPIKeyDocument{APIKeyEncrypted: " encrypted "}
	if !hasStoredAIKey(encrypted) || maskAIKeyDocument(encrypted) == nil {
		t.Fatal("encrypted API key metadata should count as stored key")
	}
	if maskAIKeyDocument(aiAPIKeyDocument{}) != nil {
		t.Fatal("missing API key should not have a mask")
	}
}

func TestResolveActiveAIGenerationConfigBranches(t *testing.T) {
	t.Setenv("AI_GENERATION_SETTINGS_MASTER_PASSPHRASE", "test-passphrase")
	dataDir := t.TempDir()
	stateStore := NewRepository(filepath.Join(dataDir, "state"))
	if err := stateStore.Ensure(); err != nil {
		t.Fatalf("Initialize returned error: %v", err)
	}

	if config, err := stateStore.ResolveActiveAIGenerationConfig(); err != nil || config != nil {
		t.Fatalf("heuristic default should not resolve an OpenRouter config: config=%+v err=%v", config, err)
	}

	modelID := "openrouter/auto"
	sharedKey := "sk-shared-secret-value"
	sharedProfileID := "shared-profile"
	if _, err := stateStore.PutAIGenerationSettings(AIGenerationSettingsUpdate{
		PreferredMode:     strPtr("llm"),
		SelectedProfileID: &sharedProfileID,
		SharedProviders: &AISharedProvidersInput{
			OpenRouter: AIProviderCredentialInput{APIKey: &sharedKey, APIKeySet: true},
		},
		ProfilesSet: true,
		Profiles: []AIProfileInput{{
			ID:                sharedProfileID,
			Label:             "Shared",
			Provider:          "openrouter",
			Credentials:       AIProfileCredentialsInput{Source: "shared"},
			ModelID:           &modelID,
			ProviderOrder:     []string{"OpenAI", "Anthropic"},
			AllowFallbacks:    false,
			RequireParameters: true,
		}},
	}); err != nil {
		t.Fatalf("PutAIGenerationSettings shared profile returned error: %v", err)
	}
	sharedConfig, err := stateStore.ResolveActiveAIGenerationConfig()
	if err != nil {
		t.Fatalf("ResolveActiveAIGenerationConfig shared profile returned error: %v", err)
	}
	if sharedConfig == nil ||
		sharedConfig.ProfileID != sharedProfileID ||
		sharedConfig.APIKey != sharedKey ||
		sharedConfig.ModelID != modelID ||
		len(sharedConfig.ProviderOrder) != 2 ||
		sharedConfig.AllowFallbacks ||
		!sharedConfig.RequireParameters {
		t.Fatalf("unexpected shared profile config: %+v", sharedConfig)
	}

	customProfileID := "custom-without-key"
	if _, err := stateStore.PutAIGenerationSettings(AIGenerationSettingsUpdate{
		SelectedProfileID: &customProfileID,
		ProfilesSet:       true,
		Profiles: []AIProfileInput{{
			ID:          customProfileID,
			Label:       "Custom without key",
			Provider:    "openrouter",
			Credentials: AIProfileCredentialsInput{Source: "custom"},
			ModelID:     &modelID,
		}},
	}); err != nil {
		t.Fatalf("PutAIGenerationSettings custom without key returned error: %v", err)
	}
	if config, err := stateStore.ResolveActiveAIGenerationConfig(); err != nil || config != nil {
		t.Fatalf("custom source without a stored key should not resolve config: config=%+v err=%v", config, err)
	}
}

func TestResolveAIGenerationConfigOverrideBranches(t *testing.T) {
	t.Setenv("AI_GENERATION_SETTINGS_MASTER_PASSPHRASE", "test-passphrase")
	dataDir := t.TempDir()
	stateStore := NewRepository(filepath.Join(dataDir, "state"))
	if err := stateStore.Ensure(); err != nil {
		t.Fatalf("Initialize returned error: %v", err)
	}

	modelID := "openrouter/base"
	sharedKey := "sk-shared-secret-value"
	nameDiscoveryModelID := "openai/gpt-5-nano"
	profileID := "shared-profile"
	if _, err := stateStore.PutAIGenerationSettings(AIGenerationSettingsUpdate{
		PreferredMode:     strPtr("heuristic"),
		SelectedProfileID: &profileID,
		ExtractionStrategyModels: &AIExtractionStrategyModelsInput{
			NameDiscoveryModelID: &nameDiscoveryModelID,
		},
		SharedProviders: &AISharedProvidersInput{
			OpenRouter: AIProviderCredentialInput{APIKey: &sharedKey, APIKeySet: true},
		},
		ProfilesSet: true,
		Profiles: []AIProfileInput{{
			ID:                profileID,
			Label:             "Shared",
			Provider:          "openrouter",
			Credentials:       AIProfileCredentialsInput{Source: "shared"},
			ModelID:           &modelID,
			ProviderOrder:     []string{"BaseProvider"},
			AllowFallbacks:    false,
			RequireParameters: true,
		}},
	}); err != nil {
		t.Fatalf("PutAIGenerationSettings returned error: %v", err)
	}

	transientModel := "openrouter/transient"
	allowFallbacks := true
	requireParameters := false
	systemPrompt := "override prompt"
	config, err := stateStore.ResolveAIGenerationConfigOverride(&profileID, &AIGenerationTransientConfig{
		ModelID:              &transientModel,
		ProviderOrder:        []string{"ProviderA", "ProviderB"},
		ProviderOrderSet:     true,
		AllowFallbacks:       &allowFallbacks,
		RequireParameters:    &requireParameters,
		SystemPromptOverride: &systemPrompt,
	})
	if err != nil {
		t.Fatalf("ResolveAIGenerationConfigOverride returned error: %v", err)
	}
	if config == nil ||
		config.ProfileID != profileID ||
		config.APIKey != sharedKey ||
		config.ModelID != transientModel ||
		len(config.ProviderOrder) != 2 ||
		!config.AllowFallbacks ||
		config.RequireParameters ||
		config.SystemPrompt == nil ||
		*config.SystemPrompt != systemPrompt ||
		config.ExtractionNameDiscoveryModelID != nameDiscoveryModelID {
		t.Fatalf("unexpected override config: %+v", config)
	}

	missingProfile := "missing"
	fallbackConfig, err := stateStore.ResolveAIGenerationConfigOverride(&missingProfile, nil)
	if !errors.Is(err, ErrAIGenerationProfileNotFound) {
		t.Fatalf("missing override profile should return not found error: config=%+v err=%v", fallbackConfig, err)
	}
	if fallbackConfig != nil {
		t.Fatalf("missing override profile should not resolve a config: %+v", fallbackConfig)
	}

	blankModel := " "
	if config, err := stateStore.ResolveAIGenerationConfigOverride(&profileID, &AIGenerationTransientConfig{ModelID: &blankModel}); err != nil || config != nil {
		t.Fatalf("blank transient model should disable resolved config: config=%+v err=%v", config, err)
	}
}

func strPtr(value string) *string {
	return &value
}
