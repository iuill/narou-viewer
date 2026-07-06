package characters

import (
	"regexp"
	"sort"
	"strings"
)

var candidateNamePattern = regexp.MustCompile(`(?:^|[「『（])([A-Za-z0-9\p{Han}\p{Katakana}々ー]{2,18}?)(?:さん|ちゃん|くん|君|様|殿|氏|先生|さま)?(?:は|が|を|に|へ|と|も|の)`)

func buildHeuristicProfiles(novelID string, fallbackEpisodeIndex string, episodes []HeuristicEpisode) []characterProfile {
	type evidence struct {
		mentions          int
		firstEpisodeIndex string
		aliases           map[string]string
		summarySentences  []historyVersion
		mentionCounts     map[string]int
	}
	byName := map[string]*evidence{}
	for _, episode := range episodes {
		for _, sentence := range splitCandidateSentences(episode.Text) {
			for _, match := range candidateNamePattern.FindAllStringSubmatch(sentence, -1) {
				if len(match) < 2 {
					continue
				}
				name := normalizeCandidateName(match[1])
				if !isLikelyCandidateName(name) {
					continue
				}
				item := byName[name]
				if item == nil {
					item = &evidence{
						firstEpisodeIndex: episode.EpisodeIndex,
						aliases:           map[string]string{name: episode.EpisodeIndex},
						mentionCounts:     map[string]int{},
					}
					byName[name] = item
				}
				item.mentions++
				item.mentionCounts[episode.EpisodeIndex]++
				if compareEpisodeIndex(episode.EpisodeIndex, item.firstEpisodeIndex) < 0 {
					item.firstEpisodeIndex = episode.EpisodeIndex
				}
				if _, ok := item.aliases[name]; !ok {
					item.aliases[name] = episode.EpisodeIndex
				}
				if len(item.summarySentences) < 3 {
					item.summarySentences = append(item.summarySentences, historyVersion{
						Text:         sentence,
						EpisodeIndex: episode.EpisodeIndex,
					})
				}
			}
		}
	}
	names := make([]string, 0, len(byName))
	for name, item := range byName {
		if item.mentions >= 2 {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	profiles := make([]characterProfile, 0, len(names))
	for _, name := range names {
		item := byName[name]
		aliases := make([]textVersion, 0, len(item.aliases))
		for alias, episodeIndex := range item.aliases {
			aliases = append(aliases, textVersion{Text: alias, EpisodeIndex: episodeIndex})
		}
		sort.SliceStable(aliases, func(i, j int) bool {
			return aliases[i].Text < aliases[j].Text
		})
		mentions := make([]episodeMentionDoc, 0, len(item.mentionCounts))
		for episodeIndex, count := range item.mentionCounts {
			mentions = append(mentions, episodeMentionDoc{EpisodeIndex: episodeIndex, Count: count})
		}
		sort.SliceStable(mentions, func(i, j int) bool {
			return compareEpisodeIndex(mentions[i].EpisodeIndex, mentions[j].EpisodeIndex) < 0
		})
		firstEpisodeIndex := item.firstEpisodeIndex
		if firstEpisodeIndex == "" {
			firstEpisodeIndex = fallbackEpisodeIndex
		}
		profiles = append(profiles, characterProfile{
			CharacterID:                 createCharacterID(novelID, name),
			CanonicalName:               textVersion{Text: name, EpisodeIndex: firstEpisodeIndex},
			FirstAppearanceEpisodeIndex: firstEpisodeIndex,
			Aliases:                     aliases,
			SummaryHistory:              item.summarySentences,
			ImportanceMetrics:           &importanceMetricsDoc{EpisodeMentions: mentions},
		})
	}
	return profiles
}

func splitCandidateSentences(text string) []string {
	normalized := strings.NewReplacer("\r\n", "\n", "\r", "\n").Replace(text)
	fields := regexp.MustCompile(`[。！？!?\n]+`).Split(normalized, -1)
	sentences := []string{}
	for _, field := range fields {
		trimmed := strings.TrimSpace(field)
		if trimmed != "" {
			sentences = append(sentences, trimmed)
		}
	}
	return sentences
}

func normalizeCandidateName(value string) string {
	return strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(strings.TrimSpace(value), "さん"), "ちゃん"), "くん")
}

func isLikelyCandidateName(value string) bool {
	if len([]rune(value)) < 2 || len([]rune(value)) > 18 {
		return false
	}
	switch value {
	case "それ", "これ", "そこ", "ここ", "ため", "よう", "もの", "こと":
		return false
	default:
		return true
	}
}
