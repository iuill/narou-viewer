package main

import (
	"context"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"time"

	"narou-viewer/services/novel-fetcher/internal/model"
	"narou-viewer/services/novel-fetcher/internal/storage"
)

func main() {
	outputDir := flag.String("output", "", "output directory for the novel-fetcher fixture")
	workSet := flag.String("work-set", "e2e", "fixture work set to build: all, e2e, or verification")
	flag.Parse()

	if *outputDir == "" {
		fmt.Fprintln(os.Stderr, "--output is required")
		os.Exit(2)
	}

	if err := buildFixture(*outputDir, *workSet); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build fixture: %v\n", err)
		os.Exit(1)
	}
}

func buildFixture(outputDir string, workSet string) error {
	store, err := storage.NewStore(outputDir)
	if err != nil {
		return err
	}
	defer store.Close()

	works, err := fixtureWorks(workSet)
	if err != nil {
		return err
	}
	for _, work := range works {
		if err := saveWork(context.Background(), store, outputDir, work); err != nil {
			return err
		}
	}

	return nil
}

func saveWork(ctx context.Context, store *storage.Store, outputDir string, work model.Work) error {
	stored, err := store.UpsertWorkToc(ctx, work, storage.FetchStatusPartial)
	if err != nil {
		return err
	}

	for index, episode := range work.Episodes {
		if _, err := store.SaveEpisodeBody(ctx, work, stored, episode, index); err != nil {
			return err
		}
	}

	if err := store.UpdateWorkFetchStatus(ctx, stored.ID, storage.FetchStatusComplete, "", "", nil); err != nil {
		return err
	}

	if work.Site == model.SiteSyosetu && work.SiteWorkID == "n1234ab" {
		return writeSyntheticIllustration(filepath.Join(outputDir, stored.Directory, "assets", "episodes", "1", "synthetic.png"))
	}

	return nil
}

func fixtureWorks(workSet string) ([]model.Work, error) {
	fetchedAt := time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)
	e2eWorks := []model.Work{
		syosetuFixtureWork(
			"n1234ab",
			"E2E ケースA 挿絵表示",
			longStory("ケースAの挿絵表示を確認する合成fixtureです。"),
			fetchedAt,
			[]model.Episode{
				illustratedEpisode("n1234ab", fetchedAt.Add(time.Minute)),
				syosetuFixtureEpisode("n1234ab", "2", "第二話", "第一章", "続き", longBody("case-a"), fetchedAt.Add(2*time.Minute)),
			},
		),
		longSyosetuFixtureWork("n2234ab", "E2E ケースB 読書ログ", "ケースBの読書状態を確認する合成fixtureです。", "case-b", fetchedAt.Add(10*time.Minute)),
		longSyosetuFixtureWork("n2234ac", "E2E ケースC 活動アンカー", "ケースCの活動アンカーを確認する合成fixtureです。", "case-c", fetchedAt.Add(20*time.Minute)),
		longSyosetuFixtureWork("n3234ab", "E2E ケースD 本文操作", "ケースDの本文操作を確認する合成fixtureです。", "case-d", fetchedAt.Add(30*time.Minute)),
		longSyosetuFixtureWork("n4234ab", "E2E ケースE 栞", "ケースEの栞操作を確認する合成fixtureです。", "case-e", fetchedAt.Add(40*time.Minute)),
		longSyosetuFixtureWork("n5234ab", "E2E ケースF エクスポート", "ケースFの YAML エクスポートを確認する合成fixtureです。", "case-f", fetchedAt.Add(50*time.Minute)),
		{
			Site:       model.SiteKakuyomu,
			SiteName:   "カクヨム",
			SiteWorkID: "0000000000000000000",
			SourceURL:  "https://kakuyomu.example/works/0000000000000000000",
			Title:      "E2E ケースG カクヨム形式",
			Author:     "テスト作者",
			Story:      "ケースGのカクヨム形式 URL を確認する合成fixtureです。",
			FetchedAt:  fetchedAt.Add(time.Hour),
			Episodes: []model.Episode{
				fixtureEpisode("0000000000000000001", "/works/0000000000000000000/episodes/0000000000000000001", "第1話", "章", "", "合成本文 case-g-01。カクヨム形式の ID 表示確認用です。", fetchedAt.Add(time.Hour+time.Minute)),
				fixtureEpisode("0000000000000000002", "/works/0000000000000000000/episodes/0000000000000000002", "第2話", "章", "", "合成本文 case-g-02。カクヨム形式の並び順確認用です。", fetchedAt.Add(time.Hour+2*time.Minute)),
			},
		},
	}
	verificationWorks := []model.Work{characterAliasVerificationWork(fetchedAt.Add(2 * time.Hour))}

	switch strings.TrimSpace(strings.ToLower(workSet)) {
	case "", "e2e":
		return e2eWorks, nil
	case "all":
		return append(e2eWorks, verificationWorks...), nil
	case "verification":
		return verificationWorks, nil
	default:
		return nil, fmt.Errorf("unknown work set %q", workSet)
	}
}

