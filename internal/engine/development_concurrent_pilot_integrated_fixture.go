//go:build auth_session_dev && auth_session_fixture

package engine

import (
	"context"
	"errors"
	"time"

	"github.com/abangkis/AkuSidecar/internal/captureruntime"
)

var pilotOfflineScenarios = []string{"pilot-overlap", "pilot-renewal-failure", "pilot-policy-drift", "pilot-opt-out", "pilot-last-close", "pilot-parent-cancel"}

func (f *concurrentIntegratedFixture) fixtureNow() time.Time {
	f.clockMu.RLock()
	defer f.clockMu.RUnlock()
	return f.now
}

func (f *concurrentIntegratedFixture) advanceFixtureClock(delta time.Duration) {
	f.clockMu.Lock()
	defer f.clockMu.Unlock()
	f.now = f.now.Add(delta)
}

// This source invents correlated fixture evidence; it does not inspect a browser.
func (f *concurrentIntegratedFixture) pilotFixture() pilotConcurrentFixture {
	return pilotConcurrentFixture{ExplicitOptIn: true, LimitationAcknowledged: true, PolicyEnabled: true, ExpectedAccountID: "synthetic:account:fixture", ScopeID: "synthetic-pilot-fixture-scope", PolicyID: concurrentPilotPolicy, PolicyGeneration: 4, ConsentGeneration: 9, Grant: pilotFixtureGrant{id: "synthetic-pilot-integrated-grant", allowOptIn: true, allowSeedReceipt: true, allowEvidence: true, allowCollection: true, allowResultCommit: true}, Source: func(ctx context.Context, c pilotChallenge) (pilotRoleEvidence, error) {
		if err := ctx.Err(); err != nil {
			return pilotRoleEvidence{}, err
		}
		now := f.fixtureNow()
		e := pilotRoleEvidence{instanceID: c.instanceID, challengeID: c.id, vectorID: c.vectorID, contractID: c.contractID, providerID: c.providerID, evidenceSchema: c.evidenceSchema, policyID: c.policyID, policyGeneration: c.policyGeneration, consentGeneration: c.consentGeneration, accountID: c.accountID, pair: c.pair, role: c.role, owner: clonePilotOwner(c.owner), operation: c.operation, identitySourceID: c.identitySourceID, identityOrigin: c.identityOrigin, settingsMethod: c.settingsMethod, settingsURL: c.settingsURL, settingsRouteID: c.settingsRouteID, settingsQualified: c.settingsQualified, targetID: c.targetID, frameID: c.frameID, documentID: c.documentID, requestID: c.requestID, acquisitionStart: c.acquisitionStart, observedAt: now, validUntil: now.Add(concurrentPilotMaxAge), identityPresent: true, identityLive: true, settingsSuccess: true, authenticated: true, cacheFresh: true, serviceWorkerOK: true, notRedirected: true, wellFormed: true, correlated: true}
		if f.pilotScenario == "pilot-renewal-failure" && c.previousVersion > 0 {
			e.identityPresent = false
		}
		return e, nil
	}}
}

func (f *concurrentIntegratedFixture) runPilot(ctx context.Context, scenario string) error {
	if err := f.open(ctx, 1, true); err != nil {
		return err
	}
	ticket, lease, done, err := f.captureStart(ctx)
	if err != nil {
		return err
	}
	defer lease.Release()
	// Every return unblocks the invented backend; close() drains it and all leases.
	defer close(f.backend.release)
	before, pid := f.collector.Snapshot(), f.collector.PID()
	driver := lease.Driver()
	generation := lease.Generation()
	if before.ActiveLeases != 1 || driver != "headless" {
		return errors.New("pilot active lease unavailable")
	}
	if err = f.open(ctx, 2, false); err != nil {
		return err
	}
	f.advanceFixtureClock(time.Second)
	renewErr := f.c.RenewEvidence(ctx)
	if scenario == "pilot-renewal-failure" {
		if renewErr == nil {
			return errors.New("pilot failed evidence admitted")
		}
	} else if renewErr != nil {
		return renewErr
	}
	if f.collector.Snapshot() != before || f.collector.PID() != pid || lease.Driver() != driver || lease.Generation() != generation || f.launches != 0 || f.c.Snapshot().ReaderWindows != 2 {
		return errors.New("pilot reader or renewal disturbed active collector")
	}
	if f.proofs.calls != 0 || f.permits.grants != 0 {
		return errors.New("pilot used legacy proof or Store permit")
	}
	switch scenario {
	case "pilot-policy-drift":
		f.c.PolicyChanged(5, 9, true)
	case "pilot-opt-out":
		if err = f.c.Disable(ctx); err != nil {
			return err
		}
		if f.c.Snapshot().ReaderWindows != 2 || f.reader.Snapshot().State != captureruntime.Ready {
			return errors.New("pilot opt-out removed readers")
		}
	case "pilot-last-close":
		if err = f.c.CloseReaderWindow(ctx, "synthetic:window:2"); err != nil {
			return err
		}
		// The final close is expected to refuse independent continuation.
		_ = f.c.CloseReaderWindow(ctx, "synthetic:window:1")
		if f.c.Snapshot().ReaderWindows != 0 || f.c.Snapshot().AdmissionOpen {
			return errors.New("pilot last close admitted independent collector")
		}
	case "pilot-parent-cancel":
		f.cancel()
	}
	if scenario == "pilot-overlap" {
		// Release the blocked Coordinator capture before accepting the original ticket.
		f.backend.release <- struct{}{}
		if err = <-done; err != nil {
			return err
		}
		lease.Release()
		if err = f.c.Complete(ctx, ticket, "synthetic:pilot-result"); err != nil {
			return err
		}
		if _, ok := f.c.SinkResult(ticket.WorkID); !ok {
			return errors.New("pilot original result missing")
		}
		if f.c.Complete(ctx, ticket, "synthetic:pilot-duplicate") == nil {
			return errors.New("pilot duplicate accepted")
		}
		stale := ticket
		stale.EvidenceVersion++
		if f.c.Claim(ctx, stale) == nil || f.c.Complete(ctx, stale, "synthetic:pilot-stale") == nil {
			return errors.New("pilot stale ticket accepted")
		}
		if f.collector.PID() != pid || f.collector.Snapshot().Generation != generation || f.collector.Snapshot().ActiveLeases != 0 || f.launches != 0 {
			return errors.New("pilot completion replaced collector")
		}
	} else {
		if f.c.Complete(ctx, ticket, "synthetic:pilot-late") == nil {
			return errors.New("pilot late result accepted")
		}
		if _, ok := f.c.SinkResult(ticket.WorkID); ok {
			return errors.New("pilot late sink write")
		}
		// Parent cancellation can retire owned managers before the capture drains.
		if scenario == "pilot-parent-cancel" {
			select {
			case <-done:
			case <-ctx.Done():
				return errors.New("pilot parent capture did not drain")
			}
		}
	}
	return nil
}
