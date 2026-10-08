import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import vm from "node:vm";
import { videoPlaybackMissing } from "../internal/httpapi/web/video-recapture-policy.js";

const app = readFileSync(new URL("../internal/httpapi/web/app.js", import.meta.url), "utf8");
function fixture() {
  const calls = []; const buttons = [];
  const context = vm.createContext({ URL, videoPlaybackMissing,
    state: { session: null, mediaRecaptureActive: false, bootstrap: { collectionRuntime: { state: "ready" } } },
    sourceDescriptor: () => ({ playbackRecoveryCapability: "native_post_recapture" }),
    safeSourceUrl: (url) => url?.startsWith("https://x.com/") ? url : null,
    collectionModeState: () => ({ canCollect: true }),
    recaptureMedia: (...args) => calls.push(args),
    document: { createElement: () => { const button = { addEventListener: (name, fn) => { button[name] = fn; } }; buttons.push(button); return button; } },
  });
  for (const [start, end] of [["function safePlaybackUrl(", "async function api("],
    ["function mediaRecaptureReasonForEntry(", "function buildForegroundRecaptureOffer("]]) {
    vm.runInContext(app.slice(app.indexOf(start), app.indexOf(end)), context);
  }
  return { context, calls, buttons };
}

test("poster-only X exposes explicit unresolved_video recapture without a passive capture", () => {
  const f = fixture();
  const entry = { id: "espn", source: "x", item: { sourceUrl: "https://x.com/ESPNAsia/status/2108207027298435417" },
    evidence: { contentKind: "video", media: [{ kind: "video_poster", sourceKind: "blob" }], mediaRecovery: { expected: ["image", "video"], outcome: "unresolved" } } };
  assert.equal(f.context.mediaRecaptureReasonForEntry(entry), "unresolved_video");
  const button = f.context.buildMediaRecaptureButton(entry);
  assert.equal(button.textContent, "Recapture video");
  assert.equal(f.calls.length, 0);
  button.click();
  assert.equal(f.calls.length, 1);
  assert.equal(f.calls[0][3], "unresolved_video");
  assert.equal(f.calls[0][2], "background");
});

test("source-specific playable URLs suppress unresolved-video recovery; images and foreign source links do not qualify", () => {
  const f = fixture();
  for (const [source, url] of [["x", "https://video.twimg.com/ext_tw_video/12345/pu/vid/clip.mp4"],
    ["instagram", "https://s.cdninstagram.com/clip.mp4"], ["facebook", "https://s.fbcdn.net/clip.mp4"],
    ["linkedin", "https://dms.licdn.com/playlist/vid/v2/example/mp4-720p-30fp-crf28/example/0/1"]]) {
    const evidence = { mediaRecovery: { expected: ["video"] }, media: [{ kind: "video", playbackMode: "inline", playbackUrl: url }] };
    assert.equal(videoPlaybackMissing(evidence, source, f.context.safePlaybackUrl), false);
    evidence.media[0].playbackUrl = "https://evil.test/clip.mp4";
    assert.equal(videoPlaybackMissing(evidence, source, f.context.safePlaybackUrl), true);
  }
  assert.equal(videoPlaybackMissing({ media: [{ kind: "image" }] }, "x", f.context.safePlaybackUrl), false);
  assert.equal(f.context.safePlaybackUrl("https://video.twimg.com/ext_tw_video/12345/master.m3u8", "x"), null);
  assert.equal(f.context.mediaRecaptureReasonForEntry({ source: "x", item: { sourceUrl: "https://evil.test/status/12345" }, evidence: { contentKind: "video" } }), "missing_media");
});
