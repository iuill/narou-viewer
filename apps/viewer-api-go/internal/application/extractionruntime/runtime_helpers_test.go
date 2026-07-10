package extractionruntime

import (
	"testing"

	"narou-viewer/apps/viewer-api-go/internal/ai"
	"narou-viewer/apps/viewer-api-go/internal/characters"
	"narou-viewer/apps/viewer-api-go/internal/library"
)

func TestRuntimeHelperCoverage(t *testing.T) {
	if got := FormatTimingFields("stage", "test", "count", 2); got != "stage=test count=2" {
		t.Fatalf("FormatTimingFields = %q", got)
	}

	tokens := estimateChatMessagesTokenCount([]ai.ChatMessage{
		{Role: "system", Content: "alpha beta"},
		{Role: "user", Content: "gamma"},
	})
	if tokens <= 0 {
		t.Fatalf("estimateChatMessagesTokenCount = %d", tokens)
	}

	inline := renderSummaryInlineTokens([]library.ReaderInline{
		{Type: "text", Text: "アリス"},
		{Type: "ruby", Text: "姫", Ruby: "ひめ"},
		{Type: "link", Children: []library.ReaderInline{{Type: "text", Text: "登場"}}},
	})
	if inline != "アリス姫登場" {
		t.Fatalf("renderSummaryInlineTokens = %q", inline)
	}
}

func TestExtractionTimingLogEnvironmentFallback(t *testing.T) {
	t.Setenv("VIEWER_CHARACTER_SUMMARY_TIMING_LOG", "1")
	if !extractionTimingLogEnabled() {
		t.Fatal("legacy timing setting should remain readable")
	}
	t.Setenv("VIEWER_EXTRACTION_TIMING_LOG", "0")
	if extractionTimingLogEnabled() {
		t.Fatal("new timing setting should take precedence")
	}
}

func TestExtractionOpenRouterResponseFormatRequiresStrictTerms(t *testing.T) {
	format := ExtractionOpenRouterResponseFormat()
	jsonSchema := format["json_schema"].(map[string]any)
	schema := jsonSchema["schema"].(map[string]any)
	required := schema["required"].([]any)
	foundTerms := false
	for _, value := range required {
		if value == "terms" {
			foundTerms = true
		}
	}
	if !foundTerms {
		t.Fatalf("root response schema must require terms: %+v", required)
	}
	properties := schema["properties"].(map[string]any)
	termItems := properties["terms"].(map[string]any)["items"].(map[string]any)
	termProperties := termItems["properties"].(map[string]any)
	reading := termProperties["reading"].(map[string]any)
	if len(reading["anyOf"].([]any)) != 2 {
		t.Fatalf("term reading must support an explicit null: %+v", reading)
	}
	descriptions := termProperties["descriptionHistory"].(map[string]any)
	if descriptions["minItems"] != 1 {
		t.Fatalf("term descriptionHistory must require at least one snapshot: %+v", descriptions)
	}
	category := termProperties["category"].(map[string]any)
	categoryValue := category["properties"].(map[string]any)["value"].(map[string]any)
	if values := categoryValue["enum"].([]any); len(values) != 7 || values[6] != "other" {
		t.Fatalf("term category enum is incomplete: %+v", values)
	}
}

func TestRuntimeGeneratedStateHelpers(t *testing.T) {
	stateDir := t.TempDir()
	runtime := NewRuntime(RuntimeDependencies{StateDir: stateDir})
	if path := runtime.CheckpointPath("novel-1", "2"); path == "" {
		t.Fatal("CheckpointPath should return a path")
	}

	if err := characters.SaveGeneratedSummaryWithEpisodes(stateDir, "novel-1", "3", []characters.GeneratedCharacter{{
		CanonicalName:               "アリス",
		CanonicalEpisodeIndex:       "1",
		FirstAppearanceEpisodeIndex: "1",
	}}, []characters.HeuristicEpisode{{EpisodeIndex: "1", Text: "アリスがいた。"}}); err != nil {
		t.Fatalf("SaveGeneratedSummaryWithEpisodes returned error: %v", err)
	}
	seed, processed, ok, err := runtime.LoadGeneratedCharactersBeforeEpisode("novel-1", "2")
	if err != nil || !ok || processed == nil || len(seed) != 1 {
		t.Fatalf("LoadGeneratedCharactersBeforeEpisode seed=%+v processed=%v ok=%v err=%v", seed, processed, ok, err)
	}

	pending := filterGeneratedUnresolvedMentionsBeforeEpisode([]characters.GeneratedUnresolvedMention{
		{Mention: "前", EpisodeIndex: "1"},
		{Mention: "後", EpisodeIndex: "3"},
		{Mention: "空"},
	}, "2")
	if len(pending) != 1 || pending[0].Mention != "前" {
		t.Fatalf("filtered pending = %+v", pending)
	}

	earliest := earliestGeneratedEpisodeDigest([]characters.GeneratedEpisodeDigest{
		{EpisodeIndex: "5"},
		{EpisodeIndex: "2"},
		{EpisodeIndex: "9"},
	}, "5")
	if earliest != "2" {
		t.Fatalf("earliestGeneratedEpisodeDigest = %q", earliest)
	}
}
