package storageusage

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"narou-viewer/apps/viewer-api-go/internal/library"
)

type CategoryID string

const (
	CategoryNovelData CategoryID = "novelData"
	CategoryCache     CategoryID = "cache"
	CategoryOther     CategoryID = "other"
)

type CategoryUsage struct {
	ID        CategoryID `json:"id"`
	Label     string     `json:"label"`
	Bytes     int64      `json:"bytes"`
	FileCount int        `json:"fileCount"`
}

type NovelUsage struct {
	NovelID        string `json:"novelId"`
	Title          string `json:"title"`
	Author         string `json:"author,omitempty"`
	SiteName       string `json:"siteName"`
	Source         string `json:"source"`
	TotalBytes     int64  `json:"totalBytes"`
	NovelDataBytes int64  `json:"novelDataBytes"`
	CacheBytes     int64  `json:"cacheBytes"`
	OtherBytes     int64  `json:"otherBytes"`
	FileCount      int    `json:"fileCount"`
}

type StorageUsage struct {
	CheckedAt  string          `json:"checkedAt"`
	TotalBytes int64           `json:"totalBytes"`
	Categories []CategoryUsage `json:"categories"`
	Novels     []NovelUsage    `json:"novels"`
	Warnings   []string        `json:"warnings,omitempty"`
}

type ProgressPhase string

const (
	ProgressPhasePreparing ProgressPhase = "preparing"
	ProgressPhaseScanning  ProgressPhase = "scanning"
	ProgressPhaseCompleted ProgressPhase = "completed"
)

type Progress struct {
	Phase         ProgressPhase `json:"phase"`
	CheckedNovels int           `json:"checkedNovels"`
	TotalNovels   int           `json:"totalNovels"`
}

type ProgressReporter func(Progress)

type Service struct {
	dataDir    string
	workLister WorkLister
}

type WorkLister interface {
	ListWorksContext(context.Context) ([]library.Work, error)
}

type workUsageTarget struct {
	relDir   string
	novelID  string
	title    string
	author   string
	siteName string
	source   string
}

type workUsageIndex struct {
	targets             []workUsageTarget
	byDir               map[string]int
	nestedPrefixByDir   map[string][]int
	customPrefixIndexes []int
}

type workListResult struct {
	works       []library.Work
	injectedErr error
}

type usageAccumulator struct {
	categories map[CategoryID]*CategoryUsage
	novels     map[string]*NovelUsage
	warnings   []string
}

type progressTracker struct {
	report       ProgressReporter
	seenNovels   map[string]struct{}
	checkedCount int
	totalCount   int
}

func New(dataDir string, workListers ...WorkLister) *Service {
	var workLister WorkLister
	for _, candidate := range workListers {
		if !isNilWorkLister(candidate) {
			workLister = candidate
			break
		}
	}
	return &Service{dataDir: strings.TrimSpace(dataDir), workLister: workLister}
}

func isNilWorkLister(workLister WorkLister) bool {
	if workLister == nil {
		return true
	}
	value := reflect.ValueOf(workLister)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func (s *Service) Collect(ctx context.Context) (StorageUsage, error) {
	return s.CollectWithProgress(ctx, nil)
}

func (s *Service) CollectWithProgress(ctx context.Context, report ProgressReporter) (StorageUsage, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return StorageUsage{}, err
	}
	dataDir := strings.TrimSpace(s.dataDir)
	if dataDir == "" {
		dataDir = "."
	}
	dataDir = filepath.Clean(dataDir)
	acc := newUsageAccumulator()
	reportProgress(report, Progress{Phase: ProgressPhasePreparing})
	if _, err := os.Stat(dataDir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			reportProgress(report, Progress{Phase: ProgressPhaseCompleted})
			return acc.result(), nil
		}
		return StorageUsage{}, err
	}
	resolvedDataDir, err := filepath.EvalSymlinks(dataDir)
	if err != nil {
		return StorageUsage{}, err
	}
	dataDir = filepath.Clean(resolvedDataDir)
	targets, err := s.loadNovelFetcherTargets(ctx, dataDir, acc)
	if err != nil {
		return StorageUsage{}, err
	}
	progress := newProgressTracker(targets, report)
	progress.emit()

	err = filepath.WalkDir(dataDir, func(path string, d fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			acc.addWarning(walkErr.Error())
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			acc.addWarning(err.Error())
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(dataDir, path)
		if err != nil {
			acc.addWarning(err.Error())
			return nil
		}
		classification := classifyPath(cleanRelPath(rel), targets)
		progress.record(classification.novel)
		acc.addFile(cleanRelPath(rel), info.Size(), classification)
		return nil
	})
	if err != nil {
		return StorageUsage{}, err
	}
	progress.complete()
	return acc.result(), nil
}

