package library

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
)

func (s *Service) ReadEpisodeDocument(episode Episode) (EpisodeDocument, error) {
	bodyPath, _, found, err := safeExistingFileUnder(s.rootDir, episode.BodyPath)
	if err != nil {
		return EpisodeDocument{}, err
	}
	if !found {
		return EpisodeDocument{}, os.ErrNotExist
	}
	bytes, err := os.ReadFile(bodyPath)
	if err != nil {
		return EpisodeDocument{}, err
	}
	var document CanonicalEpisode
	if err := json.Unmarshal(bytes, &document); err != nil {
		return EpisodeDocument{}, err
	}
	contentBlocks := episodeContentBlocksBySection(document.Blocks)
	htmlSections := mapSections(contentBlocks, blocksToHTML)
	plainSections := mapSections(contentBlocks, blocksToText)
	bodyHTML := joinSections(htmlSections)
	plain := joinSections(plainSections)
	hash := sha256.Sum256(bytes)
	return EpisodeDocument{
		Episode:       episode,
		Document:      document,
		HTML:          bodyHTML,
		Plain:         plain,
		HTMLSections:  htmlSections,
		PlainSections: plainSections,
		Etag:          `"` + hex.EncodeToString(hash[:]) + `"`,
	}, nil
}
