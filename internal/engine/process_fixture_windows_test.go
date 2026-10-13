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

	"golang.org/x/sys/windows"
)

var processFixtureCount atomic.Int64
var processFixtureCleanupFailed atomic.Bool

type processFixtureChild struct {
	cmd      *exec.Cmd
	input    io.WriteCloser
	replies  chan processFixtureReceipt
	exited   chan error
	stderr   strings.Builder
	job      windows.Handle
	identity processFixtureIdentity
}

func startProcessFixtureChild(t *testing.T, executable string, start processFixtureStart) *processFixtureChild {
	t.Helper()
	job, err := newChromeFixtureJob()
	if err != nil {
		t.Fatal(err)
	}
	child := &processFixtureChild{job: job, replies: make(chan processFixtureReceipt, 8), exited: make(chan error, 1)}
	child.cmd = exec.Command(executable)
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
		if !ok || reply.Stage != stage {
			t.Fatalf("worker receipt %q missing (received %q)", stage, reply.Stage)
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
	if os.Getenv("AKU_PROCESS_FIXTURE_ENABLED") != "1" {
		t.Skip("requires explicit VerifyProcessRestart helper")
	}
	taskRoot := os.Getenv("AKU_CHROME_FIXTURE_ROOT")
	executable := filepath.Join(taskRoot, "auth-session-process-fixture.exe")
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
			start := processFixtureStart{Grant: processFixtureGrant{processFixtureLineage: lineage}, NodePath: config.NodePath, ChromePath: config.ChromePath, DriverPath: config.DriverPath, Origin: config.Origin, AuthorityKey: nonce}
			first := startProcessFixtureChild(t, executable, start)
			paused := first.receive(t, "paused_reader")
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
			fixture.authority.Lock()
			fixture.active = savedAuthority
			fixture.authority.Unlock()
			if _, err = fmt.Fprintln(second.input, "authorize"); err != nil {
				t.Fatal(err)
			}
			recovered := second.receive(t, "recovered")
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
		})
	}
}
