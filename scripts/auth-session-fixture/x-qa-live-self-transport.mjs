// One fixed same-document HTML fetch. Native values never enter this projection.
export function qaLiveSelfCommand(method, params, expression) {
  const exact = value => JSON.stringify(params) === JSON.stringify(value);
  if (['Page.getFrameTree', 'Network.disable'].includes(method)) return exact({});
  if (method === 'Network.enable') return exact({ maxTotalBufferSize: 1000000, maxResourceBufferSize: 100000, maxPostDataSize: 0 });
  if (method === 'Runtime.evaluate') return exact({ expression, returnByValue: true, awaitPromise: true, userGesture: false });
  return false;
}
export class QALiveSelfTransport {
  #session; #frame; #loader; #request = null; #requests = 0; #response = false;
  #completed = false; #invalid = false; #documentLost = false; #events = 0; #cache = false;
  constructor(session, frame, loader) {
    if (![session, frame, loader].every(v => typeof v === 'string' && v.length > 0 && v.length <= 512)) throw new Error('qa_live_transport_invalid');
    this.#session = session; this.#frame = frame; this.#loader = loader;
  }
  observe(raw) {
    if (raw.method === 'Target.detachedFromTarget' && raw.params?.sessionId === this.#session) this.#documentLost = this.#invalid = true;
    if (raw.sessionId !== this.#session) return;
    if (++this.#events > 4096) { this.#invalid = true; return; }
    const p = raw.params ?? {};
    if (['Inspector.detached', 'Inspector.targetCrashed'].includes(raw.method) ||
        raw.method === 'Page.frameDetached' && p.frameId === this.#frame ||
        raw.method === 'Page.frameNavigated' && p.frame?.id === this.#frame ||
        raw.method === 'Page.navigatedWithinDocument' && p.frameId === this.#frame) this.#documentLost = this.#invalid = true;
    if (raw.method === 'Network.requestWillBeSent' && p.request?.url === 'https://x.com/home') {
      this.#requests = Math.min(2, this.#requests + 1);
      if (this.#requests !== 1 || p.request.method !== 'GET' || p.type !== 'Fetch' ||
          p.frameId !== this.#frame || p.loaderId !== this.#loader || p.redirectResponse ||
          typeof p.requestId !== 'string' || !p.requestId || p.requestId.length > 512) { this.#invalid = true; return; }
      this.#request = p.requestId;
    }
    if (!this.#request || p.requestId !== this.#request) return;
    if (raw.method === 'Network.requestServedFromCache') this.#cache = true;
    if (raw.method === 'Network.loadingFailed') this.#invalid = true;
    if (raw.method === 'Network.responseReceived') {
      const r = p.response;
      if (this.#response || !r || r.url !== 'https://x.com/home' || r.status !== 200 ||
          p.frameId !== this.#frame || p.loaderId !== this.#loader || p.type !== 'Fetch' ||
          r.fromDiskCache !== false || r.fromServiceWorker !== false || r.fromPrefetchCache !== false ||
          typeof r.mimeType !== 'string' || r.mimeType.toLowerCase() !== 'text/html') this.#invalid = true;
      this.#response = true;
    }
    if (raw.method === 'Network.loadingFinished') {
      if (this.#completed || !this.#response) this.#invalid = true;
      this.#completed = true;
    }
  }
  finish(frame, loader) {
    const preserved = frame === this.#frame && loader === this.#loader;
    return Object.freeze({ requests: this.#requests, completed: this.#completed,
      documentPreserved: preserved && !this.#documentLost, responseFresh: this.#requests === 1 &&
        this.#response && this.#completed && !this.#invalid && !this.#cache && preserved });
  }
}

export function finishQALiveSelf(observation, transport, elapsedMS) {
  const timely = Number.isFinite(elapsedMS) && elapsedMS >= 0 && elapsedMS < 2000;
  const identity = observation?.identity === 'candidate' && transport.responseFresh && timely;
  const outcome = ['body_limit', 'initializer_limit', 'parser_rejected'].includes(observation?.outcome) ? observation.outcome :
    !transport.documentPreserved ? 'source_unqualified' : !timely || observation?.outcome === 'timeout' ? 'deadline' : identity ? 'fresh_identity_observed' : 'identity_unavailable';
  const report = { schema: 'aku.x.qa-live-self-run.v2', source: 'home_document_fetch.v2',
    outcome, observation, transport, elapsedMS: Number.isFinite(elapsedMS) && elapsedMS >= 0 ? Math.min(60000, elapsedMS) : 60000,
    canAuthorize: false, nativeQualified: false, sessionTransferred: false };
  if (Buffer.byteLength(JSON.stringify(report), 'utf8') > 4096) throw new Error('qa_live_projection_limit');
  return Object.freeze(report);
}
