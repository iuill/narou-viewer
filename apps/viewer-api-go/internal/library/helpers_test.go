package library

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServiceHandlesMissingLibraryAndNilClose(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	if _, err := Open(missing); err == nil {
		t.Fatal("Open should fail when library.sqlite is missing")
	}
	service := NewService(missing)
	if works, err := service.ListWorks(); err != nil || len(works) != 0 {
		t.Fatalf("missing service should return empty works without error: works=%+v err=%v", works, err)
	}
	status := service.RuntimeStatus(context.Background())
	if status.Status != RuntimeStatusWarn {
		t.Fatalf("missing service should report warn, got %+v", status)
	}
	if err := os.MkdirAll(missing, 0o755); err != nil {
		t.Fatalf("mkdir missing root: %v", err)
	}
	fixtureRoot := setupLibraryFixture(t)
	rawDB, err := os.ReadFile(filepath.Join(fixtureRoot, "library.sqlite"))
	if err != nil {
		t.Fatalf("read fixture db: %v", err)
	}
	if err := os.WriteFile(filepath.Join(missing, "library.sqlite"), rawDB, 0o644); err != nil {
		t.Fatalf("write late db: %v", err)
	}
	if works, err := service.ListWorks(); err != nil || len(works) != 1 {
		t.Fatalf("service should reopen a late-created library db: works=%+v err=%v", works, err)
	}
	if err := (*Service)(nil).Close(); err != nil {
		t.Fatalf("nil Close returned error: %v", err)
	}
}

func TestLibraryHelpersNormalizeAndRender(t *testing.T) {
	if id := NovelID(Work{ID: 42}); id == "" {
		t.Fatal("NovelID without source URL should use fallback id")
	}
	if got := normalizeTocURL("https://ncode.syosetu.com/n1234"); got != "https://ncode.syosetu.com/n1234/" {
		t.Fatalf("unexpected syosetu toc URL: %q", got)
	}
	if got := normalizeTocURL("https://kakuyomu.jp/works/123/"); got != "https://kakuyomu.jp/works/123/" {
		t.Fatalf("unexpected kakuyomu toc URL: %q", got)
	}
	if got := NovelID(Work{ID: 1, Site: "KAKUYOMU", SiteWorkID: "123"}); got != NovelID(Work{ID: 2, Site: "kakuyomu", SiteWorkID: "123"}) {
		t.Fatalf("novel ID should be stable across fetcher work IDs and site case: %q", got)
	}
	if got := normalizeExternalURL("://bad"); got != "://bad" {
		t.Fatalf("parse-error URL should remain unchanged, got %q", got)
	}
	if got := normalizeAssetPath("/assets/episodes/1/pic.png"); got != "assets/episodes/1/pic.png" {
		t.Fatalf("unexpected asset path: %q", got)
	}
	if got := normalizeAssetPath("../secret"); got != "" {
		t.Fatalf("unsafe asset path should be blank, got %q", got)
	}
	if got := derefString(nil); got != "" {
		t.Fatalf("nil string pointer should deref to blank, got %q", got)
	}
	value := "x"
	if got := derefString(&value); got != "x" {
		t.Fatalf("unexpected deref value: %q", got)
	}
	if got := firstNonEmpty("", " ", "value"); got != "value" {
		t.Fatalf("unexpected firstNonEmpty: %q", got)
	}
	if got := firstNonEmpty("", " "); got != "" {
		t.Fatalf("all blank firstNonEmpty should be blank, got %q", got)
	}

	image := renderImage(BodyBlock{Src: "pic.png", Alt: "alt", Width: 10, Height: 20})
	if !strings.Contains(image, `src="pic.png"`) || !strings.Contains(image, `width="10"`) {
		t.Fatalf("unexpected rendered image: %s", image)
	}
	if got := renderImage(BodyBlock{}); got != "" {
		t.Fatalf("image without src should be blank, got %q", got)
	}
	rewritten := rewriteAssetRefs(`<img src="assets/episodes/1/pic.png">`, "novel")
	if !strings.Contains(rewritten, "/api/library/novels/novel/assets/assets/episodes/1/pic.png") {
		t.Fatalf("asset refs were not rewritten: %s", rewritten)
	}
	if got := rewriteAssetRefs(`<p>no image</p>`, "novel"); got != `<p>no image</p>` {
		t.Fatalf("non-image HTML should remain unchanged, got %q", got)
	}
	if got := rewriteAssetURL("https://example.test/pic.png", "novel"); got != "https://example.test/pic.png" {
		t.Fatalf("external asset URL should remain unchanged, got %q", got)
	}
	if got := rewriteAssetURL("../secret", "novel"); got != "../secret" {
		t.Fatalf("unsafe relative asset URL should remain unchanged, got %q", got)
	}
}

