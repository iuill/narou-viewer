package characters

import (
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

func assignGeneratedCharacterIDs(stateDir string, novelID string, processedUpToEpisodeIndex string, generated []GeneratedCharacter) ([]GeneratedCharacter, characterEventsDocument, error) {
	doc, _, err := loadCharacterEventsDocument(stateDir, novelID)
	if err != nil {
		return nil, characterEventsDocument{}, err
	}
	if doc.SchemaVersion == 0 {
		doc.SchemaVersion = characterEventsSchemaVersion
	}
	doc.NovelID = novelID
	doc.ProcessedUpToEpisodeIndex = &processedUpToEpisodeIndex
	if doc.NextCharacterOrdinal <= 0 {
		doc.NextCharacterOrdinal = 1
	}
	usedIDs := map[string]bool{}
	for _, record := range doc.Characters {
		id := strings.TrimSpace(record.CharacterID)
		if id == "" {
			continue
		}
		usedIDs[id] = true
	}
	for _, retired := range doc.RetiredCharacterIDs {
		id := strings.TrimSpace(retired.CharacterID)
		if id != "" {
			usedIDs[id] = true
		}
	}
	advanceNextStableCharacterOrdinal(novelID, &doc.NextCharacterOrdinal, usedIDs)
	result := make([]GeneratedCharacter, 0, len(generated))
	resultIDs := map[string]bool{}
	for _, item := range generated {
		name := strings.TrimSpace(item.CanonicalName)
		if !isValidGeneratedCharacterName(name) {
			continue
		}
		if strings.TrimSpace(item.CharacterID) == "" || resultIDs[item.CharacterID] {
			item.CharacterID = nextStableCharacterID(novelID, &doc.NextCharacterOrdinal, usedIDs)
		}
		usedIDs[item.CharacterID] = true
		resultIDs[item.CharacterID] = true
		result = append(result, item)
	}
	doc.RetiredCharacterIDs = retireMissingCharacterIDs(doc.RetiredCharacterIDs, doc.Characters, resultIDs)
	advanceNextStableCharacterOrdinal(novelID, &doc.NextCharacterOrdinal, usedIDs)
	return result, doc, nil
}

func loadCharacterEventsDocument(stateDir string, novelID string) (characterEventsDocument, bool, error) {
	path := filepath.Join(stateDir, "character_events", novelID+".yaml")
	doc := characterEventsDocument{}
	if ok, _, err := readCharacterEventsIfExists(path, &doc); err != nil || ok {
		return doc, ok, err
	}
	profilePath := filepath.Join(stateDir, "character_profiles", novelID+".yaml")
	var profiles profilesDocument
	ok, _, err := readCharacterProfilesIfExists(profilePath, &profiles)
	if err != nil {
		if quarantined, quarantineErr := quarantineCharacterProfile(profilePath, err); quarantined || quarantineErr != nil {
			return characterEventsDocument{SchemaVersion: characterEventsSchemaVersion, NovelID: novelID, NextCharacterOrdinal: 1}, false, quarantineErr
		}
	}
	if err != nil || !ok {
		return characterEventsDocument{SchemaVersion: characterEventsSchemaVersion, NovelID: novelID, NextCharacterOrdinal: 1}, false, err
	}
	return migrateLegacyProfilesToEvents(novelID, profiles), false, nil
}

func LoadGeneratedUnresolvedMentions(stateDir string, novelID string) ([]GeneratedUnresolvedMention, error) {
	doc, ok, err := loadCharacterEventsDocument(stateDir, novelID)
	if err != nil || !ok {
		return nil, err
	}
	result := make([]GeneratedUnresolvedMention, 0, len(doc.UnresolvedMentions))
	for _, value := range doc.UnresolvedMentions {
		mention := strings.TrimSpace(value.Mention)
		episodeIndex := strings.TrimSpace(value.EpisodeIndex)
		if mention == "" || episodeIndex == "" {
			continue
		}
		result = append(result, GeneratedUnresolvedMention{
			Mention:      mention,
			EpisodeIndex: episodeIndex,
			Reason:       strings.TrimSpace(value.Reason),
			CandidateIDs: normalizeStringList(value.CandidateIDs),
		})
	}
	return result, nil
}

func LoadGeneratedEpisodeDigests(stateDir string, novelID string) ([]GeneratedEpisodeDigest, error) {
	doc, ok, err := loadCharacterEventsDocument(stateDir, novelID)
	if err != nil || !ok {
		return nil, err
	}
	result := make([]GeneratedEpisodeDigest, 0, len(doc.EpisodeEtags))
	for _, value := range doc.EpisodeEtags {
		episodeIndex := strings.TrimSpace(value.EpisodeIndex)
		contentEtag := strings.TrimSpace(value.ContentEtag)
		if episodeIndex == "" || contentEtag == "" {
			continue
		}
		result = append(result, GeneratedEpisodeDigest{EpisodeIndex: episodeIndex, ContentEtag: contentEtag})
	}
	return result, nil
}

func applyGeneratedCharacterIDState(doc *characterEventsDocument, novelID string, options SaveGeneratedSummaryOptions) {
	if doc == nil {
		return
	}
	retired := append([]retiredCharacterID{}, doc.RetiredCharacterIDs...)
	for _, value := range options.RetiredCharacterIDs {
		id := strings.TrimSpace(value.CharacterID)
		if id == "" {
			continue
		}
		retired = append(retired, retiredCharacterID{CharacterID: id, MergedInto: strings.TrimSpace(value.MergedInto)})
	}
	doc.RetiredCharacterIDs = normalizeRetiredCharacterIDs(retired)
	identityEvents := append([]identityMergeEvent{}, doc.IdentityMergeEvents...)
	for _, value := range options.IdentityMergeEvents {
		identityEvents = append(identityEvents, identityMergeEvent{
			SourceCharacterID:     strings.TrimSpace(value.SourceCharacterID),
			TargetCharacterID:     strings.TrimSpace(value.TargetCharacterID),
			EffectiveEpisodeIndex: strings.TrimSpace(value.EffectiveEpisodeIndex),
		})
	}
	doc.IdentityMergeEvents = normalizeIdentityMergeEvents(identityEvents)
	used := map[string]bool{}
	for _, record := range doc.Characters {
		if strings.TrimSpace(record.CharacterID) != "" {
			used[record.CharacterID] = true
		}
	}
	for _, id := range options.IssuedCharacterIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			used[id] = true
		}
	}
	for _, retired := range doc.RetiredCharacterIDs {
		if strings.TrimSpace(retired.CharacterID) != "" {
			used[retired.CharacterID] = true
		}
	}
	if options.NextCharacterOrdinal > doc.NextCharacterOrdinal {
		doc.NextCharacterOrdinal = options.NextCharacterOrdinal
	}
	advanceNextStableCharacterOrdinal(novelID, &doc.NextCharacterOrdinal, used)
}

