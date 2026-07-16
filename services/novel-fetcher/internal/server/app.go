package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"narou-viewer/services/novel-fetcher/internal/application"
	"narou-viewer/services/novel-fetcher/internal/config"
	"narou-viewer/services/novel-fetcher/internal/sites"
	"narou-viewer/services/novel-fetcher/internal/storage"
	"narou-viewer/services/novel-fetcher/internal/taskqueue"
	"narou-viewer/services/novel-fetcher/internal/taskstate"
	"narou-viewer/services/novel-fetcher/internal/worker"
)

type Options struct {
	Config  config.Config
	Store   *storage.Store
	Fetcher sites.WorkFetcher
	Logger  *slog.Logger
}

type App struct {
	cfg     config.Config
	store   *storage.Store
	logger  *slog.Logger
	queue   *taskqueue.Queue
	runner  *worker.Runner
	initErr error
}

func New(options Options) *App {
	app, err := NewWithError(options)
	if err != nil {
		return &App{cfg: options.Config, store: options.Store, logger: options.Logger, initErr: err}
	}
	return app
}

func NewWithError(options Options) (*App, error) {
	queue := taskqueue.NewQueue()
	if options.Store == nil {
		return nil, errors.New("novel-fetcher storage is required")
	}
	repository := taskstate.NewSQLiteRepository(options.Store.DB())
	if err := repository.RecoverOnStartup(context.Background(), time.Now().UTC()); err != nil {
		return nil, err
	}
	queue = taskqueue.NewPersistentQueue(repository)
	service := application.NewService(application.Options{
		Store:    options.Store,
		Fetcher:  options.Fetcher,
		Reporter: queue,
	})
	return &App{
		cfg:    options.Config,
		store:  options.Store,
		logger: options.Logger,
		queue:  queue,
		runner: worker.NewRunner(worker.Options{
			Queue:        queue,
			Executor:     service,
			WorkInterval: options.Config.WorkInterval,
			Logger:       options.Logger,
		}),
	}, nil
}

func (a *App) Start(ctx context.Context) {
	if a.initErr != nil {
		return
	}
	a.runner.Start(ctx)
}

func (a *App) Shutdown(ctx context.Context) {
	a.runner.Stop(ctx)
}

func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", a.handleHealth)
	mux.HandleFunc("GET /api/v2/system/version", a.handleVersion)
	mux.HandleFunc("GET /api/v2/system/queue", a.handleQueue)
	mux.HandleFunc("GET /api/v2/tasks/summary", a.handleTasksSummary)
	mux.HandleFunc("GET /api/v2/tasks/{taskID}", a.handleTask)
	mux.HandleFunc("POST /api/v2/tasks/{taskID}/pause", a.handlePauseTask)
	mux.HandleFunc("POST /api/v2/tasks/{taskID}/resume", a.handleResumeTask)
	mux.HandleFunc("POST /api/v2/tasks/{taskID}/cancel", a.handleCancelTask)
	mux.HandleFunc("GET /api/v2/novels", a.handleListNovels)
	mux.HandleFunc("GET /api/v1/works", a.handleListWorks)
	mux.HandleFunc("GET /api/v1/works/tocs", a.handleListTocs)
	mux.HandleFunc("GET /api/v1/works/{workID}", a.handleGetWork)
	mux.HandleFunc("GET /api/v1/works/{workID}/toc", a.handleGetToc)
	mux.HandleFunc("GET /api/v1/works/{workID}/episodes/{episodeID}", a.handleGetEpisode)
	mux.HandleFunc("POST /api/v2/novels/download", a.handleDownloadNovels)
	mux.HandleFunc("POST /api/v2/novels/update", a.handleUpdateNovels)
	mux.HandleFunc("POST /api/v2/novels/resume", a.handleResumeNovels)
	mux.HandleFunc("POST /api/v2/novels/remove", a.handleRemoveNovels)
	return mux
}
