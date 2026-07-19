package httpapi

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	leadcontroller "lead-scoring/internal/lead/controller"
	appmetrics "lead-scoring/internal/platform/appmetrics"
	"lead-scoring/internal/platform/auth"

	"github.com/redis/go-redis/v9"
)

// Metrics holds request metrics
type Metrics struct {
	mu               sync.RWMutex
	totalRequests    int64
	totalErrors      int64
	totalDuration    time.Duration
	requestsByMethod map[string]int64
	requestsByStatus map[int]int64
}

func NewMetrics() *Metrics {
	return &Metrics{
		requestsByMethod: make(map[string]int64),
		requestsByStatus: make(map[int]int64),
	}
}

func (m *Metrics) RecordRequest(method string, status int, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.totalRequests++
	m.totalDuration += duration
	m.requestsByMethod[method]++
	m.requestsByStatus[status]++

	if status >= 400 {
		m.totalErrors++
	}
}

func (m *Metrics) Format() string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	output := "# HELP http_requests_total Total HTTP requests\n"
	output += "# TYPE http_requests_total counter\n"
	output += fmt.Sprintf("http_requests_total %d\n", m.totalRequests)

	output += "# HELP http_request_errors_total Total HTTP errors\n"
	output += "# TYPE http_request_errors_total counter\n"
	output += fmt.Sprintf("http_request_errors_total %d\n", m.totalErrors)

	output += "# HELP http_request_duration_seconds_total Total request duration in seconds\n"
	output += "# TYPE http_request_duration_seconds_total counter\n"
	output += fmt.Sprintf("http_request_duration_seconds_total %.2f\n", m.totalDuration.Seconds())

	output += "# HELP http_requests_by_method Total requests by method\n"
	output += "# TYPE http_requests_by_method gauge\n"
	for method, count := range m.requestsByMethod {
		output += fmt.Sprintf("http_requests_by_method{method=%q} %d\n", method, count)
	}

	output += "# HELP http_requests_by_status Total requests by status code\n"
	output += "# TYPE http_requests_by_status gauge\n"
	for status, count := range m.requestsByStatus {
		output += fmt.Sprintf("http_requests_by_status{status=\"%d\"} %d\n", status, count)
	}

	return output
}

type RouterDeps struct {
	LeadHandler *leadcontroller.LeadHandler
	WSHub       *WSHub
	DB          *sql.DB
	Redis       *redis.Client
	Logger      *slog.Logger
	APIKey      string
	RateLimit   int
	ScoreLimit  int
	AppMetrics  *appmetrics.Registry
	StaticDir   string
}

func NewRouter(deps RouterDeps) http.Handler {
	mux := http.NewServeMux()
	metrics := NewMetrics()
	if deps.AppMetrics == nil {
		deps.AppMetrics = appmetrics.NewRegistry()
	}

	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(metrics.Format()))
		_, _ = w.Write([]byte(deps.AppMetrics.Format()))
	})

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			metrics.RecordRequest(r.Method, http.StatusMethodNotAllowed, time.Since(start))
			return
		}

		ctx := r.Context()
		dbStatus := "ok"
		redisStatus := "ok"

		if err := deps.DB.PingContext(ctx); err != nil {
			dbStatus = "down"
		}

		if err := deps.Redis.Ping(ctx).Err(); err != nil {
			redisStatus = "down"
		}

		statusCode := http.StatusOK
		if dbStatus != "ok" || redisStatus != "ok" {
			statusCode = http.StatusServiceUnavailable
		}

		writeJSON(w, statusCode, map[string]any{
			"status":   "ok",
			"postgres": dbStatus,
			"redis":    redisStatus,
			"time":     time.Now().UTC(),
		})
		metrics.RecordRequest(r.Method, statusCode, time.Since(start))
	})

	metricsMiddleware := func(handler http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
			handler(rw, r)
			metrics.RecordRequest(r.Method, rw.statusCode, time.Since(start))
		}
	}

	mux.HandleFunc("/create-lead", metricsMiddleware(deps.LeadHandler.CreateLead))
	mux.HandleFunc("/v1/create-lead", metricsMiddleware(deps.LeadHandler.CreateLead))
	mux.HandleFunc("/v1/create-leads", metricsMiddleware(deps.LeadHandler.CreateLead))
	mux.HandleFunc("/v1/get-leads", metricsMiddleware(deps.LeadHandler.ListLeads))
	mux.HandleFunc("/v1/get-leads/{id}", metricsMiddleware(deps.LeadHandler.GetLead))

	mux.HandleFunc("POST /v1/leads", metricsMiddleware(deps.LeadHandler.CreateLead))
	mux.HandleFunc("GET /v1/leads", metricsMiddleware(deps.LeadHandler.ListLeads))
	mux.HandleFunc("GET /v1/leads/{id}", metricsMiddleware(deps.LeadHandler.GetLead))
	mux.HandleFunc("PATCH /v1/leads/{id}", metricsMiddleware(deps.LeadHandler.UpdateLeadStatus))

	mux.HandleFunc("POST /v1/leads/{id}/embeddings", metricsMiddleware(deps.LeadHandler.UpsertLeadEmbedding))
	mux.HandleFunc("GET /v1/leads/{id}/similar", metricsMiddleware(deps.LeadHandler.SimilarLeads))
	mux.HandleFunc("POST /v1/leads/{id}/score", metricsMiddleware(deps.LeadHandler.ScoreLead))
	mux.HandleFunc("GET /v1/leads/{id}/score", metricsMiddleware(deps.LeadHandler.GetLatestLeadScore))
	mux.HandleFunc("GET /v1/leads/{id}/scores", metricsMiddleware(deps.LeadHandler.ListLeadScores))
	mux.HandleFunc("GET /v1/jobs/{id}", metricsMiddleware(deps.LeadHandler.GetJob))

	if deps.WSHub != nil {
		mux.HandleFunc("GET /v1/ws", deps.WSHub.Handle)
	}

	if deps.StaticDir != "" {
		if info, err := os.Stat(deps.StaticDir); err == nil && info.IsDir() {
			fileServer := http.FileServer(http.Dir(deps.StaticDir))
			mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
				if strings.HasPrefix(r.URL.Path, "/v1/") ||
					r.URL.Path == "/healthz" ||
					r.URL.Path == "/metrics" ||
					r.URL.Path == "/create-lead" {
					http.NotFound(w, r)
					return
				}

				requested := filepath.Join(deps.StaticDir, filepath.Clean("/"+r.URL.Path))
				if !strings.HasPrefix(requested, filepath.Clean(deps.StaticDir)+string(os.PathSeparator)) &&
					filepath.Clean(requested) != filepath.Clean(deps.StaticDir) {
					http.NotFound(w, r)
					return
				}

				if st, err := os.Stat(requested); err == nil && !st.IsDir() {
					fileServer.ServeHTTP(w, r)
					return
				}

				index := filepath.Join(deps.StaticDir, "index.html")
				http.ServeFile(w, r, index)
			})
		}
	}

	authMiddleware := auth.NewMiddleware(deps.APIKey, deps.Redis, deps.RateLimit, deps.ScoreLimit)
	return authMiddleware.Wrap(mux)
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, statusCode int, message string) {
	writeJSON(w, statusCode, map[string]string{"error": message})
}
