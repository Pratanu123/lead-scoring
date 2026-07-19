# lead-scoring

Local Docker production-ready AI lead scoring stack: CRM-style ingestion, async RAG scoring, authenticated API, WebSocket job updates, minimal web UI, and provisioned Grafana dashboards.

## What you get

- Go API + worker
- Postgres + pgvector, Redis
- Async embed/score jobs with live WebSocket status
- API key auth + Redis rate limits
- Lead status/outcome updates for RAG feedback
- Minimal professional web UI at `http://localhost:8080`
- Prometheus metrics + Grafana Lead Scoring dashboard
- OpenSearch log pipeline via Vector

## Quick start

```bash
make dev
make migrate   # required if the DB volume already existed before jobs migration
```

Open:

- UI: http://localhost:8080
- API health: http://localhost:8080/healthz
- Grafana: http://localhost:3000 (`admin` / `admin`) → folder **Lead Scoring**
- Prometheus: http://localhost:9090
- Adminer: http://localhost:8081
- Redis Commander: http://localhost:8082
- OpenSearch Dashboards: http://localhost:5601

Default API key (also used by the UI login):

```text
dev-lead-scoring-key
```

## Auth

All `/v1/*` write/read APIs require:

```text
Authorization: Bearer dev-lead-scoring-key
```

`/healthz` and `/metrics` remain open for probes and Prometheus.

## Core API examples

Create a lead (queues an embed job):

```bash
curl -X POST http://localhost:8080/v1/leads \
  -H "Authorization: Bearer dev-lead-scoring-key" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: demo-1" \
  -d '{
    "company_name": "Acme Logistics",
    "contact_name": "Riya Shah",
    "email": "riya@acmelogistics.example",
    "source": "webinar",
    "industry": "logistics",
    "company_size": 250,
    "annual_revenue": 12000000,
    "notes": "Interested in CRM automation"
  }'
```

Enqueue scoring (returns `202` + `job_id`):

```bash
curl -X POST http://localhost:8080/v1/leads/<lead-id>/score \
  -H "Authorization: Bearer dev-lead-scoring-key"
```

Poll job status:

```bash
curl http://localhost:8080/v1/jobs/<job-id> \
  -H "Authorization: Bearer dev-lead-scoring-key"
```

Update outcome status:

```bash
curl -X PATCH http://localhost:8080/v1/leads/<lead-id> \
  -H "Authorization: Bearer dev-lead-scoring-key" \
  -H "Content-Type: application/json" \
  -d '{"status":"won"}'
```

WebSocket job stream:

```text
ws://localhost:8080/v1/ws?lead_id=<lead-id>&token=dev-lead-scoring-key
```

## Optional remote AI providers

Copy `.env.example` values into a local `.env` override or edit `.env.example`, then `make restart`:

```text
LLM_API_URL=https://your-provider.example/v1/chat/completions
LLM_API_KEY=your-api-key
LLM_MODEL=your-model-name
EMBEDDING_API_URL=https://your-provider.example/v1/embeddings
EMBEDDING_API_KEY=your-api-key
EMBEDDING_MODEL=text-embedding-3-small
```

Local hash embeddings + heuristic scoring work with no credentials. Remote LLM scoring falls back to the local scorer on failure.

## Makefile helpers

```bash
make help
make health
make lead
make score LEAD_ID=<id>
make job JOB_ID=<id>
make status LEAD_ID=<id> STATUS=won
make test
make reset
```

## Postman

Import:

- `postman/lead-scoring-mvp.postman_collection.json`
- `postman/lead-scoring-local.postman_environment.json`

The environment includes `apiKey=dev-lead-scoring-key`.

## Architecture

See [docs/architecture.md](docs/architecture.md).

## Monitoring

See [MONITORING_GUIDE.md](MONITORING_GUIDE.md) and [QUICK_START_MONITORING.md](QUICK_START_MONITORING.md).

Grafana is provisioned automatically with a Prometheus datasource and the **Lead Scoring** dashboard (request rate, score/embedding latency, jobs, LLM fallbacks, cache hit ratio).
