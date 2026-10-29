//go:build windows && auth_session_dev

package engine

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/abangkis/AkuSidecar/internal/appshell"
	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/ownedprocess"
	"github.com/abangkis/AkuSidecar/internal/ownedprofile"
	"github.com/abangkis/AkuSidecar/internal/ownedrpc"
	"github.com/abangkis/AkuSidecar/internal/readerbroker"
	"golang.org/x/sys/windows"
)

const xDailyOwnerSchema = "aku.x.daily-owner.v1"

var errXDailyOwnerReply = errors.New("daily owner response invalid")

// The process owner exclusively leases the reader. Each batch leases a fresh
// collector directory and owns separate pins; normal batch disposal cannot
// terminate the enclosing Job or release the reader lease.
type xDailyOwner struct {
	mu         sync.Mutex
	host       *xApplicationOwner
	e          *Engine
	root       string
	reader     xManualPilotWindowsRole
	generation uint64
	batch      *xDailyBatch
	failed     bool
	account    string
	origin     string
}
type xDailyBatch struct {
	owner        *xDailyOwner
	host         *xApplicationOwner
	generation   uint64
	run          domain.Run
	started      time.Time
	runtime      *nativePilotRuntime
	broker       *nativePilotBroker
	stopManagers func()
	once         sync.Once
	closeErr     error
	pinsOnce     sync.Once
	pinsErr      error
}

func newXDailyNativeOwner(parent, startup context.Context, cfg xApplicationOwnerConfig, e *Engine, _ xApplicationPostPreparer) (xDailyNativeOwner, error) {
	if parent == nil || startup == nil || parent.Err() != nil || startup.Err() != nil || !cfg.enabled || cfg.retainedProject != "" || e == nil || !e.config.Dev || !e.XDailyConfigured() {
		return nil, errors.New("daily native owner selection rejected")
	}
	if err := xApplicationSeparateProfiles(cfg.reader, cfg.collector); err != nil {
		return nil, err
	}
	// The collector descriptor identifies only the owned parent namespace here.
	// No existing collector is adopted, and no retained QA profile is inspected.
	if id, err := ownedprocess.DirectoryID(cfg.collector.Root); err != nil || id != cfg.collector.RootID {
		return nil, errors.New("daily collector root changed")
	}
	h := &xApplicationOwner{cfg: cfg, ready: make(chan error, 1), readerDone: make(chan struct{}), closed: make(chan struct{}), stopping: make(chan struct{})}
	n := &xDailyOwner{host: h, e: e, root: cfg.collector.Root, origin: "http://" + e.config.Server.HostPort()}
	fail := func(err error) (xDailyNativeOwner, error) {
		ctx, cancel := context.WithTimeout(context.Background(), nativePilotCleanup)
		defer cancel()
		if h.Close(ctx) != nil {
			return n, errors.New("daily owner cleanup uncertain")
		}
		return nil, err
	}
	for _, asset := range []xApplicationOwnerAsset{cfg.node, cfg.chrome, cfg.driver} {
		f, err := pinXApplicationAsset(asset)
		if err != nil {
			h.releasePins()
			return nil, err
		}
		h.assets = append(h.assets, f)
	}
	rootName, err := windows.UTF16PtrFromString(cfg.collector.Root)
	if err != nil {
		h.releasePins()
		return nil, errors.New("daily collector root invalid")
	}
	rootPin, err := windows.CreateFile(rootName, windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		h.releasePins()
		return nil, errors.New("daily collector root unavailable")
	}
	h.assets = append(h.assets, os.NewFile(uintptr(rootPin), "daily-collector-root"))
	var rootInfo windows.ByHandleFileInformation
	rootIdentity, identityErr := ownedprocess.DirectoryID(cfg.collector.Root)
	if windows.GetFileInformationByHandle(rootPin, &rootInfo) != nil || rootInfo.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || identityErr != nil || rootIdentity != cfg.collector.RootID {
		h.releasePins()
		return nil, errors.New("daily collector root changed")
	}
	h.readerLease, err = ownedprofile.Acquire(cfg.reader)
	if err != nil {
		h.releasePins()
		return nil, errors.New("daily reader profile already owned or unavailable")
	}
	var nonce [24]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		h.releasePins()
		return nil, err
	}
	h.nonce = hex.EncodeToString(nonce[:])
	cmd := exec.Command(cfg.node.path, cfg.driver.path)
	cmd.Env = xApplicationOwnerEnvironment(cfg.collector.Root)
	cmd.Stderr = io.Discard
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		h.releasePins()
		return nil, err
	}
	h.stdin, err = cmd.StdinPipe()
	if err != nil {
		stdout.Close()
		h.releasePins()
		return nil, err
	}
	h.worker, err = appshell.StartOwnedCommand(cmd)
	if err != nil {
		stdout.Close()
		h.stdin.Close()
		if cmd.Process != nil && (cmd.ProcessState == nil || !cmd.ProcessState.Exited()) {
			h.cleanupErr = errors.New("daily owner launch cleanup uncertain")
			h.closeOnce.Do(func() { close(h.stopping); close(h.closed) })
			return n, h.cleanupErr
		}
		h.releasePins()
		return nil, errors.New("daily owner launch failed")
	}
	h.pipe = ownedrpc.New(h.stdin)
	go n.read(stdout)
	h.parentStop = context.AfterFunc(parent, h.terminateOwnedTree)
	if json.NewEncoder(h.stdin).Encode(map[string]any{"op": "daily_owner_bootstrap", "schema": xDailyOwnerSchema, "nonce": h.nonce, "chromePath": cfg.chrome.path, "readerProfile": cfg.reader.Directory}) != nil {
		return fail(errors.New("daily owner handshake failed"))
	}
	select {
	case err = <-h.ready:
		if err != nil {
			return fail(err)
		}
	case <-startup.Done():
		return fail(errors.New("daily owner startup canceled"))
	}
	var initialized struct {
		Owner     int    `json:"owner"`
		ReaderPID uint32 `json:"readerPID"`
	}
	if err = n.call(startup, "daily_reader_initialize", nil, &initialized); err != nil || initialized.Owner <= 0 || initialized.ReaderPID == 0 {
		return fail(errors.New("daily reader initialization failed"))
	}
	n.reader, err = h.bindRole(startup, "reader", initialized.ReaderPID, initialized.Owner)
	if err != nil {
		return fail(err)
	}
	h.protocolOwner = initialized.Owner
	if err = n.CheckReader(startup); err != nil {
		return fail(err)
	}
	return n, nil
}

