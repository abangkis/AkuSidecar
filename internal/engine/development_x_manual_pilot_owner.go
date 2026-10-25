//go:build auth_session_dev

package engine

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Provenance belongs to a compiled trusted backend. No command or JSON field
// selects native provenance. The native factory lives in the Windows QA file;
// offline tests use a separate backend that always reports offline provenance.
type xManualPilotOwnerBackend interface {
	provenance() nativePilotProvenance
	now() time.Time
	snapshot(context.Context, string) (nativePilotOwner, error)
	rpc(context.Context, string, string, map[string]any, any) error
	cleanup(context.Context, string, string, bool) error
}
type xManualPilotOwnerConfig struct {
	enabled bool
	pilot   nativePilotConfig
	policy  func(context.Context) (nativePilotBrokerState, error)
}
type xManualPilotOwnerPorts struct {
	mu              sync.Mutex
	cfg             xManualPilotOwnerConfig
	backend         xManualPilotOwnerBackend
	prepared        map[string]nativePilotPreparedRole
	begun, acquired map[string]bool
	cleanupTokens   map[string]string
	cleanupUnknown  map[string]bool
}

func (xManualPilotOwnerConfig) String() string     { return "private-x-manual-owner-config" }
func (v xManualPilotOwnerConfig) GoString() string { return v.String() }
func (*xManualPilotOwnerPorts) String() string     { return "private-x-manual-owner-ports" }
func (v *xManualPilotOwnerPorts) GoString() string { return v.String() }

type xManualPilotPins struct {
	OwnerID    string `json:"ownerId"`
	ProfileID  string `json:"profileId"`
	ContextID  string `json:"contextId"`
	TargetID   string `json:"targetId"`
	FrameID    string `json:"frameId"`
	DocumentID string `json:"documentId"`
}

func (xManualPilotPins) String() string               { return "private-x-manual-pins" }
func (v xManualPilotPins) GoString() string           { return v.String() }
func (xManualPilotPins) MarshalJSON() ([]byte, error) { return []byte("{}"), nil }

type xManualPilotInspection struct {
	Pins           xManualPilotPins `json:"pins"`
	WindowCount    *int             `json:"windowCount"`
	BootstrapCount *int             `json:"bootstrapCount"`
	Paused         *bool            `json:"paused"`
	Invalid        *bool            `json:"invalid"`
	Headless       *bool            `json:"headless"`
}

