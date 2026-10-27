//go:build windows && auth_session_dev

package engine

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/abangkis/AkuSidecar/internal/appshell"
	"github.com/abangkis/AkuSidecar/internal/ownedprocess"
	"github.com/abangkis/AkuSidecar/internal/ownedprofile"
	"github.com/abangkis/AkuSidecar/internal/ownedrpc"
	"golang.org/x/sys/windows"
)

const xApplicationOwnerSchema = "aku.x.application-owner.v1"

// Only trusted composition supplies pinned package assets and profile descriptors.
// No public decoder, environment gate or application selector invokes this factory.
type xApplicationOwnerAsset struct{ path, digest string }
type xApplicationOwnerConfig struct {
	enabled              bool
	node, chrome, driver xApplicationOwnerAsset
	reader, collector    ownedprofile.Descriptor
	retainedProject      string
	mainWindow           *appshell.Window
	mainProfileDirectory string
}

type xApplicationRPC interface {
	Call(context.Context, string, map[string]any, func([]byte)) (ownedrpc.Reply, error)
	Close()
	Deliver(int64, ownedrpc.Reply) bool
	Progress(int64, []byte)
}

func (xApplicationOwnerConfig) String() string     { return "private-x-application-owner-config" }
func (v xApplicationOwnerConfig) GoString() string { return v.String() }

type xApplicationProfileLease interface {
	Validate() error
	Matches(string, string) bool
	Close() error
}

type xApplicationOwner struct {
	cfg                         xApplicationOwnerConfig
	readerLease, collectorLease xApplicationProfileLease
	assets                      []*os.File
	worker                      *appshell.OwnedCommand
	stdin                       io.WriteCloser
	pipe                        xApplicationRPC
	mainProtocol                appshell.MainBrowserProtocol
	ready                       chan error
	readerDone                  chan struct{}
	closed                      chan struct{}
	stopping                    chan struct{}
	closeOnce                   sync.Once
	cleanupErr                  error
	parentStop                  func() bool
	nonce                       string
	roleMu                      sync.Mutex
	roles                       []xManualPilotWindowsRole
	initializationMu            sync.Mutex
	initializeUsed              bool
	seedReady                   bool // Set only after the one-use private seed receipt is validated.
	seedUsed                    bool
	readerOpenUsed              bool
	protocolOwner               int
	postMu                      sync.Mutex
	postBinding                 *appshell.OwnedReaderBinding
	postRequests                map[string]bool
}

func (*xApplicationOwner) String() string     { return "private-x-application-owner" }
func (v *xApplicationOwner) GoString() string { return v.String() }

func pinXApplicationAsset(asset xApplicationOwnerAsset) (*os.File, error) {
	if !filepath.IsAbs(asset.path) || filepath.Clean(asset.path) != asset.path || len(asset.digest) != 64 {
		return nil, errors.New("application owner asset invalid")
	}
	resolved, err := filepath.EvalSymlinks(asset.path)
	if err != nil || !strings.EqualFold(resolved, asset.path) {
		return nil, errors.New("application owner asset alias")
	}
	name, err := windows.UTF16PtrFromString(asset.path)
	if err != nil {
		return nil, errors.New("application owner asset invalid")
	}
	h, err := windows.CreateFile(name, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return nil, errors.New("application owner asset unavailable")
	}
	f := os.NewFile(uintptr(h), "private-owner-asset")
	var info windows.ByHandleFileInformation
	if windows.GetFileInformationByHandle(h, &info) != nil || info.FileAttributes&(windows.FILE_ATTRIBUTE_DIRECTORY|windows.FILE_ATTRIBUTE_REPARSE_POINT) != 0 {
		f.Close()
		return nil, errors.New("application owner asset not ordinary")
	}
	hash := sha256.New()
	if _, err = io.Copy(hash, f); err != nil || hex.EncodeToString(hash.Sum(nil)) != asset.digest {
		f.Close()
		return nil, errors.New("application owner asset mismatch")
	}
	return f, nil
}

