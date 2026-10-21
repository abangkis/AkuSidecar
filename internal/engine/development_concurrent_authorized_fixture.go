//go:build auth_session_dev && auth_session_fixture

package engine

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

const fixtureReaderRevision = "synthetic:fixture-reader-v1"
const fixtureCollectorRevision = "synthetic:fixture-collector-v1"
const fixtureDatabaseScope = "synthetic:fixture-database"

func integrationFixtureGrant() fixtureGrant {
	return fixtureGrant{id: "synthetic-integration-grant", allowAuthorization: true, allowSeed: true, allowCollection: true, allowRenewal: true, allowResultSink: true}
}

// This source owns every session writer for the invented loopback service.
// A generation change or coverage gap permanently invalidates this enrollment.
// Browser probes supply authentication/identity; this local witness supplies
// only continuous lifecycle coverage between those probes and a commit barrier.
type controlledSyntheticLifecycle struct {
	mu           sync.Mutex
	account      string
	generation   uint64
	gap          bool
	started      time.Time
	pins         map[role]evidenceChallenge
	delayRenewal bool
	failRenewal  bool
}

func newControlledSyntheticLifecycle(account string) *controlledSyntheticLifecycle {
	return &controlledSyntheticLifecycle{account: account, generation: 1, started: time.Now(), pins: make(map[role]evidenceChallenge)}
}

func fixtureRevision(r role) string {
	if r == readerRole {
		return mapSyntheticIdentity("reader-revision", fixtureReaderRevision)
	}
	return mapSyntheticIdentity("collector-revision", fixtureCollectorRevision)
}

func (s *controlledSyntheticLifecycle) observe(ch evidenceChallenge, observed time.Time, actualAccount string) (roleEvidence, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.gap || s.generation != 1 || s.account != actualAccount || ch.accountID != mapSyntheticIdentity("account", "synthetic:account:"+actualAccount) ||
		ch.databaseScope != mapSyntheticIdentity("database", fixtureDatabaseScope) || ch.sourceID != fixtureSourceID || observed.Before(s.started) {
		return roleEvidence{}, authError(reasonCoverageGap)
	}
	if previous, ok := s.pins[ch.role]; ok {
		if !ownerEqual(previous.owner, ch.owner) || previous.pairID != ch.pairID || previous.pairGeneration != ch.pairGeneration {
			return roleEvidence{}, authError(reasonOwnerDrift)
		}
	} else {
		s.pins[ch.role] = ch
	}
	from := ch.previousCoverage
	if ch.previousVersion == 0 {
		from = ch.acquisitionStarted
	}
	deadline := ch.acquisitionStarted.Add(maximumEvidenceAge)
	return roleEvidence{
		instanceID: ch.instanceID, challengeID: ch.id, vectorID: ch.vectorID,
		contractID: ch.contractID, providerID: ch.providerID, providerVersion: ch.providerVersion, evidenceSchema: ch.evidenceSchema,
		sourceID: ch.sourceID, origin: ch.origin, operation: ch.operation, databaseScope: ch.databaseScope,
		accountID: mapSyntheticIdentity("account", "synthetic:account:"+actualAccount), pairID: ch.pairID, attemptID: ch.attemptID, pairGeneration: ch.pairGeneration,
		role: ch.role, owner: cloneOwner(ch.owner), frameID: ch.frameID, requestID: ch.requestID,
		fixtureGrantID: integrationFixtureGrant().id, provenance: fixtureProvenance, authentication: fixtureAuthentication, identity: fixtureIdentity,
		cacheState: fixtureCacheFresh, authenticated: true, continuityKnown: true, coverageKnown: true, coverageContinuous: true,
		sessionRevision: fixtureRevision(ch.role), acquisitionStarted: ch.acquisitionStarted, observedAt: observed,
		coverageFrom: from, coverageThrough: observed, authValidUntil: deadline, identityValidUntil: deadline, coverageValidUntil: deadline, providerValidUntil: deadline,
	}, nil
}

