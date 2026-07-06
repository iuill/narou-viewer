package storageusage

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"narou-viewer/apps/viewer-api-go/internal/library"

	_ "modernc.org/sqlite"
)

func TestCollectClassifiesNovelDataCacheAndOtherStorage(t *testing.T) {
	dataDir := t.TempDir()
	createLibrarySQLite(t, filepath.Join(dataDir, "novel-fetcher", "library.sqlite"))
	writeFile(t, filepath.Join(dataDir, "novel-fetcher", "works", "syosetu", "n1234", "episodes", "1.json"), "episode-json")
	writeFile(t, filepath.Join(dataDir, "novel-fetcher", "works", "syosetu", "n1234", "raw", "episodes", "1.html"), "raw-html")
	writeFile(t, filepath.Join(dataDir, "novel-fetcher", "works", "syosetu", "n1234", "assets", "episodes", "1", "pic.jpg"), "jpg")
	writeFile(t, filepath.Join(dataDir, "state", "reader_search.sqlite"), "reader-cache")
	writeFile(t, filepath.Join(dataDir, "state", "bookmarks.yaml"), "state")
	writeFile(t, filepath.Join(dataDir, "小説データ", "小説家になろう", "n9999 Legacy", "本文", "1.txt"), "legacy-body")
	writeFile(t, filepath.Join(dataDir, "小説データ", "小説家になろう", "n9999 Legacy", "raw", "1.html"), "legacy-raw")

	usage, err := New(dataDir).Collect(nil)
	if err != nil {
		t.Fatalf("Collect returned error: %v", err)
	}

	if got := categoryBytes(usage, CategoryNovelData); got != int64(len("episode-json")+len("jpg")+len("legacy-body")) {
		t.Fatalf("novel data bytes = %d", got)
	}
	if got := categoryBytes(usage, CategoryCache); got != int64(len("raw-html")+len("reader-cache")+len("legacy-raw")) {
		t.Fatalf("cache bytes = %d", got)
	}
	if got := categoryBytes(usage, CategoryOther); got <= int64(len("state")) {
		t.Fatalf("other bytes should include state and sqlite metadata, got %d", got)
	}
	if usage.TotalBytes != categoryBytes(usage, CategoryNovelData)+categoryBytes(usage, CategoryCache)+categoryBytes(usage, CategoryOther) {
		t.Fatalf("total bytes should match category sum: %+v", usage)
	}
	if len(usage.Novels) != 2 {
		t.Fatalf("expected current and legacy novels, got %+v", usage.Novels)
	}
	current := findNovelByTitle(usage, "Fixture Novel")
	if current == nil {
		t.Fatalf("current novel not found: %+v", usage.Novels)
	}
	if current.NovelDataBytes != int64(len("episode-json")+len("jpg")) || current.CacheBytes != int64(len("raw-html")) {
		t.Fatalf("current novel bytes = %+v", current)
	}
	legacy := findNovelByTitle(usage, "Legacy")
	if legacy == nil || legacy.Source != "legacy" || legacy.NovelDataBytes != int64(len("legacy-body")) || legacy.CacheBytes != int64(len("legacy-raw")) {
		t.Fatalf("legacy novel bytes = %+v", legacy)
	}
}

func TestCollectReportsRoughNovelProgress(t *testing.T) {
	dataDir := t.TempDir()
	createLibrarySQLite(t, filepath.Join(dataDir, "novel-fetcher", "library.sqlite"))
	writeFile(t, filepath.Join(dataDir, "novel-fetcher", "works", "syosetu", "n1234", "episodes", "1.json"), "episode-json")
	writeFile(t, filepath.Join(dataDir, "novel-fetcher", "works", "syosetu", "n1234", "raw", "episodes", "1.html"), "raw-html")
	writeFile(t, filepath.Join(dataDir, "小説データ", "小説家になろう", "n9999 Legacy", "本文", "1.txt"), "legacy-body")

	var progressEvents []Progress
	usage, err := New(dataDir).CollectWithProgress(context.Background(), func(progress Progress) {
		progressEvents = append(progressEvents, progress)
	})
	if err != nil {
		t.Fatalf("CollectWithProgress returned error: %v", err)
	}
	if len(usage.Novels) != 2 {
		t.Fatalf("expected two novels, got %+v", usage.Novels)
	}
	if len(progressEvents) == 0 {
		t.Fatal("expected progress events")
	}
	if progressEvents[0].Phase != ProgressPhasePreparing {
		t.Fatalf("first progress event should be preparing, got %+v", progressEvents[0])
	}
	last := progressEvents[len(progressEvents)-1]
	if last.Phase != ProgressPhaseCompleted || last.CheckedNovels != 2 || last.TotalNovels != 2 {
		t.Fatalf("last progress event should complete two novels, got %+v", last)
	}
}

