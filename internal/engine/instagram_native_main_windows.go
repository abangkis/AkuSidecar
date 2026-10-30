//go:build windows && auth_session_dev

package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/abangkis/AkuSidecar/internal/appshell"
	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/ownedprocess"
	"github.com/abangkis/AkuSidecar/internal/ownedprofile"
	"golang.org/x/sys/windows"
)

type nativeMainDescriptor struct {
	Source          domain.Source `json:"source"`
	Recipe          string        `json:"recipe"`
	ReaderOwnerID   string        `json:"readerOwnerID"`
	ReaderProfileID string        `json:"readerProfileID"`
}
type instagramMainNativeTransport struct {
	protocol  appshell.MainBrowserSourceProtocol
	host      *xApplicationOwner
	reader    xManualPilotWindowsRole
	root      string
	mu        sync.Mutex
	batch     *instagramMainNativeCollector
	closed    bool
	closeOnce sync.Once
	closeErr  error
}
type instagramMainNativeCollector struct {
	transport *instagramMainNativeTransport
	host      *xApplicationOwner
	role      xManualPilotWindowsRole
	challenge sourceNativeChallenge
	deadline  time.Time
	once      sync.Once
	closeErr  error
	seeded    bool
}

// ConfigureInstagramMainDailyRuntime registers the immutable packaged recipe.
// Until native qualification is reviewed it records availability only, without
// opening a profile, spawning a process, or installing a permission factory.
func ConfigureInstagramMainDailyRuntime(parent context.Context, e *Engine, window *appshell.Window, manager *captureruntime.Manager, profile, subprofile, chrome string) error {
	if e == nil || e.config.Capture.InstagramCaptureMode() != config.XCaptureConcurrent {
		return nil
	}
	trial := e.config.Dev && e.config.Capture.InstagramTrial
	if !instagramNativeQualified && !trial {
		e.instagramDaily.mu.Lock()
		if !e.instagramDaily.attached && e.instagramDaily.factory == nil {
			e.instagramDaily.reason = "native-adapter-qualification-required"
		}
		e.instagramDaily.mu.Unlock()
		return nil
	}
	if !e.config.Dev || !e.config.WindowsCaptureSplit || parent == nil || parent.Err() != nil || window == nil || manager == nil || window.PID() != manager.PID() || !appshell.ValidProfileDirectory(subprofile) {
		return errors.New("native Instagram main owner required")
	}
	if !manager.Persistent() {
		if err := manager.RetainForProcess(); err != nil {
			return err
		}
	}
	protocol, ok := window.CaptureProtocol().(appshell.MainBrowserProtocol)
	if !ok {
		return errors.New("native Instagram main transport required")
	}
	if _, ok = protocol.(appshell.MainBrowserSourceProtocol); !ok {
		return errors.New("native Instagram source transport required")
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
	descriptor := ownedprofile.Descriptor{Root: root, Directory: profile, RootID: rootID, DirectoryID: profileID}
	scope, err := xMainDailyProfileScope(descriptor, subprofile)
	if err != nil {
		return err
	}
	options, err := MainBrowserTransportOptions(e.config)
	if err != nil {
		return err
	}
	if options == nil {
		return errors.New("native Instagram packaged transport required")
	}
	image, err := mainProfileAsset(chrome)
	if err != nil {
		return err
	}
	generation := manager.Snapshot().Generation
	d := e.dailyController(domain.SourceInstagram)
	d.state.mu.Lock()
	if d.state.attached || d.state.closed {
		d.state.mu.Unlock()
		return errors.New("native Instagram already attached")
	}
	d.state.attached = true
	d.state.mu.Unlock()
	return d.configure(parent, func(startup, process context.Context) (owner sourceDailyOwner, failure error) {
		defer func() {
			if failure != nil {
				failure = sourceNativeFail(failure, "startup", "unavailable")
			}
		}()
		if !manager.Accepts(generation) {
			return nil, errors.New("native Instagram main generation changed")
		}
		parentRoot := filepath.Join(e.config.Root, "runtime", "instagram-daily-collectors")
		if err := os.MkdirAll(parentRoot, 0700); err != nil {
			return nil, err
		}
		collectorRoot, err := os.MkdirTemp(parentRoot, "main-process-")
		if err != nil {
			return nil, err
		}
		cfg := xApplicationOwnerConfig{enabled: true, node: xApplicationOwnerAsset{path: options.Node, digest: options.NodeSHA256}, driver: xApplicationOwnerAsset{path: options.Driver, digest: options.DriverSHA256}, chrome: image, reader: descriptor, mainWindow: window, mainProfileDirectory: subprofile}
		h := &xApplicationOwner{cfg: cfg, worker: protocol.TransportWorker(), readerLease: &xMainBorrowedLease{descriptor: descriptor, manager: manager, generation: generation}, closed: make(chan struct{}), stopping: make(chan struct{}), readerDone: make(chan struct{})}
		t := &instagramMainNativeTransport{protocol: protocol.(appshell.MainBrowserSourceProtocol), host: h, root: collectorRoot}
		o := &sourceNativeOwner{engine: e, source: domain.SourceInstagram, recipe: instagramNativeRecipe, profile: scope, transport: t}
		bootstrap := map[string]any{"readerPID": window.PID(), "readerProfile": profile, "profileDirectory": subprofile, "chromePath": chrome, "acknowledgeLimitations": true, "allowProcessSessionDerivation": true}
		if trial {
			bootstrap["acknowledgeUnqualifiedRecipe"] = true
		}
		if err = t.call(startup, "source_bootstrap", bootstrap, nil); err != nil {
			return o, err
		}
		var reply nativeMainDescriptor
		if err = t.call(startup, "source_initialize", nil, &reply); err != nil {
			return o, err
		}
		if reply.Source != domain.SourceInstagram || reply.Recipe != instagramNativeRecipe || reply.ReaderOwnerID != strconv.Itoa(window.PID()) || reply.ReaderProfileID != profile {
			return o, errors.New("native Instagram initialization rejected")
		}
		t.reader, err = h.bindRole(startup, "reader", uint32(window.PID()), 1)
		if err != nil {
			return o, err
		}
		return o, t.CheckReader(startup)
	}, scope)
}

func decodeSourceNativeObservation(raw []byte, source domain.Source, out *domain.Observation) error {
	if len(raw) > 512*1024 || !xNativeCaptureJSON(raw) {
		return sourceNativeFail(nil, "capture", "observation")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(out) != nil || d.Decode(new(any)) != io.EOF || out.Source != source || validateObservation(*out) != nil {
		return sourceNativeFail(nil, "capture", "observation")
	}
	return nil
}
func (t *instagramMainNativeTransport) call(ctx context.Context, op string, fields map[string]any, out any) error {
	if ctx == nil || ctx.Err() != nil {
		return sourceNativeFail(nil, "wire", "canceled")
	}
	if fields == nil {
		fields = map[string]any{}
	}
	raw, err := t.protocol.CallSource(ctx, "instagram", op, fields)
	defer clear(raw)
	if err != nil {
		return sourceNativeIGWireFailure(ctx, err)
	}
	if observation, ok := out.(*domain.Observation); ok {
		return decodeSourceNativeObservation(raw, domain.SourceInstagram, observation)
	}
	if out != nil && xManualPilotDecode(raw, out) != nil {
		return sourceNativeFail(nil, "wire", "decode")
	}
	return nil
}
func (t *instagramMainNativeTransport) CheckReader(ctx context.Context) error {
	t.mu.Lock()
	closed := t.closed
	t.mu.Unlock()
	if closed || t.host.readerLease.Validate() != nil {
		return errors.New("native Instagram reader ownership lost")
	}
	if _, err := (&xManualPilotWindowsBackend{roles: map[string]xManualPilotWindowsRole{"reader": t.reader}}).snapshot(ctx, "reader"); err != nil {
		return err
	}
	var reply nativeMainDescriptor
	if err := t.call(ctx, "source_check", nil, &reply); err != nil {
		return err
	}
	if reply.Source != domain.SourceInstagram || reply.Recipe != instagramNativeRecipe || reply.ReaderOwnerID != strconv.FormatUint(uint64(t.reader.chrome.PID), 10) || reply.ReaderProfileID != t.reader.profile {
		return errors.New("native Instagram reader changed")
	}
	return nil
}
func (t *instagramMainNativeTransport) Begin(ctx context.Context, c sourceNativeChallenge, end time.Time) (collector sourceNativeCollector, failure error) {
	defer func() {
		if failure != nil {
			failure = sourceNativeFail(failure, "collector", "owner")
		}
	}()
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed || t.batch != nil || !end.After(time.Now()) {
		return nil, errors.New("native Instagram batch unavailable")
	}
	root, err := os.MkdirTemp(t.root, "batch-")
	if err != nil {
		return nil, err
	}
	profile := filepath.Join(root, "profile")
	if err = os.Mkdir(profile, 0700); err != nil {
		return nil, err
	}
	rootID, err := ownedprocess.DirectoryID(root)
	if err != nil {
		return nil, err
	}
	profileID, err := ownedprocess.DirectoryID(profile)
	if err != nil {
		return nil, err
	}
	desc := ownedprofile.Descriptor{Root: root, Directory: profile, RootID: rootID, DirectoryID: profileID}
	lease, err := ownedprofile.Acquire(desc)
	if err != nil {
		return nil, err
	}
	h := &xApplicationOwner{cfg: t.host.cfg, worker: t.host.worker, readerLease: t.host.readerLease, collectorLease: lease, closed: t.host.closed, stopping: t.host.stopping}
	h.cfg.collector = desc
	b := &instagramMainNativeCollector{transport: t, host: h, challenge: c, deadline: end}
	t.batch = b
	var reply struct {
		Source             domain.Source `json:"source"`
		Recipe             string        `json:"recipe"`
		ReaderOwnerID      string        `json:"readerOwnerID"`
		ReaderProfileID    string        `json:"readerProfileID"`
		CollectorOwnerID   string        `json:"collectorOwnerID"`
		CollectorProfileID string        `json:"collectorProfileID"`
		CollectorPID       uint32        `json:"collectorPID"`
	}
	err = t.call(ctx, "source_begin", map[string]any{"generation": c.Generation, "nonce": c.Nonce, "profileScope": c.ProfileScope, "collectorProfile": profile, "deadlineMS": end.UnixMilli()}, &reply)
	if err != nil {
		return b, err
	}
	if reply.Source != domain.SourceInstagram || reply.Recipe != instagramNativeRecipe || reply.ReaderOwnerID != strconv.FormatUint(uint64(t.reader.chrome.PID), 10) || reply.ReaderProfileID != t.reader.profile || reply.CollectorOwnerID != strconv.FormatUint(uint64(reply.CollectorPID), 10) || reply.CollectorProfileID != profile || reply.CollectorPID == t.reader.chrome.PID {
		return b, errors.New("native Instagram batch ownership rejected")
	}
	b.role, err = h.bindRole(ctx, "collector", reply.CollectorPID, 1)
	return b, err
}
func (b *instagramMainNativeCollector) fields(c sourceNativeChallenge) map[string]any {
	return map[string]any{"generation": c.Generation, "nonce": c.Nonce, "profileScope": c.ProfileScope}
}
func (b *instagramMainNativeCollector) Seed(ctx context.Context, c sourceNativeChallenge) (sourceNativePair, error) {
	if b.seeded || b.role.chromeHandle == 0 {
		return sourceNativePair{}, errors.New("native Instagram collector enrollment required")
	}
	if err := b.transport.CheckReader(ctx); err != nil {
		return sourceNativePair{}, err
	}
	if _, err := (&xManualPilotWindowsBackend{roles: map[string]xManualPilotWindowsRole{"collector": b.role}}).snapshot(ctx, "collector"); err != nil {
		return sourceNativePair{}, err
	}
	b.seeded = true
	var pair sourceNativePair
	err := b.transport.call(ctx, "source_seed", b.fields(c), &pair)
	return pair, err
}
func (b *instagramMainNativeCollector) Proof(ctx context.Context, c sourceNativeChallenge) (sourceNativePair, error) {
	var pair sourceNativePair
	err := b.transport.call(ctx, "source_proof", b.fields(c), &pair)
	return pair, err
}
func (b *instagramMainNativeCollector) CheckOwnership(ctx context.Context, p sourceNativePair) error {
	for _, v := range []struct {
		proof sourceNativeProof
		role  xManualPilotWindowsRole
	}{{p.Reader, b.transport.reader}, {p.Collector, b.role}} {
		if v.proof.OwnerID != strconv.FormatUint(uint64(v.role.chrome.PID), 10) || v.proof.ProfileID != v.role.profile {
			return errors.New("native Instagram proof owner changed")
		}
		if _, err := (&xManualPilotWindowsBackend{roles: map[string]xManualPilotWindowsRole{v.role.role: v.role}}).snapshot(ctx, v.role.role); err != nil {
			return err
		}
	}
	return b.transport.host.readerLease.Validate()
}
func (b *instagramMainNativeCollector) Capture(ctx context.Context, payload map[string]any) (domain.Observation, error) {
	if !b.seeded || !time.Now().Before(b.deadline) {
		return domain.Observation{}, errors.New("native Instagram capture unavailable")
	}
	if err := b.transport.CheckReader(ctx); err != nil {
		return domain.Observation{}, err
	}
	if _, err := (&xManualPilotWindowsBackend{roles: map[string]xManualPilotWindowsRole{"collector": b.role}}).snapshot(ctx, "collector"); err != nil {
		return domain.Observation{}, err
	}
	fields := b.fields(b.challenge)
	fields["payload"] = payload
	var observation domain.Observation
	err := b.transport.call(ctx, "source_capture", fields, &observation)
	return observation, err
}
func (b *instagramMainNativeCollector) Close(ctx context.Context) error {
	b.once.Do(func() {
		var reply struct {
			Closed bool `json:"closed"`
		}
		err := b.transport.call(ctx, "source_end", b.fields(b.challenge), &reply)
		if err != nil || !reply.Closed {
			b.closeErr = errors.New("native Instagram target cleanup uncertain")
		}
		if b.role.chromeHandle == 0 {
			b.closeErr = errors.Join(b.closeErr, errors.New("native Instagram collector exit unobserved"))
			return
		}
		state, waitErr := windows.WaitForSingleObject(b.role.chromeHandle, 0)
		if waitErr != nil || state != uint32(windows.WAIT_OBJECT_0) {
			b.closeErr = errors.Join(b.closeErr, errors.New("native Instagram collector still alive"))
			return
		}
		// Reuse the existing independently verified pins/profile disposal path.
		disposal := &xDailyBatch{host: b.host, owner: &xDailyOwner{root: b.transport.root}}
		b.closeErr = errors.Join(b.closeErr, disposal.releasePins())
		if b.closeErr == nil {
			b.transport.mu.Lock()
			if b.transport.batch == b {
				b.transport.batch = nil
			}
			b.transport.mu.Unlock()
		}
	})
	return b.closeErr
}
func (t *instagramMainNativeTransport) Close(ctx context.Context) error {
	t.closeOnce.Do(func() { t.closeErr = t.closeOwned(ctx) })
	return t.closeErr
}
func (t *instagramMainNativeTransport) closeOwned(ctx context.Context) error {
	t.mu.Lock()
	t.closed = true
	b := t.batch
	t.mu.Unlock()
	var batchErr error
	if b != nil {
		batchErr = b.Close(ctx)
	}
	var reply struct {
		Closed bool `json:"closed"`
	}
	err := t.call(ctx, "source_close", nil, &reply)
	if err != nil || !reply.Closed {
		return errors.Join(batchErr, errors.New("native Instagram owner cleanup uncertain"))
	}
	for _, r := range t.host.roles {
		for _, pin := range []windows.Handle{r.chromeHandle, r.rootPin, r.profilePin} {
			if pin != 0 {
				err = errors.Join(err, windows.CloseHandle(pin))
			}
		}
	}
	t.host.roles = nil
	return errors.Join(batchErr, err)
}

var _ sourceNativeTransport = (*instagramMainNativeTransport)(nil)
