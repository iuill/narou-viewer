package sites

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func TestResolveKakuyomuWorkID(t *testing.T) {
	tests := map[string]string{
		"https://kakuyomu.jp/works/0000000000000000000":                              "0000000000000000000",
		"https://kakuyomu.jp/works/0000000000000000000/episodes/0000000000000000001": "0000000000000000000",
	}

	for input, expected := range tests {
		actual, err := resolveKakuyomuWorkID(input)
		if err != nil {
			t.Fatalf("resolveKakuyomuWorkID(%q) returned error: %v", input, err)
		}
		if actual != expected {
			t.Fatalf("resolveKakuyomuWorkID(%q) = %q, want %q", input, actual, expected)
		}
	}
}

func TestResolveKakuyomuWorkIDRejectsOtherHosts(t *testing.T) {
	for _, input := range []string{"", "n123", "https://ncode.syosetu.com/n1234ab/", "https://kakuyomu.jp/users/example"} {
		if _, err := resolveKakuyomuWorkID(input); err == nil {
			t.Fatalf("resolveKakuyomuWorkID(%q) returned nil error", input)
		}
	}
}

func TestParseKakuyomuToc(t *testing.T) {
	html := `<html><script id="__NEXT_DATA__" type="application/json">{
  "props": {
    "pageProps": {
      "__APOLLO_STATE__": {
        "Work:0000000000000000000": {
          "title": "先輩の妹じゃありません！",
          "author": {"__ref": "UserAccount:1"},
          "alternateAuthorName": "別名義",
          "introduction": "intro\nbody",
          "tableOfContentsV2": [{"__ref": "TableOfContentsChapter:10"}]
        },
        "UserAccount:1": {
          "activityName": "author-name"
        },
        "TableOfContentsChapter:10": {
          "chapter": {"__ref": "Chapter:10"},
          "episodeUnions": [
            {"__ref": "Chapter:11"},
            {"__ref": "Episode:20"},
            {"__ref": "Episode:9007199254740993"}
          ]
        },
        "Chapter:10": {
          "__typename": "Chapter",
          "id": "10",
          "level": 1,
          "title": "第一章"
        },
        "Chapter:11": {
          "__typename": "Chapter",
          "id": "11",
          "level": 2,
          "title": "一幕"
        },
        "Episode:20": {
          "__typename": "Episode",
          "id": "20",
          "workId": "0000000000000000000",
          "publishedAt": "2021-01-12T16:13:02Z",
          "editedAt": "2021-01-12T17:13:02Z",
          "title": "第1話"
        },
        "Episode:9007199254740993": {
          "__typename": "Episode",
          "id": 9007199254740993,
          "publishedAt": "2021-01-13T16:13:02Z",
          "title": "第2話"
        }
      }
    }
  },
  "query": {
    "workId": "0000000000000000000"
  }
}</script></html>`

	work, err := parseKakuyomuToc("https://kakuyomu.jp/works/0000000000000000000", "0000000000000000000", html)
	if err != nil {
		t.Fatalf("parseKakuyomuToc returned error: %v", err)
	}
	if work.Title != "先輩の妹じゃありません！" || work.Author != "別名義／author-name" {
		t.Fatalf("unexpected work metadata: %#v", work)
	}
	if work.Story != "intro<br>body" {
		t.Fatalf("work.Story = %q", work.Story)
	}
	if len(work.Episodes) != 2 {
		t.Fatalf("len(work.Episodes) = %d, want 2", len(work.Episodes))
	}
	first := work.Episodes[0]
	if first.Index != "20" || first.Href != "/works/0000000000000000000/episodes/20" || first.Title != "第1話" {
		t.Fatalf("unexpected first episode: %#v", first)
	}
	if first.Chapter != "第一章" || first.Subchapter != "一幕" {
		t.Fatalf("unexpected chapter values: %#v", first)
	}
	if first.PublishedAt != "2021-01-12T16:13:02Z" || first.ModifiedAt != "2021-01-12T17:13:02Z" {
		t.Fatalf("unexpected episode timestamps: %#v", first)
	}
	if work.Episodes[1].Href != "/works/0000000000000000000/episodes/9007199254740993" {
		t.Fatalf("fallback work id was not used: %#v", work.Episodes[1])
	}
	if work.Episodes[1].ModifiedAt != "2021-01-13T16:13:02Z" {
		t.Fatalf("publishedAt fallback was not used for ModifiedAt: %#v", work.Episodes[1])
	}
}

