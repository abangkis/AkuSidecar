//go:build auth_session_dev

package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// All ownership values below are invented. This compiled backend can produce
// only offline vectors, including when a private JSON object claims otherwise.
type testXManualOwnerBackend struct {
	mu              sync.Mutex
	core            map[string]nativePilotOwner
	pins            map[string]map[string]any
	ready, acquired map[string]bool
	cleaned         map[string]bool
	cleanupCalls    map[string]int
	clock           time.Time
	ops             map[string]int
	windows         int
	state           nativePilotBrokerState
	barrier         chan struct{}
	fault           string
}

func (*testXManualOwnerBackend) provenance() nativePilotProvenance { return nativePilotOfflineOnly }
func (f *testXManualOwnerBackend) now() time.Time                  { return f.clock }
func (f *testXManualOwnerBackend) snapshot(ctx context.Context, role string) (nativePilotOwner, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if ctx.Err() != nil {
		return nativePilotOwner{}, ctx.Err()
	}
	o, ok := f.core[role]
	if !ok {
		return o, errors.New("private-owner-missing")
	}
	return o, nil
}
func testXManualReply(out any, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	dest, ok := out.(*json.RawMessage)
	if !ok {
		return errors.New("private-output-invalid")
	}
	*dest = raw
	return nil
}
func (f *testXManualOwnerBackend) rpc(ctx context.Context, role, op string, fields map[string]any, out any) error {
	f.mu.Lock()
	f.ops[op]++
	pins := f.pins[role]
	switch op {
	case "manual_inventory":
		count := f.windows
		f.mu.Unlock()
		return testXManualReply(out, map[string]any{"windowCount": count, "bootstrapCount": 1})
	case "manual_prepare":
		f.ready[role] = true
		if f.fault == "os-after-prepare" && role == "reader" {
			o := f.core[role]
			o.creation = "changed-private-creation"
			f.core[role] = o
		}
		value := map[string]any{"handle": "held-" + role, "pins": pins}
		if f.fault == "qa-boolean" || f.fault == "prepare-cleanup-uncertain" {
			value["canAuthorize"] = false
		}
		if f.fault == "wrong-context" && role == "reader" {
			clone := map[string]any{}
			for k, v := range pins {
				clone[k] = v
			}
			clone["ownerId"] = "wrong-owner"
			value["pins"] = clone
		}
		f.mu.Unlock()
		return testXManualReply(out, value)
	case "manual_inspect":
		count, bootstrap := f.windows, 1
		if role == "collector" {
			count, bootstrap = 0, 0
		}
		value := map[string]any{"pins": pins, "windowCount": count, "bootstrapCount": bootstrap, "paused": !f.acquired[role], "invalid": false, "headless": role == "collector"}
		f.mu.Unlock()
		return testXManualReply(out, value)
	case "manual_acquire":
		if len(f.ready) != 2 || f.acquired[role] || fields["handle"] != "held-"+role {
			f.mu.Unlock()
			return errors.New("private-rendezvous-invalid")
		}
		f.acquired[role] = true
		if len(f.acquired) == 2 {
			close(f.barrier)
		}
		barrier := f.barrier
		f.mu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-barrier:
		}
		var req map[string]string
		if json.Unmarshal(fields["request"].(json.RawMessage), &req) != nil {
			return errors.New("private-request-invalid")
		}
		at := float64(f.clock.UnixMilli())
		value := map[string]any{"schema": nativePilotStartupWireSchema, "challenge": req["challenge"], "role": role, "accountId": nativeManualTestAccount, "ownerId": pins["ownerId"], "profileId": pins["profileId"], "contextId": pins["contextId"], "targetId": pins["targetId"], "frameId": pins["frameId"], "documentId": pins["documentId"], "identityRequestId": "request-identity", "settingsRequestId": "request-settings", "identityObservedAtMS": at, "settingsObservedAtMS": at, "elapsedMS": 1, "qualification": nativePilotSettingsQualification}
		if f.fault == "account" {
			value["accountId"] = "1908123456789012346"
		}
		if f.fault == "post-acquire-document" && role == "reader" {
			f.mu.Lock()
			f.pins[role]["documentId"] = "new-private-document"
			f.mu.Unlock()
		}
		return testXManualReply(out, value)
	default:
		f.mu.Unlock()
		return errors.New("private-op-invalid")
	}
}
func (f *testXManualOwnerBackend) cleanup(_ context.Context, role, handle string, preserve bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cleanupCalls[role]++
	f.cleaned[role] = preserve
	if f.fault == "prepare-cleanup-uncertain" && f.cleanupCalls[role] == 1 {
		return errors.New("private-cleanup-error")
	}
	return nil
}

const nativeManualTestAccount = "1908123456789012345"

