package publications

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"narou-viewer/apps/viewer-api-go/internal/state/schemaguard"
	"narou-viewer/apps/viewer-api-go/internal/state/schemaguardtest"
)

func TestNormalizeISBN13(t *testing.T) {
	for _, value := range []string{"9784040000008", "978-4-04-000000-8"} {
		if got := NormalizeISBN13(value); got != "9784040000008" {
			t.Fatalf("NormalizeISBN13(%q) = %q", value, got)
		}
	}
	for _, value := range []string{"", "4040000000", "9784040000001", "9774040000000"} {
		if got := NormalizeISBN13(value); got != "" {
			t.Fatalf("NormalizeISBN13(%q) should reject invalid ISBN, got %q", value, got)
		}
	}
}

func TestServicePutEntryISBNUsesNDLThenGoogleBooksCover(t *testing.T) {
	ndlServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("isbn"); got != "9784040000008" {
			t.Fatalf("unexpected NDL query: %s", got)
		}
		fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>
			<rss xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:dcterms="http://purl.org/dc/terms/">
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
		if r.URL.Path != "/volumes" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("q"); got != "isbn:9784040000008" {
			t.Fatalf("unexpected query: %s", got)
		}
		fmt.Fprint(w, `{
			"items": [{
				"id": "volume-1",
				"volumeInfo": {
					"title": "書籍版タイトル",
					"subtitle": "副題",
					"authors": ["著者"],
					"publisher": "出版社",
					"publishedDate": "2026-01-01",
					"industryIdentifiers": [{"type":"ISBN_13","identifier":"9784040000008"}],
					"imageLinks": {"thumbnail":"http://books.google.test/cover.jpg"},
					"infoLink": "https://books.google.test/info",
					"canonicalVolumeLink": "https://books.google.test/canonical"
				}
			}]
		}`)
	}))
	defer googleServer.Close()

	t.Setenv("NDL_SEARCH_API_BASE_URL", ndlServer.URL)
	t.Setenv("PUBLICATION_PROVIDER_NDL_ENABLED", "1")
	t.Setenv("GOOGLE_BOOKS_API_BASE_URL", googleServer.URL)
	t.Setenv("GOOGLE_BOOKS_API_KEY", "test-google-books-key")
	t.Setenv("PUBLICATION_PROVIDER_GOOGLE_BOOKS_ENABLED", "1")

	service := NewService(filepath.Join(t.TempDir(), "state"))
	service.now = func() time.Time { return time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC) }
	result, err := service.CreateEntry(context.Background(), "novel-1", EntryInput{
		Kind:   KindNovel,
		Mode:   OverrideModeISBN,
		ISBN13: "978-4-04-000000-8",
	})
	if err != nil {
		t.Fatalf("PutEntry returned error: %v", err)
	}
	entry := mustFindEntryByISBN(t, result.Entries, "9784040000008")
	if entry.Status != EntryStatusManual || entry.ISBN13 != "9784040000008" || entry.Title != "NDL書誌タイトル" {
		t.Fatalf("entry was not enriched: %+v", entry)
	}
	if entry.Authors[0] != "NDL著者" || entry.Publisher != "NDL出版社" || entry.Published != "2025" {
		t.Fatalf("NDL bibliography should be primary: %+v", entry)
	}
	if entry.Source != "NDLサーチ" || entry.SourceURL != "https://ndlsearch.ndl.go.jp/books/R100000002-I000000000" {
		t.Fatalf("NDL source should be saved: %+v", entry)
	}
	if entry.ImageURL != "https://books.google.test/cover.jpg" || entry.CoverSourceURL != "https://books.google.test/canonical" {
		t.Fatalf("Google Books cover links should be saved: %+v", entry)
	}
}

func TestServicePutEntryDisabled(t *testing.T) {
	service := NewService(filepath.Join(t.TempDir(), "state"))
	if _, err := service.repository.PutEntry("novel-1", Entry{
		ID:        "comic-1",
		Kind:      KindComic,
		Status:    EntryStatusManual,
		Override:  OverrideModeISBN,
		ISBN13:    "9784040000008",
		UpdatedAt: "2026-06-28T12:00:00Z",
	}); err != nil {
		t.Fatalf("PutEntry setup returned error: %v", err)
	}
	result, err := service.PutEntry(context.Background(), "novel-1", "comic-1", EntryInput{Mode: OverrideModeDisabled})
	if err != nil {
		t.Fatalf("PutEntry returned error: %v", err)
	}
	entry := result.Entries[0]
	if entry.Kind != KindComic || entry.Status != EntryStatusDisabled || entry.Override != OverrideModeDisabled {
		t.Fatalf("comic entry should be disabled: %+v", entry)
	}
}

