//go:build windows && auth_session_dev && auth_session_fixture

package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/abangkis/AkuSidecar/internal/appshell"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/readerbroker"
	"github.com/abangkis/AkuSidecar/internal/store"
	"golang.org/x/sys/windows"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
	"unsafe"
)

func developmentNativeFrame(t *testing.T, origin string) (chromeFixtureConfig, processFixtureStart) {
	t.Helper()
	cfg := chromeFixtureConfigFromEnv(t, origin)
	parent, err := processFixtureProcessIdentity(uint32(os.Getpid()))
	if err != nil {
		t.Fatal(err)
	}
	rootID, err := processFixtureDirectoryID(cfg.Root)
	if err != nil {
		t.Fatal(err)
	}
	lineage := processFixtureLineage{Root: cfg.Root, RootID: rootID, Nonce: readerbroker.Secret(), Parent: parent}
	raw, _ := json.Marshal(lineage)
	if err = os.WriteFile(filepath.Join(cfg.Root, "process-controller-lineage.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	image, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := readerbroker.ReadFixtureNativeHostReceipt(os.Getenv("AKU_DEV_FRONTEND_NATIVE_RECEIPT"))
	if err != nil {
		t.Fatal(err)
	}
	return cfg, processFixtureStart{Grant: processFixtureGrant{processFixtureLineage: lineage}, NodePath: cfg.NodePath, ChromePath: cfg.ChromePath, DriverPath: cfg.DriverPath, Origin: cfg.Origin, AuthorityKey: lineage.Nonce, ControllerImage: image, NativeHost: &receipt}
}

// Actual scoped main/private stdin frame, with no registration or Chrome. Every
// rejected frame must leave both ordinary and fixture runtime unopened.
func TestDevelopmentFrontendNativeUnregisteredStartup(t *testing.T) {
	if os.Getenv("AKU_DEV_FRONTEND_NATIVE_ENABLED") != "1" {
		t.Skip("requires canonical native preparation")
	}
	for _, v := range []struct {
		name, reason, scenario string
		mutate                 func(*processFixtureStart)
	}{
		{"missing_receipt", "scoped startup frame rejected", "", func(s *processFixtureStart) { s.NativeHost = nil }},
		{"foreign_pipe", "scoped startup frame rejected", "", func(s *processFixtureStart) {
			copy := *s.NativeHost
			copy.Pipe = `\\.\pipe\AkuBrowser.fixture-reader.` + readerbroker.Secret()[:32]
			s.NativeHost = &copy
		}},
		{"unregistered", "registration missing; Chrome startup denied", "", func(*processFixtureStart) {}},
		{"unknown_scenario", "native frontend scenario rejected", "unapproved_scenario", func(*processFixtureStart) {}},
	} {
		t.Run(v.name, func(t *testing.T) {
			if v.scenario != "" {
				t.Setenv(developmentFrontendNativeScenarioEnv, v.scenario)
			}
			cfg, start := developmentNativeFrame(t, "http://127.0.0.1:43219")
			v.mutate(&start)
			child := startProcessFixtureChild(t, filepath.Join(os.Getenv("AKU_CHROME_FIXTURE_ROOT"), "akusidecar.exe"), start)
			rejected := child.receive(t, "dev_frontend_native_failed")
			if !strings.Contains(rejected.Failure, v.reason) {
				t.Fatalf("wrong closed startup reason: %s", rejected.Failure)
			}
			select {
			case err := <-child.exited:
				if err == nil {
					t.Fatal("rejected native startup exited normally")
				}
			case <-time.After(5 * time.Second):
				t.Fatal("rejected native startup remained alive")
			}
			for _, name := range []string{"fixture.sqlite", "shared", "frontend-profile"} {
				if _, err := os.Lstat(filepath.Join(cfg.Root, name)); !os.IsNotExist(err) {
					t.Fatal("rejected native startup touched fixture state/profile", name)
				}
			}
			pids, err := (&chromeFixtureBridge{job: child.job}).jobPIDs()
			if err != nil || len(pids) != 0 {
				t.Fatal("rejected native startup left owned descendants")
			}
			t.Logf("case=%s actualMainRejected=true noSQLite=true noNodeChromeUI=true outerJobEmpty=true", v.name)
		})
	}
}

type nativeFrontendTestEvidence struct {
	frontendTestEvidence
	AlreadyClicked                                                                                         bool
	Helper                                                                                                 appshell.DevelopmentReaderHelperProof
	RequestID                                                                                              string
	ActualFrontendClick, HelperHandlePinned, HelperExitedNormally, NoKeeper, DuplicateStaleCancelledDenied bool
	CaptureReleased, ReasoningBlocked, CollectorRetained                                                   bool
	Pause                                                                                                  DevelopmentPauseEvidence
	ExpectedCollectorPID                                                                                   int
	Windows                                                                                                DevelopmentReaderWindowEvidence
	WindowFailures                                                                                         []string
	ExpectedReaderPID                                                                                      uint32
	ClosedBootstrapIndex                                                                                   int
	BootstrapCloseError                                                                                    string
	Scenario                                                                                               string
	QueuedRun                                                                                              domain.Run
	Commands                                                                                               []store.FixtureCommandEvidence
}

func validNativeFrontendPause(v nativeFrontendTestEvidence, persisted bool) bool {
	p := v.Pause
	return v.ExpectedCollectorPID > 0 && p.CollectorPID == v.ExpectedCollectorPID && p.ManagerPID == v.ExpectedCollectorPID && p.Driver == "headless" && p.Generation > 0 && p.ActiveLeases > 0 && p.AdmissionPaused && p.SQLiteError == "" && p.SQLitePaused == persisted && p.BeforeLeaseRelease == persisted && (!persisted || (p.CaptureSteps == 0 && p.ActiveWork == 0))
}

// This test has no synthetic UI input and cannot run during preparation. The
// Coordinator applies the reviewed unique QA host, launches this invocation and
// supplies the operational cue. An early real click proceeds immediately.
func TestDevelopmentFrontendActualMainPhysicalNative(t *testing.T) {
	runDevelopmentFrontendActualMainPhysicalNative(t, DevelopmentFrontendNativeScenarioNormal)
}

func TestDevelopmentFrontendReaderOpenParentDeathPhysicalNative(t *testing.T) {
	runDevelopmentFrontendActualMainPhysicalNative(t, DevelopmentFrontendNativeScenarioReaderOpenParentDeath)
}

func TestDevelopmentFrontendForegroundOnlyPhysicalNative(t *testing.T) {
	runDevelopmentFrontendActualMainPhysicalNative(t, DevelopmentFrontendNativeScenarioForegroundOnly)
}

func runDevelopmentFrontendActualMainPhysicalNative(t *testing.T, scenario string) {
	if os.Getenv("AKU_DEV_FRONTEND_NATIVE_ENABLED") != "1" || os.Getenv("AKU_DEV_FRONTEND_HEADLESS") != "0" {
		t.Skip("requires explicit registered physical native QA")
	}
	selectedScenario, scenarioErr := developmentFrontendNativeScenario()
	if scenarioErr != nil || selectedScenario != scenario {
		t.Fatalf("prepared native scenario mismatch expected=%q selected=%q err=%v", scenario, selectedScenario, scenarioErr)
	}
	root := os.Getenv("AKU_CHROME_FIXTURE_ROOT")
	cue := os.Getenv("AKU_DEV_FRONTEND_CUE")
	if !filepath.IsAbs(cue) || !samePath(filepath.Dir(cue), root) {
		t.Fatal("native operator cue outside exact retained scope")
	}
	if _, err := os.Lstat(cue); !os.IsNotExist(err) {
		t.Fatal("operator cue must be fresh")
	}
	receipt, err := readerbroker.ReadFixtureNativeHostReceipt(os.Getenv("AKU_DEV_FRONTEND_NATIVE_RECEIPT"))
	if err != nil {
		t.Fatal(err)
	}
	if err = receipt.ValidateApplied(root); err != nil {
		t.Fatal("registration readback failed before Chrome", err)
	}
	fixture := &chromeFixtureHTTP{}
	var nonce string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/process-authority" {
			if r.Header.Get("X-Fixture-Key") != nonce {
				http.Error(w, "denied", 403)
				return
			}
			v, _ := fixture.AuthorityEvidence(r.Context())
			_ = json.NewEncoder(w).Encode(v)
			return
		}
		if r.URL.Path == "/native-post" {
			cookie, e := r.Cookie("fixture_session")
			if e != nil {
				http.Error(w, "denied", 401)
				return
			}
			if _, ok := fixture.sessions.Load(cookie.Value); !ok {
				http.Error(w, "denied", 401)
				return
			}
			w.Header().Set("Content-Type", "text/html")
			w.Header().Set("Content-Security-Policy", "default-src 'self'; frame-ancestors 'none'")
			fmt.Fprint(w, `<!doctype html><title>Synthetic local post</title><h1 id="synthetic-native-post">Synthetic local post</h1>`)
			return
		}
		fixture.serveHTTP(w, r)
	}))
	defer server.Close()
	fixture.server = server
	cfg, start := developmentNativeFrame(t, server.URL)
	nonce = start.AuthorityKey
	child := startProcessFixtureChild(t, receipt.MainPath, start)
	ready := receiveDevelopmentReceipt(t, child, "dev_frontend_native_ready", 60*time.Second)
	var evidence nativeFrontendTestEvidence
	if err = json.Unmarshal(ready.Frontend, &evidence); err != nil {
		t.Fatal(err)
	}
	if ready.PID != child.identity.PID || ready.UIPID == 0 || ready.Root != cfg.Root || evidence.Headless || !evidence.NormalWindow || evidence.NativeActivationClosed || evidence.NativeHelpersStaged != 1 || !evidence.BrokerServerStarted || evidence.Title != "AkuBrowser QA - Synthetic native reader" || !evidence.DOM.Ready || !evidence.DOM.LinkRendered || (!evidence.DOM.LinkEnabled && !evidence.AlreadyClicked) || evidence.DOM.Broker != "ready" || !evidence.ExactWorkerObserved || !evidence.StageProof.NativePermission || !evidence.StageProof.NativeCalls || evidence.StageProof.QAHostName != receipt.HostName || evidence.StageProof.ExtensionID != readerbroker.ExtensionID || evidence.Stats.Bootstrap < 1 || evidence.Stats.AppAsset < 1 || evidence.Stats.RelayAsset < 1 || ready.UIHWND == 0 || ready.UIHWND != evidence.UIWindow.HWND || !evidence.UIWindow.Visible || evidence.UIWindow.Minimized {
		t.Fatalf("native actual frontend readiness invalid: %+v", evidence)
	}
	wstate := evidence.UIWindow
	expectedCollectorPID := evidence.CollectorPID
	if wstate.PID != ready.UIPID || !wstate.JobMember || !wstate.TitleMatched || wstate.Class != "Chrome_WidgetWin_1" || !wstate.MonitorKnown || !wstate.WorkAreaIntersection || !wstate.WithinWorkArea || (!wstate.Topmost && !wstate.AuthorizedHelperBegan) || !wstate.CloakKnown || wstate.Cloaked || !wstate.SessionKnown || wstate.WindowSession != wstate.ProcessSession || !wstate.InputDesktopKnown || !wstate.InputDesktopMatch {
		t.Fatalf("physical monitor/desktop/owned QA HWND proof incomplete: %+v", wstate)
	}
	uiIdentity := mustDevelopmentProcessIdentity(t, ready.UIPID)
	verifyProcessFixtureImage(t, uiIdentity, filepath.Join(root, "chromium", "bin", "chrome.exe"))
	if parent, e := readerbroker.ParentPID(ready.UIPID); e != nil || parent != ready.PID {
		t.Fatal("actual main does not own frontend UI")
	}
	operatorNonce := readerbroker.Secret()
	armed := map[string]any{"Stage": "ARMED", "Nonce": operatorNonce, "TaskRoot": root, "MainPID": ready.PID, "UIPID": ready.UIPID, "UIHWND": ready.UIHWND, "UIWindow": evidence.UIWindow, "Title": evidence.Title, "NativeActivationClosed": false, "AlreadyClicked": evidence.AlreadyClicked, "DOM": evidence.DOM, "TargetLabel": "Open source evidence", "Card": "QA native frontend: synthetic reader"}
	raw, _ := json.MarshalIndent(armed, "", "  ")
	if err = os.WriteFile(cue+".ready.json", raw, 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("READY/ARMED native actualMainPID=%d ownedUIPID=%d HWND=%d realLinkRendered=true enabled=%t viewportVisible=%t alreadyClicked=%t privateQAHost=true exactWorkerObserved=true taskRoot=%s cue=%s ready=%s; operator may SCROLL and click real Open source evidence once; early click proceeds immediately", ready.PID, ready.UIPID, ready.UIHWND, evidence.DOM.LinkEnabled, evidence.DOM.LinkVisible, evidence.AlreadyClicked, root, cue, cue+".ready.json")
	admin := time.NewTimer(8 * time.Minute)
	defer admin.Stop()
	tick := time.NewTicker(25 * time.Millisecond)
	defer tick.Stop()
	var click processFixtureReceipt
	cued := false
waitClick:
	for {
		select {
		case reply, ok := <-child.replies:
			if !ok || reply.Stage != "dev_frontend_native_click" || !reply.PhysicalOpen {
				t.Fatalf("native actual click missing stage=%s failure=%s", reply.Stage, reply.Failure)
			}
			click = reply
			break waitClick
		case <-tick.C:
			if !cued {
				if raw, e := os.ReadFile(cue); e == nil {
					var instruction struct {
						Nonce   string
						MainPID uint32
					}
					if json.Unmarshal(raw, &instruction) != nil || instruction.Nonce != operatorNonce || instruction.MainPID != ready.PID {
						t.Fatal("native cue identity mismatch")
					}
					if _, err = fmt.Fprintln(child.input, "operator_cue"); err != nil {
						t.Fatal(err)
					}
					cued = true
					t.Log("Coordinator cue received; physical operator deadline=120s; native helper clocks unchanged")
				}
			}
		case <-admin.C:
			t.Fatal("native operator administrative deadline; no click acceptance")
		}
	}
	if json.Unmarshal(click.Frontend, &evidence) != nil || !evidence.ActualFrontendClick || !evidence.HelperHandlePinned || evidence.Helper.PID != click.HelperPID || evidence.Helper.Parent != ready.UIPID || evidence.Helper.Image != receipt.HelperPath || !evidence.Helper.JobMember || !evidence.Helper.TopmostReleased || evidence.Helper.Created == "" || evidence.Helper.AgeMS < 0 || evidence.Helper.AgeMS >= readerbroker.PreparationLifetime.Milliseconds() {
		t.Fatal("actual Chrome native helper provenance missing")
	}
	// The source retained/revalidated this process handle while it was live.
	// Reopening the PID after its normal reply could refer to a recycled PID.
	admission := child.receive(t, "dev_frontend_native_admission_paused")
	if json.Unmarshal(admission.Frontend, &evidence) != nil || evidence.ExpectedCollectorPID != expectedCollectorPID || !validNativeFrontendPause(evidence, false) {
		t.Fatalf("native admission pause evidence invalid: %+v", evidence)
	}
	t.Logf("pausePhase=capture_blocked admissionPaused=%t sqlitePaused=%t sqliteError=%q collectorPID=%d managerPID=%d expectedCollectorPID=%d driver=%s activeLeases=%d captureSteps=%d activeWork=%d", evidence.Pause.AdmissionPaused, evidence.Pause.SQLitePaused, evidence.Pause.SQLiteError, evidence.Pause.CollectorPID, evidence.Pause.ManagerPID, evidence.ExpectedCollectorPID, evidence.Pause.Driver, evidence.Pause.ActiveLeases, evidence.Pause.CaptureSteps, evidence.Pause.ActiveWork)
	draining := child.receive(t, "dev_frontend_native_draining")
	if draining.DurablePause || draining.CaptureAndReasoningDrained || json.Unmarshal(draining.Frontend, &evidence) != nil || evidence.ExpectedCollectorPID != expectedCollectorPID || !evidence.CaptureReleased || !evidence.ReasoningBlocked || !evidence.CollectorRetained || !validNativeFrontendPause(evidence, false) {
		t.Fatal("frontend actual capture/reasoning drain boundary not proved")
	}
	t.Logf("pausePhase=reasoning_blocked admissionPaused=%t sqlitePaused=%t sqliteError=%q collectorPID=%d managerPID=%d driver=%s activeLeases=%d activeWork=%d", evidence.Pause.AdmissionPaused, evidence.Pause.SQLitePaused, evidence.Pause.SQLiteError, evidence.Pause.CollectorPID, evidence.Pause.ManagerPID, evidence.Pause.Driver, evidence.Pause.ActiveLeases, evidence.Pause.ActiveWork)
	durable := child.receive(t, "dev_frontend_native_pause_persisted")
	if !durable.DurablePause || !durable.CaptureAndReasoningDrained || json.Unmarshal(durable.Frontend, &evidence) != nil || evidence.ExpectedCollectorPID != expectedCollectorPID || !validNativeFrontendPause(evidence, true) {
		t.Fatalf("native durable pause before owner lease release invalid: %+v", evidence)
	}
	t.Logf("pausePhase=persisted_before_lease_release admissionPaused=%t sqlitePaused=%t sqliteError=%q collectorPID=%d managerPID=%d driver=%s activeLeases=%d captureSteps=%d activeWork=%d beforeLeaseRelease=%t", evidence.Pause.AdmissionPaused, evidence.Pause.SQLitePaused, evidence.Pause.SQLiteError, evidence.Pause.CollectorPID, evidence.Pause.ManagerPID, evidence.Pause.Driver, evidence.Pause.ActiveLeases, evidence.Pause.CaptureSteps, evidence.Pause.ActiveWork, evidence.Pause.BeforeLeaseRelease)
	accepted := child.receive(t, "dev_frontend_native_accepted")
	if !accepted.Readback || !accepted.ExactForeground || !accepted.APICompleted || !accepted.DurableForegroundAudit || !accepted.CaptureAndReasoningDrained || !accepted.BootstrapReused || json.Unmarshal(accepted.Frontend, &evidence) != nil || !evidence.HelperExitedNormally || !evidence.DOM.NativeOpened || evidence.DOM.Rejected || evidence.DOM.RequestID == "" || evidence.DOM.RequestID != evidence.Stats.RequestID || evidence.Stats.NativeAccepted != 1 || evidence.Stats.NativeRequests != 1 || fixture.logins.Load() != 1 || fixture.sequence.Load() != 1 {
		t.Fatal("full actual frontend native acceptance incomplete")
	}
	t.Logf("firstPhysicalFrontendClick actualChromeHelperPID=%d directUIParent=true unchangedHelperIdentityJobAge=true exactReaderPID=%d HWND=%d actualAppRelayContentWorkerNative=true readback=true APICompleted=true durableAudit=true captureReasoningDrain=true bootstrapReused=true syntheticLoginCount=1 noWarmRetry=true", click.HelperPID, accepted.ReaderPID, accepted.ReaderHWND)
	if scenario == DevelopmentFrontendNativeScenarioForegroundOnly {
		complete := child.receive(t, "dev_frontend_native_foreground_complete")
		if complete.PID != ready.PID || complete.ReaderPID != accepted.ReaderPID || complete.ReaderHWND != accepted.ReaderHWND || complete.CaptureCount != 1 || !complete.BootstrapReused || !complete.ExactForeground || fixture.logins.Load() != 1 || fixture.sequence.Load() != 1 {
			t.Fatal("foreground-only completion disagreed with physical acceptance")
		}
		verifyProcessFixtureImage(t, child.identity, receipt.MainPath)
		if _, err = fmt.Fprintln(child.input, "stop"); err != nil {
			t.Fatal(err)
		}
		select {
		case exitErr := <-child.exited:
			if exitErr != nil {
				t.Fatalf("foreground graceful actual main exit failed: %v %s", exitErr, child.stderr.String())
			}
		case <-time.After(15 * time.Second):
			t.Fatal("foreground graceful actual main did not exit")
		}
		cleanupCtx, cancelCleanup := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancelCleanup()
		if err = processFixtureWait(cleanupCtx, func() bool {
			ids, queryErr := (&chromeFixtureBridge{job: child.job}).jobPIDs()
			return queryErr == nil && len(ids) == 0 && processFixtureWindowsGone([]uintptr{uintptr(ready.UIHWND), uintptr(accepted.ReaderHWND)})
		}); err != nil {
			t.Fatal("foreground owned tree/windows survived graceful exit before fallback")
		}
		if fixture.logins.Load() != 1 || fixture.sequence.Load() != 1 {
			t.Fatal("foreground graceful cleanup caused repeat login/capture")
		}
		t.Log("physicalForegroundComplete=true gracefulOwnedCleanup=true outerTreeEmptyBeforeFallback=true syntheticLoginCount=1 lifetimeClaims=false")
		return
	}
	if scenario == DevelopmentFrontendNativeScenarioReaderOpenParentDeath {
		lifetime := child.receive(t, "dev_frontend_native_live_reader_lifetime_ready")
		verifyNativeFrontendLiveReaderParentDeath(t, child, cfg, receipt, ready, accepted, lifetime, evidence, fixture)
		return
	}
	beforeClose := child.receive(t, "dev_frontend_native_windows_before_close")
	if err = json.Unmarshal(beforeClose.Frontend, &evidence); err != nil {
		t.Fatal(err)
	}
	t.Logf("readerWindowPhase=before_bootstrap_close expectedReaderPID=%d evidence=%+v failedPredicates=%v", evidence.ExpectedReaderPID, evidence.Windows, evidence.WindowFailures)
	if evidence.ExpectedReaderPID != accepted.ReaderPID || !evidence.Windows.InventoryKnown || evidence.Windows.InventoryError != "" || evidence.Windows.WindowCount != 2 || len(evidence.Windows.BootstrapFlags) != 2 || (!evidence.Windows.BootstrapFlags[0] && !evidence.Windows.BootstrapFlags[1]) || len(evidence.WindowFailures) != 0 {
		t.Fatal("native pre-close window predicate evidence rejected")
	}
	afterClose := child.receive(t, "dev_frontend_native_windows_after_close")
	if err = json.Unmarshal(afterClose.Frontend, &evidence); err != nil {
		t.Fatal(err)
	}
	t.Logf("readerWindowPhase=after_bootstrap_close expectedReaderPID=%d closedBootstrapIndex=%d closeError=%q evidence=%+v failedPredicates=%v", evidence.ExpectedReaderPID, evidence.ClosedBootstrapIndex, evidence.BootstrapCloseError, evidence.Windows, evidence.WindowFailures)
	if evidence.ExpectedReaderPID != accepted.ReaderPID || evidence.BootstrapCloseError != "" || !evidence.Windows.InventoryKnown || evidence.Windows.InventoryError != "" || evidence.Windows.WindowCount != 1 || len(evidence.Windows.BootstrapFlags) != 1 || evidence.Windows.BootstrapFlags[0] || !evidence.Windows.CoordinatorPaused || len(evidence.WindowFailures) != 0 {
		t.Fatal("native post-close window predicate evidence rejected; see phase snapshot")
	}
	resumed := child.receive(t, "dev_frontend_native_resumed")
	if !resumed.RemainingWindowHeldPause || !resumed.FinalCloseVerified || !resumed.APIHeadlessReadback || resumed.CaptureCount != 2 || json.Unmarshal(resumed.Frontend, &evidence) != nil || !evidence.DuplicateStaleCancelledDenied || fixture.sequence.Load() != 2 || fixture.logins.Load() != 1 {
		t.Fatal("native frontend multiwindow/final-close verified continuation incomplete")
	}
	t.Log("remainingWindowHeldPause=true finalCloseNaturalExit=true sourceAccountProfileVerifiedResume=true typedActualAPIHeadless=true duplicateStaleCancelledDenied=true")
	lifetime := child.receive(t, "dev_frontend_native_lifetime_ready")
	if !lifetime.DurablePause || lifetime.SessionID == "" || lifetime.RunID == "" || lifetime.UIPID != ready.UIPID || json.Unmarshal(lifetime.Frontend, &evidence) != nil || !evidence.HelperExitedNormally || !evidence.NoKeeper {
		t.Fatal("native frontend parent lifetime not staged")
	}
	var handles []windows.Handle
	for _, peer := range []struct {
		pid   uint32
		image string
	}{{lifetime.UIPID, filepath.Join(root, "chromium", "bin", "chrome.exe")}, {uint32(lifetime.ChromePID), cfg.ChromePath}} {
		identity := mustDevelopmentProcessIdentity(t, peer.pid)
		verifyProcessFixtureImage(t, identity, peer.image)
		handle, e := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, peer.pid)
		if e != nil {
			t.Fatal(e)
		}
		defer windows.CloseHandle(handle)
		member, e := chromeFixtureJobMembership(handle, child.job)
		if e != nil || !member {
			t.Fatal("native lifetime peer outside owned outer Job")
		}
		handles = append(handles, handle)
	}
	child.stop(t, true, receipt.MainPath, []uintptr{uintptr(lifetime.UIHWND)})
	for _, handle := range handles {
		if state, e := windows.WaitForSingleObject(handle, 3000); e != nil || state != windows.WAIT_OBJECT_0 {
			t.Fatal("owned UI/Chrome survived main before fallback")
		}
	}
	state, err := store.Open(filepath.Join(cfg.Root, "fixture.sqlite"), domain.DefaultSettings("expanded", "quiet", "guarded_live", true))
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	paused, err := state.FixtureCaptureSessionPaused(context.Background(), lifetime.SessionID)
	if err != nil || !paused {
		t.Fatal("native frontend durable pause lost after main death")
	}
	command, err := state.ClaimCommandForCollector(context.Background(), lifetime.RunID, "must-not-claim", "headless")
	if err != nil || command != nil {
		t.Fatal("native frontend persisted claim fence lost", err)
	}
	t.Logf("onlyActualMainTerminated=true UIChromeRootSignalled=true nativeHelperExitedNormallyBeforeParentDeath=true noKeeper=true outerTreeEmptyBeforeFallback=true UIHWNDGone=true SQLitePauseNeverClaimedFence=true retainedRegistrationArtifacts=true taskRoot=%s", root)
}

