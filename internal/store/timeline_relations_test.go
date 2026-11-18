package store

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestProjectTimelineRelationsResolvesLoadedParentAndIDOnlyReference(t *testing.T) {
	for _, target := range []domain.ContextObject{
		{Kind: "post", ID: "12345", Permalink: "https://x.com/i/status/12345", Availability: "reference_only"},
		{Kind: "post", ID: "12345", Availability: "reference_only"},
	} {
		parent := timelineRelationItem("parent", domain.SourceX, "12345", "https://x.com/alice/status/12345", nil)
		reply := timelineRelationItem("reply", domain.SourceX, "67890", "https://x.com/bob/status/67890", []domain.DirectContext{{
			Kind: "replies_to", Provenance: "observed_response", Target: target,
		}})
		before, err := json.Marshal(reply.Evidence)
		if err != nil {
			t.Fatal(err)
		}
		items := []domain.TimelineItem{parent, reply}
		projectTimelineRelations(items)
		conversation := items[1].Conversation
		if conversation == nil || conversation.Kind != "replies_to" || conversation.Status != "resolved" || conversation.TargetTimelineID != "parent" || conversation.Provenance != "observed_response" {
			t.Fatalf("conversation=%+v", conversation)
		}
		after, err := json.Marshal(items[1].Evidence)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(before, after) {
			t.Fatal("projection mutated captured evidence")
		}
	}
}

func TestProjectTimelineRelationsIgnoresInvalidQuoteFeedCommentAndSelfEdges(t *testing.T) {
	item := timelineRelationItem("reply", domain.SourceX, "12345", "https://x.com/alice/status/12345", []domain.DirectContext{
		{Kind: "quotes", Provenance: "observed_dom", Target: domain.ContextObject{Kind: "post", ID: "67890", Availability: "reference_only"}},
		{Kind: "replies_to", Provenance: "observed_response", Target: domain.ContextObject{Kind: "post", ID: "bad-id", Availability: "reference_only"}},
		{Kind: "feed_comment", Provenance: "observed_dom", Target: domain.ContextObject{Kind: "comment", ID: "67890", Availability: "reference_only"}},
		{Kind: "replies_to", Provenance: "observed_response", Target: domain.ContextObject{Kind: "post", ID: "12345", Availability: "reference_only"}},
	})
	items := []domain.TimelineItem{item}
	projectTimelineRelations(items)
	if items[0].Conversation != nil {
		t.Fatalf("unexpected conversation projection: %+v", items[0].Conversation)
	}
}

func TestProjectTimelineRelationsRequiresOwnIdentityAndRetainsCorroboratingProvenance(t *testing.T) {
	parent := timelineRelationItem("parent", domain.SourceX, "12345", "https://x.com/alice/status/12345", nil)
	relations := []domain.DirectContext{
		{Kind: "replies_to", Provenance: "observed_response", Target: domain.ContextObject{Kind: "post", ID: "12345", Availability: "reference_only"}},
		{Kind: "replies_to", Provenance: "observed_dom", Target: domain.ContextObject{Kind: "post", ID: "12345", Availability: "reference_only"}},
	}
	for _, test := range []struct {
		platformID, permalink string
		resolved              bool
	}{
		{"67890", "https://x.com/bob/status/67890", true},
		{"67890", "https://x.com/bob/status/99999", false},
		{"67890", "https://evil.example/bob/status/67890", false},
		{"", "", false},
	} {
		items := []domain.TimelineItem{parent, timelineRelationItem("reply", domain.SourceX, test.platformID, test.permalink, relations)}
		projectTimelineRelations(items)
		if test.resolved {
			if got := items[1].Conversation; got == nil || got.Status != "resolved" || got.Provenance != "observed_response" {
				t.Fatalf("corroborating observations lost: %+v", got)
			}
		} else if items[1].Conversation != nil {
			t.Fatalf("invalid own identity admitted: %+v", items[1].Conversation)
		}
	}
}

