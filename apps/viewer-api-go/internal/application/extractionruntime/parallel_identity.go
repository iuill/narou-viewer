package extractionruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"narou-viewer/apps/viewer-api-go/internal/ai"
	appextraction "narou-viewer/apps/viewer-api-go/internal/application/extraction"
	"narou-viewer/apps/viewer-api-go/internal/characters"
	core "narou-viewer/apps/viewer-api-go/internal/extraction"
	"narou-viewer/apps/viewer-api-go/internal/store"
	"narou-viewer/apps/viewer-api-go/internal/terms"
)

const (
	defaultParallelIdentityLLMConcurrency     = 3
	maxParallelIdentityLLMConcurrency         = 20
	defaultParallelIdentityLLMStartIntervalMS = 250
	defaultParallelIdentityMaxReduceItems     = 240
	defaultParallelIdentityMaxReduceTokens    = 60000
)

var errParallelIdentityOneShotTooLarge = errors.New("parallel_identity target is too large")

type parallelIdentityCandidate struct {
	LocalID    string
	Source     string
	BatchIndex int
	Character  characters.GeneratedCharacter
}

type parallelIdentityCluster struct {
	LocalIDs      []string `json:"localIds"`
	CanonicalName string   `json:"canonicalName"`
	Confidence    float64  `json:"confidence"`
	Reason        string   `json:"reason"`
}

func (r *Runtime) GenerateParallelIdentity(ctx context.Context, config *store.ResolvedAIGenerationConfig, novelID string, upToEpisodeIndex string, seed []characters.GeneratedCharacter, seedTerms []terms.GeneratedTerm, batches []extractionBatch, progressSink func(appextraction.BatchProgress), pendingUnresolved []characters.GeneratedUnresolvedMention) ([]characters.GeneratedCharacter, extractionGenerationState, []ai.UsageRequest, error) {
	startedAt := time.Now()
	generated, state, requests, err := r.generateOpenRouterExtractionParallelIdentity(ctx, config, novelID, upToEpisodeIndex, seed, seedTerms, batches, progressSink, pendingUnresolved)
	status := "ok"
	if err != nil {
		status = "error"
	}
	r.log("parallel_identity", startedAt, "status", status, "novelId", novelID, "upToEpisodeIndex", upToEpisodeIndex, "batchCount", len(batches), "characterCount", len(generated), "requestCount", len(requests))
	return generated, state, requests, err
}

func (r *Runtime) GenerateDiscoveryParallelCorrection(ctx context.Context, config *store.ResolvedAIGenerationConfig, novelID string, upToEpisodeIndex string, seed []characters.GeneratedCharacter, seedTerms []terms.GeneratedTerm, batches []extractionBatch, progressSink func(appextraction.BatchProgress), pendingUnresolved []characters.GeneratedUnresolvedMention) ([]characters.GeneratedCharacter, extractionGenerationState, []ai.UsageRequest, error) {
	startedAt := time.Now()
	generated, state, requests, err := r.generateOpenRouterExtractionDiscoveryParallelCorrection(ctx, config, novelID, upToEpisodeIndex, seed, seedTerms, batches, progressSink, pendingUnresolved)
	status := "ok"
	if err != nil {
		status = "error"
	}
	r.log("discovery_parallel_correction", startedAt, "status", status, "novelId", novelID, "upToEpisodeIndex", upToEpisodeIndex, "batchCount", len(batches), "characterCount", len(generated), "requestCount", len(requests))
	return generated, state, requests, err
}

func (r *Runtime) generateOpenRouterExtractionParallelIdentity(ctx context.Context, config *store.ResolvedAIGenerationConfig, novelID string, upToEpisodeIndex string, seed []characters.GeneratedCharacter, seedTerms []terms.GeneratedTerm, batches []extractionBatch, progressSink func(appextraction.BatchProgress), initialUnresolved []characters.GeneratedUnresolvedMention) ([]characters.GeneratedCharacter, extractionGenerationState, []ai.UsageRequest, error) {
	if config == nil {
		return nil, extractionGenerationState{}, nil, errors.New("AI generation profile was not found.")
	}
	allocator, err := characters.LoadGeneratedCharacterIDAllocator(r.stateDir, novelID, seed)
	if err != nil {
		return nil, extractionGenerationState{}, nil, err
	}
	runtimeBatches, err := r.parallelIdentityRuntimeBatches(ctx, config, novelID, upToEpisodeIndex, nil, seedTerms, batches, initialUnresolved)
	if err != nil {
		return nil, extractionStateFromAllocator(initialUnresolved, allocator), nil, err
	}
	extracted, rawTerms, usageRequests, unresolved, err := r.extractParallelIdentityCandidates(ctx, config, novelID, upToEpisodeIndex, seedTerms, runtimeBatches, progressSink, initialUnresolved)
	if err != nil {
		return nil, extractionStateFromAllocator(initialUnresolved, allocator), usageRequests, err
	}
	candidates := seedParallelIdentityCandidates(seed)
	candidates = append(candidates, extracted...)
	clusters, identityUsage, err := r.resolveParallelIdentityClusters(ctx, config, novelID, upToEpisodeIndex, candidates)
	if err != nil {
		return nil, extractionStateFromAllocator(unresolved, allocator), usageRequests, err
	}
	if identityUsage.Kind != "" {
		identityUsage.RequestIndex = len(usageRequests)
		usageRequests = append(usageRequests, identityUsage)
	}
	generated := buildParallelIdentityGeneratedCharacters(candidates, clusters, allocator)
	generated = mergeGeneratedCharacters(nil, generated)
	generated = allocator.Assign(generated)
	sortGeneratedCharacters(generated)
	unresolved = filterResolvedGeneratedUnresolvedMentions(unresolved, generated)
	state := extractionStateFromAllocator(unresolved, allocator)
	state.Terms = core.FilterAndMergeParallelTermFacts(seedTerms, rawTerms, generated)
	return generated, state, usageRequests, nil
}

func (r *Runtime) generateOpenRouterExtractionDiscoveryParallelCorrection(ctx context.Context, config *store.ResolvedAIGenerationConfig, novelID string, upToEpisodeIndex string, seed []characters.GeneratedCharacter, seedTerms []terms.GeneratedTerm, batches []extractionBatch, progressSink func(appextraction.BatchProgress), initialUnresolved []characters.GeneratedUnresolvedMention) ([]characters.GeneratedCharacter, extractionGenerationState, []ai.UsageRequest, error) {
	if config == nil {
		return nil, extractionGenerationState{}, nil, errors.New("AI generation profile was not found.")
	}
	allocator, err := characters.LoadGeneratedCharacterIDAllocator(r.stateDir, novelID, seed)
	if err != nil {
		return nil, extractionGenerationState{}, nil, err
	}
	discoveryBatches, err := r.parallelIdentityRuntimeBatches(ctx, config, novelID, upToEpisodeIndex, nil, seedTerms, batches, initialUnresolved)
	if err != nil {
		return nil, extractionStateFromAllocator(initialUnresolved, allocator), nil, err
	}
	discovered, discoveryUsage, err := r.discoverParallelIdentityNames(ctx, extractionNameDiscoveryConfig(config), novelID, upToEpisodeIndex, discoveryBatches)
	if err != nil {
		return nil, extractionStateFromAllocator(initialUnresolved, allocator), nil, err
	}
	discovered = allocator.Assign(discovered)
	knownCharacters := append([]characters.GeneratedCharacter{}, seed...)
	knownCharacters = append(knownCharacters, discovered...)
	detailBatches, err := r.parallelIdentityRuntimeBatches(ctx, config, novelID, upToEpisodeIndex, knownCharacters, seedTerms, batches, initialUnresolved)
	if err != nil {
		return nil, extractionStateFromAllocator(initialUnresolved, allocator), discoveryUsage, err
	}
	extracted, rawTerms, usageRequests, unresolved, err := r.extractParallelIdentityCandidatesWithKnown(ctx, config, novelID, upToEpisodeIndex, knownCharacters, seedTerms, detailBatches, progressSink, initialUnresolved)
	usageRequests = append(discoveryUsage, usageRequests...)
	for index := range usageRequests {
		usageRequests[index].RequestIndex = index
	}
	if err != nil {
		return nil, extractionStateFromAllocator(initialUnresolved, allocator), usageRequests, err
	}
	candidates := seedParallelIdentityCandidates(seed)
	candidates = append(candidates, discoveryParallelIdentityCandidates(discovered)...)
	candidates = append(candidates, extracted...)
	clusters, identityUsage, err := r.resolveParallelIdentityClusters(ctx, config, novelID, upToEpisodeIndex, candidates)
	if err != nil {
		return nil, extractionStateFromAllocator(unresolved, allocator), usageRequests, err
	}
	if identityUsage.Kind != "" {
		identityUsage.RequestIndex = len(usageRequests)
		usageRequests = append(usageRequests, identityUsage)
	}
	generated := buildParallelIdentityGeneratedCharacters(candidates, clusters, allocator)
	generated = mergeGeneratedCharacters(nil, generated)
	generated = allocator.Assign(generated)
	sortGeneratedCharacters(generated)
	generated, correctionUsage, err := r.correctParallelIdentityCharacters(ctx, config, novelID, upToEpisodeIndex, generated)
	if err != nil {
		return nil, extractionStateFromAllocator(unresolved, allocator), usageRequests, err
	}
	if correctionUsage.Kind != "" {
		correctionUsage.RequestIndex = len(usageRequests)
		usageRequests = append(usageRequests, correctionUsage)
	}
	sortGeneratedCharacters(generated)
	unresolved = filterResolvedGeneratedUnresolvedMentions(unresolved, generated)
	state := extractionStateFromAllocator(unresolved, allocator)
	state.Terms = core.FilterAndMergeParallelTermFacts(seedTerms, rawTerms, generated)
	return generated, state, usageRequests, nil
}

