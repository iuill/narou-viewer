package characters

import "narou-viewer/apps/viewer-api-go/internal/state/schemaguard"

const (
	characterEventsSchemaVersion   = 1
	characterProfilesSchemaVersion = 1
)

var CharacterEventsSchemaContract = schemaguard.Contract{
	ID:                   "VA-CHAR-EVENTS",
	Path:                 "character_events/*.yaml",
	Current:              characterEventsSchemaVersion,
	ReadableLegacy:       []int{0},
	MissingPolicy:        schemaguard.MissingTreatAsLegacy,
	MissingLegacyVersion: 0,
}

var CharacterProfilesSchemaContract = schemaguard.Contract{
	ID:                   "VA-CHAR-PROFILES",
	Path:                 "character_profiles/*.yaml",
	Current:              characterProfilesSchemaVersion,
	ReadableLegacy:       []int{0},
	MissingPolicy:        schemaguard.MissingTreatAsLegacy,
	MissingLegacyVersion: 0,
}
