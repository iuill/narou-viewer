package library

import (
	"context"
	"mime"
	"path/filepath"
	"strings"
)

func (s *Service) GetAsset(ctx context.Context, novelID string, assetPath string) (*AssetResponse, error) {
	_ = ctx
	normalized := normalizeAssetPath(assetPath)
	if normalized == "" || !strings.HasPrefix(normalized, "assets/") {
		return nil, nil
	}
	work, ok, err := s.FindWork(novelID)
	if err != nil || !ok || strings.TrimSpace(work.Directory) == "" {
		return nil, err
	}
	workRoot, _, found, err := safeExistingDirUnder(s.rootDir, work.Directory)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	filePath, _, found, err := safeExistingFileUnder(workRoot, normalized)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	mediaType := mime.TypeByExtension(filepath.Ext(filePath))
	if mediaType == "" {
		mediaType = "application/octet-stream"
	}
	return &AssetResponse{FilePath: filePath, MediaType: mediaType}, nil
}
