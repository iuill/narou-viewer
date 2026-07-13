package readerassistant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"narou-viewer/apps/viewer-api-go/internal/ai"
	"narou-viewer/apps/viewer-api-go/internal/application/readertextcache"
	"narou-viewer/apps/viewer-api-go/internal/characters"
	"narou-viewer/apps/viewer-api-go/internal/library"
	"narou-viewer/apps/viewer-api-go/internal/store"
	"narou-viewer/apps/viewer-api-go/internal/terms"
)

type Library interface {
	GetToc(ctx context.Context, novelID string) (*library.TocResponse, error)
	GetEpisode(ctx context.Context, novelID string, episodeIndex string) (*library.EpisodeResponse, error)
}

type SettingsProvider interface {
	ResolveActiveAIGenerationConfig() (*store.ResolvedAIGenerationConfig, error)
}

type Dependencies struct {
	Library     Library
	Settings    SettingsProvider
	StateDir    string
	UsageDBPath string
	TextCache   *readertextcache.Store
}

type Service struct {
	library     Library
	settings    SettingsProvider
	stateDir    string
	usageDBPath string
	textCache   *readertextcache.Store
}

type Request struct {
	NovelID             string
	CurrentEpisodeIndex string
	ReaderPosition      int
	Message             string
	History             []map[string]string
}

type StreamSink func(map[string]any) bool

func NewService(deps Dependencies) *Service {
	textCache := deps.TextCache
	if textCache == nil {
		textCache = readertextcache.New(deps.StateDir)
	}
	return &Service{library: normalizeLibrary(deps.Library), settings: normalizeSettings(deps.Settings), stateDir: deps.StateDir, usageDBPath: deps.UsageDBPath, textCache: textCache}
}

func normalizeLibrary(library Library) Library {
	if isNilPort(library) {
		return nil
	}
	return library
}

func normalizeSettings(settings SettingsProvider) SettingsProvider {
	if isNilPort(settings) {
		return nil
	}
	return settings
}

