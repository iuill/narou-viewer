package charactersummary

import (
	"testing"

	"narou-viewer/apps/viewer-api-go/internal/characters"
	"narou-viewer/apps/viewer-api-go/internal/library"
)

func TestCharacterSummaryCoreHelpers(t *testing.T) {
	maxChunkChars, maxBatchChars := Limits()
	if maxChunkChars <= 0 || maxBatchChars <= 0 {
		t.Fatalf("limits = %d, %d", maxChunkChars, maxBatchChars)
	}
	episode := EpisodeInput{
		EpisodeIndex: "1",
		Title:        "第一話",
		HTML:         "<p>アリスが来た。</p>",
		ReaderDocument: library.ReaderDocument{Blocks: []library.ReaderBlock{{
			Type:    "paragraph",
			Inlines: []library.ReaderInline{{Type: "text", Text: "アリスが来た。"}},
		}}},
	}
	if got := ExtractEpisodeText(episode); got == "" {
		t.Fatalf("episode text should not be empty")
	}
	chunks := CreateChunks([]EpisodeInput{episode}, 100)
	chunks = append(chunks, CreateChunksFromText(episode, "ボブも来た。", 100)...)
	if len(chunks) == 0 {
		t.Fatalf("chunks = %+v", chunks)
	}
	batches := CreateBatches(chunks, 1000)
	if len(batches) == 0 {
		t.Fatalf("batches = %+v", batches)
	}
	budget := ResolveBatchBudget(1000, 4096, 512)
	if BudgetExceeded(1, 1, budget) {
		t.Fatalf("small input should fit budget %+v", budget)
	}
	if got := UniqueChunkEpisodeIndexes(chunks); len(got) != 1 || got[0] != "1" {
		t.Fatalf("unique indexes = %+v", got)
	}
	planned, remaining, err := PlanRuntimeBatch(batches[0], batches[0].Chunks, func(Batch) (bool, error) { return true, nil })
	if err != nil || len(planned.Chunks) == 0 || len(remaining) != 0 {
		t.Fatalf("planned=%+v remaining=%+v err=%v", planned, remaining, err)
	}
	runtimeBatches, err := PlanRuntimeBatches(batches[0], func(Batch) (bool, error) { return true, nil })
	if err != nil || len(runtimeBatches) != 1 {
		t.Fatalf("runtimeBatches=%+v err=%v", runtimeBatches, err)
	}
	if got := RuntimeBatch(batches[0], batches[0].Chunks); len(got.Chunks) != len(batches[0].Chunks) {
		t.Fatalf("runtime batch = %+v", got)
	}
	if split, err := SplitOversizedChunkBatch(batches[0], func(Batch) (bool, error) { return true, nil }); err != nil || len(split) == 0 {
		t.Fatalf("split=%+v err=%v", split, err)
	}
	if TokensFromChars(100) <= 0 {
		t.Fatalf("tokens from chars should be positive")
	}
	systemPrompt, userPrompt := BuildPrompt("novel-1", "1", nil, batches[0], nil)
	if systemPrompt == "" || userPrompt == "" {
		t.Fatalf("prompts should not be empty")
	}
	if got := ResolveSystemPrompt(nil); got == "" {
		t.Fatalf("system prompt should not be empty")
	}
	if got := RenderInlineTokens([]library.ReaderInline{{Type: "text", Text: "A"}, {Type: "lineBreak"}}); got != "A\n" {
		t.Fatalf("inline tokens = %q", got)
	}
	if got := RenderBlock(library.ReaderBlock{Type: "paragraph", Inlines: []library.ReaderInline{{Type: "text", Text: "段落"}}}); got != "段落" {
		t.Fatalf("block = %q", got)
	}
}

