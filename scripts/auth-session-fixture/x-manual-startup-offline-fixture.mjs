// Accountless fixed recipe for transport regression. It cannot launch a browser,
// use profiles, fetch remote data or mint trusted native provenance.
import { pathToFileURL } from 'node:url';
import { resolve } from 'node:path';
import { performance } from 'node:perf_hooks';
import { inspectBootstrapCurrentSelf } from './x-bootstrap-current-self.mjs';
import { projectQADirectBody, QA_AB_URL } from './x-qa-direct-ab.mjs';
import { QABoundObservation, projectBoundBootstrap } from './x-qa-bound-observation.mjs';
import { createManualStartupCapture } from './x-manual-startup-source.mjs';
import { createManualStartupOwner } from './x-manual-startup-owner.mjs';
import { parseStrictJSON } from './x-evidence-ingress.mjs';

export function offlineStartupOwner(role) {
  if (!['reader', 'collector'].includes(role)) throw new Error('offline_startup_invalid');
  return Object.freeze({ ownerId: role === 'reader' ? '1' : '2', profileId: 'offline-profile-' + role,
    contextId: role === 'reader' ? '7' : '8', targetId: 'offline-target-' + role,
    frameId: 'offline-frame-' + role, documentId: 'offline-document-' + role });
}

export async function runOfflineStartupRecipe(request, { onPrivateStartup, observationDeadline }, { now = () => performance.now(), change = () => {}, beforeIdentityRead } = {}) {
  // Fixed independent ownership values; never echo challenge pins as evidence.
  const actual = offlineStartupOwner(request.role);
  const scope = { owner: Number(actual.ownerId), profileId: actual.profileId, sessionId: 'offline-session-' + request.role, targetId: actual.targetId };
  let binder;
  binder = new QABoundObservation(scope, actual.frameId, async id => {
    if (!binder.capture.consumeRead(id)) throw new Error('offline_startup_invalid');
    return projectQADirectBody({ base64Encoded: false, body: JSON.stringify({ language: 'en', time_zone: null,
      protected: false, always_use_https: true, discoverable_by_email: false, discoverable_by_mobile_phone: false }) });
  }, now, undefined, { deferCookieMutation: false, onPrivateResult: () => {}, onPrivateStartup },undefined,undefined,undefined,{observationDeadline});
  const emit = (method, params) => binder.observe({ method, params, sessionId: scope.sessionId }, scope);
  emit('Page.frameNavigated', { frame: { id: actual.frameId, loaderId: actual.documentId, url: 'https://x.com/home' } });
  emit('Runtime.executionContextCreated', { context: { id: Number(actual.contextId), origin: 'https://x.com', auxData: { isDefault: true, frameId: actual.frameId } } });
  if (beforeIdentityRead) {
    await beforeIdentityRead(binder.startupPins(Number(actual.contextId)), { binder, emit, actual });
    binder.startupPins(Number(actual.contextId)); // Reject drift during the held pause.
  }
  const accountId = '1908123456789012345';
  const page = { location: { origin: 'https://x.com', pathname: '/home' }, __INITIAL_STATE__: {
    session: { user_id: accountId }, entities: { users: { entities: { [accountId]: { id_str: accountId } } } },
  } };
  projectBoundBootstrap({ projection: inspectBootstrapCurrentSelf(page), accountId }, (id, projection) => binder.acceptIdentity(id, projection));
  const requestId = 'offline-settings-' + request.role;
  emit('Network.requestWillBeSent', { requestId, frameId: actual.frameId, loaderId: actual.documentId, request: { method: 'GET', url: QA_AB_URL } });
  change({ binder, emit, actual, requestId });
  emit('Network.responseReceived', { requestId, frameId: actual.frameId, loaderId: actual.documentId, hasExtraInfo: false,
    response: { url: QA_AB_URL, status: 200, mimeType: 'application/json', fromDiskCache: false, fromServiceWorker: false, fromPrefetchCache: false } });
  emit('Network.loadingFinished', { requestId, encodedDataLength: 512 });
  return binder.finish(true);
}

export function offlineStartupCapture(options = {}) {
  return createManualStartupCapture({ ...options, run: (request, sink) => runOfflineStartupRecipe(request, sink, options) });
}

export function offlineStartupOwnerActor(role, options = {}) {
  offlineStartupOwner(role);
  return createManualStartupOwner({ ...options, run: sink => runOfflineStartupRecipe({ role }, sink,
    { ...options, beforeIdentityRead: async (pins, testPorts) => {
      await sink.beforeIdentityRead(pins);
      options.afterRelease?.(testPorts);
    } }) });
}

async function runOfflineOwner(role) {
  const actor = offlineStartupOwnerActor(role);
  let buffer = Buffer.alloc(0), handle = null, lines = 0;
  try {
    for await (const chunk of process.stdin) {
      if (buffer.length + chunk.length > 32768) throw new Error('offline_startup_invalid');
      buffer = Buffer.concat([buffer, chunk]);
      for (let end; (end = buffer.indexOf(10)) >= 0;) {
        const line = buffer.subarray(0, end);
        if (line.length > 16384 || ++lines > 8) throw new Error('offline_startup_invalid');
        const command = parseStrictJSON(line.toString('utf8'), { maxJSONDepth: 4, maxJSONTokens: 64 });
        const fields = Object.keys(command).sort().join(',');
        let reply;
        if (command.op === 'prepare' && fields === 'op') {
          const ready = await actor.prepare(); handle = ready.handle; reply = Buffer.from(JSON.stringify(ready));
        } else if (command.op === 'acquire' && fields === 'handle,op,request') {
          reply = await actor.acquire(command.handle, command.request);
        } else if (command.op === 'abort' && fields === 'handle,op,preserveReader' && typeof command.preserveReader === 'boolean') {
          await actor.abort(command.handle); reply = Buffer.from('{"closed":true}');
        } else throw new Error('offline_startup_invalid');
        try { await new Promise((resolve, reject) => process.stdout.write(Buffer.concat([reply, Buffer.from('\n')]), err => err ? reject(err) : resolve())); }
        finally { reply.fill(0); }
        const next = Buffer.from(buffer.subarray(end + 1)); buffer.fill(0); buffer = next;
      }
    }
    if (buffer.length) throw new Error('offline_startup_invalid');
  } finally { buffer.fill(0); if (handle) await actor.abort(handle); }
}

// One bounded private request/reply on inherited stdio, only for the Go offline
// integration test. Stdout is private test IPC, never a public diagnostic log.
if (process.argv[1] && pathToFileURL(resolve(process.argv[1])).href === import.meta.url) {
  if (process.argv.length === 4 && process.argv[2] === '--offline-manual-owner' && ['reader', 'collector'].includes(process.argv[3])) {
    try { await runOfflineOwner(process.argv[3]); } catch { process.exitCode = 2; }
  } else if (process.argv.length === 3 && process.argv[2] === '--offline-manual-startup') {
    let buffer = Buffer.alloc(0), reply;
    try {
      for await (const chunk of process.stdin) {
        if (buffer.length + chunk.length > 16384) throw new Error('offline_startup_invalid');
        buffer = Buffer.concat([buffer, chunk]);
      }
      const request = parseStrictJSON(buffer.toString('utf8'), { maxJSONDepth: 3, maxJSONTokens: 64 });
      reply = await offlineStartupCapture()(request);
      await new Promise((resolve, reject) => process.stdout.write(reply, err => err ? reject(err) : resolve()));
    } catch { process.exitCode = 2; }
    finally { buffer.fill(0); reply?.fill(0); }
  } else process.exitCode = 2;
}
