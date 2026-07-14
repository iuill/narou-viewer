package httpapi

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"narou-viewer/apps/viewer-api-go/internal/characters"
	"narou-viewer/apps/viewer-api-go/internal/library"
)

func TestExtractionEngineBuildsPromptPreviewChunks(t *testing.T) {
	t.Setenv("EXTRACTION_MAX_CHUNK_CHARS", "18")
	t.Setenv("EXTRACTION_MAX_BATCH_CHARS", "35")
	maxChunkChars, maxBatchChars := extractionLimits()
	alt := "挿絵の人物"
	title := "人物画"
	episodes := []extractionEpisodeInput{
		{
			EpisodeIndex: "1",
			Title:        "第一話",
			ReaderDocument: library.ReaderDocument{Blocks: []library.ReaderBlock{
				{Type: "paragraph", Inlines: []library.ReaderInline{
					{Type: "ruby", Text: "剣城", Ruby: "ツルギ"},
					{Type: "ruby", Text: "瑛人", Ruby: "エイト"},
					{Type: "text", Text: "は出発した。アリスは見送った。"},
					{Type: "lineBreak"},
					{Type: "link", Children: []library.ReaderInline{{Type: "text", Text: "リンク文"}}},
				}},
				{Type: "html", HTML: "<script>x</script><p>ボブは&nbsp;合流した。</p>"},
				{Type: "image", Alt: &alt, Title: &title},
			}},
		},
		{
			EpisodeIndex:   "2",
			Title:          "第二話",
			HTML:           "<style>x</style><p>HTML fallback &amp; text。</p>",
			ReaderDocument: library.ReaderDocument{Blocks: []library.ReaderBlock{}},
		},
	}

	text := extractExtractionEpisodeText(episodes[0])
	for _, phrase := range []string{"剣城瑛人は出発した。", "ボブは 合流した。", "挿絵の人物 人物画"} {
		if !strings.Contains(text, phrase) {
			t.Fatalf("extracted text should contain %q: %q", phrase, text)
		}
	}
	if fallback := extractExtractionEpisodeText(episodes[1]); fallback != "HTML fallback & text。" {
		t.Fatalf("unexpected HTML fallback text: %q", fallback)
	}

	chunks := createExtractionChunks(episodes, maxChunkChars)
	if len(chunks) < 3 || chunks[0].ChunkCount < 2 {
		t.Fatalf("expected sentence-aware chunks, got %+v", chunks)
	}
	batches := createExtractionBatches(chunks, maxBatchChars)
	if len(batches) < 2 || batches[0].BatchIndex != 1 || batches[0].BatchCount != len(batches) {
		t.Fatalf("expected multiple indexed batches, got %+v", batches)
	}
	systemPrompt, userPrompt := buildExtractionPrompt("novel-1", "2", nil, batches[0], nil)
	if !strings.Contains(systemPrompt, "本文に明示された事実だけ") || !strings.Contains(systemPrompt, "candidateCharacters") {
		t.Fatalf("default system prompt lost required guidance: %q", systemPrompt)
	}
	var promptPayload map[string]any
	if err := json.Unmarshal([]byte(userPrompt), &promptPayload); err != nil {
		t.Fatalf("prompt should be JSON: %v", err)
	}
	candidates, ok := promptPayload["candidateCharacters"].([]any)
	if !ok || len(candidates) != 0 {
		t.Fatalf("empty candidate characters should be an empty array: %+v", promptPayload)
	}
	if renderSummaryBlock(library.ReaderBlock{Type: "title", Text: "題名"}) != "題名" ||
		renderSummaryBlock(library.ReaderBlock{Type: "meta", Text: "メタ"}) != "メタ" ||
		renderSummaryBlock(library.ReaderBlock{Type: "unknown"}) != "" {
		t.Fatal("summary block rendering should preserve title/meta and ignore unknown blocks")
	}
	if chunks := createExtractionChunks([]extractionEpisodeInput{{EpisodeIndex: "3", Title: "空話"}}, maxChunkChars); len(chunks) != 1 || chunks[0].Text != "" {
		t.Fatalf("empty episode should still produce an empty chunk: %+v", chunks)
	}
}

func TestExtractionBatchesUseTokenBudget(t *testing.T) {
	chunks := []extractionChunk{
		{EpisodeIndex: "1", Title: "第一話", Text: "１２３４５６７８"},
		{EpisodeIndex: "2", Title: "第二話", Text: "１２３４５６７８"},
		{EpisodeIndex: "3", Title: "第三話", Text: "１２３４５６７８"},
	}

	batches := createExtractionBatchesWithBudget(chunks, extractionBatchBudget{MaxTextTokens: 90})
	if len(batches) != 2 || batches[0].BatchCount != 2 || len(batches[0].Chunks) != 2 || len(batches[1].Chunks) != 1 {
		t.Fatalf("token budget should pack chunks until the next chunk would exceed the budget: %+v", batches)
	}
	if got := strings.Join(batches[0].EpisodeIndexes, ","); got != "1,2" {
		t.Fatalf("first token-budgeted batch should keep episode indexes, got %s", got)
	}
	if extractionBudgetExceeded(100, 100, extractionBatchBudget{}) {
		t.Fatal("empty character summary budget should not split batches")
	}
}

