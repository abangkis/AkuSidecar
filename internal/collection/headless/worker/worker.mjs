import { fileURLToPath } from 'node:url';
import { pathToFileURL } from 'node:url';
import { resolve, dirname } from 'node:path';
import { createHash } from 'node:crypto';
import { readFile, realpath, stat } from 'node:fs/promises';
import { StringDecoder } from 'node:string_decoder';
import { launchChrome } from './chrome.mjs';
import { createBorrowedChrome, createBorrowedRPC } from './borrowed.mjs';
import { capture } from './capture.mjs';
import { sourceProvenance } from './provenance.mjs';

export const WORKER_VERSION = '1.0.0';
export const PROTOCOL_VERSION = 1;
export const GLOBAL_RPC_IDLE_TIMEOUT_MS = 120_000;
const MAX_LINE_BYTES = 1024 * 1024;
const MAX_RESPONSE_BYTES = 8 * 1024 * 1024;
const root = dirname(fileURLToPath(import.meta.url));

export function responseFor(id, ok, result, error) {
  return { id: id ?? null, ok, ...(result === undefined ? {} : { result }), ...(error ? { error } : {}) };
}

export function validateRequest(value) {
  if (!value || typeof value !== 'object' || Array.isArray(value)) throw requestError('invalid_request', 'request must be a JSON object');
  if (!(typeof value.id === 'string' && value.id.length > 0 && value.id.length <= 128)
      && !(Number.isSafeInteger(value.id) && value.id >= 0)) {
    throw requestError('invalid_request', 'id must be a non-empty string or non-negative safe integer');
  }
  if (!['init', 'capture', 'shutdown', 'setIdleHold'].includes(value.type)) throw requestError('invalid_request', 'unsupported request type');
  return value;
}

function requestError(code, message) {
  const error = new Error(message);
  error.code = code;
  return error;
}

