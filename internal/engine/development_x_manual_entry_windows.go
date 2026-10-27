//go:build windows && auth_session_dev

package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/ownedprocess"
	"github.com/abangkis/AkuSidecar/internal/ownedprofile"
	"github.com/abangkis/AkuSidecar/internal/store"
	uireaderbroker "github.com/abangkis/AkuSidecar/ui-reader-broker"
)

type xManualDevelopmentEntry struct {
	closeOnce                                  sync.Once
	closeErr                                   error
	firstReason                                string
	mu                                         sync.Mutex
	selection                                  *xQASelection
	e                                          *Engine
	parent                                     context.Context
	attempted, stopped, closed, databaseOpened bool
	cancelStart                                context.CancelFunc
	startDone                                  chan struct{}
	clock                                      store.Clock
	fixtureClose                               func()
	uiExtension                                string
	uiLease                                    *ownedprofile.Lease
	// Private test seam: no flag, HTTP input or package can select a factory.
	factory func(*Engine, *xQASelection) xManualApplicationFactory
}

func PrepareXManualDevelopment(options config.Options) (XManualDevelopmentSelection, error) {
	if options.DevelopmentXManualPlan == "" && options.DevelopmentSessionReuse != "native-x-manual" {
		return nil, nil
	}
	if !options.Dev || options.DevelopmentSessionReuse != "native-x-manual" || options.DevelopmentXManualPlan == "" || options.DevelopmentQAPilotPlan != "" || options.DevelopmentQAPilotPrepare != "" || options.WindowsCaptureSplit || options.BrowserProfilePath != "" || options.BridgeExtensionPath != "" || options.DatabaseInspect || options.DatabaseAction != "" || options.RuntimeCandidateProbe || options.DiscoverCodex || options.DiscoverChromium {
		return nil, errors.New("manual X application requires explicit isolated development selection")
	}
	// Check caller overrides before the shared selector can create its empty DB.
	root := filepath.Dir(options.DevelopmentXManualPlan)
	if options.ConfigPath != filepath.Join(root, "config", "sidecar.json") || options.DatabasePath != filepath.Join(root, "state", "pilot.db") || options.Port < 1024 || options.Port == 11122 {
		return nil, errors.New("manual X configuration/database overrides must be instance-local")
	}
	var plan XApplicationQAPlan
	planPin, err := xQAReadPinned(options.DevelopmentXManualPlan, &plan)
	if err != nil {
		return nil, errors.New("manual X plan unavailable")
	}
	defer planPin.Close()
	if options.Port != plan.Port || options.DatabasePath != plan.Database {
		return nil, errors.New("manual X overrides do not match pinned selection")
	}
	if options.AppShell && options.ChromiumPath != plan.Chrome.Path {
		return nil, errors.New("manual X UI requires the selected Chrome image")
	}
	pin, _, err := xQAPinFile(options.ConfigPath)
	if err != nil {
		return nil, errors.New("manual X instance configuration must be an ordinary pinned file")
	}
	// Reject daily-mode mixing before the QA selector creates isolated state.
	var configuration config.Config
	if json.NewDecoder(pin).Decode(&configuration) != nil || configuration.Capture.ValidateXMode() != nil || configuration.Capture.XCaptureMode() != config.XCaptureExclusive {
		pin.Close()
		return nil, errors.New("manual X configuration cannot select daily concurrent capture")
	}
	s, err := openXQASelection(context.Background(), options.DevelopmentXManualPlan)
	if err != nil {
		pin.Close()
		return nil, err
	}
	s.pins = append(s.pins, pin)
	v := &xManualDevelopmentEntry{selection: s, clock: xQASystemClock{}}
	if options.AppShell {
		if err := v.prepareUI(); err != nil {
			v.Close()
			return nil, err
		}
	}
	return v, nil
}

