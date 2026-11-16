import { readFile, writeFile, mkdir } from 'node:fs/promises';
import { resolve } from 'node:path';
import { pathToFileURL } from 'node:url';
import { createHash } from 'node:crypto';
import { root, workspace, profile, launchChrome } from './chrome.mjs';
import { probeQuoteNavigation, applyQuoteRecovery } from './quote-navigation.mjs';

export function parseOptions(args) {
  const options = { source: 'x', url: 'https://x.com/home', maxPosts: 20, maxScrolls: 6, waitMs: 30000, login: false, screenshot: false };
  const fields = { '--source': 'source', '--url': 'url', '--max-posts': 'maxPosts', '--max-scrolls': 'maxScrolls', '--wait-ms': 'waitMs' };
  for (let i = 0; i < args.length; i++) {
    const flag = args[i];
    if (flag === '--login') options.login = true;
    else if (flag === '--use-capture-profile') options.useCaptureProfile = true;
    else if (flag === '--screenshot') options.screenshot = true;
    else if (flag === '--help') options.help = true;
    else if (fields[flag]) {
      if (!args[i + 1] || args[i + 1].startsWith('--')) throw new Error(`Missing value for ${flag}`);
      options[fields[flag]] = flag === '--url' || flag === '--source' ? args[++i] : Number(args[++i]);
    } else throw new Error(`Unknown argument: ${flag}`);
  }
  if (!['x', 'facebook'].includes(options.source)) throw new Error('Supported PoC sources: x, facebook.');
  if (options.source === 'facebook' && !args.includes('--url')) options.url = 'https://www.facebook.com/';
  const u = new URL(options.url);
  if (options.login && options.useCaptureProfile) throw new Error('Manual login mode cannot use the existing capture profile.');
  const hosts = options.source === 'facebook' ? ['facebook.com', 'www.facebook.com'] : ['x.com'];
  if (u.protocol !== 'https:' || !hosts.includes(u.hostname) || u.port || u.username || u.password) throw new Error(`--url must be an HTTPS ${hosts.join('/')} URL without credentials or a custom port.`);
  for (const [name, min, max] of [['maxPosts', 1, 100], ['maxScrolls', 0, 20], ['waitMs', 1000, 60000]]) {
    if (!Number.isInteger(options[name]) || options[name] < min || options[name] > max) throw new Error(`Invalid ${name}: allowed ${min}..${max}`);
  }
  return options;
}

export async function loadScripts(source = 'x') {
  const files = source === 'facebook'
    ? ['AkuBridge/source-adapter-runtime.js', 'AkuBridge/adapters/facebook-adapter.js', 'AkuSidecar/experiments/x-headless/facebook-boundary.js', 'AkuSidecar/experiments/x-headless/facebook-time-evidence.js', 'AkuSidecar/experiments/x-headless/facebook-extract.js']
    : ['AkuBridge/source-adapter-runtime.js', 'AkuBridge/adapters/x-adapter.js', 'AkuSidecar/experiments/x-headless/quote-identity.js', 'AkuSidecar/experiments/x-headless/extract.js'];
  return Promise.all(files.map(async file => {
    const content = await readFile(resolve(workspace, file), 'utf8');
    return { file, content, sha256: createHash('sha256').update(content).digest('hex') };
  }));
}
export async function inject(browser, scripts) {
  for (const script of scripts) await browser.evaluate(script.content);
}
const collectExpression = '(async () => JSON.stringify(await globalThis.XHeadlessPoC.collect()))()';
export async function collect(browser) {
  const targets = await browser.evaluate('globalThis.XHeadlessPoC?.prepareEvidenceTargets?.() || []');
  for (const target of targets) {
    // CDP mouse movement affects only the owned headless tab, never the OS cursor.
    await browser.send('Input.dispatchMouseEvent', { type: 'mouseMoved', x: target.x, y: target.y });
    await new Promise(resolve => setTimeout(resolve, 650));
    await browser.evaluate(`globalThis.XHeadlessPoC.recordHoverEvidence(${JSON.stringify(target.token)})`);
  }
  if (targets.length) await browser.send('Input.dispatchMouseEvent', { type: 'mouseMoved', x: 0, y: 0 });
  return JSON.parse(await browser.evaluate(collectExpression));
}
const delay = ms => new Promise(r => setTimeout(r, ms));

