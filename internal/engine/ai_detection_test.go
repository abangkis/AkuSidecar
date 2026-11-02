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

	"github.com/abangkis/AkuSidecar/internal/aidetector"
	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/mediaprovenance"
)

type aiTestResolver struct {
	models  atomic.Int32
	calls   atomic.Int32
	resolve func(context.Context, []domain.TimelineItem) (domain.DeepAIResult, domain.ModelUsage, time.Duration, error)
	events  *provenanceTestLog
}

func (*aiTestResolver) Name() string { return "offline-ai-resolver" }
func (r *aiTestResolver) Model() config.ModelConfig {
	r.models.Add(1)
	if r.events != nil {
		r.events.Printf("model")
	}
	return config.ModelConfig{Model: "base-model", Effort: "low"}
}
func (r *aiTestResolver) Resolve(ctx context.Context, items []domain.TimelineItem) (domain.DeepAIResult, domain.ModelUsage, time.Duration, error) {
	r.calls.Add(1)
	if r.events != nil {
		r.events.Printf("resolve")
	}
	if r.resolve != nil {
		return r.resolve(ctx, items)
	}
	return aiTestResult(len(items)), domain.ModelUsage{}, 5 * time.Millisecond, nil
}

type aiTestProfiledResolver struct {
	*aiTestResolver
	profiles chan string
}

func (r *aiTestProfiledResolver) ModelForProfile(profile string) config.ModelConfig {
	if r.events != nil {
		r.events.Printf("profile-model:%s", profile)
	}
	return config.ModelConfig{Model: "profile-model", Effort: "high"}
}
func (r *aiTestProfiledResolver) ResolveWithProfile(ctx context.Context, items []domain.TimelineItem, profile string) (domain.DeepAIResult, domain.ModelUsage, time.Duration, error) {
	r.profiles <- profile
	return r.Resolve(ctx, items)
}
func aiTestResult(count int) domain.DeepAIResult {
	values := make([]domain.DeepAIAssessment, count)
	for i := range values {
		values[i] = domain.DeepAIAssessment{Status: "no_signal_detected", ConfidenceBand: "medium", EvidenceCodes: []string{}, AssessedObject: "social_post", SignalScope: "social_post", Rationale: "Offline bounded review"}
	}
	return domain.DeepAIResult{Assessments: values}
}

type aiEngineFixture struct {
	provenanceEngineFixture
	sessionID string
	item      domain.TimelineItem
	resolver  *aiTestResolver
}

func newAIEngineFixture(t *testing.T) aiEngineFixture {
	t.Helper()
	f := newProvenanceEngineFixture(t)
	f.e.SetMediaProvenanceInspector(nil)
	settings, err := f.state.GetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	session, err := f.state.CreateUpdateSession(context.Background(), "offline AI characterization", settings, domain.UpdatePolicy{Trigger: domain.UpdateTriggerUser, Delivery: domain.UpdateDeliveryVisible, BudgetAuthority: domain.BudgetAuthorityUser})
	if err != nil {
		t.Fatal(err)
	}
	f.exec(t, `UPDATE timeline_items SET session_id=? WHERE id='media-item'`, session.ID)
	f.observation.Snapshots[0].Blocks[0].Text = "This post was written by ChatGPT."
	raw, err := json.Marshal(f.observation)
	if err != nil {
		t.Fatal(err)
	}
	f.exec(t, `UPDATE observations SET observation_json=? WHERE id='media-obs'`, string(raw))
	item := domain.TimelineItem{ID: "media-item", SessionID: session.ID, Source: domain.SourceX, Evidence: &f.observation.Snapshots[0].Blocks[0], AIDetection: &domain.TimelineAIDetection{AssessmentID: "ai-fast-before", Status: "strong_signals", EvidenceCodes: []string{"author_declared_ai"}}}
	fast := (aidetector.FastDetector{}).Detect([]domain.TimelineItem{item})
	fast[0].ID = "ai-fast-before"
	fast[0].Status = "strong_signals"
	fast[0].SignalScope = "social_post"
	fast[0].EvidenceCodes = []string{"author_declared_ai"}
	if err := f.state.SaveAIAssessments(context.Background(), fast); err != nil {
		t.Fatal(err)
	}
	r := &aiTestResolver{}
	f.e.SetAIDeepResolver(r)
	return aiEngineFixture{f, session.ID, item, r}
}
func (f aiEngineFixture) job(t *testing.T) *domain.AIDetectionJob {
	t.Helper()
	job, err := f.state.AIDetectionJob(context.Background(), f.sessionID)
	if err != nil {
		t.Fatal(err)
	}
	return job
}
func (f aiEngineFixture) countAI(t *testing.T, deep bool) int {
	t.Helper()
	query := `SELECT COUNT(*) FROM ai_detection_jobs`
	if deep {
		query = `SELECT COUNT(*) FROM ai_assessments WHERE stage='deep'`
	}
	var count int
	if err := f.db.QueryRow(query).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}
