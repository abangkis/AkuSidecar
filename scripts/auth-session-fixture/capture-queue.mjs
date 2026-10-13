// Read-only fixture jobs. Pending descriptors survive a strategy handoff in memory.
// This is not a durable Store, and callbacks must never perform writes or refresh.
const ID = /^[a-zA-Z0-9_-]{1,80}$/;
const TRANSIENT = new Set(['SWITCHING', 'REFRESHING']);
export class FixtureCaptureQueue {
  #session;
  #jobs = new Map();
  #maxAttempts;
  constructor({ session, maxAttempts = 3 }) {
    if (!session || !['use', 'lease', 'status'].every(k => typeof session[k] === 'function') ||
        !Number.isInteger(maxAttempts) || maxAttempts < 1 || maxAttempts > 5) throw new Error('QUEUE_INVALID');
    this.#session = session;
    this.#maxAttempts = maxAttempts;
  }
  enqueueRead({ id, clientId = 'collector', read }) {
    if (typeof id !== 'string' || !ID.test(id) || !['reader', 'collector'].includes(clientId) || typeof read !== 'function') {
      throw new Error('READ_JOB_INVALID');
    }
    const existing = this.#jobs.get(id);
    if (existing) {
      if (existing.read !== read || existing.clientId !== clientId) throw new Error('READ_JOB_CONFLICT');
      return this.status(id);
    }
    if (this.#jobs.size >= 100) throw new Error('QUEUE_FULL');
    this.#jobs.set(id, { id, clientId, read, state: 'pending', attempts: 0, code: null, value: null, running: null });
    return this.status(id);
  }
  status(id) {
    const job = this.#jobs.get(id);
    return job ? Object.freeze({ id, state: job.state, attempts: job.attempts, code: job.code }) : null;
  }
  result(id) {
    const job = this.#jobs.get(id);
    return job?.state === 'completed' ? structuredClone(job.value) : null;
  }
  run(id) {
    const job = this.#jobs.get(id);
    if (!job) return Promise.resolve({ ok: false, code: 'JOB_UNKNOWN' });
    if (job.running) return job.running;
    if (job.state !== 'pending') return Promise.resolve(this.status(id));
    const state = this.#session.status().state;
    if (TRANSIENT.has(state)) return Promise.resolve(this.status(id));
    const lease = this.#session.lease(job.clientId);
    if (!lease) {
      job.state = 'blocked'; job.code = 'SESSION_NOT_READY';
      return Promise.resolve(this.status(id));
    }
    job.state = 'running';
    job.attempts++;
    const task = this.#attempt(job, lease).finally(() => { job.running = null; });
    job.running = task;
    return task;
  }
  async #attempt(job, lease) {
    let result;
    try {
      result = await this.#session.use(job.clientId, lease, async context => {
        const value = await job.read(context);
        return value?.body?.account === context?.accountId ? value : null;
      });
    }
    catch { result = { ok: false, code: 'READ_FAILED' }; }
    if (result.ok) {
      // Only synthetic post reads are admitted. No arbitrary callback output/credentials.
      const value = result.value;
      if (value?.status !== 200 || !/^fixture-post-[0-9]+$/.test(value?.body?.postId ?? '') ||
          !['alpha', 'beta'].includes(value?.body?.account)) {
        job.state = 'blocked'; job.code = 'READ_RESPONSE_INVALID';
      } else {
        job.value = { postId: value.body.postId, account: value.body.account };
        job.state = 'completed'; job.code = null;
      }
    } else {
      const state = this.#session.status().state;
      if (result.code === 'LEASE_STALE' && (state === 'READY' || TRANSIENT.has(state))) {
        job.state = job.attempts < this.#maxAttempts ? 'pending' : 'exhausted';
        job.code = job.state === 'pending' ? 'RETRY_AFTER_HANDOFF' : 'ATTEMPTS_EXHAUSTED';
      } else {
        job.state = 'blocked'; job.code = 'SESSION_OR_READ_FAILED';
      }
    }
    return this.status(job.id);
  }
  // Call after an awaited refresh/switch. Conflicts are never auto-replayed.
  async resume() {
    return Promise.all([...this.#jobs.values()].filter(j => j.state === 'pending').map(j => this.run(j.id)));
  }
}
