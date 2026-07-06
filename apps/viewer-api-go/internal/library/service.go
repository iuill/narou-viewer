package library

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"html"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"narou-viewer/apps/viewer-api-go/internal/fetcher"

	_ "modernc.org/sqlite"
)

type Service struct {
	rootDir       string
	fetcherReader fetcherLibraryReader
	mu            sync.Mutex
	db            *sql.DB
}

type Work struct {
	ID                  int
	Site                string
	SiteName            string
	SiteWorkID          string
	SourceURL           string
	Title               string
	Author              string
	Story               string
	Directory           string
	FetchedAt           string
	EpisodeLen          int
	SavedEpisodeLen     int
	FetchStatus         string
	LastFetchError      string
	LastFailedEpisodeID string
	ResumeEpisodeID     string
	ExpectedEpisodeLen  int
}

type Episode struct {
	WorkID         int
	EpisodeID      string
	SiteEpisodeID  string
	SourceURL      string
	SortOrder      int
	DisplayIndex   string
	Title          string
	Chapter        string
	Subchapter     string
	PublishedAt    string
	UpdatedAt      string
	BodyPath       string
	RawPath        string
	ContentHash    string
	FetchedAt      string
	BodyStatus     string
	LastFetchError string
}

type CanonicalEpisode struct {
	SchemaVersion int         `json:"schema_version"`
	EpisodeID     string      `json:"episode_id"`
	SiteEpisodeID string      `json:"site_episode_id"`
	SourceURL     string      `json:"source_url"`
	SortOrder     int         `json:"sort_order"`
	DisplayIndex  string      `json:"display_index"`
	Title         string      `json:"title"`
	Chapter       string      `json:"chapter"`
	Subchapter    string      `json:"subchapter"`
	PublishedAt   string      `json:"published_at"`
	UpdatedAt     string      `json:"updated_at"`
	Blocks        []BodyBlock `json:"blocks"`
	FetchedAt     time.Time   `json:"fetched_at"`
}

type BodyBlock struct {
	Type     string       `json:"type"`
	Section  string       `json:"section,omitempty"`
	HTML     string       `json:"html,omitempty"`
	Text     string       `json:"text,omitempty"`
	Src      string       `json:"src,omitempty"`
	Alt      string       `json:"alt,omitempty"`
	Width    int          `json:"width,omitempty"`
	Height   int          `json:"height,omitempty"`
	Children []BodyInline `json:"children,omitempty"`
}

type BodyInline struct {
	Type     string       `json:"type"`
	Text     string       `json:"text,omitempty"`
	Ruby     string       `json:"ruby,omitempty"`
	Href     string       `json:"href,omitempty"`
	Src      string       `json:"src,omitempty"`
	Alt      string       `json:"alt,omitempty"`
	Width    int          `json:"width,omitempty"`
	Height   int          `json:"height,omitempty"`
	Children []BodyInline `json:"children,omitempty"`
}

type EpisodeDocument struct {
	Episode       Episode
	Document      CanonicalEpisode
	HTML          string
	Plain         string
	HTMLSections  map[string]string
	PlainSections map[string]string
	Etag          string
}

func Open(rootDir string) (*Service, error) {
	db, err := openDB(rootDir)
	if err != nil {
		return nil, err
	}
	return &Service{rootDir: rootDir, db: db}, nil
}

func openDB(rootDir string) (*sql.DB, error) {
	dbPath := filepath.Join(rootDir, "library.sqlite")
	if _, err := os.Stat(dbPath); err != nil {
		return nil, err
	}
	return sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath)+"?mode=ro")
}

func NewService(rootDir string) *Service {
	service, err := Open(rootDir)
	if err != nil {
		return &Service{rootDir: rootDir}
	}
	return service
}

type fetcherLibraryReader interface {
	ListLibraryWorks(context.Context) ([]fetcher.LibraryWork, error)
	GetLibraryToc(context.Context, int) (fetcher.LibraryWork, []fetcher.LibraryEpisode, error)
	GetLibraryEpisode(context.Context, int, string) (fetcher.LibraryEpisodeResponse, error)
}

