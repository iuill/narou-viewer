package statebackup

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type archiveCandidate struct {
	path    string
	modTime time.Time
}

func PruneArchives(directory string, policy RetentionPolicy) ([]string, error) {
	if err := validateRetentionPolicy(policy); err != nil {
		return nil, err
	}
	now := time.Now
	if policy.Now != nil {
		now = policy.Now
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, err
	}
	candidates := []archiveCandidate{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "narou-viewer-") || !strings.HasSuffix(entry.Name(), ArchiveSuffix) {
			continue
		}
		path := filepath.Join(directory, entry.Name())
		info, err := os.Lstat(path)
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return nil, fmt.Errorf("retention candidate must be a regular non-symlink file: %s", path)
		}
		candidates = append(candidates, archiveCandidate{path: path, modTime: info.ModTime()})
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].modTime.Equal(candidates[j].modTime) {
			return candidates[i].path > candidates[j].path
		}
		return candidates[i].modTime.After(candidates[j].modTime)
	})
	removed := []string{}
	for index, candidate := range candidates {
		if index < policy.KeepGenerations {
			continue
		}
		if now().Sub(candidate.modTime) <= policy.MaxAge {
			continue
		}
		if err := os.Remove(candidate.path); err != nil {
			return removed, err
		}
		removed = append(removed, candidate.path)
	}
	if len(removed) > 0 {
		if err := syncDirectory(directory); err != nil {
			return removed, err
		}
	}
	return removed, nil
}

func validateRetentionPolicy(policy RetentionPolicy) error {
	if policy.KeepGenerations < 1 {
		return errors.New("retention keep generations must be at least 1")
	}
	if policy.MaxAge <= 0 {
		return errors.New("retention max age must be positive")
	}
	return nil
}
