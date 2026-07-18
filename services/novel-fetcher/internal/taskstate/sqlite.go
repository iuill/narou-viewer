package taskstate

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrTaskNotFound      = errors.New("task not found")
	ErrTaskStateConflict = errors.New("task state conflict")
	ErrTaskAlreadyActive = errors.New("task already active")
	ErrStaleTaskAttempt  = errors.New("stale task attempt")
)

type SQLiteRepository struct {
	db     *sql.DB
	readDB *sql.DB
}

func NewSQLiteRepository(db *sql.DB) *SQLiteRepository {
	return NewSQLiteRepositoryWithReader(db, db)
}

// NewSQLiteRepositoryWithReader keeps all mutations on the process-wide
// single writer connection while serving independent WAL snapshots from a
// read-only pool.
func NewSQLiteRepositoryWithReader(db *sql.DB, readDB *sql.DB) *SQLiteRepository {
	return &SQLiteRepository{db: db, readDB: readDB}
}

func (r *SQLiteRepository) Enqueue(ctx context.Context, tasks []*Task) (EnqueueResult, error) {
	if len(tasks) == 0 {
		return EnqueueResult{}, nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return EnqueueResult{}, err
	}
	defer func() { _ = tx.Rollback() }()

	result := EnqueueResult{Tasks: make([]*Task, 0, len(tasks))}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, task := range tasks {
		request, dedupeKey, fingerprint, err := RequestForTask(task)
		if err != nil {
			return EnqueueResult{}, err
		}
		requestJSONBytes, err := json.Marshal(request)
		if err != nil {
			return EnqueueResult{}, err
		}
		var existingID string
		var existingFingerprint string
		var existingStatus Status
		err = tx.QueryRowContext(ctx, `SELECT task_id, request_fingerprint, status FROM fetch_tasks WHERE dedupe_key = ? AND status IN ('queued', 'running', 'paused', 'interrupted')`, dedupeKey).Scan(&existingID, &existingFingerprint, &existingStatus)
		if err == nil {
			if existingFingerprint != fingerprint {
				return EnqueueResult{}, fmt.Errorf("%w: resource %s is already reserved by task %s", ErrTaskAlreadyActive, dedupeKey, existingID)
			}
			stored, found, getErr := r.getTx(ctx, tx, existingID)
			if getErr != nil || !found {
				if getErr == nil {
					getErr = ErrTaskNotFound
				}
				return EnqueueResult{}, getErr
			}
			*task = *stored
			result.Tasks = append(result.Tasks, stored)
			result.DeduplicatedIDs = append(result.DeduplicatedIDs, existingID)
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return EnqueueResult{}, err
		}
		primaryWorkID := task.WorkID
		if primaryWorkID > 0 {
			var reservedBy string
			err = tx.QueryRowContext(ctx, `SELECT task_id FROM fetch_tasks WHERE primary_work_id = ? AND status IN ('queued', 'running', 'paused', 'interrupted') LIMIT 1`, primaryWorkID).Scan(&reservedBy)
			if err == nil {
				return EnqueueResult{}, fmt.Errorf("%w: work %d is already reserved by task %s", ErrTaskAlreadyActive, primaryWorkID, reservedBy)
			}
			if !errors.Is(err, sql.ErrNoRows) {
				return EnqueueResult{}, err
			}
		}
		if task.ID == "" {
			task.ID = NewTaskID(task.Kind)
		}
		if task.CreatedAt.IsZero() {
			task.CreatedAt = time.Now().UTC()
		}
		warningsJSON, _ := json.Marshal(task.Warnings)
		if len(task.Warnings) == 0 {
			warningsJSON = []byte("[]")
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO fetch_tasks (
				task_id, request_version, kind, request_json, status, requested_action,
				dedupe_key, request_fingerprint, primary_work_id, target_label, phase,
				current_step, total_steps, saved_episode_count, failed_episode_id, resume_episode_id,
				message, warnings_json, error_message, attempt_count, execution_committed,
				created_at, last_enqueued_at, started_at, updated_at, paused_at, interrupted_at, finished_at
			) VALUES (?, ?, ?, ?, 'queued', '', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, 0, ?, ?, '', ?, '', '', '')
		`, task.ID, CurrentRequestVersion, task.Kind, string(requestJSONBytes), dedupeKey, fingerprint, primaryWorkID,
			task.TargetLabel, task.Phase, task.CurrentStep, task.TotalSteps, task.SavedEpisodeCount,
			task.FailedEpisodeID, task.ResumeEpisodeID, task.Message, string(warningsJSON), task.ErrorMessage,
			task.CreatedAt.UTC().Format(time.RFC3339Nano), now, now)
		if err != nil {
			return EnqueueResult{}, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO fetch_task_queue(task_id, enqueued_at) VALUES (?, ?)`, task.ID, now); err != nil {
			return EnqueueResult{}, err
		}
		task.Status = StatusQueued
		task.RequestedAction = RequestedActionNone
		task.AttemptCount = 0
		task.DedupeKey = dedupeKey
		task.RequestFingerprint = fingerprint
		task.UpdatedAt, _ = parseTime(now)
		result.Tasks = append(result.Tasks, task)
	}
	if err := tx.Commit(); err != nil {
		return EnqueueResult{}, err
	}
	return result, nil
}

