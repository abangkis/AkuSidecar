import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import vm from "node:vm";
import { boundedTimelineMedia, shouldUseTimelineCarousel } from "../internal/httpapi/web/timeline-media-carousel.js";
import { reserveMediaDimensions } from "../internal/httpapi/web/timeline-media-layout.js";

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

const app = readFileSync(new URL("../internal/httpapi/web/app.js", import.meta.url), "utf8");
const slice = (from, to) => app.slice(app.indexOf(`function ${from}(`), app.indexOf(`function ${to}(`));
function renderer() {
  const context = vm.createContext({
    URL, document: { createElement: tag => new Element(tag) },
    SOURCE_TEXT_COLLAPSE_CHARACTERS: 420, SOURCE_TEXT_COLLAPSE_LINES: 6,
    QUOTE_TEXT_COLLAPSE_CHARACTERS: 320, QUOTE_TEXT_COLLAPSE_LINES: 4,
    sourceDescriptor: () => ({ nativeHosts: ["x.com"], nativePathTokens: ["/status/"] }),
    sourceIdentity: author => ({ displayName: author }), sourceLabel: () => "X",
    buildAvatar: () => new Element("span"), postHeaderContext: () => "", formatDate: () => "",
    applyPostFreshness() {}, postHeaderContexts: new WeakMap(),
    state: { expandedTimelineText: new Set() }, buildAttachments: () => null,
    buildEngagement: () => null, videoPlaybackMissing: () => false,
    boundedTimelineMedia, shouldUseTimelineCarousel, reserveMediaDimensions,
    safePlaybackUrl: () => null, openMedia() {},
  });
  vm.runInContext([
    slice("buildSourceCard", "applyPostFreshness"),
    slice("buildQuotedPost", "buildMedia"),
    slice("buildMedia", "buildTimelineMediaCarousel"),
    slice("buildImageMediaControl", "buildVideoMedia"),
    slice("safeMediaUrl", "safePlaybackUrl"),
    slice("safeSourceUrl", "buildMediaRecaptureButton"),
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
