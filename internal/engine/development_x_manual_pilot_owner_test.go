//go:build auth_session_dev

package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// All ownership values below are invented. This compiled backend can produce
// only offline vectors, including when a private JSON object claims otherwise.
func TestXManualPilotOwnerParentPairSingleSnapshot(t *testing.T) {
	for _, fault := range []string{"none", "node-missing", "chrome-missing", "snapshot-failure"} {
		t.Run(fault, func(t *testing.T) {
			walks := 0
			walk := func(visit func(uint32, uint32) bool) error {
				walks++
				// Chrome may occur before Node in the same enumeration.
				for _, pair := range [][2]uint32{{30, 20}, {99, 90}, {20, 10}} {
					if fault == "node-missing" && pair[0] == 20 || fault == "chrome-missing" && pair[0] == 30 {
						continue
					}
					if fault == "snapshot-failure" {
						return errors.New("private-snapshot-error")
					}
					if !visit(pair[0], pair[1]) {
						break
					}
				}
				return nil
			}
			node, chrome, err := xManualPilotParentPair(20, 30, walk)
			if walks != 1 || (err == nil) != (fault == "none") || err == nil && (node != 10 || chrome != 20) {
				t.Fatal("single-snapshot parent links were missing or fabricated")
			}
		})
	}
	var generation uint32 = 10
	walk := func(visit func(uint32, uint32) bool) error { visit(20, generation); visit(30, 20); return nil }
	before, _, _ := xManualPilotParentPair(20, 30, walk)
	generation = 11
	after, _, _ := xManualPilotParentPair(20, 30, walk)
	if before == after {
		t.Fatal("parent snapshot was cached across checkpoints")
	}
}

