package service

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"lead-scoring/internal/lead/domain"
	"lead-scoring/internal/lead/embedding"
	"lead-scoring/internal/lead/repository"
	"lead-scoring/internal/lead/scoring"
	appmetrics "lead-scoring/internal/platform/appmetrics"
	"lead-scoring/internal/platform/jobs"

	"github.com/redis/go-redis/v9"
)

var ErrInvalidLead = errors.New("invalid lead")
var ErrLeadNotFound = errors.New("lead not found")
var ErrScoreNotFound = errors.New("lead score not found")
var ErrInvalidStatus = errors.New("invalid status")

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

var allowedStatuses = map[string]struct{}{
	"new":          {},
	"contacted":    {},
	"qualified":    {},
	"won":          {},
	"lost":         {},
	"disqualified": {},
	"converted":    {},
	"customer":     {},
}

const (
	cacheTTL      = 60 * time.Second
	scoreCacheTTL = 5 * time.Minute
)

type LeadService struct {
	repo     repository.Repository
	cache    *redis.Client
	scorer   scoring.Scorer
	embedder embedding.Embedder
	jobs     *jobs.Store
	metrics  *appmetrics.Registry
	logger   *slog.Logger
}

func NewLeadService(
	repo repository.Repository,
	cache *redis.Client,
	scorer scoring.Scorer,
	embedder embedding.Embedder,
	jobStore *jobs.Store,
	metrics *appmetrics.Registry,
	logger *slog.Logger,
) *LeadService {
	if scorer == nil {
		scorer = scoring.NewLocalScorer()
	}
	if embedder == nil {
		embedder = embedding.NewLocalEmbedder()
	}
	if metrics == nil {
		metrics = appmetrics.NewRegistry()
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &LeadService{
		repo:     repo,
		cache:    cache,
		scorer:   scorer,
		embedder: embedder,
		jobs:     jobStore,
		metrics:  metrics,
		logger:   logger,
	}
}

func (s *LeadService) CreateLead(ctx context.Context, input domain.CreateLeadInput) (domain.Lead, error) {
	normalized := domain.CreateLeadInput{
		CompanyName:   strings.TrimSpace(input.CompanyName),
		ContactName:   strings.TrimSpace(input.ContactName),
		Email:         strings.ToLower(strings.TrimSpace(input.Email)),
		Phone:         strings.TrimSpace(input.Phone),
		Source:        strings.TrimSpace(input.Source),
		Industry:      strings.TrimSpace(input.Industry),
		CompanySize:   input.CompanySize,
		AnnualRevenue: input.AnnualRevenue,
		Notes:         strings.TrimSpace(input.Notes),
	}

	if normalized.CompanyName == "" || normalized.Email == "" || normalized.Source == "" {
		return domain.Lead{}, ErrInvalidLead
	}

	if !strings.Contains(normalized.Email, "@") {
		return domain.Lead{}, ErrInvalidLead
	}

	lead, err := s.repo.Create(ctx, normalized)
	if err != nil {
		return domain.Lead{}, err
	}

	if s.jobs != nil {
		if job, err := s.jobs.Enqueue(ctx, jobs.TypeEmbed, lead.ID, map[string]string{"reason": "create"}); err != nil {
			s.logger.Warn("failed to enqueue embed job", "lead_id", lead.ID, "error", err)
		} else {
			s.metrics.IncJob(jobs.TypeEmbed, jobs.StatusQueued)
			s.logger.Info("embed job queued", "lead_id", lead.ID, "job_id", job.ID)
		}
	} else if _, err := s.UpsertLeadEmbedding(ctx, lead.ID); err != nil {
		s.logger.Warn("lead embedding failed after create", "lead_id", lead.ID, "error", err)
	}

	s.cacheLead(ctx, lead)
	s.invalidateLeadListCache(ctx)

	return lead, nil
}

func (s *LeadService) UpdateLeadStatus(ctx context.Context, id string, status string) (domain.Lead, error) {
	id = strings.TrimSpace(id)
	status = strings.ToLower(strings.TrimSpace(status))
	if id == "" || !uuidPattern.MatchString(id) {
		return domain.Lead{}, ErrLeadNotFound
	}
	if _, ok := allowedStatuses[status]; !ok {
		return domain.Lead{}, ErrInvalidStatus
	}

	lead, err := s.repo.UpdateStatus(ctx, id, status)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Lead{}, ErrLeadNotFound
		}
		return domain.Lead{}, err
	}

	s.cacheLead(ctx, lead)
	s.invalidateLeadListCache(ctx)
	s.invalidateScoreCache(ctx, lead.ID)
	return lead, nil
}

func (s *LeadService) ListLeads(ctx context.Context, input domain.ListLeadsInput) ([]domain.Lead, error) {
	if input.Limit <= 0 {
		input.Limit = 20
	}
	if input.Limit > 100 {
		input.Limit = 100
	}
	if input.Offset < 0 {
		input.Offset = 0
	}

	cacheKey := fmt.Sprintf("lead-scoring:leads:list:%d:%d", input.Limit, input.Offset)
	if cached, ok := s.getCachedLeads(ctx, cacheKey); ok {
		s.metrics.IncCacheHit("list")
		return cached, nil
	}
	s.metrics.IncCacheMiss("list")

	leads, err := s.repo.List(ctx, input)
	if err != nil {
		return nil, err
	}

	s.setCache(ctx, cacheKey, leads, cacheTTL)
	return leads, nil
}

