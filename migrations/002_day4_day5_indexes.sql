CREATE INDEX IF NOT EXISTS idx_lead_scores_lead_id_created_at
ON lead_scores (lead_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_lead_embeddings_model
ON lead_embeddings (embedding_model);
