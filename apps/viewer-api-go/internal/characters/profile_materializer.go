package characters

import (
	"sort"
	"strings"
)

func generatedCharactersToProfiles(novelID string, processedUpToEpisodeIndex string, generated []GeneratedCharacter) []characterProfile {
	profiles := make([]characterProfile, 0, len(generated))
	for _, item := range generated {
		name := strings.TrimSpace(item.CanonicalName)
		if !isValidGeneratedCharacterName(name) {
			continue
		}
		canonicalEpisodeIndex := firstNonEmpty(item.CanonicalEpisodeIndex, processedUpToEpisodeIndex)
		firstAppearanceEpisodeIndex := firstNonEmpty(item.FirstAppearanceEpisodeIndex, canonicalEpisodeIndex, processedUpToEpisodeIndex)
		characterID := strings.TrimSpace(item.CharacterID)
		if characterID == "" {
			characterID = createCharacterID(novelID, name)
		}
		profile := characterProfile{
			CharacterID:                 characterID,
			CanonicalName:               textVersion{Text: name, EpisodeIndex: canonicalEpisodeIndex},
			PreferredNames:              generatedTextVersionsToTextVersions(append(item.NameHistory, GeneratedTextVersion{Text: name, EpisodeIndex: canonicalEpisodeIndex})),
			FirstAppearanceEpisodeIndex: firstAppearanceEpisodeIndex,
			Aliases:                     []textVersion{{Text: name, EpisodeIndex: canonicalEpisodeIndex}},
			ImportanceMetrics:           &importanceMetricsDoc{EpisodeMentions: []episodeMentionDoc{{EpisodeIndex: processedUpToEpisodeIndex, Count: 1}}},
		}
		if item.FullName != nil && strings.TrimSpace(*item.FullName) != "" {
			index := firstNonEmpty(item.FullNameEpisodeIndex, processedUpToEpisodeIndex)
			profile.FullName = &textVersion{Text: strings.TrimSpace(*item.FullName), EpisodeIndex: index}
		}
		profile.FullNameHistory = generatedTextVersionsToTextVersions(item.FullNameHistory)
		if profile.FullName != nil {
			profile.FullNameHistory = normalizeTextVersions(append(profile.FullNameHistory, *profile.FullName))
		}
		profile.FullName = latestTextVersion(profile.FullNameHistory)
		if item.Gender != nil && strings.TrimSpace(*item.Gender) != "" {
			index := firstNonEmpty(item.GenderEpisodeIndex, processedUpToEpisodeIndex)
			profile.Gender = &textVersion{Text: strings.TrimSpace(*item.Gender), EpisodeIndex: index}
		}
		profile.GenderHistory = generatedTextVersionsToTextVersions(item.GenderHistory)
		if profile.Gender != nil {
			profile.GenderHistory = normalizeTextVersions(append(profile.GenderHistory, *profile.Gender))
		}
		profile.Gender = latestTextVersion(profile.GenderHistory)
		if len(item.Aliases) > 0 {
			profile.Aliases = []textVersion{}
			seenAliases := map[string]bool{}
			for _, alias := range item.Aliases {
				text := strings.TrimSpace(alias.Text)
				index := firstNonEmpty(alias.EpisodeIndex, processedUpToEpisodeIndex)
				key := index + "\x00" + text
				if text == "" || seenAliases[key] {
					continue
				}
				seenAliases[key] = true
				profile.Aliases = append(profile.Aliases, textVersion{Text: text, EpisodeIndex: index})
			}
			if len(profile.Aliases) == 0 {
				profile.Aliases = []textVersion{{Text: name, EpisodeIndex: canonicalEpisodeIndex}}
			}
		}
		profile.AppearanceHistory = generatedHistoryVersions(item.AppearanceHistory, processedUpToEpisodeIndex)
		profile.PersonalityHistory = generatedHistoryVersions(item.PersonalityHistory, processedUpToEpisodeIndex)
		profile.SummaryHistory = generatedHistoryVersions(item.SummaryHistory, processedUpToEpisodeIndex)
		if len(profile.AppearanceHistory) == 0 && item.Appearance != nil && strings.TrimSpace(*item.Appearance) != "" {
			profile.AppearanceHistory = []historyVersion{{Text: strings.TrimSpace(*item.Appearance), EpisodeIndex: processedUpToEpisodeIndex}}
		}
		if len(profile.PersonalityHistory) == 0 && item.Personality != nil && strings.TrimSpace(*item.Personality) != "" {
			profile.PersonalityHistory = []historyVersion{{Text: strings.TrimSpace(*item.Personality), EpisodeIndex: processedUpToEpisodeIndex}}
		}
		if len(profile.SummaryHistory) == 0 && item.Summary != nil && strings.TrimSpace(*item.Summary) != "" {
			profile.SummaryHistory = []historyVersion{{Text: strings.TrimSpace(*item.Summary), EpisodeIndex: processedUpToEpisodeIndex}}
		}
		profiles = append(profiles, profile)
	}
	return profiles
}

