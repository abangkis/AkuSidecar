package store

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"testing"
)

func directFixture(t *testing.T, s *Store, id string, b domain.Block) {
	t.Helper()
	raw, _ := json.Marshal(b)
	if _, err := s.db.Exec(`UPDATE timeline_items SET evidence_snapshot_json=?,item_json='{}',assessment_json='{}' WHERE id=?`, string(raw), id); err != nil {
		t.Fatal(err)
	}
}

func TestDirectContextWithoutLexicalTermsRefreshesEvidenceAndRejectsSelf(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	id := insertContentContextTimelineFixture(t, s, false)
	own := "https://x.com/owner/status/12345"
	relation := domain.DirectContext{Kind: "replies_to", Provenance: "observed_response", Target: domain.ContextObject{Kind: "post", Permalink: "https://x.com/other/status/67890", Text: "Reply target evidence", Availability: "captured"}}
	b := domain.Block{Permalink: own, DirectContext: []domain.DirectContext{relation, relation}}
	self := relation
	self.Target.Permalink = "https://x.com/i/status/12345"
	b.DirectContext = append(b.DirectContext, self)
	directFixture(t, s, id, b)
	result, err := s.ContentContext(ctx, id, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.DirectContext) != 1 || result.DirectContext[0].Target.Text != "Reply target evidence" || len(result.Matches) != 0 {
		t.Fatalf("%+v", result)
	}
	b.DirectContext[0].Target.Text = "Updated reply target"
	directFixture(t, s, id, b)
	next, err := s.ContentContext(ctx, id, 3)
	if err != nil || next.DirectContext[0].Target.Text != "Updated reply target" {
		t.Fatalf("%+v %v", next, err)
	}
}

