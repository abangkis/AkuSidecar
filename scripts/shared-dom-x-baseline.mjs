import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { mkdtemp, readFile, realpath, rm, writeFile } from 'node:fs/promises';
import { basename, dirname, join, resolve } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { registeredScriptsForSources } from '../../AkuBridge/source-access-policy.js';
import { sourceAssets } from '../internal/collection/headless/worker/worker.mjs';
import { projectDOMShadow } from '../internal/collection/headless/worker/observation.mjs';

const workspace = fileURLToPath(new URL('../../', import.meta.url));
const bridge = join(workspace, 'AkuBridge');
const chrome = process.argv[2] ?? 'C:/Program Files/Google/Chrome/Application/chrome.exe';
const output = process.argv[3];
const baselinePath = process.argv[4];

function timestamp(handle, id) {
  return `<a href="https://x.com/${handle}/status/${id}"><time datetime="2026-01-02T03:04:00Z">Fixture time</time></a>`;
}

function legacy(id, { text = 'Legacy fixture body', extra = '', handle = 'legacy' } = {}) {
  return `<article data-testid="tweet"><div data-testid="User-Name">${handle}</div>${timestamp(handle, id)}` +
    `<div data-testid="tweetText">${text}</div>${extra}</article>`;
}

function wrapped(id, { text = 'Wrapped fixture body', extra = '', handle = 'wrapped' } = {}) {
  return `<div role="link" data-href="/${handle}/status/${id}"><article class="group/post">` +
    `<div class="x-middot-group"><a href="https://x.com/${handle}">${handle}</a></div>` +
    `<div dir="auto" class="whitespace-pre-wrap">${text}</div>${extra}</article></div>`;
}

const placeholder = '<div role="link" data-href="/pending/status/1234500000000000009"><article></article></div>';
const wrappedPost = wrapped('1234500000000000002');
const quoteId = '1234500000000000004';
const quote = `<div role="link" data-testid="quoteTweet"><div data-testid="User-Name">quote</div>` +
  `${timestamp('quote', quoteId)}<div data-testid="tweetText">Quoted fixture body</div></div>`;
const cases = [
  ['legacy', legacy('1234500000000000001')],
  ['wrapped_without_time', wrappedPost],
  ['mixed_layouts', legacy('1234500000000000001') + wrappedPost + placeholder],
  ['legacy_quote', legacy('1234500000000000003', { extra: quote })],
  ['wrapped_quote', wrapped('1234500000000000003', {
    extra: wrapped(quoteId, { handle: 'quote', text: 'Quoted fixture body' }),
  })],
  ['media_only', legacy('1234500000000000005', { text: '',
    extra: '<div data-testid="tweetPhoto"><img width="240" height="135" src="https://pbs.twimg.com/media/synthetic.png"></div>',
  })],
  ['placeholder', placeholder],
  ['hydration_before', '<div aria-busy="true">' + placeholder + '</div>'],
  ['hydration_after', wrappedPost],
  ['conflicting_identity', legacy('1234500000000000006', { extra: timestamp('other', '1234500000000000007') })],
  ['wrapper_timestamp_agreement', wrapped('1234500000000000002', { extra: timestamp('wrapped', '1234500000000000002') })],
  ['wrapper_timestamp_conflict', wrapped('1234500000000000002', { extra: timestamp('other', '1234500000000000007') })],
  ['quote_cannot_identify_owner', '<article data-testid="tweet"><div data-testid="User-Name">owner</div>' +
    '<div data-testid="tweetText">Owner body</div>' + quote + '</article>'],
  ['quote_identity_conflict', legacy('1234500000000000003', { extra: wrapped(quoteId, {
    handle: 'quote', text: 'Quoted fixture body', extra: timestamp('other', '1234500000000000007'),
  }) })],
  ['quote_direct_link', legacy('1234500000000000003', { extra: `<div data-testid="quoteTweet">
    <a href="/quote/status/${quoteId}">Quoted post</a><div data-testid="tweetText">Quote body</div></div>` })],
  ['quote_authored_link', legacy('1234500000000000003', { extra: `<div data-testid="quoteTweet">
    <div data-testid="tweetText"><a href="/quote/status/${quoteId}">A link in the quote body</a></div></div>` })],
  ['empty', '<p>No posts to display</p>'],
  ['login_required', '<input name="password" aria-label="Fixture password">'],
  ['challenge_required', '<section id="challenge-running">Fixture challenge</section>'],
];

