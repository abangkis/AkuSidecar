import { ContractError } from "../core/contracts.mjs";

const VERDICTS = ["complete", "usable_degraded", "retryable", "invalid"];

export function admitObservationQuality(observation, { required = false } = {}) {
  const reports = observation.snapshots.flatMap((snapshot) => snapshot.qualityReports ?? []);
  const blockReports = observation.snapshots.flatMap((snapshot) =>
    snapshot.blocks.map((block) => block.captureQuality).filter(Boolean),
  );
  const summary = observation.coverage?.captureQuality ?? null;

  if (!required && reports.length === 0 && blockReports.length === 0 && !summary) {
    return observation;
  }
  if (!summary) throw new ContractError("capture-quality summary is required");
  if (required && observation.snapshots.some((snapshot) =>
    snapshot.blocks.some((block) => !block.captureQuality),
  )) {
    throw new ContractError("every admitted bridge block requires a capture-quality report");
  }
  if (required && reports.length === 0) {
    throw new ContractError("capture-quality candidate reports are required");
  }
  if (required) assertMediaRecoveryConsistency(observation);

  for (const report of reports) assertReportConsistency(report);
  for (const snapshot of observation.snapshots) {
    const availableReports = countReportSignatures(snapshot.qualityReports ?? []);
    for (const block of snapshot.blocks) {
      if (!block.captureQuality) continue;
      assertReportConsistency(block.captureQuality, block);
      const signature = reportSignature(block.captureQuality);
      if ((availableReports.get(signature) ?? 0) < 1) {
        throw new ContractError("block capture-quality report is absent from its snapshot reports");
      }
      availableReports.set(signature, availableReports.get(signature) - 1);
    }
  }

  const verdictCounts = Object.fromEntries(VERDICTS.map((verdict) => [
    verdict,
    reports.filter((report) => report.verdict === verdict).length,
  ]));
  if (summary.candidateReportCount !== reports.length) {
    throw new ContractError("capture-quality summary count does not match candidate reports");
  }
  for (const verdict of VERDICTS) {
    if (summary.verdictCounts?.[verdict] !== verdictCounts[verdict]) {
      throw new ContractError(`capture-quality ${verdict} count is inconsistent`);
    }
  }
  const expectedSummaryVerdict = verdictCounts.invalid > 0
    ? "invalid"
    : verdictCounts.retryable > 0
      ? "retryable"
      : verdictCounts.usable_degraded > 0
        ? "usable_degraded"
        : "complete";
  if (summary.verdict !== expectedSummaryVerdict) {
    throw new ContractError("capture-quality aggregate verdict is inconsistent");
  }
  const issueCounts = {};
  for (const report of reports) {
    for (const issue of report.issues) {
      const key = `${issue.field}:${issue.code}`;
      issueCounts[key] = (issueCounts[key] ?? 0) + 1;
    }
  }
  if (!sameCountMap(summary.issueCounts, issueCounts)) {
    throw new ContractError("capture-quality issue counts are inconsistent");
  }
  const retryAttempts = reports.reduce((sum, report) => sum + report.attempt, 0);
  if (summary.retryAttempts !== retryAttempts) {
    throw new ContractError("capture-quality retry attempt count is inconsistent");
  }
  if (reports.some((report) => report.profile !== summary.profile)) {
    throw new ContractError("capture-quality profiles are inconsistent");
  }
  if (verdictCounts.retryable > 0 || summary.verdict === "retryable") {
    throw new ContractError("AkuBridge returned a retryable candidate as a final observation");
  }

  let admittedBlockCount = 0;
  let degradedBlockCount = 0;
  const snapshots = observation.snapshots.map((snapshot) => ({
    ...snapshot,
    blocks: snapshot.blocks.filter((block) => {
      const verdict = block.captureQuality?.verdict;
      if (verdict === "invalid") return false;
      admittedBlockCount += 1;
      if (verdict === "usable_degraded") degradedBlockCount += 1;
      return true;
    }),
  }));
  if (admittedBlockCount === 0) {
    throw new ContractError("capture quality rejected every observed candidate");
  }

  const rejectedCandidateCount = verdictCounts.invalid;
  const admissionVerdict = rejectedCandidateCount > 0 || degradedBlockCount > 0
    ? "usable_degraded"
    : "complete";
  const note = admissionVerdict === "complete"
    ? `Capture quality admitted ${admittedBlockCount} block(s) without degradation.`
    : `Capture quality admitted ${admittedBlockCount} block(s) with ${degradedBlockCount} degraded block(s) and ${rejectedCandidateCount} rejected candidate report(s).`;
  return {
    ...observation,
    snapshots,
    coverage: {
      ...observation.coverage,
      qualityAdmission: {
        verdict: admissionVerdict,
        profile: summary.profile,
        admittedBlockCount,
        degradedBlockCount,
        rejectedCandidateCount,
        retryAttempts,
        issueCounts: summary.issueCounts,
      },
      notes: [...(observation.coverage.notes ?? []), note].slice(-10),
    },
  };
}

