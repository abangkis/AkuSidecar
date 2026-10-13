//go:build windows && auth_session_fixture

package engine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/collection"
	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
	"github.com/abangkis/AkuSidecar/internal/store"
)

var chromeFixtureRegistryMu sync.Mutex
var chromeFixtureRegistry []*chromeFixtureBridge

func TestMain(m *testing.M) {
	code := m.Run()
	marker := os.Getenv("AKU_CHROME_FIXTURE_CLEANUP_MARKER")
	root := os.Getenv("AKU_CHROME_FIXTURE_ROOT")
	if marker != "" && root != "" && allChromeFixtureBridgesReleased() && samePath(filepath.Dir(marker), root) {
		if _, err := os.Lstat(marker); errors.Is(err, os.ErrNotExist) {
			_ = os.WriteFile(marker, []byte("verified\n"), 0600)
		}
	}
	os.Exit(code)
}

func allChromeFixtureBridgesReleased() bool {
	chromeFixtureRegistryMu.Lock()
	defer chromeFixtureRegistryMu.Unlock()
	for _, bridge := range chromeFixtureRegistry {
		if bridge.cleanupError() != nil || !bridge.cleanupVerified() {
			return false
		}
	}
	return len(chromeFixtureRegistry) > 0
}

func registerChromeFixtureBridge(bridge *chromeFixtureBridge) {
	chromeFixtureRegistryMu.Lock()
	defer chromeFixtureRegistryMu.Unlock()
	chromeFixtureRegistry = append(chromeFixtureRegistry, bridge)
}

type chromeFixtureHTTP struct {
	server    *httptest.Server
	logins    atomic.Int64
	revision  atomic.Int64
	sequence  atomic.Int64
	sessions  sync.Map
	authority sync.RWMutex
	active    SyntheticSessionEvidence
}

func startChromeFixtureHTTP(t *testing.T) *chromeFixtureHTTP {
	t.Helper()
	fixture := &chromeFixtureHTTP{}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal("loopback fixture listener unavailable")
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(fixture.serveHTTP))
	server.Listener = listener
	server.Start()
	fixture.server = server
	return fixture
}

func (f *chromeFixtureHTTP) serveHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'")
	w.Header().Set("Content-Type", "application/json")
	if r.URL.Path == "/" {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<!doctype html><title>Local fixture</title><h1>Local fixture</h1>"))
		return
	}
	if r.URL.Path == "/login" && r.Method == http.MethodPost {
		account := r.URL.Query().Get("account")
		if account != "alpha" && account != "beta" {
			http.Error(w, "invalid fixture account", http.StatusBadRequest)
			return
		}
		var tokenBytes [32]byte
		if _, err := rand.Read(tokenBytes[:]); err != nil {
			http.Error(w, "fixture login failed", http.StatusInternalServerError)
			return
		}
		token := hex.EncodeToString(tokenBytes[:])
		revision := f.revision.Add(1)
		f.sessions.Store(token, chromeFixtureSession{account: account, revision: revision})
		f.logins.Add(1)
		f.authority.Lock()
		f.active = SyntheticSessionEvidence{Authenticated: true, AccountID: account, Revision: "fixture-" + strconv.FormatInt(revision, 10)}
		f.authority.Unlock()
		http.SetCookie(w, &http.Cookie{Name: "fixture_session", Value: token, Path: "/", HttpOnly: true,
			Secure: true, SameSite: http.SameSiteStrictMode, MaxAge: 3600})
		_ = json.NewEncoder(w).Encode(map[string]any{"account": account, "epoch": revision})
		return
	}
	token := ""
	if cookie, err := r.Cookie("fixture_session"); err == nil {
		token = cookie.Value
	}
	value, ok := f.sessions.Load(token)
	if !ok {
		if r.URL.Path == "/logout" && r.Method == http.MethodPost {
			f.revision.Add(1)
			http.SetCookie(w, &http.Cookie{Name: "fixture_session", Value: "", Path: "/", HttpOnly: true,
				Secure: true, SameSite: http.SameSiteStrictMode, MaxAge: -1})
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{"state": "logged_out"})
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{"state": "needs_login"})
		return
	}
	session := value.(chromeFixtureSession)
	switch {
	case r.URL.Path == "/probe":
		_ = json.NewEncoder(w).Encode(map[string]any{"account": session.account, "epoch": session.revision})
	case r.URL.Path == "/logout" && r.Method == http.MethodPost:
		f.sessions.Delete(token)
		f.revision.Add(1)
		f.authority.Lock()
		f.active = SyntheticSessionEvidence{}
		f.authority.Unlock()
		http.SetCookie(w, &http.Cookie{Name: "fixture_session", Value: "", Path: "/", HttpOnly: true,
			Secure: true, SameSite: http.SameSiteStrictMode, MaxAge: -1})
		_ = json.NewEncoder(w).Encode(map[string]any{"state": "logged_out"})
	case r.URL.Path == "/collect" && r.Method == http.MethodPost:
		id := f.sequence.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{"account": session.account, "epoch": session.revision,
			"postId": "fixture-post-" + strconv.FormatInt(id, 10)})
	default:
		http.NotFound(w, r)
	}
}

