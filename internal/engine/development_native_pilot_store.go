//go:build auth_session_dev

package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/abangkis/AkuSidecar/internal/collection/headless"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/store"
)

// This composition has no main/HTTP/native-provider registration. Only a trusted
// private Go caller can attach it after startup. Serialized diagnostics cannot
// select this route. Offline fixtures keep their existing non-Store route.
type nativePilotStoreRuntime struct {
	mu          sync.Mutex
	engine      *Engine
	runtime     *nativePilotRuntime
	permit      *store.ManualPilotPermit
	closed      bool
	revokeID    string
	work        map[string]*nativePilotStoreWork
	afterCommit func(context.Context, domain.Run) (domain.Run, error)
	admission   nativePilotStoreAdmission
}

// Implementations are private trusted Go composition. The ordinary implementation
// requires native startup; an offline Chrome implementation exists only in tests.
type nativePilotStoreAdmission interface {
	validate(context.Context) (store.DevelopmentManualPilotBinding, error)
	issue(context.Context, string, string) (*store.ManualPilotPermit, error)
}
type nativePilotNativeStoreAdmission struct {
	broker *nativePilotBroker
	store  *store.Store
}

func (a nativePilotNativeStoreAdmission) validate(ctx context.Context) (store.DevelopmentManualPilotBinding, error) {
	return a.broker.storeStartup(ctx)
}
func (a nativePilotNativeStoreAdmission) issue(ctx context.Context, session, run string) (*store.ManualPilotPermit, error) {
	return a.store.NewDevelopmentManualPilotIssuer(a.validate)(ctx, session, run)
}

type nativePilotStoreWork struct {
	result nativePilotStoreResult
	done   bool
}
type nativePilotStoreResult struct {
	// Outcome describes result persistence only. A rejected result may follow
	// a durable claim; it is never an instruction to recapture.
	Outcome                         store.ManualPilotOutcome
	ClaimOutcome                    store.ManualPilotOutcome
	Phase                           string // Closed local stage; never derived from peer text.
	CaptureDiagnostic               nativePilotCaptureDiagnostic
	Run                             domain.Run
	PersistenceError, DeliveryError error
}

// Encoding an explicit ordered array avoids fmt/Stringer redaction and binds
// every private field, including physical owner pins and document identity.
func nativePilotStoreDigest(v nativePilotBinding) string {
	owner := func(o nativePilotOwner) any {
		return []any{o.role, o.owner, o.profile, o.directory, o.image, o.creation, o.job, o.parent, o.context, o.target, o.frame, o.document, o.pid, o.generation}
	}
	raw, _ := json.Marshal([]any{v.account, v.enrollment, v.pair, v.attempt, v.derivation, v.policy, v.consent, owner(v.reader), owner(v.collector)})
	h := sha256.Sum256(raw)
	clear(raw)
	return hex.EncodeToString(h[:])
}