func reportProgress(report ProgressReporter, progress Progress) {
	if report != nil {
		report(progress)
	}
}

func newProgressTracker(index workUsageIndex, report ProgressReporter) *progressTracker {
	knownNovels := map[string]struct{}{}
	for _, target := range index.targets {
		if target.novelID == "" {
			continue
		}
		knownNovels[target.novelID] = struct{}{}
	}
	return &progressTracker{
		report:     report,
		seenNovels: map[string]struct{}{},
		totalCount: len(knownNovels),
	}
}

func (p *progressTracker) emit() {
	reportProgress(p.report, Progress{
		Phase:         ProgressPhaseScanning,
		CheckedNovels: p.checkedCount,
		TotalNovels:   p.totalCount,
	})
}

func (p *progressTracker) record(target *workUsageTarget) {
	if target == nil || target.novelID == "" {
		return
	}
	if _, ok := p.seenNovels[target.novelID]; ok {
		return
	}
	p.seenNovels[target.novelID] = struct{}{}
	p.checkedCount = len(p.seenNovels)
	if p.checkedCount > p.totalCount {
		p.totalCount = p.checkedCount
	}
	p.emit()
}

func (p *progressTracker) complete() {
	if p.totalCount > 0 && p.checkedCount < p.totalCount {
		p.checkedCount = p.totalCount
	}
	reportProgress(p.report, Progress{
		Phase:         ProgressPhaseCompleted,
		CheckedNovels: p.checkedCount,
		TotalNovels:   p.totalCount,
	})
}

func (s *Service) loadNovelFetcherTargets(ctx context.Context, dataDir string, acc *usageAccumulator) (workUsageIndex, error) {
	if err := ctx.Err(); err != nil {
		return workUsageIndex{}, err
	}
	result, err := s.listWorks(ctx, dataDir)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return workUsageIndex{}, err
		}
		if result.injectedErr != nil {
			acc.addWarning("injected library metadata could not be read: " + result.injectedErr.Error())
		}
		acc.addWarning("novel-fetcher library metadata could not be read: " + err.Error())
		return workUsageIndex{}, nil
	}
	if result.injectedErr != nil && len(result.works) == 0 {
		acc.addWarning("injected library metadata could not be read: " + result.injectedErr.Error())
	}
	targets := make([]workUsageTarget, 0, len(result.works))
	for _, work := range result.works {
		if err := ctx.Err(); err != nil {
			return workUsageIndex{}, err
		}
		relDir := cleanRelPath(filepath.Join("novel-fetcher", work.Directory))
		if relDir == "novel-fetcher" || !strings.HasPrefix(relDir, "novel-fetcher/works/") {
			relDir = cleanRelPath(filepath.Join("novel-fetcher", "works", work.Site, work.SiteWorkID))
		}
		targets = append(targets, workUsageTarget{
			relDir:   relDir,
			novelID:  library.NovelID(work),
			title:    displayOrFallback(work.Title, work.SiteWorkID),
			author:   work.Author,
			siteName: displayOrFallback(work.SiteName, work.Site),
			source:   "novel-fetcher",
		})
	}
	return newWorkUsageIndex(targets), nil
}

func (s *Service) listWorks(ctx context.Context, dataDir string) (workListResult, error) {
	result := workListResult{}
	if s.workLister != nil {
		works, err := s.workLister.ListWorksContext(ctx)
		if err == nil {
			result.works = works
			return result, nil
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return workListResult{}, err
		}
		result.injectedErr = err
	}
	service := library.NewService(filepath.Join(dataDir, "novel-fetcher"))
	defer service.Close()
	works, err := service.ListWorksContext(ctx)
	if err != nil {
		return result, err
	}
	result.works = works
	return result, nil
}

type fileClassification struct {
	category CategoryID
	novel    *workUsageTarget
}

