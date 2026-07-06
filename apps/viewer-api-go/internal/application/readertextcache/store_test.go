package readertextcache

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"narou-viewer/apps/viewer-api-go/internal/library"
)

func TestStoreSavesAndReadsByContentEtag(t *testing.T) {
	store := New(t.TempDir())

	if err := store.Save(context.Background(), "novel-1", "1", "etag-1", "本文"); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	entry, ok, err := store.Get(context.Background(), "novel-1", "1", "etag-1")
	if err != nil || !ok || entry.Text != "本文" || entry.PlainTextLength != 2 {
		t.Fatalf("Get = %+v ok=%v err=%v", entry, ok, err)
	}
	if entry, ok, err := store.Get(context.Background(), "novel-1", "1", "etag-old"); err != nil || ok || entry.Text != "" {
		t.Fatalf("stale ETag should miss, entry=%+v ok=%v err=%v", entry, ok, err)
	}
	if err := store.Save(context.Background(), "novel-1", "1", "etag-2", "更新本文"); err != nil {
		t.Fatalf("second Save returned error: %v", err)
	}
	if entry, ok, err := store.Get(context.Background(), "novel-1", "1", "etag-1"); err != nil || ok || entry.Text != "" {
		t.Fatalf("old ETag should be pruned after upsert, entry=%+v ok=%v err=%v", entry, ok, err)
	}
	if entry, ok, err := store.Get(context.Background(), "novel-1", "1", "etag-2"); err != nil || !ok || entry.Text != "更新本文" {
		t.Fatalf("new ETag should be addressable, entry=%+v ok=%v err=%v", entry, ok, err)
	}
	if err := store.Save(context.Background(), "novel-1", "2", "etag-episode-2", "二話"); err != nil {
		t.Fatalf("third Save returned error: %v", err)
	}
	entries, err := store.GetMany(context.Background(), "novel-1", []LookupKey{
		{EpisodeIndex: "1", ContentEtag: "etag-2"},
		{EpisodeIndex: "2", ContentEtag: "etag-missing"},
	})
	if err != nil {
		t.Fatalf("GetMany returned error: %v", err)
	}
	if len(entries) != 1 || entries[Key{EpisodeIndex: "1", ContentEtag: "etag-2"}].Text != "更新本文" {
		t.Fatalf("GetMany should return only requested matching keys: %+v", entries)
	}
	db, err := store.open(context.Background())
	if err != nil {
		t.Fatalf("open returned error: %v", err)
	}
	unchanged, err := hasUnchangedCurrentRow(context.Background(), db, "novel-1", "1", "etag-2", "更新本文")
	if err != nil || !unchanged {
		t.Fatalf("hasUnchangedCurrentRow should detect unchanged cache row: unchanged=%v err=%v", unchanged, err)
	}
	info, err := os.Stat(store.dbPath)
	if err != nil {
		t.Fatalf("stat reader search sqlite: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("reader search sqlite mode = %o, want 600", mode)
	}
}

func TestStoreNoopBranches(t *testing.T) {
	if store := New("   "); store != nil {
		t.Fatalf("New with empty stateDir should return nil: %+v", store)
	}
	if store := NewAtPath("   "); store != nil {
		t.Fatalf("NewAtPath with empty path should return nil: %+v", store)
	}
	if entry, ok, err := (*Store)(nil).Get(context.Background(), "novel", "1", "etag"); err != nil || ok || entry.Text != "" {
		t.Fatalf("nil Get should miss without error: entry=%+v ok=%v err=%v", entry, ok, err)
	}
	store := New(t.TempDir())
	if err := store.Save(context.Background(), "novel-1", "1", "", "本文"); err != nil {
		t.Fatalf("Save with empty ETag should be a no-op: %v", err)
	}
	if entries, err := store.GetMany(context.Background(), "novel-1", nil); err != nil || len(entries) != 0 {
		t.Fatalf("GetMany with no keys should return empty map: entries=%+v err=%v", entries, err)
	}
	if rows, err := (*Store)(nil).PruneByNovelID(context.Background(), "novel-1"); err != nil || rows != 0 {
		t.Fatalf("nil PruneByNovelID should be a no-op: rows=%d err=%v", rows, err)
	}
	if rows, err := store.PruneByNovelID(context.Background(), " "); err != nil || rows != 0 {
		t.Fatalf("empty novel prune should be a no-op: rows=%d err=%v", rows, err)
	}
	if rows, err := NewAtPath(filepath.Join(t.TempDir(), "missing", "reader_search.sqlite")).PruneByNovelID(context.Background(), "novel-1"); err != nil || rows != 0 {
		t.Fatalf("missing DB prune should be a no-op: rows=%d err=%v", rows, err)
	}
}

func TestStoreGetManyNormalizesLookupKeysAndPrunesNovel(t *testing.T) {
	store := New(t.TempDir())
	if err := store.Save(context.Background(), "novel-1", "1", "etag-1", "本文"); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	if err := store.Save(context.Background(), "novel-1", "2", "etag-2", "二話"); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	entries, err := store.GetMany(context.Background(), "novel-1", []LookupKey{
		{EpisodeIndex: " 1 ", ContentEtag: " etag-1 "},
		{EpisodeIndex: "1", ContentEtag: "etag-1"},
		{EpisodeIndex: "", ContentEtag: "etag-2"},
		{EpisodeIndex: "2", ContentEtag: ""},
	})
	if err != nil {
		t.Fatalf("GetMany returned error: %v", err)
	}
	if len(entries) != 1 || entries[Key{EpisodeIndex: "1", ContentEtag: "etag-1"}].Text != "本文" {
		t.Fatalf("GetMany should normalize and dedupe lookup keys: %+v", entries)
	}
	deleted, err := store.PruneByNovelID(context.Background(), "novel-1")
	if err != nil || deleted != 2 {
		t.Fatalf("PruneByNovelID deleted=%d err=%v", deleted, err)
	}
	if entry, ok, err := store.Get(context.Background(), "novel-1", "1", "etag-1"); err != nil || ok || entry.Text != "" {
		t.Fatalf("pruned entry should miss: entry=%+v ok=%v err=%v", entry, ok, err)
	}
}

func TestStoreOpenFailureCanBeRetried(t *testing.T) {
	root := t.TempDir()
	blockedParent := filepath.Join(root, "blocked")
	if err := os.WriteFile(blockedParent, []byte("not a dir"), 0o644); err != nil {
		t.Fatalf("write blocked parent: %v", err)
	}
	store := NewAtPath(filepath.Join(blockedParent, "reader_search.sqlite"))
	if err := store.Save(context.Background(), "novel-1", "1", "etag-1", "本文"); err == nil {
		t.Fatal("first Save should fail while parent path is blocked")
	}
	if err := os.Remove(blockedParent); err != nil {
		t.Fatalf("remove blocked parent: %v", err)
	}
	if err := store.Save(context.Background(), "novel-1", "1", "etag-1", "本文"); err != nil {
		t.Fatalf("second Save should retry open after transient failure: %v", err)
	}
}

func TestBodyTextMatchesReaderAssistantSearchTarget(t *testing.T) {
	text := BodyText(library.ReaderDocument{Blocks: []library.ReaderBlock{
		{Type: "title", Text: "ignored"},
		{Type: "paragraph", Section: "body", Inlines: []library.ReaderInline{{Type: "text", Text: " first "}}},
		{Type: "heading", Section: "body", Inlines: []library.ReaderInline{{Type: "text", Text: "second"}}},
		{Type: "html", Section: "body", PlainText: "ignored html"},
	}})
	if text != "first\nsecond" {
		t.Fatalf("BodyText = %q", text)
	}
}

func TestReaderSearchSQLiteDSNUsesFileURI(t *testing.T) {
	path := filepath.Join("tmp", "reader_search.sqlite")
	if got := readerSearchSQLiteDSN(path); got != "file:tmp/reader_search.sqlite?_pragma=busy_timeout(5000)" {
		t.Fatalf("unexpected DSN: %s", got)
	}
}
