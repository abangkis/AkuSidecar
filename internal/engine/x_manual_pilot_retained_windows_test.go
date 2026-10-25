//go:build windows && auth_session_dev && auth_session_fixture

package engine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/abangkis/AkuSidecar/internal/ownedrpc"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
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
	if strings.HasPrefix(code, "manual_seed_v2_") && len(code) <= 80 {
		parts := strings.SplitN(strings.TrimPrefix(code, "manual_seed_v2_"), "_", 4)
		if len(parts) == 4 {
			catalogs := [][]string{
				{"preconditions", "owner_before", "frame_before", "enrollment_evaluate", "enrollment_project", "enrollment_cleanup", "frame_after", "cookie_read", "cookie_select", "seed_write", "owner_after"},
				{"disabled", "replay", "unavailable", "rejected", "candidate_absent", "bootstrap_missing", "bootstrap_invalid", "wrong_page", "inspection_failed", "document_changed", "ownership_changed", "deadline", "clock_regressed"},
				{"not_attempted", "unknown", "acknowledged"},
			}
			indices := make([]int, 3)
			valid := true
			for i, part := range parts[:3] {
				n, parseErr := strconv.Atoi(part)
				if parseErr != nil || n < 0 || n >= len(catalogs[i]) || strconv.Itoa(n) != part {
					valid = false
					break
				}
				indices[i] = n
			}
			if valid {
				initial, ok := nativePilotInitialCode("manual_initial_" + parts[3])
				if ok && initial.trace != nil {
					return fmt.Sprintf("phase=%s reason=%s write=%s initial=(%s)", catalogs[0][indices[0]], catalogs[1][indices[1]], catalogs[2][indices[2]], initial.String())
				}
			}
		}
		return "phase=rpc reason=unavailable write=unknown"
	}
	for _, phase := range []string{"preconditions", "owner_before", "frame_before", "enrollment_evaluate", "enrollment_project", "enrollment_cleanup", "frame_after", "cookie_read", "cookie_select", "seed_write", "owner_after"} {
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
	runXManualRetainedQA(t, nil)
}

// Shared startup recipe; continuations never repeat enrollment or the seed.
type xManualRetainedReady struct {
	broker            *nativePilotBroker
	ports             *xManualPilotOwnerPorts
	reader, collector xManualPilotWindowsRole
	root              string
}

