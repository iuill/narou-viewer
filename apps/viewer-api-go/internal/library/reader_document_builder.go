package library

import "strings"

func readerDocument(chapter string, subchapter string, title string, sections map[string]string) ReaderDocument {
	blocks := []ReaderBlock{}
	if chapter != "" {
		blocks = append(blocks, ReaderBlock{Type: "meta", Role: "chapter", Text: chapter})
	}
	if subchapter != "" {
		blocks = append(blocks, ReaderBlock{Type: "meta", Role: "subchapter", Text: subchapter})
	}
	blocks = append(blocks, ReaderBlock{Type: "title", Text: title})
	hasPreviousSection := false
	for _, section := range []string{"introduction", "body", "postscript"} {
		sectionHTML := sections[section]
		if strings.TrimSpace(sectionHTML) == "" {
			continue
		}
		sectionBlocks := buildReaderSectionBlocks(sectionHTML, section)
		if len(sectionBlocks) == 0 {
			continue
		}
		if hasPreviousSection {
			blocks = append(blocks, readerSectionSeparatorBlock(section))
		}
		blocks = append(blocks, sectionBlocks...)
		hasPreviousSection = true
	}
	return ReaderDocument{Version: 1, Blocks: blocks}
}
