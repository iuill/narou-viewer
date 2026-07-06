package library

import (
	"strings"
	"testing"
)

func TestBuildReaderSectionBlocksHandlesBareTextAndComments(t *testing.T) {
	source := "<!-- comment -->地の文1行目\n2行目<br><p>段落</p>"
	blocks := buildReaderSectionBlocks(source, "body")
	if len(blocks) != 3 {
		t.Fatalf("blocks = %d, want 3: %+v", len(blocks), blocks)
	}
	bare := blocks[0]
	if bare.Type != "paragraph" || len(bare.Inlines) != 3 ||
		bare.Inlines[0].Text != "地の文1行目" || bare.Inlines[1].Type != "lineBreak" || bare.Inlines[2].Text != "2行目" {
		t.Fatalf("bare text should become a paragraph with line breaks: %+v", bare)
	}
	if blocks[1].Type != "paragraph" || blocks[1].Inlines[0].Type != "lineBreak" {
		t.Fatalf("top-level br should become a lineBreak paragraph: %+v", blocks[1])
	}
}

func TestBuildReaderSectionBlocksEmptyParagraphBecomesLineBreak(t *testing.T) {
	blocks := buildReaderSectionBlocks("<p></p><p>  </p>", "body")
	if len(blocks) != 2 {
		t.Fatalf("blocks = %d, want 2: %+v", len(blocks), blocks)
	}
	if blocks[0].Inlines[0].Type != "lineBreak" {
		t.Fatalf("empty paragraph should fall back to lineBreak: %+v", blocks[0])
	}
	if blocks[1].Inlines[0].Type != "text" || blocks[1].Inlines[0].Text != "  " {
		t.Fatalf("whitespace-only paragraph keeps the raw text token: %+v", blocks[1])
	}
}

func TestParseReaderInlineNodesRubyAndWrapperBranches(t *testing.T) {
	source := `<p><ruby>空<rt></rt></ruby>と<rt>裸rt</rt></p>`
	blocks := buildReaderSectionBlocks(source, "body")
	if len(blocks) != 1 || blocks[0].Type != "html" {
		t.Fatalf("bare rt should force the html fallback: %+v", blocks)
	}
	if !strings.Contains(blocks[0].HTML, "<ruby>空<rt></rt></ruby>") {
		t.Fatalf("ruby without reading should survive in fallback html: %q", blocks[0].HTML)
	}

	wrapped := buildReaderSectionBlocks(`<p><strong><span class="reader-tcy">12</span>強調</strong><span class="x">普通</span></p>`, "body")
	if len(wrapped) != 1 || wrapped[0].Type != "paragraph" {
		t.Fatalf("unexpected blocks: %+v", wrapped)
	}
	inlines := wrapped[0].Inlines
	if len(inlines) != 2 || inlines[0].Type != "tcy" || inlines[0].Text != "12" ||
		inlines[1].Text != "強調普通" {
		t.Fatalf("wrapper/tcy tokens mismatch: %+v", inlines)
	}

	styled := buildReaderSectionBlocks(`<p><span style="text-combine-upright: all">31</span>日</p>`, "body")
	if len(styled) != 1 || len(styled[0].Inlines) != 2 ||
		styled[0].Inlines[0].Type != "tcy" ||
		styled[0].Inlines[0].Text != "31" ||
		styled[0].Inlines[1].Text != "日" {
		t.Fatalf("style-based tcy should use raw style attrs before sanitize: %+v", styled[0].Inlines)
	}
}

func TestParseReaderInlineNodesUnknownElementRecursesChildren(t *testing.T) {
	blocks := buildReaderSectionBlocks(`<p><font color="red">色付き</font>と<span class="reader-tcy"></span>空tcy</p>`, "body")
	if len(blocks) != 1 || blocks[0].Type != "paragraph" {
		t.Fatalf("unexpected blocks: %+v", blocks)
	}
	if blocks[0].Inlines[0].Text != "色付きと空tcy" {
		t.Fatalf("unknown inline elements should flatten to text: %+v", blocks[0].Inlines)
	}
}

func TestParseReaderHTMLFragmentRecoversFromBrokenMarkup(t *testing.T) {
	source := `<p>開いたまま<span>入れ子</p></span></em><span/><p>次</p>`
	nodes := parseReaderHTMLFragment(source)
	if len(nodes) != 3 {
		t.Fatalf("nodes = %d, want 3: %+v", len(nodes), nodes)
	}
	if nodes[0].name != "p" || nodes[1].name != "span" || nodes[2].name != "p" {
		t.Fatalf("unexpected node names: %s %s %s", nodes[0].name, nodes[1].name, nodes[2].name)
	}
}

func TestParseReaderAttributesQuoteVariants(t *testing.T) {
	attrs := parseReaderAttributes(` a="1" b='2' c=3 d a="9"`)
	got := map[string]string{}
	for _, attr := range attrs {
		got[attr.key] = attr.value
	}
	if got["a"] != "9" || got["b"] != "2" || got["c"] != "3" {
		t.Fatalf("unexpected attributes: %+v", attrs)
	}
	if value, ok := got["d"]; !ok || value != "" {
		t.Fatalf("bare attribute should be kept with empty value: %+v", attrs)
	}
}