func TestServiceReturnsNilForMissingAndInvalidEpisodeDocuments(t *testing.T) {
	rootDir := setupLibraryFixture(t)
	service, err := Open(rootDir)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer service.Close()

	novelID := NovelID(Work{ID: 1, Site: "syosetu", SiteWorkID: "n1234ab"})
	if toc, err := service.GetToc(context.Background(), "missing"); err != nil || toc != nil {
		t.Fatalf("missing toc should be nil without error, toc=%+v err=%v", toc, err)
	}
	if episode, err := service.GetEpisode(context.Background(), "missing", "1"); err != nil || episode != nil {
		t.Fatalf("missing novel episode should be nil without error, episode=%+v err=%v", episode, err)
	}
	if episode, err := service.GetEpisode(context.Background(), novelID, "missing"); err != nil || episode != nil {
		t.Fatalf("missing episode should be nil without error, episode=%+v err=%v", episode, err)
	}

	badPath := filepath.Join(rootDir, "works/syosetu/n1234ab/episodes/1.json")
	if err := os.WriteFile(badPath, []byte("{"), 0o644); err != nil {
		t.Fatalf("write bad episode json: %v", err)
	}
	if _, err := service.GetEpisode(context.Background(), novelID, "1"); err == nil {
		t.Fatal("bad episode json should return error")
	}
	if _, err := service.ReadEpisodeDocument(Episode{BodyPath: "../library.sqlite"}); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("escaped episode body path should be treated as missing, got %v", err)
	}
	outsideEpisode := filepath.Join(t.TempDir(), "outside.json")
	if err := os.WriteFile(outsideEpisode, []byte(`{}`), 0o644); err != nil {
		t.Fatalf("write outside episode: %v", err)
	}
	if _, err := service.ReadEpisodeDocument(Episode{BodyPath: outsideEpisode}); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("absolute episode body path should be treated as missing, got %v", err)
	}
	outsideDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(outsideDir, "secret.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatalf("write outside symlink target: %v", err)
	}
	escapeDir := filepath.Join(rootDir, "escape")
	if err := os.Symlink(outsideDir, escapeDir); err != nil {
		t.Skipf("symlink is not available: %v", err)
	}
	if _, err := service.ReadEpisodeDocument(Episode{BodyPath: "escape/secret.json"}); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("symlink escaped episode body path should be treated as missing, got %v", err)
	}
}

func TestLibraryServiceWithMissingDatabaseReturnsEmptyResults(t *testing.T) {
	service := &Service{rootDir: t.TempDir()}
	if list, err := service.ListNovels(context.Background()); err != nil || len(list.Novels) != 0 {
		t.Fatalf("nil db ListNovels should be empty, list=%+v err=%v", list, err)
	}
	if toc, err := service.GetToc(context.Background(), "missing"); err != nil || toc != nil {
		t.Fatalf("nil db GetToc should return nil, toc=%+v err=%v", toc, err)
	}
	if episode, err := service.GetEpisode(context.Background(), "missing", "1"); err != nil || episode != nil {
		t.Fatalf("nil db GetEpisode should return nil, episode=%+v err=%v", episode, err)
	}
	if asset, err := service.GetAsset(context.Background(), "missing", "assets/pic.png"); err != nil || asset != nil {
		t.Fatalf("nil db GetAsset should return nil, asset=%+v err=%v", asset, err)
	}
	if _, found, err := service.FindEpisode(1, "1"); err != nil || found {
		t.Fatalf("nil db FindEpisode should not find anything, found=%v err=%v", found, err)
	}
	if episodes, err := service.ListEpisodes(1); err != nil || len(episodes) != 0 {
		t.Fatalf("nil db ListEpisodes should be empty, episodes=%+v err=%v", episodes, err)
	}
}

