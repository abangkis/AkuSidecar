export const LIVING_TOPIC_MAX_NAME = 120;
export const LIVING_TOPIC_MAX_MEMBERS = 20;

export function normalizeLivingTopicName(value) {
  return Array.from(String(value ?? "").trim()).slice(0, LIVING_TOPIC_MAX_NAME).join("");
}

export function buildLivingTopicsPath() {
  return "/api/living-topics";
}

export function buildLivingTopicPath(id) {
  const value = String(id ?? "").trim();
  return value ? `/api/living-topics/${encodeURIComponent(value)}` : "";
}

export function buildLivingTopicMembersPath(id) {
  const base = buildLivingTopicPath(id);
  return base ? `${base}/members` : "";
}

export function buildLivingTopicMemberPath(id, memoryItemId) {
  const base = buildLivingTopicMembersPath(id);
  const memory = String(memoryItemId ?? "").trim();
  return base && memory ? `${base}/${encodeURIComponent(memory)}` : "";
}

export function buildLivingTopicSnapshotsPath(id) {
  const base = buildLivingTopicPath(id);
  return base ? `${base}/snapshots` : "";
}

export function livingTopicStatusLabel(status) {
  if (status === "ready") return "Snapshot ready";
  if (status === "no_change") return "No evidence changed";
  if (status === "insufficient_evidence") return "More evidence needed";
  return "Snapshot status unknown";
}