func TestSerializeReaderHTMLNodeBranches(t *testing.T) {
	source := `<section><blockquote cite="x" class="">引用<img src="p.png" alt="絵"><wbr></blockquote></section>`
	blocks := buildReaderSectionBlocks(source, "body")
	if len(blocks) != 1 || blocks[0].Type != "html" {
		t.Fatalf("unexpected blocks: %+v", blocks)
	}
	if blocks[0].HTML != `<blockquote class>引用<img src="p.png" alt="絵"></blockquote>` {
		t.Fatalf("unexpected serialized html: %q", blocks[0].HTML)
	}
}

func TestCollectReaderTextContentSkipsRubyAnnotations(t *testing.T) {
	nodes := parseReaderHTMLFragment(`<blockquote>前<br><rp>(</rp><style>p{}</style>後</blockquote>`)
	if got := collectReaderTextContent(nodes); got != "前\n後" {
		t.Fatalf("collectReaderTextContent = %q", got)
	}
}

func TestSanitizeReaderURLBranches(t *testing.T) {
	cases := map[string]string{
		"https://example.com/a": "https://example.com/a",
		"mailto:a@example.com":  "mailto:a@example.com",
		"//cdn.example.com/x":   "//cdn.example.com/x",
		"./relative":            "./relative",
		"?query=1":              "?query=1",
		"#fragment":             "#fragment",
		"path/without/scheme":   "path/without/scheme",
		"javascript:alert(1)":   "",
		"data:text/html,x":      "",
		"bad\x01url":            "",
		"   ":                   "",
	}
	for input, want := range cases {
		if got := sanitizeReaderURL(input, readerSafeLinkURLSchemes); got != want {
			t.Fatalf("sanitizeReaderURL link (%q) = %q, want %q", input, got, want)
		}
	}
	if got := sanitizeReaderURL("mailto:a@example.com", readerSafeImageURLSchemes); got != "" {
		t.Fatalf("sanitizeReaderURL image should reject mailto, got %q", got)
	}
	if got := sanitizeReaderURL("https://example.com/image.jpg", readerSafeImageURLSchemes); got != "https://example.com/image.jpg" {
		t.Fatalf("sanitizeReaderURL image should accept https, got %q", got)
	}
}

func TestParseReaderPositiveIntegerBranches(t *testing.T) {
	if parseReaderPositiveInteger("100") == nil || *parseReaderPositiveInteger("100") != 100 {
		t.Fatal("digits should parse")
	}
	for _, input := range []string{"", "0", "-1", "1.5", "abc", "999999999999999999999999"} {
		if parseReaderPositiveInteger(input) != nil {
			t.Fatalf("parseReaderPositiveInteger(%q) should be nil", input)
		}
	}
}

func TestExtractReaderImageInfoRejectsInvalidShapes(t *testing.T) {
	noSrc := buildReaderSectionBlocks(`<p><img alt="x"></p>`, "body")
	if len(noSrc) != 1 || noSrc[0].Type != "html" {
		t.Fatalf("img without src should fall back to html: %+v", noSrc)
	}
	multiChild := buildReaderSectionBlocks(`<a href="https://example.com"><img src="p.png">文</a>`, "body")
	if len(multiChild) != 1 || multiChild[0].Type != "html" {
		t.Fatalf("anchor with extra children should fall back to html: %+v", multiChild)
	}
	badImgSrc := buildReaderSectionBlocks(`<a href="https://example.com"><img src="javascript:x"></a>`, "body")
	if len(badImgSrc) != 1 || badImgSrc[0].Type != "html" {
		t.Fatalf("linked image with unsafe src should fall back to html: %+v", badImgSrc)
	}
}

func TestReaderDocumentSkipsEmptySections(t *testing.T) {
	document := readerDocument("章", "節", "題名", map[string]string{
		"introduction": "   ",
		"body":         "<script>x</script>",
		"postscript":   "<p>後書き</p>",
	})
	if len(document.Blocks) != 4 {
		t.Fatalf("blocks = %d, want 4: %+v", len(document.Blocks), document.Blocks)
	}
	for _, block := range document.Blocks {
		if strings.Contains(block.HTML, "reader-section-separator") {
			t.Fatalf("separator should not appear when only one section renders: %+v", document.Blocks)
		}
	}
	if document.Blocks[3].Type != "paragraph" || document.Blocks[3].Section != "postscript" {
		t.Fatalf("postscript should render: %+v", document.Blocks[3])
	}
}

func TestReaderInlineJSONDefaultBranch(t *testing.T) {
	block := ReaderBlock{Type: "unknown", Text: "x"}
	if _, err := block.MarshalJSON(); err != nil {
		t.Fatalf("unknown block marshal: %v", err)
	}
	inline := ReaderInline{Type: "unknown", Text: "x"}
	if _, err := inline.MarshalJSON(); err != nil {
		t.Fatalf("unknown inline marshal: %v", err)
	}
}
