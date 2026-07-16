package characters

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"narou-viewer/apps/viewer-api-go/internal/state/schemaguard"
	"narou-viewer/apps/viewer-api-go/internal/state/schemaguardtest"
)

func TestLoadSummaryReadsCharacterProfiles(t *testing.T) {
	stateDir := t.TempDir()
	profileDir := filepath.Join(stateDir, "character_profiles")
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		t.Fatalf("mkdir profile dir: %v", err)
	}
	writeFile(t, filepath.Join(profileDir, "novel-1.yaml"), `
schema_version: 1
novel_id: novel-1
processed_up_to_episode_index: "3"
characters:
  - character_id: alice
    canonical_name:
      text: アリス
      episode_index: "1"
    full_name: null
    gender:
      text: 女性
      episode_index: "1"
    first_appearance_episode_index: "1"
    aliases:
      - text: アリス
        episode_index: "1"
    importance_metrics:
      episode_mentions:
        - episode_index: "1"
          count: 2
        - episode_index: "2"
          count: 1
    appearance_history:
      - episode_index: "1"
        text: 銀髪の少女。
    personality_history:
      - episode_index: "2"
        text: 冷静。
    summary_history:
      - episode_index: "3"
        text: 主人公の仲間。
`)

	summary, ok, err := LoadSummary(stateDir, "novel-1", "3")
	if err != nil {
		t.Fatalf("LoadSummary returned error: %v", err)
	}
	if !ok {
		t.Fatal("LoadSummary did not find profile")
	}
	if summary.Status != "ready" || summary.ProcessedUpToEpisodeIndex == nil || *summary.ProcessedUpToEpisodeIndex != "3" {
		t.Fatalf("unexpected summary status: %+v", summary)
	}
	if len(summary.Characters) != 1 || summary.Characters[0].CanonicalName != "アリス" || summary.Characters[0].Summary == nil {
		t.Fatalf("unexpected characters: %+v", summary.Characters)
	}
	if importance := summary.Characters[0].Importance.(map[string]any); importance["category"] != "regular" || importance["score"] != 0.542 {
		t.Fatalf("unexpected importance: %+v", importance)
	}

	partial, ok, err := LoadSummary(stateDir, "novel-1", "4")
	if err != nil || !ok {
		t.Fatalf("LoadSummary for later episode failed: ok=%v err=%v", ok, err)
	}
	if partial.Status != "partial" || len(partial.Characters) != 1 {
		t.Fatalf("expected partial data through the processed frontier, got %+v", partial)
	}
}

func TestSaveHeuristicSummaryWritesReadableProfiles(t *testing.T) {
	stateDir := t.TempDir()
	err := SaveHeuristicSummary(stateDir, "novel-1", "2", []HeuristicEpisode{
		{
			EpisodeIndex: "1",
			Text:         "アリスは街へ向かった。ボブはそれを見ていた。",
		},
		{
			EpisodeIndex: "2",
			Text:         "アリス様が扉を開けた。アリス先生は静かに笑った。",
		},
	})
	if err != nil {
		t.Fatalf("SaveHeuristicSummary returned error: %v", err)
	}
	summary, ok, err := LoadSummary(stateDir, "novel-1", "2")
	if err != nil || !ok {
		t.Fatalf("LoadSummary returned ok=%v err=%v", ok, err)
	}
	if summary.Status != "ready" || summary.ProcessedUpToEpisodeIndex == nil || *summary.ProcessedUpToEpisodeIndex != "2" {
		t.Fatalf("unexpected summary metadata: %+v", summary)
	}
	if len(summary.Characters) != 1 || summary.Characters[0].CanonicalName != "アリス" || summary.Characters[0].Summary == nil {
		t.Fatalf("heuristic summary should include repeated candidate only: %+v", summary.Characters)
	}
	var document profilesDocument
	if ok, _, err := readCharacterProfilesIfExists(filepath.Join(stateDir, "character_profiles", "novel-1.yaml"), &document); err != nil || !ok || document.SchemaVersion != characterProfilesSchemaVersion {
		t.Fatalf("heuristic profile schema = %d, ok=%v err=%v", document.SchemaVersion, ok, err)
	}
}

func TestLoadSummaryDoesNotOverwriteNewerHeuristicProfileWithOlderEvents(t *testing.T) {
	stateDir := t.TempDir()
	if err := SaveGeneratedSummary(stateDir, "novel-mixed", "100", []GeneratedCharacter{{
		CharacterID:                 "char_generated",
		CanonicalName:               "生成人物",
		CanonicalEpisodeIndex:       "1",
		FirstAppearanceEpisodeIndex: "1",
	}}); err != nil {
		t.Fatalf("SaveGeneratedSummary returned error: %v", err)
	}
	if err := SaveHeuristicSummary(stateDir, "novel-mixed", "120", []HeuristicEpisode{
		{EpisodeIndex: "120", Text: "ヒューリスティックはヒューリスティックを見た。"},
	}); err != nil {
		t.Fatalf("SaveHeuristicSummary returned error: %v", err)
	}
	summary, ok, err := LoadSummary(stateDir, "novel-mixed", "121")
	if err != nil || !ok || summary.ProcessedUpToEpisodeIndex == nil || *summary.ProcessedUpToEpisodeIndex != "120" {
		t.Fatalf("LoadSummary should keep newer heuristic processed index: ok=%v summary=%+v err=%v", ok, summary, err)
	}
	var profiles profilesDocument
	if ok, err := readYAMLIfExists(filepath.Join(stateDir, "character_profiles", "novel-mixed.yaml"), &profiles); err != nil || !ok || profiles.ProcessedUpToEpisodeIndex == nil || *profiles.ProcessedUpToEpisodeIndex != "120" {
		t.Fatalf("profile should not be rematerialized from older events: ok=%v profiles=%+v err=%v", ok, profiles, err)
	}
}

func TestSaveGeneratedSummaryWritesReadableProfiles(t *testing.T) {
	stateDir := t.TempDir()
	fullName := "アリス・リデル"
	gender := "女性"
	appearance := "青い服。"
	personality := "好奇心旺盛。"
	summaryText := "旅をする少女。"
	err := SaveGeneratedSummary(stateDir, "novel-1", "2", []GeneratedCharacter{
		{
			CanonicalName:               "アリス",
			CanonicalEpisodeIndex:       "1",
			FullName:                    &fullName,
			FullNameEpisodeIndex:        "2",
			Gender:                      &gender,
			GenderEpisodeIndex:          "1",
			FirstAppearanceEpisodeIndex: "1",
			Aliases: []GeneratedTextVersion{
				{Text: "アリス", EpisodeIndex: "1"},
				{Text: "アリス", EpisodeIndex: "1"},
				{Text: "アリス・リデル", EpisodeIndex: "2"},
			},
			AppearanceHistory: []GeneratedHistoryVersion{
				{EpisodeIndex: "1", Text: appearance},
				{EpisodeIndex: "1", Text: appearance},
			},
			PersonalityHistory: []GeneratedHistoryVersion{{EpisodeIndex: "2", Text: personality}},
			SummaryHistory:     []GeneratedHistoryVersion{{EpisodeIndex: "2", Text: summaryText}},
		},
		{
			CanonicalName: "x",
			Summary:       &summaryText,
		},
	})
	if err != nil {
		t.Fatalf("SaveGeneratedSummary returned error: %v", err)
	}
	summary, ok, err := LoadSummary(stateDir, "novel-1", "2")
	if err != nil || !ok {
		t.Fatalf("LoadSummary returned ok=%v err=%v", ok, err)
	}
	if summary.Status != "ready" || len(summary.Characters) != 1 {
		t.Fatalf("unexpected generated summary: %+v", summary)
	}
	character := summary.Characters[0]
	if !strings.HasPrefix(character.CharacterID, "char_") {
		t.Fatalf("generated character ID should use char_ prefix: %+v", character)
	}
	if character.CanonicalName != "アリス" ||
		character.FirstAppearanceEpisodeIndex != "1" ||
		character.FullName == nil ||
		*character.FullName != fullName ||
		character.Summary == nil ||
		*character.Summary != summaryText ||
		len(character.Aliases) != 2 {
		t.Fatalf("unexpected generated character: %+v", character)
	}
	if _, err := os.Stat(filepath.Join(stateDir, "character_events", "novel-1.yaml")); err != nil {
		t.Fatalf("generated summary should write event source: %v", err)
	}
	generated, processed, ok, err := LoadGeneratedCharacters(stateDir, "novel-1")
	if err != nil || !ok || processed == nil || *processed != "2" || len(generated) != 1 || generated[0].CharacterID != character.CharacterID {
		t.Fatalf("generated characters should load from event source: ok=%v processed=%v generated=%+v err=%v", ok, processed, generated, err)
	}

	renamedSummary := "本名で呼ばれるようになった。"
	if err := SaveGeneratedSummary(stateDir, "novel-1", "3", []GeneratedCharacter{{
		CharacterID:                 character.CharacterID,
		CanonicalName:               "リデル",
		CanonicalEpisodeIndex:       "3",
		FullName:                    &fullName,
		FullNameEpisodeIndex:        "2",
		FirstAppearanceEpisodeIndex: "1",
		Aliases: []GeneratedTextVersion{
			{Text: "アリス", EpisodeIndex: "1"},
			{Text: "アリス・リデル", EpisodeIndex: "2"},
			{Text: "リデル", EpisodeIndex: "3"},
		},
		SummaryHistory: []GeneratedHistoryVersion{{EpisodeIndex: "3", Text: renamedSummary}},
	}}); err != nil {
		t.Fatalf("second SaveGeneratedSummary returned error: %v", err)
	}
	renamed, ok, err := LoadSummary(stateDir, "novel-1", "3")
	if err != nil || !ok || len(renamed.Characters) != 1 {
		t.Fatalf("renamed generated summary should load: ok=%v summary=%+v err=%v", ok, renamed, err)
	}
	if renamed.Characters[0].CharacterID != character.CharacterID || renamed.Characters[0].CanonicalName != "リデル" {
		t.Fatalf("renamed character should keep stable id: before=%+v after=%+v", character, renamed.Characters[0])
	}
}

