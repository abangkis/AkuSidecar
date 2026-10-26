//go:build windows && auth_session_dev

package engine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/readerbroker"
	"golang.org/x/sys/windows"
)

// A process-local manual selection, bound to the exact pinned package/profiles.
// It is never reconstructed from status, persisted configuration, or a report.
type xApplicationManualConsent struct {
	scope xApplicationOwnerConfig
	used  atomic.Bool
}

func (*xApplicationManualConsent) String() string     { return "private-application-manual-consent" }
func (v *xApplicationManualConsent) GoString() string { return v.String() }

func newXApplicationManualConsent(cfg xApplicationOwnerConfig, limitationAcknowledged bool) *xApplicationManualConsent {
	if !cfg.enabled || !limitationAcknowledged {
		return nil
	}
	return &xApplicationManualConsent{scope: cfg}
}

type xApplicationPostPreparer func(context.Context, xManualPilotWindowsRole, readerbroker.Request, string) (readerbroker.NativePostPreparation, error)

// This factory is intentionally private and unregistered. A future isolated
// manual-start entry must supply explicit consent and an owned post preparer;
// compiling this function or inspecting status cannot start a session transfer.
func (e *Engine) xApplicationNativeFactory(cfg xApplicationOwnerConfig, consent *xApplicationManualConsent, prepare xApplicationPostPreparer) xManualApplicationFactory {
	return func(ctx, parent context.Context) (xManualApplicationPorts, error) {
		p := xManualApplicationPorts{dispose: func(context.Context) error { return nil }}
		if e == nil || !e.config.Dev || ctx == nil || parent == nil || ctx.Err() != nil || parent.Err() != nil || !cfg.enabled || consent == nil || consent.scope != cfg || prepare == nil || !consent.used.CompareAndSwap(false, true) {
			return p, errors.New("manual native selection unavailable")
		}
		owner, err := startXApplicationOwner(parent, ctx, cfg)
		if owner != nil {
			p.dispose = owner.Close
		}
		if err != nil {
			return p, errors.New("manual native owner unavailable")
		}
		reader, collector, err := owner.initializeNative(ctx)
		if err != nil {
			return p, err
		}
		seed, started, err := owner.seedNative(ctx)
		if err != nil {
			return p, err
		}
		var random [24]byte
		if _, err = rand.Read(random[:]); err != nil {
			return p, errors.New("manual native attempt unavailable")
		}
		nonce := hex.EncodeToString(random[:])
		parentRef := strconv.FormatUint(uint64(reader.parent.PID), 10) + "/" + reader.parent.Created
		binding := nativePilotBinding{account: seed.AccountID, enrollment: nonce + "e", pair: nonce + "p", attempt: nonce + "a", derivation: nonce + "d", policy: 1, consent: 1,
			reader: nativePilotOwner{role: "reader", profile: reader.profile, parent: parentRef}, collector: nativePilotOwner{role: "collector", profile: collector.profile, parent: parentRef}}
		policy := func(call context.Context) (nativePilotBrokerState, error) {
			return nativePilotBrokerState{account: binding.account, enrollment: binding.enrollment, pair: binding.pair, attempt: binding.attempt, derivation: binding.derivation, policy: binding.policy, consent: binding.consent,
				completeInventory: true, parentAlive: parent.Err() == nil, channelHealthy: !owner.isClosed(), invalidationsReady: true, optedIn: parent.Err() == nil}, call.Err()
		}
		b, ports, _ := newXManualPilotOwnerCaptureBroker(parent, xManualPilotOwnerConfig{enabled: true, pilot: nativePilotConfig{enabled: true, contract: nativePilotContract, limitationAcknowledged: true, binding: binding, attemptStarted: started}, policy: policy}, reader, collector)
		if b == nil {
			return p, errors.New("manual native broker unavailable")
		}
		p.broker = b
		p.admission = nativePilotNativeStoreAdmission{broker: b, store: e.store}
		var stopManagers func()
		p.dispose = func(call context.Context) error {
			if stopManagers != nil {
				stopManagers()
			}
			return owner.Close(call)
		}
		p.prepare = func(call context.Context, req readerbroker.Request, marker string) (readerbroker.NativePostPreparation, error) {
			return prepare(call, reader, req, marker)
		}
		p.buildRuntime = func(call context.Context) (*nativePilotRuntime, error) {
			readerManager, collectorManager, closeManagers, err := newXApplicationManagers(parent, owner, reader, collector)
			if err != nil {
				return nil, err
			}
			stopManagers = closeManagers
			readerManager.SetExitObserver(func(uint64) { b.invalidate("reader-lost") })
			collectorManager.SetExitObserver(func(uint64) { b.invalidate("owner-or-policy-changed") })
			return newXNativePilotRuntime(call, b, ports, readerManager, collectorManager, func(openCtx context.Context, _ readerbroker.Request) error { return owner.openNativeReader(openCtx) })
		}
		return p, nil
	}
}

