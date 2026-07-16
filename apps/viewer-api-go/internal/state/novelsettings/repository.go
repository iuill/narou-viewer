package novelsettings

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"narou-viewer/apps/viewer-api-go/internal/state/schemaguard"
	"narou-viewer/apps/viewer-api-go/internal/state/yamlfile"
)

const (
	SchemaVersion = 3
	FileName      = "novel_reader_settings.yaml"
)

var SchemaContract = schemaguard.Contract{
	ID:            "VA-NOVEL-SETTINGS",
	Path:          FileName,
	Current:       SchemaVersion,
	MissingPolicy: schemaguard.MissingReject,
}

type Settings struct {
	NovelID    string     `json:"novelId"`
	Correction Correction `json:"correction"`
	UpdatedAt  *string    `json:"updatedAt"`
}

type Correction struct {
	QuoteNormalization                     bool `json:"quoteNormalization"`
	HyphenDashNormalization                bool `json:"hyphenDashNormalization"`
	ParenthesisNormalization               bool `json:"parenthesisNormalization"`
	HalfwidthAlnumPunctuationNormalization bool `json:"halfwidthAlnumPunctuationNormalization"`
}

type Patch struct {
	QuoteNormalization                     *bool
	HyphenDashNormalization                *bool
	ParenthesisNormalization               *bool
	HalfwidthAlnumPunctuationNormalization *bool
}

func (patch Patch) IsEmpty() bool {
	return patch.QuoteNormalization == nil &&
		patch.HyphenDashNormalization == nil &&
		patch.ParenthesisNormalization == nil &&
		patch.HalfwidthAlnumPunctuationNormalization == nil
}

type Repository struct {
	path string
	mu   sync.Mutex
}

type document struct {
	SchemaVersion int               `yaml:"schema_version"`
	Revision      int               `yaml:"revision"`
	Novels        map[string]record `yaml:"novels"`
}

type record struct {
	Correction correctionRecord `yaml:"correction"`
	UpdatedAt  *string          `yaml:"updated_at"`
}

type correctionRecord struct {
	QuoteNormalization                     *bool `yaml:"quote_normalization"`
	HyphenDashNormalization                *bool `yaml:"hyphen_dash_normalization"`
	ParenthesisNormalization               *bool `yaml:"parenthesis_normalization"`
	HalfwidthAlnumPunctuationNormalization *bool `yaml:"halfwidth_alnum_punctuation_normalization"`
}

func NewRepository(stateDir string) *Repository {
	return &Repository{path: filepath.Join(stateDir, FileName)}
}

func (r *Repository) Ensure() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	return yamlfile.Ensure(r.path, emptyDocument())
}

func (r *Repository) Get(novelID string) (Settings, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	doc, err := r.readDocument()
	if err != nil {
		return Settings{}, err
	}
	record, ok := doc.Novels[novelID]
	if !ok {
		return DefaultSettings(novelID), nil
	}
	return toSettings(novelID, record), nil
}

func (r *Repository) Put(input Settings) (Settings, error) {
	return r.Patch(input.NovelID, Patch{
		QuoteNormalization:                     boolPtr(input.Correction.QuoteNormalization),
		HyphenDashNormalization:                boolPtr(input.Correction.HyphenDashNormalization),
		ParenthesisNormalization:               boolPtr(input.Correction.ParenthesisNormalization),
		HalfwidthAlnumPunctuationNormalization: boolPtr(input.Correction.HalfwidthAlnumPunctuationNormalization),
	})
}

