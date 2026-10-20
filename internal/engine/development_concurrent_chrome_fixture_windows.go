//go:build windows && auth_session_dev && auth_session_fixture

package engine

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/collection"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/readerbroker"
	"golang.org/x/sys/windows"
)

type concurrentChromeReceipt struct {
	Provider        string `json:"provider"`
	Scenario        string `json:"scenario"`
	Passed          bool   `json:"passed"`
	CleanupVerified bool   `json:"cleanupVerified"`
	LoginCount      int    `json:"loginCount"`
	SeedCount       int    `json:"seedCount"`
	OverlapVerified bool   `json:"overlapVerified"`
	CanAuthorize    bool   `json:"canAuthorize"`
	NativeQualified bool   `json:"nativeQualified"`
}

// RunDevelopmentConcurrentChromeFixture is an explicit early-dispatch QA
// composition. It never constructs an Engine, Store, native provider or app UI.
func RunDevelopmentConcurrentChromeFixture(input io.Reader, output io.Writer) error {
	request, err := parseConcurrentChromeInput(input)
	if err != nil {
		return err
	}
	if err = validateConcurrentChromeRoot(request); err != nil {
		return err
	}
	request.NodePath, err = filepath.EvalSymlinks(request.NodePath)
	if err != nil {
		return err
	}
	request.ChromePath, err = filepath.EvalSymlinks(request.ChromePath)
	if err != nil {
		return err
	}
	if request.Scenario == "parent-process-death" {
		return runConcurrentChromeParentDeath(request, output)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	f, err := newConcurrentChromeFixture(ctx, request)
	if err != nil {
		return err
	}
	if request.Scenario == "death-child" {
		if err = f.overlap(ctx, false); err == nil {
			f.server.mu.Lock()
			logins, overlap := f.server.logins, f.server.overlap
			f.server.mu.Unlock()
			err = json.NewEncoder(output).Encode(struct {
				Stage           string `json:"stage"`
				LoginCount      int    `json:"loginCount"`
				SeedCount       int    `json:"seedCount"`
				OverlapVerified bool   `json:"overlapVerified"`
			}{"owned-pair-ready", logins, f.seeds, overlap})
		}
		if err == nil {
			<-ctx.Done()
			err = errors.New("synthetic concurrent Chrome death child was not terminated")
		}
	} else {
		err = f.run(ctx, request.Scenario)
	}
	cleanupErr := f.close()
	if cleanupErr != nil {
		cleanupErr = fmt.Errorf("final owned cleanup: %w", cleanupErr)
	}
	if err = errors.Join(err, cleanupErr); err != nil {
		return fmt.Errorf("synthetic concurrent Chrome acceptance failed: %w", err)
	}
	f.server.mu.Lock()
	logins, overlap := f.server.logins, f.server.overlap
	f.server.mu.Unlock()
	if logins != 1 || f.seeds != 1 {
		return errors.New("synthetic concurrent Chrome login/seed ceiling failed")
	}
	return json.NewEncoder(output).Encode(concurrentChromeReceipt{Provider: "synthetic-concurrent-chrome-fixture", Scenario: request.Scenario, Passed: true, CleanupVerified: true, LoginCount: logins, SeedCount: f.seeds, OverlapVerified: overlap})
}

func validateConcurrentChromeRoot(request concurrentChromeInput) error {
	invalid := errors.New("synthetic concurrent Chrome fresh root rejected")
	project, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		return invalid
	}
	parent := filepath.Join(project, "runtime", "qa", "auth-session-sidecar")
	if filepath.Dir(request.Root) != parent {
		return invalid
	}
	resolved, err := filepath.EvalSymlinks(request.Root)
	if err != nil || !samePath(resolved, request.Root) {
		return invalid
	}
	info, err := os.Lstat(request.Root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return invalid
	}
	entries, err := os.ReadDir(request.Root)
	if err != nil || len(entries) != 0 {
		return invalid
	}
	for _, path := range []string{request.NodePath, request.ChromePath} {
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			return invalid
		}
		info, err := os.Lstat(resolved)
		if err != nil || !info.Mode().IsRegular() {
			return invalid
		}
	}
	return nil
}

type concurrentChromeServer struct {
	mu              sync.Mutex
	server          *httptest.Server
	token           string
	account         string
	logins          int
	activeCollector bool
	overlap         bool
	entered         chan struct{}
	release         chan struct{}
	releaseOnce     sync.Once
}

func newConcurrentChromeServer() *concurrentChromeServer {
	f := &concurrentChromeServer{account: "alpha", entered: make(chan struct{}, 1), release: make(chan struct{})}
	f.server = httptest.NewServer(http.HandlerFunc(f.serve))
	return f
}
func (f *concurrentChromeServer) unblock() { f.releaseOnce.Do(func() { close(f.release) }) }
func (f *concurrentChromeServer) serve(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'")
	w.Header().Set("Content-Type", "application/json")
	if r.URL.RawQuery != "" {
		w.WriteHeader(400)
		return
	}
	if r.URL.Path == "/" && r.Method == "GET" {
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, "<!doctype html><title>Synthetic concurrent fixture</title><p>Controlled loopback fixture</p>")
		return
	}
	if r.URL.Path == "/login" && r.Method == "POST" {
		var token [32]byte
		if _, err := rand.Read(token[:]); err != nil {
			w.WriteHeader(500)
			return
		}
		f.mu.Lock()
		if f.logins != 0 {
			f.mu.Unlock()
			w.WriteHeader(409)
			return
		}
		f.logins++
		f.token = hex.EncodeToString(token[:])
		value := f.token
		f.mu.Unlock()
		http.SetCookie(w, &http.Cookie{Name: "fixture_session", Value: value, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, Expires: time.Now().Add(time.Hour).Truncate(time.Second)})
		_ = json.NewEncoder(w).Encode(map[string]any{"account": "alpha", "epoch": 1})
		return
	}
	f.mu.Lock()
	cookie, err := r.Cookie("fixture_session")
	valid := err == nil && f.token != "" && cookie.Value == f.token
	account := f.account
	if !valid {
		f.mu.Unlock()
		w.WriteHeader(401)
		_ = json.NewEncoder(w).Encode(map[string]bool{"authenticated": false})
		return
	}
	switch {
	case r.URL.Path == "/probe" && r.Method == "GET":
		f.mu.Unlock()
	case r.URL.Path == "/collect" && r.Method == "POST":
		f.activeCollector = true
		f.mu.Unlock()
		select {
		case f.entered <- struct{}{}:
		default:
		}
		select {
		case <-f.release:
		case <-r.Context().Done():
		}
		f.mu.Lock()
		f.activeCollector = false
		f.mu.Unlock()
	case r.URL.Path == "/reader" && r.Method == "GET":
		f.overlap = f.overlap || f.activeCollector
		f.mu.Unlock()
	default:
		f.mu.Unlock()
		w.WriteHeader(404)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"account": account, "epoch": 1, "postId": "fixture-post-1"})
}