func (s *controlledSyntheticLifecycle) barrier(request syntheticLifecycleRequest) (func(), bool) {
	s.mu.Lock()
	r, rok := s.pins[readerRole]
	c, cok := s.pins[collectorRole]
	valid := !s.gap && s.generation == 1 && rok && cok &&
		request.enrollment.contractID == concurrentAuthorizationContract && request.enrollment.providerID == fixtureProviderID &&
		request.enrollment.providerVersion == fixtureProviderVersion && request.enrollment.evidenceSchema == fixtureEvidenceSchema &&
		request.enrollment.sourceID == fixtureSourceID && request.enrollment.databaseScope == mapSyntheticIdentity("database", fixtureDatabaseScope) &&
		request.enrollment.accountID == mapSyntheticIdentity("account", "synthetic:account:"+s.account) &&
		request.enrollment.policyGeneration == 4 && request.enrollment.holdGeneration == 9 &&
		request.pair.id == r.pairID && request.pair.id == c.pairID && request.pair.generation == r.pairGeneration && request.pair.generation == c.pairGeneration &&
		ownerEqual(request.reader, r.owner) && ownerEqual(request.collector, c.owner) &&
		request.readerRevision == fixtureRevision(readerRole) && request.collectorRevision == fixtureRevision(collectorRole) &&
		!request.readerFrom.Before(s.started) && !request.collectorFrom.Before(s.started) &&
		!request.barrier.Before(request.readerFrom) && !request.barrier.Before(request.collectorFrom) && !request.barrier.After(time.Now())
	if !valid {
		s.mu.Unlock()
		return nil, false
	}
	return s.mu.Unlock, true
}

func controlledFixtureAuthorization(account string, state *controlledSyntheticLifecycle, collect evidenceCollector) authorizedConcurrentFixture {
	return authorizedConcurrentFixture{DatabaseScope: fixtureDatabaseScope, AccountID: "synthetic:account:" + account,
		ReaderRevision: fixtureReaderRevision, CollectorRevision: fixtureCollectorRevision, PolicyGeneration: 4, HoldGeneration: 9,
		PolicyEnabled: true, Grant: integrationFixtureGrant(), Collect: collect, LifecycleBarrier: state.barrier}
}

type fixtureRenewalLoop struct {
	cancel      context.CancelFunc
	done        chan struct{}
	failed      chan struct{}
	count       atomic.Uint64
	mu          sync.Mutex
	err         error
	stopRequest chan struct{}
	stopOnce    sync.Once
}

func startFixtureRenewalLoop(parent context.Context, controller *concurrentController) *fixtureRenewalLoop {
	ctx, cancel := context.WithCancel(parent)
	l := &fixtureRenewalLoop{cancel: cancel, done: make(chan struct{}), failed: make(chan struct{}), stopRequest: make(chan struct{})}
	go func() {
		defer close(l.done)
		ticker := time.NewTicker(1500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-l.stopRequest:
				return
			case <-ticker.C:
				if err := controller.RenewEvidence(ctx); err != nil {
					if ctx.Err() == nil {
						l.mu.Lock()
						l.err = err
						l.mu.Unlock()
						close(l.failed)
					}
					return
				}
				l.count.Add(1)
			}
		}
	}()
	return l
}

func (l *fixtureRenewalLoop) stop(ctx context.Context) error {
	if l == nil {
		return nil
	}
	l.stopOnce.Do(func() { close(l.stopRequest) })
	select {
	case <-l.done:
		l.cancel()
		return nil
	case <-ctx.Done():
		l.cancel()
		return errors.New("synthetic renewal loop did not join")
	}
}

func fixtureAuthorityVersion(c *concurrentController) uint64 {
	a, ok := c.authorization.(*authorizedConcurrentAdapter)
	if !ok {
		return 0
	}
	a.mu.Lock()
	authority := a.authority
	a.mu.Unlock()
	if authority == nil {
		return 0
	}
	authority.mu.Lock()
	defer authority.mu.Unlock()
	return authority.versionCounter
}

func (l *fixtureRenewalLoop) failureError() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.err
}