func runXManualRetainedQA(t *testing.T, capture func(context.Context, xManualRetainedReady)) {
	t.Helper()
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
	var ports *xManualPilotOwnerPorts
	broker, ports, _ = newXManualPilotOwnerCaptureBroker(ctx, xManualPilotOwnerConfig{enabled: true, pilot: nativePilotConfig{enabled: true, contract: nativePilotContract, limitationAcknowledged: true, binding: binding, attemptStarted: started}, policy: policy}, reader, collector)
	if broker == nil {
		t.Fatal("manual-retained-broker-unavailable")
	}
	d := broker.start(ctx)
	if !d.ManualEligible || d.OfflineValidated || d.CanAuthorize || d.NativeQualified || d.StorePermit {
		t.Fatalf("manual-retained-initial-rejected reason=%s detail=(%s)", d.Reason, broker.initialDiagnostic())
	}
	t.Log("manual-retained-startup-trace" + broker.startupTrace())
	if capture != nil {
		capture(ctx, xManualRetainedReady{broker: broker, ports: ports, reader: reader, collector: collector, root: caseRoot})
		return
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
	seedCacheCode := "manual_seed_v2_9_3_0_v6_0_8_7_0_19_0_0_0_0_0_8_6_20_1_3_2_1_4"
	seedCacheWant := "phase=seed_write reason=rejected write=not_attempted initial=(role=reader phase=bound_finish reason=observed_cookie_write kind=primary first=guard_request_unbound terminal=unknown early=unknown handoff=unknown cookie=unknown extra=unknown trigger=redirect provenance=redirect condition=response_cache epoch=before_seed disk_cache=true service_worker=false prefetch_cache=absent request_frame=matching header_reason=unknown inspect_stage=unknown inspect_condition=unknown prepare_stage=unknown prepare_condition=unknown domain=unknown)"
	for _, sample := range []struct {
		err  error
		want string
	}{
		{errors.New("manual_seed_enrollment_project_bootstrap_missing_not_attempted"), "phase=enrollment_project reason=bootstrap_missing write=not_attempted"},
		{errors.New("manual_seed_seed_write_unavailable_unknown"), "phase=seed_write reason=unavailable write=unknown"},
		{errors.New("manual_seed_owner_after_ownership_changed_acknowledged"), "phase=owner_after reason=ownership_changed write=acknowledged"},
		{errors.New("manual_seed_enrollment_cleanup_unavailable_acknowledged"), "phase=enrollment_cleanup reason=unavailable write=acknowledged"},
		{errors.New("manual_seed_owner_after_PRIVATE_acknowledged"), "phase=rpc reason=unavailable write=unknown"},
		{errors.New("manual_seed_v2_09_3_0_v5_0_8_7_0_19_0_0_0_0_0_8_6_20_1_3_2_1"), "phase=rpc reason=unavailable write=unknown"},
		{errors.New("manual_seed_v2_9_3_00_v5_0_8_7_0_19_0_0_0_0_0_8_6_20_1_3_2_1"), "phase=rpc reason=unavailable write=unknown"},
		{errors.New("manual_seed_v2_9_3_0_v5_0_8_7_0_19_0_0_0_0_0_8_6_20_1_3_2_1_extra"), "phase=rpc reason=unavailable write=unknown"},
		{errors.New("manual_seed_v2_9_3_0_v6_0_8_7_0_19_0_0_0_0_0_8_6_20_1_3_2_1_99"), "phase=rpc reason=unavailable write=unknown"},
		{errors.New(seedCacheCode), seedCacheWant},
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
const {startupFailure}=await import(pathToFileURL(process.argv[3]));
const run=createRetainedManualSetup({enabled:true,transferAllowed:true,now:()=>0,
ownership:async()=>({readerPID:1,readerProfile:'offline-reader',collectorPID:2,collectorProfile:'offline-collector'}),
readEnrollment:async()=> '1908123456789012345',readAllowedCookies:async()=>[{},{}],
assertBeforeWrite:()=>{throw startupFailure('reader','bound_finish','observed_cookie_write',false,undefined,{firstCause:'guard_header_classification',guardTrigger:'response',guardProvenance:'scoped',guardEpoch:'before_seed',guardHeaderReason:'attribute_unsupported'});},
writeAllowedCookies:async()=>{throw new Error('PRIVATE_UNSAFE_CALLBACK');}});
console.log(JSON.stringify({ready:true}));
for await(const line of createInterface({input:process.stdin})){const q=JSON.parse(line);await executeManualReply(q,()=>{if(q.op==='manual_acquire')throw startupFailure('collector','inspect_before','observed_cookie_write');return run();},(id,result,code)=>console.log(JSON.stringify({id,ok:!code,...(code?{code}:{result})})));}
`
	cmd := exec.Command(node, "--input-type=module", "--eval", script, filepath.Join(project, "scripts", "auth-session-fixture", "x-manual-retained-setup.mjs"), filepath.Join(project, "scripts", "auth-session-fixture", "x-native-metadata-probe-driver.mjs"), filepath.Join(project, "scripts", "auth-session-fixture", "x-manual-startup-diagnostic.mjs"))
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
	b := &chromeFixtureBridge{cmd: cmd, stdin: stdin, pipe: ownedrpc.New(stdin), owners: map[int]*chromeFixtureProcess{}, closed: make(chan struct{}), nodeWaited: make(chan struct{})}
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
	for _, want := range []string{"phase=seed_write reason=rejected write=not_attempted initial=(role=reader phase=bound_finish reason=observed_cookie_write kind=primary first=guard_header_classification terminal=unknown early=unknown handoff=unknown cookie=unknown extra=unknown trigger=response provenance=scoped condition=unknown epoch=before_seed disk_cache=unknown service_worker=unknown prefetch_cache=unknown request_frame=unknown header_reason=attribute_unsupported inspect_stage=unknown inspect_condition=unknown prepare_stage=unknown prepare_condition=unknown domain=header)", "phase=preconditions reason=replay write=not_attempted"} {
		err = b.rpc(ctx, "manual_seed_bootstrap", map[string]any{"owner": 1}, nil)
		if xManualRetainedSeedRPCDiagnostic(err) != want {
			t.Fatal("Node-to-Go closed seed diagnostic lost")
		}
	}
	err = b.rpc(ctx, "manual_acquire", map[string]any{"owner": 1}, nil)
	if d := nativePilotInitialOf(nativePilotInitialRPC(err.Error()), "unknown", "acquire_rpc"); d.role != "collector" || d.phase != "inspect_before" || d.reason != "observed_cookie_write" {
		t.Fatal("long bounded startup code lost in Node-to-Go bridge")
	}
}

// Only transport uses the native backend method. OS snapshots and provenance
// are explicitly invented/offline; no Windows Chrome identity is asserted.
type xManualInitialOfflineBackend struct {
	transport *xManualPilotWindowsBackend
	cores     map[string]nativePilotOwner
}

func (*xManualInitialOfflineBackend) provenance() nativePilotProvenance {
	return nativePilotOfflineOnly
}
func (*xManualInitialOfflineBackend) now() time.Time { return time.Now() }
func (p *xManualInitialOfflineBackend) snapshot(_ context.Context, role string) (nativePilotOwner, error) {
	return p.cores[role], nil
}
func (p *xManualInitialOfflineBackend) rpc(ctx context.Context, role, op string, fields map[string]any, out any) error {
	return p.transport.rpc(ctx, role, op, fields, out)
}
func (p *xManualInitialOfflineBackend) cleanup(ctx context.Context, role, handle string, preserve bool) error {
	return p.transport.cleanup(ctx, role, handle, preserve)
}

func TestXManualPilotRetainedTimingProgress(t *testing.T) {
	project, node := os.Getenv("AKU_NATIVE_MANUAL_PROJECT_ROOT"), os.Getenv("AKU_CHROME_FIXTURE_NODE")
	if !filepath.IsAbs(project) || !filepath.IsAbs(node) {
		t.Skip("accountless timing bridge requires helper")
	}
	const script = `import {pathToFileURL} from 'node:url';import {createInterface} from 'node:readline';
const {createStartupTiming}=await import(pathToFileURL(process.argv[1]));
const {startupFailure}=await import(pathToFileURL(process.argv[2]));
console.log(JSON.stringify({ready:true}));
for await(const line of createInterface({input:process.stdin})){
 const q=JSON.parse(line),report=result=>console.log(JSON.stringify({event:'manual_startup_progress',id:q.id,result}));
 if(q.op==='manual_acquire'){
  const timing=createStartupTiming(report);timing.step('settings_wait','network_failed');
  timing.failure(startupFailure(q.role,'settings','source_unavailable',false,undefined,{firstCause:'settings_capture'}));
 }else console.log(JSON.stringify({id:q.id,ok:true,result:{closed:true}}));
}`
	cmd := exec.Command(node, "--input-type=module", "--eval", script, filepath.Join(project, "scripts/auth-session-fixture/x-manual-startup-timing.mjs"), filepath.Join(project, "scripts/auth-session-fixture/x-manual-startup-diagnostic.mjs"))
	cmd.Stderr = io.Discard
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal("offline output unavailable")
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal("offline input unavailable")
	}
	if cmd.Start() != nil {
		t.Fatal("offline Node unavailable")
	}
	b := &chromeFixtureBridge{cmd: cmd, stdin: stdin, pipe: ownedrpc.New(stdin), owners: map[int]*chromeFixtureProcess{}, closed: make(chan struct{}), nodeWaited: make(chan struct{})}
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
	select {
	case err = <-ready:
		if err != nil {
			t.Fatal("offline handshake failed")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("offline handshake timed out")
	}
	ledger := &nativePilotTiming{started: time.Now(), now: time.Now}
	ctx, cancel := context.WithTimeout(context.WithValue(context.Background(), nativePilotTimingKey{}, ledger), 200*time.Millisecond)
	defer cancel()
	done := nativePilotTimed(ctx, "reader", "acquire_rpc")
	backend := &xManualPilotWindowsBackend{roles: map[string]xManualPilotWindowsRole{"reader": {bridge: b, protocolOwner: 1}}}
	err = backend.rpc(ctx, "reader", "manual_acquire", nil, nil)
	done()
	detail := nativePilotTimingFailure(ctx, err, "pair", "joint")
	snapshot := ledger.freeze()
	if detail.role != "reader" || detail.phase != "settings" || detail.reason != "source_unavailable" {
		t.Fatalf("progress cause lost at deadline: %v", detail)
	}
	if snapshot.roles[0].nodeOperation != nativePilotTimingIndex("settings_wait") || snapshot.roles[0].operation != nativePilotTimingIndex("acquire_rpc") {
		t.Fatal("pending operation lost")
	}
	if detail.trace == nil || detail.trace.firstCause != "settings_capture" || !strings.Contains(snapshot.String(), "reader_settings_wait=network_failed") {
		t.Fatal("settings terminal cause or closed prerequisite lost across Node transport")
	}
	if b.pipe.Pending() != 0 {
		t.Fatal("expired RPC kept observers")
	}
}

func TestXManualPilotRetainedStartupDiagnostic(t *testing.T) {
	for _, role := range []string{"reader", "collector", "peer-first", "trace-reader", "trace-collector", "trace-ambiguous", "trace-peer-first", "trace-priority", "trace-priority-collector", "trace-budget", "trace-unbound", "trace-response", "trace-metadata", "trace-completion", "trace-context-priority", "trace-guard-peer-first", "trace-budget-candidate", "trace-candidate-budget", "trace-redirect", "trace-reused", "trace-redirect-peer-first"} {
		t.Run(role, func(t *testing.T) {
			project, node := os.Getenv("AKU_NATIVE_MANUAL_PROJECT_ROOT"), os.Getenv("AKU_CHROME_FIXTURE_NODE")
			if !filepath.IsAbs(project) || !filepath.IsAbs(node) {
				t.Skip("accountless startup bridge requires helper")
			}
			started := time.Now()
			const script = `import {pathToFileURL} from 'node:url';import {createInterface} from 'node:readline';
const {offlineXManualOwnerPort}=await import(pathToFileURL(process.argv[1]));
const {executeManualReply}=await import(pathToFileURL(process.argv[2]));
const {startupFailure}=await import(pathToFileURL(process.argv[3]));
const {offlineFirstCausePair}=await import(pathToFileURL(process.argv[5]));
const fault=process.argv[4],guardFaults={'trace-priority':'guard-priority','trace-priority-collector':'guard-priority','trace-budget':'guard-budget','trace-unbound':'guard-unbound','trace-response':'guard-response','trace-metadata':'guard-metadata','trace-completion':'guard-completion','trace-context-priority':'context-priority','trace-guard-peer-first':'guard-priority','trace-budget-candidate':'budget-candidate','trace-candidate-budget':'candidate-budget','trace-redirect':'guard-redirect','trace-reused':'guard-reused','trace-redirect-peer-first':'guard-redirect'},traced=fault.startsWith('trace-'),trace=traced?offlineFirstCausePair({faultRole:['trace-collector','trace-priority-collector'].includes(fault)?'collector':'reader',fault:guardFaults[fault]??(fault==='trace-ambiguous'?'ambiguous':'cookie')}):null;
const port=trace?trace.port:offlineXManualOwnerPort({afterRelease:async role=>{if(role===fault)throw startupFailure(role,'settings','source_unavailable');}}),peerFirst=['peer-first','trace-peer-first','trace-guard-peer-first','trace-redirect-peer-first'].includes(fault);
let fired=false,releasePeer;const peerDone=new Promise(resolve=>{releasePeer=resolve;});
console.log(JSON.stringify({ready:true}));
const tasks=new Set();for await(const line of createInterface({input:process.stdin})){const q=JSON.parse(line),task=executeManualReply(q,async()=>{
if(peerFirst&&q.op==='manual_acquire'&&!fired){fired=true;if(trace){trace.fail('reader',fault==='trace-guard-peer-first'?'guard-priority':fault==='trace-redirect-peer-first'?'guard-redirect':'cookie');port.invalidate('collector');}else port.observe('reader',{sessionId:'offline-session-reader',method:'Runtime.executionContextDestroyed',params:{executionContextId:7}});}
if(peerFirst&&fired&&q.role==='reader'&&['manual_inspect','manual_acquire'].includes(q.op))await peerDone;
return port.execute(q);},(id,result,code)=>{console.log(JSON.stringify({id,ok:!code,...(code?{code}:{result})}));if(code&&q.role==='collector')releasePeer();});tasks.add(task);void task.finally(()=>tasks.delete(task));}await Promise.allSettled(tasks);`
			dir := filepath.Join(project, "scripts", "auth-session-fixture")
			cmd := exec.Command(node, "--input-type=module", "--eval", script, filepath.Join(dir, "x-native-manual-owner-offline.mjs"), filepath.Join(dir, "x-native-metadata-probe-driver.mjs"), filepath.Join(dir, "x-manual-startup-diagnostic.mjs"), role, filepath.Join(dir, "x-manual-startup-first-cause-offline.mjs"))
			cmd.Stderr = io.Discard
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal("offline startup output unavailable")
			}
			stdin, err := cmd.StdinPipe()
			if err != nil {
				t.Fatal("offline startup input unavailable")
			}
			if cmd.Start() != nil {
				t.Fatal("offline startup Node unavailable")
			}
			bridge := &chromeFixtureBridge{cmd: cmd, stdin: stdin, pipe: ownedrpc.New(stdin), owners: map[int]*chromeFixtureProcess{}, closed: make(chan struct{}), nodeWaited: make(chan struct{})}
			ready := make(chan error, 1)
			go bridge.readOutput(stdout, ready)
			t.Cleanup(func() {
				stdin.Close()
				select {
				case <-bridge.nodeWaited:
				case <-time.After(3 * time.Second):
					cmd.Process.Kill()
					<-bridge.nodeWaited
				}
			})
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			select {
			case err = <-ready:
				if err != nil {
					t.Fatal("offline startup handshake rejected")
				}
			case <-ctx.Done():
				t.Fatal("offline startup handshake timeout")
			}
			backend := &xManualInitialOfflineBackend{transport: &xManualPilotWindowsBackend{roles: map[string]xManualPilotWindowsRole{}}, cores: map[string]nativePilotOwner{}}
			binding := nativePilotBinding{account: "1908123456789012345", enrollment: "offline-enrollment", pair: "offline-pair", attempt: "offline-attempt", derivation: "offline-derivation", policy: 1, consent: 1}
			for i, r := range []string{"reader", "collector"} {
				core := nativePilotOwner{role: r, owner: strconv.Itoa(i + 1), profile: "offline-profile-" + r, directory: "offline-directory-" + r, image: "offline-image", creation: "offline-created-" + r, job: "offline-job", parent: "offline-parent", pid: i + 1, generation: 1}
				backend.cores[r] = core
				backend.transport.roles[r] = xManualPilotWindowsRole{role: r, bridge: bridge, protocolOwner: 1}
				selected := nativePilotOwner{role: r, profile: core.profile, parent: core.parent}
				if r == "reader" {
					binding.reader = selected
				} else {
					binding.collector = selected
				}
			}
			cfg := xManualPilotOwnerConfig{enabled: true, pilot: nativePilotConfig{enabled: true, contract: nativePilotContract, limitationAcknowledged: true, binding: binding, attemptStarted: started}, policy: func(context.Context) (nativePilotBrokerState, error) {
				return nativePilotBrokerState{account: binding.account, enrollment: binding.enrollment, pair: binding.pair, attempt: binding.attempt, derivation: binding.derivation, policy: 1, consent: 1, readerWindows: 1, completeInventory: true, parentAlive: true, channelHealthy: true, invalidationsReady: true, optedIn: true}, nil
			}}
			b, _ := newXManualPilotOwnerBrokerWithBackend(ctx, cfg, backend)
			if b == nil {
				t.Fatal("offline startup composition unavailable")
			}
			d := b.start(ctx)
			detail := b.initialDiagnostic()
			wantRole, wantPhase, wantReason := role, "settings", "source_unavailable"
			if role == "peer-first" {
				wantRole, wantPhase, wantReason = "reader", "joint", "scope_lost"
				if !detail.peer {
					t.Fatal("peer delivery marker lost")
				}
			}
			if strings.HasPrefix(role, "trace-") {
				wantRole, wantPhase, wantReason = "reader", "bound_finish", "observed_cookie_write"
				if role == "trace-collector" || role == "trace-priority-collector" {
					wantRole = "collector"
				}
				if detail.trace == nil {
					t.Fatalf("private trace lost across startup bridge: %s", detail)
				}
				wantFirst, wantCookie := "cookie_candidate", "candidate_observed"
				switch role {
				case "trace-ambiguous", "trace-priority", "trace-priority-collector", "trace-guard-peer-first":
					wantFirst, wantCookie = "guard_header_classification", "guard_header_classification"
				case "trace-budget", "trace-budget-candidate":
					wantFirst, wantCookie = "guard_request_budget", "guard_request_budget"
				case "trace-unbound", "trace-redirect", "trace-reused", "trace-redirect-peer-first":
					wantFirst, wantCookie, wantReason = "guard_request_unbound", "other_only", "scope_lost"
					if role != "trace-unbound" {
						wantCookie = "not_observed"
					}
				case "trace-response":
					wantFirst, wantCookie, wantReason = "guard_response_correlation", "other_only", "scope_lost"
				case "trace-metadata":
					wantFirst, wantCookie = "guard_metadata_shape", "guard_metadata_shape"
				case "trace-completion":
					wantFirst, wantCookie, wantReason = "generic_early", "other_only", "scope_lost"
				case "trace-context-priority":
					wantFirst, wantCookie, wantReason = "context_lost", "guard_header_classification", "scope_lost"
				}
				if detail.trace.firstCause != wantFirst {
					t.Fatalf("first cause lost: %s", detail)
				}
				wantTrigger, wantProvenance := "response", "scoped"
				wantCondition, wantEpoch := "unknown", "post_seed"
				switch role {
				case "trace-budget", "trace-budget-candidate":
					wantTrigger, wantProvenance = "request", "unknown"
				case "trace-unbound", "trace-redirect", "trace-redirect-peer-first", "trace-reused":
					wantTrigger, wantProvenance, wantEpoch = "request", "root_scope", "document_committed"
					wantCondition = "root_request_duplicate"
					if role == "trace-unbound" {
						wantCondition = "root_request_method"
					}
				case "trace-response":
					wantProvenance, wantEpoch = "root_scope", "document_committed"
					wantCondition = "root_response_frame_different"
				case "trace-completion":
					wantTrigger, wantProvenance, wantEpoch = "finish", "root_scope", "document_committed"
					wantCondition = "root_extra_missing"
				}
				if detail.trace.guardTrigger != wantTrigger || detail.trace.guardProvenance != wantProvenance || detail.trace.guardCondition != wantCondition || detail.trace.guardEpoch != wantEpoch || !strings.Contains(detail.String(), "trigger="+wantTrigger+" provenance="+wantProvenance+" condition="+wantCondition+" epoch="+wantEpoch) {
					t.Fatalf("immutable guard discriminator lost across Node/Windows RPC/Go formatter: %s", detail)
				}
				if role == "trace-peer-first" || role == "trace-guard-peer-first" || role == "trace-redirect-peer-first" {
					if !detail.peer || detail.trace.terminal != "unknown" {
						t.Fatal("peer trace invented terminal outcome")
					}
				} else if detail.trace.terminal != "scope_lost" || detail.trace.early != "evaluation_failed" || detail.trace.handoff != "rejected" || detail.trace.cookie != wantCookie {
					t.Fatalf("separate terminal summary lost: %s", detail)
				}
			}
			if d.Reason != "source-unavailable" || d.OfflineValidated || d.ManualEligible || d.CanAuthorize || d.NativeQualified || d.StorePermit || detail.role != wantRole || detail.phase != wantPhase || detail.reason != wantReason {
				t.Fatalf("closed startup diagnostic path lost: %s", detail)
			}
		})
	}
}

func TestXManualPilotRetainedTraceCodes(t *testing.T) {
	project, node := os.Getenv("AKU_NATIVE_MANUAL_PROJECT_ROOT"), os.Getenv("AKU_CHROME_FIXTURE_NODE")
	if !filepath.IsAbs(project) || !filepath.IsAbs(node) {
		t.Skip("accountless trace parity requires helper")
	}
	const script = `import{pathToFileURL}from'node:url';const m=await import(pathToFileURL(process.argv[1]));const catalogs=[m.STARTUP_ROLES,m.STARTUP_PHASES,m.STARTUP_REASONS,['primary','peer'],...Object.values(m.STARTUP_TRACE)],codes=[];for(const[key,values]of Object.entries(m.STARTUP_TRACE))for(const value of values)codes.push(m.startupRPCCode(m.startupFailure('collector','inspect_before','observed_cookie_write',true,undefined,{[key]:value})));const combined=Object.fromEntries(Object.entries(m.STARTUP_TRACE).map(([key,values])=>[key,values[0]]));combined.firstCause='guard_request_unbound';combined.guardTrigger='redirect';combined.guardProvenance='redirect';combined.guardCondition='response_cache';combined.guardEpoch='before_seed';combined.guardDiskCache='true';combined.guardServiceWorker='false';combined.guardPrefetchCache='absent';combined.guardRequestFrame='matching';const combinedCode=m.startupRPCCode(m.startupFailure('reader','bound_finish','observed_cookie_write',false,undefined,combined));const headerReasonCode=m.startupRPCCode(m.startupFailure('reader','bound_finish','observed_cookie_write',false,undefined,{firstCause:'guard_header_classification',guardHeaderReason:'attribute_unsupported'}));const maximum=Object.fromEntries(Object.entries(m.STARTUP_TRACE).map(([key,values])=>[key,values.at(-1)]));const maximumCode=m.startupRPCCode(m.startupFailure('reader','bound_finish','observed_cookie_write',false,undefined,maximum));const {ManualStartupCookieGuard:G}=await import(new URL('./x-manual-startup-cookie-guard.mjs',pathToFileURL(process.argv[1])));const rootCodes=['reader','collector'].map(role=>{const g=new G('private-frame','private-session'),l=m.startupFailureLatch();g.armReload('old');g.observe({sessionId:'private-session',method:'Network.requestWillBeSent',params:{requestId:'private-request',frameId:'private-frame',loaderId:'new',type:'Document',request:{method:'GET',url:'https://x.com/home'}}});g.observe({sessionId:'private-session',method:'Network.responseReceived',params:{requestId:'private-request',frameId:'private-frame',loaderId:'new',hasExtraInfo:false,response:{url:'https://x.com/home',status:200,headers:{},fromDiskCache:true,fromServiceWorker:false}}});const error=m.startupFailure(role,'bound_finish',g.failureReason(),false,undefined,{firstCause:g.rejection(),...g.trace()});l.record(error,role,'bound_finish');return m.startupRPCCode(error);});const domainCodes=[['root_control','root_control_restore'],['guard_request_budget','unknown'],['cookie_candidate','unknown'],['context_lost','unknown']].map(([firstCause,guardCondition])=>m.startupRPCCode(m.startupFailure('reader','bound_finish','scope_lost',false,undefined,{firstCause,guardCondition})));console.log(JSON.stringify({catalogs,codes,combinedCode,headerReasonCode,maximumCode,rootCodes,domainCodes}));`
	cmd := exec.Command(node, "--input-type=module", "--eval", script, filepath.Join(project, "scripts", "auth-session-fixture", "x-manual-startup-diagnostic.mjs"))
	cmd.Stderr = io.Discard
	raw, err := cmd.Output()
	// Aggregate catalog fixture only; each private RPC still has its strict 80-byte bound.
	if err != nil || len(raw) > 32768 {
		t.Fatal("closed trace parity unavailable")
	}
	defer clear(raw)
	var payload struct {
		Catalogs         [][]string
		Codes            []string
		CombinedCode     string
		HeaderReasonCode string
		MaximumCode      string
		RootCodes        []string
		DomainCodes      []string
	}
	if json.Unmarshal(raw, &payload) != nil {
		t.Fatal("closed trace parity malformed")
	}
	want := [][]string{nativePilotInitialRoles, nativePilotInitialPhases, nativePilotInitialReasons, {"primary", "peer"}}
	want = append(want, nativePilotTraceCatalogs...)
	if !reflect.DeepEqual(payload.Catalogs, want) || len(payload.Codes) == 0 {
		t.Fatal("Node/Go trace catalog order diverged")
	}
	legacy, ok := nativePilotInitialCode("manual_initial_v2_0_8_7_0_2_7_6_2_3_0")
	if !ok || legacy.trace.firstCause != "cookie_ambiguous" || legacy.trace.cookie != "ambiguous" || legacy.trace.guardTrigger != "unknown" || legacy.trace.guardProvenance != "unknown" || legacy.trace.guardCondition != "unknown" || legacy.trace.guardEpoch != "unknown" || legacy.trace.guardDiskCache != "unknown" || legacy.trace.guardServiceWorker != "unknown" || legacy.trace.guardPrefetchCache != "unknown" || legacy.trace.guardRequestFrame != "unknown" || legacy.trace.guardHeaderReason != "unknown" {
		t.Fatal("legacy V2 catalog index changed")
	}
	legacyV3, ok := nativePilotInitialCode("manual_initial_v3_0_0_0_0_0_0_0_0_0_0_8_8")
	if !ok || legacyV3.trace.guardTrigger != "redirect" || legacyV3.trace.guardProvenance != "root_scope" || legacyV3.trace.guardCondition != "unknown" || legacyV3.trace.guardEpoch != "unknown" || legacyV3.trace.guardDiskCache != "unknown" || legacyV3.trace.guardServiceWorker != "unknown" || legacyV3.trace.guardPrefetchCache != "unknown" || legacyV3.trace.guardRequestFrame != "unknown" || !strings.Contains(legacyV3.String(), "condition=unknown epoch=unknown") {
		t.Fatal("legacy V3 trace did not retain its fields and default appended fields")
	}
	legacyV4Code := "manual_initial_v4"
	for i := 0; i < 14; i++ {
		legacyV4Code += "_0"
	}
	legacyV4, ok := nativePilotInitialCode(legacyV4Code)
	if !ok || legacyV4.trace.guardCondition != "unknown" || legacyV4.trace.guardEpoch != "unknown" || legacyV4.trace.guardDiskCache != "unknown" || legacyV4.trace.guardServiceWorker != "unknown" || legacyV4.trace.guardPrefetchCache != "unknown" || legacyV4.trace.guardRequestFrame != "unknown" {
		t.Fatal("legacy V4 trace arity or appended cache defaults changed")
	}
	legacyV5Code := "manual_initial_v5"
	for i := 0; i < 17; i++ {
		legacyV5Code += "_0"
	}
	legacyV5, ok := nativePilotInitialCode(legacyV5Code)
	if !ok || legacyV5.trace.guardDiskCache != "unknown" || legacyV5.trace.guardServiceWorker != "unknown" || legacyV5.trace.guardPrefetchCache != "unknown" || legacyV5.trace.guardRequestFrame != "unknown" {
		t.Fatal("legacy V5 trace arity or appended request-frame default changed")
	}
	cursor := 0
	for field, values := range nativePilotTraceCatalogs {
		for _, wantValue := range values {
			if cursor >= len(payload.Codes) {
				t.Fatal("Node omitted a trace catalog wire value")
			}
			code := payload.Codes[cursor]
			detail, ok := nativePilotInitialCode(code)
			if !ok || len(code) > 80 || detail.trace == nil {
				t.Fatal("valid bounded trace code rejected")
			}
			for i, got := range detail.trace.values() {
				wantValueForField := "unknown"
				if i == field {
					wantValueForField = wantValue
				}
				if got != wantValueForField {
					t.Fatalf("Node/Go trace RPC roundtrip mismatch at field %d: got %q want %q", i, got, wantValueForField)
				}
			}
			cursor++
		}
	}
	if cursor != len(payload.Codes) {
		t.Fatal("Node emitted unexpected extra trace wire values")
	}
	combined, ok := nativePilotInitialCode(payload.CombinedCode)
	if !ok || len(payload.CombinedCode) > 80 || combined.trace.guardCondition != "response_cache" || combined.trace.guardEpoch != "before_seed" || combined.trace.guardDiskCache != "true" || combined.trace.guardServiceWorker != "false" || combined.trace.guardPrefetchCache != "absent" || combined.trace.guardRequestFrame != "matching" || !strings.Contains(combined.String(), "disk_cache=true service_worker=false prefetch_cache=absent request_frame=matching header_reason=unknown") {
		t.Fatal("Node-to-Go request-frame trace rendering diverged")
	}
	headerReason, ok := nativePilotInitialCode(payload.HeaderReasonCode)
	if !ok || len(payload.HeaderReasonCode) > 80 || headerReason.trace == nil || headerReason.trace.firstCause != "guard_header_classification" || headerReason.trace.guardHeaderReason != "attribute_unsupported" || !strings.Contains(headerReason.String(), "header_reason=attribute_unsupported") {
		t.Fatal("Node-branded startup failure did not carry its known header reason through RPC to Go")
	}
	if len(payload.RootCodes) != 2 {
		t.Fatal("missing root trace roundtrips")
	}
	for _, code := range payload.RootCodes {
		detail, ok := nativePilotInitialCode(code)
		if !ok || len(code) > 80 || detail.trace == nil || detail.trace.guardCondition != "root_response_disk_cache" || detail.trace.guardEpoch != "reload_armed" || detail.trace.guardRequestFrame != "matching" || detail.trace.guardDiskCache != "true" || detail.trace.guardServiceWorker != "false" || detail.trace.guardPrefetchCache != "absent" || !strings.Contains(detail.String(), "condition=root_response_disk_cache") || !strings.Contains(detail.String(), "domain=root_policy") || strings.Contains(detail.String(), "private") {
			t.Fatal("actual root trace did not survive Node-to-Go RPC/log projection")
		}
	}
	for i, domain := range []string{"control", "coverage", "header", "lifecycle"} {
		if len(payload.DomainCodes) != 4 {
			t.Fatal("missing domain roundtrip")
		}
		detail, ok := nativePilotInitialCode(payload.DomainCodes[i])
		if !ok || !strings.Contains(detail.String(), "domain="+domain) {
			t.Fatal("closed failure domain lost across Node/Go wire")
		}
	}
	maximum, ok := nativePilotInitialCode(payload.MaximumCode)
	if !ok || len(payload.MaximumCode) > 80 || maximum.trace == nil || maximum.trace.guardHeaderReason != "capacity" {
		t.Fatal("maximum V7 catalog indices exceeded the bounded wire contract")
	}
	for _, code := range []string{"manual_initial_v3_0_0_0_0_0_0_0_0_0_0", "manual_initial_v3_0_0_0_0_0_0_0_0_0_0_99_0", "manual_initial_v3_0_0_0_0_0_0_0_0_0_0_0_99", "manual_initial_v3_0_0_0_0_0_0_0_0_0_0_00_0", "manual_initial_v3_0_0_0_0_0_0_0_0_0_0_0_0_extra"} {
		if _, ok := nativePilotInitialCode(code); ok {
			t.Fatal("noncanonical V3 guard trace accepted")
		}
	}
	for _, code := range []string{"manual_initial_v4_0_0_0_0_0_0_0_0_0_0_0_0_0", "manual_initial_v4_0_0_0_0_0_0_0_0_0_0_0_0_0_99", "manual_initial_v4_0_0_0_0_0_0_0_0_0_0_0_0_0_00", "manual_initial_v4_0_0_0_0_0_0_0_0_0_0_0_0_0_0_extra"} {
		if _, ok := nativePilotInitialCode(code); ok {
			t.Fatal("noncanonical V4 guard trace accepted")
		}
	}
	for _, code := range []string{"manual_initial_v5_0_0_0_0_0_0_0_0_0_0_0_0_0_0_0_0", "manual_initial_v5_0_0_0_0_0_0_0_0_0_0_0_0_0_0_0_0_99", "manual_initial_v5_0_0_0_0_0_0_0_0_0_0_0_0_0_0_0_00", "manual_initial_v5_0_0_0_0_0_0_0_0_0_0_0_0_0_0_0_0_extra", "manual_initial_v5_0_0_0_0_0_0_0_0_0_0_0_0_0_0_0_0_0" + strings.Repeat("0", 80)} {
		if _, ok := nativePilotInitialCode(code); ok {
			t.Fatal("noncanonical V5 cache trace accepted")
		}
	}
	validV6 := "manual_initial_v6"
	for i := 0; i < 18; i++ {
		validV6 += "_0"
	}
	legacyV6, ok := nativePilotInitialCode(validV6)
	if !ok || legacyV6.trace.guardHeaderReason != "unknown" || !strings.Contains(legacyV6.String(), "request_frame=unknown header_reason=unknown") {
		t.Fatal("legacy V6 trace did not retain its exact arity with an unknown appended header reason")
	}
	v6WithoutLast := validV6[:strings.LastIndex(validV6, "_")]
	v6WithBadFinal := strings.TrimSuffix(validV6, "_0") + "_99"
	v6WithNoncanonicalFinal := strings.TrimSuffix(validV6, "_0") + "_00"
	v6WithNoncanonicalHead := strings.Replace(validV6, "_0_", "_00_", 1)
	for _, code := range []string{v6WithoutLast, validV6 + "_0", v6WithBadFinal, v6WithNoncanonicalFinal, v6WithNoncanonicalHead, validV6 + strings.Repeat("0", 80)} {
		if _, ok := nativePilotInitialCode(code); ok {
			t.Fatal("noncanonical or unbounded V6 request-frame trace accepted")
		}
	}
	validV7 := "manual_initial_v7"
	for i := 0; i < 19; i++ {
		validV7 += "_0"
	}
	if detail, ok := nativePilotInitialCode(validV7); !ok || detail.trace == nil || detail.trace.guardHeaderReason != "unknown" {
		t.Fatal("valid V7 trace arity rejected or appended header reason changed")
	}
	v7WithoutLast := validV7[:strings.LastIndex(validV7, "_")]
	v7WithExtra := validV7 + "_0"
	v7WithBadFinal := strings.TrimSuffix(validV7, "_0") + "_9"
	v7WithNoncanonicalFinal := strings.TrimSuffix(validV7, "_0") + "_00"
	v7WithNoncanonicalHead := strings.Replace(validV7, "_0_", "_00_", 1)
	for _, code := range []string{v7WithoutLast, v7WithExtra, v7WithBadFinal, v7WithNoncanonicalFinal, v7WithNoncanonicalHead, validV7 + strings.Repeat("0", 80)} {
		if _, ok := nativePilotInitialCode(code); ok {
			t.Fatal("noncanonical or unbounded V7 header-reason trace accepted")
		}
	}
	for _, code := range []string{"manual_initial_v2_0_0_0_0_0_0_0_0_0", "manual_initial_v2_00_0_0_0_0_0_0_0_0_0", "manual_initial_v2_0_0_0_2_0_0_0_0_0_0", "manual_initial_v2_0_0_0_0_-1_0_0_0_0_0", "manual_initial_v2_0_0_0_0_0_99_0_0_0_0", "manual_initial_v2_0_0_0_0_0_0_0_0_0_0_extra", "manual_initial_v2_0_0_0_0_0_0_0_0_0_0 ", "manual_initial_v2_+0_0_0_0_0_0_0_0_0_0", "manual_initial_v2_0_0_0_0_0_0_0_0_0_0" + strings.Repeat("0", 80)} {
		if _, ok := nativePilotInitialCode(code); ok {
			t.Fatal("noncanonical or unbounded trace accepted")
		}
	}
	unsafe := nativePilotInitialDetail{role: "reader", phase: "identity", reason: "rejected", trace: &nativePilotInitialTrace{firstCause: "PRIVATE", terminal: strings.Repeat("x", 20000)}}
	if strings.Contains(fmt.Sprintf("%#v", unsafe), "PRIVATE") || strings.Contains(unsafe.String(), strings.Repeat("x", 20)) {
		t.Fatal("malformed typed trace escaped closed formatter")
	}
}
