import { performance } from 'node:perf_hooks';
import { randomUUID } from 'node:crypto';
import { resolve, relative, isAbsolute, basename, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { launchFixtureChrome } from './chrome.mjs';
import { inventoryFixtureWindows } from './chrome-window-lifecycle.mjs';
import { observeSessionMutations } from './observer.mjs';
import { runXSyntheticConcurrencyHarness } from './x-concurrency-harness.mjs';

const PROVENANCE = 'synthetic-fixture';
const PROBE = 'aku.x.concurrency-probe.v1';
const failureKinds = new WeakMap();
const fail = (kind = 'adapter') => {
  const error = new Error('synthetic_concurrency_adapter_failed');
  failureKinds.set(error, kind);
  return error;
};

export function concurrencyFixtureOrigin(value) {
  if (typeof value !== 'string') throw fail();
  let url;
  try { url = new URL(value); } catch { throw fail(); }
  if (url.origin !== value || url.protocol !== 'http:' || url.hostname !== '127.0.0.1' || !url.port ||
      url.username || url.password || url.pathname !== '/' || url.search || url.hash) throw fail();
  return value;
}

export function concurrencyFixtureRoot(root) {
  if (typeof root !== 'string' || !isAbsolute(root)) throw fail();
  const parent = resolve(dirname(fileURLToPath(import.meta.url)), '../../runtime/qa/auth-session-sidecar');
  const inside = relative(parent, root);
  if (!inside || isAbsolute(inside) || inside.startsWith('..') ||
      !/^run-x-concurrency-[a-f0-9]{32}$/.test(basename(root)) || resolve(root) !== root) throw fail();
  return root;
}

// Only the one invented host-only cookie issued by the controlled Go server.
// CDP bookkeeping fields are known but do not become transfer attributes.
export function fixtureConcurrencyCookie(raw) {
  const fields = new Set(['name', 'value', 'domain', 'path', 'expires', 'size', 'httpOnly', 'secure',
    'session', 'sameSite', 'priority', 'sameParty', 'sourceScheme', 'sourcePort', 'partitionKey', 'partitionKeyOpaque']);
  if (!raw || typeof raw !== 'object' || Object.getPrototypeOf(raw) !== Object.prototype) throw fail();
  const descriptors = Object.getOwnPropertyDescriptors(raw);
  if (Reflect.ownKeys(descriptors).some(key => typeof key !== 'string' || !fields.has(key) ||
      !Object.hasOwn(descriptors[key], 'value'))) throw fail('cookie_fields');
  if (['name', 'value', 'domain', 'path', 'expires', 'httpOnly', 'secure', 'session', 'sameSite']
    .some(key => !descriptors[key] || !Object.hasOwn(descriptors[key], 'value'))) throw fail('cookie_fields');
  if (raw.name !== 'fixture_session' || typeof raw.value !== 'string' || !/^[a-f0-9]{64}$/.test(raw.value) ||
      raw.domain !== '127.0.0.1' || raw.path !== '/') throw fail('cookie_scope');
  if (raw.secure !== true || raw.httpOnly !== true || raw.sameSite !== 'Lax' || raw.session !== false) throw fail('cookie_attributes');
  if (!Number.isFinite(raw.expires) || raw.expires <= 0 || raw.expires > Number.MAX_SAFE_INTEGER) throw fail('cookie_expiry');
  if (raw.partitionKey !== undefined || raw.partitionKeyOpaque === true || raw.sameParty === true) throw fail('cookie_partition');
  return Object.freeze({ name: raw.name, value: raw.value, domain: raw.domain, path: raw.path,
    hostOnly: true, secure: true, httpOnly: true, sameSite: 'Lax', expires: raw.expires, partitionKey: null });
}

export function createXSyntheticChromeAdapter({ executable, root, origin, launch = launchFixtureChrome,
  clock = () => performance.now() } = {}) {
  concurrencyFixtureOrigin(origin);
  concurrencyFixtureRoot(root);
  if (typeof executable !== 'string' || !executable || typeof root !== 'string' || !root ||
      typeof launch !== 'function' || typeof clock !== 'function') throw fail();
  let state = 'new', stage = 'new', owner, collector, ownerPage, derivedPage, derivedContext;
  let ownerObserver, derivedObserver, callback, pendingFault, baselineCookie;
  let closing = false, allowedContext = null, injection = null;
  let mutationObserved = 0, lossObserved = 0;
  let confirmDerivedGone;
  const derivedGone = new Promise(resolve => { confirmDerivedGone = resolve; });
  const browsers = [], protocolSubscriptions = [], targets = new Set(), retiredTargets = new Set();
  const ownerId = 'fixture-owner', generation = `generation-${randomUUID()}`;
  const binding = { ownerId, generation };
  function notify(kind) {
    if (closing) return;
    if (kind === 'mutation') mutationObserved++;
    else lossObserved++;
    pendingFault ??= kind;
    callback?.({ schema: 'aku.x.synthetic-observation.v1', provenance: PROVENANCE, kind });
  }
  async function observeBrowser(browser) {
    protocolSubscriptions.push(browser.subscribeProtocol(event => {
      if (event.method === 'Target.targetDestroyed' || event.method === 'Target.targetCrashed') {
        if (event.params?.targetId === derivedPage?.targetId) confirmDerivedGone();
        if (targets.has(event.params?.targetId)) notify('coverage_loss');
      }
      if (event.method === 'Target.targetCreated' || event.method === 'Target.targetInfoChanged') {
        const info = event.params?.targetInfo;
        if (!info || targets.has(info.targetId) || retiredTargets.has(info.targetId)) return;
        if (info.type === 'page' && allowedContext && info.browserContextId === allowedContext) return;
        if (info.url === origin || info.url?.startsWith(origin + '/')) notify('coverage_loss');
      }
    }, () => notify('coverage_loss')));
    await browser.send('Target.setDiscoverTargets', { discover: true });
  }
  async function observePage(page, role) {
    return observeSessionMutations(page, { origin, sessionId: `${role}-session`, clientId: role,
      onUnexpected: event => notify(event.kind === 'unexpected_session_mutation' ? 'mutation' : 'coverage_loss') });
  }
  async function cookies(browser, page, contextId) {
    const attached = await browser.send('Target.attachToTarget', { targetId: page.targetId, flatten: true });
    try {
      const result = await browser.sendSession(attached.sessionId, 'Network.getCookies', { urls: [origin + '/'] });
      if (!Array.isArray(result.cookies) || result.cookies.length !== 1) throw fail();
      try { return fixtureConcurrencyCookie(result.cookies[0]); }
      catch (error) { stage = `cookie_${failureKinds.get(error) ?? 'query'}`; throw fail(); }
    } finally {
      await browser.send('Target.detachFromTarget', { sessionId: attached.sessionId });
    }
  }
  async function windows() {
    const inventory = await inventoryFixtureWindows(owner, origin, ownerPage.targetId);
    if (inventory.length < 1 || !inventory.some(window => window.bootstrap)) throw fail();
    return inventory.map(window => `window-${window.id}`);
  }
  function probe(response, role, pairIndex) {
    if (response?.status !== 200 || response.body?.account !== 'alpha' || response.body?.epoch !== 1) throw fail();
    return { schema: PROBE, provenance: PROVENANCE, role, pairIndex, ...binding,
      profileId: role === 'reader' ? 'fixture-reader-profile' : 'fixture-collector-profile',
      contextId: role === 'reader' ? ownerPage.targetId : derivedPage.targetId,
      accountId: 'alpha', probeSchema: PROBE, httpStatus: 200, authState: 'authenticated', observedAt: clock() };
  }
  async function closeDerived() {
    if (!derivedContext) return { closed: true };
    // Retire only this verified context, never a reader window.
    {
      derivedObserver?.close();
      if (derivedPage) { targets.delete(derivedPage.targetId); retiredTargets.add(derivedPage.targetId); }
      await collector.send('Target.disposeBrowserContext', { browserContextId: derivedContext });
      const contexts = await collector.send('Target.getBrowserContexts');
      if (!Array.isArray(contexts.browserContextIds) || contexts.browserContextIds.includes(derivedContext)) throw fail();
      derivedContext = null;
      return { closed: true };
    }
  }
  return Object.freeze({
    diagnostic() { return `synthetic_concurrency_${stage}`; },
    async prepare() {
      if (state !== 'new') throw fail();
      state = 'preparing';
      stage = 'launch_reader';
      owner = await launch(executable, root, 'owner', 'owner', 'native_reader'); browsers.push(owner);
      stage = 'launch_collector';
      collector = await launch(executable, root, 'collector'); browsers.push(collector);
      if (owner.pid === collector.pid || owner.profile === collector.profile) throw fail();
      stage = 'initial_inventory';
      const initial = await owner.send('Target.getTargets');
      const blank = initial.targetInfos?.filter(target => target.type === 'page' && target.url === 'about:blank');
      if (blank?.length !== 1) throw fail();
      targets.add(blank[0].targetId);
      stage = 'browser_observation';
      await observeBrowser(owner); await observeBrowser(collector);
      stage = 'reader_page';
      ownerPage = await owner.pageForTarget(origin, blank[0].targetId);
      stage = 'reader_observation';
      ownerObserver = await observePage(ownerPage, 'reader');
      // Exactly one nonce-bound, invented login; no broad initialization exemption.
      stage = 'synthetic_login';
      const login = await ownerObserver.authorizeRefresh(request => request('/login', 'POST'));
      if (login?.status !== 200 || login.body?.account !== 'alpha') throw fail();
      stage = 'owner_probe';
      const self = await ownerPage.request('/probe');
      if (self?.status !== 200 || self.body?.account !== 'alpha') throw fail();
      await ownerObserver.flush();
      stage = 'baseline_cookie';
      baselineCookie = await cookies(owner, ownerPage);
      stage = 'reader_inventory';
      const windowIds = await windows();
      if (pendingFault) throw fail();
      state = 'prepared';
      stage = 'prepared';
      return { readerPID: owner.pid, collectorPID: collector.pid, distinctProfiles: true, readerWindows: windowIds.length };
    },
    async run(scenario) {
      if (state !== 'prepared' || !['overlap', 'mutation', 'coverage_loss'].includes(scenario)) throw fail();
      state = 'running';
      stage = 'running';
      const windowIds = await windows();
      const now = clock();
      const reader = { schema: 'aku.x.synthetic-reader-evidence.v1', provenance: PROVENANCE, origin, ...binding,
        profileId: 'fixture-reader-profile', contextId: ownerPage.targetId, accountId: 'alpha', probeSchema: PROBE,
        authState: 'authenticated', observedAt: now, windowIds };
      const { value, ...tuple } = baselineCookie;
      const adapters = {
        subscribeObservation(listener) {
          callback = listener;
          if (pendingFault) listener({ schema: 'aku.x.synthetic-observation.v1', provenance: PROVENANCE, kind: pendingFault });
          return () => { callback = null; };
        },
        async inspectReader() {
          await ownerObserver.flush();
          return { schema: 'aku.x.reader-window-inventory.v1', provenance: PROVENANCE, ...binding,
            profileId: reader.profileId, contextId: reader.contextId, windowIds: await windows() };
        },
        async createCollector() {
          const created = await collector.send('Target.createBrowserContext', { disposeOnDetach: true });
          if (typeof created.browserContextId !== 'string' || !created.browserContextId) throw fail();
          derivedContext = created.browserContextId; allowedContext = derivedContext;
          try {
            derivedPage = await collector.page(origin, derivedContext);
            targets.add(derivedPage.targetId);
          } finally { allowedContext = null; }
          derivedObserver = await observePage(derivedPage, 'collector');
          return { schema: 'aku.x.collector-context.v1', provenance: PROVENANCE, ...binding,
            profileId: 'fixture-collector-profile', contextId: derivedPage.targetId, resource: { context: derivedContext } };
        },
        async readCookieTuples() { return [await cookies(owner, ownerPage)]; },
        async seedCollector({ cookies: selected, collector: resource }) {
          if (selected.length !== 1 || resource.context !== derivedContext || !derivedContext) throw fail();
          const item = selected[0];
          await collector.send('Storage.setCookies', { browserContextId: derivedContext, cookies: [{ name: item.name,
            value: item.value, url: origin + '/', path: item.path, secure: item.secure, httpOnly: item.httpOnly,
            sameSite: item.sameSite, expires: item.expires }] });
          const seeded = await cookies(collector, derivedPage, derivedContext);
          if (JSON.stringify(seeded) !== JSON.stringify(selected[0])) throw fail();
          const self = await derivedPage.request('/probe');
          if (self.status !== 200 || self.body?.account !== 'alpha') throw fail();
          await derivedObserver.flush();
          return { seeded: true, tupleCount: 1 };
        },
        async readProbe({ role, pairIndex }) {
          const page = role === 'reader' ? ownerPage : derivedPage;
          const response = page.request(role === 'reader' ? '/reader' : '/collect');
          response.catch(() => {});
          if (pairIndex === 0 && role === 'collector' && scenario !== 'overlap') {
            injection = (async () => {
              const barrier = await ownerPage.request('/read-barrier');
              if (barrier.status !== 200 || barrier.body?.ready !== true) throw fail();
              if (scenario === 'mutation') {
                await ownerPage.request('/rotate', 'POST');
                await ownerObserver.flush();
              } else {
                // Deliberately unexpected: do not retire this target first.
                await derivedPage.close();
              }
            })();
            injection.catch(() => {});
          }
          // A destroyed target may leave Runtime.evaluate without a reply.
          // Retire this read only after its exact target's destruction event;
          // creation/seed effects remain tracked, and context disposal is verified.
          const result = await (role === 'collector' ? Promise.race([response,
            derivedGone.then(() => { throw fail(); })]) : response);
          if (injection) await injection;
          await (role === 'reader' ? ownerObserver : derivedObserver).flush();
          return probe(result, role, pairIndex);
        },
        closeCollector: closeDerived,
      };
      const result = await runXSyntheticConcurrencyHarness({ origin, provenance: PROVENANCE,
        ownerBinding: { schema: 'aku.x.synthetic-owner-binding.v1', provenance: PROVENANCE, ...binding,
          leaseId: `lease-${randomUUID()}`, observedAt: now }, reader,
        manifest: { schema: 'aku.x.cookie-manifest.synthetic.v1', provenance: PROVENANCE, origin, tuples: [tuple] },
        coverage: { schema: 'aku.x.synthetic-coverage.v1', provenance: PROVENANCE, readerStartup: true,
          collectorStartup: true, targetCoverage: true, workerCoverage: true, detachCoverage: true,
          mutationCoverage: true, identityCoverage: true }, adapters, clock, signal: null, timeoutMs: 6000, cleanupWaitMs: 1000 });
      baselineCookie = null;
      if (injection) await injection.catch(() => {});
      if (!result.cleanupStatus().complete) {
        let timer;
        await Promise.race([result.waitForCleanup(), new Promise(resolve => { timer = setTimeout(resolve, 1500); })]);
        clearTimeout(timer);
      }
      const cleanup = result.cleanupStatus();
      if (!cleanup.complete) { stage = cleanup.pending ? 'cleanup_pending' : 'cleanup_failed'; throw fail(); }
      if (scenario === 'mutation' && mutationObserved === 0) { stage = 'mutation_unobserved'; throw fail(); }
      if (scenario === 'coverage_loss' && lossObserved === 0) { stage = 'target_loss_unobserved'; throw fail(); }
      // Inventory is independent of authentication/observer health. Even a
      // fenced run must preserve the existing reader/bootstrap window set.
      const remainingWindows = await windows();
      if (remainingWindows.length !== windowIds.length || remainingWindows.some(id => !windowIds.includes(id))) {
        stage = 'reader_windows_changed'; throw fail();
      }
      state = 'finished';
      return { schema: 'aku.x.concurrency-chrome.synthetic.v1', scenario,
        receipt: { ...result.receipt, cleanupPending: cleanup.pending, cleanupFailed: cleanup.failed },
        canAuthorize: false, nativeQualified: false, realAuthTested: false };
    },
    async close() {
      closing = true; callback = null; baselineCookie = null;
      ownerObserver?.close(); derivedObserver?.close();
      for (const remove of protocolSubscriptions.splice(0)) remove();
      const results = await Promise.allSettled(browsers.map(browser => browser.close()));
      state = 'closed';
      if (results.some(result => result.status === 'rejected') || browsers.some(browser => !browser.isExited())) throw fail();
      return { closed: true };
    },
  });
}
