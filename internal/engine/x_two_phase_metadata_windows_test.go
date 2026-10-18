//go:build windows && auth_session_dev && auth_session_fixture

package engine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func validateXTwoPhaseDirectory(root, rootID, profileID string) error {
	currentRoot, rootErr := processFixtureDirectoryID(root)
	currentProfile, profileErr := processFixtureDirectoryID(filepath.Join(root, "shared"))
	if rootErr != nil || profileErr != nil || rootID == "" || profileID == "" || currentRoot != rootID || currentProfile != profileID {
		return errors.New("two-phase directory identity changed")
	}
	return nil
}

// Hold the owned directory against replacement throughout the handoff while
// allowing Chrome to read and write files inside it.
func pinXTwoPhaseDirectory(path string) (windows.Handle, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, errors.New("two-phase directory path invalid")
	}
	handle, err := windows.CreateFile(name, windows.FILE_LIST_DIRECTORY|windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil,
		windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return 0, errors.New("two-phase directory pin unavailable")
	}
	var info windows.ByHandleFileInformation
	if windows.GetFileInformationByHandle(handle, &info) != nil || info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		_ = windows.CloseHandle(handle)
		return 0, errors.New("two-phase directory pin rejected")
	}
	return handle, nil
}

func TestXTwoPhaseMetadataOffline(t *testing.T) {
	parent := os.Getenv("AKU_CHROME_FIXTURE_ROOT")
	if parent == "" {
		t.Skip("two-phase directory preflight requires the project-local helper")
	}
	root, err := os.MkdirTemp(parent, "run-x-two-phase-offline-")
	if err != nil {
		t.Fatal("offline directory unavailable")
	}
	profile := filepath.Join(root, "shared")
	if os.Mkdir(profile, 0700) != nil {
		t.Fatal("offline profile unavailable")
	}
	rootID, _ := processFixtureDirectoryID(root)
	profileID, _ := processFixtureDirectoryID(profile)
	if validateXTwoPhaseDirectory(root, rootID, profileID) != nil {
		t.Fatal("unchanged ordinary directory identity rejected")
	}
	if os.Rename(profile, filepath.Join(root, "old-profile")) != nil || os.Mkdir(profile, 0700) != nil {
		t.Fatal("offline profile replacement unavailable")
	}
	if validateXTwoPhaseDirectory(root, rootID, profileID) == nil {
		t.Fatal("replacement at the same profile path was accepted")
	}
	if validateXTwoPhaseDirectory(root, "", profileID) == nil {
		t.Fatal("missing root identity was accepted")
	}
	pinned, err := pinXTwoPhaseDirectory(profile)
	if err != nil {
		t.Fatal("offline profile pin unavailable")
	}
	moveErr := os.Rename(profile, filepath.Join(root, "pinned-profile"))
	_ = windows.CloseHandle(pinned)
	if moveErr == nil {
		t.Fatal("pinned profile could be moved during handoff")
	}
	if os.Rename(profile, filepath.Join(root, "released-profile")) != nil {
		t.Fatal("released profile pin still blocked cleanup")
	}
	// The canonical helper removes this ordinary, project-owned offline tree.
}

