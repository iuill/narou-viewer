package publications

type Kind string

const (
	KindNovel Kind = "novel"
	KindComic Kind = "comic"
)

type OverrideMode string

const (
	OverrideModeNone     OverrideMode = "none"
	OverrideModeISBN     OverrideMode = "isbn"
	OverrideModeDisabled OverrideMode = "disabled"
	OverrideModeVisible  OverrideMode = "visible"
)

type EntryStatus string

const (
	EntryStatusUnknown  EntryStatus = "unknown"
	EntryStatusManual   EntryStatus = "manual"
	EntryStatusDisabled EntryStatus = "disabled"
)

type Entry struct {
	ID             string       `json:"id" yaml:"id,omitempty"`
	Kind           Kind         `json:"kind" yaml:"kind"`
	Status         EntryStatus  `json:"status" yaml:"status"`
	Override       OverrideMode `json:"override" yaml:"override"`
	ISBN13         string       `json:"isbn13" yaml:"isbn13,omitempty"`
	Title          string       `json:"title" yaml:"title,omitempty"`
	Subtitle       string       `json:"subtitle" yaml:"subtitle,omitempty"`
	Authors        []string     `json:"authors" yaml:"authors,omitempty"`
	Publisher      string       `json:"publisher" yaml:"publisher,omitempty"`
	Published      string       `json:"publishedDate" yaml:"published_date,omitempty"`
	ImageURL       string       `json:"imageUrl" yaml:"image_url,omitempty"`
	DetailURL      string       `json:"detailUrl" yaml:"detail_url,omitempty"`
	Source         string       `json:"source" yaml:"source,omitempty"`
	SourceURL      string       `json:"sourceUrl" yaml:"source_url,omitempty"`
	CoverSource    string       `json:"coverSource,omitempty" yaml:"cover_source,omitempty"`
	CoverSourceURL string       `json:"coverSourceUrl,omitempty" yaml:"cover_source_url,omitempty"`
	CheckedAt      string       `json:"checkedAt" yaml:"checked_at,omitempty"`
	UpdatedAt      string       `json:"updatedAt" yaml:"updated_at"`
	ProviderID     string       `json:"providerId,omitempty" yaml:"provider_id,omitempty"`
	Warnings       []string     `json:"warnings,omitempty" yaml:"warnings,omitempty"`
}

type NovelPublications struct {
	NovelID             string  `json:"novelId" yaml:"novel_id"`
	DisplayCoverEntryID string  `json:"displayCoverEntryId,omitempty" yaml:"display_cover_entry_id,omitempty"`
	Entries             []Entry `json:"entries" yaml:"entries"`
}

type EntryInput struct {
	Kind   Kind         `json:"kind"`
	Mode   OverrideMode `json:"mode"`
	ISBN13 string       `json:"isbn13"`
}

type DisplayCoverInput struct {
	EntryID string `json:"entryId"`
}

type GoogleBooksVolume struct {
	VolumeID            string
	Title               string
	Subtitle            string
	Authors             []string
	Publisher           string
	PublishedDate       string
	ImageURL            string
	InfoLink            string
	CanonicalVolumeLink string
}

type NDLBibliography struct {
	Title         string
	Authors       []string
	Publisher     string
	PublishedDate string
	DetailURL     string
}
