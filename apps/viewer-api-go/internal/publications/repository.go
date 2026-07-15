package publications

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"narou-viewer/apps/viewer-api-go/internal/state/schemaguard"
	"narou-viewer/apps/viewer-api-go/internal/state/yamlfile"
)

const (
	FileName      = "publications.yaml"
	SchemaVersion = 1
)

var SchemaContract = schemaguard.Contract{
	ID:                   "VA-PUBLICATIONS",
	Path:                 FileName,
	Current:              SchemaVersion,
	ReadableLegacy:       []int{0},
	MissingPolicy:        schemaguard.MissingTreatAsLegacy,
	MissingLegacyVersion: 0,
}

type Repository struct {
	path string
	mu   sync.Mutex
}

type document struct {
	SchemaVersion int                 `yaml:"schema_version"`
	Novels        []NovelPublications `yaml:"novels"`
}

func NewRepository(stateDir string) *Repository {
	return &Repository{path: filepath.Join(stateDir, FileName)}
}

func (r *Repository) Get(novelID string) (NovelPublications, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	doc, err := r.readLocked()
	if err != nil {
		return NovelPublications{}, err
	}
	return normalizeNovelPublications(findNovel(doc, novelID), novelID), nil
}

func (r *Repository) ListByNovelIDs(novelIDs []string) (map[string]NovelPublications, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	doc, err := r.readLocked()
	if err != nil {
		return nil, err
	}
	result := make(map[string]NovelPublications, len(novelIDs))
	for _, novelID := range novelIDs {
		novelID = strings.TrimSpace(novelID)
		if novelID == "" {
			continue
		}
		result[novelID] = normalizeNovelPublications(findNovel(doc, novelID), novelID)
	}
	return result, nil
}

func (r *Repository) PutEntry(novelID string, entry Entry) (NovelPublications, error) {
	return r.putEntryDeleting(novelID, entry, "")
}

func (r *Repository) PutEntryDeleting(novelID string, entry Entry, deleteEntryID string) (NovelPublications, error) {
	return r.putEntryDeleting(novelID, entry, deleteEntryID)
}

func (r *Repository) putEntryDeleting(novelID string, entry Entry, deleteEntryID string) (NovelPublications, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	doc, err := r.readLocked()
	if err != nil {
		return NovelPublications{}, err
	}
	novelID = strings.TrimSpace(novelID)
	index := -1
	for i := range doc.Novels {
		if doc.Novels[i].NovelID == novelID {
			index = i
			break
		}
	}
	if index < 0 {
		doc.Novels = append(doc.Novels, NovelPublications{NovelID: novelID})
		index = len(doc.Novels) - 1
	}

	entry = normalizeEntry(entry)
	if entry.ID == "" {
		entry.ID = string(entry.Kind)
	}
	deleteEntryID = strings.TrimSpace(deleteEntryID)
	entries := normalizeEntries(doc.Novels[index].Entries)
	replaced := false
	nextEntries := entries[:0]
	for _, currentEntry := range entries {
		if deleteEntryID != "" && currentEntry.ID == deleteEntryID && currentEntry.ID != entry.ID {
			continue
		}
		if currentEntry.ID == entry.ID {
			nextEntries = append(nextEntries, entry)
			replaced = true
			continue
		}
		nextEntries = append(nextEntries, currentEntry)
	}
	if !replaced {
		nextEntries = append(nextEntries, entry)
	}
	if deleteEntryID != "" && doc.Novels[index].DisplayCoverEntryID == deleteEntryID {
		doc.Novels[index].DisplayCoverEntryID = entry.ID
	}
	doc.Novels[index].Entries = normalizeEntries(nextEntries)
	doc.Novels[index].DisplayCoverEntryID = normalizeDisplayCoverEntryID(doc.Novels[index].DisplayCoverEntryID, doc.Novels[index].Entries)

	if err := r.writeLocked(doc); err != nil {
		return NovelPublications{}, err
	}
	return normalizeNovelPublications(doc.Novels[index], novelID), nil
}

func (r *Repository) DeleteEntry(novelID string, entryID string) (NovelPublications, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	doc, err := r.readLocked()
	if err != nil {
		return NovelPublications{}, err
	}
	novelID = strings.TrimSpace(novelID)
	entryID = strings.TrimSpace(entryID)
	index := -1
	for i := range doc.Novels {
		if doc.Novels[i].NovelID == novelID {
			index = i
			break
		}
	}
	if index < 0 {
		return normalizeNovelPublications(NovelPublications{}, novelID), nil
	}

	entries := normalizeEntries(doc.Novels[index].Entries)
	nextEntries := entries[:0]
	for _, entry := range entries {
		if entry.ID == entryID {
			continue
		}
		nextEntries = append(nextEntries, entry)
	}
	doc.Novels[index].Entries = normalizeEntries(nextEntries)
	if doc.Novels[index].DisplayCoverEntryID == entryID {
		doc.Novels[index].DisplayCoverEntryID = ""
	}
	if err := r.writeLocked(doc); err != nil {
		return NovelPublications{}, err
	}
	return normalizeNovelPublications(doc.Novels[index], novelID), nil
}

