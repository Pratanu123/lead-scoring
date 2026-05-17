package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"lead-scoring/internal/lead/domain"
	"lead-scoring/internal/lead/service"
	opensearch "lead-scoring/internal/platform/opensearch"

	"github.com/redis/go-redis/v9"
)

type LeadHandler struct {
	service  *service.LeadService
	logger   *slog.Logger
	osClient *opensearch.Client
	cache    *redis.Client
}

func NewLeadHandler(service *service.LeadService, logger *slog.Logger, osClient *opensearch.Client, cache *redis.Client) *LeadHandler {
	return &LeadHandler{service: service, logger: logger, osClient: osClient, cache: cache}
}

func (h *LeadHandler) CreateLead(w http.ResponseWriter, r *http.Request) {
	h.logger.Info("CreateLead request", "method", r.Method, "path", r.URL.Path)

	if r.Method != http.MethodPost {
		h.logger.Warn("invalid method for CreateLead", "method", r.Method)
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	defer r.Body.Close()
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if cached, ok := h.getIdempotentResponse(r.Context(), idempotencyKey); ok {
		w.Header().Set("X-Idempotent-Replay", "true")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write(cached)
		return
	}

	var input domain.CreateLeadInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		h.logger.Warn("failed to decode request body", "error", err)
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	lead, err := h.service.CreateLead(r.Context(), input)
	if err != nil {
		if errors.Is(err, service.ErrInvalidLead) {
			h.logger.Warn("invalid lead data", "company_name", input.CompanyName, "email", input.Email)
			writeError(w, http.StatusBadRequest, "company_name, valid email, and source are required")
			return
		}

		h.logger.Error("failed to create lead", "error", err, "company_name", input.CompanyName)
		writeError(w, http.StatusInternalServerError, "failed to create lead")
		return
	}

	h.logger.Info("lead created successfully", "lead_id", lead.ID, "company_name", lead.CompanyName)
	if h.osClient != nil {
		if err := h.osClient.IndexLog(r.Context(), map[string]any{
			"time":         time.Now().UTC().Format(time.RFC3339Nano),
			"level":        "info",
			"msg":          "lead created",
			"lead_id":      lead.ID,
			"company_name": lead.CompanyName,
			"path":         r.URL.Path,
			"method":       r.Method,
		}); err != nil {
			h.logger.Warn("opensearch index failed", "error", err)
		}
	}
	h.setIdempotentResponse(r.Context(), idempotencyKey, lead)
	writeJSON(w, http.StatusCreated, lead)
}

func (h *LeadHandler) ListLeads(w http.ResponseWriter, r *http.Request) {
	h.logger.Info("ListLeads request", "method", r.Method, "path", r.URL.Path)

	if r.Method != http.MethodGet {
		h.logger.Warn("invalid method for ListLeads", "method", r.Method)
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	limit, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("limit")))
	if err != nil && r.URL.Query().Get("limit") != "" {
		h.logger.Warn("invalid limit parameter", "limit", r.URL.Query().Get("limit"))
		writeError(w, http.StatusBadRequest, "limit must be a valid integer")
		return
	}

	offset, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("offset")))
	if err != nil && r.URL.Query().Get("offset") != "" {
		h.logger.Warn("invalid offset parameter", "offset", r.URL.Query().Get("offset"))
		writeError(w, http.StatusBadRequest, "offset must be a valid integer")
		return
	}

	leads, err := h.service.ListLeads(r.Context(), domain.ListLeadsInput{
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		h.logger.Error("failed to list leads", "error", err, "limit", limit, "offset", offset)
		writeError(w, http.StatusInternalServerError, "failed to list leads")
		return
	}

	h.logger.Info("leads listed successfully", "count", len(leads), "limit", normalizedLimit(limit), "offset", normalizedOffset(offset))
	if h.osClient != nil {
		if err := h.osClient.IndexLog(r.Context(), map[string]any{
			"time":   time.Now().UTC().Format(time.RFC3339Nano),
			"level":  "info",
			"msg":    "leads listed",
			"count":  len(leads),
			"limit":  normalizedLimit(limit),
			"offset": normalizedOffset(offset),
			"path":   r.URL.Path,
			"method": r.Method,
		}); err != nil {
			h.logger.Warn("opensearch index failed", "error", err)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":  leads,
		"limit":  normalizedLimit(limit),
		"offset": normalizedOffset(offset),
	})
}

