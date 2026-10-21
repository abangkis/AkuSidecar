//go:build auth_session_dev && auth_session_fixture

package engine

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type fixtureClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFixtureClock() *fixtureClock {
	return &fixtureClock{now: time.Date(2026, 10, 21, 1, 2, 3, 0, time.UTC)}
}

func (c *fixtureClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fixtureClock) Advance(delta time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(delta)
}

func (c *fixtureClock) Set(value time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = value
}

func fixtureGrantForTest() fixtureGrant {
	return fixtureGrant{
		id:                 "synthetic-fixture-grant-1",
		allowAuthorization: true,
		allowSeed:          true,
		allowCollection:    true,
		allowRenewal:       true,
		allowResultSink:    true,
	}
}

func fixtureConfigForTest(enabled bool) authorizationConfig {
	createdAt := time.Date(2026, 10, 20, 23, 0, 0, 0, time.UTC)
	return authorizationConfig{
		enrollment: enrollment{
			contractID:       concurrentAuthorizationContract,
			providerID:       fixtureProviderID,
			providerVersion:  fixtureProviderVersion,
			evidenceSchema:   fixtureEvidenceSchema,
			sourceID:         fixtureSourceID,
			databaseScope:    "synthetic-db-profile-a",
			accountID:        "synthetic-account-a",
			readerProfileID:  "synthetic-reader-profile-a",
			policyGeneration: 4,
			holdGeneration:   9,
		},
		pair: pairIdentity{
			id:                "synthetic-pair-a",
			attemptID:         "synthetic-attempt-a",
			controllerEpochID: "synthetic-controller-epoch-a",
			sidecarLifetimeID: "synthetic-sidecar-lifetime-a",
			generation:        12,
		},
		reader: processOwner{
			role:              readerRole,
			ownerID:           "synthetic-reader-owner-a",
			profileID:         "synthetic-reader-profile-a",
			directoryID:       "synthetic-reader-dir-a",
			processID:         "synthetic-reader-process-a",
			processCreationID: "synthetic-reader-creation-a",
			processCreatedAt:  createdAt,
			processImageID:    "synthetic-reader-image-a",
			sidecarLifetimeID: "synthetic-sidecar-lifetime-a",
			contextID:         "synthetic-reader-context-a",
			epochID:           "synthetic-controller-epoch-a",
			generation:        3,
			driverID:          "synthetic-reader-driver-a",
			directoryPinned:   true,
			windowInventory:   []string{"synthetic-reader-window-a", "synthetic-reader-window-b"},
		},
		collector: processOwner{
			role:              collectorRole,
			ownerID:           "synthetic-collector-owner-a",
			profileID:         "synthetic-collector-profile-a",
			directoryID:       "synthetic-collector-dir-a",
			processID:         "synthetic-collector-process-a",
			processCreationID: "synthetic-collector-creation-a",
			processCreatedAt:  createdAt.Add(time.Minute),
			processImageID:    "synthetic-collector-image-a",
			sidecarLifetimeID: "synthetic-sidecar-lifetime-a",
			contextID:         "synthetic-collector-context-a",
			epochID:           "synthetic-controller-epoch-a",
			generation:        5,
			driverID:          "synthetic-collector-driver-a",
			directoryPinned:   true,
			profileFresh:      true,
			disposableContext: true,
			windowInventory:   []string{"synthetic-collector-window-a"},
		},
		policyEnabled: enabled,
	}
}

func fixtureReceiptForTest(config authorizationConfig, grant fixtureGrant, instanceID string) derivationReceipt {
	return derivationReceipt{
		instanceID:             instanceID,
		id:                     "synthetic-derivation-receipt-a",
		contractID:             config.enrollment.contractID,
		providerID:             config.enrollment.providerID,
		providerVersion:        config.enrollment.providerVersion,
		evidenceSchema:         config.enrollment.evidenceSchema,
		databaseScope:          config.enrollment.databaseScope,
		sourceID:               config.enrollment.sourceID,
		accountID:              config.enrollment.accountID,
		pairID:                 config.pair.id,
		attemptID:              config.pair.attemptID,
		pairGeneration:         config.pair.generation,
		readerOwnerID:          config.reader.ownerID,
		collectorOwnerID:       config.collector.ownerID,
		readerRevision:         "synthetic-reader-revision-1",
		collectorRevision:      "synthetic-collector-revision-A",
		transferManifest:       fixtureTransferManifest,
		fromRole:               readerRole,
		toRole:                 collectorRole,
		seedApplied:            true,
		containsCookieMaterial: false,
		fixtureGrantID:         grant.id,
	}
}