type concurrentChromeRecord struct {
	owner        concurrentOwner
	bridge       *chromeFixtureBridge
	process      *chromeFixtureProcess
	manager      *captureruntime.Manager
	identity     processFixtureIdentity
	nodeIdentity processFixtureIdentity
	profileID    string
	closeOnce    sync.Once
	absent       chan struct{}
	closeErr     error
}

type concurrentChromeFixture struct {
	request                concurrentChromeInput
	ctx                    context.Context
	cancel                 context.CancelFunc
	server                 *concurrentChromeServer
	c                      *concurrentController
	mu                     sync.Mutex
	records                map[concurrentRole]*concurrentChromeRecord
	windows                map[string]int
	coordinator            *collection.Coordinator
	loopDone               <-chan struct{}
	captureDone            <-chan struct{}
	clockOffset            atomic.Int64
	seeds                  int
	grants                 int
	gateWork               string
	collectorExited        chan struct{}
	launches               atomic.Int64
	heldLease              *captureruntime.Lease
	rootID                 string
	diagnostics            []string
	collectorExitEvents    atomic.Int64
	readerRetiringEvents   atomic.Int64
	readerUnexpectedEvents atomic.Int64
}

func newConcurrentChromeFixture(parent context.Context, request concurrentChromeInput) (*concurrentChromeFixture, error) {
	rootID, identityErr := processFixtureDirectoryID(request.Root)
	if identityErr != nil {
		return nil, errors.New("synthetic concurrent Chrome root identity unavailable")
	}
	ctx, cancel := context.WithCancel(parent)
	f := &concurrentChromeFixture{request: request, ctx: ctx, cancel: cancel, server: newConcurrentChromeServer(), records: make(map[concurrentRole]*concurrentChromeRecord), windows: make(map[string]int), collectorExited: make(chan struct{})}
	f.rootID = rootID
	readerRoot, collectorRoot := filepath.Join(request.Root, "run-reader"), filepath.Join(request.Root, "run-collector")
	for _, root := range []string{readerRoot, collectorRoot} {
		if err := os.Mkdir(root, 0700); err != nil {
			f.server.server.Close()
			cancel()
			return nil, err
		}
	}
	readerID, err := processFixtureDirectoryID(readerRoot)
	if err != nil {
		f.server.server.Close()
		cancel()
		return nil, err
	}
	collectorID, err := processFixtureDirectoryID(collectorRoot)
	if err != nil {
		f.server.server.Close()
		cancel()
		return nil, err
	}
	f.c, err = newConcurrentController(ctx, concurrentConfig{Enabled: true, DevBuild: true, FixtureBuild: true, OwnerGrant: true, ControllerGrant: true, Provider: concurrentProvider, AttemptID: "synthetic:attempt:chrome", ParentID: "synthetic:parent:chrome", ExpectedAccountID: "synthetic:account:alpha", ExpectedRevision: "synthetic:revision:fixture-1", ReaderProfileID: "synthetic:profile:chrome-reader", ReaderDirectoryID: "synthetic:directory:" + readerID, CollectorProfileID: "synthetic:profile:chrome-collector", CollectorDirectoryID: "synthetic:directory:" + collectorID, IndependentCollector: request.Scenario != "last-close-denied", now: f.now}, concurrentPorts{owners: f, verifier: f, permit: f})
	if err != nil {
		f.server.server.Close()
		cancel()
		return nil, err
	}
	if err = f.c.Start(ctx); err != nil {
		cleanup := f.close()
		return nil, errors.Join(err, cleanup)
	}
	collector, reader := f.records[concurrentCollector], f.records[concurrentReader]
	f.coordinator = collection.NewCoordinator(collector.manager, func(context.Context, string, uint64) (captureruntime.Process, error) {
		f.launches.Add(1)
		return nil, errConcurrentUnavailable
	}, func() error { return nil })
	f.gateWork, err = f.c.QueueWork("synthetic:admission-check")
	if err != nil {
		cleanup := f.close()
		return nil, errors.Join(err, cleanup)
	}
	gate := func(ctx context.Context) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		_, err := f.c.Ticket(f.gateWork)
		return err
	}
	if err = f.coordinator.BindConcurrentSession(reader.manager, collector.manager.Snapshot().Generation, f, gate, gate); err != nil {
		cleanup := f.close()
		return nil, errors.Join(err, cleanup)
	}
	f.loopDone = f.coordinator.StartWithDone(ctx)
	f.coordinator.Request("headless")
	return f, nil
}
func (f *concurrentChromeFixture) now() time.Time {
	return time.Now().Add(time.Duration(f.clockOffset.Load()))
}

func verifyConcurrentChromeImage(identity processFixtureIdentity, path string) error {
	actual, err := processFixtureProcessIdentity(identity.PID)
	if err != nil || actual != identity {
		return errors.New("synthetic concurrent Chrome process identity changed")
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, identity.PID)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	buffer := make([]uint16, 32768)
	size := uint32(len(buffer))
	if err = windows.QueryFullProcessImageName(h, 0, &buffer[0], &size); err != nil || !samePath(windows.UTF16ToString(buffer[:size]), path) {
		return errors.New("synthetic concurrent Chrome image changed")
	}
	return nil
}

