//go:build windows && auth_session_dev && auth_session_fixture

package engine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// Preparation compiles this entry point but never sets either execution gate.
// The additional Node gates are checked before a retained lease is acquired.
func xManualRetainedExecutionEnabled() bool {
	for _, name := range []string{"AKU_X_MANUAL_RETAINED_QA_ENABLED", "AKU_X_MANUAL_RETAINED_TRANSFER_ONCE", "AKU_X_QA_SHORT_TRANSFER_ALLOWED", "AKU_X_NATIVE_MANUAL_STARTUP_ALLOWED"} {
		if os.Getenv(name) != "1" {
			return false
		}
	}
	return true
}

// The general lease may create a new profile. This entry point instead takes
// the same exclusive lock and requires an already clean, physically pinned
// profile; there is no missing-manifest fallback or profile creation branch.
func acquireXManualExistingRetained(project string) (*xRetainedQAProfile, error) {
	canonical, err := xRetainedQAProjectRoot()
	if err != nil || !filepath.IsAbs(project) || !samePath(project, canonical) {
		return nil, errors.New("manual-retained-project-unavailable")
	}
	parent, parentID, err := xRetainedOrdinaryDirectory(filepath.Join(canonical, "runtime", "qa", "auth-session-sidecar"))
	if err != nil {
		return nil, errors.New("manual-retained-parent-unavailable")
	}
	manifestPath := filepath.Join(parent, xRetainedQAProfileManifestName)
	lock, err := xRetainedOpenExclusiveLock(manifestPath + ".lock")
	if err != nil {
		return nil, errors.New("manual-retained-lease-unavailable")
	}
	p := &xRetainedQAProfile{parent: parent, parentID: parentID, manifestPath: manifestPath, lockPath: manifestPath + ".lock", lock: lock}
	ready := false
	defer func() {
		if !ready {
			p.xRetainedCloseHandles()
		}
	}()
	p.parentPin, err = pinXTwoPhaseDirectory(parent)
	if err != nil {
		return nil, errors.New("manual-retained-parent-unavailable")
	}
	manifest, err := xRetainedReadManifest(manifestPath)
	if err != nil || manifest.State != "clean" || manifest.ProfileID == "" {
		return nil, errors.New("manual-retained-existing-profile-required")
	}
	p.Root, p.RootID, p.ProfileID, p.Existing = filepath.Join(parent, manifest.Root), manifest.RootID, manifest.ProfileID, true
	if !xRetainedValidRootPath(parent, p.Root) {
		return nil, errors.New("manual-retained-profile-scope-invalid")
	}
	p.rootPin, err = pinXTwoPhaseDirectory(p.Root)
	if err != nil {
		return nil, errors.New("manual-retained-root-unavailable")
	}
	p.profilePin, err = pinXTwoPhaseDirectory(filepath.Join(p.Root, "shared"))
	if err != nil || p.xRetainedValidateHeldIDs() != nil {
		return nil, errors.New("manual-retained-profile-unavailable")
	}
	var nonce [32]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		return nil, errors.New("manual-retained-scope-unavailable")
	}
	p.Nonce = hex.EncodeToString(nonce[:])
	if p.xRetainedPersist("active") != nil {
		return nil, errors.New("manual-retained-lease-unavailable")
	}
	ready = true
	return p, nil
}

type xManualRetainedSeed struct {
	AccountID        string `json:"accountId"`
	ReaderPID        uint32 `json:"readerPID"`
	ReaderProfile    string `json:"readerProfile"`
	CollectorPID     uint32 `json:"collectorPID"`
	CollectorProfile string `json:"collectorProfile"`
	Seeded           *bool  `json:"seeded"`
	HomeReady        *bool  `json:"homeReady"`
}

func (xManualRetainedSeed) String() string     { return "private-retained-seed-candidate" }
func (v xManualRetainedSeed) GoString() string { return v.String() }

