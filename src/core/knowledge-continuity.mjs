import { createHash } from "node:crypto";

export function evidenceKeyForBlock(source, block) {
  const identity = block?.platformId || block?.permalink || normalizeEvidenceText(block?.text);
  if (!identity) return "";
  const digest = createHash("sha256").update(`${source}\n${identity}`).digest("hex").slice(0, 24);
  return `${source}:${digest}`;
}

export function uniqueEvidenceKeys(observation) {
  return [
    ...new Set(
      observation.snapshots
        .flatMap((snapshot) => snapshot.blocks)
        .map((block) => block.evidenceKey)
        .filter(Boolean),
    ),
  ];
}

export function filterKnownEvidence(observation, knownEvidenceKeys) {
  const known = knownEvidenceKeys instanceof Set ? knownEvidenceKeys : new Set(knownEvidenceKeys);
  const suppressedKeys = new Set();
  const snapshots = observation.snapshots.map((snapshot) => ({
    ...snapshot,
    blocks: snapshot.blocks.filter((block) => {
      if (!known.has(block.evidenceKey)) return true;
      suppressedKeys.add(block.evidenceKey);
      return false;
    }),
  }));
  const filtered = { ...observation, snapshots };
  return {
    observation: filtered,
    exactDuplicatesSuppressed: suppressedKeys.size,
    unseenEvidenceCount: uniqueEvidenceKeys(filtered).length,
  };
}

export function normalizeEventKey(value) {
  if (typeof value !== "string") return "";
  return value
    .toLowerCase()
    .trim()
    .replace(/[^a-z0-9._:-]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 200);
}

function normalizeEvidenceText(value) {
  return typeof value === "string"
    ? value.replace(/\s+/g, " ").trim().toLowerCase().slice(0, 4_000)
    : "";
}