func TestExtractionCandidateCardsRankAndLimitCompactCandidates(t *testing.T) {
	values := []characters.GeneratedCharacter{}
	for i := 0; i < 10; i++ {
		name := "人物" + string(rune('A'+i))
		aliases := []characters.GeneratedTextVersion{{Text: name, EpisodeIndex: "1"}}
		if i == 9 {
			aliases = []characters.GeneratedTextVersion{}
			for j := 0; j < 10; j++ {
				aliases = append(aliases, characters.GeneratedTextVersion{Text: "別名" + string(rune('A'+j)), EpisodeIndex: "1"})
			}
		}
		values = append(values, characters.GeneratedCharacter{
			CharacterID:                 "char_" + name,
			CanonicalName:               name,
			CanonicalEpisodeIndex:       "1",
			FirstAppearanceEpisodeIndex: "1",
			Aliases:                     aliases,
			SummaryHistory:              []characters.GeneratedHistoryVersion{{EpisodeIndex: "2", Text: name + "の概要"}},
		})
	}
	batch := extractionBatch{
		Chunks: []extractionChunk{{EpisodeIndex: "3", Title: "第三話", Text: "別名Jが再登場した。"}},
	}
	cards := extractionCandidateCards(values, batch)
	if len(cards) != 8 {
		t.Fatalf("candidate cards should be capped to 8 entries: %+v", cards)
	}
	if cards[0]["displayName"] != "人物J" {
		t.Fatalf("matched aliases should rank the candidate first: %+v", cards)
	}
	if aliases := cards[0]["aliases"].([]string); len(aliases) != 8 {
		t.Fatalf("candidate card aliases should stay compact: %+v", aliases)
	}
	if empty := latestGeneratedHistoryText(nil); empty != "" {
		t.Fatalf("empty latest history should be blank, got %q", empty)
	}
}

func TestExtractionCandidateCardsKeepAllExactMatches(t *testing.T) {
	values := []characters.GeneratedCharacter{}
	textParts := []string{}
	for i := 0; i < 10; i++ {
		name := "既存人物" + strconv.Itoa(i+1)
		values = append(values, characters.GeneratedCharacter{
			CharacterID:                 "char_" + strconv.Itoa(i+1),
			CanonicalName:               name,
			CanonicalEpisodeIndex:       "1",
			FirstAppearanceEpisodeIndex: "1",
			Aliases:                     []characters.GeneratedTextVersion{{Text: name, EpisodeIndex: "1"}},
		})
		textParts = append(textParts, name)
	}
	values = append(values, characters.GeneratedCharacter{
		CharacterID:                 "char_recent",
		CanonicalName:               "最近の人物",
		CanonicalEpisodeIndex:       "9",
		FirstAppearanceEpisodeIndex: "9",
	})
	batch := extractionBatch{
		Chunks: []extractionChunk{{EpisodeIndex: "10", Title: "会議", Text: strings.Join(textParts, "、") + "が集まった。"}},
	}
	cards := extractionCandidateCards(values, batch)
	if len(cards) != 10 {
		t.Fatalf("all exact name matches should be kept even beyond the recency cap: %+v", cards)
	}
	for index := 0; index < 10; index++ {
		if cards[index]["characterId"] == "char_recent" {
			t.Fatalf("unmatched recency candidate should not displace exact matches: %+v", cards)
		}
	}
}

func TestExtractionCandidateCardsDoNotForceSharedShortAliases(t *testing.T) {
	values := []characters.GeneratedCharacter{}
	for i := 0; i < 12; i++ {
		values = append(values, characters.GeneratedCharacter{
			CharacterID:                 "char_shared_" + strconv.Itoa(i),
			CanonicalName:               "人物" + strconv.Itoa(i),
			CanonicalEpisodeIndex:       strconv.Itoa(i + 1),
			FirstAppearanceEpisodeIndex: strconv.Itoa(i + 1),
			Aliases:                     []characters.GeneratedTextVersion{{Text: "王", EpisodeIndex: "1"}},
		})
	}
	values = append(values, characters.GeneratedCharacter{
		CharacterID:                 "char_matched",
		CanonicalName:               "リデル",
		CanonicalEpisodeIndex:       "1",
		FirstAppearanceEpisodeIndex: "1",
		Aliases: []characters.GeneratedTextVersion{
			{Text: "古い別名1", EpisodeIndex: "1"},
			{Text: "古い別名2", EpisodeIndex: "1"},
			{Text: "青い旅人", EpisodeIndex: "3"},
		},
	})
	batch := extractionBatch{
		Chunks: []extractionChunk{{EpisodeIndex: "20", Title: "謁見", Text: "王が集まり、青い旅人が名乗った。"}},
	}
	cards := extractionCandidateCards(values, batch)
	if len(cards) != 8 {
		t.Fatalf("shared short aliases should stay within recency candidate budget: %+v", cards)
	}
	if cards[0]["characterId"] != "char_matched" {
		t.Fatalf("unique matched alias should still rank first: %+v", cards)
	}
	aliases := cards[0]["aliases"].([]string)
	if len(aliases) == 0 || aliases[0] != "青い旅人" {
		t.Fatalf("matched alias should be included in the candidate card aliases: %+v", aliases)
	}
}

