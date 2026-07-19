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

	"github.com/redis/go-redis/v9"
)

var ErrInvalidLead = errors.New("invalid lead")
var ErrLeadNotFound = errors.New("lead not found")
var ErrScoreNotFound = errors.New("lead score not found")

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

const cacheTTL = 60 * time.Second

type LeadService struct {
	repo     repository.Repository
	cache    *redis.Client
	scorer   scoring.Scorer
	embedder embedding.Embedder
	logger   *slog.Logger
}

func NewLeadService(
	repo repository.Repository,
	cache *redis.Client,
	scorer scoring.Scorer,
	embedder embedding.Embedder,
	logger *slog.Logger,
) *LeadService {
	if scorer == nil {
		scorer = scoring.NewLocalScorer()
	}
	if embedder == nil {
		embedder = embedding.NewLocalEmbedder()
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &LeadService{
		repo:     repo,
		cache:    cache,
		scorer:   scorer,
		embedder: embedder,
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

	if _, err := s.UpsertLeadEmbedding(ctx, lead.ID); err != nil {
		s.logger.Warn("lead embedding failed after create", "lead_id", lead.ID, "error", err)
	}
	s.cacheLead(ctx, lead)
	s.invalidateLeadListCache(ctx)

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
		return cached, nil
	}

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
		return cached, nil
	}

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
	lead, err := s.GetLead(ctx, id)
	if err != nil {
		return domain.EmbeddingResult{}, err
	}

	_, result, err := s.ensureLeadEmbedding(ctx, lead)
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

func (s *LeadService) ScoreLead(ctx context.Context, id string) (domain.ScoreLeadResult, error) {
	lead, err := s.GetLead(ctx, id)
	if err != nil {
		return domain.ScoreLeadResult{}, err
	}

	similarLeads, err := s.SimilarLeads(ctx, id, 5)
	if err != nil {
		return domain.ScoreLeadResult{}, err
	}

	decision, err := s.scorer.Score(ctx, lead, similarLeads)
	if err != nil {
		return domain.ScoreLeadResult{}, err
	}

	score, err := s.repo.CreateScore(ctx, lead.ID, decision.ConversionProbability, decision.Reasoning, decision.Model)
	if err != nil {
		return domain.ScoreLeadResult{}, err
	}

	return domain.ScoreLeadResult{
		Score:        score,
		SimilarLeads: similarLeads,
	}, nil
}

func (s *LeadService) LatestLeadScore(ctx context.Context, id string) (domain.LeadScore, error) {
	lead, err := s.GetLead(ctx, id)
	if err != nil {
		return domain.LeadScore{}, err
	}

	score, err := s.repo.GetLatestScore(ctx, lead.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.LeadScore{}, ErrScoreNotFound
		}
		return domain.LeadScore{}, err
	}

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

func (s *LeadService) cacheLead(ctx context.Context, lead domain.Lead) {
	s.setCache(ctx, leadCacheKey(lead.ID), lead, cacheTTL)
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
