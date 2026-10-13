import { createHash, randomUUID } from 'node:crypto';
import { AuthSessionBroker, normalizeFixtureCookies } from './broker.mjs';
import { CentralSession } from './central.mjs';

const STRATEGY_CAPABILITIES = Object.freeze({
  'reader-owned': Object.freeze({
    id: 'reader-owned',
    refreshOwner: 'reader',
    readerIsAuthority: true,
    readerIndependent: false,
    revokeClearsReader: false,
  }),
  'central-vault': Object.freeze({
    id: 'central-vault',
    refreshOwner: 'collector',
    readerIsAuthority: false,
    readerIndependent: true,
    revokeClearsReader: true,
  }),
});

const CLIENTS = Object.freeze(['reader', 'collector']);
const CLIENT_METHODS = Object.freeze(['replaceCookies', 'exportCookies', 'probe', 'clear', 'refresh']);
const STRATEGY_IDS = new Set(['reader-owned', 'central-vault']);

function isStrategyId(value) { return typeof value === 'string' && STRATEGY_IDS.has(value); }

function failure(code) { return Object.freeze({ ok: false, code }); }
function success(code = 'READY') { return Object.freeze({ ok: true, code }); }

function validAccount(value) {
  return typeof value === 'string' && value.length > 0 && value.length <= 512 && value.trim() === value;
}

function fixtureOrigin(value) {
  if (typeof value !== 'string') throw new Error('FIXTURE_ORIGIN_NOT_ALLOWED');
  let url;
  try { url = new URL(value); } catch { throw new Error('FIXTURE_ORIGIN_NOT_ALLOWED'); }
  if (url.protocol !== 'http:' || url.hostname !== '127.0.0.1' || !url.port ||
      url.username || url.password || url.pathname !== '/' || url.search || url.hash ||
      value !== url.origin) throw new Error('FIXTURE_ORIGIN_NOT_ALLOWED');
  return url.origin;
}

function validateClients(clients) {
  if (!clients || typeof clients !== 'object' || Array.isArray(clients) ||
      Object.keys(clients).length !== CLIENTS.length || CLIENTS.some(id => !clients[id]) ||
      Object.keys(clients).some(id => !CLIENTS.includes(id))) {
    throw new Error('CLIENT_ADAPTER_INVALID');
  }
  for (const id of CLIENTS) {
    if (!CLIENT_METHODS.every(method => typeof clients[id][method] === 'function')) {
      throw new Error('CLIENT_ADAPTER_INVALID');
    }
  }
}

function identity(probe) {
  if (!probe || probe.authenticated !== true || !probe.identity || !validAccount(probe.identity.accountId)) return null;
  return probe.identity.accountId;
}

function normalizedCookies(value, origin) {
  const snapshot = Array.isArray(value) ? { cookies: value } : value;
  return normalizeFixtureCookies(snapshot, origin, Date.now());
}

function cookieFingerprint(cookies) {
  const ordered = [...cookies].sort((a, b) => JSON.stringify(a).localeCompare(JSON.stringify(b)));
  return createHash('sha256').update(JSON.stringify(ordered)).digest('hex');
}

function safeProjection(engine, strategyId) {
  const raw = engine?.status?.();
  if (!raw) return null;
  if (strategyId === 'reader-owned') {
    return Object.freeze({
      state: raw.state,
      generation: raw.generation,
      authEpoch: raw.authEpoch,
      code: raw.code,
      hasContext: raw.hasContext,
      undisposedContextCount: raw.undisposedContextCount,
    });
  }
  const clients = {};
  for (const [id, item] of Object.entries(raw.clients ?? {})) {
    clients[id] = Object.freeze({ attached: item.attached, ready: item.ready, inFlight: item.inFlight });
  }
  return Object.freeze({
    state: raw.state,
    version: raw.version,
    epoch: raw.epoch,
    code: raw.code,
    activeUses: raw.activeUses,
    clients: Object.freeze(clients),
  });
}

