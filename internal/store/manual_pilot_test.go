//go:build auth_session_dev

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

type manualTestClock struct{ now time.Time }

func TestManualPilotSyntheticShorterOriginalDeadline(t *testing.T) {
	ctx := context.Background()
	for _, limit := range []time.Duration{3 * time.Second, 61 * time.Second, -time.Second} {
		t.Run(limit.String(), func(t *testing.T) {
			s := openTestStore(t)
			clock := &manualTestClock{time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC)}
			s.clock = clock
			settings, err := s.GetSettings(ctx)
			if err != nil {
				t.Fatal(err)
			}
			settings.CollectionMode = "headless"
			settings.ActiveSources = []domain.Source{domain.SourceX}
			session, err := createVisibleUpdateSession(s, ctx, "synthetic shorter deadline", settings)
			if err != nil {
				t.Fatal(err)
			}
			command, err := s.StartRun(ctx, session.Runs[0].ID, map[string]any{"captureRuntime": map[string]any{"driver": "headless"}})
			if err != nil {
				t.Fatal(err)
			}
			p, err := s.IssueSyntheticManualPilot(ctx, ManualPilotSyntheticRequest{FactoryOptIn: true, SessionID: session.ID, RunID: command.RunID, SyntheticOwnerRef: "synthetic:owner", SyntheticPairRef: "synthetic:pair", StartedAt: clock.now, ValidUntil: clock.now.Add(limit)})
			if limit != 3*time.Second {
				if err == nil || p != nil {
					t.Fatal("invalid deadline issued")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			claim, err := s.ClaimManualPilot(ctx, p, command.ID, "short-claim")
			if err != nil || claim.Handle == nil {
				t.Fatal(err)
			}
			clock.now = clock.now.Add(3 * time.Second)
			outcome, err := s.CommitManualPilotResult(ctx, claim.Handle, "short-result", manualObservation())
			if err == nil || outcome != RejectedNoMutation {
				t.Fatal("shorter limit was reset")
			}
			assertManualCounts(t, s, 0)
		})
	}
}

func (c *manualTestClock) Now() time.Time { return c.now }

type manualFixture struct {
	s       *Store
	p       *ManualPilotPermit
	c       domain.BridgeCommand
	clock   *manualTestClock
	session domain.Session
}

func newManualFixture(t *testing.T) manualFixture {
	t.Helper()
	ctx := context.Background()
	s := openTestStore(t)
	clock := &manualTestClock{time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC)}
	s.clock = clock
	settings, _ := s.GetSettings(ctx)
	settings.CollectionMode = "headless"
	settings.ActiveSources = []domain.Source{domain.SourceX, domain.SourceLinkedIn}
	session, e := createVisibleUpdateSession(s, ctx, "synthetic manual pilot", settings)
	if e != nil {
		t.Fatal(e)
	}
	c, e := s.StartRun(ctx, session.Runs[0].ID, map[string]any{"captureRuntime": map[string]any{"driver": "headless"}})
	if e != nil {
		t.Fatal(e)
	}
	p, e := s.IssueSyntheticManualPilot(ctx, ManualPilotSyntheticRequest{FactoryOptIn: true, StartedAt: s.Now(), SessionID: session.ID, RunID: c.RunID, SyntheticOwnerRef: "synthetic:owner", SyntheticPairRef: "synthetic:pair"})
	if e != nil {
		t.Fatal(e)
	}
	return manualFixture{s, p, c, clock, session}
}
func claimManual(t *testing.T, f manualFixture) *ManualPilotClaim {
	t.Helper()
	r, e := f.s.ClaimManualPilot(context.Background(), f.p, f.c.ID, "claim-1")
	if e != nil || r.Outcome != Committed || r.Handle == nil {
		t.Fatalf("claim: %+v %v", r, e)
	}
	return r.Handle
}
func manualObservation() domain.Observation {
	return domain.Observation{Source: domain.SourceX, Coverage: map[string]any{"synthetic": true}}
}
func assertManualCounts(t *testing.T, s *Store, observations int) {
	t.Helper()
	var n int
	if e := s.db.QueryRow(`SELECT COUNT(*) FROM observations`).Scan(&n); e != nil || n != observations {
		t.Fatalf("observations=%d err=%v", n, e)
	}
}

