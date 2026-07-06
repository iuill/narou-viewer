package library

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildReaderSectionBlocksSplitsNarouParagraphs(t *testing.T) {
	source := "<p id=\"L1\">昔々あるところに。</p>\n<p id=\"L2\"><br/></p>\n<p id=\"L3\">娘がいました。</p>"
	blocks := buildReaderSectionBlocks(source, "body")
	if len(blocks) != 3 {
		t.Fatalf("blocks = %d, want 3: %+v", len(blocks), blocks)
	}
	for index, block := range blocks {
		if block.Type != "paragraph" || block.Section != "body" {
			t.Fatalf("block %d should be a body paragraph: %+v", index, block)
		}
	}
	if blocks[0].Inlines[0].Type != "text" || blocks[0].Inlines[0].Text != "昔々あるところに。" {
		t.Fatalf("unexpected first paragraph: %+v", blocks[0].Inlines)
	}
	if len(blocks[1].Inlines) != 1 || blocks[1].Inlines[0].Type != "lineBreak" {
		t.Fatalf("blank paragraph should be a single lineBreak: %+v", blocks[1].Inlines)
	}
}

func TestBuildReaderSectionBlocksParsesInlineTokens(t *testing.T) {
	source := `<p><ruby>漢字<rp>(</rp><rt>かんじ</rt><rp>)</rp></ruby>を<span class="vertical-composition">12</span>月に<a href="https://example.com/x">読む</a><br>続き&#34;</p>`
	blocks := buildReaderSectionBlocks(source, "body")
	if len(blocks) != 1 || blocks[0].Type != "paragraph" {
		t.Fatalf("unexpected blocks: %+v", blocks)
	}
	inlines := blocks[0].Inlines
	if len(inlines) != 7 {
		t.Fatalf("inlines = %d, want 7: %+v", len(inlines), inlines)
	}
	if inlines[0].Type != "ruby" || inlines[0].Text != "漢字" || inlines[0].Ruby != "かんじ" {
		t.Fatalf("unexpected ruby token: %+v", inlines[0])
	}
	if inlines[1].Type != "text" || inlines[1].Text != "を" {
		t.Fatalf("unexpected text token: %+v", inlines[1])
	}
	if inlines[2].Type != "tcy" || inlines[2].Text != "12" {
		t.Fatalf("unexpected tcy token: %+v", inlines[2])
	}
	if inlines[4].Type != "link" || inlines[4].Href == nil || *inlines[4].Href != "https://example.com/x" ||
		len(inlines[4].Children) != 1 || inlines[4].Children[0].Text != "読む" {
		t.Fatalf("unexpected link token: %+v", inlines[4])
	}
	if inlines[5].Type != "lineBreak" || inlines[6].Text != `続き"` {
		t.Fatalf("unexpected tail tokens: %+v", inlines[5:])
	}
}

func TestBuildReaderSectionBlocksRejectsUnsafeLinkScheme(t *testing.T) {
	blocks := buildReaderSectionBlocks(`<p><a href="javascript:alert(1)">リンク</a></p>`, "body")
	if len(blocks) != 1 || blocks[0].Type != "paragraph" {
		t.Fatalf("unexpected blocks: %+v", blocks)
	}
	link := blocks[0].Inlines[0]
	if link.Type != "link" || link.Href != nil {
		t.Fatalf("unsafe scheme should produce a null href: %+v", link)
	}
}

func TestBuildReaderSectionBlocksDecodesNumericAndNamedEntities(t *testing.T) {
	blocks := buildReaderSectionBlocks(`<p>&#34;&#x22;&apos;&nbsp;</p>`, "body")
	if len(blocks) != 1 || blocks[0].Type != "paragraph" {
		t.Fatalf("unexpected blocks: %+v", blocks)
	}
	if got := blocks[0].Inlines[0].Text; got != `""' ` {
		t.Fatalf("decoded text = %q", got)
	}
}

func TestBuildReaderSectionBlocksKeepsDecodedTagsAsText(t *testing.T) {
	blocks := buildReaderSectionBlocks(`<p>&#60;script&#62;alert(1)&#60;/script&#62;</p>`, "body")
	if len(blocks) != 1 || blocks[0].Type != "paragraph" {
		t.Fatalf("unexpected blocks: %+v", blocks)
	}
	if got := blocks[0].Inlines[0].Text; got != "<script>alert(1)</script>" {
		t.Fatalf("decoded text should remain a text token: %+v", blocks[0].Inlines)
	}
}

