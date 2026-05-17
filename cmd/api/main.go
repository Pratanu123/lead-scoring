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

	"lead-scoring/internal/config"
	httpapi "lead-scoring/internal/http"
	leadcontroller "lead-scoring/internal/lead/controller"
	leadrepository "lead-scoring/internal/lead/repository"
	leadservice "lead-scoring/internal/lead/service"
	opensearch "lead-scoring/internal/platform/opensearch"
	"lead-scoring/internal/platform/postgres"
	redisclient "lead-scoring/internal/platform/redis"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg := config.Load()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := postgres.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("postgres connection failed", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	redisClient, err := redisclient.Connect(ctx, cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	if err != nil {
		logger.Error("redis connection failed", "error", err)
		os.Exit(1)
	}
	defer redisClient.Close()

	leadRepo := leadrepository.NewPostgresRepository(db)
	leadSvc := leadservice.NewLeadService(leadRepo, redisClient)

	var opensearchClient *opensearch.Client
	if cfg.OpenSearchEnabled {
		opensearchClient = opensearch.NewClient(cfg.OpenSearchURL, cfg.OpenSearchUser, cfg.OpenSearchPassword)
	}

	leadHandler := leadcontroller.NewLeadHandler(leadSvc, logger, opensearchClient, redisClient)

	router := httpapi.NewRouter(httpapi.RouterDeps{
		LeadHandler: leadHandler,
		DB:          db,
		Redis:       redisClient,
		Logger:      logger,
	})

	server := &http.Server{
		Addr:              ":" + cfg.HTTPPort,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logger.Info("api server started", "addr", server.Addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("api server failed", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("api server shutdown failed", "error", err)
		os.Exit(1)
	}

	logger.Info("api server stopped")
}
