//go:build auth_session_dev

package engine

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/collection"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/store"
)

const (
	// DevelopmentSessionProbeVersion versions the synthetic verifier request and response contract.
	DevelopmentSessionProbeVersion = 1
	// DevelopmentSessionMaxEvidenceAge bounds how long one source observation may be reused.
	DevelopmentSessionMaxEvidenceAge = 5 * time.Second
)

type DevelopmentSessionProbeKind string

const (
	DevelopmentSessionProbeAuthority DevelopmentSessionProbeKind = "authority"
	DevelopmentSessionProbeBrowser   DevelopmentSessionProbeKind = "browser"
)

type DevelopmentSessionAuthState string

const (
	DevelopmentSessionAuthAuthenticated DevelopmentSessionAuthState = "authenticated"
	DevelopmentSessionAuthLoggedOut     DevelopmentSessionAuthState = "logged_out"
	DevelopmentSessionAuthChallenge     DevelopmentSessionAuthState = "challenge"
	DevelopmentSessionAuthUnknown       DevelopmentSessionAuthState = "unknown"
	DevelopmentSessionAuthUnsupported   DevelopmentSessionAuthState = "unsupported"
)

// DevelopmentSessionProbeRequest is a one-use, phase-specific challenge for
// the synthetic verifier. Authority is synthetic control evidence that may be
// available before a browser exists; it does not prove a real browser login.
type DevelopmentSessionProbeRequest struct {
	Version     int
	Source      domain.Source
	ProfileID   string
	Kind        DevelopmentSessionProbeKind
	ProbeID     string
	RequestedAt time.Time
}

// DevelopmentSessionEvidence is private verifier output, not bridge readiness.
// The only supported domain is synthetic-fixture. Real source adapters are absent.
type DevelopmentSessionEvidence struct {
	Version    int
	Kind       DevelopmentSessionProbeKind
	ProbeID    string
	ObservedAt time.Time
	Domain     string
	Source     domain.Source
	ProfileID  string
	AuthState  DevelopmentSessionAuthState
	AccountID  string
	Revision   string
}

type DevelopmentBrowserEvidence struct {
	DevelopmentSessionEvidence
	EngineEpoch string
	Generation  uint64
	PID         int
	Driver      string
}

type DevelopmentSessionCapability struct {
	Provider         string
	StoreScopeID     string
	ProfileID        string
	ExpectedAccounts map[domain.Source]string
	Sources          []domain.Source
	Authority        func(context.Context, DevelopmentSessionProbeRequest) (DevelopmentSessionEvidence, error)
	Browser          func(context.Context, DevelopmentSessionProbeRequest) (DevelopmentBrowserEvidence, error)
	clock            func() time.Time
}

type developmentCheckpointProof struct {
	checkpoint store.SyntheticAccountCheckpoint
	fence      store.SyntheticCheckpointFence
	receipt    store.SyntheticVerifiedReceipt
	owner      *captureruntime.Manager
}

func developmentCheckpointFenceEqual(a, b store.SyntheticCheckpointFence) bool {
	if a.Domain != b.Domain || a.Version != b.Version || a.CheckpointID != b.CheckpointID || a.Generation != b.Generation ||
		a.HoldID != b.HoldID || a.CheckpointSHA256 != b.CheckpointSHA256 || len(a.Scope) != len(b.Scope) {
		return false
	}
	for source, scope := range a.Scope {
		if b.Scope[source] != scope {
			return false
		}
	}
	return true
}

func syntheticCheckpointExpectedEqual(a, b map[domain.Source]store.SyntheticAccountIdentity) bool {
	if len(a) != len(b) {
		return false
	}
	for source, identity := range a {
		if b[source] != identity {
			return false
		}
	}
	return len(a) > 0
}

