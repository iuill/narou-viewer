package terms

import (
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"narou-viewer/apps/viewer-api-go/internal/fsatomic"
	"narou-viewer/apps/viewer-api-go/internal/novelstate"
	"narou-viewer/apps/viewer-api-go/internal/state/schemaguard"
	"narou-viewer/apps/viewer-api-go/internal/state/yamlfile"

	"gopkg.in/yaml.v3"
)

const schemaVersion = 1

var SchemaContract = schemaguard.Contract{
	ID:            "VA-TERM-PROFILES",
	Path:          "term_profiles/*.yaml",
	Current:       schemaVersion,
	MissingPolicy: schemaguard.MissingReject,
}

type document struct {
	SchemaVersion             int             `yaml:"schema_version"`
	NovelID                   string          `yaml:"novel_id"`
	ProcessedUpToEpisodeIndex string          `yaml:"processed_up_to_episode_index"`
	Terms                     []GeneratedTerm `yaml:"terms"`
}

func EnsureStateDirs(stateDir string) error {
	return os.MkdirAll(filepath.Join(stateDir, "term_profiles"), 0o755)
}

func SaveGeneratedTerms(stateDir string, novelID string, processedUpToEpisodeIndex string, incoming []GeneratedTerm, replaceFromEpisodeIndex *string) error {
	return novelstate.WithLock(novelID, func() error {
		existing, _, ok, err := loadGeneratedTermsUnlocked(stateDir, novelID)
		if err != nil {
			return err
		}
		if !ok {
			existing = []GeneratedTerm{}
		}
		if replaceFromEpisodeIndex != nil {
			existing = ReplaceFromEpisodeIndex(existing, *replaceFromEpisodeIndex)
		}
		merged := ApplyTermDelta(existing, incoming)
		doc := document{
			SchemaVersion:             schemaVersion,
			NovelID:                   strings.TrimSpace(novelID),
			ProcessedUpToEpisodeIndex: strings.TrimSpace(processedUpToEpisodeIndex),
			Terms:                     merged,
		}
		raw, err := yaml.Marshal(doc)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Join(stateDir, "term_profiles"), 0o755); err != nil {
			return err
		}
		return fsatomic.WriteFile(profilePath(stateDir, novelID), raw, 0o644)
	})
}

func LoadGeneratedTerms(stateDir string, novelID string) ([]GeneratedTerm, *string, bool, error) {
	return loadGeneratedTermsUnlocked(stateDir, novelID)
}