func startXApplicationOwner(parent, request context.Context, cfg xApplicationOwnerConfig) (*xApplicationOwner, error) {
	if !cfg.enabled {
		return nil, errors.New("application owner disabled")
	}
	if parent == nil || request == nil || parent.Err() != nil || request.Err() != nil {
		return nil, errors.New("application owner context unavailable")
	}
	// Known local descriptors must reject overlap before creating any lease.
	// Retained descriptors are checked again after their fixed source resolves.
	if cfg.retainedProject == "" {
		if err := xApplicationSeparateProfiles(cfg.reader, cfg.collector); err != nil {
			return nil, err
		}
	}
	startup, cancelStartup := context.WithTimeout(request, nativePilotStartup)
	stopParentStartup := context.AfterFunc(parent, cancelStartup)
	defer cancelStartup()
	defer stopParentStartup()
	b := &xApplicationOwner{cfg: cfg, ready: make(chan error, 1), readerDone: make(chan struct{}), closed: make(chan struct{}), stopping: make(chan struct{})}
	// Asset validation precedes lease acquisition; default/invalid construction
	// cannot touch profile locks. Pins stay held until owned cleanup is proved.
	for _, asset := range []xApplicationOwnerAsset{cfg.node, cfg.chrome, cfg.driver} {
		f, err := pinXApplicationAsset(asset)
		if err != nil {
			b.releasePins()
			return nil, err
		}
		b.assets = append(b.assets, f)
	}
	if startup.Err() != nil {
		b.releasePins()
		return nil, errors.New("application owner startup canceled")
	}
	var err error
	if cfg.retainedProject != "" {
		var retained *xRetainedQAProfile
		retained, err = acquireXApplicationRetainedProfile(cfg.retainedProject)
		if err == nil {
			b.readerLease = retained
			cfg.reader = retained.descriptor()
			b.cfg.reader = cfg.reader
		}
	} else {
		b.readerLease, err = ownedprofile.Acquire(cfg.reader)
	}
	if err != nil {
		b.releasePins()
		return nil, errors.New("application owner reader lease unavailable")
	}
	if err = xApplicationSeparateProfiles(cfg.reader, cfg.collector); err != nil {
		b.releasePins()
		return nil, err
	}
	if startup.Err() != nil {
		b.releasePins()
		return nil, errors.New("application owner startup canceled")
	}
	b.collectorLease, err = ownedprofile.Acquire(cfg.collector)
	if err != nil {
		b.releasePins()
		return nil, errors.New("application owner collector lease unavailable")
	}
	if startup.Err() != nil {
		b.releasePins()
		return nil, errors.New("application owner startup canceled")
	}
	var nonce [24]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		b.releasePins()
		return nil, errors.New("application owner challenge unavailable")
	}
	b.nonce = hex.EncodeToString(nonce[:])
	cmd := exec.Command(cfg.node.path, cfg.driver.path)
	cmd.Env = xApplicationOwnerEnvironment(cfg.collector.Root)
	cmd.Stderr = io.Discard
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		b.releasePins()
		return nil, errors.New("application owner output unavailable")
	}
	b.stdin, err = cmd.StdinPipe()
	if err != nil {
		stdout.Close()
		b.releasePins()
		return nil, errors.New("application owner input unavailable")
	}
	// Reuse the application launcher: suspended root, lifetime Job and per-tree
	// assignment before execution. Never start Node and attach it after ready.
	b.worker, err = appshell.StartOwnedCommand(cmd)
	if err != nil {
		b.stdin.Close()
		stdout.Close()
		if cmd.Process != nil && (cmd.ProcessState == nil || !cmd.ProcessState.Exited()) {
			b.cleanupErr = errors.New("application owner launch cleanup uncertain")
			b.closeOnce.Do(func() { close(b.stopping); close(b.closed) })
			return b, b.cleanupErr
		}
		b.releasePins()
		return nil, errors.New("application owner launch failed")
	}
	b.pipe = ownedrpc.New(b.stdin)
	go b.read(stdout)
	b.parentStop = context.AfterFunc(parent, b.terminateOwnedTree)
	bootstrap := map[string]any{"op": "native_owner_bootstrap", "schema": xApplicationOwnerSchema, "nonce": b.nonce, "chromePath": cfg.chrome.path, "readerProfile": cfg.reader.Directory, "collectorProfile": cfg.collector.Directory}
	fail := func(reason string) (*xApplicationOwner, error) {
		b.terminateOwnedTree()
		ctx, cancel := context.WithTimeout(context.Background(), nativePilotCleanup)
		defer cancel()
		if b.Close(ctx) != nil {
			return b, errors.New(reason + "; cleanup=uncertain")
		}
		return nil, errors.New(reason)
	}
	if json.NewEncoder(b.stdin).Encode(bootstrap) != nil {
		return fail("application owner bootstrap write failed")
	}
	select {
	case err = <-b.ready:
		if err != nil {
			return fail("application owner handshake rejected")
		}
	case <-startup.Done():
		return fail("application owner startup canceled")
	}
	if startup.Err() != nil || b.readerLease.Validate() != nil || b.collectorLease.Validate() != nil || b.isClosed() {
		return fail("application owner scope changed")
	}
	return b, nil
}

