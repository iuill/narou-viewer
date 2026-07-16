package server

import (
	"strings"

	"narou-viewer/services/novel-fetcher/internal/taskstate"
)

func normalizeDownloadTargets(values []string) []string {
	normalized := []string{}
	seen := map[string]struct{}{}
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		key := normalizeDownloadTargetKey(trimmed)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, trimmed)
	}
	return normalized
}

func (a *App) existingDownloadNovelIDsByTarget(targets []string) (map[string][]int, error) {
	targetKeys := map[string]struct{}{}
	for _, target := range targets {
		if key := normalizeDownloadTargetKey(target); key != "" {
			targetKeys[key] = struct{}{}
		}
	}

	matches := map[string][]int{}
	if len(targetKeys) == 0 {
		return matches, nil
	}

	unresolvedSourceKeys := map[string]struct{}{}
	for key := range targetKeys {
		if site, siteWorkID, ok := parseDownloadTargetSiteKey(key); ok {
			work, found, err := a.store.FindWorkBySiteKey(site, siteWorkID)
			if err != nil {
				return nil, err
			}
			if found {
				matches[key] = appendUniqueInt(matches[key], work.ID)
			}
			continue
		}
		unresolvedSourceKeys[key] = struct{}{}
	}

	if len(unresolvedSourceKeys) == 0 {
		return matches, nil
	}

	works, err := a.store.ListWorks()
	if err != nil {
		return nil, err
	}
	for _, work := range works {
		key := normalizeDownloadTargetKey(work.SourceURL)
		if _, ok := unresolvedSourceKeys[key]; ok {
			matches[key] = appendUniqueInt(matches[key], work.ID)
		}
	}
	return matches, nil
}

func normalizeDownloadTargetKey(value string) string {
	return taskstate.CanonicalTarget(value)
}

func parseDownloadTargetSiteKey(key string) (string, string, bool) {
	const prefix = "site:"
	siteKey := strings.TrimPrefix(key, prefix)
	if siteKey == key {
		return "", "", false
	}
	parts := strings.SplitN(siteKey, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func appendUniqueInt(values []int, next int) []int {
	if next == 0 {
		return values
	}
	for _, value := range values {
		if value == next {
			return values
		}
	}
	return append(values, next)
}
