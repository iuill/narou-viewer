package extraction

import (
	"context"
	"errors"
	"testing"

	"narou-viewer/apps/viewer-api-go/internal/characters"
	core "narou-viewer/apps/viewer-api-go/internal/extraction"
	"narou-viewer/apps/viewer-api-go/internal/extraction/checkpointstore"
	"narou-viewer/apps/viewer-api-go/internal/store"
)

func runnerBatches(indexes ...string) []core.Batch {
	batches := make([]core.Batch, 0, len(indexes))
	for index, episodeIndex := range indexes {
		batches = append(batches, core.Batch{
			BatchIndex:     index + 1,
			BatchCount:     len(indexes),
			EpisodeIndexes: []string{episodeIndex},
			Chunks:         []core.Chunk{{EpisodeIndex: episodeIndex, Title: "第" + episodeIndex + "話", Text: "本文" + episodeIndex}},
		})
	}
	return batches
}

func TestGenerationRunnerResumesOnlyUnprocessedBatchesFromCheckpoint(t *testing.T) {
	config := &store.ResolvedAIGenerationConfig{ProfileID: "profile-a", ModelID: "model-a"}
	batches := runnerBatches("1", "2")
	ports := &workflowFakePorts{
		checkpoint: checkpointstore.Checkpoint{
			SchemaVersion:           1,
			NovelID:                 "novel-a",
			UpToEpisodeIndex:        "2",
			GenerationFingerprint:   CheckpointFingerprint(config, CheckpointBatchInputs(batches)),
			ProcessedEpisodeIndexes: []string{"1"},
			ProcessedBatchIndexes:   []int{1},
			Characters:              []characters.GeneratedCharacter{{CharacterID: "char_a", CanonicalName: "既存", CanonicalEpisodeIndex: "1"}},
		},
	}

	generated, _, usageRequests, err := NewWorkflow(ports).RunOpenRouterWithCheckpoint(context.Background(), config, "novel-a", "2", nil, batches, nil, nil)
	if err != nil {
		t.Fatalf("RunOpenRouterWithCheckpoint returned error: %v", err)
	}
	if ports.generateCalls != 1 || len(usageRequests) != 1 {
		t.Fatalf("runner should call provider only for remaining batch: calls=%d usage=%+v", ports.generateCalls, usageRequests)
	}
	if len(generated) != 2 || !generatedContainsName(generated, "既存") {
		t.Fatalf("runner should keep checkpoint snapshot and append generated delta: %+v", generated)
	}
	if !ports.savedCheckpoint || len(ports.checkpoint.ProcessedBatchIndexes) != 2 {
		t.Fatalf("runner should update checkpoint after remaining batch: %+v", ports.checkpoint)
	}
}

func generatedContainsName(values []characters.GeneratedCharacter, name string) bool {
	for _, value := range values {
		if value.CanonicalName == name {
			return true
		}
	}
	return false
}

func TestGenerationRunnerIgnoresMismatchedCheckpoint(t *testing.T) {
	config := &store.ResolvedAIGenerationConfig{ProfileID: "profile-a", ModelID: "model-a"}
	batches := runnerBatches("1")
	ports := &workflowFakePorts{
		checkpoint: checkpointstore.Checkpoint{
			SchemaVersion:           1,
			NovelID:                 "novel-a",
			UpToEpisodeIndex:        "1",
			GenerationFingerprint:   "stale",
			ProcessedEpisodeIndexes: []string{"1"},
			ProcessedBatchIndexes:   []int{1},
			Characters:              []characters.GeneratedCharacter{{CanonicalName: "古い"}},
		},
	}

	generated, _, usageRequests, err := NewWorkflow(ports).RunOpenRouterWithCheckpoint(context.Background(), config, "novel-a", "1", nil, batches, nil, nil)
	if err != nil {
		t.Fatalf("RunOpenRouterWithCheckpoint returned error: %v", err)
	}
	if ports.generateCalls != 1 || len(usageRequests) != 1 {
		t.Fatalf("mismatched checkpoint should be ignored and regenerated: calls=%d usage=%+v", ports.generateCalls, usageRequests)
	}
	if len(generated) != 1 || generated[0].CanonicalName == "古い" {
		t.Fatalf("mismatched checkpoint snapshot should not be reused: %+v", generated)
	}
}

func TestGenerationRunnerKeepsCompletedCheckpointWhenLaterBatchFails(t *testing.T) {
	config := &store.ResolvedAIGenerationConfig{ProfileID: "profile-a", ModelID: "model-a"}
	ports := &workflowFakePorts{
		generateErr:      errors.New("provider failed"),
		generateErrAfter: 1,
	}

	_, _, usageRequests, err := NewWorkflow(ports).RunOpenRouterWithCheckpoint(context.Background(), config, "novel-a", "2", nil, runnerBatches("1", "2"), nil, nil)
	if err == nil {
		t.Fatal("RunOpenRouterWithCheckpoint should return provider error")
	}
	if len(usageRequests) != 1 {
		t.Fatalf("runner should preserve completed batch usage on failure: %+v", usageRequests)
	}
	if !ports.savedCheckpoint || len(ports.checkpoint.ProcessedBatchIndexes) != 1 || ports.checkpoint.ProcessedBatchIndexes[0] != 1 {
		t.Fatalf("runner should keep checkpoint through last completed batch: %+v", ports.checkpoint)
	}
}