func TestXManualPilotOwnerStartupBudget(t *testing.T) {
	if ms, err := xManualPilotStartupBudget(context.Background()); err != nil || ms != nativePilotStartup.Milliseconds() {
		t.Fatal("missing parent deadline enlarged the joint limit")
	}
	for _, limit := range []time.Duration{150 * time.Millisecond, 5 * time.Second, 9 * time.Second} {
		ctx, cancel := context.WithTimeout(context.Background(), limit)
		ms, err := xManualPilotStartupBudget(ctx)
		cancel()
		if err != nil || ms <= 0 || ms > limit.Milliseconds() || ms > nativePilotStartup.Milliseconds() {
			t.Fatal("private wait hint exceeded the shared remaining deadline")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := xManualPilotStartupBudget(ctx); err == nil {
		t.Fatal("canceled startup received a new wait budget")
	}
	expired, cancelExpired := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancelExpired()
	if _, err := xManualPilotStartupBudget(expired); err == nil {
		t.Fatal("expired startup received a new wait budget")
	}
}

func TestXManualPilotObservationDeadlineIsSharedAndCapped(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond)
	pilot := nativePilotConfig{attemptStarted: now.Add(-5 * time.Second)}
	first, err := xManualPilotObservationDeadline(context.Background(), now, pilot)
	if err != nil || first != pilot.attemptStarted.Add(nativePilotLife).UnixMilli() {
		t.Fatalf("attempt deadline not preserved: %d %v", first, err)
	}
	later, err := xManualPilotObservationDeadline(context.Background(), now.Add(15*time.Second), pilot)
	if err != nil || later != first {
		t.Fatal("a later operation extended the held observation deadline")
	}
	ctx, cancel := context.WithDeadline(context.Background(), now.Add(3*time.Second))
	defer cancel()
	bounded, err := xManualPilotObservationDeadline(ctx, now, pilot)
	if err != nil || bounded != now.Add(3*time.Second).UnixMilli() {
		t.Fatal("parent deadline was not carried to PREPARE")
	}
	pilot.attemptValidUntil = now.Add(time.Second)
	bounded, err = xManualPilotObservationDeadline(ctx, now, pilot)
	if err != nil || bounded != pilot.attemptValidUntil.UnixMilli() {
		t.Fatal("shorter attempt validity was enlarged")
	}
	for _, sample := range []struct {
		ctx context.Context
		now time.Time
		cfg nativePilotConfig
	}{
		{nil, now, pilot},
		{context.Background(), time.Time{}, pilot},
		{context.Background(), now.Add(2 * time.Second), pilot},
		{context.Background(), now, nativePilotConfig{attemptStarted: now.Add(time.Second)}},
	} {
		if _, err := xManualPilotObservationDeadline(sample.ctx, sample.now, sample.cfg); err == nil {
			t.Fatal("invalid or expired PREPARE received a new deadline")
		}
	}
}

func TestXManualPilotV10RelativeAgesAreBoundedAndLegacyCodesRemainValid(t *testing.T) {
	parts := make([]string, 28)
	for i := range parts {
		parts[i] = "0"
	}
	parts[1] = "8" // bound_finish
	parts[2] = "5" // stale_observation
	parts[4] = strconv.Itoa(slices.Index(nativePilotTraceCatalogs[0], "stale_observation"))
	parts[23] = "4" // observation_expired
	copy(parts[24:], []string{"11201", "101", "11001", "101"})
	code := "manual_initial_v10_" + strings.Join(parts, "_")
	d, ok := nativePilotInitialCode(code)
	if !ok || d.trace == nil || d.trace.deadlineCondition != "observation_expired" || d.trace.ages == nil || d.trace.ages.observation != 11200 || d.trace.ages.prepare != 100 || d.trace.ages.paused != 11000 || d.trace.ages.acquire != 100 {
		t.Fatal("phase ages were lost or treated as enum indexes")
	}
	if !strings.Contains(d.String(), "observation_age_ms=11200 prepare_age_ms=100 paused_age_ms=11000 acquire_age_ms=100") {
		t.Fatal("safe relative ages absent from diagnostics")
	}
	for _, bad := range []string{"-1", "60002", "00", "1.5", "PRIVATE_COOKIE"} {
		copyParts := append([]string(nil), parts...)
		copyParts[24] = bad
		if _, ok := nativePilotInitialCode("manual_initial_v10_" + strings.Join(copyParts, "_")); ok {
			t.Fatalf("invalid age token accepted: %q", bad)
		}
	}
	for _, n := range []int{27, 29} {
		p := append(append([]string(nil), parts...), "0")[:n]
		if _, ok := nativePilotInitialCode("manual_initial_v10_" + strings.Join(p, "_")); ok {
			t.Fatal("wrong diagnostic arity accepted")
		}
	}
	legacy, ok := nativePilotInitialCode("manual_initial_v9_" + strings.Join(parts[:23], "_"))
	if !ok || legacy.trace.ages != nil || legacy.trace.deadlineCondition != "unknown" {
		t.Fatal("legacy trace unexpectedly acquired observation ages")
	}
}

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
	startupBudgets  map[string]int64
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
		if role == "collector" && (f.fault == "inspect-before" && !f.acquired[role] || f.fault == "inspect-after" && f.acquired[role]) {
			f.mu.Unlock()
			return errors.New("PRIVATE_UNTRUSTED_INSPECT")
		}
		count, bootstrap := f.windows, 1
		if role == "collector" {
			count, bootstrap = 0, 0
		}
		value := map[string]any{"pins": pins, "windowCount": count, "bootstrapCount": bootstrap, "paused": !f.acquired[role], "invalid": false, "headless": role == "collector"}
		f.mu.Unlock()
		return testXManualReply(out, value)
	case "manual_acquire":
		budget, ok := fields["startupBudgetMS"].(int64)
		if !ok || budget < 1 || budget > nativePilotStartup.Milliseconds() {
			f.mu.Unlock()
			return errors.New("private-startup-budget-invalid")
		}
		if f.startupBudgets == nil {
			f.startupBudgets = make(map[string]int64)
		}
		f.startupBudgets[role] = budget
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

func TestXManualPilotOwnerInspectDiagnostic(t *testing.T) {
	for _, phase := range []string{"inspect_before", "inspect_after"} {
		t.Run(phase, func(t *testing.T) {
			cfg, f := testXManualOwnerConfig()
			p := &xManualPilotOwnerPorts{cfg: cfg, backend: f, prepared: map[string]nativePilotPreparedRole{}, begun: map[string]bool{}, acquired: map[string]bool{}, cleanupTokens: map[string]string{}, cleanupUnknown: map[string]bool{}}
			prepared := map[string]nativePilotPreparedRole{}
			for _, role := range []string{"reader", "collector"} {
				v, err := p.prepare(context.Background(), nativePilotPrepareRequest{role: role, profile: f.core[role].profile, parent: f.core[role].parent, attempt: cfg.pilot.binding.attempt})
				if err != nil {
					t.Fatal("offline preparation failed")
				}
				prepared[role] = v
			}
			f.acquired["reader"] = true
			if phase == "inspect_before" {
				f.fault = "inspect-before"
			} else {
				f.fault = "inspect-after"
			}
			binding := cfg.pilot.binding
			binding.reader, binding.collector = prepared["reader"].owner, prepared["collector"].owner
			_, err := p.acquire(context.Background(), prepared["collector"], nativePilotCaptureChallenge{challenge: nativePilotChallenge{seal: &nativePilotSeal{}, vector: 1, role: "collector", binding: binding, owner: binding.collector, started: f.now()}, correlation: strings.Repeat("c", 48)})
			d := nativePilotInitialOf(err, "unknown", "acquire_rpc")
			if err == nil || d.role != "collector" || d.phase != phase || strings.Contains(d.String(), "PRIVATE") {
				t.Fatal("owner inspection stage lost")
			}
		})
	}
}

// This backend wraps invented ownership only; hooks isolate inspection failure
// predicates without opening a browser, touching a profile or admitting work.
type testXManualInspectBackend struct {
	*testXManualOwnerBackend
	snapshotHook func(context.Context, string) (nativePilotOwner, error)
	rpcHook      func(context.Context, string, string, map[string]any, any) error
}

func (f *testXManualInspectBackend) snapshot(ctx context.Context, role string) (nativePilotOwner, error) {
	return f.snapshotHook(ctx, role)
}
func (f *testXManualInspectBackend) rpc(ctx context.Context, role, op string, fields map[string]any, out any) error {
	return f.rpcHook(ctx, role, op, fields, out)
}

func TestXManualPilotOwnerInspectPredicates(t *testing.T) {
	tests := []struct {
		name, role, stage, condition string
		field                        string
		value                        any
	}{
		{name: "stored", stage: "go_stored", condition: "handle_mismatch"},
		{name: "core-before", stage: "go_core_before", condition: "os_snapshot"},
		{name: "rpc", stage: "go_rpc", condition: "command_failed"},
		{name: "duplicate", stage: "go_decode", condition: "reply_invalid"},
		{name: "unknown-field", stage: "go_decode", condition: "reply_invalid", field: "PRIVATE_BODY", value: "PRIVATE_VALUE"},
		{name: "null", stage: "go_decode", condition: "reply_invalid", field: "paused", value: nil},
		{name: "case-alias", stage: "go_decode", condition: "reply_invalid", field: "Paused", value: true},
		{name: "wrong-type", stage: "go_decode", condition: "reply_invalid", field: "windowCount", value: "PRIVATE_VALUE"},
		{name: "invalid-missing", stage: "go_validate", condition: "invalid_flag", field: "invalid"},
		{name: "invalid", stage: "go_validate", condition: "invalid_flag", field: "invalid", value: true},
		{name: "headless-missing", stage: "go_validate", condition: "headless_mismatch", field: "headless"},
		{name: "headless", stage: "go_validate", condition: "headless_mismatch", field: "headless", value: true},
		{name: "paused-missing", stage: "go_validate", condition: "paused_missing", field: "paused"},
		{name: "pause-required", stage: "go_validate", condition: "pause_required", field: "paused", value: false},
		{name: "windows-missing", stage: "go_validate", condition: "inventory_missing", field: "windowCount"},
		{name: "bootstrap-missing", stage: "go_validate", condition: "inventory_missing", field: "bootstrapCount"},
		{name: "windows-negative", stage: "go_validate", condition: "window_count", field: "windowCount", value: -1},
		{name: "windows-over-cap", stage: "go_validate", condition: "window_count", field: "windowCount", value: 17},
		{name: "bootstrap-negative", stage: "go_validate", condition: "bootstrap_count", field: "bootstrapCount", value: -1},
		{name: "bootstrap-over-cap", stage: "go_validate", condition: "bootstrap_count", field: "bootstrapCount", value: 17},
		{name: "reader-windows", stage: "go_validate", condition: "reader_windows", field: "windowCount", value: 0},
		{name: "reader-bootstrap", stage: "go_validate", condition: "reader_bootstrap", field: "bootstrapCount", value: 0},
		{name: "collector-windows", role: "collector", stage: "go_validate", condition: "collector_windows", field: "windowCount", value: 1},
		{name: "collector-bootstrap", role: "collector", stage: "go_validate", condition: "bootstrap_count", field: "bootstrapCount", value: 1},
		{name: "bind", stage: "go_bind", condition: "pins_mismatch"},
		{name: "core-after-error", stage: "go_core_after", condition: "os_snapshot"},
		{name: "core-after-change", stage: "go_core_after", condition: "os_changed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg, f := testXManualOwnerConfig()
			role := test.role
			if role == "" {
				role = "reader"
			}
			p := &xManualPilotOwnerPorts{cfg: cfg, backend: f, prepared: map[string]nativePilotPreparedRole{}, begun: map[string]bool{}, acquired: map[string]bool{}, cleanupTokens: map[string]string{}, cleanupUnknown: map[string]bool{}}
			prepared, err := p.prepare(context.Background(), nativePilotPrepareRequest{role: role, profile: f.core[role].profile, parent: f.core[role].parent, attempt: cfg.pilot.binding.attempt})
			if err != nil {
				t.Fatal("offline preparation failed")
			}
			if test.name == "stored" {
				prepared.handle = "PRIVATE_WRONG_HANDLE"
			}
			calls := 0
			p.backend = &testXManualInspectBackend{testXManualOwnerBackend: f, snapshotHook: func(ctx context.Context, role string) (nativePilotOwner, error) {
				calls++
				if test.name == "core-before" || test.name == "core-after-error" && calls == 2 {
					return nativePilotOwner{}, errors.New("PRIVATE_OS_ERROR")
				}
				o, err := f.snapshot(ctx, role)
				if test.name == "core-after-change" && calls == 2 {
					o.creation = "PRIVATE_CHANGED_CREATION"
				}
				return o, err
			}, rpcHook: func(ctx context.Context, role, op string, fields map[string]any, out any) error {
				if test.name == "rpc" {
					return errors.New("PRIVATE_RPC_ERROR")
				}
				if test.name == "duplicate" {
					*out.(*json.RawMessage) = json.RawMessage(`{"invalid":false,"invalid":false}`)
					return nil
				}
				if err := f.rpc(ctx, role, op, fields, out); err != nil {
					return err
				}
				var reply map[string]any
				if json.Unmarshal(*out.(*json.RawMessage), &reply) != nil {
					t.Fatal("invented reply unavailable")
				}
				if test.field != "" {
					if test.value == nil && test.name != "null" {
						delete(reply, test.field)
					} else {
						reply[test.field] = test.value
					}
				}
				if test.name == "bind" {
					reply["pins"].(map[string]any)["documentId"] = "PRIVATE_WRONG_DOCUMENT"
				}
				return testXManualReply(out, reply)
			}}
			owner, err := p.inspectPhase(context.Background(), prepared, true)
			d := nativePilotInitialOf(err, "unknown", "acquire_rpc")
			if err == nil || owner != (nativePilotOwner{}) || d.role != role || d.phase != "inspect_before" || d.trace == nil || d.trace.firstCause != "inspection" || d.trace.inspectStage != test.stage || d.trace.inspectCondition != test.condition || strings.Contains(fmt.Sprintf("%v %#v %v", d, d, err), "PRIVATE") {
				t.Fatalf("inspection predicate lost or leaked: %s", d)
			}
			if strings.Contains(d.String(), "domain=inspection") == false {
				t.Fatal("inspection domain missing")
			}
		})
	}
}

