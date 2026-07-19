package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequiresAuth(t *testing.T) {
	cases := map[string]bool{
		"/healthz":              false,
		"/metrics":              false,
		"/":                     false,
		"/leads/abc":            false,
		"/assets/index.js":      false,
		"/v1/leads":             true,
		"/v1/leads/1/score":     true,
		"/v1/ws":                true,
		"/create-lead":          true,
		"/v1/create-leads":      true,
		"/v1/get-leads":         true,
		"/v1/get-leads/some-id": true,
	}

	for path, want := range cases {
		if got := requiresAuth(path); got != want {
			t.Fatalf("requiresAuth(%q)=%v, want %v", path, got, want)
		}
	}
}

func TestExtractBearerTokenFromHeaderAndQuery(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/leads", nil)
	req.Header.Set("Authorization", "Bearer secret-key")
	if got := ExtractBearerToken(req); got != "secret-key" {
		t.Fatalf("expected secret-key, got %q", got)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/ws?token=query-key", nil)
	if got := ExtractBearerToken(req); got != "query-key" {
		t.Fatalf("expected query-key, got %q", got)
	}
}

func TestMiddlewareRejectsMissingAndInvalidKeys(t *testing.T) {
	mw := NewMiddleware("dev-key", nil, 10, 5)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	handler := mw.Wrap(next)

	req := httptest.NewRequest(http.MethodGet, "/v1/leads", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without key, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/leads", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for wrong key, got %d", rec.Code)
	}
}

func TestMiddlewareAllowsValidKeyAndPublicRoutes(t *testing.T) {
	mw := NewMiddleware("dev-key", nil, 10, 5)
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	handler := mw.Wrap(next)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !called {
		t.Fatalf("expected public healthz to pass, code=%d called=%v", rec.Code, called)
	}

	called = false
	req = httptest.NewRequest(http.MethodGet, "/v1/leads", nil)
	req.Header.Set("Authorization", "Bearer dev-key")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !called {
		t.Fatalf("expected authenticated route to pass, code=%d called=%v", rec.Code, called)
	}
}

func TestIsExpensivePath(t *testing.T) {
	if !isExpensivePath("/v1/leads/abc/score") {
		t.Fatal("expected score path to be expensive")
	}
	if !isExpensivePath("/v1/leads/abc/embeddings") {
		t.Fatal("expected embeddings path to be expensive")
	}
	if isExpensivePath("/v1/leads") {
		t.Fatal("expected list path not to be expensive")
	}
}
