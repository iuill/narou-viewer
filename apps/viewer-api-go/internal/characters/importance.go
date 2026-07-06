package characters

import (
	"math"
	"regexp"
	"sort"
	"strings"
)

func applyGeneratedImportanceMetrics(profiles []characterProfile, episodes []HeuristicEpisode) {
	if len(episodes) == 0 {
		return
	}
	fullNameHistoryFrequency := fullNameHistoryFrequencyByProfile(profiles)
	episodeExists := map[string]bool{}
	for _, episode := range episodes {
		episodeExists[episode.EpisodeIndex] = true
	}
	for index := range profiles {
		names := knownProfileNames(profiles[index], fullNameHistoryFrequency)
		pattern := knownNamePattern(names)
		mentions := []episodeMentionDoc{}
		if pattern != nil {
			for _, episode := range episodes {
				count := len(pattern.FindAllStringIndex(episode.Text, -1))
				if count > 0 {
					mentions = append(mentions, episodeMentionDoc{EpisodeIndex: episode.EpisodeIndex, Count: count})
				}
			}
		}
		if len(mentions) == 0 && episodeExists[profiles[index].FirstAppearanceEpisodeIndex] {
			mentions = append(mentions, episodeMentionDoc{EpisodeIndex: profiles[index].FirstAppearanceEpisodeIndex, Count: 1})
		}
		profiles[index].ImportanceMetrics = &importanceMetricsDoc{EpisodeMentions: mentions}
	}
}

func fullNameHistoryFrequencyByProfile(profiles []characterProfile) map[string]int {
	frequency := map[string]int{}
	for _, profile := range profiles {
		seen := map[string]bool{}
		for _, value := range profile.FullNameHistory {
			normalized := strings.ToLower(strings.TrimSpace(value.Text))
			if normalized == "" || seen[normalized] {
				continue
			}
			seen[normalized] = true
			frequency[normalized]++
		}
	}
	return frequency
}

func knownProfileNames(profile characterProfile, fullNameHistoryFrequency map[string]int) []string {
	seen := map[string]bool{}
	names := []string{}
	add := func(value string) {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" || seen[trimmed] {
			return
		}
		seen[trimmed] = true
		names = append(names, trimmed)
	}
	add(profile.CanonicalName.Text)
	if profile.FullName != nil {
		add(profile.FullName.Text)
	}
	for _, fullName := range profile.FullNameHistory {
		trimmed := strings.TrimSpace(fullName.Text)
		normalized := strings.ToLower(trimmed)
		if len([]rune(normalized)) < 2 || fullNameHistoryFrequency[normalized] != 1 {
			continue
		}
		add(trimmed)
	}
	for _, alias := range profile.Aliases {
		add(alias.Text)
	}
	sort.SliceStable(names, func(i, j int) bool {
		return len([]rune(names[i])) > len([]rune(names[j]))
	})
	return names
}

func knownNamePattern(names []string) *regexp.Regexp {
	if len(names) == 0 {
		return nil
	}
	quoted := make([]string, 0, len(names))
	for _, name := range names {
		quoted = append(quoted, regexp.QuoteMeta(name))
	}
	return regexp.MustCompile(strings.Join(quoted, "|"))
}

type importanceSnapshot struct {
	profile                 characterProfile
	score                   float64
	mentionCount            int
	mentionedEpisodeCount   int
	firstAppearancePosition int
}

