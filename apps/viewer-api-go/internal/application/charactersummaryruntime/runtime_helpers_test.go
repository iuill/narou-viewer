package charactersummaryruntime

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