func TestServicePutEntryDisabledPreservesAndVisibleRestoresEntry(t *testing.T) {
	t.Setenv("PUBLICATION_PROVIDER_NDL_ENABLED", "0")
	t.Setenv("PUBLICATION_PROVIDER_GOOGLE_BOOKS_ENABLED", "0")
	service := NewService(filepath.Join(t.TempDir(), "state"))
	created, err := service.CreateEntry(context.Background(), "novel-1", EntryInput{
		Kind:   KindNovel,
		Mode:   OverrideModeISBN,
		ISBN13: "9784040000008",
	})
	if err != nil {
		t.Fatalf("PutEntry ISBN returned error: %v", err)
	}
	entryID := created.Entries[0].ID
	disabled, err := service.PutEntry(context.Background(), "novel-1", entryID, EntryInput{Mode: OverrideModeDisabled})
	if err != nil {
		t.Fatalf("PutEntry disabled returned error: %v", err)
	}
	if disabled.Entries[0].Status != EntryStatusDisabled || disabled.Entries[0].ISBN13 != "9784040000008" {
		t.Fatalf("disabled entry should preserve ISBN: %+v", disabled.Entries[0])
	}
	visible, err := service.PutEntry(context.Background(), "novel-1", entryID, EntryInput{Mode: OverrideModeVisible})
	if err != nil {
		t.Fatalf("PutEntry visible returned error: %v", err)
	}
	if visible.Entries[0].Status != EntryStatusManual || visible.Entries[0].Override != OverrideModeISBN || visible.Entries[0].ISBN13 != "9784040000008" {
		t.Fatalf("visible entry should restore manual ISBN: %+v", visible.Entries[0])
	}
}

func TestServiceCreateEntryUpdatesSameKindISBN(t *testing.T) {
	t.Setenv("PUBLICATION_PROVIDER_NDL_ENABLED", "0")
	t.Setenv("PUBLICATION_PROVIDER_GOOGLE_BOOKS_ENABLED", "0")
	service := NewService(filepath.Join(t.TempDir(), "state"))
	service.now = func() time.Time { return time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC) }
	first, err := service.CreateEntry(context.Background(), "novel-1", EntryInput{
		Kind:   KindNovel,
		Mode:   OverrideModeISBN,
		ISBN13: "9784040000008",
	})
	if err != nil || len(first.Entries) != 1 {
		t.Fatalf("first CreateEntry returned unexpected result: first=%+v err=%v", first, err)
	}
	firstID := first.Entries[0].ID

	service.now = func() time.Time { return time.Date(2026, 6, 28, 13, 0, 0, 0, time.UTC) }
	second, err := service.CreateEntry(context.Background(), "novel-1", EntryInput{
		Kind:   KindNovel,
		Mode:   OverrideModeISBN,
		ISBN13: "978-4-04-000000-8",
	})
	if err != nil {
		t.Fatalf("second CreateEntry returned error: %v", err)
	}
	if len(second.Entries) != 1 || second.Entries[0].ID != firstID {
		t.Fatalf("same kind and ISBN should update the existing entry: %+v", second)
	}
	if second.Entries[0].UpdatedAt != "2026-06-28T13:00:00Z" {
		t.Fatalf("same ISBN update should refresh existing entry: %+v", second.Entries[0])
	}
}

func TestServicePutEntryISBNMergesSameKindISBN(t *testing.T) {
	t.Setenv("PUBLICATION_PROVIDER_NDL_ENABLED", "0")
	t.Setenv("PUBLICATION_PROVIDER_GOOGLE_BOOKS_ENABLED", "0")
	service := NewService(filepath.Join(t.TempDir(), "state"))
	service.now = func() time.Time { return time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC) }
	first, err := service.CreateEntry(context.Background(), "novel-1", EntryInput{
		Kind:   KindNovel,
		Mode:   OverrideModeISBN,
		ISBN13: "9784040000008",
	})
	if err != nil || len(first.Entries) != 1 {
		t.Fatalf("first CreateEntry returned unexpected result: first=%+v err=%v", first, err)
	}
	firstID := first.Entries[0].ID
	service.now = func() time.Time { return time.Date(2026, 6, 28, 12, 30, 0, 0, time.UTC) }
	second, err := service.CreateEntry(context.Background(), "novel-1", EntryInput{
		Kind:   KindNovel,
		Mode:   OverrideModeISBN,
		ISBN13: "9784040000015",
	})
	if err != nil || len(second.Entries) != 2 {
		t.Fatalf("second CreateEntry returned unexpected result: second=%+v err=%v", second, err)
	}
	secondID := second.Entries[1].ID

	service.now = func() time.Time { return time.Date(2026, 6, 28, 13, 0, 0, 0, time.UTC) }
	merged, err := service.PutEntry(context.Background(), "novel-1", secondID, EntryInput{
		Mode:   OverrideModeISBN,
		ISBN13: "978-4-04-000000-8",
	})
	if err != nil {
		t.Fatalf("PutEntry duplicate ISBN returned error: %v", err)
	}
	if len(merged.Entries) != 1 || merged.Entries[0].ID != firstID || merged.Entries[0].ISBN13 != "9784040000008" {
		t.Fatalf("same kind and ISBN update should merge into the existing entry: %+v", merged)
	}
	if merged.Entries[0].UpdatedAt != "2026-06-28T13:00:00Z" {
		t.Fatalf("merged entry should be refreshed: %+v", merged.Entries[0])
	}
}