function assertMediaRecoveryConsistency(observation) {
  const blocks = observation.snapshots.flatMap((snapshot) => snapshot.blocks ?? []);
  const recoveries = blocks.map((block) => block.mediaRecovery);
  if (recoveries.some((entry) => !entry)) {
    throw new ContractError("every admitted bridge block requires a media-recovery outcome");
  }
  const summary = observation.coverage?.mediaRecovery;
  if (!summary || summary.policyVersion !== "media-recovery-v1") {
    throw new ContractError("media-recovery summary is required");
  }
  if (summary.candidateCount !== recoveries.length) {
    throw new ContractError("media-recovery summary count does not match observed blocks");
  }
  for (const outcome of ["not_applicable", "primary_complete", "recovered", "unavailable"]) {
    const count = recoveries.filter((entry) => entry.outcome === outcome).length;
    if (summary.outcomes?.[outcome] !== count) {
      throw new ContractError(`media-recovery ${outcome} count is inconsistent`);
    }
  }
  for (const [index, block] of blocks.entries()) {
    const recovery = block.mediaRecovery;
    const hasMedia = (block.media?.length ?? 0) > 0;
    if (recovery.policyVersion !== "media-recovery-v1" || recovery.source !== observation.source) {
      throw new ContractError(`block ${index} media-recovery identity is inconsistent`);
    }
    if (["primary_complete", "recovered"].includes(recovery.outcome) && !hasMedia) {
      throw new ContractError(`block ${index} media-recovery outcome contradicts empty media`);
    }
    if (["not_applicable", "unavailable"].includes(recovery.outcome) && hasMedia) {
      throw new ContractError(`block ${index} media-recovery outcome contradicts media value`);
    }
    if (
      recovery.outcome === "unavailable" &&
      !block.captureQuality?.issues?.some((issue) => issue.field === "media")
    ) {
      throw new ContractError(`block ${index} unavailable media requires a quality limitation`);
    }
  }
  const expectedAttempts = recoveries.reduce((sum, entry) => sum + entry.attempts, 0);
  const expectedRecoveredCount = recoveries.reduce(
    (sum, entry) => sum + entry.recoveredCount,
    0,
  );
  const expectedMethods = [...new Set(
    recoveries.map((entry) => entry.method).filter((method) => method !== "none"),
  )].sort();
  const observedMethods = [...(summary.methods ?? [])].sort();
  if (
    summary.attempts !== expectedAttempts ||
    summary.recoveredMediaCount !== expectedRecoveredCount ||
    JSON.stringify(observedMethods) !== JSON.stringify(expectedMethods)
  ) {
    throw new ContractError("media-recovery aggregate accounting is inconsistent");
  }
  const recovered = recoveries.some((entry) => entry.outcome === "recovered");
  if (observation.coverage.fallbackUsed !== recovered) {
    throw new ContractError("fallbackUsed must match recovered media evidence");
  }
}

function assertReportConsistency(report, block = null) {
  if (report.verdict === "complete" && report.issues.length > 0) {
    throw new ContractError("complete capture-quality reports cannot contain issues");
  }
  if (report.verdict === "usable_degraded" && report.issues.length === 0) {
    throw new ContractError("degraded capture-quality reports require an issue");
  }
  if (
    report.verdict === "usable_degraded" &&
    report.issues.some((issue) => issue.severity === "critical")
  ) {
    throw new ContractError("degraded capture-quality reports cannot contain critical issues");
  }
  if (
    report.verdict === "invalid" &&
    !report.issues.some((issue) => issue.severity === "critical")
  ) {
    throw new ContractError("invalid capture-quality reports require a critical issue");
  }
  if (
    report.verdict === "retryable" &&
    !report.issues.some((issue) => issue.recoverable)
  ) {
    throw new ContractError("retryable capture-quality reports require a recoverable issue");
  }
  if (report.issues.some((issue) => issue.attempt !== report.attempt)) {
    throw new ContractError("capture-quality issue attempts must match their report attempt");
  }
  if (!block) return;
  if (!block.author && !report.issues.some((issue) => issue.field === "author")) {
    throw new ContractError("capture-quality report omitted the missing author field");
  }
  if (
    (block.media?.length ?? 0) > 0 &&
    report.issues.some((issue) => issue.field === "media" && issue.observedState !== "present")
  ) {
    throw new ContractError("capture-quality media issue contradicts the admitted media value");
  }
  if (
    block.avatarUrl &&
    report.issues.some((issue) => issue.field === "avatarUrl" && issue.observedState !== "present")
  ) {
    throw new ContractError("capture-quality avatar issue contradicts the admitted avatar value");
  }
}

function countReportSignatures(reports) {
  const counts = new Map();
  for (const report of reports) {
    const signature = reportSignature(report);
    counts.set(signature, (counts.get(signature) ?? 0) + 1);
  }
  return counts;
}

function reportSignature(report) {
  return JSON.stringify({
    profile: report.profile,
    verdict: report.verdict,
    score: report.score,
    attempt: report.attempt,
    issues: report.issues,
  });
}

function sameCountMap(left, right) {
  const keys = new Set([...Object.keys(left ?? {}), ...Object.keys(right ?? {})]);
  return [...keys].every((key) => (left?.[key] ?? 0) === (right?.[key] ?? 0));
}