func TestExtractionInputsFilterFromEpisode(t *testing.T) {
	inputs := extractionInputs{
		Episodes: []characters.HeuristicEpisode{
			{EpisodeIndex: "1", Text: "一話"},
			{EpisodeIndex: "2", Text: "二話"},
			{EpisodeIndex: "3", Text: "三話"},
		},
		Batches: []extractionBatch{{
			BatchIndex:     1,
			BatchCount:     1,
			EpisodeIndexes: []string{"1", "2", "3"},
			Chunks: []extractionChunk{
				{EpisodeIndex: "1", Text: "一話"},
				{EpisodeIndex: "2", Text: "二話"},
				{EpisodeIndex: "3", Text: "三話"},
			},
		}},
	}
	filtered := filterExtractionInputsFrom(inputs, "2")
	if len(filtered.Episodes) != 2 || filtered.Episodes[0].EpisodeIndex != "2" || len(filtered.Batches) != 1 || len(filtered.Batches[0].Chunks) != 2 {
		t.Fatalf("inputs should keep episodes from the reprocess boundary: %+v", filtered)
	}
	if blank := filterExtractionInputsFrom(inputs, " "); len(blank.Episodes) != 3 {
		t.Fatalf("blank reprocess boundary should keep all inputs: %+v", blank)
	}
	if got := earliestGeneratedEpisodeDigest([]characters.GeneratedEpisodeDigest{{EpisodeIndex: "3"}, {EpisodeIndex: "1"}, {EpisodeIndex: "9"}}, "5"); got != "1" {
		t.Fatalf("earliest digest should pick the earliest processed episode, got %q", got)
	}
}

