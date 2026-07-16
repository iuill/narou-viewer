package statedoctor

import (
	"narou-viewer/apps/viewer-api-go/internal/application/readertextcache"
	"narou-viewer/apps/viewer-api-go/internal/characters"
	"narou-viewer/apps/viewer-api-go/internal/extraction"
	"narou-viewer/apps/viewer-api-go/internal/extraction/checkpointstore"
	"narou-viewer/apps/viewer-api-go/internal/publications"
	"narou-viewer/apps/viewer-api-go/internal/state/aisettings"
	"narou-viewer/apps/viewer-api-go/internal/state/bookmarks"
	"narou-viewer/apps/viewer-api-go/internal/state/novelsettings"
	"narou-viewer/apps/viewer-api-go/internal/state/preferences"
	"narou-viewer/apps/viewer-api-go/internal/state/readingstate"
	"narou-viewer/apps/viewer-api-go/internal/state/schemaguard"
	"narou-viewer/apps/viewer-api-go/internal/terms"
)

type SchemaSupport struct {
	Versions       []int
	ThroughCurrent int
}

func SupportedSchemas() map[string]SchemaSupport {
	result := map[string]SchemaSupport{}
	for _, contract := range []schemaguard.Contract{
		readingstate.SchemaContract,
		bookmarks.SchemaContract,
		preferences.SchemaContract,
		novelsettings.SchemaContract,
		aisettings.SchemaContract,
		publications.SchemaContract,
		characters.CharacterEventsSchemaContract,
		characters.CharacterProfilesSchemaContract,
		terms.SchemaContract,
		extraction.JobSchemaContract,
		extraction.JobIndexSchemaContract,
		checkpointstore.SchemaContract,
	} {
		versions := append([]int{}, contract.ReadableLegacy...)
		versions = append(versions, contract.Current)
		result[contract.ID] = SchemaSupport{Versions: versions}
	}
	result["VA-AI-SETTINGS-CRYPTO"] = SchemaSupport{Versions: []int{0, aisettings.APIKeyCryptoVersion}}
	result["VA-AI-USAGE"] = SchemaSupport{ThroughCurrent: 1}
	result["VA-READER-SEARCH"] = SchemaSupport{Versions: []int{readertextcache.CacheVersion}}
	result["NF-LIBRARY"] = SchemaSupport{ThroughCurrent: nfLibrarySupportedMigration}
	result["NF-CANONICAL-EPISODE"] = SchemaSupport{Versions: []int{nfCanonicalEpisodeVersion}}
	return result
}

func SupportsSchemaVersion(schemaID string, observed int) bool {
	support, ok := SupportedSchemas()[schemaID]
	if !ok {
		return false
	}
	if support.ThroughCurrent > 0 {
		return observed >= 0 && observed <= support.ThroughCurrent
	}
	for _, version := range support.Versions {
		if version == observed {
			return true
		}
	}
	return false
}
