package extraction

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
	CompletedBatchCount       *int    `json:"completedBatchCount,omitempty"`
	GeneratedCharacterCount   *int    `json:"generatedCharacterCount,omitempty"`
	GeneratedTermCount        *int    `json:"generatedTermCount,omitempty"`
	CreatedAt                 string  `json:"createdAt"`
	StartedAt                 *string `json:"startedAt"`
	FinishedAt                *string `json:"finishedAt"`
	ErrorMessage              *string `json:"errorMessage"`
}

type jobDocument struct {
	SchemaVersion             int     `yaml:"schema_version"`
	Revision                  int     `yaml:"revision"`
	JobID                     string  `yaml:"job_id"`
	NovelID                   string  `yaml:"novel_id"`
	RequestedUpToEpisodeIndex string  `yaml:"requested_up_to_episode_index"`
	ProfileID                 *string `yaml:"profile_id"`
	ProfileLabel              *string `yaml:"profile_label"`
	GenerationMode            string  `yaml:"generation_mode"`
	GenerationStrategy        string  `yaml:"generation_strategy,omitempty"`
	ModelID                   *string `yaml:"model_id"`
	Status                    string  `yaml:"status"`
	Progress                  *int    `yaml:"progress,omitempty"`
	ProgressStage             *string `yaml:"progress_stage,omitempty"`
	CurrentBatchIndex         *int    `yaml:"current_batch_index,omitempty"`
	BatchCount                *int    `yaml:"batch_count,omitempty"`
	CompletedBatchCount       *int    `yaml:"completed_batch_count,omitempty"`
	GeneratedCharacterCount   *int    `yaml:"generated_character_count,omitempty"`
	GeneratedTermCount        *int    `yaml:"generated_term_count,omitempty"`
	CreatedAt                 string  `yaml:"created_at"`
	StartedAt                 *string `yaml:"started_at"`
	FinishedAt                *string `yaml:"finished_at"`
	ErrorMessage              *string `yaml:"error_message"`
}

type jobsIndexDocument struct {
	SchemaVersion int      `yaml:"schema_version"`
	Revision      int      `yaml:"revision"`
	NovelID       string   `yaml:"novel_id"`
	ActiveJobID   *string  `yaml:"active_job_id"`
	JobIDs        []string `yaml:"job_ids"`
}

type JobWithNovel struct {
	NovelID string
	Job     Job
}

type NovelStatePruneResult struct {
	ProfileDeleted     bool
	EventsDeleted      bool
	TermProfileDeleted bool
	JobsDeleted        int
	JobIndexDeleted    bool
	CheckpointsDeleted int
}