func (s *LeadService) GetLead(ctx context.Context, id string) (domain.Lead, error) {
	id = strings.TrimSpace(id)
	if id == "" || !uuidPattern.MatchString(id) {
		return domain.Lead{}, ErrLeadNotFound
	}

	cacheKey := leadCacheKey(id)
	if cached, ok := s.getCachedLead(ctx, cacheKey); ok {
		s.metrics.IncCacheHit("lead")
		return cached, nil
	}
	s.metrics.IncCacheMiss("lead")

	lead, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Lead{}, ErrLeadNotFound
		}

		return domain.Lead{}, err
	}

	s.cacheLead(ctx, lead)
	return lead, nil
}

func (s *LeadService) UpsertLeadEmbedding(ctx context.Context, id string) (domain.EmbeddingResult, error) {
	start := time.Now()
	lead, err := s.GetLead(ctx, id)
	if err != nil {
		return domain.EmbeddingResult{}, err
	}

	_, result, err := s.ensureLeadEmbedding(ctx, lead)
	s.metrics.ObserveEmbeddingDuration(time.Since(start).Milliseconds())
	return result, err
}

func (s *LeadService) SimilarLeads(ctx context.Context, id string, limit int) ([]domain.SimilarLead, error) {
	if limit <= 0 {
		limit = 5
	}
	if limit > 20 {
		limit = 20
	}

	lead, err := s.GetLead(ctx, id)
	if err != nil {
		return nil, err
	}

	vector, _, err := s.ensureLeadEmbedding(ctx, lead)
	if err != nil {
		return nil, err
	}

	return s.repo.FindSimilar(ctx, lead.ID, s.embedder.Model(), vector, limit)
}

func (s *LeadService) EnqueueScoreLead(ctx context.Context, id string) (domain.EnqueueScoreResult, error) {
	lead, err := s.GetLead(ctx, id)
	if err != nil {
		return domain.EnqueueScoreResult{}, err
	}
	if s.jobs == nil {
		return domain.EnqueueScoreResult{}, errors.New("job queue unavailable")
	}

	job, err := s.jobs.Enqueue(ctx, jobs.TypeScore, lead.ID, map[string]string{"reason": "api"})
	if err != nil {
		return domain.EnqueueScoreResult{}, err
	}
	s.metrics.IncJob(jobs.TypeScore, jobs.StatusQueued)

	return domain.EnqueueScoreResult{
		JobID:  job.ID,
		LeadID: lead.ID,
		Status: job.Status,
		Type:   job.Type,
	}, nil
}

func (s *LeadService) ScoreLead(ctx context.Context, id string) (domain.ScoreLeadResult, error) {
	start := time.Now()
	lead, err := s.GetLead(ctx, id)
	if err != nil {
		s.metrics.IncScoreRequest("error")
		return domain.ScoreLeadResult{}, err
	}

	similarLeads, err := s.SimilarLeads(ctx, id, 5)
	if err != nil {
		s.metrics.IncScoreRequest("error")
		return domain.ScoreLeadResult{}, err
	}

	decision, err := s.scorer.Score(ctx, lead, similarLeads)
	if err != nil {
		s.metrics.IncScoreRequest("error")
		return domain.ScoreLeadResult{}, err
	}

	score, err := s.repo.CreateScore(ctx, lead.ID, decision.ConversionProbability, decision.Reasoning, decision.Model)
	if err != nil {
		s.metrics.IncScoreRequest("error")
		return domain.ScoreLeadResult{}, err
	}

	result := domain.ScoreLeadResult{
		Score:        score,
		SimilarLeads: similarLeads,
	}
	s.cacheScore(ctx, lead, score)
	s.metrics.IncScoreRequest("success")
	s.metrics.ObserveScoreDuration(time.Since(start).Milliseconds())
	return result, nil
}

func (s *LeadService) LatestLeadScore(ctx context.Context, id string) (domain.LeadScore, error) {
	lead, err := s.GetLead(ctx, id)
	if err != nil {
		return domain.LeadScore{}, err
	}

	if score, ok := s.getCachedScore(ctx, lead); ok {
		s.metrics.IncCacheHit("score")
		return score, nil
	}
	s.metrics.IncCacheMiss("score")

	score, err := s.repo.GetLatestScore(ctx, lead.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.LeadScore{}, ErrScoreNotFound
		}
		return domain.LeadScore{}, err
	}

	s.cacheScore(ctx, lead, score)
	return score, nil
}

func (s *LeadService) ListLeadScores(ctx context.Context, id string, limit int) ([]domain.LeadScore, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	lead, err := s.GetLead(ctx, id)
	if err != nil {
		return nil, err
	}

	return s.repo.ListScores(ctx, lead.ID, limit)
}

