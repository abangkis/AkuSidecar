//go:build windows && auth_session_dev && auth_session_fixture

package engine

import (
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

// The bridge's generic code sanitizer is not a seed diagnostic allowlist.
// Reconstruct only exact combinations of reviewed closed categories.
func xManualRetainedSeedRPCDiagnostic(err error) string {
	if err == nil {
		return "phase=reply_decode reason=rejected write=unknown"
	}
	code := err.Error()
	for _, phase := range []string{"preconditions", "owner_before", "frame_before", "enrollment_evaluate", "enrollment_project", "frame_after", "cookie_read", "cookie_select", "seed_write", "owner_after"} {
		for _, reason := range []string{"disabled", "replay", "unavailable", "rejected", "candidate_absent", "bootstrap_missing", "bootstrap_invalid", "wrong_page", "inspection_failed", "document_changed", "ownership_changed", "deadline", "clock_regressed"} {
			for _, write := range []string{"not_attempted", "unknown", "acknowledged"} {
				if code == "manual_seed_"+phase+"_"+reason+"_"+write {
					return fmt.Sprintf("phase=%s reason=%s write=%s", phase, reason, write)
				}
			}
		}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "phase=rpc reason=deadline write=unknown"
	}
	return "phase=rpc reason=unavailable write=unknown"
}

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
	if seedErr := bridge.rpc(ctx, "manual_seed_bootstrap", map[string]any{"owner": readerProcess.owner}, &raw); seedErr != nil {
		clear(raw)
		t.Fatal("manual-retained-one-seed-rpc-rejected " + xManualRetainedSeedRPCDiagnostic(seedErr))
	}
	if xManualPilotDecode(raw, &seed) != nil {
		clear(raw)
		t.Fatal("manual-retained-one-seed-decode-rejected " + xManualRetainedSeedRPCDiagnostic(nil))
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

func TestXManualPilotRetainedSeedDiagnostics(t *testing.T) {
	for _, sample := range []struct {
		err  error
		want string
	}{
		{errors.New("manual_seed_enrollment_project_bootstrap_missing_not_attempted"), "phase=enrollment_project reason=bootstrap_missing write=not_attempted"},
		{errors.New("manual_seed_seed_write_unavailable_unknown"), "phase=seed_write reason=unavailable write=unknown"},
		{errors.New("manual_seed_owner_after_ownership_changed_acknowledged"), "phase=owner_after reason=ownership_changed write=acknowledged"},
		{errors.New("manual_seed_owner_after_PRIVATE_acknowledged"), "phase=rpc reason=unavailable write=unknown"},
		{errors.New("xnp_private_cookie_body"), "phase=rpc reason=unavailable write=unknown"},
		{context.DeadlineExceeded, "phase=rpc reason=deadline write=unknown"},
		{nil, "phase=reply_decode reason=rejected write=unknown"},
	} {
		if got := xManualRetainedSeedRPCDiagnostic(sample.err); got != sample.want {
			t.Fatal("closed seed diagnostic mismatch")
		}
	}
}

// Accountless Node-only regression uses the unchanged correlated bridge reply
// reader. It never constructs a Chrome Job, opens a profile or invokes a driver.
func TestXManualPilotRetainedSeedNodeDiagnostic(t *testing.T) {
	node, project := os.Getenv("AKU_CHROME_FIXTURE_NODE"), os.Getenv("AKU_NATIVE_MANUAL_PROJECT_ROOT")
	if !filepath.IsAbs(node) || !filepath.IsAbs(project) {
		t.Skip("accountless diagnostic bridge requires project helper")
	}
	const script = `import {pathToFileURL} from 'node:url';
import {createInterface} from 'node:readline';
const {createRetainedManualSetup}=await import(pathToFileURL(process.argv[1]));
const {executeManualReply}=await import(pathToFileURL(process.argv[2]));
const run=createRetainedManualSetup({enabled:true,transferAllowed:true,now:()=>0,
ownership:async()=>({readerPID:1,readerProfile:'offline-reader',collectorPID:2,collectorProfile:'offline-collector'}),
readEnrollment:async()=> '1908123456789012345',readAllowedCookies:async()=>[{},{}],
writeAllowedCookies:async()=>{throw new Error('PRIVATE_UNSAFE_CALLBACK');}});
console.log(JSON.stringify({ready:true}));
for await(const line of createInterface({input:process.stdin})){const q=JSON.parse(line);await executeManualReply(q,()=>run(),(id,result,code)=>console.log(JSON.stringify({id,ok:!code,...(code?{code}:{result})})));}
`
	cmd := exec.Command(node, "--input-type=module", "--eval", script, filepath.Join(project, "scripts", "auth-session-fixture", "x-manual-retained-setup.mjs"), filepath.Join(project, "scripts", "auth-session-fixture", "x-native-metadata-probe-driver.mjs"))
	cmd.Stderr = io.Discard
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal("offline diagnostic output unavailable")
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal("offline diagnostic input unavailable")
	}
	if cmd.Start() != nil {
		t.Fatal("offline diagnostic Node unavailable")
	}
	b := &chromeFixtureBridge{cmd: cmd, stdin: stdin, pending: map[int64]chan chromeFixtureReply{}, owners: map[int]*chromeFixtureProcess{}, closed: make(chan struct{}), nodeWaited: make(chan struct{})}
	ready := make(chan error, 1)
	go b.readOutput(stdout, ready)
	t.Cleanup(func() {
		_ = stdin.Close()
		select {
		case <-b.nodeWaited:
		case <-time.After(3 * time.Second):
			_ = cmd.Process.Kill()
			<-b.nodeWaited
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	select {
	case err = <-ready:
		if err != nil {
			t.Fatal("offline diagnostic handshake failed")
		}
	case <-ctx.Done():
		t.Fatal("offline diagnostic handshake timeout")
	}
	for _, want := range []string{"phase=seed_write reason=unavailable write=unknown", "phase=preconditions reason=replay write=unknown"} {
		err = b.rpc(ctx, "manual_seed_bootstrap", map[string]any{"owner": 1}, nil)
		if xManualRetainedSeedRPCDiagnostic(err) != want {
			t.Fatal("Node-to-Go closed seed diagnostic lost")
		}
	}
}
