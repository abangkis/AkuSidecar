//go:build auth_session_dev

package engine

// This boundary validates private receipts; it grants no controller, native or
// Store authority. No native startup producer is registered here. Startup
// sampling does not establish fresh identity or session continuity afterward.
import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	nativePilotContract              = "aku.x.dev-pilot.manual-startup.v2"
	nativePilotAge                   = 5 * time.Second
	nativePilotJoint                 = 2 * time.Second
	nativePilotLife                  = 60 * time.Second
	nativePilotSettingsURL           = "https://api.x.com/1.1/account/settings.json"
	nativePilotSettingsQualification = "api_v1.cookie-auth-rejection.2026-10-20"
	nativePilotStartupIdentity       = "bootstrap-current-self.startup.v1"
)

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

// Private detail only. It never changes public Reason, eligibility or permits.
type nativePilotInitialDetail struct {
	role, phase, reason string
	peer                bool
	trace               *nativePilotInitialTrace
}
type nativePilotInitialTrace struct{ firstCause, terminal, early, handoff, cookie, extra, guardTrigger, guardProvenance, guardCondition, guardEpoch, guardDiskCache, guardServiceWorker, guardPrefetchCache, guardRequestFrame, guardHeaderReason string }

// Append-only V2 catalogs with the V3/V4/V5/V6/V7 guard fields; order matches STARTUP_TRACE in Node.
var nativePilotTraceCatalogs = [][]string{
	{"unknown", "cookie_candidate", "cookie_ambiguous", "context_lost", "target_lost", "navigation", "owner_scope", "event_cap", "identity_order", "identity_mismatch", "settings_order", "response_frame", "extra_shape", "extra_status", "extra_missing", "extra_unexpected", "generic_early", "stale_observation", "cleanup_failed", "guard_request_unbound", "guard_request_budget", "guard_header_classification", "guard_response_correlation", "guard_metadata_shape", "guard_completion_missing", "guard_header_budget", "guard_event_budget", "root_control"},
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
}