func (f *concurrentChromeFixture) CreateOwner(ctx context.Context, spec concurrentOwnerSpec) (concurrentOwner, error) {
	role, root := "owner", filepath.Join(f.request.Root, "run-reader")
	if spec.Role == concurrentCollector {
		role, root = "collector", filepath.Join(f.request.Root, "run-collector")
	}
	project, _ := filepath.Abs(filepath.Join("..", ".."))
	bridge, err := startChromeFixtureBridge(ctx, chromeFixtureConfig{NodePath: f.request.NodePath, ChromePath: f.request.ChromePath, Root: root, Origin: f.server.server.URL, DriverPath: filepath.Join(project, "scripts", "auth-session-fixture", "chrome-concurrent-engine-driver.mjs"), NativeReaderStartup: role == "owner"})
	if err != nil {
		return concurrentOwner{}, err
	}
	record := &concurrentChromeRecord{bridge: bridge, absent: make(chan struct{})}
	f.mu.Lock()
	f.records[spec.Role] = record
	f.mu.Unlock()
	nodeIdentity, err := processFixtureProcessIdentity(bridge.pid)
	if err != nil {
		return concurrentOwner{}, err
	}
	if err = verifyConcurrentChromeImage(nodeIdentity, f.request.NodePath); err != nil {
		return concurrentOwner{}, err
	}
	record.nodeIdentity = nodeIdentity
	process, err := bridge.launch(ctx, role)
	if err != nil {
		return concurrentOwner{}, fmt.Errorf("synthetic concurrent Chrome %s launch: %w", spec.Role, err)
	}
	record.process = process
	identity, err := processFixtureProcessIdentity(process.pid)
	if err != nil {
		return concurrentOwner{}, err
	}
	if err = verifyConcurrentChromeImage(identity, f.request.ChromePath); err != nil {
		return concurrentOwner{}, err
	}
	record.identity = identity
	record.profileID, err = processFixtureDirectoryID(filepath.Join(root, "shared"))
	if err != nil {
		return concurrentOwner{}, err
	}
	manager, err := captureruntime.New(process)
	if err != nil {
		return concurrentOwner{}, err
	}
	record.manager = manager
	owner := concurrentOwner{ID: "synthetic:chrome-owner:" + string(spec.Role), Role: spec.Role, ProfileID: spec.ProfileID, DirectoryID: spec.DirectoryID, PID: process.PID(), CreationIdentity: "synthetic:process-created:" + identity.Created, ParentID: spec.ParentID, Generation: manager.Snapshot().Generation, Driver: spec.Driver}
	record.owner = owner
	manager.SetExitObserver(func(uint64) {
		if spec.Role == concurrentReader && process.retiring.Load() {
			f.readerRetiringEvents.Add(1)
			// The final-window RPC still needs its bounded response from Node.
			// ReaderInventoryEmpty retires the bridge after that request and the
			// verified Chrome tree exit, so the observer cannot cut its pipe early.
			return
		}
		// Done is published only after this exact role's Chrome descendants are
		// verified gone. Close its still-owned bridge Job before retiring it.
		_ = f.closeRecord(record)
		if spec.Role == concurrentCollector {
			f.collectorExitEvents.Add(1)
			f.c.CollectorLost()
			close(f.collectorExited)
		} else if !process.retiring.Load() {
			f.readerUnexpectedEvents.Add(1)
			f.c.Revoke("chrome-reader-exit")
		}
	})
	if spec.Role == concurrentReader {
		if err = process.Login(ctx); err != nil {
			return concurrentOwner{}, fmt.Errorf("synthetic concurrent Chrome reader login: %w", err)
		}
	} else {
		f.server.mu.Lock()
		token := f.server.token
		f.server.mu.Unlock()
		var result struct {
			Seeded bool `json:"seeded"`
		}
		if err = bridge.rpc(ctx, "seed_fixture", map[string]any{"owner": process.owner, "value": token}, &result); err != nil || !result.Seeded {
			return concurrentOwner{}, errors.New("synthetic concurrent Chrome seed rejected")
		}
		f.seeds++
	}
	return owner, nil
}

func (f *concurrentChromeFixture) diagnostic(stage string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.diagnostics) < 32 {
		f.diagnostics = append(f.diagnostics, stage)
	}
}

func (f *concurrentChromeFixture) CurrentOwner(ctx context.Context, role concurrentRole) (value concurrentOwner, exists bool, resultErr error) {
	stage := "lookup"
	defer func() {
		if resultErr != nil {
			f.diagnostic("current_" + string(role) + "_" + stage)
		}
	}()
	f.mu.Lock()
	record := f.records[role]
	f.mu.Unlock()
	if record == nil {
		return concurrentOwner{}, false, nil
	}
	select {
	case <-record.absent:
		return record.owner, false, nil
	default:
	}
	if record.manager == nil {
		stage = "manager_unavailable"
		return concurrentOwner{}, false, errConcurrentUnavailable
	}
	state := record.manager.Snapshot()
	if state.State != captureruntime.Ready || state.Generation != record.owner.Generation || record.manager.PID() != record.owner.PID {
		f.diagnostic("current_" + string(role) + "_not_ready")
		return record.owner, false, nil
	}
	if err := verifyConcurrentChromeImage(record.identity, f.request.ChromePath); err != nil {
		stage = "process_identity"
		return concurrentOwner{}, false, err
	}
	if err := record.bridge.containsPID(record.process.pid); err != nil {
		stage = "job_membership"
		return concurrentOwner{}, false, err
	}
	rootID, err := processFixtureDirectoryID(record.bridge.config.Root)
	if err != nil || "synthetic:directory:"+rootID != record.owner.DirectoryID {
		stage = "owner_directory"
		return concurrentOwner{}, false, errors.New("synthetic concurrent Chrome owner directory identity changed")
	}
	profileID, err := processFixtureDirectoryID(filepath.Join(record.bridge.config.Root, "shared"))
	if err != nil || profileID != record.profileID {
		stage = "profile_directory"
		return concurrentOwner{}, false, errors.New("synthetic concurrent Chrome profile identity changed")
	}
	if err := ctx.Err(); err != nil {
		stage = "context"
		return concurrentOwner{}, false, err
	}
	return record.owner, true, nil
}

