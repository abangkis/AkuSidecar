import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { projectRoot } from "../src/config.mjs";
import { createAkuBrowserApp } from "../src/http/app.mjs";
import { attachViteFrontend } from "../src/http/vite-frontend.mjs";
import { SqliteStateStore } from "../src/store/sqlite-state-store.mjs";

test("Vite middleware and the Sidecar API share one HTTP port", async (context) => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), "aku-sidecar-vite-"));
  const databasePath = path.join(directory, "state.db");
  const store = new SqliteStateStore(databasePath);
  const config = {
    host: "127.0.0.1",
    port: 0,
    publicDirectory: path.join(projectRoot, "public"),
    databasePath,
    presentation: {
      defaultLayout: "source",
      homePresentation: "timeline",
      timelineCapacity: 12,
      streamWidth: "social",
      telemetryBehavior: "flow",
    },
    limits: {
      maxBodyBytes: 1_000_000,
      maxItems: 5,
      maxScrolls: 2,
      maxAcquisitionRounds: 2,
      followUpScrolls: 1,
      maxContinuationAnchors: 3,
      defaultScrolls: 2,
      scrollFraction: 0.75,
      scrollSettleMs: 900,
      captureTimeoutMs: 45_000,
      pendingContentTimeoutMs: 5_000,
      pendingContentSettleMs: 700,
      maxBlocksPerSnapshot: 20,
      maxBlockCharacters: 4_000,
    },
  };
  const app = createAkuBrowserApp({
    config,
    store,
    reasoningProvider: { name: "vite-test-provider" },
    logger: { error() {} },
  });
  context.after(async () => {
    await app.stop();
    store.close();
    fs.rmSync(directory, { recursive: true, force: true });
  });

  await attachViteFrontend(app, config);
  const address = await app.start();
  const origin = `http://127.0.0.1:${address.port}`;

  const htmlResponse = await fetch(`${origin}/`);
  const html = await htmlResponse.text();
  const appScript = await (await fetch(`${origin}/app.js`)).text();
  const styles = await (await fetch(`${origin}/styles.css`)).text();
  const bootstrap = await (await fetch(`${origin}/api/bootstrap`)).json();
  const shadowComparison = await (
    await fetch(`${origin}/api/preferences/shadow-comparison?limit=10&offset=20`)
  ).json();
  const databaseHealthResponse = await fetch(`${origin}/api/operations/database/health`);
  const databaseHealth = await databaseHealthResponse.json();

  assert.equal(htmlResponse.status, 200);
  assert.match(html, /\/\@vite\/client/);
  assert.match(html, /\/app\.js\?runtime=bridge-relay-handshake-v2/);
  assert.match(appScript, /bootstrapRetryTimer = setTimeout/);
  assert.match(appScript, /if \(!bridgePingStarted\)/);
  assert.ok(
    appScript.indexOf("let bridgeActionLoopStarted = false") < appScript.indexOf("await bootstrap()"),
    "the bridge action loop state must be initialized before bootstrap starts",
  );
  assert.equal(
    appScript.match(/startBridgeActionLoop\(\);/g)?.length,
    1,
    "only a compatible AkuBridge handshake may start the cooperative action relay",
  );
  assert.match(
    appScript,
    /state\.bridgeReady = compatibility\?\.compatible === true;\s*if \(state\.bridgeReady\) startBridgeActionLoop\(\);/,
  );
  assert.match(appScript, /AkuBridge could not complete the requested operation/);
  assert.match(html, /Check for updates/);
  assert.match(
    styles,
    /#timeline-runner-button \{[^}]*flex: 0 0 170px;[^}]*width: 170px;[^}]*height: 44px;[^}]*white-space: nowrap;/s,
  );
  assert.doesNotMatch(
    appScript,
    /timelineRunnerButton\.textContent = "Check for updates"/,
    "timeline polling must not rewrite the unchanged button label",
  );
  assert.match(html, /id="back-to-top"/);
  assert.match(html, /aria-label="Back to top"/);
  assert.match(html, /id="app-heading" tabindex="-1"/);
  assert.match(appScript, /BACK_TO_TOP_THRESHOLD_PX = 480/);
  assert.match(appScript, /addEventListener\("scroll", scheduleBackToTopVisibility, \{ passive: true \}\)/);
  assert.match(appScript, /addEventListener\("resize", scheduleBackToTopVisibility, \{ passive: true \}\)/);
  assert.match(appScript, /Math\.round\(anchorRect\.right \+ gap\)/);
  assert.match(appScript, /backToTopButton\.style\.right = "auto"/);
  assert.match(appScript, /showSessionView\(\)[\s\S]*syncTimelineChrome\(\);\s*syncBackToTopVisibility\(\);/);
  assert.match(appScript, /window\.scrollTo\(\{ top: 0, behavior: reducedMotion \? "auto" : "smooth" \}\)/);
  assert.match(appScript, /elements\.appHeading\.focus\(\{ preventScroll: true \}\)/);
  assert.match(styles, /\.back-to-top \{[^}]*position: fixed;[^}]*z-index: 60;/s);
  assert.match(html, /class="processing-panel update-progress hidden"/);
  assert.match(html, /id="processing-detail">1\/12 steps/);
  assert.doesNotMatch(html, /class="run-contract"/);
  assert.doesNotMatch(html, /id="source-progress"/);
  assert.match(html, /FINITE KNOWLEDGE TIMELINE/);
  assert.doesNotMatch(html, /id="overview-view-button"/);
  assert.match(html, /class="overview-sources source-settings"/);
  assert.match(html, /Engine constraints/);
  assert.match(html, /id="max-items-per-source"/);
  assert.match(html, /id="fixed-engine-constraints"/);
  assert.doesNotMatch(html, /id="home-presentation"/);
  assert.match(html, /id="timeline-capacity"/);
  assert.doesNotMatch(html, /id="run-form"/);
  assert.match(appScript, /startExternalSessionDiscovery/);
  assert.match(appScript, /\/api\/sessions\/active/);
  assert.match(appScript, /\/api\/sessions\?limit=1&offset=0/);
  assert.match(appScript, /loadTimelineFeed/);
  assert.match(appScript, /\/api\/timeline\?limit=/);
  assert.match(appScript, /renderOverviewSources/);
  assert.match(appScript, /buildXQuotedPostCard/);
  assert.match(appScript, /candidate\.quotedPost/);
  assert.match(appScript, /buildXQuotedPostCard\(quotedPost, candidate\.sourceUrl\)/);
  assert.match(
    appScript,
    /safeNativePostUrl\(quotedPost\.permalink, "x"\)\s*\|\|\s*safeNativePostUrl\(parentSourceUrl, "x"\)/s,
  );
  assert.match(styles, /x-quote-card > header/);
  assert.match(appScript, /renderFixedEngineConstraints/);
  assert.match(appScript, /timeline-new-item/);
  assert.match(styles, /\.update-progress \{[^}]*position: sticky/s);
  assert.match(styles, /\.result-item\.timeline-new-item/);
  assert.match(appScript, /body: JSON\.stringify\(\{\}\)/);
  assert.match(appScript, /if \(firstCompletion\) await startRun\(\)/);
  assert.match(html, /Choose where AkuBrowser should look/);
  assert.match(html, /id="calibration-panel"/);
  assert.match(html, /id="calibration-enabled"/);
  assert.match(html, /id="calibration-batch-size"/);
  assert.match(appScript, /startPendingFirstCalibration/);
  assert.match(appScript, /more_like_this/);
  assert.match(appScript, /less_like_this/);
  assert.match(appScript, /Optional: why less\?/);
  assert.match(appScript, /await savePreferenceFeedback\(run, evidenceKey, kind, null, onSaved\)/);
  assert.match(appScript, /button\.disabled = kind !== "less_like_this"/);
  assert.doesNotMatch(appScript, /\[null, "Just less"\]/);
  assert.match(styles, /\.result-actions \{[^}]*display: grid;[^}]*grid-template-columns: minmax\(8rem, 1fr\) auto;/s);
  assert.match(styles, /\.preference-reason-menu \{[^}]*grid-column: 1 \/ -1;/s);
  assert.match(styles, /\.feedback-button\.selected \{[^}]*background: rgba\(174, 255, 90, 0\.12\);/s);
  assert.doesNotMatch(html, /onboarding-refinement|onboarding-content-types/);
  assert.match(appScript, /showTimelineDuringProcessing/);
  assert.match(appScript, /Reading \$\{label\} source/);
  assert.match(appScript, /\$\{safeStep\}\/\$\{safeTotal\} steps/);
  assert.match(appScript, /REVIEW_PAGE_SIZE = 10/);
  assert.match(appScript, /REVIEW_MAX_RUNS = 50/);
  assert.match(appScript, /IntersectionObserver/);
  assert.match(appScript, /appendPilotRunGroups/);
  assert.match(appScript, /sourceReviewOrder/);
  assert.match(appScript, /sortReviewGroupCards/);
  assert.match(html, /review-scroll-sentinel/);
  assert.match(html, /class="review-layout"/);
  assert.match(html, /class="review-stream"/);
  assert.match(html, /class="review-telemetry"/);
  assert.match(html, /class="review-telemetry" aria-labelledby="telemetry-heading"/);
  assert.match(styles, /grid-template-columns: minmax\(0, var\(--stream-width\)\) minmax\(340px, 380px\)/);
  assert.match(styles, /data-telemetry-behavior=\\?"sticky\\?"[^}]*\.review-telemetry \{[^}]*overflow-y: auto/s);
  assert.match(html, /form="runtime-settings-form">Save settings/);
  assert.match(html, /telemetry-behavior/);
  assert.match(styles, /@media \(max-width: 1050px\)/);
  assert.match(appScript, /mountPilotRunBody/);
  assert.match(appScript, /unmountPilotRunBody/);
  assert.match(appScript, /buildItemPresentation/);
  assert.doesNotMatch(appScript, /Captured evidence · not a live source copy/);
  assert.doesNotMatch(appScript, /Captured in this run/);
  assert.match(appScript, /buildLinkedInAttachment/);
  assert.match(styles, /\.linkedin-attachment > a/);
  assert.match(appScript, /buildSourceLayoutMedia/);
  assert.match(appScript, /document\.createElement\("video"\)/);
  assert.match(appScript, /Play video in native post/);
  assert.match(appScript, /appendLinkedText/);
  assert.match(appScript, /safeNativePostUrl/);
  assert.match(styles, /\.source-layout-video/);
  assert.match(styles, /\.media-viewer-stage img \{[^}]*grid-column: 2;[^}]*grid-row: 1;/s);
  assert.match(styles, /#media-viewer-previous \{[^}]*grid-column: 1;/s);
  assert.match(styles, /#media-viewer-next \{[^}]*grid-column: 3;/s);
  assert.match(styles, /\.source-layout-content a/);
  assert.match(appScript, /referrerPolicy = "no-referrer"/);
  assert.match(appScript, /fitPreferenceExperiment/);
  assert.match(html, /Local personalization/);
  assert.match(html, /Advanced preference diagnostics/);
  assert.match(html, /Eligibility-boundary comparison/);
  assert.match(html, /shadow-candidate-list/);
  assert.match(appScript, /renderShadowCandidates/);
  assert.match(html, /default-presentation/);
  assert.match(htmlResponse.headers.get("content-security-policy"), /ws:\/\/127\.0\.0\.1/);
  assert.match(htmlResponse.headers.get("content-security-policy"), /https:\/\/pbs\.twimg\.com/);
  assert.match(htmlResponse.headers.get("content-security-policy"), /https:\/\/\*\.licdn\.com/);
  assert.equal(bootstrap.provider, "vite-test-provider");
  assert.equal(bootstrap.presentation.defaultLayout, "source");
  assert.equal(bootstrap.presentation.homePresentation, "timeline");
  assert.equal(bootstrap.presentation.timelineCapacity, 12);
  assert.equal(bootstrap.sourceRegistry.length, 2);
  assert.equal(bootstrap.sourceRegistry[0].behavior, "stream");
  assert.equal(bootstrap.presentation.streamWidth, "social");
  assert.equal(bootstrap.presentation.telemetryBehavior, "flow");
  assert.equal(bootstrap.unifiedSession.maxItemsTotal, 10);
  assert.equal(shadowComparison.comparison.available, false);
  assert.equal(shadowComparison.comparison.liveInfluence, false);
  assert.deepEqual(shadowComparison.comparison.pagination, {
    total: 0,
    offset: 20,
    limit: 10,
    returned: 0,
    hasNext: false,
  });
  assert.equal(databaseHealth.database.status, "healthy");
  assert.equal(path.basename(databaseHealth.database.databasePath), "state.db");
  assert.equal(JSON.stringify(databaseHealth).includes(directory), false);
  assert.equal(JSON.stringify(databaseHealth).includes("bridge_token"), false);
  assert.equal(databaseHealthResponse.headers.has("access-control-allow-origin"), false);
});
