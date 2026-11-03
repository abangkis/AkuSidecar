package engine

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
)

type visionPersistenceProvider struct {
	reasoning.Deterministic
	planned, analyzed            int
	planningKeys, evaluationKeys []string
	verify                       func(context.Context, domain.Observation)
}

func visionPersistenceKeys(obs domain.Observation) []string {
	keys := []string{}
	for _, snapshot := range obs.Snapshots {
		for _, block := range snapshot.Blocks {
			keys = append(keys, block.EvidenceKey)
		}
	}
	return keys
}

func (p *visionPersistenceProvider) Plan(ctx context.Context, run domain.Run, obs domain.Observation, knowledge []domain.ReasonedItem) (reasoning.AcquisitionPlan, domain.ReasoningTelemetry, error) {
	p.planned++
	p.planningKeys = visionPersistenceKeys(obs)
	if p.verify != nil {
		p.verify(ctx, obs)
	}
	return p.Deterministic.Plan(ctx, run, obs, knowledge)
}

func (p *visionPersistenceProvider) Analyze(ctx context.Context, run domain.Run, obs domain.Observation, knowledge []domain.ReasonedItem) (domain.ReasoningResult, domain.ReasoningTelemetry, error) {
	p.analyzed++
	p.evaluationKeys = visionPersistenceKeys(obs)
	if p.verify != nil {
		p.verify(ctx, obs)
	}
	return p.Deterministic.Analyze(ctx, run, obs, knowledge)
}

