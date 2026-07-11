import { createHash, randomUUID } from "node:crypto";

const HIGH_SIGNAL = /\b(codex|gpt[- ]?5(?:\.6)?|sol|reset|release|released|launch|launched|shipping|available now|security|outage)\b/i;
const OPINION = /\b(opinion|thoughts?|i think|my take|prediction|hot take)\b/i;

export class DeterministicReasoningProvider {
  name = "deterministic-development-fallback";

  async planAcquisition() {
    return {
      decision: "finish",
      reason: "The deterministic development provider never requests additional browser acquisition.",
    };
  }

  async analyze({ run, observation }) {
    const candidates = uniqueBlocks(observation)
      .filter((block) => block.text.length >= 40)
      .slice(0, run.maxItems);

    const items = candidates.map((block) => {
      const sourceUrlKind = block.permalink ? "native_post" : "source_page";
      return {
        id: stableId(block.text) || randomUUID(),
        priority: classifyPriority(block.text),
        whatChanged: summarize(block.text),
        whyItMatters:
          "Development fallback only: this item was selected to verify the transport and result contract, not to make a trusted relevance judgment.",
        source: run.source,
        sourceUrl: block.permalink || observation.pageUrl,
        sourceUrlKind,
        evidenceKey: block.evidenceKey,
        eventKey: `evidence:${stableId(block.permalink || block.text)}`,
        knowledgeDelta: "new_event",
        author: block.author || "",
        publishedAt: block.publishedAt || null,
        confidence: 0.25,
        evidenceState: "unverified",
      };
    });

    return {
      summary:
        items.length > 0
          ? `Development fallback produced ${items.length} bounded item(s). Codex reasoning was not used.`
          : "No visible candidate block met the minimum development threshold.",
      items,
      candidateAssessments: uniqueBlocks(observation).map((block) => ({
        evidenceKey: block.evidenceKey,
        topicTags: [],
        contentType: "other",
        recommendedPriority: classifyPriority(block.text),
        intentRelevance: 0.5,
        novelty: 0.5,
        urgency: 0.2,
        actionability: 0.2,
        rationale: "Deterministic development fallback assessment.",
      })),
      repeatedClaimsCollapsed: countBlocks(observation) - uniqueBlocks(observation).length,
      deferredByBudget: Math.max(0, uniqueBlocks(observation).length - run.maxItems),
      limitations: [
        "Deterministic development fallback; prioritization is not suitable for pilot evaluation.",
      ],
    };
  }
}

function uniqueBlocks(observation) {
  const seen = new Set();
  const result = [];
  for (const block of observation.snapshots.flatMap((snapshot) => snapshot.blocks)) {
    const key = block.text.toLowerCase().replace(/\s+/g, " ").trim();
    if (!key || seen.has(key)) continue;
    seen.add(key);
    result.push(block);
  }
  return result;
}

function countBlocks(observation) {
  return observation.snapshots.reduce((sum, snapshot) => sum + snapshot.blocks.length, 0);
}

function classifyPriority(text) {
  if (HIGH_SIGNAL.test(text)) return "P1";
  if (OPINION.test(text)) return "P2";
  return "P3";
}

function summarize(text) {
  const compact = text.replace(/\s+/g, " ").trim();
  return compact.length <= 320 ? compact : `${compact.slice(0, 317)}...`;
}

function stableId(text) {
  return createHash("sha256").update(text).digest("hex").slice(0, 16);
}
