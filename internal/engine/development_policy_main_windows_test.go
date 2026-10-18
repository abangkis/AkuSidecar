//go:build windows && auth_session_dev && auth_session_fixture

package engine

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/readerbroker"
	"github.com/abangkis/AkuSidecar/internal/store"
	"golang.org/x/sys/windows"
)

type developmentPolicyPinnedProcess struct {
	identity processFixtureIdentity
	image    string
	handle   windows.Handle
}

func pinDevelopmentPolicyProcess(t *testing.T, pid uint32, expectedImage string) developmentPolicyPinnedProcess {
	t.Helper()
	identity, err := processFixtureProcessIdentity(pid)
	if err != nil {
		t.Fatalf("pin synthetic policy process identity: %v", err)
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, pid)
	if err != nil {
		t.Fatalf("pin synthetic policy process handle: %v", err)
	}
	t.Cleanup(func() { _ = windows.CloseHandle(handle) })
	buffer := make([]uint16, 32768)
	size := uint32(len(buffer))
	if err := windows.QueryFullProcessImageName(handle, 0, &buffer[0], &size); err != nil {
		t.Fatal("synthetic policy process image unavailable")
	}
	image := windows.UTF16ToString(buffer[:size])
	if expectedImage != "" && !samePath(image, expectedImage) {
		t.Fatalf("synthetic policy process image mismatch: got=%q want=%q", image, expectedImage)
	}
	return developmentPolicyPinnedProcess{identity: identity, image: image, handle: handle}
}

func findDevelopmentPolicyNodePID(pids []uint32, workerPID uint32, nodeImage string) (uint32, error) {
	for _, pid := range pids {
		if pid == workerPID {
			continue
		}
		handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
		if err != nil {
			continue // A short-lived utility process may exit during inventory.
		}
		buffer := make([]uint16, 32768)
		size := uint32(len(buffer))
		queryErr := windows.QueryFullProcessImageName(handle, 0, &buffer[0], &size)
		image := windows.UTF16ToString(buffer[:size])
		_ = windows.CloseHandle(handle)
		if queryErr == nil && samePath(image, nodeImage) {
			return pid, nil
		}
	}
	return 0, fmt.Errorf("Node bridge image absent from outer Job")
}

func policyProcessMustRemainLive(t *testing.T, process developmentPolicyPinnedProcess) {
	t.Helper()
	status, err := windows.WaitForSingleObject(process.handle, 0)
	if err != nil || status != uint32(windows.WAIT_TIMEOUT) {
		t.Fatalf("pinned process exited before policy drain: pid=%d image=%q status=%d err=%v", process.identity.PID, process.image, status, err)
	}
}

func policyProcessMustSignal(t *testing.T, process developmentPolicyPinnedProcess) {
	t.Helper()
	status, err := windows.WaitForSingleObject(process.handle, 15000)
	if err != nil || status != windows.WAIT_OBJECT_0 {
		t.Fatalf("pinned process did not signal after policy cleanup: pid=%d image=%q status=%d err=%v", process.identity.PID, process.image, status, err)
	}
}

func readDevelopmentPolicyCommand(t *testing.T, ctx context.Context, path, commandID string) developmentQueuedCommandSnapshot {
	t.Helper()
	h := &syntheticSidecarHarness{ctx: ctx, path: path}
	return readDevelopmentQueuedCommandSnapshot(t, h, commandID)
}

func developmentPolicyResumedPayloadEqual(beforeRaw, afterRaw, previousEpoch, resumedEpoch string, resumedGeneration uint64) bool {
	var before, after map[string]any
	if json.Unmarshal([]byte(beforeRaw), &before) != nil || json.Unmarshal([]byte(afterRaw), &after) != nil || len(before) != len(after) {
		return false
	}
	beforeRuntime, ok := before["captureRuntime"].(map[string]any)
	if !ok || beforeRuntime["driver"] != "headless" || beforeRuntime["epoch"] != previousEpoch ||
		beforeRuntime["generation"] != float64(1) {
		return false
	}
	afterRuntime, ok := after["captureRuntime"].(map[string]any)
	if !ok || afterRuntime["driver"] != "headless" || afterRuntime["epoch"] != resumedEpoch ||
		afterRuntime["generation"] != float64(resumedGeneration) || len(beforeRuntime) != len(afterRuntime) {
		return false
	}
	for key, value := range beforeRuntime {
		if key == "epoch" || key == "generation" {
			continue
		}
		if !reflect.DeepEqual(value, afterRuntime[key]) {
			return false
		}
	}
	delete(before, "captureRuntime")
	delete(after, "captureRuntime")
	return reflect.DeepEqual(before, after)
}