// Strict private control decoding: bounded, duplicate-free, exact struct fields,
// no null coercion or invalid Unicode normalization. This is not a public report.
func xManualPilotDecode(raw []byte, out any) error {
	bad := errors.New("x-owner-reply-invalid")
	if len(raw) == 0 || len(raw) > nativePilotStartupWireMax || !nativePilotStartupUnicode(raw) {
		return bad
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	nodes := 0
	var value func(int) bool
	value = func(depth int) bool {
		nodes++
		if nodes > 96 || depth > 4 {
			return false
		}
		token, err := d.Token()
		if err != nil {
			return false
		}
		delim, compound := token.(json.Delim)
		if !compound {
			return token != nil
		}
		if delim != '{' {
			return false
		}
		seen := map[string]bool{}
		for d.More() {
			key, err := d.Token()
			s, ok := key.(string)
			if err != nil || !ok || seen[s] {
				return false
			}
			seen[s] = true
			if !value(depth + 1) {
				return false
			}
		}
		end, err := d.Token()
		return err == nil && end == json.Delim('}')
	}
	if !value(0) {
		return bad
	}
	if _, err := d.Token(); err != io.EOF {
		return bad
	}
	var exact func([]byte, reflect.Type) bool
	exact = func(input []byte, t reflect.Type) bool {
		for t.Kind() == reflect.Pointer {
			t = t.Elem()
		}
		if t == reflect.TypeOf(json.RawMessage{}) || t.Kind() != reflect.Struct {
			return true
		}
		var fields map[string]json.RawMessage
		if json.Unmarshal(input, &fields) != nil {
			return false
		}
		allowed := map[string]reflect.Type{}
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			if !field.IsExported() {
				continue
			}
			name := strings.Split(field.Tag.Get("json"), ",")[0]
			if name == "-" {
				continue
			}
			if name == "" {
				name = field.Name
			}
			allowed[name] = field.Type
		}
		for name, value := range fields {
			field, ok := allowed[name]
			if !ok || !exact(value, field) {
				return false
			}
		}
		return true
	}
	if out == nil || !exact(raw, reflect.TypeOf(out)) {
		return bad
	}
	typed := json.NewDecoder(bytes.NewReader(raw))
	typed.DisallowUnknownFields()
	if typed.Decode(out) != nil || typed.Decode(new(any)) != io.EOF {
		return bad
	}
	return nil
}
func (p *xManualPilotOwnerPorts) core(ctx context.Context, role string) (nativePilotOwner, error) {
	done := nativePilotTimed(ctx, role, "os_snapshot")
	defer done()
	o, err := p.backend.snapshot(ctx, role)
	selected := p.cfg.pilot.binding.reader
	if role == "collector" {
		selected = p.cfg.pilot.binding.collector
	}
	if err != nil || o.role != role || o.pid <= 0 || o.owner != strconv.Itoa(o.pid) || o.profile != selected.profile || o.parent != selected.parent || o.generation == 0 || o.context != "" || o.target != "" || o.frame != "" || o.document != "" {
		return nativePilotOwner{}, errors.New("x-owner-os-unavailable")
	}
	for _, ref := range []string{o.owner, o.profile, o.image, o.creation, o.job, o.parent} {
		if !nativePilotRef(ref, 512) {
			return nativePilotOwner{}, errors.New("x-owner-os-unavailable")
		}
	}
	if !nativePilotRef(o.directory, 4096) {
		return nativePilotOwner{}, errors.New("x-owner-os-unavailable")
	}
	return o, nil
}
func (p *xManualPilotOwnerPorts) timedRPC(ctx context.Context, role, op string, fields map[string]any, out any) error {
	stage := map[string]string{"manual_inspect": "inspect_rpc", "manual_inventory": "inventory_rpc", "manual_acquire": "acquire_rpc"}[op]
	done := nativePilotTimed(ctx, role, stage)
	defer done()
	err := p.backend.rpc(ctx, role, op, fields, out)
	if err != nil {
		nativePilotTimingFailure(ctx, err, role, "acquire_rpc")
	}
	return err
}
func xManualPilotBind(core nativePilotOwner, pins xManualPilotPins) (nativePilotOwner, error) {
	if pins.OwnerID != core.owner || pins.ProfileID != core.profile {
		return nativePilotOwner{}, errors.New("x-owner-scope-changed")
	}
	core.context, core.target, core.frame, core.document = pins.ContextID, pins.TargetID, pins.FrameID, pins.DocumentID
	if !validNativePilotOwner(core, core.role) {
		return nativePilotOwner{}, errors.New("x-owner-scope-changed")
	}
	return core, nil
}
func (p *xManualPilotOwnerPorts) cleanup(role, handle string, preserve bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if p.backend.cleanup(ctx, role, handle, preserve && role == "reader") != nil {
		return errors.New("x-owner-cleanup-uncertain")
	}
	return nil
}

