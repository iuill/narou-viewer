package library

import "encoding/json"

type NovelSummary struct {
	NovelID                    string  `json:"novelId"`
	FetcherWorkID              string  `json:"fetcherWorkId"`
	Title                      string  `json:"title"`
	Author                     string  `json:"author"`
	SiteName                   string  `json:"siteName"`
	TocURL                     *string `json:"tocUrl"`
	Story                      string  `json:"story"`
	UpdatedAt                  *string `json:"updatedAt"`
	LastActivityAt             *string `json:"lastActivityAt"`
	LastReadEpisodeIndex       *string `json:"lastReadEpisodeIndex"`
	LastReadEpisodeTitle       *string `json:"lastReadEpisodeTitle"`
	LatestBookmarkEpisodeIndex *string `json:"latestBookmarkEpisodeIndex"`
	BookmarkCount              int     `json:"bookmarkCount"`
	TotalEpisodes              int     `json:"totalEpisodes"`
	SavedEpisodes              int     `json:"savedEpisodes,omitempty"`
	FetchStatus                string  `json:"fetchStatus,omitempty"`
	PublicationCoverImageURL   string  `json:"publicationCoverImageUrl,omitempty"`
	PublicationCoverKind       string  `json:"publicationCoverKind,omitempty"`
	PublicationCoverSource     string  `json:"publicationCoverSource,omitempty"`
	PublicationCoverSourceURL  string  `json:"publicationCoverSourceUrl,omitempty"`
	LastFetchError             *string `json:"lastFetchError,omitempty"`
	FailedEpisodeID            *string `json:"failedEpisodeId,omitempty"`
	ResumeEpisodeID            *string `json:"resumeEpisodeId,omitempty"`
}

type NovelListResult struct {
	Novels []NovelSummary `json:"novels"`
}

type TocEpisodeSummary struct {
	EpisodeIndex   string  `json:"episodeIndex"`
	Title          string  `json:"title"`
	Chapter        *string `json:"chapter"`
	Subchapter     *string `json:"subchapter"`
	SourceURL      *string `json:"sourceUrl"`
	UpdatedAt      *string `json:"updatedAt"`
	ContentEtag    string  `json:"contentEtag"`
	BodyStatus     string  `json:"bodyStatus,omitempty"`
	LastFetchError *string `json:"lastFetchError,omitempty"`
}

type TocResponse struct {
	NovelSummary
	Story    string              `json:"story"`
	Episodes []TocEpisodeSummary `json:"episodes"`
}

type EpisodeResponse struct {
	NovelID         string         `json:"novelId"`
	EpisodeIndex    string         `json:"episodeIndex"`
	Title           string         `json:"title"`
	Chapter         *string        `json:"chapter"`
	Subchapter      *string        `json:"subchapter"`
	SourceURL       *string        `json:"sourceUrl"`
	HTML            string         `json:"html"`
	ReaderDocument  ReaderDocument `json:"readerDocument"`
	PlainTextLength int            `json:"plainTextLength"`
	UpdatedAt       *string        `json:"updatedAt"`
	ContentEtag     string         `json:"contentEtag"`
}

type ReaderDocument struct {
	Version int           `json:"version"`
	Blocks  []ReaderBlock `json:"blocks"`
}

type ReaderBlock struct {
	Type        string         `json:"type"`
	Role        string         `json:"role,omitempty"`
	Text        string         `json:"text,omitempty"`
	Section     string         `json:"section,omitempty"`
	Inlines     []ReaderInline `json:"inlines,omitempty"`
	Src         string         `json:"src,omitempty"`
	Alt         *string        `json:"alt,omitempty"`
	OriginalURL *string        `json:"originalUrl,omitempty"`
	Title       *string        `json:"title,omitempty"`
	Width       *int           `json:"width,omitempty"`
	Height      *int           `json:"height,omitempty"`
	HTML        string         `json:"html,omitempty"`
	PlainText   string         `json:"plainText,omitempty"`
}