func (r *SQLiteRepository) ClaimNext(ctx context.Context, now time.Time) (*Task, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	var taskID string
	err = tx.QueryRowContext(ctx, `SELECT task_id FROM fetch_task_queue ORDER BY seq ASC LIMIT 1`).Scan(&taskID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var status Status
	if err := tx.QueryRowContext(ctx, `SELECT status FROM fetch_tasks WHERE task_id = ?`, taskID).Scan(&status); err != nil {
		return nil, err
	}
	if status != StatusQueued {
		return nil, fmt.Errorf("%w: queued task %s has status %s", ErrTaskStateConflict, taskID, status)
	}
	stamp := now.UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `UPDATE fetch_tasks SET status = 'running', requested_action = '', attempt_count = attempt_count + 1, execution_committed = 0, started_at = ?, updated_at = ? WHERE task_id = ? AND status = 'queued'`, stamp, stamp, taskID); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM fetch_task_queue WHERE task_id = ?`, taskID); err != nil {
		return nil, err
	}
	task, found, err := r.getTx(ctx, tx, taskID)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrTaskNotFound
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return task, nil
}

func (r *SQLiteRepository) Summary(ctx context.Context, recentLimit int) (Summary, error) {
	if recentLimit <= 0 {
		recentLimit = 20
	}
	tx, err := r.readDB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return Summary{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var summary Summary
	if summary.Current, err = r.oneByStatus(ctx, tx, StatusRunning); err != nil {
		return Summary{}, err
	}
	if summary.Queued, err = r.list(ctx, tx, `status = 'queued' ORDER BY (SELECT seq FROM fetch_task_queue WHERE task_id = fetch_tasks.task_id) ASC`); err != nil {
		return Summary{}, err
	}
	for index, task := range summary.Queued {
		position := index + 1
		task.QueuePosition = &position
	}
	if summary.Paused, err = r.list(ctx, tx, `status = 'paused' ORDER BY updated_at DESC LIMIT ?`, recentLimit); err != nil {
		return Summary{}, err
	}
	if summary.Interrupted, err = r.list(ctx, tx, `status = 'interrupted' ORDER BY updated_at DESC LIMIT ?`, recentLimit); err != nil {
		return Summary{}, err
	}
	if summary.RecentCompleted, err = r.list(ctx, tx, `status = 'succeeded' ORDER BY finished_at DESC LIMIT ?`, recentLimit); err != nil {
		return Summary{}, err
	}
	if summary.RecentFailed, err = r.list(ctx, tx, `status IN ('failed', 'canceled') ORDER BY finished_at DESC LIMIT ?`, recentLimit); err != nil {
		return Summary{}, err
	}
	if err := tx.QueryRowContext(ctx, `
		SELECT
			COALESCE(SUM(CASE WHEN status = 'succeeded' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status IN ('failed', 'canceled') THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status = 'canceled' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status = 'paused' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status = 'interrupted' THEN 1 ELSE 0 END), 0)
		FROM fetch_tasks
	`).Scan(&summary.CompletedCount, &summary.FailedCount, &summary.CanceledCount, &summary.PausedCount, &summary.InterruptedCount); err != nil {
		return Summary{}, err
	}
	if err := tx.Commit(); err != nil {
		return Summary{}, err
	}
	return summary, nil
}

func (r *SQLiteRepository) QueueCounts(ctx context.Context) (QueueCounts, error) {
	var queued, running, paused, interrupted int
	if err := r.readDB.QueryRowContext(ctx, `
		SELECT
			COALESCE(SUM(CASE WHEN status = 'queued' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status = 'running' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status = 'paused' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status = 'interrupted' THEN 1 ELSE 0 END), 0)
		FROM fetch_tasks
	`).Scan(&queued, &running, &paused, &interrupted); err != nil {
		return QueueCounts{}, err
	}
	return QueueCounts{Total: queued + running, Queued: queued, Running: running > 0, Paused: paused, Interrupted: interrupted}, nil
}

func (r *SQLiteRepository) HasQueuedTasks(ctx context.Context) (bool, error) {
	var count int
	err := r.readDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM fetch_tasks WHERE status = 'queued'`).Scan(&count)
	return count > 0, err
}

