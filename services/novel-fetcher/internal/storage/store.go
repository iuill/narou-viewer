package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"narou-viewer/services/novel-fetcher/internal/fetcher"
	"narou-viewer/services/novel-fetcher/internal/model"
	storageassets "narou-viewer/services/novel-fetcher/internal/storage/assets"
	"narou-viewer/services/novel-fetcher/internal/storage/migration"
	"narou-viewer/services/novel-fetcher/internal/storage/pathutil"
	"narou-viewer/services/novel-fetcher/internal/taskstate"
)

type Store struct {
	rootDir           string
	db                *sql.DB
	assetMaterializer *storageassets.Materializer
}

const canonicalEpisodeSchemaVersion = 1

type ErrUnsupportedEpisodeSchema struct {
	Path      string
	Observed  *int
	Supported int
}

var ErrInvalidTaskEpisodeCheckpoint = errors.New("invalid task episode checkpoint")

func (e ErrUnsupportedEpisodeSchema) Error() string {
	observed := "missing"
	if e.Observed != nil {
		observed = strconv.Itoa(*e.Observed)
	}
	return fmt.Sprintf(
		"unsupported NF-CANONICAL-EPISODE schema at %q: observed %s, supported %d; use a compatible build or restore a supported backup",
		e.Path,
		observed,
		e.Supported,
	)
}

const (
	FetchStatusComplete    = "complete"
	FetchStatusPartial     = "partial"
	FetchStatusFailed      = "failed"
	FetchStatusCanceled    = "canceled"
	FetchStatusPaused      = "paused"
	FetchStatusInterrupted = "interrupted"

	BodyStatusPending  = "pending"
	BodyStatusComplete = "complete"
	BodyStatusFailed   = "failed"
)

type AssetFetcher interface {
	FetchBytes(ctx context.Context, rawURL string, policy fetcher.FetchPolicy) (fetcher.BinaryResponse, error)
}

type TaskEpisodeCheckpoint struct {
	Ref           taskstate.TaskRef
	WorkID        int
	EpisodeID     string
	SortOrder     int
	NextEpisodeID string
}

func NewStore(rootDir string) (*Store, error) {
	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		return nil, err
	}

	databasePath := filepath.Join(rootDir, "library.sqlite")
	if err := preflightLibrarySchema(databasePath); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", databasePath)
	if err != nil {
		return nil, err
	}
	// The fetcher is a single-writer process. Sharing one connection across
	// storage and taskstate avoids SQLITE_BUSY races between progress writes
	// and durable task control transactions.
	db.SetMaxOpenConns(1)
	store := &Store{rootDir: rootDir, db: db}
	if err := store.initialize(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func preflightLibrarySchema(databasePath string) error {
	if _, err := os.Stat(databasePath); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}

	absolutePath, err := filepath.Abs(databasePath)
	if err != nil {
		return err
	}
	readOnlyURL := url.URL{Scheme: "file", Path: filepath.ToSlash(absolutePath)}
	query := readOnlyURL.Query()
	query.Set("mode", "ro")
	readOnlyURL.RawQuery = query.Encode()

	readOnlyDB, err := sql.Open("sqlite", readOnlyURL.String())
	if err != nil {
		return err
	}
	readOnlyDB.SetMaxOpenConns(1)
	checkErr := migration.CheckSupported(readOnlyDB, databasePath)
	closeErr := readOnlyDB.Close()
	return errors.Join(checkErr, closeErr)
}

func (s *Store) Close() error {
	_, optimizeErr := s.db.Exec(`PRAGMA optimize`)
	_, checkpointErr := s.db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`)
	closeErr := s.db.Close()
	return errors.Join(optimizeErr, checkpointErr, closeErr)
}

func (s *Store) DB() *sql.DB {
	return s.db
}

func (s *Store) SetAssetFetcher(assetFetcher AssetFetcher, policy fetcher.FetchPolicy) {
	s.assetMaterializer = storageassets.NewMaterializer(s.rootDir, assetFetcher, policy)
}

func (s *Store) initialize() error {
	return migration.Run(s.db, filepath.Join(s.rootDir, "library.sqlite"))
}

func (s *Store) ListWorks() ([]model.StoredWork, error) {
	rows, err := s.db.Query(`
		SELECT w.id, w.site, w.site_name, w.site_work_id, w.source_url, w.title, w.author, w.story, w.directory, w.fetched_at,
			COUNT(e.episode_id), SUM(CASE WHEN e.body_status = 'complete' THEN 1 ELSE 0 END),
			w.fetch_status, w.last_fetch_error, w.last_failed_episode_id, w.resume_episode_id, w.expected_episode_count
		FROM works w
		LEFT JOIN episodes e ON e.work_id = w.id
		GROUP BY w.id
		ORDER BY w.fetched_at DESC, w.title ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	works := []model.StoredWork{}
	for rows.Next() {
		work, err := scanStoredWork(rows)
		if err != nil {
			return nil, err
		}
		works = append(works, work)
	}
	return works, rows.Err()
}

const (
	strongTitleSimilarityThreshold = 0.86
	weakTitleSimilarityThreshold   = 0.7
)

func (s *Store) FindPotentialDuplicateWorks(work model.Work) ([]model.StoredWork, error) {
	normalizedTitle := normalizeTitleForMatch(work.Title)
	if normalizedTitle == "" {
		return nil, nil
	}

	works, err := s.ListWorks()
	if err != nil {
		return nil, err
	}

	matches := []model.StoredWork{}
	for _, candidate := range works {
		score := titleSimilarityScore(normalizedTitle, normalizeTitleForMatch(candidate.Title))
		if score >= strongTitleSimilarityThreshold {
			matches = append(matches, candidate)
			continue
		}
		if score < weakTitleSimilarityThreshold {
			continue
		}

		episodes, err := s.ListEpisodes(candidate.ID)
		if err != nil {
			return nil, err
		}
		if similarLeadingEpisodeTitles(work.Episodes, episodes) {
			matches = append(matches, candidate)
		}
	}
	return matches, nil
}

