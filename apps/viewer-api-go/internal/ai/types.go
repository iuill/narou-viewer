package ai

import "time"

type SettingsResponse struct {
	APIBaseURLConfigured       bool             `json:"apiBaseUrlConfigured"`
	MasterPassphraseConfigured bool             `json:"masterPassphraseConfigured"`
	PreferredMode              string           `json:"preferredMode"`
	EffectiveGenerationMode    string           `json:"effectiveGenerationMode"`
	Settings                   SettingsMetadata `json:"settings"`
}

type SettingsMetadata struct {
	SelectedProfileID        *string                  `json:"selectedProfileId"`
	SharedProviders          SharedProviders          `json:"sharedProviders"`
	Profiles                 []Profile                `json:"profiles"`
	ExtractionStrategyModels ExtractionStrategyModels `json:"extractionStrategyModels"`
	ExtractionRuntime        ExtractionRuntime        `json:"extractionRuntime"`
}

type ExtractionStrategyModels struct {
	NameDiscoveryModelID *string `json:"nameDiscoveryModelId"`
}

type ExtractionRuntime struct {
	ParallelRequestConcurrency int `json:"parallelRequestConcurrency"`
}

type SharedProviders struct {
	OpenRouter  ProviderMetadata `json:"openrouter"`
	GoogleBooks ProviderMetadata `json:"googleBooks"`
}

type ProviderMetadata struct {
	HasAPIKey    bool    `json:"hasApiKey"`
	APIKeyMasked *string `json:"apiKeyMasked"`
	UpdatedAt    *string `json:"updatedAt"`
}

type Profile struct {
	ID                string             `json:"id"`
	Label             string             `json:"label"`
	Provider          string             `json:"provider"`
	Credentials       ProfileCredentials `json:"credentials"`
	ModelID           *string            `json:"modelId"`
	ModelInfo         *ModelInfoMetadata `json:"modelInfo,omitempty"`
	ProviderOrder     []string           `json:"providerOrder"`
	AllowFallbacks    bool               `json:"allowFallbacks"`
	RequireParameters bool               `json:"requireParameters"`
	UpdatedAt         *string            `json:"updatedAt"`
}

type ModelInfoMetadata struct {
	ContextLength       int    `json:"contextLength"`
	MaxCompletionTokens int    `json:"maxCompletionTokens"`
	Source              string `json:"source"`
}

type ProfileCredentials struct {
	Source       string  `json:"source"`
	HasAPIKey    bool    `json:"hasApiKey"`
	APIKeyMasked *string `json:"apiKeyMasked"`
	UpdatedAt    *string `json:"updatedAt"`
}

type UsageResponse struct {
	Summary UsageSummary `json:"summary"`
	Runs    []UsageRun   `json:"runs"`
}

type UsageSummary struct {
	RunCount              int     `json:"runCount"`
	RequestCount          int     `json:"requestCount"`
	InputTokens           int     `json:"inputTokens"`
	OutputTokens          int     `json:"outputTokens"`
	TotalTokens           int     `json:"totalTokens"`
	CachedInputTokens     int     `json:"cachedInputTokens"`
	ReasoningOutputTokens int     `json:"reasoningOutputTokens"`
	TotalCost             float64 `json:"totalCost"`
	AverageTotalTokens    float64 `json:"averageTotalTokens"`
}

type UsageRun struct {
	RunID                 string         `json:"runId"`
	Feature               string         `json:"feature"`
	WorkflowName          string         `json:"workflowName"`
	Status                string         `json:"status"`
	StartedAt             string         `json:"startedAt"`
	FinishedAt            string         `json:"finishedAt"`
	ElapsedMs             int            `json:"elapsedMs"`
	NovelID               *string        `json:"novelId"`
	NovelTitle            *string        `json:"novelTitle"`
	CurrentEpisodeIndex   *string        `json:"currentEpisodeIndex"`
	ModelID               *string        `json:"modelId"`
	ProfileID             *string        `json:"profileId"`
	ProfileLabel          *string        `json:"profileLabel"`
	GenerationMode        string         `json:"generationMode"`
	AnswerChars           int            `json:"answerChars"`
	RequestCount          int            `json:"requestCount"`
	InputTokens           int            `json:"inputTokens"`
	OutputTokens          int            `json:"outputTokens"`
	TotalTokens           int            `json:"totalTokens"`
	CachedInputTokens     int            `json:"cachedInputTokens"`
	ReasoningOutputTokens int            `json:"reasoningOutputTokens"`
	TotalCost             float64        `json:"totalCost"`
	ToolCallCount         int            `json:"toolCallCount"`
	ToolResultCount       int            `json:"toolResultCount"`
	HasSnapshot           bool           `json:"hasSnapshot"`
	ErrorMessage          *string        `json:"errorMessage"`
	Requests              []UsageRequest `json:"requests"`
	Snapshot              any            `json:"snapshot"`
}

