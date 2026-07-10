package checkpointstore

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"

	"narou-viewer/apps/viewer-api-go/internal/characters"
	"narou-viewer/apps/viewer-api-go/internal/fsatomic"
)

type Checkpoint struct {
	SchemaVersion             int                                      `json:"schemaVersion"`
	NovelID                   string                                   `json:"novelId"`
	UpToEpisodeIndex          string                                   `json:"upToEpisodeIndex"`
	GenerationFingerprint     string                                   `json:"generationFingerprint,omitempty"`
	ProcessedEpisodeIndexes   []string                                 `json:"processedEpisodeIndexes"`
	ProcessedBatchIndexes     []int                                    `json:"processedBatchIndexes,omitempty"`
	Characters                []characters.GeneratedCharacter          `json:"characters"`
	PendingUnresolvedMentions []characters.GeneratedUnresolvedMention  `json:"pendingUnresolvedMentions,omitempty"`
	IssuedCharacterIDs        []string                                 `json:"issuedCharacterIds,omitempty"`
	RetiredCharacterIDs       []characters.GeneratedRetiredCharacterID `json:"retiredCharacterIds,omitempty"`
	NextCharacterOrdinal      int                                      `json:"nextCharacterOrdinal,omitempty"`
	UpdatedAt                 string                                   `json:"updatedAt"`
}

type FileStore struct {
	stateDir string
}

func NewFileStore(stateDir string) FileStore {
	return FileStore{stateDir: stateDir}
}

func (s FileStore) Path(novelID string, upToEpisodeIndex string) string {
	sum := sha1.Sum([]byte(novelID + "\x00" + upToEpisodeIndex))
	name := "extraction-" + hex.EncodeToString(sum[:]) + ".json"
	return filepath.Join(s.stateDir, "extraction_jobs", "checkpoints", name)
}

func (s FileStore) Load(novelID string, upToEpisodeIndex string) (Checkpoint, error) {
	path := s.Path(novelID, upToEpisodeIndex)
	raw, err := os.ReadFile(path)
	if err != nil {
		return Checkpoint{}, err
	}
	var checkpoint Checkpoint
	if err := json.Unmarshal(raw, &checkpoint); err != nil {
		return Checkpoint{}, err
	}
	return checkpoint, nil
}

func (s FileStore) Save(novelID string, upToEpisodeIndex string, checkpoint Checkpoint) error {
	path := s.Path(novelID, upToEpisodeIndex)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(checkpoint, "", "  ")
	if err != nil {
		return err
	}
	return fsatomic.WriteFile(path, raw, 0o600)
}

func (s FileStore) Delete(novelID string, upToEpisodeIndex string) error {
	return os.Remove(s.Path(novelID, upToEpisodeIndex))
}

func (s FileStore) Exists(novelID string, upToEpisodeIndex string) bool {
	_, err := os.Stat(s.Path(novelID, upToEpisodeIndex))
	return err == nil
}
