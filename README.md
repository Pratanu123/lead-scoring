# lead-scoring

Production-grade AI lead scoring backend with CRM-style ingestion, retrieval-augmented lead comparison, and AI-assisted conversion scoring.

## Day 1 Scope

- Clean Go HTTP API skeleton.
- Docker Compose with Postgres + pgvector and Redis.
- Local browser UIs for Postgres and Redis.
- Postgres schema for leads, embeddings, and AI scores.
- Working `POST /create-lead` endpoint.
- Health endpoint that checks Postgres and Redis.

## Day 2 Scope

- Scalability basics applied to a stateless Go API.
- Load-balancer-friendly lead API shape.
- Service-backed `GET /v1/leads` and `GET /v1/leads/{id}` endpoints.
- Read-path notes for pagination and horizontal scaling.

## Local Setup

Install Go on macOS only if you want to run Go commands outside Docker:

```bash
brew install go
```

Most daily commands are available through `make`:

```bash
make help
make dev
make test
make logs
```

Run Go module tidy inside Docker when dependencies change:

```bash
make tidy
```

Run the full stack directly with Docker if you prefer:

```bash
docker compose up --build -d
```

Health check:

```bash
curl http://localhost:8080/healthz
```

Developer UIs:

- API: http://localhost:8080
- Postgres UI: http://localhost:8081
- Redis UI: http://localhost:8082
- Grafana: http://localhost:3000
- OpenSearch Dashboards: http://localhost:5601
- Prometheus: http://localhost:9090

OpenSearch Dashboards login:

```text
Username: admin
Password: SecureLeadScore_2024!
```

To view API logs in OpenSearch Dashboards:

1. Open `http://localhost:5601`.
2. Go to `Dashboards Management` -> `Index patterns`.
3. Use `logs-*` as the index pattern. Do not use `log-*`.
4. Choose `time` as the time field.
5. Open `Discover`, select `logs-*`, and set the time picker to `Last 24 hours`.

Postgres UI login:

```text
System: PostgreSQL
Server: postgres
Username: root
Password: root
Database: lead_scoring
```

Create a lead:

```bash
curl -X POST http://localhost:8080/v1/create-leads \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: acme-logistics-demo-1" \
  -d '{
    "company_name": "Acme Logistics",
    "contact_name": "Riya Shah",
    "email": "riya@acmelogistics.example",
    "phone": "+91-9999999999",
    "source": "webinar",
    "industry": "logistics",
    "company_size": 250,
    "annual_revenue": 12000000,
    "notes": "Interested in CRM automation and dialer integrations"
  }'
```

List leads:

```bash
curl "http://localhost:8080/v1/get-leads?limit=10&offset=0"
```

Get one lead:

```bash
curl http://localhost:8080/v1/get-leads/<lead-id>
```

Store or refresh a lead embedding:

```bash
curl -X POST http://localhost:8080/v1/leads/<lead-id>/embeddings
```

Retrieve similar leads:

```bash
curl "http://localhost:8080/v1/leads/<lead-id>/similar?limit=5"
```

Score a lead with the local RAG scorer:

```bash
curl -X POST http://localhost:8080/v1/leads/<lead-id>/score
```

Local scoring and local hash embeddings work without credentials. To use OpenAI-compatible APIs instead, copy `.env.example` to `.env`, set the endpoints and models, then run `make restart`:

```text
LLM_API_URL=https://your-provider.example/v1/chat/completions
LLM_API_KEY=your-api-key
LLM_MODEL=your-model-name
EMBEDDING_API_URL=https://your-provider.example/v1/embeddings
EMBEDDING_API_KEY=your-api-key
EMBEDDING_MODEL=text-embedding-3-small
```

Behavior:

- Embeddings default to `local-hash-embedding-v1`. When `EMBEDDING_API_URL` is set, the API uses a remote embeddings provider and stores vectors under that model name.
- Unchanged lead text skips re-embedding via SHA-256 `content_hash`.
- Similar-lead search filters by `embedding_model` so local and remote vectors never mix.
- The scorer sends redacted lead + enriched similar-lead context (notes, size, revenue, status), expects structured conversion probability and reasoning, and persists the provider model name with the score.
- Remote LLM scoring falls back to the local heuristic scorer if the provider call fails.

Get the latest score and score history:

```bash
curl http://localhost:8080/v1/leads/<lead-id>/score
curl "http://localhost:8080/v1/leads/<lead-id>/scores?limit=20"
```

Apply migrations after pulling schema changes:

```bash
make migrate
```

## Postman

Import these files into Postman and run the collection in order:

- `postman/lead-scoring-day4-day5.postman_collection.json`
- `postman/lead-scoring-local.postman_environment.json`

The collection automatically creates a unique lead and verifies:

- First create request returns `201`.
- Identical retry returns the original response with `X-Idempotent-Replay: true`.
- Reusing the same idempotency key with another payload returns `409`.
- Embedding refresh, similar-lead retrieval, RAG scoring, latest score, and score history all work.

Reset local database volumes:

```bash
make reset
```

## Architecture Docs

See [docs/architecture.md](docs/architecture.md).

## Day 2 Schedule

`7:00-7:30`
Scalability basics for stateless APIs, connection pools, and pagination limits.

`7:30-8:00`
Design a load-balanced lead API with multiple app instances behind a reverse proxy.

`8:00-8:45`
Implement repository and service read methods.

`8:45-9:15`
Add controllers and routes for list/detail lead APIs.

`9:15-9:40`
Commit with `feat: add lead read APIs`.

`9:40-10:00`
Update README and architecture notes with the Day 2 API surface.

## Day 3-5 Scope

- Day 3: Redis read caching for lead list/detail endpoints.
- Day 4: atomic Redis idempotency, concurrent-request protection, payload-conflict detection, and content hashing for embeddings.
- Day 5: practical RAG with pgvector similarity search, local or OpenAI-compatible LLM scoring, latest score, and score history.

## Day 4 Schedule

`7:00-7:30`
Understand at-least-once delivery, retries, idempotency keys, and why a plain Redis `GET` followed by `SET` is not atomic.

`7:30-8:00`
Design the idempotency state machine: proceed, replay, conflicting payload, and request in progress.

`8:00-9:15`
Run the Postman Day 4 folder and inspect Redis keys in Redis Commander.

`9:15-10:00`
Review logs, update docs, and commit with `feat: harden create lead idempotency`.

## Day 5 Schedule

`7:00-7:30`
Understand embeddings, cosine distance, RAG context, and the separation between retrieval and scoring.

`7:30-8:00`
Design synchronous scoring now and the future async worker path.

`8:00-9:15`
Run the Postman Day 5 folder and inspect `lead_embeddings` and `lead_scores` in Adminer.

`9:15-10:00`
Review score reasoning, run tests, and commit with `feat: complete rag scoring workflow`.

## Day 1 Commit Message

```text
chore: bootstrap lead scoring API
```
