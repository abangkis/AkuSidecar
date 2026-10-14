//go:build windows && auth_session_dev && auth_session_fixture

package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/abangkis/AkuSidecar/internal/appshell"
	"github.com/abangkis/AkuSidecar/internal/readerbroker"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type frontendTestEvidence struct {
	Origin, Title, Version                                              string
	Headless, NormalWindow, NativeActivationClosed, BrokerServerStarted bool
	ExactWorkerObserved                                                 bool
	UIWindow                                                            appshell.DevelopmentFrontendWindowState
	NativeHelpersStaged, CollectorPID, AuditRejected                    int
	CollectorUnchanged, CaptureActive                                   bool
	StageProof                                                          struct {
		ContentSHA, OriginalWorkerSHA, StagedWorkerSHA, Origin, ExtensionID string
		QAHostName                                                          string
		NativePermission, NativeCalls                                       bool
	}
	DOM struct {
		DenialNotice                                                                         bool
		Rect                                                                                 struct{ Left, Top, Right, Bottom float64 }
		Title, Broker, RequestID, ClickRequestID                                             string
		Ready, LinkRendered, LinkVisible, LinkEnabled, Rejected, RelayRejected, NativeOpened bool
	}
	Stats struct {
		Bootstrap, AppAsset, RelayAsset, Runtime, NativeDenied, Cancelled int
		NativeRequests, NativeAccepted                                    int
		RequestID                                                         string
	}
}

// No UI input, JS action or native activation is used during preparation.
func TestDevelopmentFrontendActualMainHeadlessPreparation(t *testing.T) {
	if os.Getenv("AKU_DEV_FRONTEND_ENABLED") != "1" || os.Getenv("AKU_DEV_FRONTEND_HEADLESS") != "1" {
		t.Skip("requires explicit PrepareDevelopmentFrontend")
	}
	runDevelopmentFrontendActualMain(t, true)
}
func TestDevelopmentFrontendActualMainPhysicalDenial(t *testing.T) {
	if os.Getenv("AKU_DEV_FRONTEND_ENABLED") != "1" || os.Getenv("AKU_DEV_FRONTEND_HEADLESS") != "0" {
		t.Skip("requires explicit VerifyDevelopmentFrontend and operational readiness")
	}
	runDevelopmentFrontendActualMain(t, false)
}

