package library

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestServiceReadsNovelFetcherLibrary(t *testing.T) {
	rootDir := setupLibraryFixture(t)
	service, err := Open(rootDir)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer service.Close()

	ctx := context.Background()
	novelID := NovelID(Work{ID: 1, Site: "syosetu", SiteWorkID: "n1234ab"})

	list, err := service.ListNovels(ctx)
	if err != nil {
		t.Fatalf("ListNovels() error = %v", err)
	}
	if len(list.Novels) != 1 {
		t.Fatalf("ListNovels() returned %d novels, want 1", len(list.Novels))
	}
	novel := list.Novels[0]
	if novel.NovelID != novelID {
		t.Fatalf("NovelID = %q, want %q", novel.NovelID, novelID)
	}
	if work, ok, err := service.FindWork(novelID); err != nil || !ok || work.ID != 1 {
		t.Fatalf("FindWork should resolve canonical site novel ID: work=%+v ok=%v err=%v", work, ok, err)
	}
	legacyURLNovelID := encodeNovelID("url:https://ncode.syosetu.com/n1234ab/")
	if work, ok, err := service.FindWork(legacyURLNovelID); err != nil || ok || work.ID != 0 {
		t.Fatalf("FindWork should not accept pre-release URL-based novel IDs: work=%+v ok=%v err=%v", work, ok, err)
	}
	fetcherFallbackNovelID := encodeNovelID("novel-fetcher:1")
	if work, ok, err := service.FindWork(fetcherFallbackNovelID); err != nil || !ok || work.ID != 1 {
		t.Fatalf("FindWork should resolve fallback fetcher novel IDs: work=%+v ok=%v err=%v", work, ok, err)
	}
	if exists, err := service.NovelExists(novelID); err != nil || !exists {
		t.Fatalf("NovelExists should resolve canonical site novel ID: exists=%v err=%v", exists, err)
	}
	if exists, err := service.NovelExists(fetcherFallbackNovelID); err != nil || !exists {
		t.Fatalf("NovelExists should resolve fallback fetcher novel ID: exists=%v err=%v", exists, err)
	}
	for _, malformedNovelID := range []string{"not-base64", encodeNovelID("site:syosetu"), encodeNovelID("novel-fetcher:not-number")} {
		if work, ok, err := service.FindWork(malformedNovelID); err != nil || ok || work.ID != 0 {
			t.Fatalf("FindWork should reject malformed novel ID %q: work=%+v ok=%v err=%v", malformedNovelID, work, ok, err)
		}
		if exists, err := service.NovelExists(malformedNovelID); err != nil || exists {
			t.Fatalf("NovelExists should reject malformed novel ID %q: exists=%v err=%v", malformedNovelID, exists, err)
		}
	}
	if novel.Title != "API テスト作品" || novel.Author != "テスト作者" || novel.SiteName != "小説家になろう" {
		t.Fatalf("unexpected novel summary: %+v", novel)
	}
	if novel.TotalEpisodes != 2 || novel.SavedEpisodes != 1 {
		t.Fatalf("episode counts = total %d saved %d, want total 2 saved 1", novel.TotalEpisodes, novel.SavedEpisodes)
	}
	if novel.TocURL == nil || *novel.TocURL != "https://ncode.syosetu.com/n1234ab/" {
		t.Fatalf("TocURL = %v, want normalized source URL", novel.TocURL)
	}

	toc, err := service.GetToc(ctx, novelID)
	if err != nil {
		t.Fatalf("GetToc() error = %v", err)
	}
	if toc == nil {
		t.Fatal("GetToc() returned nil")
	}
	if toc.Story != "あらすじ" || len(toc.Episodes) != 2 {
		t.Fatalf("unexpected toc: %+v", toc)
	}
	if toc.Episodes[0].EpisodeIndex != "1" || toc.Episodes[0].BodyStatus != "complete" {
		t.Fatalf("first toc episode = %+v", toc.Episodes[0])
	}
	if toc.Episodes[0].UpdatedAt == nil || *toc.Episodes[0].UpdatedAt != "2026-06-01T00:00:00Z" {
		t.Fatalf("first toc episode UpdatedAt = %v, want published_at fallback", toc.Episodes[0].UpdatedAt)
	}
	if toc.Episodes[1].EpisodeIndex != "2" || toc.Episodes[1].BodyStatus != "pending" {
		t.Fatalf("second toc episode = %+v", toc.Episodes[1])
	}

	episode, err := service.GetEpisode(ctx, novelID, "1")
	if err != nil {
		t.Fatalf("GetEpisode() error = %v", err)
	}
	if episode == nil {
		t.Fatal("GetEpisode() returned nil")
	}
	if episode.NovelID != novelID || episode.EpisodeIndex != "1" || episode.Title != "本文タイトル" {
		t.Fatalf("unexpected episode identity: %+v", episode)
	}
	if episode.Chapter == nil || *episode.Chapter != "第一章" {
		t.Fatalf("Chapter = %v, want 第一章", episode.Chapter)
	}
	if episode.Subchapter == nil || *episode.Subchapter != "幕間" {
		t.Fatalf("Subchapter = %v, want 幕間", episode.Subchapter)
	}
	if episode.SourceURL == nil || *episode.SourceURL != "https://ncode.syosetu.com/n1234ab/1/" {
		t.Fatalf("SourceURL = %v, want episode source URL", episode.SourceURL)
	}
	if episode.PlainTextLength != len([]rune("前書きです\n本文です\n子テキスト\nHTML本文\n続き")) {
		t.Fatalf("PlainTextLength = %d", episode.PlainTextLength)
	}
	if !strings.Contains(episode.HTML, `<h1 class="reader-title">本文タイトル</h1>`) {
		t.Fatalf("episode HTML does not contain title: %s", episode.HTML)
	}
	if strings.Count(episode.HTML, "本文タイトル") != 1 || strings.Count(episode.HTML, "第一章") != 1 {
		t.Fatalf("episode HTML should not duplicate canonical meta/title blocks: %s", episode.HTML)
	}
	if !strings.Contains(episode.HTML, "reader-section-introduction") || !strings.Contains(episode.HTML, "reader-section-postscript") {
		t.Fatalf("episode HTML should preserve canonical content sections: %s", episode.HTML)
	}
	if !strings.Contains(episode.HTML, "/api/library/novels/"+novelID+"/assets/assets/episodes/1/pic.jpg") {
		t.Fatalf("episode HTML did not rewrite asset reference: %s", episode.HTML)
	}
	blocks := episode.ReaderDocument.Blocks
	if len(blocks) != 10 {
		t.Fatalf("unexpected reader document block count %d: %+v", len(blocks), episode.ReaderDocument)
	}
	if blocks[3].Type != "paragraph" || blocks[3].Section != "introduction" ||
		len(blocks[3].Inlines) != 1 || blocks[3].Inlines[0].Text != "前書きです" {
		t.Fatalf("unexpected introduction block: %+v", blocks[3])
	}
	if blocks[4].Type != "html" || blocks[4].Section != "body" ||
		!strings.Contains(blocks[4].HTML, "reader-section-separator") {
		t.Fatalf("expected section separator before body: %+v", blocks[4])
	}
	if blocks[5].Type != "paragraph" || blocks[5].Section != "body" || blocks[5].Inlines[0].Text != "本文です" {
		t.Fatalf("unexpected first body paragraph: %+v", blocks[5])
	}
	if blocks[6].Type != "paragraph" || blocks[6].Inlines[0].Text != "子テキスト" {
		t.Fatalf("unexpected second body paragraph: %+v", blocks[6])
	}
	if blocks[7].Type != "paragraph" || len(blocks[7].Inlines) != 3 ||
		blocks[7].Inlines[0].Text != "HTML本文" || blocks[7].Inlines[1].Type != "lineBreak" || blocks[7].Inlines[2].Text != "続き" {
		t.Fatalf("unexpected third body paragraph: %+v", blocks[7])
	}
	if blocks[8].Type != "html" || blocks[8].Section != "postscript" ||
		!strings.Contains(blocks[8].HTML, "reader-section-separator") {
		t.Fatalf("expected section separator before postscript: %+v", blocks[8])
	}
	if blocks[9].Type != "image" || blocks[9].Section != "postscript" ||
		!strings.Contains(blocks[9].Src, "/assets/assets/episodes/1/pic.jpg") {
		t.Fatalf("unexpected postscript image block: %+v", blocks[9])
	}
	if episode.ContentEtag != "sha256:episode-1" {
		t.Fatalf("ContentEtag = %q, want DB content hash", episode.ContentEtag)
	}

	missingEpisode, err := service.GetEpisode(ctx, novelID, "2")
	if err != nil {
		t.Fatalf("GetEpisode(pending) error = %v", err)
	}
	if missingEpisode != nil {
		t.Fatalf("GetEpisode(pending) = %+v, want nil", missingEpisode)
	}

	asset, err := service.GetAsset(ctx, novelID, "assets/episodes/1/pic.jpg")
	if err != nil {
		t.Fatalf("GetAsset() error = %v", err)
	}
	if asset == nil {
		t.Fatal("GetAsset() returned nil")
	}
	if asset.FilePath != filepath.Join(rootDir, "works/syosetu/n1234ab/assets/episodes/1/pic.jpg") {
		t.Fatalf("asset FilePath = %q", asset.FilePath)
	}
	if asset.MediaType != "image/jpeg" {
		t.Fatalf("asset MediaType = %q, want image/jpeg", asset.MediaType)
	}
	blockedAsset, err := service.GetAsset(ctx, novelID, "../library.sqlite")
	if err != nil {
		t.Fatalf("GetAsset(blocked) error = %v", err)
	}
	if blockedAsset != nil {
		t.Fatalf("GetAsset(blocked) = %+v, want nil", blockedAsset)
	}
	outsideAsset := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outsideAsset, []byte("secret"), 0o644); err != nil {
		t.Fatalf("write outside asset: %v", err)
	}
	symlinkAssetPath := filepath.Join(rootDir, "works/syosetu/n1234ab/assets/episodes/1/escape.txt")
	if err := os.Symlink(outsideAsset, symlinkAssetPath); err != nil {
		t.Skipf("symlink is not available: %v", err)
	}
	escapedAsset, err := service.GetAsset(ctx, novelID, "assets/episodes/1/escape.txt")
	if err != nil {
		t.Fatalf("GetAsset(symlink escape) error = %v", err)
	}
	if escapedAsset != nil {
		t.Fatalf("GetAsset(symlink escape) = %+v, want nil", escapedAsset)
	}
	status := service.RuntimeStatus(ctx)
	if status.Status != RuntimeStatusOK {
		t.Fatalf("RuntimeStatus.Status = %q, want ok: %+v", status.Status, status)
	}
	if len(status.Services) != 2 {
		t.Fatalf("RuntimeStatus.Services length = %d, want 2", len(status.Services))
	}
	if status.Services[1].ID != "library" || status.Services[1].Status != RuntimeStatusOK || status.Services[1].Summary != "1 作品" {
		t.Fatalf("library runtime status = %+v", status.Services[1])
	}
}