func generatedCharactersToEventRecords(profiles []characterProfile, generated []GeneratedCharacter, fallbackEpisodeIndex string) []characterEventRecord {
	return generatedCharactersToEventRecordsWithExisting(profiles, generated, fallbackEpisodeIndex, nil)
}

func generatedCharactersToEventRecordsWithExisting(profiles []characterProfile, generated []GeneratedCharacter, fallbackEpisodeIndex string, existing []characterEventRecord) []characterEventRecord {
	byID := map[string]GeneratedCharacter{}
	for _, item := range generated {
		if strings.TrimSpace(item.CharacterID) != "" {
			byID[item.CharacterID] = item
		}
	}
	existingByID := map[string]characterEventRecord{}
	for _, record := range existing {
		if strings.TrimSpace(record.CharacterID) != "" {
			existingByID[record.CharacterID] = record
		}
	}
	records := make([]characterEventRecord, 0, len(profiles))
	for _, profile := range profiles {
		item := byID[profile.CharacterID]
		facts := []characterFact{}
		preferredNames := []textVersion{}
		aliases := []textVersion{}
		if existingRecord, ok := existingByID[profile.CharacterID]; ok {
			facts = append(facts, existingRecord.Facts...)
			preferredNames = append(preferredNames, existingRecord.PreferredNames...)
			aliases = append(aliases, existingRecord.Aliases...)
		}
		for _, value := range profile.AppearanceHistory {
			facts = append(facts, characterFact{Kind: "appearance", Text: value.Text, EpisodeIndex: value.EpisodeIndex})
		}
		for _, value := range profile.PersonalityHistory {
			facts = append(facts, characterFact{Kind: "personality", Text: value.Text, EpisodeIndex: value.EpisodeIndex})
		}
		for _, value := range profile.SummaryHistory {
			facts = append(facts, characterFact{Kind: "summary", Text: value.Text, EpisodeIndex: value.EpisodeIndex})
		}
		preferredNames = append(preferredNames, profile.PreferredNames...)
		aliases = append(aliases, profile.Aliases...)
		mentions := []characterMention{}
		if existingRecord, ok := existingByID[profile.CharacterID]; ok {
			mentions = append(mentions, existingRecord.Mentions...)
		}
		if profile.ImportanceMetrics != nil {
			for _, mention := range profile.ImportanceMetrics.EpisodeMentions {
				mentions = append(mentions, characterMention{
					Text:         firstNonEmpty(profile.CanonicalName.Text, item.CanonicalName),
					EpisodeIndex: mention.EpisodeIndex,
					Count:        mention.Count,
				})
			}
		}
		if len(mentions) == 0 {
			mentions = append(mentions, characterMention{
				Text:         firstNonEmpty(profile.CanonicalName.Text, item.CanonicalName),
				EpisodeIndex: firstNonEmpty(profile.FirstAppearanceEpisodeIndex, fallbackEpisodeIndex),
				Count:        1,
			})
		}
		fullNameHistory := []textVersion{}
		genderHistory := []textVersion{}
		if existingRecord, ok := existingByID[profile.CharacterID]; ok {
			fullNameHistory = append(fullNameHistory, existingRecord.FullNameHistory...)
			if existingRecord.FullName != nil {
				fullNameHistory = append(fullNameHistory, *existingRecord.FullName)
			}
			genderHistory = append(genderHistory, existingRecord.GenderHistory...)
			if existingRecord.Gender != nil {
				genderHistory = append(genderHistory, *existingRecord.Gender)
			}
		}
		fullNameHistory = append(fullNameHistory, profile.FullNameHistory...)
		fullNameHistory = append(fullNameHistory, generatedTextVersionsToTextVersions(item.FullNameHistory)...)
		if profile.FullName != nil {
			fullNameHistory = append(fullNameHistory, *profile.FullName)
		}
		genderHistory = append(genderHistory, profile.GenderHistory...)
		genderHistory = append(genderHistory, generatedTextVersionsToTextVersions(item.GenderHistory)...)
		if profile.Gender != nil {
			genderHistory = append(genderHistory, *profile.Gender)
		}
		fullNameHistory = normalizeTextVersions(fullNameHistory)
		genderHistory = normalizeTextVersions(genderHistory)
		records = append(records, characterEventRecord{
			CharacterID:                 profile.CharacterID,
			CanonicalName:               profile.CanonicalName,
			PreferredNames:              normalizeTextVersions(preferredNames),
			FullName:                    latestTextVersion(fullNameHistory),
			FullNameHistory:             fullNameHistory,
			Gender:                      latestTextVersion(genderHistory),
			GenderHistory:               genderHistory,
			FirstAppearanceEpisodeIndex: profile.FirstAppearanceEpisodeIndex,
			Aliases:                     normalizeTextVersions(aliases),
			Facts:                       normalizeCharacterFacts(facts),
			Mentions:                    normalizeCharacterMentions(mentions),
		})
	}
	return records
}