func TestExtractionPromptIncludesUnresolvedMentions(t *testing.T) {
	batch := extractionBatch{
		EpisodeIndexes: []string{"20"},
		Chunks:         []extractionChunk{{EpisodeIndex: "20", Title: "正体", Text: "アリスが現れた。"}},
	}
	_, userPrompt := buildExtractionPromptWithUnresolved("novel-1", "20", nil, batch, []characters.GeneratedUnresolvedMention{
		{Mention: "黒衣の男", EpisodeIndex: "10", Reason: "正体不明", CandidateIDs: []string{"char_a"}},
		{Mention: " ", EpisodeIndex: "11"},
	}, nil)
	if !strings.Contains(userPrompt, `"unresolvedMentions"`) || !strings.Contains(userPrompt, `"黒衣の男"`) || !strings.Contains(userPrompt, `"candidateIds"`) {
		t.Fatalf("prompt should re-inject persisted unresolved mentions: %s", userPrompt)
	}
	merged := mergeGeneratedUnresolvedMentions([]characters.GeneratedUnresolvedMention{{Mention: "黒衣の男", EpisodeIndex: "10"}}, []extractionUnresolvedMention{
		{Mention: "黒衣の男", EpisodeIndex: "10", Reason: "重複"},
		{Mention: "白い影", EpisodeIndex: "12", Reason: "候補なし"},
	})
	if len(merged) != 2 || merged[1].Mention != "白い影" {
		t.Fatalf("unresolved mention merge should dedupe and append new values: %+v", merged)
	}
	filtered := filterResolvedGeneratedUnresolvedMentions(merged, []characters.GeneratedCharacter{{
		CharacterID:   "char_shadow",
		CanonicalName: "白い影",
		Aliases:       []characters.GeneratedTextVersion{{Text: "白い影", EpisodeIndex: "12"}},
	}})
	if len(filtered) != 1 || filtered[0].Mention != "黒衣の男" {
		t.Fatalf("resolved unresolved mentions should be removed from the active set: %+v", filtered)
	}
	ambiguous := filterResolvedGeneratedUnresolvedMentions([]characters.GeneratedUnresolvedMention{{Mention: "先生", EpisodeIndex: "20", CandidateIDs: []string{"char_a", "char_b"}}}, []characters.GeneratedCharacter{
		{CharacterID: "char_a", CanonicalName: "佐藤先生", Aliases: []characters.GeneratedTextVersion{{Text: "先生", EpisodeIndex: "1"}}},
		{CharacterID: "char_b", CanonicalName: "田中先生", Aliases: []characters.GeneratedTextVersion{{Text: "先生", EpisodeIndex: "1"}}},
	})
	if len(ambiguous) != 1 || ambiguous[0].Mention != "先生" {
		t.Fatalf("ambiguous unresolved mention should stay active: %+v", ambiguous)
	}
	resolvedAfterMerge := filterResolvedGeneratedUnresolvedMentions([]characters.GeneratedUnresolvedMention{{Mention: "先生", EpisodeIndex: "20", CandidateIDs: []string{"char_a"}}}, []characters.GeneratedCharacter{
		{CharacterID: "char_a", CanonicalName: "佐藤先生", Aliases: []characters.GeneratedTextVersion{{Text: "先生", EpisodeIndex: "1"}}},
	})
	if len(resolvedAfterMerge) != 0 {
		t.Fatalf("unresolved mention should be removed only after it resolves to a unique candidate: %+v", resolvedAfterMerge)
	}
	stateDir := t.TempDir()
	server := &Server{dataDir: stateDir}
	if err := characters.SaveGeneratedSummaryWithEpisodes(server.stateDir(), "novel-unresolved-cutoff", "12", nil, nil, []characters.GeneratedUnresolvedMention{
		{Mention: "残る影", EpisodeIndex: "10"},
		{Mention: "消える影", EpisodeIndex: "12"},
	}); err != nil {
		t.Fatalf("SaveGeneratedSummaryWithEpisodes unresolved returned error: %v", err)
	}
	pending, err := server.loadExtractionPendingUnresolved("novel-unresolved-cutoff", "12")
	if err != nil || len(pending) != 1 || pending[0].Mention != "残る影" {
		t.Fatalf("pending unresolved should be truncated before reprocess cutoff: pending=%+v err=%v", pending, err)
	}
	allPending, err := server.loadExtractionPendingUnresolved("novel-unresolved-cutoff", "")
	if err != nil || len(allPending) != 2 {
		t.Fatalf("blank reprocess cutoff should keep all pending unresolved mentions: pending=%+v err=%v", allPending, err)
	}
}

func TestExtractionMergeProposalRetiresMergedGeneratedID(t *testing.T) {
	allocator := characters.NewGeneratedCharacterIDAllocator("novel-merge-run", nil)
	generated, _ := applyExtractionDelta("novel-merge-run", nil, extractionDelta{
		NewCharacters: []characters.GeneratedCharacter{
			{CanonicalName: "アリス", CanonicalEpisodeIndex: "1", FirstAppearanceEpisodeIndex: "1"},
			{CanonicalName: "謎の少女", CanonicalEpisodeIndex: "2", FirstAppearanceEpisodeIndex: "2"},
		},
	}, allocator)
	if len(generated) != 2 || generated[0].CharacterID == "" || generated[1].CharacterID == "" {
		t.Fatalf("new characters should receive stable ids: %+v", generated)
	}
	sourceID := generated[1].CharacterID
	targetID := generated[0].CharacterID
	generated, _ = applyExtractionDelta("novel-merge-run", generated, extractionDelta{
		MergeProposals: []extractionMergeProposal{{
			SourceCharacterID: sourceID,
			TargetCharacterID: targetID,
			Confidence:        extractionMergeAutoApplyConfidence,
		}},
	}, allocator)
	state := extractionStateFromAllocator(nil, allocator)
	if len(generated) != 1 || generated[0].CharacterID != targetID {
		t.Fatalf("merge proposal should leave only the representative character: %+v", generated)
	}
	if len(state.RetiredCharacterIDs) != 1 || state.RetiredCharacterIDs[0].CharacterID != sourceID || state.RetiredCharacterIDs[0].MergedInto != targetID {
		t.Fatalf("merged generated id should be retained as retired state: %+v", state.RetiredCharacterIDs)
	}
}

