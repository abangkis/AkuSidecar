//go:build windows && auth_session_dev

package appshell

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/abangkis/AkuSidecar/internal/ownedprocess"
	"golang.org/x/sys/windows"
)

// This separately authorized fixture launches one real browser with a brand-new
// empty profile. It never loads a source URL, extension, login or X capability.
func TestMainBrowserTransportChromeLocal(t *testing.T) {
	chrome := os.Getenv("AKU_MAIN_TRANSPORT_CHROME")
	if chrome == "" {
		t.Skip("explicit empty-profile real Chrome fixture approval required")
	}
	node, driver := os.Getenv("AKU_CHROME_FIXTURE_NODE"), os.Getenv("AKU_MAIN_TRANSPORT_DRIVER")
	for _, path := range []string{chrome, node, driver} {
		if !filepath.IsAbs(path) {
			t.Fatal("explicit absolute fixture executable paths required")
		}
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() {
			t.Fatal("fixture executable unavailable or aliased")
		}
		actual, err := filepath.EvalSymlinks(path)
		if err != nil || !strings.EqualFold(actual, filepath.Clean(path)) {
			t.Fatal("fixture executable aliases are rejected")
		}
	}
	project, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	build := filepath.Join(project, "build")
	root := os.Getenv("AKU_MAIN_CHROME_FIXTURE_ROOT")
	if !filepath.IsAbs(root) || filepath.Clean(root) != root || !strings.EqualFold(filepath.Dir(root), build) ||
		!strings.HasPrefix(filepath.Base(root), "x-main-chrome-local-") {
		t.Fatal("explicit direct project build/x-main-chrome-local-* fixture root required")
	}
	actualBuild, err := filepath.EvalSymlinks(build)
	if err != nil || !strings.EqualFold(actualBuild, build) {
		t.Fatal("fixture build parent must be an existing plain directory")
	}
	actualRoot, err := filepath.EvalSymlinks(root)
	if err != nil || !strings.EqualFold(actualRoot, root) {
		t.Fatal("fixture root must be an existing plain directory")
	}
	if info, err := os.Lstat(filepath.Join(root, "attempt.json")); err != nil || !info.Mode().IsRegular() {
		t.Fatal("explicit fixture attempt receipt required")
	}
	// The helper owns its package/receipt/log root. Only this new child is a
	// browser profile, and only this child may be removed by fixture cleanup.
	profile, err := os.MkdirTemp(root, "empty-profile-")
	if err != nil {
		t.Fatal(err)
	}
	profileID, err := ownedprocess.DirectoryID(profile)
	if err != nil {
		t.Fatal("new fixture profile identity unavailable", err)
	}
	pin := func(path string) string {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(b)
		return hex.EncodeToString(sum[:])
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	window, err := Launch(ctx, LaunchOptions{
		Executable: chrome, UserDataDir: profile, URL: "about:blank", PrivateCDP: true, StartMinimized: true,
		MainProfileDirectory: "Default",
		MainBrowserTransport: &MainBrowserTransportOptions{Node: node, Driver: driver, NodeSHA256: pin(node), DriverSHA256: pin(driver), StartupTimeout: 10 * time.Second},
		ExtraArgs:            []string{"--profile-directory=Default", "--start-minimized", "--disable-extensions", "--disable-background-networking", "--disable-component-update", "--disable-sync", "--host-resolver-rules=MAP * ~NOTFOUND"},
	})
	if err != nil {
		t.Fatalf("fixture launch failed; retain %s until owned cleanup is verified: %v", profile, err)
	}
	t.Cleanup(func() {
		// Explicit fixture cleanup may terminate only this newly owned tree.
		// The minimized capture host has no handoff endpoint in this fixture.
		// Explicit owned shutdown remains available after its Node pipe exits.
		cleanup, stop := context.WithTimeout(context.Background(), 30*time.Second)
		defer stop()
		window.Terminate()
		if err := window.WaitForNaturalClose(cleanup); err != nil {
			t.Errorf("fixture cleanup uncertain; retain %s: %v", profile, err)
			return
		}
		actual, err := filepath.EvalSymlinks(profile)
		if err != nil || !strings.EqualFold(actual, profile) || !strings.EqualFold(filepath.Dir(actual), root) || !strings.HasPrefix(filepath.Base(actual), "empty-profile-") {
			t.Errorf("fixture cleanup containment changed; retain %s", profile)
			return
		}
		if currentID, err := ownedprocess.DirectoryID(profile); err != nil || currentID != profileID {
			t.Errorf("fixture profile identity changed; retain %s", profile)
			return
		}
		// Inspect names/metadata only, never profile file contents. Refuse aliases
		// before recursively removing the one newly created fixture directory.
		err = filepath.WalkDir(profile, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return errors.New("fixture alias prevents cleanup")
			}
			return nil
		})
		if err != nil {
			t.Errorf("fixture cleanup inventory failed; retain %s: %v", profile, err)
			return
		}
		if currentID, err := ownedprocess.DirectoryID(profile); err != nil || currentID != profileID {
			t.Errorf("fixture profile identity changed during cleanup inventory; retain %s", profile)
			return
		}
		if err := os.RemoveAll(profile); err != nil {
			t.Error(err)
		}
	})
	protocol, ok := window.CaptureProtocol().(MainBrowserProtocol)
	if !ok {
		t.Fatal("main Node protocol missing")
	}
	raw, err := protocol.Call(ctx, "Browser.getVersion", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	var version struct {
		Product string `json:"product"`
	}
	if json.Unmarshal(raw, &version) != nil || version.Product == "" {
		t.Fatal("real Chrome version projection missing")
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, uint32(window.PID()))
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(h)
	before, err := ownedprocess.IdentityFromHandle(h)
	if err != nil {
		t.Fatal(err)
	}
	checkJob := func(job windows.Handle) error {
		member, err := ownedprocess.Membership(h, job)
		if err != nil {
			return err
		}
		if !member {
			return errors.New("main fixture is outside its owned Job")
		}
		return nil
	}
	if err := window.WithMainBrowserJob(profile, "Default", checkJob); err != nil {
		t.Fatal(err)
	}
	if err := window.WithMainBrowserJob(profile, "Profile 2", checkJob); err == nil {
		t.Fatal("wrong subprofile accepted")
	}
	windowDeadline := time.Now().Add(5 * time.Second)
	for !mainFixtureTopLevelWindow(uint32(window.PID())) && time.Now().Before(windowDeadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if !mainFixtureTopLevelWindow(uint32(window.PID())) {
		t.Fatal("fixture Chrome has no visible top-level browser window")
	}
	worker := protocol.TransportWorker()
	worker.Terminate() // Only the fixture Node Job; never the main Chrome Job.
	stop, stopCancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer stopCancel()
	if err := worker.WaitForCleanup(stop); err != nil {
		t.Fatal(err)
	}
	state, err := windows.WaitForSingleObject(h, 2000)
	if err != nil || state != uint32(windows.WAIT_TIMEOUT) {
		t.Fatal("transport exit also ended fixture Chrome", state, err)
	}
	after, err := ownedprocess.IdentityFromHandle(h)
	if err != nil || before != after {
		t.Fatal("fixture Chrome process identity changed")
	}
	if err := window.WithMainBrowserJob(profile, "Default", checkJob); err != nil {
		t.Fatal(err)
	}
	if !mainFixtureTopLevelWindow(uint32(window.PID())) {
		t.Fatal("fixture Chrome window disappeared after transport exit")
	}
	t.Log("empty-profile Chrome remained owned and windowed for two seconds after fixture Node exit; no X startup or transfer attempted")
}

func mainFixtureTopLevelWindow(pid uint32) bool {
	found := false
	captureEnumerationMu.Lock()
	defer captureEnumerationMu.Unlock()
	captureEnumerationVisit = func(hwnd uintptr) {
		var actual uint32
		procGetWindowThreadProcess.Call(hwnd, uintptr(unsafe.Pointer(&actual)))
		if actual != pid {
			return
		}
		visible, _, _ := procIsWindowVisible.Call(hwnd)
		if visible == 0 {
			return
		}
		var class [128]uint16
		read, _, _ := captureWindowClass.Call(hwnd, uintptr(unsafe.Pointer(&class[0])), uintptr(len(class)))
		if read != 0 && windows.UTF16ToString(class[:]) == "Chrome_WidgetWin_1" {
			found = true
		}
	}
	defer func() { captureEnumerationVisit = nil }()
	ok, _, _ := procEnumWindows.Call(captureEnumerationCallback, 0)
	return ok != 0 && found
}
