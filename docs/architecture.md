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

Lead Created -> Embedder (local hash or remote) -> pgvector -> Similar Lead Retrieval (same model) -> Remote LLM Scorer with local fallback -> lead_scores
```

## Low-Level Design

### Services

- API service: receives lead ingestion requests and exposes scoring endpoints.
- Postgres UI: Adminer for local schema/data inspection.
- Redis UI: Redis Commander for local key inspection.
- Lead service: validates and normalizes lead data.
- Lead repository: owns SQL persistence.
- Read API path: lists and fetches leads with bounded pagination.
- Embedding path: uses an `Embedder` interface. Default is deterministic local hash vectors (`local-hash-embedding-v1`); optional OpenAI-compatible remote embeddings via `EMBEDDING_API_*`. Upserts skip when `content_hash` is unchanged.
- Scoring path: retrieves similar leads filtered by `embedding_model`, sends redacted lead context plus enriched neighbor firmographics/status to the configured local or OpenAI-compatible scorer (with local fallback), and writes conversion probability + reasoning into `lead_scores`.
- Idempotency path: uses an atomic Redis Lua script to bind a key to one request payload, protect concurrent creates, and replay the stored response.

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
GET  /v1/leads/{id}/score
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
    COALESCE(l.company_size, 0),
    COALESCE(l.annual_revenue, 0)::float8,
    COALESCE(l.notes, ''),
    l.status,
    1 - (e.embedding <=> $3::vector) AS similarity
FROM lead_embeddings e
JOIN leads l ON l.id = e.lead_id
WHERE e.lead_id <> $1
  AND e.embedding_model = $2
ORDER BY e.embedding <=> $3::vector
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
- Day 4 uses atomic Redis operations for safe create retries, detects conflicting payloads, and uses SHA-256 content hashes for embedding updates.
- Day 5 keeps RAG in Postgres with pgvector before introducing heavier vector infrastructure, while preserving score history for evaluation.
- Semantic embeddings are pluggable: local hash for offline demos, remote OpenAI-compatible embeddings for production retrieval quality.
- Similarity search is scoped to one `embedding_model` so local and remote vector spaces never mix.
- Remote LLM scoring redacts email/phone and falls back to the local heuristic scorer on provider failure.
