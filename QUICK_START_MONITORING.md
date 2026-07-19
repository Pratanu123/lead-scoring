# Quick Start Monitoring

```bash
make dev
```

Then open:

1. Grafana: http://localhost:3000 (`admin` / `admin`)
2. Dashboard: **Lead Scoring**
3. Generate traffic from the UI or:

```bash
make lead
curl -H "Authorization: Bearer dev-lead-scoring-key" http://localhost:8080/v1/leads
```

4. Confirm series in Prometheus: http://localhost:9090
5. Confirm API metrics: http://localhost:8080/metrics

Logs:

1. OpenSearch Dashboards: http://localhost:5601
2. Index pattern: `logs-*`
3. Time field: `time`
