//go:build windows && auth_session_dev && auth_session_fixture

package engine

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/store"
)

// RunDevelopmentPolicyFixture is an isolated actual-main composition. Its
// private controller frames simulate decisions; they are not human-consent
// evidence or public API requests. Existing lineage/parent/image/Job gates apply.
func RunDevelopmentPolicyFixture(input io.Reader, output io.Writer) error {
	var controller *SyntheticPolicyController
	var runtime *DevelopmentFixtureRuntime
	var engine *Engine
	var scopedStart processFixtureStart
	checkProfile := func() error {
		id, err := processFixtureDirectoryID(filepath.Join(scopedStart.Grant.Root, "shared"))
		if err != nil || id != scopedStart.Grant.ProfileID {
			return errors.New("policy fixture physical profile changed")
		}
		return nil
	}
	convert := func(v SyntheticSessionEvidence, request DevelopmentSessionProbeRequest) DevelopmentSessionEvidence {
		state := DevelopmentSessionAuthUnknown
		if v.Authenticated {
			state = DevelopmentSessionAuthAuthenticated
		}
		return DevelopmentSessionEvidence{Version: request.Version, Kind: request.Kind, ProbeID: request.ProbeID, ObservedAt: time.Now().UTC(),
			Domain: "synthetic-fixture", Source: request.Source, ProfileID: scopedStart.Grant.ProfileID, AuthState: state, AccountID: v.AccountID, Revision: v.Revision}
	}
	hooks := &processFixturePolicyHooks{}
	hooks.prepare = func(ctx context.Context, e *Engine, start processFixtureStart, scanner *bufio.Scanner, encoder *json.Encoder) error {
		engine, scopedStart = e, start
		capability := DevelopmentSessionCapability{Provider: "synthetic-fixture", StoreScopeID: start.Grant.RootID, ProfileID: start.Grant.ProfileID,
			ExpectedAccounts: map[domain.Source]string{domain.SourceX: "alpha", domain.SourceInstagram: "alpha"}, Sources: []domain.Source{domain.SourceX, domain.SourceInstagram}}
		capability.Authority = func(ctx context.Context, request DevelopmentSessionProbeRequest) (DevelopmentSessionEvidence, error) {
			if runtime == nil || checkProfile() != nil {
				return DevelopmentSessionEvidence{}, errors.New("policy fixture authority unavailable")
			}
			v, err := runtime.authority(ctx)
			if err != nil || checkProfile() != nil {
				return DevelopmentSessionEvidence{}, errors.New("policy fixture authority rejected")
			}
			return convert(v, request), nil
		}
		capability.Browser = func(ctx context.Context, request DevelopmentSessionProbeRequest) (DevelopmentBrowserEvidence, error) {
			if runtime == nil || checkProfile() != nil {
				return DevelopmentBrowserEvidence{}, errors.New("policy fixture browser unavailable")
			}
			owner := runtime.backend.current.Load()
			if owner == nil || owner.PID() != runtime.Owner.PID() {
				return DevelopmentBrowserEvidence{}, errors.New("policy fixture browser owner mismatch")
			}
			snapshot, pid := runtime.Owner.Snapshot(), runtime.Owner.PID()
			v, err := owner.Evidence(ctx)
			if err != nil || runtime.backend.current.Load() != owner || checkProfile() != nil {
				return DevelopmentBrowserEvidence{}, errors.New("policy fixture browser proof rejected")
			}
			return DevelopmentBrowserEvidence{DevelopmentSessionEvidence: convert(v, request), EngineEpoch: engine.Epoch(), Generation: snapshot.Generation, PID: pid, Driver: snapshot.Driver}, nil
		}
		var err error
		fresh := start.PolicyRecovery == nil
		controller, err = NewSyntheticPolicyController(ctx, e, capability, fresh)
		if err != nil {
			return err
		}
		if e.captureOwner != nil || e.fixtureAdmissionAllowed() {
			return errors.New("policy fixture opened before pre-owner fence")
		}
		stage := "policy_fenced"
		receipt := processFixtureReceipt{Stage: stage, PID: uint32(os.Getpid()), Epoch: e.Epoch(), ProfileID: start.Grant.ProfileID, CheckpointFencedBeforeOwner: true}
		if !fresh {
			if e.Epoch() == start.PolicyRecovery.PreviousEpoch {
				return errors.New("policy recovery reused engine epoch")
			}
			checkpoint, policyGeneration, enabled, loadErr := e.store.LoadSyntheticAccountPolicy(ctx)
			if loadErr != nil || !enabled || checkpoint.CheckpointID != start.PolicyRecovery.CheckpointID ||
				checkpoint.Generation != start.PolicyRecovery.CheckpointGeneration || policyGeneration != start.PolicyRecovery.PolicyGeneration ||
				checkpoint.OwnerEpoch != start.PolicyRecovery.PreviousEpoch {
				return errors.New("policy recovery checkpoint changed before owner")
			}
			stage = "policy_recovery_fenced"
			receipt.Stage = stage
			receipt.GrantReplayDenied = true
			receipt.CheckpointID = checkpoint.CheckpointID
			receipt.CheckpointGeneration = checkpoint.Generation
			receipt.PolicyGeneration = policyGeneration
			receipt.StoreFileID = start.PolicyRecovery.StoreFileID
			g := start.PolicyRecovery
			claim, claimErr := e.store.ClaimCommandForCollector(ctx, g.QueuedRunID, "policy-recovery-pre-owner", "headless")
			if claimErr != nil || claim != nil {
				return errors.New("policy recovery claimed queued work before owner creation")
			}
			receipt.PreOwnerClaimDenied = true
			receipt.QueuedRunID = g.QueuedRunID
			receipt.QueuedCommandID = g.QueuedCommandID
		}
		if err := encoder.Encode(receipt); err != nil {
			return err
		}
		if !scanner.Scan() || scanner.Text() != "launch_policy_owner" {
			return errors.New("policy fixture owner launch decision missing")
		}
		return nil
	}
	hooks.run = func(ctx context.Context, r *DevelopmentFixtureRuntime, start processFixtureStart) error {
		runtime = r
		if start.PolicyRecovery != nil {
			return runDevelopmentPolicyRecoveryFixtureActions(ctx, controller, r, start)
		}
		return runDevelopmentPolicyFixtureActions(ctx, controller, r, start)
	}
	return runAuthSessionProcessFixtureConfigured(input, output, true, nil, func(start processFixtureStart) error {
		// Existing reopen grants keep their native-window release proof. Policy
		// recovery is admitted only through its separate headless grant.
		if start.Grant.Reopen || start.NativeHost != nil {
			return errors.New("policy fixture requires a fresh or headless recovery grant")
		}
		return nil
	}, hooks)
}

