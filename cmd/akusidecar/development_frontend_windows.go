//go:build windows && auth_session_dev && auth_session_fixture

package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/abangkis/AkuSidecar/internal/appshell"
	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/engine"
	"github.com/abangkis/AkuSidecar/internal/httpapi"
	"github.com/abangkis/AkuSidecar/internal/readerbroker"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type developmentFrontendDOM struct {
	ClickRequestID                                                                       string
	Title, Broker, RequestID                                                             string
	Ready, LinkRendered, LinkVisible, LinkEnabled, Rejected, RelayRejected, NativeOpened bool
	DenialNotice                                                                         bool
	Rect                                                                                 struct{ Left, Top, Right, Bottom float64 }
}

const developmentFrontendReadback = `(() => {
 const a=document.querySelector('a[data-aku-native-post="x"][href="https://x.com/aku_fixture_only/status/1"]');
 const r=a?.getBoundingClientRect();const traces=window.akuNativePostDiagnostics?.read()||[];
 const terminal=traces.findLast(e=>e.phase==='terminal'&&e.outcome==='rejected'&&/^broker_[a-f0-9]{32}$/.test(e.trace));
 return {Title:document.title,Broker:window.akuReaderBrokerStatus||'',Ready:document.readyState==='complete'&&!!document.querySelector('#result-items [data-timeline-id]'),
 LinkRendered:!!r&&r.width>0&&r.height>0&&getComputedStyle(a).visibility==='visible',
 LinkVisible:!!r&&r.width>0&&r.height>0&&r.top>=0&&r.left>=0&&r.bottom<=innerHeight&&r.right<=innerWidth&&getComputedStyle(a).visibility==='visible',
 LinkEnabled:!!a&&!a.dataset.akuNativeWait&&!a.hasAttribute('aria-disabled'),Rect:r?{Left:r.left,Top:r.top,Right:r.right,Bottom:r.bottom}:{},
 RequestID:terminal?.trace||traces.findLast(e=>e.phase==='terminal'&&e.outcome==='opened')?.trace||'',ClickRequestID:traces.findLast(e=>e.phase==='click'&&/^broker_[a-f0-9]{32}$/.test(e.trace))?.trace||'',Rejected:!!terminal,RelayRejected:!!terminal&&traces.some(e=>e.trace===terminal.trace&&e.phase==='relay_request_end'&&e.status===409),
 NativeOpened:traces.some(e=>e.phase==='terminal'&&e.outcome==='opened'),
 DenialNotice:document.querySelector('#provider-notice')?.getAttribute('role')==='alert'&&document.querySelector('#provider-notice')?.textContent.includes('QA frontend: native activation is disabled.')&&getComputedStyle(document.querySelector('#provider-notice')).display!=='none'};})()`

func developmentFrontendSnapshot(ctx context.Context, p appshell.CaptureProtocol, session string) (developmentFrontendDOM, error) {
	var result developmentFrontendDOM
	raw, err := p.Call(ctx, "Runtime.evaluate", map[string]any{"expression": developmentFrontendReadback, "returnByValue": true}, session)
	if err != nil {
		return result, err
	}
	var reply struct {
		Result           struct{ Value developmentFrontendDOM }
		ExceptionDetails json.RawMessage
	}
	if err = json.Unmarshal(raw, &reply); err != nil {
		return result, err
	}
	if len(reply.ExceptionDetails) > 0 {
		return result, errors.New("frontend fixed readback failed")
	}
	return reply.Result.Value, nil
}

