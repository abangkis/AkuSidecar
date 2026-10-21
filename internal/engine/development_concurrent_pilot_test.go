//go:build auth_session_dev && auth_session_fixture

package engine

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type pilotTestClock struct {
	mu  sync.Mutex
	now time.Time
}

func newPilotTestClock() *pilotTestClock {
	return &pilotTestClock{now: time.Date(2026, 10, 21, 10, 0, 0, 0, time.UTC)}
}

func (c *pilotTestClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *pilotTestClock) Advance(delta time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(delta)
}

func (c *pilotTestClock) Set(now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = now
}

func pilotGrantForTest() pilotFixtureGrant {
	return pilotFixtureGrant{
		id:                "synthetic-pilot-grant-a",
		allowOptIn:        true,
		allowSeedReceipt:  true,
		allowEvidence:     true,
		allowCollection:   true,
		allowResultCommit: true,
	}
}

func pilotConfigForTest() pilotConfig {
	created := time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)
	return pilotConfig{
		mode:                   concurrentPilotMode,
		contractID:             concurrentPilotContract,
		providerID:             concurrentPilotProvider,
		evidenceSchema:         concurrentPilotSchema,
		policyID:               concurrentPilotPolicy,
		policyEnabled:          true,
		explicitOptIn:          true,
		limitationAcknowledged: true,
		expectedAccountID:      "synthetic-account-a",
		scopeID:                "synthetic-scope-a",
		policyGeneration:       4,
		consentGeneration:      7,
		pair: pilotPair{
			id:                "synthetic-pair-a",
			attemptID:         "synthetic-attempt-a",
			controllerEpochID: "synthetic-controller-epoch-a",
			sidecarLifetimeID: "synthetic-sidecar-lifetime-a",
			generation:        12,
		},
		reader: pilotOwner{
			role:              pilotReaderRole,
			ownerID:           "synthetic-reader-owner-a",
			profileID:         "synthetic-reader-profile-a",
			directoryID:       "synthetic-reader-dir-a",
			processID:         "synthetic-reader-pid-a",
			processCreationID: "synthetic-reader-created-a",
			processImageID:    "synthetic-reader-image-a",
			processStartedAt:  created,
			targetID:          "synthetic-reader-target-a",
			frameID:           "synthetic-reader-frame-a",
			documentID:        "synthetic-reader-document-a",
			sidecarLifetimeID: "synthetic-sidecar-lifetime-a",
			contextID:         "synthetic-reader-context-a",
			epochID:           "synthetic-controller-epoch-a",
			generation:        3,
			driverID:          "synthetic-reader-driver-a",
			directoryPinned:   true,
			processAlive:      true,
			windowInventory:   []string{"synthetic-reader-window-a", "synthetic-reader-bootstrap-a"},
		},
		collector: pilotOwner{
			role:              pilotCollectorRole,
			ownerID:           "synthetic-collector-owner-a",
			profileID:         "synthetic-collector-profile-a",
			directoryID:       "synthetic-collector-dir-a",
			processID:         "synthetic-collector-pid-a",
			processCreationID: "synthetic-collector-created-a",
			processImageID:    "synthetic-collector-image-a",
			processStartedAt:  created.Add(time.Minute),
			targetID:          "synthetic-collector-target-a",
			frameID:           "synthetic-collector-frame-a",
			documentID:        "synthetic-collector-document-a",
			sidecarLifetimeID: "synthetic-sidecar-lifetime-a",
			contextID:         "synthetic-collector-context-a",
			epochID:           "synthetic-controller-epoch-a",
			generation:        5,
			driverID:          "synthetic-collector-driver-a",
			directoryPinned:   true,
			profileFresh:      true,
			disposableContext: true,
			processAlive:      true,
			windowInventory:   []string{"synthetic-collector-window-a"},
		},
	}
}

func pilotReceiptForTest(model *concurrentPilot, config pilotConfig, seededAt time.Time) pilotSeedReceipt {
	return pilotSeedReceipt{
		instanceID:        model.instanceID,
		id:                "synthetic-pilot-seed-receipt-a",
		contractID:        config.contractID,
		providerID:        config.providerID,
		evidenceSchema:    config.evidenceSchema,
		policyID:          config.policyID,
		policyGeneration:  config.policyGeneration,
		consentGeneration: config.consentGeneration,
		accountID:         config.expectedAccountID,
		pairID:            config.pair.id,
		attemptID:         config.pair.attemptID,
		pairGeneration:    config.pair.generation,
		readerOwnerID:     config.reader.ownerID,
		collectorOwnerID:  config.collector.ownerID,
		fromRole:          pilotReaderRole,
		toRole:            pilotCollectorRole,
		manifestID:        concurrentPilotManifest,
		seedApplied:       true,
		seededAt:          seededAt,
	}
}

