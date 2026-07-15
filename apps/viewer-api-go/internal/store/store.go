package store

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"narou-viewer/apps/viewer-api-go/internal/state/aisettings"
	"narou-viewer/apps/viewer-api-go/internal/state/bookmarks"
	"narou-viewer/apps/viewer-api-go/internal/state/novelsettings"
	"narou-viewer/apps/viewer-api-go/internal/state/preferences"
	"narou-viewer/apps/viewer-api-go/internal/state/readingstate"
)

const (
	schemaVersion = readingstate.SchemaVersion

	DefaultReadingMode       = preferences.DefaultReadingMode
	DefaultReaderFontFamily  = preferences.DefaultFontFamily
	DefaultReaderTheme       = preferences.DefaultTheme
	readingStateFile         = readingstate.FileName
	bookmarksFile            = bookmarks.FileName
	readerPreferencesFile    = preferences.FileName
	novelReaderSettingsFile  = novelsettings.FileName
	readerPreferencesUpdated = "2006-01-02T15:04:05.000Z07:00"
)

var ErrBookmarkNotFound = bookmarks.ErrNotFound
var ErrNovelStateDeleted = errors.New("novel state deleted")
var ErrReadingStateVersionConflict = readingstate.ErrVersionConflict

type Store struct {
	stateDir      string
	readingState  *readingstate.Repository
	bookmarks     *bookmarks.Repository
	preferences   *preferences.Repository
	novelSettings *novelsettings.Repository
	aiSettings    *aisettings.Repository
	mu            sync.Mutex
}

type NovelStatePruneResult struct {
	ReadingStateDeleted bool
	BookmarksDeleted    int
}

type ScrollState = readingstate.ScrollState
type ReadingState = readingstate.State
type Bookmark = bookmarks.Bookmark
type ReaderPreferences = preferences.Preferences
type NovelReaderSettings = novelsettings.Settings
type NovelReaderCorrection = novelsettings.Correction
type NovelReaderCorrectionPatch = novelsettings.Patch

type ReadingStatePutInput struct {
	ReadingState
	ExpectedStateVersion *int
}

func New(dataDir string) *Store {
	stateDir := filepath.Join(dataDir, "state")
	return &Store{
		stateDir:      stateDir,
		readingState:  readingstate.NewRepository(stateDir),
		bookmarks:     bookmarks.NewRepository(stateDir),
		preferences:   preferences.NewRepository(stateDir),
		novelSettings: novelsettings.NewRepository(stateDir),
		aiSettings:    aisettings.NewRepository(stateDir),
	}
}

func (s *Store) Initialize() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(s.stateDir, 0o755); err != nil {
		return err
	}
	if err := s.readingState.Ensure(); err != nil {
		return err
	}
	if err := s.bookmarks.Ensure(); err != nil {
		return err
	}
	if err := s.preferences.Ensure(); err != nil {
		return err
	}
	if err := s.novelSettings.Ensure(); err != nil {
		return err
	}
	if err := s.aiSettings.Ensure(); err != nil {
		return err
	}
	return nil
}

func (s *Store) GetReadingState(novelID string) (ReadingState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.readingState.Get(novelID)
}

func (s *Store) PutReadingState(input ReadingStatePutInput) (ReadingState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.readingState.Put(readingstate.PutInput{
		State:                input.ReadingState,
		ExpectedStateVersion: input.ExpectedStateVersion,
	})
}

func (s *Store) GetReaderPreferences() (ReaderPreferences, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.preferences.Get()
}

func (s *Store) PutReaderPreferences(input ReaderPreferences) (ReaderPreferences, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.preferences.Put(input)
}

func (s *Store) GetNovelReaderSettings(novelID string) (NovelReaderSettings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.novelSettings.Get(novelID)
}

func (s *Store) PutNovelReaderSettings(input NovelReaderSettings) (NovelReaderSettings, error) {
	return s.PatchNovelReaderSettings(input.NovelID, NovelReaderCorrectionPatch{
		QuoteNormalization:                     boolPtr(input.Correction.QuoteNormalization),
		HyphenDashNormalization:                boolPtr(input.Correction.HyphenDashNormalization),
		ParenthesisNormalization:               boolPtr(input.Correction.ParenthesisNormalization),
		HalfwidthAlnumPunctuationNormalization: boolPtr(input.Correction.HalfwidthAlnumPunctuationNormalization),
	})
}