func isNilPort(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

func (s *Service) Respond(ctx context.Context, request Request, streamSink StreamSink) (map[string]any, error) {
	if s == nil {
		return nil, ErrUnavailable
	}
	return s.readerAssistantResponse(ctx, request.NovelID, request.CurrentEpisodeIndex, request.ReaderPosition, request.Message, request.History, streamSink)
}

var ErrUnavailable = errReaderAssistantUnavailable
var ErrContextTooLarge = errOpenRouterContextTooLarge

var errReaderAssistantUnavailable = errors.New("読書AIはLLM連携が未設定のため利用できません。AI機能の設定でOpenRouter APIキーとモデルを設定してください。")
var errOpenRouterContextTooLarge = errors.New("OpenRouter request exceeds model context length")

const (
	readerAssistantMaxAgentTurns           = 14
	readerAssistantMaxEpisodeRangeCount    = 20
	readerAssistantMaxSearchEpisodeCount   = 50
	readerAssistantMaxFullTextResults      = 50
	readerAssistantDefaultFullTextResults  = 20
	readerAssistantMaxFullTextPerEpisode   = 3
	readerAssistantFullTextCoverageBuckets = 8
	readerAssistantMaxFullTextQueryRunes   = 120
	readerAssistantMaxFullTextTerms        = 6
	readerAssistantDefaultPassageChars     = 1600
	readerAssistantMaxPassageChars         = 4000
	readerAssistantMaxPassageHitIDs        = 5
	readerAssistantDefaultMaxTokens        = 12000
)

type readerAssistantContext struct {
	NovelID                    string
	NovelTitle                 string
	CurrentEpisodeIndex        string
	CurrentEpisodeNumber       int
	CurrentEpisodeRef          map[string]any
	CurrentExcerpt             string
	CurrentPosition            int
	Message                    string
	History                    []map[string]string
	TocEpisodes                []library.TocEpisodeSummary
	RecentPreviousEpisodeCount int
	HitRegistry                *readerAssistantHitRegistry
}

type readerAssistantToolResult struct {
	Name   string
	Result map[string]any
}

type readerAssistantHitRegistry struct {
	SearchOrdinal int
	Hits          map[string]readerAssistantSearchHit
	Texts         map[string]readerAssistantEpisodeText
}

type readerAssistantSearchHit struct {
	NovelID         string
	MaxEpisodeIndex string
	EpisodeIndex    string
	EpisodeNumber   int
	Title           string
	Position        int
	MatchLength     int
	Query           string
	ContentEtag     string
}

type readerAssistantEpisodeText struct {
	Episode     *library.EpisodeResponse
	Text        string
	ContentEtag string
}

type Context = readerAssistantContext
type SearchHit = readerAssistantSearchHit
type HitRegistry = readerAssistantHitRegistry
type FullTextCandidate = readerAssistantFullTextCandidate
type UsageInput = readerAssistantUsageInput
type ToolResult = readerAssistantToolResult
type EpisodeText = readerAssistantEpisodeText

const (
	MaxEpisodeRangeCount   = readerAssistantMaxEpisodeRangeCount
	MaxFullTextQueryRunes  = readerAssistantMaxFullTextQueryRunes
	MaxFullTextTerms       = readerAssistantMaxFullTextTerms
	DefaultFullTextResults = readerAssistantDefaultFullTextResults
)

func (s *Service) readerAssistantResponse(ctx context.Context, novelID string, currentEpisodeIndex string, readerPosition int, message string, history []map[string]string, streamSink StreamSink) (map[string]any, error) {
	runID := "go-reader-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	startedAt := ai.NowISO()
	novelTitle := ""
	episodeTitle := currentEpisodeIndex
	var currentEpisodeRef map[string]any
	excerpt := ""
	var tocEpisodes []library.TocEpisodeSummary
	if s.library != nil {
		if toc, err := s.library.GetToc(ctx, novelID); err == nil && toc != nil {
			novelTitle = toc.Title
			tocEpisodes = toc.Episodes
		}
		if episode, err := s.library.GetEpisode(ctx, novelID, currentEpisodeIndex); err == nil && episode != nil {
			episodeTitle = episode.Title
			currentEpisodeRef = map[string]any{
				"episodeIndex": episode.EpisodeIndex,
				"title":        episode.Title,
				"chapter":      episode.Chapter,
				"subchapter":   episode.Subchapter,
			}
			excerpt = snippetAround(readerDocumentBodyText(episode.ReaderDocument), readerPosition, 0, 800)
		}
	}
	if currentEpisodeRef == nil {
		currentEpisodeRef = map[string]any{
			"episodeIndex": currentEpisodeIndex,
			"title":        episodeTitle,
			"chapter":      nil,
			"subchapter":   nil,
		}
	}
	config, err := s.resolveReaderAssistantConfig()
	if err != nil {
		return nil, err
	}
	if config == nil {
		return nil, errReaderAssistantUnavailable
	}
	modelID := stringPtrOrNil(config.ModelID)
	profileID := stringPtrOrNil(config.ProfileID)
	profileLabel := stringPtrOrNil(config.ProfileLabel)
	assistantContext := readerAssistantContext{
		NovelID:                    novelID,
		NovelTitle:                 novelTitle,
		CurrentEpisodeIndex:        currentEpisodeIndex,
		CurrentEpisodeNumber:       episodeNumberByIndex(tocEpisodes, currentEpisodeIndex),
		CurrentEpisodeRef:          currentEpisodeRef,
		CurrentExcerpt:             excerpt,
		CurrentPosition:            readerPosition,
		Message:                    strings.TrimSpace(message),
		History:                    history,
		TocEpisodes:                tocEpisodes,
		RecentPreviousEpisodeCount: readerAssistantRecentPreviousEpisodeCount(message),
		HitRegistry:                newReaderAssistantHitRegistry(),
	}
	result, toolRequests, toolResults, err := s.runReaderAssistantAgentLoop(ctx, assistantContext, ai.OpenRouterConfig{
		APIKey:            config.APIKey,
		ModelID:           config.ModelID,
		ProviderOrder:     config.ProviderOrder,
		AllowFallbacks:    config.AllowFallbacks,
		RequireParameters: config.RequireParameters,
		ReasoningEffort:   config.ReasoningEffort,
	}, streamSink)
	if err != nil {
		_ = s.recordReaderAssistantUsage(readerAssistantUsageInput{
			RunID:                      runID,
			Status:                     "failed",
			StartedAt:                  startedAt,
			NovelID:                    novelID,
			NovelTitle:                 novelTitle,
			CurrentEpisodeIndex:        currentEpisodeIndex,
			CurrentEpisodeNumber:       assistantContext.CurrentEpisodeNumber,
			CurrentPosition:            assistantContext.CurrentPosition,
			Answer:                     "",
			Message:                    message,
			History:                    history,
			GenerationMode:             "remote",
			ModelID:                    modelID,
			ProfileID:                  profileID,
			ProfileLabel:               profileLabel,
			ToolRequests:               toolRequests,
			ToolResults:                toolResults,
			RecentPreviousEpisodeCount: assistantContext.RecentPreviousEpisodeCount,
			ErrorMessage:               err.Error(),
		})
		return nil, err
	}
	answer := result.Answer
	inputTokens := result.InputTokens
	outputTokens := result.OutputTokens
	if inputTokens == 0 {
		inputTokens = estimateTokenCount(message)
	}
	if outputTokens == 0 {
		outputTokens = estimateTokenCount(answer)
	}
	if err := s.recordReaderAssistantUsage(readerAssistantUsageInput{
		RunID:                      runID,
		Status:                     "completed",
		StartedAt:                  startedAt,
		NovelID:                    novelID,
		NovelTitle:                 novelTitle,
		CurrentEpisodeIndex:        currentEpisodeIndex,
		CurrentEpisodeNumber:       assistantContext.CurrentEpisodeNumber,
		CurrentPosition:            assistantContext.CurrentPosition,
		Answer:                     answer,
		Message:                    message,
		History:                    history,
		GenerationMode:             "remote",
		ModelID:                    modelID,
		ProfileID:                  profileID,
		ProfileLabel:               profileLabel,
		InputTokens:                inputTokens,
		OutputTokens:               outputTokens,
		ToolRequests:               toolRequests,
		ToolResults:                toolResults,
		RecentPreviousEpisodeCount: assistantContext.RecentPreviousEpisodeCount,
	}); err != nil {
		runID = ""
	}
	var runIDValue any = runID
	if runID == "" {
		runIDValue = nil
	}
	return map[string]any{
		"answer":          answer,
		"novelId":         novelID,
		"maxEpisodeIndex": currentEpisodeIndex,
		"runId":           runIDValue,
		"toolRequests":    toolRequests,
		"toolResults":     toolResults,
		"generationMode":  "remote",
	}, nil
}

type readerAssistantUsageInput struct {
	RunID                      string
	Status                     string
	StartedAt                  string
	NovelID                    string
	NovelTitle                 string
	CurrentEpisodeIndex        string
	CurrentEpisodeNumber       int
	CurrentPosition            int
	Answer                     string
	Message                    string
	History                    []map[string]string
	GenerationMode             string
	ModelID                    *string
	ProfileID                  *string
	ProfileLabel               *string
	InputTokens                int
	OutputTokens               int
	ToolRequests               []map[string]any
	ToolResults                []map[string]any
	RecentPreviousEpisodeCount int
	ErrorMessage               string
}

func (s *Service) recordReaderAssistantUsage(input readerAssistantUsageInput) error {
	if s == nil {
		return nil
	}
	startedAt := input.StartedAt
	if startedAt == "" {
		startedAt = ai.NowISO()
	}
	finishedAt := ai.NowISO()
	novelTitlePtr := stringPtrOrNil(input.NovelTitle)
	currentEpisodeIndexPtr := &input.CurrentEpisodeIndex
	totalTokens := input.InputTokens + input.OutputTokens
	requests := readerAssistantUsageRequests(input.ToolRequests, input.InputTokens, input.OutputTokens)
	var errorMessage *string
	if strings.TrimSpace(input.ErrorMessage) != "" {
		errorMessage = stringPtrOrNil(input.ErrorMessage)
	}
	return ai.SaveUsageRun(s.usageDBPath, ai.UsageRun{
		RunID:               input.RunID,
		Feature:             "reader-assistant",
		WorkflowName:        "reader-ai-assistant",
		Status:              firstNonEmptyString(input.Status, "completed"),
		StartedAt:           startedAt,
		FinishedAt:          finishedAt,
		ElapsedMs:           elapsedMsBetweenISO(startedAt, finishedAt),
		NovelID:             &input.NovelID,
		NovelTitle:          novelTitlePtr,
		CurrentEpisodeIndex: currentEpisodeIndexPtr,
		ModelID:             input.ModelID,
		ProfileID:           input.ProfileID,
		ProfileLabel:        input.ProfileLabel,
		GenerationMode:      input.GenerationMode,
		AnswerChars:         len([]rune(input.Answer)),
		RequestCount:        len(requests),
		InputTokens:         input.InputTokens,
		OutputTokens:        input.OutputTokens,
		TotalTokens:         totalTokens,
		ToolCallCount:       len(input.ToolRequests),
		ToolResultCount:     len(input.ToolResults),
		ErrorMessage:        errorMessage,
		Requests:            requests,
		Snapshot:            readerAssistantUsageSnapshot(input, requests),
	})
}

func readerAssistantUsageSnapshot(input readerAssistantUsageInput, requests []ai.UsageRequest) map[string]any {
	return map[string]any{
		"readingContext": map[string]any{
			"novelId":                    input.NovelID,
			"novelTitle":                 input.NovelTitle,
			"currentEpisodeIndex":        input.CurrentEpisodeIndex,
			"currentEpisodeNumber":       input.CurrentEpisodeNumber,
			"currentPosition":            input.CurrentPosition,
			"maxEpisodeIndex":            input.CurrentEpisodeIndex,
			"recentPreviousEpisodeCount": input.RecentPreviousEpisodeCount,
			"recentPreviousRange":        readerAssistantUsageRecentPreviousRange(input.CurrentEpisodeNumber, input.RecentPreviousEpisodeCount),
		},
		"conversation": map[string]any{
			"historyCount": len(input.History),
			"messageChars": len([]rune(input.Message)),
			"answerChars":  len([]rune(input.Answer)),
		},
		"toolCalls":     readerAssistantToolUsageSnapshot(input.ToolRequests),
		"toolRequests":  sanitizeReaderAssistantSnapshotValue(input.ToolRequests),
		"toolResults":   sanitizeReaderAssistantSnapshotValue(input.ToolResults),
		"usageRequests": requests,
		"mode":          input.GenerationMode,
	}
}

func readerAssistantUsageRecentPreviousRange(currentNumber int, count int) map[string]any {
	if currentNumber <= 1 || count <= 0 {
		return nil
	}
	endNumber := currentNumber - 1
	startNumber := endNumber - count + 1
	if startNumber < 1 {
		startNumber = 1
	}
	return map[string]any{
		"startEpisodeNumber": startNumber,
		"endEpisodeNumber":   endNumber,
	}
}

func sanitizeReaderAssistantSnapshotValue(value any) any {
	return sanitizeReaderAssistantSnapshotValueDepth(value, 0)
}

func sanitizeReaderAssistantSnapshotValueDepth(value any, depth int) any {
	if depth > 8 {
		return nil
	}
	switch typed := value.(type) {
	case string:
		return truncateRunes(typed, 1000)
	case []map[string]any:
		limit := len(typed)
		if limit > 20 {
			limit = 20
		}
		items := make([]any, 0, limit)
		for index := 0; index < limit; index++ {
			items = append(items, sanitizeReaderAssistantSnapshotValueDepth(typed[index], depth+1))
		}
		return items
	case []any:
		limit := len(typed)
		if limit > 20 {
			limit = 20
		}
		items := make([]any, 0, limit)
		for index := 0; index < limit; index++ {
			items = append(items, sanitizeReaderAssistantSnapshotValueDepth(typed[index], depth+1))
		}
		return items
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, item := range typed {
			result[key] = sanitizeReaderAssistantSnapshotValueDepth(item, depth+1)
		}
		return result
	default:
		return typed
	}
}

func readerAssistantToolUsageSnapshot(toolRequests []map[string]any) []map[string]any {
	snapshot := make([]map[string]any, 0, len(toolRequests))
	for _, toolRequest := range toolRequests {
		name, _ := toolRequest["name"].(string)
		summary := readerAssistantToolResultMessage(name)
		snapshot = append(snapshot, map[string]any{
			"name":    name,
			"summary": summary,
		})
	}
	return snapshot
}

func (s *Service) resolveReaderAssistantConfig() (*store.ResolvedAIGenerationConfig, error) {
	if s == nil || s.settings == nil {
		return nil, nil
	}
	return s.settings.ResolveActiveAIGenerationConfig()
}

func (s *Service) runReaderAssistantAgentLoop(ctx context.Context, assistantContext readerAssistantContext, config ai.OpenRouterConfig, streamSink StreamSink) (ai.ChatResult, []map[string]any, []map[string]any, error) {
	temperature := 0.2
	config.Temperature = &temperature
	messages := []ai.ChatMessage{
		{Role: "system", Content: buildReaderAssistantInstructions(assistantContext)},
		{Role: "user", Content: buildReaderAssistantInput(assistantContext)},
	}
	tools := readerAssistantToolDefinitions()
	toolRequests := []map[string]any{}
	toolResults := []map[string]any{}
	tokenTotals := ai.ChatResult{}

	for turn := 0; turn < readerAssistantMaxAgentTurns; turn++ {
		promptTokens := estimateOpenRouterChatRequestTokens(messages, tools, nil)
		maxTokens, err := resolveOpenRouterMaxOutputTokens(ctx, config.APIKey, config.ModelID, config.ProviderOrder, readerAssistantDefaultMaxTokens, promptTokens)
		if err != nil {
			return ai.ChatResult{}, toolRequests, toolResults, err
		}
		config.MaxTokens = maxTokens
		result, err := ai.GenerateOpenRouterToolChat(ctx, config, messages, tools)
		if err != nil {
			return result, toolRequests, toolResults, err
		}
		tokenTotals.InputTokens += result.InputTokens
		tokenTotals.OutputTokens += result.OutputTokens
		tokenTotals.TotalTokens += result.TotalTokens
		tokenTotals.FinishReason = result.FinishReason
		if len(result.ToolCalls) == 0 {
			result.InputTokens = tokenTotals.InputTokens
			result.OutputTokens = tokenTotals.OutputTokens
			result.TotalTokens = tokenTotals.TotalTokens
			return result, toolRequests, toolResults, nil
		}

		messages = append(messages, ai.ChatMessage{
			Role:      "assistant",
			Content:   result.Answer,
			ToolCalls: result.ToolCalls,
		})
		for _, toolCall := range result.ToolCalls {
			if streamSink != nil {
				if !streamSink(map[string]any{"type": "tool_call", "toolName": toolCall.Function.Name, "message": readerAssistantToolCallMessage(toolCall.Function.Name)}) {
					return ai.ChatResult{}, toolRequests, toolResults, context.Canceled
				}
			}
			toolResult := s.executeReaderAssistantTool(ctx, assistantContext, toolCall.Function.Name, toolCall.Function.Arguments)
			toolRequests = append(toolRequests, map[string]any{
				"name":      toolCall.Function.Name,
				"arguments": decodeToolArguments(toolCall.Function.Arguments),
			})
			toolResults = append(toolResults, map[string]any{
				"name":   toolResult.Name,
				"result": toolResult.Result,
			})
			if streamSink != nil {
				if !streamSink(map[string]any{"type": "tool_result", "toolName": toolResult.Name, "message": readerAssistantToolResultMessage(toolResult.Name)}) {
					return ai.ChatResult{}, toolRequests, toolResults, context.Canceled
				}
			}
			messages = append(messages, ai.ChatMessage{
				Role:       "tool",
				ToolCallID: toolCall.ID,
				Name:       toolCall.Function.Name,
				Content:    mustJSON(toolResult.Result),
			})
		}
	}

	return ai.ChatResult{}, toolRequests, toolResults, errors.New("読書AIの応答生成に失敗しました: tool call の最大ターン数を超えました。")
}

func buildReaderAssistantInstructions(context readerAssistantContext) string {
	currentNumber := context.CurrentEpisodeNumber
	if currentNumber == 0 {
		currentNumber = 1
	}
	return strings.Join([]string{
		"あなたは長編小説を読むユーザーを助ける「読書AI」です。",
		"必ず日本語で、簡潔かつ根拠に忠実に答えてください。",
		"読める範囲は現在のネタバレ境界までです。境界より先の話、未読話のタイトル、未読話の内容を推測・言及してはいけません。",
		"必要な情報はツールで確認してください。手元にない本文・人物・用語情報を記憶や推測で補わないでください。",
		"現在地の確認だけなら get_current_episode を使ってください。",
		"前話を尋ねられた場合は get_previous_episode を使ってください。",
		"複数話の流れ、1〜5話のような範囲指定、ここまでの状況、読書再開用の確認では load_episode_range を使ってください。",
		"load_episode_range の startEpisodeNumber/endEpisodeNumber は1始まりです。一度に20話を超える範囲は指定せず、20話以下に分割してください。",
		"ユーザーが『直近N話』と尋ねた場合、現在話は含めず、前話から遡ってN話分として扱ってください。",
		"具体的な人物名・用語・地名・出来事の初出や過去の言及箇所など、長編全体から探す必要がある場合は search_full_text を使ってください。",
		"search_full_text は score 上位の topMatches と、既読範囲を横断する coverageMatches を返します。人物像や関係性の全体像では topMatches だけで結論を出さず、coverageMatches も確認してください。",
		"search_full_text の結果で重要そうな hitId を選び、根拠を深く読む必要がある場合は load_passages を使ってください。",
		"load_passages の hitIds は一度に最大5件です。6件以上読みたい場合は複数回に分けて呼び出してください。",
		"search_episodes は範囲が明確な小さな検索だけに使い、ユーザー発話全文を検索語にしないでください。",
		"人物関係や呼称を整理したいときは get_character_snapshot を使ってください。",
		"作品固有の用語、地名、組織、物品などの説明を確認したいときは get_term_snapshot を使ってください。",
		"人物について get_character_snapshot が未生成または情報不足なら、具体的な人物名を query にして search_full_text で既読範囲を探してください。",
		"用語について get_term_snapshot が未生成または情報不足なら、具体的な用語を query にして search_full_text で既読範囲を探してください。",
		"回答では Markdown の見出し記法（# や ##）と水平線（---）を使ってはいけません。節を分ける場合は普通の短い行や箇条書きにしてください。",
		"ツール結果で分からないことは分からないと明示し、断定しないでください。",
		"現在の作品: " + context.NovelTitle,
		"現在位置: 第" + strconv.Itoa(currentNumber) + "話まで",
		"ネタバレ境界 episodeIndex: " + context.CurrentEpisodeIndex,
	}, "\n")
}

func buildReaderAssistantInput(context readerAssistantContext) string {
	parts := []string{
		"ユーザーの質問: " + context.Message,
		"現在の episodeIndex: " + context.CurrentEpisodeIndex,
		"現在位置の文字オフセット: " + strconv.Itoa(context.CurrentPosition),
	}
	if note := readerAssistantRecentPreviousScopeNote(context); note != "" {
		parts = append(parts, note)
	}
	if len(context.History) > 0 {
		parts = append(parts, "直近の会話履歴:")
		for _, message := range context.History {
			role := "読書AI"
			if message["role"] == "user" {
				role = "ユーザー"
			}
			parts = append(parts, role+": "+message["text"])
		}
	}
	parts = append(parts, "上記に答えるため、必要ならツールを使ってから最終回答してください。")
	return strings.Join(parts, "\n")
}

func readerAssistantRecentPreviousScopeNote(context readerAssistantContext) string {
	if context.RecentPreviousEpisodeCount <= 0 {
		return ""
	}
	currentNumber := context.CurrentEpisodeNumber
	if currentNumber == 0 {
		currentNumber = episodeNumberByIndex(context.TocEpisodes, context.CurrentEpisodeIndex)
	}
	if currentNumber <= 1 {
		return "今回の質問は『直近N話』です。現在話は含めないため、要約対象にできる前話はありません。現在話の本文を要約対象に含めないでください。"
	}
	endNumber := currentNumber - 1
	startNumber := endNumber - context.RecentPreviousEpisodeCount + 1
	if startNumber < 1 {
		startNumber = 1
	}
	return "今回の質問は『直近N話』です。現在話は含めないため、要約対象範囲は第" + strconv.Itoa(startNumber) + "話〜第" + strconv.Itoa(endNumber) + "話です。load_episode_range を使う場合はこの範囲を指定し、現在話の第" + strconv.Itoa(currentNumber) + "話は要約対象に含めないでください。"
}

func readerAssistantToolDefinitions() []ai.ToolDefinition {
	return []ai.ToolDefinition{
		readerAssistantTool("get_current_episode", "現在開いている話の本文抜粋を取得する。", map[string]any{"type": "object", "properties": map[string]any{}, "required": []string{}}),
		readerAssistantTool("get_previous_episode", "現在話のひとつ前の話の本文抜粋を取得する。", map[string]any{"type": "object", "properties": map[string]any{}, "required": []string{}}),
		readerAssistantTool("load_episode_range", "既読範囲内の指定話数範囲をまとめて取得する。startEpisodeNumber/endEpisodeNumber は1始まり。未指定なら現在話までの最大20話。", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"startEpisodeNumber": map[string]any{"type": "number", "description": "読み込み開始話数。1始まり。"},
				"endEpisodeNumber":   map[string]any{"type": "number", "description": "読み込み終了話数。1始まり。現在話より先は指定しない。"},
				"output":             map[string]any{"type": "string", "enum": []string{"excerpt", "summary"}},
				"summaryPurpose":     map[string]any{"type": "string", "enum": []string{"plot", "character_relationships", "reader_resume", "custom"}},
				"summaryFocus":       map[string]any{"type": "string"},
			},
			"required": []string{},
		}),
		readerAssistantTool("search_episodes", "ネタバレ境界内の本文をキーワードで検索する。検索範囲は最大50話。", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query":              map[string]any{"type": "string"},
				"startEpisodeNumber": map[string]any{"type": "number"},
				"endEpisodeNumber":   map[string]any{"type": "number"},
			},
			"required": []string{"query"},
		}),
		readerAssistantTool("search_full_text", "現在のネタバレ境界内の全話または指定範囲を広域検索し、短いスニペットと hitId を返す。初出、過去の言及、長編全体の探索に使う。", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query":              map[string]any{"type": "string"},
				"maxResults":         map[string]any{"type": "number", "description": "score 上位候補 topMatches の最大数。matches には coverageMatches が追加される場合がある。"},
				"startEpisodeNumber": map[string]any{"type": "number"},
				"endEpisodeNumber":   map[string]any{"type": "number"},
			},
			"required": []string{"query"},
		}),
		readerAssistantTool("load_passages", "search_full_text が返した hitId の周辺本文だけを読み込む。同一run内の hitId のみ指定できる。", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"hitIds":       map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "minItems": 1, "maxItems": readerAssistantMaxPassageHitIDs},
				"contextChars": map[string]any{"type": "number"},
			},
			"required": []string{"hitIds"},
		}),
		readerAssistantTool("load_episode", "ネタバレ境界内の任意の話の本文抜粋を読み込む。", map[string]any{
			"type":       "object",
			"properties": map[string]any{"episodeIndex": map[string]any{"type": "string"}},
			"required":   []string{"episodeIndex"},
		}),
		readerAssistantTool("get_character_snapshot", "生成済みの人物情報スナップショットを取得する。", map[string]any{"type": "object", "properties": map[string]any{}, "required": []string{}}),
		readerAssistantTool("get_term_snapshot", "生成済みの用語情報スナップショットを取得する。", map[string]any{"type": "object", "properties": map[string]any{}, "required": []string{}}),
	}
}

