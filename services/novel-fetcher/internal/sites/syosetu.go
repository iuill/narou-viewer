package sites

import (
	"context"
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

var ncodePattern = regexp.MustCompile(`(?i)n\d+[a-z]+`)

type SyosetuFetcher struct {
	httpFetcher textFetcher
	policy      fetcher.FetchPolicy
	maxTocPages int
	logger      *slog.Logger
}

func NewSyosetuFetcher(
	httpFetcher *fetcher.HTTPFetcher,
	policy fetcher.FetchPolicy,
	maxTocPages int,
	logger *slog.Logger,
) *SyosetuFetcher {
	return &SyosetuFetcher{
		httpFetcher: httpFetcher,
		policy:      policy,
		maxTocPages: maxTocPages,
		logger:      logger,
	}
}

func (f *SyosetuFetcher) FetchToc(ctx context.Context, target string, report ProgressReporter) (model.Work, error) {
	ncode, err := resolveNCode(target)
	if err != nil {
		return model.Work{}, err
	}

	tocURL := fmt.Sprintf("https://ncode.syosetu.com/%s/", ncode)
	reportProgress(report, Progress{
		Phase:   "toc",
		Message: "目次を取得しています",
	})
	tocPages, err := f.fetchTocPages(ctx, tocURL)
	if err != nil {
		return model.Work{}, err
	}

	work, err := parseSyosetuToc(tocURL, ncode, tocPages)
	if err != nil {
		return model.Work{}, err
	}

	work.FetchedAt = time.Now()
	return work, nil
}

func (f *SyosetuFetcher) FetchEpisode(ctx context.Context, work model.Work, episode model.Episode, report ProgressReporter) (model.Episode, error) {
	if work.Site != model.SiteSyosetu {
		return model.Episode{}, fmt.Errorf("%w: %s", ErrUnsupportedSite, work.Site)
	}
	now := time.Now()
	return f.fetchEpisodeAt(ctx, work, episode, 0, 1, now, report)
}

func (f *SyosetuFetcher) fetchEpisodeAt(ctx context.Context, work model.Work, episode model.Episode, index int, totalEpisodes int, now time.Time, report ProgressReporter) (model.Episode, error) {
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
	element, err := parseSyosetuEpisode(rawHTML)
	if err != nil {
		return model.Episode{}, fmt.Errorf("%s: %w", episodeURL, err)
	}
	episode.Element = element
	episode.RawHTML = rawHTML
	episode.FetchedAt = now
	return episode, nil
}

func reportProgress(report ProgressReporter, progress Progress) {
	if report != nil {
		report(progress)
	}
}

func (f *SyosetuFetcher) fetchTocPages(ctx context.Context, tocURL string) ([]string, error) {
	pages := []string{}
	nextURL := tocURL
	visited := map[string]bool{}

	for len(pages) < f.maxTocPages {
		if visited[nextURL] {
			break
		}
		visited[nextURL] = true

		html, err := f.httpFetcher.FetchText(ctx, nextURL, f.policy)
		if err != nil {
			return nil, err
		}
		pages = append(pages, html)

		nextHref, err := extractNextTocHref(html)
		if err != nil {
			return nil, err
		}
		if nextHref == "" {
			break
		}
		nextURL = resolveURL(tocURL, nextHref)
	}

	if len(pages) >= f.maxTocPages {
		lastPage := pages[len(pages)-1]
		nextHref, err := extractNextTocHref(lastPage)
		if err != nil {
			return nil, err
		}
		if nextHref != "" {
			return nil, fmt.Errorf("toc page limit reached before reading all pages: maxTocPages=%d", f.maxTocPages)
		}
	}

	return pages, nil
}

func parseSyosetuToc(tocURL string, ncode string, pages []string) (model.Work, error) {
	if len(pages) == 0 {
		return model.Work{}, fmt.Errorf("toc page is empty")
	}

	firstDoc, err := goquery.NewDocumentFromReader(strings.NewReader(pages[0]))
	if err != nil {
		return model.Work{}, err
	}

	work := model.Work{
		Site:       model.SiteSyosetu,
		SiteName:   "小説家になろう",
		SiteWorkID: strings.ToLower(ncode),
		SourceURL:  tocURL,
		Title:      firstText(firstDoc, ".p-novel__title", "h1", `meta[property="og:title"]`),
		Author:     firstText(firstDoc, ".p-novel__author a", ".p-novel__author", `meta[name="author"]`),
		Story:      firstHTML(firstDoc, "#novel_ex", ".p-novel__summary"),
	}
	if work.Title == "" {
		work.Title = strings.ToLower(ncode)
	}

	for _, page := range pages {
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(page))
		if err != nil {
			return model.Work{}, err
		}
		work.Episodes = append(work.Episodes, parseSyosetuEpisodes(doc)...)
	}

	if len(work.Episodes) == 0 {
		return model.Work{}, fmt.Errorf("no episodes found in toc")
	}
	for index := range work.Episodes {
		work.Episodes[index].SourceURL = resolveURL(work.SourceURL, work.Episodes[index].Href)
	}
	return work, nil
}

