package scoring

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
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
	}, []domain.SimilarLead{{CompanyName: "Peer", Status: "won", Similarity: 0.8}})
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

func TestRemoteScorerRedactsPIIAndParsesStructuredResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("unexpected authorization header: %q", r.Header.Get("Authorization"))
		}

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		messages, ok := payload["messages"].([]any)
		if !ok || len(messages) < 2 {
			t.Fatalf("expected chat messages, got %#v", payload["messages"])
		}
		userMessage := messages[1].(map[string]any)
		content := userMessage["content"].(string)
		if strings.Contains(content, "secret@acme.example") || strings.Contains(content, "+91-9999999999") {
			t.Fatalf("expected PII redaction in LLM payload, got %s", content)
		}
		if !strings.Contains(content, "company_size") || !strings.Contains(content, "Ready to buy") {
			t.Fatalf("expected enriched RAG context in LLM payload, got %s", content)
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
		Email:       "secret@acme.example",
		Phone:       "+91-9999999999",
		Source:      "referral",
		Notes:       "Ready to buy",
	}, []domain.SimilarLead{{
		CompanyName: "Peer",
		Email:       "peer@example.com",
		CompanySize: 500,
		Notes:       "Closed successfully",
		Status:      "won",
		Similarity:  0.9,
	}})
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

func TestFallbackScorerUsesLocalWhenRemoteFails(t *testing.T) {
	scorer := NewFallbackScorer(&failingScorer{err: errors.New("remote down")}, NewLocalScorer())
	decision, err := scorer.Score(context.Background(), domain.Lead{
		CompanyName: "Acme",
		Source:      "referral",
		CompanySize: 1000,
	}, nil)
	if err != nil {
		t.Fatalf("Score returned error: %v", err)
	}

	if decision.Model != LocalModel {
		t.Fatalf("expected fallback model %q, got %q", LocalModel, decision.Model)
	}
	if !strings.Contains(decision.Reasoning, "Fallback used after remote scorer failure") {
		t.Fatalf("expected fallback note in reasoning, got %q", decision.Reasoning)
	}
}

type failingScorer struct {
	err error
}

func (s *failingScorer) Score(context.Context, domain.Lead, []domain.SimilarLead) (Decision, error) {
	return Decision{}, s.err
}