func (f *concurrentChromeFixture) closeRecord(record *concurrentChromeRecord) error {
	record.closeOnce.Do(func() {
		if record.manager != nil {
			record.manager.Terminate()
		} else {
			record.bridge.terminateOwnedTree()
		}
		// Job accounting alone can become empty before process handles signal.
		// The existing bridge publishes Done only after its pinned handles and
		// owned windows are verified released. Do not race profile retirement.
		if record.process != nil {
			select {
			case <-record.process.Done():
			case <-time.After(concurrentCleanupTimeout):
				record.closeErr = errors.New("synthetic concurrent Chrome owned process completion unverified")
				return
			}
		} else {
			select {
			case <-record.bridge.nodeWaited:
			case <-time.After(concurrentCleanupTimeout):
				record.closeErr = errors.New("synthetic concurrent Chrome bridge completion unverified")
				return
			}
		}
		if err := record.bridge.cleanupError(); err != nil {
			record.closeErr = err
			return
		}
		if !record.bridge.cleanupVerified() {
			record.closeErr = errConcurrentCleanup
			return
		}
		close(record.absent)
	})
	return record.closeErr
}
func (f *concurrentChromeFixture) CloseOwner(_ context.Context, owner concurrentOwner) error {
	f.mu.Lock()
	record := f.records[owner.Role]
	f.mu.Unlock()
	if record == nil || !sameOwner(record.owner, owner) {
		f.diagnostic("close_" + string(owner.Role) + "_owner_mismatch")
		return errConcurrentStale
	}
	err := f.closeRecord(record)
	if err != nil {
		f.diagnostic("close_" + string(owner.Role) + "_tree_unverified")
	}
	return err
}
func (f *concurrentChromeFixture) OwnerAbsent(ctx context.Context, role concurrentRole, id string) (bool, error) {
	f.mu.Lock()
	record := f.records[role]
	f.mu.Unlock()
	if record == nil {
		return true, nil
	}
	if record.owner.ID != id {
		f.diagnostic("absent_" + string(role) + "_owner_mismatch")
		return false, errConcurrentStale
	}
	select {
	case <-record.absent:
		if !record.bridge.cleanupVerified() || record.closeErr != nil {
			f.diagnostic("absent_" + string(role) + "_job_unverified")
		}
		return record.bridge.cleanupVerified(), record.closeErr
	case <-ctx.Done():
		f.diagnostic("absent_" + string(role) + "_timeout")
		return false, ctx.Err()
	}
}
func (f *concurrentChromeFixture) Prove(ctx context.Context, request concurrentProofRequest) (concurrentProof, error) {
	f.mu.Lock()
	record := f.records[request.Role]
	f.mu.Unlock()
	if record == nil {
		return concurrentProof{}, errConcurrentProof
	}
	evidence, err := record.process.Evidence(ctx)
	if err != nil {
		return concurrentProof{}, err
	}
	state := concurrentUnknown
	if evidence.Authenticated {
		state = concurrentAuthenticated
	}
	return concurrentProof{Provider: request.Provider, ControllerID: request.ControllerID, AttemptID: request.AttemptID, Epoch: request.Epoch, PairGeneration: request.PairGeneration, Role: request.Role, Challenge: request.Challenge, Owner: request.Owner, AuthState: state, AccountID: "synthetic:account:" + evidence.AccountID, Revision: "synthetic:revision:" + evidence.Revision, ObservedAt: f.now()}, nil
}
func (f *concurrentChromeFixture) Grant(context.Context, concurrentPermitRequest) (concurrentPermit, error) {
	f.grants++
	return concurrentPermit{ID: fmt.Sprintf("synthetic:chrome-permit:%d", f.grants), Granted: true}, nil
}
func (f *concurrentChromeFixture) Revoke(context.Context, string, uint64) error { return nil }

type concurrentChromeInventory struct {
	Exited         bool   `json:"exited"`
	WindowCount    int    `json:"windowCount"`
	BootstrapCount int    `json:"bootstrapCount"`
	Bootstrap      []bool `json:"bootstrap"`
	WindowKeys     []int  `json:"windowKeys"`
}