func normalizeIdentityMergeEvents(values []identityMergeEvent) []identityMergeEvent {
	generated := make([]GeneratedIdentityMergeEvent, 0, len(values))
	for _, value := range values {
		generated = append(generated, GeneratedIdentityMergeEvent{
			SourceCharacterID:     value.SourceCharacterID,
			TargetCharacterID:     value.TargetCharacterID,
			EffectiveEpisodeIndex: value.EffectiveEpisodeIndex,
		})
	}
	normalized := NormalizeGeneratedIdentityMergeEvents(generated)
	result := make([]identityMergeEvent, 0, len(normalized))
	for _, value := range normalized {
		result = append(result, identityMergeEvent{
			SourceCharacterID:     value.SourceCharacterID,
			TargetCharacterID:     value.TargetCharacterID,
			EffectiveEpisodeIndex: value.EffectiveEpisodeIndex,
		})
	}
	return result
}

func NormalizeGeneratedIdentityMergeEvents(values []GeneratedIdentityMergeEvent) []GeneratedIdentityMergeEvent {
	candidates := make([]GeneratedIdentityMergeEvent, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value.SourceCharacterID = strings.TrimSpace(value.SourceCharacterID)
		value.TargetCharacterID = strings.TrimSpace(value.TargetCharacterID)
		value.EffectiveEpisodeIndex = strings.TrimSpace(value.EffectiveEpisodeIndex)
		if value.SourceCharacterID == "" || value.TargetCharacterID == "" || value.SourceCharacterID == value.TargetCharacterID || value.EffectiveEpisodeIndex == "" {
			continue
		}
		key := value.EffectiveEpisodeIndex + "\x00" + value.SourceCharacterID + "\x00" + value.TargetCharacterID
		if seen[key] {
			continue
		}
		seen[key] = true
		candidates = append(candidates, value)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if diff := compareEpisodeIndex(candidates[i].EffectiveEpisodeIndex, candidates[j].EffectiveEpisodeIndex); diff != 0 {
			return diff < 0
		}
		if candidates[i].SourceCharacterID != candidates[j].SourceCharacterID {
			return candidates[i].SourceCharacterID < candidates[j].SourceCharacterID
		}
		return candidates[i].TargetCharacterID < candidates[j].TargetCharacterID
	})
	result := make([]GeneratedIdentityMergeEvent, 0, len(candidates))
	mergedInto := map[string]string{}
	for _, candidate := range candidates {
		seenIDs := map[string]bool{candidate.SourceCharacterID: true}
		targetID := candidate.TargetCharacterID
		cyclic := false
		for targetID != "" {
			if seenIDs[targetID] {
				cyclic = true
				break
			}
			seenIDs[targetID] = true
			targetID = mergedInto[targetID]
		}
		if cyclic {
			continue
		}
		mergedInto[candidate.SourceCharacterID] = candidate.TargetCharacterID
		result = append(result, candidate)
	}
	return result
}

