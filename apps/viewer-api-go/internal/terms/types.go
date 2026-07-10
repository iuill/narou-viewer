package terms

const (
	CategoryOrganization = "organization"
	CategoryPlace        = "place"
	CategoryItem         = "item"
	CategorySkill        = "skill"
	CategoryRace         = "race"
	CategoryEvent        = "event"
	CategoryOther        = "other"
)

type TextVersion struct {
	Text         string `json:"text" yaml:"text"`
	EpisodeIndex string `json:"episodeIndex" yaml:"episode_index"`
}

type CategoryVersion struct {
	Category     string `json:"category" yaml:"category"`
	EpisodeIndex string `json:"episodeIndex" yaml:"episode_index"`
}

type HistoryVersion struct {
	Text         string `json:"text" yaml:"text"`
	EpisodeIndex string `json:"episodeIndex" yaml:"episode_index"`
}

type GeneratedTerm struct {
	Term               string            `json:"term" yaml:"term"`
	ReadingHistory     []TextVersion     `json:"readingHistory" yaml:"reading_history"`
	CategoryHistory    []CategoryVersion `json:"categoryHistory" yaml:"category_history"`
	DescriptionHistory []HistoryVersion  `json:"descriptionHistory" yaml:"description_history"`
}

type Term struct {
	Term        string  `json:"term"`
	Reading     *string `json:"reading"`
	Category    string  `json:"category"`
	Description string  `json:"description"`
}

type TermsResponse struct {
	Status                    string  `json:"status"`
	NovelID                   string  `json:"novelId"`
	UpToEpisodeIndex          string  `json:"upToEpisodeIndex"`
	ProcessedUpToEpisodeIndex *string `json:"processedUpToEpisodeIndex"`
	Terms                     []Term  `json:"terms"`
}

func NormalizeCategory(category string) string {
	switch category {
	case CategoryOrganization, CategoryPlace, CategoryItem, CategorySkill, CategoryRace, CategoryEvent, CategoryOther:
		return category
	default:
		return CategoryOther
	}
}
