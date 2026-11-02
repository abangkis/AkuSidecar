package engine

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/store"
)

type libraryRemovalFixture struct {
	e             *Engine
	state         *store.Store
	db            *sql.DB
	item          domain.MemoryItem
	input         domain.MemoryItemInput
	topics        []string
	cancellations map[string]int
}

func newLibraryRemovalFixture(t *testing.T, members int) libraryRemovalFixture {
	t.Helper()
	e, state := testEngine(t)
	input := livingTopicMemoryInput("Library removal fixture")
	item, err := state.CreateMemoryRecallStub(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", state.Path())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	f := libraryRemovalFixture{e: e, state: state, db: db, item: item, input: input, cancellations: map[string]int{}}
	for i := 0; i < members; i++ {
		topic, err := state.CreateLivingTopic(context.Background(), fmt.Sprintf("Library topic %d", i))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := state.AddLivingTopicMember(context.Background(), topic.ID, item.ID); err != nil {
			t.Fatal(err)
		}
		f.topics = append(f.topics, topic.ID)
	}
	sort.Strings(f.topics)
	// Keep the real queue path while suppressing inference and worker scheduling.
	for _, key := range []string{"living-topics-understanding", "ai:fixture", "media-provenance", "unrelated"} {
		key := key
		e.work.active[key] = func() { f.cancellations[key]++ }
	}
	f.exec(t, `CREATE TABLE library_removal_audit(topic_id TEXT,event_trigger TEXT,lifecycle TEXT,memberships INTEGER)`)
	f.exec(t, fmt.Sprintf(`CREATE TRIGGER library_removal_queue_audit AFTER INSERT ON living_topic_understanding_jobs BEGIN
		INSERT INTO library_removal_audit SELECT NEW.topic_id,NEW.trigger,
		COALESCE((SELECT lifecycle_state FROM memory_items WHERE id='%s'),'absent'),
		(SELECT COUNT(*) FROM living_topic_memberships WHERE memory_item_id='%s'); END`, item.ID, item.ID))
	return f
}

func (f libraryRemovalFixture) exec(t *testing.T, query string, args ...any) {
	t.Helper()
	if _, err := f.db.Exec(query, args...); err != nil {
		t.Fatal(err)
	}
}

func (f libraryRemovalFixture) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := f.db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (f libraryRemovalFixture) call(ctx context.Context, action, id string) (domain.MemoryItem, error) {
	if action == "forget" {
		return f.e.ForgetLibraryItem(ctx, id)
	}
	return domain.MemoryItem{}, f.e.RemoveLibraryItem(ctx, id)
}

func libraryRemovalLocksFree(t *testing.T, e *Engine) {
	t.Helper()
	if !e.operation.TryLock() {
		t.Error("Library removal unexpectedly holds operation lock")
	} else {
		e.operation.Unlock()
	}
	if !e.mu.TryLock() {
		t.Error("Library removal unexpectedly holds mu")
	} else {
		e.mu.Unlock()
	}
}

func (f libraryRemovalFixture) assertWorkersUnchanged(t *testing.T) {
	t.Helper()
	if len(f.cancellations) != 0 || len(f.e.work.active) != 4 {
		t.Fatalf("worker/cancellation ownership changed: calls=%v active=%d", f.cancellations, len(f.e.work.active))
	}
	libraryRemovalLocksFree(t, f.e)
}

func (f libraryRemovalFixture) assertAudit(t *testing.T, action string, topics []string) {
	t.Helper()
	rows, err := f.db.Query(`SELECT topic_id,event_trigger,lifecycle,memberships FROM library_removal_audit ORDER BY rowid`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	got := []string{}
	trigger, lifecycle := "evidence_removed", "absent"
	if action == "forget" {
		trigger, lifecycle = "evidence_forgotten", "tombstone"
	}
	for rows.Next() {
		var id, event, state string
		var members int
		if err := rows.Scan(&id, &event, &state, &members); err != nil {
			t.Fatal(err)
		}
		if event != trigger || state != lifecycle || members != 0 {
			t.Fatalf("queue ran before committed mutation or with wrong trigger: %s %s %s %d", id, event, state, members)
		}
		got = append(got, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, append([]string{}, topics...)) {
		t.Fatalf("topic snapshot/queue order got=%v want=%v", got, topics)
	}
}

func TestEngineLibraryRemovalCharacterizationSuccess(t *testing.T) {
	for _, action := range []string{"remove", "forget"} {
		for _, members := range []int{0, 1, 3} {
			t.Run(fmt.Sprintf("%s/topics=%d", action, members), func(t *testing.T) {
				f := newLibraryRemovalFixture(t, members)
				result, err := f.call(context.Background(), action, f.item.ID)
				if err != nil {
					t.Fatal(err)
				}
				f.assertAudit(t, action, f.topics)
				if action == "forget" {
					stored, err := f.state.MemoryItem(context.Background(), f.item.ID)
					if err != nil || !reflect.DeepEqual(result, stored) || result.LifecycleState != domain.MemoryStateTombstone || result.Title != "" || result.Summary != "" || result.Identity.CanonicalPermalink != "" {
						t.Fatalf("Forget return/privacy contract changed: result=%+v err=%v", result, err)
					}
					if _, err := f.state.CreateMemoryRecallStub(context.Background(), f.input); !errors.Is(err, store.ErrMemoryTombstoned) {
						t.Fatalf("Forget allowed recapture: %v", err)
					}
					f.exec(t, `DELETE FROM library_removal_audit`)
					repeated, err := f.call(context.Background(), action, f.item.ID)
					if err != nil || !reflect.DeepEqual(result, repeated) {
						t.Fatalf("repeat Forget changed result: %+v %v", repeated, err)
					}
					f.assertAudit(t, action, nil)
				} else {
					if f.count(t, `SELECT COUNT(*) FROM memory_items WHERE id=?`, f.item.ID) != 0 {
						t.Fatal("Remove retained memory row")
					}
					if _, err := f.state.CreateMemoryRecallStub(context.Background(), f.input); err != nil {
						t.Fatalf("Remove blocked recapture: %v", err)
					}
				}
				f.assertWorkersUnchanged(t)
			})
		}
	}
}

type libraryRemovalContext struct {
	context.Context
	once  sync.Once
	probe func()
}

func (c *libraryRemovalContext) Done() <-chan struct{} {
	c.once.Do(c.probe)
	return c.Context.Done()
}

func TestEngineLibraryRemovalCharacterizationCallerContextAndLocks(t *testing.T) {
	for _, action := range []string{"remove", "forget"} {
		for _, cancelLookup := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/cancel_lookup=%t", action, cancelLookup), func(t *testing.T) {
				f := newLibraryRemovalFixture(t, 1)
				base, cancel := context.WithCancel(context.Background())
				defer cancel()
				observed := false
				ctx := &libraryRemovalContext{Context: base, probe: func() {
					observed = true
					libraryRemovalLocksFree(t, f.e)
					if cancelLookup {
						cancel()
					}
				}}
				result, err := f.call(ctx, action, f.item.ID)
				if !observed {
					t.Fatal("caller context was not forwarded to Store")
				}
				if cancelLookup {
					if !errors.Is(err, context.Canceled) || !reflect.DeepEqual(result, domain.MemoryItem{}) {
						t.Fatalf("context error/zero result changed: %+v %v", result, err)
					}
					if f.count(t, `SELECT COUNT(*) FROM memory_items WHERE id=? AND lifecycle_state='active'`, f.item.ID) != 1 {
						t.Fatal("cancelled lookup mutated item")
					}
					f.assertAudit(t, action, nil)
				} else if err != nil {
					t.Fatal(err)
				}
				f.assertWorkersUnchanged(t)
			})
		}
	}
}

