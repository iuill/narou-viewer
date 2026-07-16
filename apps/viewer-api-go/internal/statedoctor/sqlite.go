package statedoctor

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"narou-viewer/apps/viewer-api-go/internal/ai/usagemigration"
	"narou-viewer/apps/viewer-api-go/internal/application/readertextcache"
	"narou-viewer/apps/viewer-api-go/internal/library"
	"narou-viewer/apps/viewer-api-go/internal/state/safefile"

	_ "modernc.org/sqlite"
)

const nfLibrarySupportedMigration = 3
const nfCanonicalEpisodeVersion = 1

func (s *scanner) scanSQLite(ctx context.Context) {
	s.scanAIUsageSQLite(ctx)
	s.scanReaderSearchSQLite(ctx)
	s.scanNovelFetcherSQLite(ctx)
}

func (s *scanner) scanAIUsageSQLite(ctx context.Context) {
	path := filepath.Join(s.stateDir, "ai_usage.sqlite")
	db, exists, err := openReadOnlySQLite(path)
	if err != nil {
		s.addSQLiteOpenError("VA-AI-USAGE", path, err, strconv.Itoa(usagemigration.SupportedLatestVersion))
		return
	}
	if !exists {
		s.add(Finding{SchemaID: "VA-AI-USAGE", Path: s.rel(path), Kind: "missing", Severity: SeverityInfo, Observed: "missing", Supported: strconv.Itoa(usagemigration.SupportedLatestVersion), RecoveryHint: "AI 利用履歴が未生成なら正常です。"})
		return
	}
	defer db.Close()
	s.scanQuickCheck(ctx, "VA-AI-USAGE", path, db)
	existsLedger, observed, err := sqliteMigrationVersion(ctx, db)
	if err != nil {
		s.add(Finding{SchemaID: "VA-AI-USAGE", Path: s.rel(path), Kind: "migration_ledger_error", Severity: SeverityError, Observed: "unreadable", Supported: strconv.Itoa(usagemigration.SupportedLatestVersion), RecoveryHint: "DB を変更せず supported backup または対応 build で復旧してください。"})
		return
	}
	severity := SeverityInfo
	kind := "schema_current"
	observedText := strconv.Itoa(observed)
	hint := "番号付き migration ledger は current です。"
	if !existsLedger {
		severity = SeverityWarning
		kind = "schema_legacy"
		observedText = "no ledger"
		hint = "対応 build で baseline migration 1 を transaction 内で適用してください。"
	} else if observed > usagemigration.SupportedLatestVersion {
		severity = SeverityError
		kind = "schema_future_unknown"
		hint = "対応する新しい build または supported backup を使い、自動 drop / prune しないでください。"
	}
	if err := usagemigration.Preflight(db, path); err != nil && observed <= usagemigration.SupportedLatestVersion {
		severity = SeverityError
		kind = "schema_invalid"
		hint = "partial baseline を自動補完せず、supported backup または明示 recovery を使ってください。"
	}
	s.add(Finding{SchemaID: "VA-AI-USAGE", Path: s.rel(path), Kind: kind, Severity: severity, Observed: observedText, Supported: strconv.Itoa(usagemigration.SupportedLatestVersion), RecoveryHint: hint})
}

func (s *scanner) scanReaderSearchSQLite(ctx context.Context) {
	path := filepath.Join(s.stateDir, readertextcache.FileName)
	db, exists, err := openReadOnlySQLite(path)
	if err != nil {
		s.add(Finding{SchemaID: "VA-READER-SEARCH", Path: s.rel(path), Kind: "sqlite_open_error", Severity: SeverityWarning, Observed: "unreadable", Supported: strconv.Itoa(readertextcache.CacheVersion), RecoveryHint: "再生成可能 cache です。connection を閉じて quarantine 後に lazy rebuild してください。", RepairKind: repairReaderSearch, RepairTarget: path})
		return
	}
	if !exists {
		s.add(Finding{SchemaID: "VA-READER-SEARCH", Path: s.rel(path), Kind: "missing", Severity: SeverityInfo, Observed: "missing", Supported: strconv.Itoa(readertextcache.CacheVersion), RecoveryHint: "検索時に lazy 生成されます。"})
		return
	}
	defer db.Close()
	quickOK := s.scanQuickCheck(ctx, "VA-READER-SEARCH", path, db)
	var version int
	if err := db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil {
		s.add(Finding{SchemaID: "VA-READER-SEARCH", Path: s.rel(path), Kind: "cache_version_error", Severity: SeverityWarning, Observed: "unreadable", Supported: strconv.Itoa(readertextcache.CacheVersion), RecoveryHint: "connection を閉じて cache を quarantine / rebuild してください。", RepairKind: repairReaderSearch, RepairTarget: path})
		return
	}
	schemaOK := readertextcache.ValidateSchema(ctx, db) == nil
	if version != readertextcache.CacheVersion || !quickOK || !schemaOK {
		kind := "cache_version_mismatch"
		if version == readertextcache.CacheVersion && !schemaOK {
			kind = "cache_schema_mismatch"
		}
		s.add(Finding{SchemaID: "VA-READER-SEARCH", Path: s.rel(path), Kind: kind, Severity: SeverityWarning, Observed: strconv.Itoa(version), Supported: strconv.Itoa(readertextcache.CacheVersion), RecoveryHint: "旧 DB を上書きせず、connection close 後に quarantine して lazy rebuild してください。", RepairKind: repairReaderSearch, RepairTarget: path})
		return
	}
	s.add(Finding{SchemaID: "VA-READER-SEARCH", Path: s.rel(path), Kind: "schema_current", Severity: SeverityInfo, Observed: strconv.Itoa(version), Supported: strconv.Itoa(readertextcache.CacheVersion), RecoveryHint: "table schema と text normalization contract は current です。"})
}

