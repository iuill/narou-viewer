package readerproofread

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	"narou-viewer/apps/viewer-api-go/internal/ai"
	"narou-viewer/apps/viewer-api-go/internal/fsatomic"
	"narou-viewer/apps/viewer-api-go/internal/library"
	"narou-viewer/apps/viewer-api-go/internal/store"
)

const promptVersion = 5

var ErrUnavailable = errors.New("AI校正はLLM連携が未設定のため利用できません。AI機能の設定でOpenRouter APIキーとモデルを設定してください。")
var ErrUnsupportedDocument = errors.New("この話にはAI校正できる本文がありません。")
var ErrInvalidEpisodeIndex = errors.New("episodeIndex must be a non-negative integer string")
var ErrOutputTooLong = errors.New("この話はAI校正の出力上限を超えました。出力上限の大きいモデルを選択してください。")

type Library interface {
	GetEpisode(context.Context, string, string) (*library.EpisodeResponse, error)
}

type Settings interface {
	ResolveActiveAIGenerationConfig() (*store.ResolvedAIGenerationConfig, error)
	GetNovelReaderSettings(string) (store.NovelReaderSettings, error)
}

type GenerateFunc func(context.Context, ai.OpenRouterConfig, []ai.ChatMessage) (ai.ChatResult, error)

type Dependencies struct {
	Library     Library
	Settings    Settings
	StateDir    string
	UsageDBPath string
	Generate    GenerateFunc
}

type Service struct {
	library     Library
	settings    Settings
	stateDir    string
	usageDBPath string
	generate    GenerateFunc
	flightMu    sync.Mutex
	flights     map[string]*generationFlight
}

type generationFlight struct {
	done     chan struct{}
	response Response
	err      error
}

type Response struct {
	Status         string                  `json:"status"`
	NovelID        string                  `json:"novelId"`
	EpisodeIndex   string                  `json:"episodeIndex"`
	SourceETag     string                  `json:"sourceEtag"`
	GeneratedAt    *string                 `json:"generatedAt"`
	ModelID        *string                 `json:"modelId"`
	ReaderDocument *library.ReaderDocument `json:"readerDocument,omitempty"`
}

type storedResult struct {
	FormatVersion  int                    `json:"formatVersion"`
	PromptVersion  int                    `json:"promptVersion"`
	NovelID        string                 `json:"novelId"`
	EpisodeIndex   string                 `json:"episodeIndex"`
	SourceETag     string                 `json:"sourceEtag"`
	GeneratedAt    string                 `json:"generatedAt"`
	ModelID        string                 `json:"modelId"`
	ReaderDocument library.ReaderDocument `json:"readerDocument"`
}

type proofreadOutput struct {
	Segments []proofreadSegment `json:"segments"`
}

type proofreadSegment struct {
	ID         int      `json:"id"`
	Paragraphs []string `json:"paragraphs"`
}

type sourceSegment struct {
	ID         int
	StartBlock int
	EndBlock   int
	Section    string
	Paragraphs []string
}

func NewService(deps Dependencies) *Service {
	generate := deps.Generate
	if generate == nil {
		generate = ai.GenerateOpenRouterChat
	}
	return &Service{
		library: deps.Library, settings: deps.Settings, stateDir: deps.StateDir,
		usageDBPath: deps.UsageDBPath, generate: generate, flights: map[string]*generationFlight{},
	}
}

func (s *Service) Get(ctx context.Context, novelID string, episodeIndex string) (Response, error) {
	if !isValidEpisodeIndex(episodeIndex) {
		return Response{}, ErrInvalidEpisodeIndex
	}
	episode, err := s.loadEpisode(ctx, novelID, episodeIndex)
	if err != nil || episode == nil {
		return Response{}, err
	}
	result, ok, err := s.read(novelID, episodeIndex)
	if err != nil {
		return Response{}, err
	}
	if !ok || result.SourceETag != episode.ContentEtag || result.PromptVersion != promptVersion {
		return Response{Status: "not_generated", NovelID: novelID, EpisodeIndex: episodeIndex, SourceETag: episode.ContentEtag}, nil
	}
	generatedAt := result.GeneratedAt
	modelID := result.ModelID
	displayDocument, err := s.applyReaderCorrections(novelID, result.ReaderDocument)
	if err != nil {
		return Response{}, err
	}
	return Response{
		Status: "ready", NovelID: novelID, EpisodeIndex: episodeIndex, SourceETag: result.SourceETag,
		GeneratedAt: &generatedAt, ModelID: &modelID, ReaderDocument: &displayDocument,
	}, nil
}