func buildImportanceClassifications(profiles []characterProfile, episodeIndexes []string, upToEpisodeIndex string) map[string]any {
	episodePositionMap := buildEpisodePositionMap(episodeIndexes)
	totalEpisodesConsidered := totalEpisodesConsidered(episodeIndexes, upToEpisodeIndex, episodePositionMap)
	snapshots := []importanceSnapshot{}
	for _, profile := range profiles {
		if snapshot, ok := buildImportanceSnapshot(profile, upToEpisodeIndex, episodePositionMap, totalEpisodesConsidered); ok {
			snapshots = append(snapshots, snapshot)
		}
	}
	sort.SliceStable(snapshots, func(i, j int) bool {
		left := snapshots[i]
		right := snapshots[j]
		if right.score != left.score {
			return right.score < left.score
		}
		if right.mentionCount != left.mentionCount {
			return right.mentionCount < left.mentionCount
		}
		if left.firstAppearancePosition != right.firstAppearancePosition {
			return left.firstAppearancePosition < right.firstAppearancePosition
		}
		return left.profile.CanonicalName.Text < right.profile.CanonicalName.Text
	})
	mainLimit := max(1, int(math.Ceil(float64(len(snapshots))*0.15)))
	result := map[string]any{}
	for index, snapshot := range snapshots {
		category := "semi-regular"
		if snapshot.mentionedEpisodeCount > 1 && snapshot.score >= 0.55 && index < mainLimit {
			category = "main"
		} else if snapshot.score >= 0.2 && snapshot.mentionedEpisodeCount >= 2 {
			category = "regular"
		}
		result[snapshot.profile.CharacterID] = map[string]any{
			"category": category,
			"score":    math.Round(snapshot.score*1000) / 1000,
		}
	}
	return result
}

func buildImportanceSnapshot(profile characterProfile, upToEpisodeIndex string, episodePositionMap map[string]int, totalEpisodesConsidered int) (importanceSnapshot, bool) {
	firstAppearancePosition := episodePosition(profile.FirstAppearanceEpisodeIndex, episodePositionMap)
	if firstAppearancePosition <= 0 || totalEpisodesConsidered <= 0 {
		return importanceSnapshot{}, false
	}
	mentions := []episodeMentionDoc{}
	if profile.ImportanceMetrics != nil {
		for _, mention := range profile.ImportanceMetrics.EpisodeMentions {
			if compareEpisodeByOrder(mention.EpisodeIndex, upToEpisodeIndex, episodePositionMap) <= 0 {
				mentions = append(mentions, mention)
			}
		}
	}
	if len(mentions) == 0 {
		mentions = append(mentions, episodeMentionDoc{EpisodeIndex: profile.FirstAppearanceEpisodeIndex, Count: 1})
	}
	mentionCount := 0
	for _, mention := range mentions {
		mentionCount += mention.Count
	}
	lastMentionEpisodeIndex := mentions[len(mentions)-1].EpisodeIndex
	lastMentionPosition := episodePosition(lastMentionEpisodeIndex, episodePositionMap)
	if lastMentionPosition <= 0 {
		lastMentionPosition = firstAppearancePosition
	}
	summaryVersionCount := 0
	for _, entry := range profile.SummaryHistory {
		if compareEpisodeByOrder(entry.EpisodeIndex, upToEpisodeIndex, episodePositionMap) <= 0 {
			summaryVersionCount++
		}
	}
	mentionedEpisodeCount := len(mentions)
	episodeCoverageRatio := float64(mentionedEpisodeCount) / float64(totalEpisodesConsidered)
	mentionDensity := float64(mentionCount) / float64(totalEpisodesConsidered)
	normalizedMentionDensity := clamp01(mentionDensity / 8)
	recencyRatio := float64(lastMentionPosition) / float64(totalEpisodesConsidered)
	continuitySpanRatio := clamp01(float64(lastMentionPosition-firstAppearancePosition+1) / float64(totalEpisodesConsidered))
	summaryPresenceRatio := clamp01(float64(summaryVersionCount) / float64(max(1, mentionedEpisodeCount)))
	score := clamp01(episodeCoverageRatio*0.45 + normalizedMentionDensity*0.2 + recencyRatio*0.15 + continuitySpanRatio*0.1 + summaryPresenceRatio*0.1)
	return importanceSnapshot{
		profile:                 profile,
		score:                   score,
		mentionCount:            mentionCount,
		mentionedEpisodeCount:   mentionedEpisodeCount,
		firstAppearancePosition: firstAppearancePosition,
	}, true
}

func importanceOrder(value any) int {
	if value == nil {
		return 3
	}
	if asMap, ok := value.(map[string]any); ok {
		switch asMap["category"] {
		case "main":
			return 0
		case "regular":
			return 1
		case "semi-regular":
			return 2
		}
	}
	return 3
}

func clamp01(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}
