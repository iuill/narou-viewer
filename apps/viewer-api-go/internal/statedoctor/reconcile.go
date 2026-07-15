package statedoctor

import (
	"fmt"
	"math/big"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type jobMetadata struct {
	JobID   string `yaml:"job_id"`
	NovelID string `yaml:"novel_id"`
	Status  string `yaml:"status"`
}

type jobIndexMetadata struct {
	NovelID     string   `yaml:"novel_id"`
	ActiveJobID *string  `yaml:"active_job_id"`
	JobIDs      []string `yaml:"job_ids"`
}

type frontierMetadata struct {
	NovelID                   string  `yaml:"novel_id"`
	ProcessedUpToEpisodeIndex *string `yaml:"processed_up_to_episode_index"`
}

func (s *scanner) scanReconciliation() {
	s.scanJobIndexConsistency()
	s.scanCharacterTermConsistency()
	s.scanLibraryOrphans()
}

func (s *scanner) scanJobIndexConsistency() {
	jobsByNovel := map[string][]jobMetadata{}
	jobPaths, _ := filepath.Glob(filepath.Join(s.stateDir, "extraction_jobs", "*.yaml"))
	sort.Strings(jobPaths)
	for _, path := range jobPaths {
		file := s.yamlFiles[path]
		if !file.accepted {
			continue
		}
		var job jobMetadata
		if err := yaml.Unmarshal(file.raw, &job); err != nil || strings.TrimSpace(job.JobID) == "" || strings.TrimSpace(job.NovelID) == "" {
			s.add(Finding{SchemaID: "VA-EXTRACTION-JOBS", Path: s.rel(path), Kind: "typed_payload_invalid", Severity: SeverityError, Observed: "missing or invalid job_id/novel_id", Supported: "job schema v2", RecoveryHint: "運用正本を自動削除せず、対応 build または backup で復旧してください。"})
			continue
		}
		job.JobID = strings.TrimSpace(job.JobID)
		job.NovelID = strings.TrimSpace(job.NovelID)
		jobsByNovel[job.NovelID] = append(jobsByNovel[job.NovelID], job)
	}

	indexes := map[string]jobIndexMetadata{}
	indexValid := map[string]bool{}
	indexPaths := map[string]string{}
	paths, _ := filepath.Glob(filepath.Join(s.stateDir, "extraction_jobs", "index", "*.yaml"))
	sort.Strings(paths)
	for _, path := range paths {
		novelID := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		indexPaths[novelID] = path
		file := s.yamlFiles[path]
		if !file.accepted {
			continue
		}
		var index jobIndexMetadata
		if err := yaml.Unmarshal(file.raw, &index); err != nil {
			s.add(Finding{SchemaID: "VA-EXTRACTION-INDEX", Path: s.rel(path), Kind: "typed_payload_invalid", Severity: SeverityWarning, Observed: "invalid index payload", Supported: "index schema v2", RecoveryHint: "job file から derived index を rebuild してください。", RepairKind: repairJobIndex, RepairTarget: novelID})
			continue
		}
		indexes[novelID] = index
		indexValid[novelID] = true
	}

	novelIDs := map[string]bool{}
	for novelID := range jobsByNovel {
		novelIDs[novelID] = true
	}
	for novelID := range indexPaths {
		novelIDs[novelID] = true
	}
	ordered := make([]string, 0, len(novelIDs))
	for novelID := range novelIDs {
		ordered = append(ordered, novelID)
	}
	sort.Strings(ordered)
	for _, novelID := range ordered {
		path := indexPaths[novelID]
		if path == "" {
			path = filepath.Join(s.stateDir, "extraction_jobs", "index", novelID+".yaml")
		}
		jobs := jobsByNovel[novelID]
		activeIDs := []string{}
		expectedIDs := make([]string, 0, len(jobs))
		for _, job := range jobs {
			expectedIDs = append(expectedIDs, job.JobID)
			if job.Status == "queued" || job.Status == "running" {
				activeIDs = append(activeIDs, job.JobID)
			}
		}
		if len(activeIDs) > 1 {
			s.add(Finding{SchemaID: "VA-EXTRACTION-JOBS", Path: s.rel(filepath.Dir(path)), Kind: "multiple_active_jobs", Severity: SeverityError, Observed: fmt.Sprintf("%d active jobs", len(activeIDs)), Supported: "at most one active job per novel", RecoveryHint: "job 正本の状態機械を確認し、index rebuild だけで解消しないでください。"})
		}
		index := indexes[novelID]
		mismatch := !indexValid[novelID] || strings.TrimSpace(index.NovelID) != novelID || !sameStringSet(index.JobIDs, expectedIDs)
		if index.ActiveJobID != nil {
			mismatch = mismatch || !containsString(activeIDs, strings.TrimSpace(*index.ActiveJobID))
		} else if len(activeIDs) == 1 {
			mismatch = true
		}
		if !mismatch {
			continue
		}
		finding := Finding{
			SchemaID:     "VA-EXTRACTION-INDEX",
			Path:         s.rel(path),
			Kind:         "job_index_mismatch",
			Severity:     SeverityWarning,
			Observed:     "index differs from job files",
			Supported:    "derived index matches current jobs",
			RecoveryHint: "current job file を正本として index を quarantine 後に rebuild してください。",
		}
		if safeFileComponent(novelID) && len(activeIDs) <= 1 {
			finding.RepairKind = repairJobIndex
			finding.RepairTarget = novelID
		}
		s.add(finding)
	}
}

func (s *scanner) scanCharacterTermConsistency() {
	events := map[string]frontierMetadata{}
	eventValid := map[string]bool{}
	eventPaths, _ := filepath.Glob(filepath.Join(s.stateDir, "character_events", "*.yaml"))
	for _, path := range eventPaths {
		novelID := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		file := s.yamlFiles[path]
		if !file.accepted {
			continue
		}
		var metadata frontierMetadata
		if err := yaml.Unmarshal(file.raw, &metadata); err == nil {
			events[novelID] = metadata
			eventValid[novelID] = true
		}
	}

	profilePaths, _ := filepath.Glob(filepath.Join(s.stateDir, "character_profiles", "*.yaml"))
	for _, path := range profilePaths {
		novelID := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		file := s.yamlFiles[path]
		if !eventValid[novelID] {
			s.add(Finding{SchemaID: "VA-CHAR-PROFILES", Path: s.rel(path), Kind: "profiles_without_events", Severity: SeverityWarning, Observed: "profile exists without character events", Supported: "events are the generated source of truth", RecoveryHint: "profiles-only restore を避け、対応 events を restore するか heuristic 由来かを確認してください。"})
			continue
		}
		if file.exists && !file.accepted {
			s.add(Finding{SchemaID: "VA-CHAR-PROFILES", Path: s.rel(path), Kind: "character_profile_rebuildable", Severity: SeverityWarning, Observed: "derived profile is incompatible", Supported: "schema v1 materialized from events", RecoveryHint: "profile を quarantine し、current events から rebuild できます。", RepairKind: repairCharacterProfile, RepairTarget: novelID})
		}
	}

	termPaths, _ := filepath.Glob(filepath.Join(s.stateDir, "term_profiles", "*.yaml"))
	for _, path := range termPaths {
		novelID := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		file := s.yamlFiles[path]
		if !file.accepted {
			continue
		}
		var term frontierMetadata
		if err := yaml.Unmarshal(file.raw, &term); err != nil || term.ProcessedUpToEpisodeIndex == nil {
			continue
		}
		event, ok := events[novelID]
		if !ok || event.ProcessedUpToEpisodeIndex == nil {
			s.add(Finding{SchemaID: "VA-TERM-PROFILES", Path: s.rel(path), Kind: "term_frontier_without_character_frontier", Severity: SeverityWarning, Observed: strings.TrimSpace(*term.ProcessedUpToEpisodeIndex), Supported: "term frontier <= character commit frontier", RecoveryHint: "同じ snapshot generation の character events と term profiles を restore してください。"})
			continue
		}
		if compareEpisodeIndex(*term.ProcessedUpToEpisodeIndex, *event.ProcessedUpToEpisodeIndex) > 0 {
			s.add(Finding{SchemaID: "VA-TERM-PROFILES", Path: s.rel(path), Kind: "frontier_inversion", Severity: SeverityError, Observed: strings.TrimSpace(*term.ProcessedUpToEpisodeIndex), Supported: "<= " + strings.TrimSpace(*event.ProcessedUpToEpisodeIndex), RecoveryHint: "自動切詰めせず、同一 snapshot の events / terms を復旧して生成 commit frontier を確認してください。"})
		}
	}
}

func (s *scanner) scanLibraryOrphans() {
	if !s.libraryReadable {
		return
	}
	readingPath := filepath.Join(s.stateDir, "reading_state.yaml")
	if file := s.yamlFiles[readingPath]; file.accepted {
		var document struct {
			Novels map[string]any `yaml:"novels"`
		}
		if err := yaml.Unmarshal(file.raw, &document); err == nil {
			for novelID := range document.Novels {
				s.addOrphanFinding("VA-READING", readingPath, novelID)
			}
		}
	}
	bookmarksPath := filepath.Join(s.stateDir, "bookmarks.yaml")
	if file := s.yamlFiles[bookmarksPath]; file.accepted {
		var document struct {
			Bookmarks []struct {
				NovelID string `yaml:"novel_id"`
			} `yaml:"bookmarks"`
		}
		if err := yaml.Unmarshal(file.raw, &document); err == nil {
			seen := map[string]bool{}
			for _, bookmark := range document.Bookmarks {
				novelID := strings.TrimSpace(bookmark.NovelID)
				if !seen[novelID] {
					s.addOrphanFinding("VA-BOOKMARKS", bookmarksPath, novelID)
					seen[novelID] = true
				}
			}
		}
	}
}

func (s *scanner) addOrphanFinding(schemaID string, path string, novelID string) {
	novelID = strings.TrimSpace(novelID)
	if novelID == "" || s.libraryNovelIDs[novelID] {
		return
	}
	s.add(Finding{SchemaID: schemaID, Path: s.rel(path), Kind: "orphan_novel_state", Severity: SeverityWarning, Observed: novelID, Supported: "novel exists in NF-LIBRARY", RecoveryHint: "自動 prune せず、library restore / backend 切替 / 利用者 state の保持方針を確認してください。"})
}

func sameStringSet(left []string, right []string) bool {
	leftSet := map[string]bool{}
	rightSet := map[string]bool{}
	for _, value := range left {
		if value = strings.TrimSpace(value); value != "" {
			leftSet[value] = true
		}
	}
	for _, value := range right {
		if value = strings.TrimSpace(value); value != "" {
			rightSet[value] = true
		}
	}
	if len(leftSet) != len(rightSet) {
		return false
	}
	for value := range leftSet {
		if !rightSet[value] {
			return false
		}
	}
	return true
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func safeFileComponent(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && value != "." && value != ".." && filepath.Base(value) == value && !strings.ContainsAny(value, `/\\`)
}

func compareEpisodeIndex(left string, right string) int {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	leftNumber, leftOK := new(big.Int).SetString(left, 10)
	rightNumber, rightOK := new(big.Int).SetString(right, 10)
	if leftOK && rightOK {
		return leftNumber.Cmp(rightNumber)
	}
	return strings.Compare(left, right)
}