func TestGeneratedExtractionPreviewKeepsExistingEventMentions(t *testing.T) {
	stateDir := t.TempDir()
	if err := characters.SaveGeneratedSummaryWithEpisodes(stateDir, "novel-preview", "2", []characters.GeneratedCharacter{{
		CharacterID:                 "char_alice",
		CanonicalName:               "アリス",
		CanonicalEpisodeIndex:       "1",
		FirstAppearanceEpisodeIndex: "1",
		Aliases:                     []characters.GeneratedTextVersion{{Text: "アリス", EpisodeIndex: "1"}},
	}}, []characters.HeuristicEpisode{
		{EpisodeIndex: "1", Text: "アリスは走った。"},
		{EpisodeIndex: "2", Text: "アリスは笑った。"},
	}); err != nil {
		t.Fatalf("seed SaveGeneratedSummaryWithEpisodes returned error: %v", err)
	}
	summary, err := buildGeneratedExtractionPreview(stateDir, "novel-preview", "3", []characters.GeneratedCharacter{{
		CharacterID:                 "char_alice",
		CanonicalName:               "アリス",
		CanonicalEpisodeIndex:       "1",
		FirstAppearanceEpisodeIndex: "1",
		Aliases:                     []characters.GeneratedTextVersion{{Text: "アリス", EpisodeIndex: "1"}},
	}}, []characters.HeuristicEpisode{
		{EpisodeIndex: "3", Text: "アリスは扉を開けた。"},
	}, []string{"1", "2", "3"}, characters.SaveGeneratedSummaryOptions{})
	if err != nil {
		t.Fatalf("buildGeneratedExtractionPreview returned error: %v", err)
	}
	if len(summary.Characters) != 1 {
		t.Fatalf("preview should include the existing character: %+v", summary)
	}
	importance, ok := summary.Characters[0].Importance.(map[string]any)
	if !ok || importance["category"] != "main" {
		t.Fatalf("preview should classify using previous and new mentions: %+v", summary.Characters[0].Importance)
	}
}

func TestExtractionCandidateCardsUseFullNameHistory(t *testing.T) {
	oldFullName := "ラビット家の令嬢"
	currentFullName := "アリス・リデル"
	cards := extractionCandidateCards([]characters.GeneratedCharacter{
		{
			CharacterID:                 "char_alice",
			CanonicalName:               "アリス",
			CanonicalEpisodeIndex:       "20",
			FullName:                    &currentFullName,
			FullNameEpisodeIndex:        "20",
			FullNameHistory:             []characters.GeneratedTextVersion{{Text: oldFullName, EpisodeIndex: "5"}, {Text: currentFullName, EpisodeIndex: "20"}},
			FirstAppearanceEpisodeIndex: "1",
		},
	}, extractionBatch{Chunks: []extractionChunk{{EpisodeIndex: "25", Text: "ラビット家の令嬢は古い記録にだけ残っていた。"}}})
	if len(cards) != 1 {
		t.Fatalf("fullNameHistory match should include the candidate card: %+v", cards)
	}
	aliases, _ := cards[0]["aliases"].([]string)
	found := false
	for _, alias := range aliases {
		if alias == oldFullName {
			found = true
		}
	}
	if !found {
		t.Fatalf("matched fullNameHistory value should be exposed in the candidate card aliases: %+v", cards[0])
	}
}

func TestReuseGeneratedCharacterIDsFromRegistryKeepsStableIDForReprocessedIdentity(t *testing.T) {
	generated, state := reuseGeneratedCharacterIDsFromRegistry([]characters.GeneratedCharacter{{
		CharacterID:                 "char_new",
		CanonicalName:               "アリス",
		CanonicalEpisodeIndex:       "5",
		FirstAppearanceEpisodeIndex: "5",
	}}, []characters.GeneratedCharacter{
		{
			CharacterID:                 "char_old",
			CanonicalName:               "アリス",
			CanonicalEpisodeIndex:       "5",
			FirstAppearanceEpisodeIndex: "5",
		},
		{
			CharacterID:                 "char_future",
			CanonicalName:               "未来の人物",
			CanonicalEpisodeIndex:       "10",
			FirstAppearanceEpisodeIndex: "10",
		},
	}, extractionGenerationState{IssuedCharacterIDs: []string{"char_new"}}, "5")
	if len(generated) != 1 || generated[0].CharacterID != "char_old" {
		t.Fatalf("reprocessed matching identity should reuse the previous stable id: %+v", generated)
	}
	if len(state.RetiredCharacterIDs) != 1 || state.RetiredCharacterIDs[0].CharacterID != "char_new" || state.RetiredCharacterIDs[0].MergedInto != "char_old" {
		t.Fatalf("newly issued id should be retired into the previous id: %+v", state.RetiredCharacterIDs)
	}
	ambiguous, ambiguousState := reuseGeneratedCharacterIDsFromRegistry([]characters.GeneratedCharacter{{
		CharacterID:   "char_teacher_new",
		CanonicalName: "先生",
	}}, []characters.GeneratedCharacter{
		{CharacterID: "char_teacher_a", CanonicalName: "先生", FirstAppearanceEpisodeIndex: "1"},
		{CharacterID: "char_teacher_b", CanonicalName: "先生", FirstAppearanceEpisodeIndex: "2"},
	}, extractionGenerationState{}, "5")
	if ambiguous[0].CharacterID != "char_teacher_new" || len(ambiguousState.RetiredCharacterIDs) != 0 {
		t.Fatalf("ambiguous identity keys should not remap IDs: generated=%+v state=%+v", ambiguous, ambiguousState)
	}
	duplicated, duplicateState := reuseGeneratedCharacterIDsFromRegistry([]characters.GeneratedCharacter{
		{
			CharacterID:                 "char_old",
			CanonicalName:               "アリス",
			CanonicalEpisodeIndex:       "1",
			FirstAppearanceEpisodeIndex: "1",
		},
		{
			CharacterID:                 "char_new_duplicate",
			CanonicalName:               "アリス",
			CanonicalEpisodeIndex:       "5",
			FirstAppearanceEpisodeIndex: "5",
			SummaryHistory:              []characters.GeneratedHistoryVersion{{EpisodeIndex: "5", Text: "再登場した。"}},
		},
	}, []characters.GeneratedCharacter{{CharacterID: "char_old", CanonicalName: "アリス", FirstAppearanceEpisodeIndex: "1"}}, extractionGenerationState{}, "5")
	if len(duplicated) != 2 || duplicated[0].CharacterID != "char_old" || duplicated[1].CharacterID != "char_new_duplicate" || len(duplicateState.RetiredCharacterIDs) != 0 {
		t.Fatalf("multiple generated identities must not collapse into one registry ID: generated=%+v state=%+v", duplicated, duplicateState)
	}
}

