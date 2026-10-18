//go:build windows && auth_session_fixture

package engine

// This is a deliberately separate fixture composition. An explicit development
// dispatch can invoke it from cmd/akusidecar before ordinary startup. It never
// loads saved user configuration or registers a native host.
import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/collection"
	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/readerbroker"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
	"github.com/abangkis/AkuSidecar/internal/store"
	"golang.org/x/sys/windows"
)

type processFixtureIdentity struct {
	PID     uint32
	Created string
}
type processFixtureLineage struct {
	Root, RootID, Nonce string
	Parent              processFixtureIdentity
}
type processFixtureGrant struct {
	processFixtureLineage
	Reopen          bool
	Old             processFixtureIdentity
	ProfileID       string
	Windows         []uintptr
	TreeReleased    bool
	profilePrepared bool
}

// processFixturePolicyRecoveryGrant is a separate headless-only startup
// authority. It never changes the legacy native/window Reopen contract.
type processFixturePolicyRecoveryGrant struct {
	Nonce, RootID, ProfileID, StoreFileID   string
	OldWorker, OldNode, OldChrome           processFixtureIdentity
	CheckpointID, PreviousEpoch             string
	CheckpointGeneration, PolicyGeneration  uint64
	Enabled                                 bool
	SessionID, QueuedRunID, QueuedCommandID string
	RunIDs                                  []string
}
type processFixtureStart struct {
	NativeHost                                                                  *readerbroker.FixtureNativeHostReceipt
	Grant                                                                       processFixtureGrant
	PolicyRecovery                                                              *processFixturePolicyRecoveryGrant
	NodePath, ChromePath, DriverPath, Origin, AuthorityKey, SessionID, OldEpoch string
	RunIDs                                                                      []string
	StartupBarrier                                                              string
	ControllerImage                                                             string
}
type processFixtureReceipt struct {
	Frontend                                                                                                                                                                                                                                                                                                            json.RawMessage
	Stage                                                                                                                                                                                                                                                                                                               string
	PID                                                                                                                                                                                                                                                                                                                 uint32
	Epoch, SessionID                                                                                                                                                                                                                                                                                                    string
	RunIDs                                                                                                                                                                                                                                                                                                              []string
	ChromePID                                                                                                                                                                                                                                                                                                           int
	Windows                                                                                                                                                                                                                                                                                                             []uintptr
	Bootstrap                                                                                                                                                                                                                                                                                                           int
	ProfileID                                                                                                                                                                                                                                                                                                           string
	Generation                                                                                                                                                                                                                                                                                                          uint64
	Account, Revision                                                                                                                                                                                                                                                                                                   string
	StaleEpochDenied, DuplicateDenied, CleanupVerified                                                                                                                                                                                                                                                                  bool
	AuthorityDenied, GrantReplayDenied                                                                                                                                                                                                                                                                                  bool
	GracefulBrowserRelease                                                                                                                                                                                                                                                                                              bool
	DevelopmentSession                                                                                                                                                                                                                                                                                                  bool
	CheckpointFencedBeforeOwner, CheckpointAuthorizedAtStartup                                                                                                                                                                                                                                                          bool
	CheckpointID                                                                                                                                                                                                                                                                                                        string
	CheckpointGeneration, PolicyGeneration                                                                                                                                                                                                                                                                              uint64
	StoreFileID                                                                                                                                                                                                                                                                                                         string
	UIPID, HelperPID, ReaderPID, CollectorPID                                                                                                                                                                                                                                                                           uint32
	UIHWND, ReaderHWND                                                                                                                                                                                                                                                                                                  uint64
	Root, Address, RunID                                                                                                                                                                                                                                                                                                string
	UIRect, ButtonRect                                                                                                                                                                                                                                                                                                  DevelopmentFixtureRect
	ButtonVisible, ButtonEnabled, PresentedWithoutFocus, CaptureActive, PhysicalOpen, Readback, ExactForeground, APICompleted, DurableForegroundAudit, CaptureAndReasoningDrained, BootstrapReused, RemainingWindowHeldPause, FinalCloseVerified, APIHeadlessReadback, HelperStartupBlocked, NativeIntent, DurablePause bool
	CaptureCount                                                                                                                                                                                                                                                                                                        int64
	GrantNegatives                                                                                                                                                                                                                                                                                                      int
	NodeIdentity                                                                                                                                                                                                                                                                                                        processFixtureIdentity
	NodeParent                                                                                                                                                                                                                                                                                                          uint32
	NodeInnerJobMember                                                                                                                                                                                                                                                                                                  bool
	NodeMembershipKnown                                                                                                                                                                                                                                                                                                 bool
	InnerJobProcesses                                                                                                                                                                                                                                                                                                   int
	NodeReady                                                                                                                                                                                                                                                                                                           bool
	Failure                                                                                                                                                                                                                                                                                                             string
	HandshakeElapsedMS                                                                                                                                                                                                                                                                                                  int64
	NodeWaitAtReturn                                                                                                                                                                                                                                                                                                    uint32
	LauncherRoot, LauncherChild                                                                                                                                                                                                                                                                                         processFixtureIdentity
	LauncherWorkerRoot, LauncherWorkerChild                                                                                                                                                                                                                                                                             processFixtureIdentity
	LauncherLifetimeMember, LauncherLifetimeNoInherit                                                                                                                                                                                                                                                                   bool
	LauncherTreeMembership, LauncherIsolationVerified                                                                                                                                                                                                                                                                   bool
	PreOwnerClaimDenied, PreProofClaimDenied, QueueCompleted                                                                                                                                                                                                                                                            bool
	PreResumeBrowserProbeOK, PreResumeBrowserAuthenticated                                                                                                                                                                                                                                                              bool
	PreResumeBrowserAccountMatch, PreResumeBrowserRevision                                                                                                                                                                                                                                                              bool
	QueuedRunID, QueuedCommandID                                                                                                                                                                                                                                                                                        string
}

func processFixtureProcessIdentity(pid uint32) (processFixtureIdentity, error) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return processFixtureIdentity{}, err
	}
	defer windows.CloseHandle(h)
	return processFixtureIdentityFromHandle(h)
}

