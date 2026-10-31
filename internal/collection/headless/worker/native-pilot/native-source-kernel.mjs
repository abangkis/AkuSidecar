// Source-independent native attempt lifetime. Source recipes alone choose URLs,
// session material and account projections; this kernel never interprets them.
import {markSourceTransient} from './source-retry-policy.mjs';
import {createMainInstagramOwnedScope} from './main-browser-diagnostic-scope.mjs';
import {cdpCommandFailureDetail} from './cdp-command-failure.mjs';
const diagnostics = new WeakMap();
export const nativeSourceDiagnostic = error => diagnostics.get(error) ?? null;
export function annotateNativeSourceCleanup(error, cleanup) {
  if (diagnostics.has(error)) diagnostics.set(error, Object.freeze({...diagnostics.get(error), cleanup}));
}
export function nativeSourceFailure(stage, reason) {
  const error = new Error('native_source_failed');
  diagnostics.set(error, Object.freeze({stage, reason, cleanup: 'not_started'}));
  if (['capture', 'navigation', 'evidence'].includes(stage) && reason === 'deadline') markSourceTransient(error, 'timeout');
  return error;
}
export const nativeRecord = (value, keys) => value && [Object.prototype, null].includes(Object.getPrototypeOf(value)) &&
  Reflect.ownKeys(value).length === keys.length && keys.every(key => Object.hasOwn(Object.getOwnPropertyDescriptor(value, key) ?? {}, 'value'));
export const nativeRef = value => typeof value === 'string' && value.length > 0 && value.length <= 1024 && !/[\u0000-\u001f\u007f]/u.test(value);

// Both the compatible X owner and generic source attempts use this executor.
// Callers retain resource order, ownership checks and their existing deadline.
// Errors stay local to the optional diagnostic projector, never in the receipt.
export async function settleNativeCleanup(stages, onFailure = () => {}) {
  const outcomes = [];
  for (const [stage, release] of stages) {
    let outcome = 'not_needed';
    if (release) {
      try { await release(); outcome = 'verified'; }
      catch (error) {
        outcome = 'uncertain';
        // Diagnostics must not prevent independent resources from retiring.
        try { onFailure(stage, error); } catch {}
      }
    }
    outcomes.push(Object.freeze({stage, outcome}));
  }
  return Object.freeze(outcomes);
}

// Mechanism only: callers supply clock validation and their closed diagnostic
// vocabulary. The absolute deadline is fixed; cancellation never renews it.
export function createNativeOperationScope({now, admitted, deadlineMS, validTime = Number.isSafeInteger, fail, rejected}) {
  if (!validTime(admitted) || !validTime(deadlineMS) || deadlineMS <= admitted) throw fail('lifetime', 'clock');
  const abort = new AbortController();
  let last = admitted;
  function remaining() {
    const time = now();
    if (!validTime(time) || time < last) throw fail('lifetime', 'clock');
    last = time;
    if (abort.signal.aborted) throw fail('lifetime', 'closed');
    if (time >= deadlineMS) throw fail('lifetime', 'deadline');
    return deadlineMS - time;
  }
  async function run(stage, operation) {
    let timer, listener;
    try {
      const ms = remaining();
      const result = await Promise.race([
        Promise.resolve().then(() => operation(remaining())),
        new Promise((_, reject) => { timer = setTimeout(() => reject(fail(stage, 'deadline')), ms); }),
        new Promise((_, reject) => { listener = () => reject(fail(stage, 'canceled')); abort.signal.addEventListener('abort', listener, {once: true}); }),
      ]);
      remaining();
      return result;
    } catch (error) { throw rejected(stage, error); }
    finally { clearTimeout(timer); if (listener) abort.signal.removeEventListener('abort', listener); }
  }
  return Object.freeze({remaining, run, cancel: () => abort.abort()});
}

export function createNativeSourceAttempt({source, generation, nonce, profileScope, deadlineMS, now = Date.now, cleanupMS = 3000}) {
  const admitted = now();
  if (!['x', 'instagram'].includes(source) || !Number.isSafeInteger(generation) || generation < 1 ||
      !/^[a-f0-9]{48}$/u.test(nonce) || !nativeRef(profileScope) || !Number.isSafeInteger(admitted) ||
      !Number.isSafeInteger(deadlineMS) || deadlineMS <= admitted || deadlineMS - admitted > 60000 ||
      !Number.isSafeInteger(cleanupMS) || cleanupMS < 1 || cleanupMS > 3000) throw nativeSourceFailure('admission', 'scope');
  const binding = Object.freeze({source, generation, nonce, profileScope, deadlineMS});
  const resources = new Set();
  let state = 'active', first, cleanup = 'not_started', closing;
  function fail(stage, reason) { first ??= nativeSourceFailure(stage, reason); return first; }
  const scope = createNativeOperationScope({now, admitted, deadlineMS, fail,
    rejected(stage, error) {
      first ??= diagnostics.has(error) ? error : nativeSourceFailure(stage, 'unavailable');
      return first;
    },
  });
  const {remaining, run} = scope;
  const closeResource = resource => resource.closing ??= resource.creation.then(async value => {
    await resource.release(value, cleanupMS); resource.closed = true;
  });
  // Register before starting creation. A result arriving after cancellation
  // retains its release obligation without making the attempt live again.
  function own(stage, create, release) {
    remaining();
    const resource = {closed: false, release};
    resource.creation = Promise.resolve().then(() => { remaining(); return create(remaining()); });
    resources.add(resource);
    void resource.creation.then(() => { if (state !== 'active') void closeResource(resource).catch(() => {}); }, () => { resource.closed = true; });
    return run(stage, () => resource.creation);
  }
  async function boundedCleanup(operation) {
    let timer;
    try { return await Promise.race([operation, new Promise((_, reject) => { timer = setTimeout(() => reject(nativeSourceFailure('cleanup', 'deadline')), cleanupMS); })]); }
    finally { clearTimeout(timer); }
  }
  function close() {
    closing ??= (async () => {
      state = 'closing'; scope.cancel();
      // Reverse creation order retires child pages before their process. A
      // failed child still leaves every later release obligation runnable.
      const results = await boundedCleanup(settleNativeCleanup(
        [...resources].reverse().map((resource, index) => [index, () => closeResource(resource)]),
      )).catch(() => null);
      cleanup = results && results.every(result => result.outcome === 'verified') && [...resources].every(resource => resource.closed) ? 'verified' : 'uncertain';
      state = 'closed';
      if (first) diagnostics.set(first, Object.freeze({...diagnostics.get(first), cleanup}));
      return Object.freeze({closed: cleanup === 'verified', cleanup, source, generation, nonce});
    })();
    return closing;
  }
  return Object.freeze({binding, run, own, remaining, close, fail,
    assert(values) {
      if (!values || values.generation !== generation || values.nonce !== nonce || values.profileScope !== profileScope) throw fail('admission', 'binding');
      remaining();
    },
    status: () => Object.freeze({source, state, cleanup}),
  });
}

