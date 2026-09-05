package livingtopics

import (
	"strings"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

// Excerpts from locally captured posts, reviewed on 2026-09-05. These fixtures
// replay adversarial model outputs through the host, not a live model benchmark.
func TestLifecycleProofLocalCodexResetSources(t *testing.T) {
	const rollout = "Astra is now rolled out to all Plus and Business users too."
	const promise = "we will do the full banked reset today too for all Plus, Pro and Business users. Lands end of day."
	const schedule = "Your Codex and ChatGPT Work reset will land at 6pm PST."
	for _, tc := range []struct {
		name, source, subject, assertion string
		wantStatus                       string
		retained                         bool
	}{
		{"rollout completion", rollout, "Astra", "Astra has rolled out.", "completed", true},
		{"reset promise is not delivery", promise, "banked reset", "The banked reset completed.", "unknown", true},
		{"historical schedule is not completion", schedule, "reset", "The reset completed.", "unknown", true},
		{"summary cannot prove completion", rollout, "Astra", "Astra has rolled out.", "unknown", false},
		{"rollout cannot prove reset", rollout, "banked reset", "The banked reset completed.", "unknown", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			item := domain.MemoryItem{ID: "local-source", Source: domain.SourceX, Title: tc.assertion, Summary: tc.source}
			if tc.retained {
				item.FullContent = stringPointer(tc.source)
			}
			claim := terminalClaim(tc.assertion, tc.subject, tc.source)
			result := resolveLifecycleFixture(t, structuredResult{
				Status: "ready", Overview: tc.assertion, Claims: []structuredClaim{claim}, CoverageState: "focused",
				EvidenceRoles: []structuredEvidenceRole{{EvidenceAlias: "evidence_001", Role: "core", Subtopic: "reset lifecycle", SourceCluster: "local-source", EpistemicClass: "primary"}},
			}, []domain.MemoryItem{item})
			if len(result.Claims) != 1 || result.Claims[0].EventStatus != tc.wantStatus {
				t.Fatalf("source-backed status mismatch: %+v", result)
			}
			if tc.wantStatus == "unknown" && (result.Claims[0].Assessment != "uncertain" || result.Claims[0].Text == tc.assertion || result.Overview == tc.assertion) {
				t.Fatalf("unsupported terminal assertion survived: %+v", result)
			}
			if tc.wantStatus == "completed" && !strings.HasPrefix(result.Claims[0].Text, "Source statement: \"") {
				t.Fatalf("source-relative language must remain an attributed quotation: %+v", result.Claims[0])
			}
		})
	}
}

func TestLifecycleProofRejectsAmbiguousSourceForms(t *testing.T) {
	for _, tc := range []struct{ source, quote, status string }{
		{"Astra termination policy is documented.", "Astra termination policy is documented.", "cancelled"},
		{"Has Astra rolled out?", "Has Astra rolled out?", "completed"},
		{"Astra isn’t completed.", "Astra isn’t completed.", "completed"},
		{"Astra completed.", "Astra", "completed"},
	} {
		t.Run(tc.source, func(t *testing.T) {
			claim := terminalClaim("Astra completed.", "Astra", tc.quote)
			claim.EventStatus = tc.status
			if validTerminalProof(claim, []string{"source"}, map[string]string{"evidence_001": "source"}, map[string]string{"evidence_001": tc.source}) {
				t.Fatalf("ambiguous quote admitted as %s: %s", tc.status, tc.source)
			}
		})
	}
}
