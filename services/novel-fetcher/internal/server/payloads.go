package server

import (
	"time"

	"narou-viewer/services/novel-fetcher/internal/model"
)

func workPayload(work model.StoredWork) map[string]any {
	return map[string]any{
		"id":                     work.ID,
		"site":                   string(work.Site),
		"site_name":              work.SiteName,
		"site_work_id":           work.SiteWorkID,
		"source_url":             work.SourceURL,
		"title":                  work.Title,
		"author":                 work.Author,
		"story":                  work.Story,
		"directory":              work.Directory,
		"fetched_at":             work.FetchedAt.Format(time.RFC3339Nano),
		"episode_count":          work.EpisodeLen,
		"saved_episode_count":    work.SavedEpisodeLen,
		"fetch_status":           work.FetchStatus,
		"last_fetch_error":       work.LastFetchError,
		"failed_episode_id":      work.LastFailedEpisodeID,
		"resume_episode_id":      work.ResumeEpisodeID,
		"expected_episode_count": work.ExpectedEpisodeLen,
	}
}

func episodeSummaryPayload(episode model.StoredEpisode) map[string]any {
	return map[string]any{
		"episode_id":       episode.EpisodeID,
		"site_episode_id":  episode.SiteEpisodeID,
		"source_url":       episode.SourceURL,
		"sort_order":       episode.SortOrder,
		"display_index":    episode.DisplayIndex,
		"title":            episode.Title,
		"chapter":          episode.Chapter,
		"subchapter":       episode.Subchapter,
		"published_at":     episode.PublishedAt,
		"updated_at":       episode.UpdatedAt,
		"content_hash":     episode.ContentHash,
		"fetched_at":       episode.FetchedAt.Format(time.RFC3339Nano),
		"body_status":      episode.BodyStatus,
		"last_fetch_error": episode.LastFetchError,
	}
}
