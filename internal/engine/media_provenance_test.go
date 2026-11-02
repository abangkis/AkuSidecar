package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/mediaprovenance"
)

type provenanceTestInspector struct {
	available bool
	version   string
	calls     atomic.Int32
	inspect   func(context.Context, domain.MediaProvenanceAssessment, []string) (mediaprovenance.Result, error)
}

func (*provenanceTestInspector) Name() string      { return "offline-provenance-inspector" }
func (i *provenanceTestInspector) Version() string { return i.version }
func (i *provenanceTestInspector) Available() bool { return i.available }
func (i *provenanceTestInspector) Inspect(ctx context.Context, a domain.MediaProvenanceAssessment, hosts []string) (mediaprovenance.Result, error) {
	i.calls.Add(1)
	if i.inspect != nil {
		return i.inspect(ctx, a, hosts)
	}
	return neutralProvenanceResult(), nil
}

func neutralProvenanceResult() mediaprovenance.Result {
	return mediaprovenance.Result{ManifestState: "no_manifest", TrustState: "not_applicable", AIOrigin: "unknown"}
}

type provenanceTestLog struct {
	mu   sync.Mutex
	text string
}

func (l *provenanceTestLog) Printf(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.text += fmt.Sprintf(format, args...) + "\n"
}
func (l *provenanceTestLog) String() string { l.mu.Lock(); defer l.mu.Unlock(); return l.text }

type provenanceEngineFixture struct {
	recaptureEngineFixture
	inspector *provenanceTestInspector
	log       *provenanceTestLog
}

// Reuse the disposable timeline fixture, replacing only its evidence and ports.
// All queue, resume and worker operations use the real Engine and Store.
func newProvenanceEngineFixture(t *testing.T) provenanceEngineFixture {
	t.Helper()
	f := newRecaptureEngineFixture(t, domain.SourceX)
	raw, err := json.Marshal(f.observation)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`UPDATE observations SET observation_json=? WHERE id='media-obs'`, string(raw)); err != nil {
		t.Fatal(err)
	}
	inspector := &provenanceTestInspector{available: true, version: "offline-provenance-v1"}
	log := &provenanceTestLog{}
	f.e.logger = log
	f.e.SetMediaProvenanceInspector(inspector)
	return provenanceEngineFixture{f, inspector, log}
}

func (f provenanceEngineFixture) item(index int) domain.TimelineItem {
	return domain.TimelineItem{ID: "media-item", SessionID: "media-session", Source: domain.SourceX, Evidence: &domain.Block{Media: []map[string]any{{"kind": "image", "url": fmt.Sprintf("https://pbs.twimg.com/media/offline-%d.jpg", index)}}}}
}

func (f provenanceEngineFixture) exec(t *testing.T, query string, args ...any) {
	t.Helper()
	if _, err := f.db.Exec(query, args...); err != nil {
		t.Fatal(err)
	}
}

func (f provenanceEngineFixture) wait(t *testing.T) {
	t.Helper()
	if !f.e.WaitForIdle(3 * time.Second) {
		t.Fatal("media provenance worker did not retire")
	}
	f.e.mu.RLock()
	_, active := f.e.work.active["media-provenance"]
	f.e.mu.RUnlock()
	if active {
		t.Fatal("retired worker remains registered")
	}
}

func (f provenanceEngineFixture) status(t *testing.T, version string) string {
	t.Helper()
	var status string
	if err := f.db.QueryRow(`SELECT status FROM media_provenance_assessments WHERE verifier_version=? ORDER BY created_at,id LIMIT 1`, version).Scan(&status); err != nil {
		t.Fatal(err)
	}
	return status
}

func TestEngineMediaProvenanceCharacterizationRuntime(t *testing.T) {
	for _, mode := range []string{"nil", "unavailable", "available"} {
		t.Run(mode, func(t *testing.T) {
			e := &Engine{}
			want := domain.MediaProvenanceRuntime{Provider: mediaprovenance.ProviderName, Version: mediaprovenance.VerifierVersion}
			if mode != "nil" {
				i := &provenanceTestInspector{version: "projection-v1", available: mode == "available"}
				e.SetMediaProvenanceInspector(i)
				want = domain.MediaProvenanceRuntime{Provider: i.Name(), Version: i.Version(), Available: i.Available()}
			}
			if got := e.MediaProvenanceRuntime(); !reflect.DeepEqual(got, want) {
				t.Fatalf("runtime=%+v want=%+v", got, want)
			}
			e.SetMediaProvenanceInspector(nil)
			if got := e.MediaProvenanceRuntime(); got.Provider != mediaprovenance.ProviderName || got.Available {
				t.Fatalf("cleared inspector runtime=%+v", got)
			}
		})
	}
}

