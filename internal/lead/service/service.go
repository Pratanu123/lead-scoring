package service

import (
	"context"
	"errors"
	"strings"

	"lead-scoring/internal/lead/domain"
	"lead-scoring/internal/lead/repository"
)

var ErrInvalidLead = errors.New("invalid lead")

type LeadService struct {
	repo repository.Repository
}

func NewLeadService(repo repository.Repository) *LeadService {
	return &LeadService{repo: repo}
}

func (s *LeadService) CreateLead(ctx context.Context, input domain.CreateLeadInput) (domain.Lead, error) {
	normalized := domain.CreateLeadInput{
		CompanyName:   strings.TrimSpace(input.CompanyName),
		ContactName:   strings.TrimSpace(input.ContactName),
		Email:         strings.ToLower(strings.TrimSpace(input.Email)),
		Phone:         strings.TrimSpace(input.Phone),
		Source:        strings.TrimSpace(input.Source),
		Industry:      strings.TrimSpace(input.Industry),
		CompanySize:   input.CompanySize,
		AnnualRevenue: input.AnnualRevenue,
		Notes:         strings.TrimSpace(input.Notes),
	}

	if normalized.CompanyName == "" || normalized.Email == "" || normalized.Source == "" {
		return domain.Lead{}, ErrInvalidLead
	}

	if !strings.Contains(normalized.Email, "@") {
		return domain.Lead{}, ErrInvalidLead
	}

	return s.repo.Create(ctx, normalized)
}
