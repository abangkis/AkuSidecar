import assert from 'node:assert/strict';
import test from 'node:test';
import vm from 'node:vm';
import { Session } from 'node:inspector';

import {
  BOOTSTRAP_CURRENT_SELF_EXPRESSION,
  inspectBootstrapCurrentSelf,
  validateBootstrapCurrentSelf,
} from './x-bootstrap-current-self.mjs';

const TARGET_LOCATION = { origin: 'https://x.com', pathname: '/home' };

function makePage(state, includeState = true) {
  const page = { location: { ...TARGET_LOCATION } };
  if (includeState) page.__INITIAL_STATE__ = state;
  return page;
}

function getter(target, key, counter) {
  Object.defineProperty(target, key, {
    configurable: true,
    enumerable: true,
    get() {
      counter.count += 1;
      throw new Error('private getter was invoked');
    },
  });
  return target;
}

function baseDto() {
  return {
    schema: 'aku.x.bootstrap-current-self.v1',
    diagnostic: 'projected',
    bootstrapState: 'object',
    sessionState: 'object',
    idFields: { user_id: 'absent', user_id_str: 'string' },
    stableIdStringCandidates: 1,
    sessionIdsAgree: null,
    authFields: {
      loggedIn: 'false',
      isAuthenticated: 'absent',
      authenticated: 'absent',
      isActive: 'absent',
    },
    entityChecks: { normalizedUsers: 'match', rootUsers: 'absent' },
    canAuthorize: false,
    continuityProven: false,
  };
}

test('projects one valid session ID and a matching normalized user entity', () => {
  const id = '12345678901234567890';
  const dto = inspectBootstrapCurrentSelf(
    makePage({
      session: {
        user_id_str: id,
        loggedIn: true,
        isAuthenticated: false,
        authenticated: true,
        isActive: true,
      },
      entities: { users: { entities: { [id]: { id_str: id } } } },
    }),
  );

  assert.deepEqual(dto, {
    ...baseDto(),
    idFields: { user_id: 'absent', user_id_str: 'string' },
    authFields: {
      loggedIn: 'true',
      isAuthenticated: 'false',
      authenticated: 'true',
      isActive: 'true',
    },
  });
  assert.deepEqual(validateBootstrapCurrentSelf(dto), dto);
});

test('requires present session IDs to agree before checking entities', () => {
  const equalId = '24680';
  const agreed = inspectBootstrapCurrentSelf(
    makePage({
      session: { user_id: equalId, user_id_str: equalId },
      users: { entities: { [equalId]: { id: equalId, id_str: equalId } } },
    }),
  );
  assert.equal(agreed.stableIdStringCandidates, 2);
  assert.equal(agreed.sessionIdsAgree, true);
  assert.equal(agreed.entityChecks.rootUsers, 'match');

  const conflicted = inspectBootstrapCurrentSelf(
    makePage({
      session: { user_id: '24680', user_id_str: '13579' },
      entities: { users: { entities: { '24680': { id_str: '24680' } } } },
    }),
  );
  assert.equal(conflicted.stableIdStringCandidates, 2);
  assert.equal(conflicted.sessionIdsAgree, false);
  assert.deepEqual(conflicted.entityChecks, { normalizedUsers: 'unknown', rootUsers: 'unknown' });
});

test('compares only own entity ID fields and marks invalid or conflicting values as mismatch', () => {
  const userId = '31415';
  const inheritedOnly = Object.create({ id_str: userId });
  const cases = [
    { entry: { id_str: 'not-an-id' }, expected: 'invalid' },
    { entry: { id: 31415 }, expected: 'invalid' },
    { entry: { id_str: userId, id: '27182' }, expected: 'mismatch' },
    { entry: inheritedOnly, expected: 'absent' },
    { entry: {}, expected: 'absent' },
  ];

  for (const item of cases) {
    const dto = inspectBootstrapCurrentSelf(
      makePage({
        session: { user_id_str: userId },
        entities: { users: { entities: { [userId]: item.entry } } },
      }),
    );
    assert.equal(dto.entityChecks.normalizedUsers, item.expected);
  }
});

