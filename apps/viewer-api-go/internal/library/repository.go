package library

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"

	"narou-viewer/apps/viewer-api-go/internal/fetcher"
)

func (s *Service) ListWorks() ([]Work, error) {
	return s.ListWorksContext(context.Background())
}

func (s *Service) ListWorksContext(ctx context.Context) ([]Work, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.listWorks(ctx)
}

func (s *Service) listWorks(ctx context.Context) ([]Work, error) {
	if s != nil && s.fetcherReader != nil {
		works, err := s.fetcherReader.ListLibraryWorks(ctx)
		if err != nil {
			return nil, err
		}
		result := make([]Work, 0, len(works))
		for _, work := range works {
			result = append(result, workFromFetcher(work))
		}
		return result, nil
	}
	db, err := s.ensureDB()
	if err != nil {
		return nil, err
	}
	if db == nil {
		return []Work{}, nil
	}
	rows, err := db.QueryContext(ctx, workSelectSQL()+`
		LEFT JOIN episodes e ON e.work_id = w.id
		GROUP BY w.id
		ORDER BY w.fetched_at DESC, w.title ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	works := []Work{}
	for rows.Next() {
		work, err := scanWork(rows)
		if err != nil {
			return nil, err
		}
		works = append(works, work)
	}
	return works, rows.Err()
}

func (s *Service) FindWork(novelID string) (Work, bool, error) {
	return s.findWork(context.Background(), novelID)
}

func (s *Service) findWork(ctx context.Context, novelID string) (Work, bool, error) {
	parsed := decodeNovelID(novelID)
	switch parsed.Kind {
	case novelIDPrefixSite:
		site, siteWorkID, ok := parseSiteNovelIDPayload(parsed.Value)
		if !ok {
			return Work{}, false, nil
		}
		return s.findWorkBySiteWorkID(ctx, site, siteWorkID)
	case novelIDPrefixFetcher:
		id, err := strconv.Atoi(strings.TrimSpace(parsed.Value))
		if err != nil || id <= 0 {
			return Work{}, false, nil
		}
		return s.findWorkByID(ctx, id)
	default:
		return Work{}, false, nil
	}
}

func (s *Service) NovelExists(novelID string) (bool, error) {
	return s.novelExists(context.Background(), novelID)
}

func (s *Service) EpisodeExists(ctx context.Context, novelID string, episodeIndex string) (bool, bool, error) {
	work, found, err := s.findWork(ctx, novelID)
	if err != nil || !found {
		return found, false, err
	}
	_, found, err = s.findEpisode(ctx, work.ID, episodeIndex)
	return true, found, err
}

func (s *Service) novelExists(ctx context.Context, novelID string) (bool, error) {
	parsed := decodeNovelID(novelID)
	switch parsed.Kind {
	case novelIDPrefixSite:
		site, siteWorkID, ok := parseSiteNovelIDPayload(parsed.Value)
		if !ok {
			return false, nil
		}
		if s != nil && s.fetcherReader != nil {
			_, ok, err := s.findWorkBySiteWorkID(ctx, site, siteWorkID)
			return ok, err
		}
		return s.queryWorkExists(`LOWER(w.site) = LOWER(?) AND w.site_work_id = ?`, site, siteWorkID)
	case novelIDPrefixFetcher:
		id, err := strconv.Atoi(strings.TrimSpace(parsed.Value))
		if err != nil || id <= 0 {
			return false, nil
		}
		if s != nil && s.fetcherReader != nil {
			_, ok, err := s.findWorkByID(ctx, id)
			return ok, err
		}
		return s.queryWorkExists(`w.id = ?`, id)
	default:
		return false, nil
	}
}

func (s *Service) findWorkByID(ctx context.Context, workID int) (Work, bool, error) {
	if s != nil && s.fetcherReader != nil {
		works, err := s.listWorks(ctx)
		if err != nil {
			return Work{}, false, err
		}
		for _, work := range works {
			if work.ID == workID {
				return work, true, nil
			}
		}
		return Work{}, false, nil
	}
	return s.queryWork(`w.id = ?`, workID)
}

func (s *Service) findWorkBySiteWorkID(ctx context.Context, site string, siteWorkID string) (Work, bool, error) {
	if s != nil && s.fetcherReader != nil {
		works, err := s.listWorks(ctx)
		if err != nil {
			return Work{}, false, err
		}
		for _, work := range works {
			if strings.EqualFold(work.Site, site) && work.SiteWorkID == siteWorkID {
				return work, true, nil
			}
		}
		return Work{}, false, nil
	}
	return s.queryWork(`LOWER(w.site) = LOWER(?) AND w.site_work_id = ?`, site, siteWorkID)
}

func (s *Service) queryWork(where string, args ...any) (Work, bool, error) {
	db, err := s.ensureDB()
	if err != nil {
		return Work{}, false, err
	}
	if db == nil {
		return Work{}, false, nil
	}
	row := db.QueryRow(workSelectSQL()+`
		LEFT JOIN episodes e ON e.work_id = w.id
		WHERE `+where+`
		GROUP BY w.id
	`, args...)
	work, err := scanWork(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Work{}, false, nil
	}
	if err != nil {
		return Work{}, false, err
	}
	return work, true, nil
}

func (s *Service) queryWorkExists(where string, args ...any) (bool, error) {
	db, err := s.ensureDB()
	if err != nil {
		return false, err
	}
	if db == nil {
		return false, nil
	}
	var exists int
	err = db.QueryRow(`SELECT 1 FROM works w WHERE `+where+` LIMIT 1`, args...).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *Service) ListEpisodes(workID int) ([]Episode, error) {
	return s.listEpisodes(context.Background(), workID)
}

func (s *Service) listEpisodes(ctx context.Context, workID int) ([]Episode, error) {
	if s != nil && s.fetcherReader != nil {
		_, episodes, err := s.fetcherReader.GetLibraryToc(ctx, workID)
		if err != nil {
			if isFetcherNotFound(err) {
				return []Episode{}, nil
			}
			return nil, err
		}
		result := make([]Episode, 0, len(episodes))
		for _, episode := range episodes {
			result = append(result, episodeFromFetcher(workID, episode))
		}
		return result, nil
	}
	db, err := s.ensureDB()
	if err != nil {
		return nil, err
	}
	if db == nil {
		return []Episode{}, nil
	}
	rows, err := db.Query(`
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

	episodes := []Episode{}
	for rows.Next() {
		episode, err := scanEpisode(rows)
		if err != nil {
			return nil, err
		}
		episodes = append(episodes, episode)
	}
	return episodes, rows.Err()
}

func (s *Service) FindEpisode(workID int, episodeIndex string) (Episode, bool, error) {
	return s.findEpisode(context.Background(), workID, episodeIndex)
}

func (s *Service) findEpisode(ctx context.Context, workID int, episodeIndex string) (Episode, bool, error) {
	if s != nil && s.fetcherReader != nil {
		episodes, err := s.listEpisodes(ctx, workID)
		if err != nil {
			return Episode{}, false, err
		}
		for _, episode := range episodes {
			if episode.EpisodeID == episodeIndex || episode.DisplayIndex == episodeIndex || episode.SiteEpisodeID == episodeIndex {
				return episode, true, nil
			}
		}
		return Episode{}, false, nil
	}
	db, err := s.ensureDB()
	if err != nil {
		return Episode{}, false, err
	}
	if db == nil {
		return Episode{}, false, nil
	}
	row := db.QueryRow(`
		SELECT work_id, episode_id, site_episode_id, source_url, sort_order, display_index, title, chapter, subchapter,
			published_at, updated_at, body_path, raw_path, content_hash, fetched_at, body_status, last_fetch_error
		FROM episodes
		WHERE work_id = ? AND (episode_id = ? OR display_index = ? OR site_episode_id = ?)
	`, workID, episodeIndex, episodeIndex, episodeIndex)
	episode, err := scanEpisode(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Episode{}, false, nil
	}
	if err != nil {
		return Episode{}, false, err
	}
	return episode, true, nil
}

func workSelectSQL() string {
	return `
		SELECT w.id, w.site, w.site_name, w.site_work_id, w.source_url, w.title, w.author, w.story, w.directory, w.fetched_at,
			COUNT(e.episode_id), COALESCE(SUM(CASE WHEN e.body_status = 'complete' THEN 1 ELSE 0 END), 0),
			w.fetch_status, w.last_fetch_error, w.last_failed_episode_id, w.resume_episode_id, w.expected_episode_count
		FROM works w`
}

type scanner interface {
	Scan(dest ...any) error
}

func scanWork(row scanner) (Work, error) {
	var work Work
	err := row.Scan(
		&work.ID, &work.Site, &work.SiteName, &work.SiteWorkID, &work.SourceURL,
		&work.Title, &work.Author, &work.Story, &work.Directory, &work.FetchedAt,
		&work.EpisodeLen, &work.SavedEpisodeLen, &work.FetchStatus, &work.LastFetchError,
		&work.LastFailedEpisodeID, &work.ResumeEpisodeID, &work.ExpectedEpisodeLen,
	)
	return work, err
}

func scanEpisode(row scanner) (Episode, error) {
	var episode Episode
	err := row.Scan(
		&episode.WorkID, &episode.EpisodeID, &episode.SiteEpisodeID, &episode.SourceURL,
		&episode.SortOrder, &episode.DisplayIndex, &episode.Title, &episode.Chapter,
		&episode.Subchapter, &episode.PublishedAt, &episode.UpdatedAt, &episode.BodyPath,
		&episode.RawPath, &episode.ContentHash, &episode.FetchedAt, &episode.BodyStatus,
		&episode.LastFetchError,
	)
	return episode, err
}

func workFromFetcher(work fetcher.LibraryWork) Work {
	return Work{
		ID:                  work.ID,
		Site:                work.Site,
		SiteName:            work.SiteName,
		SiteWorkID:          work.SiteWorkID,
		SourceURL:           work.SourceURL,
		Title:               work.Title,
		Author:              work.Author,
		Story:               work.Story,
		Directory:           work.Directory,
		FetchedAt:           work.FetchedAt,
		EpisodeLen:          work.EpisodeLen,
		SavedEpisodeLen:     work.SavedEpisodeLen,
		FetchStatus:         work.FetchStatus,
		LastFetchError:      work.LastFetchError,
		LastFailedEpisodeID: work.LastFailedEpisodeID,
		ResumeEpisodeID:     work.ResumeEpisodeID,
		ExpectedEpisodeLen:  work.ExpectedEpisodeLen,
	}
}

func episodeFromFetcher(workID int, episode fetcher.LibraryEpisode) Episode {
	return Episode{
		WorkID:         workID,
		EpisodeID:      episode.EpisodeID,
		SiteEpisodeID:  episode.SiteEpisodeID,
		SourceURL:      episode.SourceURL,
		SortOrder:      episode.SortOrder,
		DisplayIndex:   episode.DisplayIndex,
		Title:          episode.Title,
		Chapter:        episode.Chapter,
		Subchapter:     episode.Subchapter,
		PublishedAt:    episode.PublishedAt,
		UpdatedAt:      episode.UpdatedAt,
		ContentHash:    episode.ContentHash,
		FetchedAt:      episode.FetchedAt,
		BodyStatus:     episode.BodyStatus,
		LastFetchError: episode.LastFetchError,
	}
}

func isFetcherNotFound(err error) bool {
	var httpError *fetcher.HTTPError
	return errors.As(err, &httpError) && httpError.StatusCode == 404
}
