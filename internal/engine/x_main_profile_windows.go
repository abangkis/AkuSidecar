//go:build windows && auth_session_dev

package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/abangkis/AkuSidecar/internal/appshell"
	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/ownedprocess"
	"github.com/abangkis/AkuSidecar/internal/ownedprofile"
	"github.com/abangkis/AkuSidecar/internal/ownedrpc"
)

func mainProfileAsset(path string) (xApplicationOwnerAsset, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return xApplicationOwnerAsset{}, errors.New("main-profile packaged asset unavailable")
	}
	sum := sha256.Sum256(raw)
	return xApplicationOwnerAsset{path: path, digest: hex.EncodeToString(sum[:])}, nil
}
func MainBrowserTransportOptions(cfg config.Config) (*appshell.MainBrowserTransportOptions, error) {
	if !cfg.Capture.XUsesMainProfile() {
		return nil, nil
	}
	if !cfg.Dev || !cfg.WindowsCaptureSplit {
		return nil, errors.New("main-profile transport requires development split")
	}
	image, err := os.Executable()
	if err != nil {
		return nil, err
	}
	bin := filepath.Dir(image)
	node, err := mainProfileAsset(filepath.Join(bin, "headless-worker", "node.exe"))
	if err != nil {
		return nil, err
	}
	var pin struct {
		NodeSHA256 string `json:"nodeSha256"`
	}
	raw, err := os.ReadFile(filepath.Join(bin, "headless-worker", "node.pin.json"))
	if err != nil || json.Unmarshal(raw, &pin) != nil || pin.NodeSHA256 != node.digest {
		return nil, errors.New("main-profile Node pin rejected")
	}
	var manifest struct {
		MainLifecycle bool `json:"mainLifecycle"`
		Driver        struct {
			File   string `json:"file"`
			SHA256 string `json:"sha256"`
		} `json:"driver"`
	}
	root := filepath.Join(bin, "main-browser-transport")
	raw, err = os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil || json.Unmarshal(raw, &manifest) != nil || !manifest.MainLifecycle || manifest.Driver.File != "main-browser-transport.cjs" {
		return nil, errors.New("main-profile transport package required")
	}
	driver, err := mainProfileAsset(filepath.Join(root, manifest.Driver.File))
	if err != nil || driver.digest != manifest.Driver.SHA256 {
		return nil, errors.New("main-profile transport pin rejected")
	}
	return &appshell.MainBrowserTransportOptions{Node: node.path, Driver: driver.path, NodeSHA256: node.digest, DriverSHA256: driver.digest, StartupTimeout: nativePilotStartup}, nil
}

func xMainDailyProfileScope(descriptor ownedprofile.Descriptor, subprofile string) (string, error) {
	if !appshell.ValidProfileDirectory(subprofile) {
		return "", errors.New("main-profile subprofile rejected")
	}
	path := filepath.Join(descriptor.Directory, subprofile)
	id, err := ownedprocess.DirectoryID(path)
	if err != nil {
		return "", err
	}
	// Keep the enclosing owned directories in the permission scope while binding
	// consent to the selected Chromium profile and its stable directory identity.
	parentScope := xDailyProfileScope(descriptor.Root, descriptor.Directory, descriptor.RootID, descriptor.DirectoryID)
	return xDailyProfileScope(descriptor.Directory, path, parentScope, id), nil
}

