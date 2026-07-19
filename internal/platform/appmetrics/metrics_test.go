package appmetrics

import (
	"strings"
	"testing"
)

func TestRegistryFormatIncludesDomainSeries(t *testing.T) {
	reg := NewRegistry()
	reg.IncScoreRequest("success")
	reg.ObserveScoreDuration(250)
	reg.ObserveEmbeddingDuration(100)
	reg.IncLLMFallback()
	reg.IncJob("score", "queued")
	reg.IncJob("score", "completed")
	reg.IncCacheHit("score")
	reg.IncCacheMiss("lead")

	out := reg.Format()
	for _, needle := range []string{
		`lead_score_requests_total{result="success"} 1`,
		"lead_score_duration_seconds ",
		"lead_embedding_duration_seconds ",
		"lead_llm_fallback_total 1",
		`lead_jobs_total{type="score",status="queued"} 1`,
		`lead_jobs_total{type="score",status="completed"} 1`,
		`lead_cache_hits_total{kind="score"} 1`,
		`lead_cache_misses_total{kind="lead"} 1`,
	} {
		if !strings.Contains(out, needle) {
			t.Fatalf("expected metrics output to contain %q\n%s", needle, out)
		}
	}
}