func TestEngineLibraryRemovalCharacterizationFailures(t *testing.T) {
	for _, action := range []string{"remove", "forget"} {
		for _, failure := range []string{"empty", "missing", "spaced_id", "lookup", "mutation", "cancelled"} {
			t.Run(action+"/"+failure, func(t *testing.T) {
				f := newLibraryRemovalFixture(t, 1)
				id, ctx := f.item.ID, context.Background()
				switch failure {
				case "empty":
					id = " \t"
				case "missing":
					id = "missing-library-item"
				case "spaced_id":
					id = " " + id + " "
				case "lookup":
					f.exec(t, `ALTER TABLE living_topic_memberships RENAME TO saved_library_memberships`)
				case "mutation":
					operation := "DELETE"
					if action == "forget" {
						operation = "UPDATE"
					}
					f.exec(t, `CREATE TRIGGER fail_library_mutation BEFORE `+operation+` ON memory_items BEGIN SELECT RAISE(ABORT,'library-mutation-failed'); END`)
				case "cancelled":
					var cancel context.CancelFunc
					ctx, cancel = context.WithCancel(ctx)
					cancel()
				}
				result, err := f.call(ctx, action, id)
				if err == nil || !reflect.DeepEqual(result, domain.MemoryItem{}) {
					t.Fatalf("failure return changed: result=%+v err=%v", result, err)
				}
				switch failure {
				case "empty", "missing", "spaced_id":
					if !errors.Is(err, store.ErrMemoryNotFound) {
						t.Fatalf("not-found error identity lost: %v", err)
					}
				case "cancelled":
					if !errors.Is(err, context.Canceled) {
						t.Fatalf("cancellation identity lost: %v", err)
					}
				case "lookup":
					if !strings.Contains(err.Error(), "no such table: living_topic_memberships") {
						t.Fatalf("lookup error changed: %v", err)
					}
				case "mutation":
					if !strings.Contains(err.Error(), "library-mutation-failed") {
						t.Fatalf("mutation error changed: %v", err)
					}
				}
				if f.count(t, `SELECT COUNT(*) FROM memory_items WHERE id=? AND lifecycle_state='active'`, f.item.ID) != 1 {
					t.Fatal("failed mutation changed privacy state")
				}
				if failure != "lookup" && f.count(t, `SELECT COUNT(*) FROM living_topic_memberships WHERE memory_item_id=?`, f.item.ID) != 1 {
					t.Fatal("failed mutation lost membership")
				}
				f.assertAudit(t, action, nil)
				f.assertWorkersUnchanged(t)
			})
		}
	}
}