func TestEngineMediaProvenanceCharacterizationUnavailable(t *testing.T) {
	for _, mode := range []string{"nil", "unavailable"} {
		t.Run(mode, func(t *testing.T) {
			f := newProvenanceEngineFixture(t)
			if _, err := f.state.QueueMediaProvenance(context.Background(), []domain.TimelineItem{f.item(0)}, f.inspector.Name(), f.inspector.Version()); err != nil {
				t.Fatal(err)
			}
			f.exec(t, `UPDATE media_provenance_assessments SET status='running'`)
			if mode == "nil" {
				f.e.SetMediaProvenanceInspector(nil)
			} else {
				f.inspector.available = false
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if err := f.e.ResumeMediaProvenance(ctx); err != nil {
				t.Fatalf("unavailable guard must precede cancelled context: %v", err)
			}
			f.e.launchMediaProvenance("missing-session")
			f.e.launchMediaProvenanceItems([]domain.TimelineItem{f.item(1)})
			f.wait(t)
			if f.status(t, f.inspector.version) != "running" || f.count(t, "provenance") != 1 || f.inspector.calls.Load() != 0 || f.log.String() != "" {
				t.Fatal("unavailable inspector touched durable work or launched inspection")
			}
		})
	}
}

func TestEngineMediaProvenanceCharacterizationResume(t *testing.T) {
	for _, mode := range []string{"recover-running", "cancelled-request", "store-error", "other-version"} {
		t.Run(mode, func(t *testing.T) {
			f := newProvenanceEngineFixture(t)
			version := f.inspector.version
			if mode == "other-version" {
				version = "older-verifier"
			}
			if _, err := f.state.QueueMediaProvenance(context.Background(), []domain.TimelineItem{f.item(0)}, f.inspector.Name(), version); err != nil {
				t.Fatal(err)
			}
			f.exec(t, `UPDATE media_provenance_assessments SET status='running',started_at='old-start',error='old-error'`)
			if mode == "store-error" {
				f.exec(t, `CREATE TRIGGER provenance_resume_abort BEFORE UPDATE OF status ON media_provenance_assessments WHEN NEW.status='queued' BEGIN SELECT RAISE(ABORT,'offline resume rejected'); END`)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "cancelled-request" {
				cancel()
			}
			seen := make(chan bool, 1)
			f.inspector.inspect = func(_ context.Context, a domain.MediaProvenanceAssessment, _ []string) (mediaprovenance.Result, error) {
				seen <- a.Status == "running" && a.StartedAt != "old-start" && a.Error == ""
				return neutralProvenanceResult(), nil
			}
			err := f.e.ResumeMediaProvenance(ctx)
			if mode == "cancelled-request" || mode == "store-error" {
				if mode == "cancelled-request" {
					if !errors.Is(err, context.Canceled) {
						t.Fatalf("resume error=%v", err)
					}
				} else if err == nil || !strings.Contains(err.Error(), "offline resume rejected") {
					t.Fatalf("resume error=%v", err)
				}
				f.wait(t)
				if f.status(t, version) != "running" || f.inspector.calls.Load() != 0 {
					t.Fatal("rejected resume launched work or changed queue")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			f.wait(t)
			if mode == "other-version" {
				if f.status(t, version) != "queued" || f.inspector.calls.Load() != 0 {
					t.Fatal("worker claimed another verifier version")
				}
				var started any
				var message string
				if err := f.db.QueryRow(`SELECT started_at,error FROM media_provenance_assessments`).Scan(&started, &message); err != nil || started != nil || message != "" {
					t.Fatalf("resume did not clear running metadata: %v %q %v", started, message, err)
				}
			} else {
				if f.status(t, version) != "completed" || f.inspector.calls.Load() != 1 {
					t.Fatal("recovered work did not finish")
				}
				select {
				case valid := <-seen:
					if !valid {
						t.Fatal("inspection preceded resume reset and durable claim")
					}
				default:
					t.Fatal("inspection missing")
				}
			}
		})
	}
}

func TestEngineMediaProvenanceCharacterizationWorker(t *testing.T) {
	for _, mode := range []string{"empty", "session-items", "version-and-hosts"} {
		t.Run(mode, func(t *testing.T) {
			f := newProvenanceEngineFixture(t)
			seen := make(chan bool, 1)
			f.inspector.inspect = func(ctx context.Context, a domain.MediaProvenanceAssessment, hosts []string) (mediaprovenance.Result, error) {
				descriptor, _ := domain.SourceByID(domain.SourceX)
				var status string
				err := f.db.QueryRowContext(ctx, `SELECT status FROM media_provenance_assessments WHERE id=?`, a.ID).Scan(&status)
				unlocked := f.e.mu.TryLock()
				if unlocked {
					f.e.mu.Unlock()
				}
				seen <- err == nil && status == "running" && a.Status == "running" && a.VerifierVersion == f.inspector.version && reflect.DeepEqual(hosts, descriptor.TrustedMediaHostSuffixes) && unlocked
				return neutralProvenanceResult(), nil
			}
			switch mode {
			case "empty":
				f.e.launchMediaProvenanceItems(nil)
			case "session-items":
				f.e.launchMediaProvenance("media-session")
			case "version-and-hosts":
				if _, err := f.state.QueueMediaProvenance(context.Background(), []domain.TimelineItem{f.item(0)}, f.inspector.Name(), "older-verifier"); err != nil {
					t.Fatal(err)
				}
				f.e.launchMediaProvenanceItems([]domain.TimelineItem{f.item(0)})
			}
			f.wait(t)
			if mode == "empty" {
				if f.inspector.calls.Load() != 0 || f.count(t, "provenance") != 0 {
					t.Fatal("empty queue inspected work")
				}
				return
			}
			if f.inspector.calls.Load() != 1 || f.status(t, f.inspector.version) != "completed" {
				t.Fatal("current work did not complete exactly once")
			}
			select {
			case valid := <-seen:
				if !valid {
					t.Fatal("claim, host contract or unlocked inspector boundary changed")
				}
			default:
				t.Fatal("inspection missing")
			}
			if mode == "version-and-hosts" && f.status(t, "older-verifier") != "queued" {
				t.Fatal("wrong verifier work was consumed")
			}
		})
	}
}

func TestEngineMediaProvenanceCharacterizationQueueBeforeActiveCheck(t *testing.T) {
	f := newProvenanceEngineFixture(t)
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	var concurrent, maxConcurrent atomic.Int32
	f.inspector.inspect = func(_ context.Context, _ domain.MediaProvenanceAssessment, _ []string) (mediaprovenance.Result, error) {
		active := concurrent.Add(1)
		defer concurrent.Add(-1)
		for old := maxConcurrent.Load(); active > old && !maxConcurrent.CompareAndSwap(old, active); old = maxConcurrent.Load() {
		}
		if f.inspector.calls.Load() == 1 {
			entered <- struct{}{}
			<-release
		}
		return neutralProvenanceResult(), nil
	}
	f.e.launchMediaProvenanceItems([]domain.TimelineItem{f.item(0)})
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("first inspector did not start")
	}
	f.e.launchMediaProvenanceItems([]domain.TimelineItem{f.item(1)})
	var queued, running int
	if err := f.db.QueryRow(`SELECT SUM(status='queued'),SUM(status='running') FROM media_provenance_assessments`).Scan(&queued, &running); err != nil {
		t.Fatal(err)
	}
	f.e.mu.RLock()
	active := len(f.e.work.active)
	f.e.mu.RUnlock()
	if queued != 1 || running != 1 || active != 1 || f.inspector.calls.Load() != 1 {
		t.Fatalf("queue-before-active check changed: queued=%d running=%d active=%d calls=%d", queued, running, active, f.inspector.calls.Load())
	}
	unblock()
	f.wait(t)
	var completed int
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM media_provenance_assessments WHERE status='completed'`).Scan(&completed); err != nil {
		t.Fatal(err)
	}
	if completed != 2 || f.inspector.calls.Load() != 2 || maxConcurrent.Load() != 1 {
		t.Fatalf("worker serialization changed: completed=%d calls=%d max=%d", completed, f.inspector.calls.Load(), maxConcurrent.Load())
	}
}

func TestEngineMediaProvenanceCharacterizationErrors(t *testing.T) {
	for _, mode := range []string{"queue", "claim", "inspect", "finish", "load-session"} {
		t.Run(mode, func(t *testing.T) {
			f := newProvenanceEngineFixture(t)
			wantLog, wantStatus, wantCalls := "", "", int32(0)
			switch mode {
			case "queue":
				f.inspector.version = ""
				wantLog = "queue degraded safely"
			case "claim":
				f.exec(t, `CREATE TRIGGER provenance_claim_abort BEFORE UPDATE OF status ON media_provenance_assessments WHEN NEW.status='running' BEGIN SELECT RAISE(ABORT,'offline claim rejected'); END`)
				wantLog, wantStatus = "claim degraded safely", "queued"
			case "inspect":
				f.inspector.inspect = func(context.Context, domain.MediaProvenanceAssessment, []string) (mediaprovenance.Result, error) {
					return mediaprovenance.Result{}, errors.New("offline inspector rejected")
				}
				wantStatus, wantCalls = "failed", 1
			case "finish":
				f.exec(t, `CREATE TRIGGER provenance_finish_abort BEFORE UPDATE OF status ON media_provenance_assessments WHEN NEW.status='completed' AND OLD.target_url LIKE '%offline-0.jpg' BEGIN SELECT RAISE(ABORT,'offline finish rejected'); END`)
				wantLog, wantStatus, wantCalls = "could not persist", "running", 2
			case "load-session":
				f.exec(t, `ALTER TABLE timeline_items RENAME TO offline_timeline_items`)
				wantLog = "could not load session"
			}
			if mode == "load-session" {
				f.e.launchMediaProvenance("media-session")
			} else {
				items := []domain.TimelineItem{f.item(0)}
				if mode == "finish" {
					items = append(items, f.item(1))
				}
				f.e.launchMediaProvenanceItems(items)
			}
			f.wait(t)
			if f.inspector.calls.Load() != wantCalls {
				t.Fatalf("inspections=%d want=%d", f.inspector.calls.Load(), wantCalls)
			}
			if wantLog != "" && !strings.Contains(f.log.String(), wantLog) {
				t.Fatalf("log=%q want=%q", f.log.String(), wantLog)
			}
			if wantStatus == "" {
				if f.count(t, "provenance") != 0 {
					t.Fatal("rejected load/queue created assessment")
				}
				return
			}
			if mode == "finish" {
				var first, second string
				if err := f.db.QueryRow(`SELECT status FROM media_provenance_assessments WHERE target_url LIKE '%offline-0.jpg'`).Scan(&first); err != nil {
					t.Fatal(err)
				}
				if err := f.db.QueryRow(`SELECT status FROM media_provenance_assessments WHERE target_url LIKE '%offline-1.jpg'`).Scan(&second); err != nil {
					t.Fatal(err)
				}
				if first != "running" || second != "completed" {
					t.Fatalf("finish error stopped worker or rewrote failed write: %s %s", first, second)
				}
			} else if f.status(t, f.inspector.version) != wantStatus {
				t.Fatal("failure changed durable status")
			}
			if mode == "inspect" {
				var manifest, trust, origin, message string
				if err := f.db.QueryRow(`SELECT manifest_state,trust_state,ai_origin,error FROM media_provenance_assessments`).Scan(&manifest, &trust, &origin, &message); err != nil {
					t.Fatal(err)
				}
				if manifest != "unavailable" || trust != "not_applicable" || origin != "unknown" || message != "offline inspector rejected" {
					t.Fatalf("inspector error result=%s %s %s %q", manifest, trust, origin, message)
				}
			}
		})
	}
}

func TestEngineMediaProvenanceCharacterizationShutdownContext(t *testing.T) {
	f := newProvenanceEngineFixture(t)
	if _, err := f.state.QueueMediaProvenance(context.Background(), []domain.TimelineItem{f.item(0)}, f.inspector.Name(), f.inspector.Version()); err != nil {
		t.Fatal(err)
	}
	entered := make(chan context.Context, 1)
	cancelled := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	f.inspector.inspect = func(ctx context.Context, _ domain.MediaProvenanceAssessment, _ []string) (mediaprovenance.Result, error) {
		entered <- ctx
		<-ctx.Done()
		close(cancelled)
		<-release
		return mediaprovenance.Result{}, ctx.Err()
	}
	request, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := f.e.ResumeMediaProvenance(request); err != nil {
		t.Fatal(err)
	}
	var worker context.Context
	select {
	case worker = <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("resume did not start inspector")
	}
	cancel()
	if worker.Err() != nil {
		t.Fatal("worker inherited request cancellation")
	}
	f.e.Shutdown()
	select {
	case <-cancelled:
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown did not cancel inspector")
	}
	if f.e.WaitForIdle(50 * time.Millisecond) {
		t.Fatal("worker retired before inspector/result completion")
	}
	unblock()
	f.wait(t)
	var status, message string
	if err := f.db.QueryRow(`SELECT status,error FROM media_provenance_assessments`).Scan(&status, &message); err != nil {
		t.Fatal(err)
	}
	if status != "failed" || message != context.Canceled.Error() {
		t.Fatalf("cancelled worker did not persist with background context: %s %q", status, message)
	}
}