func newFixtureAuthority(t *testing.T, clock *fixtureClock, config authorizationConfig) (*concurrentAuthorization, fixtureGrant) {
	t.Helper()
	grant := fixtureGrantForTest()
	authority, err := newConcurrentAuthorization(config, grant, clock.Now)
	if err != nil {
		t.Fatalf("newConcurrentAuthorization() error = %v", err)
	}
	return authority, grant
}

func newAuthorizedFixture(t *testing.T) (*concurrentAuthorization, *fixtureClock, fixtureGrant) {
	t.Helper()
	clock := newFixtureClock()
	config := fixtureConfigForTest(true)
	authority, grant := newFixtureAuthority(t, clock, config)
	if err := authority.recordFixtureDerivation(fixtureReceiptForTest(config, grant, authority.instanceID)); err != nil {
		t.Fatalf("recordFixtureDerivation() error = %v", err)
	}
	if err := authority.authorize(context.Background(), fixtureEvidenceCollector(clock, grant, nil)); err != nil {
		t.Fatalf("authorize() error = %v", err)
	}
	return authority, clock, grant
}

func fixtureEvidenceCollector(clock *fixtureClock, grant fixtureGrant, mutate func(evidenceChallenge, *roleEvidence)) evidenceCollector {
	return func(ctx context.Context, challenge evidenceChallenge) (roleEvidence, error) {
		if err := contextError(ctx); err != nil {
			return roleEvidence{}, err
		}
		observedAt := clock.Now()
		coverageFrom := challenge.previousCoverage
		if challenge.previousVersion == 0 {
			coverageFrom = challenge.acquisitionStarted
		}
		evidence := roleEvidence{
			instanceID:         challenge.instanceID,
			challengeID:        challenge.id,
			vectorID:           challenge.vectorID,
			contractID:         challenge.contractID,
			providerID:         challenge.providerID,
			providerVersion:    challenge.providerVersion,
			evidenceSchema:     challenge.evidenceSchema,
			sourceID:           challenge.sourceID,
			origin:             challenge.origin,
			operation:          challenge.operation,
			databaseScope:      challenge.databaseScope,
			accountID:          challenge.accountID,
			pairID:             challenge.pairID,
			attemptID:          challenge.attemptID,
			pairGeneration:     challenge.pairGeneration,
			role:               challenge.role,
			owner:              cloneOwner(challenge.owner),
			frameID:            challenge.frameID,
			requestID:          challenge.requestID,
			fixtureGrantID:     grant.id,
			provenance:         fixtureProvenance,
			authentication:     fixtureAuthentication,
			identity:           fixtureIdentity,
			cacheState:         fixtureCacheFresh,
			authenticated:      true,
			continuityKnown:    true,
			coverageKnown:      true,
			coverageContinuous: true,
			sessionRevision:    "synthetic-reader-revision-1",
			acquisitionStarted: challenge.acquisitionStarted,
			observedAt:         observedAt,
			coverageFrom:       coverageFrom,
			coverageThrough:    observedAt,
			authValidUntil:     challenge.acquisitionStarted.Add(maximumEvidenceAge),
			identityValidUntil: challenge.acquisitionStarted.Add(maximumEvidenceAge),
			coverageValidUntil: challenge.acquisitionStarted.Add(maximumEvidenceAge),
			providerValidUntil: challenge.acquisitionStarted.Add(maximumEvidenceAge),
		}
		if challenge.role == collectorRole {
			evidence.sessionRevision = "synthetic-collector-revision-A"
		}
		if mutate != nil {
			mutate(challenge, &evidence)
		}
		return evidence, nil
	}
}

func fixtureCommand(id string) workCommand {
	return workCommand{id: id, scope: "synthetic-db-profile-a", source: fixtureSourceID, operation: "synthetic-x.collect"}
}