func xManualRetainedRole(bridge *chromeFixtureBridge, parent, node processFixtureIdentity, role, root, profile string, pid uint32, owner int) (xManualPilotWindowsRole, error) {
	r := xManualPilotWindowsRole{role: role, bridge: bridge, protocolOwner: owner, node: node, parent: parent, root: root, profile: profile, generation: 1}
	if pid == 0 || pid == node.PID || bridge.containsPID(pid) != nil {
		return r, errors.New("manual-retained-process-unavailable")
	}
	var err error
	r.chromeHandle, err = windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, pid)
	if err != nil {
		return r, errors.New("manual-retained-process-unavailable")
	}
	valid := false
	defer func() {
		if !valid {
			xManualRetainedCloseRole(r)
		}
	}()
	r.chrome, err = processFixtureIdentityFromHandle(r.chromeHandle)
	if err != nil {
		return r, errors.New("manual-retained-process-unavailable")
	}
	r.rootPin, err = pinXTwoPhaseDirectory(root)
	if err == nil {
		r.profilePin, err = pinXTwoPhaseDirectory(profile)
	}
	if err != nil {
		return r, errors.New("manual-retained-directory-unavailable")
	}
	r.rootID, err = processFixtureDirectoryID(root)
	if err == nil {
		r.profileID, err = processFixtureDirectoryID(profile)
	}
	backend := &xManualPilotWindowsBackend{roles: map[string]xManualPilotWindowsRole{role: r}}
	if err != nil {
		return r, errors.New("manual-retained-directory-unavailable")
	}
	if _, err = backend.snapshot(context.Background(), role); err != nil {
		return r, errors.New("manual-retained-ownership-unavailable")
	}
	if role == "reader" && len(processFixtureWindows(pid)) < 1 {
		return r, errors.New("manual-retained-visible-reader-unavailable")
	}
	valid = true
	return r, nil
}

func xManualRetainedCloseRole(r xManualPilotWindowsRole) {
	for _, handle := range []windows.Handle{r.profilePin, r.rootPin, r.chromeHandle} {
		if handle != 0 {
			_ = windows.CloseHandle(handle)
		}
	}
}