func pilotEvidenceForChallenge(clock *pilotTestClock, challenge pilotChallenge) pilotRoleEvidence {
	now := clock.Now()
	return pilotRoleEvidence{
		instanceID:        challenge.instanceID,
		challengeID:       challenge.id,
		vectorID:          challenge.vectorID,
		contractID:        challenge.contractID,
		providerID:        challenge.providerID,
		evidenceSchema:    challenge.evidenceSchema,
		policyID:          challenge.policyID,
		policyGeneration:  challenge.policyGeneration,
		consentGeneration: challenge.consentGeneration,
		accountID:         challenge.accountID,
		pair:              challenge.pair,
		role:              challenge.role,
		owner:             clonePilotOwner(challenge.owner),
		operation:         challenge.operation,
		identitySourceID:  challenge.identitySourceID,
		identityOrigin:    challenge.identityOrigin,
		settingsMethod:    challenge.settingsMethod,
		settingsURL:       challenge.settingsURL,
		settingsRouteID:   challenge.settingsRouteID,
		settingsQualified: challenge.settingsQualified,
		targetID:          challenge.targetID,
		frameID:           challenge.frameID,
		documentID:        challenge.documentID,
		requestID:         challenge.requestID,
		acquisitionStart:  challenge.acquisitionStart,
		observedAt:        now,
		validUntil:        now.Add(30 * time.Second),
		identityPresent:   true,
		identityLive:      true,
		settingsSuccess:   true,
		authenticated:     true,
		cacheFresh:        true,
		serviceWorkerOK:   true,
		notRedirected:     true,
		wellFormed:        true,
		correlated:        true,
	}
}

func newPilotFixture(t *testing.T, mutate func(pilotChallenge, *pilotRoleEvidence), beforeReturn func(context.Context, pilotChallenge, *pilotTestClock)) (*concurrentPilot, *pilotTestClock, pilotConfig, *atomic.Int64) {
	t.Helper()
	clock := newPilotTestClock()
	config := pilotConfigForTest()
	var calls atomic.Int64
	source := func(ctx context.Context, challenge pilotChallenge) (pilotRoleEvidence, error) {
		calls.Add(1)
		if challenge.operation != pilotIdentityAndPinnedSettings || challenge.identitySourceID != concurrentPilotIdentitySource || challenge.identityOrigin != concurrentPilotIdentityOrigin || challenge.settingsMethod != concurrentPilotSettingsMethod || challenge.settingsURL != concurrentPilotSettingsURL || challenge.settingsRouteID != concurrentPilotSettingsRoute || challenge.settingsQualified != concurrentPilotSettingsQualification {
			return pilotRoleEvidence{}, errors.New("unqualified operation reached synthetic source")
		}
		if beforeReturn != nil {
			beforeReturn(ctx, challenge, clock)
		}
		evidence := pilotEvidenceForChallenge(clock, challenge)
		if mutate != nil {
			mutate(challenge, &evidence)
		}
		return evidence, nil
	}
	model, err := newConcurrentPilot(config, pilotGrantForTest(), clock.Now, source)
	if err != nil {
		t.Fatalf("newConcurrentPilot() error = %v", err)
	}
	return model, clock, config, &calls
}

func enablePilotFixture(t *testing.T, model *concurrentPilot, clock *pilotTestClock, config pilotConfig) {
	t.Helper()
	receipt := pilotReceiptForTest(model, config, clock.Now())
	if err := model.enable(context.Background(), receipt); err != nil {
		t.Fatalf("enable() error = %v", err)
	}
}

func requirePilotReason(t *testing.T, err error, want pilotReason) {
	t.Helper()
	var typed pilotError
	if !errors.As(err, &typed) || typed.reason != want {
		t.Fatalf("error reason = %v, want %q", err, want)
	}
}

func requirePilotRevoked(t *testing.T, model *concurrentPilot, reason pilotReason) {
	t.Helper()
	view := model.report()
	if !view.Revoked || view.AdmissionOpen || view.PilotEligible || view.Reason != string(reason) {
		t.Fatalf("pilot report after revoke = %+v, want revoked reason %q", view, reason)
	}
}

func TestConcurrentPilotDefaultOffAndUnsupportedModesHaveNoEffects(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*pilotConfig)
		want   pilotReason
	}{
		{name: "opt-in absent", mutate: func(c *pilotConfig) { c.explicitOptIn = false }, want: pilotReasonDisabled},
		{name: "policy disabled", mutate: func(c *pilotConfig) { c.policyEnabled = false }, want: pilotReasonDisabled},
		{name: "mode unrecognized", mutate: func(c *pilotConfig) { c.mode = "native" }, want: pilotReasonUnsupported},
		{name: "strict contract supplied", mutate: func(c *pilotConfig) { c.contractID = "aku.x.concurrent-authorization.v1" }, want: pilotReasonUnsupported},
		{name: "policy version mismatch", mutate: func(c *pilotConfig) { c.policyID = "x-dev-pilot.snapshot.v2" }, want: pilotReasonUnsupported},
		{name: "limitation omitted", mutate: func(c *pilotConfig) { c.limitationAcknowledged = false }, want: pilotReasonDisabled},
	} {
		t.Run(test.name, func(t *testing.T) {
			model, clock, config, calls := newPilotFixture(t, nil, nil)
			test.mutate(&config)
			model.config = clonePilotConfig(config)
			receipt := pilotReceiptForTest(model, config, clock.Now())
			err := model.enable(context.Background(), receipt)
			requirePilotReason(t, err, test.want)
			if calls.Load() != 0 || model.report().Counters.SeedReceipts != 0 || model.resultCount() != 0 {
				t.Fatalf("closed/unsupported configuration had effects: calls=%d view=%+v", calls.Load(), model.report())
			}
			view := model.report()
			if view.CanAuthorize || view.NativeQualified || view.ProductionQualified || view.StorePermit || view.PilotEligible {
				t.Fatalf("closed configuration exposed authority: %+v", view)
			}
		})
	}
	if _, err := newConcurrentPilot(pilotConfigForTest(), pilotFixtureGrant{}, newPilotTestClock().Now, func(context.Context, pilotChallenge) (pilotRoleEvidence, error) { return pilotRoleEvidence{}, nil }); err == nil {
		t.Fatal("invalid fixture grant unexpectedly created pilot")
	} else {
		requirePilotReason(t, err, pilotReasonFixtureGrant)
	}
}