func (f *chromeFixtureHTTP) AuthorityEvidence(context.Context) (SyntheticSessionEvidence, error) {
	f.authority.RLock()
	defer f.authority.RUnlock()
	return f.active, nil
}

type chromeFixtureSession struct {
	account  string
	revision int64
}

func chromeFixtureConfigFromEnv(t *testing.T, origin string) chromeFixtureConfig {
	t.Helper()
	root := os.Getenv("AKU_CHROME_FIXTURE_ROOT")
	if root == "" {
		t.Skip("Chrome fixture requires the explicit VerifyChrome helper")
	}
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("fixture source location unavailable")
	}
	project := filepath.Clean(filepath.Join(filepath.Dir(source), "..", ".."))
	var random [6]byte
	if _, err := rand.Read(random[:]); err != nil {
		t.Fatal("fixture case id unavailable")
	}
	var suffix strings.Builder
	for _, value := range strings.ToLower(t.Name()) {
		if value >= 'a' && value <= 'z' || value >= '0' && value <= '9' || value == '-' {
			suffix.WriteRune(value)
		} else if suffix.Len() == 0 || !strings.HasSuffix(suffix.String(), "-") {
			suffix.WriteByte('-')
		}
	}
	caseRoot := filepath.Join(root, "run-"+strings.Trim(suffix.String(), "-")+"-"+hex.EncodeToString(random[:]))
	if err := os.Mkdir(caseRoot, 0700); err != nil {
		t.Fatal("fixture case root could not be created")
	}
	return chromeFixtureConfig{
		NodePath: os.Getenv("AKU_TEST_NODE_PATH"), ChromePath: os.Getenv("AKU_TEST_CHROME_PATH"), Root: caseRoot,
		Origin: origin, DriverPath: filepath.Join(project, "scripts", "auth-session-fixture", "chrome-engine-driver.mjs"),
	}
}

