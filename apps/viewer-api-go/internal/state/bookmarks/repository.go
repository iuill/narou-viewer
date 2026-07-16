package bookmarks

import (
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"narou-viewer/apps/viewer-api-go/internal/state/schemaguard"
	"narou-viewer/apps/viewer-api-go/internal/state/yamlfile"
)

const (
	SchemaVersion = 3
	FileName      = "bookmarks.yaml"
)

var ErrNotFound = errors.New("bookmark not found")
var ErrInvalidBookmark = errors.New("invalid bookmark")

var SchemaContract = schemaguard.Contract{
	ID:            "VA-BOOKMARKS",
	Path:          FileName,
	Current:       SchemaVersion,
	MissingPolicy: schemaguard.MissingReject,
}

type Bookmark struct {
	ID           string  `json:"id"`
	NovelID      string  `json:"novelId"`
	EpisodeIndex string  `json:"episodeIndex"`
	Position     int     `json:"position"`
	Label        *string `json:"label"`
	CreatedAt    string  `json:"createdAt"`
}

type Repository struct {
	path string
	mu   sync.Mutex
}

type document struct {
	SchemaVersion int      `yaml:"schema_version"`
	Revision      int      `yaml:"revision"`
	Bookmarks     []record `yaml:"bookmarks"`
}

type record struct {
	ID           string  `yaml:"id"`
	NovelID      string  `yaml:"novel_id"`
	EpisodeIndex string  `yaml:"episode_index"`
	Position     int     `yaml:"position,omitempty"`
	LineNumber   int     `yaml:"line_number,omitempty"`
	Label        *string `yaml:"label"`
	CreatedAt    string  `yaml:"created_at"`
}

func NewRepository(stateDir string) *Repository {
	return &Repository{path: filepath.Join(stateDir, FileName)}
}

func (r *Repository) Ensure() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	return yamlfile.Ensure(r.path, emptyDocument())
}

func (r *Repository) List(novelID string) ([]Bookmark, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	doc, err := r.readDocument()
	if err != nil {
		return nil, err
	}
	bookmarks := make([]Bookmark, 0, len(doc.Bookmarks))
	for _, record := range doc.Bookmarks {
		if novelID == "" || record.NovelID == novelID {
			bookmarks = append(bookmarks, toBookmark(record))
		}
	}
	sort.SliceStable(bookmarks, func(i, j int) bool {
		return bookmarks[i].CreatedAt > bookmarks[j].CreatedAt
	})
	return bookmarks, nil
}

func (r *Repository) Create(input Bookmark) (Bookmark, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if strings.TrimSpace(input.NovelID) == "" || !IsEpisodeIndex(input.EpisodeIndex) {
		return Bookmark{}, ErrInvalidBookmark
	}
	doc, err := r.readDocument()
	if err != nil {
		return Bookmark{}, err
	}
	now := isoNow()
	createdRecord := record{
		ID:           newID(),
		NovelID:      input.NovelID,
		EpisodeIndex: input.EpisodeIndex,
		Position:     normalizePosition(input.Position),
		Label:        normalizeLabelPtr(input.Label),
		CreatedAt:    now,
	}
	doc.Revision++
	doc.Bookmarks = append([]record{createdRecord}, doc.Bookmarks...)
	if err := yamlfile.WriteAtomic(r.path, doc); err != nil {
		return Bookmark{}, err
	}
	return toBookmark(createdRecord), nil
}

func (r *Repository) Delete(bookmarkID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	doc, err := r.readDocument()
	if err != nil {
		return err
	}
	next := doc.Bookmarks[:0]
	deleted := false
	for _, bookmark := range doc.Bookmarks {
		if bookmark.ID == bookmarkID {
			deleted = true
			continue
		}
		next = append(next, bookmark)
	}
	if !deleted {
		return ErrNotFound
	}
	doc.Bookmarks = next
	doc.Revision++
	return yamlfile.WriteAtomic(r.path, doc)
}

func (r *Repository) PruneNovel(novelID string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	doc, err := r.readDocument()
	if err != nil {
		return 0, err
	}
	next := doc.Bookmarks[:0]
	deleted := 0
	for _, bookmark := range doc.Bookmarks {
		if bookmark.NovelID == novelID {
			deleted++
			continue
		}
		next = append(next, bookmark)
	}
	if deleted == 0 {
		return 0, nil
	}
	doc.Bookmarks = next
	doc.Revision++
	if err := yamlfile.WriteAtomic(r.path, doc); err != nil {
		return 0, err
	}
	return deleted, nil
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
	return document{SchemaVersion: SchemaVersion, Revision: 0, Bookmarks: []record{}}
}

func normalizeDocument(raw document) document {
	doc := emptyDocument()
	if raw.Revision >= 0 {
		doc.Revision = raw.Revision
	}
	for _, rawRecord := range raw.Bookmarks {
		if rawRecord.ID == "" || rawRecord.NovelID == "" || !IsEpisodeIndex(rawRecord.EpisodeIndex) || rawRecord.CreatedAt == "" {
			continue
		}
		doc.Bookmarks = append(doc.Bookmarks, record{
			ID:           rawRecord.ID,
			NovelID:      rawRecord.NovelID,
			EpisodeIndex: rawRecord.EpisodeIndex,
			Position:     normalizePosition(rawRecord.Position),
			Label:        normalizeLabelPtr(rawRecord.Label),
			CreatedAt:    rawRecord.CreatedAt,
		})
	}
	return doc
}

func toBookmark(record record) Bookmark {
	return Bookmark{
		ID:           record.ID,
		NovelID:      record.NovelID,
		EpisodeIndex: record.EpisodeIndex,
		Position:     record.Position,
		Label:        record.Label,
		CreatedAt:    record.CreatedAt,
	}
}

func IsEpisodeIndex(value string) bool {
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

func normalizePosition(value int) int {
	if value < 0 {
		return 0
	}
	return value
}

func normalizeLabelPtr(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func isoNow() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05.000Z07:00")
}

func newID() string {
	return fmt.Sprintf("bm_%s_%06x", strconv.FormatInt(time.Now().UnixMilli(), 36), rand.Int31n(0xffffff))
}