export async function sourceAssets(bridgePath, source) {
  if (!['x','facebook','instagram','linkedin'].includes(source)) throw requestError('unsupported_source','no headless source assets');
  const bridge = await realpath(bridgePath);
  const bridgeStat = await stat(bridge);
  if (!bridgeStat.isDirectory()) throw requestError('invalid_init', 'bridgePath must name the AkuBridge directory');
  const shared = [
    { relative: 'AkuBridge/capture-primitives.js', path: resolve(bridge, 'capture-primitives.js'), execute: true },
    { relative: 'AkuBridge/source-adapter-runtime.js', path: resolve(bridge, 'source-adapter-runtime.js'), execute: true },
  ];
  const selected = source === 'x'
    ? [
        { relative: 'AkuBridge/adapters/x-adapter.js', path: resolve(bridge, 'adapters/x-adapter.js'), execute: true },
        { relative: 'worker/vendor/x-quote-identity.js', path: resolve(root, 'vendor/x-quote-identity.js'), execute: true },
        { relative: 'worker/vendor/x-extract.js', path: resolve(root, 'vendor/x-extract.js'), execute: true },
      ]
    : source === 'facebook' ? [
        { relative: 'AkuBridge/adapters/facebook-adapter.js', path: resolve(bridge, 'adapters/facebook-adapter.js'), execute: true },
        { relative: 'worker/vendor/facebook-boundary.js', path: resolve(root, 'vendor/facebook-boundary.js'), execute: true },
        { relative: 'worker/vendor/facebook-time-evidence.js', path: resolve(root, 'vendor/facebook-time-evidence.js'), execute: true },
        { relative: 'worker/vendor/facebook-photo-evidence.js', path: resolve(root, 'vendor/facebook-photo-evidence.js'), execute: true },
        { relative: 'worker/vendor/facebook-extract.js', path: resolve(root, 'vendor/facebook-extract.js'), execute: true },
      ] : [
        { relative: 'AkuBridge/bounded-capture-policy.js', path: resolve(bridge, 'bounded-capture-policy.js'), execute: true },
        { relative: 'AkuBridge/media-post-processor.js', path: resolve(bridge, 'media-post-processor.js'), execute: true },
        ...(source === 'linkedin' ? ['linkedin-permalink-policy.js','linkedin-timestamp-policy.js'].map(name=>({relative:`AkuBridge/${name}`,path:resolve(bridge,name),execute:true})) : []),
        { relative: `AkuBridge/adapters/${source}-adapter.js`, path: resolve(bridge, `adapters/${source}-adapter.js`), execute: true },
        { relative: 'worker/vendor/adapter-extract.js', path: resolve(root, 'vendor/adapter-extract.js'), execute: true },
      ];
  selected.splice(selected.findIndex(asset => asset.relative === `AkuBridge/adapters/${source}-adapter.js`) + 1, 0,
    { relative: 'AkuBridge/source-freshness-runtime.js', path: resolve(bridge, 'source-freshness-runtime.js'), execute: true });
  const workerModules = ['capture-primitives.mjs', 'capture.mjs', 'chrome.mjs', 'borrowed.mjs', 'observation.mjs', 'provenance.mjs', 'quote-navigation.mjs', 'structured-media.mjs', 'additional-source-media.mjs', 'photo-recapture.mjs', 'x-text-recovery.mjs', 'headless-freshness.mjs', 'source-freshness-contract.mjs', 'worker.mjs', 'package.json']
    .map(name => ({ relative: `worker/${name}`, path: resolve(root, name), execute: false }));
  const assets = [];
  for (const asset of [...shared, ...selected, ...workerModules]) {
    const info = await stat(asset.path);
    if (!info.isFile() || info.size > 1024 * 1024) throw requestError('invalid_init', `required source asset is unavailable: ${asset.relative}`);
    const content = await readFile(asset.path, 'utf8');
    assets.push({ ...asset, content, sha256: createHash('sha256').update(content).digest('hex') });
  }
  const resolverAsset = source === 'instagram' || source === 'linkedin'
    ? { relative:`AkuBridge/${source}-main-world-media-resolver.js`,path:resolve(bridge,`${source}-main-world-media-resolver.js`),execute:false,
        exportName:source==='instagram' ? 'resolveInstagramStructuredMediaInMainWorld' : 'resolveLinkedInStructuredMediaInMainWorld',
        runtimeRevision:`${source}-main-world-media-resolver-${source==='instagram' ? 'v2' : 'v1'}` }
    : source === 'x'
    ? { relative: 'AkuBridge/x-main-world-media-resolver.js', path: resolve(bridge, 'x-main-world-media-resolver.js'), execute: false,
        exportName: 'resolveXStructuredMediaInMainWorld', runtimeRevision: 'x-main-world-media-resolver-v1' }
    : { relative: 'AkuBridge/facebook-main-world-media-resolver.js', path: resolve(bridge, 'facebook-main-world-media-resolver.js'), execute: false,
        exportName: 'resolveFacebookStructuredMediaInMainWorld', runtimeRevision: 'facebook-main-world-media-resolver-v1' };
  const structuredMediaResolver = { available: false, functionSource: '', runtimeRevision: resolverAsset.runtimeRevision,
    relative: resolverAsset.relative, sha256: null };
  try {
    const info = await stat(resolverAsset.path);
    if (info.isFile() && info.size <= 1024 * 1024) {
      const content = await readFile(resolverAsset.path, 'utf8');
      const sha256 = createHash('sha256').update(content).digest('hex');
      assets.push({ relative: resolverAsset.relative, path: resolverAsset.path, execute: false, content, sha256 });
      structuredMediaResolver.sha256 = sha256;
      const module = await import(pathToFileURL(await realpath(resolverAsset.path)).href);
      const exportedResolver = module[resolverAsset.exportName];
      if (typeof exportedResolver === 'function') {
        structuredMediaResolver.available = true;
        structuredMediaResolver.functionSource = exportedResolver.toString();
      }
    }
  } catch {
    // Structured media is optional. Missing or unloadable Bridge resolver leaves DOM capture intact.
  }
  Object.defineProperty(assets, 'structuredMediaResolver', { value: structuredMediaResolver, enumerable: false });
  if(source==='instagram') {
    const feedResolver={available:false,functionSource:'',runtimeRevision:'instagram-main-world-feed-resolver-v1'};
    try {
      const path=resolve(bridge,'instagram-main-world-feed-resolver.js');
      const content=await readFile(path,'utf8');
      if(Buffer.byteLength(content)<=1024*1024) {
        assets.push({relative:'AkuBridge/instagram-main-world-feed-resolver.js',path,execute:false,content,sha256:createHash('sha256').update(content).digest('hex')});
        const module=await import(pathToFileURL(await realpath(path)).href);
        if(typeof module.resolveInstagramStructuredFeedInMainWorld==='function') {
          feedResolver.available=true;feedResolver.functionSource=module.resolveInstagramStructuredFeedInMainWorld.toString();
        }
      }
    } catch { /* Optional bounded fallback does not change DOM collection. */ }
    Object.defineProperty(assets,'structuredFeedResolver',{value:feedResolver,enumerable:false});
  }
  return assets;
}