func TestConcurrentPilotLongCaptureKeepsTicketAndCollectorAcrossRenewals(t *testing.T) {
	model, clock, config, calls := newPilotFixture(t, nil, nil)
	enablePilotFixture(t, model, clock, config)
	ticket, err := model.dispatch("synthetic-command-capture-a")
	if err != nil {
		t.Fatalf("dispatch() error = %v", err)
	}
	if err := model.claim(ticket); err != nil {
		t.Fatalf("claim() error = %v", err)
	}
	for i := 0; i < 4; i++ {
		clock.Advance(3 * time.Second)
		if err := model.renew(context.Background()); err != nil {
			t.Fatalf("renew %d error = %v", i+1, err)
		}
	}
	if clock.Now().Sub(model.attemptStart) != 12*time.Second {
		t.Fatalf("capture duration = %s, want 12s", clock.Now().Sub(model.attemptStart))
	}
	if ticket.evidenceVersion != 1 || model.report().EvidenceVersion != 5 {
		t.Fatalf("ticket/current versions = %d/%d, want original 1/current 5", ticket.evidenceVersion, model.report().EvidenceVersion)
	}
	if ticket.collectorPID != config.collector.processID || ticket.collectorGen != config.collector.generation || ticket.collectorDriver != config.collector.driverID || ticket.collectorContext != config.collector.contextID {
		t.Fatalf("collector stamp changed in ticket: %+v", ticket)
	}
	if model.permit.readerEvidence.documentID != config.reader.documentID || model.permit.collectorEvidence.documentID != config.collector.documentID {
		t.Fatalf("same-document renewal changed a pinned document: reader=%q collector=%q", model.permit.readerEvidence.documentID, model.permit.collectorEvidence.documentID)
	}
	if model.permit.readerEvidence.requestID != model.permit.collectorEvidence.requestID {
		t.Fatalf("same lexical request ID should be valid across pinned role contexts: reader=%q collector=%q", model.permit.readerEvidence.requestID, model.permit.collectorEvidence.requestID)
	}
	result := pilotResult{
		id:              "synthetic-result-capture-a",
		ticketID:        ticket.id,
		commandID:       ticket.commandID,
		scopeID:         ticket.scopeID,
		sourceID:        ticket.sourceID,
		operation:       ticket.operation,
		evidenceVersion: ticket.evidenceVersion,
		pairID:          ticket.pairID,
		pairGeneration:  ticket.pairGeneration,
		fence:           ticket.fence,
		collectorOwner:  ticket.collectorOwnerID,
		content:         "synthetic capture result",
	}
	if err := model.acceptResult(ticket, result); err != nil {
		t.Fatalf("acceptResult() error = %v", err)
	}
	if model.resultCount() != 1 || calls.Load() != 10 {
		t.Fatalf("sink/source counts = %d/%d, want 1/10", model.resultCount(), calls.Load())
	}
	if status, _ := model.ticketStatus(ticket); status != pilotWorkCompleted {
		t.Fatalf("ticket status = %q", status)
	}
	view := model.report()
	if !view.PilotEligible || view.Continuity != "unknown" || view.GlobalCoverage != "partial" || view.RefreshQualification != "not_qualified" || view.Counters.EvidenceVectors != 5 || view.Counters.AcceptedResults != 1 {
		t.Fatalf("pilot result did not retain bounded semantics: %+v", view)
	}
}

func TestConcurrentPilotRequestIDsAreScopedToRoleAndOwnerContext(t *testing.T) {
	model, clock, config, _ := newPilotFixture(t, nil, nil)
	enablePilotFixture(t, model, clock, config)
	readerEvidence := model.permit.readerEvidence
	collectorEvidence := model.permit.collectorEvidence
	if readerEvidence.requestID != collectorEvidence.requestID {
		t.Fatalf("fixture request IDs differ across independent contexts: %q / %q", readerEvidence.requestID, collectorEvidence.requestID)
	}
	readerKey := pilotRequestUseKey(pilotReaderRole, config.reader, readerEvidence.requestID)
	collectorKey := pilotRequestUseKey(pilotCollectorRole, config.collector, collectorEvidence.requestID)
	if readerKey == collectorKey {
		t.Fatal("same lexical request ID in distinct role contexts collided")
	}

	// Simulate a request-sequence replay while keeping a fresh challenge ID.
	// The evidence remains challenge-bound, so only the one-use redemption
	// ledger should reject this same-role reuse.
	model.vectorSeq = 0
	requirePilotReason(t, model.renew(context.Background()), pilotReasonEvidenceInvalid)
	requirePilotRevoked(t, model, pilotReasonEvidenceInvalid)
	if model.report().EvidenceVersion != 1 {
		t.Fatalf("same-role replay advanced evidence version: %+v", model.report())
	}
}