func newVisionPersistenceFixture(t *testing.T) (bridgeCommandFixture, *visionPersistenceProvider) {
	t.Helper()
	f := newBridgeCommandFixture(t, domain.SourceInstagram)
	settings, err := f.state.GetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	settings.CalibrationEnabled, settings.AIDetectionEnabled = false, false
	settings.SemanticEventMode = "show_all"
	if err := f.state.SaveSettings(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
	p := &visionPersistenceProvider{}
	f.e.provider = p
	return f, p
}

func acceptVisionPersistenceObservation(t *testing.T, f bridgeCommandFixture, blocks []domain.Block) {
	t.Helper()
	command := f.claim(t)
	obs := domain.Observation{Source: domain.SourceInstagram, CapturedAt: domain.Now(), Snapshots: []domain.Snapshot{{Blocks: blocks}}, Coverage: map[string]any{"performedScrolls": float64(0)}}
	if err := validateObservation(obs); err != nil {
		t.Fatal(err)
	}
	if err := f.state.SaveObservation(context.Background(), command.ID, f.run.ID, obs); err != nil {
		t.Fatal(err)
	}
}

func visionPersistenceCount(t *testing.T, f bridgeCommandFixture, query string, args ...any) int {
	t.Helper()
	var value int
	if err := f.db.QueryRow(query, args...).Scan(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

func visionPersistencePriorRun(t *testing.T, f bridgeCommandFixture) domain.Run {
	t.Helper()
	// Historical rows belong to a separate session/run; all queue mutations use Store.
	f.exec(t, `INSERT INTO sessions(id,intent,status,max_items_per_source,max_items_total,created_at,coverage_json) SELECT 'vision-prior-session','prior vision fixture','completed',max_items_per_source,max_items_total,created_at,coverage_json FROM sessions WHERE id=?`, f.session.ID)
	f.exec(t, `INSERT INTO runs(id,session_id,source,ordinal,status,stage,created_at) VALUES('vision-prior-run','vision-prior-session','instagram',0,'completed','completed',?)`, domain.Now())
	return domain.Run{ID: "vision-prior-run", SessionID: "vision-prior-session", Source: domain.SourceInstagram}
}

func assertVisionPersistenceStopped(t *testing.T, f bridgeCommandFixture, p *visionPersistenceProvider, err error) {
	t.Helper()
	if err == nil || !strings.HasPrefix(err.Error(), "enqueue media-only vision candidates: enqueue vision evaluation: ") || !strings.Contains(err.Error(), "vision persistence fixture rejected") {
		t.Fatal("enqueue error contract changed", err)
	}
	run, loadErr := f.state.GetRun(context.Background(), f.run.ID)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if run.Status != "reasoning" || run.Coverage["visionEvaluation"] != nil || run.Coverage["acquisitionPlanning"] != nil || p.planned != 0 || p.analyzed != 0 {
		t.Fatal("failed handoff continued or wrote success receipt", run, p)
	}
	for _, table := range []string{"candidate_assessments", "timeline_items", "reasoning_invocations"} {
		if visionPersistenceCount(t, f, "SELECT COUNT(*) FROM "+table+" WHERE run_id=?", run.ID) != 0 {
			t.Fatal("failed handoff wrote " + table)
		}
	}
	if visionPersistenceCount(t, f, `SELECT COUNT(*) FROM bridge_commands WHERE run_id=?`, run.ID) != 1 {
		t.Fatal("failed handoff queued follow-up capture")
	}
	if visionPersistenceCount(t, f, `SELECT COUNT(*) FROM run_stage_timings WHERE run_id=? AND stage IN ('evaluated','selected','added')`, run.ID) != 0 {
		t.Fatal("failed handoff reached evaluation/completion")
	}
}

func TestEngineVisionLanePersistenceCharacterizationPartialEnqueue(t *testing.T) {
	for _, mode := range []string{"media-only", "mixed"} {
		t.Run(mode, func(t *testing.T) {
			f, p := newVisionPersistenceFixture(t)
			blocks := []domain.Block{visionLaneBlock("partial-first", ""), visionLaneBlock("partial-second", "")}
			if mode == "mixed" {
				blocks = append(blocks, visionLaneBlock("caption", "Independent text evidence must wait for successful vision receipt."))
			}
			acceptVisionPersistenceObservation(t, f, blocks)
			f.exec(t, `CREATE TRIGGER vision_partial_reject BEFORE INSERT ON vision_evaluation_jobs WHEN NEW.evidence_key='instagram:p:partial-second' BEGIN SELECT RAISE(ABORT,'vision persistence fixture rejected'); END`)
			err := f.e.process(context.Background(), f.run.ID, true)
			assertVisionPersistenceStopped(t, f, p, err)
			jobs, err := f.state.ListVisionEvaluations(context.Background(), f.run.ID, f.run.Source)
			if err != nil {
				t.Fatal(err)
			}
			if len(jobs) != 1 || jobs[0].EvidenceKey != "instagram:p:partial-first" || jobs[0].Status != "pending" || jobs[0].Reason != "media_only_bounded_lane" || jobs[0].AttemptCount != 0 || jobs[0].RunID != f.run.ID || jobs[0].SessionID != f.session.ID {
				t.Fatal("earlier enqueue was rolled back or reassigned", jobs)
			}
			if visionPersistenceCount(t, f, `SELECT COUNT(*) FROM vision_evaluation_jobs`) != 1 {
				t.Fatal("failed candidate was persisted")
			}
		})
	}
}

func TestEngineVisionLanePersistenceCharacterizationDeferredPromotionSurvivesFailure(t *testing.T) {
	for _, mode := range []string{"first-insert-fails", "second-insert-fails"} {
		t.Run(mode, func(t *testing.T) {
			f, p := newVisionPersistenceFixture(t)
			prior := visionPersistencePriorRun(t, f)
			inputs := []domain.VisionEvaluationInput{}
			for i := 0; i < 9; i++ {
				block := visionLaneBlock(fmt.Sprintf("peer-%d", i), "")
				inputs = append(inputs, domain.VisionEvaluationInput{EvidenceKey: block.EvidenceKey, Candidate: block})
			}
			jobs, err := f.state.EnqueueVisionEvaluations(context.Background(), prior, inputs, 8)
			if err != nil || len(jobs) != 9 {
				t.Fatal("prior queue", jobs, err)
			}
			var pending, deferred *domain.VisionEvaluationJob
			for i := range jobs {
				if jobs[i].Status == "pending" && pending == nil {
					pending = &jobs[i]
				}
				if jobs[i].Status == "deferred" {
					deferred = &jobs[i]
				}
			}
			if pending == nil || deferred == nil || visionPersistenceCount(t, f, `SELECT COUNT(*) FROM vision_evaluation_jobs WHERE status='pending'`) != 8 {
				t.Fatal("prior capacity fixture invalid", jobs)
			}
			// Expose one vacancy without invoking a Store method that promotes it first.
			f.exec(t, `UPDATE vision_evaluation_jobs SET status='ready',completed_at=? WHERE id=?`, domain.Now(), pending.ID)
			if visionPersistenceCount(t, f, `SELECT COUNT(*) FROM vision_evaluation_jobs WHERE status='pending'`) != 7 || visionPersistenceCount(t, f, `SELECT COUNT(*) FROM vision_evaluation_jobs WHERE id=? AND status='deferred'`, deferred.ID) != 1 {
				t.Fatal("vacancy/promotion precondition changed")
			}
			acceptVisionPersistenceObservation(t, f, []domain.Block{visionLaneBlock("promotion-first", ""), visionLaneBlock("promotion-second", "")})
			failKey := "instagram:p:promotion-first"
			if mode == "second-insert-fails" {
				failKey = "instagram:p:promotion-second"
			}
			f.exec(t, fmt.Sprintf(`CREATE TRIGGER vision_promotion_reject BEFORE INSERT ON vision_evaluation_jobs WHEN NEW.evidence_key='%s' BEGIN SELECT RAISE(ABORT,'vision persistence fixture rejected'); END`, failKey))
			err = f.e.process(context.Background(), f.run.ID, true)
			assertVisionPersistenceStopped(t, f, p, err)
			priorJobs, err := f.state.ListVisionEvaluations(context.Background(), prior.ID, prior.Source)
			if err != nil || len(priorJobs) != 9 {
				t.Fatal("prior jobs changed", priorJobs, err)
			}
			promoted := false
			for _, job := range priorJobs {
				if job.ID == deferred.ID {
					promoted = job.Status == "pending" && job.Reason == "capacity_available" && job.RunID == prior.ID && job.SessionID == prior.SessionID && job.AttemptCount == 0
				}
			}
			if !promoted || visionPersistenceCount(t, f, `SELECT COUNT(*) FROM vision_evaluation_jobs WHERE status='pending'`) != 8 {
				t.Fatal("peer promotion was rolled back", priorJobs)
			}
			current, err := f.state.ListVisionEvaluations(context.Background(), f.run.ID, f.run.Source)
			if err != nil {
				t.Fatal(err)
			}
			want := 0
			if mode == "second-insert-fails" {
				want = 1
			}
			if len(current) != want {
				t.Fatal("partial current enqueue changed", current)
			}
			if want == 1 && (current[0].Status != "deferred" || current[0].Reason != "capacity_full" || current[0].EvidenceKey != "instagram:p:promotion-first") {
				t.Fatal("source-wide promoted capacity was ignored", current)
			}
			if visionPersistenceCount(t, f, `SELECT COUNT(*) FROM vision_evaluation_jobs`) != 9+want {
				t.Fatal("failed insert persisted or prior jobs disappeared")
			}
		})
	}
}

func visionPersistenceJobSnapshot(t *testing.T, f bridgeCommandFixture, id string) string {
	t.Helper()
	var value string
	if err := f.db.QueryRow(`SELECT json_array(id,run_id,session_id,source,evidence_key,canonical_identity,media_fingerprint,status,reason,attempt_count,queued_at,next_attempt_at,last_error,hex(candidate_json),typeof(candidate_json),hex(result_json),typeof(result_json),created_at,started_at,completed_at) FROM vision_evaluation_jobs WHERE id=?`, id).Scan(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

func TestEngineVisionLanePersistenceCharacterizationCrossRunDeduplication(t *testing.T) {
	for _, mode := range []string{"media-only", "mixed"} {
		t.Run(mode, func(t *testing.T) {
			f, p := newVisionPersistenceFixture(t)
			prior := visionPersistencePriorRun(t, f)
			visual := visionLaneBlock("shared-media", "")
			jobs, err := f.state.EnqueueVisionEvaluations(context.Background(), prior, []domain.VisionEvaluationInput{{EvidenceKey: visual.EvidenceKey, Candidate: visual}}, 8)
			if err != nil || len(jobs) != 1 {
				t.Fatal("prior dedup fixture", jobs, err)
			}
			f.exec(t, `UPDATE vision_evaluation_jobs SET status='retry_wait',reason='transient_failure',attempt_count=2,next_attempt_at='2030-01-01T00:00:00Z',last_error='prior fixture failure',result_json='{"fixture":"retained"}' WHERE id=?`, jobs[0].ID)
			before := visionPersistenceJobSnapshot(t, f, jobs[0].ID)
			blocks := []domain.Block{visual}
			if mode == "mixed" {
				blocks = append(blocks, visionLaneBlock("caption", "Independent caption evidence can enter text reasoning after the receipt."))
			}
			acceptVisionPersistenceObservation(t, f, blocks)
			wantCoverage := map[string]any{"policy": "manual_review", "available": false, "availability": "Generated locally from trusted media metadata. No image is sent to an AI provider.", "capacity": float64(8), "active": float64(0), "pending": float64(0), "deferred": float64(0), "evaluating": float64(0), "retrying": float64(0), "ready": float64(0), "failed": float64(0)}
			p.verify = func(ctx context.Context, obs domain.Observation) {
				run, err := f.state.GetRun(ctx, f.run.ID)
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(run.Coverage["visionEvaluation"], wantCoverage) || visionLaneJSON(t, obs.Coverage["visionEvaluation"]) != visionLaneJSON(t, wantCoverage) {
					t.Fatal("text reasoning preceded run-scoped durable receipt", run.Coverage, obs.Coverage)
				}
			}
			if err := f.e.process(context.Background(), f.run.ID, true); err != nil {
				t.Fatal(err)
			}
			if visionPersistenceJobSnapshot(t, f, jobs[0].ID) != before || visionPersistenceCount(t, f, `SELECT COUNT(*) FROM vision_evaluation_jobs`) != 1 {
				t.Fatal("deduplication duplicated/reassigned/reset prior job")
			}
			current, err := f.state.ListVisionEvaluations(context.Background(), f.run.ID, f.run.Source)
			if err != nil || len(current) != 0 {
				t.Fatal("new run acquired the old job", current, err)
			}
			global, err := f.state.VisionEvaluationSummary(context.Background(), "", f.run.Source)
			if err != nil || global.Active != 1 || global.Retrying != 1 {
				t.Fatal("deduplication lost source-wide existing work", global, err)
			}
			run, err := f.state.GetRun(context.Background(), f.run.ID)
			if err != nil {
				t.Fatal(err)
			}
			if run.Status != "completed" || !reflect.DeepEqual(run.Coverage["visionEvaluation"], wantCoverage) {
				t.Fatal("run-scoped receipt/completion changed", run)
			}
			wantCalls := 0
			if mode == "mixed" {
				wantCalls = 1
			}
			if p.planned != wantCalls || p.analyzed != wantCalls {
				t.Fatal("deduplicated media entered text reasoning", p)
			}
			if mode == "mixed" && (!reflect.DeepEqual(p.planningKeys, []string{"instagram:p:caption"}) || !reflect.DeepEqual(p.evaluationKeys, []string{"instagram:p:caption"})) {
				t.Fatal("media/text isolation changed", p)
			}
			if mode == "media-only" && run.Summary != "1 Instagram media-only candidate held for bounded vision evaluation; no text-bearing candidate entered reasoning." {
				t.Fatal("summary stopped counting input candidates", run.Summary)
			}
			if visionPersistenceCount(t, f, `SELECT COUNT(*) FROM bridge_commands WHERE run_id=?`, f.run.ID) != 1 {
				t.Fatal("deduplication queued another capture")
			}
			if got := visionPersistenceCount(t, f, `SELECT COUNT(*) FROM timeline_items WHERE run_id=?`, f.run.ID); got != wantCalls {
				t.Fatal("unexpected Timeline delivery", got, wantCalls)
			}
		})
	}
}
