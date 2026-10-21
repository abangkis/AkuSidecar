//go:build auth_session_dev && auth_session_fixture

package engine

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

const (
	authorizedReaderRevision    = "synthetic:authorized-reader-revision"
	authorizedCollectorRevision = "synthetic:authorized-collector-revision"
)

func authorizedFixtureForController(cfg concurrentConfig) authorizedConcurrentFixture {
	return authorizedConcurrentFixture{
		DatabaseScope: "synthetic:database:profile-a", AccountID: cfg.ExpectedAccountID,
		ReaderRevision: authorizedReaderRevision, CollectorRevision: authorizedCollectorRevision,
		PolicyGeneration: 4, HoldGeneration: 9, PolicyEnabled: true,
		Grant: fixtureGrantForTest(),
	}
}

func newAuthorizedControllerForTest(t *testing.T, collectorFactory func(*concurrentTestClock) evidenceCollector) (*concurrentController, context.Context, context.CancelFunc, *concurrentTestClock, *fakeConcurrentOwnerPort) {
	t.Helper()
	parent, cancel := context.WithCancel(context.Background())
	clock := newConcurrentTestClock()
	owners := newFakeConcurrentOwners()
	cfg := concurrentTestConfig(parent, clock, false)
	cfg.ExpectedRevision = "" // The selected authority pins the two role revisions separately.
	fixture := authorizedFixtureForController(cfg)
	if collectorFactory == nil {
		fixture.Collect = authorizedControllerEvidence(clock, fixture.Grant, nil)
	} else {
		fixture.Collect = collectorFactory(clock)
	}
	controller, err := newAuthorizedConcurrentController(parent, cfg, concurrentPorts{owners: owners}, fixture)
	if err != nil {
		cancel()
		t.Fatalf("newAuthorizedConcurrentController: %v", err)
	}
	return controller, parent, cancel, clock, owners
}

func authorizedControllerEvidence(clock *concurrentTestClock, grant fixtureGrant, mutate func(evidenceChallenge, *roleEvidence)) evidenceCollector {
	return func(ctx context.Context, challenge evidenceChallenge) (roleEvidence, error) {
		if err := ctx.Err(); err != nil {
			return roleEvidence{}, err
		}
		observed := clock.Now()
		coverageFrom := challenge.previousCoverage
		if challenge.previousVersion == 0 {
			coverageFrom = challenge.acquisitionStarted
		}
		evidence := roleEvidence{
			instanceID: challenge.instanceID, challengeID: challenge.id, vectorID: challenge.vectorID,
			contractID: challenge.contractID, providerID: challenge.providerID, providerVersion: challenge.providerVersion,
			evidenceSchema: challenge.evidenceSchema, sourceID: challenge.sourceID, origin: challenge.origin,
			operation: challenge.operation, databaseScope: challenge.databaseScope, accountID: challenge.accountID,
			pairID: challenge.pairID, attemptID: challenge.attemptID, pairGeneration: challenge.pairGeneration,
			role: challenge.role, owner: cloneOwner(challenge.owner), frameID: challenge.frameID, requestID: challenge.requestID,
			fixtureGrantID: grant.id, provenance: fixtureProvenance, authentication: fixtureAuthentication,
			identity: fixtureIdentity, cacheState: fixtureCacheFresh, authenticated: true, continuityKnown: true,
			coverageKnown: true, coverageContinuous: true, acquisitionStarted: challenge.acquisitionStarted,
			observedAt: observed, coverageFrom: coverageFrom, coverageThrough: observed,
			authValidUntil:     challenge.acquisitionStarted.Add(maximumEvidenceAge),
			identityValidUntil: challenge.acquisitionStarted.Add(maximumEvidenceAge),
			coverageValidUntil: challenge.acquisitionStarted.Add(maximumEvidenceAge),
			providerValidUntil: challenge.acquisitionStarted.Add(maximumEvidenceAge),
		}
		if challenge.role == readerRole {
			evidence.sessionRevision = mapSyntheticIdentity("reader-revision", authorizedReaderRevision)
		} else {
			evidence.sessionRevision = mapSyntheticIdentity("collector-revision", authorizedCollectorRevision)
		}
		if mutate != nil {
			mutate(challenge, &evidence)
		}
		return evidence, nil
	}
}

func finishAuthorizedControllerTest(t *testing.T, controller *concurrentController, cancel context.CancelFunc) {
	t.Helper()
	cancel()
	ctx, done := context.WithTimeout(context.Background(), 2*time.Second)
	defer done()
	if err := controller.WaitParentCleanup(ctx); err != nil {
		t.Errorf("parent cleanup: %v", err)
	}
}

func startAuthorizedControllerTest(t *testing.T, controller *concurrentController) {
	t.Helper()
	if err := controller.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
}

