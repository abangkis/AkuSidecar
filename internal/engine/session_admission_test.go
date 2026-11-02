package engine

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/collection"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/store"
)

type sessionAdmissionFixture struct {
	e       *Engine
	state   *store.Store
	db      *sql.DB
	manager *captureruntime.Manager
}

func newSessionAdmissionFixture(t *testing.T) sessionAdmissionFixture {
	t.Helper()
	e, state := testEngine(t)
	manager := attachTestCapture(t, e)
	db, err := sql.Open("sqlite", state.Path())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	t.Cleanup(func() {
		e.Shutdown()
		if !e.WaitForIdle(3 * time.Second) {
			t.Error("session admission workers did not retire")
		}
	})
	return sessionAdmissionFixture{e, state, db, manager}
}
func (f sessionAdmissionFixture) exec(t *testing.T, query string, args ...any) {
	t.Helper()
	if _, err := f.db.Exec(query, args...); err != nil {
		t.Fatal(err)
	}
}
func (f sessionAdmissionFixture) countSessions(t *testing.T) int {
	t.Helper()
	var count int
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}
func admissionVisiblePolicy() domain.UpdatePolicy {
	return domain.UpdatePolicy{Trigger: domain.UpdateTriggerUser, Delivery: domain.UpdateDeliveryVisible, BudgetAuthority: domain.BudgetAuthorityUser}
}

func admissionHeartbeatWithoutX() domain.BridgeHeartbeat {
	h := ExpectedHeartbeat()
	h.SourceAccess.GrantedSources = []string{"linkedin", "instagram", "facebook"}
	for i := range h.SourceAccess.Sources {
		if h.SourceAccess.Sources[i].Source == "x" {
			h.SourceAccess.Sources[i] = domain.BridgeSourceReadiness{Source: "x", Reason: "permission_not_granted"}
		}
	}
	return h
}
func (f sessionAdmissionFixture) cancellationProbe(t *testing.T) *[2]int {
	t.Helper()
	calls := new([2]int)
	f.e.mu.Lock()
	f.e.work.registerActive("ai:admission:old", func() { calls[0]++ })
	f.e.work.registerActive("media-provenance", func() { calls[1]++ })
	f.e.mu.Unlock()
	t.Cleanup(func() {
		f.e.mu.Lock()
		f.e.work.retireActive("ai:admission:old")
		f.e.work.retireActive("media-provenance")
		f.e.mu.Unlock()
	})
	return calls
}

