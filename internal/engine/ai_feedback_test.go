package engine

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
)

type aiFeedbackTestLog struct {
	provenanceTestLog
	observe func(string)
}

func (l *aiFeedbackTestLog) Printf(format string, args ...any) {
	message := fmt.Sprintf(format, args...)
	if l.observe != nil {
		l.observe(message)
	}
	l.provenanceTestLog.Printf("%s", message)
}

type aiFeedbackFixture struct {
	aiEngineFixture
	feedbackLog *aiFeedbackTestLog
}

func newAIFeedbackFixture(t *testing.T) aiFeedbackFixture {
	t.Helper()
	f := newAIEngineFixture(t)
	settings, err := f.state.GetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	settings.AIDetectionEnabled = true
	if err := f.state.SaveSettings(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
	log := &aiFeedbackTestLog{}
	f.e.logger = log
	return aiFeedbackFixture{f, log}
}

func (f aiFeedbackFixture) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var count int
	if err := f.db.QueryRow(query, args...).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func aiFeedbackOperationHeld(t *testing.T, e *Engine) {
	t.Helper()
	if e.operation.TryLock() {
		e.operation.Unlock()
		t.Error("feedback operation lock released before handoff")
	}
}

func aiFeedbackLocksReleased(t *testing.T, e *Engine) {
	t.Helper()
	if !e.operation.TryLock() {
		t.Fatal("feedback operation lock retained after return")
	}
	e.operation.Unlock()
	if !e.mu.TryLock() {
		t.Fatal("feedback mu retained after return")
	}
	e.mu.Unlock()
}

type aiFeedbackLockContext struct {
	context.Context
	probe func()
}

func (c aiFeedbackLockContext) Done() <-chan struct{} {
	c.probe()
	return c.Context.Done()
}

func TestEngineAIFeedbackCharacterizationMutationLock(t *testing.T) {
	for _, action := range []string{"add", "undo"} {
		t.Run(action, func(t *testing.T) {
			f := newAIFeedbackFixture(t)
			f.e.SetAIDeepResolver(nil)
			input := domain.AIFeedbackInput{Verdict: "ai", TargetType: "post", SignalScope: "social_post"}
			var prior domain.AIFeedbackEvent
			if action == "undo" {
				var err error
				prior, err = f.e.AddAIFeedback(context.Background(), f.item.ID, input)
				if err != nil {
					t.Fatal(err)
				}
			}
			var once sync.Once
			observed := false
			ctx := aiFeedbackLockContext{Context: context.Background(), probe: func() {
				// Probe the first database context access, before the durable mutation.
				once.Do(func() { observed = true; aiFeedbackOperationHeld(t, f.e) })
			}}
			var err error
			if action == "add" {
				_, err = f.e.AddAIFeedback(ctx, f.item.ID, input)
			} else {
				_, err = f.e.UndoAIFeedback(ctx, prior.ID)
			}
			if err != nil || !observed {
				t.Fatalf("mutation probe observed=%t err=%v", observed, err)
			}
			aiFeedbackLocksReleased(t, f.e)
		})
	}
}

type aiFeedbackResolver struct {
	*aiTestResolver
	onModel func()
}

func (r *aiFeedbackResolver) Model() config.ModelConfig { r.onModel(); return r.aiTestResolver.Model() }

type aiFeedbackResolveCall struct {
	ctx   context.Context
	items []domain.TimelineItem
}

func feedbackControlledResolver(t *testing.T, f aiFeedbackFixture, onModel func()) (*aiFeedbackResolver, <-chan aiFeedbackResolveCall, func()) {
	t.Helper()
	entered := make(chan aiFeedbackResolveCall, 1)
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	r := &aiFeedbackResolver{aiTestResolver: &aiTestResolver{}, onModel: onModel}
	r.resolve = func(ctx context.Context, items []domain.TimelineItem) (domain.DeepAIResult, domain.ModelUsage, time.Duration, error) {
		entered <- aiFeedbackResolveCall{ctx, items}
		<-release
		return aiTestResult(len(items)), domain.ModelUsage{}, time.Millisecond, nil
	}
	f.e.SetAIDeepResolver(r)
	return r, entered, unblock
}

func TestEngineAIFeedbackCharacterizationAdd(t *testing.T) {
	for _, target := range []struct{ kind, scope string }{{"post", "social_post"}, {"media", "attached_media"}, {"quote", "quoted_post"}, {"account", "author_account"}} {
		for _, verdict := range []string{"ai", "not_ai", "unsure"} {
			for _, enabled := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/enabled=%t", target.kind, verdict, enabled), func(t *testing.T) {
					f := newAIFeedbackFixture(t)
					ctx := context.Background()
					settings, err := f.state.GetSettings(ctx)
					if err != nil {
						t.Fatal(err)
					}
					settings.AIDetectionEnabled = enabled
					if err := f.state.SaveSettings(ctx, settings); err != nil {
						t.Fatal(err)
					}
					r, entered, unblock := feedbackControlledResolver(t, f, func() {
						aiFeedbackOperationHeld(t, f.e)
						if f.count(t, `SELECT COUNT(*) FROM ai_feedback_events`) != 1 || f.count(t, `SELECT COUNT(*) FROM ai_detection_jobs`) != 0 {
							t.Error("Deep handoff preceded feedback persistence")
						}
					})
					input := domain.AIFeedbackInput{TargetType: target.kind, SignalScope: target.scope, Verdict: verdict, Reason: "insufficient_evidence"}
					feedback, err := f.e.AddAIFeedback(ctx, f.item.ID, input)
					if err != nil || feedback.ID == "" || feedback.TimelineID != f.item.ID || feedback.SessionID != f.sessionID || feedback.TargetType != input.TargetType || feedback.SignalScope != input.SignalScope || feedback.Verdict != input.Verdict || feedback.Reason != input.Reason || feedback.SupersedesID != "" {
						t.Fatalf("feedback=%+v err=%v", feedback, err)
					}
					aiFeedbackLocksReleased(t, f.e)
					wantCalls := 0
					if enabled && verdict == "unsure" {
						wantCalls = 1
					}
					if int(r.models.Load()) != wantCalls || f.countAI(t, false) != wantCalls || f.count(t, `SELECT COUNT(*) FROM ai_feedback_events WHERE id=?`, feedback.ID) != 1 {
						t.Fatalf("handoff models=%d jobs=%d", r.models.Load(), f.countAI(t, false))
					}
					if wantCalls == 1 {
						select {
						case call := <-entered:
							if len(call.items) != 1 || call.items[0].ID != f.item.ID || call.items[0].AIDetection == nil || call.items[0].AIDetection.PersonalPolicy == nil || !call.items[0].AIDetection.PersonalPolicy.ReviewRequested {
								t.Fatalf("review input=%+v", call.items)
							}
						case <-time.After(3 * time.Second):
							t.Fatal("bounded review did not start")
						}
						unblock()
						if !f.e.WaitForIdle(3 * time.Second) {
							t.Fatal("review worker did not retire")
						}
					}
					history, err := f.e.AIFeedbackHistory(ctx, f.item.ID)
					if err != nil || !reflect.DeepEqual(history, []domain.AIFeedbackEvent{feedback}) {
						t.Fatalf("history=%+v err=%v", history, err)
					}
				})
			}
		}
	}
}

