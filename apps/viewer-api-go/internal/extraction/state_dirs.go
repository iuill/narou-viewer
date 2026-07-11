package extraction

import (
	"bytes"
	"errors"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func EnsureStateDirs(stateDir string) error {
	jobsMu.Lock()
	defer jobsMu.Unlock()

	legacyDir := filepath.Join(stateDir, "character_jobs")
	jobsDir := filepath.Join(stateDir, "extraction_jobs")
	if _, err := os.Stat(jobsDir); errors.Is(err, os.ErrNotExist) {
		if _, legacyErr := os.Stat(legacyDir); legacyErr == nil {
			if renameErr := os.Rename(legacyDir, jobsDir); renameErr != nil {
				log.Printf("extraction: legacy job directory migration failed: %v", renameErr)
			}
		}
	}
	if err := os.MkdirAll(filepath.Join(jobsDir, "index"), 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(legacyDir); err == nil {
		migrateLegacyJobFiles(legacyDir, jobsDir)
	}
	return nil
}

func migrateLegacyJobFiles(legacyDir string, jobsDir string) {
	err := filepath.WalkDir(legacyDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			log.Printf("extraction: legacy job migration skipped %s: %v", path, walkErr)
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(legacyDir, path)
		if err != nil {
			log.Printf("extraction: legacy job migration could not resolve %s: %v", path, err)
			return nil
		}
		destination := filepath.Join(jobsDir, relative)
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			log.Printf("extraction: legacy job migration could not create destination for %s: %v", path, err)
			return nil
		}
		if existing, err := os.ReadFile(destination); err == nil {
			legacy, legacyErr := os.ReadFile(path)
			if legacyErr == nil && bytes.Equal(existing, legacy) {
				if removeErr := os.Remove(path); removeErr != nil {
					log.Printf("extraction: legacy duplicate could not be removed %s: %v", path, removeErr)
				}
				return nil
			}
			destination = availableLegacyConflictPath(filepath.Join(jobsDir, "legacy_conflicts", relative))
			if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
				log.Printf("extraction: legacy job conflict could not be archived %s: %v", path, err)
				return nil
			}
			log.Printf("extraction: legacy job conflict kept canonical destination and archived %s as %s", path, destination)
		} else if !errors.Is(err, os.ErrNotExist) {
			log.Printf("extraction: legacy job migration could not inspect destination %s: %v", destination, err)
			return nil
		}
		if err := os.Rename(path, destination); err != nil {
			log.Printf("extraction: legacy job migration failed for %s: %v", path, err)
		}
		return nil
	})
	if err != nil {
		log.Printf("extraction: legacy job directory migration failed: %v", err)
		return
	}
	removeEmptyLegacyDirectories(legacyDir)
}

func availableLegacyConflictPath(path string) string {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return path
	}
	extension := filepath.Ext(path)
	base := strings.TrimSuffix(path, extension)
	for suffix := 2; ; suffix++ {
		candidate := base + "." + strconv.Itoa(suffix) + extension
		if _, err := os.Stat(candidate); errors.Is(err, os.ErrNotExist) {
			return candidate
		}
	}
}

func removeEmptyLegacyDirectories(root string) {
	directories := []string{}
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err == nil && entry.IsDir() {
			directories = append(directories, path)
		}
		return nil
	})
	for index := len(directories) - 1; index >= 0; index-- {
		if err := os.Remove(directories[index]); err != nil && !errors.Is(err, os.ErrNotExist) {
			if !errors.Is(err, fs.ErrInvalid) {
				log.Printf("extraction: migrated legacy job directory remains %s: %v", directories[index], err)
			}
		}
	}
}
