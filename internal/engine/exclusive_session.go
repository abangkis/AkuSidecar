//go:build auth_session_fixture || auth_session_dev

package engine

import (
	"context"
	"errors"
	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/collection"
	"sync"
	"time"
)

// sessionEvidence is verified private evidence; it is never exposed in status.
type sessionEvidence struct {
	Authenticated bool
	AccountID     string
	Revision      string
}

type exclusiveSessionBinding struct {
	mu           sync.Mutex
	e            *Engine
	expected     string
	probe        func(context.Context) (sessionEvidence, error)
	browserProbe func(context.Context) (sessionEvidence, error)
	revision     string
	needsUser    bool
}

func (e *Engine) bindExclusiveSession(expectedAccount string,
	probe func(context.Context) (sessionEvidence, error),
	browserProbe func(context.Context) (sessionEvidence, error), development bool) error {
	if !e.config.Dev || (e.fixtureAdmission != nil && !development) || e.collectionRuntime == nil || e.captureOwner == nil || expectedAccount == "" || probe == nil || e.collectionRuntime.ExclusiveSessionBound() {
		return errors.New("synthetic fixture binding unavailable")
	}
	if development && e.fixtureAdmission == nil {
		e.fixtureAdmission = &fixtureAdmissionGate{paused: true}
	}
	b := &exclusiveSessionBinding{e: e, expected: expectedAccount, probe: probe, browserProbe: browserProbe}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	authority, err := b.verify(ctx)
	if err != nil {
		return err
	}
	if browserProbe != nil {
		if err = b.verifyBrowser(ctx, authority); err != nil {
			return err
		}
	}
	if ids, err := e.store.ActiveMediaRecaptureIDs(ctx); err != nil || len(ids) > 0 {
		return errors.New("synthetic fixture cannot adopt media recapture work")
	}
	if e.fixtureAdmission == nil {
		e.fixtureAdmission = &fixtureAdmissionGate{paused: true}
	}
	if err := e.collectionRuntime.BindExclusiveSession(collection.ExclusiveSessionHooks{
		BeforeReader: b.pause, BeforeHeadless: b.prepareHeadless, HeadlessReady: b.resume,
		Paused: func() bool { return !e.fixtureAdmissionAllowed() },
	}); err != nil {
		return err
	}
	if development {
		e.developmentPause = b.pause
		e.developmentVerifyBrowser = func(ctx context.Context) error {
			b.mu.Lock()
			defer b.mu.Unlock()
			authority, err := b.verify(ctx)
			if err != nil {
				return err
			}
			return b.verifyBrowser(ctx, authority)
		}
	}
	return b.pause(ctx)
}

func (b *exclusiveSessionBinding) verify(ctx context.Context) (sessionEvidence, error) {
	if b.needsUser {
		return sessionEvidence{}, errors.New("synthetic session needs user action")
	}
	v, err := b.probe(ctx)
	if err != nil || !v.Authenticated || v.AccountID != b.expected || v.Revision == "" {
		b.needsUser = true
		return sessionEvidence{}, errors.New("synthetic session verification required")
	}
	return v, nil
}

func (b *exclusiveSessionBinding) verifyBrowser(ctx context.Context, authority sessionEvidence) error {
	if b.browserProbe == nil {
		return nil
	}
	v, err := b.browserProbe(ctx)
	if err != nil || !v.Authenticated || v.AccountID != authority.AccountID || v.Revision != authority.Revision {
		b.needsUser = true
		return errors.New("synthetic browser session verification required")
	}
	return nil
}

func (b *exclusiveSessionBinding) busy() bool {
	e := b.e
	e.fixtureAdmission.mu.Lock()
	running := e.fixtureAdmission.running
	e.fixtureAdmission.mu.Unlock()
	e.mu.RLock()
	active := len(e.active) + len(e.pending)
	e.mu.RUnlock()
	return running > 0 || active > 0
}

