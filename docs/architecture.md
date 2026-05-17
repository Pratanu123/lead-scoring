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
            +--> cache, idempotency

Observability:

Go API JSON logs -> Vector -> OpenSearch -> OpenSearch Dashboards
Go API /metrics -> Prometheus -> Grafana

Scale-out path:

Client / CRM
    -> Load Balancer
        -> API Instance 1
        -> API Instance 2
        -> API Instance N
            -> Shared Postgres
            -> Shared Redis

Local developer UIs:

Browser -> Adminer          -> Postgres
Browser -> Redis Commander  -> Redis

RAG flow:

Lead Created -> Local Embedding -> pgvector -> Similar Lead Retrieval -> RAG Scoring -> lead_scores
```

## Low-Level Design

### Services

- API service: receives lead ingestion requests and exposes scoring endpoints.
- Postgres UI: Adminer for local schema/data inspection.
- Redis UI: Redis Commander for local key inspection.
- Lead service: validates and normalizes lead data.
- Lead repository: owns SQL persistence.
- Read API path: lists and fetches leads with bounded pagination.
- Embedding path: converts lead text into deterministic local embeddings and stores them in pgvector.
- Scoring path: retrieves similar leads and writes conversion probability + reasoning into `lead_scores`.

### Database Schema

- `leads`: source-of-truth CRM lead profile.
- `lead_embeddings`: vector representation of lead text using `vector(1536)`.
- `lead_scores`: model output with conversion probability and reasoning.

### APIs

```text
GET  /healthz
POST /create-lead
POST /v1/create-lead
POST /v1/create-leads
POST /v1/leads
GET  /v1/leads
GET  /v1/leads/{id}
GET  /v1/get-leads
GET  /v1/get-leads/{id}
POST /v1/leads/{id}/embeddings
GET  /v1/leads/{id}/similar
POST /v1/leads/{id}/score
```

Planned APIs:

```text
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
- Day 2 keeps the API stateless, so horizontal scaling is just more API instances behind a load balancer.
- `GET /v1/leads` enforces bounded `limit` and `offset` values to avoid unbounded scans.
- The same service/repository layers now back both write and read paths, which keeps controller logic thin as the surface area grows.
- Day 3 caches lead reads in Redis with short TTLs and invalidates list caches after writes.
- Day 4 uses `Idempotency-Key` for safe create retries and SHA-256 content hashes for embedding updates.
- Day 5 keeps RAG in Postgres with pgvector before introducing heavier vector infrastructure.
