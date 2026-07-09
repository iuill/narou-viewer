package extraction

import (
	"os"
	"path/filepath"
)

func EnsureStateDirs(stateDir string) error {
	return os.MkdirAll(filepath.Join(stateDir, "character_jobs", "index"), 0o755)
}