func profilesToEventRecords(profiles []characterProfile) []characterEventRecord {
	records := make([]characterEventRecord, 0, len(profiles))
	for _, profile := range profiles {
		records = append(records, generatedCharactersToEventRecords([]characterProfile{profile}, []GeneratedCharacter{profileToGeneratedCharacter(profile)}, profile.FirstAppearanceEpisodeIndex)...)
	}
	return records
}

func eventRecordsToProfiles(records []characterEventRecord) []characterProfile {
	profiles := make([]characterProfile, 0, len(records))
	for _, record := range records {
		fullNameHistory := normalizeTextVersions(append(record.FullNameHistory, derefTextVersion(record.FullName)...))
		genderHistory := normalizeTextVersions(append(record.GenderHistory, derefTextVersion(record.Gender)...))
		profile := characterProfile{
			CharacterID:                 strings.TrimSpace(record.CharacterID),
			CanonicalName:               textVersion{Text: strings.TrimSpace(record.CanonicalName.Text), EpisodeIndex: strings.TrimSpace(record.CanonicalName.EpisodeIndex)},
			PreferredNames:              normalizeTextVersions(record.PreferredNames),
			FullName:                    latestTextVersion(fullNameHistory),
			FullNameHistory:             fullNameHistory,
			Gender:                      latestTextVersion(genderHistory),
			GenderHistory:               genderHistory,
			FirstAppearanceEpisodeIndex: strings.TrimSpace(record.FirstAppearanceEpisodeIndex),
			Aliases:                     normalizeTextVersions(record.Aliases),
			ImportanceMetrics:           &importanceMetricsDoc{},
		}
		if len(profile.PreferredNames) == 0 && profile.CanonicalName.Text != "" && profile.CanonicalName.EpisodeIndex != "" {
			profile.PreferredNames = []textVersion{profile.CanonicalName}
		}
		for _, fact := range record.Facts {
			version := historyVersion{EpisodeIndex: fact.EpisodeIndex, Text: fact.Text}
			switch fact.Kind {
			case "appearance":
				profile.AppearanceHistory = append(profile.AppearanceHistory, version)
			case "personality":
				profile.PersonalityHistory = append(profile.PersonalityHistory, version)
			default:
				profile.SummaryHistory = append(profile.SummaryHistory, version)
			}
		}
		profile.AppearanceHistory = normalizeHistoryVersions(profile.AppearanceHistory)
		profile.PersonalityHistory = normalizeHistoryVersions(profile.PersonalityHistory)
		profile.SummaryHistory = normalizeHistoryVersions(profile.SummaryHistory)
		for _, mention := range normalizeCharacterMentions(record.Mentions) {
			profile.ImportanceMetrics.EpisodeMentions = append(profile.ImportanceMetrics.EpisodeMentions, episodeMentionDoc{
				EpisodeIndex: mention.EpisodeIndex,
				Count:        mention.Count,
			})
		}
		if profile.CharacterID != "" && profile.CanonicalName.Text != "" {
			profiles = append(profiles, profile)
		}
	}
	sort.SliceStable(profiles, func(i, j int) bool {
		diff := compareEpisodeIndex(profiles[i].FirstAppearanceEpisodeIndex, profiles[j].FirstAppearanceEpisodeIndex)
		if diff != 0 {
			return diff < 0
		}
		return profiles[i].CanonicalName.Text < profiles[j].CanonicalName.Text
	})
	return profiles
}

