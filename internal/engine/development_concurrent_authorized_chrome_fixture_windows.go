//go:build windows && auth_session_dev && auth_session_fixture

package engine

import (
	"context"
	"errors"
	"fmt"
	"time"
)

func (f *concurrentChromeFixture) collectAuthorizedRPC(ctx context.Context, record *concurrentChromeRecord) error {
	if !f.authorized || record != f.records[concurrentCollector] || !f.collectionRPCInFlight.CompareAndSwap(false, true) {
		return errors.New("synthetic protected collection lane unavailable")
	}
	defer f.collectionRPCInFlight.Store(false)
	if err := ctx.Err(); err != nil {
		return err
	}
	var result struct {
		Status int  `json:"status"`
		Item   bool `json:"item"`
	}
	if err := record.bridge.rpcRoundTrip(ctx, "collect", map[string]any{"owner": record.process.owner}, &result); err != nil {
		return err
	}
	if result.Status != 200 || !result.Item {
		return errors.New("synthetic protected browser collection failed")
	}
	return nil
}

func (f *concurrentChromeFixture) collectAuthorized(ctx context.Context, ch evidenceChallenge) (roleEvidence, error) {
	f.server.mu.Lock()
	delay, fail := f.server.delayRenewal, f.server.failRenewal
	f.server.mu.Unlock()
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
	ownerRole := concurrentReader
	if ch.role == collectorRole {
		ownerRole = concurrentCollector
	}
	owner, exists, err := f.CurrentOwner(ctx, ownerRole)
	if err != nil || !exists {
		return roleEvidence{}, authError(reasonOwnerDrift)
	}
	// Pin browser/process/directory identity outside authority/source locks. The
	// challenge cannot substitute an owner merely by echoing its own fields.
	f.c.mu.Lock()
	epoch, parent := f.c.epoch, f.c.cfg.ParentID
	f.c.mu.Unlock()
	expected := mapConcurrentOwner(owner, ch.role, parent, epoch, ch.owner.processCreatedAt, ch.owner.windowInventory)
	if !ownerBindingEqual(expected, ch.owner) {
		return roleEvidence{}, authError(reasonOwnerDrift)
	}
	f.mu.Lock()
	record := f.records[ownerRole]
	f.mu.Unlock()
	probe, err := record.process.Evidence(ctx)
	if err != nil {
		return roleEvidence{}, err
	}
	if !probe.Authenticated || probe.AccountID != "alpha" || probe.Revision != "fixture-1" {
		return roleEvidence{}, authError(reasonUnknownIdentity)
	}
	if err = ctx.Err(); err != nil {
		return roleEvidence{}, err
	}
	return f.server.observe(ch, time.Now(), probe.AccountID)
}

func (f *concurrentChromeFixture) authorizationReceipt() *concurrentChromeAuthorizationReceipt {
	if !f.authorized {
		return nil
	}
	f.server.mu.Lock()
	probes, elapsed := f.server.probesDuringCollect, int64(0)
	if !f.server.collectStarted.IsZero() && !f.server.collectEnded.IsZero() {
		elapsed = f.server.collectEnded.Sub(f.server.collectStarted).Milliseconds()
	} else if !f.server.collectStarted.IsZero() {
		elapsed = time.Since(f.server.collectStarted).Milliseconds()
	}
	f.server.mu.Unlock()
	return &concurrentChromeAuthorizationReceipt{AuthoritySelected: true, AutomaticRenewals: f.renewal.count.Load(), ProbesDuringCollect: probes,
		OriginalTicketVersion: f.originalVersion, FinalEvidenceVersion: f.finalVersion, ProtectedRequestMillis: elapsed, StaleDenied: f.staleDenied, DuplicateDenied: f.duplicateDenied}
}