func fixtureResultFor(ticket workTicket) fixtureResult {
	return fixtureResult{
		id:               "synthetic-result-" + ticket.ticketID,
		ticketID:         ticket.ticketID,
		commandID:        ticket.commandID,
		scope:            ticket.scope,
		source:           ticket.source,
		operation:        ticket.operation,
		evidenceVersion:  ticket.evidenceVersion,
		pairID:           ticket.pairID,
		pairGeneration:   ticket.pairGeneration,
		fence:            ticket.fence,
		collectorOwnerID: ticket.collectorOwnerID,
	}
}

func requireReason(t *testing.T, err error, want authorizationReason) {
	t.Helper()
	if err == nil {
		t.Fatalf("error reason = <nil>, want %q", want)
	}
	var typed authorizationError
	if !errors.As(err, &typed) || typed.reason != want {
		t.Fatalf("error reason = %v, want %q", err, want)
	}
}

func TestAdmissionStartsClosedAndDisabledFixtureHasNoEffects(t *testing.T) {
	clock := newFixtureClock()
	config := fixtureConfigForTest(false)
	authority, grant := newFixtureAuthority(t, clock, config)
	view := authority.view()
	if view.nativeQualified || view.canAuthorize || view.fixturePermitValid {
		t.Fatalf("closed fixture exposed authority: %+v", view)
	}
	requireReason(t, authority.recordFixtureDerivation(fixtureReceiptForTest(config, grant, authority.instanceID)), reasonDisabled)
	requireReason(t, authority.authorize(context.Background(), fixtureEvidenceCollector(clock, grant, nil)), reasonDisabled)
	_, err := authority.dispatch(fixtureCommand("synthetic-work-disabled"))
	requireReason(t, err, reasonDisabled)
	sideEffects, results := authority.fixtureCounts()
	if sideEffects != 0 || results != 0 {
		t.Fatalf("disabled fixture effects/results = %d/%d, want 0/0", sideEffects, results)
	}
	if _, err := newConcurrentAuthorization(config, fixtureGrant{}, clock.Now); reasonOf(err) != reasonFixtureGrant {
		t.Fatalf("missing grant error = %v, want %q", err, reasonFixtureGrant)
	}
}

func TestInvalidConfigurationsAndNativeProviderAreRejected(t *testing.T) {
	clock := newFixtureClock()
	cases := []struct {
		name   string
		mutate func(*authorizationConfig)
		want   authorizationReason
	}{
		{name: "native proposed provider", mutate: func(c *authorizationConfig) { c.enrollment.providerID = "x-concurrent" }, want: reasonUnsupportedProvider},
		{name: "same physical profile", mutate: func(c *authorizationConfig) { c.collector.profileID = c.reader.profileID }, want: reasonInvalidConfiguration},
		{name: "native owner identity", mutate: func(c *authorizationConfig) { c.reader.processID = "chrome-1234" }, want: reasonInvalidConfiguration},
		{name: "unversioned evidence schema", mutate: func(c *authorizationConfig) { c.enrollment.evidenceSchema = "synthetic-unknown-schema" }, want: reasonUnsupportedProvider},
		{name: "collector is not disposable", mutate: func(c *authorizationConfig) { c.collector.disposableContext = false }, want: reasonInvalidConfiguration},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			config := fixtureConfigForTest(true)
			test.mutate(&config)
			_, err := newConcurrentAuthorization(config, fixtureGrantForTest(), clock.Now)
			requireReason(t, err, test.want)
		})
	}
}

func TestDistinctRoleRevisionsAuthorizeButNativeFlagsRemainFalse(t *testing.T) {
	authority, _, _ := newAuthorizedFixture(t)
	view := authority.view()
	if !view.fixturePermitValid || view.evidenceVersion != 1 || view.pairGeneration != 12 {
		t.Fatalf("fixture view = %+v", view)
	}
	if view.nativeQualified || view.canAuthorize {
		t.Fatalf("fixture permit changed native flags: %+v", view)
	}
	if authority.permit.readerRevision == authority.permit.collectorRevision {
		t.Fatal("reader and collector revisions unexpectedly collapsed")
	}
	if authority.permit.readerRevision != "synthetic-reader-revision-1" || authority.permit.collectorRevision != "synthetic-collector-revision-A" {
		t.Fatalf("accepted role revisions = %q/%q", authority.permit.readerRevision, authority.permit.collectorRevision)
	}
}

