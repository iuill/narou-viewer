package preferences

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"narou-viewer/apps/viewer-api-go/internal/state/yamlfile"
)

const (
	SchemaVersion = 3
	FileName      = "reader_preferences.yaml"

	DefaultReadingMode = "vertical"
	DefaultFontFamily  = "mincho"
	DefaultTheme       = "classic"
)

type Preferences struct {
	ReadingMode string  `json:"readingMode"`
	FontFamily  string  `json:"fontFamily"`
	Theme       string  `json:"theme"`
	UpdatedAt   *string `json:"updatedAt"`
}

type Repository struct {
	path string
	mu   sync.Mutex
}

type document struct {
	SchemaVersion int    `yaml:"schema_version"`
	Revision      int    `yaml:"revision"`
	Reader        record `yaml:"reader"`
}

type record struct {
	ReadingMode string  `yaml:"reading_mode"`
	FontFamily  string  `yaml:"font_family"`
	Theme       string  `yaml:"theme"`
	UpdatedAt   *string `yaml:"updated_at"`
}

func NewRepository(stateDir string) *Repository {
	return &Repository{path: filepath.Join(stateDir, FileName)}
}

func (r *Repository) Ensure() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	return yamlfile.Ensure(r.path, emptyDocument())
}

func (r *Repository) Get() (Preferences, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	doc, err := r.readDocument()
	if err != nil {
		return Preferences{}, err
	}
	return toPreferences(doc.Reader), nil
}

func (r *Repository) Put(input Preferences) (Preferences, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	doc, err := r.readDocument()
	if err != nil {
		return Preferences{}, err
	}
	now := isoNow()
	doc.Revision++
	if IsReadingMode(input.ReadingMode) {
		doc.Reader.ReadingMode = input.ReadingMode
	}
	if IsFontFamily(input.FontFamily) {
		doc.Reader.FontFamily = input.FontFamily
	}
	if IsTheme(input.Theme) {
		doc.Reader.Theme = input.Theme
	}
	doc.Reader.UpdatedAt = &now
	if err := yamlfile.WriteAtomic(r.path, doc); err != nil {
		return Preferences{}, err
	}
	return toPreferences(doc.Reader), nil
}

func (r *Repository) readDocument() (document, error) {
	var raw document
	if err := yamlfile.Read(r.path, &raw); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return emptyDocument(), nil
		}
		return document{}, err
	}
	return normalizeDocument(raw), nil
}

func emptyDocument() document {
	return document{
		SchemaVersion: SchemaVersion,
		Revision:      0,
		Reader: record{
			ReadingMode: DefaultReadingMode,
			FontFamily:  DefaultFontFamily,
			Theme:       DefaultTheme,
		},
	}
}

func normalizeDocument(raw document) document {
	doc := emptyDocument()
	if raw.Revision >= 0 {
		doc.Revision = raw.Revision
	}
	if IsReadingMode(raw.Reader.ReadingMode) {
		doc.Reader.ReadingMode = raw.Reader.ReadingMode
	}
	if IsFontFamily(raw.Reader.FontFamily) {
		doc.Reader.FontFamily = raw.Reader.FontFamily
	}
	if IsTheme(raw.Reader.Theme) {
		doc.Reader.Theme = raw.Reader.Theme
	}
	doc.Reader.UpdatedAt = stringPtrOrNil(raw.Reader.UpdatedAt)
	return doc
}

func toPreferences(record record) Preferences {
	return Preferences{
		ReadingMode: record.ReadingMode,
		FontFamily:  record.FontFamily,
		Theme:       record.Theme,
		UpdatedAt:   record.UpdatedAt,
	}
}

func IsReadingMode(value string) bool {
	return value == "vertical" || value == "horizontal"
}

func IsFontFamily(value string) bool {
	return value == "mincho" || value == "gothic"
}

func IsTheme(value string) bool {
	return value == "classic" || value == "paper" || value == "forest" || value == "ocean" || value == "midnight"
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
