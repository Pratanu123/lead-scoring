package jobs

import (
	"encoding/json"
	"testing"
)

func TestJobEventJSONRoundTrip(t *testing.T) {
	event := Event{
		JobID:  "job-1",
		LeadID: "lead-1",
		Type:   TypeScore,
		Status: StatusCompleted,
	}

	payload, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}

	var decoded Event
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal event: %v", err)
	}

	if decoded.JobID != event.JobID || decoded.LeadID != event.LeadID || decoded.Type != TypeScore || decoded.Status != StatusCompleted {
		t.Fatalf("unexpected decoded event: %+v", decoded)
	}
}

func TestJobConstants(t *testing.T) {
	if TypeEmbed != "embed" || TypeScore != "score" {
		t.Fatalf("unexpected job type constants: %q %q", TypeEmbed, TypeScore)
	}
	if StatusQueued != "queued" || StatusRunning != "running" || StatusCompleted != "completed" || StatusFailed != "failed" {
		t.Fatal("unexpected job status constants")
	}
}
