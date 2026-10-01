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
  if (!['init', 'capture', 'shutdown'].includes(value.type)) throw requestError('invalid_request', 'unsupported request type');
  return value;
}

function requestError(code, message) {
  const error = new Error(message);
  error.code = code;
  return error;
}

async function sourceAssets(bridgePath, source) {
  const bridge = await realpath(bridgePath);
  const bridgeStat = await stat(bridge);
  if (!bridgeStat.isDirectory()) throw requestError('invalid_init', 'bridgePath must name the AkuBridge directory');
  const shared = [
    { relative: 'AkuBridge/source-adapter-runtime.js', path: resolve(bridge, 'source-adapter-runtime.js'), execute: true },
  ];
  const selected = source === 'x'
    ? [
        { relative: 'AkuBridge/adapters/x-adapter.js', path: resolve(bridge, 'adapters/x-adapter.js'), execute: true },
        { relative: 'worker/vendor/x-quote-identity.js', path: resolve(root, 'vendor/x-quote-identity.js'), execute: true },
        { relative: 'worker/vendor/x-extract.js', path: resolve(root, 'vendor/x-extract.js'), execute: true },
      ]
    : [
        { relative: 'AkuBridge/adapters/facebook-adapter.js', path: resolve(bridge, 'adapters/facebook-adapter.js'), execute: true },
        { relative: 'worker/vendor/facebook-boundary.js', path: resolve(root, 'vendor/facebook-boundary.js'), execute: true },
        { relative: 'worker/vendor/facebook-time-evidence.js', path: resolve(root, 'vendor/facebook-time-evidence.js'), execute: true },
        { relative: 'worker/vendor/facebook-extract.js', path: resolve(root, 'vendor/facebook-extract.js'), execute: true },
      ];
  const workerModules = ['capture.mjs', 'chrome.mjs', 'borrowed.mjs', 'observation.mjs', 'provenance.mjs', 'quote-navigation.mjs', 'worker.mjs', 'package.json']
    .map(name => ({ relative: `worker/${name}`, path: resolve(root, name), execute: false }));
  const assets = [];
  for (const asset of [...shared, ...selected, ...workerModules]) {
    const info = await stat(asset.path);
    if (!info.isFile() || info.size > 1024 * 1024) throw requestError('invalid_init', `required source asset is unavailable: ${asset.relative}`);
    const content = await readFile(asset.path, 'utf8');
    assets.push({ ...asset, content, sha256: createHash('sha256').update(content).digest('hex') });
  }
  return assets;
}

export async function runWorker({ input = process.stdin, output = process.stdout, errorOutput = process.stderr } = {}) {
  let browser = null;
  let assetsBySource = null;
  let shuttingDown = false;
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
  const closeBrowser = async () => {
    if (!browser) return;
    const owned = browser;
    browser = null;
    await owned.close();
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
      if (request.type === 'init') {
        if (browser) throw requestError('already_initialized', 'worker already owns a Chrome process');
        const borrowed = request.backend === 'browser_quiet_hidden';
        const options = borrowed ? validateBorrowedInit(request) : validateInit(request);
        const loadedAssets = {
          x: await sourceAssets(options.bridgePath, 'x'),
          facebook: await sourceAssets(options.bridgePath, 'facebook'),
        };
        browser = borrowed
          ? createBorrowedChrome(borrowedRPC.send, request.chromeVersion)
          : await launchChrome(options);
        assetsBySource = loadedAssets;
        const result = {
          pid: browser.pid,
          chromeVersion: browser.version,
          workerDriver: { name: borrowed ? 'aku-quiet-worker' : 'aku-headless-worker', version: WORKER_VERSION, protocolVersion: PROTOCOL_VERSION },
        };
        await write(responseFor(id, true, result));
        return;
      }
      if (request.type === 'capture') {
        if (!browser || !assetsBySource) throw requestError('not_initialized', 'send init before capture');
        const result = await capture(browser, assetsBySource, request.source, request.payload);
        await write(responseFor(id, true, result));
        return;
      }
      shuttingDown = true;
      await closeBrowser();
      await write(responseFor(id, true, { stopped: true }));
      input.pause?.();
      input.destroy?.();
    } catch (error) {
      const safe = {
        code: error?.code || 'worker_error',
        message: String(error?.message || 'worker request failed').slice(0, 500),
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
      queued = queued.then(() => handle(raw));
    }
  });
  input.on('end', () => {
    borrowedRPC.close();
    lineBuffer += decoder.end();
    if (lineBuffer.trim()) queued = queued.then(() => handle(lineBuffer));
    queued = queued.then(closeBrowser).catch(error => {
      if (errorOutput) errorOutput.write(`[headless-worker] shutdown error: ${String(error?.message || error).slice(0, 300)}\n`);
    });
  });
  const interrupt = () => {
    borrowedRPC.close();
    shuttingDown = true;
    void closeBrowser().catch(error => errorOutput?.write(`[headless-worker] interrupt close error: ${String(error?.message || error).slice(0, 300)}\n`));
  };
  process.once('SIGINT', interrupt);
  process.once('SIGTERM', interrupt);
  return new Promise(resolveRun => input.once('close', () => {
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