func (f aiEngineFixture) launch() {
	f.e.launchDeepDetectionItems(f.sessionID, []domain.TimelineItem{f.item})
}

func TestEngineAIDetectionCharacterizationFast(t *testing.T) {
	for _, mode := range []string{"success", "cancelled-request", "load-error", "save-error"} {
		t.Run(mode, func(t *testing.T) {
			f := newAIEngineFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "cancelled-request" {
				cancel()
			}
			if mode == "load-error" {
				f.exec(t, `ALTER TABLE timeline_items RENAME TO offline_timeline_items`)
			}
			if mode == "save-error" {
				f.exec(t, `CREATE TRIGGER ai_fast_abort BEFORE INSERT ON ai_assessments BEGIN SELECT RAISE(ABORT,'offline fast save rejected'); END`)
			}
			err := f.e.runFastDetection(ctx, f.sessionID)
			if mode == "success" {
				if err != nil {
					t.Fatal(err)
				}
				var count int
				if err := f.db.QueryRow(`SELECT COUNT(*) FROM ai_assessments WHERE stage='fast' AND provider='local-deterministic'`).Scan(&count); err != nil || count != 2 {
					t.Fatalf("fast persistence=%d %v", count, err)
				}
			} else if mode == "cancelled-request" {
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("context=%v", err)
				}
			} else if err == nil {
				t.Fatal("Store error lost")
			}
			if f.countAI(t, false) != 0 || f.resolver.calls.Load() != 0 {
				t.Fatal("fast path launched deep work")
			}
		})
	}
}

