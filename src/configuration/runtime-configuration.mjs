import { ContractError } from "../core/contracts.mjs";

const DEFINITIONS = {
  missingSourceTabPolicy: {
    key: "runtime.missing_source_tab_policy",
    applyMode: "next_run",
    values: new Set(["open_missing_tab", "fail_fast"]),
    read: (config) => config.limits.missingSourceTabPolicy,
    apply: (config, value) => { config.limits.missingSourceTabPolicy = value; },
  },
  defaultPresentation: {
    key: "ui.default_presentation",
    applyMode: "live",
    values: new Set(["source", "brief"]),
    read: (config) => config.presentation.defaultLayout,
    apply: (config, value) => { config.presentation.defaultLayout = value; },
  },
  homePresentation: {
    key: "ui.home_presentation",
    applyMode: "live",
    values: new Set(["timeline", "overview"]),
    read: (config) => config.presentation.homePresentation,
    apply: (config, value) => { config.presentation.homePresentation = value; },
  },
  streamWidth: {
    key: "ui.stream_width",
    applyMode: "live",
    values: new Set(["compact", "social", "comfortable", "wide"]),
    read: (config) => config.presentation.streamWidth,
    apply: (config, value) => { config.presentation.streamWidth = value; },
  },
  telemetryBehavior: {
    key: "ui.telemetry_behavior",
    applyMode: "live",
    values: new Set(["flow", "sticky"]),
    read: (config) => config.presentation.telemetryBehavior,
    apply: (config, value) => { config.presentation.telemetryBehavior = value; },
  },
  reasoningProvider: {
    key: "startup.reasoning_provider",
    applyMode: "restart",
    values: new Set(["deterministic", "codex-sdk"]),
    read: (config) => config.reasoning.provider,
    apply: (config, value) => { config.reasoning.provider = value; },
  },
  planningModel: modelDefinition("startup.planning_model", "planningModel"),
  evaluationModel: modelDefinition("startup.evaluation_model", "evaluationModel"),
  planningEffort: reasoningDefinition(
    "startup.planning_effort",
    "planningEffort",
    new Set(["minimal", "low", "medium", "high", "xhigh"]),
  ),
  evaluationEffort: reasoningDefinition(
    "startup.evaluation_effort",
    "evaluationEffort",
    new Set(["minimal", "low", "medium", "high", "xhigh"]),
  ),
  planningPolicy: {
    ...reasoningDefinition(
      "startup.planning_policy",
      "planningPolicy",
      new Set(["always", "deterministic_sparse_gap"]),
    ),
    apply(config, value) {
      config.reasoning.planningPolicy = value;
      config.limits.acquisitionPlanningPolicy = value;
    },
  },
  timeoutMs: {
    key: "startup.reasoning_timeout_ms",
    applyMode: "restart",
    parse: (value) => Number.parseInt(value, 10),
    valid: (value) => Number.isInteger(value) && value >= 1_000 && value <= 600_000,
    read: (config) => config.reasoning.timeoutMs,
    apply: (config, value) => { config.reasoning.timeoutMs = value; },
  },
};

export function applyPersistedConfiguration(config, store) {
  for (const [name, definition] of Object.entries(DEFINITIONS)) {
    if (metadata(config, name).environmentOverride !== null) continue;
    const value = persistedValue(store, definition);
    if (value !== null) definition.apply(config, value);
  }
}

export function updateDashboardConfiguration(config, store, input) {
  if (!input || typeof input !== "object" || Array.isArray(input)) {
    throw new ContractError("configuration update must be an object");
  }
  const names = Object.keys(input);
  if (names.length === 0 || names.some((name) => !DEFINITIONS[name])) {
    throw new ContractError("configuration update contains an unknown or empty setting");
  }
  for (const name of names) {
    const definition = DEFINITIONS[name];
    if (metadata(config, name).environmentOverride !== null) {
      throw new ContractError(`${name} is locked by an environment override`);
    }
    const value = normalize(definition, input[name]);
    if (value === null) throw new ContractError(`${name} has an invalid value`);
  }
  for (const name of names) {
    const definition = DEFINITIONS[name];
    const value = normalize(definition, input[name]);
    store.setSetting(definition.key, String(value));
    if (definition.applyMode !== "restart") definition.apply(config, value);
  }
}

export function configurationView(config, store) {
  return Object.fromEntries(
    Object.entries(DEFINITIONS).map(([name, definition]) => {
      const settingMetadata = metadata(config, name);
      const persisted = persistedValue(store, definition);
      const effective = definition.read(config);
      const environmentOverride = settingMetadata.environmentOverride;
      const source = environmentOverride !== null
        ? "environment"
        : persisted !== null
          ? "dashboard"
          : "default";
      return [name, {
        effectiveValue: effective,
        persistedValue: persisted,
        defaultValue: settingMetadata.defaultValue ?? null,
        source,
        environmentOverride,
        applyMode: definition.applyMode,
        restartRequired:
          definition.applyMode === "restart" && persisted !== null && persisted !== effective,
      }];
    }),
  );
}

function metadata(config, name) {
  return {
    defaultValue: config.runtimeConfiguration?.[name]?.defaultValue ?? null,
    environmentOverride:
      config.runtimeConfiguration?.[name]?.environmentOverride ?? null,
  };
}

function persistedValue(store, definition) {
  const raw = store.getSetting(definition.key);
  return raw === null ? null : normalize(definition, raw);
}

function normalize(definition, value) {
  const parsed = definition.parse ? definition.parse(value) : value;
  const valid = definition.valid
    ? definition.valid(parsed)
    : definition.values.has(parsed);
  return valid ? parsed : null;
}

function reasoningDefinition(key, property, values) {
  return {
    key,
    applyMode: "restart",
    values,
    read: (config) => config.reasoning[property],
    apply: (config, value) => { config.reasoning[property] = value; },
  };
}

function modelDefinition(key, property) {
  return {
    key,
    applyMode: "restart",
    parse: (value) => typeof value === "string" ? value.trim() : "",
    valid: (value) => /^[a-z0-9][a-z0-9._-]{1,99}$/i.test(value),
    read: (config) => config.reasoning[property],
    apply: (config, value) => { config.reasoning[property] = value; },
  };
}