func (p *xManualPilotOwnerPorts) failedPrepare(core nativePilotOwner, cause error) (nativePilotPreparedRole, error) {
	if p.cleanup(core.role, "", true) == nil {
		return nativePilotPreparedRole{}, nativePilotInitialWrap(cause, core.role, "prepare")
	}
	// This is solely a local cleanup correlation, returned WITH an error. It is
	// never evidence, a native identifier alias or a handle sent to the driver.
	// The broker can retry null-handle cleanup and retain the uncertainty even if
	// that retry later acknowledges release.
	var nonce [24]byte
	token := "cleanup-only"
	if _, err := rand.Read(nonce[:]); err == nil {
		token += "-" + hex.EncodeToString(nonce[:])
	}
	partial := nativePilotPreparedRole{owner: core, handle: token}
	p.mu.Lock()
	p.cleanupTokens[core.role] = token
	p.cleanupUnknown[core.role] = true
	p.prepared[core.role] = partial
	p.mu.Unlock()
	return partial, nativePilotInitialWrap(cause, core.role, "prepare")
}
func (p *xManualPilotOwnerPorts) abort(ctx context.Context, prepared nativePilotPreparedRole, preserve bool) error {
	p.mu.Lock()
	stored, ok := p.prepared[prepared.owner.role]
	token := p.cleanupTokens[prepared.owner.role]
	uncertain := p.cleanupUnknown[prepared.owner.role]
	p.mu.Unlock()
	if !ok || stored != prepared {
		return errors.New("x-owner-cleanup-uncertain")
	}
	handle := prepared.handle
	if token != "" && handle == token {
		handle = ""
	}
	err := p.backend.cleanup(ctx, prepared.owner.role, handle, preserve && prepared.owner.role == "reader")
	if err != nil || uncertain {
		p.mu.Lock()
		p.cleanupUnknown[prepared.owner.role] = true
		p.mu.Unlock()
		return errors.New("x-owner-cleanup-uncertain")
	}
	return nil
}
func (p *xManualPilotOwnerPorts) prepare(ctx context.Context, req nativePilotPrepareRequest) (nativePilotPreparedRole, error) {
	p.mu.Lock()
	if p.begun[req.role] {
		p.mu.Unlock()
		return nativePilotPreparedRole{}, errors.New("x-owner-prepare-replayed")
	}
	p.begun[req.role] = true
	p.mu.Unlock()
	core, err := p.core(ctx, req.role)
	if err != nil {
		return nativePilotPreparedRole{}, err
	}
	if req.profile != core.profile || req.parent != core.parent || req.attempt != p.cfg.pilot.binding.attempt {
		return nativePilotPreparedRole{}, errors.New("x-owner-request-mismatch")
	}
	var raw json.RawMessage
	if err := p.backend.rpc(ctx, req.role, "manual_prepare", nil, &raw); err != nil {
		return p.failedPrepare(core, err)
	}
	defer clear(raw)
	var ready struct {
		Handle string           `json:"handle"`
		Pins   xManualPilotPins `json:"pins"`
	}
	if xManualPilotDecode(raw, &ready) != nil || !nativePilotRef(ready.Handle, 512) {
		return p.failedPrepare(core, nil)
	}
	partial := nativePilotPreparedRole{owner: core, handle: ready.Handle}
	p.mu.Lock()
	p.prepared[req.role] = partial
	p.mu.Unlock()
	after, err := p.core(ctx, req.role)
	if err != nil || after != core {
		return partial, errors.New("x-owner-os-changed")
	}
	bound, err := xManualPilotBind(core, ready.Pins)
	if err != nil {
		return partial, err
	}
	partial.owner = bound
	p.mu.Lock()
	p.prepared[req.role] = partial
	p.mu.Unlock()
	return partial, nil
}
func (p *xManualPilotOwnerPorts) inspect(ctx context.Context, prepared nativePilotPreparedRole) (nativePilotOwner, error) {
	return p.inspectPhase(ctx, prepared, false)
}
func (p *xManualPilotOwnerPorts) inspectPhase(ctx context.Context, prepared nativePilotPreparedRole, requirePause bool) (nativePilotOwner, error) {
	p.mu.Lock()
	stored, ok := p.prepared[prepared.owner.role]
	acquired := p.acquired[prepared.owner.role]
	p.mu.Unlock()
	phase := "inspect_before"
	if acquired && !requirePause {
		phase = "inspect_after"
	}
	fail := func(stage, condition string) (nativePilotOwner, error) {
		trace := nativePilotInitialTrace{firstCause: "inspection", terminal: "unavailable", inspectStage: stage, inspectCondition: condition}
		return nativePilotOwner{}, &nativePilotInitialError{(nativePilotInitialDetail{role: prepared.owner.role, phase: phase, reason: "unavailable", trace: &trace}).closed()}
	}
	if !ok || stored != prepared {
		return fail("go_stored", "handle_mismatch")
	}
	core, err := p.core(ctx, prepared.owner.role)
	if err != nil {
		return fail("go_core_before", "os_snapshot")
	}
	var raw json.RawMessage
	if err := p.timedRPC(ctx, prepared.owner.role, "manual_inspect", nil, &raw); err != nil {
		clear(raw)
		// The transport's branded Node diagnostic owns the originating predicate.
		// Arbitrary RPC error text never enters the private closed diagnostic.
		var typed *nativePilotInitialError
		if errors.As(err, &typed) {
			return nativePilotOwner{}, nativePilotInitialWrap(err, prepared.owner.role, phase)
		}
		return fail("go_rpc", "command_failed")
	}
	defer clear(raw)
	var result xManualPilotInspection
	if xManualPilotDecode(raw, &result) != nil {
		return fail("go_decode", "reply_invalid")
	}
	if result.Invalid == nil || *result.Invalid {
		return fail("go_validate", "invalid_flag")
	}
	if result.Headless == nil || *result.Headless != (prepared.owner.role == "collector") {
		return fail("go_validate", "headless_mismatch")
	}
	if result.Paused == nil {
		return fail("go_validate", "paused_missing")
	}
	if result.WindowCount == nil || result.BootstrapCount == nil {
		return fail("go_validate", "inventory_missing")
	}
	if requirePause && !*result.Paused {
		return fail("go_validate", "pause_required")
	}
	if *result.WindowCount < 0 || *result.WindowCount > 16 {
		return fail("go_validate", "window_count")
	}
	if *result.BootstrapCount < 0 || *result.BootstrapCount > 16 {
		return fail("go_validate", "bootstrap_count")
	}
	if prepared.owner.role == "reader" && *result.WindowCount < 1 {
		return fail("go_validate", "reader_windows")
	}
	if prepared.owner.role == "reader" && *result.BootstrapCount != 1 {
		return fail("go_validate", "reader_bootstrap")
	}
	if prepared.owner.role == "collector" && *result.WindowCount != 0 {
		return fail("go_validate", "collector_windows")
	}
	if prepared.owner.role == "collector" && *result.BootstrapCount != 0 {
		return fail("go_validate", "bootstrap_count")
	}
	bound, err := xManualPilotBind(core, result.Pins)
	if err != nil || bound != prepared.owner {
		return fail("go_bind", "pins_mismatch")
	}
	after, err := p.core(ctx, prepared.owner.role)
	if err != nil {
		return fail("go_core_after", "os_snapshot")
	}
	if after != core {
		return fail("go_core_after", "os_changed")
	}
	return bound, nil
}

