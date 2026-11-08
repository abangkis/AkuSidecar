package store

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestCompositionPlanStoreOrderingAndKnowledge(t *testing.T) {
	state, sessionID := compositionPlanFixture(t)
	ctx := context.Background()
	if err := state.ComposeSession(ctx, sessionID); err != nil {
		t.Fatal(err)
	}
	rows, err := state.db.QueryContext(ctx, `SELECT id,rank FROM timeline_items WHERE session_id=? ORDER BY rank`, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for rows.Next() {
		var id string
		var rank int
		if err := rows.Scan(&id, &rank); err != nil {
			t.Fatal(err)
		}
		if rank != len(got) {
			t.Fatalf("rank=%d, want %d", rank, len(got))
		}
		got = append(got, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	if want := []string{"composition-b", "composition-d", "composition-c"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("order=%v, want %v", got, want)
	}
	var evidence string
	if err := state.db.QueryRowContext(ctx, `SELECT group_concat(evidence_key, ',') FROM (SELECT evidence_key FROM knowledge_events ORDER BY evidence_key)`).Scan(&evidence); err != nil {
		t.Fatal(err)
	}
	if evidence != "composition-b,composition-d" {
		t.Fatalf("knowledge=%q; only kept items with EventKey should be saved", evidence)
	}
}

func TestCompositionPlanStoreFailuresPreserveState(t *testing.T) {
	tests := []struct {
		name, setup, message string
	}{
		{"rank update", `CREATE TRIGGER composition_failure BEFORE UPDATE OF rank ON timeline_items WHEN OLD.id='composition-c' BEGIN SELECT RAISE(ABORT,'composition update failure'); END`, "composition update failure"},
		{"knowledge insert", `CREATE TRIGGER composition_failure BEFORE INSERT ON knowledge_events WHEN NEW.evidence_key='composition-d' BEGIN SELECT RAISE(ABORT,'composition knowledge failure'); END`, "composition knowledge failure"},
		{"timeline delete", `CREATE TRIGGER composition_failure BEFORE DELETE ON timeline_items WHEN OLD.id='composition-a' BEGIN SELECT RAISE(ABORT,'composition delete failure'); END`, "composition delete failure"},
		{"invalid pruned JSON", `UPDATE timeline_items SET item_json='invalid' WHERE id='composition-a'`, "invalid character"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state, sessionID := compositionPlanFixture(t)
			ctx := context.Background()
			if _, err := state.db.ExecContext(ctx, test.setup); err != nil {
				t.Fatal(err)
			}
			if err := state.ComposeSession(ctx, sessionID); err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("error=%v, want %q", err, test.message)
			}
			var ranks string
			if err := state.db.QueryRowContext(ctx, `SELECT group_concat(rank, ',') FROM (SELECT rank FROM timeline_items WHERE session_id=? ORDER BY id)`, sessionID).Scan(&ranks); err != nil {
				t.Fatal(err)
			}
			if ranks != "5,9,4,8" {
				t.Fatalf("failure changed rows/ranks: %q", ranks)
			}
			var knowledgeCount int
			if err := state.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM knowledge_events`).Scan(&knowledgeCount); err != nil {
				t.Fatal(err)
			}
			if knowledgeCount != 0 {
				t.Fatalf("failure persisted %d knowledge rows", knowledgeCount)
			}
		})
	}
}

func compositionPlanFixture(t *testing.T) (*Store, string) {
	t.Helper()
	ctx := context.Background()
	state := openTestStore(t)
	settings, err := state.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	session, err := createVisibleUpdateSession(state, ctx, "composition plan fixture", settings)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.db.ExecContext(ctx, `UPDATE sessions SET max_items_total=3 WHERE id=?`, session.ID); err != nil {
		t.Fatal(err)
	}
	run := session.Runs[0]
	fixtures := []struct {
		id, created, eventKey string
		score                 float64
		rank                  int
	}{
		{"composition-a", "2026-01-02T00:00:00Z", "event-a", .9, 5},
		{"composition-b", "2026-01-03T00:00:00Z", "event-b", 1, 9},
		{"composition-c", "2026-01-02T00:00:00Z", "", .9, 4},
		{"composition-d", "2026-01-01T00:00:00Z", "event-d", .9, 8},
	}
	for _, fixture := range fixtures {
		itemRaw, err := json.Marshal(domain.ReasonedItem{EvidenceKey: fixture.id, Source: run.Source, EventKey: fixture.eventKey})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := state.db.ExecContext(ctx, `INSERT INTO candidate_assessments(run_id,evidence_key,source,assessment_json,base_score,preference_score,final_score,selected,created_at) VALUES(?,?,?,?,?,?,?,?,?)`, run.ID, fixture.id, run.Source, "{}", fixture.score, 0, fixture.score, 1, fixture.created); err != nil {
			t.Fatal(err)
		}
		if _, err := state.db.ExecContext(ctx, `INSERT INTO timeline_items(id,session_id,run_id,source,evidence_key,rank,item_json,assessment_json,coverage_json,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, fixture.id, session.ID, run.ID, run.Source, fixture.id, fixture.rank, string(itemRaw), "{}", "{}", fixture.created); err != nil {
			t.Fatal(err)
		}
	}
	return state, session.ID
}