func (r *Runtime) parallelIdentityRuntimeBatches(ctx context.Context, config *store.ResolvedAIGenerationConfig, novelID string, upToEpisodeIndex string, knownCharacters []characters.GeneratedCharacter, knownTerms []terms.GeneratedTerm, batches []extractionBatch, pendingUnresolved []characters.GeneratedUnresolvedMention) ([]extractionBatch, error) {
	runtimeBatches := []extractionBatch{}
	for _, batch := range batches {
		boundary := extractionBatchBoundary(batch)
		projectedCharacters := projectGeneratedCharactersAtBoundary(knownCharacters, boundary)
		projectedUnresolved := filterGeneratedUnresolvedAtBoundary(pendingUnresolved, boundary)
		remaining := append([]extractionChunk{}, batch.Chunks...)
		for len(remaining) > 0 {
			runtimeBatch, nextRemaining, err := r.nextRuntimeBatchForFocus(ctx, config, novelID, upToEpisodeIndex, projectedCharacters, knownTerms, batch, remaining, "parallel_entities", projectedUnresolved)
			if err != nil {
				return nil, err
			}
			runtimeBatches = append(runtimeBatches, runtimeBatch)
			remaining = nextRemaining
		}
	}
	return renumberParallelIdentityRuntimeBatches(runtimeBatches), nil
}

func renumberParallelIdentityRuntimeBatches(values []extractionBatch) []extractionBatch {
	for index := range values {
		values[index].BatchIndex = index + 1
		values[index].BatchCount = len(values)
	}
	return values
}

type parallelIdentityExtractionResult struct {
	RequestIndex int
	Batch        extractionBatch
	Candidates   []parallelIdentityCandidate
	Unresolved   []extractionUnresolvedMention
	Terms        []terms.GeneratedTerm
	Usage        ai.UsageRequest
	Err          error
}

type parallelIdentityLLMStartLimiter struct {
	mu       sync.Mutex
	next     time.Time
	interval time.Duration
}

func newParallelIdentityLLMStartLimiter(interval time.Duration) *parallelIdentityLLMStartLimiter {
	return &parallelIdentityLLMStartLimiter{interval: interval}
}

func (l *parallelIdentityLLMStartLimiter) Wait(ctx context.Context) error {
	if l.interval <= 0 {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	if !l.next.IsZero() && now.Before(l.next) {
		timer := time.NewTimer(time.Until(l.next))
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return ctx.Err()
		case <-timer.C:
		}
		now = time.Now()
	}
	l.next = now.Add(l.interval)
	return nil
}

func parallelIdentityLLMConcurrency() int {
	concurrency := positiveEnvIntWithFallback("EXTRACTION_LLM_CONCURRENCY", "CHARACTER_SUMMARY_LLM_CONCURRENCY", defaultParallelIdentityLLMConcurrency)
	if concurrency > maxParallelIdentityLLMConcurrency {
		return maxParallelIdentityLLMConcurrency
	}
	return concurrency
}

func parallelIdentityLLMStartInterval() time.Duration {
	raw := envWithFallback("EXTRACTION_LLM_START_INTERVAL_MS", "CHARACTER_SUMMARY_LLM_START_INTERVAL_MS")
	if raw == "" {
		return time.Duration(defaultParallelIdentityLLMStartIntervalMS) * time.Millisecond
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return time.Duration(defaultParallelIdentityLLMStartIntervalMS) * time.Millisecond
	}
	return time.Duration(value) * time.Millisecond
}

func parallelIdentityMaxReduceItems() int {
	return positiveEnvIntWithFallback("EXTRACTION_PARALLEL_MAX_REDUCE_ITEMS", "CHARACTER_SUMMARY_PARALLEL_MAX_REDUCE_ITEMS", defaultParallelIdentityMaxReduceItems)
}

func parallelIdentityMaxReduceTokens() int {
	return positiveEnvIntWithFallback("EXTRACTION_PARALLEL_MAX_REDUCE_TOKENS", "CHARACTER_SUMMARY_PARALLEL_MAX_REDUCE_TOKENS", defaultParallelIdentityMaxReduceTokens)
}

func runParallelIdentityLLMJobs(ctx context.Context, jobCount int, run func(context.Context, int) error) error {
	if jobCount == 0 {
		return nil
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sem := make(chan struct{}, parallelIdentityLLMConcurrency())
	startLimiter := newParallelIdentityLLMStartLimiter(parallelIdentityLLMStartInterval())
	var wg sync.WaitGroup
	var errMu sync.Mutex
	var firstErr error
	setErr := func(err error) {
		if err == nil {
			return
		}
		errMu.Lock()
		if firstErr == nil {
			firstErr = err
			cancel()
		}
		errMu.Unlock()
	}

	for index := 0; index < jobCount; index++ {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			wg.Wait()
			errMu.Lock()
			defer errMu.Unlock()
			if firstErr != nil {
				return firstErr
			}
			return ctx.Err()
		}

		index := index
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			if err := startLimiter.Wait(ctx); err != nil {
				setErr(err)
				return
			}
			if err := run(ctx, index); err != nil {
				setErr(err)
			}
		}()
	}
	wg.Wait()
	errMu.Lock()
	defer errMu.Unlock()
	return firstErr
}

func (r *Runtime) extractParallelIdentityCandidates(ctx context.Context, config *store.ResolvedAIGenerationConfig, novelID string, upToEpisodeIndex string, knownTerms []terms.GeneratedTerm, batches []extractionBatch, progressSink func(appextraction.BatchProgress), initialUnresolved []characters.GeneratedUnresolvedMention) ([]parallelIdentityCandidate, []terms.GeneratedTerm, []ai.UsageRequest, []characters.GeneratedUnresolvedMention, error) {
	return r.extractParallelIdentityCandidatesWithKnown(ctx, config, novelID, upToEpisodeIndex, nil, knownTerms, batches, progressSink, initialUnresolved)
}

