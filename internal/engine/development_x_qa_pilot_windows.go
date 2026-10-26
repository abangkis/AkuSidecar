//go:build windows && auth_session_dev

package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/readerbroker"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
	"github.com/abangkis/AkuSidecar/internal/store"
	"golang.org/x/sys/windows"
)

type XApplicationQAOptions struct {
	Dev       bool
	PlanPath  string
	Synthetic bool
}

type XApplicationQACommand struct {
	Op                        string                `json:"op"`
	AcknowledgeLimitations    bool                  `json:"acknowledgeLimitations,omitempty"`
	AllowOneSessionDerivation bool                  `json:"allowOneSessionDerivation,omitempty"`
	Request                   *readerbroker.Request `json:"request,omitempty"`
}

func DecodeXApplicationQACommand(raw []byte) (XApplicationQACommand, error) {
	var v XApplicationQACommand
	if xManualPilotDecode(raw, &v) != nil {
		return v, errors.New("QA command invalid")
	}
	valid := false
	switch v.Op {
	case "start":
		valid = v.Request == nil
	case "open_post":
		valid = v.Request != nil && !v.AcknowledgeLimitations && !v.AllowOneSessionDerivation
	case "status", "settle", "stop":
		valid = v.Request == nil && !v.AcknowledgeLimitations && !v.AllowOneSessionDerivation
	}
	if !valid {
		return v, errors.New("QA command unavailable")
	}
	return v, nil
}

type XApplicationQAReport struct {
	Status          XManualStatus `json:"status"`
	Synthetic       bool          `json:"synthetic"`
	NativeQualified bool          `json:"nativeQualified"`
	Observations    int           `json:"observations"`
	Blocks          int           `json:"blocks"`
}
type xQASyntheticKit struct {
	clock      store.Clock
	factory    func(*Engine) xManualApplicationFactory
	foreground func(context.Context, readerbroker.Target) error
	close      func()
}

// Installed only by the combined fixture build. The native factory never calls
// this registration and public reports cannot select or supply a backend.
var newXQASyntheticKit func() (*xQASyntheticKit, error)

type xQASystemClock struct{}

func (xQASystemClock) Now() time.Time { return time.Now() }

type XApplicationQAPilot struct {
	mu        sync.Mutex
	selection *xQASelection
	e         *Engine
	state     *store.Store
	parent    context.Context
	cancel    context.CancelFunc
	synthetic bool
	kit       *xQASyntheticKit
	attempted bool
	runtime   *xManualApplication
	run       string
	closeOnce sync.Once
	closeErr  error
}

func (*XApplicationQAPilot) String() string     { return "private-isolated-X-QA-pilot" }
func (q *XApplicationQAPilot) GoString() string { return q.String() }