func testXManualOwnerConfig() (xManualPilotOwnerConfig, *testXManualOwnerBackend) {
	// Whole seconds round-trip exactly through the wire's floating millisecond
	// projection. This frozen clock belongs only to the offline fake backend.
	f := &testXManualOwnerBackend{core: map[string]nativePilotOwner{}, pins: map[string]map[string]any{}, ready: map[string]bool{}, acquired: map[string]bool{}, cleaned: map[string]bool{}, cleanupCalls: map[string]int{}, clock: time.Now().UTC().Truncate(time.Second), ops: map[string]int{}, windows: 1, barrier: make(chan struct{})}
	b := nativePilotBinding{account: nativeManualTestAccount, enrollment: "private-enrollment", pair: "private-pair", attempt: "private-attempt", derivation: "private-derivation", policy: 1, consent: 1}
	for i, role := range []string{"reader", "collector"} {
		pid := 412 + i*100
		profile := "owned-profile-" + role
		core := nativePilotOwner{role: role, owner: strconv.Itoa(pid), profile: profile, directory: "physical-directory-" + role, image: "owned-chrome-image", creation: "creation-" + role, job: "one-shared-owned-job", parent: "one-owned-parent", pid: pid, generation: 1}
		f.core[role] = core
		f.pins[role] = map[string]any{"ownerId": core.owner, "profileId": core.profile, "contextId": "2", "targetId": "target-" + role, "frameId": "frame-" + role, "documentId": "document-" + role}
		selected := nativePilotOwner{role: role, profile: profile, parent: core.parent}
		if role == "reader" {
			b.reader = selected
		} else {
			b.collector = selected
		}
	}
	f.state = nativePilotBrokerState{account: b.account, enrollment: b.enrollment, pair: b.pair, attempt: b.attempt, derivation: b.derivation, policy: b.policy, consent: b.consent, readerWindows: 1, completeInventory: true, parentAlive: true, channelHealthy: true, invalidationsReady: true, optedIn: true}
	cfg := xManualPilotOwnerConfig{enabled: true, pilot: nativePilotConfig{contract: nativePilotContract, enabled: true, limitationAcknowledged: true, binding: b, attemptStarted: f.clock}, policy: func(context.Context) (nativePilotBrokerState, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		return f.state, nil
	}}
	return cfg, f
}
func testXManualBroker(t *testing.T, cfg xManualPilotOwnerConfig, f *testXManualOwnerBackend) *nativePilotBroker {
	t.Helper()
	parent, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	b, d := newXManualPilotOwnerBrokerWithBackend(parent, cfg, f)
	if b == nil || d.Reason != "admission-closed" {
		t.Fatal("offline owner composition unavailable")
	}
	return b
}
func assertXManualOffline(t *testing.T, d nativePilotDiagnostic, reason string) {
	t.Helper()
	if d.Reason != reason || d.CanAuthorize || d.NativeQualified || d.StorePermit || d.ManualEligible {
		t.Fatalf("unexpected closed diagnostic: %+v", d)
	}
}