func TestBuildReaderSectionBlocksRejectsEntityEncodedUnsafeLinkScheme(t *testing.T) {
	blocks := buildReaderSectionBlocks(`<p><a href="java&#115;cript:alert(1)">リンク</a></p>`, "body")
	if len(blocks) != 1 || blocks[0].Type != "paragraph" {
		t.Fatalf("unexpected blocks: %+v", blocks)
	}
	link := blocks[0].Inlines[0]
	if link.Type != "link" || link.Href != nil {
		t.Fatalf("entity-encoded unsafe scheme should be rejected: %+v", link)
	}
}

func TestBuildReaderSectionBlocksExtractsImages(t *testing.T) {
	source := `<p><img src="assets/episodes/1/pic.jpg" alt="挿絵" width="100" height="50"></p>` +
		`<a href="https://example.com/orig" title="原寸"><img src="assets/episodes/1/pic2.jpg"></a>`
	blocks := buildReaderSectionBlocks(source, "body")
	if len(blocks) != 2 {
		t.Fatalf("blocks = %d, want 2: %+v", len(blocks), blocks)
	}
	first := blocks[0]
	if first.Type != "image" || first.Src != "assets/episodes/1/pic.jpg" ||
		first.Alt == nil || *first.Alt != "挿絵" ||
		first.Width == nil || *first.Width != 100 || first.Height == nil || *first.Height != 50 ||
		first.OriginalURL != nil {
		t.Fatalf("unexpected paragraph image block: %+v", first)
	}
	second := blocks[1]
	if second.Type != "image" || second.Src != "assets/episodes/1/pic2.jpg" ||
		second.OriginalURL == nil || *second.OriginalURL != "https://example.com/orig" ||
		second.Title == nil || *second.Title != "原寸" {
		t.Fatalf("unexpected linked image block: %+v", second)
	}
}

func TestBuildReaderSectionBlocksUnwrapsContainersAndDropsScripts(t *testing.T) {
	source := `<div><p>本文</p><script>alert(1)</script></div>`
	blocks := buildReaderSectionBlocks(source, "body")
	if len(blocks) != 1 || blocks[0].Type != "paragraph" || blocks[0].Inlines[0].Text != "本文" {
		t.Fatalf("unexpected blocks: %+v", blocks)
	}
}

func TestBuildReaderSectionBlocksFallsBackToSanitizedHTML(t *testing.T) {
	source := `<blockquote onclick="alert(1)" class="quote">引用&amp;文</blockquote>`
	blocks := buildReaderSectionBlocks(source, "body")
	if len(blocks) != 1 || blocks[0].Type != "html" {
		t.Fatalf("unexpected blocks: %+v", blocks)
	}
	if blocks[0].HTML != `<blockquote class="quote">引用&amp;文</blockquote>` {
		t.Fatalf("unexpected sanitized html: %q", blocks[0].HTML)
	}
	if blocks[0].PlainText != "引用&文" {
		t.Fatalf("unexpected plain text: %q", blocks[0].PlainText)
	}
}

func TestBuildReaderSectionBlocksSanitizesHTMLBlockContract(t *testing.T) {
	source := `<blockquote class="quote" data-x="drop" onclick="alert(1)">` +
		`<a href="https://example.com/path" title="参照" target="_blank">安全</a>` +
		`<a href="javascript:alert(1)">危険</a>` +
		`<img src="https://example.com/cover.jpg" alt="表紙" width="120" height="80" onerror="alert(1)">` +
		`<img src="mailto:cover@example.com">` +
		`<img src="data:image/png;base64,abc">` +
		`<script>alert(1)</script>` +
		`<unknown><strong>本文</strong></unknown>` +
		`</blockquote>`
	blocks := buildReaderSectionBlocks(source, "body")
	if len(blocks) != 1 || blocks[0].Type != "html" {
		t.Fatalf("unexpected blocks: %+v", blocks)
	}
	wantHTML := `<blockquote class="quote"><a href="https://example.com/path" title="参照">安全</a><a>危険</a><img src="https://example.com/cover.jpg" alt="表紙" width="120" height="80"><img><img><strong>本文</strong></blockquote>`
	if blocks[0].HTML != wantHTML {
		t.Fatalf("sanitized html = %q, want %q", blocks[0].HTML, wantHTML)
	}
	if strings.Contains(blocks[0].HTML, "onclick") || strings.Contains(blocks[0].HTML, "data-x") || strings.Contains(blocks[0].HTML, "javascript:") || strings.Contains(blocks[0].HTML, "<script") {
		t.Fatalf("sanitized html kept unsafe content: %q", blocks[0].HTML)
	}
	if blocks[0].PlainText != "安全危険本文" {
		t.Fatalf("plain text = %q", blocks[0].PlainText)
	}
}

