package extraction

import (
	"log"
	"os"
	"path/filepath"
)

func EnsureStateDirs(stateDir string) error {
	jobsMu.Lock()
	defer jobsMu.Unlock()

	jobsDir := filepath.Join(stateDir, "extraction_jobs")
	for _, obsoletePath := range []string{
		filepath.Join(stateDir, "character_jobs"),
		filepath.Join(jobsDir, "legacy_conflicts"),
	} {
		if err := os.RemoveAll(obsoletePath); err != nil {
			log.Printf("extraction: obsolete job artifacts could not be removed %s: %v", obsoletePath, err)
		}
	}
	return os.MkdirAll(filepath.Join(jobsDir, "index"), 0o755)
}