func (r *SQLiteRepository) Get(ctx context.Context, taskID string) (*Task, bool, error) {
	return r.getTx(ctx, r.readDB, taskID)
}

func (r *SQLiteRepository) RequestPause(ctx context.Context, taskID string) (ControlResult, error) {
	return r.requestAction(ctx, taskID, RequestedActionPause)
}

func (r *SQLiteRepository) RequestCancel(ctx context.Context, taskID string) (ControlResult, error) {
	return r.requestAction(ctx, taskID, RequestedActionCancel)
}

func (r *SQLiteRepository) RequestResume(ctx context.Context, taskID string) (ControlResult, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return ControlResult{}, err
	}
	defer func() { _ = tx.Rollback() }()
	task, found, err := r.getTx(ctx, tx, taskID)
	if err != nil {
		return ControlResult{}, err
	}
	if !found {
		return ControlResult{}, ErrTaskNotFound
	}
	if task.Status == StatusQueued || task.Status == StatusRunning {
		return ControlResult{Task: task, Changed: false}, nil
	}
	if task.Status == StatusCanceled || task.Status == StatusSucceeded {
		return ControlResult{Task: task, Changed: false}, fmt.Errorf("%w: task %s cannot resume from %s", ErrTaskStateConflict, taskID, task.Status)
	}
	var dedupeConflict string
	err = tx.QueryRowContext(ctx, `SELECT task_id FROM fetch_tasks WHERE dedupe_key = ? AND task_id != ? AND status IN ('queued', 'running', 'paused', 'interrupted') LIMIT 1`, task.DedupeKey, taskID).Scan(&dedupeConflict)
	if err == nil {
		return ControlResult{}, fmt.Errorf("%w: resource %s is already reserved by task %s", ErrTaskAlreadyActive, task.DedupeKey, dedupeConflict)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return ControlResult{}, err
	}
	if task.WorkID > 0 {
		var conflict string
		err = tx.QueryRowContext(ctx, `SELECT task_id FROM fetch_tasks WHERE primary_work_id = ? AND task_id != ? AND status IN ('queued', 'running', 'paused', 'interrupted') LIMIT 1`, task.WorkID, taskID).Scan(&conflict)
		if err == nil {
			return ControlResult{}, fmt.Errorf("%w: work is reserved by task %s", ErrTaskAlreadyActive, conflict)
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return ControlResult{}, err
		}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `UPDATE fetch_tasks SET status = 'queued', requested_action = '', execution_committed = 0, finished_at = '', paused_at = '', interrupted_at = '', last_enqueued_at = ?, updated_at = ? WHERE task_id = ?`, now, now, taskID); err != nil {
		if isActiveReservationConstraint(err) {
			return ControlResult{}, fmt.Errorf("%w: resource %s became reserved while resuming task %s", ErrTaskAlreadyActive, task.DedupeKey, taskID)
		}
		return ControlResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO fetch_task_queue(task_id, enqueued_at) VALUES (?, ?)`, taskID, now); err != nil {
		return ControlResult{}, err
	}
	task, found, err = r.getTx(ctx, tx, taskID)
	if err != nil {
		return ControlResult{}, err
	}
	if !found {
		return ControlResult{}, ErrTaskNotFound
	}
	if err := tx.Commit(); err != nil {
		return ControlResult{}, err
	}
	return ControlResult{Task: task, Changed: true}, nil
}

func isActiveReservationConstraint(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique constraint") &&
		(strings.Contains(message, "fetch_tasks.dedupe_key") || strings.Contains(message, "fetch_tasks_reserved_dedupe_idx"))
}

func (r *SQLiteRepository) requestAction(ctx context.Context, taskID string, action RequestedAction) (ControlResult, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return ControlResult{}, err
	}
	defer func() { _ = tx.Rollback() }()
	task, found, err := r.getTx(ctx, tx, taskID)
	if err != nil {
		return ControlResult{}, err
	}
	if !found {
		return ControlResult{}, ErrTaskNotFound
	}
	if task.Status == StatusSucceeded || task.Status == StatusCanceled || (task.Status == StatusFailed && action == RequestedActionPause) || (task.Status == StatusInterrupted && action == RequestedActionPause) {
		return ControlResult{}, fmt.Errorf("%w: task %s cannot %s from %s", ErrTaskStateConflict, taskID, action, task.Status)
	}
	if task.Status == StatusPaused && action == RequestedActionPause {
		return ControlResult{Task: task, Changed: false}, nil
	}
	if task.Status == StatusRunning && task.ExecutionCommitted {
		return ControlResult{Task: task, Changed: false}, fmt.Errorf("%w: task %s has already committed its result", ErrTaskStateConflict, taskID)
	}
	if task.Status == StatusRunning && task.RequestedAction == action {
		return ControlResult{Task: task, Changed: false}, nil
	}
	if task.Status == StatusRunning && task.RequestedAction != RequestedActionNone {
		return ControlResult{Task: task, Changed: false}, fmt.Errorf("%w: task %s already has requested action %s", ErrTaskStateConflict, taskID, task.RequestedAction)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if task.Status == StatusQueued && action == RequestedActionPause {
		if _, err := tx.ExecContext(ctx, `DELETE FROM fetch_task_queue WHERE task_id = ?`, taskID); err != nil {
			return ControlResult{}, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE fetch_tasks SET status = 'paused', requested_action = '', paused_at = ?, updated_at = ? WHERE task_id = ?`, now, now, taskID); err != nil {
			return ControlResult{}, err
		}
	} else if task.Status == StatusQueued && action == RequestedActionCancel {
		if _, err := tx.ExecContext(ctx, `DELETE FROM fetch_task_queue WHERE task_id = ?`, taskID); err != nil {
			return ControlResult{}, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE fetch_tasks SET status = 'canceled', requested_action = '', finished_at = ?, updated_at = ?, message = 'Task cancelled' WHERE task_id = ?`, now, now, taskID); err != nil {
			return ControlResult{}, err
		}
	} else if task.Status == StatusPaused && action == RequestedActionCancel {
		if _, err := tx.ExecContext(ctx, `UPDATE fetch_tasks SET status = 'canceled', requested_action = '', finished_at = ?, updated_at = ?, message = 'Task cancelled' WHERE task_id = ?`, now, now, taskID); err != nil {
			return ControlResult{}, err
		}
	} else if task.Status == StatusInterrupted && action == RequestedActionCancel || task.Status == StatusFailed && action == RequestedActionCancel {
		if _, err := tx.ExecContext(ctx, `UPDATE fetch_tasks SET status = 'canceled', requested_action = '', finished_at = ?, updated_at = ?, message = 'Task cancelled' WHERE task_id = ?`, now, now, taskID); err != nil {
			return ControlResult{}, err
		}
	} else {
		if task.Status != StatusRunning {
			return ControlResult{}, fmt.Errorf("%w: task %s cannot %s from %s", ErrTaskStateConflict, taskID, action, task.Status)
		}
		result, err := tx.ExecContext(ctx, `UPDATE fetch_tasks SET requested_action = ?, updated_at = ? WHERE task_id = ? AND status = 'running' AND execution_committed = 0 AND requested_action = ''`, action, now, taskID)
		if err != nil {
			return ControlResult{}, err
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return ControlResult{}, err
		}
		if changed != 1 {
			return ControlResult{}, fmt.Errorf("%w: task %s control lost the commit fence", ErrTaskStateConflict, taskID)
		}
	}
	task, found, err = r.getTx(ctx, tx, taskID)
	if err != nil {
		return ControlResult{}, err
	}
	if !found {
		return ControlResult{}, ErrTaskNotFound
	}
	if err := tx.Commit(); err != nil {
		return ControlResult{}, err
	}
	return ControlResult{Task: task, Changed: true}, nil
}

func (r *SQLiteRepository) ReadRequestedAction(ctx context.Context, ref TaskRef) (RequestedAction, error) {
	var action RequestedAction
	err := r.readDB.QueryRowContext(ctx, `SELECT requested_action FROM fetch_tasks WHERE task_id = ? AND status = 'running' AND attempt_count = ?`, ref.TaskID, ref.Attempt).Scan(&action)
	if errors.Is(err, sql.ErrNoRows) {
		return RequestedActionNone, ErrStaleTaskAttempt
	}
	return action, err
}

func (r *SQLiteRepository) UpdateProgress(ctx context.Context, ref TaskRef, progress Progress) error {
	result, err := r.db.ExecContext(ctx, `UPDATE fetch_tasks SET phase = ?, current_step = ?, total_steps = ?, message = CASE WHEN ? <> '' THEN ? ELSE message END, updated_at = ? WHERE task_id = ? AND status = 'running' AND attempt_count = ?`, progress.Phase, progress.CurrentStep, progress.TotalSteps, progress.Message, progress.Message, time.Now().UTC().Format(time.RFC3339Nano), ref.TaskID, ref.Attempt)
	if err != nil {
		return err
	}
	return requireAttemptUpdate(result)
}

func (r *SQLiteRepository) UpdateMessage(ctx context.Context, ref TaskRef, message string) error {
	result, err := r.db.ExecContext(ctx, `UPDATE fetch_tasks SET message = ?, updated_at = ? WHERE task_id = ? AND status = 'running' AND attempt_count = ?`, message, time.Now().UTC().Format(time.RFC3339Nano), ref.TaskID, ref.Attempt)
	if err != nil {
		return err
	}
	return requireAttemptUpdate(result)
}

func (r *SQLiteRepository) AddWarning(ctx context.Context, ref TaskRef, warning string) error {
	if strings.TrimSpace(warning) == "" {
		return nil
	}
	task, found, err := r.getTx(ctx, r.db, ref.TaskID)
	if err != nil {
		return err
	}
	if !found || task.AttemptCount != ref.Attempt || task.Status != StatusRunning {
		return ErrStaleTaskAttempt
	}
	for _, current := range task.Warnings {
		if current == warning {
			return nil
		}
	}
	task.Warnings = append(task.Warnings, warning)
	warnings, _ := json.Marshal(task.Warnings)
	result, err := r.db.ExecContext(ctx, `UPDATE fetch_tasks SET warnings_json = ?, updated_at = ? WHERE task_id = ? AND status = 'running' AND attempt_count = ?`, string(warnings), time.Now().UTC().Format(time.RFC3339Nano), ref.TaskID, ref.Attempt)
	if err != nil {
		return err
	}
	return requireAttemptUpdate(result)
}

func (r *SQLiteRepository) SetTarget(ctx context.Context, ref TaskRef, target string) error {
	return r.updateString(ctx, ref, `target_label`, target)
}

func (r *SQLiteRepository) SetWorkID(ctx context.Context, ref TaskRef, workID int) error {
	if workID <= 0 {
		return fmt.Errorf("work id must be positive")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	task, found, err := r.getTx(ctx, tx, ref.TaskID)
	if err != nil {
		return err
	}
	if !found || task.AttemptCount != ref.Attempt || task.Status != StatusRunning {
		return ErrStaleTaskAttempt
	}
	if task.WorkID == workID {
		return tx.Commit()
	}
	if task.WorkID != 0 {
		return fmt.Errorf("task %s already belongs to work %d", ref.TaskID, task.WorkID)
	}
	var reservedBy string
	err = tx.QueryRowContext(ctx, `SELECT task_id FROM fetch_tasks WHERE primary_work_id = ? AND task_id != ? AND status IN ('queued', 'running', 'paused', 'interrupted') LIMIT 1`, workID, ref.TaskID).Scan(&reservedBy)
	if err == nil {
		return fmt.Errorf("%w: work %d is already reserved by task %s", ErrTaskAlreadyActive, workID, reservedBy)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	task.WorkID = workID
	request, dedupe, fingerprint, err := RequestForTask(task)
	if err != nil {
		return err
	}
	requestJSON, _ := json.Marshal(request)
	result, err := tx.ExecContext(ctx, `UPDATE fetch_tasks SET request_json = ?, dedupe_key = ?, request_fingerprint = ?, primary_work_id = ?, updated_at = ? WHERE task_id = ? AND status = 'running' AND attempt_count = ?`, string(requestJSON), dedupe, fingerprint, workID, time.Now().UTC().Format(time.RFC3339Nano), ref.TaskID, ref.Attempt)
	if err != nil {
		return err
	}
	if err := requireAttemptUpdate(result); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *SQLiteRepository) SetSavedEpisodeCount(ctx context.Context, ref TaskRef, count int) error {
	return r.updateInt(ctx, ref, `saved_episode_count`, count)
}

func (r *SQLiteRepository) SetFailureEpisode(ctx context.Context, ref TaskRef, failedEpisodeID string, resumeEpisodeID string) error {
	result, err := r.db.ExecContext(ctx, `UPDATE fetch_tasks SET failed_episode_id = ?, resume_episode_id = ?, updated_at = ? WHERE task_id = ? AND status = 'running' AND attempt_count = ?`, failedEpisodeID, resumeEpisodeID, time.Now().UTC().Format(time.RFC3339Nano), ref.TaskID, ref.Attempt)
	if err != nil {
		return err
	}
	return requireAttemptUpdate(result)
}

func (r *SQLiteRepository) Finalize(ctx context.Context, ref TaskRef, outcome Outcome) error {
	if outcome.Status == StatusQueued || outcome.Status == StatusRunning || outcome.Status == "" {
		return fmt.Errorf("invalid terminal task status %q", outcome.Status)
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	task, found, err := r.getTx(ctx, tx, ref.TaskID)
	if err != nil {
		return err
	}
	if !found || task.Status != StatusRunning || task.AttemptCount != ref.Attempt {
		return ErrStaleTaskAttempt
	}
	if task.ExecutionCommitted || outcome.ExecutionCommitted {
		outcome = Outcome{Status: StatusSucceeded, Message: "Task completed", ExecutionCommitted: true}
	} else {
		switch task.RequestedAction {
		case RequestedActionPause:
			outcome.Status, outcome.Message, outcome.Error = StatusPaused, "Task paused", ErrTaskPauseRequested
		case RequestedActionCancel:
			outcome.Status, outcome.Message, outcome.Error = StatusCanceled, "Task cancelled", ErrTaskCancelRequested
		}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	message := outcome.Message
	if message == "" {
		switch outcome.Status {
		case StatusSucceeded:
			message = "Task completed"
		case StatusCanceled:
			message = "Task cancelled"
		}
	}
	errorMessage := ""
	if outcome.Error != nil {
		errorMessage = outcome.Error.Error()
	}
	result, err := tx.ExecContext(ctx, `UPDATE fetch_tasks SET status = ?, requested_action = '', message = CASE WHEN ? <> '' THEN ? ELSE message END, error_message = ?, execution_committed = CASE WHEN ? THEN 1 ELSE execution_committed END, finished_at = ?, updated_at = ?, paused_at = CASE WHEN ? = 'paused' THEN ? ELSE paused_at END, interrupted_at = CASE WHEN ? = 'interrupted' THEN ? ELSE interrupted_at END WHERE task_id = ? AND status = 'running' AND attempt_count = ?`, outcome.Status, message, message, errorMessage, outcome.ExecutionCommitted, now, now, outcome.Status, now, outcome.Status, now, ref.TaskID, ref.Attempt)
	if err != nil {
		return err
	}
	if err := requireAttemptUpdate(result); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *SQLiteRepository) RecoverOnStartup(ctx context.Context, now time.Time) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, `SELECT kind, request_version, request_json FROM fetch_tasks WHERE status IN ('queued', 'running', 'paused', 'interrupted', 'failed')`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var kind string
		var requestVersion int
		var requestJSON string
		if err := rows.Scan(&kind, &requestVersion, &requestJSON); err != nil {
			_ = rows.Close()
			return err
		}
		if requestVersion != CurrentRequestVersion {
			_ = rows.Close()
			return fmt.Errorf("unsupported task request version %d", requestVersion)
		}
		request, err := DecodeRequest(requestJSON)
		if err != nil {
			_ = rows.Close()
			return err
		}
		if request.Kind != kind {
			_ = rows.Close()
			return fmt.Errorf("task request kind %q does not match stored kind %q", request.Kind, kind)
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	_ = rows.Close()
	var invalid int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM fetch_tasks t LEFT JOIN fetch_task_queue q ON q.task_id = t.task_id WHERE t.status = 'queued' AND q.task_id IS NULL`).Scan(&invalid); err != nil {
		return err
	}
	if invalid > 0 {
		return fmt.Errorf("%w: queued task has no queue row", ErrTaskStateConflict)
	}
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM fetch_task_queue q JOIN fetch_tasks t ON t.task_id = q.task_id WHERE t.status <> 'queued'`).Scan(&invalid); err != nil {
		return err
	}
	if invalid > 0 {
		return fmt.Errorf("%w: non-queued task has queue row", ErrTaskStateConflict)
	}
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM fetch_tasks WHERE status = 'running'`).Scan(&invalid); err != nil {
		return err
	}
	if invalid > 1 {
		return fmt.Errorf("%w: more than one running task", ErrTaskStateConflict)
	}
	rows, err = tx.QueryContext(ctx, `SELECT task_id, requested_action, execution_committed FROM fetch_tasks WHERE status = 'running'`)
	if err != nil {
		return err
	}
	defer rows.Close()
	stamp := now.UTC().Format(time.RFC3339Nano)
	for rows.Next() {
		var taskID string
		var rawAction string
		var committed int
		if err := rows.Scan(&taskID, &rawAction, &committed); err != nil {
			return err
		}
		action := RequestedAction(rawAction)
		status := StatusInterrupted
		message := "Task interrupted during process recovery"
		if committed != 0 {
			status, message = StatusSucceeded, "Task completed"
		} else if action == RequestedActionCancel {
			status, message = StatusCanceled, "Task cancelled during process recovery"
		}
		if _, err := tx.ExecContext(ctx, `UPDATE fetch_tasks SET status = ?, requested_action = '', message = ?, finished_at = ?, updated_at = ?, paused_at = '', interrupted_at = CASE WHEN ? = 'interrupted' THEN ? ELSE interrupted_at END WHERE task_id = ? AND status = 'running'`, status, message, stamp, stamp, status, stamp, taskID); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return tx.Commit()
}

type queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func (r *SQLiteRepository) getTx(ctx context.Context, q queryer, taskID string) (*Task, bool, error) {
	row := q.QueryRowContext(ctx, `SELECT task_id, request_version, kind, request_json, status, requested_action, dedupe_key, request_fingerprint, primary_work_id, target_label, phase, current_step, total_steps, saved_episode_count, failed_episode_id, resume_episode_id, message, warnings_json, error_message, attempt_count, execution_committed, created_at, started_at, updated_at, paused_at, interrupted_at, finished_at FROM fetch_tasks WHERE task_id = ?`, taskID)
	return scanTask(row)
}

func (r *SQLiteRepository) oneByStatus(ctx context.Context, q queryer, status Status) (*Task, error) {
	rows, err := r.list(ctx, q, `status = ? ORDER BY updated_at DESC LIMIT 1`, status)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return rows[0], nil
}

func (r *SQLiteRepository) list(ctx context.Context, q queryer, clause string, args ...any) ([]*Task, error) {
	query := `SELECT task_id, request_version, kind, request_json, status, requested_action, dedupe_key, request_fingerprint, primary_work_id, target_label, phase, current_step, total_steps, saved_episode_count, failed_episode_id, resume_episode_id, message, warnings_json, error_message, attempt_count, execution_committed, created_at, started_at, updated_at, paused_at, interrupted_at, finished_at FROM fetch_tasks WHERE ` + clause
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tasks := []*Task{}
	for rows.Next() {
		task, found, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		if found {
			tasks = append(tasks, task)
		}
	}
	return tasks, rows.Err()
}