func TestExtractionHistoryHelpers(t *testing.T) {
	if got := summarizeGeneratedHistory(nil); got != "なし" {
		t.Fatalf("empty generated history should be rendered as none, got %q", got)
	}
	history := []characters.GeneratedHistoryVersion{
		{EpisodeIndex: "10", Text: "後"},
		{EpisodeIndex: "2", Text: "先"},
		{EpisodeIndex: "2", Text: "同話別"},
	}
	got := summarizeGeneratedHistory(history)
	if got != "第2話: 先 / 第2話: 同話別 / 第10話: 後" {
		t.Fatalf("generated history should be sorted by episode and text, got %q", got)
	}
	if got := firstNonEmptySummaryString(" ", " 値 ", "後"); got != "値" {
		t.Fatalf("firstNonEmptySummaryString should trim first value, got %q", got)
	}
	if got := firstNonEmptySummaryString(" ", ""); got != "" {
		t.Fatalf("firstNonEmptySummaryString should return empty fallback, got %q", got)
	}
}

func TestExtractionMergeProposalsAreOrderIndependent(t *testing.T) {
	existing := []characters.GeneratedCharacter{
		{CharacterID: "char_a", CanonicalName: "A", CanonicalEpisodeIndex: "1", FirstAppearanceEpisodeIndex: "1"},
		{CharacterID: "char_b", CanonicalName: "B", CanonicalEpisodeIndex: "2", FirstAppearanceEpisodeIndex: "2"},
		{CharacterID: "char_c", CanonicalName: "C", CanonicalEpisodeIndex: "3", FirstAppearanceEpisodeIndex: "3"},
	}
	merged, changed := applyExtractionDelta("novel-1", existing, extractionDelta{
		MergeProposals: []extractionMergeProposal{
			{SourceCharacterID: "char_b", TargetCharacterID: "char_c", Confidence: 1, Reason: "同一"},
			{SourceCharacterID: "char_a", TargetCharacterID: "char_b", Confidence: 1, Reason: "同一"},
		},
	}, nil)
	if changed != 2 || len(merged) != 1 || merged[0].CharacterID != "char_a" {
		t.Fatalf("merge proposals should be applied by connected component, changed=%d merged=%+v", changed, merged)
	}
	unchanged, changed := applyExtractionDelta("novel-1", existing, extractionDelta{
		MergeProposals: []extractionMergeProposal{{SourceCharacterID: "char_b", TargetCharacterID: "char_c", Confidence: 0.5, Reason: "低信頼"}},
	}, nil)
	if changed != 0 || len(unchanged) != 3 {
		t.Fatalf("low-confidence merge proposals should not be applied automatically: changed=%d merged=%+v", changed, unchanged)
	}
}

