# Architecture

## High-Level Design

```text
Browser UI / Client / CRM
    |
    +--> Go API (auth + rate limit)
    |       |
    |       +--> Lead Controller / Service / Repository
    |       +--> Jobs enqueue (Postgres + Redis queue)
    |       +--> WebSocket job events
    |       +--> Static web UI
    |
    +--> Go Worker
            |
            +--> Embedder (local or remote)
            +--> Scorer (local or remote + fallback)
            +--> Postgres / Redis

Observability:

Go API JSON logs -> Vector -> OpenSearch -> OpenSearch Dashboards
Go API /metrics -> Prometheus -> Grafana (provisioned Lead Scoring dashboard)

Scale-out path:

Client
    -> Load Balancer
        -> API Instance N
        -> Worker Instance N
            -> Shared Postgres
            -> Shared Redis
```

## Low-Level Design

### Services

- API service: authenticated lead APIs, async job enqueue, WebSocket updates, static UI.
- Worker service: claims embed/score jobs from Redis, executes RAG pipeline, updates job state.
- Postgres UI: Adminer for local schema/data inspection.
- Redis UI: Redis Commander for local key inspection.
- Lead service: validates/normalizes leads, caches reads/scores, enqueues jobs.
- Lead repository: owns SQL persistence.
- Embedding path: pluggable `Embedder` (`local-hash-embedding-v1` or OpenAI-compatible remote).
- Scoring path: similar leads filtered by embedding model, redacted LLM context, local fallback.
- Idempotency path: atomic Redis Lua for create-lead retries.
- Auth path: shared `API_KEY` bearer auth and Redis fixed-window rate limits.

### Database Schema

- `leads`: CRM lead profile including outcome `status`.
- `lead_embeddings`: vector representation using `vector(1536)`.
- `lead_scores`: conversion probability + reasoning history.
- `jobs`: async embed/score job state machine.

### APIs

```text
GET  /healthz
GET  /metrics
GET  /                 (web UI)
POST /v1/leads
GET  /v1/leads
GET  /v1/leads/{id}
PATCH /v1/leads/{id}
POST /v1/leads/{id}/embeddings
GET  /v1/leads/{id}/similar
POST /v1/leads/{id}/score      -> 202 + job_id
GET  /v1/leads/{id}/score
GET  /v1/leads/{id}/scores
GET  /v1/jobs/{id}
GET  /v1/ws?lead_id=...&token=...
```

## Async RAG Flow

```text
CreateLead -> enqueue embed job -> worker embeds into pgvector
ScoreLead  -> enqueue score job -> worker retrieves similar -> scores -> lead_scores
UI         -> WebSocket job events -> refresh latest score
```

## Scaling Notes

- Postgres remains the transactional source of truth.
- pgvector is sufficient before introducing a dedicated vector DB.
- Redis handles idempotency, read/score cache, rate limits, job wake signals, and pub/sub events.
- API is stateless; add more API/worker replicas behind a load balancer as needed.
- Similarity search is scoped to one `embedding_model`.
- Remote LLM scoring redacts email/phone and falls back to the local heuristic scorer.