func TestSaveGeneratedSummaryKeepsJapaneseOneRuneNames(t *testing.T) {
	stateDir := t.TempDir()
	summaryText := "一字名の人物。"
	if err := SaveGeneratedSummary(stateDir, "novel-1", "1", []GeneratedCharacter{
		{
			CanonicalName: "凛",
			Summary:       &summaryText,
		},
		{
			CanonicalName: "x",
			Summary:       &summaryText,
		},
	}); err != nil {
		t.Fatalf("SaveGeneratedSummary returned error: %v", err)
	}
	summary, ok, err := LoadSummary(stateDir, "novel-1", "1")
	if err != nil || !ok {
		t.Fatalf("LoadSummary returned ok=%v err=%v", ok, err)
	}
	if len(summary.Characters) != 1 || summary.Characters[0].CanonicalName != "凛" {
		t.Fatalf("Japanese one-rune generated names should be preserved while ASCII noise is filtered: %+v", summary.Characters)
	}
}

func TestGeneratedSummaryDisplayNameRespectsEpisodeBoundaryAndRematerializes(t *testing.T) {
	stateDir := t.TempDir()
	if err := SaveGeneratedSummary(stateDir, "novel-1", "20", []GeneratedCharacter{{
		CharacterID:           "char_mystery",
		CanonicalName:         "アリス",
		CanonicalEpisodeIndex: "20",
		NameHistory: []GeneratedTextVersion{
			{Text: "謎の少女", EpisodeIndex: "1"},
			{Text: "アリス", EpisodeIndex: "20"},
		},
		FirstAppearanceEpisodeIndex: "1",
		Aliases: []GeneratedTextVersion{
			{Text: "謎の少女", EpisodeIndex: "1"},
			{Text: "アリス", EpisodeIndex: "20"},
		},
		SummaryHistory: []GeneratedHistoryVersion{{EpisodeIndex: "20", Text: "正体が判明した。"}},
	}}); err != nil {
		t.Fatalf("SaveGeneratedSummary returned error: %v", err)
	}
	early, ok, err := LoadSummary(stateDir, "novel-1", "1")
	if err != nil || !ok || len(early.Characters) != 1 {
		t.Fatalf("early summary should load: ok=%v summary=%+v err=%v", ok, early, err)
	}
	if early.Characters[0].CanonicalName != "謎の少女" || len(early.Characters[0].Aliases) != 1 || early.Characters[0].Aliases[0] != "謎の少女" || early.Characters[0].Summary != nil {
		t.Fatalf("future canonical name and facts should not leak into early summary: %+v", early.Characters[0])
	}
	late, ok, err := LoadSummary(stateDir, "novel-1", "20")
	if err != nil || !ok || len(late.Characters) != 1 {
		t.Fatalf("late summary should load: ok=%v summary=%+v err=%v", ok, late, err)
	}
	if late.Characters[0].CanonicalName != "アリス" || late.Characters[0].Summary == nil {
		t.Fatalf("late summary should use the newer canonical name and facts: %+v", late.Characters[0])
	}

	if err := os.Remove(filepath.Join(stateDir, "character_profiles", "novel-1.yaml")); err != nil {
		t.Fatalf("remove materialized profile: %v", err)
	}
	rematerialized, ok, err := LoadSummary(stateDir, "novel-1", "20")
	if err != nil || !ok || len(rematerialized.Characters) != 1 || rematerialized.Characters[0].CanonicalName != "アリス" {
		t.Fatalf("missing profile should be rematerialized from events: ok=%v summary=%+v err=%v", ok, rematerialized, err)
	}
}

func TestCharacterProfileDisplayNameFallsBackWithoutFutureLeak(t *testing.T) {
	profile := characterProfile{
		CanonicalName: textVersion{Text: "アリス", EpisodeIndex: "20"},
		PreferredNames: []textVersion{
			{Text: "謎の少女", EpisodeIndex: "1"},
			{Text: "アリス", EpisodeIndex: "20"},
		},
		Aliases: []textVersion{
			{Text: "彼女", EpisodeIndex: "1"},
			{Text: "アリス", EpisodeIndex: "20"},
		},
	}
	if got := profile.displayName("1"); got != "謎の少女" {
		t.Fatalf("early display name should use the latest visible preferred name, got %q", got)
	}
	if got := profile.displayName("20"); got != "アリス" {
		t.Fatalf("late display name should use the revealed preferred name, got %q", got)
	}
	sameEpisodePreferred := characterProfile{
		CanonicalName: textVersion{Text: "メルセデス・グリューネヴァルト", EpisodeIndex: "90"},
		PreferredNames: []textVersion{
			{Text: "ハンナ", EpisodeIndex: "90"},
		},
	}
	if got := sameEpisodePreferred.displayName("90"); got != "メルセデス・グリューネヴァルト" {
		t.Fatalf("canonical name should win over a preferred name from the same episode, got %q", got)
	}
	duplicateAliases := aliases([]textVersion{
		{Text: "フェリックス", EpisodeIndex: "3"},
		{Text: " フェリックス ", EpisodeIndex: "4"},
		{Text: "フェリック", EpisodeIndex: "4"},
		{Text: "未来名", EpisodeIndex: "99"},
	}, "10")
	if strings.Join(duplicateAliases, "/") != "フェリックス/フェリック" {
		t.Fatalf("visible aliases should be deduplicated by text while hiding future aliases: %+v", duplicateAliases)
	}
	aliasOnly := characterProfile{
		CanonicalName: textVersion{Text: "未来名", EpisodeIndex: "9"},
		PreferredNames: []textVersion{
			{Text: "未来名", EpisodeIndex: "9"},
		},
		Aliases: []textVersion{{Text: "旅人", EpisodeIndex: "2"}},
	}
	if got := aliasOnly.displayName("2"); got != "旅人" {
		t.Fatalf("visible alias should be used when preferred names are future-only, got %q", got)
	}
	fallback := characterProfile{CanonicalName: textVersion{Text: "記録名", EpisodeIndex: "9"}}
	if got := fallback.displayName("1"); got != "記録名" {
		t.Fatalf("canonical text should remain the last fallback for malformed legacy data, got %q", got)
	}
}

func TestEventRecordsToProfilesRestoresFactsMentionsAndPreferredFallback(t *testing.T) {
	fullName := textVersion{Text: "アリス・リデル", EpisodeIndex: "20"}
	gender := textVersion{Text: "女性", EpisodeIndex: "20"}
	profiles := eventRecordsToProfiles([]characterEventRecord{
		{
			CharacterID:                 "char_alice",
			CanonicalName:               textVersion{Text: "アリス", EpisodeIndex: "20"},
			FullName:                    &fullName,
			Gender:                      &gender,
			FirstAppearanceEpisodeIndex: "1",
			Aliases: []textVersion{
				{Text: "謎の少女", EpisodeIndex: "1"},
				{Text: "アリス", EpisodeIndex: "20"},
			},
			Facts: []characterFact{
				{Kind: "appearance", Text: "銀髪。", EpisodeIndex: "1"},
				{Kind: "personality", Text: "慎重。", EpisodeIndex: "2"},
				{Kind: "summary", Text: "正体が判明。", EpisodeIndex: "20"},
				{Kind: "summary", Text: "正体が判明。", EpisodeIndex: "20"},
				{Kind: "summary", Text: "", EpisodeIndex: "21"},
			},
			Mentions: []characterMention{
				{Text: "謎の少女", EpisodeIndex: "1", Count: 2},
				{Text: "謎の少女", EpisodeIndex: "1", Count: 1},
				{Text: "", EpisodeIndex: "2", Count: 5},
			},
		},
		{
			CharacterID:                 "",
			CanonicalName:               textVersion{Text: "名無し", EpisodeIndex: "1"},
			FirstAppearanceEpisodeIndex: "1",
		},
	})
	if len(profiles) != 1 {
		t.Fatalf("only valid event records should be restored: %+v", profiles)
	}
	profile := profiles[0]
	if len(profile.PreferredNames) != 1 || profile.PreferredNames[0].Text != "アリス" {
		t.Fatalf("canonical name should seed preferred_names when events omit it: %+v", profile.PreferredNames)
	}
	if profile.FullName == nil || profile.FullName.Text != "アリス・リデル" || profile.Gender == nil || profile.Gender.Text != "女性" {
		t.Fatalf("full name and gender should be restored: %+v", profile)
	}
	if len(profile.AppearanceHistory) != 1 || len(profile.PersonalityHistory) != 1 || len(profile.SummaryHistory) != 1 {
		t.Fatalf("facts should be restored and deduplicated: %+v", profile)
	}
	if profile.ImportanceMetrics == nil || len(profile.ImportanceMetrics.EpisodeMentions) != 1 || profile.ImportanceMetrics.EpisodeMentions[0].Count != 3 {
		t.Fatalf("mentions should be normalized and summed: %+v", profile.ImportanceMetrics)
	}
}