func eventRecordsToGeneratedCharacters(records []characterEventRecord) []GeneratedCharacter {
	result := make([]GeneratedCharacter, 0, len(records))
	for _, record := range records {
		fullNameHistory := normalizeTextVersions(append(record.FullNameHistory, derefTextVersion(record.FullName)...))
		genderHistory := normalizeTextVersions(append(record.GenderHistory, derefTextVersion(record.Gender)...))
		item := GeneratedCharacter{
			CharacterID:                 record.CharacterID,
			CanonicalName:               strings.TrimSpace(record.CanonicalName.Text),
			CanonicalEpisodeIndex:       record.CanonicalName.EpisodeIndex,
			NameHistory:                 generatedTextVersions(record.PreferredNames),
			FirstAppearanceEpisodeIndex: record.FirstAppearanceEpisodeIndex,
			Aliases:                     generatedTextVersions(record.Aliases),
		}
		if len(item.NameHistory) == 0 && item.CanonicalName != "" && item.CanonicalEpisodeIndex != "" {
			item.NameHistory = []GeneratedTextVersion{{Text: item.CanonicalName, EpisodeIndex: item.CanonicalEpisodeIndex}}
		}
		if record.FullName != nil && strings.TrimSpace(record.FullName.Text) != "" {
			text := strings.TrimSpace(record.FullName.Text)
			item.FullName = &text
			item.FullNameEpisodeIndex = record.FullName.EpisodeIndex
		}
		item.FullNameHistory = generatedTextVersions(fullNameHistory)
		if record.Gender != nil && strings.TrimSpace(record.Gender.Text) != "" {
			text := strings.TrimSpace(record.Gender.Text)
			item.Gender = &text
			item.GenderEpisodeIndex = record.Gender.EpisodeIndex
		}
		item.GenderHistory = generatedTextVersions(genderHistory)
		for _, fact := range record.Facts {
			version := GeneratedHistoryVersion{EpisodeIndex: fact.EpisodeIndex, Text: fact.Text}
			switch fact.Kind {
			case "appearance":
				item.AppearanceHistory = append(item.AppearanceHistory, version)
			case "personality":
				item.PersonalityHistory = append(item.PersonalityHistory, version)
			default:
				item.SummaryHistory = append(item.SummaryHistory, version)
			}
		}
		item.AppearanceHistory = normalizeGeneratedHistoryVersionList(item.AppearanceHistory)
		item.PersonalityHistory = normalizeGeneratedHistoryVersionList(item.PersonalityHistory)
		item.SummaryHistory = normalizeGeneratedHistoryVersionList(item.SummaryHistory)
		if item.CanonicalName != "" {
			result = append(result, item)
		}
	}
	sortGeneratedCharacters(result)
	return result
}