func TestAuthorizedControllerDefaultOffRequiresExplicitSyntheticFixture(t *testing.T) {
	owners := newFakeConcurrentOwners()
	disabled, err := newAuthorizedConcurrentController(nil, concurrentConfig{}, concurrentPorts{owners: owners}, authorizedConcurrentFixture{})
	if err != nil {
		t.Fatalf("default-off constructor: %v", err)
	}
	if err := disabled.Start(context.Background()); !errors.Is(err, errConcurrentDisabled) {
		t.Fatalf("default-off Start error = %v", err)
	}
	if creates, closes, opens := owners.counts(); creates != 0 || closes != 0 || opens != 0 {
		t.Fatalf("default-off touched owner port: creates=%d closes=%d opens=%d", creates, closes, opens)
	}

	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	clock := newConcurrentTestClock()
	base := concurrentTestConfig(parent, clock, false)
	fixture := authorizedFixtureForController(base)
	fixture.Collect = authorizedControllerEvidence(clock, fixture.Grant, nil)
	for _, test := range []struct {
		name    string
		config  concurrentConfig
		fixture authorizedConcurrentFixture
	}{
		{name: "native provider name", config: func() concurrentConfig { c := base; c.Provider = "x-concurrent"; return c }(), fixture: fixture},
		{name: "missing fixture grant", config: base, fixture: func() authorizedConcurrentFixture { f := fixture; f.Grant = fixtureGrant{}; return f }()},
		{name: "account mismatch", config: base, fixture: func() authorizedConcurrentFixture { f := fixture; f.AccountID = "synthetic:account:other"; return f }()},
		{name: "independent collector", config: func() concurrentConfig { c := base; c.IndependentCollector = true; return c }(), fixture: fixture},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := newAuthorizedConcurrentController(parent, test.config, concurrentPorts{owners: owners}, test.fixture); !errors.Is(err, errConcurrentUnavailable) {
				t.Fatalf("constructor error = %v, want unavailable", err)
			}
			if creates, _, _ := owners.counts(); creates != 0 {
				t.Fatalf("invalid constructor created %d owners", creates)
			}
		})
	}

	controller, _, stop, _, _ := newAuthorizedControllerForTest(t, nil)
	defer finishAuthorizedControllerTest(t, controller, stop)
	startAuthorizedControllerTest(t, controller)
	if controller.ports.verifier != nil || controller.ports.permit != nil {
		t.Fatal("authorized controller unexpectedly installed legacy verifier or permit ports")
	}
	adapter, ok := controller.authorization.(*authorizedConcurrentAdapter)
	if !ok || adapter.authority == nil {
		t.Fatal("new path did not retain its private fixture authority")
	}
	view := adapter.authority.view()
	if view.nativeQualified || view.canAuthorize || !view.fixturePermitValid || !controller.Snapshot().AdmissionOpen {
		t.Fatalf("fixture state changed native gates or failed to open: authority=%+v controller=%+v", view, controller.Snapshot())
	}
	if adapter.authority.permit.readerRevision == adapter.authority.permit.collectorRevision {
		t.Fatal("controller collapsed independently pinned role revisions")
	}
	if adapter.authority.sideEffects != 0 {
		t.Fatalf("startup performed capture side effects: %d", adapter.authority.sideEffects)
	}
}

func TestAuthorizedControllerLongCaptureAcrossThreeRenewalsKeepsCollectorAndTicketVersion(t *testing.T) {
	controller, _, cancel, clock, _ := newAuthorizedControllerForTest(t, nil)
	defer finishAuthorizedControllerTest(t, controller, cancel)
	startAuthorizedControllerTest(t, controller)
	adapter := controller.authorization.(*authorizedConcurrentAdapter)
	originalCollector := controller.Snapshot().Collector
	workID, err := controller.QueueWork("synthetic:authorized-long-capture")
	if err != nil {
		t.Fatal(err)
	}
	ticket, err := controller.Ticket(workID)
	if err != nil {
		t.Fatal(err)
	}
	if ticket.EvidenceVersion != 1 || ticket.authorizationTicketID == "" {
		t.Fatalf("initial ticket did not pin the first evidence version: %+v", ticket)
	}
	if err := controller.Claim(context.Background(), ticket); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	for renewal := 1; renewal <= 3; renewal++ {
		clock.Advance(4 * time.Second)
		if err := controller.RenewEvidence(context.Background()); err != nil {
			t.Fatalf("RenewEvidence %d: %v", renewal, err)
		}
		if got := adapter.authority.view().evidenceVersion; got != uint64(renewal+1) {
			t.Fatalf("renewal %d evidence version = %d", renewal, got)
		}
		current := controller.Snapshot().Collector
		if !sameOwner(current, originalCollector) {
			t.Fatalf("renewal %d replaced collector: before=%+v after=%+v", renewal, originalCollector, current)
		}
		if ticket.EvidenceVersion != 1 {
			t.Fatalf("renewal retagged the dispatch ticket: %+v", ticket)
		}
	}
	if clock.Now().Sub(ticketTime(adapter.authority, ticket)) != 12*time.Second {
		t.Fatalf("capture duration = %s, want 12s", clock.Now().Sub(ticketTime(adapter.authority, ticket)))
	}
	if err := controller.Complete(context.Background(), ticket, "synthetic:authorized-result"); err != nil {
		t.Fatalf("Complete through three renewals: %v", err)
	}
	if got, ok := controller.SinkResult(workID); !ok || got != "synthetic:authorized-result" {
		t.Fatalf("controller result sink = %q, %v", got, ok)
	}
	if _, committed := adapter.authority.fixtureCounts(); committed != 1 {
		t.Fatalf("authority result count = %d, want 1", committed)
	}
	if state, _ := controller.WorkState(workID); state != concurrentWorkComplete {
		t.Fatalf("work state = %d, want complete", state)
	}
	if creates, _, _ := controller.ports.owners.(*fakeConcurrentOwnerPort).counts(); creates != 2 {
		t.Fatalf("owner creation count = %d, want exactly reader+collector", creates)
	}
}