func TestDirectContextResolvesExactOlderParentAndActiveFullCopy(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	id := insertContentContextTimelineFixture(t, s, false)
	parent := domain.Block{Permalink: "https://x.com/original/status/67890", Text: "Original parent", Author: "Original author"}
	raw, _ := json.Marshal(parent)
	if _, err := s.db.Exec(`INSERT INTO timeline_items(id,session_id,run_id,source,evidence_key,rank,item_json,assessment_json,coverage_json,created_at,evidence_snapshot_json)
 SELECT 'parent',session_id,run_id,source,'parent-evidence',1,'{}','{}','{}','2020-01-01',? FROM timeline_items WHERE id=?`, string(raw), id); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 205; i++ {
		if _, err := s.db.Exec(`INSERT INTO timeline_items(id,session_id,run_id,source,evidence_key,rank,item_json,assessment_json,coverage_json,created_at,evidence_snapshot_json)
 SELECT ?,session_id,run_id,source,?,1,'{}','{}','{}','2030-01-01','{}' FROM timeline_items WHERE id=?`, fmt.Sprint("unrelated", i), fmt.Sprint("unrelated-evidence", i), id); err != nil {
			t.Fatal(err)
		}
	}
	relation := domain.DirectContext{Kind: "replies_to", Provenance: "observed_response", Target: domain.ContextObject{Kind: "post", Permalink: "https://x.com/i/status/67890", Availability: "reference_only"}}
	directFixture(t, s, id, domain.Block{Permalink: "https://x.com/owner/status/12345", DirectContext: []domain.DirectContext{relation}})
	result, err := s.ContentContext(ctx, id, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.DirectContext) != 1 || result.DirectContext[0].Target.Text != "Original parent" || result.DirectContext[0].Target.EvidenceOrigin != "local_timeline" {
		t.Fatalf("%+v", result)
	}
	if _, err = s.db.Exec(`DELETE FROM timeline_items WHERE id='parent'`); err != nil {
		t.Fatal(err)
	}
	input := libraryInput("exact-parent", domain.SourceX, "Summary must not become original", "Summary", domain.Now())
	input.Identity.CanonicalPermalink = parent.Permalink
	memory, err := s.CreateMemoryRecallStub(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	result, err = s.ContentContext(ctx, id, 3)
	if err != nil || result.DirectContext[0].Target.Text != "" {
		t.Fatalf("recall stub used as original: %+v %v", result, err)
	}
	if _, err = s.KeepMemoryFullCopy(ctx, memory.ID, domain.MemoryFullCopyInput{Content: "Authored full copy"}); err != nil {
		t.Fatal(err)
	}
	result, err = s.ContentContext(ctx, id, 3)
	if err != nil || result.DirectContext[0].Target.Text != "Authored full copy" || result.DirectContext[0].Target.EvidenceOrigin != "local_memory" {
		t.Fatalf("%+v %v", result, err)
	}
}

func TestDirectContextLegacyDoesNotTrustReplyURLAndContextFingerprintIgnoresTime(t *testing.T) {
	s := openTestStore(t)
	id := insertContentContextTimelineFixture(t, s, false)
	b := domain.Block{Permalink: "https://x.com/owner/status/12345", RelationshipType: "reply", ParentPermalink: "https://x.com/parent/status/67890"}
	directFixture(t, s, id, b)
	result, err := s.ContentContext(context.Background(), id, 3)
	if err != nil || len(result.DirectContext) != 0 {
		t.Fatalf("%+v %v", result, err)
	}
	b.DirectContext = []domain.DirectContext{{Kind: "feed_comment", ActorURL: "https://www.linkedin.com/in/alice", CapturedAt: domain.Now(), Target: domain.ContextObject{Kind: "comment", Availability: "reference_only"}}}
	first := continuityContextFingerprint(b)
	b.DirectContext[0].CapturedAt = "later"
	if continuityContextFingerprint(b) != first {
		t.Fatal("time caused resurfacing")
	}
	b.DirectContext[0].ActorURL = "https://www.linkedin.com/in/bob"
	if continuityContextFingerprint(b) != first {
		t.Fatal("direct context changed provider reevaluation policy")
	}
}

func TestDirectContextRejectsIDOnlySelfAndResolvesIDOnlyParent(t *testing.T) {
	s := openTestStore(t)
	id := insertContentContextTimelineFixture(t, s, false)
	self := domain.DirectContext{Kind: "replies_to", Provenance: "observed_response", Target: domain.ContextObject{Kind: "post", ID: "12345", Availability: "reference_only"}}
	directFixture(t, s, id, domain.Block{PlatformID: "x:status:12345", DirectContext: []domain.DirectContext{self}})
	result, err := s.ContentContext(context.Background(), id, 3)
	if err != nil || len(result.DirectContext) != 0 {
		t.Fatalf("self surfaced: %+v %v", result, err)
	}
	self.Target.ID = "67890"
	directFixture(t, s, id, domain.Block{PlatformID: "x:status:12345", DirectContext: []domain.DirectContext{self}})
	result, err = s.ContentContext(context.Background(), id, 3)
	if err != nil || len(result.DirectContext) != 1 || result.DirectContext[0].Target.Permalink != "https://x.com/i/status/67890" {
		t.Fatalf("%+v %v", result, err)
	}
}

func TestDirectContextReadsLaterRetainedInteractionWithoutReevaluation(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	id := insertContentContextTimelineFixture(t, s, false)
	original := domain.Block{EvidenceKey: "linkedin:activity:1234567", PlatformID: "linkedin:activity:1234567", Permalink: "https://www.linkedin.com/feed/update/urn:li:activity:1234567"}
	if _, err := s.db.Exec(`UPDATE timeline_items SET source='linkedin' WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}
	directFixture(t, s, id, original)
	var runID string
	if err := s.db.QueryRow(`SELECT run_id FROM timeline_items WHERE id=?`, id).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	for index, actor := range []string{"alice", "bob"} {
		candidate := original
		candidate.DirectContext = []domain.DirectContext{{Kind: "feed_comment", ActorURL: "https://www.linkedin.com/in/" + actor, ObservedText: actor + " commented", Provenance: "observed_dom", Target: domain.ContextObject{Kind: "comment", Availability: "reference_only"}}}
		raw, _ := json.Marshal(domain.Observation{Source: domain.SourceLinkedIn, Snapshots: []domain.Snapshot{{Blocks: []domain.Block{candidate}}}})
		commandID := fmt.Sprint("context-command-", index)
		if _, err := s.db.Exec(`INSERT INTO bridge_commands(id,run_id,type,status,payload_json,created_at) VALUES(?,?,'collect_visible','completed','{}',?)`, commandID, runID, domain.Now()); err != nil {
			t.Fatal(err)
		}
		if _, err := s.db.Exec(`INSERT INTO observations(id,run_id,command_id,source,observation_json,captured_at,created_at) VALUES(?,?,?,'linkedin',?,?,?)`, fmt.Sprint("context-observation-", index), runID, commandID, string(raw), fmt.Sprintf("2026-09-%02dT00:00:00Z", index+1), domain.Now()); err != nil {
			t.Fatal(err)
		}
	}
	result, err := s.ContentContext(ctx, id, 3)
	if err != nil || len(result.DirectContext) != 2 {
		t.Fatalf("%+v %v", result, err)
	}
	var count int
	if err = s.db.QueryRow(`SELECT count(*) FROM timeline_items`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("context read changed timeline: %d %v", count, err)
	}
}

func TestDirectContextDoesNotReadRunningOrPreparedObservations(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	id := insertContentContextTimelineFixture(t, s, false)
	block := domain.Block{EvidenceKey: "x:status:12345", PlatformID: "x:status:12345", Permalink: "https://x.com/owner/status/12345"}
	directFixture(t, s, id, block)
	settings, err := s.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	settings.ActiveSources = []domain.Source{domain.SourceX}
	session, err := createPreparedUpdateSession(s, ctx, "prepared context", settings)
	if err != nil {
		t.Fatal(err)
	}
	runs, err := s.listRuns(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	command, err := s.StartRun(ctx, runs[0].ID, map[string]any{"source": domain.SourceX})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ClaimCommand(ctx, runs[0].ID, "context-test"); err != nil {
		t.Fatal(err)
	}
	block.DirectContext = []domain.DirectContext{{Kind: "replies_to", Provenance: "observed_response", Target: domain.ContextObject{Kind: "post", ID: "67890", Text: "Hidden body", Availability: "captured"}}}
	if err = s.SaveObservation(ctx, command.ID, runs[0].ID, domain.Observation{Source: domain.SourceX, CapturedAt: domain.Now(), Snapshots: []domain.Snapshot{{Blocks: []domain.Block{block}}}, Coverage: map[string]any{}}); err != nil {
		t.Fatal(err)
	}
	assertEmpty := func() {
		t.Helper()
		result, err := s.ContentContext(ctx, id, 3)
		if err != nil || len(result.DirectContext) != 0 {
			t.Fatalf("hidden context leaked %+v %v", result, err)
		}
	}
	assertEmpty()
	if _, err = s.db.Exec(`UPDATE sessions SET status='completed',completed_at=? WHERE id=?`, domain.Now(), session.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec(`UPDATE auto_update_batches SET state='prepared' WHERE session_id=?`, session.ID); err != nil {
		t.Fatal(err)
	}
	assertEmpty()
	if _, err = s.db.Exec(`UPDATE auto_update_batches SET state='visible' WHERE session_id=?`, session.ID); err != nil {
		t.Fatal(err)
	}
	result, err := s.ContentContext(ctx, id, 3)
	if err != nil || len(result.DirectContext) != 1 {
		t.Fatalf("visible context unavailable %+v %v", result, err)
	}
}

func TestDirectContextExcludesXQuotesButRetainsTimelineEvidence(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	id := insertContentContextTimelineFixture(t, s, false)
	reply := domain.DirectContext{Kind: "replies_to", Provenance: "observed_response", Target: domain.ContextObject{Kind: "post", ID: "99999", Availability: "reference_only"}}
	typedQuote := domain.DirectContext{Kind: "quotes", Provenance: "observed_dom", Target: domain.ContextObject{Kind: "post", ID: "67890", Text: "Typed quote capture", Availability: "captured"}}
	b := domain.Block{
		Permalink:     "https://x.com/owner/status/12345",
		QuotedPost:    map[string]any{"text": "Legacy inline quote", "permalink": "https://x.com/quote/status/67890"},
		DirectContext: []domain.DirectContext{typedQuote, reply},
	}
	directFixture(t, s, id, b)

	result, err := s.ContentContext(ctx, id, 3)
	if err != nil || len(result.DirectContext) != 1 || result.DirectContext[0].Kind != "replies_to" {
		t.Fatalf("quote leaked or reply missing: %+v %v", result, err)
	}
	item, err := s.TimelineItem(ctx, id)
	if err != nil || item.Evidence == nil || item.Evidence.QuotedPost["text"] != "Legacy inline quote" || len(item.Evidence.DirectContext) != 2 {
		t.Fatalf("quote Timeline evidence changed: %+v %v", item.Evidence, err)
	}

	// Legacy snapshots may have only quotedPost; it must not be synthesized
	// into Related Context, while the typed reply remains visible.
	b.DirectContext = []domain.DirectContext{reply}
	directFixture(t, s, id, b)
	result, err = s.ContentContext(ctx, id, 3)
	if err != nil || len(result.DirectContext) != 1 || result.DirectContext[0].Kind != "replies_to" {
		t.Fatalf("legacy quote leaked or reply missing: %+v %v", result, err)
	}
}