func (b *exclusiveSessionBinding) pause(ctx context.Context) error {
	e := b.e
	// Fence new collector steps immediately, even while another control settles.
	e.fixtureAdmission.mu.Lock()
	e.fixtureAdmission.paused = true
	e.fixtureAdmission.pauseEpoch++
	e.fixtureAdmission.mu.Unlock()
	b.mu.Lock()
	defer b.mu.Unlock()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		if !b.busy() {
			e.schedule.Lock()
			session, err := e.store.ActiveSession(ctx)
			if b.busy() {
				err = captureruntime.ErrBusy
			}
			if err == nil && !b.busy() {
				if session != nil {
					err = e.store.PauseFixtureCaptureSession(ctx, session.ID)
					if err == nil {
						e.releaseSessionCaptureLease(session.ID)
					}
				}
				// Paused markers from an earlier Engine must also release startup
				// adoption leases, after proving that no claimed command remains.
				if err == nil {
					ids, loadErr := e.store.ListFixturePausedCaptureSessions(ctx)
					err = loadErr
					for _, id := range ids {
						if err != nil {
							break
						}
						err = e.store.PauseFixtureCaptureSession(ctx, id)
						if err == nil {
							e.releaseSessionCaptureLease(id)
						}
					}
				}
			}
			e.schedule.Unlock()
			if err == nil {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			b.needsUser = true
			return errors.New("synthetic capture drain incomplete")
		case <-tick.C:
		}
	}
}

func (b *exclusiveSessionBinding) prepareHeadless(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	v, err := b.verify(ctx)
	if err == nil {
		b.revision = v.Revision
	}
	return err
}

func (b *exclusiveSessionBinding) resume(ctx context.Context, generation uint64) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	e := b.e
	e.fixtureAdmission.mu.Lock()
	developmentHold := e.fixtureAdmission.developmentHold
	e.fixtureAdmission.mu.Unlock()
	if developmentHold {
		return captureruntime.ErrBusy
	}
	if e.fixtureAdmissionAllowed() {
		return nil
	}
	if b.busy() {
		return captureruntime.ErrBusy
	}
	if e.collectionRuntime.ExclusiveReaderPending() {
		return captureruntime.ErrBusy
	}
	e.fixtureAdmission.mu.Lock()
	pauseEpoch := e.fixtureAdmission.pauseEpoch
	e.fixtureAdmission.mu.Unlock()
	snapshot := e.captureOwner.Snapshot()
	if snapshot.State != captureruntime.Ready || snapshot.Driver != "headless" || snapshot.Generation != generation {
		return captureruntime.ErrUnavailable
	}
	v, err := b.verify(ctx)
	if err != nil {
		return err
	}
	if b.revision != "" && v.Revision != b.revision {
		b.needsUser = true
		return errors.New("synthetic session changed during handoff")
	}
	if err = b.verifyBrowser(ctx, v); err != nil {
		return err
	}
	ids, err := e.store.ListFixturePausedCaptureSessions(ctx)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if session, loadErr := e.store.GetSession(ctx, id); loadErr != nil {
			return loadErr
		} else if session.Status == "queued" || session.Status == "running" {
			if err = e.ensureCaptureSession(id); err != nil {
				return err
			}
		}
		if err = e.store.ResumeFixtureCaptureSession(ctx, id, e.epoch, generation); err != nil {
			e.releaseSessionCaptureLease(id)
			return err
		}
	}
	e.fixtureAdmission.mu.Lock()
	changed := e.fixtureAdmission.pauseEpoch != pauseEpoch
	if !changed {
		e.fixtureAdmission.paused = false
	}
	e.fixtureAdmission.mu.Unlock()
	if changed {
		return captureruntime.ErrBusy
	}
	// A run not yet materialized before pause is admitted by the existing
	// durable session advancement path after the ownership boundary settles.
	if active, loadErr := e.store.ActiveSession(ctx); loadErr == nil && active != nil {
		_, err = e.startNext(ctx, active.ID)
	}
	return err
}
