package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"narou-viewer/services/novel-fetcher/internal/config"
	"narou-viewer/services/novel-fetcher/internal/fetcher"
	"narou-viewer/services/novel-fetcher/internal/server"
	"narou-viewer/services/novel-fetcher/internal/sites"
	"narou-viewer/services/novel-fetcher/internal/storage"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg := config.Load()

	store, err := storage.NewStore(cfg.DataDir)
	if err != nil {
		logger.Error("failed to open storage", "error", err)
		os.Exit(1)
	}
	defer store.Close()
	httpFetcher := fetcher.NewHTTPFetcher(fetcher.HTTPFetcherOptions{
		UserAgent: cfg.UserAgent,
		Timeout:   cfg.RequestTimeout,
		Logger:    logger,
	})
	store.SetAssetFetcher(httpFetcher, cfg.FetchPolicy)
	siteFetcher := sites.NewMultiFetcher(
		sites.NewSyosetuFetcher(httpFetcher, cfg.FetchPolicy, cfg.MaxTocPages, logger),
		sites.NewKakuyomuFetcher(httpFetcher, cfg.FetchPolicy, logger),
	)
	app := server.New(server.Options{
		Config:  cfg,
		Store:   store,
		Fetcher: siteFetcher,
		Logger:  logger,
	})
	app.Start(context.Background())

	httpServer := &http.Server{
		Addr:              cfg.Addr(),
		Handler:           app.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		logger.Info("novel-fetcher listening", "addr", cfg.Addr(), "dataDir", cfg.DataDir)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("novel-fetcher failed", "error", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	httpCtx, cancelHTTP := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelHTTP()
	if err := httpServer.Shutdown(httpCtx); err != nil {
		logger.Error("novel-fetcher http shutdown failed", "error", err)
		os.Exit(1)
	}

	workerCtx, cancelWorker := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelWorker()
	app.Shutdown(workerCtx)
}
