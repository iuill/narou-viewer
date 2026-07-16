package statedoctor

import (
	"fmt"
	"sort"
	"strings"
)

type Severity string

const (
	SeverityInfo    Severity = "info"
	SeverityWarning Severity = "warning"
	SeverityError   Severity = "error"
)

type Finding struct {
	ID           string   `json:"id"`
	SchemaID     string   `json:"schema_id"`
	Path         string   `json:"path"`
	Kind         string   `json:"kind"`
	Severity     Severity `json:"severity"`
	Observed     string   `json:"observed,omitempty"`
	Supported    string   `json:"supported,omitempty"`
	RecoveryHint string   `json:"recovery_hint,omitempty"`
	RepairKind   string   `json:"repair_kind,omitempty"`
	RepairTarget string   `json:"repair_target,omitempty"`
}

type Summary struct {
	Inventory  int `json:"inventory"`
	Warnings   int `json:"warnings"`
	Errors     int `json:"errors"`
	Repairable int `json:"repairable"`
}

type Report struct {
	DataDir  string    `json:"data_dir"`
	Findings []Finding `json:"findings"`
	Applied  []string  `json:"applied,omitempty"`
	Summary  Summary   `json:"summary"`
}

func (r Report) HasIssues() bool {
	return r.Summary.Warnings > 0 || r.Summary.Errors > 0
}

func (r *Report) finalize() {
	sort.SliceStable(r.Findings, func(i, j int) bool {
		left := r.Findings[i]
		right := r.Findings[j]
		if left.Path != right.Path {
			return left.Path < right.Path
		}
		if left.SchemaID != right.SchemaID {
			return left.SchemaID < right.SchemaID
		}
		if left.Kind != right.Kind {
			return left.Kind < right.Kind
		}
		return left.ID < right.ID
	})
	r.Summary = Summary{}
	for _, finding := range r.Findings {
		switch finding.Severity {
		case SeverityWarning:
			r.Summary.Warnings++
		case SeverityError:
			r.Summary.Errors++
		default:
			r.Summary.Inventory++
		}
		if finding.RepairKind != "" {
			r.Summary.Repairable++
		}
	}
}

func Human(report Report) string {
	var output strings.Builder
	for _, finding := range report.Findings {
		fmt.Fprintf(&output, "[%s] %s %s: %s", finding.Severity, finding.SchemaID, finding.Path, finding.Kind)
		if finding.Observed != "" || finding.Supported != "" {
			fmt.Fprintf(&output, " (observed=%s supported=%s)", emptyDash(finding.Observed), emptyDash(finding.Supported))
		}
		if finding.RecoveryHint != "" {
			fmt.Fprintf(&output, "\n  recovery: %s", finding.RecoveryHint)
		}
		if finding.RepairKind != "" {
			fmt.Fprintf(&output, "\n  repair: --apply --finding %s", finding.ID)
		}
		output.WriteByte('\n')
	}
	fmt.Fprintf(
		&output,
		"summary: inventory=%d warnings=%d errors=%d repairable=%d\n",
		report.Summary.Inventory,
		report.Summary.Warnings,
		report.Summary.Errors,
		report.Summary.Repairable,
	)
	return output.String()
}

func emptyDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}
