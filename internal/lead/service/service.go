package service

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"

	"lead-scoring/internal/lead/domain"
	"lead-scoring/internal/lead/repository"

	"github.com/redis/go-redis/v9"
)

var ErrInvalidLead = errors.New("invalid lead")
var ErrLeadNotFound = errors.New("lead not found")

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

const (
	cacheTTL            = 60 * time.Second
	embeddingDimensions = 1536
	localEmbeddingModel = "local-hash-embedding-v1"
	localScoringModel   = "local-rag-scorer-v1"
)

type LeadService struct {
	repo  repository.Repository
	cache *redis.Client
}

func NewLeadService(repo repository.Repository, cache *redis.Client) *LeadService {
	return &LeadService{repo: repo, cache: cache}
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

	_, _ = s.UpsertLeadEmbedding(ctx, lead.ID)
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

	content := leadEmbeddingContent(lead)
	contentHash := hashText(content)
	vector := deterministicVector(content)

	return s.repo.UpsertEmbedding(ctx, lead.ID, localEmbeddingModel, contentHash, vector)
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

	content := leadEmbeddingContent(lead)
	vector := deterministicVector(content)
	if _, err := s.repo.UpsertEmbedding(ctx, lead.ID, localEmbeddingModel, hashText(content), vector); err != nil {
		return nil, err
	}

	return s.repo.FindSimilar(ctx, lead.ID, vector, limit)
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

	probability := conversionProbability(lead, similarLeads)
	reasoning := scoreReasoning(lead, similarLeads, probability)
	score, err := s.repo.CreateScore(ctx, lead.ID, probability, reasoning, localScoringModel)
	if err != nil {
		return domain.ScoreLeadResult{}, err
	}

	return domain.ScoreLeadResult{
		Score:        score,
		SimilarLeads: similarLeads,
	}, nil
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

func deterministicVector(content string) string {
	vector := make([]float64, embeddingDimensions)
	tokens := strings.Fields(content)
	if len(tokens) == 0 {
		tokens = []string{"empty"}
	}

	for _, token := range tokens {
		hash := fnv.New64a()
		_, _ = hash.Write([]byte(token))
		sum := hash.Sum64()
		index := int(sum % embeddingDimensions)
		weight := 1.0
		if sum%2 == 0 {
			weight = -1.0
		}
		vector[index] += weight
	}

	var magnitude float64
	for _, value := range vector {
		magnitude += value * value
	}
	magnitude = math.Sqrt(magnitude)
	if magnitude == 0 {
		magnitude = 1
	}

	values := make([]string, len(vector))
	for i, value := range vector {
		values[i] = fmt.Sprintf("%.6f", value/magnitude)
	}

	return "[" + strings.Join(values, ",") + "]"
}

func conversionProbability(lead domain.Lead, similarLeads []domain.SimilarLead) float64 {
	score := 0.25

	switch strings.ToLower(lead.Source) {
	case "referral":
		score += 0.20
	case "webinar":
		score += 0.15
	case "inbound", "website":
		score += 0.12
	case "manual":
		score += 0.05
	}

	if lead.CompanySize >= 1000 {
		score += 0.14
	} else if lead.CompanySize >= 250 {
		score += 0.10
	} else if lead.CompanySize >= 50 {
		score += 0.06
	}

	if lead.AnnualRevenue >= 10000000 {
		score += 0.12
	} else if lead.AnnualRevenue >= 1000000 {
		score += 0.07
	}

	if strings.TrimSpace(lead.Notes) != "" {
		score += 0.05
	}

	if len(similarLeads) > 0 {
		similarities := make([]float64, 0, len(similarLeads))
		for _, similar := range similarLeads {
			similarities = append(similarities, similar.Similarity)
		}
		sort.Sort(sort.Reverse(sort.Float64Slice(similarities)))
		score += math.Max(0, similarities[0]) * 0.18
	}

	if score > 0.95 {
		return 0.95
	}
	if score < 0.05 {
		return 0.05
	}
	return math.Round(score*10000) / 10000
}

func scoreReasoning(lead domain.Lead, similarLeads []domain.SimilarLead, probability float64) string {
	reasons := []string{
		fmt.Sprintf("Lead %s has a %.1f%% estimated conversion probability.", lead.CompanyName, probability*100),
	}

	if strings.TrimSpace(lead.Source) != "" {
		reasons = append(reasons, fmt.Sprintf("Source signal is %q.", lead.Source))
	}
	if lead.CompanySize > 0 {
		reasons = append(reasons, fmt.Sprintf("Company size signal is %d employees.", lead.CompanySize))
	}
	if lead.AnnualRevenue > 0 {
		reasons = append(reasons, fmt.Sprintf("Revenue signal is %.0f.", lead.AnnualRevenue))
	}
	if len(similarLeads) > 0 {
		reasons = append(reasons, fmt.Sprintf("RAG found %d similar lead(s); closest match is %s with %.2f similarity.", len(similarLeads), similarLeads[0].CompanyName, similarLeads[0].Similarity))
	} else {
		reasons = append(reasons, "RAG found no embedded historical leads yet, so the score relies on lead attributes.")
	}

	return strings.Join(reasons, " ")
}
