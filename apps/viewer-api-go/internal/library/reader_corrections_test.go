package library

import "testing"

func TestApplyReaderCorrectionsNormalizesDoubleQuotesWithoutChangingLength(t *testing.T) {
	document := ReaderDocument{
		Version: 1,
		Blocks: []ReaderBlock{
			{Type: "title", Text: `第1話 "引用"`},
			{
				Type:    "paragraph",
				Section: "body",
				Inlines: []ReaderInline{
					{Type: "text", Text: `「あるいは、"前世"を何一つ活かせず」`},
					{Type: "ruby", Text: `“弱点”`, Ruby: "ウィークポイント"},
					{Type: "tcy", Text: `"!?`},
					{Type: "lineBreak"},
					{Type: "link", Children: []ReaderInline{{Type: "text", Text: `"リンク"`}}},
				},
			},
		},
	}

	corrected := ApplyReaderCorrections(document, ReaderCorrectionSettings{QuoteNormalization: true})

	if corrected.Blocks[0].Text != "第1話 〝引用〟" {
		t.Fatalf("unexpected title correction: %q", corrected.Blocks[0].Text)
	}
	inlines := corrected.Blocks[1].Inlines
	if inlines[0].Text != "「あるいは、〝前世〟を何一つ活かせず」" {
		t.Fatalf("unexpected text correction: %q", inlines[0].Text)
	}
	if inlines[1].Text != "〝弱点〟" {
		t.Fatalf("unexpected ruby base correction: %q", inlines[1].Text)
	}
	if inlines[2].Text != "〝!?" {
		t.Fatalf("unexpected tcy correction: %q", inlines[2].Text)
	}
	if inlines[4].Children[0].Text != "〝リンク〟" {
		t.Fatalf("unexpected link child correction: %q", inlines[4].Children[0].Text)
	}
	if countReaderRunes(document) != countReaderRunes(corrected) {
		t.Fatalf("correction should keep reader text length: before=%d after=%d", countReaderRunes(document), countReaderRunes(corrected))
	}
}

func TestApplyReaderCorrectionsCarriesQuoteStateAcrossInlineTokens(t *testing.T) {
	document := ReaderDocument{
		Version: 1,
		Blocks: []ReaderBlock{{
			Type:    "paragraph",
			Section: "body",
			Inlines: []ReaderInline{
				{Type: "text", Text: `"前`},
				{Type: "ruby", Text: `世`, Ruby: "ぜ"},
				{Type: "text", Text: `"`},
				{Type: "lineBreak"},
				{Type: "text", Text: `"リンク`},
				{Type: "link", Children: []ReaderInline{{Type: "text", Text: `先"`}}},
			},
		}},
	}

	corrected := ApplyReaderCorrections(document, ReaderCorrectionSettings{QuoteNormalization: true})
	inlines := corrected.Blocks[0].Inlines

	if inlines[0].Text != `〝前` || inlines[2].Text != `〟` {
		t.Fatalf("quote state should span ruby boundary: %+v", inlines)
	}
	if inlines[4].Text != `〝リンク` || inlines[5].Children[0].Text != `先〟` {
		t.Fatalf("quote state should span link boundary: %+v", inlines)
	}
}

func TestApplyReaderCorrectionsNormalizesHTMLBlockText(t *testing.T) {
	document := ReaderDocument{
		Version: 1,
		Blocks: []ReaderBlock{{
			Type:      "html",
			Section:   "body",
			HTML:      `<blockquote title="keep &quot;attr&quot;">"引用"<br>"続き"</blockquote>`,
			PlainText: `"引用"` + "\n" + `"続き"`,
		}},
	}

	corrected := ApplyReaderCorrections(document, ReaderCorrectionSettings{QuoteNormalization: true})
	block := corrected.Blocks[0]

	if block.HTML != `<blockquote title="keep &quot;attr&quot;">〝引用〟<br>〝続き〟</blockquote>` {
		t.Fatalf("unexpected html correction: %q", block.HTML)
	}
	if block.PlainText != "〝引用〟\n〝続き〟" {
		t.Fatalf("unexpected plain text correction: %q", block.PlainText)
	}
	if countReaderRunes(document) != countReaderRunes(corrected) {
		t.Fatalf("html correction should keep reader text length: before=%d after=%d", countReaderRunes(document), countReaderRunes(corrected))
	}
}