func TestCollectHandlesMissingDataDirAndCanceledContext(t *testing.T) {
	usage, err := New(filepath.Join(t.TempDir(), "missing")).Collect(context.Background())
	if err != nil {
		t.Fatalf("missing data dir should not fail: %v", err)
	}
	if usage.TotalBytes != 0 || len(usage.Novels) != 0 {
		t.Fatalf("missing data dir should return empty usage: %+v", usage)
	}
	if len(usage.Warnings) != 0 {
		t.Fatalf("missing data dir should not report warnings: %+v", usage.Warnings)
	}

	dataDir := t.TempDir()
	writeFile(t, filepath.Join(dataDir, "state", "reader_search.sqlite"), "cache")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = New(dataDir).Collect(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled context should be returned, got %v", err)
	}
}

func TestCollectFollowsRootDataDirSymlink(t *testing.T) {
	realDataDir := t.TempDir()
	linkDataDir := filepath.Join(t.TempDir(), "data-link")
	if err := os.Symlink(realDataDir, linkDataDir); err != nil {
		t.Skipf("symlink is unavailable: %v", err)
	}
	writeFile(t, filepath.Join(realDataDir, "novel-fetcher", "works", "syosetu", "n1234", "episodes", "1.json"), "body")

	usage, err := New(linkDataDir).Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect returned error: %v", err)
	}

	if usage.TotalBytes != int64(len("body")) {
		t.Fatalf("symlinked data dir should be scanned, got %+v", usage)
	}
}

func TestCollectUsesInjectedWorkListerBeforeLocalSQLiteFallback(t *testing.T) {
	dataDir := t.TempDir()
	writeFile(t, filepath.Join(dataDir, "novel-fetcher", "works", "syosetu", "n1234", "episodes", "1.json"), "body")
	work := library.Work{
		ID:         42,
		Site:       "syosetu",
		SiteName:   "小説家になろう",
		SiteWorkID: "n1234",
		Title:      "Injected Title",
		Author:     "Injected Author",
		Directory:  "works/syosetu/n1234",
	}
	lister := &stubWorkLister{works: []library.Work{work}}

	usage, err := New(dataDir, lister).Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect returned error: %v", err)
	}

	if lister.calls != 1 {
		t.Fatalf("injected work lister should be called once, got %d", lister.calls)
	}
	novel := findNovelByTitle(usage, "Injected Title")
	if novel == nil {
		t.Fatalf("injected metadata should be used: %+v", usage.Novels)
	}
	if novel.NovelID != library.NovelID(work) || novel.Author != "Injected Author" {
		t.Fatalf("injected novel identity should be preserved: %+v", novel)
	}
}

func TestCollectWarnsWhenInjectedWorkListerFailsAndFallbackIsEmpty(t *testing.T) {
	dataDir := t.TempDir()
	writeFile(t, filepath.Join(dataDir, "novel-fetcher", "works", "syosetu", "n1234", "episodes", "1.json"), "body")
	lister := &stubWorkLister{err: errors.New("fetcher unavailable")}

	usage, err := New(dataDir, lister).Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect returned error: %v", err)
	}

	if len(usage.Warnings) != 1 || !strings.Contains(usage.Warnings[0], "fetcher unavailable") {
		t.Fatalf("empty fallback should preserve injected metadata warning: %+v", usage.Warnings)
	}
	novel := findNovelByTitle(usage, "n1234")
	if novel == nil || !strings.HasPrefix(novel.NovelID, "unlisted:") {
		t.Fatalf("file should still be attributed by unlisted fallback: %+v", usage.Novels)
	}
}

func TestCollectFallsBackToLocalSQLiteWhenInjectedWorkListerFails(t *testing.T) {
	dataDir := t.TempDir()
	createLibrarySQLite(t, filepath.Join(dataDir, "novel-fetcher", "library.sqlite"))
	writeFile(t, filepath.Join(dataDir, "novel-fetcher", "works", "syosetu", "n1234", "episodes", "1.json"), "body")
	lister := &stubWorkLister{err: errors.New("fetcher unavailable")}

	usage, err := New(dataDir, lister).Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect returned error: %v", err)
	}

	if lister.calls != 1 {
		t.Fatalf("injected work lister should be called once, got %d", lister.calls)
	}
	novel := findNovelByTitle(usage, "Fixture Novel")
	if novel == nil || novel.NovelDataBytes != int64(len("body")) {
		t.Fatalf("local sqlite fallback should provide metadata: %+v", usage.Novels)
	}
	if len(usage.Warnings) != 0 {
		t.Fatalf("successful local sqlite fallback should not warn: %+v", usage.Warnings)
	}
}