func (n *xDailyOwner) read(input io.Reader) {
	h := n.host
	defer close(h.readerDone)
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	ready := false
	for scanner.Scan() {
		if !ready {
			var hello struct {
				Ready  bool   `json:"ready"`
				Schema string `json:"schema"`
				Nonce  string `json:"nonce"`
			}
			if xManualPilotDecode(scanner.Bytes(), &hello) != nil || !hello.Ready || hello.Schema != xDailyOwnerSchema || hello.Nonce != h.nonce {
				break
			}
			ready = true
			h.ready <- nil
			continue
		}
		var q struct {
			ID         int64           `json:"id"`
			OK         bool            `json:"ok"`
			Code       string          `json:"code"`
			Event      string          `json:"event"`
			Result     json.RawMessage `json:"result"`
			Diagnostic json.RawMessage `json:"diagnostic"`
		}
		if json.Unmarshal(scanner.Bytes(), &q) != nil || q.ID <= 0 {
			break
		}
		if q.Event == "manual_startup_progress" {
			h.pipe.Progress(q.ID, q.Result)
			continue
		}
		if q.Event != "" {
			break
		}
		if q.OK {
			h.pipe.Deliver(q.ID, ownedrpc.Reply{Result: q.Result})
		} else {
			h.pipe.Deliver(q.ID, ownedrpc.Reply{Code: closedXApplicationCode(q.Code), Diagnostic: q.Diagnostic})
		}
	}
	if !ready {
		select {
		case h.ready <- errors.New("daily owner handshake rejected"):
		default:
		}
	}
	h.pipe.Close()
	h.terminateOwnedTree()
}
func (n *xDailyOwner) call(ctx context.Context, op string, fields map[string]any, out any) error {
	if ctx == nil || ctx.Err() != nil || n.host.isClosed() {
		return errors.New("daily owner unavailable")
	}
	r, err := n.host.pipe.Call(ctx, op, fields, nil)
	defer clear(r.Result)
	if err != nil {
		return err
	}
	if r.Code != "" {
		return &xApplicationRPCError{code: r.Code, diagnostic: r.Diagnostic}
	}
	if out != nil && xManualPilotDecode(r.Result, out) != nil {
		return errXDailyOwnerReply
	}
	return nil
}
func (n *xDailyOwner) CheckReader(ctx context.Context) error {
	if n.host.isClosed() || n.host.readerLease.Validate() != nil {
		return errors.New("daily reader ownership lost")
	}
	backend := &xManualPilotWindowsBackend{roles: map[string]xManualPilotWindowsRole{"reader": n.reader}}
	if _, err := backend.snapshot(ctx, "reader"); err != nil {
		return err
	}
	var r struct {
		ReaderPID     uint32 `json:"readerPID"`
		ReaderProfile string `json:"readerProfile"`
	}
	if n.call(ctx, "daily_reader_check", nil, &r) != nil || r.ReaderPID != n.reader.chrome.PID || r.ReaderProfile != n.reader.profile {
		return errors.New("daily reader changed")
	}
	return nil
}
func (n *xDailyOwner) Close(ctx context.Context) error {
	n.mu.Lock()
	batch := n.batch
	n.failed = true
	n.mu.Unlock()
	var batchErr, pinErr error
	if batch != nil {
		batchErr = batch.Close(ctx)
	}
	hostErr := n.host.Close(ctx)
	// EOF/parent shutdown also proves the collector exited. Release retained
	// batch pins only after the complete process Job has drained successfully.
	// This does not prove that a target in borrowed Chrome was retired.
	if batch != nil && hostErr == nil {
		pinErr = batch.releasePins()
	}
	if joined := errors.Join(batchErr, hostErr, pinErr); joined != nil {
		var known *xDailyCleanupError
		if errors.As(joined, &known) {
			return &xDailyCleanupError{detail: known.detail.closed(), cause: joined}
		}
		return xDailyCleanupFail(joined, "owner_close", "unverified")
	}
	return nil
}

