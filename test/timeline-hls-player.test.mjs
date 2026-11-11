import assert from "node:assert/strict";
import test from "node:test";
import { createHash } from "node:crypto";
import { readFile } from "node:fs/promises";
import Hls from "../internal/httpapi/web/vendor/hls.mjs";
import { createTimelineHlsPlayer } from "../internal/httpapi/web/timeline-hls-player.js";

test("vendored full HLS engine retains the reviewed official version and checksum", async () => {
  assert.equal(Hls.version, "1.7.4");
  const bytes = await readFile(new URL("../internal/httpapi/web/vendor/hls.mjs", import.meta.url));
  assert.equal(createHash("sha256").update(bytes).digest("hex"), "ddb66e85c32e3222c2ab479fe1d9f4d5b47ba0df18454f2b7ac5ada3ad7817b6");
});

function fixture() {
  let engine;
  class Engine {
    static isSupported() { return true; }
    static Events = { MANIFEST_PARSED: "manifest", LEVEL_SWITCHED: "level", ERROR: "error" };
    constructor(config) { engine = this; this.config = config; this.listeners = new Map(); this.levels = [{ height: 360, width: 640, bitrate: 100000 }, { height: 720, width: 1280, bitrate: 800000 }]; this.destroyCount = 0; }
    on(event, listener) { this.listeners.set(event, listener); }
    emit(event, value) { this.listeners.get(event)?.(event, value); }
    attachMedia(video) { this.video = video; }
    loadSource(url) { this.url = url; }
    set nextLevel(_value) { throw new Error("Eager buffer flushing is not allowed for manual quality changes."); }
    set currentLevel(_value) { throw new Error("Immediate buffer flushing is not allowed for manual quality changes."); }
    destroy() { this.destroyCount++; }
  }
  const states = [], errors = [];
  const video = { currentTime: 18, paused: false };
  const session = createTimelineHlsPlayer(Engine)(video, { url: "https://example.org/master.m3u8", policy: { authorizeRequest: context => context.url, validatePlaylist() {} }, onState: state => states.push(state), onError: error => errors.push(error) });
  return { engine, session, states, errors, video };
}

test("manual selection persists until Auto without resetting playback, and active quality follows the engine", async () => {
  const { engine, session, states, video } = fixture();
  engine.emit("manifest");
  assert.equal((await session.ready).qualities.length, 2);
  assert.equal(session.setQuality("1"), true); assert.equal(engine.loadLevel, 1);
  assert.equal(states.at(-1).selected, "1"); assert.equal(states.at(-1).active, null); assert.equal(states.at(-1).pending, true);
  engine.emit("level", { level: 1 }); assert.equal(states.at(-1).pending, false);
  assert.equal(session.setQuality("auto"), true); assert.equal(engine.loadLevel, -1);
  assert.equal(states.at(-1).selected, "auto"); assert.equal(states.at(-1).active, "1");
  assert.equal(session.setQuality("200"), false); assert.equal(video.currentTime, 18); assert.equal(video.paused, false);
  assert.equal(engine.config.preserveManualLevelOnError, true); assert.equal(engine.config.enableWorker, false);
  session.dispose();
});

test("dispose rejects pending readiness, is idempotent and ignores stale events", async () => {
  const { engine, session, states, errors } = fixture();
  session.dispose(); session.dispose();
  await assert.rejects(session.ready, { name: "AbortError" });
  engine.emit("manifest"); engine.emit("level", { level: 1 }); engine.emit("error", { fatal: true });
  assert.equal(engine.destroyCount, 1); assert.deepEqual(states, []); assert.deepEqual(errors, []);
  assert.equal(session.setQuality("auto"), false);
});

test("fatal playback failure tears down before reporting and reports only once", async () => {
  const { engine, session, errors } = fixture();
  engine.emit("manifest"); await session.ready;
  engine.emit("error", { fatal: false }); assert.equal(engine.destroyCount, 0);
  engine.emit("error", { fatal: true, details: "decode" }); engine.emit("error", { fatal: true });
  assert.equal(engine.destroyCount, 1); assert.equal(errors.length, 1); assert.match(errors[0].message, /decode/);
});

test("engine codec filtering cannot leave a UHD-only Auto session after metadata admission", async () => {
  const { engine, session, errors, states } = fixture();
  engine.levels = [{ width: 3840, height: 2160, bitrate: 35000000 }];
  engine.emit("manifest");
  await assert.rejects(session.ready, /supported lower-quality fallback/);
  assert.equal(engine.destroyCount, 1);
  assert.equal(errors.length, 1);
  assert.deepEqual(states, []);
});