// Close only windows belonging to the pinned QA Chrome root. Remaining owned
// descendants may be terminated only after the root has exited gracefully.
func closeXTwoPhaseChrome(bridge *chromeFixtureBridge, root windows.Handle, pid uint32) (int, int, error) {
	identity, err := processFixtureIdentityFromHandle(root)
	if err != nil || identity.PID != pid || bridge.containsPID(pid) != nil {
		return 0, 0, errors.New("phase-one process identity unavailable")
	}
	handles := processFixtureWindows(pid)
	if len(handles) > 32 {
		return 0, 0, errors.New("phase-one window scope exceeded")
	}
	rootState, waitErr := windows.WaitForSingleObject(root, 0)
	if waitErr != nil || rootState != windows.WAIT_OBJECT_0 && rootState != uint32(windows.WAIT_TIMEOUT) || len(handles) == 0 && rootState != windows.WAIT_OBJECT_0 {
		return 0, 0, errors.New("phase-one visible window scope unavailable")
	}
	for _, hwnd := range handles {
		present, _, _ := processFixtureUser.NewProc("IsWindow").Call(hwnd)
		if present == 0 {
			continue
		}
		current, err := processFixtureProcessIdentity(pid)
		if err != nil || current != identity || processFixtureWindowPID(hwnd) != pid || bridge.containsPID(pid) != nil {
			return 0, 0, errors.New("phase-one window ownership changed")
		}
		posted, _, _ := processFixtureUser.NewProc("PostMessageW").Call(hwnd, 0x0010, 0, 0)
		if posted == 0 {
			return 0, 0, errors.New("phase-one graceful close failed")
		}
	}
	state, err := windows.WaitForSingleObject(root, 15000)
	if err != nil || state != windows.WAIT_OBJECT_0 || !processFixtureWindowsGone(handles) {
		return len(handles), 0, errors.New("phase-one root or windows still active")
	}
	natural, cancelNatural := context.WithTimeout(context.Background(), 3*time.Second)
	err = bridge.assertBridgeOnly(natural)
	cancelNatural()
	terminated := 0
	if err != nil {
		pids, listErr := bridge.jobPIDs()
		if listErr != nil || len(pids) > 64 {
			return len(handles), 0, errors.New("phase-one residual scope unavailable")
		}
		for _, remaining := range pids {
			if remaining == bridge.pid {
				continue
			}
			handle, openErr := windows.OpenProcess(windows.PROCESS_TERMINATE|windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, remaining)
			if openErr != nil {
				// A process may have exited since the Job snapshot; final proof is
				// still the authoritative empty Chrome tree below.
				continue
			}
			bridge.jobMu.Lock()
			member, memberErr := chromeFixtureJobMembership(handle, bridge.job)
			bridge.jobMu.Unlock()
			if memberErr != nil || !member || remaining == bridge.pid {
				_ = windows.CloseHandle(handle)
				return len(handles), terminated, errors.New("phase-one residual handle outside owned Job")
			}
			killErr := windows.TerminateProcess(handle, 1)
			_ = windows.CloseHandle(handle)
			if killErr != nil {
				return len(handles), terminated, errors.New("phase-one owned residual cleanup failed")
			}
			terminated++
		}
	}
	final, cancelFinal := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelFinal()
	if bridge.assertBridgeOnly(final) != nil {
		return len(handles), terminated, errors.New("phase-one Chrome tree release unverified")
	}
	return len(handles), terminated, nil
}