// One enumeration supplies both parent links for this checkpoint only. Process
// creation, handles, images, Job membership and directories are checked separately.
func xManualPilotParentPair(node, chrome uint32, walk func(func(uint32, uint32) bool) error) (uint32, uint32, error) {
	if node == 0 || chrome == 0 || node == chrome || walk == nil {
		return 0, 0, errors.New("x-owner-parent-unavailable")
	}
	var nodeParent, chromeParent uint32
	err := walk(func(pid, parent uint32) bool {
		if pid == node {
			nodeParent = parent
		}
		if pid == chrome {
			chromeParent = parent
		}
		return nodeParent == 0 || chromeParent == 0
	})
	if err != nil || nodeParent == 0 || chromeParent == 0 {
		return 0, 0, errors.New("x-owner-parent-unavailable")
	}
	return nodeParent, chromeParent, nil
}

func xManualPilotStartupBudget(ctx context.Context) (int64, error) {
	if ctx == nil || ctx.Err() != nil {
		return 0, errors.New("x-owner-startup-deadline")
	}
	remaining := nativePilotJoint
	if deadline, ok := ctx.Deadline(); ok {
		if left := time.Until(deadline); left < remaining {
			remaining = left
		}
	}
	// Round down: the private hint may shorten a wait, never extend Go's
	// authoritative context deadline, including transport and final inspection.
	ms := remaining.Milliseconds()
	if ms <= 0 {
		return 0, errors.New("x-owner-startup-deadline")
	}
	return ms, nil
}