func syosetuFixtureWork(siteWorkID string, title string, story string, fetchedAt time.Time, episodes []model.Episode) model.Work {
	return model.Work{
		Site:       model.SiteSyosetu,
		SiteName:   "小説家になろう",
		SiteWorkID: siteWorkID,
		SourceURL:  "https://ncode.syosetu.com/" + siteWorkID + "/",
		Title:      title,
		Author:     "テスト作者",
		Story:      story,
		FetchedAt:  fetchedAt,
		Episodes:   episodes,
	}
}

func longSyosetuFixtureWork(siteWorkID string, title string, story string, motif string, fetchedAt time.Time) model.Work {
	return syosetuFixtureWork(
		siteWorkID,
		title,
		story,
		fetchedAt,
		[]model.Episode{
			syosetuFixtureEpisode(
				siteWorkID,
				"1",
				"第一話",
				"第一章",
				"開幕",
				fmt.Sprintf("合成本文 %s-01。初期表示確認用の短い本文です。", motif),
				fetchedAt.Add(time.Minute),
			),
			syosetuFixtureEpisode(siteWorkID, "2", "第二話", "第一章", "続き", longBody(motif), fetchedAt.Add(2*time.Minute)),
		},
	)
}

func characterAliasVerificationWork(fetchedAt time.Time) model.Work {
	episodes := []model.Episode{
		verificationEpisode("1", "第一話　検証人物A", "第一章", "導入", []string{
			"検証人物Aの正式名はサンプル・アオである。記録上の短縮名はアオ、呼称はアオさんである。",
			"検証人物Bの正式名はサンプル・セラである。役職呼称はセラ係長、短縮名はセラである。",
			"検証人物Cの正式名はサンプル・レンである。検証人物Cは検証人物Aをアオさんと呼ぶ。",
		}, fetchedAt.Add(time.Minute)),
		verificationEpisode("2", "第二話　共通呼称", "第一章", "呼称の揺れ", []string{
			"検証人物Dの正式名はサンプル・ハルであり、共通呼称は先生である。",
			"検証人物Eの正式名はサンプル・ナギであり、同じく共通呼称は先生である。",
			"この話では、先生という呼称が検証人物Dと検証人物Eの両方に使われる。",
		}, fetchedAt.Add(2*time.Minute)),
		verificationEpisode("3", "第三話　別名", "第一章", "偽名", []string{
			"検証人物Bは別名クロを一時的に使う。クロは検証人物Bと同一人物である。",
			"検証人物Aは別名クロを検証人物Bとは知らずに記録する。",
			"検証人物Fは、共通呼称先生を使う人物がいたとだけ記録する。",
		}, fetchedAt.Add(3*time.Minute)),
		verificationEpisode("4", "第四話　先生の分離", "第二章", "分離", []string{
			"検証人物Dは先生と呼ばれるが、第三話の先生とは別人である。",
			"検証人物Eも先生と呼ばれ、第三話の先生候補として記録される。",
			"検証人物Bは、検証人物Dと検証人物Eを別人物として扱う。",
		}, fetchedAt.Add(4*time.Minute)),
		verificationEpisode("5", "第五話　血縁呼称", "第二章", "血縁", []string{
			"検証人物Gの正式名はサンプル・ユウトである。検証人物Aは検証人物Gを兄さんと呼ぶ。",
			"検証人物Dは検証人物Gをユウトと呼ぶ。",
			"別記録では、検証人物Gに短縮名ユウが使われる。",
		}, fetchedAt.Add(5*time.Minute)),
		verificationEpisode("6", "第六話　役職名", "第二章", "役職名", []string{
			"検証人物Hの正式名はサンプル・イリスである。検証人物Hは一時的に隊長代理と呼ばれる。",
			"隊長という語は、検証人物Bと検証人物Hの両方に使われる。",
			"この話では、隊長代理は検証人物Hだけを指す。",
		}, fetchedAt.Add(6*time.Minute)),
		verificationEpisode("7", "第七話　短縮名", "第三章", "再登場", []string{
			"短縮名ユウを使う検証人物Iが登場する。",
			"検証人物Aは、検証人物Iと検証人物Gが同一人物か判断できない。",
			"検証人物Cは、検証人物Iを別人物候補として記録する。",
		}, fetchedAt.Add(7*time.Minute)),
		verificationEpisode("8", "第八話　追加血縁呼称", "第三章", "血縁呼称", []string{
			"検証人物Jの正式名はサンプル・リサである。検証人物Aは検証人物Jを伯母さんと呼ぶ。",
			"検証人物Jは、別記録で鍵の人と呼ばれる。",
			"検証人物Iは、検証人物Jを伯母さんとは呼ばず、鍵の人と呼ぶ。",
		}, fetchedAt.Add(8*time.Minute)),
		verificationEpisode("9", "第九話　同一人物候補", "第三章", "同一人物の判明", []string{
			"鍵の人は検証人物Jである。",
			"短縮名ユウは、検証人物Gの短縮名としても、検証人物Iの名前としても使われる。",
			"検証人物Iは検証人物Gと完全な同一人物ではない、という記録を残す。",
		}, fetchedAt.Add(9*time.Minute)),
		verificationEpisode("10", "第十話　名前を分ける", "第三章", "整理", []string{
			"検証人物B、別名クロ、セラ係長は同一人物として整理する。",
			"検証人物D先生と検証人物E先生は別人物として整理する。",
			"検証人物Gと検証人物Iは、短縮名ユウを共有するが別人物として整理する。",
		}, fetchedAt.Add(10*time.Minute)),
	}

	return model.Work{
		Site:       model.SiteVerification,
		SiteName:   "LLM検証用",
		SiteWorkID: "llm-character-alias-001",
		SourceURL:  "https://verification.local/works/llm-character-alias-001",
		Title:      "E2E 人物名寄せ検証",
		Author:     "検証用データ",
		Story:      "キャラクター抽出で、本名・別名・役職名・血縁呼称・偽名を混同しないか確認するための合成検証データです。",
		FetchedAt:  fetchedAt,
		Episodes:   episodes,
	}
}