func xApplicationSeparateProfiles(reader, collector ownedprofile.Descriptor) error {
	if reader.Root == collector.Root || reader.DirectoryID == collector.DirectoryID {
		return errors.New("application owner profiles overlap")
	}
	for _, pair := range [][2]string{{reader.Root, collector.Root}, {collector.Root, reader.Root}} {
		rel, err := filepath.Rel(pair[0], pair[1])
		if err != nil || rel == "." || !filepath.IsAbs(rel) && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return errors.New("application owner roots overlap")
		}
	}
	return nil
}

// The effective worker program must match the pinned driver. Inheriting Node
// preload/module/proxy/debug variables would let code run before that driver.
func xApplicationOwnerEnvironment(tempRoot string) []string {
	var env []string
	for _, name := range []string{"SystemRoot", "WINDIR", "USERPROFILE", "LOCALAPPDATA", "APPDATA", "HOMEDRIVE", "HOMEPATH"} {
		if value, ok := os.LookupEnv(name); ok {
			env = append(env, name+"="+value)
		}
	}
	return append(env, "TEMP="+tempRoot, "TMP="+tempRoot)
}

func (b *xApplicationOwner) read(input io.Reader) {
	defer close(b.readerDone)
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	ready := false
	for scanner.Scan() {
		var message struct {
			Ready      bool            `json:"ready"`
			Schema     string          `json:"schema"`
			Nonce      string          `json:"nonce"`
			ID         int64           `json:"id"`
			OK         bool            `json:"ok"`
			Code       string          `json:"code"`
			Diagnostic json.RawMessage `json:"diagnostic"`
			Event      string          `json:"event"`
			Result     json.RawMessage `json:"result"`
		}
		if json.Unmarshal(scanner.Bytes(), &message) != nil {
			break
		}
		if !ready {
			var hello struct {
				Ready  bool   `json:"ready"`
				Schema string `json:"schema"`
				Nonce  string `json:"nonce"`
			}
			if xManualPilotDecode(scanner.Bytes(), &hello) != nil || !hello.Ready || hello.Schema != xApplicationOwnerSchema || hello.Nonce != b.nonce {
				b.ready <- errors.New("application owner handshake invalid")
				break
			}
			ready = true
			b.ready <- nil
			continue
		}
		if message.Ready {
			break
		}
		if message.Event == "manual_startup_progress" {
			b.pipe.Progress(message.ID, message.Result)
			continue
		}
		if message.Event != "" {
			break
		}
		if message.ID <= 0 {
			break
		}
		if message.OK {
			b.pipe.Deliver(message.ID, ownedrpc.Reply{Result: message.Result})
		} else {
			b.pipe.Deliver(message.ID, ownedrpc.Reply{Code: closedXApplicationCode(message.Code), Diagnostic: message.Diagnostic})
		}
	}
	if !ready {
		select {
		case b.ready <- errors.New("application owner handshake unavailable"):
		default:
		}
	}
	b.pipe.Close()
	b.terminateOwnedTree()
}