// Composition binds an existing owned browser, never an external browser/profile.
// It installs a factory only; the ordinary process-consent endpoint starts X.
func ConfigureXMainDailyRuntime(parent context.Context, e *Engine, window *appshell.Window, manager *captureruntime.Manager, profile, subprofile, chrome string) error {
	if !e.config.Capture.XUsesMainProfile() {
		return nil
	}
	protocol, ok := window.CaptureProtocol().(appshell.MainBrowserProtocol)
	if !ok || manager == nil || !manager.Persistent() || window.PID() != manager.PID() || !appshell.ValidProfileDirectory(subprofile) {
		return errors.New("main-profile owned browser required")
	}
	root := filepath.Dir(profile)
	rootID, err := ownedprocess.DirectoryID(root)
	if err != nil {
		return err
	}
	profileID, err := ownedprocess.DirectoryID(profile)
	if err != nil {
		return err
	}
	options, err := MainBrowserTransportOptions(e.config)
	if err != nil {
		return err
	}
	node := xApplicationOwnerAsset{path: options.Node, digest: options.NodeSHA256}
	driver := xApplicationOwnerAsset{path: options.Driver, digest: options.DriverSHA256}
	image, err := mainProfileAsset(chrome)
	if err != nil {
		return err
	}
	descriptor := ownedprofile.Descriptor{Root: root, Directory: profile, RootID: rootID, DirectoryID: profileID}
	profileScope, err := xMainDailyProfileScope(descriptor, subprofile)
	if err != nil {
		return err
	}
	generation := manager.Snapshot().Generation
	return e.configureXDailyFactory(parent, func(startup, process context.Context) (xDailyNativeOwner, error) {
		if !manager.Accepts(generation) {
			return nil, errors.New("main-profile generation changed")
		}
		collectorParent := filepath.Join(e.config.Root, "runtime", "x-daily-collectors")
		if err := os.MkdirAll(collectorParent, 0700); err != nil {
			return nil, err
		}
		collectorRoot, err := os.MkdirTemp(collectorParent, "main-process-")
		if err != nil {
			return nil, err
		}
		id, err := ownedprocess.DirectoryID(collectorRoot)
		if err != nil {
			return nil, err
		}
		lease := &xMainBorrowedLease{descriptor: descriptor, manager: manager, generation: generation}
		cfg := xApplicationOwnerConfig{enabled: true, node: node, chrome: image, driver: driver, reader: descriptor,
			collector: ownedprofile.Descriptor{Root: collectorRoot, Directory: filepath.Join(collectorRoot, "unopened"), RootID: id}, mainWindow: window, mainProfileDirectory: subprofile}
		h := &xApplicationOwner{cfg: cfg, worker: protocol.TransportWorker(), mainProtocol: protocol, readerLease: lease, closed: make(chan struct{}), stopping: make(chan struct{}), readerDone: make(chan struct{})}
		h.pipe = &xMainBorrowedRPC{protocol: protocol}
		n := &xDailyOwner{host: h, e: e, root: collectorRoot, origin: "http://" + e.config.Server.HostPort()}
		fail := func(err error) (xDailyNativeOwner, error) {
			cleanup, cancel := context.WithTimeout(context.Background(), nativePilotCleanup)
			defer cancel()
			if h.Close(cleanup) != nil {
				return n, errors.Join(err, errors.New("main-profile X cleanup uncertain"))
			}
			return nil, err
		}
		if _, err = protocol.CallX(startup, "daily_owner_bootstrap", map[string]any{"chromePath": chrome, "readerProfile": profile, "readerPID": window.PID(), "profileDirectory": subprofile, "acknowledgeLimitations": true, "allowProcessSessionDerivation": true}); err != nil {
			return fail(err)
		}
		var initialized struct {
			Owner     int    `json:"owner"`
			ReaderPID uint32 `json:"readerPID"`
		}
		if err = n.call(startup, "daily_reader_initialize", nil, &initialized); err != nil {
			return fail(err)
		}
		if initialized.Owner <= 0 || initialized.ReaderPID != uint32(window.PID()) {
			return fail(errors.New("main-profile X initialization rejected"))
		}
		n.reader, err = h.bindRole(startup, "reader", initialized.ReaderPID, initialized.Owner)
		if err != nil {
			return fail(err)
		}
		h.protocolOwner = initialized.Owner
		if err = n.CheckReader(startup); err != nil {
			return fail(err)
		}
		// Engine joins the batch before closing this borrower. A parent-cancel
		// callback must not race scoped close against a still-running seed.
		go func() {
			select {
			case <-process.Done():
				return
			case <-h.worker.Closed():
			}
			e.xDaily.mu.Lock()
			if e.xDaily.owner == n && !e.xDaily.closed {
				e.xDaily.authorized = false
				if e.xDaily.reason == "" {
					e.xDaily.reason = "main-transport-lost"
					e.xDaily.failurePhase = "reader-owner"
				}
				if e.xDaily.cancel != nil {
					e.xDaily.cancel()
				}
			}
			e.xDaily.mu.Unlock()
		}()
		return n, nil
	}, profileScope)
}

type xMainBorrowedLease struct {
	descriptor ownedprofile.Descriptor
	manager    *captureruntime.Manager
	generation uint64
}

