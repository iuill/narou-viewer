package library

import (
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

func safeJoinUnder(rootDir string, relPath string) (string, bool) {
	rootDir = filepath.Clean(rootDir)
	normalized := normalizeRelativePath(relPath)
	if normalized == "" {
		return "", false
	}
	filePath := filepath.Clean(filepath.Join(rootDir, filepath.FromSlash(normalized)))
	rel, err := filepath.Rel(rootDir, filePath)
	if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", false
	}
	return filePath, true
}

func safeExistingFileUnder(rootDir string, relPath string) (string, fs.FileInfo, bool, error) {
	filePath, ok := safeJoinUnder(rootDir, relPath)
	if !ok {
		return "", nil, false, nil
	}
	info, err := os.Stat(filePath)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil, false, nil
	}
	if err != nil {
		return "", nil, false, err
	}
	if info.IsDir() {
		return "", nil, false, nil
	}
	if !isPathInsideAfterSymlinkEval(rootDir, filePath) {
		return "", nil, false, nil
	}
	return filePath, info, true, nil
}

func safeExistingDirUnder(rootDir string, relPath string) (string, fs.FileInfo, bool, error) {
	dirPath, ok := safeJoinUnder(rootDir, relPath)
	if !ok {
		return "", nil, false, nil
	}
	info, err := os.Stat(dirPath)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil, false, nil
	}
	if err != nil {
		return "", nil, false, err
	}
	if !info.IsDir() {
		return "", nil, false, nil
	}
	if !isPathInsideAfterSymlinkEval(rootDir, dirPath) {
		return "", nil, false, nil
	}
	return dirPath, info, true, nil
}

func isPathInsideAfterSymlinkEval(rootDir string, filePath string) bool {
	realRoot, err := filepath.EvalSymlinks(rootDir)
	if err != nil {
		return false
	}
	realFilePath, err := filepath.EvalSymlinks(filePath)
	if err != nil {
		return false
	}
	realRel, err := filepath.Rel(realRoot, realFilePath)
	if err != nil || realRel == "." || strings.HasPrefix(realRel, ".."+string(filepath.Separator)) || filepath.IsAbs(realRel) {
		return false
	}
	return true
}

func normalizeRelativePath(relPath string) string {
	trimmedPath := strings.TrimSpace(relPath)
	if trimmedPath == "" || filepath.IsAbs(trimmedPath) || path.IsAbs(trimmedPath) || strings.HasPrefix(trimmedPath, `\`) || hasWindowsVolumeName(trimmedPath) {
		return ""
	}
	var segments []string
	for _, segment := range strings.FieldsFunc(trimmedPath, func(r rune) bool { return r == '/' || r == '\\' }) {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			continue
		}
		if segment == "." || segment == ".." {
			return ""
		}
		segments = append(segments, segment)
	}
	return strings.Join(segments, "/")
}

func hasWindowsVolumeName(path string) bool {
	return len(path) >= 2 && path[1] == ':'
}
