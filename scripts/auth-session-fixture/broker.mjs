import { randomUUID } from 'node:crypto';

/**
 * Local-fixture-only authentication session broker.
 *
 * Public adapter contract:
 *
 *   owner.probe({ origin }) -> { authenticated, identity: { accountId } }
 *   owner.exportCookies({ origin, accountId, authEpoch }) -> { cookies: CDPCookie[] }
 *   derived.create({ origin, generation }) -> context
 *   derived.seed({ context, origin, cookies: CookieParam[] }) -> void
 *   derived.probe({ context, origin }) -> { authenticated, identity: { accountId } }
 *   derived.dispose({ context }) -> void
 *
 * The broker accepts only explicitly configured HTTP loopback fixture origins.
 * Cookie values exist only in the adapter handoff and are never returned by
 * status, leases, or operation results. The derived context is never exported.
 */

const STATES = Object.freeze({
  UNSEEDED: 'UNSEEDED',
  BOOTSTRAPPING: 'BOOTSTRAPPING',
  VERIFIED: 'VERIFIED',
  STALE: 'STALE',
  NEEDS_USER: 'NEEDS_USER',
  REVOKED: 'REVOKED',
});

const LOOPBACK_HOSTS = new Set(['127.0.0.1', 'localhost']);
const SAME_SITE_VALUES = new Set(['Strict', 'Lax', 'None']);
const MAX_FIXTURE_ORIGINS = 16;
const MAX_COOKIES = 256;
const MAX_COOKIE_NAME_LENGTH = 256;
const MAX_COOKIE_VALUE_LENGTH = 16 * 1024;
const MAX_ACCOUNT_ID_LENGTH = 512;

function codedError(code) {
  const error = new Error(code);
  error.code = code;
  return error;
}

function fixtureOrigin(value) {
  if (typeof value !== 'string') throw codedError('FIXTURE_ORIGIN_NOT_ALLOWED');
  let url;
  try {
    url = new URL(value);
  } catch {
    throw codedError('FIXTURE_ORIGIN_NOT_ALLOWED');
  }
  if (url.protocol !== 'http:' || !LOOPBACK_HOSTS.has(url.hostname) ||
      url.username || url.password || url.pathname !== '/' || url.search || url.hash ||
      value !== url.origin) {
    throw codedError('FIXTURE_ORIGIN_NOT_ALLOWED');
  }
  return url.origin;
}

function partitionSite(value) {
  if (typeof value !== 'string') throw codedError('COOKIE_SNAPSHOT_INVALID');
  let url;
  try {
    url = new URL(value);
  } catch {
    throw codedError('PARTITION_KEY_OUTSIDE_FIXTURE');
  }
  if (url.protocol !== 'http:' || !LOOPBACK_HOSTS.has(url.hostname) || url.port ||
      url.username || url.password || url.pathname !== '/' || url.search || url.hash ||
      value !== `http://${url.hostname}`) {
    throw codedError('PARTITION_KEY_OUTSIDE_FIXTURE');
  }
  return value;
}

function safeAccountId(value) {
  return typeof value === 'string' && value.length > 0 &&
    value.length <= MAX_ACCOUNT_ID_LENGTH && value.trim() === value;
}

function readIdentity(probe) {
  if (!probe || probe.authenticated !== true || !probe.identity ||
      !safeAccountId(probe.identity.accountId)) return null;
  return Object.freeze({ accountId: probe.identity.accountId });
}