func TestLoadGeneratedCharactersTreatsEmptyProcessedEventsAsGenerated(t *testing.T) {
	stateDir := t.TempDir()
	if err := SaveGeneratedSummary(stateDir, "novel-empty", "100", nil); err != nil {
		t.Fatalf("SaveGeneratedSummary empty returned error: %v", err)
	}
	generated, processed, ok, err := LoadGeneratedCharacters(stateDir, "novel-empty")
	if err != nil || !ok || processed == nil || *processed != "100" || len(generated) != 0 {
		t.Fatalf("empty processed event document should still be generated: ok=%v processed=%v generated=%+v err=%v", ok, processed, generated, err)
	}
	summary, ok, err := LoadSummary(stateDir, "novel-empty", "100")
	if err != nil || !ok || summary.Status != "ready" || len(summary.Characters) != 0 {
		t.Fatalf("empty processed summary should be ready with no characters: ok=%v summary=%+v err=%v", ok, summary, err)
	}
}

func TestLoadGeneratedCharactersMigratesLegacyProfiles(t *testing.T) {
	stateDir := t.TempDir()
	profileDir := filepath.Join(stateDir, "character_profiles")
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		t.Fatalf("mkdir profile dir: %v", err)
	}
	writeFile(t, filepath.Join(profileDir, "novel-1.yaml"), `
novel_id: novel-1
processed_up_to_episode_index: "4"
characters:
  - character_id: char_bob
    canonical_name:
      text: ボブ
      episode_index: "2"
    full_name: null
    gender: null
    first_appearance_episode_index: "2"
    aliases:
      - text: ボブ
        episode_index: "2"
    appearance_history: []
    personality_history: []
    summary_history:
      - episode_index: "4"
        text: 後から加わる。
  - character_id: char_alice
    canonical_name:
      text: アリス
      episode_index: "1"
    full_name:
      text: アリス・リデル
      episode_index: "3"
    gender:
      text: 女性
      episode_index: "1"
    first_appearance_episode_index: "1"
    aliases:
      - text: アリス
        episode_index: "1"
      - text: リデル
        episode_index: "3"
    appearance_history:
      - episode_index: "1"
        text: 青い服。
    personality_history:
      - episode_index: "2"
        text: 好奇心旺盛。
    summary_history:
      - episode_index: "4"
        text: 仲間を導く。
`)

	generated, processed, ok, err := LoadGeneratedCharacters(stateDir, "novel-1")
	if err != nil || !ok || processed == nil || *processed != "4" {
		t.Fatalf("LoadGeneratedCharacters should migrate legacy profiles: ok=%v processed=%v err=%v", ok, processed, err)
	}
	if len(generated) != 2 || generated[0].CharacterID != "char_alice" || generated[0].FullName == nil || len(generated[0].AppearanceHistory) != 1 || generated[1].CharacterID != "char_bob" {
		t.Fatalf("unexpected migrated generated characters: %+v", generated)
	}
	if _, err := os.Stat(filepath.Join(stateDir, "character_events", "novel-1.yaml")); !os.IsNotExist(err) {
		t.Fatalf("legacy migration read should not write event file eagerly: %v", err)
	}
}

func TestAssignGeneratedCharacterIDsKeepsExplicitIDsAndSeparatesNameMatches(t *testing.T) {
	existing := []GeneratedCharacter{{
		CharacterID:                 "char_existing",
		CanonicalName:               "アリス",
		CanonicalEpisodeIndex:       "1",
		FirstAppearanceEpisodeIndex: "1",
		Aliases:                     []GeneratedTextVersion{{Text: "リデル", EpisodeIndex: "2"}},
	}}
	incoming := []GeneratedCharacter{
		{
			CanonicalName:               "リデル",
			CanonicalEpisodeIndex:       "3",
			FirstAppearanceEpisodeIndex: "1",
		},
		{
			CharacterID:                 "char_existing",
			CanonicalName:               "ボブ",
			CanonicalEpisodeIndex:       "3",
			FirstAppearanceEpisodeIndex: "3",
		},
		{
			CanonicalName:               "クレア",
			CanonicalEpisodeIndex:       "4",
			FirstAppearanceEpisodeIndex: "4",
		},
	}
	assigned := AssignGeneratedCharacterIDs("novel-1", existing, incoming)
	if len(assigned) != 3 {
		t.Fatalf("unexpected assigned characters: %+v", assigned)
	}
	if assigned[0].CharacterID == "" || assigned[0].CharacterID == "char_existing" {
		t.Fatalf("name-only matches should not reuse existing stable ids: %+v", assigned)
	}
	if assigned[1].CharacterID != "char_existing" ||
		assigned[2].CharacterID == "" || assigned[2].CharacterID == assigned[1].CharacterID {
		t.Fatalf("explicit ids should be kept and new characters should receive separate stable ids: %+v", assigned)
	}
}

func TestGeneratedCharacterEventLoadAndNormalizationBranches(t *testing.T) {
	stateDir := t.TempDir()
	if generated, processed, ok, err := LoadGeneratedCharacters(stateDir, "missing"); err != nil || ok || processed != nil || generated != nil {
		t.Fatalf("missing generated characters should be empty: ok=%v processed=%v generated=%+v err=%v", ok, processed, generated, err)
	}

	eventsDir := filepath.Join(stateDir, "character_events")
	if err := os.MkdirAll(eventsDir, 0o755); err != nil {
		t.Fatalf("mkdir events dir: %v", err)
	}
	writeFile(t, filepath.Join(eventsDir, "broken.yaml"), `characters: [`)
	if _, _, ok, err := LoadGeneratedCharacters(stateDir, "broken"); err == nil || ok {
		t.Fatalf("invalid event yaml should fail: ok=%v err=%v", ok, err)
	}

	history := normalizeGeneratedHistoryVersionList([]GeneratedHistoryVersion{
		{EpisodeIndex: "", Text: " fallback episode "},
		{EpisodeIndex: "3", Text: " "},
		{EpisodeIndex: "2", Text: "先"},
		{EpisodeIndex: "2", Text: "先"},
	})
	if len(history) != 1 || history[0].EpisodeIndex != "2" || history[0].Text != "先" {
		t.Fatalf("history normalization should trim and dedupe valid history entries: %+v", history)
	}
	if isValidGeneratedCharacterName("") || isValidGeneratedCharacterName("x") || !isValidGeneratedCharacterName("凛") {
		t.Fatal("generated character name validation should reject blanks/noise and keep Japanese one-rune names")
	}
}

func TestSaveGeneratedSummaryWithEpisodesBuildsImportanceMetrics(t *testing.T) {
	stateDir := t.TempDir()
	fullName := "アリス・リデル"
	err := SaveGeneratedSummaryWithEpisodes(stateDir, "novel-1", "3", []GeneratedCharacter{
		{
			CanonicalName:               "アリス",
			CanonicalEpisodeIndex:       "1",
			FullName:                    &fullName,
			FullNameEpisodeIndex:        "1",
			FirstAppearanceEpisodeIndex: "1",
			Aliases: []GeneratedTextVersion{
				{Text: "アリス", EpisodeIndex: "1"},
				{Text: "アリス・リデル", EpisodeIndex: "1"},
				{Text: "リデル", EpisodeIndex: "3"},
			},
		},
	}, []HeuristicEpisode{
		{EpisodeIndex: "1", Text: "アリスはアリス・リデルと呼ばれた。", ContentEtag: "etag-1"},
		{EpisodeIndex: "2", Text: "別の場面。", ContentEtag: "etag-2"},
		{EpisodeIndex: "3", Text: "リデルは扉を開けた。", ContentEtag: "etag-3"},
	})
	if err != nil {
		t.Fatalf("SaveGeneratedSummaryWithEpisodes returned error: %v", err)
	}
	var doc profilesDocument
	ok, err := readYAMLIfExists(filepath.Join(stateDir, "character_profiles", "novel-1.yaml"), &doc)
	if err != nil || !ok || len(doc.Characters) != 1 || doc.Characters[0].ImportanceMetrics == nil {
		t.Fatalf("generated profile should include importance metrics: ok=%v err=%v doc=%+v", ok, err, doc)
	}
	mentions := doc.Characters[0].ImportanceMetrics.EpisodeMentions
	if len(mentions) != 2 ||
		mentions[0].EpisodeIndex != "1" ||
		mentions[0].Count != 2 ||
		mentions[1].EpisodeIndex != "3" ||
		mentions[1].Count != 1 {
		t.Fatalf("importance metrics should count generated names in episode text: %+v", mentions)
	}
	digests, err := LoadGeneratedEpisodeDigests(stateDir, "novel-1")
	if err != nil || len(digests) != 3 || digests[0].ContentEtag != "etag-1" || digests[2].ContentEtag != "etag-3" {
		t.Fatalf("episode etags should be saved with generated events: digests=%+v err=%v", digests, err)
	}
}

