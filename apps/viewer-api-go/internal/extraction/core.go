package extraction

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"narou-viewer/apps/viewer-api-go/internal/characters"
	"narou-viewer/apps/viewer-api-go/internal/library"
	"narou-viewer/apps/viewer-api-go/internal/terms"
)

const (
	defaultExtractionMaxChunkChars = 4000
	defaultExtractionMaxBatchChars = 12000
)

type EpisodeInput struct {
	EpisodeIndex   string
	Title          string
	Chapter        *string
	Subchapter     *string
	HTML           string
	ReaderDocument library.ReaderDocument
}

type Chunk struct {
	EpisodeIndex string
	Title        string
	Chapter      *string
	Subchapter   *string
	ChunkIndex   int
	ChunkCount   int
	Text         string
}

type Batch struct {
	BatchIndex     int
	BatchCount     int
	EpisodeIndexes []string
	Chunks         []Chunk
}

type BatchBudget struct {
	MaxTextChars  int
	MaxTextTokens int
}

type Delta struct {
	NewCharacters      []characters.GeneratedCharacter
	CharacterUpdates   []characters.GeneratedCharacter
	MergeProposals     []MergeProposal
	UnresolvedMentions []UnresolvedMention
	LegacyCharacters   []characters.GeneratedCharacter
	Terms              []terms.GeneratedTerm
}

type MergeProposal struct {
	SourceCharacterID string  `json:"sourceCharacterId"`
	TargetCharacterID string  `json:"targetCharacterId"`
	Confidence        float64 `json:"confidence"`
	Reason            string  `json:"reason"`
}

type UnresolvedMention struct {
	Mention      string `json:"mention"`
	EpisodeIndex string `json:"episodeIndex"`
	Reason       string `json:"reason"`
}

type GenerationState struct {
	UnresolvedMentions  []characters.GeneratedUnresolvedMention
	IssuedCharacterIDs  []string
	RetiredCharacterIDs []characters.GeneratedRetiredCharacterID
	NextOrdinal         int
	Terms               []terms.GeneratedTerm
}

const MergeAutoApplyConfidence = 0.75

var htmlTagPattern = regexp.MustCompile(`<[^>]+>`)
var htmlScriptPattern = regexp.MustCompile(`(?is)<script[\s\S]*?</script>`)
var htmlStylePattern = regexp.MustCompile(`(?is)<style[\s\S]*?</style>`)

func Limits() (int, int) {
	return PositiveEnvIntWithFallback("EXTRACTION_MAX_CHUNK_CHARS", "CHARACTER_SUMMARY_MAX_CHUNK_CHARS", defaultExtractionMaxChunkChars),
		PositiveEnvIntWithFallback("EXTRACTION_MAX_BATCH_CHARS", "CHARACTER_SUMMARY_MAX_BATCH_CHARS", defaultExtractionMaxBatchChars)
}

func PositiveEnvIntWithFallback(name string, legacyName string, fallback int) int {
	if strings.TrimSpace(os.Getenv(name)) != "" {
		return PositiveEnvInt(name, fallback)
	}
	return PositiveEnvInt(legacyName, fallback)
}

func PositiveEnvInt(name string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(os.Getenv(name)))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func normalizeSummaryWhitespace(source string) string {
	return strings.TrimSpace(strings.Join(strings.Fields(strings.ReplaceAll(source, "\u00a0", " ")), " "))
}

func stripSummaryHTML(source string) string {
	withoutScript := htmlScriptPattern.ReplaceAllString(source, " ")
	withoutStyle := htmlStylePattern.ReplaceAllString(withoutScript, " ")
	withoutTags := htmlTagPattern.ReplaceAllString(withoutStyle, " ")
	return normalizeSummaryWhitespace(html.UnescapeString(withoutTags))
}

func RenderInlineTokens(tokens []library.ReaderInline) string {
	parts := []string{}
	for _, token := range tokens {
		switch token.Type {
		case "text", "ruby", "tcy":
			parts = append(parts, token.Text)
		case "lineBreak":
			parts = append(parts, "\n")
		case "link":
			parts = append(parts, RenderInlineTokens(token.Children))
		}
	}
	return strings.Join(parts, "")
}

func RenderExtractionInlineTokens(tokens []library.ReaderInline) string {
	parts := []string{}
	for _, token := range tokens {
		switch token.Type {
		case "text", "tcy":
			parts = append(parts, token.Text)
		case "ruby":
			if strings.TrimSpace(token.Ruby) != "" {
				parts = append(parts, token.Text+"《"+token.Ruby+"》")
			} else {
				parts = append(parts, token.Text)
			}
		case "lineBreak":
			parts = append(parts, "\n")
		case "link":
			parts = append(parts, RenderExtractionInlineTokens(token.Children))
		}
	}
	return strings.Join(parts, "")
}

func RenderBlock(block library.ReaderBlock) string {
	switch block.Type {
	case "meta", "title":
		return block.Text
	case "paragraph":
		return RenderInlineTokens(block.Inlines)
	case "html":
		if strings.TrimSpace(block.PlainText) != "" {
			return block.PlainText
		}
		return stripSummaryHTML(block.HTML)
	case "image":
		values := []string{}
		if block.Alt != nil && strings.TrimSpace(*block.Alt) != "" {
			values = append(values, strings.TrimSpace(*block.Alt))
		}
		if block.Title != nil && strings.TrimSpace(*block.Title) != "" {
			values = append(values, strings.TrimSpace(*block.Title))
		}
		return strings.Join(values, " ")
	default:
		return ""
	}
}

func ExtractEpisodeText(episode EpisodeInput) string {
	parts := []string{}
	for _, block := range episode.ReaderDocument.Blocks {
		if block.Type != "paragraph" && block.Type != "html" && block.Type != "image" {
			continue
		}
		rendered := RenderBlock(block)
		if strings.TrimSpace(rendered) != "" {
			parts = append(parts, rendered)
		}
	}
	text := normalizeSummaryWhitespace(strings.Join(parts, " "))
	if text != "" {
		return text
	}
	return stripSummaryHTML(episode.HTML)
}

func ExtractEpisodePromptText(episode EpisodeInput) string {
	parts := []string{}
	for _, block := range episode.ReaderDocument.Blocks {
		switch block.Type {
		case "paragraph":
			parts = append(parts, RenderExtractionInlineTokens(block.Inlines))
		case "html", "image":
			parts = append(parts, RenderBlock(block))
		}
	}
	text := normalizeSummaryWhitespace(strings.Join(parts, " "))
	if text != "" {
		return text
	}
	return stripSummaryHTML(episode.HTML)
}

func splitSummarySentenceChunks(text string, maxChars int) []string {
	normalized := normalizeSummaryWhitespace(text)
	if normalized == "" {
		return []string{}
	}
	if len([]rune(normalized)) <= maxChars {
		return []string{normalized}
	}
	fragments := splitSummaryFragments(normalized)
	chunks := []string{}
	current := ""
	for _, fragment := range fragments {
		next := fragment
		if current != "" {
			next = current + fragment
		}
		if len([]rune(next)) <= maxChars {
			current = next
			continue
		}
		if current != "" {
			chunks = append(chunks, current)
			current = ""
		}
		if len([]rune(fragment)) <= maxChars {
			current = fragment
			continue
		}
		pieces := splitRunesEvery(fragment, maxChars)
		for index, piece := range pieces {
			if index == len(pieces)-1 && len([]rune(piece)) < maxChars {
				current = piece
			} else {
				chunks = append(chunks, piece)
			}
		}
	}
	if current != "" {
		chunks = append(chunks, current)
	}
	return chunks
}

func splitSummaryFragments(text string) []string {
	fragments := []string{}
	start := 0
	runes := []rune(text)
	for index, r := range runes {
		switch r {
		case '。', '！', '？', '!', '?':
			fragment := strings.TrimSpace(string(runes[start : index+1]))
			if fragment != "" {
				fragments = append(fragments, fragment)
			}
			start = index + 1
		}
	}
	if start < len(runes) {
		fragment := strings.TrimSpace(string(runes[start:]))
		if fragment != "" {
			fragments = append(fragments, fragment)
		}
	}
	return fragments
}

func splitRunesEvery(text string, maxChars int) []string {
	runes := []rune(text)
	pieces := []string{}
	for offset := 0; offset < len(runes); offset += maxChars {
		end := offset + maxChars
		if end > len(runes) {
			end = len(runes)
		}
		pieces = append(pieces, string(runes[offset:end]))
	}
	return pieces
}

func CreateChunks(episodes []EpisodeInput, maxChunkChars int) []Chunk {
	chunks := []Chunk{}
	for _, episode := range episodes {
		text := ExtractEpisodePromptText(episode)
		chunks = append(chunks, CreateChunksFromText(episode, text, maxChunkChars)...)
	}
	return chunks
}

func CreateChunksFromText(episode EpisodeInput, text string, maxChunkChars int) []Chunk {
	textChunks := splitSummarySentenceChunks(text, maxChunkChars)
	if len(textChunks) == 0 {
		textChunks = []string{""}
	}
	chunks := make([]Chunk, 0, len(textChunks))
	for index, text := range textChunks {
		chunks = append(chunks, Chunk{
			EpisodeIndex: episode.EpisodeIndex,
			Title:        episode.Title,
			Chapter:      episode.Chapter,
			Subchapter:   episode.Subchapter,
			ChunkIndex:   index + 1,
			ChunkCount:   len(textChunks),
			Text:         text,
		})
	}
	return chunks
}

func CreateBatches(chunks []Chunk, maxBatchChars int) []Batch {
	return CreateBatchesWithBudget(chunks, BatchBudget{MaxTextChars: maxBatchChars})
}