func (v nativePilotInitialTrace) values() [15]string {
	return [15]string{v.firstCause, v.terminal, v.early, v.handoff, v.cookie, v.extra, v.guardTrigger, v.guardProvenance, v.guardCondition, v.guardEpoch, v.guardDiskCache, v.guardServiceWorker, v.guardPrefetchCache, v.guardRequestFrame, v.guardHeaderReason}
}
func nativePilotTraceValues(v [15]string) nativePilotInitialTrace {
	return nativePilotInitialTrace{v[0], v[1], v[2], v[3], v[4], v[5], v[6], v[7], v[8], v[9], v[10], v[11], v[12], v[13], v[14]}
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
	result := fmt.Sprintf("first=%s terminal=%s early=%s handoff=%s cookie=%s extra=%s trigger=%s provenance=%s condition=%s epoch=%s disk_cache=%s service_worker=%s prefetch_cache=%s request_frame=%s header_reason=%s", v.firstCause, v.terminal, v.early, v.handoff, v.cookie, v.extra, v.guardTrigger, v.guardProvenance, v.guardCondition, v.guardEpoch, v.guardDiskCache, v.guardServiceWorker, v.guardPrefetchCache, v.guardRequestFrame, v.guardHeaderReason)
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
	case "cookie_candidate", "cookie_ambiguous", "guard_header_classification":
		return result + " domain=header"
	case "guard_request_budget", "guard_header_budget", "guard_event_budget", "guard_metadata_shape":
		return result + " domain=coverage"
	case "context_lost", "target_lost", "navigation", "owner_scope", "event_cap":
		return result + " domain=lifecycle"
	case "identity_order", "identity_mismatch", "stale_observation":
		return result + " domain=identity"
	case "settings_order", "response_frame", "extra_shape", "extra_status", "extra_missing", "extra_unexpected":
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
func nativePilotInitialCode(code string) (nativePilotInitialDetail, bool) {
	if strings.HasPrefix(code, "manual_initial_v2_") || strings.HasPrefix(code, "manual_initial_v3_") || strings.HasPrefix(code, "manual_initial_v4_") || strings.HasPrefix(code, "manual_initial_v5_") || strings.HasPrefix(code, "manual_initial_v6_") || strings.HasPrefix(code, "manual_initial_v7_") {
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
		}
		parts := strings.Split(strings.TrimPrefix(code, prefix), "_")
		if len(parts) != 4+traceCount {
			return nativePilotInitialDetail{}, false
		}
		catalogs := [][]string{nativePilotInitialRoles, nativePilotInitialPhases, nativePilotInitialReasons, {"primary", "peer"}}
		catalogs = append(catalogs, nativePilotTraceCatalogs[:traceCount]...)
		values := make([]string, 19)
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
		trace := nativePilotTraceValues([15]string{values[4], values[5], values[6], values[7], values[8], values[9], values[10], values[11], values[12], values[13], values[14], values[15], values[16], values[17], values[18]})
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
	deadline := started.Add(nativePilotJoint)
	if absolute := a.attemptDeadline(); absolute.Before(deadline) {
		deadline = absolute
	}
	// Bound real callback time independently of the injected policy clock.
	probeCtx, cancel := context.WithTimeout(ctx, deadline.Sub(started))
	defer cancel()
	reason = a.ownerReason(probeCtx)
	sampleExpires := a.attemptDeadline()
	requests := make(map[nativePilotRequestKey]bool)
	for _, owner := range []nativePilotOwner{a.cfg.binding.reader, a.cfg.binding.collector} {
		if reason != "" {
			break
		}
		c := nativePilotChallenge{seal: a.seal, vector: version, role: owner.role, binding: a.cfg.binding, owner: owner, started: started.Round(0)}
		receipt, err := a.ports.source.observe(probeCtx, c)
		if err != nil {
			a.mu.Lock()
			if a.initial.role == "" {
				a.initial = nativePilotInitialOf(err, "unknown", "acquire_rpc")
			}
			a.mu.Unlock()
			reason = "source-unavailable"
			break
		}
		if drift := a.ownerReason(probeCtx); drift != "" {
			reason = drift
			break
		}
		a.mu.Lock()
		now, invalid := a.checkLocked(probeCtx)
		if invalid != "" {
			reason = invalid
		} else if !now.Before(deadline) {
			reason = "joint-deadline-exceeded"
		}
		if reason == "" {
			for i, response := range []nativePilotResponse{receipt.identity, receipt.settings} {
				if failure := a.validateResponse(c, response, i == 0, now, requests); failure != "" {
					reason = failure
					break
				}
				for _, limit := range []time.Time{response.validUntil.Round(0), response.observed.Round(0).Add(nativePilotAge)} {
					if limit.Before(sampleExpires) {
						sampleExpires = limit
					}
				}
			}
		}
		a.mu.Unlock()
	}
	if reason == "" {
		reason = a.ownerReason(probeCtx)
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
	if reason == "" && (!now.Before(deadline) || !now.Round(0).Before(sampleExpires)) {
		reason = "evidence-expired"
	}
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

func (a *nativePilotAdapter) validateResponse(c nativePilotChallenge, r nativePilotResponse, identity bool, now time.Time, requests map[nativePilotRequestKey]bool) string {
	if r.challenge != c || !nativePilotRef(r.request, 512) || !r.fresh || !r.direct || !r.wellFormed || !r.clockValid || r.elapsed < 0 || r.elapsed >= nativePilotJoint {
		return "evidence-invalid"
	}
	key := nativePilotRequestKey{owner: c.owner, request: r.request}
	if a.seen[key] || requests[key] {
		return "request-replayed"
	}
	requests[key] = true
	observed, until, wall := r.observed.Round(0), r.validUntil.Round(0), now.Round(0)
	if observed.Before(c.started) || observed.After(wall) || !until.After(observed) || !wall.Before(until) || !wall.Before(observed.Add(nativePilotAge)) {
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
