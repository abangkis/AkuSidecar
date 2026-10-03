// Bounded visible-DOM collector for Instagram and LinkedIn using AkuBridge adapters.
// This is experimental evidence collection, not production admission.
(() => {
  const MAX_CANDIDATES = 80;
  const MAX_MEDIA = 20;
  const COMMENT_SELECTOR = '.comments-comment-item, .comments-comment-entity, [data-id^="urn:li:comment:"], [data-comment-id]';
  const compactText = value => String(typeof value === 'object' && value !== null
    ? value.innerText ?? value.textContent ?? '' : value ?? '').replace(/\s+/g, ' ').trim();
  const readNode = node => !node ? '' : node.nodeType === 3 ? node.nodeValue || '' : node.nodeType !== 1 ? ''
    : node.getAttribute?.('data-testid')==='expandable-text-button' ? ''
    : node.tagName === 'IMG' ? node.getAttribute('alt') || '' : node.tagName === 'BR' ? '\n'
      : [...(node.childNodes || [])].map(readNode).join('') + (/^(DIV|P|LI|SECTION|ARTICLE)$/.test(node.tagName) ? '\n' : '');
  const structuredText = value => String(typeof value === 'string' ? value : readNode(value))
    .replace(/\r\n?/g, '\n').split('\n').map(line => line.replace(/[\t\f\v\u00a0 ]+/g, ' ').trim())
    .join('\n').replace(/\n{3,}/g, '\n\n').trim();
  const normalizeHttpUrl = value => {
    try { const url = new URL(value, location.href); return /^https?:$/.test(url.protocol) ? url.href : null; }
    catch { return null; }
  };
  const normalizeHttpsUrl = value => {
    const normalized = normalizeHttpUrl(value);
    try { return normalized && new URL(normalized).protocol === 'https:' ? normalized : null; }
    catch { return null; }
  };
  const uniqueElements = values => [...new Set(values || [])];
  const helpers = { compactText, structuredText, normalizeHttpUrl, normalizeHttpsUrl, uniqueElements };
  const scrollState=globalThis.AkuHeadlessAdapterScrollState ||= {root:null};
  function scrollable(root) {
    if(!root || root===document.scrollingElement || root===document.body) return false;
    const style=typeof getComputedStyle==='function' ? getComputedStyle(root) : null;
    return root.clientHeight>0 && root.scrollHeight>root.clientHeight+2 && /auto|scroll/i.test(style?.overflowY || style?.overflow || '');
  }
  function chooseScrollRoot(adapter,candidates=[]) {
    if(scrollState.root?.isConnected && scrollable(scrollState.root)) return scrollState.root;
    for(const candidate of candidates.filter(isVisible)) {
      for(let root=candidate.parentElement;root && root!==document.body;root=root.parentElement) {
        if(scrollable(root)) {scrollState.root=root;return root;}
      }
    }
    for(const selector of adapter?.scrollRootSelectors || []) {
      for(const root of document.querySelectorAll(selector)) if(scrollable(root)) {scrollState.root=root;return root;}
    }
    scrollState.root=null;return null;
  }
  function scrollInfo(root=scrollState.root) {
    return {y:root ? Math.max(0,Math.trunc(root.scrollTop || 0)) : Number.isFinite(scrollY) ? scrollY : 0,
      height:root ? root.scrollHeight : document.scrollingElement?.scrollHeight ?? null,
      viewportWidth:Number.isFinite(innerWidth) ? innerWidth : null,
      viewportHeight:root ? root.clientHeight : Number.isFinite(innerHeight) ? innerHeight : null,
      context:root ? 'source_container' : 'window'};
  }
  function scrollSourceTo(top) {
    if(!Number.isFinite(top) || top<0) throw Error('invalid_scroll_position');
    const root=chooseScrollRoot(sourceAdapter().adapter);
    (root || window).scrollTo({top,behavior:'instant'});
  }
  function scrollSourceBy(fraction) {
    if(!Number.isFinite(fraction) || fraction<0.1 || fraction>1) throw Error('invalid_scroll_fraction');
    const root=chooseScrollRoot(sourceAdapter().adapter);
    (root || window).scrollBy(0,Math.round((root ? root.clientHeight : innerHeight)*fraction));
  }

  function sourceForHost(hostname) {
    const host = String(hostname || '').toLowerCase();
    if (host === 'instagram.com' || host === 'www.instagram.com') return 'instagram';
    if (host === 'www.linkedin.com') return 'linkedin';
    return null;
  }

  function sourceAdapter() {
    const source = sourceForHost(location.hostname);
    if (!source) return { source: null, adapter: null, reason: 'unsupported_hostname' };
    try {
      const adapter = globalThis.AkuSourceAdapters?.get(source);
      if (adapter && adapter.matchesPage?.()) return { source, adapter, reason: null };
      return { source, adapter: null, reason: 'adapter_unavailable' };
    } catch { return { source, adapter: null, reason: 'adapter_unavailable' }; }
  }

  function parseHttps(value) {
    try {
      const url = new URL(value, location.href);
      if (url.protocol !== 'https:' || url.username || url.password || url.port) return null;
      return url;
    } catch { return null; }
  }

  function instagramIdentity(value) {
    const url = parseHttps(value);
    if (!url || !['instagram.com', 'www.instagram.com'].includes(url.hostname.toLowerCase())) return null;
    const match = url.pathname.match(/^\/(p|reel|tv)\/([A-Za-z0-9_-]+)\/?$/i);
    if (!match) return null;
    const kind = match[1].toLowerCase();
    const shortcode = match[2];
    return { id: `instagram:${kind}:${shortcode}`, permalink: `https://www.instagram.com/${kind}/${shortcode}/` };
  }

  function linkedinIdentity(value) {
    const raw = String(value ?? '').trim();
    const url = /^https?:\/\//i.test(raw) ? parseHttps(raw) : null;
    let match = url && url.hostname.toLowerCase() === 'www.linkedin.com'
      ? url.pathname.match(/^\/feed\/update\/(urn:li:(activity|ugcPost|share):(\d{5,30}))\/?$/i)
        || url.pathname.match(/^\/posts\/[^/]*-(activity|ugcpost|share)-(\d{5,30})(?:-[A-Za-z0-9_-]+)?\/?$/i)
      : null;
    if (match) return linkedinRecord(match.length === 4 ? match[2] : match[1], match.length === 4 ? match[3] : match[2]);
    if (url) return null;
    match = raw.match(/^urn:li:(activity|ugcPost|share):(\d{5,30})$/i)
      || raw.match(/^(activity|ugcPost|share)[:-](\d{5,30})$/i);
    return match ? linkedinRecord(match[1], match[2]) : null;
  }

  function linkedinRecord(kind, nativeId) {
    const normalizedKind = String(kind).toLowerCase();
    const pathKind = normalizedKind === 'ugcpost' ? 'ugcPost' : normalizedKind;
    return {
      id: `linkedin:${normalizedKind}:${nativeId}`,
      permalink: `https://www.linkedin.com/feed/update/urn:li:${pathKind}:${nativeId}/`,
    };
  }

  function postIdentity(source, adapter, container, explicitIdentity = null) {
    const values = [];
    const details = adapter.findPermalinkDetails?.(container, helpers);
    if (details?.url) values.push(details.url);
    const domPermalink = adapter.findDomPermalink?.(container);
    if (domPermalink) values.push(domPermalink);
    if (source === 'linkedin') {
      values.push(container.getAttribute?.('data-urn'), container.getAttribute?.('data-id'));
      for (const element of [...(container.querySelectorAll?.('[data-urn], [data-id], a[href]') || [])].slice(0, 100)) {
        values.push(element.getAttribute?.('data-urn'), element.getAttribute?.('data-id'), element.href,
          element.getAttribute?.('href'));
      }
    } else {
      for (const anchor of container.querySelectorAll?.('a[href]') || []) values.push(anchor.href, anchor.getAttribute?.('href'));
    }
    const identities = new Map();
    for (const value of values) {
      if (!value) continue;
      const identity = source === 'instagram' ? instagramIdentity(value) : linkedinIdentity(value);
      if (identity) identities.set(identity.id, identity);
    }
    if (identities.size === 0 && explicitIdentity) identities.set(explicitIdentity.id, explicitIdentity);
    if (!identities.size) return { reason: 'missing_identity' };
    if (identities.size > 1) return { reason: 'ambiguous_identity' };
    const identity = [...identities.values()][0];
    if (explicitIdentity && identity.id !== explicitIdentity.id) return { reason: 'identity_mismatch' };
    const adapterId = adapter.platformIdFromCandidates?.([identity.permalink]);
    if (adapterId !== identity.id) return { reason: 'identity_mismatch' };
    return { identity, identitySource: explicitIdentity?.provenance || (explicitIdentity && values.every(value => !value || !(source === 'instagram'
      ? instagramIdentity(value) : linkedinIdentity(value))) ? 'explicit_target_route' : explicitIdentity ? 'adapter_recovery' : 'native_dom') };
  }

  function isVisible(element) {
    if (!element?.getBoundingClientRect) return false;
    for (let current = element; current; current = current.parentElement) {
      const style = typeof getComputedStyle === 'function' ? getComputedStyle(current) : null;
      if (style && (style.display === 'none' || style.visibility === 'hidden' || Number(style.opacity) === 0)) return false;
    }
    const rect = element.getBoundingClientRect();
    return rect.width > 0 && rect.height > 0 && rect.bottom > 0 && rect.top < innerHeight
      && rect.right > 0 && rect.left < innerWidth;
  }

  function authorAmbiguous(container, source) {
    if (source !== 'linkedin') return false;
    const authors = new Set([...(container.querySelectorAll?.('button[aria-label^="Open control menu for post by "]') || [])]
      .map(button => compactText(button.getAttribute?.('aria-label')).replace(/^Open control menu for post by\s+/i, '').slice(0, 300))
      .filter(Boolean));
    return authors.size > 1;
  }

  function safeMediaUrl(value, adapter) {
    const url = parseHttps(value);
    if (!url) return null;
    const host = url.hostname.toLowerCase();
    const trusted = (adapter.mediaHosts || []).some(suffix => host === suffix || host.endsWith(`.${suffix}`));
    if (!trusted) return null;
    url.hash = '';
    return url.href;
  }

  function dimensions(element, fallbackRoot) {
    const rect = element?.getBoundingClientRect?.() || {};
    const root = fallbackRoot?.getBoundingClientRect?.() || {};
    return {
      width: Math.max(0, Math.round(rect.width || element?.naturalWidth || element?.videoWidth || root.width || 0)),
      height: Math.max(0, Math.round(rect.height || element?.naturalHeight || element?.videoHeight || root.height || 0)),
    };
  }

  function ownMediaRoot(root, adapter, kind, alt, excluded=null) {
    const videos = uniqueElements([...(root?.tagName === 'VIDEO' ? [root] : []), ...(root?.querySelectorAll?.('video') || [])]);
    const images = uniqueElements([...(root?.tagName === 'IMG' ? [root] : []), ...(root?.querySelectorAll?.('img') || [])]);
    const results = [];
    if (kind === 'video') {
      for (const video of videos) {
        if (!isVisible(video) || excluded?.contains(video)) continue;
        const size = dimensions(video, root);
        const poster = safeMediaUrl(video.currentSrc?.startsWith('blob:') ? video.poster : video.poster || video.getAttribute?.('poster'), adapter);
        const playback = safeMediaUrl(video.currentSrc || video.src || [...(video.querySelectorAll?.('source[src]') || [])].map(source => source.src).find(Boolean), adapter);
        if ((!poster && !playback) || size.width < 180 || size.height < 90) continue;
        results.push({ kind: 'video', url: poster || playback, posterUrl: poster, playbackUrl: playback,
          playbackMode: playback ? 'inline' : 'native', alt: compactText(video.getAttribute?.('aria-label') || alt || 'Video preview').slice(0, 300),
          width: size.width, height: size.height, loaded: video.readyState > 0,
          loadedMeaning: 'video_metadata_ready', sourceKind: video.currentSrc?.startsWith('blob:') ? 'blob' : video.currentSrc ? 'direct' : 'unset',
          readyState: Number.isFinite(video.readyState) ? video.readyState : 0, paused: Boolean(video.paused),
          currentTime: Number.isFinite(video.currentTime) ? video.currentTime : null,
          duration: Number.isFinite(video.duration) ? video.duration : null });
      }
      if (!videos.length) for (const image of images) {
        if (!isVisible(image) || excluded?.contains(image) || adapter.shouldSkipImage?.(image)) continue;
        const size = dimensions(image, root);
        const url = safeMediaUrl(image.currentSrc || image.src || image.getAttribute?.('src'), adapter);
        if (!url || size.width < 180 || size.height < 90) continue;
        results.push({ kind: 'video', url, posterUrl: url, playbackUrl: null, playbackMode: 'native',
          alt: compactText(image.alt || alt || 'Video preview').slice(0, 300), width: size.width, height: size.height,
          loaded: Boolean(image.complete && image.naturalWidth > 0), loadedMeaning: 'image_decoded', sourceKind: 'poster' });
      }
    } else {
      for (const image of images) {
        if (!isVisible(image) || excluded?.contains(image) || adapter.shouldSkipImage?.(image)
          || /profile picture/i.test(image.alt || '')
          || (adapter.source==='linkedin' && image.closest?.('a[href*="/in/"], a[href*="/company/"], a[href*="/school/"], a[href*="/showcase/"]'))) continue;
        const size = dimensions(image, root);
        const url = safeMediaUrl(image.currentSrc || image.src || image.getAttribute?.('src'), adapter);
        if (!url || size.width < 180 || size.height < 90) continue;
        const videoPoster=image.closest?.('[data-vjs-player], [data-view-name*="video" i]');
        results.push({ kind: videoPoster ? 'video' : 'image', url,...(videoPoster ? {posterUrl:url,playbackUrl:null,playbackMode:'native'} : {}),alt: compactText(image.alt || alt || '').slice(0, 300),
          width: size.width, height: size.height, loaded: Boolean(image.complete && image.naturalWidth > 0),
          loadedMeaning: 'image_decoded', sourceKind: image.currentSrc ? 'current_src' : image.src ? 'src_property' : 'src_attribute' });
      }
    }
    return results;
  }

  function normalizeMediaCandidate(candidate, adapter) {
    if (!candidate || typeof candidate !== 'object') return null;
    const video = candidate.kind === 'video' || candidate.kind === 'video_poster';
    const posterUrl = video ? safeMediaUrl(candidate.posterUrl || candidate.url, adapter) : null;
    const playbackUrl = video ? safeMediaUrl(candidate.playbackUrl, adapter) : null;
    const url = video ? posterUrl || playbackUrl : safeMediaUrl(candidate.url, adapter);
    if (!url) return null;
    const width = Math.max(0, Math.min(8192, Math.round(Number(candidate.width) || 0)));
    const height = Math.max(0, Math.min(8192, Math.round(Number(candidate.height) || 0)));
    if (width < 180 || height < 90) return null;
    return { kind: video ? 'video' : 'image', url, ...(video ? { posterUrl, playbackUrl, playbackMode: playbackUrl ? 'inline' : 'native' } : {}),
      alt: compactText(candidate.alt).slice(0, 300), width, height, loaded: candidate.loaded === true,
      loadedMeaning: video ? 'video_metadata_ready' : 'image_decoded',
      ...(video ? { sourceKind: ['blob', 'direct', 'unset', 'poster'].includes(candidate.sourceKind) ? candidate.sourceKind : 'unset',
        readyState: Number.isSafeInteger(candidate.readyState) ? candidate.readyState : undefined,
        paused: typeof candidate.paused === 'boolean' ? candidate.paused : undefined,
        currentTime: Number.isFinite(candidate.currentTime) ? candidate.currentTime : null,
        duration: Number.isFinite(candidate.duration) ? candidate.duration : null } : {}) };
  }

  function unavailable(value) {
    if (value === true) return true;
    if (value === false || value === null || value === undefined) return false;
    if (typeof value === 'string') return /^(?:unavailable|not_available|blocked)$/i.test(value.trim());
    if (typeof value === 'object') {
      if (typeof value.unavailable === 'boolean') return value.unavailable;
      if (typeof value.available === 'boolean') return !value.available;
      return /^(?:unavailable|not_available|blocked)$/i.test(String(value.status || value.state || ''));
    }
    return false;
  }

  function mediaFor(source, adapter, container, excluded) {
    const mediaHelpers = { ...helpers, excludeRoot: excluded, collectRootCandidates: (root, options) =>
      excluded.contains(root) ? [] : ownMediaRoot(root, adapter, options?.kind, options?.alt,excluded) };
    let expected = uniqueElements(adapter.mediaAcquisition.detectExpectedKinds(container, mediaHelpers))
      .filter(kind => ['image', 'video', 'document'].includes(kind)).slice(0, 8);
    const recovered = adapter.mediaAcquisition.extractCandidates(container, mediaHelpers);
    // Initial Bridge capture inspects owned DOM images/videos, independently of
    // recovery-only legacy selectors. Keep the same exclusions here.
    const raw=[...(Array.isArray(recovered) ? recovered : []),...ownMediaRoot(container,adapter,'image','',excluded),
      ...([...container.querySelectorAll('video')].some(video=>!excluded.contains(video) && isVisible(video))
        ? ownMediaRoot(container,adapter,'video','',excluded) : [])];
    const media = [];
    const seen = new Set();
    for (const candidate of Array.isArray(raw) ? raw : []) {
      const normalized = normalizeMediaCandidate(candidate, adapter);
      if (!normalized) continue;
      const key = `${normalized.kind}:${normalized.url}`;
      if (seen.has(key)) continue;
      seen.add(key); media.push(normalized);
      if (media.length >= MAX_MEDIA) break;
    }
    expected=uniqueElements([...expected,...media.map(item=>item.kind)]);
    const expectedWithoutUrl = expected.filter(kind => kind === 'video'
      ? !media.some(entry => entry.kind === 'video')
      : kind === 'image' ? !media.some(entry => entry.kind === 'image') : true);
    const playerIds = source === 'linkedin'
      ? uniqueElements([...(container.querySelectorAll?.('[data-vjs-player][id]') || [])]
        .filter(player => !excluded.contains(player) && isVisible(player))
        .map(player => String(player.id || player.getAttribute?.('id') || '').trim().slice(0, 240)).filter(Boolean)).slice(0, 16)
      : [];
    return { expected, media, evidence: {
      status: expectedWithoutUrl.length ? 'missing_expected_url' : media.length ? 'observed_urls_partial' : 'no_media_observed',
      expectedWithoutUrl, notReady: media.filter(entry => !entry.loaded).length,
      ...(source === 'linkedin' ? { playerIds } : {}),
      videoStreamStatus: expected.includes('video') ? media.some(entry => entry.kind === 'video' && entry.playbackUrl)
        ? 'direct_dom_playback_observed' : 'unknown' : 'not_expected',
    } };
  }

  function makeExcluded(container, otherCandidates) {
    return { contains(node) {
      if (!node || !container.contains?.(node)) return true;
      if (node.closest?.(COMMENT_SELECTOR)) return true;
      return otherCandidates.some(other => other !== container && container.contains?.(other) && other.contains?.(node));
    } };
  }

  function sourceOwnedAnchor(anchor, excluded) { return !excluded.contains(anchor); }

  function linkedinUnwrappedUrl(value) {
    const direct = normalizeHttpsUrl(value);
    if (!direct) return null;
    try {
      const url = new URL(direct);
      if (url.hostname !== 'www.linkedin.com' || !/^\/safety\/go\/?$/i.test(url.pathname)) return direct;
      const target = normalizeHttpsUrl(url.searchParams.get('url'));
      if (!target) return null;
      const host = new URL(target).hostname.toLowerCase();
      return host === 'linkedin.com' || host.endsWith('.linkedin.com') ? null : target;
    } catch { return null; }
  }

  function attachmentLinks(container, source, adapter, excluded, permalink, capturedAt) {
    if (typeof adapter.extractAttachments !== 'function') return [];
    const ownedUrls = new Set();
    for (const anchor of container.querySelectorAll?.('a[href]') || []) {
      if (!sourceOwnedAnchor(anchor, excluded)) continue;
      const href = source === 'linkedin' ? linkedinUnwrappedUrl(anchor.href) : normalizeHttpsUrl(anchor.href);
      if (href) ownedUrls.add(href);
    }
    const values = adapter.extractAttachments(container, { ...helpers, capturedAt, permalink }) || [];
    return values.filter(value => value && typeof value.url === 'string' && ownedUrls.has(value.url))
      .slice(0, 8).map(value => ({
        ...value,
        kind: ['job', 'link_preview', 'document'].includes(value.kind) ? value.kind : 'link_preview',
        title: compactText(value.title).slice(0, 300), url: normalizeHttpsUrl(value.url),
        ...(value.imageUrl ? { imageUrl: safeMediaUrl(value.imageUrl, adapter) } : {}),
      })).filter(value => value.url);
  }

  function linksFor(container, excluded) {
    const root = container.querySelector?.('[data-testid="expandable-text-box"]') || container;
    const links = [];
    const seen = new Set();
    for (const anchor of root.querySelectorAll?.('a[href]') || []) {
      if (!sourceOwnedAnchor(anchor, excluded)) continue;
      const href = normalizeHttpUrl(anchor.href);
      if (!href || seen.has(href)) continue;
      seen.add(href);
      links.push({ text: compactText(anchor.innerText || anchor.textContent).slice(0, 300), href });
      if (links.length >= 10) break;
    }
    return links;
  }

  function timestamp(container, adapter, presentation, excluded, capturedAt) {
    const values = uniqueElements([...(container.querySelectorAll?.('time[datetime]') || [])]
      .filter(time => !excluded.contains(time)).map(time => String(time.getAttribute?.('datetime') || time.dateTime || ''))
      .filter(value => Number.isFinite(Date.parse(value)) && Date.parse(value) <= Date.parse(capturedAt) + 60_000));
    if (values.length === 1) return { publishedAt: new Date(values[0]).toISOString(), source: 'native_datetime', estimated: false, precision: 'exact' };
    if (values.length > 1) return { publishedAt: null, source: 'conflicting_native_time', estimated: false, precision: 'unknown' };
    const estimate = adapter.estimateRelativeTimestamp?.(presentation?.timestampText, capturedAt);
    if (estimate?.publishedAt && Number.isFinite(Date.parse(estimate.publishedAt))) return {
      publishedAt: new Date(estimate.publishedAt).toISOString(), source: 'relative_text_estimate', estimated: true,
      precision: estimate.precision || 'unknown',
    };
    return { publishedAt: null, source: presentation?.timestampAvailability === 'not_exposed_promoted' ? 'not_exposed_promoted' : 'unavailable', estimated: false, precision: 'unknown' };
  }

  function boundedObject(value) { return value && typeof value === 'object' && !Array.isArray(value) ? value : {}; }

  function extractPost(source, adapter, container, allCandidates, explicitIdentity = null) {
    if (!isVisible(container)) return { rejection: 'not_visible' };
    const result = postIdentity(source, adapter, container, explicitIdentity);
    if (!result.identity) return { rejection: result.reason };
    if (authorAmbiguous(container, source)) return { rejection: 'ambiguous_author' };
    const author = compactText(adapter.findAuthor(container, helpers)).slice(0, 300);
    if (!author) return { rejection: 'missing_author' };
    const excluded = makeExcluded(container, allCandidates);
    const identity = result.identity;
    const rawText = structuredText(adapter.extractText?.(container, helpers) ?? '');
    const textCharacters = Array.from(rawText);
    const textTruncated = textCharacters.length > 4000;
    const text = textCharacters.slice(0, 4000).join('');
    const semantics = boundedObject(adapter.extractSemantics(container, helpers));
    const presentation = boundedObject(adapter.extractPresentation?.(container, helpers));
    const media = mediaFor(source, adapter, container, excluded);
    const attachments = attachmentLinks(container, source, adapter, excluded, identity.permalink, new Date().toISOString());
    const avatar = safeMediaUrl(adapter.findAvatar?.(container, helpers), adapter);
    const capturedAt = new Date().toISOString();
    const time = timestamp(container, adapter, presentation, excluded, capturedAt);
    presentation.timestampSource = time.source;
    presentation.timestampEstimated = time.estimated;
    presentation.timestampPrecision = time.precision;
    presentation.permalinkSource = result.identitySource === 'explicit_target_route' ? 'explicit_target_route'
      : result.identitySource === 'adapter_recovery' ? 'adapter_recovery' : source === 'instagram' ? 'native_post_anchor' : 'native_urn_or_permalink';
    const contentRoot = container.querySelector?.(adapter.contentRootSelector || '[data-testid="expandable-text-box"]');
    const controls = contentRoot ? [...contentRoot.querySelectorAll?.(adapter.contentExpansion?.buttonSelector || 'button,[role="button"]') || []]
      .some(button => /^(?:…\s*)?(?:more|show more|see more)$/i.test(compactText(button))) : false;
    const directContext = typeof adapter.extractDirectContext === 'function'
      ? adapter.extractDirectContext(container, { ...helpers, permalink: identity.permalink, quotedPost: null }) : [];
    const links = linksFor(container, excluded);
    if (!text && !media.media.length && !attachments.length) return { rejection: 'empty_evidence' };
    return { post: {
      id: identity.id, permalink: identity.permalink, author, avatar, text, publishedAt: time.publishedAt,
      ...semantics, presentation, attachments, links, directContext: Array.isArray(directContext) ? directContext.slice(0, 8) : [],
      media: media.media, mediaExpected: media.expected, mediaEvidence: {...media.evidence,
        ...(source==='linkedin' ? {playerIds:uniqueElements([...(container.querySelectorAll?.('[data-vjs-player]') || [])]
          .filter(root=>!excluded.contains(root) && isVisible(root)).map(root=>root.id).filter(id=>typeof id==='string' && id.length>0 && id.length<=240)).slice(0,16)} : {})},
      textCollapsed: controls, textTruncated, textStatus: controls ? 'visible_text_may_be_collapsed' : 'visible_text_no_collapse_control',
      limitations: ['visible_dom_only', 'no_production_admission', ...(controls ? ['text_may_be_collapsed'] : []),
        ...(textTruncated ? ['text_truncated'] : []),
        ...(media.expected.includes('video') && media.evidence.videoStreamStatus === 'unknown' ? ['video_stream_not_resolved'] : [])],
    } };
  }

  async function collect() {
    const selected = sourceAdapter();
    const source = selected.source;
    const adapter = selected.adapter;
    const base = { source, adapterVersion: adapter?.version || null, visibility: document.visibilityState || 'unknown',
      url: location.href, title: String(document.title || '').slice(0, 300), posts: [], candidateCount: 0,
      discoveryStrategy: 'none', rejected: 0, rejectionReasons: {}, documentReady: document.readyState === 'complete',
      loginRequired: false, challengeDetected: false, sourceUnavailable: false,
      scroll: { y: Number.isFinite(scrollY) ? scrollY : 0, height: document.scrollingElement?.scrollHeight ?? null,
        viewportWidth: Number.isFinite(innerWidth) ? innerWidth : null, viewportHeight: Number.isFinite(innerHeight) ? innerHeight : null } };
    if (!adapter) {
      base.sourceUnavailable = selected.reason === 'unsupported_hostname' ? 'unsupported_hostname' : false;
      base.rejectionReasons.adapter_unavailable = selected.reason === 'adapter_unavailable' ? 1 : 0;
      return base;
    }
    let discovery;
    try { discovery = adapter.discoverCandidates(helpers) || { candidates: [], strategy: 'none' }; }
    catch { discovery = { candidates: [], strategy: 'adapter_error' }; }
    let candidates = uniqueElements(discovery.candidates).filter(Boolean);
    const bindings=new WeakMap();
    let targetFallbackStatus='not_needed';
    if(source==='instagram' && candidates.length===0) {
      const route=instagramIdentity(location.href);
      if(route) {
        const roots=uniqueElements([...(discovery.readinessCandidates || []),...document.querySelectorAll('main article, [role="dialog"] article')])
          .filter(root=>isVisible(root) && compactText(adapter.findAuthor(root,helpers)));
        targetFallbackStatus=roots.length===1 ? 'single_native_article' : roots.length ? 'ambiguous' : 'no_native_article';
        if(roots.length===1) {candidates=roots;bindings.set(roots[0],{...route,provenance:'explicit_target_route'});}
      }
    }
    const bounded = candidates.slice(0, MAX_CANDIDATES);
    base.scroll=scrollInfo(chooseScrollRoot(adapter,bounded));
    if(source==='linkedin' && typeof adapter.recoverPermalinks==='function') {
      const missing=bounded.filter(root=>isVisible(root) && !authorAmbiguous(root,source)
        && compactText(adapter.findAuthor(root,helpers)) && postIdentity(source,adapter,root).reason==='missing_identity');
      if(missing.length) {
        const deadline=Date.now()+5000;
        try {
          const recovered=await adapter.recoverPermalinks(missing,deadline,{...helpers,isVisibleInViewport:isVisible,
            findPermalinkDetails:root=>{const identity=postIdentity(source,adapter,root).identity;return identity ? {url:identity.permalink} : null;},
            waitForValue:async(fn,attempts,interval)=>{for(let n=0;n<attempts && Date.now()<deadline;n++){const value=fn();if(value)return value;await new Promise(r=>setTimeout(r,Math.min(interval,Math.max(0,deadline-Date.now()))));}return null;}});
          for(const root of missing) {const identity=linkedinIdentity(recovered?.get(root)?.url);if(identity)bindings.set(root,{...identity,provenance:'adapter_recovery'});}
        } catch { /* A missing permalink remains rejected rather than inferred. */ }
      }
    }
    const rejectionReasons = { not_visible: 0, missing_identity: 0, ambiguous_identity: 0, identity_mismatch: 0,
      ambiguous_author: 0, missing_author: 0, empty_evidence: 0, conflicting_post_binding: 0 };
    const records = [];
    for (const candidate of bounded) {
      try {
        const result = extractPost(source, adapter, candidate, bounded,bindings.get(candidate));
        if (result.post) records.push({ container: candidate, post: result.post });
        else rejectionReasons[result.rejection] = (rejectionReasons[result.rejection] || 0) + 1;
      } catch { rejectionReasons.adapter_error = (rejectionReasons.adapter_error || 0) + 1; }
    }
    const byId = new Map();
    for (const entry of records) {
      const group = byId.get(entry.post.id) || [];
      group.push(entry); byId.set(entry.post.id, group);
    }
    const posts = [];
    for (const group of byId.values()) {
      const binding = new Set(group.map(({ post }) => `${post.author}\0${post.text}`));
      if (binding.size > 1) { rejectionReasons.conflicting_post_binding += group.length; continue; }
      group.sort((left, right) => evidenceScore(right.post) - evidenceScore(left.post));
      posts.push(group[0].post);
      rejectionReasons.duplicate_candidate = (rejectionReasons.duplicate_candidate || 0) + group.length - 1;
    }
    let loginRequired = false;
    try { loginRequired = adapter.loginRequired?.() === true; } catch { loginRequired = false; }
    let sourceUnavailable = false;
    try { sourceUnavailable = unavailable(adapter.availability?.()); } catch { sourceUnavailable = false; }
    const challengeText = String(document.body?.innerText || '').slice(0, 8000);
    const challengeDetected = /\/checkpoint|\/challenge/i.test(location.pathname)
      || Boolean(document.querySelector('iframe[src*="captcha" i], [data-test-id*="challenge" i], #challenge-running, form[action*="checkpoint" i]'))
      || (posts.length === 0 && /^(?:verify you are human|unusual activity detected|security check required|robot check|captcha)$/im.test(challengeText.trim()));
    return { ...base, candidateCount: candidates.length, discoveryStrategy: discovery.strategy || 'unknown',
      candidateDiagnostics:{structuralCandidates:discovery.readinessCandidates?.length ?? discovery.semanticCandidateCount ?? 0,
        eligibleCandidates:candidates.length,actionAnchoredCandidates:discovery.actionAnchoredCandidateCount || 0,targetFallbackStatus,
        permalinkRecoveryCount:bounded.filter(root=>bindings.get(root)?.provenance==='adapter_recovery').length},
      candidatesTruncated: candidates.length > MAX_CANDIDATES, rejected: Object.values(rejectionReasons).reduce((sum, count) => sum + count, 0),
      rejectionReasons, posts, loginRequired, sourceUnavailable, challengeDetected,
      authenticatedUiObserved: source === 'instagram'
        ? Boolean(document.querySelector('a[href="/accounts/edit/"], a[href^="/direct/inbox"]'))
        : Boolean(document.querySelector('button[aria-label*="Me" i], a[href^="/mynetwork/"]')) };
  }

  function evidenceScore(post) {
    return post.text.length + post.media.length * 500 + post.attachments.length * 300 + post.links.length * 10;
  }

  globalThis.XHeadlessPoC = {
    collect,
    scrollSourceTo,
    scrollSourceBy,
    prepareEvidenceTargets() { return []; },
    recordHoverEvidence() { return false; },
  };
})();