func readerAssistantTool(name string, description string, parameters any) ai.ToolDefinition {
	return ai.ToolDefinition{Type: "function", Function: ai.ToolFunction{Name: name, Description: description, Parameters: parameters}}
}

func (s *Service) executeReaderAssistantTool(ctx context.Context, contextInfo readerAssistantContext, name string, rawArguments string) readerAssistantToolResult {
	args := decodeToolArguments(rawArguments)
	switch name {
	case "get_current_episode":
		result := map[string]any{
			"novelTitle":     contextInfo.NovelTitle,
			"currentEpisode": contextInfo.CurrentEpisodeRef,
			"excerpt":        truncateRunes(contextInfo.CurrentExcerpt, 700),
			"scopeNote":      "現在話より先の本文は参照していません。",
		}
		if contextInfo.RecentPreviousEpisodeCount > 0 {
			result["excerpt"] = ""
			result["scopeNote"] = "この質問の『直近N話』は現在話を含めないため、現在話の本文抜粋は返していません。"
		}
		return readerAssistantToolResult{Name: name, Result: result}
	case "get_previous_episode":
		return readerAssistantToolResult{Name: name, Result: s.previousEpisodeResult(ctx, contextInfo.NovelID, contextInfo.NovelTitle, contextInfo.CurrentEpisodeIndex, contextInfo.TocEpisodes)}
	case "load_episode_range":
		startNumber, endNumber, err := resolveReaderAssistantEpisodeRange(contextInfo, args)
		if err != nil {
			return readerAssistantToolRecovery(name, err)
		}
		if args["output"] == "summary" {
			summaryPurpose := readerAssistantSummaryPurpose(args["summaryPurpose"])
			summaryFocus := readerAssistantSummaryFocus(args["summaryFocus"])
			return readerAssistantToolResult{Name: name, Result: s.summarizeEpisodeRangeResult(ctx, contextInfo.NovelID, contextInfo.TocEpisodes, startNumber, endNumber, summaryPurpose, summaryFocus)}
		}
		return readerAssistantToolResult{Name: name, Result: s.loadEpisodeRangeResult(ctx, contextInfo.NovelID, contextInfo.NovelTitle, contextInfo.TocEpisodes, startNumber, endNumber)}
	case "search_episodes":
		query, _ := args["query"].(string)
		if strings.TrimSpace(query) == "" {
			return readerAssistantToolRecovery(name, errors.New("query is required."))
		}
		startNumber, endNumber, err := resolveReaderAssistantSearchRange(contextInfo, args)
		if err != nil {
			return readerAssistantToolRecovery(name, err)
		}
		return readerAssistantToolResult{Name: name, Result: s.searchEpisodesResultInRange(ctx, contextInfo.NovelID, strings.TrimSpace(query), contextInfo.TocEpisodes, startNumber, endNumber)}
	case "search_full_text":
		query, _ := args["query"].(string)
		normalizedQuery, err := readerAssistantFullTextQueryArg(query)
		if err != nil {
			return readerAssistantToolRecovery(name, err)
		}
		startNumber, endNumber, err := resolveReaderAssistantFullTextSearchRange(contextInfo, args)
		if err != nil {
			return readerAssistantToolRecovery(name, err)
		}
		maxResults, err := readerAssistantMaxResultsArg(args["maxResults"])
		if err != nil {
			return readerAssistantToolRecovery(name, err)
		}
		return readerAssistantToolResult{Name: name, Result: s.searchFullTextResult(ctx, contextInfo, normalizedQuery, startNumber, endNumber, maxResults)}
	case "load_passages":
		hitIDs, err := readerAssistantHitIDsArg(args["hitIds"])
		if err != nil {
			return readerAssistantToolRecovery(name, err)
		}
		contextChars, err := readerAssistantContextCharsArg(args["contextChars"])
		if err != nil {
			return readerAssistantToolRecovery(name, err)
		}
		return s.loadPassagesResult(ctx, contextInfo, hitIDs, contextChars)
	case "load_episode":
		episodeIndex, _ := args["episodeIndex"].(string)
		if strings.TrimSpace(episodeIndex) == "" {
			episodeIndex = contextInfo.CurrentEpisodeIndex
		}
		if !readerAssistantEpisodeVisible(contextInfo, episodeIndex) {
			return readerAssistantToolRecovery(name, errors.New("episodeIndex is outside the spoiler boundary."))
		}
		return readerAssistantToolResult{Name: name, Result: s.loadEpisodeResult(ctx, contextInfo.NovelID, episodeIndex)}
	case "get_character_snapshot":
		return readerAssistantToolResult{Name: name, Result: s.characterSnapshotResult(contextInfo.NovelID, contextInfo.CurrentEpisodeIndex, contextInfo.TocEpisodes)}
	case "get_term_snapshot":
		return readerAssistantToolResult{Name: name, Result: s.termSnapshotResult(contextInfo.NovelID, contextInfo.CurrentEpisodeIndex, contextInfo.TocEpisodes)}
	default:
		return readerAssistantToolRecovery(name, errors.New("unsupported reader assistant tool: "+name))
	}
}

func readerAssistantToolRecovery(toolName string, err error) readerAssistantToolResult {
	guidance := "範囲や話数をネタバレ境界内・上限内に調整して、必要なら別のツール呼び出しで再試行してください。"
	if strings.Contains(err.Error(), "episode range must be ") {
		guidance = "範囲を上限内に分割して再試行してください。例: 第1〜30話なら第1〜20話、第21〜30話に分けて読み込んでください。"
	}
	return readerAssistantToolResult{Name: "tool_recovery", Result: map[string]any{
		"toolName": toolName,
		"error":    err.Error(),
		"guidance": guidance,
	}}
}

func decodeToolArguments(raw string) map[string]any {
	args := map[string]any{}
	if strings.TrimSpace(raw) == "" {
		return args
	}
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return map[string]any{}
	}
	return args
}

