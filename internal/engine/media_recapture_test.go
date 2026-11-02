package engine

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/collection"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/mediaprovenance"
	"github.com/abangkis/AkuSidecar/internal/store"
)

type recaptureEngineFixture struct {
	e           *Engine
	state       *store.Store
	db          *sql.DB
	observation domain.Observation
}

// Only persisted evidence is seeded directly. Queue, ownership, claim and result
// operations use the real Engine/Store with fake process/inspector boundaries.
func newRecaptureEngineFixture(t *testing.T, source domain.Source) recaptureEngineFixture {
	t.Helper()
	e, state := testEngine(t)
	db, err := sql.Open("sqlite", state.Path())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	t.Cleanup(func() {
		e.Shutdown()
		if !e.WaitForIdle(3 * time.Second) {
			t.Error("recapture fixture workers did not retire")
		}
	})
	native := "https://x.com/fixture/status/1234567890123456789"
	if source == domain.SourceFacebook {
		native = "https://www.facebook.com/photo/?fbid=1234567890123456789"
	}
	if source == domain.SourceLinkedIn {
		native = "https://www.linkedin.com/feed/update/urn:li:activity:1234567890123456789"
	}
	key := string(source) + ":recapture-characterization"
	block := domain.Block{EvidenceKey: key, PlatformID: "1234567890123456789", Permalink: native, Author: "Fixture", Text: "Offline media recapture evidence", MediaRecovery: map[string]any{"outcome": "unavailable"}}
	observation := domain.Observation{Source: source, CapturedAt: domain.Now(), Snapshots: []domain.Snapshot{{Blocks: []domain.Block{block}}}, Coverage: map[string]any{"quality": "complete"}}
	raw, err := json.Marshal(observation)
	if err != nil {
		t.Fatal(err)
	}
	now := domain.Now()
	if _, err := db.Exec(`INSERT INTO observations(id,run_id,command_id,source,observation_json,captured_at,created_at) VALUES('media-obs','media-run','media-command',?,?,?,?)`, source, string(raw), now, now); err != nil {
		t.Fatal(err)
	}
	item, err := json.Marshal(domain.ReasonedItem{Source: source, SourceURL: native, EvidenceKey: key})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO timeline_items(id,session_id,run_id,source,evidence_key,rank,item_json,assessment_json,created_at) VALUES('media-item','media-session','media-run',?,?,0,?,'{}',?)`, source, key, string(item), now); err != nil {
		t.Fatal(err)
	}
	image := "https://pbs.twimg.com/media/fixture.jpg"
	if source == domain.SourceLinkedIn {
		image = "https://media.licdn.com/dms/image/fixture.jpg"
	}
	if source == domain.SourceFacebook {
		image = "https://media.fbcdn.net/fixture.jpg"
	}
	observation.Snapshots[0].Blocks[0].Media = []map[string]any{{"kind": "image", "url": image}}
	return recaptureEngineFixture{e, state, db, observation}
}

func (f recaptureEngineFixture) count(t *testing.T, table string) int {
	t.Helper()
	var query string
	switch table {
	case "jobs":
		query = `SELECT COUNT(*) FROM media_recaptures`
	case "provenance":
		query = `SELECT COUNT(*) FROM media_provenance_assessments`
	default:
		t.Fatal("unsupported fixture count")
	}
	var count int
	if err := f.db.QueryRow(query).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func (f recaptureEngineFixture) queue(t *testing.T) domain.MediaRecapture {
	t.Helper()
	job, err := f.e.QueueMediaRecapture(context.Background(), "media-item", domain.MediaRecaptureBackground)
	if err != nil {
		t.Fatal(err)
	}
	return job
}

func TestEngineMediaRecaptureCharacterizationQueueGuards(t *testing.T) {
	for _, guard := range []string{"development", "missing-item", "transition", "active-session", "readiness", "consent", "stopped-owner"} {
		t.Run(guard, func(t *testing.T) {
			f := newRecaptureEngineFixture(t, domain.SourceX)
			id, want := "media-item", ""
			var manager *captureruntime.Manager
			switch guard {
			case "development":
				f.e.developmentSources = map[domain.Source]bool{domain.SourceX: true}
				id, want = "missing", "development session media recapture unavailable"
			case "missing-item":
				id, want = "missing", "sql: no rows"
			case "transition", "active-session":
				settings, err := f.state.GetSettings(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				if _, err := f.state.CreateUpdateSession(context.Background(), "active guard fixture", settings, domain.UpdatePolicy{Trigger: domain.UpdateTriggerUser, Delivery: domain.UpdateDeliveryVisible, BudgetAuthority: domain.BudgetAuthorityUser}); err != nil {
					t.Fatal(err)
				}
				f.e.ResetCaptureHeartbeat()
				want = "finish the active update before recapturing media"
				if guard == "transition" {
					manager = attachTestCapture(t, f.e)
					coordinator := collection.NewCoordinator(manager, nil, func() error { return nil })
					coordinator.Request("headless")
					f.e.AttachCollectionCoordinator(coordinator)
					want = "wait for the collection mode transition before recapturing media"
				}
			case "readiness":
				f.e.ResetCaptureHeartbeat()
				want = "AkuBridge v2 is not ready:"
			case "consent":
				manager = attachTestCapture(t, f.e)
				if err := manager.Replace(context.Background(), func(context.Context, uint64) (captureruntime.Process, error) {
					return &headlessTestProcess{&runtimeTestProcess{done: make(chan error, 1)}}, nil
				}); err != nil {
					t.Fatal(err)
				}
				coordinator := collection.NewCoordinator(manager, nil, func() error { return nil })
				coordinator.Request("headless")
				f.e.AttachCollectionCoordinator(coordinator)
				f.e.mu.Lock()
				f.e.headlessAccess = nil
				f.e.mu.Unlock()
				want = "headless recapture source is unsupported or access has not been granted"
			case "stopped-owner":
				manager = attachTestCapture(t, f.e)
				manager.Terminate()
				want = "capture runtime is stopped"
			}
			leasesBefore := 0
			if manager != nil {
				leasesBefore = manager.Snapshot().ActiveLeases
			}
			_, err := f.e.QueueMediaRecapture(context.Background(), id, domain.MediaRecaptureBackground)
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("guard=%s err=%v want=%s", guard, err, want)
			}
			if f.count(t, "jobs") != 0 {
				t.Fatal("queue rejection persisted a job")
			}
			if manager != nil && manager.Snapshot().ActiveLeases != leasesBefore {
				t.Fatal("queue rejection changed existing lease ownership")
			}
		})
	}
}

func TestEngineMediaRecaptureCharacterizationQueueOwnership(t *testing.T) {
	for _, mode := range []string{"unmanaged", "bridge", "quiet", "headless", "store-rejection", "foreground"} {
		t.Run(mode, func(t *testing.T) {
			f := newRecaptureEngineFixture(t, domain.SourceX)
			var manager *captureruntime.Manager
			if mode != "unmanaged" {
				manager = attachTestCapture(t, f.e)
				if mode == "headless" {
					if err := manager.Replace(context.Background(), func(context.Context, uint64) (captureruntime.Process, error) {
						return &headlessTestProcess{&runtimeTestProcess{done: make(chan error, 1)}}, nil
					}); err != nil {
						t.Fatal(err)
					}
				}
				coordinator := collection.NewCoordinator(manager, nil, func() error { return nil })
				coordinator.Request(manager.Snapshot().Driver)
				if mode == "quiet" || mode == "foreground" {
					coordinator.SetBrowserCollector(manager.Snapshot().Generation, &readinessQuietBackend{ready: true})
				}
				f.e.AttachCollectionCoordinator(coordinator)
			}
			if mode == "foreground" {
				prior, err := f.state.CreateMediaRecapture(context.Background(), "media-item", domain.MediaRecaptureBackground)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := f.state.ClaimMediaRecapture(context.Background(), prior.ID, "fixture"); err != nil {
					t.Fatal(err)
				}
				noMedia := f.observation
				noMedia.Snapshots = []domain.Snapshot{{Blocks: []domain.Block{{EvidenceKey: f.observation.Snapshots[0].Blocks[0].EvidenceKey, PlatformID: "1234567890123456789", Permalink: f.observation.Snapshots[0].Blocks[0].Permalink, Text: "Unavailable", MediaRecovery: map[string]any{"outcome": "unavailable"}}}}}
				if _, err := f.state.CompleteMediaRecapture(context.Background(), prior.ID, noMedia); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "store-rejection" {
				_, err := f.e.QueueMediaRecaptureForReason(context.Background(), "media-item", domain.MediaRecaptureBackground, domain.MediaRecapturePlaybackError)
				if err == nil || !strings.Contains(err.Error(), "recapturable inline playback URL") {
					t.Fatalf("Store error changed: %v", err)
				}
				if manager.Snapshot().ActiveLeases != 0 || f.count(t, "jobs") != 0 {
					t.Fatal("Store rejection leaked a job/lease")
				}
				return
			}
			captureMode := domain.MediaRecaptureBackground
			if mode == "foreground" {
				captureMode = domain.MediaRecaptureForeground
			}
			job, err := f.e.QueueMediaRecapture(context.Background(), "media-item", captureMode)
			if err != nil {
				t.Fatal(err)
			}
			if job.Status != "queued" || job.Payload["reason"] != domain.MediaRecaptureMissingMedia {
				t.Fatalf("default queue payload: %+v", job)
			}
			want := collection.BackendBridge
			if mode == "quiet" {
				want = collection.BackendQuiet
			}
			if mode == "headless" {
				want = collection.BackendHeadless
			}
			if got, err := store.CaptureCollector(job.Payload); err != nil || got != want {
				t.Fatalf("collector=%s want=%s err=%v", got, want, err)
			}
			if manager == nil {
				if job.Payload["captureRuntime"] != nil {
					t.Fatal("unmanaged job gained an owner stamp")
				}
			} else {
				stamp, ok := job.Payload["captureRuntime"].(map[string]any)
				if !ok || stamp["epoch"] != f.e.Epoch() || stamp["driver"] != manager.Snapshot().Driver || !captureGenerationMatches(stamp["generation"], manager.Snapshot().Generation) || manager.Snapshot().ActiveLeases != 1 {
					t.Fatalf("owned queue=%+v snapshot=%+v", job, manager.Snapshot())
				}
				if _, err := f.e.QueueMediaRecapture(context.Background(), "media-item", captureMode); err == nil {
					t.Fatal("duplicate queue accepted")
				}
				if manager.Snapshot().ActiveLeases != 1 {
					t.Fatal("duplicate queue leaked its candidate lease")
				}
			}
			if _, err := f.e.FailMediaRecapture(context.Background(), job.ID, domain.Failure{Message: "fixture cleanup"}); err != nil {
				t.Fatal(err)
			}
			if manager != nil {
				waitCaptureLeases(t, manager, 0)
			}
		})
	}
}

type recaptureTestInspector struct {
	version string
	inspect func(context.Context, domain.MediaProvenanceAssessment, []string) (mediaprovenance.Result, error)
}

func (*recaptureTestInspector) Name() string      { return "offline-recapture-inspector" }
func (i *recaptureTestInspector) Version() string { return i.version }
func (*recaptureTestInspector) Available() bool   { return true }
func (i *recaptureTestInspector) Inspect(ctx context.Context, a domain.MediaProvenanceAssessment, hosts []string) (mediaprovenance.Result, error) {
	return i.inspect(ctx, a, hosts)
}

func TestEngineMediaRecaptureCharacterizationProvenance(t *testing.T) {
	for _, mode := range []string{"success", "inspector-error", "queue-error", "rejected-observation"} {
		t.Run(mode, func(t *testing.T) {
			f := newRecaptureEngineFixture(t, domain.SourceX)
			job := f.queue(t)
			if _, err := f.e.ClaimMediaRecapture(context.Background(), job.ID, "fixture"); err != nil {
				t.Fatal(err)
			}
			var calls atomic.Int32
			observed := make(chan bool, 1)
			inspector := &recaptureTestInspector{version: "offline-recapture-v1"}
			inspector.inspect = func(ctx context.Context, a domain.MediaProvenanceAssessment, _ []string) (mediaprovenance.Result, error) {
				calls.Add(1)
				stored, err := f.state.MediaRecapture(ctx, job.ID)
				var count int
				queryErr := f.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM timeline_evidence_overrides WHERE timeline_id=?`, job.TimelineID).Scan(&count)
				observed <- err == nil && queryErr == nil && stored.Status == "completed" && count == 1 && a.TimelineID == job.TimelineID
				if mode == "inspector-error" {
					return mediaprovenance.Result{}, errors.New("offline inspector failure")
				}
				return mediaprovenance.Result{ManifestState: "no_manifest", TrustState: "not_applicable", AIOrigin: "unknown"}, nil
			}
			if mode == "queue-error" {
				inspector.version = ""
			}
			f.e.SetMediaProvenanceInspector(inspector)
			if mode == "rejected-observation" {
				f.observation.Snapshots = nil
			}
			result, err := f.e.AcceptMediaRecapture(context.Background(), job.ID, f.observation)
			if mode == "rejected-observation" {
				if err == nil || f.count(t, "provenance") != 0 || calls.Load() != 0 {
					t.Fatalf("rejected observation launched provenance: %+v %v", result, err)
				}
				return
			}
			if err != nil || result.Status != "completed" || result.Outcome != "recovered" {
				t.Fatalf("provenance replaced primary result: %+v %v", result, err)
			}
			if !f.e.WaitForIdle(3 * time.Second) {
				t.Fatal("inspector worker did not retire")
			}
			if mode == "queue-error" {
				if calls.Load() != 0 || f.count(t, "provenance") != 0 {
					t.Fatal("invalid provenance version launched inspection")
				}
			} else {
				select {
				case durable := <-observed:
					if !durable {
						t.Fatal("inspection preceded durable recapture/evidence")
					}
				default:
					t.Fatal("provenance inspector was not called")
				}
				var status string
				if err := f.db.QueryRow(`SELECT status FROM media_provenance_assessments`).Scan(&status); err != nil {
					t.Fatal(err)
				}
				want := "completed"
				if mode == "inspector-error" {
					want = "failed"
				}
				if status != want {
					t.Fatalf("assessment status=%s want=%s", status, want)
				}
			}
			stored, err := f.e.MediaRecapture(context.Background(), job.ID)
			if err != nil || stored.Status != "completed" {
				t.Fatalf("primary durable result changed: %+v %v", stored, err)
			}
		})
	}
}

