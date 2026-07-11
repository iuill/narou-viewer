package characters

import "strings"

func migrateLegacyProfilesToEvents(novelID string, profiles profilesDocument) characterEventsDocument {
	doc := characterEventsDocument{
		SchemaVersion:             1,
		NovelID:                   novelID,
		ProcessedUpToEpisodeIndex: profiles.ProcessedUpToEpisodeIndex,
		NextCharacterOrdinal:      1,
		IdentityMergeEvents:       append([]identityMergeEvent{}, profiles.IdentityMergeEvents...),
		Characters:                profilesToEventRecords(profiles.Characters),
	}
	used := map[string]bool{}
	for _, record := range doc.Characters {
		if strings.TrimSpace(record.CharacterID) != "" {
			used[record.CharacterID] = true
		}
	}
	advanceNextStableCharacterOrdinal(novelID, &doc.NextCharacterOrdinal, used)
	return doc
}
