package service

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"lead-scoring/internal/lead/domain"
	"lead-scoring/internal/lead/scoring"
)

const testLeadID = "11111111-1111-1111-1111-111111111111"

func TestScoreLeadPersistsRAGResult(t *testing.T) {
	repo := &fakeRepository{
		lead: domain.Lead{
			ID:            testLeadID,
			CompanyName:   "Acme Logistics",
			Email:         "buyer@acme.example",
			Source:        "referral",
			Industry:      "logistics",
			CompanySize:   1200,
			AnnualRevenue: 25000000,
			Notes:         "Ready to evaluate CRM automation",
		},
		similarLeads: []domain.SimilarLead{
			{
				ID:          "22222222-2222-2222-2222-222222222222",
				CompanyName: "Converted Logistics Co",
				Similarity:  0.82,
			},
		},
	}

	svc := NewLeadService(repo, nil, scoring.NewLocalScorer())
	result, err := svc.ScoreLead(context.Background(), testLeadID)
	if err != nil {
		t.Fatalf("ScoreLead returned error: %v", err)
	}

	if repo.createScoreCalls != 1 {
		t.Fatalf("expected one persisted score, got %d", repo.createScoreCalls)
	}
	if result.Score.Model != scoring.LocalModel {
		t.Fatalf("expected model %q, got %q", scoring.LocalModel, result.Score.Model)
	}
	if result.Score.ConversionProbability <= 0.5 {
		t.Fatalf("expected strong conversion probability, got %.4f", result.Score.ConversionProbability)
	}
	if len(result.SimilarLeads) != 1 {
		t.Fatalf("expected one similar lead, got %d", len(result.SimilarLeads))
	}
}

func TestLatestLeadScoreMapsMissingScore(t *testing.T) {
	repo := &fakeRepository{
		lead:           domain.Lead{ID: testLeadID},
		latestScoreErr: sql.ErrNoRows,
	}

	svc := NewLeadService(repo, nil, scoring.NewLocalScorer())
	_, err := svc.LatestLeadScore(context.Background(), testLeadID)
	if !errors.Is(err, ErrScoreNotFound) {
		t.Fatalf("expected ErrScoreNotFound, got %v", err)
	}
}

func TestListLeadScoresBoundsLimit(t *testing.T) {
	repo := &fakeRepository{
		lead: domain.Lead{ID: testLeadID},
	}

	svc := NewLeadService(repo, nil, scoring.NewLocalScorer())
	if _, err := svc.ListLeadScores(context.Background(), testLeadID, 1000); err != nil {
		t.Fatalf("ListLeadScores returned error: %v", err)
	}

	if repo.listScoresLimit != 100 {
		t.Fatalf("expected bounded limit 100, got %d", repo.listScoresLimit)
	}
}

type fakeRepository struct {
	lead             domain.Lead
	similarLeads     []domain.SimilarLead
	latestScore      domain.LeadScore
	latestScoreErr   error
	createScoreCalls int
	listScoresLimit  int
}

func (f *fakeRepository) Create(context.Context, domain.CreateLeadInput) (domain.Lead, error) {
	return f.lead, nil
}

func (f *fakeRepository) List(context.Context, domain.ListLeadsInput) ([]domain.Lead, error) {
	return []domain.Lead{f.lead}, nil
}

func (f *fakeRepository) GetByID(context.Context, string) (domain.Lead, error) {
	return f.lead, nil
}

func (f *fakeRepository) UpsertEmbedding(context.Context, string, string, string, string) (domain.EmbeddingResult, error) {
	return domain.EmbeddingResult{
		LeadID:    f.lead.ID,
		Model:     localEmbeddingModel,
		CreatedAt: time.Now(),
	}, nil
}

func (f *fakeRepository) FindSimilar(context.Context, string, string, int) ([]domain.SimilarLead, error) {
	return f.similarLeads, nil
}

func (f *fakeRepository) CreateScore(_ context.Context, leadID string, probability float64, reasoning string, model string) (domain.LeadScore, error) {
	f.createScoreCalls++
	score := domain.LeadScore{
		ID:                    "33333333-3333-3333-3333-333333333333",
		LeadID:                leadID,
		ConversionProbability: probability,
		Reasoning:             reasoning,
		Model:                 model,
		CreatedAt:             time.Now(),
	}
	f.latestScore = score
	return score, nil
}

func (f *fakeRepository) GetLatestScore(context.Context, string) (domain.LeadScore, error) {
	if f.latestScoreErr != nil {
		return domain.LeadScore{}, f.latestScoreErr
	}
	return f.latestScore, nil
}

func (f *fakeRepository) ListScores(_ context.Context, _ string, limit int) ([]domain.LeadScore, error) {
	f.listScoresLimit = limit
	return []domain.LeadScore{f.latestScore}, nil
}
