package service

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"lead-scoring/internal/lead/domain"
	"lead-scoring/internal/lead/embedding"
	"lead-scoring/internal/lead/scoring"
	appmetrics "lead-scoring/internal/platform/appmetrics"
	"lead-scoring/internal/platform/jobs"
)

const testLeadID = "11111111-1111-1111-1111-111111111111"

func newTestService(repo *fakeRepository, scorer scoring.Scorer) *LeadService {
	return NewLeadService(repo, nil, scorer, embedding.NewLocalEmbedder(), nil, appmetrics.NewRegistry(), discardLogger())
}

func newTestServiceWithJobs(repo *fakeRepository, queue *fakeJobQueue, scorer scoring.Scorer) *LeadService {
	return NewLeadService(repo, nil, scorer, embedding.NewLocalEmbedder(), queue, appmetrics.NewRegistry(), discardLogger())
}

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

	svc := newTestService(repo, scoring.NewLocalScorer())
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

	svc := newTestService(repo, scoring.NewLocalScorer())
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

	svc := newTestService(repo, scoring.NewLocalScorer())
	if _, err := svc.UpsertLeadEmbedding(context.Background(), testLeadID); err != nil {
		t.Fatalf("UpsertLeadEmbedding returned error: %v", err)
	}

	if repo.upsertEmbeddingCalls != 1 {
		t.Fatalf("expected one upsert after hash change, got %d", repo.upsertEmbeddingCalls)
	}
}

func TestUpdateLeadStatus(t *testing.T) {
	repo := &fakeRepository{
		lead: domain.Lead{ID: testLeadID, Status: "new"},
	}
	svc := newTestService(repo, scoring.NewLocalScorer())
	lead, err := svc.UpdateLeadStatus(context.Background(), testLeadID, "won")
	if err != nil {
		t.Fatalf("UpdateLeadStatus returned error: %v", err)
	}
	if lead.Status != "won" {
		t.Fatalf("expected won, got %q", lead.Status)
	}
}

func TestUpdateLeadStatusRejectsInvalidStatus(t *testing.T) {
	repo := &fakeRepository{lead: domain.Lead{ID: testLeadID, Status: "new"}}
	svc := newTestService(repo, scoring.NewLocalScorer())
	_, err := svc.UpdateLeadStatus(context.Background(), testLeadID, "not-a-status")
	if !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("expected ErrInvalidStatus, got %v", err)
	}
}

func TestCreateLeadEnqueuesEmbedJob(t *testing.T) {
	repo := &fakeRepository{
		lead: domain.Lead{
			ID:          testLeadID,
			CompanyName: "Acme",
			Email:       "buyer@acme.example",
			Source:      "website",
			Status:      "new",
		},
	}
	queue := &fakeJobQueue{}
	svc := newTestServiceWithJobs(repo, queue, scoring.NewLocalScorer())

	lead, err := svc.CreateLead(context.Background(), domain.CreateLeadInput{
		CompanyName: "Acme",
		Email:       "buyer@acme.example",
		Source:      "website",
	})
	if err != nil {
		t.Fatalf("CreateLead returned error: %v", err)
	}
	if lead.ID != testLeadID {
		t.Fatalf("unexpected lead id %q", lead.ID)
	}
	if len(queue.enqueued) != 1 {
		t.Fatalf("expected one enqueue, got %d", len(queue.enqueued))
	}
	if queue.enqueued[0].Type != jobs.TypeEmbed {
		t.Fatalf("expected embed job, got %q", queue.enqueued[0].Type)
	}
	if queue.enqueued[0].LeadID != testLeadID {
		t.Fatalf("expected lead id on job, got %q", queue.enqueued[0].LeadID)
	}
	if repo.upsertEmbeddingCalls != 0 {
		t.Fatalf("expected async create path to skip sync embed, got %d upserts", repo.upsertEmbeddingCalls)
	}
}

func TestEnqueueScoreLeadReturnsJob(t *testing.T) {
	repo := &fakeRepository{lead: domain.Lead{ID: testLeadID}}
	queue := &fakeJobQueue{}
	svc := newTestServiceWithJobs(repo, queue, scoring.NewLocalScorer())

	result, err := svc.EnqueueScoreLead(context.Background(), testLeadID)
	if err != nil {
		t.Fatalf("EnqueueScoreLead returned error: %v", err)
	}
	if result.Type != jobs.TypeScore || result.Status != jobs.StatusQueued {
		t.Fatalf("unexpected enqueue result: %+v", result)
	}
	if result.JobID == "" {
		t.Fatal("expected job id")
	}
	if len(queue.enqueued) != 1 || queue.enqueued[0].Type != jobs.TypeScore {
		t.Fatalf("expected one score job, got %+v", queue.enqueued)
	}
}