func (f *concurrentChromeFixture) inventory(ctx context.Context) (concurrentChromeInventory, error) {
	record := f.records[concurrentReader]
	var result concurrentChromeInventory
	if err := record.bridge.rpc(ctx, "inventory", map[string]any{"owner": record.process.owner}, &result); err != nil {
		return result, err
	}
	if result.WindowCount < 0 || result.WindowCount > 16 || len(result.WindowKeys) != result.WindowCount || len(result.Bootstrap) != result.WindowCount {
		return result, errors.New("synthetic concurrent Chrome inventory invalid")
	}
	return result, nil
}
func (f *concurrentChromeFixture) OpenReaderWindow(ctx context.Context, _ concurrentOwner, id, _ string, bootstrap bool) error {
	record := f.records[concurrentReader]
	if !bootstrap {
		if err := record.process.OpenFixtureIndependentWindow(ctx); err != nil {
			return err
		}
	}
	inventory, err := f.inventory(ctx)
	if err != nil {
		return err
	}
	if inventory.BootstrapCount != 1 {
		return errors.New("synthetic concurrent Chrome bootstrap missing")
	}
	selected := -1
	for index, key := range inventory.WindowKeys {
		used := false
		for _, tracked := range f.windows {
			used = used || tracked == key
		}
		if !used && inventory.Bootstrap[index] == bootstrap {
			if selected != -1 {
				return errors.New("synthetic concurrent Chrome ambiguous window")
			}
			selected = key
		}
	}
	if selected < 0 {
		return errors.New("synthetic concurrent Chrome window not found")
	}
	f.windows[id] = selected
	var result struct {
		Authenticated bool `json:"authenticated"`
	}
	if err = record.bridge.rpc(ctx, "reader_request", map[string]any{"owner": record.process.owner}, &result); err != nil || !result.Authenticated {
		return errors.New("synthetic concurrent Chrome reader request rejected")
	}
	return nil
}
func (f *concurrentChromeFixture) CloseReaderWindow(ctx context.Context, _ concurrentOwner, id string) error {
	inventory, err := f.inventory(ctx)
	if err != nil {
		return err
	}
	key, exists := f.windows[id]
	if !exists {
		return errConcurrentUnavailable
	}
	index := -1
	for candidate, value := range inventory.WindowKeys {
		if value == key {
			index = candidate
		}
	}
	if index < 0 {
		return errConcurrentStale
	}
	if err = f.records[concurrentReader].process.CloseFixtureWindow(ctx, index); err != nil {
		return err
	}
	delete(f.windows, id)
	return nil
}
func (f *concurrentChromeFixture) ReaderInventoryEmpty(ctx context.Context, _ concurrentOwner) (bool, error) {
	record := f.records[concurrentReader]
	select {
	case <-record.absent:
		return len(f.windows) == 0 && record.bridge.cleanupVerified(), record.closeErr
	default:
	}
	inventory, err := f.inventory(ctx)
	if err == nil && inventory.Exited || record.process.rootExited.Load() || record.process.exited.Load() {
		select {
		case <-record.process.Done():
		case <-ctx.Done():
			return false, ctx.Err()
		}
		if err := f.closeRecord(record); err != nil {
			return false, err
		}
		return len(f.windows) == 0, nil
	}
	return err == nil && inventory.WindowCount == 0, err
}
func (f *concurrentChromeFixture) Capture(ctx context.Context, source domain.Source, _ map[string]any) (domain.Observation, error) {
	if source != domain.SourceX {
		return domain.Observation{}, errConcurrentUnavailable
	}
	record := f.records[concurrentCollector]
	err := record.bridge.collect(ctx, record.process.owner)
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	return domain.Observation{}, err
}

func (f *concurrentChromeFixture) open(ctx context.Context, sequence int, bootstrap bool) error {
	var input, output bytes.Buffer
	if err := readerbroker.Write(&input, readerbroker.Request{RequestID: fmt.Sprintf("broker_%032x", sequence), Source: "x", URL: "https://x.com/synthetic/status/123"}); err != nil {
		return err
	}
	if err := readerbroker.RouteConcurrentFixture(ctx, &input, &output, func(ctx context.Context, _ readerbroker.Request) error {
		lease, release, err := f.coordinator.BorrowNativeReader(ctx)
		if err != nil {
			return err
		}
		defer lease.Release()
		defer release()
		if lease.Driver() != "browser" || f.records[concurrentReader].manager.PID() == f.records[concurrentCollector].manager.PID() {
			return errors.New("synthetic concurrent Chrome reader owner mismatch")
		}
		return f.c.OpenReaderWindow(ctx, fmt.Sprintf("synthetic:window:%d", sequence), fmt.Sprintf("synthetic:target:%d", sequence), bootstrap)
	}); err != nil {
		return err
	}
	var reply struct {
		Accepted bool `json:"syntheticAccepted"`
	}
	if err := readerbroker.Read(&output, &reply); err != nil || !reply.Accepted {
		return errors.New("synthetic concurrent Chrome framed reply invalid")
	}
	return nil
}

func (f *concurrentChromeFixture) captureStart(ctx context.Context) (concurrentTicket, *captureruntime.Lease, <-chan error, error) {
	id, err := f.c.QueueWork("synthetic:chrome-work")
	if err != nil {
		return concurrentTicket{}, nil, nil, err
	}
	ticket, err := f.c.Ticket(id)
	if err != nil {
		return ticket, nil, nil, err
	}
	if err = f.c.Claim(ctx, ticket); err != nil {
		return ticket, nil, nil, err
	}
	lease, err := f.records[concurrentCollector].manager.Acquire()
	if err != nil {
		return ticket, nil, nil, err
	}
	if lease.Generation() != ticket.CollectorGeneration || lease.Driver() != "headless" {
		lease.Release()
		return ticket, nil, nil, errConcurrentStale
	}
	done := make(chan error, 1)
	drained := make(chan struct{})
	f.captureDone = drained
	captureCtx, cancel := context.WithCancel(f.ctx)
	stop := context.AfterFunc(ctx, cancel)
	go func() {
		defer close(drained)
		defer cancel()
		defer stop()
		_, err := f.coordinator.Capture(captureCtx, domain.SourceX, nil)
		done <- err
	}()
	select {
	case <-f.server.entered:
		return ticket, lease, done, nil
	case err := <-done:
		if err == nil {
			done <- nil
			return ticket, lease, done, nil
		}
		lease.Release()
		return ticket, nil, nil, fmt.Errorf("synthetic concurrent Chrome capture did not enter: %w", err)
	case <-ctx.Done():
		lease.Release()
		return ticket, nil, nil, ctx.Err()
	}
}

// overlap requires an authenticated protected server request to remain active
// while the broker opens a second reader window and performs its own request.
func (f *concurrentChromeFixture) overlap(ctx context.Context, complete bool) error {
	if err := f.open(ctx, 1, true); err != nil {
		return err
	}
	ticket, lease, done, err := f.captureStart(ctx)
	if err != nil {
		return err
	}
	if complete {
		defer lease.Release()
	} else {
		f.heldLease = lease
	}
	collector := f.records[concurrentCollector]
	before, pid := collector.manager.Snapshot(), collector.manager.PID()
	if err = f.open(ctx, 2, false); err != nil {
		return err
	}
	f.server.mu.Lock()
	overlap := f.server.overlap
	f.server.mu.Unlock()
	inventory, err := f.inventory(ctx)
	if err != nil {
		return err
	}
	if !overlap || inventory.WindowCount != 2 || inventory.BootstrapCount != 1 || f.c.Snapshot().ReaderWindows != 2 || collector.manager.Snapshot() != before || collector.manager.PID() != pid || before.ActiveLeases != 1 || f.coordinator.Status().BatchPaused || f.launches.Load() != 0 {
		return errors.New("synthetic concurrent Chrome protected overlap/owner invariant failed")
	}
	if !complete {
		return nil
	}
	f.server.unblock()
	if err = <-done; err != nil {
		return err
	}
	lease.Release()
	if err = f.c.Complete(ctx, ticket, "synthetic:chrome-result"); err != nil {
		return err
	}
	if _, ok := f.c.SinkResult(ticket.WorkID); !ok {
		return errors.New("synthetic concurrent Chrome memory result missing")
	}
	if f.c.Complete(ctx, ticket, "synthetic:duplicate") == nil {
		return errors.New("synthetic concurrent Chrome duplicate accepted")
	}
	if f.records[concurrentReader].manager.Snapshot().State != captureruntime.Ready || f.c.Snapshot().ReaderWindows != 2 || collector.manager.Snapshot().Generation != before.Generation || collector.manager.PID() != pid || collector.manager.Snapshot().ActiveLeases != 0 {
		return errors.New("synthetic concurrent Chrome capture closed reader or changed collector")
	}
	return nil
}

