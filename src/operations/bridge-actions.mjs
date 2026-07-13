import crypto from "node:crypto";

const DEFAULT_TIMEOUT_MS = 15_000;
const RETAINED_ACTIONS = 32;

export function createBridgeActions({
  now = () => Date.now(),
  timeoutMs = DEFAULT_TIMEOUT_MS,
  expectedBuildId,
} = {}) {
  if (!expectedBuildId) throw new Error("expectedBuildId is required");
  let active = null;
  const retained = new Map();

  function expire() {
    if (!active || terminal(active.status) || now() <= active.expiresAtMs) return;
    active = finish(active, "failed", now(), {
      errorCategory: "extension_unreachable",
      message: "AkuBridge did not complete reload_self before the bounded deadline.",
    });
    retain(active);
  }

  function retain(action) {
    retained.delete(action.requestId);
    retained.set(action.requestId, action);
    while (retained.size > RETAINED_ACTIONS) retained.delete(retained.keys().next().value);
  }

  return {
    requestReload(input, currentHeartbeat = null) {
      expire();
      const requestId = cleanRequired(input?.requestId, "requestId", 128);
      const actor = cleanRequired(input?.actor, "actor", 30);
      const reason = cleanRequired(input?.reason, "reason", 500);
      const replay = retained.get(requestId);
      if (replay) {
        if (replay.actor !== actor || replay.reason !== reason) {
          throw new BridgeActionConflict("requestId was already used with different reload_self input");
        }
        return publicAction(replay);
      }
      if (active && !terminal(active.status)) {
        throw new BridgeActionConflict("another AkuBridge cooperative action is active");
      }
      const createdAtMs = now();
      active = {
        id: crypto.randomUUID(),
        requestId,
        type: "reload_self",
        actor,
        reason,
        status: "pending",
        createdAt: timestamp(createdAtMs),
        createdAtMs,
        expiresAt: timestamp(createdAtMs + timeoutMs),
        expiresAtMs: createdAtMs + timeoutMs,
        deliveredAt: null,
        acceptedAt: null,
        completedAt: null,
        previousBuildId: clean(currentHeartbeat?.buildId, 160),
        expectedBuildId,
        observedBuildId: null,
        errorCategory: null,
        message: null,
      };
      retain(active);
      return publicAction(active);
    },

    next() {
      expire();
      if (!active || active.status !== "pending") return null;
      active = {
        ...active,
        status: "delivered",
        deliveredAt: timestamp(now()),
      };
      retain(active);
      return publicAction(active);
    },

    accept(actionId) {
      expire();
      if (!active || active.id !== actionId) {
        throw new BridgeActionNotFound("unknown or expired AkuBridge action");
      }
      if (active.status === "accepted" || active.status === "completed") {
        return publicAction(active);
      }
      if (active.status !== "delivered") {
        throw new BridgeActionConflict(`cannot accept an action in ${active.status} state`);
      }
      active = {
        ...active,
        status: "accepted",
        acceptedAt: timestamp(now()),
      };
      retain(active);
      return publicAction(active);
    },

    observeHeartbeat(heartbeat) {
      expire();
      if (!active || active.status !== "accepted") return null;
      if (heartbeat?.buildId !== active.expectedBuildId) return publicAction(active);
      active = finish(active, "completed", now(), {
        observedBuildId: heartbeat.buildId,
        message: "AkuBridge reload_self completed and the expected build heartbeat was observed.",
      });
      retain(active);
      return publicAction(active);
    },

    get(actionId) {
      expire();
      const action = active?.id === actionId
        ? active
        : [...retained.values()].find((candidate) => candidate.id === actionId);
      if (!action) throw new BridgeActionNotFound("unknown AkuBridge action");
      return publicAction(action);
    },
  };
}

export class BridgeActionConflict extends Error {}
export class BridgeActionNotFound extends Error {}

function finish(action, status, atMs, fields) {
  return { ...action, ...fields, status, completedAt: timestamp(atMs) };
}

function publicAction(action) {
  const {
    createdAtMs: _createdAtMs,
    expiresAtMs: _expiresAtMs,
    ...view
  } = action;
  return structuredClone(view);
}

function terminal(status) {
  return status === "completed" || status === "failed";
}

function timestamp(value) {
  return new Date(value).toISOString();
}

function cleanRequired(value, field, maximum) {
  const cleaned = clean(value, maximum);
  if (!cleaned) throw new TypeError(`${field} is required`);
  return cleaned;
}

function clean(value, maximum) {
  if (typeof value !== "string") return null;
  const normalized = value.trim();
  return normalized ? normalized.slice(0, maximum) : null;
}
