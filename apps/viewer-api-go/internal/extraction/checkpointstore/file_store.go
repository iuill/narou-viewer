package checkpointstore

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"narou-viewer/apps/viewer-api-go/internal/characters"
	"narou-viewer/apps/viewer-api-go/internal/fsatomic"
	"narou-viewer/apps/viewer-api-go/internal/state/filequarantine"
	"narou-viewer/apps/viewer-api-go/internal/state/safefile"
	"narou-viewer/apps/viewer-api-go/internal/state/schemaguard"
	"narou-viewer/apps/viewer-api-go/internal/terms"
)

const SchemaVersion = 4

var SchemaContract = schemaguard.Contract{
	ID:            "VA-EXTRACTION-CHECKPOINT",
	Path:          "extraction_jobs/checkpoints/*.json",
	Current:       SchemaVersion,
	MissingPolicy: schemaguard.MissingReject,
}

type IncompatibleError struct {
	Path            string
	QuarantinedPath string
	Reason          string
	Err             error
}

func (e *IncompatibleError) Error() string {
	message := fmt.Sprintf("extraction checkpoint is incompatible and was quarantined: %s", e.Path)
	if e.Reason != "" {
		message += ": " + e.Reason
	}
	return message
}

func (e *IncompatibleError) Unwrap() error {
	return e.Err
}

func IsIncompatible(err error) bool {
	var incompatible *IncompatibleError
	return errors.As(err, &incompatible)
}

type Checkpoint struct {
	SchemaVersion             int                                      `json:"schemaVersion"`
	NovelID                   string                                   `json:"novelId"`
	UpToEpisodeIndex          string                                   `json:"upToEpisodeIndex"`
	GenerationFingerprint     string                                   `json:"generationFingerprint,omitempty"`
	ProcessedEpisodeIndexes   []string                                 `json:"processedEpisodeIndexes"`
	ProcessedBatchIndexes     []int                                    `json:"processedBatchIndexes,omitempty"`
	Characters                []characters.GeneratedCharacter          `json:"characters"`
	Terms                     []terms.GeneratedTerm                    `json:"terms"`
	PendingUnresolvedMentions []characters.GeneratedUnresolvedMention  `json:"pendingUnresolvedMentions,omitempty"`
	IssuedCharacterIDs        []string                                 `json:"issuedCharacterIds,omitempty"`
	RetiredCharacterIDs       []characters.GeneratedRetiredCharacterID `json:"retiredCharacterIds,omitempty"`
	IdentityMergeEvents       []characters.GeneratedIdentityMergeEvent `json:"identityMergeEvents,omitempty"`
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
	raw, err := safefile.ReadRegular(path, safefile.MaxCanonicalStateBytes)
	if err != nil {
		return Checkpoint{}, err
	}
	if _, err := schemaguard.CheckJSON(raw, SchemaContract); err != nil {
		return Checkpoint{}, err
	}
	var checkpoint Checkpoint
	if err := json.Unmarshal(raw, &checkpoint); err != nil {
		_, malformedErr := schemaguard.Malformed(SchemaContract, err)
		return Checkpoint{}, malformedErr
	}
	return checkpoint, nil
}

func (s FileStore) Save(novelID string, upToEpisodeIndex string, checkpoint Checkpoint) error {
	path := s.Path(novelID, upToEpisodeIndex)
	if _, err := os.Stat(path); err == nil {
		if _, err := s.Load(novelID, upToEpisodeIndex); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	checkpoint.SchemaVersion = SchemaVersion
	checkpoint.NovelID = novelID
	checkpoint.UpToEpisodeIndex = upToEpisodeIndex
	raw, err := json.MarshalIndent(checkpoint, "", "  ")
	if err != nil {
		return err
	}
	return fsatomic.WriteFile(path, raw, 0o600)
}

func (s FileStore) Quarantine(novelID string, upToEpisodeIndex string, reason string, cause error) error {
	path := s.Path(novelID, upToEpisodeIndex)
	quarantinedPath, err := filequarantine.Move(path, "unsupported")
	if err != nil {
		return err
	}
	return &IncompatibleError{
		Path:            path,
		QuarantinedPath: quarantinedPath,
		Reason:          reason,
		Err:             cause,
	}
}

func (s FileStore) Delete(novelID string, upToEpisodeIndex string) error {
	return os.Remove(s.Path(novelID, upToEpisodeIndex))
}

func (s FileStore) Exists(novelID string, upToEpisodeIndex string) bool {
	_, err := os.Stat(s.Path(novelID, upToEpisodeIndex))
	return err == nil
}