func TestChromeFixtureSharedProfileSmoke(t *testing.T) {
	root := os.Getenv("AKU_CHROME_FIXTURE_ROOT")
	if root == "" {
		t.Skip("Chrome fixture requires the explicit VerifyChrome helper")
	}
	fixture := startChromeFixtureHTTP(t)
	defer fixture.server.Close()
	config := chromeFixtureConfigFromEnv(t, fixture.server.URL)
	remoteOrigin := config
	remoteOrigin.Origin = "https://example.com"
	if _, err := startChromeFixtureBridge(context.Background(), remoteOrigin); err == nil {
		t.Fatal("fixture adapter accepted a non-loopback origin")
	}
	if err := os.Mkdir(filepath.Join(config.Root, "shared"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := startChromeFixtureBridge(context.Background(), config); err == nil {
		t.Fatal("fixture adapter accepted a pre-existing profile")
	}
	if err := os.Remove(filepath.Join(config.Root, "shared")); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	bridge, err := startChromeFixtureBridge(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	registerChromeFixtureBridge(bridge)
	t.Cleanup(func() {
		bridge.terminateOwnedTree()
		if err := bridge.cleanupError(); err != nil || !bridge.cleanupVerified() {
			t.Errorf("owned fixture tree cleanup was not verified: %v", err)
		}
	})

	collector, err := bridge.launch(ctx, "collector")
	if err != nil {
		t.Fatal(err)
	}
	if err := collector.Login(ctx); err != nil {
		t.Fatal(err)
	}
	initial, err := collector.Evidence(ctx)
	if err != nil || !initial.Authenticated || initial.AccountID != "alpha" || initial.Revision == "" {
		t.Fatalf("initial synthetic login evidence invalid: %v", err)
	}
	if fixture.logins.Load() != 1 {
		t.Fatal("synthetic login count was not one")
	}
	if err = collector.CloseForRetry(ctx); err != nil {
		t.Fatal(err)
	}
	owner, err := bridge.launch(ctx, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if owner.PID() == collector.PID() {
		t.Fatal("headed reader reused the collector PID")
	}
	evidence, err := owner.Evidence(ctx)
	if err != nil || !evidence.Authenticated || evidence.AccountID != initial.AccountID || evidence.Revision != initial.Revision {
		t.Fatalf("same-profile reader authentication did not persist: %v", err)
	}
	windows, bootstrap, err := owner.FixtureInventory(ctx)
	if err != nil || windows != 2 || bootstrap != 1 {
		t.Fatalf("headed browser inventory did not include the bootstrap window: windows=%d bootstrap=%d err=%v", windows, bootstrap, err)
	}
	if err = owner.CloseFixtureWindow(ctx, 1); err != nil {
		t.Fatal(err)
	}
	windows, _, err = owner.FixtureInventory(ctx)
	if err != nil || windows != 1 {
		t.Fatalf("one remaining bootstrap/reader window was not retained: windows=%d err=%v", windows, err)
	}
	select {
	case <-owner.done:
		t.Fatal("browser exited while one fixture window remained")
	case <-time.After(150 * time.Millisecond):
	}
	if err = owner.CloseFixtureWindow(ctx, 0); err != nil {
		t.Fatal(err)
	}
	select {
	case <-owner.done:
	case <-ctx.Done():
		t.Fatal("natural final-window exit was not observed after process-tree release")
	}
	if err = owner.CloseForRetry(ctx); err != nil {
		t.Fatal(err)
	}
	resumed, err := bridge.launch(ctx, "collector")
	if err != nil {
		t.Fatal(err)
	}
	if resumed.PID() == owner.PID() {
		t.Fatal("collector resume reused the headed reader PID")
	}
	evidence, err = resumed.Evidence(ctx)
	if err != nil || !evidence.Authenticated || evidence.AccountID != "alpha" || evidence.Revision != initial.Revision {
		t.Fatalf("collector did not reopen the same authenticated synthetic account: %v", err)
	}
	if _, err := owner.Evidence(ctx); err == nil {
		t.Fatal("stale headed owner remained able to read the fixture session")
	}
	if fixture.logins.Load() != 1 {
		t.Fatal("resume performed a second synthetic login")
	}
}

type chromeFixtureCaptureBackend struct {
	bridge       *chromeFixtureBridge
	current      atomic.Pointer[chromeFixtureProcess]
	entered      chan struct{}
	release      chan struct{}
	blockFirst   atomic.Bool
	calls        atomic.Int64
	payloadMu    sync.Mutex
	firstPayload map[string]any
}

func newChromeFixtureCaptureBackend(bridge *chromeFixtureBridge, blockFirst bool) *chromeFixtureCaptureBackend {
	backend := &chromeFixtureCaptureBackend{bridge: bridge}
	if blockFirst {
		backend.entered = make(chan struct{})
		backend.release = make(chan struct{})
		backend.blockFirst.Store(true)
	}
	return backend
}

func (b *chromeFixtureCaptureBackend) Capture(ctx context.Context, source domain.Source, payload map[string]any) (domain.Observation, error) {
	call := b.calls.Add(1)
	process := b.current.Load()
	if process == nil {
		return domain.Observation{}, errors.New("fixture Chrome owner is unavailable")
	}
	if err := b.bridge.collect(ctx, process.owner); err != nil {
		return domain.Observation{}, err
	}
	if call == 1 {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return domain.Observation{}, err
		}
		var copied map[string]any
		if err = json.Unmarshal(encoded, &copied); err != nil {
			return domain.Observation{}, err
		}
		b.payloadMu.Lock()
		b.firstPayload = copied
		b.payloadMu.Unlock()
	}
	if b.blockFirst.CompareAndSwap(true, false) {
		close(b.entered)
		select {
		case <-b.release:
		case <-ctx.Done():
			return domain.Observation{}, ctx.Err()
		}
	}
	return syntheticObservation(source), nil
}

func (b *chromeFixtureCaptureBackend) capturedPayload() map[string]any {
	b.payloadMu.Lock()
	defer b.payloadMu.Unlock()
	return b.firstPayload
}

type chromeFixtureEngineHarness struct {
	bridge      *chromeFixtureBridge
	fixture     *chromeFixtureHTTP
	backend     *chromeFixtureCaptureBackend
	manager     *captureruntime.Manager
	engine      *Engine
	state       *store.Store
	coordinator *collection.Coordinator
	ctx         context.Context
	cancel      context.CancelFunc
	dbPath      string
}

func newChromeFixtureEngineHarness(t *testing.T, fixtureConfig chromeFixtureConfig, bridge *chromeFixtureBridge,
	fixture *chromeFixtureHTTP, backend *chromeFixtureCaptureBackend, existing *captureruntime.Manager, dbPath string) *chromeFixtureEngineHarness {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	settings := domain.DefaultSettings("expanded", "quiet", "guarded_live", true)
	settings.CollectionMode = "headless"
	settings.ActiveSources = []domain.Source{domain.SourceX, domain.SourceInstagram}
	settings.CalibrationEnabled = false
	if dbPath == "" {
		dbPath = filepath.Join(fixtureConfig.Root, "fixture.sqlite")
	}
	state, err := store.Open(dbPath, settings)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	if existing == nil {
		if _, err = state.CompleteOnboarding(ctx, settings.ActiveSources); err != nil {
			_ = state.Close()
			cancel()
			t.Fatal(err)
		}
	}
	h := &chromeFixtureEngineHarness{bridge: bridge, fixture: fixture, backend: backend, state: state, ctx: ctx, cancel: cancel, dbPath: dbPath}
	if existing == nil {
		initial, launchErr := bridge.launch(ctx, "collector")
		if launchErr != nil {
			_ = state.Close()
			cancel()
			t.Fatal(launchErr)
		}
		backend.current.Store(initial)
		if err = initial.Login(ctx); err != nil {
			_ = state.Close()
			cancel()
			t.Fatal(err)
		}
		manager, managerErr := captureruntime.New(initial)
		if managerErr != nil {
			_ = state.Close()
			cancel()
			t.Fatal(managerErr)
		}
		h.manager = manager
		t.Cleanup(manager.Terminate)
	} else {
		h.manager = existing
		if backend.current.Load() == nil {
			_ = state.Close()
			cancel()
			t.Fatal("restarted fixture has no tracked Chrome owner")
		}
	}
	h.engine = New(state, reasoning.Deterministic{}, config.Config{Dev: true, Root: fixtureConfig.Root,
		Database: config.DatabaseConfig{Path: dbPath}, Capture: config.CaptureConfig{MaxAcquisitionRounds: 1}},
		log.New(io.Discard, "", 0))
	h.engine.RecordHeartbeat(ExpectedHeartbeat())
	if err = h.engine.AttachCaptureRuntime(ctx, h.manager); err != nil {
		h.stopEngineStore()
		t.Fatal(err)
	}
	var coordinator *collection.Coordinator
	coordinator = collection.NewCoordinator(h.manager, func(launchCtx context.Context, mode string, generation uint64) (captureruntime.Process, error) {
		role := ""
		if mode == "native_reader" {
			role = "owner"
		} else if mode == "headless" {
			role = "collector"
		} else {
			return nil, errors.New("fixture rejected an unexpected Chrome role")
		}
		process, launchErr := bridge.launch(launchCtx, role)
		if launchErr != nil {
			return nil, launchErr
		}
		backend.current.Store(process)
		if mode == "headless" {
			coordinator.SetSyntheticHeadlessCollector(generation, backend)
		}
		return process, nil
	}, func() error { return nil })
	coordinator.SetSyntheticHeadlessCollector(h.manager.Snapshot().Generation, backend)
	h.coordinator = coordinator
	h.engine.AttachCollectionCoordinator(coordinator)
	browserProbe := func(probeCtx context.Context) (SyntheticSessionEvidence, error) {
		process := backend.current.Load()
		if process == nil {
			return SyntheticSessionEvidence{}, errors.New("fixture Chrome owner is unavailable")
		}
		return process.Evidence(probeCtx)
	}
	if err = h.engine.BindSyntheticExclusiveWithBrowserProbe("alpha", fixture.AuthorityEvidence, browserProbe); err != nil {
		h.stopEngineStore()
		t.Fatal(err)
	}
	coordinator.Start(ctx)
	coordinator.Request("headless")
	if existing == nil {
		waitChromeFixture(t, func() bool {
			status := coordinator.Status()
			return status.Effective == "headless" && !status.BatchPaused
		}, "initial synthetic browser proof and collector admission")
	} else {
		waitChromeFixture(t, func() bool {
			status := coordinator.Status()
			return status.Effective == "browser" && status.BatchPaused
		}, "Engine restart retains the live browser owner and paused collector")
	}
	t.Cleanup(h.stopEngineStore)
	return h
}

func (h *chromeFixtureEngineHarness) stopEngineStore() {
	if h == nil {
		return
	}
	if h.cancel != nil {
		h.cancel()
		h.cancel = nil
	}
	if h.engine != nil {
		h.engine.Shutdown()
		h.engine = nil
	}
	if h.state != nil {
		_ = h.state.Close()
		h.state = nil
	}
}

func startChromeFixtureForTest(t *testing.T, fixture *chromeFixtureHTTP) (chromeFixtureConfig, *chromeFixtureBridge) {
	t.Helper()
	config := chromeFixtureConfigFromEnv(t, fixture.server.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	bridge, err := startChromeFixtureBridge(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	registerChromeFixtureBridge(bridge)
	t.Cleanup(func() {
		bridge.terminateOwnedTree()
		if err := bridge.cleanupError(); err != nil || !bridge.cleanupVerified() {
			t.Errorf("fixture Job Object cleanup was not verified: %v", err)
		}
	})
	return config, bridge
}

func waitChromeFixture(t *testing.T, predicate func() bool, label string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for !predicate() {
		if time.Now().After(deadline) {
			t.Fatal("timed out: " + label)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func closeChromeFixtureReaderWindow(t *testing.T, ctx context.Context, owner *chromeFixtureProcess) {
	t.Helper()
	flags, err := owner.FixtureBootstrapWindows(ctx)
	if err != nil {
		t.Fatalf("inventory reader windows before close: %v", err)
	}
	priorCount := len(flags)
	for i := len(flags) - 1; i >= 0; i-- {
		if !flags[i] {
			if err = owner.CloseFixtureWindow(ctx, i); err != nil {
				t.Fatalf("close tracked reader window: %v", err)
			}
			waitChromeFixture(t, func() bool {
				select {
				case <-owner.done:
					return true
				default:
				}
				updated, inventoryErr := owner.FixtureBootstrapWindows(ctx)
				return inventoryErr == nil && len(updated) < priorCount
			}, "closed fixture reader target disappears from confirmed inventory")
			return
		}
	}
	t.Fatal("fixture inventory did not contain a reader window")
}

func closeChromeFixtureWindowsNaturally(t *testing.T, ctx context.Context, owner *chromeFixtureProcess) {
	t.Helper()
	flags, err := owner.FixtureBootstrapWindows(ctx)
	if err != nil || len(flags) < 2 || countBootstrapWindows(flags) != 1 {
		t.Fatalf("expected tracked reader and bootstrap windows before user close (inventory stage): %v %v", flags, err)
	}
	closeChromeFixtureReaderWindow(t, ctx, owner)
	flags, err = owner.FixtureBootstrapWindows(ctx)
	if err != nil || len(flags) != 1 || !flags[0] {
		t.Fatalf("expected only the tracked bootstrap after reader close (post-reader inventory stage): %v %v", flags, err)
	}
	if err = owner.CloseFixtureWindow(ctx, 0); err != nil {
		t.Fatalf("close final bootstrap window: %v", err)
	}
	select {
	case <-owner.done:
	case <-ctx.Done():
		t.Fatal("natural final-window close did not complete the owned Chrome tree")
	}
	if !owner.rootExited.Load() || !owner.exited.Load() {
		t.Fatal("final-window close lacked separate Chrome root-exit and Job-tree release evidence")
	}
	if err = owner.bridge.assertBridgeOnly(ctx); err != nil {
		t.Fatalf("final-window close left Chrome descendants in the owned Job Object: %v", err)
	}
}

func hasFixtureSessionID(ids []string, expected string) bool {
	for _, id := range ids {
		if id == expected {
			return true
		}
	}
	return false
}

func TestChromeFixtureEngineHandoffDurableRestartAndWindows(t *testing.T) {
	if os.Getenv("AKU_CHROME_FIXTURE_ROOT") == "" {
		t.Skip("Chrome fixture requires the explicit VerifyChrome helper")
	}
	fixture := startChromeFixtureHTTP(t)
	defer fixture.server.Close()
	config, bridge := startChromeFixtureForTest(t, fixture)
	backend := newChromeFixtureCaptureBackend(bridge, true)
	h := newChromeFixtureEngineHarness(t, config, bridge, fixture, backend, nil, "")
	testCtx, cancelTest := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancelTest()
	session, err := h.engine.StartVisibleUpdate(testCtx, "real Chrome fixture handoff")
	if err != nil {
		t.Fatal(err)
	}
	if len(session.Runs) != 2 {
		t.Fatalf("fixture continuation plan has %d runs, want 2", len(session.Runs))
	}
	h.engine.StartHeadlessCollection(h.ctx)
	select {
	case <-backend.entered:
	case <-testCtx.Done():
		t.Fatal("real authenticated Chrome collector did not enter its bounded capture")
	}
	collector := backend.current.Load()
	if collector == nil || h.manager.Snapshot().Driver != "headless" || h.manager.PID() != collector.PID() {
		t.Fatal("active Chrome capture did not retain its headless owner")
	}
	type readerResult struct {
		lease   *captureruntime.Lease
		release func()
		err     error
	}
	readerReady := make(chan readerResult, 1)
	releaseReader := make(chan struct{})
	var releaseReaderOnce sync.Once
	releaseReaderLease := func() { releaseReaderOnce.Do(func() { close(releaseReader) }) }
	t.Cleanup(releaseReaderLease)
	go func() {
		leaseCtx, cancel := context.WithTimeout(h.ctx, 120*time.Second)
		defer cancel()
		lease, release, borrowErr := h.engine.BorrowNativeReader(leaseCtx)
		readerReady <- readerResult{lease: lease, release: release, err: borrowErr}
		if borrowErr != nil {
			return
		}
		select {
		case <-releaseReader:
			lease.Release()
			release()
		case <-h.ctx.Done():
			lease.Release()
			release()
		}
	}()
	waitChromeFixture(t, func() bool { return h.coordinator.Status().BatchPaused }, "reader fences collector admission while capture drains")
	if collector.exited.Load() || backend.current.Load() != collector || h.coordinator.Status().Effective != "headless" {
		t.Fatal("collector ownership changed before the active Chrome capture drained")
	}
	close(backend.release)
	var reader readerResult
	select {
	case reader = <-readerReady:
	case <-testCtx.Done():
		t.Fatal("native reader did not acquire after the real collector drain")
	}
	if reader.err != nil || reader.lease == nil || reader.release == nil || reader.lease.Driver() != "browser" {
		t.Fatalf("real native reader acquisition failed: %v", reader.err)
	}
	owner := backend.current.Load()
	if owner == nil || owner.PID() == collector.PID() || !collector.exited.Load() || h.manager.PID() != owner.PID() {
		t.Fatal("Chrome owner did not switch after the prior process tree exited")
	}
	if bridge.releaseProofs.Load() < 2 || bridge.launchCount.Load() != 2 {
		t.Fatalf("replacement launch lacked prior Job membership proof: proofs=%d launches=%d", bridge.releaseProofs.Load(), bridge.launchCount.Load())
	}
	flags, err := owner.FixtureBootstrapWindows(testCtx)
	if err != nil || len(flags) != 2 || countBootstrapWindows(flags) != 1 {
		t.Fatalf("headed Chrome did not track its bootstrap and reader windows: %v %v", flags, err)
	}
	if err = owner.OpenExtensionsPage(testCtx); err != nil {
		t.Fatal(err)
	}
	flags, err = owner.FixtureBootstrapWindows(testCtx)
	if err != nil || len(flags) != 3 || countBootstrapWindows(flags) != 1 {
		t.Fatalf("second headed reader window was not tracked with bootstrap: %v %v", flags, err)
	}
	closeChromeFixtureReaderWindow(t, testCtx, owner)
	flags, err = owner.FixtureBootstrapWindows(testCtx)
	if err != nil || len(flags) != 2 || countBootstrapWindows(flags) != 1 {
		t.Fatalf("closing one user reader window did not retain one reader plus bootstrap: %v %v", flags, err)
	}
	h.coordinator.Retry()
	time.Sleep(200 * time.Millisecond)
	if owner.exited.Load() || h.manager.PID() != owner.PID() || h.coordinator.Status().Effective != "browser" || !h.coordinator.Status().BatchPaused {
		t.Fatal("collector resumed or closed a remaining headed reader window")
	}
	if !h.engine.WaitForIdle(5 * time.Second) {
		t.Fatal("captured fixture observation did not settle")
	}
	firstRun, err := h.state.GetRun(testCtx, session.Runs[0].ID)
	if err != nil || firstRun.Status != "completed" {
		t.Fatalf("drained active capture was not durably completed: run=%s err=%v", firstRun.Status, err)
	}
	secondRun, err := h.state.GetRun(testCtx, session.Runs[1].ID)
	if err != nil || secondRun.Status != "waiting_for_bridge" {
		t.Fatalf("continuation was not held as durable bridge work: run=%s err=%v", secondRun.Status, err)
	}
	if command, claimErr := h.engine.claimCommandForCollector(testCtx, secondRun.ID, "fixture", "headless", "headless"); claimErr != nil || command != nil {
		t.Fatalf("paused continuation was admitted: command=%v err=%v", command, claimErr)
	}
	oldPayload := backend.capturedPayload()
	if len(oldPayload) == 0 {
		t.Fatal("active Chrome command owner evidence was not retained by the test fixture")
	}
	pausedIDs, err := h.state.ListFixturePausedCaptureSessions(testCtx)
	if err != nil || !hasFixtureSessionID(pausedIDs, session.ID) {
		t.Fatalf("SQLite did not retain the paused session: ids=%v err=%v", pausedIDs, err)
	}
	dbPath := h.dbPath
	manager := h.manager
	h.stopEngineStore()
	releaseReaderLease()
	if reader.lease != nil {
		reader.lease.Release()
		reader.release()
	}
	h = newChromeFixtureEngineHarness(t, config, bridge, fixture, backend, manager, dbPath)
	waitChromeFixture(t, func() bool {
		status := h.coordinator.Status()
		return status.Failure != "" && status.BatchPaused && status.Effective == "browser"
	}, "restarted Engine retains paused queue while headed windows remain")
	if h.manager.PID() != owner.PID() || bridge.launchCount.Load() != 2 {
		t.Fatal("Engine restart replaced a headed owner whose actual windows remained open")
	}
	pausedIDs, err = h.state.ListFixturePausedCaptureSessions(testCtx)
	if err != nil || !hasFixtureSessionID(pausedIDs, session.ID) {
		t.Fatalf("paused SQLite session was not retained across Engine and Store restart: %v %v", pausedIDs, err)
	}
	reloaded, err := h.state.GetSession(testCtx, session.ID)
	if err != nil || len(reloaded.Runs) != len(session.Runs) || reloaded.Runs[0].ID != session.Runs[0].ID || reloaded.Runs[1].ID != session.Runs[1].ID {
		t.Fatalf("restart did not preserve continuation run IDs: %v", err)
	}
	flags, err = owner.FixtureBootstrapWindows(testCtx)
	if err != nil || len(flags) != 2 || countBootstrapWindows(flags) != 1 {
		t.Fatalf("reader window inventory was lost after Engine restart: %v %v", flags, err)
	}
	closeChromeFixtureReaderWindow(t, testCtx, owner)
	flags, err = owner.FixtureBootstrapWindows(testCtx)
	if err != nil || len(flags) != 1 || countBootstrapWindows(flags) != 1 {
		t.Fatalf("final tracked bootstrap window was not retained for its owner: %v %v", flags, err)
	}
	time.Sleep(200 * time.Millisecond)
	if owner.exited.Load() || h.manager.PID() != owner.PID() || bridge.launchCount.Load() != 2 || !h.coordinator.Status().BatchPaused {
		t.Fatal("collector resumed while the tracked bootstrap window remained open")
	}
	if err = owner.CloseFixtureWindow(testCtx, 0); err != nil {
		t.Fatal(err)
	}
	select {
	case <-owner.done:
	case <-testCtx.Done():
		t.Fatal("natural final-window close did not complete the owned Chrome tree")
	}
	waitChromeFixture(t, func() bool {
		status := h.coordinator.Status()
		return status.Effective == "headless" && !status.BatchPaused && h.manager.Snapshot().State == captureruntime.Ready
	}, "verified natural owner exit resumes the collector and durable queue")
	if bridge.launchCount.Load() != 3 || bridge.releaseProofs.Load() < 3 {
		t.Fatalf("resumed collector was launched without a fresh whole-tree proof: launches=%d proofs=%d", bridge.launchCount.Load(), bridge.releaseProofs.Load())
	}
	resumed := backend.current.Load()
	if resumed == nil || resumed.PID() == owner.PID() {
		t.Fatal("resumed collector did not receive a new Chrome PID")
	}
	evidence, err := resumed.Evidence(testCtx)
	authority, authorityErr := fixture.AuthorityEvidence(testCtx)
	if err != nil || authorityErr != nil || !evidence.Authenticated || evidence.AccountID != "alpha" || evidence.Revision != authority.Revision {
		t.Fatalf("same-profile synthetic authentication did not survive Engine restart: %v", err)
	}
	if _, err = owner.Evidence(testCtx); err == nil {
		t.Fatal("stale reader owner could still use the synthetic Chrome session")
	}
	if fixture.logins.Load() != 1 {
		t.Fatal("collector resume performed an automatic synthetic login")
	}
	pausedIDs, err = h.state.ListFixturePausedCaptureSessions(testCtx)
	if err != nil || hasFixtureSessionID(pausedIDs, session.ID) {
		t.Fatalf("session pause marker remained after verified resume: %v %v", pausedIDs, err)
	}
	if err = h.engine.validateCaptureOwner(session.ID, oldPayload); err == nil {
		t.Fatal("old Engine epoch/generation evidence remained valid after restart")
	}
	command, err := h.engine.claimCommandForCollector(testCtx, session.Runs[1].ID, "fixture-resume", "headless", "headless")
	if err != nil || command == nil {
		t.Fatalf("resumed continuation could not be claimed: command=%v err=%v", command, err)
	}
	stamp, ok := command.Payload["captureRuntime"].(map[string]any)
	if !ok || stamp["epoch"] != h.engine.Epoch() || !captureGenerationMatches(stamp["generation"], h.manager.Snapshot().Generation) {
		t.Fatal("resumed continuation did not receive the current Engine epoch and Chrome generation")
	}
	resumedObservation, captureErr := backend.Capture(testCtx, session.Runs[1].Source, command.Payload)
	if captureErr != nil || backend.calls.Load() != 2 || fixture.sequence.Load() != 2 {
		t.Fatalf("resumed source was not captured by the new Chrome collector: calls=%d posts=%d err=%v", backend.calls.Load(), fixture.sequence.Load(), captureErr)
	}
	if _, err = h.engine.AcceptObservation(testCtx, command.ID, session.Runs[1].ID, resumedObservation); err != nil {
		t.Fatal(err)
	}
	if _, err = h.engine.AcceptObservation(testCtx, command.ID, session.Runs[1].ID, syntheticObservation(session.Runs[1].Source)); err == nil {
		t.Fatal("duplicate resumed observation was admitted")
	}
	if !h.engine.WaitForIdle(5 * time.Second) {
		t.Fatal("resumed continuation reasoning did not drain")
	}
}

func countBootstrapWindows(flags []bool) int {
	count := 0
	for _, bootstrap := range flags {
		if bootstrap {
			count++
		}
	}
	return count
}

func TestChromeFixtureCleanupFailureBlocksReplacement(t *testing.T) {
	if os.Getenv("AKU_CHROME_FIXTURE_ROOT") == "" {
		t.Skip("Chrome fixture requires the explicit VerifyChrome helper")
	}
	fixture := startChromeFixtureHTTP(t)
	defer fixture.server.Close()
	config, bridge := startChromeFixtureForTest(t, fixture)
	backend := newChromeFixtureCaptureBackend(bridge, false)
	h := newChromeFixtureEngineHarness(t, config, bridge, fixture, backend, nil, "")
	initial := backend.current.Load()
	initial.failCleanup.Store(true)
	ctx, cancel := context.WithTimeout(h.ctx, 4*time.Second)
	defer cancel()
	_, _, err := h.engine.BorrowNativeReader(ctx)
	if err == nil {
		t.Fatal("native reader was admitted after a fixture cleanup fault")
	}
	snapshot := h.manager.Snapshot()
	if snapshot.State != captureruntime.Blocked || snapshot.Driver != "headless" || h.manager.PID() != initial.PID() {
		t.Fatalf("failed Chrome cleanup did not retain its exact owner: %+v", snapshot)
	}
	if bridge.launchCount.Load() != 1 || backend.current.Load() != initial || initial.exited.Load() || !h.coordinator.Status().BatchPaused {
		t.Fatal("replacement Chrome launched despite unverified profile release")
	}
	if err := bridge.containsPID(uint32(initial.PID())); err != nil {
		t.Fatal("cleanup fault fixture did not leave the owned Chrome process in its Job Object")
	}
}

func TestChromeFixtureUnknownInventoryFailsClosed(t *testing.T) {
	if os.Getenv("AKU_CHROME_FIXTURE_ROOT") == "" {
		t.Skip("Chrome fixture requires the explicit VerifyChrome helper")
	}
	fixture := startChromeFixtureHTTP(t)
	defer fixture.server.Close()
	config, bridge := startChromeFixtureForTest(t, fixture)
	backend := newChromeFixtureCaptureBackend(bridge, false)
	h := newChromeFixtureEngineHarness(t, config, bridge, fixture, backend, nil, "")
	initial := backend.current.Load()
	ctx, cancel := context.WithTimeout(h.ctx, 4*time.Second)
	defer cancel()
	if err := initial.DisconnectFixtureTransport(ctx); err != nil {
		t.Fatal(err)
	}
	_, _, borrowErr := h.engine.BorrowNativeReader(ctx)
	if borrowErr == nil {
		t.Fatal("native reader was admitted after CDP inventory became unknown")
	}
	waitChromeFixture(t, func() bool {
		return h.coordinator.Status().Failure != "" && h.coordinator.Status().BatchPaused
	}, "unknown CDP inventory blocks capture admission")
	if bridge.launchCount.Load() != 1 || backend.current.Load() != initial || h.manager.PID() != initial.PID() {
		t.Fatal("unknown CDP inventory allowed a replacement Chrome launch")
	}
	if err := initial.ReplacementReadiness(ctx); err == nil {
		t.Fatal("transport loss was treated as a verified empty-window inventory")
	}
}

func TestChromeFixtureLogoutAndAccountChangeBlockResume(t *testing.T) {
	if os.Getenv("AKU_CHROME_FIXTURE_ROOT") == "" {
		t.Skip("Chrome fixture requires the explicit VerifyChrome helper")
	}
	for _, scenario := range []string{"logout", "account-change"} {
		t.Run(scenario, func(t *testing.T) {
			fixture := startChromeFixtureHTTP(t)
			defer fixture.server.Close()
			config, bridge := startChromeFixtureForTest(t, fixture)
			backend := newChromeFixtureCaptureBackend(bridge, false)
			h := newChromeFixtureEngineHarness(t, config, bridge, fixture, backend, nil, "")
			leaseCtx, cancel := context.WithTimeout(h.ctx, 25*time.Second)
			defer cancel()
			lease, release, err := h.engine.BorrowNativeReader(leaseCtx)
			if err != nil {
				t.Fatal(err)
			}
			owner := backend.current.Load()
			if scenario == "logout" {
				err = owner.Logout(leaseCtx)
			} else {
				err = owner.ChangeAccount(leaseCtx)
			}
			if err != nil {
				t.Fatal(err)
			}
			userInitiatedLogins := fixture.logins.Load()
			lease.Release()
			release()
			closeChromeFixtureWindowsNaturally(t, leaseCtx, owner)
			waitChromeFixture(t, func() bool { return h.coordinator.Status().Failure != "" }, "authority mismatch blocks synthetic resume")
			if bridge.launchCount.Load() != 2 || h.manager.PID() != owner.PID() || !h.coordinator.Status().BatchPaused {
				t.Fatal("logout/account change launched a replacement collector")
			}
			if fixture.logins.Load() != userInitiatedLogins {
				t.Fatal("session failure triggered an automatic second login")
			}
		})
	}
}

func TestChromeFixtureBrowserCookieLossBlocksAdmission(t *testing.T) {
	if os.Getenv("AKU_CHROME_FIXTURE_ROOT") == "" {
		t.Skip("Chrome fixture requires the explicit VerifyChrome helper")
	}
	fixture := startChromeFixtureHTTP(t)
	defer fixture.server.Close()
	config, bridge := startChromeFixtureForTest(t, fixture)
	backend := newChromeFixtureCaptureBackend(bridge, false)
	h := newChromeFixtureEngineHarness(t, config, bridge, fixture, backend, nil, "")
	testCtx, cancelTest := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancelTest()
	session, err := h.engine.StartVisibleUpdate(testCtx, "fixture cookie loss remains paused")
	if err != nil || len(session.Runs) == 0 {
		t.Fatalf("fixture queue was not created: runs=%d err=%v", len(session.Runs), err)
	}
	lease, release, err := h.engine.BorrowNativeReader(testCtx)
	if err != nil {
		t.Fatal(err)
	}
	owner := backend.current.Load()
	if err = owner.ClearFixtureCookie(testCtx); err != nil {
		t.Fatal(err)
	}
	authority, err := fixture.AuthorityEvidence(testCtx)
	if err != nil || !authority.Authenticated || authority.AccountID != "alpha" {
		t.Fatalf("synthetic server authority unexpectedly changed: %+v %v", authority, err)
	}
	lease.Release()
	release()
	closeChromeFixtureWindowsNaturally(t, testCtx, owner)
	waitChromeFixture(t, func() bool {
		status := h.coordinator.Status()
		return status.Failure != "" && status.BatchPaused && status.Effective == "headless" && bridge.launchCount.Load() == 3
	}, "post-launch Chrome probe rejects a missing synthetic profile cookie")
	select {
	case <-owner.done:
	case <-testCtx.Done():
		t.Fatal("reader owner did not release after the final test window closed")
	}
	resumed := backend.current.Load()
	if resumed == nil || resumed.PID() == owner.PID() {
		t.Fatal("fresh headless Chrome owner was not launched for the post-launch proof")
	}
	evidence, err := resumed.Evidence(testCtx)
	if err != nil || evidence.Authenticated {
		t.Fatalf("fresh Chrome unexpectedly retained a cleared fixture cookie: %+v %v", evidence, err)
	}
	authority, authorityErr := fixture.AuthorityEvidence(testCtx)
	if fixture.logins.Load() != 1 || authorityErr != nil || !authority.Authenticated {
		t.Fatal("browser cookie loss triggered login or changed server-side fixture authority")
	}
	if command, claimErr := h.engine.claimCommandForCollector(testCtx, session.Runs[0].ID, "fixture-cookie-loss", "headless", "headless"); claimErr != nil || command != nil {
		t.Fatalf("queue reopened despite failed fresh-browser cookie proof: command=%v err=%v", command, claimErr)
	}
	pausedIDs, err := h.state.ListFixturePausedCaptureSessions(testCtx)
	if err != nil || !hasFixtureSessionID(pausedIDs, session.ID) {
		t.Fatalf("failed post-launch browser proof did not retain the SQLite pause marker: %v %v", pausedIDs, err)
	}
}
