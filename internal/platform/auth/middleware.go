package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type Middleware struct {
	apiKey            string
	redis             *redis.Client
	rateLimitPerMin   int
	scoreRateLimitRPM int
}

func NewMiddleware(apiKey string, redisClient *redis.Client, rateLimitPerMin int, scoreRateLimitRPM int) *Middleware {
	if rateLimitPerMin <= 0 {
		rateLimitPerMin = 120
	}
	if scoreRateLimitRPM <= 0 {
		scoreRateLimitRPM = 30
	}
	return &Middleware{
		apiKey:            strings.TrimSpace(apiKey),
		redis:             redisClient,
		rateLimitPerMin:   rateLimitPerMin,
		scoreRateLimitRPM: scoreRateLimitRPM,
	}
}

func (m *Middleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if !requiresAuth(path) {
			next.ServeHTTP(w, r)
			return
		}

		token := ExtractBearerToken(r)
		if token == "" || token != m.apiKey {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}

		limit := m.rateLimitPerMin
		if isExpensivePath(path) {
			limit = m.scoreRateLimitRPM
		}
		allowed, err := m.allow(r.Context(), token, path, limit)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "rate limiter unavailable"})
			return
		}
		if !allowed {
			w.Header().Set("Retry-After", "60")
			writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "rate limit exceeded"})
			return
		}

		next.ServeHTTP(w, r)
	})
}

func requiresAuth(path string) bool {
	if path == "/healthz" || path == "/metrics" {
		return false
	}
	return strings.HasPrefix(path, "/v1/") || path == "/create-lead" || strings.HasPrefix(path, "/v1/create") || strings.HasPrefix(path, "/v1/get-leads")
}

func ExtractBearerToken(r *http.Request) string {
	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
		return strings.TrimSpace(authHeader[7:])
	}
	if token := strings.TrimSpace(r.URL.Query().Get("token")); token != "" {
		return token
	}
	return ""
}

func (m *Middleware) allow(ctx context.Context, token string, path string, limit int) (bool, error) {
	if m.redis == nil {
		return true, nil
	}

	bucket := "general"
	if isExpensivePath(path) {
		bucket = "expensive"
	}
	minute := time.Now().UTC().Format("200601021504")
	key := fmt.Sprintf("lead-scoring:ratelimit:%s:%s:%s", bucket, token, minute)

	count, err := m.redis.Incr(ctx, key).Result()
	if err != nil {
		return false, err
	}
	if count == 1 {
		_ = m.redis.Expire(ctx, key, 2*time.Minute).Err()
	}
	return count <= int64(limit), nil
}

func isExpensivePath(path string) bool {
	return strings.HasSuffix(path, "/score") || strings.HasSuffix(path, "/embeddings")
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