func ticketTime(authority *concurrentAuthorization, ticket concurrentTicket) time.Time {
	authority.mu.Lock()
	defer authority.mu.Unlock()
	canonical := authority.tickets[ticket.authorizationTicketID]
	if canonical == nil {
		return time.Time{}
	}
	return canonical.ticket.dispatchAt
}

func TestAuthorizedControllerRejectsReplayedEvidenceAndCrossInstanceTickets(t *testing.T) {
	var savedMu sync.Mutex
	saved := make(map[role]roleEvidence)
	firstCollector := func(clock *concurrentTestClock) evidenceCollector {
		return func(ctx context.Context, challenge evidenceChallenge) (roleEvidence, error) {
			evidence, err := authorizedControllerEvidence(clock, fixtureGrantForTest(), nil)(ctx, challenge)
			if err == nil {
				savedMu.Lock()
				saved[challenge.role] = evidence
				savedMu.Unlock()
			}
			return evidence, err
		}
	}
	first, _, cancelFirst, _, _ := newAuthorizedControllerForTest(t, firstCollector)
	startAuthorizedControllerTest(t, first)
	defer finishAuthorizedControllerTest(t, first, cancelFirst)

	secondCollector := func(_ *concurrentTestClock) evidenceCollector {
		return func(_ context.Context, challenge evidenceChallenge) (roleEvidence, error) {
			savedMu.Lock()
			defer savedMu.Unlock()
			return saved[challenge.role], nil
		}
	}
	second, _, cancelSecond, _, _ := newAuthorizedControllerForTest(t, secondCollector)
	defer finishAuthorizedControllerTest(t, second, cancelSecond)
	if err := second.Start(context.Background()); err == nil {
		t.Fatal("second controller accepted another instance's saved evidence")
	}
	if second.Snapshot().AdmissionOpen || second.authorization.(*authorizedConcurrentAdapter).authority.view().fixturePermitValid {
		t.Fatalf("replayed evidence exposed authority: snapshot=%+v", second.Snapshot())
	}

	third, _, cancelThird, _, _ := newAuthorizedControllerForTest(t, nil)
	defer finishAuthorizedControllerTest(t, third, cancelThird)
	startAuthorizedControllerTest(t, third)
	firstWork, err := first.QueueWork("synthetic:first-instance-work")
	if err != nil {
		t.Fatal(err)
	}
	firstTicket, err := first.Ticket(firstWork)
	if err != nil {
		t.Fatal(err)
	}
	thirdWork, err := third.QueueWork("synthetic:third-instance-work")
	if err != nil {
		t.Fatal(err)
	}
	if err := third.Claim(context.Background(), firstTicket); !errors.Is(err, errConcurrentStale) {
		t.Fatalf("cross-instance Claim error = %v, want stale", err)
	}
	if state, _ := third.WorkState(thirdWork); state != concurrentWorkQueued {
		t.Fatalf("cross-instance ticket changed local work state to %d", state)
	}
}