func (s *LeadService) GetJob(ctx context.Context, id string) (jobs.Job, error) {
	if s.jobs == nil {
		return jobs.Job{}, jobs.ErrNotFound
	}
	return s.jobs.Get(ctx, id)
}

func (s *LeadService) ensureLeadEmbedding(ctx context.Context, lead domain.Lead) (string, domain.EmbeddingResult, error) {
	content := leadEmbeddingContent(lead)
	contentHash := hashText(content)
	model := s.embedder.Model()

	existing, err := s.repo.GetEmbedding(ctx, lead.ID, model)
	if err == nil && existing.ContentHash == contentHash {
		return existing.Vector, domain.EmbeddingResult{
			LeadID:      existing.LeadID,
			Model:       existing.Model,
			ContentHash: existing.ContentHash,
			CreatedAt:   existing.CreatedAt,
		}, nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", domain.EmbeddingResult{}, err
	}

	embedded, err := s.embedder.Embed(ctx, content)
	if err != nil {
		return "", domain.EmbeddingResult{}, err
	}

	result, err := s.repo.UpsertEmbedding(ctx, lead.ID, model, contentHash, embedded.Vector)
	if err != nil {
		return "", domain.EmbeddingResult{}, err
	}

	return embedded.Vector, result, nil
}

func leadCacheKey(id string) string {
	return "lead-scoring:lead:" + id
}

func scoreCacheKey(leadID string, contentHash string) string {
	return "lead-scoring:score:" + leadID + ":" + contentHash
}

func (s *LeadService) cacheLead(ctx context.Context, lead domain.Lead) {
	s.setCache(ctx, leadCacheKey(lead.ID), lead, cacheTTL)
}

func (s *LeadService) cacheScore(ctx context.Context, lead domain.Lead, score domain.LeadScore) {
	hash := hashText(leadEmbeddingContent(lead))
	s.setCache(ctx, scoreCacheKey(lead.ID, hash), score, scoreCacheTTL)
}

func (s *LeadService) getCachedScore(ctx context.Context, lead domain.Lead) (domain.LeadScore, bool) {
	if s.cache == nil {
		return domain.LeadScore{}, false
	}
	hash := hashText(leadEmbeddingContent(lead))
	value, err := s.cache.Get(ctx, scoreCacheKey(lead.ID, hash)).Result()
	if err != nil {
		return domain.LeadScore{}, false
	}
	var score domain.LeadScore
	if err := json.Unmarshal([]byte(value), &score); err != nil {
		return domain.LeadScore{}, false
	}
	return score, true
}

func (s *LeadService) invalidateScoreCache(ctx context.Context, leadID string) {
	if s.cache == nil {
		return
	}
	var cursor uint64
	pattern := "lead-scoring:score:" + leadID + ":*"
	for {
		keys, nextCursor, err := s.cache.Scan(ctx, cursor, pattern, 50).Result()
		if err != nil {
			return
		}
		if len(keys) > 0 {
			_ = s.cache.Del(ctx, keys...).Err()
		}
		if nextCursor == 0 {
			return
		}
		cursor = nextCursor
	}
}

func (s *LeadService) getCachedLead(ctx context.Context, key string) (domain.Lead, bool) {
	if s.cache == nil {
		return domain.Lead{}, false
	}

	value, err := s.cache.Get(ctx, key).Result()
	if err != nil {
		return domain.Lead{}, false
	}

	var lead domain.Lead
	if err := json.Unmarshal([]byte(value), &lead); err != nil {
		return domain.Lead{}, false
	}

	return lead, true
}

func (s *LeadService) getCachedLeads(ctx context.Context, key string) ([]domain.Lead, bool) {
	if s.cache == nil {
		return nil, false
	}

	value, err := s.cache.Get(ctx, key).Result()
	if err != nil {
		return nil, false
	}

	var leads []domain.Lead
	if err := json.Unmarshal([]byte(value), &leads); err != nil {
		return nil, false
	}

	return leads, true
}

func (s *LeadService) setCache(ctx context.Context, key string, payload any, ttl time.Duration) {
	if s.cache == nil {
		return
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return
	}

	_ = s.cache.Set(ctx, key, data, ttl).Err()
}

func (s *LeadService) invalidateLeadListCache(ctx context.Context) {
	if s.cache == nil {
		return
	}

	var cursor uint64
	for {
		keys, nextCursor, err := s.cache.Scan(ctx, cursor, "lead-scoring:leads:list:*", 50).Result()
		if err != nil {
			return
		}
		if len(keys) > 0 {
			_ = s.cache.Del(ctx, keys...).Err()
		}
		if nextCursor == 0 {
			return
		}
		cursor = nextCursor
	}
}

func leadEmbeddingContent(lead domain.Lead) string {
	parts := []string{
		lead.CompanyName,
		lead.ContactName,
		lead.Email,
		lead.Source,
		lead.Industry,
		fmt.Sprintf("%d", lead.CompanySize),
		fmt.Sprintf("%.2f", lead.AnnualRevenue),
		lead.Notes,
	}
	return strings.ToLower(strings.Join(parts, " "))
}

func hashText(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