func TestXManualPilotOwnerInspectPreservesNodeCause(t *testing.T) {
	cfg, f := testXManualOwnerConfig()
	p := &xManualPilotOwnerPorts{cfg: cfg, backend: f, prepared: map[string]nativePilotPreparedRole{}, begun: map[string]bool{}, acquired: map[string]bool{}, cleanupTokens: map[string]string{}, cleanupUnknown: map[string]bool{}}
	prepared, err := p.prepare(context.Background(), nativePilotPrepareRequest{role: "reader", profile: f.core["reader"].profile, parent: f.core["reader"].parent, attempt: cfg.pilot.binding.attempt})
	if err != nil {
		t.Fatal("offline preparation failed")
	}
	for _, phase := range []string{"inspect_before", "inspect_after"} {
		trace := nativePilotInitialTrace{firstCause: "inspection", inspectStage: "snapshot_runtime", inspectCondition: "command_failed"}
		origin := (nativePilotInitialDetail{role: "collector", phase: phase, reason: "scope_lost", peer: true, trace: &trace}).closed()
		p.backend = &testXManualInspectBackend{testXManualOwnerBackend: f, snapshotHook: f.snapshot, rpcHook: func(context.Context, string, string, map[string]any, any) error {
			return fmt.Errorf("PRIVATE_TRANSPORT: %w", &nativePilotInitialError{origin})
		}}
		_, err := p.inspectPhase(context.Background(), prepared, true)
		if got := nativePilotInitialOf(err, "unknown", "acquire_rpc"); got.String() != origin.String() {
			t.Fatalf("Node originating diagnostic replaced: %s", got)
		}
	}
}