func (f *concurrentChromeFixture) run(ctx context.Context, scenario string) error {
	if err := f.duplicateGates(ctx); err != nil {
		return err
	}
	if scenario == "overlap-last-close" || scenario == "last-close-denied" || scenario == "mismatch" {
		if err := f.overlap(ctx, true); err != nil {
			return err
		}
		collector := f.records[concurrentCollector]
		before, pid := collector.manager.Snapshot(), collector.manager.PID()
		grants := f.grants
		if err := f.c.CloseReaderWindow(ctx, "synthetic:window:2"); err != nil {
			return err
		}
		inventory, err := f.inventory(ctx)
		if err != nil || inventory.WindowCount != 1 || inventory.BootstrapCount != 1 || f.c.Snapshot().ReaderWindows != 1 {
			return errors.New("synthetic concurrent Chrome multiwindow close failed")
		}
		if scenario == "mismatch" {
			f.server.mu.Lock()
			f.server.account = "beta"
			f.server.mu.Unlock()
		}
		err = f.c.CloseReaderWindow(ctx, "synthetic:window:1")
		if err != nil && scenario == "overlap-last-close" {
			return fmt.Errorf("last reader close and fresh continuation: %w", err)
		}
		if scenario != "overlap-last-close" {
			if err == nil || f.c.Snapshot().AdmissionOpen || f.grants != grants {
				return errors.New("synthetic concurrent Chrome dependent/mismatched continuation admitted")
			}
		} else {
			if err != nil || !f.c.Snapshot().AdmissionOpen || f.grants != grants+1 {
				return errors.New("synthetic concurrent Chrome fresh continuation missing")
			}
			ticket, lease, done, err := f.captureStart(ctx)
			if err != nil {
				return err
			}
			defer lease.Release()
			if err = <-done; err != nil {
				return err
			}
			lease.Release()
			if err = f.c.Complete(ctx, ticket, "synthetic:independent-chrome-result"); err != nil {
				return err
			}
		}
		if collector.manager.Snapshot() != before || collector.manager.PID() != pid || f.open(ctx, 3, false) == nil {
			return errors.New("synthetic concurrent Chrome last close changed collector/reopened reader")
		}
		return nil
	}
	if err := f.open(ctx, 1, true); err != nil {
		return err
	}
	ticket, lease, done, err := f.captureStart(ctx)
	if err != nil {
		return err
	}
	defer lease.Release()
	before, pid := f.records[concurrentCollector].manager.Snapshot(), f.records[concurrentCollector].manager.PID()
	if err = f.open(ctx, 2, false); err != nil {
		return err
	}
	if f.records[concurrentCollector].manager.Snapshot() != before || f.records[concurrentCollector].manager.PID() != pid {
		return errors.New("synthetic concurrent Chrome reader replaced active collector")
	}
	switch scenario {
	case "expiry":
		f.clockOffset.Add(int64(concurrentProofMaxAge + time.Second))
	case "opt-out":
		if err = f.c.Disable(ctx); err != nil {
			return err
		}
		if !f.records[concurrentCollector].bridge.cleanupVerified() || f.records[concurrentReader].bridge.cleanupVerified() || f.c.Snapshot().ReaderWindows != 2 || f.records[concurrentReader].manager.Snapshot().State != captureruntime.Ready {
			return errors.New("synthetic concurrent Chrome selective cleanup failed")
		}
		inventory, err := f.inventory(ctx)
		if err != nil || inventory.WindowCount != 2 {
			return errors.New("synthetic concurrent Chrome opt-out closed reader windows")
		}
	case "collector-loss":
		// Kill the exact owned bridge process, not an observer callback. Existing
		// bridge handling verifies all descendants before Manager receives Done.
		record := f.records[concurrentCollector]
		identity := record.nodeIdentity
		if err = verifyConcurrentChromeImage(identity, f.request.NodePath); err != nil {
			return err
		}
		h, err := windows.OpenProcess(windows.PROCESS_TERMINATE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, identity.PID)
		if err != nil {
			return err
		}
		actual, identityErr := processFixtureIdentityFromHandle(h)
		record.bridge.jobMu.Lock()
		member, memberErr := chromeFixtureJobMembership(h, record.bridge.job)
		record.bridge.jobMu.Unlock()
		if identityErr != nil || actual != identity || memberErr != nil || !member {
			windows.CloseHandle(h)
			return errors.New("synthetic concurrent Chrome bridge identity changed")
		}
		err = windows.TerminateProcess(h, 74)
		windows.CloseHandle(h)
		if err != nil {
			return err
		}
		select {
		case <-f.collectorExited:
		case <-ctx.Done():
			return errors.New("synthetic concurrent Chrome process exit observer missing")
		}
		inventory, err := f.inventory(ctx)
		if err != nil || inventory.WindowCount != 2 || f.c.Snapshot().AdmissionOpen {
			return errors.New("synthetic concurrent Chrome loss closed reader or admitted work")
		}
	case "parent-cancel":
		f.cancel()
		if err = f.c.WaitParentCleanup(ctx); err != nil {
			return err
		}
	default:
		return errors.New("synthetic concurrent Chrome scenario unavailable")
	}
	f.server.unblock()
	captureErr := <-done
	lease.Release()
	if scenario == "parent-cancel" && !errors.Is(captureErr, context.Canceled) {
		return errors.New("synthetic concurrent Chrome parent cancellation missed capture")
	}
	if f.c.Complete(ctx, ticket, "synthetic:late-chrome-result") == nil {
		return errors.New("synthetic concurrent Chrome fenced result accepted")
	}
	if _, ok := f.c.SinkResult(ticket.WorkID); ok {
		return errors.New("synthetic concurrent Chrome fenced sink mutated")
	}
	if f.open(ctx, 3, false) == nil {
		return errors.New("synthetic concurrent Chrome fenced reader admitted")
	}
	return nil
}