func OpenXApplicationQAPilot(parent context.Context, options XApplicationQAOptions) (q *XApplicationQAPilot, err error) {
	if !options.Dev || parent == nil || parent.Err() != nil || options.PlanPath == "" || options.Synthetic && newXQASyntheticKit == nil {
		return nil, errors.New("isolated QA requires explicit supported development opt-in")
	}
	s, err := openXQASelection(parent, options.PlanPath)
	if err != nil {
		return nil, err
	}
	life, cancel := context.WithCancel(parent)
	q = &XApplicationQAPilot{selection: s, parent: life, cancel: cancel, synthetic: options.Synthetic}
	owned := q
	defer func() {
		if err != nil {
			cancel()
			if owned.state != nil {
				owned.state.Close()
			}
			if owned.kit != nil {
				owned.kit.close()
			}
			s.close()
		}
	}()
	if options.Synthetic {
		q.kit, err = newXQASyntheticKit()
		if err != nil {
			return nil, errors.New("synthetic QA unavailable")
		}
	}
	settings := domain.DefaultSettings("expanded", "quiet", "promote_unused_budget", false)
	var clock store.Clock = xQASystemClock{}
	if q.kit != nil {
		clock = q.kit.clock
	}
	q.state, err = store.OpenDevelopmentQADatabase(s.plan.Database, settings, clock, s.versionMarker)
	if err != nil {
		return nil, errors.New("isolated QA database unavailable")
	}
	cfg := config.Config{Root: s.plan.Root, Dev: true, Capture: config.CaptureConfig{MaxAcquisitionRounds: 1}}
	q.e = New(q.state, reasoning.Deterministic{}, cfg, log.New(io.Discard, "", 0))
	return q, nil
}
func (q *XApplicationQAPilot) Origin() string {
	return "http://127.0.0.1:" + fmt.Sprint(q.selection.plan.Port)
}
func (q *XApplicationQAPilot) Start(ctx context.Context, acknowledgeLimitations, allowOneSessionDerivation bool) error {
	q.mu.Lock()
	if q.attempted || !acknowledgeLimitations || allowOneSessionDerivation == q.synthetic || ctx == nil || ctx.Err() != nil || q.parent.Err() != nil {
		q.mu.Unlock()
		return errors.New("explicit one-use QA start rejected")
	}
	q.attempted = true
	q.mu.Unlock()
	settings, err := q.state.GetSettings(ctx)
	if err != nil {
		return errors.New("QA settings unavailable")
	}
	settings.CollectionMode = "headless"
	settings.ActiveSources = []domain.Source{domain.SourceX}
	session, err := q.state.CreateUpdateSession(ctx, "Isolated manual X QA pilot", settings, domain.UpdatePolicy{Trigger: domain.UpdateTriggerUser, Delivery: domain.UpdateDeliveryVisible, BudgetAuthority: domain.BudgetAuthorityUser})
	if err != nil {
		return errors.New("QA session unavailable")
	}
	command, err := q.state.StartRun(ctx, session.Runs[0].ID, map[string]any{"captureRuntime": map[string]any{"driver": "headless"}, "scrolls": 1, "maxBlocksPerSnapshot": 5})
	if err != nil {
		_ = q.e.CancelSession(context.Background(), session.ID)
		return errors.New("QA command unavailable")
	}
	q.mu.Lock()
	q.run = command.RunID
	q.mu.Unlock()
	var factory xManualApplicationFactory
	if q.kit != nil {
		factory = q.kit.factory(q.e)
	} else {
		prepare, prepareErr := newXApplicationPostPreparer(q.Origin())
		if prepareErr != nil {
			_ = q.e.CancelSession(context.Background(), session.ID)
			return errors.New("QA reader preparer unavailable")
		}
		factory = q.e.xApplicationNativeFactory(q.selection.owner, newXApplicationManualConsent(q.selection.owner, true), prepare)
	}
	r, err := q.e.startXManualApplication(ctx, q.parent, session.ID, command.RunID, command.ID, factory)
	q.mu.Lock()
	q.runtime = r
	q.mu.Unlock()
	if err != nil {
		_ = q.e.CancelSession(context.Background(), session.ID)
		return errors.New("QA startup unavailable")
	}
	r.step(q.parent)
	return nil
}
func (q *XApplicationQAPilot) Report() XApplicationQAReport {
	out := XApplicationQAReport{Status: q.e.XManualStatus(), Synthetic: q.synthetic}
	q.mu.Lock()
	run := q.run
	q.mu.Unlock()
	if run != "" {
		if observations, err := q.state.Observations(q.parent, run); err == nil {
			out.Observations = len(observations)
			for _, o := range observations {
				for _, s := range o.Snapshots {
					out.Blocks += len(s.Blocks)
				}
			}
		}
	}
	return out
}
func (q *XApplicationQAPilot) OpenPost(ctx context.Context, request readerbroker.Request) error {
	marker := "/split-reader-intent?id=" + domain.NewID("split")
	prepared, selected, err := q.e.PrepareXManualReader(ctx, request, marker)
	if err != nil || !selected || prepared.VerifyForeground == nil || prepared.NavigatePost == nil {
		return errors.New("QA reader preparation rejected")
	}
	wait := waitXQAReaderForeground
	if q.kit != nil {
		wait = q.kit.foreground
	}
	if err = wait(ctx, prepared.Target); err != nil {
		return errors.New("QA reader requires manual foreground")
	}
	if prepared.VerifyForeground(ctx) != nil {
		return errors.New("QA reader foreground verification rejected")
	}
	if prepared.NavigatePost(ctx) != nil {
		return errors.New("QA reader navigation rejected")
	}
	return nil
}
func waitXQAReaderForeground(ctx context.Context, target readerbroker.Target) error {
	if ctx == nil || target.HWND == 0 || target.PID == 0 || !time.Now().Before(target.Expires) {
		return errors.New("QA target unavailable")
	}
	call, cancel := context.WithDeadline(ctx, target.Expires)
	defer cancel()
	foreground := windows.NewLazySystemDLL("user32.dll").NewProc("GetForegroundWindow")
	tick := time.NewTicker(25 * time.Millisecond)
	defer tick.Stop()
	for {
		hwnd, _, _ := foreground.Call()
		if uint64(hwnd) == target.HWND {
			return nil
		}
		select {
		case <-call.Done():
			return errors.New("QA target did not become foreground")
		case <-tick.C:
		}
	}
}
func (q *XApplicationQAPilot) Settle(ctx context.Context) error {
	q.mu.Lock()
	r := q.runtime
	q.mu.Unlock()
	if r == nil {
		return errors.New("QA capture not started")
	}
	r.mu.Lock()
	started := r.captureStarted
	r.mu.Unlock()
	if !started || ctx == nil {
		return errors.New("QA capture not started")
	}
	select {
	case <-r.captureDone:
	case <-ctx.Done():
		return errors.New("QA capture did not settle")
	}
	if !q.e.WaitForIdle(nativePilotCleanup) {
		return errors.New("QA reasoning did not settle")
	}
	return nil
}
func (q *XApplicationQAPilot) Stop(ctx context.Context) error { return q.e.StopXManual(ctx) }
func (q *XApplicationQAPilot) Close() error {
	q.closeOnce.Do(func() {
		q.cancel()
		q.e.Shutdown()
		status := q.e.XManualStatus()
		if status.State != "disabled" && status.Cleanup != "verified" || !q.e.WaitForIdle(nativePilotCleanup) {
			q.closeErr = errors.New("QA cleanup uncertain")
			return
		}
		if q.kit != nil {
			q.kit.close()
		}
		if q.state.Close() != nil {
			q.closeErr = errors.New("QA database close uncertain")
			return
		}
		q.selection.close()
	})
	return q.closeErr
}
