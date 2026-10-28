package engine

import (
	"strings"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestExecutingBridgeSurfacePolicy(t *testing.T) {
	runtime, _ := testEngine(t)
	valid := ExpectedHeartbeat()
	if status := runtime.RecordHeartbeat(valid); !status.Compatible {
		t.Fatalf("matching loaded policy rejected: %+v", status)
	}
	for _, policy := range []domain.BridgeCaptureSurfacePolicy{
		{},
		{Schema: "old", Fingerprint: ExpectedBridgeSurfacePolicyFingerprint},
		{Schema: "aku.surface-policy.v1", Fingerprint: strings.Repeat("0", 64)},
	} {
		stale := ExpectedHeartbeat()
		stale.CaptureSurfacePolicy = policy
		status := runtime.RecordHeartbeat(stale)
		if status.Compatible || !strings.Contains(strings.Join(status.Reasons, " "), "reload required") {
			t.Fatalf("stale or missing executing policy accepted: %+v", status)
		}
	}
}
