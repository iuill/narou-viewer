package characters

import (
	"path/filepath"
	"sort"
	"strings"

	"narou-viewer/apps/viewer-api-go/internal/novelstate"
)

func LoadSummary(stateDir string, novelID string, upToEpisodeIndex string) (SummaryResponse, bool, error) {
	return LoadSummaryForEpisodes(stateDir, novelID, upToEpisodeIndex, nil)
}

func LoadSummaryForEpisodes(stateDir string, novelID string, upToEpisodeIndex string, episodeIndexes []string) (SummaryResponse, bool, error) {
	path := filepath.Join(stateDir, "character_profiles", novelID+".yaml")
	var doc profilesDocument
	ok, err := readYAMLIfExists(path, &doc)
	if err != nil {
		return SummaryResponse{}, false, err
	}
	if shouldMaterializeGeneratedSummary(stateDir, novelID, upToEpisodeIndex, ok, doc) {
		materialized, err := MaterializeGeneratedSummary(stateDir, novelID)
		if err != nil {
			return SummaryResponse{}, false, err
		}
		if materialized {
			ok, err = readYAMLIfExists(path, &doc)
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
	if processed == nil || compareEpisodeIndex(*processed, upToEpisodeIndex) < 0 {
		status = "not_generated"
	}
	characters := []Character{}
	if status == "ready" {
		visibleProfiles := []characterProfile{}
		for _, profile := range doc.Characters {
			if !episodeWithin(profile.FirstAppearanceEpisodeIndex, upToEpisodeIndex) {
				continue
			}
			visibleProfiles = append(visibleProfiles, profile)
		}
		importanceByCharacterID := buildImportanceClassifications(visibleProfiles, episodeIndexes, upToEpisodeIndex)
		for _, profile := range visibleProfiles {
			characters = append(characters, profile.toCharacter(upToEpisodeIndex, importanceByCharacterID[profile.CharacterID]))
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
	doc := profilesDocument{
		NovelID:                   novelID,
		ProcessedUpToEpisodeIndex: &processedUpToEpisodeIndex,
		Characters:                buildHeuristicProfiles(novelID, processedUpToEpisodeIndex, episodes),
	}
	return writeYAMLAtomic(filepath.Join(stateDir, "character_profiles", novelID+".yaml"), doc)
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
			eventsDoc.UnresolvedMentions = truncateUnresolvedMentionsBeforeEpisode(eventsDoc.UnresolvedMentions, replaceFromEpisodeIndex)
			eventsDoc.EpisodeEtags = truncateEpisodeEtagsBeforeEpisode(eventsDoc.EpisodeEtags, replaceFromEpisodeIndex)
		}
		applyGeneratedCharacterIDState(&eventsDoc, novelID, options)
		eventsDoc.Characters = mergeRetiredCharacterEventRecords(eventsDoc.Characters, eventsDoc.RetiredCharacterIDs)
		profiles := generatedCharactersToProfiles(novelID, processedUpToEpisodeIndex, assigned)
		applyGeneratedImportanceMetrics(profiles, episodes)
		eventsDoc.Characters = generatedCharactersToEventRecordsWithExisting(profiles, assigned, processedUpToEpisodeIndex, eventsDoc.Characters)
		if options.SetUnresolvedMentions {
			eventsDoc.UnresolvedMentions = unresolvedMentionsToEventRecords(options.UnresolvedMentions)
		}
		eventsDoc.EpisodeEtags = mergeEpisodeEtags(eventsDoc.EpisodeEtags, episodes)
		materializedProfiles := eventRecordsToProfiles(eventsDoc.Characters)
		doc := profilesDocument{
			NovelID:                   novelID,
			ProcessedUpToEpisodeIndex: &processedUpToEpisodeIndex,
			Characters:                materializedProfiles,
		}
		if err := writeYAMLAtomic(filepath.Join(stateDir, "character_events", novelID+".yaml"), eventsDoc); err != nil {
			return err
		}
		return writeYAMLAtomic(filepath.Join(stateDir, "character_profiles", novelID+".yaml"), doc)
	})
}

func MaterializeGeneratedSummary(stateDir string, novelID string) (bool, error) {
	materialized := false
	err := novelstate.WithLock(novelID, func() error {
		doc, ok, err := loadCharacterEventsDocument(stateDir, novelID)
		if err != nil || !ok || doc.ProcessedUpToEpisodeIndex == nil {
			return err
		}
		profiles := eventRecordsToProfiles(doc.Characters)
		profileDoc := profilesDocument{
			NovelID:                   novelID,
			ProcessedUpToEpisodeIndex: doc.ProcessedUpToEpisodeIndex,
			Characters:                profiles,
		}
		if err := writeYAMLAtomic(filepath.Join(stateDir, "character_profiles", novelID+".yaml"), profileDoc); err != nil {
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
	return eventRecordsToGeneratedCharacters(doc.Characters), doc.ProcessedUpToEpisodeIndex, doc.ProcessedUpToEpisodeIndex != nil || len(doc.Characters) > 0, nil
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
	return eventRecordsToGeneratedCharacters(truncated), previousProcessedEpisodeIndex(doc.ProcessedUpToEpisodeIndex, fromEpisodeIndex), true, nil
}