func CreateBatchesWithBudget(chunks []Chunk, budget BatchBudget) []Batch {
	rawBatches := [][]Chunk{}
	current := []Chunk{}
	currentChars := 0
	currentTokens := 0
	for _, chunk := range chunks {
		chunkChars := len([]rune(chunk.Text))
		chunkTokens := ChunkTokenCost(chunk)
		if len(current) > 0 && BudgetExceeded(currentChars+chunkChars, currentTokens+chunkTokens, budget) {
			rawBatches = append(rawBatches, current)
			current = []Chunk{}
			currentChars = 0
			currentTokens = 0
		}
		current = append(current, chunk)
		currentChars += chunkChars
		currentTokens += chunkTokens
	}
	if len(current) > 0 {
		rawBatches = append(rawBatches, current)
	}
	batches := make([]Batch, 0, len(rawBatches))
	for index, raw := range rawBatches {
		batches = append(batches, Batch{
			BatchIndex:     index + 1,
			BatchCount:     len(rawBatches),
			EpisodeIndexes: UniqueChunkEpisodeIndexes(raw),
			Chunks:         raw,
		})
	}
	return batches
}

func BudgetExceeded(chars int, tokens int, budget BatchBudget) bool {
	if budget.MaxTextTokens > 0 {
		return tokens > budget.MaxTextTokens
	}
	if budget.MaxTextChars > 0 {
		return chars > budget.MaxTextChars
	}
	return false
}

func ChunkTokenCost(chunk Chunk) int {
	// Include a small per-chunk overhead for episode metadata and JSON punctuation.
	return EstimateTokenCount(chunk.Text) + 32
}

func UniqueChunkEpisodeIndexes(chunks []Chunk) []string {
	result := []string{}
	seen := map[string]bool{}
	for _, chunk := range chunks {
		if seen[chunk.EpisodeIndex] {
			continue
		}
		seen[chunk.EpisodeIndex] = true
		result = append(result, chunk.EpisodeIndex)
	}
	return result
}

type BatchFitFunc func(Batch) (bool, error)

func PlanRuntimeBatch(template Batch, chunks []Chunk, fits BatchFitFunc) (Batch, []Chunk, error) {
	if len(chunks) == 0 {
		return RuntimeBatch(template, nil), nil, nil
	}
	current := []Chunk{}
	for index, chunk := range chunks {
		candidateChunks := append(append([]Chunk{}, current...), chunk)
		candidate := RuntimeBatch(template, candidateChunks)
		ok, err := fits(candidate)
		if err != nil {
			return Batch{}, nil, err
		}
		if ok {
			current = candidateChunks
			continue
		}
		if len(current) > 0 {
			return RuntimeBatch(template, current), chunks[index:], nil
		}
		split, err := SplitOversizedChunkBatch(RuntimeBatch(template, []Chunk{chunk}), fits)
		if err != nil {
			return Batch{}, nil, err
		}
		if len(split) == 0 {
			return Batch{}, nil, errors.New("extraction batch split returned no chunks")
		}
		remaining := []Chunk{}
		for _, batch := range split[1:] {
			remaining = append(remaining, batch.Chunks...)
		}
		remaining = append(remaining, chunks[index+1:]...)
		return split[0], remaining, nil
	}
	return RuntimeBatch(template, current), nil, nil
}

func PlanRuntimeBatches(batch Batch, fits BatchFitFunc) ([]Batch, error) {
	if len(batch.Chunks) == 0 {
		return []Batch{batch}, nil
	}
	ok, err := fits(batch)
	if err != nil {
		return nil, err
	}
	if ok {
		return []Batch{batch}, nil
	}
	if len(batch.Chunks) == 1 {
		split, err := SplitOversizedChunkBatch(batch, fits)
		if err != nil {
			return nil, err
		}
		return split, nil
	}

	result := []Batch{}
	current := []Chunk{}
	for _, chunk := range batch.Chunks {
		candidateChunks := append(append([]Chunk{}, current...), chunk)
		candidate := RuntimeBatch(batch, candidateChunks)
		ok, err := fits(candidate)
		if err != nil {
			return nil, err
		}
		if ok {
			current = candidateChunks
			continue
		}
		if len(current) > 0 {
			result = append(result, RuntimeBatch(batch, current))
			current = nil
		}
		single := RuntimeBatch(batch, []Chunk{chunk})
		singleFits, err := fits(single)
		if err != nil {
			return nil, err
		}
		if singleFits {
			current = []Chunk{chunk}
			continue
		}
		split, err := SplitOversizedChunkBatch(single, fits)
		if err != nil {
			return nil, err
		}
		result = append(result, split...)
	}
	if len(current) > 0 {
		result = append(result, RuntimeBatch(batch, current))
	}
	return result, nil
}

func RuntimeBatch(template Batch, chunks []Chunk) Batch {
	return Batch{
		BatchIndex:     template.BatchIndex,
		BatchCount:     template.BatchCount,
		EpisodeIndexes: UniqueChunkEpisodeIndexes(chunks),
		Chunks:         append([]Chunk{}, chunks...),
	}
}

func SplitOversizedChunkBatch(batch Batch, fits BatchFitFunc) ([]Batch, error) {
	if len(batch.Chunks) != 1 {
		return []Batch{batch}, nil
	}
	chunk := batch.Chunks[0]
	runes := []rune(chunk.Text)
	if len(runes) <= 1 {
		return nil, fmt.Errorf("extraction batch cannot fit in model context even after splitting: episodeIndex=%s", chunk.EpisodeIndex)
	}
	mid := len(runes) / 2
	if mid < 1 {
		mid = 1
	}
	left := chunk
	left.Text = string(runes[:mid])
	left.ChunkCount = MaxInt(chunk.ChunkCount, 2)
	right := chunk
	right.Text = string(runes[mid:])
	right.ChunkIndex = chunk.ChunkIndex + 1
	right.ChunkCount = MaxInt(chunk.ChunkCount, 2)
	pieces := []Batch{}
	for _, piece := range []Chunk{left, right} {
		pieceBatch := RuntimeBatch(batch, []Chunk{piece})
		ok, err := fits(pieceBatch)
		if err != nil {
			return nil, err
		}
		if ok {
			pieces = append(pieces, pieceBatch)
			continue
		}
		split, err := SplitOversizedChunkBatch(pieceBatch, fits)
		if err != nil {
			return nil, err
		}
		pieces = append(pieces, split...)
	}
	return pieces, nil
}

const (
	defaultPromptReserveTokens = 2048
	defaultMinBatchTokens      = 512
	defaultMaxCompletionTokens = 12000
)

func ResolveBatchBudget(fallbackChars int, contextLength int, maxCompletionTokens int) BatchBudget {
	budget := BatchBudget{MaxTextChars: fallbackChars, MaxTextTokens: TokensFromChars(fallbackChars)}
	if contextLength <= 0 {
		return budget
	}
	outputReserve := maxCompletionTokens
	if outputReserve <= 0 {
		outputReserve = defaultMaxCompletionTokens
	}
	available := contextLength - outputReserve - defaultPromptReserveTokens
	if available <= 0 {
		return budget
	}
	tokenBudget := available / 4
	if tokenBudget < defaultMinBatchTokens {
		tokenBudget = available
	}
	if tokenBudget < 1 {
		return budget
	}
	return BatchBudget{MaxTextTokens: tokenBudget}
}

func TokensFromChars(chars int) int {
	if chars <= 0 {
		return 0
	}
	return chars
}

func BuildDefaultSystemPrompt() string {
	return strings.Join([]string{
		"あなたは日本語の普通の物語小説から、キャラクター情報と作品固有の用語情報の差分を同時抽出する専用アシスタントです。",
		"本文に明示された事実だけを使い、推測や補完はしないでください。",
		"出力は必ず JSON 形式の差分更新だけにしてください。",
		"最優先方針: 抽出対象は「人として登場する個人キャラクター」だけです。",
		"入力の candidateCharacters は今回の本文に関係しそうな既存人物候補だけです。",
		"既存人物候補に紐づく新情報は characterUpdates に characterId を指定して入れてください。",
		"候補にない新人物は newCharacters に入れてください。candidateCharacters が空の場合は、本文に明示的に登場する人物をすべて newCharacters に入れてください。",
		"既存全キャラクターを再出力しないでください。ただし初回抽出で candidateCharacters が空の場合、検出した人物を省略しないでください。",
		"aliases は本人を指す固有名・通称・表記揺れだけにしてください。「先生」「隊長」「伯母さん」「鍵の人」のような役職・関係・説明だけの語は aliases に入れず、必要なら summaryHistory に書いてください。",
		"本文が同一人物だと明示する複数IDは mergeProposals に入れてください。推測だけでmergeしないでください。",
		"appearanceHistory は身体的外見だけを書いてください。",
		"personalityHistory には持続的な性向だけを書いてください。",
		"fullName や gender が話数により変化・判明する場合は、最新値だけでなく fullNameHistory / genderHistory に時点ごとの値を残してください。",
		"summaryHistory には、その時点で新しく分かった人物像、立場、関係性、主要な行動を 1〜2 文で短く要約してください。",
		"各履歴項目は episodeIndex と text を必ず持ちます。",
		"同じ内容を重複して入れないでください。",
		"terms には人物名を含めず、組織・場所・物品・技能・種族・出来事などの作品固有語だけを入れてください。",
		"term の category は organization / place / item / skill / race / event / other のいずれかです。",
		"reading は本文中の明示的なルビ、または表記自体が読みとして明示された場合だけ記録し、読みを推測しないでください。不明なら null にしてください。",
		"term の descriptionHistory は差分断片ではなく、その話時点までの既知情報をまとめた自己完結型 snapshot にしてください。未来話の情報を混ぜないでください。",
	}, " ")
}

func ResolveSystemPrompt(systemPromptOverride *string) string {
	if systemPromptOverride != nil && strings.TrimSpace(*systemPromptOverride) != "" {
		return strings.TrimSpace(*systemPromptOverride)
	}
	return BuildDefaultSystemPrompt()
}