// Only the exact target created here is reserved and retired. An acknowledged
// close is insufficient: its absence must be verified before releasing pins.
export async function createNativeOwnedPage(transport, timeoutMS) {
  let targetId, sessionId, closing;
  // The native IG reader gets a sealed local-timeout capability with the same
  // exact-resource ledger as diagnostics. Ordinary transport.call is unchanged.
  const owned=typeof transport.createInstagramOwnedScope==='function'?transport.createInstagramOwnedScope():createMainInstagramOwnedScope({
    alive:()=>transport.alive?.()??true,privateTarget:id=>transport.privateTarget?.(id)??false,privateSession:id=>transport.privateSession?.(id)??false,
    reserveTarget:id=>transport.reserveTarget(id),reserveSession:id=>transport.reserveSession(id),
    releaseTarget:id=>transport.releaseTarget?.(id),releaseSession:id=>transport.releaseSession?.(id),
    subscribe:fn=>transport.subscribe(fn),call:(...args)=>transport.call(...args),
  });
  const subscriptions=new Set();
  const setupDeadline = Date.now() + timeoutMS;
  const call = (method, params = {}, session = '', ms, late) => {
    const budget = ms ?? Math.min(10000, setupDeadline - Date.now());
    if (budget < 1) throw nativeSourceFailure('target', 'deadline');
    return owned.call(method, params, session, budget, late);
  };
  const close = () => closing ??= (async () => {
    let subscriptionFailure=false;
    for(const off of [...subscriptions])try{off();}catch{subscriptionFailure=true;}
    if (!targetId) { if(!owned.cleanupVerified())throw nativeSourceFailure('cleanup','target_unverified');return; }
    await call('Target.closeTarget', {targetId}, '', 1000);
    const until = Date.now() + 2000;
    while (Date.now() < until) {
      const result = await call('Target.getTargets', {}, '', Math.max(1, until - Date.now()));
      if (!Array.isArray(result.targetInfos)) throw nativeSourceFailure('cleanup', 'inventory');
      if (!result.targetInfos.some(target => target.targetId === targetId)) {
        if(sessionId)owned.releaseSession(sessionId);owned.releaseTarget(targetId);
        if(subscriptionFailure||!owned.cleanupVerified())throw nativeSourceFailure('cleanup','unverified');
        owned.dispose();
        return;
      }
      await new Promise(resolve => setTimeout(resolve, 25));
    }
    throw nativeSourceFailure('cleanup', 'target_present');
  })();
  try {
    const target = await call('Target.createTarget', {url: 'about:blank', background: true, forTab: false});
    if (!nativeRef(target.targetId)) throw nativeSourceFailure('target', 'identity');
    targetId = target.targetId;
    const attached = await call('Target.attachToTarget', {targetId, flatten: true});
    if (!nativeRef(attached.sessionId)) throw nativeSourceFailure('target', 'identity');
    sessionId = attached.sessionId;
    for (const method of ['Page.enable', 'Runtime.enable', 'Network.enable']) await call(method, {}, sessionId);
    return Object.freeze({
      on(listener) {
        if(typeof transport.subscribe!=='function'||typeof listener!=='function'||closing)throw nativeSourceFailure('evidence','unavailable');
        let active=true;
        const unsubscribe=owned.on(event=>{if(active&&!closing&&event.sessionId===sessionId)listener(event);});
        const off=()=>{if(!active)return;active=false;subscriptions.delete(off);unsubscribe();};
        subscriptions.add(off);return off;
      },
      send: (method, params = {}, ms = timeoutMS, late) => call(method, params, sessionId, ms, late),
      navigate: (url, ms = timeoutMS) => call('Page.navigate', {url}, sessionId, ms),
      async evaluate(expression, ms = timeoutMS) {
        const reply = await call('Runtime.evaluate', {expression, returnByValue: true, awaitPromise: true, userGesture: false}, sessionId, ms);
        if (reply.exceptionDetails) throw nativeSourceFailure('evidence', 'evaluation');
        return reply.result?.value;
      }, close,
    });
  } catch (error) {
    const failure=nativeSourceDiagnostic(error)?error:nativeSourceFailure('target',cdpCommandFailureDetail(error)?.kind==='timeout'?'deadline':'unavailable');
    try { await close(); } catch { annotateNativeSourceCleanup(failure,'uncertain'); }
    throw failure;
  }
}
