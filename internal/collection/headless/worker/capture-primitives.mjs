import { runInNewContext } from 'node:vm';

const cache = new WeakMap();

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
