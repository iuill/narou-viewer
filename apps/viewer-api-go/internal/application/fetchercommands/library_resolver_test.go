package fetchercommands

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"narou-viewer/apps/viewer-api-go/internal/library"

	_ "modernc.org/sqlite"
)

func TestLibraryWorkIDResolverBranches(t *testing.T) {
	if workID, ok, err := (LibraryWorkIDResolver{}).FetcherWorkID("novel"); !errors.Is(err, ErrWorkIDResolverUnavailable) || ok || workID != "" {
		t.Fatalf("nil resolver should report unavailable: workID=%q ok=%v err=%v", workID, ok, err)
	}
	emptyLibrary := library.NewService(filepath.Join(t.TempDir(), "missing-library"))
	if workID, ok, err := NewLibraryWorkIDResolver(emptyLibrary).FetcherWorkID("missing"); err != nil || ok || workID != "" {
		t.Fatalf("empty library should report missing work: workID=%q ok=%v err=%v", workID, ok, err)
	}

	libraryRoot := newResolverLibraryFixture(t)
	fixtureLibrary := library.NewService(libraryRoot)
	novelID := library.NovelID(library.Work{ID: 7, Site: "syosetu", SiteWorkID: "n1234"})
	if workID, ok, err := NewLibraryWorkIDResolver(fixtureLibrary).FetcherWorkID(novelID); err != nil || !ok || workID != "7" {
		t.Fatalf("fixture library resolver should return fetcher work ID: workID=%q ok=%v err=%v", workID, ok, err)
	}
}

func newResolverLibraryFixture(t *testing.T) string {
	t.Helper()
	libraryRoot := filepath.Join(t.TempDir(), "novel-fetcher")
	if err := os.MkdirAll(libraryRoot, 0o755); err != nil {
		t.Fatalf("mkdir library root: %v", err)
	}
	db, err := sql.Open("sqlite", filepath.Join(libraryRoot, "library.sqlite"))
	if err != nil {
		t.Fatalf("open library sqlite: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`
		CREATE TABLE works (
			id INTEGER PRIMARY KEY,
			site TEXT NOT NULL,
			site_name TEXT NOT NULL,
			site_work_id TEXT NOT NULL,
			source_url TEXT NOT NULL,
			title TEXT NOT NULL,
			author TEXT NOT NULL,
			story TEXT NOT NULL,
			directory TEXT NOT NULL,
			fetched_at TEXT NOT NULL,
			fetch_status TEXT NOT NULL,
			last_fetch_error TEXT NOT NULL,
			last_failed_episode_id TEXT NOT NULL,
			resume_episode_id TEXT NOT NULL,
			expected_episode_count INTEGER NOT NULL
		);
		CREATE TABLE episodes (
			work_id INTEGER NOT NULL,
			episode_id TEXT NOT NULL,
			body_status TEXT NOT NULL
		);
		INSERT INTO works VALUES (7, 'syosetu', '小説家になろう', 'n1234', 'https://ncode.syosetu.com/n1234/', 'Fixture Novel', 'Author', 'Story', 'works/syosetu/n1234', '2026-01-01T00:00:00Z', 'complete', '', '', '', 1);
		INSERT INTO episodes VALUES (7, '1', 'complete');
	`); err != nil {
		t.Fatalf("seed library sqlite: %v", err)
	}
	return libraryRoot
}
