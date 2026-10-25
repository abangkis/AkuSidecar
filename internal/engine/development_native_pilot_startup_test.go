//go:build auth_session_dev

package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// The existing invented-native fixture is reused without modifying its accepted
// tests. Every capture here is explicitly offline; it echoes invented scope to
// exercise wire transport, not real startup document acquisition choreography.
func TestNativePilotStartupOwnershipCheckpoints(t *testing.T) {
	f := newTestNativePilotFixture()
	var captures atomic.Int32
	testNativeStartupSource(t, f, func(_ context.Context, ch nativePilotCaptureChallenge) ([]byte, error) {
		captures.Add(1)
		return marshalNativeStartupWire(ch)
	})
	previous := f.ports.owners
	var checks []int32
	f.ports.owners = func(ctx context.Context) (nativePilotState, error) {
		checks = append(checks, captures.Load())
		return previous(ctx)
	}
	a := f.adapter(t)
	initialNativePilotOffline(t, a)
	if len(checks) != 2 || checks[0] != 0 || checks[1] != 2 {
		t.Fatalf("expected a fresh pair check before acquisition and after both receipts, got %v", checks)
	}
}

func TestNativePilotStartupFinalOwnershipStillFencesPublication(t *testing.T) {
	for _, lateDeadline := range []bool{false, true} {
		t.Run(fmt.Sprint(lateDeadline), func(t *testing.T) {
			f := newTestNativePilotFixture()
			previous := f.ports.owners
			checks := 0
			f.ports.owners = func(ctx context.Context) (nativePilotState, error) {
				checks++
				if checks == 2 {
					if lateDeadline {
						f.advance(nativePilotStartup)
					} else {
						f.mu.Lock()
						f.state.binding.collector.document = "changed-after-both-receipts"
						f.mu.Unlock()
					}
				}
				return previous(ctx)
			}
			a := f.adapter(t)
			v, d := a.initialEvidence(context.Background())
			if v != nil || a.current != nil || d.OfflineValidated || checks != 2 {
				t.Fatal("final ownership or deadline fence bypassed")
			}
		})
	}
}

func testNativeStartupWire(c nativePilotCaptureChallenge) map[string]any {
	o := c.challenge.owner
	at := float64(c.challenge.started.UnixMilli())
	return map[string]any{"schema": nativePilotStartupWireSchema, "challenge": c.correlation, "role": c.challenge.role, "accountId": c.challenge.binding.account, "ownerId": o.owner, "profileId": o.profile, "contextId": o.context, "targetId": o.target, "frameId": o.frame, "documentId": o.document, "identityRequestId": "offline-identity-" + o.role, "settingsRequestId": "offline-settings-" + o.role, "identityObservedAtMS": at, "settingsObservedAtMS": at, "elapsedMS": 1.0, "qualification": nativePilotSettingsQualification}
}
func marshalNativeStartupWire(c nativePilotCaptureChallenge) ([]byte, error) {
	return json.Marshal(testNativeStartupWire(c))
}

