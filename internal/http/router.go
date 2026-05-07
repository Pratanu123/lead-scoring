package httpapi

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	leadcontroller "lead-scoring/internal/lead/controller"

	"github.com/redis/go-redis/v9"
)

type RouterDeps struct {
	LeadHandler *leadcontroller.LeadHandler
	DB          *sql.DB
	Redis       *redis.Client
}

func NewRouter(deps RouterDeps) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
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
	})

	mux.HandleFunc("/v1/create-lead", deps.LeadHandler.CreateLead)

	return mux
}

func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, statusCode int, message string) {
	writeJSON(w, statusCode, map[string]string{"error": message})
}
