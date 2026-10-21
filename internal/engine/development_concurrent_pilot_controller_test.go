//go:build auth_session_dev && auth_session_fixture

package engine

import (
	"context"
	"errors"
	"testing"
	"time"
)

func pilotControllerTest(t *testing.T) (*concurrentController, *concurrentTestClock, *pilotConcurrentAdapter) {
	t.Helper()
	parent, cancel := context.WithCancel(context.Background())
	clock := newConcurrentTestClock()
	cfg := concurrentTestConfig(parent, clock, false)
	sourceClock := &pilotTestClock{now: clock.Now()}
	f := pilotConcurrentFixture{ExplicitOptIn: true, LimitationAcknowledged: true, PolicyEnabled: true, ExpectedAccountID: cfg.ExpectedAccountID, ScopeID: "synthetic-pilot-scope", PolicyID: concurrentPilotPolicy, PolicyGeneration: 4, ConsentGeneration: 9, Grant: pilotGrantForTest()}
	f.Source = func(ctx context.Context, c pilotChallenge) (pilotRoleEvidence, error) {
		sourceClock.Set(clock.Now())
		return pilotEvidenceForChallenge(sourceClock, c), nil
	}
	c, e := newPilotConcurrentController(parent, cfg, concurrentPorts{owners: newFakeConcurrentOwners()}, f)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		cancel()
		ctx, done := context.WithTimeout(context.Background(), time.Second)
		defer done()
		_ = c.WaitParentCleanup(ctx)
	})
	if e = c.Start(context.Background()); e != nil {
		t.Fatal(e)
	}
	if e = c.OpenReaderWindow(context.Background(), "synthetic:reader:main", "synthetic:target:x", false); e != nil {
		t.Fatal(e)
	}
	return c, clock, c.authorization.(*pilotConcurrentAdapter)
}

func TestConcurrentPilotControllerRenewalTicketAndAtomicCommit(t *testing.T) {
	c, clock, a := pilotControllerTest(t)
	id, e := c.QueueWork("synthetic:content")
	if e != nil {
		t.Fatal(e)
	}
	ticket, e := c.Ticket(id)
	if e != nil {
		t.Fatal(e)
	}
	m := a.model
	pid := m.collector.processID
	ownerBefore := *c.collector
	raw := a.byWork[id]
	fail := errors.New("local commit failed")
	if e = a.Claim(raw, func() error { return fail }); !errors.Is(e, fail) {
		t.Fatal(e)
	}
	if s, e := m.ticketStatus(a.tickets[raw.ID]); e != nil || s != pilotWorkPending {
		t.Fatal(s, e)
	}
	if e = c.Claim(context.Background(), ticket); e != nil {
		t.Fatal(e)
	}
	if e = c.OpenReaderWindow(context.Background(), "synthetic:extra", "synthetic:target:x", false); e != nil {
		t.Fatal(e)
	}
	clock.Advance(time.Second)
	if e = c.RenewEvidence(context.Background()); e != nil {
		t.Fatal(e)
	}
	if m.collector.processID != pid || !sameOwner(ownerBefore, *c.collector) || c.work[id].state != concurrentWorkActive || m.version <= ticket.EvidenceVersion {
		t.Fatal("reader open or renewal changed active collection")
	}
	if e = a.Complete(raw, "synthetic:fixture", func() error { return fail }); !errors.Is(e, fail) {
		t.Fatal(e)
	}
	if m.resultCount() != 0 {
		t.Fatal("partial result commit")
	}
	if e = c.Complete(context.Background(), ticket, "synthetic:fixture"); e != nil {
		t.Fatal(e)
	}
	if e = c.Complete(context.Background(), ticket, "synthetic:duplicate"); e == nil {
		t.Fatal("duplicate accepted")
	}
	report := m.report()
	if report.CanAuthorize || report.NativeQualified || report.ProductionQualified || report.StorePermit {
		t.Fatal("synthetic pilot acquired native or Store authority")
	}
}

func TestConcurrentPilotControllerValidationFailureAndFences(t *testing.T) {
	for _, transition := range []string{"validate", "expiry", "policy", "owner", "channel", "optout", "last-reader"} {
		t.Run(transition, func(t *testing.T) {
			c, clock, a := pilotControllerTest(t)
			id, _ := c.QueueWork("synthetic:fixture")
			ticket, e := c.Ticket(id)
			if e != nil {
				t.Fatal(e)
			}
			if e = c.Claim(context.Background(), ticket); e != nil {
				t.Fatal(e)
			}
			version := a.model.version
			switch transition {
			case "validate":
				e = a.RenewEvidence(context.Background(), a.binding, func(context.Context) error { return errors.New("binding lost") })
				if e == nil || a.model.version != version {
					t.Fatal("published failed validation")
				}
			case "expiry":
				clock.Advance(5 * time.Second)
			case "policy":
				c.PolicyChanged(5, 9, true)
			case "owner":
				c.OwnerChanged(concurrentCollector)
			case "channel":
				c.Revoke(string(pilotReasonChannelLost))
			case "optout":
				if e = c.Disable(context.Background()); e != nil {
					t.Fatal(e)
				}
				if c.readerWindowCount() == 0 {
					t.Fatal("optout removed reader")
				}
			case "last-reader":
				if e = a.UpdateReaderWindows(nil); e == nil {
					t.Fatal("last-reader accepted")
				}
			}
			if e = c.Complete(context.Background(), ticket, "synthetic:late"); e == nil {
				t.Fatal("late result accepted")
			}
			if _, ok := c.SinkResult(id); ok {
				t.Fatal("late sink write")
			}
		})
	}
}