func scanTask(row interface{ Scan(...any) error }) (*Task, bool, error) {
	var task Task
	var primaryWorkID int
	var requestVersion int
	var requestJSON, status, requestedAction, warningsJSON string
	var committed int
	var createdAt, startedAt, updatedAt, pausedAt, interruptedAt, finishedAt string
	if err := row.Scan(&task.ID, &requestVersion, &task.Kind, &requestJSON, &status, &requestedAction, &task.DedupeKey, &task.RequestFingerprint, &primaryWorkID, &task.TargetLabel, &task.Phase, &task.CurrentStep, &task.TotalSteps, &task.SavedEpisodeCount, &task.FailedEpisodeID, &task.ResumeEpisodeID, &task.Message, &warningsJSON, &task.ErrorMessage, &task.AttemptCount, &committed, &createdAt, &startedAt, &updatedAt, &pausedAt, &interruptedAt, &finishedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, err
	}
	request, err := DecodeRequest(requestJSON)
	if err != nil || requestVersion != CurrentRequestVersion || request.Kind != task.Kind {
		return nil, false, fmt.Errorf("unsupported task request version or malformed request for %s", task.ID)
	}
	if primaryWorkID != request.WorkID {
		return nil, false, fmt.Errorf("task %s has inconsistent work identity", task.ID)
	}
	task.Target, task.WorkID = request.Target, request.WorkID
	task.Force, task.ForceRedownload, task.SkipUnchanged = request.Options.Force, request.Options.ForceRedownload, request.Options.SkipUnchanged
	task.Status, task.RequestedAction, task.ExecutionCommitted = Status(status), RequestedAction(requestedAction), committed != 0
	if err := json.Unmarshal([]byte(warningsJSON), &task.Warnings); err != nil {
		return nil, false, err
	}
	task.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return nil, false, err
	}
	task.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return nil, false, err
	}
	task.StartedAt, err = optionalTime(startedAt)
	if err != nil {
		return nil, false, err
	}
	task.PausedAt, err = optionalTime(pausedAt)
	if err != nil {
		return nil, false, err
	}
	task.InterruptedAt, err = optionalTime(interruptedAt)
	if err != nil {
		return nil, false, err
	}
	task.FinishedAt, err = optionalTime(finishedAt)
	if err != nil {
		return nil, false, err
	}
	return &task, true, nil
}