func TestEngineAIFeedbackCharacterizationErrors(t *testing.T) {
	for _, mode := range []string{"verdict", "target", "scope", "reason", "missing", "cancelled", "write", "settings", "item", "nil-resolver", "queue", "ai-bypasses-settings", "disabled-bypasses-item"} {
		t.Run(mode, func(t *testing.T) {
			f := newAIFeedbackFixture(t)
			ctx := context.Background()
			input := domain.AIFeedbackInput{Verdict: "unsure", TargetType: "post", SignalScope: "social_post"}
			id, want := f.item.ID, ""
			switch mode {
			case "verdict":
				input.Verdict, want = "clear", `unsupported AI feedback verdict "clear"`
			case "target":
				input.TargetType, want = "unknown", `unsupported AI feedback targetType "unknown"`
			case "scope":
				input.SignalScope, want = "attached_media", `AI feedback targetType "post" requires signalScope "social_post"`
			case "reason":
				input.Reason, want = "unknown", `unsupported AI feedback reason "unknown"`
			case "missing":
				id = "missing"
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "write":
				f.exec(t, `CREATE TRIGGER feedback_write_abort BEFORE INSERT ON ai_feedback_events BEGIN SELECT RAISE(ABORT,'offline feedback rejected'); END`)
				want = "offline feedback rejected"
			case "settings", "ai-bypasses-settings":
				f.exec(t, `CREATE TRIGGER feedback_bad_settings AFTER INSERT ON ai_feedback_events BEGIN UPDATE settings SET value_json='{' WHERE key='runtime'; END`)
				if mode == "ai-bypasses-settings" {
					input.Verdict = "ai"
				}
			case "item", "disabled-bypasses-item":
				if mode == "disabled-bypasses-item" {
					settings, err := f.state.GetSettings(ctx)
					if err != nil {
						t.Fatal(err)
					}
					settings.AIDetectionEnabled = false
					if err := f.state.SaveSettings(ctx, settings); err != nil {
						t.Fatal(err)
					}
				}
				f.exec(t, `CREATE TRIGGER feedback_delete_item AFTER INSERT ON ai_feedback_events BEGIN DELETE FROM timeline_items WHERE id=NEW.timeline_id; END`)
			case "nil-resolver":
				f.e.SetAIDeepResolver(nil)
			case "queue":
				f.exec(t, `CREATE TRIGGER feedback_queue_abort BEFORE INSERT ON ai_detection_jobs BEGIN SELECT RAISE(ABORT,'offline review queue rejected'); END`)
			}
			f.feedbackLog.observe = func(string) {
				aiFeedbackOperationHeld(t, f.e)
				if f.count(t, `SELECT COUNT(*) FROM ai_feedback_events`) != 1 {
					t.Error("logged review failure preceded durable feedback")
				}
			}
			feedback, err := f.e.AddAIFeedback(ctx, id, input)
			aiFeedbackLocksReleased(t, f.e)
			saved := mode == "settings" || mode == "item" || mode == "nil-resolver" || mode == "queue" || mode == "ai-bypasses-settings" || mode == "disabled-bypasses-item"
			if saved {
				if err != nil || feedback.ID == "" || f.count(t, `SELECT COUNT(*) FROM ai_feedback_events WHERE id=?`, feedback.ID) != 1 {
					t.Fatalf("durable result=%+v err=%v", feedback, err)
				}
				log := f.feedbackLog.String()
				if mode == "settings" && !strings.Contains(log, "could not inspect settings for feedback "+feedback.ID+": decode settings: ") {
					t.Fatalf("settings log=%s", log)
				}
				if mode == "item" && !strings.Contains(log, "could not load feedback item "+feedback.ID+": sql: no rows") {
					t.Fatalf("item log=%s", log)
				}
				if mode == "queue" && !strings.Contains(log, "offline review queue rejected") {
					t.Fatalf("queue log=%s", log)
				}
				if (mode == "ai-bypasses-settings" || mode == "disabled-bypasses-item" || mode == "nil-resolver") && log != "" {
					t.Fatalf("bypassed read produced log=%s", log)
				}
			} else {
				if err == nil || !reflect.DeepEqual(feedback, domain.AIFeedbackEvent{}) || f.count(t, `SELECT COUNT(*) FROM ai_feedback_events`) != 0 {
					t.Fatalf("rejected feedback=%+v err=%v", feedback, err)
				}
				if mode == "missing" && !errors.Is(err, sql.ErrNoRows) {
					t.Fatalf("missing err=%v", err)
				}
				if mode == "cancelled" && !errors.Is(err, context.Canceled) {
					t.Fatalf("cancelled err=%v", err)
				}
				if want != "" && !strings.Contains(err.Error(), want) {
					t.Fatalf("err=%v want=%q", err, want)
				}
			}
			if f.countAI(t, false) != 0 || f.resolver.calls.Load() != 0 {
				t.Fatal("guard or failed read launched review")
			}
		})
	}
}

