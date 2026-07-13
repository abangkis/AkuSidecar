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
  const waiters = new Set();
  let relayLastSeenAtMs = null;

  function expire() {
    if (!active || terminal(active.status) || now() <= active.expiresAtMs) return;
    const errorCategory = expiryCategory(active, relayLastSeenAtMs);
    active = finish(active, "failed", now(), {
      errorCategory,
      message: expiryMessage(errorCategory),
    });
    retain(active);
  }

  function retain(action) {
    retained.delete(action.requestId);
    retained.set(action.requestId, action);
    while (retained.size > RETAINED_ACTIONS) retained.delete(retained.keys().next().value);
  }

  function claimNext() {
    relayLastSeenAtMs = now();
    expire();
    if (!active || active.status !== "pending") return null;
    active = {
      ...active,
      status: "delivered",
      deliveredAt: timestamp(now()),
    };
    retain(active);
    return publicAction(active);
  }

  function wakeWaiters() {
    for (const wake of [...waiters]) wake();
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
      wakeWaiters();
      return publicAction(active);
    },

    next() {
      return claimNext();
    },

    waitForNext(waitMs = 0) {
      const immediate = claimNext();
      if (immediate || waitMs <= 0) return Promise.resolve(immediate);
      return new Promise((resolve) => {
        let timer;
        const finishWait = () => {
          clearTimeout(timer);
          waiters.delete(finishWait);
          resolve(claimNext());
        };
        timer = setTimeout(finishWait, waitMs);
        waiters.add(finishWait);
      });
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
      const observedBuildId = clean(heartbeat?.buildId, 160);
      active = {
        ...active,
        observedBuildId,
        heartbeatObservedAt: timestamp(now()),
      };
      if (observedBuildId !== active.expectedBuildId) {
        retain(active);
        return publicAction(active);
      }
      active = finish(active, "completed", now(), {
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

function expiryCategory(action, relayLastSeenAtMs) {
  if (action.status === "pending") {
    return relayLastSeenAtMs === null || relayLastSeenAtMs < action.createdAtMs
      ? "relay_page_stale"
      : "relay_not_delivered";
  }
  if (action.status === "delivered") return "extension_not_accepted";
  if (action.status === "accepted" && action.observedBuildId) return "build_mismatch";
  return "reload_heartbeat_timeout";
}

function expiryMessage(category) {
  return {
    relay_page_stale: "AkuBrowser relay page did not request the cooperative action before the deadline.",
    relay_not_delivered: "AkuBrowser relay did not claim reload_self before the deadline.",
    extension_not_accepted: "AkuBridge did not accept the delivered reload_self action before the deadline.",
    reload_heartbeat_timeout: "AkuBridge accepted reload_self but no post-reload heartbeat arrived before the deadline.",
    build_mismatch: "AkuBridge reloaded but did not announce the expected build identity before the deadline.",
  }[category];
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
