package store

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

const hydratedQuotePlayback = "https://video.twimg.com/amplify_video/2120214011451170817/pl/QiogQZ8zkTrcsBbj.m3u8?tag=29&v=cfc"

func quotePlaybackFixture(t *testing.T, inline bool) domain.Block {
	t.Helper()
	media := map[string]any{
		"kind": "video", "playbackMode": "native", "playbackUrl": nil,
		"url":       "https://pbs.twimg.com/amplify_video_thumb/2120214011451170817/img/poster?format=webp&name=medium",
		"posterUrl": "https://pbs.twimg.com/amplify_video_thumb/2120214011451170817/img/poster?format=webp&name=medium",
		"width":     518, "height": 291,
	}
	if inline {
		media["playbackMode"], media["playbackUrl"] = "inline", hydratedQuotePlayback
	}
	block := domain.Block{
		EvidenceKey: "x:quote-playback", PlatformID: "x:status:2120267268517249036",
		Permalink: "https://x.com/thsottiaux/status/2120267268517249036", Author: "Tibo",
		Text: "How many of you have a small business?", ContentKind: "post", RelationshipType: "quote",
		ParentPermalink: "https://x.com/OpenAI/status/2120216592395174382",
		QuotedPost: map[string]any{
			"permalink": "https://x.com/OpenAI/status/2120216592395174382", "author": "OpenAI",
			"text": "Now in beta: Businesses in ChatGPT.", "media": []map[string]any{media},
		},
		Media: []map[string]any{{"kind": "image", "url": "https://pbs.twimg.com/media/primary.jpg"}},
	}
	// Match the production observation decoder's []any representation.
	raw, err := json.Marshal(block)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &block); err != nil {
		t.Fatal(err)
	}
	return block
}

func TestTimelineQuotePlaybackHydration(t *testing.T) {
	previous, current := quotePlaybackFixture(t, false), quotePlaybackFixture(t, true)
	before, _ := json.Marshal(previous)
	current.QuotedPost["avatarUrl"] = "https://pbs.twimg.com/profile_images/later.jpg"
	result := enrichTimelineQuotePlayback(domain.SourceX, previous, current)
	if got := timelineQuoteMedia(result.QuotedPost["media"])[0]["playbackUrl"]; got != hydratedQuotePlayback {
		t.Fatal("later quote playback was not retained", got)
	}
	if result.Text != previous.Text || !reflect.DeepEqual(result.Media, previous.Media) || result.QuotedPost["avatarUrl"] != nil {
		t.Fatal("playback hydration replaced unrelated first-observation content")
	}
	after, _ := json.Marshal(previous)
	if string(before) != string(after) {
		t.Fatal("playback projection mutated stored observation maps")
	}
	result = enrichTimelineQuotePlayback(domain.SourceX, result, quotePlaybackFixture(t, false))
	if got := timelineQuoteMedia(result.QuotedPost["media"])[0]["playbackUrl"]; got != hydratedQuotePlayback {
		t.Fatal("a later poster-only snapshot downgraded playback")
	}
}

