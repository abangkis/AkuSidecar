//go:build windows && auth_session_dev

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

	"github.com/abangkis/AkuSidecar/internal/appshell"

	"github.com/abangkis/AkuSidecar/internal/ownedprocess"

	"golang.org/x/sys/windows"
)

// A trusted application composition supplies an already-owned private transport.
// The scope port validates the held profile lease; it cannot be selected by JSON.
// This adapter launches no process, acquires no profile and transfers no session.
type xManualPilotWindowsTransport interface {
	isClosed() bool
	nativeManualNodePID() uint32
	nativeManualPaths() (string, string)
	nativeManualScope(string, string) bool
	nativeManualJob(func(windows.Handle) error) error
	rpcRoundTripObserved(context.Context, string, map[string]any, any, func([]byte)) error
	terminateOwnedTree()
}

// Each role has independent Chrome/profile pins; Node and Job may be shared.
type xManualPilotWindowsRole struct {
	role                             string
	bridge                           xManualPilotWindowsTransport
	protocolOwner                    int
	node, chrome, parent             ownedprocess.Identity
	chromeHandle                     windows.Handle
	root, rootID, profile, profileID string
	rootPin, profilePin              windows.Handle
	generation                       uint64
	mainWindow                       *appshell.Window
	mainProfileDirectory             string
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

func xManualPilotWindowsImage(identity ownedprocess.Identity, path string) error {
	actual, err := ownedprocess.ProcessIdentity(identity.PID)
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
	if err != nil || expectedErr != nil || !strings.EqualFold(filepath.Clean(resolved), filepath.Clean(expected)) {
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
	actual, err := ownedprocess.DirectoryID(path)
	if err != nil || pinned != id || actual != id {
		return errors.New("x-owner-directory-changed")
	}
	return nil
}
func xManualPilotWindowsScope(record xManualPilotWindowsRole) bool {
	if record.bridge == nil || !record.bridge.nativeManualScope(record.root, record.profile) {
		return false
	}
	rel, err := filepath.Rel(record.root, record.profile)
	return err == nil && rel != "." && !filepath.IsAbs(rel) && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
func (b *xManualPilotWindowsBackend) snapshot(ctx context.Context, role string) (nativePilotOwner, error) {
	if ctx != nil && ctx.Err() != nil {
		return nativePilotOwner{}, ctx.Err()
	}
	r, ok := b.roles[role]
	if !ok || ctx == nil || ctx.Err() != nil || r.bridge == nil || r.bridge.isClosed() || r.role != role || r.protocolOwner <= 0 || r.node.PID != r.bridge.nativeManualNodePID() || r.parent.PID != uint32(os.Getpid()) || r.chromeHandle == 0 || r.generation == 0 || !xManualPilotWindowsScope(r) {
		return nativePilotOwner{}, xManualPilotCoreFailure(nil, "windows_scope")
	}
	actualParent, err := ownedprocess.ProcessIdentity(r.parent.PID)
	if err != nil || actualParent != r.parent {
		return nativePilotOwner{}, xManualPilotCoreFailure(nil, "parent_changed")
	}
	identity, err := ownedprocess.IdentityFromHandle(r.chromeHandle)
	status, waitErr := windows.WaitForSingleObject(r.chromeHandle, 0)
	if err != nil || identity != r.chrome || waitErr != nil || status != uint32(windows.WAIT_TIMEOUT) {
		return nativePilotOwner{}, xManualPilotCoreFailure(nil, "chrome_changed")
	}
	nodePath, chromePath := r.bridge.nativeManualPaths()
	if xManualPilotWindowsImage(r.node, nodePath) != nil || xManualPilotWindowsImage(r.chrome, chromePath) != nil {
		return nativePilotOwner{}, xManualPilotCoreFailure(nil, "image_changed")
	}
	nodeParent, chromeParent, err := xManualPilotParentPair(r.node.PID, r.chrome.PID, xManualPilotWindowsProcessParents)
	if err != nil || nodeParent != r.parent.PID {
		return nativePilotOwner{}, xManualPilotCoreFailure(nil, "node_parent")
	}
	if r.mainWindow == nil && chromeParent != r.node.PID || r.mainWindow != nil && (r.role != "reader" || chromeParent != r.parent.PID || r.mainWindow.PID() != int(r.chrome.PID)) {
		return nativePilotOwner{}, xManualPilotCoreFailure(nil, "chrome_parent")
	}
	var job windows.Handle
	checked := false
	checkJob := func(held windows.Handle) error {
		checked = true
		job = held
		members := []uint32{r.node.PID, r.chrome.PID}
		if r.mainWindow != nil {
			members = []uint32{r.chrome.PID}
		}
		for _, pid := range members {
			h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
			if err != nil {
				return xManualPilotCoreFailure(nil, "job_unavailable")
			}
			member, err := ownedprocess.Membership(h, job)
			windows.CloseHandle(h)
			if err != nil || !member {
				return xManualPilotCoreFailure(nil, "job_changed")
			}
		}
		return nil
	}
	checkOwnerJob := r.bridge.nativeManualJob
	if r.mainWindow != nil {
		// Reader and transport are siblings under Sidecar, with independent
		// Jobs. Prove each actual boundary rather than forging a shared Job.
		if err = r.bridge.nativeManualJob(func(held windows.Handle) error {
			h, e := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, r.node.PID)
			if e != nil {
				return e
			}
			defer windows.CloseHandle(h)
			member, e := ownedprocess.Membership(h, held)
			if e != nil || !member {
				return errors.New("x-main-transport-job-changed")
			}
			return nil
		}); err != nil {
			return nativePilotOwner{}, err
		}
		checkOwnerJob = func(check func(windows.Handle) error) error {
			return r.mainWindow.WithMainBrowserJob(r.profile, r.mainProfileDirectory, check)
		}
	}
	if err := checkOwnerJob(checkJob); err != nil {
		return nativePilotOwner{}, err
	}
	if !checked || job == 0 {
		return nativePilotOwner{}, xManualPilotCoreFailure(nil, "job_unavailable")
	}
	if xManualPilotWindowsDirectory(r.rootPin, r.root, r.rootID) != nil || xManualPilotWindowsDirectory(r.profilePin, r.profile, r.profileID) != nil {
		return nativePilotOwner{}, xManualPilotCoreFailure(nil, "directory_changed")
	}
	if role == "collector" && len(ownedprocess.VisibleWindows(r.chrome.PID)) != 0 {
		return nativePilotOwner{}, xManualPilotCoreFailure(nil, "collector_not_headless")
	}
	return nativePilotOwner{role: role, owner: strconv.FormatUint(uint64(r.chrome.PID), 10), profile: r.profile, directory: r.profileID, image: chromePath, creation: r.chrome.Created, job: strconv.FormatUint(uint64(job), 10), parent: strconv.FormatUint(uint64(r.parent.PID), 10) + "/" + r.parent.Created, pid: int(r.chrome.PID), generation: r.generation}, nil
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
			return nativePilotCaptureTransportError(ctx, err)
		}
		if detail, ok := nativePilotInitialCode(err.Error()); ok {
			return &nativePilotInitialError{detail}
		}
		if ctx.Err() != nil {
			return nativePilotInitialWrap(ctx.Err(), "pair", "joint")
		}
		if op == "manual_prepare" {
			return nativePilotPrepareFailure(nil, role, "go_rpc", "command_failed")
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

// Default-off private construction. This code fixes native provenance; no
// config/JSON flag selects it. No application factory is registered here.
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
