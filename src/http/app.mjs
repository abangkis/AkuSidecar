import fs from "node:fs";
import http from "node:http";
import path from "node:path";
import { buildSourceRegistry, SOURCE_CATALOG } from "../core/source-registry.mjs";
import { URL } from "node:url";
import { ContractError } from "../core/contracts.mjs";
import { JobEngine } from "../core/job-engine.mjs";
import {
  applyPersistedConfiguration,
  configurationView,
  updateDashboardConfiguration,
} from "../configuration/runtime-configuration.mjs";
import { providerCapabilities } from "../reasoning/provider-capabilities.mjs";
import { inspectSqliteDatabase } from "../store/sqlite-operations.mjs";
import { createBridgeDiagnostics } from "../operations/bridge-diagnostics.mjs";
import { createBridgeActions } from "../operations/bridge-actions.mjs";
import { BRIDGE_REQUIREMENTS } from "../operations/bridge-compatibility.mjs";
import { getOnboardingProfile, saveOnboardingProfile } from "../core/onboarding-profile.mjs";
import { CalibrationEngine } from "../core/calibration-engine.mjs";

const MIME_TYPES = new Map([
  [".html", "text/html; charset=utf-8"],
  [".js", "text/javascript; charset=utf-8"],
  [".css", "text/css; charset=utf-8"],
  [".json", "application/json; charset=utf-8"],
  [".svg", "image/svg+xml"],
  [".png", "image/png"],
]);

export const BRIDGE_CONTRACT_VERSION = "aku-browser.bridge.v1";
export const APP_VERSION = "0.6.3";

export function createAkuBrowserApp({
  config,
  store,
  reasoningProvider,
  logger = console,
  enforceBridgeCompatibility = false,
}) {
  config.calibration ??= {
    enabled: true,
    triggerPolicy: "first_run",
    batchSize: 10,
    maxItemsPerSource: 5,
    liveInfluence: false,
  };
  config.preference ??= {
    enabled: true,
    maxRankDisplacement: 2,
    minimumScoreDelta: 0.03,
    automaticFitFeedbackDelta: 5,
  };
  applyPersistedConfiguration(config, store);
  const engine = new JobEngine({
    store,
    reasoningProvider,
    limits: config.limits,
    preferencePolicy: config.preference,
    logger,
  });
  const bridgeToken = store.getOrCreateBridgeToken();
  const calibrationEngine = new CalibrationEngine({
    store,
    maxItems: config.calibration.batchSize,
    maxItemsPerSource: config.calibration.maxItemsPerSource,
  });
  const bridgeDiagnostics = createBridgeDiagnostics();
  const bridgeActions = createBridgeActions({
    expectedBuildId: `aku-bridge-${BRIDGE_REQUIREMENTS.minimumExtensionVersion}-${BRIDGE_REQUIREMENTS.runtimeRevision}`,
  });
  let frontend = null;

  const server = http.createServer(async (request, response) => {
    try {
      applySecurityHeaders(response, {
        viteDevelopment: frontend?.name === "vite",
        host: config.host,
        port: config.port,
      });
      applyExtensionCors(request, response);
      if (request.method === "OPTIONS") {
        response.writeHead(204);
        response.end();
        return;
      }

      const url = new URL(request.url, `http://${config.host}:${config.port}`);
      if (url.pathname.startsWith("/api/")) {
        response.setHeader("Cache-Control", "no-store");
        await handleApi({
          request,
          response,
          url,
          engine,
          store,
          bridgeToken,
          bridgeDiagnostics,
          bridgeActions,
          config,
          calibrationEngine,
          enforceBridgeCompatibility,
        });
        return;
      }
      if (frontend) {
        serveFrontend(frontend.middleware, request, response, logger);
        return;
      }
      serveStatic(response, url.pathname, config.publicDirectory);
    } catch (error) {
      const status = error instanceof ContractError ? 400 : 500;
      logger.error?.("request failed", { path: request.url, error: error.message });
      sendJson(response, status, {
        error: error.name ?? "Error",
        message: error.message ?? "Unexpected server error",
        details: error instanceof ContractError ? error.details : undefined,
      });
    }
  });

  return {
    server,
    engine,
    calibrationEngine,
    setFrontend(nextFrontend) {
      if (server.listening) {
        throw new Error("The development frontend must be attached before the Sidecar starts.");
      }
      if (
        !nextFrontend ||
        typeof nextFrontend.middleware !== "function" ||
        typeof nextFrontend.close !== "function"
      ) {
        throw new TypeError("A frontend requires middleware and close functions.");
      }
      frontend = nextFrontend;
    },
    async start() {
      await new Promise((resolve, reject) => {
        server.once("error", reject);
        server.listen(config.port, config.host, () => {
          server.off("error", reject);
          resolve();
        });
      });
      return server.address();
    },
    async stop() {
      if (frontend) {
        const currentFrontend = frontend;
        frontend = null;
        await currentFrontend.close();
      }
      if (server.listening) {
        await new Promise((resolve, reject) =>
          server.close((error) => (error ? reject(error) : resolve())),
        );
      }
    },
  };
}