func BuildPrompt(novelID string, upToEpisodeIndex string, knownCharacters []characters.GeneratedCharacter, batch Batch, systemPromptOverride *string) (string, string) {
	return BuildPromptWithContext(novelID, upToEpisodeIndex, knownCharacters, nil, batch, nil, systemPromptOverride)
}

func BuildPromptWithUnresolved(novelID string, upToEpisodeIndex string, knownCharacters []characters.GeneratedCharacter, batch Batch, unresolvedMentions []characters.GeneratedUnresolvedMention, systemPromptOverride *string) (string, string) {
	return BuildPromptWithContext(novelID, upToEpisodeIndex, knownCharacters, nil, batch, unresolvedMentions, systemPromptOverride)
}

func BuildPromptWithContext(novelID string, upToEpisodeIndex string, knownCharacters []characters.GeneratedCharacter, knownTerms []terms.GeneratedTerm, batch Batch, unresolvedMentions []characters.GeneratedUnresolvedMention, systemPromptOverride *string) (string, string) {
	candidateCharacters := CandidateCards(knownCharacters, batch)
	candidateTerms := TermCandidateCards(knownTerms, batch)
	payload := map[string]any{
		"novelId":             novelID,
		"upToEpisodeIndex":    upToEpisodeIndex,
		"candidateCharacters": candidateCharacters,
		"knownTerms":          candidateTerms,
		"episodes":            chunkPromptPayload(batch.Chunks),
		"outputContract":      "Return only delta fields: newCharacters, characterUpdates, mergeProposals, unresolvedMentions, terms. terms must always be present; use [] when there is no term delta. If candidateCharacters is empty, put every explicitly appearing person in newCharacters.",
	}
	if len(candidateCharacters) == 0 {
		payload["generationTask"] = "initialCharacterExtraction"
		payload["initialExtractionInstruction"] = "candidateCharacters is empty. Extract all individual human characters explicitly appearing in episodes into newCharacters; do not return an empty delta unless the episodes contain no people."
	}
	if unresolved := unresolvedMentionPromptPayload(unresolvedMentions); len(unresolved) > 0 {
		payload["unresolvedMentions"] = unresolved
	}
	raw, _ := json.MarshalIndent(payload, "", "  ")
	return ResolveSystemPrompt(systemPromptOverride), string(raw)
}

func TermCandidateCards(values []terms.GeneratedTerm, batch Batch) []map[string]any {
	type scoredTerm struct {
		value terms.GeneratedTerm
		score int
	}
	batchText := batchTextForCandidateSearch(batch)
	scored := make([]scoredTerm, 0, len(values))
	for _, value := range values {
		term := strings.TrimSpace(value.Term)
		if term == "" || len(value.DescriptionHistory) == 0 {
			continue
		}
		score := latestTermEpisode(value)
		if strings.Contains(batchText, term) {
			score += 1_000_000
		}
		scored = append(scored, scoredTerm{value: value, score: score})
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		return scored[i].value.Term < scored[j].value.Term
	})
	if len(scored) > 16 {
		scored = scored[:16]
	}
	result := make([]map[string]any, 0, len(scored))
	for _, candidate := range scored {
		value := candidate.value
		card := map[string]any{
			"term":               strings.TrimSpace(value.Term),
			"category":           terms.CategoryOther,
			"latestDescription":  value.DescriptionHistory[len(value.DescriptionHistory)-1].Text,
			"latestEpisodeIndex": value.DescriptionHistory[len(value.DescriptionHistory)-1].EpisodeIndex,
		}
		if len(value.ReadingHistory) > 0 {
			card["reading"] = value.ReadingHistory[len(value.ReadingHistory)-1].Text
		} else {
			card["reading"] = nil
		}
		if len(value.CategoryHistory) > 0 {
			card["category"] = terms.NormalizeCategory(value.CategoryHistory[len(value.CategoryHistory)-1].Category)
		}
		result = append(result, card)
	}
	return result
}

func latestTermEpisode(value terms.GeneratedTerm) int {
	latest := 0
	for _, history := range value.DescriptionHistory {
		if episode, err := strconv.Atoi(history.EpisodeIndex); err == nil && episode > latest {
			latest = episode
		}
	}
	return latest
}

func unresolvedMentionPromptPayload(values []characters.GeneratedUnresolvedMention) []map[string]any {
	if len(values) == 0 {
		return nil
	}
	values = append([]characters.GeneratedUnresolvedMention{}, values...)
	sort.SliceStable(values, func(i, j int) bool {
		diff := CompareEpisodeString(values[i].EpisodeIndex, values[j].EpisodeIndex)
		if diff != 0 {
			return diff > 0
		}
		return values[i].Mention < values[j].Mention
	})
	result := []map[string]any{}
	for _, value := range values {
		mention := strings.TrimSpace(value.Mention)
		episodeIndex := strings.TrimSpace(value.EpisodeIndex)
		if mention == "" || episodeIndex == "" {
			continue
		}
		item := map[string]any{
			"mention":      mention,
			"episodeIndex": episodeIndex,
		}
		if strings.TrimSpace(value.Reason) != "" {
			item["reason"] = strings.TrimSpace(value.Reason)
		}
		if len(value.CandidateIDs) > 0 {
			item["candidateIds"] = value.CandidateIDs
		}
		result = append(result, item)
		if len(result) >= 32 {
			break
		}
	}
	return result
}

func CandidateCards(values []characters.GeneratedCharacter, batch Batch) []map[string]any {
	type scored struct {
		value        characters.GeneratedCharacter
		score        int
		exactMatch   bool
		matchedNames []string
	}
	if len(values) == 0 {
		return []map[string]any{}
	}
	batchText := strings.ToLower(batchTextForCandidateSearch(batch))
	IdentityFrequency := IdentityFrequency(values)
	scoredValues := []scored{}
	for index, value := range values {
		score := MaxInt(0, 1000-index)
		exactMatch := false
		matchedNames := []string{}
		for _, name := range GeneratedIdentityKeys(value) {
			normalized := strings.ToLower(strings.TrimSpace(name))
			if normalized != "" && !isGenericCharacterAlias(name) && strings.Contains(batchText, normalized) {
				matchedNames = append(matchedNames, strings.TrimSpace(name))
				if ExactCandidateKey(normalized, IdentityFrequency) {
					exactMatch = true
				}
				score += 10000 + len([]rune(normalized))*20
			}
		}
		score += latestGeneratedCharacterEpisode(value)
		scoredValues = append(scoredValues, scored{value: value, score: score, exactMatch: exactMatch, matchedNames: matchedNames})
	}
	sort.SliceStable(scoredValues, func(i, j int) bool {
		if scoredValues[i].score != scoredValues[j].score {
			return scoredValues[i].score > scoredValues[j].score
		}
		return scoredValues[i].value.CanonicalName < scoredValues[j].value.CanonicalName
	})
	cards := []map[string]any{}
	recencySlots := 8
	for _, scoredValue := range scoredValues {
		if !scoredValue.exactMatch {
			continue
		}
		cards = append(cards, candidateCard(scoredValue.value, scoredValue.matchedNames...))
		recencySlots--
	}
	if recencySlots < 0 {
		recencySlots = 0
	}
	for _, scoredValue := range scoredValues {
		if recencySlots <= 0 {
			break
		}
		if scoredValue.exactMatch {
			continue
		}
		cards = append(cards, candidateCard(scoredValue.value, scoredValue.matchedNames...))
		recencySlots--
	}
	return cards
}

func IdentityFrequency(values []characters.GeneratedCharacter) map[string]int {
	frequency := map[string]int{}
	for _, value := range values {
		seen := map[string]bool{}
		for _, key := range GeneratedIdentityKeys(value) {
			normalized := strings.ToLower(strings.TrimSpace(key))
			if normalized == "" || strings.HasPrefix(normalized, "id:") || seen[normalized] {
				continue
			}
			seen[normalized] = true
			frequency[normalized]++
		}
	}
	return frequency
}

func ExactCandidateKey(normalized string, frequency map[string]int) bool {
	return len([]rune(normalized)) >= 2 && frequency[normalized] == 1
}

func batchTextForCandidateSearch(batch Batch) string {
	parts := []string{}
	for _, chunk := range batch.Chunks {
		parts = append(parts, chunk.Title, chunk.Text)
	}
	return strings.Join(parts, "\n")
}

func candidateCard(value characters.GeneratedCharacter, preferredAliases ...string) map[string]any {
	aliases := []string{}
	seenAliases := map[string]bool{}
	for _, alias := range preferredAliases {
		alias = strings.TrimSpace(alias)
		if alias == "" || strings.HasPrefix(strings.ToLower(alias), "id:") || alias == strings.TrimSpace(value.CanonicalName) || isGenericCharacterAlias(alias) || seenAliases[alias] {
			continue
		}
		seenAliases[alias] = true
		aliases = append(aliases, alias)
	}
	for _, alias := range value.Aliases {
		text := strings.TrimSpace(alias.Text)
		if text != "" && !isGenericCharacterAlias(text) && !seenAliases[text] {
			seenAliases[text] = true
			aliases = append(aliases, text)
		}
	}
	if len(aliases) > 8 {
		aliases = aliases[:8]
	}
	return map[string]any{
		"characterId":          value.CharacterID,
		"displayName":          value.CanonicalName,
		"aliases":              aliases,
		"firstAppearance":      FirstNonEmptyString(value.FirstAppearanceEpisodeIndex, value.CanonicalEpisodeIndex),
		"identitySummary":      LatestGeneratedHistoryText(value.SummaryHistory),
		"recentAppearanceFact": LatestGeneratedHistoryText(value.AppearanceHistory),
		"recentPersonality":    LatestGeneratedHistoryText(value.PersonalityHistory),
	}
}

