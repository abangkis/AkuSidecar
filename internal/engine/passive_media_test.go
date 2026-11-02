package engine

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/mediaprovenance"
)

type passiveMediaFixture struct {
	recaptureEngineFixture
	input        domain.PassiveXMediaEvidence
	cleanupCalls int
	cancelCalls  int
}

func newPassiveMediaFixture(t *testing.T) *passiveMediaFixture {
	t.Helper()
	f := &passiveMediaFixture{recaptureEngineFixture: newRecaptureEngineFixture(t, domain.SourceX)}
	f.input = domain.PassiveXMediaEvidence{CandidateID: "x:status:1234567890123456789", Provenance: "passive_x_cache", Media: []domain.PassiveXMediaCandidate{{Kind: "image", URL: "https://pbs.twimg.com/media/passive-fixture.jpg#ignored", Provenance: "x_response_graphql"}}}
	if _, err := f.db.Exec(`INSERT INTO sessions(id,intent,status,max_items_per_source,max_items_total,created_at) VALUES('passive-cleanup','offline cleanup','completed',1,1,?)`, domain.Now()); err != nil {
		t.Fatal(err)
	}
	f.e.collectionIntents = map[string]func(){"passive-cleanup": func() {
		f.cleanupCalls++
		if !f.e.operation.TryLock() {
			t.Error("terminal cleanup ran before operation unlock")
		} else {
			f.e.operation.Unlock()
		}
	}}
	// Exercise the real queue without starting an inspector or changing worker ownership.
	for _, key := range []string{"media-provenance", "ai:passive-fixture", "unrelated"} {
		f.e.work.active[key] = func() { f.cancelCalls++ }
	}
	t.Cleanup(func() {
		f.e.mu.Lock()
		for _, key := range []string{"media-provenance", "ai:passive-fixture", "unrelated"} {
			delete(f.e.work.active, key)
		}
		f.e.mu.Unlock()
	})
	return f
}

func (f *passiveMediaFixture) exec(t *testing.T, query string) {
	t.Helper()
	if _, err := f.db.Exec(query); err != nil {
		t.Fatal(err)
	}
}

func (f *passiveMediaFixture) assertFinalState(t *testing.T, jobs, provenance int) {
	t.Helper()
	if f.cleanupCalls != 1 {
		t.Errorf("terminal cleanup calls=%d, want 1", f.cleanupCalls)
	}
	if f.count(t, "jobs") != jobs || f.count(t, "provenance") != provenance {
		t.Error("unexpected durable job counts")
	}
	if f.cancelCalls != 0 || len(f.e.work.active) != 3 {
		t.Error("passive enrichment changed cancellation/worker ownership")
	}
	if !f.e.operation.TryLock() {
		t.Error("operation lock retained")
	} else {
		f.e.operation.Unlock()
	}
	if !f.e.mu.TryLock() {
		t.Error("mu retained")
	} else {
		f.e.mu.Unlock()
	}
}

type passiveMediaInspector struct{ available func() bool }

func (*passiveMediaInspector) Name() string      { return "offline-passive-inspector" }
func (*passiveMediaInspector) Version() string   { return "offline-passive-v1" }
func (p *passiveMediaInspector) Available() bool { return p.available() }
func (*passiveMediaInspector) Inspect(context.Context, domain.MediaProvenanceAssessment, []string) (mediaprovenance.Result, error) {
	panic("passive characterization must not start inspection")
}

func TestEnginePassiveMediaCharacterizationDevelopmentGate(t *testing.T) {
	for _, sources := range []map[domain.Source]bool{{}, {domain.SourceX: true}} {
		// Nil Store/capture dependencies prove rejection precedes any durable work or cleanup.
		e := &Engine{developmentSources: sources}
		job, updated, err := e.ApplyPassiveXMediaEvidence(context.Background(), "missing", "bridge", domain.PassiveXMediaEvidence{})
		if err == nil || err.Error() != "development session passive media recapture unavailable" || updated || !reflect.DeepEqual(job, domain.MediaRecapture{}) {
			t.Fatalf("development rejection changed: %+v %t %v", job, updated, err)
		}
		if !e.operation.TryLock() {
			t.Fatal("development rejection retained operation")
		}
		e.operation.Unlock()
	}
}