/**
 * Switches between two local fixture-only session strategies. The controllers
 * are wrappers over AuthSessionBroker and CentralSession; they do not load
 * user-supplied strategy code or accept production endpoints.
 */
export class SessionStrategyController {
  #id = randomUUID();
  #origin;
  #expectedAccountId;
  #clients;
  #timeoutMs;
  #active = null;
  #target = null;
  #strategyToFence = null;
  #pendingCleanup = null;
  #pendingSnapshot = null;
  #state = 'IDLE';
  #generation = 0;
  #operationEpoch = 0;
  #lastCode = null;
  #activeUses = 0;
  #activeRefreshes = 0;
  #drainWaiters = new Set();
  #switchPromise = null;
  #refreshPromise = null;
  #revocationPromise = null;
  #engineRevocations = new WeakMap();
  #sourceFingerprint = null;
  #sourceRestorePromise = null;
  #sourceClearPending = false;

  constructor({ origin, expectedAccountId, clients, timeoutMs = 5_000 } = {}) {
    this.#origin = fixtureOrigin(origin);
    if (!validAccount(expectedAccountId)) throw new Error('ACCOUNT_MISMATCH');
    validateClients(clients);
    if (!Number.isInteger(timeoutMs) || timeoutMs < 1 || timeoutMs > 120_000) {
      throw new Error('TIMEOUT_INVALID');
    }
    this.#expectedAccountId = expectedAccountId;
    this.#clients = Object.freeze({ reader: clients.reader, collector: clients.collector });
    this.#timeoutMs = timeoutMs;
  }

  static listStrategies() { return Object.freeze(Object.values(STRATEGY_CAPABILITIES)); }

  listStrategies() { return SessionStrategyController.listStrategies(); }

  status() {
    const handle = this.#active ?? this.#target ?? this.#pendingCleanup;
    return Object.freeze({
      state: this.#state,
      ready: this.#state === 'READY' && this.#active !== null,
      activeStrategy: this.#active?.id ?? null,
      pendingStrategy: this.#target?.id ?? null,
      generation: this.#generation,
      inFlight: this.#activeUses + this.#activeRefreshes,
      code: this.#lastCode,
      cleanupPending: this.#pendingCleanup !== null,
      capabilities: handle ? STRATEGY_CAPABILITIES[handle.id] : null,
      engine: handle ? safeProjection(handle.engine, handle.id) : null,
    });
  }

  lease(clientId) {
    if (this.#state !== 'READY' || !this.#active || !CLIENTS.includes(clientId)) return null;
    const engineLease = this.#active.engine.lease(
      this.#active.id === 'reader-owned' ? undefined : clientId,
    );
    if (!engineLease) return null;
    return Object.freeze({
      controllerId: this.#id,
      generation: this.#generation,
      strategyId: this.#active.id,
      clientId,
      engineLease,
    });
  }