func syntheticCheckpointStateEqual(a, b store.SyntheticAccountCheckpoint) bool {
	if a.Domain != b.Domain || a.Version != b.Version || a.CheckpointID != b.CheckpointID || a.Generation != b.Generation ||
		a.VerificationTime != b.VerificationTime || a.OwnerEpoch != b.OwnerEpoch || a.HeadlessGeneration != b.HeadlessGeneration || a.OwnerPID != b.OwnerPID ||
		!syntheticCheckpointExpectedEqual(a.ExpectedAccounts, b.ExpectedAccounts) || len(a.AcceptedRevisions) != len(b.AcceptedRevisions) {
		return false
	}
	for source, revision := range a.AcceptedRevisions {
		if b.AcceptedRevisions[source] != revision {
			return false
		}
	}
	return true
}

// PrepareDevelopmentSessionCheckpoint fences capture before a synthetic owner
// is created. Fresh enrollment is available only to the explicitly granted
// development fixture; reopen loads and matches the persisted identity map.
func (e *Engine) PrepareDevelopmentSessionCheckpoint(ctx context.Context, capability DevelopmentSessionCapability, freshEnrollment bool) error {
	if !e.config.Dev || e.store == nil || capability.Provider != "synthetic-fixture" ||
		strings.TrimSpace(capability.StoreScopeID) == "" || strings.TrimSpace(capability.ProfileID) == "" ||
		len(capability.Sources) == 0 || len(capability.ExpectedAccounts) != len(capability.Sources) {
		return errors.New("development checkpoint preparation unavailable")
	}
	sources := make(map[domain.Source]bool, len(capability.Sources))
	expected := make(map[domain.Source]store.SyntheticAccountIdentity, len(capability.Sources))
	for _, source := range capability.Sources {
		if !supportedDevelopmentSource(source) || sources[source] {
			return errors.New("development checkpoint source unsupported")
		}
		sources[source] = true
		accountID := capability.ExpectedAccounts[source]
		if strings.TrimSpace(accountID) == "" || accountID != strings.TrimSpace(accountID) {
			return errors.New("development checkpoint identity incomplete")
		}
		expected[source] = store.SyntheticAccountIdentity{AccountID: accountID, StoreScopeID: capability.StoreScopeID, ProfileID: capability.ProfileID}
	}
	for source := range capability.ExpectedAccounts {
		if !sources[source] {
			return errors.New("development checkpoint identity unsupported")
		}
	}
	settings, err := e.store.GetSettings(ctx)
	if err != nil || settings.CollectionMode != "headless" {
		return errors.New("development checkpoint requires headless collection")
	}
	for _, source := range settings.ActiveSources {
		if !sources[source] {
			return errors.New("development checkpoint active source unsupported")
		}
	}
	checkpoint, fence, err := e.store.PrepareSyntheticAccountCheckpoint(ctx, expected, freshEnrollment)
	if err != nil {
		return err
	}
	checkpoint.ExpectedAccounts = copyDevelopmentCheckpointAccounts(checkpoint.ExpectedAccounts)
	checkpoint.AcceptedRevisions = copyDevelopmentCheckpointRevisions(checkpoint.AcceptedRevisions)
	fence.Scope = copyDevelopmentCheckpointScope(fence.Scope)
	e.developmentCheckpointMu.Lock()
	e.developmentCheckpoint = &checkpoint
	e.developmentCheckpointFence = &fence
	e.developmentCheckpointAuthorizedAtStartup = false
	e.developmentCheckpointAuthorize = nil
	e.developmentCheckpointRevoke = nil
	e.developmentCheckpointMu.Unlock()
	return nil
}

func supportedDevelopmentSource(source domain.Source) bool {
	return source == domain.SourceX || source == domain.SourceInstagram || source == domain.SourceLinkedIn
}

func copyDevelopmentCheckpointAccounts(in map[domain.Source]store.SyntheticAccountIdentity) map[domain.Source]store.SyntheticAccountIdentity {
	out := make(map[domain.Source]store.SyntheticAccountIdentity, len(in))
	for source, identity := range in {
		out[source] = identity
	}
	return out
}

