// Private QA timing only. Never transports identity, credentials or page data.
export class QACheckpointRendezvous {
  #state = 'not_started'; #arrivals = new Set(); #waiters = []; #timer;
  arrive(role) {
    if (!['reader', 'collector'].includes(role) || this.#arrivals.has(role) ||
        !['not_started', 'waiting'].includes(this.#state)) return Promise.reject(new Error('qa_rendezvous_closed'));
    this.#arrivals.add(role);
    const result = new Promise((resolve, reject) => this.#waiters.push({ resolve, reject }));
    if (this.#arrivals.size === 2) {
      clearTimeout(this.#timer); this.#state = 'released';
      for (const waiter of this.#waiters) waiter.resolve();
      this.#waiters = [];
    } else {
      this.#state = 'waiting';
      this.#timer = setTimeout(() => this.#reject('timed_out'), 1000);
    }
    return result;
  }
  #reject(state) {
    clearTimeout(this.#timer); this.#state = state;
    for (const waiter of this.#waiters) waiter.reject(new Error('qa_rendezvous_closed'));
    this.#waiters = [];
  }
  cancel() { if (this.#state === 'waiting') this.#reject('aborted'); }
  snapshot() {
    return Object.freeze({ schema: 'aku.x.qa-checkpoint-rendezvous.v1', state: this.#state,
      arrivals: this.#arrivals.size, waitLimitMS: 1000, canAuthorize: false });
  }
}

export function synchronizedQAReport(experiment, rendezvous, readerWindow) {
  rendezvous.cancel();
  return Object.freeze({ schema: 'aku.x.qa-synchronized-concurrency.v1', experiment,
    rendezvous: rendezvous.snapshot(), readerWindow,
    accountComparison: experiment.completedCheckpoints === 4 ? 'match' : 'unknown', canAuthorize: false });
}