func (r *Repository) PutDisplayCoverEntryID(novelID string, entryID string) (NovelPublications, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	doc, err := r.readLocked()
	if err != nil {
		return NovelPublications{}, err
	}
	novelID = strings.TrimSpace(novelID)
	entryID = strings.TrimSpace(entryID)
	index := -1
	for i := range doc.Novels {
		if doc.Novels[i].NovelID == novelID {
			index = i
			break
		}
	}
	if index < 0 {
		doc.Novels = append(doc.Novels, NovelPublications{NovelID: novelID})
		index = len(doc.Novels) - 1
	}
	doc.Novels[index].DisplayCoverEntryID = entryID
	doc.Novels[index].Entries = normalizeEntries(doc.Novels[index].Entries)
	doc.Novels[index].DisplayCoverEntryID = normalizeDisplayCoverEntryID(entryID, doc.Novels[index].Entries)
	if err := r.writeLocked(doc); err != nil {
		return NovelPublications{}, err
	}
	return normalizeNovelPublications(doc.Novels[index], novelID), nil
}

func (r *Repository) Ensure() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	return yamlfile.Ensure(r.path, document{SchemaVersion: SchemaVersion, Novels: []NovelPublications{}})
}

func (r *Repository) PruneNovel(novelID string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	novelID = strings.TrimSpace(novelID)
	if novelID == "" {
		return 0, nil
	}
	doc, err := r.readLocked()
	if err != nil {
		return 0, err
	}
	nextNovels := doc.Novels[:0]
	deleted := 0
	for _, novel := range doc.Novels {
		if novel.NovelID == novelID {
			deleted += len(normalizeEntries(novel.Entries))
			continue
		}
		nextNovels = append(nextNovels, novel)
	}
	if deleted == 0 {
		return 0, nil
	}
	doc.Novels = nextNovels
	if err := r.writeLocked(doc); err != nil {
		return 0, err
	}
	return deleted, nil
}

func (r *Repository) readLocked() (document, error) {
	var doc document
	if _, err := yamlfile.ReadGuarded(r.path, SchemaContract, &doc); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return document{SchemaVersion: SchemaVersion, Novels: []NovelPublications{}}, nil
		}
		return document{}, err
	}
	if doc.SchemaVersion == 0 {
		doc.SchemaVersion = SchemaVersion
	}
	if doc.Novels == nil {
		doc.Novels = []NovelPublications{}
	}
	return doc, nil
}

func (r *Repository) writeLocked(doc document) error {
	if doc.SchemaVersion == 0 {
		doc.SchemaVersion = SchemaVersion
	}
	if doc.Novels == nil {
		doc.Novels = []NovelPublications{}
	}
	return yamlfile.WriteAtomic(r.path, doc)
}

func findNovel(doc document, novelID string) NovelPublications {
	for _, novel := range doc.Novels {
		if novel.NovelID == novelID {
			return novel
		}
	}
	return NovelPublications{}
}

func normalizeNovelPublications(novel NovelPublications, novelID string) NovelPublications {
	novel.NovelID = strings.TrimSpace(novelID)
	novel.Entries = normalizeEntries(novel.Entries)
	novel.DisplayCoverEntryID = normalizeDisplayCoverEntryID(novel.DisplayCoverEntryID, novel.Entries)
	return novel
}

func normalizeEntries(entries []Entry) []Entry {
	seen := map[string]bool{}
	grouped := map[Kind][]Entry{
		KindNovel: {},
		KindComic: {},
	}
	for _, entry := range entries {
		entry = normalizeEntry(entry)
		if entry.Kind == "" {
			continue
		}
		if entry.ID == "" {
			entry.ID = string(entry.Kind)
		}
		if seen[entry.ID] {
			entry.ID = uniqueEntryID(entry.ID, seen)
		}
		seen[entry.ID] = true
		grouped[entry.Kind] = append(grouped[entry.Kind], entry)
	}
	result := make([]Entry, 0, len(entries))
	for _, kind := range []Kind{KindNovel, KindComic} {
		result = append(result, grouped[kind]...)
	}
	return result
}

func normalizeEntry(entry Entry) Entry {
	entry.ID = strings.TrimSpace(entry.ID)
	entry.Kind = normalizeKind(entry.Kind)
	if entry.Status == "" {
		entry.Status = EntryStatusUnknown
	}
	if entry.Override == "" {
		entry.Override = OverrideModeNone
	}
	return entry
}

func normalizeDisplayCoverEntryID(entryID string, entries []Entry) string {
	entryID = strings.TrimSpace(entryID)
	if entryID == "" {
		return ""
	}
	for _, entry := range entries {
		if entry.ID == entryID && entry.Status != EntryStatusDisabled && entry.ImageURL != "" {
			return entryID
		}
	}
	return ""
}

func uniqueEntryID(base string, seen map[string]bool) string {
	base = strings.TrimSpace(base)
	if base == "" {
		base = "entry"
	}
	for suffix := 2; ; suffix++ {
		candidate := base + "-" + strconv.Itoa(suffix)
		if !seen[candidate] {
			return candidate
		}
	}
}

func normalizeKind(kind Kind) Kind {
	switch kind {
	case KindNovel, KindComic:
		return kind
	default:
		return ""
	}
}