func normalizeRetiredCharacterIDs(values []retiredCharacterID) []retiredCharacterID {
	byID := map[string]retiredCharacterID{}
	for _, value := range values {
		id := strings.TrimSpace(value.CharacterID)
		if id == "" {
			continue
		}
		existing := byID[id]
		existing.CharacterID = id
		if strings.TrimSpace(value.MergedInto) != "" {
			existing.MergedInto = strings.TrimSpace(value.MergedInto)
		}
		byID[id] = existing
	}
	result := make([]retiredCharacterID, 0, len(byID))
	for _, value := range byID {
		result = append(result, value)
	}
	sort.SliceStable(result, func(i, j int) bool {
		return result[i].CharacterID < result[j].CharacterID
	})
	return result
}

func mergeRetiredCharacterEventRecords(records []characterEventRecord, retired []retiredCharacterID) []characterEventRecord {
	mergedInto := map[string]string{}
	for _, value := range retired {
		sourceID := strings.TrimSpace(value.CharacterID)
		targetID := strings.TrimSpace(value.MergedInto)
		if sourceID == "" || targetID == "" || sourceID == targetID {
			continue
		}
		mergedInto[sourceID] = targetID
	}
	return mergeCharacterEventRecordsByID(records, mergedInto)
}

func mergeIdentityCharacterEventRecordsAtBoundary(records []characterEventRecord, events []identityMergeEvent, boundary string) []characterEventRecord {
	mergedInto := map[string]string{}
	for _, value := range normalizeIdentityMergeEvents(events) {
		if compareEpisodeIndex(value.EffectiveEpisodeIndex, boundary) > 0 {
			continue
		}
		mergedInto[value.SourceCharacterID] = value.TargetCharacterID
	}
	return mergeCharacterEventRecordsByID(records, mergedInto)
}

func mergeCharacterEventRecordsByID(records []characterEventRecord, mergedInto map[string]string) []characterEventRecord {
	if len(records) == 0 {
		return records
	}
	if len(mergedInto) == 0 {
		return records
	}
	resolveTarget := func(sourceID string) string {
		seen := map[string]bool{}
		targetID := strings.TrimSpace(mergedInto[sourceID])
		for targetID != "" && !seen[targetID] {
			seen[targetID] = true
			next := strings.TrimSpace(mergedInto[targetID])
			if next == "" {
				return targetID
			}
			targetID = next
		}
		return targetID
	}
	byID := map[string]characterEventRecord{}
	order := []string{}
	for _, record := range records {
		id := strings.TrimSpace(record.CharacterID)
		if id == "" {
			continue
		}
		if _, ok := byID[id]; !ok {
			order = append(order, id)
		}
		byID[id] = record
	}
	for sourceID := range mergedInto {
		targetID := resolveTarget(sourceID)
		if targetID == "" || targetID == sourceID {
			continue
		}
		source, sourceOK := byID[sourceID]
		target, targetOK := byID[targetID]
		if !sourceOK || !targetOK {
			continue
		}
		target.PreferredNames = normalizeTextVersions(append(target.PreferredNames, source.PreferredNames...))
		target.Aliases = normalizeTextVersions(append(target.Aliases, source.Aliases...))
		target.FullNameHistory = normalizeTextVersions(append(append(target.FullNameHistory, derefTextVersion(target.FullName)...), append(source.FullNameHistory, derefTextVersion(source.FullName)...)...))
		target.FullName = latestTextVersion(target.FullNameHistory)
		target.GenderHistory = normalizeTextVersions(append(append(target.GenderHistory, derefTextVersion(target.Gender)...), append(source.GenderHistory, derefTextVersion(source.Gender)...)...))
		target.Gender = latestTextVersion(target.GenderHistory)
		if compareEpisodeIndex(source.FirstAppearanceEpisodeIndex, target.FirstAppearanceEpisodeIndex) < 0 || target.FirstAppearanceEpisodeIndex == "" {
			target.FirstAppearanceEpisodeIndex = source.FirstAppearanceEpisodeIndex
		}
		target.Facts = normalizeCharacterFacts(append(target.Facts, source.Facts...))
		target.Mentions = normalizeCharacterMentions(append(target.Mentions, source.Mentions...))
		byID[targetID] = target
		delete(byID, sourceID)
	}
	result := make([]characterEventRecord, 0, len(byID))
	for _, id := range order {
		if record, ok := byID[id]; ok {
			result = append(result, record)
		}
	}
	return result
}