func TestApplyReaderCorrectionsNormalizesConsecutiveHyphensWithoutChangingLength(t *testing.T) {
	document := ReaderDocument{
		Version: 1,
		Blocks: []ReaderBlock{
			{Type: "title", Text: "第1話 ---- 開始"},
			{
				Type:    "paragraph",
				Section: "body",
				Inlines: []ReaderInline{
					{Type: "text", Text: "　----王都から"},
					{Type: "ruby", Text: "錬金術----2つ", Ruby: "れんきんじゅつ"},
					{Type: "text", Text: "単独-ハイフンは残す"},
				},
			},
			{
				Type:      "html",
				Section:   "body",
				HTML:      `<p title="keep----attr">本文----だけ</p>`,
				PlainText: `本文----だけ`,
			},
		},
	}

	corrected := ApplyReaderCorrections(document, ReaderCorrectionSettings{HyphenDashNormalization: true})

	if corrected.Blocks[0].Text != "第1話 ―――― 開始" {
		t.Fatalf("unexpected title correction: %q", corrected.Blocks[0].Text)
	}
	inlines := corrected.Blocks[1].Inlines
	if inlines[0].Text != "　――――王都から" {
		t.Fatalf("unexpected text correction: %q", inlines[0].Text)
	}
	if inlines[1].Text != "錬金術――――2つ" {
		t.Fatalf("unexpected ruby base correction: %q", inlines[1].Text)
	}
	if inlines[2].Text != "単独-ハイフンは残す" {
		t.Fatalf("single hyphen should stay unchanged: %q", inlines[2].Text)
	}
	if corrected.Blocks[2].HTML != `<p title="keep----attr">本文――――だけ</p>` {
		t.Fatalf("unexpected html correction: %q", corrected.Blocks[2].HTML)
	}
	if corrected.Blocks[2].PlainText != "本文――――だけ" {
		t.Fatalf("unexpected plain text correction: %q", corrected.Blocks[2].PlainText)
	}
	if countReaderRunes(document) != countReaderRunes(corrected) {
		t.Fatalf("correction should keep reader text length: before=%d after=%d", countReaderRunes(document), countReaderRunes(corrected))
	}
}

func TestApplyReaderCorrectionsNormalizesHTMLHyphensAcrossInlineNodeBoundaries(t *testing.T) {
	document := ReaderDocument{
		Version: 1,
		Blocks: []ReaderBlock{{
			Type:      "html",
			Section:   "body",
			HTML:      `<p><a href="/">-</a><em>-</em><strong>-</strong><span>-</span><ruby>-<rt>よみ</rt></ruby>終わり</p>`,
			PlainText: `-----よみ終わり`,
		}},
	}

	corrected := ApplyReaderCorrections(document, ReaderCorrectionSettings{HyphenDashNormalization: true})
	block := corrected.Blocks[0]

	if block.HTML != `<p><a href="/">―</a><em>―</em><strong>―</strong><span>―</span><ruby>―<rt>よみ</rt></ruby>終わり</p>` {
		t.Fatalf("unexpected html correction: %q", block.HTML)
	}
	if block.PlainText != `―――――よみ終わり` {
		t.Fatalf("unexpected plain text correction: %q", block.PlainText)
	}
}