func (n *xDailyOwner) BeginBatch(ctx, parent context.Context, generation uint64, run domain.Run) (xDailyNativeBatch, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	admission := []struct {
		failed    bool
		condition string
	}{
		{n.failed, "owner_failed"}, {n.batch != nil, "batch_active"}, {generation != n.generation+1, "generation"},
		{run.Source != domain.SourceX, "source"}, {run.ID == "", "run_id"}, {run.SessionID == "", "session_id"},
		{ctx == nil, "context"}, {parent == nil, "parent"},
	}
	for _, check := range admission {
		if check.failed {
			return nil, xDailyStartupFail(nil, "admission", check.condition)
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, xDailyStartupFail(err, "admission", "context")
	}
	if err := parent.Err(); err != nil {
		return nil, xDailyStartupFail(err, "admission", "parent")
	}
	if err := n.CheckReader(ctx); err != nil {
		n.failed = true
		return nil, xDailyNativeStartupFail(err, "reader_check", "ownership", nil, nativePilotDiagnostic{})
	}
	n.generation = generation
	var nonce [24]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, xDailyStartupFail(err, "nonce", "random")
	}
	root := filepath.Join(n.root, "batch-"+hex.EncodeToString(nonce[:]))
	profile := filepath.Join(root, "profile")
	if err := os.Mkdir(root, 0700); err != nil {
		n.failed = true
		return nil, xDailyStartupFail(err, "collector_root", "create")
	}
	if err := os.Mkdir(profile, 0700); err != nil {
		n.failed = true
		return nil, xDailyStartupFail(err, "collector_profile", "create")
	}
	rootID, err := ownedprocess.DirectoryID(root)
	if err != nil {
		return nil, xDailyStartupFail(err, "root_identity", "identity")
	}
	profileID, err := ownedprocess.DirectoryID(profile)
	if err != nil {
		return nil, xDailyStartupFail(err, "profile_identity", "identity")
	}
	desc := ownedprofile.Descriptor{Root: root, Directory: profile, RootID: rootID, DirectoryID: profileID}
	lease, err := ownedprofile.Acquire(desc)
	if err != nil {
		n.failed = true
		return nil, xDailyStartupFail(err, "collector_lease", "lease")
	}
	// Per-batch hosts borrow transport and reader lease; their Close is never
	// invoked. Only their independent role pins and collector lease are released.
	h := &xApplicationOwner{cfg: n.host.cfg, worker: n.host.worker, pipe: n.host.pipe, readerLease: n.host.readerLease, collectorLease: lease, closed: n.host.closed, stopping: n.host.stopping}
	h.cfg.collector = desc
	b := &xDailyBatch{owner: n, host: h, generation: generation, run: run}
	n.batch = b
	fail := func(cause error, stage, condition string, diagnostic nativePilotDiagnostic) (xDailyNativeBatch, error) {
		n.failed = true
		if condition == "rpc" && errors.Is(cause, errXDailyOwnerReply) {
			condition = "decode"
		}
		// Freeze the first safe domain projection before cancellation/cleanup.
		failure := xDailyNativeStartupFail(cause, stage, condition, b.broker, diagnostic)
		// A timed-out private RPC may still be acquiring owners in Node. Do not
		// leave that late startup alive or advertise reader preservation.
		if ctx.Err() != nil || !b.started.IsZero() && !time.Now().Before(b.Deadline()) {
			n.host.terminateOwnedTree()
		}
		return b, failure
	}
	var announced struct {
		Owner        int    `json:"owner"`
		ReaderPID    uint32 `json:"readerPID"`
		CollectorPID uint32 `json:"collectorPID"`
	}
	if err := n.call(ctx, "daily_batch_begin", map[string]any{"generation": generation, "collectorProfile": profile}, &announced); err != nil {
		return fail(err, "batch_begin_rpc", "rpc", nativePilotDiagnostic{})
	}
	for _, check := range []struct {
		failed    bool
		condition string
	}{
		{announced.Owner <= 0, "owner"}, {announced.ReaderPID != n.reader.chrome.PID, "reader_pid"}, {announced.CollectorPID == 0, "collector_pid"},
	} {
		if check.failed {
			return fail(nil, "batch_begin_reply", check.condition, nativePilotDiagnostic{})
		}
	}
	reader, err := h.bindRole(ctx, "reader", announced.ReaderPID, announced.Owner)
	if err != nil {
		return fail(err, "reader_bind", "ownership", nativePilotDiagnostic{})
	}
	collector, err := h.bindRole(ctx, "collector", announced.CollectorPID, announced.Owner)
	if err != nil {
		return fail(err, "collector_bind", "ownership", nativePilotDiagnostic{})
	}
	reader.bridge = b
	collector.bridge = b
	b.started = time.Now()
	seedCtx, cancel := context.WithDeadline(ctx, b.Deadline())
	defer cancel()
	var receipt xApplicationSeedReceipt
	if err := n.call(seedCtx, "daily_batch_seed", map[string]any{"generation": generation, "owner": announced.Owner}, &receipt); err != nil {
		return fail(err, "seed_rpc", "rpc", nativePilotDiagnostic{})
	}
	for _, check := range []struct {
		failed    bool
		condition string
	}{
		{!nativePilotAccount(receipt.AccountID), "account"}, {receipt.Seeded == nil || !*receipt.Seeded, "seeded"},
		{receipt.HomeReady == nil || !*receipt.HomeReady, "home_ready"}, {receipt.ReaderPID != reader.chrome.PID, "reader_pid"},
		{receipt.CollectorPID != collector.chrome.PID, "collector_pid"}, {receipt.ReaderProfile != reader.profile, "reader_profile"},
		{receipt.CollectorProfile != collector.profile, "collector_profile"}, {n.account != "" && n.account != receipt.AccountID, "account_changed"},
	} {
		if check.failed {
			return fail(nil, "seed_receipt", check.condition, nativePilotDiagnostic{})
		}
	}
	n.account = receipt.AccountID
	key := hex.EncodeToString(nonce[:])
	parentRef := strconv.FormatUint(uint64(reader.parent.PID), 10) + "/" + reader.parent.Created
	binding := nativePilotBinding{account: receipt.AccountID, enrollment: key + "e", pair: key + "p", attempt: key + "a", derivation: key + "d", policy: 1, consent: 1,
		reader: nativePilotOwner{role: "reader", profile: reader.profile, parent: parentRef}, collector: nativePilotOwner{role: "collector", profile: collector.profile, parent: parentRef}}
	policy := func(call context.Context) (nativePilotBrokerState, error) {
		return nativePilotBrokerState{account: binding.account, enrollment: binding.enrollment, pair: binding.pair, attempt: binding.attempt, derivation: binding.derivation, policy: 1, consent: 1, completeInventory: true, parentAlive: parent.Err() == nil, channelHealthy: !n.host.isClosed(), invalidationsReady: true, optedIn: parent.Err() == nil}, call.Err()
	}
	broker, ports, diagnostic := newXManualPilotOwnerCaptureBroker(parent, xManualPilotOwnerConfig{enabled: true, pilot: nativePilotConfig{enabled: true, contract: nativePilotContract, limitationAcknowledged: true, binding: binding, attemptStarted: b.started}, policy: policy}, reader, collector)
	if broker == nil {
		return fail(nil, "broker_create", "configuration", diagnostic)
	}
	b.broker = broker
	if diagnostic = broker.start(seedCtx); !diagnostic.ManualEligible {
		return fail(nil, "broker_start", "eligibility", diagnostic)
	}
	rm, cm, stop, err := b.managers(parent, reader, collector)
	if err != nil {
		return fail(err, "managers", "manager", nativePilotDiagnostic{})
	}
	b.stopManagers = stop
	rm.SetExitObserver(func(uint64) { broker.invalidate("reader-lost") })
	cm.SetExitObserver(func(uint64) { broker.invalidate("owner-or-policy-changed") })
	b.runtime, err = newXNativePilotRuntime(seedCtx, broker, ports, rm, cm, func(call context.Context, _ readerbroker.Request) error { return n.CheckReader(call) })
	if err != nil {
		return fail(err, "runtime", "runtime", nativePilotDiagnostic{})
	}
	if err = n.e.bindNativePilotStoreAdmission(seedCtx, b.runtime, run.SessionID, run.ID, nativePilotDailyStoreAdmission{broker: broker, store: n.e.store}); err != nil {
		return fail(err, "store_admission", "authorization", nativePilotDiagnostic{})
	}
	return b, nil
}
func (b *xDailyBatch) StartedAt() time.Time { return b.started }
func (b *xDailyBatch) Deadline() time.Time  { return b.started.Add(nativePilotLife) }
func (b *xDailyBatch) Capture(ctx context.Context, command string) (domain.Run, error) {
	if b.runtime == nil {
		return domain.Run{}, errors.New("daily batch not ready")
	}
	result := b.owner.e.captureNativePilot(ctx, b.runtime, command)
	if result.PersistenceError != nil {
		return domain.Run{}, result.PersistenceError
	}
	if result.DeliveryError != nil {
		return result.Run, result.DeliveryError
	}
	return b.owner.e.store.GetRun(ctx, b.run.ID)
}
func (b *xDailyBatch) Close(ctx context.Context) error {
	b.once.Do(func() {
		detail := XDailyCleanupDiagnostic{Broker: "not_needed", BatchEnd: "not_attempted", CollectorExit: "not_attempted", PinRelease: "not_attempted", StoreReconcile: "not_needed"}
		var first error
		fail := func(stage, condition string, err error) {
			if first == nil {
				first = err
				detail.Stage, detail.Condition = stage, condition
			}
		}
		if b.broker != nil {
			b.broker.stop("batch-complete")
			b.broker.mu.Lock()
			uncertain := b.broker.cleanupFailed
			b.broker.mu.Unlock()
			if uncertain {
				detail.Broker = "uncertain"
				fail("broker_stop", "unverified", errors.New("daily broker cleanup uncertain"))
			} else {
				detail.Broker = "verified"
			}
		}
		if b.stopManagers != nil {
			b.stopManagers()
		}
		var receipt struct {
			Closed              bool                       `json:"closed"`
			Cleanup             *XDailyCleanupResources    `json:"cleanup"`
			BootstrapDiagnostic *XDailyBootstrapDiagnostic `json:"bootstrapDiagnostic"`
		}
		if err := b.owner.call(ctx, "daily_batch_end", map[string]any{"generation": b.generation}, &receipt); err != nil {
			detail.BatchEnd = "uncertain"
			fail("batch_end", "rpc", err)
		} else {
			detail.Resources = receipt.Cleanup
			detail.Bootstrap = receipt.BootstrapDiagnostic
			bootstrapInvalid := receipt.BootstrapDiagnostic != nil && (!receipt.BootstrapDiagnostic.valid() || receipt.Cleanup == nil || receipt.Cleanup.Bootstrap != "uncertain")
			if receipt.Cleanup == nil || !receipt.Cleanup.valid() || receipt.Closed != receipt.Cleanup.complete() || bootstrapInvalid {
				detail.BatchEnd = "uncertain"
				fail("batch_end", "receipt", errors.New("daily cleanup receipt invalid"))
			} else if !receipt.Closed {
				detail.BatchEnd = "uncertain"
				fail("batch_end", "unverified", errors.New("daily batch cleanup uncertain"))
			} else {
				detail.BatchEnd = "verified"
			}
		}
		// A failed control/target retirement cannot skip independent confirmation
		// of collector exit. Pins/profile may be released only after that proof.
		exited, seenCollector := true, false
		for _, r := range b.host.roles {
			if r.role == "collector" {
				seenCollector = true
				state, err := windows.WaitForSingleObject(r.chromeHandle, 0)
				if err != nil || state != uint32(windows.WAIT_OBJECT_0) {
					exited = false
					fail("collector_exit", "alive", errors.New("daily collector still alive"))
				}
			}
		}
		if !seenCollector && detail.BatchEnd != "verified" {
			exited = false
			fail("collector_exit", "unavailable", errors.New("daily collector exit not observed"))
		}
		if exited {
			detail.CollectorExit = "verified"
			if err := b.releasePins(); err != nil {
				detail.PinRelease = "uncertain"
				fail("pin_release", "release", err)
			} else {
				detail.PinRelease = "verified"
			}
		} else {
			detail.CollectorExit = "uncertain"
		}
		if first == nil {
			b.host.roles = nil
			b.owner.mu.Lock()
			if b.owner.batch == b {
				b.owner.batch = nil
			}
			b.owner.mu.Unlock()
		} else {
			b.closeErr = &xDailyCleanupError{detail: detail.closed(), cause: first}
		}
	})
	return b.closeErr
}
func (b *xDailyBatch) releasePins() error {
	b.pinsOnce.Do(func() {
		for _, r := range b.host.roles {
			for _, pin := range []windows.Handle{r.chromeHandle, r.rootPin, r.profilePin} {
				if pin != 0 && windows.CloseHandle(pin) != nil {
					b.pinsErr = errors.New("daily pin release uncertain")
				}
			}
		}
		if b.pinsErr == nil {
			b.pinsErr = b.host.collectorLease.Close()
		}
		if b.pinsErr == nil {
			root := b.host.cfg.collector.Root
			id, err := ownedprocess.DirectoryID(root)
			if err != nil || id != b.host.cfg.collector.RootID || filepath.Dir(root) != b.owner.root {
				b.pinsErr = errors.New("daily collector cleanup identity changed")
				return
			}
			if err = filepath.WalkDir(root, func(_ string, entry os.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if entry.Type()&os.ModeSymlink != 0 {
					return errors.New("daily collector cleanup alias")
				}
				return nil
			}); err != nil {
				b.pinsErr = errors.New("daily collector cleanup scope uncertain")
				return
			}
			if os.RemoveAll(root) != nil {
				b.pinsErr = errors.New("daily collector cleanup incomplete")
			}
		}
	})
	return b.pinsErr
}
func (b *xDailyBatch) isClosed() bool                      { return b.owner.host.isClosed() }
func (b *xDailyBatch) nativeManualNodePID() uint32         { return b.host.nativeManualNodePID() }
func (b *xDailyBatch) nativeManualPaths() (string, string) { return b.host.nativeManualPaths() }
func (b *xDailyBatch) nativeManualScope(root, profile string) bool {
	return b.host.nativeManualScope(root, profile)
}
func (b *xDailyBatch) nativeManualJob(check func(windows.Handle) error) error {
	return b.host.nativeManualJob(check)
}
func (b *xDailyBatch) terminateOwnedTree() { b.owner.host.terminateOwnedTree() }
func (b *xDailyBatch) rpcRoundTripObserved(ctx context.Context, op string, fields map[string]any, out any, progress func([]byte)) error {
	copyFields := map[string]any{"generation": b.generation}
	for k, v := range fields {
		copyFields[k] = v
	}
	r, err := b.host.pipe.Call(ctx, op, copyFields, progress)
	defer clear(r.Result)
	if err != nil {
		return err
	}
	if r.Code != "" {
		return &xApplicationRPCError{code: r.Code, diagnostic: r.Diagnostic}
	}
	if out != nil && json.Unmarshal(r.Result, out) != nil {
		return errors.New("daily batch response invalid")
	}
	return nil
}