function browserBaseline({ cases, assets, gate, mode }) {
  const out = document.getElementById('result');
  const main = document.querySelector('main');
  const location = new URL('https://x.com/home');
  const window = { location, innerWidth, innerHeight, scrollX: 0, scrollY: 0, scrollTo() {} };
  let receive;
  const chrome = { runtime: { onMessage: { addListener(fn) { receive = fn; } } } };
  globalThis.AkuHeadlessCapturePolicy = { allowContentExpansion: false };
  for (const code of assets) new Function('window', 'chrome', 'location', code)(window, chrome, location);
  const shadow = globalThis.AkuXDOMShadow;
  let shadowCalls = 0;
  let shadowMutations = 0;
  if (shadow) {
    const observer = new MutationObserver(() => {});
    observer.observe(document, { subtree: true, childList: true, attributes: true, characterData: true });
    globalThis.AkuXDOMShadow = { ...shadow, assess(sample) {
      observer.takeRecords();
      shadowCalls++;
      const result = shadow.assess(sample);
      shadowMutations += observer.takeRecords().length;
      return result;
    } };
  }
  const ready = new Function(gate + '\nreturn isSourceCaptureReady;')();
  const request = (message) => new Promise((resolve) => receive(message, null, resolve));
  const compactPost = (post) => ({
    id: post.platformId ?? (post.id ? `x:status:${post.id}` : null),
    permalink: post.permalink,
    identity: post.presentation?.primaryIdentity?.status ?? post.identityComparison?.sharedDecision ?? null,
    author: post.author,
    text: post.text,
    media: post.media?.length ?? 0,
    quoteId: post.quotedPost?.permalink?.match(/\/status\/(\d+)$/)?.[1] ?? null,
    quoteText: post.quotedPost?.text ?? null,
  });

  async function sampleWindowed(name) {
    const { readiness } = await request({ type: 'AKU_BROWSER_PROBE_SOURCE_READY', source: 'x' });
    const windowed = {
      adapter: readiness.adapterVersion,
      readiness: readiness.state,
      diagnosis: readiness.diagnosis,
      readinessRejections: readiness.candidateRejections ?? null,
      captureInvoked: ready(readiness),
      posts: [],
      sharedDOMShadow: readiness.sharedDOMShadow,
    };
    if (windowed.captureInvoked) {
      const result = await request({ type: 'AKU_BROWSER_COLLECT_VISIBLE', payload: {
        source: 'x', scrolls: 0, restoreScroll: false, qualityRetryBudget: 0, sourceFreshness: {},
        sourceReadiness: readiness,
      } });
      if (!result.ok) throw new Error(`${name}: Windowed capture failed: ${result.message ?? result.code}`);
      const { blocks, qualityReports } = result.observation.snapshots[0];
      if (JSON.stringify(result.observation.coverage.sharedDOMShadow) !== JSON.stringify(readiness.sharedDOMShadow)) {
        throw new Error(`${name}: Windowed observation lost shadow diagnostics`);
      }
      windowed.posts = blocks.map(compactPost);
      windowed.quality = qualityReports.map(({ verdict, issues }) => ({
        verdict, issues: issues.map(({ field, code }) => `${field}:${code}`),
      }));
    }
    return windowed;
  }

  async function sampleHeadless() {
    const snapshot = await globalThis.XHeadlessPoC.collect();
    return {
      adapter: snapshot.adapterVersion,
      readiness: snapshot.sourceReadiness ?? (snapshot.challengeDetected ? 'challenge_required' : null),
      candidateCount: snapshot.candidateCount,
      rejections: snapshot.rejectionReasons,
      posts: snapshot.posts.map(compactPost),
      sharedDOMShadow: snapshot.sharedDOMShadow,
    };
  }

  async function run() {
    const rows = [];
    for (const [name, body] of cases) {
      main.innerHTML = body;
      const result = mode === 'windowed' ? await sampleWindowed(name) : await sampleHeadless();
      rows.push({ name, result });
    }
    out.textContent = JSON.stringify({ rows, shadowCalls, shadowMutations });
  }
  const start = () => run().catch(error => {
    out.textContent = JSON.stringify({ error: String(error.stack ?? error) });
  });
  if (document.readyState === 'complete') start();
  else globalThis.addEventListener('load', start, { once: true });
}

