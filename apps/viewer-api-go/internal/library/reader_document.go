package library

// ReaderDocument v1 is the server-side canonical reader document contract.
// Frontend pagination, visibility tracking, and speech operate on block / inline
// units, so section HTML is sanitized and split into paragraph, image, and HTML
// fallback blocks before it is returned by the API. The frontend still
// sanitizes rendered HTML as a defense-in-depth boundary for stored data.

import (
	"html"
	"regexp"
	"strconv"
	"strings"
)

type readerHTMLNode struct {
	isText   bool
	value    string
	name     string
	attrs    []readerHTMLAttr
	rawAttrs []readerHTMLAttr
	children []*readerHTMLNode
}

type readerHTMLAttr struct {
	key   string
	value string
}

var (
	readerVoidTags              = newReaderTagSet("area", "base", "br", "col", "embed", "hr", "img", "input", "link", "meta", "param", "source", "track", "wbr")
	readerTopLevelContainerTags = newReaderTagSet("article", "aside", "div", "footer", "header", "main", "nav")
	readerInlineWrapperTags     = newReaderTagSet("b", "code", "em", "i", "mark", "s", "small", "span", "strong", "sub", "sup", "u")
	readerSafeBlockTags         = newReaderTagSet("a", "blockquote", "br", "code", "em", "hr", "i", "img", "li", "mark", "ol", "p", "rp", "rt", "ruby", "s", "small", "span", "strong", "sub", "sup", "u", "ul")
	readerDropContentTags       = newReaderTagSet("script", "style", "iframe", "object", "embed", "link", "meta")
	readerSafeGlobalAttributes  = newReaderTagSet("class", "title", "lang", "dir")
	readerSafeLinkURLSchemes    = newReaderTagSet("http", "https", "mailto")
	readerSafeImageURLSchemes   = newReaderTagSet("http", "https")
)

var (
	readerHTMLTokenPattern   = regexp.MustCompile(`<!--[\s\S]*?-->|</?[^>]+>|[^<]+`)
	readerClosingTagPattern  = regexp.MustCompile(`^<\s*/\s*([a-zA-Z0-9:-]+)`)
	readerOpeningTagPattern  = regexp.MustCompile(`^<\s*([a-zA-Z0-9:-]+)([\s\S]*?)/?\s*>$`)
	readerSelfClosingPattern = regexp.MustCompile(`/\s*>$`)
	readerAttributePattern   = regexp.MustCompile("([^\\s\"'<>/=]+)(?:\\s*=\\s*(?:\"([^\"]*)\"|'([^']*)'|([^\\s\"'=<>`]+)))?")
	readerURLControlPattern  = regexp.MustCompile(`[\x00-\x1f\x7f]`)
	readerURLSchemePattern   = regexp.MustCompile(`^([a-zA-Z][a-zA-Z\d+.-]*):`)
	readerDigitsPattern      = regexp.MustCompile(`^\d+$`)
	readerTcyClassPattern    = regexp.MustCompile(`(?i)vertical-composition|reader-tcy`)
	readerTcyStylePattern    = regexp.MustCompile(`(?i)text-combine-upright`)
)

func newReaderTagSet(names ...string) map[string]bool {
	set := make(map[string]bool, len(names))
	for _, name := range names {
		set[name] = true
	}
	return set
}

func escapeReaderHTML(source string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&#39;",
	)
	return replacer.Replace(source)
}

func decodeReaderHTMLEntities(source string) string {
	return strings.ReplaceAll(html.UnescapeString(source), "\u00a0", " ")
}

func readerNodeAttr(node *readerHTMLNode, key string) string {
	for _, attr := range node.attrs {
		if attr.key == key {
			return attr.value
		}
	}
	return ""
}

func normalizeOptionalReaderText(value string) string {
	return strings.TrimSpace(value)
}