// Codes are opaque closed-alphabet transport data; the domain retains its exact
// diagnostic catalogs. Raw child stderr or exceptions never cross this boundary.
func closedXApplicationCode(code string) string {
	if code == "" || len(code) > 1024 {
		return "application_owner_failed"
	}
	for _, c := range code {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_') {
			return "application_owner_failed"
		}
	}
	return code
}

func (b *xApplicationOwner) releasePins() error {
	failed := false
	b.roleMu.Lock()
	for _, role := range b.roles {
		for _, h := range []windows.Handle{role.chromeHandle, role.rootPin, role.profilePin} {
			if h != 0 {
				if windows.CloseHandle(h) != nil {
					failed = true
				}
			}
		}
	}
	b.roles = nil
	b.roleMu.Unlock()
	if b.collectorLease != nil {
		if b.collectorLease.Close() != nil {
			failed = true
		}
	}
	for _, f := range b.assets {
		if f.Close() != nil {
			failed = true
		}
	}
	b.assets = nil
	// A retained profile may become clean only after every other owned pin closes.
	if b.readerLease != nil && !failed {
		if b.readerLease.Close() != nil {
			failed = true
		}
	}

	if failed {
		return errors.New("application owner pin release uncertain")
	}
	return nil
}

func (b *xApplicationOwner) terminateOwnedTree() {
	if b == nil {
		return
	}
	if b.mainProtocol != nil {
		b.closeBorrowedMain()
		return
	}
	b.closeOnce.Do(func() {
		close(b.stopping)
		go func() {
			if b.pipe != nil {
				b.pipe.Close()
			}
			if b.stdin != nil {
				_ = b.stdin.Close()
			}
			ctx, cancel := context.WithTimeout(context.Background(), nativePilotCleanup)
			defer cancel()
			if b.worker != nil {
				select {
				case <-b.worker.Closed():
				default:
					_ = b.worker.WithOwnedJob(func(job windows.Handle) error { return windows.TerminateJobObject(job, 1) })
				}
				if b.worker.WaitForCleanup(ctx) != nil {
					b.cleanupErr = errors.New("application owner tree cleanup uncertain")
				}
				select {
				case <-b.readerDone:
				case <-ctx.Done():
					if b.cleanupErr == nil {
						b.cleanupErr = errors.New("application owner pipe cleanup uncertain")
					}
				}
			}
			if b.cleanupErr == nil {
				b.cleanupErr = b.releasePins()
			}
			close(b.closed)
		}()
	})
}

func (b *xApplicationOwner) Close(ctx context.Context) error {
	if b == nil {
		return nil
	}
	b.terminateOwnedTree()
	if ctx == nil {
		return errors.New("application owner cleanup context unavailable")
	}
	select {
	case <-b.closed:
		if b.parentStop != nil {
			b.parentStop()
		}
		return b.cleanupErr
	case <-ctx.Done():
		return errors.New("application owner cleanup deadline")
	}
}
func (b *xApplicationOwner) isClosed() bool {
	if b == nil {
		return true
	}
	select {
	case <-b.stopping:
		return true
	default:
	}
	select {
	case <-b.closed:
		return true
	default:
	}
	if b.worker == nil {
		return true
	}
	select {
	case <-b.worker.Closed():
		return true
	default:
		return false
	}
}
func (b *xApplicationOwner) nativeManualNodePID() uint32 { return uint32(b.worker.PID()) }
func (b *xApplicationOwner) nativeManualPaths() (string, string) {
	return b.cfg.node.path, b.cfg.chrome.path
}
func (b *xApplicationOwner) nativeManualScope(root, profile string) bool {
	return !b.isClosed() && (b.readerLease != nil && b.readerLease.Matches(root, profile) || b.collectorLease != nil && b.collectorLease.Matches(root, profile))
}
func (b *xApplicationOwner) nativeManualJob(check func(windows.Handle) error) error {
	return b.worker.WithOwnedJob(check)
}
func (b *xApplicationOwner) rpcRoundTripObserved(ctx context.Context, op string, fields map[string]any, out any, progress func([]byte)) error {
	switch op {
	case "manual_prepare", "manual_acquire", "manual_inspect", "manual_inventory", "manual_abort", "manual_capture", "manual_capture_cancel":
	default:
		return errors.New("application owner operation rejected")
	}
	if b.isClosed() {
		return errors.New("application owner closed")
	}
	r, err := b.pipe.Call(ctx, op, fields, progress)
	if err != nil {
		return err
	}
	if r.Code != "" {
		return &xApplicationRPCError{code: r.Code, diagnostic: r.Diagnostic}
	}
	if out != nil && len(r.Result) > 0 && json.Unmarshal(r.Result, out) != nil {
		return errors.New("application owner response invalid")
	}
	return nil
}