func readDevelopmentPolicyCommandCount(t *testing.T, path, status string) int {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatal("open policy Store query-only connection")
	}
	defer db.Close()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM bridge_commands WHERE status=?`, status).Scan(&count); err != nil {
		t.Fatal("read policy command count")
	}
	return count
}

func loadDevelopmentPolicy(t *testing.T, path string) (store.SyntheticAccountCheckpoint, uint64, bool) {
	t.Helper()
	state, err := store.OpenReadOnly(path)
	if err != nil {
		t.Fatalf("open synthetic policy Store read-only: %v", err)
	}
	defer state.Close()
	checkpoint, generation, enabled, err := state.LoadSyntheticAccountPolicy(context.Background())
	if err != nil {
		t.Fatalf("read synthetic policy: %v", err)
	}
	return checkpoint, generation, enabled
}

func readDevelopmentPolicyPreview(t *testing.T, receipt processFixtureReceipt, action string) SyntheticPolicyPreview {
	t.Helper()
	var preview SyntheticPolicyPreview
	if len(receipt.Frontend) == 0 || json.Unmarshal(receipt.Frontend, &preview) != nil || preview.ID == "" || preview.Action != action ||
		len(preview.Accounts) != 2 || preview.Accounts[domain.SourceX] != "alpha" || preview.Accounts[domain.SourceInstagram] != "alpha" {
		t.Fatalf("synthetic policy %s preview invalid", action)
	}
	return preview
}

func pinDevelopmentPolicyRoots(t *testing.T, child *processFixtureChild, chromePID int, nodeImage, chromeImage string) []developmentPolicyPinnedProcess {
	t.Helper()
	bridge := &chromeFixtureBridge{job: child.job}
	pids, err := bridge.jobPIDs()
	if err != nil {
		t.Fatal("read policy fixture Job roots", err)
	}
	nodePID, err := findDevelopmentPolicyNodePID(pids, child.identity.PID, nodeImage)
	if err != nil {
		t.Fatal(err)
	}
	roots := []developmentPolicyPinnedProcess{
		pinDevelopmentPolicyProcess(t, child.identity.PID, ""),
		pinDevelopmentPolicyProcess(t, nodePID, nodeImage),
		pinDevelopmentPolicyProcess(t, uint32(chromePID), chromeImage),
	}
	if roots[0].identity.PID == roots[1].identity.PID || roots[0].identity.PID == roots[2].identity.PID || roots[1].identity.PID == roots[2].identity.PID {
		t.Fatal("policy fixture process roots overlap")
	}
	if !containsDevelopmentPolicyPID(pids, roots[2].identity.PID) {
		t.Fatal("actual Chrome root was outside the outer Job")
	}
	parent, err := readerbroker.ParentPID(roots[1].identity.PID)
	if err != nil || parent != roots[0].identity.PID {
		t.Fatal("actual Node bridge was not a direct child of the fixture worker")
	}
	return roots
}

func containsDevelopmentPolicyPID(pids []uint32, expected uint32) bool {
	for _, pid := range pids {
		if pid == expected {
			return true
		}
	}
	return false
}

func waitDevelopmentPolicyWorkerExit(t *testing.T, child *processFixtureChild, roots []developmentPolicyPinnedProcess) {
	t.Helper()
	select {
	case err := <-child.exited:
		if err != nil {
			t.Fatalf("graceful policy worker exit: %v; stderr=%s", err, child.stderr.String())
		}
	case <-time.After(20 * time.Second):
		t.Fatal("graceful policy worker did not exit")
	}
	for _, process := range roots {
		policyProcessMustSignal(t, process)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := processFixtureWait(ctx, func() bool {
		pids, queryErr := (&chromeFixtureBridge{job: child.job}).jobPIDs()
		return queryErr == nil && len(pids) == 0
	}); err != nil {
		t.Fatal("graceful policy worker did not release its whole outer Job")
	}
}

func newDevelopmentPolicyRecoveryStart(t *testing.T, base processFixtureStart, checkpoint store.SyntheticAccountCheckpoint, policyGeneration uint64,
	profileID, sessionID string, runIDs []string, runID, commandID string, roots []developmentPolicyPinnedProcess) processFixtureStart {
	t.Helper()
	if len(roots) != 3 || len(runIDs) == 0 || runID == "" || commandID == "" {
		t.Fatal("policy recovery fixture bindings incomplete")
	}
	var nonceBytes [32]byte
	if _, err := rand.Read(nonceBytes[:]); err != nil {
		t.Fatal("policy recovery nonce unavailable")
	}
	storeID, err := processFixtureFileID(filepath.Join(base.Grant.Root, "fixture.sqlite"))
	if err != nil {
		t.Fatal("policy recovery physical Store identity unavailable")
	}
	base.Grant.ProfileID = profileID
	base.OldEpoch = checkpoint.OwnerEpoch
	base.PolicyRecovery = &processFixturePolicyRecoveryGrant{
		Nonce: hex.EncodeToString(nonceBytes[:]), RootID: base.Grant.RootID, ProfileID: profileID, StoreFileID: storeID,
		OldWorker: roots[0].identity, OldNode: roots[1].identity, OldChrome: roots[2].identity,
		CheckpointID: checkpoint.CheckpointID, PreviousEpoch: checkpoint.OwnerEpoch, CheckpointGeneration: checkpoint.Generation,
		PolicyGeneration: policyGeneration, Enabled: true, SessionID: sessionID, RunIDs: append([]string(nil), runIDs...),
		QueuedRunID: runID, QueuedCommandID: commandID,
	}
	return base
}

func assertPolicyRecoveryGrantRejectsMutations(t *testing.T, start processFixtureStart) {
	t.Helper()
	parent, err := processFixtureProcessIdentity(uint32(os.Getpid()))
	if err != nil || parent != start.Grant.Parent {
		t.Fatal("policy recovery controller identity unavailable")
	}
	mutations := []struct {
		name string
		edit func(*processFixtureStart)
	}{
		{"wrong physical profile", func(s *processFixtureStart) { s.PolicyRecovery.ProfileID = "wrong-profile" }},
		{"wrong physical Store", func(s *processFixtureStart) { s.PolicyRecovery.StoreFileID = "wrong-store" }},
		{"wrong owner epoch", func(s *processFixtureStart) { s.OldEpoch = "wrong-epoch" }},
		{"legacy profile preparation mix", func(s *processFixtureStart) { s.Grant.profilePrepared = true }},
		{"legacy startup barrier mix", func(s *processFixtureStart) { s.StartupBarrier = "node_before_ready" }},
		{"legacy reopen mix", func(s *processFixtureStart) { s.Grant.Reopen = true }},
		{"legacy old worker mix", func(s *processFixtureStart) { s.Grant.Old = processFixtureIdentity{PID: 1, Created: "old"} }},
		{"legacy window mix", func(s *processFixtureStart) { s.Grant.Windows = []uintptr{1} }},
		{"legacy tree-release mix", func(s *processFixtureStart) { s.Grant.TreeReleased = true }},
		{"legacy session mix", func(s *processFixtureStart) { s.SessionID = "other-session" }},
		{"legacy run mix", func(s *processFixtureStart) { s.RunIDs = []string{"other-run"} }},
		{"disabled policy mix", func(s *processFixtureStart) { s.PolicyRecovery.Enabled = false }},
	}
	for _, mutation := range mutations {
		bad := start
		grant := *start.PolicyRecovery
		grant.RunIDs = append([]string(nil), grant.RunIDs...)
		bad.PolicyRecovery = &grant
		mutation.edit(&bad)
		if err := validateProcessFixturePolicyRecoveryGrantForParent(start.Grant.Root, bad, parent); err == nil {
			t.Errorf("policy recovery grant accepted %s", mutation.name)
		}
	}
}

func assertDevelopmentPolicyPreOwnerJob(t *testing.T, child *processFixtureChild) {
	t.Helper()
	pids, err := (&chromeFixtureBridge{job: child.job}).jobPIDs()
	if err != nil || len(pids) != 1 || pids[0] != child.identity.PID {
		t.Fatalf("policy recovery pre-owner Job was not Go-only: pids=%v err=%v", pids, err)
	}
}

func TestDevelopmentPolicyMainChromeFixture(t *testing.T) {
	if os.Getenv("AKU_PROCESS_FIXTURE_ENABLED") != "1" {
		t.Skip("requires explicit VerifyDevelopment process fixture helper")
	}
	if os.Getenv("AKU_CHROME_FIXTURE_ROOT") == "" {
		t.Skip("requires the isolated Chrome fixture run root")
	}

	fixture := &chromeFixtureHTTP{}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal("loopback fixture listener unavailable")
	}
	var nonceBytes [32]byte
	if _, err := rand.Read(nonceBytes[:]); err != nil {
		t.Fatal("fixture nonce unavailable")
	}
	nonce := hex.EncodeToString(nonceBytes[:])
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
		t.Fatal("test controller identity unavailable")
	}
	rootID, err := processFixtureDirectoryID(config.Root)
	if err != nil {
		t.Fatal("fresh fixture root identity unavailable")
	}
	lineage := processFixtureLineage{Root: config.Root, RootID: rootID, Nonce: nonce, Parent: parent}
	lineageRaw, err := json.Marshal(lineage)
	if err != nil {
		t.Fatal("fixture lineage encoding failed")
	}
	lineagePath := filepath.Join(config.Root, "process-controller-lineage.json")
	if err := os.WriteFile(lineagePath, lineageRaw, 0600); err != nil {
		t.Fatal("fixture lineage could not be persisted")
	}
	controllerImage, err := os.Executable()
	if err != nil {
		t.Fatal("test controller image unavailable")
	}
	start := processFixtureStart{
		Grant:    processFixtureGrant{processFixtureLineage: lineage},
		NodePath: config.NodePath, ChromePath: config.ChromePath, DriverPath: config.DriverPath,
		Origin: config.Origin, AuthorityKey: nonce, ControllerImage: controllerImage,
	}
	executable := filepath.Join(os.Getenv("AKU_CHROME_FIXTURE_ROOT"), "aku-sidecar-dev.exe")
	if _, err := os.Stat(executable); err != nil {
		t.Fatal("dedicated development fixture executable not built")
	}
	child := startProcessFixtureChildWithProvider(t, executable, start, "synthetic-policy-fixture")
	fenced := child.receive(t, "policy_fenced")
	if !fenced.CheckpointFencedBeforeOwner || fenced.Epoch == "" || fenced.ProfileID == "" || fenced.PID != child.identity.PID {
		t.Fatal("main did not persist its provisional hold before creating an owner")
	}
	startupPIDs, err := (&chromeFixtureBridge{job: child.job}).jobPIDs()
	if err != nil || len(startupPIDs) != 1 || startupPIDs[0] != child.identity.PID {
		t.Fatalf("pre-owner outer Job was not Go-only: pids=%v err=%v", startupPIDs, err)
	}
	profilePath := filepath.Join(config.Root, "shared")
	if profileID, idErr := processFixtureDirectoryID(profilePath); idErr != nil || profileID != fenced.ProfileID {
		t.Fatal("policy hold did not bind the fresh physical Chrome profile")
	}
	if fixture.logins.Load() != 0 || fixture.sequence.Load() != 0 {
		t.Fatal("pre-owner policy fence performed login or capture")
	}
	dbPath := filepath.Join(config.Root, "fixture.sqlite")
	ctx := context.Background()
	state, err := store.Open(dbPath, domain.DefaultSettings("expanded", "quiet", "guarded_live", true))
	if err != nil {
		t.Fatal("open fresh fixture Store")
	}
	settings, err := state.GetSettings(ctx)
	if err != nil || settings.CollectionMode != "headless" || len(settings.ActiveSources) != 2 ||
		settings.ActiveSources[0] != domain.SourceX || settings.ActiveSources[1] != domain.SourceInstagram {
		_ = state.Close()
		t.Fatal("fixture Store did not retain the pinned headless source settings", err)
	}
	session, err := state.CreateUpdateSession(ctx, "synthetic policy queued capture", settings, domain.UpdatePolicy{
		Trigger: domain.UpdateTriggerUser, Delivery: domain.UpdateDeliveryVisible, BudgetAuthority: domain.BudgetAuthorityUser,
	})
	if err != nil {
		_ = state.Close()
		t.Fatal("create queued policy fixture session")
	}
	run, err := state.AdvanceSession(ctx, session.ID)
	if err != nil || run == nil {
		_ = state.Close()
		t.Fatal("create queued policy fixture run", err)
	}
	payload := capturePayload(*run, session.ID, settings, 1, nil, "")
	payload["captureCollector"] = collectorStamp("headless")
	payload["captureRuntime"] = map[string]any{"driver": "headless", "epoch": fenced.Epoch, "generation": 1}
	command, err := state.StartRun(ctx, run.ID, payload)
	if err != nil {
		_ = state.Close()
		t.Fatal("create queued policy fixture command")
	}
	seededSession, err := state.GetSession(ctx, session.ID)
	if err != nil || len(seededSession.Runs) != 2 {
		_ = state.Close()
		t.Fatal("policy fixture did not retain both planned source runs", err)
	}
	provisional := policyMetadata(t, &syntheticSidecarHarness{ctx: ctx, path: dbPath})
	if len(provisional) != 1 || provisional["synthetic_account_checkpoint_enrollment_hold"] == "" {
		_ = state.Close()
		t.Fatal("fresh enrollment did not leave exactly one provisional hold")
	}
	if _, _, _, err := state.LoadSyntheticAccountPolicy(ctx); err == nil {
		_ = state.Close()
		t.Fatal("provisional hold appeared as a completed account checkpoint")
	}
	claim, claimErr := state.ClaimCommandForCollector(ctx, run.ID, "pre-enrollment-probe", "headless")
	if claimErr == nil || claim != nil {
		_ = state.Close()
		t.Fatal("provisional enrollment did not reject the queued capture claim")
	}
	queueHarness := &syntheticSidecarHarness{ctx: ctx, path: dbPath}
	queuedBefore := readDevelopmentQueuedCommandSnapshot(t, queueHarness, command.ID)
	if queuedBefore.status != "queued" || queuedBefore.payload == "" {
		_ = state.Close()
		t.Fatal("policy fixture work was not durably queued")
	}
	if err := state.Close(); err != nil {
		t.Fatal("close Store seed handle before owner launch")
	}

	send := func(frame string) {
		t.Helper()
		if _, err := fmt.Fprintln(child.input, frame); err != nil {
			t.Fatalf("send private policy fixture frame %q: %v", frame, err)
		}
	}
	readPreview := func(stage, action string) (processFixtureReceipt, SyntheticPolicyPreview) {
		t.Helper()
		receipt := child.receive(t, stage)
		return receipt, readDevelopmentPolicyPreview(t, receipt, action)
	}
	assertQueuePayload := func() developmentQueuedCommandSnapshot {
		t.Helper()
		row := readDevelopmentPolicyCommand(t, ctx, dbPath, command.ID)
		if row.payload != queuedBefore.payload {
			t.Fatal("policy confirmation rewrote queued command payload")
		}
		return row
	}
	assertCheckpoint := func() {
		t.Helper()
		checkpoint, generation, enabled := loadDevelopmentPolicy(t, dbPath)
		if generation != 1 || enabled || checkpoint.Generation != 1 || checkpoint.CheckpointID == "" ||
			len(checkpoint.ExpectedAccounts) != 2 || len(checkpoint.AcceptedRevisions) != 0 {
			t.Fatal("confirmed enrollment was not expected-only and disabled")
		}
		for _, source := range []domain.Source{domain.SourceX, domain.SourceInstagram} {
			identity, ok := checkpoint.ExpectedAccounts[source]
			if !ok || identity.AccountID != "alpha" || identity.StoreScopeID != rootID || identity.ProfileID != fenced.ProfileID {
				t.Fatalf("enrollment identity escaped its physical scope for %s", source)
			}
		}
	}

	send("launch_policy_owner")
	enrollmentReceipt, firstEnrollment := readPreview("policy_enrollment_preview", "enroll")
	if enrollmentReceipt.ChromePID <= 0 || enrollmentReceipt.ProfileID != fenced.ProfileID || enrollmentReceipt.Epoch != fenced.Epoch {
		t.Fatal("enrollment preview did not bind the launched physical Chrome owner")
	}
	if fixture.logins.Load() != 1 || fixture.sequence.Load() != 0 {
		t.Fatal("Chrome fixture login/capture count invalid before enrollment confirmation")
	}
	chromeImage, err := filepath.EvalSymlinks(config.ChromePath)
	if err != nil {
		t.Fatal("configured fixture Chrome image unavailable")
	}
	nodeImage, err := filepath.EvalSymlinks(config.NodePath)
	if err != nil {
		t.Fatal("configured fixture Node image unavailable")
	}
	workerProcess := pinDevelopmentPolicyProcess(t, child.identity.PID, executable)
	chromeProcess := pinDevelopmentPolicyProcess(t, uint32(enrollmentReceipt.ChromePID), chromeImage)
	outerPIDs, err := (&chromeFixtureBridge{job: child.job}).jobPIDs()
	if err != nil {
		t.Fatal("read actual outer Job inventory")
	}
	nodePID, err := findDevelopmentPolicyNodePID(outerPIDs, child.identity.PID, nodeImage)
	if err != nil {
		t.Fatal(err)
	}
	nodeProcess := pinDevelopmentPolicyProcess(t, nodePID, nodeImage)
	if nodeProcess.identity.PID == workerProcess.identity.PID || nodeProcess.identity.PID == chromeProcess.identity.PID {
		t.Fatal("policy fixture process identities overlapped")
	}
	containsPID := func(pids []uint32, expected uint32) bool {
		for _, pid := range pids {
			if pid == expected {
				return true
			}
		}
		return false
	}
	if !containsPID(outerPIDs, workerProcess.identity.PID) || !containsPID(outerPIDs, nodeProcess.identity.PID) || !containsPID(outerPIDs, chromeProcess.identity.PID) {
		t.Fatal("outer Job did not contain Go, Node bridge, and actual Chrome roots")
	}
	nodeParent, err := readerbroker.ParentPID(nodeProcess.identity.PID)
	if err != nil || nodeParent != workerProcess.identity.PID {
		t.Fatal("actual Node bridge was not a direct child of the fixture worker")
	}
	for _, process := range []developmentPolicyPinnedProcess{workerProcess, nodeProcess, chromeProcess} {
		policyProcessMustRemainLive(t, process)
	}

	canceledMeta := policyMetadata(t, queueHarness)
	if len(canceledMeta) != 1 || !reflect.DeepEqual(canceledMeta, provisional) {
		t.Fatal("provisional enrollment metadata changed before cancellation")
	}
	send("cancel_enrollment:" + firstEnrollment.ID)
	child.receive(t, "policy_enrollment_canceled")
	if after := policyMetadata(t, queueHarness); !reflect.DeepEqual(after, provisional) {
		t.Fatal("canceled enrollment removed or changed its durable hold")
	}
	secondEnrollmentReceipt, secondEnrollment := readPreview("policy_enrollment_preview", "enroll")
	if secondEnrollment.ID == firstEnrollment.ID || secondEnrollmentReceipt.ChromePID != enrollmentReceipt.ChromePID {
		t.Fatal("enrollment cancellation was replayable or changed Chrome owner")
	}
	send("confirm_enrollment:" + secondEnrollment.ID)
	enrolled := child.receive(t, "policy_enrolled_disabled")
	if enrolled.ChromePID != enrollmentReceipt.ChromePID || enrolled.ProfileID != fenced.ProfileID || enrolled.Epoch != fenced.Epoch {
		t.Fatal("confirmed enrollment changed physical owner lineage")
	}
	assertCheckpoint()
	enrolledMeta := policyMetadata(t, queueHarness)
	if len(enrolledMeta) != 4 || enrolledMeta["synthetic_account_checkpoint_enrollment_hold"] != "" {
		t.Fatal("confirmation did not atomically replace provisional hold with checkpoint metadata")
	}
	if row := assertQueuePayload(); row.status != "queued" || row.claimedAt.Valid || row.claimedBy.Valid {
		t.Fatal("enrollment confirmation changed or claimed queued work")
	}
	if fixture.logins.Load() != 1 || fixture.sequence.Load() != 0 {
		t.Fatal("enrollment performed a second login or capture")
	}

	fixture.authority.Lock()
	savedAuthority := fixture.active
	fixture.active = SyntheticSessionEvidence{Authenticated: true, AccountID: "another-fixture-account", Revision: savedAuthority.Revision}
	fixture.authority.Unlock()
	send("preview_enable")
	rejected := child.receive(t, "policy_preview_rejected")
	if rejected.ChromePID != enrollmentReceipt.ChromePID || rejected.ProfileID != fenced.ProfileID {
		t.Fatal("changed-account rejection lost physical owner scope")
	}
	assertCheckpoint()
	if row := assertQueuePayload(); row.status != "queued" {
		t.Fatal("rejected account observation changed queued work")
	}
	fixture.authority.Lock()
	fixture.active = savedAuthority
	fixture.authority.Unlock()
	if fixture.logins.Load() != 1 || fixture.sequence.Load() != 0 {
		t.Fatal("rejected account preview performed capture or another login")
	}

	send("preview_enable")
	enableReceipt, enablePreview := readPreview("policy_enable_preview", "enable")
	if enableReceipt.ChromePID != enrollmentReceipt.ChromePID || enableReceipt.ProfileID != fenced.ProfileID || enableReceipt.Epoch != fenced.Epoch {
		t.Fatal("enable preview changed physical owner lineage")
	}
	send("confirm_enable:" + enablePreview.ID)
	send("start_capture")
	active := child.receive(t, "policy_capture_active")
	if active.ChromePID != enrollmentReceipt.ChromePID || active.ProfileID != fenced.ProfileID || active.Epoch != fenced.Epoch ||
		!active.CaptureActive || !active.CheckpointAuthorizedAtStartup || fixture.logins.Load() != 1 || fixture.sequence.Load() != 1 {
		t.Fatal("confirmed policy did not start the verified, single-login capture")
	}
	checkpoint, policyGeneration, enabled := loadDevelopmentPolicy(t, dbPath)
	if !enabled || policyGeneration != 2 || len(checkpoint.ExpectedAccounts) != 2 {
		t.Fatal("confirmed opt-in was not durably enabled")
	}
	claimed := assertQueuePayload()
	if claimed.status != "claimed" || !claimed.claimedAt.Valid || !claimed.claimedBy.Valid {
		t.Fatal("opt-in did not actually claim the queued capture")
	}
	for _, process := range []developmentPolicyPinnedProcess{workerProcess, nodeProcess, chromeProcess} {
		policyProcessMustRemainLive(t, process)
	}

	send("disable_policy")
	draining := child.receive(t, "policy_disabled_draining")
	if draining.ChromePID != enrollmentReceipt.ChromePID || !draining.DurablePause || fixture.logins.Load() != 1 || fixture.sequence.Load() != 1 {
		t.Fatal("opt-out did not remain at the durable active-drain boundary")
	}
	_, policyGeneration, enabled = loadDevelopmentPolicy(t, dbPath)
	if enabled || policyGeneration != 3 {
		t.Fatal("opt-out was not persisted before capture drain")
	}
	if row := assertQueuePayload(); row.status != "claimed" {
		t.Fatal("opt-out canceled or completed active capture before release")
	}
	for _, process := range []developmentPolicyPinnedProcess{workerProcess, nodeProcess, chromeProcess} {
		policyProcessMustRemainLive(t, process)
	}

	send("release_capture")
	optedOut := child.receive(t, "policy_opted_out")
	if optedOut.ChromePID != enrollmentReceipt.ChromePID || !optedOut.CleanupVerified || fixture.logins.Load() != 1 || fixture.sequence.Load() != 1 {
		t.Fatal("opt-out did not drain and clean the single Chrome capture")
	}
	_, policyGeneration, enabled = loadDevelopmentPolicy(t, dbPath)
	if enabled || policyGeneration != 3 {
		t.Fatal("opt-out policy changed after drain")
	}
	completed := assertQueuePayload()
	if completed.status != "completed" || completed.claimedBy.String == "" {
		t.Fatal("active capture did not complete before owner cleanup")
	}
	if readDevelopmentPolicyCommandCount(t, dbPath, "claimed") != 0 {
		t.Fatal("opt-out left claimed work after drain")
	}
	readback, err := store.OpenReadOnly(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	retained, readErr := readback.GetSession(ctx, session.ID)
	closeErr := readback.Close()
	if readErr != nil || closeErr != nil || len(retained.Runs) != len(seededSession.Runs) {
		t.Fatal("opt-out lost the planned source queue", readErr, closeErr)
	}
	for i, item := range retained.Runs {
		if item.ID != seededSession.Runs[i].ID || item.Source != seededSession.Runs[i].Source {
			t.Fatal("opt-out reassigned planned work")
		}
		if item.ID != run.ID && (item.Status == "completed" || item.Status == "failed" || item.Status == "cancelled") {
			t.Fatal("unstarted source work was executed or canceled by opt-out")
		}
	}
	if pids, queryErr := (&chromeFixtureBridge{job: child.job}).jobPIDs(); queryErr != nil || len(pids) != 1 || pids[0] != workerProcess.identity.PID {
		t.Fatalf("outer Job did not retain only the Go worker after opt-out: pids=%v err=%v", pids, queryErr)
	}
	policyProcessMustSignal(t, nodeProcess)
	policyProcessMustSignal(t, chromeProcess)
	policyProcessMustRemainLive(t, workerProcess)
	if currentRootID, idErr := processFixtureDirectoryID(config.Root); idErr != nil || currentRootID != rootID {
		t.Fatal("fresh physical fixture root changed during policy lifecycle")
	}
	if currentProfileID, idErr := processFixtureDirectoryID(profilePath); idErr != nil || currentProfileID != fenced.ProfileID {
		t.Fatal("fresh physical Chrome profile changed during policy lifecycle")
	}
	if afterLineage, readErr := os.ReadFile(lineagePath); readErr != nil || string(afterLineage) != string(lineageRaw) {
		t.Fatal("controller lineage changed during policy lifecycle")
	}
	if _, err := os.Stat(filepath.Join(config.Root, "process-recovery-consumed.json")); !os.IsNotExist(err) {
		t.Fatal("fresh enrollment unexpectedly issued a process-reopen grant")
	}
	if fixture.logins.Load() != 1 || fixture.sequence.Load() != 1 {
		t.Fatal("policy lifecycle performed repeated login or capture")
	}

	child.stop(t, false, executable, nil)
	if status, waitErr := windows.WaitForSingleObject(workerProcess.handle, 3000); waitErr != nil || status != windows.WAIT_OBJECT_0 {
		t.Fatal("Go policy fixture worker did not signal after stop")
	}
	if pids, queryErr := (&chromeFixtureBridge{job: child.job}).jobPIDs(); queryErr != nil || len(pids) != 0 {
		t.Fatalf("outer Job not empty after graceful stop: pids=%v err=%v", pids, queryErr)
	}
	if fixture.logins.Load() != 1 || fixture.sequence.Load() != 1 {
		t.Fatal("graceful stop changed synthetic login/capture totals")
	}
	t.Logf("actualMainPolicy=true preOwnerGoOnly=true provisionalClaimDenied=true canceledEnrollmentRetainedHold=true changedAuthorityDenied=true disabledBeforeDrain=true activeCaptureCompleted=true unstartedSourceRetained=true chromePID=%d nodePID=%d rootsSignalled=true allOwnedJobsEmpty=true loginCount=1 captureCount=1", chromeProcess.identity.PID, nodeProcess.identity.PID)
}

func TestDevelopmentPolicyMainChromeFixtureColdRestart(t *testing.T) {
	if os.Getenv("AKU_PROCESS_FIXTURE_ENABLED") != "1" || os.Getenv("AKU_CHROME_FIXTURE_ROOT") == "" {
		t.Skip("requires explicit VerifyDevelopment process fixture helper")
	}
	fixture := &chromeFixtureHTTP{}
	var authorityUnavailable atomic.Bool
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal("loopback fixture listener unavailable")
	}
	var nonceBytes [32]byte
	if _, err := rand.Read(nonceBytes[:]); err != nil {
		t.Fatal("fixture nonce unavailable")
	}
	nonce := hex.EncodeToString(nonceBytes[:])
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/process-authority" {
			if r.Header.Get("X-Fixture-Key") != nonce {
				http.Error(w, "denied", http.StatusForbidden)
				return
			}
			if authorityUnavailable.Load() {
				http.Error(w, "synthetic authority unavailable", http.StatusServiceUnavailable)
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
		t.Fatal("test controller identity unavailable")
	}
	rootID, err := processFixtureDirectoryID(config.Root)
	if err != nil {
		t.Fatal("fixture root identity unavailable")
	}
	lineage := processFixtureLineage{Root: config.Root, RootID: rootID, Nonce: nonce, Parent: parent}
	lineageRaw, err := json.Marshal(lineage)
	if err != nil {
		t.Fatal("fixture lineage encoding failed")
	}
	lineagePath := filepath.Join(config.Root, "process-controller-lineage.json")
	if err := os.WriteFile(lineagePath, lineageRaw, 0600); err != nil {
		t.Fatal("fixture lineage could not be persisted")
	}
	controllerImage, err := os.Executable()
	if err != nil {
		t.Fatal("test controller image unavailable")
	}
	executable := filepath.Join(os.Getenv("AKU_CHROME_FIXTURE_ROOT"), "aku-sidecar-dev.exe")
	if _, err := os.Stat(executable); err != nil {
		t.Fatal("dedicated development fixture executable not built")
	}
	base := processFixtureStart{
		Grant: processFixtureGrant{processFixtureLineage: lineage}, NodePath: config.NodePath, ChromePath: config.ChromePath,
		DriverPath: config.DriverPath, Origin: config.Origin, AuthorityKey: nonce, ControllerImage: controllerImage,
	}
	chromeImage, err := filepath.EvalSymlinks(config.ChromePath)
	if err != nil {
		t.Fatal("configured fixture Chrome image unavailable")
	}
	nodeImage, err := filepath.EvalSymlinks(config.NodePath)
	if err != nil {
		t.Fatal("configured fixture Node image unavailable")
	}

	// Seed one genuinely eligible headless command while the fresh controller is
	// paused. The test controller keeps its full immutable row snapshot around
	// every later admission probe.
	first := startProcessFixtureChildWithProvider(t, executable, base, "synthetic-policy-fixture")
	fenced := first.receive(t, "policy_fenced")
	if !fenced.CheckpointFencedBeforeOwner || fenced.PID != first.identity.PID || fenced.ProfileID == "" {
		t.Fatal("fresh actual main did not fence before owner creation")
	}
	startupPIDs, err := (&chromeFixtureBridge{job: first.job}).jobPIDs()
	if err != nil || len(startupPIDs) != 1 || startupPIDs[0] != first.identity.PID {
		t.Fatalf("fresh pre-owner Job was not Go-only: pids=%v err=%v", startupPIDs, err)
	}
	dbPath := filepath.Join(config.Root, "fixture.sqlite")
	ctx := context.Background()
	state, err := store.Open(dbPath, domain.DefaultSettings("expanded", "quiet", "guarded_live", true))
	if err != nil {
		t.Fatal("open fresh fixture Store")
	}
	settings, err := state.GetSettings(ctx)
	if err != nil || settings.CollectionMode != "headless" || len(settings.ActiveSources) != 2 {
		_ = state.Close()
		t.Fatal("fixture Store did not retain headless source settings", err)
	}
	sessionSettings := settings
	sessionSettings.ActiveSources = []domain.Source{domain.SourceX}
	session, err := state.CreateUpdateSession(ctx, "synthetic policy cold restart", sessionSettings, domain.UpdatePolicy{
		Trigger: domain.UpdateTriggerUser, Delivery: domain.UpdateDeliveryVisible, BudgetAuthority: domain.BudgetAuthorityUser,
	})
	if err != nil {
		_ = state.Close()
		t.Fatal("create queued policy fixture session")
	}
	run, err := state.AdvanceSession(ctx, session.ID)
	if err != nil || run == nil {
		_ = state.Close()
		t.Fatal("create queued policy fixture run", err)
	}
	payload := capturePayload(*run, session.ID, sessionSettings, 1, nil, "")
	payload["captureCollector"] = collectorStamp("headless")
	payload["captureRuntime"] = map[string]any{"driver": "headless", "epoch": fenced.Epoch, "generation": 1}
	command, err := state.StartRun(ctx, run.ID, payload)
	if err != nil {
		_ = state.Close()
		t.Fatal("create eligible queued policy fixture command")
	}
	seededSession, err := state.GetSession(ctx, session.ID)
	if err != nil || len(seededSession.Runs) != 1 || seededSession.Runs[0].ID != run.ID {
		_ = state.Close()
		t.Fatal("policy fixture session did not retain its planned runs", err)
	}
	if err := state.Close(); err != nil {
		t.Fatal("close Store seed handle before owner launch")
	}
	queuedBefore := readDevelopmentPolicyCommand(t, ctx, dbPath, command.ID)
	if queuedBefore.status != "queued" || queuedBefore.payload == "" || queuedBefore.claimedBy.Valid || queuedBefore.claimedAt.Valid {
		t.Fatal("eligible policy command was not durably queued")
	}
	send := func(child *processFixtureChild, frame string) {
		t.Helper()
		if _, err := fmt.Fprintln(child.input, frame); err != nil {
			t.Fatalf("send private policy fixture frame %q: %v", frame, err)
		}
	}
	readPreview := func(child *processFixtureChild, stage, action string) SyntheticPolicyPreview {
		t.Helper()
		receipt := child.receive(t, stage)
		return readDevelopmentPolicyPreview(t, receipt, action)
	}
	assertQueued := func(want developmentQueuedCommandSnapshot) {
		t.Helper()
		got := readDevelopmentPolicyCommand(t, ctx, dbPath, command.ID)
		if got != want {
			t.Fatalf("queued work changed across closed admission: before=%+v after=%+v", want, got)
		}
	}
	completeFreshEnable := func(child *processFixtureChild) []developmentPolicyPinnedProcess {
		t.Helper()
		send(child, "launch_policy_owner")
		firstEnrollmentReceipt := child.receive(t, "policy_enrollment_preview")
		firstEnrollment := readDevelopmentPolicyPreview(t, firstEnrollmentReceipt, "enroll")
		if firstEnrollmentReceipt.ChromePID <= 0 || firstEnrollmentReceipt.ProfileID != fenced.ProfileID || firstEnrollmentReceipt.Epoch != fenced.Epoch {
			t.Fatal("fresh enrollment owner lost its physical profile scope")
		}
		roots := pinDevelopmentPolicyRoots(t, child, firstEnrollmentReceipt.ChromePID, nodeImage, chromeImage)
		if fixture.logins.Load() != 1 || fixture.sequence.Load() != 0 {
			t.Fatal("fresh owner login/capture counts invalid")
		}
		send(child, "cancel_enrollment:"+firstEnrollment.ID)
		child.receive(t, "policy_enrollment_canceled")
		secondEnrollment := readPreview(child, "policy_enrollment_preview", "enroll")
		send(child, "confirm_enrollment:"+secondEnrollment.ID)
		child.receive(t, "policy_enrolled_disabled")
		fixture.authority.Lock()
		saved := fixture.active
		fixture.active = SyntheticSessionEvidence{Authenticated: true, AccountID: "changed-fixture-account", Revision: saved.Revision}
		fixture.authority.Unlock()
		send(child, "preview_enable")
		child.receive(t, "policy_preview_rejected")
		fixture.authority.Lock()
		fixture.active = saved
		fixture.authority.Unlock()
		send(child, "preview_enable")
		enableReceipt := child.receive(t, "policy_enable_preview")
		enable := readDevelopmentPolicyPreview(t, enableReceipt, "enable")
		if enableReceipt.ProfileID != fenced.ProfileID || enableReceipt.Epoch != fenced.Epoch {
			t.Fatal("fresh enable preview changed physical owner scope")
		}
		send(child, "confirm_enable:"+enable.ID)
		send(child, "stop_policy_worker")
		idle := child.receive(t, "policy_enabled_idle")
		if idle.ProfileID != fenced.ProfileID || !idle.CheckpointAuthorizedAtStartup || !idle.CheckpointFencedBeforeOwner || !idle.GracefulBrowserRelease {
			t.Fatal("fresh policy enable did not keep its startup evidence")
		}
		if fixture.logins.Load() != 1 || fixture.sequence.Load() != 0 {
			t.Fatal("fresh enable unexpectedly repeated login or captured work")
		}
		return roots
	}
	firstRoots := completeFreshEnable(first)
	waitDevelopmentPolicyWorkerExit(t, first, firstRoots)
	assertQueued(queuedBefore)
	profileID := fenced.ProfileID
	storeID, err := processFixtureFileID(dbPath)
	if err != nil {
		t.Fatal("fresh Store physical identity unavailable")
	}
	checkpoint, policyGeneration, enabled := loadDevelopmentPolicy(t, dbPath)
	if !enabled || checkpoint.CheckpointID == "" || checkpoint.Generation < 2 || checkpoint.OwnerEpoch != fenced.Epoch ||
		len(checkpoint.ExpectedAccounts) != 2 || len(checkpoint.AcceptedRevisions) != 2 || policyGeneration == 0 {
		t.Fatal("fresh enabled checkpoint was not durably authorized")
	}
	if current, idErr := processFixtureFileID(dbPath); idErr != nil || current != storeID {
		t.Fatal("fresh fixture SQLite identity changed before recovery")
	}

	makeRecovery := func(roots []developmentPolicyPinnedProcess) processFixtureStart {
		t.Helper()
		recovery := newDevelopmentPolicyRecoveryStart(t, base, checkpoint, policyGeneration, profileID, session.ID,
			[]string{seededSession.Runs[0].ID}, run.ID, command.ID, roots)
		parent, parentErr := processFixtureProcessIdentity(uint32(os.Getpid()))
		if parentErr != nil || parent != recovery.Grant.Parent {
			t.Fatal("policy recovery controller identity unavailable")
		}
		if err := validateProcessFixturePolicyRecoveryGrantForParent(config.Root, recovery, parent); err != nil {
			t.Fatalf("issued private policy recovery grant did not validate after whole-tree release: %v", err)
		}
		return recovery
	}
	assertRecoveryReceipt := func(receipt processFixtureReceipt, stage string, recovery processFixtureStart, ownerReady bool) {
		t.Helper()
		grant := recovery.PolicyRecovery
		if receipt.Stage != stage || receipt.ProfileID != profileID || receipt.StoreFileID != storeID || receipt.CheckpointID != checkpoint.CheckpointID ||
			receipt.CheckpointGeneration != checkpoint.Generation || receipt.PolicyGeneration != policyGeneration || receipt.Epoch == "" ||
			!receipt.CheckpointFencedBeforeOwner || receipt.QueuedRunID != run.ID || receipt.QueuedCommandID != command.ID ||
			!receipt.GrantReplayDenied ||
			(!ownerReady && !receipt.PreOwnerClaimDenied) || (ownerReady && !receipt.PreProofClaimDenied) ||
			(grant.ProfileID != profileID || grant.StoreFileID != storeID) {
			t.Fatalf("%s receipt did not bind the cold recovery scope: %+v", stage, receipt)
		}
		if ownerReady && (receipt.ChromePID <= 0 || receipt.Epoch == checkpoint.OwnerEpoch) {
			t.Fatal("recovery owner did not receive a new epoch")
		}
		if ownerReady {
			t.Logf("recoveryPreResumeBrowser probeOK=%t authenticated=%t expectedAccount=%t revisionPresent=%t",
				receipt.PreResumeBrowserProbeOK, receipt.PreResumeBrowserAuthenticated,
				receipt.PreResumeBrowserAccountMatch, receipt.PreResumeBrowserRevision)
		}
	}

	// Recovery attempt one proves owner startup remains fenced and a fresh
	// authority failure cannot authorize the queued command. The controller then
	// kills that exact worker; its granted Node and Chrome roots must signal and
	// the inherited outer Job must become empty before another grant is issued.
	failedRecovery := makeRecovery(firstRoots)
	assertPolicyRecoveryGrantRejectsMutations(t, failedRecovery)
	assertQueued(queuedBefore)
	failedWorker := startProcessFixtureChildWithProvider(t, executable, failedRecovery, "synthetic-policy-fixture")
	preOwner := failedWorker.receive(t, "policy_recovery_fenced")
	assertDevelopmentPolicyPreOwnerJob(t, failedWorker)
	assertRecoveryReceipt(preOwner, "policy_recovery_fenced", failedRecovery, false)
	assertQueued(queuedBefore)
	send(failedWorker, "launch_policy_owner")
	ownerReady := failedWorker.receive(t, "policy_recovery_owner_ready")
	assertRecoveryReceipt(ownerReady, "policy_recovery_owner_ready", failedRecovery, true)
	failedRoots := pinDevelopmentPolicyRoots(t, failedWorker, ownerReady.ChromePID, nodeImage, chromeImage)
	assertQueued(queuedBefore)
	if err := consumeProcessFixturePolicyRecoveryGrantForParent(config.Root, failedRecovery, parent); err == nil {
		t.Fatal("replayed headless policy recovery grant was accepted")
	}
	authorityUnavailable.Store(true)
	send(failedWorker, "resume_policy")
	denied := failedWorker.receive(t, "policy_resume_denied")
	if !denied.AuthorityDenied || denied.CaptureActive || denied.ProfileID != profileID || denied.CheckpointID != checkpoint.CheckpointID ||
		denied.CheckpointGeneration != checkpoint.Generation || denied.PolicyGeneration != policyGeneration ||
		!strings.Contains(denied.Failure, "authorityReady=false") ||
		!strings.Contains(denied.Failure, "checkpointUnchanged=unchanged") {
		t.Fatalf("unavailable authority did not produce the expected closed recovery fence: %+v", denied)
	}
	checkpointAfterFailure, policyAfterFailure, enabledAfterFailure := loadDevelopmentPolicy(t, dbPath)
	if !enabledAfterFailure || policyAfterFailure != policyGeneration || !syntheticCheckpointStateEqual(checkpointAfterFailure, checkpoint) {
		t.Fatal("failed fresh authority proof changed persisted policy records")
	}
	assertQueued(queuedBefore)
	if fixture.logins.Load() != 1 || fixture.sequence.Load() != 0 {
		t.Fatal("failed authority recovery performed a new login or capture")
	}
	failedWorker.stop(t, true, executable, nil)
	for _, process := range failedRoots {
		policyProcessMustSignal(t, process)
	}

	// A second, single-use grant is issued only after abrupt release. The new
	// actual-main owner reuses the same SQLite file and physical profile, proves
	// current authority/browser state, resumes once, drains the preserved work,
	// and exits gracefully.
	authorityUnavailable.Store(false)
	successRecovery := makeRecovery(failedRoots)
	assertQueued(queuedBefore)
	successWorker := startProcessFixtureChildWithProvider(t, executable, successRecovery, "synthetic-policy-fixture")
	successFence := successWorker.receive(t, "policy_recovery_fenced")
	assertDevelopmentPolicyPreOwnerJob(t, successWorker)
	assertRecoveryReceipt(successFence, "policy_recovery_fenced", successRecovery, false)
	assertQueued(queuedBefore)
	send(successWorker, "launch_policy_owner")
	successOwner := successWorker.receive(t, "policy_recovery_owner_ready")
	assertRecoveryReceipt(successOwner, "policy_recovery_owner_ready", successRecovery, true)
	successRoots := pinDevelopmentPolicyRoots(t, successWorker, successOwner.ChromePID, nodeImage, chromeImage)
	assertQueued(queuedBefore)
	send(successWorker, "resume_policy")
	active := successWorker.receive(t, "policy_resumed_active")
	if !active.CaptureActive || !active.CheckpointAuthorizedAtStartup || active.ProfileID != profileID || active.StoreFileID != storeID ||
		active.Epoch == checkpoint.OwnerEpoch || active.CheckpointGeneration <= checkpoint.Generation || fixture.logins.Load() != 1 || fixture.sequence.Load() != 1 {
		t.Fatal("fresh actual-main verification did not resume the existing identity without login")
	}
	resumedCheckpoint, resumedPolicyGeneration, resumedEnabled := loadDevelopmentPolicy(t, dbPath)
	if !resumedEnabled || resumedPolicyGeneration != policyGeneration || resumedCheckpoint.CheckpointID != checkpoint.CheckpointID ||
		resumedCheckpoint.Generation != active.CheckpointGeneration || resumedCheckpoint.OwnerEpoch != active.Epoch ||
		!syntheticCheckpointExpectedEqual(resumedCheckpoint.ExpectedAccounts, checkpoint.ExpectedAccounts) ||
		!reflect.DeepEqual(resumedCheckpoint.AcceptedRevisions, checkpoint.AcceptedRevisions) {
		t.Fatal("fresh recovery changed account authority while rotating its owner proof")
	}
	claimed := readDevelopmentPolicyCommand(t, ctx, dbPath, command.ID)
	if claimed.status != "claimed" || !claimed.claimedAt.Valid || !claimed.claimedBy.Valid ||
		!developmentPolicyResumedPayloadEqual(queuedBefore.payload, claimed.payload, fenced.Epoch, active.Epoch, active.Generation) ||
		active.Generation == 0 || active.QueuedRunID != run.ID || active.QueuedCommandID != command.ID || successRecovery.PolicyRecovery.SessionID != session.ID {
		t.Fatalf("recovery claim did not preserve command identity and semantic payload: status=%s claimedAt=%t claimedBy=%t payloadLength=%d runIDMatch=%t commandIDMatch=%t sessionIDMatch=%t ownerGeneration=%d",
			claimed.status, claimed.claimedAt.Valid, claimed.claimedBy.Valid, len(claimed.payload), active.QueuedRunID == run.ID,
			active.QueuedCommandID == command.ID, successRecovery.PolicyRecovery.SessionID == session.ID, active.Generation)
	}
	send(successWorker, "release_capture")
	drained := successWorker.receive(t, "policy_recovered_drained")
	if !drained.QueueCompleted || drained.Epoch != active.Epoch || drained.CheckpointGeneration != active.CheckpointGeneration {
		t.Fatal("successful recovery did not report a completed queue drain")
	}
	completed := readDevelopmentPolicyCommand(t, ctx, dbPath, command.ID)
	if completed.status != "completed" || completed.payload != claimed.payload || !completed.claimedBy.Valid {
		t.Fatal("recovered capture was not completed with its preserved payload")
	}
	if !readDevelopmentPolicyCommand(t, ctx, dbPath, command.ID).claimedBy.Valid {
		t.Fatal("recovered command claim receipt disappeared")
	}
	if readDevelopmentPolicyCommandCount(t, dbPath, "claimed") != 0 || fixture.logins.Load() != 1 || fixture.sequence.Load() != 1 {
		t.Fatal("recovery drain left claimed work or repeated login/capture")
	}
	assertQueue := readDevelopmentPolicyCommand(t, ctx, dbPath, command.ID)
	if assertQueue.status != "completed" || assertQueue.payload != claimed.payload {
		t.Fatal("completed recovery queue record changed after drain")
	}
	readback, err := store.OpenReadOnly(dbPath)
	if err != nil {
		t.Fatal("open retained session readback")
	}
	retained, readErr := readback.GetSession(ctx, session.ID)
	closeErr := readback.Close()
	if readErr != nil || closeErr != nil || len(retained.Runs) != 1 || retained.Runs[0].ID != run.ID || retained.Runs[0].BridgeCommandStatus != "completed" {
		t.Fatal("recovery lost or reassigned the queued session", readErr, closeErr)
	}
	if currentRootID, idErr := processFixtureDirectoryID(config.Root); idErr != nil || currentRootID != rootID {
		t.Fatal("cold recovery changed the physical fixture root")
	}
	if currentProfileID, idErr := processFixtureDirectoryID(filepath.Join(config.Root, "shared")); idErr != nil || currentProfileID != profileID {
		t.Fatal("cold recovery changed the physical Chrome profile")
	}
	if currentStoreID, idErr := processFixtureFileID(dbPath); idErr != nil || currentStoreID != storeID {
		t.Fatal("cold recovery changed the physical fixture SQLite file")
	}
	if afterLineage, readErr := os.ReadFile(lineagePath); readErr != nil || string(afterLineage) != string(lineageRaw) {
		t.Fatal("policy restart changed controller lineage")
	}
	send(successWorker, "stop_policy_worker")
	waitDevelopmentPolicyWorkerExit(t, successWorker, successRoots)
	if pids, queryErr := (&chromeFixtureBridge{job: successWorker.job}).jobPIDs(); queryErr != nil || len(pids) != 0 {
		t.Fatalf("successful recovery left a process in its outer Job: pids=%v err=%v", pids, queryErr)
	}
	t.Logf("actualMainPolicyRecovery=true gracefulOldOwnerReleased=true abruptFailedOwnerReleased=true sameProfile=true sameSQLite=true preOwnerClaimDenied=true preProofClaimDenied=true replayDenied=true authorityUnavailableDenied=true queuePreservedThroughFailure=true resumedCaptureDrained=true loginCount=1 captureCount=1 allOwnedJobsEmpty=true oldWorkerPID=%d newWorkerPID=%d", first.identity.PID, successWorker.identity.PID)
}
