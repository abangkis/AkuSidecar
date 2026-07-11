import {
  validateAcquisitionPlan,
  validateReasoningResult,
} from "../core/contracts.mjs";
import { providerCapabilities } from "./provider-capabilities.mjs";

export async function runProviderConformance(provider, fixture, options = {}) {
  const checks = [];
  const providerId = options.providerId ?? provider?.providerId ?? inferProviderId(provider);
  const manifest = providerCapabilities(providerId);
  check(checks, "manifest", Boolean(manifest), `No capability manifest for ${providerId}.`);
  check(checks, "provider_name", typeof provider?.name === "string" && provider.name.length > 0);
  check(checks, "analyze_method", typeof provider?.analyze === "function");
  check(checks, "planning_method", typeof provider?.planAcquisition === "function");

  if (typeof provider?.analyze === "function") {
    try {
      const response = await provider.analyze({
        run: fixture.run,
        observation: fixture.observation,
        observations: [fixture.observation],
        knowledgeContext: fixture.knowledgeContext,
      });
      const invocation = unwrapInvocation(response);
      const result = validateReasoningResult(invocation.output, fixture.run.maxItems);
      const observed = new Set(
        fixture.observation.snapshots.flatMap((snapshot) =>
          snapshot.blocks.map((block) => block.evidenceKey),
        ),
      );
      const assessed = result.candidateAssessments.map((entry) => entry.evidenceKey);
      check(
        checks,
        "candidate_coverage",
        assessed.length === observed.size && assessed.every((key) => observed.has(key)) &&
          new Set(assessed).size === assessed.length,
        "Candidate assessments must cover each observed evidence key exactly once.",
      );
      check(
        checks,
        "item_provenance",
        result.items.every((item) => observed.has(item.evidenceKey)),
        "Every item must reference observed evidence.",
      );
      check(
        checks,
        "telemetry_envelope",
        invocation.telemetry === null || validTelemetry(invocation.telemetry),
        "Telemetry must be null or a valid phase envelope.",
      );
    } catch (error) {
      check(checks, "candidate_evaluation", false, error.message);
    }
  }

  if (typeof provider?.planAcquisition === "function") {
    try {
      const response = await provider.planAcquisition({
        run: fixture.run,
        observation: fixture.observation,
        knowledgeContext: fixture.knowledgeContext,
        budget: fixture.budget,
      });
      const invocation = unwrapInvocation(response);
      validateAcquisitionPlan(invocation.output);
      check(
        checks,
        "planning_telemetry",
        invocation.telemetry === null || validTelemetry(invocation.telemetry),
        "Planning telemetry must be null or a valid phase envelope.",
      );
    } catch (error) {
      check(checks, "acquisition_planning", false, error.message);
    }
  }

  const passed = checks.every((entry) => entry.passed);
  return {
    version: 1,
    providerId,
    providerName: provider?.name ?? null,
    passed,
    pilotQualityEligible: manifest?.pilotQualityEligible === true && passed,
    checks,
  };
}

function unwrapInvocation(response) {
  if (
    response && typeof response === "object" &&
    Object.prototype.hasOwnProperty.call(response, "output") &&
    Object.prototype.hasOwnProperty.call(response, "telemetry")
  ) {
    return response;
  }
  return { output: response, telemetry: null, evaluatedEvidenceKeys: null };
}

function validTelemetry(value) {
  return value && typeof value === "object" &&
    typeof value.provider === "string" &&
    ["candidate_evaluation", "acquisition_planning"].includes(value.phase) &&
    ["completed", "failed"].includes(value.status) &&
    Number.isFinite(value.durationMs) && value.durationMs >= 0;
}

function check(target, id, passed, detail = "") {
  target.push({ id, passed: passed === true, detail: passed ? "" : detail || `${id} failed.` });
}

function inferProviderId(provider) {
  return provider?.name === "codex-sdk" ? "codex-sdk" :
    provider?.name === "deterministic-development-fallback" ? "deterministic" :
      String(provider?.name ?? "unknown");
}
