//go:build windows && auth_session_dev && auth_session_fixture

package engine

import (
	"database/sql"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/store"
	"strings"
	"testing"
)

func TestDevelopmentLiveReaderPreparedUnclaimedQueue(t *testing.T) {
	started := "synthetic-start"
	run := domain.Run{ID: "original-run", SessionID: "original-session", Ordinal: 1, Status: "waiting_for_bridge", Stage: "capture", BridgeCommandStatus: "queued", StartedAt: &started}
	commands := []store.FixtureCommandEvidence{{ID: "original-command", RunID: run.ID, Type: "collect_visible", Status: "queued", Collector: "headless", Driver: "headless", PayloadSHA256: strings.Repeat("a", 64)}}
	if err := ValidateDevelopmentLiveReaderQueue(run, commands); err != nil {
		t.Fatal("prepared but unclaimed work rejected", err)
	}
	for _, test := range []struct {
		name   string
		mutate func(*domain.Run, *[]store.FixtureCommandEvidence)
	}{
		{"no_actual_command", func(r *domain.Run, c *[]store.FixtureCommandEvidence) { *c = nil }},
		{"unstarted_run", func(r *domain.Run, c *[]store.FixtureCommandEvidence) { r.Status = "queued"; r.StartedAt = nil }},
		{"claimed", func(r *domain.Run, c *[]store.FixtureCommandEvidence) { (*c)[0].Status = "claimed" }},
		{"old_claim_metadata", func(r *domain.Run, c *[]store.FixtureCommandEvidence) {
			(*c)[0].ClaimedAt = sql.NullString{Valid: true, String: ""}
		}},
		{"foreign_run", func(r *domain.Run, c *[]store.FixtureCommandEvidence) { (*c)[0].RunID = "replacement" }},
		{"wrong_collector", func(r *domain.Run, c *[]store.FixtureCommandEvidence) { (*c)[0].Collector = "bridge" }},
		{"duplicate_command", func(r *domain.Run, c *[]store.FixtureCommandEvidence) { *c = append(*c, (*c)[0]) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := run
			c := append([]store.FixtureCommandEvidence(nil), commands...)
			test.mutate(&r, &c)
			if ValidateDevelopmentLiveReaderQueue(r, c) == nil {
				t.Fatal("invalid queue accepted")
			}
		})
	}
}