func parseReaderPositiveInteger(value string) *int {
	normalized := normalizeOptionalReaderText(value)
	if normalized == "" || !readerDigitsPattern.MatchString(normalized) {
		return nil
	}
	parsed, err := strconv.Atoi(normalized)
	if err != nil || parsed <= 0 {
		return nil
	}
	return &parsed
}

func sanitizeReaderURL(value string, safeSchemes map[string]bool) string {
	normalized := normalizeOptionalReaderText(value)
	if normalized == "" {
		return ""
	}
	if readerURLControlPattern.MatchString(normalized) {
		return ""
	}
	for _, prefix := range []string{"//", "/", "./", "../", "#", "?"} {
		if strings.HasPrefix(normalized, prefix) {
			return normalized
		}
	}
	scheme := readerURLSchemePattern.FindStringSubmatch(normalized)
	if scheme == nil {
		return normalized
	}
	if safeSchemes[strings.ToLower(scheme[1])] {
		return normalized
	}
	return ""
}

func sanitizeReaderAttributes(name string, attrs []readerHTMLAttr) []readerHTMLAttr {
	sanitized := []readerHTMLAttr{}
	for _, attr := range attrs {
		if strings.HasPrefix(attr.key, "on") {
			continue
		}
		if readerSafeGlobalAttributes[attr.key] {
			sanitized = append(sanitized, attr)
			continue
		}
		if name == "a" && attr.key == "href" {
			if href := sanitizeReaderURL(attr.value, readerSafeLinkURLSchemes); href != "" {
				sanitized = append(sanitized, readerHTMLAttr{key: attr.key, value: href})
			}
			continue
		}
		if name == "img" && attr.key == "src" {
			if src := sanitizeReaderURL(attr.value, readerSafeImageURLSchemes); src != "" {
				sanitized = append(sanitized, readerHTMLAttr{key: attr.key, value: src})
			}
			continue
		}
		if name == "img" && (attr.key == "alt" || attr.key == "width" || attr.key == "height") {
			sanitized = append(sanitized, attr)
		}
	}
	return sanitized
}

func parseReaderAttributes(source string) []readerHTMLAttr {
	attrs := []readerHTMLAttr{}
	seen := map[string]int{}
	for _, match := range readerAttributePattern.FindAllStringSubmatch(source, -1) {
		name := strings.ToLower(match[1])
		value := match[2]
		if value == "" {
			value = match[3]
		}
		if value == "" {
			value = match[4]
		}
		decoded := decodeReaderHTMLEntities(value)
		if index, ok := seen[name]; ok {
			attrs[index].value = decoded
			continue
		}
		seen[name] = len(attrs)
		attrs = append(attrs, readerHTMLAttr{key: name, value: decoded})
	}
	return attrs
}

func parseReaderHTMLFragment(source string) []*readerHTMLNode {
	root := &readerHTMLNode{name: "root"}
	stack := []*readerHTMLNode{root}
	for _, token := range readerHTMLTokenPattern.FindAllString(source, -1) {
		if strings.HasPrefix(token, "<!--") {
			continue
		}
		if strings.HasPrefix(token, "</") {
			closing := readerClosingTagPattern.FindStringSubmatch(token)
			if closing == nil {
				continue
			}
			closingTag := strings.ToLower(closing[1])
			for index := len(stack) - 1; index > 0; index-- {
				if stack[index].name == closingTag {
					stack = stack[:index]
					break
				}
			}
			continue
		}
		if strings.HasPrefix(token, "<") {
			opening := readerOpeningTagPattern.FindStringSubmatch(token)
			if opening == nil {
				continue
			}
			name := strings.ToLower(opening[1])
			rawAttrs := parseReaderAttributes(opening[2])
			element := &readerHTMLNode{
				name:     name,
				attrs:    sanitizeReaderAttributes(name, rawAttrs),
				rawAttrs: rawAttrs,
			}
			parent := stack[len(stack)-1]
			parent.children = append(parent.children, element)
			if !readerSelfClosingPattern.MatchString(token) && !readerVoidTags[name] {
				stack = append(stack, element)
			}
			continue
		}
		decoded := decodeReaderHTMLEntities(token)
		if decoded != "" {
			parent := stack[len(stack)-1]
			parent.children = append(parent.children, &readerHTMLNode{isText: true, value: decoded})
		}
	}
	return root.children
}