func (l *xMainBorrowedLease) Validate() error {
	if !l.manager.Accepts(l.generation) {
		return errors.New("main-profile generation revoked")
	}
	id, err := ownedprocess.DirectoryID(l.descriptor.Directory)
	if err != nil || id != l.descriptor.DirectoryID {
		return errors.New("main-profile directory changed")
	}
	return nil
}
func (l *xMainBorrowedLease) Matches(root, profile string) bool {
	return root == l.descriptor.Root && profile == l.descriptor.Directory && l.Validate() == nil
}
func (*xMainBorrowedLease) Close() error { return nil } // Capability release, never profile ownership release.

type xMainBorrowedRPC struct {
	protocol appshell.MainBrowserProtocol
	mu       sync.Mutex
	closed   bool
}

func (r *xMainBorrowedRPC) Call(ctx context.Context, op string, fields map[string]any, progress func([]byte)) (ownedrpc.Reply, error) {
	r.mu.Lock()
	closed := r.closed
	r.mu.Unlock()
	if closed {
		return ownedrpc.Reply{}, ownedrpc.ErrClosed
	}
	if fields == nil {
		fields = map[string]any{}
	}
	var raw json.RawMessage
	var err error
	if observed, ok := r.protocol.(appshell.MainBrowserProgressProtocol); ok && progress != nil && xMainStartupProgressOperation(op) {
		var mu sync.Mutex
		count, terminal := 0, false
		forward := func(value []byte) {
			if len(value) > 512 || !xMainStartupProgressValid(op, value) {
				return
			}
			mu.Lock()
			defer mu.Unlock()
			if terminal || count >= 32 {
				return
			}
			count++
			progress(append([]byte(nil), value...))
		}
		raw, err = observed.CallXWithProgress(ctx, op, fields, forward)
		mu.Lock()
		terminal = true
		mu.Unlock()
	} else {
		raw, err = r.protocol.CallX(ctx, op, fields)
	}
	var failure *appshell.MainBrowserError
	if errors.As(err, &failure) {
		return ownedrpc.Reply{Code: closedXApplicationCode(failure.Code), Diagnostic: failure.Diagnostic}, nil
	}
	return ownedrpc.Reply{Result: raw}, err
}

func xMainStartupProgressOperation(operation string) bool {
	switch operation {
	case "manual_acquire", "manual_inspect", "manual_inventory":
		return true
	default:
		return false
	}
}

func xMainStartupProgressValid(operation string, raw []byte) bool {
	if !xMainStartupProgressOperation(operation) {
		return false
	}
	if len(raw) == 0 || len(raw) > 512 {
		return false
	}
	var value struct {
		Operation    string  `json:"operation"`
		ElapsedMS    int64   `json:"elapsedMS"`
		Cause        string  `json:"cause"`
		SettingsWait *string `json:"settingsWait"`
	}
	if !nativePilotProgressDecode(raw, &value) || value.ElapsedMS < 0 || value.ElapsedMS > nativePilotLife.Milliseconds() {
		return false
	}
	switch value.Operation {
	case "inspect_rpc", "inventory_rpc", "acquire_rpc", "identity", "rendezvous", "settings_wait", "cleanup", "root_restore", "complete":
	default:
		return false
	}
	if value.Cause != "" {
		if _, ok := nativePilotInitialCode(value.Cause); !ok {
			return false
		}
	}
	if value.SettingsWait != nil {
		found := false
		for _, state := range nativePilotSettingsWaitStates {
			if state == *value.SettingsWait {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
func (r *xMainBorrowedRPC) Close()                           { r.mu.Lock(); r.closed = true; r.mu.Unlock() }
func (*xMainBorrowedRPC) Deliver(int64, ownedrpc.Reply) bool { return false }
func (*xMainBorrowedRPC) Progress(int64, []byte)             {}

func (b *xApplicationOwner) closeBorrowedMain() {
	b.closeOnce.Do(func() {
		close(b.stopping)
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), nativePilotCleanup)
			defer cancel()
			raw, err := b.mainProtocol.CallX(ctx, "daily_owner_close", map[string]any{})
			var result struct {
				Closed bool `json:"closed"`
			}
			if err != nil || json.Unmarshal(raw, &result) != nil || !result.Closed {
				b.cleanupErr = errors.New("borrowed main X cleanup uncertain")
			}
			clear(raw)
			if b.cleanupErr == nil {
				b.cleanupErr = b.releasePins()
			}
			b.pipe.Close()
			close(b.readerDone)
			close(b.closed)
		}()
	})
}
