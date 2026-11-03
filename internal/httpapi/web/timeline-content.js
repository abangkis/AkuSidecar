import { classifyPostFreshness, postHeaderContext } from "./post-freshness.js";

export function createTimelineContent({
  QUOTE_TEXT_COLLAPSE_CHARACTERS, QUOTE_TEXT_COLLAPSE_LINES, SOURCE_TEXT_COLLAPSE_CHARACTERS, SOURCE_TEXT_COLLAPSE_LINES, buildAIDetectionControls, buildActions, buildForegroundRecaptureOffer, buildMedia, buildMediaRecaptureButton, buildSourceIcon, buildTimelineContentContextAnchor, closeTimelineContentContextDrawer, formatDate, humanize, initials, mediaRecaptureReasonForEntry, getPostScreenshotUI, safeMediaUrl, safeSourceUrl, sourceDescriptor, sourceIdentity, sourceLabel, state, syncTimelineContentContextTabs
}) {
  const postHeaderContexts = new WeakMap();

  function buildTimelineItem(entry, { contentContext = true } = {}) {
    if (entry.semanticEvent?.relation === "duplicate_report") return buildCollapsedDuplicate(entry, { contentContext });
    const item = buildExpandedTimelineItem(entry);
    return contentContext ? buildTimelineContentContextAnchor(entry, item) : item;
  }

  function buildCollapsedDuplicate(entry, { contentContext = true } = {}) {
    const container = document.createElement("article");
    container.className = "presentable-item semantic-duplicate-item";
    const summary = document.createElement("div");
    summary.className = "semantic-duplicate-summary";
    const copy = document.createElement("div");
    const label = document.createElement("span");
    label.className = "semantic-duplicate-label";
    label.textContent = "Cross-author semantic duplicate";
    const claim = document.createElement("strong");
    claim.textContent = entry.semanticEvent.canonicalClaim || entry.item?.whatChanged || "Same reported event";
    const provenance = document.createElement("small");
    provenance.textContent = [sourceLabel(entry.source), entry.item?.author, `${entry.semanticEvent.reportCount || 2} reports in this event`].filter(Boolean).join(" \u00b7 ");
    copy.append(label, claim, provenance);
    const toggle = document.createElement("button");
    toggle.type = "button";
    toggle.className = "secondary-button semantic-duplicate-toggle";
    toggle.textContent = "Show report";
    toggle.setAttribute("aria-expanded", "false");
    const expandedReport = buildExpandedTimelineItem(entry);
    const report = contentContext ? buildTimelineContentContextAnchor(entry, expandedReport) : expandedReport;
    report.classList.add("semantic-duplicate-report", "hidden");
    toggle.addEventListener("click", () => {
      const expanded = toggle.getAttribute("aria-expanded") === "true";
      if (expanded && state.timelineContentContextActiveID === entry.id) {
        closeTimelineContentContextDrawer({ clearActive: true, focusTrigger: false });
      }
      toggle.setAttribute("aria-expanded", String(!expanded));
      toggle.textContent = expanded ? "Show report" : "Hide report";
      report.classList.toggle("hidden", expanded);
      container.classList.toggle("is-report-expanded", !expanded);
      syncTimelineContentContextTabs();
    });
    summary.append(copy, toggle);
    container.append(summary, report);
    return container;
  }

  function buildExpandedTimelineItem(entry) {
    const container = document.createElement("article");
    container.className = "presentable-item";
    const toolbar = document.createElement("div");
    toolbar.className = "item-presentation-toolbar";
    const signalSlot = document.createElement("span");
    signalSlot.className = "item-signal-slot";
    const ai = buildAIDetectionControls(entry);
    signalSlot.append(buildSourceIcon(entry.source), ai.badge);
    const label = document.createElement("span");
    label.className = "presentation-label";
    const toggle = document.createElement("button");
    toggle.type = "button";
    toggle.className = "presentation-toggle";
    const brief = buildBrief(entry);
    const source = buildSourceCard(entry);
    container.dataset.freshness = source.dataset.freshness;
    const actions = buildActions(entry);
    let layout = entry.evidence && state.bootstrap?.settings?.defaultPresentation !== "brief" ? "source" : "brief";
    const render = () => {
      const sourceActive = layout === "source" && Boolean(entry.evidence);
      source.classList.toggle("hidden", !sourceActive);
      brief.classList.toggle("hidden", sourceActive);
      label.textContent = sourceActive ? "Source layout" : "Brief";
      toggle.textContent = sourceActive ? "Switch to brief" : "Switch to source layout";
      toggle.disabled = !entry.evidence;
    };
    toggle.addEventListener("click", () => {
      layout = layout === "source" ? "brief" : "source";
      render();
    });
    const presentationActions = document.createElement("div");
    presentationActions.className = "item-presentation-actions";
    presentationActions.append(toggle, getPostScreenshotUI().createMenu(entry));
    toolbar.append(signalSlot, label, presentationActions);
    container.append(toolbar);
    container.append(ai.details);
    container.append(brief, source, actions);
    render();
    return container;
  }

  function buildBrief(entry) {
    const item = entry.item ?? {};
    const brief = document.createElement("div");
    brief.className = "item-layout-view";
    const badge = document.createElement("span");
    badge.className = "evidence-badge";
    badge.textContent = [humanize(item.evidenceState), `${Math.round((item.confidence ?? 0) * 100)}%`, humanize(item.knowledgeDelta)].filter(Boolean).join(" · ");
    const title = document.createElement("h3");
    title.textContent = item.whatChanged;
    const why = document.createElement("p");
    why.className = "why-it-matters";
    why.textContent = item.whyItMatters;
    const provenance = document.createElement("p");
    provenance.className = "provenance";
    provenance.textContent = [sourceLabel(entry.source), item.author, item.publishedAt ? formatDate(item.publishedAt) : null].filter(Boolean).join(" · ");
    brief.append(badge, title, why, provenance);
    return brief;
  }

  function buildSourceCard(entry) {
    const evidence = entry.evidence ?? {};
    const item = entry.item ?? {};
    const source = entry.source;
    const descriptor = sourceDescriptor(source) || {};
    const card = document.createElement("div");
    card.className = `source-layout-card source-${source}`;
    const header = document.createElement("header");
    const avatar = buildAvatar(evidence.avatarUrl, source, evidence.author || item.author);
    const identity = document.createElement("div");
    const author = document.createElement("strong");
    const parsed = sourceIdentity(evidence.author || item.author, source);
    author.textContent = parsed.displayName;
    const context = document.createElement("span");
    const presentation = evidence.presentation ?? {};
    const headerContext = {
      publishedAt: evidence.publishedAt || item.publishedAt,
      timestampText: presentation.timestampText,
      connectionDegree: presentation.connectionDegree,
      secondary: parsed.secondary,
    };
    context.textContent = postHeaderContext(headerContext)
      || formatDate(evidence.publishedAt || item.publishedAt);
    identity.append(author);
    if (presentation.headline) {
      const headline = document.createElement("span");
      headline.className = "source-layout-headline";
      headline.textContent = presentation.headline;
      identity.append(headline);
    }
    if (context.textContent) identity.append(context);
    header.append(avatar, identity);
    applyPostFreshness(card, {
      publishedAt: evidence.publishedAt || item.publishedAt,
      timestampText: presentation.timestampText || parsed.secondary,
    });
    postHeaderContexts.set(card, { context, value: headerContext });

    if (descriptor.socialContextPlacement === "above" && presentation.socialContext) {
      const social = document.createElement("div");
      social.className = "source-social-context-above";
      const socialAvatarUrl = safeMediaUrl(presentation.socialContextAvatarUrl);
      if (socialAvatarUrl) {
        const socialAvatar = document.createElement("img");
        socialAvatar.src = socialAvatarUrl;
        socialAvatar.alt = "";
        socialAvatar.loading = "lazy";
        socialAvatar.referrerPolicy = "no-referrer";
        social.append(socialAvatar);
      }
      const socialCopy = document.createElement("span");
      socialCopy.textContent = presentation.socialContext;
      social.append(socialCopy);
      card.append(social);
    }
    card.append(header);

    const content = document.createElement("div");
    content.className = `source-layout-content source-${descriptor.presentationStyle || "social"}-content`;
    if (descriptor.socialContextPlacement !== "above" && presentation.socialContext) {
      const social = document.createElement("p");
      social.className = "source-social-context-content";
      social.textContent = presentation.socialContext;
      content.append(social);
    }
    const hasCapturedContent = typeof evidence.text === "string"
      || (Array.isArray(evidence.media) && evidence.media.length > 0)
      || (Array.isArray(evidence.attachments) && evidence.attachments.length > 0)
      || (evidence.quotedPost && Object.keys(evidence.quotedPost).length > 0);
    const sourceText = hasCapturedContent ? evidence.text : item.whatChanged;
    if (typeof sourceText === "string" && sourceText.trim()) {
      content.append(buildExpandableText(sourceText, {
        characterLimit: SOURCE_TEXT_COLLAPSE_CHARACTERS,
        lineLimit: SOURCE_TEXT_COLLAPSE_LINES,
        label: "post",
        expansionKey: entry.id ? `${entry.id}|post` : null,
      }));
    }
    const quote = buildQuotedPost(evidence.quotedPost, source, entry.id ? `${entry.id}|quote` : null);
    if (quote) content.append(quote);
    if (content.children.length) card.append(content);
    const attachments = buildAttachments(evidence.attachments, source);
    if (attachments) card.append(attachments);
    const nativePostUrl = safeSourceUrl(item.sourceUrl || evidence.permalink, source);
    const media = buildMedia(evidence.media, source, evidence.contentKind, nativePostUrl, entry);
    if (media) card.append(media);
    const recaptureReason = mediaRecaptureReasonForEntry(entry);
    if (recaptureReason === "unresolved_video" || evidence.mediaRecovery?.outcome === "unavailable") {
      const unavailable = document.createElement("div");
      unavailable.className = "source-layout-media-unavailable";
      const message = document.createElement("span");
      message.textContent = recaptureReason === "unresolved_video"
        ? "Video playback was not captured."
        : "Media was present at the source but unavailable in this captured view.";
      unavailable.append(message);
      unavailable.append(state.foregroundRecaptureOffers.has(entry.id) && state.bootstrap?.collectionRuntime?.effective !== "headless"
        ? buildForegroundRecaptureOffer(entry)
        : buildMediaRecaptureButton(entry, recaptureReason));
      card.append(unavailable);
    }
    const engagement = buildEngagement(evidence.engagement, source);
    if (engagement) card.append(engagement);
    return card;
  }

  function applyPostFreshness(card, value) {
    const result = classifyPostFreshness(value);
    card.dataset.freshness = result.key;
    if (Number.isFinite(result.referenceAt)) card.dataset.freshnessReferenceAt = String(result.referenceAt);
    else delete card.dataset.freshnessReferenceAt;
    const item = card.closest(".presentable-item");
    if (item) item.dataset.freshness = result.key;
  }

  function refreshVisiblePostFreshness() {
    const now = Date.now();
    for (const card of document.querySelectorAll(".source-layout-card[data-freshness]")) {
      applyPostFreshness(card, { referenceAt: Number(card.dataset.freshnessReferenceAt), now });
      const header = postHeaderContexts.get(card);
      if (header && Number.isFinite(Number(card.dataset.freshnessReferenceAt))) {
        header.context.textContent = postHeaderContext({
          ...header.value,
          referenceAt: Number(card.dataset.freshnessReferenceAt),
          now,
        });
      }
    }
  }

  function buildAvatar(url, source, author) {
    if (safeMediaUrl(url)) {
      const image = document.createElement("img");
      image.className = "source-layout-avatar";
      image.src = url;
      image.alt = "";
      image.loading = "lazy";
      image.referrerPolicy = "no-referrer";
      image.addEventListener("error", () => image.replaceWith(buildAvatar(null, source, author)), { once: true });
      return image;
    }
    const fallback = document.createElement("span");
    fallback.className = "source-layout-badge";
    const descriptor = sourceDescriptor(source);
    fallback.textContent = descriptor?.avatarFallback === "source_icon"
      ? (descriptor.iconText || sourceLabel(source).slice(0, 1))
      : initials(author);
    return fallback;
  }

  function buildQuotedPost(value, source, expansionKey = null) {
    const text = typeof value?.text === "string" ? value.text : "";
    const media = buildMedia(value?.media, source, value?.contentKind, safeSourceUrl(value?.permalink, source));
    if (!text.trim() && !media) return null;
    const quote = document.createElement("section");
    quote.className = "source-quote-card";
    const header = document.createElement("header");
    const avatar = buildAvatar(value.avatarUrl, source, value.author);
    const identity = document.createElement("div");
    const author = document.createElement("strong");
    author.textContent = value.author || "Quoted source";
    const timestamp = document.createElement("span");
    timestamp.textContent = formatDate(value.publishedAt);
    identity.append(author, timestamp);
    header.append(avatar, identity);
    quote.append(header);
    if (text.trim()) {
      quote.append(buildExpandableText(text, {
        characterLimit: QUOTE_TEXT_COLLAPSE_CHARACTERS,
        lineLimit: QUOTE_TEXT_COLLAPSE_LINES,
        label: "quoted post",
        expansionKey,
      }));
    }
    if (media) quote.append(media);
    return quote;
  }

  function buildExpandableText(value, { characterLimit, lineLimit, label, expansionKey = null }) {
    const wrapper = document.createElement("div");
    wrapper.className = "expandable-text";
    const text = document.createElement("p");
    text.className = "expandable-text-copy";
    text.textContent = value || "";
    wrapper.append(text);

    const logicalLines = text.textContent.split(/\r?\n/).length;
    if (text.textContent.length <= characterLimit && logicalLines <= lineLimit) return wrapper;

    const initiallyExpanded = expansionKey && state.expandedTimelineText.has(expansionKey);
    text.classList.toggle("is-collapsed", !initiallyExpanded);
    text.style.setProperty("--collapse-lines", String(lineLimit));
    const toggle = document.createElement("button");
    toggle.type = "button";
    toggle.className = "content-expander";
    toggle.textContent = initiallyExpanded ? "Show less" : "Show more";
    toggle.setAttribute("aria-expanded", String(Boolean(initiallyExpanded)));
    toggle.setAttribute("aria-label", `${initiallyExpanded ? "Show less" : "Show more"} ${label} text`);
    toggle.addEventListener("click", () => {
      const expanded = toggle.getAttribute("aria-expanded") === "true";
      text.classList.toggle("is-collapsed", expanded);
      toggle.setAttribute("aria-expanded", String(!expanded));
      toggle.setAttribute("aria-label", `${expanded ? "Show more" : "Show less"} ${label} text`);
      toggle.textContent = expanded ? "Show more" : "Show less";
      if (expansionKey) {
        if (expanded) state.expandedTimelineText.delete(expansionKey);
        else state.expandedTimelineText.add(expansionKey);
      }
    });
    wrapper.append(toggle);
    return wrapper;
  }

  function buildAttachments(values, source) {
    const attachments = (Array.isArray(values) ? values : [])
      .map((value) => ({ ...value, safeUrl: safeMediaUrl(value.url) }))
      .filter((value) => value.safeUrl && value.title)
      .slice(0, 3);
    if (!attachments.length) return null;
    const list = document.createElement("div");
    list.className = "source-layout-attachments";
    for (const value of attachments) {
      const link = document.createElement("a");
      link.className = "source-layout-attachment";
      link.href = value.safeUrl;
      link.target = "_blank";
      link.rel = "noopener noreferrer";
      const imageUrl = safeMediaUrl(value.imageUrl);
      if (imageUrl) {
        const image = document.createElement("img");
        image.src = imageUrl;
        image.alt = "";
        image.loading = "lazy";
        image.referrerPolicy = "no-referrer";
        link.append(image);
      } else {
        const fallback = document.createElement("span");
        fallback.className = "source-layout-attachment-fallback";
        fallback.textContent = sourceDescriptor(source)?.iconText || sourceLabel(source).slice(0, 1);
        link.append(fallback);
      }
      const copy = document.createElement("span");
      copy.className = "source-layout-attachment-copy";
      const title = document.createElement("strong");
      title.textContent = value.title;
      copy.append(title);
      const subtitle = [value.subtitle, value.detail].filter(Boolean).join(" · ");
      if (subtitle) {
        const detail = document.createElement("span");
        detail.textContent = subtitle;
        copy.append(detail);
      }
      const domain = document.createElement("small");
      domain.textContent = value.domain || value.actionLabel || "Open attachment";
      copy.append(domain);
      link.append(copy);
      list.append(link);
    }
    return list;
  }

  function buildEngagement(value, source) {
    if (!value || typeof value !== "object") return null;
    const definitions = (sourceDescriptor(source)?.engagementMetrics ?? [])
      .map((metric) => [metric.key, metric.icon]);
    const available = definitions.filter(([key]) => value[key]);
    if (!available.length) return null;
    const footer = document.createElement("footer");
    footer.className = "source-layout-engagement";
    for (const [key, icon] of available) {
      const metric = document.createElement("span");
      metric.textContent = `${icon} ${value[key]}`;
      footer.append(metric);
    }
    return footer;
  }

  return {
    buildTimelineItem, buildCollapsedDuplicate, buildExpandedTimelineItem, buildBrief, buildSourceCard, applyPostFreshness, refreshVisiblePostFreshness, buildAvatar, buildQuotedPost, buildExpandableText, buildAttachments, buildEngagement
  };
}
