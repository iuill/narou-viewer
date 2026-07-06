package sites

import (
	"context"
	"errors"
	"strings"
	"testing"

	"narou-viewer/services/novel-fetcher/internal/fetcher"
	"narou-viewer/services/novel-fetcher/internal/model"
)

func TestResolveNCode(t *testing.T) {
	tests := map[string]string{
		"n1234ab":                              "n1234ab",
		"N1234AB":                              "n1234ab",
		"https://ncode.syosetu.com/n1234ab/":   "n1234ab",
		"https://ncode.syosetu.com/n1234ab/1/": "n1234ab",
	}

	for input, expected := range tests {
		actual, err := resolveNCode(input)
		if err != nil {
			t.Fatalf("resolveNCode(%q) returned error: %v", input, err)
		}
		if actual != expected {
			t.Fatalf("resolveNCode(%q) = %q, want %q", input, actual, expected)
		}
	}
}

func TestResolveNCodeRejectsEmptyAndOtherHosts(t *testing.T) {
	for _, input := range []string{"", "https://example.com/n1234ab/"} {
		if _, err := resolveNCode(input); err == nil {
			t.Fatalf("resolveNCode(%q) returned nil error", input)
		}
	}
}

func TestSiteFetcherConstructorsWireOptions(t *testing.T) {
	httpFetcher := fetcher.NewHTTPFetcher(fetcher.HTTPFetcherOptions{UserAgent: "test-agent"})
	policy := fetcher.FetchPolicy{MaxRetries: 2}

	syosetu := NewSyosetuFetcher(httpFetcher, policy, 4, nil)
	if syosetu.httpFetcher != httpFetcher || syosetu.policy.MaxRetries != 2 || syosetu.maxTocPages != 4 {
		t.Fatalf("syosetu constructor = %#v", syosetu)
	}

	kakuyomu := NewKakuyomuFetcher(httpFetcher, policy, nil)
	if kakuyomu.httpFetcher != httpFetcher || kakuyomu.policy.MaxRetries != 2 {
		t.Fatalf("kakuyomu constructor = %#v", kakuyomu)
	}
}

func TestMultiFetcherFallsThroughUnsupportedSites(t *testing.T) {
	expected := model.Work{Title: "ok"}
	fetcher := NewMultiFetcher(
		stubWorkFetcher{err: ErrUnsupportedSite},
		stubWorkFetcher{work: expected},
	)

	actual, err := fetcher.FetchToc(context.Background(), "https://example.com", nil)
	if err != nil {
		t.Fatalf("FetchToc returned error: %v", err)
	}
	if actual.Title != "ok" {
		t.Fatalf("work = %#v", actual)
	}
}

func TestMultiFetcherReturnsErrors(t *testing.T) {
	boom := errors.New("boom")
	fetcher := NewMultiFetcher(stubWorkFetcher{err: boom})
	if _, err := fetcher.FetchToc(context.Background(), "https://example.com", nil); !errors.Is(err, boom) {
		t.Fatalf("FetchToc error = %v, want boom", err)
	}

	empty := NewMultiFetcher()
	if _, err := empty.FetchToc(context.Background(), "https://example.com", nil); !errors.Is(err, ErrUnsupportedSite) {
		t.Fatalf("empty FetchToc error = %v, want ErrUnsupportedSite", err)
	}
}

func TestMultiFetcherFetchEpisodeFallsThroughUnsupportedSites(t *testing.T) {
	expected := model.Episode{Index: "20", Title: "第1話"}
	fetcher := NewMultiFetcher(
		stubWorkFetcher{err: ErrUnsupportedSite},
		stubWorkFetcher{episode: expected},
	)

	actual, err := fetcher.FetchEpisode(
		context.Background(),
		model.Work{Site: model.SiteKakuyomu, SourceURL: "https://kakuyomu.jp/works/1"},
		model.Episode{Index: "20"},
		nil,
	)
	if err != nil {
		t.Fatalf("FetchEpisode returned error: %v", err)
	}
	if actual.Title != "第1話" {
		t.Fatalf("episode = %#v", actual)
	}
}

