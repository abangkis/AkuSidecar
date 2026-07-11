export const PROVIDER_CAPABILITIES = Object.freeze({
  "codex-sdk": Object.freeze({
    id: "codex-sdk",
    execution: "local_cli_remote_model",
    phases: Object.freeze(["candidate_evaluation", "acquisition_planning"]),
    structuredOutput: true,
    modelConfigurable: true,
    reasoningEffortConfigurable: true,
    usageTelemetry: "provider_reported",
    pilotQualityEligible: true,
    defaultConformanceRun: false,
  }),
  deterministic: Object.freeze({
    id: "deterministic",
    execution: "in_process",
    phases: Object.freeze(["candidate_evaluation", "acquisition_planning"]),
    structuredOutput: true,
    modelConfigurable: false,
    reasoningEffortConfigurable: false,
    usageTelemetry: "not_reported",
    pilotQualityEligible: false,
    defaultConformanceRun: true,
  }),
});

export function providerCapabilities(providerId) {
  return PROVIDER_CAPABILITIES[providerId] ?? null;
}
