package readingstate

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"narou-viewer/apps/viewer-api-go/internal/state/yamlfile"
)

var ErrVersionConflict = errors.New("reading state version conflict")

type Repository struct {
	path string
	mu   sync.Mutex
}

func NewRepository(stateDir string) *Repository {
	return &Repository{path: filepath.Join(stateDir, FileName)}
}

func (r *Repository) Ensure() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	return ensureYAML(r.path, emptyDocument())
}

func (r *Repository) Get(novelID string) (State, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	doc, err := r.readDocument()
	if err != nil {
		return State{}, err
	}
	record, ok := doc.Novels[novelID]
	if !ok {
		return State{NovelID: novelID, Position: 0, StateVersion: 0}, nil
	}
	return toState(novelID, record), nil
}

func (r *Repository) Put(input PutInput) (State, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	doc, err := r.readDocument()
	if err != nil {
		return State{}, err
	}
	now := isoNow()
	current := doc.Novels[input.NovelID]
	if input.ExpectedStateVersion != nil && current.StateVersion != *input.ExpectedStateVersion {
		return toState(input.NovelID, current), ErrVersionConflict
	}
	episodeIndex := normalizeEpisodeIndexPtr(input.LastReadEpisodeIndex)
	position := normalizePosition(input.Position)
	if episodeIndex == nil {
		position = 0
	}
	scroll := normalizeScrollStatePtr(input.Scroll)
	doc.Revision++
	doc.Novels[input.NovelID] = record{
		LastReadEpisodeIndex: episodeIndex,
		Position:             position,
		Scroll:               scroll,
		UpdatedAt:            &now,
		StateVersion:         current.StateVersion + 1,
		UpdatedByClientID:    normalizeClientIDPtr(input.UpdatedByClientID),
	}
	if err := writeYAMLAtomic(r.path, doc); err != nil {
		return State{}, err
	}
	return toState(input.NovelID, doc.Novels[input.NovelID]), nil
}

func (r *Repository) Prune(novelID string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	doc, err := r.readDocument()
	if err != nil {
		return false, err
	}
	result := false
	if current, ok := doc.Novels[novelID]; ok {
		if !current.Deleted {
			result = current.LastReadEpisodeIndex != nil || current.Position != 0 || current.Scroll != nil || current.UpdatedAt != nil
		}
		doc.Novels[novelID] = record{
			StateVersion: current.StateVersion + 1,
			Deleted:      true,
		}
	} else {
		doc.Novels[novelID] = record{
			StateVersion: 1,
			Deleted:      true,
		}
	}
	doc.Revision++
	if err := writeYAMLAtomic(r.path, doc); err != nil {
		return false, err
	}
	return result, nil
}

func (r *Repository) IsDeleted(novelID string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	doc, err := r.readDocument()
	if err != nil {
		return false, err
	}
	return doc.Novels[novelID].Deleted, nil
}

func (r *Repository) readDocument() (document, error) {
	var raw document
	if err := readYAML(r.path, &raw); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return emptyDocument(), nil
		}
		return document{}, err
	}
	return normalizeDocument(raw), nil
}

func normalizeDocument(raw document) document {
	doc := emptyDocument()
	if raw.Revision >= 0 {
		doc.Revision = raw.Revision
	}
	for novelID, rawRecord := range raw.Novels {
		if strings.TrimSpace(novelID) == "" {
			continue
		}
		doc.Novels[novelID] = record{
			LastReadEpisodeIndex: normalizeEpisodeIndexPtr(rawRecord.LastReadEpisodeIndex),
			Position:             normalizePosition(rawRecord.Position),
			Scroll:               normalizeScrollStatePtr(rawRecord.Scroll),
			UpdatedAt:            stringPtrOrNil(rawRecord.UpdatedAt),
			StateVersion:         normalizePosition(rawRecord.StateVersion),
			UpdatedByClientID:    normalizeClientIDPtr(rawRecord.UpdatedByClientID),
			Deleted:              rawRecord.Deleted,
		}
		if doc.Novels[novelID].Deleted {
			deletedRecord := doc.Novels[novelID]
			deletedRecord.LastReadEpisodeIndex = nil
			deletedRecord.Position = 0
			deletedRecord.Scroll = nil
			deletedRecord.UpdatedAt = nil
			deletedRecord.UpdatedByClientID = nil
			if deletedRecord.StateVersion < 1 {
				deletedRecord.StateVersion = 1
			}
			doc.Novels[novelID] = deletedRecord
		}
	}
	return doc
}

func toState(novelID string, record record) State {
	if record.Deleted {
		return State{
			NovelID:      novelID,
			Position:     0,
			StateVersion: record.StateVersion,
		}
	}
	return State{
		NovelID:              novelID,
		LastReadEpisodeIndex: record.LastReadEpisodeIndex,
		Position:             record.Position,
		Scroll:               record.Scroll,
		UpdatedAt:            record.UpdatedAt,
		StateVersion:         record.StateVersion,
		UpdatedByClientID:    record.UpdatedByClientID,
	}
}

func isEpisodeIndex(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func normalizeEpisodeIndexPtr(value *string) *string {
	if value == nil || !isEpisodeIndex(*value) {
		return nil
	}
	return value
}

func normalizePosition(value int) int {
	if value < 0 {
		return 0
	}
	return value
}

func normalizeScrollStatePtr(value *ScrollState) *ScrollState {
	if value == nil || value.Type != "ratio" {
		return nil
	}
	return &ScrollState{Type: "ratio", Value: min(1, max(0, value.Value))}
}

func normalizeClientIDPtr(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func stringPtrOrNil(value *string) *string {
	if value == nil {
		return nil
	}
	return value
}

func isoNow() string {
	return time.Now().UTC().Format(timestampLayout)
}

func ensureYAML(path string, value any) error {
	return yamlfile.Ensure(path, value)
}

func readYAML(path string, target any) error {
	return yamlfile.Read(path, target)
}

func writeYAMLAtomic(path string, value any) error {
	return yamlfile.WriteAtomic(path, value)
}
