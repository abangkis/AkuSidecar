import { randomUUID } from 'node:crypto';

// Generic collector admission facade. Reader windows are owned by the caller.
export class ExclusiveFallbackController {
  #id = randomUUID(); #generation = 0; #revision; #state = 'SWITCHING';
  #phase = 'startup'; #mode; #caps; #adapters; #account; #timeout;
  #tail = Promise.resolve(); #reads = new Set(); #stopped = false;
  #effects = new Set();
  #readerIntent = 0; #collector = false; #code = null;
  constructor({ expectedAccountId, capabilities, adapters, timeoutMs = 2000 }) {
    if (typeof expectedAccountId !== 'string' || !expectedAccountId.trim() ||
        !Number.isFinite(timeoutMs) || timeoutMs <= 0 ||
        !['readerCount', 'openReader', 'pauseCollector', 'resumeCollector', 'verifySession'].every(k => typeof adapters?.[k] === 'function')) throw new Error('CONTROLLER_INVALID');
    this.#account = expectedAccountId; this.#adapters = adapters; this.#timeout = timeoutMs;
    this.#caps = capabilities; this.#mode = this.#qualified(capabilities) ? 'concurrent' : 'exclusive';
  }
  #qualified(c) { return ['cookieObservation', 'startupCoverage', 'ownership'].every(k => c?.[k] === 'verified'); }
  status() { return Object.freeze({ state: this.#state, mode: this.#mode, phase: this.#phase, generation: this.#generation, code: this.#code, cleanupPending: this.#effects.size > 0 }); }
  #track(effect) {
    this.#effects.add(effect);
    const settled = () => {
      this.#effects.delete(effect);
      if (this.#stopped && this.#effects.size === 0 && !this.#collector && this.#code !== 'COLLECTOR_STOP_FAILED') { this.#phase = 'stopped'; this.#code = null; }
    };
    effect.then(settled, settled);
    return effect;
  }
  #fence(phase) {
    ++this.#generation; this.#state = 'SWITCHING'; this.#phase = phase; this.#code = null;
    for (const r of this.#reads) r.abort.abort();
    return this.#generation;
  }
  #current(g) { return !this.#stopped && g === this.#generation; }
  #fail(code) { if (!this.#stopped) { this.#state = 'NEEDS_USER'; this.#phase = 'blocked'; this.#code = code; } return { ok: false, code }; }
  #serialize(fn) { const task = this.#tail.then(fn); this.#tail = task.catch(() => {}); return task; }
  async #bounded(p) {
    let timer;
    try { return await Promise.race([Promise.resolve(p), new Promise((_, reject) => { timer = setTimeout(() => reject(new Error('TIMEOUT')), this.#timeout); })]); }
    finally { clearTimeout(timer); }
  }
  async #count() { const n = await this.#bounded(this.#adapters.readerCount()); if (!Number.isInteger(n) || n < 0) throw new Error('INVENTORY_FAILED'); return n; }
  async #verify() {
    const v = await this.#bounded(this.#adapters.verifySession());
    if (!v?.authenticated || v.accountId !== this.#account || typeof v.revision !== 'string' || !v.revision) throw new Error('SESSION_INVALID');
    return v;
  }
  async #pause() {
    const pausing = this.#track(Promise.resolve().then(() => this.#adapters.pauseCollector()).then(result => {
      if (result?.stopped !== true) throw new Error('COLLECTOR_STOP_FAILED');
      this.#collector = false;
      return result;
    }));
    await this.#bounded(pausing);
  }
  async #drainPause(g) {
    try {
      await this.#bounded(Promise.all([...this.#reads].map(r => r.done)));
      if (!this.#current(g)) return false;
      await this.#pause();
      return this.#current(g);
    } catch { this.#fail('DRAIN_OR_PAUSE_FAILED'); return false; }
  }
  async #admit(g) {
    if (!this.#current(g) || this.#state === 'NEEDS_USER' || this.#state === 'READY') return this.status();
    try {
      if (this.#mode === 'exclusive') {
        const count = await this.#count(); if (!this.#current(g)) return this.status();
        if (count > 0 || this.#readerIntent) { this.#phase = 'reader_paused'; return this.status(); }
      }
      const verified = await this.#verify(); if (!this.#current(g)) return this.status();
      let startAccepted = true;
      // Invocation may partly start a process before rejecting or timing out.
      this.#collector = true;
      const starting = Promise.resolve().then(() => this.#adapters.resumeCollector(verified));
      const lateCleanup = async () => {
        if (!startAccepted || !this.#current(g)) {
          this.#collector = true;
          try { await this.#pause(); } catch { if (this.#stopped) this.#code = 'COLLECTOR_STOP_FAILED'; else this.#fail('COLLECTOR_STOP_FAILED'); }
        }
      };
      this.#track(starting.then(lateCleanup, lateCleanup));
      let result;
      try { result = await this.#bounded(starting); } catch (error) { startAccepted = false; throw error; }
      // A late start must be stopped even when its initiating epoch is obsolete.
      this.#collector = true;
      if (!this.#current(g) || (this.#mode === 'exclusive' && this.#readerIntent)) { await this.#pause(); return this.status(); }
      if (result?.ready !== true || result.accountId !== this.#account || result.revision !== verified.revision) throw new Error('COLLECTOR_START_FAILED');
      const after = await this.#verify(); if (!this.#current(g)) { await this.#pause(); return this.status(); }
      if (after.revision !== verified.revision) throw new Error('REVISION_CHANGED');
      if (this.#mode === 'exclusive') {
        const count = await this.#count(); if (!this.#current(g)) { await this.#pause(); return this.status(); }
        if (count !== 0 || this.#readerIntent) { await this.#pause(); this.#phase = 'reader_paused'; return this.status(); }
      }
      this.#revision = after.revision; this.#state = 'READY'; this.#phase = 'collector_ready';
    } catch {
      if (this.#collector) { try { await this.#pause(); } catch { return this.#fail('COLLECTOR_STOP_FAILED'); } }
      this.#fail('SESSION_OR_START_FAILED');
    }
    return this.status();
  }
  start() { const g = this.#generation; return this.#serialize(() => this.#admit(g)); }
  setCapabilities(caps) {
    this.#caps = caps;
    if (this.#stopped || this.#mode === 'exclusive' || this.#qualified(caps)) return Promise.resolve(this.status());
    this.#mode = 'exclusive';
    if (this.#state === 'NEEDS_USER') return Promise.resolve(this.status());
    const g = this.#fence('draining');
    return this.#serialize(async () => { if (await this.#drainPause(g)) await this.#admit(g); return this.status(); });
  }
  requestReader() {
    if (this.#stopped || this.#state === 'NEEDS_USER') return Promise.resolve({ ok: false, code: 'SESSION_NOT_READY' });
    ++this.#readerIntent;
    const g = this.#mode === 'exclusive' ? this.#fence('reader_requested') : this.#generation;
    return this.#serialize(async () => {
      try {
        if (this.#mode === 'exclusive' && !(await this.#drainPause(g))) return { ok: false, code: this.#code ?? 'LEASE_STALE' };
        if (!this.#current(g)) return { ok: false, code: 'LEASE_STALE' };
        await this.#bounded(this.#adapters.openReader());
        if (!this.#current(g)) return { ok: false, code: 'LEASE_STALE' };
        if (this.#mode === 'exclusive') this.#phase = 'reader_paused';
        return { ok: true };
      } catch { return this.#fail('READER_OPEN_FAILED'); }
      finally { --this.#readerIntent; }
    });
  }
  readerClosed() {
    const g = this.#generation;
    return this.#serialize(async () => {
      if (!this.#current(g) || this.#state === 'NEEDS_USER' || this.#mode !== 'exclusive' || this.#state === 'READY') return this.status();
      return this.#admit(g);
    });
  }
  lease(clientId) { return clientId === 'collector' && this.#state === 'READY' && !this.#stopped ? Object.freeze({ controllerId: this.#id, mode: this.#mode, generation: this.#generation }) : null; }
  #observedReader() {
    const g = this.#fence('reader_paused');
    this.#serialize(async () => { await this.#drainPause(g); return this.status(); });
  }
  async use(clientId, lease, read) {
    const g = lease?.generation;
    const valid = () => clientId === 'collector' && lease?.controllerId === this.#id && lease.mode === this.#mode && this.#current(g) && this.#state === 'READY';
    if (!valid()) return { ok: false, code: 'LEASE_STALE' };
    const record = { abort: new AbortController(), physical: null }; let finish;
    record.done = new Promise(resolve => { finish = resolve; }); this.#reads.add(record);
    try {
      if (this.#mode === 'exclusive') { const n = await this.#count(); if (!valid()) return { ok: false, code: 'LEASE_STALE' }; if (n !== 0) { this.#observedReader(); return { ok: false, code: 'LEASE_STALE' }; } }
      const before = await this.#verify(); if (!valid()) return { ok: false, code: 'LEASE_STALE' };
      if (before.revision !== this.#revision) throw new Error('REVISION_CHANGED');
      record.physical = Promise.resolve(read({ accountId: this.#account, generation: g, signal: record.abort.signal }));
      const value = await this.#bounded(record.physical);
      if (!valid()) return { ok: false, code: 'LEASE_STALE' };
      const after = await this.#verify(); if (!valid()) return { ok: false, code: 'LEASE_STALE' };
      if (after.revision !== this.#revision) throw new Error('REVISION_CHANGED');
      if (this.#mode === 'exclusive') { const n = await this.#count(); if (!valid()) return { ok: false, code: 'LEASE_STALE' }; if (n !== 0) { this.#observedReader(); return { ok: false, code: 'LEASE_STALE' }; } }
      return { ok: true, value };
    } catch { if (!valid()) return { ok: false, code: 'LEASE_STALE' }; return this.#fail('SESSION_OR_READ_FAILED'); }
    finally {
      const drained = () => { this.#reads.delete(record); finish(); };
      if (record.physical) record.physical.then(drained, drained); else drained();
    }
  }
  stop() {
    if (this.#stopped) return this.#tail.then(() => this.status());
    this.#fence('stopping'); this.#stopped = true; this.#state = 'REVOKED'; this.#phase = 'stopped';
    return this.#serialize(async () => {
      try {
        await this.#bounded(Promise.all([...this.#effects]));
        await this.#pause();
        this.#phase = 'stopped'; this.#code = null;
      } catch {
        this.#phase = this.#effects.size ? 'cleanup_pending' : 'stop_failed';
        this.#code = this.#effects.size ? 'CLEANUP_PENDING' : 'COLLECTOR_STOP_FAILED';
      }
      return this.status();
    });
  }
}