func TestEngineAIFeedbackCharacterizationHistoryAndUndo(t *testing.T) {
	f := newAIFeedbackFixture(t)
	f.e.SetAIDeepResolver(nil)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	// History remains readable while mutation ownership is held.
	f.e.operation.Lock()
	type result struct {
		events []domain.AIFeedbackEvent
		err    error
	}
	read := make(chan result, 1)
	go func() { events, err := f.e.AIFeedbackHistory(ctx, f.item.ID); read <- result{events, err} }()
	var empty []domain.AIFeedbackEvent
	select {
	case got := <-read:
		empty = got.events
		f.e.operation.Unlock()
		if got.err != nil || empty != nil {
			t.Fatalf("empty history=%v err=%v", empty, got.err)
		}
	case <-ctx.Done():
		f.e.operation.Unlock()
		t.Fatal("history acquired mutation lock")
	}
	raw, err := json.Marshal(map[string]any{"feedback": empty})
	if err != nil || string(raw) != `{"feedback":null}` {
		t.Fatalf("empty wire shape=%s err=%v", raw, err)
	}
	add := func(verdict, target, scope string) domain.AIFeedbackEvent {
		t.Helper()
		value, err := f.e.AddAIFeedback(context.Background(), f.item.ID, domain.AIFeedbackInput{Verdict: verdict, TargetType: target, SignalScope: scope, Reason: "insufficient_evidence"})
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	first := add("ai", "post", "social_post")
	second := add("not_ai", "post", "social_post")
	account := add("ai", "account", "author_account")
	if second.SupersedesID != first.ID || account.SupersedesID != "" {
		t.Fatalf("supersedes second=%+v account=%+v", second, account)
	}
	if value, err := f.e.UndoAIFeedback(context.Background(), first.ID); err == nil || err.Error() != "only the current AI feedback decision can be cleared" || !reflect.DeepEqual(value, domain.AIFeedbackEvent{}) {
		t.Fatalf("stale undo=%+v err=%v", value, err)
	}
	cleared, err := f.e.UndoAIFeedback(context.Background(), second.ID)
	if err != nil || cleared.ID == second.ID || cleared.Verdict != "clear" || cleared.SupersedesID != second.ID || cleared.Reason != "" || cleared.TargetKey != second.TargetKey {
		t.Fatalf("clear=%+v err=%v", cleared, err)
	}
	if _, err := f.e.UndoAIFeedback(context.Background(), cleared.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("clear undo err=%v", err)
	}
	f.exec(t, `UPDATE ai_feedback_events SET created_at='2099-01-01T00:00:00Z' WHERE id=?`, first.ID)
	history, err := f.e.AIFeedbackHistory(context.Background(), f.item.ID)
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{}
	for _, value := range history {
		ids = append(ids, value.ID)
	}
	if !reflect.DeepEqual(ids, []string{cleared.ID, account.ID, second.ID, first.ID}) || f.count(t, `SELECT COUNT(*) FROM ai_feedback_events`) != 4 {
		t.Fatalf("append-only history=%v", ids)
	}
	f.exec(t, `INSERT INTO timeline_items(id,session_id,run_id,source,evidence_key,rank,item_json,assessment_json,coverage_json,created_at) SELECT 'feedback-same-account',session_id,run_id,source,'x:another-account-post',1,json_set(item_json,'$.author','Fixture'),'{}','{}',created_at FROM timeline_items WHERE id=?`, f.item.ID)
	shared, err := f.e.AIFeedbackHistory(context.Background(), "feedback-same-account")
	if err != nil || len(shared) != 1 || shared[0].ID != account.ID {
		t.Fatalf("account history=%+v err=%v", shared, err)
	}
	aiFeedbackLocksReleased(t, f.e)
}

func TestEngineAIFeedbackCharacterizationReadUndoErrors(t *testing.T) {
	for _, action := range []string{"history", "undo"} {
		for _, mode := range []string{"missing", "cancelled", "storage"} {
			t.Run(action+"/"+mode, func(t *testing.T) {
				f := newAIFeedbackFixture(t)
				f.e.SetAIDeepResolver(nil)
				ctx := context.Background()
				feedback, err := f.e.AddAIFeedback(ctx, f.item.ID, domain.AIFeedbackInput{Verdict: "ai", TargetType: "post", SignalScope: "social_post"})
				if err != nil {
					t.Fatal(err)
				}
				id := feedback.ID
				if action == "history" {
					id = f.item.ID
				}
				if mode == "missing" {
					id = "missing"
				}
				if mode == "cancelled" {
					var cancel context.CancelFunc
					ctx, cancel = context.WithCancel(ctx)
					cancel()
				}
				if mode == "storage" {
					if action == "history" {
						f.exec(t, `ALTER TABLE ai_feedback_events RENAME TO offline_feedback_events`)
					} else {
						f.exec(t, `CREATE TRIGGER feedback_clear_abort BEFORE INSERT ON ai_feedback_events WHEN NEW.verdict='clear' BEGIN SELECT RAISE(ABORT,'offline clear rejected'); END`)
					}
				}
				if action == "history" {
					events, readErr := f.e.AIFeedbackHistory(ctx, id)
					err = readErr
					if events != nil {
						t.Fatalf("failed history=%+v", events)
					}
				} else {
					value, undoErr := f.e.UndoAIFeedback(ctx, id)
					err = undoErr
					if !reflect.DeepEqual(value, domain.AIFeedbackEvent{}) {
						t.Fatalf("failed undo=%+v", value)
					}
				}
				if err == nil {
					t.Fatal("expected propagated Store error")
				}
				if mode == "missing" && !errors.Is(err, sql.ErrNoRows) {
					t.Fatalf("missing err=%v", err)
				}
				if mode == "cancelled" && !errors.Is(err, context.Canceled) {
					t.Fatalf("cancelled err=%v", err)
				}
				if mode == "storage" && action == "undo" && (!strings.Contains(err.Error(), "offline clear rejected") || f.count(t, `SELECT COUNT(*) FROM ai_feedback_events`) != 1) {
					t.Fatalf("clear rollback err=%v", err)
				}
				aiFeedbackLocksReleased(t, f.e)
			})
		}
	}
}

func TestEngineAIFeedbackCharacterizationCancellationAndContexts(t *testing.T) {
	for _, mode := range []string{"launch", "queue-error", "nil-resolver", "disabled", "not-ai", "request-cancel", "undo-running"} {
		t.Run(mode, func(t *testing.T) {
			f := newAIFeedbackFixture(t)
			ctx, cancelRequest := context.WithCancel(context.Background())
			defer cancelRequest()
			r, entered, unblock := feedbackControlledResolver(t, f, func() {
				aiFeedbackOperationHeld(t, f.e)
				if mode == "request-cancel" {
					cancelRequest()
				}
			})
			keys := []string{"ai:" + f.sessionID + ":old", "ai:other:old", "media-provenance"}
			calls := [3]int{}
			f.e.mu.Lock()
			for index, key := range keys {
				index := index
				f.e.work.registerActive(key, func() {
					calls[index]++
					aiFeedbackOperationHeld(t, f.e)
					if f.e.mu.TryLock() {
						f.e.mu.Unlock()
						t.Error("cancellation released mu")
					}
					if f.count(t, `SELECT COUNT(*) FROM ai_feedback_events`) != 1 || f.countAI(t, false) != 0 {
						t.Error("feedback/cancel/queue order changed")
					}
				})
			}
			f.e.mu.Unlock()
			retire := func() {
				f.e.mu.Lock()
				for _, key := range keys {
					f.e.work.retireActive(key)
				}
				f.e.mu.Unlock()
			}
			t.Cleanup(retire)
			input := domain.AIFeedbackInput{Verdict: "unsure", TargetType: "post", SignalScope: "social_post"}
			switch mode {
			case "queue-error":
				f.exec(t, `CREATE TRIGGER feedback_queue_abort BEFORE INSERT ON ai_detection_jobs BEGIN SELECT RAISE(ABORT,'offline queue rejected'); END`)
			case "nil-resolver":
				f.e.SetAIDeepResolver(nil)
			case "disabled":
				settings, err := f.state.GetSettings(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				settings.AIDetectionEnabled = false
				if err := f.state.SaveSettings(context.Background(), settings); err != nil {
					t.Fatal(err)
				}
			case "not-ai":
				input.Verdict = "not_ai"
			}
			feedback, err := f.e.AddAIFeedback(ctx, f.item.ID, input)
			if err != nil || feedback.ID == "" {
				t.Fatalf("feedback=%+v err=%v", feedback, err)
			}
			aiFeedbackLocksReleased(t, f.e)
			wantCancel, wantJobs := 1, 1
			if mode == "nil-resolver" || mode == "disabled" || mode == "not-ai" {
				wantCancel, wantJobs = 0, 0
			}
			if mode == "queue-error" {
				wantJobs = 0
			}
			if calls != [3]int{wantCancel, 0, 0} || f.countAI(t, false) != wantJobs {
				t.Fatalf("cancel=%v jobs=%d", calls, f.countAI(t, false))
			}
			if wantJobs == 1 {
				var call aiFeedbackResolveCall
				select {
				case call = <-entered:
				case <-time.After(3 * time.Second):
					t.Fatal("review did not start")
				}
				if call.ctx.Err() != nil || len(call.items) != 1 || call.items[0].ID != f.item.ID {
					t.Fatalf("review context=%v items=%+v", call.ctx.Err(), call.items)
				}
				if mode == "request-cancel" && ctx.Err() != context.Canceled {
					t.Fatal("request cancellation fixture did not fire")
				}
				if mode == "undo-running" {
					value, err := f.e.UndoAIFeedback(context.Background(), feedback.ID)
					if err != nil || value.Verdict != "clear" || call.ctx.Err() != nil || calls != [3]int{1, 0, 0} || f.count(t, `SELECT COUNT(*) FROM ai_feedback_events`) != 2 {
						t.Fatalf("undo cancelled or rewrote running review: value=%+v err=%v", value, err)
					}
				}
				retire()
				unblock()
				if !f.e.WaitForIdle(3 * time.Second) {
					t.Fatal("review did not retire")
				}
				if job := f.job(t); job == nil || job.Status != "completed" || r.calls.Load() != 1 {
					t.Fatalf("review result=%+v calls=%d", job, r.calls.Load())
				}
			}
		})
	}
}