func TestLibraryServiceErrorAndAssetBranches(t *testing.T) {
	rootDir := setupLibraryFixture(t)
	service, err := Open(rootDir)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer service.Close()
	novelID := NovelID(Work{ID: 1, Site: "syosetu", SiteWorkID: "n1234ab"})
	if asset, err := service.GetAsset(context.Background(), novelID, "not-assets/pic.jpg"); err != nil || asset != nil {
		t.Fatalf("non asset path should return nil, asset=%+v err=%v", asset, err)
	}
	if asset, err := service.GetAsset(context.Background(), novelID, "assets/episodes/1"); err != nil || asset != nil {
		t.Fatalf("asset directory should return nil, asset=%+v err=%v", asset, err)
	}
	if got := NewService(rootDir); got.db == nil {
		t.Fatal("NewService should open existing library")
	}

	badRoot := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(badRoot, "library.sqlite"))
	if err != nil {
		t.Fatalf("open bad sqlite: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE unrelated (id INTEGER)`); err != nil {
		t.Fatalf("create bad schema: %v", err)
	}
	db.Close()
	badService, err := Open(badRoot)
	if err != nil {
		t.Fatalf("Open bad schema returned error: %v", err)
	}
	defer badService.Close()
	if _, err := badService.ListWorks(); err == nil {
		t.Fatal("bad schema ListWorks should return error")
	}
	if status := badService.RuntimeStatus(context.Background()); status.Status != RuntimeStatusError {
		t.Fatalf("bad schema RuntimeStatus should be error, got %+v", status)
	}
}

func TestRuntimeStatusWarnsForEmptyLibrary(t *testing.T) {
	rootDir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(rootDir, "library.sqlite"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	execSQL(t, db, `
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
			site_episode_id TEXT NOT NULL,
			source_url TEXT NOT NULL,
			sort_order INTEGER NOT NULL,
			display_index TEXT NOT NULL,
			title TEXT NOT NULL,
			chapter TEXT NOT NULL,
			subchapter TEXT NOT NULL,
			published_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			body_path TEXT NOT NULL,
			raw_path TEXT NOT NULL,
			content_hash TEXT NOT NULL,
			fetched_at TEXT NOT NULL,
			body_status TEXT NOT NULL,
			last_fetch_error TEXT NOT NULL
		);
	`)
	db.Close()

	service, err := Open(rootDir)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer service.Close()
	status := service.RuntimeStatus(context.Background())
	if status.Status != RuntimeStatusWarn {
		t.Fatalf("empty library should warn, got %+v", status)
	}
}

func TestListNovelsSortsAndUsesSummaryFallbacks(t *testing.T) {
	rootDir := setupLibraryFixture(t)
	db, err := sql.Open("sqlite", filepath.Join(rootDir, "library.sqlite"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	execSQL(t, db, `
		INSERT INTO works (
			id, site, site_name, site_work_id, source_url, title, author, story, directory, fetched_at,
			fetch_status, expected_episode_count
		) VALUES (
			2, 'kakuyomu', '', 'k1', '', '', '', '', 'works/kakuyomu/k1', '2026-06-03T12:00:00Z',
			'', 0
		)
	`)
	db.Close()
	service, err := Open(rootDir)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer service.Close()
	list, err := service.ListNovels(context.Background())
	if err != nil {
		t.Fatalf("ListNovels returned error: %v", err)
	}
	if len(list.Novels) != 2 {
		t.Fatalf("expected two novels, got %+v", list)
	}
	if list.Novels[0].FetcherWorkID != "2" || list.Novels[0].Title != "Work 2" || list.Novels[0].SiteName != "novel-fetcher" {
		t.Fatalf("fallback summary was not used for newest work: %+v", list.Novels[0])
	}
}