func profileToGeneratedCharacter(profile characterProfile) GeneratedCharacter {
	item := GeneratedCharacter{
		CharacterID:                 profile.CharacterID,
		CanonicalName:               profile.CanonicalName.Text,
		CanonicalEpisodeIndex:       profile.CanonicalName.EpisodeIndex,
		NameHistory:                 generatedTextVersions(profile.PreferredNames),
		FirstAppearanceEpisodeIndex: profile.FirstAppearanceEpisodeIndex,
		Aliases:                     generatedTextVersions(profile.Aliases),
		AppearanceHistory:           generatedHistoryVersionsFromProfile(profile.AppearanceHistory),
		PersonalityHistory:          generatedHistoryVersionsFromProfile(profile.PersonalityHistory),
		SummaryHistory:              generatedHistoryVersionsFromProfile(profile.SummaryHistory),
	}
	if profile.FullName != nil {
		text := profile.FullName.Text
		item.FullName = &text
		item.FullNameEpisodeIndex = profile.FullName.EpisodeIndex
	}
	item.FullNameHistory = generatedTextVersions(profile.FullNameHistory)
	if profile.Gender != nil {
		text := profile.Gender.Text
		item.Gender = &text
		item.GenderEpisodeIndex = profile.Gender.EpisodeIndex
	}
	item.GenderHistory = generatedTextVersions(profile.GenderHistory)
	return item
}

func generatedTextVersions(values []textVersion) []GeneratedTextVersion {
	result := make([]GeneratedTextVersion, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value.Text) != "" && strings.TrimSpace(value.EpisodeIndex) != "" {
			result = append(result, GeneratedTextVersion{Text: strings.TrimSpace(value.Text), EpisodeIndex: strings.TrimSpace(value.EpisodeIndex)})
		}
	}
	return result
}

func generatedTextVersionsToTextVersions(values []GeneratedTextVersion) []textVersion {
	result := make([]textVersion, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value.Text) != "" && strings.TrimSpace(value.EpisodeIndex) != "" {
			result = append(result, textVersion{Text: strings.TrimSpace(value.Text), EpisodeIndex: strings.TrimSpace(value.EpisodeIndex)})
		}
	}
	return normalizeTextVersions(result)
}

func generatedHistoryVersionsFromProfile(values []historyVersion) []GeneratedHistoryVersion {
	result := make([]GeneratedHistoryVersion, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value.Text) != "" && strings.TrimSpace(value.EpisodeIndex) != "" {
			result = append(result, GeneratedHistoryVersion{Text: strings.TrimSpace(value.Text), EpisodeIndex: strings.TrimSpace(value.EpisodeIndex)})
		}
	}
	return result
}

