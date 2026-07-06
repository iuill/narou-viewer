package sites

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"

	"narou-viewer/services/novel-fetcher/internal/fetcher"
	"narou-viewer/services/novel-fetcher/internal/model"
)

var kakuyomuWorkPathPattern = regexp.MustCompile(`^/works/(\d+)(?:/.*)?$`)

type KakuyomuFetcher struct {
	httpFetcher textFetcher
	policy      fetcher.FetchPolicy
	logger      *slog.Logger
}

func NewKakuyomuFetcher(
	httpFetcher *fetcher.HTTPFetcher,
	policy fetcher.FetchPolicy,
	logger *slog.Logger,
) *KakuyomuFetcher {
	return &KakuyomuFetcher{
		httpFetcher: httpFetcher,
		policy:      policy,
		logger:      logger,
	}
}

func (f *KakuyomuFetcher) FetchToc(ctx context.Context, target string, report ProgressReporter) (model.Work, error) {
	workID, err := resolveKakuyomuWorkID(target)
	if err != nil {
		return model.Work{}, err
	}

	tocURL := fmt.Sprintf("https://kakuyomu.jp/works/%s", workID)
	reportProgress(report, Progress{
		Phase:   "toc",
		Message: "カクヨムの目次を取得しています",
	})
	rawTocHTML, err := f.httpFetcher.FetchText(ctx, tocURL, f.policy)
	if err != nil {
		return model.Work{}, err
	}

	work, err := parseKakuyomuToc(tocURL, workID, rawTocHTML)
	if err != nil {
		return model.Work{}, err
	}

	work.FetchedAt = time.Now()
	return work, nil
}

func (f *KakuyomuFetcher) FetchEpisode(ctx context.Context, work model.Work, episode model.Episode, report ProgressReporter) (model.Episode, error) {
	if work.Site != model.SiteKakuyomu {
		return model.Episode{}, fmt.Errorf("%w: %s", ErrUnsupportedSite, work.Site)
	}
	now := time.Now()
	return f.fetchEpisodeAt(ctx, work, episode, 0, 1, now, report)
}

func (f *KakuyomuFetcher) fetchEpisodeAt(ctx context.Context, work model.Work, episode model.Episode, index int, totalEpisodes int, now time.Time, report ProgressReporter) (model.Episode, error) {
	if err := ctx.Err(); err != nil {
		return model.Episode{}, err
	}

	reportProgress(report, Progress{
		Phase:       "episode",
		CurrentStep: index + 1,
		TotalSteps:  totalEpisodes,
		Message:     fmt.Sprintf("%d / %d 話を取得中: %s", index+1, totalEpisodes, episode.Title),
	})
	episodeURL := resolveURL(work.SourceURL, episode.Href)
	rawHTML, err := f.httpFetcher.FetchText(ctx, episodeURL, f.policy)
	if err != nil {
		return model.Episode{}, err
	}
	element, err := parseKakuyomuEpisode(rawHTML)
	if err != nil {
		return model.Episode{}, fmt.Errorf("%s: %w", episodeURL, err)
	}
	episode.Element = element
	episode.RawHTML = rawHTML
	episode.FetchedAt = now
	return episode, nil
}

func resolveKakuyomuWorkID(target string) (string, error) {
	trimmed := strings.TrimSpace(target)
	if trimmed == "" {
		return "", fmt.Errorf("target is empty")
	}

	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Host == "" {
		return "", fmt.Errorf("%w: target is not a kakuyomu URL", ErrUnsupportedSite)
	}
	if parsed.Host != "kakuyomu.jp" {
		return "", fmt.Errorf("%w: %s", ErrUnsupportedSite, parsed.Host)
	}
	match := kakuyomuWorkPathPattern.FindStringSubmatch(parsed.Path)
	if len(match) < 2 {
		return "", fmt.Errorf("target is not a kakuyomu work URL")
	}
	return match[1], nil
}

func parseKakuyomuToc(tocURL string, workID string, html string) (model.Work, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return model.Work{}, err
	}

	nextData, err := extractKakuyomuNextData(doc)
	if err != nil {
		return model.Work{}, err
	}
	if nextData.Query.WorkID != "" {
		workID = nextData.Query.WorkID
	}

	workData := mapValue(nextData.Props.PageProps.ApolloState, "Work:"+workID)
	if workData == nil {
		return model.Work{}, fmt.Errorf("kakuyomu work data was not found: %s", workID)
	}

	work := model.Work{
		Site:       model.SiteKakuyomu,
		SiteName:   "カクヨム",
		SiteWorkID: workID,
		SourceURL:  tocURL,
		Title:      stringValue(workData["title"]),
		Author:     kakuyomuAuthor(workData, nextData.Props.PageProps.ApolloState),
		Story:      strings.ReplaceAll(stringValue(workData["introduction"]), "\n", "<br>"),
	}
	if work.Title == "" {
		work.Title = workID
	}

	work.Episodes = parseKakuyomuEpisodes(workID, workData, nextData.Props.PageProps.ApolloState)
	if len(work.Episodes) == 0 {
		return model.Work{}, fmt.Errorf("no episodes found in kakuyomu toc")
	}
	for index := range work.Episodes {
		work.Episodes[index].SourceURL = resolveURL(work.SourceURL, work.Episodes[index].Href)
	}
	return work, nil
}