func verificationEpisode(index string, title string, chapter string, subchapter string, paragraphs []string, fetchedAt time.Time) model.Episode {
	bodyParagraphs := make([]string, 0, len(paragraphs))
	for _, paragraph := range paragraphs {
		bodyParagraphs = append(bodyParagraphs, "<p>"+paragraph+"</p>")
	}
	body := strings.Join(bodyParagraphs, "\n")
	href := "/works/llm-character-alias-001/episodes/" + index

	return model.Episode{
		Index:        index,
		Href:         href,
		SourceURL:    "https://verification.local" + href,
		Title:        title,
		FileSubtitle: title,
		Chapter:      chapter,
		Subchapter:   subchapter,
		PublishedAt:  "2026/06/29 12:00",
		ModifiedAt:   "2026/06/29 12:00",
		FetchedAt:    fetchedAt,
		Element: model.EpisodeElement{
			DataType:     "html",
			Introduction: "",
			Body:         body,
			Postscript:   "",
		},
		RawHTML: "<html><body>" + body + "</body></html>",
	}
}

func fixtureEpisode(index string, href string, title string, chapter string, subchapter string, body string, fetchedAt time.Time) model.Episode {
	bodyHTML := "<p>" + body + "</p>"
	if strings.HasPrefix(strings.TrimSpace(body), "<") {
		bodyHTML = body
	}

	return model.Episode{
		Index:        index,
		Href:         href,
		Title:        title,
		FileSubtitle: title,
		Chapter:      chapter,
		Subchapter:   subchapter,
		PublishedAt:  "2026/05/09 12:00",
		ModifiedAt:   "2026/05/09 12:00",
		FetchedAt:    fetchedAt,
		Element: model.EpisodeElement{
			DataType:     "html",
			Introduction: "<p>合成前書きです。</p>",
			Body:         bodyHTML,
			Postscript:   "<p>合成後書きです。</p>",
		},
		RawHTML: "<html><body>" + bodyHTML + "</body></html>",
	}
}

func syosetuFixtureEpisode(siteWorkID string, index string, title string, chapter string, subchapter string, body string, fetchedAt time.Time) model.Episode {
	return fixtureEpisode(index, "/"+siteWorkID+"/"+index+"/", title, chapter, subchapter, body, fetchedAt)
}

func illustratedEpisode(siteWorkID string, fetchedAt time.Time) model.Episode {
	episode := syosetuFixtureEpisode(siteWorkID, "1", "第一話", "第一章", "開幕", "合成本文 case-a-01。挿絵表示確認用の短い本文です。", fetchedAt)
	episode.Element.Body = `<p>合成本文 case-a-01。挿絵表示確認用の短い本文です。</p><p><img src="../assets/episodes/1/synthetic.png" alt="合成挿絵" width="400" height="300"></p>`
	return episode
}

func writeSyntheticIllustration(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	canvas := image.NewRGBA(image.Rect(0, 0, 400, 300))
	for y := 0; y < 300; y++ {
		for x := 0; x < 400; x++ {
			canvas.Set(x, y, color.RGBA{R: 244, G: 211, B: 94, A: 255})
		}
	}
	for y := 60; y < 240; y++ {
		for x := 110; x < 290; x++ {
			dx := x - 200
			dy := y - 150
			if dx*dx+dy*dy < 90*90 {
				canvas.Set(x, y, color.RGBA{R: 0, G: 131, B: 128, A: 255})
			}
		}
	}

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	return png.Encode(file, canvas)
}

func longStory(seed string) string {
	return strings.Repeat(seed+"移行後のライブラリ表示と折りたたみ挙動を確認するため、十分な長さを持たせています。", 4)
}

func longBody(motif string) string {
	paragraphs := make([]string, 0, 80)
	for i := 1; i <= 80; i++ {
		paragraphs = append(paragraphs, fmt.Sprintf("<p>合成本文 %s-%02d。ページ送りと読書位置の保存を確認するテスト文です。</p>", motif, i))
	}
	return strings.Join(paragraphs, "")
}
