package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"narou-viewer/apps/viewer-api-go/internal/ai"
	"narou-viewer/apps/viewer-api-go/internal/application/readerproofread"
	"narou-viewer/apps/viewer-api-go/internal/library"
	"narou-viewer/apps/viewer-api-go/internal/store"
)

type routeProofreadLibrary struct{}

func (routeProofreadLibrary) GetEpisode(context.Context, string, string) (*library.EpisodeResponse, error) {
	return &library.EpisodeResponse{
		NovelID: "novel-a", EpisodeIndex: "1", ContentEtag: "etag-a",
		ReaderDocument: library.ReaderDocument{Version: 1, Blocks: []library.ReaderBlock{{
			Type: "paragraph", Section: "body", Inlines: []library.ReaderInline{{Type: "text", Text: "本 文"}},
		}}},
	}, nil
}

type routeProofreadSettings struct{}

func (routeProofreadSettings) ResolveActiveAIGenerationConfig() (*store.ResolvedAIGenerationConfig, error) {
	return &store.ResolvedAIGenerationConfig{APIKey: "key", ModelID: "model"}, nil
}

func (routeProofreadSettings) GetNovelReaderSettings(string) (store.NovelReaderSettings, error) {
	return store.NovelReaderSettings{}, nil
}

func TestReaderAIProofreadRoutes(t *testing.T) {
	service := readerproofread.NewService(readerproofread.Dependencies{
		Library: routeProofreadLibrary{}, Settings: routeProofreadSettings{}, StateDir: t.TempDir(),
		Generate: func(context.Context, ai.OpenRouterConfig, []ai.ChatMessage) (ai.ChatResult, error) {
			answer, _ := json.Marshal(map[string]any{
				"segments": []any{map[string]any{"id": 0, "paragraphs": []string{"本文"}}},
			})
			return ai.ChatResult{Answer: string(answer)}, nil
		},
	})
	handler := NewServerWithDependencies(ServerDependencies{DataDir: t.TempDir(), ReaderProofread: service})
	path := "/api/library/novels/novel-a/episodes/1/ai-proofread"

	get := httptest.NewRecorder()
	handler.ServeHTTP(get, httptest.NewRequest(http.MethodGet, path, nil))
	if get.Code != http.StatusOK || !jsonContainsStatus(t, get.Body.Bytes(), "not_generated") {
		t.Fatalf("GET status=%d body=%s", get.Code, get.Body.String())
	}

	post := httptest.NewRecorder()
	postRequest := httptest.NewRequest(http.MethodPost, path, nil)
	postRequest.Header.Set(apiContractVersionHeader, apiContractVersion)
	handler.ServeHTTP(post, postRequest)
	if post.Code != http.StatusOK || !jsonContainsStatus(t, post.Body.Bytes(), "ready") {
		t.Fatalf("POST status=%d body=%s", post.Code, post.Body.String())
	}

	deleteResponse := httptest.NewRecorder()
	deleteRequest := httptest.NewRequest(http.MethodDelete, path, nil)
	deleteRequest.Header.Set(apiContractVersionHeader, apiContractVersion)
	handler.ServeHTTP(deleteResponse, deleteRequest)
	if deleteResponse.Code != http.StatusNoContent {
		t.Fatalf("DELETE status=%d body=%s", deleteResponse.Code, deleteResponse.Body.String())
	}

	bad := httptest.NewRecorder()
	handler.ServeHTTP(bad, httptest.NewRequest(http.MethodGet, "/api/library/novels/novel-a/episodes/nope/ai-proofread", nil))
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("bad status=%d body=%s", bad.Code, bad.Body.String())
	}

	method := httptest.NewRecorder()
	methodRequest := httptest.NewRequest(http.MethodPut, path, nil)
	methodRequest.Header.Set(apiContractVersionHeader, apiContractVersion)
	handler.ServeHTTP(method, methodRequest)
	if method.Code != http.StatusMethodNotAllowed {
		t.Fatalf("method status=%d body=%s", method.Code, method.Body.String())
	}
}

func jsonContainsStatus(t *testing.T, raw []byte, want string) bool {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	return payload["status"] == want
}

func TestReaderAIProofreadRouteErrors(t *testing.T) {
	path := "/api/library/novels/novel-a/episodes/1/ai-proofread"

	unavailable := httptest.NewRecorder()
	(&Server{}).handleEpisodeAIProofread(unavailable, httptest.NewRequest(http.MethodGet, path, nil), "novel-a", "1")
	if unavailable.Code != http.StatusServiceUnavailable {
		t.Fatalf("unavailable status=%d", unavailable.Code)
	}

	noConfigService := readerproofread.NewService(readerproofread.Dependencies{
		Library: routeProofreadLibrary{}, StateDir: t.TempDir(),
	})
	noConfig := httptest.NewRecorder()
	(&Server{readerProofread: noConfigService}).handleEpisodeAIProofread(
		noConfig, httptest.NewRequest(http.MethodPost, path, nil), "novel-a", "1",
	)
	if noConfig.Code != http.StatusServiceUnavailable {
		t.Fatalf("no config status=%d body=%s", noConfig.Code, noConfig.Body.String())
	}

	richService := readerproofread.NewService(readerproofread.Dependencies{
		Library: routeRichProofreadLibrary{}, Settings: routeProofreadSettings{}, StateDir: t.TempDir(),
	})
	rich := httptest.NewRecorder()
	(&Server{readerProofread: richService}).handleEpisodeAIProofread(
		rich, httptest.NewRequest(http.MethodPost, path, nil), "novel-a", "1",
	)
	if rich.Code != http.StatusUnprocessableEntity {
		t.Fatalf("rich status=%d body=%s", rich.Code, rich.Body.String())
	}
}

type routeRichProofreadLibrary struct{}

func (routeRichProofreadLibrary) GetEpisode(context.Context, string, string) (*library.EpisodeResponse, error) {
	return &library.EpisodeResponse{
		NovelID: "novel-a", EpisodeIndex: "1", ContentEtag: "etag-a",
		ReaderDocument: library.ReaderDocument{Version: 1, Blocks: []library.ReaderBlock{{
			Type: "paragraph", Section: "body",
			Inlines: []library.ReaderInline{{Type: "ruby", Text: "本文", Ruby: "ほんぶん"}},
		}}},
	}, nil
}