func runDevelopmentPolicyFixtureActions(ctx context.Context, c *SyntheticPolicyController, r *DevelopmentFixtureRuntime, start processFixtureStart) error {
	command := func(expected string) error {
		if !r.scanner.Scan() || r.scanner.Text() != expected {
			return errors.New("policy fixture decision frame rejected")
		}
		return ctx.Err()
	}
	send := func(stage string, preview *SyntheticPolicyPreview) error {
		var raw json.RawMessage
		if preview != nil {
			var err error
			raw, err = json.Marshal(preview)
			if err != nil {
				return err
			}
		}
		return r.encoder.Encode(processFixtureReceipt{Stage: stage, PID: uint32(os.Getpid()), ChromePID: r.Owner.PID(), Epoch: r.Engine.Epoch(), ProfileID: start.Grant.ProfileID, Frontend: raw,
			CheckpointFencedBeforeOwner: true, CheckpointAuthorizedAtStartup: developmentCheckpointAuthorizedAtStartup(r.Engine), CaptureActive: stage == "policy_capture_active",
			DurablePause: stage == "policy_disabled_draining", CleanupVerified: stage == "policy_opted_out" && r.bridge.cleanupVerified(),
			GracefulBrowserRelease: stage == "policy_enabled_idle"})
	}
	preview, err := c.Preview(ctx, r.Owner, "enroll")
	if err != nil {
		return err
	}
	if err := send("policy_enrollment_preview", &preview); err != nil {
		return err
	}
	if err := command("cancel_enrollment:" + preview.ID); err != nil {
		return err
	}
	if err := c.Confirm(ctx, preview.ID, false, nil); err == nil {
		return errors.New("policy fixture canceled enrollment accepted")
	}
	if err := send("policy_enrollment_canceled", nil); err != nil {
		return err
	}
	preview, err = c.Preview(ctx, r.Owner, "enroll")
	if err != nil {
		return err
	}
	if err := send("policy_enrollment_preview", &preview); err != nil {
		return err
	}
	if err := command("confirm_enrollment:" + preview.ID); err != nil {
		return err
	}
	if err := c.Confirm(ctx, preview.ID, true, nil); err != nil {
		return err
	}
	if _, _, enabled, err := r.State.LoadSyntheticAccountPolicy(ctx); err != nil || enabled {
		return errors.New("policy fixture enrollment was not disabled")
	}
	if err := send("policy_enrolled_disabled", nil); err != nil {
		return err
	}
	if err := command("preview_enable"); err != nil {
		return err
	}
	if _, err := c.Preview(ctx, r.Owner, "enable"); err == nil {
		return errors.New("policy fixture changed authority accepted")
	}
	if err := send("policy_preview_rejected", nil); err != nil {
		return err
	}
	if err := command("preview_enable"); err != nil {
		return err
	}
	preview, err = c.Preview(ctx, r.Owner, "enable")
	if err != nil {
		return err
	}
	if err := send("policy_enable_preview", &preview); err != nil {
		return err
	}
	if err := command("confirm_enable:" + preview.ID); err != nil {
		return err
	}
	if err := c.Confirm(ctx, preview.ID, true, r.Coordinator); err != nil {
		return err
	}
	if !r.scanner.Scan() {
		return errors.New("policy fixture post-confirmation decision missing")
	}
	if r.scanner.Text() == "stop_policy_worker" {
		if !developmentCheckpointAuthorizedAtStartup(r.Engine) {
			return errors.New("enabled policy did not authorize before idle worker shutdown")
		}
		if err := closeIdleDevelopmentPolicyOwner(ctx, c, r); err != nil {
			return err
		}
		return send("policy_enabled_idle", nil)
	}
	if r.scanner.Text() != "start_capture" {
		return errors.New("policy fixture post-confirmation decision rejected")
	}
	r.Engine.StartHeadlessCollection(ctx)
	select {
	case <-r.backend.entered:
	case <-ctx.Done():
		return ctx.Err()
	}
	if err := send("policy_capture_active", nil); err != nil {
		return err
	}
	if !r.scanner.Scan() {
		return errors.New("policy fixture shutdown decision missing")
	}
	if r.scanner.Text() == "stop_policy_worker" {
		return nil
	}
	if r.scanner.Text() != "disable_policy" {
		return errors.New("policy fixture shutdown decision rejected")
	}
	disabled := make(chan error, 1)
	go func() { disabled <- c.Disable(ctx) }()
	if err := processFixtureWait(ctx, func() bool {
		_, _, enabled, err := r.State.LoadSyntheticAccountPolicy(ctx)
		return err == nil && !enabled
	}); err != nil {
		return err
	}
	select {
	case err := <-disabled:
		return errors.New("policy fixture opt-out completed before active capture drained: " + policyFixtureFailure(err))
	default:
	}
	if err := send("policy_disabled_draining", nil); err != nil {
		return err
	}
	if err := command("release_capture"); err != nil {
		return err
	}
	close(r.backend.release)
	select {
	case err := <-disabled:
		if err != nil {
			return err
		}
	case <-ctx.Done():
		return ctx.Err()
	}
	if !r.bridge.cleanupVerified() || r.bridge.cleanupError() != nil {
		return errors.New("policy fixture Chrome/Node tree cleanup not verified")
	}
	if err := send("policy_opted_out", nil); err != nil {
		return err
	}
	if err := command("stop"); err != nil {
		return err
	}
	return r.encoder.Encode(processFixtureReceipt{Stage: "stopped", PID: uint32(os.Getpid()), CleanupVerified: true})
}

