package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"narou-viewer/apps/viewer-api-go/internal/ai"
	"narou-viewer/apps/viewer-api-go/internal/config"
	"narou-viewer/apps/viewer-api-go/internal/runtime"
	"narou-viewer/apps/viewer-api-go/internal/statebarrier"
)

func main() {
	runCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg := config.Load()
	writerLock, err := statebarrier.AcquireViewerAPI(cfg.DataDir)
	if err != nil {
		log.Fatalf("acquire viewer-api state writer barrier: %v", err)
	}
	defer writerLock.Close()
	if _, err := ai.ResolveOpenRouterReasoningRequest(ai.OpenRouterConfig{}); err != nil {
		log.Fatalf("validate OPENROUTER_REASONING_EFFORT: %v", err)
	}
	handlerResult := runtime.NewHandler(cfg.DataDir)
	if handlerResult.InitErr != nil {
		log.Fatalf("initialize viewer-api-go state: %v", handlerResult.InitErr)
	}
	if err := handlerResult.StartBackground(runCtx); err != nil {
		log.Fatalf("start viewer-api-go background jobs: %v", err)
	}
	handler := handlerResult.Handler

	server := &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		<-runCtx.Done()
		stop()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if shutdownHandler, ok := handler.(interface {
			Shutdown(context.Context) error
		}); ok {
			if err := shutdownHandler.Shutdown(shutdownCtx); err != nil {
				log.Printf("viewer-api-go handler shutdown: %v", err)
			}
		}
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("viewer-api-go server shutdown: %v", err)
		}
	}()

	log.Printf("viewer-api-go listening on %s", cfg.Addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
