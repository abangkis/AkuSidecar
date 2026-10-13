//go:build auth_session_fixture

package engine

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/collection"
)

// SyntheticSessionEvidence is in-memory fixture evidence, never public status.
type SyntheticSessionEvidence struct {
	Authenticated bool
	AccountID     string
	Revision      string
}

type syntheticExclusiveBinding struct {
	mu        sync.Mutex
	e         *Engine
	expected  string
	probe     func(context.Context) (SyntheticSessionEvidence, error)
	revision  string
	needsUser bool
}

// BindSyntheticExclusive is only compiled with auth_session_fixture. It must be
// called before coordinator/collector startup, on an isolated development Store.
// No production config, environment variable or HTTP endpoint enables this path.
func (e *Engine) BindSyntheticExclusive(expectedAccount string, probe func(context.Context) (SyntheticSessionEvidence, error)) error {
	if !e.config.Dev || e.fixtureAdmission != nil || e.collectionRuntime == nil || e.captureOwner == nil || expectedAccount == "" || probe == nil {
		return errors.New("synthetic fixture binding unavailable")
	}
	b := &syntheticExclusiveBinding{e: e, expected: expectedAccount, probe: probe}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := b.verify(ctx); err != nil {
		return err
	}
	if ids, err := e.store.ActiveMediaRecaptureIDs(ctx); err != nil || len(ids) > 0 {
		return errors.New("synthetic fixture cannot adopt media recapture work")
	}
	e.fixtureAdmission = &fixtureAdmissionGate{paused: true}
	if err := e.collectionRuntime.BindSyntheticExclusive(collection.SyntheticExclusiveHooks{
		BeforeReader: b.pause, BeforeHeadless: b.prepareHeadless, HeadlessReady: b.resume,
		Paused: func() bool { return !e.fixtureAdmissionAllowed() },
	}); err != nil {
		return err
	}
	return b.pause(ctx)
}

func (b *syntheticExclusiveBinding) verify(ctx context.Context) (SyntheticSessionEvidence, error) {
	if b.needsUser {
		return SyntheticSessionEvidence{}, errors.New("synthetic session needs user action")
	}
	v, err := b.probe(ctx)
	if err != nil || !v.Authenticated || v.AccountID != b.expected || v.Revision == "" {
		b.needsUser = true
		return SyntheticSessionEvidence{}, errors.New("synthetic session verification required")
	}
	return v, nil
}

func (b *syntheticExclusiveBinding) busy() bool {
	e := b.e
	e.fixtureAdmission.mu.Lock()
	running := e.fixtureAdmission.running
	e.fixtureAdmission.mu.Unlock()
	e.mu.RLock()
	active := len(e.active) + len(e.pending)
	e.mu.RUnlock()
	return running > 0 || active > 0
}

func (b *syntheticExclusiveBinding) pause(ctx context.Context) error {
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

func (b *syntheticExclusiveBinding) prepareHeadless(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	v, err := b.verify(ctx)
	if err == nil {
		b.revision = v.Revision
	}
	return err
}

func (b *syntheticExclusiveBinding) resume(ctx context.Context, generation uint64) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	e := b.e
	if e.fixtureAdmissionAllowed() {
		return nil
	}
	if b.busy() {
		return captureruntime.ErrBusy
	}
	if e.collectionRuntime.SyntheticReaderPending() {
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