function serveFrontend(middleware, request, response, logger) {
  middleware(request, response, (error) => {
    if (error) {
      logger.error?.("Vite frontend request failed", {
        path: request.url,
        error: error.message,
      });
      if (response.headersSent) {
        response.destroy(error);
      } else {
        sendJson(response, 500, {
          error: error.name ?? "Error",
          message: error.message ?? "Vite frontend request failed",
        });
      }
      return;
    }
    if (!response.writableEnded) {
      sendJson(response, 404, { error: "NotFound", message: "Frontend route not found" });
    }
  });
}

async function handleApi({ request, response, url, engine, store, bridgeToken, bridgeDiagnostics, bridgeActions, config, calibrationEngine, enforceBridgeCompatibility }) {
  if (request.method === "GET" && url.pathname === "/api/calibration/active") {
    sendJson(response, 200, { calibration: calibrationEngine.getActive() });
    return;
  }

  if (request.method === "POST" && url.pathname === "/api/calibration/sessions") {
    const body = await readJson(request, config.limits.maxBodyBytes);
    sendJson(response, 201, {
      calibration: calibrationEngine.createFromUnifiedSession(body.unifiedSessionId, {
        triggerKind: body.triggerKind,
        maxItems: config.calibration.batchSize,
      }),
    });
    return;
  }

  const calibrationMatch = url.pathname.match(/^\/api\/calibration\/sessions\/([^/]+)$/);
  if (request.method === "GET" && calibrationMatch) {
    const calibration = calibrationEngine.get(decodeURIComponent(calibrationMatch[1]));
    sendJson(response, calibration ? 200 : 404, { calibration });
    return;
  }

  const calibrationDecisionMatch = url.pathname.match(
    /^\/api\/calibration\/sessions\/([^/]+)\/samples\/(\d+)$/,
  );
  if (request.method === "PUT" && calibrationDecisionMatch) {
    const body = await readJson(request, config.limits.maxBodyBytes);
    sendJson(response, 200, {
      calibration: calibrationEngine.decide(
        decodeURIComponent(calibrationDecisionMatch[1]),
        Number.parseInt(calibrationDecisionMatch[2], 10),
        body,
      ),
    });
    return;
  }
  if (request.method === "GET" && url.pathname === "/api/onboarding") {
    sendJson(response, 200, { onboarding: getOnboardingProfile(store) });
    return;
  }

  if (request.method === "PUT" && url.pathname === "/api/onboarding") {
    const body = await readJson(request, config.limits.maxBodyBytes);
    const firstCompletion = getOnboardingProfile(store).status !== "completed";
    const onboarding = saveOnboardingProfile(store, body);
    if (firstCompletion) {
      store.setSetting(
        "calibration.first_run_status",
        config.calibration.enabled ? "pending" : "disabled",
      );
    }
    updateDashboardConfiguration(config, store, {
      activeSources: onboarding.profile.activeSources,
    });
    sendJson(response, 200, { onboarding });
    return;
  }
  if (request.method === "GET" && url.pathname === "/api/configuration/runtime") {
    sendJson(response, 200, { configuration: configurationView(config, store) });
    return;
  }

  if (request.method === "PUT" && url.pathname === "/api/configuration/runtime") {
    const body = await readJson(request, config.limits.maxBodyBytes);
    updateDashboardConfiguration(config, store, body);
    sendJson(response, 200, { configuration: configurationView(config, store) });
    return;
  }

  if (request.method === "GET" && url.pathname === "/api/health") {
    sendJson(response, 200, {
      version: APP_VERSION,
      bridgeContractVersion: BRIDGE_CONTRACT_VERSION,
      status: "ok",
      provider: engine.reasoningProvider.name,
      providerCapabilities: providerCapabilities(config.reasoning?.provider),
      reasoning: {
        model: config.reasoning?.evaluationModel ?? config.reasoning?.model ?? null,
        planningModel: config.reasoning?.planningModel ?? config.reasoning?.model ?? null,
        evaluationModel: config.reasoning?.evaluationModel ?? config.reasoning?.model ?? null,
        planningEffort: config.reasoning?.planningEffort ?? null,
        evaluationEffort: config.reasoning?.evaluationEffort ?? null,
        planningPolicy: config.reasoning?.planningPolicy ?? null,
      },
      time: new Date().toISOString(),
    });
    return;
  }

  if (request.method === "GET" && url.pathname === "/api/operations/database/health") {
    const health = inspectSqliteDatabase(config.databasePath);
    sendJson(response, health.status === "healthy" ? 200 : 503, {
      database: {
        ...health,
        databasePath: path.basename(health.databasePath),
      },
    });
    return;
  }

  if (request.method === "POST" && url.pathname === "/api/operations/bridge/heartbeat") {
    const body = await readJson(request, config.limits.maxBodyBytes);
    const heartbeat = bridgeDiagnostics.recordHeartbeat(body);
    const cooperativeAction = bridgeActions.observeHeartbeat(heartbeat);
    sendJson(response, 202, {
      heartbeat,
      compatibility: bridgeDiagnostics.compatibility(),
      cooperativeAction,
    });
    return;
  }

  if (request.method === "GET" && url.pathname === "/api/operations/bridge/health") {
    const runs = engine.listRuns(30).map((run) => engine.getRun(run.id)).filter(Boolean);
    sendJson(response, 200, { bridge: bridgeDiagnostics.report(runs) });
    return;
  }

  if (request.method === "POST" && url.pathname === "/api/operations/bridge/actions/reload-self") {
    requireBridgeIdentity(request, store);
    const body = await readJson(request, config.limits.maxBodyBytes);
    let action;
    try {
      action = bridgeActions.requestReload(
        body,
        bridgeDiagnostics.report().runtime.heartbeat,
      );
    } catch (error) {
      throw new ContractError(error.message);
    }
    sendJson(response, 202, { action });
    return;
  }

  if (request.method === "GET" && url.pathname === "/api/operations/bridge/actions/next") {
    const waitMs = parseBridgeActionWait(url.searchParams.get("waitMs"));
    const waiter = new AbortController();
    response.once("close", () => waiter.abort());
    sendJson(response, 200, {
      action: await bridgeActions.waitForNext(waitMs, { signal: waiter.signal }),
    });
    return;
  }

  const bridgeActionMatch = url.pathname.match(
    /^\/api\/operations\/bridge\/actions\/([^/]+)$/,
  );
  if (request.method === "GET" && bridgeActionMatch) {
    requireBridgeIdentity(request, store);
    try {
      sendJson(response, 200, {
        action: bridgeActions.get(decodeURIComponent(bridgeActionMatch[1])),
      });
    } catch (error) {
      throw new ContractError(error.message);
    }
    return;
  }

  const bridgeActionAcceptMatch = url.pathname.match(
    /^\/api\/operations\/bridge\/actions\/([^/]+)\/accept$/,
  );
  if (request.method === "POST" && bridgeActionAcceptMatch) {
    requireBridgeIdentity(request, store);
    try {
      sendJson(response, 202, {
        action: bridgeActions.accept(decodeURIComponent(bridgeActionAcceptMatch[1])),
      });
    } catch (error) {
      throw new ContractError(error.message);
    }
    return;
  }

  if (request.method === "GET" && url.pathname === "/api/bootstrap") {
    sendJson(response, 200, {
      version: APP_VERSION,
      bridgeContractVersion: BRIDGE_CONTRACT_VERSION,
      provider: engine.reasoningProvider.name,
      providerCapabilities: providerCapabilities(config.reasoning?.provider),
      reasoning: {
        model: config.reasoning?.evaluationModel ?? config.reasoning?.model ?? null,
        planningModel: config.reasoning?.planningModel ?? config.reasoning?.model ?? null,
        evaluationModel: config.reasoning?.evaluationModel ?? config.reasoning?.model ?? null,
        planningEffort: config.reasoning?.planningEffort ?? null,
        evaluationEffort: config.reasoning?.evaluationEffort ?? null,
        planningPolicy: config.reasoning?.planningPolicy ?? null,
      },
      bridgeToken,
      onboarding: getOnboardingProfile(store),
      calibration: {
        firstRunStatus: store.getSetting("calibration.first_run_status") ?? "not_started",
        active: calibrationEngine.getActive(),
        enabled: config.calibration.enabled,
        triggerPolicy: config.calibration.triggerPolicy,
        batchSize: config.calibration.batchSize,
        liveInfluence: engine.getPreferenceRuntime().liveInfluence,
      },
      preferenceRuntime: engine.getPreferenceRuntime(),
      presentation: config.presentation,
      sourceRegistry: buildSourceRegistry(config.sources?.active ?? ["x", "linkedin"]),
      limits: config.limits,
      supportedModes: ["catch_up", "manual_live"],
      supportedSources: SOURCE_CATALOG.map((source) => source.id),
      unifiedSession: {
        sources: [...(config.sources?.active ?? ["x", "linkedin"])],
        maxItemsPerSource: config.limits.maxItems,
        maxItemsTotal: Math.min(
          10,
          config.limits.maxItems * (config.sources?.active?.length ?? 2),
        ),
        execution: "sequential",
      },
    });
    return;
  }

  if (request.method === "GET" && url.pathname === "/api/runs") {
    const limit = Math.max(1, Math.min(50, Number.parseInt(url.searchParams.get("limit") ?? "20", 10)));
    sendJson(response, 200, { runs: engine.listRuns(limit) });
    return;
  }

  if (request.method === "GET" && url.pathname === "/api/pilot/review") {
    const limit = Math.max(
      1,
      Math.min(10, Number.parseInt(url.searchParams.get("limit") ?? "10", 10)),
    );
    const offset = Math.max(
      0,
      Math.min(40, Number.parseInt(url.searchParams.get("offset") ?? "0", 10)),
    );
    sendJson(response, 200, {
      review: engine.getPilotReview({
        limit,
        offset,
        maxRuns: 50,
        source: url.searchParams.get("source") ?? "all",
        verdict: url.searchParams.get("verdict") ?? "all",
      }),
    });
    return;
  }

  if (request.method === "GET" && url.pathname === "/api/preferences/profile") {
    sendJson(response, 200, { profile: engine.getPreferenceProfile() });
    return;
  }

  if (request.method === "GET" && url.pathname === "/api/preferences/runtime") {
    sendJson(response, 200, { runtime: engine.getPreferenceRuntime() });
    return;
  }

  if (request.method === "POST" && url.pathname === "/api/preferences/runtime/refit") {
    sendJson(response, 200, { runtime: engine.refitPreferenceRuntime() });
    return;
  }

  if (request.method === "POST" && url.pathname === "/api/preferences/runtime/reset") {
    sendJson(response, 200, { runtime: engine.resetPreferenceRuntime() });
    return;
  }

  if (request.method === "GET" && url.pathname === "/api/preferences/replay") {
    sendJson(response, 200, { replay: engine.getPreferenceReplay() });
    return;
  }

  if (request.method === "GET" && url.pathname === "/api/preferences/benchmark") {
    sendJson(response, 200, { benchmark: engine.getEngineReplayBenchmark() });
    return;
  }

  if (request.method === "GET" && url.pathname === "/api/preferences/experiment") {
    sendJson(response, 200, { experiment: engine.getPreferenceExperiment() });
    return;
  }

  if (request.method === "POST" && url.pathname === "/api/preferences/experiment/fit") {
    sendJson(response, 200, { experiment: engine.fitPreferenceExperiment() });
    return;
  }

  if (request.method === "GET" && url.pathname === "/api/preferences/shadow-comparison") {
    const limit = boundedIntegerQuery(url, "limit", { fallback: 50, minimum: 1, maximum: 100 });
    const offset = boundedIntegerQuery(url, "offset", {
      fallback: 0,
      minimum: 0,
      maximum: Number.MAX_SAFE_INTEGER,
    });
    sendJson(response, 200, {
      comparison: engine.getPreferenceShadowComparison({ limit, offset }),
    });
    return;
  }

  if (request.method === "GET" && url.pathname === "/api/knowledge") {
    const source = url.searchParams.get("source") ?? "x";
    const mode = url.searchParams.get("mode") ?? "catch_up";
    const limit = Number.parseInt(url.searchParams.get("limit") ?? "20", 10);
    sendJson(response, 200, {
      knowledge: engine.getKnowledgeContext(source, mode, limit),
    });
    return;
  }

  const knowledgeEventMatch = url.pathname.match(/^\/api\/knowledge\/events\/([^/]+)$/);
  if (request.method === "GET" && knowledgeEventMatch) {
    const source = url.searchParams.get("source") ?? "x";
    const mode = url.searchParams.get("mode") ?? "catch_up";
    const limit = Number.parseInt(url.searchParams.get("limit") ?? "50", 10);
    sendJson(response, 200, {
      versions: engine.getKnowledgeEventHistory(
        source,
        mode,
        decodeURIComponent(knowledgeEventMatch[1]),
        limit,
      ),
    });
    return;
  }

  if (request.method === "POST" && url.pathname === "/api/runs") {
    if (enforceBridgeCompatibility) assertCompatibleBridge(bridgeDiagnostics);
    const body = await readJson(request, config.limits.maxBodyBytes);
    sendJson(response, 201, { run: engine.startRun(body) });
    return;
  }

  if (request.method === "POST" && url.pathname === "/api/sessions") {
    if (enforceBridgeCompatibility) assertCompatibleBridge(bridgeDiagnostics);
    const body = await readJson(request, config.limits.maxBodyBytes);
    sendJson(response, 201, {
      session: engine.startUnifiedSession({
        ...body,
        sources: body.sources ?? config.sources?.active ?? ["x", "linkedin"],
      }),
    });
    return;
  }

  if (request.method === "GET" && url.pathname === "/api/sessions") {
    const limit = boundedIntegerQuery(url, "limit", { fallback: 1, minimum: 1, maximum: 10 });
    const offset = boundedIntegerQuery(url, "offset", {
      fallback: 0,
      minimum: 0,
      maximum: Number.MAX_SAFE_INTEGER,
    });
    sendJson(response, 200, engine.getTimelineSessions({ limit, offset }));
    return;
  }

  if (request.method === "GET" && url.pathname === "/api/timeline") {
    const capacity = config.presentation?.timelineCapacity ?? 12;
    const limit = boundedIntegerQuery(url, "limit", {
      fallback: capacity,
      minimum: 1,
      maximum: Math.min(50, capacity),
    });
    const offset = boundedIntegerQuery(url, "offset", {
      fallback: 0,
      minimum: 0,
      maximum: Number.MAX_SAFE_INTEGER,
    });
    sendJson(response, 200, {
      timeline: engine.getTimelineFeed({ capacity, limit, offset }),
    });
    return;
  }

  if (request.method === "GET" && url.pathname === "/api/sessions/active") {
    sendJson(response, 200, { session: engine.getActiveUnifiedSession() });
    return;
  }

  const sessionMatch = url.pathname.match(/^\/api\/sessions\/([^/]+)$/);
  if (request.method === "GET" && sessionMatch) {
    const session = engine.getUnifiedSession(decodeURIComponent(sessionMatch[1]));
    if (!session) {
      sendJson(response, 404, { error: "NotFound", message: "Unified session not found" });
      return;
    }
    sendJson(response, 200, { session });
    return;
  }

  const cancelSessionMatch = url.pathname.match(/^\/api\/sessions\/([^/]+)\/cancel$/);
  if (request.method === "POST" && cancelSessionMatch) {
    sendJson(response, 200, {
      session: engine.cancelUnifiedSession(decodeURIComponent(cancelSessionMatch[1])),
    });
    return;
  }

  const runMatch = url.pathname.match(/^\/api\/runs\/([^/]+)$/);
  if (request.method === "GET" && runMatch) {
    const run = engine.getRun(decodeURIComponent(runMatch[1]));
    if (!run) {
      sendJson(response, 404, { error: "NotFound", message: "Run not found" });
      return;
    }
    sendJson(response, 200, { run });
    return;
  }

  const cancelMatch = url.pathname.match(/^\/api\/runs\/([^/]+)\/cancel$/);
  if (request.method === "POST" && cancelMatch) {
    const run = engine.cancelRun(decodeURIComponent(cancelMatch[1]));
    if (!run) {
      sendJson(response, 404, { error: "NotFound", message: "Run not found" });
      return;
    }
    sendJson(response, 200, { run });
    return;
  }

  const feedbackMatch = url.pathname.match(/^\/api\/runs\/([^/]+)\/feedback$/);
  if (request.method === "POST" && feedbackMatch) {
    const body = await readJson(request, config.limits.maxBodyBytes);
    sendJson(response, 201, {
      run: engine.addFeedback(decodeURIComponent(feedbackMatch[1]), body),
    });
    return;
  }

  const preferenceFeedbackMatch = url.pathname.match(
    /^\/api\/runs\/([^/]+)\/preference-feedback$/,
  );
  if (request.method === "POST" && preferenceFeedbackMatch) {
    const body = await readJson(request, config.limits.maxBodyBytes);
    sendJson(response, 201, {
      run: engine.addPreferenceFeedback(
        decodeURIComponent(preferenceFeedbackMatch[1]),
        body,
      ),
    });
    return;
  }

  if (request.method === "GET" && url.pathname === "/api/bridge/commands/next") {
    requireBridgeIdentity(request, store);
    const runId = url.searchParams.get("runId");
    const bridgeId = request.headers["x-aku-bridge-id"] || "aku-bridge";
    if (!runId) throw new ContractError("runId is required");
    const command = engine.claimBridgeCommand(runId, String(bridgeId).slice(0, 200));
    if (!command) {
      response.writeHead(204);
      response.end();
      return;
    }
    sendJson(response, 200, { command });
    return;
  }

  const observationMatch = url.pathname.match(
    /^\/api\/bridge\/commands\/([^/]+)\/observation$/,
  );
  if (request.method === "POST" && observationMatch) {
    requireBridgeIdentity(request, store);
    const body = await readJson(request, config.limits.maxBodyBytes);
    const run = engine.acceptBridgeObservation(
      decodeURIComponent(observationMatch[1]),
      body.runId,
      body.observation,
    );
    sendJson(response, 202, { run });
    return;
  }

  const bridgeFailureMatch = url.pathname.match(
    /^\/api\/bridge\/commands\/([^/]+)\/failure$/,
  );
  if (request.method === "POST" && bridgeFailureMatch) {
    requireBridgeIdentity(request, store);
    const body = await readJson(request, config.limits.maxBodyBytes);
    const run = engine.failBridgeCommand(
      decodeURIComponent(bridgeFailureMatch[1]),
      body.runId,
      body.error,
    );
    sendJson(response, 200, { run });
    return;
  }

  sendJson(response, 404, { error: "NotFound", message: "Route not found" });
}

