package extraction

import (
	"context"

	"narou-viewer/apps/viewer-api-go/internal/ai"
	"narou-viewer/apps/viewer-api-go/internal/characters"
	core "narou-viewer/apps/viewer-api-go/internal/extraction"
	"narou-viewer/apps/viewer-api-go/internal/extraction/checkpointstore"
	"narou-viewer/apps/viewer-api-go/internal/store"
)

type RequestOptions struct {
	ProfileID          *string
	Transient          *store.AIGenerationTransientConfig
	ResolvedConfig     *store.ResolvedAIGenerationConfig
	GenerationMode     string
	GenerationStrategy string
	ProfileResolution  bool
	PreviewOnly        bool
	BatchProgressSink  func(BatchProgress)
	SummaryInputs      *Inputs
}

type BatchProgress struct {
	Phase                   string
	Batch                   core.Batch
	CompletedBatchCount     int
	ElapsedMs               int64
	GeneratedCharacterCount int
	MergedCharacterCount    int
}

type Inputs struct {
	Episodes []characters.HeuristicEpisode
	Batches  []core.Batch
}

type BatchResult struct {
	Delta core.Delta
	Usage ai.UsageRequest
}

type PromptPreview struct {
	SystemPrompt string               `json:"systemPrompt"`
	Batches      []PromptPreviewBatch `json:"batches"`
}

type PromptPreviewBatch struct {
	BatchIndex     int                  `json:"batchIndex"`
	BatchCount     int                  `json:"batchCount"`
	EpisodeIndexes []string             `json:"episodeIndexes"`
	ChunkCount     int                  `json:"chunkCount"`
	Chunks         []PromptPreviewChunk `json:"chunks"`
}

type PromptPreviewChunk struct {
	EpisodeIndex string  `json:"episodeIndex"`
	Title        string  `json:"title"`
	Chapter      *string `json:"chapter"`
	Subchapter   *string `json:"subchapter"`
	ChunkIndex   int     `json:"chunkIndex"`
	ChunkCount   int     `json:"chunkCount"`
	Text         string  `json:"text"`
}

type PreparedPreview struct {
	Inputs  Inputs
	Preview PromptPreview
}

type Result struct {
	NovelID                   string                 `json:"novelId"`
	NovelTitle                string                 `json:"novelTitle"`
	UpToEpisodeIndex          string                 `json:"upToEpisodeIndex"`
	ProcessedUpToEpisodeIndex *string                `json:"processedUpToEpisodeIndex"`
	ProfileID                 *string                `json:"profileId"`
	ProfileLabel              *string                `json:"profileLabel"`
	GenerationMode            string                 `json:"generationMode"`
	GenerationStrategy        string                 `json:"generationStrategy"`
	ModelID                   *string                `json:"modelId"`
	Characters                []characters.Character `json:"characters"`
}

type SettingsProvider interface {
	GetAIGenerationSettings() (ai.SettingsResponse, error)
}

type Generator interface {
	LockTarget(novelID string, upToEpisodeIndex string) func()
	GenerateAndSave(ctx context.Context, novelID string, upToEpisodeIndex string, resolvedOverride *store.ResolvedAIGenerationConfig, strategy string, progressSink func(BatchProgress)) error
	GeneratePreview(ctx context.Context, novelID string, upToEpisodeIndex string, resolvedOverride *store.ResolvedAIGenerationConfig, strategy string, progressSink func(BatchProgress), episodeIndexes []string, preloaded *Inputs) (characters.SummaryResponse, error)
}