func TestForgedCrossRoleReplayAndUnknownEvidenceAreRejected(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(evidenceChallenge, *roleEvidence)
		want   authorizationReason
	}{
		{name: "role swap", mutate: func(_ evidenceChallenge, e *roleEvidence) { e.role = collectorRole }, want: reasonInvalidEvidence},
		{name: "stale challenge replay", mutate: func(_ evidenceChallenge, e *roleEvidence) { e.challengeID = "synthetic-old-challenge" }, want: reasonInvalidEvidence},
		{name: "wrong contract", mutate: func(_ evidenceChallenge, e *roleEvidence) { e.contractID = "synthetic-other-contract" }, want: reasonInvalidEvidence},
		{name: "wrong provider version", mutate: func(_ evidenceChallenge, e *roleEvidence) { e.providerVersion = "synthetic-provider-v2" }, want: reasonInvalidEvidence},
		{name: "wrong scope", mutate: func(_ evidenceChallenge, e *roleEvidence) { e.databaseScope = "synthetic-db-other" }, want: reasonInvalidEvidence},
		{name: "wrong schema", mutate: func(_ evidenceChallenge, e *roleEvidence) { e.evidenceSchema = "synthetic-schema-other" }, want: reasonInvalidEvidence},
		{name: "native source claim", mutate: func(_ evidenceChallenge, e *roleEvidence) { e.claimedNativeSource = true }, want: reasonInvalidEvidence},
		{name: "wrong frame binding", mutate: func(_ evidenceChallenge, e *roleEvidence) { e.frameID = "synthetic-frame-forged" }, want: reasonInvalidEvidence},
		{name: "wrong request binding", mutate: func(_ evidenceChallenge, e *roleEvidence) { e.requestID = "synthetic-request-forged" }, want: reasonInvalidEvidence},
		{name: "unknown authentication", mutate: func(_ evidenceChallenge, e *roleEvidence) { e.authenticated = false; e.authentication = "" }, want: reasonUnknownAuth},
		{name: "unknown identity", mutate: func(_ evidenceChallenge, e *roleEvidence) { e.identity = "" }, want: reasonUnknownIdentity},
		{name: "unknown lineage", mutate: func(_ evidenceChallenge, e *roleEvidence) { e.continuityKnown = false }, want: reasonUnknownLineage},
		{name: "unknown coverage", mutate: func(_ evidenceChallenge, e *roleEvidence) { e.coverageKnown = false }, want: reasonUnknownLineage},
		{name: "coverage gap", mutate: func(_ evidenceChallenge, e *roleEvidence) { e.coverageGap = true }, want: reasonCoverageGap},
		{name: "cross-role owner", mutate: func(ch evidenceChallenge, e *roleEvidence) {
			if ch.role == readerRole {
				e.owner.role = collectorRole
				e.owner.ownerID = "synthetic-collector-owner-a"
			}
		}, want: reasonOwnerDrift},
		{name: "fixture provenance", mutate: func(_ evidenceChallenge, e *roleEvidence) { e.provenance = "synthetic-unqualified" }, want: reasonInvalidEvidence},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			clock := newFixtureClock()
			config := fixtureConfigForTest(true)
			authority, grant := newFixtureAuthority(t, clock, config)
			if err := authority.recordFixtureDerivation(fixtureReceiptForTest(config, grant, authority.instanceID)); err != nil {
				t.Fatal(err)
			}
			err := authority.authorize(context.Background(), fixtureEvidenceCollector(clock, grant, test.mutate))
			requireReason(t, err, test.want)
			if authority.view().fixturePermitValid {
				t.Fatal("invalid evidence published a permit")
			}
		})
	}
}

func TestBindingAndEvidenceDeadlinesIncludePublication(t *testing.T) {
	for _, test := range []struct {
		name  string
		delta time.Duration
		want  authorizationReason
	}{
		{name: "over two second binding", delta: maximumBindingAge + time.Millisecond, want: reasonBindingDeadline},
		{name: "exact five second evidence age", delta: maximumEvidenceAge, want: reasonEvidenceExpired},
	} {
		t.Run(test.name, func(t *testing.T) {
			clock := newFixtureClock()
			config := fixtureConfigForTest(true)
			authority, grant := newFixtureAuthority(t, clock, config)
			if err := authority.recordFixtureDerivation(fixtureReceiptForTest(config, grant, authority.instanceID)); err != nil {
				t.Fatal(err)
			}
			collector := fixtureEvidenceCollector(clock, grant, func(ch evidenceChallenge, _ *roleEvidence) {
				if ch.role == collectorRole {
					clock.Advance(test.delta)
				}
			})
			requireReason(t, authority.authorize(context.Background(), collector), test.want)
		})
	}
}