type ReaderInline struct {
	Type     string         `json:"type"`
	Text     string         `json:"text,omitempty"`
	Ruby     string         `json:"ruby,omitempty"`
	Href     *string        `json:"href,omitempty"`
	Children []ReaderInline `json:"children,omitempty"`
}

// MarshalJSON は TS viewer-api の ReaderBlock union と同じ JSON 形状
// （type ごとの固定 field 集合、image の null field 明示など）を出力する。
// frontend は block.type で分岐して field を直接参照するため、
// omitempty による field 欠落や余剰 field を避ける。
func (b ReaderBlock) MarshalJSON() ([]byte, error) {
	switch b.Type {
	case "meta":
		return json.Marshal(struct {
			Type string `json:"type"`
			Role string `json:"role"`
			Text string `json:"text"`
		}{b.Type, b.Role, b.Text})
	case "title":
		return json.Marshal(struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}{b.Type, b.Text})
	case "paragraph":
		inlines := b.Inlines
		if inlines == nil {
			inlines = []ReaderInline{}
		}
		return json.Marshal(struct {
			Type    string         `json:"type"`
			Section string         `json:"section"`
			Inlines []ReaderInline `json:"inlines"`
		}{b.Type, b.Section, inlines})
	case "image":
		return json.Marshal(struct {
			Type        string  `json:"type"`
			Section     string  `json:"section"`
			Src         string  `json:"src"`
			Alt         *string `json:"alt"`
			OriginalURL *string `json:"originalUrl"`
			Title       *string `json:"title"`
			Width       *int    `json:"width"`
			Height      *int    `json:"height"`
		}{b.Type, b.Section, b.Src, b.Alt, b.OriginalURL, b.Title, b.Width, b.Height})
	case "html":
		return json.Marshal(struct {
			Type      string `json:"type"`
			Section   string `json:"section"`
			HTML      string `json:"html"`
			PlainText string `json:"plainText"`
		}{b.Type, b.Section, b.HTML, b.PlainText})
	default:
		type readerBlockAlias ReaderBlock
		return json.Marshal(readerBlockAlias(b))
	}
}

// MarshalJSON は TS viewer-api の ReaderInlineToken union と同じ JSON 形状を出力する。
func (i ReaderInline) MarshalJSON() ([]byte, error) {
	switch i.Type {
	case "text":
		return json.Marshal(struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}{i.Type, i.Text})
	case "ruby":
		return json.Marshal(struct {
			Type string `json:"type"`
			Text string `json:"text"`
			Ruby string `json:"ruby"`
		}{i.Type, i.Text, i.Ruby})
	case "lineBreak":
		return json.Marshal(struct {
			Type string `json:"type"`
		}{i.Type})
	case "tcy":
		return json.Marshal(struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}{i.Type, i.Text})
	case "link":
		children := i.Children
		if children == nil {
			children = []ReaderInline{}
		}
		return json.Marshal(struct {
			Type     string         `json:"type"`
			Href     *string        `json:"href"`
			Children []ReaderInline `json:"children"`
		}{i.Type, i.Href, children})
	default:
		type readerInlineAlias ReaderInline
		return json.Marshal(readerInlineAlias(i))
	}
}

type RuntimeServiceStatus string

const (
	RuntimeStatusOK    RuntimeServiceStatus = "ok"
	RuntimeStatusWarn  RuntimeServiceStatus = "warn"
	RuntimeStatusError RuntimeServiceStatus = "error"
)

type RuntimeStatusService struct {
	ID      string               `json:"id"`
	Label   string               `json:"label"`
	Status  RuntimeServiceStatus `json:"status"`
	Summary string               `json:"summary"`
	Detail  string               `json:"detail"`
}

type RuntimeStatusResponse struct {
	Status    RuntimeServiceStatus   `json:"status"`
	CheckedAt string                 `json:"checkedAt"`
	Services  []RuntimeStatusService `json:"services"`
}

type AssetResponse struct {
	FilePath  string
	MediaType string
}
