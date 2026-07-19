# Monitoring Guide

This stack ships with working metrics and log shipping. No extra bootstrap is required beyond `make dev`.

## Components

| Component | URL | Purpose |
|-----------|-----|---------|
| API `/metrics` | http://localhost:8080/metrics | Prometheus scrape target |
| Prometheus | http://localhost:9090 | Time-series store |
| Grafana | http://localhost:3000 | Dashboards (`admin` / `admin`) |
| Vector | container logs | Ships API JSON logs to OpenSearch |
| OpenSearch Dashboards | http://localhost:5601 | Log discovery |

## Grafana

1. Open http://localhost:3000
2. Login with `admin` / `admin`
3. Open folder **Lead Scoring** → dashboard **Lead Scoring**

Provisioned files:

- `grafana/provisioning/datasources/datasource.yml`
- `grafana/provisioning/dashboards/dashboards.yml`
- `grafana/dashboards/lead-scoring.json`

Dashboard panels:

- HTTP request rate / error rate
- Score latency
- Embedding latency
- Jobs by type/status
- LLM fallback count
- Cache hit ratio

## Metrics exposed by the API

HTTP:

- `http_requests_total`
- `http_request_errors_total`
- `http_request_duration_seconds_total`
- `http_requests_by_method`
- `http_requests_by_status`

Domain:

- `lead_score_requests_total{result}`
- `lead_score_duration_seconds`
- `lead_embedding_duration_seconds`
- `lead_llm_fallback_total`
- `lead_jobs_total{type,status}`
- `lead_cache_hits_total{kind}`
- `lead_cache_misses_total{kind}`

## OpenSearch logs

1. Open http://localhost:5601 (`admin` / `SecureLeadScore_2024!`)
2. Create index pattern `logs-*`
3. Choose time field `time`
4. Explore in Discover

## Prometheus scrape config

`prometheus.yml` scrapes:

- Prometheus itself
- `api:8080/metrics`

Bare Postgres/Redis ports are intentionally not scraped (no exporters in this MVP).
