import assert from "node:assert/strict";
import test from "node:test";
import { createTimelineHlsUhdAdmission, observeTimelineHlsDisplay } from "../internal/httpapi/web/timeline-hls-uhd.js";
import { createTimelineHlsLoader } from "../internal/httpapi/web/timeline-hls-loader.js";

const base = "https://media.example/fixture/";
const settings = 'BANDWIDTH=35000000,RESOLUTION=3840x2160,FRAME-RATE=30,VIDEO-RANGE=SDR,CODECS="avc1.640033,mp4a.40.2",AUDIO="shared"';
const master = (fields = settings) => ['#EXTM3U', '#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="shared",URI="audio.m3u8"', '#EXT-X-STREAM-INF:BANDWIDTH=100000,RESOLUTION=640x360,CODECS="avc1.42001e,mp4a.40.2",AUDIO="shared"', 'hd.m3u8', '#EXT-X-STREAM-INF:' + fields, 'uhd.m3u8'].join('\n');
const playlist = (name, durations = [5, 5, 5, 5]) => ['#EXTM3U', `#EXT-X-MAP:URI="${name}-init.mp4"`, ...durations.flatMap((duration, index) => [`#EXTINF:${duration},`, `${name}-${index}.m4s`]), '#EXT-X-ENDLIST'].join('\n');
const context = { url: base + "master.m3u8", type: "manifest" };
const uhd = { url: base + "uhd-0.m4s", type: "media-fragment", frag: { type: "main", level: 1, sn: 0 } };
const supported = async () => ({ supported: true, smooth: true, powerEfficient: false });
function fixture(options = {}) {
  const controller = new AbortController();
  const levels = [{ url: [base + "hd.m3u8"], width: 640, height: 360 }, { url: [base + "uhd.m3u8"], width: 3840, height: 2160 }];
  const reads = [];
  const admission = createTimelineHlsUhdAdmission({ signal: controller.signal, getLevels: () => levels, decodingInfo: supported, ...options });
  const read = async c => { reads.push(c.url); return c.type === "audioTrack" ? playlist("audio", [5.01, 5.01, 5.01, 5.01]) : playlist("uhd"); };
  return { admission, controller, levels, reads, read };
}

test("UHD admission verifies metadata, the exact audio group and decoder suitability before media enlargement", async () => {
  const f = fixture();
  assert.equal(f.admission.resourceProfile(uhd), null);
  const text = await f.admission.transformPlaylist(master(), context, f.read);
  assert.match(text, /uhd\.m3u8/);
  assert.deepEqual(f.reads, [base + "uhd.m3u8", base + "audio.m3u8"]);
  assert.ok(f.admission.resourceProfile(uhd));
  for (const request of [
    { ...uhd, url: base + "audio-0.m4s", frag: { type: "audio", level: 1, sn: 0 } },
    { ...uhd, frag: { type: "main", level: 0, sn: 0 } },
    { ...uhd, url: base + "uhd-init.mp4", frag: { type: "main", level: 1, sn: "initSegment" } },
    { ...uhd, url: base + "uhd-0.m4s?different=1" },
  ]) assert.equal(f.admission.resourceProfile(request), null);
});

for (const [name, fields, decodingInfo, video, audio] of [
  ["unknown frame rate", settings.replace(',FRAME-RATE=30', '')],
  ["4K60", settings.replace('FRAME-RATE=30', 'FRAME-RATE=60')],
  ["unknown SDR", settings.replace(',VIDEO-RANGE=SDR', '')],
  ["HDR", settings.replace('VIDEO-RANGE=SDR', 'VIDEO-RANGE=PQ')],
  ["peak bitrate above bound despite low average", settings.replace('BANDWIDTH=35000000', 'BANDWIDTH=35000001,AVERAGE-BANDWIDTH=10000000')],
  ["malformed peak bitrate", settings.replace('BANDWIDTH=35000000', 'BANDWIDTH=35e6')],
  ["DCI dimensions", settings.replace('3840x2160', '4096x2160')],
  ["unqualified codec", settings.replace('avc1.640033', 'hvc1.1.6.L153.B0')],
  ["unsupported decoder", settings, async () => ({ supported: false, smooth: true })],
  ["unsmooth decoder", settings, async () => ({ supported: true, smooth: false })],
  ["unknown decoder", settings, async () => undefined],
  ["video duration above 20 seconds", settings, supported, playlist("uhd", [5, 5, 5, 5.01])],
  ["video segment above six seconds", settings, supported, playlist("uhd", [6.01, 5])],
  ["audio duration outside AAC tolerance", settings, supported, undefined, playlist("audio", [5, 5, 5, 5.051])],
]) {
  test(`ineligible ${name} is excluded while the valid HD variant survives`, async () => {
    const f = fixture({ decodingInfo: decodingInfo ?? supported });
    const text = await f.admission.transformPlaylist(master(fields), context, async c => c.type === "audioTrack" ? audio ?? playlist("audio") : video ?? playlist("uhd"));
    assert.match(text, /hd\.m3u8/);
    assert.doesNotMatch(text, /\nuhd\.m3u8/);
    assert.equal(f.admission.resourceProfile(uhd), null);
  });
}

