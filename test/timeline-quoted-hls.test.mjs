import assert from "node:assert/strict";
import test from "node:test";
import { createTimelineContent } from "../internal/httpapi/web/timeline-content.js";
import { createTimelineMedia } from "../internal/httpapi/web/timeline-media.js";

class Element extends EventTarget {
  children = [];
  className = "";
  style = { setProperty() {} };
  constructor(tag) { super(); this.tagName = tag.toUpperCase(); }
  append(...children) { this.children.push(...children); }
  setAttribute(name, value) { this[name] = value; }
}
const descendants = element => [element, ...element.children.flatMap(descendants)];

test("a hydrated OpenAI quote and primary video use the same generic Timeline admission and poster component", t => {
  const previous = globalThis.document;
  globalThis.document = { createElement: tag => new Element(tag) };
  t.after(() => { globalThis.document = previous; });
  t.mock.method(globalThis, "fetch", () => assert.fail("Rendering must not fetch a video playlist"));
  const quotedMedia = {
    kind: "video", playbackMode: "inline",
    playbackUrl: "https://video.twimg.com/amplify_video/2120214011451170817/pl/QiogQZ8zkTrcsBbj.m3u8?tag=29&v=cfc",
    // Synthetic poster suffix; asset identity is the reported OpenAI video ID.
    posterUrl: "https://pbs.twimg.com/amplify_video_thumb/2120214011451170817/img/fixture.jpg",
    width: 520, height: 293,
  };
  const renderer = createTimelineMedia({ safeMediaUrl: value => value, safePlaybackUrl: () => null, sourceLabel: () => "X", configureNativePostLink: (node, href) => { node.href = href; } });
  const calls = [];
  const buildMedia = (...args) => { calls.push(args); return renderer.buildMedia(...args); };
  const content = createTimelineContent({ buildMedia, safeSourceUrl: value => value, safeMediaUrl: () => null, sourceLabel: () => "X", sourceDescriptor: () => ({}), initials: () => "O", formatDate: () => "" });
  const permalink = "https://x.com/OpenAI/status/2120216592395174382";
  const quote = content.buildQuotedPost({ author: "OpenAI", permalink, media: [quotedMedia] }, "x");
  assert.equal(quote.className, "source-quote-card");
  assert.deepEqual(calls[0][0], [quotedMedia]);
  assert.equal(calls[0][3], permalink);
  const primary = buildMedia([quotedMedia], "x", "video", permalink);
  const quoteButton = descendants(quote).find(node => node.className.includes("is-video-poster"));
  const primaryButton = descendants(primary).find(node => node.className.includes("is-video-poster"));
  assert.equal(quoteButton.tagName, "BUTTON");
  assert.equal(quoteButton["aria-label"], primaryButton["aria-label"]);
  assert.equal(quoteButton["aria-label"], "Play X video in AkuBrowser");
  const nativeQuote = content.buildQuotedPost({ author: "OpenAI", permalink, media: [{ ...quotedMedia, playbackMode: "native", playbackUrl: null }] }, "x");
  const nativePoster = descendants(nativeQuote).find(node => node.className.includes("is-video-poster"));
  assert.equal(nativePoster.tagName, "A");
  assert.equal(nativePoster.href, permalink);
});