func TestSyosetuFetcherFetchEpisodeRejectsOtherSites(t *testing.T) {
	fetcher := &SyosetuFetcher{httpFetcher: &fakeTextFetcher{}}
	_, err := fetcher.FetchEpisode(
		context.Background(),
		model.Work{Site: model.SiteKakuyomu, SourceURL: "https://kakuyomu.jp/works/1"},
		model.Episode{Index: "20"},
		nil,
	)
	if !errors.Is(err, ErrUnsupportedSite) {
		t.Fatalf("FetchEpisode error = %v, want ErrUnsupportedSite", err)
	}
}

func TestKakuyomuFetcherFetchEpisodeRejectsOtherSites(t *testing.T) {
	fetcher := &KakuyomuFetcher{httpFetcher: &fakeTextFetcher{}}
	_, err := fetcher.FetchEpisode(
		context.Background(),
		model.Work{Site: model.SiteSyosetu, SourceURL: "https://ncode.syosetu.com/n1234ab/"},
		model.Episode{Index: "1"},
		nil,
	)
	if !errors.Is(err, ErrUnsupportedSite) {
		t.Fatalf("FetchEpisode error = %v, want ErrUnsupportedSite", err)
	}
}

func TestSyosetuFetcherfetchWholeWorkUsesTocPagesAndEpisodeHTML(t *testing.T) {
	httpFetcher := &fakeTextFetcher{responses: map[string]string{
		"https://ncode.syosetu.com/n1234ab/": `
<html><body>
<h1 class="p-novel__title">通し取得作品</h1>
<div class="p-novel__author">作者</div>
<div class="p-eplist__sublist">
<a href="/n1234ab/1/" class="p-eplist__subtitle">第一話</a>
</div>
<a class="c-pager__item--next" href="/n1234ab/?p=2">次</a>
</body></html>`,
		"https://ncode.syosetu.com/n1234ab/?p=2": `
<html><body>
<div class="p-eplist__sublist">
<a href="/n1234ab/2/" class="p-eplist__subtitle">第二話</a>
</div>
</body></html>`,
		"https://ncode.syosetu.com/n1234ab/1/": `
<div class="js-novel-text p-novel__text p-novel__text--preface"><p>前書き</p></div>
<div class="js-novel-text p-novel__text"><p>一本文</p></div>`,
		"https://ncode.syosetu.com/n1234ab/2/": `
<div class="js-novel-text p-novel__text"><p>二本文</p></div>
<div class="js-novel-text p-novel__text p-novel__text--afterword"><p>後書き</p></div>`,
	}}
	fetcher := &SyosetuFetcher{
		httpFetcher: httpFetcher,
		maxTocPages: 3,
	}
	progresses := []Progress{}

	work, err := fetchWholeWork(context.Background(), fetcher, "N1234AB", func(progress Progress) {
		progresses = append(progresses, progress)
	})
	if err != nil {
		t.Fatalf("fetchWholeWork returned error: %v", err)
	}
	if work.Title != "通し取得作品" || work.Author != "作者" || len(work.Episodes) != 2 {
		t.Fatalf("work = %#v", work)
	}
	if work.Episodes[0].RawHTML == "" || work.Episodes[0].Element.Introduction != "<p>前書き</p>" {
		t.Fatalf("first episode = %#v", work.Episodes[0])
	}
	if work.Episodes[1].Element.Postscript != "<p>後書き</p>" {
		t.Fatalf("second episode = %#v", work.Episodes[1])
	}
	if len(httpFetcher.requests) != 4 {
		t.Fatalf("requests = %#v", httpFetcher.requests)
	}
	if len(progresses) < 3 || progresses[0].Phase != "toc" || progresses[len(progresses)-1].CurrentStep != 2 {
		t.Fatalf("progresses = %#v", progresses)
	}
}

