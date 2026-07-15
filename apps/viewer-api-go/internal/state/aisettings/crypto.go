package aisettings

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"golang.org/x/crypto/scrypt"
	"io"
	"os"
	"strings"
	"sync"
)

const (
	aiAPIKeyCryptoVersion = 1
	aiAPIKeySaltBytes     = 16
	aiAPIKeyIVBytes       = 12
	aiAPIKeyTagBytes      = 16
	aiAPIKeyKeyBytes      = 32
)

var aiAPIKeyDerivedKeys sync.Map

type AIGenerationSettingsCryptoError struct {
	Message string
	Err     error
}

func (e *AIGenerationSettingsCryptoError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

func (e *AIGenerationSettingsCryptoError) Unwrap() error {
	return e.Err
}

func IsAIGenerationSettingsCryptoError(err error) bool {
	var cryptoError *AIGenerationSettingsCryptoError
	return errors.As(err, &cryptoError)
}

func validateAIGenerationAPIKeyVersions(doc aiGenerationSettingsDocument) error {
	values := []aiAPIKeyDocument{
		doc.SharedProviders.OpenRouter,
		doc.SharedProviders.GoogleBooks,
	}
	for _, profile := range doc.Profiles {
		values = append(values, profile.Credentials.aiAPIKeyDocument)
	}
	for _, value := range values {
		if value.APIKeyVersion != 0 && value.APIKeyVersion != aiAPIKeyCryptoVersion {
			return &AIGenerationSettingsCryptoError{Message: fmt.Sprintf("unsupported AI API key version: %d", value.APIKeyVersion)}
		}
	}
	return nil
}

func migratePlaintextAIGenerationAPIKeys(doc aiGenerationSettingsDocument) (aiGenerationSettingsDocument, bool, error) {
	if aiSettingsMasterPassphrase() == "" {
		return doc, false, nil
	}
	changed := false
	shared, migrated, err := migratePlaintextAIAPIKeyDocument(doc.SharedProviders.OpenRouter)
	if err != nil {
		return doc, false, err
	}
	if migrated {
		doc.SharedProviders.OpenRouter = shared
		changed = true
	}
	googleBooks, migrated, err := migratePlaintextAIAPIKeyDocument(doc.SharedProviders.GoogleBooks)
	if err != nil {
		return doc, false, err
	}
	if migrated {
		doc.SharedProviders.GoogleBooks = googleBooks
		changed = true
	}
	for index := range doc.Profiles {
		credentials := normalizeAIProfileCredentialsDocument(doc.Profiles[index].Credentials)
		if credentials.Source != "custom" {
			continue
		}
		migratedCredentials, migrated, err := migratePlaintextAIAPIKeyDocument(credentials.aiAPIKeyDocument)
		if err != nil {
			return doc, false, err
		}
		if migrated {
			credentials.aiAPIKeyDocument = migratedCredentials
			doc.Profiles[index].Credentials = credentials
			changed = true
		}
	}
	return doc, changed, nil
}

func migratePlaintextAIAPIKeyDocument(value aiAPIKeyDocument) (aiAPIKeyDocument, bool, error) {
	key := normalizeStringPtr(value.APIKey)
	if key == nil {
		return value, false, nil
	}
	encrypted, err := encryptAIAPIKey(*key)
	if err != nil {
		return aiAPIKeyDocument{}, false, err
	}
	encrypted.UpdatedAt = value.UpdatedAt
	return encrypted, true, nil
}

func encryptAIAPIKey(apiKey string) (aiAPIKeyDocument, error) {
	passphrase := aiSettingsMasterPassphrase()
	if passphrase == "" {
		return aiAPIKeyDocument{}, &AIGenerationSettingsCryptoError{Message: "AI_GENERATION_SETTINGS_MASTER_PASSPHRASE is required to store API credentials"}
	}
	salt := make([]byte, aiAPIKeySaltBytes)
	iv := make([]byte, aiAPIKeyIVBytes)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return aiAPIKeyDocument{}, err
	}
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return aiAPIKeyDocument{}, err
	}
	key, err := scrypt.Key([]byte(passphrase), salt, 16384, 8, 1, aiAPIKeyKeyBytes)
	if err != nil {
		return aiAPIKeyDocument{}, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return aiAPIKeyDocument{}, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return aiAPIKeyDocument{}, err
	}
	sealed := gcm.Seal(nil, iv, []byte(apiKey), nil)
	tagSize := gcm.Overhead()
	ciphertext := sealed[:len(sealed)-tagSize]
	tag := sealed[len(sealed)-tagSize:]
	return aiAPIKeyDocument{
		APIKey:          nil,
		APIKeyEncrypted: base64.StdEncoding.EncodeToString(ciphertext),
		APIKeySalt:      base64.StdEncoding.EncodeToString(salt),
		APIKeyIV:        base64.StdEncoding.EncodeToString(iv),
		APIKeyTag:       base64.StdEncoding.EncodeToString(tag),
		APIKeyVersion:   aiAPIKeyCryptoVersion,
	}, nil
}

