package repository

import (
	"context"
	"database/sql"

	"lead-scoring/internal/lead/domain"
)

type Repository interface {
	Create(ctx context.Context, input domain.CreateLeadInput) (domain.Lead, error)
	List(ctx context.Context, input domain.ListLeadsInput) ([]domain.Lead, error)
	GetByID(ctx context.Context, id string) (domain.Lead, error)
	UpsertEmbedding(ctx context.Context, leadID string, model string, contentHash string, vector string) (domain.EmbeddingResult, error)
	FindSimilar(ctx context.Context, leadID string, vector string, limit int) ([]domain.SimilarLead, error)
	CreateScore(ctx context.Context, leadID string, probability float64, reasoning string, model string) (domain.LeadScore, error)
}

type PostgresRepository struct {
	db *sql.DB
}

func NewPostgresRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Create(ctx context.Context, input domain.CreateLeadInput) (domain.Lead, error) {
	const query = `
INSERT INTO leads (
    company_name,
    contact_name,
    email,
    phone,
    source,
    industry,
    company_size,
    annual_revenue,
    notes
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING
    id,
    company_name,
    COALESCE(contact_name, ''),
    email,
    COALESCE(phone, ''),
    source,
    COALESCE(industry, ''),
    COALESCE(company_size, 0),
    COALESCE(annual_revenue, 0)::float8,
    COALESCE(notes, ''),
    status,
    created_at,
    updated_at;
`

	var lead domain.Lead
	err := r.db.QueryRowContext(
		ctx,
		query,
		input.CompanyName,
		nullableString(input.ContactName),
		input.Email,
		nullableString(input.Phone),
		input.Source,
		nullableString(input.Industry),
		nullableInt(input.CompanySize),
		nullableFloat(input.AnnualRevenue),
		nullableString(input.Notes),
	).Scan(
		&lead.ID,
		&lead.CompanyName,
		&lead.ContactName,
		&lead.Email,
		&lead.Phone,
		&lead.Source,
		&lead.Industry,
		&lead.CompanySize,
		&lead.AnnualRevenue,
		&lead.Notes,
		&lead.Status,
		&lead.CreatedAt,
		&lead.UpdatedAt,
	)
	if err != nil {
		return domain.Lead{}, err
	}

	return lead, nil
}

func (r *PostgresRepository) List(ctx context.Context, input domain.ListLeadsInput) ([]domain.Lead, error) {
	const query = `
SELECT
    id,
    company_name,
    COALESCE(contact_name, ''),
    email,
    COALESCE(phone, ''),
    source,
    COALESCE(industry, ''),
    COALESCE(company_size, 0),
    COALESCE(annual_revenue, 0)::float8,
    COALESCE(notes, ''),
    status,
    created_at,
    updated_at
FROM leads
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;
`

	rows, err := r.db.QueryContext(ctx, query, input.Limit, input.Offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	leads := make([]domain.Lead, 0, input.Limit)
	for rows.Next() {
		var lead domain.Lead
		if err := rows.Scan(
			&lead.ID,
			&lead.CompanyName,
			&lead.ContactName,
			&lead.Email,
			&lead.Phone,
			&lead.Source,
			&lead.Industry,
			&lead.CompanySize,
			&lead.AnnualRevenue,
			&lead.Notes,
			&lead.Status,
			&lead.CreatedAt,
			&lead.UpdatedAt,
		); err != nil {
			return nil, err
		}

		leads = append(leads, lead)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return leads, nil
}

func (r *PostgresRepository) GetByID(ctx context.Context, id string) (domain.Lead, error) {
	const query = `
SELECT
    id,
    company_name,
    COALESCE(contact_name, ''),
    email,
    COALESCE(phone, ''),
    source,
    COALESCE(industry, ''),
    COALESCE(company_size, 0),
    COALESCE(annual_revenue, 0)::float8,
    COALESCE(notes, ''),
    status,
    created_at,
    updated_at
FROM leads
WHERE id = $1;
`

	var lead domain.Lead
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&lead.ID,
		&lead.CompanyName,
		&lead.ContactName,
		&lead.Email,
		&lead.Phone,
		&lead.Source,
		&lead.Industry,
		&lead.CompanySize,
		&lead.AnnualRevenue,
		&lead.Notes,
		&lead.Status,
		&lead.CreatedAt,
		&lead.UpdatedAt,
	)
	if err != nil {
		return domain.Lead{}, err
	}

	return lead, nil
}

func (r *PostgresRepository) UpsertEmbedding(ctx context.Context, leadID string, model string, contentHash string, vector string) (domain.EmbeddingResult, error) {
	const query = `
INSERT INTO lead_embeddings (
    lead_id,
    embedding_model,
    content_hash,
    embedding
) VALUES ($1, $2, $3, $4::vector)
ON CONFLICT (lead_id, embedding_model)
DO UPDATE SET
    content_hash = EXCLUDED.content_hash,
    embedding = EXCLUDED.embedding,
    created_at = now()
RETURNING lead_id, embedding_model, content_hash, created_at;
`

	var result domain.EmbeddingResult
	err := r.db.QueryRowContext(ctx, query, leadID, model, contentHash, vector).Scan(
		&result.LeadID,
		&result.Model,
		&result.ContentHash,
		&result.CreatedAt,
	)
	if err != nil {
		return domain.EmbeddingResult{}, err
	}

	return result, nil
}

func (r *PostgresRepository) FindSimilar(ctx context.Context, leadID string, vector string, limit int) ([]domain.SimilarLead, error) {
	const query = `
SELECT
    l.id,
    l.company_name,
    l.email,
    l.source,
    COALESCE(l.industry, ''),
    1 - (e.embedding <=> $2::vector) AS similarity
FROM lead_embeddings e
JOIN leads l ON l.id = e.lead_id
WHERE e.lead_id <> $1
ORDER BY e.embedding <=> $2::vector
LIMIT $3;
`

	rows, err := r.db.QueryContext(ctx, query, leadID, vector, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	similarLeads := make([]domain.SimilarLead, 0, limit)
	for rows.Next() {
		var lead domain.SimilarLead
		if err := rows.Scan(
			&lead.ID,
			&lead.CompanyName,
			&lead.Email,
			&lead.Source,
			&lead.Industry,
			&lead.Similarity,
		); err != nil {
			return nil, err
		}

		similarLeads = append(similarLeads, lead)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return similarLeads, nil
}

func (r *PostgresRepository) CreateScore(ctx context.Context, leadID string, probability float64, reasoning string, model string) (domain.LeadScore, error) {
	const query = `
INSERT INTO lead_scores (
    lead_id,
    conversion_probability,
    reasoning,
    model
) VALUES ($1, $2, $3, $4)
RETURNING
    id,
    lead_id,
    conversion_probability::float8,
    reasoning,
    model,
    created_at;
`

	var score domain.LeadScore
	err := r.db.QueryRowContext(ctx, query, leadID, probability, reasoning, model).Scan(
		&score.ID,
		&score.LeadID,
		&score.ConversionProbability,
		&score.Reasoning,
		&score.Model,
		&score.CreatedAt,
	)
	if err != nil {
		return domain.LeadScore{}, err
	}

	return score, nil
}

func nullableString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: value != ""}
}

func nullableInt(value int) sql.NullInt64 {
	return sql.NullInt64{Int64: int64(value), Valid: value > 0}
}

func nullableFloat(value float64) sql.NullFloat64 {
	return sql.NullFloat64{Float64: value, Valid: value > 0}
}
