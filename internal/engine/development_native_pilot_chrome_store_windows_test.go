//go:build windows && auth_session_dev && auth_session_fixture

package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/readerbroker"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
	"github.com/abangkis/AkuSidecar/internal/store"
	"golang.org/x/sys/windows"
)

// This capability is only compiled as part of the explicit fixture test. Its
// records are constructed from newly launched Job-owned processes below, never
// decoded from JSON, and native storeStartup remains a separate strict path.
type nativeChromeStoreFixtureAdmission struct {
	broker  *nativePilotBroker
	store   *store.Store
	root    string
	records map[string]*nativeManualChromeRecord
	seal    *nativePilotSeal
}

func (a *nativeChromeStoreFixtureAdmission) validate(ctx context.Context) (store.DevelopmentManualPilotBinding, error) {
	var empty store.DevelopmentManualPilotBinding
	if a == nil || a.seal == nil || len(a.records) != 2 || os.Getenv("AKU_MANUAL_CHROME_STORE_ENABLED") != "1" {
		return empty, errors.New("owned fixture scope unavailable")
	}
	b := a.broker
	if d := b.check(ctx); !d.OfflineValidated || d.ManualEligible || d.CanAuthorize || d.NativeQualified || d.StorePermit {
		return empty, errors.New("offline fixture validation rejected")
	}
	for _, r := range a.records {
		rel, err := filepath.Rel(a.root, r.bridge.config.Root)
		if err != nil || rel == "." || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) || r.rootPin == 0 || r.profilePin == 0 || r.osCheck() != nil {
			return empty, errors.New("fixture physical scope rejected")
		}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	c := b.controller
	if b.stopped || !b.cfg.offlineTest || c == nil {
		return empty, errors.New("offline fixture closed")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	v := c.vector
	if c.stopped || !c.offlineTest || v == nil || v.provenance != nativePilotOfflineOnly || v.seal != c.adapter.seal || v.binding != c.adapter.cfg.binding {
		return empty, errors.New("offline vector rejected")
	}
	end := b.deadline()
	if v.expires.Before(end) {
		end = v.expires
	}
	return store.DevelopmentManualPilotBinding{Digest: nativePilotStoreDigest(v.binding), Attempt: v.binding.attempt, OwnerGeneration: v.binding.collector.generation, StartedAt: b.cfg.pilot.attemptStarted, Deadline: end}, nil
}
func (a *nativeChromeStoreFixtureAdmission) issue(ctx context.Context, session, run string) (*store.ManualPilotPermit, error) {
	v, err := a.validate(ctx)
	if err != nil {
		return nil, err
	}
	p, err := a.store.IssueSyntheticManualPilot(ctx, store.ManualPilotSyntheticRequest{FactoryOptIn: true, SessionID: session, RunID: run, StartedAt: v.StartedAt, ValidUntil: v.Deadline, SyntheticOwnerRef: "synthetic:chrome-fixture-sha256:" + v.Digest, SyntheticPairRef: "synthetic:chrome-fixture-sha256:" + v.Digest})
	if err != nil {
		return nil, err
	}
	after, err := a.validate(ctx)
	if err != nil || after != v {
		revoke, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_, _ = a.store.RevokeManualPilot(revoke, p, domain.NewID("fixture-revoke"))
		return nil, errors.New("fixture invalidated during issuance")
	}
	return p, nil
}

func nativeChromeStoreLaunch(t *testing.T, ctx context.Context, root, project, parentID string, f *nativeManualChromeHTTP, registry *[]*nativeManualChromeRecord) map[string]*nativeManualChromeRecord {
	t.Helper()
	records := map[string]*nativeManualChromeRecord{}
	for _, role := range []string{"reader", "collector"} {
		roleRoot := filepath.Join(root, "run-"+role)
		if err := os.Mkdir(roleRoot, 0700); err != nil {
			t.Fatal(err)
		}
		cfg := chromeFixtureConfig{NodePath: os.Getenv("AKU_CHROME_FIXTURE_NODE"), ChromePath: os.Getenv("AKU_CHROME_FIXTURE_EXE"), Root: roleRoot, Origin: f.server.URL, DriverPath: filepath.Join(project, "scripts", "auth-session-fixture", "chrome-manual-pilot-driver.mjs"), NativeReaderStartup: role == "reader"}
		bridge, err := startChromeFixtureBridge(ctx, cfg)
		if err != nil {
			t.Fatal("bridge: " + nativeManualChromeBridgeReason(err))
		}
		r := &nativeManualChromeRecord{role: role, bridge: bridge, parent: parentID}
		records[role] = r
		*registry = append(*registry, r)
		t.Cleanup(func() {
			bridge.terminateOwnedTree()
			select {
			case <-bridge.nodeWaited:
			case <-time.After(5 * time.Second):
				t.Error("Node exit unverified")
			}
			if r.process != nil {
				select {
				case <-r.process.Done():
				case <-time.After(5 * time.Second):
					t.Error("Chrome exit unverified")
				}
			}
			if bridge.cleanupError() != nil || !bridge.cleanupVerified() {
				t.Error("Job cleanup unverified")
			}
			if r.profilePin != 0 {
				windows.CloseHandle(r.profilePin)
			}
			if r.rootPin != 0 {
				windows.CloseHandle(r.rootPin)
			}
		})
		if err := bridge.assertBridgeOnly(ctx); err != nil {
			t.Fatal(err)
		}
		r.node, err = processFixtureProcessIdentity(bridge.pid)
		if err != nil {
			t.Fatal(err)
		}
		launch := "owner"
		if role == "collector" {
			launch = "collector"
		}
		r.process, err = bridge.launch(ctx, launch)
		if err != nil {
			t.Fatal("fresh Chrome launch failed")
		}
		r.chrome, err = processFixtureProcessIdentity(r.process.pid)
		if err != nil {
			t.Fatal(err)
		}
		r.rootID, err = processFixtureDirectoryID(roleRoot)
		if err != nil {
			t.Fatal(err)
		}
		profile := filepath.Join(roleRoot, "shared")
		r.profileID, err = processFixtureDirectoryID(profile)
		if err != nil {
			t.Fatal(err)
		}
		r.rootPin, err = nativeManualChromePin(roleRoot)
		if err != nil {
			t.Fatal(err)
		}
		r.profilePin, err = nativeManualChromePin(profile)
		if err != nil {
			t.Fatal(err)
		}
		if err := r.osCheck(); err != nil {
			t.Fatal(err)
		}
		if role == "reader" {
			if r.process.Login(ctx) != nil {
				t.Fatal("loopback login failed")
			}
		} else {
			f.mu.Lock()
			token := f.token
			f.mu.Unlock()
			var seed struct {
				Seeded bool `json:"seeded"`
			}
			if bridge.rpc(ctx, "seed_fixture", map[string]any{"owner": 1, "value": token}, &seed) != nil || !seed.Seeded {
				t.Fatal("invented cookie seed failed")
			}
		}
	}
	if records["reader"].chrome.PID == records["collector"].chrome.PID || records["reader"].profileID == records["collector"].profileID {
		t.Fatal("fixture owners overlap")
	}
	return records
}

func nativeChromeStoreBroker(t *testing.T, ctx context.Context, records map[string]*nativeManualChromeRecord, started time.Time) *nativePilotBroker {
	t.Helper()
	binding := nativePilotBinding{account: nativeManualChromeAccount, enrollment: "offline-store-fixture", pair: domain.NewID("offline-pair"), attempt: domain.NewID("offline-attempt"), derivation: domain.NewID("offline-seed"), policy: 1, consent: 1}
	binding.reader = nativePilotOwner{role: "reader", profile: filepath.Join(records["reader"].bridge.config.Root, "shared"), parent: records["reader"].parent}
	binding.collector = nativePilotOwner{role: "collector", profile: filepath.Join(records["collector"].bridge.config.Root, "shared"), parent: records["collector"].parent}
	var prepared atomic.Int32
	ports := nativePilotBrokerPorts{now: time.Now,
		prepare: func(call context.Context, req nativePilotPrepareRequest) (nativePilotPreparedRole, error) {
			r := records[req.role]
			var ready struct {
				Handle string                 `json:"handle"`
				Pins   nativeManualChromePins `json:"pins"`
			}
			if err := r.bridge.rpc(call, "prepare", map[string]any{"owner": 1}, &ready); err != nil {
				return nativePilotPreparedRole{}, err
			}
			o, snapshot, err := r.inspect(call)
			if err != nil || !snapshot.Paused || snapshot.Pins != ready.Pins {
				return nativePilotPreparedRole{}, errors.New("fixture prepare pins rejected")
			}
			r.prepared = nativePilotPreparedRole{owner: o, handle: ready.Handle}
			prepared.Add(1)
			return r.prepared, nil
		},
		inspect: func(call context.Context, p nativePilotPreparedRole) (nativePilotOwner, error) {
			o, _, err := records[p.owner.role].inspect(call)
			return o, err
		},
		acquire: func(call context.Context, p nativePilotPreparedRole, ch nativePilotCaptureChallenge) ([]byte, error) {
			if prepared.Load() != 2 {
				return nil, errors.New("fixture pair not prepared")
			}
			request, err := ch.requestJSON()
			if err != nil {
				return nil, err
			}
			defer clear(request)
			var raw json.RawMessage
			if err := records[p.owner.role].bridge.rpc(call, "acquire", map[string]any{"owner": 1, "handle": p.handle, "request": json.RawMessage(request)}, &raw); err != nil {
				return nil, err
			}
			defer clear(raw)
			return bytes.Clone(raw), nil
		},
		state: func(call context.Context) (nativePilotBrokerState, error) {
			r := records["reader"]
			return nativePilotBrokerState{account: binding.account, enrollment: binding.enrollment, pair: binding.pair, attempt: binding.attempt, derivation: binding.derivation, policy: 1, consent: 1, readerWindows: len(processFixtureWindows(r.chrome.PID)), completeInventory: true, parentAlive: call.Err() == nil, channelHealthy: !r.bridge.isClosed(), invalidationsReady: true, optedIn: true}, nil
		},
		abort: func(call context.Context, p nativePilotPreparedRole, preserve bool) error {
			var reply struct {
				Closed bool `json:"closed"`
			}
			err := records[p.owner.role].bridge.rpc(call, "abort", map[string]any{"owner": 1, "handle": p.handle, "preserveReader": preserve}, &reply)
			if err != nil || !reply.Closed {
				return errors.New("fixture abort failed")
			}
			return nil
		},
	}
	b, _ := newNativePilotBroker(ctx, nativePilotBrokerConfig{enabled: true, offlineTest: true, pilot: nativePilotConfig{enabled: true, contract: nativePilotContract, limitationAcknowledged: true, binding: binding, attemptStarted: started}}, ports)
	if b == nil {
		t.Fatal("offline broker unavailable")
	}
	return b
}

func TestNativePilotChromeStore(t *testing.T) {
	if os.Getenv("AKU_MANUAL_CHROME_STORE_ENABLED") != "1" {
		t.Skip("explicit synthetic Chrome Store helper required")
	}
	project, parentRoot, marker := os.Getenv("AKU_NATIVE_MANUAL_PROJECT_ROOT"), os.Getenv("AKU_CHROME_FIXTURE_ROOT"), os.Getenv("AKU_CHROME_FIXTURE_CLEANUP_MARKER")
	if !filepath.IsAbs(project) || !filepath.IsAbs(parentRoot) || !samePath(filepath.Dir(marker), parentRoot) {
		t.Fatal("fixture root unavailable")
	}
	prior, err := os.Getwd()
	if err != nil || os.Chdir(filepath.Join(project, "internal", "engine")) != nil {
		t.Fatal("fixture workspace unavailable")
	}
	defer os.Chdir(prior)
	var registry []*nativeManualChromeRecord
	t.Cleanup(func() {
		for _, r := range registry {
			if r.bridge.cleanupError() != nil || !r.bridge.cleanupVerified() {
				t.Error("owned cleanup not verified")
				return
			}
		}
		if len(registry) != 4 {
			t.Error("fixture role inventory incomplete")
			return
		}
		if err := os.WriteFile(marker, []byte("verified\n"), 0600); err != nil {
			t.Error(err)
		}
	})
	for _, scenario := range []string{"capture-and-reasoning", "revoke-during-capture"} {
		t.Run(scenario, func(t *testing.T) {
			root := filepath.Join(parentRoot, "run-"+scenario)
			if err := os.Mkdir(root, 0700); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 55*time.Second)
			defer cancel()
			started := time.Now()
			identity, err := processFixtureProcessIdentity(uint32(os.Getpid()))
			if err != nil {
				t.Fatal(err)
			}
			parentID := strconv.FormatUint(uint64(identity.PID), 10) + "/" + identity.Created
			f := startNativeManualChromeHTTP(t)
			f.feedEntered = make(chan struct{}, 1)
			f.feedRelease = make(chan struct{})
			records := nativeChromeStoreLaunch(t, ctx, root, project, parentID, f, &registry)
			b := nativeChromeStoreBroker(t, ctx, records, started)
			d := b.start(ctx)
			if !d.OfflineValidated || d.ManualEligible || d.NativeQualified || d.CanAuthorize || d.StorePermit {
				t.Fatalf("startup %+v", d)
			}
			if _, err := b.storeStartup(ctx); err == nil {
				t.Fatal("offline proof became native authority")
			}
			reader, err := captureruntime.New(records["reader"].process)
			if err != nil {
				t.Fatal(err)
			}
			collector, err := captureruntime.New(records["collector"].process)
			if err != nil {
				t.Fatal(err)
			}
			backend := &nativePilotChromeCapture{bridge: records["collector"].bridge, owner: records["collector"].prepared.owner}
			runtime, err := newNativePilotRuntime(b, reader, collector, backend, func(call context.Context, _ readerbroker.Request) error {
				return records["reader"].process.OpenFixtureIndependentWindow(call)
			})
			if err != nil {
				t.Fatal(err)
			}
			settings := domain.DefaultSettings("expanded", "quiet", "promote_unused_budget", false)
			settings.CollectionMode = "headless"
			settings.ActiveSources = []domain.Source{domain.SourceX}
			s, err := store.Open(filepath.Join(root, "synthetic.db"), settings)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			session, err := s.CreateUpdateSession(ctx, "Synthetic Chrome DOM capture", settings, domain.UpdatePolicy{Trigger: domain.UpdateTriggerUser, Delivery: domain.UpdateDeliveryVisible, BudgetAuthority: domain.BudgetAuthorityUser})
			if err != nil {
				t.Fatal(err)
			}
			command, err := s.StartRun(ctx, session.Runs[0].ID, map[string]any{"captureRuntime": map[string]any{"driver": "headless"}})
			if err != nil {
				t.Fatal(err)
			}
			e := New(s, reasoning.Deterministic{}, config.Config{Dev: true, Capture: config.CaptureConfig{MaxAcquisitionRounds: 1}}, log.New(io.Discard, "", 0))
			defer func() { e.Shutdown(); e.WaitForIdle(3 * time.Second) }()
			admission := &nativeChromeStoreFixtureAdmission{broker: b, store: s, root: root, records: records, seal: &nativePilotSeal{}}
			if err := e.bindNativePilotStoreAdmission(ctx, runtime, session.ID, command.RunID, admission); err != nil {
				t.Fatal(err)
			}
			results := make(chan nativePilotStoreResult, 1)
			go func() { results <- e.captureNativePilot(ctx, runtime, command.ID) }()
			select {
			case <-f.feedEntered:
			case <-ctx.Done():
				t.Fatal("Chrome DOM feed request did not arrive")
			}
			if err := runtime.openReader(ctx, manualRuntimeRequest()); err != nil {
				t.Fatal("reader while capture: ", err)
			}
			if o, snapshot, err := records["reader"].inspect(ctx); err != nil || snapshot.WindowCount != 2 || snapshot.BootstrapCount != 1 || o != records["reader"].prepared.owner {
				t.Fatal("reader/bootstrap changed during capture")
			}
			if o, _, err := records["collector"].inspect(ctx); err != nil || o != backend.owner || collector.PID() != backend.owner.pid || collector.Snapshot().Generation != backend.owner.generation {
				t.Fatal("collector changed during capture")
			}
			if scenario == "revoke-during-capture" {
				b.cancel(command.ID)
			}
			close(f.feedRelease)
			var result nativePilotStoreResult
			select {
			case result = <-results:
			case <-ctx.Done():
				t.Fatal("capture did not return")
			}
			obs, err := s.Observations(ctx, command.RunID)
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "revoke-during-capture" {
				if result.Outcome == store.Committed || len(obs) != 0 {
					t.Fatalf("late capture persisted %+v", result)
				}
				if backend.calls.Load() != 1 {
					t.Fatal("capture count changed")
				}
				t.Log("late real DOM result rejected after fixture revocation")
				return
			}
			if result.Outcome != store.Committed || result.PersistenceError != nil || result.DeliveryError != nil || len(obs) != 1 {
				t.Fatalf("capture result=%+v observations=%d", result, len(obs))
			}
			if !e.WaitForIdle(3 * time.Second) {
				t.Fatal("reasoning did not finish")
			}
			run, err := s.GetRun(ctx, command.RunID)
			if err != nil || run.Status != "completed" {
				t.Fatal("actual reasoning continuation failed")
			}
			block := obs[0].Snapshots[0].Blocks[0]
			if block.Text != "A real Chrome DOM post about bounded local collection." || block.PlatformID != "x:status:1908123456789012346" || block.Permalink != "https://x.com/synthetic/status/1908123456789012346" {
				t.Fatal("persisted evidence did not match actual fixture DOM")
			}
			if again := e.captureNativePilot(ctx, runtime, command.ID); again.Outcome != store.AlreadyCommitted || backend.calls.Load() != 1 {
				t.Fatal("duplicate triggered another Chrome capture")
			}
			for _, index := range []int{1, 0} {
				var reply map[string]any
				if err := records["reader"].bridge.rpc(ctx, "close_window", map[string]any{"owner": 1, "index": index}, &reply); err != nil {
					t.Fatal("reader close failed")
				}
			}
			if d := b.check(ctx); d.OfflineValidated || d.ManualEligible {
				t.Fatal("last reader loss did not close admission")
			}
			t.Log("real Chrome DOM -> Store claim/result -> deterministic Engine reasoning verified; reader overlap and exactly-once replay preserved")
		})
	}
}