func TestServicePutEntryValidation(t *testing.T) {
	service := NewService(filepath.Join(t.TempDir(), "state"))
	if _, err := service.CreateEntry(context.Background(), "novel-1", EntryInput{Kind: Kind("audio"), Mode: OverrideModeISBN, ISBN13: "9784040000008"}); err != ErrInvalidKind {
		t.Fatalf("invalid kind should be rejected, got %v", err)
	}
	if _, err := service.CreateEntry(context.Background(), "novel-1", EntryInput{Kind: KindNovel, Mode: OverrideModeNone, ISBN13: "9784040000008"}); err != ErrInvalidOverride {
		t.Fatalf("invalid create mode should be rejected, got %v", err)
	}
	if _, err := service.PutEntry(context.Background(), "novel-1", "", EntryInput{Mode: OverrideModeNone}); err != ErrInvalidEntry {
		t.Fatalf("blank entry ID should be rejected, got %v", err)
	}
	if _, err := service.PutEntry(context.Background(), "novel-1", "missing", EntryInput{Mode: OverrideModeNone}); err != ErrInvalidEntry {
		t.Fatalf("missing entry ID should be rejected, got %v", err)
	}
	if _, err := service.repository.PutEntry("novel-1", Entry{
		ID:       "novel-1",
		Kind:     KindNovel,
		Status:   EntryStatusManual,
		Override: OverrideModeISBN,
		ISBN13:   "9784040000008",
	}); err != nil {
		t.Fatalf("PutEntry setup returned error: %v", err)
	}
	if _, err := service.PutEntry(context.Background(), "novel-1", "novel-1", EntryInput{Mode: OverrideMode("bad")}); err != ErrInvalidOverride {
		t.Fatalf("invalid override should be rejected, got %v", err)
	}
	if _, err := service.PutEntry(context.Background(), "novel-1", "novel-1", EntryInput{Kind: KindComic, Mode: OverrideModeNone}); err != ErrInvalidEntry {
		t.Fatalf("mismatched entry kind should be rejected, got %v", err)
	}
	if _, err := service.CreateEntry(context.Background(), "novel-1", EntryInput{Kind: KindNovel, Mode: OverrideModeISBN, ISBN13: "9784040000001"}); err != ErrInvalidISBN13 {
		t.Fatalf("invalid ISBN should be rejected, got %v", err)
	}
}

func TestServicePutEntryISBNFallsBackWhenProvidersFail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()
	t.Setenv("NDL_SEARCH_API_BASE_URL", server.URL)
	t.Setenv("PUBLICATION_PROVIDER_NDL_ENABLED", "1")
	t.Setenv("GOOGLE_BOOKS_API_BASE_URL", server.URL)
	t.Setenv("GOOGLE_BOOKS_API_KEY", "test-google-books-key")
	t.Setenv("PUBLICATION_PROVIDER_GOOGLE_BOOKS_ENABLED", "1")

	service := NewService(filepath.Join(t.TempDir(), "state"))
	result, err := service.CreateEntry(context.Background(), "novel-1", EntryInput{
		Kind:   KindNovel,
		Mode:   OverrideModeISBN,
		ISBN13: "9784040000008",
	})
	if err != nil {
		t.Fatalf("provider errors should not block manual ISBN save: %v", err)
	}
	entry := mustFindEntryByISBN(t, result.Entries, "9784040000008")
	if entry.Status != EntryStatusManual || entry.ISBN13 != "9784040000008" {
		t.Fatalf("ISBN entry should be saved without provider metadata: %+v", entry)
	}
	if entry.Source != "" || entry.ImageURL != "" {
		t.Fatalf("failed providers should not set source metadata: %+v", entry)
	}
	if len(entry.Warnings) != 2 {
		t.Fatalf("provider failures should be saved as warnings: %+v", entry)
	}
}

func TestServicePutEntryISBNWarnsWhenGoogleBooksHasNoCover(t *testing.T) {
	googleServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("projection") != "" {
			t.Fatalf("Google Books lookup should not use projection=lite: %s", r.URL.RawQuery)
		}
		if r.URL.Query().Get("key") != "test-google-books-key" {
			t.Fatalf("Google Books lookup should include API key")
		}
		fmt.Fprint(w, `{
			"items": [{
				"id": "volume-no-cover",
				"volumeInfo": {
					"title": "Google書誌",
					"industryIdentifiers": [{"type":"ISBN_13","identifier":"9784040000008"}],
					"canonicalVolumeLink": "https://books.google.test/volume"
				}
			}]
		}`)
	}))
	defer googleServer.Close()
	t.Setenv("PUBLICATION_PROVIDER_NDL_ENABLED", "0")
	t.Setenv("GOOGLE_BOOKS_API_BASE_URL", googleServer.URL)
	t.Setenv("GOOGLE_BOOKS_API_KEY", "test-google-books-key")
	t.Setenv("PUBLICATION_PROVIDER_GOOGLE_BOOKS_ENABLED", "1")

	service := NewService(filepath.Join(t.TempDir(), "state"))
	result, err := service.CreateEntry(context.Background(), "novel-1", EntryInput{
		Kind:   KindNovel,
		Mode:   OverrideModeISBN,
		ISBN13: "9784040000008",
	})
	if err != nil {
		t.Fatalf("PutEntry returned error: %v", err)
	}
	entry := mustFindEntryByISBN(t, result.Entries, "9784040000008")
	if entry.CoverSourceURL != "https://books.google.test/volume" {
		t.Fatalf("Google Books link should be saved even without cover: %+v", entry)
	}
	if !containsString(entry.Warnings, "google_books_cover_missing") {
		t.Fatalf("missing Google Books cover should be saved as warning: %+v", entry)
	}
}

