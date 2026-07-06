package httpapi

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"narou-viewer/apps/viewer-api-go/internal/library"
	"narou-viewer/apps/viewer-api-go/internal/store"
)

func TestPublicationRoutes(t *testing.T) {
	ndlServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("isbn") != "9784040000008" {
			t.Fatalf("unexpected NDL request: %s", r.URL.RawQuery)
		}
		fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>
			<rss xmlns:dc="http://purl.org/dc/elements/1.1/">
				<channel>
					<item>
						<title>NDL書誌タイトル</title>
						<link>https://ndlsearch.ndl.go.jp/books/R100000002-I000000000</link>
						<dc:creator>NDL著者</dc:creator>
						<dc:publisher>NDL出版社</dc:publisher>
						<dc:date>2025</dc:date>
						<dc:identifier>ISBN 978-4-04-000000-8</dc:identifier>
					</item>
				</channel>
			</rss>`)
	}))
	defer ndlServer.Close()
	googleServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/volumes" || r.URL.Query().Get("q") != "isbn:9784040000008" {
			t.Fatalf("unexpected Google Books request: path=%s query=%s", r.URL.Path, r.URL.RawQuery)
		}
		if r.URL.Query().Get("key") != "test-google-books-yaml-key" {
			t.Fatalf("Google Books request should use YAML API key: %s", r.URL.RawQuery)
		}
		fmt.Fprint(w, `{
			"items": [{
				"id": "volume-1",
				"volumeInfo": {
					"title": "書籍版タイトル",
					"authors": ["著者A"],
					"publisher": "出版社",
					"publishedDate": "2026-01-01",
					"industryIdentifiers": [{"type":"ISBN_13","identifier":"9784040000008"}],
					"imageLinks": {"thumbnail":"http://books.google.test/cover.jpg"},
					"canonicalVolumeLink": "https://books.google.test/volume"
				}
			}]
		}`)
	}))
	defer googleServer.Close()
	t.Setenv("NDL_SEARCH_API_BASE_URL", ndlServer.URL)
	t.Setenv("GOOGLE_BOOKS_API_BASE_URL", googleServer.URL)
	t.Setenv("GOOGLE_BOOKS_API_KEY", "")
	t.Setenv("AI_GENERATION_SETTINGS_MASTER_PASSPHRASE", "test-passphrase")

	dataDir := newHTTPAPITestData(t)
	stateStore := store.New(dataDir)
	if err := stateStore.Initialize(); err != nil {
		t.Fatalf("initialize store: %v", err)
	}
	googleBooksAPIKey := "test-google-books-yaml-key"
	if _, err := stateStore.PutAIGenerationSettings(store.AIGenerationSettingsUpdate{
		SharedProviders: &store.AISharedProvidersInput{
			GoogleBooks: store.AIProviderCredentialInput{APIKey: &googleBooksAPIKey, APIKeySet: true},
		},
	}); err != nil {
		t.Fatalf("store Google Books API key: %v", err)
	}
	handler := newTestServerWithLibraryAndStore(dataDir, library.NewService(filepath.Join(dataDir, "novel-fetcher")), stateStore)
	novelID := library.NovelID(library.Work{ID: 1, Site: "syosetu", SiteWorkID: "n1234"})

	initial := requestJSON(t, handler, http.MethodGet, "/api/library/novels/"+novelID+"/publications", nil, http.StatusOK)
	if entries := initial["entries"].([]any); len(entries) != 0 {
		t.Fatalf("initial publications should not include placeholder entries: %+v", initial)
	}

	updated := requestJSON(t, handler, http.MethodPost, "/api/library/novels/"+novelID+"/publications/entries", map[string]any{
		"kind":   "novel",
		"mode":   "isbn",
		"isbn13": "978-4-04-000000-8",
	}, http.StatusOK)
	entry := findJSONEntryByISBN(t, updated["entries"].([]any), "9784040000008")
	if entry["status"] != "manual" || entry["title"] != "NDL書誌タイトル" || entry["imageUrl"] != "https://books.google.test/cover.jpg" {
		t.Fatalf("publication entry should be enriched: %+v", entry)
	}
	if entry["source"] != "NDLサーチ" || entry["coverSource"] != "Google Books" {
		t.Fatalf("publication entry should expose bibliography and cover sources: %+v", entry)
	}

	cover := requestJSON(t, handler, http.MethodPut, "/api/library/novels/"+novelID+"/publications/display-cover", map[string]any{
		"entryId": entry["id"].(string),
	}, http.StatusOK)
	if cover["displayCoverEntryId"] != entry["id"] {
		t.Fatalf("display cover entry should be saved: %+v", cover)
	}

	reset := requestJSON(t, handler, http.MethodPut, "/api/library/novels/"+novelID+"/publications/entries/"+entry["id"].(string), map[string]any{
		"mode": "none",
	}, http.StatusOK)
	if resetEntry := findOptionalJSONEntryByISBN(reset["entries"].([]any), "9784040000008"); resetEntry != nil {
		t.Fatalf("none override should remove extra entry: %+v", resetEntry)
	}
}

func TestPublicationRouteSkipsSavedGoogleBooksKeyWhenProviderDisabled(t *testing.T) {
	var googleServerCalled atomic.Bool
	googleServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		googleServerCalled.Store(true)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer googleServer.Close()
	t.Setenv("GOOGLE_BOOKS_API_BASE_URL", googleServer.URL)
	t.Setenv("GOOGLE_BOOKS_API_KEY", "")
	t.Setenv("PUBLICATION_PROVIDER_NDL_ENABLED", "0")
	t.Setenv("PUBLICATION_PROVIDER_GOOGLE_BOOKS_ENABLED", "1")
	t.Setenv("AI_GENERATION_SETTINGS_MASTER_PASSPHRASE", "correct-passphrase")

	dataDir := newHTTPAPITestData(t)
	stateStore := store.New(dataDir)
	if err := stateStore.Initialize(); err != nil {
		t.Fatalf("initialize store: %v", err)
	}
	googleBooksAPIKey := "encrypted-google-books-key"
	if _, err := stateStore.PutAIGenerationSettings(store.AIGenerationSettingsUpdate{
		SharedProviders: &store.AISharedProvidersInput{
			GoogleBooks: store.AIProviderCredentialInput{APIKey: &googleBooksAPIKey, APIKeySet: true},
		},
	}); err != nil {
		t.Fatalf("store Google Books API key: %v", err)
	}

	t.Setenv("AI_GENERATION_SETTINGS_MASTER_PASSPHRASE", "wrong-passphrase")
	t.Setenv("PUBLICATION_PROVIDER_GOOGLE_BOOKS_ENABLED", "0")
	handler := newTestServerWithLibraryAndStore(dataDir, library.NewService(filepath.Join(dataDir, "novel-fetcher")), stateStore)
	novelID := library.NovelID(library.Work{ID: 1, Site: "syosetu", SiteWorkID: "n1234"})

	updated := requestJSON(t, handler, http.MethodPost, "/api/library/novels/"+novelID+"/publications/entries", map[string]any{
		"kind":   "novel",
		"mode":   "isbn",
		"isbn13": "978-4-04-000000-8",
	}, http.StatusOK)
	if googleServerCalled.Load() {
		t.Fatal("Google Books server was called")
	}
	entry := findJSONEntryByISBN(t, updated["entries"].([]any), "9784040000008")
	if entry["status"] != "manual" || entry["isbn13"] != "9784040000008" {
		t.Fatalf("publication entry should be saved without Google Books lookup: %+v", entry)
	}
}

func TestPublicationRouteSkipsSavedGoogleBooksKeyWhenUpdateDoesNotLookupISBN(t *testing.T) {
	var googleServerCalls atomic.Int32
	googleServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		googleServerCalls.Add(1)
		if r.URL.Path != "/volumes" || r.URL.Query().Get("q") != "isbn:9784040000008" {
			t.Fatalf("unexpected Google Books request: path=%s query=%s", r.URL.Path, r.URL.RawQuery)
		}
		if r.URL.Query().Get("key") != "encrypted-google-books-key" {
			t.Fatalf("Google Books request should use saved API key: %s", r.URL.RawQuery)
		}
		fmt.Fprint(w, `{
			"items": [{
				"id": "volume-1",
				"volumeInfo": {
					"title": "書籍版タイトル",
					"industryIdentifiers": [{"type":"ISBN_13","identifier":"9784040000008"}],
					"imageLinks": {"thumbnail":"http://books.google.test/cover.jpg"}
				}
			}]
		}`)
	}))
	defer googleServer.Close()
	t.Setenv("GOOGLE_BOOKS_API_BASE_URL", googleServer.URL)
	t.Setenv("GOOGLE_BOOKS_API_KEY", "")
	t.Setenv("PUBLICATION_PROVIDER_NDL_ENABLED", "0")
	t.Setenv("PUBLICATION_PROVIDER_GOOGLE_BOOKS_ENABLED", "1")
	t.Setenv("AI_GENERATION_SETTINGS_MASTER_PASSPHRASE", "correct-passphrase")

	dataDir := newHTTPAPITestData(t)
	stateStore := store.New(dataDir)
	if err := stateStore.Initialize(); err != nil {
		t.Fatalf("initialize store: %v", err)
	}
	googleBooksAPIKey := "encrypted-google-books-key"
	if _, err := stateStore.PutAIGenerationSettings(store.AIGenerationSettingsUpdate{
		SharedProviders: &store.AISharedProvidersInput{
			GoogleBooks: store.AIProviderCredentialInput{APIKey: &googleBooksAPIKey, APIKeySet: true},
		},
	}); err != nil {
		t.Fatalf("store Google Books API key: %v", err)
	}
	handler := newTestServerWithLibraryAndStore(dataDir, library.NewService(filepath.Join(dataDir, "novel-fetcher")), stateStore)
	novelID := library.NovelID(library.Work{ID: 1, Site: "syosetu", SiteWorkID: "n1234"})

	created := requestJSON(t, handler, http.MethodPost, "/api/library/novels/"+novelID+"/publications/entries", map[string]any{
		"kind":   "novel",
		"mode":   "isbn",
		"isbn13": "978-4-04-000000-8",
	}, http.StatusOK)
	entry := findJSONEntryByISBN(t, created["entries"].([]any), "9784040000008")
	if googleServerCalls.Load() != 1 {
		t.Fatalf("Google Books server should be called once while creating ISBN entry: %d", googleServerCalls.Load())
	}

	t.Setenv("AI_GENERATION_SETTINGS_MASTER_PASSPHRASE", "wrong-passphrase")
	updated := requestJSON(t, handler, http.MethodPut, "/api/library/novels/"+novelID+"/publications/entries/"+entry["id"].(string), map[string]any{
		"mode": "none",
	}, http.StatusOK)
	if googleServerCalls.Load() != 1 {
		t.Fatalf("Google Books server should not be called for mode:none update: %d", googleServerCalls.Load())
	}
	if resetEntry := findOptionalJSONEntryByISBN(updated["entries"].([]any), "9784040000008"); resetEntry != nil {
		t.Fatalf("none override should remove extra entry without Google Books lookup: %+v", resetEntry)
	}
}

func TestPublicationRoutesRejectInvalidInput(t *testing.T) {
	dataDir := newHTTPAPITestData(t)
	stateStore := store.New(dataDir)
	if err := stateStore.Initialize(); err != nil {
		t.Fatalf("initialize store: %v", err)
	}
	handler := newTestServerWithLibraryAndStore(dataDir, library.NewService(filepath.Join(dataDir, "novel-fetcher")), stateStore)
	novelID := library.NovelID(library.Work{ID: 1, Site: "syosetu", SiteWorkID: "n1234"})

	requestJSON(t, handler, http.MethodPost, "/api/library/novels/"+novelID+"/publications/entries", map[string]any{
		"kind": "audio",
		"mode": "isbn",
	}, http.StatusBadRequest)
	requestJSON(t, handler, http.MethodPost, "/api/library/novels/"+novelID+"/publications/entries", map[string]any{
		"kind":   "novel",
		"mode":   "isbn",
		"isbn13": "9784040000001",
	}, http.StatusBadRequest)
	requestJSON(t, handler, http.MethodGet, "/api/library/novels/missing/publications", nil, http.StatusNotFound)
	requestJSON(t, handler, http.MethodPost, "/api/library/novels/"+novelID+"/publications", map[string]any{}, http.StatusMethodNotAllowed)
	requestJSON(t, handler, http.MethodGet, "/api/library/novels/"+novelID+"/publications/entries/novel", nil, http.StatusMethodNotAllowed)
}

func findJSONEntryByISBN(t *testing.T, entries []any, isbn13 string) map[string]any {
	t.Helper()
	if entry := findOptionalJSONEntryByISBN(entries, isbn13); entry != nil {
		return entry
	}
	t.Fatalf("entry with ISBN %s was not found: %+v", isbn13, entries)
	return nil
}

func findOptionalJSONEntryByISBN(entries []any, isbn13 string) map[string]any {
	for _, rawEntry := range entries {
		entry, ok := rawEntry.(map[string]any)
		if ok && entry["isbn13"] == isbn13 {
			return entry
		}
	}
	return nil
}