func preserveIdentityMergeSourceRecords(records []characterEventRecord, events []identityMergeEvent, existing []characterEventRecord) []characterEventRecord {
	sourceIDs := map[string]bool{}
	for _, event := range normalizeIdentityMergeEvents(events) {
		sourceIDs[event.SourceCharacterID] = true
	}
	if len(sourceIDs) == 0 {
		return records
	}
	present := map[string]bool{}
	for _, record := range records {
		present[strings.TrimSpace(record.CharacterID)] = true
	}
	result := append([]characterEventRecord{}, records...)
	for _, record := range existing {
		id := strings.TrimSpace(record.CharacterID)
		if id != "" && sourceIDs[id] && !present[id] {
			present[id] = true
			result = append(result, record)
		}
	}
	return result
}

func mergeEpisodeEtags(existing []episodeEtag, episodes []HeuristicEpisode) []episodeEtag {
	byIndex := map[string]string{}
	for _, value := range existing {
		episodeIndex := strings.TrimSpace(value.EpisodeIndex)
		contentEtag := strings.TrimSpace(value.ContentEtag)
		if episodeIndex != "" && contentEtag != "" {
			byIndex[episodeIndex] = contentEtag
		}
	}
	for _, episode := range episodes {
		episodeIndex := strings.TrimSpace(episode.EpisodeIndex)
		contentEtag := strings.TrimSpace(episode.ContentEtag)
		if episodeIndex != "" && contentEtag != "" {
			byIndex[episodeIndex] = contentEtag
		}
	}
	result := make([]episodeEtag, 0, len(byIndex))
	for episodeIndex, contentEtag := range byIndex {
		result = append(result, episodeEtag{EpisodeIndex: episodeIndex, ContentEtag: contentEtag})
	}
	sort.SliceStable(result, func(i, j int) bool {
		return compareEpisodeIndex(result[i].EpisodeIndex, result[j].EpisodeIndex) < 0
	})
	return result
}

func truncateCharacterEventRecordsBeforeEpisode(records []characterEventRecord, fromEpisodeIndex string) []characterEventRecord {
	fromEpisodeIndex = strings.TrimSpace(fromEpisodeIndex)
	if fromEpisodeIndex == "" {
		return records
	}
	result := make([]characterEventRecord, 0, len(records))
	for _, record := range records {
		if strings.TrimSpace(record.FirstAppearanceEpisodeIndex) != "" && compareEpisodeIndex(record.FirstAppearanceEpisodeIndex, fromEpisodeIndex) >= 0 {
			continue
		}
		truncated := record
		truncated.PreferredNames = filterTextVersionsBeforeEpisode(record.PreferredNames, fromEpisodeIndex)
		if len(truncated.PreferredNames) == 0 && record.CanonicalName.Text != "" && compareEpisodeIndex(record.CanonicalName.EpisodeIndex, fromEpisodeIndex) < 0 {
			truncated.PreferredNames = []textVersion{record.CanonicalName}
		}
		if len(truncated.PreferredNames) == 0 {
			truncated.PreferredNames = filterTextVersionsBeforeEpisode(record.Aliases, fromEpisodeIndex)
		}
		if len(truncated.PreferredNames) == 0 {
			continue
		}
		truncated.CanonicalName = truncated.PreferredNames[len(truncated.PreferredNames)-1]
		truncated.FullNameHistory = filterTextVersionsBeforeEpisode(append(record.FullNameHistory, derefTextVersion(record.FullName)...), fromEpisodeIndex)
		truncated.FullName = latestTextVersion(truncated.FullNameHistory)
		truncated.GenderHistory = filterTextVersionsBeforeEpisode(append(record.GenderHistory, derefTextVersion(record.Gender)...), fromEpisodeIndex)
		truncated.Gender = latestTextVersion(truncated.GenderHistory)
		truncated.Aliases = filterTextVersionsBeforeEpisode(record.Aliases, fromEpisodeIndex)
		if len(truncated.Aliases) == 0 {
			truncated.Aliases = []textVersion{truncated.CanonicalName}
		}
		truncated.Facts = filterCharacterFactsBeforeEpisode(record.Facts, fromEpisodeIndex)
		truncated.Mentions = filterCharacterMentionsBeforeEpisode(record.Mentions, fromEpisodeIndex)
		result = append(result, truncated)
	}
	return result
}

