const REPLY_PROVENANCE = new Set(["observed_dom", "observed_response", "legacy_capture"]);

// The Timeline receives resolved local item IDs, not platform URL parsers.
// Plan only the cards admitted into this view (after AI routing/pagination).
// Groups keep their newest/ranked member's position; their parent reads first.
export function planTimelineConversations(items = []) {
  const entries = Array.isArray(items) ? items : [];
  const byID = new Map(), duplicateIDs = new Set();
  entries.forEach(entry => {
    if (byID.has(entry.id)) duplicateIDs.add(entry.id);
    else byID.set(entry.id, entry);
  });
  const parents = new Map();
  for (const entry of entries) {
    const relation = entry.conversation;
    const parent = byID.get(relation?.targetTimelineId);
    if (!duplicateIDs.has(entry.id) && !duplicateIDs.has(parent?.id)
      && relation?.kind === "replies_to" && relation.status === "resolved"
      && REPLY_PROVENANCE.has(relation.provenance) && parent && parent !== entry
      && entry.source && parent.source === entry.source) parents.set(entry.id, parent.id);
  }

  // Reject cyclic edges instead of inventing a conversation root. Descendants
  // may still attach to an actual retained card whose cyclic edge was removed.
  const done = new Set();
  for (const start of parents.keys()) {
    const path = [], local = new Map();
    let current = start;
    while (parents.has(current) && !done.has(current) && !local.has(current)) {
      local.set(current, path.length); path.push(current); current = parents.get(current);
    }
    if (local.has(current)) for (const id of path.slice(local.get(current))) parents.delete(id);
    for (const id of path) done.add(id);
  }

  const rootByID = new Map();
  const rootFor = id => {
    const path = [];
    let current = id;
    while (parents.has(current) && !rootByID.has(current)) { path.push(current); current = parents.get(current); }
    const root = rootByID.get(current) || current;
    for (const member of path) rootByID.set(member, root);
    return root;
  };
  const groups = new Map();
  const children = new Map();
  for (const entry of entries) {
    // Ambiguous item IDs never participate, but preserve every input card.
    const root = duplicateIDs.has(entry.id) ? entry : rootFor(entry.id);
    if (!groups.has(root)) groups.set(root, { anchor: entry, members: [] });
    groups.get(root).members.push(entry);
    const parentID = parents.get(entry.id);
    if (parentID) {
      if (!children.has(parentID)) children.set(parentID, []);
      children.get(parentID).push(entry);
    }
  }
  return [...groups.values()].map(group => {
    if (group.members.length === 1) return { anchor: group.anchor, members: [{ entry: group.members[0], parentID: "", depth: 0 }] };
    const root = group.members.find(entry => !parents.has(entry.id));
    const members = [], stack = [{ entry: root, parentID: "", depth: 0 }];
    while (stack.length) {
      const member = stack.pop(); members.push(member);
      const replies = children.get(member.entry.id) || [];
      for (let index = replies.length - 1; index >= 0; index--) {
        stack.push({ entry: replies[index], parentID: member.entry.id, depth: member.depth + 1 });
      }
    }
    return { anchor: group.anchor, members };
  });
}

export function decorateTimelineConversation(card, member, group, {
  documentRef = document, formatDate = value => value || "", checkedAt = entry => entry.createdAt,
  navigateToParent = () => {}, formatAuthor = value => value,
} = {}) {
  const { entry, parentID } = member;
  const grouped = group.members.length > 1;
  const relation = entry.conversation;
  const knownReply = relation?.kind === "replies_to" && REPLY_PROVENANCE.has(relation.provenance);
  const ambiguousCapture = entry.source === "x" && entry.evidence?.presentation?.directContextStatus === "ambiguous";
  if (!grouped && !knownReply && !ambiguousCapture) return;
  if (grouped) {
    card.classList.add("timeline-conversation-member");
    card.dataset.conversationId = group.members[0].entry.id;
    card.dataset.conversationDepth = String(member.depth);
    card.classList.add(parentID ? "timeline-conversation-reply" : "timeline-conversation-root");
  }
  const buildCue = () => {
    const cue = documentRef.createElement("div");
    cue.className = "timeline-conversation-cue";
    if (parentID) {
      const parent = group.members.find(value => value.entry.id === parentID)?.entry;
      const author = formatAuthor(parent?.evidence?.author || parent?.item?.author || "post", parent?.source);
      const link = documentRef.createElement("button");
      link.type = "button"; link.className = "timeline-reply-parent";
      link.title = `View parent post by ${author}`;
      link.setAttribute("aria-label", link.title);
      const arrow = documentRef.createElement("span");
      arrow.className = "timeline-reply-arrow";
      arrow.textContent = "↳";
      arrow.setAttribute("aria-hidden", "true");
      const name = documentRef.createElement("span");
      name.className = "timeline-reply-name";
      name.textContent = `Reply to ${author}`;
      const hint = documentRef.createElement("span");
      hint.className = "timeline-reply-action";
      hint.textContent = "View parent";
      hint.setAttribute("aria-hidden", "true");
      link.append(arrow, name, hint);
      link.addEventListener("click", () => navigateToParent(parentID));
      cue.append(link);
    } else {
      const label = documentRef.createElement("span");
      label.className = grouped ? "timeline-conversation-label" : "timeline-conversation-unresolved";
      label.textContent = grouped ? `Conversation · ${group.members.length} posts`
        : ambiguousCapture || relation?.status === "ambiguous" ? "Reply · parent identity is ambiguous"
          : relation?.status === "resolved" ? "Reply · parent could not be matched in this view" : "Reply · parent not in this view";
      cue.append(label);
    }
    if (entry.sessionId !== group.anchor.sessionId) {
      const date = documentRef.createElement("small");
      date.textContent = `From another check · ${formatDate(checkedAt(entry))}`;
      cue.append(date);
    }
    return cue;
  };
  // Both presentation modes retain the cue; only their visible copy is exposed.
  const identity = !card.classList.contains("semantic-duplicate-item")
    && card.querySelector?.(".source-layout-card > header > div");
  if (identity) {
    identity.append(buildCue());
    card.querySelector(":scope > .item-layout-view")?.prepend(buildCue());
  } else card.prepend(buildCue());
}