func TestProviderValidityCapsPublishedPermitExpiry(t *testing.T) {
	clock := newFixtureClock()
	config := fixtureConfigForTest(true)
	authority, grant := newFixtureAuthority(t, clock, config)
	if err := authority.recordFixtureDerivation(fixtureReceiptForTest(config, grant, authority.instanceID)); err != nil {
		t.Fatal(err)
	}
	shortValidity := clock.Now().Add(900 * time.Millisecond)
	collector := fixtureEvidenceCollector(clock, grant, func(challenge evidenceChallenge, evidence *roleEvidence) {
		if challenge.role == readerRole {
			evidence.providerValidUntil = shortValidity
		}
	})
	if err := authority.authorize(context.Background(), collector); err != nil {
		t.Fatal(err)
	}
	if got := authority.view().expiresAt; !got.Equal(shortValidity) {
		t.Fatalf("permit expiry = %s, want earliest provider expiry %s", got, shortValidity)
	}
}

func TestCollectorReceivesTwoSecondBoundedContext(t *testing.T) {
	clock := newFixtureClock()
	config := fixtureConfigForTest(true)
	authority, grant := newFixtureAuthority(t, clock, config)
	if err := authority.recordFixtureDerivation(fixtureReceiptForTest(config, grant, authority.instanceID)); err != nil {
		t.Fatal(err)
	}
	base := fixtureEvidenceCollector(clock, grant, nil)
	collector := func(ctx context.Context, challenge evidenceChallenge) (roleEvidence, error) {
		deadline, ok := ctx.Deadline()
		remaining := time.Until(deadline)
		if !ok || remaining <= 0 || remaining > maximumBindingAge {
			return roleEvidence{}, errors.New("collector did not receive the expected bounded context")
		}
		return base(ctx, challenge)
	}
	if err := authority.authorize(context.Background(), collector); err != nil {
		t.Fatalf("authorize with bounded collector context: %v", err)
	}
}

func TestChangedRoleRevisionDuringRenewalFencesPermit(t *testing.T) {
	for _, changedRole := range []role{readerRole, collectorRole} {
		t.Run(string(changedRole), func(t *testing.T) {
			authority, clock, grant := newAuthorizedFixture(t)
			clock.Advance(time.Second)
			collector := fixtureEvidenceCollector(clock, grant, func(challenge evidenceChallenge, evidence *roleEvidence) {
				if challenge.role == changedRole {
					evidence.sessionRevision = "synthetic-changed-session-revision"
				}
			})
			requireReason(t, authority.renew(context.Background(), collector), reasonSessionChanged)
			if authority.view().fixturePermitValid {
				t.Fatal("changed session revision kept the permit")
			}
		})
	}
}

func TestLongCaptureSurvivesSeveralContinuousRenewals(t *testing.T) {
	authority, clock, grant := newAuthorizedFixture(t)
	initialPair := authority.config.pair
	ticket, err := authority.dispatch(fixtureCommand("synthetic-long-capture"))
	if err != nil {
		t.Fatal(err)
	}
	if err := authority.claim(ticket); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		clock.Advance(4 * time.Second)
		if err := authority.renew(context.Background(), fixtureEvidenceCollector(clock, grant, nil)); err != nil {
			t.Fatalf("renewal %d: %v", i+1, err)
		}
	}
	if got := authority.view().evidenceVersion; got != 4 {
		t.Fatalf("evidence version = %d, want 4", got)
	}
	if !pairEqual(initialPair, authority.config.pair) || ticket.evidenceVersion != 1 {
		t.Fatal("renewal changed pair identity or retagged the original ticket")
	}
	if err := authority.complete(ticket, fixtureResultFor(ticket)); err != nil {
		t.Fatalf("complete through renewal chain: %v", err)
	}
	_, committed := authority.fixtureCounts()
	if committed != 1 {
		t.Fatalf("committed results = %d, want 1", committed)
	}
}

