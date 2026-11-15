import { createContext, runInContext, runInNewContext } from 'node:vm';

const cache = new WeakMap();
const policyCache = new WeakMap();
const qualityCache = new WeakMap();

// Evaluate the exact shared policy and actual adapter semantics from the
// provenance-hashed asset set. Page-provided verdicts/profiles are never used.
export function xCandidateQualityPolicyFor(assets) {
  return candidateQualityPolicyFor(assets, 'x');
}

export function instagramCandidateQualityPolicyFor(assets) {
  return candidateQualityPolicyFor(assets, 'instagram');
}

export function linkedinCandidateQualityPolicyFor(assets) {
  return candidateQualityPolicyFor(assets, 'linkedin');
}

function candidateQualityPolicyFor(assets, source) {
  let contracts = qualityCache.get(assets);
  if (!contracts) { contracts = new Map(); qualityCache.set(assets, contracts); }
  if (contracts.has(source)) return contracts.get(source);
  const label = source === 'x' ? 'X' : source === 'linkedin' ? 'LinkedIn' : 'Instagram';
  const context = createContext({URL});
  for (const relative of ['AkuBridge/capture-primitives.js', 'AkuBridge/source-adapter-runtime.js',
    `AkuBridge/adapters/${source}-adapter.js`, 'AkuBridge/capture-quality-policy.js']) {
    const asset = assets?.find(entry => entry.relative === relative);
    if (typeof asset?.content !== 'string') throw new Error(`Shared ${label} candidate quality asset is required: ${relative}`);
    runInContext(asset.content, context, {timeout:1000, filename:relative});
  }
  const policy = context.AkuCaptureQualityPolicy;
  const adapter = context.AkuSourceAdapters.get(source);
  if (typeof policy?.evaluateCandidate !== 'function' || typeof policy?.normalizeFacts !== 'function' ||
      typeof policy?.observeCandidateFacts !== 'function') throw new Error(`Shared ${label} candidate quality contract is unavailable.`);
  const contract = Object.freeze({
    // This is URL observation, never playback or account/admission proof. Use
    // the same strict native identity and progressive-media policy as Bridge.
    hasObservedPlayback(post) {
      if (source !== 'linkedin' || Object.getOwnPropertyDescriptor(post?.qualityFacts ?? {}, 'schema')?.value !== 'aku.linkedin.candidate-quality-facts.v1'
          || post.mediaEvidence?.videoStreamStatus !== 'direct_dom_playback_observed'
          || post.mediaEvidence?.expectedWithoutUrl?.includes('video')) return false;
      return Boolean(adapter.freshness.headless.primaryIdentity(post)) &&
        adapter.mediaAcquisition.isComplete(post.media, {contentKind:'video',expectedKinds:['video']});
    },
    assess(candidate, rawFacts, textCompleteness, expectedKinds = []) {
      if (!rawFacts || typeof rawFacts !== 'object' || Array.isArray(rawFacts) ||
          Object.getOwnPropertyDescriptor(rawFacts, 'schema')?.value !== `aku.${source}.candidate-quality-facts.v1`) return null;
      const observed = policy.normalizeFacts(rawFacts);
      // Recovery/enrichment may have replaced text or media since DOM sampling.
      // Retry authority is terminal here and remains in the capture controller.
      const normalizedCandidate = {...candidate,
        media: candidate.media.map(item => ({...item, kind:item.kind==='video_poster'?'video':item.kind})),
        quotedPost: candidate.quotedPost && {...candidate.quotedPost,
          media:Array.isArray(candidate.quotedPost.media) ? candidate.quotedPost.media.map(item=>({...item,kind:item.kind==='video_poster'?'video':item.kind})) : []},
      };
      const facts = {...observed,
        stableTextIdentity:String(candidate.text ?? '').replace(/\s+/g,' ').trim().length >= 40,
        timestampSignalDetected:observed.timestampSignalDetected || Boolean(candidate.presentation?.timestampText),
        publishedAtNotExposed:candidate.presentation?.timestampSource==='not_exposed_promoted' || candidate.presentation?.timestampAvailability==='not_exposed_promoted',
        mediaPlaybackMissing:typeof adapter.mediaAcquisition.isComplete==='function' &&
          !adapter.mediaAcquisition.isComplete(normalizedCandidate.media,{contentKind:candidate.contentKind,
            expectedKinds:Array.isArray(expectedKinds)?expectedKinds.filter(kind=>['image','video'].includes(kind)):[]}),
      };
      const report = policy.evaluateCandidate({candidate:normalizedCandidate,facts,
        profileId:adapter.qualityProfile,evidenceProfile:adapter.evidenceProfile,candidateKey:null,attempt:0,retriesRemaining:0});
      return structuredClone({...report,policyVersion:policy.runtimeRevision,stage:'terminal_observation',
        retriesRemaining:0,textCompleteness:['expanded','no_collapse_observed','recovery_verified','partial','truncated'].includes(textCompleteness)?textCompleteness:'unknown'});
    },
  });
  contracts.set(source,contract);
  return contract;
}

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
