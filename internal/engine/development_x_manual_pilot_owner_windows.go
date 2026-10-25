//go:build windows && auth_session_dev && auth_session_fixture

package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Supplied only by the already-owned private QA setup. This does not launch a
// process, acquire a retained lease, open a profile or transfer credentials.
// Each role has independent Chrome/profile pins; Node and Job may be shared.
type xManualPilotWindowsRole struct {
	role                             string
	bridge                           *chromeFixtureBridge
	protocolOwner                    int
	node, chrome, parent             processFixtureIdentity
	chromeHandle                     windows.Handle
	root, rootID, profile, profileID string
	rootPin, profilePin              windows.Handle
	generation                       uint64
}
type xManualPilotWindowsBackend struct {
	roles map[string]xManualPilotWindowsRole
}

func (xManualPilotWindowsRole) String() string         { return "private-x-manual-windows-role" }
func (v xManualPilotWindowsRole) GoString() string     { return v.String() }
func (*xManualPilotWindowsBackend) String() string     { return "private-x-manual-windows-backend" }
func (v *xManualPilotWindowsBackend) GoString() string { return v.String() }
func (*xManualPilotWindowsBackend) provenance() nativePilotProvenance {
	return nativePilotTrustedStartup
}
func (*xManualPilotWindowsBackend) now() time.Time { return time.Now() }

func xManualPilotWindowsImage(identity processFixtureIdentity, path string) error {
	actual, err := processFixtureProcessIdentity(identity.PID)
	if err != nil || actual != identity {
		return errors.New("x-owner-process-changed")
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, identity.PID)
	if err != nil {
		return errors.New("x-owner-image-unavailable")
	}
	defer windows.CloseHandle(h)
	buffer := make([]uint16, 32768)
	size := uint32(len(buffer))
	if windows.QueryFullProcessImageName(h, 0, &buffer[0], &size) != nil {
		return errors.New("x-owner-image-unavailable")
	}
	resolved, err := filepath.EvalSymlinks(windows.UTF16ToString(buffer[:size]))
	expected, expectedErr := filepath.EvalSymlinks(path)
	if err != nil || expectedErr != nil || !samePath(resolved, expected) {
		return errors.New("x-owner-image-changed")
	}
	return nil
}
func xManualPilotWindowsProcessParents(visit func(uint32, uint32) bool) error {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return errors.New("x-owner-parent-unavailable")
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		if !visit(entry.ProcessID, entry.ParentProcessID) {
			return nil
		}
	}
	return errors.New("x-owner-parent-unavailable")
}
func xManualPilotWindowsDirectory(handle windows.Handle, path, id string) error {
	if handle == 0 || !filepath.IsAbs(path) || id == "" {
		return errors.New("x-owner-directory-unavailable")
	}
	var info windows.ByHandleFileInformation
	if windows.GetFileInformationByHandle(handle, &info) != nil || info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return errors.New("x-owner-directory-unavailable")
	}
	pinned := fmt.Sprintf("%08x:%08x%08x:%08x%08x", info.VolumeSerialNumber, info.FileIndexHigh, info.FileIndexLow, info.CreationTime.HighDateTime, info.CreationTime.LowDateTime)
	actual, err := processFixtureDirectoryID(path)
	if err != nil || pinned != id || actual != id {
		return errors.New("x-owner-directory-changed")
	}
	return nil
}
func xManualPilotWindowsScope(record xManualPilotWindowsRole) bool {
	// The bridge's own constructor has already constrained its root to this
	// checkout's QA parent. The collector may use a sibling owned control root.
	parent := record.bridge.config.Root
	for filepath.Base(parent) != "auth-session-sidecar" {
		next := filepath.Dir(parent)
		if next == parent {
			return false
		}
		parent = next
	}
	if filepath.Base(filepath.Dir(parent)) != "qa" || filepath.Base(filepath.Dir(filepath.Dir(parent))) != "runtime" {
		return false
	}
	for _, path := range []string{record.root, record.profile} {
		rel, err := filepath.Rel(parent, path)
		if err != nil || rel == "." || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return false
		}
	}
	rel, err := filepath.Rel(record.root, record.profile)
	return err == nil && rel != "." && !filepath.IsAbs(rel) && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