func TestServiceRejectsSymlinkEscapedWorkDirectoryAssets(t *testing.T) {
	rootDir := setupLibraryFixture(t)
	outsideWorkRoot := filepath.Join(t.TempDir(), "outside-work")
	outsideWorkAssetDir := filepath.Join(outsideWorkRoot, "assets/episodes/1")
	if err := os.MkdirAll(outsideWorkAssetDir, 0o755); err != nil {
		t.Fatalf("mkdir outside work asset dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outsideWorkAssetDir, "pic.jpg"), []byte("outside"), 0o644); err != nil {
		t.Fatalf("write outside work asset: %v", err)
	}
	workDirSymlink := filepath.Join(rootDir, "works/escaped-work")
	if err := os.Symlink(outsideWorkRoot, workDirSymlink); err != nil {
		t.Skipf("symlink is not available: %v", err)
	}
	db, err := sql.Open("sqlite", filepath.Join(rootDir, "library.sqlite"))
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	execSQL(t, db, `UPDATE works SET directory = 'works/escaped-work' WHERE id = 1`)
	if err := db.Close(); err != nil {
		t.Fatalf("db.Close() error = %v", err)
	}

	service, err := Open(rootDir)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer service.Close()

	novelID := NovelID(Work{ID: 1, Site: "syosetu", SiteWorkID: "n1234ab"})
	escapedWorkAsset, err := service.GetAsset(context.Background(), novelID, "assets/episodes/1/pic.jpg")
	if err != nil {
		t.Fatalf("GetAsset(work dir symlink escape) error = %v", err)
	}
	if escapedWorkAsset != nil {
		t.Fatalf("GetAsset(work dir symlink escape) = %+v, want nil", escapedWorkAsset)
	}
}

func setupLibraryFixture(t *testing.T) string {
	t.Helper()

	rootDir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(rootDir, "library.sqlite"))
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	defer db.Close()

	execSQL(t, db, `
		CREATE TABLE works (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			site TEXT NOT NULL,
			site_name TEXT NOT NULL,
			site_work_id TEXT NOT NULL,
			source_url TEXT NOT NULL,
			title TEXT NOT NULL,
			author TEXT NOT NULL,
			story TEXT NOT NULL,
			directory TEXT NOT NULL,
			fetched_at TEXT NOT NULL,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			fetch_status TEXT NOT NULL DEFAULT 'complete',
			last_fetch_error TEXT NOT NULL DEFAULT '',
			last_failed_episode_id TEXT NOT NULL DEFAULT '',
			resume_episode_id TEXT NOT NULL DEFAULT '',
			expected_episode_count INTEGER NOT NULL DEFAULT 0,
			UNIQUE(site, site_work_id)
		)
	`)
	execSQL(t, db, `
		CREATE TABLE episodes (
			work_id INTEGER NOT NULL REFERENCES works(id) ON DELETE CASCADE,
			episode_id TEXT NOT NULL,
			site_episode_id TEXT NOT NULL,
			source_url TEXT NOT NULL DEFAULT '',
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
			body_status TEXT NOT NULL DEFAULT 'complete',
			last_fetch_error TEXT NOT NULL DEFAULT '',
			last_attempted_at TEXT NOT NULL DEFAULT '',
			PRIMARY KEY(work_id, episode_id)
		)
	`)
	execSQL(t, db, `CREATE INDEX episodes_work_sort_idx ON episodes(work_id, sort_order)`)

	execSQL(t, db, `
		INSERT INTO works (
			id, site, site_name, site_work_id, source_url, title, author, story, directory, fetched_at,
			fetch_status, expected_episode_count
		) VALUES (
			1, 'syosetu', '小説家になろう', 'n1234ab', 'https://ncode.syosetu.com/n1234ab/',
			'API テスト作品', 'テスト作者', 'あらすじ', 'works/syosetu/n1234ab', '2026-06-01T12:00:00Z',
			'complete', 2
		)
	`)
	execSQL(t, db, `
		INSERT INTO episodes (
			work_id, episode_id, site_episode_id, source_url, sort_order, display_index, title, chapter, subchapter,
			published_at, updated_at, body_path, raw_path, content_hash, fetched_at, body_status
		) VALUES
			(1, '1', '1', 'https://ncode.syosetu.com/n1234ab/1/', 1, '1', 'DBタイトル', 'DB章', '',
			 '2026-06-01T00:00:00Z', '', 'works/syosetu/n1234ab/episodes/1.json',
			 'works/syosetu/n1234ab/raw/episodes/1.html', 'sha256:episode-1', '2026-06-02T01:00:00Z', 'complete'),
			(1, '2', '2', 'https://ncode.syosetu.com/n1234ab/2/', 2, '2', '未保存話', '', '',
			 '2026-06-03T00:00:00Z', '2026-06-03T00:00:00Z', '',
			 '', '', '2026-06-03T01:00:00Z', 'pending')
	`)

	episodeDir := filepath.Join(rootDir, "works/syosetu/n1234ab/episodes")
	if err := os.MkdirAll(episodeDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(episodeDir) error = %v", err)
	}
	assetDir := filepath.Join(rootDir, "works/syosetu/n1234ab/assets/episodes/1")
	if err := os.MkdirAll(assetDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(assetDir) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(assetDir, "pic.jpg"), []byte("jpg"), 0o644); err != nil {
		t.Fatalf("WriteFile(asset) error = %v", err)
	}

	canonical := CanonicalEpisode{
		SchemaVersion: 1,
		EpisodeID:     "1",
		SiteEpisodeID: "1",
		SourceURL:     "https://ncode.syosetu.com/n1234ab/1/",
		SortOrder:     1,
		DisplayIndex:  "1",
		Title:         "本文タイトル",
		Chapter:       "第一章",
		Subchapter:    "幕間",
		PublishedAt:   "2026-06-01T00:00:00Z",
		UpdatedAt:     "2026-06-02T00:00:00Z",
		FetchedAt:     time.Date(2026, 6, 2, 1, 0, 0, 0, time.UTC),
		Blocks: []BodyBlock{
			{Type: "meta", Text: "第一章"},
			{Type: "title", Text: "本文タイトル"},
			{Type: "paragraph", Section: "introduction", Text: "前書きです"},
			{Type: "paragraph", Section: "body", Text: "本文です"},
			{Type: "paragraph", Section: "body", Children: []BodyInline{{Type: "text", Text: "子テキスト"}}},
			{Type: "html", Section: "body", HTML: "<p>HTML本文<br>続き</p>"},
			{Type: "image", Section: "postscript", Src: "assets/episodes/1/pic.jpg", Alt: "挿絵", Width: 320, Height: 240},
		},
	}
	bytes, err := json.Marshal(canonical)
	if err != nil {
		t.Fatalf("json.Marshal(canonical) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(episodeDir, "1.json"), bytes, 0o644); err != nil {
		t.Fatalf("WriteFile(episode) error = %v", err)
	}

	return rootDir
}

func execSQL(t *testing.T, db *sql.DB, statement string) {
	t.Helper()
	if _, err := db.Exec(statement); err != nil {
		t.Fatalf("db.Exec(%q) error = %v", statement, err)
	}
}
