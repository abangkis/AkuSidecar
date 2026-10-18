//go:build windows && auth_session_dev && auth_session_fixture

package engine

import (
	"bytes"
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
	"strings"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

type xLocalPopupClick struct {
	Trusted  bool `json:"trusted"`
	Active   bool `json:"active"`
	Returned bool `json:"returned"`
}

func decodeXLocalPopupClick(raw []byte) (xLocalPopupClick, error) {
	invalid := errors.New("local popup click rejected")
	result := xLocalPopupClick{}
	if len(raw) == 0 || len(raw) > 128 {
		return result, invalid
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	start, err := decoder.Token()
	if err != nil || start != json.Delim('{') {
		return result, invalid
	}
	seen := map[string]bool{}
	for decoder.More() {
		key, err := decoder.Token()
		name, ok := key.(string)
		if err != nil || !ok || seen[name] || name != "trusted" && name != "active" && name != "returned" {
			return xLocalPopupClick{}, invalid
		}
		value, err := decoder.Token()
		flag, ok := value.(bool)
		if err != nil || !ok {
			return xLocalPopupClick{}, invalid
		}
		seen[name] = true
		switch name {
		case "trusted":
			result.Trusted = flag
		case "active":
			result.Active = flag
		case "returned":
			result.Returned = flag
		}
	}
	end, err := decoder.Token()
	if err != nil || end != json.Delim('}') || len(seen) != 3 || decoder.Decode(new(any)) != io.EOF {
		return xLocalPopupClick{}, invalid
	}
	return result, nil
}

func TestXLocalPopupOffline(t *testing.T) {
	valid := []byte(`{"trusted":true,"active":true,"returned":false}`)
	got, err := decodeXLocalPopupClick(valid)
	if err != nil || !got.Trusted || !got.Active || got.Returned {
		t.Fatal("bounded negative popup outcome rejected")
	}
	for _, raw := range []string{`{"trusted":true,"active":true,"returned":true,"url":"private"}`, `{"trusted":true,"trusted":false,"active":true,"returned":true}`, `{"trusted":true,"active":true}`, `{"trusted":1,"active":true,"returned":true}`, string(valid) + `{}`, string(bytes.Repeat([]byte(" "), 129))} {
		if _, err := decodeXLocalPopupClick([]byte(raw)); err == nil {
			t.Fatal("unsafe or ambiguous click event accepted")
		}
	}
}

// Queue one native mouse sequence to the owned QA browser HWND. This does not
// move the cursor, activate a window or inject input into another application.
func postXLocalPopupClick(bridge *chromeFixtureBridge, root windows.Handle, pid uint32, hwnd uintptr) error {
	if _, err := xPopupWindowSnapshot(bridge, root, pid); err != nil || processFixtureWindowPID(hwnd) != pid {
		return errors.New("local popup input ownership unavailable")
	}
	target := hwnd
	var rect struct{ Left, Top, Right, Bottom int32 }
	queried, _, _ := processFixtureUser.NewProc("GetClientRect").Call(target, uintptr(unsafe.Pointer(&rect)))
	if queried == 0 || rect.Right < 400 || rect.Bottom < 400 || rect.Right > 32767 || rect.Bottom > 32767 {
		return errors.New("local popup render geometry unavailable")
	}
	position := uintptr(uint32(rect.Right/2) | uint32(rect.Bottom/2)<<16)
	for _, event := range []struct{ message, flags uintptr }{{0x0200, 0}, {0x0201, 1}, {0x0202, 0}} {
		if processFixtureWindowPID(target) != pid || bridge.containsPID(pid) != nil {
			return errors.New("local popup target changed")
		}
		posted, _, _ := processFixtureUser.NewProc("PostMessageW").Call(target, event.message, event.flags, position)
		if posted == 0 {
			return errors.New("local popup scoped input failed")
		}
	}
	return nil
}

func TestXLocalPopup(t *testing.T) {
	if os.Getenv("AKU_X_LOCAL_POPUP_ENABLED") != "1" {
		t.Skip("local popup requires the explicit QA helper")
	}
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("local popup source unavailable")
	}
	project := filepath.Clean(filepath.Join(filepath.Dir(source), "..", ".."))
	qaParent := filepath.Join(project, "runtime", "qa", "auth-session-sidecar")
	reportPath := os.Getenv("AKU_X_LOCAL_POPUP_REPORT_PATH")
	if !samePath(filepath.Dir(reportPath), qaParent) || !regexp.MustCompile(`^x-local-popup-[a-f0-9]{32}\.report\.json$`).MatchString(filepath.Base(reportPath)) {
		t.Fatal("local popup report scope rejected")
	}
	physical := os.Getenv("AKU_X_LOCAL_POPUP_PHYSICAL") == "1"
	progressPath := os.Getenv("AKU_X_LOCAL_POPUP_PROGRESS_PATH")
	if physical && (!samePath(filepath.Dir(progressPath), qaParent) || filepath.Base(progressPath) != strings.TrimSuffix(filepath.Base(reportPath), "report.json")+"progress.jsonl") {
		t.Fatal("local popup progress scope rejected")
	}
	report := struct {
		Schema                string               `json:"schema"`
		Input                 string               `json:"input"`
		Diagnostic            string               `json:"diagnostic"`
		Click                 *xLocalPopupClick    `json:"click"`
		PopupDocumentLoaded   *bool                `json:"popupDocumentLoaded"`
		ProfileFileOperations string               `json:"profileFileOperations"`
		DirectoryPinsEnabled  bool                 `json:"directoryPinsEnabled"`
		Windows               *xPopupWindowSummary `json:"windows"`
		CanAuthorize          bool                 `json:"canAuthorize"`
	}{Schema: "aku.x.local-popup.v1", Input: "scoped_native_messages_not_physical", Diagnostic: "setup_incomplete", ProfileFileOperations: "not_run"}
	if physical {
		report.Input = "manual_physical_click_requested"
	}
	defer func() {
		output, err := os.OpenFile(reportPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			t.Error("local popup report unavailable")
			return
		}
		encodeErr := json.NewEncoder(output).Encode(report)
		closeErr := output.Close()
		if encodeErr != nil || closeErr != nil {
			t.Error("local popup report failed")
		}
	}()
	var nonceBytes [32]byte
	if _, err := rand.Read(nonceBytes[:]); err != nil {
		t.Fatal("local popup nonce unavailable")
	}
	nonce := hex.EncodeToString(nonceBytes[:])
	ready, loaded := make(chan struct{}, 1), make(chan struct{}, 1)
	clicked := make(chan xLocalPopupClick, 1)
	var rootHTML, popupHTML string
	var origin string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; connect-src 'self'; base-uri 'none'; frame-ancestors 'none'")
		if r.Method == http.MethodGet && (r.URL.Path == "/" || r.URL.Path == "/popup") {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			if r.URL.Path == "/" {
				_, _ = io.WriteString(w, rootHTML)
			} else {
				_, _ = io.WriteString(w, popupHTML)
			}
			return
		}
		if r.Method != http.MethodPost || r.Header.Get("Origin") != origin || r.Header.Get("X-Aku-Local-Popup") != nonce {
			http.Error(w, "denied", http.StatusForbidden)
			return
		}
		raw, err := io.ReadAll(io.LimitReader(r.Body, 129))
		if err != nil || len(raw) > 128 {
			http.Error(w, "invalid", http.StatusBadRequest)
			return
		}
		switch r.URL.Path {
		case "/ready":
			if len(raw) != 0 {
				http.Error(w, "invalid", 400)
				return
			}
			select {
			case ready <- struct{}{}:
			default:
			}
		case "/popup-loaded":
			if len(raw) != 0 {
				http.Error(w, "invalid", 400)
				return
			}
			select {
			case loaded <- struct{}{}:
			default:
			}
		case "/click":
			value, err := decodeXLocalPopupClick(raw)
			if err != nil {
				http.Error(w, "invalid", 400)
				return
			}
			select {
			case clicked <- value:
			default:
			}
		default:
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	origin = server.URL
	t.Cleanup(server.Close)
	caseRoot := filepath.Join(os.Getenv("AKU_CHROME_FIXTURE_ROOT"), "run-x-local-popup-"+nonce[:16])
	if os.Mkdir(caseRoot, 0700) != nil {
		t.Fatal("local popup root unavailable")
	}
	var directories []windows.Handle
	t.Cleanup(func() {
		for _, handle := range directories {
			_ = windows.CloseHandle(handle)
		}
	})
	rootDirectory, err := pinXTwoPhaseDirectory(caseRoot)
	if err != nil {
		t.Fatal("local popup root pin failed")
	}
	directories = append(directories, rootDirectory)
	rootID, _ := processFixtureDirectoryID(caseRoot)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	config := chromeFixtureConfig{NodePath: os.Getenv("AKU_TEST_NODE_PATH"), ChromePath: os.Getenv("AKU_TEST_CHROME_PATH"), Root: caseRoot, Origin: origin, DriverPath: filepath.Join(project, "scripts", "auth-session-fixture", "x-local-popup-control.mjs")}
	bridge, err := startChromeFixtureBridge(ctx, config)
	if err != nil {
		t.Fatal("local popup bridge failed")
	}
	registerChromeFixtureBridge(bridge)
	var root windows.Handle
	t.Cleanup(func() {
		bridge.terminateOwnedTree()
		if bridge.cleanupError() != nil || !bridge.cleanupVerified() {
			t.Error("local popup Job cleanup unverified")
		}
		if root != 0 {
			state, err := windows.WaitForSingleObject(root, 5000)
			if err != nil || state != windows.WAIT_OBJECT_0 {
				t.Error("local popup Chrome exit unverified")
			}
			_ = windows.CloseHandle(root)
		}
	})
	var documents struct {
		Root  string `json:"root"`
		Popup string `json:"popup"`
	}
	if bridge.rpc(ctx, "local_popup_documents", nil, &documents) != nil || len(documents.Root) > 16384 || len(documents.Popup) > 8192 || !strings.Contains(documents.Root, "__AKU_LOCAL_POPUP_NONCE__") || !strings.Contains(documents.Popup, "__AKU_LOCAL_POPUP_NONCE__") {
		t.Fatal("local popup static documents rejected")
	}
	rootHTML = strings.ReplaceAll(documents.Root, "__AKU_LOCAL_POPUP_NONCE__", nonce)
	popupHTML = strings.ReplaceAll(documents.Popup, "__AKU_LOCAL_POPUP_NONCE__", nonce)
	owner, err := bridge.launch(ctx, "collector")
	if err != nil {
		t.Fatal("local popup Chrome launch failed")
	}
	root, err = windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(owner.PID()))
	if err != nil {
		t.Fatal("local popup Chrome pin failed")
	}
	verifyXChromeFixtureImage(t, uint32(owner.PID()), config.ChromePath)
	verifyXChromeFixtureImage(t, bridge.pid, config.NodePath)
	profile := filepath.Join(caseRoot, "shared")
	profileDirectory, err := pinXTwoPhaseDirectory(profile)
	if err != nil {
		t.Fatal("local popup profile pin failed")
	}
	directories = append(directories, profileDirectory)
	report.DirectoryPinsEnabled = true
	profileID, _ := processFixtureDirectoryID(profile)
	if validateXTwoPhaseDirectory(caseRoot, rootID, profileID) != nil {
		t.Fatal("local popup profile identity rejected")
	}
	probe := filepath.Join(profile, "local-popup-write-probe")
	report.ProfileFileOperations = "failed"
	if os.WriteFile(probe, []byte("synthetic"), 0600) != nil || os.Rename(probe, probe+"-renamed") != nil || os.Remove(probe+"-renamed") != nil {
		t.Fatal("ordinary profile file operations failed under pins")
	}
	report.ProfileFileOperations = "passed"
	report.Diagnostic = "page_not_ready"
	select {
	case <-ready:
	case <-time.After(15 * time.Second):
		t.Fatal("local popup page not ready")
	}
	baseline, err := xPopupWindowSnapshot(bridge, root, uint32(owner.PID()))
	if err != nil || len(baseline) == 0 {
		t.Fatal("local popup baseline window unavailable")
	}
	var initial uintptr
	for hwnd, state := range baseline {
		if state.visible {
			if initial != 0 {
				t.Fatal("local popup initial window ambiguous")
			}
			initial = hwnd
		}
	}
	if initial == 0 {
		t.Fatal("local popup initial window not visible")
	}
	tracker := newXPopupWindowTracker(baseline)
	summary := tracker.summary()
	report.Windows = &summary
	clickTimeout := 10 * time.Second
	if physical {
		progress, err := os.OpenFile(progressPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			t.Fatal("local popup progress unavailable")
		}
		writeErr := json.NewEncoder(progress).Encode(map[string]string{"stage": "local_popup_ready_for_physical_click"})
		closeErr := progress.Close()
		if writeErr != nil || closeErr != nil {
			t.Fatal("local popup ready marker failed")
		}
		t.Log("Local popup ready: click Open local popup once after the coordinator's instruction; no account/login")
		clickTimeout = 120 * time.Second
	} else {
		report.Diagnostic = "scoped_input_unavailable"
		if err := postXLocalPopupClick(bridge, root, uint32(owner.PID()), initial); err != nil {
			t.Fatal("local popup scoped click unavailable:", err)
		}
	}
	var click xLocalPopupClick
	report.Diagnostic = "click_not_observed"
	select {
	case click = <-clicked:
	case <-time.After(clickTimeout):
		t.Fatal("local popup click was not observed")
	}
	report.Click = &click
	childLoaded := false
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-loaded:
			childLoaded = true
		default:
		}
		snapshot, err := xPopupWindowSnapshot(bridge, root, uint32(owner.PID()))
		tracker.sample(snapshot, err)
		if childLoaded && tracker.summary().AdditionalVisible > 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	summary = tracker.summary()
	report.PopupDocumentLoaded = &childLoaded
	report.Diagnostic = "popup_outcome_incomplete"
	if !click.Trusted || !click.Active || !click.Returned || !childLoaded || summary.AdditionalVisible == 0 || summary.Coverage != "sampled_owned_root_windows" {
		t.Fatal("local popup behavior not qualified; fixed diagnostics retained")
	}
	report.Diagnostic = "local_popup_observed"
	t.Log("Local popup passed: trusted click, active user gesture, returned handle, loaded local child, additional visible owned window; no account or native-login qualification")
}
