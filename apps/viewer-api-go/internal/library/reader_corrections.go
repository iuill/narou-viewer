package library

type ReaderCorrectionSettings struct {
	QuoteNormalization                     bool
	HyphenDashNormalization                bool
	ParenthesisNormalization               bool
	HalfwidthAlnumPunctuationNormalization bool
}

type readerCorrectionState struct {
	inQuote bool
}

func ApplyReaderCorrections(document ReaderDocument, settings ReaderCorrectionSettings) ReaderDocument {
	if !settings.QuoteNormalization && !settings.HyphenDashNormalization && !settings.ParenthesisNormalization && !settings.HalfwidthAlnumPunctuationNormalization {
		return document
	}
	next := document
	next.Blocks = make([]ReaderBlock, len(document.Blocks))
	for index, block := range document.Blocks {
		next.Blocks[index] = applyReaderBlockCorrections(block, settings)
	}
	return next
}

func applyReaderBlockCorrections(block ReaderBlock, settings ReaderCorrectionSettings) ReaderBlock {
	state := &readerCorrectionState{}
	switch block.Type {
	case "meta", "title":
		block.Text = state.applyText(block.Text, settings.withoutHalfwidthAlnumPunctuationNormalization())
	case "paragraph":
		block.Inlines = applyReaderInlineCorrections(block.Inlines, settings, state)
	case "html":
		block.HTML = applyReaderHTMLCorrections(block.HTML, settings)
		block.PlainText = (&readerCorrectionState{}).applyText(block.PlainText, settings)
	}
	return block
}

func applyReaderInlineCorrections(tokens []ReaderInline, settings ReaderCorrectionSettings, state *readerCorrectionState) []ReaderInline {
	if len(tokens) == 0 {
		return tokens
	}
	next := make([]ReaderInline, len(tokens))
	for index, token := range tokens {
		switch token.Type {
		case "text", "ruby", "tcy":
			token.Text = state.applyText(token.Text, settings)
		case "lineBreak":
			state.inQuote = false
		case "link":
			token.Children = applyReaderInlineCorrections(token.Children, settings, state)
		}
		next[index] = token
	}
	return next
}

func applyReaderHTMLCorrections(source string, settings ReaderCorrectionSettings) string {
	nodes := parseReaderHTMLFragment(source)
	state := &readerCorrectionState{}
	applyReaderHTMLNodeCorrections(nodes, settings, state)
	if settings.HyphenDashNormalization {
		normalizeReaderHTMLHyphensAcrossTextNodes(nodes)
	}
	var rendered string
	for _, node := range nodes {
		rendered += serializeReaderHTMLNode(node)
	}
	return rendered
}

func applyReaderHTMLNodeCorrections(nodes []*readerHTMLNode, settings ReaderCorrectionSettings, state *readerCorrectionState) {
	for _, node := range nodes {
		if node.isText {
			node.value = state.applyText(node.value, settings)
			continue
		}
		if node.name == "br" {
			state.inQuote = false
			continue
		}
		applyReaderHTMLNodeCorrections(node.children, settings, state)
	}
}

type readerHTMLRuneRef struct {
	node  *readerHTMLNode
	index int
}

func isReaderHTMLInlineHyphenRunElement(name string) bool {
	switch name {
	case "a", "abbr", "b", "bdi", "bdo", "cite", "code", "data", "em", "i", "kbd", "mark", "q", "rb", "rp", "rt", "rtc", "ruby", "s", "samp", "small", "span", "strong", "sub", "sup", "time", "u", "var", "wbr":
		return true
	default:
		return false
	}
}

func normalizeReaderHTMLHyphensAcrossTextNodes(nodes []*readerHTMLNode) {
	var pending []readerHTMLRuneRef
	var flush = func() {
		if len(pending) >= 2 {
			for _, ref := range pending {
				runes := []rune(ref.node.value)
				if ref.index >= 0 && ref.index < len(runes) {
					runes[ref.index] = '―'
					ref.node.value = string(runes)
				}
			}
		}
		pending = nil
	}

	var walk func([]*readerHTMLNode)
	walk = func(currentNodes []*readerHTMLNode) {
		for _, node := range currentNodes {
			if node.isText {
				for index, r := range []rune(node.value) {
					if r == '-' || r == '―' {
						pending = append(pending, readerHTMLRuneRef{node: node, index: index})
					} else {
						flush()
					}
				}
				continue
			}
			if node.name == "br" {
				flush()
				continue
			}
			if !isReaderHTMLInlineHyphenRunElement(node.name) {
				flush()
				walk(node.children)
				flush()
				continue
			}
			walk(node.children)
		}
	}
	walk(nodes)
	flush()
}

func (state *readerCorrectionState) applyText(text string, settings ReaderCorrectionSettings) string {
	if !settings.QuoteNormalization && !settings.HyphenDashNormalization && !settings.ParenthesisNormalization && !settings.HalfwidthAlnumPunctuationNormalization {
		return text
	}
	runes := []rune(text)
	for index, r := range runes {
		switch r {
		case '"', '“', '”', '〝', '〟':
			if settings.QuoteNormalization {
				if state.inQuote {
					runes[index] = '〟'
				} else {
					runes[index] = '〝'
				}
				state.inQuote = !state.inQuote
			}
		case '\n', '\r':
			state.inQuote = false
		case '(':
			if settings.ParenthesisNormalization {
				runes[index] = '（'
			}
		case ')':
			if settings.ParenthesisNormalization {
				runes[index] = '）'
			}
		case '!':
			if settings.HalfwidthAlnumPunctuationNormalization {
				runes[index] = '！'
			}
		case '?':
			if settings.HalfwidthAlnumPunctuationNormalization {
				runes[index] = '？'
			}
		default:
			if settings.HalfwidthAlnumPunctuationNormalization {
				runes[index] = normalizeHalfwidthAlnumRune(r)
			}
		}
	}
	if settings.HyphenDashNormalization {
		normalizeConsecutiveHyphensToDash(runes)
	}
	return string(runes)
}

func (settings ReaderCorrectionSettings) withoutHalfwidthAlnumPunctuationNormalization() ReaderCorrectionSettings {
	settings.HalfwidthAlnumPunctuationNormalization = false
	return settings
}

func normalizeHalfwidthAlnumRune(r rune) rune {
	switch {
	case '0' <= r && r <= '9':
		return '０' + (r - '0')
	case 'A' <= r && r <= 'Z':
		return 'Ａ' + (r - 'A')
	case 'a' <= r && r <= 'z':
		return 'ａ' + (r - 'a')
	default:
		return r
	}
}

func normalizeConsecutiveHyphensToDash(runes []rune) {
	for index := 0; index < len(runes); {
		if runes[index] != '-' {
			index++
			continue
		}
		start := index
		for index < len(runes) && runes[index] == '-' {
			index++
		}
		if index-start < 2 {
			continue
		}
		for dashIndex := start; dashIndex < index; dashIndex++ {
			runes[dashIndex] = '―'
		}
	}
}