func serializeReaderHTMLNode(node *readerHTMLNode) string {
	if node.isText {
		return escapeReaderHTML(node.value)
	}
	if !readerSafeBlockTags[node.name] {
		if readerDropContentTags[node.name] {
			return ""
		}
		var builder strings.Builder
		for _, child := range node.children {
			builder.WriteString(serializeReaderHTMLNode(child))
		}
		return builder.String()
	}
	var attrBuilder strings.Builder
	for _, attr := range sanitizeReaderAttributes(node.name, node.attrs) {
		if attr.value != "" {
			attrBuilder.WriteString(" " + attr.key + `="` + escapeReaderHTML(attr.value) + `"`)
		} else {
			attrBuilder.WriteString(" " + attr.key)
		}
	}
	var contentBuilder strings.Builder
	for _, child := range node.children {
		contentBuilder.WriteString(serializeReaderHTMLNode(child))
	}
	if readerVoidTags[node.name] {
		return "<" + node.name + attrBuilder.String() + ">"
	}
	return "<" + node.name + attrBuilder.String() + ">" + contentBuilder.String() + "</" + node.name + ">"
}

func collectReaderTextContent(nodes []*readerHTMLNode) string {
	var builder strings.Builder
	for _, node := range nodes {
		if node.isText {
			builder.WriteString(node.value)
			continue
		}
		if node.name == "br" {
			builder.WriteString("\n")
			continue
		}
		if node.name == "rp" {
			continue
		}
		if readerDropContentTags[node.name] {
			continue
		}
		builder.WriteString(collectReaderTextContent(node.children))
	}
	return builder.String()
}

func mergeAdjacentReaderTextTokens(tokens []ReaderInline) []ReaderInline {
	merged := []ReaderInline{}
	for _, token := range tokens {
		if token.Type == "text" && len(merged) > 0 && merged[len(merged)-1].Type == "text" {
			merged[len(merged)-1].Text += token.Text
			continue
		}
		merged = append(merged, token)
	}
	return merged
}

func isReaderTcyNode(node *readerHTMLNode) bool {
	if className := readerNodeAttr(node, "class"); className != "" && readerTcyClassPattern.MatchString(className) {
		return true
	}
	if style := readerNodeAttr(node, "style"); style != "" && readerTcyStylePattern.MatchString(style) {
		return true
	}
	for _, attr := range node.rawAttrs {
		if attr.key == "style" && readerTcyStylePattern.MatchString(attr.value) {
			return true
		}
	}
	return false
}