func TestConcurrentPilotAttemptLifetimeAndSourceDeadlinesAreIndependent(t *testing.T) {
	t.Run("sixty-second absolute cap", func(t *testing.T) {
		model, clock, config, _ := newPilotFixture(t, nil, nil)
		clock.Advance(time.Second) // Seed/evidence starts after the attempt clock.
		enablePilotFixture(t, model, clock, config)
		for clock.Now().Before(model.attemptStop.Add(-4 * time.Second)) {
			clock.Advance(4 * time.Second)
			if err := model.renew(context.Background()); err != nil {
				t.Fatalf("renew before cap error = %v at %s", err, clock.Now())
			}
		}
		clock.Advance(model.attemptStop.Sub(clock.Now()) - time.Second)
		if err := model.renew(context.Background()); err != nil {
			t.Fatalf("renew at 59s error = %v", err)
		}
		if !clock.Now().Before(model.attemptStop) {
			t.Fatalf("test advanced beyond cap before final check: %s", clock.Now())
		}
		clock.Set(model.attemptStop)
		view := model.report()
		if !view.AttemptExpired || view.PilotEligible || view.EvidenceVersion > 20 {
			t.Fatalf("renewals extended the absolute attempt: %+v", view)
		}
		requirePilotReason(t, model.renew(context.Background()), pilotReasonAttemptExpired)
	})

	t.Run("source validity is the earliest deadline", func(t *testing.T) {
		model, clock, config, _ := newPilotFixture(t, func(_ pilotChallenge, evidence *pilotRoleEvidence) {
			evidence.validUntil = evidence.observedAt.Add(time.Second)
		}, nil)
		enablePilotFixture(t, model, clock, config)
		if !model.permit.expiresAt.Equal(clock.Now().Add(time.Second)) {
			t.Fatalf("permit expiry = %s, want source deadline %s", model.permit.expiresAt, clock.Now().Add(time.Second))
		}
		clock.Advance(time.Second)
		_, err := model.dispatch("synthetic-command-expired")
		requirePilotReason(t, err, pilotReasonEvidenceExpired)
	})

	t.Run("five-second evidence age", func(t *testing.T) {
		model, clock, config, _ := newPilotFixture(t, func(_ pilotChallenge, evidence *pilotRoleEvidence) {
			evidence.validUntil = evidence.observedAt.Add(time.Minute)
		}, nil)
		enablePilotFixture(t, model, clock, config)
		if !model.permit.expiresAt.Equal(clock.Now().Add(concurrentPilotMaxAge)) {
			t.Fatalf("permit expiry = %s, want five-second age cap", model.permit.expiresAt)
		}
		clock.Advance(concurrentPilotMaxAge)
		_, err := model.dispatch("synthetic-command-stale")
		requirePilotReason(t, err, pilotReasonEvidenceExpired)
	})

	t.Run("two-second joint vector deadline", func(t *testing.T) {
		model, clock, config, _ := newPilotFixture(t, nil, func(_ context.Context, challenge pilotChallenge, clock *pilotTestClock) {
			if challenge.role == pilotCollectorRole && challenge.previousVersion == 0 {
				clock.Advance(2100 * time.Millisecond)
			}
		})
		receipt := pilotReceiptForTest(model, config, clock.Now())
		requirePilotReason(t, model.enable(context.Background(), receipt), pilotReasonJointDeadline)
		requirePilotRevoked(t, model, pilotReasonJointDeadline)
	})

	t.Run("exact joint deadline is rejected", func(t *testing.T) {
		model, clock, config, _ := newPilotFixture(t, nil, func(_ context.Context, challenge pilotChallenge, clock *pilotTestClock) {
			if challenge.role == pilotCollectorRole && challenge.previousVersion == 0 {
				clock.Advance(concurrentPilotJointMax)
			}
		})
		receipt := pilotReceiptForTest(model, config, clock.Now())
		requirePilotReason(t, model.enable(context.Background(), receipt), pilotReasonJointDeadline)
	})

	t.Run("clock rollback", func(t *testing.T) {
		model, clock, config, _ := newPilotFixture(t, nil, nil)
		enablePilotFixture(t, model, clock, config)
		clock.Set(clock.Now().Add(-time.Nanosecond))
		_, err := model.dispatch("synthetic-command-rollback")
		requirePilotReason(t, err, pilotReasonClockRegression)
		requirePilotRevoked(t, model, pilotReasonClockRegression)
	})
}

func TestConcurrentPilotRejectsChallengeReplayRoleAndOwnerMismatch(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(pilotChallenge, *pilotRoleEvidence)
	}{
		{name: "missing identity proof", mutate: func(ch pilotChallenge, ev *pilotRoleEvidence) {
			if ch.role == pilotReaderRole {
				ev.identityPresent = false
			}
		}},
		{name: "cross-role proof", mutate: func(ch pilotChallenge, ev *pilotRoleEvidence) {
			ev.role = pilotCollectorRole
			if ch.role == pilotCollectorRole {
				ev.role = pilotReaderRole
			}
		}},
		{name: "replayed challenge", mutate: func(_ pilotChallenge, ev *pilotRoleEvidence) { ev.challengeID = "synthetic-old-challenge" }},
		{name: "replayed request", mutate: func(_ pilotChallenge, ev *pilotRoleEvidence) { ev.requestID = "synthetic-old-request" }},
		{name: "cross-instance proof", mutate: func(_ pilotChallenge, ev *pilotRoleEvidence) { ev.instanceID = "synthetic-other-instance" }},
		{name: "owner mismatch", mutate: func(_ pilotChallenge, ev *pilotRoleEvidence) { ev.owner.processID = "synthetic-other-process" }},
		{name: "pair mismatch", mutate: func(_ pilotChallenge, ev *pilotRoleEvidence) { ev.pair.generation++ }},
	} {
		t.Run(test.name, func(t *testing.T) {
			model, clock, config, calls := newPilotFixture(t, test.mutate, nil)
			receipt := pilotReceiptForTest(model, config, clock.Now())
			err := model.enable(context.Background(), receipt)
			if err == nil || calls.Load() == 0 {
				t.Fatalf("invalid proof accepted, calls=%d", calls.Load())
			}
			view := model.report()
			if view.PilotEligible || !view.Revoked || view.EvidenceVersion != 0 {
				t.Fatalf("invalid proof left authority: %+v", view)
			}
		})
	}
}

