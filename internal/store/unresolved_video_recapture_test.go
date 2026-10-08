package store

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func unresolvedVideoFixture(t *testing.T, state *Store) (string, domain.Block) {
	t.Helper()
	id, key := insertUnavailableMediaFixture(t, state)
	block := domain.Block{EvidenceKey: key, PlatformID: key, Author: "Example", Text: "Video fixture", Permalink: "https://x.com/example/status/12345", ContentKind: "video",
		Media:         []map[string]any{{"kind": "video_poster", "url": "https://pbs.twimg.com/ext_tw_video_thumb/12345/pu/img/poster.jpg", "sourceKind": "blob"}},
		MediaRecovery: map[string]any{"expected": []string{"video", "image"}, "outcome": "unresolved", "limitation": "video_stream_not_resolved"}}
	obs := domain.Observation{Source: domain.SourceX, Snapshots: []domain.Snapshot{{Index: 0, Blocks: []domain.Block{block}}}}
	raw, _ := json.Marshal(obs)
	if _, err := state.db.Exec(`UPDATE observations SET observation_json=? WHERE id='observation-recapture-fixture'`, string(raw)); err != nil {
		t.Fatal(err)
	}
	return id, block
}

func TestUnresolvedVideoRecaptureRequiresPlaybackAndRetainsPoster(t *testing.T) {
	for _, mode := range []string{"poster_only", "empty", "image_only", "poster_signal_only", "safe_mp4", "unsafe_mp4", "foreign_post"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			state := openTestStore(t)
			id, block := unresolvedVideoFixture(t, state)
			if mode == "poster_signal_only" {
				block.ContentKind = ""
				block.MediaRecovery = nil
				raw, _ := json.Marshal(domain.Observation{Source: domain.SourceX, Snapshots: []domain.Snapshot{{Blocks: []domain.Block{block}}}})
				if _, err := state.db.Exec(`UPDATE observations SET observation_json=? WHERE id='observation-recapture-fixture'`, string(raw)); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := state.CreateMediaRecapture(ctx, id, domain.MediaRecaptureBackground); err == nil {
				t.Fatal("legacy missing-media reason must reject a poster")
			}
			if _, err := state.CreateMediaRecaptureForReason(ctx, id, domain.MediaRecaptureForeground, domain.MediaRecaptureUnresolvedVideo); err == nil {
				t.Fatal("foreground requires a failed background attempt")
			}
			job, err := state.CreateMediaRecaptureForReason(ctx, id, domain.MediaRecaptureBackground, domain.MediaRecaptureUnresolvedVideo)
			if err != nil {
				t.Fatal(err)
			}
			if job.Payload["reason"] != domain.MediaRecaptureUnresolvedVideo || job.Payload["foregroundAuthorized"] != false {
				t.Fatal(job.Payload)
			}
			if _, err = state.ClaimMediaRecapture(ctx, job.ID, "test"); err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "empty":
				block.Media = nil
				block.ContentKind = ""
				block.MediaRecovery = nil
			case "image_only", "poster_signal_only":
				block.Media = []map[string]any{{"kind": "image", "url": "https://pbs.twimg.com/media/image.jpg"}}
			case "safe_mp4", "unsafe_mp4":
				host := "video.twimg.com"
				if mode == "unsafe_mp4" {
					host = "video.twimg.com.evil.test"
				}
				block.Media = []map[string]any{{"kind": "video", "url": "https://pbs.twimg.com/ext_tw_video_thumb/12345/pu/img/poster.jpg", "playbackMode": "inline", "playbackUrl": "https://" + host + "/ext_tw_video/12345/pu/vid/clip.mp4"}}
			case "foreign_post":
				block.Permalink = "https://x.com/other/status/99999"
			}
			job, err = state.CompleteMediaRecapture(ctx, job.ID, domain.Observation{Source: domain.SourceX, Snapshots: []domain.Snapshot{{Blocks: []domain.Block{block}}}})
			if mode == "foreign_post" {
				if err == nil {
					t.Fatal("same evidence key must not authorize another native post")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			want := "unavailable"
			if mode == "safe_mp4" {
				want = "recovered"
			}
			if job.Outcome != want {
				t.Fatalf("outcome=%s want=%s", job.Outcome, want)
			}
			items, err := state.ListTimeline(ctx, 24, 0)
			if err != nil || len(items) != 1 {
				t.Fatalf("items=%d err=%v", len(items), err)
			}
			evidence := items[0].Evidence
			if mode == "empty" && (len(evidence.Media) != 1 || evidence.Media[0]["kind"] != "video_poster") {
				t.Fatal("failed capture discarded saved poster")
			}
			if want == "unavailable" {
				if !unresolvedVideo(*evidence, domain.SourceX) {
					t.Fatal("poster-only result incorrectly satisfied video")
				}
				if _, err = state.CreateMediaRecaptureForReason(ctx, id, domain.MediaRecaptureForeground, domain.MediaRecaptureMissingMedia); err == nil {
					t.Fatal("foreground must preserve the reason gate")
				}
				if _, err = state.CreateMediaRecaptureForReason(ctx, id, domain.MediaRecaptureForeground, domain.MediaRecaptureUnresolvedVideo); err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err = state.CreateMediaRecaptureForReason(ctx, id, domain.MediaRecaptureBackground, domain.MediaRecaptureUnresolvedVideo); err == nil {
					t.Fatal("playable video must not qualify as unresolved")
				}
			}
		})
	}
}