func (f *concurrentChromeFixture) runAuthorized(ctx context.Context, scenario string) error {
	if f.c.authorization == nil || f.grants != 0 {
		return errors.New("synthetic Chrome authority selection failed")
	}
	if err := f.duplicateGates(ctx); err != nil {
		return err
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
	collector := f.records[concurrentCollector]
	before, pid := collector.manager.Snapshot(), collector.manager.PID()
	if err = f.open(ctx, 2, false); err != nil {
		return err
	}
	f.server.mu.Lock()
	overlap := f.server.overlap
	f.server.mu.Unlock()
	if !overlap || collector.manager.Snapshot() != before || collector.manager.PID() != pid || before.ActiveLeases != 1 || f.launches.Load() != 0 || f.c.Snapshot().ReaderWindows != 2 {
		return errors.New("synthetic authorized Chrome overlap invariant failed")
	}
	switch scenario {
	case "authorized-renewal", "authorized-last-close-denied":
		if scenario == "authorized-renewal" {
			timer := time.NewTimer(11 * time.Second)
			defer timer.Stop()
			select {
			case <-timer.C:
			case <-f.renewal.failed:
				return fmt.Errorf("synthetic automatic Chrome renewal failed (count=%d): %w", f.renewal.count.Load(), f.renewal.failureError())
			case <-ctx.Done():
				return ctx.Err()
			}
			if f.renewal.count.Load() < 3 {
				f.server.mu.Lock()
				probes := f.server.probesDuringCollect
				f.server.mu.Unlock()
				return fmt.Errorf("synthetic automatic Chrome renewal count insufficient (count=%d probes=%d): %w", f.renewal.count.Load(), probes, f.renewal.failureError())
			}
		}
		if err = f.renewal.stop(ctx); err != nil {
			return err
		}
		f.finalVersion = fixtureAuthorityVersion(f.c)
		stale := ticket
		stale.EvidenceVersion++
		f.staleDenied = f.c.Complete(ctx, stale, "synthetic:stale-chrome-result") != nil
		if !f.staleDenied {
			return errors.New("synthetic stale Chrome ticket accepted")
		}
		f.server.unblock()
		if err = <-done; err != nil {
			return err
		}
		lease.Release()
		if err = f.c.Complete(ctx, ticket, "synthetic:authorized-chrome-result"); err != nil {
			return err
		}
		if _, ok := f.c.SinkResult(ticket.WorkID); !ok {
			return errors.New("synthetic authorized Chrome sink missing")
		}
		f.duplicateDenied = f.c.Complete(ctx, ticket, "synthetic:duplicate-chrome-result") != nil
		if !f.duplicateDenied {
			return errors.New("synthetic duplicate Chrome result accepted")
		}
		inventory, inventoryErr := f.inventory(ctx)
		if inventoryErr != nil || inventory.WindowCount != 2 || inventory.BootstrapCount != 1 || collector.manager.PID() != pid || collector.manager.Snapshot().Generation != before.Generation || collector.manager.Snapshot().ActiveLeases != 0 || f.c.Snapshot().ReaderWindows != 2 {
			return errors.New("synthetic authorized Chrome completion changed owners")
		}
		if scenario == "authorized-last-close-denied" {
			if err = f.c.CloseReaderWindow(ctx, "synthetic:window:2"); err != nil {
				return err
			}
			if f.c.CloseReaderWindow(ctx, "synthetic:window:1") == nil || f.c.Snapshot().AdmissionOpen {
				return errors.New("synthetic Chrome independent continuation accepted")
			}
		}
		return nil
	case "authorized-renewal-delay", "authorized-renewal-failure", "authorized-account-drift", "authorized-coverage-gap":
		f.server.mu.Lock()
		switch scenario {
		case "authorized-renewal-delay":
			f.server.delayRenewal = true
		case "authorized-renewal-failure":
			f.server.failRenewal = true
		case "authorized-account-drift":
			f.server.account = "beta"
			f.server.generation++
		case "authorized-coverage-gap":
			f.server.gap = true
		}
		f.server.mu.Unlock()
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
		inventory, inventoryErr := f.inventory(ctx)
		if inventoryErr != nil || inventory.WindowCount != 2 || !collector.bridge.cleanupVerified() || f.records[concurrentReader].bridge.cleanupVerified() || f.c.Snapshot().ReaderWindows != 2 {
			return errors.New("synthetic authorized Chrome opt out lost windows")
		}
	default:
		return errors.New("synthetic authorized Chrome scenario unavailable")
	}
	if err = f.renewal.stop(ctx); err != nil {
		return err
	}
	f.finalVersion = fixtureAuthorityVersion(f.c)
	f.server.unblock()
	<-done
	lease.Release()
	if f.c.Complete(ctx, ticket, "synthetic:late-authorized-chrome-result") == nil {
		return errors.New("synthetic fenced Chrome result accepted")
	}
	if _, ok := f.c.SinkResult(ticket.WorkID); ok {
		return errors.New("synthetic fenced Chrome sink mutated")
	}
	if f.grants != 0 {
		return errors.New("synthetic legacy Chrome authority consulted")
	}
	return nil
}