func (s *Service) Generate(ctx context.Context, novelID string, episodeIndex string) (Response, error) {
	if !isValidEpisodeIndex(episodeIndex) {
		return Response{}, ErrInvalidEpisodeIndex
	}
	key := novelID + "\x00" + episodeIndex
	s.flightMu.Lock()
	if flight, ok := s.flights[key]; ok {
		s.flightMu.Unlock()
		select {
		case <-flight.done:
			return flight.response, flight.err
		case <-ctx.Done():
			return Response{}, ctx.Err()
		}
	}
	flight := &generationFlight{done: make(chan struct{})}
	s.flights[key] = flight
	s.flightMu.Unlock()

	flight.response, flight.err = s.generateOnce(ctx, novelID, episodeIndex)
	s.flightMu.Lock()
	delete(s.flights, key)
	close(flight.done)
	s.flightMu.Unlock()
	return flight.response, flight.err
}

func (s *Service) generateOnce(ctx context.Context, novelID string, episodeIndex string) (Response, error) {
	episode, err := s.loadEpisode(ctx, novelID, episodeIndex)
	if err != nil || episode == nil {
		return Response{}, err
	}
	segments := editableSegments(episode.ReaderDocument)
	if len(segments) == 0 {
		return Response{}, ErrUnsupportedDocument
	}
	if s.settings == nil {
		return Response{}, ErrUnavailable
	}
	config, err := s.settings.ResolveActiveAIGenerationConfig()
	if err != nil {
		return Response{}, err
	}
	if config == nil {
		return Response{}, ErrUnavailable
	}
	input := proofreadOutput{Segments: make([]proofreadSegment, 0, len(segments))}
	for _, segment := range segments {
		input.Segments = append(input.Segments, proofreadSegment{ID: segment.ID, Paragraphs: segment.Paragraphs})
	}
	rawInput, err := json.Marshal(input)
	if err != nil {
		return Response{}, err
	}
	openRouterConfig := ai.OpenRouterConfig{
		APIKey: config.APIKey, ModelID: config.ModelID, ProviderOrder: config.ProviderOrder,
		AllowFallbacks: config.AllowFallbacks, RequireParameters: config.RequireParameters,
		ReasoningEffort: config.ReasoningEffort, MaxTokens: 12000,
		ResponseFormat: proofreadResponseFormat(),
	}
	started := time.Now()
	result, err := s.generate(ctx, openRouterConfig, []ai.ChatMessage{
		{Role: "system", Content: proofreadInstructions()},
		{Role: "user", Content: string(rawInput)},
	})
	if err != nil {
		if errors.Is(err, ai.ErrOpenRouterTruncatedResponse) {
			err = fmt.Errorf("%w: %v", ErrOutputTooLong, err)
		}
		s.recordUsage(started, novelID, episodeIndex, config, result, err)
		return Response{}, err
	}
	var output proofreadOutput
	if err := json.Unmarshal([]byte(result.Answer), &output); err != nil {
		s.recordUsage(started, novelID, episodeIndex, config, result, err)
		return Response{}, fmt.Errorf("AI校正結果を読み取れませんでした: %w", err)
	}
	corrected, err := applyOutput(episode.ReaderDocument, segments, output)
	if err != nil {
		s.recordUsage(started, novelID, episodeIndex, config, result, err)
		return Response{}, err
	}
	generatedAt := time.Now().UTC().Format(time.RFC3339Nano)
	stored := storedResult{
		FormatVersion: 1, PromptVersion: promptVersion, NovelID: novelID, EpisodeIndex: episodeIndex,
		SourceETag: episode.ContentEtag, GeneratedAt: generatedAt, ModelID: config.ModelID, ReaderDocument: corrected,
	}
	if err := s.write(stored); err != nil {
		s.recordUsage(started, novelID, episodeIndex, config, result, err)
		return Response{}, err
	}
	s.recordUsage(started, novelID, episodeIndex, config, result, nil)
	displayDocument, err := s.applyReaderCorrections(novelID, corrected)
	if err != nil {
		return Response{}, err
	}
	modelID := config.ModelID
	return Response{
		Status: "ready", NovelID: novelID, EpisodeIndex: episodeIndex, SourceETag: episode.ContentEtag,
		GeneratedAt: &generatedAt, ModelID: &modelID, ReaderDocument: &displayDocument,
	}, nil
}