func (r *Runtime) extractParallelIdentityCandidatesWithKnown(ctx context.Context, config *store.ResolvedAIGenerationConfig, novelID string, upToEpisodeIndex string, knownCharacters []characters.GeneratedCharacter, knownTerms []terms.GeneratedTerm, batches []extractionBatch, progressSink func(appextraction.BatchProgress), initialUnresolved []characters.GeneratedUnresolvedMention) ([]parallelIdentityCandidate, []terms.GeneratedTerm, []ai.UsageRequest, []characters.GeneratedUnresolvedMention, error) {
	results := make([]parallelIdentityExtractionResult, len(batches))
	var sinkMu sync.Mutex
	completedCount := 0
	completedCandidateCount := 0
	completedTermNames := map[string]bool{}
	runErr := runParallelIdentityLLMJobs(ctx, len(batches), func(requestCtx context.Context, index int) error {
		batch := batches[index]
		boundary := extractionBatchBoundary(batch)
		projectedCharacters := projectGeneratedCharactersAtBoundary(knownCharacters, boundary)
		projectedUnresolved := filterGeneratedUnresolvedAtBoundary(initialUnresolved, boundary)
		startedAt := time.Now()
		if progressSink != nil {
			sinkMu.Lock()
			progressSink(appextraction.BatchProgress{Phase: "start", Batch: batch})
			sinkMu.Unlock()
		}
		result, err := r.generateOpenRouterBatchForFocus(requestCtx, config, novelID, upToEpisodeIndex, projectedCharacters, knownTerms, batch, "parallel_entities", projectedUnresolved)
		extraction := parallelIdentityExtractionResult{RequestIndex: index, Batch: batch, Usage: result.Usage, Err: err}
		if extraction.Usage.Kind != "" {
			result.Usage.RequestIndex = index
			extraction.Usage = result.Usage
		}
		if err == nil {
			extraction.Unresolved = result.Delta.UnresolvedMentions
			extraction.Terms = result.Delta.Terms
			extraction.Candidates = parallelIdentityCandidatesFromDeltaWithKnown(novelID, index, batch, result.Delta, projectedCharacters)
		}
		results[index] = extraction
		if progressSink != nil {
			sinkMu.Lock()
			completedCount++
			completedCandidateCount += len(extraction.Candidates)
			for _, term := range extraction.Terms {
				if name := strings.TrimSpace(term.Term); name != "" {
					completedTermNames[name] = true
				}
			}
			progressSink(appextraction.BatchProgress{
				Phase:                   "complete",
				Batch:                   batch,
				CompletedBatchCount:     completedCount,
				ElapsedMs:               time.Since(startedAt).Milliseconds(),
				GeneratedCharacterCount: len(extraction.Candidates),
				MergedCharacterCount:    completedCandidateCount,
				GeneratedTermCount:      len(extraction.Terms),
				MergedTermCount:         len(completedTermNames),
			})
			sinkMu.Unlock()
		}
		return err
	})

	candidates := []parallelIdentityCandidate{}
	usageRequests := []ai.UsageRequest{}
	rawTerms := []terms.GeneratedTerm{}
	unresolved := append([]characters.GeneratedUnresolvedMention{}, initialUnresolved...)
	for _, result := range results {
		if result.Usage.Kind != "" {
			usageRequests = append(usageRequests, result.Usage)
		}
		if result.Err != nil {
			continue
		}
		candidates = append(candidates, result.Candidates...)
		unresolved = mergeGeneratedUnresolvedMentions(unresolved, result.Unresolved)
		rawTerms = append(rawTerms, result.Terms...)
	}
	if runErr != nil {
		return nil, nil, usageRequests, unresolved, runErr
	}
	return candidates, rawTerms, usageRequests, unresolved, nil
}

func extractionBatchBoundary(batch extractionBatch) string {
	boundary := ""
	for _, episodeIndex := range batch.EpisodeIndexes {
		if boundary == "" || compareEpisodeString(episodeIndex, boundary) > 0 {
			boundary = episodeIndex
		}
	}
	return boundary
}

func projectGeneratedCharactersAtBoundary(values []characters.GeneratedCharacter, boundary string) []characters.GeneratedCharacter {
	if strings.TrimSpace(boundary) == "" {
		return nil
	}
	result := make([]characters.GeneratedCharacter, 0, len(values))
	for _, value := range values {
		if value.FirstAppearanceEpisodeIndex != "" && compareEpisodeString(value.FirstAppearanceEpisodeIndex, boundary) > 0 {
			continue
		}
		value.NameHistory = filterGeneratedTextVersionsAtBoundary(value.NameHistory, boundary)
		value.FullNameHistory = filterGeneratedTextVersionsAtBoundary(value.FullNameHistory, boundary)
		value.GenderHistory = filterGeneratedTextVersionsAtBoundary(value.GenderHistory, boundary)
		value.Aliases = filterGeneratedTextVersionsAtBoundary(value.Aliases, boundary)
		value.AppearanceHistory = filterGeneratedHistoryVersionsAtBoundary(value.AppearanceHistory, boundary)
		value.PersonalityHistory = filterGeneratedHistoryVersionsAtBoundary(value.PersonalityHistory, boundary)
		value.SummaryHistory = filterGeneratedHistoryVersionsAtBoundary(value.SummaryHistory, boundary)
		if len(value.NameHistory) > 0 {
			latest := value.NameHistory[len(value.NameHistory)-1]
			value.CanonicalName = latest.Text
			value.CanonicalEpisodeIndex = latest.EpisodeIndex
		} else if value.CanonicalEpisodeIndex != "" && compareEpisodeString(value.CanonicalEpisodeIndex, boundary) > 0 {
			continue
		}
		value.FullName, value.FullNameEpisodeIndex = core.LatestGeneratedTextVersionValue(nil, "", value.FullNameHistory)
		value.Gender, value.GenderEpisodeIndex = core.LatestGeneratedTextVersionValue(nil, "", value.GenderHistory)
		result = append(result, value)
	}
	return result
}

func filterGeneratedTextVersionsAtBoundary(values []characters.GeneratedTextVersion, boundary string) []characters.GeneratedTextVersion {
	result := make([]characters.GeneratedTextVersion, 0, len(values))
	for _, value := range values {
		if compareEpisodeString(value.EpisodeIndex, boundary) <= 0 {
			result = append(result, value)
		}
	}
	return result
}

func filterGeneratedHistoryVersionsAtBoundary(values []characters.GeneratedHistoryVersion, boundary string) []characters.GeneratedHistoryVersion {
	result := make([]characters.GeneratedHistoryVersion, 0, len(values))
	for _, value := range values {
		if compareEpisodeString(value.EpisodeIndex, boundary) <= 0 {
			result = append(result, value)
		}
	}
	return result
}

func filterGeneratedUnresolvedAtBoundary(values []characters.GeneratedUnresolvedMention, boundary string) []characters.GeneratedUnresolvedMention {
	result := make([]characters.GeneratedUnresolvedMention, 0, len(values))
	for _, value := range values {
		if compareEpisodeString(value.EpisodeIndex, boundary) <= 0 {
			result = append(result, value)
		}
	}
	return result
}

func parallelIdentityCandidatesFromDelta(novelID string, requestIndex int, batch extractionBatch, delta extractionDelta) []parallelIdentityCandidate {
	return parallelIdentityCandidatesFromDeltaWithKnown(novelID, requestIndex, batch, delta, nil)
}

func parallelIdentityCandidatesFromDeltaWithKnown(novelID string, requestIndex int, batch extractionBatch, delta extractionDelta, knownCharacters []characters.GeneratedCharacter) []parallelIdentityCandidate {
	localNovelID := fmt.Sprintf("%s-local-%d", novelID, requestIndex+1)
	allocator := characters.NewGeneratedCharacterIDAllocator(localNovelID, knownCharacters)
	generated, _ := applyExtractionDelta(localNovelID, knownCharacters, delta, allocator)
	knownByID := generatedCharacterMapByID(knownCharacters)
	candidates := make([]parallelIdentityCandidate, 0, len(generated))
	for index, item := range generated {
		itemID := strings.TrimSpace(item.CharacterID)
		if known, ok := knownByID[itemID]; ok && reflect.DeepEqual(known, item) {
			continue
		}
		if itemID == "" || knownByID[itemID].CharacterID == "" {
			item.CharacterID = ""
		}
		candidates = append(candidates, parallelIdentityCandidate{
			LocalID:    fmt.Sprintf("b%d-c%d", requestIndex+1, index+1),
			Source:     "batch",
			BatchIndex: batch.BatchIndex,
			Character:  item,
		})
	}
	return candidates
}

func generatedCharacterMapByID(values []characters.GeneratedCharacter) map[string]characters.GeneratedCharacter {
	result := map[string]characters.GeneratedCharacter{}
	for _, value := range values {
		id := strings.TrimSpace(value.CharacterID)
		if id != "" {
			result[id] = value
		}
	}
	return result
}

func seedParallelIdentityCandidates(seed []characters.GeneratedCharacter) []parallelIdentityCandidate {
	candidates := make([]parallelIdentityCandidate, 0, len(seed))
	for _, item := range seed {
		id := strings.TrimSpace(item.CharacterID)
		if id == "" {
			continue
		}
		candidates = append(candidates, parallelIdentityCandidate{
			LocalID:   "seed:" + id,
			Source:    "seed",
			Character: item,
		})
	}
	return candidates
}