func TestBuildReaderSectionBlocksPreservesHorizontalRules(t *testing.T) {
	topLevel := buildReaderSectionBlocks(`<p>前半</p><hr><p>後半</p>`, "body")
	if len(topLevel) != 3 || topLevel[1].Type != "html" || topLevel[1].HTML != "<hr>" {
		t.Fatalf("top-level hr should be preserved as an html block: %+v", topLevel)
	}
	inline := buildReaderSectionBlocks(`<p>場面転換<hr>続き</p>`, "body")
	if len(inline) != 1 || inline[0].Type != "html" || !strings.Contains(inline[0].HTML, "<hr>") {
		t.Fatalf("inline hr should fall back to an html block keeping the hr: %+v", inline)
	}
}

func TestReaderDocumentInsertsSectionSeparators(t *testing.T) {
	document := readerDocument("", "", "題名", map[string]string{
		"introduction": "<p>前書き</p>",
		"body":         "<p>本文</p>",
		"postscript":   "<p>後書き</p>",
	})
	kinds := []string{}
	for _, block := range document.Blocks {
		label := block.Type
		if block.Type == "html" && strings.Contains(block.HTML, "reader-section-separator") {
			label = "separator:" + block.Section
		}
		kinds = append(kinds, label)
	}
	want := []string{"title", "paragraph", "separator:body", "paragraph", "separator:postscript", "paragraph"}
	if strings.Join(kinds, ",") != strings.Join(want, ",") {
		t.Fatalf("unexpected block sequence: %v", kinds)
	}

	bodyOnly := readerDocument("", "", "題名", map[string]string{"body": "<p>本文</p>"})
	for _, block := range bodyOnly.Blocks {
		if strings.Contains(block.HTML, "reader-section-separator") {
			t.Fatalf("single section should not get a separator: %+v", bodyOnly.Blocks)
		}
	}
}

func TestReaderDocumentJSONShapeMatchesTSContract(t *testing.T) {
	document := readerDocument("第一章", "", "題名", map[string]string{
		"body": `<p>本文<br><a href="javascript:x">リンク</a></p><p><img src="assets/p.png"></p><blockquote>引用</blockquote>`,
	})
	raw, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("marshal reader document: %v", err)
	}
	var decoded struct {
		Version int              `json:"version"`
		Blocks  []map[string]any `json:"blocks"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal reader document: %v", err)
	}
	expectKeys := func(block map[string]any, keys ...string) {
		t.Helper()
		if len(block) != len(keys) {
			t.Fatalf("unexpected key set: %+v, want %v", block, keys)
		}
		for _, key := range keys {
			if _, ok := block[key]; !ok {
				t.Fatalf("missing key %q: %+v", key, block)
			}
		}
	}
	expectKeys(decoded.Blocks[0], "type", "role", "text")
	expectKeys(decoded.Blocks[1], "type", "text")
	paragraph := decoded.Blocks[2]
	expectKeys(paragraph, "type", "section", "inlines")
	inlines := paragraph["inlines"].([]any)
	expectKeys(inlines[0].(map[string]any), "type", "text")
	expectKeys(inlines[1].(map[string]any), "type")
	link := inlines[2].(map[string]any)
	expectKeys(link, "type", "href", "children")
	if link["href"] != nil {
		t.Fatalf("unsafe link href should serialize as null: %+v", link)
	}
	if _, ok := link["children"].([]any); !ok {
		t.Fatalf("link children should serialize as an array: %+v", link)
	}
	image := decoded.Blocks[3]
	expectKeys(image, "type", "section", "src", "alt", "originalUrl", "title", "width", "height")
	if image["alt"] != nil || image["width"] != nil {
		t.Fatalf("absent image metadata should serialize as null: %+v", image)
	}
	htmlBlock := decoded.Blocks[4]
	expectKeys(htmlBlock, "type", "section", "html", "plainText")
}