func TestExtractionEngineNormalizesDeltaResponses(t *testing.T) {
	delta := []byte(`{
	  "processedUpToEpisodeIndex":"5",
	"newCharacters":[{
	    "canonicalName":{"text":" クレア ","episodeIndex":"5"},
	    "fullName":{"text":"クレア・ベル","episodeIndex":"5"},
	    "fullNameHistory":[{"text":"ベル","episodeIndex":"3"},{"text":"クレア・ベル","episodeIndex":"5"}],
	    "gender":{"text":"女性","episodeIndex":"5"},
	    "genderHistory":[{"text":"不明","episodeIndex":"3"},{"text":"女性","episodeIndex":"5"}],
	    "firstAppearanceEpisodeIndex":"5",
	    "aliases":[{"text":"クレア","episodeIndex":"5"}],
	    "appearanceHistory":[],
	    "personalityHistory":[],
	    "summaryHistory":[{"episodeIndex":"5","text":"新たに登場した。"}]
	  }],
	  "characterUpdates":[{
	    "characterId":"char_existing",
	    "canonicalName":null,
	    "fullName":{"text":"アリス・スミス","episodeIndex":"5"},
	    "fullNameHistory":[{"text":"アリス旧姓","episodeIndex":"2"},{"text":"アリス・スミス","episodeIndex":"5"}],
	    "gender":null,
	    "genderHistory":[],
	    "firstAppearanceEpisodeIndex":"1",
	    "aliases":[{"text":"アリス","episodeIndex":"5"}],
	    "appearanceHistory":[{"episodeIndex":"5","text":"外套を着ている。"}],
	    "personalityHistory":[],
	    "summaryHistory":[{"episodeIndex":"5","text":"一行を導く。"}]
	  }],
	  "mergeProposals":[{"sourceCharacterId":"char_dup","targetCharacterId":"char_existing","confidence":0.99,"reason":"本文で同一人物と明示"}],
	  "unresolvedMentions":[{"mention":"先生","episodeIndex":"5","reason":"複数候補あり"}],
	  "terms":[]
	}`)
	normalized, err := normalizeExtractionOpenRouterResponse(delta, "novel-1", "5")
	if err != nil {
		t.Fatalf("delta response should normalize: %v", err)
	}
	var foundNew bool
	var foundUpdate bool
	for _, item := range normalized.NewCharacters {
		if item.CanonicalName == "クレア" && len(item.FullNameHistory) == 2 && len(item.GenderHistory) == 2 {
			foundNew = true
		}
	}
	for _, item := range normalized.CharacterUpdates {
		if item.CharacterID == "char_existing" && item.FullName != nil && len(item.FullNameHistory) == 2 {
			foundUpdate = true
		}
	}
	if len(normalized.NewCharacters) != 1 || len(normalized.CharacterUpdates) != 1 || len(normalized.MergeProposals) != 1 || len(normalized.UnresolvedMentions) != 1 || !foundNew || !foundUpdate {
		t.Fatalf("unexpected delta normalization: %+v", normalized)
	}

	legacy := []byte(`{"characters":[{"canonicalName":"ボブ","summary":"騎士。","appearance":null,"personality":"忠実"}],"terms":[]}`)
	if _, err := normalizeExtractionOpenRouterResponse(legacy, "novel-1", "4"); err == nil {
		t.Fatal("legacy characters response should be rejected")
	}
}

func TestExtractionEngineMergesGeneratedCharacters(t *testing.T) {
	oldFullName := "アリス旧姓"
	fullName := "アリス・スミス"
	oldGender := "不明"
	gender := "女性"
	merged := mergeGeneratedCharacters(
		[]characters.GeneratedCharacter{{
			CanonicalName:               "アリス",
			CanonicalEpisodeIndex:       "2",
			FullName:                    &oldFullName,
			FullNameEpisodeIndex:        "2",
			FullNameHistory:             []characters.GeneratedTextVersion{{Text: oldFullName, EpisodeIndex: "2"}},
			Gender:                      &oldGender,
			GenderEpisodeIndex:          "2",
			GenderHistory:               []characters.GeneratedTextVersion{{Text: oldGender, EpisodeIndex: "2"}},
			FirstAppearanceEpisodeIndex: "2",
			Aliases:                     []characters.GeneratedTextVersion{{Text: "アリス", EpisodeIndex: "2"}},
			SummaryHistory:              []characters.GeneratedHistoryVersion{{EpisodeIndex: "2", Text: "旅の仲間。"}},
		}},
		[]characters.GeneratedCharacter{{
			CanonicalName:               "アリス",
			CanonicalEpisodeIndex:       "1",
			FullName:                    &fullName,
			FullNameEpisodeIndex:        "3",
			Gender:                      &gender,
			GenderEpisodeIndex:          "3",
			FirstAppearanceEpisodeIndex: "1",
			Aliases:                     []characters.GeneratedTextVersion{{Text: "アリス・スミス", EpisodeIndex: "3"}},
			SummaryHistory:              []characters.GeneratedHistoryVersion{{EpisodeIndex: "3", Text: "冷静に判断する。"}},
		}},
	)
	if len(merged) != 1 || merged[0].FirstAppearanceEpisodeIndex != "1" || merged[0].FullName == nil || len(merged[0].SummaryHistory) != 2 {
		t.Fatalf("identity merge should preserve earliest/latest facts: %+v", merged)
	}
	if len(merged[0].FullNameHistory) != 2 || len(merged[0].GenderHistory) != 2 || *merged[0].FullName != fullName || *merged[0].Gender != gender {
		t.Fatalf("identity merge should preserve fullName/gender histories and expose latest values: %+v", merged[0])
	}
	if id := generatedCharacterID("novel-1", "アリス"); !strings.HasPrefix(id, "char_") {
		t.Fatalf("character id should keep char_ prefix: %s", id)
	}
}