func (s *scanner) scanNovelFetcherSQLite(ctx context.Context) {
	path := filepath.Join(s.novelFetcherDir, "library.sqlite")
	db, exists, err := openReadOnlySQLite(path)
	if err != nil {
		s.addSQLiteOpenError("NF-LIBRARY", path, err, strconv.Itoa(nfLibrarySupportedMigration))
		return
	}
	if !exists {
		s.add(Finding{SchemaID: "NF-LIBRARY", Path: s.rel(path), Kind: "missing", Severity: SeverityInfo, Observed: "missing", Supported: strconv.Itoa(nfLibrarySupportedMigration), RecoveryHint: "novel-fetcher storage が未生成なら正常です。"})
		return
	}
	defer db.Close()
	quickOK := s.scanQuickCheck(ctx, "NF-LIBRARY", path, db)
	existsLedger, observed, ledgerErr := sqliteMigrationVersion(ctx, db)
	if ledgerErr != nil {
		s.add(Finding{SchemaID: "NF-LIBRARY", Path: s.rel(path), Kind: "migration_ledger_error", Severity: SeverityError, Observed: "unreadable", Supported: strconv.Itoa(nfLibrarySupportedMigration), RecoveryHint: "DB と works/** を同じ consistency group の backup から復旧してください。"})
		return
	}
	severity := SeverityInfo
	kind := "schema_current"
	observedText := strconv.Itoa(observed)
	hint := "migration ledger は current です。"
	if !existsLedger {
		severity = SeverityWarning
		kind = "schema_legacy"
		observedText = "no ledger"
		hint = "対応 novel-fetcher build で番号付き migration を適用してください。"
	} else if observed > nfLibrarySupportedMigration {
		severity = SeverityError
		kind = "schema_future_unknown"
		hint = "対応する新しい novel-fetcher build または consistency group backup を使ってください。"
	}
	s.add(Finding{SchemaID: "NF-LIBRARY", Path: s.rel(path), Kind: kind, Severity: severity, Observed: observedText, Supported: strconv.Itoa(nfLibrarySupportedMigration), RecoveryHint: hint})
	if !quickOK || observed > nfLibrarySupportedMigration {
		return
	}
	if err := s.scanNovelFetcherRows(ctx, db); err != nil {
		s.add(Finding{SchemaID: "NF-LIBRARY", Path: s.rel(path), Kind: "storage_contract_error", Severity: SeverityError, Observed: "query failed", Supported: strconv.Itoa(nfLibrarySupportedMigration), RecoveryHint: "対応 schema の library.sqlite と works/** を同じ snapshot から復旧してください。"})
	}
}