func TestApplyReaderCorrectionsDoesNotNormalizeHTMLHyphensAcrossBlockBoundaries(t *testing.T) {
	document := ReaderDocument{
		Version: 1,
		Blocks: []ReaderBlock{{
			Type:      "html",
			Section:   "body",
			HTML:      `<p>-</p><p>-</p><ul><li>-</li><li>-</li></ul><blockquote>-</blockquote><p>-</p>`,
			PlainText: `- - - - - -`,
		}},
	}

	corrected := ApplyReaderCorrections(document, ReaderCorrectionSettings{HyphenDashNormalization: true})
	block := corrected.Blocks[0]

	if block.HTML != `<p>-</p><p>-</p><ul><li>-</li><li>-</li></ul><blockquote>-</blockquote><p>-</p>` {
		t.Fatalf("block-separated hyphens should stay unchanged: %q", block.HTML)
	}
	if block.PlainText != `- - - - - -` {
		t.Fatalf("unexpected plain text correction: %q", block.PlainText)
	}
}

func TestApplyReaderCorrectionsNormalizesParenthesesWithoutChangingLength(t *testing.T) {
	document := ReaderDocument{
		Version: 1,
		Blocks: []ReaderBlock{
			{Type: "title", Text: "第1話 (前)"},
			{
				Type:    "paragraph",
				Section: "body",
				Inlines: []ReaderInline{
					{Type: "text", Text: "胸(私に胸はない。)を張る。"},
					{Type: "ruby", Text: "相方(ゴーレム)", Ruby: "あいかた"},
				},
			},
			{
				Type:      "html",
				Section:   "body",
				HTML:      `<p title="keep(attr)">本文(だけ)</p>`,
				PlainText: `本文(だけ)`,
			},
		},
	}

	corrected := ApplyReaderCorrections(document, ReaderCorrectionSettings{ParenthesisNormalization: true})

	if corrected.Blocks[0].Text != "第1話 （前）" {
		t.Fatalf("unexpected title correction: %q", corrected.Blocks[0].Text)
	}
	inlines := corrected.Blocks[1].Inlines
	if inlines[0].Text != "胸（私に胸はない。）を張る。" {
		t.Fatalf("unexpected text correction: %q", inlines[0].Text)
	}
	if inlines[1].Text != "相方（ゴーレム）" {
		t.Fatalf("unexpected ruby base correction: %q", inlines[1].Text)
	}
	if corrected.Blocks[2].HTML != `<p title="keep(attr)">本文（だけ）</p>` {
		t.Fatalf("unexpected html correction: %q", corrected.Blocks[2].HTML)
	}
	if corrected.Blocks[2].PlainText != "本文（だけ）" {
		t.Fatalf("unexpected plain text correction: %q", corrected.Blocks[2].PlainText)
	}
	if countReaderRunes(document) != countReaderRunes(corrected) {
		t.Fatalf("correction should keep reader text length: before=%d after=%d", countReaderRunes(document), countReaderRunes(corrected))
	}
}

