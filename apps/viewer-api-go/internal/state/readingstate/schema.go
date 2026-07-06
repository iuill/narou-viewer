package readingstate

const (
	SchemaVersion = 3
	FileName      = "reading_state.yaml"

	timestampLayout = "2006-01-02T15:04:05.000Z07:00"
)

type ScrollState struct {
	Type  string  `json:"type" yaml:"type"`
	Value float64 `json:"value" yaml:"value"`
}

type State struct {
	NovelID              string       `json:"novelId"`
	LastReadEpisodeIndex *string      `json:"lastReadEpisodeIndex"`
	Position             int          `json:"position"`
	Scroll               *ScrollState `json:"scroll"`
	UpdatedAt            *string      `json:"updatedAt"`
	StateVersion         int          `json:"stateVersion"`
	UpdatedByClientID    *string      `json:"updatedByClientId"`
}

type PutInput struct {
	State
	ExpectedStateVersion *int
}

type document struct {
	SchemaVersion int               `yaml:"schema_version"`
	Revision      int               `yaml:"revision"`
	Novels        map[string]record `yaml:"novels"`
}

type record struct {
	LastReadEpisodeIndex *string      `yaml:"last_read_episode_index"`
	Position             int          `yaml:"position,omitempty"`
	LineNumber           int          `yaml:"line_number,omitempty"`
	Scroll               *ScrollState `yaml:"scroll,omitempty"`
	UpdatedAt            *string      `yaml:"updated_at"`
	StateVersion         int          `yaml:"state_version,omitempty"`
	UpdatedByClientID    *string      `yaml:"updated_by_client_id,omitempty"`
	Deleted              bool         `yaml:"deleted,omitempty"`
}

func emptyDocument() document {
	return document{SchemaVersion: SchemaVersion, Revision: 0, Novels: map[string]record{}}
}
