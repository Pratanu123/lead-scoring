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

List leads:

```bash
curl "http://localhost:8080/v1/leads?limit=10&offset=0"
```

Get one lead:

```bash
curl http://localhost:8080/v1/leads/<lead-id>
```

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

## Day 1 Commit Message

```text
chore: bootstrap lead scoring API
```
