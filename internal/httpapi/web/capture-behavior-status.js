export function captureBehaviorDiagnostics(entries, { sourceLabel, formatMoment } = {}) {
  if (!Array.isArray(entries)) return [];
  return entries.filter(entry => entry?.enabled === true && entry.allowed === false).map(entry => {
    const label = sourceLabel ? sourceLabel(entry.source) : String(entry.source || "Source");
    const reasons = {
      capture_active: "capture already active",
      capture_spacing: "capture spacing",
      temporary_backoff: "temporary backoff",
      challenge_required: "manual verification required",
    };
    const reason = reasons[entry.reason] || "capture admission waiting";
    const hasTime = Number.isFinite(Date.parse(entry.notBefore));
    return `${label}: ${reason}${hasTime ? ` until ${formatMoment ? formatMoment(entry.notBefore) : entry.notBefore}` : ""}`;
  });
}