func (h *LeadHandler) GetLead(w http.ResponseWriter, r *http.Request) {
	h.logger.Info("GetLead request", "method", r.Method, "path", r.URL.Path)

	if r.Method != http.MethodGet {
		h.logger.Warn("invalid method for GetLead", "method", r.Method)
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		id = strings.TrimPrefix(r.URL.Path, "/v1/leads/")
	}

	h.logger.Info("fetching lead", "lead_id", id)

	lead, err := h.service.GetLead(r.Context(), id)
	if err != nil {
		if errors.Is(err, service.ErrLeadNotFound) {
			h.logger.Warn("lead not found", "lead_id", id)
			writeError(w, http.StatusNotFound, "lead not found")
			return
		}

		h.logger.Error("failed to fetch lead", "error", err, "lead_id", id)
		writeError(w, http.StatusInternalServerError, "failed to fetch lead")
		return
	}

	h.logger.Info("lead fetched successfully", "lead_id", lead.ID, "company_name", lead.CompanyName)
	if h.osClient != nil {
		if err := h.osClient.IndexLog(r.Context(), map[string]any{
			"time":         time.Now().UTC().Format(time.RFC3339Nano),
			"level":        "info",
			"msg":          "lead fetched",
			"lead_id":      lead.ID,
			"company_name": lead.CompanyName,
			"path":         r.URL.Path,
			"method":       r.Method,
		}); err != nil {
			h.logger.Warn("opensearch index failed", "error", err)
		}
	}
	writeJSON(w, http.StatusOK, lead)
}

func (h *LeadHandler) UpsertLeadEmbedding(w http.ResponseWriter, r *http.Request) {
	h.logger.Info("UpsertLeadEmbedding request", "method", r.Method, "path", r.URL.Path)

	if r.Method != http.MethodPost {
		h.logger.Warn("invalid method for UpsertLeadEmbedding", "method", r.Method)
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	result, err := h.service.UpsertLeadEmbedding(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, service.ErrLeadNotFound) {
			writeError(w, http.StatusNotFound, "lead not found")
			return
		}

		h.logger.Error("failed to upsert embedding", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to upsert lead embedding")
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (h *LeadHandler) SimilarLeads(w http.ResponseWriter, r *http.Request) {
	h.logger.Info("SimilarLeads request", "method", r.Method, "path", r.URL.Path)

	if r.Method != http.MethodGet {
		h.logger.Warn("invalid method for SimilarLeads", "method", r.Method)
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	limit, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("limit")))
	if err != nil && r.URL.Query().Get("limit") != "" {
		writeError(w, http.StatusBadRequest, "limit must be a valid integer")
		return
	}

	similarLeads, err := h.service.SimilarLeads(r.Context(), r.PathValue("id"), limit)
	if err != nil {
		if errors.Is(err, service.ErrLeadNotFound) {
			writeError(w, http.StatusNotFound, "lead not found")
			return
		}

		h.logger.Error("failed to find similar leads", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to find similar leads")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items": similarLeads,
		"limit": normalizedSimilarLimit(limit),
	})
}

func (h *LeadHandler) ScoreLead(w http.ResponseWriter, r *http.Request) {
	h.logger.Info("ScoreLead request", "method", r.Method, "path", r.URL.Path)

	if r.Method != http.MethodPost {
		h.logger.Warn("invalid method for ScoreLead", "method", r.Method)
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	result, err := h.service.ScoreLead(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, service.ErrLeadNotFound) {
			writeError(w, http.StatusNotFound, "lead not found")
			return
		}

		h.logger.Error("failed to score lead", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to score lead")
		return
	}

	writeJSON(w, http.StatusCreated, result)
}

func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, statusCode int, message string) {
	writeJSON(w, statusCode, map[string]string{"error": message})
}

func normalizedLimit(limit int) int {
	if limit <= 0 {
		return 20
	}
	if limit > 100 {
		return 100
	}
	return limit
}

func normalizedOffset(offset int) int {
	if offset < 0 {
		return 0
	}
	return offset
}

func normalizedSimilarLimit(limit int) int {
	if limit <= 0 {
		return 5
	}
	if limit > 20 {
		return 20
	}
	return limit
}

func (h *LeadHandler) getIdempotentResponse(ctx context.Context, key string) ([]byte, bool) {
	if h.cache == nil || key == "" {
		return nil, false
	}

	value, err := h.cache.Get(ctx, idempotencyCacheKey(key)).Bytes()
	if err != nil {
		return nil, false
	}

	return value, true
}

func (h *LeadHandler) setIdempotentResponse(ctx context.Context, key string, lead domain.Lead) {
	if h.cache == nil || key == "" {
		return
	}

	payload, err := json.Marshal(lead)
	if err != nil {
		return
	}

	_ = h.cache.Set(ctx, idempotencyCacheKey(key), payload, 24*time.Hour).Err()
}

func idempotencyCacheKey(key string) string {
	hash := sha256.Sum256([]byte(key))
	return "lead-scoring:idempotency:create-lead:" + hex.EncodeToString(hash[:])
}