func TestSaveGeneratedSummaryDoesNotReuseRetiredStableIDs(t *testing.T) {
	stateDir := t.TempDir()
	initial := []GeneratedCharacter{
		{CanonicalName: "アリス", CanonicalEpisodeIndex: "1", FirstAppearanceEpisodeIndex: "1"},
		{CanonicalName: "ボブ", CanonicalEpisodeIndex: "1", FirstAppearanceEpisodeIndex: "1"},
		{CanonicalName: "クレア", CanonicalEpisodeIndex: "1", FirstAppearanceEpisodeIndex: "1"},
	}
	if err := SaveGeneratedSummary(stateDir, "novel-ids", "1", initial); err != nil {
		t.Fatalf("initial SaveGeneratedSummary returned error: %v", err)
	}
	generated, _, ok, err := LoadGeneratedCharacters(stateDir, "novel-ids")
	if err != nil || !ok || len(generated) != 3 {
		t.Fatalf("initial generated characters should load: ok=%v generated=%+v err=%v", ok, generated, err)
	}
	deletedID := generated[2].CharacterID
	if err := SaveGeneratedSummary(stateDir, "novel-ids", "2", []GeneratedCharacter{
		generated[0],
		generated[1],
		{CanonicalName: "ダリア", CanonicalEpisodeIndex: "2", FirstAppearanceEpisodeIndex: "2"},
	}); err != nil {
		t.Fatalf("second SaveGeneratedSummary returned error: %v", err)
	}
	updated, _, ok, err := LoadGeneratedCharacters(stateDir, "novel-ids")
	if err != nil || !ok || len(updated) != 3 {
		t.Fatalf("updated generated characters should load: ok=%v generated=%+v err=%v", ok, updated, err)
	}
	newID := updated[2].CharacterID
	if newID == "" || newID == deletedID {
		t.Fatalf("new character should not reuse retired id: deleted=%s updated=%+v", deletedID, updated)
	}
	var events characterEventsDocument
	if ok, err := readYAMLIfExists(filepath.Join(stateDir, "character_events", "novel-ids.yaml"), &events); err != nil || !ok {
		t.Fatalf("events should load: ok=%v err=%v", ok, err)
	}
	if len(events.RetiredCharacterIDs) != 1 || events.RetiredCharacterIDs[0].CharacterID != deletedID {
		t.Fatalf("deleted stable id should be retained as retired: %+v", events.RetiredCharacterIDs)
	}
	allocator, err := LoadGeneratedCharacterIDAllocator(stateDir, "novel-ids", updated)
	if err != nil {
		t.Fatalf("LoadGeneratedCharacterIDAllocator returned error: %v", err)
	}
	assigned := allocator.Assign([]GeneratedCharacter{{CanonicalName: "エマ", CanonicalEpisodeIndex: "3", FirstAppearanceEpisodeIndex: "3"}})
	if len(assigned) != 1 || assigned[0].CharacterID == "" || assigned[0].CharacterID == deletedID || assigned[0].CharacterID == newID {
		t.Fatalf("persisted allocator should skip live and retired ids: deleted=%s new=%s assigned=%+v", deletedID, newID, assigned)
	}
}

func TestSaveGeneratedSummaryWithEpisodesMaterializesMergedEventMentions(t *testing.T) {
	stateDir := t.TempDir()
	if err := SaveGeneratedSummaryWithEpisodes(stateDir, "novel-mentions", "2", []GeneratedCharacter{{
		CharacterID:                 "char_alice",
		CanonicalName:               "アリス",
		CanonicalEpisodeIndex:       "1",
		FirstAppearanceEpisodeIndex: "1",
		Aliases:                     []GeneratedTextVersion{{Text: "アリス", EpisodeIndex: "1"}},
	}}, []HeuristicEpisode{
		{EpisodeIndex: "1", Text: "アリスは走った。"},
		{EpisodeIndex: "2", Text: "アリスは笑った。"},
	}); err != nil {
		t.Fatalf("initial SaveGeneratedSummaryWithEpisodes returned error: %v", err)
	}
	if err := SaveGeneratedSummaryWithEpisodes(stateDir, "novel-mentions", "3", []GeneratedCharacter{{
		CharacterID:                 "char_alice",
		CanonicalName:               "アリス",
		CanonicalEpisodeIndex:       "1",
		FirstAppearanceEpisodeIndex: "1",
		Aliases:                     []GeneratedTextVersion{{Text: "アリス", EpisodeIndex: "1"}},
	}}, []HeuristicEpisode{
		{EpisodeIndex: "3", Text: "アリスは扉を開けた。"},
	}); err != nil {
		t.Fatalf("delta SaveGeneratedSummaryWithEpisodes returned error: %v", err)
	}
	var profiles profilesDocument
	if ok, err := readYAMLIfExists(filepath.Join(stateDir, "character_profiles", "novel-mentions.yaml"), &profiles); err != nil || !ok || len(profiles.Characters) != 1 {
		t.Fatalf("profiles should load: ok=%v doc=%+v err=%v", ok, profiles, err)
	}
	mentions := profiles.Characters[0].ImportanceMetrics.EpisodeMentions
	if len(mentions) != 3 || mentions[0].EpisodeIndex != "1" || mentions[1].EpisodeIndex != "2" || mentions[2].EpisodeIndex != "3" {
		t.Fatalf("materialized profile should include previous and new event mentions: %+v", mentions)
	}
}

func TestSaveGeneratedSummaryPreservesAttributeHistoryFromSameRun(t *testing.T) {
	stateDir := t.TempDir()
	formerName := "ラビット家の令嬢"
	currentName := "アリス・リデル"
	genderUnknown := "不明"
	genderFemale := "女性"
	if err := SaveGeneratedSummaryWithEpisodes(stateDir, "novel-attribute-history", "20", []GeneratedCharacter{{
		CharacterID:                 "char_alice",
		CanonicalName:               "アリス",
		CanonicalEpisodeIndex:       "1",
		FullName:                    &currentName,
		FullNameEpisodeIndex:        "20",
		FullNameHistory:             []GeneratedTextVersion{{Text: formerName, EpisodeIndex: "5"}, {Text: currentName, EpisodeIndex: "20"}},
		Gender:                      &genderFemale,
		GenderEpisodeIndex:          "20",
		GenderHistory:               []GeneratedTextVersion{{Text: genderUnknown, EpisodeIndex: "5"}, {Text: genderFemale, EpisodeIndex: "20"}},
		FirstAppearanceEpisodeIndex: "1",
		Aliases:                     []GeneratedTextVersion{{Text: "アリス", EpisodeIndex: "1"}},
	}}, []HeuristicEpisode{
		{EpisodeIndex: "5", Text: "ラビット家の令嬢は記録に残った。"},
		{EpisodeIndex: "20", Text: "アリス・リデルは名乗った。"},
	}); err != nil {
		t.Fatalf("SaveGeneratedSummaryWithEpisodes returned error: %v", err)
	}
	var savedProfiles profilesDocument
	if ok, err := readYAMLIfExists(filepath.Join(stateDir, "character_profiles", "novel-attribute-history.yaml"), &savedProfiles); err != nil || !ok || len(savedProfiles.Characters) != 1 || len(savedProfiles.Characters[0].FullNameHistory) != 2 || len(savedProfiles.Characters[0].GenderHistory) != 2 {
		t.Fatalf("saved profile should retain attribute histories: ok=%v profiles=%+v err=%v", ok, savedProfiles, err)
	}
	mentions := savedProfiles.Characters[0].ImportanceMetrics.EpisodeMentions
	if len(mentions) != 2 || mentions[0].EpisodeIndex != "5" || mentions[1].EpisodeIndex != "20" {
		t.Fatalf("mention count should include full name history values: %+v", mentions)
	}
	early, ok, err := LoadSummary(stateDir, "novel-attribute-history", "5")
	if err != nil || !ok || len(early.Characters) != 1 || early.Characters[0].FullName == nil || *early.Characters[0].FullName != formerName || early.Characters[0].Gender == nil || *early.Characters[0].Gender != genderUnknown {
		t.Fatalf("early summary should use visible attribute history: ok=%v summary=%+v profiles=%+v err=%v", ok, early, savedProfiles, err)
	}
	latest, ok, err := LoadSummary(stateDir, "novel-attribute-history", "20")
	if err != nil || !ok || len(latest.Characters) != 1 || latest.Characters[0].FullName == nil || *latest.Characters[0].FullName != currentName || latest.Characters[0].Gender == nil || *latest.Characters[0].Gender != genderFemale {
		t.Fatalf("latest summary should use newest attribute history: ok=%v summary=%+v err=%v", ok, latest, err)
	}
	seed, _, ok, err := LoadGeneratedCharacters(stateDir, "novel-attribute-history")
	if err != nil || !ok || len(seed) != 1 || len(seed[0].FullNameHistory) != 2 || len(seed[0].GenderHistory) != 2 {
		t.Fatalf("generated seed should retain attribute histories: ok=%v seed=%+v err=%v", ok, seed, err)
	}
}