test('does not select missing, numeric, malformed, or oversized session IDs', () => {
  const cases = [
    { session: {}, expected: { user_id: 'absent', user_id_str: 'absent' } },
    { session: { user_id: 12 }, expected: { user_id: 'number', user_id_str: 'absent' } },
    { session: { user_id_str: '0' }, expected: { user_id: 'absent', user_id_str: 'invalid' } },
    {
      session: { user_id_str: '123456789012345678901' },
      expected: { user_id: 'absent', user_id_str: 'invalid' },
    },
  ];

  for (const item of cases) {
    const dto = inspectBootstrapCurrentSelf(makePage({ session: item.session }));
    assert.deepEqual(dto.idFields, item.expected);
    assert.equal(dto.stableIdStringCandidates, 0);
    assert.equal(dto.sessionIdsAgree, null);
    assert.deepEqual(dto.entityChecks, { normalizedUsers: 'unknown', rootUsers: 'unknown' });
  }
});

test('ignores nested authors and handles when choosing current self', () => {
  const id = '98765';
  const nested = inspectBootstrapCurrentSelf(
    makePage({
      session: { screen_name: 'only-a-handle' },
      timeline: { instructions: [{ entries: [{ content: { itemContent: { user: { id_str: id } } } }] }] },
      entities: { users: { entities: { [id]: { id_str: id } } } },
    }),
  );
  assert.deepEqual(nested.idFields, { user_id: 'absent', user_id_str: 'absent' });
  assert.equal(nested.stableIdStringCandidates, 0);
  assert.deepEqual(nested.entityChecks, { normalizedUsers: 'unknown', rootUsers: 'unknown' });
});

test('keeps false and missing auth observations distinct and never grants authority', () => {
  const dto = inspectBootstrapCurrentSelf(
    makePage({ session: { user_id_str: '777', loggedIn: false, isActive: true } }),
  );
  assert.equal(dto.authFields.loggedIn, 'false');
  assert.equal(dto.authFields.isAuthenticated, 'absent');
  assert.equal(dto.authFields.authenticated, 'absent');
  assert.equal(dto.authFields.isActive, 'true');
  assert.equal(dto.canAuthorize, false);
  assert.equal(dto.continuityProven, false);
});

test('checks the page location before touching bootstrap state', () => {
  const counter = { count: 0 };
  const page = { location: { origin: 'https://example.com', pathname: '/home' } };
  getter(page, '__INITIAL_STATE__', counter);

  const dto = inspectBootstrapCurrentSelf(page);
  assert.equal(dto.diagnostic, 'wrong_page');
  assert.equal(dto.bootstrapState, 'absent');
  assert.equal(counter.count, 0);
});

test('does not invoke accessors on location, bootstrap state, session, ID, or auth paths', () => {
  const locationCounter = { count: 0 };
  const location = { pathname: '/home' };
  getter(location, 'origin', locationCounter);
  const pageWithLocationGetter = { location, __INITIAL_STATE__: { session: {} } };
  assert.equal(inspectBootstrapCurrentSelf(pageWithLocationGetter).diagnostic, 'wrong_page');
  assert.equal(locationCounter.count, 0);

  const bootstrapCounter = { count: 0 };
  const pageWithBootstrapGetter = { location: { ...TARGET_LOCATION } };
  getter(pageWithBootstrapGetter, '__INITIAL_STATE__', bootstrapCounter);
  const bootstrapResult = inspectBootstrapCurrentSelf(pageWithBootstrapGetter);
  assert.equal(bootstrapResult.bootstrapState, 'accessor');
  assert.equal(bootstrapCounter.count, 0);

  const sessionCounter = { count: 0 };
  const stateWithSessionGetter = {};
  getter(stateWithSessionGetter, 'session', sessionCounter);
  const sessionResult = inspectBootstrapCurrentSelf(makePage(stateWithSessionGetter));
  assert.equal(sessionResult.sessionState, 'accessor');
  assert.equal(sessionCounter.count, 0);

  const idCounter = { count: 0 };
  const sessionWithIdGetter = { user_id: '888' };
  getter(sessionWithIdGetter, 'user_id_str', idCounter);
  const idResult = inspectBootstrapCurrentSelf(makePage({ session: sessionWithIdGetter }));
  assert.equal(idResult.idFields.user_id_str, 'accessor');
  assert.equal(idResult.stableIdStringCandidates, 1);
  assert.equal(idCounter.count, 0);

  const authCounter = { count: 0 };
  const sessionWithAuthGetters = { user_id_str: '888' };
  for (const key of ['loggedIn', 'isAuthenticated', 'authenticated', 'isActive']) {
    getter(sessionWithAuthGetters, key, authCounter);
  }
  const authResult = inspectBootstrapCurrentSelf(makePage({ session: sessionWithAuthGetters }));
  assert.deepEqual(authResult.authFields, {
    loggedIn: 'accessor',
    isAuthenticated: 'accessor',
    authenticated: 'accessor',
    isActive: 'accessor',
  });
  assert.equal(authCounter.count, 0);
});

