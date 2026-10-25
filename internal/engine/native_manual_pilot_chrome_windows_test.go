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
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Only the explicit standalone helper runs this test. Identity/cookies are
// invented loopback data; actual OS and CDP ownership never promote provenance.
const nativeManualChromeAccount = "1908123456789012345"

type nativeManualChromeHTTP struct {
	mu                       sync.Mutex
	server                   *httptest.Server
	token                    string
	logins, settings         atomic.Int32
	feedEntered, feedRelease chan struct{}
}

func startNativeManualChromeHTTP(t *testing.T) *nativeManualChromeHTTP {
	t.Helper()
	f := &nativeManualChromeHTTP{}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json")
		if r.URL.RawQuery != "" {
			w.WriteHeader(400)
			return
		}
		if r.Method == "GET" && r.URL.Path == "/" {
			w.Header().Set("Content-Type", "text/html")
			io.WriteString(w, "<!doctype html><title>Private loopback fixture</title><h1>Synthetic ownership fixture</h1>")
			return
		}
		if r.Method == "POST" && r.URL.Path == "/login" {
			var token [32]byte
			if _, err := rand.Read(token[:]); err != nil {
				w.WriteHeader(500)
				return
			}
			f.mu.Lock()
			f.token = hex.EncodeToString(token[:])
			value := f.token
			f.mu.Unlock()
			f.logins.Add(1)
			http.SetCookie(w, &http.Cookie{Name: "fixture_session", Value: value, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode})
			json.NewEncoder(w).Encode(map[string]string{"accountId": nativeManualChromeAccount})
			return
		}
		cookie, err := r.Cookie("fixture_session")
		f.mu.Lock()
		valid := err == nil && f.token != "" && cookie.Value == f.token
		f.mu.Unlock()
		if !valid {
			w.WriteHeader(401)
			io.WriteString(w, `{"authenticated":false}`)
			return
		}
		switch r.URL.Path {
		case "/feed":
			if f.feedEntered == nil || f.feedRelease == nil {
				w.WriteHeader(404)
				return
			}
			select {
			case f.feedEntered <- struct{}{}:
			default:
			}
			select {
			case <-f.feedRelease:
			case <-r.Context().Done():
				return
			}
			w.Header().Set("Content-Type", "text/html")
			io.WriteString(w, `<!doctype html><title>Synthetic DOM feed</title><article data-fixture-post="1908123456789012346"><span data-author>Synthetic Author</span><div data-text>A real Chrome DOM post about bounded local collection.</div><a data-permalink href="https://x.com/synthetic/status/1908123456789012346">Post</a></article>`)
		case "/startup":
			w.Header().Set("Content-Type", "text/html")
			io.WriteString(w, `<!doctype html><title>Owned startup fixture</title><script>globalThis.__aku_fixture_self='1908123456789012345';debugger;fetch('/settings',{cache:'no-store'}).then(r=>r.json());</script>`)
		case "/settings":
			f.settings.Add(1)
			io.WriteString(w, `{"language":"en","time_zone":null,"protected":false,"always_use_https":true,"discoverable_by_email":false,"discoverable_by_mobile_phone":false,"authenticated":true}`)
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(f.server.Close)
	return f
}

type nativeManualChromePins struct {
	OwnerID    string `json:"ownerId"`
	ProfileID  string `json:"profileId"`
	ContextID  string `json:"contextId"`
	TargetID   string `json:"targetId"`
	FrameID    string `json:"frameId"`
	DocumentID string `json:"documentId"`
}
type nativeManualChromeInspection struct {
	Pins           nativeManualChromePins `json:"pins"`
	WindowCount    int                    `json:"windowCount"`
	BootstrapCount int                    `json:"bootstrapCount"`
	Paused         bool                   `json:"paused"`
	Invalid        bool                   `json:"invalid"`
	Headless       bool                   `json:"headless"`
}
type nativeManualChromeRecord struct {
	role                      string
	bridge                    *chromeFixtureBridge
	process                   *chromeFixtureProcess
	node, chrome              processFixtureIdentity
	rootID, profileID, parent string
	rootPin, profilePin       windows.Handle
	prepared                  nativePilotPreparedRole
}

func nativeManualChromeImage(identity processFixtureIdentity, expected string) error {
	actual, err := processFixtureProcessIdentity(identity.PID)
	if err != nil || actual != identity {
		return errors.New("owned-process-changed")
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, identity.PID)
	if err != nil {
		return errors.New("owned-process-unavailable")
	}
	defer windows.CloseHandle(h)
	buffer := make([]uint16, 32768)
	size := uint32(len(buffer))
	if windows.QueryFullProcessImageName(h, 0, &buffer[0], &size) != nil {
		return errors.New("owned-image-unavailable")
	}
	image, err := filepath.EvalSymlinks(windows.UTF16ToString(buffer[:size]))
	expected, expectedErr := filepath.EvalSymlinks(expected)
	if err != nil || expectedErr != nil || !samePath(image, expected) {
		return errors.New("owned-image-changed")
	}
	return nil
}
func nativeManualChromeParent(pid uint32) (uint32, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0, errors.New("owned-parent-unavailable")
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		if entry.ProcessID == pid {
			return entry.ParentProcessID, nil
		}
	}
	return 0, errors.New("owned-parent-unavailable")
}
func nativeManualChromePin(path string) (windows.Handle, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, errors.New("owned-directory-unavailable")
	}
	h, err := windows.CreateFile(name, windows.FILE_READ_ATTRIBUTES|windows.FILE_LIST_DIRECTORY, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return 0, errors.New("owned-directory-unavailable")
	}
	var info windows.ByHandleFileInformation
	if windows.GetFileInformationByHandle(h, &info) != nil || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 {
		windows.CloseHandle(h)
		return 0, errors.New("owned-directory-invalid")
	}
	return h, nil
}

