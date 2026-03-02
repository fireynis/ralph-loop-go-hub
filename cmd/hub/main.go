package main

import (
	"context"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/fireynis/ralph-hub/internal/config"
	"github.com/fireynis/ralph-hub/internal/frontend"
	"github.com/fireynis/ralph-hub/internal/server"
	"github.com/fireynis/ralph-hub/internal/store"
	"github.com/fireynis/ralph-hub/internal/webhook"
	"github.com/fireynis/ralph-hub/internal/ws"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	// Initialize store based on configured driver.
	var st store.Store
	switch cfg.Storage.Driver {
	case "sqlite":
		st, err = store.NewSQLiteStore(cfg.Storage.SQLite.Path)
	case "postgres":
		st, err = store.NewPostgresStore(cfg.Storage.Postgres.DSN)
	default:
		log.Fatalf("unknown storage driver: %q", cfg.Storage.Driver)
	}
	if err != nil {
		log.Fatalf("failed to initialize %s store: %v", cfg.Storage.Driver, err)
	}
	defer st.Close()

	hub := ws.NewHub()
	dispatcher := webhook.New(cfg.Webhooks)
	srv := server.New(cfg, st, hub, dispatcher)

	frontendFS, err := fs.Sub(frontend.Dist, "dist")
	if err != nil {
		log.Fatalf("failed to access embedded frontend: %v", err)
	}
	srv.SetFrontendFS(frontendFS)

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	httpServer := &http.Server{
		Addr:    addr,
		Handler: srv.Handler(),
	}

	// Graceful shutdown: listen for SIGINT / SIGTERM.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("ralph-hub starting on %s", addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	// Block until a shutdown signal is received.
	<-ctx.Done()
	log.Println("shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("forced shutdown: %v", err)
	}

	log.Println("ralph-hub stopped")
}