function assertCompatibleBridge(bridgeDiagnostics) {
  const compatibility = bridgeDiagnostics.compatibility();
  if (compatibility.compatible) return;
  throw new ContractError(
    `AkuBridge update/reload required: ${compatibility.reasons.join(" ")}`,
    compatibility,
  );
}

function boundedIntegerQuery(url, name, { fallback, minimum, maximum }) {
  const parsed = Number.parseInt(url.searchParams.get(name) ?? "", 10);
  const value = Number.isFinite(parsed) ? parsed : fallback;
  return Math.max(minimum, Math.min(maximum, value));
}


function requireBridgeIdentity(request, store) {
  if (!store.matchesBridgeToken(request.headers["x-aku-bridge-token"])) {
    throw new ContractError("invalid bridge token");
  }
  if (request.headers["x-aku-bridge-contract"] !== BRIDGE_CONTRACT_VERSION) {
    throw new ContractError("unsupported bridge contract version");
  }
}

async function readJson(request, maxBytes) {
  const chunks = [];
  let total = 0;
  for await (const chunk of request) {
    total += chunk.length;
    if (total > maxBytes) throw new ContractError("request body is too large");
    chunks.push(chunk);
  }
  if (chunks.length === 0) return {};
  try {
    return JSON.parse(Buffer.concat(chunks).toString("utf8"));
  } catch {
    throw new ContractError("request body must be valid JSON");
  }
}