func TestAuthorizedControllerCoverageGapAndExpiryFenceWork(t *testing.T) {
	t.Run("final snapshot cannot repair a dispatch gap", func(t *testing.T) {
		collector := authorizedControllerEvidence
		controller, _, cancel, clock, _ := newAuthorizedControllerForTest(t, nil)
		defer finishAuthorizedControllerTest(t, controller, cancel)
		startAuthorizedControllerTest(t, controller)
		adapter := controller.authorization.(*authorizedConcurrentAdapter)
		work, err := controller.QueueWork("synthetic:coverage-gap")
		if err != nil {
			t.Fatal(err)
		}
		ticket, err := controller.Ticket(work)
		if err != nil {
			t.Fatal(err)
		}
		clock.Advance(time.Second)
		if err := controller.Claim(context.Background(), ticket); !errors.Is(err, errConcurrentProof) {
			t.Fatalf("claim without coverage through barrier = %v", err)
		}
		fixture := adapter.fixture
		adapter.fixture.Collect = collector(clock, fixture.Grant, func(challenge evidenceChallenge, evidence *roleEvidence) {
			if challenge.previousVersion > 0 {
				evidence.coverageFrom = challenge.acquisitionStarted // Fresh endpoint snapshot omits the missing interval.
			}
		})
		if err := controller.RenewEvidence(context.Background()); !errors.Is(err, errConcurrentProof) {
			t.Fatalf("final-only evidence renewal error = %v", err)
		}
		if controller.Snapshot().AdmissionOpen {
			t.Fatal("final-only snapshot repaired the coverage gap")
		}
		if state, _ := controller.WorkState(work); state != concurrentWorkQueued {
			t.Fatalf("unstarted work state = %d, want retained queued work", state)
		}
	})

	t.Run("exact expiry cancels active work and preserves queued work", func(t *testing.T) {
		controller, _, cancel, clock, _ := newAuthorizedControllerForTest(t, nil)
		defer finishAuthorizedControllerTest(t, controller, cancel)
		startAuthorizedControllerTest(t, controller)
		activeID, _ := controller.QueueWork("synthetic:active-at-expiry")
		pendingID, _ := controller.QueueWork("synthetic:pending-at-expiry")
		ticket, err := controller.Ticket(activeID)
		if err != nil {
			t.Fatal(err)
		}
		if err := controller.Claim(context.Background(), ticket); err != nil {
			t.Fatal(err)
		}
		clock.Advance(maximumEvidenceAge)
		if snapshot := controller.Snapshot(); snapshot.AdmissionOpen || snapshot.Active != 0 || snapshot.Queued != 1 {
			t.Fatalf("expiry snapshot = %+v", snapshot)
		}
		if err := controller.Complete(context.Background(), ticket, "synthetic:too-late"); err == nil {
			t.Fatal("active result completed at exact evidence expiry")
		}
		if _, ok := controller.SinkResult(activeID); ok {
			t.Fatal("expired result reached controller sink")
		}
		if state, _ := controller.WorkState(pendingID); state != concurrentWorkQueued {
			t.Fatalf("pending work state = %d, want queued", state)
		}
		adapter := controller.authorization.(*authorizedConcurrentAdapter)
		_, results := adapter.authority.fixtureCounts()
		if results != 0 {
			t.Fatalf("expired authority results = %d", results)
		}
	})
}

func TestAuthorizedControllerRevokePolicyOwnerAndDisableWinBlockedRenewal(t *testing.T) {
	for _, mode := range []string{"revoke", "disable", "policy", "owner", "parent-cancel"} {
		t.Run(mode, func(t *testing.T) {
			entered := make(chan struct{}, 1)
			release := make(chan struct{})
			var closeRelease sync.Once
			collectorFactory := func(clock *concurrentTestClock) evidenceCollector {
				return func(ctx context.Context, challenge evidenceChallenge) (roleEvidence, error) {
					if challenge.previousVersion > 0 && challenge.role == readerRole {
						select {
						case entered <- struct{}{}:
						default:
						}
						select {
						case <-release:
						case <-ctx.Done():
							return roleEvidence{}, ctx.Err()
						}
					}
					return authorizedControllerEvidence(clock, fixtureGrantForTest(), nil)(ctx, challenge)
				}
			}
			controller, _, cancel, clock, owners := newAuthorizedControllerForTest(t, collectorFactory)
			defer func() {
				closeRelease.Do(func() { close(release) })
				finishAuthorizedControllerTest(t, controller, cancel)
			}()
			startAuthorizedControllerTest(t, controller)
			if _, err := controller.QueueWork("synthetic:queued-during-renewal"); err != nil {
				t.Fatal(err)
			}
			var originalCollector concurrentOwner
			if mode == "owner" {
				originalCollector = controller.Snapshot().Collector
			}
			clock.Advance(time.Second)
			renewDone := make(chan error, 1)
			go func() { renewDone <- controller.RenewEvidence(context.Background()) }()
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("renewal evidence collector did not block")
			}

			var parentDone chan struct{}
			switch mode {
			case "revoke":
				controller.Revoke("synthetic-test-revoke")
			case "disable":
				if err := controller.Disable(context.Background()); err != nil {
					t.Fatalf("Disable during renewal: %v", err)
				}
			case "policy":
				controller.PolicyChanged(5, 10, true)
			case "owner":
				owners.mutate(concurrentCollector, func(owner concurrentOwner) concurrentOwner { owner.Generation++; return owner })
				controller.OwnerChanged(concurrentCollector)
			case "parent-cancel":
				parentDone = make(chan struct{})
				go func() { cancel(); close(parentDone) }()
			}
			if mode != "parent-cancel" {
				closeRelease.Do(func() { close(release) })
			}
			select {
			case err := <-renewDone:
				if err == nil {
					t.Fatal("renewal published after a controller fence changed")
				}
			case <-time.After(2 * time.Second):
				t.Fatal("renewal did not return after cancellation or release")
			}
			if mode == "owner" {
				// The drift signal fences the pair. Restore the synthetic owner port
				// snapshot afterward so owned-tree cleanup remains verifiable.
				owners.mutate(concurrentCollector, func(concurrentOwner) concurrentOwner { return originalCollector })
			}
			if mode == "parent-cancel" {
				select {
				case <-parentDone:
				case <-time.After(time.Second):
					t.Fatal("parent cancellation did not return")
				}
				ctx, done := context.WithTimeout(context.Background(), 2*time.Second)
				if err := controller.WaitParentCleanup(ctx); err != nil {
					done()
					t.Fatalf("parent cleanup: %v", err)
				}
				done()
			}
			snapshot := controller.Snapshot()
			if snapshot.AdmissionOpen || snapshot.Active != 0 || snapshot.Queued != 1 {
				t.Fatalf("fence outcome snapshot = %+v", snapshot)
			}
			if adapter := controller.authorization.(*authorizedConcurrentAdapter); adapter.authority.view().fixturePermitValid {
				t.Fatal("authority retained a permit after controller fence")
			}
		})
	}
}