func mustJSON(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func (s *Service) readerAssistantToolContext(ctx context.Context, novelID string, novelTitle string, currentEpisodeIndex string, currentEpisodeRef map[string]any, currentExcerpt string, message string, tocEpisodes []library.TocEpisodeSummary, streamSink StreamSink) ([]map[string]any, []map[string]any, bool) {
	requests := []map[string]any{}
	results := []map[string]any{}
	addTool := func(name string, arguments map[string]any, result map[string]any) bool {
		requests = append(requests, map[string]any{"name": name, "arguments": arguments})
		results = append(results, map[string]any{"name": name, "result": result})
		if streamSink != nil {
			if !streamSink(map[string]any{"type": "tool_call", "toolName": name, "message": readerAssistantToolCallMessage(name)}) {
				return false
			}
			if !streamSink(map[string]any{"type": "tool_result", "toolName": name, "message": readerAssistantToolResultMessage(name)}) {
				return false
			}
		}
		return true
	}

	if !addTool("get_current_episode", map[string]any{}, map[string]any{
		"novelTitle":     novelTitle,
		"currentEpisode": currentEpisodeRef,
		"excerpt":        truncateRunes(currentExcerpt, 700),
		"scopeNote":      "現在話より先の本文は参照していません。",
	}) {
		return requests, results, false
	}

	if previous := s.previousEpisodeResult(ctx, novelID, novelTitle, currentEpisodeIndex, tocEpisodes); previous != nil {
		if !addTool("get_previous_episode", map[string]any{}, previous) {
			return requests, results, false
		}
	}

	if loadResult := s.loadEpisodeResult(ctx, novelID, currentEpisodeIndex); loadResult != nil {
		if !addTool("load_episode", map[string]any{"episodeIndex": currentEpisodeIndex}, loadResult) {
			return requests, results, false
		}
	}

	startNumber, endNumber := readerAssistantRangeAround(tocEpisodes, currentEpisodeIndex)
	if rangeResult := s.loadEpisodeRangeResult(ctx, novelID, novelTitle, tocEpisodes, startNumber, endNumber); rangeResult != nil {
		if !addTool("load_episode_range", map[string]any{
			"startEpisodeNumber": startNumber,
			"endEpisodeNumber":   endNumber,
			"output":             "excerpt",
		}, rangeResult) {
			return requests, results, false
		}
	}

	query := readerAssistantSearchQuery(message)
	if query != "" {
		if !addTool("search_episodes", map[string]any{
			"query":              query,
			"isRegex":            false,
			"startEpisodeNumber": 1,
			"endEpisodeNumber":   episodeNumberByIndex(tocEpisodes, currentEpisodeIndex),
		}, s.searchEpisodesResult(ctx, novelID, query, tocEpisodes, currentEpisodeIndex)) {
			return requests, results, false
		}
	}

	if !addTool("get_character_snapshot", map[string]any{
		"upToEpisodeIndex": currentEpisodeIndex,
	}, s.characterSnapshotResult(novelID, currentEpisodeIndex, tocEpisodes)) {
		return requests, results, false
	}

	if !addTool("get_term_snapshot", map[string]any{
		"upToEpisodeIndex": currentEpisodeIndex,
	}, s.termSnapshotResult(novelID, currentEpisodeIndex, tocEpisodes)) {
		return requests, results, false
	}

	if summary := s.summarizeEpisodeRangeResult(ctx, novelID, tocEpisodes, startNumber, endNumber, "reader_resume", nil); summary != nil {
		if !addTool("summarize_episode_range", map[string]any{
			"startEpisodeNumber": startNumber,
			"endEpisodeNumber":   endNumber,
			"summaryPurpose":     "reader_resume",
			"summaryFocus":       nil,
		}, summary) {
			return requests, results, false
		}
	}

	return requests, results, true
}

func NormalizeHistory(value any) []map[string]string {
	items, ok := value.([]any)
	if !ok {
		return []map[string]string{}
	}
	result := []map[string]string{}
	for _, item := range items {
		record, ok := item.(map[string]any)
		if !ok {
			continue
		}
		role, _ := record["role"].(string)
		if role != "user" && role != "assistant" {
			continue
		}
		text, _ := record["text"].(string)
		text = truncateRunes(text, 600)
		if strings.TrimSpace(text) == "" {
			continue
		}
		result = append(result, map[string]string{"role": role, "text": text})
	}
	if len(result) > 8 {
		return result[len(result)-8:]
	}
	return result
}

func readerAssistantToolCallMessage(name string) string {
	switch name {
	case "get_current_episode":
		return "現在の話を確認しています。"
	case "get_previous_episode":
		return "前話を確認しています。"
	case "load_episode_range", "summarize_episode_range":
		return "既読範囲の本文を確認しています。"
	case "search_episodes", "search_full_text":
		return "作品内を検索しています。"
	case "load_passages":
		return "検索ヒット周辺を読み込んでいます。"
	case "load_episode":
		return "指定話を読み込んでいます。"
	case "get_character_snapshot":
		return "キャラクター情報を確認しています。"
	case "get_term_snapshot":
		return "用語情報を確認しています。"
	default:
		return "読書文脈を確認しています。"
	}
}

func readerAssistantToolResultMessage(name string) string {
	switch name {
	case "get_current_episode":
		return "現在の話を確認しました。"
	case "get_previous_episode":
		return "前話を確認しました。"
	case "load_episode_range", "summarize_episode_range":
		return "既読範囲の本文を確認しました。"
	case "search_episodes", "search_full_text":
		return "作品内を検索しました。"
	case "load_passages":
		return "検索ヒット周辺を読み込みました。"
	case "load_episode":
		return "指定話を読み込みました。"
	case "get_character_snapshot":
		return "キャラクター情報を確認しました。"
	case "get_term_snapshot":
		return "用語情報を確認しました。"
	default:
		return "読書文脈を確認しました。"
	}
}

func (s *Service) previousEpisodeResult(ctx context.Context, novelID string, novelTitle string, currentEpisodeIndex string, tocEpisodes []library.TocEpisodeSummary) map[string]any {
	currentNumber := episodeNumberByIndex(tocEpisodes, currentEpisodeIndex)
	if currentNumber <= 1 {
		return map[string]any{
			"status":     "not_available",
			"novelTitle": novelTitle,
			"episode":    nil,
			"excerpt":    "",
		}
	}
	previous := tocEpisodes[currentNumber-2]
	episode, _ := s.readerAssistantEpisode(ctx, novelID, previous.EpisodeIndex)
	if episode == nil {
		return map[string]any{
			"status":     "not_available",
			"novelTitle": novelTitle,
			"episode":    nil,
			"excerpt":    "",
		}
	}
	return map[string]any{
		"status":     "ready",
		"novelTitle": novelTitle,
		"episode":    episodeReference(episode),
		"excerpt":    truncateRunes(readerDocumentBodyText(episode.ReaderDocument), 700),
	}
}

func (s *Service) loadEpisodeResult(ctx context.Context, novelID string, episodeIndex string) map[string]any {
	episode, _ := s.readerAssistantEpisode(ctx, novelID, episodeIndex)
	if episode == nil {
		return nil
	}
	return map[string]any{
		"episode": episodeReference(episode),
		"excerpt": truncateRunes(readerDocumentBodyText(episode.ReaderDocument), 700),
	}
}

func (s *Service) loadEpisodeRangeResult(ctx context.Context, novelID string, novelTitle string, tocEpisodes []library.TocEpisodeSummary, startNumber int, endNumber int) map[string]any {
	if s == nil || s.library == nil || len(tocEpisodes) == 0 {
		return nil
	}
	if startNumber < 1 {
		startNumber = 1
	}
	if endNumber > len(tocEpisodes) {
		endNumber = len(tocEpisodes)
	}
	if endNumber < startNumber {
		return nil
	}
	if endNumber-startNumber+1 > 20 {
		endNumber = startNumber + 19
	}
	episodes := []map[string]any{}
	for number := startNumber; number <= endNumber; number++ {
		summary := tocEpisodes[number-1]
		episode, _ := s.readerAssistantEpisode(ctx, novelID, summary.EpisodeIndex)
		if episode == nil {
			continue
		}
		episodes = append(episodes, map[string]any{
			"episode": episodeReference(episode),
			"excerpt": truncateRunes(readerDocumentBodyText(episode.ReaderDocument), 650),
		})
	}
	return map[string]any{
		"novelTitle":         novelTitle,
		"startEpisodeNumber": startNumber,
		"endEpisodeNumber":   endNumber,
		"episodeCount":       len(episodes),
		"output":             "excerpt",
		"episodes":           episodes,
	}
}

func (s *Service) searchEpisodesResult(ctx context.Context, novelID string, query string, tocEpisodes []library.TocEpisodeSummary, maxEpisodeIndex string) map[string]any {
	endNumber := episodeNumberByIndex(tocEpisodes, maxEpisodeIndex)
	if endNumber == 0 {
		return map[string]any{
			"query":        query,
			"matches":      []map[string]any{},
			"error":        "current episode was not found in the table of contents.",
			"output":       "excerpt",
			"spoilerGuard": "closed",
		}
	}
	startNumber := 1
	if endNumber-startNumber+1 > 50 {
		startNumber = endNumber - 49
	}
	return s.searchEpisodesResultInRange(ctx, novelID, query, tocEpisodes, startNumber, endNumber)
}

func (s *Service) searchEpisodesResultInRange(ctx context.Context, novelID string, query string, tocEpisodes []library.TocEpisodeSummary, startNumber int, endNumber int) map[string]any {
	matches := []map[string]any{}
	lowerQuery := strings.ToLower(query)
	for number := startNumber; number <= endNumber && len(matches) < 5; number++ {
		episode, _ := s.readerAssistantEpisode(ctx, novelID, tocEpisodes[number-1].EpisodeIndex)
		if episode == nil {
			continue
		}
		text := readerDocumentBodyText(episode.ReaderDocument)
		lowerText := strings.ToLower(text)
		bytePosition := strings.Index(lowerText, lowerQuery)
		if bytePosition < 0 {
			continue
		}
		position := runeOffsetForByteIndex(lowerText, bytePosition)
		matchLength := len([]rune(lowerQuery))
		matches = append(matches, map[string]any{
			"episode":  episodeReference(episode),
			"position": position,
			"snippet":  snippetAround(text, position, matchLength, 140),
		})
	}
	return map[string]any{
		"query":              query,
		"isRegex":            false,
		"startEpisodeNumber": startNumber,
		"endEpisodeNumber":   endNumber,
		"episodeCount":       maxInt(0, endNumber-startNumber+1),
		"matches":            matches,
	}
}

func (s *Service) searchFullTextResult(ctx context.Context, contextInfo readerAssistantContext, query string, startNumber int, endNumber int, maxResults int) map[string]any {
	startedAt := time.Now()
	registry := contextInfo.HitRegistry
	if registry == nil {
		registry = newReaderAssistantHitRegistry()
		contextInfo.HitRegistry = registry
	}
	registry.SearchOrdinal++
	searchOrdinal := registry.SearchOrdinal
	candidates := []readerAssistantFullTextCandidate{}
	stats := readerAssistantFullTextSearchStats{}
	cachedTexts, cacheUsable := s.readerAssistantFullTextCacheEntries(ctx, contextInfo, startNumber, endNumber, &stats)
	lowerQuery := strings.ToLower(query)
	queryTerms := readerAssistantSearchTerms(lowerQuery)
	for number := startNumber; number <= endNumber; number++ {
		if ctx.Err() != nil {
			break
		}
		if number < 1 || number > len(contextInfo.TocEpisodes) {
			continue
		}
		summary := contextInfo.TocEpisodes[number-1]
		episodeText := s.readerAssistantEpisodeTextForSearch(ctx, contextInfo.NovelID, summary, registry, cachedTexts, cacheUsable, &stats)
		if episodeText == nil || strings.TrimSpace(episodeText.Text) == "" {
			continue
		}
		lowerText := strings.ToLower(episodeText.Text)
		titleText := strings.ToLower(strings.Join([]string{stringValue(episodeText.Episode.Chapter), stringValue(episodeText.Episode.Subchapter), episodeText.Episode.Title}, " "))
		positions := readerAssistantFindQueryPositions(lowerText, lowerQuery, queryTerms, readerAssistantMaxFullTextPerEpisode)
		titleScore := readerAssistantTitleScore(titleText, lowerQuery, queryTerms)
		for _, position := range positions {
			score := readerAssistantFullTextScore(lowerText, position, lowerQuery, queryTerms, titleScore)
			candidates = append(candidates, readerAssistantFullTextCandidate{
				Episode:     episodeText.Episode,
				Text:        episodeText.Text,
				ContentEtag: episodeText.ContentEtag,
				Number:      number,
				Position:    position,
				MatchLength: readerAssistantMatchLengthAt(lowerText, position, lowerQuery, queryTerms),
				Score:       score,
			})
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Score == candidates[j].Score {
			if candidates[i].Number == candidates[j].Number {
				return candidates[i].Position < candidates[j].Position
			}
			return candidates[i].Number < candidates[j].Number
		}
		return candidates[i].Score > candidates[j].Score
	})
	candidateCount := len(candidates)
	matchedEpisodeCount, firstMatchedEpisodeNumber, lastMatchedEpisodeNumber := readerAssistantFullTextDistribution(candidates)
	topMatchesTruncated := candidateCount > maxResults
	topCandidates := candidates
	if topMatchesTruncated {
		topCandidates = topCandidates[:maxResults]
	}
	coverageCandidates := readerAssistantCoverageFullTextCandidates(candidates, topCandidates, readerAssistantFullTextCoverageBuckets)
	selectedCandidates := append([]readerAssistantFullTextSelectedCandidate{}, readerAssistantSelectedFullTextCandidates(topCandidates, "top_score")...)
	selectedCandidates = append(selectedCandidates, readerAssistantSelectedFullTextCandidates(coverageCandidates, "coverage")...)
	matches := make([]map[string]any, 0, len(selectedCandidates))
	topMatches := make([]map[string]any, 0, len(topCandidates))
	coverageMatches := make([]map[string]any, 0, len(coverageCandidates))
	for index, selected := range selectedCandidates {
		candidate := selected.Candidate
		hitID := fmt.Sprintf("s%d_h%03d", searchOrdinal, index+1)
		registry.Hits[hitID] = readerAssistantSearchHit{
			NovelID:         contextInfo.NovelID,
			MaxEpisodeIndex: contextInfo.CurrentEpisodeIndex,
			EpisodeIndex:    candidate.Episode.EpisodeIndex,
			EpisodeNumber:   candidate.Number,
			Title:           candidate.Episode.Title,
			Position:        candidate.Position,
			MatchLength:     candidate.MatchLength,
			Query:           query,
			ContentEtag:     candidate.ContentEtag,
		}
		match := map[string]any{
			"hitId":           hitID,
			"episode":         episodeReference(candidate.Episode),
			"episodeIndex":    candidate.Episode.EpisodeIndex,
			"episodeNumber":   candidate.Number,
			"title":           candidate.Episode.Title,
			"position":        candidate.Position,
			"snippet":         snippetAround(candidate.Text, candidate.Position, candidate.MatchLength, 170),
			"score":           candidate.Score,
			"selectionReason": selected.SelectionReason,
		}
		matches = append(matches, match)
		if selected.SelectionReason == "coverage" {
			coverageMatches = append(coverageMatches, match)
		} else {
			topMatches = append(topMatches, match)
		}
	}
	omittedCandidateCount := maxInt(0, candidateCount-len(matches))
	truncated := omittedCandidateCount > 0
	metadata := map[string]any{
		"returnedCount":             len(matches),
		"candidateCount":            candidateCount,
		"matchedEpisodeCount":       matchedEpisodeCount,
		"firstMatchedEpisodeNumber": firstMatchedEpisodeNumber,
		"lastMatchedEpisodeNumber":  lastMatchedEpisodeNumber,
		"topMatchesTruncated":       topMatchesTruncated,
		"omittedCandidateCount":     omittedCandidateCount,
		"truncated":                 truncated,
		"elapsedMs":                 time.Since(startedAt).Milliseconds(),
		"cacheHitCount":             stats.CacheHitCount,
		"cacheMissCount":            stats.CacheMissCount,
		"cacheDisabledCount":        stats.CacheDisabledCount,
		"cacheErrorCount":           stats.CacheErrorCount,
		"memoryCacheHitCount":       stats.MemoryCacheHitCount,
		"loadedEpisodeCount":        stats.LoadedEpisodeCount,
		"failedEpisodeCount":        stats.FailedEpisodeCount,
	}
	return map[string]any{
		"query":                     query,
		"maxEpisodeIndex":           contextInfo.CurrentEpisodeIndex,
		"startEpisodeNumber":        startNumber,
		"endEpisodeNumber":          endNumber,
		"searchedEpisodeCount":      maxInt(0, endNumber-startNumber+1),
		"maxResults":                maxResults,
		"returnedCount":             len(matches),
		"candidateCount":            candidateCount,
		"matchedEpisodeCount":       matchedEpisodeCount,
		"firstMatchedEpisodeNumber": firstMatchedEpisodeNumber,
		"lastMatchedEpisodeNumber":  lastMatchedEpisodeNumber,
		"topMatchesTruncated":       topMatchesTruncated,
		"omittedCandidateCount":     omittedCandidateCount,
		"truncated":                 truncated,
		"matches":                   matches,
		"topMatches":                topMatches,
		"coverageMatches":           coverageMatches,
		"metadata":                  metadata,
	}
}

type readerAssistantFullTextSearchStats struct {
	CacheHitCount       int
	CacheMissCount      int
	CacheDisabledCount  int
	CacheErrorCount     int
	MemoryCacheHitCount int
	LoadedEpisodeCount  int
	FailedEpisodeCount  int
}

type readerAssistantFullTextCandidate struct {
	Episode     *library.EpisodeResponse
	Text        string
	ContentEtag string
	Number      int
	Position    int
	MatchLength int
	Score       float64
}

type readerAssistantFullTextSelectedCandidate struct {
	Candidate       readerAssistantFullTextCandidate
	SelectionReason string
}

func readerAssistantSelectedFullTextCandidates(candidates []readerAssistantFullTextCandidate, reason string) []readerAssistantFullTextSelectedCandidate {
	selected := make([]readerAssistantFullTextSelectedCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		selected = append(selected, readerAssistantFullTextSelectedCandidate{
			Candidate:       candidate,
			SelectionReason: reason,
		})
	}
	return selected
}

func readerAssistantFullTextDistribution(candidates []readerAssistantFullTextCandidate) (int, any, any) {
	first, last, ok := readerAssistantFullTextEpisodeBounds(candidates)
	if !ok {
		return 0, nil, nil
	}
	seenEpisodes := map[int]bool{}
	for _, candidate := range candidates {
		seenEpisodes[candidate.Number] = true
	}
	return len(seenEpisodes), first, last
}

func readerAssistantCoverageFullTextCandidates(candidates []readerAssistantFullTextCandidate, topCandidates []readerAssistantFullTextCandidate, bucketCount int) []readerAssistantFullTextCandidate {
	if len(candidates) == 0 || bucketCount <= 0 {
		return nil
	}
	first, last, ok := readerAssistantFullTextEpisodeBounds(candidates)
	if !ok {
		return nil
	}
	episodeSpan := last - first + 1
	if episodeSpan < bucketCount {
		bucketCount = episodeSpan
	}
	if bucketCount <= 0 {
		return nil
	}
	topKeys := map[string]bool{}
	for _, candidate := range topCandidates {
		topKeys[readerAssistantFullTextCandidateKey(candidate)] = true
	}
	coverage := []readerAssistantFullTextCandidate{}
	coverageKeys := map[string]bool{}
	for bucket := 0; bucket < bucketCount; bucket++ {
		bucketStart := first + (episodeSpan*bucket)/bucketCount
		bucketEnd := first + (episodeSpan*(bucket+1))/bucketCount - 1
		var selected readerAssistantFullTextCandidate
		found := false
		for _, candidate := range candidates {
			if candidate.Number < bucketStart || candidate.Number > bucketEnd {
				continue
			}
			key := readerAssistantFullTextCandidateKey(candidate)
			if topKeys[key] || coverageKeys[key] {
				continue
			}
			selected = candidate
			found = true
			break
		}
		if found {
			coverage = append(coverage, selected)
			coverageKeys[readerAssistantFullTextCandidateKey(selected)] = true
		}
	}
	return coverage
}

func readerAssistantFullTextEpisodeBounds(candidates []readerAssistantFullTextCandidate) (int, int, bool) {
	if len(candidates) == 0 {
		return 0, 0, false
	}
	first := candidates[0].Number
	last := candidates[0].Number
	for _, candidate := range candidates {
		if candidate.Number < first {
			first = candidate.Number
		}
		if candidate.Number > last {
			last = candidate.Number
		}
	}
	return first, last, true
}

func readerAssistantFullTextCandidateKey(candidate readerAssistantFullTextCandidate) string {
	episodeIndex := ""
	if candidate.Episode != nil {
		episodeIndex = candidate.Episode.EpisodeIndex
	}
	return fmt.Sprintf("%s:%d", episodeIndex, candidate.Position)
}

func (s *Service) loadPassagesResult(ctx context.Context, contextInfo readerAssistantContext, hitIDs []string, contextChars int) readerAssistantToolResult {
	if contextInfo.HitRegistry == nil {
		return readerAssistantToolRecovery("load_passages", errors.New("search_full_text must be called before load_passages."))
	}
	passages := []map[string]any{}
	for _, hitID := range hitIDs {
		hit, ok := contextInfo.HitRegistry.Hits[hitID]
		if !ok {
			return readerAssistantToolRecovery("load_passages", errors.New("hitId was not found in this reader-assistant run. Run search_full_text again and use the returned hitId."))
		}
		if hit.NovelID != contextInfo.NovelID || hit.MaxEpisodeIndex != contextInfo.CurrentEpisodeIndex || !readerAssistantEpisodeVisible(contextInfo, hit.EpisodeIndex) {
			return readerAssistantToolRecovery("load_passages", errors.New("hitId is outside the current reader context or spoiler boundary."))
		}
		episodeText := s.readerAssistantEpisodeText(ctx, contextInfo.NovelID, hit.EpisodeIndex, contextInfo.HitRegistry)
		if episodeText == nil || episodeText.Episode == nil {
			return readerAssistantToolRecovery("load_passages", errors.New("hit episode could not be loaded."))
		}
		if hit.ContentEtag != "" && episodeText.ContentEtag != "" && hit.ContentEtag != episodeText.ContentEtag {
			return readerAssistantToolRecovery("load_passages", errors.New("hit episode content changed after search. Run search_full_text again."))
		}
		start, end := readerAssistantPassageRange(episodeText.Text, hit.Position, hit.MatchLength, contextChars)
		passages = append(passages, map[string]any{
			"hitId":           hitID,
			"episode":         episodeReference(episodeText.Episode),
			"episodeIndex":    hit.EpisodeIndex,
			"episodeNumber":   hit.EpisodeNumber,
			"title":           hit.Title,
			"position":        hit.Position,
			"range":           map[string]any{"start": start, "end": end},
			"text":            substringRunes(episodeText.Text, start, end),
			"truncatedBefore": start > 0,
			"truncatedAfter":  end < len([]rune(episodeText.Text)),
		})
	}
	return readerAssistantToolResult{Name: "load_passages", Result: map[string]any{"passages": passages, "contextChars": contextChars}}
}

func (s *Service) characterSnapshotResult(novelID string, currentEpisodeIndex string, tocEpisodes []library.TocEpisodeSummary) map[string]any {
	episodeIndexes := episodeIndexesFromToc(tocEpisodes)
	summary, ok, err := characters.LoadSummaryForEpisodes(s.stateDir, novelID, currentEpisodeIndex, episodeIndexes)
	if err != nil || !ok || summary.Status == "not_generated" {
		return map[string]any{
			"status":                    "not_generated",
			"processedUpToEpisodeIndex": nil,
			"characterCount":            0,
			"characters":                []any{},
			"fallbackTool":              "search_full_text",
			"fallbackHint":              "キャラクター一覧が未生成です。人物名・用語・地名などの具体語を確認する場合は、search_full_text で既読範囲を検索できます。",
		}
	}
	items := []map[string]any{}
	for _, character := range summary.Characters {
		if len(items) >= 20 {
			break
		}
		items = append(items, map[string]any{
			"canonicalName": character.CanonicalName,
			"aliases":       character.Aliases,
			"summary":       character.Summary,
		})
	}
	result := map[string]any{
		"status":                    summary.Status,
		"processedUpToEpisodeIndex": summary.ProcessedUpToEpisodeIndex,
		"characterCount":            len(summary.Characters),
		"characters":                items,
	}
	if summary.Status == "partial" {
		result["fallbackTool"] = "search_full_text"
		result["fallbackHint"] = "人物一覧は生成済みの話までの内容です。現在のネタバレ境界までを確認する場合は、search_full_text で既読範囲を検索できます。"
	}
	return result
}

func (s *Service) termSnapshotResult(novelID string, currentEpisodeIndex string, tocEpisodes []library.TocEpisodeSummary) map[string]any {
	episodeIndexes := episodeIndexesFromToc(tocEpisodes)
	characterSummary, _, err := characters.LoadSummaryForEpisodes(s.stateDir, novelID, currentEpisodeIndex, episodeIndexes)
	if err != nil {
		return termSnapshotNotGenerated("用語一覧を読み込めませんでした。具体的な用語を search_full_text で検索してください。")
	}
	committedFrontier := ""
	if characterSummary.ProcessedUpToEpisodeIndex != nil {
		committedFrontier = *characterSummary.ProcessedUpToEpisodeIndex
	}
	response, err := terms.BuildResponse(s.stateDir, novelID, currentEpisodeIndex, committedFrontier)
	if err != nil {
		return termSnapshotNotGenerated("用語一覧を読み込めませんでした。具体的な用語を search_full_text で検索してください。")
	}
	items := make([]map[string]any, 0, min(len(response.Terms), 30))
	for _, term := range response.Terms {
		if len(items) >= 30 {
			break
		}
		items = append(items, map[string]any{
			"term":        term.Term,
			"reading":     term.Reading,
			"category":    term.Category,
			"description": term.Description,
		})
	}
	result := map[string]any{
		"status":                    response.Status,
		"processedUpToEpisodeIndex": response.ProcessedUpToEpisodeIndex,
		"termCount":                 len(response.Terms),
		"terms":                     items,
	}
	if len(response.Terms) > len(items) {
		result["truncated"] = true
		result["fallbackTool"] = "search_full_text"
		result["fallbackHint"] = "用語一覧は件数が多いため先頭30件のみ返しています。含まれない用語は search_full_text で既読範囲から検索できます。"
	}
	if response.Status != "ready" {
		result["fallbackTool"] = "search_full_text"
		if response.Status == "partial" {
			result["fallbackHint"] = "用語一覧は生成済みの話までの内容です。現在のネタバレ境界までを確認する場合は、search_full_text で既読範囲を検索できます。"
		} else {
			result["fallbackHint"] = "ネタバレ境界までの用語一覧はまだ生成されていません。具体的な用語を search_full_text で既読範囲から検索できます。"
		}
	}
	return result
}

func termSnapshotNotGenerated(hint string) map[string]any {
	return map[string]any{
		"status":                    "not_generated",
		"processedUpToEpisodeIndex": nil,
		"termCount":                 0,
		"terms":                     []any{},
		"fallbackTool":              "search_full_text",
		"fallbackHint":              hint,
	}
}

func (s *Service) summarizeEpisodeRangeResult(ctx context.Context, novelID string, tocEpisodes []library.TocEpisodeSummary, startNumber int, endNumber int, summaryPurpose string, summaryFocus any) map[string]any {
	if len(tocEpisodes) == 0 || endNumber < startNumber {
		return nil
	}
	summaries := []string{}
	for number := startNumber; number <= endNumber && len(summaries) < 20; number++ {
		episode, _ := s.readerAssistantEpisode(ctx, novelID, tocEpisodes[number-1].EpisodeIndex)
		if episode == nil {
			continue
		}
		text := truncateRunes(readerDocumentBodyText(episode.ReaderDocument), 180)
		if text != "" {
			summaries = append(summaries, episode.Title+": "+text)
		}
	}
	if summaryPurpose == "" {
		summaryPurpose = "custom"
	}
	return map[string]any{
		"status":              "ready",
		"summaryPurpose":      summaryPurpose,
		"summaryFocus":        summaryFocus,
		"startEpisodeNumber":  startNumber,
		"endEpisodeNumber":    endNumber,
		"sourceEpisodeCount":  len(summaries),
		"summary":             strings.Join(summaries, "\n"),
		"generatedBy":         "local",
		"spoilerBoundaryNote": "現在話より先の本文は参照していません。",
	}
}

func readerAssistantSummaryPurpose(value any) string {
	text, _ := value.(string)
	switch strings.TrimSpace(text) {
	case "plot", "character_relationships", "reader_resume", "custom":
		return strings.TrimSpace(text)
	default:
		return "custom"
	}
}

func readerAssistantSummaryFocus(value any) any {
	text, _ := value.(string)
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	return truncateRunes(text, 400)
}

func (s *Service) readerAssistantEpisode(ctx context.Context, novelID string, episodeIndex string) (*library.EpisodeResponse, error) {
	if s == nil || s.library == nil {
		return nil, nil
	}
	return s.library.GetEpisode(ctx, novelID, episodeIndex)
}

func episodeReference(episode *library.EpisodeResponse) map[string]any {
	if episode == nil {
		return nil
	}
	return map[string]any{
		"episodeIndex": episode.EpisodeIndex,
		"title":        episode.Title,
		"chapter":      episode.Chapter,
		"subchapter":   episode.Subchapter,
	}
}

func readerAssistantRangeAround(tocEpisodes []library.TocEpisodeSummary, currentEpisodeIndex string) (int, int) {
	currentNumber := episodeNumberByIndex(tocEpisodes, currentEpisodeIndex)
	if currentNumber == 0 {
		return 1, 1
	}
	start := currentNumber - 2
	if start < 1 {
		start = 1
	}
	return start, currentNumber
}

func newReaderAssistantHitRegistry() *readerAssistantHitRegistry {
	return &readerAssistantHitRegistry{
		Hits:  map[string]readerAssistantSearchHit{},
		Texts: map[string]readerAssistantEpisodeText{},
	}
}

func (s *Service) readerAssistantEpisodeText(ctx context.Context, novelID string, episodeIndex string, registry *readerAssistantHitRegistry) *readerAssistantEpisodeText {
	if registry != nil {
		if cached, ok := registry.Texts[episodeIndex]; ok {
			return &cached
		}
	}
	return s.readerAssistantFreshEpisodeText(ctx, novelID, episodeIndex, registry)
}

func (s *Service) readerAssistantFullTextCacheEntries(ctx context.Context, contextInfo readerAssistantContext, startNumber int, endNumber int, stats *readerAssistantFullTextSearchStats) (map[readertextcache.Key]readertextcache.Entry, bool) {
	if s == nil || s.textCache == nil {
		return nil, false
	}
	keys := make([]readertextcache.LookupKey, 0, maxInt(0, endNumber-startNumber+1))
	for number := startNumber; number <= endNumber; number++ {
		if number < 1 || number > len(contextInfo.TocEpisodes) {
			continue
		}
		summary := contextInfo.TocEpisodes[number-1]
		if strings.TrimSpace(summary.ContentEtag) == "" {
			continue
		}
		keys = append(keys, readertextcache.LookupKey{EpisodeIndex: summary.EpisodeIndex, ContentEtag: summary.ContentEtag})
	}
	entries, err := s.textCache.GetMany(ctx, contextInfo.NovelID, keys)
	if err != nil {
		log.Printf("viewer-api-go: reader assistant text cache lookup failed: novelID=%s err=%v", contextInfo.NovelID, err)
		if stats != nil {
			stats.CacheErrorCount++
		}
		return nil, false
	}
	return entries, true
}

func (s *Service) readerAssistantEpisodeTextForSearch(ctx context.Context, novelID string, summary library.TocEpisodeSummary, registry *readerAssistantHitRegistry, cachedTexts map[readertextcache.Key]readertextcache.Entry, cacheUsable bool, stats *readerAssistantFullTextSearchStats) *readerAssistantEpisodeText {
	if registry != nil {
		if cached, ok := registry.Texts[summary.EpisodeIndex]; ok && readerAssistantTextMatchesETag(cached, summary.ContentEtag) {
			if stats != nil {
				stats.MemoryCacheHitCount++
			}
			return &cached
		}
	}
	if cacheUsable && summary.ContentEtag != "" {
		entry, ok := cachedTexts[readertextcache.Key{EpisodeIndex: summary.EpisodeIndex, ContentEtag: summary.ContentEtag}]
		if ok {
			if stats != nil {
				stats.CacheHitCount++
				stats.LoadedEpisodeCount++
			}
			result := readerAssistantEpisodeText{
				Episode:     readerAssistantEpisodeFromTocSummary(novelID, summary, entry),
				Text:        entry.Text,
				ContentEtag: summary.ContentEtag,
			}
			if registry != nil {
				registry.Texts[summary.EpisodeIndex] = result
			}
			return &result
		}
		if stats != nil {
			stats.CacheMissCount++
		}
	} else if stats != nil {
		stats.CacheDisabledCount++
	}
	result := s.readerAssistantFreshEpisodeTextWithCache(ctx, novelID, summary.EpisodeIndex, registry, summary.ContentEtag != "")
	if result == nil {
		if stats != nil {
			stats.FailedEpisodeCount++
		}
		return nil
	}
	if stats != nil {
		stats.LoadedEpisodeCount++
	}
	return result
}

func (s *Service) readerAssistantFreshEpisodeText(ctx context.Context, novelID string, episodeIndex string, registry *readerAssistantHitRegistry) *readerAssistantEpisodeText {
	return s.readerAssistantFreshEpisodeTextWithCache(ctx, novelID, episodeIndex, registry, true)
}

func (s *Service) readerAssistantFreshEpisodeTextWithCache(ctx context.Context, novelID string, episodeIndex string, registry *readerAssistantHitRegistry, saveCache bool) *readerAssistantEpisodeText {
	episode, _ := s.readerAssistantEpisode(ctx, novelID, episodeIndex)
	if episode == nil {
		return nil
	}
	text := readerDocumentBodyText(episode.ReaderDocument)
	result := readerAssistantEpisodeText{
		Episode:     episode,
		Text:        text,
		ContentEtag: episode.ContentEtag,
	}
	if saveCache && s != nil && s.textCache != nil {
		_ = s.textCache.Save(ctx, novelID, episodeIndex, episode.ContentEtag, text)
	}
	if registry != nil {
		registry.Texts[episodeIndex] = result
	}
	return &result
}

func readerAssistantTextMatchesETag(text readerAssistantEpisodeText, contentEtag string) bool {
	return contentEtag == "" || text.ContentEtag == contentEtag
}

func readerAssistantEpisodeFromTocSummary(novelID string, summary library.TocEpisodeSummary, entry readertextcache.Entry) *library.EpisodeResponse {
	plainTextLength := entry.PlainTextLength
	if plainTextLength == 0 && entry.Text != "" {
		plainTextLength = len([]rune(entry.Text))
	}
	return &library.EpisodeResponse{
		NovelID:         novelID,
		EpisodeIndex:    summary.EpisodeIndex,
		Title:           summary.Title,
		Chapter:         summary.Chapter,
		Subchapter:      summary.Subchapter,
		SourceURL:       summary.SourceURL,
		PlainTextLength: plainTextLength,
		UpdatedAt:       summary.UpdatedAt,
		ContentEtag:     summary.ContentEtag,
	}
}

func resolveReaderAssistantFullTextSearchRange(contextInfo readerAssistantContext, args map[string]any) (int, int, error) {
	maxNumber := contextInfo.CurrentEpisodeNumber
	if maxNumber == 0 {
		maxNumber = episodeNumberByIndex(contextInfo.TocEpisodes, contextInfo.CurrentEpisodeIndex)
	}
	if maxNumber == 0 {
		return 1, 1, errors.New("current episode was not found in the table of contents.")
	}
	startNumber, err := readerAssistantEpisodeNumberArg(args["startEpisodeNumber"], 1, maxNumber)
	if err != nil {
		return 0, 0, err
	}
	endNumber, err := readerAssistantEpisodeNumberArg(args["endEpisodeNumber"], maxNumber, maxNumber)
	if err != nil {
		return 0, 0, err
	}
	if startNumber > endNumber {
		return 0, 0, errors.New("startEpisodeNumber must be less than or equal to endEpisodeNumber.")
	}
	return startNumber, endNumber, nil
}

func readerAssistantMaxResultsArg(value any) (int, error) {
	if value == nil {
		return readerAssistantDefaultFullTextResults, nil
	}
	number, err := readerAssistantPositiveIntArg(value, "maxResults")
	if err != nil {
		return 0, err
	}
	if number > readerAssistantMaxFullTextResults {
		return 0, errors.New("maxResults must be 50 or fewer.")
	}
	return number, nil
}

func readerAssistantContextCharsArg(value any) (int, error) {
	if value == nil {
		return readerAssistantDefaultPassageChars, nil
	}
	number, err := readerAssistantPositiveIntArg(value, "contextChars")
	if err != nil {
		return 0, err
	}
	if number > readerAssistantMaxPassageChars {
		return 0, errors.New("contextChars must be 4000 or fewer.")
	}
	return number, nil
}

func readerAssistantFullTextQueryArg(value string) (string, error) {
	query := strings.TrimSpace(value)
	if query == "" {
		return "", errors.New("query is required.")
	}
	if len([]rune(query)) > readerAssistantMaxFullTextQueryRunes {
		return "", fmt.Errorf("query must be %d characters or fewer. Use a concrete name or term.", readerAssistantMaxFullTextQueryRunes)
	}
	if readerAssistantSearchTermCount(query) > readerAssistantMaxFullTextTerms {
		return "", fmt.Errorf("query must contain %d terms or fewer. Use a concrete name or term.", readerAssistantMaxFullTextTerms)
	}
	return query, nil
}

func readerAssistantPositiveIntArg(value any, name string) (int, error) {
	var number int
	switch typed := value.(type) {
	case float64:
		if typed != float64(int(typed)) {
			return 0, errors.New(name + " must be an integer.")
		}
		number = int(typed)
	case int:
		number = typed
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err != nil {
			return 0, errors.New(name + " must be an integer.")
		}
		number = parsed
	default:
		return 0, errors.New(name + " must be an integer.")
	}
	if number < 1 {
		return 0, errors.New(name + " must be positive.")
	}
	return number, nil
}

func readerAssistantHitIDsArg(value any) ([]string, error) {
	items, ok := value.([]any)
	if !ok || len(items) == 0 {
		return nil, errors.New("hitIds must be a non-empty array.")
	}
	if len(items) > readerAssistantMaxPassageHitIDs {
		return nil, errors.New("hitIds must contain 5 or fewer items.")
	}
	result := []string{}
	seen := map[string]bool{}
	for _, item := range items {
		hitID, _ := item.(string)
		hitID = strings.TrimSpace(hitID)
		if hitID == "" {
			return nil, errors.New("hitIds must contain only non-empty strings.")
		}
		if seen[hitID] {
			continue
		}
		seen[hitID] = true
		result = append(result, hitID)
	}
	if len(result) == 0 {
		return nil, errors.New("hitIds must be a non-empty array.")
	}
	return result, nil
}

func readerAssistantSearchTerms(lowerQuery string) []string {
	fields := strings.Fields(lowerQuery)
	terms := []string{}
	seen := map[string]bool{}
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" || seen[field] {
			continue
		}
		seen[field] = true
		terms = append(terms, field)
		if len(terms) >= readerAssistantMaxFullTextTerms {
			break
		}
	}
	if len(terms) == 0 && strings.TrimSpace(lowerQuery) != "" {
		terms = append(terms, strings.TrimSpace(lowerQuery))
	}
	return terms
}