func TestXTwoPhaseMetadata(t *testing.T) {
	if os.Getenv("AKU_X_TWO_PHASE_ENABLED") != "1" {
		t.Skip("two-phase QA requires explicit manual-login helper")
	}
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("two-phase source root unavailable")
	}
	project := filepath.Clean(filepath.Join(filepath.Dir(source), "..", ".."))
	qaParent := filepath.Join(project, "runtime", "qa", "auth-session-sidecar")
	outputPath := func(variable, suffix string) string {
		path := os.Getenv(variable)
		if !samePath(filepath.Dir(path), qaParent) || !regexp.MustCompile(`^x-two-phase-[a-f0-9]{32}\.`+suffix+`$`).MatchString(filepath.Base(path)) {
			t.Fatal("two-phase output scope invalid")
		}
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("two-phase output must be new")
		}
		return path
	}
	progressPath := outputPath("AKU_X_TWO_PHASE_PROGRESS_PATH", `progress\.jsonl`)
	resultPath := outputPath("AKU_X_TWO_PHASE_RESULT_PATH", `result\.json`)
	reportPath := outputPath("AKU_X_TWO_PHASE_REPORT_PATH", `report\.json`)
	progress, err := os.OpenFile(progressPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal("two-phase progress unavailable")
	}
	t.Cleanup(func() { _ = progress.Close() })
	stage := func(value string) {
		if json.NewEncoder(progress).Encode(map[string]string{"stage": value}) != nil {
			t.Fatal("two-phase progress write failed")
		}
		_ = progress.Sync()
		t.Log("Two-phase QA stage:", value)
	}
	server := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(server.Close)
	var scopeNonce [8]byte
	if _, err := rand.Read(scopeNonce[:]); err != nil {
		t.Fatal("two-phase scope unavailable")
	}
	caseRoot := filepath.Join(os.Getenv("AKU_CHROME_FIXTURE_ROOT"), "run-x-two-phase-"+hex.EncodeToString(scopeNonce[:]))
	if os.Mkdir(caseRoot, 0700) != nil {
		t.Fatal("two-phase case root unavailable")
	}
	var directories []windows.Handle
	t.Cleanup(func() {
		for _, directory := range directories {
			_ = windows.CloseHandle(directory)
		}
	})
	rootDirectory, err := pinXTwoPhaseDirectory(caseRoot)
	if err != nil {
		t.Fatal("two-phase root pin unavailable")
	}
	directories = append(directories, rootDirectory)
	rootID, err := processFixtureDirectoryID(caseRoot)
	if err != nil {
		t.Fatal("two-phase root identity unavailable")
	}
	config := chromeFixtureConfig{NodePath: os.Getenv("AKU_TEST_NODE_PATH"), ChromePath: os.Getenv("AKU_TEST_CHROME_PATH"),
		Root: caseRoot, Origin: server.URL, DriverPath: filepath.Join(project, "scripts", "auth-session-fixture", "x-two-phase-metadata-driver.mjs")}
	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Minute)
	defer cancel()
	bridge, err := startChromeFixtureBridge(ctx, config)
	if err != nil {
		t.Fatal("two-phase bridge startup failed:", err)
	}
	registerChromeFixtureBridge(bridge)
	var roots []windows.Handle
	t.Cleanup(func() {
		bridge.terminateOwnedTree()
		if bridge.cleanupError() != nil || !bridge.cleanupVerified() {
			t.Error("two-phase owned Job cleanup unverified")
		}
		for _, root := range roots {
			state, waitErr := windows.WaitForSingleObject(root, 5000)
			if waitErr != nil || state != windows.WAIT_OBJECT_0 {
				t.Error("two-phase Chrome root exit unverified")
			}
			_ = windows.CloseHandle(root)
		}
		if !t.Failed() {
			stage("cleanup_verified")
		}
	})
	pin := func(owner *chromeFixtureProcess) windows.Handle {
		root, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(owner.PID()))
		if err != nil {
			t.Fatal("two-phase Chrome root handle unavailable")
		}
		roots = append(roots, root)
		verifyXChromeFixtureImage(t, uint32(owner.PID()), config.ChromePath)
		return root
	}
	stage("bridge_ready")
	first, err := bridge.launch(ctx, "collector")
	if err != nil {
		t.Fatal("two-phase normal Chrome launch failed:", err)
	}
	firstRoot := pin(first)
	verifyXChromeFixtureImage(t, bridge.pid, config.NodePath)
	profileDirectory, err := pinXTwoPhaseDirectory(filepath.Join(caseRoot, "shared"))
	if err != nil {
		t.Fatal("two-phase profile pin unavailable")
	}
	directories = append(directories, profileDirectory)
	profileID, err := processFixtureDirectoryID(filepath.Join(caseRoot, "shared"))
	if err != nil || validateXTwoPhaseDirectory(caseRoot, rootID, profileID) != nil {
		t.Fatal("two-phase new profile identity unavailable")
	}
	var baseline map[uintptr]xPopupWindowState
	baselineVisible := false
	baselineDeadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(baselineDeadline) {
		baseline, err = xPopupWindowSnapshot(bridge, firstRoot, uint32(first.PID()))
		visible := false
		for _, state := range baseline {
			visible = visible || state.visible
		}
		if err == nil && visible {
			baselineVisible = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	tracker := newXPopupWindowTracker(baseline)
	if err != nil || !baselineVisible {
		tracker.failed = true
	}
	stage("phase1_manual_login")
	manual := "timeout"
	manualPopup := "not_observed"
	deadline := time.Now().Add(5 * time.Minute)
	for time.Now().Before(deadline) {
		snapshot, inventoryErr := xPopupWindowSnapshot(bridge, firstRoot, uint32(first.PID()))
		tracker.sample(snapshot, inventoryErr)
		info, err := os.Lstat(resultPath)
		if err == nil {
			if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > 128 {
				t.Fatal("two-phase manual result invalid")
			}
			file, openErr := os.Open(resultPath)
			if openErr != nil {
				t.Fatal("two-phase manual result unavailable")
			}
			raw, readErr := io.ReadAll(io.LimitReader(file, 129))
			_ = file.Close()
			if readErr != nil {
				t.Fatal("two-phase manual result read failed")
			}
			manual, manualPopup, err = decodeXTwoPhaseOutcome(raw)
			if err != nil {
				t.Fatal("two-phase manual result rejected")
			}
			break
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Fatal("two-phase manual result scope unavailable")
		}
		time.Sleep(200 * time.Millisecond)
	}
	report := struct {
		Schema                      string                 `json:"schema"`
		ManualPhaseOneOutcome       string                 `json:"manualPhaseOneOutcome"`
		ManualSource                string                 `json:"manualSource"`
		OldRootExited               bool                   `json:"oldRootExited"`
		OldChromeTreeReleased       bool                   `json:"oldChromeTreeReleased"`
		ClosedWindowCount           int                    `json:"closedWindowCount"`
		ResidualProcessesTerminated int                    `json:"residualProcessesTerminated"`
		ProfileDirectorySame        bool                   `json:"profileDirectorySame"`
		WindowDiagnostics           xPopupWindowSummary    `json:"windowDiagnostics"`
		ManualPopupObservation      string                 `json:"manualPopupObservation"`
		PhaseTwoHomeObserved        bool                   `json:"phaseTwoHomeObserved"`
		PermitInherited             bool                   `json:"permitInherited"`
		Diagnostic                  string                 `json:"diagnostic"`
		Observation                 *xNativeMetadataReport `json:"observation,omitempty"`
		CanAuthorize                bool                   `json:"canAuthorize"`
	}{Schema: "aku.x.two-phase-metadata.v2", ManualPhaseOneOutcome: manual, ManualSource: "manual_user_report", WindowDiagnostics: tracker.summary(), ManualPopupObservation: manualPopup}
	if manual == "timeout" {
		report.ManualSource = "harness_deadline"
	}
	defer func() {
		output, err := os.OpenFile(reportPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			t.Error("two-phase report unavailable")
			return
		}
		defer output.Close()
		if json.NewEncoder(output).Encode(report) != nil {
			t.Error("two-phase report write failed")
		}
	}()
	if manual != "home_visible" {
		report.Diagnostic = "phase1_login_not_confirmed"
		t.Fatal("two-phase normal login was not confirmed")
	}
	stage("phase1_closing")
	windowsClosed, residuals, err := closeXTwoPhaseChrome(bridge, firstRoot, uint32(first.PID()))
	report.ClosedWindowCount, report.ResidualProcessesTerminated = windowsClosed, residuals
	if err != nil {
		report.Diagnostic = "phase1_release_failed"
		t.Fatal("two-phase old Chrome release failed:", err)
	}
	report.OldRootExited, report.OldChromeTreeReleased = true, true
	if validateXTwoPhaseDirectory(caseRoot, rootID, profileID) != nil {
		report.Diagnostic = "profile_identity_changed"
		t.Fatal("two-phase profile changed before grant")
	}
	stage("phase1_released")
	var nonce [32]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal("two-phase private handoff nonce unavailable")
	}
	var authorization struct {
		Authorized bool `json:"authorized"`
	}
	if bridge.rpc(ctx, "authorize_observation", map[string]any{"owner": first.owner, "oldPID": first.PID(), "nonce": hex.EncodeToString(nonce[:])}, &authorization) != nil || !authorization.Authorized {
		report.Diagnostic = "handoff_grant_rejected"
		t.Fatal("two-phase private handoff grant rejected")
	}
	stage("phase2_starting")
	second, err := bridge.launch(ctx, "collector")
	if err != nil {
		report.Diagnostic = "phase2_start_failed"
		t.Fatal("two-phase observed Chrome launch failed:", err)
	}
	pin(second)
	if second.PID() == first.PID() || validateXTwoPhaseDirectory(caseRoot, rootID, profileID) != nil {
		report.Diagnostic = "phase2_identity_mismatch"
		t.Fatal("two-phase new process or same-profile proof failed")
	}
	report.ProfileDirectorySame = true
	stage("phase2_checking_home")
	deadline = time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		var state struct {
			Ready bool `json:"ready"`
		}
		if err := bridge.rpc(ctx, "login_state", map[string]any{"owner": second.owner}, &state); err != nil {
			report.Diagnostic = "phase2_home_check_failed"
			t.Fatal("two-phase Home check failed:", err)
		}
		if state.Ready {
			report.PhaseTwoHomeObserved = true
			break
		}
		time.Sleep(time.Second)
	}
	if !report.PhaseTwoHomeObserved {
		report.Diagnostic = "phase2_home_not_ready"
		t.Fatal("Home unavailable after reopen; no second login attempted")
	}
	stage("phase2_observing")
	var raw json.RawMessage
	if err := bridge.rpc(ctx, "start_observation", map[string]any{"owner": second.owner}, &raw); err != nil {
		report.Diagnostic = "phase2_observation_failed"
		t.Fatal("two-phase observation failed:", err)
	}
	metadata, err := decodeXNativeMetadata(raw)
	if err != nil {
		report.Diagnostic = "metadata_projection_rejected"
		t.Fatal("two-phase metadata failed private output boundary")
	}
	report.Observation = &metadata
	if metadata.CandidateCount == 0 {
		report.Diagnostic = "no_native_operations_seen"
		t.Fatal("two-phase metadata sample contained no native operations")
	}
	report.Diagnostic = "metadata_observed"
	t.Logf("Two-phase metadata: %d candidates, %d dropped; no authorization or lineage proof", metadata.CandidateCount, metadata.DroppedCandidates)
	stage("complete")
}
