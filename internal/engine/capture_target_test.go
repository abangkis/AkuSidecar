package engine

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestTargetCaptureSessionOfflineTimeline(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(map[bool]string{false: "distinct_primary_quote", true: "unavailable_primary"}[missing], func(t *testing.T) {
			f := newSessionAdmissionFixture(t)
			ctx := context.Background()
			target := &domain.CaptureTarget{Source: domain.SourceX, Permalink: "https://x.com/primary/status/123456789"}
			before, _ := f.e.Settings(ctx)
			session, err := f.e.StartVisibleTargetUpdate(ctx, "offline targeted quote qualification", target)
			if err != nil {
				t.Fatal(err)
			}
			if len(session.Runs) != 1 || session.Runs[0].Source != domain.SourceX || session.MaxItemsTotal != 1 || session.MaxItemsPerSource != 1 {
				t.Fatalf("scope %+v", session)
			}
			command, err := f.e.ClaimCommand(ctx, session.Runs[0].ID, "offline-target")
			if err != nil || command == nil {
				t.Fatalf("claim %v %v", command, err)
			}
			p := command.Payload
			if p["pageUrl"] != target.Permalink || p["targetUrl"] != target.Permalink || p["scrolls"] != float64(0) || p["maxAcquisitionRounds"] != float64(1) || p["sourceFreshnessPolicy"] != "preserve_target" || p["sameTabMutationAllowed"] != false {
				t.Fatalf("payload %+v", p)
			}
			primary, quote := "2026-11-14T15:12:49Z", "2026-11-14T14:12:49Z"
			block := domain.Block{Author: "Primary", Text: "A detailed engineering update about browser automation and software development.", Permalink: target.Permalink, PlatformID: "123456789", PublishedAt: &primary, Presentation: map[string]any{"timestampSource": "native_datetime"}, QuotedPost: map[string]any{"permalink": "https://x.com/quote/status/987654321", "publishedAt": quote, "text": "Quoted engineering update"}}
			if missing {
				block.PublishedAt = nil
				block.Presentation = map[string]any{"timestampSource": "unavailable", "timestampEstimated": false, "timestampPrecision": "unknown"}
			}
			neighbor := domain.Block{Author: "Neighbor", Text: "Unrelated post", Permalink: "https://x.com/other/status/111111111"}
			observation := domain.Observation{Source: domain.SourceX, PageURL: target.Permalink, CapturedAt: domain.Now(), Snapshots: []domain.Snapshot{{Blocks: []domain.Block{neighbor, block}}}, Coverage: map[string]any{"quality": "complete"}}
			if _, err = f.e.AcceptObservation(ctx, command.ID, command.RunID, observation); err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(5 * time.Second)
			for {
				s, e := f.state.GetSession(ctx, session.ID)
				if e != nil {
					t.Fatal(e)
				}
				if s.Status == "completed" {
					break
				}
				if s.Status == "failed" || time.Now().After(deadline) {
					t.Fatalf("session %+v", s)
				}
				time.Sleep(10 * time.Millisecond)
			}
			var commands int
			if err = f.db.QueryRow(`SELECT COUNT(*) FROM bridge_commands WHERE run_id=?`, command.RunID).Scan(&commands); err != nil || commands != 1 {
				t.Fatalf("follow-up commands %d %v", commands, err)
			}
			observations, err := f.state.Observations(ctx, command.RunID)
			if err != nil || len(observations) != 1 || len(observations[0].Snapshots[0].Blocks) != 1 {
				t.Fatalf("observation scope %+v %v", observations, err)
			}
			timeline, err := f.e.Timeline(ctx, 10, 0)
			if err != nil || len(timeline) != 1 {
				t.Fatalf("timeline %+v %v", timeline, err)
			}
			evidence := timeline[0].Evidence
			if evidence == nil || evidence.Permalink != target.Permalink || evidence.QuotedPost["publishedAt"] != quote {
				t.Fatalf("ownership %+v", evidence)
			}
			if missing {
				if evidence.PublishedAt != nil || timeline[0].Item.PublishedAt != nil || evidence.Presentation["timestampSource"] != "unavailable" {
					t.Fatalf("fabricated timestamp %+v", evidence)
				}
			} else if evidence.PublishedAt == nil || *evidence.PublishedAt != primary {
				t.Fatalf("primary timestamp %+v", evidence)
			}
			after, _ := f.e.Settings(ctx)
			if !reflect.DeepEqual(before, after) {
				t.Fatal("saved settings changed")
			}
		})
	}
}

