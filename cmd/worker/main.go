package main

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"lead-scoring/internal/config"
	"lead-scoring/internal/lead/embedding"
	leadrepository "lead-scoring/internal/lead/repository"
	leadscoring "lead-scoring/internal/lead/scoring"
	leadservice "lead-scoring/internal/lead/service"
	appmetrics "lead-scoring/internal/platform/appmetrics"
	"lead-scoring/internal/platform/jobs"
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
	}

	var leadScorer leadscoring.Scorer = leadscoring.NewLocalScorer()
	if cfg.LLMAPIURL != "" && cfg.LLMModel != "" {
		leadScorer = leadscoring.NewFallbackScorer(
			leadscoring.NewRemoteScorer(cfg.LLMAPIURL, cfg.LLMAPIKey, cfg.LLMModel),
			leadscoring.NewLocalScorer(),
		).WithFallbackHook(metricsRegistry.IncLLMFallback)
	}

	leadSvc := leadservice.NewLeadService(leadRepo, redisClient, leadScorer, leadEmbedder, jobStore, metricsRegistry, logger)

	logger.Info("worker started")
	for {
		select {
		case <-ctx.Done():
			logger.Info("worker stopped")
			return
		default:
		}

		job, err := jobStore.ClaimNext(ctx)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				continue
			}
			// redis.Nil from BRPop timeout
			if err.Error() == "redis: nil" {
				continue
			}
			logger.Warn("claim next job failed", "error", err)
			time.Sleep(time.Second)
			continue
		}

		metricsRegistry.IncJob(job.Type, jobs.StatusRunning)
		logger.Info("processing job", "job_id", job.ID, "type", job.Type, "lead_id", job.LeadID)

		switch job.Type {
		case jobs.TypeEmbed:
			result, err := leadSvc.UpsertLeadEmbedding(ctx, job.LeadID)
			if err != nil {
				_ = jobStore.Fail(ctx, job.ID, err)
				metricsRegistry.IncJob(job.Type, jobs.StatusFailed)
				logger.Error("embed job failed", "job_id", job.ID, "error", err)
				continue
			}
			if err := jobStore.Complete(ctx, job.ID, result); err != nil {
				logger.Error("complete embed job failed", "job_id", job.ID, "error", err)
				continue
			}
			metricsRegistry.IncJob(job.Type, jobs.StatusCompleted)
		case jobs.TypeScore:
			result, err := leadSvc.ScoreLead(ctx, job.LeadID)
			if err != nil {
				_ = jobStore.Fail(ctx, job.ID, err)
				metricsRegistry.IncJob(job.Type, jobs.StatusFailed)
				logger.Error("score job failed", "job_id", job.ID, "error", err)
				continue
			}
			if err := jobStore.Complete(ctx, job.ID, result); err != nil {
				logger.Error("complete score job failed", "job_id", job.ID, "error", err)
				continue
			}
			metricsRegistry.IncJob(job.Type, jobs.StatusCompleted)
		default:
			_ = jobStore.Fail(ctx, job.ID, errors.New("unknown job type"))
			metricsRegistry.IncJob(job.Type, jobs.StatusFailed)
		}
	}
}
