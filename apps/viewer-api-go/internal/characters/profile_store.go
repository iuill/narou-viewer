package characters

import (
	"path/filepath"
	"sort"
	"strings"

	"narou-viewer/apps/viewer-api-go/internal/novelstate"
	"narou-viewer/apps/viewer-api-go/internal/state/schemaguard"
)

func LoadSummary(stateDir string, novelID string, upToEpisodeIndex string) (SummaryResponse, bool, error) {
	return LoadSummaryForEpisodes(stateDir, novelID, upToEpisodeIndex, nil)
}

func LoadSummaryForEpisodes(stateDir string, novelID string, upToEpisodeIndex string, episodeIndexes []string) (SummaryResponse, bool, error) {
	path := filepath.Join(stateDir, "character_profiles", novelID+".yaml")
	var doc profilesDocument
	ok, _, err := readCharacterProfilesIfExists(path, &doc)
	if err != nil {
		if _, isGuardError := schemaguard.AsGuardError(err); !isGuardError {
			return SummaryResponse{}, false, err
		}
		materialized, materializeErr := MaterializeGeneratedSummary(stateDir, novelID)
		if materializeErr != nil {
			return SummaryResponse{}, false, materializeErr
		}
		if !materialized {
			return SummaryResponse{}, false, nil
		}
		ok, _, err = readCharacterProfilesIfExists(path, &doc)
		if err != nil {
			return SummaryResponse{}, false, err
		}
	}
	if shouldMaterializeGeneratedSummary(stateDir, novelID, upToEpisodeIndex, ok, doc) {
		materialized, err := MaterializeGeneratedSummary(stateDir, novelID)
		if err != nil {
			return SummaryResponse{}, false, err
		}
		if materialized {
			ok, _, err = readCharacterProfilesIfExists(path, &doc)
			if err != nil {
				return SummaryResponse{}, false, err
			}
		}
	}
	if !ok {
		return SummaryResponse{}, false, nil
	}
	processed := doc.ProcessedUpToEpisodeIndex
	status := "ready"
	visibilityBoundary := upToEpisodeIndex
	if processed == nil {
		status = "not_generated"
	} else if compareEpisodeIndex(*processed, upToEpisodeIndex) < 0 {
		status = "partial"
		visibilityBoundary = *processed
	}
	characters := []Character{}
	if status == "ready" || status == "partial" {
		profiles := doc.Characters
		if len(doc.IdentityMergeEvents) > 0 {
			records := mergeIdentityCharacterEventRecordsAtBoundary(profilesToEventRecords(doc.Characters), doc.IdentityMergeEvents, visibilityBoundary)
			profiles = eventRecordsToProfiles(records)
		}
		visibleProfiles := []characterProfile{}
		for _, profile := range profiles {
			if !episodeWithin(profile.FirstAppearanceEpisodeIndex, visibilityBoundary) {
				continue
			}
			visibleProfiles = append(visibleProfiles, profile)
		}
		importanceByCharacterID := buildImportanceClassifications(visibleProfiles, episodeIndexes, visibilityBoundary)
		for _, profile := range visibleProfiles {
			characters = append(characters, profile.toCharacter(visibilityBoundary, importanceByCharacterID[profile.CharacterID]))
		}
		sort.SliceStable(characters, func(i, j int) bool {
			leftOrder := importanceOrder(characters[i].Importance)
			rightOrder := importanceOrder(characters[j].Importance)
			if leftOrder != rightOrder {
				return leftOrder < rightOrder
			}
			episodeDiff := compareEpisodeIndex(characters[i].FirstAppearanceEpisodeIndex, characters[j].FirstAppearanceEpisodeIndex)
			if episodeDiff != 0 {
				return episodeDiff < 0
			}
			return characters[i].CanonicalName < characters[j].CanonicalName
		})
	}
	return SummaryResponse{
		Status:                    status,
		NovelID:                   novelID,
		UpToEpisodeIndex:          upToEpisodeIndex,
		ProcessedUpToEpisodeIndex: processed,
		Characters:                characters,
	}, true, nil
}

