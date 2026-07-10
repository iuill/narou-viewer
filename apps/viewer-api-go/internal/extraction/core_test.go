package extraction

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"narou-viewer/apps/viewer-api-go/internal/characters"
	"narou-viewer/apps/viewer-api-go/internal/library"
	"narou-viewer/apps/viewer-api-go/internal/terms"
)

func TestExtractionLimitsPreferNewEnvironmentAndFallbackToLegacy(t *testing.T) {
	t.Setenv("CHARACTER_SUMMARY_MAX_CHUNK_CHARS", "20")
	t.Setenv("CHARACTER_SUMMARY_MAX_BATCH_CHARS", "40")
	chunk, batch := Limits()
	if chunk != 20 || batch != 40 {
		t.Fatalf("legacy fallback limits = (%d, %d)", chunk, batch)
	}
	t.Setenv("EXTRACTION_MAX_CHUNK_CHARS", "30")
	t.Setenv("EXTRACTION_MAX_BATCH_CHARS", "60")
	chunk, batch = Limits()
	if chunk != 30 || batch != 60 {
		t.Fatalf("new extraction limits should win = (%d, %d)", chunk, batch)
	}
}

func TestExtractionEngineBuildsPromptPreviewChunks(t *testing.T) {
	t.Setenv("EXTRACTION_MAX_CHUNK_CHARS", "18")
	t.Setenv("EXTRACTION_MAX_BATCH_CHARS", "35")
	maxChunkChars, maxBatchChars := Limits()
	alt := "挿絵の人物"
	title := "人物画"
	episodes := []EpisodeInput{
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

	text := ExtractEpisodeText(episodes[0])
	for _, phrase := range []string{"剣城瑛人は出発した。", "ボブは 合流した。", "挿絵の人物 人物画"} {
		if !strings.Contains(text, phrase) {
			t.Fatalf("extracted text should contain %q: %q", phrase, text)
		}
	}
	if fallback := ExtractEpisodeText(episodes[1]); fallback != "HTML fallback & text。" {
		t.Fatalf("unexpected HTML fallback text: %q", fallback)
	}

	chunks := CreateChunks(episodes, maxChunkChars)
	if len(chunks) < 3 || chunks[0].ChunkCount < 2 {
		t.Fatalf("expected sentence-aware chunks, got %+v", chunks)
	}
	batches := CreateBatches(chunks, maxBatchChars)
	if len(batches) < 2 || batches[0].BatchIndex != 1 || batches[0].BatchCount != len(batches) {
		t.Fatalf("expected multiple indexed batches, got %+v", batches)
	}
	systemPrompt, userPrompt := BuildPrompt("novel-1", "2", nil, batches[0], nil)
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
	if RenderBlock(library.ReaderBlock{Type: "title", Text: "題名"}) != "題名" ||
		RenderBlock(library.ReaderBlock{Type: "meta", Text: "メタ"}) != "メタ" ||
		RenderBlock(library.ReaderBlock{Type: "unknown"}) != "" {
		t.Fatal("summary block rendering should preserve title/meta and ignore unknown blocks")
	}
	if chunks := CreateChunks([]EpisodeInput{{EpisodeIndex: "3", Title: "空話"}}, maxChunkChars); len(chunks) != 1 || chunks[0].Text != "" {
		t.Fatalf("empty episode should still produce an empty chunk: %+v", chunks)
	}
}

func TestExtractionBatchesUseTokenBudget(t *testing.T) {
	chunks := []Chunk{
		{EpisodeIndex: "1", Title: "第一話", Text: "１２３４５６７８"},
		{EpisodeIndex: "2", Title: "第二話", Text: "１２３４５６７８"},
		{EpisodeIndex: "3", Title: "第三話", Text: "１２３４５６７８"},
	}

	batches := CreateBatchesWithBudget(chunks, BatchBudget{MaxTextTokens: 90})
	if len(batches) != 2 || batches[0].BatchCount != 2 || len(batches[0].Chunks) != 2 || len(batches[1].Chunks) != 1 {
		t.Fatalf("token budget should pack chunks until the next chunk would exceed the budget: %+v", batches)
	}
	if got := strings.Join(batches[0].EpisodeIndexes, ","); got != "1,2" {
		t.Fatalf("first token-budgeted batch should keep episode indexes, got %s", got)
	}
	if BudgetExceeded(100, 100, BatchBudget{}) {
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
	batch := Batch{
		Chunks: []Chunk{{EpisodeIndex: "3", Title: "第三話", Text: "別名Jが再登場した。"}},
	}
	cards := CandidateCards(values, batch)
	if len(cards) != 8 {
		t.Fatalf("candidate cards should be capped to 8 entries: %+v", cards)
	}
	if cards[0]["displayName"] != "人物J" {
		t.Fatalf("matched aliases should rank the candidate first: %+v", cards)
	}
	if aliases := cards[0]["aliases"].([]string); len(aliases) != 8 {
		t.Fatalf("candidate card aliases should stay compact: %+v", aliases)
	}
	if empty := LatestGeneratedHistoryText(nil); empty != "" {
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
	batch := Batch{
		Chunks: []Chunk{{EpisodeIndex: "10", Title: "会議", Text: strings.Join(textParts, "、") + "が集まった。"}},
	}
	cards := CandidateCards(values, batch)
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
	batch := Batch{
		Chunks: []Chunk{{EpisodeIndex: "20", Title: "謁見", Text: "王が集まり、青い旅人が名乗った。"}},
	}
	cards := CandidateCards(values, batch)
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

func TestExtractionCandidateCardsUseFullNameHistory(t *testing.T) {
	oldFullName := "ラビット家の令嬢"
	currentFullName := "アリス・リデル"
	cards := CandidateCards([]characters.GeneratedCharacter{
		{
			CharacterID:                 "char_alice",
			CanonicalName:               "アリス",
			CanonicalEpisodeIndex:       "20",
			FullName:                    &currentFullName,
			FullNameEpisodeIndex:        "20",
			FullNameHistory:             []characters.GeneratedTextVersion{{Text: oldFullName, EpisodeIndex: "5"}, {Text: currentFullName, EpisodeIndex: "20"}},
			FirstAppearanceEpisodeIndex: "1",
		},
	}, Batch{Chunks: []Chunk{{EpisodeIndex: "25", Text: "ラビット家の令嬢は古い記録にだけ残っていた。"}}})
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

func TestExtractionHistoryHelpers(t *testing.T) {
	t.Setenv("CHARACTER_SUMMARY_TEST_INT", "bad")
	if got := PositiveEnvInt("CHARACTER_SUMMARY_TEST_INT", 7); got != 7 {
		t.Fatalf("invalid env int should use fallback, got %d", got)
	}
	t.Setenv("CHARACTER_SUMMARY_TEST_INT", "11")
	if got := PositiveEnvInt("CHARACTER_SUMMARY_TEST_INT", 7); got != 11 {
		t.Fatalf("positive env int should be used, got %d", got)
	}
	if EstimateTokenCount("") != 0 || EstimateTokenCount("abc") <= 0 {
		t.Fatal("token estimate should ignore blank text and count non-empty text")
	}
	if CompareEpisodeString("10", "2") <= 0 || CompareEpisodeString("same", "same") != 0 || CompareEpisodeString("alpha", "beta") >= 0 {
		t.Fatal("episode comparison should compare numeric values first and fallback to lexical order")
	}
	if CompareEpisodeString(" 2", "2") >= 0 {
		t.Fatal("episode comparison should preserve legacy whitespace-sensitive fallback behavior")
	}
	if got := SummarizeGeneratedHistory(nil); got != "なし" {
		t.Fatalf("empty generated history should be rendered as none, got %q", got)
	}
	history := []characters.GeneratedHistoryVersion{
		{EpisodeIndex: "10", Text: "後"},
		{EpisodeIndex: "2", Text: "先"},
		{EpisodeIndex: "2", Text: "同話別"},
	}
	got := SummarizeGeneratedHistory(history)
	if got != "第2話: 先 / 第2話: 同話別 / 第10話: 後" {
		t.Fatalf("generated history should be sorted by episode and text, got %q", got)
	}
	if got := FirstNonEmptyString(" ", " 値 ", "後"); got != "値" {
		t.Fatalf("firstNonEmptySummaryString should trim first value, got %q", got)
	}
	if got := FirstNonEmptyString(" ", ""); got != "" {
		t.Fatalf("firstNonEmptySummaryString should return empty fallback, got %q", got)
	}
	values := []characters.GeneratedCharacter{
		{CanonicalName: "後", FirstAppearanceEpisodeIndex: "10"},
		{CanonicalName: "先", FirstAppearanceEpisodeIndex: "2"},
		{CanonicalName: "同話B", FirstAppearanceEpisodeIndex: "2"},
		{CanonicalName: "同話A", FirstAppearanceEpisodeIndex: "2"},
	}
	SortGeneratedCharacters(values)
	if got := values[0].CanonicalName + "," + values[1].CanonicalName + "," + values[3].CanonicalName; got != "先,同話A,後" {
		t.Fatalf("generated characters should sort by episode then name, got %s", got)
	}
	prompt := BuildDefaultSystemPrompt()
	if !strings.Contains(prompt, "candidateCharacters が空") || !strings.Contains(prompt, "すべて newCharacters") || !strings.Contains(prompt, "役職・関係・説明だけの語") {
		t.Fatalf("default prompt should spell out initial extraction and alias constraints: %s", prompt)
	}
	_, userPrompt := BuildPrompt("novel-1", "1", nil, Batch{}, nil)
	var promptPayload map[string]any
	if err := json.Unmarshal([]byte(userPrompt), &promptPayload); err != nil {
		t.Fatalf("prompt payload should be JSON: %v", err)
	}
	if promptPayload["generationTask"] != "initialCharacterExtraction" || !strings.Contains(promptPayload["outputContract"].(string), "newCharacters") {
		t.Fatalf("initial prompt payload should explicitly request new characters: %+v", promptPayload)
	}
}

func TestExtractionEngineNormalizesRichAndLegacyResponses(t *testing.T) {
	delta := []byte(`{
	  "processedUpToEpisodeIndex":"5",
	"newCharacters":[{
	    "canonicalName":{"text":" クレア ","episodeIndex":"5"},
	    "fullName":{"text":"クレア・ベル","episodeIndex":"5"},
	    "fullNameHistory":[{"text":"ベル","episodeIndex":"3"},{"text":"クレア・ベル","episodeIndex":"5"}],
	    "gender":{"text":"女性","episodeIndex":"5"},
	    "genderHistory":[{"text":"不明","episodeIndex":"3"},{"text":"女性","episodeIndex":"5"}],
	    "firstAppearanceEpisodeIndex":"5",
	    "aliases":[{"text":"クレア","episodeIndex":"5"},{"text":"隊長","episodeIndex":"5"}],
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
	normalized, err := NormalizeOpenRouterResponse(delta, "novel-1", "5")
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
	if len(normalized.NewCharacters[0].Aliases) != 1 || normalized.NewCharacters[0].Aliases[0].Text != "クレア" {
		t.Fatalf("generic aliases should be filtered during normalization: %+v", normalized.NewCharacters[0].Aliases)
	}

	rich := []byte(`{
	  "processedUpToEpisodeIndex":"3",
	"characters":[{
	    "canonicalName":{"text":" アリス ","episodeIndex":"1"},
	    "fullName":{"text":"アリス・スミス","episodeIndex":"2"},
	    "fullNameHistory":[{"text":"アリス旧姓","episodeIndex":"1"},{"text":"アリス・スミス","episodeIndex":"2"}],
	    "gender":{"text":"女性","episodeIndex":"1"},
	    "genderHistory":[{"text":"女性","episodeIndex":"1"}],
	    "firstAppearanceEpisodeIndex":"1",
	    "aliases":[{"text":"アリス","episodeIndex":"1"},{"text":"アリス","episodeIndex":"1"}],
	    "appearanceHistory":[{"episodeIndex":"1","text":"銀髪"}],
	    "personalityHistory":[{"episodeIndex":"2","text":"冷静"}],
	    "summaryHistory":[{"episodeIndex":"3","text":"仲間。"}]
	  }],
	  "terms":[]
	}`)
	normalized, err = NormalizeOpenRouterResponse(rich, "novel-1", "3")
	if err != nil {
		t.Fatalf("rich response should normalize: %v", err)
	}
	if len(normalized.LegacyCharacters) != 1 || normalized.LegacyCharacters[0].CanonicalName != "アリス" || normalized.LegacyCharacters[0].FullName == nil || len(normalized.LegacyCharacters[0].FullNameHistory) != 2 || len(normalized.LegacyCharacters[0].Aliases) != 1 {
		t.Fatalf("unexpected rich normalization: %+v", normalized)
	}

	legacy := []byte(`{"characters":[{"canonicalName":"ボブ","summary":"騎士。","appearance":null,"personality":"忠実"}],"terms":[]}`)
	normalized, err = NormalizeOpenRouterResponse(legacy, "novel-1", "4")
	if err != nil {
		t.Fatalf("legacy response should normalize: %v", err)
	}
	if len(normalized.LegacyCharacters) != 1 || normalized.LegacyCharacters[0].CanonicalEpisodeIndex != "4" || len(normalized.LegacyCharacters[0].SummaryHistory) != 1 {
		t.Fatalf("unexpected legacy normalization: %+v", normalized)
	}
	if _, err := NormalizeOpenRouterResponse([]byte(`{"characters":{}}`), "novel-1", "4"); err == nil {
		t.Fatal("malformed characters payload should fail")
	}
	withInvalidMerge, err := NormalizeOpenRouterResponse([]byte(`{
		"processedUpToEpisodeIndex":"7",
		"newCharacters":[{"canonicalName":"有効人物"}],
		"characterUpdates":[],
		"mergeProposals":[
			{"sourceCharacterId":"char_b","targetCharacterId":"char_a","confidence":2,"reason":" clamp "},
			{"sourceCharacterId":"char_b","targetCharacterId":"char_a","confidence":0.5},
			{"sourceCharacterId":"char_a","targetCharacterId":"char_a","confidence":1},
			{"sourceCharacterId":"","targetCharacterId":"char_a","confidence":1}
		],
		"unresolvedMentions":[
			{"mention":" 謎の影 ","episodeIndex":"bad","reason":"候補なし"},
			{"mention":"","episodeIndex":"7"}
		],
		"terms":[]
	}`), "novel-1", "7")
	if err != nil {
		t.Fatalf("response with normalized merge proposal should parse: %v", err)
	}
	if len(withInvalidMerge.MergeProposals) != 1 || withInvalidMerge.MergeProposals[0].Confidence != 1 || withInvalidMerge.MergeProposals[0].Reason != "clamp" {
		t.Fatalf("merge proposals should be deduped and clamped: %+v", withInvalidMerge.MergeProposals)
	}
	if len(withInvalidMerge.UnresolvedMentions) != 1 || withInvalidMerge.UnresolvedMentions[0].EpisodeIndex != "7" {
		t.Fatalf("unresolved mentions should fall back to processed episode: %+v", withInvalidMerge.UnresolvedMentions)
	}
}

func TestExtractionTermsContractAndNormalization(t *testing.T) {
	for _, raw := range []string{
		`{"characters":[]}`,
		`{"characters":[],"terms":null}`,
		`{"characters":[],"terms":{}}`,
		`{"characters":[],"terms":[{"term":"聖剣","reading":42,"category":{"value":"item","episodeIndex":"1"},"descriptionHistory":[{"text":"剣。","episodeIndex":"1"}]}]}`,
	} {
		if _, err := NormalizeOpenRouterResponse([]byte(raw), "novel-1", "1"); err == nil {
			t.Fatalf("malformed terms payload should fail: %s", raw)
		}
	}
	if delta, err := NormalizeOpenRouterResponse([]byte(`{"characters":[],"terms":[]}`), "novel-1", "1"); err != nil || delta.Terms == nil || len(delta.Terms) != 0 {
		t.Fatalf("empty terms must be valid and normalized to an empty slice: delta=%+v err=%v", delta, err)
	}

	delta, err := NormalizeOpenRouterResponse([]byte(`{
		"processedUpToEpisodeIndex":"3",
		"characters":[],
		"terms":[{
			"term":" 聖剣 ",
			"reading":{"text":"せいけん","episodeIndex":"3"},
			"category":{"value":"unknown","episodeIndex":"3"},
			"descriptionHistory":[{"text":"王家に伝わる剣。","episodeIndex":"3"}]
		}]
	}`), "novel-1", "3")
	if err != nil || len(delta.Terms) != 1 {
		t.Fatalf("valid term should normalize: delta=%+v err=%v", delta, err)
	}
	term := delta.Terms[0]
	if term.Term != "聖剣" || term.ReadingHistory[0].Text != "せいけん" || term.CategoryHistory[0].Category != terms.CategoryOther {
		t.Fatalf("unexpected normalized term: %+v", term)
	}
}

func TestExtractionResponseRejectsEpisodeIndexesOutsideCurrentBatch(t *testing.T) {
	for label, raw := range map[string]string{
		"term": `{
			"processedUpToEpisodeIndex":"20",
			"characters":[],
			"terms":[{"term":"帝国評議会","reading":null,"category":{"value":"organization","episodeIndex":"20"},"descriptionHistory":[{"text":"正体。","episodeIndex":"1"}]}]
		}`,
		"character": `{
			"processedUpToEpisodeIndex":"20",
			"characters":[{"canonicalName":{"text":"黒騎士","episodeIndex":"1"},"firstAppearanceEpisodeIndex":"20","summaryHistory":[{"text":"人物。","episodeIndex":"20"}]}],
			"terms":[]
		}`,
		"unresolved": `{
			"processedUpToEpisodeIndex":"20",
			"characters":[],
			"unresolvedMentions":[{"mention":"謎の声","episodeIndex":"1"}],
			"terms":[]
		}`,
	} {
		t.Run(label, func(t *testing.T) {
			if _, err := NormalizeOpenRouterResponseForEpisodes([]byte(raw), "novel-1", "20", []string{"20"}); err == nil || !strings.Contains(err.Error(), "outside the current extraction batch") {
				t.Fatalf("out-of-batch episodeIndex should fail: %v", err)
			}
		})
	}
}

func TestValidateDeltaEpisodeIndexesAcceptsCompleteInBatchDelta(t *testing.T) {
	textVersions := []characters.GeneratedTextVersion{{Text: "情報", EpisodeIndex: "5"}}
	historyVersions := []characters.GeneratedHistoryVersion{{Text: "説明", EpisodeIndex: "5"}}
	character := characters.GeneratedCharacter{
		CanonicalEpisodeIndex:       "5",
		FirstAppearanceEpisodeIndex: "5",
		FullNameEpisodeIndex:        "5",
		GenderEpisodeIndex:          "5",
		NameHistory:                 textVersions,
		FullNameHistory:             textVersions,
		GenderHistory:               textVersions,
		Aliases:                     textVersions,
		AppearanceHistory:           historyVersions,
		PersonalityHistory:          historyVersions,
		SummaryHistory:              historyVersions,
	}
	delta := Delta{
		LegacyCharacters:   []characters.GeneratedCharacter{character},
		NewCharacters:      []characters.GeneratedCharacter{character},
		CharacterUpdates:   []characters.GeneratedCharacter{character},
		UnresolvedMentions: []UnresolvedMention{{Mention: "影", EpisodeIndex: "5"}},
		Terms: []terms.GeneratedTerm{{
			Term:               "王都",
			ReadingHistory:     []terms.TextVersion{{Text: "おうと", EpisodeIndex: "5"}},
			CategoryHistory:    []terms.CategoryVersion{{Category: "place", EpisodeIndex: "5"}},
			DescriptionHistory: []terms.HistoryVersion{{Text: "都。", EpisodeIndex: "5"}},
		}},
	}
	if err := ValidateDeltaEpisodeIndexes(delta, []string{"5"}); err != nil {
		t.Fatalf("complete in-batch delta should be accepted: %v", err)
	}
	if err := ValidateDeltaEpisodeIndexes(delta, nil); err != nil {
		t.Fatalf("empty allowlist should preserve compatibility: %v", err)
	}
}

func TestExtractionRubyTermCandidatesAndCharacterNameFiltering(t *testing.T) {
	if got := RenderExtractionInlineTokens([]library.ReaderInline{{Type: "ruby", Text: "聖剣", Ruby: "せいけん"}}); got != "聖剣《せいけん》" {
		t.Fatalf("ruby prompt rendering = %q", got)
	}

	known := make([]terms.GeneratedTerm, 0, 20)
	for index := 1; index <= 20; index++ {
		known = append(known, terms.GeneratedTerm{
			Term:               "用語" + strconv.Itoa(index),
			CategoryHistory:    []terms.CategoryVersion{{Category: terms.CategoryItem, EpisodeIndex: strconv.Itoa(index)}},
			DescriptionHistory: []terms.HistoryVersion{{Text: "説明", EpisodeIndex: strconv.Itoa(index)}},
		})
	}
	cards := TermCandidateCards(known, Batch{Chunks: []Chunk{{Text: "用語1が登場する。"}}})
	if len(cards) != 16 || cards[0]["term"] != "用語1" {
		t.Fatalf("known term cards should prioritize exact text matches and cap at 16: %+v", cards)
	}

	incoming := []terms.GeneratedTerm{
		{Term: "アリス", DescriptionHistory: []terms.HistoryVersion{{Text: "人物名", EpisodeIndex: "1"}}},
		{Term: "謎の少女", DescriptionHistory: []terms.HistoryVersion{{Text: "人物の旧称", EpisodeIndex: "1"}}},
		{Term: "聖剣", DescriptionHistory: []terms.HistoryVersion{{Text: "武器", EpisodeIndex: "1"}}},
	}
	filtered := FilterAndMergeTermDeltas(nil, incoming, []characters.GeneratedCharacter{{
		CanonicalName: "アリス",
		NameHistory:   []characters.GeneratedTextVersion{{Text: "謎の少女", EpisodeIndex: "1"}},
	}})
	if len(filtered) != 1 || filtered[0].Term != "聖剣" {
		t.Fatalf("character names must be removed from term deltas: %+v", filtered)
	}
	existing := []terms.GeneratedTerm{
		{Term: "黒騎士", DescriptionHistory: []terms.HistoryVersion{{Text: "正体不明の存在。", EpisodeIndex: "1"}}},
		{Term: "王都", DescriptionHistory: []terms.HistoryVersion{{Text: "王国の首都。", EpisodeIndex: "1"}}},
	}
	filtered = FilterAndMergeTermDeltas(existing, nil, []characters.GeneratedCharacter{{
		CanonicalName: "騎士団長",
		Aliases:       []characters.GeneratedTextVersion{{Text: "黒騎士", EpisodeIndex: "2"}},
	}})
	if len(filtered) != 1 || filtered[0].Term != "王都" {
		t.Fatalf("existing terms that later resolve to a character must be removed: %+v", filtered)
	}
	parallelFacts := []terms.GeneratedTerm{
		{Term: "騎士団長", DescriptionHistory: []terms.HistoryVersion{{Text: "人物。", EpisodeIndex: "2"}}},
		{Term: "王都", DescriptionHistory: []terms.HistoryVersion{{Text: "城壁がある。", EpisodeIndex: "2"}}},
	}
	filtered = FilterAndMergeParallelTermFacts(existing, parallelFacts, []characters.GeneratedCharacter{{
		CanonicalName: "騎士団長",
		Aliases:       []characters.GeneratedTextVersion{{Text: "黒騎士", EpisodeIndex: "2"}},
	}})
	if len(filtered) != 1 || filtered[0].Term != "王都" || filtered[0].DescriptionHistory[1].Text != "王国の首都。 城壁がある。" {
		t.Fatalf("parallel facts must fold cumulatively and exclude character names: %+v", filtered)
	}
}

func TestExtractionEngineMergesGeneratedCharacters(t *testing.T) {
	oldFullName := "アリス旧姓"
	fullName := "アリス・スミス"
	oldGender := "不明"
	gender := "女性"
	merged := MergeGeneratedCharacters(
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
	if id := GeneratedCharacterID("novel-1", "アリス"); !strings.HasPrefix(id, "char_") {
		t.Fatalf("character id should keep char_ prefix: %s", id)
	}
}

func TestExtractionPromptCompactsUnresolvedMentions(t *testing.T) {
	values := []characters.GeneratedUnresolvedMention{
		{Mention: " ", EpisodeIndex: "99"},
		{Mention: "古い影", EpisodeIndex: "1", Reason: "古い", CandidateIDs: []string{"char_old"}},
	}
	for index := 0; index < 40; index++ {
		values = append(values, characters.GeneratedUnresolvedMention{
			Mention:      "謎" + strconv.Itoa(index),
			EpisodeIndex: strconv.Itoa(index + 2),
			Reason:       "未確定",
			CandidateIDs: []string{"char_" + strconv.Itoa(index)},
		})
	}
	override := "system override"
	systemPrompt, userPrompt := BuildPromptWithUnresolved("novel-1", "42", nil, Batch{}, values, &override)
	if systemPrompt != override {
		t.Fatalf("system prompt override should be used, got %q", systemPrompt)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(userPrompt), &payload); err != nil {
		t.Fatalf("prompt should be JSON: %v", err)
	}
	unresolved, ok := payload["unresolvedMentions"].([]any)
	if !ok || len(unresolved) != 32 {
		t.Fatalf("unresolved mentions should be compacted to 32 entries: %+v", payload["unresolvedMentions"])
	}
	first, ok := unresolved[0].(map[string]any)
	if !ok || first["mention"] != "謎39" || first["episodeIndex"] != "41" || first["reason"] != "未確定" {
		t.Fatalf("unresolved mentions should be sorted newest first with metadata: %+v", first)
	}
	if _, ok := first["candidateIds"].([]any); !ok {
		t.Fatalf("candidate IDs should be included in unresolved prompt payload: %+v", first)
	}
}

func TestReuseGeneratedCharacterIDsFromRegistryKeepsStableIDForReprocessedIdentity(t *testing.T) {
	generated, state := ReuseGeneratedCharacterIDsFromRegistry([]characters.GeneratedCharacter{{
		CharacterID:                 "char_new",
		CanonicalName:               "アリス",
		CanonicalEpisodeIndex:       "5",
		FirstAppearanceEpisodeIndex: "5",
	}}, []characters.GeneratedCharacter{
		{CharacterID: "char_old", CanonicalName: "アリス", CanonicalEpisodeIndex: "5", FirstAppearanceEpisodeIndex: "5"},
		{CharacterID: "char_future", CanonicalName: "未来の人物", CanonicalEpisodeIndex: "10", FirstAppearanceEpisodeIndex: "10"},
	}, GenerationState{IssuedCharacterIDs: []string{"char_new"}}, "5")
	if len(generated) != 1 || generated[0].CharacterID != "char_old" {
		t.Fatalf("reprocessed matching identity should reuse the previous stable id: %+v", generated)
	}
	if len(state.RetiredCharacterIDs) != 1 || state.RetiredCharacterIDs[0].CharacterID != "char_new" || state.RetiredCharacterIDs[0].MergedInto != "char_old" {
		t.Fatalf("newly issued id should be retired into the previous id: %+v", state.RetiredCharacterIDs)
	}
	ambiguous, ambiguousState := ReuseGeneratedCharacterIDsFromRegistry([]characters.GeneratedCharacter{{
		CharacterID:   "char_teacher_new",
		CanonicalName: "先生",
	}}, []characters.GeneratedCharacter{
		{CharacterID: "char_teacher_a", CanonicalName: "先生", FirstAppearanceEpisodeIndex: "1"},
		{CharacterID: "char_teacher_b", CanonicalName: "先生", FirstAppearanceEpisodeIndex: "2"},
	}, GenerationState{}, "5")
	if ambiguous[0].CharacterID != "char_teacher_new" || len(ambiguousState.RetiredCharacterIDs) != 0 {
		t.Fatalf("ambiguous identity keys should not remap IDs: generated=%+v state=%+v", ambiguous, ambiguousState)
	}
	genericRegistry, genericState := ReuseGeneratedCharacterIDsFromRegistry([]characters.GeneratedCharacter{
		{CharacterID: "char_new_haru", CanonicalName: "先生", SummaryHistory: []characters.GeneratedHistoryVersion{{EpisodeIndex: "2", Text: "ハル・ミズノとして夜学で教える。"}}},
		{CharacterID: "char_new_nagi", CanonicalName: "先生", SummaryHistory: []characters.GeneratedHistoryVersion{{EpisodeIndex: "2", Text: "ナギ・シオンとして薬師をしている。"}}},
	}, []characters.GeneratedCharacter{
		{CharacterID: "char_old_teacher", CanonicalName: "先生", FirstAppearanceEpisodeIndex: "1"},
	}, GenerationState{}, "5")
	if len(genericRegistry) != 2 || genericRegistry[0].CharacterID == "char_old_teacher" || genericRegistry[1].CharacterID == "char_old_teacher" || len(genericState.RetiredCharacterIDs) != 0 {
		t.Fatalf("single generic registry identity should not remap generated characters: generated=%+v state=%+v", genericRegistry, genericState)
	}
}

func TestExtractionApplyDeltaMergesByStableIDOnly(t *testing.T) {
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
	updated, changed := ApplyDelta("novel-1", existing, Delta{
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

	merged, changed := ApplyDelta("novel-1", updated, Delta{
		MergeProposals: []MergeProposal{{SourceCharacterID: "char_b", TargetCharacterID: "char_a", Confidence: 1, Reason: "本文で同一人物と明示"}},
	}, nil)
	if changed != 1 || len(merged) != 2 || GeneratedCharacterIndexByID(merged, "char_b") >= 0 || GeneratedCharacterIndexByID(merged, "char_a") < 0 {
		t.Fatalf("explicit merge proposal should merge only the referenced IDs: changed=%d merged=%+v", changed, merged)
	}

	legacy, changed := ApplyDelta("novel-1", nil, Delta{
		LegacyCharacters: []characters.GeneratedCharacter{{CanonicalName: "レガシー", CanonicalEpisodeIndex: "1", FirstAppearanceEpisodeIndex: "1"}},
	}, nil)
	if changed != 1 || len(legacy) != 1 || legacy[0].CharacterID == "" {
		t.Fatalf("legacy delta should assign stable IDs and replace generated list: changed=%d legacy=%+v", changed, legacy)
	}
	allocator := characters.NewGeneratedCharacterIDAllocator("novel-allocator", nil)
	allocated, changed := ApplyDelta("novel-allocator", nil, Delta{
		NewCharacters: []characters.GeneratedCharacter{{CanonicalName: "採番対象", CanonicalEpisodeIndex: "1", FirstAppearanceEpisodeIndex: "1"}},
	}, allocator)
	if changed != 1 || len(allocated) != 1 || allocated[0].CharacterID == "" || allocator.NextCharacterOrdinal() <= 1 {
		t.Fatalf("allocator-backed delta should assign ordinal IDs: changed=%d allocated=%+v next=%d", changed, allocated, allocator.NextCharacterOrdinal())
	}
}

func TestExtractionApplyDeltaMergesDuplicateNewCharactersBeforeIDAllocation(t *testing.T) {
	allocator := characters.NewGeneratedCharacterIDAllocator("novel-duplicates", nil)
	updated, changed := ApplyDelta("novel-duplicates", nil, Delta{
		NewCharacters: []characters.GeneratedCharacter{
			{
				CanonicalName:               "セラ・クロウ",
				CanonicalEpisodeIndex:       "1",
				FirstAppearanceEpisodeIndex: "1",
				Aliases:                     []characters.GeneratedTextVersion{{Text: "セラ・クロウ", EpisodeIndex: "1"}},
			},
			{
				CanonicalName:               "セラ・クロウ",
				CanonicalEpisodeIndex:       "1",
				FirstAppearanceEpisodeIndex: "1",
				Aliases:                     []characters.GeneratedTextVersion{{Text: "クロ", EpisodeIndex: "2"}, {Text: "隊長", EpisodeIndex: "2"}},
			},
		},
	}, allocator)
	if changed != 1 || len(updated) != 1 || updated[0].CharacterID == "" {
		t.Fatalf("duplicate new characters should merge before stable IDs are assigned: changed=%d updated=%+v", changed, updated)
	}
	aliases := map[string]bool{}
	for _, alias := range updated[0].Aliases {
		aliases[alias.Text] = true
	}
	if !aliases["クロ"] || aliases["隊長"] {
		t.Fatalf("merged aliases should keep proper nicknames and drop role-only aliases: %+v", updated[0].Aliases)
	}
}

func TestExtractionApplyDeltaDoesNotMergeGenericNewCharactersBeforeIDAllocation(t *testing.T) {
	updated, changed := ApplyDelta("novel-generic", nil, Delta{
		NewCharacters: []characters.GeneratedCharacter{
			{
				CanonicalName:               "先生",
				CanonicalEpisodeIndex:       "2",
				FirstAppearanceEpisodeIndex: "2",
				SummaryHistory:              []characters.GeneratedHistoryVersion{{EpisodeIndex: "2", Text: "ハル・ミズノとして夜学で教える。"}},
			},
			{
				CanonicalName:               "先生",
				CanonicalEpisodeIndex:       "2",
				FirstAppearanceEpisodeIndex: "2",
				SummaryHistory:              []characters.GeneratedHistoryVersion{{EpisodeIndex: "2", Text: "ナギ・シオンとして薬師をしている。"}},
			},
		},
	}, nil)
	if changed != 2 || len(updated) != 2 {
		t.Fatalf("generic canonical names should not merge before stable IDs are assigned: changed=%d updated=%+v", changed, updated)
	}
	if updated[0].CharacterID == "" || updated[1].CharacterID == "" || updated[0].CharacterID == updated[1].CharacterID {
		t.Fatalf("generic new characters should receive distinct stable IDs: %+v", updated)
	}
}

func TestExtractionApplyDeltaMergeProposalsAreOrderIndependent(t *testing.T) {
	existing := []characters.GeneratedCharacter{
		{CharacterID: "char_a", CanonicalName: "A", CanonicalEpisodeIndex: "1", FirstAppearanceEpisodeIndex: "1"},
		{CharacterID: "char_b", CanonicalName: "B", CanonicalEpisodeIndex: "2", FirstAppearanceEpisodeIndex: "2"},
		{CharacterID: "char_c", CanonicalName: "C", CanonicalEpisodeIndex: "3", FirstAppearanceEpisodeIndex: "3"},
	}
	merged, changed := ApplyDelta("novel-1", existing, Delta{
		MergeProposals: []MergeProposal{
			{SourceCharacterID: "char_b", TargetCharacterID: "char_c", Confidence: 1, Reason: "同一"},
			{SourceCharacterID: "char_a", TargetCharacterID: "char_b", Confidence: 1, Reason: "同一"},
		},
	}, nil)
	if changed != 2 || len(merged) != 1 || merged[0].CharacterID != "char_a" {
		t.Fatalf("merge proposals should be applied by connected component, changed=%d merged=%+v", changed, merged)
	}
	unchanged, changed := ApplyDelta("novel-1", existing, Delta{
		MergeProposals: []MergeProposal{{SourceCharacterID: "char_b", TargetCharacterID: "char_c", Confidence: 0.5, Reason: "低信頼"}},
	}, nil)
	if changed != 0 || len(unchanged) != 3 {
		t.Fatalf("low-confidence merge proposals should not be applied automatically: changed=%d merged=%+v", changed, unchanged)
	}
}

func TestExtractionApplyDeltaCanonicalUpdateUsesNewerName(t *testing.T) {
	existing := []characters.GeneratedCharacter{{
		CharacterID:                 "char_mystery",
		CanonicalName:               "謎の少女",
		CanonicalEpisodeIndex:       "1",
		NameHistory:                 []characters.GeneratedTextVersion{{Text: "謎の少女", EpisodeIndex: "1"}},
		FirstAppearanceEpisodeIndex: "1",
		Aliases:                     []characters.GeneratedTextVersion{{Text: "謎の少女", EpisodeIndex: "1"}},
	}}
	updated, changed := ApplyDelta("novel-1", existing, Delta{
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

func TestExtractionRuntimePlanningSplitsAndReturnsRemaining(t *testing.T) {
	batch := Batch{
		BatchIndex: 1,
		BatchCount: 1,
		Chunks: []Chunk{
			{EpisodeIndex: "1", Title: "一", ChunkIndex: 1, ChunkCount: 1, Text: "1234"},
			{EpisodeIndex: "2", Title: "二", ChunkIndex: 1, ChunkCount: 1, Text: "5678"},
			{EpisodeIndex: "3", Title: "三", ChunkIndex: 1, ChunkCount: 1, Text: "90"},
		},
	}
	fitsSixChars := func(candidate Batch) (bool, error) {
		total := 0
		for _, chunk := range candidate.Chunks {
			total += len([]rune(chunk.Text))
		}
		return total <= 6, nil
	}
	runtimeBatch, remaining, err := PlanRuntimeBatch(batch, batch.Chunks, fitsSixChars)
	if err != nil {
		t.Fatalf("PlanRuntimeBatch returned error: %v", err)
	}
	if len(runtimeBatch.Chunks) != 1 || len(remaining) != 2 || runtimeBatch.EpisodeIndexes[0] != "1" {
		t.Fatalf("runtime planner should return the first fitting batch and remaining chunks: batch=%+v remaining=%+v", runtimeBatch, remaining)
	}

	tooLarge := RuntimeBatch(batch, []Chunk{{EpisodeIndex: "9", Title: "九", ChunkIndex: 1, ChunkCount: 1, Text: "12345678"}})
	split, err := PlanRuntimeBatches(tooLarge, fitsSixChars)
	if err != nil {
		t.Fatalf("PlanRuntimeBatches returned error: %v", err)
	}
	if len(split) != 2 || split[0].Chunks[0].Text != "1234" || split[1].Chunks[0].Text != "5678" {
		t.Fatalf("oversized single chunk should be split recursively: %+v", split)
	}

	empty, err := PlanRuntimeBatches(Batch{}, fitsSixChars)
	if err != nil || len(empty) != 1 {
		t.Fatalf("empty runtime batch should pass through: result=%+v err=%v", empty, err)
	}
	allFit, err := PlanRuntimeBatches(batch, func(Batch) (bool, error) { return true, nil })
	if err != nil || len(allFit) != 1 || len(allFit[0].Chunks) != 3 {
		t.Fatalf("fully fitting batch should pass through: result=%+v err=%v", allFit, err)
	}
	packed, err := PlanRuntimeBatches(batch, fitsSixChars)
	if err != nil {
		t.Fatalf("PlanRuntimeBatches returned error: %v", err)
	}
	if len(packed) != 2 || len(packed[0].Chunks) != 1 || len(packed[1].Chunks) != 2 {
		t.Fatalf("runtime batches should pack chunks until the next candidate overflows: %+v", packed)
	}
	_, _, err = PlanRuntimeBatch(batch, batch.Chunks, func(Batch) (bool, error) {
		return false, assertAnError{}
	})
	if err == nil {
		t.Fatal("runtime planner should return fit callback errors")
	}
	_, err = PlanRuntimeBatches(batch, func(Batch) (bool, error) {
		return false, assertAnError{}
	})
	if err == nil {
		t.Fatal("runtime batch planner should return fit callback errors")
	}
	_, err = SplitOversizedChunkBatch(batch, fitsSixChars)
	if err != nil {
		t.Fatalf("multi-chunk split request should pass through without error: %v", err)
	}
	_, err = SplitOversizedChunkBatch(RuntimeBatch(batch, []Chunk{{EpisodeIndex: "1", Text: "x"}}), func(Batch) (bool, error) { return false, nil })
	if err == nil {
		t.Fatal("single-rune oversized chunk should fail instead of recursing forever")
	}
	firstTooLarge := Batch{Chunks: []Chunk{
		{EpisodeIndex: "1", ChunkIndex: 1, ChunkCount: 1, Text: "12345678"},
		{EpisodeIndex: "2", ChunkIndex: 1, ChunkCount: 1, Text: "90"},
	}}
	firstSplit, remaining, err := PlanRuntimeBatch(firstTooLarge, firstTooLarge.Chunks, fitsSixChars)
	if err != nil {
		t.Fatalf("PlanRuntimeBatch with oversized first chunk returned error: %v", err)
	}
	if len(firstSplit.Chunks) != 1 || firstSplit.Chunks[0].Text != "1234" || len(remaining) != 2 {
		t.Fatalf("oversized first chunk should split and keep remaining pieces: batch=%+v remaining=%+v", firstSplit, remaining)
	}
}

type assertAnError struct{}

func (assertAnError) Error() string { return "assertion error" }

func TestExtractionResolveBatchBudgetUsesModelContext(t *testing.T) {
	if got := TokensFromChars(0); got != 0 {
		t.Fatalf("zero chars should map to zero tokens, got %d", got)
	}
	if got := TokensFromChars(5); got != 5 {
		t.Fatalf("character token estimate should be conservative for Japanese text, got %d", got)
	}
	nilBudget := ResolveBatchBudget(12001, 0, 0)
	if nilBudget.MaxTextChars != 12001 || nilBudget.MaxTextTokens != 12001 {
		t.Fatalf("missing model context should use fallback char and token budget: %+v", nilBudget)
	}
	budget := ResolveBatchBudget(12000, 128000, 16000)
	if budget.MaxTextTokens <= TokensFromChars(12000) || budget.MaxTextChars != 0 {
		t.Fatalf("model context should expand token batch budget: %+v", budget)
	}
	smallBudget := ResolveBatchBudget(12000, 2600, 0)
	if smallBudget.MaxTextChars != 12000 || smallBudget.MaxTextTokens != 12000 {
		t.Fatalf("too-small model context should keep fallback budget: %+v", smallBudget)
	}
}

func TestExtractionUnresolvedMentionsMergeAndFilter(t *testing.T) {
	merged := MergeGeneratedUnresolvedMentions([]characters.GeneratedUnresolvedMention{
		{Mention: "黒衣の男", EpisodeIndex: "10"},
	}, []UnresolvedMention{
		{Mention: "黒衣の男", EpisodeIndex: "10", Reason: "重複"},
		{Mention: "白い影", EpisodeIndex: "12", Reason: "候補なし"},
		{Mention: " ", EpisodeIndex: "13"},
	})
	if len(merged) != 2 || merged[1].Mention != "白い影" {
		t.Fatalf("unresolved mention merge should dedupe and append new values: %+v", merged)
	}
	filtered := FilterResolvedGeneratedUnresolvedMentions(merged, []characters.GeneratedCharacter{{
		CharacterID:   "char_shadow",
		CanonicalName: "白い影",
		Aliases:       []characters.GeneratedTextVersion{{Text: "白い影", EpisodeIndex: "12"}},
	}})
	if len(filtered) != 1 || filtered[0].Mention != "黒衣の男" {
		t.Fatalf("resolved unresolved mentions should be removed from the active set: %+v", filtered)
	}
	ambiguous := FilterResolvedGeneratedUnresolvedMentions([]characters.GeneratedUnresolvedMention{{Mention: "先生", EpisodeIndex: "20", CandidateIDs: []string{"char_a", "char_b"}}}, []characters.GeneratedCharacter{
		{CharacterID: "char_a", CanonicalName: "佐藤先生", Aliases: []characters.GeneratedTextVersion{{Text: "先生", EpisodeIndex: "1"}}},
		{CharacterID: "char_b", CanonicalName: "田中先生", Aliases: []characters.GeneratedTextVersion{{Text: "先生", EpisodeIndex: "1"}}},
	})
	if len(ambiguous) != 1 || ambiguous[0].Mention != "先生" {
		t.Fatalf("ambiguous unresolved mention should stay active: %+v", ambiguous)
	}
	resolvedAfterMerge := FilterResolvedGeneratedUnresolvedMentions([]characters.GeneratedUnresolvedMention{{Mention: "先生", EpisodeIndex: "20", CandidateIDs: []string{"char_a"}}}, []characters.GeneratedCharacter{
		{CharacterID: "char_a", CanonicalName: "佐藤先生", Aliases: []characters.GeneratedTextVersion{{Text: "先生", EpisodeIndex: "1"}}},
	})
	if len(resolvedAfterMerge) != 0 {
		t.Fatalf("unresolved mention should be removed only after it resolves to a unique candidate: %+v", resolvedAfterMerge)
	}
	mismatchedCandidate := FilterResolvedGeneratedUnresolvedMentions([]characters.GeneratedUnresolvedMention{{Mention: "先生", EpisodeIndex: "20", CandidateIDs: []string{"char_other"}}}, []characters.GeneratedCharacter{
		{CharacterID: "char_a", CanonicalName: "佐藤先生", Aliases: []characters.GeneratedTextVersion{{Text: "先生", EpisodeIndex: "1"}}},
	})
	if len(mismatchedCandidate) != 1 {
		t.Fatalf("mismatched candidate IDs should keep unresolved mention active: %+v", mismatchedCandidate)
	}
	if got := NormalizeSummaryStringList([]string{" b ", "a", "b", ""}); strings.Join(got, ",") != "a,b" {
		t.Fatalf("summary string list should trim, dedupe, and sort: %+v", got)
	}
}
