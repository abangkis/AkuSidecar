import { evaluateBridgeCompatibility } from "./bridge-compatibility.mjs";

const SOURCES = ["x", "linkedin"];
const HEARTBEAT_FRESH_MS = 90_000;

export function createBridgeDiagnostics({ now = () => Date.now() } = {}) {
  let heartbeat = null;

  return {
    recordHeartbeat(input) {
      heartbeat = sanitizeHeartbeat(input, new Date(now()).toISOString());
      return heartbeat;
    },
    report(runs = []) {
      return buildBridgeHealth({ heartbeat, runs, now: now() });
    },
    compatibility() {
      return evaluateBridgeCompatibility(heartbeat);
    },
  };
}

export function sanitizeHeartbeat(input, receivedAt = new Date().toISOString()) {
  const capabilities = input?.capabilities ?? input ?? {};
  return {
    receivedAt,
    bridgeId: clean(capabilities.bridgeId, 80),
    extensionVersion: clean(capabilities.extensionVersion, 40),
    runtimeRevision: clean(capabilities.runtimeRevision, 100),
    buildId: clean(capabilities.buildId, 160),
    adapterVersions: boundedStringMap(capabilities.adapterVersions, 10, 100),
    contractVersion: clean(capabilities.contractVersion, 100),
    manifestVersion: integer(capabilities.manifestVersion),
    sources: boundedStrings(capabilities.sources, 10, 30),
    actions: boundedStrings(capabilities.actions, 30, 100),
    authority: clean(capabilities.authority, 80),
    captureLimits: {
      maxScrolls: integer(capabilities.captureLimits?.maxScrolls),
      maxSnapshots: integer(capabilities.captureLimits?.maxSnapshots),
      maxBlocksPerSnapshot: integer(capabilities.captureLimits?.maxBlocksPerSnapshot),
    },
  };
}

export function buildBridgeHealth({ heartbeat, runs = [], now = Date.now() }) {
  const ageMs = heartbeat ? Math.max(0, now - Date.parse(heartbeat.receivedAt)) : null;
  const runtime = !heartbeat
    ? { status: "unavailable", ageMs: null, heartbeat: null }
    : {
        status: ageMs <= HEARTBEAT_FRESH_MS ? "healthy" : "stale",
        ageMs,
        heartbeat,
      };
  const sources = Object.fromEntries(SOURCES.map((source) => [source, latestSourceHealth(source, runs)]));
  const observed = Object.values(sources).filter((source) => source.status !== "unobserved");
  const degradedSources = observed.filter((source) => source.status !== "healthy").length;
  const compatibility = evaluateBridgeCompatibility(heartbeat);
  const status = runtime.status === "unavailable"
    ? "unavailable"
    : !compatibility.compatible
      ? "incompatible"
    : runtime.status === "stale" || degradedSources > 0
      ? "degraded"
      : "healthy";
  return {
    version: 1,
    status,
    checkedAt: new Date(now).toISOString(),
    runtime,
    compatibility,
    sources,
    summary: {
      observedSources: observed.length,
      degradedSources,
      restorationFailures: observed.filter((source) => source.restoration?.restored === false).length,
    },
  };
}

function latestSourceHealth(source, runs) {
  const run = runs.find((entry) => (entry?.source ?? entry?.request?.source) === source && entry.observations?.length);
  if (!run) return { status: "unobserved", lastObservedAt: null };
  const storedObservation = run.observations.at(-1);
  const observation = storedObservation?.payload ?? storedObservation;
  const coverage = observation?.coverage ?? {};
  const adapter = coverage.adapterHealth ?? {};
  const status = adapter.state === "healthy" && coverage.restored !== false
    ? "healthy"
    : adapter.state || coverage.restored === false
      ? "degraded"
      : "unknown";
  return {
    status,
    lastObservedAt: clean(observation.capturedAt ?? coverage.checkedThrough, 50),
    adapterVersion: clean(coverage.adapterVersion, 100),
    adapterHealth: {
      state: clean(adapter.state, 30),
      strategies: boundedStrings(adapter.strategies, 20, 160),
      selectorCounts: boundedNumberMap(adapter.selectorCounts, 30),
      fieldCoverage: sanitizeFieldCoverage(adapter.fieldCoverage),
      domSignature: clean(adapter.domSignature, 200),
    },
    frontier: {
      newCandidateCount: integer(coverage.frontier?.newCandidateCount),
      hasMoreCandidateSignal: coverage.frontier?.hasMoreCandidateSignal === true,
    },
    sourceEvents: countEvents(coverage.sourceEvents),
    restoration: {
      attempted: coverage.restoreAttempted === true,
      restored: coverage.restored === true ? true : coverage.restored === false ? false : null,
      scope: clean(coverage.restorationScope, 50),
    },
    lifecycle: {
      ownership: clean(coverage.sourceTabOwnership, 30),
      opened: coverage.sourceTabOpened === true,
      recoveryCount: integer(coverage.sourceTabRecoveryCount),
      readinessState: clean(coverage.sourceReadinessState, 50),
    },
  };
}

function clean(value, max) {
  return typeof value === "string" ? value.trim().slice(0, max) || null : null;
}

function integer(value) {
  return Number.isFinite(value) ? Math.max(0, Math.trunc(value)) : null;
}

function boundedStrings(value, limit, max) {
  return Array.isArray(value)
    ? [...new Set(value.map((entry) => clean(entry, max)).filter(Boolean))].slice(0, limit)
    : [];
}

function boundedNumberMap(value, limit) {
  if (!value || typeof value !== "object" || Array.isArray(value)) return {};
  return Object.fromEntries(Object.entries(value).slice(0, limit).map(([key, count]) => [clean(key, 160), integer(count)]).filter(([key]) => key));
}

function boundedStringMap(value, limit, max) {
  if (!value || typeof value !== "object" || Array.isArray(value)) return {};
  return Object.fromEntries(Object.entries(value).slice(0, limit)
    .map(([key, entry]) => [clean(key, 80), clean(entry, max)])
    .filter(([key, entry]) => key && entry));
}

function sanitizeFieldCoverage(value) {
  if (!value || typeof value !== "object" || Array.isArray(value)) return {};
  return Object.fromEntries(Object.entries(value).slice(0, 30).map(([key, counts]) => [
    clean(key, 80),
    { present: integer(counts?.present), total: integer(counts?.total) },
  ]).filter(([key]) => key));
}

function countEvents(events) {
  const counts = {};
  for (const event of Array.isArray(events) ? events.slice(0, 100) : []) {
    const type = clean(event?.type, 100);
    if (type) counts[type] = (counts[type] ?? 0) + 1;
  }
  return counts;
}
