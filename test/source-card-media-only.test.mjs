import { readFrontendAppSource } from "./fixtures/frontend-source.mjs";
import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import vm from "node:vm";
import { boundedTimelineMedia, shouldUseTimelineCarousel } from "../internal/httpapi/web/timeline-media-carousel.js";
import { reserveMediaDimensions } from "../internal/httpapi/web/timeline-media-layout.js";
import { timelineHlsPlayback } from "../internal/httpapi/web/timeline-hls-policies.js";
import { primaryMediaMissing, videoPlaybackMissing } from "../internal/httpapi/web/video-recapture-policy.js";

class Element {
  children = []; className = ""; value = "";
  classList = { toggle() {} }; style = { setProperty() {} };
  constructor(tag) { this.tagName = tag.toUpperCase(); }
  append(...children) { this.children.push(...children); }
  set textContent(value) { this.value = String(value); this.children = []; }
  get textContent() { return this.value + this.children.map(child => child.textContent).join(""); }
  setAttribute(name, value) { this[name] = value; }
  addEventListener() {}
}

const app = readFrontendAppSource();
const slice = (from, to) => app.slice(app.indexOf(`function ${from}(`), app.indexOf(`function ${to}(`));
function renderer() {
  const context = vm.createContext({
    URL, document: { createElement: tag => new Element(tag) },
    SOURCE_TEXT_COLLAPSE_CHARACTERS: 420, SOURCE_TEXT_COLLAPSE_LINES: 6,
    QUOTE_TEXT_COLLAPSE_CHARACTERS: 320, QUOTE_TEXT_COLLAPSE_LINES: 4,
    sourceDescriptor: source => source === "linkedin"
      ? { nativeHosts: ["www.linkedin.com"], nativePathTokens: ["/feed/update/", "/posts/"], playbackRecoveryCapability: "native_post_recapture" }
      : { nativeHosts: ["x.com"], nativePathTokens: ["/status/"] },
    sourceIdentity: author => ({ displayName: author }), sourceLabel: () => "X",
    buildAvatar: () => new Element("span"), postHeaderContext: () => "", formatDate: () => "",
    applyPostFreshness() {}, postHeaderContexts: new WeakMap(),
    state: { expandedTimelineText: new Set(), foregroundRecaptureOffers: new Map() },
    buildEngagement: () => null, primaryMediaMissing, videoPlaybackMissing,
    collectionModeState: () => ({ canCollect: true }),
    boundedTimelineMedia, shouldUseTimelineCarousel, reserveMediaDimensions, timelineHlsPlayback,
    safePlaybackUrl: () => null, openMedia() {},
    configureNativePostLink: (link, href) => { link.href = href; },
  });
  vm.runInContext([
    slice("buildSourceCard", "applyPostFreshness"),
    slice("buildAttachments", "buildEngagement"),
    slice("buildQuotedPost", "buildMedia"),
    slice("buildMedia", "buildTimelineMediaCarousel"),
    slice("buildImageMediaControl", "buildVideoMedia"),
    slice("buildVideoMedia", "activateInlineVideo"),
    slice("safeMediaUrl", "safePlaybackUrl"),
    slice("safeSourceUrl", "buildForegroundRecaptureOffer"),
  ].join("\n"), context);
  return entry => context.buildSourceCard(entry);
}
const render = renderer();
const nodes = (element, predicate) => [element, ...element.children.flatMap(child => nodes(child, predicate))].filter(predicate);
const byClass = (element, className) => nodes(element, node => node.className.split(" ").includes(className));
const photo = { kind: "image", url: "https://pbs.twimg.com/media/synthetic.jpg", alt: "Synthetic image", width: 1200, height: 800 };
const entry = evidence => ({ id: "synthetic-post", source: "x", item: { whatChanged: "AI summary must stay in brief view" }, evidence: { author: "Synthetic author", ...evidence } });

test("image-only source cards show the captured image without a summary or empty text container", () => {
  for (const evidence of [{ text: "", media: [photo] }, { text: "  \n", media: [photo] }, { media: [photo] }]) {
    const card = render(entry(evidence));
    assert.equal(byClass(card, "source-layout-content").length, 0);
    assert.equal(byClass(card, "expandable-text").length, 0);
    assert.equal(byClass(card, "content-expander").length, 0);
    assert.doesNotMatch(card.textContent, /AI summary/);
    const images = nodes(card, node => node.tagName === "IMG");
    assert.equal(images.length, 1);
    assert.equal(images[0].src, photo.url);
    assert.equal(images[0].width, 1200);
  }
});

test("image captions preserve authored names, numbers and paragraph breaks", () => {
  const caption = "Tibo @thsottiaux\n\nImage 81 27 811 37K";
  const card = render(entry({ text: caption, media: [photo] }));
  assert.deepEqual(byClass(card, "expandable-text-copy").map(node => node.textContent), [caption]);
  assert.equal(nodes(card, node => node.tagName === "IMG").length, 1);
  assert.doesNotMatch(card.textContent, /AI summary/);
});

test("a captured LinkedIn image does not show a missing-media warning after cancelled acquisition", () => {
  const card = render({ ...entry({
    media: [{ ...photo, url: "https://media.licdn.com/synthetic.jpg" }],
    mediaRecovery: { outcome: "unavailable", trace: ["acquisition_cancelled"], expectedKinds: [] },
  }), source: "linkedin" });
  assert.equal(nodes(card, node => node.tagName === "IMG").length, 1);
  assert.equal(byClass(card, "source-layout-media-unavailable").length, 0);
  assert.equal(byClass(card, "recapture-button").length, 0);
});