type fetcherLibraryTocBatchReader interface {
	ListLibraryTocs(context.Context, []int) (map[int][]fetcher.LibraryEpisode, error)
}

func NewServiceWithFetcher(rootDir string, reader fetcherLibraryReader) *Service {
	return &Service{rootDir: rootDir, fetcherReader: reader}
}

func (s *Service) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return nil
	}
	db := s.db
	s.db = nil
	return db.Close()
}

func (s *Service) ensureDB() (*sql.DB, error) {
	if s == nil {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db != nil {
		return s.db, nil
	}
	db, err := openDB(s.rootDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	s.db = db
	return db, nil
}

func (s *Service) ListNovels(ctx context.Context) (NovelListResult, error) {
	works, err := s.listWorks(ctx)
	if err != nil {
		return NovelListResult{}, err
	}
	novels := make([]NovelSummary, 0, len(works))
	for _, work := range works {
		novels = append(novels, work.toSummary())
	}
	sort.SliceStable(novels, func(i, j int) bool {
		leftUpdatedAt := derefString(novels[i].UpdatedAt)
		rightUpdatedAt := derefString(novels[j].UpdatedAt)
		if leftUpdatedAt != rightUpdatedAt {
			return leftUpdatedAt > rightUpdatedAt
		}
		return novels[i].Title < novels[j].Title
	})
	return NovelListResult{Novels: novels}, nil
}

func (s *Service) GetToc(ctx context.Context, novelID string) (*TocResponse, error) {
	work, ok, err := s.findWork(ctx, novelID)
	if err != nil || !ok {
		return nil, err
	}
	episodes, err := s.listEpisodes(ctx, work.ID)
	if err != nil {
		return nil, err
	}
	summary := work.toSummary()
	response := &TocResponse{
		NovelSummary: summary,
		Story:        work.Story,
		Episodes:     make([]TocEpisodeSummary, 0, len(episodes)),
	}
	for _, episode := range episodes {
		response.Episodes = append(response.Episodes, episode.toSummary())
	}
	if response.TotalEpisodes == 0 {
		response.TotalEpisodes = len(response.Episodes)
	}
	return response, nil
}

func (s *Service) GetTocsByFetcherWorkIDs(ctx context.Context, fetcherWorkIDs []string) (map[string][]TocEpisodeSummary, error) {
	workIDs := normalizeFetcherWorkIDs(fetcherWorkIDs)
	result := make(map[string][]TocEpisodeSummary, len(workIDs))
	if len(workIDs) == 0 {
		return result, nil
	}
	if s != nil && s.fetcherReader != nil {
		if batchReader, ok := s.fetcherReader.(fetcherLibraryTocBatchReader); ok {
			episodesByWorkID, err := batchReader.ListLibraryTocs(ctx, workIDs)
			if err != nil {
				return nil, err
			}
			for _, workID := range workIDs {
				episodes, ok := episodesByWorkID[workID]
				if !ok {
					continue
				}
				result[strconv.Itoa(workID)] = tocSummariesFromEpisodes(episodesFromFetcher(workID, episodes))
			}
			return result, nil
		}
	}
	for _, workID := range workIDs {
		episodes, err := s.listEpisodes(ctx, workID)
		if err != nil {
			return nil, err
		}
		result[strconv.Itoa(workID)] = tocSummariesFromEpisodes(episodes)
	}
	return result, nil
}

func normalizeFetcherWorkIDs(values []string) []int {
	result := []int{}
	seen := map[int]struct{}{}
	for _, value := range values {
		workID, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || workID <= 0 {
			continue
		}
		if _, ok := seen[workID]; ok {
			continue
		}
		seen[workID] = struct{}{}
		result = append(result, workID)
	}
	return result
}

func episodesFromFetcher(workID int, values []fetcher.LibraryEpisode) []Episode {
	episodes := make([]Episode, 0, len(values))
	for _, episode := range values {
		episodes = append(episodes, episodeFromFetcher(workID, episode))
	}
	return episodes
}

func tocSummariesFromEpisodes(episodes []Episode) []TocEpisodeSummary {
	summaries := make([]TocEpisodeSummary, 0, len(episodes))
	for _, episode := range episodes {
		summaries = append(summaries, episode.toSummary())
	}
	return summaries
}

func (s *Service) GetEpisode(ctx context.Context, novelID string, episodeIndex string) (*EpisodeResponse, error) {
	if s != nil && s.fetcherReader != nil {
		return s.getFetcherEpisode(ctx, novelID, episodeIndex)
	}
	work, ok, err := s.FindWork(novelID)
	if err != nil || !ok {
		return nil, err
	}
	episode, ok, err := s.FindEpisode(work.ID, episodeIndex)
	if err != nil || !ok || episode.BodyStatus != "complete" || strings.TrimSpace(episode.BodyPath) == "" {
		return nil, err
	}
	document, err := s.ReadEpisodeDocument(episode)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	canonical := document.Document
	title := firstNonEmpty(canonical.Title, episode.Title, "Episode "+episodeIndex)
	chapter := firstNonEmpty(canonical.Chapter, episode.Chapter)
	subchapter := firstNonEmpty(canonical.Subchapter, episode.Subchapter)
	updatedAt := firstNonEmpty(episode.UpdatedAt, canonical.UpdatedAt, canonical.FetchedAt.Format(time.RFC3339Nano), episode.FetchedAt, time.Unix(0, 0).UTC().Format(time.RFC3339))
	contentEtag := firstNonEmpty(episode.ContentHash, strings.Trim(document.Etag, `"`))
	htmlSections := rewriteSectionAssetRefs(document.HTMLSections, novelID)
	bodyHTML := buildEpisodeHTML(chapter, subchapter, title, htmlSections)

	return &EpisodeResponse{
		NovelID:         novelID,
		EpisodeIndex:    episode.DisplayIndex,
		Title:           title,
		Chapter:         stringPtr(chapter),
		Subchapter:      stringPtr(subchapter),
		SourceURL:       stringPtr(normalizeExternalURL(firstNonEmpty(episode.SourceURL, canonical.SourceURL))),
		HTML:            bodyHTML,
		ReaderDocument:  readerDocument(chapter, subchapter, title, htmlSections),
		PlainTextLength: len([]rune(document.Plain)),
		UpdatedAt:       stringPtr(updatedAt),
		ContentEtag:     contentEtag,
	}, nil
}

func (s *Service) getFetcherEpisode(ctx context.Context, novelID string, episodeIndex string) (*EpisodeResponse, error) {
	work, ok, err := s.findWork(ctx, novelID)
	if err != nil || !ok {
		return nil, err
	}
	episode, ok, err := s.findEpisode(ctx, work.ID, episodeIndex)
	if err != nil || !ok || episode.BodyStatus != "complete" {
		return nil, err
	}
	payload, err := s.fetcherReader.GetLibraryEpisode(ctx, work.ID, episode.EpisodeID)
	if err != nil {
		if isFetcherNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	var canonical CanonicalEpisode
	if err := json.Unmarshal(payload.Canonical, &canonical); err != nil {
		return nil, err
	}
	document := episodeDocumentFromCanonical(episode, canonical, payload.Canonical)
	title := firstNonEmpty(canonical.Title, episode.Title, "Episode "+episodeIndex)
	chapter := firstNonEmpty(canonical.Chapter, episode.Chapter)
	subchapter := firstNonEmpty(canonical.Subchapter, episode.Subchapter)
	updatedAt := firstNonEmpty(episode.UpdatedAt, canonical.UpdatedAt, canonical.FetchedAt.Format(time.RFC3339Nano), episode.FetchedAt, time.Unix(0, 0).UTC().Format(time.RFC3339))
	contentEtag := firstNonEmpty(episode.ContentHash, strings.Trim(document.Etag, `"`))
	htmlSections := rewriteSectionAssetRefs(document.HTMLSections, novelID)
	bodyHTML := buildEpisodeHTML(chapter, subchapter, title, htmlSections)

	return &EpisodeResponse{
		NovelID:         novelID,
		EpisodeIndex:    episode.DisplayIndex,
		Title:           title,
		Chapter:         stringPtr(chapter),
		Subchapter:      stringPtr(subchapter),
		SourceURL:       stringPtr(normalizeExternalURL(firstNonEmpty(episode.SourceURL, canonical.SourceURL))),
		HTML:            bodyHTML,
		ReaderDocument:  readerDocument(chapter, subchapter, title, htmlSections),
		PlainTextLength: len([]rune(document.Plain)),
		UpdatedAt:       stringPtr(updatedAt),
		ContentEtag:     contentEtag,
	}, nil
}

func episodeDocumentFromCanonical(episode Episode, canonical CanonicalEpisode, raw []byte) EpisodeDocument {
	contentBlocks := episodeContentBlocksBySection(canonical.Blocks)
	htmlSections := mapSections(contentBlocks, blocksToHTML)
	plainSections := mapSections(contentBlocks, blocksToText)
	sum := sha256.Sum256(raw)
	return EpisodeDocument{
		Episode:       episode,
		Document:      canonical,
		HTML:          joinSections(htmlSections),
		Plain:         joinSections(plainSections),
		HTMLSections:  htmlSections,
		PlainSections: plainSections,
		Etag:          `"` + hex.EncodeToString(sum[:]) + `"`,
	}
}

func (s *Service) RuntimeStatus(ctx context.Context) RuntimeStatusResponse {
	result, err := s.ListNovels(ctx)
	libraryStatus := RuntimeStatusService{
		ID:      "library",
		Label:   "ローカルライブラリ",
		Status:  RuntimeStatusOK,
		Summary: strconv.Itoa(len(result.Novels)) + " 作品",
		Detail:  "novel-fetcher のライブラリから作品データを " + strconv.Itoa(len(result.Novels)) + " 件検出しました。",
	}
	if err != nil {
		libraryStatus.Status = RuntimeStatusError
		libraryStatus.Summary = "未接続"
		libraryStatus.Detail = "novel-fetcher のローカルライブラリを読み取れませんでした。"
	} else if len(result.Novels) == 0 {
		libraryStatus.Status = RuntimeStatusWarn
		libraryStatus.Summary = "作品なし"
		libraryStatus.Detail = "novel-fetcher のライブラリにはまだ作品がありません。保存後に一覧へ反映されます。"
	}
	services := []RuntimeStatusService{
		{
			ID:      "viewer-api",
			Label:   "viewer-api-go",
			Status:  RuntimeStatusOK,
			Summary: "応答中",
			Detail:  "Go viewer-api は起動しており、ステータス API に応答しています。",
		},
		libraryStatus,
	}
	status := RuntimeStatusOK
	for _, service := range services {
		if service.Status == RuntimeStatusError {
			status = RuntimeStatusError
			break
		}
		if service.Status == RuntimeStatusWarn {
			status = RuntimeStatusWarn
		}
	}
	return RuntimeStatusResponse{
		Status:    status,
		CheckedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Services:  services,
	}
}

func blocksToHTML(blocks []BodyBlock) string {
	var builder strings.Builder
	for _, block := range blocks {
		switch block.Type {
		case "html":
			builder.WriteString(block.HTML)
		case "image":
			builder.WriteString(renderImage(block))
		default:
			text := block.Text
			if text == "" {
				text = inlinesToText(block.Children)
			}
			builder.WriteString("<p>")
			builder.WriteString(strings.ReplaceAll(html.EscapeString(text), "\n", "<br>"))
			builder.WriteString("</p>")
		}
	}
	return builder.String()
}

func blocksToText(blocks []BodyBlock) string {
	lines := []string{}
	for _, block := range blocks {
		text := block.Text
		if text == "" {
			text = inlinesToText(block.Children)
		}
		if text == "" && block.HTML != "" {
			text = htmlToText(block.HTML)
		}
		if text != "" {
			lines = append(lines, text)
		}
	}
	return strings.Join(lines, "\n")
}

func inlinesToText(inlines []BodyInline) string {
	var builder strings.Builder
	for _, inline := range inlines {
		if inline.Text != "" {
			builder.WriteString(inline.Text)
		}
		if len(inline.Children) > 0 {
			builder.WriteString(inlinesToText(inline.Children))
		}
	}
	return builder.String()
}

func htmlToText(value string) string {
	replacer := strings.NewReplacer("<br>", "\n", "<br/>", "\n", "<br />", "\n", "</p>", "\n", "</div>", "\n", "</section>", "\n")
	text := replacer.Replace(value)
	var builder strings.Builder
	inTag := false
	for _, r := range text {
		switch r {
		case '<':
			inTag = true
		case '>':
			inTag = false
		default:
			if !inTag {
				builder.WriteRune(r)
			}
		}
	}
	lines := []string{}
	for _, line := range strings.Split(html.UnescapeString(builder.String()), "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			lines = append(lines, trimmed)
		}
	}
	return strings.Join(lines, "\n")
}

func episodeContentBlocksBySection(blocks []BodyBlock) map[string][]BodyBlock {
	content := map[string][]BodyBlock{
		"introduction": {},
		"body":         {},
		"postscript":   {},
	}
	for _, block := range blocks {
		if block.Section == "introduction" || block.Section == "body" || block.Section == "postscript" {
			content[block.Section] = append(content[block.Section], block)
		}
	}
	return content
}

func mapSections(blocks map[string][]BodyBlock, mapper func([]BodyBlock) string) map[string]string {
	result := map[string]string{}
	for _, section := range []string{"introduction", "body", "postscript"} {
		if value := mapper(blocks[section]); strings.TrimSpace(value) != "" {
			result[section] = value
		}
	}
	return result
}

func joinSections(sections map[string]string) string {
	parts := []string{}
	for _, section := range []string{"introduction", "body", "postscript"} {
		if value := sections[section]; strings.TrimSpace(value) != "" {
			parts = append(parts, value)
		}
	}
	return strings.Join(parts, "\n")
}

func (w Work) toSummary() NovelSummary {
	totalEpisodes := w.ExpectedEpisodeLen
	if totalEpisodes == 0 {
		totalEpisodes = w.EpisodeLen
	}
	return NovelSummary{
		NovelID:         NovelID(w),
		FetcherWorkID:   strconv.Itoa(w.ID),
		Title:           firstNonEmpty(w.Title, "Work "+strconv.Itoa(w.ID)),
		Author:          w.Author,
		SiteName:        firstNonEmpty(w.SiteName, "novel-fetcher"),
		TocURL:          stringPtr(normalizeTocURL(w.SourceURL)),
		Story:           w.Story,
		UpdatedAt:       stringPtr(firstNonEmpty(w.FetchedAt, time.Unix(0, 0).UTC().Format(time.RFC3339))),
		LastActivityAt:  stringPtr(firstNonEmpty(w.FetchedAt, time.Unix(0, 0).UTC().Format(time.RFC3339))),
		TotalEpisodes:   totalEpisodes,
		SavedEpisodes:   w.SavedEpisodeLen,
		FetchStatus:     firstNonEmpty(w.FetchStatus, "complete"),
		LastFetchError:  stringPtr(w.LastFetchError),
		FailedEpisodeID: stringPtr(w.LastFailedEpisodeID),
		ResumeEpisodeID: stringPtr(w.ResumeEpisodeID),
	}
}

func (e Episode) toSummary() TocEpisodeSummary {
	updatedAt := firstNonEmpty(e.UpdatedAt, e.PublishedAt, e.FetchedAt)
	return TocEpisodeSummary{
		EpisodeIndex:   e.DisplayIndex,
		Title:          firstNonEmpty(e.Title, "Episode "+e.DisplayIndex),
		Chapter:        stringPtr(e.Chapter),
		Subchapter:     stringPtr(e.Subchapter),
		SourceURL:      stringPtr(normalizeExternalURL(e.SourceURL)),
		UpdatedAt:      stringPtr(updatedAt),
		ContentEtag:    e.ContentHash,
		BodyStatus:     e.BodyStatus,
		LastFetchError: stringPtr(e.LastFetchError),
	}
}

func buildEpisodeHTML(chapter string, subchapter string, title string, sections map[string]string) string {
	parts := []string{}
	if chapter != "" {
		parts = append(parts, `<p class="reader-meta">`+html.EscapeString(chapter)+`</p>`)
	}
	if subchapter != "" {
		parts = append(parts, `<p class="reader-meta">`+html.EscapeString(subchapter)+`</p>`)
	}
	parts = append(parts, `<h1 class="reader-title">`+html.EscapeString(title)+`</h1>`)
	for _, section := range []string{"introduction", "body", "postscript"} {
		if sectionHTML := sections[section]; strings.TrimSpace(sectionHTML) != "" {
			parts = append(parts, `<section class="reader-section reader-section-`+section+`">`+sectionHTML+`</section>`)
		}
	}
	return strings.Join(parts, "")
}

func rewriteSectionAssetRefs(sections map[string]string, novelID string) map[string]string {
	result := map[string]string{}
	for section, sectionHTML := range sections {
		result[section] = rewriteAssetRefs(sectionHTML, novelID)
	}
	return result
}

func normalizeTocURL(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if !strings.HasSuffix(trimmed, "/") {
		trimmed += "/"
	}
	return trimmed
}

func normalizeExternalURL(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return trimmed
	}
	if parsed.Host == "kakuyomu.jp" && len(parsed.Path) > 1 {
		parsed.Path = strings.TrimRight(parsed.Path, "/")
		return parsed.String()
	}
	return trimmed
}

func normalizeAssetPath(assetPath string) string {
	var segments []string
	for _, segment := range regexp.MustCompile(`[\\/]+`).Split(assetPath, -1) {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			continue
		}
		if segment == "." || segment == ".." {
			return ""
		}
		segments = append(segments, segment)
	}
	return strings.Join(segments, "/")
}