func closeIdleDevelopmentPolicyOwner(ctx context.Context, c *SyntheticPolicyController, r *DevelopmentFixtureRuntime) error {
	if c == nil || r == nil || r.Engine == nil || r.Owner == nil || r.Coordinator == nil || r.backend == nil {
		return errors.New("policy fixture idle owner unavailable")
	}
	c.mu.Lock()
	if c.stopped || !c.running || c.cancel == nil {
		c.mu.Unlock()
		return errors.New("policy fixture idle controller is not running")
	}
	c.hold()
	c.mu.Unlock()
	if !r.Engine.WaitForIdle(5 * time.Second) {
		return errors.New("policy fixture idle reasoning did not drain")
	}
	if err := r.Engine.PauseDevelopmentSession(ctx); err != nil {
		return errors.New("policy fixture idle admission fence did not drain")
	}
	c.mu.Lock()
	cancel := c.cancel
	done := r.Engine.developmentCoordinatorDone
	c.mu.Unlock()
	cancel()
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
			return errors.New("policy fixture idle coordinator shutdown incomplete")
		}
	}
	if !r.Engine.WaitForIdle(5 * time.Second) {
		return errors.New("policy fixture idle reasoning shutdown incomplete")
	}
	snapshot := r.Owner.Snapshot()
	if snapshot.State != captureruntime.Ready || snapshot.Driver != "headless" || snapshot.ActiveLeases != 0 {
		return errors.New("policy fixture owner was not idle before graceful release")
	}
	process := r.backend.current.Load()
	if process == nil || process.PID() != r.Owner.PID() {
		return errors.New("policy fixture owner identity changed before graceful release")
	}
	if err := process.CloseForRetry(ctx); err != nil {
		return errors.New("policy fixture graceful browser release failed")
	}
	c.mu.Lock()
	c.running = false
	c.stopped = true
	c.cancel = nil
	c.mu.Unlock()
	return nil
}