func TestCoverageGapCannotBeRepairedByFinalSnapshot(t *testing.T) {
	authority, clock, grant := newAuthorizedFixture(t)
	ticket, err := authority.dispatch(fixtureCommand("synthetic-gap-capture"))
	if err != nil {
		t.Fatal(err)
	}
	if err := authority.claim(ticket); err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Second)
	gap := fixtureEvidenceCollector(clock, grant, func(_ evidenceChallenge, e *roleEvidence) { e.coverageGap = true })
	requireReason(t, authority.renew(context.Background(), gap), reasonCoverageGap)
	if err := authority.complete(ticket, fixtureResultFor(ticket)); reasonOf(err) != reasonCoverageGap {
		t.Fatalf("result after coverage gap error = %v", err)
	}
	requireReason(t, authority.renew(context.Background(), fixtureEvidenceCollector(clock, grant, nil)), reasonCoverageGap)
	if authority.view().fixturePermitValid {
		t.Fatal("final fresh snapshot repaired a prior coverage gap")
	}
}

func TestClaimAndCommitRequireCoverageThroughEachBarrier(t *testing.T) {
	authority, clock, grant := newAuthorizedFixture(t)
	ticket, err := authority.dispatch(fixtureCommand("synthetic-barrier-work"))
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Second)
	requireReason(t, authority.claim(ticket), reasonCoverageGap)
	if err := authority.renew(context.Background(), fixtureEvidenceCollector(clock, grant, nil)); err != nil {
		t.Fatalf("renew before claim: %v", err)
	}
	if err := authority.claim(ticket); err != nil {
		t.Fatalf("claim after coverage checkpoint: %v", err)
	}
	clock.Advance(time.Second)
	requireReason(t, authority.complete(ticket, fixtureResultFor(ticket)), reasonCoverageGap)
	if err := authority.renew(context.Background(), fixtureEvidenceCollector(clock, grant, nil)); err != nil {
		t.Fatalf("renew before commit: %v", err)
	}
	if err := authority.complete(ticket, fixtureResultFor(ticket)); err != nil {
		t.Fatalf("commit after coverage checkpoint: %v", err)
	}
}

func TestNonFinalReaderWindowChangePreservesPairAndCollector(t *testing.T) {
	authority, _, _ := newAuthorizedFixture(t)
	pair := authority.config.pair
	collectorOwner := cloneOwner(authority.config.collector)
	if err := authority.updateReaderWindows([]string{"synthetic-reader-window-b", "synthetic-reader-window-c"}); err != nil {
		t.Fatal(err)
	}
	view := authority.view()
	if !view.fixturePermitValid || !pairEqual(pair, authority.config.pair) || !ownerEqual(collectorOwner, authority.config.collector) {
		t.Fatalf("non-final window change altered authority: view=%+v", view)
	}
	requireReason(t, authority.updateReaderWindows(nil), reasonReaderLost)
	if authority.view().fixturePermitValid {
		t.Fatal("last reader window did not revoke dependent authority")
	}
}

func TestExpiryFencesActiveResultsAndPreservesPendingWork(t *testing.T) {
	authority, clock, _ := newAuthorizedFixture(t)
	active, err := authority.dispatch(fixtureCommand("synthetic-active-work"))
	if err != nil {
		t.Fatal(err)
	}
	if err := authority.claim(active); err != nil {
		t.Fatal(err)
	}
	pending, err := authority.dispatch(fixtureCommand("synthetic-pending-work"))
	if err != nil {
		t.Fatal(err)
	}
	oldFence := authority.view().fence
	clock.Set(authority.view().expiresAt)
	requireReason(t, authority.check(), reasonEvidenceExpired)
	view := authority.view()
	if view.fixturePermitValid || view.fence <= oldFence {
		t.Fatalf("expiry did not fence authority: %+v", view)
	}
	if status, _ := authority.ticketStatus(active); status != workInvalidated {
		t.Fatalf("active ticket status = %q, want %q", status, workInvalidated)
	}
	if status, _ := authority.ticketStatus(pending); status != workPending {
		t.Fatalf("unstarted ticket status = %q, want %q", status, workPending)
	}
	_, results := authority.fixtureCounts()
	if results != 0 {
		t.Fatalf("results committed after expiry = %d", results)
	}
}