test("a LinkedIn link preview stays visible without offering recapture for its thumbnail", () => {
  for (const trace of [["acquisition_cancelled"], ["owner_unverified"], ["deadline_exceeded"]]) {
    const card = render({ ...entry({
      permalink: "https://www.linkedin.com/feed/update/urn:li:share:1234567890/",
      attachments: [{ kind: "link_preview", title: "Oracle WebLogic", url: "https://blogs.oracle.com/fixture",
        imageUrl: "https://media.licdn.com/fixture-thumbnail.jpg" }],
      mediaRecovery: { outcome: "unavailable", trace, expectedKinds: [] },
    }), source: "linkedin" });
    assert.equal(byClass(card, "source-layout-attachment")[0].href, "https://blogs.oracle.com/fixture");
    assert.equal(nodes(card, node => node.tagName === "IMG")[0].src, "https://media.licdn.com/fixture-thumbnail.jpg");
    assert.equal(byClass(card, "source-layout-media-unavailable").length, 0);
    assert.equal(byClass(card, "recapture-button").length, 0);
  }
});

test("missing LinkedIn media offers recapture only with a native post link", () => {
  for (const mediaRecovery of [
    { outcome: "unavailable", expectedKinds: ["image"] },
    { outcome: "observed_dom_only", status: "missing_expected_url", expected: ["image"] },
  ]) {
    const post = { ...entry({ mediaRecovery }), source: "linkedin" };
    const withoutLink = render(post);
    assert.match(byClass(withoutLink, "source-layout-media-unavailable")[0].textContent, /native post link was not captured/);
    assert.equal(byClass(withoutLink, "recapture-button").length, 0);

    const withLink = render({ ...post, evidence: { ...post.evidence,
      permalink: "https://www.linkedin.com/feed/update/urn:li:activity:123/" } });
    assert.equal(byClass(withLink, "recapture-button")[0].textContent, "Recapture");
    assert.doesNotMatch(withLink.textContent, /native post link was not captured/);
  }
});

test("a LinkedIn video poster still offers video recapture with a native post link", () => {
  const card = render({ ...entry({
    permalink: "https://www.linkedin.com/feed/update/urn:li:activity:123/",
    contentKind: "video",
    media: [{ ...photo, kind: "video_poster", url: "https://media.licdn.com/synthetic.jpg" }],
    mediaRecovery: { outcome: "unavailable" },
  }), source: "linkedin" });
  assert.match(byClass(card, "source-layout-media-unavailable")[0].textContent, /Video playback was not captured/);
  assert.equal(byClass(card, "recapture-button")[0].textContent, "Recapture video");
});

test("a quote without parent text stays nested and is not promoted to a parent caption", () => {
  const card = render(entry({ text: "", quotedPost: { author: "Quoted author", text: "Quoted caption", media: [photo] } }));
  const quotes = byClass(card, "source-quote-card");
  assert.equal(quotes.length, 1);
  assert.deepEqual(byClass(card, "expandable-text-copy").map(node => node.textContent), ["Quoted caption"]);
  assert.ok(quotes[0].textContent.includes("Quoted caption"));
  assert.equal(nodes(quotes[0], node => node.tagName === "IMG").length, 1);
  assert.doesNotMatch(card.textContent, /AI summary/);
});

test("media-only quotes remain visible beneath their own author without invented text", () => {
  const card = render(entry({ text: "", quotedPost: { author: "Quoted author", text: "", media: [photo] } }));
  const quotes = byClass(card, "source-quote-card");
  assert.equal(quotes.length, 1);
  assert.ok(quotes[0].textContent.includes("Quoted author"));
  assert.equal(nodes(quotes[0], node => node.tagName === "IMG").length, 1);
  assert.equal(byClass(card, "expandable-text").length, 0);
  assert.doesNotMatch(card.textContent, /AI summary/);
});

test("parent caption and quoted caption keep separate text and media ownership", () => {
  const quotedPhoto = { ...photo, url: "https://pbs.twimg.com/media/quoted.jpg" };
  const card = render(entry({ text: "Parent caption", media: [photo], quotedPost: { author: "Quoted author", text: "Quoted caption", media: [quotedPhoto] } }));
  assert.deepEqual(byClass(card, "expandable-text-copy").map(node => node.textContent), ["Parent caption", "Quoted caption"]);
  const quote = byClass(card, "source-quote-card")[0];
  assert.deepEqual(nodes(quote, node => node.tagName === "IMG").map(node => node.src), [quotedPhoto.url]);
  assert.equal(nodes(card, node => node.tagName === "IMG").length, 2);
});

test("empty captured text differs from unavailable source evidence", () => {
  assert.equal(byClass(render(entry({ text: "" })), "expandable-text").length, 0);
  const unavailable = render({ source: "x", item: { author: "Fallback author", whatChanged: "Legacy summary" } });
  assert.deepEqual(byClass(unavailable, "expandable-text-copy").map(node => node.textContent), ["Legacy summary"]);
});

test("empty and unsafe quote media do not create an empty quote card", () => {
  for (const quotedPost of [{}, { text: "", media: [] }, { text: "", media: [{ ...photo, url: "javascript:invalid" }] }]) {
    assert.equal(byClass(render(entry({ text: "Parent caption", quotedPost })), "source-quote-card").length, 0);
  }
});