  start(strategyId) {
    if (!isStrategyId(strategyId)) return Promise.resolve(failure('STRATEGY_UNKNOWN'));
    if (this.#state === 'REVOKED') return Promise.resolve(failure('SESSION_REVOKED'));
    if (this.#switchPromise || this.#refreshPromise) return Promise.resolve(failure('BUSY'));
    if (this.#active) return Promise.resolve(failure('ALREADY_STARTED'));
    return this.#beginSwitch(strategyId, true);
  }

  switchTo(strategyId) {
    if (!isStrategyId(strategyId)) return Promise.resolve(failure('STRATEGY_UNKNOWN'));
    if (this.#state === 'REVOKED') return Promise.resolve(failure('SESSION_REVOKED'));
    if (this.#state === 'CONFLICT') return Promise.resolve(failure('SESSION_CONFLICT'));
    if (this.#switchPromise || this.#refreshPromise) return Promise.resolve(failure('BUSY'));
    if (this.#active?.id === strategyId && this.#state === 'READY') {
      return Promise.resolve(Object.freeze({ ok: true, code: 'UNCHANGED' }));
    }
    if (!this.#active && this.#state === 'IDLE') return this.#beginSwitch(strategyId, true);
    if (!this.#active && !this.#pendingCleanup) return Promise.resolve(failure('SESSION_NOT_READY'));
    return this.#beginSwitch(strategyId, false);
  }

  // Observation comes only from the explicitly attached fixture browser adapter.
  // Fence even during refresh/handoff: a background response is not refresh authority.
  notifyExternalSessionChange(clientId) {
    if (!CLIENTS.includes(clientId)) return failure('CLIENT_UNKNOWN');
    if (['IDLE', 'REVOKED', 'CONFLICT'].includes(this.#state)) return failure('SESSION_NOT_READY');
    const handles = new Set([this.#active, this.#target, this.#strategyToFence].filter(Boolean));
    for (const handle of handles) {
      if (handle.id === 'central-vault') handle.engine.notifyUnexpectedRotation(clientId);
    }
    this.#operationEpoch += 1;
    this.#generation += 1;
    this.#state = 'CONFLICT';
    this.#lastCode = 'EXTERNAL_SESSION_CHANGE';
    return failure('EXTERNAL_SESSION_CHANGE');
  }

  #beginSwitch(strategyId, starting) {
    this.#state = 'SWITCHING';
    this.#lastCode = null;
    this.#generation += 1;
    const token = ++this.#operationEpoch;
    const promise = this.#switch(strategyId, starting, token);
    this.#switchPromise = promise;
    promise.finally(() => {
      if (this.#switchPromise === promise) this.#switchPromise = null;
    }).catch(() => undefined);
    return promise;
  }

  async #switch(strategyId, starting, token) {
    let snapshot = this.#pendingSnapshot;
    try {
      if (!(await this.#waitForDrain())) return this.#transitionFailure(token, 'DRAIN_TIMEOUT');
      if (!this.#current(token)) return failure('AUTH_EPOCH_CHANGED');

      if (this.#active && this.#state === 'SWITCHING') {
        const engineStatus = this.#active.engine.status();
        const readyState = this.#active.id === 'reader-owned' ? 'VERIFIED' : 'READY';
        if (engineStatus.state !== readyState || !snapshot && this.#pendingCleanup) {
          return this.#transitionFailure(token, 'SOURCE_STRATEGY_NOT_READY');
        }
        if (!snapshot) {
          const source = await this.#readSource(token);
          if (!source.ok) return this.#transitionFailure(token, source.code);
          snapshot = source.snapshot;
        }
      } else if (!snapshot) {
        const source = await this.#readSource(token);
        if (!source.ok) return this.#transitionFailure(token, source.code);
        snapshot = source.snapshot;
      }
      if (!this.#current(token)) return failure('AUTH_EPOCH_CHANGED');

      if (this.#pendingCleanup) {
        const cleanup = await this.#revokeEngine(this.#pendingCleanup);
        if (!this.#current(token)) return failure('AUTH_EPOCH_CHANGED');
        if (!cleanup.ok) {
          this.#pendingSnapshot = snapshot;
          return this.#transitionFailure(token, cleanup.code);
        }
        const wasCentral = this.#pendingCleanup.id === 'central-vault';
        this.#pendingCleanup = null;
        this.#pendingSnapshot = null;
        if (wasCentral) {
          const restored = await this.#restoreReader(snapshot, token);
          if (!restored.ok) return this.#transitionFailure(token, restored.code);
        }
      }

      if (this.#active) {
        const previous = this.#active;
        this.#strategyToFence = previous;
        const cleanup = await this.#revokeEngine(previous);
        if (!this.#current(token)) return failure('AUTH_EPOCH_CHANGED');
        if (!cleanup.ok) {
          this.#active = null;
          this.#pendingCleanup = previous;
          this.#pendingSnapshot = snapshot;
          return this.#transitionFailure(token, cleanup.code);
        }
        this.#active = null;
        this.#strategyToFence = null;
        if (previous.id === 'central-vault') {
          const restored = await this.#restoreReader(snapshot, token);
          if (!restored.ok) return this.#transitionFailure(token, restored.code);
        }
      }
      if (!this.#current(token)) return failure('AUTH_EPOCH_CHANGED');

      const target = this.#makeEngine(strategyId);
      this.#target = target;
      this.#strategyToFence = target;
      const result = strategyId === 'reader-owned'
        ? await target.engine.bootstrap({ origin: this.#origin, expectedAccountId: this.#expectedAccountId })
        : await target.engine.initialize({ cookies: snapshot.cookies, accountId: snapshot.accountId });
      if (!this.#current(token)) return failure('AUTH_EPOCH_CHANGED');
      if (!result?.ok) {
        const code = result?.code ?? 'STRATEGY_START_FAILED';
        const cleanup = await this.#revokeEngine(target);
        if (!this.#current(token)) return failure('AUTH_EPOCH_CHANGED');
        this.#target = null;
        this.#strategyToFence = null;
        if (!cleanup.ok) this.#pendingCleanup = target;
        this.#pendingSnapshot = cleanup.ok ? null : snapshot;
        return this.#transitionFailure(token, code);
      }

      const finalSource = await this.#readSource(token);
      if (!finalSource.ok || finalSource.snapshot.fingerprint !== snapshot.fingerprint) {
        const code = finalSource.ok ? 'SOURCE_SESSION_CHANGED' : finalSource.code;
        const cleanup = await this.#revokeEngine(target);
        if (!this.#current(token)) return failure('AUTH_EPOCH_CHANGED');
        this.#target = null;
        this.#strategyToFence = null;
        if (!cleanup.ok) this.#pendingCleanup = target;
        this.#pendingSnapshot = cleanup.ok ? null : snapshot;
        return this.#transitionFailure(token, code);
      }

      this.#sourceFingerprint = snapshot.fingerprint;
      this.#active = target;
      this.#target = null;
      this.#strategyToFence = null;
      this.#pendingSnapshot = null;
      this.#pendingCleanup = null;
      this.#state = 'READY';
      this.#lastCode = null;
      return success('READY');
    } catch {
      if (!this.#current(token)) return failure('AUTH_EPOCH_CHANGED');
      return this.#transitionFailure(token, 'STRATEGY_START_FAILED');
    }
  }

  #transitionFailure(token, code) {
    if (this.#current(token)) {
      this.#state = 'NEEDS_USER';
      this.#lastCode = code;
    }
    return failure(code);
  }

  #current(token) {
    return this.#operationEpoch === token && this.#state !== 'REVOKED';
  }

  async #readSource(token) {
    let first;
    try { first = await this.#clients.reader.probe({ origin: this.#origin }); }
    catch { return { ok: false, code: 'SOURCE_PROBE_FAILED' }; }
    if (!this.#current(token)) return { ok: false, code: 'AUTH_EPOCH_CHANGED' };
    if (identity(first) !== this.#expectedAccountId) {
      return { ok: false, code: first?.authenticated ? 'ACCOUNT_MISMATCH' : 'SOURCE_AUTH_REQUIRED' };
    }
    let cookies;
    try {
      const raw = await this.#clients.reader.exportCookies({
        origin: this.#origin, accountId: this.#expectedAccountId, generation: this.#generation,
      });
      cookies = normalizedCookies(raw, this.#origin);
    } catch {
      return { ok: false, code: 'COOKIE_EXPORT_FAILED' };
    }
    if (!this.#current(token)) return { ok: false, code: 'AUTH_EPOCH_CHANGED' };
    let finalProbe;
    try { finalProbe = await this.#clients.reader.probe({ origin: this.#origin }); }
    catch { return { ok: false, code: 'SOURCE_PROBE_FAILED' }; }
    if (!this.#current(token)) return { ok: false, code: 'AUTH_EPOCH_CHANGED' };
    if (identity(finalProbe) !== this.#expectedAccountId) return { ok: false, code: 'SOURCE_SESSION_CHANGED' };
    const snapshot = Object.freeze({
      accountId: this.#expectedAccountId,
      cookies: Object.freeze([...cookies]),
      fingerprint: cookieFingerprint(cookies),
    });
    return { ok: true, snapshot };
  }

  async #restoreReader(snapshot, token) {
    if (!this.#current(token)) return failure('AUTH_EPOCH_CHANGED');
    let mutation;
    try {
      mutation = Promise.resolve(this.#clients.reader.replaceCookies({
        origin: this.#origin, cookies: snapshot.cookies, accountId: snapshot.accountId,
      }));
    } catch { return failure('SOURCE_RESTORE_FAILED'); }
    this.#sourceRestorePromise = mutation;
    try { await mutation; }
    catch { this.#sourceClearPending = true; return failure('SOURCE_RESTORE_FAILED'); }
    finally {
      if (this.#sourceRestorePromise === mutation) this.#sourceRestorePromise = null;
    }
    if (!this.#current(token)) return failure('AUTH_EPOCH_CHANGED');
    const restored = await this.#readSource(token);
    if (!restored.ok) return failure(restored.code);
    if (restored.snapshot.fingerprint !== snapshot.fingerprint) {
      this.#sourceClearPending = true;
      return failure('SOURCE_RESTORE_MISMATCH');
    }
    this.#sourceClearPending = false;
    return success();
  }

  #makeEngine(strategyId) {
    if (strategyId === 'central-vault') {
      const engine = new CentralSession({
        origin: this.#origin,
        expectedAccountId: this.#expectedAccountId,
        authority: this.#clients.collector,
        refreshClientId: 'collector',
        bootstrapClientId: 'reader',
        clients: this.#clients,
        timeoutMs: this.#timeoutMs,
      });
      return { id: strategyId, engine };
    }

    const contexts = new Map();
    const owner = {
      probe: args => this.#clients.reader.probe(args),
      exportCookies: args => this.#clients.reader.exportCookies(args),
    };
    const derived = {
      create: async ({ origin, generation }) => {
        const context = { id: randomUUID(), origin, generation };
        contexts.set(context.id, context);
        return context;
      },
      seed: async ({ context, origin, cookies }) => {
        await this.#clients.collector.replaceCookies({ origin, cookies, contextId: context.id });
        context.fingerprint = cookieFingerprint(cookies);
      },
      probe: async ({ context, origin }) => {
        const probe = await this.#clients.collector.probe({ origin, contextId: context.id });
        if (identity(probe) !== this.#expectedAccountId) return probe;
        const raw = await this.#clients.collector.exportCookies({ origin, accountId: this.#expectedAccountId,
          contextId: context.id });
        const cookies = normalizedCookies(raw, origin);
        if (context.fingerprint && cookieFingerprint(cookies) !== context.fingerprint) {
          return { authenticated: false, identity: null };
        }
        return probe;
      },
      dispose: async ({ context }) => {
        const result = await this.#clients.collector.clear({ origin: context.origin, contextId: context.id });
        if (result === false) return false;
        contexts.delete(context.id);
        return true;
      },
    };
    const engine = new AuthSessionBroker({
      fixtureOrigins: [this.#origin], owner, derived,
    });
    return { id: strategyId, engine };
  }

  use(clientId, lease, fn) {
    if (typeof fn !== 'function') return Promise.resolve(failure('USE_CALLBACK_INVALID'));
    if (!CLIENTS.includes(clientId) || !this.#leaseMatches(clientId, lease)) {
      return Promise.resolve(failure('LEASE_STALE'));
    }
    const handle = this.#active;
    const token = this.#operationEpoch;
    const hostGeneration = this.#generation;
    this.#activeUses += 1;
    const finish = () => {
      this.#activeUses -= 1;
      this.#signalDrain();
    };

    const task = handle.id === 'central-vault'
      ? handle.engine.use(clientId, lease.engineLease, context => fn(Object.freeze({
        ...context, strategyId: handle.id, hostGeneration,
      }))).then(result => {
        if (!this.#current(token) || this.#generation !== hostGeneration) return failure('LEASE_STALE');
        if (!result?.ok) {
          const engineState = handle.engine.status().state;
          if (engineState !== 'READY') {
            this.#state = engineState === 'CONFLICT' ? 'CONFLICT' : 'NEEDS_USER';
            this.#lastCode = result?.code ?? 'USE_FAILED';
            this.#generation += 1;
          }
          return failure(result?.code ?? 'USE_FAILED');
        }
        return Object.freeze({ ok: true, value: result.value });
      }).catch(() => failure('USE_FAILED'))
      : this.#useReaderOwned(handle, clientId, lease, fn, token, hostGeneration);
    return Promise.resolve(task).finally(finish);
  }

  #leaseMatches(clientId, lease) {
    return this.#state === 'READY' && !!this.#active && !!lease &&
      lease.controllerId === this.#id && lease.clientId === clientId &&
      lease.strategyId === this.#active.id && lease.generation === this.#generation &&
      this.#active.engine.admitResult(lease.engineLease);
  }

  async #useReaderOwned(handle, clientId, lease, fn, token, hostGeneration) {
    if (!handle.engine.admitResult(lease.engineLease)) return failure('LEASE_STALE');
    const context = Object.freeze({ origin: this.#origin, accountId: this.#expectedAccountId,
      clientId, strategyId: handle.id, hostGeneration, version: lease.engineLease.generation,
      epoch: lease.engineLease.authEpoch });
    let outcome;
    try { outcome = { ok: true, value: await fn(context) }; }
    catch { outcome = { ok: false }; }
    if (!this.#current(token) || this.#generation !== hostGeneration ||
        !handle.engine.admitResult(lease.engineLease)) return failure('LEASE_STALE');
    const adapter = this.#clients[clientId];
    let probe;
    try { probe = await adapter.probe({ origin: this.#origin }); }
    catch { return this.#failReaderUse('CLIENT_AUTH_REQUIRED'); }
    if (!this.#current(token) || this.#generation !== hostGeneration ||
        !handle.engine.admitResult(lease.engineLease)) return failure('LEASE_STALE');
    if (identity(probe) !== this.#expectedAccountId) {
      return this.#failReaderUse(probe?.authenticated ? 'CLIENT_IDENTITY_MISMATCH' : 'CLIENT_AUTH_REQUIRED');
    }
    let observed;
    try {
      observed = normalizedCookies(await adapter.exportCookies({
        origin: this.#origin, accountId: this.#expectedAccountId,
      }), this.#origin);
    } catch { return this.#failReaderUse('COOKIE_OBSERVATION_FAILED'); }
    if (!this.#current(token) || this.#generation !== hostGeneration ||
        !handle.engine.admitResult(lease.engineLease)) return failure('LEASE_STALE');
    if (cookieFingerprint(observed) !== this.#sourceFingerprint) {
      return this.#failReaderUse('UNEXPECTED_ROTATION', 'CONFLICT');
    }
    return outcome.ok ? Object.freeze({ ok: true, value: outcome.value }) : failure('USE_FAILED');
  }

  #failReaderUse(code, state = 'NEEDS_USER') {
    this.#state = state;
    this.#lastCode = code;
    this.#generation += 1;
    return failure(code);
  }

  refresh() {
    if (this.#state !== 'READY' || !this.#active) return Promise.resolve(failure('SESSION_NOT_READY'));
    if (this.#switchPromise || this.#refreshPromise) return Promise.resolve(failure('BUSY'));
    this.#state = 'REFRESHING';
    this.#lastCode = null;
    this.#generation += 1;
    const token = ++this.#operationEpoch;
    this.#activeRefreshes += 1;
    const handle = this.#active;
    const promise = this.#refresh(handle, token).finally(() => {
      this.#activeRefreshes -= 1;
      this.#signalDrain();
      if (this.#refreshPromise === promise) this.#refreshPromise = null;
    });
    this.#refreshPromise = promise;
    return promise;
  }

  async #refresh(handle, token) {
    if (!(await this.#waitForDrain())) return this.#transitionFailure(token, 'DRAIN_TIMEOUT');
    if (!this.#current(token)) return failure('AUTH_EPOCH_CHANGED');
    let result;
    if (handle.id === 'central-vault') {
      try { result = await handle.engine.refresh(); }
      catch { result = failure('REFRESH_FAILED'); }
    } else {
      try {
        await this.#clients.reader.refresh({ origin: this.#origin, accountId: this.#expectedAccountId });
      } catch { return this.#transitionFailure(token, 'AUTHORITY_REFRESH_FAILED'); }
      if (!this.#current(token)) return failure('AUTH_EPOCH_CHANGED');
      result = await handle.engine.bootstrap({ origin: this.#origin, expectedAccountId: this.#expectedAccountId });
    }
    if (!this.#current(token)) return failure('AUTH_EPOCH_CHANGED');
    if (!result?.ok) return this.#transitionFailure(token, result?.code ?? 'REFRESH_FAILED');
    const readerAttached = handle.id !== 'central-vault' || handle.engine.status().clients?.reader?.attached !== false;
    if (readerAttached) {
      const source = await this.#readSource(token);
      if (!source.ok) return this.#transitionFailure(token, source.code);
      this.#sourceFingerprint = source.snapshot.fingerprint;
    }
    this.#state = 'READY';
    this.#lastCode = null;
    return success('READY');
  }

  detach(clientId = 'reader') {
    return this.#clientMutation('detach', clientId);
  }

  attach(clientId = 'reader') {
    return this.#clientMutation('attach', clientId);
  }

  #clientMutation(action, clientId) {
    if (!CLIENTS.includes(clientId)) return Promise.resolve(failure('CLIENT_UNKNOWN'));
    const recoveringReaderOwnedCollector = action === 'attach' && clientId === 'collector' &&
      this.#active?.id === 'reader-owned' && this.#state === 'NEEDS_USER' && this.#lastCode === 'DETACHED';
    if ((this.#state !== 'READY' && !recoveringReaderOwnedCollector) || !this.#active) {
      return Promise.resolve(failure('SESSION_NOT_READY'));
    }
    if (this.#switchPromise || this.#refreshPromise) return Promise.resolve(failure('BUSY'));
    const handle = this.#active;
    if (handle.id === 'reader-owned' && clientId === 'reader') {
      return Promise.resolve(failure('SOURCE_CLIENT_REQUIRED'));
    }
    this.#state = 'SWITCHING';
    this.#generation += 1;
    const token = ++this.#operationEpoch;
    const promise = this.#runClientMutation(action, clientId, handle, token);
    this.#switchPromise = promise;
    promise.finally(() => {
      if (this.#switchPromise === promise) this.#switchPromise = null;
    }).catch(() => undefined);
    return promise;
  }

  async #runClientMutation(action, clientId, handle, token) {
    if (!(await this.#waitForDrain())) return this.#transitionFailure(token, 'DRAIN_TIMEOUT');
    if (!this.#current(token)) return failure('AUTH_EPOCH_CHANGED');
    let result;
    if (handle.id === 'central-vault') {
      result = await handle.engine[action](clientId);
    } else if (action === 'detach') {
      result = await handle.engine.revoke();
    } else {
      result = await handle.engine.bootstrap({ origin: this.#origin, expectedAccountId: this.#expectedAccountId });
    }
    if (!this.#current(token)) return failure('AUTH_EPOCH_CHANGED');
    if (!result?.ok) return this.#transitionFailure(token, result?.code ?? `${action.toUpperCase()}_FAILED`);
    if (handle.id === 'reader-owned' && action === 'detach') {
      this.#state = 'NEEDS_USER';
      this.#lastCode = 'DETACHED';
      return success('DETACHED');
    }
    this.#state = 'READY';
    this.#lastCode = null;
    return success(action === 'attach' ? 'ATTACHED' : 'DETACHED');
  }

  revoke() {
    if (this.#revocationPromise) return this.#revocationPromise;
    if (this.#state !== 'REVOKED') {
      this.#state = 'REVOKED';
      this.#lastCode = 'REVOKED';
      this.#generation += 1;
      this.#operationEpoch += 1;
    }
    const handle = this.#strategyToFence ?? this.#target ?? this.#active ?? this.#pendingCleanup;
    this.#active = null;
    this.#target = null;
    this.#pendingSnapshot = null;
    if (!handle && (this.#sourceRestorePromise || this.#sourceClearPending)) {
      const restore = this.#sourceRestorePromise ?? Promise.resolve();
      const cleanupPromise = restore.catch(() => undefined).then(async () => {
        try {
          await this.#clients.reader.clear({ origin: this.#origin });
          this.#sourceClearPending = false;
          return success('REVOKED');
        } catch { this.#sourceClearPending = true; return failure('CLEAR_FAILED'); }
      });
      const promise = cleanupPromise.then(result => {
        if (!result.ok) {
          this.#lastCode = result.code;
          return result;
        }
        this.#lastCode = 'REVOKED';
        return result;
      }).finally(() => {
        if (this.#revocationPromise === promise) this.#revocationPromise = null;
      });
      this.#revocationPromise = promise;
      return promise;
    }
    if (!handle) {
      this.#pendingCleanup = null;
      return Promise.resolve(success('REVOKED'));
    }
    const cleanupPromise = this.#revokeEngine(handle);
    const promise = cleanupPromise.then(result => {
      if (result.ok) {
        if (this.#pendingCleanup === handle) this.#pendingCleanup = null;
        this.#strategyToFence = null;
        return success('REVOKED');
      }
      this.#pendingCleanup = handle;
      this.#strategyToFence = handle;
      this.#lastCode = result.code;
      return failure(result.code);
    }).finally(() => {
      if (this.#revocationPromise === promise) this.#revocationPromise = null;
    });
    this.#revocationPromise = promise;
    return promise;
  }

  #revokeEngine(handle) {
    const existing = this.#engineRevocations.get(handle);
    if (existing) return existing;
    let operation;
    try { operation = Promise.resolve(handle.engine.revoke()); }
    catch { operation = Promise.resolve(failure('CLEANUP_FAILED')); }
    const promise = operation.then(result => {
      if (result?.ok) return success('REVOKED');
      const code = typeof result?.code === 'string' ? result.code : 'CLEANUP_FAILED';
      this.#engineRevocations.delete(handle);
      return failure(code);
    }, () => {
      this.#engineRevocations.delete(handle);
      return failure('CLEANUP_FAILED');
    });
    this.#engineRevocations.set(handle, promise);
    return promise;
  }

  #waitForDrain() {
    if (this.#activeUses === 0) return Promise.resolve(true);
    return new Promise(resolve => {
      let settled = false;
      const finish = value => {
        if (settled) return;
        settled = true;
        clearTimeout(timer);
        this.#drainWaiters.delete(onDrain);
        resolve(value);
      };
      const onDrain = () => {
        if (this.#activeUses === 0) finish(true);
      };
      const timer = setTimeout(() => finish(false), this.#timeoutMs);
      this.#drainWaiters.add(onDrain);
    });
  }

  #signalDrain() {
    for (const waiter of [...this.#drainWaiters]) waiter();
  }
}

export const SessionStrategies = STRATEGY_CAPABILITIES;