func TestEnginePassiveMediaCharacterizationSuccessAndHandoff(t *testing.T) {
	for _, mode := range []string{"nil-inspector", "unavailable", "queued-active-worker", "queue-error", "timeline-error", "cancel-after-read"} {
		t.Run(mode, func(t *testing.T) {
			f := newPassiveMediaFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			log := &provenanceTestLog{}
			f.e.logger = log
			availableCalls := 0
			if mode != "nil-inspector" {
				f.e.mediaOrigin = &passiveMediaInspector{available: func() bool {
					availableCalls++
					if f.e.operation.TryLock() {
						f.e.operation.Unlock()
						t.Error("provenance handoff lost operation lock")
					}
					if f.count(t, "jobs") != 1 {
						t.Error("provenance handoff preceded Store commit")
					}
					if f.cleanupCalls != 0 {
						t.Error("cleanup preceded provenance handoff")
					}
					if mode == "cancel-after-read" {
						cancel()
					}
					return mode != "unavailable"
				}}
			}
			if mode == "queue-error" {
				f.exec(t, `CREATE TRIGGER fail_passive_queue BEFORE INSERT ON media_provenance_assessments BEGIN SELECT RAISE(ABORT,'passive-queue-failed'); END`)
			}
			if mode == "timeline-error" {
				// SQLite accepts text in this INTEGER-affinity column; the later Go scan fails.
				f.exec(t, `CREATE TRIGGER fail_passive_timeline AFTER INSERT ON timeline_evidence_overrides BEGIN UPDATE timeline_items SET rank='passive-read-fault' WHERE id=NEW.timeline_id; END`)
			}
			job, updated, err := f.e.ApplyPassiveXMediaEvidence(ctx, "media-item", "  passive-bridge  ", f.input)
			if err != nil || !updated || job.ID == "" || job.Status != "completed" || job.Outcome != "recovered" {
				t.Fatalf("Store success changed: %+v %t %v", job, updated, err)
			}
			if job.Payload["browserOperation"] != "none" || job.Payload["foregroundAuthorized"] != false {
				t.Fatal("passive capture authority changed", job.Payload)
			}
			var claimedBy string
			if err := f.db.QueryRow(`SELECT claimed_by FROM media_recaptures WHERE id=?`, job.ID).Scan(&claimedBy); err != nil || claimedBy != "passive-bridge" {
				t.Fatal("bridge attribution changed", claimedBy, err)
			}
			wantProvenance, wantAvailable := 0, 1
			if mode == "queued-active-worker" || mode == "cancel-after-read" {
				wantProvenance = 1
			}
			if mode == "nil-inspector" || mode == "timeline-error" {
				wantAvailable = 0
			}
			if availableCalls != wantAvailable {
				t.Fatalf("inspector availability calls=%d want=%d", availableCalls, wantAvailable)
			}
			if mode == "queue-error" && !strings.Contains(log.String(), "passive-queue-failed") {
				t.Fatal("post-commit queue error not logged")
			}
			if mode == "timeline-error" {
				if _, readErr := f.state.TimelineItem(context.Background(), "media-item"); readErr == nil {
					t.Fatal("Timeline read fault did not take effect")
				}
			}
			if mode == "cancel-after-read" && !errors.Is(ctx.Err(), context.Canceled) {
				t.Fatal("request cancellation probe did not run")
			}
			f.assertFinalState(t, 1, wantProvenance)
		})
	}
}

func TestEnginePassiveMediaCharacterizationNoOp(t *testing.T) {
	f := newPassiveMediaFixture(t)
	if _, updated, err := f.state.ApplyPassiveXMediaEvidence(context.Background(), "media-item", "seed", f.input); err != nil || !updated {
		t.Fatal("seed passive evidence", updated, err)
	}
	f.e.mediaOrigin = &passiveMediaInspector{available: func() bool { t.Error("no-op reached provenance handoff"); return false }}
	job, updated, err := f.e.ApplyPassiveXMediaEvidence(context.Background(), "media-item", "repeat", f.input)
	if err != nil || updated || !reflect.DeepEqual(job, domain.MediaRecapture{}) {
		t.Fatalf("no-op result changed: %+v %t %v", job, updated, err)
	}
	f.assertFinalState(t, 1, 0)
}

