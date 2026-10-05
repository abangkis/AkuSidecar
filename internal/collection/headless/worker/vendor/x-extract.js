// This PoC collector reuses the real adapter; it is not the production admission pipeline.
(() => {
  const primitives = globalThis.AkuCapturePrimitives;
  if (!primitives) throw new Error('Shared capture primitives were not loaded.');
  const compactText = value => String(typeof value === 'object' && value !== null
    ? value.innerText ?? value.textContent ?? '' : value ?? '').replace(/\s+/g, ' ').trim();
  const normalizeHttpUrl = value => {
    try { const u = new URL(value, location.href); return /^https?:$/.test(u.protocol) ? u.href : null; }
    catch { return null; }
  };
  const structuredText = primitives.structuredText;
  const helpers = { compactText, structuredText, normalizeHttpUrl, uniqueElements: values => [...new Set(values)] };
  const expansionStates = new WeakMap();
  const ownControls = (container, quote, policy) => [...container.querySelectorAll(policy.buttonSelector)]
    .filter(button => !quote?.contains(button));
  async function expandDetail(adapter, container) {
    if (globalThis.AkuHeadlessCapturePolicy?.allowContentExpansion === false) return;
    const quote = adapter.findQuotedRoot(container);
    const ownLink = [...container.querySelectorAll('time')].find(t => !quote?.contains(t))?.closest('a[href]')?.href;
    if (location.hostname === 'x.com' && canonical(location.href) !== canonical(ownLink)) return;
    if (expansionStates.has(container)) return;
    const expansion = await primitives.expandContent({ container, adapter, readText: structuredText,
      route: () => location.href, deadlineAt: globalThis.AkuHeadlessCapturePolicy?.deadlineAt ?? Infinity });
    const state = expansion.state === 'expanded_no_restore_control' ? 'expanded'
      : expansion.state === 'route_changed' ? 'navigation_changed'
      : expansion.state === 'no_collapse_observed' ? 'visible_text_no_collapse_control' : expansion.state;
    expansionStates.set(container, state);
  }
  const canonical = primitives.canonicalizeXPermalink;
  function findMedia(adapter, container, excludeRoot = null) {
    const records = [];
    const seen = new Set();
    // Reuse the adapter's attachment selectors; avatars and inline emoji are excluded.
    const elements = [...container.querySelectorAll('video'), ...container.querySelectorAll(adapter.imageSelector)];
    for (const element of elements) {
      if (excludeRoot?.contains(element)) continue;
      const video = element.tagName === 'VIDEO';
      const posterImage = !video && Boolean(element.closest(adapter.mediaRendering.videoRootSelector));
      const url = normalizeHttpUrl(video ? element.poster : element.currentSrc || element.src);
      if (!url || !adapter.mediaHosts.includes(new URL(url).hostname)) continue;
      // Detail video controls can nest the author's avatar under the video root.
      // Its trusted CDN host alone does not make it an attachment or video poster.
      const mediaURL = new URL(url);
      if (mediaURL.hostname === 'pbs.twimg.com' && mediaURL.pathname.startsWith('/profile_images/')) continue;
      const kind = video || posterImage ? 'video_poster' : 'image';
      const key = `${kind}:${url}`;
      if (seen.has(key)) continue;
      seen.add(key);
      records.push({ kind, url, alt: element.alt || '',
        loaded: video ? element.readyState > 0 : element.complete && element.naturalWidth > 0,
        loadedMeaning: video ? 'video_metadata_ready' : 'image_decoded',
        width: video ? element.videoWidth : element.naturalWidth,
        height: video ? element.videoHeight : element.naturalHeight,
        ...(video ? { readyState: element.readyState, paused: element.paused, currentTime: element.currentTime,
          duration: Number.isFinite(element.duration) ? element.duration : null,
          sourceKind: element.currentSrc?.startsWith('blob:') ? 'blob' : element.currentSrc ? 'direct' : 'unset' } : {}),
      });
      if (records.length >= 20) break;
    }
    return records;
  }
  function mediaEvidence(media, expected) {
    const expectedWithoutUrl = expected.filter(kind => !media.some(m => kind === 'video' ? m.kind === 'video_poster' : m.kind === kind));
    return { status: expectedWithoutUrl.length ? 'missing_expected_url' : media.length ? 'observed_urls_partial' : 'no_media_observed',
      expectedWithoutUrl, notReady: media.filter(m => !m.loaded).length };
  }
  globalThis.XHeadlessPoC = {
    clickQuote(primaryId) {
      const adapter = globalThis.AkuSourceAdapters.get('x');
      for (const container of adapter.discoverCandidates(helpers).candidates) {
        const quote = adapter.findQuotedRoot(container);
        if (!quote) continue;
        const ownIds = [...container.querySelectorAll('time')].filter(t => !quote.contains(t))
          .map(t => canonical(t.closest('a[href]')?.href)?.match(/\/status\/(\d+)/)?.[1]).filter(Boolean);
        if (new Set(ownIds).size !== 1 || ownIds[0] !== primaryId) continue;
        if (quote.getAttribute('role') !== 'link' && quote.getAttribute('data-testid') !== 'quoteTweet') return false;
        quote.click();
        return true;
      }
      return false;
    },
    async collect() {
      const adapter = globalThis.AkuSourceAdapters.get('x');
      for (const container of adapter.discoverCandidates(helpers).candidates) {
        const box = container.getBoundingClientRect();
        if (box.height > 0 && box.bottom > 0 && box.top < innerHeight) await expandDetail(adapter, container);
      }
      const discovery = adapter.discoverCandidates(helpers);
      let rejected = 0;
      const rejectionReasons = { missing_identity: 0, ambiguous_identity: 0, missing_author: 0, empty_evidence: 0 };
      const posts = discovery.candidates.filter(container => {
        const box = container.getBoundingClientRect();
        return box.height > 0 && box.bottom > 0 && box.top < innerHeight;
      }).flatMap(container => {
        const quote = adapter.findQuotedRoot(container);
        const times = [...container.querySelectorAll('time')].filter(t => !quote?.contains(t));
        const urls = [...new Set(times.map(t => canonical(t.closest('a[href]')?.href)).filter(Boolean))];
        const identity = primitives.resolvePrimaryIdentity({ permalinks: urls });
        const author = adapter.findAuthor(container, helpers);
        if (urls.length !== 1 || !author) {
          rejected++;
          rejectionReasons[!urls.length ? 'missing_identity' : urls.length > 1 ? 'ambiguous_identity' : 'missing_author']++;
          return [];
        }
        const permalink = urls[0];
        const selectedTextRoot = container.querySelector(adapter.contentRootSelector);
        const text = quote?.contains(selectedTextRoot) ? '' : adapter.extractText(container, helpers);
        const semantics = adapter.extractSemantics(container, helpers);
        const avatar = adapter.findAvatar(container, helpers);
        const media = findMedia(adapter, container, quote);
        const mediaExpected = adapter.mediaAcquisition.detectExpectedKinds(container, { ...helpers, excludeRoot: quote });
        const quotedPost = adapter.extractQuotedPost(container, { ...helpers, findMedia: root => findMedia(adapter, root) });
        const textCollapsed = ownControls(container, quote, adapter.contentExpansion)
          .some(button => primitives.expansionLabel(compactText(button), 'more'));
        if (quotedPost) {
          const identity = globalThis.XHeadlessQuoteIdentity.resolve(container, permalink.match(/\/status\/(\d+)/)[1], quotedPost.permalink);
          quotedPost.permalink = identity.permalink;
          quotedPost.identityStatus = identity.status;
          quotedPost.identityProvenance = identity.provenance;
          quotedPost.identityEvidence = identity.evidence;
          semantics.parentPermalink = identity.permalink;
          quotedPost.mediaExpected = [...new Set(adapter.mediaAcquisition.detectExpectedKinds(quote, helpers))];
          quotedPost.mediaEvidence = mediaEvidence(quotedPost.media, quotedPost.mediaExpected);
          quotedPost.textCollapsed = [...quote.querySelectorAll(adapter.contentExpansion.buttonSelector)]
            .some(button => primitives.expansionLabel(compactText(button), 'more'));
          quotedPost.textStatus = quotedPost.textCollapsed ? 'requires_permalink_capture' : 'visible_text_no_collapse_control';
          quotedPost.textCompleteness = primitives.textCompleteness({ collapsed: quotedPost.textCollapsed, textStatus: quotedPost.textStatus });
        }
        if (!text && !media.length && !quote && !mediaExpected.length) { rejected++; rejectionReasons.empty_evidence++; return []; }
        return [{ id: permalink.match(/\/status\/(\d+)/)[1], permalink, author, avatar,
          text, publishedAt: times.find(t => canonical(t.closest('a[href]')?.href) === permalink)?.dateTime || null,
          ...semantics, media, mediaExpected: [...new Set(mediaExpected)], mediaEvidence: mediaEvidence(media, [...new Set(mediaExpected)]),
          quotedPostObserved: Boolean(quote),
          quotedPost, textCollapsed,
          identityComparison: { legacyDecision: 'identified', sharedDecision: identity.status,
            agrees: identity.status === 'identified' && identity.permalink === permalink },
          textCompleteness: primitives.textCompleteness({ collapsed: textCollapsed,
            textStatus: expansionStates.get(container) || (textCollapsed ? 'requires_permalink_capture' : 'visible_text_no_collapse_control') }),
          textStatus: textCollapsed ? expansionStates.get(container) || 'requires_permalink_capture'
            : expansionStates.get(container) || 'visible_text_no_collapse_control',
          limitations: ['visible_dom_only', 'no_production_admission',
            ...(textCollapsed ? ['text_may_be_collapsed'] : []),
            ...(quotedPost?.textCollapsed ? ['quoted_text_may_be_collapsed'] : []),
            ...(quotedPost && !quotedPost.permalink ? ['quoted_identity_unresolved'] : []),
            ...(quotedPost?.mediaExpected.includes('video') || quotedPost?.media.some(m => m.kind === 'video_poster') ? ['quoted_video_stream_not_resolved'] : []),
            ...(mediaExpected.includes('video') ? ['video_stream_not_resolved'] : [])],
        }];
      });
      const loginLink = Boolean(document.querySelector('a[href="/login"], a[href^="/i/flow/login"]'));
      const challenge = Boolean(document.querySelector('iframe[src*="captcha"], iframe[src*="arkoselabs"], #challenge-running'))
        || /verify you are human|unusual activity|authenticate your account/i.test(document.body?.innerText ?? '');
      return { adapterVersion: adapter.version, visibility: document.visibilityState,
        url: location.href, title: document.title, candidateCount: discovery.candidates.length,
        discoveryStrategy: discovery.strategy, rejected, rejectionReasons, posts,
        scroll: { y: scrollY, height: document.scrollingElement?.scrollHeight ?? null, viewportWidth: innerWidth, viewportHeight: innerHeight },
        loginRequired: adapter.loginRequired() || (!posts.length && location.pathname === '/home' && loginLink),
        challengeDetected: challenge, documentReady: document.readyState === 'complete',
        authenticatedUiObserved: Boolean(document.querySelector('[data-testid="SideNav_AccountSwitcher_Button"]')),
      };
    },
  };
})();