func processFixtureIdentityFromHandle(h windows.Handle) (processFixtureIdentity, error) {
	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(h, &created, &exited, &kernel, &user); err != nil {
		return processFixtureIdentity{}, err
	}
	pid, err := windows.GetProcessId(h)
	if err != nil {
		return processFixtureIdentity{}, err
	}
	return processFixtureIdentity{pid, fmt.Sprintf("%08x%08x", created.HighDateTime, created.LowDateTime)}, nil
}
func processFixtureDirectoryID(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("fixture directory identity unavailable")
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil || !samePath(path, resolved) {
		return "", errors.New("fixture directory resolves elsewhere")
	}
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	h, err := windows.CreateFile(p, windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(h)
	var data windows.ByHandleFileInformation
	if err = windows.GetFileInformationByHandle(h, &data); err != nil {
		return "", err
	}
	if data.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return "", errors.New("fixture directory is a reparse point")
	}
	return fmt.Sprintf("%08x:%08x%08x:%08x%08x", data.VolumeSerialNumber, data.FileIndexHigh, data.FileIndexLow, data.CreationTime.HighDateTime, data.CreationTime.LowDateTime), nil
}
func processFixtureFileID(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("fixture file identity unavailable")
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil || !samePath(path, resolved) {
		return "", errors.New("fixture file resolves elsewhere")
	}
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	h, err := windows.CreateFile(p, windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(h)
	var data windows.ByHandleFileInformation
	if err = windows.GetFileInformationByHandle(h, &data); err != nil {
		return "", err
	}
	if data.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || data.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0 {
		return "", errors.New("fixture file identity is not an ordinary file")
	}
	return fmt.Sprintf("%08x:%08x%08x:%08x%08x", data.VolumeSerialNumber, data.FileIndexHigh, data.FileIndexLow, data.CreationTime.HighDateTime, data.CreationTime.LowDateTime), nil
}

var processFixtureUser = windows.NewLazySystemDLL("user32.dll")

func processFixtureWindowPID(hwnd uintptr) uint32 {
	var pid uint32
	processFixtureUser.NewProc("GetWindowThreadProcessId").Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	return pid
}
func processFixtureWindows(pid uint32) []uintptr {
	var result []uintptr
	callback := syscall.NewCallback(func(hwnd, _ uintptr) uintptr {
		visible, _, _ := processFixtureUser.NewProc("IsWindowVisible").Call(hwnd)
		if visible != 0 && processFixtureWindowPID(hwnd) == pid {
			result = append(result, hwnd)
		}
		return 1
	})
	processFixtureUser.NewProc("EnumWindows").Call(callback, 0)
	return result
}
func processFixtureWindowsGone(handles []uintptr) bool {
	for _, hwnd := range handles {
		present, _, _ := processFixtureUser.NewProc("IsWindow").Call(hwnd)
		if present != 0 {
			return false
		}
	}
	return true
}
func validateProcessFixtureLineage(root string, g *processFixtureGrant) error {
	parent, err := processFixtureProcessIdentity(uint32(os.Getppid()))
	if err != nil {
		return errors.New("fixture parent identity unavailable")
	}
	return validateProcessFixtureLineageForParent(root, g, parent)
}

func validateProcessFixtureLineageForParent(root string, g *processFixtureGrant, parentIdentity processFixtureIdentity) error {
	project, scopeErr := filepath.Abs(filepath.Join("..", ".."))
	if scopeErr != nil {
		return errors.New("fixture project root unavailable")
	}
	qaParent := filepath.Join(project, "runtime", "qa", "auth-session-sidecar")
	resolvedParent, scopeErr := filepath.EvalSymlinks(qaParent)
	if scopeErr != nil || !samePath(qaParent, resolvedParent) {
		return errors.New("fixture QA parent unavailable")
	}
	rel, scopeErr := filepath.Rel(qaParent, root)
	if scopeErr != nil || !filepath.IsAbs(root) || filepath.IsAbs(rel) || strings.Contains(rel, "..") || !strings.HasPrefix(filepath.Base(root), "run-") {
		return errors.New("fixture lineage outside project QA scope")
	}
	if g == nil || g.Root != root || len(g.Nonce) != 64 || strings.Trim(g.Nonce, "0123456789abcdef") != "" {
		return errors.New("fixture lineage invalid")
	}
	if parentIdentity.PID == 0 || parentIdentity.Created == "" || parentIdentity != g.Parent {
		return errors.New("fixture parent identity changed")
	}
	rootID, err := processFixtureDirectoryID(root)
	if err != nil || rootID != g.RootID {
		return errors.New("fixture root identity changed")
	}
	lineagePath := filepath.Join(root, "process-controller-lineage.json")
	lineageInfo, err := os.Lstat(lineagePath)
	if err != nil || !lineageInfo.Mode().IsRegular() || lineageInfo.Mode()&os.ModeSymlink != 0 || lineageInfo.Size() > 4096 {
		return errors.New("fixture lineage file invalid")
	}
	lineageResolved, err := filepath.EvalSymlinks(lineagePath)
	if err != nil || !samePath(lineageResolved, lineagePath) {
		return errors.New("fixture lineage file resolves elsewhere")
	}
	raw, err := os.ReadFile(lineagePath)
	if err != nil {
		return err
	}
	var saved processFixtureLineage
	if json.Unmarshal(raw, &saved) != nil || saved != g.processFixtureLineage {
		return errors.New("fixture lineage does not belong to parent")
	}
	return nil
}
func validateProcessFixtureGrant(root string, g *processFixtureGrant) error {
	if err := validateProcessFixtureLineage(root, g); err != nil {
		return err
	}
	var err error
	if g.Reopen {
		if !g.TreeReleased || g.Old.PID == 0 || g.Old.Created == "" || len(g.Windows) < 2 || !processFixtureWindowsGone(g.Windows) {
			return errors.New("fixture release proof incomplete")
		}
		// PID reuse is conservative denial, never permission to kill or reuse it.
		if _, err = processFixtureProcessIdentity(g.Old.PID); err == nil {
			return errors.New("fixture previous process still exists")
		}
		profileID, idErr := processFixtureDirectoryID(filepath.Join(root, "shared"))
		if idErr != nil || profileID != g.ProfileID {
			return errors.New("fixture profile identity changed")
		}
	} else if g.profilePrepared {
		profileID, idErr := processFixtureDirectoryID(filepath.Join(root, "shared"))
		if idErr != nil || g.ProfileID == "" || profileID != g.ProfileID {
			return errors.New("prepared development profile identity changed")
		}
	} else if g.ProfileID != "" {
		return errors.New("fresh fixture profile identity must be empty")
	} else if _, err = os.Lstat(filepath.Join(root, "shared")); !errors.Is(err, os.ErrNotExist) {
		return errors.New("fixture profile is not fresh")
	}
	return nil
}

func fixturePolicyRecoveryConsumedPath(root, nonce string) string {
	return filepath.Join(root, "process-policy-recovery-consumed-"+nonce+".json")
}

func processFixtureIdentityReleased(expected processFixtureIdentity) error {
	if expected.PID == 0 || expected.Created == "" {
		return errors.New("policy recovery old process identity missing")
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, expected.PID)
	if err != nil {
		return errors.New("policy recovery old process release unavailable")
	}
	defer windows.CloseHandle(h)
	actual, err := processFixtureIdentityFromHandle(h)
	if err != nil || actual != expected {
		return errors.New("policy recovery old process identity changed")
	}
	status, err := windows.WaitForSingleObject(h, 0)
	if err != nil || status != uint32(windows.WAIT_OBJECT_0) {
		return errors.New("policy recovery old process still active")
	}
	return nil
}

func validateProcessFixturePolicyRecoveryGrant(root string, start processFixtureStart) error {
	parent, err := processFixtureProcessIdentity(uint32(os.Getppid()))
	if err != nil {
		return errors.New("policy recovery parent identity unavailable")
	}
	return validateProcessFixturePolicyRecoveryGrantForParent(root, start, parent)
}

func validateProcessFixturePolicyRecoveryGrantForParent(root string, start processFixtureStart, parentIdentity processFixtureIdentity) error {
	g := start.PolicyRecovery
	if g == nil || start.NativeHost != nil || start.Grant.Reopen || start.Grant.Old != (processFixtureIdentity{}) || len(start.Grant.Windows) != 0 || start.Grant.TreeReleased || start.Grant.profilePrepared ||
		start.SessionID != "" || len(start.RunIDs) != 0 || start.StartupBarrier != "" || start.OldEpoch != g.PreviousEpoch || g.Nonce == "" || len(g.Nonce) != 64 || strings.Trim(g.Nonce, "0123456789abcdef") != "" ||
		g.RootID == "" || g.ProfileID == "" || g.StoreFileID == "" || g.CheckpointID == "" || g.PreviousEpoch == "" ||
		g.CheckpointGeneration == 0 || g.PolicyGeneration == 0 || !g.Enabled || g.SessionID == "" || g.QueuedRunID == "" || g.QueuedCommandID == "" || len(g.RunIDs) == 0 {
		return errors.New("policy recovery grant incomplete")
	}
	if len(g.RunIDs) > 8 {
		return errors.New("policy recovery run binding invalid")
	}
	if err := validateProcessFixtureLineageForParent(root, &start.Grant, parentIdentity); err != nil {
		return err
	}
	rootID, err := processFixtureDirectoryID(root)
	if err != nil || rootID != g.RootID || rootID != start.Grant.RootID {
		return errors.New("policy recovery root identity changed")
	}
	profileID, err := processFixtureDirectoryID(filepath.Join(root, "shared"))
	if err != nil || profileID != g.ProfileID || start.Grant.ProfileID != g.ProfileID {
		return errors.New("policy recovery physical profile changed")
	}
	storePath := filepath.Join(root, "fixture.sqlite")
	storeID, err := processFixtureFileID(storePath)
	if err != nil || storeID != g.StoreFileID {
		return errors.New("policy recovery physical Store changed")
	}
	if g.OldWorker == g.OldNode || g.OldWorker == g.OldChrome || g.OldNode == g.OldChrome {
		return errors.New("policy recovery old process identities overlap")
	}
	for _, identity := range []processFixtureIdentity{g.OldWorker, g.OldNode, g.OldChrome} {
		if err := processFixtureIdentityReleased(identity); err != nil {
			return err
		}
	}
	state, err := store.OpenReadOnly(storePath)
	if err != nil {
		return errors.New("policy recovery Store unavailable")
	}
	defer state.Close()
	checkpoint, policyGeneration, enabled, err := state.LoadSyntheticAccountPolicy(context.Background())
	if err != nil || checkpoint.CheckpointID != g.CheckpointID || checkpoint.Generation != g.CheckpointGeneration || checkpoint.OwnerEpoch != g.PreviousEpoch ||
		policyGeneration != g.PolicyGeneration || enabled != g.Enabled {
		return errors.New("policy recovery checkpoint changed")
	}
	if !checkpointAccountsMatchPhysicalScope(checkpoint.ExpectedAccounts, rootID, profileID) {
		return errors.New("policy recovery checkpoint scope changed")
	}
	session, err := state.GetSession(context.Background(), g.SessionID)
	if err != nil || len(session.Runs) != len(g.RunIDs) {
		return errors.New("policy recovery queued session changed")
	}
	queuedRunBound := false
	for i, runID := range g.RunIDs {
		if session.Runs[i].ID != runID {
			return errors.New("policy recovery queued run changed")
		}
		queuedRunBound = queuedRunBound || runID == g.QueuedRunID
	}
	if !queuedRunBound {
		return errors.New("policy recovery command run is outside the queued session")
	}
	queuedRun, err := state.GetRun(context.Background(), g.QueuedRunID)
	if err != nil || queuedRun.SessionID != g.SessionID || queuedRun.BridgeCommandStatus != "queued" {
		return errors.New("policy recovery work is not durably queued")
	}
	commands, err := state.FixtureRunCommandEvidence(context.Background(), g.QueuedRunID)
	if err != nil || len(commands) != 1 || commands[0].ID != g.QueuedCommandID || commands[0].Status != "queued" ||
		commands[0].Collector != "headless" || commands[0].Driver != "headless" || commands[0].ClaimedAt.Valid || commands[0].ClaimedBy.Valid {
		return errors.New("policy recovery queued command changed")
	}
	return nil
}

func checkpointAccountsMatchPhysicalScope(accounts map[domain.Source]store.SyntheticAccountIdentity, rootID, profileID string) bool {
	if len(accounts) != 2 {
		return false
	}
	for _, source := range []domain.Source{domain.SourceX, domain.SourceInstagram} {
		identity, ok := accounts[source]
		if !ok || identity.AccountID != "alpha" || identity.StoreScopeID != rootID || identity.ProfileID != profileID {
			return false
		}
	}
	return true
}

func consumeProcessFixturePolicyRecoveryGrant(root string, start processFixtureStart) error {
	if err := validateProcessFixturePolicyRecoveryGrant(root, start); err != nil {
		return err
	}
	return consumeProcessFixturePolicyRecoveryGrantAfterValidation(root, start)
}

func consumeProcessFixturePolicyRecoveryGrantForParent(root string, start processFixtureStart, parentIdentity processFixtureIdentity) error {
	if err := validateProcessFixturePolicyRecoveryGrantForParent(root, start, parentIdentity); err != nil {
		return err
	}
	return consumeProcessFixturePolicyRecoveryGrantAfterValidation(root, start)
}

func consumeProcessFixturePolicyRecoveryGrantAfterValidation(root string, start processFixtureStart) error {
	claim, err := os.OpenFile(fixturePolicyRecoveryConsumedPath(root, start.PolicyRecovery.Nonce), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return errors.New("policy recovery grant already consumed")
	}
	defer claim.Close()
	return json.NewEncoder(claim).Encode(struct {
		Nonce     string
		WorkerPID int
	}{start.PolicyRecovery.Nonce, os.Getpid()})
}

func processFixturePreservesProfile(start processFixtureStart) bool {
	return start.Grant.Reopen || start.PolicyRecovery != nil
}

func validateProcessFixtureStartScope(root string, start processFixtureStart) error {
	if start.PolicyRecovery != nil {
		return validateProcessFixturePolicyRecoveryGrant(root, start)
	}
	return validateProcessFixtureGrant(root, &start.Grant)
}

func consumeProcessFixtureGrant(root string, g *processFixtureGrant) error {
	if err := validateProcessFixtureGrant(root, g); err != nil {
		return err
	}
	if !g.Reopen {
		return errors.New("fixture recovery grant required")
	}
	claim, err := os.OpenFile(filepath.Join(root, "process-recovery-consumed.json"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return errors.New("fixture recovery grant already consumed")
	}
	defer claim.Close()
	return json.NewEncoder(claim).Encode(struct {
		Nonce     string
		WorkerPID int
	}{g.Nonce, os.Getpid()})
}

// Exercise individual invalid fields against an otherwise valid private grant.
// These checks neither launch processes nor mutate directory/profile identity.
func processFixtureGrantNegatives(root string, g processFixtureGrant) (int, error) {
	mutations := []func(*processFixtureGrant){
		func(v *processFixtureGrant) { v.Parent.Created = "wrong" },
		func(v *processFixtureGrant) { v.Nonce = strings.Repeat("0", 64) },
		func(v *processFixtureGrant) { v.RootID = "wrong" },
		func(v *processFixtureGrant) { v.ProfileID = "wrong" },
		func(v *processFixtureGrant) { v.TreeReleased = false },
		func(v *processFixtureGrant) { v.Old = v.Parent },
		func(v *processFixtureGrant) {
			hwnd, _, _ := processFixtureUser.NewProc("GetDesktopWindow").Call()
			v.Windows = []uintptr{hwnd, hwnd}
		},
	}
	for _, mutate := range mutations {
		bad := g
		mutate(&bad)
		if validateProcessFixtureGrant(root, &bad) == nil {
			return 0, errors.New("invalid recovery grant accepted")
		}
	}
	return len(mutations), nil
}

type processFixtureBackend struct {
	bridge           *chromeFixtureBridge
	current          atomic.Pointer[chromeFixtureProcess]
	entered, release chan struct{}
	calls            atomic.Int64
}

func processFixtureObservation(source domain.Source) domain.Observation {
	return domain.Observation{Source: source, PageURL: "http://127.0.0.1/synthetic", CapturedAt: domain.Now(), Snapshots: []domain.Snapshot{{Blocks: []domain.Block{{EvidenceKey: string(source) + ":synthetic", PlatformID: string(source) + ":synthetic", Author: "Fixture", Text: "Synthetic fixture product update for local testing."}}}}, Coverage: map[string]any{"quality": "complete"}}
}
func (b *processFixtureBackend) Capture(ctx context.Context, source domain.Source, _ map[string]any) (domain.Observation, error) {
	p := b.current.Load()
	if p == nil {
		return domain.Observation{}, errors.New("fixture collector unavailable")
	}
	if err := b.bridge.collect(ctx, p.owner); err != nil {
		return domain.Observation{}, err
	}
	if b.calls.Add(1) == 1 && b.entered != nil {
		close(b.entered)
		select {
		case <-b.release:
		case <-ctx.Done():
			return domain.Observation{}, ctx.Err()
		}
	}
	return processFixtureObservation(source), nil
}
func processFixtureWait(ctx context.Context, predicate func() bool) error {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for !predicate() {
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

// RunAuthSessionProcessFixture accepts its only startup/recovery grant over
// inherited private stdin from its verified actual controller parent.
func RunAuthSessionProcessFixture(input io.Reader, output io.Writer) error {
	return runAuthSessionProcessFixture(input, output, false, nil)
}

func runAuthSessionProcessFixture(input io.Reader, output io.Writer, development bool, composition func(context.Context, *DevelopmentFixtureRuntime) error) (returnErr error) {
	return runAuthSessionProcessFixtureChecked(input, output, development, composition, nil)
}

func runAuthSessionProcessFixtureChecked(input io.Reader, output io.Writer, development bool, composition func(context.Context, *DevelopmentFixtureRuntime) error, validateStart func(processFixtureStart) error) (returnErr error) {
	return runAuthSessionProcessFixtureConfigured(input, output, development, composition, validateStart, nil)
}

// Private hooks let the dual-tag policy composition fence before Node and own
// startup instead of passing through the already-authorized legacy binder.
type processFixturePolicyHooks struct {
	prepare func(context.Context, *Engine, processFixtureStart, *bufio.Scanner, *json.Encoder) error
	run     func(context.Context, *DevelopmentFixtureRuntime, processFixtureStart) error
}

func runAuthSessionProcessFixtureConfigured(input io.Reader, output io.Writer, development bool, composition func(context.Context, *DevelopmentFixtureRuntime) error, validateStart func(processFixtureStart) error, policy *processFixturePolicyHooks) (returnErr error) {
	if policy != nil && (!development || composition != nil || policy.prepare == nil || policy.run == nil) {
		return errors.New("fixture policy composition invalid")
	}
	executable, err := os.Executable()
	expectedImage := "auth-session-process-fixture.exe"
	if development {
		expectedImage = "aku-sidecar-dev.exe"
		if composition != nil {
			expectedImage = "akusidecar.exe"
		}
	}
	if err != nil || !strings.EqualFold(filepath.Base(executable), expectedImage) {
		return errors.New("fixture worker image invalid")
	}
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 4096), 64*1024)
	encoder := json.NewEncoder(output)
	if err = encoder.Encode(processFixtureReceipt{Stage: "gated", PID: uint32(os.Getpid())}); err != nil {
		return err
	}
	if !scanner.Scan() {
		return errors.New("fixture startup grant missing")
	}
	var start processFixtureStart
	if err = json.Unmarshal(scanner.Bytes(), &start); err != nil {
		return errors.New("fixture startup frame invalid")
	}
	if start.PolicyRecovery != nil && policy == nil {
		return errors.New("policy recovery grant requires its isolated main composition")
	}
	if err = validateProcessFixtureStartScope(start.Grant.Root, start); err != nil {
		return err
	}
	if development {
		if err = validateDevelopmentController(start); err != nil {
			return err
		}
	}
	if !samePath(filepath.Dir(executable), filepath.Dir(start.Grant.Root)) {
		return errors.New("fixture worker image outside controller run")
	}
	if development && start.StartupBarrier != "" {
		return errors.New("development runner rejects fault modes")
	}
	if validateStart != nil {
		if err := validateStart(start); err != nil {
			return err
		}
	} else if start.NativeHost != nil {
		return errors.New("native frontend receipt requires its explicit provider")
	}
	if (start.StartupBarrier == "launcher_before_job" || start.StartupBarrier == "launcher_abort_before_job" || start.StartupBarrier == "launcher_resumed_tree") && !start.Grant.Reopen {
		return runProcessFixtureLauncher(scanner, encoder, executable, start)
	}
	if start.StartupBarrier != "" && (start.StartupBarrier != "node_ready_before_job" && start.StartupBarrier != "node_before_ready" && start.StartupBarrier != "node_stalled_timeout" || start.Grant.Reopen) {
		return errors.New("fixture startup barrier invalid")
	}
	grantNegatives := 0
	policyGrantReplayDenied := false
	if start.Grant.Reopen {
		grantNegatives, err = processFixtureGrantNegatives(start.Grant.Root, start.Grant)
		if err != nil {
			return err
		}
		if err = consumeProcessFixtureGrant(start.Grant.Root, &start.Grant); err != nil {
			return err
		}
		if consumeProcessFixtureGrant(start.Grant.Root, &start.Grant) == nil {
			return errors.New("replayed recovery grant accepted")
		}
	} else if start.PolicyRecovery != nil {
		if err = consumeProcessFixturePolicyRecoveryGrant(start.Grant.Root, start); err != nil {
			return err
		}
		if consumeProcessFixturePolicyRecoveryGrant(start.Grant.Root, start) == nil {
			return errors.New("replayed policy recovery grant accepted")
		}
		policyGrantReplayDenied = true
	}
	lifetime := 90 * time.Second
	if composition != nil {
		lifetime = 10 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), lifetime)
	defer cancel()
	settings := domain.DefaultSettings("expanded", "quiet", "guarded_live", true)
	settings.CollectionMode = "headless"
	settings.ActiveSources = []domain.Source{domain.SourceX, domain.SourceInstagram}
	settings.CalibrationEnabled = false
	var state *store.Store
	var e *Engine
	var coordinatorDone <-chan struct{}
	resourcesCloseDeferred := false
	defer func() {
		if resourcesCloseDeferred {
			return
		}
		if e != nil {
			e.Shutdown()
		}
		if state != nil {
			_ = state.Close()
		}
	}()
	var provider reasoning.Provider = reasoning.Deterministic{}
	var reasoningGate *developmentFixtureReasoningGate
	checkpointFencedBeforeOwner := false
	if development {
		profilePath := filepath.Join(start.Grant.Root, "shared")
		if !processFixturePreservesProfile(start) {
			if err = os.Mkdir(profilePath, 0700); err != nil {
				return errors.New("development profile preparation failed")
			}
			start.Grant.ProfileID, err = processFixtureDirectoryID(profilePath)
			if err != nil {
				return errors.New("development profile identity unavailable")
			}
			start.Grant.profilePrepared = true
		} else {
			profileID, profileErr := processFixtureDirectoryID(profilePath)
			if profileErr != nil || profileID != start.Grant.ProfileID {
				return errors.New("development profile identity changed")
			}
		}
		state, err = store.Open(filepath.Join(start.Grant.Root, "fixture.sqlite"), settings)
		if err != nil {
			return err
		}
		if !processFixturePreservesProfile(start) {
			if _, err = state.CompleteOnboarding(ctx, settings.ActiveSources); err != nil {
				return err
			}
		}
		if composition != nil {
			reasoningGate = &developmentFixtureReasoningGate{entered: make(chan struct{}), release: make(chan struct{})}
			provider = reasoningGate
		}
		e = New(state, provider, config.Config{Dev: true, Root: start.Grant.Root, Database: config.DatabaseConfig{Path: filepath.Join(start.Grant.Root, "fixture.sqlite")}, Capture: config.CaptureConfig{MaxAcquisitionRounds: 1}}, log.New(io.Discard, "", 0))
		e.RecordHeartbeat(ExpectedHeartbeat())
		if policy != nil {
			if start.PolicyRecovery != nil && !policyGrantReplayDenied {
				return errors.New("policy recovery replay denial was not proven")
			}
			err = policy.prepare(ctx, e, start, scanner, encoder)
		} else {
			err = prepareDevelopmentProcessCheckpoint(ctx, e, start.Grant.RootID, start.Grant.ProfileID, !processFixturePreservesProfile(start))
		}
		if err != nil {
			return fmt.Errorf("development checkpoint preparation failed: %w", err)
		}
		checkpointFencedBeforeOwner = true
	}
	bridgeConfig := chromeFixtureConfig{NodePath: start.NodePath, ChromePath: start.ChromePath, DriverPath: start.DriverPath, Root: start.Grant.Root, Origin: start.Origin, processGrant: &start.Grant, processStart: &start}
	bridgeConfig.NativeReaderStartup = composition != nil
	var barrierNode windows.Handle
	barrierReported := make(chan struct{})
	defer func() {
		select {
		case <-barrierReported:
			if barrierNode != 0 {
				_ = windows.CloseHandle(barrierNode)
			}
		default:
		}
	}()
	if start.StartupBarrier == "node_before_ready" || start.StartupBarrier == "node_stalled_timeout" {
		if development && !checkpointFencedBeforeOwner {
			return errors.New("development checkpoint did not fence before Node startup")
		}
		bridgeConfig.beforeReadySignal = func(pid uint32, job windows.Handle) error {
			defer close(barrierReported)
			identity, identityErr := processFixtureProcessIdentity(pid)
			if identityErr != nil {
				return identityErr
			}
			parent, parentErr := readerbroker.ParentPID(pid)
			if parentErr != nil || parent != uint32(os.Getpid()) {
				return errors.New("fixture Node parent mismatch")
			}
			var openErr error
			barrierNode, openErr = windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, pid)
			if openErr != nil {
				return openErr
			}
			member, membershipErr := chromeFixtureJobMembership(barrierNode, job)
			if membershipErr != nil || member {
				return errors.New("fixture Node assigned before ready")
			}
			pids, queryErr := (&chromeFixtureBridge{job: job}).jobPIDs()
			if queryErr != nil || len(pids) != 0 {
				return errors.New("fixture inner Job not empty before ready")
			}
			return encoder.Encode(processFixtureReceipt{Stage: "node_stalled_before_ready", PID: uint32(os.Getpid()), NodeIdentity: identity, NodeParent: parent, NodeMembershipKnown: true, InnerJobProcesses: len(pids), NodeReady: false})
		}
	}
	if start.StartupBarrier == "node_ready_before_job" {
		bridgeConfig.beforeJobAssignment = func(pid uint32, job windows.Handle) error {
			identity, identityErr := processFixtureProcessIdentity(pid)
			if identityErr != nil {
				return identityErr
			}
			parent, parentErr := readerbroker.ParentPID(pid)
			if parentErr != nil || parent != uint32(os.Getpid()) {
				return errors.New("fixture Node parent mismatch")
			}
			h, openErr := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
			if openErr != nil {
				return openErr
			}
			defer windows.CloseHandle(h)
			member, membershipErr := chromeFixtureJobMembership(h, job)
			if membershipErr != nil || member {
				return errors.New("fixture Node already assigned before barrier")
			}
			pids, queryErr := (&chromeFixtureBridge{job: job}).jobPIDs()
			if queryErr != nil || len(pids) != 0 {
				return errors.New("fixture inner Job not empty before barrier")
			}
			if encodeErr := encoder.Encode(processFixtureReceipt{Stage: "node_ready_unassigned", PID: uint32(os.Getpid()), NodeIdentity: identity, NodeParent: parent, NodeInnerJobMember: member, NodeMembershipKnown: true, InnerJobProcesses: len(pids), NodeReady: true}); encodeErr != nil {
				return encodeErr
			}
			// Deterministic private test boundary, never a timing-based sleep or
			// an ordinary setting. The controller kills this actual Go process.
			if !scanner.Scan() || string(scanner.Bytes()) != "continue_node_assignment" {
				return errors.New("fixture startup barrier continuation missing")
			}
			return nil
		}
	}
	if development && !checkpointFencedBeforeOwner {
		return errors.New("development checkpoint did not fence before Node startup")
	}
	handshakeStarted := time.Now()
	bridge, err := startChromeFixtureBridge(ctx, bridgeConfig)
	if err != nil {
		if start.StartupBarrier == "node_before_ready" || start.StartupBarrier == "node_stalled_timeout" {
			select {
			case <-barrierReported:
			case <-time.After(2 * time.Second):
				return errors.New("fixture pre-ready observation unsettled")
			}
		}
		if start.StartupBarrier == "node_stalled_timeout" && err.Error() == "fixture bridge handshake timed out" && barrierNode != 0 {
			status, waitErr := windows.WaitForSingleObject(barrierNode, 0)
			if waitErr != nil {
				return waitErr
			}
			if encodeErr := encoder.Encode(processFixtureReceipt{Stage: "handshake_timeout", PID: uint32(os.Getpid()), Failure: err.Error(), HandshakeElapsedMS: time.Since(handshakeStarted).Milliseconds(), NodeWaitAtReturn: status}); encodeErr != nil {
				return encodeErr
			}
			if !scanner.Scan() || string(scanner.Bytes()) != "exit_after_timeout" {
				return errors.New("fixture timeout exit gate missing")
			}
		}
		return err
	}
	defer func() {
		bridge.terminateOwnedTree()
		if bridge.cleanupError() != nil || !bridge.cleanupVerified() {
			returnErr = errors.New("fixture worker cleanup not verified")
		}
	}()
	initial, err := bridge.launch(ctx, "collector")
	if err != nil {
		return err
	}
	if !processFixturePreservesProfile(start) {
		if err = initial.Login(ctx); err != nil {
			return err
		}
	}
	backend := &processFixtureBackend{bridge: bridge}
	backend.current.Store(initial)
	if !start.Grant.Reopen {
		backend.entered = make(chan struct{})
		backend.release = make(chan struct{})
	}
	manager, err := captureruntime.New(initial)
	if err != nil {
		return err
	}
	defer manager.Terminate()
	if !development {
		state, err = store.Open(filepath.Join(start.Grant.Root, "fixture.sqlite"), settings)
		if err != nil {
			return err
		}
		if !start.Grant.Reopen {
			if _, err = state.CompleteOnboarding(ctx, settings.ActiveSources); err != nil {
				return err
			}
		}
		if composition != nil {
			reasoningGate = &developmentFixtureReasoningGate{entered: make(chan struct{}), release: make(chan struct{})}
			provider = reasoningGate
		}
		e = New(state, provider, config.Config{Dev: true, Root: start.Grant.Root, Database: config.DatabaseConfig{Path: filepath.Join(start.Grant.Root, "fixture.sqlite")}, Capture: config.CaptureConfig{MaxAcquisitionRounds: 1}}, log.New(io.Discard, "", 0))
		e.RecordHeartbeat(ExpectedHeartbeat())
	}
	defer func() {
		cancel()
		e.Shutdown()
		done := coordinatorDone
		if done == nil {
			done = e.developmentCoordinatorDone
		}
		if done != nil {
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				returnErr = errors.New("fixture coordinator shutdown incomplete")
			}
		}
		if !e.WaitForIdle(3 * time.Second) {
			returnErr = errors.New("fixture reasoning shutdown incomplete")
		}
		if err := state.Close(); err != nil {
			returnErr = errors.New("fixture Store shutdown incomplete")
		}
	}()
	resourcesCloseDeferred = true
	if err = e.AttachCaptureRuntime(ctx, manager); err != nil {
		return err
	}
	var coordinator *collection.Coordinator
	coordinator = collection.NewCoordinator(manager, func(launchCtx context.Context, mode string, generation uint64) (captureruntime.Process, error) {
		role := "collector"
		if mode == "native_reader" {
			role = "owner"
		} else if mode != "headless" {
			return nil, errors.New("unexpected fixture role")
		}
		p, launchErr := bridge.launch(launchCtx, role)
		if launchErr == nil {
			backend.current.Store(p)
			if mode == "headless" {
				coordinator.SetSyntheticHeadlessCollector(generation, backend)
			}
		}
		return p, launchErr
	}, func() error { return nil })
	coordinator.SetSyntheticHeadlessCollector(manager.Snapshot().Generation, backend)
	e.AttachCollectionCoordinator(coordinator)
	authority := func(probeCtx context.Context) (SyntheticSessionEvidence, error) {
		req, requestErr := http.NewRequestWithContext(probeCtx, http.MethodGet, start.Origin+"/process-authority", nil)
		if requestErr != nil {
			return SyntheticSessionEvidence{}, requestErr
		}
		req.Header.Set("X-Fixture-Key", start.AuthorityKey)
		client := http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("fixture authority redirect denied") }}
		reply, requestErr := client.Do(req)
		if requestErr != nil {
			return SyntheticSessionEvidence{}, requestErr
		}
		defer reply.Body.Close()
		if reply.StatusCode != 200 {
			return SyntheticSessionEvidence{}, errors.New("fixture authority denied")
		}
		var evidence SyntheticSessionEvidence
		requestErr = json.NewDecoder(io.LimitReader(reply.Body, 4096)).Decode(&evidence)
		return evidence, requestErr
	}
	if policy != nil {
		return policy.run(ctx, &DevelopmentFixtureRuntime{Engine: e, State: state, Owner: manager, Coordinator: coordinator, Root: start.Grant.Root, backend: backend, bridge: bridge, scanner: scanner, encoder: encoder, authority: authority}, start)
	}
	if start.Grant.Reopen {
		paused, pausedErr := state.ListFixturePausedCaptureSessions(ctx)
		if pausedErr != nil || !processFixtureHasSession(paused, start.SessionID) {
			return errors.New("restart lost durable pause")
		}
		if e.Epoch() == start.OldEpoch {
			return errors.New("restart reused engine epoch")
		}
	}
	browserProbe := func(probeCtx context.Context) (SyntheticSessionEvidence, error) {
		process := backend.current.Load()
		if process == nil || (development && process.PID() != manager.PID()) {
			return SyntheticSessionEvidence{}, errors.New("development actual browser owner mismatch")
		}
		v, err := process.Evidence(probeCtx)
		if development && (backend.current.Load() != process || process.PID() != manager.PID()) {
			return SyntheticSessionEvidence{}, errors.New("development actual browser owner changed")
		}
		return v, err
	}
	authorityDenied := false
	if start.Grant.Reopen {
		if err = bindProcessSession(ctx, e, manager, coordinator, start, authority, browserProbe, development); err == nil || err.Error() != "synthetic session verification required" {
			return errors.New("lost authority did not deny restart binding")
		}
		paused, pauseErr := state.ListFixturePausedCaptureSessions(ctx)
		browser, browserErr := browserProbe(ctx)
		if pauseErr != nil || !processFixtureHasSession(paused, start.SessionID) || browserErr != nil || !browser.Authenticated || browser.AccountID != "alpha" {
			return errors.New("authority denial lost persisted pause or browser lineage")
		}
		authorityDenied = true
		if err = encoder.Encode(processFixtureReceipt{Stage: "authority_denied", PID: uint32(os.Getpid()), Epoch: e.Epoch(), AuthorityDenied: true, CheckpointFencedBeforeOwner: checkpointFencedBeforeOwner}); err != nil {
			return err
		}
		if !scanner.Scan() || string(scanner.Bytes()) != "authorize" {
			return errors.New("fixture fresh authority gate missing")
		}
	}
	if err = bindProcessSession(ctx, e, manager, coordinator, start, authority, browserProbe, development); err != nil {
		return fmt.Errorf("fixture binder: %w", err)
	}
	checkpointAuthorizedAtStartup := developmentCheckpointAuthorizedAtStartup(e)
	if development && !checkpointAuthorizedAtStartup {
		return errors.New("development checkpoint authorization missing after startup binding")
	}
	if !development {
		coordinatorDone = coordinator.StartWithDone(ctx)
		coordinator.Request("headless")
	}
	if err = processFixtureWait(ctx, func() bool { s := coordinator.Status(); return s.Effective == "headless" && !s.BatchPaused }); err != nil {
		return err
	}
	if composition != nil {
		if !development || start.Grant.Reopen {
			return errors.New("development broker requires fresh scoped grant")
		}
		return composition(ctx, &DevelopmentFixtureRuntime{Engine: e, State: state, Owner: manager, Coordinator: coordinator, Root: start.Grant.Root, NativeHost: start.NativeHost, backend: backend, bridge: bridge, reasoning: reasoningGate, scanner: scanner, encoder: encoder, authority: authority})
	}
	evidence, err := backend.current.Load().Evidence(ctx)
	if err != nil {
		return err
	}
	receipt := processFixtureReceipt{PID: uint32(os.Getpid()), Epoch: e.Epoch(), Generation: manager.Snapshot().Generation, Account: evidence.AccountID, Revision: evidence.Revision, AuthorityDenied: authorityDenied, GrantNegatives: grantNegatives, GrantReplayDenied: start.Grant.Reopen, DevelopmentSession: coordinator.Status().DevelopmentSession, CheckpointFencedBeforeOwner: checkpointFencedBeforeOwner, CheckpointAuthorizedAtStartup: checkpointAuthorizedAtStartup}
	if !start.Grant.Reopen {
		session, startErr := e.StartVisibleUpdate(ctx, "full fixture OS process restart")
		if startErr != nil {
			return startErr
		}
		if len(session.Runs) != 2 {
			return errors.New("fixture continuation plan changed")
		}
		e.StartHeadlessCollection(ctx)
		select {
		case <-backend.entered:
		case <-ctx.Done():
			return ctx.Err()
		}
		type readerResult struct {
			lease   *captureruntime.Lease
			release func()
			err     error
		}
		ready := make(chan readerResult, 1)
		go func() { l, r, borrowErr := e.BorrowNativeReader(ctx); ready <- readerResult{l, r, borrowErr} }()
		if err = processFixtureWait(ctx, func() bool { return coordinator.Status().BatchPaused }); err != nil {
			return err
		}
		close(backend.release)
		var reader readerResult
		select {
		case reader = <-ready:
		case <-ctx.Done():
			return ctx.Err()
		}
		if reader.err != nil {
			return reader.err
		}
		defer reader.release()
		defer reader.lease.Release()
		if !e.WaitForIdle(5 * time.Second) {
			return errors.New("fixture active capture did not drain")
		}
		first, firstErr := state.GetRun(ctx, session.Runs[0].ID)
		second, secondErr := state.GetRun(ctx, session.Runs[1].ID)
		if firstErr != nil || secondErr != nil || first.Status != "completed" || second.Status != "waiting_for_bridge" {
			return errors.New("fixture durable run state invalid")
		}
		paused, pausedErr := state.ListFixturePausedCaptureSessions(ctx)
		if pausedErr != nil || !processFixtureHasSession(paused, session.ID) {
			return errors.New("fixture durable pause missing")
		}
		if command, claimErr := e.claimCommandForCollector(ctx, second.ID, "fixture-paused", "headless", "headless"); claimErr != nil || command != nil {
			return errors.New("fixture paused command admitted")
		}
		owner := backend.current.Load()
		readerEvidence, readerEvidenceErr := owner.Evidence(ctx)
		if readerEvidenceErr != nil || !readerEvidence.Authenticated || readerEvidence.AccountID != evidence.AccountID || readerEvidence.Revision != evidence.Revision {
			return errors.New("reader lost synthetic account lineage")
		}
		count, bootstrap, inventoryErr := owner.FixtureInventory(ctx)
		if inventoryErr != nil || count != 2 || bootstrap != 1 {
			return errors.New("fixture reader inventory invalid")
		}
		handles := processFixtureWindows(uint32(owner.PID()))
		if len(handles) != count {
			return errors.New("native fixture window inventory mismatch")
		}
		profileID, idErr := processFixtureDirectoryID(filepath.Join(start.Grant.Root, "shared"))
		if idErr != nil {
			return idErr
		}
		receipt.Stage = "paused_reader"
		receipt.SessionID = session.ID
		receipt.RunIDs = []string{first.ID, second.ID}
		receipt.ChromePID = owner.PID()
		receipt.Windows = handles
		receipt.Bootstrap = bootstrap
		receipt.ProfileID = profileID
	} else {
		session, sessionErr := state.GetSession(ctx, start.SessionID)
		if sessionErr != nil || len(session.Runs) != 2 || len(start.RunIDs) != 2 || session.Runs[0].ID != start.RunIDs[0] || session.Runs[1].ID != start.RunIDs[1] {
			return errors.New("restart changed durable IDs")
		}
		command, claimErr := e.claimCommandForCollector(ctx, start.RunIDs[1], "fixture-restart", "headless", "headless")
		if claimErr != nil || command == nil {
			return errors.New("restart continuation not admitted")
		}
		if err = e.validateCaptureOwner(start.SessionID, command.Payload); err != nil {
			return errors.New("current restart tuple rejected")
		}
		copied, _ := json.Marshal(command.Payload)
		var stale map[string]any
		_ = json.Unmarshal(copied, &stale)
		stamp, ok := stale["captureRuntime"].(map[string]any)
		if !ok {
			return errors.New("restart tuple absent")
		}
		stamp["epoch"] = start.OldEpoch
		if err = e.validateCaptureOwner(start.SessionID, stale); err == nil {
			return errors.New("stale epoch accepted")
		}
		receipt.StaleEpochDenied = true
		observation, captureErr := backend.Capture(ctx, session.Runs[1].Source, command.Payload)
		if captureErr != nil {
			return captureErr
		}
		if _, err = e.AcceptObservation(ctx, command.ID, start.RunIDs[1], observation); err != nil {
			return err
		}
		if _, err = e.AcceptObservation(ctx, command.ID, start.RunIDs[1], observation); err == nil {
			return errors.New("duplicate observation accepted")
		}
		receipt.DuplicateDenied = true
		if !e.WaitForIdle(5 * time.Second) {
			return errors.New("restart continuation did not drain")
		}
		for _, runID := range start.RunIDs {
			run, runErr := state.GetRun(ctx, runID)
			if runErr != nil || run.Status != "completed" {
				return errors.New("restart run not completed")
			}
		}
		paused, pausedErr := state.ListFixturePausedCaptureSessions(ctx)
		if pausedErr != nil || processFixtureHasSession(paused, start.SessionID) {
			return errors.New("restart pause not cleared by verified binder hooks")
		}
		receipt.Stage = "recovered"
		receipt.SessionID = start.SessionID
		receipt.RunIDs = start.RunIDs
		receipt.ChromePID = initial.PID()
	}
	if err = encoder.Encode(receipt); err != nil {
		return err
	}
	if !scanner.Scan() || string(scanner.Bytes()) != "stop" {
		return errors.New("fixture stop frame missing")
	}
	e.Shutdown()
	cancel()
	manager.Terminate()
	bridge.terminateOwnedTree()
	if bridge.cleanupError() != nil || !bridge.cleanupVerified() {
		return errors.New("fixture graceful tree cleanup unknown")
	}
	return encoder.Encode(processFixtureReceipt{Stage: "stopped", PID: uint32(os.Getpid()), CleanupVerified: true})
}
func processFixtureHasSession(ids []string, expected string) bool {
	for _, id := range ids {
		if id == expected {
			return true
		}
	}
	return false
}