func TestManualPilotAtomicResultReplayAndFollowUp(t *testing.T) {
	f := newManualFixture(t)
	ctx := context.Background()
	h := claimManual(t, f)
	obs := manualObservation()
	o, e := f.s.CommitManualPilotResult(ctx, h, "result-1", obs)
	if e != nil || o != Committed {
		t.Fatalf("result %s %v", o, e)
	}
	if _, ok := obs.Coverage["contentIdentity"]; ok {
		t.Fatal("caller observation mutated")
	}
	f.clock.now = f.clock.now.Add(time.Minute)
	o, e = f.s.CommitManualPilotResult(ctx, h, "result-1", obs)
	if e != nil || o != AlreadyCommitted {
		t.Fatalf("expired replay %s %v", o, e)
	}
	obs.Coverage["different"] = true
	o, e = f.s.CommitManualPilotResult(ctx, h, "result-1", obs)
	if e == nil || o != RejectedNoMutation {
		t.Fatalf("conflict %s %v", o, e)
	}
	assertManualCounts(t, f.s, 1)
	receipt, e := f.s.LookupManualPilotOp(ctx, "result-1")
	if e != nil || receipt.DecisionAt == "" {
		t.Fatalf("receipt %+v %v", receipt, e)
	}
}

func TestManualPilotFollowUpFreshClaimAndSessionFence(t *testing.T) {
	f := newManualFixture(t)
	ctx := context.Background()
	h := claimManual(t, f)
	if o, e := f.s.CommitManualPilotResult(ctx, h, "result-1", manualObservation()); e != nil || o != Committed {
		t.Fatal(o, e)
	}
	c, e := f.s.QueueFollowUp(ctx, f.c.RunID, map[string]any{"captureRuntime": map[string]any{"driver": "headless"}})
	if e != nil {
		t.Fatal(e)
	}
	if v, e := f.s.ClaimCommand(ctx, c.RunID, "generic"); e == nil || v != nil {
		t.Fatal("generic follow-up bypass")
	}
	r, e := f.s.ClaimManualPilot(ctx, f.p, c.ID, "claim-2")
	if e != nil || r.Handle == nil {
		t.Fatal(r, e)
	}
	if o, e := f.s.CommitManualPilotResult(ctx, r.Handle, "result-2", manualObservation()); e != nil || o != Committed {
		t.Fatal(o, e)
	}
	other, e := f.s.StartRun(ctx, f.session.Runs[1].ID, map[string]any{"captureRuntime": map[string]any{"driver": "headless"}})
	if e != nil {
		t.Fatal(e)
	}
	if v, e := f.s.ClaimCommand(ctx, other.RunID, "generic"); e == nil || v != nil {
		t.Fatal("other run generic bypass")
	}
	if r, e := f.s.ClaimManualPilot(ctx, f.p, other.ID, "claim-other"); e == nil || r.Handle != nil {
		t.Fatal("other run private bypass")
	}
	assertManualCounts(t, f.s, 2)
}