func TestEnginePassiveMediaCharacterizationFailures(t *testing.T) {
	for _, mode := range []string{"provenance", "candidate", "identity", "untrusted", "missing", "cancelled", "transaction"} {
		t.Run(mode, func(t *testing.T) {
			f := newPassiveMediaFixture(t)
			f.e.mediaOrigin = &passiveMediaInspector{available: func() bool { t.Error("failed Store reached provenance handoff"); return false }}
			input, id, ctx := f.input, "media-item", context.Background()
			want := ""
			switch mode {
			case "provenance":
				input.Provenance = "untrusted"
				want = "passive media provenance must be passive_x_cache"
			case "candidate":
				input.CandidateID = "invalid"
				want = "candidateId must use x:status:<5-30 digits>"
			case "identity":
				input.CandidateID = "x:status:9876543210987654321"
				want = "candidateId does not match the timeline item's X platform identity"
			case "untrusted":
				input.Media = []domain.PassiveXMediaCandidate{{Kind: "image", URL: "https://untrusted.example/image.jpg", Provenance: "x_response_graphql"}}
			case "missing":
				id = "missing-item"
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "transaction":
				f.exec(t, `CREATE TRIGGER fail_passive_write BEFORE INSERT ON timeline_evidence_overrides BEGIN SELECT RAISE(ABORT,'passive-write-failed'); END`)
				want = "passive-write-failed"
			}
			job, updated, err := f.e.ApplyPassiveXMediaEvidence(ctx, id, "bridge", input)
			if err == nil || updated || !reflect.DeepEqual(job, domain.MediaRecapture{}) {
				t.Fatalf("Store error/zero result changed: %+v %t %v", job, updated, err)
			}
			if want != "" && !strings.Contains(err.Error(), want) {
				t.Fatalf("original error changed: got=%v want=%q", err, want)
			}
			if mode == "cancelled" && !errors.Is(err, context.Canceled) {
				t.Fatal("cancellation identity lost", err)
			}
			f.assertFinalState(t, 0, 0)
		})
	}
}

type passiveMediaContext struct {
	context.Context
	once  sync.Once
	probe func()
}

func (c *passiveMediaContext) Done() <-chan struct{} { c.once.Do(c.probe); return c.Context.Done() }

func TestEnginePassiveMediaCharacterizationCallerContextAndOperation(t *testing.T) {
	for _, cancelLookup := range []bool{false, true} {
		name := "success"
		if cancelLookup {
			name = "cancelled-lookup"
		}
		t.Run(name, func(t *testing.T) {
			f := newPassiveMediaFixture(t)
			base, cancel := context.WithCancel(context.Background())
			defer cancel()
			observed := false
			ctx := &passiveMediaContext{Context: base, probe: func() {
				observed = true
				if f.e.operation.TryLock() {
					f.e.operation.Unlock()
					t.Error("Store apply lost operation lock")
				}
				if !f.e.mu.TryLock() {
					t.Error("Store apply unexpectedly holds mu")
				} else {
					f.e.mu.Unlock()
				}
				if cancelLookup {
					cancel()
				}
			}}
			job, updated, err := f.e.ApplyPassiveXMediaEvidence(ctx, "media-item", "bridge", f.input)
			if !observed {
				t.Fatal("caller context not forwarded to Store")
			}
			if cancelLookup {
				if !errors.Is(err, context.Canceled) || updated || !reflect.DeepEqual(job, domain.MediaRecapture{}) {
					t.Fatalf("lookup cancellation changed: %+v %t %v", job, updated, err)
				}
				f.assertFinalState(t, 0, 0)
			} else {
				if err != nil || !updated {
					t.Fatal("successful context probe", updated, err)
				}
				f.assertFinalState(t, 1, 0)
			}
		})
	}
}