func parseSyosetuEpisodes(doc *goquery.Document) []model.Episode {
	episodes := []model.Episode{}

	doc.Find(".p-eplist__sublist").Each(func(_ int, selection *goquery.Selection) {
		link := selection.Find(".p-eplist__subtitle").First()
		href, _ := link.Attr("href")
		title := strings.TrimSpace(link.Text())
		index := extractEpisodeIndex(href)
		if index == "" || title == "" {
			return
		}

		update := strings.TrimSpace(selection.Find(".p-eplist__update").First().Text())
		publishedAt := normalizeSpace(stripUpdateMarker(update))
		modifiedAt := ""
		if revised, ok := selection.Find(".p-eplist__update span[title]").First().Attr("title"); ok {
			modifiedAt = strings.TrimSpace(strings.TrimSuffix(revised, " 改稿"))
		}

		episodes = append(episodes, model.Episode{
			Index:        index,
			Href:         href,
			Chapter:      findNearestChapter(selection),
			Subchapter:   "",
			Title:        title,
			FileSubtitle: title,
			PublishedAt:  publishedAt,
			ModifiedAt:   modifiedAt,
			Element: model.EpisodeElement{
				DataType: "html",
			},
		})
	})

	return episodes
}

func parseSyosetuEpisode(html string) (model.EpisodeElement, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return model.EpisodeElement{}, err
	}

	body := firstHTML(doc, ".js-novel-text.p-novel__text:not(.p-novel__text--preface):not(.p-novel__text--afterword)")
	if body == "" {
		body = firstHTML(doc, ".js-novel-text.p-novel__text")
	}
	if strings.TrimSpace(body) == "" {
		return model.EpisodeElement{}, fmt.Errorf("syosetu episode body was not found")
	}

	return model.EpisodeElement{
		DataType:     "html",
		Introduction: firstHTML(doc, ".js-novel-text.p-novel__text--preface"),
		Body:         body,
		Postscript:   firstHTML(doc, ".js-novel-text.p-novel__text--afterword"),
	}, nil
}

func resolveNCode(target string) (string, error) {
	trimmed := strings.TrimSpace(target)
	if trimmed == "" {
		return "", fmt.Errorf("target is empty")
	}

	if parsed, err := url.Parse(trimmed); err == nil && parsed.Host != "" {
		if parsed.Host != "ncode.syosetu.com" {
			return "", fmt.Errorf("%w: %s", ErrUnsupportedSite, parsed.Host)
		}
		if match := ncodePattern.FindString(parsed.Path); match != "" {
			return strings.ToLower(match), nil
		}
	}

	if match := ncodePattern.FindString(trimmed); match != "" {
		return strings.ToLower(match), nil
	}
	return "", fmt.Errorf("target is not a supported ncode or syosetu URL")
}

func extractNextTocHref(html string) (string, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return "", err
	}
	href, _ := doc.Find("a.c-pager__item--next").First().Attr("href")
	return href, nil
}

func firstText(doc *goquery.Document, selectors ...string) string {
	for _, selector := range selectors {
		selection := doc.Find(selector).First()
		if selection.Length() == 0 {
			continue
		}
		if content, ok := selection.Attr("content"); ok {
			if trimmed := strings.TrimSpace(content); trimmed != "" {
				return trimmed
			}
		}
		if text := strings.TrimSpace(selection.Text()); text != "" {
			return text
		}
	}
	return ""
}

func firstHTML(doc *goquery.Document, selectors ...string) string {
	for _, selector := range selectors {
		selection := doc.Find(selector).First()
		if selection.Length() == 0 {
			continue
		}
		html, err := selection.Html()
		if err == nil && strings.TrimSpace(html) != "" {
			return strings.TrimSpace(html)
		}
	}
	return ""
}

func extractEpisodeIndex(href string) string {
	parts := strings.Split(strings.Trim(href, "/"), "/")
	if len(parts) == 0 {
		return ""
	}
	last := parts[len(parts)-1]
	if regexp.MustCompile(`^\d+$`).MatchString(last) {
		return last
	}
	return ""
}

func findNearestChapter(selection *goquery.Selection) string {
	chapter := ""
	selection.PrevAll().EachWithBreak(func(_ int, sibling *goquery.Selection) bool {
		if goquery.NodeName(sibling) == "div" && sibling.HasClass("p-eplist__chapter-title") {
			chapter = strings.TrimSpace(sibling.Text())
			return false
		}
		return true
	})
	return chapter
}

func stripUpdateMarker(value string) string {
	replacer := regexp.MustCompile(`（\s*改\s*）`)
	return replacer.ReplaceAllString(value, "")
}

func normalizeSpace(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func resolveURL(base string, href string) string {
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