function serveStatic(response, pathname, publicDirectory) {
  const requested = pathname === "/" ? "/index.html" : pathname;
  const decoded = decodeURIComponent(requested);
  const absolute = path.resolve(publicDirectory, `.${decoded}`);
  const relative = path.relative(publicDirectory, absolute);
  if (relative.startsWith("..") || path.isAbsolute(relative)) {
    sendJson(response, 403, { error: "Forbidden", message: "Invalid static path" });
    return;
  }
  if (!fs.existsSync(absolute) || !fs.statSync(absolute).isFile()) {
    sendJson(response, 404, { error: "NotFound", message: "File not found" });
    return;
  }
  response.setHeader("Content-Type", MIME_TYPES.get(path.extname(absolute)) ?? "application/octet-stream");
  response.setHeader("Cache-Control", "no-cache");
  response.writeHead(200);
  fs.createReadStream(absolute).pipe(response);
}

function applySecurityHeaders(response, { viteDevelopment = false, host, port } = {}) {
  response.setHeader("X-Content-Type-Options", "nosniff");
  response.setHeader("Referrer-Policy", "no-referrer");
  response.setHeader("X-Frame-Options", "DENY");
  const styleSource = viteDevelopment ? "style-src 'self' 'unsafe-inline'" : "style-src 'self'";
  const connectSource = viteDevelopment
    ? `connect-src 'self' ws://${host}:${port}`
    : "connect-src 'self'";
  response.setHeader(
    "Content-Security-Policy",
    `default-src 'self'; script-src 'self'; ${styleSource}; img-src 'self' data: https://pbs.twimg.com https://video.twimg.com https://licdn.com https://*.licdn.com; ${connectSource}; frame-ancestors 'none'; base-uri 'none'; form-action 'self'`,
  );
}

