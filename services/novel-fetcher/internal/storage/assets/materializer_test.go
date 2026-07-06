package assets

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"path/filepath"
	"strings"
	"testing"

	"narou-viewer/services/novel-fetcher/internal/fetcher"
	"narou-viewer/services/novel-fetcher/internal/model"
)

type fakeAssetFetcher struct {
	response fetcher.BinaryResponse
	err      error
	urls     []string
}

func (f *fakeAssetFetcher) FetchBytes(_ context.Context, rawURL string, _ fetcher.FetchPolicy) (fetcher.BinaryResponse, error) {
	f.urls = append(f.urls, rawURL)
	if f.err != nil {
		return fetcher.BinaryResponse{}, f.err
	}
	return f.response, nil
}

func TestNormalizeAssetURLRejectsLocalNetworkTargets(t *testing.T) {
	tests := []string{
		"http://localhost/image.png",
		"http://127.0.0.1/image.png",
		"http://10.0.0.1/image.png",
		"http://192.168.0.1/image.png",
		"http://[::1]/image.png",
		"data:image/png;base64,abc",
	}

	for _, test := range tests {
		if got := normalizeAssetURL(test, "https://ncode.syosetu.com/n1234ab/1/"); got != "" {
			t.Fatalf("normalizeAssetURL(%q) = %q, want empty", test, got)
		}
	}
}

func TestMaterializerHelpers(t *testing.T) {
	if imageExtensionFromContentType("image/jpeg") != "jpg" ||
		imageExtensionFromContentType("image/gif") != "gif" ||
		imageExtensionFromContentType("image/webp") != "webp" ||
		imageExtensionFromContentType("image/bmp") != "bmp" ||
		imageExtensionFromContentType("text/plain") != "" {
		t.Fatal("imageExtensionFromContentType returned unexpected values")
	}
	if got := insertBeforeTagEnd(`" alt="x">`, ` width="3"`); got != `" alt="x" width="3">` {
		t.Fatalf("insertBeforeTagEnd normal = %q", got)
	}
	if got := insertBeforeTagEnd(`" />`, ` width="3"`); got != `"  width="3"/>` {
		t.Fatalf("insertBeforeTagEnd self-closing = %q", got)
	}
	if got := sha256Hex([]byte("asset")); !strings.HasPrefix(got, "sha256:") || len(got) != len("sha256:")+64 {
		t.Fatalf("sha256Hex returned %q", got)
	}
}

func TestMaterializerLocalizesEpisodeAssets(t *testing.T) {
	imageBytes := testPNG(t, 2, 3)
	assetFetcher := &fakeAssetFetcher{
		response: fetcher.BinaryResponse{Bytes: imageBytes, ContentType: "image/png"},
	}
	materializer := NewMaterializer("/library", assetFetcher, fetcher.FetchPolicy{})
	writes := map[string][]byte{}

	episode, assets, err := materializer.LocalizeEpisodeAssets(context.Background(), LocalizeInput{
		WorkDirectory: "works/12",
		WorkID:        12,
		EpisodeID:     "episode-1",
		WorkURL:       "https://example.com/work/",
		Episode: model.Episode{
			Href: "episodes/1",
			Element: model.EpisodeElement{
				DataType: "html",
				Body:     `<p><img src="../images/sample.png" alt="sample"></p>`,
			},
		},
		WriteFile: func(path string, body []byte) error {
			writes[path] = append([]byte(nil), body...)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("LocalizeEpisodeAssets returned error: %v", err)
	}
	if len(assetFetcher.urls) != 1 || assetFetcher.urls[0] != "https://example.com/work/images/sample.png" {
		t.Fatalf("unexpected fetched URLs: %+v", assetFetcher.urls)
	}
	if len(assets) != 1 {
		t.Fatalf("expected one localized asset, got %+v", assets)
	}
	asset := assets[0]
	if asset.MediaType != "image/png" || asset.ByteLength != len(imageBytes) || asset.Width != 2 || asset.Height != 3 {
		t.Fatalf("unexpected asset metadata: %+v", asset)
	}
	if !strings.Contains(episode.Element.Body, `src="../assets/episodes/episode-1/0-`) ||
		!strings.Contains(episode.Element.Body, `width="2"`) ||
		!strings.Contains(episode.Element.Body, `height="3"`) {
		t.Fatalf("episode body was not rewritten with local asset and size attributes: %s", episode.Element.Body)
	}
	if len(writes) != 1 {
		t.Fatalf("expected one asset write, got %+v", writes)
	}
	for path, body := range writes {
		if !strings.HasPrefix(path, filepath.Join("/library", "works", "12", "assets", "episodes", "episode-1")) {
			t.Fatalf("unexpected write path: %s", path)
		}
		if !bytes.Equal(body, imageBytes) {
			t.Fatal("written bytes did not match fetched image")
		}
	}
}

func TestMaterializerSkipsNonFatalAssetFailures(t *testing.T) {
	materializer := NewMaterializer("/library", &fakeAssetFetcher{err: errors.New("network failed")}, fetcher.FetchPolicy{})
	input := LocalizeInput{
		WorkDirectory: "works/1",
		WorkID:        1,
		EpisodeID:     "1",
		WorkURL:       "https://example.com/work/",
		Episode: model.Episode{Element: model.EpisodeElement{
			DataType: "html",
			Body:     `<img src="https://example.com/image.png">`,
		}},
		WriteFile: func(string, []byte) error {
			t.Fatal("WriteFile should not be called for skipped assets")
			return nil
		},
	}
	episode, assets, err := materializer.LocalizeEpisodeAssets(context.Background(), input)
	if err != nil {
		t.Fatalf("LocalizeEpisodeAssets should ignore non-fatal fetch errors: %v", err)
	}
	if len(assets) != 0 || episode.Element.Body != input.Episode.Element.Body {
		t.Fatalf("asset should be skipped without rewriting, episode=%+v assets=%+v", episode, assets)
	}

	materializer = NewMaterializer("/library", &fakeAssetFetcher{err: context.Canceled}, fetcher.FetchPolicy{})
	if _, _, err := materializer.LocalizeEpisodeAssets(context.Background(), input); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation should be propagated, got %v", err)
	}
}

func testPNG(t *testing.T, width int, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	img.Set(0, 0, color.RGBA{R: 1, G: 2, B: 3, A: 255})
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buffer.Bytes()
}
