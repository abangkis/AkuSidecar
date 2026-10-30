const terminalRunStatuses = new Set(["completed", "partial", "failed", "cancelled"]);

export function sourceCaptureSurfaceReleasable(run) {
  // Native collectors retire their resources through their own batch owner.
  if (!runUsesBrowserCapture(run)) return false;
  return terminalRunStatuses.has(run?.status) ||
    (run?.status === "reasoning" && run?.stage === "candidate_evaluation");
}

export async function releaseCompletedSourceSurfaces({
  session,
  releasedSources,
  releaseSource,
}) {
  if (!session?.id || terminalRunStatuses.has(session.status)) {
    return { ready: true, released: 0, error: null };
  }
  let released = 0;
  for (const run of session.runs ?? []) {
    if (!run?.source || !sourceCaptureSurfaceReleasable(run)) continue;
    const key = `${session.id}:${run.source}`;
    if (releasedSources.has(key)) continue;
    releasedSources.add(key);
    try {
      await releaseSource(session.id, run.source);
      released += 1;
    } catch (error) {
      releasedSources.delete(key);
      return { ready: false, released, error };
    }
  }
  return { ready: true, released, error: null };
}

// Unknown/legacy runs remain conservative. This metadata never authorizes capture.
export function sessionUsesBrowserCapture(session) {
 return !Array.isArray(session?.runs) || session.runs.length === 0 ||
   session.runs.some(runUsesBrowserCapture);
}

export function runUsesBrowserCapture(run) {
 return !["headless", "browser_quiet_hidden"].includes(run?.captureCollector);
}