type xDailyRoleProcess struct {
	role  xManualPilotWindowsRole
	batch *xDailyBatch
	done  chan error
	stop  chan struct{}
	ended chan struct{}
	once  sync.Once
}

func (p *xDailyRoleProcess) Driver() string {
	if p.role.role == "reader" {
		return "browser"
	}
	return "headless"
}
func (p *xDailyRoleProcess) PID() int           { return int(p.role.chrome.PID) }
func (p *xDailyRoleProcess) Done() <-chan error { return p.done }
func (p *xDailyRoleProcess) Terminate() {
	// Revoke this batch; broker abort retires its collector, not the reader Job.
	if p.batch.broker != nil {
		p.batch.broker.invalidate("owner-or-policy-changed")
	}
}
func (p *xDailyRoleProcess) CloseForRetry(context.Context) error {
	return errors.New("daily batch replacement forbidden")
}
func (p *xDailyRoleProcess) OpenExtensionsPage(context.Context) error {
	return errors.New("daily role navigation forbidden")
}
func (p *xDailyRoleProcess) ReplacementReadiness(context.Context) error {
	return errors.New("daily batch replacement forbidden")
}
func (b *xDailyBatch) managers(parent context.Context, reader, collector xManualPilotWindowsRole) (*captureruntime.Manager, *captureruntime.Manager, func(), error) {
	var processes []*xDailyRoleProcess
	stop := func() {
		for _, p := range processes {
			p.once.Do(func() { close(p.stop) })
			<-p.ended
		}
	}
	makeManager := func(role xManualPilotWindowsRole) (*captureruntime.Manager, error) {
		p := &xDailyRoleProcess{role: role, batch: b, done: make(chan error), stop: make(chan struct{}), ended: make(chan struct{})}
		processes = append(processes, p)
		go func() {
			defer close(p.ended)
			defer close(p.done)
			tick := time.NewTicker(25 * time.Millisecond)
			defer tick.Stop()
			for {
				select {
				case <-parent.Done():
					return
				case <-p.stop:
					return
				case <-tick.C:
					state, err := windows.WaitForSingleObject(role.chromeHandle, 0)
					if err != nil || state != uint32(windows.WAIT_TIMEOUT) {
						return
					}
				}
			}
		}()
		return captureruntime.New(p)
	}
	r, err := makeManager(reader)
	if err != nil {
		stop()
		return nil, nil, func() {}, err
	}
	c, err := makeManager(collector)
	if err != nil {
		stop()
		return nil, nil, func() {}, err
	}
	return r, c, stop, nil
}

