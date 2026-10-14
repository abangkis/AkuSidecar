//go:build windows && auth_session_fixture

package engine

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/readerbroker"
	"golang.org/x/sys/windows"
)

var processFixtureCount atomic.Int64
var processFixtureCleanupFailed atomic.Bool

type processFixtureChild struct {
	cmd         *exec.Cmd
	input       io.WriteCloser
	replies     chan processFixtureReceipt
	exited      chan error
	stderr      strings.Builder
	diagnostics strings.Builder
	job         windows.Handle
	identity    processFixtureIdentity
}

func startProcessFixtureChild(t *testing.T, executable string, start processFixtureStart) *processFixtureChild {
	t.Helper()
	job, err := newChromeFixtureJob()
	if err != nil {
		t.Fatal(err)
	}
	child := &processFixtureChild{job: job, replies: make(chan processFixtureReceipt, 8), exited: make(chan error, 1)}
	arguments := []string{}
	if strings.EqualFold(filepath.Base(executable), "aku-sidecar-dev.exe") || strings.EqualFold(filepath.Base(executable), "akusidecar.exe") {
		arguments = []string{"--dev", "--dev-session-reuse=synthetic-fixture"}
		if os.Getenv("AKU_DEV_BROKER_ENABLED") == "1" {
			arguments = []string{"--dev", "--dev-session-reuse=synthetic-broker-fixture"}
		}
		if os.Getenv("AKU_DEV_FRONTEND_ENABLED") == "1" {
			arguments = []string{"--dev", "--dev-session-reuse=synthetic-frontend-fixture"}
		}
		if os.Getenv("AKU_DEV_FRONTEND_NATIVE_ENABLED") == "1" {
			arguments = []string{"--dev", "--dev-session-reuse=synthetic-frontend-native-fixture"}
		}
	}
	child.cmd = exec.Command(executable, arguments...)
	child.cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	child.cmd.Stderr = &child.stderr
	stdout, err := child.cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	child.input, err = child.cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = child.cmd.Start(); err != nil {
		windows.CloseHandle(job)
		t.Fatal(err)
	}
	processFixtureCount.Add(1)
	t.Cleanup(func() {
		_ = child.input.Close()
		b := &chromeFixtureBridge{job: child.job}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		pids, pidErr := b.jobPIDs()
		if pidErr == nil && len(pids) > 0 {
			_ = windows.TerminateJobObject(child.job, 1)
		}
		verified := processFixtureWait(ctx, func() bool { ids, queryErr := b.jobPIDs(); return queryErr == nil && len(ids) == 0 }) == nil
		if !verified {
			processFixtureCleanupFailed.Store(true)
			t.Error("process fixture containment Job did not prove empty")
			return
		}
		_ = windows.CloseHandle(child.job)
	})
	go func() {
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 4096), 64*1024)
		for scanner.Scan() {
			var reply processFixtureReceipt
			if json.Unmarshal(scanner.Bytes(), &reply) == nil {
				child.replies <- reply
			} else if child.diagnostics.Len() < 4096 {
				line := scanner.Text()
				remaining := 4096 - child.diagnostics.Len()
				if len(line) > remaining {
					line = line[:remaining]
				}
				child.diagnostics.WriteString(line)
				child.diagnostics.WriteByte('\n')
			}
		}
		close(child.replies)
	}()
	go func() { child.exited <- child.cmd.Wait() }()
	gate := child.receive(t, "gated")
	if gate.PID != uint32(child.cmd.Process.Pid) {
		t.Fatal("worker gate OS PID mismatch")
	}
	child.identity, err = processFixtureProcessIdentity(gate.PID)
	if err != nil {
		t.Fatal(err)
	}
	verifyProcessFixtureImage(t, child.identity, executable)
	if err = assignChromeFixtureProcess(job, gate.PID); err != nil {
		t.Fatal(err)
	}
	pids, err := (&chromeFixtureBridge{job: job}).jobPIDs()
	if err != nil || len(pids) != 1 || pids[0] != gate.PID {
		t.Fatal("startup gate did not contain only worker")
	}
	if err = json.NewEncoder(child.input).Encode(start); err != nil {
		t.Fatal(err)
	}
	return child
}
func (c *processFixtureChild) receive(t *testing.T, stage string) processFixtureReceipt {
	t.Helper()
	select {
	case reply, ok := <-c.replies:
		if !ok {
			select {
			case exitErr := <-c.exited:
				t.Fatalf("worker receipt %q missing: exited=%v stderr=%s diagnostics=%s", stage, exitErr, c.stderr.String(), c.diagnostics.String())
			case <-time.After(2 * time.Second):
				t.Fatalf("worker receipt %q missing: stdout closed before exit settled", stage)
			}
		}
		if !ok || reply.Stage != stage {
			t.Fatalf("worker receipt %q missing (received %q failure=%q)", stage, reply.Stage, reply.Failure)
		}
		return reply
	case <-time.After(35 * time.Second):
		t.Fatal("worker receipt timeout: " + stage)
	}
	return processFixtureReceipt{}
}
func verifyProcessFixtureImage(t *testing.T, identity processFixtureIdentity, path string) {
	t.Helper()
	actual, err := processFixtureProcessIdentity(identity.PID)
	if err != nil || actual != identity {
		t.Fatal("owned fixture PID creation identity changed")
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, identity.PID)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(h)
	buffer := make([]uint16, 32768)
	size := uint32(len(buffer))
	if err = windows.QueryFullProcessImageName(h, 0, &buffer[0], &size); err != nil || !samePath(windows.UTF16ToString(buffer[:size]), path) {
		t.Fatal("owned fixture image path changed")
	}
}
func (c *processFixtureChild) stop(t *testing.T, abrupt bool, executable string, handles []uintptr) {
	t.Helper()
	if abrupt {
		verifyProcessFixtureImage(t, c.identity, executable)
		h, err := windows.OpenProcess(windows.PROCESS_TERMINATE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, c.identity.PID)
		if err != nil {
			t.Fatal(err)
		}
		err = windows.TerminateProcess(h, 73)
		windows.CloseHandle(h)
		if err != nil {
			t.Fatal(err)
		}
	} else {
		if _, err := fmt.Fprintln(c.input, "stop"); err != nil {
			t.Fatal(err)
		}
		reply := c.receive(t, "stopped")
		if !reply.CleanupVerified {
			t.Fatal("graceful worker did not prove inner Job empty")
		}
	}
	select {
	case exitErr := <-c.exited:
		if !abrupt && exitErr != nil {
			t.Fatalf("graceful worker exit: %v %s", exitErr, c.stderr.String())
		}
		if abrupt && exitErr == nil {
			t.Fatal("abrupt fixture unexpectedly exited normally")
		}
	case <-time.After(15 * time.Second):
		t.Fatal("owned worker did not exit")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := processFixtureWait(ctx, func() bool {
		ids, queryErr := (&chromeFixtureBridge{job: c.job}).jobPIDs()
		return queryErr == nil && len(ids) == 0 && processFixtureWindowsGone(handles)
	}); err != nil {
		t.Fatal("whole worker tree/native windows did not release before successor")
	}
}

func TestProcessFixtureFullOSRestart(t *testing.T) {
	runProcessFixtureFullOSRestart(t, "auth-session-process-fixture.exe")
}

func runProcessFixtureFullOSRestart(t *testing.T, image string) {
	if os.Getenv("AKU_PROCESS_FIXTURE_ENABLED") != "1" {
		t.Skip("requires explicit VerifyProcessRestart helper")
	}
	taskRoot := os.Getenv("AKU_CHROME_FIXTURE_ROOT")
	executable := filepath.Join(taskRoot, image)
	if _, err := os.Stat(executable); err != nil {
		t.Fatal("dedicated fixture worker not built")
	}
	for _, mode := range []string{"graceful", "abrupt"} {
		t.Run(mode, func(t *testing.T) {
			var nonceBytes [32]byte
			if _, err := rand.Read(nonceBytes[:]); err != nil {
				t.Fatal(err)
			}
			nonce := hex.EncodeToString(nonceBytes[:])
			fixture := &chromeFixtureHTTP{}
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/process-authority" {
					if r.Header.Get("X-Fixture-Key") != nonce {
						http.Error(w, "denied", http.StatusForbidden)
						return
					}
					evidence, _ := fixture.AuthorityEvidence(r.Context())
					_ = json.NewEncoder(w).Encode(evidence)
					return
				}
				fixture.serveHTTP(w, r)
			}))
			server.Listener = listener
			server.Start()
			defer server.Close()
			fixture.server = server
			config := chromeFixtureConfigFromEnv(t, server.URL)
			parent, err := processFixtureProcessIdentity(uint32(os.Getpid()))
			if err != nil {
				t.Fatal(err)
			}
			rootID, err := processFixtureDirectoryID(config.Root)
			if err != nil {
				t.Fatal(err)
			}
			lineage := processFixtureLineage{Root: config.Root, RootID: rootID, Nonce: nonce, Parent: parent}
			raw, _ := json.Marshal(lineage)
			if err = os.WriteFile(filepath.Join(config.Root, "process-controller-lineage.json"), raw, 0600); err != nil {
				t.Fatal(err)
			}
			controllerImage, imageErr := os.Executable()
			if imageErr != nil {
				t.Fatal(imageErr)
			}
			start := processFixtureStart{Grant: processFixtureGrant{processFixtureLineage: lineage}, NodePath: config.NodePath, ChromePath: config.ChromePath, DriverPath: config.DriverPath, Origin: config.Origin, AuthorityKey: nonce, ControllerImage: controllerImage}
			first := startProcessFixtureChild(t, executable, start)
			paused := first.receive(t, "paused_reader")
			if paused.DevelopmentSession != (image == "aku-sidecar-dev.exe") {
				t.Fatal("worker used the wrong integration binder")
			}
			if image == "aku-sidecar-dev.exe" && (!paused.CheckpointFencedBeforeOwner || !paused.CheckpointAuthorizedAtStartup) {
				t.Fatal("development checkpoint did not fence before owner startup and authorize from verified startup")
			}
			if paused.Epoch == "" || paused.SessionID == "" || len(paused.RunIDs) != 2 || len(paused.Windows) != 2 || paused.Bootstrap != 1 || paused.Account != "alpha" || fixture.logins.Load() != 1 || fixture.sequence.Load() != 1 {
				t.Fatal("first worker actual persisted/capture evidence invalid")
			}
			pids, err := (&chromeFixtureBridge{job: first.job}).jobPIDs()
			if err != nil {
				t.Fatal(err)
			}
			workerMember, browserMember := false, false
			for _, pid := range pids {
				workerMember = workerMember || pid == first.identity.PID
				browserMember = browserMember || pid == uint32(paused.ChromePID)
			}
			if !workerMember || !browserMember {
				t.Fatal("outer containment did not include worker and actual Chrome")
			}
			for _, hwnd := range paused.Windows {
				if processFixtureWindowPID(hwnd) != uint32(paused.ChromePID) {
					t.Fatal("reader HWND was not owned Chrome")
				}
			}
			chromeIdentity, identityErr := processFixtureProcessIdentity(uint32(paused.ChromePID))
			if identityErr != nil {
				t.Fatal("old Chrome root identity unavailable before stop")
			}
			chromeImage, resolveErr := filepath.EvalSymlinks(config.ChromePath)
			if resolveErr != nil {
				t.Fatal("configured Chrome image unavailable")
			}
			verifyProcessFixtureImage(t, chromeIdentity, chromeImage)
			chromeRoot, handleErr := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, uint32(paused.ChromePID))
			if handleErr != nil {
				t.Fatal("old Chrome root handle unavailable before stop")
			}
			defer windows.CloseHandle(chromeRoot)
			t.Logf("before_stop mode=%s workerPID=%d chromePID=%d windows=%v bootstrap=%d epoch=%s loginCount=%d collected=%d", mode, first.identity.PID, paused.ChromePID, paused.Windows, paused.Bootstrap, paused.Epoch, fixture.logins.Load(), fixture.sequence.Load())
			first.stop(t, mode == "abrupt", executable, paused.Windows)
			// Empty Job accounting may precede process-handle signalling. Keep the
			// actual pre-stop root handle and require both before profile reopen.
			if status, waitErr := windows.WaitForSingleObject(chromeRoot, 3000); waitErr != nil || status != windows.WAIT_OBJECT_0 {
				t.Fatal("old Chrome root not signalled before recovery grant")
			}
			t.Logf("after_stop mode=%s oldRootExited=true oldChromeRootSignalled=true oldWholeTreeEmpty=true oldNativeWindowsGone=true readerPreserved=false innerJobDuplicated=false", mode)
			// Issue exactly one private IPC grant only after all release proofs above.
			start.Grant.Reopen = true
			start.Grant.Old = first.identity
			start.Grant.ProfileID = paused.ProfileID
			start.Grant.Windows = paused.Windows
			start.Grant.TreeReleased = true
			start.SessionID = paused.SessionID
			start.RunIDs = paused.RunIDs
			start.OldEpoch = paused.Epoch
			fixture.authority.Lock()
			savedAuthority := fixture.active
			fixture.active = SyntheticSessionEvidence{}
			fixture.authority.Unlock()
			second := startProcessFixtureChild(t, executable, start)
			denied := second.receive(t, "authority_denied")
			if !denied.AuthorityDenied || fixture.sequence.Load() != 1 || fixture.logins.Load() != 1 {
				t.Fatal("lost authority admitted restart continuation")
			}
			if image == "aku-sidecar-dev.exe" && (!denied.CheckpointFencedBeforeOwner || denied.CheckpointAuthorizedAtStartup) {
				t.Fatal("development checkpoint did not remain closed after lost authority")
			}
			fixture.authority.Lock()
			fixture.active = savedAuthority
			fixture.authority.Unlock()
			if _, err = fmt.Fprintln(second.input, "authorize"); err != nil {
				t.Fatal(err)
			}
			recovered := second.receive(t, "recovered")
			if recovered.DevelopmentSession != (image == "aku-sidecar-dev.exe") {
				t.Fatal("recovery used the wrong integration binder")
			}
			if image == "aku-sidecar-dev.exe" && (!recovered.CheckpointFencedBeforeOwner || !recovered.CheckpointAuthorizedAtStartup) {
				t.Fatal("development checkpoint did not close before reopen owner and authorize after fresh proof")
			}
			if second.identity.PID == first.identity.PID || recovered.Epoch == paused.Epoch || !recovered.StaleEpochDenied || !recovered.DuplicateDenied || recovered.Account != paused.Account || recovered.Revision != paused.Revision || fixture.logins.Load() != 1 || fixture.sequence.Load() != 2 {
				t.Fatal("new process restart recovery contract failed")
			}
			if recovered.Generation != 1 {
				t.Fatal("fresh Manager did not begin its own generation")
			}
			if !recovered.AuthorityDenied || !recovered.GrantReplayDenied || recovered.GrantNegatives != 7 {
				t.Fatal("recovery permission/auth negatives missing")
			}
			second.stop(t, false, executable, nil)
			t.Logf("recovered mode=%s workerPID=%d distinctPID=true freshEpoch=%s generation=%d sameProfile=true sameSQLiteIDs=true account=%s sameRevision=true loginCount=%d collected=%d staleEpochOnlyDenied=%t duplicateCurrentDenied=%t pauseClearedByVerifiedHooks=true allOwnedTreesEmpty=true", mode, second.identity.PID, recovered.Epoch, recovered.Generation, recovered.Account, fixture.logins.Load(), fixture.sequence.Load(), recovered.StaleEpochDenied, recovered.DuplicateDenied)
			t.Logf("negatives mode=%s lostAuthorityDenied=true durablePauseRetained=true noCollectionUntilFreshAuthority=true recoveryGrantInvalidFields=%d recoveryGrantReplayDenied=true", mode, recovered.GrantNegatives)
			if image == "aku-sidecar-dev.exe" {
				t.Logf("checkpoint mode=%s startupFencedBeforeOwner=%t startupAuthorized=%t reopenFencedBeforeOwner=%t lostAuthorityKeptClosed=%t freshProofAuthorized=%t", mode, paused.CheckpointFencedBeforeOwner, paused.CheckpointAuthorizedAtStartup, recovered.CheckpointFencedBeforeOwner, !denied.CheckpointAuthorizedAtStartup, recovered.CheckpointAuthorizedAtStartup)
			}
		})
	}
}

