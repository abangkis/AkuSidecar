import {randomBytes} from 'node:crypto';
import {isAbsolute, relative, resolve, sep} from 'node:path';
import {lstat, realpath} from 'node:fs/promises';
import {launchChrome} from '../chrome.mjs';
import {captureIsolated, validateCapture, NATIVE_CAPTURE_CAPABILITIES} from '../capture.mjs';
import {canonicalSourceURL} from '../observation.mjs';
import {createNativeSourceAttempt, createNativeOwnedPage, nativeSourceFailure, nativeSourceDiagnostic, annotateNativeSourceCleanup, nativeRecord, nativeRef} from './native-source-kernel.mjs';
import {sourceTransientClass, SOURCE_RETRY_LIMIT} from './source-retry-policy.mjs';
import {INSTAGRAM_NATIVE_RECIPE, readInstagramEvidence, eraseInstagramCookies, verifyInstagramPair} from './instagram-native-session.mjs';

export {INSTAGRAM_NATIVE_RECIPE};
const keys = ['generation', 'nonce', 'profileScope'];
const exact = (value, fields) => { if (!nativeRecord(value, fields)) throw nativeSourceFailure('admission', 'fields'); };
const validPath = path => typeof path === 'string' && isAbsolute(path) && resolve(path) === path && nativeRef(path);
function separateProfiles(reader, collector) {
  if (!validPath(collector)) return false;
  return [[reader, collector], [collector, reader]].every(([left, right]) => {
    const rel = relative(left, right); return rel && (isAbsolute(rel) || rel === '..' || rel.startsWith('..' + sep));
  });
}
async function launchNativeCollector(options) {
  // The Go owner creates/leases this directory and validates the process before
  // source_seed. Do not create arbitrary caller-supplied profile directories.
  const info = await lstat(options.profilePath);
  if (!info.isDirectory() || info.isSymbolicLink() || await realpath(options.profilePath) !== options.profilePath) throw nativeSourceFailure('admission', 'profile');
  return launchChrome({...options,retainLateResources:true});
}