func TestParseKakuyomuEpisode(t *testing.T) {
	html := `<div class="widget-episodeBody js-episode-body"><p>本文</p></div>`

	element, err := parseKakuyomuEpisode(html)
	if err != nil {
		t.Fatalf("parseKakuyomuEpisode returned error: %v", err)
	}
	if element.DataType != "html" || element.Body != "<p>本文</p>" {
		t.Fatalf("unexpected element: %#v", element)
	}
}

func TestKakuyomuFetcherfetchWholeWorkUsesTocAndEpisodeHTML(t *testing.T) {
	httpFetcher := &fakeTextFetcher{responses: map[string]string{
		"https://kakuyomu.jp/works/0000000000000000000": `<html><script id="__NEXT_DATA__" type="application/json">{
  "props": {"pageProps": {"__APOLLO_STATE__": {
    "Work:0000000000000000000": {
      "title": "通し取得カクヨム",
      "author": {"__ref": "UserAccount:1"},
      "introduction": "紹介",
      "tableOfContentsV2": [{"__ref": "TableOfContentsChapter:10"}]
    },
    "UserAccount:1": {"activityName": "作者"},
    "TableOfContentsChapter:10": {
      "episodeUnions": [
        {"__ref": "Episode:20"},
        {"__ref": "Episode:21"}
      ]
    },
    "Episode:20": {"__typename": "Episode", "id": "20", "workId": "0000000000000000000", "title": "第1話"},
    "Episode:21": {"__typename": "Episode", "id": "21", "workId": "0000000000000000000", "title": "第2話"}
  }}},
  "query": {"workId": "0000000000000000000"}
}</script></html>`,
		"https://kakuyomu.jp/works/0000000000000000000/episodes/20": `<div class="widget-episodeBody js-episode-body"><p>一本文</p></div>`,
		"https://kakuyomu.jp/works/0000000000000000000/episodes/21": `<div class="p-episode__body"><p>二本文</p></div>`,
	}}
	fetcher := &KakuyomuFetcher{httpFetcher: httpFetcher}
	progresses := []Progress{}

	work, err := fetchWholeWork(context.Background(), fetcher, "https://kakuyomu.jp/works/0000000000000000000", func(progress Progress) {
		progresses = append(progresses, progress)
	})
	if err != nil {
		t.Fatalf("fetchWholeWork returned error: %v", err)
	}
	if work.Title != "通し取得カクヨム" || work.Author != "作者" || len(work.Episodes) != 2 {
		t.Fatalf("work = %#v", work)
	}
	if work.Episodes[0].RawHTML == "" || work.Episodes[0].Element.Body != "<p>一本文</p>" {
		t.Fatalf("first episode = %#v", work.Episodes[0])
	}
	if work.Episodes[1].Element.Body != "<p>二本文</p>" {
		t.Fatalf("second episode = %#v", work.Episodes[1])
	}
	if len(httpFetcher.requests) != 3 {
		t.Fatalf("requests = %#v", httpFetcher.requests)
	}
	if len(progresses) < 3 || progresses[0].Phase != "toc" || progresses[len(progresses)-1].CurrentStep != 2 {
		t.Fatalf("progresses = %#v", progresses)
	}
}

func TestKakuyomuFetcherfetchWholeWorkReturnsEpisodeFetchError(t *testing.T) {
	httpFetcher := &fakeTextFetcher{
		responses: map[string]string{
			"https://kakuyomu.jp/works/1": `<script id="__NEXT_DATA__">{
  "props": {"pageProps": {"__APOLLO_STATE__": {
    "Work:1": {
      "title": "x",
      "tableOfContents": [{"__typename": "Episode", "id": "2", "title": "話"}]
    }
  }}},
  "query": {"workId": "1"}
}</script>`,
		},
		errs: map[string]error{
			"https://kakuyomu.jp/works/1/episodes/2": context.Canceled,
		},
	}
	fetcher := &KakuyomuFetcher{httpFetcher: httpFetcher}

	if _, err := fetchWholeWork(context.Background(), fetcher, "https://kakuyomu.jp/works/1", nil); err != context.Canceled {
		t.Fatalf("fetchWholeWork error = %v, want context.Canceled", err)
	}
}