func parseReaderInlineNodes(nodes []*readerHTMLNode) ([]ReaderInline, bool) {
	tokens := []ReaderInline{}
	unsupported := false
	for _, node := range nodes {
		if node.isText {
			if node.value != "" {
				tokens = append(tokens, ReaderInline{Type: "text", Text: node.value})
			}
			continue
		}
		if node.name == "br" {
			tokens = append(tokens, ReaderInline{Type: "lineBreak"})
			continue
		}
		if node.name == "rp" {
			continue
		}
		if node.name == "rt" {
			unsupported = true
			continue
		}
		if node.name == "ruby" {
			var rubyBuilder strings.Builder
			baseNodes := []*readerHTMLNode{}
			for _, child := range node.children {
				if !child.isText && child.name == "rt" {
					rubyBuilder.WriteString(collectReaderTextContent(child.children))
					continue
				}
				if !child.isText && child.name == "rp" {
					continue
				}
				baseNodes = append(baseNodes, child)
			}
			rubyText := normalizeOptionalReaderText(rubyBuilder.String())
			baseText := normalizeOptionalReaderText(collectReaderTextContent(baseNodes))
			if baseText != "" && rubyText != "" {
				tokens = append(tokens, ReaderInline{Type: "ruby", Text: baseText, Ruby: rubyText})
				continue
			}
			fallbackTokens, fallbackUnsupported := parseReaderInlineNodes(node.children)
			tokens = append(tokens, fallbackTokens...)
			unsupported = unsupported || fallbackUnsupported
			continue
		}
		if node.name == "a" {
			childTokens, childUnsupported := parseReaderInlineNodes(node.children)
			link := ReaderInline{Type: "link", Children: childTokens}
			if href := sanitizeReaderURL(readerNodeAttr(node, "href"), readerSafeLinkURLSchemes); href != "" {
				link.Href = &href
			}
			tokens = append(tokens, link)
			unsupported = unsupported || childUnsupported
			continue
		}
		if isReaderTcyNode(node) {
			if text := normalizeOptionalReaderText(collectReaderTextContent(node.children)); text != "" {
				tokens = append(tokens, ReaderInline{Type: "tcy", Text: text})
				continue
			}
		}
		if readerInlineWrapperTags[node.name] {
			childTokens, childUnsupported := parseReaderInlineNodes(node.children)
			tokens = append(tokens, childTokens...)
			unsupported = unsupported || childUnsupported
			continue
		}
		// inline の <hr> は区切りとして保持したいので、paragraph を不成立にして
		// html ブロックへフォールバックさせる。
		if node.name == "hr" {
			unsupported = true
			continue
		}
		if node.name == "img" || node.name == "p" || readerTopLevelContainerTags[node.name] {
			unsupported = true
			continue
		}
		childTokens, childUnsupported := parseReaderInlineNodes(node.children)
		tokens = append(tokens, childTokens...)
		unsupported = unsupported || childUnsupported
	}
	return mergeAdjacentReaderTextTokens(tokens), unsupported
}

type readerImageInfo struct {
	src         string
	alt         string
	originalURL string
	title       string
	width       *int
	height      *int
}

func extractReaderImageInfo(node *readerHTMLNode) *readerImageInfo {
	if node.isText {
		return nil
	}
	if node.name == "img" {
		src := sanitizeReaderURL(readerNodeAttr(node, "src"), readerSafeImageURLSchemes)
		if src == "" {
			return nil
		}
		return &readerImageInfo{
			src:    src,
			alt:    normalizeOptionalReaderText(readerNodeAttr(node, "alt")),
			title:  normalizeOptionalReaderText(readerNodeAttr(node, "title")),
			width:  parseReaderPositiveInteger(readerNodeAttr(node, "width")),
			height: parseReaderPositiveInteger(readerNodeAttr(node, "height")),
		}
	}
	if node.name == "a" && len(node.children) == 1 {
		child := node.children[0]
		if !child.isText && child.name == "img" {
			image := extractReaderImageInfo(child)
			if image == nil {
				return nil
			}
			if originalURL := sanitizeReaderURL(readerNodeAttr(node, "href"), readerSafeLinkURLSchemes); originalURL != "" {
				image.originalURL = originalURL
			}
			if title := normalizeOptionalReaderText(readerNodeAttr(node, "title")); title != "" {
				image.title = title
			}
			return image
		}
	}
	return nil
}

func extractReaderParagraphImageInfo(node *readerHTMLNode) *readerImageInfo {
	meaningful := []*readerHTMLNode{}
	for _, child := range node.children {
		if child.isText {
			if strings.TrimSpace(child.value) != "" {
				meaningful = append(meaningful, child)
			}
			continue
		}
		if child.name == "br" {
			continue
		}
		meaningful = append(meaningful, child)
	}
	if len(meaningful) != 1 || meaningful[0].isText {
		return nil
	}
	return extractReaderImageInfo(meaningful[0])
}

