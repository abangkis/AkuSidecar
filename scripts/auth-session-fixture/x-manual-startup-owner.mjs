// Private two-phase owner actor. An actual driver starts a new startup document,
// pauses before identity evaluation, then reports observed pins. A later random
// acquire challenge releases that same pause; no navigation/rebinding is allowed.
import { randomBytes } from 'node:crypto';
import { createManualStartupCapture } from './x-manual-startup-source.mjs';
import {startupFailureLatch,startupPrepareFailure} from './x-manual-startup-diagnostic.mjs';
import {consumeFreshnessPublication} from './x-identity-freshness-ledger.mjs';
import {createStartupWaitBudget} from './x-manual-startup-wait-budget.mjs';
import {MANUAL_STARTUP_MAX_MS} from './x-manual-startup-limits.mjs';
import {ManualObservationDeadline} from './x-manual-observation-deadline.mjs';

const FIELDS = ['ownerId', 'profileId', 'contextId', 'targetId', 'frameId', 'documentId'];
const unavailable = () => new Error('manual_startup_owner_unavailable');
function pins(value) {
  if (!value || typeof value !== 'object' || ![Object.prototype, null].includes(Object.getPrototypeOf(value))) throw unavailable();
  const descriptors = Object.getOwnPropertyDescriptors(value);
  const names = Reflect.ownKeys(descriptors);
  if (names.length !== FIELDS.length || names.some(k => !FIELDS.includes(k))) throw unavailable();
  const result = Object.create(null);
  for (const k of FIELDS) {
    const d = descriptors[k], v = d?.value;
    if (!d || !Object.hasOwn(d, 'value') || typeof v !== 'string' || Buffer.byteLength(v, 'utf8') > 512 || !v || /[\u0000-\u001f\u007f]/u.test(v)) throw unavailable();
    result[k] = v;
  }
  return Object.freeze(result);
}
function deferred() {
  let resolve, reject;
  const promise = new Promise((yes, no) => { resolve = yes; reject = no; });
  // A phase can be aborted before its caller begins waiting.
  void promise.catch(() => {});
  return { promise, resolve, reject };
}

export function createManualStartupOwner({ run, role='unknown',diagnostics=startupFailureLatch(), now, wallNow, requireFreshness=false }) {
  if (typeof run !== 'function'||typeof requireFreshness!=='boolean') throw unavailable();
  const ready = deferred(), release = deferred(), abort = new AbortController();
  const handle = randomBytes(24).toString('hex');
  let state = 'new', actual = null, running = null, sink = null, waitBudget = null, observationDeadline=null;
  const close = () => {
    state = 'closed'; abort.abort(); ready.reject(unavailable()); release.reject(unavailable());
  };
  const attach = signal => {
    if (signal?.aborted) { close(); throw unavailable(); }
    signal?.addEventListener('abort', close, { once: true });
    return () => signal?.removeEventListener('abort', close);
  };
  const retire = async () => {
    close();
    try { await running; } catch { /* Closed outcome only. */ }
    sink = actual = null;
  };
  const capture = createManualStartupCapture({ role,diagnostics,now, wallNow, run: async (request, output) => {
    if (state !== 'ready' || !actual || FIELDS.some(k => actual[k] !== request[k])) throw unavailable();
    observationDeadline.acquired();
    state = 'acquiring'; sink = output.onPrivateStartup; release.resolve();
    await running;
    if (abort.signal.aborted || state !== 'acquiring') throw unavailable();
    state = 'complete';
  } });
  return Object.freeze({
    async prepare(signal, deadlineMS=(wallNow??Date.now)()+MANUAL_STARTUP_MAX_MS) {
      if (state !== 'new') throw diagnostics.record(startupPrepareFailure(null,role,'actor_entry','state_invalid'),role,'prepare','unavailable','manual_startup_owner_unavailable');
      let detach;
      try { detach = attach(signal); }
      catch(error) { throw diagnostics.record(startupPrepareFailure(error,role,'actor_entry','state_invalid'),role,'prepare','unavailable','manual_startup_owner_unavailable'); }
      try{observationDeadline=new ManualObservationDeadline(deadlineMS,{now,wallNow,role});}
      catch(error){detach();close();throw diagnostics.record(error,role,'prepare','deadline');}
      state = 'preparing';
      running = Promise.resolve().then(() => run({ signal: abort.signal,
        observationDeadline,
        waitForSettings: promise => {
          if (!waitBudget || state !== 'acquiring') throw unavailable();
          return waitBudget.wait(promise);
        },
        beforeIdentityRead: async value => {
          let observed;
          try { observed = pins(value); }
          catch(error) { throw state==='preparing'?startupPrepareFailure(error,role,'actor_pins','pins_invalid'):error; }
          observationDeadline.prepared();
          if (state === 'preparing') { actual = observed; state = 'ready'; ready.resolve(); }
          else if (state !== 'acquiring' || FIELDS.some(k => actual[k] !== observed[k])) throw unavailable();
          await release.promise;
          if (abort.signal.aborted || state !== 'acquiring') throw unavailable();
        },
        onPrivateStartup: value => {
          if (state !== 'acquiring' || !sink || abort.signal.aborted) throw unavailable();
          if(requireFreshness&&!consumeFreshnessPublication(value,role,actual))throw unavailable();
          sink(value);
        },
      })).then(() => {
        if (state === 'preparing' || state === 'ready') throw startupPrepareFailure(null,role,'actor_runner','ready_missing');
      }).catch(error => { const failure=diagnostics.record(state==='preparing'?startupPrepareFailure(error,role,'actor_runner','command_failed'):error,role,state==='preparing'?'prepare':'identity','unavailable','manual_startup_owner_unavailable');close();throw failure; });
      void running.catch(() => {});
      try { await ready.promise; return Object.freeze({ handle, pins: actual }); }
      catch(error) { const failure=diagnostics.record(startupPrepareFailure(error,role,'actor_ready','state_invalid'),role,'prepare','unavailable','manual_startup_owner_unavailable');close();throw failure; }
      finally { detach(); }
    },
    async acquire(token, request, signal, startupBudgetMS = MANUAL_STARTUP_MAX_MS) {
      if (token !== handle || state !== 'ready') { close(); throw unavailable(); }
      const detach = attach(signal);
      try {
        waitBudget = createStartupWaitBudget(Math.min(startupBudgetMS,observationDeadline.remaining()), { role: request.role, signal: abort.signal });
        return await capture(request, signal);
      }
      catch(error) { const failure=diagnostics.record(error,role,'acquire_rpc','unavailable','manual_startup_owner_unavailable');close();throw failure; }
      finally { sink = null; detach(); }
    },
    async abort(token) {
      if (token !== handle) throw unavailable();
      // The trusted runner must resume/clean its paused resources on rejection.
      // Native Job retirement belongs to the Go owner port, not this actor.
      await retire();
    },
    // Trusted owner cleanup when PREPARE has not yet delivered its opaque handle.
    // This only revokes; it never supplies evidence or releases admission.
    cancel: retire,
  });
}