func shouldMaterializeGeneratedSummary(stateDir string, novelID string, upToEpisodeIndex string, profileExists bool, profile profilesDocument) bool {
	if !profileExists || profile.ProcessedUpToEpisodeIndex == nil {
		return true
	}
	if compareEpisodeIndex(*profile.ProcessedUpToEpisodeIndex, upToEpisodeIndex) >= 0 {
		return false
	}
	events, ok, err := loadCharacterEventsDocument(stateDir, novelID)
	if err != nil || !ok || events.ProcessedUpToEpisodeIndex == nil {
		return false
	}
	return compareEpisodeIndex(*events.ProcessedUpToEpisodeIndex, *profile.ProcessedUpToEpisodeIndex) >= 0
}

func SaveHeuristicSummary(stateDir string, novelID string, processedUpToEpisodeIndex string, episodes []HeuristicEpisode) error {
	return novelstate.WithLock(novelID, func() error {
		path := filepath.Join(stateDir, "character_profiles", novelID+".yaml")
		if err := prepareCharacterProfileForWrite(path); err != nil {
			return err
		}
		doc := profilesDocument{
			SchemaVersion:             characterProfilesSchemaVersion,
			NovelID:                   novelID,
			ProcessedUpToEpisodeIndex: &processedUpToEpisodeIndex,
			Characters:                buildHeuristicProfiles(novelID, processedUpToEpisodeIndex, episodes),
		}
		return writeYAMLAtomic(path, doc)
	})
}

func SaveGeneratedSummary(stateDir string, novelID string, processedUpToEpisodeIndex string, generated []GeneratedCharacter) error {
	return SaveGeneratedSummaryWithEpisodes(stateDir, novelID, processedUpToEpisodeIndex, generated, nil)
}

func SaveGeneratedSummaryWithEpisodes(stateDir string, novelID string, processedUpToEpisodeIndex string, generated []GeneratedCharacter, episodes []HeuristicEpisode, unresolvedMentions ...[]GeneratedUnresolvedMention) error {
	options := SaveGeneratedSummaryOptions{}
	if len(unresolvedMentions) > 0 {
		options.UnresolvedMentions = unresolvedMentions[0]
		options.SetUnresolvedMentions = true
	}
	return SaveGeneratedSummaryWithOptions(stateDir, novelID, processedUpToEpisodeIndex, generated, episodes, options)
}