func TestXManualPilotRetainedQA(t *testing.T) {
	if !xManualRetainedExecutionEnabled() {
		t.Skip("retained manual QA requires separate execution and one-transfer authorization")
	}
	project := os.Getenv("AKU_NATIVE_MANUAL_PROJECT_ROOT")
	caseRoot := os.Getenv("AKU_CHROME_FIXTURE_ROOT")
	marker := os.Getenv("AKU_CHROME_FIXTURE_CLEANUP_MARKER")
	if !filepath.IsAbs(project) || !filepath.IsAbs(caseRoot) || !samePath(filepath.Dir(caseRoot), filepath.Join(project, "runtime", "qa", "auth-session-sidecar")) || !samePath(filepath.Dir(marker), caseRoot) {
		t.Fatal("manual-retained-helper-scope-unavailable")
	}
	for _, path := range []string{os.Getenv("AKU_CHROME_FIXTURE_NODE"), os.Getenv("AKU_CHROME_FIXTURE_EXE"), filepath.Join(project, "scripts", "auth-session-fixture", "x-retained-source-qualification-driver.mjs")} {
		info, err := os.Stat(path)
		if !filepath.IsAbs(path) || err != nil || !info.Mode().IsRegular() {
			t.Fatal("manual-retained-executable-unavailable")
		}
	}
	prior, err := os.Getwd()
	if err != nil || os.Chdir(filepath.Join(project, "internal", "engine")) != nil {
		t.Fatal("manual-retained-workspace-unavailable")
	}
	defer os.Chdir(prior)
	// The immutable attempt horizon precedes the lease, preparation and seed.
	started := time.Now()
	ctx, cancel := context.WithDeadline(context.Background(), started.Add(nativePilotLife))
	defer cancel()
	lease, err := acquireXManualExistingRetained(project)
	if err != nil {
		t.Fatal("manual-retained-existing-lease-unavailable")
	}
	clean := true // No process exists until bridge construction succeeds.
	t.Cleanup(func() {
		if lease.Release(clean) != nil {
			t.Error("manual-retained-lease-release-unverified")
			return
		}
		if clean && os.WriteFile(marker, []byte("verified\n"), 0600) != nil {
			t.Error("manual-retained-cleanup-marker-unavailable")
		}
	})
	server := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(server.Close)
	config := chromeFixtureConfig{NodePath: os.Getenv("AKU_CHROME_FIXTURE_NODE"), ChromePath: os.Getenv("AKU_CHROME_FIXTURE_EXE"), Root: lease.Root, Origin: server.URL, DriverPath: filepath.Join(project, "scripts", "auth-session-fixture", "x-retained-source-qualification-driver.mjs"), qaProfileLease: lease}
	clean = false
	bridge, err := startChromeFixtureBridge(ctx, config)
	if err != nil {
		t.Fatal("manual-retained-bridge-unavailable")
	}
	var broker *nativePilotBroker
	var roles []xManualPilotWindowsRole
	protocolOwner := 0
	t.Cleanup(func() {
		if broker != nil {
			broker.invalidate("parent-exited")
		}
		cleanupCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if protocolOwner > 0 {
			_ = bridge.rpc(cleanupCtx, "release", map[string]any{"owner": protocolOwner}, nil)
		}
		bridge.terminateOwnedTree()
		select {
		case <-bridge.nodeWaited:
		case <-time.After(5 * time.Second):
			t.Error("manual-retained-node-exit-unverified")
		}
		clean = bridge.cleanupError() == nil && bridge.cleanupVerified()
		for _, r := range roles {
			xManualRetainedCloseRole(r)
		}
		if !clean {
			t.Error("manual-retained-job-cleanup-unverified")
		}
	})
	if bridge.assertBridgeOnly(ctx) != nil {
		t.Fatal("manual-retained-prelaunch-job-invalid")
	}
	readerProcess, err := bridge.launch(ctx, "collector")
	if err != nil {
		t.Fatal("manual-retained-reader-launch-failed")
	}
	protocolOwner = readerProcess.owner
	var home struct {
		Ready *bool `json:"ready"`
	}
	var raw json.RawMessage
	// Target creation can precede Home hydration. Poll only the existing UI
	// readiness predicate; this never retries login, seed or startup evidence.
	homeDeadline := time.Now().Add(15 * time.Second)
	for {
		if bridge.rpc(ctx, "login_state", map[string]any{"owner": readerProcess.owner}, &raw) != nil || xManualPilotDecode(raw, &home) != nil || home.Ready == nil {
			clear(raw)
			t.Fatal("manual-retained-home-check-unavailable")
		}
		clear(raw)
		if *home.Ready {
			break
		}
		if !time.Now().Before(homeDeadline) {
			t.Fatal("manual-retained-home-unavailable-no-login-attempted")
		}
		select {
		case <-ctx.Done():
			t.Fatal("manual-retained-attempt-expired-before-home")
		case <-time.After(250 * time.Millisecond):
		}
	}
	var preparation struct {
		CollectorPID    uint32 `json:"collectorPID"`
		Headless        *bool  `json:"headless"`
		IsolatedContext *bool  `json:"isolatedContext"`
		DistinctProfile *bool  `json:"distinctProfile"`
		SeedAttempted   *bool  `json:"seedAttempted"`
	}
	if bridge.rpc(ctx, "prepare_short_concurrency", map[string]any{"owner": readerProcess.owner}, &raw) != nil || xManualPilotDecode(raw, &preparation) != nil || preparation.CollectorPID == 0 || preparation.Headless == nil || !*preparation.Headless || preparation.IsolatedContext == nil || !*preparation.IsolatedContext || preparation.DistinctProfile == nil || !*preparation.DistinctProfile || preparation.SeedAttempted == nil || *preparation.SeedAttempted {
		clear(raw)
		t.Fatal("manual-retained-collector-prepare-failed")
	}
	clear(raw)
	parent, err := processFixtureProcessIdentity(uint32(os.Getpid()))
	if err != nil {
		t.Fatal("manual-retained-parent-unavailable")
	}
	node, err := processFixtureProcessIdentity(bridge.pid)
	if err != nil {
		t.Fatal("manual-retained-node-unavailable")
	}
	reader, err := xManualRetainedRole(bridge, parent, node, "reader", lease.Root, filepath.Join(lease.Root, "shared"), readerProcess.pid, readerProcess.owner)
	if err != nil {
		t.Fatal("manual-retained-reader-ownership-invalid")
	}
	roles = append(roles, reader)
	collector, err := xManualRetainedRole(bridge, parent, node, "collector", caseRoot, filepath.Join(caseRoot, "derived-headless"), preparation.CollectorPID, readerProcess.owner)
	if err != nil {
		t.Fatal("manual-retained-collector-ownership-invalid")
	}
	roles = append(roles, collector)
	var seed xManualRetainedSeed
	if bridge.rpc(ctx, "manual_seed_bootstrap", map[string]any{"owner": readerProcess.owner}, &raw) != nil || xManualPilotDecode(raw, &seed) != nil {
		clear(raw)
		t.Fatal("manual-retained-one-seed-unavailable")
	}
	clear(raw)
	if !nativePilotAccount(seed.AccountID) || seed.Seeded == nil || !*seed.Seeded || seed.HomeReady == nil || !*seed.HomeReady || seed.ReaderPID != readerProcess.pid || seed.CollectorPID != preparation.CollectorPID || seed.CollectorPID == seed.ReaderPID || !samePath(seed.ReaderProfile, filepath.Join(lease.Root, "shared")) || !samePath(seed.CollectorProfile, filepath.Join(caseRoot, "derived-headless")) {
		t.Fatal("manual-retained-private-seed-scope-invalid")
	}
	var random [24]byte
	if _, err = rand.Read(random[:]); err != nil {
		t.Fatal("manual-retained-attempt-scope-unavailable")
	}
	nonce := hex.EncodeToString(random[:])
	parentRef := strconv.FormatUint(uint64(parent.PID), 10) + "/" + parent.Created
	binding := nativePilotBinding{account: seed.AccountID, enrollment: nonce + "e", pair: nonce + "p", attempt: nonce + "a", derivation: nonce + "d", policy: 1, consent: 1, reader: nativePilotOwner{role: "reader", profile: reader.profile, parent: parentRef}, collector: nativePilotOwner{role: "collector", profile: collector.profile, parent: parentRef}}
	policy := func(call context.Context) (nativePilotBrokerState, error) {
		return nativePilotBrokerState{account: binding.account, enrollment: binding.enrollment, pair: binding.pair, attempt: binding.attempt, derivation: binding.derivation, policy: binding.policy, consent: binding.consent, readerWindows: 1, completeInventory: true, parentAlive: ctx.Err() == nil, channelHealthy: !bridge.isClosed(), invalidationsReady: true, optedIn: true}, call.Err()
	}
	broker, _ = newXManualPilotOwnerBroker(ctx, xManualPilotOwnerConfig{enabled: true, pilot: nativePilotConfig{enabled: true, contract: nativePilotContract, limitationAcknowledged: true, binding: binding, attemptStarted: started}, policy: policy}, reader, collector)
	if broker == nil {
		t.Fatal("manual-retained-broker-unavailable")
	}
	d := broker.start(ctx)
	if !d.ManualEligible || d.OfflineValidated || d.CanAuthorize || d.NativeQualified || d.StorePermit {
		t.Fatalf("manual-retained-initial-rejected reason=%s", d.Reason)
	}
	var frozenCollector nativePilotPreparedRole
	for _, prepared := range broker.prepared {
		if prepared.owner.role == "collector" {
			frozenCollector = prepared
		}
	}
	if actual, err := broker.ports.inspect(ctx, frozenCollector); err != nil || actual != frozenCollector.owner {
		t.Fatal("manual-retained-collector-before-open-invalid")
	}
	admitted := 0
	// The atomic controller callback commits a local QA reservation only. The
	// subsequent OS action is checked after completion; this is not a production
	// atomic OS broker, and any failed or ambiguous action retires the QA owners.
	if d = broker.openReader(ctx, func() bool { admitted++; return true }); d.Reason != "reader-admitted" || admitted != 1 {
		t.Fatal("manual-retained-reader-admission-failed")
	}
	var opened struct {
		Opened         *bool `json:"opened"`
		WindowCount    int   `json:"windowCount"`
		BootstrapCount int   `json:"bootstrapCount"`
	}
	var reply json.RawMessage
	openErr := bridge.rpc(ctx, "manual_open_reader", map[string]any{"owner": readerProcess.owner}, &reply)
	decodeErr := xManualPilotDecode(reply, &opened)
	clear(reply)
	if openErr != nil || decodeErr != nil || opened.Opened == nil || !*opened.Opened || opened.WindowCount < 2 || opened.WindowCount > 16 || opened.BootstrapCount != 1 || len(processFixtureWindows(reader.chrome.PID)) < 2 {
		t.Fatal("manual-retained-reserved-reader-action-failed")
	}
	if d = broker.check(ctx); !d.ManualEligible || d.CanAuthorize || d.NativeQualified || d.StorePermit {
		t.Fatal("manual-retained-reserved-reader-postcheck-failed")
	}
	if actual, err := broker.ports.inspect(ctx, frozenCollector); err != nil || actual != frozenCollector.owner {
		t.Fatal("manual-retained-collector-after-open-invalid")
	}
	ticket, d := broker.dispatch(ctx, "one-private-memory-result")
	if ticket == nil || !d.ManualEligible {
		t.Fatal("manual-retained-dispatch-failed")
	}
	pinnedCollector := ticket.collector
	if broker.claim(ctx, ticket, func() bool { return true }).Reason != "work-claimed" {
		t.Fatal("manual-retained-claim-failed")
	}
	results := 0
	if broker.complete(ctx, ticket, func() bool { results++; return true }).Reason != "result-completed" {
		t.Fatal("manual-retained-result-failed")
	}
	if broker.complete(ctx, ticket, func() bool { results++; return true }).Reason != "result-duplicate" || results != 1 || ticket.collector != pinnedCollector {
		t.Fatal("manual-retained-result-fence-failed")
	}
	t.Log("private retained manual QA completed one in-memory result; public authority remains false")
}