test('does not invoke accessors in either user map, the direct ID entry, or entity ID fields', () => {
  const userId = '54321';
  const pathCases = [
    { target: 'normalizedUsers', path: ['entities'] },
    { target: 'normalizedUsers', path: ['entities', 'users'] },
    { target: 'normalizedUsers', path: ['entities', 'users', 'entities'] },
    { target: 'rootUsers', path: ['users'] },
    { target: 'rootUsers', path: ['users', 'entities'] },
  ];

  for (const item of pathCases) {
    const counter = { count: 0 };
    const state = { session: { user_id_str: userId } };
    let parent = state;
    for (let index = 0; index < item.path.length - 1; index += 1) {
      const child = {};
      parent[item.path[index]] = child;
      parent = child;
    }
    getter(parent, item.path[item.path.length - 1], counter);
    const dto = inspectBootstrapCurrentSelf(makePage(state));
    assert.equal(dto.entityChecks[item.target], 'accessor');
    assert.equal(counter.count, 0);
  }

  const entryCounter = { count: 0 };
  const map = {};
  getter(map, userId, entryCounter);
  const entryResult = inspectBootstrapCurrentSelf(
    makePage({ session: { user_id_str: userId }, entities: { users: { entities: map } } }),
  );
  assert.equal(entryResult.entityChecks.normalizedUsers, 'accessor');
  assert.equal(entryCounter.count, 0);

  for (const idKey of ['id_str', 'id']) {
    const counter = { count: 0 };
    const entry = { id_str: userId, id: userId };
    getter(entry, idKey, counter);
    const dto = inspectBootstrapCurrentSelf(
      makePage({ session: { user_id_str: userId }, entities: { users: { entities: { [userId]: entry } } } }),
    );
    assert.equal(dto.entityChecks.normalizedUsers, 'accessor');
    assert.equal(counter.count, 0);
  }
});

test('reports thrown and revoked proxy failures without exposing errors or source values', () => {
  const privateMarker = 'PRIVATE-BOOTSTRAP-ERROR-MARKER';
  const throwingState = new Proxy({}, {
    getOwnPropertyDescriptor() {
      throw new Error(privateMarker);
    },
  });
  const fromStateProxy = inspectBootstrapCurrentSelf(makePage(throwingState));
  assert.equal(fromStateProxy.diagnostic, 'inspection_failed');
  assert.equal(JSON.stringify(fromStateProxy).includes(privateMarker), false);

  const { proxy: revoked, revoke } = Proxy.revocable({}, {});
  revoke();
  const fromRevokedProxy = inspectBootstrapCurrentSelf(makePage(revoked));
  assert.equal(fromRevokedProxy.diagnostic, 'inspection_failed');
  assert.equal(JSON.stringify(fromRevokedProxy).includes(privateMarker), false);
  assert.equal(JSON.stringify(fromRevokedProxy).includes('revoked'), false);
});

test('distinguishes missing and invalid bootstrap state', () => {
  const deleted = makePage({ session: {} });
  delete deleted.__INITIAL_STATE__;
  assert.equal(inspectBootstrapCurrentSelf(deleted).diagnostic, 'bootstrap_missing');

  const undefinedState = inspectBootstrapCurrentSelf(makePage(undefined));
  assert.equal(undefinedState.diagnostic, 'bootstrap_invalid');
  assert.equal(undefinedState.bootstrapState, 'invalid');
});