func TestNativePilotStartupFailureDetails(t *testing.T) {
	for _, role := range []string{"reader", "collector"} {
		t.Run(role, func(t *testing.T) {
			f := newTestNativePilotFixture()
			testNativeStartupSource(t, f, func(_ context.Context, ch nativePilotCaptureChallenge) ([]byte, error) {
				if ch.challenge.role == role {
					return nil, nativePilotInitialRPC("manual_initial_" + role + "_settings_source_unavailable_primary")
				}
				return marshalNativeStartupWire(ch)
			})
			a := f.adapter(t)
			v, d := a.initialEvidence(context.Background())
			if v != nil || d.Reason != "source-unavailable" || d.ManualEligible || d.OfflineValidated || a.initial.role != role || a.initial.phase != "settings" {
				t.Fatal("initial role detail lost or authority changed")
			}
		})
	}
	for _, fault := range []string{"scope_mismatch", "time_invalid", "invalid_shape"} {
		t.Run(fault, func(t *testing.T) {
			f := newTestNativePilotFixture()
			testNativeStartupSource(t, f, func(_ context.Context, ch nativePilotCaptureChallenge) ([]byte, error) {
				if ch.challenge.role == "reader" {
					return marshalNativeStartupWire(ch)
				}
				m := testNativeStartupWire(ch)
				switch fault {
				case "scope_mismatch":
					m["documentId"] = "wrong"
				case "time_invalid":
					m["elapsedMS"] = 6000
				default:
					return []byte(`{"private":"invalid"}`), nil
				}
				return json.Marshal(m)
			})
			a := f.adapter(t)
			v, _ := a.initialEvidence(context.Background())
			if v != nil || a.initial.role != "collector" || a.initial.phase != "wire_decode" || a.initial.reason != fault {
				t.Fatal("closed wire failure category lost")
			}
		})
	}
	for _, deadline := range []bool{false, true} {
		t.Run(fmt.Sprint(deadline), func(t *testing.T) {
			f := newTestNativePilotFixture()
			entered := make(chan struct{}, 2)
			testNativeStartupSource(t, f, func(ctx context.Context, _ nativePilotCaptureChallenge) ([]byte, error) {
				entered <- struct{}{}
				<-ctx.Done()
				return nil, ctx.Err()
			})
			ctx, cancel := context.WithCancel(context.Background())
			if deadline {
				ctx, cancel = context.WithTimeout(context.Background(), 20*time.Millisecond)
			}
			defer cancel()
			a := f.adapter(t)
			done := make(chan struct{})
			go func() { a.initialEvidence(ctx); close(done) }()
			<-entered
			<-entered
			if !deadline {
				cancel()
			}
			<-done
			if a.initial.role != "pair" || a.initial.phase != "joint" {
				t.Fatal("unattributed cancellation blamed a role")
			}
		})
	}
}

func TestNativePilotStartupDiagnosticClosed(t *testing.T) {
	var typedNil *nativePilotInitialError
	if d := nativePilotInitialOf(typedNil, "reader", "identity"); d.role != "unknown" {
		t.Fatal("typed nil diagnostic was attributed")
	}
	invalid := &nativePilotInitialError{nativePilotInitialDetail{role: "PRIVATE_ID", phase: "PRIVATE_BODY", reason: "PRIVATE_COOKIE"}}
	for _, value := range []any{invalid, invalid.detail, nativePilotInitialOf(invalid, "reader", "identity"), nativePilotInitialOf(nil, "PRIVATE_ID", "PRIVATE_BODY")} {
		if strings.Contains(fmt.Sprintf("%v %#v", value, value), "PRIVATE") {
			t.Fatal("malformed typed diagnostic leaked")
		}
	}
	for _, role := range nativePilotInitialRoles {
		for _, phase := range nativePilotInitialPhases {
			for _, reason := range nativePilotInitialReasons {
				for _, kind := range []string{"primary", "peer"} {
					code := "manual_initial_" + role + "_" + phase + "_" + reason + "_" + kind
					d, ok := nativePilotInitialCode(code)
					if !ok || len(code) > 80 || d.role != role || d.phase != phase || d.reason != reason || d.peer != (kind == "peer") {
						t.Fatal("bounded diagnostic parity failed")
					}
				}
			}
		}
	}
	if d := nativePilotInitialOf(nativePilotInitialRPC("xnp_PRIVATE"), "reader", "identity"); d.role != "unknown" {
		t.Fatal("untrusted error pattern promoted")
	}
}

