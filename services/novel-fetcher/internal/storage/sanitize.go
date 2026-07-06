package storage

import "narou-viewer/services/novel-fetcher/internal/storage/pathutil"

func sanitizeFilename(value string) string {
	return pathutil.Filename(value)
}

func truncateRunes(value string, limit int) string {
	return pathutil.TruncateRunes(value, limit)
}