func (s *Store) FindWorkByID(id int) (model.StoredWork, bool, error) {
	row := s.db.QueryRow(`
		SELECT w.id, w.site, w.site_name, w.site_work_id, w.source_url, w.title, w.author, w.story, w.directory, w.fetched_at,
			COUNT(e.episode_id), SUM(CASE WHEN e.body_status = 'complete' THEN 1 ELSE 0 END),
			w.fetch_status, w.last_fetch_error, w.last_failed_episode_id, w.resume_episode_id, w.expected_episode_count
		FROM works w
		LEFT JOIN episodes e ON e.work_id = w.id
		WHERE w.id = ?
		GROUP BY w.id
	`, id)
	work, err := scanStoredWork(row)
	if errors.Is(err, sql.ErrNoRows) {
		return model.StoredWork{}, false, nil
	}
	if err != nil {
		return model.StoredWork{}, false, err
	}
	return work, true, nil
}

func normalizeTitleForMatch(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func similarTitleMatch(a string, b string) bool {
	return titleSimilarityScore(a, b) >= strongTitleSimilarityThreshold
}

func titleSimilarityScore(a string, b string) float64 {
	if a == "" || b == "" {
		return 0
	}
	if a == b {
		return 1
	}

	aRunes := []rune(a)
	bRunes := []rune(b)
	if min(len(aRunes), len(bRunes)) < 12 {
		return 0
	}

	return ngramDiceCoefficient(aRunes, bRunes, 3)
}

func similarLeadingEpisodeTitles(incoming []model.Episode, stored []model.StoredEpisode) bool {
	limit := min(5, len(incoming), len(stored))
	if limit < 3 {
		return false
	}

	matches := 0
	for index := 0; index < limit; index++ {
		if similarTitleMatch(normalizeTitleForMatch(incoming[index].Title), normalizeTitleForMatch(stored[index].Title)) {
			matches++
		}
	}
	return matches >= 3
}

func ngramDiceCoefficient(a []rune, b []rune, n int) float64 {
	if n <= 0 || len(a) < n || len(b) < n {
		return 0
	}

	aGrams := runeNgramCounts(a, n)
	bGramCount := len(b) - n + 1
	intersection := 0
	for index := 0; index <= len(b)-n; index++ {
		gram := string(b[index : index+n])
		count := aGrams[gram]
		if count == 0 {
			continue
		}
		intersection++
		aGrams[gram] = count - 1
	}

	return float64(2*intersection) / float64(len(a)-n+1+bGramCount)
}

func runeNgramCounts(value []rune, n int) map[string]int {
	counts := map[string]int{}
	for index := 0; index <= len(value)-n; index++ {
		counts[string(value[index:index+n])]++
	}
	return counts
}

func (s *Store) FindWorkBySiteKey(site string, siteWorkID string) (model.StoredWork, bool, error) {
	work, err := s.findWorkBySiteKey(site, siteWorkID)
	if errors.Is(err, sql.ErrNoRows) {
		return model.StoredWork{}, false, nil
	}
	if err != nil {
		return model.StoredWork{}, false, err
	}
	return work, true, nil
}

func (s *Store) findWorkBySiteKey(site string, siteWorkID string) (model.StoredWork, error) {
	row := s.db.QueryRow(`
		SELECT w.id, w.site, w.site_name, w.site_work_id, w.source_url, w.title, w.author, w.story, w.directory, w.fetched_at,
			COUNT(e.episode_id), SUM(CASE WHEN e.body_status = 'complete' THEN 1 ELSE 0 END),
			w.fetch_status, w.last_fetch_error, w.last_failed_episode_id, w.resume_episode_id, w.expected_episode_count
		FROM works w
		LEFT JOIN episodes e ON e.work_id = w.id
		WHERE w.site = ? AND w.site_work_id = ?
		GROUP BY w.id
	`, site, siteWorkID)
	return scanStoredWork(row)
}

func (s *Store) ListEpisodes(workID int) ([]model.StoredEpisode, error) {
	rows, err := s.db.Query(`
		SELECT work_id, episode_id, site_episode_id, source_url, sort_order, display_index, title, chapter, subchapter,
			published_at, updated_at, body_path, raw_path, content_hash, fetched_at, body_status, last_fetch_error
		FROM episodes
		WHERE work_id = ?
		ORDER BY sort_order ASC
	`, workID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	episodes := []model.StoredEpisode{}
	for rows.Next() {
		episode, err := scanStoredEpisode(rows)
		if err != nil {
			return nil, err
		}
		episodes = append(episodes, episode)
	}
	return episodes, rows.Err()
}

func (s *Store) FindEpisode(workID int, episodeID string) (model.StoredEpisode, bool, error) {
	row := s.db.QueryRow(`
		SELECT work_id, episode_id, site_episode_id, source_url, sort_order, display_index, title, chapter, subchapter,
			published_at, updated_at, body_path, raw_path, content_hash, fetched_at, body_status, last_fetch_error
		FROM episodes
		WHERE work_id = ? AND episode_id = ?
	`, workID, episodeID)
	episode, err := scanStoredEpisode(row)
	if errors.Is(err, sql.ErrNoRows) {
		return model.StoredEpisode{}, false, nil
	}
	if err != nil {
		return model.StoredEpisode{}, false, err
	}
	return episode, true, nil
}

func (s *Store) findEpisodeRequired(workID int, episodeID string) (model.StoredEpisode, error) {
	episode, ok, err := s.FindEpisode(workID, episodeID)
	if err != nil {
		return model.StoredEpisode{}, err
	}
	if !ok {
		return model.StoredEpisode{}, os.ErrNotExist
	}
	return episode, nil
}

func (s *Store) ReadCanonicalEpisode(episode model.StoredEpisode) (model.CanonicalEpisode, error) {
	path := filepath.Join(s.rootDir, episode.BodyPath)
	bytes, err := os.ReadFile(path)
	if err != nil {
		return model.CanonicalEpisode{}, err
	}
	if err := validateCanonicalEpisodeSchema(path, bytes); err != nil {
		return model.CanonicalEpisode{}, err
	}

	var document model.CanonicalEpisode
	if err := json.Unmarshal(bytes, &document); err != nil {
		return model.CanonicalEpisode{}, err
	}
	return document, nil
}

func validateCanonicalEpisodeSchema(path string, document []byte) error {
	var header struct {
		SchemaVersion *int `json:"schema_version"`
	}
	if err := json.Unmarshal(document, &header); err != nil {
		return fmt.Errorf("parse NF-CANONICAL-EPISODE schema header at %q: %w", path, err)
	}
	if header.SchemaVersion == nil || *header.SchemaVersion != canonicalEpisodeSchemaVersion {
		return ErrUnsupportedEpisodeSchema{
			Path:      path,
			Observed:  header.SchemaVersion,
			Supported: canonicalEpisodeSchemaVersion,
		}
	}
	return nil
}

func (s *Store) guardCanonicalEpisodeBeforeSave(workID int, episodeID string, targetRelativePath string) error {
	paths := []string{targetRelativePath}
	var storedRelativePath string
	err := s.db.QueryRow(`
		SELECT body_path
		FROM episodes
		WHERE work_id = ? AND episode_id = ?
	`, workID, episodeID).Scan(&storedRelativePath)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err == nil && storedRelativePath != "" && storedRelativePath != targetRelativePath {
		paths = append(paths, storedRelativePath)
	}
	return s.validateCanonicalEpisodeFiles(paths)
}

func (s *Store) validateCanonicalEpisodeFiles(relativePaths []string) error {
	for _, relativePath := range uniqueRelativePaths(relativePaths) {
		if strings.TrimSpace(relativePath) == "" {
			continue
		}
		path := filepath.Join(s.rootDir, relativePath)
		document, readErr := os.ReadFile(path)
		if errors.Is(readErr, os.ErrNotExist) {
			continue
		}
		if readErr != nil {
			return readErr
		}
		if err := validateCanonicalEpisodeSchema(path, document); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) PreflightWorkMutation(storedWork model.StoredWork, incomingWork model.Work) error {
	episodes := []model.StoredEpisode{}
	if storedWork.ID != 0 {
		var err error
		episodes, err = s.ListEpisodes(storedWork.ID)
		if err != nil {
			return err
		}
	}

	paths := make([]string, 0, len(episodes)+len(incomingWork.Episodes))
	for _, episode := range episodes {
		paths = append(paths, episode.BodyPath)
	}
	nextDirectory := filepath.ToSlash(filepath.Join("works", string(incomingWork.Site), sanitizePathSegment(incomingWork.SiteWorkID)))
	for index, episode := range incomingWork.Episodes {
		episodeID := canonicalEpisodeID(episode, index)
		paths = append(paths, filepath.ToSlash(filepath.Join(nextDirectory, "episodes", sanitizePathSegment(episodeID)+".json")))
	}
	return s.validateCanonicalEpisodeFiles(paths)
}

func (s *Store) UpsertWorkToc(ctx context.Context, work model.Work, status string) (stored model.StoredWork, err error) {
	if err := validateUniqueEpisodeIDs(work.Episodes); err != nil {
		return model.StoredWork{}, err
	}
	existing, ok, err := s.FindWorkBySiteKey(string(work.Site), work.SiteWorkID)
	if err != nil {
		return model.StoredWork{}, err
	}
	if !ok {
		existing = model.StoredWork{}
	}
	if err := s.PreflightWorkMutation(existing, work); err != nil {
		return model.StoredWork{}, err
	}
	if err := os.MkdirAll(s.rootDir, 0o755); err != nil {
		return model.StoredWork{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.StoredWork{}, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	fetchedAt := formatTime(work.FetchedAt)
	if fetchedAt == "" {
		fetchedAt = now
	}
	directory := filepath.ToSlash(filepath.Join("works", string(work.Site), sanitizePathSegment(work.SiteWorkID)))
	existingID := 0
	row := tx.QueryRow(`SELECT id FROM works WHERE site = ? AND site_work_id = ?`, string(work.Site), work.SiteWorkID)
	if scanErr := row.Scan(&existingID); scanErr != nil && !errors.Is(scanErr, sql.ErrNoRows) {
		err = scanErr
		return model.StoredWork{}, err
	}

	if existingID == 0 {
		result, execErr := tx.Exec(`
			INSERT INTO works (
				site, site_name, site_work_id, source_url, title, author, story, directory, fetched_at,
				fetch_status, last_fetch_error, last_failed_episode_id, resume_episode_id, expected_episode_count,
				created_at, updated_at
			)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', '', '', ?, ?, ?)
		`, string(work.Site), work.SiteName, work.SiteWorkID, work.SourceURL, work.Title, work.Author, work.Story, directory, fetchedAt, status, len(work.Episodes), now, now)
		if execErr != nil {
			err = execErr
			return model.StoredWork{}, err
		}
		id64, idErr := result.LastInsertId()
		if idErr != nil {
			err = idErr
			return model.StoredWork{}, err
		}
		existingID = int(id64)
	} else {
		if _, execErr := tx.Exec(`
			UPDATE works
			SET site_name = ?, source_url = ?, title = ?, author = ?, story = ?, directory = ?, fetched_at = ?,
				fetch_status = ?, last_fetch_error = '', last_failed_episode_id = '', resume_episode_id = '',
				expected_episode_count = ?, updated_at = ?
			WHERE id = ?
		`, work.SiteName, work.SourceURL, work.Title, work.Author, work.Story, directory, fetchedAt, status, len(work.Episodes), now, existingID); execErr != nil {
			err = execErr
			return model.StoredWork{}, err
		}
	}

	workDir := filepath.Join(s.rootDir, directory)
	for _, relative := range []string{"episodes", filepath.Join("raw", "episodes"), filepath.Join("assets", "episodes")} {
		if mkdirErr := os.MkdirAll(filepath.Join(workDir, relative), 0o755); mkdirErr != nil {
			err = mkdirErr
			return model.StoredWork{}, err
		}
	}

	currentEpisodeIDs := map[string]bool{}
	for index, episode := range work.Episodes {
		episodeID := canonicalEpisodeID(episode, index)
		currentEpisodeIDs[episodeID] = true
		if _, execErr := tx.Exec(`
			INSERT INTO episodes (
				work_id, episode_id, site_episode_id, source_url, sort_order, display_index, title, chapter, subchapter,
				published_at, updated_at, body_path, raw_path, content_hash, fetched_at,
				body_status, last_fetch_error, last_attempted_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', '', '', '', ?, '', '')
			ON CONFLICT(work_id, episode_id) DO UPDATE SET
				site_episode_id = excluded.site_episode_id,
				source_url = excluded.source_url,
				sort_order = excluded.sort_order,
				display_index = excluded.display_index,
				title = excluded.title,
				chapter = excluded.chapter,
				subchapter = excluded.subchapter,
				published_at = excluded.published_at,
				updated_at = excluded.updated_at
		`,
			existingID,
			episodeID,
			episode.Index,
			episodeSourceURL(work.SourceURL, episode),
			index,
			episode.Index,
			episode.Title,
			episode.Chapter,
			episode.Subchapter,
			episode.PublishedAt,
			episode.ModifiedAt,
			BodyStatusPending,
		); execErr != nil {
			err = execErr
			return model.StoredWork{}, err
		}
	}

	staleFiles, staleErr := s.removeEpisodesMissingFromToc(tx, existingID, currentEpisodeIDs)
	if staleErr != nil {
		err = staleErr
		return model.StoredWork{}, err
	}

	if err = tx.Commit(); err != nil {
		return model.StoredWork{}, err
	}
	_ = s.removeRelativeFiles(staleFiles)

	return s.findWorkBySiteKey(string(work.Site), work.SiteWorkID)
}

func (s *Store) SaveEpisodeBody(ctx context.Context, work model.Work, storedWork model.StoredWork, episode model.Episode, sortOrder int) (stored model.StoredEpisode, err error) {
	return s.saveEpisodeBody(ctx, work, storedWork, episode, sortOrder, nil)
}

func (s *Store) SaveEpisodeBodyForTask(ctx context.Context, ref taskstate.TaskRef, work model.Work, storedWork model.StoredWork, episode model.Episode, sortOrder int, nextEpisodeID string) (model.StoredEpisode, error) {
	checkpoint := TaskEpisodeCheckpoint{
		Ref:           ref,
		WorkID:        storedWork.ID,
		EpisodeID:     canonicalEpisodeID(episode, sortOrder),
		SortOrder:     sortOrder,
		NextEpisodeID: nextEpisodeID,
	}
	return s.saveEpisodeBody(ctx, work, storedWork, episode, sortOrder, &checkpoint)
}

func (s *Store) saveEpisodeBody(ctx context.Context, work model.Work, storedWork model.StoredWork, episode model.Episode, sortOrder int, checkpoint *TaskEpisodeCheckpoint) (stored model.StoredEpisode, err error) {
	episodeID := canonicalEpisodeID(episode, sortOrder)
	bodyRelPath := filepath.ToSlash(filepath.Join(storedWork.Directory, "episodes", sanitizePathSegment(episodeID)+".json"))
	if err := s.guardCanonicalEpisodeBeforeSave(storedWork.ID, episodeID, bodyRelPath); err != nil {
		return model.StoredEpisode{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.StoredEpisode{}, err
	}
	fileRollbacks := []fileRollbackEntry{}
	writeTrackedFile := func(path string, bytes []byte) error {
		rollback, snapshotErr := snapshotFile(path)
		if snapshotErr != nil {
			return snapshotErr
		}
		fileRollbacks = append(fileRollbacks, rollback)
		return writeFileAtomic(path, bytes)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
			restoreFileRollbacks(fileRollbacks)
		}
	}()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	rawRelPath := ""
	if episode.RawHTML != "" {
		rawRelPath = filepath.ToSlash(filepath.Join(storedWork.Directory, "raw", "episodes", sanitizePathSegment(episodeID)+".html"))
		if writeErr := writeTrackedFile(filepath.Join(s.rootDir, rawRelPath), []byte(episode.RawHTML)); writeErr != nil {
			err = writeErr
			return model.StoredEpisode{}, err
		}
	}

	staleFiles, staleErr := s.collectFilesReplacedByEpisodeSave(tx, storedWork.ID, episodeID, bodyRelPath, rawRelPath)
	if staleErr != nil {
		err = staleErr
		return model.StoredEpisode{}, err
	}
	if _, execErr := tx.Exec(`DELETE FROM assets WHERE work_id = ? AND episode_id = ?`, storedWork.ID, episodeID); execErr != nil {
		err = execErr
		return model.StoredEpisode{}, err
	}
	episode.SourceURL = episodeSourceURL(work.SourceURL, episode)
	localizedEpisode, assets, localizeErr := s.assetMaterializer.LocalizeEpisodeAssets(ctx, storageassets.LocalizeInput{
		WorkDirectory: storedWork.Directory,
		WorkID:        storedWork.ID,
		EpisodeID:     episodeID,
		WorkURL:       work.SourceURL,
		Episode:       episode,
		WriteFile:     writeTrackedFile,
	})
	if localizeErr != nil {
		err = localizeErr
		return model.StoredEpisode{}, err
	}
	canonical := toCanonicalEpisode(localizedEpisode, episodeID, sortOrder)
	canonicalBytes, marshalErr := marshalCanonicalEpisode(canonical)
	if marshalErr != nil {
		err = marshalErr
		return model.StoredEpisode{}, err
	}
	contentHash := sha256Hex(canonicalBytes)
	if writeErr := writeTrackedFile(filepath.Join(s.rootDir, bodyRelPath), canonicalBytes); writeErr != nil {
		err = writeErr
		return model.StoredEpisode{}, err
	}

	if _, execErr := tx.Exec(`
		INSERT INTO episodes (
			work_id, episode_id, site_episode_id, source_url, sort_order, display_index, title, chapter, subchapter,
			published_at, updated_at, body_path, raw_path, content_hash, fetched_at,
			body_status, last_fetch_error, last_attempted_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', ?)
		ON CONFLICT(work_id, episode_id) DO UPDATE SET
			site_episode_id = excluded.site_episode_id,
			source_url = excluded.source_url,
			sort_order = excluded.sort_order,
			display_index = excluded.display_index,
			title = excluded.title,
			chapter = excluded.chapter,
			subchapter = excluded.subchapter,
			published_at = excluded.published_at,
			updated_at = excluded.updated_at,
			body_path = excluded.body_path,
			raw_path = excluded.raw_path,
			content_hash = excluded.content_hash,
			fetched_at = excluded.fetched_at,
			body_status = excluded.body_status,
			last_fetch_error = '',
			last_attempted_at = excluded.last_attempted_at
	`,
		storedWork.ID,
		episodeID,
		episode.Index,
		episode.SourceURL,
		sortOrder,
		episode.Index,
		episode.Title,
		episode.Chapter,
		episode.Subchapter,
		episode.PublishedAt,
		episode.ModifiedAt,
		bodyRelPath,
		rawRelPath,
		contentHash,
		formatTime(episode.FetchedAt),
		BodyStatusComplete,
		now,
	); execErr != nil {
		err = execErr
		return model.StoredEpisode{}, err
	}

	for _, asset := range assets {
		if _, execErr := tx.Exec(`
			INSERT INTO assets (
				asset_id, work_id, episode_id, source_url, storage_path, media_type,
				byte_length, width, height, content_hash, fetched_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`,
			asset.AssetID,
			storedWork.ID,
			episodeID,
			asset.SourceURL,
			asset.StoragePath,
			asset.MediaType,
			asset.ByteLength,
			asset.Width,
			asset.Height,
			asset.ContentHash,
			now,
		); execErr != nil {
			err = execErr
			return model.StoredEpisode{}, err
		}
	}
	staleFiles = excludeRelativePaths(staleFiles, assetStoragePaths(assets))
	if checkpoint != nil {
		completedAt := time.Now().UTC().Format(time.RFC3339Nano)
		if _, execErr := tx.ExecContext(ctx, `
			INSERT INTO fetch_task_episode_checkpoints (
				task_id, work_id, episode_id, sort_order, content_hash, completed_attempt, completed_at
			) VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(task_id, work_id, episode_id) DO UPDATE SET
				sort_order = excluded.sort_order,
				content_hash = excluded.content_hash,
				completed_attempt = excluded.completed_attempt,
				completed_at = excluded.completed_at
		`, checkpoint.Ref.TaskID, checkpoint.WorkID, checkpoint.EpisodeID, checkpoint.SortOrder, contentHash, checkpoint.Ref.Attempt, completedAt); execErr != nil {
			err = execErr
			return model.StoredEpisode{}, err
		}
		result, execErr := tx.ExecContext(ctx, `
			UPDATE fetch_tasks
			SET saved_episode_count = MAX(saved_episode_count, ?), current_step = MAX(current_step, ?), resume_episode_id = ?, updated_at = ?
			WHERE task_id = ? AND status = 'running' AND attempt_count = ?
		`, checkpoint.SortOrder+1, checkpoint.SortOrder+1, checkpoint.NextEpisodeID, completedAt, checkpoint.Ref.TaskID, checkpoint.Ref.Attempt)
		if execErr != nil {
			err = execErr
			return model.StoredEpisode{}, err
		}
		if err = requireTaskAttemptUpdate(result); err != nil {
			return model.StoredEpisode{}, err
		}
	}

	if err = tx.Commit(); err != nil {
		return model.StoredEpisode{}, err
	}
	_ = s.removeRelativeFiles(staleFiles)
	return s.findEpisodeRequired(storedWork.ID, episodeID)
}

func (s *Store) RecordTaskEpisodeCheckpoint(ctx context.Context, ref taskstate.TaskRef, workID int, episodeID string, sortOrder int, nextEpisodeID string) error {
	episode, found, err := s.FindEpisode(workID, episodeID)
	if err != nil {
		return err
	}
	if !found || episode.BodyStatus != BodyStatusComplete {
		return fmt.Errorf("%w: episode %s is not complete", ErrInvalidTaskEpisodeCheckpoint, episodeID)
	}
	valid, err := s.validateStoredEpisodeFile(episode)
	if err != nil {
		return err
	}
	if !valid {
		return fmt.Errorf("%w: episode %s canonical body is invalid", ErrInvalidTaskEpisodeCheckpoint, episodeID)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO fetch_task_episode_checkpoints (
			task_id, work_id, episode_id, sort_order, content_hash, completed_attempt, completed_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(task_id, work_id, episode_id) DO UPDATE SET
			sort_order = excluded.sort_order,
			content_hash = excluded.content_hash,
			completed_attempt = excluded.completed_attempt,
			completed_at = excluded.completed_at
	`, ref.TaskID, workID, episodeID, sortOrder, episode.ContentHash, ref.Attempt, now); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE fetch_tasks
		SET saved_episode_count = MAX(saved_episode_count, ?), current_step = MAX(current_step, ?), resume_episode_id = ?, updated_at = ?
		WHERE task_id = ? AND status = 'running' AND attempt_count = ?
	`, sortOrder+1, sortOrder+1, nextEpisodeID, now, ref.TaskID, ref.Attempt)
	if err != nil {
		return err
	}
	if err := requireTaskAttemptUpdate(result); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) IsTaskEpisodeCheckpointValid(ctx context.Context, ref taskstate.TaskRef, work model.Work, storedWork model.StoredWork, episodeRef model.Episode, sortOrder int) (bool, bool, error) {
	episodeID := canonicalEpisodeID(episodeRef, sortOrder)
	var checkpointHash string
	err := s.db.QueryRowContext(ctx, `
		SELECT c.content_hash
		FROM fetch_task_episode_checkpoints c
		JOIN fetch_tasks t ON t.task_id = c.task_id
		WHERE c.task_id = ? AND c.work_id = ? AND c.episode_id = ? AND c.completed_attempt <= ?
	`, ref.TaskID, storedWork.ID, episodeID, ref.Attempt).Scan(&checkpointHash)
	if errors.Is(err, sql.ErrNoRows) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	episode, found, err := s.FindEpisode(storedWork.ID, episodeID)
	if err != nil || !found || episode.BodyStatus != BodyStatusComplete || episode.ContentHash == "" || episode.ContentHash != checkpointHash {
		return false, true, err
	}
	document, valid, err := s.readStoredEpisodeFile(episode)
	if err != nil || !valid {
		return false, true, err
	}
	return document.EpisodeID == episodeID &&
		document.SiteEpisodeID == episodeRef.Index &&
		strings.TrimSpace(document.SourceURL) == episodeSourceURL(work.SourceURL, episodeRef) &&
		document.PublishedAt == episodeRef.PublishedAt &&
		document.UpdatedAt == episodeRef.ModifiedAt, true, nil
}

func (s *Store) validateStoredEpisodeFile(episode model.StoredEpisode) (bool, error) {
	_, valid, err := s.readStoredEpisodeFile(episode)
	return valid, err
}

func (s *Store) readStoredEpisodeFile(episode model.StoredEpisode) (model.CanonicalEpisode, bool, error) {
	path := filepath.Join(s.rootDir, episode.BodyPath)
	bytes, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return model.CanonicalEpisode{}, false, nil
		}
		return model.CanonicalEpisode{}, false, err
	}
	if err := validateCanonicalEpisodeSchema(path, bytes); err != nil {
		return model.CanonicalEpisode{}, false, err
	}
	if sha256Hex(bytes) != episode.ContentHash {
		return model.CanonicalEpisode{}, false, nil
	}
	var document model.CanonicalEpisode
	if err := json.Unmarshal(bytes, &document); err != nil {
		return model.CanonicalEpisode{}, false, err
	}
	return document, true, nil
}

func validateUniqueEpisodeIDs(episodes []model.Episode) error {
	seen := map[string]bool{}
	for index, episode := range episodes {
		episodeID := canonicalEpisodeID(episode, index)
		if seen[episodeID] {
			return fmt.Errorf("duplicate episode id in toc: %s", episodeID)
		}
		seen[episodeID] = true
	}
	return nil
}

func (s *Store) removeEpisodesMissingFromToc(tx *sql.Tx, workID int, currentEpisodeIDs map[string]bool) ([]string, error) {
	rows, err := tx.Query(`
		SELECT episode_id, body_path, raw_path
		FROM episodes
		WHERE work_id = ?
	`, workID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	staleEpisodeIDs := []string{}
	staleFiles := []string{}
	for rows.Next() {
		var episodeID string
		var bodyPath string
		var rawPath string
		if err := rows.Scan(&episodeID, &bodyPath, &rawPath); err != nil {
			return nil, err
		}
		if currentEpisodeIDs[episodeID] {
			continue
		}
		staleEpisodeIDs = append(staleEpisodeIDs, episodeID)
		staleFiles = appendRelativePath(staleFiles, bodyPath)
		staleFiles = appendRelativePath(staleFiles, rawPath)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for _, episodeID := range staleEpisodeIDs {
		assetPaths, err := collectAssetPaths(tx, workID, episodeID)
		if err != nil {
			return nil, err
		}
		staleFiles = append(staleFiles, assetPaths...)
		if _, err := tx.Exec(`DELETE FROM assets WHERE work_id = ? AND episode_id = ?`, workID, episodeID); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(`DELETE FROM episodes WHERE work_id = ? AND episode_id = ?`, workID, episodeID); err != nil {
			return nil, err
		}
	}
	return uniqueRelativePaths(staleFiles), nil
}

func (s *Store) collectFilesReplacedByEpisodeSave(tx *sql.Tx, workID int, episodeID string, nextBodyPath string, nextRawPath string) ([]string, error) {
	staleFiles, err := collectAssetPaths(tx, workID, episodeID)
	if err != nil {
		return nil, err
	}

	row := tx.QueryRow(`SELECT body_path, raw_path FROM episodes WHERE work_id = ? AND episode_id = ?`, workID, episodeID)
	var oldBodyPath string
	var oldRawPath string
	if err := row.Scan(&oldBodyPath, &oldRawPath); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return uniqueRelativePaths(staleFiles), nil
		}
		return nil, err
	}
	if oldBodyPath != "" && oldBodyPath != nextBodyPath {
		staleFiles = appendRelativePath(staleFiles, oldBodyPath)
	}
	if oldRawPath != "" && oldRawPath != nextRawPath {
		staleFiles = appendRelativePath(staleFiles, oldRawPath)
	}
	return uniqueRelativePaths(staleFiles), nil
}

func collectAssetPaths(tx *sql.Tx, workID int, episodeID string) ([]string, error) {
	rows, err := tx.Query(`SELECT storage_path FROM assets WHERE work_id = ? AND episode_id = ?`, workID, episodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	paths := []string{}
	for rows.Next() {
		var storagePath string
		if err := rows.Scan(&storagePath); err != nil {
			return nil, err
		}
		paths = appendRelativePath(paths, storagePath)
	}
	return paths, rows.Err()
}

func (s *Store) removeRelativeFiles(relativePaths []string) error {
	for _, relativePath := range uniqueRelativePaths(relativePaths) {
		if relativePath == "" {
			continue
		}
		if err := os.Remove(filepath.Join(s.rootDir, filepath.FromSlash(relativePath))); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func appendRelativePath(paths []string, path string) []string {
	if strings.TrimSpace(path) == "" {
		return paths
	}
	return append(paths, filepath.ToSlash(path))
}

func uniqueRelativePaths(paths []string) []string {
	seen := map[string]bool{}
	unique := []string{}
	for _, path := range paths {
		normalized := filepath.ToSlash(strings.TrimSpace(path))
		if normalized == "" || seen[normalized] {
			continue
		}
		seen[normalized] = true
		unique = append(unique, normalized)
	}
	return unique
}

func assetStoragePaths(assets []storageassets.Asset) []string {
	paths := make([]string, 0, len(assets))
	for _, asset := range assets {
		paths = appendRelativePath(paths, asset.StoragePath)
	}
	return paths
}

func excludeRelativePaths(paths []string, excluded []string) []string {
	if len(paths) == 0 || len(excluded) == 0 {
		return paths
	}

	excludedSet := map[string]bool{}
	for _, path := range excluded {
		normalized := filepath.ToSlash(strings.TrimSpace(path))
		if normalized != "" {
			excludedSet[normalized] = true
		}
	}

	retained := []string{}
	for _, path := range paths {
		normalized := filepath.ToSlash(strings.TrimSpace(path))
		if normalized == "" || excludedSet[normalized] {
			continue
		}
		retained = append(retained, normalized)
	}
	return uniqueRelativePaths(retained)
}

func (s *Store) MarkEpisodeFailed(ctx context.Context, workID int, episodeID string, fetchError error) error {
	message := ""
	if fetchError != nil {
		message = fetchError.Error()
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `
		UPDATE episodes
		SET body_status = CASE WHEN body_status = ? THEN body_status ELSE ? END,
			last_fetch_error = ?, last_attempted_at = ?
		WHERE work_id = ? AND episode_id = ?
	`, BodyStatusComplete, BodyStatusFailed, message, now, workID, episodeID)
	return err
}

func (s *Store) UpdateWorkFetchStatus(ctx context.Context, workID int, status string, failedEpisodeID string, resumeEpisodeID string, fetchError error) error {
	message := ""
	if fetchError != nil {
		message = fetchError.Error()
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE works
		SET fetch_status = ?, last_fetch_error = ?, last_failed_episode_id = ?, resume_episode_id = ?, updated_at = ?
		WHERE id = ?
	`, status, message, failedEpisodeID, resumeEpisodeID, time.Now().UTC().Format(time.RFC3339Nano), workID)
	return err
}

func (s *Store) CompleteWorkForTask(ctx context.Context, ref taskstate.TaskRef, workID int) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `
		UPDATE works
		SET fetch_status = ?, last_fetch_error = '', last_failed_episode_id = '', resume_episode_id = '', updated_at = ?
		WHERE id = ?
	`, FetchStatusComplete, now, workID); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE fetch_tasks
		SET execution_committed = 1, updated_at = ?
		WHERE task_id = ? AND status = 'running' AND attempt_count = ? AND requested_action = ''
	`, now, ref.TaskID, ref.Attempt)
	if err != nil {
		return err
	}
	if err := requireTaskAttemptUpdate(result); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) RemoveWork(id int, withFiles bool) error {
	work, ok, err := s.FindWorkByID(id)
	if err != nil {
		return err
	}
	if !ok {
		return os.ErrNotExist
	}
	if _, err := s.db.Exec(`DELETE FROM works WHERE id = ?`, id); err != nil {
		return err
	}
	if withFiles && work.Directory != "" {
		if err := os.RemoveAll(filepath.Join(s.rootDir, work.Directory)); err != nil {
			return err
		}
	}
	return s.Maintain(context.Background())
}

func (s *Store) Maintain(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `PRAGMA incremental_vacuum`); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `PRAGMA optimize`)
	return err
}

type rowScanner interface {
	Scan(dest ...any) error
}

type fileRollbackEntry struct {
	path    string
	exists  bool
	content []byte
}

func snapshotFile(path string) (fileRollbackEntry, error) {
	content, err := os.ReadFile(path)
	if err == nil {
		return fileRollbackEntry{path: path, exists: true, content: content}, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return fileRollbackEntry{path: path}, nil
	}
	return fileRollbackEntry{}, err
}

func restoreFileRollbacks(entries []fileRollbackEntry) {
	for index := len(entries) - 1; index >= 0; index-- {
		entry := entries[index]
		if entry.exists {
			_ = writeFileAtomic(entry.path, entry.content)
			continue
		}
		_ = os.Remove(entry.path)
	}
}

func scanStoredWork(scanner rowScanner) (model.StoredWork, error) {
	var work model.StoredWork
	var site string
	var fetchedAt string
	var savedEpisodeLen sql.NullInt64
	if err := scanner.Scan(
		&work.ID,
		&site,
		&work.SiteName,
		&work.SiteWorkID,
		&work.SourceURL,
		&work.Title,
		&work.Author,
		&work.Story,
		&work.Directory,
		&fetchedAt,
		&work.EpisodeLen,
		&savedEpisodeLen,
		&work.FetchStatus,
		&work.LastFetchError,
		&work.LastFailedEpisodeID,
		&work.ResumeEpisodeID,
		&work.ExpectedEpisodeLen,
	); err != nil {
		return model.StoredWork{}, err
	}
	work.Site = model.Site(site)
	work.FetchedAt = parseTime(fetchedAt)
	if savedEpisodeLen.Valid {
		work.SavedEpisodeLen = int(savedEpisodeLen.Int64)
	}
	if work.FetchStatus == "" {
		work.FetchStatus = FetchStatusComplete
	}
	if work.ExpectedEpisodeLen == 0 {
		work.ExpectedEpisodeLen = work.EpisodeLen
	}
	return work, nil
}

func scanStoredEpisode(scanner rowScanner) (model.StoredEpisode, error) {
	var episode model.StoredEpisode
	var fetchedAt string
	if err := scanner.Scan(
		&episode.WorkID,
		&episode.EpisodeID,
		&episode.SiteEpisodeID,
		&episode.SourceURL,
		&episode.SortOrder,
		&episode.DisplayIndex,
		&episode.Title,
		&episode.Chapter,
		&episode.Subchapter,
		&episode.PublishedAt,
		&episode.UpdatedAt,
		&episode.BodyPath,
		&episode.RawPath,
		&episode.ContentHash,
		&fetchedAt,
		&episode.BodyStatus,
		&episode.LastFetchError,
	); err != nil {
		return model.StoredEpisode{}, err
	}
	episode.FetchedAt = parseTime(fetchedAt)
	if episode.BodyStatus == "" {
		episode.BodyStatus = BodyStatusComplete
	}
	return episode, nil
}

func episodeSourceURL(workURL string, episode model.Episode) string {
	if strings.TrimSpace(episode.SourceURL) != "" {
		return strings.TrimSpace(episode.SourceURL)
	}
	return resolveURL(workURL, episode.Href)
}

func toCanonicalEpisode(episode model.Episode, episodeID string, sortOrder int) model.CanonicalEpisode {
	blocks := []model.BodyBlock{}
	if episode.Chapter != "" {
		blocks = append(blocks, model.BodyBlock{Type: "meta", Section: "chapter", Text: episode.Chapter})
	}
	if episode.Subchapter != "" {
		blocks = append(blocks, model.BodyBlock{Type: "meta", Section: "subchapter", Text: episode.Subchapter})
	}
	blocks = append(blocks, model.BodyBlock{Type: "title", Text: episode.Title})
	appendHTML := func(section string, html string) {
		if strings.TrimSpace(html) != "" {
			blocks = append(blocks, model.BodyBlock{Type: "html", Section: section, HTML: html})
		}
	}
	appendHTML("introduction", episode.Element.Introduction)
	appendHTML("body", episode.Element.Body)
	appendHTML("postscript", episode.Element.Postscript)

	return model.CanonicalEpisode{
		SchemaVersion: canonicalEpisodeSchemaVersion,
		EpisodeID:     episodeID,
		SiteEpisodeID: episode.Index,
		SourceURL:     episode.SourceURL,
		SortOrder:     sortOrder,
		DisplayIndex:  episode.Index,
		Title:         episode.Title,
		Chapter:       episode.Chapter,
		Subchapter:    episode.Subchapter,
		PublishedAt:   episode.PublishedAt,
		UpdatedAt:     episode.ModifiedAt,
		Blocks:        blocks,
		FetchedAt:     episode.FetchedAt,
	}
}

func marshalCanonicalEpisode(episode model.CanonicalEpisode) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(episode); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func canonicalEpisodeID(episode model.Episode, fallback int) string {
	if episode.Index != "" {
		return episode.Index
	}
	return strconv.Itoa(fallback + 1)
}

func resolveURL(base string, href string) string {
	if strings.TrimSpace(href) == "" {
		return href
	}

	baseURL, err := url.Parse(base)
	if err != nil {
		return href
	}
	ref, err := url.Parse(href)
	if err != nil {
		return href
	}
	return baseURL.ResolveReference(ref).String()
}

func sanitizePathSegment(value string) string {
	return pathutil.Segment(value)
}

func sha256Hex(bytes []byte) string {
	sum := sha256.Sum256(bytes)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func requireTaskAttemptUpdate(result sql.Result) error {
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return taskstate.ErrStaleTaskAttempt
	}
	return nil
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return time.Now().UTC().Format(time.RFC3339Nano)
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func parseTime(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

func writeFileAtomic(path string, bytes []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	tempPath := filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+"."+strconv.FormatInt(time.Now().UnixNano(), 10)+".tmp")
	if err := os.WriteFile(tempPath, bytes, 0o644); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}
