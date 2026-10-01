package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

// These fixtures freeze the pre-refactor wire contract at the engine boundary.
// They cover settings projection and the follow-up policy that must not reopen
// or mutate a shared source tab after the initial acquisition.
func TestCapturePayloadPreservesBridgeWireContract(t *testing.T) {
	for _, test := range []struct {
		name         string
		round        int
		continuation map[string]any
		reason       string
	}{
		{name: "first", round: 1},
		{name: "followup", round: 2, continuation: map[string]any{"cursor": "frontier-1"}, reason: "missing_media"},
	} {
		t.Run(test.name, func(t *testing.T) {
			settings := domain.DefaultSettings("standard", "quiet", "guarded_live", true)
			settings.CaptureVisibility = "adaptive_fidelity"
			settings.OpenMissingSource = true
			settings.MaxScrolls = 3
			settings.QualityRetrySettleMS = 1250
			settings.SourceHydrationTimeoutMS[domain.SourceFacebook] = 29000
			payload := capturePayload(domain.Run{Source: domain.SourceFacebook}, "session-lease", settings, test.round, test.continuation, test.reason)
			actualJSON, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			expectedJSON, err := os.ReadFile(filepath.Join("testdata", "collection-"+test.name+".json"))
			if err != nil {
				t.Fatal(err)
			}
			var actual, expected any
			if err := json.Unmarshal(actualJSON, &actual); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(expectedJSON, &expected); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(actual, expected) {
				t.Fatalf("Bridge command contract changed\nactual: %s\nexpected: %s", actualJSON, expectedJSON)
			}
		})
	}
}
