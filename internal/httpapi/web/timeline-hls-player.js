import Hls from "./vendor/hls.mjs";
import { createTimelineHlsLoader } from "./timeline-hls-loader.js";
import { createTimelineHlsUhdAdmission, observeTimelineHlsDisplay } from "./timeline-hls-uhd.js";

// The engine factory is a test seam, not a source adapter or a network escape.
export function createTimelineHlsPlayer(HlsEngine) {
  return function attachTimelineHls(video, { url, policy, onState = () => {}, onError = () => {} }) {
    if (!HlsEngine.isSupported()) throw new Error("HLS playback is not supported.");
    policy.authorizeRequest({ url, type: "manifest" });
    const controller = new AbortController();
    let hls;
    let disposed = false;
    let settled = false;
    let resolveReady, rejectReady;
    const ready = new Promise((resolve, reject) => { resolveReady = resolve; rejectReady = reject; });
    void ready.catch(() => {});
    let qualities = [];
    let selected = "auto";
    let active = null;
    let display;
    function state() { return { qualities: qualities.map(q => ({ ...q })), selected, active, pending: selected !== "auto" && selected !== active }; }
    function emit() { if (!disposed) onState(state()); }
    function dispose() {
      if (disposed) return;
      disposed = true;
      controller.abort();
      display?.dispose();
      hls?.destroy();
      if (!settled) { settled = true; rejectReady(new DOMException("HLS playback disposed.", "AbortError")); }
    }
    function fail(error) {
      if (disposed) return;
      if (!settled) { settled = true; rejectReady(error); }
      dispose();
      onError(error);
    }
    const admission = createTimelineHlsUhdAdmission({ signal: controller.signal, getLevels: () => hls?.levels ?? [] });
    const Loader = createTimelineHlsLoader({ policy, signal: controller.signal, onTerminalError: fail, admission,
      onPilotActivated: () => { if (hls) hls.config.backBufferLength = 6; } });
    const loadPolicy = { default: { maxTimeToFirstByteMs: 15_000, maxLoadTimeMs: 15_000, timeoutRetry: null, errorRetry: null } };
    try {
      hls = new HlsEngine({
        loader: Loader, enableWorker: false, progressive: false, lowLatencyMode: false,
        startLevel: 0, maxBufferLength: 6, maxMaxBufferLength: 6, backBufferLength: 10,
        maxBufferSize: 8 * 1024 * 1024, preserveManualLevelOnError: true,
        capLevelToPlayerSize: false, capLevelOnFPSDrop: false,
        enableSoftwareAES: false, emeEnabled: false, enableInterstitialPlayback: false,
        enableWebVTT: false, enableIMSC1: false, enableCEA708Captions: false,
        subtitleStreamController: undefined, subtitleTrackController: undefined,
        contentSteeringController: undefined, interstitialsController: undefined,
        manifestLoadPolicy: loadPolicy, playlistLoadPolicy: loadPolicy, fragLoadPolicy: loadPolicy,
      });
      hls.on(HlsEngine.Events.MANIFEST_PARSED, () => {
        if (disposed) return;
        if (hls.levels.some(level => level.width >= 3840 || level.height >= 2160) &&
            !hls.levels.some(level => level.width > 0 && level.height > 0 && level.width < 3840 && level.height < 2160)) {
          fail(new Error("UHD pilot requires a supported lower-quality fallback."));
          return;
        }
        qualities = hls.levels.map((level, index) => ({ id: String(index), width: level.width ?? 0, height: level.height ?? 0,
          bitrate: level.bitrate ?? 0, label: `${level.height ? `${level.height}p` : "Video"} · ${Math.round((level.bitrate ?? 0) / 1000)} kbps` }));
        display?.update();
        emit();
        if (!settled) { settled = true; resolveReady(state()); }
      });
      hls.on(HlsEngine.Events.LEVEL_SWITCHED, (_event, data) => {
        if (disposed) return;
        active = qualities[data.level]?.id ?? null;
        emit();
      });
      hls.on(HlsEngine.Events.ERROR, (_event, data) => {
        if (data.fatal) fail(new Error(`HLS playback failed: ${data.details || data.type || "unknown error"}.`));
      });
      hls.attachMedia(video);
      display = observeTimelineHlsDisplay(video, () => hls.levels, cap => { if (!disposed) hls.autoLevelCapping = cap; });
      hls.loadSource(url);
    } catch (error) { fail(error); }
    return {
      ready,
      setQuality(id) {
        if (disposed) return false;
        if (id !== "auto" && !qualities.some(q => q.id === id)) return false;
        selected = id;
        // Conservative switching preserves buffered audio/video until the new
        // quality is loaded. The eager nextLevel flush can cut through an
        // alternate-audio segment and leave a hole during an audio-group switch.
        hls.loadLevel = id === "auto" ? -1 : Number(id);
        emit();
        return true;
      },
      dispose,
    };
  };
}

export const attachTimelineHls = createTimelineHlsPlayer(Hls);
