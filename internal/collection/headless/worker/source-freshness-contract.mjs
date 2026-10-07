import { createContext, runInContext } from 'node:vm';

const cache = new WeakMap();
const sources = new Set(['x', 'linkedin', 'instagram', 'facebook']);

// Evaluate the actual provenance-hashed adapter, using only its pure freshness
// methods in Node. Source URL/identity rules remain in the adapter asset.
export function sourceFreshnessContractFor(assets, source) {
  const disabled = () => Object.freeze({ source, enabled: false });
  if (!sources.has(source) || !Array.isArray(assets)) return disabled();
  let bySource = cache.get(assets);
  if (!bySource) { bySource = new Map(); cache.set(assets, bySource); }
  if (bySource.has(source)) return bySource.get(source);
  const context = createContext({ URL });
  let result = disabled();
  try {
    for (const relative of ['AkuBridge/capture-primitives.js', 'AkuBridge/source-adapter-runtime.js',
      `AkuBridge/adapters/${source}-adapter.js`]) {
      const asset = assets.find(entry => entry.relative === relative);
      if (typeof asset?.content !== 'string') throw new Error('Source contract asset unavailable');
      runInContext(asset.content, context, { timeout: 1000, filename: relative });
    }
    const adapter = context.AkuSourceAdapters.get(source);
    const contract = adapter.freshness.headless;
    if (contract?.enabled === true && adapter.freshness.revealSupported === true &&
        typeof contract.version === 'string' && contract.version &&
        typeof contract.matchesFeedURL === 'function' && typeof contract.primaryIdentity === 'function') {
      result = Object.freeze({ source, enabled: true, version: contract.version,
        matchesFeedURL: contract.matchesFeedURL, primaryIdentity: contract.primaryIdentity });
    }
  } catch {
    // Missing or invalid capabilities cannot enable source mutations.
  }
  bySource.set(source, result);
  return result;
}
