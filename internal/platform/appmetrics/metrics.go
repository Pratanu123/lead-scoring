package appmetrics

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
)

type Registry struct {
	mu sync.Mutex

	scoreRequestsTotal   sync.Map // result -> *int64
	scoreDurationMsTotal atomic.Int64
	scoreDurationCount   atomic.Int64

	embeddingDurationMsTotal atomic.Int64
	embeddingDurationCount   atomic.Int64

	llmFallbackTotal atomic.Int64

	jobsTotal sync.Map // "type|status" -> *int64

	cacheHitsTotal   sync.Map // kind -> *int64
	cacheMissesTotal sync.Map // kind -> *int64
}

func NewRegistry() *Registry {
	return &Registry{}
}

func (r *Registry) IncScoreRequest(result string) {
	incLabeled(&r.scoreRequestsTotal, result)
}

func (r *Registry) ObserveScoreDuration(ms int64) {
	r.scoreDurationMsTotal.Add(ms)
	r.scoreDurationCount.Add(1)
}

func (r *Registry) ObserveEmbeddingDuration(ms int64) {
	r.embeddingDurationMsTotal.Add(ms)
	r.embeddingDurationCount.Add(1)
}

func (r *Registry) IncLLMFallback() {
	r.llmFallbackTotal.Add(1)
}

func (r *Registry) IncJob(jobType string, status string) {
	incLabeled(&r.jobsTotal, jobType+"|"+status)
}

func (r *Registry) IncCacheHit(kind string) {
	incLabeled(&r.cacheHitsTotal, kind)
}

func (r *Registry) IncCacheMiss(kind string) {
	incLabeled(&r.cacheMissesTotal, kind)
}

func (r *Registry) Format() string {
	var b strings.Builder

	b.WriteString("# HELP lead_score_requests_total Total lead score operations\n")
	b.WriteString("# TYPE lead_score_requests_total counter\n")
	r.scoreRequestsTotal.Range(func(key, value any) bool {
		fmt.Fprintf(&b, "lead_score_requests_total{result=%q} %d\n", key.(string), atomic.LoadInt64(value.(*int64)))
		return true
	})

	count := r.scoreDurationCount.Load()
	totalSec := float64(r.scoreDurationMsTotal.Load()) / 1000.0
	avg := 0.0
	if count > 0 {
		avg = totalSec / float64(count)
	}
	b.WriteString("# HELP lead_score_duration_seconds Average lead score duration in seconds\n")
	b.WriteString("# TYPE lead_score_duration_seconds gauge\n")
	fmt.Fprintf(&b, "lead_score_duration_seconds %.6f\n", avg)
	b.WriteString("# HELP lead_score_duration_seconds_count Score duration samples\n")
	b.WriteString("# TYPE lead_score_duration_seconds_count counter\n")
	fmt.Fprintf(&b, "lead_score_duration_seconds_count %d\n", count)

	embCount := r.embeddingDurationCount.Load()
	embTotalSec := float64(r.embeddingDurationMsTotal.Load()) / 1000.0
	embAvg := 0.0
	if embCount > 0 {
		embAvg = embTotalSec / float64(embCount)
	}
	b.WriteString("# HELP lead_embedding_duration_seconds Average embedding duration in seconds\n")
	b.WriteString("# TYPE lead_embedding_duration_seconds gauge\n")
	fmt.Fprintf(&b, "lead_embedding_duration_seconds %.6f\n", embAvg)
	b.WriteString("# HELP lead_embedding_duration_seconds_count Embedding duration samples\n")
	b.WriteString("# TYPE lead_embedding_duration_seconds_count counter\n")
	fmt.Fprintf(&b, "lead_embedding_duration_seconds_count %d\n", embCount)

	b.WriteString("# HELP lead_llm_fallback_total Remote LLM fallback count\n")
	b.WriteString("# TYPE lead_llm_fallback_total counter\n")
	fmt.Fprintf(&b, "lead_llm_fallback_total %d\n", r.llmFallbackTotal.Load())

	b.WriteString("# HELP lead_jobs_total Job lifecycle events\n")
	b.WriteString("# TYPE lead_jobs_total counter\n")
	r.jobsTotal.Range(func(key, value any) bool {
		parts := strings.SplitN(key.(string), "|", 2)
		jobType, status := "unknown", "unknown"
		if len(parts) == 2 {
			jobType, status = parts[0], parts[1]
		}
		fmt.Fprintf(&b, "lead_jobs_total{type=%q,status=%q} %d\n", jobType, status, atomic.LoadInt64(value.(*int64)))
		return true
	})

	b.WriteString("# HELP lead_cache_hits_total Cache hits by kind\n")
	b.WriteString("# TYPE lead_cache_hits_total counter\n")
	r.cacheHitsTotal.Range(func(key, value any) bool {
		fmt.Fprintf(&b, "lead_cache_hits_total{kind=%q} %d\n", key.(string), atomic.LoadInt64(value.(*int64)))
		return true
	})

	b.WriteString("# HELP lead_cache_misses_total Cache misses by kind\n")
	b.WriteString("# TYPE lead_cache_misses_total counter\n")
	r.cacheMissesTotal.Range(func(key, value any) bool {
		fmt.Fprintf(&b, "lead_cache_misses_total{kind=%q} %d\n", key.(string), atomic.LoadInt64(value.(*int64)))
		return true
	})

	return b.String()
}

func incLabeled(m *sync.Map, key string) {
	actual, _ := m.LoadOrStore(key, new(int64))
	atomic.AddInt64(actual.(*int64), 1)
}