export async function runWorker({
  input = process.stdin,
  output = process.stdout,
  errorOutput = process.stderr,
  launchChromeImpl = launchChrome,
  captureImpl = capture,
  sourceAssetsImpl = sourceAssets,
  setTimer = setTimeout,
  clearTimer = clearTimeout,
  idleTimeoutMs = GLOBAL_RPC_IDLE_TIMEOUT_MS,
} = {}) {
  const boundedIdleTimeoutMs = Number.isFinite(idleTimeoutMs)
    ? Math.max(1, Math.min(GLOBAL_RPC_IDLE_TIMEOUT_MS, Math.trunc(idleTimeoutMs)))
    : GLOBAL_RPC_IDLE_TIMEOUT_MS;
  let browser = null;
  let assetsBySource = null;
  let launchOptions = null;
  let initialized = false;
  let ownedBackend = false;
  let cleanupUnverified = false;
  let idleHeld = false;
  let shuttingDown = false;
  let inputEnded = false;
  let inputFinished = false;
  let pendingRequests = 0;
  let idleTimer = null;
  let idleTimerGeneration = 0;
  let idleCloseQueued = false;
  let lineBuffer = '';
  const decoder = new StringDecoder('utf8');
  let queued = Promise.resolve();

  const write = value => {
    if (output.destroyed) return Promise.resolve();
    let line = JSON.stringify(value);
    if (Buffer.byteLength(line, 'utf8') > MAX_RESPONSE_BYTES) {
      line = JSON.stringify(responseFor(value.id, false, undefined, { code: 'response_too_large', message: 'response exceeded the 8 MiB limit' }));
    }
    return new Promise(resolveWrite => output.write(line + '\n', resolveWrite));
  };
  const log = message => errorOutput?.write(`[headless-worker] ${message}\n`);
  const cancelIdleTimer = () => {
    idleTimerGeneration++;
    if (idleTimer !== null) {
      clearTimer(idleTimer);
      idleTimer = null;
    }
  };
  const scheduleIdleTimer = () => {
    if (!initialized || !ownedBackend || !browser || idleHeld || shuttingDown || inputEnded
        || pendingRequests > 0 || idleTimer !== null || idleCloseQueued) return;
    const generation = ++idleTimerGeneration;
    idleTimer = setTimer(() => {
      if (generation !== idleTimerGeneration) return;
      idleTimer = null;
      if (!browser || !ownedBackend || idleHeld || shuttingDown || inputEnded || pendingRequests > 0) {
        scheduleIdleTimer();
        return;
      }
      idleCloseQueued = true;
      queued = queued.then(async () => {
        try {
          if (browser && ownedBackend && !idleHeld && !shuttingDown && !inputEnded && pendingRequests === 0) {
            await closeBrowser();
          }
        } catch (error) {
          log(`idle Chrome close error: ${String(error?.message || error).slice(0, 300)}`);
        } finally {
          idleCloseQueued = false;
          scheduleIdleTimer();
        }
      }, async () => {
        idleCloseQueued = false;
        scheduleIdleTimer();
      });
    }, boundedIdleTimeoutMs);
  };
  const closeBrowser = async () => {
    if (!browser) return;
    const closing = browser;
    browser = null;
    try {
      await closing.close();
    } catch (error) {
      if (ownedBackend) cleanupUnverified = true;
      throw error;
    }
  };
  const launchOwnedChrome = async options => {
    try {
      return await launchChromeImpl(options);
    } catch (error) {
      if (error?.code === 'owned_chrome_cleanup_failed') cleanupUnverified = true;
      throw error;
    }
  };
  const ensureBrowser = async () => {
    if (browser) return browser;
    if (!initialized || !assetsBySource) throw requestError('not_initialized', 'send init before capture');
    if (cleanupUnverified) {
      throw requestError('owned_chrome_cleanup_unverified', 'previous Chrome process exit was not verified; refusing to launch another process for this profile');
    }
    if (!ownedBackend || !launchOptions) throw requestError('browser_unavailable', 'the initialized browser is unavailable');
    browser = await launchOwnedChrome(launchOptions);
    return browser;
  };
  const enqueueRequest = raw => {
    cancelIdleTimer();
    pendingRequests++;
    queued = queued.then(async () => {
      try {
        await handle(raw);
      } finally {
        pendingRequests--;
        scheduleIdleTimer();
      }
    }, async () => {
      pendingRequests--;
      scheduleIdleTimer();
    });
  };
  const appendBrowserClose = context => {
    queued = queued.then(async () => {
      await closeBrowser();
    }, async () => {
      await closeBrowser();
    }).catch(error => {
      log(`${context}: ${String(error?.message || error).slice(0, 300)}`);
    });
  };
  const borrowedRPC = createBorrowedRPC(write);
  const handle = async raw => {
    let id = null;
    try {
      if (Buffer.byteLength(raw, 'utf8') > MAX_LINE_BYTES) throw requestError('request_too_large', 'request line exceeds the 1 MiB limit');
      let parsed;
      try { parsed = JSON.parse(raw); } catch { throw requestError('invalid_json', 'request line is not valid JSON'); }
      const request = validateRequest(parsed);
      id = request.id;
      if (shuttingDown) throw requestError('worker_shutting_down', 'worker is shutting down');
      if (request.type === 'setIdleHold') {
        if (typeof request.held !== 'boolean') throw requestError('invalid_request', 'held must be a boolean');
        idleHeld = request.held;
        if (idleHeld) cancelIdleTimer();
        else scheduleIdleTimer();
        await write(responseFor(id, true, { held: idleHeld }));
        return;
      }
      if (request.type === 'init') {
        if (initialized) throw requestError('already_initialized', 'worker is already initialized');
        if (cleanupUnverified) {
          throw requestError('owned_chrome_cleanup_unverified', 'previous Chrome process exit was not verified; refusing to initialize another browser');
        }
        const borrowed = request.backend === 'browser_quiet_hidden';
        const options = borrowed ? validateBorrowedInit(request) : validateInit(request);
        const loadedAssets = {};
        for (const source of borrowed ? ['x','facebook'] : ['x','facebook','instagram','linkedin']) {
          loadedAssets[source] = await sourceAssetsImpl(options.bridgePath, source);
        }
        const initializedBrowser = borrowed
          ? createBorrowedChrome(borrowedRPC.send, request.chromeVersion)
          : await launchOwnedChrome(options);
        browser = initializedBrowser;
        assetsBySource = loadedAssets;
        launchOptions = borrowed ? null : options;
        ownedBackend = !borrowed;
        initialized = true;
        const result = {
          pid: browser.pid,
          chromeVersion: browser.version,
          workerDriver: { name: borrowed ? 'aku-quiet-worker' : 'aku-headless-worker', version: WORKER_VERSION, protocolVersion: PROTOCOL_VERSION },
          ...(!borrowed ? { idleReleaseVersion: 1 } : {}),
        };
        await write(responseFor(id, true, result));
        return;
      }
      if (request.type === 'capture') {
        const activeBrowser = await ensureBrowser();
        const result = await captureImpl(activeBrowser, assetsBySource, request.source, request.payload);
        await write(responseFor(id, true, result));
        return;
      }
      cancelIdleTimer();
      shuttingDown = true;
      await closeBrowser();
      await write(responseFor(id, true, { stopped: true }));
      input.pause?.();
      input.destroy?.();
    } catch (error) {
      const safe = {
        code: error?.code || 'worker_error',
        message: String(error?.message || 'worker request failed').slice(0, 500),
        ...(['empty_unverified','invalid_observation'].includes(error?.code) && error.diagnostics ? {diagnostics:error.diagnostics} : {}),
      };
      await write(responseFor(id, false, undefined, safe));
      if (errorOutput && error?.stack) errorOutput.write(`[headless-worker] ${safe.code}: ${safe.message}\n`);
    }
  };

  input.on('data', chunk => {
    lineBuffer += decoder.write(chunk);
    if (Buffer.byteLength(lineBuffer, 'utf8') > 16 * 1024 * 1024 && !lineBuffer.includes('\n')) {
      lineBuffer = '';
      void write(responseFor(null, false, undefined, { code: 'request_too_large', message: 'request line exceeds the 1 MiB limit' }));
      return;
    }
    let newline;
    while ((newline = lineBuffer.indexOf('\n')) >= 0) {
      const raw = lineBuffer.slice(0, newline).replace(/\r$/, '');
      lineBuffer = lineBuffer.slice(newline + 1);
      if (!raw) continue;
      // CDP replies must bypass the serialized capture queue: capture awaits
      // these replies itself. Ordinary commands still have the 1 MiB bound.
      let fastReply;
      try { fastReply = JSON.parse(raw); } catch {}
      if (borrowedRPC.receive(fastReply)) continue;
      enqueueRequest(raw);
    }
  });
  const finishInput = () => {
    if (inputFinished) return;
    inputFinished = true;
    inputEnded = true;
    cancelIdleTimer();
    borrowedRPC.close();
    lineBuffer += decoder.end();
    if (lineBuffer.trim()) enqueueRequest(lineBuffer);
    appendBrowserClose('shutdown error');
  };
  input.on('end', finishInput);
  const interrupt = () => {
    cancelIdleTimer();
    borrowedRPC.close();
    shuttingDown = true;
    appendBrowserClose('interrupt close error');
  };
  process.once('SIGINT', interrupt);
  process.once('SIGTERM', interrupt);
  return new Promise(resolveRun => input.once('close', () => {
    finishInput();
    void queued.finally(() => {
      process.removeListener('SIGINT', interrupt);
      process.removeListener('SIGTERM', interrupt);
      resolveRun();
    });
  }));
}

