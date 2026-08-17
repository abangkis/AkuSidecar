package store

import (
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestInboxCapturePerformanceSeparatesBoundedCoverageFromQuality(t *testing.T) {
	complete := inboxCapturePerformance([]domain.Observation{{
		Source:    domain.SourceLinkedIn,
		Snapshots: []domain.Snapshot{{Blocks: []domain.Block{{Text: "Complete bounded candidate"}}}},
		Coverage: map[string]any{
			"status":        "partial",
			"captureMethod": "native_dom",
			"captureQuality": map[string]any{
				"verdict": "complete",
			},
			"adapterHealth": map[string]any{"state": "healthy"},
		},
	}})
	if complete == nil || complete.Outcome != "complete" || complete.RawCoverageStatus != "partial" || complete.Scope != "bounded_viewport" {
		t.Fatalf("complete capture performance=%+v", complete)
	}

	degraded := inboxCapturePerformance([]domain.Observation{{
		Source:    domain.SourceInstagram,
		Snapshots: []domain.Snapshot{{Blocks: []domain.Block{{Text: "Fallback candidate"}}}},
		Coverage: map[string]any{
			"status":        "partial",
			"captureMethod": "instagram_structured_feed_json",
			"captureQuality": map[string]any{
				"verdict": "usable_degraded",
			},
		},
	}})
	if degraded == nil || degraded.Outcome != "degraded" || degraded.CaptureMethod != "instagram_structured_feed_json" {
		t.Fatalf("degraded capture performance=%+v", degraded)
	}
}