func TestAuthorizedControllerReaderInventoryAndIndependentContinuation(t *testing.T) {
	controller, _, cancel, _, owners := newAuthorizedControllerForTest(t, nil)
	defer finishAuthorizedControllerTest(t, controller, cancel)
	startAuthorizedControllerTest(t, controller)
	if err := controller.OpenReaderWindow(context.Background(), "synthetic:window-one", "synthetic:target-one", false); err != nil {
		t.Fatal(err)
	}
	if err := controller.OpenReaderWindow(context.Background(), "synthetic:window-two", "synthetic:target-two", false); err != nil {
		t.Fatal(err)
	}
	collector := controller.Snapshot().Collector
	if err := controller.CloseReaderWindow(context.Background(), "synthetic:window-one"); err != nil {
		t.Fatalf("non-final window close: %v", err)
	}
	if snapshot := controller.Snapshot(); !snapshot.AdmissionOpen || snapshot.ReaderWindows != 1 || !sameOwner(snapshot.Collector, collector) {
		t.Fatalf("non-final reader inventory change altered authority: %+v", snapshot)
	}
	if err := controller.CloseReaderWindow(context.Background(), "synthetic:window-two"); err == nil {
		t.Fatal("last reader close allowed independent collector continuation")
	}
	if snapshot := controller.Snapshot(); snapshot.AdmissionOpen || snapshot.ReaderWindows != 0 {
		t.Fatalf("last-reader loss did not revoke: %+v", snapshot)
	}
	if !owners.has(concurrentCollector) || owners.has(concurrentReader) {
		t.Fatal("reader loss closed the retained collector or kept the retired reader")
	}
	if creates, closes, _ := owners.counts(); creates != 2 || closes != 1 {
		t.Fatalf("reader window changes altered owners: creates=%d closes=%d", creates, closes)
	}
}

func TestAuthorizedControllerDisableThenFinalReaderCloseCompletesCleanup(t *testing.T) {
	controller, _, cancel, _, owners := newAuthorizedControllerForTest(t, nil)
	defer finishAuthorizedControllerTest(t, controller, cancel)
	startAuthorizedControllerTest(t, controller)
	const windowID = "synthetic:window:disable-retained"
	if err := controller.OpenReaderWindow(context.Background(), windowID, "synthetic:target:disable-retained", false); err != nil {
		t.Fatal(err)
	}
	if err := controller.Disable(context.Background()); err != nil {
		t.Fatalf("Disable: %v", err)
	}
	if snapshot := controller.Snapshot(); snapshot.Lifecycle != concurrentDisabledReaderRetained || snapshot.ReaderWindows != 1 || snapshot.HasCollector || !snapshot.HasReader {
		t.Fatalf("disabled retained-reader state = %+v", snapshot)
	}
	if err := controller.CloseReaderWindow(context.Background(), windowID); err != nil {
		t.Fatalf("final reader close after Disable: %v", err)
	}
	if snapshot := controller.Snapshot(); snapshot.Lifecycle != concurrentCleanupComplete || snapshot.HasReader || snapshot.HasCollector || snapshot.ReaderWindows != 0 {
		t.Fatalf("cleanup after final reader close = %+v", snapshot)
	}
	if owners.has(concurrentReader) || owners.has(concurrentCollector) {
		t.Fatal("final close left an owned reader or collector")
	}
}

func TestAuthorizedControllerCancellationDuplicateAndTicketIsolation(t *testing.T) {
	controller, _, cancel, _, _ := newAuthorizedControllerForTest(t, nil)
	defer finishAuthorizedControllerTest(t, controller, cancel)
	startAuthorizedControllerTest(t, controller)
	work, err := controller.QueueWork("synthetic:cancel-before-claim")
	if err != nil {
		t.Fatal(err)
	}
	ticket, err := controller.Ticket(work)
	if err != nil {
		t.Fatal(err)
	}
	if err := controller.Cancel(ticket); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if err := controller.Claim(context.Background(), ticket); err == nil {
		t.Fatal("claim revived a cancelled ticket")
	}
	if err := controller.Cancel(ticket); err == nil {
		t.Fatal("duplicate cancel succeeded")
	}
	if state, _ := controller.WorkState(work); state != concurrentWorkCancelled {
		t.Fatalf("cancelled work state = %d", state)
	}

	completeWork, _ := controller.QueueWork("synthetic:duplicate-result")
	completeTicket, err := controller.Ticket(completeWork)
	if err != nil {
		t.Fatal(err)
	}
	if err := controller.Claim(context.Background(), completeTicket); err != nil {
		t.Fatal(err)
	}
	if err := controller.Complete(context.Background(), completeTicket, "synthetic:one-result"); err != nil {
		t.Fatal(err)
	}
	if err := controller.Complete(context.Background(), completeTicket, "synthetic:duplicate-result"); err == nil {
		t.Fatal("duplicate result succeeded")
	}
	changed := completeTicket
	changed.EvidenceVersion++
	if err := controller.Claim(context.Background(), changed); err == nil {
		t.Fatal("ticket with forged evidence version succeeded")
	}

	newInstance, _, cancelNew, _, _ := newAuthorizedControllerForTest(t, nil)
	defer finishAuthorizedControllerTest(t, newInstance, cancelNew)
	startAuthorizedControllerTest(t, newInstance)
	newWork, _ := newInstance.QueueWork("synthetic:restart-work")
	if err := newInstance.Claim(context.Background(), completeTicket); !errors.Is(err, errConcurrentStale) {
		t.Fatalf("ticket survived controller restart: %v", err)
	}
	if state, _ := newInstance.WorkState(newWork); state != concurrentWorkQueued {
		t.Fatalf("restart ticket changed new work state to %d", state)
	}
}

