package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildFixtureProducesDeterministicDatabase(t *testing.T) {
	outputDir := t.TempDir()
	if err := buildFixture(outputDir, "e2e"); err != nil {
		t.Fatalf("first buildFixture returned error: %v", err)
	}
	firstDatabase, err := os.ReadFile(filepath.Join(outputDir, "library.sqlite"))
	if err != nil {
		t.Fatalf("read first fixture database: %v", err)
	}

	if err := buildFixture(outputDir, "e2e"); err != nil {
		t.Fatalf("second buildFixture returned error: %v", err)
	}
	secondDatabase, err := os.ReadFile(filepath.Join(outputDir, "library.sqlite"))
	if err != nil {
		t.Fatalf("read second fixture database: %v", err)
	}

	if !bytes.Equal(firstDatabase, secondDatabase) {
		t.Fatal("fixture database changed between identical builds")
	}
}

func TestE2EWorksIncludeDedicatedReaderFixtures(t *testing.T) {
	works, err := fixtureWorks("e2e")
	if err != nil {
		t.Fatalf("fixtureWorks returned error: %v", err)
	}

	wantTitles := map[string]string{
		"n3234ab": "E2E ケースD 本文操作",
		"n6234ab": "E2E ケースH 作品内検索",
	}
	for _, work := range works {
		if wantTitle, ok := wantTitles[work.SiteWorkID]; ok {
			if work.Title != wantTitle || len(work.Episodes) != 2 {
				t.Fatalf("unexpected exclusive fixture %s: %+v", work.SiteWorkID, work)
			}
			delete(wantTitles, work.SiteWorkID)
		}
	}
	if len(wantTitles) != 0 {
		t.Fatalf("exclusive fixtures were not found: %+v", wantTitles)
	}
}

func TestVerificationWorksIncludeReaderCorrectionFixture(t *testing.T) {
	works, err := fixtureWorks("verification")
	if err != nil {
		t.Fatalf("fixtureWorks returned error: %v", err)
	}

	for _, work := range works {
		if work.SiteWorkID != "reader-correction-001" {
			continue
		}
		if work.Title != "校正確認用 チルダ・連続ピリオド合成本文" || len(work.Episodes) != 2 {
			t.Fatalf("unexpected correction fixture: %+v", work)
		}
		patternBody := work.Episodes[0].Element.Body
		for _, pattern := range []string{
			"1~2",
			"sample~value",
			"........",
			"A.B",
			"「会話です」",
			"【隅付き括弧】",
			"……三点リーダー",
			"ABCDEFGHIJKLMNOPQRSTUVWXYZ",
			"0123456789",
			"&quot;",
			"ＡＢＣＤＥＦＧ",
			"０１２３４５６７８９",
			"！＂＃＄％＆＇",
			"≠6 ≤7 ≥8",
			"→ ← ↑ ↓",
			"<ruby>",
			"<a href=",
		} {
			if !strings.Contains(patternBody, pattern) {
				t.Fatalf("correction fixture pattern body does not contain %q: %s", pattern, patternBody)
			}
		}

		storyBody := work.Episodes[1].Element.Body
		for _, pattern := range []string{
			"レベル1~2",
			"銀貨5~8枚",
			"........まあ",
			"&quot;KEEP OUT!&quot;",
			"GATE-A17 / OPEN? [Y/N]",
			"ＧＡＴＥ－Ａ１７【封鎖中】",
			"ERROR:CODE-02",
			"ＩＤ：ＡＢＣ－１２３",
			"「いるよ...ずっと、ここに」",
			"歯車A.B",
			"右へ→",
			"<ruby>暁鐘",
		} {
			if !strings.Contains(storyBody, pattern) {
				t.Fatalf("correction fixture story body does not contain %q: %s", pattern, storyBody)
			}
		}
		return
	}

	t.Fatal("reader correction verification fixture was not found")
}
