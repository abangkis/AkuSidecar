package httpapi

import (
	"context"
	"encoding/json"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"testing"
)

func TestTargetCaptureHTTPAdmission(t *testing.T) {
	for _, tc := range []struct {
		name, target string
		status       int
	}{
		{"canonical", `{"source":"x","permalink":"https://x.com/primary/status/123456789"}`, 201},
		{"external", `{"source":"x","permalink":"https://example.com/primary/status/123456789"}`, 400},
		{"feed", `{"source":"x","permalink":"https://x.com/home"}`, 400},
		{"query", `{"source":"x","permalink":"https://x.com/primary/status/123456789?x=1"}`, 400},
		{"unsupported", `{"source":"linkedin","permalink":"https://www.linkedin.com/feed/update/urn:li:activity:12345"}`, 400},
		{"missing_source", `{"permalink":"https://x.com/primary/status/123456789"}`, 400},
		{"unknown_field", `{"source":"x","permalink":"https://x.com/primary/status/123456789","override":true}`, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newSessionUpdatesFixture(t)
			ctx := context.Background()
			if _, err := f.s.store.CompleteOnboarding(ctx, []domain.Source{domain.SourceX}); err != nil {
				t.Fatal(err)
			}
			body := `{"intent":"offline one post","captureTarget":` + tc.target + `}`
			payload := f.json(t, f.serve(sessionUpdatesRequest("POST", "/api/updates", body), true), tc.status)
			if tc.status != 201 {
				if f.count(t, `SELECT COUNT(*) FROM sessions`) != 0 {
					t.Fatal("invalid target created session")
				}
				return
			}
			var session domain.Session
			if err := json.Unmarshal(payload["session"], &session); err != nil {
				t.Fatal(err)
			}
			target, err := domain.SessionCaptureTarget(session)
			if err != nil || target == nil || len(session.Runs) != 1 || session.Runs[0].Source != domain.SourceX || session.MaxItemsTotal != 1 {
				t.Fatalf("session %+v %v", session, err)
			}
		})
	}
}