func discoveryParallelIdentityCandidates(discovered []characters.GeneratedCharacter) []parallelIdentityCandidate {
	candidates := make([]parallelIdentityCandidate, 0, len(discovered))
	for index, item := range discovered {
		candidates = append(candidates, parallelIdentityCandidate{
			LocalID:    fmt.Sprintf("d%d", index+1),
			Source:     "discovery",
			BatchIndex: 0,
			Character:  item,
		})
	}
	return candidates
}

func (r *Runtime) resolveParallelIdentityClusters(ctx context.Context, config *store.ResolvedAIGenerationConfig, novelID string, upToEpisodeIndex string, candidates []parallelIdentityCandidate) ([]parallelIdentityCluster, ai.UsageRequest, error) {
	clusters, usage, err := r.resolveParallelIdentityClustersOneShot(ctx, config, novelID, upToEpisodeIndex, candidates)
	if err == nil || !isParallelIdentityOneShotTooLarge(err) {
		return clusters, usage, err
	}
	return r.resolveParallelIdentityClustersByNameGroups(ctx, config, novelID, upToEpisodeIndex, candidates)
}

func (r *Runtime) resolveParallelIdentityClustersOneShot(ctx context.Context, config *store.ResolvedAIGenerationConfig, novelID string, upToEpisodeIndex string, candidates []parallelIdentityCandidate) ([]parallelIdentityCluster, ai.UsageRequest, error) {
	if len(candidates) == 0 {
		return nil, ai.UsageRequest{}, nil
	}
	if len(candidates) == 1 {
		return []parallelIdentityCluster{{LocalIDs: []string{candidates[0].LocalID}, CanonicalName: candidates[0].Character.CanonicalName, Confidence: 1}}, ai.UsageRequest{}, nil
	}
	if maxItems := parallelIdentityMaxReduceItems(); len(candidates) > maxItems {
		return nil, ai.UsageRequest{}, fmt.Errorf("%w for one-shot identity resolution: %d candidates exceeds limit %d; use serial or reduce target episodes.", errParallelIdentityOneShotTooLarge, len(candidates), maxItems)
	}
	startedAt := time.Now()
	systemPrompt := strings.Join([]string{
		"あなたは日本語小説のキャラクター同一人物判定専用アシスタントです。",
		"入力された候補人物を、本文上で同一人物と判断できるものだけ cluster 化してください。",
		"名前や別名が似ているだけで同一人物にしないでください。",
		"役職、関係名、説明的な呼称だけを根拠に統合しないでください。",
		"出力は JSON のみです。",
	}, " ")
	payload := map[string]any{
		"novelId":          novelID,
		"upToEpisodeIndex": upToEpisodeIndex,
		"candidates":       parallelIdentityCandidateCards(candidates),
		"outputContract":   "Return {\"clusters\":[{\"localIds\":[...],\"canonicalName\":\"...\",\"confidence\":0.0,\"reason\":\"...\"}]}. Include only clusters you are confident about; omitted candidates will be kept as separate people.",
	}
	raw, _ := json.MarshalIndent(payload, "", "  ")
	userPrompt := string(raw)
	responseFormat := map[string]any{"type": "json_object"}
	promptTokens := estimateOpenRouterChatRequestTokens([]ai.ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}, nil, responseFormat)
	if maxTokens := parallelIdentityMaxReduceTokens(); promptTokens > maxTokens {
		return nil, ai.UsageRequest{}, fmt.Errorf("%w for one-shot identity resolution: estimated prompt tokens %d exceeds limit %d; use serial or reduce target episodes.", errParallelIdentityOneShotTooLarge, promptTokens, maxTokens)
	}
	maxTokens, err := resolveOpenRouterMaxOutputTokens(ctx, config.APIKey, config.ModelID, config.ProviderOrder, extractionDefaultMaxTokens, promptTokens)
	if err != nil {
		return nil, ai.UsageRequest{}, err
	}
	if maxTokens < extractionMinimumCompletionTokens {
		return nil, ai.UsageRequest{}, fmt.Errorf("OpenRouter request has only %d output tokens available for character identity resolution; at least %d are required.", maxTokens, extractionMinimumCompletionTokens)
	}
	temperature := 0.1
	result, err := ai.GenerateOpenRouterChat(ctx, ai.OpenRouterConfig{
		APIKey:            config.APIKey,
		ModelID:           config.ModelID,
		ProviderOrder:     config.ProviderOrder,
		AllowFallbacks:    config.AllowFallbacks,
		RequireParameters: config.RequireParameters,
		Temperature:       &temperature,
		MaxTokens:         maxTokens,
		ResponseFormat:    responseFormat,
	}, []ai.ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	})
	status := "ok"
	if err != nil {
		status = "error"
	}
	r.log("parallel_identity_resolve_openrouter", startedAt, "status", status, "novelId", novelID, "upToEpisodeIndex", upToEpisodeIndex, "candidates", len(candidates), "inputTokens", result.InputTokens, "outputTokens", result.OutputTokens, "totalTokens", result.TotalTokens)
	if err != nil {
		return nil, ai.UsageRequest{}, err
	}
	var decoded struct {
		Clusters []parallelIdentityCluster `json:"clusters"`
	}
	if err := json.Unmarshal([]byte(result.Answer), &decoded); err != nil {
		return nil, ai.UsageRequest{}, err
	}
	usage := ai.UsageRequest{
		Kind:         "extraction_identity_resolution",
		InputTokens:  result.InputTokens,
		OutputTokens: result.OutputTokens,
		TotalTokens:  result.TotalTokens,
	}
	if usage.InputTokens <= 0 {
		usage.InputTokens = promptTokens
	}
	if usage.TotalTokens <= 0 {
		usage.TotalTokens = usage.InputTokens + usage.OutputTokens
	}
	return completeParallelIdentitySingletons(filterAutoApplicableParallelIdentityClusters(decoded.Clusters), candidates), usage, nil
}

func filterAutoApplicableParallelIdentityClusters(values []parallelIdentityCluster) []parallelIdentityCluster {
	result := make([]parallelIdentityCluster, 0, len(values))
	for _, value := range values {
		if value.Confidence < 0 {
			value.Confidence = 0
		} else if value.Confidence > 1 {
			value.Confidence = 1
		}
		value.LocalIDs = normalizeParallelIdentityLocalIDs(value.LocalIDs)
		if value.Confidence < core.MergeAutoApplyConfidence || len(value.LocalIDs) < 2 {
			continue
		}
		result = append(result, value)
	}
	return result
}

func (r *Runtime) resolveParallelIdentityClustersByNameGroups(ctx context.Context, config *store.ResolvedAIGenerationConfig, novelID string, upToEpisodeIndex string, candidates []parallelIdentityCandidate) ([]parallelIdentityCluster, ai.UsageRequest, error) {
	groups := parallelIdentityCandidateNameGroups(candidates, parallelIdentityFallbackChunkSize())
	clusters := []parallelIdentityCluster{}
	usageParts := []ai.UsageRequest{}
	for _, group := range groups {
		groupClusters, groupUsage, err := r.resolveParallelIdentityClustersOneShot(ctx, config, novelID, upToEpisodeIndex, group)
		if err != nil {
			if !isParallelIdentityOneShotTooLarge(err) {
				return nil, aggregateParallelIdentityUsage("extraction_identity_resolution", usageParts), err
			}
			log.Printf("viewer-api-go: parallel_identity identity resolution degraded %d candidates to singleton clusters (novel=%s upTo=%s): %v", len(group), novelID, upToEpisodeIndex, err)
			groupClusters = completeParallelIdentitySingletons(nil, group)
		}
		clusters = append(clusters, groupClusters...)
		if groupUsage.Kind != "" {
			usageParts = append(usageParts, groupUsage)
		}
	}
	return completeParallelIdentitySingletons(mergeOverlappingParallelIdentityClusters(clusters), candidates), aggregateParallelIdentityUsage("extraction_identity_resolution", usageParts), nil
}

type parallelIdentityDiscoveredName struct {
	Name         string   `json:"name"`
	Aliases      []string `json:"aliases"`
	EpisodeIndex string   `json:"episodeIndex"`
	Reason       string   `json:"reason"`
}

