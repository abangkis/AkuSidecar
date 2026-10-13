import { createHash, randomUUID } from 'node:crypto';
import { normalizeFixtureCookies } from './broker.mjs';

const STATES = Object.freeze({
  NEW: 'NEW', INITIALIZING: 'INITIALIZING', READY: 'READY',
  REFRESHING: 'REFRESHING', NEEDS_USER: 'NEEDS_USER', CONFLICT: 'CONFLICT',
  REVOKED: 'REVOKED',
});
const ACCOUNT_ID_MAX = 512;
const CLIENT_ID = /^[a-z][a-z0-9_-]{0,63}$/;
const SAFE_CODES = new Set([
  'COOKIE_EXPIRED', 'COOKIE_SNAPSHOT_INVALID', 'COOKIE_OUTSIDE_FIXTURE',
  'PARTITION_KEY_OUTSIDE_FIXTURE', 'FIXTURE_ORIGIN_NOT_ALLOWED',
]);

function failure(code) { return Object.freeze({ ok: false, code }); }
function safeAccount(value) {
  return typeof value === 'string' && value.length > 0 && value.length <= ACCOUNT_ID_MAX &&
    value.trim() === value;
}
function fixtureOrigin(value) {
  if (typeof value !== 'string') throw new Error('FIXTURE_ORIGIN_NOT_ALLOWED');
  let url;
  try { url = new URL(value); } catch { throw new Error('FIXTURE_ORIGIN_NOT_ALLOWED'); }
  if (url.protocol !== 'http:' || url.hostname !== '127.0.0.1' || url.username || url.password ||
      url.pathname !== '/' || url.search || url.hash || value !== url.origin) {
    throw new Error('FIXTURE_ORIGIN_NOT_ALLOWED');
  }
  return url.origin;
}
function cookieList(value, origin, now) {
  return normalizeFixtureCookies(Array.isArray(value) ? { cookies: value } : value, origin, now);
}
function cookieFingerprint(cookies) {
  const ordered = [...cookies].sort((a, b) =>
    `${a.domain}\0${a.path}\0${a.name}\0${JSON.stringify(a.partitionKey ?? null)}\0${a.value}`
      .localeCompare(`${b.domain}\0${b.path}\0${b.name}\0${JSON.stringify(b.partitionKey ?? null)}\0${b.value}`));
  return createHash('sha256').update(JSON.stringify(ordered)).digest('hex');
}
function validAdapter(value, methods) {
  return !!value && methods.every(method => typeof value[method] === 'function');
}

/** In-memory, fixture-only coordinator for one synthetic browser session. */
export class CentralSession {
  #id = randomUUID();
  #origin;
  #expectedAccountId;
  #authority;
  #bootstrapClientId;
  #refreshClientId;
  #clients = new Map();
  #now;
  #timeoutMs;
  #state = STATES.NEW;
  #version = 0;
  #epoch = 0;
  #accountId = null;
  #vault = null;
  #fingerprint = null;
  #activeUses = 0;
  #clientUses = new Map();
  #drainWaiters = new Set();
  #writerTail = Promise.resolve();
  #pendingRefresh = null;
  #revokePromise = null;
  #revocationComplete = false;
  #pendingEffects = new Set();
  #effectWaiters = new Set();
  #code = null;

