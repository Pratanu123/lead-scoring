package service

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"lead-scoring/internal/lead/domain"
	"lead-scoring/internal/lead/embedding"
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
				Status:      "won",
				Similarity:  0.82,
			},
		},
	}

	svc := NewLeadService(repo, nil, scoring.NewLocalScorer(), embedding.NewLocalEmbedder(), discardLogger())
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
	if repo.findSimilarModel != embedding.LocalModel {
		t.Fatalf("expected FindSimilar model %q, got %q", embedding.LocalModel, repo.findSimilarModel)
	}
}

func TestUpsertLeadEmbeddingSkipsUnchangedContentHash(t *testing.T) {
	content := leadEmbeddingContent(domain.Lead{
		ID:          testLeadID,
		CompanyName: "Acme Logistics",
		Email:       "buyer@acme.example",
		Source:      "referral",
	})
	contentHash := hashText(content)
	repo := &fakeRepository{
		lead: domain.Lead{
			ID:          testLeadID,
			CompanyName: "Acme Logistics",
			Email:       "buyer@acme.example",
			Source:      "referral",
		},
		embedding: domain.EmbeddingRecord{
			LeadID:      testLeadID,
			Model:       embedding.LocalModel,
			ContentHash: contentHash,
			Vector:      "[0.1,0.2]",
			CreatedAt:   time.Now(),
		},
	}

	svc := NewLeadService(repo, nil, scoring.NewLocalScorer(), embedding.NewLocalEmbedder(), discardLogger())
	result, err := svc.UpsertLeadEmbedding(context.Background(), testLeadID)
	if err != nil {
		t.Fatalf("UpsertLeadEmbedding returned error: %v", err)
	}

	if repo.upsertEmbeddingCalls != 0 {
		t.Fatalf("expected content-hash skip with zero upserts, got %d", repo.upsertEmbeddingCalls)
	}
	if result.ContentHash != contentHash {
		t.Fatalf("expected content hash %q, got %q", contentHash, result.ContentHash)
	}
}

func TestUpsertLeadEmbeddingWritesWhenHashChanges(t *testing.T) {
	repo := &fakeRepository{
		lead: domain.Lead{
			ID:          testLeadID,
			CompanyName: "Acme Logistics",
			Email:       "buyer@acme.example",
			Source:      "referral",
			Notes:       "updated notes",
		},
		embedding: domain.EmbeddingRecord{
			LeadID:      testLeadID,
			Model:       embedding.LocalModel,
			ContentHash: "stale-hash",
			Vector:      "[0.1,0.2]",
			CreatedAt:   time.Now(),
		},
	}

	svc := NewLeadService(repo, nil, scoring.NewLocalScorer(), embedding.NewLocalEmbedder(), discardLogger())
	if _, err := svc.UpsertLeadEmbedding(context.Background(), testLeadID); err != nil {
		t.Fatalf("UpsertLeadEmbedding returned error: %v", err)
	}

	if repo.upsertEmbeddingCalls != 1 {
		t.Fatalf("expected one upsert after hash change, got %d", repo.upsertEmbeddingCalls)
	}
}

func TestLatestLeadScoreMapsMissingScore(t *testing.T) {
	repo := &fakeRepository{
		lead:           domain.Lead{ID: testLeadID},
		latestScoreErr: sql.ErrNoRows,
	}

	svc := NewLeadService(repo, nil, scoring.NewLocalScorer(), embedding.NewLocalEmbedder(), discardLogger())
	_, err := svc.LatestLeadScore(context.Background(), testLeadID)
	if !errors.Is(err, ErrScoreNotFound) {
		t.Fatalf("expected ErrScoreNotFound, got %v", err)
	}
}

func TestListLeadScoresBoundsLimit(t *testing.T) {
	repo := &fakeRepository{
		lead: domain.Lead{ID: testLeadID},
	}

	svc := NewLeadService(repo, nil, scoring.NewLocalScorer(), embedding.NewLocalEmbedder(), discardLogger())
	if _, err := svc.ListLeadScores(context.Background(), testLeadID, 1000); err != nil {
		t.Fatalf("ListLeadScores returned error: %v", err)
	}

	if repo.listScoresLimit != 100 {
		t.Fatalf("expected bounded limit 100, got %d", repo.listScoresLimit)
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type fakeRepository struct {
	lead                 domain.Lead
	similarLeads         []domain.SimilarLead
	embedding            domain.EmbeddingRecord
	getEmbeddingErr      error
	latestScore          domain.LeadScore
	latestScoreErr       error
	createScoreCalls     int
	upsertEmbeddingCalls int
	listScoresLimit      int
	findSimilarModel     string
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

func (f *fakeRepository) GetEmbedding(context.Context, string, string) (domain.EmbeddingRecord, error) {
	if f.getEmbeddingErr != nil {
		return domain.EmbeddingRecord{}, f.getEmbeddingErr
	}
	if f.embedding.LeadID == "" {
		return domain.EmbeddingRecord{}, sql.ErrNoRows
	}
	return f.embedding, nil
}

func (f *fakeRepository) UpsertEmbedding(_ context.Context, leadID string, model string, contentHash string, _ string) (domain.EmbeddingResult, error) {
	f.upsertEmbeddingCalls++
	result := domain.EmbeddingResult{
		LeadID:      leadID,
		Model:       model,
		ContentHash: contentHash,
		CreatedAt:   time.Now(),
	}
	f.embedding = domain.EmbeddingRecord{
		LeadID:      leadID,
		Model:       model,
		ContentHash: contentHash,
		Vector:      "[1.0]",
		CreatedAt:   result.CreatedAt,
	}
	return result, nil
}

func (f *fakeRepository) FindSimilar(_ context.Context, _ string, model string, _ string, _ int) ([]domain.SimilarLead, error) {
	f.findSimilarModel = model
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