func TestGenerationRunnerPreviewDoesNotSaveCheckpointAndEmitsProgress(t *testing.T) {
	config := &store.ResolvedAIGenerationConfig{ProfileID: "profile-a", ModelID: "model-a"}
	ports := &workflowFakePorts{}
	progress := []BatchProgress{}

	_, _, usageRequests, err := NewWorkflow(ports).RunOpenRouterPreview(context.Background(), config, "novel-a", "1", nil, runnerBatches("1"), func(value BatchProgress) {
		progress = append(progress, value)
	}, nil)
	if err != nil {
		t.Fatalf("RunOpenRouterPreview returned error: %v", err)
	}
	if ports.savedCheckpoint {
		t.Fatalf("preview runner should not save checkpoint: %+v", ports.checkpoint)
	}
	if len(usageRequests) != 1 {
		t.Fatalf("preview runner should collect usage: %+v", usageRequests)
	}
	if len(progress) != 2 || progress[0].Phase != "start" || progress[1].Phase != "complete" {
		t.Fatalf("progress should be start then complete: %+v", progress)
	}
}

func TestGenerationRunnerReturnsAllocatorAndPlanningErrors(t *testing.T) {
	config := &store.ResolvedAIGenerationConfig{ProfileID: "profile-a", ModelID: "model-a"}
	if _, _, _, err := NewWorkflow(&workflowFakePorts{allocatorErr: errors.New("allocator failed")}).RunOpenRouterWithCheckpoint(context.Background(), config, "novel-a", "1", nil, runnerBatches("1"), nil, nil); err == nil {
		t.Fatal("checkpoint runner should return allocator errors")
	}
	if _, _, _, err := NewWorkflow(&workflowFakePorts{planErr: errors.New("planning failed")}).RunOpenRouterPreview(context.Background(), config, "novel-a", "1", nil, runnerBatches("1"), nil, nil); err == nil {
		t.Fatal("preview runner should return planning errors")
	}
}

func TestGenerationRunnerSmallHelpers(t *testing.T) {
	if allEpisodeIndexesProcessed(nil, []string{"1"}) {
		t.Fatal("empty episode indexes should not be treated as processed")
	}
	if allEpisodeIndexesProcessed([]string{"1"}, nil) {
		t.Fatal("empty processed indexes should not cover a batch")
	}
	if allEpisodeIndexesProcessed([]string{"1", "2"}, []string{"1"}) {
		t.Fatal("partial processed indexes should not cover a batch")
	}
	if !allEpisodeIndexesProcessed([]string{"1", "2"}, []string{"2", "1"}) {
		t.Fatal("processed indexes should cover all batch episodes independent of order")
	}
	if values := appendUniqueInt([]int{1}, 1); len(values) != 1 {
		t.Fatalf("appendUniqueInt should not append duplicates: %+v", values)
	}
	if values := mergeStringSets([]string{"1"}, []string{"", "1", "2"}); len(values) != 2 || values[1] != "2" {
		t.Fatalf("mergeStringSets should skip blanks and duplicates: %+v", values)
	}
	checkpoint := NormalizeCheckpoint(checkpointstore.Checkpoint{})
	if checkpoint.Characters == nil || checkpoint.PendingUnresolvedMentions == nil || checkpoint.IssuedCharacterIDs == nil || checkpoint.RetiredCharacterIDs == nil {
		t.Fatalf("NormalizeCheckpoint should initialize slices: %+v", checkpoint)
	}
	if fingerprint := CheckpointFingerprint(&store.ResolvedAIGenerationConfig{SystemPrompt: testString("prompt")}, func() {}); fingerprint == "" {
		t.Fatal("CheckpointFingerprint should fall back to a stable hash for unmarshalable extra")
	}
}

func TestWorkflowNilRunnerEntryPointsReturnEmptyResults(t *testing.T) {
	if generated, state, usage, err := (*Workflow)(nil).RunOpenRouterWithCheckpoint(context.Background(), nil, "novel-a", "1", nil, nil, nil, nil); err != nil || generated != nil || len(state.UnresolvedMentions) != 0 || usage != nil {
		t.Fatalf("nil checkpoint runner result = generated:%+v state:%+v usage:%+v err:%v", generated, state, usage, err)
	}
	if generated, state, usage, err := (*Workflow)(nil).RunOpenRouterPreview(context.Background(), nil, "novel-a", "1", nil, nil, nil, nil); err != nil || generated != nil || len(state.UnresolvedMentions) != 0 || usage != nil {
		t.Fatalf("nil preview runner result = generated:%+v state:%+v usage:%+v err:%v", generated, state, usage, err)
	}
}

func testString(value string) *string {
	return &value
}
