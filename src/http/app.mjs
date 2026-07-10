import fs from "node:fs";
import http from "node:http";
import path from "node:path";
import { URL } from "node:url";
import { ContractError } from "../core/contracts.mjs";
import { JobEngine } from "../core/job-engine.mjs";

const MIME_TYPES = new Map([
  [".html", "text/html; charset=utf-8"],
  [".js", "text/javascript; charset=utf-8"],
  [".css", "text/css; charset=utf-8"],
  [".json", "application/json; charset=utf-8"],
  [".svg", "image/svg+xml"],
  [".png", "image/png"],
]);

export const BRIDGE_CONTRACT_VERSION = "aku-browser.bridge.v1";

export function createAkuBrowserApp({ config, store, reasoningProvider, logger = console }) {
  const engine = new JobEngine({
    store,
    reasoningProvider,
    limits: config.limits,
    logger,
  });
  const bridgeToken = store.getOrCreateBridgeToken();

  const server = http.createServer(async (request, response) => {
    try {
      applySecurityHeaders(response);
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
          config,
        });
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
      if (!server.listening) return;
      await new Promise((resolve, reject) =>
        server.close((error) => (error ? reject(error) : resolve())),
      );
    },
  };
}

async function handleApi({ request, response, url, engine, store, bridgeToken, config }) {
  if (request.method === "GET" && url.pathname === "/api/health") {
    sendJson(response, 200, {
      status: "ok",
      provider: engine.reasoningProvider.name,
      time: new Date().toISOString(),
    });
    return;
  }

  if (request.method === "GET" && url.pathname === "/api/bootstrap") {
    sendJson(response, 200, {
      version: "0.1.0",
      bridgeContractVersion: BRIDGE_CONTRACT_VERSION,
      provider: engine.reasoningProvider.name,
      bridgeToken,
      limits: config.limits,
      supportedModes: ["catch_up", "manual_live"],
      supportedSources: ["x", "linkedin"],
    });
    return;
  }

  if (request.method === "GET" && url.pathname === "/api/runs") {
    const limit = Math.max(1, Math.min(50, Number.parseInt(url.searchParams.get("limit") ?? "20", 10)));
    sendJson(response, 200, { runs: engine.listRuns(limit) });
    return;
  }

  if (request.method === "POST" && url.pathname === "/api/runs") {
    const body = await readJson(request, config.limits.maxBodyBytes);
    sendJson(response, 201, { run: engine.startRun(body) });
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

function applySecurityHeaders(response) {
  response.setHeader("X-Content-Type-Options", "nosniff");
  response.setHeader("Referrer-Policy", "no-referrer");
  response.setHeader("X-Frame-Options", "DENY");
  response.setHeader(
    "Content-Security-Policy",
    "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'",
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
