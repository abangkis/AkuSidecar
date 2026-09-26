import {
  contentContextKnownTypes, contentContextKnownTypesDescription,
  contentContextRelationLabel, contentContextObjectCaptureLabel,
} from "./timeline-content-context-state.js";

const SVG_NS = "http://www.w3.org/2000/svg";

export function syncTimelineContentContextTab(tab, { active = false } = {}) {
  if (!tab) return;
  tab.setAttribute("aria-expanded", String(active));
  const cue = tab.dataset.contextCueDescription || "";
  const label = active ? "Close related context" : cue ? "Related context. " + cue : "Related context";
  tab.setAttribute("aria-label", label);
  tab.title = active ? "Close related context" : cue || "Conversation, interaction, and local context";
}

export function buildTimelineContentContextTab(entry, onToggle, documentRef = document) {
  const anchor = documentRef.createElement("div");
  anchor.className = "timeline-content-context-anchor";
  const tab = documentRef.createElement("button");
  tab.type = "button";
  tab.className = "timeline-content-context-tab";
  tab.dataset.timelineContentContextId = entry.id;
  tab.setAttribute("aria-controls", "timeline-content-context-drawer");
  const types = contentContextKnownTypes({ source: entry.source || entry.item?.source, directContext: entry.evidence?.directContext });
  const cue = contentContextKnownTypesDescription(types);
  if (cue) {
    tab.classList.add("has-direct-context-cue");
    tab.dataset.contextCueDescription = cue;
    const icon = documentRef.createElementNS(SVG_NS, "svg");
    icon.classList.add("timeline-content-context-cue-icon");
    icon.setAttribute("viewBox", "0 0 20 20");
    icon.setAttribute("aria-hidden", "true");
    icon.setAttribute("focusable", "false");
    const path = documentRef.createElementNS(SVG_NS, "path");
    path.setAttribute("d", "M5 3v4a5 5 0 0 0 5 5h5m-3-3 3 3-3 3");
    icon.append(path);
    for (const [cx, cy] of [[5, 3], [5, 7], [15, 12]]) {
      const node = documentRef.createElementNS(SVG_NS, "circle");
      node.setAttribute("cx", String(cx)); node.setAttribute("cy", String(cy)); node.setAttribute("r", "1.6");
      icon.append(node);
    }
    tab.append(icon);
  }
  const label = documentRef.createElement("span");
  label.className = "timeline-content-context-tab-label";
  label.textContent = "Related context";
  tab.append(label);
  syncTimelineContentContextTab(tab);
  tab.addEventListener("click", onToggle);
  anchor.append(tab);
  return { anchor, tab };
}

export function renderTimelineDirectContext(relation, source, { documentRef = document, safeSourceUrl = () => "", configureNativePostLink = () => {}, formatDate = () => "" } = {}) {
  const article = documentRef.createElement("article");
  article.className = "timeline-content-context-match timeline-direct-context";
  const title = documentRef.createElement("strong");
  title.className = "timeline-direct-context-relation";
  title.textContent = contentContextRelationLabel(source, relation);
  article.append(title);
  if (relation?.observedText) {
    const observed = documentRef.createElement("p"); observed.textContent = "Platform label: " + relation.observedText; article.append(observed);
  }
  const appendObject = (object, label) => {
    if (!object) return;
    const box = documentRef.createElement("div"); box.className = "timeline-direct-context-object";
    const heading = documentRef.createElement("strong"); heading.textContent = [label, object.author].filter(Boolean).join(" · "); box.append(heading);
    const status = documentRef.createElement("span"); status.className = "timeline-direct-context-capture-status";
    const captureLabel = contentContextObjectCaptureLabel(object);
    status.classList.add(captureLabel === "Content not captured" ? "is-not-captured" : captureLabel === "Partial capture" ? "is-partial" : "is-captured");
    status.textContent = captureLabel; box.append(status);
    if (object.text || object.hasMedia) { const text = documentRef.createElement("p"); text.textContent = object.text || "Media captured with this post."; box.append(text); }
    const url = safeSourceUrl(object.permalink, source);
    if (url) { const link = documentRef.createElement("a"); link.className = "source-link"; link.href = url; configureNativePostLink(link, url, source); link.textContent = "Open source"; box.append(link); }
    const meta = documentRef.createElement("small");
    meta.textContent = [object.evidenceOrigin === "local_timeline" ? "Retained Timeline" : object.evidenceOrigin === "local_memory" ? "Local full copy" : "Source capture", object.capturedAt ? "captured " + formatDate(object.capturedAt) : null].filter(Boolean).join(" · ");
    if (meta.textContent) box.append(meta); article.append(box);
  };
  appendObject(relation?.parent, "Parent comment");
  appendObject(relation?.target, relation?.kind === "feed_reply" ? "Reply" : relation?.kind === "feed_comment" ? "Comment" : "Post");
  if (relation?.kind === "feed_reply" && !relation.parent) { const missing = documentRef.createElement("small"); missing.textContent = "Parent comment not captured."; article.append(missing); }
  if (relation?.capturedAt) { const time = documentRef.createElement("small"); time.textContent = "Observed " + formatDate(relation.capturedAt); article.append(time); }
  return article;
}