func readerAssistantSearchTermCount(query string) int {
	fields := strings.Fields(query)
	seen := map[string]bool{}
	count := 0
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" || seen[field] {
			continue
		}
		seen[field] = true
		count++
	}
	if count == 0 && strings.TrimSpace(query) != "" {
		return 1
	}
	return count
}

func readerAssistantFindQueryPositions(lowerText string, lowerQuery string, terms []string, maxPerEpisode int) []int {
	positions := []int{}
	totalLimit := maxPerEpisode
	if len(terms) > 1 {
		totalLimit = maxPerEpisode * len(terms)
	}
	addPositions := func(needle string, limit int) {
		if needle == "" {
			return
		}
		searchFrom := 0
		added := 0
		for added < limit && len(positions) < totalLimit && searchFrom < len(lowerText) {
			bytePosition := strings.Index(lowerText[searchFrom:], needle)
			if bytePosition < 0 {
				return
			}
			position := runeOffsetForByteIndex(lowerText, searchFrom+bytePosition)
			if !intSliceContains(positions, position) {
				positions = append(positions, position)
				added++
			}
			searchFrom += bytePosition + len(needle)
		}
	}
	addPositions(lowerQuery, maxPerEpisode)
	if len(positions) < totalLimit {
		for _, term := range terms {
			addPositions(term, maxPerEpisode)
			if len(positions) >= totalLimit {
				break
			}
		}
	}
	sort.Ints(positions)
	return positions
}