func TestXManualPilotOwnerInspectPauseAndPhase(t *testing.T) {
	cfg, f := testXManualOwnerConfig()
	p := &xManualPilotOwnerPorts{cfg: cfg, backend: f, prepared: map[string]nativePilotPreparedRole{}, begun: map[string]bool{}, acquired: map[string]bool{}, cleanupTokens: map[string]string{}, cleanupUnknown: map[string]bool{}}
	prepared, err := p.prepare(context.Background(), nativePilotPrepareRequest{role: "reader", profile: f.core["reader"].profile, parent: f.core["reader"].parent, attempt: cfg.pilot.binding.attempt})
	if err != nil {
		t.Fatal("offline preparation failed")
	}
	f.acquired["reader"], p.acquired["reader"] = true, true
	if owner, err := p.inspect(context.Background(), prepared); err != nil || owner != prepared.owner {
		t.Fatal("post-acquire inspection unexpectedly requires pause")
	}
	if _, err := p.inspectPhase(context.Background(), prepared, true); err == nil || nativePilotInitialOf(err, "unknown", "acquire_rpc").phase != "inspect_before" {
		t.Fatal("pre-acquire inspection pause predicate weakened")
	}
	f.windows = 0
	_, err = p.inspect(context.Background(), prepared)
	if d := nativePilotInitialOf(err, "unknown", "acquire_rpc"); err == nil || d.phase != "inspect_after" || d.trace.inspectCondition != "reader_windows" {
		t.Fatal("post-acquire local phase lost")
	}
}