func truncateUnresolvedMentionsBeforeEpisode(values []unresolvedMention, fromEpisodeIndex string) []unresolvedMention {
	result := []unresolvedMention{}
	for _, value := range values {
		if strings.TrimSpace(value.EpisodeIndex) != "" && compareEpisodeIndex(value.EpisodeIndex, fromEpisodeIndex) < 0 {
			result = append(result, value)
		}
	}
	return result
}

func truncateEpisodeEtagsBeforeEpisode(values []episodeEtag, fromEpisodeIndex string) []episodeEtag {
	result := []episodeEtag{}
	for _, value := range values {
		if strings.TrimSpace(value.EpisodeIndex) != "" && compareEpisodeIndex(value.EpisodeIndex, fromEpisodeIndex) < 0 {
			result = append(result, value)
		}
	}
	return result
}

func truncateIdentityMergeEventsBeforeEpisode(values []identityMergeEvent, fromEpisodeIndex string) []identityMergeEvent {
	result := []identityMergeEvent{}
	for _, value := range values {
		if strings.TrimSpace(value.EffectiveEpisodeIndex) != "" && compareEpisodeIndex(value.EffectiveEpisodeIndex, fromEpisodeIndex) < 0 {
			result = append(result, value)
		}
	}
	return normalizeIdentityMergeEvents(result)
}

func previousProcessedEpisodeIndex(current *string, fromEpisodeIndex string) *string {
	if current == nil {
		return nil
	}
	if strings.TrimSpace(fromEpisodeIndex) == "" || compareEpisodeIndex(*current, fromEpisodeIndex) < 0 {
		value := *current
		return &value
	}
	if number, err := strconv.Atoi(fromEpisodeIndex); err == nil && number > 1 {
		value := strconv.Itoa(number - 1)
		return &value
	}
	return nil
}

func filterTextVersionsBeforeEpisode(values []textVersion, fromEpisodeIndex string) []textVersion {
	filtered := []textVersion{}
	for _, value := range values {
		if strings.TrimSpace(value.EpisodeIndex) != "" && compareEpisodeIndex(value.EpisodeIndex, fromEpisodeIndex) < 0 {
			filtered = append(filtered, value)
		}
	}
	return normalizeTextVersions(filtered)
}

func filterCharacterFactsBeforeEpisode(values []characterFact, fromEpisodeIndex string) []characterFact {
	filtered := []characterFact{}
	for _, value := range values {
		if strings.TrimSpace(value.EpisodeIndex) != "" && compareEpisodeIndex(value.EpisodeIndex, fromEpisodeIndex) < 0 {
			filtered = append(filtered, value)
		}
	}
	return normalizeCharacterFacts(filtered)
}

func filterCharacterMentionsBeforeEpisode(values []characterMention, fromEpisodeIndex string) []characterMention {
	filtered := []characterMention{}
	for _, value := range values {
		if strings.TrimSpace(value.EpisodeIndex) != "" && compareEpisodeIndex(value.EpisodeIndex, fromEpisodeIndex) < 0 {
			filtered = append(filtered, value)
		}
	}
	return normalizeCharacterMentions(filtered)
}

func retireMissingCharacterIDs(existing []retiredCharacterID, previous []characterEventRecord, liveIDs map[string]bool) []retiredCharacterID {
	result := append([]retiredCharacterID{}, existing...)
	seen := map[string]bool{}
	for _, retired := range result {
		id := strings.TrimSpace(retired.CharacterID)
		if id != "" {
			seen[id] = true
		}
	}
	for _, record := range previous {
		id := strings.TrimSpace(record.CharacterID)
		if id == "" || liveIDs[id] || seen[id] {
			continue
		}
		seen[id] = true
		result = append(result, retiredCharacterID{CharacterID: id})
	}
	sort.SliceStable(result, func(i, j int) bool {
		return result[i].CharacterID < result[j].CharacterID
	})
	return result
}