func TestSaveGeneratedSummaryMergesRetiredSourceEventHistory(t *testing.T) {
	stateDir := t.TempDir()
	nameB := "ベータ旧名"
	sourceSummary := "かつて別名で動いていた。"
	if err := SaveGeneratedSummaryWithEpisodes(stateDir, "novel-merge-events", "2", []GeneratedCharacter{
		{
			CharacterID:                 "char_a",
			CanonicalName:               "アルファ",
			CanonicalEpisodeIndex:       "1",
			FirstAppearanceEpisodeIndex: "1",
			Aliases:                     []GeneratedTextVersion{{Text: "アルファ", EpisodeIndex: "1"}},
		},
		{
			CharacterID:                 "char_b",
			CanonicalName:               "ベータ",
			CanonicalEpisodeIndex:       "2",
			FullName:                    &nameB,
			FullNameEpisodeIndex:        "2",
			FullNameHistory:             []GeneratedTextVersion{{Text: nameB, EpisodeIndex: "2"}},
			FirstAppearanceEpisodeIndex: "2",
			Aliases:                     []GeneratedTextVersion{{Text: "ベータ別名", EpisodeIndex: "2"}},
			SummaryHistory:              []GeneratedHistoryVersion{{EpisodeIndex: "2", Text: sourceSummary}},
		},
	}, []HeuristicEpisode{
		{EpisodeIndex: "1", Text: "アルファは歩いた。"},
		{EpisodeIndex: "2", Text: "ベータ別名は笑った。"},
	}); err != nil {
		t.Fatalf("initial SaveGeneratedSummaryWithEpisodes returned error: %v", err)
	}
	if err := SaveGeneratedSummaryWithOptions(stateDir, "novel-merge-events", "3", []GeneratedCharacter{{
		CharacterID:                 "char_a",
		CanonicalName:               "アルファ",
		CanonicalEpisodeIndex:       "1",
		FirstAppearanceEpisodeIndex: "1",
	}}, []HeuristicEpisode{{EpisodeIndex: "3", Text: "アルファはベータでもあった。"}}, SaveGeneratedSummaryOptions{
		RetiredCharacterIDs: []GeneratedRetiredCharacterID{{CharacterID: "char_b", MergedInto: "char_a"}},
	}); err != nil {
		t.Fatalf("merge SaveGeneratedSummaryWithOptions returned error: %v", err)
	}
	var profiles profilesDocument
	if ok, err := readYAMLIfExists(filepath.Join(stateDir, "character_profiles", "novel-merge-events.yaml"), &profiles); err != nil || !ok || len(profiles.Characters) != 1 {
		t.Fatalf("profiles should load after merge: ok=%v profiles=%+v err=%v", ok, profiles, err)
	}
	mentions := profiles.Characters[0].ImportanceMetrics.EpisodeMentions
	if len(mentions) != 3 || mentions[0].EpisodeIndex != "1" || mentions[1].EpisodeIndex != "2" || mentions[2].EpisodeIndex != "3" {
		t.Fatalf("target profile should inherit source mentions: %+v", mentions)
	}
	if profiles.Characters[0].FullName == nil || profiles.Characters[0].FullName.Text != nameB || len(profiles.Characters[0].FullNameHistory) != 1 {
		t.Fatalf("target profile should inherit source attribute history: %+v", profiles.Characters[0])
	}
	if len(profiles.Characters[0].PreferredNames) != 2 || profiles.Characters[0].PreferredNames[1].Text != "ベータ" {
		t.Fatalf("target profile should inherit source preferred names: %+v", profiles.Characters[0].PreferredNames)
	}
	if len(profiles.Characters[0].Aliases) != 2 || profiles.Characters[0].Aliases[1].Text != "ベータ別名" {
		t.Fatalf("target profile should inherit source aliases: %+v", profiles.Characters[0].Aliases)
	}
	if len(profiles.Characters[0].SummaryHistory) != 1 || profiles.Characters[0].SummaryHistory[0].Text != sourceSummary {
		t.Fatalf("target profile should inherit source facts: %+v", profiles.Characters[0].SummaryHistory)
	}
}

func TestIdentityMergeEventOnlyAppliesAtEffectiveEpisode(t *testing.T) {
	stateDir := t.TempDir()
	novelID := "novel-timed-identity"
	blackKnight := GeneratedCharacter{
		CharacterID:                 "char_black_knight",
		CanonicalName:               "黒騎士",
		CanonicalEpisodeIndex:       "1",
		FirstAppearanceEpisodeIndex: "1",
		NameHistory:                 []GeneratedTextVersion{{Text: "黒騎士", EpisodeIndex: "1"}},
		Aliases:                     []GeneratedTextVersion{{Text: "黒騎士", EpisodeIndex: "1"}},
	}
	alice := GeneratedCharacter{
		CharacterID:                 "char_alice",
		CanonicalName:               "アリス",
		CanonicalEpisodeIndex:       "2",
		FirstAppearanceEpisodeIndex: "2",
		NameHistory:                 []GeneratedTextVersion{{Text: "アリス", EpisodeIndex: "2"}},
		Aliases:                     []GeneratedTextVersion{{Text: "アリス", EpisodeIndex: "2"}},
	}
	if err := SaveGeneratedSummaryWithOptions(stateDir, novelID, "20", []GeneratedCharacter{blackKnight, alice}, nil, SaveGeneratedSummaryOptions{
		IdentityMergeEvents: []GeneratedIdentityMergeEvent{{
			SourceCharacterID:     "char_black_knight",
			TargetCharacterID:     "char_alice",
			EffectiveEpisodeIndex: "20",
		}},
	}); err != nil {
		t.Fatalf("SaveGeneratedSummaryWithOptions returned error: %v", err)
	}

	beforeReveal, ok, err := LoadSummary(stateDir, novelID, "2")
	if err != nil || !ok {
		t.Fatalf("LoadSummary before reveal failed: ok=%v err=%v", ok, err)
	}
	if len(beforeReveal.Characters) != 2 {
		t.Fatalf("before reveal characters = %+v, want two separate identities", beforeReveal.Characters)
	}
	for _, character := range beforeReveal.Characters {
		if len(character.Aliases) != 1 {
			t.Fatalf("before reveal aliases leaked identity relation: %+v", beforeReveal.Characters)
		}
	}

	afterReveal, ok, err := LoadSummary(stateDir, novelID, "20")
	if err != nil || !ok {
		t.Fatalf("LoadSummary after reveal failed: ok=%v err=%v", ok, err)
	}
	if len(afterReveal.Characters) != 1 || len(afterReveal.Characters[0].Aliases) != 2 {
		t.Fatalf("after reveal characters = %+v, want merged identity", afterReveal.Characters)
	}
	mergedSeed, _, ok, err := LoadGeneratedCharacters(stateDir, novelID)
	if err != nil || !ok || len(mergedSeed) != 1 {
		t.Fatalf("LoadGeneratedCharacters after reveal = %+v ok=%v err=%v", mergedSeed, ok, err)
	}
	rawSeed, seedEvents, _, ok, err := LoadGeneratedCharacterState(stateDir, novelID)
	if err != nil || !ok || len(rawSeed) != 2 || len(seedEvents) != 1 {
		t.Fatalf("LoadGeneratedCharacterState = raw=%+v events=%+v ok=%v err=%v", rawSeed, seedEvents, ok, err)
	}
	if err := SaveGeneratedSummaryWithOptions(stateDir, novelID, "21", rawSeed, nil, SaveGeneratedSummaryOptions{IdentityMergeEvents: seedEvents}); err != nil {
		t.Fatalf("incremental SaveGeneratedSummaryWithOptions returned error: %v", err)
	}
	beforeRevealAgain, ok, err := LoadSummary(stateDir, novelID, "2")
	if err != nil || !ok || len(beforeRevealAgain.Characters) != 2 {
		t.Fatalf("incremental save lost pre-reveal identities: characters=%+v ok=%v err=%v", beforeRevealAgain.Characters, ok, err)
	}
	aliasesByName := map[string][]string{}
	for _, character := range beforeRevealAgain.Characters {
		aliasesByName[character.CanonicalName] = character.Aliases
	}
	if len(aliasesByName["黒騎士"]) != 1 || aliasesByName["黒騎士"][0] != "黒騎士" || len(aliasesByName["アリス"]) != 1 || aliasesByName["アリス"][0] != "アリス" {
		t.Fatalf("incremental save leaked merged aliases before reveal: %+v", beforeRevealAgain.Characters)
	}
}

func TestIdentityMergeEventsDropDeterministicCycle(t *testing.T) {
	stateDir := t.TempDir()
	generated := []GeneratedCharacter{
		{CharacterID: "char_a", CanonicalName: "甲", CanonicalEpisodeIndex: "1", FirstAppearanceEpisodeIndex: "1", NameHistory: []GeneratedTextVersion{{Text: "甲", EpisodeIndex: "1"}}},
		{CharacterID: "char_b", CanonicalName: "乙", CanonicalEpisodeIndex: "1", FirstAppearanceEpisodeIndex: "1", NameHistory: []GeneratedTextVersion{{Text: "乙", EpisodeIndex: "1"}}},
	}
	events := []GeneratedIdentityMergeEvent{
		{SourceCharacterID: "char_a", TargetCharacterID: "char_b", EffectiveEpisodeIndex: "2"},
		{SourceCharacterID: "char_b", TargetCharacterID: "char_a", EffectiveEpisodeIndex: "2"},
	}
	if err := SaveGeneratedSummaryWithOptions(stateDir, "novel-cycle", "2", generated, nil, SaveGeneratedSummaryOptions{IdentityMergeEvents: events}); err != nil {
		t.Fatalf("save cycle: %v", err)
	}
	summary, ok, err := LoadSummary(stateDir, "novel-cycle", "2")
	if err != nil || !ok || len(summary.Characters) != 1 || summary.Characters[0].CharacterID != "char_b" {
		t.Fatalf("cycle normalization should keep deterministic A to B event: summary=%+v ok=%v err=%v", summary, ok, err)
	}
}