func TestExtractionDeltaAppliesByStableIDOnly(t *testing.T) {
	existing := []characters.GeneratedCharacter{
		{
			CharacterID:                 "char_a",
			CanonicalName:               "佐藤先生",
			CanonicalEpisodeIndex:       "1",
			FirstAppearanceEpisodeIndex: "1",
			Aliases:                     []characters.GeneratedTextVersion{{Text: "先生", EpisodeIndex: "1"}},
			SummaryHistory:              []characters.GeneratedHistoryVersion{{EpisodeIndex: "1", Text: "担任。"}},
		},
		{
			CharacterID:                 "char_b",
			CanonicalName:               "田中先生",
			CanonicalEpisodeIndex:       "1",
			FirstAppearanceEpisodeIndex: "1",
			Aliases:                     []characters.GeneratedTextVersion{{Text: "先生", EpisodeIndex: "1"}},
			SummaryHistory:              []characters.GeneratedHistoryVersion{{EpisodeIndex: "1", Text: "顧問。"}},
		},
	}
	updated, changed := applyExtractionDelta("novel-1", existing, extractionDelta{
		CharacterUpdates: []characters.GeneratedCharacter{{
			CharacterID:                 "char_b",
			CanonicalName:               "田中先生",
			CanonicalEpisodeIndex:       "2",
			FirstAppearanceEpisodeIndex: "1",
			Aliases:                     []characters.GeneratedTextVersion{{Text: "先生", EpisodeIndex: "2"}},
			SummaryHistory:              []characters.GeneratedHistoryVersion{{EpisodeIndex: "2", Text: "大会に同行する。"}},
		}},
		NewCharacters: []characters.GeneratedCharacter{{
			CanonicalName:               "山田先生",
			CanonicalEpisodeIndex:       "2",
			FirstAppearanceEpisodeIndex: "2",
			Aliases:                     []characters.GeneratedTextVersion{{Text: "先生", EpisodeIndex: "2"}},
		}},
	}, nil)
	if changed != 2 || len(updated) != 3 {
		t.Fatalf("same alias on different stable IDs should not auto-merge: changed=%d updated=%+v", changed, updated)
	}
	if updated[0].CharacterID == updated[1].CharacterID || updated[1].CharacterID == updated[2].CharacterID {
		t.Fatalf("all characters should keep separate ids: %+v", updated)
	}

	merged, changed := applyExtractionDelta("novel-1", updated, extractionDelta{
		MergeProposals: []extractionMergeProposal{{SourceCharacterID: "char_b", TargetCharacterID: "char_a", Confidence: 1, Reason: "本文で同一人物と明示"}},
	}, nil)
	if changed != 1 || len(merged) != 2 || generatedCharacterIndexByID(merged, "char_b") >= 0 || generatedCharacterIndexByID(merged, "char_a") < 0 {
		t.Fatalf("explicit merge proposal should merge only the referenced IDs: changed=%d merged=%+v", changed, merged)
	}
}

func TestExtractionDeltaCanonicalUpdateUsesNewerName(t *testing.T) {
	existing := []characters.GeneratedCharacter{{
		CharacterID:                 "char_mystery",
		CanonicalName:               "謎の少女",
		CanonicalEpisodeIndex:       "1",
		NameHistory:                 []characters.GeneratedTextVersion{{Text: "謎の少女", EpisodeIndex: "1"}},
		FirstAppearanceEpisodeIndex: "1",
		Aliases:                     []characters.GeneratedTextVersion{{Text: "謎の少女", EpisodeIndex: "1"}},
	}}
	updated, changed := applyExtractionDelta("novel-1", existing, extractionDelta{
		CharacterUpdates: []characters.GeneratedCharacter{{
			CharacterID:                 "char_mystery",
			CanonicalName:               "アリス",
			CanonicalEpisodeIndex:       "20",
			NameHistory:                 []characters.GeneratedTextVersion{{Text: "アリス", EpisodeIndex: "20"}},
			FirstAppearanceEpisodeIndex: "1",
			Aliases:                     []characters.GeneratedTextVersion{{Text: "アリス", EpisodeIndex: "20"}},
		}},
	}, nil)
	if changed != 1 || len(updated) != 1 || updated[0].CanonicalName != "アリス" || updated[0].CanonicalEpisodeIndex != "20" {
		t.Fatalf("newer canonical update should replace display name for future requests: changed=%d updated=%+v", changed, updated)
	}
	if len(updated[0].NameHistory) != 2 || len(updated[0].Aliases) != 2 {
		t.Fatalf("old and new canonical names should be retained as history/aliases: %+v", updated[0])
	}
}
