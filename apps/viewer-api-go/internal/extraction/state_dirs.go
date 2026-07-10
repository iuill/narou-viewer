package extraction

import (
	"errors"
	"log"
	"os"
	"path/filepath"
)

func EnsureStateDirs(stateDir string) error {
	legacyDir := filepath.Join(stateDir, "character_jobs")
	jobsDir := filepath.Join(stateDir, "extraction_jobs")
	if _, err := os.Stat(jobsDir); errors.Is(err, os.ErrNotExist) {
		if _, legacyErr := os.Stat(legacyDir); legacyErr == nil {
			if renameErr := os.Rename(legacyDir, jobsDir); renameErr != nil {
				log.Printf("extraction: legacy job directory migration failed: %v", renameErr)
			}
		}
	}
	return os.MkdirAll(filepath.Join(stateDir, "extraction_jobs", "index"), 0o755)
}
