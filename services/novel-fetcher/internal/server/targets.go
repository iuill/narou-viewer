package server

import (
	"net/url"
	"regexp"
	"strings"

	"narou-viewer/services/novel-fetcher/internal/model"
)

var (
	downloadTargetNcodePattern       = regexp.MustCompile(`(?i)n\d+[a-z]+`)
	downloadTargetKakuyomuPathRegexp = regexp.MustCompile(`^/works/(\d+)(?:/.*)?$`)
)

func normalizeStrings(values []string) []string {
	normalized := []string{}
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			normalized = append(normalized, trimmed)
		}
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
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}

	if parsed, err := url.Parse(trimmed); err == nil && parsed.Host != "" {
		host := strings.ToLower(parsed.Host)
		if host == "ncode.syosetu.com" {
			if match := downloadTargetNcodePattern.FindString(parsed.Path); match != "" {
				return downloadTargetSiteKey(string(model.SiteSyosetu), strings.ToLower(match))
			}
		}
		if host == "kakuyomu.jp" {
			if match := downloadTargetKakuyomuPathRegexp.FindStringSubmatch(parsed.Path); len(match) >= 2 {
				return downloadTargetSiteKey(string(model.SiteKakuyomu), match[1])
			}
		}
		parsed.Scheme = "https"
		parsed.Host = host
		parsed.Path = strings.TrimRight(parsed.Path, "/")
		parsed.RawQuery = ""
		parsed.Fragment = ""
		return "url:" + strings.ToLower(parsed.String())
	}

	if match := downloadTargetNcodePattern.FindString(trimmed); match != "" {
		return downloadTargetSiteKey(string(model.SiteSyosetu), strings.ToLower(match))
	}
	return "url:" + strings.ToLower(strings.TrimRight(trimmed, "/"))
}

func downloadTargetSiteKey(site string, siteWorkID string) string {
	return "site:" + site + ":" + strings.ToLower(siteWorkID)
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
