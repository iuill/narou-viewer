package assets

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"narou-viewer/services/novel-fetcher/internal/fetcher"
	"narou-viewer/services/novel-fetcher/internal/model"
	"narou-viewer/services/novel-fetcher/internal/storage/pathutil"
)

type Fetcher interface {
	FetchBytes(ctx context.Context, rawURL string, policy fetcher.FetchPolicy) (fetcher.BinaryResponse, error)
}

type Asset struct {
	AssetID     string
	SourceURL   string
	StoragePath string
	MediaType   string
	ByteLength  int
	Width       int
	Height      int
	ContentHash string
}

type Materializer struct {
	rootDir string
	fetcher Fetcher
	policy  fetcher.FetchPolicy
}

func NewMaterializer(rootDir string, assetFetcher Fetcher, policy fetcher.FetchPolicy) *Materializer {
	if assetFetcher == nil {
		return nil
	}
	return &Materializer{
		rootDir: rootDir,
		fetcher: assetFetcher,
		policy:  policy,
	}
}

func (m *Materializer) LocalizeEpisodeAssets(ctx context.Context, input LocalizeInput) (model.Episode, []Asset, error) {
	if m == nil || m.fetcher == nil || input.Episode.Element.DataType == "text" {
		return input.Episode, nil, nil
	}

	episode := input.Episode
	episodeURL := resolveURL(input.WorkURL, episode.Href)
	assetIndex := 0
	localizedAssets := []Asset{}
	localize := func(html string) (string, error) {
		var localizeErr error
		localized := imageSrcPattern.ReplaceAllStringFunc(html, func(match string) string {
			if localizeErr != nil {
				return match
			}
			parts := imageSrcPattern.FindStringSubmatch(match)
			if len(parts) != 4 {
				return match
			}

			asset, relPath, ok, err := m.saveRemoteAsset(ctx, input, assetIndex, parts[2], episodeURL)
			assetIndex++
			if err != nil {
				localizeErr = err
				return match
			}
			if !ok {
				return match
			}
			localizedAssets = append(localizedAssets, asset)
			return parts[1] + relPath + injectImageSizeAttributes(parts[3], asset.Width, asset.Height)
		})
		return localized, localizeErr
	}

	var err error
	if episode.Element.Introduction, err = localize(episode.Element.Introduction); err != nil {
		return model.Episode{}, nil, err
	}
	if episode.Element.Body, err = localize(episode.Element.Body); err != nil {
		return model.Episode{}, nil, err
	}
	if episode.Element.Postscript, err = localize(episode.Element.Postscript); err != nil {
		return model.Episode{}, nil, err
	}
	return episode, localizedAssets, nil
}

type LocalizeInput struct {
	WorkDirectory string
	WorkID        int
	EpisodeID     string
	WorkURL       string
	Episode       model.Episode
	WriteFile     func(string, []byte) error
}

func (m *Materializer) saveRemoteAsset(ctx context.Context, input LocalizeInput, assetIndex int, source string, episodeURL string) (Asset, string, bool, error) {
	assetURL := normalizeAssetURL(source, episodeURL)
	if assetURL == "" {
		return Asset{}, "", false, nil
	}

	response, err := m.fetcher.FetchBytes(ctx, assetURL, m.policy)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return Asset{}, "", false, err
		}
		return Asset{}, "", false, nil
	}

	mediaType := response.ContentType
	extension := imageExtensionFromContentType(mediaType)
	if extension == "" {
		return Asset{}, "", false, nil
	}

	width, height := decodeImageSize(response.Bytes)
	contentHash := sha256Hex(response.Bytes)
	hashPart := strings.TrimPrefix(contentHash, "sha256:")
	if len(hashPart) > 16 {
		hashPart = hashPart[:16]
	}

	assetID := strconv.Itoa(input.WorkID) + ":" + input.EpisodeID + ":" + strconv.Itoa(assetIndex) + ":" + hashPart
	fileName := strconv.Itoa(assetIndex) + "-" + hashPart + "." + extension
	storagePath := filepath.ToSlash(filepath.Join(input.WorkDirectory, "assets", "episodes", pathutil.Segment(input.EpisodeID), fileName))
	if err := input.WriteFile(filepath.Join(m.rootDir, storagePath), response.Bytes); err != nil {
		return Asset{}, "", false, err
	}

	episodeBodyDir := filepath.ToSlash(filepath.Join(input.WorkDirectory, "episodes"))
	relPath, err := filepath.Rel(filepath.FromSlash(episodeBodyDir), filepath.FromSlash(storagePath))
	if err != nil {
		relPath = storagePath
	}
	relPath = filepath.ToSlash(relPath)

	return Asset{
		AssetID:     assetID,
		SourceURL:   assetURL,
		StoragePath: storagePath,
		MediaType:   mediaType,
		ByteLength:  len(response.Bytes),
		Width:       width,
		Height:      height,
		ContentHash: contentHash,
	}, relPath, true, nil
}

