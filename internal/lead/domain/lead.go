package domain

import "time"

type Lead struct {
	ID            string    `json:"id"`
	CompanyName   string    `json:"company_name"`
	ContactName   string    `json:"contact_name,omitempty"`
	Email         string    `json:"email"`
	Phone         string    `json:"phone,omitempty"`
	Source        string    `json:"source"`
	Industry      string    `json:"industry,omitempty"`
	CompanySize   int       `json:"company_size,omitempty"`
	AnnualRevenue float64   `json:"annual_revenue,omitempty"`
	Notes         string    `json:"notes,omitempty"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type CreateLeadInput struct {
	CompanyName   string  `json:"company_name"`
	ContactName   string  `json:"contact_name"`
	Email         string  `json:"email"`
	Phone         string  `json:"phone"`
	Source        string  `json:"source"`
	Industry      string  `json:"industry"`
	CompanySize   int     `json:"company_size"`
	AnnualRevenue float64 `json:"annual_revenue"`
	Notes         string  `json:"notes"`
}

type ListLeadsInput struct {
	Limit  int
	Offset int
}

type EmbeddingResult struct {
	LeadID      string    `json:"lead_id"`
	Model       string    `json:"model"`
	ContentHash string    `json:"content_hash"`
	CreatedAt   time.Time `json:"created_at"`
}

type SimilarLead struct {
	ID          string  `json:"id"`
	CompanyName string  `json:"company_name"`
	Email       string  `json:"email"`
	Source      string  `json:"source"`
	Industry    string  `json:"industry,omitempty"`
	Similarity  float64 `json:"similarity"`
}

type LeadScore struct {
	ID                    string    `json:"id"`
	LeadID                string    `json:"lead_id"`
	ConversionProbability float64   `json:"conversion_probability"`
	Reasoning             string    `json:"reasoning"`
	Model                 string    `json:"model"`
	CreatedAt             time.Time `json:"created_at"`
}

type ScoreLeadResult struct {
	Score        LeadScore     `json:"score"`
	SimilarLeads []SimilarLead `json:"similar_leads"`
}