func copyDevelopmentCheckpointRevisions(in map[domain.Source]string) map[domain.Source]string {
	out := make(map[domain.Source]string, len(in))
	for source, revision := range in {
		out[source] = revision
	}
	return out
}

func copyDevelopmentCheckpointScope(in map[domain.Source]store.SyntheticAccountScope) map[domain.Source]store.SyntheticAccountScope {
	out := make(map[domain.Source]store.SyntheticAccountScope, len(in))
	for source, scope := range in {
		out[source] = scope
	}
	return out
}

// BindDevelopmentSession requires both source authority and current browser proof.
// It must precede startup resume and collection workers on a scoped development Store.
func (e *Engine) BindDevelopmentSession(ctx context.Context, capability DevelopmentSessionCapability) error {
	if !e.config.Dev || capability.Provider != "synthetic-fixture" || strings.TrimSpace(capability.StoreScopeID) == "" || strings.TrimSpace(capability.ProfileID) == "" || len(capability.Sources) == 0 || capability.Authority == nil || capability.Browser == nil {
		return errors.New("development session capability unavailable")
	}
	sources := append([]domain.Source(nil), capability.Sources...)
	expectedAccounts := make(map[domain.Source]string, len(capability.ExpectedAccounts))
	for source, accountID := range capability.ExpectedAccounts {
		expectedAccounts[source] = accountID
	}
	allowed := map[domain.Source]bool{}
	for _, source := range sources {
		if !supportedDevelopmentSource(source) || allowed[source] {
			return errors.New("development session source unsupported")
		}
		allowed[source] = true
	}
	if len(expectedAccounts) != len(allowed) {
		return errors.New("development session expected identities incomplete")
	}
	for source, accountID := range expectedAccounts {
		if !allowed[source] || strings.TrimSpace(accountID) == "" {
			return errors.New("development session expected identities unsupported")
		}
	}
	preparedExpected := make(map[domain.Source]store.SyntheticAccountIdentity, len(expectedAccounts))
	for source, accountID := range expectedAccounts {
		preparedExpected[source] = store.SyntheticAccountIdentity{AccountID: accountID, StoreScopeID: capability.StoreScopeID, ProfileID: capability.ProfileID}
	}
	e.developmentCheckpointMu.Lock()
	prepared := e.developmentCheckpoint
	preparedOK := prepared != nil && e.developmentCheckpointFence != nil &&
		syntheticCheckpointExpectedEqual(prepared.ExpectedAccounts, preparedExpected)
	e.developmentCheckpointMu.Unlock()
	if !preparedOK {
		return errors.New("development session checkpoint preparation required")
	}
	settings, err := e.store.GetSettings(ctx)
	if err != nil {
		return errors.New("development session settings unavailable")
	}
	if settings.CollectionMode != "headless" {
		return errors.New("development session requires headless collection")
	}
	for _, source := range settings.ActiveSources {
		if !allowed[source] {
			return errors.New("development session source unsupported")
		}
	}
	identitySources := append([]domain.Source(nil), sources...)
	sort.Slice(identitySources, func(i, j int) bool { return identitySources[i] < identitySources[j] })
	identities := make([][2]string, 0, len(identitySources))
	for _, source := range identitySources {
		identities = append(identities, [2]string{string(source), expectedAccounts[source]})
	}
	expectedIdentity, err := json.Marshal(identities)
	if err != nil {
		return errors.New("development session expected identities unavailable")
	}
	now := func() time.Time {
		if capability.clock != nil {
			return capability.clock().UTC()
		}
		return time.Now().UTC()
	}
	newRequest := func(source domain.Source, kind DevelopmentSessionProbeKind) DevelopmentSessionProbeRequest {
		return DevelopmentSessionProbeRequest{
			Version: DevelopmentSessionProbeVersion, Source: source, ProfileID: capability.ProfileID,
			Kind: kind, ProbeID: domain.NewID("devprobe"), RequestedAt: now(),
		}
	}
	matches := func(v DevelopmentSessionEvidence, request DevelopmentSessionProbeRequest, now time.Time) bool {
		return request.Version == DevelopmentSessionProbeVersion && request.ProbeID != "" &&
			v.Version == request.Version && v.Kind == request.Kind && v.ProbeID == request.ProbeID &&
			!v.ObservedAt.IsZero() && !v.ObservedAt.Before(request.RequestedAt) && !v.ObservedAt.After(now) &&
			now.Sub(v.ObservedAt) <= DevelopmentSessionMaxEvidenceAge &&
			v.Domain == "synthetic-fixture" && v.Source == request.Source && v.ProfileID == request.ProfileID &&
			v.AuthState == DevelopmentSessionAuthAuthenticated && v.AccountID == expectedAccounts[request.Source] &&
			strings.TrimSpace(v.Revision) != ""
	}
	probe := func(probeCtx context.Context, browser bool) (sessionEvidence, error) {
		if probeCtx.Err() != nil {
			return sessionEvidence{}, errors.New("development source probe canceled")
		}
		var owner *captureruntime.Manager
		var ownerSnapshot captureruntime.Snapshot
		var ownerPID int
		var ownerEpoch string
		if browser {
			owner = e.captureOwner
			if owner == nil {
				return sessionEvidence{}, errors.New("development browser owner unavailable")
			}
			ownerSnapshot, ownerPID, ownerEpoch = owner.Snapshot(), owner.PID(), e.Epoch()
			if ownerSnapshot.State != captureruntime.Ready || ownerPID <= 0 {
				return sessionEvidence{}, errors.New("development browser owner proof required")
			}
		}
		revisions := make([][2]string, 0, len(sources))
		observations := make([]struct {
			request  DevelopmentSessionProbeRequest
			evidence DevelopmentSessionEvidence
		}, 0, len(sources))
		for _, source := range sources {
			kind := DevelopmentSessionProbeAuthority
			if browser {
				kind = DevelopmentSessionProbeBrowser
			}
			request := newRequest(source, kind)
			var v DevelopmentSessionEvidence
			if browser {
				before, pid := owner.Snapshot(), owner.PID()
				if e.captureOwner != owner || before != ownerSnapshot || pid != ownerPID {
					return sessionEvidence{}, errors.New("development browser owner proof required")
				}
				proof, probeErr := capability.Browser(probeCtx, request)
				after := owner.Snapshot()
				if probeErr != nil || before.State != captureruntime.Ready || before != after || after != ownerSnapshot || pid != ownerPID || e.captureOwner != owner || owner.PID() != ownerPID || proof.EngineEpoch != ownerEpoch || proof.Generation != ownerSnapshot.Generation || proof.PID != ownerPID || proof.Driver != ownerSnapshot.Driver {
					return sessionEvidence{}, errors.New("development browser owner proof required")
				}
				v = proof.DevelopmentSessionEvidence
			} else {
				authority, probeErr := capability.Authority(probeCtx, request)
				if probeErr != nil {
					return sessionEvidence{}, errors.New("development source authority unavailable")
				}
				v = authority
			}
			if probeCtx.Err() != nil {
				return sessionEvidence{}, errors.New("development source probe canceled")
			}
			if !matches(v, request, now()) {
				return sessionEvidence{}, errors.New("development source identity proof required")
			}
			revisions = append(revisions, [2]string{string(source), v.Revision})
			observations = append(observations, struct {
				request  DevelopmentSessionProbeRequest
				evidence DevelopmentSessionEvidence
			}{request: request, evidence: v})
		}
		finalObservedAt := now()
		if probeCtx.Err() != nil {
			return sessionEvidence{}, errors.New("development source probe canceled")
		}
		if browser && (e.captureOwner != owner || owner.Snapshot() != ownerSnapshot || owner.PID() != ownerPID || e.Epoch() != ownerEpoch) {
			return sessionEvidence{}, errors.New("development browser owner changed during evidence collection")
		}
		for _, observation := range observations {
			if !matches(observation.evidence, observation.request, finalObservedAt) {
				return sessionEvidence{}, errors.New("development source evidence expired")
			}
		}
		raw, _ := json.Marshal(revisions)
		observedAt := time.Time{}
		for _, observation := range observations {
			if observedAt.IsZero() || observation.evidence.ObservedAt.Before(observedAt) {
				observedAt = observation.evidence.ObservedAt.UTC()
			}
		}
		return sessionEvidence{
			Authenticated: true, AccountID: string(expectedIdentity), Revision: string(raw), ObservedAt: observedAt,
			OwnerEpoch: ownerEpoch, OwnerGeneration: ownerSnapshot.Generation, OwnerPID: ownerPID, OwnerDriver: ownerSnapshot.Driver,
		}, nil
	}
	receiptProbe := func(probeCtx context.Context) (developmentCheckpointProof, error) {
		e.developmentCheckpointMu.Lock()
		var checkpoint *store.SyntheticAccountCheckpoint
		var capturedFence *store.SyntheticCheckpointFence
		if e.developmentCheckpoint != nil && e.developmentCheckpointFence != nil {
			copyCheckpoint := *e.developmentCheckpoint
			copyCheckpoint.ExpectedAccounts = copyDevelopmentCheckpointAccounts(e.developmentCheckpoint.ExpectedAccounts)
			copyCheckpoint.AcceptedRevisions = copyDevelopmentCheckpointRevisions(e.developmentCheckpoint.AcceptedRevisions)
			copyFence := *e.developmentCheckpointFence
			copyFence.Scope = copyDevelopmentCheckpointScope(e.developmentCheckpointFence.Scope)
			checkpoint, capturedFence = &copyCheckpoint, &copyFence
		}
		e.developmentCheckpointMu.Unlock()
		if checkpoint == nil || capturedFence == nil {
			return developmentCheckpointProof{}, errors.New("development checkpoint fence unavailable")
		}
		owner := developmentCurrentOwner(e)
		if owner == nil {
			return developmentCheckpointProof{}, errors.New("development browser owner unavailable")
		}
		ownerSnapshot, ownerPID, ownerEpoch := owner.Snapshot(), owner.PID(), e.Epoch()
		authority, probeErr := probe(probeCtx, false)
		if probeErr != nil {
			return developmentCheckpointProof{}, probeErr
		}
		browser, probeErr := probe(probeCtx, true)
		if probeErr != nil || authority.AccountID != browser.AccountID || authority.Revision != browser.Revision {
			return developmentCheckpointProof{}, errors.New("development checkpoint vectors do not match")
		}
		if browser.OwnerEpoch == "" || browser.OwnerPID <= 0 || browser.OwnerDriver != "headless" ||
			browser.OwnerGeneration == 0 || browser.OwnerGeneration > uint64(1<<53-1) {
			return developmentCheckpointProof{}, errors.New("development checkpoint owner proof required")
		}
		if developmentCurrentOwner(e) != owner || ownerSnapshot.State != captureruntime.Ready || ownerSnapshot.Driver != "headless" ||
			ownerSnapshot.Generation != browser.OwnerGeneration || ownerPID != browser.OwnerPID || ownerEpoch != browser.OwnerEpoch ||
			owner.Snapshot() != ownerSnapshot || owner.PID() != ownerPID || e.Epoch() != ownerEpoch {
			return developmentCheckpointProof{}, errors.New("development checkpoint owner changed")
		}
		var revisionPairs [][2]string
		if json.Unmarshal([]byte(browser.Revision), &revisionPairs) != nil || len(revisionPairs) != len(expectedAccounts) {
			return developmentCheckpointProof{}, errors.New("development checkpoint revision vector unavailable")
		}
		revisions := make(map[domain.Source]string, len(revisionPairs))
		for _, pair := range revisionPairs {
			source := domain.Source(pair[0])
			if !allowed[source] || revisions[source] != "" || strings.TrimSpace(pair[1]) == "" {
				return developmentCheckpointProof{}, errors.New("development checkpoint revision vector invalid")
			}
			revisions[source] = pair[1]
		}
		if len(revisions) != len(expectedAccounts) {
			return developmentCheckpointProof{}, errors.New("development checkpoint revision vector incomplete")
		}
		capturedAt := authority.ObservedAt
		if capturedAt.IsZero() || browser.ObservedAt.IsZero() {
			return developmentCheckpointProof{}, errors.New("development checkpoint evidence time unavailable")
		}
		if browser.ObservedAt.Before(capturedAt) {
			capturedAt = browser.ObservedAt
		}
		capturedAt = capturedAt.UTC()
		verifiedNow := now()
		if capturedAt.After(verifiedNow) || verifiedNow.Sub(capturedAt) > DevelopmentSessionMaxEvidenceAge {
			return developmentCheckpointProof{}, errors.New("development checkpoint evidence expired")
		}
		e.developmentCheckpointMu.Lock()
		currentCheckpoint, currentFence := e.developmentCheckpoint, e.developmentCheckpointFence
		checkpointUnchanged := currentCheckpoint != nil && currentFence != nil &&
			syntheticCheckpointStateEqual(*currentCheckpoint, *checkpoint) &&
			developmentCheckpointFenceEqual(*currentFence, *capturedFence)
		receipt := store.SyntheticVerifiedReceipt{
			Domain: "synthetic-fixture", CheckpointID: capturedFence.CheckpointID, Generation: capturedFence.Generation, HoldID: capturedFence.HoldID,
			Accounts: make(map[domain.Source]string, len(expectedAccounts)), StoreScopeIDs: make(map[domain.Source]string, len(expectedAccounts)),
			ProfileIDs: make(map[domain.Source]string, len(expectedAccounts)), Revisions: revisions,
			OwnerEpoch: browser.OwnerEpoch, HeadlessGeneration: browser.OwnerGeneration, OwnerPID: browser.OwnerPID,
			CapturedAt: capturedAt,
		}
		for source, accountID := range expectedAccounts {
			receipt.Accounts[source] = accountID
			receipt.StoreScopeIDs[source] = capability.StoreScopeID
			receipt.ProfileIDs[source] = capability.ProfileID
		}
		e.developmentCheckpointMu.Unlock()
		if !checkpointUnchanged || developmentCurrentOwner(e) != owner || owner.Snapshot() != ownerSnapshot || owner.PID() != ownerPID || e.Epoch() != ownerEpoch {
			return developmentCheckpointProof{}, errors.New("development checkpoint changed during verification")
		}
		return developmentCheckpointProof{checkpoint: *checkpoint, fence: *capturedFence, receipt: receipt, owner: owner}, nil
	}
	e.developmentCheckpointAuthorize = func(authorizeCtx context.Context, admissionGuard func() error) error {
		proof, receiptErr := receiptProbe(authorizeCtx)
		if receiptErr != nil {
			return receiptErr
		}
		if admissionGuard != nil && admissionGuard() != nil {
			return errors.New("development checkpoint admission changed during verification")
		}
		receipt := proof.receipt
		e.captureMu.Lock()
		defer e.captureMu.Unlock()
		if e.captureOwner != proof.owner || proof.owner == nil || proof.owner.PID() != receipt.OwnerPID ||
			proof.owner.Snapshot().Generation != receipt.HeadlessGeneration || e.epoch != receipt.OwnerEpoch {
			return errors.New("development checkpoint owner changed during authorization")
		}
		e.developmentCheckpointMu.Lock()
		defer e.developmentCheckpointMu.Unlock()
		currentCheckpoint, currentFence := e.developmentCheckpoint, e.developmentCheckpointFence
		fenceUnchanged := currentCheckpoint != nil && currentFence != nil && syntheticCheckpointStateEqual(*currentCheckpoint, proof.checkpoint) &&
			developmentCheckpointFenceEqual(*currentFence, proof.fence)
		if !fenceUnchanged {
			return errors.New("development checkpoint fence changed during verification")
		}
		if authorizeErr := e.store.AuthorizeSyntheticAccountCheckpoint(authorizeCtx, proof.fence, receipt); authorizeErr != nil {
			e.developmentCheckpointFence = nil
			return authorizeErr
		}
		if e.developmentCheckpoint == nil || e.developmentCheckpoint.CheckpointID != receipt.CheckpointID || e.developmentCheckpoint.Generation != receipt.Generation ||
			e.developmentCheckpointFence == nil || !developmentCheckpointFenceEqual(*e.developmentCheckpointFence, proof.fence) {
			e.developmentCheckpointFence = nil
			return errors.New("development checkpoint changed during authorization")
		}
		checkpoint := *e.developmentCheckpoint
		checkpoint.Generation++
		checkpoint.AcceptedRevisions = copyDevelopmentCheckpointRevisions(receipt.Revisions)
		checkpoint.VerificationTime = receipt.CapturedAt.UTC().Format(time.RFC3339Nano)
		checkpoint.OwnerEpoch = receipt.OwnerEpoch
		checkpoint.HeadlessGeneration = receipt.HeadlessGeneration
		checkpoint.OwnerPID = receipt.OwnerPID
		e.developmentCheckpoint = &checkpoint
		e.developmentCheckpointFence = nil
		e.developmentCheckpointAuthorizedAtStartup = true
		return nil
	}
	e.developmentCheckpointRevoke = func(revokeCtx context.Context) error {
		e.developmentCheckpointMu.Lock()
		defer e.developmentCheckpointMu.Unlock()
		checkpoint := e.developmentCheckpoint
		if checkpoint == nil {
			return errors.New("development checkpoint unavailable")
		}
		e.developmentCheckpointFence = nil
		fence, fenceErr := e.store.BeginSyntheticCheckpointFence(revokeCtx, checkpoint.CheckpointID, checkpoint.Generation)
		if fenceErr != nil {
			return fenceErr
		}
		e.developmentCheckpointFence = &fence
		return nil
	}
	if e.captureOwner == nil || e.collectionRuntime == nil || e.collectionRuntime.ExclusiveSessionBound() {
		return errors.New("development session startup binding unavailable")
	}
	e.developmentSources = allowed
	e.collectionRuntime.SetDevelopmentSession(true, sources...)
	if e.fixtureAdmission == nil {
		e.fixtureAdmission = &fixtureAdmissionGate{paused: true}
	}
	// Saved settings do not override an older frozen session plan. Reject an
	// unsupported durable route before resume can advance or create its command.
	ids, err := e.store.ListFixturePausedCaptureSessions(ctx)
	if err != nil {
		return errors.New("development session queue unavailable")
	}
	active, err := e.store.ActiveSession(ctx)
	if err != nil {
		return errors.New("development session state unavailable")
	}
	if active != nil {
		ids = append(ids, active.ID)
	}
	for _, id := range ids {
		saved, err := e.store.GetSession(ctx, id)
		if err != nil {
			return errors.New("development session state unavailable")
		}
		if saved.Status != "queued" && saved.Status != "running" {
			continue
		}
		if !hybridCollectionSession(saved) {
			return errors.New("development session cannot adopt non-headless work")
		}
		for _, run := range saved.Runs {
			driver, ok := hybridDriverForRun(saved, run.Source)
			if !allowed[run.Source] || !ok || driver != "headless" {
				return errors.New("development session cannot adopt unsupported source route")
			}
		}
	}
	if err := e.bindExclusiveSession(string(expectedIdentity), func(c context.Context) (sessionEvidence, error) { return probe(c, false) }, func(c context.Context) (sessionEvidence, error) { return probe(c, true) }, true); err != nil {
		return err
	}
	return nil
}