func (r *nativeManualChromeRecord) osCheck() error {
	if nativeManualChromeImage(r.node, r.bridge.config.NodePath) != nil || nativeManualChromeImage(r.chrome, r.bridge.config.ChromePath) != nil {
		return errors.New("owned-image-or-creation-changed")
	}
	if r.bridge.containsPID(r.node.PID) != nil || r.bridge.containsPID(r.chrome.PID) != nil {
		return errors.New("owned-job-changed")
	}
	for _, pid := range []uint32{r.node.PID, r.chrome.PID} {
		h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
		if err != nil {
			return errors.New("owned-job-unavailable")
		}
		member, err := chromeFixtureJobMembership(h, r.bridge.job)
		windows.CloseHandle(h)
		if err != nil || !member {
			return errors.New("owned-job-changed")
		}
	}
	if parent, err := nativeManualChromeParent(r.node.PID); err != nil || parent != uint32(os.Getpid()) {
		return errors.New("owned-node-parent-changed")
	}
	if parent, err := nativeManualChromeParent(r.chrome.PID); err != nil || parent != r.node.PID {
		return errors.New("owned-chrome-parent-changed")
	}
	root, err := processFixtureDirectoryID(r.bridge.config.Root)
	profile, profileErr := processFixtureDirectoryID(filepath.Join(r.bridge.config.Root, "shared"))
	if err != nil || profileErr != nil || root != r.rootID || profile != r.profileID {
		return errors.New("owned-directory-changed")
	}
	if r.role == "collector" && len(processFixtureWindows(r.chrome.PID)) != 0 {
		return errors.New("collector-not-headless")
	}
	return nil
}
func (r *nativeManualChromeRecord) inspect(ctx context.Context) (nativePilotOwner, nativeManualChromeInspection, error) {
	var snapshot nativeManualChromeInspection
	if r.osCheck() != nil {
		return nativePilotOwner{}, snapshot, errors.New("owned-os-check-failed")
	}
	if r.bridge.rpc(ctx, "inspect", map[string]any{"owner": r.process.owner}, &snapshot) != nil || snapshot.Invalid || snapshot.Headless != (r.role == "collector") || snapshot.Pins.OwnerID != strconv.FormatUint(uint64(r.chrome.PID), 10) || snapshot.Pins.ProfileID != filepath.Join(r.bridge.config.Root, "shared") {
		return nativePilotOwner{}, snapshot, errors.New("owned-source-check-failed")
	}
	if r.role == "reader" && (snapshot.WindowCount < 1 || snapshot.BootstrapCount != 1 || len(processFixtureWindows(r.chrome.PID)) < 1) {
		return nativePilotOwner{}, snapshot, errors.New("owned-reader-inventory-failed")
	}
	p := snapshot.Pins
	o := nativePilotOwner{role: r.role, owner: p.OwnerID, profile: p.ProfileID, directory: r.profileID, image: r.bridge.config.ChromePath, creation: r.chrome.Created, job: strconv.FormatUint(uint64(r.bridge.job), 10), parent: r.parent, context: p.ContextID, target: p.TargetID, frame: p.FrameID, document: p.DocumentID, pid: int(r.chrome.PID), generation: 1}
	if !validNativePilotOwner(o, r.role) {
		return nativePilotOwner{}, snapshot, errors.New("owned-source-pins-invalid")
	}
	return o, snapshot, nil
}