func TestManualPilotRevocationExpiryAndCancel(t *testing.T) {
	for _, stage := range []string{"before-claim", "after-claim", "precommit-claim-expiry", "precommit-result-expiry", "precommit-result-revoke", "cancel", "source-change", "owner-change", "incarnation-change"} {
		t.Run(stage, func(t *testing.T) {
			f := newManualFixture(t)
			ctx := context.Background()
			if stage == "before-claim" {
				if o, e := f.s.RevokeManualPilot(ctx, f.p, "revoke"); e != nil || o != Committed {
					t.Fatal(o, e)
				}
				r, e := f.s.ClaimManualPilot(ctx, f.p, f.c.ID, "claim-1")
				if e == nil || r.Handle != nil {
					t.Fatal("revoked claim")
				}
				return
			}
			if stage == "precommit-claim-expiry" {
				f.s.manualPilotBeforeCommit = func() { f.clock.now = f.clock.now.Add(time.Minute) }
				r, e := f.s.ClaimManualPilot(ctx, f.p, f.c.ID, "claim-1")
				if e == nil || r.Handle != nil {
					t.Fatal("expired claim")
				}
				var status string
				_ = f.s.db.QueryRow(`SELECT status FROM bridge_commands WHERE id=?`, f.c.ID).Scan(&status)
				if status != "queued" {
					t.Fatal(status)
				}
				return
			}
			h := claimManual(t, f)
			switch stage {
			case "after-claim":
				_, _ = f.s.RevokeManualPilot(ctx, f.p, "revoke")
			case "precommit-result-expiry":
				f.s.manualPilotBeforeCommit = func() { f.clock.now = f.clock.now.Add(time.Minute) }
			case "precommit-result-revoke":
				f.s.manualPilotBeforeCommit = func() { f.p.revoked.Store(true) }
			case "cancel":
				if e := f.s.CancelSession(ctx, f.session.ID); e != nil {
					t.Fatal(e)
				}
			case "source-change":
				_, _ = f.s.db.Exec(`UPDATE runs SET source='facebook' WHERE id=?`, f.c.RunID)
			case "owner-change":
				_, _ = f.s.db.Exec(`UPDATE bridge_commands SET claimed_by='other' WHERE id=?`, f.c.ID)
			case "incarnation-change":
				_, _ = f.s.db.Exec(`UPDATE meta SET value='other' WHERE key='manual_pilot_incarnation_v1'`)
			}
			o, e := f.s.CommitManualPilotResult(ctx, h, "result-1", manualObservation())
			if e == nil || o != RejectedNoMutation {
				t.Fatalf("accepted %s %v", o, e)
			}
			assertManualCounts(t, f.s, 0)
		})
	}
}

func TestManualPilotGenericGuardsAndOpaqueAuthority(t *testing.T) {
	f := newManualFixture(t)
	ctx := context.Background()
	if c, e := f.s.ClaimCommandForCollector(ctx, f.c.RunID, "generic", "headless"); e == nil || c != nil {
		t.Fatal("generic claim")
	}
	h := claimManual(t, f)
	if e := f.s.SaveObservation(ctx, f.c.ID, f.c.RunID, manualObservation()); e == nil {
		t.Fatal("generic save")
	}
	if _, e := f.s.FailCommand(ctx, f.c.ID, f.c.RunID, domain.Failure{}); e == nil {
		t.Fatal("generic failure")
	}
	if e := f.s.FailRun(ctx, f.c.RunID, domain.Failure{}); e == nil {
		t.Fatal("generic run failure")
	}
	if e := f.s.FailQueuedRuns(ctx, f.session.ID, domain.Failure{}); e == nil {
		t.Fatal("generic queued failure")
	}
	expired, e := f.s.ExpiredBridgeCommands(ctx, f.clock.now.Add(time.Hour))
	if e != nil || len(expired) != 0 {
		t.Fatal("generic recovery", expired, e)
	}
	if _, e := json.Marshal(f.p); e == nil {
		t.Fatal("permit marshaled")
	}
	if _, e := json.Marshal(h); e == nil {
		t.Fatal("claim marshaled")
	}
	if fmt.Sprintf("%+v", f.p) != "[manual pilot permit redacted]" {
		t.Fatal("permit not redacted")
	}
	copied := *f.s
	if r, e := copied.ClaimManualPilot(ctx, f.p, f.c.ID, "copied"); e == nil || r.Handle != nil {
		t.Fatal("copied store accepted")
	}
	reopened, e := Open(f.s.path, domain.DefaultSettings("expanded", "quiet", "promote_unused_budget", true))
	if e != nil {
		t.Fatal(e)
	}
	defer reopened.Close()
	if o, e := reopened.CommitManualPilotResult(ctx, h, "result-1", manualObservation()); e == nil || o != RejectedNoMutation {
		t.Fatal("reopened accepted")
	}
	if c, e := reopened.ClaimCommand(ctx, f.c.RunID, "generic"); e == nil || c != nil {
		t.Fatal("reopened generic bypass")
	}
	assertManualCounts(t, f.s, 0)
}