func (p *xManualPilotOwnerPorts) acquire(ctx context.Context, prepared nativePilotPreparedRole, ch nativePilotCaptureChallenge) ([]byte, error) {
	p.mu.Lock()
	stored, ok := p.prepared[prepared.owner.role]
	replayed := p.acquired[prepared.owner.role]
	if ok && !replayed && stored == prepared {
		p.acquired[prepared.owner.role] = true
	}
	p.mu.Unlock()
	if !ok || replayed || stored != prepared || ch.challenge.owner != prepared.owner {
		return nil, errors.New("x-owner-acquire-mismatch")
	}
	if _, err := p.inspectPhase(ctx, prepared, true); err != nil {
		return nil, nativePilotInitialWrap(err, prepared.owner.role, "inspect_before")
	}
	request, err := ch.requestJSON()
	if err != nil {
		return nil, errors.New("x-owner-challenge-invalid")
	}
	defer clear(request)
	budget, err := xManualPilotStartupBudget(ctx)
	if err != nil {
		return nil, nativePilotInitialWrap(context.DeadlineExceeded, "pair", "joint")
	}
	var raw json.RawMessage
	if err := p.timedRPC(ctx, prepared.owner.role, "manual_acquire", map[string]any{"handle": prepared.handle, "request": json.RawMessage(request), "startupBudgetMS": budget}, &raw); err != nil {
		clear(raw)
		return nil, nativePilotInitialWrap(err, prepared.owner.role, "acquire_rpc")
	}
	if _, err := p.inspect(ctx, prepared); err != nil {
		clear(raw)
		return nil, nativePilotInitialWrap(err, prepared.owner.role, "inspect_after")
	}
	return raw, nil
}
func (p *xManualPilotOwnerPorts) state(ctx context.Context) (nativePilotBrokerState, error) {
	s, err := p.cfg.policy(ctx)
	if err != nil {
		return nativePilotBrokerState{}, errors.New("x-owner-policy-unavailable")
	}
	for _, role := range []string{"reader", "collector"} {
		if _, err := p.core(ctx, role); err != nil {
			return nativePilotBrokerState{}, err
		}
	}
	var raw json.RawMessage
	if p.timedRPC(ctx, "reader", "manual_inventory", nil, &raw) != nil {
		return nativePilotBrokerState{}, errors.New("x-owner-inventory-unavailable")
	}
	defer clear(raw)
	var inventory struct {
		WindowCount    *int `json:"windowCount"`
		BootstrapCount *int `json:"bootstrapCount"`
	}
	if xManualPilotDecode(raw, &inventory) != nil || inventory.WindowCount == nil || inventory.BootstrapCount == nil || *inventory.WindowCount < 0 || *inventory.WindowCount > 16 || *inventory.BootstrapCount != 1 && *inventory.WindowCount > 0 {
		return nativePilotBrokerState{}, errors.New("x-owner-inventory-invalid")
	}
	s.readerWindows = *inventory.WindowCount
	s.parentAlive = s.parentAlive && ctx.Err() == nil
	return s, nil
}
func newXManualPilotOwnerBrokerWithBackend(parent context.Context, cfg xManualPilotOwnerConfig, backend xManualPilotOwnerBackend) (*nativePilotBroker, nativePilotDiagnostic) {
	b, _, d := newXManualPilotOwnerBrokerPorts(parent, cfg, backend)
	return b, d
}
func newXManualPilotOwnerBrokerPorts(parent context.Context, cfg xManualPilotOwnerConfig, backend xManualPilotOwnerBackend) (*nativePilotBroker, *xManualPilotOwnerPorts, nativePilotDiagnostic) {
	if !cfg.enabled {
		return nil, nil, nativePilotDiagnostic{Reason: "admission-disabled"}
	}
	if backend == nil || cfg.policy == nil || (backend.provenance() != nativePilotTrustedStartup && backend.provenance() != nativePilotOfflineOnly) {
		return nil, nil, nativePilotDiagnostic{Reason: "invalid-configuration"}
	}
	p := &xManualPilotOwnerPorts{cfg: cfg, backend: backend, prepared: map[string]nativePilotPreparedRole{}, begun: map[string]bool{}, acquired: map[string]bool{}, cleanupTokens: map[string]string{}, cleanupUnknown: map[string]bool{}}
	ports := nativePilotBrokerPorts{now: backend.now, prepare: p.prepare, inspect: p.inspect, acquire: p.acquire, state: p.state, abort: p.abort}
	b, d := newNativePilotBroker(parent, nativePilotBrokerConfig{enabled: true, offlineTest: backend.provenance() == nativePilotOfflineOnly, pilot: cfg.pilot}, ports)
	return b, p, d
}
