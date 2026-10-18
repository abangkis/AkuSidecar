import test from "node:test";
import assert from "node:assert/strict";
import vm from "node:vm";
import { readFileSync } from "node:fs";
import { planTimelineConversations, decorateTimelineConversation } from "../internal/httpapi/web/timeline-conversations.js";

const post = (id, target = "", extra = {}) => ({ id, source: "fixture-network", sessionId: "new", createdAt: "2026-10-13", item: { author: id },
  ...(target ? { conversation: { kind: "replies_to", status: "resolved", targetTimelineId: target, provenance: "observed_response" } } : {}), ...extra });
const order = groups => groups.flatMap(group => group.members.map(member => member.entry.id));

test("generic conversation grouping anchors at the newest member and keeps every card once", () => {
  const items = [post("reply", "root"), post("unrelated"), post("root", "", { sessionId: "old" }), post("other")];
  const before = structuredClone(items);
  const groups = planTimelineConversations(items);
  assert.deepEqual(order(groups), ["root", "reply", "unrelated", "other"]);
  assert.equal(groups[0].anchor.id, "reply");
  assert.deepEqual(groups[0].members.map(({ parentID, depth }) => ({ parentID, depth })), [{ parentID: "", depth: 0 }, { parentID: "root", depth: 1 }]);
  assert.deepEqual(items, before, "read presentation never rewrites rank, batch, evidence or timestamps");
});

test("reply chains and sibling branches read parent-first with stable sibling order", () => {
  const groups = planTimelineConversations([post("leaf", "middle"), post("sibling", "root"), post("middle", "root"), post("root")]);
  assert.deepEqual(order(groups), ["root", "sibling", "middle", "leaf"]);
  assert.equal(groups[0].members.at(-1).depth, 2);
});

test("missing, ambiguous, self, cross-source, quote and invented relationships never move cards", () => {
  for (const reply of [
    post("reply", "missing"), post("reply", "reply"),
    post("reply", "root", { source: "other-network" }),
    post("reply", "root", { conversation: { kind: "quotes", status: "resolved", targetTimelineId: "root", provenance: "observed_dom" } }),
    post("reply", "root", { conversation: { kind: "replies_to", status: "ambiguous", targetTimelineId: "root", provenance: "observed_dom" } }),
    post("reply", "root", { conversation: { kind: "replies_to", status: "resolved", targetTimelineId: "root", provenance: "model_guess" } }),
  ]) assert.deepEqual(order(planTimelineConversations([reply, post("other"), post("root")])), ["reply", "other", "root"]);
  assert.deepEqual(order(planTimelineConversations([post("reply", "root"), post("root"), post("root")])), ["reply", "root", "root"]);
});

test("cycles fail closed without dropping cards, and a long reply chain is stack-safe", () => {
  assert.deepEqual(order(planTimelineConversations([post("a", "b"), post("other"), post("b", "a")])), ["a", "other", "b"]);
  const items = Array.from({ length: 1000 }, (_, index) => post(String(index), index < 999 ? String(index + 1) : ""));
  const groups = planTimelineConversations(items);
  assert.equal(groups.length, 1);
  assert.equal(groups[0].members[0].entry.id, "999");
  assert.equal(groups[0].members.at(-1).entry.id, "0");
  assert.equal(new Set(order(groups)).size, 1000);
});

function element(tag = "div") {
  const classes = new Set();
  return { tag, children: [], dataset: {}, attributes: {}, textContent: "", listeners: {},
    classList: { add: (...values) => values.forEach(value => classes.add(value)), remove: value => classes.delete(value), contains: value => classes.has(value) },
    setAttribute(name, value) { this.attributes[name] = value; },
    append(...values) { this.children.push(...values); }, prepend(...values) { this.children.unshift(...values); },
    replaceChildren(...values) { this.children = values; },
    addEventListener(name, callback) { this.listeners[name] = callback; },
    querySelectorAll() { return this.children.filter(child => child.dataset.timelineId); },
    scrollIntoView(options) { this.scrollOptions = options; }, focus(options) { this.focusOptions = options; },
  };
}
const documentRef = { createElement: element };