func TestManualPilotCrossStoreCancellationOrders(t *testing.T) {
	for _, order := range []string{"cancel-first", "result-first", "concurrent-writer"} {
		t.Run(order, func(t *testing.T) {
			f := newManualFixture(t)
			ctx := context.Background()
			h := claimManual(t, f)
			other, e := Open(f.s.path, domain.DefaultSettings("expanded", "quiet", "promote_unused_budget", true))
			if e != nil {
				t.Fatal(e)
			}
			defer other.Close()
			if order == "cancel-first" {
				if e = other.CancelSession(ctx, f.session.ID); e != nil {
					t.Fatal(e)
				}
				o, e := f.s.CommitManualPilotResult(ctx, h, "result", manualObservation())
				if e == nil || o != RejectedNoMutation {
					t.Fatal(o, e)
				}
				assertManualCounts(t, f.s, 0)
				return
			}
			var done chan error
			if order == "concurrent-writer" {
				done = make(chan error, 1)
				f.s.manualPilotBeforeCommit = func() {
					started := make(chan struct{})
					go func() { close(started); done <- other.CancelSession(ctx, f.session.ID) }()
					<-started
				}
			}
			o, e := f.s.CommitManualPilotResult(ctx, h, "result", manualObservation())
			if e != nil || o != Committed {
				t.Fatal(o, e)
			}
			if done != nil {
				// A competing SQLite writer may lose its snapshot rather than wait. Its
				// caller must retry cancellation, never reinterpret this as permission.
				select {
				case e = <-done:
				case <-time.After(5 * time.Second):
					t.Fatal("competing writer stuck")
				}
				if e != nil {
					e = other.CancelSession(ctx, f.session.ID)
				}
			} else {
				e = other.CancelSession(ctx, f.session.ID)
			}
			if e != nil {
				t.Fatal(e)
			}
			assertManualCounts(t, f.s, 1)
			v, err := f.s.LookupManualPilotOp(ctx, "result")
			if err != nil || v.Outcome != Committed {
				t.Fatal(v, err)
			}
			if r, e := f.s.ClaimManualPilot(ctx, f.p, f.c.ID, "new-claim"); e == nil || r.Handle != nil {
				t.Fatal("cancelled scope admitted")
			}
		})
	}
}

func TestManualPilotIssuerRejectsUnapprovedPlan(t *testing.T) {
	for _, mode := range []string{"browser", "wrong-source", "no-opt-in"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			s := openTestStore(t)
			settings, _ := s.GetSettings(ctx)
			settings.CollectionMode = "headless"
			settings.ActiveSources = []domain.Source{domain.SourceX}
			if mode == "browser" {
				settings.CollectionMode = "browser"
			}
			if mode == "wrong-source" {
				settings.ActiveSources = []domain.Source{domain.SourceLinkedIn}
			}
			session, e := createVisibleUpdateSession(s, ctx, "synthetic", settings)
			if e != nil {
				t.Fatal(e)
			}
			_, e = s.IssueSyntheticManualPilot(ctx, ManualPilotSyntheticRequest{FactoryOptIn: mode != "no-opt-in", StartedAt: s.Now(), SessionID: session.ID, RunID: session.Runs[0].ID, SyntheticOwnerRef: "synthetic:owner", SyntheticPairRef: "synthetic:pair"})
			if e == nil {
				t.Fatal("issuer accepted invalid plan")
			}
			var n int
			_ = s.db.QueryRow(`SELECT COUNT(*) FROM meta WHERE key LIKE 'manual_pilot_%'`).Scan(&n)
			if n != 0 {
				t.Fatal("rejected issuer wrote state", n)
			}
		})
	}
}