func TestUnresolvedVideoPolicyAcrossSources(t *testing.T) {
	for source, url := range map[domain.Source]string{domain.SourceX: "https://video.twimg.com/ext_tw_video/12345/pu/vid/clip.mp4", domain.SourceInstagram: "https://s.cdninstagram.com/clip.mp4", domain.SourceFacebook: "https://s.fbcdn.net/clip.mp4", domain.SourceLinkedIn: "https://dms.licdn.com/playlist/vid/v2/example/mp4-720p-30fp-crf28/example/0/1"} {
		block := domain.Block{Media: []map[string]any{{"kind": "image", "url": "https://example.test/image.jpg"}}}
		if unresolvedVideo(block, source) {
			t.Fatalf("image-only post admitted for %s", source)
		}
		block.MediaRecovery = map[string]any{"expected": []any{"video", "image"}}
		if !unresolvedVideo(block, source) {
			t.Fatalf("expected video without playback rejected for %s", source)
		}
		block.Media = append(block.Media, map[string]any{"kind": "video", "playbackMode": "inline", "playbackUrl": url})
		if unresolvedVideo(block, source) {
			t.Fatalf("playable video admitted for %s", source)
		}
	}
}

func TestXPlaybackErrorRecaptureRejectsFailedURL(t *testing.T) {
	for _, changed := range []bool{false, true} {
		t.Run(map[bool]string{false: "same_url", true: "new_url"}[changed], func(t *testing.T) {
			ctx := context.Background()
			state := openTestStore(t)
			id, block := unresolvedVideoFixture(t, state)
			failed := "https://video.twimg.com/ext_tw_video/12345/pu/vid/old.mp4"
			block.Media[0]["kind"] = "video"
			block.Media[0]["playbackMode"] = "inline"
			block.Media[0]["playbackUrl"] = failed
			raw, _ := json.Marshal(domain.Observation{Source: domain.SourceX, Snapshots: []domain.Snapshot{{Blocks: []domain.Block{block}}}})
			if _, err := state.db.Exec(`UPDATE observations SET observation_json=? WHERE id='observation-recapture-fixture'`, string(raw)); err != nil {
				t.Fatal(err)
			}
			job, err := state.CreateMediaRecaptureForReason(ctx, id, domain.MediaRecaptureBackground, domain.MediaRecapturePlaybackError)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := state.ClaimMediaRecapture(ctx, job.ID, "fixture"); err != nil {
				t.Fatal(err)
			}
			if changed {
				block.Media[0]["playbackUrl"] = "https://video.twimg.com/ext_tw_video/12345/pu/vid/new.mp4"
			}
			job, err = state.CompleteMediaRecapture(ctx, job.ID, domain.Observation{Source: domain.SourceX, Snapshots: []domain.Snapshot{{Blocks: []domain.Block{block}}}})
			if err != nil {
				t.Fatal(err)
			}
			want := "unavailable"
			if changed {
				want = "recovered"
			}
			if job.Outcome != want {
				t.Fatalf("outcome=%s want=%s", job.Outcome, want)
			}
		})
	}
}