func TestAuthorizedControllerAtomicResultAndRevokeAtBothSinks(t *testing.T) {
	t.Run("commit linearizes first", func(t *testing.T) {
		controller, _, cancel, _, _ := newAuthorizedControllerForTest(t, nil)
		defer finishAuthorizedControllerTest(t, controller, cancel)
		startAuthorizedControllerTest(t, controller)
		work, _ := controller.QueueWork("synthetic:commit-wins")
		ticket, _ := controller.Ticket(work)
		if err := controller.Claim(context.Background(), ticket); err != nil {
			t.Fatal(err)
		}
		if err := controller.Complete(context.Background(), ticket, "synthetic:committed"); err != nil {
			t.Fatal(err)
		}
		controller.Revoke("after-commit")
		adapter := controller.authorization.(*authorizedConcurrentAdapter)
		_, results := adapter.authority.fixtureCounts()
		_, controllerResult := controller.SinkResult(work)
		if results != 1 || !controllerResult {
			t.Fatalf("commit diverged across sinks: authority=%d controller=%v", results, controllerResult)
		}
	})

	t.Run("revoke linearizes before commit", func(t *testing.T) {
		controller, _, cancel, _, owners := newAuthorizedControllerForTest(t, nil)
		defer finishAuthorizedControllerTest(t, controller, cancel)
		startAuthorizedControllerTest(t, controller)
		work, _ := controller.QueueWork("synthetic:revoke-wins")
		ticket, _ := controller.Ticket(work)
		if err := controller.Claim(context.Background(), ticket); err != nil {
			t.Fatal(err)
		}
		owners.mu.Lock()
		owners.blockCurrent = concurrentCollector
		owners.currentEntered = make(chan struct{}, 1)
		owners.currentRelease = make(chan struct{})
		entered, release := owners.currentEntered, owners.currentRelease
		owners.mu.Unlock()
		completeDone := make(chan error, 1)
		go func() { completeDone <- controller.Complete(context.Background(), ticket, "synthetic:too-late") }()
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("completion did not reach owner recheck")
		}
		controller.Revoke("race-revoke")
		close(release)
		if err := <-completeDone; err == nil {
			t.Fatal("completion committed after revoke")
		}
		adapter := controller.authorization.(*authorizedConcurrentAdapter)
		_, results := adapter.authority.fixtureCounts()
		_, controllerResult := controller.SinkResult(work)
		if results != 0 || controllerResult {
			t.Fatalf("revoke/result race diverged: authority=%d controller=%v", results, controllerResult)
		}
		if state, _ := controller.WorkState(work); state != concurrentWorkCancelled {
			t.Fatalf("revoked active work state = %d", state)
		}
	})

	t.Run("callback failure does not commit authority sink", func(t *testing.T) {
		controller, _, cancel, _, _ := newAuthorizedControllerForTest(t, nil)
		defer finishAuthorizedControllerTest(t, controller, cancel)
		startAuthorizedControllerTest(t, controller)
		work, _ := controller.QueueWork("synthetic:callback-fails")
		ticket, _ := controller.Ticket(work)
		if err := controller.Claim(context.Background(), ticket); err != nil {
			t.Fatal(err)
		}
		adapter := controller.authorization.(*authorizedConcurrentAdapter)
		failure := errors.New("synthetic local callback failure")
		authorityTicket := concurrentAuthorizationTicket{ID: ticket.authorizationTicketID, EvidenceVersion: ticket.EvidenceVersion, WorkID: ticket.WorkID}
		err := adapter.Complete(authorityTicket, "synthetic:callback-result", func() error { return failure })
		if !errors.Is(err, failure) {
			t.Fatalf("callback failure error = %v", err)
		}
		_, results := adapter.authority.fixtureCounts()
		if results != 0 {
			t.Fatalf("authority committed result after callback failure: %d", results)
		}
		if status, _ := adapter.authority.ticketStatus(adapter.authorityTicket[ticket.authorizationTicketID]); status != workClaimed {
			t.Fatalf("authority ticket status after callback failure = %q", status)
		}
	})
}