func classifyPath(rel string, index workUsageIndex) fileClassification {
	if target, rest, ok := matchNovelFetcherWork(rel, index); ok {
		return fileClassification{category: classifyWorkFile(rest), novel: target}
	}
	if target, rest, ok := fallbackNovelFetcherWork(rel); ok {
		return fileClassification{category: classifyWorkFile(rest), novel: target}
	}
	if target, rest, ok := legacyNovelWork(rel); ok {
		return fileClassification{category: classifyLegacyWorkFile(rest), novel: target}
	}
	if isCachePath(rel) {
		return fileClassification{category: CategoryCache}
	}
	return fileClassification{category: CategoryOther}
}

func newWorkUsageIndex(targets []workUsageTarget) workUsageIndex {
	sort.SliceStable(targets, func(i, j int) bool {
		return len(targets[i].relDir) > len(targets[j].relDir)
	})
	index := workUsageIndex{
		targets:             targets,
		byDir:               map[string]int{},
		nestedPrefixByDir:   map[string][]int{},
		customPrefixIndexes: []int{},
	}
	for i, target := range targets {
		key, _, ok := novelFetcherWorkPathKey(target.relDir)
		if ok && key == target.relDir {
			index.byDir[key] = i
			continue
		}
		if ok && strings.HasPrefix(target.relDir, key+"/") {
			index.nestedPrefixByDir[key] = append(index.nestedPrefixByDir[key], i)
			continue
		}
		index.customPrefixIndexes = append(index.customPrefixIndexes, i)
	}
	return index
}

func matchNovelFetcherWork(rel string, index workUsageIndex) (*workUsageTarget, string, bool) {
	key, rest, ok := novelFetcherWorkPathKey(rel)
	if ok {
		if target, rest, found := matchPrefixTargets(rel, index, index.nestedPrefixByDir[key]); found {
			return target, rest, true
		}
		if targetIndex, found := index.byDir[key]; found {
			return &index.targets[targetIndex], rest, true
		}
	}
	if target, rest, found := matchPrefixTargets(rel, index, index.customPrefixIndexes); found {
		return target, rest, true
	}
	return nil, "", false
}

func matchPrefixTargets(rel string, index workUsageIndex, targetIndexes []int) (*workUsageTarget, string, bool) {
	for _, targetIndex := range targetIndexes {
		target := &index.targets[targetIndex]
		if rel == target.relDir {
			return target, "", true
		}
		prefix := target.relDir + "/"
		if strings.HasPrefix(rel, prefix) {
			return target, strings.TrimPrefix(rel, prefix), true
		}
	}
	return nil, "", false
}

func fallbackNovelFetcherWork(rel string) (*workUsageTarget, string, bool) {
	key, rest, ok := novelFetcherWorkPathKey(rel)
	if !ok || rest == "" {
		return nil, "", false
	}
	parts := strings.SplitN(strings.TrimPrefix(key, "novel-fetcher/works/"), "/", 2)
	target := &workUsageTarget{
		relDir:   key,
		novelID:  "unlisted:" + parts[0] + ":" + parts[1],
		title:    parts[1],
		siteName: parts[0],
		source:   "novel-fetcher",
	}
	return target, rest, true
}

func novelFetcherWorkPathKey(rel string) (string, string, bool) {
	const prefix = "novel-fetcher/works/"
	if !strings.HasPrefix(rel, prefix) {
		return "", "", false
	}
	rest := strings.TrimPrefix(rel, prefix)
	parts := strings.SplitN(rest, "/", 3)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	key := prefix + parts[0] + "/" + parts[1]
	if len(parts) == 2 {
		return key, "", true
	}
	return key, parts[2], true
}

func legacyNovelWork(rel string) (*workUsageTarget, string, bool) {
	const prefix = "小説データ/"
	if !strings.HasPrefix(rel, prefix) {
		return nil, "", false
	}
	rest := strings.TrimPrefix(rel, prefix)
	parts := strings.SplitN(rest, "/", 3)
	if len(parts) < 3 || parts[0] == "" || parts[1] == "" {
		return nil, "", false
	}
	title := legacyNovelTitle(parts[1])
	target := &workUsageTarget{
		relDir:   prefix + parts[0] + "/" + parts[1],
		novelID:  "legacy:" + parts[0] + ":" + parts[1],
		title:    title,
		siteName: parts[0],
		source:   "legacy",
	}
	return target, parts[2], true
}

func legacyNovelTitle(dirName string) string {
	first, rest, ok := strings.Cut(dirName, " ")
	if !ok || !looksLikeLegacyWorkID(first) {
		return dirName
	}
	trimmed := strings.TrimSpace(rest)
	if trimmed == "" {
		return dirName
	}
	return trimmed
}