var imageSrcPattern = regexp.MustCompile(`(?i)(<img\b[^>]*\bsrc\s*=\s*["'])([^"']+)(["'][^>]*>)`)
var widthAttrPattern = regexp.MustCompile(`(?i)\bwidth\s*=`)
var heightAttrPattern = regexp.MustCompile(`(?i)\bheight\s*=`)

func normalizeAssetURL(source string, episodeURL string) string {
	trimmed := strings.TrimSpace(source)
	if trimmed == "" || strings.HasPrefix(trimmed, "data:") || strings.HasPrefix(trimmed, "#") {
		return ""
	}

	if strings.HasPrefix(trimmed, "//") {
		trimmed = "https:" + trimmed
	} else if strings.HasPrefix(trimmed, "/") || strings.HasPrefix(trimmed, "./") || strings.HasPrefix(trimmed, "../") {
		trimmed = resolveURL(episodeURL, trimmed)
	}

	if !strings.HasPrefix(trimmed, "http://") && !strings.HasPrefix(trimmed, "https://") {
		return ""
	}
	if err := fetcher.ValidateRemoteURL(trimmed); err != nil {
		return ""
	}

	if strings.Contains(trimmed, ".mitemin.net") {
		trimmed = strings.Replace(trimmed, "viewimagebig", "viewimage", 1)
	}
	return trimmed
}

func resolveURL(base string, href string) string {
	if strings.TrimSpace(href) == "" {
		return href
	}

	baseURL, err := url.Parse(base)
	if err != nil {
		return href
	}
	ref, err := url.Parse(href)
	if err != nil {
		return href
	}
	return baseURL.ResolveReference(ref).String()
}

func imageExtensionFromContentType(contentType string) string {
	switch strings.ToLower(strings.TrimSpace(contentType)) {
	case "image/jpeg":
		return "jpg"
	case "image/png":
		return "png"
	case "image/gif":
		return "gif"
	case "image/webp":
		return "webp"
	case "image/bmp":
		return "bmp"
	default:
		return ""
	}
}

func decodeImageSize(imageBytes []byte) (int, int) {
	config, _, err := image.DecodeConfig(bytes.NewReader(imageBytes))
	if err != nil {
		return 0, 0
	}
	return config.Width, config.Height
}

func injectImageSizeAttributes(suffix string, width int, height int) string {
	result := suffix
	if width > 0 && !widthAttrPattern.MatchString(result) {
		result = insertBeforeTagEnd(result, ` width="`+strconv.Itoa(width)+`"`)
	}
	if height > 0 && !heightAttrPattern.MatchString(result) {
		result = insertBeforeTagEnd(result, ` height="`+strconv.Itoa(height)+`"`)
	}
	return result
}

func insertBeforeTagEnd(tagSuffix string, attribute string) string {
	trimmedRight := strings.TrimRight(tagSuffix, " \t\r\n")
	if strings.HasSuffix(trimmedRight, "/>") {
		index := strings.LastIndex(tagSuffix, "/>")
		return tagSuffix[:index] + attribute + tagSuffix[index:]
	}
	index := strings.LastIndex(tagSuffix, ">")
	if index < 0 {
		return tagSuffix + attribute
	}
	return tagSuffix[:index] + attribute + tagSuffix[index:]
}

func sha256Hex(bytes []byte) string {
	sum := sha256.Sum256(bytes)
	return "sha256:" + hex.EncodeToString(sum[:])
}