func TestXManualPilotOwnerDefaultOff(t *testing.T) {
	cfg, f := testXManualOwnerConfig()
	cfg.enabled = false
	b, d := newXManualPilotOwnerBrokerWithBackend(nil, cfg, nil)
	assertXManualOffline(t, d, "admission-disabled")
	if b != nil || len(f.ops) != 0 {
		t.Fatal("disabled owner port had effects")
	}
}
func TestXManualPilotOwnerSharedTransportFlow(t *testing.T) {
	cfg, f := testXManualOwnerConfig()
	b := testXManualBroker(t, cfg, f)
	d := b.start(context.Background())
	assertXManualOffline(t, d, "manual-attempt-valid")
	if !d.OfflineValidated {
		t.Fatal("offline owner vector missing")
	}
	ticket, d := b.dispatch(context.Background(), "capture-1")
	assertXManualOffline(t, d, "work-dispatched")
	collector := ticket.collector
	assertXManualOffline(t, b.claim(context.Background(), ticket, func() bool { return true }), "work-claimed")
	assertXManualOffline(t, b.openReader(context.Background(), func() bool { f.mu.Lock(); f.windows++; f.mu.Unlock(); return true }), "reader-admitted")
	results := 0
	assertXManualOffline(t, b.complete(context.Background(), ticket, func() bool { results++; return true }), "result-completed")
	assertXManualOffline(t, b.complete(context.Background(), ticket, func() bool { results++; return true }), "result-duplicate")
	if results != 1 || ticket.collector != collector || f.ops["manual_prepare"] != 2 || f.ops["manual_acquire"] != 2 || collector.context != "2" {
		t.Fatal("shared transport serialized rendezvous, remapped identity or replaced collector")
	}
	assertXManualOffline(t, b.invalidate("opted-out"), "opted-out")
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.cleaned["reader"] || f.cleaned["collector"] {
		t.Fatal("cleanup did not preserve reader")
	}
}
func TestXManualPilotOwnerFailuresNeverPromote(t *testing.T) {
	for _, fault := range []string{"qa-boolean", "wrong-context", "os-after-prepare", "post-acquire-document", "account"} {
		t.Run(fault, func(t *testing.T) {
			cfg, f := testXManualOwnerConfig()
			f.fault = fault
			b := testXManualBroker(t, cfg, f)
			d := b.start(context.Background())
			if d.OfflineValidated || d.ManualEligible || d.CanAuthorize || d.NativeQualified || d.StorePermit || !b.stopped {
				t.Fatal("invalid private ownership/evidence admitted")
			}
			if _, d := b.dispatch(context.Background(), "late"); d.OfflineValidated || d.ManualEligible {
				t.Fatal("failed owner attempt revived")
			}
		})
	}
}
func TestXManualPilotOwnerKnownPolicyLoss(t *testing.T) {
	for _, fault := range []string{"channel", "observers", "optout", "consent", "last-window"} {
		t.Run(fault, func(t *testing.T) {
			cfg, f := testXManualOwnerConfig()
			b := testXManualBroker(t, cfg, f)
			if d := b.start(context.Background()); !d.OfflineValidated {
				t.Fatalf("offline startup failed: reason=%s", d.Reason)
			}
			ticket, _ := b.dispatch(context.Background(), "capture-1")
			b.claim(context.Background(), ticket, func() bool { return true })
			f.mu.Lock()
			switch fault {
			case "channel":
				f.state.channelHealthy = false
			case "observers":
				f.state.invalidationsReady = false
			case "optout":
				f.state.optedIn = false
			case "consent":
				f.state.consent++
			case "last-window":
				f.windows = 0
			}
			f.mu.Unlock()
			d := b.complete(context.Background(), ticket, func() bool { t.Fatal("invalidated result callback"); return true })
			if d.OfflineValidated || d.ManualEligible {
				t.Fatal("known policy loss ignored")
			}
		})
	}
}

func TestXManualPilotOwnerExpiredBeforeAcquisition(t *testing.T) {
	cfg, f := testXManualOwnerConfig()
	cfg.pilot.attemptStarted = f.clock.Add(-nativePilotLife)
	b := testXManualBroker(t, cfg, f)
	d := b.start(context.Background())
	assertXManualOffline(t, d, "attempt-expired")
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.ops) != 0 {
		t.Fatal("expired attempt opened owner transport")
	}
}

func TestXManualPilotOwnerPrepareCleanupUncertain(t *testing.T) {
	cfg, f := testXManualOwnerConfig()
	f.fault = "prepare-cleanup-uncertain"
	b := testXManualBroker(t, cfg, f)
	d := b.start(context.Background())
	assertXManualOffline(t, d, "cleanup-uncertain")
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.ops["manual_acquire"] != 0 || f.cleanupCalls["reader"] < 2 && f.cleanupCalls["collector"] < 2 {
		t.Fatal("ambiguous preparation cleanup was not tracked and retried")
	}
}
func TestXManualPilotOwnerPrivateDecode(t *testing.T) {
	for _, raw := range []string{`{"windowCount":1,"bootstrapCount":1,"canAuthorize":false}`, `{"windowCount":1,"windowCount":1,"bootstrapCount":1}`, `{"WindowCount":1,"bootstrapCount":1}`, `{"windowCount":1,"WindowCount":1,"bootstrapCount":1}`, `{"windowCount":null,"bootstrapCount":1}`, `{"windowCount":1,"bootstrapCount":"\ud800"}`} {
		var result struct {
			WindowCount    *int `json:"windowCount"`
			BootstrapCount *int `json:"bootstrapCount"`
		}
		if xManualPilotDecode([]byte(raw), &result) == nil {
			t.Fatal("unqualified/ambiguous control reply accepted")
		}
	}
	var ready struct {
		Handle string           `json:"handle"`
		Pins   xManualPilotPins `json:"pins"`
	}
	for _, raw := range []string{`{"Handle":"private","pins":{"ownerId":"1"}}`, `{"handle":"private","pins":{"OwnerId":"1"}}`, `{"handle":"private","pins":{"ownerId":"1","OwnerId":"1"}}`} {
		if xManualPilotDecode([]byte(raw), &ready) == nil {
			t.Fatal("case-aliased private key accepted")
		}
	}
	cfg, _ := testXManualOwnerConfig()
	for _, value := range []any{cfg, &xManualPilotOwnerPorts{}, xManualPilotPins{OwnerID: "private-native-owner"}} {
		raw, err := json.Marshal(value)
		if err != nil || string(raw) != "{}" {
			t.Fatal("private owner structure serialized")
		}
		if strings.Contains(fmt.Sprintf("%#v", value), "private-native-owner") {
			t.Fatal("private native identity formatted")
		}
	}
}