func (v *xManualDevelopmentEntry) prepareUI() error {
	root := filepath.Join(v.selection.plan.Root, "profiles", "ui")
	directory := filepath.Join(root, "profile")
	if os.Mkdir(root, 0700) != nil || os.Mkdir(directory, 0700) != nil {
		return errors.New("fresh manual X UI profile required")
	}
	r, err := ownedprocess.DirectoryID(root)
	if err != nil {
		return errors.New("manual X UI identity unavailable")
	}
	d, err := ownedprocess.DirectoryID(directory)
	if err != nil {
		return errors.New("manual X UI identity unavailable")
	}
	v.uiLease, err = ownedprofile.Acquire(ownedprofile.Descriptor{Root: root, Directory: directory, RootID: r, DirectoryID: d})
	if err != nil {
		return errors.New("manual X UI lease unavailable")
	}
	v.uiExtension = filepath.Join(v.selection.plan.Root, "ui-reader-broker")
	if uireaderbroker.Materialize(v.uiExtension, "http://127.0.0.1:"+fmt.Sprint(v.selection.plan.Port)) != nil {
		return errors.New("manual X UI broker assets unavailable")
	}
	for _, name := range []string{"manifest.json", "content.js", "service-worker.js"} {
		f, _, err := xQAPinFile(filepath.Join(v.uiExtension, name))
		if err != nil {
			return errors.New("manual X UI broker pin unavailable")
		}
		v.selection.pins = append(v.selection.pins, f)
	}
	return nil
}

func (v *xManualDevelopmentEntry) UIExtensionPath() string { return v.uiExtension }

func (v *xManualDevelopmentEntry) ValidateConfig(cfg config.Config) error {
	p := v.selection.plan
	if !cfg.Dev || cfg.Root != p.Root || cfg.Server.Host != "127.0.0.1" || cfg.Server.Port != p.Port || cfg.Database.Path != p.Database || cfg.WindowsCaptureSplit || cfg.Deployment.Mode == "production-installed-app" {
		return errors.New("manual X loaded configuration escaped isolated selection")
	}
	return nil
}

func (v *xManualDevelopmentEntry) OpenDatabase(settings domain.Settings) (*store.Store, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.closed || v.databaseOpened {
		return nil, errors.New("manual X database already consumed")
	}
	v.databaseOpened = true
	return store.OpenDevelopmentQADatabase(v.selection.plan.Database, settings, v.clock, v.selection.versionMarker)
}

func (v *xManualDevelopmentEntry) Attach(parent context.Context, e *Engine) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.closed || v.e != nil || !v.databaseOpened || parent == nil || parent.Err() != nil || e == nil || e.store == nil || e.store.Path() != v.selection.plan.Database || v.ValidateConfig(e.config) != nil {
		return errors.New("manual X attachment scope rejected")
	}
	e.xManual.mu.Lock()
	defer e.xManual.mu.Unlock()
	if e.xManual.closed || e.xManual.route != nil || e.xManual.starter != nil {
		return errors.New("manual X application already configured")
	}
	v.e = e
	v.parent = parent
	e.xManual.starter = v
	return nil
}

func (v *xManualDevelopmentEntry) status() XManualStatus {
	v.mu.Lock()
	defer v.mu.Unlock()
	s := XManualStatus{Source: "x", State: "disabled", Reason: "explicit-start-required", Available: !v.attempted && !v.closed && !v.stopped}
	if v.startDone != nil {
		select {
		case <-v.startDone:
		default:
			s.State = "starting"
			if v.stopped || v.closed {
				s.State = "stopping"
			}
			s.Reason = ""
			s.Available = false
			return s
		}
	}
	if v.stopped || v.closed {
		s.State = "stopped"
		s.Reason = "opted-out"
		s.Available = false
	} else if v.attempted {
		s.State = "failed"
		s.Reason = "start-consumed"
		s.Available = false
	}
	if v.firstReason != "" {
		s.Reason = v.firstReason
	}
	return s
}

func (v *xManualDevelopmentEntry) failStart(reason string) error {
	v.mu.Lock()
	if v.firstReason == "" {
		v.firstReason = reason
	}
	v.mu.Unlock()
	return errors.New("manual X start failed: " + reason)
}

