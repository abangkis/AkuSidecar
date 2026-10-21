//go:build auth_session_dev

package engine

// This boundary validates private receipts; it grants no controller, native or
// Store authority. No native startup producer is registered here. Startup
// sampling does not establish fresh identity or session continuity afterward.
import (
	"context"
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
	return nativePilotAccount(b.account) && b.policy > 0 && b.consent > 0 && validNativePilotOwner(r, "reader") && validNativePilotOwner(c, "collector") && r.owner != c.owner && r.profile != c.profile && r.directory != c.directory && r.pid != c.pid && r.context != c.context && r.parent == c.parent
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