func TestProcessFixtureStartupBeforeNodeJob(t *testing.T) {
	runProcessFixtureStartup(t, "node_ready_before_job")
}
func TestProcessFixtureStartupBeforeReady(t *testing.T) {
	runProcessFixtureStartup(t, "node_before_ready")
}
func TestProcessFixtureHandshakeTimeout(t *testing.T) {
	runProcessFixtureStartup(t, "node_stalled_timeout")
}
func runProcessFixtureStartup(t *testing.T, mode string) {
	if os.Getenv("AKU_PROCESS_FIXTURE_ENABLED") != "1" {
		t.Skip("requires explicit VerifyProcessRestart helper")
	}
	fixture := startChromeFixtureHTTP(t)
	defer fixture.server.Close()
	config := chromeFixtureConfigFromEnv(t, fixture.server.URL)
	executable := filepath.Join(os.Getenv("AKU_CHROME_FIXTURE_ROOT"), "auth-session-process-fixture.exe")
	var nonceBytes [32]byte
	if _, err := rand.Read(nonceBytes[:]); err != nil {
		t.Fatal(err)
	}
	parent, err := processFixtureProcessIdentity(uint32(os.Getpid()))
	if err != nil {
		t.Fatal(err)
	}
	rootID, err := processFixtureDirectoryID(config.Root)
	if err != nil {
		t.Fatal(err)
	}
	lineage := processFixtureLineage{Root: config.Root, RootID: rootID, Nonce: hex.EncodeToString(nonceBytes[:]), Parent: parent}
	raw, _ := json.Marshal(lineage)
	if err = os.WriteFile(filepath.Join(config.Root, "process-controller-lineage.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	start := processFixtureStart{Grant: processFixtureGrant{processFixtureLineage: lineage}, NodePath: config.NodePath, ChromePath: config.ChromePath, DriverPath: config.DriverPath, Origin: config.Origin, AuthorityKey: lineage.Nonce, StartupBarrier: mode}
	child := startProcessFixtureChild(t, executable, start)
	stage := "node_ready_unassigned"
	if mode != "node_ready_before_job" {
		stage = "node_stalled_before_ready"
	}
	barrier := child.receive(t, stage)
	if barrier.PID != child.identity.PID || barrier.NodeIdentity.PID == 0 || barrier.NodeIdentity.Created == "" || barrier.NodeParent != child.identity.PID || !barrier.NodeMembershipKnown || barrier.NodeInnerJobMember || barrier.InnerJobProcesses != 0 {
		t.Fatal("actual pre-assignment barrier proof invalid")
	}
	if barrier.NodeReady != (mode == "node_ready_before_job") {
		t.Fatal("stalled marker confused with successful ready")
	}
	actualNode, err := processFixtureProcessIdentity(barrier.NodeIdentity.PID)
	if err != nil || actualNode != barrier.NodeIdentity {
		t.Fatal("Node creation identity changed")
	}
	nodeImage, err := filepath.EvalSymlinks(config.NodePath)
	if err != nil {
		t.Fatal(err)
	}
	verifyProcessFixtureImage(t, actualNode, nodeImage)
	nodeParent, parentErr := readerbroker.ParentPID(actualNode.PID)
	if parentErr != nil || nodeParent != child.identity.PID {
		t.Fatal("Node was not direct child of Go fixture")
	}
	goHandle, err := windows.OpenProcess(windows.PROCESS_TERMINATE|windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, child.identity.PID)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(goHandle)
	nodeHandle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, actualNode.PID)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(nodeHandle)
	outerMember, membershipErr := chromeFixtureJobMembership(nodeHandle, child.job)
	if membershipErr != nil || !outerMember {
		t.Fatal("Node outside actual outer containment")
	}
	assertNoStartupWork := func() {
		t.Helper()
		for _, name := range []string{"shared", "fixture.sqlite", "process-profile-lineage.json", "process-recovery-consumed.json"} {
			if _, statErr := os.Lstat(filepath.Join(config.Root, name)); !os.IsNotExist(statErr) {
				t.Fatalf("startup created unexpected profile/state/grant: %s", name)
			}
		}
		if fixture.logins.Load() != 0 || fixture.sequence.Load() != 0 || len(processFixtureWindows(actualNode.PID)) != 0 || len(processFixtureWindows(child.identity.PID)) != 0 {
			t.Fatal("startup unexpectedly opened windows/login/capture")
		}
	}
	assertNoStartupWork()
	pids, err := (&chromeFixtureBridge{job: child.job}).jobPIDs()
	if err != nil || len(pids) != 2 {
		t.Fatalf("pre-assignment outer tree must contain exactly Go+Node: %v %v", pids, err)
	}
	goFound, nodeFound := false, false
	for _, pid := range pids {
		goFound = goFound || pid == child.identity.PID
		nodeFound = nodeFound || pid == actualNode.PID
	}
	if !goFound || !nodeFound {
		t.Fatal("unexpected pre-assignment outer Job peers")
	}
	t.Logf("startup_barrier mode=%s readySeen=%t goPID=%d nodePID=%d nodeCreated=%s nodeParent=%d nodeImageVerified=true innerJobMember=false innerJobProcessCount=0 outerJobMember=true outerTreeExactlyGoNode=true chromeLaunches=0 windows=0 logins=0 captures=0", mode, barrier.NodeReady, child.identity.PID, actualNode.PID, actualNode.Created, nodeParent)
	verifyProcessFixtureImage(t, child.identity, executable)
	if mode == "node_stalled_timeout" {
		timeout := child.receive(t, "handshake_timeout")
		if timeout.Failure != "fixture bridge handshake timed out" || timeout.NodeReady || timeout.HandshakeElapsedMS < 8000 || timeout.HandshakeElapsedMS > 14000 {
			t.Fatalf("original handshake timeout not observed: failure=%q elapsedMS=%d ready=%t", timeout.Failure, timeout.HandshakeElapsedMS, timeout.NodeReady)
		}
		if timeout.NodeWaitAtReturn != windows.WAIT_OBJECT_0 {
			t.Fatalf("handshake timeout returned before pinned Node signal: nodeWaitAtReturn=%d elapsedMS=%d", timeout.NodeWaitAtReturn, timeout.HandshakeElapsedMS)
		}
		if status, waitErr := windows.WaitForSingleObject(goHandle, 0); waitErr != nil || status != uint32(windows.WAIT_TIMEOUT) {
			t.Fatal("Go did not stay alive while reporting timeout cleanup")
		}
		if status, waitErr := windows.WaitForSingleObject(nodeHandle, 0); waitErr != nil || status != windows.WAIT_OBJECT_0 {
			t.Fatal("Node cleanup not complete while Go still alive")
		}
		ids, queryErr := (&chromeFixtureBridge{job: child.job}).jobPIDs()
		if queryErr != nil || len(ids) != 1 || ids[0] != child.identity.PID {
			t.Fatalf("timeout must leave exactly live Go before exit gate: %v %v", ids, queryErr)
		}
		assertNoStartupWork()
		t.Logf("handshake_timeout elapsedMS=%d originalTimeoutMS=8000 failure=handshake_timed_out readySeen=false goStillAlive=true pinnedNodeSignalledAtReturn=true outerOnlyGo=true noWork=true fallbackCleanupNotUsed=true", timeout.HandshakeElapsedMS)
		if _, err = fmt.Fprintln(child.input, "exit_after_timeout"); err != nil {
			t.Fatal(err)
		}
	} else if err = windows.TerminateProcess(goHandle, 75); err != nil {
		t.Fatal(err)
	}
	if status, waitErr := windows.WaitForSingleObject(goHandle, 3000); waitErr != nil || status != windows.WAIT_OBJECT_0 {
		t.Fatal("pinned Go root did not signal after termination")
	}
	select {
	case exitErr := <-child.exited:
		if exitErr == nil {
			t.Fatal("startup Go fixture did not report expected failure exit")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("startup Go process Wait did not finish")
	}
	if status, waitErr := windows.WaitForSingleObject(nodeHandle, 5000); waitErr != nil || status != windows.WAIT_OBJECT_0 {
		ids, queryErr := (&chromeFixtureBridge{job: child.job}).jobPIDs()
		t.Fatalf("unassigned Node survived Go death before fallback cleanup: nodeWait=%d error=%v outerPIDs=%v queryError=%v", status, waitErr, ids, queryErr)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err = processFixtureWait(ctx, func() bool {
		ids, queryErr := (&chromeFixtureBridge{job: child.job}).jobPIDs()
		return queryErr == nil && len(ids) == 0
	}); err != nil {
		t.Fatal("startup outer tree not empty before fallback cleanup")
	}
	assertNoStartupWork()
	if start.Grant.Reopen || start.Grant.TreeReleased {
		t.Fatal("startup improperly issued profile reopen authority")
	}
	t.Log("startup_release goRootSignalled=true nodeRootSignalled=true outerTreeEmpty=true innerJobNeverAssigned=true controllerJobRetained=true fallbackCleanupNotUsed=true noChrome=true noWindows=true noProfileOrSQLite=true loginCount=0 captures=0 reopenGrantIssued=false")
}