func TestParseKakuyomuTocSupportsInlineLegacyTableOfContents(t *testing.T) {
	html := `<script id="__NEXT_DATA__">{
  "props": {"pageProps": {"__APOLLO_STATE__": {
    "Work:1": {
      "title": "",
      "author": {"__ref": "UserAccount:1"},
      "introduction": "",
      "tableOfContents": [
        {"__typename": "Chapter", "level": "1", "title": "章"},
        {"__typename": "Episode", "id": "2", "title": "話", "publishedAt": "2026-05-09"}
      ]
    },
    "UserAccount:1": {"activityName": "作者"}
  }}},
  "query": {}
}</script>`

	work, err := parseKakuyomuToc("https://kakuyomu.jp/works/1", "1", html)
	if err != nil {
		t.Fatalf("parseKakuyomuToc returned error: %v", err)
	}
	if work.Title != "1" || work.Author != "作者" {
		t.Fatalf("metadata = %#v", work)
	}
	if len(work.Episodes) != 1 || work.Episodes[0].Chapter != "章" || work.Episodes[0].Href != "/works/1/episodes/2" {
		t.Fatalf("episodes = %#v", work.Episodes)
	}
}

func TestParseKakuyomuTocRejectsMissingData(t *testing.T) {
	for _, html := range []string{
		`<html></html>`,
		`<script id="__NEXT_DATA__">{bad json}</script>`,
		`<script id="__NEXT_DATA__">{"props":{"pageProps":{"__APOLLO_STATE__":{}}}}</script>`,
		`<script id="__NEXT_DATA__">{"props":{"pageProps":{"__APOLLO_STATE__":{"Work:1":{"title":"x"}}}}}</script>`,
	} {
		if _, err := parseKakuyomuToc("https://kakuyomu.jp/works/1", "1", html); err == nil {
			t.Fatalf("parseKakuyomuToc(%q) returned nil error", html)
		}
	}
}

func TestParseKakuyomuEpisodeFallbackSelectorAndMissingBody(t *testing.T) {
	element, err := parseKakuyomuEpisode(`<div class="p-episode__body"><p>本文</p></div>`)
	if err != nil {
		t.Fatalf("parseKakuyomuEpisode fallback returned error: %v", err)
	}
	if element.Body != "<p>本文</p>" {
		t.Fatalf("body = %q", element.Body)
	}
	if _, err := parseKakuyomuEpisode(`<div>missing</div>`); err == nil {
		t.Fatal("parseKakuyomuEpisode missing body returned nil error")
	}
}

func TestKakuyomuValueHelpers(t *testing.T) {
	state := map[string]any{"User:1": map[string]any{"name": "tester"}}
	if mapValue(state, "missing") != nil || mapValue(nil, "User:1") != nil || mapValue(state, "") != nil {
		t.Fatal("mapValue nil branches failed")
	}
	if refValue(map[string]any{"__ref": "User:1"}) != "User:1" {
		t.Fatal("refValue failed")
	}
	if stringValue(float64(12)) != "12" || stringValue(json.Number("9007199254740993")) != "9007199254740993" || stringValue(true) != "" {
		t.Fatal("stringValue failed")
	}
	if intValue(float64(2)) != 2 || intValue(json.Number("3")) != 3 || intValue("1") != 1 || intValue("x") != 0 {
		t.Fatal("intValue failed")
	}
}

func TestExtractKakuyomuNextDataAcceptsScriptWithoutType(t *testing.T) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(`<script id="__NEXT_DATA__">{"props":{"pageProps":{"__APOLLO_STATE__":{"x":{}}}}}</script>`))
	if err != nil {
		t.Fatalf("NewDocumentFromReader returned error: %v", err)
	}
	data, err := extractKakuyomuNextData(doc)
	if err != nil {
		t.Fatalf("extractKakuyomuNextData returned error: %v", err)
	}
	if len(data.Props.PageProps.ApolloState) != 1 {
		t.Fatalf("ApolloState = %#v", data.Props.PageProps.ApolloState)
	}
}