func TestConcurrentPilotValidatesLiveIdentityTargetFrameAndPinnedRoute(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(pilotChallenge, *pilotRoleEvidence)
		want   pilotReason
	}{
		{name: "stale bootstrap", mutate: func(_ pilotChallenge, ev *pilotRoleEvidence) { ev.identityLive = false }, want: pilotReasonIdentityMissing},
		{name: "cache marker", mutate: func(_ pilotChallenge, ev *pilotRoleEvidence) { ev.cacheFresh = false }, want: pilotReasonEvidenceInvalid},
		{name: "service worker", mutate: func(_ pilotChallenge, ev *pilotRoleEvidence) { ev.serviceWorkerOK = false }, want: pilotReasonEvidenceInvalid},
		{name: "redirected response", mutate: func(_ pilotChallenge, ev *pilotRoleEvidence) { ev.notRedirected = false }, want: pilotReasonEvidenceInvalid},
		{name: "malformed response", mutate: func(_ pilotChallenge, ev *pilotRoleEvidence) { ev.wellFormed = false }, want: pilotReasonEvidenceInvalid},
		{name: "lost correlation", mutate: func(_ pilotChallenge, ev *pilotRoleEvidence) { ev.correlated = false }, want: pilotReasonEvidenceInvalid},
		{name: "wrong target", mutate: func(_ pilotChallenge, ev *pilotRoleEvidence) { ev.targetID = "synthetic-wrong-target" }, want: pilotReasonEvidenceInvalid},
		{name: "wrong frame", mutate: func(_ pilotChallenge, ev *pilotRoleEvidence) { ev.frameID = "synthetic-wrong-frame" }, want: pilotReasonEvidenceInvalid},
		{name: "wrong source route", mutate: func(_ pilotChallenge, ev *pilotRoleEvidence) { ev.settingsURL = "https://api.x.com/other" }, want: pilotReasonEvidenceInvalid},
		{name: "unqualified route", mutate: func(_ pilotChallenge, ev *pilotRoleEvidence) { ev.settingsQualified = "synthetic-unreviewed" }, want: pilotReasonEvidenceInvalid},
		{name: "identity unavailable", mutate: func(_ pilotChallenge, ev *pilotRoleEvidence) { ev.identityPresent = false }, want: pilotReasonIdentityMissing},
		{name: "settings rejected", mutate: func(_ pilotChallenge, ev *pilotRoleEvidence) { ev.settingsSuccess = false }, want: pilotReasonAuthentication},
		{name: "account mismatch", mutate: func(_ pilotChallenge, ev *pilotRoleEvidence) { ev.accountID = "synthetic-different-account" }, want: pilotReasonAccountMismatch},
	} {
		t.Run(test.name, func(t *testing.T) {
			model, clock, config, _ := newPilotFixture(t, test.mutate, nil)
			receipt := pilotReceiptForTest(model, config, clock.Now())
			requirePilotReason(t, model.enable(context.Background(), receipt), test.want)
			requirePilotRevoked(t, model, test.want)
		})
	}
}

func TestConcurrentPilotUnknownCoverageAndUnclassifiedWritesStayDiagnostic(t *testing.T) {
	model, clock, config, _ := newPilotFixture(t, nil, nil)
	enablePilotFixture(t, model, clock, config)
	if err := model.observeEvent(pilotEventUnclassifiedCookieWrite); err != nil {
		t.Fatalf("unclassified write error = %v", err)
	}
	if err := model.windowOpened("synthetic-reader-window-c"); err != nil {
		t.Fatalf("tracked window open error = %v", err)
	}
	if err := model.windowClosed("synthetic-reader-bootstrap-a"); err != nil {
		t.Fatalf("non-final window close error = %v", err)
	}
	view := model.report()
	if !view.PilotEligible || view.Continuity != "unknown" || view.GlobalCoverage != "partial" || view.Counters.UnclassifiedCookieWrites != 1 || view.Counters.ReaderWindowOpens != 1 || view.Counters.ReaderWindowCloses != 1 {
		t.Fatalf("diagnostic event altered bounded eligibility: %+v", view)
	}
}

func TestConcurrentPilotRecognizedMutationAndKnownGapsRevoke(t *testing.T) {
	for _, test := range []struct {
		name  string
		event pilotEvent
		want  pilotReason
	}{
		{name: "auth_token write", event: pilotEventAuthTokenWrite, want: pilotReasonCookieMutation},
		{name: "ct0 write", event: pilotEventCT0Write, want: pilotReasonCookieMutation},
		{name: "detach", event: pilotEventChannelDetached, want: pilotReasonChannelLost},
		{name: "crash", event: pilotEventChannelCrashed, want: pilotReasonChannelLost},
		{name: "overflow", event: pilotEventChannelOverflow, want: pilotReasonChannelLost},
		{name: "ambiguous correlation", event: pilotEventCorrelationAmbiguous, want: pilotReasonChannelLost},
		{name: "identity drift", event: pilotEventIdentityChanged, want: pilotReasonAccountMismatch},
		{name: "auth rejection", event: pilotEventAuthenticationRejected, want: pilotReasonAuthentication},
		{name: "cleanup uncertainty", event: pilotEventCleanupUncertain, want: pilotReasonOwnerDrift},
	} {
		t.Run(test.name, func(t *testing.T) {
			model, clock, config, _ := newPilotFixture(t, nil, nil)
			enablePilotFixture(t, model, clock, config)
			requirePilotReason(t, model.observeEvent(test.event), test.want)
			requirePilotRevoked(t, model, test.want)
		})
	}
}

