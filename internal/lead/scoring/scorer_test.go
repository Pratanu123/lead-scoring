package scoring

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"lead-scoring/internal/lead/domain"
)

func TestLocalScorerReturnsGroundedDecision(t *testing.T) {
	scorer := NewLocalScorer()
	decision, err := scorer.Score(context.Background(), domain.Lead{
		CompanyName:   "Acme",
		Source:        "referral",
		CompanySize:   1000,
		AnnualRevenue: 20000000,
	}, []domain.SimilarLead{{CompanyName: "Peer", Similarity: 0.8}})
	if err != nil {
		t.Fatalf("Score returned error: %v", err)
	}

	if decision.Model != LocalModel {
		t.Fatalf("expected model %q, got %q", LocalModel, decision.Model)
	}
	if decision.ConversionProbability <= 0.5 {
		t.Fatalf("expected probability above 0.5, got %.4f", decision.ConversionProbability)
	}
	if decision.Reasoning == "" {
		t.Fatal("expected non-empty reasoning")
	}
}

func TestRemoteScorerParsesStructuredResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("unexpected authorization header: %q", r.Header.Get("Authorization"))
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]string{
						"content": `{"conversion_probability":0.73,"reasoning":"Strong fit based on referral source and similar leads."}`,
					},
				},
			},
		})
	}))
	defer server.Close()

	scorer := NewRemoteScorer(server.URL, "test-key", "test-model")
	decision, err := scorer.Score(context.Background(), domain.Lead{
		CompanyName: "Acme",
		Source:      "referral",
	}, nil)
	if err != nil {
		t.Fatalf("Score returned error: %v", err)
	}

	if decision.Model != "test-model" {
		t.Fatalf("expected test-model, got %q", decision.Model)
	}
	if decision.ConversionProbability != 0.73 {
		t.Fatalf("expected probability 0.73, got %.4f", decision.ConversionProbability)
	}
}