test("a lower declaration sharing a qualified video playlist cannot upgrade its requests", async () => {
  const f = fixture();
  await f.admission.transformPlaylist(master(), context, f.read);
  f.levels[0].url = f.levels[1].url;
  assert.equal(f.admission.resourceProfile({ ...uhd, frag: { ...uhd.frag, level: 0 } }), null);
});

test("a UHD-only master uses native fallback instead of opening small-display Auto to UHD", async () => {
  const f = fixture();
  const text = master().replace(/#EXT-X-STREAM-INF:BANDWIDTH=100000[^\n]+\nhd\.m3u8\n/, '');
  await assert.rejects(f.admission.transformPlaylist(text, context, f.read), /No supported bounded/);
  assert.deepEqual(f.reads, []);
  assert.equal(f.admission.resourceProfile(uhd), null);
});

test("preflight deduplicates shared audio and stops after four candidates", async () => {
  const f = fixture();
  const text = master() + Array.from({ length: 5 }, (_, i) => `\n#EXT-X-STREAM-INF:${settings}\nuhd-${i}.m3u8`).join('');
  const filtered = await f.admission.transformPlaylist(text, context, f.read);
  assert.equal(f.reads.filter(url => url.endsWith('audio.m3u8')).length, 1);
  assert.equal(f.reads.length, 5);
  assert.equal(filtered.split('\n').filter(line => line.startsWith('#EXT-X-STREAM-INF:')).length, 5);
});

test("preflight preserves policy context when audio and video share a playlist URL", async () => {
  const f = fixture();
  const reads = [];
  const text = master().replace('URI="audio.m3u8"', 'URI="uhd.m3u8"');
  await f.admission.transformPlaylist(text, context, async c => { reads.push(c.type); return playlist('uhd'); });
  assert.deepEqual(reads, ['level', 'audioTrack']);
});

test("preflight deadline excludes UHD without stalling valid HD or accepting late decoder results", async () => {
  let release;
  const f = fixture({ decodingInfo: () => new Promise(resolve => { release = resolve; }), preflightMilliseconds: 5 });
  const keepAlive = setTimeout(() => {}, 100);
  try {
    const text = await f.admission.transformPlaylist(master(), context, f.read);
    assert.doesNotMatch(text, /\nuhd\.m3u8/);
    release({ supported: true, smooth: true });
    await Promise.resolve();
    assert.deepEqual(f.reads, []);
    assert.equal(f.admission.resourceProfile(uhd), null);
  } finally { clearTimeout(keepAlive); }
});

test("cancellation during preflight prevents later audio reads and pilot admission", async () => {
  const f = fixture();
  const pending = f.admission.transformPlaylist(master(), context, async c => {
    f.reads.push(c.url);
    f.controller.abort();
    return playlist("uhd");
  });
  await assert.rejects(pending, { name: "AbortError" });
  assert.deepEqual(f.reads, [base + "uhd.m3u8"]);
  assert.throws(() => f.admission.resourceProfile(uhd), { name: "AbortError" });
});

test("a changed qualified playlist is rejected rather than reusing stale large-resource eligibility", async () => {
  const f = fixture();
  await f.admission.transformPlaylist(master(), context, f.read);
  await assert.rejects(f.admission.transformPlaylist(playlist("uhd", [6, 6, 6, 6]), { url: base + "uhd.m3u8", type: "level" }, f.read), /changed/);
});

const policy = { authorizeRequest: c => c.url, validatePlaylist() {} };
function load(Loader, context) {
  const loader = new Loader();
  const finished = new Promise(resolve => loader.load(context, {}, { onSuccess: response => resolve({ response }), onError: error => resolve({ error }), onTimeout: () => resolve({ timeout: true }), onAbort: () => resolve({ aborted: true }) }));
  return { loader, finished };
}

test("HD using shared AAC stays on its normal cumulative budget, including all preflight reads", async t => {
  const f = fixture();
  const bodies = new Map([[context.url, master()], [base + "uhd.m3u8", playlist("uhd")], [base + "audio.m3u8", playlist("audio")]]);
  const spent = [...bodies.values()].reduce((sum, text) => sum + new TextEncoder().encode(text).length, 0);
  let activated = 0;
  t.mock.method(globalThis, 'fetch', async url => new Response(bodies.get(url) ?? new Uint8Array(11)));
  const Loader = createTimelineHlsLoader({ policy, admission: f.admission, limits: { playlist: 65536, resource: 12 * 1024 * 1024, playback: spent + 10, timeout: 15000 }, onPilotActivated: () => activated++ });
  assert.ok((await load(Loader, context).finished).response);
  const audio = load(Loader, { ...uhd, url: base + "audio-0.m4s", frag: { type: 'audio', level: 0, sn: 0 } });
  assert.match((await audio.finished).error.text, /byte limit/);
  assert.equal(activated, 0);
});

test("only a qualified UHD video fragment gets 32 MiB and a single cumulative upgrade", async t => {
  const f = fixture();
  await f.admission.transformPlaylist(master(), context, f.read);
  let activated = 0;
  t.mock.method(globalThis, 'fetch', async () => new Response(new Uint8Array(13 * 1024 * 1024)));
  const Loader = createTimelineHlsLoader({ policy, admission: f.admission, onPilotActivated: () => activated++ });
  assert.ok((await load(Loader, uhd).finished).response);
  assert.ok((await load(Loader, uhd).finished).response);
  assert.equal(activated, 1);
});

test("the UHD resource boundary accepts exactly32MiB and rejects the next streamed byte", async t => {
  const f = fixture();
  await f.admission.transformPlaylist(master(), context, f.read);
  let size = 32 * 1024 * 1024;
  t.mock.method(globalThis, 'fetch', async () => {
    let offset = 0;
    return new Response(new ReadableStream({ pull(controller) {
      if (offset === size) { controller.close(); return; }
      const length = Math.min(65536, size - offset); offset += length;
      controller.enqueue(new Uint8Array(length));
    } }));
  });
  const Loader = createTimelineHlsLoader({ policy, admission: f.admission });
  assert.equal((await load(Loader, uhd).finished).response.data.byteLength, size);
  size++;
  assert.match((await load(Loader, uhd).finished).error.text, /byte limit/);
});

test("source validation rejects the raw master before UHD preflight reads", async t => {
  const f = fixture(); let requests = 0;
  t.mock.method(globalThis, 'fetch', async () => { requests++; return new Response(master()); });
  const Loader = createTimelineHlsLoader({ policy: { ...policy, validatePlaylist() { throw Error('Source denied master'); } }, admission: f.admission });
  assert.match((await load(Loader, context).finished).error.text, /Source denied/);
  assert.equal(requests, 1); assert.equal(f.admission.resourceProfile(uhd), null);
});

test("destroy aborts a network preflight and never admits a late response or starts audio", async t => {
  const f = fixture(); const urls = []; let release, childSignal;
  t.mock.method(globalThis, 'fetch', async (url, init) => {
    urls.push(url);
    if (url === context.url) return new Response(master());
    childSignal = init.signal;
    return new Promise(resolve => { release = resolve; });
  });
  const Loader = createTimelineHlsLoader({ policy, admission: f.admission, signal: f.controller.signal });
  const request = load(Loader, context);
  while (!release) await new Promise(resolve => setImmediate(resolve));
  request.loader.destroy();
  assert.equal((await request.finished).aborted, true);
  assert.equal(childSignal.aborted, true);
  release(new Response(playlist('uhd')));
  await new Promise(resolve => setImmediate(resolve));
  assert.equal(f.admission.resourceProfile(uhd), null);
  assert.deepEqual(urls, [context.url, base + 'uhd.m3u8']);
});

test("audio preflight failure excludes UHD while valid HD still loads", async t => {
  const f = fixture();
  t.mock.method(globalThis, 'fetch', async url => url === context.url ? new Response(master()) :
    url.endsWith('uhd.m3u8') ? new Response(playlist('uhd')) : new Response('missing audio', { status: 404 }));
  const Loader = createTimelineHlsLoader({ policy, admission: f.admission });
  const result = await load(Loader, context).finished;
  assert.match(result.response.data, /hd\.m3u8/);
  assert.doesNotMatch(result.response.data, /\nuhd\.m3u8/);
  assert.equal(f.admission.resourceProfile(uhd), null);
});

test("an oversized candidate header excludes UHD without reading its body or exhausting valid HD", async t => {
  const f = fixture(); let bodyReads = 0;
  t.mock.method(globalThis, 'fetch', async url => url === context.url ? new Response(master()) : {
    ok: true, headers: new Headers({ 'Content-Length': String(100 * 1024 * 1024) }),
    body: { getReader() { bodyReads++; throw Error('Oversized candidate body must not be read'); } },
  });
  const Loader = createTimelineHlsLoader({ policy, admission: f.admission });
  const result = await load(Loader, context).finished;
  assert.match(result.response.data, /hd\.m3u8/);
  assert.doesNotMatch(result.response.data, /\nuhd\.m3u8/);
  assert.equal(bodyReads, 0);
  assert.equal(f.admission.resourceProfile(uhd), null);
});

test("preflight cannot hide exhaustion of the normal session quota by excluding its candidate", async t => {
  const f = fixture(); let activations = 0;
  t.mock.method(globalThis, 'fetch', async url => new Response(url === context.url ? master() : playlist('uhd')));
  const Loader = createTimelineHlsLoader({ policy, admission: f.admission, onPilotActivated: () => activations++,
    limits: { playlist: 65536, resource: 12 * 1024 * 1024, timeout: 15000, playback: new TextEncoder().encode(master() + playlist('uhd')).length - 1 } });
  assert.match((await load(Loader, context).finished).error.text, /byte limit/);
  assert.equal(activations, 0);
});

test("the pilot upgrade preserves previous consumption and cannot reset its session allowance", async t => {
  const admission = { resourceProfile: c => c.url.endsWith('pilot') ? { resource: 12, playback: 20 } : null };
  t.mock.method(globalThis, 'fetch', async () => new Response(new Uint8Array(6)));
  const Loader = createTimelineHlsLoader({ policy, admission, limits: { playlist: 64, resource: 8, playback: 10, timeout: 15000 } });
  assert.ok((await load(Loader, { url: base + 'normal', type: 'media-fragment' }).finished).response);
  assert.ok((await load(Loader, { url: base + 'pilot', type: 'media-fragment' }).finished).response);
  assert.ok((await load(Loader, { url: base + 'pilot', type: 'media-fragment' }).finished).response);
  assert.match((await load(Loader, { url: base + 'pilot', type: 'media-fragment' }).finished).error.text, /byte limit/);
});

test("display capping follows fractional DPR, resize/fullscreen and stops every late callback after disposal", () => {
  const events = new EventTarget(), documentEvents = new EventTarget();
  let ratio = 0.5, rect = { width: 1280, height: 720 }, resize, dprCallback;
  let disconnected = false, queries = 0;
  class Observer { constructor(callback) { resize = callback; } observe() {} disconnect() { disconnected = true; } }
  events.matchMedia = () => { queries++; return { addEventListener(_name, callback) { dprCallback = callback; }, removeEventListener() {} }; };
  const caps = [];
  const levels = [{ width: 640, height: 360 }, { width: 1280, height: 720 }, { width: 3840, height: 2160 }];
  const display = observeTimelineHlsDisplay({ getBoundingClientRect: () => rect }, () => levels, cap => caps.push(cap), { observerClass: Observer, eventTarget: events, documentTarget: documentEvents, pixelRatio: () => ratio });
  display.update(); assert.equal(caps.at(-1), 0);
  ratio = 2; dprCallback(); assert.equal(caps.at(-1), 1);
  rect = { width: 320, height: 180 }; events.dispatchEvent(new Event('resize')); assert.equal(caps.at(-1), 0);
  rect = { width: 1920, height: 1080 }; documentEvents.dispatchEvent(new Event('fullscreenchange')); assert.equal(caps.at(-1), 2);
  display.dispose(); const count = caps.length, queryCount = queries;
  resize(); dprCallback(); events.dispatchEvent(new Event('resize')); display.update();
  assert.equal(caps.length, count); assert.equal(queries, queryCount); assert.equal(disconnected, true);
});
