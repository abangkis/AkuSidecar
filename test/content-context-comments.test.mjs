import test from "node:test";
import assert from "node:assert/strict";
import { renderTimelineDirectContext } from "../internal/httpapi/web/timeline-content-context-ui.js";
import { contentContextObjectCaptureReason } from "../internal/httpapi/web/timeline-content-context-state.js";

// Exercise the real renderer, including textContent escaping and child order.
class Element {
  constructor(tag) { this.tag = tag; this.children = []; this.value = ""; this.classList = { add() {} }; }
  append(...children) { this.children.push(...children); }
  set textContent(value) { this.value = String(value); this.children = []; }
  get textContent() { return this.value + this.children.map(child => child.textContent).join("\n"); }
}
const documentRef = { createElement: tag => new Element(tag) };
const relation = (id, text) => ({ kind: "feed_comment", actor: "Synthetic account", actorUrl: "https://www.linkedin.com/in/synthetic", observedText: "Synthetic account commented", matchBasis: "actor_profile", provenance: "observed_dom", target: { kind: "comment", id, availability: "captured", text } });

test("Related Context renders both same-account comments without picking a banner trigger", () => {
  const rows = [relation("12345", "First observed comment"), relation("67890", "Second observed comment")];
  const rendered = rows.map(row => renderTimelineDirectContext(row, "linkedin", { documentRef }));
  assert.equal(rendered.length, 2);
  for (const [index, article] of rendered.entries()) {
    assert.match(article.textContent, /Matched by account profile/);
    assert.match(article.textContent, /does not identify which comment prompted the feed/);
    assert.match(article.textContent, /Platform label: Synthetic account commented/);
    assert.ok(article.textContent.includes(rows[index].target.text));
    assert.doesNotMatch(article.textContent, /Content not captured/);
  }
});

test("missing-body reasons and legacy unknown reason render distinctly", () => {
  const reasons = ["actor_identity_unresolved", "comments_not_observed", "comments_not_visible", "matching_comments_not_observed", "comment_author_unresolved", "comment_body_not_observed", "comment_body_ambiguous", "comment_identity_ambiguous", "comment_read_failed"];
  const labels = new Set();
  for (const captureReason of reasons) {
    const row = relation("12345", "");
    row.target = { kind: "comment", availability: "reference_only", captureReason };
    const description = contentContextObjectCaptureReason(row.target);
    labels.add(description);
    const rendered = renderTimelineDirectContext(row, "linkedin", { documentRef });
    assert.ok(rendered.textContent.includes(description));
    assert.match(rendered.textContent, /Content not captured/);
  }
  assert.equal(labels.size, reasons.length);
  assert.match(contentContextObjectCaptureReason({ kind: "comment", availability: "reference_only" }), /reason was not recorded/);
  assert.equal(contentContextObjectCaptureReason({ kind: "post", availability: "reference_only" }), "");
  assert.equal(contentContextObjectCaptureReason({ kind: "comment", availability: "captured", text: "Retained body", captureReason: "comment_body_not_observed" }), "");
});

test("capture bounds disclose omitted comments and text remains plain content", () => {
  const row = relation("12345", "<script>unsafe()</script>");
  row.omittedCommentCount = 3;
  row.commentReadErrorCount = 1;
  const rendered = renderTimelineDirectContext(row, "linkedin", { documentRef });
  assert.match(rendered.textContent, /3 additional matching comments exceeded the capture limit of 8/);
  assert.match(rendered.textContent, /1 comment element could not be read reliably/);
  assert.ok(rendered.textContent.includes(row.target.text));
});