func (f *concurrentChromeFixture) duplicateGates(ctx context.Context) error {
	reader, collector := f.records[concurrentReader], f.records[concurrentCollector]
	if reader.process.Login(ctx) == nil {
		return errors.New("synthetic concurrent Chrome duplicate login admitted")
	}
	f.server.mu.Lock()
	token, logins := f.server.token, f.server.logins
	f.server.mu.Unlock()
	if collector.bridge.rpc(ctx, "seed_fixture", map[string]any{"owner": collector.process.owner, "value": token}, nil) == nil || logins != 1 {
		return errors.New("synthetic concurrent Chrome duplicate seed admitted")
	}
	for _, record := range []*concurrentChromeRecord{reader, collector} {
		evidence, err := record.process.Evidence(ctx)
		if err != nil || !evidence.Authenticated || evidence.AccountID != "alpha" || evidence.Revision != "fixture-1" {
			return errors.New("synthetic concurrent Chrome duplicate mutated session")
		}
	}
	return nil
}

func (f *concurrentChromeFixture) close() error {
	preexistingFailure := false
	if f.c != nil {
		f.c.mu.Lock()
		preexistingFailure = f.c.cleanupFailed
		f.c.mu.Unlock()
	}
	f.cancel()
	f.server.unblock()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var failures []error
	if f.loopDone != nil {
		select {
		case <-f.loopDone:
		case <-ctx.Done():
			failures = append(failures, errors.New("synthetic concurrent Chrome coordinator did not drain"))
		}
	}
	if f.captureDone != nil {
		select {
		case <-f.captureDone:
		case <-ctx.Done():
			failures = append(failures, errors.New("synthetic concurrent Chrome capture did not drain"))
		}
	}
	if f.heldLease != nil {
		f.heldLease.Release()
	}
	if f.c != nil {
		if err := f.c.WaitParentCleanup(ctx); err != nil {
			f.mu.Lock()
			stages := strings.Join(f.diagnostics, ",")
			f.mu.Unlock()
			failures = append(failures, fmt.Errorf("controller cleanup (preexistingFailure=%t collectorExitEvents=%d readerRetiringEvents=%d readerUnexpectedEvents=%d stages=%s): %w", preexistingFailure, f.collectorExitEvents.Load(), f.readerRetiringEvents.Load(), f.readerUnexpectedEvents.Load(), stages, err))
		}
	}
	// Constructor failures can leave a bridge not yet returned to the controller.
	// The adapter still owns it and must verify its full Job before deletion.
	f.mu.Lock()
	records := make([]*concurrentChromeRecord, 0, len(f.records))
	for _, record := range f.records {
		records = append(records, record)
	}
	f.mu.Unlock()
	for _, record := range records {
		if err := f.closeRecord(record); err != nil {
			failures = append(failures, err)
		}
		if !record.bridge.cleanupVerified() {
			failures = append(failures, errConcurrentCleanup)
		}
		if record.manager != nil && record.manager.Snapshot().ActiveLeases != 0 {
			failures = append(failures, errors.New("synthetic concurrent Chrome lease retained"))
		}
	}
	f.server.server.Close()
	if len(failures) > 0 {
		return errors.Join(failures...)
	}
	if err := removeConcurrentChromeRoot(f.request.Root, f.rootID); err != nil {
		return fmt.Errorf("profile root retirement: %w", err)
	}
	return nil
}

func removeConcurrentChromeRoot(root, expectedID string) error {
	if !filepath.IsAbs(root) || !concurrentChromeRootName.MatchString(filepath.Base(root)) {
		return errors.New("synthetic concurrent Chrome cleanup root name rejected")
	}
	project, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		return errors.New("synthetic concurrent Chrome cleanup project unavailable")
	}
	parent := filepath.Join(project, "runtime", "qa", "auth-session-sidecar")
	resolvedParent, err := filepath.EvalSymlinks(parent)
	if err != nil || !samePath(parent, resolvedParent) || !samePath(filepath.Dir(root), resolvedParent) {
		return errors.New("synthetic concurrent Chrome cleanup parent rejected")
	}
	actualID, err := processFixtureDirectoryID(root)
	if err != nil || expectedID == "" || actualID != expectedID {
		return errors.New("synthetic concurrent Chrome cleanup root identity changed")
	}
	if err := filepath.WalkDir(root, func(path string, _ os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			if os.IsNotExist(err) {
				return errors.New("synthetic concurrent Chrome cleanup child metadata missing")
			}
			if os.IsPermission(err) {
				return errors.New("synthetic concurrent Chrome cleanup child metadata denied")
			}
			return errors.New("synthetic concurrent Chrome cleanup child metadata unavailable")
		}
		if !samePath(resolved, path) {
			return errors.New("synthetic concurrent Chrome cleanup child alias rejected")
		}
		name, err := windows.UTF16PtrFromString(path)
		if err != nil {
			return err
		}
		attributes, err := windows.GetFileAttributes(name)
		if err != nil || attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
			return errors.New("synthetic concurrent Chrome cleanup child reparse or metadata unavailable")
		}
		return nil
	}); err != nil {
		return err
	}
	return os.RemoveAll(root)
}