func runDevelopmentFrontend(ctx context.Context, r *engine.DevelopmentFixtureRuntime) (returnErr error) {
	return runDevelopmentFrontendMode(ctx, r, false)
}
func runDevelopmentFrontendNative(ctx context.Context, r *engine.DevelopmentFixtureRuntime) error {
	return runDevelopmentFrontendMode(ctx, r, true)
}
func runDevelopmentFrontendMode(ctx context.Context, r *engine.DevelopmentFixtureRuntime, native bool) (returnErr error) {
	if os.Getenv("AKU_DEV_FRONTEND_ENABLED") != "1" {
		return errors.New("frontend fixture requires explicit staged validation")
	}
	project := os.Getenv("AKU_DEV_FRONTEND_PROJECT")
	if !filepath.IsAbs(project) {
		return errors.New("frontend project path unavailable")
	}
	if err := r.VerifyFrontend(ctx); err != nil {
		return err
	}
	seed := engine.SeedDevelopmentFrontendTimeline
	if native {
		seed = engine.SeedDevelopmentFrontendNativeTimeline
	}
	if _, err := seed(ctx, r.Engine); err != nil {
		return err
	}
	cfg := config.Config{Dev: true, Root: r.Root, WindowsCaptureSplit: true, Server: config.ServerConfig{Host: "127.0.0.1", Port: 0}}
	server, err := httpapi.New(cfg, r.State, r.Engine, log.New(io.Discard, "", 0))
	if err != nil {
		return err
	}
	if err = server.SetSplitCaptureRuntime(r.Owner); err != nil {
		return err
	}
	title := httpapi.DevelopmentFrontendTitle
	if native {
		if r.NativeHost == nil || os.Getenv("AKU_DEV_FRONTEND_HEADLESS") != "0" {
			return errors.New("native frontend fresh registered frame required")
		}
		if err = r.NativeHost.ValidateApplied(filepath.Dir(r.Root)); err != nil {
			return err
		}
		title = httpapi.DevelopmentFrontendNativeTitle
		if err = server.LabelDevelopmentFrontend(true); err != nil {
			return err
		}
		server.SetSplitDirectNativeReader(r.PrepareNativePost, func(context.Context) error { return nil })
	} else {
		if err = server.CloseDevelopmentFrontendNativeActivation(); err != nil {
			return err
		}
	}
	observation, err := server.ObserveDevelopmentFrontend()
	if err != nil {
		return err
	}
	address, err := server.Start()
	if err != nil {
		return err
	}
	defer func() {
		stop, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := server.Stop(stop); err != nil {
			returnErr = err
		}
	}()
	origin := "http://" + address.String()
	stage := stageDevelopmentFrontendExtension
	if native {
		stage = func(p, root, origin string) (string, frontendStageProof, error) {
			return stageDevelopmentFrontendNativeExtension(p, root, origin, r.NativeHost.HostName)
		}
	}
	extension, proof, err := stage(project, r.Root, origin)
	if err != nil {
		return err
	}
	mainRoot := os.Getenv("AKU_CHROME_FIXTURE_ROOT")
	self, err := os.Executable()
	if err != nil || !strings.EqualFold(filepath.Dir(self), mainRoot) {
		return errors.New("frontend main outside QA staging")
	}
	payload := filepath.Join(mainRoot, "chromium", "bin", "chrome.exe")
	discovery, err := appshell.DiscoverSplitUI(ctx, self, payload)
	if err != nil {
		return err
	}
	profile := filepath.Join(r.Root, "frontend-profile")
	if _, err = os.Lstat(profile); !os.IsNotExist(err) {
		return errors.New("frontend UI profile must be fresh")
	}
	if err = os.Mkdir(profile, 0700); err != nil {
		return err
	}
	// Re-read payload safety before launch. There is no native helper/host setup.
	if err = verifyDevelopmentFrontendStage(project, extension, proof); err != nil {
		return err
	}
	if !native {
		if _, err = os.Stat(filepath.Join(mainRoot, "aku-reader-broker.exe")); !os.IsNotExist(err) {
			return errors.New("frontend QA must not stage a native helper")
		}
	} else {
		if err = r.NativeHost.ValidateApplied(mainRoot); err != nil {
			return err
		}
	}
	batch, err := r.StartBatch(ctx)
	if err != nil {
		return err
	}
	defer r.StopCollection()
	collectorPID := r.CurrentPID()
	before := r.Owner.Snapshot()
	headless := os.Getenv("AKU_DEV_FRONTEND_HEADLESS") == "1"
	lifetime := os.Getenv("AKU_DEV_FRONTEND_LIFETIME_ENABLED") == "1"
	args := []string{"--disable-extensions-except=" + extension, "--window-size=1440,1000"}
	if headless {
		args = append(args, "--headless=new")
	}
	if native {
		args = append(args, "--enable-features=LaunchWindowsNativeHostsDirectly")
	}
	ui, err := appshell.LaunchSession(ctx, appshell.LaunchOptions{Executable: discovery.Executable, ExtensionPath: extension, UserDataDir: profile, URL: origin + "/", NormalWindow: true, PrivateCDP: true, SuppressStartupWindow: true, ExtraArgs: args}, server.SetAppShellStartup, func(error) {})
	if err != nil {
		return err
	}
	defer ui.Terminate()
	type nativeClick struct {
		Request readerbroker.Request
		Helper  appshell.DevelopmentReaderHelperProof
	}
	clicks := make(chan nativeClick, 2)
	activations := make(chan developmentActivation, 2)
	var helperMu sync.Mutex
	var helper appshell.DevelopmentReaderHelperProof
	if native {
		brokerCtx, cancelBroker := context.WithCancel(ctx)
		defer cancelBroker()
		done := make(chan error, 1)
		go func() {
			done <- ui.ServeDevelopmentReaderBrokerFixture(brokerCtx, r.NativeHost.Pipe, func(c context.Context, request readerbroker.Request, exchange func(readerbroker.Target) (readerbroker.Reply, error)) error {
				helperMu.Lock()
				proof := helper
				helperMu.Unlock()
				select {
				case clicks <- nativeClick{request, proof}:
				default:
					return errors.New("native frontend duplicate helper call")
				}
				return server.HandleReaderBroker(c, request, func(target readerbroker.Target) (readerbroker.Reply, error) {
					reply, err := exchange(target)
					activations <- developmentActivation{target, reply}
					return reply, err
				})
			}, func(proof appshell.DevelopmentReaderHelperProof) { helperMu.Lock(); helper = proof; helperMu.Unlock() })
		}()
		defer func() {
			cancelBroker()
			select {
			case err := <-done:
				if err != nil {
					returnErr = err
				}
			case <-time.After(3 * time.Second):
				returnErr = errors.New("native frontend broker shutdown incomplete")
			}
		}()
	}
	protocol, err := ui.DevelopmentFrontendProtocol()
	if err != nil {
		return err
	}
	var session string
	workerSeen := false
	if err = developmentWait(ctx, func() bool {
		raw, e := protocol.Call(ctx, "Target.getTargets", nil, "")
		if e != nil {
			return false
		}
		var targets struct {
			TargetInfos []struct{ TargetID, Type, URL string }
		}
		if json.Unmarshal(raw, &targets) != nil {
			return false
		}
		for _, target := range targets.TargetInfos {
			if target.Type == "service_worker" && target.URL == "chrome-extension://"+proof.ExtensionID+"/service-worker.js" {
				workerSeen = true
			}
		}
		for _, target := range targets.TargetInfos {
			if target.Type == "page" && strings.HasPrefix(target.URL, origin+"/") {
				raw, e = protocol.Call(ctx, "Target.attachToTarget", map[string]any{"targetId": target.TargetID, "flatten": true}, "")
				if e != nil {
					return false
				}
				var attached struct{ SessionID string }
				if json.Unmarshal(raw, &attached) == nil && attached.SessionID != "" {
					session = attached.SessionID
					return true
				}
			}
		}
		return false
	}); err != nil {
		return err
	}
	var dom developmentFrontendDOM
	var early *nativeClick
	readyCtx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	if err = developmentWait(readyCtx, func() bool {
		var e error
		dom, e = developmentFrontendSnapshot(readyCtx, protocol, session)
		if native && early == nil {
			select {
			case click := <-clicks:
				early = &click
			default:
			}
		}
		return e == nil && dom.Ready && dom.Broker == "ready" && dom.LinkRendered && (dom.LinkEnabled || (early != nil && dom.ClickRequestID == early.Request.RequestID)) && dom.Title == title
	}); err != nil {
		return fmt.Errorf("frontend DOM-ready/readback failed: %w safeDOM=%+v safeStats=%+v", err, dom, observation.Snapshot())
	}
	stats := observation.Snapshot()
	// Startup WATCH/READY messages wake the exact staged worker. Check its
	// actual extension URL without invoking any action or logging capabilities.
	if !workerSeen {
		raw, e := protocol.Call(ctx, "Target.getTargets", nil, "")
		if e != nil {
			return e
		}
		var targets struct{ TargetInfos []struct{ Type, URL string } }
		if json.Unmarshal(raw, &targets) != nil {
			return errors.New("frontend worker target readback failed")
		}
		for _, target := range targets.TargetInfos {
			if target.Type == "service_worker" && target.URL == "chrome-extension://"+proof.ExtensionID+"/service-worker.js" {
				workerSeen = true
			}
		}
	}
	if stats.Bootstrap < 1 || stats.AppAsset < 1 || stats.RelayAsset < 1 || !workerSeen || r.CaptureCount() != 1 || (!native && r.Owner.Snapshot() != before) {
		return errors.New("frontend real assets/batch ownership proof failed")
	}
	uiReadback := ui.DevelopmentFrontendWindowReadback()
	if lifetime {
		uiReadback = ui.DevelopmentFrontendLifetimeWindowReadback()
	}
	if !headless && !lifetime {
		uiReadback, err = ui.PresentDevelopmentFrontend(ctx, title, func() bool { helperMu.Lock(); defer helperMu.Unlock(); return native && helper.PID != 0 })
		if err != nil || !uiReadback.Visible || uiReadback.Minimized || uiReadback.HWND == 0 || !uiReadback.JobMember || !uiReadback.TitleMatched {
			return fmt.Errorf("physical frontend window presentation/readback failed: %v safeWindow=%+v", err, uiReadback)
		}
	}
	if native && early == nil {
		select {
		case click := <-clicks:
			early = &click
		default:
		}
	}
	stageName := "dev_frontend_ready"
	nativeHelperCount := 0
	if native {
		stageName = "dev_frontend_native_ready"
		nativeHelperCount = 1
	}
	if err = r.Emit(map[string]any{"Stage": stageName, "PID": os.Getpid(), "UIPID": ui.PID(), "UIHWND": uiReadback.HWND, "Root": r.Root, "SessionID": batch.ID, "Frontend": map[string]any{"Origin": origin, "Title": dom.Title, "Version": discovery.Version, "StageProof": proof, "DOM": dom, "Stats": stats, "UIWindow": uiReadback, "ExactWorkerObserved": workerSeen, "CollectorPID": collectorPID, "NormalWindow": true, "Headless": headless, "AlreadyClicked": early != nil, "NativeActivationClosed": !native, "NativeHelpersStaged": nativeHelperCount, "BrokerServerStarted": native}}); err != nil {
		return err
	}
	if lifetime {
		return verifyDevelopmentFrontendAutomaticLifetime(ctx, r, ui, protocol, session, observation, batch, collectorPID)
	}
	if headless {
		if err := developmentFrontendScreenshot(ctx, protocol, session, filepath.Join(r.Root, "frontend-viewport.png")); err != nil {
			return err
		}
		control, err := r.ReadControl()
		if err != nil || control != "finish_prepare" {
			return errors.New("headless frontend preparation cannot accept native actions")
		}
		return r.Emit(map[string]any{"Stage": "dev_frontend_prepared", "PID": os.Getpid(), "Frontend": map[string]any{"Headless": true, "DOM": dom, "Stats": stats, "NativeHelpersStaged": 0, "BrokerServerStarted": false}})
	}
	controls := make(chan string, 1)
	go func() {
		for {
			c, e := r.ReadControl()
			if e != nil {
				controls <- "controller_closed"
				return
			}
			controls <- c
		}
	}()
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	var deadline <-chan time.Time
	if native {
		var click nativeClick
		if early != nil {
			click = *early
		} else {
			waiting := true
			for waiting {
				select {
				case click = <-clicks:
					waiting = false
				case c := <-controls:
					if c != "operator_cue" || deadline != nil {
						return errors.New("native frontend cue invalid")
					}
					deadline = time.After(120 * time.Second)
				case <-deadline:
					return errors.New("native frontend operator deadline: no trusted helper click")
				case <-ctx.Done():
					return ctx.Err()
				case <-ui.Done():
					return errors.New("native frontend UI exited")
				case <-tick.C:
					dom, err = developmentFrontendSnapshot(ctx, protocol, session)
					if err != nil {
						return err
					}
					if dom.Rejected {
						return errors.New("native frontend click rejected before authorized helper")
					}
				}
			}
		}
		return verifyDevelopmentFrontendNativeJourney(ctx, r, server, origin, ui, protocol, session, click.Request, click.Helper, activations, observation, batch, collectorPID, controls)
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ui.Done():
			return errors.New("frontend UI exited before acceptance")
		case c := <-controls:
			if c != "operator_cue" {
				return errors.New("frontend controller closed or unsupported control")
			}
			if deadline == nil {
				deadline = time.After(120 * time.Second)
			}
		case <-deadline:
			return errors.New("frontend operator deadline: no correlated physical native-post denial")
		case <-tick.C:
			dom, err = developmentFrontendSnapshot(ctx, protocol, session)
			if err != nil {
				return err
			}
			if dom.NativeOpened {
				return errors.New("frontend native opened despite closed activation")
			}
			stats = observation.Snapshot()
			if dom.Rejected && dom.RelayRejected && dom.LinkVisible && dom.DenialNotice && stats.NativeDenied == 1 && stats.Cancelled > 0 && dom.RequestID == stats.RequestID {
				if err = r.VerifyFrontend(ctx); err != nil {
					return err
				}
				if r.CurrentPID() != collectorPID || r.Owner.Snapshot() != before || r.CaptureCount() != 1 || r.Coordinator.Status().BatchPaused {
					return errors.New("closed frontend action changed capture ownership")
				}
				audit, e := r.State.ReadSplitActionAudit(ctx, "", 20)
				if e != nil {
					return e
				}
				rejected := 0
				for _, a := range audit {
					if a.ActionType == "open_native_post" && a.Phase == "result" && a.Outcome == "rejected" {
						rejected++
					} else {
						return errors.New("frontend audit reached native admission")
					}
				}
				if rejected != 1 {
					return errors.New("frontend rejected audit missing")
				}
				return r.Emit(map[string]any{"Stage": "dev_frontend_denied", "PID": os.Getpid(), "UIPID": ui.PID(), "PhysicalOpen": true, "Root": r.Root, "Frontend": map[string]any{"DOM": dom, "Stats": stats, "AuditRejected": rejected, "CollectorUnchanged": true, "CaptureActive": true, "NativeHelpersStaged": 0, "BrokerServerStarted": false}})
			}
		}
	}
}

