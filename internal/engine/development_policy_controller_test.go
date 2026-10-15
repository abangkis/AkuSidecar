//go:build auth_session_dev && auth_session_fixture

package engine

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func newPolicyControllerHarness(t *testing.T, path string, fresh bool, backend *syntheticCaptureBackend, edit func(*DevelopmentSessionCapability)) (*syntheticSidecarHarness, *SyntheticPolicyController) {
	t.Helper()
	var controller *SyntheticPolicyController
	h := newSyntheticSidecarHarnessBeforeOwner(t, path, backend, func(h *syntheticSidecarHarness) {
		capability := developmentCapability(h)
		if edit != nil {
			edit(&capability)
		}
		var err error
		controller, err = NewSyntheticPolicyController(h.ctx, h.e, capability, fresh)
		if err != nil {
			t.Fatal(err)
		}
		if h.owners.Load() != 0 || h.e.captureOwner != nil || h.e.fixtureAdmissionAllowed() {
			t.Fatal("controller did not close before owner creation")
		}
	})
	return h, controller
}

func policyEnroll(t *testing.T, h *syntheticSidecarHarness, c *SyntheticPolicyController) {
	t.Helper()
	preview, err := c.Preview(h.ctx, h.m, "enroll")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Confirm(h.ctx, preview.ID, true, nil); err != nil {
		t.Fatal(err)
	}
}

func policyEnable(t *testing.T, h *syntheticSidecarHarness, c *SyntheticPolicyController) {
	t.Helper()
	preview, err := c.Preview(h.ctx, h.m, "enable")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Confirm(h.ctx, preview.ID, true, h.c); err != nil {
		t.Fatal(err)
	}
	syntheticWait(t, func() bool { return !h.c.Status().BatchPaused }, "policy verified admission")
}