func TestRepositoryEnsureAndRoundTrip(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "state")
	repo := NewRepository(stateDir)
	if err := repo.Ensure(); err != nil {
		t.Fatalf("Ensure returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(stateDir, FileName)); err != nil {
		t.Fatalf("publications file should exist: %v", err)
	}
	result, err := repo.PutEntry("novel-1", Entry{
		Kind:      KindNovel,
		Status:    EntryStatusManual,
		Override:  OverrideModeISBN,
		ISBN13:    "9784040000008",
		UpdatedAt: "2026-06-28T12:00:00Z",
	})
	if err != nil {
		t.Fatalf("PutEntry returned error: %v", err)
	}
	if len(result.Entries) != 1 || result.Entries[0].ISBN13 != "9784040000008" {
		t.Fatalf("entry round trip mismatch: %+v", result)
	}
	loaded, err := repo.Get("novel-1")
	if err != nil || loaded.Entries[0].ISBN13 != "9784040000008" {
		t.Fatalf("Get should load saved entry: loaded=%+v err=%v", loaded, err)
	}
	missing, err := repo.Get("missing")
	if err != nil || len(missing.Entries) != 0 {
		t.Fatalf("missing novel should return no entries: missing=%+v err=%v", missing, err)
	}
}

func TestRepositoryAndServiceListByNovelIDs(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "state")
	repo := NewRepository(stateDir)
	if _, err := repo.PutEntry("novel-1", Entry{
		ID:        "novel-1",
		Kind:      KindNovel,
		Status:    EntryStatusManual,
		Override:  OverrideModeISBN,
		ISBN13:    "9784040000008",
		UpdatedAt: "2026-06-28T12:00:00Z",
	}); err != nil {
		t.Fatalf("PutEntry novel-1 returned error: %v", err)
	}
	if _, err := repo.PutEntry("novel-2", Entry{
		ID:        "comic-1",
		Kind:      KindComic,
		Status:    EntryStatusManual,
		Override:  OverrideModeISBN,
		ISBN13:    "9784040000015",
		UpdatedAt: "2026-06-28T12:00:00Z",
	}); err != nil {
		t.Fatalf("PutEntry novel-2 returned error: %v", err)
	}
	result, err := repo.ListByNovelIDs([]string{" novel-1 ", "", "missing"})
	if err != nil {
		t.Fatalf("ListByNovelIDs returned error: %v", err)
	}
	if result["novel-1"].Entries[0].ISBN13 != "9784040000008" {
		t.Fatalf("ListByNovelIDs should load saved entry: %+v", result)
	}
	if missing := result["missing"]; missing.NovelID != "missing" || len(missing.Entries) != 0 {
		t.Fatalf("ListByNovelIDs should include normalized missing novels: %+v", missing)
	}
	if _, ok := result[""]; ok {
		t.Fatalf("ListByNovelIDs should ignore blank novel IDs: %+v", result)
	}

	service := &Service{repository: repo}
	serviceResult, err := service.ListByNovelIDs([]string{"novel-2"})
	if err != nil {
		t.Fatalf("service ListByNovelIDs returned error: %v", err)
	}
	if serviceResult["novel-2"].Entries[0].ISBN13 != "9784040000015" {
		t.Fatalf("service ListByNovelIDs should delegate to repository: %+v", serviceResult)
	}
	nilServiceResult, err := (*Service)(nil).ListByNovelIDs([]string{" novel-3 ", ""})
	if err != nil {
		t.Fatalf("nil service ListByNovelIDs returned error: %v", err)
	}
	if nilServiceResult["novel-3"].NovelID != "novel-3" || len(nilServiceResult["novel-3"].Entries) != 0 {
		t.Fatalf("nil service ListByNovelIDs should normalize requested IDs: %+v", nilServiceResult)
	}
}

func TestRepositoryDeleteEntryClearsDisplayCover(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "state")
	repo := NewRepository(stateDir)
	if _, err := repo.PutEntry("novel-1", Entry{
		ID:        "comic-1",
		Kind:      KindComic,
		Status:    EntryStatusManual,
		Override:  OverrideModeISBN,
		ISBN13:    "9784040000008",
		ImageURL:  "https://example.test/comic.jpg",
		UpdatedAt: "2026-06-28T12:00:00Z",
	}); err != nil {
		t.Fatalf("PutEntry returned error: %v", err)
	}
	if result, err := repo.PutDisplayCoverEntryID("novel-1", "comic-1"); err != nil || result.DisplayCoverEntryID != "comic-1" {
		t.Fatalf("PutDisplayCoverEntryID should save selected cover: result=%+v err=%v", result, err)
	}
	result, err := repo.DeleteEntry("novel-1", "comic-1")
	if err != nil {
		t.Fatalf("DeleteEntry returned error: %v", err)
	}
	if result.DisplayCoverEntryID != "" {
		t.Fatalf("DeleteEntry should clear removed display cover: %+v", result)
	}
	if len(result.Entries) != 0 {
		t.Fatalf("DeleteEntry should remove the entry without restoring placeholders: %+v", result)
	}
	if missing, err := repo.DeleteEntry("missing", "comic-1"); err != nil || len(missing.Entries) != 0 {
		t.Fatalf("DeleteEntry for missing novel should return no entries: missing=%+v err=%v", missing, err)
	}
}