func parseTime(value string) (time.Time, error) { return time.Parse(time.RFC3339Nano, value) }

func optionalTime(value string) (*time.Time, error) {
	if value == "" {
		return nil, nil
	}
	parsed, err := parseTime(value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func requireAttemptUpdate(result sql.Result) error {
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrStaleTaskAttempt
	}
	return nil
}

func (r *SQLiteRepository) updateString(ctx context.Context, ref TaskRef, column, value string) error {
	result, err := r.db.ExecContext(ctx, `UPDATE fetch_tasks SET `+column+` = ?, updated_at = ? WHERE task_id = ? AND status = 'running' AND attempt_count = ?`, value, time.Now().UTC().Format(time.RFC3339Nano), ref.TaskID, ref.Attempt)
	if err != nil {
		return err
	}
	return requireAttemptUpdate(result)
}

func (r *SQLiteRepository) updateInt(ctx context.Context, ref TaskRef, column string, value int) error {
	result, err := r.db.ExecContext(ctx, `UPDATE fetch_tasks SET `+column+` = ?, updated_at = ? WHERE task_id = ? AND status = 'running' AND attempt_count = ?`, value, time.Now().UTC().Format(time.RFC3339Nano), ref.TaskID, ref.Attempt)
	if err != nil {
		return err
	}
	return requireAttemptUpdate(result)
}
