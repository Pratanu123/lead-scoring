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
	"lead-scoring/internal/lead/embedding"
	leadrepository "lead-scoring/internal/lead/repository"
	leadscoring "lead-scoring/internal/lead/scoring"
	leadservice "lead-scoring/internal/lead/service"
	appmetrics "lead-scoring/internal/platform/appmetrics"
	"lead-scoring/internal/platform/idempotency"
	"lead-scoring/internal/platform/jobs"
	opensearch "lead-scoring/internal/platform/opensearch"
	"lead-scoring/internal/platform/postgres"
	redisclient "lead-scoring/internal/platform/redis"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg := config.Load()
	metricsRegistry := appmetrics.NewRegistry()

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
	jobStore := jobs.NewStore(db, redisClient)

	var leadEmbedder embedding.Embedder = embedding.NewLocalEmbedder()
	if cfg.EmbeddingAPIURL != "" {
		leadEmbedder = embedding.NewRemoteEmbedder(cfg.EmbeddingAPIURL, cfg.EmbeddingAPIKey, cfg.EmbeddingModel)
		logger.Info("remote embedding enabled", "model", leadEmbedder.Model())
	}

	var leadScorer leadscoring.Scorer = leadscoring.NewLocalScorer()
	if cfg.LLMAPIURL != "" && cfg.LLMModel != "" {
		leadScorer = leadscoring.NewFallbackScorer(
			leadscoring.NewRemoteScorer(cfg.LLMAPIURL, cfg.LLMAPIKey, cfg.LLMModel),
			leadscoring.NewLocalScorer(),
		).WithFallbackHook(metricsRegistry.IncLLMFallback)
		logger.Info("remote llm scorer enabled with local fallback", "model", cfg.LLMModel)
	}

	leadSvc := leadservice.NewLeadService(leadRepo, redisClient, leadScorer, leadEmbedder, jobStore, metricsRegistry, logger)

	var opensearchClient *opensearch.Client
	if cfg.OpenSearchEnabled {
		opensearchClient = opensearch.NewClient(cfg.OpenSearchURL, cfg.OpenSearchUser, cfg.OpenSearchPassword)
	}

	idempotencyStore := idempotency.NewStore(redisClient)
	leadHandler := leadcontroller.NewLeadHandler(leadSvc, logger, opensearchClient, idempotencyStore)
	wsHub := httpapi.NewWSHub(jobStore, cfg.APIKey, logger)

	router := httpapi.NewRouter(httpapi.RouterDeps{
		LeadHandler: leadHandler,
		WSHub:       wsHub,
		DB:          db,
		Redis:       redisClient,
		Logger:      logger,
		APIKey:      cfg.APIKey,
		RateLimit:   cfg.RateLimitPerMinute,
		ScoreLimit:  cfg.ScoreRateLimitRPM,
		AppMetrics:  metricsRegistry,
		StaticDir:   cfg.StaticDir,
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