func decryptAIAPIKey(value aiAPIKeyDocument) (string, error) {
	if key := normalizeStringPtr(value.APIKey); key != nil {
		return *key, nil
	}
	if strings.TrimSpace(value.APIKeyEncrypted) == "" &&
		strings.TrimSpace(value.APIKeySalt) == "" &&
		strings.TrimSpace(value.APIKeyIV) == "" &&
		strings.TrimSpace(value.APIKeyTag) == "" {
		return "", nil
	}
	version := value.APIKeyVersion
	if version == 0 {
		version = aiAPIKeyCryptoVersion
	}
	if version != aiAPIKeyCryptoVersion {
		return "", &AIGenerationSettingsCryptoError{Message: fmt.Sprintf("unsupported AI API key version: %d", value.APIKeyVersion)}
	}
	passphrase := aiSettingsMasterPassphrase()
	if passphrase == "" {
		return "", &AIGenerationSettingsCryptoError{Message: "AI_GENERATION_SETTINGS_MASTER_PASSPHRASE is required to read API credentials"}
	}
	ciphertext, err := base64.StdEncoding.DecodeString(value.APIKeyEncrypted)
	if err != nil {
		return "", &AIGenerationSettingsCryptoError{Message: "failed to decode encrypted AI API key ciphertext", Err: err}
	}
	salt, err := base64.StdEncoding.DecodeString(value.APIKeySalt)
	if err != nil {
		return "", &AIGenerationSettingsCryptoError{Message: "failed to decode encrypted AI API key salt", Err: err}
	}
	iv, err := base64.StdEncoding.DecodeString(value.APIKeyIV)
	if err != nil {
		return "", &AIGenerationSettingsCryptoError{Message: "failed to decode encrypted AI API key IV", Err: err}
	}
	tag, err := base64.StdEncoding.DecodeString(value.APIKeyTag)
	if err != nil {
		return "", &AIGenerationSettingsCryptoError{Message: "failed to decode encrypted AI API key tag", Err: err}
	}
	if len(iv) != aiAPIKeyIVBytes {
		return "", &AIGenerationSettingsCryptoError{Message: fmt.Sprintf("invalid encrypted AI API key IV length: %d", len(iv))}
	}
	if len(tag) != aiAPIKeyTagBytes {
		return "", &AIGenerationSettingsCryptoError{Message: fmt.Sprintf("invalid encrypted AI API key tag length: %d", len(tag))}
	}
	key, err := deriveAIAPIKeyEncryptionKey(passphrase, salt)
	if err != nil {
		return "", &AIGenerationSettingsCryptoError{Message: "failed to derive encrypted AI API key", Err: err}
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", &AIGenerationSettingsCryptoError{Message: "failed to initialize AI API key cipher", Err: err}
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", &AIGenerationSettingsCryptoError{Message: "failed to initialize AI API key cipher mode", Err: err}
	}
	sealed := append(append([]byte{}, ciphertext...), tag...)
	plain, err := gcm.Open(nil, iv, sealed, nil)
	if err != nil {
		return "", &AIGenerationSettingsCryptoError{Message: "failed to decrypt AI API key", Err: err}
	}
	return string(plain), nil
}

func deriveAIAPIKeyEncryptionKey(passphrase string, salt []byte) ([]byte, error) {
	cacheID := aiAPIKeyDerivedKeyCacheID(passphrase, salt)
	if cached, ok := aiAPIKeyDerivedKeys.Load(cacheID); ok {
		return append([]byte(nil), cached.([]byte)...), nil
	}
	key, err := scrypt.Key([]byte(passphrase), salt, 16384, 8, 1, aiAPIKeyKeyBytes)
	if err != nil {
		return nil, err
	}
	cachedKey, _ := aiAPIKeyDerivedKeys.LoadOrStore(cacheID, append([]byte(nil), key...))
	return append([]byte(nil), cachedKey.([]byte)...), nil
}

func aiAPIKeyDerivedKeyCacheID(passphrase string, salt []byte) string {
	sum := sha256.Sum256(append(append([]byte(passphrase), 0), salt...))
	return base64.StdEncoding.EncodeToString(sum[:])
}

func aiSettingsMasterPassphrase() string {
	return strings.TrimSpace(os.Getenv("AI_GENERATION_SETTINGS_MASTER_PASSPHRASE"))
}