func (s *Service) Delete(novelID string, episodeIndex string) error {
	if !isValidEpisodeIndex(episodeIndex) {
		return ErrInvalidEpisodeIndex
	}
	err := os.Remove(s.path(novelID, episodeIndex))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func PruneNovelState(stateDir string, novelID string) (int, error) {
	sum := sha256.Sum256([]byte(novelID))
	dir := filepath.Join(stateDir, "reader_ai_proofreads", hex.EncodeToString(sum[:]))
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	deleted := 0
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
			deleted++
		}
	}
	return deleted, os.RemoveAll(dir)
}

func (s *Service) loadEpisode(ctx context.Context, novelID string, episodeIndex string) (*library.EpisodeResponse, error) {
	if s == nil || s.library == nil {
		return nil, nil
	}
	return s.library.GetEpisode(ctx, novelID, episodeIndex)
}

func (s *Service) applyReaderCorrections(novelID string, document library.ReaderDocument) (library.ReaderDocument, error) {
	if s == nil || s.settings == nil {
		return document, nil
	}
	settings, err := s.settings.GetNovelReaderSettings(novelID)
	if err != nil {
		return library.ReaderDocument{}, err
	}
	return library.ApplyReaderCorrections(document, library.ReaderCorrectionSettings{
		QuoteNormalization:                     settings.Correction.QuoteNormalization,
		HyphenDashNormalization:                settings.Correction.HyphenDashNormalization,
		ParenthesisNormalization:               settings.Correction.ParenthesisNormalization,
		HalfwidthAlnumPunctuationNormalization: settings.Correction.HalfwidthAlnumPunctuationNormalization,
	}), nil
}

func editableSegments(document library.ReaderDocument) []sourceSegment {
	segments := []sourceSegment{}
	for index := 0; index < len(document.Blocks); {
		block := document.Blocks[index]
		_, ok := simpleParagraphText(block)
		if !ok || isBlankParagraph(block) {
			index++
			continue
		}
		segment := sourceSegment{ID: len(segments), StartBlock: index, Section: block.Section}
		for index < len(document.Blocks) {
			nextText, nextOK := simpleParagraphText(document.Blocks[index])
			if !nextOK || isBlankParagraph(document.Blocks[index]) || document.Blocks[index].Section != segment.Section {
				break
			}
			segment.Paragraphs = append(segment.Paragraphs, nextText)
			index++
		}
		segment.EndBlock = index
		segments = append(segments, segment)
	}
	return segments
}

func isBlankParagraph(block library.ReaderBlock) bool {
	text, ok := simpleParagraphText(block)
	return ok && strings.TrimSpace(text) == ""
}

func simpleParagraphText(block library.ReaderBlock) (string, bool) {
	if block.Type != "paragraph" {
		return "", false
	}
	var builder strings.Builder
	for _, inline := range block.Inlines {
		switch inline.Type {
		case "text":
			builder.WriteString(inline.Text)
		case "lineBreak":
			builder.WriteString("\n")
		default:
			return "", false
		}
	}
	return builder.String(), true
}

