package controller

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"lead-scoring/internal/lead/domain"
	"lead-scoring/internal/lead/embedding"
	"lead-scoring/internal/lead/scoring"
	"lead-scoring/internal/lead/service"
	appmetrics "lead-scoring/internal/platform/appmetrics"
	"lead-scoring/internal/platform/jobs"
)

const controllerLeadID = "11111111-1111-1111-1111-111111111111"

func TestUpdateLeadStatusHandler(t *testing.T) {
	repo := &controllerFakeRepo{lead: domain.Lead{ID: controllerLeadID, Status: "new"}}
	svc := service.NewLeadService(repo, nil, scoring.NewLocalScorer(), embedding.NewLocalEmbedder(), nil, appmetrics.NewRegistry(), discardControllerLogger())
	handler := NewLeadHandler(svc, discardControllerLogger(), nil, nil)

	body := bytes.NewBufferString(`{"status":"qualified"}`)
	req := httptest.NewRequest(http.MethodPatch, "/v1/leads/"+controllerLeadID, body)
	req.SetPathValue("id", controllerLeadID)
	rec := httptest.NewRecorder()

	handler.UpdateLeadStatus(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var lead domain.Lead
	if err := json.Unmarshal(rec.Body.Bytes(), &lead); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if lead.Status != "qualified" {
		t.Fatalf("expected qualified, got %q", lead.Status)
	}
}

func TestScoreLeadHandlerEnqueuesJob(t *testing.T) {
	repo := &controllerFakeRepo{lead: domain.Lead{ID: controllerLeadID}}
	queue := &controllerFakeQueue{}
	svc := service.NewLeadService(repo, nil, scoring.NewLocalScorer(), embedding.NewLocalEmbedder(), queue, appmetrics.NewRegistry(), discardControllerLogger())
	handler := NewLeadHandler(svc, discardControllerLogger(), nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/v1/leads/"+controllerLeadID+"/score", nil)
	req.SetPathValue("id", controllerLeadID)
	rec := httptest.NewRecorder()

	handler.ScoreLead(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload domain.EnqueueScoreResult
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.JobID == "" || payload.Type != jobs.TypeScore {
		t.Fatalf("unexpected enqueue payload: %+v", payload)
	}
}

func TestGetJobHandler(t *testing.T) {
	repo := &controllerFakeRepo{lead: domain.Lead{ID: controllerLeadID}}
	queue := &controllerFakeQueue{
		jobsByID: map[string]jobs.Job{
			"job-9": {ID: "job-9", Type: jobs.TypeScore, LeadID: controllerLeadID, Status: jobs.StatusRunning},
		},
	}
	svc := service.NewLeadService(repo, nil, scoring.NewLocalScorer(), embedding.NewLocalEmbedder(), queue, appmetrics.NewRegistry(), discardControllerLogger())
	handler := NewLeadHandler(svc, discardControllerLogger(), nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/jobs/job-9", nil)
	req.SetPathValue("id", "job-9")
	rec := httptest.NewRecorder()
	handler.GetJob(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func discardControllerLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type controllerFakeQueue struct {
	jobsByID map[string]jobs.Job
}

func (f *controllerFakeQueue) Enqueue(_ context.Context, jobType string, leadID string, _ any) (jobs.Job, error) {
	job := jobs.Job{ID: "job-score-1", Type: jobType, LeadID: leadID, Status: jobs.StatusQueued}
	if f.jobsByID == nil {
		f.jobsByID = map[string]jobs.Job{}
	}
	f.jobsByID[job.ID] = job
	return job, nil
}

func (f *controllerFakeQueue) Get(_ context.Context, id string) (jobs.Job, error) {
	job, ok := f.jobsByID[id]
	if !ok {
		return jobs.Job{}, jobs.ErrNotFound
	}
	return job, nil
}

type controllerFakeRepo struct {
	lead domain.Lead
}

func (f *controllerFakeRepo) Create(context.Context, domain.CreateLeadInput) (domain.Lead, error) {
	return f.lead, nil
}
func (f *controllerFakeRepo) List(context.Context, domain.ListLeadsInput) ([]domain.Lead, error) {
	return []domain.Lead{f.lead}, nil
}
func (f *controllerFakeRepo) GetByID(context.Context, string) (domain.Lead, error) {
	return f.lead, nil
}
func (f *controllerFakeRepo) UpdateStatus(_ context.Context, _ string, status string) (domain.Lead, error) {
	f.lead.Status = status
	return f.lead, nil
}
func (f *controllerFakeRepo) GetEmbedding(context.Context, string, string) (domain.EmbeddingRecord, error) {
	return domain.EmbeddingRecord{}, sql.ErrNoRows
}
func (f *controllerFakeRepo) UpsertEmbedding(context.Context, string, string, string, string) (domain.EmbeddingResult, error) {
	return domain.EmbeddingResult{LeadID: f.lead.ID}, nil
}
func (f *controllerFakeRepo) FindSimilar(context.Context, string, string, string, int) ([]domain.SimilarLead, error) {
	return nil, nil
}
func (f *controllerFakeRepo) CreateScore(context.Context, string, float64, string, string) (domain.LeadScore, error) {
	return domain.LeadScore{}, nil
}
func (f *controllerFakeRepo) GetLatestScore(context.Context, string) (domain.LeadScore, error) {
	return domain.LeadScore{}, sql.ErrNoRows
}
func (f *controllerFakeRepo) ListScores(context.Context, string, int) ([]domain.LeadScore, error) {
	return nil, nil
}