func TestDevelopmentFrontendActualMainAutomaticReaderLifetime(t *testing.T) {
	if os.Getenv("AKU_DEV_FRONTEND_LIFETIME_ENABLED") != "1" || os.Getenv("AKU_DEV_FRONTEND_HEADLESS") != "0" {
		t.Skip("requires explicit automatic lifetime fixture")
	}
	runDevelopmentFrontendActualMain(t, false)
}
func runDevelopmentFrontendActualMain(t *testing.T, headless bool) {
	nonce := readerbroker.Secret()
	fixture := &chromeFixtureHTTP{}
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
		fixture.serveHTTP(w, r)
	}))
	defer server.Close()
	fixture.server = server
	cfg := chromeFixtureConfigFromEnv(t, server.URL)
	parent, err := processFixtureProcessIdentity(uint32(os.Getpid()))
	if err != nil {
		t.Fatal(err)
	}
	rootID, err := processFixtureDirectoryID(cfg.Root)
	if err != nil {
		t.Fatal(err)
	}
	lineage := processFixtureLineage{Root: cfg.Root, RootID: rootID, Nonce: nonce, Parent: parent}
	raw, _ := json.Marshal(lineage)
	if err = os.WriteFile(filepath.Join(cfg.Root, "process-controller-lineage.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	image, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	start := processFixtureStart{Grant: processFixtureGrant{processFixtureLineage: lineage}, NodePath: cfg.NodePath, ChromePath: cfg.ChromePath, DriverPath: cfg.DriverPath, Origin: cfg.Origin, AuthorityKey: nonce, ControllerImage: image}
	mainImage := filepath.Join(os.Getenv("AKU_CHROME_FIXTURE_ROOT"), "akusidecar.exe")
	child := startProcessFixtureChild(t, mainImage, start)
	ready := receiveDevelopmentReceipt(t, child, "dev_frontend_ready", 60*time.Second)
	var evidence frontendTestEvidence
	if err = json.Unmarshal(ready.Frontend, &evidence); err != nil {
		t.Fatal(err)
	}
	if ready.PID != child.identity.PID || ready.UIPID == 0 || ready.Root != cfg.Root || evidence.Headless != headless || !evidence.NormalWindow || !evidence.NativeActivationClosed || evidence.NativeHelpersStaged != 0 || evidence.BrokerServerStarted || evidence.Title != "AkuBrowser QA - Native activation DISABLED" || !evidence.DOM.Ready || !evidence.DOM.LinkRendered || !evidence.DOM.LinkEnabled || evidence.DOM.Broker != "ready" || evidence.Stats.Bootstrap < 1 || evidence.Stats.AppAsset < 1 || evidence.Stats.RelayAsset < 1 || evidence.StageProof.NativePermission || evidence.StageProof.NativeCalls || evidence.StageProof.ExtensionID != "dlibmmlopdahibfniinemhnghlifiple" || evidence.StageProof.ContentSHA == "" || evidence.StageProof.StagedWorkerSHA == evidence.StageProof.OriginalWorkerSHA {
		t.Fatalf("frontend real DOM/closed staging evidence invalid: %+v", evidence)
	}
	verifyProcessFixtureImage(t, mustDevelopmentProcessIdentity(t, ready.UIPID), filepath.Join(os.Getenv("AKU_CHROME_FIXTURE_ROOT"), "chromium", "bin", "chrome.exe"))
	automaticLifetime := os.Getenv("AKU_DEV_FRONTEND_LIFETIME_ENABLED") == "1"
	if !evidence.ExactWorkerObserved || (!headless && (ready.UIHWND == 0 || ready.UIHWND != evidence.UIWindow.HWND || (!automaticLifetime && (!evidence.UIWindow.Visible || evidence.UIWindow.Minimized)))) {
		t.Fatal("actual staged worker/window readback invalid")
	}
	if automaticLifetime && (evidence.UIWindow.PID != ready.UIPID || !evidence.UIWindow.JobMember || !evidence.UIWindow.TitleMatched || evidence.UIWindow.Class != "Chrome_WidgetWin_1") {
		t.Fatal("automatic owned UI window identity invalid")
	}
	if headless && (evidence.UIWindow.Visible || evidence.UIWindow.HWND != 0) {
		t.Fatal("headless frontend unexpectedly has a visible native UI")
	}
	if evidence.DOM.Rect.Right <= evidence.DOM.Rect.Left || evidence.DOM.Rect.Bottom <= evidence.DOM.Rect.Top {
		t.Fatal("rendered native link rectangle unavailable")
	}
	if p, err := readerbroker.ParentPID(ready.UIPID); err != nil || p != ready.PID {
		t.Fatal("frontend UI not owned by actual main")
	}
	helperSamples := 0
	checkPeers := func() {
		pids, err := (&chromeFixtureBridge{job: child.job}).jobPIDs()
		if err != nil {
			t.Fatal(err)
		}
		for _, pid := range pids {
			image, err := readerbroker.ProcessImage(pid)
			if err != nil {
				continue
			}
			if strings.EqualFold(filepath.Base(image), "aku-reader-broker.exe") {
				helperSamples++
				t.Fatal("native helper appeared in frontend QA Job")
			}
		}
	}
	checkPeers()
	if os.Getenv("AKU_DEV_FRONTEND_LIFETIME_ENABLED") == "1" {
		if evidence.UIWindow.PID != ready.UIPID {
			t.Fatal("automatic raw frontend HWND ownership disagreed")
		}
		lifetime := receiveDevelopmentReceipt(t, child, "dev_frontend_automatic_lifetime_ready", 60*time.Second)
		var queue nativeFrontendTestEvidence
		var finalFrontend frontendTestEvidence
		if json.Unmarshal(lifetime.Frontend, &finalFrontend) != nil || finalFrontend.DOM.ClickRequestID != "" || finalFrontend.DOM.RequestID != "" || finalFrontend.DOM.NativeOpened || finalFrontend.DOM.Rejected || finalFrontend.DOM.RelayRejected || finalFrontend.Stats.NativeRequests != 0 || finalFrontend.Stats.NativeAccepted != 0 || finalFrontend.Stats.NativeDenied != 0 || finalFrontend.Stats.Cancelled != 0 {
			t.Fatal("automatic lifetime observed frontend/native action")
		}
		var closed struct {
			NativeActivationClosed            bool
			NativeHelpersStaged, ManualClicks int
			NoKeeper                          bool
		}
		if json.Unmarshal(lifetime.Frontend, &queue) != nil || json.Unmarshal(lifetime.Frontend, &closed) != nil || !closed.NativeActivationClosed || closed.NativeHelpersStaged != 0 || closed.ManualClicks != 0 || !closed.NoKeeper || !lifetime.DurablePause || lifetime.CaptureCount != 1 || lifetime.SessionID != ready.SessionID || lifetime.UIPID != ready.UIPID || lifetime.Root != cfg.Root || lifetime.RunID != queue.QueuedRun.ID {
			t.Fatal("automatic lifetime receipt did not retain closed activation/queue identity")
		}
		if err := ValidateDevelopmentLiveReaderQueue(queue.QueuedRun, queue.Commands); err != nil {
			t.Fatal(err)
		}
		v := queue.Windows
		if !v.InventoryKnown || v.InventoryError != "" || !hasBootstrapWindow(v.BootstrapFlags) || !v.CoordinatorPaused || !v.SQLiteKnown || !v.Pause.SQLitePaused || !v.Pause.AdmissionPaused || v.Pause.SQLiteError != "" || v.Pause.CollectorPID != int(lifetime.ReaderPID) || v.Pause.ManagerPID != int(lifetime.ReaderPID) || v.Pause.Driver != "browser" || v.Pause.Generation == 0 || v.Pause.CaptureSteps != 0 || v.Pause.ActiveWork != 0 || fixture.logins.Load() != 1 || fixture.sequence.Load() != 1 {
			t.Fatalf("automatic live-reader pause/owner/bootstrap invalid: %+v", v)
		}
		checkPeers()
		verifyDevelopmentLiveReaderParentDeathCore(t, child, cfg, mainImage, lifetime, queue, fixture)
		t.Log("automaticLifetimeComplete=true nativeActivationClosed=true helpers=0 manualClicks=0 physicalForegroundClaim=false")
		return
	}
	t.Logf("actualMainPID=%d ownedUIPID=%d headless=%t exactQAOrigin=true actualAppAsset=true actualInjectedRelay=true actualStoreTimelineRendered=true trustedContentReady=true nativeHelpersStaged=0 brokerServerStarted=false nativeCallsRemoved=true independentUIProfile=true CfT=%s title=%q taskRoot=%s", ready.PID, ready.UIPID, headless, evidence.Version, evidence.Title, cfg.Root)
	if headless {
		if _, err = fmt.Fprintln(child.input, "finish_prepare"); err != nil {
			t.Fatal(err)
		}
		done := child.receive(t, "dev_frontend_prepared")
		if json.Unmarshal(done.Frontend, &evidence) != nil || !evidence.Headless || evidence.DOM.NativeOpened || evidence.Stats.NativeDenied != 0 || evidence.Stats.Cancelled != 0 {
			t.Fatal("headless preparation performed an action")
		}
	} else {
		cue := os.Getenv("AKU_DEV_FRONTEND_CUE")
		root := os.Getenv("AKU_CHROME_FIXTURE_ROOT")
		if !filepath.IsAbs(cue) || !samePath(filepath.Dir(cue), root) {
			t.Fatal("frontend operator cue outside exact QA root")
		}
		if _, err = os.Lstat(cue); !os.IsNotExist(err) {
			t.Fatal("operator cue must be fresh")
		}
		operatorNonce := readerbroker.Secret()
		armed := map[string]any{"Stage": "ARMED", "Nonce": operatorNonce, "TaskRoot": root, "MainPID": ready.PID, "UIPID": ready.UIPID, "UIHWND": ready.UIHWND, "UIWindow": evidence.UIWindow, "Title": evidence.Title, "NativeActivationClosed": true, "DOM": evidence.DOM}
		raw, _ := json.MarshalIndent(armed, "", "  ")
		if err = os.WriteFile(cue+".ready.json", raw, 0600); err != nil {
			t.Fatal(err)
		}
		t.Logf("READY/ARMED actual frontend native-disabled link rendered/enabled viewportVisible=%t cue=%s ready=%s; operator may scroll real QA page; no synthetic clicks; deadline begins only coordinator cue", evidence.DOM.LinkVisible, cue, cue+".ready.json")
		timer := time.NewTicker(25 * time.Millisecond)
		defer timer.Stop()
		admin := time.NewTimer(8 * time.Minute)
		defer admin.Stop()
		cued := false
		done := false
		for !done {
			select {
			case <-admin.C:
				t.Fatal("frontend staging administration deadline")
			case receipt, ok := <-child.replies:
				if !ok || receipt.Stage != "dev_frontend_denied" || !receipt.PhysicalOpen {
					t.Fatal("frontend physical denial missing", receipt.Stage, receipt.Failure)
				}
				if json.Unmarshal(receipt.Frontend, &evidence) != nil || !evidence.DOM.Rejected || !evidence.DOM.RelayRejected || !evidence.DOM.LinkVisible || !evidence.DOM.DenialNotice || evidence.DOM.NativeOpened || evidence.DOM.RequestID != evidence.Stats.RequestID || evidence.Stats.NativeDenied != 1 || evidence.Stats.Cancelled < 1 || evidence.AuditRejected != 1 || !evidence.CollectorUnchanged || !evidence.CaptureActive {
					t.Fatal("physical UI/real relay denial/cancel/readback failed")
				}
				done = true
			case <-timer.C:
				checkPeers()
				if !cued {
					if raw, err := os.ReadFile(cue); err == nil {
						var instruction struct {
							Nonce   string
							MainPID uint32
						}
						if json.Unmarshal(raw, &instruction) != nil || instruction.Nonce != operatorNonce || instruction.MainPID != ready.PID {
							t.Fatal("operator cue mismatch")
						}
						if _, err = fmt.Fprintln(child.input, "operator_cue"); err != nil {
							t.Fatal(err)
						}
						cued = true
					}
				}
			}
		}
	}
	select {
	case err := <-child.exited:
		if err != nil {
			t.Fatalf("frontend main exit failed: %v %s", err, child.stderr.String())
		}
	case <-time.After(15 * time.Second):
		t.Fatal("frontend actual main cleanup timed out")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err = processFixtureWait(ctx, func() bool {
		pids, e := (&chromeFixtureBridge{job: child.job}).jobPIDs()
		return e == nil && len(pids) == 0
	}); err != nil {
		t.Fatal("frontend owned UI/process tree survived before fallback")
	}
	if fixture.logins.Load() != 1 {
		t.Fatal("frontend repeated synthetic login")
	}
	t.Logf("frontendAcceptanceHeadless=%t nativeHelperSamplesSeen=%d loginCount=1 actualMainExit=0 outerOwnedTreeEmptyBeforeFallback=true nativeActivationProof=false", headless, helperSamples)
}
