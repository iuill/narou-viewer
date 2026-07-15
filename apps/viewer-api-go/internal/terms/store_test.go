package terms

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"narou-viewer/apps/viewer-api-go/internal/state/schemaguard"
	"narou-viewer/apps/viewer-api-go/internal/state/schemaguardtest"
)

func TestSaveAndLoadGeneratedTermsRoundTripAndEmptyState(t *testing.T) {
	stateDir := t.TempDir()
	if err := EnsureStateDirs(stateDir); err != nil {
		t.Fatalf("EnsureStateDirs returned error: %v", err)
	}
	if _, _, ok, err := LoadGeneratedTerms(stateDir, "novel-1"); err != nil || ok {
		t.Fatalf("missing state should return ok=false: ok=%v err=%v", ok, err)
	}
	generated := []GeneratedTerm{{
		Term:               " 魔導院 ",
		ReadingHistory:     []TextVersion{{Text: "まどういん", EpisodeIndex: "12"}},
		CategoryHistory:    []CategoryVersion{{Category: CategoryOrganization, EpisodeIndex: "12"}},
		DescriptionHistory: []HistoryVersion{{Text: "王都にある教育機関。", EpisodeIndex: "12"}},
	}}
	if err := SaveGeneratedTerms(stateDir, "novel-1", "42", generated, nil); err != nil {
		t.Fatalf("SaveGeneratedTerms returned error: %v", err)
	}
	loaded, frontier, ok, err := LoadGeneratedTerms(stateDir, "novel-1")
	if err != nil || !ok || frontier == nil || *frontier != "42" || len(loaded) != 1 {
		t.Fatalf("unexpected roundtrip: loaded=%+v frontier=%v ok=%v err=%v", loaded, frontier, ok, err)
	}
	if loaded[0].Term != "魔導院" || loaded[0].ReadingHistory[0].Text != "まどういん" {
		t.Fatalf("roundtrip did not normalize the term: %+v", loaded[0])
	}

	if err := SaveGeneratedTerms(stateDir, "empty", "5", nil, nil); err != nil {
		t.Fatalf("save empty terms: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(stateDir, "term_profiles", "empty.yaml"))
	if err != nil || !strings.Contains(string(raw), "schema_version: 1") || !strings.Contains(string(raw), "terms: []") {
		t.Fatalf("empty generated state should persist terms: []: raw=%s err=%v", raw, err)
	}
	response, err := BuildResponse(stateDir, "empty", "5", "5")
	if err != nil || response.Status != "ready" || len(response.Terms) != 0 {
		t.Fatalf("empty generated state should be ready: %+v err=%v", response, err)
	}
}

func TestApplyTermDeltaUsesExactTrimmedIdentityAndIncomingWinsSameEpisode(t *testing.T) {
	existing := []GeneratedTerm{{
		Term:               "魔導院",
		ReadingHistory:     []TextVersion{{Text: "旧読み", EpisodeIndex: "2"}},
		CategoryHistory:    []CategoryVersion{{Category: CategoryPlace, EpisodeIndex: "2"}},
		DescriptionHistory: []HistoryVersion{{Text: "旧説明", EpisodeIndex: "2"}},
	}}
	incoming := []GeneratedTerm{
		{
			Term:               " 魔導院 ",
			ReadingHistory:     []TextVersion{{Text: "まどういん", EpisodeIndex: "2"}},
			CategoryHistory:    []CategoryVersion{{Category: "unknown", EpisodeIndex: "2"}},
			DescriptionHistory: []HistoryVersion{{Text: "新説明", EpisodeIndex: "2"}},
		},
		{Term: "魔導 院", DescriptionHistory: []HistoryVersion{{Text: "別語", EpisodeIndex: "3"}}},
	}
	merged := ApplyTermDelta(existing, incoming)
	if len(merged) != 2 {
		t.Fatalf("exact identity should keep internal-space variant separate: %+v", merged)
	}
	if merged[0].ReadingHistory[0].Text != "まどういん" || merged[0].CategoryHistory[0].Category != CategoryOther || merged[0].DescriptionHistory[0].Text != "新説明" {
		t.Fatalf("incoming same-episode values should win: %+v", merged[0])
	}
}

func TestApplyParallelTermFactsStoresFactsAndProjectsCumulativeDescription(t *testing.T) {
	existing := []GeneratedTerm{{
		Term:               "白銀騎士団",
		DescriptionHistory: []HistoryVersion{{Text: "王都直属の騎士団。", EpisodeIndex: "1"}},
	}}
	incoming := []GeneratedTerm{
		{Term: "白銀騎士団", DescriptionHistory: []HistoryVersion{{Text: "団長はアリス。", EpisodeIndex: "10"}}},
		{Term: "白銀騎士団", DescriptionHistory: []HistoryVersion{{Text: "辺境の村へ派遣された。", EpisodeIndex: "2"}}},
	}

	merged := ApplyParallelTermFacts(existing, incoming)
	if len(merged) != 1 || len(merged[0].DescriptionHistory) != 1 || len(merged[0].DescriptionFacts) != 2 {
		t.Fatalf("parallel facts should be stored without repeated snapshots: %+v", merged)
	}
	if got := ProjectTerms(merged, "2")[0].Description; got != "王都直属の騎士団。 辺境の村へ派遣された。" {
		t.Fatalf("episode 2 projection = %q", got)
	}
	if got := ProjectTerms(merged, "10")[0].Description; got != "王都直属の騎士団。 辺境の村へ派遣された。 団長はアリス。" {
		t.Fatalf("episode 10 projection = %q", got)
	}
}

func TestApplyParallelTermFactsKeepsFactsFromSplitChunksInSameEpisode(t *testing.T) {
	merged := ApplyParallelTermFacts(nil, []GeneratedTerm{
		{Term: "魔導院", DescriptionHistory: []HistoryVersion{{Text: "王都にある。", EpisodeIndex: "5"}}},
		{Term: "魔導院", DescriptionHistory: []HistoryVersion{{Text: "魔術師を育成する。", EpisodeIndex: "5"}}},
	})
	if len(merged) != 1 || len(merged[0].DescriptionFacts) != 1 || len(merged[0].DescriptionHistory) != 0 {
		t.Fatalf("same-episode facts should share one compact fact record: %+v", merged)
	}
	if got := merged[0].DescriptionFacts[0].Text; got != "王都にある。 魔術師を育成する。" {
		t.Fatalf("same-episode fact = %q", got)
	}
}

func TestApplyParallelTermFactsPreservesExistingRewordedSnapshots(t *testing.T) {
	existing := []GeneratedTerm{{
		Term: "白銀騎士団",
		DescriptionHistory: []HistoryVersion{
			{Text: "王都直属の騎士団。", EpisodeIndex: "1"},
			{Text: "王都直属で辺境防衛も担う騎士団。", EpisodeIndex: "2"},
		},
	}}
	merged := ApplyParallelTermFacts(existing, []GeneratedTerm{{
		Term:               "白銀騎士団",
		DescriptionHistory: []HistoryVersion{{Text: "団長はアリス。", EpisodeIndex: "3"}},
	}})
	if got := merged[0].DescriptionHistory[1].Text; got != "王都直属で辺境防衛も担う騎士団。" {
		t.Fatalf("existing snapshot must not be recomposed: %q", got)
	}
	if got := ProjectTerms(merged, "3")[0].Description; got != "王都直属で辺境防衛も担う騎士団。 団長はアリス。" {
		t.Fatalf("new fact should extend latest existing snapshot: %q", got)
	}
}

func TestReplaceFromEpisodeIndexTruncatesEveryHistoryAndDropsDescriptionlessTerms(t *testing.T) {
	generated := []GeneratedTerm{
		{
			Term:               "魔導院",
			ReadingHistory:     []TextVersion{{Text: "old", EpisodeIndex: "1"}, {Text: "new", EpisodeIndex: "3"}},
			CategoryHistory:    []CategoryVersion{{Category: CategoryPlace, EpisodeIndex: "1"}, {Category: CategoryOrganization, EpisodeIndex: "3"}},
			DescriptionHistory: []HistoryVersion{{Text: "old", EpisodeIndex: "1"}, {Text: "new", EpisodeIndex: "3"}},
		},
		{Term: "後発語", DescriptionHistory: []HistoryVersion{{Text: "only", EpisodeIndex: "3"}}},
	}
	truncated := ReplaceFromEpisodeIndex(generated, "3")
	if len(truncated) != 1 || len(truncated[0].ReadingHistory) != 1 || len(truncated[0].CategoryHistory) != 1 || len(truncated[0].DescriptionHistory) != 1 {
		t.Fatalf("replace boundary did not truncate every history: %+v", truncated)
	}
}

func TestProjectionKeepsReadingCategoryAndDescriptionInsideBoundary(t *testing.T) {
	generated := []GeneratedTerm{{
		Term:               "魔導院",
		ReadingHistory:     []TextVersion{{Text: "まどういん", EpisodeIndex: "2"}, {Text: "未来読み", EpisodeIndex: "4"}},
		CategoryHistory:    []CategoryVersion{{Category: CategoryPlace, EpisodeIndex: "2"}, {Category: CategoryOrganization, EpisodeIndex: "4"}},
		DescriptionHistory: []HistoryVersion{{Text: "初期説明", EpisodeIndex: "2"}, {Text: "未来説明", EpisodeIndex: "4"}},
	}}
	atThree := ProjectTerms(generated, "3")
	if len(atThree) != 1 || atThree[0].Reading == nil || *atThree[0].Reading != "まどういん" || atThree[0].Category != CategoryPlace || atThree[0].Description != "初期説明" {
		t.Fatalf("future values leaked into projection: %+v", atThree)
	}
	if projected := ProjectTerms(generated, "1"); len(projected) != 0 {
		t.Fatalf("term without description at boundary should be hidden: %+v", projected)
	}
}

func TestBuildResponseCapsTermsAtCharacterCommitFrontier(t *testing.T) {
	stateDir := t.TempDir()
	generated := []GeneratedTerm{{
		Term:               "魔導院",
		ReadingHistory:     []TextVersion{{Text: "未来読み", EpisodeIndex: "4"}},
		CategoryHistory:    []CategoryVersion{{Category: CategoryOrganization, EpisodeIndex: "4"}},
		DescriptionHistory: []HistoryVersion{{Text: "初期説明", EpisodeIndex: "2"}, {Text: "未来説明", EpisodeIndex: "4"}},
	}}
	if err := SaveGeneratedTerms(stateDir, "novel-1", "4", generated, nil); err != nil {
		t.Fatalf("SaveGeneratedTerms returned error: %v", err)
	}
	response, err := BuildResponse(stateDir, "novel-1", "4", "2")
	if err != nil || response.Status != "partial" || response.ProcessedUpToEpisodeIndex == nil || *response.ProcessedUpToEpisodeIndex != "2" || len(response.Terms) != 1 {
		t.Fatalf("unexpected capped response: %+v err=%v", response, err)
	}
	if response.Terms[0].Reading != nil || response.Terms[0].Category != CategoryOther || response.Terms[0].Description != "初期説明" {
		t.Fatalf("orphan future term history leaked past character frontier: %+v", response.Terms[0])
	}
}

func TestLoadGeneratedTermsBoundaryHelpers(t *testing.T) {
	stateDir := t.TempDir()
	generated := []GeneratedTerm{{
		Term:               "語",
		ReadingHistory:     []TextVersion{{Text: "ご", EpisodeIndex: "2"}, {Text: "ごう", EpisodeIndex: "10"}},
		CategoryHistory:    []CategoryVersion{{Category: CategoryOther, EpisodeIndex: "2"}, {Category: CategoryEvent, EpisodeIndex: "10"}},
		DescriptionHistory: []HistoryVersion{{Text: "two", EpisodeIndex: "2"}, {Text: "ten", EpisodeIndex: "10"}},
	}}
	if err := SaveGeneratedTerms(stateDir, "novel-1", "10", generated, nil); err != nil {
		t.Fatalf("SaveGeneratedTerms returned error: %v", err)
	}
	atOrBefore, frontier, ok, err := LoadGeneratedTermsAtOrBefore(stateDir, "novel-1", "2")
	if err != nil || !ok || frontier == nil || *frontier != "2" || len(atOrBefore) != 1 || len(atOrBefore[0].DescriptionHistory) != 1 {
		t.Fatalf("unexpected at-or-before result: %+v frontier=%v ok=%v err=%v", atOrBefore, frontier, ok, err)
	}
	before, _, ok, err := LoadGeneratedTermsBeforeEpisode(stateDir, "novel-1", "10")
	if err != nil || !ok || len(before) != 1 || len(before[0].ReadingHistory) != 1 || before[0].DescriptionHistory[0].Text != "two" {
		t.Fatalf("unexpected before result: %+v ok=%v err=%v", before, ok, err)
	}
}

func TestNormalizeCategory(t *testing.T) {
	for _, category := range []string{CategoryOrganization, CategoryPlace, CategoryItem, CategorySkill, CategoryRace, CategoryEvent, CategoryOther} {
		if NormalizeCategory(category) != category {
			t.Fatalf("valid category changed: %s", category)
		}
	}
	if NormalizeCategory("unknown") != CategoryOther {
		t.Fatal("unknown category should normalize to other")
	}
}

func TestMalformedGeneratedTermsPropagatesThroughReadAPIs(t *testing.T) {
	stateDir := t.TempDir()
	if err := EnsureStateDirs(stateDir); err != nil {
		t.Fatalf("EnsureStateDirs returned error: %v", err)
	}
	if err := os.WriteFile(profilePath(stateDir, "broken"), []byte("schema_version: ["), 0o644); err != nil {
		t.Fatalf("write malformed profile: %v", err)
	}
	if _, _, _, err := LoadGeneratedTerms(stateDir, "broken"); err == nil {
		t.Fatal("LoadGeneratedTerms should reject malformed YAML")
	}
	if _, _, _, err := LoadGeneratedTermsAtOrBefore(stateDir, "broken", "1"); err == nil {
		t.Fatal("LoadGeneratedTermsAtOrBefore should propagate malformed YAML")
	}
	if _, _, _, err := LoadGeneratedTermsBeforeEpisode(stateDir, "broken", "1"); err == nil {
		t.Fatal("LoadGeneratedTermsBeforeEpisode should propagate malformed YAML")
	}
	if _, err := BuildResponse(stateDir, "broken", "1", "1"); err == nil {
		t.Fatal("BuildResponse should propagate malformed YAML")
	}
}

func TestTermProfilesSchemaGuardRejectsMutationWithoutTouchingFile(t *testing.T) {
	tests := []struct {
		name       string
		document   string
		wantStatus schemaguard.Status
	}{
		{name: "future", document: "schema_version: 99\nnovel_id: guarded\nterms: []\n", wantStatus: schemaguard.StatusFutureUnknown},
		{name: "missing", document: "novel_id: guarded\nterms: []\n", wantStatus: schemaguard.StatusUnsupportedLegacy},
		{name: "malformed", document: "schema_version: 1\nterms: invalid\n", wantStatus: schemaguard.StatusMalformed},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stateDir := t.TempDir()
			if err := EnsureStateDirs(stateDir); err != nil {
				t.Fatalf("EnsureStateDirs: %v", err)
			}
			path := profilePath(stateDir, "guarded")
			if err := os.WriteFile(path, []byte(test.document), 0o644); err != nil {
				t.Fatalf("write fixture: %v", err)
			}
			err := schemaguardtest.AssertFileUntouched(t, path, func() error {
				return SaveGeneratedTerms(stateDir, "guarded", "1", []GeneratedTerm{{
					Term:               "合成語",
					DescriptionHistory: []HistoryVersion{{Text: "合成説明", EpisodeIndex: "1"}},
				}}, nil)
			})
			var guardError *schemaguard.GuardError
			if !errors.As(err, &guardError) || guardError.Result.Status != test.wantStatus {
				t.Fatalf("error = %#v, want GuardError status %s", err, test.wantStatus)
			}
		})
	}
}

func TestTermStoreBoundaryHelperEdges(t *testing.T) {
	if got := compareEpisode("alpha", "beta"); got >= 0 {
		t.Fatalf("compareEpisode fallback = %d, want negative", got)
	}
	if got := minEpisode("", " 2 "); got != "2" {
		t.Fatalf("minEpisode empty left = %q", got)
	}
	if got := minEpisode("2", ""); got != "2" {
		t.Fatalf("minEpisode empty right = %q", got)
	}
	if valueOrEmpty(nil) != "" {
		t.Fatal("valueOrEmpty(nil) should be empty")
	}
	if stringPointer("  ") != nil {
		t.Fatal("stringPointer(blank) should be nil")
	}
}
