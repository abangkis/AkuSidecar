const DIAGNOSTIC_FIELDS = [
  "capturedCandidates",
  "skippedResurfaces",
  "evaluatedCandidates",
  "failedCaptureRuns",
  "failedReasoningRuns",
  "failedPlanningRuns",
  "planningFallbackRuns",
];

function countFrom(diagnostics, field) {
  const value = diagnostics?.[field];
  return Number.isSafeInteger(value) && value >= 0 ? value : null;
}

function evaluationFailureCount(diagnostics) {
  const totalFailures = countFrom(diagnostics, "failedReasoningRuns");
  const planningFailures = countFrom(diagnostics, "failedPlanningRuns");
  if (totalFailures === null) return null;
  if (planningFailures === null) return totalFailures;
  return Math.max(0, totalFailures - planningFailures);
}

function plural(count, singular, pluralForm = `${singular}s`) {
  return `${count} ${count === 1 ? singular : pluralForm}`;
}

function failureDetails(diagnostics, omittedFields = new Set()) {
  const details = [];
  const captureFailures = countFrom(diagnostics, "failedCaptureRuns");
  const planningFailures = countFrom(diagnostics, "failedPlanningRuns");
  const evaluationFailures = evaluationFailureCount(diagnostics);
  const planningFallbacks = countFrom(diagnostics, "planningFallbackRuns");

  if (captureFailures > 0 && !omittedFields.has("failedCaptureRuns")) details.push(plural(captureFailures, "capture run") + " failed");
  if (planningFailures > 0 && !omittedFields.has("failedPlanningRuns")) details.push(plural(planningFailures, "planning run") + " failed");
  if (evaluationFailures > 0 && !omittedFields.has("failedReasoningRuns")) details.push(plural(evaluationFailures, "evaluation run") + " failed");
  if (planningFallbacks > 0) {
    details.push(`Optional planning was skipped in ${plural(planningFallbacks, "run")}; evaluation continued`);
  }
  return details;
}

function hasDiagnostics(diagnostics) {
  return diagnostics && typeof diagnostics === "object" && DIAGNOSTIC_FIELDS.some((field) => countFrom(diagnostics, field) !== null);
}

export function formatTimelineCheckSummary(latestCheck) {
  const addedItems = countFrom(latestCheck, "addedItems");
  const diagnostics = latestCheck?.diagnostics;

  if (addedItems > 0) {
    const captured = countFrom(diagnostics, "capturedCandidates");
    const lead = `${plural(addedItems, "new item")}${captured === null ? "" : ` from ${plural(captured, "captured candidate")}`}`;
    return [lead, ...failureDetails(diagnostics)].join(" · ");
  }

  if (!hasDiagnostics(diagnostics)) return "No new items";

  const captured = countFrom(diagnostics, "capturedCandidates");
  const evaluated = countFrom(diagnostics, "evaluatedCandidates");
  const skippedResurfaces = countFrom(diagnostics, "skippedResurfaces");
  const outcome = latestCheck?.outcome;
  let lead;

  switch (outcome) {
    case "planning_failed": {
      const failedRuns = countFrom(diagnostics, "failedPlanningRuns");
      lead = failedRuns > 0 ? `${plural(failedRuns, "planning run")} failed before evaluation` : "Planning failed before evaluation";
      if (captured > 0) lead += ` · ${plural(captured, "candidate")} captured`;
      break;
    }
    case "reasoning_failed": {
      const failedRuns = evaluationFailureCount(diagnostics);
      lead = failedRuns > 0 ? `${plural(failedRuns, "evaluation run")} failed` : "Evaluation could not complete";
      if (evaluated > 0) lead += ` · ${plural(evaluated, "candidate")} evaluated`;
      else if (captured > 0) lead += ` · ${plural(captured, "candidate")} captured`;
      break;
    }
    case "capture_failed": {
      const failedRuns = countFrom(diagnostics, "failedCaptureRuns");
      lead = failedRuns > 0 ? `${plural(failedRuns, "source capture run")} failed` : "Source capture failed";
      if (captured > 0) lead += ` · ${plural(captured, "candidate")} captured`;
      break;
    }
    case "empty_capture":
      lead = "No candidates were captured";
      break;
    case "unchanged":
      if (skippedResurfaces > 0) {
        lead = `No new items · ${plural(skippedResurfaces, "unchanged candidate")} stayed in cooldown`;
      } else {
        lead = "No new items · unchanged candidates remain in cooldown";
      }
      break;
    case "no_selected_items":
      lead = evaluated > 0
        ? `${plural(evaluated, "candidate")} evaluated; none were selected`
        : "No items were selected";
      break;
    case "duplicate_reports_only":
      lead = "Reports matched events already represented in the timeline";
      break;
    default:
      return "No new items";
  }

  const omittedFields = new Set({
    planning_failed: ["failedPlanningRuns"],
    reasoning_failed: ["failedReasoningRuns"],
    capture_failed: ["failedCaptureRuns"],
  }[outcome] ?? []);
  const details = failureDetails(diagnostics, omittedFields);
  return [lead, ...details].join(" · ");
}