func nativeManualChromeBridgeReason(err error) string {
	if err == nil {
		return "bridge-ready"
	}
	switch err.Error() {
	case "fixture configuration is incomplete":
		return "configuration-incomplete"
	case "fixture executable paths must be absolute":
		return "executable-path-invalid"
	case "fixture executable is unavailable":
		return "executable-unavailable"
	case "fixture bridge did not start":
		return "node-start-failed"
	case "fixture bridge handshake failed":
		return "node-handshake-failed"
	case "fixture bridge handshake timed out":
		return "node-handshake-timeout"
	case "fixture bridge Job Object assignment failed":
		return "node-job-assignment-failed"
	case "fixture bridge Job Object membership is unknown":
		return "node-job-membership-unknown"
	default:
		return "bridge-startup-failed"
	}
}

func nativeManualChromeRPCReason(err error) string {
	if errors.Is(err, context.Canceled) {
		return "context-canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "deadline-exceeded"
	}
	if err == nil {
		return "rpc-complete"
	}
	switch err.Error() {
	case "fixture_manual_rejected", "fixture_manual_setup", "fixture_manual_ready", "fixture_manual_identity", "fixture_manual_settings_wait", "fixture_manual_settings_projection", "fixture_manual_document_check", "fixture_manual_publish", "fixture_manual_captured", "fixture_manual_cleanup_resume", "fixture_manual_cleanup_debugger", "fixture_manual_cleanup_network", "fixture_manual_complete":
		return err.Error()
	case "fixture bridge response invalid":
		return "rpc-response-invalid"
	case "fixture bridge exited", "fixture bridge is closed":
		return "rpc-bridge-closed"
	default:
		return "rpc-failed"
	}
}