func TestRevocationPolicyAndOwnerChangesWinDuringRenewal(t *testing.T) {
	for _, mode := range []string{"revoke", "policy", "owner"} {
		t.Run(mode, func(t *testing.T) {
			authority, clock, grant := newAuthorizedFixture(t)
			started := make(chan struct{})
			release := make(chan struct{})
			collector := func(ctx context.Context, challenge evidenceChallenge) (roleEvidence, error) {
				if challenge.role == readerRole {
					close(started)
					select {
					case <-release:
					case <-ctx.Done():
						return roleEvidence{}, ctx.Err()
					}
				}
				return fixtureEvidenceCollector(clock, grant, nil)(ctx, challenge)
			}
			result := make(chan error, 1)
			go func() { result <- authority.renew(context.Background(), collector) }()
			<-started
			switch mode {
			case "revoke":
				authority.revoke()
			case "policy":
				authority.policyChanged(5, 10, true)
			case "owner":
				_ = authority.ownerDrift(readerRole)
			}
			close(release)
			err := <-result
			if err == nil {
				t.Fatal("renewal published after concurrent invalidation")
			}
			if authority.view().fixturePermitValid {
				t.Fatal("concurrent invalidation left a permit active")
			}
		})
	}
}

func TestRenewalCancellationIsBoundedAndFencesAuthority(t *testing.T) {
	authority, clock, grant := newAuthorizedFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	collector := func(ctx context.Context, challenge evidenceChallenge) (roleEvidence, error) {
		if challenge.role == readerRole {
			close(started)
			<-ctx.Done()
			return roleEvidence{}, ctx.Err()
		}
		return fixtureEvidenceCollector(clock, grant, nil)(ctx, challenge)
	}
	result := make(chan error, 1)
	go func() { result <- authority.renew(ctx, collector) }()
	<-started
	cancel()
	err := <-result
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled renewal error = %v, want context cancellation", err)
	}
	if authority.view().fixturePermitValid {
		t.Fatal("cancelled renewal kept predecessor permit")
	}
}

func TestEvidenceReplayStaleTicketsAndRestartIsolation(t *testing.T) {
	clock := newFixtureClock()
	config := fixtureConfigForTest(true)
	first, grant := newFixtureAuthority(t, clock, config)
	if err := first.recordFixtureDerivation(fixtureReceiptForTest(config, grant, first.instanceID)); err != nil {
		t.Fatal(err)
	}
	savedEvidence := make(map[role]roleEvidence, 2)
	firstCollector := fixtureEvidenceCollector(clock, grant, nil)
	if err := first.authorize(context.Background(), func(ctx context.Context, challenge evidenceChallenge) (roleEvidence, error) {
		evidence, collectErr := firstCollector(ctx, challenge)
		savedEvidence[challenge.role] = evidence
		return evidence, collectErr
	}); err != nil {
		t.Fatal(err)
	}
	ticket, err := first.dispatch(fixtureCommand("synthetic-restart-work"))
	if err != nil {
		t.Fatal(err)
	}
	changedTicket := ticket
	changedTicket.evidenceVersion++
	requireReason(t, first.claim(changedTicket), reasonStaleTicket)

	second, secondGrant := newFixtureAuthority(t, clock, config)
	requireReason(t, second.recordFixtureDerivation(fixtureReceiptForTest(config, secondGrant, first.instanceID)), reasonInvalidEvidence)
	if err := second.recordFixtureDerivation(fixtureReceiptForTest(config, secondGrant, second.instanceID)); err != nil {
		t.Fatal(err)
	}
	requireReason(t, second.authorize(context.Background(), func(_ context.Context, challenge evidenceChallenge) (roleEvidence, error) {
		return savedEvidence[challenge.role], nil
	}), reasonInvalidEvidence)
	third, thirdGrant := newFixtureAuthority(t, clock, config)
	if err := third.recordFixtureDerivation(fixtureReceiptForTest(config, thirdGrant, third.instanceID)); err != nil {
		t.Fatal(err)
	}
	if err := third.authorize(context.Background(), fixtureEvidenceCollector(clock, thirdGrant, nil)); err != nil {
		t.Fatal(err)
	}
	requireReason(t, third.claim(ticket), reasonStaleTicket)
	if status, err := first.ticketStatus(ticket); err != nil || status != workPending {
		t.Fatalf("original instance ticket status = %q, err = %v", status, err)
	}

	// A new challenge cannot consume a vector saved from an earlier evidence
	// version, even when scope and owner are otherwise unchanged.
	requireReason(t, first.renew(context.Background(), func(_ context.Context, challenge evidenceChallenge) (roleEvidence, error) {
		return savedEvidence[challenge.role], nil
	}), reasonInvalidEvidence)
}

