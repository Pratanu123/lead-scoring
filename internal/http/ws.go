package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"lead-scoring/internal/platform/auth"
	"lead-scoring/internal/platform/jobs"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type WSHub struct {
	jobs   *jobs.Store
	apiKey string
	logger *slog.Logger
}

func NewWSHub(jobStore *jobs.Store, apiKey string, logger *slog.Logger) *WSHub {
	return &WSHub{jobs: jobStore, apiKey: strings.TrimSpace(apiKey), logger: logger}
}

func (h *WSHub) Handle(w http.ResponseWriter, r *http.Request) {
	if h.jobs == nil {
		http.Error(w, "websocket unavailable", http.StatusServiceUnavailable)
		return
	}

	token := auth.ExtractBearerToken(r)
	if token == "" || token != h.apiKey {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	leadID := strings.TrimSpace(r.URL.Query().Get("lead_id"))
	if leadID == "" {
		http.Error(w, "lead_id is required", http.StatusBadRequest)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.logger.Warn("websocket upgrade failed", "error", err)
		return
	}
	defer conn.Close()

	_ = conn.WriteJSON(map[string]any{
		"type":    "connected",
		"lead_id": leadID,
		"time":    time.Now().UTC(),
	})

	pubsub := h.jobs.Subscribe(r.Context())
	if pubsub == nil {
		_ = conn.WriteJSON(map[string]string{"type": "error", "error": "pubsub unavailable"})
		return
	}
	defer pubsub.Close()

	ch := pubsub.Channel()
	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ping.C:
			if err := conn.WriteControl(websocket.PingMessage, []byte("ping"), time.Now().Add(5*time.Second)); err != nil {
				return
			}
		case msg, ok := <-ch:
			if !ok {
				return
			}
			var event jobs.Event
			if err := json.Unmarshal([]byte(msg.Payload), &event); err != nil {
				continue
			}
			if event.LeadID != leadID {
				continue
			}
			if err := conn.WriteJSON(event); err != nil {
				return
			}
		}
	}
}