func normalizeTextVersions(values []textVersion) []textVersion {
	result := []textVersion{}
	seen := map[string]bool{}
	for _, value := range values {
		text := strings.TrimSpace(value.Text)
		episodeIndex := strings.TrimSpace(value.EpisodeIndex)
		key := episodeIndex + "\x00" + text
		if text == "" || episodeIndex == "" || seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, textVersion{Text: text, EpisodeIndex: episodeIndex})
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

func normalizeHistoryVersions(values []historyVersion) []historyVersion {
	result := []historyVersion{}
	seen := map[string]bool{}
	for _, value := range values {
		text := strings.TrimSpace(value.Text)
		episodeIndex := strings.TrimSpace(value.EpisodeIndex)
		key := episodeIndex + "\x00" + text
		if text == "" || episodeIndex == "" || seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, historyVersion{EpisodeIndex: episodeIndex, Text: text})
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

func normalizeCharacterFacts(values []characterFact) []characterFact {
	result := []characterFact{}
	seen := map[string]bool{}
	for _, value := range values {
		kind := firstNonEmpty(value.Kind, "summary")
		text := strings.TrimSpace(value.Text)
		episodeIndex := strings.TrimSpace(value.EpisodeIndex)
		key := kind + "\x00" + episodeIndex + "\x00" + text
		if text == "" || episodeIndex == "" || seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, characterFact{Kind: kind, Text: text, EpisodeIndex: episodeIndex})
	}
	sort.SliceStable(result, func(i, j int) bool {
		diff := compareEpisodeIndex(result[i].EpisodeIndex, result[j].EpisodeIndex)
		if diff != 0 {
			return diff < 0
		}
		if result[i].Kind != result[j].Kind {
			return result[i].Kind < result[j].Kind
		}
		return result[i].Text < result[j].Text
	})
	return result
}

func normalizeCharacterMentions(values []characterMention) []characterMention {
	byKey := map[string]characterMention{}
	for _, value := range values {
		text := strings.TrimSpace(value.Text)
		episodeIndex := strings.TrimSpace(value.EpisodeIndex)
		if text == "" || episodeIndex == "" {
			continue
		}
		count := value.Count
		if count <= 0 {
			count = 1
		}
		key := episodeIndex + "\x00" + text
		existing := byKey[key]
		existing.Text = text
		existing.EpisodeIndex = episodeIndex
		existing.Count += count
		byKey[key] = existing
	}
	result := make([]characterMention, 0, len(byKey))
	for _, value := range byKey {
		result = append(result, value)
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

func mergeUnresolvedMentions(existing []unresolvedMention, incoming []GeneratedUnresolvedMention) []unresolvedMention {
	result := append([]unresolvedMention{}, existing...)
	seen := map[string]bool{}
	for _, value := range result {
		key := strings.TrimSpace(value.EpisodeIndex) + "\x00" + strings.TrimSpace(value.Mention)
		if key != "\x00" {
			seen[key] = true
		}
	}
	for _, value := range incoming {
		mention := strings.TrimSpace(value.Mention)
		episodeIndex := strings.TrimSpace(value.EpisodeIndex)
		if mention == "" || episodeIndex == "" {
			continue
		}
		key := episodeIndex + "\x00" + mention
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, unresolvedMention{
			Mention:      mention,
			EpisodeIndex: episodeIndex,
			Reason:       strings.TrimSpace(value.Reason),
			CandidateIDs: normalizeStringList(value.CandidateIDs),
		})
	}
	sort.SliceStable(result, func(i, j int) bool {
		diff := compareEpisodeIndex(result[i].EpisodeIndex, result[j].EpisodeIndex)
		if diff != 0 {
			return diff < 0
		}
		return result[i].Mention < result[j].Mention
	})
	return result
}

func unresolvedMentionsToEventRecords(values []GeneratedUnresolvedMention) []unresolvedMention {
	return mergeUnresolvedMentions(nil, values)
}

func generatedHistoryVersions(values []GeneratedHistoryVersion, fallbackEpisodeIndex string) []historyVersion {
	result := []historyVersion{}
	seen := map[string]bool{}
	for _, value := range values {
		text := strings.TrimSpace(value.Text)
		index := firstNonEmpty(value.EpisodeIndex, fallbackEpisodeIndex)
		key := index + "\x00" + text
		if text == "" || seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, historyVersion{Text: text, EpisodeIndex: index})
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
