export const timelineHlsLimits = Object.freeze({ playlist: 64 * 1024, resource: 12 * 1024 * 1024, playback: 64 * 1024 * 1024, timeout: 15_000 });

function newStats() {
  return { aborted: false, loaded: 0, retry: 0, total: 0, chunkCount: 0, bwEstimate: 0,
    loading: { start: 0, first: 0, end: 0 }, parsing: { start: 0, end: 0 }, buffering: { start: 0, first: 0, end: 0 } };
}

// One factory per playback session: every loader instance shares its budget.
// No native loader may bypass this boundary, including ancillary HLS requests.
export function createTimelineHlsLoader({ policy, signal, onTerminalError = () => {}, limits = timelineHlsLimits, admission, onPilotActivated = () => {} }) {
  if (!policy?.authorizeRequest || !policy?.validatePlaylist) throw new Error("HLS resource policy is required.");
  let remaining = limits.playback;
  let allowance = limits.playback;
  let terminalError = null;
  function terminal(message) {
    const error = message instanceof Error ? message : new Error(message);
    if (!terminalError) { terminalError = error; onTerminalError(error); }
    return error;
  }
  function budgetError() { return Object.assign(new Error("HLS playback exceeds its byte limit."), { sessionBudget: true }); }
  // Preflight uses the same trust rules, request bounds, cancellation and
  // cumulative accounting. An unsuitable candidate can be excluded without
  // poisoning lower variants, but exhaustion of the shared quota remains fatal.
  async function readPreflight(context, preflightSignal) {
    const url = policy.authorizeRequest(context);
    if (typeof url !== "string" || !url) throw Error("HLS resource policy denied request.");
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), limits.timeout);
    const signals = [preflightSignal, controller.signal, ...(signal ? [signal] : [])];
    const requestSignal = AbortSignal.any(signals);
    let reader;
    try {
      requestSignal.throwIfAborted();
      const response = await fetch(url, { signal: requestSignal, credentials: "omit", redirect: "error", referrerPolicy: "no-referrer" });
      requestSignal.throwIfAborted();
      if (!response.ok || !response.body) throw Error("HLS preflight playlist unavailable.");
      const hint = Number(response.headers.get("content-length"));
      if (hint > limits.playlist) throw Error("HLS preflight playlist exceeds its byte limit.");
      if (hint > remaining) throw budgetError();
      reader = response.body.getReader();
      const chunks = [];
      let loaded = 0;
      while (true) {
        const { done, value } = await reader.read();
        requestSignal.throwIfAborted();
        if (done) break;
        remaining -= value.byteLength;
        loaded += value.byteLength;
        if (remaining < 0) throw budgetError();
        if (loaded > limits.playlist) throw Error("HLS preflight playlist exceeds its byte limit.");
        chunks.push(value);
      }
      const bytes = new Uint8Array(loaded);
      let offset = 0;
      for (const chunk of chunks) { bytes.set(chunk, offset); offset += chunk.byteLength; }
      const text = new TextDecoder().decode(bytes);
      policy.validatePlaylist(text, { ...context, url });
      return text;
    } finally {
      clearTimeout(timer);
      controller.abort();
      if (reader) { await reader.cancel().catch(() => {}); reader.releaseLock(); }
    }
  }
  return class TimelineHlsLoader {
    constructor() {
      this.context = null;
      this.stats = newStats();
      this.controller = null;
      this.callbacks = null;
      this.response = null;
      this.generation = 0;
      this.done = false;
      this.cleanup = () => {};
    }
    load(context, config, callbacks) {
      this.abort();
      const generation = ++this.generation;
      this.context = context;
      this.callbacks = callbacks;
      const retry = this.stats.retry;
      Object.assign(this.stats, newStats(), { retry });
      this.done = false;
      const stats = this.stats;
      stats.loading.start = performance.now();
      const controller = this.controller = new AbortController();
      const abort = () => this.abort();
      signal?.addEventListener("abort", abort, { once: true });
      let timedOut = false;
      const timer = setTimeout(() => { timedOut = true; controller.abort(); }, limits.timeout);
      this.cleanup = () => { clearTimeout(timer); signal?.removeEventListener("abort", abort); };
      const current = () => generation === this.generation && !stats.aborted;
      if (signal?.aborted) { this.abort(); return; }
      void (async () => {
        let reader;
        try {
          if (terminalError) throw terminalError;
          let url;
          try { url = policy.authorizeRequest(context); }
          catch (error) { throw terminal(error); }
          if (typeof url !== "string" || !url) throw terminal("HLS resource policy denied request.");
          if (!current()) return;
          const playlist = ["manifest", "level", "audioTrack", "subtitleTrack"].includes(context.type);
          const profile = !playlist ? admission?.resourceProfile({ ...context, url }) : null;
          if (profile && profile.playback > allowance) {
            remaining += profile.playback - allowance;
            allowance = profile.playback;
            onPilotActivated();
          }
          const limit = playlist ? limits.playlist : profile?.resource ?? limits.resource;
          const response = this.response = await fetch(url, { signal: controller.signal, credentials: "omit", redirect: "error", referrerPolicy: "no-referrer" });
          if (!current()) return;
          stats.loading.first = performance.now();
          if (!response.ok || !response.body) throw new Error("HLS resource unavailable.");
          const sizeHint = Number(response.headers.get("content-length"));
          if (sizeHint > remaining) throw terminal(budgetError());
          if (sizeHint > limit) throw terminal("HLS resource exceeds its byte limit.");
          reader = response.body.getReader();
          const chunks = [];
          while (current()) {
            const { done, value } = await reader.read();
            if (!current()) return;
            if (done) break;
            remaining -= value.byteLength;
            stats.loaded += value.byteLength;
            stats.chunkCount++;
            if (remaining < 0) throw terminal(budgetError());
            if (stats.loaded > limit) throw terminal("HLS resource exceeds its byte limit.");
            chunks.push(value);
          }
          if (!current()) return;
          const bytes = new Uint8Array(stats.loaded);
          let offset = 0;
          for (const chunk of chunks) { bytes.set(chunk, offset); offset += chunk.byteLength; }
          stats.total = stats.loaded;
          stats.loading.end = performance.now();
          let data = playlist ? new TextDecoder().decode(bytes) : bytes.buffer;
          if (playlist) {
            try { policy.validatePlaylist(data, { ...context, url }); }
            catch (error) { throw terminal(error); }
            if (admission) data = await admission.transformPlaylist(data, { ...context, url },
              (context, preflightSignal) => readPreflight(context, AbortSignal.any([controller.signal, preflightSignal])), controller.signal);
          }
          if (!current()) return;
          this.cleanup();
          callbacks.onProgress?.(stats, context, data, response);
          if (current()) { this.done = true; callbacks.onSuccess({ url, data, code: response.status }, stats, context, response); }
        } catch (error) {
          if (!current()) return;
          // Required audio and video share one failure boundary. HLS gap
          // recovery must never turn a missing audio segment into silent video.
          terminal(timedOut ? new Error("HLS resource timed out.") : error);
          if (!current()) return;
          this.cleanup();
          this.done = true;
          controller.abort();
          if (timedOut) callbacks.onTimeout(stats, context, this.response);
          else callbacks.onError({ code: this.response?.status ?? 0, text: error.message }, context, this.response, stats);
        } finally {
          if (reader) { await reader.cancel().catch(() => {}); reader.releaseLock(); }
          if (current()) this.cleanup();
        }
      })();
    }
    abort() {
      if (!this.controller) return;
      const notify = !this.stats.aborted && !this.done;
      if (notify) this.stats.aborted = true;
      this.generation++;
      this.controller.abort();
      this.cleanup();
      this.controller = null;
      if (notify) this.callbacks?.onAbort?.(this.stats, this.context, this.response);
    }
    destroy() { this.abort(); this.callbacks = null; this.context = null; this.response = null; }
    getCacheAge() { return null; }
    getResponseHeader(name) { return this.response?.headers.get(name) ?? null; }
  };
}