function cookieParam(cookie, origin, nowMs) {
  if (!cookie || typeof cookie !== 'object' || Array.isArray(cookie)) {
    throw codedError('COOKIE_SNAPSHOT_INVALID');
  }

  const url = new URL(origin);
  const domain = cookie.domain;
  if (typeof domain !== 'string' || !domain || domain.endsWith('.')) {
    throw codedError('COOKIE_SNAPSHOT_INVALID');
  }
  const domainHost = domain.startsWith('.') ? domain.slice(1) : domain;
  if (domainHost.toLowerCase() !== url.hostname) {
    throw codedError('COOKIE_OUTSIDE_FIXTURE');
  }

  if (typeof cookie.name !== 'string' || cookie.name.length === 0 ||
      cookie.name.length > MAX_COOKIE_NAME_LENGTH || /[\s;,=]/.test(cookie.name) ||
      typeof cookie.value !== 'string' || cookie.value.length === 0 ||
      cookie.value.length > MAX_COOKIE_VALUE_LENGTH ||
      typeof cookie.path !== 'string' || !cookie.path.startsWith('/') ||
      /[\r\n;]/.test(cookie.path) || typeof cookie.httpOnly !== 'boolean' ||
      typeof cookie.secure !== 'boolean') {
    throw codedError('COOKIE_SNAPSHOT_INVALID');
  }

  const session = cookie.session;
  let expires = cookie.expires;
  if (expires === undefined && session === true) expires = -1;
  if (typeof expires !== 'number' || !Number.isFinite(expires) ||
      (session === true && expires > 0) || (session === false && expires <= 0)) {
    throw codedError('COOKIE_SNAPSHOT_INVALID');
  }
  if (expires > 0 && expires <= nowMs / 1000) throw codedError('COOKIE_EXPIRED');

  const sameSite = cookie.sameSite;
  if (sameSite !== undefined && sameSite !== 'Unspecified' &&
      !SAME_SITE_VALUES.has(sameSite)) {
    throw codedError('COOKIE_SNAPSHOT_INVALID');
  }
  if (sameSite === 'None' && cookie.secure !== true) {
    throw codedError('COOKIE_SNAPSHOT_INVALID');
  }

  let partitionKey;
  if (cookie.partitionKey !== undefined && cookie.partitionKey !== null) {
    if (typeof cookie.partitionKey === 'string') {
      partitionKey = partitionSite(cookie.partitionKey);
    } else if (cookie.partitionKey && typeof cookie.partitionKey === 'object' &&
        !Array.isArray(cookie.partitionKey) &&
        typeof cookie.partitionKey.topLevelSite === 'string' &&
        typeof cookie.partitionKey.hasCrossSiteAncestor === 'boolean') {
      partitionKey = Object.freeze({
        topLevelSite: partitionSite(cookie.partitionKey.topLevelSite),
        hasCrossSiteAncestor: cookie.partitionKey.hasCrossSiteAncestor,
      });
    } else {
      throw codedError('COOKIE_SNAPSHOT_INVALID');
    }
  }
  if (cookie.partitionKeyOpaque === true) {
    throw codedError('PARTITION_KEY_OUTSIDE_FIXTURE');
  }

  const result = {
    name: cookie.name,
    value: cookie.value,
    domain,
    path: cookie.path,
    expires,
    httpOnly: cookie.httpOnly,
    secure: cookie.secure,
  };
  if (sameSite !== undefined && sameSite !== 'Unspecified') result.sameSite = sameSite;
  if (partitionKey !== undefined) result.partitionKey = partitionKey;
  return Object.freeze(result);
}

function cookieParams(snapshot, origin, nowMs) {
  if (!snapshot || !Array.isArray(snapshot.cookies) ||
      snapshot.cookies.length === 0 || snapshot.cookies.length > MAX_COOKIES) {
    throw codedError('COOKIE_SNAPSHOT_INVALID');
  }
  return Object.freeze(snapshot.cookies.map(cookie => cookieParam(cookie, origin, nowMs)));
}

function validAdapter(adapter, methods) {
  return adapter && methods.every(method => typeof adapter[method] === 'function');
}

export class AuthSessionBroker {
  #owner;
  #derived;
  #fixtureOrigins;
  #now;
  #brokerId = randomUUID();
  #tail = Promise.resolve();
  #state = STATES.UNSEEDED;
  #generation = 0;
  #authEpoch = 0;
  #code = null;
  #context = null;
  #orphans = new Set();
  #identity = null;
  #origin = null;
  #staleEpisode = null;
  #nextEpisodeId = 1;