type WorkflowPorts interface {
	LockTarget(novelID string, upToEpisodeIndex string) func()
	GetAIGenerationSettings() (ai.SettingsResponse, error)
	ResolveActiveAIGenerationConfig() (*store.ResolvedAIGenerationConfig, error)
	NovelTitle(ctx context.Context, novelID string) *string
	RecordUsage(run ai.UsageRun) error
	Limits() (int, int)
	LoadInputs(ctx context.Context, novelID string, upToEpisodeIndex string, maxChunkChars int, maxBatchChars int, afterEpisodeIndex string) (Inputs, error)
	LoadGenerationSeed(novelID string, upToEpisodeIndex string) ([]characters.GeneratedCharacter, *string, bool, error)
	LoadGeneratedCharactersBeforeEpisode(novelID string, episodeIndex string) ([]characters.GeneratedCharacter, *string, bool, error)
	ReprocessFromEpisode(ctx context.Context, novelID string, processedEpisodeIndex *string, requestedUpToEpisodeIndex string) (string, error)
	MaterializeGeneratedSummary(novelID string) error
	LoadPendingUnresolved(novelID string, reprocessFromEpisodeIndex string) ([]characters.GeneratedUnresolvedMention, error)
	RebatchInputs(ctx context.Context, inputs Inputs, config *store.ResolvedAIGenerationConfig, fallbackMaxBatchChars int) Inputs
	LoadIDAllocator(novelID string, seed []characters.GeneratedCharacter) (*characters.GeneratedCharacterIDAllocator, error)
	PlanRuntimeBatch(ctx context.Context, config *store.ResolvedAIGenerationConfig, novelID string, upToEpisodeIndex string, knownCharacters []characters.GeneratedCharacter, template core.Batch, chunks []core.Chunk, unresolvedMentions []characters.GeneratedUnresolvedMention) (core.Batch, []core.Chunk, error)
	GenerateBatch(ctx context.Context, config *store.ResolvedAIGenerationConfig, novelID string, upToEpisodeIndex string, knownCharacters []characters.GeneratedCharacter, batch core.Batch, unresolvedMentions []characters.GeneratedUnresolvedMention) (BatchResult, error)
	GenerateParallelIdentity(ctx context.Context, config *store.ResolvedAIGenerationConfig, novelID string, upToEpisodeIndex string, seed []characters.GeneratedCharacter, batches []core.Batch, progressSink func(BatchProgress), pendingUnresolved []characters.GeneratedUnresolvedMention) ([]characters.GeneratedCharacter, core.GenerationState, []ai.UsageRequest, error)
	GenerateDiscoveryParallelCorrection(ctx context.Context, config *store.ResolvedAIGenerationConfig, novelID string, upToEpisodeIndex string, seed []characters.GeneratedCharacter, batches []core.Batch, progressSink func(BatchProgress), pendingUnresolved []characters.GeneratedUnresolvedMention) ([]characters.GeneratedCharacter, core.GenerationState, []ai.UsageRequest, error)
	LoadCheckpoint(novelID string, upToEpisodeIndex string) (checkpointstore.Checkpoint, error)
	SaveCheckpoint(novelID string, upToEpisodeIndex string, checkpoint checkpointstore.Checkpoint) error
	DeleteCheckpoint(novelID string, upToEpisodeIndex string) error
	SaveGeneratedSummary(novelID string, upToEpisodeIndex string, generated []characters.GeneratedCharacter, episodes []characters.HeuristicEpisode, options characters.SaveGeneratedSummaryOptions) error
	BuildGeneratedPreview(novelID string, upToEpisodeIndex string, generated []characters.GeneratedCharacter, episodes []characters.HeuristicEpisode, episodeIndexes []string, options characters.SaveGeneratedSummaryOptions) (characters.SummaryResponse, error)
	SaveHeuristicSummary(novelID string, upToEpisodeIndex string, episodes []characters.HeuristicEpisode) error
	BuildHeuristicPreview(novelID string, upToEpisodeIndex string, episodes []characters.HeuristicEpisode, episodeIndexes []string) (characters.SummaryResponse, error)
	LoadRequiredPreview(novelID string, upToEpisodeIndex string, episodeIndexes []string) (characters.SummaryResponse, error)
	CheckpointExists(novelID string, upToEpisodeIndex string) bool
}
