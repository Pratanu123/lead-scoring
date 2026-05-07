# lead-scoring

Production-grade AI lead scoring backend with CRM-style ingestion, retrieval-augmented lead comparison, and AI-assisted conversion scoring.

## Day 1 Scope

- Clean Go HTTP API skeleton.
- Docker Compose with Postgres + pgvector and Redis.
- Local browser UIs for Postgres and Redis.
- Postgres schema for leads, embeddings, and AI scores.
- Working `POST /create-lead` endpoint.
- Health endpoint that checks Postgres and Redis.

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
curl -X POST http://localhost:8080/create-lead \
  -H "Content-Type: application/json" \
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

Reset local database volumes:

```bash
make reset
```

## Architecture Docs

See [docs/architecture.md](docs/architecture.md).

## Day 1 Commit Message

```text
chore: bootstrap lead scoring API
```