  constructor({ fixtureOrigins, owner, derived, now = () => Date.now() } = {}) {
    if (!Array.isArray(fixtureOrigins) || fixtureOrigins.length === 0 ||
        fixtureOrigins.length > MAX_FIXTURE_ORIGINS) {
      throw codedError('FIXTURE_ORIGIN_NOT_ALLOWED');
    }
    const origins = fixtureOrigins.map(fixtureOrigin);
    if (new Set(origins).size !== origins.length) throw codedError('FIXTURE_ORIGIN_NOT_ALLOWED');
    if (!validAdapter(owner, ['probe', 'exportCookies'])) throw codedError('OWNER_ADAPTER_INVALID');
    if (!validAdapter(derived, ['create', 'seed', 'probe', 'dispose'])) {
      throw codedError('DERIVED_ADAPTER_INVALID');
    }
    if (typeof now !== 'function') throw codedError('CLOCK_INVALID');
    this.#fixtureOrigins = new Set(origins);
    this.#owner = owner;
    this.#derived = derived;
    this.#now = now;
  }

  status() {
    return Object.freeze({
      state: this.#state,
      generation: this.#generation,
      authEpoch: this.#authEpoch,
      code: this.#code,
      hasContext: this.#context !== null,
      undisposedContextCount: this.#orphans.size,
    });
  }

  lease() {
    if (this.#state !== STATES.VERIFIED || !this.#context || !this.#identity || !this.#origin) {
      return null;
    }
    return Object.freeze({
      brokerId: this.#brokerId,
      generation: this.#generation,
      authEpoch: this.#authEpoch,
      accountId: this.#identity.accountId,
      origin: this.#origin,
    });
  }

  admitResult(lease) {
    return this.#leaseMatches(lease) && this.#state === STATES.VERIFIED && this.#context !== null;
  }

  getContext(lease) {
    return this.admitResult(lease) ? this.#context : null;
  }