func PreflightPruneNovelState(stateDir string, novelID string) error {
	novelID = strings.TrimSpace(novelID)
	if novelID == "" {
		return nil
	}
	var doc document
	_, err := yamlfile.ReadGuarded(profilePath(stateDir, novelID), SchemaContract, &doc)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func loadGeneratedTermsUnlocked(stateDir string, novelID string) ([]GeneratedTerm, *string, bool, error) {
	var doc document
	_, err := yamlfile.ReadGuarded(profilePath(stateDir, novelID), SchemaContract, &doc)
	if errors.Is(err, os.ErrNotExist) {
		return []GeneratedTerm{}, nil, false, nil
	}
	if err != nil {
		return nil, nil, false, err
	}
	processed := strings.TrimSpace(doc.ProcessedUpToEpisodeIndex)
	return ApplyTermDelta(nil, doc.Terms), &processed, true, nil
}

func LoadGeneratedTermsAtOrBefore(stateDir string, novelID string, committedFrontier string) ([]GeneratedTerm, *string, bool, error) {
	generated, processed, ok, err := LoadGeneratedTerms(stateDir, novelID)
	if err != nil || !ok {
		return generated, processed, ok, err
	}
	visibilityCap := minEpisode(strings.TrimSpace(committedFrontier), valueOrEmpty(processed))
	projected := truncateAfter(generated, visibilityCap)
	return projected, stringPointer(visibilityCap), true, nil
}

func LoadGeneratedTermsBeforeEpisode(stateDir string, novelID string, episodeIndex string) ([]GeneratedTerm, *string, bool, error) {
	generated, processed, ok, err := LoadGeneratedTerms(stateDir, novelID)
	if err != nil || !ok {
		return generated, processed, ok, err
	}
	result := make([]GeneratedTerm, 0, len(generated))
	for _, term := range generated {
		term.ReadingHistory = filterTextVersions(term.ReadingHistory, func(index string) bool { return compareEpisode(index, episodeIndex) < 0 })
		term.CategoryHistory = filterCategoryVersions(term.CategoryHistory, func(index string) bool { return compareEpisode(index, episodeIndex) < 0 })
		term.DescriptionHistory = filterHistoryVersions(term.DescriptionHistory, func(index string) bool { return compareEpisode(index, episodeIndex) < 0 })
		term.DescriptionFacts = filterHistoryVersions(term.DescriptionFacts, func(index string) bool { return compareEpisode(index, episodeIndex) < 0 })
		if len(term.DescriptionHistory) > 0 || len(term.DescriptionFacts) > 0 {
			result = append(result, term)
		}
	}
	return result, processed, true, nil
}

func ApplyTermDelta(existing []GeneratedTerm, incoming []GeneratedTerm) []GeneratedTerm {
	byTerm := make(map[string]GeneratedTerm, len(existing)+len(incoming))
	order := make([]string, 0, len(existing)+len(incoming))
	for _, candidate := range append(append([]GeneratedTerm{}, existing...), incoming...) {
		key := strings.TrimSpace(candidate.Term)
		if key == "" {
			continue
		}
		current, found := byTerm[key]
		if !found {
			current = GeneratedTerm{Term: key}
			order = append(order, key)
		}
		current.Term = key
		current.ReadingHistory = mergeTextVersions(current.ReadingHistory, candidate.ReadingHistory)
		current.CategoryHistory = mergeCategoryVersions(current.CategoryHistory, candidate.CategoryHistory)
		current.DescriptionHistory = mergeHistoryVersions(current.DescriptionHistory, candidate.DescriptionHistory)
		current.DescriptionFacts = mergeFactVersions(current.DescriptionFacts, candidate.DescriptionFacts)
		byTerm[key] = current
	}
	result := make([]GeneratedTerm, 0, len(byTerm))
	for _, key := range order {
		term := byTerm[key]
		if len(term.DescriptionHistory) > 0 || len(term.DescriptionFacts) > 0 {
			result = append(result, term)
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		left := firstDescriptionEpisode(result[i])
		right := firstDescriptionEpisode(result[j])
		if compared := compareEpisode(left, right); compared != 0 {
			return compared < 0
		}
		return result[i].Term < result[j].Term
	})
	return result
}

// ApplyParallelTermFacts stores episode-local facts without expanding every
// intermediate cumulative snapshot. Projection composes only the requested
// boundary's display text.
func ApplyParallelTermFacts(existing []GeneratedTerm, incoming []GeneratedTerm) []GeneratedTerm {
	facts := CombineTermFacts(incoming)
	for index := range facts {
		facts[index].DescriptionFacts = facts[index].DescriptionHistory
		facts[index].DescriptionHistory = nil
	}
	return ApplyTermDelta(existing, facts)
}

func CombineTermFacts(incoming []GeneratedTerm) []GeneratedTerm {
	byTerm := map[string]GeneratedTerm{}
	order := []string{}
	for _, candidate := range incoming {
		key := strings.TrimSpace(candidate.Term)
		if key == "" {
			continue
		}
		current, found := byTerm[key]
		if !found {
			current = GeneratedTerm{Term: key}
			order = append(order, key)
		}
		current.ReadingHistory = mergeTextVersions(current.ReadingHistory, candidate.ReadingHistory)
		current.CategoryHistory = mergeCategoryVersions(current.CategoryHistory, candidate.CategoryHistory)
		descriptions := map[string]string{}
		for _, history := range current.DescriptionHistory {
			descriptions[strings.TrimSpace(history.EpisodeIndex)] = strings.TrimSpace(history.Text)
		}
		for _, history := range candidate.DescriptionHistory {
			episodeIndex := strings.TrimSpace(history.EpisodeIndex)
			if episodeIndex != "" {
				descriptions[episodeIndex] = mergeDescriptionSnapshot(descriptions[episodeIndex], history.Text)
			}
		}
		current.DescriptionHistory = current.DescriptionHistory[:0]
		for episodeIndex, text := range descriptions {
			if text != "" {
				current.DescriptionHistory = append(current.DescriptionHistory, HistoryVersion{Text: text, EpisodeIndex: episodeIndex})
			}
		}
		sort.Slice(current.DescriptionHistory, func(i, j int) bool {
			return compareEpisode(current.DescriptionHistory[i].EpisodeIndex, current.DescriptionHistory[j].EpisodeIndex) < 0
		})
		byTerm[key] = current
	}
	result := make([]GeneratedTerm, 0, len(order))
	for _, key := range order {
		result = append(result, byTerm[key])
	}
	return result
}

func mergeDescriptionSnapshot(previous string, current string) string {
	previous = strings.TrimSpace(previous)
	current = strings.TrimSpace(current)
	switch {
	case previous == "":
		return current
	case current == "":
		return previous
	case strings.Contains(current, previous):
		return current
	case strings.Contains(previous, current):
		return previous
	default:
		return previous + " " + current
	}
}

func ReplaceFromEpisodeIndex(generated []GeneratedTerm, fromEpisodeIndex string) []GeneratedTerm {
	result := make([]GeneratedTerm, 0, len(generated))
	for _, term := range generated {
		term.ReadingHistory = filterTextVersions(term.ReadingHistory, func(index string) bool { return compareEpisode(index, fromEpisodeIndex) < 0 })
		term.CategoryHistory = filterCategoryVersions(term.CategoryHistory, func(index string) bool { return compareEpisode(index, fromEpisodeIndex) < 0 })
		term.DescriptionHistory = filterHistoryVersions(term.DescriptionHistory, func(index string) bool { return compareEpisode(index, fromEpisodeIndex) < 0 })
		term.DescriptionFacts = filterHistoryVersions(term.DescriptionFacts, func(index string) bool { return compareEpisode(index, fromEpisodeIndex) < 0 })
		if len(term.DescriptionHistory) > 0 || len(term.DescriptionFacts) > 0 {
			result = append(result, term)
		}
	}
	return result
}

func ProjectTerms(generated []GeneratedTerm, boundary string) []Term {
	type projectedTerm struct {
		term       Term
		firstIndex string
	}
	projected := make([]projectedTerm, 0, len(generated))
	for _, generatedTerm := range generated {
		descriptions := filterHistoryVersions(generatedTerm.DescriptionHistory, func(index string) bool { return compareEpisode(index, boundary) <= 0 })
		facts := filterHistoryVersions(generatedTerm.DescriptionFacts, func(index string) bool { return compareEpisode(index, boundary) <= 0 })
		if len(descriptions) == 0 && len(facts) == 0 {
			continue
		}
		description := ""
		baseEpisode := ""
		if len(descriptions) > 0 {
			description = descriptions[len(descriptions)-1].Text
			baseEpisode = descriptions[len(descriptions)-1].EpisodeIndex
		}
		for _, fact := range facts {
			if baseEpisode == "" || compareEpisode(fact.EpisodeIndex, baseEpisode) > 0 {
				description = mergeDescriptionSnapshot(description, fact.Text)
			}
		}
		term := Term{Term: strings.TrimSpace(generatedTerm.Term), Category: CategoryOther, Description: description}
		readings := filterTextVersions(generatedTerm.ReadingHistory, func(index string) bool { return compareEpisode(index, boundary) <= 0 })
		if len(readings) > 0 {
			reading := readings[len(readings)-1].Text
			term.Reading = &reading
		}
		categories := filterCategoryVersions(generatedTerm.CategoryHistory, func(index string) bool { return compareEpisode(index, boundary) <= 0 })
		if len(categories) > 0 {
			term.Category = NormalizeCategory(categories[len(categories)-1].Category)
		}
		projected = append(projected, projectedTerm{term: term, firstIndex: firstDescriptionEpisode(generatedTerm)})
	}
	sort.SliceStable(projected, func(i, j int) bool {
		if compared := compareEpisode(projected[i].firstIndex, projected[j].firstIndex); compared != 0 {
			return compared < 0
		}
		return projected[i].term.Term < projected[j].term.Term
	})
	result := make([]Term, len(projected))
	for index := range projected {
		result[index] = projected[index].term
	}
	return result
}

func BuildResponse(stateDir string, novelID string, requestedBoundary string, committedFrontier string) (TermsResponse, error) {
	generated, termFrontier, ok, err := LoadGeneratedTerms(stateDir, novelID)
	if err != nil {
		return TermsResponse{}, err
	}
	response := TermsResponse{Status: "not_generated", NovelID: novelID, UpToEpisodeIndex: requestedBoundary, Terms: []Term{}}
	if !ok || termFrontier == nil || strings.TrimSpace(committedFrontier) == "" {
		return response, nil
	}
	effectiveFrontier := minEpisode(*termFrontier, committedFrontier)
	response.ProcessedUpToEpisodeIndex = stringPointer(effectiveFrontier)
	visibilityCap := minEpisode(requestedBoundary, effectiveFrontier)
	response.Terms = ProjectTerms(generated, visibilityCap)
	if compareEpisode(effectiveFrontier, requestedBoundary) >= 0 {
		response.Status = "ready"
	} else {
		response.Status = "partial"
	}
	return response, nil
}

func truncateAfter(generated []GeneratedTerm, boundary string) []GeneratedTerm {
	result := make([]GeneratedTerm, 0, len(generated))
	for _, term := range generated {
		term.ReadingHistory = filterTextVersions(term.ReadingHistory, func(index string) bool { return compareEpisode(index, boundary) <= 0 })
		term.CategoryHistory = filterCategoryVersions(term.CategoryHistory, func(index string) bool { return compareEpisode(index, boundary) <= 0 })
		term.DescriptionHistory = filterHistoryVersions(term.DescriptionHistory, func(index string) bool { return compareEpisode(index, boundary) <= 0 })
		term.DescriptionFacts = filterHistoryVersions(term.DescriptionFacts, func(index string) bool { return compareEpisode(index, boundary) <= 0 })
		if len(term.DescriptionHistory) > 0 || len(term.DescriptionFacts) > 0 {
			result = append(result, term)
		}
	}
	return result
}

func mergeTextVersions(existing []TextVersion, incoming []TextVersion) []TextVersion {
	byEpisode := map[string]TextVersion{}
	for _, version := range append(append([]TextVersion{}, existing...), incoming...) {
		version.Text = strings.TrimSpace(version.Text)
		version.EpisodeIndex = strings.TrimSpace(version.EpisodeIndex)
		if version.Text != "" && version.EpisodeIndex != "" {
			byEpisode[version.EpisodeIndex] = version
		}
	}
	result := make([]TextVersion, 0, len(byEpisode))
	for _, version := range byEpisode {
		result = append(result, version)
	}
	sort.Slice(result, func(i, j int) bool { return compareEpisode(result[i].EpisodeIndex, result[j].EpisodeIndex) < 0 })
	return result
}

func mergeCategoryVersions(existing []CategoryVersion, incoming []CategoryVersion) []CategoryVersion {
	byEpisode := map[string]CategoryVersion{}
	for _, version := range append(append([]CategoryVersion{}, existing...), incoming...) {
		version.EpisodeIndex = strings.TrimSpace(version.EpisodeIndex)
		if version.EpisodeIndex != "" {
			version.Category = NormalizeCategory(strings.TrimSpace(version.Category))
			byEpisode[version.EpisodeIndex] = version
		}
	}
	result := make([]CategoryVersion, 0, len(byEpisode))
	for _, version := range byEpisode {
		result = append(result, version)
	}
	sort.Slice(result, func(i, j int) bool { return compareEpisode(result[i].EpisodeIndex, result[j].EpisodeIndex) < 0 })
	return result
}

func mergeHistoryVersions(existing []HistoryVersion, incoming []HistoryVersion) []HistoryVersion {
	byEpisode := map[string]HistoryVersion{}
	for _, version := range append(append([]HistoryVersion{}, existing...), incoming...) {
		version.Text = strings.TrimSpace(version.Text)
		version.EpisodeIndex = strings.TrimSpace(version.EpisodeIndex)
		if version.Text != "" && version.EpisodeIndex != "" {
			byEpisode[version.EpisodeIndex] = version
		}
	}
	result := make([]HistoryVersion, 0, len(byEpisode))
	for _, version := range byEpisode {
		result = append(result, version)
	}
	sort.Slice(result, func(i, j int) bool { return compareEpisode(result[i].EpisodeIndex, result[j].EpisodeIndex) < 0 })
	return result
}

func mergeFactVersions(existing []HistoryVersion, incoming []HistoryVersion) []HistoryVersion {
	byEpisode := map[string]HistoryVersion{}
	for _, version := range append(append([]HistoryVersion{}, existing...), incoming...) {
		version.Text = strings.TrimSpace(version.Text)
		version.EpisodeIndex = strings.TrimSpace(version.EpisodeIndex)
		if version.Text == "" || version.EpisodeIndex == "" {
			continue
		}
		current := byEpisode[version.EpisodeIndex]
		current.EpisodeIndex = version.EpisodeIndex
		current.Text = mergeDescriptionSnapshot(current.Text, version.Text)
		byEpisode[version.EpisodeIndex] = current
	}
	result := make([]HistoryVersion, 0, len(byEpisode))
	for _, version := range byEpisode {
		result = append(result, version)
	}
	sort.Slice(result, func(i, j int) bool { return compareEpisode(result[i].EpisodeIndex, result[j].EpisodeIndex) < 0 })
	return result
}

func firstDescriptionEpisode(term GeneratedTerm) string {
	first := ""
	for _, versions := range [][]HistoryVersion{term.DescriptionHistory, term.DescriptionFacts} {
		for _, version := range versions {
			if first == "" || compareEpisode(version.EpisodeIndex, first) < 0 {
				first = version.EpisodeIndex
			}
		}
	}
	return first
}

func filterTextVersions(versions []TextVersion, keep func(string) bool) []TextVersion {
	result := make([]TextVersion, 0, len(versions))
	for _, version := range versions {
		if keep(version.EpisodeIndex) {
			result = append(result, version)
		}
	}
	return result
}

func filterCategoryVersions(versions []CategoryVersion, keep func(string) bool) []CategoryVersion {
	result := make([]CategoryVersion, 0, len(versions))
	for _, version := range versions {
		if keep(version.EpisodeIndex) {
			result = append(result, version)
		}
	}
	return result
}

func filterHistoryVersions(versions []HistoryVersion, keep func(string) bool) []HistoryVersion {
	result := make([]HistoryVersion, 0, len(versions))
	for _, version := range versions {
		if keep(version.EpisodeIndex) {
			result = append(result, version)
		}
	}
	return result
}

func compareEpisode(left string, right string) int {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	leftNumber, leftOK := new(big.Int).SetString(left, 10)
	rightNumber, rightOK := new(big.Int).SetString(right, 10)
	if leftOK && rightOK {
		return leftNumber.Cmp(rightNumber)
	}
	return strings.Compare(left, right)
}

func minEpisode(left string, right string) string {
	if strings.TrimSpace(left) == "" {
		return strings.TrimSpace(right)
	}
	if strings.TrimSpace(right) == "" || compareEpisode(left, right) <= 0 {
		return strings.TrimSpace(left)
	}
	return strings.TrimSpace(right)
}

func profilePath(stateDir string, novelID string) string {
	return filepath.Join(stateDir, "term_profiles", strings.TrimSpace(novelID)+".yaml")
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func stringPointer(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