func readerImageBlock(section string, image *readerImageInfo) ReaderBlock {
	block := ReaderBlock{Type: "image", Section: section, Src: image.src}
	if image.alt != "" {
		alt := image.alt
		block.Alt = &alt
	}
	if image.originalURL != "" {
		originalURL := image.originalURL
		block.OriginalURL = &originalURL
	}
	if image.title != "" {
		title := image.title
		block.Title = &title
	}
	block.Width = image.width
	block.Height = image.height
	return block
}

func createReaderTextTokens(text string) []ReaderInline {
	lines := strings.Split(text, "\n")
	tokens := []ReaderInline{}
	for index, line := range lines {
		if line != "" {
			tokens = append(tokens, ReaderInline{Type: "text", Text: line})
		}
		if index < len(lines)-1 {
			tokens = append(tokens, ReaderInline{Type: "lineBreak"})
		}
	}
	if len(tokens) == 0 {
		tokens = append(tokens, ReaderInline{Type: "text", Text: ""})
	}
	return tokens
}

func appendReaderSectionBlocks(nodes []*readerHTMLNode, section string, blocks []ReaderBlock) []ReaderBlock {
	for _, node := range nodes {
		if node.isText {
			if strings.TrimSpace(node.value) == "" {
				continue
			}
			blocks = append(blocks, ReaderBlock{Type: "paragraph", Section: section, Inlines: createReaderTextTokens(node.value)})
			continue
		}
		if node.name == "br" {
			blocks = append(blocks, ReaderBlock{Type: "paragraph", Section: section, Inlines: []ReaderInline{{Type: "lineBreak"}}})
			continue
		}
		if readerTopLevelContainerTags[node.name] {
			blocks = appendReaderSectionBlocks(node.children, section, blocks)
			continue
		}
		if node.name == "p" {
			if image := extractReaderParagraphImageInfo(node); image != nil {
				blocks = append(blocks, readerImageBlock(section, image))
				continue
			}
			tokens, unsupported := parseReaderInlineNodes(node.children)
			if !unsupported {
				if len(tokens) == 0 {
					tokens = []ReaderInline{{Type: "lineBreak"}}
				}
				blocks = append(blocks, ReaderBlock{Type: "paragraph", Section: section, Inlines: tokens})
				continue
			}
			blocks = appendReaderHTMLFallbackBlock(node, section, blocks)
			continue
		}
		if image := extractReaderImageInfo(node); image != nil {
			blocks = append(blocks, readerImageBlock(section, image))
			continue
		}
		blocks = appendReaderHTMLFallbackBlock(node, section, blocks)
	}
	return blocks
}

func appendReaderHTMLFallbackBlock(node *readerHTMLNode, section string, blocks []ReaderBlock) []ReaderBlock {
	rendered := serializeReaderHTMLNode(node)
	plainText := strings.TrimSpace(collectReaderTextContent([]*readerHTMLNode{node}))
	if rendered == "" && plainText == "" {
		return blocks
	}
	return append(blocks, ReaderBlock{Type: "html", Section: section, HTML: rendered, PlainText: plainText})
}

func buildReaderSectionBlocks(sectionHTML string, section string) []ReaderBlock {
	return appendReaderSectionBlocks(parseReaderHTMLFragment(sectionHTML), section, []ReaderBlock{})
}

// readerSectionSeparatorBlock は前書き・本文・後書きの境界を表す区切りブロックを返す。
// 取得元サイトの <hr> 相当は novel-fetcher の section 分割時点で失われているため、
// section 分割で失われる境界を、reader 表示用の区切りとしてここで復元する。
func readerSectionSeparatorBlock(section string) ReaderBlock {
	return ReaderBlock{
		Type:      "html",
		Section:   section,
		HTML:      `<hr class="reader-section-separator">`,
		PlainText: "",
	}
}
