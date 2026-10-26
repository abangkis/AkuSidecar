//go:build auth_session_dev

package engine

// This boundary validates private receipts; it grants no controller, native or
// Store authority. No native startup producer is registered here. Startup
// sampling does not establish fresh identity or session continuity afterward.
import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	nativePilotContract = "aku.x.dev-pilot.manual-startup.v2"
	// Startup work and sample freshness are independent bounds. Ownership and
	// transport may take longer; every sample must still be fresh at publication.
	nativePilotSettingsURL           = "https://api.x.com/1.1/account/settings.json"
	nativePilotSettingsQualification = "api_v1.cookie-auth-rejection.2026-10-20"
	nativePilotStartupIdentity       = "bootstrap-current-self.startup.v1"
)

//go:embed x-manual-timing-policy.json
var nativePilotTimingPolicyJSON []byte

// One immutable startup policy is shared with Node. It is a source asset, not a
// runtime opt-in or a grant to transfer sessions for longer than authorized.
var nativePilotTimingPolicy = func() map[string]int64 {
	policy, err := decodeNativePilotTimingPolicy(nativePilotTimingPolicyJSON)
	if err != nil {
		panic("manual timing policy invalid")
	}
	return policy
}()
var (
	nativePilotAge                   = time.Duration(nativePilotTimingPolicy["evidenceMaxAgeMS"]) * time.Millisecond
	nativePilotStartup               = time.Duration(nativePilotTimingPolicy["startupTotalMS"]) * time.Millisecond
	nativePilotLife                  = time.Duration(nativePilotTimingPolicy["attemptAuthorizationMaxMS"]) * time.Millisecond
	nativePilotCleanup               = time.Duration(nativePilotTimingPolicy["ownerCleanupMS"]) * time.Millisecond
	nativePilotCaptureCleanupTimeout = time.Duration(nativePilotTimingPolicy["captureCleanupMS"]) * time.Millisecond
)

