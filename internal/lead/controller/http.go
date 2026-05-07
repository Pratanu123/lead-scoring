package controller

import (
	"encoding/json"
	"errors"
	"net/http"

	"lead-scoring/internal/lead/domain"
	"lead-scoring/internal/lead/service"
)

type LeadHandler struct {
	service *service.LeadService
}

func NewLeadHandler(service *service.LeadService) *LeadHandler {
	return &LeadHandler{service: service}
}

func (h *LeadHandler) CreateLead(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	defer r.Body.Close()

	var input domain.CreateLeadInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	lead, err := h.service.CreateLead(r.Context(), input)
	if err != nil {
		if errors.Is(err, service.ErrInvalidLead) {
			writeError(w, http.StatusBadRequest, "company_name, valid email, and source are required")
			return
		}

		writeError(w, http.StatusInternalServerError, "failed to create lead")
		return
	}

	writeJSON(w, http.StatusCreated, lead)
}

func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, statusCode int, message string) {
	writeJSON(w, statusCode, map[string]string{"error": message})
}
