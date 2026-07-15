package statedoctor

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"narou-viewer/apps/viewer-api-go/internal/application/readertextcache"
	"narou-viewer/apps/viewer-api-go/internal/characters"
	"narou-viewer/apps/viewer-api-go/internal/extraction"
)

func Apply(ctx context.Context, dataDir string, findingIDs []string) (Report, error) {
	if len(findingIDs) == 0 {
		return Report{}, errors.New("--apply requires at least one --finding ID")
	}
	report, err := Scan(ctx, dataDir)
	if err != nil {
		return Report{}, err
	}
	available := map[string]Finding{}
	for _, finding := range report.Findings {
		available[finding.ID] = finding
	}
	selected := make([]Finding, 0, len(findingIDs))
	seen := map[string]bool{}
	for _, id := range findingIDs {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		finding, ok := available[id]
		if !ok {
			return report, fmt.Errorf("finding ID is not present in the current dry-run report: %s", id)
		}
		if finding.RepairKind == "" {
			return report, fmt.Errorf("finding is diagnostic-only and cannot be auto-repaired: %s", id)
		}
		seen[id] = true
		selected = append(selected, finding)
	}
	sort.SliceStable(selected, func(i, j int) bool { return selected[i].ID < selected[j].ID })
	stateDir := filepath.Join(filepath.Clean(dataDir), "state")
	applied := make([]string, 0, len(selected))
	for _, finding := range selected {
		switch finding.RepairKind {
		case repairJobIndex:
			if !safeFileComponent(finding.RepairTarget) {
				return report, fmt.Errorf("unsafe job index repair target for %s", finding.ID)
			}
			if err := extraction.RebuildJobIndex(stateDir, finding.RepairTarget); err != nil {
				return report, fmt.Errorf("repair %s: %w", finding.ID, err)
			}
		case repairCharacterProfile:
			if !safeFileComponent(finding.RepairTarget) {
				return report, fmt.Errorf("unsafe character profile repair target for %s", finding.ID)
			}
			materialized, err := characters.MaterializeGeneratedSummary(stateDir, finding.RepairTarget)
			if err != nil {
				return report, fmt.Errorf("repair %s: %w", finding.ID, err)
			}
			if !materialized {
				return report, fmt.Errorf("repair %s: current character events are not materializable", finding.ID)
			}
		case repairReaderSearch:
			if _, err := readertextcache.New(stateDir).Rebuild(ctx); err != nil {
				return report, fmt.Errorf("repair %s: %w", finding.ID, err)
			}
		default:
			return report, fmt.Errorf("unsupported repair kind %q for %s", finding.RepairKind, finding.ID)
		}
		applied = append(applied, finding.ID)
	}
	repaired, err := Scan(ctx, dataDir)
	if err != nil {
		return report, err
	}
	repaired.Applied = applied
	return repaired, nil
}