func TestNativePilotStartupCanceledGuard(t *testing.T) {
	s := &nativePilotStartupSource{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := s.observe(ctx, nativePilotChallenge{})
	if d := nativePilotInitialOf(err, "unknown", "acquire_rpc"); d.role != "pair" || d.phase != "joint" || d.reason != "canceled" {
		t.Fatal("canceled entry lost joint attribution")
	}
	s.initial = nativePilotInitialDetail{role: "collector", phase: "settings", reason: "source_unavailable"}
	s.mu.Lock()
	err = s.canceledLocked(context.DeadlineExceeded)
	s.mu.Unlock()
	if d := nativePilotInitialOf(err, "unknown", "acquire_rpc"); d.role != "collector" || d.phase != "settings" {
		t.Fatal("post-output deadline replaced observed primary")
	}
}

func TestNativePilotStartupTimingFrozenFirstCause(t *testing.T) {
	f := newTestNativePilotFixture()
	entered := make(chan struct{}, 2)
	left := make(chan struct{}, 2)
	testNativeStartupSource(t, f, func(ctx context.Context, ch nativePilotCaptureChallenge) ([]byte, error) {
		defer func() { left <- struct{}{} }()
		done := nativePilotTimed(ctx, ch.challenge.role, "acquire_rpc")
		defer done()
		if ch.challenge.role == "reader" {
			nativePilotTimingProgress(ctx, "reader", []byte(`{"operation":"settings_wait","elapsedMS":12,"cause":"manual_initial_reader_settings_source_unavailable_primary"}`))
		} else {
			nativePilotTimingProgress(ctx, "collector", []byte(`{"operation":"rendezvous","elapsedMS":3,"cause":""}`))
		}
		entered <- struct{}{}
		<-ctx.Done()
		return nil, ctx.Err()
	})
	a := f.adapter(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { a.initialEvidence(ctx); close(done) }()
	<-entered
	<-entered
	cancel()
	<-done
	<-left
	<-left
	d := a.initial
	if d.role != "reader" || d.phase != "settings" || d.reason != "source_unavailable" || d.timing == nil {
		t.Fatalf("lost first closed failure: %v", d)
	}
	if d.timing.roles[0].operation != nativePilotTimingIndex("acquire_rpc") || d.timing.roles[1].nodeOperation != nativePilotTimingIndex("rendezvous") {
		t.Fatalf("lost pending roles: %v", d)
	}
}

func TestNativePilotStartupTimingBoundsAndIsolation(t *testing.T) {
	now := time.Unix(100, 0)
	ledger := &nativePilotTiming{started: now, now: func() time.Time { return now }}
	ctx := context.WithValue(context.Background(), nativePilotTimingKey{}, ledger)
	readerDone := nativePilotTimed(ctx, "reader", "inspect_rpc")
	now = now.Add(20 * time.Millisecond)
	readerDone()
	collectorDone := nativePilotTimed(ctx, "collector", "acquire_rpc")
	for _, raw := range []string{
		`{"operation":"PRIVATE","elapsedMS":0,"cause":""}`,
		`{"operation":"identity","operation":"cleanup","elapsedMS":0,"cause":""}`,
		`{"operation":"identity","elapsedMS":-1,"cause":""}`,
		`{"operation":"identity","elapsedMS":60001,"cause":""}`,
		`{"operation":"identity","elapsedMS":null,"cause":""}`,
		`{"operation":"identity","elapsedMS":0,"cause":"","secret":"PRIVATE"}`,
	} {
		nativePilotTimingProgress(ctx, "collector", []byte(raw))
	}
	if ledger.roles[1].nodeOp != 0 {
		t.Fatal("malformed diagnostic accepted")
	}
	nativePilotTimingProgress(ctx, "collector", []byte(`{"operation":"settings_wait","elapsedMS":7,"cause":"manual_initial_reader_identity_unavailable_primary"}`))
	if ledger.first != nil {
		t.Fatal("cross-role cause accepted")
	}
	nativePilotTimingProgress(ctx, "collector", []byte(`{"operation":"settings_wait","elapsedMS":7,"cause":"manual_initial_reader_identity_unavailable_peer"}`))
	if ledger.first == nil || ledger.first.role != "reader" || !ledger.first.peer {
		t.Fatal("branded peer-first origin lost")
	}
	now = now.Add(30 * time.Millisecond)
	snapshot := *ledger.freeze()
	collectorDone()
	nativePilotTimingProgress(ctx, "collector", []byte(`{"operation":"cleanup","elapsedMS":2,"cause":""}`))
	now = now.Add(time.Minute)
	if *ledger.freeze() != snapshot {
		t.Fatal("frozen timeout mutated")
	}
	if snapshot.roles[0].totals[nativePilotTimingIndex("inspect_rpc")] != 20 || snapshot.roles[1].activeMS != 30 {
		t.Fatal("role durations mixed")
	}
	if strings.Contains(snapshot.String(), "PRIVATE") {
		t.Fatal("diagnostic leaked data")
	}
}

func TestNativePilotStartupSettingsProgressClosed(t *testing.T) {
	if !strings.Contains((nativePilotInitialTrace{firstCause: "settings_capture"}).String(), "domain=settings") {
		t.Fatal("settings terminal failure escaped its diagnostic domain")
	}
	now := time.Unix(100, 0)
	ledger := &nativePilotTiming{started: now, now: func() time.Time { return now }}
	ctx := context.WithValue(context.Background(), nativePilotTimingKey{}, ledger)
	reader := nativePilotTimed(ctx, "reader", "acquire_rpc")
	defer reader()
	collector := nativePilotTimed(ctx, "collector", "acquire_rpc")
	defer collector()
	for index, state := range nativePilotSettingsWaitStates {
		raw, _ := json.Marshal(map[string]any{"operation": "settings_wait", "elapsedMS": 10, "cause": "", "settingsWait": state})
		nativePilotTimingProgress(ctx, "collector", raw)
		if ledger.roles[1].settingsWait != index || ledger.roles[0].settingsWait != 0 {
			t.Fatal("closed settings state lost or crossed roles")
		}
	}
	nativePilotTimingProgress(ctx, "collector", []byte(`{"operation":"settings_wait","elapsedMS":15,"cause":"","settingsWait":"extra_pending"}`))
	for _, invalid := range []string{
		`{"operation":"settings_wait","elapsedMS":1,"cause":"","settingsWait":"PRIVATE_TOKEN"}`,
		`{"operation":"settings_wait","elapsedMS":1,"cause":"","settingsWait":null}`,
		`{"operation":"settings_wait","elapsedMS":1,"cause":"","settingsWait":4}`,
		`{"operation":"settings_wait","elapsedMS":1,"cause":"","settingsWait":"body_pending","settingsWait":"body_complete"}`,
		`{"operation":"settings_wait","elapsedMS":1,"settingsWait":"body_complete"}`,
	} {
		nativePilotTimingProgress(ctx, "collector", []byte(invalid))
	}
	snapshot := *ledger.freeze()
	if !strings.Contains(snapshot.String(), "collector_settings_wait=extra_pending") || strings.Contains(snapshot.String(), "PRIVATE") {
		t.Fatal("settings prerequisite was overwritten or leaked data")
	}
	nativePilotTimingProgress(ctx, "collector", []byte(`{"operation":"settings_wait","elapsedMS":2,"cause":"","settingsWait":"body_complete"}`))
	if *ledger.freeze() != snapshot {
		t.Fatal("late settings progress rewrote the frozen timeout")
	}
}

func testNativeStartupSource(t *testing.T, f *testNativePilotFixture, capture nativePilotCapture) *nativePilotStartupSource {
	t.Helper()
	if capture == nil {
		capture = func(_ context.Context, c nativePilotCaptureChallenge) ([]byte, error) {
			return marshalNativeStartupWire(c)
		}
	}
	s, err := newNativePilotStartupSource(nativePilotStartupConfig{enabled: true, offlineOnly: true, binding: f.cfg.binding, capture: capture})
	if err != nil {
		t.Fatal("offline startup source unavailable")
	}
	f.ports.source = s
	return s
}

func TestNativePilotStartupConcurrentPair(t *testing.T) {
	f := newTestNativePilotFixture()
	entered, release := make(chan string, 2), make(chan struct{})
	var calls atomic.Int32
	s := testNativeStartupSource(t, f, func(ctx context.Context, c nativePilotCaptureChallenge) ([]byte, error) {
		calls.Add(1)
		raw, err := c.requestJSON()
		var fields map[string]string
		if err != nil || json.Unmarshal(raw, &fields) != nil || len(fields) != 8 || fields["documentId"] != c.challenge.owner.document {
			return nil, errors.New("offline request malformed")
		}
		entered <- c.challenge.role
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-release:
		}
		return marshalNativeStartupWire(c)
	})
	a := f.adapter(t)
	type outcome struct {
		v *nativePilotVector
		d nativePilotDiagnostic
	}
	done := make(chan outcome, 1)
	go func() { v, d := a.initialEvidence(context.Background()); done <- outcome{v, d} }()
	roles := map[string]bool{}
	for i := 0; i < 2; i++ {
		select {
		case role := <-entered:
			roles[role] = true
		case <-time.After(time.Second):
			close(release)
			t.Fatal("roles were not captured concurrently")
		}
	}
	close(release)
	out := <-done
	assertNativePilotClosed(t, out.d, "startup-validated")
	if out.v == nil || !out.d.OfflineValidated || calls.Load() != 2 || !roles["reader"] || !roles["collector"] {
		t.Fatal("joint offline vector missing")
	}
	_, err := s.observe(context.Background(), s.collector)
	if err == nil || calls.Load() != 2 {
		t.Fatal("collector receipt replayed or recaptured")
	}
	for _, value := range []any{s, s.cfg, nativePilotCaptureChallenge{challenge: s.reader, correlation: "private-correlation"}} {
		if strings.Contains(fmt.Sprintf("%#v", value), "private-correlation") || strings.Contains(fmt.Sprintf("%+v", value), f.cfg.binding.account) {
			t.Fatal("private source formatting leaked")
		}
		raw, err := json.Marshal(value)
		if err != nil || string(raw) != "{}" {
			t.Fatal("private source serialized")
		}
	}
}

func TestNativePilotStartupStrictDecoder(t *testing.T) {
	f := newTestNativePilotFixture()
	ch := nativePilotChallenge{seal: &nativePilotSeal{}, vector: 1, role: "reader", binding: f.cfg.binding, owner: f.cfg.binding.reader, started: f.now}
	c := nativePilotCaptureChallenge{challenge: ch, correlation: strings.Repeat("a", 48)}
	valid, _ := marshalNativeStartupWire(c)
	if _, err := decodeNativePilotStartup(valid, ch, c.correlation); err != nil {
		t.Fatal("valid offline wire rejected")
	}
	cases := map[string]func(map[string]any){
		"unknown":             func(m map[string]any) { m["canAuthorize"] = false },
		"missing":             func(m map[string]any) { delete(m, "frameId") },
		"null":                func(m map[string]any) { m["accountId"] = nil },
		"number-id":           func(m map[string]any) { m["accountId"] = 1908123456789012345 },
		"handle":              func(m map[string]any) { m["accountId"] = "@user" },
		"synthetic-id":        func(m map[string]any) { m["accountId"] = "synthetic:account" },
		"wrong-schema":        func(m map[string]any) { m["schema"] = "aku.x.qa-bound-observation.v1" },
		"wrong-challenge":     func(m map[string]any) { m["challenge"] = "old-challenge" },
		"wrong-role":          func(m map[string]any) { m["role"] = "collector" },
		"wrong-owner":         func(m map[string]any) { m["ownerId"] = "different-owner" },
		"wrong-context":       func(m map[string]any) { m["contextId"] = "different-context" },
		"new-document":        func(m map[string]any) { m["documentId"] = "new-navigation-document" },
		"wrong-qualification": func(m map[string]any) { m["qualification"] = "http200" },
		"duplicate-request":   func(m map[string]any) { m["settingsRequestId"] = m["identityRequestId"] },
		"long-request":        func(m map[string]any) { m["settingsRequestId"] = strings.Repeat("r", 513) },
		"null-time":           func(m map[string]any) { m["identityObservedAtMS"] = nil },
		"negative-elapsed":    func(m map[string]any) { m["elapsedMS"] = -1 },
		"joint-limit":         func(m map[string]any) { m["elapsedMS"] = 6000 },
		"overflow-time":       func(m map[string]any) { m["identityObservedAtMS"] = 9e18 },
		"stale-bootstrap": func(m map[string]any) {
			m["identityObservedAtMS"] = float64(ch.started.Add(-time.Millisecond).UnixMilli())
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			m := testNativeStartupWire(c)
			mutate(m)
			raw, _ := json.Marshal(m)
			if _, err := decodeNativePilotStartup(raw, ch, c.correlation); err == nil {
				t.Fatal("invalid private wire accepted")
			}
		})
	}
	for name, raw := range map[string][]byte{
		"duplicate":         append(append([]byte{}, valid[:len(valid)-1]...), []byte(`,"role":"reader"}`)...),
		"escaped-duplicate": append(append([]byte{}, valid[:len(valid)-1]...), []byte(`,"\u0072ole":"reader"}`)...),
		"invalid-surrogate": bytes.Replace(valid, []byte(`"role":"reader"`), []byte(`"role":"\ud800"`), 1),
		"invalid-utf8":      append([]byte{'{', '"', 255, '"', ':', '0', '}'}, ' '),
		"trailing":          append(append([]byte{}, valid...), []byte(`{}`)...),
		"too-large":         bytes.Repeat([]byte(" "), nativePilotStartupWireMax+1),
		"public-qa-report":  []byte(`{"canAuthorize":false,"identity":"current_self_observed","login":"protected_response_observed"}`),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeNativePilotStartup(raw, ch, c.correlation); err == nil {
				t.Fatal("malformed/private-report substitute accepted")
			}
		})
	}
	if !nativePilotStartupUnicode([]byte(`"\ud83d\ude00"`)) || nativePilotStartupUnicode([]byte(`"\ud83d"`)) {
		t.Fatal("strict unicode validation regressed")
	}
}