func TestTimelineQuotePlaybackRejectsConflictingEvidence(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*domain.Block)
	}{
		{"primary identity", func(b *domain.Block) { b.Permalink = "https://x.com/other/status/999" }},
		{"platform identity", func(b *domain.Block) { b.PlatformID = "x:status:999" }},
		{"different valid platform identity", func(b *domain.Block) { b.PlatformID = "x:status:2120267268517249037" }},
		{"primary text", func(b *domain.Block) { b.Text = "Changed primary text" }},
		{"quote identity", func(b *domain.Block) { b.QuotedPost["permalink"] = "https://x.com/other/status/999" }},
		{"quote parent", func(b *domain.Block) { b.ParentPermalink = "https://x.com/other/status/999" }},
		{"missing parent", func(b *domain.Block) { b.ParentPermalink = "" }},
		{"quote author", func(b *domain.Block) { b.QuotedPost["author"] = "Other" }},
		{"quote text", func(b *domain.Block) { b.QuotedPost["text"] = "Different quoted content" }},
		{"quote conflict", func(b *domain.Block) { b.QuotedPost["identityStatus"] = "conflicting_identity" }},
		{"foreign host", func(b *domain.Block) {
			timelineQuoteMedia(b.QuotedPost["media"])[0]["playbackUrl"] = "https://example.com/amplify_video/2120214011451170817/pl/master.m3u8"
		}},
		{"foreign playback asset", func(b *domain.Block) {
			timelineQuoteMedia(b.QuotedPost["media"])[0]["playbackUrl"] = "https://video.twimg.com/amplify_video/999/pl/master.m3u8"
		}},
		{"foreign poster asset", func(b *domain.Block) {
			timelineQuoteMedia(b.QuotedPost["media"])[0]["posterUrl"] = "https://pbs.twimg.com/amplify_video_thumb/999/img/poster"
		}},
		{"primary media only", func(b *domain.Block) {
			b.Media = timelineQuoteMedia(b.QuotedPost["media"])
			b.QuotedPost["media"] = []any{}
		}},
		{"malformed media", func(b *domain.Block) { b.QuotedPost["media"] = []any{"invalid"} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			previous, current := quotePlaybackFixture(t, false), quotePlaybackFixture(t, true)
			tc.change(&current)
			result := enrichTimelineQuotePlayback(domain.SourceX, previous, current)
			if !reflect.DeepEqual(result, previous) {
				t.Fatal("conflicting evidence hydrated the quote", result)
			}
		})
	}
	previous, current := quotePlaybackFixture(t, false), quotePlaybackFixture(t, true)
	if got := enrichTimelineQuotePlayback(domain.SourceLinkedIn, previous, current); !reflect.DeepEqual(got, previous) {
		t.Fatal("X quote hydration changed another source")
	}
}

func TestTimelineReadModelHydratesQuotePlaybackAndPreservesOverride(t *testing.T) {
	for _, override := range []bool{false, true} {
		t.Run(map[bool]string{false: "saved snapshots", true: "recapture override"}[override], func(t *testing.T) {
			s, f := timelineReadFixture(t)
			first, later := quotePlaybackFixture(t, false), quotePlaybackFixture(t, true)
			first.EvidenceKey, later.EvidenceKey = f.Item.EvidenceKey, f.Item.EvidenceKey
			observation := domain.Observation{Source: domain.SourceX, Snapshots: []domain.Snapshot{
				{Blocks: []domain.Block{first}}, {Blocks: []domain.Block{later}}, {Blocks: []domain.Block{first}},
			}}
			raw, err := json.Marshal(observation)
			if err != nil {
				t.Fatal(err)
			}
			timelineReadExec(t, s, `UPDATE observations SET observation_json=? WHERE run_id=?`, string(raw), f.Run.ID)
			if override {
				timelineReadExec(t, s, `INSERT INTO media_recaptures(id,timeline_id,source,target_url,evidence_key,status,payload_json,created_at) VALUES('quote-recapture',?,'x','https://x.com/thsottiaux/status/2120267268517249036',?,'completed','{}','fixture')`, f.Item.ID, f.Item.EvidenceKey)
				timelineReadExec(t, s, `INSERT INTO timeline_evidence_overrides(timeline_id,recapture_id,evidence_json,updated_at) VALUES(?,'quote-recapture',?,'fixture')`, f.Item.ID, `{"text":"explicit override"}`)
			}
			item, err := s.TimelineItem(context.Background(), f.Item.ID)
			if err != nil || item.Evidence == nil {
				t.Fatal("timeline projection failed", item.Evidence, err)
			}
			if override {
				if item.Evidence.Text != "explicit override" {
					t.Fatal("recapture override lost precedence")
				}
			} else if got := timelineQuoteMedia(item.Evidence.QuotedPost["media"])[0]["playbackUrl"]; got != hydratedQuotePlayback {
				t.Fatal("public Timeline read discarded hydrated quote playback", got)
			}
			var saved string
			if err := s.db.QueryRow(`SELECT observation_json FROM observations WHERE run_id=? LIMIT 1`, f.Run.ID).Scan(&saved); err != nil || saved != string(raw) {
				t.Fatal("Timeline read changed persisted capture", err)
			}
		})
	}
}