func TestCancelDuplicateResultsReaderLossAndIndependentMode(t *testing.T) {
	authority, _, _ := newAuthorizedFixture(t)
	requireReason(t, authority.requestIndependentCollector(), reasonIndependentCollector)
	cancelled, err := authority.dispatch(fixtureCommand("synthetic-cancelled-work"))
	if err != nil {
		t.Fatal(err)
	}
	if err := authority.claim(cancelled); err != nil {
		t.Fatal(err)
	}
	if err := authority.cancel(cancelled); err != nil {
		t.Fatal(err)
	}
	requireReason(t, authority.complete(cancelled, fixtureResultFor(cancelled)), reasonTicketCancelled)
	requireReason(t, authority.cancel(cancelled), reasonTicketCancelled)

	completed, err := authority.dispatch(fixtureCommand("synthetic-completed-work"))
	if err != nil {
		t.Fatal(err)
	}
	if err := authority.claim(completed); err != nil {
		t.Fatal(err)
	}
	if err := authority.complete(completed, fixtureResultFor(completed)); err != nil {
		t.Fatal(err)
	}
	requireReason(t, authority.complete(completed, fixtureResultFor(completed)), reasonDuplicateResult)
	authority.readerLost()
	if authority.view().fixturePermitValid {
		t.Fatal("last reader loss did not revoke dependent authority")
	}
	if status, _ := authority.ticketStatus(completed); status != workCompleted {
		t.Fatalf("already committed result status = %q", status)
	}
}

func TestMemorySinkAndRevokeShareTheAuthorityLock(t *testing.T) {
	authority, _, _ := newAuthorizedFixture(t)
	ticket, err := authority.dispatch(fixtureCommand("synthetic-atomic-work"))
	if err != nil {
		t.Fatal(err)
	}
	if err := authority.claim(ticket); err != nil {
		t.Fatal(err)
	}
	// A completed commit is valid if it wins the shared lock before revoke.
	if err := authority.complete(ticket, fixtureResultFor(ticket)); err != nil {
		t.Fatal(err)
	}
	authority.revoke()
	_, results := authority.fixtureCounts()
	if results != 1 {
		t.Fatalf("committed results before revoke = %d, want 1", results)
	}
	if authority.view().fixturePermitValid {
		t.Fatal("revoke did not close authority after the committed result")
	}

	blocked, clock, grant := newAuthorizedFixture(t)
	blockedTicket, err := blocked.dispatch(fixtureCommand("synthetic-revoke-first"))
	if err != nil {
		t.Fatal(err)
	}
	if err := blocked.claim(blockedTicket); err != nil {
		t.Fatal(err)
	}
	blocked.revoke()
	_ = clock
	_ = grant
	requireReason(t, blocked.complete(blockedTicket, fixtureResultFor(blockedTicket)), reasonRevoked)
	_, results = blocked.fixtureCounts()
	if results != 0 {
		t.Fatalf("result committed after revoke = %d", results)
	}
}

func TestMonotonicClockRegressionFencesPermit(t *testing.T) {
	authority, clock, _ := newAuthorizedFixture(t)
	oldFence := authority.view().fence
	clock.Set(clock.Now().Add(-time.Second))
	requireReason(t, authority.check(), reasonClockRegression)
	view := authority.view()
	if view.fixturePermitValid || view.fence <= oldFence || view.reason != reasonClockRegression {
		t.Fatalf("clock regression state = %+v", view)
	}
}

func reasonOf(err error) authorizationReason {
	if err == nil {
		return ""
	}
	var typed authorizationError
	if errors.As(err, &typed) {
		return typed.reason
	}
	return ""
}