func TestXManualPilotOwnerInspectBrokerPreservesEarlierCause(t *testing.T) {
	trace := nativePilotInitialTrace{firstCause: "inspection", inspectStage: "go_bind", inspectCondition: "pins_mismatch"}
	origin := (nativePilotInitialDetail{role: "reader", phase: "inspect_before", reason: "unavailable", trace: &trace}).closed()
	b := &nativePilotBroker{initial: origin, controller: &nativePilotManualController{adapter: &nativePilotAdapter{initial: nativePilotInitialDetail{role: "pair", phase: "joint", reason: "canceled"}}}}
	if got := b.initialDiagnostic(); got.String() != origin.String() {
		t.Fatalf("later source cancellation replaced owner predicate: %s", got)
	}
	b.initial = nativePilotInitialDetail{}
	b.controller.adapter.initial = origin
	if got := b.initialDiagnostic(); got.String() != origin.String() {
		t.Fatal("source origin lost without prior broker failure")
	}
	// A source that already recorded a branded cookie rejection must keep it;
	// inspection during teardown cannot change the first observed cause.
	cookieTrace := nativePilotInitialTrace{firstCause: "cookie_candidate", cookie: "candidate_observed", terminal: "observed_cookie_write"}
	cookie := (nativePilotInitialDetail{role: "collector", phase: "bound_finish", reason: "observed_cookie_write", trace: &cookieTrace}).closed()
	b.controller.adapter.initial = cookie
	b.noteInitial(&nativePilotInitialError{origin}, "reader", "inspect_before")
	if got := b.initialDiagnostic(); got.String() != cookie.String() {
		t.Fatal("later inspection replaced branded source cause")
	}
}

