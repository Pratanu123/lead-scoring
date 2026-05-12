# 🚀 Quick Start: Viewing Logs, Metrics & Dashboards

## ✅ All Services Running

```
✓ API              http://localhost:8080      (metrics at /metrics)
✓ OpenSearch       https://localhost:9200     (REST API)
✓ OpenSearch Dashboard  http://localhost:5601
✓ Prometheus       http://localhost:9090
✓ Grafana          http://localhost:3000
```

---

## 📊 **1. CHECK METRICS IN PROMETHEUS**

### Step 1: Access Prometheus
```
http://localhost:9090
```

### Step 2: View Raw Metrics from API

Click **Graph** tab, then in the **Expression** field, type:

```
http_requests_total
```

Click **Execute**. You should see:

```
http_requests_total: 9
```

### Step 3: Try These Queries

**See requests by method:**
```promql
sum(http_requests_by_method)
```

**See requests by status code:**
```promql
sum(http_requests_by_status)
```

**See error rate:**
```promql
http_request_errors_total / http_requests_total
```

**See average response time:**
```promql
http_request_duration_seconds_total / http_requests_total
```

---

## 📈 **2. CREATE GRAFANA DASHBOARD**

### Step 1: Access Grafana
```
http://localhost:3000
Username: admin
Password: admin
```

You'll be asked to change password (skip or set new one).

### Step 2: Add Prometheus Data Source

1. Click **Configuration** (⚙️ icon in left sidebar)
2. Select **Data Sources**
3. Click **Add data source**
4. Select **Prometheus**
5. URL: `http://prometheus:9090`
6. Click **Save & test** ✓

### Step 3: Create Your First Dashboard

1. Click **+** (Create icon in left sidebar)
2. Select **Dashboard**
3. Click **Add panel**
4. In the **Metrics** secti, select **Prometheus**
5. Enter this query:
   ```
   http_requests_total
   ```
6. Click outside or press Enter
7. Title: "Total API Requests"
8. Click **Save** at top right

### Step 4: Add More Panels

Repeat Step 3 with these queries:

**Panel 2: Requests by Method**
```promql
http_requests_by_method
```
- Visualization: **Pie chart**

**Panel 3: Requests by Status**
```promql
http_requests_by_status
```
- Visualization: **Table**

**Panel 4: Error Count**
```promql
http_request_errors_total
```
- Visualization: **Stat**

---

## 📝 **3. VIEW LOGS IN OPENSEARCH DASHBOARDS**

### Step 1: Access OpenSearch Dashboards
```
http://localhost:5601
Username: admin
Password: SecureLeadScore_2024!
```

### Step 2: Create Index Pattern (First Time Only)

1. Click **Stack Management** (bottom left)
2. Select **Index Patterns**
3. Click **Create index pattern**
4. Pattern: `logs-*`
5. Click **Next step**
6. Time field: `@timestamp`
7. Click **Create index pattern**

### Step 3: View Logs

1. Click **Discover** in left sidebar
2. Select `logs-*` from dropdown
3. You should see API request logs with:
   - Timestamp
   - Level (INFO, WARN, ERROR)
   - Message
   - Request details

### Step 4: Search Logs

**Find errors:**
```
level:ERROR
```

**Find specific endpoint:**
```
path:"/v1/leads"
```

**Find slow requests:**
```
duration:>500ms
```

---

## 🔄 **Generate More Traffic to See Data**

```bash
# Create more leads
for i in {4..10}; do
  curl -X POST http://localhost:8080/v1/leads \
    -H "Content-Type: application/json" \
    -d "{\"company_name\":\"Company$i\",\"contact_name\":\"Contact$i\",\"email\":\"contact$i@example.com\",\"source\":\"test\",\"industry\":\"tech\"}"
done

# List leads multiple times
for i in {1..5}; do
  curl http://localhost:8080/v1/get-leads
done

# Generate an error (missing required field)
curl -X POST http://localhost:8080/v1/leads \
  -H "Content-Type: application/json" \
  -d '{"contact_name":"No Company"}'
```

Then refresh your dashboards to see updated metrics!

---

## 📋 **Full Architecture**

```
Your API (Go)
    ↓
/metrics endpoint (Prometheus format)
    ↓
Prometheus ←→ Grafana (visualize metrics)
    
Your API (structured logs to stdout)
    ↓
(To be configured) Log forwarder (Vector/Filebeat)
    ↓
OpenSearch (full-text search)
    ↓
OpenSearch Dashboards (visualize logs)
```

---

## 🛠️ **Troubleshooting**

### "No data" in Prometheus
- Check: Does API have `/metrics` endpoint? ✓ Yes!
- Check: Is Prometheus scraping it? http://localhost:9090/targets

### "No metrics showing" in Grafana
- Try: Click **Refresh** (icon in top-right)
- Try: Change time range (top-right "Last 1 hour" → "Last 5 minutes")

### OpenSearch Dashboards won't load
- Try: Refresh page
- Try: Clear browser cache (Ctrl+Shift+Delete)
- Check: OpenSearch is healthy
  ```bash
  curl -k -u admin:SecureLeadScore_2024! https://localhost:9200/_cluster/health
  ```

---

## 📚 **Next Steps**

1. ✅ **Metrics**: Already working! Navigate to Prometheus and Grafana.
2. 📝 **Logs**: Create more data and check OpenSearch Dashboards (Discover tab).
3. 🚨 **Alerting**: Set up alert rules in Prometheus for high error rates.
4. 📤 **Log Shipping**: Add Vector or Filebeat to forward logs to OpenSearch.

---

## 🎯 **Quick Commands**

```bash
# See metrics endpoint
curl http://localhost:8080/metrics

# See API health
curl http://localhost:8080/healthz

# Check Prometheus targets
open http://localhost:9090/targets

# Check Prometheus alerts
open http://localhost:9090/alerts

# Create test lead
curl -X POST http://localhost:8080/v1/leads \
  -H "Content-Type: application/json" \
  -d '{"company_name":"Test","email":"test@example.com","source":"test"}'

# List all leads
curl http://localhost:8080/v1/get-leads
```

---

## Service Credentials

| Service | URL | Username | Password |
|---------|-----|----------|----------|
| Open Search Dashboards | http://localhost:5601 | admin | SecureLeadScore_2024! |
| Grafana | http://localhost:3000 | admin | admin |
| Prometheus | http://localhost:9090 | (none) | (none) |
| OpenSearch REST API | https://localhost:9200 | admin | SecureLeadScore_2024! |