func developmentCheckpointAuthorizedAtStartup(e *Engine) bool {
	if e == nil {
		return false
	}
	e.developmentCheckpointMu.Lock()
	defer e.developmentCheckpointMu.Unlock()
	return e.developmentCheckpointAuthorizedAtStartup
}

// StartDevelopmentSession is the shared integrated startup boundary. All recovery
// callbacks execute only after binding a paused queue. The scoped owner must be
// ready before this call; initialize configures that verified owner and does not
// create it. No callback may log in. Only accepted-capture reasoning is resumed.
func (e *Engine) StartDevelopmentSession(ctx context.Context, owner *captureruntime.Manager, coordinator *collection.Coordinator, capability DevelopmentSessionCapability, initialize func(context.Context) error) error {
	if owner == nil || coordinator == nil {
		return errors.New("development startup ownership unavailable")
	}
	if e.captureOwner == nil {
		if err := e.AttachCaptureRuntime(ctx, owner); err != nil {
			return err
		}
	} else if e.captureOwner != owner {
		return errors.New("development startup owner changed")
	}
	e.AttachCollectionCoordinator(coordinator)
	if err := e.BindDevelopmentSession(ctx, capability); err != nil {
		return err
	}
	closeCheckpoint := func() {
		e.developmentCheckpointMu.Lock()
		revoke := e.developmentCheckpointRevoke
		e.developmentCheckpointMu.Unlock()
		if revoke != nil {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = revoke(cleanupCtx)
		}
	}
	if initialize != nil {
		if err := initialize(ctx); err != nil {
			closeCheckpoint()
			return err
		}
	}
	e.developmentCheckpointMu.Lock()
	authorize := e.developmentCheckpointAuthorize
	e.developmentCheckpointMu.Unlock()
	if authorize == nil {
		closeCheckpoint()
		return errors.New("development checkpoint authorization unavailable")
	}
	if err := authorize(ctx, nil); err != nil {
		closeCheckpoint()
		return err
	}
	if _, err := e.ResumePendingReasoning(ctx); err != nil {
		closeCheckpoint()
		return err
	}
	coordinator.Request("headless")
	e.developmentCoordinatorDone = coordinator.StartWithDone(ctx)
	return nil
}

