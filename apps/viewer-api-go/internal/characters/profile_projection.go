package characters

import "strings"

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func (p characterProfile) toCharacter(upToEpisodeIndex string, importance any) Character {
	displayName := p.displayName(upToEpisodeIndex)
	return Character{
		CharacterID:                 p.CharacterID,
		CanonicalName:               displayName,
		FullName:                    latestTextValuePtr(p.FullNameHistory, p.FullName, upToEpisodeIndex),
		Gender:                      latestTextValuePtr(p.GenderHistory, p.Gender, upToEpisodeIndex),
		FirstAppearanceEpisodeIndex: p.FirstAppearanceEpisodeIndex,
		Aliases:                     aliases(p.Aliases, upToEpisodeIndex),
		Appearance:                  latestHistoryText(p.AppearanceHistory, upToEpisodeIndex),
		Personality:                 latestHistoryText(p.PersonalityHistory, upToEpisodeIndex),
		Summary:                     latestHistoryText(p.SummaryHistory, upToEpisodeIndex),
		Importance:                  importance,
	}
}

func (p characterProfile) displayName(upToEpisodeIndex string) string {
	var latest *textVersion
	candidates := append([]textVersion{}, p.PreferredNames...)
	canonicalIndex := len(candidates)
	candidates = append(candidates, p.CanonicalName)
	for index := range candidates {
		value := &candidates[index]
		if value.Text == "" || !episodeWithin(value.EpisodeIndex, upToEpisodeIndex) {
			continue
		}
		if latest == nil || compareEpisodeIndex(value.EpisodeIndex, latest.EpisodeIndex) > 0 || (compareEpisodeIndex(value.EpisodeIndex, latest.EpisodeIndex) == 0 && index == canonicalIndex) {
			latest = value
		}
	}
	if latest != nil {
		return latest.Text
	}
	for _, alias := range p.Aliases {
		if alias.Text != "" && episodeWithin(alias.EpisodeIndex, upToEpisodeIndex) {
			return alias.Text
		}
	}
	return p.CanonicalName.Text
}

func textValuePtr(value *textVersion, upToEpisodeIndex string) *string {
	if value == nil || value.Text == "" || !episodeWithin(value.EpisodeIndex, upToEpisodeIndex) {
		return nil
	}
	return &value.Text
}

func latestTextValuePtr(history []textVersion, fallback *textVersion, upToEpisodeIndex string) *string {
	versions := append([]textVersion{}, history...)
	versions = append(versions, derefTextVersion(fallback)...)
	var latest *textVersion
	for index := range versions {
		value := &versions[index]
		if value.Text == "" || !episodeWithin(value.EpisodeIndex, upToEpisodeIndex) {
			continue
		}
		if latest == nil || compareEpisodeIndex(value.EpisodeIndex, latest.EpisodeIndex) > 0 {
			latest = value
		}
	}
	if latest == nil {
		return nil
	}
	return &latest.Text
}

func latestTextVersion(values []textVersion) *textVersion {
	var latest *textVersion
	for index := range values {
		value := &values[index]
		if strings.TrimSpace(value.Text) == "" || strings.TrimSpace(value.EpisodeIndex) == "" {
			continue
		}
		if latest == nil || compareEpisodeIndex(value.EpisodeIndex, latest.EpisodeIndex) > 0 {
			latest = value
		}
	}
	if latest == nil {
		return nil
	}
	result := *latest
	return &result
}

func derefTextVersion(value *textVersion) []textVersion {
	if value == nil {
		return nil
	}
	return []textVersion{*value}
}

func aliases(values []textVersion, upToEpisodeIndex string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		text := strings.TrimSpace(value.Text)
		if text == "" || !episodeWithin(value.EpisodeIndex, upToEpisodeIndex) || seen[text] {
			continue
		}
		seen[text] = true
		result = append(result, text)
	}
	return result
}

func latestHistoryText(values []historyVersion, upToEpisodeIndex string) *string {
	var latest *historyVersion
	for index := range values {
		value := &values[index]
		if value.Text == "" || !episodeWithin(value.EpisodeIndex, upToEpisodeIndex) {
			continue
		}
		if latest == nil || compareEpisodeIndex(value.EpisodeIndex, latest.EpisodeIndex) > 0 {
			latest = value
		}
	}
	if latest == nil {
		return nil
	}
	return &latest.Text
}
