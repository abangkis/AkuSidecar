const PLATFORM_LABELS = Object.freeze({
  x: "X",
  linkedin: "LinkedIn",
  facebook: "Facebook",
  instagram: "Instagram",
});

function escapeHTML(value) {
  return String(value ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}

function text(value) {
  return typeof value === "string" ? value : "";
}

function safeClass(value) {
  const candidate = text(value).toLowerCase();
  return /^[a-z0-9_-]{1,40}$/.test(candidate) ? candidate : "unknown";
}

function safeDimensions(value) {
  const number = Number(value);
  return Number.isFinite(number) && number > 0 ? Math.round(number) : 0;
}

function safeRasterDataURL(value) {
  if (typeof value !== "string") return "";
  const match = value.match(/^data:image\/(?:png|jpeg|jpg|webp|gif|avif);base64,([a-z0-9+/]*={0,2})$/i);
  if (!match) return "";
  const payload = match[1];
  return payload.length > 0 && payload.length % 4 === 0 && !/=/.test(payload.slice(0, -2)) ? value : "";
}

function mappedImage(media, originalURL) {
  if (!media || typeof media !== "object" || typeof originalURL !== "string" || !originalURL) return "";
  try {
    if (new URL(originalURL).protocol !== "https:") return "";
  } catch {
    return "";
  }
  if (!Object.prototype.hasOwnProperty.call(media, originalURL)) return "";
  return safeRasterDataURL(media[originalURL]);
}

function imageMarkup({ media, url, alt, width = 0, height = 0, className = "" }) {
  const dataURL = mappedImage(media, url);
  if (!dataURL) {
    return `<div class="post-screenshot-media-unavailable" role="img" aria-label="${escapeHTML(alt)} unavailable"><span>Image preview unavailable</span></div>`;
  }
  const dimensions = `${width ? ` width="${width}"` : ""}${height ? ` height="${height}"` : ""}`;
  return `<img${className ? ` class="${escapeHTML(className)}"` : ""} src="${escapeHTML(dataURL)}" alt="${escapeHTML(alt)}"${dimensions} />`;
}

function renderIdentity({ media, avatarURL, author, source, publishedAt, headline = "" }) {
  const avatar = mappedImage(media, avatarURL)
    ? imageMarkup({ media, url: avatarURL, alt: "", className: "source-layout-avatar" })
    : `<span class="source-layout-badge" aria-hidden="true">${escapeHTML(initials(author, source))}</span>`;
  const date = text(publishedAt);
  return `<header>${avatar}<div><strong>${escapeHTML(author || "Unknown author")}</strong>${headline ? `<span class="source-layout-headline">${escapeHTML(headline)}</span>` : ""}${date ? `<span>${escapeHTML(date)}</span>` : ""}</div></header>`;
}

function initials(author, source) {
  const value = text(author).trim();
  if (value) return value.split(/\s+/).slice(0, 2).map((part) => [...part][0] || "").join("").toUpperCase();
  return (PLATFORM_LABELS[safeClass(source)] || "?").slice(0, 1);
}

function createMediaBudget() {
  return { included: 0 };
}

function renderMedia(values, contentKind, media, budget, scopeLabel) {
  const all = Array.isArray(values) ? values : [];
  if (!all.length) return "";
  budget.included += all.length;
  const rendered = all.map((value, index) => {
    const item = value && typeof value === "object" ? value : {};
    const isVideo = item.kind === "video" || (contentKind === "video" && all.length === 1);
    const originalURL = text(item.posterUrl || item.url);
    const alt = text(item.alt || item.description) || `${scopeLabel} image ${index + 1}`;
    const width = safeDimensions(item.width);
    const height = safeDimensions(item.height);
    return `<figure class="source-layout-media-item${isVideo ? " is-video-poster" : ""}">${imageMarkup({ media, url: originalURL, alt, width, height })}${isVideo ? '<figcaption class="source-layout-video-label">Video poster</figcaption>' : ""}</figure>`;
  }).join("");
  return `<div class="source-layout-media post-screenshot-media media-count-${all.length}" data-post-screenshot-media-count="${all.length}">${rendered}</div>`;
}

function renderQuote(quotedPost, source, media, budget) {
  if (!quotedPost || typeof quotedPost !== "object") return "";
  const author = text(quotedPost.author) || "Quoted source";
  const body = text(quotedPost.text);
  const quoteMedia = renderMedia(quotedPost.media, text(quotedPost.contentKind), media, budget, "Quoted post");
  const publishedAt = text(quotedPost.publishedAt);
  const avatarURL = text(quotedPost.avatarUrl);
  const content = body ? `<div class="expandable-text"><p class="expandable-text-copy">${escapeHTML(body)}</p></div>` : "";
  if (!body && !quoteMedia) return "";
  return `<section class="source-quote-card" data-post-screenshot-quote="true">${renderIdentity({ media, avatarURL, author, source, publishedAt })}${content}${quoteMedia}</section>`;
}

function renderAttachments(values, source, media) {
  const attachments = Array.isArray(values) ? values : [];
  if (!attachments.length) return "";
  const fallback = (PLATFORM_LABELS[safeClass(source)] || "?").slice(0, 1);
  return `<div class="source-layout-attachments">${attachments.map((value) => {
    const attachment = value && typeof value === "object" ? value : {};
    const title = text(attachment.title) || "Attachment";
    const subtitle = [text(attachment.subtitle), text(attachment.detail)].filter(Boolean).join(" · ");
    const domain = text(attachment.domain || attachment.actionLabel) || "Attachment preview";
    const imageURL = text(attachment.imageUrl);
    const preview = mappedImage(media, imageURL)
      ? imageMarkup({ media, url: imageURL, alt: "", className: "post-screenshot-attachment-image" })
      : imageURL
        ? '<div class="post-screenshot-media-unavailable post-screenshot-attachment-unavailable" role="img" aria-label="Attachment image preview unavailable"><span>Preview unavailable</span></div>'
        : `<span class="source-layout-attachment-fallback" aria-hidden="true">${escapeHTML(fallback)}</span>`;
    return `<div class="source-layout-attachment">${preview}<span class="source-layout-attachment-copy"><strong>${escapeHTML(title)}</strong>${subtitle ? `<span>${escapeHTML(subtitle)}</span>` : ""}<small>${escapeHTML(domain)}</small></span></div>`;
  }).join("")}</div>`;
}

function renderEngagement(value) {
  if (!value || typeof value !== "object") return "";
  const entries = Object.entries(value).filter(([, count]) => count !== null && count !== undefined && count !== "");
  if (!entries.length) return "";
  return `<footer class="source-layout-engagement">${entries.map(([label, count]) => `<span>${escapeHTML(label.replaceAll("_", " "))}: ${escapeHTML(count)}</span>`).join("")}</footer>`;
}

function safeStylesheet(css) {
  return String(css ?? "").replace(/</g, "\\3C ");
}

/**
 * Render a complete, self-contained source-card document for screenshot capture.
 * The worker injects only downloaded raster image data URLs through `media`.
 */
export function buildPostScreenshotHTML(entry, { css = "", theme = "dark", media = {} } = {}) {
  const warnings = [];
  const chosenTheme = theme === "light" ? "light" : "dark";
  const source = safeClass(entry?.source);
  const evidence = entry?.evidence && typeof entry.evidence === "object" ? entry.evidence : {};
  const item = entry?.item && typeof entry.item === "object" ? entry.item : {};
  const presentation = evidence.presentation && typeof evidence.presentation === "object" ? evidence.presentation : {};
  const author = text(evidence.author || item.author);
  const publishedAt = text(presentation.timestampText || evidence.publishedAt || item.publishedAt);
  const headline = text(presentation.headline);
  const avatarURL = text(evidence.avatarUrl);
  const budget = createMediaBudget();
  const hasCapturedPostText = typeof evidence.text === "string";
  const postText = hasCapturedPostText ? evidence.text : "";
  const socialContext = text(presentation.socialContext);
  const socialContextAbove = ["linkedin", "facebook", "instagram"].includes(source);
  const sourceContentClass = source === "linkedin" ? "professional" : "social";
  const postBody = [
    socialContext && !socialContextAbove ? `<p class="source-social-context-content">${escapeHTML(socialContext)}</p>` : "",
    postText.trim() ? `<div class="expandable-text"><p class="expandable-text-copy">${escapeHTML(postText)}</p></div>` : "",
    !hasCapturedPostText ? '<p class="post-screenshot-unavailable-text">No post text was captured.</p>' : "",
    renderQuote(evidence.quotedPost, source, media, budget),
  ].filter(Boolean).join("");
  const postMedia = renderMedia(evidence.media, text(evidence.contentKind), media, budget, "Post");
  const attachments = renderAttachments(evidence.attachments, source, media);
  const platform = PLATFORM_LABELS[source] || source.replaceAll("_", " ");
  const socialAvatar = text(presentation.socialContextAvatarUrl);
  const socialHeader = socialContext && socialContextAbove
    ? `<div class="source-social-context-above">${mappedImage(media, socialAvatar) ? imageMarkup({ media, url: socialAvatar, alt: "" }) : ""}<span>${escapeHTML(socialContext)}</span></div>`
    : "";
  const root = `<article class="source-layout-card source-${source}" data-freshness="unknown" data-post-screenshot-export-root="true" data-post-screenshot-source="${escapeHTML(source)}" data-post-screenshot-media-count="${budget.included}">${socialHeader}${renderIdentity({ media, avatarURL, author, source, publishedAt, headline })}<div class="post-screenshot-platform">${escapeHTML(platform)}</div>${postBody ? `<div class="source-layout-content source-${sourceContentClass}-content">${postBody}</div>` : ""}${attachments}${postMedia}${renderEngagement(evidence.engagement)}</article>`;
  const html = `<!doctype html><html lang="en" data-post-screenshot-document="true" data-post-screenshot-theme="${chosenTheme}"><head><meta charset="utf-8"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; img-src data:; style-src 'unsafe-inline'; base-uri 'none'; form-action 'none'"><meta name="viewport" content="width=device-width, initial-scale=1"><title>Captured post screenshot</title><style>${safeStylesheet(css)}
:root[data-post-screenshot-theme="dark"] { color-scheme: dark; --page: #080b09; --panel: #171d1a; --text: #f2f5f2; --muted: #bdc6bf; --quiet: #94a197; --line: #303a33; --line-strong: #46534a; --accent: #b7ff5e; }
:root[data-post-screenshot-theme="light"] { color-scheme: light; --page: #f4f6f3; --panel: #ffffff; --text: #18201a; --muted: #46534a; --quiet: #637066; --line: #d8ded9; --line-strong: #c6d0c8; --accent: #416e17; }
html { background: var(--page); color: var(--text); }
body { box-sizing: border-box; width: 720px; max-width: 100%; min-height: 0; margin: 0 auto; padding: 20px; background: var(--page); color: var(--text); font-family: Inter, ui-sans-serif, system-ui, sans-serif; }
[data-post-screenshot-export-root] { position: relative; overflow: hidden; border: 1px solid var(--line-strong); border-radius: 16px; background: var(--panel); }
[data-post-screenshot-export-root] .source-layout-content { padding: 4px 18px 18px; }
[data-post-screenshot-export-root] .source-layout-content p { white-space: pre-wrap; overflow-wrap: anywhere; }
[data-post-screenshot-export-root] .source-layout-media { display: grid !important; grid-template-columns: minmax(0, 1fr) !important; gap: 10px; margin: 8px 18px 16px; background: transparent; }
[data-post-screenshot-export-root] .source-layout-media-item { display: block; margin: 0; overflow: visible; background: transparent; }
[data-post-screenshot-export-root] .source-layout-media-item img { display: block; width: auto; max-width: 100%; height: auto; max-height: none; margin: 0 auto; object-fit: contain; }
[data-post-screenshot-export-root] .source-layout-avatar { width: 42px; height: 42px; flex: 0 0 auto; border-radius: 50%; object-fit: cover; }
[data-post-screenshot-export-root] .source-layout-attachments { margin: 0 18px 16px; }
[data-post-screenshot-theme="light"] .source-x { background: #f7f9f7; }
[data-post-screenshot-theme="light"] .source-linkedin, [data-post-screenshot-theme="light"] .source-facebook, [data-post-screenshot-theme="light"] .source-instagram { background: #ffffff; }
[data-post-screenshot-export-root] .source-layout-attachment .post-screenshot-attachment-image { width: 74px; height: 72px; object-fit: contain; }
[data-post-screenshot-export-root] .source-layout-attachment > .post-screenshot-attachment-unavailable { width: 74px; min-width: 74px; min-height: 72px; padding: 5px; font-size: 0.62rem; text-align: center; }
.post-screenshot-platform { position: absolute; top: 16px; right: 18px; color: var(--quiet); font-size: 0.76rem; font-weight: 700; }
.post-screenshot-media-unavailable { display: grid; min-height: 112px; place-items: center; padding: 14px; border: 1px dashed var(--line-strong); border-radius: 8px; color: var(--quiet); background: color-mix(in srgb, var(--panel) 84%, var(--text) 16%); font-size: 0.82rem; }
.post-screenshot-truncation-warning, .post-screenshot-unavailable-text { margin: 12px 18px; color: var(--quiet); font-size: 0.78rem; line-height: 1.45; }
.post-screenshot-attachment-image { width: 74px; height: 72px; object-fit: contain; }
.source-layout-video-label { display: block; padding: 5px 0; color: var(--quiet); font-size: 0.72rem; }
@media (max-width: 720px) { body { width: auto; padding: 12px; } }
</style></head><body data-post-screenshot-export-height="document"><main data-post-screenshot-theme="${chosenTheme}" data-post-screenshot-width="800">${root}</main></body></html>`;
  return { html, warnings };
}