func TestTargetCaptureRejectedObservationDoesNotPersist(t *testing.T) {
	f := newSessionAdmissionFixture(t)
	ctx := context.Background()
	target := &domain.CaptureTarget{Source: domain.SourceX, Permalink: "https://x.com/primary/status/123456789"}
	session, err := f.e.StartVisibleTargetUpdate(ctx, "offline target guard", target)
	if err != nil {
		t.Fatal(err)
	}
	command, err := f.e.ClaimCommand(ctx, session.Runs[0].ID, "offline-target")
	if err != nil || command == nil {
		t.Fatalf("claim %+v %v", command, err)
	}
	value := domain.Observation{Source: domain.SourceX, PageURL: target.Permalink, CapturedAt: domain.Now(), Snapshots: []domain.Snapshot{{Blocks: []domain.Block{{Author: "Neighbor", Text: "Neighbor quoting target", Permalink: "https://x.com/neighbor/status/999999999", QuotedPost: map[string]any{"permalink": target.Permalink}}}}}}
	if _, err := f.e.AcceptObservation(ctx, command.ID, command.RunID, value); err == nil {
		t.Fatal("quote satisfied primary target")
	}
	observations, err := f.state.Observations(ctx, command.RunID)
	if err != nil || len(observations) != 0 {
		t.Fatalf("rejected evidence persisted: %+v %v", observations, err)
	}
}

func TestTargetCaptureRequiresEnabledGrantedSource(t *testing.T) {
	f := newSessionAdmissionFixture(t)
	ctx := context.Background()
	settings, err := f.e.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	settings.ActiveSources = []domain.Source{domain.SourceLinkedIn}
	if _, err := f.e.SaveSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	if _, err := f.e.StartVisibleTargetUpdate(ctx, "offline disabled target", &domain.CaptureTarget{Source: domain.SourceX, Permalink: "https://x.com/primary/status/123456789"}); err == nil {
		t.Fatal("target bypassed enabled sources")
	}
	if f.countSessions(t) != 0 {
		t.Fatal("denied target persisted a session")
	}
}

func TestTargetCaptureRejectsWrongPrimary(t *testing.T) {
	target := domain.CaptureTarget{Source: domain.SourceX, Permalink: "https://x.com/primary/status/123456789"}
	for _, page := range []string{target.Permalink, "https://x.com/home"} {
		value := domain.Observation{Source: domain.SourceX, PageURL: page, Snapshots: []domain.Snapshot{{Blocks: []domain.Block{{Permalink: "https://x.com/neighbor/status/999999999", QuotedPost: map[string]any{"permalink": target.Permalink}}}}}}
		if scopeTargetObservation(&value, target) == nil {
			t.Fatal("neighbor or quote satisfied primary target")
		}
	}
}

func TestTargetCaptureHeadlessRouting(t *testing.T) {
	e, _, _, _ := newHybridCollectionEngine(t, nil)
	ctx := context.Background()
	target := &domain.CaptureTarget{Source: domain.SourceX, Permalink: "https://x.com/primary/status/123456789"}
	session, err := e.StartVisibleTargetUpdate(ctx, "offline headless target", target)
	if err != nil {
		t.Fatal(err)
	}
	command, err := e.claimCommandForDriver(ctx, session.Runs[0].ID, "offline-headless", "headless")
	if err != nil || command == nil {
		t.Fatalf("claim %+v %v", command, err)
	}
	if command.Payload["pageUrl"] != target.Permalink || command.Payload["captureCollector"].(map[string]any)["backend"] != "headless" {
		t.Fatalf("payload %+v", command.Payload)
	}
	if _, err = e.ownedCapturePayload(session.Runs[0], session.ID, domain.Settings{}, 2, nil, ""); err == nil {
		t.Fatal("target allowed second round")
	}
	if err = e.CancelSession(ctx, session.ID); err != nil {
		t.Fatal(err)
	}
}