func TestEnqueueScoreLeadRequiresQueue(t *testing.T) {
	repo := &fakeRepository{lead: domain.Lead{ID: testLeadID}}
	svc := newTestService(repo, scoring.NewLocalScorer())
	_, err := svc.EnqueueScoreLead(context.Background(), testLeadID)
	if err == nil || !strings.Contains(err.Error(), "job queue unavailable") {
		t.Fatalf("expected queue unavailable error, got %v", err)
	}
}

func TestGetJob(t *testing.T) {
	repo := &fakeRepository{lead: domain.Lead{ID: testLeadID}}
	queue := &fakeJobQueue{
		jobsByID: map[string]jobs.Job{
			"job-1": {ID: "job-1", Type: jobs.TypeScore, LeadID: testLeadID, Status: jobs.StatusCompleted},
		},
	}
	svc := newTestServiceWithJobs(repo, queue, scoring.NewLocalScorer())
	job, err := svc.GetJob(context.Background(), "job-1")
	if err != nil {
		t.Fatalf("GetJob returned error: %v", err)
	}
	if job.Status != jobs.StatusCompleted {
		t.Fatalf("expected completed, got %q", job.Status)
	}
}

func TestCreateLeadValidation(t *testing.T) {
	repo := &fakeRepository{}
	svc := newTestService(repo, scoring.NewLocalScorer())
	_, err := svc.CreateLead(context.Background(), domain.CreateLeadInput{
		CompanyName: "",
		Email:       "bad",
		Source:      "",
	})
	if !errors.Is(err, ErrInvalidLead) {
		t.Fatalf("expected ErrInvalidLead, got %v", err)
	}
}

func TestLatestLeadScoreMapsMissingScore(t *testing.T) {
	repo := &fakeRepository{
		lead:           domain.Lead{ID: testLeadID},
		latestScoreErr: sql.ErrNoRows,
	}

	svc := newTestService(repo, scoring.NewLocalScorer())
	_, err := svc.LatestLeadScore(context.Background(), testLeadID)
	if !errors.Is(err, ErrScoreNotFound) {
		t.Fatalf("expected ErrScoreNotFound, got %v", err)
	}
}

func TestListLeadScoresBoundsLimit(t *testing.T) {
	repo := &fakeRepository{
		lead: domain.Lead{ID: testLeadID},
	}

	svc := newTestService(repo, scoring.NewLocalScorer())
	if _, err := svc.ListLeadScores(context.Background(), testLeadID, 1000); err != nil {
		t.Fatalf("ListLeadScores returned error: %v", err)
	}

	if repo.listScoresLimit != 100 {
		t.Fatalf("expected bounded limit 100, got %d", repo.listScoresLimit)
	}
}

func TestSimilarLeadsBoundsLimit(t *testing.T) {
	repo := &fakeRepository{
		lead: domain.Lead{
			ID:          testLeadID,
			CompanyName: "Acme",
			Email:       "a@b.com",
			Source:      "web",
		},
		similarLeads: []domain.SimilarLead{{ID: "22222222-2222-2222-2222-222222222222", CompanyName: "Peer"}},
	}
	svc := newTestService(repo, scoring.NewLocalScorer())
	items, err := svc.SimilarLeads(context.Background(), testLeadID, 1000)
	if err != nil {
		t.Fatalf("SimilarLeads returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one similar lead, got %d", len(items))
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type fakeJobQueue struct {
	enqueued []jobs.Job
	jobsByID map[string]jobs.Job
}

func (f *fakeJobQueue) Enqueue(_ context.Context, jobType string, leadID string, _ any) (jobs.Job, error) {
	job := jobs.Job{
		ID:     "job-" + jobType + "-" + leadID[:8],
		Type:   jobType,
		LeadID: leadID,
		Status: jobs.StatusQueued,
	}
	f.enqueued = append(f.enqueued, job)
	if f.jobsByID == nil {
		f.jobsByID = map[string]jobs.Job{}
	}
	f.jobsByID[job.ID] = job
	return job, nil
}

func (f *fakeJobQueue) Get(_ context.Context, id string) (jobs.Job, error) {
	job, ok := f.jobsByID[id]
	if !ok {
		return jobs.Job{}, jobs.ErrNotFound
	}
	return job, nil
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

func (f *fakeRepository) Create(_ context.Context, input domain.CreateLeadInput) (domain.Lead, error) {
	if f.lead.ID == "" {
		f.lead = domain.Lead{
			ID:          testLeadID,
			CompanyName: input.CompanyName,
			Email:       input.Email,
			Source:      input.Source,
			Status:      "new",
		}
	}
	return f.lead, nil
}

func (f *fakeRepository) List(context.Context, domain.ListLeadsInput) ([]domain.Lead, error) {
	return []domain.Lead{f.lead}, nil
}

func (f *fakeRepository) GetByID(context.Context, string) (domain.Lead, error) {
	return f.lead, nil
}

func (f *fakeRepository) UpdateStatus(_ context.Context, _ string, status string) (domain.Lead, error) {
	f.lead.Status = status
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