export async function capture(options, captureRegistration = null) {
  const started = Date.now();
  const runDir = resolve(root, 'runs', new Date().toISOString().replace(/[:.]/g, '-') + '-' + process.pid);
  await mkdir(runDir, { recursive: true });
  const report = { schema: `aku.${options.source || 'x'}-headless-poc.v1`, source: options.source || 'x', startedAt: new Date(started).toISOString(),
    status: 'starting', headless: true, profileScope: 'poc_only', target: options.url,
    limits: options, posts: [], samples: [], limitations: ['not_imported_into_sidecar', 'no_freshness_parity_claim', 'no_production_quality_validation'] };
  let browser;
  let interrupted = false;
  const interrupt = () => { interrupted = true; void browser?.close().catch(() => {}); };
  process.once('SIGINT', interrupt);
  process.once('SIGTERM', interrupt);
  try {
    const scripts = await loadScripts(options.source);
    report.sources = scripts.map(({ file, sha256 }) => ({ file, sha256 }));
    const existingCapture = options.useCaptureProfile ? captureRegistration ?? await (await import('./capture-profile.mjs')).configuredCapture() : null;
    if (existingCapture) {
      report.profileScope = 'existing_registered_capture';
      report.profilePath = existingCapture.profilePath;
      report.profileDirectory = existingCapture.directory;
    }
    browser = await launchChrome({ existingCapture });
    report.browserExecutable = browser.binary;
    report.browser = { ...browser.version, binarySha256: browser.binarySha256 };
    await browser.send('Emulation.setDeviceMetricsOverride', { width: 1280, height: 900, deviceScaleFactor: 1, mobile: false });
    const navigation = await browser.send('Page.navigate', { url: options.url });
    if (navigation.errorText) throw new Error(`Navigation failed: ${navigation.errorText}`);
    const deadline = Date.now() + options.waitMs;
    let snapshot;
    while (Date.now() < deadline) {
      if (interrupted) throw new Error('Capture interrupted by user.');
      await delay(750);
      // Navigation replaces the JS context. Only retry that known transient.
      try {
        await inject(browser, scripts);
        snapshot = await collect(browser);
      } catch (error) {
        if (/context.*destroyed|Cannot find context/i.test(error.message)) continue;
        throw error;
      }
      if (snapshot.posts.length || snapshot.loginRequired || snapshot.challengeDetected || snapshot.sourceUnavailable) break;
    }
    if (!snapshot) throw new Error('No page snapshot within readiness deadline.');
    const quoteResolution = options.source === 'facebook' ? null : await probeQuoteNavigation(browser, snapshot, options, async () => {
      await inject(browser, scripts);
      return collect(browser);
    });
    if (quoteResolution) report.quoteIdentityProbe = quoteResolution.probe;
    const quoteRecovery = quoteResolution?.recovery;
    const posts = new Map();
    let duplicateObservations = 0;
    let unchanged = 0;
    for (let round = 0; round <= options.maxScrolls; round++) {
      if (interrupted) throw new Error('Capture interrupted by user.');
      if (round > 0) {
        await browser.evaluate('globalThis.XHeadlessPoC.scrollNext ? globalThis.XHeadlessPoC.scrollNext() : window.scrollBy(0, Math.round(innerHeight * 0.8))');
        await delay(1200);
        snapshot = await collect(browser);
      }
      // One bounded hydration observation; no media clicks or playback commands.
      await delay(750);
      snapshot = applyQuoteRecovery(await collect(browser), quoteRecovery);
      const previous = posts.size;
      for (const post of snapshot.posts) {
        if (posts.has(post.id)) {
          duplicateObservations++;
          // Keep later hydrated evidence for an existing identity.
          const old = posts.get(post.id);
          if (post.media.length >= old.media.length && post.text.length >= old.text.length) posts.set(post.id, post);
        } else if (posts.size < options.maxPosts) posts.set(post.id, post);
      }
      const { posts: ignored, ...sample } = snapshot;
      report.samples.push({ round, observedPosts: snapshot.posts.length, uniquePosts: posts.size,
        newPosts: posts.size - previous, elapsedMs: Date.now() - started, ...sample });
      unchanged = posts.size === previous ? unchanged + 1 : 0;
      if (snapshot.loginRequired || snapshot.challengeDetected || snapshot.sourceUnavailable || posts.size >= options.maxPosts || unchanged >= 3) {
        report.stopReason = snapshot.loginRequired ? 'login_required' : snapshot.challengeDetected ? 'challenge_required'
          : snapshot.sourceUnavailable ? 'source_unavailable' : posts.size >= options.maxPosts ? 'post_limit' : 'three_rounds_without_new_identity';
        break;
      }
    }
    report.posts = [...posts.values()];
    report.duplicateObservations = duplicateObservations;
    report.stopReason ??= 'scroll_limit';
    report.status = snapshot.challengeDetected ? 'challenge_required' : snapshot.loginRequired ? 'login_required' : snapshot.sourceUnavailable ? 'source_unavailable'
      : report.posts.length ? 'captured_partial' : 'empty_unverified';
    if (options.screenshot) {
      const screenshot = await browser.send('Page.captureScreenshot', { format: 'png' });
      await writeFile(resolve(runDir, 'page.png'), Buffer.from(screenshot.data, 'base64'));
    }
  } catch (error) {
    report.status = 'error'; report.error = error.message;
  } finally {
    process.removeListener('SIGINT', interrupt);
    process.removeListener('SIGTERM', interrupt);
    if (browser) {
      report.documentResponses = browser.documentResponses();
      report.mediaResponses = browser.mediaResponses();
      try { await browser.close(); } catch (error) { report.shutdownError = error.message; }
    }
    report.durationMs = Date.now() - started;
    const output = resolve(runDir, 'capture.json');
    await writeFile(output, JSON.stringify(report, null, 2) + '\n');
    console.log(JSON.stringify({ status: report.status, posts: report.posts.length, durationMs: report.durationMs, output, ...(report.error ? { error: report.error } : {}) }));
  }
  return report;
}

async function login(options) {
  console.log(`Opening a visible PoC browser with its OWN profile: ${profile}`);
  console.log(`Log into ${options.source} manually. Close all PoC browser windows when finished. No credentials are read or exported.`);
  const browser = await launchChrome({ headed: true, timeoutMs: 30000 });
  const stop = () => browser.close().catch(() => {});
  process.once('SIGINT', stop);
  try {
    await browser.send('Page.navigate', { url: options.source === 'facebook' ? 'https://www.facebook.com/login/' : 'https://x.com/i/flow/login' });
    await browser.exited;
  } finally { process.removeListener('SIGINT', stop); await browser.close(); }
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  try {
    const options = parseOptions(process.argv.slice(2));
    if (options.help) console.log('node capture.mjs [--url https://x.com/home] [--max-posts 20] [--max-scrolls 6] [--wait-ms 30000] [--screenshot] [--use-capture-profile]\nnode capture.mjs --login  (visible manual login, PoC profile only)\nnode shared-profile-test.mjs (controlled stop/test/restore of registered AkuSidecar)');
    else if (options.login) await login(options);
    else { const report = await capture(options); if (report.status !== 'captured_partial') process.exitCode = 2; }
  } catch (error) { console.error(error.message); process.exitCode = 1; }
}
