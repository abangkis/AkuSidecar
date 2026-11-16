// PoC MAIN-world resolver, following AkuBridge's bounded descriptor-only React traversal.
// Only explicit quote relations on the DOM-identified primary tweet are accepted.
(() => {
  const object = value => value !== null && typeof value === 'object';
  const data = (value, key) => {
    try { return Object.getOwnPropertyDescriptor(value, key)?.value; } catch { return undefined; }
  };
  const names = value => { try { return Object.getOwnPropertyNames(value).slice(0, 80); } catch { return []; } };
  const numeric = value => typeof value === 'string' && /^\d{5,30}$/.test(value) ? value
    : typeof value === 'number' && Number.isSafeInteger(value) && /^\d{5,30}$/.test(String(value)) ? String(value) : null;
  const id = value => numeric(data(value, 'rest_id')) || numeric(data(value, 'id_str'))
    || numeric(data(value, 'tweet_id')) || numeric(data(value, 'tweetId'));
  globalThis.XHeadlessQuoteIdentity = {
    resolve(container, primaryId, domPermalink) {
      const roots = names(container).filter(key => /^__(?:reactProps|reactFiber|reactContainer)\$/.test(key))
        .map(key => data(container, key)).filter(object);
      const queue = roots.map(value => ({ value, depth: 0 }));
      const seen = new Set(), related = new Set();
      let visited = 0, matched = 0;
      const quoteFieldNames = new Set(), primaryShapeKeys = new Set();
      while (queue.length && visited < 3000) {
        const { value, depth } = queue.shift();
        if (!object(value) || seen.has(value)) continue;
        seen.add(value); visited++;
        for (const key of names(value)) if (/quote/i.test(key)) quoteFieldNames.add(key);
        if (numeric(data(value, 'id')) === primaryId || id(value) === primaryId) {
          for (const key of names(value)) primaryShapeKeys.add(key);
        }
        if (id(value) === primaryId) {
          matched++;
          const legacy = data(value, 'legacy');
          for (const candidate of [numeric(data(value, 'quoted_status_id_str')), numeric(data(legacy, 'quoted_status_id_str'))]) {
            if (candidate && candidate !== primaryId) related.add(candidate);
          }
          const result = data(data(value, 'quoted_status_result'), 'result');
          const nested = id(result) || id(data(result, 'tweet'));
          if (nested && nested !== primaryId) related.add(nested);
        }
        if (depth >= 12) continue;
        for (const key of names(value)) {
          // Ancestor fiber props hold the owning Tweet; explicit primary ID still gates admission.
          if (['sibling', 'alternate', 'stateNode', '_owner'].includes(key)) continue;
          const child = data(value, key);
          if (object(child) && !seen.has(child) && !data(child, 'nodeType')) queue.push({ value: child, depth: depth + 1 });
        }
      }
      const domId = domPermalink?.match(/\/status\/(\d+)/)?.[1] || null;
      const bounded = queue.length > 0;
      const ids = [...related];
      const conflict = ids.length > 1 || domId && ids.some(candidate => candidate !== domId) || domId === primaryId;
      const evidence = { visited, matchedPrimaryNodes: matched, explicitQuoteIds: ids.slice(0, 4), bounded,
        quoteFieldNames: [...quoteFieldNames].slice(0, 20), primaryShapeKeys: [...primaryShapeKeys].slice(0, 40) };
      if (conflict) return { permalink: null, status: 'conflicting_identity', provenance: null, evidence };
      if (domId) return { permalink: domPermalink, status: 'identified', provenance: 'observed_dom', evidence };
      if (ids.length === 1 && !bounded) return { permalink: `https://x.com/i/status/${ids[0]}`,
        status: 'identified', provenance: 'main_structured_quote_relation', evidence };
      return { permalink: null, status: bounded ? 'bounded_unresolved' : 'unknown', provenance: null, evidence };
    },
  };
})();
