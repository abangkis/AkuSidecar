// Experimental Facebook collector using the existing adapter, not production admission.
(() => {
  const compactText = value => String(typeof value === 'object' && value !== null
    ? value.innerText ?? value.textContent ?? '' : value ?? '').replace(/\s+/g, ' ').trim();
  const readNode = (node, owner=globalThis.FacebookHeadlessBoundary.owner(node?.nodeType===1?node:node?.parentElement)) => !node ? '' : node.nodeType === 3 ? node.nodeValue || '' : node.nodeType !== 1 ? ''
    : owner && !globalThis.FacebookHeadlessBoundary.owns(node,owner) ? ''
    : node.getAttribute('role') === 'button' && /^(see more|see less|show more|show less)$/i.test(compactText(node)) ? ''
      : node.tagName === 'IMG' ? node.getAttribute('alt') || '' : node.tagName === 'BR' ? '\n'
      : [...node.childNodes].map(child=>readNode(child,owner)).join('') + (/^(DIV|P|LI|SECTION|ARTICLE)$/.test(node.tagName) ? '\n' : '');
  const structuredText = value => String(typeof value === 'string' ? value : readNode(value))
    .replace(/\r\n?/g, '\n').split('\n').map(line => line.replace(/[\t\f\v\u00a0 ]+/g, ' ').trim()).join('\n').replace(/\n{3,}/g, '\n\n').trim();
  const normalizeHttpUrl = value => { try { const u = new URL(value, location.href); return /^https?:$/.test(u.protocol) ? u.href : null; } catch { return null; } };
  const helpers = { compactText, structuredText, normalizeHttpUrl, uniqueElements: values => [...new Set(values)] };
  const delay = ms => new Promise(resolve => setTimeout(resolve, ms));
  const state = globalThis.FacebookHeadlessEvidenceState ||= { cache: new WeakMap(), attempts: new WeakMap(), targets: new Map(), sequence: 0 };
  const boundary = globalThis.FacebookHeadlessBoundary;
  if(!boundary)throw new Error('Facebook PoC ownership boundary was not loaded.');
  const fingerprint = (adapter, container) => adapter.findAuthor(container, helpers) + ':' + adapter.extractText(container, helpers).slice(0, 120);
  function ownPostElement(element, container) {
    return boundary.owns(element,container);
  }
  function timestampAnchors(container) {
    return [...container.querySelectorAll('a,[role="link"], abbr[data-utime], time')].filter(anchor => {
      if (!ownPostElement(anchor, container)) return false;
      if (anchor.closest('h2,h3,h4,strong') || anchor.querySelector('img,video')) return false;
      if (anchor.closest('[data-ad-preview="message"], [data-ad-comet-preview="message"]'))return false;
      const rect = anchor.getBoundingClientRect();
      return rect.width > 0 && rect.height > 0 && rect.width < 250 && rect.height < 80 && rect.top >= 0 && rect.bottom <= innerHeight
        && (anchor.tagName !== 'A' || anchor.target === '_blank' || anchor.hasAttribute('aria-describedby')
          || /\/posts\/|story_fbid=|\/permalink\//.test(anchor.href) || compactText(anchor).length <= 80);
    }).sort((left, right) => Number(!right.getAttribute('href')) - Number(!left.getAttribute('href'))
      || Number(right.target === '_blank') - Number(left.target === '_blank')
      || Number(right.tagName !== 'A') - Number(left.tagName !== 'A')).slice(0, 3);
  }
  function relativeTime(value, adapter, capturedAt) {
    const text = compactText(value).replace(/[\u034f\u200b-\u200d\u2060\ufeff]/g, '');
    const match = text.match(/^(\d{1,3})\s*(minutes?|mins?|hours?|hrs?|days?|weeks?|years?|m|h|d|w|y)(?:\s+ago)?$/i);
    if (!match) return null;
    const unit = /^(m|min)/i.test(match[2]) ? 'm' : /^(h|hr)/i.test(match[2]) ? 'h' : match[2][0].toLowerCase();
    return adapter.estimateRelativeTimestamp(`${match[1]}${unit}`, capturedAt);
  }
  function timestampEvidence(container, adapter, capturedAt) {
    const own = element => ownPostElement(element, container);
    const nativeValues = [...container.querySelectorAll('time[datetime]')].filter(own).map(element => element.getAttribute('datetime'))
      .filter(value => /^\d{4}-\d\d-\d\dT.*(?:Z|[+-]\d\d:\d\d)$/.test(value || '') && Number.isFinite(Date.parse(value)) && Date.parse(value) <= Date.parse(capturedAt) + 60000);
    if (new Set(nativeValues).size > 1) return {publishedAt:null,source:'conflicting_native_time',estimated:false};
    if (new Set(nativeValues).size === 1) return { publishedAt: new Date(nativeValues[0]).toISOString(), source: 'native_datetime', estimated: false };
    const unixValues = [...container.querySelectorAll('[data-utime]')].filter(own)
      .map(element => element.getAttribute('data-utime')).filter(value => /^\d{10}$/.test(value || '') && Number(value) * 1000 <= Date.parse(capturedAt) + 60000);
    if (new Set(unixValues).size > 1) return {publishedAt:null,source:'conflicting_native_time',estimated:false};
    if (new Set(unixValues).size === 1) return { publishedAt: new Date(Number(unixValues[0]) * 1000).toISOString(), source: 'native_unix_attribute', estimated: false };
    const permalink=adapter.findPermalinkDetails(container,helpers)?.url;
    const candidateId=adapter.platformIdFromCandidates([permalink]);
    const structured=candidateId ? globalThis.FacebookHeadlessTimeEvidence?.resolve(candidateId,adapter,capturedAt) : null;
    if(structured?.conflict) return {publishedAt:null,source:'conflicting_structured_time',estimated:false,evidence:structured};
    if(structured?.publishedAt) return {publishedAt:structured.publishedAt,source:'native_structured_story',estimated:false,evidence:structured};
    const cached = state.cache.get(container);
    const texts = [adapter.extractPresentation(container, helpers).timestampText,
      ...(cached?.fingerprint === fingerprint(adapter, container) && cached.permalink === permalink ? [cached.tooltipText] : []),
      ...timestampAnchors(container).map(anchor => compactText(anchor))];
    for (const text of texts) {
      const estimate = relativeTime(text, adapter, capturedAt);
      if (estimate) return { ...estimate, source: 'relative_text_estimate', text };
    }
    return { publishedAt: null, source: 'unavailable', estimated: false, evidence: structured };
  }
  function mediaFor(adapter, container) {
    const seen = new Set();
    return adapter.mediaAcquisition.extractCandidates(container, { ...helpers, collectRootCandidates(root, { kind }) {
      const elements = root.matches('img,video') ? [root] : [...root.querySelectorAll('img,video')];
      return elements.filter(element=>boundary.owns(element,container)).flatMap(element => {
        const video = element.tagName === 'VIDEO';
        if (!video && adapter.shouldSkipImage?.(element)) return [];
        const url = normalizeHttpUrl(video ? element.poster : element.currentSrc || element.src);
        if (!url) return [];
        const parsed = new URL(url);
        if (parsed.protocol !== 'https:' || parsed.username || parsed.password || parsed.port
          || !adapter.mediaHosts.some(host => parsed.hostname === host || parsed.hostname.endsWith('.' + host))) return [];
        const mediaKind = video || kind === 'video' ? 'video_poster' : 'image';
        const key = `${mediaKind}:${url}`;
        if (seen.has(key) || seen.size >= 20) return [];
        seen.add(key);
        return [{ kind: mediaKind, url, alt: element.alt || '', loaded: video ? element.readyState > 0 : element.complete && element.naturalWidth > 0,
          loadedMeaning: video ? 'video_metadata_ready' : 'image_decoded', width: video ? element.videoWidth : element.naturalWidth,
          height: video ? element.videoHeight : element.naturalHeight,
          ...(video ? { readyState: element.readyState, paused: element.paused, currentTime: element.currentTime,
            sourceKind: element.currentSrc?.startsWith('blob:') ? 'blob' : element.currentSrc ? 'direct' : 'unset' } : {}) }];
      });
    } });
  }
  async function extract(adapter, container) {
    const authors=new Set([...container.querySelectorAll('[aria-label^="Actions for this post by "]')]
      .map(node=>compactText(node.getAttribute('aria-label'))));
    if(authors.size>1)return {rejection:'ambiguous_primary_author'};
    const details = adapter.findPermalinkDetails(container, helpers);
    const id = adapter.platformIdFromCandidates([details?.url]);
    const author = adapter.findAuthor(container, helpers);
    if (!id || !details?.url || !author) return { rejection: !id ? 'missing_identity' : 'missing_author' };
    const root = container.querySelector(adapter.contentRootSelector);
    const policy = adapter.contentExpansion;
    const button = root && [...root.querySelectorAll(policy.buttonSelector)].find(b => /^(see more|show more|more)$/i.test(compactText(b)));
    const before = adapter.extractText(container, helpers);
      const allowExpansion=globalThis.AkuHeadlessCapturePolicy?.allowContentExpansion !== false;
      let expansion = button ? allowExpansion ? 'expand_failed' : 'expansion_skipped_detect_only' : 'visible_text_no_collapse_control';
    const originalUrl = location.href;
    try {
      if (button && allowExpansion) {
        button.click();
        for (let attempt = 0; attempt < policy.attempts; attempt++) {
          await delay(policy.intervalMs);
          if (location.href !== originalUrl) throw new Error('Facebook text expansion unexpectedly navigated away.');
          if (adapter.extractText(container, helpers).length > before.length) { expansion = 'expanded'; break; }
        }
      }
      const text = adapter.extractText(container, helpers);
      const media = mediaFor(adapter, container);
      if (!text && !media.length) return { rejection: 'empty_evidence' };
      const mediaExpected = [...new Set(adapter.mediaAcquisition.detectExpectedKinds(container, helpers))];
      const expectedWithoutUrl = mediaExpected.filter(kind => !media.some(m => m.kind === (kind === 'video' ? 'video_poster' : kind)));
      const presentation = adapter.extractPresentation(container, helpers);
      const capturedAt = new Date().toISOString();
      const timestamp = timestampEvidence(container, adapter, capturedAt);
      const semantics = adapter.extractSemantics(container, helpers);
      const collapsed = Boolean(root && [...root.querySelectorAll(policy.buttonSelector)].some(b => /^(see more|show more|more)$/i.test(compactText(b))));
      return { post: { id, permalink: details.url, permalinkProvenance: details.source, author, text,
        avatar: adapter.findAvatar(container, helpers), capturedAt, publishedAt: timestamp.publishedAt,
        timestampSource: timestamp.source, timestampEstimated: timestamp.estimated, timestampText: timestamp.text || presentation.timestampText,
        timestampEvidence: timestamp.evidence || null,
        hoverEvidence: state.cache.get(container)?.fingerprint === fingerprint(adapter, container) ? state.cache.get(container).diagnostics : null,
        ...semantics, presentation, media, mediaExpected,
        mediaEvidence: { status: expectedWithoutUrl.length ? 'missing_expected_url' : media.length ? 'observed_urls_partial' : 'no_media_observed', expectedWithoutUrl },
        textCollapsed: collapsed, textStatus: expansion,
        limitations: ['visible_dom_only', 'no_production_admission', 'no_structured_media_resolver',
          ...(collapsed ? ['text_may_be_collapsed'] : []), ...(mediaExpected.includes('video') ? ['video_stream_not_resolved'] : []),
          ...(semantics.relationshipType === 'repost' ? ['shared_post_body_not_separated'] : [])] } };
    } finally {
      if (expansion === 'expanded' && policy.restorable && /^(see less|show less|less)$/i.test(compactText(button))) {
        button.click();
        for (let attempt = 0; attempt < 8; attempt++) { await delay(30); if (adapter.extractText(container, helpers).length <= before.length) break; }
      }
    }
  }
  globalThis.XHeadlessPoC = {
    prepareEvidenceTargets() {
      const adapter = globalThis.AkuSourceAdapters.get('facebook');
      const targets = [];
      const scope=boundary.captureScope(boundary.candidates(adapter.discoverCandidates(helpers).candidates),adapter,helpers);
      for (const container of scope.hoverCandidates) {
        const box = container.getBoundingClientRect();
        if (box.height <= 0 || box.bottom <= 0 || box.top >= innerHeight) continue;
        const key = fingerprint(adapter, container);
        if (state.attempts.get(container) === key) continue;
        // Facebook can expose a placeholder current-page href even when it parses
        // as a native permalink. Observe its own header before trusting identity.
        const anchors = timestampAnchors(container);
        if (!anchors.length) continue;
        for (const anchor of anchors) {
          if (targets.length >= 4) break;
          const rect = anchor.getBoundingClientRect();
          const token = String(++state.sequence);
          const beforePermalink=adapter.findPermalinkDetails(container,helpers)?.url||null;
          state.targets.set(token, { container, anchor, fingerprint: key, beforeIdentity: Boolean(beforePermalink),beforePermalink });
          targets.push({ token, x: rect.left + rect.width / 2, y: rect.top + rect.height / 2 });
        }
        state.attempts.set(container, key);
        if (targets.length >= 4) break;
      }
      return targets;
    },
    recordHoverEvidence(token) {
      const target = state.targets.get(token); state.targets.delete(token);
      const adapter = globalThis.AkuSourceAdapters.get('facebook');
      if (!target?.container.isConnected || target.fingerprint !== fingerprint(adapter, target.container)) return;
      const described = (target.anchor.getAttribute('aria-describedby') || '').split(/\s+/).filter(Boolean).map(id => document.getElementById(id));
      const tooltipText = described.filter(node => node?.getAttribute('role') === 'tooltip').map(compactText).join(' ');
      const afterIdentity = Boolean(adapter.findPermalinkDetails(target.container, helpers));
      const previous = state.cache.get(target.container);
      const permalink = adapter.findPermalinkDetails(target.container,helpers)?.url || null;
      const samePrevious = previous?.fingerprint === target.fingerprint && previous?.permalink === permalink ? previous : null;
      state.cache.set(target.container, { fingerprint: target.fingerprint, permalink,
        tooltipText: tooltipText || samePrevious?.tooltipText || '',
        diagnostics: { attempted: true, permalinkRecoveredByHover: previous?.diagnostics.permalinkRecoveredByHover || !target.beforeIdentity && afterIdentity,
          permalinkChangedByHover:Boolean(previous?.diagnostics.permalinkChangedByHover||target.beforePermalink&&target.beforePermalink!==permalink),
          scopedTooltipObserved: Boolean(tooltipText || samePrevious?.tooltipText),
          timestampAnchorHrefExposed: /^https:\/\/www\.facebook\.com\//.test(target.anchor.href || '') } });
    },
    scrollNext() {
      const adapter = globalThis.AkuSourceAdapters.get('facebook');
      const scope=boundary.captureScope(boundary.candidates(adapter.discoverCandidates(helpers).candidates),adapter,helpers);
      const next = scope.candidates.find(container => container.getBoundingClientRect().top >= innerHeight);
      if (next) next.scrollIntoView({ block: 'start' });
      else window.scrollBy(0, Math.round(innerHeight * 0.8 * adapter.captureTuning.scrollStepMultiplier));
    },
    async collect() {
      const adapter = globalThis.AkuSourceAdapters.get('facebook');
      const discovery = adapter.discoverCandidates(helpers);
      const scope=boundary.captureScope(boundary.candidates(discovery.candidates),adapter,helpers);
      const posts = [], rejectionReasons = {}, identityDiagnostics = [];
      for (const container of scope.candidates) {
        const box = container.getBoundingClientRect();
        if (box.height <= 0 || box.bottom <= 0 || box.top >= innerHeight) continue;
        const result = await extract(adapter, container);
        if (result.post) posts.push(result.post);
        else {
          rejectionReasons[result.rejection] = (rejectionReasons[result.rejection] || 0) + 1;
          if (result.rejection === 'missing_identity' && identityDiagnostics.length < 4) {
            identityDiagnostics.push({ eligibleHoverAnchors: timestampAnchors(container).length, hovered: state.cache.get(container)?.diagnostics || null,
              anchors: [...container.querySelectorAll('a[href]')].slice(0, 12).map(anchor => {
                const url = new URL(anchor.href, location.href);
                const rect = anchor.getBoundingClientRect();
                return { pathShape: url.pathname.split('/').map(part => /^\d+$/.test(part) ? ':number' : /^pfbid/.test(part) ? ':pfbid'
                  : /^(posts|groups|permalink\.php|story\.php|profile\.php|videos|watch|reel|photo|photo\.php)$/.test(part) ? part : part ? ':name' : '').join('/'),
                  queryKeys: [...url.searchParams.keys()], storyIdKind: /^pfbid[A-Za-z0-9]+$/.test(url.searchParams.get('story_fbid') || '') ? 'pfbid'
                    : /^\d+$/.test(url.searchParams.get('story_fbid') || '') ? 'numeric' : 'absent',
                  targetBlank: anchor.target === '_blank', header: Boolean(anchor.closest('h2,h3,h4,strong')),
                  media: Boolean(anchor.querySelector('img,video')), width: Math.round(rect.width), height: Math.round(rect.height) };
              }) });
          }
        }
      }
      // Never choose a first/longest record when one ID has conflicting owners.
      const conflicted=new Set();
      for(const post of posts)if(posts.some(other=>other.id===post.id&&(other.author!==post.author||other.text!==post.text)))conflicted.add(post.id);
      rejectionReasons.conflicting_post_binding=posts.filter(post=>conflicted.has(post.id)).length;
      const admitted=posts.filter((post,index)=>!conflicted.has(post.id)&&posts.findIndex(other=>other.id===post.id)===index);
      const loginRequired = adapter.loginRequired();
      return { source: 'facebook', adapterVersion: adapter.version, visibility: document.visibilityState, url: location.href, title: document.title,
        candidateCount: discovery.candidates.length, discoveryStrategy: discovery.strategy, candidateDiagnostics: discovery.candidateDiagnostics,
        dialogScope:{status:scope.status,candidateCount:scope.candidates.length},
        photoEvidence:globalThis.FacebookHeadlessPhotoEvidence?.collect() || null,
        rejected: Object.values(rejectionReasons).reduce((sum,count) => sum + count, 0), rejectionReasons, identityDiagnostics, posts:admitted,
        boundaryDiagnostics:boundary.diagnostics(discovery.candidates),
        sourceUnavailable: adapter.availability(), loginRequired,
        challengeDetected: /\/checkpoint|\/challenge/i.test(location.pathname) || Boolean(document.querySelector('iframe[src*="captcha"]')),
        authenticatedUiObserved: !loginRequired && Boolean(document.querySelector('[aria-label="Your profile"], [aria-label="Account"], [aria-label="Akun"], [aria-label="Profil Anda"], [aria-label="Messenger"]')),
        documentReady: document.readyState === 'complete',
        scroll: { y: scrollY, height: document.scrollingElement?.scrollHeight ?? null, viewportWidth: innerWidth, viewportHeight: innerHeight } };
    },
  };
})();