func TestNativePilotStartupPairFailureAndReplay(t *testing.T) {
	for _, kind := range []string{"projection-failure", "account-mismatch", "owner-drift", "joint-timeout", "canceled"} {
		t.Run(kind, func(t *testing.T) {
			f := newTestNativePilotFixture()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var captures atomic.Int32
			testNativeStartupSource(t, f, func(_ context.Context, c nativePilotCaptureChallenge) ([]byte, error) {
				captures.Add(1)
				if c.challenge.role == "collector" {
					switch kind {
					case "projection-failure":
						return nil, errors.New("raw-private-secret")
					case "account-mismatch":
						m := testNativeStartupWire(c)
						m["accountId"] = "1908123456789012346"
						return json.Marshal(m)
					case "owner-drift":
						f.mu.Lock()
						f.state.binding.collector.creation = "replacement-process"
						f.mu.Unlock()
					case "joint-timeout":
						f.advance(nativePilotStartup)
					case "canceled":
						cancel()
					}
				}
				return marshalNativeStartupWire(c)
			})
			a := f.adapter(t)
			v, d := a.initialEvidence(ctx)
			if v != nil || d.OfflineValidated || d.ManualEligible || strings.Contains(d.Reason, "secret") {
				t.Fatal("failed pair published")
			}
			_, d = a.initialEvidence(context.Background())
			if !a.stopped || d.OfflineValidated || captures.Load() > 2 {
				t.Fatal("failed startup replayed")
			}
		})
	}
}