func verifyNativeFrontendLiveReaderParentDeath(t *testing.T, child *processFixtureChild, cfg chromeFixtureConfig, receipt readerbroker.FixtureNativeHostReceipt, ready, accepted, lifetime processFixtureReceipt, prior nativeFrontendTestEvidence, fixture *chromeFixtureHTTP) {
	t.Helper()
	var evidence nativeFrontendTestEvidence
	if json.Unmarshal(lifetime.Frontend, &evidence) != nil || lifetime.DurablePause != true || lifetime.CaptureCount != 1 || lifetime.SessionID != ready.SessionID || lifetime.RunID == "" || lifetime.UIPID != ready.UIPID || lifetime.UIHWND != ready.UIHWND || lifetime.ReaderPID != accepted.ReaderPID || lifetime.ReaderHWND != accepted.ReaderHWND || lifetime.Root != cfg.Root || evidence.Scenario != DevelopmentFrontendNativeScenarioReaderOpenParentDeath || !evidence.HelperExitedNormally || evidence.QueuedRun.ID != lifetime.RunID || evidence.QueuedRun.SessionID != lifetime.SessionID || evidence.QueuedRun.Ordinal != 1 || evidence.QueuedRun.Status != "waiting_for_bridge" {
		t.Fatalf("live-reader parent-death stage incomplete receipt=%+v evidence=%+v", lifetime, evidence)
	}
	if err := ValidateDevelopmentLiveReaderQueue(evidence.QueuedRun, evidence.Commands); err != nil {
		t.Fatal(err)
	}
	if evidence.Windows.InventoryKnown != true || evidence.Windows.InventoryError != "" || evidence.Windows.WindowCount < 1 || len(evidence.Windows.BootstrapFlags) != evidence.Windows.WindowCount || !hasBootstrapWindow(evidence.Windows.BootstrapFlags) || !evidence.Windows.CoordinatorPaused || !evidence.Windows.SQLiteKnown || evidence.Windows.Pause.SQLiteError != "" || !evidence.Windows.Pause.SQLitePaused || !evidence.Windows.Pause.AdmissionPaused || evidence.Windows.Pause.Driver != "browser" || evidence.Windows.Pause.CollectorPID != int(lifetime.ReaderPID) || evidence.Windows.Pause.ManagerPID != int(lifetime.ReaderPID) || evidence.Windows.Pause.Generation == 0 || evidence.Windows.Pause.CaptureSteps != 0 || evidence.Windows.Pause.ActiveWork != 0 || prior.HelperExitedNormally != true || fixture.logins.Load() != 1 || fixture.sequence.Load() != 1 {
		t.Fatalf("live-reader pause/window/login evidence invalid windows=%+v previous=%+v logins=%d sequence=%d", evidence.Windows, prior, fixture.logins.Load(), fixture.sequence.Load())
	}
	t.Logf("readerOpenParentDeathReady=true scenario=%s session=%s queuedRun=%s queuedStatus=%s queuedCommandStatus=%s UI=%d/%d reader=%d/%d windows=%+v captureCount=%d syntheticLogins=%d sourceSequence=%d", evidence.Scenario, lifetime.SessionID, lifetime.RunID, evidence.QueuedRun.Status, evidence.QueuedRun.BridgeCommandStatus, lifetime.UIPID, lifetime.UIHWND, lifetime.ReaderPID, lifetime.ReaderHWND, evidence.Windows, lifetime.CaptureCount, fixture.logins.Load(), fixture.sequence.Load())
	verifyDevelopmentLiveReaderParentDeathCore(t, child, cfg, receipt.MainPath, lifetime, evidence, fixture)
}