func (r *Runtime) discoverParallelIdentityNames(ctx context.Context, config *store.ResolvedAIGenerationConfig, novelID string, upToEpisodeIndex string, batches []extractionBatch) ([]characters.GeneratedCharacter, []ai.UsageRequest, error) {
	if len(batches) == 0 {
		return nil, nil, nil
	}
	type discoveryResult struct {
		RequestIndex int
		Names        []parallelIdentityDiscoveredName
		Usage        ai.UsageRequest
		Err          error
	}
	results := make([]discoveryResult, len(batches))
	runErr := runParallelIdentityLLMJobs(ctx, len(batches), func(requestCtx context.Context, index int) error {
		names, usage, err := r.discoverParallelIdentityNamesForBatch(requestCtx, config, novelID, upToEpisodeIndex, index, batches[index])
		results[index] = discoveryResult{RequestIndex: index, Names: names, Usage: usage, Err: err}
		return err
	})
	merged := []parallelIdentityDiscoveredName{}
	usageRequests := []ai.UsageRequest{}
	for _, result := range results {
		if result.Usage.Kind != "" {
			result.Usage.RequestIndex = len(usageRequests)
			usageRequests = append(usageRequests, result.Usage)
		}
		for _, item := range result.Names {
			name := strings.TrimSpace(item.Name)
			if name == "" {
				continue
			}
			item.Name = name
			merged = append(merged, item)
		}
	}
	if runErr != nil {
		return nil, usageRequests, runErr
	}
	return discoveredNamesToGeneratedCharacters(merged, upToEpisodeIndex), usageRequests, nil
}

func (r *Runtime) discoverParallelIdentityNamesForBatch(ctx context.Context, config *store.ResolvedAIGenerationConfig, novelID string, upToEpisodeIndex string, requestIndex int, batch extractionBatch) ([]parallelIdentityDiscoveredName, ai.UsageRequest, error) {
	startedAt := time.Now()
	systemPrompt := strings.Join([]string{
		"あなたは日本語小説の登場人物名候補を発見するアシスタントです。",
		"本文に登場する人物名、通称、役職名つきの人物呼称だけを抽出してください。",
		"地名、組織名、種族名、一般名詞、説明だけの語は除外してください。",
		"出力は JSON のみです。",
	}, " ")
	payload := map[string]any{
		"novelId":          novelID,
		"upToEpisodeIndex": upToEpisodeIndex,
		"batch":            extractionBatchPromptPayload(batch),
		"outputContract":   "Return {\"characters\":[{\"name\":\"...\",\"aliases\":[\"...\"],\"episodeIndex\":\"...\",\"reason\":\"...\"}]}.",
	}
	raw, _ := json.MarshalIndent(payload, "", "  ")
	userPrompt := string(raw)
	responseFormat := map[string]any{"type": "json_object"}
	promptTokens := estimateOpenRouterChatRequestTokens([]ai.ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}, nil, responseFormat)
	maxTokens, err := resolveOpenRouterMaxOutputTokens(ctx, config.APIKey, config.ModelID, config.ProviderOrder, 2048, promptTokens)
	if err != nil {
		return nil, ai.UsageRequest{}, err
	}
	temperature := 0.1
	result, err := ai.GenerateOpenRouterChat(ctx, ai.OpenRouterConfig{
		APIKey:            config.APIKey,
		ModelID:           config.ModelID,
		ProviderOrder:     config.ProviderOrder,
		AllowFallbacks:    config.AllowFallbacks,
		RequireParameters: config.RequireParameters,
		Temperature:       &temperature,
		MaxTokens:         maxTokens,
		ResponseFormat:    responseFormat,
	}, []ai.ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	})
	status := "ok"
	if err != nil {
		status = "error"
	}
	r.log("parallel_identity_name_discovery_openrouter", startedAt, "status", status, "novelId", novelID, "upToEpisodeIndex", upToEpisodeIndex, "requestIndex", requestIndex, "inputTokens", result.InputTokens, "outputTokens", result.OutputTokens, "totalTokens", result.TotalTokens)
	usage := ai.UsageRequest{}
	if result.InputTokens > 0 || result.OutputTokens > 0 || result.TotalTokens > 0 {
		usage = ai.UsageRequest{
			Kind:         "extraction_name_discovery",
			InputTokens:  result.InputTokens,
			OutputTokens: result.OutputTokens,
			TotalTokens:  result.TotalTokens,
		}
		if usage.InputTokens <= 0 {
			usage.InputTokens = promptTokens
		}
		if usage.TotalTokens <= 0 {
			usage.TotalTokens = usage.InputTokens + usage.OutputTokens
		}
	}
	if err != nil {
		return nil, usage, err
	}
	var decoded struct {
		Characters []parallelIdentityDiscoveredName `json:"characters"`
	}
	if err := json.Unmarshal([]byte(result.Answer), &decoded); err != nil {
		return nil, usage, err
	}
	normalized, err := normalizeDiscoveredNamesForBatch(decoded.Characters, batch, upToEpisodeIndex)
	if err != nil {
		return nil, usage, err
	}
	return normalized, usage, nil
}

func normalizeDiscoveredNamesForBatch(values []parallelIdentityDiscoveredName, batch extractionBatch, upToEpisodeIndex string) ([]parallelIdentityDiscoveredName, error) {
	allowed := map[string]bool{}
	boundary := ""
	for _, episodeIndex := range append(append([]string{}, batch.EpisodeIndexes...), extractionBatchChunkEpisodeIndexes(batch)...) {
		episodeIndex = strings.TrimSpace(episodeIndex)
		if !isDigitsEpisodeIndex(episodeIndex) {
			continue
		}
		allowed[episodeIndex] = true
		if boundary == "" || compareEpisodeString(episodeIndex, boundary) > 0 {
			boundary = episodeIndex
		}
	}
	if boundary == "" {
		return nil, errors.New("name discovery batch has no valid episode indexes")
	}
	result := make([]parallelIdentityDiscoveredName, len(values))
	for index, value := range values {
		episodeIndex := strings.TrimSpace(value.EpisodeIndex)
		if episodeIndex == "" {
			episodeIndex = boundary
		}
		if !isDigitsEpisodeIndex(episodeIndex) || !allowed[episodeIndex] || (isDigitsEpisodeIndex(upToEpisodeIndex) && compareEpisodeString(episodeIndex, upToEpisodeIndex) > 0) {
			return nil, fmt.Errorf("name discovery response contained episodeIndex %q outside the current discovery batch", value.EpisodeIndex)
		}
		value.EpisodeIndex = episodeIndex
		result[index] = value
	}
	return result, nil
}

func extractionBatchChunkEpisodeIndexes(batch extractionBatch) []string {
	result := make([]string, 0, len(batch.Chunks))
	for _, chunk := range batch.Chunks {
		result = append(result, chunk.EpisodeIndex)
	}
	return result
}