func applyOutput(document library.ReaderDocument, source []sourceSegment, output proofreadOutput) (library.ReaderDocument, error) {
	if len(output.Segments) != len(source) {
		return library.ReaderDocument{}, errors.New("AI校正結果のsegment数が一致しません。")
	}
	byID := make(map[int]proofreadSegment, len(output.Segments))
	for _, segment := range output.Segments {
		if _, exists := byID[segment.ID]; exists {
			return library.ReaderDocument{}, errors.New("AI校正結果に重複したsegmentがあります。")
		}
		byID[segment.ID] = segment
	}
	blocks := append([]library.ReaderBlock{}, document.Blocks...)
	for index := len(source) - 1; index >= 0; index-- {
		segment := source[index]
		corrected, ok := byID[segment.ID]
		if !ok || len(corrected.Paragraphs) == 0 {
			return library.ReaderDocument{}, errors.New("AI校正結果に必要な本文segmentがありません。")
		}
		if nonWhitespaceText(segment.Paragraphs) != nonWhitespaceText(corrected.Paragraphs) {
			return library.ReaderDocument{}, errors.New("AI校正結果が空白・改行以外の原文を変更したため破棄しました。")
		}
		if !preservesSentenceEndingParagraphStyle(segment.Paragraphs, corrected.Paragraphs) {
			return library.ReaderDocument{}, errors.New("AI校正結果が原文の文末段落スタイルを変更したため破棄しました。")
		}
		if !preservesIdeographicIndentation(segment.Paragraphs, corrected.Paragraphs) {
			return library.ReaderDocument{}, errors.New("AI校正結果が原文の全角字下げを変更したため破棄しました。")
		}
		replacement := make([]library.ReaderBlock, 0, len(corrected.Paragraphs))
		for _, paragraph := range corrected.Paragraphs {
			if strings.TrimSpace(paragraph) == "" {
				continue
			}
			replacement = append(replacement, library.ReaderBlock{
				Type: "paragraph", Section: segment.Section,
				Inlines: paragraphInlines(paragraph),
			})
		}
		if len(replacement) == 0 {
			return library.ReaderDocument{}, errors.New("AI校正結果から本文が失われました。")
		}
		blocks = append(blocks[:segment.StartBlock], append(replacement, blocks[segment.EndBlock:]...)...)
	}
	return library.ReaderDocument{Version: document.Version, Blocks: blocks}, nil
}

func nonWhitespaceText(paragraphs []string) string {
	var builder strings.Builder
	for _, paragraph := range paragraphs {
		for _, r := range paragraph {
			if !unicode.IsSpace(r) {
				builder.WriteRune(r)
			}
		}
	}
	return builder.String()
}

func preservesSentenceEndingParagraphStyle(source []string, corrected []string) bool {
	sourceBoundaries := sentenceEndingParagraphBoundaries(source)
	correctedBoundaries := sentenceEndingParagraphBoundaries(corrected)
	if len(sourceBoundaries) != len(correctedBoundaries) {
		return false
	}
	for offset := range sourceBoundaries {
		if _, ok := correctedBoundaries[offset]; !ok {
			return false
		}
	}
	return true
}

func sentenceEndingParagraphBoundaries(paragraphs []string) map[int]struct{} {
	boundaries := map[int]struct{}{}
	offset := 0
	for index, paragraph := range paragraphs {
		offset += len([]rune(nonWhitespaceText([]string{paragraph})))
		if index < len(paragraphs)-1 && endsWithSentencePunctuation(paragraph) {
			boundaries[offset] = struct{}{}
		}
	}
	return boundaries
}

func preservesIdeographicIndentation(source []string, corrected []string) bool {
	sourceIndentation := ideographicIndentationByOffset(source)
	correctedIndentation := ideographicIndentationByOffset(corrected)
	for offset, indentation := range sourceIndentation {
		if correctedIndentation[offset] != indentation {
			return false
		}
	}
	return true
}

func ideographicIndentationByOffset(paragraphs []string) map[int]string {
	indentation := map[int]string{}
	offset := 0
	for _, paragraph := range paragraphs {
		var prefix strings.Builder
		for _, r := range paragraph {
			if !unicode.IsSpace(r) {
				break
			}
			prefix.WriteRune(r)
		}
		value := prefix.String()
		if strings.ContainsRune(value, '\u3000') {
			indentation[offset] = value
		}
		offset += len([]rune(nonWhitespaceText([]string{paragraph})))
	}
	return indentation
}

func paragraphInlines(paragraph string) []library.ReaderInline {
	paragraph = strings.ReplaceAll(paragraph, "\r\n", "\n")
	paragraph = strings.ReplaceAll(paragraph, "\r", "\n")
	parts := strings.Split(paragraph, "\n")
	inlines := make([]library.ReaderInline, 0, len(parts)*2-1)
	for index, part := range parts {
		if part != "" {
			inlines = append(inlines, library.ReaderInline{Type: "text", Text: part})
		}
		if index < len(parts)-1 {
			inlines = append(inlines, library.ReaderInline{Type: "lineBreak"})
		}
	}
	return inlines
}

func endsWithSentencePunctuation(text string) bool {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) == 0 {
		return false
	}
	return strings.ContainsRune("。！？!?…‥」』）】〕〉》”’", runes[len(runes)-1])
}

