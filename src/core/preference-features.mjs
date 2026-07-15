export const CANONICAL_TOPIC_FACETS = Object.freeze([
  "ai_models",
  "software_engineering",
  "developer_tools",
  "security",
  "data_infrastructure",
  "geospatial",
  "science",
  "space",
  "business",
  "finance",
  "policy",
  "education",
  "health",
  "climate_energy",
  "culture_entertainment",
  "sports",
  "career_hiring",
  "other",
]);

export const PREFERENCE_CONTINUOUS_FIELDS = Object.freeze([
  "novelty",
  "urgency",
  "actionability",
  "materiality",
  "evidenceStrength",
]);

const FACETS = new Set(CANONICAL_TOPIC_FACETS);
const TAG_RULES = [
  ["ai_models", /\b(ai|llm|model|openai|anthropic|gemini|agent|machine learning)\b/i],
  ["software_engineering", /\b(software|engineering|developer|programming|code|api|framework)\b/i],
  ["developer_tools", /\b(devtool|tooling|sdk|ide|github|git|compiler|runtime)\b/i],
  ["security", /\b(security|privacy|vulnerability|cyber|auth|encryption)\b/i],
  ["data_infrastructure", /\b(data|database|cloud|infrastructure|distributed|warehouse|storage)\b/i],
  ["geospatial", /\b(geo|geospatial|gis|map|mapping|satellite)\b/i],
  ["space", /\b(space|nasa|rocket|orbit|moon|mars|artemis)\b/i],
  ["science", /\b(science|research|physics|chemistry|biology)\b/i],
  ["finance", /\b(finance|market|stock|bank|economy|investment)\b/i],
  ["policy", /\b(policy|government|regulation|law|politic)\b/i],
  ["education", /\b(education|learning|course|school|university)\b/i],
  ["health", /\b(health|medical|medicine|biotech)\b/i],
  ["climate_energy", /\b(climate|energy|battery|electric|solar|carbon)\b/i],
  ["culture_entertainment", /\b(culture|entertainment|movie|music|game|art)\b/i],
  ["sports", /\b(sport|football|soccer|basketball|tennis)\b/i],
  ["career_hiring", /\b(hiring|career|job|intern|recruit)\b/i],
  ["business", /\b(business|company|startup|enterprise|product|sales)\b/i],
];

export function normalizePreferenceAssessment(assessment = {}) {
  const novelty = score(assessment.novelty, 0.5);
  const urgency = score(assessment.urgency, 0.25);
  const actionability = score(assessment.actionability, 0.35);
  return {
    ...assessment,
    topicTags: uniqueStrings(assessment.topicTags, 5),
    topicFacets: canonicalTopicFacets(assessment.topicFacets, assessment.topicTags),
    novelty,
    urgency,
    actionability,
    materiality: score(assessment.materiality, (novelty + actionability) / 2),
    evidenceStrength: score(assessment.evidenceStrength, 0.5),
  };
}

export function canonicalTopicFacets(facets, tags = []) {
  const explicit = uniqueStrings(facets, 3).filter((facet) => FACETS.has(facet));
  if (explicit.length) return explicit;
  const matched = [];
  for (const tag of uniqueStrings(tags, 10)) {
    const rule = TAG_RULES.find(([, pattern]) => pattern.test(tag));
    if (rule && !matched.includes(rule[0])) matched.push(rule[0]);
    if (matched.length === 3) break;
  }
  return matched.length ? matched : ["other"];
}

export function genericMaterialityScore(assessment = {}) {
  const value = normalizePreferenceAssessment(assessment);
  return clamp(
    value.materiality * 0.4 +
    value.novelty * 0.2 +
    value.actionability * 0.15 +
    value.urgency * 0.1 +
    value.evidenceStrength * 0.15,
    0,
    1,
  );
}

export function preferenceFeedbackWeight(signal) {
  if (signal.kind === "neutral") return 0.75;
  if (signal.kind === "more_like_this") return signal.origin === "calibration" ? 1.1 : 1;
  if (signal.kind !== "less_like_this") return 0;
  if (["not_interested", "wrong_topic"].includes(signal.reasonCode)) return 1;
  if (!signal.reasonCode) return 0.5;
  return 0;
}

export function feedbackLaneForReason(kind, reasonCode) {
  if (kind === "more_like_this" || kind === "neutral") return "preference";
  if (kind !== "less_like_this") return "ignored";
  if (["not_interested", "wrong_topic"].includes(reasonCode)) return "preference";
  if (reasonCode === "already_known") return "continuity";
  if (reasonCode === "duplicate") return "deduplication";
  if (reasonCode === "stale_or_superseded") return "recency";
  return "ambiguous_preference";
}

function uniqueStrings(values, limit) {
  return [...new Set((Array.isArray(values) ? values : [])
    .map((value) => String(value ?? "").trim().toLowerCase())
    .filter(Boolean))].slice(0, limit);
}

function score(value, fallback) {
  return Number.isFinite(value) ? clamp(value, 0, 1) : fallback;
}

function clamp(value, minimum, maximum) {
  return Math.min(maximum, Math.max(minimum, value));
}