  constructor({ origin, expectedAccountId, authority, clients, refreshClientId,
    bootstrapClientId = refreshClientId, timeoutMs = 5_000, now = () => Date.now() } = {}) {
    this.#origin = fixtureOrigin(origin);
    if (!safeAccount(expectedAccountId)) throw new Error('ACCOUNT_MISMATCH');
    if (!clients || typeof clients !== 'object' || Array.isArray(clients) ||
        Object.keys(clients).length === 0) throw new Error('CLIENT_ADAPTER_INVALID');
    for (const [id, adapter] of Object.entries(clients)) {
      if (!CLIENT_ID.test(id) || !validAdapter(adapter,
        ['replaceCookies', 'exportCookies', 'probe', 'clear'])) {
        throw new Error('CLIENT_ADAPTER_INVALID');
      }
      this.#clients.set(id, { id, adapter, attached: true, ready: false, instance: 0 });
      this.#clientUses.set(id, 0);
    }
    if (!this.#clients.has(refreshClientId)) throw new Error('REFRESH_CLIENT_INVALID');
    if (!this.#clients.has(bootstrapClientId)) throw new Error('BOOTSTRAP_CLIENT_INVALID');
    if (!validAdapter(authority, ['probe', 'exportCookies', 'refresh', 'clear'])) {
      throw new Error('AUTHORITY_ADAPTER_INVALID');
    }
    if (!Number.isInteger(timeoutMs) || timeoutMs < 1 || timeoutMs > 120_000) {
      throw new Error('TIMEOUT_INVALID');
    }
    if (typeof now !== 'function') throw new Error('CLOCK_INVALID');
    this.#expectedAccountId = expectedAccountId;
    this.#authority = authority;
    this.#bootstrapClientId = bootstrapClientId;
    this.#refreshClientId = refreshClientId;
    this.#timeoutMs = timeoutMs;
    this.#now = now;
  }

  status() {
    const clients = {};
    for (const [id, client] of this.#clients) {
      clients[id] = Object.freeze({
        attached: client.attached,
        ready: client.ready && this.#state === STATES.READY,
        inFlight: this.#clientUses.get(id) ?? 0,
      });
    }
    return Object.freeze({
      state: this.#state, version: this.#version, epoch: this.#epoch,
      accountId: this.#accountId, code: this.#code,
      activeUses: this.#activeUses, clients: Object.freeze(clients),
    });
  }

  lease(clientId) {
    const client = this.#clients.get(clientId);
    if (!client || !this.#isReady(client)) return null;
    return Object.freeze({
      sessionId: this.#id, origin: this.#origin, accountId: this.#accountId,
      clientId, clientInstance: client.instance, version: this.#version, epoch: this.#epoch,
    });
  }

  admitResult(lease) {
    const client = lease && this.#clients.get(lease.clientId);
    return !!client && this.#leaseMatches(lease, client) && this.#isReady(client);
  }

  initialize({ cookies, accountId } = {}) {
    if (this.#state !== STATES.NEW) return Promise.resolve(failure('ALREADY_INITIALIZED'));
    this.#state = STATES.INITIALIZING;
    this.#code = null;
    const token = this.#token();
    return this.#enqueue(async () => {
      try {
        const bootstrap = this.#clients.get(this.#bootstrapClientId);
        const probe = await this.#call(bootstrap.adapter.probe({ origin: this.#origin }), 'BOOTSTRAP_PROBE_FAILED');
        if (!this.#tokenCurrent(token)) return failure('AUTH_EPOCH_CHANGED');
        if (!this.#identityMatches(probe) || (accountId !== undefined && accountId !== this.#expectedAccountId)) {
          return this.#needsUser(token, 'ACCOUNT_MISMATCH');
        }
        const raw = cookies === undefined
          ? await this.#call(bootstrap.adapter.exportCookies({ origin: this.#origin,
            accountId: this.#expectedAccountId, version: 0, epoch: token.epoch }), 'COOKIE_EXPORT_FAILED')
          : cookies;
        const normalized = this.#normalize(raw);
        const sourceCookies = this.#normalize(await this.#call(
          bootstrap.adapter.exportCookies({ origin: this.#origin, accountId: this.#expectedAccountId,
            version: 0, epoch: token.epoch }), 'COOKIE_EXPORT_FAILED'));
        if (cookieFingerprint(normalized) !== cookieFingerprint(sourceCookies)) {
          return this.#needsUser(token, 'COOKIE_SNAPSHOT_MISMATCH');
        }
        this.#vault = normalized;
        this.#fingerprint = cookieFingerprint(normalized);
        this.#accountId = this.#expectedAccountId;
        this.#version = 1;
        const publishToken = this.#token();
        const fanout = await this.#fanout(publishToken, normalized, { skipReplaceClientId: null });
        if (!fanout.ok) return this.#needsUser(publishToken, fanout.code);
        const finalProbe = await this.#call(this.#authority.probe({ origin: this.#origin }), 'AUTHORITY_PROBE_FAILED');
        if (!this.#tokenCurrent(publishToken) || !this.#identityMatches(finalProbe)) {
          return this.#needsUser(publishToken, 'AUTHORITY_SESSION_CHANGED');
        }
        this.#state = STATES.READY;
        return Object.freeze({ ok: true, code: 'READY', version: this.#version, epoch: this.#epoch });
      } catch (error) {
        return this.#needsUser(token, this.#safeCode(error, 'INITIALIZE_FAILED'));
      }
    });
  }

  use(clientId, lease, fn) {
    const client = this.#clients.get(clientId);
    if (typeof fn !== 'function') return Promise.resolve(failure('USE_CALLBACK_INVALID'));
    if (!client || !this.#leaseMatches(lease, client) || !this.#isReady(client)) {
      return Promise.resolve(failure('LEASE_STALE'));
    }
    const version = this.#version;
    const epoch = this.#epoch;
    const context = Object.freeze({ origin: this.#origin, accountId: this.#accountId,
      clientId, version, epoch });
    this.#activeUses += 1;
    this.#clientUses.set(clientId, (this.#clientUses.get(clientId) ?? 0) + 1);
    const task = Promise.resolve().then(() => fn(context)).then(
      value => ({ ok: true, value }), () => ({ ok: false }),
    ).then(async outcome => {
      if (!this.#sameAuthSnapshot(version, epoch, client, lease.clientInstance)) return failure('LEASE_STALE');
      let probe;
      try {
        probe = await this.#call(client.adapter.probe({ origin: this.#origin }), 'CLIENT_PROBE_FAILED');
      } catch {
        if (this.#sameAuthSnapshot(version, epoch, client, lease.clientInstance)) {
          this.#failClosed('CLIENT_AUTH_REQUIRED');
        }
        return failure('CLIENT_AUTH_REQUIRED');
      }
      if (!this.#sameAuthSnapshot(version, epoch, client, lease.clientInstance)) return failure('LEASE_STALE');
      if (!this.#identityMatches(probe)) {
        const code = probe?.authenticated === true ? 'CLIENT_IDENTITY_MISMATCH' : 'CLIENT_AUTH_REQUIRED';
        this.#failClosed(code);
        return failure(code);
      }
      let observed;
      try {
        observed = this.#normalize(await this.#call(client.adapter.exportCookies({
          origin: this.#origin, accountId: this.#accountId, version, epoch,
        }), 'COOKIE_OBSERVATION_FAILED'));
      } catch {
        if (this.#sameAuthSnapshot(version, epoch, client, lease.clientInstance)) {
          this.#failClosed('COOKIE_OBSERVATION_FAILED');
        }
        return failure('COOKIE_OBSERVATION_FAILED');
      }
      if (!this.#sameAuthSnapshot(version, epoch, client, lease.clientInstance)) return failure('LEASE_STALE');
      if (cookieFingerprint(observed) !== this.#fingerprint) {
        this.#failClosed('UNEXPECTED_ROTATION', STATES.CONFLICT);
        return failure('UNEXPECTED_ROTATION');
      }
      if (!this.#operationCurrent(version, epoch, client, lease.clientInstance)) return failure('LEASE_STALE');
      return outcome.ok ? Object.freeze({ ok: true, value: outcome.value }) : failure('USE_FAILED');
    }).finally(() => {
      this.#activeUses -= 1;
      this.#clientUses.set(clientId, (this.#clientUses.get(clientId) ?? 1) - 1);
      this.#signalDrain();
    });
    return task;
  }

  refresh({ expectedVersion = this.#version, expectedEpoch = this.#epoch } = {}) {
    const key = `${expectedVersion}:${expectedEpoch}`;
    if (this.#pendingRefresh && this.#pendingRefresh.key === key) return this.#pendingRefresh.promise;
    if (this.#state !== STATES.READY || expectedVersion !== this.#version || expectedEpoch !== this.#epoch) {
      return Promise.resolve(failure('STALE_UPDATE'));
    }
    this.#state = STATES.REFRESHING;
    this.#code = null;
    const token = this.#token();
    const promise = this.#enqueue(async () => {
      if (!(await this.#waitForDrain())) return this.#needsUser(token, 'DRAIN_TIMEOUT');
      if (!this.#tokenCurrent(token)) return failure('AUTH_EPOCH_CHANGED');
      try {
        if (!(await this.#waitForPendingEffects())) return this.#needsUser(token, 'PENDING_OPERATION_TIMEOUT');
        await this.#call(this.#authority.refresh({ origin: this.#origin, accountId: this.#accountId,
          version: token.version, epoch: token.epoch }), 'AUTHORITY_REFRESH_FAILED');
        if (!this.#tokenCurrent(token)) return failure('AUTH_EPOCH_CHANGED');
        const authorityProbe = await this.#call(this.#authority.probe({ origin: this.#origin }), 'AUTHORITY_PROBE_FAILED');
        if (!this.#identityMatches(authorityProbe)) return this.#needsUser(token, 'ACCOUNT_MISMATCH');
        const nextCookies = this.#normalize(await this.#call(this.#authority.exportCookies({
          origin: this.#origin, accountId: this.#accountId, version: token.version, epoch: token.epoch,
        }), 'COOKIE_EXPORT_FAILED'));
        if (!this.#tokenCurrent(token)) return failure('AUTH_EPOCH_CHANGED');
        this.#vault = nextCookies;
        this.#fingerprint = cookieFingerprint(nextCookies);
        this.#version = token.version + 1;
        const publishToken = this.#token();
        const fanout = await this.#fanout(publishToken, nextCookies, { skipReplaceClientId: this.#refreshClientId });
        if (!fanout.ok) return this.#needsUser(publishToken, fanout.code);
        const finalProbe = await this.#call(this.#authority.probe({ origin: this.#origin }), 'AUTHORITY_PROBE_FAILED');
        if (!this.#tokenCurrent(publishToken) || !this.#identityMatches(finalProbe)) {
          return this.#needsUser(publishToken, 'AUTHORITY_SESSION_CHANGED');
        }
        this.#state = STATES.READY;
        return Object.freeze({ ok: true, code: 'READY', version: this.#version, epoch: this.#epoch });
      } catch (error) {
        return this.#needsUser(token, this.#safeCode(error, 'REFRESH_FAILED'));
      }
    });
    this.#pendingRefresh = { key, promise };
    const clearPending = () => { if (this.#pendingRefresh?.promise === promise) this.#pendingRefresh = null; };
    promise.then(clearPending, clearPending);
    return promise;
  }

  async attach(clientId) {
    const client = this.#clients.get(clientId);
    if (!client) return failure('CLIENT_UNKNOWN');
    if (this.#state !== STATES.READY || !this.#vault) return failure('SESSION_NOT_READY');
    if (client.attached && client.ready) return Object.freeze({ ok: true, version: this.#version, epoch: this.#epoch });
    client.attached = true;
    client.ready = false;
    client.instance += 1;
    const token = this.#token();
    return this.#enqueue(async () => {
      if (!this.#readyTokenCurrent(token)) return failure('AUTH_EPOCH_CHANGED');
      try {
        if (!(await this.#waitForPendingEffects())) return failure('PENDING_OPERATION_TIMEOUT');
        await this.#call(client.adapter.replaceCookies({ origin: this.#origin, cookies: this.#vault,
          version: this.#version, epoch: this.#epoch }), 'CLIENT_SEED_FAILED');
        const probe = await this.#call(client.adapter.probe({ origin: this.#origin }), 'CLIENT_PROBE_FAILED');
        const observed = this.#normalize(await this.#call(client.adapter.exportCookies({ origin: this.#origin,
          accountId: this.#accountId, version: this.#version, epoch: this.#epoch }), 'COOKIE_OBSERVATION_FAILED'));
        if (!this.#readyTokenCurrent(token) || !this.#identityMatches(probe) ||
            cookieFingerprint(observed) !== this.#fingerprint) {
          client.ready = false;
          return failure('CLIENT_SESSION_MISMATCH');
        }
        client.ready = true;
        return Object.freeze({ ok: true, version: this.#version, epoch: this.#epoch });
      } catch (error) {
        client.ready = false;
        return failure(this.#safeCode(error, 'CLIENT_SEED_FAILED'));
      }
    });
  }

  async detach(clientId) {
    const client = this.#clients.get(clientId);
    if (!client) return failure('CLIENT_UNKNOWN');
    client.attached = false;
    client.ready = false;
    client.instance += 1;
    return this.#enqueue(async () => {
      if (!(await this.#waitForDrain(() => this.#clientUses.get(clientId) ?? 0))) {
        return failure('DRAIN_TIMEOUT');
      }
      try {
        if (!(await this.#waitForPendingEffects())) return failure('PENDING_OPERATION_TIMEOUT');
        await this.#call(client.adapter.clear({ origin: this.#origin }), 'CLIENT_CLEAR_FAILED');
        return Object.freeze({ ok: true });
      } catch (error) { return failure(this.#safeCode(error, 'CLIENT_CLEAR_FAILED')); }
    });
  }

  notifyUnexpectedRotation(clientId, _observation = undefined) {
    const client = this.#clients.get(clientId);
    if (!client?.attached || ![STATES.READY, STATES.REFRESHING, STATES.INITIALIZING].includes(this.#state)) {
      return failure('SESSION_NOT_READY');
    }
    this.#failClosed('UNEXPECTED_ROTATION', STATES.CONFLICT);
    return failure('UNEXPECTED_ROTATION');
  }

  revoke() {
    if (this.#state !== STATES.REVOKED) {
      this.#state = STATES.REVOKED;
      this.#epoch += 1;
      this.#code = 'REVOKED';
      this.#vault = null;
      this.#fingerprint = null;
      this.#accountId = null;
      for (const client of this.#clients.values()) client.ready = false;
    }
    if (this.#revocationComplete) return Promise.resolve(Object.freeze({ ok: true, code: 'REVOKED' }));
    if (this.#revokePromise) return this.#revokePromise;
    const revokeVersion = this.#version;
    const promise = this.#enqueue(async () => {
      const drained = await this.#waitForDrain();
      if (!drained) return failure('DRAIN_TIMEOUT');
      const effectsSettled = await this.#waitForPendingEffects();
      if (!effectsSettled) return failure('PENDING_OPERATION_TIMEOUT');
      const clearers = new Set([this.#authority, ...[...this.#clients.values()].map(client => client.adapter)]);
      for (const adapter of clearers) {
        try { await this.#call(adapter.clear({ origin: this.#origin }), 'CLEAR_FAILED'); }
        catch { return failure('CLEAR_FAILED'); }
      }
      this.#revocationComplete = true;
      return Object.freeze({ ok: true, code: 'REVOKED', version: revokeVersion, epoch: this.#epoch });
    });
    this.#revokePromise = promise;
    const clearPending = () => { if (this.#revokePromise === promise) this.#revokePromise = null; };
    promise.then(clearPending, clearPending);
    return promise;
  }

  #isReady(client) {
    return this.#state === STATES.READY && client.attached && client.ready && !!this.#vault;
  }
  #leaseMatches(lease, client) {
    return !!lease && lease.sessionId === this.#id && lease.origin === this.#origin &&
      lease.accountId === this.#accountId && lease.clientId === client.id &&
      lease.clientInstance === client.instance && lease.version === this.#version && lease.epoch === this.#epoch;
  }
  #operationCurrent(version, epoch, client, instance) {
    return this.#state === STATES.READY && this.#version === version && this.#epoch === epoch &&
      client.attached && client.ready && client.instance === instance;
  }
  #sameAuthSnapshot(version, epoch, client, instance) {
    return this.#state !== STATES.REVOKED && this.#version === version && this.#epoch === epoch &&
      client.attached && client.instance === instance;
  }
  #token() { return Object.freeze({ version: this.#version, epoch: this.#epoch }); }
  #tokenCurrent(token) {
    return this.#epoch === token.epoch &&
      (this.#state === STATES.INITIALIZING || this.#state === STATES.REFRESHING);
  }
  #readyTokenCurrent(token) {
    return this.#state === STATES.READY && this.#epoch === token.epoch && this.#version === token.version;
  }
  #identityMatches(probe) {
    return !!probe && probe.authenticated === true && probe.identity &&
      probe.identity.accountId === this.#expectedAccountId;
  }
  #normalize(value) { return cookieList(value, this.#origin, this.#now()); }
  #safeCode(error, fallback) {
    return error && SAFE_CODES.has(error.code)
      ? error.code : fallback;
  }
  async #call(promise, code) {
    let timer;
    const effect = Promise.resolve(promise);
    this.#pendingEffects.add(effect);
    const settled = () => {
      this.#pendingEffects.delete(effect);
      if (this.#pendingEffects.size === 0) {
        for (const resolve of this.#effectWaiters) resolve(true);
        this.#effectWaiters.clear();
      }
    };
    effect.then(settled, settled);
    try {
      return await Promise.race([
        effect,
        new Promise((_, reject) => { timer = setTimeout(() => reject(Object.assign(new Error(code), { code })), this.#timeoutMs); }),
      ]);
    } finally { clearTimeout(timer); }
  }
  #enqueue(work) {
    const task = this.#writerTail.then(work, work);
    this.#writerTail = task.catch(() => undefined);
    return task;
  }
  #needsUser(token, code) {
    if (this.#tokenCurrent(token)) {
      this.#state = STATES.NEEDS_USER;
      this.#code = code;
      for (const client of this.#clients.values()) client.ready = false;
    }
    return failure(code);
  }
  #failClosed(code, state = STATES.NEEDS_USER) {
    if (this.#state === STATES.REVOKED) return;
    this.#state = state;
    this.#epoch += 1;
    this.#code = code;
    for (const client of this.#clients.values()) client.ready = false;
  }
  #signalDrain() {
    if (this.#activeUses === 0) {
      for (const resolve of this.#drainWaiters) resolve(true);
      this.#drainWaiters.clear();
    }
    for (const waiter of this.#drainWaiters) waiter.check?.();
  }
  #waitForDrain(count = () => this.#activeUses) {
    if (count() === 0) return Promise.resolve(true);
    return new Promise(resolve => {
      let timer;
      const finish = result => {
        clearTimeout(timer);
        this.#drainWaiters.delete(check);
        resolve(result);
      };
      const check = () => { if (count() === 0) finish(true); };
      check.check = check;
      this.#drainWaiters.add(check);
      timer = setTimeout(() => finish(false), this.#timeoutMs);
      check();
    });
  }
  #waitForPendingEffects() {
    if (this.#pendingEffects.size === 0) return Promise.resolve(true);
    return new Promise(resolve => {
      let timer;
      const finish = result => {
        clearTimeout(timer);
        this.#effectWaiters.delete(finish);
        resolve(result);
      };
      this.#effectWaiters.add(finish);
      timer = setTimeout(() => finish(false), this.#timeoutMs);
    });
  }
  async #fanout(token, cookies, { skipReplaceClientId } = {}) {
    for (const client of this.#clients.values()) {
      if (!client.attached) continue;
      const instance = client.instance;
      client.ready = false;
      if (!this.#tokenCurrent(token)) return failure('AUTH_EPOCH_CHANGED');
      try {
        if (client.id !== skipReplaceClientId) {
          await this.#call(client.adapter.replaceCookies({ origin: this.#origin, cookies,
            version: this.#version, epoch: this.#epoch }), 'CLIENT_SEED_FAILED');
        }
        const probe = await this.#call(client.adapter.probe({ origin: this.#origin }), 'CLIENT_PROBE_FAILED');
        const observed = this.#normalize(await this.#call(client.adapter.exportCookies({ origin: this.#origin,
          accountId: this.#accountId, version: this.#version, epoch: this.#epoch }), 'COOKIE_OBSERVATION_FAILED'));
        if (!this.#tokenCurrent(token)) return failure('AUTH_EPOCH_CHANGED');
        if (!client.attached || client.instance !== instance) continue;
        if (!this.#identityMatches(probe)) return failure('CLIENT_IDENTITY_MISMATCH');
        if (cookieFingerprint(observed) !== cookieFingerprint(cookies)) return failure('CLIENT_COOKIE_MISMATCH');
        client.ready = true;
      } catch (error) { return failure(this.#safeCode(error, 'CLIENT_FANOUT_FAILED')); }
    }
    return Object.freeze({ ok: true });
  }
}

export const CentralSessionState = STATES;