func TestRepositoryPutEntryDeletingTransfersDisplayCover(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "state")
	repo := NewRepository(stateDir)
	if _, err := repo.PutEntry("novel-1", Entry{
		ID:        "novel-1",
		Kind:      KindNovel,
		Status:    EntryStatusManual,
		Override:  OverrideModeISBN,
		ISBN13:    "9784040000008",
		ImageURL:  "https://example.test/novel-1.jpg",
		UpdatedAt: "2026-06-28T12:00:00Z",
	}); err != nil {
		t.Fatalf("PutEntry target returned error: %v", err)
	}
	if _, err := repo.PutEntry("novel-1", Entry{
		ID:        "novel-2",
		Kind:      KindNovel,
		Status:    EntryStatusManual,
		Override:  OverrideModeISBN,
		ISBN13:    "9784040000015",
		ImageURL:  "https://example.test/novel-2.jpg",
		UpdatedAt: "2026-06-28T12:30:00Z",
	}); err != nil {
		t.Fatalf("PutEntry duplicate returned error: %v", err)
	}
	if _, err := repo.PutDisplayCoverEntryID("novel-1", "novel-2"); err != nil {
		t.Fatalf("PutDisplayCoverEntryID returned error: %v", err)
	}
	result, err := repo.PutEntryDeleting("novel-1", Entry{
		ID:        "novel-1",
		Kind:      KindNovel,
		Status:    EntryStatusManual,
		Override:  OverrideModeISBN,
		ISBN13:    "9784040000008",
		ImageURL:  "https://example.test/novel-1-updated.jpg",
		UpdatedAt: "2026-06-28T13:00:00Z",
	}, "novel-2")
	if err != nil {
		t.Fatalf("PutEntryDeleting returned error: %v", err)
	}
	if len(result.Entries) != 1 || result.Entries[0].ID != "novel-1" {
		t.Fatalf("PutEntryDeleting should remove the merged entry: %+v", result)
	}
	if result.DisplayCoverEntryID != "novel-1" {
		t.Fatalf("PutEntryDeleting should transfer display cover to the merged target: %+v", result)
	}
}

func TestRepositoryPutEntryClearsInvalidDisplayCover(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "state")
	repo := NewRepository(stateDir)
	if _, err := repo.PutEntry("novel-1", Entry{
		ID:        "novel-1",
		Kind:      KindNovel,
		Status:    EntryStatusManual,
		Override:  OverrideModeISBN,
		ISBN13:    "9784040000008",
		ImageURL:  "https://example.test/novel.jpg",
		UpdatedAt: "2026-06-28T12:00:00Z",
	}); err != nil {
		t.Fatalf("PutEntry setup returned error: %v", err)
	}
	if _, err := repo.PutDisplayCoverEntryID("novel-1", "novel-1"); err != nil {
		t.Fatalf("PutDisplayCoverEntryID returned error: %v", err)
	}
	result, err := repo.PutEntry("novel-1", Entry{
		ID:        "novel-1",
		Kind:      KindNovel,
		Status:    EntryStatusUnknown,
		Override:  OverrideModeNone,
		UpdatedAt: "2026-06-28T12:00:00Z",
	})
	if err != nil {
		t.Fatalf("PutEntry unknown returned error: %v", err)
	}
	if result.DisplayCoverEntryID != "" {
		t.Fatalf("PutEntry should clear display cover when selected entry loses its image: %+v", result)
	}
}

func TestServiceSetDisplayCover(t *testing.T) {
	service := NewService(filepath.Join(t.TempDir(), "state"))
	if _, err := service.repository.PutEntry("novel-1", Entry{
		ID:        "novel-1",
		Kind:      KindNovel,
		Status:    EntryStatusManual,
		Override:  OverrideModeISBN,
		ISBN13:    "9784040000008",
		ImageURL:  "https://example.test/novel.jpg",
		UpdatedAt: "2026-06-28T12:00:00Z",
	}); err != nil {
		t.Fatalf("PutEntry novel returned error: %v", err)
	}
	if _, err := service.repository.PutEntry("novel-1", Entry{
		ID:        "comic-1",
		Kind:      KindComic,
		Status:    EntryStatusManual,
		Override:  OverrideModeISBN,
		ISBN13:    "9784040000015",
		ImageURL:  "https://example.test/comic.jpg",
		UpdatedAt: "2026-06-28T12:00:00Z",
	}); err != nil {
		t.Fatalf("PutEntry comic returned error: %v", err)
	}

	result, err := service.SetDisplayCover("novel-1", DisplayCoverInput{EntryID: "comic-1"})
	if err != nil || result.DisplayCoverEntryID != "comic-1" {
		t.Fatalf("SetDisplayCover should save visible entry with image: result=%+v err=%v", result, err)
	}
	if _, err := service.SetDisplayCover("novel-1", DisplayCoverInput{EntryID: "missing"}); err != ErrInvalidEntry {
		t.Fatalf("SetDisplayCover should reject missing entry, got %v", err)
	}
	if _, err := service.SetDisplayCover("novel-1", DisplayCoverInput{EntryID: "comic"}); err != ErrInvalidEntry {
		t.Fatalf("SetDisplayCover should reject entries without image, got %v", err)
	}
	cleared, err := service.SetDisplayCover("novel-1", DisplayCoverInput{EntryID: ""})
	if err != nil || cleared.DisplayCoverEntryID != "" {
		t.Fatalf("SetDisplayCover should clear selection: cleared=%+v err=%v", cleared, err)
	}
}

func TestNormalizeEntriesDeduplicatesIDs(t *testing.T) {
	entries := normalizeEntries([]Entry{
		{ID: "dup", Kind: KindNovel, Status: EntryStatusManual, Override: OverrideModeISBN, ISBN13: "9784040000008"},
		{ID: "dup", Kind: KindNovel, Status: EntryStatusManual, Override: OverrideModeISBN, ISBN13: "9784040000015"},
		{ID: "ignored", Kind: Kind("audio")},
	})
	if len(entries) != 2 {
		t.Fatalf("normalizeEntries should keep real entries without placeholders: %+v", entries)
	}
	if entries[0].ID != "dup" || entries[1].ID != "dup-2" {
		t.Fatalf("normalizeEntries should deduplicate IDs: %+v", entries)
	}
}

