package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"narou-viewer/apps/viewer-api-go/internal/storageusage"
)

type storageUsageProgressState string

const (
	storageUsageProgressIdle      storageUsageProgressState = "idle"
	storageUsageProgressRunning   storageUsageProgressState = "running"
	storageUsageProgressCompleted storageUsageProgressState = "completed"
	storageUsageProgressError     storageUsageProgressState = "error"
)

const maxStorageUsageProgressRuns = 32

type storageUsageProgressResponse struct {
	RequestID     string                     `json:"requestId,omitempty"`
	State         storageUsageProgressState  `json:"state"`
	Phase         storageusage.ProgressPhase `json:"phase"`
	CheckedNovels int                        `json:"checkedNovels"`
	TotalNovels   int                        `json:"totalNovels"`
	StartedAt     string                     `json:"startedAt,omitempty"`
	UpdatedAt     string                     `json:"updatedAt,omitempty"`
	Error         string                     `json:"error,omitempty"`
}

type storageUsageProgressStore struct {
	mu      sync.Mutex
	current storageUsageProgressResponse
	nextRun uint64
	active  string
	byID    map[string]storageUsageProgressResponse
	order   []string
}

func newStorageUsageProgressStore() *storageUsageProgressStore {
	return &storageUsageProgressStore{
		current: storageUsageProgressResponse{
			State: storageUsageProgressIdle,
			Phase: storageusage.ProgressPhasePreparing,
		},
		byID: map[string]storageUsageProgressResponse{},
	}
}

func (s *Server) handleStorageUsage(w http.ResponseWriter, r *http.Request) {
	if !methodOnly(w, r, http.MethodGet) {
		return
	}
	progress := s.ensureStorageUsageProgress()
	requestID := storageUsageProgressRequestID(r)
	runID := progress.start(requestID)
	usage, err := storageusage.New(s.dataDir, s.library).CollectWithProgress(r.Context(), func(progressUpdate storageusage.Progress) {
		progress.update(runID, progressUpdate)
	})
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			progress.reset(runID)
		} else {
			progress.fail(runID, err.Error())
		}
		writeError(w, http.StatusInternalServerError, "Storage usage could not be collected.")
		return
	}
	writeJSON(w, http.StatusOK, usage)
}

func (s *Server) handleStorageUsageProgress(w http.ResponseWriter, r *http.Request) {
	if !methodOnly(w, r, http.MethodGet) {
		return
	}
	writeJSON(w, http.StatusOK, s.ensureStorageUsageProgress().snapshot(storageUsageProgressRequestID(r)))
}

func (s *Server) ensureStorageUsageProgress() *storageUsageProgressStore {
	if s.storageProgress == nil {
		s.storageProgress = newStorageUsageProgressStore()
	}
	return s.storageProgress
}

func storageUsageProgressRequestID(r *http.Request) string {
	return strings.TrimSpace(r.URL.Query().Get("requestId"))
}

func (s *storageUsageProgressStore) start(requestID string) string {
	now := time.Now().UTC().Format(time.RFC3339)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextRun++
	if requestID == "" {
		requestID = "storage-scan-" + strconv.FormatUint(s.nextRun, 10)
	}
	s.active = requestID
	progress := storageUsageProgressResponse{
		RequestID: requestID,
		State:     storageUsageProgressRunning,
		Phase:     storageusage.ProgressPhasePreparing,
		StartedAt: now,
		UpdatedAt: now,
	}
	s.current = progress
	s.byID[requestID] = progress
	s.rememberLocked(requestID)
	return requestID
}

func (s *storageUsageProgressStore) rememberLocked(requestID string) {
	for i := 0; i < len(s.order); i++ {
		if s.order[i] == requestID {
			s.order = append(s.order[:i], s.order[i+1:]...)
			i--
		}
	}
	s.order = append(s.order, requestID)
	for len(s.order) > maxStorageUsageProgressRuns {
		expired := s.order[0]
		s.order = s.order[1:]
		if expired != s.active {
			delete(s.byID, expired)
		}
	}
}

func (s *storageUsageProgressStore) update(requestID string, progress storageusage.Progress) {
	now := time.Now().UTC().Format(time.RFC3339)
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.byID[requestID]
	if !ok {
		return
	}
	state := storageUsageProgressRunning
	if progress.Phase == storageusage.ProgressPhaseCompleted {
		state = storageUsageProgressCompleted
	}
	current.State = state
	current.Phase = progress.Phase
	current.CheckedNovels = progress.CheckedNovels
	current.TotalNovels = progress.TotalNovels
	current.UpdatedAt = now
	s.byID[requestID] = current
	if requestID == s.active {
		s.current = current
	}
}

func (s *storageUsageProgressStore) fail(requestID string, message string) {
	now := time.Now().UTC().Format(time.RFC3339)
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.byID[requestID]
	if !ok {
		return
	}
	current.State = storageUsageProgressError
	current.UpdatedAt = now
	current.Error = message
	s.byID[requestID] = current
	if requestID == s.active {
		s.current = current
	}
}

func (s *storageUsageProgressStore) reset(requestID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.byID[requestID]; !ok {
		return
	}
	progress := storageUsageProgressResponse{
		RequestID: requestID,
		State:     storageUsageProgressIdle,
		Phase:     storageusage.ProgressPhasePreparing,
	}
	s.byID[requestID] = progress
	if requestID == s.active {
		s.current = progress
	}
}

func (s *storageUsageProgressStore) snapshot(requestID string) storageUsageProgressResponse {
	s.mu.Lock()
	defer s.mu.Unlock()
	if requestID != "" {
		if progress, ok := s.byID[requestID]; ok {
			return progress
		}
		return storageUsageProgressResponse{
			RequestID: requestID,
			State:     storageUsageProgressIdle,
			Phase:     storageusage.ProgressPhasePreparing,
		}
	}
	return s.current
}