func isDigitsEpisodeIndex(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func discoveredNamesToGeneratedCharacters(values []parallelIdentityDiscoveredName, fallbackEpisodeIndex string) []characters.GeneratedCharacter {
	generated := make([]characters.GeneratedCharacter, 0, len(values))
	for _, value := range values {
		name := strings.TrimSpace(value.Name)
		if name == "" {
			continue
		}
		episodeIndex := strings.TrimSpace(value.EpisodeIndex)
		if episodeIndex == "" {
			episodeIndex = fallbackEpisodeIndex
		}
		aliases := []characters.GeneratedTextVersion{{Text: name, EpisodeIndex: episodeIndex}}
		for _, alias := range value.Aliases {
			alias = strings.TrimSpace(alias)
			if alias != "" && alias != name {
				aliases = append(aliases, characters.GeneratedTextVersion{Text: alias, EpisodeIndex: episodeIndex})
			}
		}
		character := characters.GeneratedCharacter{
			CanonicalName:               name,
			CanonicalEpisodeIndex:       episodeIndex,
			FirstAppearanceEpisodeIndex: episodeIndex,
			Aliases:                     mergeGeneratedTextVersionLists(aliases),
			NameHistory:                 []characters.GeneratedTextVersion{{Text: name, EpisodeIndex: episodeIndex}},
		}
		if reason := strings.TrimSpace(value.Reason); reason != "" {
			character.SummaryHistory = []characters.GeneratedHistoryVersion{{EpisodeIndex: episodeIndex, Text: reason}}
		}
		generated = append(generated, character)
	}
	return generated
}

type parallelIdentityCorrection struct {
	CharacterID   string   `json:"characterId"`
	CanonicalName string   `json:"canonicalName"`
	Aliases       []string `json:"aliases"`
	Keep          *bool    `json:"keep"`
	Reason        string   `json:"reason"`
}

func (r *Runtime) correctParallelIdentityCharacters(ctx context.Context, config *store.ResolvedAIGenerationConfig, novelID string, upToEpisodeIndex string, generated []characters.GeneratedCharacter) ([]characters.GeneratedCharacter, ai.UsageRequest, error) {
	corrected, usage, err := r.correctParallelIdentityCharactersOneShot(ctx, config, novelID, upToEpisodeIndex, generated)
	if err == nil || !isParallelIdentityOneShotTooLarge(err) {
		return corrected, usage, err
	}
	return r.correctParallelIdentityCharactersInChunks(ctx, config, novelID, upToEpisodeIndex, generated)
}

func (r *Runtime) correctParallelIdentityCharactersOneShot(ctx context.Context, config *store.ResolvedAIGenerationConfig, novelID string, upToEpisodeIndex string, generated []characters.GeneratedCharacter) ([]characters.GeneratedCharacter, ai.UsageRequest, error) {
	if len(generated) == 0 {
		return generated, ai.UsageRequest{}, nil
	}
	if maxItems := parallelIdentityMaxReduceItems(); len(generated) > maxItems {
		return nil, ai.UsageRequest{}, fmt.Errorf("%w for one-shot correction: %d characters exceeds limit %d; use serial or reduce target episodes.", errParallelIdentityOneShotTooLarge, len(generated), maxItems)
	}
	startedAt := time.Now()
	systemPrompt := strings.Join([]string{
		"あなたは日本語小説のキャラクター一覧を最終補正するアシスタントです。",
		"人物でない候補、重複後に残った不要候補、不自然な代表名を補正してください。",
		"本文根拠が弱い候補は keep=false にできます。出力は JSON のみです。",
	}, " ")
	payload := map[string]any{
		"novelId":          novelID,
		"upToEpisodeIndex": upToEpisodeIndex,
		"characters":       correctionCharacterCards(generated),
		"outputContract":   "Return {\"characters\":[{\"characterId\":\"...\",\"canonicalName\":\"...\",\"aliases\":[\"...\"],\"keep\":true,\"reason\":\"...\"}]}. Omitted characters are kept unchanged.",
	}
	raw, _ := json.MarshalIndent(payload, "", "  ")
	userPrompt := string(raw)
	responseFormat := map[string]any{"type": "json_object"}
	promptTokens := estimateOpenRouterChatRequestTokens([]ai.ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}, nil, responseFormat)
	if maxTokens := parallelIdentityMaxReduceTokens(); promptTokens > maxTokens {
		return nil, ai.UsageRequest{}, fmt.Errorf("%w for one-shot correction: estimated prompt tokens %d exceeds limit %d; use serial or reduce target episodes.", errParallelIdentityOneShotTooLarge, promptTokens, maxTokens)
	}
	maxTokens, err := resolveOpenRouterMaxOutputTokens(ctx, config.APIKey, config.ModelID, config.ProviderOrder, extractionDefaultMaxTokens, promptTokens)
	if err != nil {
		return nil, ai.UsageRequest{}, err
	}
	temperature := 0.1
	result, err := ai.GenerateOpenRouterChat(ctx, ai.OpenRouterConfig{
		APIKey:            config.APIKey,
		ModelID:           config.ModelID,
		ProviderOrder:     config.ProviderOrder,
		AllowFallbacks:    config.AllowFallbacks,
		RequireParameters: config.RequireParameters,
		Temperature:       &temperature,
		MaxTokens:         maxTokens,
		ResponseFormat:    responseFormat,
	}, []ai.ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	})
	status := "ok"
	if err != nil {
		status = "error"
	}
	r.log("parallel_identity_correction_openrouter", startedAt, "status", status, "novelId", novelID, "upToEpisodeIndex", upToEpisodeIndex, "characterCount", len(generated), "inputTokens", result.InputTokens, "outputTokens", result.OutputTokens, "totalTokens", result.TotalTokens)
	if err != nil {
		return nil, ai.UsageRequest{}, err
	}
	var decoded struct {
		Characters []parallelIdentityCorrection `json:"characters"`
	}
	if err := json.Unmarshal([]byte(result.Answer), &decoded); err != nil {
		return nil, ai.UsageRequest{}, err
	}
	corrected := applyParallelIdentityCorrections(generated, decoded.Characters, upToEpisodeIndex)
	usage := ai.UsageRequest{
		Kind:         "extraction_correction",
		InputTokens:  result.InputTokens,
		OutputTokens: result.OutputTokens,
		TotalTokens:  result.TotalTokens,
	}
	if usage.InputTokens <= 0 {
		usage.InputTokens = promptTokens
	}
	if usage.TotalTokens <= 0 {
		usage.TotalTokens = usage.InputTokens + usage.OutputTokens
	}
	return corrected, usage, nil
}

func (r *Runtime) correctParallelIdentityCharactersInChunks(ctx context.Context, config *store.ResolvedAIGenerationConfig, novelID string, upToEpisodeIndex string, generated []characters.GeneratedCharacter) ([]characters.GeneratedCharacter, ai.UsageRequest, error) {
	maxItems := parallelIdentityFallbackChunkSize()
	corrected := make([]characters.GeneratedCharacter, 0, len(generated))
	usageParts := []ai.UsageRequest{}
	for start := 0; start < len(generated); start += maxItems {
		end := start + maxItems
		if end > len(generated) {
			end = len(generated)
		}
		chunk := generated[start:end]
		chunkCorrected, chunkUsage, err := r.correctParallelIdentityCharactersOneShot(ctx, config, novelID, upToEpisodeIndex, chunk)
		if err != nil {
			if !isParallelIdentityOneShotTooLarge(err) {
				return nil, aggregateParallelIdentityUsage("extraction_correction", usageParts), err
			}
			if len(chunk) == 1 {
				log.Printf("viewer-api-go: parallel_identity correction kept character %s uncorrected because its prompt exceeds limits (novel=%s upTo=%s): %v", chunk[0].CharacterID, novelID, upToEpisodeIndex, err)
				chunkCorrected = chunk
				chunkUsage = ai.UsageRequest{}
			} else {
				chunkCorrected, chunkUsage, err = r.correctParallelIdentityCharactersInChunks(ctx, config, novelID, upToEpisodeIndex, chunk[:len(chunk)/2])
				if err != nil {
					return nil, aggregateParallelIdentityUsage("extraction_correction", append(append([]ai.UsageRequest{}, usageParts...), chunkUsage)), err
				}
				remainingCorrected, remainingUsage, err := r.correctParallelIdentityCharactersInChunks(ctx, config, novelID, upToEpisodeIndex, chunk[len(chunk)/2:])
				if err != nil {
					return nil, aggregateParallelIdentityUsage("extraction_correction", append(append([]ai.UsageRequest{}, usageParts...), chunkUsage, remainingUsage)), err
				}
				chunkCorrected = append(chunkCorrected, remainingCorrected...)
				chunkUsage = aggregateParallelIdentityUsage("extraction_correction", []ai.UsageRequest{chunkUsage, remainingUsage})
			}
		}
		corrected = append(corrected, chunkCorrected...)
		if chunkUsage.Kind != "" {
			usageParts = append(usageParts, chunkUsage)
		}
	}
	return corrected, aggregateParallelIdentityUsage("extraction_correction", usageParts), nil
}

func applyParallelIdentityCorrections(generated []characters.GeneratedCharacter, corrections []parallelIdentityCorrection, fallbackEpisodeIndex string) []characters.GeneratedCharacter {
	correctionByID := map[string]parallelIdentityCorrection{}
	for _, correction := range corrections {
		id := strings.TrimSpace(correction.CharacterID)
		if id != "" {
			correctionByID[id] = correction
		}
	}
	result := []characters.GeneratedCharacter{}
	for _, character := range generated {
		correction, ok := correctionByID[strings.TrimSpace(character.CharacterID)]
		if ok && correction.Keep != nil && !*correction.Keep {
			continue
		}
		if ok {
			if name := strings.TrimSpace(correction.CanonicalName); name != "" {
				episodeIndex := generatedCharacterNameEpisodeIndex(character, name)
				if episodeIndex == "" {
					episodeIndex = fallbackEpisodeIndex
				}
				character.CanonicalName = name
				character.CanonicalEpisodeIndex = episodeIndex
				character.NameHistory = mergeGeneratedTextVersionLists(character.NameHistory, []characters.GeneratedTextVersion{{Text: name, EpisodeIndex: episodeIndex}})
			}
			aliases := []characters.GeneratedTextVersion{}
			for _, alias := range correction.Aliases {
				alias = strings.TrimSpace(alias)
				if alias != "" {
					episodeIndex := generatedCharacterNameEpisodeIndex(character, alias)
					if episodeIndex == "" {
						episodeIndex = fallbackEpisodeIndex
					}
					aliases = append(aliases, characters.GeneratedTextVersion{Text: alias, EpisodeIndex: episodeIndex})
				}
			}
			character.Aliases = mergeGeneratedTextVersionLists(character.Aliases, aliases)
		}
		result = append(result, character)
	}
	return result
}