func TestSaveGeneratedSummaryWithOptionsReplacesReprocessedEventRange(t *testing.T) {
	stateDir := t.TempDir()
	if err := SaveGeneratedSummaryWithEpisodes(stateDir, "novel-reprocess", "2", []GeneratedCharacter{{
		CharacterID:                 "char_alice",
		CanonicalName:               "アリス",
		CanonicalEpisodeIndex:       "1",
		FirstAppearanceEpisodeIndex: "1",
		Aliases:                     []GeneratedTextVersion{{Text: "アリス", EpisodeIndex: "1"}},
	}}, []HeuristicEpisode{
		{EpisodeIndex: "1", Text: "アリスは走った。", ContentEtag: "old-1"},
		{EpisodeIndex: "2", Text: "アリスは笑った。", ContentEtag: "old-2"},
	}); err != nil {
		t.Fatalf("initial SaveGeneratedSummaryWithEpisodes returned error: %v", err)
	}
	seed, processed, ok, err := LoadGeneratedCharactersBeforeEpisode(stateDir, "novel-reprocess", "2")
	if err != nil || !ok || processed == nil || *processed != "1" || len(seed) != 1 {
		t.Fatalf("seed before changed episode should be truncated: ok=%v processed=%v seed=%+v err=%v", ok, processed, seed, err)
	}
	emptySeed, emptyProcessed, ok, err := LoadGeneratedCharactersBeforeEpisode(stateDir, "novel-reprocess", "1")
	if err != nil || !ok || emptyProcessed != nil || len(emptySeed) != 0 {
		t.Fatalf("seed before first changed episode should be empty: ok=%v processed=%v seed=%+v err=%v", ok, emptyProcessed, emptySeed, err)
	}
	allSeed, allProcessed, ok, err := LoadGeneratedCharactersBeforeEpisode(stateDir, "novel-reprocess", "")
	if err != nil || !ok || allProcessed == nil || len(allSeed) != 1 {
		t.Fatalf("blank cutoff should load full generated seed: ok=%v processed=%v seed=%+v err=%v", ok, allProcessed, allSeed, err)
	}
	if err := SaveGeneratedSummaryWithOptions(stateDir, "novel-reprocess", "2", []GeneratedCharacter{{
		CharacterID:                 "char_alice",
		CanonicalName:               "アリス",
		CanonicalEpisodeIndex:       "1",
		FirstAppearanceEpisodeIndex: "1",
		Aliases:                     []GeneratedTextVersion{{Text: "アリス", EpisodeIndex: "1"}},
	}}, []HeuristicEpisode{
		{EpisodeIndex: "2", Text: "アリスは扉を開けた。", ContentEtag: "new-2"},
	}, SaveGeneratedSummaryOptions{ReplaceFromEpisodeIndex: "2"}); err != nil {
		t.Fatalf("replacement SaveGeneratedSummaryWithOptions returned error: %v", err)
	}
	var profiles profilesDocument
	if ok, err := readYAMLIfExists(filepath.Join(stateDir, "character_profiles", "novel-reprocess.yaml"), &profiles); err != nil || !ok || len(profiles.Characters) != 1 {
		t.Fatalf("profiles should load after replacement: ok=%v doc=%+v err=%v", ok, profiles, err)
	}
	mentions := profiles.Characters[0].ImportanceMetrics.EpisodeMentions
	if len(mentions) != 2 || mentions[0].EpisodeIndex != "1" || mentions[0].Count != 1 || mentions[1].EpisodeIndex != "2" || mentions[1].Count != 1 {
		t.Fatalf("replacement should keep old range before cutoff and replace changed range without double count: %+v", mentions)
	}
	digests, err := LoadGeneratedEpisodeDigests(stateDir, "novel-reprocess")
	if err != nil || len(digests) != 2 || digests[1].ContentEtag != "new-2" {
		t.Fatalf("replacement should update episode etags from cutoff: digests=%+v err=%v", digests, err)
	}
}

func TestSaveGeneratedSummaryPersistsUnresolvedMentions(t *testing.T) {
	stateDir := t.TempDir()
	if err := SaveGeneratedSummaryWithEpisodes(stateDir, "novel-unresolved", "10", nil, nil, []GeneratedUnresolvedMention{
		{Mention: "黒衣の男", EpisodeIndex: "10", Reason: "正体不明"},
		{Mention: "黒衣の男", EpisodeIndex: "10", Reason: "重複"},
	}); err != nil {
		t.Fatalf("SaveGeneratedSummaryWithEpisodes unresolved returned error: %v", err)
	}
	mentions, err := LoadGeneratedUnresolvedMentions(stateDir, "novel-unresolved")
	if err != nil || len(mentions) != 1 || mentions[0].Mention != "黒衣の男" || mentions[0].EpisodeIndex != "10" {
		t.Fatalf("unresolved mentions should be persisted and deduplicated: mentions=%+v err=%v", mentions, err)
	}
	if err := SaveGeneratedSummaryWithOptions(stateDir, "novel-unresolved", "11", nil, nil, SaveGeneratedSummaryOptions{
		SetUnresolvedMentions: true,
		UnresolvedMentions:    []GeneratedUnresolvedMention{},
	}); err != nil {
		t.Fatalf("empty unresolved replacement returned error: %v", err)
	}
	mentions, err = LoadGeneratedUnresolvedMentions(stateDir, "novel-unresolved")
	if err != nil || len(mentions) != 0 {
		t.Fatalf("empty unresolved replacement should delete persisted active mentions: mentions=%+v err=%v", mentions, err)
	}
	merged := mergeUnresolvedMentions([]unresolvedMention{{Mention: "既存", EpisodeIndex: "1"}}, []GeneratedUnresolvedMention{
		{Mention: "  ", EpisodeIndex: "2"},
		{Mention: "新規", EpisodeIndex: "2", Reason: "候補なし", CandidateIDs: []string{" char_b ", "char_b", "char_a"}},
	})
	if len(merged) != 2 || len(merged[1].CandidateIDs) != 2 || merged[1].CandidateIDs[0] != "char_a" {
		t.Fatalf("mergeUnresolvedMentions should trim, sort, and dedupe candidates: %+v", merged)
	}
	if got := normalizeStringList([]string{" b ", "a", "b", " "}); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("normalizeStringList should trim, sort, and dedupe: %+v", got)
	}
	etags := mergeEpisodeEtags([]episodeEtag{{EpisodeIndex: "2", ContentEtag: "old"}}, []HeuristicEpisode{
		{EpisodeIndex: " ", ContentEtag: "ignored"},
		{EpisodeIndex: "1", ContentEtag: "etag-1"},
		{EpisodeIndex: "2", ContentEtag: "etag-2"},
	})
	if len(etags) != 2 || etags[0].EpisodeIndex != "1" || etags[1].ContentEtag != "etag-2" {
		t.Fatalf("mergeEpisodeEtags should update and sort etags: %+v", etags)
	}
}

func TestSaveGeneratedSummaryPersistsIssuedRetiredIDsBeforeDurableCharacters(t *testing.T) {
	stateDir := t.TempDir()
	issuedOnlyID := stableCharacterIDForOrdinal("novel-run-ids", 1)
	retiredID := stableCharacterIDForOrdinal("novel-run-ids", 2)
	if err := SaveGeneratedSummaryWithOptions(stateDir, "novel-run-ids", "1", []GeneratedCharacter{{
		CharacterID:                 issuedOnlyID,
		CanonicalName:               "アリス",
		CanonicalEpisodeIndex:       "1",
		FirstAppearanceEpisodeIndex: "1",
	}}, nil, SaveGeneratedSummaryOptions{
		IssuedCharacterIDs:   []string{issuedOnlyID, retiredID},
		RetiredCharacterIDs:  []GeneratedRetiredCharacterID{{CharacterID: retiredID, MergedInto: issuedOnlyID}},
		NextCharacterOrdinal: 3,
	}); err != nil {
		t.Fatalf("SaveGeneratedSummaryWithOptions returned error: %v", err)
	}
	allocator, err := LoadGeneratedCharacterIDAllocator(stateDir, "novel-run-ids", nil)
	if err != nil {
		t.Fatalf("LoadGeneratedCharacterIDAllocator returned error: %v", err)
	}
	assigned := allocator.Assign([]GeneratedCharacter{{CanonicalName: "ボブ", CanonicalEpisodeIndex: "2", FirstAppearanceEpisodeIndex: "2"}})
	if len(assigned) != 1 || assigned[0].CharacterID == "" || assigned[0].CharacterID == issuedOnlyID || assigned[0].CharacterID == retiredID {
		t.Fatalf("allocator should not reuse issued or retired ids: issued=%s retired=%s assigned=%+v", issuedOnlyID, retiredID, assigned)
	}
	var events characterEventsDocument
	if ok, err := readYAMLIfExists(filepath.Join(stateDir, "character_events", "novel-run-ids.yaml"), &events); err != nil || !ok {
		t.Fatalf("events should load: ok=%v err=%v", ok, err)
	}
	if events.NextCharacterOrdinal < 3 || len(events.RetiredCharacterIDs) != 1 || events.RetiredCharacterIDs[0].CharacterID != retiredID {
		t.Fatalf("events should retain run allocator state: %+v", events)
	}
}

