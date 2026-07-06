package library

import (
	"encoding/base64"
	"strconv"
	"strings"
)

const (
	novelIDPrefixSite    = "site:"
	novelIDPrefixFetcher = "novel-fetcher:"
)

func NovelID(work Work) string {
	if siteID := canonicalSiteNovelID(work); siteID != "" {
		return encodeNovelID(novelIDPrefixSite + siteID)
	}
	return encodeNovelID(novelIDPrefixFetcher + strconv.Itoa(work.ID))
}

type decodedNovelID struct {
	Kind  string
	Value string
}

func decodeNovelID(novelID string) decodedNovelID {
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(novelID))
	if err != nil {
		return decodedNovelID{}
	}
	value := string(raw)
	for _, prefix := range []string{novelIDPrefixSite, novelIDPrefixFetcher} {
		if strings.HasPrefix(value, prefix) {
			return decodedNovelID{Kind: prefix, Value: strings.TrimSpace(strings.TrimPrefix(value, prefix))}
		}
	}
	return decodedNovelID{}
}

func canonicalSiteNovelID(work Work) string {
	site := strings.ToLower(strings.TrimSpace(work.Site))
	siteWorkID := strings.TrimSpace(work.SiteWorkID)
	if site == "" || siteWorkID == "" {
		return ""
	}
	return site + ":" + siteWorkID
}

func parseSiteNovelIDPayload(value string) (string, string, bool) {
	site, siteWorkID, ok := strings.Cut(value, ":")
	site = strings.TrimSpace(site)
	siteWorkID = strings.TrimSpace(siteWorkID)
	return site, siteWorkID, ok && site != "" && siteWorkID != ""
}

func encodeNovelID(value string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}