func policyMetadata(t *testing.T, h *syntheticSidecarHarness) map[string]string {
	t.Helper()
	db, err := sql.Open("sqlite", h.path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.Query(`SELECT key,value FROM meta WHERE key LIKE 'synthetic_account_checkpoint%' ORDER BY key`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	result := map[string]string{}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			t.Fatal(err)
		}
		result[key] = value
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestDevelopmentPolicyControllerEnrollmentCancelAndReplay(t *testing.T) {
	h, c := newPolicyControllerHarness(t, "", true, &syntheticCaptureBackend{}, nil)
	_, run, command := queueDevelopmentFixtureCommand(t, h, "policy pending enrollment", false)
	before := readDevelopmentQueuedCommandSnapshot(t, h, command)
	preview, err := c.Preview(h.ctx, h.m, "enroll")
	if err != nil {
		t.Fatal(err)
	}
	preview.Accounts[domain.SourceX] = "mutated UI copy"
	if err := c.Confirm(h.ctx, preview.ID, false, nil); err == nil {
		t.Fatal("canceled confirmation accepted")
	}
	if err := c.Confirm(h.ctx, preview.ID, true, nil); err == nil {
		t.Fatal("canceled confirmation replay accepted")
	}
	if _, exists := policyMetadata(t, h)["synthetic_account_checkpoint"]; exists {
		t.Fatal("cancel persisted account enrollment")
	}
	if claim, _ := h.state.ClaimCommandForCollector(h.ctx, run, "unconfirmed", "headless"); claim != nil {
		t.Fatal("unconfirmed enrollment admitted queued work")
	}
	policyEnroll(t, h, c)
	cp, _, enabled, err := h.state.LoadSyntheticAccountPolicy(h.ctx)
	if err != nil || enabled || cp.ExpectedAccounts[domain.SourceX].AccountID != "alpha" {
		t.Fatal("enrollment was not immutable/default-off", err)
	}
	if err := c.Resume(h.ctx, h.m, h.c); err == nil {
		t.Fatal("disabled enrollment resumed")
	}
	if after := readDevelopmentQueuedCommandSnapshot(t, h, command); after != before {
		t.Fatal("confirmation/cancel mutated queue")
	}
}

func TestDevelopmentPolicyControllerEnableNeedsFreshProofAndDisableDrains(t *testing.T) {
	backend := &syntheticCaptureBackend{entered: make(chan struct{}), release: make(chan struct{})}
	h, c := newPolicyControllerHarness(t, "", true, backend, nil)
	policyEnroll(t, h, c)
	_, _, command := queueDevelopmentFixtureCommand(t, h, "policy verified capture", false)
	policyEnable(t, h, c)
	h.e.StartHeadlessCollection(h.ctx)
	select {
	case <-backend.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("verified policy did not dispatch capture")
	}
	if row := readDevelopmentQueuedCommandSnapshot(t, h, command); row.status != "claimed" {
		t.Fatal("capture was not actually claimed")
	}
	disabled := make(chan error, 1)
	go func() { disabled <- c.Disable(h.ctx) }()
	syntheticWait(t, func() bool {
		_, _, enabled, err := h.state.LoadSyntheticAccountPolicy(h.ctx)
		return err == nil && !enabled
	}, "durable opt-out before active drain")
	select {
	case err := <-disabled:
		t.Fatal("opt-out returned before capture drained", err)
	default:
	}
	if h.initial.closed.Load() {
		t.Fatal("opt-out killed active owner")
	}
	close(backend.release)
	select {
	case err := <-disabled:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("opt-out did not finish after drain")
	}
	if !h.initial.closed.Load() || h.owners.Load() != 0 {
		t.Fatal("opt-out left its owner alive")
	}
	if row := readDevelopmentQueuedCommandSnapshot(t, h, command); row.status != "completed" {
		t.Fatalf("active work lost: %+v", row)
	}
	if err := c.Resume(h.ctx, h.m, h.c); err == nil {
		t.Fatal("stopped controller resumed")
	}
}

func TestDevelopmentPolicyControllerRejectsChangedExpiredCanceledAndForeignConfirmations(t *testing.T) {
	for _, mode := range []string{"expired", "canceled-context", "canceled-lifetime", "foreign-id", "changed-account", "changed-revision", "changed-owner", "stale-checkpoint", "stale-observation"} {
		t.Run(mode, func(t *testing.T) {
			var changed atomic.Bool
			h, c := newPolicyControllerHarness(t, "", true, &syntheticCaptureBackend{}, func(cap *DevelopmentSessionCapability) {
				authority := cap.Authority
				cap.Authority = func(ctx context.Context, r DevelopmentSessionProbeRequest) (DevelopmentSessionEvidence, error) {
					v, err := authority(ctx, r)
					if changed.Load() {
						if mode == "changed-account" {
							v.AccountID = "another-private-account"
						}
						if mode == "changed-revision" {
							v.Revision = "changed-private-session"
						}
						if mode == "stale-observation" {
							v.ObservedAt = r.RequestedAt.Add(-time.Second)
						}
					}
					return v, err
				}
				browser := cap.Browser
				cap.Browser = func(ctx context.Context, r DevelopmentSessionProbeRequest) (DevelopmentBrowserEvidence, error) {
					v, err := browser(ctx, r)
					if changed.Load() && mode == "changed-revision" {
						v.Revision = "changed-private-session"
					}
					return v, err
				}
			})
			policyEnroll(t, h, c)
			_, _, command := queueDevelopmentFixtureCommand(t, h, "reject stale policy decision", false)
			before := readDevelopmentQueuedCommandSnapshot(t, h, command)
			preview, err := c.Preview(h.ctx, h.m, "enable")
			if err != nil {
				t.Fatal(err)
			}
			ctx := h.ctx
			id := preview.ID
			switch mode {
			case "expired":
				c.clock = func() time.Time { return time.Now().UTC().Add(3 * time.Minute) }
			case "canceled-context":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "foreign-id":
				id = "unissued-confirmation"
			case "canceled-lifetime":
				h.cancel()
				ctx = context.Background()
			case "changed-account", "changed-revision", "stale-observation":
				changed.Store(true)
			case "changed-owner":
				h.m.Terminate()
			case "stale-checkpoint":
				cp, g, _, err := h.state.LoadSyntheticAccountPolicy(h.ctx)
				if err != nil {
					t.Fatal(err)
				}
				if err := h.state.ApplySyntheticAccountPolicy(h.ctx, cp, g, "disable"); err != nil {
					t.Fatal(err)
				}
			}
			err = c.Confirm(ctx, id, true, h.c)
			if err == nil {
				t.Fatal("invalid confirmation accepted")
			}
			if strings.Contains(err.Error(), "private-account") || strings.Contains(err.Error(), "private-session") {
				t.Fatal("error leaked observation")
			}
			if err := c.Confirm(h.ctx, preview.ID, true, h.c); err == nil {
				t.Fatal("failed decision replayed")
			}
			_, _, enabled, err := h.state.LoadSyntheticAccountPolicy(context.Background())
			if err != nil || enabled {
				t.Fatal("failed decision enabled policy", err)
			}
			if after := readDevelopmentQueuedCommandSnapshot(t, &syntheticSidecarHarness{ctx: context.Background(), path: h.path}, command); after != before {
				t.Fatal("failed decision modified queue")
			}
		})
	}
}

func TestDevelopmentPolicyControllerPostConfirmStartupProofFailsClosed(t *testing.T) {
	var armed atomic.Bool
	var calls atomic.Int64
	h, c := newPolicyControllerHarness(t, "", true, &syntheticCaptureBackend{}, func(cap *DevelopmentSessionCapability) {
		browser := cap.Browser
		cap.Browser = func(ctx context.Context, r DevelopmentSessionProbeRequest) (DevelopmentBrowserEvidence, error) {
			v, err := browser(ctx, r)
			if armed.Load() && calls.Add(1) > 2 {
				v.AuthState = DevelopmentSessionAuthLoggedOut
			}
			return v, err
		}
	})
	policyEnroll(t, h, c)
	_, run, command := queueDevelopmentFixtureCommand(t, h, "post-confirm proof", false)
	before := readDevelopmentQueuedCommandSnapshot(t, h, command)
	preview, err := c.Preview(h.ctx, h.m, "enable")
	if err != nil {
		t.Fatal(err)
	}
	armed.Store(true) // two sources during confirmation pass; startup must observe again.
	if err := c.Confirm(h.ctx, preview.ID, true, h.c); err == nil {
		t.Fatal("startup reused preview/confirmation proof")
	}
	if h.e.fixtureAdmissionAllowed() {
		t.Fatal("failed startup opened local admission")
	}
	if claim, _ := h.state.ClaimCommandForCollector(h.ctx, run, "failed-startup", "headless"); claim != nil {
		t.Fatal("failed startup left Store permit")
	}
	if after := readDevelopmentQueuedCommandSnapshot(t, h, command); after != before {
		t.Fatal("failed startup changed queue")
	}
}

func TestDevelopmentPolicyControllerColdResumeVerifiesWithoutEnrollment(t *testing.T) {
	h, c := newPolicyControllerHarness(t, "", true, &syntheticCaptureBackend{}, nil)
	policyEnroll(t, h, c)
	policyEnable(t, h, c)
	cp, _, _, err := h.state.LoadSyntheticAccountPolicy(h.ctx)
	if err != nil {
		t.Fatal(err)
	}
	// End only this synthetic owner/runtime before opening a successor controller.
	c.cancel()
	h.e.Shutdown()
	h.m.Terminate()
	select {
	case <-h.e.developmentCoordinatorDone:
	case <-time.After(time.Second):
		t.Fatal("old polling not joined")
	}
	if err := h.state.Close(); err != nil {
		t.Fatal(err)
	}
	var probes atomic.Int64
	next, resumed := newPolicyControllerHarness(t, h.path, false, &syntheticCaptureBackend{}, func(cap *DevelopmentSessionCapability) {
		browser := cap.Browser
		cap.Browser = func(ctx context.Context, r DevelopmentSessionProbeRequest) (DevelopmentBrowserEvidence, error) {
			probes.Add(1)
			return browser(ctx, r)
		}
	})
	if next.e.fixtureAdmissionAllowed() {
		t.Fatal("cold controller inherited admission")
	}
	if err := resumed.Resume(next.ctx, next.m, next.c); err != nil {
		t.Fatal(err)
	}
	syntheticWait(t, func() bool { return !next.c.Status().BatchPaused }, "fresh cold proof")
	after, _, _, err := next.state.LoadSyntheticAccountPolicy(next.ctx)
	a, _ := json.Marshal(cp.ExpectedAccounts)
	b, _ := json.Marshal(after.ExpectedAccounts)
	if err != nil || string(a) != string(b) || after.OwnerEpoch == cp.OwnerEpoch || probes.Load() == 0 {
		t.Fatal("cold resume reenrolled or skipped new owner proof", err)
	}
}

func TestDevelopmentPolicyControllerLegacyMigrationAndDisabledSuccessor(t *testing.T) {
	h := newSyntheticSidecarHarness(t, "", &syntheticCaptureBackend{})
	capability := developmentCapability(h)
	if err := h.e.PrepareDevelopmentSessionCheckpoint(h.ctx, capability, true); err != nil {
		t.Fatal(err)
	}
	c, err := NewSyntheticPolicyController(h.ctx, h.e, capability, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Preview(h.ctx, h.m, "enable"); err == nil {
		t.Fatal("legacy checkpoint skipped explicit migration")
	}
	preview, err := c.Preview(h.ctx, h.m, "migrate")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Confirm(h.ctx, preview.ID, true, nil); err != nil {
		t.Fatal(err)
	}
	_, generation, enabled, err := h.state.LoadSyntheticAccountPolicy(h.ctx)
	if err != nil || generation != 1 || enabled {
		t.Fatal("migration enabled capture", err)
	}
	if err := c.Disable(h.ctx); err != nil {
		t.Fatal(err)
	}
	if !h.initial.closed.Load() {
		t.Fatal("disabled preview owner survived opt-out")
	}
	next, successor := newPolicyControllerHarness(t, h.path, false, &syntheticCaptureBackend{}, nil)
	if err := successor.Resume(next.ctx, next.m, next.c); err == nil {
		t.Fatal("cold successor ignored opt-out")
	}
	policyEnable(t, next, successor)
}

func TestDevelopmentPolicyControllerCanceledDrainRetainsPolicyAndWork(t *testing.T) {
	backend := &syntheticCaptureBackend{entered: make(chan struct{}), release: make(chan struct{})}
	h, c := newPolicyControllerHarness(t, "", true, backend, nil)
	policyEnroll(t, h, c)
	_, _, command := queueDevelopmentFixtureCommand(t, h, "bounded opt-out drain", false)
	policyEnable(t, h, c)
	h.e.StartHeadlessCollection(h.ctx)
	select {
	case <-backend.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("capture did not enter")
	}
	ctx, cancel := context.WithCancel(h.ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- c.Disable(ctx) }()
	syntheticWait(t, func() bool {
		_, _, enabled, err := h.state.LoadSyntheticAccountPolicy(h.ctx)
		return err == nil && !enabled
	}, "opt-out persisted before canceling drain")
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("unfinished drain reported full opt-out completion")
		}
	case <-time.After(time.Second):
		t.Fatal("canceled drain did not stop waiting")
	}
	_, _, enabled, err := h.state.LoadSyntheticAccountPolicy(h.ctx)
	if err != nil || enabled || h.initial.closed.Load() {
		t.Fatal("failed drain lost disabled policy or killed active owner", err)
	}
	if row := readDevelopmentQueuedCommandSnapshot(t, h, command); row.status != "claimed" {
		t.Fatal("drain timeout modified active work")
	}
	close(backend.release)
	syntheticWait(t, func() bool { return h.e.WaitForIdle(time.Millisecond) }, "remaining capture finishes after canceled drain")
	if err := c.Disable(h.ctx); err != nil {
		t.Fatal(err)
	}
}

func TestDevelopmentPolicyControllerConfirmationRequestDoesNotOwnRuntime(t *testing.T) {
	h, c := newPolicyControllerHarness(t, "", true, &syntheticCaptureBackend{}, nil)
	policyEnroll(t, h, c)
	preview, err := c.Preview(h.ctx, h.m, "enable")
	if err != nil {
		t.Fatal(err)
	}
	request, cancel := context.WithCancel(h.ctx)
	if err := c.Confirm(request, preview.ID, true, h.c); err != nil {
		cancel()
		t.Fatal(err)
	}
	cancel()
	select {
	case <-h.e.developmentCoordinatorDone:
		t.Fatal("completed confirmation request stopped runtime")
	case <-time.After(30 * time.Millisecond):
	}
	if err := c.Disable(h.ctx); err != nil {
		t.Fatal(err)
	}
}
