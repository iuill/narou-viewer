package characters

import (
	"sort"
	"strings"
)

func AssignGeneratedCharacterIDs(novelID string, existing []GeneratedCharacter, incoming []GeneratedCharacter) []GeneratedCharacter {
	return NewGeneratedCharacterIDAllocator(novelID, existing).Assign(incoming)
}

func normalizeGeneratedHistoryVersionList(values []GeneratedHistoryVersion) []GeneratedHistoryVersion {
	result := []GeneratedHistoryVersion{}
	seen := map[string]bool{}
	for _, value := range values {
		text := strings.TrimSpace(value.Text)
		episodeIndex := strings.TrimSpace(value.EpisodeIndex)
		key := episodeIndex + "\x00" + text
		if text == "" || episodeIndex == "" || seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, GeneratedHistoryVersion{EpisodeIndex: episodeIndex, Text: text})
	}
	sort.SliceStable(result, func(i, j int) bool {
		diff := compareEpisodeIndex(result[i].EpisodeIndex, result[j].EpisodeIndex)
		if diff != 0 {
			return diff < 0
		}
		return result[i].Text < result[j].Text
	})
	return result
}

func sortGeneratedCharacters(values []GeneratedCharacter) {
	sort.SliceStable(values, func(i, j int) bool {
		diff := compareEpisodeIndex(values[i].FirstAppearanceEpisodeIndex, values[j].FirstAppearanceEpisodeIndex)
		if diff != 0 {
			return diff < 0
		}
		return values[i].CanonicalName < values[j].CanonicalName
	})
}

func isValidGeneratedCharacterName(value string) bool {
	runes := []rune(value)
	runeCount := len(runes)
	if runeCount < 1 || runeCount > 80 {
		return false
	}
	if runeCount == 1 && runes[0] <= 127 {
		return false
	}
	switch value {
	case "それ", "これ", "そこ", "ここ", "ため", "よう", "もの", "こと":
		return false
	default:
		return true
	}
}
