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

type Job struct {
	JobID                     string  `json:"jobId"`
	RequestedUpToEpisodeIndex string  `json:"requestedUpToEpisodeIndex"`
	ProfileID                 *string `json:"profileId"`
	ProfileLabel              *string `json:"profileLabel"`
	GenerationMode            string  `json:"generationMode"`
	GenerationStrategy        string  `json:"generationStrategy"`
	ModelID                   *string `json:"modelId"`
	Status                    string  `json:"status"`
	Progress                  *int    `json:"progress,omitempty"`
	ProgressStage             *string `json:"progressStage,omitempty"`
	CurrentBatchIndex         *int    `json:"currentBatchIndex,omitempty"`
	BatchCount                *int    `json:"batchCount,omitempty"`
	GeneratedCharacterCount   *int    `json:"generatedCharacterCount,omitempty"`
	CreatedAt                 string  `json:"createdAt"`
	StartedAt                 *string `json:"startedAt"`
	FinishedAt                *string `json:"finishedAt"`
	ErrorMessage              *string `json:"errorMessage"`
}
