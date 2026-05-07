# Architecture

## High-Level Design

```text
Client / CRM
    |
    v
Go API
    |
    +--> Lead Controller
    |       |
    |       v
    |   Lead Service
    |       |
    |       v
    |   Lead Repository
    |       |
    |       v
    |   Postgres
    |
    +--> Redis
            |
            +--> future cache, rate limit, idempotency

Local developer UIs:

Browser -> Adminer          -> Postgres
Browser -> Redis Commander  -> Redis

Future RAG flow:

Lead Created -> Embedding Job -> pgvector -> Similar Lead Retrieval -> LLM Scoring -> lead_scores
```

## Low-Level Design

### Services

- API service: receives lead ingestion requests and exposes scoring endpoints.
- Postgres UI: Adminer for local schema/data inspection.
- Redis UI: Redis Commander for local key inspection.
- Lead service: validates and normalizes lead data.
- Lead repository: owns SQL persistence.
- Embedding service: planned Day 10+ component that converts lead text into vectors.
- Scoring service: planned component that retrieves similar leads and asks an LLM for probability + reasoning.

### Database Schema

- `leads`: source-of-truth CRM lead profile.
- `lead_embeddings`: vector representation of lead text using `vector(1536)`.
- `lead_scores`: model output with conversion probability and reasoning.

### APIs

```text
GET  /healthz
POST /create-lead
POST /v1/create-lead
POST /v1/leads
```

Planned APIs:

```text
POST /v1/leads/{id}/score
GET  /v1/leads/{id}/similar
GET  /v1/leads/{id}/scores
```

## RAG Storage Example

Store an embedding:

```sql
INSERT INTO lead_embeddings (
    lead_id,
    embedding_model,
    content_hash,
    embedding
) VALUES (
    $1,
    'text-embedding-3-small',
    $2,
    $3::vector
);
```

Query similar leads:

```sql
SELECT
    l.id,
    l.company_name,
    l.industry,
    1 - (e.embedding <=> $1::vector) AS similarity
FROM lead_embeddings e
JOIN leads l ON l.id = e.lead_id
ORDER BY e.embedding <=> $1::vector
LIMIT 5;
```

## Scaling Notes

- Postgres remains the transactional source of truth.
- pgvector avoids an extra vector database while the project is small to mid-scale.
- Redis is reserved for idempotency keys, short-lived scoring cache, and rate limiting.
- Embeddings and scoring should move to async workers once lead creation latency matters.