func LatestGeneratedHistoryText(values []characters.GeneratedHistoryVersion) string {
	if len(values) == 0 {
		return ""
	}
	copied := append([]characters.GeneratedHistoryVersion{}, values...)
	sort.SliceStable(copied, func(i, j int) bool {
		return CompareEpisodeString(copied[i].EpisodeIndex, copied[j].EpisodeIndex) > 0
	})
	return copied[0].Text
}

func latestGeneratedCharacterEpisode(value characters.GeneratedCharacter) int {
	latest := FirstNonEmptyString(value.FirstAppearanceEpisodeIndex, value.CanonicalEpisodeIndex)
	for _, list := range [][]characters.GeneratedHistoryVersion{value.AppearanceHistory, value.PersonalityHistory, value.SummaryHistory} {
		for _, history := range list {
			if CompareEpisodeString(history.EpisodeIndex, latest) > 0 {
				latest = history.EpisodeIndex
			}
		}
	}
	number, _ := strconv.Atoi(latest)
	return number
}

func chunkPromptPayload(chunks []Chunk) []map[string]any {
	result := make([]map[string]any, 0, len(chunks))
	for _, chunk := range chunks {
		result = append(result, map[string]any{
			"episodeIndex": chunk.EpisodeIndex,
			"title":        chunk.Title,
			"chapter":      chunk.Chapter,
			"subchapter":   chunk.Subchapter,
			"chunk":        strconv.Itoa(chunk.ChunkIndex) + "/" + strconv.Itoa(chunk.ChunkCount),
			"text":         chunk.Text,
		})
	}
	return result
}

func SummarizeGeneratedHistory(values []characters.GeneratedHistoryVersion) string {
	if len(values) == 0 {
		return "なし"
	}
	copied := append([]characters.GeneratedHistoryVersion{}, values...)
	sort.SliceStable(copied, func(i, j int) bool {
		diff := CompareEpisodeString(copied[i].EpisodeIndex, copied[j].EpisodeIndex)
		if diff != 0 {
			return diff < 0
		}
		return copied[i].Text < copied[j].Text
	})
	parts := []string{}
	for _, value := range copied {
		parts = append(parts, "第"+value.EpisodeIndex+"話: "+value.Text)
	}
	return strings.Join(parts, " / ")
}

func FirstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func NormalizeOpenRouterResponse(raw []byte, novelID string, fallbackEpisodeIndex string) (Delta, error) {
	var payload struct {
		ProcessedUpToEpisodeIndex string              `json:"processedUpToEpisodeIndex"`
		Characters                []json.RawMessage   `json:"characters"`
		NewCharacters             []json.RawMessage   `json:"newCharacters"`
		CharacterUpdates          []json.RawMessage   `json:"characterUpdates"`
		MergeProposals            []MergeProposal     `json:"mergeProposals"`
		UnresolvedMentions        []UnresolvedMention `json:"unresolvedMentions"`
		Terms                     json.RawMessage     `json:"terms"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return Delta{}, errors.New("OpenRouter response was not valid JSON.")
	}
	if payload.Characters == nil && payload.NewCharacters == nil && payload.CharacterUpdates == nil {
		return Delta{}, errors.New("OpenRouter response did not match the expected extraction schema.")
	}
	if payload.Terms == nil || string(payload.Terms) == "null" {
		return Delta{}, errors.New("OpenRouter response did not match the expected extraction schema: terms is required.")
	}
	var rawTerms []json.RawMessage
	if err := json.Unmarshal(payload.Terms, &rawTerms); err != nil {
		return Delta{}, errors.New("OpenRouter response did not match the expected extraction schema: terms must be an array.")
	}
	if !isDigitsString(payload.ProcessedUpToEpisodeIndex) {
		payload.ProcessedUpToEpisodeIndex = fallbackEpisodeIndex
	}
	delta := Delta{
		MergeProposals:     normalizeMergeProposals(payload.MergeProposals),
		UnresolvedMentions: NormalizeUnresolvedMentions(payload.UnresolvedMentions, payload.ProcessedUpToEpisodeIndex, fallbackEpisodeIndex),
	}
	for _, rawTerm := range rawTerms {
		term, keep, err := normalizeOpenRouterTerm(rawTerm, payload.ProcessedUpToEpisodeIndex, fallbackEpisodeIndex)
		if err != nil {
			return Delta{}, err
		}
		if keep {
			delta.Terms = append(delta.Terms, term)
		}
	}
	delta.Terms = terms.ApplyTermDelta(nil, delta.Terms)
	seenIDs := map[string]bool{}
	for _, rawItem := range payload.Characters {
		character, ok := normalizeOpenRouterCharacter(rawItem, payload.ProcessedUpToEpisodeIndex, fallbackEpisodeIndex)
		if !ok {
			continue
		}
		id := strings.TrimSpace(character.CharacterID)
		if id == "" {
			id = GeneratedCharacterID(novelID, character.CanonicalName)
		}
		if seenIDs[id] {
			continue
		}
		seenIDs[id] = true
		delta.LegacyCharacters = append(delta.LegacyCharacters, character)
	}
	for _, rawItem := range payload.NewCharacters {
		character, ok := normalizeOpenRouterCharacter(rawItem, payload.ProcessedUpToEpisodeIndex, fallbackEpisodeIndex)
		if ok {
			delta.NewCharacters = append(delta.NewCharacters, character)
		}
	}
	for _, rawItem := range payload.CharacterUpdates {
		character, ok := normalizeOpenRouterCharacter(rawItem, payload.ProcessedUpToEpisodeIndex, fallbackEpisodeIndex)
		if ok && strings.TrimSpace(character.CharacterID) != "" {
			delta.CharacterUpdates = append(delta.CharacterUpdates, character)
		}
	}
	SortGeneratedCharacters(delta.LegacyCharacters)
	SortGeneratedCharacters(delta.NewCharacters)
	SortGeneratedCharacters(delta.CharacterUpdates)
	return delta, nil
}

func NormalizeOpenRouterResponseForEpisodes(raw []byte, novelID string, fallbackEpisodeIndex string, allowedEpisodeIndexes []string) (Delta, error) {
	delta, err := NormalizeOpenRouterResponse(raw, novelID, fallbackEpisodeIndex)
	if err != nil {
		return Delta{}, err
	}
	if err := ValidateDeltaEpisodeIndexes(delta, allowedEpisodeIndexes); err != nil {
		return Delta{}, err
	}
	return delta, nil
}

func ValidateDeltaEpisodeIndexes(delta Delta, allowedEpisodeIndexes []string) error {
	allowed := map[string]bool{}
	for _, episodeIndex := range allowedEpisodeIndexes {
		episodeIndex = strings.TrimSpace(episodeIndex)
		if episodeIndex != "" {
			allowed[episodeIndex] = true
		}
	}
	if len(allowed) == 0 {
		return nil
	}
	validate := func(episodeIndex string) bool {
		episodeIndex = strings.TrimSpace(episodeIndex)
		return episodeIndex == "" || allowed[episodeIndex]
	}
	validateTextVersions := func(values []characters.GeneratedTextVersion) bool {
		for _, value := range values {
			if !validate(value.EpisodeIndex) {
				return false
			}
		}
		return true
	}
	validateHistoryVersions := func(values []characters.GeneratedHistoryVersion) bool {
		for _, value := range values {
			if !validate(value.EpisodeIndex) {
				return false
			}
		}
		return true
	}
	validateCharacter := func(character characters.GeneratedCharacter) bool {
		if !validate(character.CanonicalEpisodeIndex) || !validate(character.FirstAppearanceEpisodeIndex) ||
			!validate(character.FullNameEpisodeIndex) || !validate(character.GenderEpisodeIndex) {
			return false
		}
		for _, values := range [][]characters.GeneratedTextVersion{
			character.NameHistory,
			character.FullNameHistory,
			character.GenderHistory,
			character.Aliases,
		} {
			if !validateTextVersions(values) {
				return false
			}
		}
		for _, values := range [][]characters.GeneratedHistoryVersion{
			character.AppearanceHistory,
			character.PersonalityHistory,
			character.SummaryHistory,
		} {
			if !validateHistoryVersions(values) {
				return false
			}
		}
		return true
	}
	for _, values := range [][]characters.GeneratedCharacter{delta.LegacyCharacters, delta.NewCharacters, delta.CharacterUpdates} {
		for _, character := range values {
			if !validateCharacter(character) {
				return extractionEpisodeBoundaryError()
			}
		}
	}
	for _, mention := range delta.UnresolvedMentions {
		if !validate(mention.EpisodeIndex) {
			return extractionEpisodeBoundaryError()
		}
	}
	for _, term := range delta.Terms {
		for _, value := range term.ReadingHistory {
			if !validate(value.EpisodeIndex) {
				return extractionEpisodeBoundaryError()
			}
		}
		for _, value := range term.CategoryHistory {
			if !validate(value.EpisodeIndex) {
				return extractionEpisodeBoundaryError()
			}
		}
		for _, value := range term.DescriptionHistory {
			if !validate(value.EpisodeIndex) {
				return extractionEpisodeBoundaryError()
			}
		}
	}
	return nil
}

func extractionEpisodeBoundaryError() error {
	return errors.New("OpenRouter response contained an episodeIndex outside the current extraction batch.")
}

func normalizeOpenRouterTerm(raw json.RawMessage, episodeIndexes ...string) (terms.GeneratedTerm, bool, error) {
	var item map[string]json.RawMessage
	if err := json.Unmarshal(raw, &item); err != nil {
		return terms.GeneratedTerm{}, false, extractionTermContractError()
	}
	for _, key := range []string{"term", "reading", "category", "descriptionHistory"} {
		if _, ok := item[key]; !ok {
			return terms.GeneratedTerm{}, false, extractionTermContractError()
		}
	}
	var termText string
	if err := json.Unmarshal(item["term"], &termText); err != nil {
		return terms.GeneratedTerm{}, false, extractionTermContractError()
	}
	termText = strings.TrimSpace(termText)
	readingHistory := []terms.TextVersion{}
	if string(item["reading"]) != "null" {
		value, err := decodeTermVersionObject(item["reading"], "text", episodeIndexes...)
		if err != nil {
			return terms.GeneratedTerm{}, false, err
		}
		if strings.TrimSpace(value["text"]) != "" {
			readingHistory = append(readingHistory, terms.TextVersion{Text: strings.TrimSpace(value["text"]), EpisodeIndex: value["episodeIndex"]})
		}
	}
	categoryValue, err := decodeTermVersionObject(item["category"], "value", episodeIndexes...)
	if err != nil {
		return terms.GeneratedTerm{}, false, err
	}
	categoryHistory := []terms.CategoryVersion{{
		Category:     terms.NormalizeCategory(strings.TrimSpace(categoryValue["value"])),
		EpisodeIndex: categoryValue["episodeIndex"],
	}}
	var rawDescriptions []json.RawMessage
	if err := json.Unmarshal(item["descriptionHistory"], &rawDescriptions); err != nil || len(rawDescriptions) == 0 {
		return terms.GeneratedTerm{}, false, extractionTermContractError()
	}
	descriptions := []terms.HistoryVersion{}
	for _, rawDescription := range rawDescriptions {
		value, err := decodeTermVersionObject(rawDescription, "text", episodeIndexes...)
		if err != nil {
			return terms.GeneratedTerm{}, false, err
		}
		if strings.TrimSpace(value["text"]) != "" {
			descriptions = append(descriptions, terms.HistoryVersion{Text: strings.TrimSpace(value["text"]), EpisodeIndex: value["episodeIndex"]})
		}
	}
	if termText == "" || len(descriptions) == 0 {
		return terms.GeneratedTerm{}, false, nil
	}
	return terms.GeneratedTerm{Term: termText, ReadingHistory: readingHistory, CategoryHistory: categoryHistory, DescriptionHistory: descriptions}, true, nil
}

func decodeTermVersionObject(raw json.RawMessage, valueKey string, episodeIndexes ...string) (map[string]string, error) {
	var item map[string]json.RawMessage
	if err := json.Unmarshal(raw, &item); err != nil || len(item) != 2 || item[valueKey] == nil || item["episodeIndex"] == nil {
		return nil, extractionTermContractError()
	}
	var value string
	var episodeIndex string
	if json.Unmarshal(item[valueKey], &value) != nil || json.Unmarshal(item["episodeIndex"], &episodeIndex) != nil {
		return nil, extractionTermContractError()
	}
	if !isDigitsString(episodeIndex) {
		episodeIndex = firstDigitsSummaryString(episodeIndexes...)
	}
	if !isDigitsString(episodeIndex) {
		return nil, extractionTermContractError()
	}
	return map[string]string{valueKey: value, "episodeIndex": episodeIndex}, nil
}

func extractionTermContractError() error {
	return errors.New("OpenRouter response did not match the expected extraction term schema.")
}

func normalizeMergeProposals(values []MergeProposal) []MergeProposal {
	result := []MergeProposal{}
	seen := map[string]bool{}
	for _, value := range values {
		source := strings.TrimSpace(value.SourceCharacterID)
		target := strings.TrimSpace(value.TargetCharacterID)
		if source == "" || target == "" || source == target {
			continue
		}
		key := source + "\x00" + target
		if seen[key] {
			continue
		}
		seen[key] = true
		confidence := value.Confidence
		if confidence < 0 {
			confidence = 0
		} else if confidence > 1 {
			confidence = 1
		}
		result = append(result, MergeProposal{
			SourceCharacterID: source,
			TargetCharacterID: target,
			Confidence:        confidence,
			Reason:            strings.TrimSpace(value.Reason),
		})
	}
	return result
}

func NormalizeUnresolvedMentions(values []UnresolvedMention, episodeIndexes ...string) []UnresolvedMention {
	result := []UnresolvedMention{}
	for _, value := range values {
		mention := normalizeSummaryWhitespace(value.Mention)
		episodeIndex := firstDigitsSummaryString(append([]string{value.EpisodeIndex}, episodeIndexes...)...)
		if mention == "" || episodeIndex == "" {
			continue
		}
		result = append(result, UnresolvedMention{
			Mention:      mention,
			EpisodeIndex: episodeIndex,
			Reason:       strings.TrimSpace(value.Reason),
		})
	}
	return result
}

func normalizeOpenRouterCharacter(raw json.RawMessage, processedEpisodeIndex string, fallbackEpisodeIndex string) (characters.GeneratedCharacter, bool) {
	var item map[string]json.RawMessage
	if err := json.Unmarshal(raw, &item); err != nil {
		return characters.GeneratedCharacter{}, false
	}
	characterID := decodeGeneratedString(item["characterId"])
	canonicalName, canonicalEpisodeIndex, ok := decodeGeneratedTextVersion(item["canonicalName"], processedEpisodeIndex, fallbackEpisodeIndex)
	if !ok {
		canonicalName = decodeGeneratedString(item["displayName"])
		canonicalEpisodeIndex = firstDigitsSummaryString(processedEpisodeIndex, fallbackEpisodeIndex)
		ok = canonicalName != "" && isDigitsString(canonicalEpisodeIndex)
	}
	if !ok && characterID == "" {
		return characters.GeneratedCharacter{}, false
	}
	firstAppearance := decodeGeneratedString(item["firstAppearanceEpisodeIndex"])
	if !isDigitsString(firstAppearance) {
		firstAppearance = canonicalEpisodeIndex
	}
	character := characters.GeneratedCharacter{
		CharacterID:                 characterID,
		CanonicalName:               canonicalName,
		CanonicalEpisodeIndex:       canonicalEpisodeIndex,
		FirstAppearanceEpisodeIndex: firstAppearance,
		Aliases:                     decodeGeneratedTextVersionList(item["aliases"], processedEpisodeIndex, fallbackEpisodeIndex),
		AppearanceHistory:           decodeGeneratedHistoryVersionList(item["appearanceHistory"], processedEpisodeIndex, fallbackEpisodeIndex),
		PersonalityHistory:          decodeGeneratedHistoryVersionList(item["personalityHistory"], processedEpisodeIndex, fallbackEpisodeIndex),
		SummaryHistory:              decodeGeneratedHistoryVersionList(item["summaryHistory"], processedEpisodeIndex, fallbackEpisodeIndex),
	}
	if canonicalName != "" && canonicalEpisodeIndex != "" {
		character.NameHistory = []characters.GeneratedTextVersion{{Text: canonicalName, EpisodeIndex: canonicalEpisodeIndex}}
	}
	if len(character.Aliases) == 0 && canonicalName != "" && canonicalEpisodeIndex != "" {
		character.Aliases = []characters.GeneratedTextVersion{{Text: canonicalName, EpisodeIndex: canonicalEpisodeIndex}}
	}
	if text, episodeIndex, ok := decodeGeneratedTextVersion(item["fullName"], processedEpisodeIndex, fallbackEpisodeIndex); ok {
		character.FullName = &text
		character.FullNameEpisodeIndex = episodeIndex
		character.FullNameHistory = MergeGeneratedTextVersionLists(character.FullNameHistory, []characters.GeneratedTextVersion{{Text: text, EpisodeIndex: episodeIndex}})
	}
	character.FullNameHistory = MergeGeneratedTextVersionLists(character.FullNameHistory, decodeGeneratedTextVersionList(item["fullNameHistory"], processedEpisodeIndex, fallbackEpisodeIndex))
	character.FullName, character.FullNameEpisodeIndex = LatestGeneratedTextVersionValue(character.FullName, character.FullNameEpisodeIndex, character.FullNameHistory)
	if text, episodeIndex, ok := decodeGeneratedTextVersion(item["gender"], processedEpisodeIndex, fallbackEpisodeIndex); ok {
		character.Gender = &text
		character.GenderEpisodeIndex = episodeIndex
		character.GenderHistory = MergeGeneratedTextVersionLists(character.GenderHistory, []characters.GeneratedTextVersion{{Text: text, EpisodeIndex: episodeIndex}})
	}
	character.GenderHistory = MergeGeneratedTextVersionLists(character.GenderHistory, decodeGeneratedTextVersionList(item["genderHistory"], processedEpisodeIndex, fallbackEpisodeIndex))
	character.Gender, character.GenderEpisodeIndex = LatestGeneratedTextVersionValue(character.Gender, character.GenderEpisodeIndex, character.GenderHistory)
	if len(character.AppearanceHistory) == 0 {
		if text := decodeGeneratedNullableString(item["appearance"]); text != "" {
			character.AppearanceHistory = []characters.GeneratedHistoryVersion{{EpisodeIndex: processedEpisodeIndex, Text: text}}
		}
	}
	if len(character.PersonalityHistory) == 0 {
		if text := decodeGeneratedNullableString(item["personality"]); text != "" {
			character.PersonalityHistory = []characters.GeneratedHistoryVersion{{EpisodeIndex: processedEpisodeIndex, Text: text}}
		}
	}
	if len(character.SummaryHistory) == 0 {
		if text := decodeGeneratedNullableString(item["summary"]); text != "" {
			character.SummaryHistory = []characters.GeneratedHistoryVersion{{EpisodeIndex: processedEpisodeIndex, Text: text}}
		}
	}
	character.Aliases = FilterGeneratedCharacterAliases(character)
	return character, true
}

func decodeGeneratedString(raw json.RawMessage) string {
	var text string
	if len(raw) == 0 || json.Unmarshal(raw, &text) != nil {
		return ""
	}
	return normalizeSummaryWhitespace(text)
}

func decodeGeneratedNullableString(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	return decodeGeneratedString(raw)
}

func decodeGeneratedTextVersion(raw json.RawMessage, episodeIndexes ...string) (string, string, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", "", false
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		text = normalizeSummaryWhitespace(text)
		episodeIndex := firstDigitsSummaryString(episodeIndexes...)
		return text, episodeIndex, text != "" && isDigitsString(episodeIndex)
	}
	var version characters.GeneratedTextVersion
	if err := json.Unmarshal(raw, &version); err != nil {
		return "", "", false
	}
	text = normalizeSummaryWhitespace(version.Text)
	candidates := append([]string{version.EpisodeIndex}, episodeIndexes...)
	episodeIndex := firstDigitsSummaryString(candidates...)
	return text, episodeIndex, text != "" && isDigitsString(episodeIndex)
}

func decodeGeneratedTextVersionList(raw json.RawMessage, episodeIndexes ...string) []characters.GeneratedTextVersion {
	if len(raw) == 0 || string(raw) == "null" {
		return []characters.GeneratedTextVersion{}
	}
	var values []json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		return []characters.GeneratedTextVersion{}
	}
	result := []characters.GeneratedTextVersion{}
	for _, value := range values {
		text, episodeIndex, ok := decodeGeneratedTextVersion(value, episodeIndexes...)
		if ok {
			result = append(result, characters.GeneratedTextVersion{Text: text, EpisodeIndex: episodeIndex})
		}
	}
	return NormalizeGeneratedTextVersions(result)
}

func decodeGeneratedHistoryVersionList(raw json.RawMessage, episodeIndexes ...string) []characters.GeneratedHistoryVersion {
	if len(raw) == 0 || string(raw) == "null" {
		return []characters.GeneratedHistoryVersion{}
	}
	var values []characters.GeneratedHistoryVersion
	if err := json.Unmarshal(raw, &values); err != nil {
		return []characters.GeneratedHistoryVersion{}
	}
	fallbackEpisodeIndex := FirstNonEmptyString(episodeIndexes...)
	for index := range values {
		if !isDigitsString(values[index].EpisodeIndex) {
			values[index].EpisodeIndex = fallbackEpisodeIndex
		}
	}
	return NormalizeGeneratedHistoryVersions(values)
}

func firstDigitsSummaryString(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" && isDigitsString(trimmed) {
			return trimmed
		}
	}
	return ""
}

func NormalizeGeneratedTextVersions(values []characters.GeneratedTextVersion) []characters.GeneratedTextVersion {
	result := []characters.GeneratedTextVersion{}
	seen := map[string]bool{}
	for _, value := range values {
		text := normalizeSummaryWhitespace(value.Text)
		if text == "" || !isDigitsString(value.EpisodeIndex) {
			continue
		}
		key := value.EpisodeIndex + "\x00" + text
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, characters.GeneratedTextVersion{Text: text, EpisodeIndex: value.EpisodeIndex})
	}
	sort.SliceStable(result, func(i, j int) bool {
		diff := CompareEpisodeString(result[i].EpisodeIndex, result[j].EpisodeIndex)
		if diff != 0 {
			return diff < 0
		}
		return result[i].Text < result[j].Text
	})
	return result
}

func NormalizeGeneratedHistoryVersions(values []characters.GeneratedHistoryVersion) []characters.GeneratedHistoryVersion {
	result := []characters.GeneratedHistoryVersion{}
	seen := map[string]bool{}
	for _, value := range values {
		text := normalizeSummaryWhitespace(value.Text)
		if text == "" || !isDigitsString(value.EpisodeIndex) {
			continue
		}
		key := value.EpisodeIndex + "\x00" + text
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, characters.GeneratedHistoryVersion{EpisodeIndex: value.EpisodeIndex, Text: text})
	}
	sort.SliceStable(result, func(i, j int) bool {
		diff := CompareEpisodeString(result[i].EpisodeIndex, result[j].EpisodeIndex)
		if diff != 0 {
			return diff < 0
		}
		return result[i].Text < result[j].Text
	})
	return result
}

func MergeGeneratedCharacters(existing []characters.GeneratedCharacter, incoming []characters.GeneratedCharacter) []characters.GeneratedCharacter {
	merged := append([]characters.GeneratedCharacter{}, existing...)
	for _, next := range incoming {
		next.Aliases = FilterGeneratedCharacterAliases(next)
		matched := []int{}
		for index, current := range merged {
			if generatedCharactersShareIdentity(current, next) {
				matched = append(matched, index)
			}
		}
		if len(matched) == 0 {
			merged = append(merged, next)
			continue
		}
		combined := merged[matched[0]]
		for _, index := range matched[1:] {
			combined = MergeGeneratedCharacter(combined, merged[index])
		}
		combined = MergeGeneratedCharacter(combined, next)
		for index := len(matched) - 1; index >= 0; index-- {
			remove := matched[index]
			merged = append(merged[:remove], merged[remove+1:]...)
		}
		merged = append(merged, combined)
	}
	SortGeneratedCharacters(merged)
	return merged
}

func ReuseGeneratedCharacterIDsFromRegistry(generated []characters.GeneratedCharacter, registry []characters.GeneratedCharacter, state GenerationState, upToEpisodeIndex string) ([]characters.GeneratedCharacter, GenerationState) {
	if len(generated) == 0 || len(registry) == 0 {
		return generated, state
	}
	identityFrequency := IdentityFrequency(registry)
	idsByKey := map[string]map[string]bool{}
	for _, item := range registry {
		id := strings.TrimSpace(item.CharacterID)
		if id == "" {
			continue
		}
		firstAppearance := FirstNonEmptyString(item.FirstAppearanceEpisodeIndex, item.CanonicalEpisodeIndex)
		if strings.TrimSpace(upToEpisodeIndex) != "" && firstAppearance != "" && CompareEpisodeString(firstAppearance, upToEpisodeIndex) > 0 {
			continue
		}
		for _, key := range generatedMergeIdentityKeys(item) {
			key = strings.ToLower(strings.TrimSpace(key))
			if !ExactCandidateKey(key, identityFrequency) {
				continue
			}
			if idsByKey[key] == nil {
				idsByKey[key] = map[string]bool{}
			}
			idsByKey[key][id] = true
		}
	}
	uniqueIDByKey := map[string]string{}
	for key, ids := range idsByKey {
		if len(ids) != 1 {
			continue
		}
		for id := range ids {
			uniqueIDByKey[key] = id
		}
	}
	if len(uniqueIDByKey) == 0 {
		return generated, state
	}
	usedGeneratedIDs := map[string]bool{}
	for _, item := range generated {
		if id := strings.TrimSpace(item.CharacterID); id != "" {
			usedGeneratedIDs[id] = true
		}
	}
	result := append([]characters.GeneratedCharacter{}, generated...)
	for index, item := range result {
		currentID := strings.TrimSpace(item.CharacterID)
		targetID := ""
		for _, key := range generatedMergeIdentityKeys(item) {
			key = strings.ToLower(strings.TrimSpace(key))
			if !ExactCandidateKey(key, identityFrequency) {
				continue
			}
			id := uniqueIDByKey[key]
			if id == "" {
				continue
			}
			if targetID != "" && targetID != id {
				targetID = ""
				break
			}
			targetID = id
		}
		if targetID == "" || targetID == currentID {
			continue
		}
		delete(usedGeneratedIDs, currentID)
		result[index].CharacterID = targetID
		usedGeneratedIDs[targetID] = true
		if currentID != "" {
			state.RetiredCharacterIDs = append(state.RetiredCharacterIDs, characters.GeneratedRetiredCharacterID{CharacterID: currentID, MergedInto: targetID})
		}
	}
	result = MergeGeneratedCharactersByID(result)
	state.RetiredCharacterIDs = NormalizeGeneratedRetiredCharacterIDs(state.RetiredCharacterIDs)
	return result, state
}

func MergeGeneratedCharactersByID(values []characters.GeneratedCharacter) []characters.GeneratedCharacter {
	byID := map[string]characters.GeneratedCharacter{}
	withoutID := []characters.GeneratedCharacter{}
	order := []string{}
	for _, value := range values {
		id := strings.TrimSpace(value.CharacterID)
		if id == "" {
			withoutID = append(withoutID, value)
			continue
		}
		if existing, ok := byID[id]; ok {
			byID[id] = MergeGeneratedCharacter(existing, value)
			continue
		}
		byID[id] = value
		order = append(order, id)
	}
	result := make([]characters.GeneratedCharacter, 0, len(withoutID)+len(byID))
	result = append(result, withoutID...)
	for _, id := range order {
		result = append(result, byID[id])
	}
	SortGeneratedCharacters(result)
	return result
}

func NormalizeGeneratedRetiredCharacterIDs(values []characters.GeneratedRetiredCharacterID) []characters.GeneratedRetiredCharacterID {
	byID := map[string]characters.GeneratedRetiredCharacterID{}
	for _, value := range values {
		id := strings.TrimSpace(value.CharacterID)
		if id == "" {
			continue
		}
		existing := byID[id]
		existing.CharacterID = id
		if mergedInto := strings.TrimSpace(value.MergedInto); mergedInto != "" {
			existing.MergedInto = mergedInto
		}
		byID[id] = existing
	}
	result := make([]characters.GeneratedRetiredCharacterID, 0, len(byID))
	for _, value := range byID {
		result = append(result, value)
	}
	sort.SliceStable(result, func(i, j int) bool {
		return result[i].CharacterID < result[j].CharacterID
	})
	return result
}

func ApplyDelta(novelID string, existing []characters.GeneratedCharacter, delta Delta, allocator *characters.GeneratedCharacterIDAllocator) ([]characters.GeneratedCharacter, int) {
	generated := append([]characters.GeneratedCharacter{}, existing...)
	changed := 0
	if len(delta.LegacyCharacters) > 0 {
		next := assignGeneratedCharactersForDelta(novelID, generated, MergeGeneratedCharacters(nil, delta.LegacyCharacters), allocator)
		generated = MergeGeneratedCharacters(generated, next)
		return generated, len(next)
	}
	if len(delta.NewCharacters) > 0 {
		next := assignGeneratedCharactersForDelta(novelID, generated, MergeGeneratedCharacters(nil, delta.NewCharacters), allocator)
		for _, item := range next {
			before := len(generated)
			generated = MergeGeneratedCharacters(generated, []characters.GeneratedCharacter{item})
			if len(generated) != before || strings.TrimSpace(item.CharacterID) != "" {
				changed++
			}
		}
	}
	for _, update := range delta.CharacterUpdates {
		if strings.TrimSpace(update.CharacterID) == "" {
			continue
		}
		index := GeneratedCharacterIndexByID(generated, update.CharacterID)
		if index < 0 {
			continue
		}
		generated[index] = MergeGeneratedCharacter(generated[index], update)
		changed++
	}
	generated, changed = ApplyMergeProposals(generated, delta.MergeProposals, changed, allocator)
	SortGeneratedCharacters(generated)
	return generated, changed
}

func CharacterNameSet(generated []characters.GeneratedCharacter) map[string]bool {
	result := map[string]bool{}
	for _, character := range generated {
		for _, value := range []string{character.CanonicalName, valueOrEmptyString(character.FullName)} {
			if value = strings.TrimSpace(value); value != "" {
				result[value] = true
			}
		}
		for _, versions := range [][]characters.GeneratedTextVersion{character.NameHistory, character.FullNameHistory, character.Aliases} {
			for _, version := range versions {
				if value := strings.TrimSpace(version.Text); value != "" {
					result[value] = true
				}
			}
		}
	}
	return result
}

func FilterAndMergeTermDeltas(existing []terms.GeneratedTerm, incoming []terms.GeneratedTerm, generatedCharacters []characters.GeneratedCharacter) []terms.GeneratedTerm {
	names := CharacterNameSet(generatedCharacters)
	merged := terms.ApplyTermDelta(existing, incoming)
	filtered := make([]terms.GeneratedTerm, 0, len(merged))
	for _, term := range merged {
		if !names[strings.TrimSpace(term.Term)] {
			filtered = append(filtered, term)
		}
	}
	return filtered
}

func valueOrEmptyString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func assignGeneratedCharactersForDelta(novelID string, existing []characters.GeneratedCharacter, incoming []characters.GeneratedCharacter, allocator *characters.GeneratedCharacterIDAllocator) []characters.GeneratedCharacter {
	if allocator != nil {
		return allocator.Assign(incoming)
	}
	return characters.AssignGeneratedCharacterIDs(novelID, existing, incoming)
}

func ApplyMergeProposals(generated []characters.GeneratedCharacter, proposals []MergeProposal, changed int, allocator *characters.GeneratedCharacterIDAllocator) ([]characters.GeneratedCharacter, int) {
	if len(proposals) == 0 || len(generated) < 2 {
		return generated, changed
	}
	indexByID := map[string]int{}
	for index, item := range generated {
		if strings.TrimSpace(item.CharacterID) != "" {
			indexByID[item.CharacterID] = index
		}
	}
	parent := map[string]string{}
	var find func(string) string
	find = func(value string) string {
		if parent[value] == "" {
			parent[value] = value
			return value
		}
		if parent[value] != value {
			parent[value] = find(parent[value])
		}
		return parent[value]
	}
	union := func(left, right string) {
		leftRoot := find(left)
		rightRoot := find(right)
		if leftRoot != rightRoot {
			parent[rightRoot] = leftRoot
		}
	}
	for _, proposal := range proposals {
		if proposal.Confidence < MergeAutoApplyConfidence {
			continue
		}
		source := strings.TrimSpace(proposal.SourceCharacterID)
		target := strings.TrimSpace(proposal.TargetCharacterID)
		if source == "" || target == "" || source == target {
			continue
		}
		if _, ok := indexByID[source]; !ok {
			continue
		}
		if _, ok := indexByID[target]; !ok {
			continue
		}
		union(source, target)
	}
	components := map[string][]string{}
	for id := range parent {
		root := find(id)
		components[root] = append(components[root], id)
	}
	removeIDs := map[string]bool{}
	for _, ids := range components {
		if len(ids) < 2 {
			continue
		}
		sort.Strings(ids)
		representativeID := MergeRepresentativeID(generated, indexByID, ids)
		representativeIndex := indexByID[representativeID]
		merged := generated[representativeIndex]
		for _, id := range ids {
			if id == representativeID {
				continue
			}
			source := generated[indexByID[id]]
			source.CharacterID = representativeID
			merged = MergeGeneratedCharacter(merged, source)
			removeIDs[id] = true
			if allocator != nil {
				allocator.Retire(id, representativeID)
			}
			changed++
		}
		generated[representativeIndex] = merged
	}
	if len(removeIDs) == 0 {
		return generated, changed
	}
	result := make([]characters.GeneratedCharacter, 0, len(generated)-len(removeIDs))
	for _, item := range generated {
		if removeIDs[item.CharacterID] {
			continue
		}
		result = append(result, item)
	}
	return result, changed
}

func MergeRepresentativeID(generated []characters.GeneratedCharacter, indexByID map[string]int, ids []string) string {
	best := ids[0]
	for _, id := range ids[1:] {
		left := generated[indexByID[id]]
		right := generated[indexByID[best]]
		diff := CompareEpisodeString(FirstNonEmptyString(left.FirstAppearanceEpisodeIndex, left.CanonicalEpisodeIndex), FirstNonEmptyString(right.FirstAppearanceEpisodeIndex, right.CanonicalEpisodeIndex))
		if diff < 0 || (diff == 0 && id < best) {
			best = id
		}
	}
	return best
}

func MergeGeneratedUnresolvedMentions(existing []characters.GeneratedUnresolvedMention, incoming []UnresolvedMention) []characters.GeneratedUnresolvedMention {
	result := append([]characters.GeneratedUnresolvedMention{}, existing...)
	seen := map[string]bool{}
	for _, value := range result {
		key := strings.TrimSpace(value.EpisodeIndex) + "\x00" + strings.TrimSpace(value.Mention)
		if key != "\x00" {
			seen[key] = true
		}
	}
	for _, value := range incoming {
		mention := strings.TrimSpace(value.Mention)
		episodeIndex := strings.TrimSpace(value.EpisodeIndex)
		if mention == "" || episodeIndex == "" {
			continue
		}
		key := episodeIndex + "\x00" + mention
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, characters.GeneratedUnresolvedMention{
			Mention:      mention,
			EpisodeIndex: episodeIndex,
			Reason:       strings.TrimSpace(value.Reason),
		})
	}
	sort.SliceStable(result, func(i, j int) bool {
		diff := CompareEpisodeString(result[i].EpisodeIndex, result[j].EpisodeIndex)
		if diff != 0 {
			return diff < 0
		}
		return result[i].Mention < result[j].Mention
	})
	return result
}

func FilterResolvedGeneratedUnresolvedMentions(values []characters.GeneratedUnresolvedMention, generated []characters.GeneratedCharacter) []characters.GeneratedUnresolvedMention {
	if len(values) == 0 || len(generated) == 0 {
		return values
	}
	idsByKey := map[string]map[string]bool{}
	for _, character := range generated {
		characterID := strings.TrimSpace(character.CharacterID)
		if characterID == "" {
			continue
		}
		for _, key := range GeneratedIdentityKeys(character) {
			key = strings.TrimSpace(key)
			if key != "" && !strings.HasPrefix(key, "id:") {
				if idsByKey[key] == nil {
					idsByKey[key] = map[string]bool{}
				}
				idsByKey[key][characterID] = true
			}
		}
	}
	result := []characters.GeneratedUnresolvedMention{}
	for _, value := range values {
		ids := idsByKey[strings.TrimSpace(value.Mention)]
		if len(ids) != 1 {
			result = append(result, value)
			continue
		}
		if len(value.CandidateIDs) > 0 {
			candidates := NormalizeSummaryStringList(value.CandidateIDs)
			if len(candidates) != 1 {
				result = append(result, value)
				continue
			}
			matched := false
			for id := range ids {
				if candidates[0] != id {
					continue
				}
				matched = true
				break
			}
			if !matched {
				result = append(result, value)
			}
			continue
		}
	}
	return result
}

func NormalizeSummaryStringList(values []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func GeneratedCharacterIndexByID(values []characters.GeneratedCharacter, characterID string) int {
	characterID = strings.TrimSpace(characterID)
	if characterID == "" {
		return -1
	}
	for index, value := range values {
		if strings.TrimSpace(value.CharacterID) == characterID {
			return index
		}
	}
	return -1
}

func generatedCharactersShareIdentity(left characters.GeneratedCharacter, right characters.GeneratedCharacter) bool {
	leftID := strings.TrimSpace(left.CharacterID)
	rightID := strings.TrimSpace(right.CharacterID)
	if leftID != "" || rightID != "" {
		return leftID != "" && leftID == rightID
	}
	keys := map[string]bool{}
	for _, value := range generatedMergeIdentityKeys(left) {
		keys[value] = true
	}
	for _, value := range generatedMergeIdentityKeys(right) {
		if keys[value] {
			return true
		}
	}
	return false
}

func GeneratedIdentityKeys(value characters.GeneratedCharacter) []string {
	keys := []string{}
	if strings.TrimSpace(value.CharacterID) != "" {
		keys = append(keys, "id:"+strings.TrimSpace(value.CharacterID))
	}
	if strings.TrimSpace(value.CanonicalName) != "" {
		keys = append(keys, strings.TrimSpace(value.CanonicalName))
	}
	if value.FullName != nil && strings.TrimSpace(*value.FullName) != "" {
		keys = append(keys, strings.TrimSpace(*value.FullName))
	}
	for _, fullName := range value.FullNameHistory {
		if strings.TrimSpace(fullName.Text) != "" {
			keys = append(keys, strings.TrimSpace(fullName.Text))
		}
	}
	for _, alias := range value.Aliases {
		if strings.TrimSpace(alias.Text) != "" {
			keys = append(keys, strings.TrimSpace(alias.Text))
		}
	}
	return keys
}

func generatedMergeIdentityKeys(value characters.GeneratedCharacter) []string {
	keys := []string{}
	for _, key := range GeneratedIdentityKeys(value) {
		key = strings.TrimSpace(key)
		if key == "" || strings.HasPrefix(strings.ToLower(key), "id:") || isGenericCharacterAlias(key) {
			continue
		}
		keys = append(keys, key)
	}
	return keys
}

func FilterGeneratedCharacterAliases(value characters.GeneratedCharacter) []characters.GeneratedTextVersion {
	protected := map[string]bool{}
	for _, text := range []string{value.CanonicalName} {
		text = strings.TrimSpace(text)
		if text != "" {
			protected[text] = true
		}
	}
	if value.FullName != nil {
		text := strings.TrimSpace(*value.FullName)
		if text != "" {
			protected[text] = true
		}
	}
	for _, history := range [][]characters.GeneratedTextVersion{value.NameHistory, value.FullNameHistory} {
		for _, item := range history {
			text := strings.TrimSpace(item.Text)
			if text != "" {
				protected[text] = true
			}
		}
	}
	result := []characters.GeneratedTextVersion{}
	for _, alias := range value.Aliases {
		text := strings.TrimSpace(alias.Text)
		if text == "" {
			continue
		}
		if isGenericCharacterAlias(text) && !protected[text] {
			continue
		}
		result = append(result, characters.GeneratedTextVersion{Text: text, EpisodeIndex: alias.EpisodeIndex})
	}
	return NormalizeGeneratedTextVersions(result)
}

func isGenericCharacterAlias(value string) bool {
	switch strings.TrimSpace(value) {
	case "先生", "隊長", "隊長代理", "工房長", "副官",
		"伯母", "伯母さん", "叔母", "叔母さん", "兄", "兄さん", "姉", "姉さん",
		"鍵の人", "白い影", "青い手の少年", "古道具屋の主人",
		"主人", "少年", "少女", "男", "女", "男性", "女性":
		return true
	default:
		return false
	}
}

func MergeGeneratedCharacter(left characters.GeneratedCharacter, right characters.GeneratedCharacter) characters.GeneratedCharacter {
	result := left
	if strings.TrimSpace(result.CharacterID) == "" {
		result.CharacterID = strings.TrimSpace(right.CharacterID)
	}
	result.NameHistory = MergeGeneratedTextVersionLists(result.NameHistory, right.NameHistory)
	if CompareEpisodeString(right.FirstAppearanceEpisodeIndex, result.FirstAppearanceEpisodeIndex) < 0 || result.FirstAppearanceEpisodeIndex == "" {
		result.FirstAppearanceEpisodeIndex = right.FirstAppearanceEpisodeIndex
	}
	if strings.TrimSpace(result.CanonicalName) != "" && strings.TrimSpace(result.CanonicalEpisodeIndex) != "" {
		result.NameHistory = MergeGeneratedTextVersionLists(result.NameHistory, []characters.GeneratedTextVersion{{Text: result.CanonicalName, EpisodeIndex: result.CanonicalEpisodeIndex}})
	}
	if strings.TrimSpace(right.CanonicalName) != "" && strings.TrimSpace(right.CanonicalEpisodeIndex) != "" {
		result.NameHistory = MergeGeneratedTextVersionLists(result.NameHistory, []characters.GeneratedTextVersion{{Text: right.CanonicalName, EpisodeIndex: right.CanonicalEpisodeIndex}})
	}
	if strings.TrimSpace(right.CanonicalName) != "" && (CompareEpisodeString(right.CanonicalEpisodeIndex, result.CanonicalEpisodeIndex) >= 0 || result.CanonicalEpisodeIndex == "") {
		if strings.TrimSpace(result.CanonicalName) != "" && result.CanonicalName != right.CanonicalName {
			result.Aliases = MergeGeneratedTextVersionLists(result.Aliases, []characters.GeneratedTextVersion{{Text: result.CanonicalName, EpisodeIndex: result.CanonicalEpisodeIndex}})
		}
		result.CanonicalName = right.CanonicalName
		result.CanonicalEpisodeIndex = right.CanonicalEpisodeIndex
	}
	result.FullNameHistory = MergeGeneratedTextVersionLists(result.FullNameHistory, GeneratedTextVersionFromPtr(result.FullName, result.FullNameEpisodeIndex), right.FullNameHistory, GeneratedTextVersionFromPtr(right.FullName, right.FullNameEpisodeIndex))
	result.FullName, result.FullNameEpisodeIndex = LatestGeneratedTextVersionValue(result.FullName, result.FullNameEpisodeIndex, result.FullNameHistory)
	result.GenderHistory = MergeGeneratedTextVersionLists(result.GenderHistory, GeneratedTextVersionFromPtr(result.Gender, result.GenderEpisodeIndex), right.GenderHistory, GeneratedTextVersionFromPtr(right.Gender, right.GenderEpisodeIndex))
	result.Gender, result.GenderEpisodeIndex = LatestGeneratedTextVersionValue(result.Gender, result.GenderEpisodeIndex, result.GenderHistory)
	result.Aliases = MergeGeneratedTextVersionLists(result.Aliases, right.Aliases)
	if strings.TrimSpace(result.CanonicalName) != "" && strings.TrimSpace(result.CanonicalEpisodeIndex) != "" {
		result.Aliases = MergeGeneratedTextVersionLists(result.Aliases, []characters.GeneratedTextVersion{{Text: result.CanonicalName, EpisodeIndex: result.CanonicalEpisodeIndex}})
	}
	result.AppearanceHistory = MergeGeneratedHistoryVersionLists(result.AppearanceHistory, right.AppearanceHistory)
	result.PersonalityHistory = MergeGeneratedHistoryVersionLists(result.PersonalityHistory, right.PersonalityHistory)
	result.SummaryHistory = MergeGeneratedHistoryVersionLists(result.SummaryHistory, right.SummaryHistory)
	result.Aliases = FilterGeneratedCharacterAliases(result)
	return result
}

func GeneratedTextVersionFromPtr(text *string, episodeIndex string) []characters.GeneratedTextVersion {
	if text == nil || strings.TrimSpace(*text) == "" || strings.TrimSpace(episodeIndex) == "" {
		return nil
	}
	return []characters.GeneratedTextVersion{{Text: strings.TrimSpace(*text), EpisodeIndex: strings.TrimSpace(episodeIndex)}}
}

func LatestGeneratedTextVersionValue(current *string, currentEpisodeIndex string, history []characters.GeneratedTextVersion) (*string, string) {
	latestText := ""
	latestEpisodeIndex := ""
	if current != nil && strings.TrimSpace(*current) != "" && strings.TrimSpace(currentEpisodeIndex) != "" {
		latestText = strings.TrimSpace(*current)
		latestEpisodeIndex = strings.TrimSpace(currentEpisodeIndex)
	}
	for _, value := range history {
		text := strings.TrimSpace(value.Text)
		episodeIndex := strings.TrimSpace(value.EpisodeIndex)
		if text == "" || episodeIndex == "" {
			continue
		}
		if latestEpisodeIndex == "" || CompareEpisodeString(episodeIndex, latestEpisodeIndex) >= 0 {
			latestText = text
			latestEpisodeIndex = episodeIndex
		}
	}
	if latestText == "" || latestEpisodeIndex == "" {
		return nil, ""
	}
	return &latestText, latestEpisodeIndex
}

func MergeGeneratedTextVersionLists(lists ...[]characters.GeneratedTextVersion) []characters.GeneratedTextVersion {
	values := []characters.GeneratedTextVersion{}
	for _, list := range lists {
		values = append(values, list...)
	}
	return NormalizeGeneratedTextVersions(values)
}

func MergeGeneratedHistoryVersionLists(lists ...[]characters.GeneratedHistoryVersion) []characters.GeneratedHistoryVersion {
	values := []characters.GeneratedHistoryVersion{}
	for _, list := range lists {
		values = append(values, list...)
	}
	return NormalizeGeneratedHistoryVersions(values)
}

func SortGeneratedCharacters(values []characters.GeneratedCharacter) {
	sort.SliceStable(values, func(i, j int) bool {
		diff := CompareEpisodeString(values[i].FirstAppearanceEpisodeIndex, values[j].FirstAppearanceEpisodeIndex)
		if diff != 0 {
			return diff < 0
		}
		return values[i].CanonicalName < values[j].CanonicalName
	})
}

func GeneratedCharacterID(novelID string, canonicalName string) string {
	sum := sha1.Sum([]byte(novelID + ":" + canonicalName))
	return "char_" + hex.EncodeToString(sum[:])[:12]
}

func isDigitsString(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func MaxInt(left int, right int) int {
	if left > right {
		return left
	}
	return right
}

func EstimateTokenCount(value string) int {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0
	}
	runes := len([]rune(trimmed))
	return MaxInt(runes, (len(trimmed)+3)/4)
}

func CompareEpisodeString(left string, right string) int {
	leftNumber, leftErr := strconv.Atoi(left)
	rightNumber, rightErr := strconv.Atoi(right)
	if leftErr == nil && rightErr == nil {
		if leftNumber < rightNumber {
			return -1
		}
		if leftNumber > rightNumber {
			return 1
		}
		return 0
	}
	return strings.Compare(left, right)
}