func TestCharacterSummaryMergeHelpers(t *testing.T) {
	fullName := "アリス・リデル"
	values := []characters.GeneratedCharacter{{
		CharacterID:                 "char_2",
		CanonicalName:               "ボブ",
		CanonicalEpisodeIndex:       "2",
		FirstAppearanceEpisodeIndex: "2",
	}, {
		CharacterID:                 "char_1",
		CanonicalName:               "アリス",
		CanonicalEpisodeIndex:       "1",
		FirstAppearanceEpisodeIndex: "1",
		FullName:                    &fullName,
		Aliases:                     []characters.GeneratedTextVersion{{Text: "リデル", EpisodeIndex: "1"}},
		SummaryHistory:              []characters.GeneratedHistoryVersion{{EpisodeIndex: "1", Text: "主人公。"}},
	}, {
		CharacterID:           "char_1",
		CanonicalName:         "アリス",
		CanonicalEpisodeIndex: "3",
		SummaryHistory:        []characters.GeneratedHistoryVersion{{EpisodeIndex: "3", Text: "不思議な部屋に入った。"}},
	}}
	if got := CandidateCards(values, Batch{BatchIndex: 1}); len(got) != 3 {
		t.Fatalf("candidate cards = %+v", got)
	}
	frequency := IdentityFrequency(values)
	if ExactCandidateKey("アリス", frequency) || !ExactCandidateKey("ボブ", frequency) {
		t.Fatalf("frequency = %+v", frequency)
	}
	if LatestGeneratedHistoryText(values[1].SummaryHistory) == "" || SummarizeGeneratedHistory(values[1].SummaryHistory) == "" {
		t.Fatalf("history helpers returned empty")
	}
	mergedByID := MergeGeneratedCharactersByID(values)
	if len(mergedByID) != 2 {
		t.Fatalf("mergedByID = %+v", mergedByID)
	}
	retired := NormalizeGeneratedRetiredCharacterIDs([]characters.GeneratedRetiredCharacterID{
		{CharacterID: " char_b ", MergedInto: "char_a"},
		{CharacterID: "char_b", MergedInto: "char_a"},
	})
	if len(retired) != 1 || retired[0].CharacterID != "char_b" {
		t.Fatalf("retired = %+v", retired)
	}

	allocator := characters.NewGeneratedCharacterIDAllocator("novel-1", nil)
	generated, changed := ApplyDelta("novel-1", nil, Delta{
		NewCharacters: []characters.GeneratedCharacter{{CanonicalName: "キャロル", CanonicalEpisodeIndex: "4"}},
	}, allocator)
	if changed == 0 || len(generated) != 1 || generated[0].CharacterID == "" {
		t.Fatalf("generated=%+v changed=%d", generated, changed)
	}
	generated, changed = ApplyMergeProposals(generated, []MergeProposal{{
		SourceCharacterID: generated[0].CharacterID,
		TargetCharacterID: generated[0].CharacterID,
		Confidence:        MergeAutoApplyConfidence,
	}}, changed, allocator)
	if len(generated) != 1 || changed == 0 {
		t.Fatalf("generated=%+v changed=%d", generated, changed)
	}
	if got := MergeRepresentativeID(values, map[string]int{"char_1": 1, "char_2": 0}, []string{"char_2", "char_1"}); got != "char_1" {
		t.Fatalf("representative = %q", got)
	}
	unresolved := MergeGeneratedUnresolvedMentions([]characters.GeneratedUnresolvedMention{{Mention: "黒衣の男", EpisodeIndex: "1"}}, []UnresolvedMention{{Mention: "少女", EpisodeIndex: "2"}})
	if len(unresolved) != 2 {
		t.Fatalf("unresolved = %+v", unresolved)
	}
	_ = FilterResolvedGeneratedUnresolvedMentions(unresolved, generated)
	if got := NormalizeSummaryStringList([]string{" B ", "A", "A", ""}); len(got) != 2 || got[0] != "A" {
		t.Fatalf("normalized strings = %+v", got)
	}
	if got := GeneratedCharacterIndexByID(values, "char_1"); got != 1 {
		t.Fatalf("index = %d", got)
	}
	if got := GeneratedIdentityKeys(values[1]); len(got) == 0 {
		t.Fatalf("identity keys = %+v", got)
	}
	merged := MergeGeneratedCharacter(values[1], values[2])
	if len(merged.SummaryHistory) != 2 {
		t.Fatalf("merged character = %+v", merged)
	}
	if got := FirstNonEmptyString("", "x"); got != "x" {
		t.Fatalf("first non-empty = %q", got)
	}
	if got := MergeGeneratedTextVersionLists(values[1].Aliases, []characters.GeneratedTextVersion{{Text: "アリス", EpisodeIndex: "2"}}); len(got) != 2 {
		t.Fatalf("merged text versions = %+v", got)
	}
	if got := MergeGeneratedHistoryVersionLists(values[1].SummaryHistory, values[2].SummaryHistory); len(got) != 2 {
		t.Fatalf("merged history versions = %+v", got)
	}
	SortGeneratedCharacters(values)
	if values[0].CharacterID != "char_1" {
		t.Fatalf("sorted = %+v", values)
	}
	if got := GeneratedCharacterID("novel-1", "アリス"); got == "" {
		t.Fatalf("generated id should not be empty")
	}
}