func TestCollectIgnoresTypedNilWorkLister(t *testing.T) {
	dataDir := t.TempDir()
	createLibrarySQLite(t, filepath.Join(dataDir, "novel-fetcher", "library.sqlite"))
	writeFile(t, filepath.Join(dataDir, "novel-fetcher", "works", "syosetu", "n1234", "episodes", "1.json"), "body")
	var lister *stubWorkLister

	usage, err := New(dataDir, lister).Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect returned error: %v", err)
	}

	if novel := findNovelByTitle(usage, "Fixture Novel"); novel == nil {
		t.Fatalf("typed nil lister should allow local sqlite metadata fallback: %+v", usage.Novels)
	}
}

func TestCollectReturnsInjectedWorkListerCancellation(t *testing.T) {
	dataDir := t.TempDir()
	lister := &stubWorkLister{err: context.Canceled}

	_, err := New(dataDir, lister).Collect(context.Background())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("injected work lister cancellation should be returned, got %v", err)
	}
}

func TestCollectReportsInjectedAndFallbackMetadataWarnings(t *testing.T) {
	dataDir := t.TempDir()
	writeFile(t, filepath.Join(dataDir, "novel-fetcher", "library.sqlite"), "not sqlite")
	lister := &stubWorkLister{err: errors.New("fetcher unavailable")}

	usage, err := New(dataDir, lister).Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect returned error: %v", err)
	}

	if len(usage.Warnings) != 2 {
		t.Fatalf("expected injected and local metadata warnings, got %+v", usage.Warnings)
	}
	if !strings.Contains(usage.Warnings[0], "fetcher unavailable") {
		t.Fatalf("first warning should include injected error: %+v", usage.Warnings)
	}
	if !strings.Contains(usage.Warnings[1], "novel-fetcher library metadata could not be read") {
		t.Fatalf("second warning should include local metadata error: %+v", usage.Warnings)
	}
}

func TestCollectFallsBackForUnlistedNovelFetcherWorksAndMetadataWarnings(t *testing.T) {
	dataDir := t.TempDir()
	writeFile(t, filepath.Join(dataDir, "novel-fetcher", "library.sqlite"), "not sqlite")
	writeFile(t, filepath.Join(dataDir, "novel-fetcher", "works", "kakuyomu", "work-1", "raw", "episode.html"), "raw")
	writeFile(t, filepath.Join(dataDir, "novel-fetcher", "works", "kakuyomu", "work-1", "episodes", "episode.json"), "body")

	usage, err := New(dataDir).Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect returned error: %v", err)
	}
	if len(usage.Warnings) == 0 {
		t.Fatalf("invalid sqlite metadata should be reported as a warning: %+v", usage)
	}
	novel := findNovelByTitle(usage, "work-1")
	if novel == nil || novel.Source != "novel-fetcher" || novel.NovelDataBytes != int64(len("body")) || novel.CacheBytes != int64(len("raw")) {
		t.Fatalf("unlisted work should be attributed by directory: %+v", novel)
	}
}