// PauseDevelopmentSession prepares a bound, idle headless development runtime for
// a lifecycle test. It grants no reader/foreground capability and has no HTTP route.
func (e *Engine) PauseDevelopmentSession(ctx context.Context) error {
	if e.developmentPause == nil || e.captureOwner == nil || e.collectionRuntime == nil || e.collectionRuntime.ExclusiveReaderPending() {
		return errors.New("development pause unavailable")
	}
	snapshot := e.captureOwner.Snapshot()
	if snapshot.State != captureruntime.Ready || snapshot.Driver != "headless" {
		return errors.New("development pause requires ready headless owner")
	}
	// Keep this explicit lifecycle hold closed through coordinator polling. It
	// grants no window intent; a later process requires a fresh verified binding.
	e.fixtureAdmission.mu.Lock()
	e.fixtureAdmission.developmentHold = true
	e.fixtureAdmission.paused = true
	e.fixtureAdmission.pauseEpoch++
	e.fixtureAdmission.mu.Unlock()
	return e.developmentPause(ctx)
}

func (e *Engine) VerifyDevelopmentReader(ctx context.Context) error {
	if e.developmentVerifyBrowser == nil || e.captureOwner == nil || e.captureOwner.Snapshot().State != captureruntime.Ready || e.captureOwner.Snapshot().Driver != "browser" {
		return errors.New("development reader verification unavailable")
	}
	return e.developmentVerifyBrowser(ctx)
}