func TestConcurrentPilotTicketFreshnessCancellationAndDuplicateResult(t *testing.T) {
	t.Run("duplicate result", func(t *testing.T) {
		model, clock, config, _ := newPilotFixture(t, nil, nil)
		enablePilotFixture(t, model, clock, config)
		ticket, err := model.dispatch("synthetic-command-duplicate")
		if err != nil {
			t.Fatal(err)
		}
		if err := model.claim(ticket); err != nil {
			t.Fatal(err)
		}
		result := pilotResult{id: "synthetic-result-duplicate", ticketID: ticket.id, commandID: ticket.commandID, scopeID: ticket.scopeID, sourceID: ticket.sourceID, operation: ticket.operation, evidenceVersion: ticket.evidenceVersion, pairID: ticket.pairID, pairGeneration: ticket.pairGeneration, fence: ticket.fence, collectorOwner: ticket.collectorOwnerID, content: "synthetic"}
		if err := model.acceptResult(ticket, result); err != nil {
			t.Fatal(err)
		}
		requirePilotReason(t, model.acceptResult(ticket, result), pilotReasonDuplicateResult)
		if model.resultCount() != 1 {
			t.Fatalf("result sink count = %d", model.resultCount())
		}
		_ = clock
	})

	t.Run("cancelled pending work", func(t *testing.T) {
		model, clock, config, _ := newPilotFixture(t, nil, nil)
		enablePilotFixture(t, model, clock, config)
		ticket, err := model.dispatch("synthetic-command-cancel")
		if err != nil {
			t.Fatal(err)
		}
		if err := model.cancel(ticket); err != nil {
			t.Fatal(err)
		}
		requirePilotReason(t, model.claim(ticket), pilotReasonTicketCancelled)
		_, err = model.dispatch(ticket.commandID)
		requirePilotReason(t, err, pilotReasonDuplicateCommand)
		if model.resultCount() != 0 {
			t.Fatal("cancelled work reached result sink")
		}
	})

	t.Run("stale checkpoint", func(t *testing.T) {
		model, clock, config, _ := newPilotFixture(t, nil, nil)
		enablePilotFixture(t, model, clock, config)
		ticket, err := model.dispatch("synthetic-command-stale-result")
		if err != nil {
			t.Fatal(err)
		}
		if err := model.claim(ticket); err != nil {
			t.Fatal(err)
		}
		clock.Advance(concurrentPilotMaxAge)
		result := pilotResult{id: "synthetic-result-stale", ticketID: ticket.id, commandID: ticket.commandID, scopeID: ticket.scopeID, sourceID: ticket.sourceID, operation: ticket.operation, evidenceVersion: ticket.evidenceVersion, pairID: ticket.pairID, pairGeneration: ticket.pairGeneration, fence: ticket.fence, collectorOwner: ticket.collectorOwnerID, content: "synthetic"}
		requirePilotReason(t, model.acceptResult(ticket, result), pilotReasonEvidenceExpired)
		if model.resultCount() != 0 {
			t.Fatal("stale work reached result sink")
		}
	})
}

func TestConcurrentPilotExternalCancellationWinsAfterBlockedSourceReturns(t *testing.T) {
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	var block atomic.Bool
	model, clock, config, _ := newPilotFixture(t, nil, func(_ context.Context, _ pilotChallenge, _ *pilotTestClock) {
		if block.CompareAndSwap(true, false) {
			entered <- struct{}{}
			<-release
		}
	})
	enablePilotFixture(t, model, clock, config)
	block.Store(true)
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	renewed := make(chan error, 1)
	go func() { renewed <- model.renew(parent) }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("renewal source did not block")
	}
	cancel()
	close(release)
	select {
	case err := <-renewed:
		requirePilotReason(t, err, pilotReasonSourceUnavailable)
	case <-time.After(time.Second):
		t.Fatal("cancelled renewal did not return")
	}
	requirePilotRevoked(t, model, pilotReasonSourceUnavailable)
	if model.report().EvidenceVersion != 1 {
		t.Fatalf("cancelled vector published: %+v", model.report())
	}
}

func TestConcurrentPilotCancellationWhileWaitingForPublicationLock(t *testing.T) {
	model, clock, config, _ := newPilotFixture(t, nil, nil)
	enablePilotFixture(t, model, clock, config)
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	publicationReady := make(chan struct{})
	releasePublication := make(chan struct{})
	renewed := make(chan error, 1)
	go func() {
		renewed <- model.renewVectorWithHook(parent, false, func() {
			close(publicationReady) // Both source calls have completed.
			<-releasePublication
		})
	}()
	select {
	case <-publicationReady:
	case <-time.After(time.Second):
		t.Fatal("renewal did not reach the publication boundary")
	}
	model.mu.Lock()
	close(releasePublication)
	cancel() // The source has returned; final publication is blocked on this mutex.
	model.mu.Unlock()
	select {
	case err := <-renewed:
		requirePilotReason(t, err, pilotReasonSourceUnavailable)
	case <-time.After(time.Second):
		t.Fatal("blocked publication did not return")
	}
	requirePilotRevoked(t, model, pilotReasonSourceUnavailable)
	if model.report().EvidenceVersion != 1 {
		t.Fatalf("cancelled publication advanced evidence version: %+v", model.report())
	}
}