func (r *Repository) Patch(novelID string, patch Patch) (Settings, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	doc, err := r.readDocument()
	if err != nil {
		return Settings{}, err
	}
	current := DefaultSettings(novelID).Correction
	var currentUpdatedAt *string
	if record, ok := doc.Novels[novelID]; ok {
		currentSettings := toSettings(novelID, record)
		current = currentSettings.Correction
		currentUpdatedAt = currentSettings.UpdatedAt
	}
	if patch.IsEmpty() {
		return Settings{
			NovelID:    novelID,
			Correction: current,
			UpdatedAt:  currentUpdatedAt,
		}, nil
	}
	if patch.QuoteNormalization != nil {
		current.QuoteNormalization = *patch.QuoteNormalization
	}
	if patch.HyphenDashNormalization != nil {
		current.HyphenDashNormalization = *patch.HyphenDashNormalization
	}
	if patch.ParenthesisNormalization != nil {
		current.ParenthesisNormalization = *patch.ParenthesisNormalization
	}
	if patch.HalfwidthAlnumPunctuationNormalization != nil {
		current.HalfwidthAlnumPunctuationNormalization = *patch.HalfwidthAlnumPunctuationNormalization
	}
	now := isoNow()
	doc.Revision++
	doc.Novels[novelID] = record{
		Correction: correctionRecord{
			QuoteNormalization:                     boolPtr(current.QuoteNormalization),
			HyphenDashNormalization:                boolPtr(current.HyphenDashNormalization),
			ParenthesisNormalization:               boolPtr(current.ParenthesisNormalization),
			HalfwidthAlnumPunctuationNormalization: boolPtr(current.HalfwidthAlnumPunctuationNormalization),
		},
		UpdatedAt: &now,
	}
	if err := yamlfile.WriteAtomic(r.path, doc); err != nil {
		return Settings{}, err
	}
	return toSettings(novelID, doc.Novels[novelID]), nil
}

func (r *Repository) PruneNovel(novelID string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	doc, err := r.readDocument()
	if err != nil {
		return false, err
	}
	if _, ok := doc.Novels[novelID]; !ok {
		return false, nil
	}
	delete(doc.Novels, novelID)
	doc.Revision++
	if err := yamlfile.WriteAtomic(r.path, doc); err != nil {
		return false, err
	}
	return true, nil
}

func (r *Repository) readDocument() (document, error) {
	var raw document
	if _, err := yamlfile.ReadGuarded(r.path, SchemaContract, &raw); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return emptyDocument(), nil
		}
		return document{}, err
	}
	return normalizeDocument(raw), nil
}

func emptyDocument() document {
	return document{SchemaVersion: SchemaVersion, Revision: 0, Novels: map[string]record{}}
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
			Correction: correctionRecord{
				QuoteNormalization:                     rawRecord.Correction.QuoteNormalization,
				HyphenDashNormalization:                rawRecord.Correction.HyphenDashNormalization,
				ParenthesisNormalization:               rawRecord.Correction.ParenthesisNormalization,
				HalfwidthAlnumPunctuationNormalization: rawRecord.Correction.HalfwidthAlnumPunctuationNormalization,
			},
			UpdatedAt: stringPtrOrNil(rawRecord.UpdatedAt),
		}
	}
	return doc
}

func DefaultSettings(novelID string) Settings {
	return Settings{
		NovelID: novelID,
		Correction: Correction{
			QuoteNormalization:                     true,
			HyphenDashNormalization:                true,
			ParenthesisNormalization:               true,
			HalfwidthAlnumPunctuationNormalization: true,
		},
	}
}

func toSettings(novelID string, record record) Settings {
	return Settings{
		NovelID: novelID,
		Correction: Correction{
			QuoteNormalization:                     boolValueOrDefault(record.Correction.QuoteNormalization, true),
			HyphenDashNormalization:                boolValueOrDefault(record.Correction.HyphenDashNormalization, true),
			ParenthesisNormalization:               boolValueOrDefault(record.Correction.ParenthesisNormalization, true),
			HalfwidthAlnumPunctuationNormalization: boolValueOrDefault(record.Correction.HalfwidthAlnumPunctuationNormalization, true),
		},
		UpdatedAt: record.UpdatedAt,
	}
}

func boolPtr(value bool) *bool {
	return &value
}

func boolValueOrDefault(value *bool, defaultValue bool) bool {
	if value == nil {
		return defaultValue
	}
	return *value
}

func stringPtrOrNil(value *string) *string {
	if value == nil {
		return nil
	}
	return value
}

func isoNow() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05.000Z07:00")
}