func TestApplyReaderCorrectionsNormalizesHalfwidthAlnumPunctuationOnlyInBody(t *testing.T) {
	document := ReaderDocument{
		Version: 1,
		Blocks: []ReaderBlock{
			{Type: "title", Text: "第1話 Alpha!? 123"},
			{
				Type:    "paragraph",
				Section: "body",
				Inlines: []ReaderInline{
					{Type: "text", Text: "Alpha!? 123"},
					{Type: "ruby", Text: "Beta9?", Ruby: "ベータ"},
					{Type: "text", Text: " 全角ＡＢＣ！？は維持"},
					{Type: "tcy", Text: "12"},
					{Type: "link", Href: testStringPtr("https://example.com/API"), Children: []ReaderInline{{Type: "text", Text: "URL"}}},
					{Type: "text", Text: " HTTP/2"},
				},
			},
			{
				Type:      "html",
				Section:   "body",
				HTML:      `<h2>HTML Heading 7?</h2><p title="keep Alpha!? 123"><ruby>API<rt>API</rt></ruby> Gamma!? 456 <code>HTTP/2</code></p>`,
				PlainText: `HTML Heading 7?APIAPI Gamma!? 456 HTTP/2`,
			},
		},
	}

	corrected := ApplyReaderCorrections(document, ReaderCorrectionSettings{HalfwidthAlnumPunctuationNormalization: true})

	if corrected.Blocks[0].Text != "第1話 Alpha!? 123" {
		t.Fatalf("title should not receive halfwidth alnum punctuation correction: %q", corrected.Blocks[0].Text)
	}
	inlines := corrected.Blocks[1].Inlines
	if inlines[0].Text != "Ａｌｐｈａ！？ １２３" {
		t.Fatalf("unexpected text correction: %q", inlines[0].Text)
	}
	if inlines[1].Text != "Ｂｅｔａ９？" {
		t.Fatalf("unexpected ruby base correction: %q", inlines[1].Text)
	}
	if inlines[2].Text != " 全角ＡＢＣ！？は維持" {
		t.Fatalf("unexpected fullwidth text correction: %q", inlines[2].Text)
	}
	if inlines[3].Text != "１２" {
		t.Fatalf("unexpected tcy correction: %q", inlines[3].Text)
	}
	if inlines[4].Children[0].Text != "ＵＲＬ" {
		t.Fatalf("unexpected link child correction: %q", inlines[4].Children[0].Text)
	}
	if inlines[5].Text != " ＨＴＴＰ/２" {
		t.Fatalf("unexpected code-like text correction: %q", inlines[5].Text)
	}
	if inlines[1].Ruby != "ベータ" {
		t.Fatalf("ruby reading should not be corrected: %q", inlines[1].Ruby)
	}
	if corrected.Blocks[2].HTML != `ＨＴＭＬ Ｈｅａｄｉｎｇ ７？<p title="keep Alpha!? 123"><ruby>ＡＰＩ<rt>ＡＰＩ</rt></ruby> Ｇａｍｍａ！？ ４５６ <code>ＨＴＴＰ/２</code></p>` {
		t.Fatalf("unexpected html correction: %q", corrected.Blocks[2].HTML)
	}
	if corrected.Blocks[2].PlainText != "ＨＴＭＬ Ｈｅａｄｉｎｇ ７？ＡＰＩＡＰＩ Ｇａｍｍａ！？ ４５６ ＨＴＴＰ/２" {
		t.Fatalf("unexpected plain text correction: %q", corrected.Blocks[2].PlainText)
	}
	if countReaderRunes(document) != countReaderRunes(corrected) {
		t.Fatalf("correction should keep reader text length: before=%d after=%d", countReaderRunes(document), countReaderRunes(corrected))
	}
}