// Diagnostic replication only: the original strict decoder remains the sole
// accepting boundary. No native value, timestamp or body is logged.
func nativeManualChromeWireStage(raw []byte, ch nativePilotCaptureChallenge) string {
	if _, err := decodeNativePilotStartup(raw, ch.challenge, ch.correlation); err == nil {
		return "wire-valid"
	}
	if len(raw) == 0 || len(raw) > nativePilotStartupWireMax {
		return "wire-size"
	}
	if !nativePilotStartupUnicode(raw) {
		return "wire-unicode"
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || len(fields) != 16 {
		return "wire-envelope"
	}
	var values = make(map[string]string)
	for _, key := range []string{"schema", "challenge", "role", "accountId", "ownerId", "profileId", "contextId", "targetId", "frameId", "documentId", "identityRequestId", "settingsRequestId", "qualification"} {
		var s string
		if json.Unmarshal(fields[key], &s) != nil || !nativePilotRef(s, 512) {
			return "wire-string-shape"
		}
		values[key] = s
	}
	if values["schema"] != nativePilotStartupWireSchema || values["qualification"] != nativePilotSettingsQualification || !nativePilotAccount(values["accountId"]) {
		return "wire-source"
	}
	o := ch.challenge.owner
	for key, want := range map[string]string{"challenge": ch.correlation, "role": ch.challenge.role, "ownerId": o.owner, "profileId": o.profile, "contextId": o.context, "targetId": o.target, "frameId": o.frame, "documentId": o.document} {
		if values[key] != want {
			return "wire-scope"
		}
	}
	if values["identityRequestId"] == values["settingsRequestId"] {
		return "wire-request-replay"
	}
	numbers := make(map[string]float64)
	for _, key := range []string{"identityObservedAtMS", "settingsObservedAtMS", "elapsedMS"} {
		var n float64
		if json.Unmarshal(fields[key], &n) != nil || math.IsNaN(n) || math.IsInf(n, 0) || n < 0 {
			return "wire-time-shape"
		}
		numbers[key] = n
	}
	if numbers["elapsedMS"] >= float64(nativePilotStartup.Milliseconds()) {
		return "wire-elapsed"
	}
	convert := func(ms float64) (time.Time, bool) {
		if ms > 8640000000000000 {
			return time.Time{}, false
		}
		sec, fraction := math.Modf(ms / 1000)
		return time.Unix(int64(sec), int64(fraction*1e9)).UTC(), true
	}
	identity, ok := convert(numbers["identityObservedAtMS"])
	settings, ok2 := convert(numbers["settingsObservedAtMS"])
	if !ok || !ok2 {
		return "wire-time-range"
	}
	if identity.Before(ch.challenge.started) {
		return "wire-time-before-start"
	}
	if settings.Before(identity) {
		return "wire-time-order"
	}
	return "wire-rejected"
}

func nativeManualChromeInitialReasons(b *nativePilotBroker) (string, string) {
	b.mu.Lock()
	c := b.controller
	b.mu.Unlock()
	if c == nil {
		return "none", "none"
	}
	c.mu.Lock()
	controller := c.reason
	a := c.adapter
	c.mu.Unlock()
	a.mu.Lock()
	adapter := a.reason
	a.mu.Unlock()
	if controller == "" {
		controller = "none"
	}
	if adapter == "" {
		adapter = "none"
	}
	return controller, adapter
}

func nativeManualChromeTimeFlags(raw []byte, ch nativePilotCaptureChallenge) (beforeStart, beforeIdentity, future, elapsed bool) {
	var sample struct {
		Identity float64 `json:"identityObservedAtMS"`
		Settings float64 `json:"settingsObservedAtMS"`
		Elapsed  float64 `json:"elapsedMS"`
	}
	if json.Unmarshal(raw, &sample) != nil {
		return
	}
	for _, n := range []float64{sample.Identity, sample.Settings, sample.Elapsed} {
		if math.IsNaN(n) || math.IsInf(n, 0) || n < 0 {
			return
		}
	}
	if sample.Identity == 0 || sample.Settings == 0 || sample.Identity > 8640000000000000 || sample.Settings > 8640000000000000 {
		return
	}
	convert := func(ms float64) time.Time {
		sec, fraction := math.Modf(ms / 1000)
		return time.Unix(int64(sec), int64(fraction*1e9)).UTC()
	}
	identity, settings, now := convert(sample.Identity), convert(sample.Settings), time.Now().UTC()
	return identity.Before(ch.challenge.started), settings.Before(identity), identity.After(now) || settings.After(now), sample.Elapsed >= float64(nativePilotStartup.Milliseconds())
}

func TestNativeManualPilotChrome(t *testing.T) {
	if os.Getenv("AKU_NATIVE_MANUAL_CHROME_ENABLED") != "1" {
		t.Skip("synthetic Windows ownership requires its explicit helper")
	}
	project := os.Getenv("AKU_NATIVE_MANUAL_PROJECT_ROOT")
	parentRoot := os.Getenv("AKU_CHROME_FIXTURE_ROOT")
	marker := os.Getenv("AKU_CHROME_FIXTURE_CLEANUP_MARKER")
	if !filepath.IsAbs(project) || !filepath.IsAbs(parentRoot) || !samePath(filepath.Dir(marker), parentRoot) {
		t.Fatal("fixture paths unavailable")
	}
	nodePath, chromePath := os.Getenv("AKU_CHROME_FIXTURE_NODE"), os.Getenv("AKU_CHROME_FIXTURE_EXE")
	driverPath := filepath.Join(project, "scripts", "auth-session-fixture", "chrome-manual-pilot-driver.mjs")
	for _, executable := range []string{nodePath, chromePath, driverPath} {
		info, statErr := os.Stat(executable)
		if !filepath.IsAbs(executable) || statErr != nil || !info.Mode().IsRegular() {
			t.Fatal("fixture executable preflight failed")
		}
	}
	prior, err := os.Getwd()
	if err != nil || os.Chdir(filepath.Join(project, "internal", "engine")) != nil {
		t.Fatal("fixture workspace unavailable")
	}
	defer os.Chdir(prior)
	var registryMu sync.Mutex
	var registry []*nativeManualChromeRecord
	t.Cleanup(func() {
		registryMu.Lock()
		defer registryMu.Unlock()
		if len(registry) != 4 {
			t.Error("owned role registration incomplete")
			return
		}
		for _, r := range registry {
			if r.bridge.cleanupError() != nil || !r.bridge.cleanupVerified() {
				t.Error("owned Job cleanup unverified")
				return
			}
		}
		if os.WriteFile(marker, []byte("verified\n"), 0600) != nil {
			t.Error("owned cleanup marker unavailable")
		}
	})
	for _, scenario := range []string{"ownership-result-last-window", "startup-context-drift"} {
		t.Run(scenario, func(t *testing.T) {
			fixture := startNativeManualChromeHTTP(t)
			var random [12]byte
			if _, err := rand.Read(random[:]); err != nil {
				t.Fatal("fixture scope unavailable")
			}
			nonce := hex.EncodeToString(random[:])
			root := filepath.Join(parentRoot, "run-manual-"+nonce)
			if os.Mkdir(root, 0700) != nil {
				t.Fatal("fresh role scope unavailable")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 55*time.Second)
			defer cancel()
			started := time.Now()
			parentIdentity, err := processFixtureProcessIdentity(uint32(os.Getpid()))
			if err != nil {
				t.Fatal("actual test parent identity unavailable")
			}
			parentID := strconv.FormatUint(uint64(parentIdentity.PID), 10) + "/" + parentIdentity.Created
			records := map[string]*nativeManualChromeRecord{}
			for _, role := range []string{"reader", "collector"} {
				roleRoot := filepath.Join(root, "run-"+role)
				if os.Mkdir(roleRoot, 0700) != nil {
					t.Fatal("fresh profile root unavailable")
				}
				cfg := chromeFixtureConfig{NodePath: nodePath, ChromePath: chromePath, Root: roleRoot, Origin: fixture.server.URL, DriverPath: driverPath, NativeReaderStartup: role == "reader"}
				bridge, err := startChromeFixtureBridge(ctx, cfg)
				if err != nil {
					t.Fatal("owned Node bridge unavailable: " + nativeManualChromeBridgeReason(err))
				}
				r := &nativeManualChromeRecord{role: role, bridge: bridge, parent: parentID}
				records[role] = r
				registryMu.Lock()
				registry = append(registry, r)
				registryMu.Unlock()
				t.Cleanup(func() {
					bridge.terminateOwnedTree()
					select {
					case <-bridge.nodeWaited:
					case <-time.After(5 * time.Second):
						t.Error("owned Node exit unverified")
					}
					if r.process != nil {
						select {
						case <-r.process.Done():
						case <-time.After(5 * time.Second):
							t.Error("owned Chrome root exit unverified")
						}
					}
					if bridge.cleanupError() != nil || !bridge.cleanupVerified() {
						t.Error("owned Job did not become empty")
					}
					if r.profilePin != 0 {
						windows.CloseHandle(r.profilePin)
					}
					if r.rootPin != 0 {
						windows.CloseHandle(r.rootPin)
					}
				})
				if bridge.assertBridgeOnly(ctx) != nil {
					t.Fatal("Chrome existed before assigned Node Job")
				}
				r.node, err = processFixtureProcessIdentity(bridge.pid)
				if err != nil {
					t.Fatal("actual Node creation identity unavailable")
				}
				launchRole := "owner"
				if role == "collector" {
					launchRole = "collector"
				}
				r.process, err = bridge.launch(ctx, launchRole)
				if err != nil {
					t.Fatal("fresh owned Chrome launch failed")
				}
				r.chrome, err = processFixtureProcessIdentity(r.process.pid)
				if err != nil {
					t.Fatal("actual Chrome creation identity unavailable")
				}
				r.rootID, err = processFixtureDirectoryID(roleRoot)
				if err != nil {
					t.Fatal("physical role directory unavailable")
				}
				profile := filepath.Join(roleRoot, "shared")
				r.profileID, err = processFixtureDirectoryID(profile)
				if err != nil {
					t.Fatal("physical profile identity unavailable")
				}
				r.rootPin, err = nativeManualChromePin(roleRoot)
				if err != nil {
					t.Fatal("physical role pin unavailable")
				}
				r.profilePin, err = nativeManualChromePin(profile)
				if err != nil {
					t.Fatal("physical profile pin unavailable")
				}
				if r.osCheck() != nil {
					t.Fatal("actual process/profile/Job containment failed")
				}
				if role == "reader" {
					if r.process.Login(ctx) != nil {
						t.Fatal("synthetic loopback login failed")
					}
				} else {
					fixture.mu.Lock()
					value := fixture.token
					fixture.mu.Unlock()
					var seed struct {
						Seeded bool `json:"seeded"`
					}
					if bridge.rpc(ctx, "seed_fixture", map[string]any{"owner": r.process.owner, "value": value}, &seed) != nil || !seed.Seeded {
						t.Fatal("synthetic one-cookie seed failed")
					}
				}
			}
			if records["reader"].chrome.PID == records["collector"].chrome.PID || records["reader"].profileID == records["collector"].profileID || records["reader"].rootID == records["collector"].rootID {
				t.Fatal("actual owners or physical profiles overlap")
			}
			binding := nativePilotBinding{account: nativeManualChromeAccount, enrollment: "offline-explicit-enrollment", pair: "offline-pair-" + nonce, attempt: "offline-attempt-" + nonce, derivation: "offline-once-seed-" + nonce, policy: 1, consent: 1, reader: nativePilotOwner{role: "reader", profile: filepath.Join(records["reader"].bridge.config.Root, "shared"), parent: parentID}, collector: nativePilotOwner{role: "collector", profile: filepath.Join(records["collector"].bridge.config.Root, "shared"), parent: parentID}}
			var preparedCount, acquireCount atomic.Int32
			var driftApplied atomic.Bool
			var broker *nativePilotBroker
			var d nativePilotDiagnostic
			ports := nativePilotBrokerPorts{now: time.Now,
				prepare: func(call context.Context, req nativePilotPrepareRequest) (nativePilotPreparedRole, error) {
					r := records[req.role]
					var ready struct {
						Handle string                 `json:"handle"`
						Pins   nativeManualChromePins `json:"pins"`
					}
					if r.bridge.rpc(call, "prepare", map[string]any{"owner": r.process.owner}, &ready) != nil {
						return nativePilotPreparedRole{}, errors.New("chrome-prepare-failed")
					}
					o, snapshot, err := r.inspect(call)
					if err != nil || !snapshot.Paused || ready.Pins != snapshot.Pins {
						return nativePilotPreparedRole{}, errors.New("chrome-ready-pins-invalid")
					}
					p := nativePilotPreparedRole{owner: o, handle: ready.Handle}
					r.prepared = p
					preparedCount.Add(1)
					return p, nil
				},
				inspect: func(call context.Context, p nativePilotPreparedRole) (nativePilotOwner, error) {
					o, _, err := records[p.owner.role].inspect(call)
					return o, err
				},
				acquire: func(call context.Context, p nativePilotPreparedRole, ch nativePilotCaptureChallenge) ([]byte, error) {
					if preparedCount.Load() != 2 {
						return nil, errors.New("chrome-pair-not-ready")
					}
					r := records[p.owner.role]
					if r.prepared != p {
						return nil, errors.New("chrome-ready-handle-changed")
					}
					if scenario == "startup-context-drift" && p.owner.role == "reader" {
						var drift struct {
							Changed bool `json:"changed"`
						}
						if r.bridge.rpc(call, "context_drift", map[string]any{"owner": r.process.owner}, &drift) != nil || !drift.Changed {
							return nil, errors.New("chrome-drift-unavailable")
						}
						driftApplied.Store(true)
					}
					request, err := ch.requestJSON()
					if err != nil {
						return nil, err
					}
					defer clear(request)
					var raw json.RawMessage
					acquireCount.Add(1)
					if err := r.bridge.rpc(call, "acquire", map[string]any{"owner": r.process.owner, "handle": p.handle, "request": json.RawMessage(request)}, &raw); err != nil {
						t.Logf("acquire role=%s stage=%s", p.owner.role, nativeManualChromeRPCReason(err))
						return nil, errors.New("chrome-acquire-failed")
					}
					defer clear(raw)
					t.Logf("acquire role=%s stage=%s", p.owner.role, nativeManualChromeWireStage(raw, ch))
					_, decodeErr := decodeNativePilotStartup(raw, ch.challenge, ch.correlation)
					beforeStart, beforeIdentity, future, elapsed := nativeManualChromeTimeFlags(raw, ch)
					t.Logf("wire role=%s decode_accepted=%t identity_predates_challenge=%t settings_predates_identity=%t future_timestamp=%t elapsed_exceeded=%t", p.owner.role, decodeErr == nil, beforeStart, beforeIdentity, future, elapsed)
					return bytes.Clone(raw), nil
				},
				state: func(call context.Context) (nativePilotBrokerState, error) {
					r := records["reader"]
					count := len(processFixtureWindows(r.chrome.PID))
					return nativePilotBrokerState{account: binding.account, enrollment: binding.enrollment, pair: binding.pair, attempt: binding.attempt, derivation: binding.derivation, policy: binding.policy, consent: binding.consent, readerWindows: count, completeInventory: true, parentAlive: call.Err() == nil, channelHealthy: !r.bridge.isClosed(), invalidationsReady: true, optedIn: true}, nil
				},
				abort: func(call context.Context, p nativePilotPreparedRole, preserve bool) error {
					if broker != nil {
						controller, adapter := nativeManualChromeInitialReasons(broker)
						t.Logf("pre-cleanup controller=%s adapter=%s", controller, adapter)
					}
					r := records[p.owner.role]
					var reply struct {
						Closed bool `json:"closed"`
					}
					if r.bridge.rpc(call, "abort", map[string]any{"owner": r.process.owner, "handle": p.handle, "preserveReader": preserve}, &reply) != nil || !reply.Closed {
						return errors.New("chrome-abort-failed")
					}
					return nil
				},
			}
			broker, d = newNativePilotBroker(ctx, nativePilotBrokerConfig{enabled: true, offlineTest: true, pilot: nativePilotConfig{contract: nativePilotContract, enabled: true, limitationAcknowledged: true, binding: binding, attemptStarted: started}}, ports)
			if broker == nil {
				t.Fatal("private offline Chrome broker unavailable")
			}
			if d := broker.openReader(ctx, func() bool { t.Fatal("reader admitted before evidence"); return true }); d.Reason != "admission-closed" {
				t.Fatal("initial reader admission was not closed")
			}
			d = broker.start(ctx)
			if !d.OfflineValidated {
				t.Logf("initial reason=%s prepared=%d acquired=%d logins=%d settings=%d", d.Reason, preparedCount.Load(), acquireCount.Load(), fixture.logins.Load(), fixture.settings.Load())
			}
			if d.CanAuthorize || d.NativeQualified || d.StorePermit || d.ManualEligible {
				t.Fatal("real Chrome changed offline authority")
			}
			if scenario == "startup-context-drift" {
				if !driftApplied.Load() || d.OfflineValidated || broker.controller != nil && broker.controller.vector != nil {
					t.Fatal("changed actual context admitted")
				}
				t.Log("actual startup context loss rejected; offline authority remains closed")
				return
			}
			if !d.OfflineValidated || d.Reason != "manual-attempt-valid" || preparedCount.Load() != 2 || acquireCount.Load() != 2 || fixture.logins.Load() != 1 || fixture.settings.Load() != 2 {
				t.Fatal("strict initial two-role loopback evidence failed")
			}
			ticket, d := broker.dispatch(ctx, "owned-capture")
			if ticket == nil || !d.OfflineValidated {
				t.Fatal("offline work ticket missing")
			}
			collector := ticket.collector
			if d := broker.claim(ctx, ticket, func() bool { return true }); d.Reason != "work-claimed" {
				t.Fatal("offline claim failed")
			}
			if d := broker.openReader(ctx, func() bool { return records["reader"].process.OpenFixtureIndependentWindow(ctx) == nil }); d.Reason != "reader-admitted" {
				t.Fatal("owned independent reader failed")
			}
			if _, snapshot, err := records["reader"].inspect(ctx); err != nil || snapshot.WindowCount != 2 || snapshot.BootstrapCount != 1 {
				t.Fatal("reader/bootstrap inventory not preserved")
			}
			if o, _, err := records["collector"].inspect(ctx); err != nil || o != collector {
				t.Fatal("opening reader changed collector PID/generation/document/profile")
			}
			results := 0
			if d := broker.complete(ctx, ticket, func() bool { results++; return true }); d.Reason != "result-completed" {
				t.Fatal("offline result failed")
			}
			if d := broker.complete(ctx, ticket, func() bool { results++; return true }); d.Reason != "result-duplicate" || results != 1 {
				t.Fatal("offline result committed more than once")
			}
			reader := records["reader"]
			for _, index := range []int{1, 0} {
				var closed struct {
					Requested, Exited bool
					Remaining         int
					RootExited        bool
				}
				if reader.bridge.rpc(ctx, "close_window", map[string]any{"owner": reader.process.owner, "index": index}, &closed) != nil {
					t.Fatal("owned reader close failed")
				}
				if index == 1 && closed.Remaining != 1 {
					t.Fatal("bootstrap window was not retained")
				}
			}
			if processFixtureWait(ctx, func() bool { return len(processFixtureWindows(reader.chrome.PID)) == 0 }) != nil {
				t.Fatal("last reader window remained")
			}
			if d := broker.check(ctx); d.OfflineValidated || d.ManualEligible {
				t.Fatal("last-reader closure preserved admission")
			}
			t.Log("actual OS/Job/profile ownership and paused-document binding verified; collector preserved; one result; last-reader closure fenced offline admission")
		})
	}
}
