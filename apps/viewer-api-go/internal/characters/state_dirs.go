package characters

import (
	"os"
	"path/filepath"
)

func EnsureStateDirs(stateDir string) error {
	for _, dir := range []string{
		filepath.Join(stateDir, "character_profiles"),
		filepath.Join(stateDir, "character_events"),
		filepath.Join(stateDir, "character_jobs", "index"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}