func TestConcurrentPilotNonFinalWindowChangesDoNotStrandBlockedRenewal(t *testing.T) {
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	var block atomic.Bool
	model, clock, config, _ := newPilotFixture(t, nil, func(_ context.Context, _ pilotChallenge, _ *pilotTestClock) {
		if block.CompareAndSwap(true, false) {
			entered <- struct{}{}
			<-release
		}
	})
	enablePilotFixture(t, model, clock, config)
	ticket, err := model.dispatch("synthetic-command-window-race")
	if err != nil {
		t.Fatalf("dispatch() error = %v", err)
	}
	if err := model.claim(ticket); err != nil {
		t.Fatalf("claim() error = %v", err)
	}
	block.Store(true)
	renewed := make(chan error, 1)
	go func() { renewed <- model.renew(context.Background()) }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("renewal source did not block")
	}
	if err := model.windowOpened("synthetic-reader-window-during-renewal"); err != nil {
		t.Fatalf("non-final window open during renewal error = %v", err)
	}
	if err := model.windowClosed("synthetic-reader-bootstrap-a"); err != nil {
		t.Fatalf("non-final window close during renewal error = %v", err)
	}
	close(release)
	select {
	case err := <-renewed:
		if err != nil {
			t.Fatalf("renewal after normal inventory change error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("renewal remained busy after normal inventory change")
	}
	view := model.report()
	if !view.PilotEligible || view.EvidenceVersion != 2 || ticket.evidenceVersion != 1 || model.permit.collectorOwner.ownerID != ticket.collectorOwnerID {
		t.Fatalf("normal inventory changes stranded or replaced the permit/ticket: view=%+v ticket=%+v permit=%+v", view, ticket, model.permit)
	}
	if ticket.collectorPID != config.collector.processID || ticket.collectorGen != config.collector.generation || ticket.collectorDriver != config.collector.driverID || ticket.collectorContext != config.collector.contextID {
		t.Fatalf("collector ticket stamp changed after inventory update: %+v", ticket)
	}
}

func TestConcurrentPilotDispatchUsesValidatedClockReading(t *testing.T) {
	model, clock, config, _ := newPilotFixture(t, nil, nil)
	enablePilotFixture(t, model, clock, config)
	base := clock.Now()
	var reads atomic.Int64
	model.clock = func() time.Time { return base.Add(time.Duration(reads.Add(1)) * time.Millisecond) }
	ticket, err := model.dispatch("synthetic-command-clock-stamp")
	if err != nil {
		t.Fatalf("dispatch() error = %v", err)
	}
	if reads.Load() != 1 || !ticket.dispatchAt.Equal(model.lastClock) {
		t.Fatalf("dispatch clock read was not the validated linearization point: reads=%d dispatchAt=%s lastClock=%s", reads.Load(), ticket.dispatchAt, model.lastClock)
	}
}

func TestConcurrentPilotBlockedRenewalCannotOutliveInvalidation(t *testing.T) {
	for _, test := range []struct {
		name       string
		invalidate func(*concurrentPilot)
		want       pilotReason
	}{
		{name: "opt out", invalidate: func(m *concurrentPilot) { m.optOut() }, want: pilotReasonOptedOut},
		{name: "last reader close", invalidate: func(m *concurrentPilot) {
			if err := m.windowClosed("synthetic-reader-window-a"); err != nil {
				panic(err)
			}
			_ = m.windowClosed("synthetic-reader-bootstrap-a")
		}, want: pilotReasonReaderLost},
		{name: "parent exit", invalidate: func(m *concurrentPilot) { m.parentExit() }, want: pilotReasonParentExited},
		{name: "policy drift", invalidate: func(m *concurrentPilot) { m.policyChanged(5, 8, true) }, want: pilotReasonPolicyChanged},
		{name: "owner drift", invalidate: func(m *concurrentPilot) {
			owner := clonePilotOwner(m.reader)
			owner.processID = "synthetic-new-reader-process"
			m.ownerChanged(pilotReaderRole, owner)
		}, want: pilotReasonOwnerDrift},
	} {
		t.Run(test.name, func(t *testing.T) {
			entered := make(chan struct{}, 1)
			release := make(chan struct{})
			var block atomic.Bool
			model, clock, config, _ := newPilotFixture(t, nil, func(ctx context.Context, challenge pilotChallenge, _ *pilotTestClock) {
				if block.CompareAndSwap(true, false) {
					entered <- struct{}{}
					<-release // Deliberately return after cancellation to test final fencing.
					if ctx.Err() == nil {
						t.Error("source context was not cancelled by invalidation")
					}
				}
				_ = challenge
			})
			enablePilotFixture(t, model, clock, config)
			block.Store(true)
			renewed := make(chan error, 1)
			go func() { renewed <- model.renew(context.Background()) }()
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("renewal source did not block")
			}
			test.invalidate(model)
			close(release)
			select {
			case err := <-renewed:
				if err == nil {
					t.Fatal("late renewal published after invalidation")
				}
			case <-time.After(time.Second):
				t.Fatal("blocked renewal did not finish")
			}
			requirePilotRevoked(t, model, test.want)
			if model.report().EvidenceVersion != 1 {
				t.Fatalf("late renewal advanced version: %+v", model.report())
			}
		})
	}
}

