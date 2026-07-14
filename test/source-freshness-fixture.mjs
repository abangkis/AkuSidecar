export function sourceFreshnessFixture(source, outcome = "active_feed_ready") {
  const followUp = outcome === "follow_up_preserved";
  return {
    policyVersion: "source-freshness-recovery-v1",
    adapterFreshnessVersion: `${source}-freshness-v1`,
    source,
    status: "ready",
    outcome,
    verification: followUp ? "frontier_contract" : "active_dispatch",
    evidence: followUp ? "follow_up_no_freshness_mutation" : "active_at_dispatch",
    backgroundAtDispatch: false,
    opened: false,
    wakeAttempted: false,
    activated: false,
    probeCount: 1,
    pendingContentDetected: false,
    pendingContentLabel: "",
    pendingContentAction: "not_detected",
    feedChanged: false,
    feedMutation: false,
    waitMs: 5,
    preActionScrollY: 0,
  };
}