// Managers borrow the exact already-owned roles. They cannot relaunch or adopt
// a process. Full disposal remains the application owner's Job responsibility.
type xApplicationRoleProcess struct {
	owner       *xApplicationOwner
	role        xManualPilotWindowsRole
	done        chan error
	stop, ended chan struct{}
	once        sync.Once
}

func (p *xApplicationRoleProcess) Driver() string {
	if p.role.role == "reader" {
		return "browser"
	}
	return "headless"
}
func (p *xApplicationRoleProcess) PID() int           { return int(p.role.chrome.PID) }
func (p *xApplicationRoleProcess) Done() <-chan error { return p.done }
func (p *xApplicationRoleProcess) Terminate()         { p.owner.terminateOwnedTree() }
func (*xApplicationRoleProcess) CloseForRetry(context.Context) error {
	return errors.New("manual owner replacement forbidden")
}
func (*xApplicationRoleProcess) OpenExtensionsPage(context.Context) error {
	return errors.New("manual owner extension navigation forbidden")
}
func (*xApplicationRoleProcess) ReplacementReadiness(context.Context) error {
	return errors.New("manual owner replacement forbidden")
}
func (p *xApplicationRoleProcess) close() { p.once.Do(func() { close(p.stop) }); <-p.ended }

func newXApplicationManagers(parent context.Context, owner *xApplicationOwner, reader, collector xManualPilotWindowsRole) (*captureruntime.Manager, *captureruntime.Manager, func(), error) {
	var processes []*xApplicationRoleProcess
	closeAll := func() {
		for _, p := range processes {
			p.close()
		}
	}
	makeManager := func(role xManualPilotWindowsRole) (*captureruntime.Manager, error) {
		if parent == nil || parent.Err() != nil || owner == nil || owner.isClosed() || role.bridge != owner || role.chromeHandle == 0 {
			return nil, errors.New("manual manager owner unavailable")
		}
		p := &xApplicationRoleProcess{owner: owner, role: role, done: make(chan error), stop: make(chan struct{}), ended: make(chan struct{})}
		go func() {
			defer close(p.ended)
			defer close(p.done)
			tick := time.NewTicker(25 * time.Millisecond)
			defer tick.Stop()
			for {
				select {
				case <-parent.Done():
					return
				case <-p.stop:
					return
				case <-tick.C:
					state, err := windows.WaitForSingleObject(role.chromeHandle, 0)
					if err != nil || state != uint32(windows.WAIT_TIMEOUT) {
						return
					}
				}
			}
		}()
		processes = append(processes, p)
		return captureruntime.New(p)
	}
	r, err := makeManager(reader)
	if err != nil {
		closeAll()
		return nil, nil, func() {}, err
	}
	c, err := makeManager(collector)
	if err != nil {
		closeAll()
		return nil, nil, func() {}, err
	}
	return r, c, closeAll, nil
}