test('rejects unexpected private fields and authority claims with one fixed validator error', () => {
  const extra = baseDto();
  extra.privateToken = 'PRIVATE-DTO-MARKER';
  assert.throws(() => validateBootstrapCurrentSelf(extra), { message: 'bootstrap_projection_invalid' });

  const rawId = baseDto();
  rawId.idFields.user_id_str = '123456';
  assert.throws(() => validateBootstrapCurrentSelf(rawId), { message: 'bootstrap_projection_invalid' });

  const authority = baseDto();
  authority.canAuthorize = true;
  assert.throws(() => validateBootstrapCurrentSelf(authority), { message: 'bootstrap_projection_invalid' });

  const badCount = baseDto();
  badCount.stableIdStringCandidates = 2;
  assert.throws(() => validateBootstrapCurrentSelf(badCount), { message: 'bootstrap_projection_invalid' });

  const badAgreement = baseDto();
  badAgreement.sessionIdsAgree = true;
  assert.throws(() => validateBootstrapCurrentSelf(badAgreement), { message: 'bootstrap_projection_invalid' });
});

test('validator does not invoke input accessors and reconstructs a sanitized DTO', () => {
  const counter = { count: 0 };
  const input = baseDto();
  getter(input, 'diagnostic', counter);

  assert.throws(() => validateBootstrapCurrentSelf(input), { message: 'bootstrap_projection_invalid' });
  assert.equal(counter.count, 0);

  const nullProto = Object.assign(Object.create(null), baseDto());
  nullProto.idFields = Object.assign(Object.create(null), nullProto.idFields);
  nullProto.authFields = Object.assign(Object.create(null), nullProto.authFields);
  nullProto.entityChecks = Object.assign(Object.create(null), nullProto.entityChecks);
  const sanitized = validateBootstrapCurrentSelf(nullProto);
  assert.deepEqual(sanitized, baseDto());
  assert.notEqual(sanitized, nullProto);
});

test('serialized expression runs in a VM page context and returns the same validated DTO', () => {
  const id = '654321';
  const raw = vm.runInNewContext(BOOTSTRAP_CURRENT_SELF_EXPRESSION, {
    location: { ...TARGET_LOCATION },
    __INITIAL_STATE__: {
      session: { user_id_str: id },
      entities: { users: { entities: { [id]: { id_str: id } } } },
    },
  });

  const sanitized = validateBootstrapCurrentSelf(raw);
  assert.equal(sanitized.diagnostic, 'projected');
  assert.equal(sanitized.entityChecks.normalizedUsers, 'match');
  assert.equal(sanitized.canAuthorize, false);
  assert.equal(sanitized.continuityProven, false);
});

test('serialized expression reads native location but preserves the bootstrap accessor', () => {
  const calls = { location: 0, origin: 0, pathname: 0, bootstrap: 0 };
  const liveLocation = {};
  Object.defineProperty(liveLocation, 'origin', {
    get() {
      calls.origin += 1;
      return 'https://x.com';
    },
  });
  Object.defineProperty(liveLocation, 'pathname', {
    get() {
      calls.pathname += 1;
      return '/home';
    },
  });

  const windowLike = {};
  Object.defineProperty(windowLike, 'location', {
    get() {
      calls.location += 1;
      return liveLocation;
    },
  });
  Object.defineProperty(windowLike, '__INITIAL_STATE__', {
    get() {
      calls.bootstrap += 1;
      return { session: { user_id_str: '555' } };
    },
  });

  const dto = vm.runInNewContext(BOOTSTRAP_CURRENT_SELF_EXPRESSION, windowLike);
  assert.equal(dto.diagnostic, 'bootstrap_invalid');
  assert.equal(dto.bootstrapState, 'accessor');
  assert.equal(dto.stableIdStringCandidates, 0);
  assert.deepEqual(calls, { location: 1, origin: 1, pathname: 1, bootstrap: 0 });
  const sanitized = validateBootstrapCurrentSelf(dto);
  assert.equal(sanitized.diagnostic, 'bootstrap_invalid');
  assert.equal(sanitized.bootstrapState, 'accessor');
});