func TestRepositoryPruneNovel(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "state")
	repo := NewRepository(stateDir)
	if _, err := repo.PutEntry("novel-1", Entry{
		Kind:      KindNovel,
		Status:    EntryStatusManual,
		Override:  OverrideModeISBN,
		ISBN13:    "9784040000008",
		UpdatedAt: "2026-06-28T12:00:00Z",
	}); err != nil {
		t.Fatalf("PutEntry novel-1 returned error: %v", err)
	}
	if _, err := repo.PutEntry("novel-2", Entry{
		Kind:      KindComic,
		Status:    EntryStatusManual,
		Override:  OverrideModeISBN,
		ISBN13:    "9784040000015",
		UpdatedAt: "2026-06-28T12:00:00Z",
	}); err != nil {
		t.Fatalf("PutEntry novel-2 returned error: %v", err)
	}

	deleted, err := repo.PruneNovel("novel-1")
	if err != nil {
		t.Fatalf("PruneNovel returned error: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("PruneNovel should count normalized entries, got %d", deleted)
	}
	pruned, err := repo.Get("novel-1")
	if err != nil || len(pruned.Entries) != 0 {
		t.Fatalf("pruned novel should load no entries: pruned=%+v err=%v", pruned, err)
	}
	kept, err := repo.Get("novel-2")
	if err != nil || len(kept.Entries) != 1 || kept.Entries[0].ISBN13 != "9784040000015" {
		t.Fatalf("other novel should be kept: kept=%+v err=%v", kept, err)
	}
	if deleted, err := repo.PruneNovel("missing"); err != nil || deleted != 0 {
		t.Fatalf("missing prune should be a no-op: deleted=%d err=%v", deleted, err)
	}
	if deleted, err := repo.PruneNovel(" "); err != nil || deleted != 0 {
		t.Fatalf("blank prune should be a no-op: deleted=%d err=%v", deleted, err)
	}
}

func TestGoogleBooksLookupHandlesNoMatchAndHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("q") {
		case "isbn:9784040000008":
			fmt.Fprint(w, `{"items":[{"id":"other","volumeInfo":{"industryIdentifiers":[{"type":"ISBN_13","identifier":"9784040000015"}]}}]}`)
		default:
			w.WriteHeader(http.StatusBadGateway)
		}
	}))
	defer server.Close()

	client := &GoogleBooksClient{
		baseURL: server.URL,
		apiKey:  "test-google-books-key",
		client:  server.Client(),
	}
	volume, err := client.LookupISBN(context.Background(), "9784040000008")
	if err != nil || volume != nil {
		t.Fatalf("no matching ISBN should return nil volume without error: volume=%+v err=%v", volume, err)
	}
	if _, err := client.LookupISBN(context.Background(), "9784040000022"); err == nil {
		t.Fatal("HTTP errors should be returned")
	}
}

func TestGoogleBooksEnabled(t *testing.T) {
	t.Setenv("PUBLICATION_PROVIDER_GOOGLE_BOOKS_ENABLED", "")
	if !GoogleBooksEnabled() {
		t.Fatal("Google Books should be enabled by default")
	}
	t.Setenv("PUBLICATION_PROVIDER_GOOGLE_BOOKS_ENABLED", "false")
	if GoogleBooksEnabled() {
		t.Fatal("Google Books should honor false env")
	}
}

func TestGoogleBooksAPIKeyRequired(t *testing.T) {
	t.Setenv("GOOGLE_BOOKS_API_KEY", "")
	if GoogleBooksAPIKeyConfigured() {
		t.Fatal("empty Google Books API key should be treated as missing")
	}
	t.Setenv("GOOGLE_BOOKS_API_KEY", "test-google-books-key")
	if !GoogleBooksAPIKeyConfigured() {
		t.Fatal("Google Books API key should be detected")
	}

	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	client := &GoogleBooksClient{baseURL: server.URL, client: server.Client()}
	volume, err := client.LookupISBN(context.Background(), "9784040000008")
	if err != nil || volume != nil || called {
		t.Fatalf("missing API key should skip lookup: volume=%+v err=%v called=%v", volume, err, called)
	}
	if volume, err := client.LookupISBNWithAPIKey(context.Background(), "9784040000008", "test-google-books-key"); err == nil || volume != nil || !called {
		t.Fatalf("request API key should be used for lookup: volume=%+v err=%v called=%v", volume, err, called)
	}
	if got := (*GoogleBooksClient)(nil).APIKey(); got != "" {
		t.Fatalf("nil client APIKey = %q", got)
	}
}