type xApplicationRPCError struct {
	code       string
	diagnostic json.RawMessage
}

func (e *xApplicationRPCError) Error() string            { return e.code }
func (e *xApplicationRPCError) GoString() string         { return e.Error() }
func (e *xApplicationRPCError) diagnosticJSON() []byte   { return e.diagnostic }
func (e *xApplicationRPCError) dailyFailureCode() string { return closedXApplicationCode(e.code) }

// initializeNative starts only the fixed packaged recipe. Announced PIDs are
// candidates until both bindRole calls verify the live OS-owned processes.
// A failed or malformed attempt retires the owned tree; it cannot be retried.
func (b *xApplicationOwner) initializeNative(ctx context.Context) (xManualPilotWindowsRole, xManualPilotWindowsRole, error) {
	var empty xManualPilotWindowsRole
	if b == nil || ctx == nil || ctx.Err() != nil || b.isClosed() {
		return empty, empty, errors.New("application owner initialization unavailable")
	}
	b.initializationMu.Lock()
	if b.initializeUsed {
		b.initializationMu.Unlock()
		return empty, empty, errors.New("application owner initialization replay")
	}
	b.initializeUsed = true
	b.initializationMu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, time.Duration(nativePilotTimingPolicy["collectorHomeReadyMS"])*time.Millisecond)
	defer cancel()
	fail := func() (xManualPilotWindowsRole, xManualPilotWindowsRole, error) {
		b.terminateOwnedTree()
		return empty, empty, errors.New("application owner initialization rejected")
	}
	reply, err := b.pipe.Call(ctx, "native_owner_initialize", nil, nil)
	defer clear(reply.Result)
	if err != nil || reply.Code != "" {
		return fail()
	}
	var announced struct {
		Owner        int    `json:"owner"`
		ReaderPID    uint32 `json:"readerPID"`
		CollectorPID uint32 `json:"collectorPID"`
	}
	if xManualPilotDecode(reply.Result, &announced) != nil || announced.Owner <= 0 || announced.ReaderPID == 0 || announced.CollectorPID == 0 || announced.ReaderPID == announced.CollectorPID {
		return fail()
	}
	reader, err := b.bindRole(ctx, "reader", announced.ReaderPID, announced.Owner)
	if err != nil {
		return fail()
	}
	collector, err := b.bindRole(ctx, "collector", announced.CollectorPID, announced.Owner)
	if err != nil {
		return fail()
	}
	b.initializationMu.Lock()
	b.protocolOwner = announced.Owner
	b.initializationMu.Unlock()
	return reader, collector, nil
}