const windowedFiles = registeredScriptsForSources(['x']).find(script => script.id === 'aku-source-x-feed').js;
const headless = (await sourceAssets(bridge, 'x')).filter(asset => asset.execute);
const digest = (content) => createHash('sha256').update(content).digest('hex');
const windowed = await Promise.all(windowedFiles.map(async relative => ({
  relative: `AkuBridge/${relative}`, content: await readFile(join(bridge, relative), 'utf8'),
})));
for (const asset of headless.filter(asset => asset.relative.startsWith('AkuBridge/'))) {
  assert.equal(asset.content, windowed.find(file => file.relative === asset.relative)?.content, asset.relative);
}
const runtimePath = 'AkuBridge/shared-dom-engine/browser-runtime.js';
const windowedRuntime = windowed.find(asset => asset.relative === runtimePath);
const headlessRuntime = headless.find(asset => asset.relative === runtimePath);
assert.ok(windowedRuntime && headlessRuntime, 'Both loaders must carry the shared browser artifact');
assert.equal(digest(windowedRuntime.content), headlessRuntime.sha256);
const workerSource = await readFile(join(bridge, 'service-worker.js'), 'utf8');
const gate = workerSource.match(/^function isSourceCaptureReady\(readiness\) \{[\s\S]*?^\}/m)?.[0];
assert.ok(gate, 'Windowed service-worker readiness gate');
const scratchRoot = await realpath(join(workspace, 'SharedTemp'));
const scratch = await mkdtemp(join(scratchRoot, 'shared-dom-x-baseline-'));

async function runFixture(mode, assets) {
  const fixture = join(scratch, `${mode}.html`);
  const spec = { cases, mode, assets: assets.map(asset => asset.content), gate };
  const script = `(${browserBaseline.toString()})(${JSON.stringify(spec)});`.replaceAll('</script', '<\\/script');
  await writeFile(fixture, `<!doctype html><meta charset="utf-8"><title>Synthetic X DOM baseline</title>
<meta http-equiv="Content-Security-Policy" content="default-src 'none'; script-src 'unsafe-inline' 'unsafe-eval'; style-src 'unsafe-inline'">
<style>article { min-height: 32px; }</style>
<base href="https://x.com/home"><main></main><pre id="result"></pre><script>${script}</script>`);
  const run = spawnSync(chrome, ['--headless', '--dump-dom', '--virtual-time-budget=5000',
    '--disable-background-networking', '--disable-component-update', '--no-first-run', '--no-default-browser-check',
    '--host-resolver-rules=MAP * ~NOTFOUND', '--window-size=1200,1000',
    `--user-data-dir=${join(scratch, `${mode}-profile`)}`, pathToFileURL(fixture).href],
  { encoding: 'utf8', timeout: 25_000, maxBuffer: 8 * 1024 * 1024, windowsHide: true });
  assert.equal(run.status, 0, String(run.error ?? run.stderr).slice(-1_200));
  const serialized = run.stdout.match(/<pre id="result">([\s\S]*?)<\/pre>/)?.[1];
  assert.ok(serialized, 'Browser fixture did not produce a result');
  const data = JSON.parse(serialized.replaceAll('&lt;', '<').replaceAll('&gt;', '>').replaceAll('&amp;', '&'));
  assert.ok(!data.error, data.error);
  assert.equal(data.rows.length, cases.length);
  assert.equal(data.shadowCalls, cases.length);
  assert.equal(data.shadowMutations, 0);
  return data;
}