func (b *nativePilotBroker) storeStartup(ctx context.Context) (store.DevelopmentManualPilotBinding, error) {
	var result store.DevelopmentManualPilotBinding
	// check validates the actual vector and owner state; none of its public
	// diagnostic flags is an issuance proof.
	b.check(ctx)
	b.mu.Lock()
	defer b.mu.Unlock()
	c := b.controller
	if b.stopped || c == nil || b.cfg.offlineTest {
		return result, errors.New("trusted startup unavailable")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	v := c.vector
	if c.stopped || v == nil || c.offlineTest || v.provenance != nativePilotTrustedStartup || v.seal != c.adapter.seal || v.binding != c.adapter.cfg.binding {
		return result, errors.New("trusted startup vector rejected")
	}
	end := b.deadline()
	if v.expires.Before(end) {
		end = v.expires
	}
	return store.DevelopmentManualPilotBinding{Digest: nativePilotStoreDigest(v.binding), Attempt: v.binding.attempt, OwnerGeneration: v.binding.collector.generation, StartedAt: b.cfg.pilot.attemptStarted, Deadline: end}, nil
}

func (l *nativePilotStoreRuntime) close() {
	l.mu.Lock()
	l.closed = true
	p := l.permit
	l.mu.Unlock()
	if p != nil {
		// Revoke closes the atomic permit first, before waiting for SQLite.
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_, _ = l.engine.store.RevokeManualPilot(ctx, p, l.revokeID)
	}
}

func (e *Engine) bindNativePilotStore(ctx context.Context, r *nativePilotRuntime, sessionID, runID string) error {
	if e == nil || r == nil {
		return errors.New("manual Store wiring unavailable")
	}
	return e.bindNativePilotStoreAdmission(ctx, r, sessionID, runID, nativePilotNativeStoreAdmission{broker: r.broker, store: e.store})
}
func (e *Engine) bindNativePilotStoreAdmission(ctx context.Context, r *nativePilotRuntime, sessionID, runID string, admission nativePilotStoreAdmission) error {
	if e == nil || !e.config.Dev || e.store == nil || r == nil || r.broker == nil {
		return errors.New("manual Store wiring unavailable")
	}
	b := r.broker
	if admission == nil {
		return errors.New("manual Store admission unavailable")
	}
	v, err := admission.validate(ctx)
	if err != nil {
		return err
	}
	l := &nativePilotStoreRuntime{engine: e, runtime: r, revokeID: domain.NewID("manual-revoke"), work: map[string]*nativePilotStoreWork{}, afterCommit: e.afterAcceptedObservation, admission: admission}
	b.mu.Lock()
	r.mu.Lock()
	if b.stopped || b.onStop != nil || r.storeBinding {
		r.mu.Unlock()
		b.mu.Unlock()
		return errors.New("manual Store wiring closed or duplicate")
	}
	r.storeBinding = true
	r.mu.Unlock()
	b.onStop = l.close
	b.mu.Unlock()
	p, err := admission.issue(ctx, sessionID, runID)
	if err != nil {
		l.close()
		return err
	}
	l.mu.Lock()
	l.permit = p
	closed := l.closed
	if !closed {
		r.mu.Lock()
		r.storeLink = l
		r.mu.Unlock()
	}
	l.mu.Unlock()
	if closed {
		l.close()
		return errors.New("manual Store startup invalidated")
	}
	// Timers use only the remaining original budget, never a fresh 60 seconds.
	delay := v.Deadline.Sub(e.store.Now())
	time.AfterFunc(max(delay, 0), func() { b.stop("attempt-expired") })
	if _, err = admission.validate(ctx); err != nil {
		l.close()
		return err
	}
	return nil
}

// captureNativePilot requires a durable claim before touching the backend.
// A work reservation is never removed: ambiguity, errors and replays cannot
// silently turn into another capture. Follow-ups use new commands/claims.
func (e *Engine) captureNativePilot(ctx context.Context, r *nativePilotRuntime, commandID string) nativePilotStoreResult {
	rejected := func(err error) nativePilotStoreResult {
		return nativePilotStoreResult{Outcome: store.RejectedNoMutation, Phase: "admission", PersistenceError: err}
	}
	if r == nil {
		return rejected(errors.New("manual Store route unavailable"))
	}
	r.mu.Lock()
	l := r.storeLink
	r.mu.Unlock()
	if l == nil || l.engine != e {
		return rejected(errors.New("manual Store route unavailable"))
	}
	l.mu.Lock()
	if previous := l.work[commandID]; previous != nil {
		result := previous.result
		if !previous.done {
			result = nativePilotStoreResult{Outcome: store.Indeterminate, Phase: "in_flight", PersistenceError: errors.New("manual work already in flight")}
		} else if result.Outcome == store.Committed {
			result.Outcome = store.AlreadyCommitted
		}
		l.mu.Unlock()
		return result
	}
	if l.closed || len(l.work) >= 32 {
		l.mu.Unlock()
		return rejected(errors.New("manual Store admission closed"))
	}
	w := &nativePilotStoreWork{}
	l.work[commandID] = w
	l.mu.Unlock()
	result := e.captureNativePilotOnce(ctx, l, commandID)
	l.mu.Lock()
	w.result, w.done = result, true
	l.mu.Unlock()
	return result
}

func (e *Engine) captureNativePilotOnce(ctx context.Context, l *nativePilotStoreRuntime, commandID string) nativePilotStoreResult {
	out := nativePilotStoreResult{Outcome: store.RejectedNoMutation, Phase: "dispatch"}
	fail := func(err error) nativePilotStoreResult { out.PersistenceError = err; return out }
	if ctx == nil {
		return fail(errors.New("manual context required"))
	}
	r, b := l.runtime, l.runtime.broker
	stop := context.AfterFunc(ctx, func() { b.invalidate("canceled") })
	defer stop()
	ticket, d := b.dispatch(ctx, commandID)
	if ticket == nil || !nativePilotRuntimeAllowed(d) {
		return fail(errors.New("manual dispatch rejected"))
	}
	if !nativePilotRuntimeAllowed(b.claim(ctx, ticket, func() bool { return true })) {
		return fail(errors.New("manual ticket claim rejected"))
	}
	out.Phase = "store_claim"
	claim, err := e.store.ClaimManualPilotWithBehavior(ctx, l.permit, commandID, "claim:"+l.revokeID+":"+commandID, e.capturePacingPolicy())
	out.ClaimOutcome = claim.Outcome
	if err != nil || claim.Handle == nil || claim.Command == nil {
		if err == nil {
			err = errors.New("manual claim lacks confirmed handle")
		}
		return fail(err)
	}
	// A confirmed claim is not a committed result.
	out.Outcome = store.RejectedNoMutation
	out.Phase = "owner_check"
	if _, err := l.admission.validate(ctx); err != nil {
		l.close()
		return fail(err)
	}
	lease, err := r.collector.Acquire()
	if err != nil {
		return fail(err)
	}
	defer lease.Release()
	ownerOK := func() bool {
		return nativePilotRuntimeOwner(r.collector, lease, ticket.collector, "headless") && r.backend.nativePilotCollectorOwner() == ticket.collector
	}
	if !ownerOK() {
		b.invalidate("owner-or-policy-changed")
		return fail(errors.New("manual collector changed"))
	}
	out.Phase = "capture"
	work, done, err := e.beginCaptureBehavior(ctx, domain.SourceX)
	if err != nil {
		reason := "unavailable"
		if errors.Is(err, store.ErrSourceCapturePaused) {
			reason = "source_paused"
		}
		out.CaptureDiagnostic = nativePilotCaptureDetail(err, "behavior", reason)
		l.close()
		return fail(err)
	}
	defer done()
	observation, err := r.coordinator.Capture(work, domain.SourceX, claim.Command.Payload)
	if err != nil {
		// Record the capture cause before challenge-pause persistence or revocation
		// can produce a different error. The diagnostic does not change outcomes.
		out.CaptureDiagnostic = nativePilotCaptureDetail(err, "coordinator", "unavailable")
		var challenge *headless.CaptureError
		if errors.As(err, &challenge) && domain.IsAccessChallenge(challenge.Code) {
			e.captureBehaviorMu.Lock()
			pauseErr := e.store.FailManualPilotChallenge(ctx, claim.Handle, challenge.ChallengeCategory)
			if pauseErr == nil {
				for _, cancel := range e.captureWork {
					cancel()
				}
			}
			e.captureBehaviorMu.Unlock()
			if pauseErr != nil {
				err = pauseErr
			}
		}
		l.close()
		return fail(err)
	}
	if !ownerOK() {
		b.invalidate("owner-or-policy-changed")
		return fail(errors.New("manual result owner rejected"))
	}
	out.Phase = "result_validation"
	if observation.Source != domain.SourceX {
		b.invalidate("evidence-invalid")
		return fail(errors.New("manual result source rejected"))
	}
	normalizeObservation(&observation)
	if err := validateObservation(observation); err != nil {
		return fail(err)
	}
	// The effect is an in-memory ticket transition only. SQLite and scheduling
	// run after both broker and controller locks have been released.
	if !nativePilotRuntimeAllowed(b.complete(ctx, ticket, func() bool { return true })) {
		l.close()
		return fail(errors.New("manual result admission rejected"))
	}
	out.Phase = "store_commit"
	out.Outcome, out.PersistenceError = e.store.CommitManualPilotResult(ctx, claim.Handle, "result:"+l.revokeID+":"+commandID, observation)
	if out.Outcome != store.Committed && out.Outcome != store.AlreadyCommitted {
		return out
	}
	out.Phase = "reasoning_delivery"
	run, err := e.store.GetRun(ctx, claim.Command.RunID)
	if err != nil {
		out.DeliveryError = err
		return out
	}
	out.Run, out.DeliveryError = l.afterCommit(ctx, run)
	if out.DeliveryError == nil {
		out.Phase = "completed"
	}
	return out
}
