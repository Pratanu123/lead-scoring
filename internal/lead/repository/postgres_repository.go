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

func nullableString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: value != ""}
}

func nullableInt(value int) sql.NullInt64 {
	return sql.NullInt64{Int64: int64(value), Valid: value > 0}
}

func nullableFloat(value float64) sql.NullFloat64 {
	return sql.NullFloat64{Float64: value, Valid: value > 0}
}
