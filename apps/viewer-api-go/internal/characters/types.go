package characters

type SummaryResponse struct {
	Status                    string      `json:"status"`
	NovelID                   string      `json:"novelId"`
	UpToEpisodeIndex          string      `json:"upToEpisodeIndex"`
	ProcessedUpToEpisodeIndex *string     `json:"processedUpToEpisodeIndex"`
	Characters                []Character `json:"characters"`
}

type Character struct {
	CharacterID                 string      `json:"characterId"`
	CanonicalName               string      `json:"canonicalName"`
	FullName                    *string     `json:"fullName"`
	Gender                      *string     `json:"gender"`
	FirstAppearanceEpisodeIndex string      `json:"firstAppearanceEpisodeIndex"`
	Aliases                     []string    `json:"aliases"`
	Appearance                  *string     `json:"appearance"`
	Personality                 *string     `json:"personality"`
	Summary                     *string     `json:"summary"`
	Importance                  interface{} `json:"importance"`
}