func TestConcurrentPilotSeedReplayAndWindowCleanupRules(t *testing.T) {
	t.Run("missing and replayed receipt", func(t *testing.T) {
		model, clock, config, calls := newPilotFixture(t, nil, nil)
		requirePilotReason(t, model.enable(context.Background(), pilotSeedReceipt{}), pilotReasonMissingSeed)
		if calls.Load() != 0 {
			t.Fatalf("missing receipt reached evidence source %d times", calls.Load())
		}
		receipt := pilotReceiptForTest(model, config, clock.Now())
		if err := model.enable(context.Background(), receipt); err != nil {
			t.Fatal(err)
		}
		requirePilotReason(t, model.enable(context.Background(), receipt), pilotReasonSeedReplay)
		if model.report().Counters.SeedReceipts != 1 || calls.Load() != 2 {
			t.Fatalf("receipt replay had effects: %+v calls=%d", model.report(), calls.Load())
		}
	})

	t.Run("future-dated receipt", func(t *testing.T) {
		model, clock, config, calls := newPilotFixture(t, nil, nil)
		receipt := pilotReceiptForTest(model, config, clock.Now().Add(time.Second))
		requirePilotReason(t, model.enable(context.Background(), receipt), pilotReasonSeedMismatch)
		if calls.Load() != 0 || model.report().Counters.SeedReceipts != 0 {
			t.Fatalf("future receipt reached evidence source: calls=%d report=%+v", calls.Load(), model.report())
		}
	})

	t.Run("opt out retains reader windows", func(t *testing.T) {
		model, clock, config, _ := newPilotFixture(t, nil, nil)
		enablePilotFixture(t, model, clock, config)
		model.optOut()
		if len(model.reader.windowInventory) != len(config.reader.windowInventory) || model.collector.processAlive || model.report().Reason != string(pilotReasonOptedOut) {
			t.Fatalf("opt-out cleanup mismatch reader=%v collectorAlive=%v report=%+v", model.reader.windowInventory, model.collector.processAlive, model.report())
		}
	})

	t.Run("parent exit closes both modeled owners", func(t *testing.T) {
		model, clock, config, _ := newPilotFixture(t, nil, nil)
		enablePilotFixture(t, model, clock, config)
		model.parentExit()
		if model.reader.processAlive || model.collector.processAlive || len(model.reader.windowInventory) != 0 || len(model.collector.windowInventory) != 0 {
			t.Fatalf("parent exit left modeled owners: reader=%+v collector=%+v", model.reader, model.collector)
		}
	})
}

func TestConcurrentPilotReportIsFixedAndPrivacyBounded(t *testing.T) {
	const secret = "private-secret-marker"
	model, clock, config, _ := newPilotFixture(t, nil, nil)
	enablePilotFixture(t, model, clock, config)
	model.config.expectedAccountID = "synthetic-account-" + secret
	if err := model.observeEvent(pilotEventUnclassifiedCookieWrite); err != nil {
		t.Fatal(err)
	}
	if err := model.observeEvent(pilotEvent("raw-header-" + secret)); err == nil {
		t.Fatal("unknown event accepted")
	}
	if got := model.report().Reason; got != "" {
		t.Fatalf("unknown event polluted report: %q", got)
	}
	encoded, err := json.Marshal(model.report())
	if err != nil {
		t.Fatal(err)
	}
	serialized := string(encoded)
	for _, forbidden := range []string{secret, config.expectedAccountID, model.reader.ownerID, "targetID", "documentID", "requestID", "cookie", "raw-header"} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("report leaked %q: %s", forbidden, serialized)
		}
	}
	var payload map[string]any
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatal(err)
	}
	wantKeys := []string{"contractId", "mode", "policyId", "evidenceVersion", "pilotEligible", "admissionOpen", "canAuthorize", "nativeQualified", "productionQualified", "storePermit", "snapshotFreshness", "globalCoverage", "continuity", "refreshQualification", "attemptExpired", "revoked", "reason", "counters"}
	if payload["contractId"] != concurrentPilotContract {
		t.Fatalf("report contract ID = %v", payload["contractId"])
	}
	if len(payload) != len(wantKeys) {
		t.Fatalf("report keys = %v", payload)
	}
	for _, key := range wantKeys {
		if _, ok := payload[key]; !ok {
			t.Fatalf("report missing key %q: %v", key, payload)
		}
	}
	counters, ok := payload["counters"].(map[string]any)
	if !ok || counters["seedReceipts"] != float64(1) || counters["evidenceVectors"] != float64(1) || counters["unclassifiedCookieWrites"] != float64(1) {
		t.Fatalf("fixed counters did not serialize: %v", payload["counters"])
	}
	if model.config.expectedAccountID == config.expectedAccountID {
		t.Fatal("privacy sentinel was not applied")
	}
}

func TestConcurrentPilotSanitizesArbitrarySourceErrors(t *testing.T) {
	for _, sourceErr := range []error{errors.New("token=" + "private-secret-marker"), pilotError{reason: pilotReason("private-secret-marker")}} {
		clock := newPilotTestClock()
		config := pilotConfigForTest()
		source := func(context.Context, pilotChallenge) (pilotRoleEvidence, error) {
			return pilotRoleEvidence{}, sourceErr
		}
		model, err := newConcurrentPilot(config, pilotGrantForTest(), clock.Now, source)
		if err != nil {
			t.Fatal(err)
		}
		receipt := pilotReceiptForTest(model, config, clock.Now())
		err = model.enable(context.Background(), receipt)
		requirePilotReason(t, err, pilotReasonSourceUnavailable)
		report, marshalErr := json.Marshal(model.report())
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		if strings.Contains(string(report), "private-secret-marker") {
			t.Fatalf("source error leaked: %s", report)
		}
	}
}