func SaveGeneratedSummaryWithOptions(stateDir string, novelID string, processedUpToEpisodeIndex string, generated []GeneratedCharacter, episodes []HeuristicEpisode, options SaveGeneratedSummaryOptions) error {
	return novelstate.WithLock(novelID, func() error {
		assigned, eventsDoc, err := assignGeneratedCharacterIDs(stateDir, novelID, processedUpToEpisodeIndex, generated)
		if err != nil {
			return err
		}
		replaceFromEpisodeIndex := strings.TrimSpace(options.ReplaceFromEpisodeIndex)
		if replaceFromEpisodeIndex != "" {
			eventsDoc.Characters = truncateCharacterEventRecordsBeforeEpisode(eventsDoc.Characters, replaceFromEpisodeIndex)
			eventsDoc.IdentityMergeEvents = truncateIdentityMergeEventsBeforeEpisode(eventsDoc.IdentityMergeEvents, replaceFromEpisodeIndex)
			eventsDoc.UnresolvedMentions = truncateUnresolvedMentionsBeforeEpisode(eventsDoc.UnresolvedMentions, replaceFromEpisodeIndex)
			eventsDoc.EpisodeEtags = truncateEpisodeEtagsBeforeEpisode(eventsDoc.EpisodeEtags, replaceFromEpisodeIndex)
		}
		applyGeneratedCharacterIDState(&eventsDoc, novelID, options)
		eventsDoc.Characters = mergeRetiredCharacterEventRecords(eventsDoc.Characters, eventsDoc.RetiredCharacterIDs)
		profiles := generatedCharactersToProfiles(novelID, processedUpToEpisodeIndex, assigned)
		applyGeneratedImportanceMetrics(profiles, episodes)
		existingEventRecords := eventsDoc.Characters
		eventsDoc.Characters = generatedCharactersToEventRecordsWithExisting(profiles, assigned, processedUpToEpisodeIndex, existingEventRecords)
		eventsDoc.Characters = preserveIdentityMergeSourceRecords(eventsDoc.Characters, eventsDoc.IdentityMergeEvents, existingEventRecords)
		if options.SetUnresolvedMentions {
			eventsDoc.UnresolvedMentions = unresolvedMentionsToEventRecords(options.UnresolvedMentions)
		}
		eventsDoc.EpisodeEtags = mergeEpisodeEtags(eventsDoc.EpisodeEtags, episodes)
		materializedProfiles := eventRecordsToProfiles(eventsDoc.Characters)
		doc := profilesDocument{
			SchemaVersion:             characterProfilesSchemaVersion,
			NovelID:                   novelID,
			ProcessedUpToEpisodeIndex: &processedUpToEpisodeIndex,
			IdentityMergeEvents:       append([]identityMergeEvent{}, eventsDoc.IdentityMergeEvents...),
			Characters:                materializedProfiles,
		}
		profilePath := filepath.Join(stateDir, "character_profiles", novelID+".yaml")
		if err := prepareCharacterProfileForWrite(profilePath); err != nil {
			return err
		}
		if err := writeYAMLAtomic(filepath.Join(stateDir, "character_events", novelID+".yaml"), eventsDoc); err != nil {
			return err
		}
		return writeYAMLAtomic(profilePath, doc)
	})
}

func MaterializeGeneratedSummary(stateDir string, novelID string) (bool, error) {
	materialized := false
	err := novelstate.WithLock(novelID, func() error {
		profilePath := filepath.Join(stateDir, "character_profiles", novelID+".yaml")
		if err := prepareCharacterProfileForWrite(profilePath); err != nil {
			return err
		}
		doc, ok, err := loadCharacterEventsDocument(stateDir, novelID)
		if err != nil || !ok || doc.ProcessedUpToEpisodeIndex == nil {
			return err
		}
		profiles := eventRecordsToProfiles(doc.Characters)
		profileDoc := profilesDocument{
			SchemaVersion:             characterProfilesSchemaVersion,
			NovelID:                   novelID,
			ProcessedUpToEpisodeIndex: doc.ProcessedUpToEpisodeIndex,
			IdentityMergeEvents:       append([]identityMergeEvent{}, doc.IdentityMergeEvents...),
			Characters:                profiles,
		}
		if err := writeYAMLAtomic(profilePath, profileDoc); err != nil {
			return err
		}
		materialized = true
		return nil
	})
	return materialized, err
}

func LoadGeneratedCharacters(stateDir string, novelID string) ([]GeneratedCharacter, *string, bool, error) {
	doc, ok, err := loadCharacterEventsDocument(stateDir, novelID)
	if err != nil || !ok {
		if err != nil {
			return nil, nil, false, err
		}
		// A legacy profile can still be materialized through the migration path.
		if doc.ProcessedUpToEpisodeIndex == nil && len(doc.Characters) == 0 {
			return nil, nil, false, nil
		}
	}
	records := doc.Characters
	if doc.ProcessedUpToEpisodeIndex != nil {
		records = mergeIdentityCharacterEventRecordsAtBoundary(records, doc.IdentityMergeEvents, *doc.ProcessedUpToEpisodeIndex)
	}
	return eventRecordsToGeneratedCharacters(records), doc.ProcessedUpToEpisodeIndex, doc.ProcessedUpToEpisodeIndex != nil || len(doc.Characters) > 0, nil
}