test("conversation cues use safe text, preserve older-check provenance and navigate to the immediate parent", () => {
  const groups = planTimelineConversations([post("reply", "root"), post("root", "", { sessionId: "old", item: { author: "<img onerror=unsafe>" }, createdAt: "2026-10-12" })]);
  const cards = groups[0].members.map(() => element("article"));
  const navigated = [];
  groups[0].members.forEach((member, index) => decorateTimelineConversation(cards[index], member, groups[0], {
    documentRef, navigateToParent: id => navigated.push(id),
  }));
  const cue = cards[1].children[0];
  assert.equal(cue.children[0].children[1].textContent, "Reply to <img onerror=unsafe>");
  assert.equal(cue.children[0].attributes["aria-label"], "View parent post by <img onerror=unsafe>");
  cue.children[0].listeners.click();
  assert.deepEqual(navigated, ["root"]);
  assert.equal(cards[0].children[0].children[1].textContent, "From another check · 2026-10-12");
  assert.equal(cards[0].dataset.conversationId, "root");
  const missing = post("orphan", "", { conversation: { kind: "replies_to", status: "not_in_view", provenance: "observed_dom" } });
  const group = planTimelineConversations([missing])[0];
  const card = element(); decorateTimelineConversation(card, group.members[0], group, { documentRef });
  assert.equal(card.children[0].children[0].textContent, "Reply · parent not in this view");
});

const app = readFileSync(new URL("../internal/httpapi/web/app.js", import.meta.url), "utf8");
function render(items, mode = "drawer") {
  const elements = new Map();
  const $ = selector => { if (!elements.has(selector)) elements.set(selector, element()); return elements.get(selector); };
  const observed = [], drawer = [], timers = [];
  const state = { bootstrap: { settings: { aiDetectionPresentation: mode } }, seenTimelineItems: new Set(),
    passiveMediaEvidenceAttempts: new Map(), expandedTimelineText: new Set(), expandedAIDetails: new Set(), expandedAIFeedbackOptions: new Set(), timelineContentContext: new Map() };
  const context = { $, state, document: documentRef, window: { matchMedia: () => ({ matches: true }), setTimeout: callback => timers.push(callback) }, planTimelineConversations, decorateTimelineConversation,
    sourceIdentity: author => ({ displayName: author }),
    schedulePassiveMediaEnrichment() {}, renderTimelineSidePane: values => drawer.push(...values), scheduleAIDeepRefresh() {}, stopObservingInlineVideoVisibilityWithin() {},
    formatTimelineCheckSummary: () => "2 updates", formatDate: value => value, renderTimelineActions() {},
    buildTimelineItem: () => element("article"), observeTimelineItem: (_, id) => observed.push(id), syncTimelineContentContextTabs() {}, scheduleBackToTop() {}, scheduleTimelineContentContextPosition() {} };
  vm.createContext(context);
  vm.runInContext(app.slice(app.indexOf("function routeAIDetectedItems("), app.indexOf("function renderTimeline(items")), context);
  vm.runInContext(app.slice(app.indexOf("function renderTimelineNow("), app.indexOf("function renderTimelineActions(")), context);
  context.items = items;
  vm.runInContext('renderTimelineNow(items, {sessionId:"new",completedAt:"new-time"}, [{sessionId:"new",completedAt:"new-time"},{sessionId:"old",completedAt:"old-time"}])', context);
  return { elements, state, observed, drawer, timers };
}