func TestAuthorizedControllerRejectsEvidenceAcquisitionOverTwoSeconds(t *testing.T) {
	mutated := false
	collectorFactory := func(clock *concurrentTestClock) evidenceCollector {
		return authorizedControllerEvidence(clock, fixtureGrantForTest(), func(challenge evidenceChallenge, _ *roleEvidence) {
			if challenge.role == collectorRole && challenge.previousVersion == 0 {
				mutated = true
				clock.Advance(concurrentBindingDeadline + time.Second)
			}
		})
	}
	controller, _, cancel, _, _ := newAuthorizedControllerForTest(t, collectorFactory)
	defer finishAuthorizedControllerTest(t, controller, cancel)
	if err := controller.Start(context.Background()); err == nil {
		t.Fatalf("startup accepted evidence acquired after the two-second binding deadline (mutated=%v)", mutated)
	}
	if !mutated {
		t.Fatal("startup evidence collector did not apply the fake-clock deadline advance")
	}
	if controller.Snapshot().AdmissionOpen {
		t.Fatal("late startup evidence opened controller admission")
	}
}

func TestAuthorizedControllerRenewalReceivesBoundedCooperativeContext(t *testing.T) {
	seen := make(chan time.Duration, 1)
	collectorFactory := func(clock *concurrentTestClock) evidenceCollector {
		return func(ctx context.Context, challenge evidenceChallenge) (roleEvidence, error) {
			if challenge.previousVersion > 0 {
				deadline, ok := ctx.Deadline()
				if !ok {
					return roleEvidence{}, errors.New("renewal collector has no deadline")
				}
				select {
				case seen <- time.Until(deadline):
				default:
				}
			}
			return authorizedControllerEvidence(clock, fixtureGrantForTest(), nil)(ctx, challenge)
		}
	}
	controller, _, cancel, clock, _ := newAuthorizedControllerForTest(t, collectorFactory)
	defer finishAuthorizedControllerTest(t, controller, cancel)
	startAuthorizedControllerTest(t, controller)
	clock.Advance(time.Second)
	if err := controller.RenewEvidence(context.Background()); err != nil {
		t.Fatal(err)
	}
	if remaining := <-seen; remaining <= 0 || remaining > concurrentBindingDeadline {
		t.Fatalf("renewal collector deadline remaining = %s", remaining)
	}
}

func TestAuthorizedControllerRenewalCancellationDoesNotLeakCollector(t *testing.T) {
	entered := make(chan struct{}, 1)
	collectorFactory := func(clock *concurrentTestClock) evidenceCollector {
		return func(ctx context.Context, challenge evidenceChallenge) (roleEvidence, error) {
			if challenge.previousVersion > 0 && challenge.role == readerRole {
				entered <- struct{}{}
				<-ctx.Done()
				return roleEvidence{}, ctx.Err()
			}
			return authorizedControllerEvidence(clock, fixtureGrantForTest(), nil)(ctx, challenge)
		}
	}
	controller, _, cancel, clock, _ := newAuthorizedControllerForTest(t, collectorFactory)
	defer finishAuthorizedControllerTest(t, controller, cancel)
	startAuthorizedControllerTest(t, controller)
	clock.Advance(time.Second)
	ctx, cancelRenew := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- controller.RenewEvidence(ctx) }()
	<-entered
	cancelRenew()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled evidence acquisition renewed the permit")
		}
	case <-time.After(time.Second):
		t.Fatal("cooperative collector remained blocked after cancellation")
	}
	if controller.Snapshot().AdmissionOpen {
		t.Fatal("cancelled renewal left admission open")
	}
}

type blockingAuthorizedSeam struct {
	concurrentAuthorizationSeam
	claimEntered    chan struct{}
	claimRelease    chan struct{}
	completeEntered chan struct{}
	completeRelease chan struct{}
}

func blockSeamCall(entered, release chan struct{}) {
	if entered == nil || release == nil {
		return
	}
	select {
	case entered <- struct{}{}:
	default:
	}
	<-release
}

func (s *blockingAuthorizedSeam) Claim(ticket concurrentAuthorizationTicket, commit func() error) error {
	blockSeamCall(s.claimEntered, s.claimRelease)
	return s.concurrentAuthorizationSeam.Claim(ticket, commit)
}

func (s *blockingAuthorizedSeam) Complete(ticket concurrentAuthorizationTicket, result string, commit func() error) error {
	blockSeamCall(s.completeEntered, s.completeRelease)
	return s.concurrentAuthorizationSeam.Complete(ticket, result, commit)
}

