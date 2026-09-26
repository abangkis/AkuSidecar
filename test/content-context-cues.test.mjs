import test from "node:test";
import assert from "node:assert/strict";

import {
  contentContextKnownTypes,
  contentContextKnownTypesDescription,
  contentContextRelationLabel,
  contentContextObjectCaptureLabel,
} from "../internal/httpapi/web/timeline-content-context-state.js";

const xPost = { kind: "post", availability: "captured", text: "Captured body" };
const liComment = { kind: "comment", availability: "reference_only" };
const liCapture = { kind: "comment", availability: "partial", text: "Partial body" };

test("known conversation cues exclude X quotes and require a valid source-compatible relation", () => {
  const types = contentContextKnownTypes({ source: "x", directContext: [
    { kind: "replies_to", provenance: "observed_response", target: xPost },
    { kind: "quotes", provenance: "observed_dom", target: xPost },
    { kind: "feed_reply", provenance: "observed_dom", target: liComment },
    { kind: "replies_to", provenance: "model_guess", target: xPost },
    { kind: "replies_to", provenance: "observed_dom", target: { kind: "post", availability: "captured" } },
  ] });
  assert.deepEqual(types, ["Reply to post"]);
  assert.equal(contentContextKnownTypesDescription(types), "Known conversation: reply to post.");
  assert.deepEqual(contentContextKnownTypes({ source: "x", directContext: [], quotedPost: { text: "Visible inline quote" } }), []);
  assert.deepEqual(contentContextKnownTypes({ source: "facebook", directContext: [
    { kind: "replies_to", provenance: "observed_response", target: xPost },
  ] }), []);
});

test("LinkedIn comment and reply types need no invented actor or parent names", () => {
  const directContext = [
    { kind: "feed_comment", provenance: "observed_dom", target: liComment, actor: "Ayu" },
    { kind: "feed_reply", provenance: "observed_dom", target: liComment, actor: "Bima", parent: { ...liComment, author: "Dara" } },
  ];
  assert.deepEqual(contentContextKnownTypes({ source: "linkedin", directContext }), ["Comment", "Reply to comment"]);
  assert.equal(contentContextRelationLabel("linkedin", directContext[0]), "Comment by Ayu");
  assert.equal(contentContextRelationLabel("linkedin", directContext[1]), "Bima replied to Dara");
  assert.equal(contentContextRelationLabel("linkedin", { kind: "feed_comment" }), "Comment");
  assert.equal(contentContextRelationLabel("linkedin", { kind: "feed_reply", actor: "Bima" }), "Reply to comment");
});

test("legacy X quote capture does not create a Related Context cue", () => {
  for (const quotedPost of [
    { text: "Inline quote" },
    { media: [{}] },
    { permalink: "https://x.com/i/status/67890" },
    {},
  ]) {
    assert.deepEqual(contentContextKnownTypes({ source: "x", quotedPost }), []);
  }
});

test("capture labels distinguish missing body, partial body, and media-only evidence", () => {
  assert.equal(contentContextObjectCaptureLabel({ availability: "reference_only" }), "Content not captured");
  assert.equal(contentContextObjectCaptureLabel({ availability: "partial", text: "Part" }), "Partial capture");
  assert.equal(contentContextObjectCaptureLabel({ availability: "captured", hasMedia: true }), "Captured evidence");
  assert.equal(contentContextObjectCaptureLabel({ availability: "captured", text: "Body" }), "Captured evidence");
});