func TestManualPilotOriginalStartDeadline(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	clock := &manualTestClock{time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC)}
	s.clock = clock
	settings, _ := s.GetSettings(ctx)
	settings.CollectionMode = "headless"
	settings.ActiveSources = []domain.Source{domain.SourceX}
	session, e := createVisibleUpdateSession(s, ctx, "synthetic delayed issuer", settings)
	if e != nil {
		t.Fatal(e)
	}
	c, e := s.StartRun(ctx, session.Runs[0].ID, map[string]any{"captureRuntime": map[string]any{"driver": "headless"}})
	if e != nil {
		t.Fatal(e)
	}
	start := clock.now
	clock.now = clock.now.Add(50 * time.Second)
	request := ManualPilotSyntheticRequest{FactoryOptIn: true, StartedAt: start, SessionID: session.ID, RunID: c.RunID, SyntheticOwnerRef: "synthetic:owner", SyntheticPairRef: "synthetic:pair"}
	p, e := s.IssueSyntheticManualPilot(ctx, request)
	if e != nil {
		t.Fatal(e)
	}
	clock.now = start.Add(60 * time.Second)
	if r, e := s.ClaimManualPilot(ctx, p, c.ID, "claim"); e == nil || r.Handle != nil {
		t.Fatal("issuance enlarged original deadline")
	}
}

func TestManualPilotCommitAmbiguity(t *testing.T) {
	for _, mode := range []string{"claim-present", "result-present", "result-absent", "result-unavailable"} {
		t.Run(mode, func(t *testing.T) {
			f := newManualFixture(t)
			ctx := context.Background()
			var h *ManualPilotClaim
			if mode != "claim-present" {
				h = claimManual(t, f)
			}
			f.s.manualPilotCommit = func(tx *sql.Tx) error {
				if mode == "claim-present" || mode == "result-present" {
					if e := tx.Commit(); e != nil {
						return e
					}
				} else {
					_ = tx.Rollback()
				}
				if mode == "result-unavailable" {
					_ = f.s.db.Close()
				}
				return errors.New("synthetic commit response lost")
			}
			if mode == "claim-present" {
				r, e := f.s.ClaimManualPilot(ctx, f.p, f.c.ID, "claim-1")
				if e != nil || r.Outcome != AlreadyCommitted || r.Handle == nil {
					t.Fatal(r, e)
				}
				return
			}
			o, e := f.s.CommitManualPilotResult(ctx, h, "result-1", manualObservation())
			if mode == "result-present" {
				if e != nil || o != AlreadyCommitted {
					t.Fatal(o, e)
				}
				assertManualCounts(t, f.s, 1)
			} else if e == nil || o != Indeterminate {
				t.Fatal(o, e)
			}
		})
	}
}

func TestManualPilotIssuerIncompatibilityAndClosedRestart(t *testing.T) {
	f := newManualFixture(t)
	ctx := context.Background()
	if _, e := f.s.IssueSyntheticManualPilot(ctx, ManualPilotSyntheticRequest{FactoryOptIn: true, StartedAt: f.s.Now(), SessionID: f.session.ID, RunID: f.c.RunID, SyntheticOwnerRef: "native:owner", SyntheticPairRef: "synthetic:pair"}); e == nil {
		t.Fatal("native issuer")
	}
	other := openTestStore(t)
	_, e := other.db.Exec(`INSERT INTO meta(key,value) VALUES('synthetic_account_checkpoint','unchanged')`)
	if e != nil {
		t.Fatal(e)
	}
	_, e = other.IssueSyntheticManualPilot(ctx, ManualPilotSyntheticRequest{FactoryOptIn: true, StartedAt: other.Now(), SessionID: "synthetic-session", RunID: "synthetic-run", SyntheticOwnerRef: "synthetic:owner", SyntheticPairRef: "synthetic:pair"})
	if e == nil {
		t.Fatal("checkpoint overwritten")
	}
	var raw string
	_ = other.db.QueryRow(`SELECT value FROM meta WHERE key='synthetic_account_checkpoint'`).Scan(&raw)
	if raw != "unchanged" {
		t.Fatal(raw)
	}

}