func LoadGeneratedCharacterState(stateDir string, novelID string) ([]GeneratedCharacter, []GeneratedIdentityMergeEvent, *string, bool, error) {
	doc, ok, err := loadCharacterEventsDocument(stateDir, novelID)
	if err != nil || !ok {
		if err != nil {
			return nil, nil, nil, false, err
		}
		if doc.ProcessedUpToEpisodeIndex == nil && len(doc.Characters) == 0 {
			return nil, nil, nil, false, nil
		}
	}
	return eventRecordsToGeneratedCharacters(doc.Characters), generatedIdentityMergeEvents(doc.IdentityMergeEvents), doc.ProcessedUpToEpisodeIndex, doc.ProcessedUpToEpisodeIndex != nil || len(doc.Characters) > 0, nil
}

func LoadGeneratedCharactersBeforeEpisode(stateDir string, novelID string, fromEpisodeIndex string) ([]GeneratedCharacter, *string, bool, error) {
	fromEpisodeIndex = strings.TrimSpace(fromEpisodeIndex)
	if fromEpisodeIndex == "" {
		return LoadGeneratedCharacters(stateDir, novelID)
	}
	doc, ok, err := loadCharacterEventsDocument(stateDir, novelID)
	if err != nil || !ok {
		if err != nil {
			return nil, nil, false, err
		}
		if doc.ProcessedUpToEpisodeIndex == nil && len(doc.Characters) == 0 {
			return nil, nil, false, nil
		}
	}
	truncated := truncateCharacterEventRecordsBeforeEpisode(doc.Characters, fromEpisodeIndex)
	processed := previousProcessedEpisodeIndex(doc.ProcessedUpToEpisodeIndex, fromEpisodeIndex)
	if processed != nil {
		truncated = mergeIdentityCharacterEventRecordsAtBoundary(truncated, doc.IdentityMergeEvents, *processed)
	}
	return eventRecordsToGeneratedCharacters(truncated), processed, true, nil
}

func LoadGeneratedCharacterStateBeforeEpisode(stateDir string, novelID string, fromEpisodeIndex string) ([]GeneratedCharacter, []GeneratedIdentityMergeEvent, *string, bool, error) {
	fromEpisodeIndex = strings.TrimSpace(fromEpisodeIndex)
	if fromEpisodeIndex == "" {
		return LoadGeneratedCharacterState(stateDir, novelID)
	}
	doc, ok, err := loadCharacterEventsDocument(stateDir, novelID)
	if err != nil || !ok {
		if err != nil {
			return nil, nil, nil, false, err
		}
		if doc.ProcessedUpToEpisodeIndex == nil && len(doc.Characters) == 0 {
			return nil, nil, nil, false, nil
		}
	}
	truncated := truncateCharacterEventRecordsBeforeEpisode(doc.Characters, fromEpisodeIndex)
	processed := previousProcessedEpisodeIndex(doc.ProcessedUpToEpisodeIndex, fromEpisodeIndex)
	events := truncateIdentityMergeEventsBeforeEpisode(doc.IdentityMergeEvents, fromEpisodeIndex)
	return eventRecordsToGeneratedCharacters(truncated), generatedIdentityMergeEvents(events), processed, true, nil
}

func generatedIdentityMergeEvents(values []identityMergeEvent) []GeneratedIdentityMergeEvent {
	result := make([]GeneratedIdentityMergeEvent, 0, len(values))
	for _, value := range normalizeIdentityMergeEvents(values) {
		result = append(result, GeneratedIdentityMergeEvent{
			SourceCharacterID:     value.SourceCharacterID,
			TargetCharacterID:     value.TargetCharacterID,
			EffectiveEpisodeIndex: value.EffectiveEpisodeIndex,
		})
	}
	return result
}
