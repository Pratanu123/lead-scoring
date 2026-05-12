# Monitoring & Observability Guide

## Overview

Your lead-scoring project includes 3 monitoring tools:

1. **OpenSearch Dashboards** (Logs aggregation)
2. **Prometheus** (Metrics collection)
3. **Grafana** (Dashboards & visualization)

---

## 1. OpenSearch Dashboards (Logs)

### Access
```
URL: http://localhost:5601
Username: admin
Password: SecureLeadScore_2024!
```

### What's Being Logged?

Currently, all API requests are logged as JSON to stdout:

```json
{
  "time": "2025-01-15T10:30:00Z",
  "level": "INFO",
  "msg": "CreateLead request",
  "method": "POST",
  "path": "/v1/leads"
}
```

### View Logs in OpenSearch

#### Step 1: Create an Index Pattern (First Time Only)

1. Go to http://localhost:5601
2. Click **Stack Management** (bottom left)
3. Select **Index Patterns**
4. Click **Create index pattern**
5. Enter pattern name: `logs-*`
6. Click **Next step**
7. Select `@timestamp` as the time field
8. Click **Create index pattern**

#### Step 2: View Logs

1. Click **Discover** in the left sidebar
2. Select `logs-*` from the dropdown
3. View all logged events with timestamps and fields

#### Step 3: Search & Filter

- **Search bar**: Type `level:ERROR` to find errors
- **Field filters**: Click any field value to filter (e.g., `method: POST`)
- **Time range**: Select time range at top right

### Coming Soon: Send Logs to OpenSearch

To automatically forward API logs to OpenSearch, we'll add a log exporter that sends JSON logs to OpenSearch's HTTP API:

```go
// Future: Install go.uber.org/zap with OpenSearch exporter
// All structured logs will be pushed to OpenSearch automatically
```

---

## 2. Prometheus (Metrics)

### Access
```
URL: http://localhost:9090
```

### What Metrics Are Collected?

Prometheus is configured to scrape metrics from:

- **API** → `http://api:8080/metrics` (currently returns 404)
- **Postgres** → Connection pool stats
- **Redis** → Connection stats
- **Prometheus itself** → Internal health metrics

### View Metrics in Prometheus

#### Step 1: Access Prometheus UI

1. Go to http://localhost:9090
2. Click **Graph** tab

#### Step 2: Query Metrics

In the **Expression** field, type a metric name. Prometheus auto-suggests available metrics:

```promql
# View all metrics starting with "http_"
http_*

# Count HTTP requests
http_requests_total

# Request duration in seconds
http_request_duration_seconds

# Request size in bytes
http_request_size_bytes
```

#### Step 3: Execute Query

1. Type a metric name
2. Click **Execute**
3. View the result as **Table** or **Graph**

#### Example Queries

```promql
# Total requests
sum(http_requests_total)

# Requests by method
sum(http_requests_total) by (method)

# Error rate
sum(rate(http_requests_total{status=~"[45].."}[5m]))

# API latency (99th percentile)
histogram_quantile(0.99, http_request_duration_seconds)
```

---

## 3. Grafana (Dashboards)

### Access
```
URL: http://localhost:3000
Username: admin
Password: admin
```

### Set Up Data Sources

#### Step 1: Add Prometheus as Data Source

1. Go to http://localhost:3000
2. Click **Configuration** (gear icon) → **Data sources**
3. Click **Add data source**
4. Select **Prometheus**
5. Set URL: `http://prometheus:9090`
6. Click **Save & test**

#### Step 2: Add OpenSearch as Data Source

1. Click **Configuration** → **Data sources**
2. Click **Add data source**
3. Select **Opensearch**
4. Set URL: `https://opensearch:9200`
5. Set Username: `admin`
6. Set Password: `SecureLeadScore_2024!`
7. Enable **Skip TLS verify** (for local dev)
8. Click **Save & test**

### Create Your First Dashboard

#### Step 1: Create Dashboard

1. Click **+** (Create) → **Dashboard**
2. Click **Add panel**
3. Name: "API Request Rate"

#### Step 2: Add Prometheus Query

1. Select **Prometheus** as data source
2. In the **Metrics** field, enter:
   ```
   rate(http_requests_total[5m])
   ```
3. Click **Run queries**
4. Click **Apply**

#### Step 3: View Dashboard

- Metrics will display as a time-series graph
- Customize with different visualization types (Gauge, Stat, Table, etc.)

### Pre-Built Dashboard Templates