// Page pixels only: this never captures browser chrome, an address bar or the
// acknowledgement fragment, and never changes viewport/scroll/input state.
func developmentFrontendScreenshot(ctx context.Context, p appshell.CaptureProtocol, session, path string) error {
	raw, err := p.Call(ctx, "Page.getLayoutMetrics", nil, session)
	if err != nil {
		return err
	}
	var metrics struct {
		CSSContentSize struct{ Width, Height float64 }
	}
	if json.Unmarshal(raw, &metrics) != nil || metrics.CSSContentSize.Width <= 0 || metrics.CSSContentSize.Height <= 0 {
		return errors.New("frontend screenshot dimensions unavailable")
	}
	if metrics.CSSContentSize.Height > 4096 {
		metrics.CSSContentSize.Height = 4096
	}
	raw, err = p.Call(ctx, "Page.captureScreenshot", map[string]any{"format": "png", "captureBeyondViewport": true, "clip": map[string]any{"x": 0, "y": 0, "width": metrics.CSSContentSize.Width, "height": metrics.CSSContentSize.Height, "scale": 1}}, session)
	if err != nil {
		return err
	}
	var reply struct{ Data string }
	if json.Unmarshal(raw, &reply) != nil {
		return errors.New("frontend screenshot response invalid")
	}
	pixels, err := base64.StdEncoding.DecodeString(reply.Data)
	if err != nil || len(pixels) < 8 {
		return errors.New("frontend screenshot pixels missing")
	}
	return os.WriteFile(path, pixels, 0600)
}
