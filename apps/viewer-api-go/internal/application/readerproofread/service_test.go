package readerproofread

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"narou-viewer/apps/viewer-api-go/internal/ai"
	"narou-viewer/apps/viewer-api-go/internal/library"
	"narou-viewer/apps/viewer-api-go/internal/store"
)

type fakeLibrary struct {
	episode *library.EpisodeResponse
	err     error
}

func (f fakeLibrary) GetEpisode(context.Context, string, string) (*library.EpisodeResponse, error) {
	if f.episode == nil {
		return nil, f.err
	}
	copy := *f.episode
	return &copy, f.err
}

type fakeSettings struct {
	config         *store.ResolvedAIGenerationConfig
	readerSettings store.NovelReaderSettings
	err            error
}

func (f fakeSettings) ResolveActiveAIGenerationConfig() (*store.ResolvedAIGenerationConfig, error) {
	return f.config, f.err
}

func (f fakeSettings) GetNovelReaderSettings(string) (store.NovelReaderSettings, error) {
	return f.readerSettings, f.err
}

func testEpisode() *library.EpisodeResponse {
	return &library.EpisodeResponse{
		NovelID: "novel-a", EpisodeIndex: "1", ContentEtag: "source-etag",
		ReaderDocument: library.ReaderDocument{Version: 1, Blocks: []library.ReaderBlock{
			{Type: "title", Text: "第一話"},
			{Type: "paragraph", Section: "body", Inlines: []library.ReaderInline{{Type: "text", Text: "文の途"}, {Type: "lineBreak"}, {Type: "text", Text: "中です。"}}},
			{Type: "paragraph", Section: "body", Inlines: []library.ReaderInline{{Type: "text", Text: " 次の文です。"}}},
			{Type: "image", Section: "body", Src: "/image.jpg"},
			{Type: "paragraph", Section: "postscript", Inlines: []library.ReaderInline{{Type: "ruby", Text: "後書", Ruby: "あとがき"}}},
		}},
	}
}

func TestGenerateStoresAndLoadsProofreadDocument(t *testing.T) {
	dir := t.TempDir()
	var received []ai.ChatMessage
	service := NewService(Dependencies{
		Library: fakeLibrary{episode: testEpisode()},
		Settings: fakeSettings{config: &store.ResolvedAIGenerationConfig{
			ProfileID: "default", ProfileLabel: "Default", APIKey: "test-key", ModelID: "test-model",
			AllowFallbacks: true,
		}},
		StateDir: dir,
		Generate: func(_ context.Context, config ai.OpenRouterConfig, messages []ai.ChatMessage) (ai.ChatResult, error) {
			received = messages
			if config.ModelID != "test-model" || config.ResponseFormat == nil {
				t.Fatalf("config = %+v", config)
			}
			answer, _ := json.Marshal(proofreadOutput{Segments: []proofreadSegment{{
				ID: 0, Paragraphs: []string{"文の途中です。", "次の文です。"},
			}}})
			return ai.ChatResult{Answer: string(answer), InputTokens: 10, OutputTokens: 5, TotalTokens: 15}, nil
		},
	})

	before, err := service.Get(context.Background(), "novel-a", "1")
	if err != nil || before.Status != "not_generated" {
		t.Fatalf("before = %+v err=%v", before, err)
	}
	generated, err := service.Generate(context.Background(), "novel-a", "1")
	if err != nil {
		t.Fatal(err)
	}
	if len(received) != 2 || generated.Status != "ready" || generated.ReaderDocument == nil {
		t.Fatalf("generated = %+v messages=%+v", generated, received)
	}
	prompt, ok := received[0].Content.(string)
	if !ok || !strings.Contains(prompt, "孤立して混入した不自然な空白") {
		t.Fatalf("proofread prompt does not cover stray in-sentence spaces: %q", received[0].Content)
	}
	blocks := generated.ReaderDocument.Blocks
	if len(blocks) != 5 || blocks[1].Inlines[0].Text != "文の途中です。" ||
		blocks[2].Inlines[0].Text != "次の文です。" || blocks[3].Type != "image" {
		t.Fatalf("blocks = %+v", blocks)
	}
	loaded, err := service.Get(context.Background(), "novel-a", "1")
	if err != nil || loaded.Status != "ready" || loaded.ReaderDocument == nil {
		t.Fatalf("loaded = %+v err=%v", loaded, err)
	}
	if err := service.Delete("novel-a", "1"); err != nil {
		t.Fatal(err)
	}
	after, err := service.Get(context.Background(), "novel-a", "1")
	if err != nil || after.Status != "not_generated" {
		t.Fatalf("after = %+v err=%v", after, err)
	}
}

