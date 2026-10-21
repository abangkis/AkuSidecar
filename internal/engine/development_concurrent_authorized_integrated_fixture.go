//go:build auth_session_dev && auth_session_fixture

package engine

import (
	"context"
	"errors"
	"strings"
	"time"
)

var authorizedOfflineScenarios = []string{"authorized-renewal", "authorized-renewal-delay", "authorized-renewal-failure", "authorized-account-drift", "authorized-policy-drift", "authorized-coverage-gap", "authorized-opt-out", "authorized-last-close-denied", "authorized-parent-cancel"}

func (f *concurrentIntegratedFixture) collectAuthorized(ctx context.Context, ch evidenceChallenge) (roleEvidence, error) {
	f.lifecycle.mu.Lock()
	delay, fail := f.lifecycle.delayRenewal, f.lifecycle.failRenewal
	f.lifecycle.mu.Unlock()
	if ch.previousVersion > 0 && delay {
		timer := time.NewTimer(maximumBindingAge + time.Second)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return roleEvidence{}, ctx.Err()
		}
	}
	if ch.previousVersion > 0 && fail {
		return roleEvidence{}, errors.New("synthetic controlled renewal failed")
	}
	if err := ctx.Err(); err != nil {
		return roleEvidence{}, err
	}
	return f.lifecycle.observe(ch, time.Now(), "fixture")
}

func (f *concurrentIntegratedFixture) runAuthorized(ctx context.Context, scenario string) error {
	if f.c.authorization == nil || f.proofs.calls != 0 || f.permits.grants != 0 {
		return errors.New("synthetic authority selection failed")
	}
	if err := f.open(ctx, 1, true); err != nil {
		return err
	}
	ticket, lease, done, err := f.captureStart(ctx)
	if err != nil {
		return err
	}
	defer lease.Release()
	f.originalVersion = ticket.EvidenceVersion
	f.renewal = startFixtureRenewalLoop(f.ctx, f.c)
	before, pid := f.collector.Snapshot(), f.collector.PID()
	if err = f.open(ctx, 2, false); err != nil {
		return err
	}
	if f.collector.Snapshot() != before || f.collector.PID() != pid || before.ActiveLeases != 1 || f.launches != 0 || f.c.Snapshot().ReaderWindows != 2 {
		return errors.New("synthetic authorized overlap invariant failed")
	}
	switch scenario {
	case "authorized-renewal", "authorized-last-close-denied":
		if scenario == "authorized-renewal" {
			timer := time.NewTimer(11 * time.Second)
			defer timer.Stop()
			select {
			case <-timer.C:
			case <-f.renewal.failed:
				return errors.New("synthetic automatic renewal failed")
			case <-ctx.Done():
				return ctx.Err()
			}
			if f.renewal.count.Load() < 3 {
				return errors.New("synthetic automatic renewal count insufficient")
			}
		}
		if err = f.renewal.stop(ctx); err != nil {
			return err
		}
		f.finalVersion = fixtureAuthorityVersion(f.c)
		stale := ticket
		stale.EvidenceVersion++
		if f.c.Complete(ctx, stale, "synthetic:stale") == nil {
			return errors.New("synthetic stale ticket accepted")
		}
		close(f.backend.release)
		if err = <-done; err != nil {
			return err
		}
		lease.Release()
		if err = f.c.Complete(ctx, ticket, "synthetic:authorized-result"); err != nil {
			return err
		}
		if _, ok := f.c.SinkResult(ticket.WorkID); !ok {
			return errors.New("synthetic authorized sink missing")
		}
		if f.c.Complete(ctx, ticket, "synthetic:duplicate") == nil {
			return errors.New("synthetic duplicate result accepted")
		}
		if f.collector.PID() != pid || f.collector.Snapshot().Generation != before.Generation || f.collector.Snapshot().ActiveLeases != 0 || f.c.Snapshot().ReaderWindows != 2 {
			return errors.New("synthetic authorized completion changed owners")
		}
		if scenario == "authorized-last-close-denied" {
			if err = f.c.CloseReaderWindow(ctx, "synthetic:window:2"); err != nil {
				return err
			}
			if f.c.CloseReaderWindow(ctx, "synthetic:window:1") == nil || f.c.Snapshot().AdmissionOpen {
				return errors.New("synthetic independent continuation accepted")
			}
		}
		return nil
	case "authorized-renewal-delay", "authorized-renewal-failure", "authorized-account-drift", "authorized-coverage-gap":
		f.lifecycle.mu.Lock()
		switch scenario {
		case "authorized-renewal-delay":
			f.lifecycle.delayRenewal = true
		case "authorized-renewal-failure":
			f.lifecycle.failRenewal = true
		case "authorized-account-drift":
			f.lifecycle.account = "beta"
			f.lifecycle.generation++
		case "authorized-coverage-gap":
			f.lifecycle.gap = true
		}
		f.lifecycle.mu.Unlock()
		select {
		case <-f.renewal.failed:
		case <-ctx.Done():
			return ctx.Err()
		}
	case "authorized-policy-drift":
		f.c.PolicyChanged(5, 9, true)
	case "authorized-opt-out":
		if err = f.c.Disable(ctx); err != nil {
			return err
		}
		if f.c.Snapshot().ReaderWindows != 2 {
			return errors.New("synthetic opt out lost reader windows")
		}
	case "authorized-parent-cancel":
		f.cancel()
		if err = f.c.WaitParentCleanup(ctx); err != nil {
			return err
		}
	default:
		return errors.New("synthetic authorized scenario unavailable")
	}
	if err = f.renewal.stop(ctx); err != nil {
		return err
	}
	f.finalVersion = fixtureAuthorityVersion(f.c)
	close(f.backend.release)
	<-done
	lease.Release()
	if f.c.Complete(ctx, ticket, "synthetic:late-authorized-result") == nil {
		return errors.New("synthetic fenced result accepted")
	}
	if _, ok := f.c.SinkResult(ticket.WorkID); ok {
		return errors.New("synthetic fenced sink mutated")
	}
	if f.proofs.calls != 0 || f.permits.grants != 0 {
		return errors.New("synthetic legacy authority consulted")
	}
	return nil
}

func isAuthorizedFixtureScenario(scenario string) bool {
	return strings.HasPrefix(scenario, "authorized-")
}
