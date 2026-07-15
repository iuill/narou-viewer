package ai

import (
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"narou-viewer/apps/viewer-api-go/internal/ai/usagemigration"

	_ "modernc.org/sqlite"
)

const usageRunListLimit = 50

var usageWriteMu sync.Mutex
var usageMigrationMu sync.Mutex

func LoadUsage(dbPath string) (UsageResponse, bool, error) {
	db, ok, err := openUsageDB(dbPath)
	if err != nil || !ok {
		return UsageResponse{}, ok, err
	}
	defer db.Close()

	response, err := loadUsageSummary(db)
	if err != nil {
		return UsageResponse{}, false, err
	}
	response.Runs = []UsageRun{}

	rows, err := db.Query(`
		SELECT
			r.run_id, r.feature, r.workflow_name, r.status, r.started_at, r.finished_at, r.elapsed_ms,
			r.novel_id, r.novel_title, r.current_episode_index, r.model_id, r.profile_id, r.profile_label,
			r.generation_mode, r.answer_chars, r.request_count, r.input_tokens, r.output_tokens, r.total_tokens,
			r.cached_input_tokens, r.reasoning_output_tokens, r.total_cost, r.tool_call_count, r.tool_result_count,
			CASE WHEN s.run_id IS NULL THEN 0 ELSE 1 END AS has_snapshot,
			r.error_message
		FROM ai_usage_runs r
		LEFT JOIN ai_usage_run_snapshots s ON s.run_id = r.run_id
		ORDER BY r.started_at DESC
		LIMIT ?
	`, usageRunListLimit)
	if err != nil {
		return UsageResponse{}, false, err
	}
	runs := []UsageRun{}
	for rows.Next() {
		run, err := scanUsageRun(rows)
		if err != nil {
			_ = rows.Close()
			return UsageResponse{}, false, err
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return UsageResponse{}, false, err
	}
	if err := rows.Close(); err != nil {
		return UsageResponse{}, false, err
	}
	// VA-AI-USAGE intentionally uses one SQLite connection. Release the run
	// result set before loading child requests so the nested queries cannot
	// wait forever for the connection held by rows.
	for _, run := range runs {
		requests, err := loadUsageRequests(db, run.RunID)
		if err != nil {
			return UsageResponse{}, false, err
		}
		run.Requests = requests
		response.Runs = append(response.Runs, run)
	}
	return response, len(response.Runs) > 0, nil
}

func SaveUsageRun(dbPath string, run UsageRun) error {
	usageWriteMu.Lock()
	defer usageWriteMu.Unlock()
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return err
	}
	db, err := openUsageWriteDB(dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`
		INSERT OR REPLACE INTO ai_usage_runs (
			run_id, feature, workflow_name, status, started_at, finished_at, elapsed_ms,
			novel_id, novel_title, current_episode_index, model_id, profile_id, profile_label,
			generation_mode, answer_chars, request_count, input_tokens, output_tokens, total_tokens,
			cached_input_tokens, reasoning_output_tokens, total_cost, tool_call_count, tool_result_count,
			error_message
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		run.RunID, run.Feature, run.WorkflowName, run.Status, run.StartedAt, run.FinishedAt, run.ElapsedMs,
		run.NovelID, run.NovelTitle, run.CurrentEpisodeIndex, run.ModelID, run.ProfileID, run.ProfileLabel,
		run.GenerationMode, run.AnswerChars, run.RequestCount, run.InputTokens, run.OutputTokens, run.TotalTokens,
		run.CachedInputTokens, run.ReasoningOutputTokens, run.TotalCost, run.ToolCallCount, run.ToolResultCount,
		run.ErrorMessage,
	); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM ai_usage_requests WHERE run_id = ?`, run.RunID); err != nil {
		return err
	}
	for _, request := range run.Requests {
		toolNames, _ := json.Marshal(normalizeStringList(request.ToolNames))
		toolSummaries, _ := json.Marshal(normalizeStringList(request.ToolSummaries))
		if _, err := tx.Exec(`
			INSERT INTO ai_usage_requests (
				run_id, request_index, kind, parent_request_index, tool_names, tool_summaries,
				input_tokens, output_tokens, total_tokens, cached_input_tokens, reasoning_output_tokens, cost
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`,
			run.RunID, request.RequestIndex, request.Kind, request.ParentRequestIndex, string(toolNames), string(toolSummaries),
			request.InputTokens, request.OutputTokens, request.TotalTokens, request.CachedInputTokens, request.ReasoningOutputTokens, request.Cost,
		); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`DELETE FROM ai_usage_run_snapshots WHERE run_id = ?`, run.RunID); err != nil {
		return err
	}
	if run.Snapshot != nil {
		raw, err := json.Marshal(run.Snapshot)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`
			INSERT OR REPLACE INTO ai_usage_run_snapshots (run_id, snapshot_json)
			VALUES (?, ?)
		`, run.RunID, string(raw)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func PruneUsageByNovelID(dbPath string, novelID string) (int, error) {
	usageWriteMu.Lock()
	defer usageWriteMu.Unlock()

	novelID = strings.TrimSpace(novelID)
	if novelID == "" {
		return 0, nil
	}
	info, err := os.Stat(dbPath)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if info.Size() == 0 {
		return 0, nil
	}
	db, err := openUsageWriteDB(dbPath)
	if err != nil {
		return 0, err
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	for _, statement := range []string{
		`DELETE FROM ai_usage_requests WHERE run_id IN (SELECT run_id FROM ai_usage_runs WHERE novel_id = ?)`,
		`DELETE FROM ai_usage_run_snapshots WHERE run_id IN (SELECT run_id FROM ai_usage_runs WHERE novel_id = ?)`,
	} {
		if _, err := tx.Exec(statement, novelID); err != nil {
			return 0, err
		}
	}
	deleted, err := tx.Exec(`DELETE FROM ai_usage_runs WHERE novel_id = ?`, novelID)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	rowsAffected, err := deleted.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(rowsAffected), nil
}

func PreflightUsagePrune(dbPath string) error {
	info, err := os.Stat(dbPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Size() == 0 {
		return nil
	}
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath)+"?mode=ro&_pragma=busy_timeout(5000)")
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	return usagemigration.Preflight(db, dbPath)
}

func usageSQLiteDSN(dbPath string) string {
	return "file:" + filepath.ToSlash(dbPath) + "?_pragma=busy_timeout(5000)"
}

func usageReadOnlySQLiteDSN(dbPath string) string {
	return "file:" + filepath.ToSlash(dbPath) + "?mode=ro&_pragma=query_only(1)&_pragma=busy_timeout(5000)"
}

// EnsureUsageDB applies supported migrations before the HTTP server starts.
// Missing and zero-length databases remain lazy so unused installations do not
// create AI usage state during startup.
func EnsureUsageDB(dbPath string) error {
	info, err := os.Stat(dbPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Size() == 0 {
		return nil
	}
	db, err := openUsageWriteDB(dbPath)
	if err != nil {
		return err
	}
	return db.Close()
}

func ensureUsageDBFileMode(dbPath string) error {
	file, err := os.OpenFile(dbPath, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Chmod(dbPath, 0o600)
}

func openUsageWriteDB(dbPath string) (*sql.DB, error) {
	usageMigrationMu.Lock()
	defer usageMigrationMu.Unlock()

	if _, err := os.Stat(dbPath); errors.Is(err, os.ErrNotExist) {
		if err := ensureUsageDBFileMode(dbPath); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", usageSQLiteDSN(dbPath))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := usagemigration.Run(db, dbPath); err != nil {
		db.Close()
		return nil, err
	}
	if err := os.Chmod(dbPath, 0o600); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func loadUsageSummary(db *sql.DB) (UsageResponse, error) {
	var summary UsageSummary
	err := db.QueryRow(`
		SELECT
			COUNT(*) AS run_count,
			COALESCE(SUM(request_count), 0),
			COALESCE(SUM(input_tokens), 0),
			COALESCE(SUM(output_tokens), 0),
			COALESCE(SUM(total_tokens), 0),
			COALESCE(SUM(cached_input_tokens), 0),
			COALESCE(SUM(reasoning_output_tokens), 0),
			COALESCE(SUM(total_cost), 0)
		FROM ai_usage_runs
	`).Scan(
		&summary.RunCount,
		&summary.RequestCount,
		&summary.InputTokens,
		&summary.OutputTokens,
		&summary.TotalTokens,
		&summary.CachedInputTokens,
		&summary.ReasoningOutputTokens,
		&summary.TotalCost,
	)
	if err != nil {
		return UsageResponse{}, err
	}
	if summary.RunCount > 0 {
		summary.AverageTotalTokens = float64(summary.TotalTokens) / float64(summary.RunCount)
	}
	return UsageResponse{Summary: summary, Runs: []UsageRun{}}, nil
}

func LoadUsageRun(dbPath string, runID string) (UsageRun, bool, error) {
	db, ok, err := openUsageDB(dbPath)
	if err != nil || !ok {
		return UsageRun{}, false, err
	}
	defer db.Close()

	row := db.QueryRow(`
		SELECT
			r.run_id, r.feature, r.workflow_name, r.status, r.started_at, r.finished_at, r.elapsed_ms,
			r.novel_id, r.novel_title, r.current_episode_index, r.model_id, r.profile_id, r.profile_label,
			r.generation_mode, r.answer_chars, r.request_count, r.input_tokens, r.output_tokens, r.total_tokens,
			r.cached_input_tokens, r.reasoning_output_tokens, r.total_cost, r.tool_call_count, r.tool_result_count,
			CASE WHEN s.run_id IS NULL THEN 0 ELSE 1 END AS has_snapshot,
			r.error_message
		FROM ai_usage_runs r
		LEFT JOIN ai_usage_run_snapshots s ON s.run_id = r.run_id
		WHERE r.run_id = ?
	`, runID)
	run, err := scanUsageRun(row)
	if errors.Is(err, sql.ErrNoRows) {
		return UsageRun{}, false, nil
	}
	if err != nil {
		return UsageRun{}, false, err
	}
	requests, err := loadUsageRequests(db, run.RunID)
	if err != nil {
		return UsageRun{}, false, err
	}
	run.Requests = requests
	snapshot, err := loadUsageSnapshot(db, run.RunID)
	if err != nil {
		return UsageRun{}, false, err
	}
	run.Snapshot = snapshot
	return run, true, nil
}

func openUsageDB(dbPath string) (*sql.DB, bool, error) {
	info, err := os.Stat(dbPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	} else if err != nil {
		return nil, false, err
	}
	if info.Size() == 0 {
		return nil, false, nil
	}
	db, err := sql.Open("sqlite", usageReadOnlySQLiteDSN(dbPath))
	if err != nil {
		return nil, false, err
	}
	db.SetMaxOpenConns(1)
	if err := usagemigration.Preflight(db, dbPath); err != nil {
		db.Close()
		return nil, false, err
	}
	return db, true, nil
}

func loadUsageRequests(db *sql.DB, runID string) ([]UsageRequest, error) {
	rows, err := db.Query(`
		SELECT request_index, kind, parent_request_index, tool_names, tool_summaries,
			input_tokens, output_tokens, total_tokens, cached_input_tokens, reasoning_output_tokens, cost
		FROM ai_usage_requests
		WHERE run_id = ?
		ORDER BY request_index ASC
	`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	requests := []UsageRequest{}
	for rows.Next() {
		var request UsageRequest
		var parentRequestIndex sql.NullInt64
		var toolNamesJSON string
		var toolSummariesJSON string
		if err := rows.Scan(
			&request.RequestIndex,
			&request.Kind,
			&parentRequestIndex,
			&toolNamesJSON,
			&toolSummariesJSON,
			&request.InputTokens,
			&request.OutputTokens,
			&request.TotalTokens,
			&request.CachedInputTokens,
			&request.ReasoningOutputTokens,
			&request.Cost,
		); err != nil {
			return nil, err
		}
		if parentRequestIndex.Valid {
			value := int(parentRequestIndex.Int64)
			request.ParentRequestIndex = &value
		}
		request.ToolNames = decodeStringList(toolNamesJSON)
		request.ToolSummaries = decodeStringList(toolSummariesJSON)
		requests = append(requests, request)
	}
	return requests, rows.Err()
}

type usageRunScanner interface {
	Scan(dest ...any) error
}

func scanUsageRun(scanner usageRunScanner) (UsageRun, error) {
	var run UsageRun
	var novelID sql.NullString
	var novelTitle sql.NullString
	var currentEpisodeIndex sql.NullString
	var modelID sql.NullString
	var profileID sql.NullString
	var profileLabel sql.NullString
	var errorMessage sql.NullString
	var hasSnapshot int
	if err := scanner.Scan(
		&run.RunID,
		&run.Feature,
		&run.WorkflowName,
		&run.Status,
		&run.StartedAt,
		&run.FinishedAt,
		&run.ElapsedMs,
		&novelID,
		&novelTitle,
		&currentEpisodeIndex,
		&modelID,
		&profileID,
		&profileLabel,
		&run.GenerationMode,
		&run.AnswerChars,
		&run.RequestCount,
		&run.InputTokens,
		&run.OutputTokens,
		&run.TotalTokens,
		&run.CachedInputTokens,
		&run.ReasoningOutputTokens,
		&run.TotalCost,
		&run.ToolCallCount,
		&run.ToolResultCount,
		&hasSnapshot,
		&errorMessage,
	); err != nil {
		return UsageRun{}, err
	}
	run.NovelID = nullStringPtr(novelID)
	run.NovelTitle = nullStringPtr(novelTitle)
	run.CurrentEpisodeIndex = nullStringPtr(currentEpisodeIndex)
	run.ModelID = nullStringPtr(modelID)
	run.ProfileID = nullStringPtr(profileID)
	run.ProfileLabel = nullStringPtr(profileLabel)
	run.ErrorMessage = nullStringPtr(errorMessage)
	run.HasSnapshot = hasSnapshot == 1
	run.Requests = []UsageRequest{}
	return run, nil
}

func loadUsageSnapshot(db *sql.DB, runID string) (any, error) {
	var raw string
	err := db.QueryRow(`SELECT snapshot_json FROM ai_usage_run_snapshots WHERE run_id = ?`, runID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return nil, nil
	}
	return decoded, nil
}

func nullStringPtr(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

func decodeStringList(value string) []string {
	var decoded []string
	if err := json.Unmarshal([]byte(value), &decoded); err != nil {
		return []string{}
	}
	return normalizeStringList(decoded)
}

func normalizeStringList(value []string) []string {
	if value == nil {
		return []string{}
	}
	return value
}