func readerAssistantTitleScore(titleText string, lowerQuery string, terms []string) float64 {
	score := 0.0
	if strings.Contains(titleText, lowerQuery) {
		score += 6
	}
	for _, term := range terms {
		if strings.Contains(titleText, term) {
			score += 1.5
		}
	}
	return score
}

func readerAssistantFullTextScore(lowerText string, position int, lowerQuery string, terms []string, titleScore float64) float64 {
	score := 1.0 + titleScore
	if lowerQuery != "" {
		bytePosition := byteIndexForRuneOffset(lowerText, position)
		if bytePosition >= 0 && strings.HasPrefix(lowerText[bytePosition:], lowerQuery) {
			score += 10
		}
	}
	window := strings.ToLower(substringRunes(lowerText, maxInt(0, position-240), position+240))
	matchedTerms := 0
	for _, term := range terms {
		if strings.Contains(window, term) {
			score += 2
			matchedTerms++
		}
	}
	if len(terms) > 1 && matchedTerms == len(terms) {
		score += 4
	}
	if position < 800 {
		score += 0.5
	}
	return score
}

func readerAssistantMatchLengthAt(lowerText string, position int, lowerQuery string, terms []string) int {
	bytePosition := byteIndexForRuneOffset(lowerText, position)
	if bytePosition >= 0 && lowerQuery != "" && strings.HasPrefix(lowerText[bytePosition:], lowerQuery) {
		return len([]rune(lowerQuery))
	}
	for _, term := range terms {
		if bytePosition >= 0 && strings.HasPrefix(lowerText[bytePosition:], term) {
			return len([]rune(term))
		}
	}
	return 0
}