func TestProjectTimelineRelationsMarksDuplicateAndConflictingParentsAmbiguous(t *testing.T) {
	duplicateParentReply := timelineRelationItem("duplicate-reply", domain.SourceX, "90001", "https://x.com/replier/status/90001", []domain.DirectContext{{
		Kind: "replies_to", Provenance: "observed_response", Target: domain.ContextObject{Kind: "post", ID: "12345", Availability: "reference_only"},
	}})
	duplicateParents := []domain.TimelineItem{
		timelineRelationItem("parent-a", domain.SourceX, "12345", "https://x.com/alice/status/12345", nil),
		timelineRelationItem("parent-b", domain.SourceX, "12345", "https://x.com/other/status/12345", nil),
		duplicateParentReply,
	}
	projectTimelineRelations(duplicateParents)
	if got := duplicateParents[2].Conversation; got == nil || got.Status != "ambiguous" || got.TargetTimelineID != "" {
		t.Fatalf("duplicate parent conversation=%+v", got)
	}

	conflictingReply := timelineRelationItem("conflicting-reply", domain.SourceX, "90002", "https://x.com/replier/status/90002", []domain.DirectContext{
		{Kind: "replies_to", Provenance: "observed_response", Target: domain.ContextObject{Kind: "post", ID: "12345", Availability: "reference_only"}},
		{Kind: "replies_to", Provenance: "observed_response", Target: domain.ContextObject{Kind: "post", ID: "54321", Availability: "reference_only"}},
	})
	conflicting := []domain.TimelineItem{conflictingReply}
	projectTimelineRelations(conflicting)
	if got := conflicting[0].Conversation; got == nil || got.Status != "ambiguous" || got.TargetTimelineID != "" {
		t.Fatalf("conflicting parent conversation=%+v", got)
	}
}

func TestProjectTimelineRelationsKeepsIdentitySourceScopedAndReportsMissingParent(t *testing.T) {
	linkedinItem := timelineRelationItem("linkedin", domain.SourceLinkedIn, "12345", "", nil)
	reply := timelineRelationItem("reply", domain.SourceX, "67890", "https://x.com/bob/status/67890", []domain.DirectContext{{
		Kind: "replies_to", Provenance: "observed_response", Target: domain.ContextObject{Kind: "post", ID: "12345", Availability: "reference_only"},
	}})
	items := []domain.TimelineItem{linkedinItem, reply}
	projectTimelineRelations(items)
	if got := items[1].Conversation; got == nil || got.Status != "not_in_view" || got.TargetTimelineID != "" {
		t.Fatalf("cross-source conversation=%+v", got)
	}
}

