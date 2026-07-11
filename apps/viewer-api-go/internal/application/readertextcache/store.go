package readertextcache

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"

	"narou-viewer/apps/viewer-api-go/internal/extraction"
	"narou-viewer/apps/viewer-api-go/internal/library"
)

const fileName = "reader_search.sqlite"
const maxLookupKeysPerQuery = 400

type Store struct {
	dbPath string
	mu     sync.Mutex
	db     *sql.DB
}

type Entry struct {
	Text            string
	PlainTextLength int
}

type Key struct {
	EpisodeIndex string
	ContentEtag  string
}

type LookupKey = Key

func New(stateDir string) *Store {
	stateDir = strings.TrimSpace(stateDir)
	if stateDir == "" {
		return nil
	}
	return NewAtPath(filepath.Join(stateDir, fileName))
}

func NewAtPath(dbPath string) *Store {
	dbPath = strings.TrimSpace(dbPath)
	if dbPath == "" {
		return nil
	}
	return &Store{dbPath: dbPath}
}

func BodyText(document library.ReaderDocument) string {
	parts := []string{}
	for _, block := range document.Blocks {
		switch block.Type {
		case "paragraph", "heading":
			if text := strings.TrimSpace(extraction.RenderInlineTokens(block.Inlines)); text != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.Join(parts, "\n")
}

func (s *Store) Get(ctx context.Context, novelID string, episodeIndex string, contentEtag string) (Entry, bool, error) {
	if s == nil || strings.TrimSpace(contentEtag) == "" {
		return Entry{}, false, nil
	}
	db, err := s.open(ctx)
	if err != nil {
		return Entry{}, false, err
	}
	return getWithDB(ctx, db, novelID, episodeIndex, contentEtag)
}

func (s *Store) GetMany(ctx context.Context, novelID string, keys []LookupKey) (map[Key]Entry, error) {
	if s == nil || len(keys) == 0 {
		return map[Key]Entry{}, nil
	}
	db, err := s.open(ctx)
	if err != nil {
		return nil, err
	}
	entries := map[Key]Entry{}
	normalizedKeys := normalizeLookupKeys(keys)
	for start := 0; start < len(normalizedKeys); start += maxLookupKeysPerQuery {
		end := start + maxLookupKeysPerQuery
		if end > len(normalizedKeys) {
			end = len(normalizedKeys)
		}
		chunkEntries, err := getManyWithDB(ctx, db, novelID, normalizedKeys[start:end])
		if err != nil {
			return nil, err
		}
		for key, entry := range chunkEntries {
			entries[key] = entry
		}
	}
	return entries, nil
}

func (s *Store) Save(ctx context.Context, novelID string, episodeIndex string, contentEtag string, text string) error {
	if s == nil || strings.TrimSpace(contentEtag) == "" {
		return nil
	}
	db, err := s.open(ctx)
	if err != nil {
		return err
	}
	unchanged, err := hasUnchangedCurrentRow(ctx, db, novelID, episodeIndex, contentEtag, text)
	if err != nil {
		return err
	}
	if unchanged {
		return nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `
INSERT INTO reader_search_texts (
	novel_id,
	episode_index,
	content_etag,
	text,
	plain_text_length,
	updated_at
) VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(novel_id, episode_index, content_etag) DO UPDATE SET
	text = excluded.text,
	plain_text_length = excluded.plain_text_length,
	updated_at = CURRENT_TIMESTAMP
`, novelID, episodeIndex, contentEtag, text, len([]rune(text)))
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
DELETE FROM reader_search_texts
WHERE novel_id = ? AND episode_index = ? AND content_etag <> ?
`, novelID, episodeIndex, contentEtag); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) PruneByNovelID(ctx context.Context, novelID string) (int, error) {
	if s == nil {
		return 0, nil
	}
	novelID = strings.TrimSpace(novelID)
	if novelID == "" {
		return 0, nil
	}
	if _, err := os.Stat(s.dbPath); errors.Is(err, os.ErrNotExist) {
		return 0, nil
	} else if err != nil {
		return 0, err
	}
	db, err := s.open(ctx)
	if err != nil {
		return 0, err
	}
	result, err := db.ExecContext(ctx, `DELETE FROM reader_search_texts WHERE novel_id = ?`, novelID)
	if err != nil {
		return 0, err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(rowsAffected), nil
}

func (s *Store) open(ctx context.Context) (*sql.DB, error) {
	if s == nil {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db != nil {
		return s.db, nil
	}
	if err := ensureParentDir(s.dbPath); err != nil {
		return nil, err
	}
	if err := ensureDBFileMode(s.dbPath); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", readerSearchSQLiteDSN(s.dbPath))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	initCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := initSchema(initCtx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	s.db = db
	return s.db, nil
}

func ensureParentDir(dbPath string) error {
	return os.MkdirAll(filepath.Dir(dbPath), 0o755)
}

func ensureDBFileMode(dbPath string) error {
	file, err := os.OpenFile(dbPath, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Chmod(dbPath, 0o600)
}

func getWithDB(ctx context.Context, db *sql.DB, novelID string, episodeIndex string, contentEtag string) (Entry, bool, error) {
	var entry Entry
	err := db.QueryRowContext(ctx, `
SELECT text, plain_text_length
FROM reader_search_texts
WHERE novel_id = ? AND episode_index = ? AND content_etag = ?
`, novelID, episodeIndex, contentEtag).Scan(&entry.Text, &entry.PlainTextLength)
	if err == nil {
		return entry, true, nil
	}
	if err == sql.ErrNoRows {
		return Entry{}, false, nil
	}
	return Entry{}, false, err
}

func hasUnchangedCurrentRow(ctx context.Context, db *sql.DB, novelID string, episodeIndex string, contentEtag string, text string) (bool, error) {
	var textMatches bool
	var plainTextLength int
	var staleCount int
	err := db.QueryRowContext(ctx, `
SELECT text = ?, plain_text_length,
	(SELECT COUNT(*) FROM reader_search_texts WHERE novel_id = ? AND episode_index = ? AND content_etag <> ?)
FROM reader_search_texts
WHERE novel_id = ? AND episode_index = ? AND content_etag = ?
`, text, novelID, episodeIndex, contentEtag, novelID, episodeIndex, contentEtag).Scan(&textMatches, &plainTextLength, &staleCount)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return textMatches && plainTextLength == len([]rune(text)) && staleCount == 0, nil
}

func getManyWithDB(ctx context.Context, db *sql.DB, novelID string, keys []LookupKey) (map[Key]Entry, error) {
	if len(keys) == 0 {
		return map[Key]Entry{}, nil
	}
	var query strings.Builder
	query.WriteString(`
SELECT episode_index, content_etag, text, plain_text_length
FROM reader_search_texts
WHERE novel_id = ? AND (
`)
	args := []any{novelID}
	for i, key := range keys {
		if i > 0 {
			query.WriteString(" OR ")
		}
		query.WriteString("(episode_index = ? AND content_etag = ?)")
		args = append(args, key.EpisodeIndex, key.ContentEtag)
	}
	query.WriteString(")")
	rows, err := db.QueryContext(ctx, query.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := map[Key]Entry{}
	for rows.Next() {
		var key Key
		var entry Entry
		if err := rows.Scan(&key.EpisodeIndex, &key.ContentEtag, &entry.Text, &entry.PlainTextLength); err != nil {
			return nil, err
		}
		entries[key] = entry
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

func normalizeLookupKeys(keys []LookupKey) []LookupKey {
	normalized := make([]LookupKey, 0, len(keys))
	seen := map[LookupKey]struct{}{}
	for _, key := range keys {
		key.EpisodeIndex = strings.TrimSpace(key.EpisodeIndex)
		key.ContentEtag = strings.TrimSpace(key.ContentEtag)
		if key.EpisodeIndex == "" || key.ContentEtag == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, key)
	}
	return normalized
}

func initSchema(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS reader_search_texts (
	novel_id TEXT NOT NULL,
	episode_index TEXT NOT NULL,
	content_etag TEXT NOT NULL,
	text TEXT NOT NULL,
	plain_text_length INTEGER NOT NULL,
	updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	PRIMARY KEY (novel_id, episode_index, content_etag)
);
CREATE INDEX IF NOT EXISTS idx_reader_search_texts_episode
	ON reader_search_texts(novel_id, episode_index);
`)
	return err
}

func readerSearchSQLiteDSN(dbPath string) string {
	return "file:" + filepath.ToSlash(dbPath) + "?_pragma=busy_timeout(5000)"
}
