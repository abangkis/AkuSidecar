export const LIVING_TOPIC_MAX_NAME = 120;
export const LIVING_TOPIC_MAX_MEMBERS = 20;
export const LIVING_TOPIC_TAB_SNAPSHOT = "snapshot";
export const LIVING_TOPIC_TAB_EVIDENCE = "evidence";

export function normalizeLivingTopicTab(value) {
  return value === LIVING_TOPIC_TAB_EVIDENCE ? LIVING_TOPIC_TAB_EVIDENCE : LIVING_TOPIC_TAB_SNAPSHOT;
}

export function normalizeLivingTopicName(value) {
  return Array.from(String(value ?? "").trim()).slice(0, LIVING_TOPIC_MAX_NAME).join("");
}

export function buildLivingTopicsPath() {
  return "/api/living-topics";
}

export function buildLivingTopicNotificationsPath() {
  return "/api/living-topics/notifications";
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

export function buildLivingTopicMemberMovePath(id, memoryItemId) {
  const base = buildLivingTopicMemberPath(id, memoryItemId);
  return base ? `${base}/move` : "";
}

export function buildLivingTopicMoveUndoPath(moveId) {
  const value = String(moveId ?? "").trim();
  return value ? `/api/living-topic-moves/${encodeURIComponent(value)}/undo` : "";
}

export function buildLivingTopicSnapshotsPath(id) {
  const base = buildLivingTopicPath(id);
  return base ? `${base}/snapshots` : "";
}

export function buildLivingTopicActivationPath(id) {
  const base = buildLivingTopicPath(id);
  return base ? `${base}/activation` : "";
}

export function buildLivingTopicSeenPath(id) {
  const base = buildLivingTopicPath(id);
  return base ? `${base}/seen` : "";
}

export function normalizeLivingTopicNotifications(value) {
  const count = Math.max(0, Math.trunc(Number(value?.newEvidenceCount) || 0));
  const topics = Math.max(0, Math.trunc(Number(value?.topicsWithNewEvidence) || 0));
  return {
    newEvidenceCount: count,
    topicsWithNewEvidence: topics,
    latestEvidenceAt: String(value?.latestEvidenceAt || ""),
  };
}

export function buildLivingTopicCandidateActionPath(id, memoryItemId, action) {
  const base = buildLivingTopicPath(id);
  const memory = String(memoryItemId ?? "").trim();
  const normalizedAction = ["accept", "reject", "undo"].includes(action) ? action : "";
  return base && memory && normalizedAction ? `${base}/candidates/${encodeURIComponent(memory)}/${normalizedAction}` : "";
}

export function livingTopicStatusLabel(status) {
  if (status === "ready") return "Current understanding";
  if (status === "no_change") return "No evidence changed";
  if (status === "insufficient_evidence") return "More evidence needed";
  return "Understanding status unknown";
}

export function livingTopicUnderstandingLabel(status) {
  if (status === "pending") return "Refresh queued";
  if (status === "running") return "Updating understanding";
  if (status === "current") return "Understanding current";
  if (status === "insufficient_evidence") return "Needs evidence";
  if (status === "failed") return "Refresh needs attention";
  return "Waiting for evidence";
}