func TestGeneratedCharacterIDAllocatorTracksRunState(t *testing.T) {
	allocator := NewGeneratedCharacterIDAllocator("novel-allocator", nil)
	assigned := allocator.Assign([]GeneratedCharacter{
		{CanonicalName: "アリス", CanonicalEpisodeIndex: "1", FirstAppearanceEpisodeIndex: "1"},
		{CanonicalName: "ボブ", CanonicalEpisodeIndex: "1", FirstAppearanceEpisodeIndex: "1"},
	})
	if len(assigned) != 2 || assigned[0].CharacterID == "" || assigned[1].CharacterID == "" {
		t.Fatalf("allocator should assign stable ids: %+v", assigned)
	}
	issued := allocator.IssuedCharacterIDs()
	if len(issued) != 2 {
		t.Fatalf("allocator should expose issued ids: %+v", issued)
	}
	allocator.Retire(assigned[1].CharacterID, assigned[0].CharacterID)
	retired := allocator.RetiredCharacterIDs()
	if len(retired) != 1 || retired[0].CharacterID != assigned[1].CharacterID || retired[0].MergedInto != assigned[0].CharacterID {
		t.Fatalf("allocator should expose retired ids: %+v", retired)
	}
	nextOrdinal := allocator.NextCharacterOrdinal()
	resumed := NewGeneratedCharacterIDAllocator("novel-allocator", nil)
	resumed.ApplyState(nextOrdinal, issued, retired)
	nextAssigned := resumed.Assign([]GeneratedCharacter{{CanonicalName: "クレア", CanonicalEpisodeIndex: "2", FirstAppearanceEpisodeIndex: "2"}})
	if len(nextAssigned) != 1 || nextAssigned[0].CharacterID == "" || nextAssigned[0].CharacterID == assigned[0].CharacterID || nextAssigned[0].CharacterID == assigned[1].CharacterID {
		t.Fatalf("resumed allocator should skip live, issued, and retired ids: next=%+v previous=%+v", nextAssigned, assigned)
	}
}

func TestGeneratedEventTruncationHelpers(t *testing.T) {
	facts := filterCharacterFactsBeforeEpisode([]characterFact{
		{Kind: "summary", Text: "残る", EpisodeIndex: "1"},
		{Kind: "summary", Text: "消える", EpisodeIndex: "2"},
		{Kind: "summary", Text: "", EpisodeIndex: "1"},
	}, "2")
	if len(facts) != 1 || facts[0].Text != "残る" {
		t.Fatalf("facts should be truncated before reprocess boundary: %+v", facts)
	}
	unresolved := truncateUnresolvedMentionsBeforeEpisode([]unresolvedMention{
		{Mention: "残る", EpisodeIndex: "1"},
		{Mention: "消える", EpisodeIndex: "2"},
		{Mention: "空", EpisodeIndex: ""},
	}, "2")
	if len(unresolved) != 1 || unresolved[0].Mention != "残る" {
		t.Fatalf("unresolved mentions should be truncated before reprocess boundary: %+v", unresolved)
	}
	if previousProcessedEpisodeIndex(nil, "2") != nil {
		t.Fatal("nil processed index should remain nil")
	}
	truncatedRecords := truncateCharacterEventRecordsBeforeEpisode([]characterEventRecord{{
		CharacterID:                 "char_alice",
		CanonicalName:               textVersion{Text: "アリス", EpisodeIndex: "1"},
		PreferredNames:              []textVersion{{Text: "アリス", EpisodeIndex: "1"}},
		FullName:                    &textVersion{Text: "アリス・リデル", EpisodeIndex: "3"},
		FullNameHistory:             []textVersion{{Text: "アリス", EpisodeIndex: "1"}, {Text: "アリス・リデル", EpisodeIndex: "3"}},
		Gender:                      &textVersion{Text: "女性", EpisodeIndex: "3"},
		GenderHistory:               []textVersion{{Text: "不明", EpisodeIndex: "1"}, {Text: "女性", EpisodeIndex: "3"}},
		FirstAppearanceEpisodeIndex: "1",
		Aliases:                     []textVersion{{Text: "アリス", EpisodeIndex: "1"}},
	}}, "2")
	if len(truncatedRecords) != 1 ||
		truncatedRecords[0].FullName == nil ||
		truncatedRecords[0].FullName.Text != "アリス" ||
		truncatedRecords[0].Gender == nil ||
		truncatedRecords[0].Gender.Text != "不明" {
		t.Fatalf("attribute histories should restore the latest visible value at cutoff: %+v", truncatedRecords)
	}
}

func TestLoadSummaryFiltersFutureCharacterDetails(t *testing.T) {
	stateDir := t.TempDir()
	profileDir := filepath.Join(stateDir, "character_profiles")
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		t.Fatalf("mkdir profile dir: %v", err)
	}
	writeFile(t, filepath.Join(profileDir, "novel-1.yaml"), `
novel_id: novel-1
processed_up_to_episode_index: "3"
characters:
  - character_id: alice
    canonical_name:
      text: アリス
      episode_index: "1"
    full_name:
      text: アリス・リデル
      episode_index: "3"
    full_name_history:
      - text: アリス
        episode_index: "1"
      - text: アリス・リデル
        episode_index: "3"
    gender:
      text: 女性
      episode_index: "2"
    gender_history:
      - text: 不明
        episode_index: "1"
      - text: 女性
        episode_index: "2"
    first_appearance_episode_index: "1"
    aliases:
      - text: アリス
        episode_index: "1"
      - text: リデル
        episode_index: "3"
    importance_metrics:
      episode_mentions:
        - episode_index: "1"
          count: 1
        - episode_index: "3"
          count: 5
    appearance_history:
      - episode_index: "1"
        text: ローブ姿。
      - episode_index: "3"
        text: 王冠をかぶる。
    personality_history:
      - episode_index: "3"
        text: 勇敢。
    summary_history:
      - episode_index: "1"
        text: 旅人。
      - episode_index: "3"
        text: 女王。
  - character_id: bob
    canonical_name:
      text: ボブ
      episode_index: "3"
    first_appearance_episode_index: "3"
    aliases: []
    appearance_history: []
    personality_history: []
    summary_history: []
`)

	summary, ok, err := LoadSummary(stateDir, "novel-1", "1")
	if err != nil || !ok {
		t.Fatalf("LoadSummary returned ok=%v err=%v", ok, err)
	}
	if len(summary.Characters) != 1 {
		t.Fatalf("future character should be hidden: %+v", summary.Characters)
	}
	alice := summary.Characters[0]
	if alice.FullName == nil || *alice.FullName != "アリス" || alice.Gender == nil || *alice.Gender != "不明" || alice.Personality != nil {
		t.Fatalf("future attributes should fall back to latest visible values: %+v", alice)
	}
	if len(alice.Aliases) != 1 || alice.Aliases[0] != "アリス" {
		t.Fatalf("future aliases should be hidden: %+v", alice.Aliases)
	}
	if alice.Appearance == nil || *alice.Appearance != "ローブ姿。" || alice.Summary == nil || *alice.Summary != "旅人。" {
		t.Fatalf("latest visible history was not selected: %+v", alice)
	}
	if importance := alice.Importance.(map[string]any); importance["category"] != "semi-regular" || importance["score"] != 0.825 {
		t.Fatalf("future mentions should be excluded: %+v", importance)
	}
}