func (s *Store) PatchNovelReaderSettings(novelID string, patch NovelReaderCorrectionPatch) (NovelReaderSettings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if deleted, err := s.isNovelDeletedLocked(novelID); err != nil {
		return NovelReaderSettings{}, err
	} else if deleted {
		return NovelReaderSettings{}, ErrNovelStateDeleted
	}

	return s.novelSettings.Patch(novelID, patch)
}

func (s *Store) ListBookmarks(novelID string) ([]Bookmark, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.bookmarks.List(novelID)
}

func (s *Store) CreateBookmark(input Bookmark) (Bookmark, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if deleted, err := s.isNovelDeletedLocked(input.NovelID); err != nil {
		return Bookmark{}, err
	} else if deleted {
		return Bookmark{}, ErrNovelStateDeleted
	}

	return s.bookmarks.Create(input)
}

func (s *Store) DeleteBookmark(bookmarkID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.bookmarks.Delete(bookmarkID)
}

func (s *Store) PruneNovelState(novelID string) (NovelStatePruneResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	novelID = strings.TrimSpace(novelID)
	if novelID == "" {
		return NovelStatePruneResult{}, nil
	}
	if err := s.preflightPruneNovelStateLocked(novelID); err != nil {
		return NovelStatePruneResult{}, err
	}

	result := NovelStatePruneResult{}
	readingStateDeleted, err := s.readingState.Prune(novelID)
	if err != nil {
		return NovelStatePruneResult{}, err
	}
	result.ReadingStateDeleted = readingStateDeleted

	bookmarksDeleted, err := s.bookmarks.PruneNovel(novelID)
	if err != nil {
		return NovelStatePruneResult{}, err
	}
	result.BookmarksDeleted = bookmarksDeleted
	if _, err := s.novelSettings.PruneNovel(novelID); err != nil {
		return NovelStatePruneResult{}, err
	}
	return result, nil
}

func (s *Store) PreflightPruneNovelState(novelID string) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	novelID = strings.TrimSpace(novelID)
	if novelID == "" {
		return nil
	}
	return s.preflightPruneNovelStateLocked(novelID)
}

func (s *Store) preflightPruneNovelStateLocked(novelID string) error {
	if _, err := s.readingState.Get(novelID); err != nil {
		return err
	}
	if _, err := s.bookmarks.List(novelID); err != nil {
		return err
	}
	if _, err := s.novelSettings.Get(novelID); err != nil {
		return err
	}
	return nil
}

func (s *Store) isNovelDeletedLocked(novelID string) (bool, error) {
	return s.readingState.IsDeleted(novelID)
}

func IsEpisodeIndex(value string) bool {
	return bookmarks.IsEpisodeIndex(value)
}

func NormalizeEpisodeIndex(value any) (string, bool) {
	switch typed := value.(type) {
	case string:
		if IsEpisodeIndex(typed) {
			return typed, true
		}
	case float64:
		if typed >= 0 && typed == float64(int64(typed)) {
			return strconv.FormatInt(int64(typed), 10), true
		}
	}
	return "", false
}

func NormalizePosition(value any) (int, bool) {
	switch typed := value.(type) {
	case float64:
		if typed >= 0 && typed == float64(int64(typed)) {
			return int(typed), true
		}
	case string:
		if IsEpisodeIndex(typed) {
			parsed, err := strconv.Atoi(typed)
			return parsed, err == nil
		}
	}
	return 0, false
}

func NormalizeClientID(value any) (*string, bool) {
	if value == nil {
		return nil, true
	}
	typed, ok := value.(string)
	if !ok {
		return nil, false
	}
	trimmed := strings.TrimSpace(typed)
	if trimmed == "" {
		return nil, false
	}
	return &trimmed, true
}

func NormalizeScrollState(value any) (*ScrollState, bool) {
	if value == nil {
		return nil, true
	}
	record, ok := value.(map[string]any)
	if !ok || record["type"] != "ratio" {
		return nil, false
	}
	number, ok := record["value"].(float64)
	if !ok {
		return nil, false
	}
	return &ScrollState{Type: "ratio", Value: min(1, max(0, number))}, true
}

func IsReadingMode(value string) bool {
	return preferences.IsReadingMode(value)
}

func IsReaderFontFamily(value string) bool {
	return preferences.IsFontFamily(value)
}

func IsReaderTheme(value string) bool {
	return preferences.IsTheme(value)
}

func isoNow() string {
	return time.Now().UTC().Format(readerPreferencesUpdated)
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

func stringPtrOrNil(value *string) *string {
	if value == nil {
		return nil
	}
	return value
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