func runDevelopmentPolicyRecoveryFixtureActions(ctx context.Context, c *SyntheticPolicyController, r *DevelopmentFixtureRuntime, start processFixtureStart) error {
	g := start.PolicyRecovery
	if g == nil || start.Grant.Reopen || r.NativeHost != nil || r.Owner == nil || r.backend.entered == nil || r.backend.release == nil {
		return errors.New("policy recovery composition unavailable")
	}
	checkpoint, policyGeneration, enabled, err := r.State.LoadSyntheticAccountPolicy(ctx)
	if err != nil || !enabled || checkpoint.CheckpointID != g.CheckpointID || checkpoint.Generation != g.CheckpointGeneration ||
		policyGeneration != g.PolicyGeneration || checkpoint.OwnerEpoch != g.PreviousEpoch || r.Engine.Epoch() == g.PreviousEpoch {
		return errors.New("policy recovery checkpoint or epoch changed before owner proof")
	}
	previousCheckpoint := checkpoint
	if r.Engine.fixtureAdmissionAllowed() {
		return errors.New("policy recovery admission opened before owner proof")
	}
	queuedRunID := g.QueuedRunID
	claim, claimErr := r.State.ClaimCommandForCollector(ctx, queuedRunID, "policy-recovery-pre-proof", "headless")
	if claimErr != nil || claim != nil {
		return errors.New("policy recovery claimed queued work before fresh proof")
	}
	preResumeBrowser, preResumeBrowserErr := r.backend.current.Load().Evidence(ctx)
	preResumeBrowserProbeOK := preResumeBrowserErr == nil
	preResumeBrowserAuthenticated := preResumeBrowserErr == nil && preResumeBrowser.Authenticated
	preResumeBrowserAccountMatch := preResumeBrowserErr == nil && preResumeBrowser.AccountID == c.expected[domain.SourceX].AccountID
	preResumeBrowserRevision := preResumeBrowserErr == nil && strings.TrimSpace(preResumeBrowser.Revision) != ""
	if err := r.encoder.Encode(processFixtureReceipt{Stage: "policy_recovery_owner_ready", PID: uint32(os.Getpid()), ChromePID: r.Owner.PID(), Epoch: r.Engine.Epoch(),
		ProfileID: g.ProfileID, CheckpointFencedBeforeOwner: true, CheckpointID: checkpoint.CheckpointID,
		CheckpointGeneration: checkpoint.Generation, PolicyGeneration: policyGeneration, StoreFileID: g.StoreFileID,
		GrantReplayDenied:   true,
		PreProofClaimDenied: true, PreResumeBrowserProbeOK: preResumeBrowserProbeOK, PreResumeBrowserAuthenticated: preResumeBrowserAuthenticated,
		PreResumeBrowserAccountMatch: preResumeBrowserAccountMatch, PreResumeBrowserRevision: preResumeBrowserRevision,
		QueuedRunID: g.QueuedRunID, QueuedCommandID: g.QueuedCommandID}); err != nil {
		return err
	}
	if !r.scanner.Scan() || r.scanner.Text() != "resume_policy" {
		return errors.New("policy recovery resume decision missing")
	}
	resumeErr := c.Resume(ctx, r.Owner, r.Coordinator)
	if resumeErr != nil {
		diagnostic := collectSyntheticPolicyRecoveryProofDiagnostic(ctx, c, r, checkpoint, policyGeneration)
		if r.Engine.fixtureAdmissionAllowed() {
			return errors.New("failed policy recovery opened capture admission")
		}
		after, afterGeneration, afterEnabled, loadErr := r.State.LoadSyntheticAccountPolicy(ctx)
		if loadErr != nil || !afterEnabled || afterGeneration != policyGeneration || !syntheticCheckpointStateEqual(after, checkpoint) {
			return errors.New("failed policy recovery changed the persisted checkpoint")
		}
		claim, claimErr := r.State.ClaimCommandForCollector(ctx, queuedRunID, "policy-recovery-failed-proof", "headless")
		if claimErr != nil || claim != nil {
			return errors.New("failed policy recovery claimed queued work")
		}
		if err := r.encoder.Encode(processFixtureReceipt{Stage: "policy_resume_denied", PID: uint32(os.Getpid()), ChromePID: r.Owner.PID(), Epoch: r.Engine.Epoch(),
			ProfileID: g.ProfileID, AuthorityDenied: !diagnostic.authorityReady, CheckpointFencedBeforeOwner: true, CheckpointID: checkpoint.CheckpointID,
			CheckpointGeneration: checkpoint.Generation, PolicyGeneration: policyGeneration, StoreFileID: g.StoreFileID,
			Failure: diagnostic.String(), QueuedRunID: g.QueuedRunID, QueuedCommandID: g.QueuedCommandID}); err != nil {
			return err
		}
		if !r.scanner.Scan() || r.scanner.Text() != "stop_after_resume_denied" {
			return errors.New("failed policy recovery shutdown decision missing")
		}
		return nil
	}
	if !developmentCheckpointAuthorizedAtStartup(r.Engine) {
		return errors.New("policy recovery did not pass fresh checkpoint authorization")
	}
	r.Engine.StartHeadlessCollection(ctx)
	select {
	case <-r.backend.entered:
	case <-ctx.Done():
		return ctx.Err()
	}
	if !r.Engine.fixtureAdmissionAllowed() {
		return errors.New("policy recovery collector opened before admission was authorized")
	}
	checkpoint, policyGeneration, enabled, err = r.State.LoadSyntheticAccountPolicy(ctx)
	if err != nil || !enabled || policyGeneration != g.PolicyGeneration || checkpoint.CheckpointID != g.CheckpointID ||
		checkpoint.Generation <= g.CheckpointGeneration || checkpoint.OwnerEpoch != r.Engine.Epoch() ||
		!syntheticCheckpointExpectedEqual(checkpoint.ExpectedAccounts, previousCheckpoint.ExpectedAccounts) ||
		!reflect.DeepEqual(checkpoint.AcceptedRevisions, previousCheckpoint.AcceptedRevisions) {
		return errors.New("resumed policy checkpoint did not bind the new owner")
	}
	if err := r.encoder.Encode(processFixtureReceipt{Stage: "policy_resumed_active", PID: uint32(os.Getpid()), ChromePID: r.Owner.PID(), Epoch: r.Engine.Epoch(),
		Generation: r.Owner.Snapshot().Generation,
		ProfileID:  g.ProfileID, CaptureActive: true, CheckpointAuthorizedAtStartup: true, CheckpointID: checkpoint.CheckpointID,
		CheckpointGeneration: checkpoint.Generation, PolicyGeneration: policyGeneration, StoreFileID: g.StoreFileID,
		QueuedRunID: g.QueuedRunID, QueuedCommandID: g.QueuedCommandID}); err != nil {
		return err
	}
	if !r.scanner.Scan() || r.scanner.Text() != "release_capture" {
		return errors.New("policy recovery capture release decision missing")
	}
	close(r.backend.release)
	if !r.Engine.WaitForIdle(5 * time.Second) {
		return errors.New("resumed policy capture did not drain")
	}
	if err := processFixtureWait(ctx, func() bool {
		run, loadErr := r.State.GetRun(ctx, g.QueuedRunID)
		return loadErr == nil && run.BridgeCommandStatus == "completed"
	}); err != nil {
		return errors.New("resumed policy queue command did not complete")
	}
	if err := r.encoder.Encode(processFixtureReceipt{Stage: "policy_recovered_drained", PID: uint32(os.Getpid()), ChromePID: r.Owner.PID(), Epoch: r.Engine.Epoch(),
		ProfileID: g.ProfileID, CheckpointAuthorizedAtStartup: true, CheckpointID: checkpoint.CheckpointID,
		CheckpointGeneration: checkpoint.Generation, PolicyGeneration: policyGeneration, StoreFileID: g.StoreFileID,
		QueueCompleted: true, QueuedRunID: g.QueuedRunID, QueuedCommandID: g.QueuedCommandID}); err != nil {
		return err
	}
	if !r.scanner.Scan() || r.scanner.Text() != "stop_policy_worker" {
		return errors.New("policy recovery graceful shutdown decision missing")
	}
	return nil
}