func TestCharacterCandidateAndImportanceHelpers(t *testing.T) {
	if isLikelyCandidateName("それ") || isLikelyCandidateName("これ") || isLikelyCandidateName(strings.Repeat("長", 19)) {
		t.Fatal("common words and oversized names should be rejected")
	}
	if !isLikelyCandidateName("アリス") || normalizeCandidateName(" アリスさん ") != "アリス" {
		t.Fatal("valid candidate names should be accepted and normalized")
	}
	if firstNonEmpty("", "  ", "x") != "x" || firstNonEmpty("", " ") != "" {
		t.Fatal("firstNonEmpty should return the first trimmed non-empty value")
	}
	history := generatedHistoryVersions([]GeneratedHistoryVersion{
		{EpisodeIndex: "2", Text: "後"},
		{EpisodeIndex: "", Text: "補完"},
		{EpisodeIndex: "1", Text: "先"},
		{EpisodeIndex: "1", Text: "先"},
		{EpisodeIndex: "3", Text: " "},
	}, "9")
	if len(history) != 3 ||
		history[0].EpisodeIndex != "1" ||
		history[1].EpisodeIndex != "2" ||
		history[2].EpisodeIndex != "9" {
		t.Fatalf("generatedHistoryVersions should normalize, dedupe, and sort: %+v", history)
	}
	if clamp01(-0.1) != 0 || clamp01(1.2) != 1 || clamp01(0.4) != 0.4 {
		t.Fatal("clamp01 should clamp to the unit interval")
	}
	if importanceOrder(nil) != 3 ||
		importanceOrder(map[string]any{"category": "main"}) != 0 ||
		importanceOrder(map[string]any{"category": "regular"}) != 1 ||
		importanceOrder(map[string]any{"category": "semi-regular"}) != 2 ||
		importanceOrder(map[string]any{"category": "other"}) != 3 {
		t.Fatal("importanceOrder should map known categories to display order")
	}
	positions := buildEpisodePositionMap([]string{"1", "", "3"})
	if totalEpisodesConsidered([]string{"1", "", "3"}, "3", positions) != 3 || totalEpisodesConsidered(nil, "bad", nil) != 0 {
		t.Fatal("totalEpisodesConsidered should use TOC positions and reject invalid fallback indexes")
	}
	if episodePosition("2", nil) != 2 || episodePosition("bad", nil) != 0 {
		t.Fatal("episodePosition should fall back to numeric episode indexes")
	}

	profiles := []characterProfile{
		{
			CharacterID:                 "alice",
			CanonicalName:               textVersion{Text: "アリス", EpisodeIndex: "1"},
			FirstAppearanceEpisodeIndex: "1",
			SummaryHistory:              []historyVersion{{Text: "主人公。", EpisodeIndex: "1"}},
			ImportanceMetrics: &importanceMetricsDoc{EpisodeMentions: []episodeMentionDoc{
				{EpisodeIndex: "1", Count: 5},
				{EpisodeIndex: "2", Count: 5},
				{EpisodeIndex: "3", Count: 5},
			}},
		},
		{
			CharacterID:                 "bob",
			CanonicalName:               textVersion{Text: "ボブ", EpisodeIndex: "2"},
			FirstAppearanceEpisodeIndex: "2",
			ImportanceMetrics:           nil,
		},
		{
			CharacterID:                 "invalid",
			CanonicalName:               textVersion{Text: "無効", EpisodeIndex: "bad"},
			FirstAppearanceEpisodeIndex: "bad",
		},
	}
	classifications := buildImportanceClassifications(profiles, []string{"1", "2", "3"}, "3")
	if classifications["invalid"] != nil {
		t.Fatalf("invalid first appearance should not produce importance: %+v", classifications)
	}
	if classifications["alice"].(map[string]any)["category"] != "main" {
		t.Fatalf("high coverage character should be main: %+v", classifications["alice"])
	}
	if classifications["bob"].(map[string]any)["category"] != "semi-regular" {
		t.Fatalf("fallback mention character should be semi-regular: %+v", classifications["bob"])
	}
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestCharacterHelpersCoverEmptyAndComparisonCases(t *testing.T) {
	if latestHistoryText(nil, "1") != nil || textValuePtr(&textVersion{}, "1") != nil {
		t.Fatal("empty history and text versions should return nil")
	}
	classifications := buildImportanceClassifications([]characterProfile{
		{
			CharacterID:                 "alice",
			CanonicalName:               textVersion{Text: "アリス", EpisodeIndex: "1"},
			FirstAppearanceEpisodeIndex: "1",
			ImportanceMetrics:           &importanceMetricsDoc{EpisodeMentions: []episodeMentionDoc{{EpisodeIndex: "1", Count: 3}, {EpisodeIndex: "2", Count: 2}}},
			SummaryHistory:              []historyVersion{{EpisodeIndex: "1", Text: "旅人"}},
		},
		{
			CharacterID:                 "bob",
			CanonicalName:               textVersion{Text: "ボブ", EpisodeIndex: "2"},
			FirstAppearanceEpisodeIndex: "2",
			ImportanceMetrics:           &importanceMetricsDoc{EpisodeMentions: []episodeMentionDoc{{EpisodeIndex: "2", Count: 1}}},
		},
	}, []string{"1", "2", "3"}, "3")
	if got := classifications["alice"].(map[string]any); got["category"] != "main" || got["score"] != 0.558 {
		t.Fatalf("unexpected main importance: %+v", got)
	}
	if got := classifications["bob"].(map[string]any); got["category"] != "semi-regular" || got["score"] != 0.292 {
		t.Fatalf("unexpected fallback importance: %+v", got)
	}
	if !episodeWithin("1", "1") || episodeWithin("2", "1") {
		t.Fatal("episodeWithin returned unexpected result")
	}
	if importanceOrder(map[string]any{"category": "regular"}) != 1 || importanceOrder(nil) != 3 {
		t.Fatal("importanceOrder returned unexpected result")
	}
	if compareEpisodeIndex("1", "2") >= 0 || compareEpisodeIndex("2", "1") <= 0 || compareEpisodeIndex("2", "2") != 0 || compareEpisodeIndex("a", "b") >= 0 || compareEpisodeIndex("b", "a") <= 0 || compareEpisodeIndex("a", "a") != 0 {
		t.Fatal("episode comparison did not order numeric and lexical values")
	}
}

func TestLoadSummaryQuarantinesInvalidYAML(t *testing.T) {
	stateDir := t.TempDir()
	profileDir := filepath.Join(stateDir, "character_profiles")
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		t.Fatalf("mkdir profile dir: %v", err)
	}
	path := filepath.Join(profileDir, "novel-1.yaml")
	writeFile(t, path, "characters: [")
	if _, ok, err := LoadSummary(stateDir, "novel-1", "1"); err != nil || ok {
		t.Fatalf("invalid derived profile should be quarantined, ok=%v err=%v", ok, err)
	}
	quarantined, err := filepath.Glob(path + ".unsupported-*")
	if err != nil || len(quarantined) != 1 {
		t.Fatalf("quarantined profiles = %v, err=%v", quarantined, err)
	}
	raw, err := os.ReadFile(quarantined[0])
	if err != nil || string(raw) != "characters: [" {
		t.Fatalf("quarantined bytes = %q, err=%v", raw, err)
	}
}

func TestLoadSummaryQuarantinesFutureProfileAndRebuildsFromEvents(t *testing.T) {
	stateDir := t.TempDir()
	if err := SaveGeneratedSummary(stateDir, "novel-future-profile", "1", []GeneratedCharacter{{
		CharacterID:                 "character-1",
		CanonicalName:               "合成人物",
		CanonicalEpisodeIndex:       "1",
		FirstAppearanceEpisodeIndex: "1",
	}}); err != nil {
		t.Fatalf("SaveGeneratedSummary: %v", err)
	}
	path := filepath.Join(stateDir, "character_profiles", "novel-future-profile.yaml")
	future := []byte("schema_version: 99\nnovel_id: novel-future-profile\ncharacters: []\n")
	if err := os.WriteFile(path, future, 0o644); err != nil {
		t.Fatalf("write future profile: %v", err)
	}

	summary, ok, err := LoadSummary(stateDir, "novel-future-profile", "1")
	if err != nil || !ok || len(summary.Characters) != 1 || summary.Characters[0].CanonicalName != "合成人物" {
		t.Fatalf("rebuilt summary = %+v, ok=%v err=%v", summary, ok, err)
	}
	quarantined, err := filepath.Glob(path + ".unsupported-*")
	if err != nil || len(quarantined) != 1 {
		t.Fatalf("quarantined profiles = %v, err=%v", quarantined, err)
	}
	raw, err := os.ReadFile(quarantined[0])
	if err != nil || string(raw) != string(future) {
		t.Fatalf("quarantined future bytes = %q, err=%v", raw, err)
	}
	var rebuilt profilesDocument
	if ok, _, err := readCharacterProfilesIfExists(path, &rebuilt); err != nil || !ok || rebuilt.SchemaVersion != characterProfilesSchemaVersion {
		t.Fatalf("rebuilt profile = %+v, ok=%v err=%v", rebuilt, ok, err)
	}
}

func TestCharacterEventsSchemaGuardRejectsMutationWithoutTouchingFile(t *testing.T) {
	stateDir := t.TempDir()
	eventsDir := filepath.Join(stateDir, "character_events")
	if err := os.MkdirAll(eventsDir, 0o755); err != nil {
		t.Fatalf("mkdir events: %v", err)
	}
	path := filepath.Join(eventsDir, "novel-future-events.yaml")
	if err := os.WriteFile(path, []byte("schema_version: 99\nnovel_id: novel-future-events\ncharacters: []\n"), 0o644); err != nil {
		t.Fatalf("write future events: %v", err)
	}
	err := schemaguardtest.AssertFileUntouched(t, path, func() error {
		return SaveGeneratedSummary(stateDir, "novel-future-events", "1", []GeneratedCharacter{{CanonicalName: "合成人物"}})
	})
	var guardError *schemaguard.GuardError
	if !errors.As(err, &guardError) || guardError.Result.Status != schemaguard.StatusFutureUnknown {
		t.Fatalf("error = %#v, want future GuardError", err)
	}
	if _, statErr := os.Stat(filepath.Join(stateDir, "character_profiles", "novel-future-events.yaml")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("profile should not be written after event rejection: %v", statErr)
	}
}

func TestCharacterEventsLegacyVersionZeroMigratesOnWrite(t *testing.T) {
	stateDir := t.TempDir()
	eventsDir := filepath.Join(stateDir, "character_events")
	if err := os.MkdirAll(eventsDir, 0o755); err != nil {
		t.Fatalf("mkdir events: %v", err)
	}
	path := filepath.Join(eventsDir, "novel-legacy-events.yaml")
	if err := os.WriteFile(path, []byte("novel_id: novel-legacy-events\nnext_character_ordinal: 1\ncharacters: []\n"), 0o644); err != nil {
		t.Fatalf("write legacy events: %v", err)
	}
	if err := SaveGeneratedSummary(stateDir, "novel-legacy-events", "1", []GeneratedCharacter{{CanonicalName: "合成人物"}}); err != nil {
		t.Fatalf("SaveGeneratedSummary: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(raw), "schema_version: 1") {
		t.Fatalf("migrated event bytes = %q, err=%v", raw, err)
	}
}
