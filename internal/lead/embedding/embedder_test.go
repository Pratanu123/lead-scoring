package embedding

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLocalEmbedderIsDeterministic(t *testing.T) {
	embedder := NewLocalEmbedder()
	first, err := embedder.Embed(context.Background(), "acme logistics webinar")
	if err != nil {
		t.Fatalf("Embed returned error: %v", err)
	}
	second, err := embedder.Embed(context.Background(), "acme logistics webinar")
	if err != nil {
		t.Fatalf("Embed returned error: %v", err)
	}

	if first.Model != LocalModel {
		t.Fatalf("expected model %q, got %q", LocalModel, first.Model)
	}
	if first.Vector != second.Vector {
		t.Fatal("expected deterministic local embeddings")
	}
	if !strings.HasPrefix(first.Vector, "[") || !strings.HasSuffix(first.Vector, "]") {
		t.Fatalf("expected pgvector literal, got %q", first.Vector)
	}
}

func TestRemoteEmbedderParsesResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer embed-key" {
			t.Fatalf("unexpected authorization header: %q", r.Header.Get("Authorization"))
		}

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload["model"] != "text-embedding-3-small" {
			t.Fatalf("unexpected model: %#v", payload["model"])
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model": "text-embedding-3-small",
			"data": []map[string]any{
				{"embedding": []float64{0.5, -0.5, 0.0}},
			},
		})
	}))
	defer server.Close()

	embedder := NewRemoteEmbedder(server.URL, "embed-key", "text-embedding-3-small")
	result, err := embedder.Embed(context.Background(), "acme logistics")
	if err != nil {
		t.Fatalf("Embed returned error: %v", err)
	}

	if result.Model != "text-embedding-3-small" {
		t.Fatalf("expected remote model, got %q", result.Model)
	}
	if result.Vector != "[0.500000,-0.500000,0.000000]" {
		t.Fatalf("unexpected vector: %q", result.Vector)
	}
}