func (s *scanner) scanNovelFetcherRows(ctx context.Context, db *sql.DB) error {
	workRows, err := db.QueryContext(ctx, `SELECT id, site, site_work_id FROM works`)
	if err != nil {
		return err
	}
	for workRows.Next() {
		var work library.Work
		if err := workRows.Scan(&work.ID, &work.Site, &work.SiteWorkID); err != nil {
			workRows.Close()
			return err
		}
		s.libraryNovelIDs[library.NovelID(work)] = true
	}
	if err := workRows.Close(); err != nil {
		return err
	}
	s.libraryReadable = true

	episodeRows, err := db.QueryContext(ctx, `SELECT body_path, content_hash FROM episodes WHERE body_path <> ''`)
	if err != nil {
		return err
	}
	defer episodeRows.Close()
	referenced := map[string]bool{}
	for episodeRows.Next() {
		var bodyPath string
		var contentHash string
		if err := episodeRows.Scan(&bodyPath, &contentHash); err != nil {
			return err
		}
		clean, ok := safeRelativeStoragePath(bodyPath)
		if !ok {
			s.add(Finding{SchemaID: "NF-CANONICAL-EPISODE", Path: s.rel(filepath.Join(s.novelFetcherDir, bodyPath)), Kind: "unsafe_body_path", Severity: SeverityError, Observed: "outside novel-fetcher root", Supported: "relative works/** path", RecoveryHint: "DB を直接正規化せず、対応 backup から DB と works/** を一体で復旧してください。"})
			continue
		}
		absolute := filepath.Join(s.novelFetcherDir, clean)
		referenced[filepath.Clean(absolute)] = true
		s.scanCanonicalEpisode(absolute, contentHash)
	}
	if err := episodeRows.Err(); err != nil {
		return err
	}
	return s.scanUnreferencedCanonicalEpisodes(referenced)
}

func (s *scanner) scanCanonicalEpisode(path string, expectedHash string) {
	info, lstatErr := os.Lstat(path)
	if lstatErr == nil && info.Mode()&os.ModeSymlink != 0 {
		s.add(Finding{SchemaID: "NF-CANONICAL-EPISODE", Path: s.rel(path), Kind: "symlink_not_scanned", Severity: SeverityError, Observed: "symlink", Supported: "regular file schema v1", RecoveryHint: "link 先を読まず、NF-CANONICAL consistency group 内の regular file を復旧してください。"})
		return
	}
	if lstatErr == nil {
		resolvedRoot, rootErr := filepath.EvalSymlinks(s.novelFetcherDir)
		resolvedPath, pathErr := filepath.EvalSymlinks(path)
		if rootErr != nil || pathErr != nil || !pathWithinRoot(resolvedRoot, resolvedPath) {
			s.add(Finding{SchemaID: "NF-CANONICAL-EPISODE", Path: s.rel(path), Kind: "resolved_path_outside_storage", Severity: SeverityError, Observed: "outside novel-fetcher root", Supported: "regular file under novel-fetcher/works", RecoveryHint: "link 経由の外部 file を読まず、storage root 内へ consistency group を復旧してください。"})
			return
		}
	}
	raw, err := safefile.ReadRegular(path, safefile.MaxCanonicalStateBytes)
	if errors.Is(err, os.ErrNotExist) {
		s.add(Finding{SchemaID: "NF-CANONICAL-EPISODE", Path: s.rel(path), Kind: "missing_body_file", Severity: SeverityError, Observed: "missing", Supported: strconv.Itoa(nfCanonicalEpisodeVersion), RecoveryHint: "library.sqlite と works/** を同じ consistency group backup から復旧するか、明示再取得してください。"})
		return
	}
	if err != nil {
		s.add(Finding{SchemaID: "NF-CANONICAL-EPISODE", Path: s.rel(path), Kind: "read_error", Severity: SeverityError, Observed: "unreadable", Supported: strconv.Itoa(nfCanonicalEpisodeVersion), RecoveryHint: "権限と storage を確認し、元 file を変更しないでください。"})
		return
	}
	var header struct {
		SchemaVersion *int `json:"schema_version"`
	}
	if err := json.Unmarshal(raw, &header); err != nil {
		s.add(Finding{SchemaID: "NF-CANONICAL-EPISODE", Path: s.rel(path), Kind: "schema_malformed", Severity: SeverityError, Observed: "malformed", Supported: strconv.Itoa(nfCanonicalEpisodeVersion), RecoveryHint: "自動削除せず、同じ consistency group の backup または明示再取得で復旧してください。"})
		return
	}
	observed := "missing"
	if header.SchemaVersion != nil {
		observed = strconv.Itoa(*header.SchemaVersion)
	}
	if header.SchemaVersion == nil || *header.SchemaVersion != nfCanonicalEpisodeVersion {
		s.add(Finding{SchemaID: "NF-CANONICAL-EPISODE", Path: s.rel(path), Kind: "schema_unsupported", Severity: SeverityError, Observed: observed, Supported: strconv.Itoa(nfCanonicalEpisodeVersion), RecoveryHint: "対応 novel-fetcher build または consistency group backup を使い、現行 build で再保存しないでください。"})
		return
	}
	s.add(Finding{SchemaID: "NF-CANONICAL-EPISODE", Path: s.rel(path), Kind: "schema_current", Severity: SeverityInfo, Observed: observed, Supported: strconv.Itoa(nfCanonicalEpisodeVersion), RecoveryHint: "canonical episode schema は current です。"})
	sum := sha256.Sum256(raw)
	actualHash := "sha256:" + hex.EncodeToString(sum[:])
	if strings.TrimSpace(expectedHash) != "" && actualHash != expectedHash {
		s.add(Finding{SchemaID: "NF-CANONICAL-EPISODE", Path: s.rel(path), Kind: "content_hash_mismatch", Severity: SeverityError, Observed: "mismatch", Supported: "DB content_hash", RecoveryHint: "DB と file の片側だけを修正せず、同一 snapshot から復旧または明示再取得してください。"})
	}
}