// Reader opening requires validated private seed/startup state. The generic
// RPC operation allowlist cannot bypass this one-use entry.
func (b *xApplicationOwner) openNativeReader(ctx context.Context) error {
	if b == nil || ctx == nil || ctx.Err() != nil || b.isClosed() {
		return errors.New("application owner reader unavailable")
	}
	b.initializationMu.Lock()
	if !b.seedReady || b.protocolOwner <= 0 || b.readerOpenUsed {
		b.initializationMu.Unlock()
		return errors.New("application owner reader unavailable")
	}
	b.readerOpenUsed = true
	owner := b.protocolOwner
	b.initializationMu.Unlock()
	reply, err := b.pipe.Call(ctx, "manual_open_reader", map[string]any{"owner": owner}, nil)
	defer clear(reply.Result)
	var opened struct {
		Opened         bool `json:"opened"`
		WindowCount    int  `json:"windowCount"`
		BootstrapCount int  `json:"bootstrapCount"`
	}
	if err != nil || reply.Code != "" || xManualPilotDecode(reply.Result, &opened) != nil || !opened.Opened || opened.WindowCount < 2 || opened.WindowCount > 16 || opened.BootstrapCount != 1 {
		b.terminateOwnedTree()
		return errors.New("application owner reader rejected")
	}
	return nil
}

// A PID announcement is a candidate only. Verify its exact process creation,
// image, parent, Job and pinned profile before exposing a role to the broker.
func (b *xApplicationOwner) bindRole(ctx context.Context, role string, pid uint32, protocolOwner int) (xManualPilotWindowsRole, error) {
	r := xManualPilotWindowsRole{}
	if b.isClosed() || ctx == nil || ctx.Err() != nil || protocolOwner <= 0 || pid == 0 || pid == b.nativeManualNodePID() || pid == uint32(os.Getpid()) {
		return r, errors.New("application owner role rejected")
	}
	var d ownedprofile.Descriptor
	switch role {
	case "reader":
		d = b.cfg.reader
	case "collector":
		d = b.cfg.collector
	default:
		return r, errors.New("application owner role rejected")
	}
	b.roleMu.Lock()
	defer b.roleMu.Unlock()
	if b.isClosed() {
		return r, errors.New("application owner closed")
	}
	for _, bound := range b.roles {
		if bound.role == role || bound.chrome.PID == pid {
			return r, errors.New("application owner role already bound")
		}
	}
	r = xManualPilotWindowsRole{role: role, bridge: b, protocolOwner: protocolOwner, root: d.Root, profile: d.Directory, rootID: d.RootID, profileID: d.DirectoryID, generation: 1}
	if role == "reader" && b.cfg.mainWindow != nil {
		r.mainWindow, r.mainProfileDirectory = b.cfg.mainWindow, b.cfg.mainProfileDirectory
	}
	valid := false
	defer func() {
		if !valid {
			for _, h := range []windows.Handle{r.chromeHandle, r.rootPin, r.profilePin} {
				if h != 0 {
					windows.CloseHandle(h)
				}
			}
		}
	}()
	var err error
	r.parent, err = ownedprocess.ProcessIdentity(uint32(os.Getpid()))
	if err != nil {
		return xManualPilotWindowsRole{}, errors.New("application owner parent unavailable")
	}
	r.node, err = ownedprocess.ProcessIdentity(b.nativeManualNodePID())
	if err != nil {
		return xManualPilotWindowsRole{}, errors.New("application owner worker unavailable")
	}
	r.chromeHandle, err = windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, pid)
	if err == nil {
		r.chrome, err = ownedprocess.IdentityFromHandle(r.chromeHandle)
	}
	if err != nil {
		return xManualPilotWindowsRole{}, errors.New("application owner process unavailable")
	}
	for _, pin := range []struct {
		path   string
		handle *windows.Handle
	}{{d.Root, &r.rootPin}, {d.Directory, &r.profilePin}} {
		name, err := windows.UTF16PtrFromString(pin.path)
		if err != nil {
			return xManualPilotWindowsRole{}, errors.New("application owner directory unavailable")
		}
		*pin.handle, err = windows.CreateFile(name, windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
		if err != nil {
			return xManualPilotWindowsRole{}, errors.New("application owner directory unavailable")
		}
	}
	backend := &xManualPilotWindowsBackend{roles: map[string]xManualPilotWindowsRole{role: r}}
	if _, err = backend.snapshot(ctx, role); err != nil {
		return xManualPilotWindowsRole{}, err
	}
	b.roles = append(b.roles, r)
	valid = true
	return r, nil
}