func proofreadInstructions() string {
	return `あなたは日本語小説の表示補助用校正器です。入力JSONのsegmentsを同じidで返してください。
不自然な改行、空白、段落分割だけを保守的に修正してください。語彙、文体、台詞、固有名詞、意味、句読点、表記は変更しないでください。
入力に含まれない空行を追加しないでください。入力は意図的な空行を境界に分割済みであり、segmentをまたいで段落を結合しないでください。
「。」「！」「？」や閉じ括弧の位置では、入力にあるparagraph境界を維持し、入力にない境界を追加しないでください。
日本語文中に孤立して混入した不自然な空白は、文脈から判断して削除してください。例:「森の 奥へ進んだ。」は「森の奥へ進んだ。」にします。
英単語間、ASCII表現、字下げ、意図的な間や強調など、意味または表現上の意図があり得る空白は維持してください。
意図的か判断できない箇所は原文を維持してください。文章を追加、削除、要約しないでください。
paragraphs配列の要素は表示上の段落です。JSON schemaに厳密に従ってください。`
}

func proofreadResponseFormat() any {
	return map[string]any{
		"type": "json_schema",
		"json_schema": map[string]any{
			"name":   "reader_proofread",
			"strict": true,
			"schema": map[string]any{
				"type": "object", "additionalProperties": false, "required": []string{"segments"},
				"properties": map[string]any{"segments": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object", "additionalProperties": false, "required": []string{"id", "paragraphs"},
						"properties": map[string]any{
							"id":         map[string]any{"type": "integer"},
							"paragraphs": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
						},
					},
				}},
			},
		},
	}
}

func (s *Service) path(novelID string, episodeIndex string) string {
	sum := sha256.Sum256([]byte(novelID))
	return filepath.Join(s.stateDir, "reader_ai_proofreads", hex.EncodeToString(sum[:]), episodeIndex+".json")
}

func isValidEpisodeIndex(value string) bool {
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

func (s *Service) read(novelID string, episodeIndex string) (storedResult, bool, error) {
	raw, err := os.ReadFile(s.path(novelID, episodeIndex))
	if errors.Is(err, os.ErrNotExist) {
		return storedResult{}, false, nil
	}
	if err != nil {
		return storedResult{}, false, err
	}
	var result storedResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return storedResult{}, false, err
	}
	if result.NovelID != novelID || result.EpisodeIndex != episodeIndex {
		return storedResult{}, false, errors.New("AI校正結果の識別子が一致しません。")
	}
	return result, true, nil
}

func (s *Service) write(result storedResult) error {
	path := s.path(result.NovelID, result.EpisodeIndex)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return fsatomic.WriteFile(path, raw, 0o600)
}

func (s *Service) recordUsage(started time.Time, novelID string, episodeIndex string, config *store.ResolvedAIGenerationConfig, result ai.ChatResult, runErr error) {
	if strings.TrimSpace(s.usageDBPath) == "" || config == nil {
		return
	}
	now := time.Now()
	status := "completed"
	var errorMessage *string
	if runErr != nil {
		status = "failed"
		message := runErr.Error()
		errorMessage = &message
	}
	modelID, profileID, profileLabel := config.ModelID, config.ProfileID, config.ProfileLabel
	feature, workflow, runID := "reader_proofread", "reader_proofread", fmt.Sprintf("reader-proofread-%d", started.UnixNano())
	if err := ai.SaveUsageRun(s.usageDBPath, ai.UsageRun{
		RunID: runID, Feature: feature, WorkflowName: workflow, Status: status,
		StartedAt: started.UTC().Format(time.RFC3339Nano), FinishedAt: now.UTC().Format(time.RFC3339Nano),
		ElapsedMs: int(now.Sub(started).Milliseconds()), NovelID: &novelID, CurrentEpisodeIndex: &episodeIndex,
		ModelID: &modelID, ProfileID: &profileID, ProfileLabel: &profileLabel, GenerationMode: "remote",
		AnswerChars: len([]rune(result.Answer)), RequestCount: 1, InputTokens: result.InputTokens,
		OutputTokens: result.OutputTokens, TotalTokens: result.TotalTokens, ErrorMessage: errorMessage,
		Requests: []ai.UsageRequest{{RequestIndex: 0, Kind: "reader_proofread", InputTokens: result.InputTokens, OutputTokens: result.OutputTokens, TotalTokens: result.TotalTokens}},
	}); err != nil {
		log.Printf("viewer-api-go: failed to save reader proofread usage: %v", err)
	}
}