func (b *xManualPilotWindowsBackend) snapshot(ctx context.Context, role string) (nativePilotOwner, error) {
	r, ok := b.roles[role]
	if !ok || ctx == nil || ctx.Err() != nil || r.bridge == nil || r.bridge.isClosed() || r.role != role || r.protocolOwner <= 0 || r.node.PID != r.bridge.pid || r.parent.PID != uint32(os.Getpid()) || r.chromeHandle == 0 || r.generation == 0 || !xManualPilotWindowsScope(r) {
		return nativePilotOwner{}, errors.New("x-owner-windows-unavailable")
	}
	actualParent, err := processFixtureProcessIdentity(r.parent.PID)
	if err != nil || actualParent != r.parent {
		return nativePilotOwner{}, errors.New("x-owner-parent-changed")
	}
	identity, err := processFixtureIdentityFromHandle(r.chromeHandle)
	status, waitErr := windows.WaitForSingleObject(r.chromeHandle, 0)
	if err != nil || identity != r.chrome || waitErr != nil || status != uint32(windows.WAIT_TIMEOUT) {
		return nativePilotOwner{}, errors.New("x-owner-chrome-changed")
	}
	if xManualPilotWindowsImage(r.node, r.bridge.config.NodePath) != nil || xManualPilotWindowsImage(r.chrome, r.bridge.config.ChromePath) != nil {
		return nativePilotOwner{}, errors.New("x-owner-image-or-creation-changed")
	}
	nodeParent, chromeParent, err := xManualPilotParentPair(r.node.PID, r.chrome.PID, xManualPilotWindowsProcessParents)
	if err != nil || nodeParent != r.parent.PID {
		return nativePilotOwner{}, errors.New("x-owner-node-parent-changed")
	}
	if chromeParent != r.node.PID {
		return nativePilotOwner{}, errors.New("x-owner-chrome-parent-changed")
	}
	r.bridge.jobMu.Lock()
	job := r.bridge.job
	for _, pid := range []uint32{r.node.PID, r.chrome.PID} {
		h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
		if err != nil {
			r.bridge.jobMu.Unlock()
			return nativePilotOwner{}, errors.New("x-owner-job-unavailable")
		}
		member, err := chromeFixtureJobMembership(h, job)
		windows.CloseHandle(h)
		if err != nil || !member {
			r.bridge.jobMu.Unlock()
			return nativePilotOwner{}, errors.New("x-owner-job-changed")
		}
	}
	r.bridge.jobMu.Unlock()
	if xManualPilotWindowsDirectory(r.rootPin, r.root, r.rootID) != nil || xManualPilotWindowsDirectory(r.profilePin, r.profile, r.profileID) != nil {
		return nativePilotOwner{}, errors.New("x-owner-physical-directory-changed")
	}
	if role == "collector" && len(processFixtureWindows(r.chrome.PID)) != 0 {
		return nativePilotOwner{}, errors.New("x-owner-collector-not-headless")
	}
	return nativePilotOwner{role: role, owner: strconv.FormatUint(uint64(r.chrome.PID), 10), profile: r.profile, directory: r.profileID, image: r.bridge.config.ChromePath, creation: r.chrome.Created, job: strconv.FormatUint(uint64(job), 10), parent: strconv.FormatUint(uint64(r.parent.PID), 10) + "/" + r.parent.Created, pid: int(r.chrome.PID), generation: r.generation}, nil
}
func (b *xManualPilotWindowsBackend) rpc(ctx context.Context, role, op string, fields map[string]any, out any) error {
	r, ok := b.roles[role]
	if !ok || r.bridge == nil || ctx == nil || ctx.Err() != nil {
		return errors.New("x-owner-transport-unavailable")
	}
	switch op {
	case "manual_prepare", "manual_acquire", "manual_inspect", "manual_inventory", "manual_abort", "manual_capture", "manual_capture_cancel":
	default:
		return errors.New("x-owner-operation-unavailable")
	}
	input := map[string]any{"owner": r.protocolOwner, "role": role}
	for key, value := range fields {
		if key != "handle" && key != "request" && key != "preserveReader" && !(op == "manual_acquire" && key == "startupBudgetMS") {
			return errors.New("x-owner-operation-unavailable")
		}
		input[key] = value
	}
	// Only this fixed role protocol bypasses rpcMu. Both roles must enter the
	// initial identity rendezvous together on a shared Node. Writes and replies
	// retain the existing bridge's per-message correlation and pipe locks.
	progressCount := 0
	if err := r.bridge.rpcRoundTripObserved(ctx, op, input, out, func(raw []byte) {
		if progressCount < 32 {
			progressCount++
			nativePilotTimingProgress(ctx, role, raw)
		}
	}); err != nil {
		if op == "manual_capture" {
			switch err.Error() {
			case "manual_capture_challenge_cloudflare", "manual_capture_challenge_x_human_verification", "manual_capture_challenge_unknown":
				return err
			default:
				return errors.New("native X capture transport rejected")
			}
		}
		if detail, ok := nativePilotInitialCode(err.Error()); ok {
			return &nativePilotInitialError{detail}
		}
		if ctx.Err() != nil {
			return nativePilotInitialWrap(ctx.Err(), "pair", "joint")
		}
		return nativePilotInitialWrap(nil, "unknown", "acquire_rpc")
	}
	return nil
}
func (b *xManualPilotWindowsBackend) cleanup(ctx context.Context, role, handle string, preserve bool) error {
	var token any = handle
	if handle == "" {
		token = nil
	}
	var result struct {
		Closed *bool `json:"closed"`
	}
	var raw json.RawMessage
	if b.rpc(ctx, role, "manual_abort", map[string]any{"handle": token, "preserveReader": preserve && role == "reader"}, &raw) != nil {
		return errors.New("x-owner-cleanup-uncertain")
	}
	defer clear(raw)
	if xManualPilotDecode(raw, &result) != nil || result.Closed == nil || !*result.Closed {
		return errors.New("x-owner-cleanup-uncertain")
	}
	return nil
}

