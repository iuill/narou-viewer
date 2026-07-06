package model

import "time"

type Site string

const (
	SiteSyosetu      Site = "syosetu"
	SiteKakuyomu     Site = "kakuyomu"
	SiteVerification Site = "verification"
)

type Work struct {
	ID         int
	Site       Site
	SiteName   string
	SiteWorkID string
	SourceURL  string
	Title      string
	Author     string
	Story      string
	Tags       []string
	Episodes   []Episode
	FetchedAt  time.Time
}

type Episode struct {
	Index        string
	Href         string
	SourceURL    string
	Chapter      string
	Subchapter   string
	Title        string
	FileSubtitle string
	PublishedAt  string
	ModifiedAt   string
	Element      EpisodeElement
	RawHTML      string
	FetchedAt    time.Time
}

type EpisodeElement struct {
	DataType     string
	Introduction string
	Body         string
	Postscript   string
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

type CanonicalEpisode struct {
	SchemaVersion int         `json:"schema_version"`
	EpisodeID     string      `json:"episode_id"`
	SiteEpisodeID string      `json:"site_episode_id"`
	SourceURL     string      `json:"source_url,omitempty"`
	SortOrder     int         `json:"sort_order"`
	DisplayIndex  string      `json:"display_index"`
	Title         string      `json:"title"`
	Chapter       string      `json:"chapter,omitempty"`
	Subchapter    string      `json:"subchapter,omitempty"`
	PublishedAt   string      `json:"published_at,omitempty"`
	UpdatedAt     string      `json:"updated_at,omitempty"`
	Blocks        []BodyBlock `json:"blocks"`
	FetchedAt     time.Time   `json:"fetched_at"`
}

type StoredWork struct {
	ID                  int
	Site                Site
	SiteName            string
	SiteWorkID          string
	SourceURL           string
	Title               string
	Author              string
	Story               string
	Directory           string
	FetchedAt           time.Time
	EpisodeLen          int
	SavedEpisodeLen     int
	FetchStatus         string
	LastFetchError      string
	LastFailedEpisodeID string
	ResumeEpisodeID     string
	ExpectedEpisodeLen  int
}

type StoredEpisode struct {
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
	FetchedAt      time.Time
	BodyStatus     string
	LastFetchError string
}
