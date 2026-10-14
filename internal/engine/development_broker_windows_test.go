//go:build windows && auth_session_dev && auth_session_fixture

package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/readerbroker"
	"github.com/abangkis/AkuSidecar/internal/store"
	"golang.org/x/sys/windows"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDevelopmentSessionActualMainBrokerFirstPhysicalOpen(t *testing.T) {
	if os.Getenv("AKU_DEV_BROKER_ENABLED") != "1" {
		t.Skip("requires explicit VerifyDevelopmentBroker and operational readiness")
	}
	cue := os.Getenv("AKU_DEV_BROKER_CUE")
	root := os.Getenv("AKU_CHROME_FIXTURE_ROOT")
	if cue == "" || !filepath.IsAbs(cue) || !samePath(filepath.Dir(cue), root) {
		t.Fatal("operator cue must be in exact unique QA task root")
	}
	if _, err := os.Lstat(cue); !os.IsNotExist(err) {
		t.Fatal("operator cue must be fresh")
	}
	nonce := readerbroker.Secret()
	operatorNonce := readerbroker.Secret()
	fixture := &chromeFixtureHTTP{}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/process-authority" {
			if r.Header.Get("X-Fixture-Key") != nonce {
				http.Error(w, "denied", 403)
				return
			}
			v, _ := fixture.AuthorityEvidence(r.Context())
			json.NewEncoder(w).Encode(v)
			return
		}
		if r.URL.Path == "/native-post" {
			cookie, err := r.Cookie("fixture_session")
			if err != nil {
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
	server.Listener = listener
	server.Start()
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
	if err := os.WriteFile(filepath.Join(cfg.Root, "process-controller-lineage.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	image, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	start := processFixtureStart{Grant: processFixtureGrant{processFixtureLineage: lineage}, NodePath: cfg.NodePath, ChromePath: cfg.ChromePath, DriverPath: cfg.DriverPath, Origin: cfg.Origin, AuthorityKey: nonce, ControllerImage: image}
	mainImage := filepath.Join(root, "akusidecar.exe")
	child := startProcessFixtureChild(t, mainImage, start)
	ready := child.receive(t, "dev_broker_ready")
	if ready.PID != child.identity.PID || ready.UIPID == 0 || ready.UIHWND == 0 || !ready.ButtonVisible || !ready.ButtonEnabled || !ready.PresentedWithoutFocus || !ready.CaptureActive || ready.CollectorPID == 0 || ready.Root != cfg.Root || !brokerUIRect(ready.ButtonRect).inside(brokerUIRect(ready.UIRect)) {
		t.Fatal("actual main/UI/capture ARM evidence invalid")
	}
	if uiParent, err := readerbroker.ParentPID(ready.UIPID); err != nil || uiParent != child.identity.PID {
		t.Fatal("actual main does not own UI")
	}
	verifyProcessFixtureImage(t, mustDevelopmentProcessIdentity(t, ready.UIPID), filepath.Join(root, "aku-fixture-ui.exe"))
	armed := struct {
		Stage, Nonce, TaskRoot, CuePath                                    string
		MainPID, UIPID                                                     uint32
		UIHWND                                                             uint64
		ButtonVisible, ButtonEnabled, CaptureActive, PresentedWithoutFocus bool
		ButtonRect, UIRect                                                 DevelopmentFixtureRect
	}{"ARMED", operatorNonce, root, cue, ready.PID, ready.UIPID, ready.UIHWND, true, true, true, true, ready.ButtonRect, ready.UIRect}
	raw, _ = json.MarshalIndent(armed, "", "  ")
	if err := os.WriteFile(cue+".ready.json", raw, 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("READY/ARMED actualMainPID=%d ownedUIPID=%d HWND=%d buttonVisible=true buttonEnabled=true captureActive=true cue=%s ready=%s; operator deadline starts only coordinator cue; early physical Open is accepted immediately", ready.PID, ready.UIPID, ready.UIHWND, cue, cue+".ready.json")
	adminCtx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	tick := time.NewTicker(25 * time.Millisecond)
	defer tick.Stop()
	clicked := false
	cued := false
	for !clicked && !cued {
		select {
		case reply, ok := <-child.replies:
			if !ok || reply.Stage != "dev_broker_click" || !reply.PhysicalOpen {
				t.Fatal("early physical click evidence invalid", reply.Stage)
			}
			clicked = true
		case <-tick.C:
			if raw, err := os.ReadFile(cue); err == nil {
				var instruction struct {
					Nonce   string
					MainPID uint32
				}
				if json.Unmarshal(raw, &instruction) != nil || instruction.Nonce != operatorNonce || instruction.MainPID != ready.PID {
					t.Fatal("operator cue identity mismatch")
				}
				if _, err := fmt.Fprintln(child.input, "operator_cue"); err != nil {
					t.Fatal(err)
				}
				cued = true
				t.Log("Coordinator cue received; physical click deadline=120 seconds, native clocks unchanged")
			}
		case <-adminCtx.Done():
			t.Fatal("coordinator readiness cue missing; no physical acceptance claimed")
		}
	}
	if !clicked {
		reply := receiveDevelopmentReceipt(t, child, "dev_broker_click", 125*time.Second)
		if !reply.PhysicalOpen || reply.UIPID != ready.UIPID || reply.UIHWND != ready.UIHWND {
			t.Fatal("physical UI Open evidence invalid")
		}
	}
	accepted := child.receive(t, "dev_broker_accepted")
	if !accepted.Readback || !accepted.ExactForeground || !accepted.APICompleted || !accepted.DurableForegroundAudit || !accepted.CaptureAndReasoningDrained || !accepted.BootstrapReused || fixture.logins.Load() != 1 || fixture.sequence.Load() != 1 {
		t.Fatal("first physical actual-main/server/native acceptance incomplete")
	}
	t.Logf("firstPhysicalOpen exactReaderPID=%d HWND=%d readback=true actualServerAPICompleted=true durableForegroundAudit=true captureAndReasoningDrain=true bootstrapReused=true loginCount=1 noWarmRetry=true", accepted.ReaderPID, accepted.ReaderHWND)
	resumed := child.receive(t, "dev_broker_resumed")
	if !resumed.RemainingWindowHeldPause || !resumed.FinalCloseVerified || !resumed.APIHeadlessReadback || resumed.CaptureCount != 2 || fixture.sequence.Load() != 2 || fixture.logins.Load() != 1 {
		t.Fatal("multiwindow/final-close actual API resume incomplete")
	}
	t.Log("independentWindowHeldPause=true actualBootstrapClosed=true finalWindowNaturalExit=true sameAccountSameProfileContinuation=true actualAPIHeadlessReadback=true changedForegroundDuplicateStaleEpochCancelledAdmissionDenied=true")
	lifetime := child.receive(t, "dev_broker_lifetime_ready")
	if !lifetime.HelperStartupBlocked || lifetime.NativeIntent || lifetime.HelperPID == 0 || !lifetime.DurablePause || lifetime.SessionID == "" || lifetime.RunID == "" {
		t.Fatal("UI/helper lifetime boundary not staged")
	}
	peers := []struct {
		pid   uint32
		image string
	}{{lifetime.UIPID, filepath.Join(root, "aku-fixture-ui.exe")}, {lifetime.HelperPID, filepath.Join(root, "aku-reader-broker.exe")}, {uint32(lifetime.ChromePID), cfg.ChromePath}}
	handles := make([]windows.Handle, 0, len(peers))
	for _, peer := range peers {
		identity := mustDevelopmentProcessIdentity(t, peer.pid)
		resolved, err := filepath.EvalSymlinks(peer.image)
		if err != nil {
			t.Fatal(err)
		}
		verifyProcessFixtureImage(t, identity, resolved)
		h, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, peer.pid)
		if err != nil {
			t.Fatal(err)
		}
		defer windows.CloseHandle(h)
		handles = append(handles, h)
		member, err := chromeFixtureJobMembership(h, child.job)
		if err != nil || !member {
			t.Fatal("owned lifetime peer outside outer test Job")
		}
	}
	if parent, err := readerbroker.ParentPID(lifetime.HelperPID); err != nil || parent != lifetime.UIPID {
		t.Fatal("staged helper not a direct UI child")
	}
	child.stop(t, true, mainImage, []uintptr{uintptr(lifetime.UIHWND)})
	for _, h := range handles {
		if state, err := windows.WaitForSingleObject(h, 3000); err != nil || state != windows.WAIT_OBJECT_0 {
			t.Fatal("pinned UI/helper/Chrome root survived actual main before fallback")
		}
	}
	state, err := store.Open(filepath.Join(cfg.Root, "fixture.sqlite"), domain.DefaultSettings("expanded", "quiet", "guarded_live", true))
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	paused, err := state.FixtureCaptureSessionPaused(context.Background(), lifetime.SessionID)
	if err != nil || !paused {
		t.Fatal("actual main death lost durable queue pause")
	}
	command, err := state.ClaimCommandForCollector(context.Background(), lifetime.RunID, "must-not-claim", "headless")
	if err != nil || command != nil {
		t.Fatal("actual main death lost SQLite claim fence", err)
	}
	t.Logf("onlyActualMainTerminated=true ownedUIRootSignalled=true unchangedHelperStartupRootSignalled=true helperNativeIntent=false ChromeRootSignalled=true outerTreeEmptyBeforeFallback=true UIHWNDGone=true durableSQLitePauseAndNeverClaimedFence=true queuedSession=%s", lifetime.SessionID)
}

func mustDevelopmentProcessIdentity(t *testing.T, pid uint32) processFixtureIdentity {
	t.Helper()
	identity, err := processFixtureProcessIdentity(pid)
	if err != nil {
		t.Fatal(err)
	}
	return identity
}
func receiveDevelopmentReceipt(t *testing.T, child *processFixtureChild, stage string, timeout time.Duration) processFixtureReceipt {
	t.Helper()
	select {
	case reply, ok := <-child.replies:
		if !ok || reply.Stage != stage {
			t.Fatalf("development receipt missing expected=%q received=%q failure=%q", stage, reply.Stage, reply.Failure)
		}
		return reply
	case <-time.After(timeout):
		t.Fatal("development receipt timeout", stage)
	}
	return processFixtureReceipt{}
}