func verifyDevelopmentLiveReaderParentDeathCore(t *testing.T, child *processFixtureChild, cfg chromeFixtureConfig, mainPath string, lifetime processFixtureReceipt, evidence nativeFrontendTestEvidence, fixture *chromeFixtureHTTP) {
	t.Helper()

	type pinnedPeer struct {
		pid   uint32
		image string
		hwnd  uint64
	}
	peers := []pinnedPeer{
		{pid: lifetime.UIPID, image: filepath.Join(filepath.Dir(cfg.Root), "chromium", "bin", "chrome.exe"), hwnd: lifetime.UIHWND},
		{pid: lifetime.ReaderPID, image: cfg.ChromePath, hwnd: lifetime.ReaderHWND},
	}
	handles := make([]windows.Handle, 0, len(peers))
	defer func() {
		for _, handle := range handles {
			_ = windows.CloseHandle(handle)
		}
	}()
	for _, peer := range peers {
		if peer.pid == 0 || peer.hwnd == 0 {
			t.Fatal("live UI/reader identity missing")
		}
		identity := mustDevelopmentProcessIdentity(t, peer.pid)
		verifyProcessFixtureImage(t, identity, peer.image)
		handle, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, peer.pid)
		if err != nil {
			t.Fatal(err)
		}
		handles = append(handles, handle)
		var created, exited, kernel, user windows.Filetime
		if err = windows.GetProcessTimes(handle, &created, &exited, &kernel, &user); err != nil || fmt.Sprintf("%08x%08x", created.HighDateTime, created.LowDateTime) != identity.Created {
			t.Fatal("live UI/reader pinned-handle creation identity changed")
		}
		imageBuffer := make([]uint16, 32768)
		imageSize := uint32(len(imageBuffer))
		if err = windows.QueryFullProcessImageName(handle, 0, &imageBuffer[0], &imageSize); err != nil || !samePath(windows.UTF16ToString(imageBuffer[:imageSize]), peer.image) {
			t.Fatal("live UI/reader pinned-handle image changed")
		}
		member, err := chromeFixtureJobMembership(handle, child.job)
		if err != nil || !member {
			t.Fatal("live UI/reader root escaped owned outer Job")
		}
		isWindow, _, _ := windows.NewLazySystemDLL("user32.dll").NewProc("IsWindow").Call(uintptr(peer.hwnd))
		var windowPID uint32
		windows.NewLazySystemDLL("user32.dll").NewProc("GetWindowThreadProcessId").Call(uintptr(peer.hwnd), uintptr(unsafe.Pointer(&windowPID)))
		if isWindow == 0 || windowPID != peer.pid {
			t.Fatal("live UI/reader HWND did not belong to its pinned process root")
		}
	}
	// This terminates only the already-pinned actual Sidecar main. The retained
	// peer handles and HWNDs prove the reader and UI die before any fallback.
	child.stop(t, true, mainPath, []uintptr{uintptr(lifetime.UIHWND), uintptr(lifetime.ReaderHWND)})
	for _, handle := range handles {
		if state, err := windows.WaitForSingleObject(handle, 3000); err != nil || state != windows.WAIT_OBJECT_0 {
			t.Fatal("live reader/UI root survived actual main before fallback")
		}
	}
	if fixture.logins.Load() != 1 || fixture.sequence.Load() != 1 {
		t.Fatalf("reader-open parent death caused silent resume or second login: logins=%d sequence=%d", fixture.logins.Load(), fixture.sequence.Load())
	}

	state, err := store.Open(filepath.Join(cfg.Root, "fixture.sqlite"), domain.DefaultSettings("expanded", "quiet", "guarded_live", true))
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	paused, err := state.FixtureCaptureSessionPaused(context.Background(), lifetime.SessionID)
	if err != nil || !paused {
		t.Fatal("original live-reader durable pause lost after main death", err)
	}
	session, err := state.GetSession(context.Background(), lifetime.SessionID)
	if err != nil || session.ID != lifetime.SessionID {
		t.Fatal("original paused session missing after main death", err)
	}
	queuedRun, err := state.GetRun(context.Background(), lifetime.RunID)
	if err != nil || !reflect.DeepEqual(queuedRun, evidence.QueuedRun) || queuedRun.Status != "waiting_for_bridge" {
		t.Fatalf("original queued second-run metadata changed after main death: expected=%+v actual=%+v err=%v", evidence.QueuedRun, queuedRun, err)
	}
	matchedRun := false
	for _, run := range session.Runs {
		if run.ID == lifetime.RunID {
			matchedRun = reflect.DeepEqual(run, evidence.QueuedRun)
		}
	}
	if !matchedRun {
		t.Fatal("original queued run/session relationship changed after main death")
	}
	commands, err := state.FixtureRunCommandEvidence(context.Background(), lifetime.RunID)
	if err != nil || !reflect.DeepEqual(commands, evidence.Commands) {
		t.Fatal("original queued command payload/claim metadata changed after main death", err)
	}
	if err = ValidateDevelopmentLiveReaderQueue(queuedRun, commands); err != nil {
		t.Fatal(err)
	}
	command, err := state.ClaimCommandForCollector(context.Background(), lifetime.RunID, "must-not-claim", "headless")
	if err != nil || command != nil {
		t.Fatal("live-reader persisted claim fence lost after main death", err)
	}
	afterClaim, err := state.GetRun(context.Background(), lifetime.RunID)
	if err != nil || !reflect.DeepEqual(afterClaim, evidence.QueuedRun) {
		t.Fatalf("post-death blocked claim altered original queued run metadata: expected=%+v actual=%+v err=%v", evidence.QueuedRun, afterClaim, err)
	}
	afterCommands, err := state.FixtureRunCommandEvidence(context.Background(), lifetime.RunID)
	if err != nil || !reflect.DeepEqual(afterCommands, evidence.Commands) {
		t.Fatal("blocked post-death claim altered original command payload/metadata", err)
	}
	t.Logf("originalQueuedCommandID=%s payloadSHA256=%s status=%s nullClaimFields=true exactCommandRowRetained=true", commands[0].ID, commands[0].PayloadSHA256, commands[0].Status)
	t.Logf("onlyActualMainTerminated=true UIReaderHandlesPinned=true creationImagesAndJobVerified=true UIReaderHWNDsGone=true outerTreeEmptyBeforeFallback=true originalSessionAndQueuedRunRetained=true queuedClaimBlocked=true durablePause=true captureCount=1 syntheticLoginCount=1 noHeadlessFallback=true taskRoot=%s", cfg.Root)
}

func hasBootstrapWindow(flags []bool) bool {
	for _, flag := range flags {
		if flag {
			return true
		}
	}
	return false
}
