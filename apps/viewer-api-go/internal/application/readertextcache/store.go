package readertextcache

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"

	"narou-viewer/apps/viewer-api-go/internal/extraction"
	"narou-viewer/apps/viewer-api-go/internal/library"
	"narou-viewer/apps/viewer-api-go/internal/state/filequarantine"
)

const FileName = "reader_search.sqlite"
const CacheVersion = 1
const NormalizationContractVersion = 1
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
	return NewAtPath(filepath.Join(stateDir, FileName))
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
	return s.openLocked(ctx)
}

func (s *Store) openLocked(ctx context.Context) (*sql.DB, error) {
	if err := ensureParentDir(s.dbPath); err != nil {
		return nil, err
	}
	_, statErr := os.Stat(s.dbPath)
	isNew := errors.Is(statErr, os.ErrNotExist)
	if statErr != nil && !isNew {
		return nil, statErr
	}
	if isNew {
		if err := ensureDBFileMode(s.dbPath); err != nil {
			return nil, err
		}
	}
	db, err := openSQLite(s.dbPath)
	if err != nil {
		return nil, err
	}
	initCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if !isNew {
		label, validationErr := validateExistingCache(initCtx, db)
		if validationErr != nil {
			if err := db.Close(); err != nil {
				return nil, err
			}
			if _, err := quarantineCacheFiles(s.dbPath, label); err != nil {
				return nil, err
			}
			if err := ensureDBFileMode(s.dbPath); err != nil {
				return nil, err
			}
			db, err = openSQLite(s.dbPath)
			if err != nil {
				return nil, err
			}
			isNew = true
		}
	}
	if isNew {
		if err := initSchema(initCtx, db); err != nil {
			_ = db.Close()
			return nil, err
		}
	}
	if err := os.Chmod(s.dbPath, 0o600); err != nil {
		_ = db.Close()
		return nil, err
	}
	s.db = db
	return s.db, nil
}

func openSQLite(dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", readerSearchSQLiteDSN(dbPath))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return db, nil
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
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
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
`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", CacheVersion)); err != nil {
		return err
	}
	return tx.Commit()
}

func readerSearchSQLiteDSN(dbPath string) string {
	return "file:" + filepath.ToSlash(dbPath) + "?_pragma=busy_timeout(5000)"
}

func validateExistingCache(ctx context.Context, db *sql.DB) (string, error) {
	var version int
	if err := db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil {
		return "corrupt", err
	}
	if version != CacheVersion {
		return "unsupported", fmt.Errorf("reader search cache version %d is unsupported; current version is %d", version, CacheVersion)
	}
	if err := ValidateSchema(ctx, db); err != nil {
		return "corrupt", err
	}
	rows, err := db.QueryContext(ctx, `PRAGMA quick_check`)
	if err != nil {
		return "corrupt", err
	}
	defer rows.Close()
	sawResult := false
	for rows.Next() {
		sawResult = true
		var result string
		if err := rows.Scan(&result); err != nil {
			return "corrupt", err
		}
		if result != "ok" {
			return "corrupt", fmt.Errorf("reader search cache quick_check failed: %s", result)
		}
	}
	if err := rows.Err(); err != nil {
		return "corrupt", err
	}
	if !sawResult {
		return "corrupt", errors.New("reader search cache quick_check returned no result")
	}
	return "", nil
}

func ValidateSchema(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `PRAGMA table_info(reader_search_texts)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	columns := map[string]bool{}
	primaryKeyColumns := map[int]string{}
	for rows.Next() {
		var cid int
		var name string
		var columnType string
		var notNull int
		var defaultValue sql.NullString
		var primaryKey int
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return err
		}
		columns[name] = true
		if primaryKey > 0 {
			primaryKeyColumns[primaryKey] = name
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	requiredColumns := []string{"novel_id", "episode_index", "content_etag", "text", "plain_text_length", "updated_at"}
	for _, column := range requiredColumns {
		if !columns[column] {
			return fmt.Errorf("reader search cache column %s is missing", column)
		}
	}
	conflictColumns := []string{"novel_id", "episode_index", "content_etag"}
	if primaryKeyMatches(primaryKeyColumns, conflictColumns) {
		return nil
	}
	indexRows, err := db.QueryContext(ctx, `PRAGMA index_list(reader_search_texts)`)
	if err != nil {
		return err
	}
	defer indexRows.Close()
	uniqueIndexes := []string{}
	for indexRows.Next() {
		var sequence int
		var name string
		var unique int
		var origin string
		var partial int
		if err := indexRows.Scan(&sequence, &name, &unique, &origin, &partial); err != nil {
			return err
		}
		if unique == 0 || partial != 0 {
			continue
		}
		uniqueIndexes = append(uniqueIndexes, name)
	}
	if err := indexRows.Err(); err != nil {
		_ = indexRows.Close()
		return err
	}
	if err := indexRows.Close(); err != nil {
		return err
	}
	for _, name := range uniqueIndexes {
		indexColumns, err := sqliteIndexColumns(ctx, db, name)
		if err != nil {
			return err
		}
		if sameColumnSet(indexColumns, conflictColumns) {
			return nil
		}
	}
	return errors.New("reader search cache requires a UNIQUE or PRIMARY KEY constraint on (novel_id, episode_index, content_etag)")
}

func primaryKeyMatches(columns map[int]string, expected []string) bool {
	if len(columns) != len(expected) {
		return false
	}
	actual := make([]string, 0, len(columns))
	for ordinal := 1; ordinal <= len(columns); ordinal++ {
		actual = append(actual, columns[ordinal])
	}
	return sameColumnSet(actual, expected)
}

func sqliteIndexColumns(ctx context.Context, db *sql.DB, indexName string) ([]string, error) {
	query := `SELECT name FROM pragma_index_info('` + strings.ReplaceAll(indexName, `'`, `''`) + `') ORDER BY seqno`
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	columns := []string{}
	for rows.Next() {
		var name sql.NullString
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		if !name.Valid {
			columns = append(columns, "")
			continue
		}
		columns = append(columns, name.String)
	}
	return columns, rows.Err()
}

func sameColumnSet(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	seen := map[string]bool{}
	for _, value := range left {
		seen[value] = true
	}
	for _, value := range right {
		if !seen[value] {
			return false
		}
	}
	return true
}

func quarantineCacheFiles(dbPath string, label string) (string, error) {
	quarantinedPath, err := filequarantine.Move(dbPath, label)
	if err != nil {
		return "", err
	}
	for _, suffix := range []string{"-journal", "-wal", "-shm"} {
		path := dbPath + suffix
		if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return "", err
		}
		if _, err := filequarantine.Move(path, label); err != nil {
			return "", err
		}
	}
	return quarantinedPath, nil
}

func (s *Store) Rebuild(ctx context.Context) (string, error) {
	if s == nil {
		return "", nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db != nil {
		if err := s.db.Close(); err != nil {
			return "", err
		}
		s.db = nil
	}
	quarantinedPath := ""
	if _, err := os.Stat(s.dbPath); err == nil {
		var quarantineErr error
		quarantinedPath, quarantineErr = quarantineCacheFiles(s.dbPath, "rebuild")
		if quarantineErr != nil {
			return "", quarantineErr
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if _, err := s.openLocked(ctx); err != nil {
		return quarantinedPath, err
	}
	return quarantinedPath, nil
}