export function validateInit(request) {
  const value = request.chrome ? request : {};
  for (const key of ['chrome', 'profile', 'bridgePath']) {
    if (typeof value[key] !== 'string' || !value[key] || !isAbsolutePath(value[key])) {
      throw requestError('invalid_init', `${key} must be an explicit absolute path`);
    }
  }
  if (value.profileDirectory !== undefined && (typeof value.profileDirectory !== 'string'
      || !/^[A-Za-z0-9 _-]{1,64}$/.test(value.profileDirectory))) {
    throw requestError('invalid_init', 'profileDirectory must be a simple Chrome profile name');
  }
  return { chromePath: value.chrome, profilePath: value.profile, bridgePath: value.bridgePath, profileDirectory: value.profileDirectory };
}

export function validateBorrowedInit(request) {
  if (request.backend !== 'browser_quiet_hidden' || typeof request.bridgePath !== 'string'
      || !isAbsolutePath(request.bridgePath) || !request.chromeVersion
      || typeof request.chromeVersion !== 'object' || request.chrome || request.profile) {
    throw requestError('invalid_init', 'Quiet requires host-owned Chrome metadata and explicit source assets');
  }
  return {bridgePath: request.bridgePath};
}

function isAbsolutePath(value) {
  return /^[A-Za-z]:[\\/]/.test(value) || value.startsWith('\\\\') || value.startsWith('/');
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  void runWorker().catch(error => {
    process.stderr.write(`[headless-worker] fatal: ${String(error?.message || error).slice(0, 300)}\n`);
    process.exitCode = 1;
  });
}