func (n *xDailyOwner) PrepareReader(ctx context.Context, req readerbroker.Request, marker string) (readerbroker.NativePostPreparation, error) {
	if !xApplicationMarkerPath.MatchString(marker) || n.CheckReader(ctx) != nil {
		return readerbroker.NativePostPreparation{}, errors.New("daily reader unavailable")
	}
	n.host.postMu.Lock()
	if n.host.postBinding == nil {
		var err error
		if n.reader.mainWindow != nil {
			n.host.postBinding, err = n.reader.mainWindow.BorrowMainReaderBinding()
		} else {
			err = n.host.nativeManualJob(func(job windows.Handle) error {
				var e error
				n.host.postBinding, e = appshell.NewOwnedReaderBinding(job, n.reader.chrome.PID)
				return e
			})
		}
		if err != nil {
			n.host.postMu.Unlock()
			return readerbroker.NativePostPreparation{}, err
		}
	}
	binding := n.host.postBinding
	n.host.postMu.Unlock()
	p := xApplicationPostPorts{readerPID: n.reader.chrome.PID, check: n.CheckReader, bind: binding.Prepare, waitForeground: binding.WaitForForeground, release: binding.Release, now: time.Now, allowMainOrigin: true,
		call: func(call context.Context, op string, fields map[string]any, out *json.RawMessage) error {
			fields["owner"] = n.host.protocolOwner
			r, err := n.host.pipe.Call(call, op, fields, nil)
			if err != nil || r.Code != "" {
				clear(r.Result)
				return errors.New("daily post rejected")
			}
			*out = r.Result
			return nil
		}}
	return prepareXApplicationPost(ctx, req, n.origin+marker, p)
}
