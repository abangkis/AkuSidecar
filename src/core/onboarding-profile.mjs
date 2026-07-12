import { ContractError } from "./contracts.mjs";

const SETTING_KEY = "user.onboarding_profile_v0";
const SOURCES = new Set(["x", "linkedin"]);
const INTERESTS = new Set([
  "ai",
  "technology",
  "software_development",
  "science",
  "gaming",
  "comedy",
  "entertainment",
  "beauty_style",
  "art_design",
  "business",
  "finance",
  "sports",
  "food",
  "travel",
  "music",
  "health_fitness",
  "news_current_events",
  "culture",
  "education",
]);

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
  const selectedInterests = cleanEnumList(input?.selectedInterests, INTERESTS, "selectedInterests");
  const activeSources = cleanEnumList(input?.activeSources, SOURCES, "activeSources");

  if (selectedInterests.length === 0) {
    throw new ContractError("Choose at least one interest.");
  }
  if (selectedInterests.length > 5) {
    throw new ContractError("Choose no more than five interests.");
  }
  if (activeSources.length === 0) {
    throw new ContractError("Choose at least one active source.");
  }

  const profile = {
    version: 0,
    status: "completed",
    origin: "explicit_onboarding",
    selectedInterests,
    activeSources,
    completedAt: now.toISOString(),
  };
  store.setSetting(SETTING_KEY, JSON.stringify(profile));
  return { status: "completed", profile };
}

function cleanEnumList(value, allowed, field) {
  if (!Array.isArray(value)) return [];
  const cleaned = [...new Set(value.map((item) => String(item).trim().toLowerCase()))];
  const invalid = cleaned.find((item) => !allowed.has(item));
  if (invalid) throw new ContractError(`${field} contains an unsupported value.`, { value: invalid });
  return cleaned;
}
