//go:build auth_session_dev && auth_session_fixture

package engine

import (
	"context"
	"testing"
	"time"
)

func newRealClockLifecycleController(t *testing.T) (*concurrentController, *controlledSyntheticLifecycle, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	cfg := concurrentTestConfig(ctx, newConcurrentTestClock(), false)
	cfg.now, cfg.ExpectedRevision, cfg.ExpectedAccountID = time.Now, "", "synthetic:account:fixture"
	source := newControlledSyntheticLifecycle("fixture")
	collect := func(ctx context.Context, ch evidenceChallenge) (roleEvidence, error) {
		if err := ctx.Err(); err != nil {
			return roleEvidence{}, err
		}
		return source.observe(ch, time.Now(), "fixture")
	}
	c, err := newAuthorizedConcurrentController(ctx, cfg, concurrentPorts{owners: newFakeConcurrentOwners()}, controlledFixtureAuthorization("fixture", source, collect))
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	if err = c.Start(ctx); err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() { finishAuthorizedControllerTest(t, c, cancel) })
	return c, source, cancel
}

func TestControlledLifecycleRealClockBarrierCommitsOriginalTicket(t *testing.T) {
	c, _, _ := newRealClockLifecycleController(t)
	if err := c.OpenReaderWindow(context.Background(), "synthetic:real-clock-window", "synthetic:target", true); err != nil {
		t.Fatal(err)
	}
	id, err := c.QueueWork("synthetic:real-clock")
	if err != nil {
		t.Fatal(err)
	}
	ticket, err := c.Ticket(id)
	if err != nil {
		t.Fatal(err)
	}
	if err = c.Claim(context.Background(), ticket); err != nil {
		t.Fatal(err)
	}
	// Real Windows wall-clock observations may share a coarse timestamp. A
	// renewal link must strictly advance coverage, so wait for an actual tick.
	<-time.After(20 * time.Millisecond)
	if err = c.RenewEvidence(context.Background()); err != nil {
		t.Fatal(err)
	}
	if fixtureAuthorityVersion(c) != ticket.EvidenceVersion+1 {
		t.Fatal("renewal did not advance exact evidence version")
	}
	if err = c.Complete(context.Background(), ticket, "synthetic:real-clock-result"); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.SinkResult(id); !ok {
		t.Fatal("memory sink missing")
	}
	if c.Complete(context.Background(), ticket, "synthetic:duplicate") == nil {
		t.Fatal("duplicate accepted")
	}
}

func TestControlledLifecycleRejectsGapChangeScopeSourceOwnerRevisionAndStale(t *testing.T) {
	for _, mutation := range []string{"gap", "generation", "account", "scope", "source", "owner", "revision", "stale", "missing-release"} {
		t.Run(mutation, func(t *testing.T) {
			c, source, _ := newRealClockLifecycleController(t)
			id, err := c.QueueWork("synthetic:negative")
			if err != nil {
				t.Fatal(err)
			}
			ticket, err := c.Ticket(id)
			if err != nil {
				t.Fatal(err)
			}
			if err = c.Claim(context.Background(), ticket); err != nil {
				t.Fatal(err)
			}
			a := c.authorization.(*authorizedConcurrentAdapter).authority
			source.mu.Lock()
			switch mutation {
			case "gap":
				source.gap = true
			case "generation":
				source.generation++
			case "account":
				source.account = "beta"
			}
			source.mu.Unlock()
			a.mu.Lock()
			switch mutation {
			case "scope":
				a.config.enrollment.databaseScope = "synthetic-wrong-scope"
			case "source":
				a.config.enrollment.sourceID = "synthetic-wrong-source"
			case "owner":
				a.permit.readerOwner.processID = "synthetic-wrong-process"
			case "revision":
				a.permit.readerRevision = "synthetic-wrong-revision"
			case "stale":
				a.clock = func() time.Time { return time.Now().Add(maximumEvidenceAge + time.Second) }
			case "missing-release":
				a.config.lifecycleBarrier = func(syntheticLifecycleRequest) (func(), bool) { return nil, true }
			}
			a.mu.Unlock()
			if c.Complete(context.Background(), ticket, "synthetic:forbidden") == nil {
				t.Fatal("changed lifecycle committed result")
			}
			if c.Snapshot().AdmissionOpen {
				t.Fatal("changed lifecycle retained admission")
			}
			if _, ok := c.SinkResult(id); ok {
				t.Fatal("changed lifecycle mutated sink")
			}
		})
	}
}

func TestControlledLifecycleWriterWaitsThroughCommit(t *testing.T) {
	c, source, _ := newRealClockLifecycleController(t)
	id, err := c.QueueWork("synthetic:atomic-source")
	if err != nil {
		t.Fatal(err)
	}
	ticket, err := c.Ticket(id)
	if err != nil {
		t.Fatal(err)
	}
	if err = c.Claim(context.Background(), ticket); err != nil {
		t.Fatal(err)
	}
	adapter := c.authorization.(*authorizedConcurrentAdapter)
	authority, canonical, err := adapter.resolveTicket(concurrentAuthorizationTicket{ID: ticket.authorizationTicketID, EvidenceVersion: ticket.EvidenceVersion, WorkID: ticket.WorkID})
	if err != nil {
		t.Fatal(err)
	}
	started, mutated := make(chan struct{}), make(chan struct{})
	err = authority.completeWithCommit(canonical, fixtureResultFor(canonical), func() error {
		go func() { close(started); source.mu.Lock(); source.gap = true; source.mu.Unlock(); close(mutated) }()
		<-started
		select {
		case <-mutated:
			t.Error("source writer interleaved with sink barrier")
		default:
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-mutated:
	case <-time.After(time.Second):
		t.Fatal("source writer did not drain after commit")
	}
	if authority.check() == nil {
		t.Fatal("post-commit lifecycle gap remained admitted")
	}
}
