package characters

type profilesDocument struct {
	SchemaVersion             int                  `yaml:"schema_version"`
	NovelID                   string               `yaml:"novel_id"`
	ProcessedUpToEpisodeIndex *string              `yaml:"processed_up_to_episode_index"`
	IdentityMergeEvents       []identityMergeEvent `yaml:"identity_merge_events,omitempty"`
	Characters                []characterProfile   `yaml:"characters"`
}

type characterEventsDocument struct {
	SchemaVersion             int                    `yaml:"schema_version"`
	NovelID                   string                 `yaml:"novel_id"`
	ProcessedUpToEpisodeIndex *string                `yaml:"processed_up_to_episode_index"`
	NextCharacterOrdinal      int                    `yaml:"next_character_ordinal"`
	RetiredCharacterIDs       []retiredCharacterID   `yaml:"retired_character_ids,omitempty"`
	IdentityMergeEvents       []identityMergeEvent   `yaml:"identity_merge_events,omitempty"`
	UnresolvedMentions        []unresolvedMention    `yaml:"unresolved_mentions,omitempty"`
	EpisodeEtags              []episodeEtag          `yaml:"episode_etags,omitempty"`
	Characters                []characterEventRecord `yaml:"characters"`
}

type retiredCharacterID struct {
	CharacterID string `yaml:"character_id"`
	MergedInto  string `yaml:"merged_into,omitempty"`
}

type identityMergeEvent struct {
	SourceCharacterID     string `yaml:"source_character_id"`
	TargetCharacterID     string `yaml:"target_character_id"`
	EffectiveEpisodeIndex string `yaml:"effective_episode_index"`
}

type unresolvedMention struct {
	Mention      string   `yaml:"mention"`
	EpisodeIndex string   `yaml:"episode_index"`
	Reason       string   `yaml:"reason,omitempty"`
	CandidateIDs []string `yaml:"candidate_ids,omitempty"`
}

type episodeEtag struct {
	EpisodeIndex string `yaml:"episode_index"`
	ContentEtag  string `yaml:"content_etag"`
}

type characterEventRecord struct {
	CharacterID                 string             `yaml:"character_id"`
	CanonicalName               textVersion        `yaml:"canonical_name"`
	PreferredNames              []textVersion      `yaml:"preferred_names,omitempty"`
	FullName                    *textVersion       `yaml:"full_name"`
	FullNameHistory             []textVersion      `yaml:"full_name_history,omitempty"`
	Gender                      *textVersion       `yaml:"gender"`
	GenderHistory               []textVersion      `yaml:"gender_history,omitempty"`
	FirstAppearanceEpisodeIndex string             `yaml:"first_appearance_episode_index"`
	Aliases                     []textVersion      `yaml:"aliases"`
	Facts                       []characterFact    `yaml:"facts"`
	Mentions                    []characterMention `yaml:"mentions"`
}

type characterFact struct {
	Kind         string `yaml:"kind"`
	Text         string `yaml:"text"`
	EpisodeIndex string `yaml:"episode_index"`
}

type characterMention struct {
	Text         string `yaml:"text"`
	EpisodeIndex string `yaml:"episode_index"`
	Count        int    `yaml:"count,omitempty"`
}

type characterProfile struct {
	CharacterID                 string                `yaml:"character_id"`
	CanonicalName               textVersion           `yaml:"canonical_name"`
	PreferredNames              []textVersion         `yaml:"preferred_names,omitempty"`
	FullName                    *textVersion          `yaml:"full_name"`
	FullNameHistory             []textVersion         `yaml:"full_name_history,omitempty"`
	Gender                      *textVersion          `yaml:"gender"`
	GenderHistory               []textVersion         `yaml:"gender_history,omitempty"`
	FirstAppearanceEpisodeIndex string                `yaml:"first_appearance_episode_index"`
	Aliases                     []textVersion         `yaml:"aliases"`
	AppearanceHistory           []historyVersion      `yaml:"appearance_history"`
	PersonalityHistory          []historyVersion      `yaml:"personality_history"`
	SummaryHistory              []historyVersion      `yaml:"summary_history"`
	ImportanceMetrics           *importanceMetricsDoc `yaml:"importance_metrics"`
}

type textVersion struct {
	Text         string `yaml:"text"`
	EpisodeIndex string `yaml:"episode_index"`
}

type historyVersion struct {
	Text         string `yaml:"text"`
	EpisodeIndex string `yaml:"episode_index"`
}

type importanceMetricsDoc struct {
	EpisodeMentions []episodeMentionDoc `yaml:"episode_mentions"`
}

type episodeMentionDoc struct {
	EpisodeIndex string `yaml:"episode_index"`
	Count        int    `yaml:"count"`
}

type HeuristicEpisode struct {
	EpisodeIndex string
	Text         string
	ContentEtag  string
}

type GeneratedCharacter struct {
	CharacterID                 string
	CanonicalName               string
	CanonicalEpisodeIndex       string
	NameHistory                 []GeneratedTextVersion
	FullName                    *string
	FullNameEpisodeIndex        string
	FullNameHistory             []GeneratedTextVersion
	Gender                      *string
	GenderEpisodeIndex          string
	GenderHistory               []GeneratedTextVersion
	FirstAppearanceEpisodeIndex string
	Aliases                     []GeneratedTextVersion
	AppearanceHistory           []GeneratedHistoryVersion
	PersonalityHistory          []GeneratedHistoryVersion
	SummaryHistory              []GeneratedHistoryVersion
	Appearance                  *string
	Personality                 *string
	Summary                     *string
}

type GeneratedUnresolvedMention struct {
	Mention      string
	EpisodeIndex string
	Reason       string
	CandidateIDs []string
}

type GeneratedRetiredCharacterID struct {
	CharacterID string
	MergedInto  string
}

type GeneratedIdentityMergeEvent struct {
	SourceCharacterID     string
	TargetCharacterID     string
	EffectiveEpisodeIndex string
}

type GeneratedEpisodeDigest struct {
	EpisodeIndex string
	ContentEtag  string
}

type SaveGeneratedSummaryOptions struct {
	ReplaceFromEpisodeIndex string
	UnresolvedMentions      []GeneratedUnresolvedMention
	SetUnresolvedMentions   bool
	IssuedCharacterIDs      []string
	RetiredCharacterIDs     []GeneratedRetiredCharacterID
	IdentityMergeEvents     []GeneratedIdentityMergeEvent
	NextCharacterOrdinal    int
}

type GeneratedCharacterIDAllocator struct {
	novelID     string
	nextOrdinal int
	usedIDs     map[string]bool
	issuedIDs   map[string]bool
	retiredIDs  map[string]string
}

type GeneratedTextVersion struct {
	Text         string `json:"text"`
	EpisodeIndex string `json:"episodeIndex"`
}

type GeneratedHistoryVersion struct {
	EpisodeIndex string `json:"episodeIndex"`
	Text         string `json:"text"`
}