type libraryRemovalLog struct {
	messages []string
	observe  func(string)
}

func (l *libraryRemovalLog) Printf(format string, args ...any) {
	message := fmt.Sprintf(format, args...)
	l.messages = append(l.messages, message)
	if l.observe != nil {
		l.observe(message)
	}
}

func TestEngineLibraryRemovalCharacterizationQueueFailureContinuesWithBackgroundContext(t *testing.T) {
	for _, action := range []string{"remove", "forget"} {
		t.Run(action, func(t *testing.T) {
			f := newLibraryRemovalFixture(t, 3)
			f.exec(t, fmt.Sprintf(`CREATE TRIGGER fail_first_library_queue BEFORE INSERT ON living_topic_understanding_jobs WHEN NEW.topic_id='%s' BEGIN SELECT RAISE(ABORT,'first-library-queue-failed'); END`, f.topics[0]))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			log := &libraryRemovalLog{observe: func(message string) {
				libraryRemovalLocksFree(t, f.e)
				if !strings.Contains(message, f.topics[0]) || !strings.Contains(message, "first-library-queue-failed") {
					t.Error("queue error was not logged for first topic", message)
				}
				cancel()
			}}
			f.e.logger = log
			result, err := f.call(ctx, action, f.item.ID)
			if err != nil || len(log.messages) != 1 || !errors.Is(ctx.Err(), context.Canceled) {
				t.Fatalf("post-commit queue failure changed caller result: %+v %v logs=%v", result, err, log.messages)
			}
			if action == "forget" && result.LifecycleState != domain.MemoryStateTombstone {
				t.Fatal("Forget lost committed return value")
			}
			f.assertAudit(t, action, f.topics[1:])
			f.assertWorkersUnchanged(t)
		})
	}
}