func runConcurrentChromeParentDeath(request concurrentChromeInput, output io.Writer) error {
	rootID, err := processFixtureDirectoryID(request.Root)
	if err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	job, err := newChromeFixtureJob()
	if err != nil {
		return err
	}
	observer := &chromeFixtureBridge{job: job}
	verified := false
	defer func() {
		if !verified {
			_ = windows.TerminateJobObject(job, 1)
		}
		_ = windows.CloseHandle(job)
	}()
	cmd := exec.Command(executable, "--dev", "--dev-session-reuse", "synthetic-concurrent-chrome-fixture", "--config", filepath.Join(request.Root, "absent-config", "sidecar.json"), "--database", filepath.Join(request.Root, "absent-database", "sidecar.db"))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	cmd.Stderr = io.Discard
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err = cmd.Start(); err != nil {
		return err
	}
	waited := make(chan error, 1)
	go func() { waited <- cmd.Wait() }()
	defer func() {
		_ = stdin.Close()
		if !verified {
			_ = cmd.Process.Kill()
			select {
			case <-waited:
			case <-time.After(5 * time.Second):
			}
		}
	}()
	identity, err := processFixtureProcessIdentity(uint32(cmd.Process.Pid))
	if err != nil {
		return err
	}
	if err = verifyConcurrentChromeImage(identity, executable); err != nil {
		return err
	}
	if err = assignChromeFixtureProcess(job, identity.PID); err != nil {
		return errors.New("synthetic concurrent Chrome parent observer Job assignment failed")
	}
	pids, err := observer.jobPIDs()
	if err != nil || len(pids) != 1 || pids[0] != identity.PID {
		return errors.New("synthetic concurrent Chrome parent observer initial membership invalid")
	}
	request.Scenario = "death-child"
	if err = json.NewEncoder(stdin).Encode(request); err != nil {
		return err
	}
	_ = stdin.Close()
	ready := make(chan error, 1)
	go func() {
		var receipt struct {
			Stage           string `json:"stage"`
			LoginCount      int    `json:"loginCount"`
			SeedCount       int    `json:"seedCount"`
			OverlapVerified bool   `json:"overlapVerified"`
		}
		decoder := json.NewDecoder(io.LimitReader(stdout, 1024))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&receipt); err != nil || receipt.Stage != "owned-pair-ready" || receipt.LoginCount != 1 || receipt.SeedCount != 1 || !receipt.OverlapVerified {
			ready <- errors.New("synthetic concurrent Chrome parent child readiness rejected")
		} else {
			ready <- nil
		}
	}()
	select {
	case err = <-ready:
		if err != nil {
			return err
		}
	case <-time.After(30 * time.Second):
		return errors.New("synthetic concurrent Chrome parent child readiness timed out")
	}
	pids, err = observer.jobPIDs()
	if err != nil || len(pids) < 5 {
		return errors.New("synthetic concurrent Chrome parent child trees not observed")
	}
	// Pin every observed owned peer before killing its Sidecar parent. Empty
	// Job accounting can precede handle signalling; PID lookup after death is
	// neither identity nor process-completion evidence.
	var peerHandles []windows.Handle
	defer func() {
		for _, handle := range peerHandles {
			_ = windows.CloseHandle(handle)
		}
	}()
	for _, pid := range pids {
		peer, openErr := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, pid)
		if openErr != nil {
			return errors.New("synthetic concurrent Chrome parent peer handle unavailable")
		}
		peerHandles = append(peerHandles, peer)
		peerIdentity, identityErr := processFixtureIdentityFromHandle(peer)
		member, memberErr := chromeFixtureJobMembership(peer, job)
		if identityErr != nil || peerIdentity.PID != pid || peerIdentity.Created == "" || memberErr != nil || !member {
			return errors.New("synthetic concurrent Chrome parent peer identity/membership rejected")
		}
		buffer := make([]uint16, 32768)
		size := uint32(len(buffer))
		if err = windows.QueryFullProcessImageName(peer, 0, &buffer[0], &size); err != nil || !filepath.IsAbs(windows.UTF16ToString(buffer[:size])) {
			return errors.New("synthetic concurrent Chrome parent peer image unavailable")
		}
	}
	// Pin and recheck the exact Sidecar child. No outer Job termination occurs
	// before observing its independent kill-on-close inner Jobs become empty.
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_TERMINATE|windows.SYNCHRONIZE, false, identity.PID)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	actual, err := processFixtureIdentityFromHandle(h)
	member, memberErr := chromeFixtureJobMembership(h, job)
	if err != nil || actual != identity || memberErr != nil || !member {
		return errors.New("synthetic concurrent Chrome parent child identity changed")
	}
	if err = verifyConcurrentChromeImage(identity, executable); err != nil {
		return err
	}
	if err = windows.TerminateProcess(h, 75); err != nil {
		return err
	}
	select {
	case err = <-waited:
		if err == nil {
			return errors.New("synthetic concurrent Chrome parent child exited normally")
		}
	case <-time.After(10 * time.Second):
		return errors.New("synthetic concurrent Chrome parent child did not exit")
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		pids, err = observer.jobPIDs()
		if err == nil && len(pids) == 0 {
			verified = true
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if !verified {
		return errors.New("synthetic concurrent Chrome parent death left owned descendants; emergency cleanup is not acceptance")
	}
	settlementDeadline := time.Now().Add(concurrentCleanupTimeout)
	for _, handle := range peerHandles {
		remaining := time.Until(settlementDeadline)
		if remaining <= 0 {
			return errors.New("synthetic concurrent Chrome parent peer completion deadline exceeded")
		}
		status, waitErr := windows.WaitForSingleObject(handle, uint32(remaining/time.Millisecond))
		if waitErr != nil || status != windows.WAIT_OBJECT_0 {
			return errors.New("synthetic concurrent Chrome parent peer completion unverified")
		}
	}
	if err = removeConcurrentChromeRoot(request.Root, rootID); err != nil {
		return err
	}
	return json.NewEncoder(output).Encode(concurrentChromeReceipt{Provider: "synthetic-concurrent-chrome-fixture", Scenario: "parent-process-death", Passed: true, CleanupVerified: true, LoginCount: 1, SeedCount: 1, OverlapVerified: true})
}