func TestApplyReaderCorrectionsSettingsAreIndependent(t *testing.T) {
	tests := []struct {
		name     string
		settings ReaderCorrectionSettings
		want     string
	}{
		{name: "none", settings: ReaderCorrectionSettings{}, want: `"前世"----(胸) Alpha!? 123`},
		{name: "quote", settings: ReaderCorrectionSettings{QuoteNormalization: true}, want: `〝前世〟----(胸) Alpha!? 123`},
		{name: "hyphen", settings: ReaderCorrectionSettings{HyphenDashNormalization: true}, want: `"前世"――――(胸) Alpha!? 123`},
		{name: "parenthesis", settings: ReaderCorrectionSettings{ParenthesisNormalization: true}, want: `"前世"----（胸） Alpha!? 123`},
		{name: "halfwidth", settings: ReaderCorrectionSettings{HalfwidthAlnumPunctuationNormalization: true}, want: `"前世"----(胸) Ａｌｐｈａ！？ １２３`},
		{name: "quote hyphen", settings: ReaderCorrectionSettings{QuoteNormalization: true, HyphenDashNormalization: true}, want: `〝前世〟――――(胸) Alpha!? 123`},
		{name: "quote parenthesis", settings: ReaderCorrectionSettings{QuoteNormalization: true, ParenthesisNormalization: true}, want: `〝前世〟----（胸） Alpha!? 123`},
		{name: "quote halfwidth", settings: ReaderCorrectionSettings{QuoteNormalization: true, HalfwidthAlnumPunctuationNormalization: true}, want: `〝前世〟----(胸) Ａｌｐｈａ！？ １２３`},
		{name: "hyphen parenthesis", settings: ReaderCorrectionSettings{HyphenDashNormalization: true, ParenthesisNormalization: true}, want: `"前世"――――（胸） Alpha!? 123`},
		{name: "hyphen halfwidth", settings: ReaderCorrectionSettings{HyphenDashNormalization: true, HalfwidthAlnumPunctuationNormalization: true}, want: `"前世"――――(胸) Ａｌｐｈａ！？ １２３`},
		{name: "parenthesis halfwidth", settings: ReaderCorrectionSettings{ParenthesisNormalization: true, HalfwidthAlnumPunctuationNormalization: true}, want: `"前世"----（胸） Ａｌｐｈａ！？ １２３`},
		{name: "quote hyphen parenthesis", settings: ReaderCorrectionSettings{QuoteNormalization: true, HyphenDashNormalization: true, ParenthesisNormalization: true}, want: `〝前世〟――――（胸） Alpha!? 123`},
		{name: "quote hyphen halfwidth", settings: ReaderCorrectionSettings{QuoteNormalization: true, HyphenDashNormalization: true, HalfwidthAlnumPunctuationNormalization: true}, want: `〝前世〟――――(胸) Ａｌｐｈａ！？ １２３`},
		{name: "quote parenthesis halfwidth", settings: ReaderCorrectionSettings{QuoteNormalization: true, ParenthesisNormalization: true, HalfwidthAlnumPunctuationNormalization: true}, want: `〝前世〟----（胸） Ａｌｐｈａ！？ １２３`},
		{name: "hyphen parenthesis halfwidth", settings: ReaderCorrectionSettings{HyphenDashNormalization: true, ParenthesisNormalization: true, HalfwidthAlnumPunctuationNormalization: true}, want: `"前世"――――（胸） Ａｌｐｈａ！？ １２３`},
		{name: "all", settings: ReaderCorrectionSettings{QuoteNormalization: true, HyphenDashNormalization: true, ParenthesisNormalization: true, HalfwidthAlnumPunctuationNormalization: true}, want: `〝前世〟――――（胸） Ａｌｐｈａ！？ １２３`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			document := ReaderDocument{
				Version: 1,
				Blocks: []ReaderBlock{{
					Type:    "paragraph",
					Section: "body",
					Inlines: []ReaderInline{{Type: "text", Text: `"前世"----(胸) Alpha!? 123`}},
				}},
			}

			corrected := ApplyReaderCorrections(document, tt.settings)
			if got := corrected.Blocks[0].Inlines[0].Text; got != tt.want {
				t.Fatalf("unexpected corrected text: got=%q want=%q", got, tt.want)
			}
		})
	}
}

func TestApplyReaderCorrectionsDisabledKeepsDocument(t *testing.T) {
	document := ReaderDocument{
		Version: 1,
		Blocks:  []ReaderBlock{{Type: "paragraph", Section: "body", Inlines: []ReaderInline{{Type: "text", Text: `"前世"`}}}},
	}

	corrected := ApplyReaderCorrections(document, ReaderCorrectionSettings{})

	if corrected.Blocks[0].Inlines[0].Text != `"前世"` {
		t.Fatalf("disabled correction should not change text: %+v", corrected)
	}
}

func countReaderRunes(document ReaderDocument) int {
	count := 0
	for _, block := range document.Blocks {
		if block.Type == "html" {
			count += len([]rune(block.PlainText))
			continue
		}
		count += len([]rune(block.Text))
		count += countReaderInlineRunes(block.Inlines)
	}
	return count
}

func testStringPtr(value string) *string {
	return &value
}

func countReaderInlineRunes(tokens []ReaderInline) int {
	count := 0
	for _, token := range tokens {
		count += len([]rune(token.Text))
		count += countReaderInlineRunes(token.Children)
	}
	return count
}