func TestEngineAIDetectionCharacterizationAdmission(t *testing.T) {
	for _, mode := range []string{"nil-resolver", "empty-shortlist", "disabled", "settings-error", "queue-error"} {
		t.Run(mode, func(t *testing.T) {
			f := newAIEngineFixture(t)
			keys := []string{"ai:" + f.sessionID + ":old", "ai:other:old", "media-provenance"}
			calls := [3]int{}
			f.e.mu.Lock()
			for i, key := range keys {
				index := i
				f.e.work.registerActive(key, func() { calls[index]++ })
			}
			f.e.mu.Unlock()
			t.Cleanup(func() {
				f.e.mu.Lock()
				for _, key := range keys {
					f.e.work.retireActive(key)
				}
				f.e.mu.Unlock()
			})
			switch mode {
			case "nil-resolver":
				f.e.SetAIDeepResolver(nil)
			case "empty-shortlist":
				f.item.AIDetection = nil
			case "disabled":
				settings, err := f.state.GetSettings(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				settings.AIDetectionEnabled = false
				if err := f.state.SaveSettings(context.Background(), settings); err != nil {
					t.Fatal(err)
				}
			case "settings-error":
				f.exec(t, `ALTER TABLE settings RENAME TO offline_settings`)
			case "queue-error":
				f.exec(t, `CREATE TRIGGER ai_queue_abort BEFORE INSERT ON ai_detection_jobs BEGIN SELECT RAISE(ABORT,'offline AI queue rejected'); END`)
			}
			f.launch()
			want := 1
			if mode == "nil-resolver" || mode == "empty-shortlist" {
				want = 0
			}
			if calls[0] != want || calls[1] != 0 || calls[2] != 0 || f.countAI(t, false) != 0 || f.resolver.calls.Load() != 0 {
				t.Fatalf("admission/cancellation order changed: calls=%v jobs=%d", calls, f.countAI(t, false))
			}
		})
	}
}

func TestEngineAIDetectionCharacterizationSessionLoading(t *testing.T) {
	for _, mode := range []string{"session-items", "empty-session", "load-before-resolver-guard"} {
		t.Run(mode, func(t *testing.T) {
			f := newAIEngineFixture(t)
			sessionID := f.sessionID
			if mode == "empty-session" {
				sessionID = "missing-session"
			}
			if mode == "load-before-resolver-guard" {
				f.e.SetAIDeepResolver(nil)
				f.exec(t, `ALTER TABLE timeline_items RENAME TO offline_timeline_items`)
			}
			f.e.launchDeepDetection(sessionID)
			f.wait(t)
			if mode == "session-items" {
				if job := f.job(t); job == nil || job.Status != "completed" || f.resolver.calls.Load() != 1 {
					t.Fatalf("session wrapper did not pass persisted candidates: %+v", job)
				}
			} else if f.countAI(t, false) != 0 || f.resolver.calls.Load() != 0 {
				t.Fatal("empty/error session launched work")
			}
			if mode == "load-before-resolver-guard" && !strings.Contains(f.log.String(), "could not load session") {
				t.Fatal("resolver guard bypassed the session read/error contract")
			}
		})
	}
}

func TestEngineAIDetectionCharacterizationProfileAndResult(t *testing.T) {
	for _, profiled := range []bool{false, true} {
		name := "plain"
		if profiled {
			name = "profiled"
		}
		t.Run(name, func(t *testing.T) {
			f := newAIEngineFixture(t)
			settings, err := f.state.GetSettings(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			events := &provenanceTestLog{}
			f.resolver.events = events
			p := &aiTestProfiledResolver{aiTestResolver: f.resolver, profiles: make(chan string, 1)}
			if profiled {
				f.e.SetAIDeepResolver(p)
			}
			seen := make(chan bool, 1)
			input, output := int64(23), int64(7)
			f.resolver.resolve = func(ctx context.Context, items []domain.TimelineItem) (domain.DeepAIResult, domain.ModelUsage, time.Duration, error) {
				job, err := f.state.AIDetectionJob(ctx, f.sessionID)
				wantModel, wantEffort := "base-model", "low"
				if profiled {
					wantModel, wantEffort = "profile-model", "high"
				}
				unlocked := f.e.mu.TryLock()
				if unlocked {
					f.e.mu.Unlock()
				}
				seen <- err == nil && job != nil && job.Status == "running" && job.Model == wantModel && job.Effort == wantEffort && job.CandidateCount == 1 && len(items) == 1 && items[0].ID == f.item.ID && unlocked
				return aiTestResult(len(items)), domain.ModelUsage{Input: &input, Output: &output, ProviderModel: "reported-model", NativeReasoning: "medium", CallerLatencyMS: 17, QueueWaitMS: 2, ProviderExecutionMS: 11, ResponseTotalMS: 15}, 5 * time.Millisecond, nil
			}
			// A completed job is observable only after its deep assessment exists.
			f.exec(t, `CREATE TRIGGER ai_completion_order BEFORE UPDATE OF status ON ai_detection_jobs WHEN NEW.status='completed' AND NOT EXISTS(SELECT 1 FROM ai_assessments WHERE session_id=NEW.session_id AND stage='deep') BEGIN SELECT RAISE(ABORT,'assessment must precede completion'); END`)
			f.e.launchDeepDetectionItems(f.sessionID, []domain.TimelineItem{{ID: "not-a-candidate"}, f.item})
			f.wait(t)
			select {
			case valid := <-seen:
				if !valid {
					t.Fatal("job/selection/model/lock contract changed")
				}
			default:
				t.Fatal("resolver missing")
			}
			if f.resolver.models.Load() != 1 || f.resolver.calls.Load() != 1 {
				t.Fatal("resolver dispatch changed")
			}
			if profiled {
				select {
				case actual := <-p.profiles:
					if actual != settings.ReasoningAIDeepProfile {
						t.Fatalf("profile=%s", actual)
					}
				default:
					t.Fatal("profiled path unused")
				}
			}
			wantEvents := "model\nresolve\n"
			if profiled {
				wantEvents = "model\nprofile-model:" + settings.ReasoningAIDeepProfile + "\nresolve\n"
			}
			if events.String() != wantEvents {
				t.Fatalf("model/profile order=%q", events.String())
			}
			job := f.job(t)
			if job == nil || job.Status != "completed" || job.DurationMS != 5 || job.Model != "reported-model" || job.Effort != "medium" || job.CallerLatencyMS != 17 || job.QueueWaitMS != 2 || job.ProviderExecutionMS != 11 || job.ResponseTotalMS != 15 || job.InputTokens == nil || *job.InputTokens != input || job.OutputTokens == nil || *job.OutputTokens != output {
				t.Fatalf("job metadata=%+v", job)
			}
			var prior, provider, version, scope, object, rationale string
			if err := f.db.QueryRow(`SELECT supersedes_id,provider,detector_version,signal_scope,assessed_object,rationale FROM ai_assessments WHERE stage='deep'`).Scan(&prior, &provider, &version, &scope, &object, &rationale); err != nil {
				t.Fatal(err)
			}
			if prior != "ai-fast-before" || provider != f.resolver.Name() || version != aidetector.DeepDetectorVersion || scope != "social_post" || object != "social_post" || rationale != "Offline bounded review" {
				t.Fatal("assessment projection/supersession changed")
			}
		})
	}
}

func TestEngineAIDetectionCharacterizationErrors(t *testing.T) {
	for _, mode := range []string{"start", "resolve", "resolve-cancelled", "usage-limit", "assessment-save", "finish"} {
		t.Run(mode, func(t *testing.T) {
			f := newAIEngineFixture(t)
			want, wantCalls := "failed", int32(1)
			switch mode {
			case "start":
				f.exec(t, `CREATE TRIGGER ai_start_abort BEFORE UPDATE OF status ON ai_detection_jobs WHEN NEW.status='running' BEGIN SELECT RAISE(ABORT,'offline AI start rejected'); END`)
				wantCalls = 0
			case "assessment-save":
				f.exec(t, `CREATE TRIGGER ai_assessment_abort BEFORE INSERT ON ai_assessments WHEN NEW.stage='deep' BEGIN SELECT RAISE(ABORT,'offline AI assessment rejected'); END`)
			case "finish":
				f.exec(t, `CREATE TRIGGER ai_finish_abort BEFORE UPDATE OF status ON ai_detection_jobs WHEN NEW.status='completed' BEGIN SELECT RAISE(ABORT,'offline AI finish rejected'); END`)
				want = "running"
			default:
				failure := errors.New("offline AI resolve rejected")
				if mode == "resolve-cancelled" {
					failure = context.Canceled
					want = "cancelled"
				}
				if mode == "usage-limit" {
					failure = errors.New("You've hit your usage limit. Try again later.")
					f.exec(t, `CREATE TRIGGER ai_usage_pause_order BEFORE INSERT ON meta WHEN NEW.key='auto_update_usage_limit_pause' AND NOT EXISTS(SELECT 1 FROM ai_detection_jobs WHERE status='failed') BEGIN SELECT RAISE(ABORT,'job failure must precede usage pause'); END`)
					select {
					case <-f.e.scheduler.wake:
					default:
					}
				}
				f.resolver.resolve = func(context.Context, []domain.TimelineItem) (domain.DeepAIResult, domain.ModelUsage, time.Duration, error) {
					return domain.DeepAIResult{}, domain.ModelUsage{}, 9 * time.Millisecond, failure
				}
			}
			f.launch()
			f.wait(t)
			job := f.job(t)
			if job == nil || job.Status != want || f.resolver.calls.Load() != wantCalls {
				t.Fatalf("job=%+v calls=%d", job, f.resolver.calls.Load())
			}
			wantDeep := 0
			if mode == "finish" {
				wantDeep = 1
			}
			if f.countAI(t, true) != wantDeep {
				t.Fatal("assessment/error ordering changed")
			}
			pause, err := f.state.AutoUpdateUsageLimitPause(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if mode == "usage-limit" {
				if pause == nil || pause.SessionID != f.sessionID {
					t.Fatalf("usage limit pause=%+v", pause)
				}
				select {
				case <-f.e.scheduler.wake:
				default:
					t.Fatal("usage limit did not wake scheduler")
				}
			} else if pause != nil {
				t.Fatal("unrelated error paused Auto Update")
			}
			if mode == "resolve-cancelled" && strings.Contains(f.log.String(), "degraded safely") {
				t.Fatal("cancellation logged as ordinary failure")
			}
		})
	}
}

func TestEngineAIDetectionCharacterizationCancellation(t *testing.T) {
	for _, success := range []bool{false, true} {
		name := "cancelled-error"
		if success {
			name = "success-after-cancellation"
		}
		t.Run(name, func(t *testing.T) {
			f := newAIEngineFixture(t)
			entered, cancelled := make(chan struct{}), make(chan struct{})
			release := make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			t.Cleanup(unblock)
			f.resolver.resolve = func(ctx context.Context, items []domain.TimelineItem) (domain.DeepAIResult, domain.ModelUsage, time.Duration, error) {
				close(entered)
				<-ctx.Done()
				close(cancelled)
				<-release
				if success {
					return aiTestResult(len(items)), domain.ModelUsage{}, time.Millisecond, nil
				}
				return domain.DeepAIResult{}, domain.ModelUsage{}, time.Millisecond, errors.New("provider interrupted")
			}
			f.launch()
			select {
			case <-entered:
			case <-time.After(3 * time.Second):
				t.Fatal("resolver did not start")
			}
			f.e.cancelDeepDetections()
			select {
			case <-cancelled:
			case <-time.After(3 * time.Second):
				t.Fatal("resolver was not cancelled")
			}
			if f.e.WaitForIdle(50 * time.Millisecond) {
				t.Fatal("cancelled worker retired before result handling")
			}
			unblock()
			f.wait(t)
			want, wantDeep := "cancelled", 0
			if success {
				want, wantDeep = "completed", 1
			}
			if job := f.job(t); job == nil || job.Status != want || f.countAI(t, true) != wantDeep {
				t.Fatalf("cancelled result contract changed: %+v", job)
			}
		})
	}
}

func TestEngineAIDetectionCharacterizationProvenanceCancellation(t *testing.T) {
	f := newAIEngineFixture(t)
	entered, cancelled := make(chan struct{}), make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	f.inspector.inspect = func(ctx context.Context, _ domain.MediaProvenanceAssessment, _ []string) (mediaprovenance.Result, error) {
		close(entered)
		<-ctx.Done()
		close(cancelled)
		<-release
		return mediaprovenance.Result{}, ctx.Err()
	}
	f.e.SetMediaProvenanceInspector(f.inspector)
	f.e.launchMediaProvenanceItems([]domain.TimelineItem{f.provenanceEngineFixture.item(0)})
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("provenance worker did not start")
	}
	keys := []string{"ai:first:job", "ai:second:job", "unrelated", "media-provenance-extra"}
	calls := [4]int{}
	held := true
	f.e.mu.Lock()
	for i, key := range keys {
		index := i
		f.e.work.registerActive(key, func() {
			calls[index]++
			locked := !f.e.mu.TryLock()
			if !locked {
				f.e.mu.Unlock()
			}
			held = held && locked
		})
	}
	f.e.mu.Unlock()
	t.Cleanup(func() {
		f.e.mu.Lock()
		for _, key := range keys {
			f.e.work.retireActive(key)
		}
		f.e.mu.Unlock()
	})
	f.e.cancelDeepDetections()
	if calls != [4]int{1, 1, 0, 0} || !held {
		t.Fatalf("cancellation scope/lock changed: %v held=%t", calls, held)
	}
	f.e.mu.Lock()
	for _, key := range keys {
		f.e.work.retireActive(key)
	}
	f.e.mu.Unlock()
	select {
	case <-cancelled:
	case <-time.After(3 * time.Second):
		t.Fatal("AI cancellation omitted media provenance")
	}
	if f.e.WaitForIdle(50 * time.Millisecond) {
		t.Fatal("provenance worker unregistered before durable result")
	}
	unblock()
	f.wait(t)
	if f.status(t, f.inspector.version) != "failed" {
		t.Fatal("provenance cancellation result did not persist")
	}
}

func TestEngineAIDetectionCharacterizationSettingsCancellation(t *testing.T) {
	for _, reject := range []bool{false, true} {
		name := "saved-disable"
		if reject {
			name = "rejected-save"
		}
		t.Run(name, func(t *testing.T) {
			// Settings/retention uses a clean Store, without synthetic orphan
			// timeline rows needed by the isolated assessment fixtures above.
			e, state := testEngine(t)
			t.Cleanup(func() {
				e.Shutdown()
				if !e.WaitForIdle(time.Second) {
					t.Error("settings fixture did not retire")
				}
			})
			db, err := sql.Open("sqlite", state.Path())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			settings, err := e.Settings(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			settings.AIDetectionEnabled = false
			if reject {
				if _, err := db.Exec(`CREATE TRIGGER ai_settings_abort BEFORE UPDATE ON settings BEGIN SELECT RAISE(ABORT,'offline settings rejected'); END`); err != nil {
					t.Fatal(err)
				}
			}
			calls, durable := 0, false
			e.mu.Lock()
			e.work.registerActive("media-provenance", func() {
				calls++
				saved, err := state.GetSettings(context.Background())
				durable = err == nil && !saved.AIDetectionEnabled
			})
			e.mu.Unlock()
			t.Cleanup(func() { e.mu.Lock(); e.work.retireActive("media-provenance"); e.mu.Unlock() })
			_, err = e.SaveSettings(context.Background(), settings)
			if reject {
				if err == nil || calls != 0 {
					t.Fatalf("rejected save cancelled work: calls=%d err=%v", calls, err)
				}
			} else if err != nil || calls != 1 || !durable {
				t.Fatalf("disable cancellation preceded persistence: calls=%d durable=%t err=%v", calls, durable, err)
			}
		})
	}
}