function applyExtensionCors(request, response) {
  const origin = request.headers.origin;
  if (typeof origin === "string" && origin.startsWith("chrome-extension://")) {
    response.setHeader("Access-Control-Allow-Origin", origin);
    response.setHeader("Vary", "Origin");
    response.setHeader("Access-Control-Allow-Methods", "GET, POST, OPTIONS");
    response.setHeader(
      "Access-Control-Allow-Headers",
      "Content-Type, X-Aku-Bridge-Token, X-Aku-Bridge-Id, X-Aku-Bridge-Contract",
    );
  }
}

function sendJson(response, status, payload) {
  if (response.headersSent) return;
  const body = JSON.stringify(payload);
  response.setHeader("Content-Type", "application/json; charset=utf-8");
  response.setHeader("Content-Length", Buffer.byteLength(body));
  response.writeHead(status);
  response.end(body);
}

function parseBridgeActionWait(value) {
  if (value === null) return 0;
  if (!/^\d+$/.test(value)) throw new ContractError("waitMs must be an integer between 0 and 30000");
  const waitMs = Number(value);
  if (!Number.isSafeInteger(waitMs) || waitMs < 0 || waitMs > 30_000) {
    throw new ContractError("waitMs must be an integer between 0 and 30000");
  }
  return waitMs;
}