func (v *xManualDevelopmentEntry) stop() {
	v.mu.Lock()
	v.stopped = true
	cancel := v.cancelStart
	v.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (v *xManualDevelopmentEntry) waitStart(ctx context.Context) error {
	v.mu.Lock()
	done := v.startDone
	v.mu.Unlock()
	if done == nil {
		return nil
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return errors.New("manual X startup cleanup incomplete")
	}
}

func (v *xManualDevelopmentEntry) startPending() bool {
	v.mu.Lock()
	done := v.startDone
	v.mu.Unlock()
	if done == nil {
		return false
	}
	select {
	case <-done:
		return false
	default:
		return true
	}
}

func (v *xManualDevelopmentEntry) start(ctx context.Context, request XManualStartRequest) error {
	v.mu.Lock()
	if v.e == nil || v.closed || v.stopped || v.attempted || ctx == nil || ctx.Err() != nil || v.parent.Err() != nil || !request.AcknowledgeLimitations || !request.AllowOneSessionDerivation {
		v.mu.Unlock()
		return errors.New("manual X start unavailable or consumed")
	}
	v.attempted = true
	v.startDone = make(chan struct{})
	call, cancel := context.WithCancel(ctx)
	v.cancelStart = cancel
	e := v.e
	parent := v.parent
	v.mu.Unlock()
	stopParent := context.AfterFunc(parent, cancel)
	defer func() {
		stopParent()
		cancel()
		v.mu.Lock()
		v.cancelStart = nil
		close(v.startDone)
		v.mu.Unlock()
	}()
	// Use the existing source gate before creating an eligible pending command.
	e.xManual.mu.Lock()
	if e.xManual.closed || e.xManual.starting || e.xManual.route != nil {
		e.xManual.mu.Unlock()
		return v.failStart("source-busy")
	}
	e.xManual.starting = true
	e.xManual.mu.Unlock()
	defer func() { e.xManual.mu.Lock(); e.xManual.starting = false; e.xManual.mu.Unlock() }()
	settings, err := e.store.GetSettings(call)
	if err != nil {
		return v.failStart("settings-unavailable")
	}
	settings.CollectionMode = "headless"
	settings.ActiveSources = []domain.Source{domain.SourceX}
	session, err := e.store.CreateUpdateSession(call, "Manual X development pilot", settings, domain.UpdatePolicy{Trigger: domain.UpdateTriggerUser, Delivery: domain.UpdateDeliveryVisible, BudgetAuthority: domain.BudgetAuthorityUser})
	if err != nil {
		return v.failStart("session-unavailable")
	}
	fail := func(reason string) error {
		_ = e.CancelSession(context.Background(), session.ID)
		return v.failStart(reason)
	}
	command, err := e.store.StartRun(call, session.Runs[0].ID, map[string]any{"captureRuntime": map[string]any{"driver": "headless"}, "scrolls": 1, "maxBlocksPerSnapshot": 5})
	if err != nil {
		return fail("command-unavailable")
	}
	factory := v.factory
	var build xManualApplicationFactory
	if factory != nil {
		build = factory(e, v.selection)
	} else {
		prepare, err := newXApplicationPostPreparer("http://" + e.config.Server.HostPort())
		if err != nil {
			return fail("reader-preparer-unavailable")
		}
		build = e.xApplicationNativeFactory(v.selection.owner, newXApplicationManualConsent(v.selection.owner, true), prepare)
	}
	r, err := e.startXManualApplication(call, parent, session.ID, command.RunID, command.ID, build)
	if err != nil {
		return fail("startup-failed")
	}
	r.step(parent)
	return nil
}

func (v *xManualDevelopmentEntry) Close() error {
	v.closeOnce.Do(func() { v.closeErr = v.closeOwned() })
	return v.closeErr
}

func (v *xManualDevelopmentEntry) closeOwned() error {
	v.mu.Lock()
	v.closed = true
	e := v.e
	v.mu.Unlock()
	v.stop()
	if e != nil {
		e.shutdownXManual()
		call, cancel := context.WithTimeout(context.Background(), nativePilotCleanup)
		defer cancel()
		if err := v.waitStart(call); err != nil {
			return err
		}
		s := e.XManualStatus()
		if s.Cleanup != "" && s.Cleanup != "verified" {
			return errors.New("manual X cleanup uncertain; selection remains pinned")
		}
	}
	if v.fixtureClose != nil {
		v.fixtureClose()
	}
	if v.uiLease != nil {
		_ = v.uiLease.Close()
	}
	v.selection.close()
	return nil
}