func looksLikeLegacyWorkID(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	if len(value) < 2 || value[0] != 'n' {
		return false
	}
	index := 1
	for index < len(value) && value[index] >= '0' && value[index] <= '9' {
		index++
	}
	if index == 1 {
		return false
	}
	for index < len(value) {
		if value[index] < 'a' || value[index] > 'z' {
			return false
		}
		index++
	}
	return true
}

func classifyWorkFile(rest string) CategoryID {
	first := firstPathSegment(rest)
	switch first {
	case "raw":
		return CategoryCache
	case "episodes", "assets":
		return CategoryNovelData
	default:
		return CategoryNovelData
	}
}

func classifyLegacyWorkFile(rest string) CategoryID {
	switch firstPathSegment(rest) {
	case "raw":
		return CategoryCache
	default:
		return CategoryNovelData
	}
}

func isCachePath(rel string) bool {
	return rel == "state/reader_search.sqlite" ||
		strings.HasPrefix(rel, "state/reader_search.sqlite-") ||
		strings.HasPrefix(rel, "tmp/") ||
		strings.HasPrefix(rel, ".narou/")
}

func firstPathSegment(path string) string {
	path = strings.Trim(path, "/")
	if path == "" {
		return ""
	}
	first, _, _ := strings.Cut(path, "/")
	return first
}

func cleanRelPath(path string) string {
	cleaned := filepath.ToSlash(filepath.Clean(path))
	if cleaned == "." {
		return ""
	}
	return strings.TrimPrefix(cleaned, "./")
}

func displayOrFallback(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value != "" {
		return value
	}
	return strings.TrimSpace(fallback)
}

func newUsageAccumulator() *usageAccumulator {
	return &usageAccumulator{
		categories: map[CategoryID]*CategoryUsage{
			CategoryNovelData: {ID: CategoryNovelData, Label: "小説データ"},
			CategoryCache:     {ID: CategoryCache, Label: "キャッシュ"},
			CategoryOther:     {ID: CategoryOther, Label: "その他"},
		},
		novels: map[string]*NovelUsage{},
	}
}

func (a *usageAccumulator) addFile(rel string, size int64, classification fileClassification) {
	category := classification.category
	if category == "" {
		category = CategoryOther
	}
	entry := a.categories[category]
	entry.Bytes += size
	entry.FileCount++
	if classification.novel == nil {
		return
	}
	novel := a.ensureNovel(*classification.novel)
	novel.TotalBytes += size
	novel.FileCount++
	switch category {
	case CategoryNovelData:
		novel.NovelDataBytes += size
	case CategoryCache:
		novel.CacheBytes += size
	default:
		novel.OtherBytes += size
	}
}

func (a *usageAccumulator) ensureNovel(target workUsageTarget) *NovelUsage {
	novel, ok := a.novels[target.novelID]
	if ok {
		return novel
	}
	novel = &NovelUsage{
		NovelID:  target.novelID,
		Title:    displayOrFallback(target.title, target.novelID),
		Author:   target.author,
		SiteName: displayOrFallback(target.siteName, "不明"),
		Source:   displayOrFallback(target.source, "unknown"),
	}
	a.novels[target.novelID] = novel
	return novel
}

func (a *usageAccumulator) addWarning(message string) {
	message = strings.TrimSpace(message)
	if message == "" {
		return
	}
	a.warnings = append(a.warnings, message)
}

func (a *usageAccumulator) result() StorageUsage {
	categories := []CategoryUsage{
		*a.categories[CategoryNovelData],
		*a.categories[CategoryCache],
		*a.categories[CategoryOther],
	}
	novels := make([]NovelUsage, 0, len(a.novels))
	var total int64
	for _, category := range categories {
		total += category.Bytes
	}
	for _, novel := range a.novels {
		if novel.TotalBytes > 0 {
			novels = append(novels, *novel)
		}
	}
	sort.SliceStable(novels, func(i, j int) bool {
		if novels[i].TotalBytes != novels[j].TotalBytes {
			return novels[i].TotalBytes > novels[j].TotalBytes
		}
		return novels[i].Title < novels[j].Title
	})
	return StorageUsage{
		CheckedAt:  time.Now().UTC().Format(time.RFC3339),
		TotalBytes: total,
		Categories: categories,
		Novels:     novels,
		Warnings:   a.warnings,
	}
}