func TestStorageUsageHelpersCoverFallbackBranches(t *testing.T) {
	targets := []workUsageTarget{{
		relDir:   "novel-fetcher/works/syosetu/n1234",
		novelID:  "novel-1",
		title:    "Title",
		siteName: "Site",
		source:   "novel-fetcher",
	}, {
		relDir:   "novel-fetcher/custom/syosetu/n5678",
		novelID:  "novel-2",
		title:    "Custom",
		siteName: "Site",
		source:   "novel-fetcher",
	}, {
		relDir:   "novel-fetcher/works/syosetu/n1234/special",
		novelID:  "novel-3",
		title:    "Nested",
		siteName: "Site",
		source:   "novel-fetcher",
	}}
	index := newWorkUsageIndex(targets)
	if classification := classifyPath("novel-fetcher/works/syosetu/n1234/memo.txt", index); classification.category != CategoryNovelData || classification.novel == nil {
		t.Fatalf("matched work metadata should be novel data: %+v", classification)
	}
	if classification := classifyPath("novel-fetcher/works/syosetu/n1234", index); classification.category != CategoryNovelData || classification.novel == nil || classification.novel.novelID != "novel-1" {
		t.Fatalf("matched work directory should use map index: %+v", classification)
	}
	if classification := classifyPath("novel-fetcher/custom/syosetu/n5678/raw/page.html", index); classification.category != CategoryCache || classification.novel == nil || classification.novel.novelID != "novel-2" {
		t.Fatalf("custom metadata path should fall back to prefix scan: %+v", classification)
	}
	if classification := classifyPath("novel-fetcher/works/syosetu/n1234/special/raw/page.html", index); classification.category != CategoryCache || classification.novel == nil || classification.novel.novelID != "novel-3" {
		t.Fatalf("nested metadata path should keep longest match semantics: %+v", classification)
	}
	if classification := classifyPath("novel-fetcher/works/kakuyomu/work-2/assets/pic.jpg", workUsageIndex{}); classification.category != CategoryNovelData || classification.novel == nil || classification.novel.novelID != "unlisted:kakuyomu:work-2" {
		t.Fatalf("fallback work should be classified: %+v", classification)
	}
	if classification := classifyPath("novel-fetcher/works/bad", workUsageIndex{}); classification.category != CategoryOther || classification.novel != nil {
		t.Fatalf("incomplete work path should be other: %+v", classification)
	}
	if got := classifyWorkFile(""); got != CategoryNovelData {
		t.Fatalf("empty work file rest should default to novel data, got %s", got)
	}
	if got := firstPathSegment(""); got != "" {
		t.Fatalf("empty first segment = %q", got)
	}
	if got := cleanRelPath("."); got != "" {
		t.Fatalf("cleanRelPath('.') = %q", got)
	}
	if target, _, ok := legacyNovelWork("小説データ/site/My Novel/本文/1.txt"); !ok || target.title != "My Novel" {
		t.Fatalf("legacy title without id prefix should be preserved: %+v", target)
	}
	if target, _, ok := legacyNovelWork("小説データ/site/n1234a Title/本文/1.txt"); !ok || target.title != "Title" {
		t.Fatalf("legacy title with ncode prefix should be trimmed: %+v", target)
	}
	if got := displayOrFallback("", " fallback "); got != "fallback" {
		t.Fatalf("displayOrFallback should trim fallback, got %q", got)
	}

	acc := newUsageAccumulator()
	acc.addWarning("")
	acc.addWarning(" warning ")
	acc.addFile("unknown", 3, fileClassification{})
	result := acc.result()
	if len(result.Warnings) != 1 || result.Warnings[0] != "warning" {
		t.Fatalf("warnings should trim and skip blank values: %+v", result.Warnings)
	}
	if categoryBytes(result, CategoryOther) != 3 {
		t.Fatalf("empty classification should count as other: %+v", result.Categories)
	}
}

type stubWorkLister struct {
	works []library.Work
	calls int
	err   error
}

func (s *stubWorkLister) ListWorksContext(context.Context) ([]library.Work, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return s.works, nil
}

func categoryBytes(usage StorageUsage, categoryID CategoryID) int64 {
	for _, category := range usage.Categories {
		if category.ID == categoryID {
			return category.Bytes
		}
	}
	return 0
}

func findNovelByTitle(usage StorageUsage, title string) *NovelUsage {
	for i := range usage.Novels {
		if usage.Novels[i].Title == title {
			return &usage.Novels[i]
		}
	}
	return nil
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func createLibrarySQLite(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir library dir: %v", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()
	_, err = db.Exec(`
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
	fetch_status TEXT NOT NULL DEFAULT 'complete',
	last_fetch_error TEXT NOT NULL DEFAULT '',
	last_failed_episode_id TEXT NOT NULL DEFAULT '',
	resume_episode_id TEXT NOT NULL DEFAULT '',
	expected_episode_count INTEGER NOT NULL DEFAULT 0
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
	body_status TEXT NOT NULL DEFAULT 'complete',
	last_fetch_error TEXT NOT NULL DEFAULT ''
);
INSERT INTO works VALUES (
	1, 'syosetu', '小説家になろう', 'n1234', 'https://ncode.syosetu.com/n1234/',
	'Fixture Novel', 'Author', 'Story', 'works/syosetu/n1234', '2026-01-01T00:00:00Z',
	'complete', '', '', '', 1
);
INSERT INTO episodes VALUES (
	1, '1', '1', 'https://ncode.syosetu.com/n1234/1/', 0, '1', 'Episode 1',
	'', '', '', '', 'works/syosetu/n1234/episodes/1.json',
	'works/syosetu/n1234/raw/episodes/1.html', 'hash', '', 'complete', ''
);
`)
	if err != nil {
		t.Fatalf("seed sqlite: %v", err)
	}
}
