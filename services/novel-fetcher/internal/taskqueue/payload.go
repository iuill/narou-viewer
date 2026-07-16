package taskqueue

import "time"

type Summary struct {
	Current          map[string]any
	Queued           []map[string]any
	Paused           []map[string]any
	Interrupted      []map[string]any
	RecentCompleted  []map[string]any
	RecentFailed     []map[string]any
	CompletedCount   int
	FailedCount      int
	CanceledCount    int
	PausedCount      int
	InterruptedCount int
}

type StatusCounts struct {
	Total   int
	Running bool
}

func Payloads(tasks []*Task) []map[string]any {
	payloads := make([]map[string]any, 0, len(tasks))
	for _, task := range tasks {
		if payload := Payload(task); payload != nil {
			payloads = append(payloads, payload)
		}
	}
	return payloads
}

func Payload(task *Task) map[string]any {
	if task == nil {
		return nil
	}

	payload := map[string]any{
		"id":               task.ID,
		"task_id":          task.ID,
		"type":             task.Kind,
		"status":           task.Status,
		"requested_action": task.RequestedAction,
		"attempt_count":    task.AttemptCount,
		"targets":          task.Targets,
		"novel_ids":        IntIDsToStrings(task.NovelIDs),
		"message":          task.Message,
		"created_at":       task.CreatedAt.Format(time.RFC3339Nano),
	}
	if task.QueuePosition != nil {
		payload["queue_position"] = *task.QueuePosition
	} else {
		payload["queue_position"] = nil
	}
	payload["can_pause"] = task.Status == StatusQueued || task.Status == StatusRunning
	payload["can_resume"] = task.Status == StatusPaused || task.Status == StatusInterrupted || task.Status == StatusFailed
	payload["can_cancel"] = task.Status == StatusQueued || task.Status == StatusRunning || task.Status == StatusPaused || task.Status == StatusInterrupted || task.Status == StatusFailed
	switch task.Kind {
	case "download":
		payload["force"] = task.Force
	case "update":
		payload["force_redownload"] = task.ForceRedownload
		payload["skip_unchanged"] = task.SkipUnchanged
	}
	if task.TargetLabel != "" {
		payload["novel_title"] = task.TargetLabel
	}
	if task.StartedAt != nil {
		payload["started_at"] = task.StartedAt.Format(time.RFC3339Nano)
	}
	if task.FinishedAt != nil {
		payload["finished_at"] = task.FinishedAt.Format(time.RFC3339Nano)
	}
	if task.PausedAt != nil {
		payload["paused_at"] = task.PausedAt.Format(time.RFC3339Nano)
	}
	if task.InterruptedAt != nil {
		payload["interrupted_at"] = task.InterruptedAt.Format(time.RFC3339Nano)
	}
	if task.ErrorMessage != "" {
		payload["error"] = task.ErrorMessage
	}
	if len(task.Warnings) > 0 {
		payload["warnings"] = task.Warnings
	}
	if task.FailedEpisodeID != "" {
		payload["failed_episode_id"] = task.FailedEpisodeID
	}
	if task.ResumeEpisodeID != "" {
		payload["resume_episode_id"] = task.ResumeEpisodeID
	}
	if task.SavedEpisodeCount > 0 {
		payload["saved_episode_count"] = task.SavedEpisodeCount
	}
	if task.Phase != "" {
		payload["phase"] = task.Phase
	}
	if task.TotalSteps > 0 {
		payload["current_step"] = task.CurrentStep
		payload["total_steps"] = task.TotalSteps
		payload["progress"] = float64(task.CurrentStep) / float64(task.TotalSteps) * 100
	}
	return payload
}