func TestEditableSegmentsPreserveBlankParagraphsAsBoundaries(t *testing.T) {
	document := library.ReaderDocument{Version: 1, Blocks: []library.ReaderBlock{
		{Type: "paragraph", Section: "body", Inlines: []library.ReaderInline{{Type: "text", Text: "前半"}}},
		{Type: "paragraph", Section: "body", Inlines: []library.ReaderInline{{Type: "lineBreak"}}},
		{Type: "paragraph", Section: "body", Inlines: []library.ReaderInline{{Type: "text", Text: "後半"}}},
	}}
	segments := editableSegments(document)
	if len(segments) != 2 || segments[0].EndBlock != 1 || segments[1].StartBlock != 2 {
		t.Fatalf("segments=%+v", segments)
	}
	output := proofreadOutput{Segments: []proofreadSegment{
		{ID: 0, Paragraphs: []string{"前半"}},
		{ID: 1, Paragraphs: []string{"後半"}},
	}}
	corrected, err := applyOutput(document, segments, output)
	if err != nil {
		t.Fatal(err)
	}
	if len(corrected.Blocks) != 3 || !isBlankParagraph(corrected.Blocks[1]) {
		t.Fatalf("corrected=%+v", corrected.Blocks)
	}
}

func TestApplyOutputPreservesSentenceEndingParagraphStyleAndAcceptsWhitespaceCleanup(t *testing.T) {
	document := library.ReaderDocument{Version: 1, Blocks: []library.ReaderBlock{
		{Type: "paragraph", Section: "body", Inlines: []library.ReaderInline{{Type: "text", Text: "一文目です。"}}},
		{Type: "paragraph", Section: "body", Inlines: []library.ReaderInline{{Type: "text", Text: "３匹程度 。"}}},
	}}
	segments := editableSegments(document)
	if _, err := applyOutput(document, segments, proofreadOutput{Segments: []proofreadSegment{{
		ID: 0, Paragraphs: []string{"一文目です。３匹程度 。"},
	}}}); err == nil {
		t.Fatal("sentence-ending source paragraphs must not be merged")
	}
	corrected, err := applyOutput(document, segments, proofreadOutput{Segments: []proofreadSegment{{
		ID: 0, Paragraphs: []string{"一文目です。", "３匹程度。"},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if got := corrected.Blocks[1].Inlines[0].Text; got != "３匹程度。" {
		t.Fatalf("whitespace cleanup = %q", got)
	}

	singleParagraph := library.ReaderDocument{Version: 1, Blocks: []library.ReaderBlock{{
		Type: "paragraph", Section: "body", Inlines: []library.ReaderInline{{Type: "text", Text: "一文目です。二文目です。"}},
	}}}
	singleSegment := editableSegments(singleParagraph)
	if _, err := applyOutput(singleParagraph, singleSegment, proofreadOutput{Segments: []proofreadSegment{{
		ID: 0, Paragraphs: []string{"一文目です。", "二文目です。"},
	}}}); err == nil {
		t.Fatal("AI must not introduce sentence-ending paragraph boundaries absent from the source")
	}

	midSentence := library.ReaderDocument{Version: 1, Blocks: []library.ReaderBlock{
		{Type: "paragraph", Section: "body", Inlines: []library.ReaderInline{{Type: "text", Text: "文の"}}},
		{Type: "paragraph", Section: "body", Inlines: []library.ReaderInline{{Type: "text", Text: "途中です。"}}},
	}}
	midSentenceSegments := editableSegments(midSentence)
	if _, err := applyOutput(midSentence, midSentenceSegments, proofreadOutput{Segments: []proofreadSegment{{
		ID: 0, Paragraphs: []string{"文の途中です。"},
	}}}); err != nil {
		t.Fatalf("mid-sentence boundary should remain editable: %v", err)
	}
	if endsWithSentencePunctuation("   ") {
		t.Fatal("blank text must not be treated as sentence-ending")
	}
	if preservesSentenceEndingParagraphStyle(
		[]string{"一。", "二。三。"},
		[]string{"一。二。", "三。"},
	) {
		t.Fatal("moving a sentence-ending paragraph boundary must fail")
	}
}

func TestGetAppliesCurrentDeterministicCorrectionsToStoredProofread(t *testing.T) {
	dir := t.TempDir()
	readerSettings := store.NovelReaderSettings{}
	readerSettings.Correction.HyphenDashNormalization = true
	episode := testEpisode()
	episode.ReaderDocument = library.ReaderDocument{Version: 1, Blocks: []library.ReaderBlock{{
		Type: "paragraph", Section: "body", Inlines: []library.ReaderInline{{Type: "text", Text: "校正版--本文"}},
	}}}
	service := NewService(Dependencies{
		Library: fakeLibrary{episode: episode}, Settings: fakeSettings{readerSettings: readerSettings}, StateDir: dir,
	})
	if err := service.write(storedResult{
		FormatVersion: 1, PromptVersion: promptVersion, NovelID: "novel-a", EpisodeIndex: "1",
		SourceETag: episode.ContentEtag, ReaderDocument: episode.ReaderDocument,
	}); err != nil {
		t.Fatal(err)
	}
	response, err := service.Get(context.Background(), "novel-a", "1")
	if err != nil || response.ReaderDocument == nil {
		t.Fatalf("response=%+v err=%v", response, err)
	}
	if got := response.ReaderDocument.Blocks[0].Inlines[0].Text; got == "校正版--本文" {
		t.Fatalf("normal correction was not applied: %q", got)
	}
}

func TestGenerateRejectsUnavailableAndInvalidOutputs(t *testing.T) {
	episode := testEpisode()
	service := NewService(Dependencies{Library: fakeLibrary{episode: episode}, StateDir: t.TempDir()})
	if _, err := service.Generate(context.Background(), "novel-a", "1"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}

	service = NewService(Dependencies{
		Library:  fakeLibrary{episode: episode},
		Settings: fakeSettings{config: &store.ResolvedAIGenerationConfig{APIKey: "key", ModelID: "model"}},
		StateDir: t.TempDir(),
		Generate: func(context.Context, ai.OpenRouterConfig, []ai.ChatMessage) (ai.ChatResult, error) {
			return ai.ChatResult{Answer: `{"segments":[]}`}, nil
		},
	})
	if _, err := service.Generate(context.Background(), "novel-a", "1"); err == nil {
		t.Fatal("missing segment should fail")
	}
}

func TestGenerateSkipsRichOnlyDocumentAndPropagatesErrors(t *testing.T) {
	rich := testEpisode()
	rich.ReaderDocument.Blocks = []library.ReaderBlock{{
		Type: "paragraph", Section: "body", Inlines: []library.ReaderInline{{Type: "ruby", Text: "本文", Ruby: "ほんぶん"}},
	}}
	service := NewService(Dependencies{Library: fakeLibrary{episode: rich}, StateDir: t.TempDir()})
	if _, err := service.Generate(context.Background(), "novel-a", "1"); !errors.Is(err, ErrUnsupportedDocument) {
		t.Fatalf("err = %v", err)
	}

	expected := errors.New("load failed")
	service = NewService(Dependencies{Library: fakeLibrary{err: expected}, StateDir: t.TempDir()})
	if _, err := service.Get(context.Background(), "novel-a", "1"); !errors.Is(err, expected) {
		t.Fatalf("err = %v", err)
	}
}

func TestOutputValidationAndStaleStoredResult(t *testing.T) {
	document := testEpisode().ReaderDocument
	segments := editableSegments(document)
	for name, output := range map[string]proofreadOutput{
		"duplicate": {Segments: []proofreadSegment{{ID: 0, Paragraphs: []string{"a"}}, {ID: 0, Paragraphs: []string{"b"}}}},
		"blank":     {Segments: []proofreadSegment{{ID: 0, Paragraphs: []string{"  "}}}},
		"missing":   {Segments: []proofreadSegment{}},
		"rewrite":   {Segments: []proofreadSegment{{ID: 0, Paragraphs: []string{"別の文章です。"}}}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := applyOutput(document, segments, output); err == nil {
				t.Fatal("invalid output should fail")
			}
		})
	}

	dir := t.TempDir()
	service := NewService(Dependencies{Library: fakeLibrary{episode: testEpisode()}, StateDir: dir})
	stored := storedResult{
		FormatVersion: 1, PromptVersion: promptVersion, NovelID: "novel-a", EpisodeIndex: "1",
		SourceETag: "old", GeneratedAt: "2026-01-01T00:00:00Z", ModelID: "model",
		ReaderDocument: document,
	}
	if err := service.write(stored); err != nil {
		t.Fatal(err)
	}
	response, err := service.Get(context.Background(), "novel-a", "1")
	if err != nil || response.Status != "not_generated" {
		t.Fatalf("response=%+v err=%v", response, err)
	}
	if err := service.Delete("novel-a", "1"); err != nil {
		t.Fatal(err)
	}
	if err := service.Delete("novel-a", "1"); err != nil {
		t.Fatal(err)
	}
}

func TestGenerateHandlesSettingsGeneratorAndJSONErrors(t *testing.T) {
	episode := testEpisode()
	expected := errors.New("settings failed")
	service := NewService(Dependencies{
		Library: fakeLibrary{episode: episode}, Settings: fakeSettings{err: expected}, StateDir: t.TempDir(),
	})
	if _, err := service.Generate(context.Background(), "novel-a", "1"); !errors.Is(err, expected) {
		t.Fatalf("err=%v", err)
	}

	expected = errors.New("provider failed")
	service = NewService(Dependencies{
		Library:  fakeLibrary{episode: episode},
		Settings: fakeSettings{config: &store.ResolvedAIGenerationConfig{APIKey: "key", ModelID: "model"}},
		StateDir: t.TempDir(),
		Generate: func(context.Context, ai.OpenRouterConfig, []ai.ChatMessage) (ai.ChatResult, error) {
			return ai.ChatResult{}, expected
		},
	})
	if _, err := service.Generate(context.Background(), "novel-a", "1"); !errors.Is(err, expected) {
		t.Fatalf("err=%v", err)
	}

	service.generate = func(context.Context, ai.OpenRouterConfig, []ai.ChatMessage) (ai.ChatResult, error) {
		return ai.ChatResult{Answer: "not-json"}, nil
	}
	if _, err := service.Generate(context.Background(), "novel-a", "1"); err == nil {
		t.Fatal("malformed JSON should fail")
	}
}

func TestReadRejectsCorruptAndMismatchedFiles(t *testing.T) {
	service := NewService(Dependencies{Library: fakeLibrary{episode: testEpisode()}, StateDir: t.TempDir()})
	path := service.path("novel-a", "1")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.read("novel-a", "1"); err == nil {
		t.Fatal("corrupt result should fail")
	}
	if err := service.write(storedResult{
		FormatVersion: 1, PromptVersion: promptVersion, NovelID: "novel-a", EpisodeIndex: "1",
		SourceETag: "source-etag", ReaderDocument: testEpisode().ReaderDocument,
	}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var stored map[string]any
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatal(err)
	}
	stored["novelId"] = "other"
	raw, _ = json.Marshal(stored)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.read("novel-a", "1"); err == nil {
		t.Fatal("mismatched result should fail")
	}
}

func TestGenerateRecordsUsageWithoutPromptOrOutputSnapshot(t *testing.T) {
	dir := t.TempDir()
	dbPath := dir + "/ai_usage.sqlite"
	service := NewService(Dependencies{
		Library: fakeLibrary{episode: testEpisode()},
		Settings: fakeSettings{config: &store.ResolvedAIGenerationConfig{
			ProfileID: "profile", ProfileLabel: "Profile", APIKey: "key", ModelID: "model",
		}},
		StateDir: dir, UsageDBPath: dbPath,
		Generate: func(context.Context, ai.OpenRouterConfig, []ai.ChatMessage) (ai.ChatResult, error) {
			return ai.ChatResult{
				Answer:      `{"segments":[{"id":0,"paragraphs":["文の途中です。","次の文です。"]}]}`,
				InputTokens: 8, OutputTokens: 4, TotalTokens: 12,
			}, nil
		},
	})
	if _, err := service.Generate(context.Background(), "novel-a", "1"); err != nil {
		t.Fatal(err)
	}
	usage, ok, err := ai.LoadUsage(dbPath)
	if err != nil || !ok || len(usage.Runs) != 1 {
		t.Fatalf("usage=%+v ok=%v err=%v", usage, ok, err)
	}
	run := usage.Runs[0]
	if run.Feature != "reader_proofread" || run.TotalTokens != 12 || run.HasSnapshot {
		t.Fatalf("run=%+v", run)
	}
}

func TestNilLibraryAndFailedUsageAreSafe(t *testing.T) {
	service := NewService(Dependencies{StateDir: t.TempDir()})
	response, err := service.Get(context.Background(), "novel-a", "1")
	if err != nil || response.Status != "" {
		t.Fatalf("response=%+v err=%v", response, err)
	}

	dbPath := t.TempDir() + "/ai_usage.sqlite"
	config := &store.ResolvedAIGenerationConfig{
		ProfileID: "profile", ProfileLabel: "Profile", APIKey: "key", ModelID: "model",
	}
	service.usageDBPath = dbPath
	service.recordUsage(
		time.Unix(1, 0), "novel-a", "1", config, ai.ChatResult{},
		errors.New("provider failed"),
	)
	usage, ok, err := ai.LoadUsage(dbPath)
	if err != nil || !ok || len(usage.Runs) != 1 || usage.Runs[0].Status != "failed" || usage.Runs[0].ErrorMessage == nil {
		t.Fatalf("usage=%+v ok=%v err=%v", usage, ok, err)
	}
}

func TestPruneNovelStateDeletesAllEpisodeResults(t *testing.T) {
	dir := t.TempDir()
	service := NewService(Dependencies{StateDir: dir})
	for _, episodeIndex := range []string{"1", "2"} {
		if err := service.write(storedResult{
			FormatVersion: 1, PromptVersion: promptVersion, NovelID: "novel-a", EpisodeIndex: episodeIndex,
			SourceETag: "etag", ReaderDocument: testEpisode().ReaderDocument,
		}); err != nil {
			t.Fatal(err)
		}
	}
	deleted, err := PruneNovelState(dir, "novel-a")
	if err != nil || deleted != 2 {
		t.Fatalf("deleted=%d err=%v", deleted, err)
	}
	deleted, err = PruneNovelState(dir, "novel-a")
	if err != nil || deleted != 0 {
		t.Fatalf("second deleted=%d err=%v", deleted, err)
	}
}