func readerAssistantPassageRange(text string, position int, matchLength int, contextChars int) (int, int) {
	runes := len([]rune(text))
	if runes == 0 {
		return 0, 0
	}
	if position < 0 {
		position = 0
	}
	if position > runes {
		position = runes
	}
	before := contextChars / 2
	after := contextChars - before
	endTarget := position + maxInt(matchLength, 1) + after
	start := position - before
	if start < 0 {
		endTarget -= start
		start = 0
	}
	if endTarget > runes {
		start -= endTarget - runes
		endTarget = runes
		if start < 0 {
			start = 0
		}
	}
	return start, endTarget
}

func substringRunes(text string, start int, end int) string {
	runes := []rune(text)
	if start < 0 {
		start = 0
	}
	if end < start {
		end = start
	}
	if start > len(runes) {
		start = len(runes)
	}
	if end > len(runes) {
		end = len(runes)
	}
	return string(runes[start:end])
}

func byteIndexForRuneOffset(text string, offset int) int {
	if offset < 0 {
		return -1
	}
	if offset == 0 {
		return 0
	}
	count := 0
	for index := range text {
		if count == offset {
			return index
		}
		count++
	}
	if count == offset {
		return len(text)
	}
	return -1
}

func intSliceContains(values []int, target int) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func resolveReaderAssistantEpisodeRange(contextInfo readerAssistantContext, args map[string]any) (int, int, error) {
	currentNumber := contextInfo.CurrentEpisodeNumber
	if currentNumber == 0 {
		currentNumber = episodeNumberByIndex(contextInfo.TocEpisodes, contextInfo.CurrentEpisodeIndex)
	}
	if currentNumber == 0 {
		return 1, 1, errors.New("current episode was not found in the table of contents.")
	}
	hasStart := args["startEpisodeNumber"] != nil
	hasEnd := args["endEpisodeNumber"] != nil
	if contextInfo.RecentPreviousEpisodeCount > 0 && currentNumber > 1 && !hasStart && !hasEnd {
		endNumber := currentNumber - 1
		startNumber := endNumber - contextInfo.RecentPreviousEpisodeCount + 1
		if startNumber < 1 {
			startNumber = 1
		}
		if endNumber-startNumber+1 > readerAssistantMaxEpisodeRangeCount {
			return 0, 0, fmtEpisodeRangeError(readerAssistantMaxEpisodeRangeCount)
		}
		return startNumber, endNumber, nil
	}

	defaultStart := currentNumber - readerAssistantMaxEpisodeRangeCount + 1
	if defaultStart < 1 {
		defaultStart = 1
	}
	if hasStart || hasEnd {
		defaultStart = 1
	}
	startNumber, err := readerAssistantEpisodeNumberArg(args["startEpisodeNumber"], defaultStart, currentNumber)
	if err != nil {
		return 0, 0, err
	}
	endNumber, err := readerAssistantEpisodeNumberArg(args["endEpisodeNumber"], currentNumber, currentNumber)
	if err != nil {
		return 0, 0, err
	}
	if startNumber > endNumber {
		return 0, 0, errors.New("startEpisodeNumber must be less than or equal to endEpisodeNumber.")
	}
	if endNumber-startNumber+1 > readerAssistantMaxEpisodeRangeCount {
		return 0, 0, fmtEpisodeRangeError(readerAssistantMaxEpisodeRangeCount)
	}
	return startNumber, endNumber, nil
}

func resolveReaderAssistantSearchRange(contextInfo readerAssistantContext, args map[string]any) (int, int, error) {
	maxNumber := contextInfo.CurrentEpisodeNumber
	if maxNumber == 0 {
		maxNumber = episodeNumberByIndex(contextInfo.TocEpisodes, contextInfo.CurrentEpisodeIndex)
	}
	if maxNumber == 0 {
		return 1, 1, errors.New("current episode was not found in the table of contents.")
	}
	hasStart := args["startEpisodeNumber"] != nil
	hasEnd := args["endEpisodeNumber"] != nil
	defaultStart := maxNumber - readerAssistantMaxSearchEpisodeCount + 1
	if defaultStart < 1 {
		defaultStart = 1
	}
	if hasStart || hasEnd {
		defaultStart = 1
	}
	startNumber, err := readerAssistantEpisodeNumberArg(args["startEpisodeNumber"], defaultStart, maxNumber)
	if err != nil {
		return 0, 0, err
	}
	endNumber, err := readerAssistantEpisodeNumberArg(args["endEpisodeNumber"], maxNumber, maxNumber)
	if err != nil {
		return 0, 0, err
	}
	if startNumber > endNumber {
		return 0, 0, errors.New("startEpisodeNumber must be less than or equal to endEpisodeNumber.")
	}
	if endNumber-startNumber+1 > readerAssistantMaxSearchEpisodeCount {
		return 0, 0, errors.New("search range must be 50 episodes or fewer.")
	}
	return startNumber, endNumber, nil
}

func readerAssistantEpisodeNumberArg(value any, fallback int, maxNumber int) (int, error) {
	if value == nil {
		return fallback, nil
	}
	var number int
	switch typed := value.(type) {
	case float64:
		if typed != float64(int(typed)) {
			return 0, errors.New("episode number must be an integer.")
		}
		number = int(typed)
	case int:
		number = typed
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err != nil {
			return 0, errors.New("episode number must be an integer.")
		}
		number = parsed
	default:
		return 0, errors.New("episode number must be an integer.")
	}
	if number < 1 || number > maxNumber {
		return 0, errors.New("episode number must be between 1 and the current episode number.")
	}
	return number, nil
}

func fmtEpisodeRangeError(maxCount int) error {
	return errors.New("episode range must be " + strconv.Itoa(maxCount) + " episodes or fewer.")
}

func readerAssistantEpisodeVisible(contextInfo readerAssistantContext, episodeIndex string) bool {
	targetNumber := episodeNumberByIndex(contextInfo.TocEpisodes, episodeIndex)
	if targetNumber == 0 {
		return false
	}
	currentNumber := contextInfo.CurrentEpisodeNumber
	if currentNumber == 0 {
		currentNumber = episodeNumberByIndex(contextInfo.TocEpisodes, contextInfo.CurrentEpisodeIndex)
	}
	return currentNumber > 0 && targetNumber <= currentNumber
}

var readerAssistantRecentPattern = regexp.MustCompile(`直近\s*([0-9０-９]+)\s*話`)

func readerAssistantRecentPreviousEpisodeCount(message string) int {
	if regexp.MustCompile(`(?:現在|今(?:の|読んでいる)?|表示中)の?話?(?:も|を)?含`).MatchString(message) {
		return 0
	}
	match := readerAssistantRecentPattern.FindStringSubmatch(message)
	if len(match) < 2 {
		return 0
	}
	value, err := strconv.Atoi(toASCIIDigits(match[1]))
	if err != nil || value < 1 {
		return 0
	}
	return value
}

func toASCIIDigits(value string) string {
	return strings.Map(func(r rune) rune {
		if r >= '０' && r <= '９' {
			return r - '０' + '0'
		}
		return r
	}, value)
}

func episodeNumberByIndex(tocEpisodes []library.TocEpisodeSummary, episodeIndex string) int {
	for index, episode := range tocEpisodes {
		if episode.EpisodeIndex == episodeIndex {
			return index + 1
		}
	}
	return 0
}

func readerAssistantSearchQuery(message string) string {
	fields := strings.Fields(strings.TrimSpace(message))
	best := ""
	for _, field := range fields {
		field = strings.Trim(field, "「」『』。、,.!?！？:：;；()（）[]【】")
		if len([]rune(field)) > len([]rune(best)) && len([]rune(field)) <= 80 {
			best = field
		}
	}
	if len([]rune(best)) < 2 {
		return ""
	}
	return best
}

func snippetAround(text string, position int, matchLength int, limit int) string {
	runes := []rune(text)
	if len(runes) == 0 {
		return ""
	}
	if position > len(runes) {
		position = len(runes)
	}
	start := position - limit/2
	if start < 0 {
		start = 0
	}
	end := start + limit
	if end > len(runes) {
		end = len(runes)
	}
	return strings.TrimSpace(string(runes[start:end]))
}

func runeOffsetForByteIndex(text string, byteIndex int) int {
	if byteIndex <= 0 {
		return 0
	}
	if byteIndex >= len(text) {
		return len([]rune(text))
	}
	return len([]rune(text[:byteIndex]))
}