func decodeNativePilotTimingPolicy(raw []byte) (map[string]int64, error) {
	bad := errors.New("manual timing policy invalid")
	if len(raw) == 0 || len(raw) > 4096 {
		return nil, bad
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	if token, err := d.Token(); err != nil || token != json.Delim('{') {
		return nil, bad
	}
	values := map[string]json.RawMessage{}
	for d.More() {
		token, err := d.Token()
		key, ok := token.(string)
		if err != nil || !ok || values[key] != nil {
			return nil, bad
		}
		var value json.RawMessage
		if d.Decode(&value) != nil {
			return nil, bad
		}
		values[key] = value
	}
	if token, err := d.Token(); err != nil || token != json.Delim('}') {
		return nil, bad
	}
	if _, err := d.Token(); err != io.EOF {
		return nil, bad
	}
	keys := []string{"startupTotalMS", "evidenceMaxAgeMS", "cdpCommandMS", "earlyCommandMS", "bootstrapObservationMS", "bootstrapEvaluationMS", "rendezvousMS", "settingsFallbackMS", "collectorHomeReadyMS", "collectorExitMS", "ownerCleanupMS", "captureCleanupMS", "captureCommandMS", "captureHydrationMS", "attemptAuthorizationMaxMS"}
	if len(values) != len(keys)+1 {
		return nil, bad
	}
	var schema string
	if json.Unmarshal(values["schema"], &schema) != nil || schema != "aku.x.manual-timing.v1" {
		return nil, bad
	}
	result := map[string]int64{}
	for _, key := range keys {
		var v int64
		if json.Unmarshal(values[key], &v) != nil || v <= 0 || v > 600000 {
			return nil, bad
		}
		result[key] = v
	}
	if result["attemptAuthorizationMaxMS"] != 60000 || result["startupTotalMS"] > result["attemptAuthorizationMaxMS"] || result["earlyCommandMS"] <= result["cdpCommandMS"] || result["bootstrapEvaluationMS"] > result["cdpCommandMS"] {
		return nil, bad
	}
	if result["captureHydrationMS"] < 1000 || result["captureHydrationMS"] > 45000 || result["captureHydrationMS"] > result["attemptAuthorizationMaxMS"] {
		return nil, bad
	}
	for _, key := range []string{"evidenceMaxAgeMS", "cdpCommandMS", "earlyCommandMS", "bootstrapObservationMS", "bootstrapEvaluationMS", "rendezvousMS", "settingsFallbackMS", "collectorExitMS", "ownerCleanupMS", "captureCleanupMS", "captureCommandMS"} {
		if result[key] > result["startupTotalMS"] {
			return nil, bad
		}
	}
	return result, nil
}

// Private pins are actual source references, never synthetic IDs or revisions.
type nativePilotOwner struct {
	role, owner, profile, directory, image, creation, job, parent string
	context, target, frame, document                              string
	pid                                                           int
	generation                                                    uint64
}

type nativePilotBinding struct {
	// derivation references the separately authorized exact source/destination
	// two-cookie receipt. The trusted owner port must verify this receipt against
	// the pair/attempt, without seeding or exposing credential material here.
	account, enrollment, pair, attempt, derivation string
	policy, consent                                uint64
	reader, collector                              nativePilotOwner
}

// The owner port must inventory every reader window (including bootstrap
// windows), resolve physical directories and verify process creation/image/Job.
// A count without a complete trusted inventory is insufficient.
type nativePilotState struct {
	binding                                                                                  nativePilotBinding
	readerWindows                                                                            int
	completeInventory, parentAlive, ownersAlive, channelHealthy, invalidationsReady, optedIn bool
}

type nativePilotConfig struct {
	contract                        string
	enabled, limitationAcknowledged bool
	binding                         nativePilotBinding
	// Captured before the earlier of initial acquisition or authorized seeding.
	attemptStarted time.Time
	// Optional explicit owner/consent/attempt limit, distinct from sample freshness.
	attemptValidUntil time.Time
}

// This selects the accepted startup recipe only, never live renewal. The trusted
// source must acquire a NEW per-attempt startup document/current-self projection
// and fresh pinned settings response. Historical QA DTOs and stale bootstrap
// rereads cannot implement the private receipt interface.
type nativePilotProvenance uint8

const (
	nativePilotUnqualified nativePilotProvenance = iota
	nativePilotTrustedStartup
	nativePilotOfflineOnly
)

type nativePilotChallenge struct {
	seal    *nativePilotSeal
	vector  uint64
	role    string
	binding nativePilotBinding
	owner   nativePilotOwner
	started time.Time
}

// Each startup response must be freshly correlated to the challenge
// and exact document/request. Source times are wall timestamps; elapsed is a
// duration from that source's own monotonic clock, never its absolute clock.
// Trusted producers must enforce body/type/semantic predicates before receipt
// construction. This adapter does not decode or promote public QA reports.
type nativePilotResponse struct {
	challenge                                                           nativePilotChallenge
	request, source, method, url, qualification                         string
	account                                                             string
	observed, validUntil                                                time.Time
	elapsed                                                             time.Duration
	currentSelf, semanticSuccess, fresh, direct, wellFormed, clockValid bool
}
type nativePilotReceipt struct{ identity, settings nativePilotResponse }

type nativePilotSource interface {
	provenance() nativePilotProvenance
	observe(context.Context, nativePilotChallenge) (nativePilotReceipt, error)
}
type nativePilotPorts struct {
	// now must preserve local monotonic readings and be safe for concurrent use.
	now    func() time.Time
	owners func(context.Context) (nativePilotState, error)
	source nativePilotSource
}

// Only closed diagnostics may cross the adapter boundary. Validation success is
// an offline readiness result, not initial admission or renewal authorization.
type nativePilotDiagnostic struct {
	Reason           string `json:"reason"`
	OfflineValidated bool   `json:"offlineValidated"`
	ManualEligible   bool   `json:"manualEligible"`
	CanAuthorize     bool   `json:"canAuthorize"`
	NativeQualified  bool   `json:"nativeQualified"`
	StorePermit      bool   `json:"storePermit"`
}

// Diagnostic-only monotonic clock. No observation, policy clock or deadline is
// derived from this ledger. Fixed slots bound memory and exclude private data.
var nativePilotTimingOps = [...]string{"idle", "os_snapshot", "inspect_rpc", "inventory_rpc", "acquire_rpc", "wire_decode", "identity", "rendezvous", "settings_wait", "cleanup", "root_restore", "complete"}
var nativePilotSettingsWaitStates = [...]string{"unknown", "request_pending", "request_before_identity", "request_frame", "request_document", "request_history", "response_pending", "completion_pending", "extra_pending", "metadata_complete", "network_failed", "candidate_rejected", "scope_lost", "body_pending", "body_complete", "body_failed"}

type nativePilotTimingKey struct{}
type nativePilotRoleTiming struct {
	op, nodeOp            int
	settingsWait          int
	started, nodeReceived time.Time
	nodeMS                int64
	sequence              uint64
	totals                [len(nativePilotTimingOps)]time.Duration
	nodeTotals            [len(nativePilotTimingOps)]time.Duration
}
type nativePilotTimingRoleSnapshot struct {
	operation, nodeOperation int
	settingsWait             int
	activeMS, nodeMS         int64
	totals                   [len(nativePilotTimingOps)]int64
	nodeTotals               [len(nativePilotTimingOps)]int64
}
type nativePilotFlowTiming struct {
	role, stage, condition                          string
	elapsedMS, remainingMS                          int64
	hasSamples                                      bool
	identityAgeMS, settingsAgeMS, sampleRemainingMS int64
}
type nativePilotTimingSnapshot struct {
	elapsedMS int64
	roles     [2]nativePilotTimingRoleSnapshot
	flow      [24]nativePilotFlowTiming
	flowCount int
}
type nativePilotTiming struct {
	mu      sync.Mutex
	now     func() time.Time
	started time.Time
	roles   [2]nativePilotRoleTiming
	first   *nativePilotInitialDetail
	frozen  *nativePilotTimingSnapshot
	flow    []nativePilotFlowTiming
}

// Durations and closed categories only. Diagnostic state cannot grant authority
// or replace the separately latched first operational error.
func nativePilotFlowTrace(ctx context.Context, role, stage, condition string, at, deadline time.Time, receipt *nativePilotReceipt) {
	l := nativePilotTimingOf(ctx)
	if l == nil {
		return
	}
	allowed := func(value string, values ...string) bool {
		for _, known := range values {
			if value == known {
				return true
			}
		}
		return false
	}
	if !allowed(role, "reader", "collector", "pair") || !allowed(stage, "startup_enter", "owners_before_enter", "owners_before_exit", "receipt_enter", "receipt_exit", "identity_check", "settings_check", "owners_after_enter", "owners_after_exit", "publication") || !allowed(condition, "start", "received", "accepted", "rejected", "source_error", "owner_error", "receipt_invalid", "timestamp_before_start", "timestamp_future", "validity_order", "source_expired", "age_expired", "startup_deadline", "sample_horizon_expired") {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.frozen != nil || len(l.flow) >= 24 {
		return
	}
	bound := func(d time.Duration) int64 {
		if d > nativePilotLife {
			d = nativePilotLife
		}
		if d < -nativePilotLife {
			d = -nativePilotLife
		}
		return d.Milliseconds()
	}
	e := nativePilotFlowTiming{role: role, stage: stage, condition: condition, elapsedMS: nativePilotTimingMS(l.now().Sub(l.started)), remainingMS: bound(deadline.Sub(at))}
	if receipt != nil {
		e.hasSamples = true
		e.identityAgeMS, e.settingsAgeMS = bound(at.Sub(receipt.identity.observed)), bound(at.Sub(receipt.settings.observed))
		until := receipt.identity.observed.Add(nativePilotAge)
		for _, end := range []time.Time{receipt.identity.validUntil, receipt.settings.validUntil, receipt.settings.observed.Add(nativePilotAge)} {
			if end.Before(until) {
				until = end
			}
		}
		e.sampleRemainingMS = bound(until.Sub(at))
	}
	l.flow = append(l.flow, e)
}

func nativePilotTimingIndex(value string) int {
	for i, op := range nativePilotTimingOps {
		if op == value {
			return i
		}
	}
	return 0
}
func nativePilotTimingRole(role string) int {
	if role == "reader" {
		return 0
	}
	if role == "collector" {
		return 1
	}
	return -1
}
func nativePilotTimingMS(d time.Duration) int64 {
	if d < 0 {
		return 0
	}
	if d > nativePilotLife {
		return nativePilotLife.Milliseconds()
	}
	return d.Milliseconds()
}
func nativePilotTimingOf(ctx context.Context) *nativePilotTiming {
	if ctx == nil {
		return nil
	}
	v, _ := ctx.Value(nativePilotTimingKey{}).(*nativePilotTiming)
	return v
}
func nativePilotTimed(ctx context.Context, role, op string) func() {
	l, i, slot := nativePilotTimingOf(ctx), nativePilotTimingRole(role), nativePilotTimingIndex(op)
	if l == nil || i < 0 || slot == 0 || ctx.Err() != nil {
		return func() {}
	}
	l.mu.Lock()
	if l.frozen != nil {
		l.mu.Unlock()
		return func() {}
	}
	r := &l.roles[i]
	r.sequence++
	seq := r.sequence
	r.op = slot
	r.started = l.now()
	r.nodeOp = 0
	r.nodeMS = 0
	r.settingsWait = 0
	l.mu.Unlock()
	return func() {
		l.mu.Lock()
		defer l.mu.Unlock()
		r := &l.roles[i]
		if l.frozen == nil && ctx.Err() == nil && r.sequence == seq && r.op == slot {
			r.totals[slot] += l.now().Sub(r.started)
			r.op = 0
		}
	}
}
func nativePilotTimingFailure(ctx context.Context, err error, role, phase string) nativePilotInitialDetail {
	d := nativePilotInitialOf(err, role, phase)
	l := nativePilotTimingOf(ctx)
	if l == nil {
		return d
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.first == nil {
		if l.frozen != nil {
			return d
		}
		copy := d
		copy.timing = nil
		l.first = &copy
	}
	return *l.first
}

// A progress event is correlated to one pending private RPC. It is evidence of
// progress only; it cannot produce account proof or authorize a capture.
func nativePilotTimingProgress(ctx context.Context, role string, raw []byte) {
	l, i := nativePilotTimingOf(ctx), nativePilotTimingRole(role)
	if l == nil || i < 0 || len(raw) > 512 || ctx.Err() != nil {
		return
	}
	var p struct {
		Operation    string  `json:"operation"`
		ElapsedMS    int64   `json:"elapsedMS"`
		Cause        string  `json:"cause"`
		SettingsWait *string `json:"settingsWait"`
	}
	// Unknown/duplicate fields are rejected by the private transport decoder.
	if !nativePilotProgressDecode(raw, &p) || p.ElapsedMS < 0 || p.ElapsedMS > nativePilotLife.Milliseconds() {
		return
	}
	op := nativePilotTimingIndex(p.Operation)
	if op == 0 {
		return
	}
	settingsWait := 0
	if p.SettingsWait != nil {
		settingsWait = -1
		for j, state := range nativePilotSettingsWaitStates {
			if state == *p.SettingsWait {
				settingsWait = j
				break
			}
		}
		if settingsWait < 0 {
			return
		}
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.frozen != nil {
		return
	}
	r := &l.roles[i]
	if r.op == 0 {
		return
	}
	r.nodeOp = op
	if p.SettingsWait != nil {
		r.settingsWait = settingsWait
	}
	r.nodeMS = p.ElapsedMS
	if duration := time.Duration(p.ElapsedMS) * time.Millisecond; duration > r.nodeTotals[op] {
		r.nodeTotals[op] = duration
	}
	r.nodeReceived = l.now()
	if p.Cause != "" && l.first == nil {
		if d, ok := nativePilotInitialCode(p.Cause); ok && (d.role == role || d.role == "pair" || d.peer && nativePilotTimingRole(d.role) >= 0) {
			d.timing = nil
			l.first = &d
		}
	}
}
func nativePilotProgressDecode(raw []byte, out any) bool {
	d := json.NewDecoder(bytes.NewReader(raw))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return false
	}
	seen := map[string]bool{}
	for d.More() {
		t, e := d.Token()
		key, ok := t.(string)
		if e != nil || !ok || seen[key] || (key != "operation" && key != "elapsedMS" && key != "cause" && key != "settingsWait") {
			return false
		}
		seen[key] = true
		var value json.RawMessage
		if d.Decode(&value) != nil || bytes.Equal(value, []byte("null")) {
			return false
		}
	}
	if t, e := d.Token(); e != nil || t != json.Delim('}') || !seen["operation"] || !seen["elapsedMS"] || !seen["cause"] || len(seen) < 3 || len(seen) > 4 {
		return false
	}
	if _, e := d.Token(); e != io.EOF {
		return false
	}
	return json.Unmarshal(raw, out) == nil
}
func (l *nativePilotTiming) freeze() *nativePilotTimingSnapshot {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.frozen != nil {
		return l.frozen
	}
	now := l.now()
	s := &nativePilotTimingSnapshot{elapsedMS: nativePilotTimingMS(now.Sub(l.started)), flowCount: len(l.flow)}
	copy(s.flow[:], l.flow)
	for i, r := range l.roles {
		v := &s.roles[i]
		v.operation = r.op
		v.nodeOperation = r.nodeOp
		v.settingsWait = r.settingsWait
		if r.op != 0 {
			v.activeMS = nativePilotTimingMS(now.Sub(r.started))
			r.totals[r.op] += now.Sub(r.started)
		}
		if r.nodeOp != 0 {
			v.nodeMS = nativePilotTimingMS(time.Duration(r.nodeMS)*time.Millisecond + now.Sub(r.nodeReceived))
		}
		for j, total := range r.totals {
			v.totals[j] = nativePilotTimingMS(total)
			v.nodeTotals[j] = nativePilotTimingMS(r.nodeTotals[j])
		}
	}
	l.frozen = s
	return s
}
func (s nativePilotTimingSnapshot) String() string {
	bound := func(ms int64) int64 {
		if ms < 0 {
			return 0
		}
		if ms > nativePilotLife.Milliseconds() {
			return nativePilotLife.Milliseconds()
		}
		return ms
	}
	name := func(i int) string {
		if i < 0 || i >= len(nativePilotTimingOps) {
			return "unknown"
		}
		return nativePilotTimingOps[i]
	}
	out := fmt.Sprintf(" elapsed_ms=%d", bound(s.elapsedMS))
	for i, role := range []string{"reader", "collector"} {
		r := s.roles[i]
		out += fmt.Sprintf(" %s_active=%s %s_active_ms=%d %s_node=%s %s_node_age_ms=%d", role, name(r.operation), role, bound(r.activeMS), role, name(r.nodeOperation), role, bound(r.nodeMS))
		if r.settingsWait > 0 && r.settingsWait < len(nativePilotSettingsWaitStates) {
			out += fmt.Sprintf(" %s_settings_wait=%s", role, nativePilotSettingsWaitStates[r.settingsWait])
		}
		for j, ms := range r.totals {
			if ms > 0 {
				out += fmt.Sprintf(" %s_%s_ms=%d", role, name(j), bound(ms))
			}
		}
		for j, ms := range r.nodeTotals {
			if ms > 0 {
				out += fmt.Sprintf(" %s_node_%s_max_ms=%d", role, name(j), bound(ms))
			}
		}
	}
	for i, e := range s.flow[:s.flowCount] {
		out += fmt.Sprintf(" flow_%d=(role=%s stage=%s condition=%s elapsed_ms=%d startup_remaining_ms=%d", i, e.role, e.stage, e.condition, e.elapsedMS, e.remainingMS)
		if e.hasSamples {
			out += fmt.Sprintf(" identity_age_ms=%d settings_age_ms=%d sample_remaining_ms=%d", e.identityAgeMS, e.settingsAgeMS, e.sampleRemainingMS)
		}
		out += ")"
	}
	return out
}

// Private detail only. It never changes public Reason, eligibility or permits.
type nativePilotInitialDetail struct {
	role, phase, reason string
	peer                bool
	trace               *nativePilotInitialTrace
	timing              *nativePilotTimingSnapshot
}
type nativePilotInitialTrace struct{ firstCause, terminal, early, handoff, cookie, extra, guardTrigger, guardProvenance, guardCondition, guardEpoch, guardDiskCache, guardServiceWorker, guardPrefetchCache, guardRequestFrame, guardHeaderReason, inspectStage, inspectCondition, prepareStage, prepareCondition string }

// Append-only V2 catalogs with the V3-V9 fields; order matches STARTUP_TRACE in Node.
var nativePilotTraceCatalogs = [][]string{
	{"unknown", "cookie_candidate", "cookie_ambiguous", "context_lost", "target_lost", "navigation", "owner_scope", "event_cap", "identity_order", "identity_mismatch", "settings_order", "response_frame", "extra_shape", "extra_status", "extra_missing", "extra_unexpected", "generic_early", "stale_observation", "cleanup_failed", "guard_request_unbound", "guard_request_budget", "guard_header_classification", "guard_response_correlation", "guard_metadata_shape", "guard_completion_missing", "guard_header_budget", "guard_event_budget", "root_control", "inspection", "settings_capture", "preparation"},
	{"unknown", "unavailable", "rejected", "identity_unavailable", "source_unavailable", "stale_observation", "cleanup_unverified", "scope_lost", "observed_cookie_write", "deadline", "canceled", "replay", "sample_missing", "sample_duplicate", "invalid_shape", "scope_mismatch", "time_invalid", "bound"},
	{"unknown", "captured", "deadline", "script_limit", "wrong_page", "unexpected_pause", "evaluation_failed", "setup_failed"},
	{"unknown", "accepted", "rejected"},
	{"unknown", "not_observed", "candidate_observed", "ambiguous", "other_only", "guard_request_unbound", "guard_request_budget", "guard_header_classification", "guard_response_correlation", "guard_metadata_shape", "guard_completion_missing", "guard_header_budget", "guard_event_budget"},
	{"unknown", "not_selected", "not_required", "required_pending", "complete", "missing", "status_mismatch", "unexpected", "invalid"},
	{"unknown", "baseline", "arm", "metadata", "request", "document", "response", "extra", "redirect", "finish"},
	{"unknown", "absent", "scoped", "origin_scope", "frame_scope", "loader_missing", "redirect", "reused", "root_scope"},
	{"unknown", "predecessor_missing", "redirect_shape", "previous_response", "chain_completed", "chain_failed", "chain_cache", "previous_resource", "current_resource", "previous_origin", "previous_frame", "previous_loader", "current_origin", "current_frame", "current_loader", "loader_mismatch", "redirect_url", "redirect_status", "extra_flag", "headers_shape", "response_cache", "frame_mismatch", "request_shape", "request_url", "protected_source", "resource_type", "request_method", "session_missing", "frame_malformed", "loader_malformed", "ordinary", "request_absent", "root_arm_closed", "root_arm_repeated", "root_arm_before_seed", "root_arm_loader", "root_arm_session", "root_commit_duplicate", "root_commit_loader_invalid", "root_commit_old_loader", "root_commit_url", "root_commit_parent", "root_commit_loader_mismatch", "root_request_duplicate", "root_request_redirect", "root_request_history", "root_request_method", "root_request_url", "root_request_loader_invalid", "root_request_old_loader", "root_request_commit_mismatch", "root_request_reused", "root_response_duplicate", "root_response_loader", "root_response_url", "root_response_status", "root_response_extra_flag", "root_response_disk_cache", "root_response_service_worker", "root_response_prefetch_cache", "root_extra_duplicate", "root_extra_status", "root_loading_failed", "root_served_from_cache", "root_extra_unexpected", "root_missing", "root_response_missing", "root_commit_missing", "root_extra_missing", "root_response_frame_absent", "root_response_frame_malformed", "root_response_frame_different", "root_control_apply", "root_control_restore"},
	{"unknown", "before_seed", "post_seed", "reload_armed", "document_committed"},
	{"unknown", "absent", "false", "true", "malformed"},
	{"unknown", "absent", "false", "true", "malformed"},
	{"unknown", "absent", "false", "true", "malformed"},
	{"unknown", "absent", "malformed", "different", "matching"},
	{"unknown", "name_unknown", "syntax", "attribute_unsupported", "attribute_invalid", "scope_domain", "scope_path", "metadata_shape", "capacity"},
	{"unknown", "node_snapshot", "node_validate", "go_stored", "go_core_before", "go_rpc", "go_decode", "go_validate", "go_bind", "go_core_after", "snapshot_attach", "snapshot_page", "snapshot_runtime", "snapshot_frame", "snapshot_inventory"},
	{"unknown", "command_failed", "snapshot_shape", "owner_dead", "reference_invalid", "headless_mismatch", "window_count", "bootstrap_count", "reader_windows", "reader_bootstrap", "collector_windows", "owner_mismatch", "profile_mismatch", "target_mismatch", "session_mismatch", "frame_mismatch", "document_mismatch", "context_mismatch", "lifecycle_unready", "handle_mismatch", "os_snapshot", "reply_invalid", "invalid_flag", "paused_missing", "pause_required", "inventory_missing", "pins_mismatch", "os_changed"},
	{"unknown", "go_admission", "go_core_before", "go_request", "go_rpc", "go_decode", "go_core_after", "go_bind", "node_actor", "node_reply", "node_snapshot", "node_validate", "node_distinct", "actor_entry", "actor_pins", "actor_ready", "actor_runner", "driver_setup", "snapshot_attach", "snapshot_page", "snapshot_runtime", "snapshot_frame", "snapshot_inventory"},
	{"unknown", "replay", "os_snapshot", "owner_shape", "reference_invalid", "profile_mismatch", "parent_mismatch", "attempt_mismatch", "command_failed", "reply_invalid", "handle_invalid", "pins_invalid", "os_changed", "owner_mismatch", "state_invalid", "ready_missing", "roles_not_distinct", "snapshot_shape", "owner_dead", "headless_mismatch", "window_count", "bootstrap_count", "reader_windows", "reader_bootstrap", "collector_windows", "target_mismatch", "session_mismatch", "frame_mismatch", "document_mismatch", "context_mismatch", "lifecycle_unready", "windows_scope", "parent_changed", "chrome_changed", "image_changed", "node_parent", "chrome_parent", "job_unavailable", "job_changed", "directory_changed", "collector_not_headless"},
}

func (v nativePilotInitialTrace) values() [19]string {
	return [19]string{v.firstCause, v.terminal, v.early, v.handoff, v.cookie, v.extra, v.guardTrigger, v.guardProvenance, v.guardCondition, v.guardEpoch, v.guardDiskCache, v.guardServiceWorker, v.guardPrefetchCache, v.guardRequestFrame, v.guardHeaderReason, v.inspectStage, v.inspectCondition, v.prepareStage, v.prepareCondition}
}
func nativePilotTraceValues(v [19]string) nativePilotInitialTrace {
	return nativePilotInitialTrace{v[0], v[1], v[2], v[3], v[4], v[5], v[6], v[7], v[8], v[9], v[10], v[11], v[12], v[13], v[14], v[15], v[16], v[17], v[18]}
}
func (v nativePilotInitialTrace) closed() nativePilotInitialTrace {
	values := v.values()
	for i, value := range values {
		values[i] = "unknown"
		for _, known := range nativePilotTraceCatalogs[i] {
			if value == known {
				values[i] = value
				break
			}
		}
	}
	return nativePilotTraceValues(values)
}
func (v nativePilotInitialTrace) String() string {
	v = v.closed()
	result := fmt.Sprintf("first=%s terminal=%s early=%s handoff=%s cookie=%s extra=%s trigger=%s provenance=%s condition=%s epoch=%s disk_cache=%s service_worker=%s prefetch_cache=%s request_frame=%s header_reason=%s inspect_stage=%s inspect_condition=%s prepare_stage=%s prepare_condition=%s", v.firstCause, v.terminal, v.early, v.handoff, v.cookie, v.extra, v.guardTrigger, v.guardProvenance, v.guardCondition, v.guardEpoch, v.guardDiskCache, v.guardServiceWorker, v.guardPrefetchCache, v.guardRequestFrame, v.guardHeaderReason, v.inspectStage, v.inspectCondition, v.prepareStage, v.prepareCondition)
	if strings.HasPrefix(v.guardCondition, "root_control_") {
		return result + " domain=control"
	}
	switch v.guardCondition {
	case "root_response_disk_cache", "root_response_service_worker", "root_response_prefetch_cache", "root_served_from_cache":
		return result + " domain=root_policy"
	}
	if strings.HasPrefix(v.guardCondition, "root_") {
		return result + " domain=root_structure"
	}
	switch v.firstCause {
	case "preparation":
		return result + " domain=preparation"
	case "inspection":
		return result + " domain=inspection"
	case "cookie_candidate", "cookie_ambiguous", "guard_header_classification":
		return result + " domain=header"
	case "guard_request_budget", "guard_header_budget", "guard_event_budget", "guard_metadata_shape":
		return result + " domain=coverage"
	case "context_lost", "target_lost", "navigation", "owner_scope", "event_cap":
		return result + " domain=lifecycle"
	case "identity_order", "identity_mismatch", "stale_observation":
		return result + " domain=identity"
	case "settings_order", "response_frame", "extra_shape", "extra_status", "extra_missing", "extra_unexpected", "settings_capture":
		return result + " domain=settings"
	case "cleanup_failed":
		return result + " domain=cleanup"
	}
	return result + " domain=unknown"
}
func (v nativePilotInitialTrace) GoString() string { return v.String() }

type nativePilotInitialError struct{ detail nativePilotInitialDetail }

var nativePilotInitialRoles = []string{"reader", "collector", "pair", "unknown"}
var nativePilotInitialPhases = []string{"prepare", "inspect_before", "inspect_after", "acquire_rpc", "identity", "rendezvous", "settings", "cleanup", "bound_finish", "wire_decode", "joint", "navigation", "page_ready", "guard_bind"}
var nativePilotInitialReasons = []string{"unavailable", "rejected", "identity_unavailable", "source_unavailable", "stale_observation", "cleanup_unverified", "scope_lost", "observed_cookie_write", "deadline", "canceled", "replay", "sample_missing", "sample_duplicate", "invalid_shape", "scope_mismatch", "time_invalid"}

func (e *nativePilotInitialError) Error() string    { return "startup-projection-rejected" }
func (e *nativePilotInitialError) GoString() string { return e.Error() }
func (d nativePilotInitialDetail) closed() nativePilotInitialDetail {
	has := func(values []string, v string) bool {
		for _, value := range values {
			if value == v {
				return true
			}
		}
		return false
	}
	if !has(nativePilotInitialRoles, d.role) || !has(nativePilotInitialPhases, d.phase) || !has(nativePilotInitialReasons, d.reason) {
		return nativePilotInitialDetail{role: "unknown", phase: "acquire_rpc", reason: "unavailable"}
	}
	if d.trace != nil {
		safe := d.trace.closed()
		d.trace = &safe
	}
	return d
}
func (d nativePilotInitialDetail) String() string {
	d = d.closed()
	value := fmt.Sprintf("role=%s phase=%s reason=%s kind=%s", d.role, d.phase, d.reason, map[bool]string{false: "primary", true: "peer"}[d.peer])
	if d.trace != nil {
		value += " " + d.trace.String()
	}
	if d.timing != nil {
		value += d.timing.String()
	}
	return value
}
func (d nativePilotInitialDetail) GoString() string { return d.String() }
func nativePilotInitialWrap(err error, role, phase string) error {
	var typed *nativePilotInitialError
	if errors.As(err, &typed) {
		if typed == nil {
			return &nativePilotInitialError{nativePilotInitialDetail{role: "unknown", phase: "acquire_rpc", reason: "unavailable"}}
		}
		return &nativePilotInitialError{typed.detail.closed()}
	}
	reason := "unavailable"
	if errors.Is(err, context.Canceled) {
		role, phase, reason = "pair", "joint", "canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		role, phase, reason = "pair", "joint", "deadline"
	}
	return &nativePilotInitialError{(nativePilotInitialDetail{role: role, phase: phase, reason: reason}).closed()}
}

// Preparation metadata is diagnostic only. Earlier branded causes and joint
// cancellation/deadline retain their original origin and semantics.
func nativePilotPrepareFailure(err error, role, stage, condition string) error {
	var typed *nativePilotInitialError
	if errors.As(err, &typed) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return nativePilotInitialWrap(err, role, "prepare")
	}
	trace := nativePilotInitialTrace{firstCause: "preparation", prepareStage: stage, prepareCondition: condition}
	return &nativePilotInitialError{(nativePilotInitialDetail{role: role, phase: "prepare", reason: "unavailable", trace: &trace}).closed()}
}
func nativePilotInitialCode(code string) (nativePilotInitialDetail, bool) {
	if strings.HasPrefix(code, "manual_initial_v2_") || strings.HasPrefix(code, "manual_initial_v3_") || strings.HasPrefix(code, "manual_initial_v4_") || strings.HasPrefix(code, "manual_initial_v5_") || strings.HasPrefix(code, "manual_initial_v6_") || strings.HasPrefix(code, "manual_initial_v7_") || strings.HasPrefix(code, "manual_initial_v8_") || strings.HasPrefix(code, "manual_initial_v9_") {
		if len(code) > 80 {
			return nativePilotInitialDetail{}, false
		}
		prefix, traceCount := "manual_initial_v4_", 10
		if strings.HasPrefix(code, "manual_initial_v2_") {
			prefix, traceCount = "manual_initial_v2_", 6
		} else if strings.HasPrefix(code, "manual_initial_v3_") {
			prefix, traceCount = "manual_initial_v3_", 8
		} else if strings.HasPrefix(code, "manual_initial_v5_") {
			prefix, traceCount = "manual_initial_v5_", 13
		} else if strings.HasPrefix(code, "manual_initial_v6_") {
			prefix, traceCount = "manual_initial_v6_", 14
		} else if strings.HasPrefix(code, "manual_initial_v7_") {
			prefix, traceCount = "manual_initial_v7_", 15
		} else if strings.HasPrefix(code, "manual_initial_v8_") {
			prefix, traceCount = "manual_initial_v8_", 17
		} else if strings.HasPrefix(code, "manual_initial_v9_") {
			prefix, traceCount = "manual_initial_v9_", 19
		}
		parts := strings.Split(strings.TrimPrefix(code, prefix), "_")
		if len(parts) != 4+traceCount {
			return nativePilotInitialDetail{}, false
		}
		catalogs := [][]string{nativePilotInitialRoles, nativePilotInitialPhases, nativePilotInitialReasons, {"primary", "peer"}}
		catalogs = append(catalogs, nativePilotTraceCatalogs[:traceCount]...)
		values := make([]string, 23)
		for i := range values {
			values[i] = "unknown"
		}
		for i, part := range parts {
			n, err := strconv.Atoi(part)
			if err != nil || n < 0 || n >= len(catalogs[i]) || strconv.Itoa(n) != part {
				return nativePilotInitialDetail{}, false
			}
			values[i] = catalogs[i][n]
		}
		trace := nativePilotTraceValues([19]string{values[4], values[5], values[6], values[7], values[8], values[9], values[10], values[11], values[12], values[13], values[14], values[15], values[16], values[17], values[18], values[19], values[20], values[21], values[22]})
		return nativePilotInitialDetail{role: values[0], phase: values[1], reason: values[2], peer: values[3] == "peer", trace: &trace}, true
	}
	if len(code) <= 96 {
		for _, role := range nativePilotInitialRoles {
			for _, phase := range nativePilotInitialPhases {
				for _, reason := range nativePilotInitialReasons {
					for _, kind := range []string{"primary", "peer"} {
						if code == "manual_initial_"+role+"_"+phase+"_"+reason+"_"+kind {
							return nativePilotInitialDetail{role: role, phase: phase, reason: reason, peer: kind == "peer"}, true
						}
					}
				}
			}
		}
	}
	return nativePilotInitialDetail{}, false
}
func nativePilotInitialRPC(code string) error {
	if detail, ok := nativePilotInitialCode(code); ok {
		return &nativePilotInitialError{detail}
	}
	return nativePilotInitialWrap(nil, "unknown", "acquire_rpc")
}
func nativePilotInitialOf(err error, role, phase string) nativePilotInitialDetail {
	return nativePilotInitialWrap(err, role, phase).(*nativePilotInitialError).detail
}

type nativePilotSeal struct{ marker byte }
type nativePilotVector struct {
	seal       *nativePilotSeal
	version    uint64
	expires    time.Time
	binding    nativePilotBinding
	provenance nativePilotProvenance
}

type nativePilotRequestKey struct {
	owner   nativePilotOwner
	request string
}
type nativePilotAdapter struct {
	mu                       sync.Mutex
	cfg                      nativePilotConfig
	ports                    nativePilotPorts
	seal                     *nativePilotSeal
	last                     time.Time
	version                  uint64
	busy, attempted, stopped bool
	reason                   string
	initial                  nativePilotInitialDetail
	startupTiming            *nativePilotTimingSnapshot
	current                  *nativePilotVector
	seen                     map[nativePilotRequestKey]bool
}

// Fixed formatting prevents accidental diagnostic formatting of private values.
func (nativePilotOwner) String() string         { return "private-native-owner" }
func (nativePilotBinding) String() string       { return "private-native-binding" }
func (nativePilotState) String() string         { return "private-native-state" }
func (nativePilotConfig) String() string        { return "private-native-config" }
func (nativePilotChallenge) String() string     { return "private-native-challenge" }
func (nativePilotResponse) String() string      { return "private-native-response" }
func (nativePilotReceipt) String() string       { return "private-native-receipt" }
func (*nativePilotVector) String() string       { return "private-native-vector" }
func (*nativePilotAdapter) String() string      { return "private-native-adapter" }
func (v nativePilotOwner) GoString() string     { return v.String() }
func (v nativePilotBinding) GoString() string   { return v.String() }
func (v nativePilotState) GoString() string     { return v.String() }
func (v nativePilotConfig) GoString() string    { return v.String() }
func (v nativePilotChallenge) GoString() string { return v.String() }
func (v nativePilotResponse) GoString() string  { return v.String() }
func (v nativePilotReceipt) GoString() string   { return v.String() }
func (v *nativePilotVector) GoString() string   { return v.String() }
func (v *nativePilotAdapter) GoString() string  { return v.String() }

func newNativePilotAdapter(cfg nativePilotConfig, ports nativePilotPorts) (*nativePilotAdapter, nativePilotDiagnostic) {
	a := &nativePilotAdapter{cfg: cfg, ports: ports, seal: &nativePilotSeal{}, seen: make(map[nativePilotRequestKey]bool)}
	if !cfg.enabled {
		return nil, nativePilotDiagnostic{Reason: "admission-disabled"}
	}
	if cfg.contract != nativePilotContract || !cfg.limitationAcknowledged || !validNativePilotBinding(cfg.binding) || cfg.attemptStarted.IsZero() || ports.now == nil || ports.owners == nil || !cfg.attemptValidUntil.IsZero() && !cfg.attemptValidUntil.After(cfg.attemptStarted) {
		return nil, nativePilotDiagnostic{Reason: "invalid-configuration"}
	}
	a.last = cfg.attemptStarted
	return a, nativePilotDiagnostic{Reason: "admission-closed"}
}

func validNativePilotOwner(o nativePilotOwner, role string) bool {
	if o.role != role || o.pid <= 0 || o.generation == 0 || !nativePilotRef(o.directory, 4096) {
		return false
	}
	for _, s := range []string{o.owner, o.profile, o.image, o.creation, o.job, o.parent, o.context, o.target, o.frame, o.document} {
		if !nativePilotRef(s, 512) {
			return false
		}
	}
	return true
}
func validNativePilotBinding(b nativePilotBinding) bool {
	r, c := b.reader, b.collector
	for _, s := range []string{b.enrollment, b.pair, b.attempt, b.derivation} {
		if !nativePilotRef(s, 512) {
			return false
		}
	}
	// CDP context IDs are scoped to their pinned process/session. Equal lexical
	// values in distinct owned processes do not merge those contexts. Per-role
	// receipt validation still compares the entire owner/process/context tuple.
	return nativePilotAccount(b.account) && b.policy > 0 && b.consent > 0 && validNativePilotOwner(r, "reader") && validNativePilotOwner(c, "collector") && r.owner != c.owner && r.profile != c.profile && r.directory != c.directory && r.pid != c.pid && r.parent == c.parent
}
func nativePilotAccount(s string) bool {
	if len(s) < 1 || len(s) > 20 || s[0] < '1' || s[0] > '9' {
		return false
	}
	for i := 1; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
func nativePilotRef(s string, max int) bool {
	if len(s) == 0 || len(s) > max {
		return false
	}
	for _, c := range s {
		if c < 32 || c == 127 {
			return false
		}
	}
	return true
}

func (a *nativePilotAdapter) stopLocked(reason string) nativePilotDiagnostic {
	a.stopped, a.reason, a.current = true, reason, nil
	return nativePilotDiagnostic{Reason: reason}
}
func (a *nativePilotAdapter) checkLocked(ctx context.Context) (time.Time, string) {
	if a.stopped {
		return time.Time{}, a.reason
	}
	if ctx == nil || ctx.Err() != nil {
		return time.Time{}, "canceled"
	}
	now := a.ports.now()
	if now.Before(a.last) {
		return now, "clock-regressed"
	}
	a.last = now
	if !now.Before(a.attemptDeadline()) {
		return now, "attempt-expired"
	}
	if a.current != nil && !now.Before(a.current.expires) {
		return now, "evidence-expired"
	}
	return now, ""
}
func (a *nativePilotAdapter) ownerReason(ctx context.Context) string {
	s, err := a.ports.owners(ctx)
	if err != nil {
		return "owner-unavailable"
	}
	if !s.optedIn {
		return "opted-out"
	}
	if !s.parentAlive {
		return "parent-exited"
	}
	if !s.channelHealthy || !s.invalidationsReady {
		return "evidence-channel-lost"
	}
	if !s.completeInventory || s.readerWindows < 1 {
		return "reader-lost"
	}
	if !s.ownersAlive || s.binding != a.cfg.binding {
		return "owner-or-policy-changed"
	}
	return ""
}

func (a *nativePilotAdapter) attemptDeadline() time.Time {
	end := a.cfg.attemptStarted.Add(nativePilotLife)
	if !a.cfg.attemptValidUntil.IsZero() && a.cfg.attemptValidUntil.Before(end) {
		end = a.cfg.attemptValidUntil
	}
	return end
}

// initialEvidence serializes one fresh startup vector and cannot renew, reseed
// or restart the attempt. Sample validity constrains initial acceptance only;
// the manual attempt then expires at its separately pinned absolute horizon.
// Ports execute without the state mutex. They must honor context cancellation;
// arbitrary Go callbacks cannot be forcibly interrupted. Publication is fenced.
func (a *nativePilotAdapter) initialEvidence(ctx context.Context) (*nativePilotVector, nativePilotDiagnostic) {
	a.mu.Lock()
	started, reason := a.checkLocked(ctx)
	if reason != "" {
		d := a.stopLocked(reason)
		a.mu.Unlock()
		return nil, d
	}
	if a.busy {
		a.mu.Unlock()
		return nil, nativePilotDiagnostic{Reason: "acquisition-busy"}
	}
	if a.attempted {
		a.mu.Unlock()
		return nil, nativePilotDiagnostic{Reason: "startup-already-attempted"}
	}
	a.busy, a.attempted = true, true
	version := uint64(1)
	a.mu.Unlock()
	defer func() { a.mu.Lock(); a.busy = false; a.mu.Unlock() }()
	provenance := nativePilotUnqualified
	if a.ports.source != nil {
		provenance = a.ports.source.provenance()
	}
	if provenance != nativePilotTrustedStartup && provenance != nativePilotOfflineOnly {
		a.mu.Lock()
		defer a.mu.Unlock()
		return nil, a.stopLocked("identity-unavailable")
	}
	deadline := started.Add(nativePilotStartup)
	if absolute := a.attemptDeadline(); absolute.Before(deadline) {
		deadline = absolute
	}
	// Bound real callback time independently of the injected policy clock.
	probeCtx, cancel := context.WithTimeout(ctx, deadline.Sub(started))
	defer cancel()
	timing := &nativePilotTiming{now: time.Now, started: time.Now()}
	probeCtx = context.WithValue(probeCtx, nativePilotTimingKey{}, timing)
	nativePilotFlowTrace(probeCtx, "pair", "startup_enter", "start", started, deadline, nil)
	defer func() {
		snapshot := timing.freeze()
		timing.mu.Lock()
		first := timing.first
		timing.mu.Unlock()
		a.mu.Lock()
		defer a.mu.Unlock()
		a.startupTiming = snapshot
		if a.stopped {
			if first != nil {
				a.initial = *first
			}
			if a.initial.role == "" {
				a.initial = nativePilotInitialOf(probeCtx.Err(), "pair", "joint")
			}
			a.initial.timing = snapshot
		}
	}()
	nativePilotFlowTrace(probeCtx, "pair", "owners_before_enter", "start", a.ports.now(), deadline, nil)
	reason = a.ownerReason(probeCtx)
	ownerOutcome := "accepted"
	if reason != "" {
		ownerOutcome = "owner_error"
	}
	nativePilotFlowTrace(probeCtx, "pair", "owners_before_exit", ownerOutcome, a.ports.now(), deadline, nil)
	sampleExpires := a.attemptDeadline()
	limitingRole := "pair"
	var limitingReceipt *nativePilotReceipt
	requests := make(map[nativePilotRequestKey]bool)
	for _, owner := range []nativePilotOwner{a.cfg.binding.reader, a.cfg.binding.collector} {
		if reason != "" {
			break
		}
		c := nativePilotChallenge{seal: a.seal, vector: version, role: owner.role, binding: a.cfg.binding, owner: owner, started: started.Round(0)}
		nativePilotFlowTrace(probeCtx, owner.role, "receipt_enter", "start", a.ports.now(), deadline, nil)
		receipt, err := a.ports.source.observe(probeCtx, c)
		if err != nil {
			nativePilotFlowTrace(probeCtx, owner.role, "receipt_exit", "source_error", a.ports.now(), deadline, nil)
			a.mu.Lock()
			if a.initial.role == "" {
				a.initial = nativePilotInitialOf(err, "unknown", "acquire_rpc")
			}
			a.mu.Unlock()
			reason = "source-unavailable"
			break
		}
		a.mu.Lock()
		now, invalid := a.checkLocked(probeCtx)
		nativePilotFlowTrace(probeCtx, owner.role, "receipt_exit", "received", now, deadline, &receipt)
		if invalid != "" {
			reason = invalid
		} else if !now.Before(deadline) {
			reason = "joint-deadline-exceeded"
		}
		if reason == "" {
			for i, response := range []nativePilotResponse{receipt.identity, receipt.settings} {
				failure := a.validateResponse(c, response, i == 0, now, requests)
				stage, condition := "identity_check", "accepted"
				if i != 0 {
					stage = "settings_check"
				}
				if failure != "" {
					condition = "receipt_invalid"
					if temporal := nativePilotSampleTimeFailure(c, response, now); temporal != "" {
						condition = temporal
					}
				}
				nativePilotFlowTrace(probeCtx, owner.role, stage, condition, now, deadline, &receipt)
				if failure != "" {
					reason = failure
					break
				}
				for _, limit := range []time.Time{response.validUntil.Round(0), response.observed.Round(0).Add(nativePilotAge)} {
					if limit.Before(sampleExpires) {
						sampleExpires = limit
						limitingRole = owner.role
						copy := receipt
						limitingReceipt = &copy
					}
				}
			}
		}
		a.mu.Unlock()
	}
	if reason == "" {
		// The composed source acquires both roles together and buffers the second
		// receipt. Recheck the whole pair once before publication, rather than
		// repeating native inspection between already-buffered receipts. Each
		// owner port still checks its own scope before and after acquisition.
		nativePilotFlowTrace(probeCtx, "pair", "owners_after_enter", "start", a.ports.now(), deadline, nil)
		reason = a.ownerReason(probeCtx)
		ownerOutcome = "accepted"
		if reason != "" {
			ownerOutcome = "owner_error"
		}
		nativePilotFlowTrace(probeCtx, "pair", "owners_after_exit", ownerOutcome, a.ports.now(), deadline, nil)
	}
	if reason == "" && a.ports.source.provenance() != provenance {
		reason = "source-unavailable"
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	now, invalid := a.checkLocked(probeCtx)
	if invalid != "" {
		reason = invalid
	}
	publication := "accepted"
	if reason == "" {
		if !now.Before(deadline) {
			reason, publication = "evidence-expired", "startup_deadline"
		} else if !now.Round(0).Before(sampleExpires) {
			reason, publication = "evidence-expired", "sample_horizon_expired"
		}
	} else {
		publication = "rejected"
	}
	nativePilotFlowTrace(probeCtx, limitingRole, "publication", publication, now, deadline, limitingReceipt)
	if reason != "" {
		return nil, a.stopLocked(reason)
	}
	for key := range requests {
		a.seen[key] = true
	}
	a.version = version
	a.current = &nativePilotVector{seal: a.seal, version: version, expires: a.attemptDeadline(), binding: a.cfg.binding, provenance: provenance}
	return a.current, nativePilotResult("startup-validated", provenance)
}

func nativePilotSampleTimeFailure(c nativePilotChallenge, r nativePilotResponse, now time.Time) string {
	observed, until, wall := r.observed.Round(0), r.validUntil.Round(0), now.Round(0)
	if observed.Before(c.started) {
		return "timestamp_before_start"
	}
	if observed.After(wall) {
		return "timestamp_future"
	}
	if !until.After(observed) {
		return "validity_order"
	}
	if !wall.Before(observed.Add(nativePilotAge)) {
		return "age_expired"
	}
	if !wall.Before(until) {
		return "source_expired"
	}
	return ""
}
func (a *nativePilotAdapter) validateResponse(c nativePilotChallenge, r nativePilotResponse, identity bool, now time.Time, requests map[nativePilotRequestKey]bool) string {
	if r.challenge != c || !nativePilotRef(r.request, 512) || !r.fresh || !r.direct || !r.wellFormed || !r.clockValid || r.elapsed < 0 || r.elapsed >= nativePilotStartup {
		return "evidence-invalid"
	}
	key := nativePilotRequestKey{owner: c.owner, request: r.request}
	if a.seen[key] || requests[key] {
		return "request-replayed"
	}
	requests[key] = true
	if nativePilotSampleTimeFailure(c, r, now) != "" {
		return "evidence-expired"
	}
	if identity {
		if r.source != nativePilotStartupIdentity || r.method != "" || r.url != "" || r.qualification != "" || !r.currentSelf || !r.semanticSuccess || !nativePilotAccount(r.account) {
			return "identity-unavailable"
		}
		if r.account != a.cfg.binding.account {
			return "account-mismatch"
		}
	} else if r.source != "" || r.account != "" || r.currentSelf || r.method != "GET" || r.url != nativePilotSettingsURL || r.qualification != nativePilotSettingsQualification || !r.semanticSuccess {
		return "authentication-rejected"
	}
	return ""
}

// valid is a future controller seam, not an authorization interface. Initial
// acquisition is one-use; repeated use checks neither refresh nor resample it.
// A future consumer MUST reject offline provenance for runtime admission.
func (a *nativePilotAdapter) valid(ctx context.Context, vector *nativePilotVector) nativePilotDiagnostic {
	a.mu.Lock()
	_, reason := a.checkLocked(ctx)
	a.mu.Unlock()
	if reason == "" {
		reason = a.ownerReason(ctx)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	_, invalid := a.checkLocked(ctx)
	if invalid != "" {
		reason = invalid
	}
	if reason != "" {
		return a.stopLocked(reason)
	}
	if vector == nil || vector != a.current || vector.seal != a.seal || vector.binding != a.cfg.binding || vector.version != a.version {
		return nativePilotDiagnostic{Reason: "stale-vector"}
	}
	return nativePilotResult("manual-attempt-valid", vector.provenance)
}

func nativePilotResult(reason string, provenance nativePilotProvenance) nativePilotDiagnostic {
	return nativePilotDiagnostic{Reason: reason, OfflineValidated: provenance == nativePilotOfflineOnly, ManualEligible: provenance == nativePilotTrustedStartup}
}

// Event arguments are intentionally closed; caller error text is never exposed.
func (a *nativePilotAdapter) invalidate(event string) nativePilotDiagnostic {
	a.mu.Lock()
	defer a.mu.Unlock()
	switch event {
	case "opted-out", "reader-lost", "parent-exited", "owner-or-policy-changed", "evidence-channel-lost", "candidate-cookie-write", "canceled", "identity-mismatch", "authentication-rejected", "cleanup-uncertain":
	default:
		event = "evidence-invalid"
	}
	if a.stopped {
		return nativePilotDiagnostic{Reason: a.reason}
	}
	return a.stopLocked(event)
}