func generatedCharacterNameEpisodeIndex(character characters.GeneratedCharacter, name string) string {
	name = strings.TrimSpace(name)
	best := ""
	consider := func(text string, episodeIndex string) {
		if strings.TrimSpace(text) != name || !isDigitsEpisodeIndex(episodeIndex) {
			return
		}
		if best == "" || compareEpisodeString(episodeIndex, best) < 0 {
			best = episodeIndex
		}
	}
	consider(character.CanonicalName, character.CanonicalEpisodeIndex)
	for _, versions := range [][]characters.GeneratedTextVersion{character.NameHistory, character.FullNameHistory, character.Aliases} {
		for _, version := range versions {
			consider(version.Text, version.EpisodeIndex)
		}
	}
	return best
}

func correctionCharacterCards(values []characters.GeneratedCharacter) []map[string]any {
	cards := make([]map[string]any, 0, len(values))
	for _, character := range values {
		cards = append(cards, map[string]any{
			"characterId":                 character.CharacterID,
			"canonicalName":               character.CanonicalName,
			"aliases":                     generatedTextVersionTexts(character.Aliases),
			"firstAppearanceEpisodeIndex": character.FirstAppearanceEpisodeIndex,
			"summary":                     latestGeneratedHistoryText(character.SummaryHistory),
			"appearance":                  latestGeneratedHistoryText(character.AppearanceHistory),
			"personality":                 latestGeneratedHistoryText(character.PersonalityHistory),
		})
	}
	return cards
}

func parallelIdentityCandidateCards(candidates []parallelIdentityCandidate) []map[string]any {
	cards := make([]map[string]any, 0, len(candidates))
	for _, candidate := range candidates {
		character := candidate.Character
		card := map[string]any{
			"localId":                     candidate.LocalID,
			"source":                      candidate.Source,
			"canonicalName":               character.CanonicalName,
			"aliases":                     generatedTextVersionTexts(character.Aliases),
			"firstAppearanceEpisodeIndex": firstNonEmptySummaryString(character.FirstAppearanceEpisodeIndex, character.CanonicalEpisodeIndex),
			"summary":                     latestGeneratedHistoryText(character.SummaryHistory),
			"appearance":                  latestGeneratedHistoryText(character.AppearanceHistory),
			"personality":                 latestGeneratedHistoryText(character.PersonalityHistory),
		}
		if candidate.BatchIndex > 0 {
			card["batchIndex"] = candidate.BatchIndex
		}
		if character.FullName != nil && strings.TrimSpace(*character.FullName) != "" {
			card["fullName"] = strings.TrimSpace(*character.FullName)
		}
		if character.Gender != nil && strings.TrimSpace(*character.Gender) != "" {
			card["gender"] = strings.TrimSpace(*character.Gender)
		}
		cards = append(cards, card)
	}
	return cards
}

func generatedTextVersionTexts(values []characters.GeneratedTextVersion) []string {
	result := []string{}
	seen := map[string]bool{}
	for _, value := range values {
		text := strings.TrimSpace(value.Text)
		if text == "" || seen[text] {
			continue
		}
		seen[text] = true
		result = append(result, text)
		if len(result) >= 12 {
			break
		}
	}
	return result
}

func isParallelIdentityOneShotTooLarge(err error) bool {
	return errors.Is(err, errParallelIdentityOneShotTooLarge)
}

func parallelIdentityFallbackChunkSize() int {
	maxItems := parallelIdentityMaxReduceItems()
	if maxItems < 2 {
		maxItems = 2
	}
	return maxItems
}

func aggregateParallelIdentityUsage(kind string, values []ai.UsageRequest) ai.UsageRequest {
	usage := ai.UsageRequest{Kind: kind}
	hasUsage := false
	for _, value := range values {
		if value.Kind == "" {
			continue
		}
		hasUsage = true
		usage.InputTokens += value.InputTokens
		usage.OutputTokens += value.OutputTokens
		usage.TotalTokens += value.TotalTokens
		usage.CachedInputTokens += value.CachedInputTokens
		usage.ReasoningOutputTokens += value.ReasoningOutputTokens
		usage.Cost += value.Cost
	}
	if !hasUsage {
		return ai.UsageRequest{}
	}
	if usage.TotalTokens <= 0 {
		usage.TotalTokens = usage.InputTokens + usage.OutputTokens
	}
	return usage
}

func mergeOverlappingParallelIdentityClusters(clusters []parallelIdentityCluster) []parallelIdentityCluster {
	if len(clusters) < 2 {
		return clusters
	}
	parent := make([]int, len(clusters))
	for index := range parent {
		parent[index] = index
	}
	var find func(int) int
	find = func(index int) int {
		if parent[index] != index {
			parent[index] = find(parent[index])
		}
		return parent[index]
	}
	union := func(a int, b int) {
		rootA := find(a)
		rootB := find(b)
		if rootA != rootB {
			parent[rootB] = rootA
		}
	}

	ownerByLocalID := map[string]int{}
	for index, cluster := range clusters {
		for _, localID := range cluster.LocalIDs {
			localID = strings.TrimSpace(localID)
			if localID == "" {
				continue
			}
			if owner, ok := ownerByLocalID[localID]; ok {
				union(owner, index)
			} else {
				ownerByLocalID[localID] = index
			}
		}
	}

	clusterIndexes := map[int][]int{}
	clusterOrder := []int{}
	for index := range clusters {
		root := find(index)
		if _, ok := clusterIndexes[root]; !ok {
			clusterOrder = append(clusterOrder, root)
		}
		clusterIndexes[root] = append(clusterIndexes[root], index)
	}

	merged := []parallelIdentityCluster{}
	for _, root := range clusterOrder {
		combined := parallelIdentityCluster{}
		seenLocalIDs := map[string]bool{}
		for _, index := range clusterIndexes[root] {
			cluster := clusters[index]
			for _, localID := range cluster.LocalIDs {
				localID = strings.TrimSpace(localID)
				if localID == "" || seenLocalIDs[localID] {
					continue
				}
				seenLocalIDs[localID] = true
				combined.LocalIDs = append(combined.LocalIDs, localID)
			}
			if combined.CanonicalName == "" {
				combined.CanonicalName = strings.TrimSpace(cluster.CanonicalName)
			}
			if cluster.Confidence > combined.Confidence {
				combined.Confidence = cluster.Confidence
			}
			if combined.Reason == "" {
				combined.Reason = strings.TrimSpace(cluster.Reason)
			}
		}
		if len(combined.LocalIDs) > 0 {
			merged = append(merged, combined)
		}
	}
	return merged
}

func parallelIdentityCandidateNameGroups(candidates []parallelIdentityCandidate, maxItems int) [][]parallelIdentityCandidate {
	if len(candidates) == 0 {
		return nil
	}
	if maxItems < 1 {
		maxItems = 1
	}
	parent := make([]int, len(candidates))
	for index := range parent {
		parent[index] = index
	}
	var find func(int) int
	find = func(index int) int {
		if parent[index] != index {
			parent[index] = find(parent[index])
		}
		return parent[index]
	}
	union := func(a int, b int) {
		rootA := find(a)
		rootB := find(b)
		if rootA != rootB {
			parent[rootB] = rootA
		}
	}

	ownerByName := map[string]int{}
	for index, candidate := range candidates {
		for _, key := range parallelIdentityCandidateNameKeys(candidate.Character) {
			if owner, ok := ownerByName[key]; ok {
				union(owner, index)
			} else {
				ownerByName[key] = index
			}
		}
	}

	groupIndexes := map[int][]int{}
	groupOrder := []int{}
	for index := range candidates {
		root := find(index)
		if _, ok := groupIndexes[root]; !ok {
			groupOrder = append(groupOrder, root)
		}
		groupIndexes[root] = append(groupIndexes[root], index)
	}

	groups := [][]parallelIdentityCandidate{}
	for _, root := range groupOrder {
		groups = append(groups, parallelIdentityCandidateComponentGroups(candidates, groupIndexes[root], maxItems)...)
	}
	return groups
}

