// Short feasibility only. No Store/native-revision authority or continuity proof.
export const QA_SHORT_MANIFEST = Object.freeze({ schema: 'aku.x.qa-short-transfer-plan.v1',
  source: 'https://api.x.com/1.1/account/settings.json', direction: 'reader_to_collector_once',
  destination: 'fresh_headless_disposable_context', maxTupleBytes: 8192, maxTotalBytes: 16384,
  tuples: Object.freeze([
    Object.freeze({ name: 'auth_token', domain: '.x.com', path: '/', secure: true, httpOnly: true, sameSite: 'None' }),
    Object.freeze({ name: 'ct0', domain: '.x.com', path: '/', secure: true, httpOnly: false, sameSite: 'Lax' }),
  ]) });
const fail = () => new Error('qa_short_feasibility_rejected');
const record = (v, keys) => {
  if (!v || typeof v !== 'object' || Array.isArray(v) || ![Object.prototype, null].includes(Object.getPrototypeOf(v))) throw fail();
  const ds = Object.getOwnPropertyDescriptors(v);
  if (Reflect.ownKeys(ds).length !== keys.length || keys.some(k => !ds[k] || !Object.hasOwn(ds[k], 'value'))) throw fail();
  return Object.fromEntries(keys.map(k => [k, ds[k].value]));
};
export function selectQAShortCookies(value, nowSeconds = Date.now() / 1000) {
  const root = record(value, ['cookies']);
  if (!Array.isArray(root.cookies) || root.cookies.length > 32 || !Number.isFinite(nowSeconds)) throw fail();
  const items = Object.getOwnPropertyDescriptors(root.cookies);
  if (Reflect.ownKeys(items).some(k => k !== 'length' && !/^(0|[1-9][0-9]*)$/.test(k))) throw fail();
  const selected = [];
  for (let i = 0; i < root.cookies.length; i++) {
    if (!items[i] || !Object.hasOwn(items[i], 'value')) throw fail();
    const raw = items[i].value;
    const name = Object.getOwnPropertyDescriptor(raw ?? {}, 'name');
    if (!name || !Object.hasOwn(name, 'value')) throw fail();
    const expected = QA_SHORT_MANIFEST.tuples.find(v => v.name === name.value);
    if (!expected) continue; // Never read incidental cookies' value properties.
    const ds = Object.getOwnPropertyDescriptors(raw);
    const allowed = ['name', 'value', 'domain', 'path', 'expires', 'size', 'httpOnly', 'secure', 'session', 'sameSite',
      'priority', 'sameParty', 'sourceScheme', 'sourcePort', 'partitionKey', 'partitionKeyOpaque'];
    if (Reflect.ownKeys(ds).some(k => !allowed.includes(k) || !Object.hasOwn(ds[k], 'value'))) throw fail();
    const c = Object.fromEntries(Object.entries(ds).map(([k, d]) => [k, d.value]));
    if (Object.keys(expected).some(k => c[k] !== expected[k]) || selected.some(v => v.name === c.name) ||
        typeof c.value !== 'string' || !c.value || c.session !== false || !Number.isFinite(c.expires) || c.expires <= nowSeconds || c.expires > Number.MAX_SAFE_INTEGER ||
        c.priority !== 'Medium' || c.sourceScheme !== 'Secure' || c.sourcePort !== 443 ||
        c.sameParty !== undefined && c.sameParty !== false || c.partitionKey !== undefined || c.partitionKeyOpaque === true) throw fail();
    const tuple = { ...expected, value: c.value, expires: c.expires, priority: c.priority, sourceScheme: c.sourceScheme, sourcePort: c.sourcePort,
      ...(c.sameParty === false ? { sameParty: false } : {}) };
    if (Buffer.byteLength(JSON.stringify(tuple), 'utf8') > 8192) throw fail();
    selected.push(tuple);
  }
  if (selected.length !== 2 || Buffer.byteLength(JSON.stringify(selected), 'utf8') > 16384) throw fail();
  return QA_SHORT_MANIFEST.tuples.map(expected => selected.find(v => v.name === expected.name));
}
export function shortQACheckpoint(observation) {
  const source = observation?.source;
  const passed = observation?.schema === 'aku.x.qa-bound-observation.v1' &&
    ['bound', 'observed_cookie_write'].includes(observation.diagnostic) && observation.identity === 'current_self_observed' &&
    observation.login === 'protected_response_observed' && observation.freshness === 'within_5s' && observation.cleanupVerified === true &&
    source?.diagnostic === 'http_200_unqualified' && source.status === 200 && source.responseFresh === true && source.completed === true &&
    source.matchingRequests === 1 && source.bodyReadAttempts === 1 && source.body?.category === 'settings_match' && source.body.settingsMatch === true &&
    observation.canAuthorize === false && observation.sessionTransferred === false;
  return Object.freeze({ schema: 'aku.x.qa-short-checkpoint.v1', passed, observation: observation ?? null,
    mutationGate: 'deferred_for_short_qa', continuity: 'unknown', canAuthorize: false });
}
export async function runQAShortFeasibility(adapter, now = () => performance.now()) {
  let expectedId = null, transferred = false, seedAttempted = false, completed = 0, overlapMS = 0, stopped = false;
  let outcome = 'checkpoint_rejected';
  const snapshots = [];
  const start = now();
  const bounded = async action => {
    let timer;
    const remaining = 60000 - (now() - start);
    if (!Number.isFinite(remaining) || remaining <= 0) { outcome = 'deadline'; throw fail(); }
    try { return await Promise.race([Promise.resolve().then(action), new Promise((_, reject) => {
      timer = setTimeout(() => { outcome = 'deadline'; stopped = true; reject(fail()); }, remaining);
    })]); } finally { clearTimeout(timer); }
  };
  const check = async role => {
    const reject = () => { outcome = role === 'reader' ? 'reader_checkpoint_rejected' : 'collector_checkpoint_rejected'; throw fail(); };
    let sample;
    try { sample = await adapter.check(role); } catch { if (stopped) throw fail(); reject(); }
    if (stopped) throw fail();
    if (sample?.checkpoint?.schema === 'aku.x.qa-short-checkpoint.v1') snapshots.push({ role, checkpoint: sample.checkpoint });
    if (!sample?.checkpoint?.passed || typeof sample.accountId !== 'string' || !/^[1-9][0-9]{0,19}$/.test(sample.accountId)) reject();
    if (expectedId === null) { if (role !== 'reader') reject(); expectedId = sample.accountId; }
    if (sample.accountId !== expectedId) { outcome = 'account_mismatch'; throw fail(); }
    if (!Number.isFinite(sample.start) || !Number.isFinite(sample.end) || sample.start < start || sample.end > now() ||
        sample.end <= sample.start || sample.end - sample.start > 5000) reject();
    const interval = sample.networkInterval;
    if (!interval || !Number.isFinite(interval.start) || !Number.isFinite(interval.end) || interval.end <= interval.start ||
        interval.end - interval.start > 5000 || Math.abs(Date.now() - interval.end) > 5000) reject();
    completed++;
    return interval;
  };
  let collectorCreated = false;
  try {
    await bounded(() => adapter.prepareCollector()); collectorCreated = true;
    await bounded(() => check('reader'));
    outcome = 'seed_rejected'; seedAttempted = true; transferred = null;
    await bounded(() => adapter.seedOnce()); transferred = true;
    await bounded(() => check('collector'));
    const pair = await bounded(() => Promise.allSettled([check('reader'), check('collector')]));
    if (pair.some(v => v.status !== 'fulfilled')) throw fail();
    const [reader, collector] = pair.map(v => v.value);
    overlapMS = Math.max(0, Math.min(reader.end, collector.end) - Math.max(reader.start, collector.start));
    outcome = 'reader_checkpoint_rejected';
    if (!await bounded(() => adapter.readerPreserved())) { outcome = 'reader_window_changed'; throw fail(); }
    if (overlapMS <= 0) { outcome = 'overlap_not_observed'; throw fail(); }
    outcome = 'short_overlap_observed';
  } catch { /* Fixed categories only; do not reflect browser or input errors. */ }
  stopped = true;
  let cleanupVerified = false;
  try { cleanupVerified = await adapter.closeCollector() === true; } catch { /* outer Job remains final authority */ }
  expectedId = null;
  const durationMS = now() - start;
  if (!cleanupVerified) outcome = 'cleanup_unverified';
  if (!Number.isFinite(durationMS) || durationMS < 0 || durationMS > 60000) outcome = 'deadline';
  return Object.freeze({ schema: 'aku.x.qa-short-concurrency.v1', outcome, collectorCreated, seedAttempted, sessionTransferred: transferred,
    checkpoints: Object.freeze([...snapshots]), completedCheckpoints: completed, overlapMS, durationMS, cleanupVerified,
    accountMatch: completed === 4 && outcome === 'short_overlap_observed' ? 'match' : 'unknown',
    continuity: 'unknown', mutationGate: 'deferred_for_short_qa', refresh: 'not_qualified',
    canAuthorize: false, nativeQualified: false });
}