func TestXManualPilotOwnerInspectionCodeVersions(t *testing.T) {
	for _, version := range []struct{ number, count int }{{2, 6}, {3, 8}, {4, 10}, {5, 13}, {6, 14}, {7, 15}, {8, 17}, {9, 19}} {
		code := fmt.Sprintf("manual_initial_v%d", version.number) + strings.Repeat("_0", 4+version.count)
		detail, ok := nativePilotInitialCode(code)
		if !ok || detail.trace == nil || detail.trace.inspectStage != "unknown" || detail.trace.inspectCondition != "unknown" || detail.trace.prepareStage != "unknown" || detail.trace.prepareCondition != "unknown" {
			t.Fatalf("V%d compatibility/defaults lost", version.number)
		}
		for _, bad := range []string{code[:strings.LastIndex(code, "_")], code + "_0", strings.TrimSuffix(code, "_0") + "_99", strings.TrimSuffix(code, "_0") + "_00", strings.Replace(code, "_0_", "_+0_", 1), code + strings.Repeat("0", 80)} {
			if _, ok := nativePilotInitialCode(bad); ok {
				t.Fatalf("V%d noncanonical or over-cap code accepted", version.number)
			}
		}
	}
	for field := 15; field < 17; field++ {
		for index, value := range nativePilotTraceCatalogs[field] {
			parts := make([]string, 21)
			for i := range parts {
				parts[i] = "0"
			}
			parts[4] = strconv.Itoa(slices.Index(nativePilotTraceCatalogs[0], "inspection"))
			parts[4+field] = strconv.Itoa(index)
			code := "manual_initial_v8_" + strings.Join(parts, "_")
			d, ok := nativePilotInitialCode(code)
			if !ok || len(code) > 80 || d.trace == nil || d.trace.firstCause != "inspection" || d.trace.values()[field] != value || !strings.Contains(d.String(), "domain=inspection") {
				t.Fatal("inspection catalog index or wire arity lost")
			}
		}
	}
	unsafe := nativePilotInitialTrace{firstCause: "inspection", inspectStage: "PRIVATE_STAGE", inspectCondition: strings.Repeat("PRIVATE", 1000)}
	if strings.Contains(fmt.Sprintf("%v %#v", unsafe, unsafe), "PRIVATE") || !strings.Contains(unsafe.String(), "inspect_stage=unknown inspect_condition=unknown") {
		t.Fatal("untrusted inspection fields escaped closed formatting")
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

func TestXManualPilotOwnerSharedRemainingBudget(t *testing.T) {
	cfg, f := testXManualOwnerConfig()
	b := testXManualBroker(t, cfg, f)
	ctx, cancel := context.WithTimeout(context.Background(), 750*time.Millisecond)
	defer cancel()
	d := b.start(ctx)
	assertXManualOffline(t, d, "manual-attempt-valid")
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, role := range []string{"reader", "collector"} {
		budget := f.startupBudgets[role]
		if budget <= 0 || budget > 750 {
			t.Fatal("owner acquisition reset or exceeded the parent's remaining budget")
		}
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

func TestXManualPilotOwnerPreparePredicates(t *testing.T) {
	tests := []struct{ fault, stage, condition string }{
		{"replay", "go_admission", "replay"},
		{"snapshot", "go_core_before", "os_snapshot"},
		{"core-shape", "go_core_before", "owner_shape"},
		{"core-reference", "go_core_before", "reference_invalid"},
		{"core-profile", "go_core_before", "profile_mismatch"},
		{"request-profile", "go_request", "profile_mismatch"},
		{"request-parent", "go_request", "parent_mismatch"},
		{"request-attempt", "go_request", "attempt_mismatch"},
		{"rpc", "go_rpc", "command_failed"},
		{"decode", "go_decode", "reply_invalid"},
		{"handle", "go_decode", "handle_invalid"},
		{"after-error", "go_core_after", "directory_changed"},
		{"after-change", "go_core_after", "os_changed"},
		{"bind-owner", "go_bind", "owner_mismatch"},
		{"bind-profile", "go_bind", "profile_mismatch"},
		{"bind-pins", "go_bind", "pins_invalid"},
	}
	for _, role := range []string{"reader", "collector"} {
		for _, test := range tests {
			t.Run(role+"/"+test.fault, func(t *testing.T) {
				cfg, f := testXManualOwnerConfig()
				// Reject the first cleanup too: it must never replace the prepare cause.
				f.fault = "prepare-cleanup-uncertain"
				calls := 0
				backend := &testXManualInspectBackend{testXManualOwnerBackend: f}
				backend.snapshotHook = func(ctx context.Context, selected string) (nativePilotOwner, error) {
					calls++
					o, err := f.snapshot(ctx, selected)
					if test.fault == "snapshot" {
						return o, errors.New("PRIVATE_TOKEN")
					}
					if calls == 1 {
						switch test.fault {
						case "core-shape":
							o.owner = "PRIVATE_OWNER"
						case "core-reference":
							o.directory = ""
						case "core-profile":
							o.profile = "PRIVATE_PROFILE"
						}
					} else {
						if test.fault == "after-error" {
							return o, xManualPilotCoreFailure(nil, "directory_changed")
						}
						if test.fault == "after-change" {
							o.creation = "PRIVATE_CREATION"
						}
					}
					return o, err
				}
				backend.rpcHook = func(ctx context.Context, selected, op string, fields map[string]any, out any) error {
					if op != "manual_prepare" {
						return f.rpc(ctx, selected, op, fields, out)
					}
					if test.fault == "rpc" {
						return errors.New("PRIVATE_TOKEN auth_token=PRIVATE_COOKIE")
					}
					pins := map[string]any{}
					for k, v := range f.pins[selected] {
						pins[k] = v
					}
					value := map[string]any{"handle": "offline-handle", "pins": pins}
					switch test.fault {
					case "decode":
						value["PRIVATE_FIELD"] = "PRIVATE_BODY"
					case "handle":
						value["handle"] = ""
					case "bind-owner":
						pins["ownerId"] = "PRIVATE_OWNER"
					case "bind-profile":
						pins["profileId"] = "PRIVATE_PROFILE"
					case "bind-pins":
						pins["documentId"] = ""
					}
					return testXManualReply(out, value)
				}
				p := &xManualPilotOwnerPorts{cfg: cfg, backend: backend, prepared: map[string]nativePilotPreparedRole{}, begun: map[string]bool{}, acquired: map[string]bool{}, cleanupTokens: map[string]string{}, cleanupUnknown: map[string]bool{}}
				q := nativePilotPrepareRequest{role: role, profile: f.core[role].profile, parent: f.core[role].parent, attempt: cfg.pilot.binding.attempt}
				switch test.fault {
				case "replay":
					p.begun[role] = true
				case "request-profile":
					q.profile = "PRIVATE_PROFILE"
				case "request-parent":
					q.parent = "PRIVATE_PARENT"
				case "request-attempt":
					q.attempt = "PRIVATE_ATTEMPT"
				}
				_, err := p.prepare(context.Background(), q)
				if err == nil {
					t.Fatal("invalid prepare admitted")
				}
				d := nativePilotInitialOf(err, "unknown", "acquire_rpc")
				if d.role != role || d.phase != "prepare" || d.reason != "unavailable" || d.trace == nil || d.trace.firstCause != "preparation" || d.trace.prepareStage != test.stage || d.trace.prepareCondition != test.condition {
					t.Fatalf("prepare predicate lost: %s", d)
				}
				if strings.Contains(fmt.Sprintf("%v %#v", d, d), "PRIVATE") {
					t.Fatal("prepare diagnostic leaked input")
				}
				b := &nativePilotBroker{}
				b.noteInitial(err, role, "prepare")
				b.noteInitial(context.Canceled, "pair", "joint")
				b.noteInitial(errors.New("PRIVATE_CLEANUP"), role, "cleanup")
				if b.initialDiagnostic().String() != d.String() {
					t.Fatal("secondary failure replaced prepare origin")
				}
			})
		}
	}
}

type testXManualPrepareCleanupBackend struct {
	*testXManualInspectBackend
	entered, release chan struct{}
}

func (f *testXManualPrepareCleanupBackend) cleanup(ctx context.Context, role, _ string, _ bool) error {
	if role == "reader" {
		close(f.entered)
		select {
		case <-f.release:
		case <-ctx.Done():
		}
		return errors.New("PRIVATE_CLEANUP")
	}
	return nil
}
func TestXManualPilotOwnerPreparePeerReplyBeforeCleanup(t *testing.T) {
	cfg, f := testXManualOwnerConfig()
	backend := &testXManualPrepareCleanupBackend{testXManualInspectBackend: &testXManualInspectBackend{testXManualOwnerBackend: f, snapshotHook: f.snapshot}, entered: make(chan struct{}), release: make(chan struct{})}
	backend.rpcHook = func(ctx context.Context, role, op string, fields map[string]any, out any) error {
		if role == "collector" {
			return errors.New("PRIVATE_PEER")
		}
		return testXManualReply(out, map[string]any{"PRIVATE_BODY": true})
	}
	p := &xManualPilotOwnerPorts{cfg: cfg, backend: backend, prepared: map[string]nativePilotPreparedRole{}, begun: map[string]bool{}, acquired: map[string]bool{}, cleanupTokens: map[string]string{}, cleanupUnknown: map[string]bool{}}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	start := func(role string) error {
		_, err := p.prepare(ctx, nativePilotPrepareRequest{role: role, profile: f.core[role].profile, parent: f.core[role].parent, attempt: cfg.pilot.binding.attempt})
		return err
	}
	reader := make(chan error, 1)
	go func() { reader <- start("reader") }()
	select {
	case <-backend.entered:
	case <-ctx.Done():
		t.Fatal("reader did not enter cleanup")
	}
	peer := start("collector")
	close(backend.release)
	var primary error
	select {
	case primary = <-reader:
	case <-ctx.Done():
		t.Fatal("reader prepare did not return")
	}
	for _, err := range []error{peer, primary} {
		d := nativePilotInitialOf(err, "unknown", "acquire_rpc")
		if d.role != "reader" || d.trace == nil || d.trace.prepareStage != "go_decode" || d.trace.prepareCondition != "reply_invalid" {
			t.Fatal("cleanup delay allowed a later peer to replace the cause")
		}
	}
	if !nativePilotInitialOf(peer, "collector", "prepare").peer || nativePilotInitialOf(primary, "reader", "prepare").peer {
		t.Fatal("peer delivery lost origin attribution")
	}
}

func TestXManualPilotOwnerPrepareCodeAndOrigin(t *testing.T) {
	for field := 17; field < 19; field++ {
		for index, value := range nativePilotTraceCatalogs[field] {
			parts := make([]string, 23)
			for i := range parts {
				parts[i] = "0"
			}
			parts[4] = strconv.Itoa(slices.Index(nativePilotTraceCatalogs[0], "preparation"))
			parts[4+field] = strconv.Itoa(index)
			code := "manual_initial_v9_" + strings.Join(parts, "_")
			d, ok := nativePilotInitialCode(code)
			if !ok || len(code) > 80 || d.trace.values()[field] != value || !strings.Contains(d.String(), "domain=preparation") {
				t.Fatal("V9 preparation catalog failed")
			}
		}
	}
	unsafe := nativePilotPrepareFailure(errors.New("PRIVATE_TOKEN"), "reader", "PRIVATE_STAGE", "PRIVATE_CONDITION")
	if strings.Contains(unsafe.Error(), "PRIVATE") {
		t.Fatal("untrusted prepare values escaped")
	}
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded, nativePilotInitialWrap(nil, "collector", "settings")} {
		before := nativePilotInitialOf(cause, "reader", "prepare")
		after := nativePilotInitialOf(nativePilotPrepareFailure(cause, "reader", "go_rpc", "command_failed"), "reader", "prepare")
		if before.String() != after.String() {
			t.Fatal("prepare replaced earlier cause or joint deadline")
		}
	}
}

func TestXManualPilotOwnerPrepareCleanupUncertain(t *testing.T) {
	cfg, f := testXManualOwnerConfig()
	f.fault = "prepare-cleanup-uncertain"
	b := testXManualBroker(t, cfg, f)
	d := b.start(context.Background())
	assertXManualOffline(t, d, "cleanup-uncertain")
	origin := b.initialDiagnostic()
	if origin.trace == nil || origin.phase != "prepare" || origin.trace.prepareStage != "go_decode" || origin.trace.prepareCondition != "reply_invalid" {
		t.Fatal("cleanup uncertainty replaced prepare decode cause")
	}
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
