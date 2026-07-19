package runtime

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"narou-viewer/apps/viewer-api-go/internal/characters"
	extractdomain "narou-viewer/apps/viewer-api-go/internal/extraction"

	_ "modernc.org/sqlite"
)

func TestNewHandlerInitializesRuntime(t *testing.T) {
	dataDir := t.TempDir()
	result := NewHandler(dataDir)
	if result.InitErr != nil {
		t.Fatalf("NewHandler returned init error: %v", result.InitErr)
	}
	if result.Handler == nil {
		t.Fatal("NewHandler should return a handler")
	}
	if _, err := os.Stat(filepath.Join(dataDir, "state")); err != nil {
		t.Fatalf("NewHandler should initialize state dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "state", "publications.yaml")); err != nil {
		t.Fatalf("NewHandler should initialize publications state: %v", err)
	}
	if shutdownHandler, ok := result.Handler.(interface {
		Shutdown(context.Context) error
	}); ok {
		if err := shutdownHandler.Shutdown(context.Background()); err != nil {
			t.Fatalf("Shutdown returned error: %v", err)
		}
	} else {
		t.Fatalf("handler should support shutdown: %T", result.Handler)
	}
}

func TestNewHandlerReportsStateInitializationError(t *testing.T) {
	blockedParent := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(blockedParent, []byte("not a dir"), 0o644); err != nil {
		t.Fatalf("write blocked parent: %v", err)
	}
	result := NewHandler(blockedParent)
	if result.InitErr == nil {
		t.Fatal("NewHandler should report state initialization errors")
	}
	if result.Handler == nil {
		t.Fatal("NewHandler should still return a degraded handler")
	}
	health := runtimeRequestJSON(t, result.Handler, "/api/health")
	if health["status"] != "warn" {
		t.Fatalf("degraded runtime health should report warn: %+v", health)
	}
	status := runtimeRequestJSON(t, result.Handler, "/api/system/status")
	services, ok := status["services"].([]any)
	if !ok {
		t.Fatalf("runtime status should include services: %+v", status)
	}
	foundStateWarning := false
	for _, serviceValue := range services {
		service, ok := serviceValue.(map[string]any)
		if ok && service["id"] == "state" && service["status"] == "warn" {
			foundStateWarning = true
		}
	}
	if !foundStateWarning {
		t.Fatalf("degraded runtime status should expose state warning: %+v", status)
	}
	if shutdownHandler, ok := result.Handler.(interface {
		Shutdown(context.Context) error
	}); ok {
		_ = shutdownHandler.Shutdown(context.Background())
	}
}

func TestNewHandlerRejectsFutureUsageSchemaDuringStartup(t *testing.T) {
	dataDir := t.TempDir()
	stateDir := filepath.Join(dataDir, "state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("mkdir state: %v", err)
	}
	dbPath := filepath.Join(stateDir, "ai_usage.sqlite")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open usage fixture: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY); INSERT INTO schema_migrations(version) VALUES (99)`); err != nil {
		t.Fatalf("seed future usage fixture: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close usage fixture: %v", err)
	}

	result := NewHandler(dataDir)
	if result.InitErr == nil {
		t.Fatal("NewHandler should reject a future AI usage schema during startup")
	}
}

func TestHandlerResultStartsBackgroundAfterInitialization(t *testing.T) {
	dataDir := t.TempDir()
	stateDir := filepath.Join(dataDir, "state")
	if err := characters.EnsureStateDirs(stateDir); err != nil {
		t.Fatalf("EnsureStateDirs returned error: %v", err)
	}
	startedAt := "2026-01-01T00:00:00Z"
	job := extractdomain.Job{
		JobID:                     "job-running",
		RequestedUpToEpisodeIndex: "1",
		GenerationMode:            "heuristic",
		Status:                    "running",
		CreatedAt:                 "2026-01-01T00:00:00Z",
		StartedAt:                 &startedAt,
	}
	if err := extractdomain.SaveJob(stateDir, "novel-1", job); err != nil {
		t.Fatalf("SaveJob returned error: %v", err)
	}

	result := NewHandler(dataDir)
	if result.InitErr != nil {
		t.Fatalf("NewHandler returned init error: %v", result.InitErr)
	}
	t.Cleanup(func() {
		if shutdownHandler, ok := result.Handler.(interface {
			Shutdown(context.Context) error
		}); ok {
			_ = shutdownHandler.Shutdown(context.Background())
		}
	})

	jobs, ok, err := extractdomain.LoadJobs(stateDir, "novel-1")
	if err != nil || !ok || len(jobs) != 1 {
		t.Fatalf("LoadJobs after NewHandler: jobs=%+v ok=%v err=%v", jobs, ok, err)
	}
	if jobs[0].Status != "running" {
		t.Fatalf("NewHandler should not recover running jobs, got status %q", jobs[0].Status)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := result.StartBackground(ctx); err != nil {
		t.Fatalf("StartBackground returned error: %v", err)
	}
	jobs, ok, err = extractdomain.LoadJobs(stateDir, "novel-1")
	if err != nil || !ok || len(jobs) != 1 {
		t.Fatalf("LoadJobs after StartBackground: jobs=%+v ok=%v err=%v", jobs, ok, err)
	}
	if jobs[0].Status != extractdomain.JobStatusInterrupted {
		t.Fatalf("StartBackground should mark running jobs interrupted, got status %q", jobs[0].Status)
	}
}

func TestHandlerResultStartBackgroundReturnsInitError(t *testing.T) {
	initErr := errors.New("initialization failed")
	called := false
	result := HandlerResult{
		InitErr: initErr,
		startBackground: func(context.Context) {
			called = true
		},
	}

	if err := result.StartBackground(context.Background()); !errors.Is(err, initErr) {
		t.Fatalf("StartBackground should return InitErr, got %v", err)
	}
	if called {
		t.Fatal("StartBackground should not start background work after initialization failure")
	}
}

func runtimeRequestJSON(t *testing.T, handler http.Handler, path string) map[string]any {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
	if response.Code != http.StatusOK {
		t.Fatalf("%s returned status %d: %s", path, response.Code, response.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s response: %v", path, err)
	}
	return body
}