func TestLinkedInReplyContractStoresAndProjectsPostParent(t *testing.T) {
	const parentID = "linkedin:activity:1234567890123456789"
	const replyID = "linkedin:activity:1234567890123456790"
	const parentURL = "https://www.linkedin.com/feed/update/urn:li:activity:1234567890123456789/"
	const replyURL = "https://www.linkedin.com/feed/update/urn:li:activity:1234567890123456790/"
	observation := domain.Observation{
		Source: domain.SourceLinkedIn, PageURL: "https://www.linkedin.com/feed/", CapturedAt: domain.Now(),
		Snapshots: []domain.Snapshot{{Blocks: []domain.Block{
			{EvidenceKey: "linkedin:fixture-parent", PlatformID: parentID, Permalink: parentURL, Text: "Parent fixture"},
			{EvidenceKey: "linkedin:fixture-reply", PlatformID: replyID, Permalink: replyURL, Text: "Reply fixture",
				DirectContext: []domain.DirectContext{{Kind: "replies_to", Provenance: "observed_dom",
					Target: domain.ContextObject{Kind: "post", Permalink: parentURL, Availability: "reference_only"}}}},
		}}},
	}
	if path := os.Getenv("AKU_REPLY_FIXTURE_OBSERVATION"); path != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, &observation); err != nil {
			t.Fatal(err)
		}
		t.Log("Using the observation emitted by the LinkedIn adapter fixture")
	}
	for _, block := range observation.Snapshots[0].Blocks {
		if err := domain.ValidateDirectContext(observation.Source, block.DirectContext); err != nil {
			t.Fatal(err)
		}
	}
	ctx := context.Background()
	s := openTestStore(t)
	settings, err := s.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	settings.ActiveSources = []domain.Source{domain.SourceLinkedIn}
	session, err := s.CreateUpdateSession(ctx, "LinkedIn reply contract fixture", settings, domain.UpdatePolicy{
		Trigger: domain.UpdateTriggerUser, Delivery: domain.UpdateDeliveryVisible, BudgetAuthority: domain.BudgetAuthorityUser,
	})
	if err != nil {
		t.Fatal(err)
	}
	run := session.Runs[0]
	command, err := s.StartRun(ctx, run.ID, map[string]any{"source": "linkedin"})
	if err != nil {
		t.Fatal(err)
	}
	if claimed, err := s.ClaimCommand(ctx, run.ID, "fixture"); err != nil || claimed == nil {
		t.Fatalf("claim fixture: %v, %v", claimed, err)
	}
	if err := s.SaveObservation(ctx, command.ID, run.ID, observation); err != nil {
		t.Fatal(err)
	}
	stored, err := s.Observations(ctx, run.ID)
	if err != nil || len(stored) != 1 {
		t.Fatalf("stored observations: %d, %v", len(stored), err)
	}
	items := make([]domain.TimelineItem, 0, 2)
	for _, block := range stored[0].Snapshots[0].Blocks {
		items = append(items, domain.TimelineItem{ID: block.PlatformID, SessionID: session.ID, RunID: run.ID,
			Source: domain.SourceLinkedIn, EvidenceKey: block.EvidenceKey})
	}
	if err := s.CompleteRun(ctx, run, domain.ReasoningResult{}, nil, items, nil); err != nil {
		t.Fatal(err)
	}
	page, err := s.ListTimeline(ctx, 2, 0)
	if err != nil || len(page) != 2 || page[0].ID != parentID || page[1].ID != replyID {
		t.Fatalf("projected posts: %+v, %v", page, err)
	}
	if got := page[1].Conversation; got == nil || got.Kind != "replies_to" || got.Status != "resolved" ||
		got.TargetTimelineID != parentID || got.Provenance != "observed_dom" {
		t.Fatalf("projected reply: %+v", got)
	}
	if path := os.Getenv("AKU_REPLY_FIXTURE_TIMELINE"); path != "" {
		raw, err := json.Marshal(page)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestListTimelineProjectsRelationsAfterPageSelectionInBothModes(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	session, parentID := insertSemanticTimelineFixture(t, s, "new_event")
	runs, err := s.listRuns(ctx, session.ID)
	if err != nil || len(runs) == 0 {
		t.Fatalf("runs=%+v err=%v", runs, err)
	}
	parent := domain.Block{EvidenceKey: "x:semantic-fixture", PlatformID: "x:status:12345", Permalink: "https://x.com/alice/status/12345"}
	storeTimelineEvidenceSnapshot(t, s, parentID, parent)

	replyID := "timeline-relation-reply"
	replyEvidenceKey := "x:relation-reply"
	reply := domain.Block{
		EvidenceKey: replyEvidenceKey, PlatformID: "x:status:67890", Permalink: "https://x.com/bob/status/67890",
		DirectContext: []domain.DirectContext{{Kind: "replies_to", Provenance: "observed_response", Target: domain.ContextObject{Kind: "post", ID: "12345", Availability: "reference_only"}}},
	}
	itemRaw, _ := json.Marshal(domain.ReasonedItem{EvidenceKey: replyEvidenceKey, Source: domain.SourceX, WhatChanged: "Reply item"})
	evidenceRaw, _ := json.Marshal(reply)
	if _, err := s.db.ExecContext(ctx, `INSERT INTO timeline_items(id,session_id,run_id,source,evidence_key,rank,item_json,assessment_json,coverage_json,created_at,evidence_snapshot_json) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, replyID, session.ID, runs[0].ID, domain.SourceX, replyEvidenceKey, 1, string(itemRaw), "{}", "{}", domain.Now(), string(evidenceRaw)); err != nil {
		t.Fatal(err)
	}

	for _, mode := range []string{"show_all", "hide"} {
		settings, err := s.GetSettings(ctx)
		if err != nil {
			t.Fatal(err)
		}
		settings.SemanticEventMode = mode
		if err := s.SaveSettings(ctx, settings); err != nil {
			t.Fatal(err)
		}

		page, err := s.ListTimeline(ctx, 1, 1)
		if err != nil || len(page) != 1 || page[0].ID != replyID {
			t.Fatalf("mode=%s selected page=%+v err=%v", mode, page, err)
		}
		if got := page[0].Conversation; got == nil || got.Status != "not_in_view" || got.TargetTimelineID != "" {
			t.Fatalf("mode=%s out-of-page parent conversation=%+v", mode, got)
		}

		page, err = s.ListTimeline(ctx, 2, 0)
		if err != nil || len(page) != 2 || page[1].ID != replyID {
			t.Fatalf("mode=%s full selected page=%+v err=%v", mode, page, err)
		}
		if got := page[1].Conversation; got == nil || got.Status != "resolved" || got.TargetTimelineID != parentID {
			t.Fatalf("mode=%s loaded parent conversation=%+v", mode, got)
		}
	}
}

func timelineRelationItem(id string, source domain.Source, platformID, permalink string, relations []domain.DirectContext) domain.TimelineItem {
	return domain.TimelineItem{
		ID: id, Source: source,
		Evidence: &domain.Block{PlatformID: platformID, Permalink: permalink, DirectContext: relations},
	}
}

func storeTimelineEvidenceSnapshot(t *testing.T, s *Store, timelineID string, block domain.Block) {
	t.Helper()
	raw, err := json.Marshal(block)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE timeline_items SET evidence_snapshot_json=? WHERE id=?`, string(raw), timelineID); err != nil {
		t.Fatal(err)
	}
}