type kakuyomuNextData struct {
	Props struct {
		PageProps struct {
			ApolloState map[string]any `json:"__APOLLO_STATE__"`
		} `json:"pageProps"`
	} `json:"props"`
	Query struct {
		WorkID string `json:"workId"`
	} `json:"query"`
}

func extractKakuyomuNextData(doc *goquery.Document) (kakuyomuNextData, error) {
	var data kakuyomuNextData
	jsonText := strings.TrimSpace(doc.Find(`script#__NEXT_DATA__[type="application/json"]`).First().Text())
	if jsonText == "" {
		jsonText = strings.TrimSpace(doc.Find("script#__NEXT_DATA__").First().Text())
	}
	if jsonText == "" {
		return data, fmt.Errorf("kakuyomu __NEXT_DATA__ was not found")
	}
	decoder := json.NewDecoder(strings.NewReader(jsonText))
	decoder.UseNumber()
	if err := decoder.Decode(&data); err != nil {
		return data, fmt.Errorf("kakuyomu __NEXT_DATA__ parse failed: %w", err)
	}
	if len(data.Props.PageProps.ApolloState) == 0 {
		return data, fmt.Errorf("kakuyomu Apollo state was not found")
	}
	return data, nil
}

func kakuyomuAuthor(workData map[string]any, state map[string]any) string {
	author := ""
	if authorRef := refValue(workData["author"]); authorRef != "" {
		author = stringValue(mapValue(state, authorRef)["activityName"])
	}
	alternate := stringValue(workData["alternateAuthorName"])
	if alternate != "" && author != "" {
		return alternate + "／" + author
	}
	if alternate != "" {
		return alternate
	}
	return author
}

func parseKakuyomuEpisodes(workID string, workData map[string]any, state map[string]any) []model.Episode {
	tocItems := sliceValue(workData["tableOfContentsV2"])
	if len(tocItems) == 0 {
		tocItems = sliceValue(workData["tableOfContents"])
	}

	episodes := []model.Episode{}
	currentChapter := ""
	currentSubchapter := ""

	emitChapter := func(chapter map[string]any) {
		switch intValue(chapter["level"]) {
		case 1:
			currentChapter = stringValue(chapter["title"])
			currentSubchapter = ""
		case 2:
			currentSubchapter = stringValue(chapter["title"])
		}
	}
	emitEpisode := func(episode map[string]any) {
		id := stringValue(episode["id"])
		title := stringValue(episode["title"])
		if id == "" || title == "" {
			return
		}
		episodeWorkID := stringValue(episode["workId"])
		if episodeWorkID == "" {
			episodeWorkID = workID
		}
		href := fmt.Sprintf("/works/%s/episodes/%s", episodeWorkID, id)
		modifiedAt := stringValue(episode["editedAt"])
		if modifiedAt == "" {
			modifiedAt = stringValue(episode["updatedAt"])
		}
		publishedAt := stringValue(episode["publishedAt"])
		if modifiedAt == "" {
			modifiedAt = publishedAt
		}
		episodes = append(episodes, model.Episode{
			Index:        id,
			Href:         href,
			Chapter:      currentChapter,
			Subchapter:   currentSubchapter,
			Title:        title,
			FileSubtitle: title,
			PublishedAt:  publishedAt,
			ModifiedAt:   modifiedAt,
			Element: model.EpisodeElement{
				DataType: "html",
			},
		})
	}

	for _, tocItem := range tocItems {
		item := mapValue(state, refValue(tocItem))
		if item == nil {
			item = mapAny(tocItem)
		}
		if item == nil {
			continue
		}

		if chapter := mapValue(state, refValue(item["chapter"])); chapter != nil {
			emitChapter(chapter)
		}

		for _, union := range sliceValue(item["episodeUnions"]) {
			entry := mapValue(state, refValue(union))
			if entry == nil {
				entry = mapAny(union)
			}
			switch stringValue(entry["__typename"]) {
			case "Chapter":
				emitChapter(entry)
			case "Episode":
				emitEpisode(entry)
			}
		}

		switch stringValue(item["__typename"]) {
		case "Chapter":
			emitChapter(item)
		case "Episode":
			emitEpisode(item)
		}
	}

	return episodes
}

func parseKakuyomuEpisode(html string) (model.EpisodeElement, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return model.EpisodeElement{}, err
	}
	body := firstHTML(doc, "div.widget-episodeBody.js-episode-body", "div.widget-episodeBody", "div.p-episode__body")
	if body == "" {
		return model.EpisodeElement{}, fmt.Errorf("kakuyomu episode body was not found")
	}
	return model.EpisodeElement{
		DataType: "html",
		Body:     body,
	}, nil
}

func mapValue(values map[string]any, key string) map[string]any {
	if values == nil || key == "" {
		return nil
	}
	return mapAny(values[key])
}

func mapAny(value any) map[string]any {
	if mapped, ok := value.(map[string]any); ok {
		return mapped
	}
	return nil
}

func sliceValue(value any) []any {
	if values, ok := value.([]any); ok {
		return values
	}
	return nil
}

func refValue(value any) string {
	return stringValue(mapAny(value)["__ref"])
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case float64:
		return fmt.Sprintf("%.0f", typed)
	case json.Number:
		return typed.String()
	default:
		return ""
	}
}

func intValue(value any) int {
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case json.Number:
		parsed, _ := typed.Int64()
		return int(parsed)
	case string:
		switch typed {
		case "1":
			return 1
		case "2":
			return 2
		}
	}
	return 0
}