func TestEngineSessionAdmissionCharacterizationPolicy(t *testing.T) {
	for _, mode := range []string{"not_started", "pending", "completed", "prepared"} {
		t.Run(mode, func(t *testing.T) {
			f := newSessionAdmissionFixture(t)
			f.exec(t, `INSERT INTO meta(key,value) VALUES('calibration_first_run_status',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, mode)
			var session domain.Session
			var err error
			want := admissionVisiblePolicy()
			if mode == "prepared" {
				want = domain.UpdatePolicy{Trigger: domain.UpdateTriggerScheduler, Delivery: domain.UpdateDeliveryPrepared, BudgetAuthority: domain.BudgetAuthorityAutomatic}
				session, err = f.e.startSession(context.Background(), "offline prepared admission", want)
			} else {
				session, err = f.e.StartVisibleUpdate(context.Background(), "offline visible admission")
				if mode == "pending" {
					want.Trigger = domain.UpdateTriggerOnboarding
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			if session.Trigger != want.Trigger || session.Delivery != want.Delivery || session.BudgetAuthority != want.BudgetAuthority {
				t.Fatalf("policy=%s/%s/%s want=%+v", session.Trigger, session.Delivery, session.BudgetAuthority, want)
			}
			if f.countSessions(t) != 1 || f.manager.Snapshot().ActiveLeases != 1 {
				t.Fatal("session or capture lease not retained")
			}
			f.e.captureMu.Lock()
			lease := f.e.captureSessions[session.ID]
			f.e.captureMu.Unlock()
			if lease == nil {
				t.Fatal("durable session has no remembered lease")
			}
			var batches int
			if err := f.db.QueryRow(`SELECT COUNT(*) FROM auto_update_batches WHERE session_id=?`, session.ID).Scan(&batches); err != nil {
				t.Fatal(err)
			}
			if (mode == "prepared" && batches != 1) || (mode != "prepared" && batches != 0) {
				t.Fatalf("prepared rows=%d", batches)
			}
		})
	}
}

func TestEngineSessionAdmissionCharacterizationGuards(t *testing.T) {
	for _, mode := range []string{"transition-before-context", "health-before-policy", "policy-before-onboarding", "onboarding-before-bridge", "calibration-before-bridge", "bridge", "missing-headless-runtime", "mode-mismatch", "no-sources", "development"} {
		t.Run(mode, func(t *testing.T) {
			f := newSessionAdmissionFixture(t)
			calls := f.cancellationProbe(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			policy := admissionVisiblePolicy()
			want := ""
			switch mode {
			case "transition-before-context":
				c := collection.NewCoordinator(f.manager, nil, func() error { return nil })
				f.e.AttachCollectionCoordinator(c)
				c.Request("headless")
				cancel()
				want = "collection mode transition is pending"
			case "health-before-policy":
				f.exec(t, `INSERT INTO observations(id,run_id,command_id,source,observation_json,captured_at,created_at) VALUES('orphan','missing','missing','x','{}',?,?)`, domain.Now(), domain.Now())
				policy.Trigger = "invalid"
				want = "database cleanup required"
			case "policy-before-onboarding":
				policy.Trigger = "invalid"
				f.exec(t, `UPDATE meta SET value='not_started' WHERE key='onboarding_status'`)
				want = policy.Validate().Error()
			case "onboarding-before-bridge":
				f.exec(t, `UPDATE meta SET value='not_started' WHERE key='onboarding_status'`)
				f.e.ResetCaptureHeartbeat()
				want = "complete onboarding before starting an update"
			case "calibration-before-bridge":
				settings, err := f.state.GetSettings(ctx)
				if err != nil {
					t.Fatal(err)
				}
				session, err := f.state.CreateUpdateSession(ctx, "offline calibration parent", settings, admissionVisiblePolicy())
				if err != nil {
					t.Fatal(err)
				}
				f.exec(t, `INSERT INTO calibration_sessions(id,session_id,trigger_kind,status,max_items,sample_count,created_at,updated_at) VALUES('active-calibration',?,'manual','reviewing',2,1,?,?)`, session.ID, domain.Now(), domain.Now())
				f.e.ResetCaptureHeartbeat()
				want = "complete the active calibration"
			case "bridge":
				f.e.ResetCaptureHeartbeat()
				want = "AkuBridge v2 is not ready"
			case "missing-headless-runtime", "mode-mismatch":
				settings, err := f.state.GetSettings(ctx)
				if err != nil {
					t.Fatal(err)
				}
				settings.CollectionMode = "headless"
				if err = f.state.SaveSettings(ctx, settings); err != nil {
					t.Fatal(err)
				}
				want = "hybrid collection requires the managed capture runtime"
				if mode == "mode-mismatch" {
					f.e.AttachCollectionCoordinator(collection.NewCoordinator(f.manager, nil, func() error { return nil }))
					want = "collection runtime does not match the saved collection mode"
				}
			case "no-sources":
				h := ExpectedHeartbeat()
				h.SourceAccess.GrantedSources = nil
				for i := range h.SourceAccess.Sources {
					h.SourceAccess.Sources[i].Ready = false
					h.SourceAccess.Sources[i].PermissionGranted = false
					h.SourceAccess.Sources[i].ScriptRegistered = false
				}
				f.e.RecordHeartbeat(h)
				want = noGrantedActiveSourceError().Error()
			case "development":
				f.e.developmentSources = map[domain.Source]bool{domain.SourceX: true}
				want = "development session requires headless collection"
			}
			before := f.countSessions(t)
			_, err := f.e.startSession(ctx, "rejected offline admission", policy)
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("err=%v want=%q", err, want)
			}
			if f.countSessions(t) != before || f.manager.Snapshot().ActiveLeases != 0 || *calls != [2]int{} {
				t.Fatalf("rejected admission changed session/lease/enrichment: %v", *calls)
			}
			if !f.e.operation.TryLock() {
				t.Fatal("rejected admission retained operation lock")
			}
			f.e.operation.Unlock()
		})
	}
}

func TestEngineSessionAdmissionCharacterizationSourceSelection(t *testing.T) {
	for _, mode := range []string{"browser", "headless", "incompatible"} {
		t.Run(mode, func(t *testing.T) {
			f := newSessionAdmissionFixture(t)
			settings, err := f.state.GetSettings(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			settings.ActiveSources = []domain.Source{domain.SourceInstagram, domain.SourceX, domain.SourceLinkedIn}
			h := admissionHeartbeatWithoutX()
			f.e.RecordHeartbeat(h)
			if mode == "headless" {
				c := collection.NewCoordinator(f.manager, nil, func() error { return nil })
				f.e.AttachCollectionCoordinator(c)
				c.Request("headless")
				if err := f.manager.Replace(context.Background(), func(context.Context, uint64) (captureruntime.Process, error) {
					return &headlessTestProcess{&runtimeTestProcess{done: make(chan error, 1)}}, nil
				}); err != nil {
					t.Fatal(err)
				}
				f.e.mu.Lock()
				f.e.headlessAccess = []domain.Source{domain.SourceLinkedIn, domain.SourceInstagram}
				f.e.mu.Unlock()
			}
			if mode == "incompatible" {
				f.e.ResetCaptureHeartbeat()
			}
			before := append([]domain.Source(nil), settings.ActiveSources...)
			got := f.e.grantedActiveSources(settings)
			want := []domain.Source{domain.SourceInstagram, domain.SourceLinkedIn}
			if mode == "incompatible" {
				want = nil
			}
			if !reflect.DeepEqual(got, want) || !reflect.DeepEqual(settings.ActiveSources, before) {
				t.Fatalf("sources=%v want=%v settings=%v", got, want, settings.ActiveSources)
			}
		})
	}
	const want = "No active source is ready in AkuBridge. Open AkuBrowser Settings, use each source's Grant access or Sign in action, accept Chrome's permission prompt, and let its capture script register."
	if noGrantedActiveSourceError().Error() != want {
		t.Fatal("shared error contract changed")
	}
}

func TestEngineSessionAdmissionCharacterizationFilteredSettings(t *testing.T) {
	f := newSessionAdmissionFixture(t)
	settings, err := f.state.GetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	settings.ActiveSources = []domain.Source{domain.SourceInstagram, domain.SourceX, domain.SourceLinkedIn}
	if err = f.state.SaveSettings(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
	h := admissionHeartbeatWithoutX()
	f.e.RecordHeartbeat(h)
	session, err := f.e.StartVisibleUpdate(context.Background(), "offline filtered admission")
	if err != nil {
		t.Fatal(err)
	}
	if len(session.Runs) != 2 || session.Runs[0].Source != domain.SourceInstagram || session.Runs[1].Source != domain.SourceLinkedIn {
		t.Fatalf("admitted runs=%+v", session.Runs)
	}
	saved, err := f.state.GetSettings(context.Background())
	if err != nil || !reflect.DeepEqual(saved.ActiveSources, settings.ActiveSources) {
		t.Fatalf("admission persisted filtered preferences: %v %v", saved.ActiveSources, err)
	}
}

func TestEngineSessionAdmissionCharacterizationCreationFailures(t *testing.T) {
	for _, mode := range []string{"invalid-intent", "store-write", "store-readback", "unavailable-owner", "schedule-write"} {
		t.Run(mode, func(t *testing.T) {
			f := newSessionAdmissionFixture(t)
			calls := f.cancellationProbe(t)
			intent := "offline creation failure"
			switch mode {
			case "invalid-intent":
				intent = strings.Repeat("x", 10001)
			case "store-write":
				f.exec(t, `CREATE TRIGGER admission_create_abort BEFORE INSERT ON sessions BEGIN SELECT RAISE(ABORT,'offline session insert rejected'); END`)
			case "store-readback":
				f.exec(t, `CREATE TRIGGER admission_readback_failure AFTER INSERT ON sessions BEGIN UPDATE sessions SET max_items_total=1.5 WHERE id=NEW.id; END`)
			case "unavailable-owner":
				if err := f.manager.Replace(context.Background(), func(context.Context, uint64) (captureruntime.Process, error) {
					return nil, errors.New("offline owner unavailable")
				}); err == nil {
					t.Fatal("owner replacement unexpectedly succeeded")
				}
			case "schedule-write":
				f.exec(t, `CREATE TRIGGER admission_schedule_abort BEFORE INSERT ON bridge_commands BEGIN SELECT RAISE(ABORT,'offline initial scheduling rejected'); END`)
			}
			_, err := f.e.StartVisibleUpdate(context.Background(), intent)
			if err == nil {
				t.Fatal("creation/scheduling failure lost")
			}
			if *calls != [2]int{1, 1} {
				t.Fatalf("creation failure moved before enrichment cancellation: %v", *calls)
			}
			wantSessions, wantLeases := 0, 0
			if mode == "store-readback" {
				wantSessions = 1
			}
			if mode == "schedule-write" {
				wantSessions, wantLeases = 1, 1
			}
			if f.countSessions(t) != wantSessions || f.manager.Snapshot().ActiveLeases != wantLeases {
				t.Fatalf("durability/lease failure contract: sessions=%d leases=%d err=%v", f.countSessions(t), f.manager.Snapshot().ActiveLeases, err)
			}
			f.e.captureMu.Lock()
			remembered := len(f.e.captureSessions)
			f.e.captureMu.Unlock()
			if remembered != wantLeases {
				t.Fatalf("remembered leases=%d want=%d", remembered, wantLeases)
			}
			if !f.e.operation.TryLock() {
				t.Fatal("failure retained operation lock")
			}
			f.e.operation.Unlock()
		})
	}
}

func TestEngineSessionAdmissionCharacterizationLockAndContext(t *testing.T) {
	for _, cancelRequest := range []bool{false, true} {
		name := "release-before-schedule"
		if cancelRequest {
			name = "cancel-after-durable-create"
		}
		t.Run(name, func(t *testing.T) {
			f := newSessionAdmissionFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			f.e.schedule.Lock()
			locked := true
			defer func() {
				if locked {
					f.e.schedule.Unlock()
				}
			}()
			type result struct {
				session domain.Session
				err     error
			}
			done := make(chan result, 1)
			go func() {
				s, err := f.e.StartVisibleUpdate(ctx, "offline schedule lock ordering")
				done <- result{s, err}
			}()
			deadline := time.Now().Add(3 * time.Second)
			for f.countSessions(t) == 0 {
				if time.Now().After(deadline) {
					t.Fatal("session not created while scheduler blocked")
				}
				time.Sleep(time.Millisecond)
			}
			for !f.e.operation.TryLock() {
				if time.Now().After(deadline) {
					t.Fatal("operation held while waiting for schedule")
				}
				time.Sleep(time.Millisecond)
			}
			f.e.captureMu.Lock()
			remembered := len(f.e.captureSessions)
			f.e.captureMu.Unlock()
			f.e.operation.Unlock()
			if remembered != 1 || f.manager.Snapshot().ActiveLeases != 1 {
				t.Fatal("operation released before remembering durable capture ownership")
			}
			select {
			case got := <-done:
				t.Fatalf("start skipped held schedule lock: %+v", got)
			default:
			}
			if cancelRequest {
				cancel()
			}
			f.e.schedule.Unlock()
			locked = false
			select {
			case got := <-done:
				if cancelRequest {
					if !errors.Is(got.err, context.Canceled) {
						t.Fatalf("request cancellation=%v", got.err)
					}
				} else if got.err != nil || got.session.ID == "" {
					t.Fatalf("admission=%+v", got)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("admission did not finish after scheduler unlock")
			}
			if f.countSessions(t) != 1 || f.manager.Snapshot().ActiveLeases != 1 {
				t.Fatal("post-create cancellation rolled back durable session/ownership")
			}
		})
	}
	t.Run("cancel-before-operation", func(t *testing.T) {
		f := newSessionAdmissionFixture(t)
		calls := f.cancellationProbe(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		f.e.operation.Lock()
		done := make(chan error, 1)
		go func() { _, err := f.e.StartVisibleUpdate(ctx, "offline cancelled request"); done <- err }()
		select {
		case err := <-done:
			f.e.operation.Unlock()
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("calibration context=%v", err)
			}
		case <-time.After(3 * time.Second):
			f.e.operation.Unlock()
			t.Fatal("visible policy read unexpectedly waited for operation")
		}
		if f.countSessions(t) != 0 || *calls != [2]int{} {
			t.Fatal("cancelled policy read admitted work")
		}
	})
}