func renderImage(block BodyBlock) string {
	if strings.TrimSpace(block.Src) == "" {
		return ""
	}
	attrs := []string{`src="` + html.EscapeString(block.Src) + `"`}
	if block.Alt != "" {
		attrs = append(attrs, `alt="`+html.EscapeString(block.Alt)+`"`)
	}
	if block.Width > 0 {
		attrs = append(attrs, `width="`+strconv.Itoa(block.Width)+`"`)
	}
	if block.Height > 0 {
		attrs = append(attrs, `height="`+strconv.Itoa(block.Height)+`"`)
	}
	return "<p><img " + strings.Join(attrs, " ") + "></p>"
}

var imageSourcePattern = regexp.MustCompile(`(?i)(<img\b[^>]*\bsrc\s*=\s*["'])([^"']+)(["'][^>]*>)`)

func rewriteAssetRefs(source string, novelID string) string {
	return imageSourcePattern.ReplaceAllStringFunc(source, func(match string) string {
		parts := imageSourcePattern.FindStringSubmatch(match)
		if len(parts) != 4 {
			return match
		}
		return parts[1] + rewriteAssetURL(parts[2], novelID) + parts[3]
	})
}

func rewriteAssetURL(source string, novelID string) string {
	trimmed := strings.TrimSpace(source)
	assetPath := ""
	if strings.HasPrefix(trimmed, "../assets/") {
		assetPath = strings.TrimPrefix(trimmed, "../")
	} else if strings.HasPrefix(trimmed, "assets/") {
		assetPath = trimmed
	}
	if assetPath == "" {
		return source
	}
	return "/api/library/novels/" + url.PathEscape(novelID) + "/assets/" + encodeAssetPath(assetPath)
}

func encodeAssetPath(assetPath string) string {
	segments := strings.Split(assetPath, "/")
	for i, segment := range segments {
		segments[i] = url.PathEscape(segment)
	}
	return strings.Join(segments, "/")
}

func stringPtr(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