// Private sealed composition only. This implements the native candidate recipe;
// it does not declare it qualified. The public registry gates it on the sealed
// recipe record, never a request-provided proof or "allow unqualified" flag.
export async function createInstagramDailyOwner(params, transport, {loadAssets, createCollector = launchNativeCollector,
  runCapture = captureIsolated, now = Date.now} = {}) {
  exact(params, ['chromePath', 'readerProfile', 'readerPID', 'profileDirectory', 'acknowledgeLimitations', 'allowProcessSessionDerivation']);
  if (!validPath(params.chromePath) || !validPath(params.readerProfile) || !Number.isSafeInteger(params.readerPID) || params.readerPID < 1 ||
      typeof params.profileDirectory !== 'string' || !params.profileDirectory || params.profileDirectory.length > 128 || /[\\/\u0000-\u001f]/u.test(params.profileDirectory) ||
      params.acknowledgeLimitations !== true || params.allowProcessSessionDerivation !== true ||
      !['call', 'alive', 'reserveTarget', 'reserveSession', 'releaseTarget', 'releaseSession'].every(key => typeof transport?.[key] === 'function') ||
      ![loadAssets, createCollector, runCapture, now].every(value => typeof value === 'function')) throw nativeSourceFailure('admission', 'owner');
  const secret = randomBytes(32);
  const descriptor = Object.freeze({source: 'instagram', recipe: INSTAGRAM_NATIVE_RECIPE.recipe, readerOwnerID: String(params.readerPID), readerProfileID: params.readerProfile});
  let initialized = false, closed = false, generation = 0, batch, terminal = false, busy = false, previousAccount, lastBatchCleanup, closing;
  let recoverable = false, retryCount = 0;
  const check = () => { if (!initialized || closed || terminal || !transport.alive()) throw nativeSourceFailure('lifetime', 'owner'); };
  const disposeCookies = result => { if (result) eraseInstagramCookies(result.cookies); };
  async function evidence(role, nonce) {
    const page = role === 'reader' ? batch.reader : batch.collectorEvidence;
    const ownerID = role === 'reader' ? descriptor.readerOwnerID : String(batch.collector.pid);
    const profileID = role === 'reader' ? params.readerProfile : batch.profile;
    return readInstagramEvidence(page, batch.attempt, role, ownerID, profileID, secret, now, nonce);
  }
  async function pair(nonce = batch.attempt.binding.nonce) {
    let reader, collector;
    try {
      const results = await Promise.allSettled([evidence('reader', nonce), evidence('collector', nonce)]);
      reader = results[0].status === 'fulfilled' ? results[0].value : undefined;
      collector = results[1].status === 'fulfilled' ? results[1].value : undefined;
      const rejected = results.find(result => result.status === 'rejected');
      if (rejected) throw rejected.reason;
      const verified = verifyInstagramPair(reader.proof, collector.proof, {...batch.attempt.binding, nonce}, now());
      if (previousAccount !== undefined && verified.reader.accountKey !== previousAccount ||
          batch.revision !== undefined && verified.reader.sessionRevision !== batch.revision) throw nativeSourceFailure('evidence', 'changed');
      previousAccount = verified.reader.accountKey; batch.revision = verified.reader.sessionRevision;
      return verified;
    } finally { disposeCookies(reader); disposeCookies(collector); }
  }
  async function navigateHome(page) {
    await batch.attempt.run('navigation', async ms => {
      await page.navigate(INSTAGRAM_NATIVE_RECIPE.home, ms);
      const until = Math.min(Date.now() + 8000, Date.now() + ms);
      while (Date.now() < until) {
        try {
          if (await page.evaluate("location.origin==='https://www.instagram.com'&&location.pathname==='/'&&['interactive','complete'].includes(document.readyState)", Math.max(1, until - Date.now()))) return;
        } catch { /* Navigation can destroy the previous execution context. */ }
        await new Promise(resolve => setTimeout(resolve, 100));
      }
      throw nativeSourceFailure('navigation', 'home');
    });
  }
  const allowedURL = url => url === INSTAGRAM_NATIVE_RECIPE.home || Boolean(canonicalSourceURL('instagram', url));
  function wrapCapturePage(page) {
    const call = (method, ...args) => batch.attempt.run('capture', ms => page[method](...args, ms));
    return Object.freeze({
      navigate(url) { if (!allowedURL(url)) throw nativeSourceFailure('capture', 'url'); return call('navigate', url); },
      evaluate(expression) { if (typeof expression !== 'string' || expression.length > 1024 * 1024) throw nativeSourceFailure('capture', 'expression'); return call('evaluate', expression); },
      send(method, values = {}) {
        if (!['Page.navigate', 'Runtime.evaluate', 'Page.addScriptToEvaluateOnNewDocument', 'Input.dispatchMouseEvent'].includes(method) ||
            method === 'Page.navigate' && !allowedURL(values.url)) throw nativeSourceFailure('capture', 'method');
        return call('send', method, values);
      }, close: () => page.close(),
    });
  }
  async function end() {
    if (!batch) return lastBatchCleanup ?? {closed: true, cleanup: 'verified', source: 'instagram'};
    const receipt = await batch.attempt.close();
    if (!receipt.closed) terminal = true;
    else if (recoverable && retryCount <= SOURCE_RETRY_LIMIT && !closed && transport.alive()) terminal = false;
    recoverable = false;
    lastBatchCleanup = receipt;
    batch = null;
    return receipt;
  }
  function close() {
    closing ??= (async () => {
      closed = true;
      const receipt = await end(); secret.fill(0);
      if (!receipt.closed) throw nativeSourceFailure('cleanup', 'unverified');
    })();
    return closing;
  }
  async function execute(operation, values) {
    // End/close cancel a running capture and join its resource obligations.
    if (operation === 'source_close') { exact(values, []); await close(); return {closed: true}; }
    if (operation === 'source_end') {
      exact(values, keys);
      if (!batch || keys.some(key => values[key] !== batch.attempt.binding[key])) throw nativeSourceFailure('admission', 'binding');
      return {closed: (await end()).closed};
    }
    if (busy) throw nativeSourceFailure('admission', 'busy');
    busy = true;
    try {
      if (operation === 'source_initialize') {
        exact(values, []);
        if (initialized || closed || !transport.alive()) throw nativeSourceFailure('admission', 'replay');
        initialized = true; return descriptor;
      }
      check();
      if (operation === 'source_check' && nativeRecord(values, [])) return descriptor;
      if (operation === 'source_begin') {
        exact(values, [...keys, 'collectorProfile', 'deadlineMS']);
        if (batch || values.generation !== generation + 1 || !separateProfiles(params.readerProfile, values.collectorProfile)) throw nativeSourceFailure('admission', 'batch');
        const attempt = createNativeSourceAttempt({source: 'instagram', ...values, now});
        lastBatchCleanup = undefined;
        batch = {attempt, profile: values.collectorProfile, state: 'starting', pages: new Set()}; generation = values.generation;
        // No session read or cookie import occurs until the Go process owner
        // validates and enrolls the returned collector PID, then calls seed.
        const current = batch;
        current.reader = await attempt.own('target', ms => createNativeOwnedPage(transport, ms), page => page.close());
        current.collector = await attempt.own('collector', () => createCollector({chromePath: params.chromePath, profilePath: values.collectorProfile}), async collector => {
          const results = await Promise.allSettled([...current.pages].map(page => page.close()));
          let processClosed = false;
          try { await collector.close(); processClosed = true; } finally {
            if (!processClosed || results.some(result => result.status === 'rejected')) throw nativeSourceFailure('cleanup', 'collector');
          }
        });
        if (!Number.isSafeInteger(current.collector.pid) || current.collector.pid < 1 || current.collector.pid === params.readerPID ||
            current.collector.profilePath !== values.collectorProfile || typeof current.collector.createTemporaryPage !== 'function' || typeof current.collector.close !== 'function') throw nativeSourceFailure('collector', 'identity');
        current.state = 'prepared';
        return {...descriptor, collectorOwnerID: String(current.collector.pid), collectorProfileID: values.collectorProfile, collectorPID: current.collector.pid};
      }
      if (!batch) throw nativeSourceFailure('admission', 'batch');
      const current = batch;
      exact(values, operation === 'source_capture' ? [...keys, 'payload'] : keys);
      if (operation === 'source_proof') {
        if (!/^[a-f0-9]{48}$/u.test(values.nonce)) throw nativeSourceFailure('admission', 'nonce');
        current.attempt.assert({...values, nonce: current.attempt.binding.nonce});
      } else current.attempt.assert(values);
      if (operation === 'source_seed') {
        if (current.state !== 'prepared') throw nativeSourceFailure('session', 'replay');
        current.state = 'seeding';
        let reader;
        try {
          reader = await evidence('reader');
          if (previousAccount !== undefined && reader.proof.accountKey !== previousAccount) throw nativeSourceFailure('evidence', 'changed');
          current.revision = reader.proof.sessionRevision;
          current.home = await current.attempt.own('target', async ms => {
            const page = await current.collector.createTemporaryPage(ms); current.pages.add(page); return page;
          }, page => page.close());
          await current.attempt.run('session', ms => current.home.send('Network.setCookies', {cookies: reader.cookies}, ms));
          current.collectorEvidence = await current.attempt.own('target', async ms => {
            const page = await current.collector.createTemporaryPage(ms); current.pages.add(page); return page;
          }, page => page.close());
        } finally { disposeCookies(reader); }
        await navigateHome(current.home);
        const proof = await pair(); current.state = 'active'; return proof;
      }
      if (current.state !== 'active') throw nativeSourceFailure('admission', 'state');
      if (operation === 'source_proof') return pair(values.nonce);
      if (operation === 'source_check') {
        let result; try { result = await evidence('reader'); return result.proof; } finally { disposeCookies(result); }
      }
      if (operation === 'source_capture') {
        if (current.captured) throw nativeSourceFailure('capture', 'replay');
        current.captured = true;
        const options = validateCapture('instagram', values.payload);
        if (options.acquisitionRound !== 1 || options.explicitPageUrl) throw nativeSourceFailure('capture', 'scope');
        await pair();
        const assets = await current.attempt.run('assets', () => loadAssets('instagram'));
        const browser = Object.freeze({backend: 'headless_worker', captureCapabilities: NATIVE_CAPTURE_CAPABILITIES.instagram,
          forSource: async source => { if (source !== 'instagram') throw nativeSourceFailure('capture', 'source'); return wrapCapturePage(current.home); },
          createTemporaryPage: async () => wrapCapturePage(await current.attempt.own('media', async ms => {
            const page = await current.collector.createTemporaryPage(ms); current.pages.add(page); return page;
          }, page => page.close())),
        });
        const ms = Math.floor(Math.min(options.captureTimeoutMs, current.attempt.remaining()));
        if (ms < 1000) throw nativeSourceFailure('capture', 'deadline');
        const result = await current.attempt.run('capture', () => runCapture(browser, {instagram: assets}, 'instagram', {...values.payload, captureTimeoutMs: ms}));
        if (result?.source !== 'instagram' || Buffer.byteLength(JSON.stringify(result), 'utf8') > 512 * 1024) throw nativeSourceFailure('capture', 'observation');
        await pair(); retryCount = 0; return result;
      }
      throw nativeSourceFailure('admission', 'operation');
    } catch (error) {
      recoverable = !terminal && sourceTransientClass(error) !== '';
      if (recoverable) retryCount++;
      terminal = true;
      const failure = nativeSourceDiagnostic(error) ? error : nativeSourceFailure('operation', 'unavailable');
      // Preserve the first cause separately from resource retirement results.
      if (batch) { const receipt = await batch.attempt.close(); annotateNativeSourceCleanup(failure, receipt.cleanup); }
      throw failure;
    } finally { busy = false; }
  }
  return Object.freeze({execute, close, capabilities: INSTAGRAM_NATIVE_RECIPE});
}
