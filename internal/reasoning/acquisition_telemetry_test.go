package reasoning

import (
	"math"
	"strings"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestPlanningTelemetryDistinguishesUnknownFromObservedZero(t *testing.T) {
	for _, test := range []struct {
		name     string
		coverage map[string]any
		want     []string
	}{
		{"missing", nil, []string{`"performedScrolls":null`, `"newCandidateCount":null`, `"hasMoreCandidateSignal":null`, `"anchorCount":null`, `"continuationReady":null`, `"retryAttempts":null`}},
		{"observed zero", map[string]any{"performedScrolls": 0, "frontier": map[string]any{"newCandidateCount": float64(0), "hasMoreCandidateSignal": false, "anchorKeys": []any{}}}, []string{`"performedScrolls":0`, `"newCandidateCount":0`, `"hasMoreCandidateSignal":false`, `"anchorCount":0`, `"continuationReady":false`}},
		{"malformed", map[string]any{"performedScrolls": -1, "frontier": map[string]any{"newCandidateCount": 1.5, "hasMoreCandidateSignal": "false", "anchorKeys": []any{42}}}, []string{`"performedScrolls":null`, `"newCandidateCount":null`, `"hasMoreCandidateSignal":null`, `"continuationReady":null`}},
		{"explicit blocked", map[string]any{"frontier": map[string]any{"anchorKeys": []string{"private-anchor"}, "continuationReady": false}}, []string{`"anchorCount":1`, `"continuationReady":false`}},
		{"explicit unknown", map[string]any{"frontier": map[string]any{"anchorKeys": []string{"private-anchor"}, "continuationReady": nil}}, []string{`"anchorCount":1`, `"continuationReady":null`}},
	} {
		t.Run(test.name, func(t *testing.T) {
			prompt := buildPlanningPrompt(domain.Run{Source: domain.SourceX}, domain.Observation{Source: domain.SourceX, Coverage: test.coverage}, nil)
			for _, want := range test.want {
				if !strings.Contains(prompt, want) {
					t.Fatalf("missing %s in %s", want, prompt)
				}
			}
			if strings.Contains(prompt, "private-anchor") {
				t.Fatal("native anchor leaked to planner")
			}
		})
	}
	for _, invalid := range []any{math.NaN(), math.Inf(1), float64(math.MaxInt32) + 1, int64(-1)} {
		if planningCount(invalid) != nil {
			t.Fatalf("invalid count accepted: %v", invalid)
		}
	}
}