func TestConcurrentPilotControllerAdmissionAndCancellation(t *testing.T) {
	parent := context.Background()
	clock := newConcurrentTestClock()
	cfg := concurrentTestConfig(parent, clock, false)
	cfg.Enabled = false
	if _, e := newPilotConcurrentController(parent, cfg, concurrentPorts{}, pilotConcurrentFixture{}); e != nil {
		t.Fatal(e)
	}
	cfg.Enabled = true
	if _, e := newPilotConcurrentController(parent, cfg, concurrentPorts{}, pilotConcurrentFixture{}); e == nil {
		t.Fatal("unapproved pilot admitted")
	}
	c, _, a := pilotControllerTest(t)
	id, _ := c.QueueWork("synthetic:fixture")
	ticket, e := c.Ticket(id)
	if e != nil {
		t.Fatal(e)
	}
	raw := a.byWork[id]
	bad := raw
	bad.WorkID += "other"
	if e = a.Claim(bad, func() error { return nil }); e == nil {
		t.Fatal("cross-work accepted")
	}
	if e = c.Cancel(ticket); e != nil {
		t.Fatal(e)
	}
	if e = c.Claim(context.Background(), ticket); e == nil {
		t.Fatal("cancelled claim accepted")
	}
	if e = a.RequestIndependentCollector(); e == nil {
		t.Fatal("independent collector accepted")
	}
}

func TestConcurrentPilotControllerCancelCommitIdempotent(t *testing.T) {
	c, _, a := pilotControllerTest(t)
	id, _ := c.QueueWork("synthetic:cancel")
	_, e := c.Ticket(id)
	if e != nil {
		t.Fatal(e)
	}
	raw := a.byWork[id]
	calls := 0
	commit := func() error { calls++; return nil }
	fail := errors.New("cancel commit failed")
	if e = a.Cancel(raw, func() error { return fail }); !errors.Is(e, fail) {
		t.Fatal(e)
	}
	if status, err := a.model.ticketStatus(a.tickets[raw.ID]); err != nil || status != pilotWorkPending {
		t.Fatal("failed cancellation mutated pilot ticket")
	}
	if e = a.Cancel(raw, commit); e != nil {
		t.Fatal(e)
	}
	if e = a.Cancel(raw, commit); e != nil {
		t.Fatal(e)
	}
	if calls != 1 {
		t.Fatal(calls)
	}
}

func TestConcurrentPilotControllerMissingOrMismatchedInitialProof(t *testing.T) {
	for _, kind := range []string{"missing-identity", "account-mismatch"} {
		t.Run(kind, func(t *testing.T) {
			parent, cancel := context.WithCancel(context.Background())
			clock := newConcurrentTestClock()
			cfg := concurrentTestConfig(parent, clock, false)
			f := pilotConcurrentFixture{ExplicitOptIn: true, LimitationAcknowledged: true, PolicyEnabled: true,
				ExpectedAccountID: cfg.ExpectedAccountID, ScopeID: "synthetic-pilot-scope", PolicyID: concurrentPilotPolicy,
				PolicyGeneration: 4, ConsentGeneration: 9, Grant: pilotGrantForTest()}
			f.Source = func(_ context.Context, challenge pilotChallenge) (pilotRoleEvidence, error) {
				evidence := pilotEvidenceForChallenge(&pilotTestClock{now: clock.Now()}, challenge)
				if kind == "missing-identity" {
					evidence.identityPresent = false
				} else {
					evidence.accountID = "synthetic-other-account"
				}
				return evidence, nil
			}
			c, err := newPilotConcurrentController(parent, cfg, concurrentPorts{owners: newFakeConcurrentOwners()}, f)
			if err != nil {
				cancel()
				t.Fatal(err)
			}
			t.Cleanup(func() {
				cancel()
				ctx, done := context.WithTimeout(context.Background(), time.Second)
				defer done()
				_ = c.WaitParentCleanup(ctx)
			})
			if err := c.Start(context.Background()); err == nil || c.Snapshot().AdmissionOpen {
				t.Fatal("missing or mismatched identity admitted")
			}
		})
	}
}
