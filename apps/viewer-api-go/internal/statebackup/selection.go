package statebackup

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"narou-viewer/apps/viewer-api-go/internal/publications"
	"narou-viewer/apps/viewer-api-go/internal/state/aisettings"
	"narou-viewer/apps/viewer-api-go/internal/state/bookmarks"
	"narou-viewer/apps/viewer-api-go/internal/state/novelsettings"
	"narou-viewer/apps/viewer-api-go/internal/state/preferences"
	"narou-viewer/apps/viewer-api-go/internal/state/readingstate"
)

type sourceFile struct {
	absolute string
	record   FileRecord
}

func collectPayloadFiles(dataDir string) ([]sourceFile, error) {
	dataDir = filepath.Clean(dataDir)
	files := []sourceFile{}
	addExact := func(relative string, group string) error {
		path := filepath.Join(dataDir, filepath.FromSlash(relative))
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("backup payload must be a regular non-symlink file: %s", relative)
		}
		files = append(files, sourceFile{absolute: path, record: FileRecord{Path: relative, Group: group, Size: info.Size(), Mode: uint32(info.Mode().Perm())}})
		return nil
	}
	for _, relative := range []string{
		"novel-fetcher/library.sqlite",
		"novel-fetcher/library.sqlite-wal",
	} {
		if err := addExact(relative, GroupNFCanonical); err != nil {
			return nil, err
		}
	}
	if err := walkPayloadDir(dataDir, "novel-fetcher/works", GroupNFCanonical, func(string) bool { return true }, &files); err != nil {
		return nil, err
	}
	for _, name := range []string{
		readingstate.FileName,
		bookmarks.FileName,
		preferences.FileName,
		novelsettings.FileName,
		aisettings.FileName,
		publications.FileName,
	} {
		if err := addExact(filepath.ToSlash(filepath.Join("state", name)), GroupVACore); err != nil {
			return nil, err
		}
	}
	for _, dir := range []string{"state/character_events", "state/term_profiles"} {
		if err := walkPayloadDir(dataDir, dir, GroupVAExtraction, func(relative string) bool {
			return filepath.Ext(relative) == ".yaml" && filepath.Dir(relative) == dir
		}, &files); err != nil {
			return nil, err
		}
	}
	if err := walkPayloadDir(dataDir, "state/extraction_jobs", GroupVAExtraction, extractionPayloadPath, &files); err != nil {
		return nil, err
	}
	for _, relative := range []string{"state/ai_usage.sqlite", "state/ai_usage.sqlite-journal"} {
		if err := addExact(relative, GroupVAHistory); err != nil {
			return nil, err
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].record.Path < files[j].record.Path })
	return files, nil
}

func walkPayloadDir(dataDir string, relativeRoot string, group string, include func(string) bool, files *[]sourceFile) error {
	absoluteRoot := filepath.Join(dataDir, filepath.FromSlash(relativeRoot))
	rootInfo, err := os.Lstat(absoluteRoot)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		return fmt.Errorf("backup payload root must be a non-symlink directory: %s", relativeRoot)
	}
	return filepath.WalkDir(absoluteRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(dataDir, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if entry.IsDir() {
			if relative == "state/extraction_jobs/index" {
				return filepath.SkipDir
			}
			return nil
		}
		if !include(relative) {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("backup payload must be a regular non-symlink file: %s", relative)
		}
		*files = append(*files, sourceFile{absolute: path, record: FileRecord{Path: relative, Group: group, Size: info.Size(), Mode: uint32(info.Mode().Perm())}})
		return nil
	})
}

func extractionPayloadPath(relative string) bool {
	relative = filepath.ToSlash(relative)
	if filepath.ToSlash(filepath.Dir(relative)) == "state/extraction_jobs" {
		return filepath.Ext(relative) == ".yaml"
	}
	return filepath.ToSlash(filepath.Dir(relative)) == "state/extraction_jobs/checkpoints" && filepath.Ext(relative) == ".json"
}

func groupForPayloadPath(relative string) (string, bool) {
	relative = filepath.ToSlash(filepath.Clean(filepath.FromSlash(relative)))
	if relative == "." || relative == "" || strings.HasPrefix(relative, "../") || filepath.IsAbs(relative) {
		return "", false
	}
	if relative == "novel-fetcher/library.sqlite" || relative == "novel-fetcher/library.sqlite-wal" || strings.HasPrefix(relative, "novel-fetcher/works/") {
		return GroupNFCanonical, true
	}
	for _, name := range []string{
		readingstate.FileName,
		bookmarks.FileName,
		preferences.FileName,
		novelsettings.FileName,
		aisettings.FileName,
		publications.FileName,
	} {
		if relative == filepath.ToSlash(filepath.Join("state", name)) {
			return GroupVACore, true
		}
	}
	if (strings.HasPrefix(relative, "state/character_events/") || strings.HasPrefix(relative, "state/term_profiles/")) && filepath.Ext(relative) == ".yaml" {
		return GroupVAExtraction, true
	}
	if extractionPayloadPath(relative) {
		return GroupVAExtraction, true
	}
	if relative == "state/ai_usage.sqlite" || relative == "state/ai_usage.sqlite-journal" {
		return GroupVAHistory, true
	}
	return "", false
}

func groupForSchema(schemaID string) (string, bool) {
	switch schemaID {
	case "NF-LIBRARY", "NF-CANONICAL-EPISODE":
		return GroupNFCanonical, true
	case "VA-READING", "VA-BOOKMARKS", "VA-PREFERENCES", "VA-NOVEL-SETTINGS", "VA-AI-SETTINGS", "VA-AI-SETTINGS-CRYPTO", "VA-PUBLICATIONS":
		return GroupVACore, true
	case "VA-CHAR-EVENTS", "VA-TERM-PROFILES", "VA-EXTRACTION-JOBS", "VA-EXTRACTION-CHECKPOINT":
		return GroupVAExtraction, true
	case "VA-AI-USAGE":
		return GroupVAHistory, true
	case "VA-CHAR-PROFILES", "VA-EXTRACTION-INDEX", "VA-READER-SEARCH":
		return GroupVACache, true
	default:
		return "", false
	}
}