func parallelIdentityCandidateComponentGroups(candidates []parallelIdentityCandidate, indexes []int, maxItems int) [][]parallelIdentityCandidate {
	if len(indexes) == 0 {
		return nil
	}
	if maxItems < 1 {
		maxItems = 1
	}
	if maxItems == 1 {
		groups := make([][]parallelIdentityCandidate, 0, len(indexes))
		for _, index := range indexes {
			groups = append(groups, []parallelIdentityCandidate{candidates[index]})
		}
		return groups
	}

	anchorIndex := -1
	for _, index := range indexes {
		if strings.TrimSpace(candidates[index].Character.CharacterID) != "" {
			anchorIndex = index
			break
		}
	}
	if anchorIndex < 0 || len(indexes) <= maxItems {
		groups := [][]parallelIdentityCandidate{}
		for start := 0; start < len(indexes); start += maxItems {
			end := start + maxItems
			if end > len(indexes) {
				end = len(indexes)
			}
			group := make([]parallelIdentityCandidate, 0, end-start)
			for _, index := range indexes[start:end] {
				group = append(group, candidates[index])
			}
			groups = append(groups, group)
		}
		return groups
	}

	nonAnchorIndexes := make([]int, 0, len(indexes)-1)
	for _, index := range indexes {
		if index != anchorIndex {
			nonAnchorIndexes = append(nonAnchorIndexes, index)
		}
	}
	groups := [][]parallelIdentityCandidate{}
	capacity := maxItems - 1
	for start := 0; start < len(nonAnchorIndexes); start += capacity {
		end := start + capacity
		if end > len(nonAnchorIndexes) {
			end = len(nonAnchorIndexes)
		}
		group := make([]parallelIdentityCandidate, 0, 1+end-start)
		group = append(group, candidates[anchorIndex])
		for _, index := range nonAnchorIndexes[start:end] {
			group = append(group, candidates[index])
		}
		groups = append(groups, group)
	}
	return groups
}

func parallelIdentityCandidateNameKeys(character characters.GeneratedCharacter) []string {
	values := []string{character.CanonicalName}
	if character.FullName != nil {
		values = append(values, *character.FullName)
	}
	for _, fullName := range character.FullNameHistory {
		values = append(values, fullName.Text)
	}
	for _, alias := range character.Aliases {
		values = append(values, alias.Text)
	}
	keys := []string{}
	seen := map[string]bool{}
	for _, value := range values {
		key := normalizeParallelIdentityNameKey(value)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		keys = append(keys, key)
	}
	return keys
}

func normalizeParallelIdentityNameKey(value string) string {
	value = strings.Map(func(r rune) rune {
		if r == '　' || r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			return -1
		}
		return r
	}, strings.ToLower(strings.TrimSpace(value)))
	return value
}

func completeParallelIdentitySingletons(clusters []parallelIdentityCluster, candidates []parallelIdentityCandidate) []parallelIdentityCluster {
	allowed := map[string]bool{}
	for _, candidate := range candidates {
		localID := strings.TrimSpace(candidate.LocalID)
		if localID != "" {
			allowed[localID] = true
		}
	}

	known := map[string]bool{}
	result := []parallelIdentityCluster{}
	for _, cluster := range clusters {
		localIDs := []string{}
		for _, id := range normalizeParallelIdentityLocalIDs(cluster.LocalIDs) {
			if !allowed[id] || known[id] {
				continue
			}
			known[id] = true
			localIDs = append(localIDs, id)
		}
		if len(localIDs) == 0 {
			continue
		}
		cluster.LocalIDs = localIDs
		result = append(result, cluster)
	}
	for _, candidate := range candidates {
		localID := strings.TrimSpace(candidate.LocalID)
		if localID == "" || known[localID] {
			continue
		}
		result = append(result, parallelIdentityCluster{
			LocalIDs:      []string{localID},
			CanonicalName: candidate.Character.CanonicalName,
			Confidence:    1,
		})
	}
	return result
}

func normalizeParallelIdentityLocalIDs(values []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func buildParallelIdentityGeneratedCharacters(candidates []parallelIdentityCandidate, clusters []parallelIdentityCluster, allocator *characters.GeneratedCharacterIDAllocator) []characters.GeneratedCharacter {
	candidateByID := map[string]parallelIdentityCandidate{}
	for _, candidate := range candidates {
		candidateByID[candidate.LocalID] = candidate
	}
	generated := []characters.GeneratedCharacter{}
	for _, cluster := range clusters {
		clusterCandidates := []parallelIdentityCandidate{}
		for _, id := range cluster.LocalIDs {
			candidate, ok := candidateByID[id]
			if ok {
				clusterCandidates = append(clusterCandidates, candidate)
			}
		}
		if len(clusterCandidates) == 0 {
			continue
		}
		generated = append(generated, mergeParallelIdentityCluster(clusterCandidates, cluster, allocator))
	}
	return generated
}

func mergeParallelIdentityCluster(candidates []parallelIdentityCandidate, cluster parallelIdentityCluster, allocator *characters.GeneratedCharacterIDAllocator) characters.GeneratedCharacter {
	representativeID := parallelIdentityRepresentativeID(candidates)
	var merged characters.GeneratedCharacter
	for index, candidate := range candidates {
		item := candidate.Character
		if representativeID != "" {
			itemID := strings.TrimSpace(item.CharacterID)
			if itemID != "" && itemID != representativeID && allocator != nil {
				allocator.Retire(itemID, representativeID)
			}
			item.CharacterID = representativeID
		} else {
			item.CharacterID = ""
		}
		if index == 0 {
			merged = item
		} else {
			merged = mergeGeneratedCharacter(merged, item)
		}
	}
	if strings.TrimSpace(cluster.CanonicalName) != "" && strings.TrimSpace(merged.CanonicalName) == "" {
		merged.CanonicalName = strings.TrimSpace(cluster.CanonicalName)
		merged.CanonicalEpisodeIndex = firstNonEmptySummaryString(merged.FirstAppearanceEpisodeIndex, merged.CanonicalEpisodeIndex)
	}
	return merged
}

func parallelIdentityRepresentativeID(candidates []parallelIdentityCandidate) string {
	type candidateID struct {
		id              string
		appearanceIndex string
	}
	ids := []candidateID{}
	for _, candidate := range candidates {
		id := strings.TrimSpace(candidate.Character.CharacterID)
		if id == "" {
			continue
		}
		ids = append(ids, candidateID{
			id:              id,
			appearanceIndex: firstNonEmptySummaryString(candidate.Character.FirstAppearanceEpisodeIndex, candidate.Character.CanonicalEpisodeIndex),
		})
	}
	if len(ids) == 0 {
		return ""
	}
	sort.SliceStable(ids, func(i, j int) bool {
		diff := compareEpisodeString(ids[i].appearanceIndex, ids[j].appearanceIndex)
		if diff != 0 {
			return diff < 0
		}
		return ids[i].id < ids[j].id
	})
	return ids[0].id
}

func extractionConfigWithModel(config *store.ResolvedAIGenerationConfig, modelID string) *store.ResolvedAIGenerationConfig {
	if config == nil {
		return nil
	}
	copied := *config
	if strings.TrimSpace(modelID) != "" {
		copied.ModelID = modelID
	}
	return &copied
}

func extractionNameDiscoveryConfig(config *store.ResolvedAIGenerationConfig) *store.ResolvedAIGenerationConfig {
	if config == nil {
		return nil
	}
	return extractionConfigWithModel(config, config.ExtractionNameDiscoveryModelID)
}

func extractionBatchPromptPayload(batch extractionBatch) map[string]any {
	chunks := make([]map[string]any, 0, len(batch.Chunks))
	for _, chunk := range batch.Chunks {
		chunks = append(chunks, map[string]any{
			"episodeIndex": chunk.EpisodeIndex,
			"title":        chunk.Title,
			"chunkIndex":   chunk.ChunkIndex,
			"chunkCount":   chunk.ChunkCount,
			"text":         truncateRunes(chunk.Text, 6000),
		})
	}
	return map[string]any{
		"batchIndex":     batch.BatchIndex,
		"batchCount":     batch.BatchCount,
		"episodeIndexes": batch.EpisodeIndexes,
		"chunks":         chunks,
	}
}