func TestNDLLookupAndParser(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("isbn"); got != "9784040000008" {
			t.Fatalf("unexpected ISBN query: %s", got)
		}
		fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>
			<rss xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:dcterms="http://purl.org/dc/terms/">
				<channel>
					<item>
						<title>不一致</title>
						<dc:identifier>9784040000015</dc:identifier>
					</item>
					<item>
						<title>NDLタイトル</title>
						<link>https://ndl.example.test/detail</link>
						<dc:creator>著者A</dc:creator>
						<dc:creator>著者A</dc:creator>
						<dc:publisher>出版社A</dc:publisher>
						<dcterms:issued>2024</dcterms:issued>
						<dc:identifier>978-4-04-000000-8</dc:identifier>
					</item>
				</channel>
			</rss>`)
	}))
	defer server.Close()

	client := &NDLClient{baseURL: server.URL, client: server.Client()}
	bibliography, err := client.LookupISBN(context.Background(), "9784040000008")
	if err != nil {
		t.Fatalf("LookupISBN returned error: %v", err)
	}
	if bibliography == nil || bibliography.Title != "NDLタイトル" || len(bibliography.Authors) != 1 || bibliography.PublishedDate != "2024" {
		t.Fatalf("unexpected NDL bibliography: %+v", bibliography)
	}
}

func TestNDLEnabledAndErrors(t *testing.T) {
	t.Setenv("PUBLICATION_PROVIDER_NDL_ENABLED", "")
	if !NDLSearchEnabled() {
		t.Fatal("NDL should be enabled by default")
	}
	t.Setenv("PUBLICATION_PROVIDER_NDL_ENABLED", "0")
	if NDLSearchEnabled() {
		t.Fatal("NDL should honor disabled env")
	}
	if bibliography, err := (*NDLClient)(nil).LookupISBN(context.Background(), "9784040000008"); err != nil || bibliography != nil {
		t.Fatalf("nil NDL client should return nil without error: bibliography=%+v err=%v", bibliography, err)
	}
	if _, err := (&NDLClient{baseURL: "://bad", client: http.DefaultClient}).LookupISBN(context.Background(), "9784040000008"); err == nil {
		t.Fatal("bad NDL base URL should return an error")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()
	if _, err := (&NDLClient{baseURL: server.URL, client: server.Client()}).LookupISBN(context.Background(), "9784040000008"); err == nil {
		t.Fatal("NDL HTTP errors should be returned")
	}
}

func TestNDLParserRequiresISBNIdentifier(t *testing.T) {
	bibliography, err := parseNDLOpenSearch(strings.NewReader(`<?xml version="1.0" encoding="UTF-8"?>
		<rss xmlns:dc="http://purl.org/dc/elements/1.1/">
			<channel>
				<item>
					<title>ISBNなしタイトル</title>
				</item>
			</channel>
		</rss>`), "9784040000008")
	if err != nil {
		t.Fatalf("parseNDLOpenSearch returned error: %v", err)
	}
	if bibliography != nil {
		t.Fatalf("item without ISBN identifier should not be adopted: %+v", bibliography)
	}
}

func TestServiceEnsureGetAndNoneOverride(t *testing.T) {
	service := NewService(filepath.Join(t.TempDir(), "state"))
	if err := service.Ensure(); err != nil {
		t.Fatalf("Ensure returned error: %v", err)
	}
	initial, err := service.Get("novel-1")
	if err != nil || len(initial.Entries) != 0 {
		t.Fatalf("Get should return no entries before registration: initial=%+v err=%v", initial, err)
	}
	created, err := service.CreateEntry(context.Background(), "novel-1", EntryInput{
		Kind:   KindNovel,
		Mode:   OverrideModeISBN,
		ISBN13: "9784040000008",
	})
	if err != nil || len(created.Entries) != 1 {
		t.Fatalf("CreateEntry returned unexpected result: created=%+v err=%v", created, err)
	}
	result, err := service.PutEntry(context.Background(), "novel-1", created.Entries[0].ID, EntryInput{Mode: OverrideModeNone})
	if err != nil {
		t.Fatalf("none override returned error: %v", err)
	}
	if len(result.Entries) != 0 {
		t.Fatalf("none override should delete the entry: %+v", result)
	}
	if err := (*Service)(nil).Ensure(); err != nil {
		t.Fatalf("nil service Ensure should be a no-op: %v", err)
	}
	if got, err := (*Service)(nil).Get("novel-1"); err != nil || len(got.Entries) != 0 {
		t.Fatalf("nil service Get should return no entries: got=%+v err=%v", got, err)
	}
}

func TestGoogleBooksLookupNilAndBadBaseURL(t *testing.T) {
	if volume, err := (*GoogleBooksClient)(nil).LookupISBN(context.Background(), "9784040000008"); err != nil || volume != nil {
		t.Fatalf("nil client should return nil without error: volume=%+v err=%v", volume, err)
	}
	client := &GoogleBooksClient{baseURL: "://bad", client: http.DefaultClient}
	client.apiKey = "test-google-books-key"
	if _, err := client.LookupISBN(context.Background(), "9784040000008"); err == nil {
		t.Fatal("bad base URL should return an error")
	}
}

func TestGoogleBooksImageEntryFallbacks(t *testing.T) {
	if got := selectGoogleBooksImageURL(map[string]string{"large": "http://example.test/large.jpg"}); got != "https://example.test/large.jpg" {
		t.Fatalf("large image should be preferred and normalized: %q", got)
	}
	if got := selectGoogleBooksImageURL(map[string]string{"z": "https://example.test/z.jpg", "a": "https://example.test/a.jpg"}); got != "https://example.test/a.jpg" {
		t.Fatalf("unknown image keys should fall back deterministically: %q", got)
	}
	if got := selectGoogleBooksImageURL(nil); got != "" {
		t.Fatalf("nil image links should return empty string: %q", got)
	}
	if got := firstNonEmpty(" ", "value"); got != "value" {
		t.Fatalf("firstNonEmpty should skip blanks: %q", got)
	}
	if got := firstNonEmpty(" ", ""); got != "" {
		t.Fatalf("firstNonEmpty should return empty when no values match: %q", got)
	}
}

func TestMergeGoogleBooksVolumeFillsMissingBibliography(t *testing.T) {
	entry := mergeGoogleBooksVolume(Entry{ProviderID: googleBooksProviderID}, &GoogleBooksVolume{
		Title:               "Googleタイトル",
		Subtitle:            "副題",
		Authors:             []string{"Google著者"},
		Publisher:           "Google出版社",
		PublishedDate:       "2024-01-01",
		ImageURL:            "https://books.google.test/cover.jpg",
		InfoLink:            "https://books.google.test/info",
		CanonicalVolumeLink: "https://books.google.test/canonical",
	})
	if entry.Title != "Googleタイトル" || entry.Subtitle != "副題" || entry.Authors[0] != "Google著者" {
		t.Fatalf("Google Books should fill missing bibliography: %+v", entry)
	}
	if entry.DetailURL != "https://books.google.test/canonical" || entry.Source != "Google Books" {
		t.Fatalf("Google Books should become source when no primary source exists: %+v", entry)
	}
	if entry.ProviderID != googleBooksProviderID {
		t.Fatalf("provider ID should not be duplicated: %q", entry.ProviderID)
	}
	if got := appendProviderID("ndl_search+google_books", googleBooksProviderID); got != "ndl_search+google_books" {
		t.Fatalf("appendProviderID should keep existing provider: %q", got)
	}
}

func TestParseNDLOpenSearchNoMatchAndInvalidXML(t *testing.T) {
	if bibliography, err := parseNDLOpenSearch(strings.NewReader(`<rss><channel><item><title>no isbn</title><identifier>9784040000015</identifier></item></channel></rss>`), "9784040000008"); err != nil || bibliography != nil {
		t.Fatalf("no matching NDL item should return nil without error: bibliography=%+v err=%v", bibliography, err)
	}
	if _, err := parseNDLOpenSearch(strings.NewReader(`<rss><channel><item>`), "9784040000008"); err == nil {
		t.Fatal("invalid NDL XML should return an error")
	}
	item := ndlItem{Title: "identifierなし"}
	if bibliography := item.toBibliography("9784040000008"); bibliography != nil {
		t.Fatalf("items without identifiers should not be accepted for ISBN lookup: %+v", bibliography)
	}
}

func TestRepositoryReadEmptyAndInvalidYAML(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("mkdir state: %v", err)
	}
	path := filepath.Join(stateDir, FileName)
	if err := os.WriteFile(path, []byte("  \n"), 0o644); err != nil {
		t.Fatalf("write empty yaml: %v", err)
	}
	repo := NewRepository(stateDir)
	if got, err := repo.Get("novel-1"); err != nil || len(got.Entries) != 0 {
		t.Fatalf("empty yaml should load no entries: got=%+v err=%v", got, err)
	}
	if err := os.WriteFile(path, []byte("novels: ["), 0o644); err != nil {
		t.Fatalf("write invalid yaml: %v", err)
	}
	if _, err := repo.Get("novel-1"); err == nil {
		t.Fatal("invalid yaml should return an error")
	}
}

func TestRepositorySchemaGuardRejectsMutationAndPruneWithoutTouchingFile(t *testing.T) {
	tests := []struct {
		name       string
		document   string
		wantStatus schemaguard.Status
	}{
		{name: "future", document: "schema_version: 99\nnovels: []\n", wantStatus: schemaguard.StatusFutureUnknown},
		{name: "malformed", document: "schema_version: [\n", wantStatus: schemaguard.StatusMalformed},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stateDir := t.TempDir()
			path := filepath.Join(stateDir, FileName)
			if err := os.WriteFile(path, []byte(test.document), 0o644); err != nil {
				t.Fatalf("write fixture: %v", err)
			}
			repository := NewRepository(stateDir)
			for _, action := range []struct {
				name string
				run  func() error
			}{
				{name: "put", run: func() error {
					_, err := repository.PutEntry("novel-1", Entry{ID: "novel", Kind: KindNovel})
					return err
				}},
				{name: "prune", run: func() error {
					_, err := repository.PruneNovel("novel-1")
					return err
				}},
			} {
				t.Run(action.name, func(t *testing.T) {
					err := schemaguardtest.AssertFileUntouched(t, path, action.run)
					var guardError *schemaguard.GuardError
					if !errors.As(err, &guardError) || guardError.Result.Status != test.wantStatus {
						t.Fatalf("error = %#v, want GuardError status %s", err, test.wantStatus)
					}
				})
			}
		})
	}
}

func TestRepositoryMigratesLegacyVersionZeroOnWrite(t *testing.T) {
	for _, document := range []string{
		"novels: []\n",
		"schema_version: 0\nnovels: []\n",
	} {
		stateDir := t.TempDir()
		path := filepath.Join(stateDir, FileName)
		if err := os.WriteFile(path, []byte(document), 0o644); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
		if _, err := NewRepository(stateDir).PutEntry("novel-1", Entry{ID: "novel", Kind: KindNovel}); err != nil {
			t.Fatalf("PutEntry legacy v0: %v", err)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read migrated fixture: %v", err)
		}
		if !strings.Contains(string(raw), "schema_version: 1") {
			t.Fatalf("legacy document was not migrated to v1: %s", raw)
		}
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func mustFindEntryByISBN(t *testing.T, entries []Entry, isbn13 string) Entry {
	t.Helper()
	for _, entry := range entries {
		if entry.ISBN13 == isbn13 {
			return entry
		}
	}
	t.Fatalf("entry with ISBN %s was not found: %+v", isbn13, entries)
	return Entry{}
}
