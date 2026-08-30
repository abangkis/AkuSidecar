import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const index = readFileSync(new URL("../internal/httpapi/web/index.html", import.meta.url), "utf8");
const app = readFileSync(new URL("../internal/httpapi/web/app.js", import.meta.url), "utf8");
const styles = readFileSync(new URL("../internal/httpapi/web/styles.css", import.meta.url), "utf8");

test("Library exposes a compact accessible filter disclosure", () => {
  assert.match(index, /id="library-filters-toggle"[^>]+aria-expanded="false"[^>]+aria-controls="library-filter-fields"/);
  assert.match(index, /id="library-filter-fields" class="library-filter-fields hidden"/);
  assert.match(app, /function toggleLibraryFilters\(\)/);
  assert.match(app, /fields\.classList\.toggle\("hidden", !expanded\)/);
  assert.match(app, /toggle\.setAttribute\("aria-expanded", String\(!fields\.classList\.contains\("hidden"\)\)\)/);
});

test("Library search surfaces current Living Topic knowledge separately", () => {
  assert.match(index, /id="library-topic-knowledge"[^>]*aria-labelledby="library-topic-knowledge-heading"/);
  assert.match(index, /id="library-topic-knowledge-results"[^>]*role="list"/);
  assert.match(index, /Only current supported understanding is searched/);
  assert.match(app, /topicKnowledge: \[\]/);
  assert.match(app, /buildLibraryRequestPath\([\s\S]*?false,\s*true,\s*\)/);
  assert.match(app, /function renderLibraryTopicKnowledge\(\)/);
  assert.match(app, /function buildLibraryTopicKnowledgeCard\(knowledge\)/);
  assert.match(app, /knowledge\.claims\.slice\(0, 3\)/);
  assert.match(styles, /\.library-topic-knowledge \{/);
  assert.match(styles, /\.library-topic-knowledge-results \{[^}]*grid-template-columns: repeat\(2, minmax\(0, 1fr\)\)/);
});

test("Library storage summary and Spring Cleaning remain read-only and review-only", () => {
  assert.match(index, /id="library-tabs"[^>]+role="tablist"/);
  assert.match(index, /id="library-saved-tab"[^>]+role="tab"[^>]+aria-selected="true"[^>]+aria-controls="library-saved-view"/);
  assert.match(index, /id="library-library-tab"[^>]+role="tab"[^>]+aria-selected="false"[^>]+aria-controls="library-saved-view"/);
  assert.match(index, /id="library-cleaning-tab"[^>]+role="tab"[^>]+aria-selected="false"[^>]+aria-controls="library-cleaning-view"/);
  assert.match(index, /id="library-saved-view"[^>]+role="tabpanel"/);
  assert.match(index, /id="library-cleaning-view"[^>]+class="library-tabpanel hidden"[^>]+role="tabpanel"[^>]+[^>]*hidden/);
  assert.match(index, /id="library-storage-summary"[^>]+role="status"/);
  assert.match(index, /id="library-saved-pressure-heading">Saved backlog<\/h3>/);
  assert.match(index, /id="library-saved-pressure-summary"[^>]+role="status"/);
  assert.match(index, /id="library-saved-recommendations"[^>]+role="list"/);
  assert.match(index, /id="library-spring-cleaning-heading">Spring Cleaning<\/h3>/);
  assert.match(index, /Nothing is removed automatically/);
  assert.match(app, /buildLibraryStorageRequestPath\(\)/);
  assert.match(app, /async function loadLibraryStorage\(\)/);
  assert.match(app, /function setLibraryTab\(tab, focus = false\)/);
  assert.match(app, /function handleLibraryTabKeydown\(event\)/);
  assert.match(app, /function maybeLoadLibraryStorage\(\)/);
  const libraryLoaderStart = app.indexOf("async function loadLibrary({ append = false } = {})");
  const storageLoaderStart = app.indexOf("async function loadLibraryStorage()");
  assert.ok(libraryLoaderStart >= 0 && storageLoaderStart > libraryLoaderStart);
  assert.doesNotMatch(app.slice(libraryLoaderStart, storageLoaderStart), /loadLibraryStorage/);
  assert.match(app, /reclaimableBytes/);
  assert.match(app, /review_full_copy/);
  assert.match(app, /selectLibraryItem\(recommendation\.id/);
  assert.match(app, /function reviewLibraryStorageRecommendation\(recommendation\)[\s\S]*setLibraryTab\(LIBRARY_TAB_LIBRARY\)[\s\S]*requestAnimationFrame/);
  assert.match(app, /function buildLibrarySavedPressure\(pressure\)/);
  assert.match(app, /\["Local-copy Saved", formatLibraryStorageCount\(pressure\.localCopyItems\)\]/);
  assert.match(app, /\["Source-dependent Saved", formatLibraryStorageCount\(pressure\.sourceDependentItems\)\]/);
  assert.match(app, /function buildLibrarySavedRecommendation\(recommendation\)/);
  assert.match(app, /function reviewSavedRecommendation\(recommendation\)[\s\S]*setLibraryTab\(LIBRARY_TAB_SAVED\)[\s\S]*requestAnimationFrame/);
  assert.match(app, /review\.dataset\.reviewAction = recommendation\.reviewAction \|\| "review_saved"/);
  assert.match(styles, /\.library-storage-breakdown \{[^}]*grid-template-columns/);
  assert.match(styles, /\.library-storage-recommendations \{[^}]*grid-template-columns/);
});

test("Library storage state invalidates after memory lifecycle success", () => {
  assert.match(app, /function invalidateLibraryStorage\(\) \{\s*resetLibraryStorage\(\);/);
  assert.match(app, /entry\.personalMemory = \{\s*retentionTier: response\.retentionTier,\s*saved: true,\s*permanentKeep: response\.permanentKeep === true,\s*\};\s*invalidateLibraryStorage\(\);/);
  assert.match(app, /async function sendFeedback\(id, direction, reason\)[\s\S]*invalidateLibraryStorage\(\);/);
  assert.match(app, /state\.library\.detailError = null;\s*}\s*reloadLibraryStorage\(\);/);
  assert.match(app, /function refreshLibrary\(\)[\s\S]*reloadLibraryStorage\(\);/);
});

test("Library switches between full-width grid and adaptive master-detail", () => {
  assert.match(app, /layout\?\.classList\.toggle\("has-selection", Boolean\(state\.library\.selectedId\)\)/);
  assert.match(styles, /\.library-results \{[^}]*grid-template-columns: repeat\(2, minmax\(0, 1fr\)\)/);
  assert.match(styles, /\.library-layout\.has-selection \{[^}]*grid-template-columns: minmax\(250px, 0\.72fr\) minmax\(360px, 1\.28fr\)/);
  assert.match(styles, /\.library-layout\.has-selection \.library-results \{[^}]*grid-template-columns: 1fr/);
  assert.match(styles, /\.library-layout\.has-selection \{ grid-template-columns: 1fr; \}/);
  assert.match(styles, /\.library-layout\.has-selection \.library-detail \{ order: 1; \}/);
  assert.match(styles, /\.library-layout\.has-selection \.library-results-column \{ order: 2; \}/);
});

test("Timeline actions keep primary, preference, and provenance controls distinct", () => {
  assert.match(app, /primary\.className = "timeline-primary-actions"/);
  assert.match(app, /actions\.append\(primary, feedback\)/);
  assert.match(app, /save\.className = "feedback-button memory-read-later-button"/);
  assert.match(app, /save\.textContent = saved \? "Saved" : busy \? "Adding…" : "Read later"/);
  assert.match(app, /buildTimelineReadLaterPath\(entry\.id\)/);
  assert.doesNotMatch(app, /keep\.className = "feedback-button memory-keep-button"/);
  assert.match(app, /statusTimer = window\.setTimeout\(\(\) => \{/);
  assert.match(styles, /\.semantic-correction-actions \{[^}]*grid-column: 1 \/ -1/);
});
