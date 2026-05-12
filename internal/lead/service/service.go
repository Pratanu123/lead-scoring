package service

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strings"

	"lead-scoring/internal/lead/domain"
	"lead-scoring/internal/lead/repository"
)

var ErrInvalidLead = errors.New("invalid lead")
var ErrLeadNotFound = errors.New("lead not found")

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

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

func (s *LeadService) ListLeads(ctx context.Context, input domain.ListLeadsInput) ([]domain.Lead, error) {
	if input.Limit <= 0 {
		input.Limit = 20
	}
	if input.Limit > 100 {
		input.Limit = 100
	}
	if input.Offset < 0 {
		input.Offset = 0
	}

	return s.repo.List(ctx, input)
}

func (s *LeadService) GetLead(ctx context.Context, id string) (domain.Lead, error) {
	id = strings.TrimSpace(id)
	if id == "" || !uuidPattern.MatchString(id) {
		return domain.Lead{}, ErrLeadNotFound
	}

	lead, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Lead{}, ErrLeadNotFound
		}

		return domain.Lead{}, err
	}

	return lead, nil
}