func TestEngineMediaRecaptureCharacterizationFailureCancellation(t *testing.T) {
	for _, mode := range []string{"x-challenge", "ordinary-failure", "other-source", "store-error"} {
		t.Run(mode, func(t *testing.T) {
			source := domain.SourceX
			if mode == "other-source" {
				source = domain.SourceLinkedIn
			}
			f := newRecaptureEngineFixture(t, source)
			job := f.queue(t)
			if mode == "store-error" {
				if _, err := f.state.FailMediaRecapture(context.Background(), job.ID, domain.Failure{Code: "prior"}); err != nil {
					t.Fatal(err)
				}
			}
			calls, durable := 0, false
			f.e.captureBehaviorMu.Lock()
			f.e.captureWork = map[string]context.CancelFunc{"fixture": func() {
				calls++
				stored, err := f.state.MediaRecapture(context.Background(), job.ID)
				durable = err == nil && stored.Status == "failed" && stored.Error != nil && domain.IsAccessChallenge(stored.Error.Code)
			}}
			f.e.captureBehaviorMu.Unlock()
			code := "challenge_required"
			if mode == "ordinary-failure" {
				code = "fixture_failure"
			}
			_, err := f.e.FailMediaRecapture(context.Background(), job.ID, domain.Failure{Code: code})
			if mode == "store-error" {
				if err == nil {
					t.Fatal("terminal replay accepted")
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if mode == "x-challenge" {
				if calls != 1 || !durable {
					t.Fatalf("cancellation preceded durable challenge: calls=%d durable=%t", calls, durable)
				}
			} else if calls != 0 {
				t.Fatalf("unrelated failure canceled capture: %d", calls)
			}
			f.e.captureBehaviorMu.Lock()
			delete(f.e.captureWork, "fixture")
			f.e.captureBehaviorMu.Unlock()
		})
	}
}

func TestEngineMediaRecaptureCharacterizationCleanup(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		name := "terminal-result"
		if canceled {
			name = "canceled-request"
		}
		t.Run(name, func(t *testing.T) {
			f := newRecaptureEngineFixture(t, domain.SourceFacebook)
			manager := attachTestCapture(t, f.e)
			job, err := f.state.CreateOwnedMediaRecaptureWithAdmission(context.Background(), "media-item", domain.MediaRecaptureBackground, domain.MediaRecaptureMissingMedia, collection.BackendBridge, domain.MediaRecaptureCaptureAdmission{Policy: domain.MediaRecaptureAdmissionHybridHeadlessV1, Driver: "browser", Phase: "waiting"})
			if err != nil {
				t.Fatal(err)
			}
			stamp := map[string]any{"driver": "browser", "epoch": f.e.Epoch(), "generation": int(manager.Snapshot().Generation)}
			if ok, err := f.state.BindMediaRecaptureBrowserAdmission(context.Background(), job.ID, stamp); err != nil || !ok {
				t.Fatalf("bind admission=%t %v", ok, err)
			}
			job, err = f.state.MediaRecapture(context.Background(), job.ID)
			if err != nil {
				t.Fatal(err)
			}
			if err := f.e.ensureAdmittedBrowserRecapture(job); err != nil {
				t.Fatal(err)
			}
			started := make(chan bool, 1)
			ack := make(chan struct{})
			var once sync.Once
			defer once.Do(func() { close(ack) })
			f.e.SetBrowserCollectionCleanup(func(ctx context.Context, id string, generation uint64) error {
				unlocked := f.e.captureBehaviorMu.TryLock()
				if unlocked {
					f.e.captureBehaviorMu.Unlock()
				}
				started <- ctx.Err() == nil && id == job.ID && generation == manager.Snapshot().Generation && unlocked
				<-ack
				return nil
			})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if canceled {
				if _, err := f.state.FailMediaRecapture(context.Background(), job.ID, domain.Failure{Code: "already_terminal"}); err != nil {
					t.Fatal(err)
				}
				cancel()
			}
			_, err = f.e.FailMediaRecapture(ctx, job.ID, domain.Failure{Code: "fixture_failure"})
			if canceled {
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("request cancellation lost: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			select {
			case valid := <-started:
				if !valid {
					t.Fatal("cleanup context, identity or unlock order changed")
				}
			case <-time.After(2 * time.Second):
				t.Fatal("terminal cleanup did not start")
			}
			if manager.Snapshot().ActiveLeases != 1 {
				t.Fatal("lease released before cleanup acknowledgement")
			}
			stored, err := f.state.MediaRecapture(context.Background(), job.ID)
			if err != nil || stored.Payload["captureCleanup"] != "pending" {
				t.Fatalf("cleanup pending state=%+v %v", stored, err)
			}
			once.Do(func() { close(ack) })
			waitCaptureLeases(t, manager, 0)
			stored, err = f.state.MediaRecapture(context.Background(), job.ID)
			if err != nil || stored.Payload["captureCleanup"] != "released" {
				t.Fatalf("cleanup acknowledgement=%+v %v", stored, err)
			}
		})
	}
}

func TestEngineMediaRecaptureCharacterizationContext(t *testing.T) {
	for _, operation := range []string{"queue", "claim", "status", "observation", "failure"} {
		t.Run(operation, func(t *testing.T) {
			f := newRecaptureEngineFixture(t, domain.SourceX)
			job := f.queue(t)
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			var err error
			switch operation {
			case "queue":
				_, err = f.e.QueueMediaRecapture(ctx, "media-item", domain.MediaRecaptureBackground)
			case "claim":
				_, err = f.e.ClaimMediaRecapture(ctx, job.ID, "fixture")
			case "status":
				_, err = f.e.MediaRecapture(ctx, job.ID)
			case "observation":
				_, err = f.e.AcceptMediaRecapture(ctx, job.ID, f.observation)
			case "failure":
				_, err = f.e.FailMediaRecapture(ctx, job.ID, domain.Failure{})
			}
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("request context lost: %v", err)
			}
			stored, err := f.state.MediaRecapture(context.Background(), job.ID)
			if err != nil || stored.Status != "queued" || f.count(t, "jobs") != 1 || f.count(t, "provenance") != 0 {
				t.Fatalf("canceled request changed state: %+v %v", stored, err)
			}
		})
	}
}
