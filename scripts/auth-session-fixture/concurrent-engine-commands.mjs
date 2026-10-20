// Private synthetic-only commands. No caller-selected site, CDP method, cookie
// name/attributes, profile, context or activation target crosses this boundary.
export function concurrentEngineCommand(raw) {
  const fail = () => { throw new Error('fixture_concurrent_command_invalid'); };
  if (!raw || typeof raw !== 'object' || Array.isArray(raw) || Object.getPrototypeOf(raw) !== Object.prototype) fail();
  const fields = {
    fixture_bootstrap: ['op', 'withholdReady'],
    launch: ['id', 'op', 'role', 'nativeReaderStartup'],
    login: ['id', 'op', 'owner'], probe: ['id', 'op', 'owner'], collect: ['id', 'op', 'owner'],
    reader_request: ['id', 'op', 'owner'], inventory: ['id', 'op', 'owner'],
    open_independent_window: ['id', 'op', 'owner'], release: ['id', 'op', 'owner'],
    close_window: ['id', 'op', 'owner', 'index'], seed_fixture: ['id', 'op', 'owner', 'value'],
  }[raw.op];
  if (!fields || Reflect.ownKeys(raw).length !== fields.length || Reflect.ownKeys(raw).some(key => !fields.includes(key))) fail();
  if (raw.op === 'fixture_bootstrap') { if (raw.withholdReady !== false) fail(); return { ...raw }; }
  if (!Number.isSafeInteger(raw.id) || raw.id < 1) fail();
  if (raw.op === 'launch') {
    if (!['owner', 'collector'].includes(raw.role) || raw.nativeReaderStartup !== (raw.role === 'owner')) fail();
  } else if (!Number.isSafeInteger(raw.owner) || raw.owner < 1) fail();
  if (raw.op === 'seed_fixture' && (typeof raw.value !== 'string' || !/^[a-f0-9]{64}$/.test(raw.value))) fail();
  if (raw.op === 'close_window' && (!Number.isSafeInteger(raw.index) || raw.index < 0 || raw.index > 15)) fail();
  return { ...raw };
}

export function concurrentFixtureCookie(value, origin) {
  if (typeof value !== 'string' || !/^[a-f0-9]{64}$/.test(value)) throw new Error('fixture_concurrent_seed_invalid');
  let parsed;
  try { parsed = new URL(origin); } catch { throw new Error('fixture_concurrent_seed_invalid'); }
  if (parsed.origin !== origin || parsed.protocol !== 'http:' || parsed.hostname !== '127.0.0.1' || !parsed.port) throw new Error('fixture_concurrent_seed_invalid');
  return { name: 'fixture_session', value, url: origin + '/', path: '/', secure: true, httpOnly: true, sameSite: 'Lax', expires: Math.floor(Date.now() / 1000) + 3600 };
}