To import a community dashboard:

1. Click **+** (Create) → **Import**
2. Paste dashboard ID:
   - **1860** → Node Exporter Full
   - **3662** → Prometheus
   - **11074** → Node Exporter for Prometheus

3. Select Prometheus as data source
4. Click **Import**

---

## Testing the Full Stack

### Step 1: Generate API Traffic

Create some sample leads to generate logs and metrics:

```bash
# Create a lead
curl -X POST http://localhost:8080/v1/leads \
  -H "Content-Type: application/json" \
  -d '{
    "company_name": "TestCorp",
    "contact_name": "John Doe",
    "email": "john@testcorp.example",
    "phone": "+1-555-0100",
    "source": "api_test",
    "industry": "tech"
  }'

# Check health (generates log)
curl http://localhost:8080/healthz

# List leads (generates log)
curl http://localhost:8080/v1/get-leads
```

### Step 2: View Logs in OpenSearch

1. Go to http://localhost:5601
2. Click **Discover**
3. Select `logs-*` index
4. You should see the API request logs with timestamps

### Step 3: View Metrics in Prometheus

1. Go to http://localhost:9090
2. Execute query: `up` (checks if targets are up)
3. You should see:
   - `prometheus=1`
   - May see `api=0` (because /metrics endpoint not yet implemented)

### Step 4: Create Grafana Dashboard

1. Go to http://localhost:3000
2. Create a new dashboard
3. Add Prometheus data source
4. Add a panel with query: `up`
5. Save dashboard

---

## Logs Flow (Proposed Design)

```
API (slog JSON output)
    ↓
Container Stdout
    ↓
Vector / Fluentd / Logstash (log forwarder)
    ↓
OpenSearch (logs-*)
    ↓
OpenSearch Dashboards (visualize)
```

### To Enable Log Shipping

Install a log forwarder like **Vector** or **Filebeat** to send API logs to OpenSearch:

```yaml
# docker-compose.yml addition
vector:
  image: timberio/vector:latest
  environment:
    VECTOR_CP_INPUTS: "syslog"
  volumes:
    - ./vector.toml:/etc/vector/vector.toml:ro
  depends_on:
    - opensearch
```

---

## Metrics Flow (Proposed Design)

```
API (/metrics endpoint, Prometheus format)
    ↓
Prometheus (scrapes every 10s)
    ↓
Time-series database (TSDB)
    ↓
Grafana (queries & visualizes)
```

### To Enable Metrics Exposure

The API needs to:

1. **Install Prometheus client library**:
   ```bash
   go get github.com/prometheus/client_golang/prometheus
   ```

2. **Register metrics**:
   ```go
   httpRequestsTotal := prometheus.NewCounterVec(
       prometheus.CounterOpts{Name: "http_requests_total"},
       []string{"method", "status"},
   )
   ```

3. **Expose `/metrics` endpoint**:
   ```go
   http.Handle("/metrics", promhttp.Handler())
   ```

---

## Troubleshooting

### OpenSearch Dashboards shows "No data"

**Cause**: No logs are being sent to OpenSearch yet.

**Fix**: Currently logs go to stdout. Add a log shipper (Vector/Filebeat) to forward to OpenSearch.

### Prometheus shows "No targets"

**Cause**: Target API is not responding to `/metrics`.

**Fix**: Implement the `/metrics` endpoint in the API or add Prometheus instrumentation.

### Can't connect to OpenSearch in Grafana

**Cause**: SSL/TLS certificate validation failing.

**Fix**: In Grafana data source settings, enable **Skip TLS verification** for local dev.

---

## Next Steps

1. **Add Prometheus instrumentation** to API (http_requests_total, latency histograms)
2. **Set up log forwarding** to OpenSearch (Vector/Filebeat)
3. **Create Grafana dashboards** for:
   - API request rate and errors
   - Database connection pool usage
   - Redis cache hit rate
   - Lead creation funnel
4. **Set up alerting rules** in Prometheus for:
   - High error rate (>1%)
   - Slow API responses (p95 > 500ms)
   - Database connection pool exhaustion

---

## Service URLs Quick Reference

| Tool | URL | Credentials |
|------|-----|-------------|
| OpenSearch Dashboards | http://localhost:5601 | admin / SecureLeadScore_2024! |
| Prometheus | http://localhost:9090 | None |
| Grafana | http://localhost:3000 | admin / admin |
| OpenSearch API | http://localhost:9200 | admin / SecureLeadScore_2024! |