test("actual Timeline rendering preserves batch boundaries, observes each card and keeps AI filtering authoritative", () => {
  const parent = post("root", "", { sessionId: "old" });
  const items = [post("reply", "root"), post("unrelated"), parent, post("older", "", { sessionId: "old" })];
  const rendered = render(items);
  assert.deepEqual(rendered.observed, ["root", "reply", "unrelated", "older"]);
  const rows = rendered.elements.get("#result-items").children;
  const markers = rows.filter(row => !row.dataset.timelineId);
  assert.equal(markers.length, 2, "moving an older parent does not introduce spurious old/new batch separators");
  assert.equal(markers[1].classList.contains("timeline-history-boundary"), true);
  const cards = rows.filter(row => row.dataset.timelineId);
  assert.equal(cards[0].children[0].children[1].textContent, "From another check · old-time");
  cards[1].children[0].children[0].listeners.click();
  assert.equal(cards[0].scrollOptions.behavior, "instant");
  assert.equal(cards[0].focusOptions.preventScroll, true);
  assert.equal(cards[0].classList.contains("timeline-conversation-target"), true);
  rendered.timers[0]();
  assert.equal(cards[0].classList.contains("timeline-conversation-target"), false);
  const filtered = render([...items.slice(0, 2), { ...parent, aiDetection: { routeToSignals: true } }]);
  assert.deepEqual(filtered.observed, ["reply", "unrelated"]);
  assert.deepEqual(filtered.drawer.map(entry => entry.id), ["root"]);
  assert.deepEqual(filtered.state.timelineItems.map(entry => entry.id), ["reply", "unrelated", "root"]);
});

test("reply chips stay beside source identity and remain available in brief mode", () => {
  const group = planTimelineConversations([post("reply", "root"), post("root")])[0];
  const card = element("article"), identity = element(), brief = element();
  card.querySelector = selector => selector === ".source-layout-card > header > div" ? identity
    : selector === ":scope > .item-layout-view" ? brief : null;
  const visited = [];
  decorateTimelineConversation(card, group.members[1], group, { documentRef,
    navigateToParent: id => visited.push(id), formatAuthor: name => name.toUpperCase() });
  assert.equal(identity.children.length, 1);
  assert.equal(brief.children.length, 1);
  const button = identity.children[0].children[0];
  assert.equal(button.children[1].textContent, "Reply to ROOT");
  assert.equal(button.children[2].textContent, "View parent");
  assert.equal(button.type, "button");
  button.listeners.click(); brief.children[0].children[0].listeners.click();
  assert.deepEqual(visited, ["root", "root"]);
});

test("branch chips name their immediate parent while unresolved replies have no connector or guessed destination", () => {
  const group = planTimelineConversations([post("leaf", "middle"), post("sibling", "root"), post("middle", "root"), post("root")])[0];
  for (const member of group.members.filter(value => value.parentID)) {
    const card = element(); decorateTimelineConversation(card, member, group, { documentRef });
    assert.equal(card.children[0].children[0].children[1].textContent, `Reply to ${member.parentID}`);
  }
  for (const status of ["not_in_view", "ambiguous"]) {
    const entry = post("orphan", "", {conversation:{kind:"replies_to",status,provenance:"observed_dom"}});
    const single = planTimelineConversations([entry])[0], card = element();
    decorateTimelineConversation(card, single.members[0], single, {documentRef});
    assert.equal(card.classList.contains("timeline-conversation-member"), false);
    assert.equal(card.children[0].children[0].tag, "span");
    assert.equal(card.children[0].children[0].className, "timeline-conversation-unresolved");
  }
});

test("conflicting native reply capture has a neutral label without a fabricated parent edge", () => {
  const entry = { ...post("conflict"), source: "x", evidence: { presentation: { directContextStatus: "ambiguous" } } };
  const group = planTimelineConversations([entry, { ...post("other"), source: "x" }])[0];
  const card = element(); decorateTimelineConversation(card, group.members[0], group, { documentRef });
  assert.equal(card.classList.contains("timeline-conversation-member"), false);
  assert.equal(card.children[0].children[0].textContent, "Reply · parent identity is ambiguous");
  assert.equal(card.children[0].children[0].tag, "span");
});
