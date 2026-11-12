import { runInNewContext } from 'node:vm';

const cache = new WeakMap();
const policyCache = new WeakMap();

// Reuse the page's provenance-hashed policy for pure playback admission in
// Observation. No browser globals, transport or playback are available here.
export function boundedCapturePolicyFor(assets) {
  if (policyCache.has(assets)) return policyCache.get(assets);
  const asset = assets?.find(entry => entry.relative === 'AkuBridge/bounded-capture-policy.js');
  if (typeof asset?.content !== 'string') throw new Error('Shared bounded capture policy asset is required.');
  const context = { URL };
  runInNewContext(asset.content, context, { timeout: 1000, filename: asset.relative });
  const policy = context.AkuBoundedCapturePolicy;
  if (typeof policy?.xPlaybackUrl !== 'function') throw new Error('Shared X playback policy is unavailable.');
  policyCache.set(assets, policy);
  return policy;
}

// Execute the same audited, provenance-hashed Bridge asset used in browser pages.
// It contains pure helpers and DOM operations; Node uses only the pure functions.
export function capturePrimitivesFor(assets) {
  if (cache.has(assets)) return cache.get(assets);
  const asset = assets?.find(entry => entry.relative === 'AkuBridge/capture-primitives.js');
  if (typeof asset?.content !== 'string') throw new Error('Shared capture primitives asset is required.');
  const context = { URL };
  runInNewContext(asset.content, context, { timeout: 1000, filename: asset.relative });
  const primitives = context.AkuCapturePrimitives;
  if (!primitives || typeof primitives.evaluateTextReplacement !== 'function') {
    throw new Error('Shared capture primitives contract is unavailable.');
  }
  cache.set(assets, primitives);
  return primitives;
}