func readerAssistantUsageRequests(toolRequests []map[string]any, inputTokens int, outputTokens int) []ai.UsageRequest {
	requests := make([]ai.UsageRequest, 0, len(toolRequests)+1)
	for index, toolRequest := range toolRequests {
		name, _ := toolRequest["name"].(string)
		requests = append(requests, ai.UsageRequest{
			RequestIndex:  index,
			Kind:          "tool_call",
			ToolNames:     []string{name},
			ToolSummaries: []string{readerAssistantToolResultMessage(name)},
		})
	}
	requests = append(requests, ai.UsageRequest{
		RequestIndex:  len(requests),
		Kind:          "final_answer",
		ToolNames:     []string{},
		ToolSummaries: []string{},
		InputTokens:   inputTokens,
		OutputTokens:  outputTokens,
		TotalTokens:   inputTokens + outputTokens,
	})
	return requests
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func elapsedMsBetweenISO(startedAt string, finishedAt string) int {
	started, err := time.Parse(time.RFC3339Nano, startedAt)
	if err != nil {
		return 0
	}
	finished, err := time.Parse(time.RFC3339Nano, finishedAt)
	if err != nil || finished.Before(started) {
		return 0
	}
	return int(finished.Sub(started).Milliseconds())
}

func maxInt(left int, right int) int {
	if left > right {
		return left
	}
	return right
}

func estimateTokenCount(value string) int {
	trimmed := strings.TrimSpace(value)
	runes := len([]rune(trimmed))
	if runes == 0 {
		return 0
	}
	// Japanese prose is often much closer to one token per rune than the
	// common English "chars / 4" estimate. Use the larger value for safety.
	return maxInt(runes, (len(trimmed)+3)/4)
}

func estimateChatMessagesTokenCount(messages []ai.ChatMessage) int {
	total := 0
	for _, message := range messages {
		total += estimateTokenCount(message.Role)
		switch content := message.Content.(type) {
		case string:
			total += estimateTokenCount(content)
		case nil:
		default:
			raw, err := json.Marshal(content)
			if err == nil {
				total += estimateTokenCount(string(raw))
			}
		}
		for _, toolCall := range message.ToolCalls {
			total += estimateTokenCount(toolCall.Function.Name)
			total += estimateTokenCount(toolCall.Function.Arguments)
		}
	}
	return total
}

func estimateOpenRouterChatRequestTokens(messages []ai.ChatMessage, tools []ai.ToolDefinition, responseFormat any) int {
	payload := map[string]any{
		"messages": messages,
	}
	if len(tools) > 0 {
		payload["tools"] = tools
	}
	if responseFormat != nil {
		payload["response_format"] = responseFormat
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return estimateChatMessagesTokenCount(messages)
	}
	return estimateTokenCount(string(raw))
}

func resolveOpenRouterMaxOutputTokens(ctx context.Context, apiKey string, modelID string, providerOrder []string, fallback int, promptTokens int) (int, error) {
	if fallback <= 0 {
		fallback = 4096
	}
	maxTokens := fallback
	lookupCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	info, ok := ai.LookupOpenRouterModelInfo(lookupCtx, apiKey, modelID, providerOrder)
	cancel()
	if ok {
		if info.MaxCompletionTokens > 0 {
			maxTokens = info.MaxCompletionTokens
		}
		if info.ContextLength > 0 && promptTokens > 0 {
			available := info.ContextLength - promptTokens - 256
			if available <= 0 {
				return 0, fmt.Errorf("%w: prompt estimate %d tokens is too large for context length %d. Reduce conversation history, tool results, or target text.", errOpenRouterContextTooLarge, promptTokens, info.ContextLength)
			}
			if available < maxTokens {
				maxTokens = available
			}
		}
	}
	if maxTokens < 1 {
		return 0, errors.New("OpenRouter max_tokens could not be resolved.")
	}
	return maxTokens, nil
}

func stringPtrOrNil(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return &value
}

func truncateRunes(value string, limit int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit])
}

func readerDocumentBodyText(document library.ReaderDocument) string {
	return readertextcache.BodyText(document)
}

func episodeIndexesFromToc(episodes []library.TocEpisodeSummary) []string {
	indexes := make([]string, 0, len(episodes))
	for _, episode := range episodes {
		indexes = append(indexes, episode.EpisodeIndex)
	}
	return indexes
}

func NewHitRegistry() *HitRegistry {
	return newReaderAssistantHitRegistry()
}

func SearchQuery(message string) string {
	return readerAssistantSearchQuery(message)
}

func SnippetAround(text string, position int, matchLength int, contextChars int) string {
	return snippetAround(text, position, matchLength, contextChars)
}

func RuneOffsetForByteIndex(text string, byteIndex int) int {
	return runeOffsetForByteIndex(text, byteIndex)
}

func UsageRequests(toolRequests []map[string]any, inputTokens int, outputTokens int) []ai.UsageRequest {
	return readerAssistantUsageRequests(toolRequests, inputTokens, outputTokens)
}

func UsageRecentPreviousRange(currentNumber int, count int) map[string]any {
	return readerAssistantUsageRecentPreviousRange(currentNumber, count)
}

func SanitizeSnapshotValue(value any) any {
	return sanitizeReaderAssistantSnapshotValue(value)
}

func RangeAround(tocEpisodes []library.TocEpisodeSummary, episodeIndex string) (int, int) {
	return readerAssistantRangeAround(tocEpisodes, episodeIndex)
}

func ResolveEpisodeRange(contextInfo Context, args map[string]any) (int, int, error) {
	return resolveReaderAssistantEpisodeRange(contextInfo, args)
}

func EpisodeNumberArg(value any, minNumber int, maxNumber int) (int, error) {
	return readerAssistantEpisodeNumberArg(value, minNumber, maxNumber)
}

func EpisodeVisible(contextInfo Context, episodeIndex string) bool {
	return readerAssistantEpisodeVisible(contextInfo, episodeIndex)
}

func ToolResultMessage(name string) string {
	return readerAssistantToolResultMessage(name)
}

func RecentPreviousScopeNote(contextInfo Context) string {
	return readerAssistantRecentPreviousScopeNote(contextInfo)
}

func ToolRecovery(name string, err error) ToolResult {
	return readerAssistantToolRecovery(name, err)
}

func FmtEpisodeRangeError(maxCount int) error {
	return fmtEpisodeRangeError(maxCount)
}

func RecentPreviousEpisodeCount(message string) int {
	return readerAssistantRecentPreviousEpisodeCount(message)
}

func MaxResultsArg(value any) (int, error) {
	return readerAssistantMaxResultsArg(value)
}

func FullTextQueryArg(value string) (string, error) {
	return readerAssistantFullTextQueryArg(value)
}

func ContextCharsArg(value any) (int, error) {
	return readerAssistantContextCharsArg(value)
}

func HitIDsArg(value any) ([]string, error) {
	return readerAssistantHitIDsArg(value)
}

func SearchTerms(query string) []string {
	return readerAssistantSearchTerms(query)
}

func FindQueryPositions(text string, query string, terms []string, maxResults int) []int {
	return readerAssistantFindQueryPositions(text, query, terms, maxResults)
}

func TitleScore(title string, query string, terms []string) float64 {
	return readerAssistantTitleScore(title, query, terms)
}

func FullTextScore(text string, position int, query string, terms []string, titleScore float64) float64 {
	return readerAssistantFullTextScore(text, position, query, terms, titleScore)
}

func CoverageFullTextCandidates(primary []FullTextCandidate, fallback []FullTextCandidate, bucketCount int) []FullTextCandidate {
	return readerAssistantCoverageFullTextCandidates(primary, fallback, bucketCount)
}

func FullTextDistribution(candidates []FullTextCandidate) (int, any, any) {
	return readerAssistantFullTextDistribution(candidates)
}

func MatchLengthAt(text string, position int, query string, terms []string) int {
	return readerAssistantMatchLengthAt(text, position, query, terms)
}

func PassageRange(text string, position int, matchLength int, contextChars int) (int, int) {
	return readerAssistantPassageRange(text, position, matchLength, contextChars)
}

func SubstringRunes(text string, start int, end int) string {
	return substringRunes(text, start, end)
}

func ByteIndexForRuneOffset(text string, offset int) int {
	return byteIndexForRuneOffset(text, offset)
}

func IntSliceContains(values []int, target int) bool {
	return intSliceContains(values, target)
}

func StringValue(value *string) string {
	return stringValue(value)
}

func ToolDefinitions() []ai.ToolDefinition {
	return readerAssistantToolDefinitions()
}

func EpisodeReference(episode *library.EpisodeResponse) map[string]any {
	return episodeReference(episode)
}

func EpisodeNumberByIndex(tocEpisodes []library.TocEpisodeSummary, episodeIndex string) int {
	return episodeNumberByIndex(tocEpisodes, episodeIndex)
}

func ResolveSearchRange(contextInfo Context, args map[string]any) (int, int, error) {
	return resolveReaderAssistantSearchRange(contextInfo, args)
}

func ResolveFullTextSearchRange(contextInfo Context, args map[string]any) (int, int, error) {
	return resolveReaderAssistantFullTextSearchRange(contextInfo, args)
}

func BuildInput(contextInfo Context) string {
	return buildReaderAssistantInput(contextInfo)
}

func BuildInstructions(contextInfo Context) string {
	return buildReaderAssistantInstructions(contextInfo)
}

func DecodeToolArguments(raw string) map[string]any {
	return decodeToolArguments(raw)
}

func MustJSON(value any) string {
	return mustJSON(value)
}

func SummaryPurpose(value any) string {
	return readerAssistantSummaryPurpose(value)
}

func SummaryFocus(value any) any {
	return readerAssistantSummaryFocus(value)
}

func EstimateOpenRouterChatRequestTokens(messages []ai.ChatMessage, tools []ai.ToolDefinition, responseFormat any) int {
	return estimateOpenRouterChatRequestTokens(messages, tools, responseFormat)
}

func ResolveOpenRouterMaxOutputTokens(ctx context.Context, apiKey string, modelID string, providerOrder []string, fallback int, promptTokens int) (int, error) {
	return resolveOpenRouterMaxOutputTokens(ctx, apiKey, modelID, providerOrder, fallback, promptTokens)
}

func (s *Service) ToolContext(ctx context.Context, novelID string, novelTitle string, currentEpisodeIndex string, currentEpisodeRef map[string]any, currentExcerpt string, message string, tocEpisodes []library.TocEpisodeSummary, streamSink StreamSink) ([]map[string]any, []map[string]any, bool) {
	return s.readerAssistantToolContext(ctx, novelID, novelTitle, currentEpisodeIndex, currentEpisodeRef, currentExcerpt, message, tocEpisodes, streamSink)
}

func (s *Service) PreviousEpisodeResult(ctx context.Context, novelID string, novelTitle string, currentEpisodeIndex string, tocEpisodes []library.TocEpisodeSummary) map[string]any {
	return s.previousEpisodeResult(ctx, novelID, novelTitle, currentEpisodeIndex, tocEpisodes)
}

func (s *Service) LoadEpisodeRangeResult(ctx context.Context, novelID string, novelTitle string, tocEpisodes []library.TocEpisodeSummary, startNumber int, endNumber int) map[string]any {
	return s.loadEpisodeRangeResult(ctx, novelID, novelTitle, tocEpisodes, startNumber, endNumber)
}

func (s *Service) Episode(ctx context.Context, novelID string, episodeIndex string) (*library.EpisodeResponse, error) {
	return s.readerAssistantEpisode(ctx, novelID, episodeIndex)
}

func (s *Service) CharacterSnapshotResult(novelID string, currentEpisodeIndex string, tocEpisodes []library.TocEpisodeSummary) map[string]any {
	return s.characterSnapshotResult(novelID, currentEpisodeIndex, tocEpisodes)
}

func (s *Service) TermSnapshotResult(novelID string, currentEpisodeIndex string, tocEpisodes []library.TocEpisodeSummary) map[string]any {
	return s.termSnapshotResult(novelID, currentEpisodeIndex, tocEpisodes)
}

func (s *Service) ResolveConfig() (*store.ResolvedAIGenerationConfig, error) {
	return s.resolveReaderAssistantConfig()
}

func (s *Service) RecordUsage(input UsageInput) error {
	return s.recordReaderAssistantUsage(input)
}

func (s *Service) SearchFullTextResult(ctx context.Context, contextInfo Context, query string, startNumber int, endNumber int, maxResults int) map[string]any {
	return s.searchFullTextResult(ctx, contextInfo, query, startNumber, endNumber, maxResults)
}

func (s *Service) LoadPassagesResult(ctx context.Context, contextInfo Context, hitIDs []string, contextChars int) ToolResult {
	return s.loadPassagesResult(ctx, contextInfo, hitIDs, contextChars)
}

func (s *Service) SearchEpisodesResult(ctx context.Context, novelID string, query string, tocEpisodes []library.TocEpisodeSummary, maxEpisodeIndex string) map[string]any {
	return s.searchEpisodesResult(ctx, novelID, query, tocEpisodes, maxEpisodeIndex)
}

func (s *Service) ExecuteTool(ctx context.Context, contextInfo Context, name string, rawArguments string) ToolResult {
	return s.executeReaderAssistantTool(ctx, contextInfo, name, rawArguments)
}

func (s *Service) EpisodeText(ctx context.Context, novelID string, episodeIndex string, registry *HitRegistry) *EpisodeText {
	return s.readerAssistantEpisodeText(ctx, novelID, episodeIndex, registry)
}

func (s *Service) RunAgentLoop(ctx context.Context, assistantContext Context, config ai.OpenRouterConfig, streamSink StreamSink) (ai.ChatResult, []map[string]any, []map[string]any, error) {
	return s.runReaderAssistantAgentLoop(ctx, assistantContext, config, streamSink)
}