type UsageRequest struct {
	RequestIndex          int      `json:"requestIndex"`
	Kind                  string   `json:"kind"`
	ParentRequestIndex    *int     `json:"parentRequestIndex"`
	ToolNames             []string `json:"toolNames"`
	ToolSummaries         []string `json:"toolSummaries"`
	InputTokens           int      `json:"inputTokens"`
	OutputTokens          int      `json:"outputTokens"`
	TotalTokens           int      `json:"totalTokens"`
	CachedInputTokens     int      `json:"cachedInputTokens"`
	ReasoningOutputTokens int      `json:"reasoningOutputTokens"`
	Cost                  float64  `json:"cost"`
}

type Job struct {
	JobID                     string            `json:"jobId"`
	NovelID                   string            `json:"novelId"`
	NovelTitle                *string           `json:"novelTitle"`
	NovelAuthor               *string           `json:"novelAuthor"`
	RequestedUpToEpisodeIndex string            `json:"requestedUpToEpisodeIndex"`
	ProfileID                 *string           `json:"profileId"`
	ProfileLabel              *string           `json:"profileLabel"`
	GenerationMode            string            `json:"generationMode"`
	GenerationStrategy        string            `json:"generationStrategy,omitempty"`
	ModelID                   *string           `json:"modelId"`
	Status                    string            `json:"status"`
	Progress                  *int              `json:"progress,omitempty"`
	ProgressStage             *string           `json:"progressStage,omitempty"`
	CurrentBatchIndex         *int              `json:"currentBatchIndex,omitempty"`
	BatchCount                *int              `json:"batchCount,omitempty"`
	CompletedBatchCount       *int              `json:"completedBatchCount,omitempty"`
	GeneratedCharacterCount   *int              `json:"generatedCharacterCount,omitempty"`
	GeneratedTermCount        *int              `json:"generatedTermCount,omitempty"`
	ActiveWorkers             []JobActiveWorker `json:"activeWorkers,omitempty"`
	CreatedAt                 string            `json:"createdAt"`
	StartedAt                 *string           `json:"startedAt"`
	FinishedAt                *string           `json:"finishedAt"`
	ErrorMessage              *string           `json:"errorMessage"`
}

type JobActiveWorker struct {
	WorkerIndex       int    `json:"workerIndex"`
	BatchIndex        int    `json:"batchIndex"`
	StartEpisodeIndex string `json:"startEpisodeIndex"`
	EndEpisodeIndex   string `json:"endEpisodeIndex"`
	Phase             string `json:"phase"`
}

func DefaultSettings(preferredMode string) SettingsResponse {
	profileID := "default"
	modelID := "openrouter/auto"
	return SettingsResponse{
		APIBaseURLConfigured:       false,
		MasterPassphraseConfigured: false,
		PreferredMode:              preferredMode,
		EffectiveGenerationMode:    effectiveMode(preferredMode),
		Settings: SettingsMetadata{
			SelectedProfileID: &profileID,
			SharedProviders: SharedProviders{
				OpenRouter: ProviderMetadata{
					HasAPIKey:    false,
					APIKeyMasked: nil,
					UpdatedAt:    nil,
				},
				GoogleBooks: ProviderMetadata{
					HasAPIKey:    false,
					APIKeyMasked: nil,
					UpdatedAt:    nil,
				},
			},
			Profiles: []Profile{
				{
					ID:       profileID,
					Label:    "Default",
					Provider: "openrouter",
					Credentials: ProfileCredentials{
						Source:       "shared",
						HasAPIKey:    false,
						APIKeyMasked: nil,
						UpdatedAt:    nil,
					},
					ModelID:           &modelID,
					ProviderOrder:     []string{},
					AllowFallbacks:    false,
					RequireParameters: true,
					UpdatedAt:         nil,
				},
			},
			ExtractionStrategyModels: ExtractionStrategyModels{},
			ExtractionRuntime:        ExtractionRuntime{ParallelRequestConcurrency: 3},
		},
	}
}

func EmptyUsage() UsageResponse {
	return UsageResponse{
		Summary: UsageSummary{},
		Runs:    []UsageRun{},
	}
}

func NowISO() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func effectiveMode(preferredMode string) string {
	if preferredMode == "heuristic" {
		return "heuristic"
	}
	return "disabled"
}