try {
  const windowedResult = await runFixture('windowed', windowed);
  const headlessResult = await runFixture('headless', headless);
  const data = {
    rows: windowedResult.rows.map(({ name, result }, index) => {
      assert.equal(name, headlessResult.rows[index].name);
      return { name, windowed: result, headless: headlessResult.rows[index].result };
    }),
    shadowCalls: windowedResult.shadowCalls + headlessResult.shadowCalls,
    shadowMutations: windowedResult.shadowMutations + headlessResult.shadowMutations,
  };
  const get = name => data.rows.find(row => row.name === name);
  for (const { name, windowed, headless } of data.rows) {
    assert.deepEqual(headless.posts, windowed.posts, name);
    assert.equal(headless.readiness, windowed.readiness, name);
  }
  for (const mode of ['windowed', 'headless']) {
    assert.equal(get('legacy')[mode].posts[0].id, 'x:status:1234500000000000001');
    assert.equal(get('legacy_quote')[mode].posts[0].quoteId, quoteId);
    assert.equal(get('wrapped_quote')[mode].posts[0].quoteId, quoteId);
    assert.equal(get('wrapped_without_time')[mode].posts[0].permalink, 'https://x.com/wrapped/status/1234500000000000002');
    assert.equal(get('mixed_layouts')[mode].posts.length, 2);
    assert.equal(get('media_only')[mode].posts[0].media, 1);
    assert.equal(get('placeholder')[mode].posts.length, 0);
    assert.equal(get('hydration_before')[mode].posts.length, 0);
    assert.equal(get('hydration_after')[mode].posts.length, 1);
    assert.equal(get('conflicting_identity')[mode].posts.length, 0);
    assert.equal(get('wrapper_timestamp_conflict')[mode].posts.length, 0);
    assert.equal(get('wrapper_timestamp_agreement')[mode].posts.length, 1);
    assert.equal(get('quote_cannot_identify_owner')[mode].posts.length, 0);
    assert.equal(get('quote_identity_conflict')[mode].posts[0].quoteId, null);
    assert.equal(get('quote_direct_link')[mode].posts[0].quoteId, quoteId);
    assert.equal(get('quote_authored_link')[mode].posts[0].quoteId, null);
    assert.equal(get('login_required')[mode].readiness, 'login_required');
    assert.equal(get('challenge_required')[mode].readiness, 'challenge_required');
  }
  assert.equal(get('placeholder').windowed.readiness, 'feed_hydrating');
  assert.equal(get('hydration_after').windowed.readiness, 'feed_ready');
  assert.equal(get('conflicting_identity').headless.rejections.ambiguous_identity, 1);
  const expectedShadowCodes = {
    legacy: 'ready', wrapped_without_time: 'ready', mixed_layouts: 'ready', legacy_quote: 'ready',
    wrapped_quote: 'ready', media_only: 'ready', placeholder: 'no_valid_candidates',
    hydration_before: 'hydrating', hydration_after: 'ready', conflicting_identity: 'no_valid_candidates',
    wrapper_timestamp_agreement: 'ready', wrapper_timestamp_conflict: 'no_valid_candidates',
    quote_cannot_identify_owner: 'no_valid_candidates', quote_identity_conflict: 'ready',
    quote_direct_link: 'ready', quote_authored_link: 'ready',
    empty: 'empty_unverified', login_required: 'login_required', challenge_required: 'challenge_required',
  };
  for (const { name, windowed, headless } of data.rows) {
    assert.equal(windowed.sharedDOMShadow.code, expectedShadowCodes[name], name);
    assert.deepEqual(headless.sharedDOMShadow, windowed.sharedDOMShadow, name);
    assert.deepEqual(projectDOMShadow(headless).sharedDOMShadow, headless.sharedDOMShadow, name);
  }
  assert.equal(get('placeholder').windowed.sharedDOMShadow.rejections.missing_required_field, 1);
  assert.equal(get('mixed_layouts').windowed.sharedDOMShadow.counts.accepted, 2);
  assert.equal(get('conflicting_identity').windowed.sharedDOMShadow.rejections.identity_conflict, 1);
  let baselineSha256;
  if (baselinePath) {
    const baseline = await readFile(resolve(baselinePath), 'utf8');
    const before = new Map(JSON.parse(baseline).rows.map(row => [row.name, row]));
    const comparable = ({ adapter, sharedDOMShadow, posts, ...result }) => ({
      ...result, posts: posts.map(({ permalink, ...post }) => post),
    });
    for (const name of ['legacy', 'legacy_quote', 'media_only', 'empty', 'login_required', 'challenge_required']) {
      for (const mode of ['windowed', 'headless']) {
        assert.deepEqual(comparable(get(name)[mode]), comparable(before.get(name)[mode]), `${name} ${mode}`);
      }
    }
    baselineSha256 = digest(baseline);
  }
  const assets = [...new Map([...windowed, ...headless].map(asset => [asset.relative, asset])).values()];
  const report = JSON.stringify({
    scope: 'offline real DOM, production in-page paths, no navigation to X',
    baselineSha256,
    browserArtifact: { windowedSha256: digest(windowedRuntime.content), headlessSha256: headlessRuntime.sha256 },
    assets: Object.fromEntries([...assets, { relative: 'AkuBridge/service-worker.js', content: workerSource }]
      .map(({ relative, content }) => [relative, digest(content)])),
    ...data,
  }, null, 2) + '\n';
  if (output) await writeFile(resolve(output), report, { flag: 'wx' });
  else process.stdout.write(report);
} finally {
  const resolved = await realpath(scratch);
  assert.equal(dirname(resolved), scratchRoot);
  assert.ok(basename(resolved).startsWith('shared-dom-x-baseline-'));
  await rm(resolved, { recursive: true, force: true, maxRetries: 3, retryDelay: 200 });
}