func TestSyosetuFetcherfetchWholeWorkStopsAtTocPageLimit(t *testing.T) {
	httpFetcher := &fakeTextFetcher{responses: map[string]string{
		"https://ncode.syosetu.com/n1234ab/": `<a class="c-pager__item--next" href="/n1234ab/?p=2">次</a>`,
	}}
	fetcher := &SyosetuFetcher{
		httpFetcher: httpFetcher,
		maxTocPages: 1,
	}

	if _, err := fetcher.FetchToc(context.Background(), "n1234ab", nil); err == nil {
		t.Fatal("FetchToc returned nil error after hitting toc page limit")
	}
}

func TestSyosetuFetcherfetchWholeWorkReturnsEpisodeParseErrorWithURL(t *testing.T) {
	httpFetcher := &fakeTextFetcher{responses: map[string]string{
		"https://ncode.syosetu.com/n1234ab/": `
<div class="p-eplist__sublist">
<a href="/n1234ab/1/" class="p-eplist__subtitle">第一話</a>
</div>`,
		"https://ncode.syosetu.com/n1234ab/1/": `<div>missing body</div>`,
	}}
	fetcher := &SyosetuFetcher{
		httpFetcher: httpFetcher,
		maxTocPages: 2,
	}

	_, err := fetchWholeWork(context.Background(), fetcher, "n1234ab", nil)
	if err == nil || !strings.Contains(err.Error(), "https://ncode.syosetu.com/n1234ab/1/") {
		t.Fatalf("fetchWholeWork error = %v", err)
	}
}

func TestParseSyosetuToc(t *testing.T) {
	html := `
<html>
<body>
<h1 class="p-novel__title">テスト作品</h1>
<div class="p-novel__author"><a>テスト作者</a></div>
<div id="novel_ex"><p>あらすじ</p></div>
<div class="p-eplist__chapter-title">第一章</div>
<div class="p-eplist__sublist">
<a href="/n1234ab/1/" class="p-eplist__subtitle">第一話</a>
<div class="p-eplist__update">2026/05/01 12:00</div>
</div>
<div class="p-eplist__sublist">
<a href="/n1234ab/2/" class="p-eplist__subtitle">第二話</a>
<div class="p-eplist__update">2026/05/02 12:00<span title="2026/05/03 09:00 改稿">（<u>改</u>）</span></div>
</div>
</body>
</html>`

	work, err := parseSyosetuToc("https://ncode.syosetu.com/n1234ab/", "n1234ab", []string{html})
	if err != nil {
		t.Fatalf("parseSyosetuToc returned error: %v", err)
	}
	if work.Title != "テスト作品" || work.Author != "テスト作者" {
		t.Fatalf("unexpected work metadata: %#v", work)
	}
	if len(work.Episodes) != 2 {
		t.Fatalf("len(work.Episodes) = %d, want 2", len(work.Episodes))
	}
	if work.Episodes[0].Chapter != "第一章" {
		t.Fatalf("chapter = %q, want 第一章", work.Episodes[0].Chapter)
	}
	if work.Episodes[1].ModifiedAt != "2026/05/03 09:00" {
		t.Fatalf("modifiedAt = %q", work.Episodes[1].ModifiedAt)
	}
}

func TestParseSyosetuTocUsesFallbackTitleAndRejectsEmptyEpisodes(t *testing.T) {
	work, err := parseSyosetuToc("https://ncode.syosetu.com/n1234ab/", "N1234AB", []string{`
<html><body>
<meta name="author" content="meta author">
<div class="p-eplist__sublist">
<a href="/n1234ab/1/" class="p-eplist__subtitle">第一話</a>
</div>
</body></html>`})
	if err != nil {
		t.Fatalf("parseSyosetuToc returned error: %v", err)
	}
	if work.Title != "n1234ab" || work.Author != "meta author" {
		t.Fatalf("work metadata = %#v", work)
	}

	if _, err := parseSyosetuToc("https://ncode.syosetu.com/n1234ab/", "n1234ab", nil); err == nil {
		t.Fatal("parseSyosetuToc with no pages returned nil error")
	}
	if _, err := parseSyosetuToc("https://ncode.syosetu.com/n1234ab/", "n1234ab", []string{"<html></html>"}); err == nil {
		t.Fatal("parseSyosetuToc without episodes returned nil error")
	}
}