test('serialized expression returns fixed inspection failure when native location access throws', () => {
  const calls = { bootstrap: 0 };
  const windowLike = {};
  Object.defineProperty(windowLike, 'location', {
    get() {
      throw new Error('PRIVATE-LOCATION-ERROR');
    },
  });
  Object.defineProperty(windowLike, '__INITIAL_STATE__', {
    get() {
      calls.bootstrap += 1;
      return { session: { user_id_str: '999' } };
    },
  });

  const dto = vm.runInNewContext(BOOTSTRAP_CURRENT_SELF_EXPRESSION, windowLike);
  assert.equal(dto.diagnostic, 'inspection_failed');
  assert.equal(dto.bootstrapState, 'absent');
  assert.equal(calls.bootstrap, 0);
  assert.equal(JSON.stringify(dto).includes('PRIVATE-LOCATION-ERROR'), false);
});

test('actual V8 inspector accepts populated, missing and accessor projections with side-effect rejection enabled', async () => {
  // This connects only to this fresh Node test worker; no port or other
  // process is opened. VM execution alone does not enforce V8's guard.
  const previousLocation = Object.getOwnPropertyDescriptor(globalThis, 'location');
  const previousBootstrap = Object.getOwnPropertyDescriptor(globalThis, '__INITIAL_STATE__');
  const session = new Session();
  session.connect();
  const evaluate = () => new Promise((accept, reject) => {
    session.post('Runtime.evaluate', { expression: BOOTSTRAP_CURRENT_SELF_EXPRESSION,
      returnByValue: true, awaitPromise: false, userGesture: false, throwOnSideEffect: true, disableBreaks: true, timeout: 1500 },
    (error, result) => {
      if (error) { reject(new Error('inspector_command_failed')); return; }
      if (Object.hasOwn(result, 'exceptionDetails')) { reject(new Error('guarded_projection_rejected')); return; }
      try { accept(validateBootstrapCurrentSelf(result.result?.value)); }
      catch { reject(new Error('inspector_projection_invalid')); }
    });
  });
  try {
    Object.defineProperty(globalThis, 'location', { configurable: true, value: { ...TARGET_LOCATION } });
    const id = '123456';
    Object.defineProperty(globalThis, '__INITIAL_STATE__', { configurable: true, value: {
      session: { user_id: id, user_id_str: id },
      entities: { users: { entities: { [id]: { id_str: id } } } },
      users: { entities: { [id]: { id_str: id } } },
    } });
    const populated = await evaluate();
    assert.equal(populated.diagnostic, 'projected');
    assert.equal(populated.bootstrapState, 'object');
    assert.equal(populated.sessionState, 'object');
    assert.deepEqual(populated.idFields, { user_id: 'string', user_id_str: 'string' });
    assert.equal(populated.stableIdStringCandidates, 2);
    assert.equal(populated.sessionIdsAgree, true);
    assert.deepEqual(populated.entityChecks, { normalizedUsers: 'match', rootUsers: 'match' });
    assert.equal(JSON.stringify(populated).includes(id), false);
    delete globalThis.__INITIAL_STATE__;
    assert.equal((await evaluate()).diagnostic, 'bootstrap_missing');
    let getterCalls = 0;
    Object.defineProperty(globalThis, '__INITIAL_STATE__', { configurable: true, get() { getterCalls += 1; throw new Error('private getter'); } });
    const accessor = await evaluate();
    assert.equal(accessor.diagnostic, 'bootstrap_invalid');
    assert.equal(accessor.bootstrapState, 'accessor');
    assert.equal(getterCalls, 0);
  } finally {
    session.disconnect();
    if (previousLocation === undefined) delete globalThis.location;
    else Object.defineProperty(globalThis, 'location', previousLocation);
    if (previousBootstrap === undefined) delete globalThis.__INITIAL_STATE__;
    else Object.defineProperty(globalThis, '__INITIAL_STATE__', previousBootstrap);
  }
});
