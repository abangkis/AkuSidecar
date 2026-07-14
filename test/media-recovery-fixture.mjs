export function mediaRecoveryFixture(source = "x", outcome = "not_applicable") {
  const recovered = outcome === "recovered";
  const unavailable = outcome === "unavailable";
  return {
    policyVersion: "media-recovery-v1",
    strategyVersion: `${source}-media-recovery-v1`,
    source,
    outcome,
    attempts: recovered || unavailable ? 1 : 0,
    recoveredCount: recovered ? 1 : 0,
    method: recovered ? "alternate_dom" : "none",
    limitation: unavailable
      ? "Rendered media remained unavailable after the bounded adapter recovery."
      : "",
  };
}

export function mediaRecoverySummaryFixture(recoveries) {
  const values = Array.isArray(recoveries) ? recoveries : [];
  return {
    policyVersion: "media-recovery-v1",
    candidateCount: values.length,
    outcomes: Object.fromEntries(
      ["not_applicable", "primary_complete", "recovered", "unavailable"].map((outcome) => [
        outcome,
        values.filter((entry) => entry.outcome === outcome).length,
      ]),
    ),
    attempts: values.reduce((sum, entry) => sum + entry.attempts, 0),
    recoveredMediaCount: values.reduce((sum, entry) => sum + entry.recoveredCount, 0),
    methods: [...new Set(values.map((entry) => entry.method).filter((method) => method !== "none"))],
  };
}