  bootstrap({ origin, expectedAccountId } = {}) {
    let allowedOrigin;
    try {
      allowedOrigin = fixtureOrigin(origin);
    } catch {
      return Promise.resolve(Object.freeze({ ok: false, code: 'FIXTURE_ORIGIN_NOT_ALLOWED' }));
    }
    if (!this.#fixtureOrigins.has(allowedOrigin)) {
      return Promise.resolve(Object.freeze({ ok: false, code: 'FIXTURE_ORIGIN_NOT_ALLOWED' }));
    }
    if (expectedAccountId !== undefined && !safeAccountId(expectedAccountId)) {
      return Promise.resolve(Object.freeze({ ok: false, code: 'ACCOUNT_MISMATCH' }));
    }

    const previousContext = this.#context;
    const operation = this.#fence(STATES.BOOTSTRAPPING);
    this.#context = null;
    this.#identity = null;
    this.#origin = allowedOrigin;
    this.#staleEpisode = null;

    return this.#enqueue(async () => {
      const cleaned = await this.#disposeAll([previousContext]);
      if (!this.#isCurrent(operation)) return this.#cancelled();
      if (!cleaned) return this.#needsUser(operation, 'CONTEXT_DISPOSAL_FAILED');
      return this.#bootstrapInQueue({
        origin: allowedOrigin,
        expectedAccountId,
        operation,
      });
    });
  }

  markStale(lease) {
    if (!this.admitResult(lease)) return null;
    const episode = Object.freeze({
      brokerId: this.#brokerId,
      episodeId: this.#nextEpisodeId++,
      generation: this.#generation,
      authEpoch: this.#authEpoch,
      accountId: this.#identity.accountId,
      origin: this.#origin,
    });
    this.#state = STATES.STALE;
    this.#code = 'SESSION_STALE';
    this.#staleEpisode = {
      episode,
      generation: this.#generation,
      authEpoch: this.#authEpoch,
      consumed: false,
    };
    return episode;
  }

  recoverStale({ episode } = {}) {
    const active = this.#staleEpisode;
    if (this.#state !== STATES.STALE || !active || active.consumed ||
        !episode || episode.brokerId !== this.#brokerId ||
        episode.episodeId !== active.episode.episodeId ||
        episode.generation !== active.generation ||
        episode.authEpoch !== active.authEpoch ||
        episode.accountId !== active.episode.accountId ||
        episode.origin !== active.episode.origin) {
      return Promise.resolve(Object.freeze({ ok: false, code: 'RECOVERY_NOT_AVAILABLE' }));
    }
    active.consumed = true;
    const previousContext = this.#context;
    const operation = this.#fence(STATES.BOOTSTRAPPING);
    this.#context = null;
    this.#identity = null;
    this.#origin = active.episode.origin;

    return this.#enqueue(async () => {
      const cleaned = await this.#disposeAll([previousContext]);
      if (!this.#isCurrent(operation)) return this.#cancelled();
      if (!cleaned) return this.#needsUser(operation, 'CONTEXT_DISPOSAL_FAILED');
      return this.#bootstrapInQueue({
        origin: active.episode.origin,
        expectedAccountId: active.episode.accountId,
        operation,
      });
    });
  }

  revoke() {
    const previousContext = this.#context;
    const operation = this.#fence(STATES.REVOKED);
    this.#context = null;
    this.#identity = null;
    this.#origin = null;
    this.#staleEpisode = null;
    this.#code = 'REVOKED';
    return this.#enqueue(async () => {
      const cleaned = await this.#disposeAll([previousContext]);
      if (!cleaned) {
        if (this.#sameEpoch(operation)) {
          this.#state = STATES.NEEDS_USER;
          this.#code = 'CONTEXT_DISPOSAL_FAILED';
        }
        return Object.freeze({ ok: false, code: 'CONTEXT_DISPOSAL_FAILED' });
      }
      if (this.#sameEpoch(operation)) {
        this.#state = STATES.REVOKED;
        this.#code = 'REVOKED';
      }
      return Object.freeze({ ok: true, code: 'REVOKED' });
    });
  }

  #fence(state) {
    this.#generation += 1;
    this.#authEpoch += 1;
    this.#state = state;
    this.#code = null;
    return Object.freeze({ generation: this.#generation, authEpoch: this.#authEpoch });
  }

  #leaseMatches(lease) {
    return !!lease && lease.brokerId === this.#brokerId &&
      lease.generation === this.#generation &&
      lease.authEpoch === this.#authEpoch && this.#identity !== null &&
      lease.accountId === this.#identity.accountId && lease.origin === this.#origin;
  }

  #isCurrent(operation) {
    return this.#state === STATES.BOOTSTRAPPING &&
      operation.generation === this.#generation && operation.authEpoch === this.#authEpoch;
  }

  #sameEpoch(operation) {
    return operation.generation === this.#generation && operation.authEpoch === this.#authEpoch;
  }

  #needsUser(operation, code) {
    if (this.#isCurrent(operation)) {
      this.#context = null;
      this.#identity = null;
      this.#state = STATES.NEEDS_USER;
      this.#code = code;
    }
    return Object.freeze({ ok: false, code });
  }

  #cancelled() {
    return Object.freeze({ ok: false, code: 'AUTH_EPOCH_CHANGED' });
  }

  #enqueue(work) {
    const task = this.#tail.then(work, work);
    this.#tail = task.catch(() => undefined);
    return task;
  }

  async #dispose(context) {
    if (!context) return true;
    try {
      const result = await this.#derived.dispose({ context });
      if (result === false) throw codedError('CONTEXT_DISPOSAL_FAILED');
      this.#orphans.delete(context);
      return true;
    } catch {
      this.#orphans.add(context);
      return false;
    }
  }

  async #disposeAll(extraContexts = []) {
    const contexts = new Set([...this.#orphans, ...extraContexts].filter(Boolean));
    let cleaned = true;
    for (const context of contexts) {
      if (!(await this.#dispose(context))) cleaned = false;
    }
    return cleaned && this.#orphans.size === 0;
  }

  async #bootstrapInQueue({ origin, expectedAccountId, operation }) {
    let context = null;
    const fail = async code => {
      const disposed = await this.#dispose(context);
      context = null;
      if (!disposed) code = 'CONTEXT_DISPOSAL_FAILED';
      if (!this.#isCurrent(operation)) {
        return disposed ? this.#cancelled()
          : Object.freeze({ ok: false, code: 'CONTEXT_DISPOSAL_FAILED' });
      }
      if (this.#isCurrent(operation)) {
        this.#context = null;
        this.#identity = null;
        this.#state = STATES.NEEDS_USER;
        this.#code = code;
      }
      return Object.freeze({ ok: false, code });
    };
    const cancelled = async () => {
      const disposed = await this.#dispose(context);
      context = null;
      return disposed ? this.#cancelled()
        : Object.freeze({ ok: false, code: 'CONTEXT_DISPOSAL_FAILED' });
    };

    try {
      let ownerProbe;
      try {
        ownerProbe = await this.#owner.probe({ origin });
      } catch {
        return await fail('OWNER_PROBE_FAILED');
      }
      if (!this.#isCurrent(operation)) return await cancelled();
      const ownerIdentity = readIdentity(ownerProbe);
      if (!ownerIdentity) return await fail('OWNER_AUTH_REQUIRED');
      if (expectedAccountId !== undefined && ownerIdentity.accountId !== expectedAccountId) {
        return await fail('ACCOUNT_MISMATCH');
      }

      let snapshot;
      try {
        snapshot = await this.#owner.exportCookies({
          origin,
          accountId: ownerIdentity.accountId,
          authEpoch: operation.authEpoch,
        });
      } catch {
        return await fail('COOKIE_EXPORT_FAILED');
      }
      if (!this.#isCurrent(operation)) return await cancelled();

      let cookies;
      try {
        cookies = cookieParams(snapshot, origin, this.#now());
      } catch (error) {
        return await fail(error?.code && typeof error.code === 'string'
          ? error.code
          : 'COOKIE_SNAPSHOT_INVALID');
      }

      try {
        context = await this.#derived.create({ origin, generation: operation.generation });
      } catch {
        return await fail('DERIVED_CREATE_FAILED');
      }
      if (!context) return await fail('DERIVED_CREATE_FAILED');
      if (!this.#isCurrent(operation)) return await cancelled();

      try {
        await this.#derived.seed({ context, origin, cookies });
      } catch {
        return await fail('DERIVED_SEED_FAILED');
      }
      if (!this.#isCurrent(operation)) return await cancelled();

      let derivedProbe;
      try {
        derivedProbe = await this.#derived.probe({ context, origin });
      } catch {
        return await fail('DERIVED_PROBE_FAILED');
      }
      if (!this.#isCurrent(operation)) return await cancelled();
      const derivedIdentity = readIdentity(derivedProbe);
      if (!derivedIdentity) return await fail('DERIVED_AUTH_REQUIRED');
      if (derivedIdentity.accountId !== ownerIdentity.accountId) {
        return await fail('DERIVED_IDENTITY_MISMATCH');
      }

      let finalOwnerProbe;
      try {
        finalOwnerProbe = await this.#owner.probe({ origin });
      } catch {
        return await fail('OWNER_PROBE_FAILED');
      }
      if (!this.#isCurrent(operation)) return await cancelled();
      const finalOwnerIdentity = readIdentity(finalOwnerProbe);
      if (!finalOwnerIdentity || finalOwnerIdentity.accountId !== ownerIdentity.accountId) {
        return await fail('OWNER_SESSION_CHANGED');
      }

      this.#context = context;
      context = null;
      this.#identity = ownerIdentity;
      this.#origin = origin;
      this.#state = STATES.VERIFIED;
      this.#code = null;
      this.#staleEpisode = null;
      return Object.freeze({ ok: true, code: 'VERIFIED', lease: this.lease() });
    } catch {
      return await fail('BROKER_FAILED');
    }
  }
}

export const AuthSessionState = STATES;
// Shared strict normalization for the second, central-vault fixture experiment.
export { cookieParams as normalizeFixtureCookies };