func (s *scanner) scanUnreferencedCanonicalEpisodes(referenced map[string]bool) error {
	root := filepath.Join(s.novelFetcherDir, "works")
	if _, err := os.Stat(root); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" || filepath.Base(filepath.Dir(path)) != "episodes" {
			return nil
		}
		if !referenced[filepath.Clean(path)] {
			s.add(Finding{SchemaID: "NF-CANONICAL-EPISODE", Path: s.rel(path), Kind: "orphan_body_file", Severity: SeverityWarning, Observed: "not referenced by DB", Supported: "episode row + body file", RecoveryHint: "自動削除せず、library.sqlite と works/** の snapshot generation を確認してください。"})
		}
		return nil
	})
}

func (s *scanner) scanQuickCheck(ctx context.Context, schemaID string, path string, db *sql.DB) bool {
	rows, err := db.QueryContext(ctx, `PRAGMA quick_check`)
	if err != nil {
		s.add(Finding{SchemaID: schemaID, Path: s.rel(path), Kind: "sqlite_integrity_error", Severity: SeverityError, Observed: "quick_check failed", Supported: "ok", RecoveryHint: sqliteRecoveryHint(schemaID)})
		return false
	}
	defer rows.Close()
	ok := false
	for rows.Next() {
		var result string
		if err := rows.Scan(&result); err != nil {
			s.add(Finding{SchemaID: schemaID, Path: s.rel(path), Kind: "sqlite_integrity_error", Severity: SeverityError, Observed: "quick_check unreadable", Supported: "ok", RecoveryHint: sqliteRecoveryHint(schemaID)})
			return false
		}
		if result == "ok" {
			ok = true
			continue
		}
		s.add(Finding{SchemaID: schemaID, Path: s.rel(path), Kind: "sqlite_integrity_error", Severity: SeverityError, Observed: "corrupt", Supported: "ok", RecoveryHint: sqliteRecoveryHint(schemaID)})
		return false
	}
	return ok && rows.Err() == nil
}

func openReadOnlySQLite(path string) (*sql.DB, bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, true, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, true, errors.New("refusing to follow sqlite symlink")
	}
	if info.Size() == 0 {
		return nil, true, errors.New("empty sqlite file")
	}
	dsn := "file:" + filepath.ToSlash(path) + "?mode=ro&_pragma=query_only(1)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, true, err
	}
	db.SetMaxOpenConns(1)
	return db, true, nil
}

func pathWithinRoot(root string, path string) bool {
	root = filepath.Clean(root)
	path = filepath.Clean(path)
	return path == root || strings.HasPrefix(path, root+string(filepath.Separator))
}

func sqliteMigrationVersion(ctx context.Context, db *sql.DB) (bool, int, error) {
	var exists int
	if err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = 'schema_migrations')`).Scan(&exists); err != nil {
		return false, 0, err
	}
	if exists == 0 {
		return false, 0, nil
	}
	var observed int
	if err := db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&observed); err != nil {
		return true, 0, err
	}
	return true, observed, nil
}

func (s *scanner) addSQLiteOpenError(schemaID string, path string, err error, supported string) {
	_ = err
	s.add(Finding{SchemaID: schemaID, Path: s.rel(path), Kind: "sqlite_open_error", Severity: SeverityError, Observed: "unreadable", Supported: supported, RecoveryHint: sqliteRecoveryHint(schemaID)})
}

func sqliteRecoveryHint(schemaID string) string {
	if schemaID == "VA-READER-SEARCH" {
		return "再生成可能 cache です。connection close 後に quarantine / rebuild してください。"
	}
	return "自動 drop せず、対応 build または同じ consistency group の supported backup を使ってください。"
}

func safeRelativeStoragePath(path string) (string, bool) {
	path = strings.TrimSpace(path)
	if path == "" || filepath.IsAbs(path) {
		return "", false
	}
	clean := filepath.Clean(path)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", false
	}
	return clean, true
}