// Default-off, private QA construction only. Provenance is fixed by this native
// implementation, not selected from config/JSON. No caller is registered here.
func newXManualPilotOwnerBroker(parent context.Context, cfg xManualPilotOwnerConfig, reader, collector xManualPilotWindowsRole) (*nativePilotBroker, nativePilotDiagnostic) {
	b, _, d := newXManualPilotOwnerCaptureBroker(parent, cfg, reader, collector)
	return b, d
}
func newXManualPilotOwnerCaptureBroker(parent context.Context, cfg xManualPilotOwnerConfig, reader, collector xManualPilotWindowsRole) (*nativePilotBroker, *xManualPilotOwnerPorts, nativePilotDiagnostic) {
	if !cfg.enabled {
		return nil, nil, nativePilotDiagnostic{Reason: "admission-disabled"}
	}
	if reader.role != "reader" || collector.role != "collector" || reader.bridge == nil || collector.bridge == nil || reader.chrome.PID == collector.chrome.PID || reader.profileID == "" || reader.profileID == collector.profileID {
		return nil, nil, nativePilotDiagnostic{Reason: "invalid-configuration"}
	}
	backend := &xManualPilotWindowsBackend{roles: map[string]xManualPilotWindowsRole{"reader": reader, "collector": collector}}
	return newXManualPilotOwnerBrokerPorts(parent, cfg, backend)
}
