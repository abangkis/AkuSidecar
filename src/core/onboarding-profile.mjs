import { ContractError } from "./contracts.mjs";

const SETTING_KEY = "user.onboarding_profile_v0";
const CONTENT_TYPES = new Set([
  "announcement",
  "tutorial",
  "opinion",
  "research",
  "opportunity",
  "discovery",
]);
const SOURCES = new Set(["x", "linkedin"]);

export function getOnboardingProfile(store) {
  const raw = store.getSetting(SETTING_KEY);
  if (!raw) return { status: "not_started", profile: null };
  try {
    const profile = JSON.parse(raw);
    return profile?.status === "completed"
      ? { status: "completed", profile }
      : { status: "not_started", profile: null };
  } catch {
    return { status: "not_started", profile: null };
  }
}

export function saveOnboardingProfile(store, input, now = new Date()) {
  const interestStatements = cleanStrings(input?.interestStatements, 10, 280);
  const topicSeeds = cleanStrings(input?.topicSeeds, 20, 80);
  const preferredContentTypes = cleanEnumList(
    input?.preferredContentTypes,
    CONTENT_TYPES,
    "preferredContentTypes",
  );
  const activeSources = cleanEnumList(input?.activeSources, SOURCES, "activeSources");

  if (interestStatements.length === 0 && topicSeeds.length === 0) {
    throw new ContractError("Onboarding needs at least one explicit interest or topic seed.");
  }
  if (preferredContentTypes.length === 0) {
    throw new ContractError("Choose at least one preferred content form.");
  }
  if (activeSources.length === 0) {
    throw new ContractError("Choose at least one active source.");
  }

  const profile = {
    version: 0,
    status: "completed",
    origin: "explicit_onboarding",
    interestStatements,
    topicSeeds,
    preferredContentTypes,
    activeSources,
    completedAt: now.toISOString(),
  };
  store.setSetting(SETTING_KEY, JSON.stringify(profile));
  return { status: "completed", profile };
}

function cleanStrings(value, maximum, maximumLength) {
  if (!Array.isArray(value)) return [];
  return [...new Set(value.map((item) => String(item).trim()).filter(Boolean))]
    .slice(0, maximum)
    .map((item) => item.slice(0, maximumLength));
}

function cleanEnumList(value, allowed, field) {
  if (!Array.isArray(value)) return [];
  const cleaned = [...new Set(value.map((item) => String(item).trim().toLowerCase()))];
  const invalid = cleaned.find((item) => !allowed.has(item));
  if (invalid) throw new ContractError(`${field} contains an unsupported value.`, { value: invalid });
  return cleaned;
}