func TestExtractNextTocHrefAndResolveURL(t *testing.T) {
	href, err := extractNextTocHref(`<a class="c-pager__item--next" href="/n1234ab/?p=2">次</a>`)
	if err != nil {
		t.Fatalf("extractNextTocHref returned error: %v", err)
	}
	if href != "/n1234ab/?p=2" {
		t.Fatalf("href = %q", href)
	}
	if got := resolveURL("https://ncode.syosetu.com/n1234ab/", "../n1234ab/2/"); got != "https://ncode.syosetu.com/n1234ab/2/" {
		t.Fatalf("resolveURL = %q", got)
	}
	if got := resolveURL("://bad", "/path"); got != "/path" {
		t.Fatalf("resolveURL invalid base = %q", got)
	}
}

func TestParseSyosetuEpisode(t *testing.T) {
	html := `
<div class="js-novel-text p-novel__text p-novel__text--preface"><p>前書き</p></div>
<div class="js-novel-text p-novel__text"><p>本文</p></div>
<div class="js-novel-text p-novel__text p-novel__text--afterword"><p>後書き</p></div>`

	element, err := parseSyosetuEpisode(html)
	if err != nil {
		t.Fatalf("parseSyosetuEpisode returned error: %v", err)
	}
	if element.Introduction != "<p>前書き</p>" || element.Body != "<p>本文</p>" || element.Postscript != "<p>後書き</p>" {
		t.Fatalf("unexpected element: %#v", element)
	}
}

func TestParseSyosetuEpisodeRejectsMissingBody(t *testing.T) {
	if _, err := parseSyosetuEpisode(`<div class="unexpected"><p>ブロックページ</p></div>`); err == nil {
		t.Fatal("parseSyosetuEpisode succeeded without a body selector, want error")
	}
}

func fetchWholeWork(ctx context.Context, fetcher WorkFetcher, target string, report ProgressReporter) (model.Work, error) {
	work, err := fetcher.FetchToc(ctx, target, report)
	if err != nil {
		return model.Work{}, err
	}
	episodes := make([]model.Episode, 0, len(work.Episodes))
	totalEpisodes := len(work.Episodes)
	for index, episode := range work.Episodes {
		fetched, err := fetcher.FetchEpisode(ctx, work, episode, func(progress Progress) {
			if progress.TotalSteps <= 1 {
				progress.CurrentStep = index + 1
				progress.TotalSteps = totalEpisodes
			}
			reportProgress(report, progress)
		})
		if err != nil {
			return model.Work{}, err
		}
		episodes = append(episodes, fetched)
	}
	work.Episodes = episodes
	return work, nil
}

type stubWorkFetcher struct {
	work    model.Work
	episode model.Episode
	err     error
}

func (f stubWorkFetcher) FetchToc(_ context.Context, _ string, _ ProgressReporter) (model.Work, error) {
	return f.work, f.err
}

func (f stubWorkFetcher) FetchEpisode(_ context.Context, _ model.Work, episode model.Episode, _ ProgressReporter) (model.Episode, error) {
	if f.episode.Index != "" || f.episode.Title != "" {
		return f.episode, f.err
	}
	return episode, f.err
}

type fakeTextFetcher struct {
	responses map[string]string
	errs      map[string]error
	requests  []string
}

func (f *fakeTextFetcher) FetchText(_ context.Context, rawURL string, _ fetcher.FetchPolicy) (string, error) {
	f.requests = append(f.requests, rawURL)
	if err := f.errs[rawURL]; err != nil {
		return "", err
	}
	if response, ok := f.responses[rawURL]; ok {
		return response, nil
	}
	return "", errors.New("unexpected URL: " + rawURL)
}