func TestAuthorizedControllerRechecksCancellationAtCommitBoundary(t *testing.T) {
	t.Run("operation cancellation while claim waits", func(t *testing.T) {
		controller, _, cancel, _, _ := newAuthorizedControllerForTest(t, nil)
		defer finishAuthorizedControllerTest(t, controller, cancel)
		startAuthorizedControllerTest(t, controller)
		work, _ := controller.QueueWork("synthetic:cancel-claim-boundary")
		ticket, _ := controller.Ticket(work)
		base := controller.authorization
		seam := &blockingAuthorizedSeam{concurrentAuthorizationSeam: base, claimEntered: make(chan struct{}, 1), claimRelease: make(chan struct{})}
		controller.mu.Lock()
		controller.authorization = seam
		controller.mu.Unlock()
		ctx, cancelCall := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- controller.Claim(ctx, ticket) }()
		<-seam.claimEntered
		cancelCall()
		close(seam.claimRelease)
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatalf("claim after operation cancellation = %v", err)
		}
		if state, _ := controller.WorkState(work); state != concurrentWorkQueued {
			t.Fatalf("cancelled claim changed local work state to %d", state)
		}
		adapter := base.(*authorizedConcurrentAdapter)
		sideEffects, results := adapter.authority.fixtureCounts()
		if sideEffects != 0 || results != 0 {
			t.Fatalf("cancelled claim changed authority sinks: sideEffects=%d results=%d", sideEffects, results)
		}
	})

	t.Run("parent cancellation while complete waits", func(t *testing.T) {
		controller, _, cancel, _, _ := newAuthorizedControllerForTest(t, nil)
		startAuthorizedControllerTest(t, controller)
		work, _ := controller.QueueWork("synthetic:parent-cancel-complete-boundary")
		ticket, _ := controller.Ticket(work)
		if err := controller.Claim(context.Background(), ticket); err != nil {
			t.Fatal(err)
		}
		base := controller.authorization
		seam := &blockingAuthorizedSeam{concurrentAuthorizationSeam: base, completeEntered: make(chan struct{}, 1), completeRelease: make(chan struct{})}
		controller.mu.Lock()
		controller.authorization = seam
		controller.mu.Unlock()
		done := make(chan error, 1)
		go func() { done <- controller.Complete(context.Background(), ticket, "synthetic:parent-cancelled-result") }()
		<-seam.completeEntered
		cancel()
		close(seam.completeRelease)
		if err := <-done; err == nil {
			t.Fatal("complete committed after parent cancellation")
		}
		ctx, doneWait := context.WithTimeout(context.Background(), 2*time.Second)
		if err := controller.WaitParentCleanup(ctx); err != nil {
			doneWait()
			t.Fatalf("parent cleanup: %v", err)
		}
		doneWait()
		if _, ok := controller.SinkResult(work); ok {
			t.Fatal("parent-cancelled result reached controller sink")
		}
		adapter := base.(*authorizedConcurrentAdapter)
		_, results := adapter.authority.fixtureCounts()
		if results != 0 {
			t.Fatalf("parent-cancelled result reached authority sink: %d", results)
		}
		if state, _ := controller.WorkState(work); state != concurrentWorkCancelled {
			t.Fatalf("parent-cancelled active work state = %d", state)
		}
		// The parent watcher has already completed; avoid a second cleanup wait
		// from a deferred helper with a different lifecycle expectation.
	})
}

func TestAuthorizedControllerTicketMetadataCannotCrossWork(t *testing.T) {
	controller, _, cancel, _, _ := newAuthorizedControllerForTest(t, nil)
	defer finishAuthorizedControllerTest(t, controller, cancel)
	startAuthorizedControllerTest(t, controller)
	first, _ := controller.QueueWork("synthetic:ticket-a")
	second, _ := controller.QueueWork("synthetic:ticket-b")
	ticketA, err := controller.Ticket(first)
	if err != nil {
		t.Fatal(err)
	}
	ticketB, err := controller.Ticket(second)
	if err != nil {
		t.Fatal(err)
	}
	forged := ticketB
	forged.authorizationTicketID = ticketA.authorizationTicketID
	forged.EvidenceVersion = ticketA.EvidenceVersion
	if err := controller.Claim(context.Background(), forged); err == nil {
		t.Fatal("authority ticket for one work item claimed another")
	}
	if state, _ := controller.WorkState(second); state != concurrentWorkQueued {
		t.Fatalf("forged ticket changed second work state %d", state)
	}
}

func TestAuthorizedControllerRenewalLeavesQueuePendingAndRejectsLateTicket(t *testing.T) {
	controller, _, cancel, clock, _ := newAuthorizedControllerForTest(t, nil)
	defer finishAuthorizedControllerTest(t, controller, cancel)
	startAuthorizedControllerTest(t, controller)
	work, _ := controller.QueueWork("synthetic:stale-ticket-version")
	ticket, err := controller.Ticket(work)
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(4 * time.Second)
	if err := controller.RenewEvidence(context.Background()); err != nil {
		t.Fatal(err)
	}
	if ticket.EvidenceVersion != 1 {
		t.Fatalf("dispatch ticket version was retagged: %d", ticket.EvidenceVersion)
	}
	changed := ticket
	changed.EvidenceVersion = 2
	if err := controller.Claim(context.Background(), changed); err == nil {
		t.Fatal("forged current version replaced the dispatch version")
	}
	if err := controller.Claim(context.Background(), ticket); err != nil {
		t.Fatalf("valid old ticket did not traverse renewal chain: %v", err)
	}
	if state, _ := controller.WorkState(work); state != concurrentWorkActive {
		t.Fatalf("valid old ticket state = %d", state)
	}
	if got := controller.authorization.(*authorizedConcurrentAdapter).authority.view().evidenceVersion; got != 2 {
		t.Fatalf("current authority version = %d, want 2", got)
	}
}