type syntheticPolicyRecoveryProofDiagnostic struct {
	phase, owner, checkpoint                                                                                                                           string
	authorityReady, browserReady, revisionsMatch, browserProbeOK, browserAuthenticated, browserAccountMatch, browserRevisionPresent, browserOwnerMatch bool
}

func (d syntheticPolicyRecoveryProofDiagnostic) String() string {
	return fmt.Sprintf("phase=%s ownerReady=%s authorityReady=%t browserReady=%t revisionsMatch=%t browserProbeOK=%t browserAuthenticated=%t browserAccountMatch=%t browserRevisionPresent=%t browserOwnerMatch=%t checkpointUnchanged=%s",
		d.phase, d.owner, d.authorityReady, d.browserReady, d.revisionsMatch, d.browserProbeOK, d.browserAuthenticated, d.browserAccountMatch,
		d.browserRevisionPresent, d.browserOwnerMatch, d.checkpoint)
}

func collectSyntheticPolicyRecoveryProofDiagnostic(ctx context.Context, c *SyntheticPolicyController, r *DevelopmentFixtureRuntime,
	previous store.SyntheticAccountCheckpoint, previousPolicyGeneration uint64) syntheticPolicyRecoveryProofDiagnostic {
	d := syntheticPolicyRecoveryProofDiagnostic{phase: c.lastStartFailure, owner: "unavailable", checkpoint: "unavailable"}
	if r == nil || r.Engine == nil || r.Owner == nil || c == nil {
		return d
	}
	snapshot := r.Owner.Snapshot()
	d.owner = "ready"
	if r.Engine.captureOwner != r.Owner || r.Owner.PID() <= 0 || snapshot.State != captureruntime.Ready || snapshot.Driver != "headless" {
		d.owner = "rejected"
	}
	d.authorityReady, d.browserReady, d.revisionsMatch = true, true, true
	d.browserProbeOK, d.browserAuthenticated, d.browserAccountMatch, d.browserRevisionPresent, d.browserOwnerMatch = true, true, true, true, true
	for _, source := range c.capability.Sources {
		request := DevelopmentSessionProbeRequest{Version: DevelopmentSessionProbeVersion, Source: source, ProfileID: c.capability.ProfileID,
			Kind: DevelopmentSessionProbeAuthority, ProbeID: domain.NewID("recovery-diagnostic"), RequestedAt: time.Now().UTC()}
		authority, authorityErr := c.capability.Authority(ctx, request)
		authorityOK := authorityErr == nil && authority.Version == request.Version && authority.Source == source &&
			authority.Kind == request.Kind && authority.Domain == "synthetic-fixture" && authority.ProfileID == request.ProfileID &&
			authority.AuthState == DevelopmentSessionAuthAuthenticated && authority.AccountID == c.expected[source].AccountID && strings.TrimSpace(authority.Revision) != ""
		if !authorityOK {
			d.authorityReady = false
		}
		request.Kind = DevelopmentSessionProbeBrowser
		request.ProbeID = domain.NewID("recovery-diagnostic")
		request.RequestedAt = time.Now().UTC()
		browser, browserErr := c.capability.Browser(ctx, request)
		browserEvidence := browser.DevelopmentSessionEvidence
		if browserErr != nil {
			d.browserProbeOK = false
		}
		if browserErr != nil || browserEvidence.AuthState != DevelopmentSessionAuthAuthenticated {
			d.browserAuthenticated = false
		}
		if browserErr != nil || browserEvidence.AccountID != c.expected[source].AccountID {
			d.browserAccountMatch = false
		}
		if browserErr != nil || strings.TrimSpace(browserEvidence.Revision) == "" {
			d.browserRevisionPresent = false
		}
		if browserErr != nil || browser.EngineEpoch != r.Engine.Epoch() || browser.Generation != snapshot.Generation || browser.PID != r.Owner.PID() || browser.Driver != snapshot.Driver {
			d.browserOwnerMatch = false
		}
		browserOK := browserErr == nil && browserEvidence.Version == request.Version && browserEvidence.Source == source &&
			browserEvidence.Kind == request.Kind && browserEvidence.Domain == "synthetic-fixture" && browserEvidence.ProfileID == request.ProfileID &&
			browserEvidence.AuthState == DevelopmentSessionAuthAuthenticated && browserEvidence.AccountID == c.expected[source].AccountID && strings.TrimSpace(browserEvidence.Revision) != "" &&
			browser.EngineEpoch == r.Engine.Epoch() && browser.Generation == snapshot.Generation && browser.PID == r.Owner.PID() && browser.Driver == snapshot.Driver
		if !browserOK {
			d.browserReady = false
		}
		if !authorityOK || !browserOK || authority.AccountID != browserEvidence.AccountID || authority.Revision != browserEvidence.Revision {
			d.revisionsMatch = false
		}
	}
	checkpoint, policyGeneration, enabled, err := r.State.LoadSyntheticAccountPolicy(ctx)
	if err == nil && enabled && policyGeneration == previousPolicyGeneration && syntheticCheckpointStateEqual(checkpoint, previous) {
		d.checkpoint = "unchanged"
	} else {
		d.checkpoint = "changed_or_unavailable"
	}
	return d
}

func policyFixtureFailure(err error) string {
	if err == nil {
		return "unexpected success"
	}
	return "closed with error"
}
