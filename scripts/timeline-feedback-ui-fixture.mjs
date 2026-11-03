// Accountless rendering/interaction fixture. Run under the Go-owned process tree.
import assert from "node:assert/strict";
import { createServer } from "node:http";
import { readFile, writeFile, mkdir } from "node:fs/promises";
import { resolve, join } from "node:path";
import { launchChrome } from "../internal/collection/headless/worker/chrome.mjs";
import { readFrontendAppSource } from "../test/fixtures/frontend-source.mjs";

const [chromePath, root, artifacts, sourcePath] = process.argv.slice(2);
assert.ok(chromePath && root && artifacts);
await mkdir(artifacts, { recursive: true });
const web = resolve(import.meta.dirname, "../internal/httpapi/web");
const app = sourcePath ? await readFile(sourcePath, "utf8") : readFrontendAppSource();
const extracted = app.includes('import { createTimelineFeedbackUI } from "./timeline-feedback.js";');
function source(from, to) {
  const start = app.indexOf(from), end = app.indexOf(to, start + from.length);
  assert.ok(start >= 0 && end > start, from);
  return app.slice(start, end);
}
const functions = source("function feedbackButton(", "function logInteractionTrace(")
  + source("function buildActions(", "function timelineContentContextAnchor(")
  + source("async function api(", "async function bridgeApi(")
  + source("function enforceCurrentSidecarEpoch(", "async function fetchFromSidecar(")
  + source("async function fetchFromSidecar(", "\nsyncRunButtons();")
  + (extracted ? source("const timelineFeedbackUI =", "const timelineScrollIdle =") : source("async function sendFeedback(", "function openMedia("));
const html = `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><link rel="stylesheet" href="/styles.css"></head>
<body style="padding:24px"><main style="max-width:760px;margin:auto"><h1>Timeline feedback QA</h1><p>Synthetic local post. No account or capture is used.</p>
<article class="presentable-item"><div class="item-presentation-toolbar">X · Synthetic author · Brief</div><div class="brief-layout" style="padding:18px"><h2>A bounded local update</h2><p>More/Less changes preference feedback while this card stays in place.</p></div></article>
<p id="notice" role="alert"></p><div style="height:1600px"></div></main><script type="module">
${extracted ? 'import { createTimelineFeedbackUI } from "/timeline-feedback.js";' : ""}
const state = { bootstrap: { instanceEpoch: "fixture-epoch" }, sidecarEpochReloading: false };
window.qa = { errors: [], invalidations: 0, traces: [] };
function invalidateLibraryStorage() { window.qa.invalidations++; }
function showError(error) { window.qa.errors.push(error.message); document.querySelector('#notice').textContent = error.message; }
function logInteractionTrace(action, trace, phase, details) { window.qa.traces.push({ action, trace, phase, ...details }); }
function buildSourceLink() { const a=document.createElement('a'); a.href='#local-evidence'; a.textContent='Open source evidence'; return a; }
function buildTimelineReadLaterAction() { const b=document.createElement('button'); b.type='button'; b.className='feedback-button memory-read-later-button'; b.textContent='Read later'; return b; }
function buildSemanticCorrectionActions() { throw Error('No semantic correction in this fixture'); }
${functions}
const entry = { id: 'item/one', feedback: { direction: 'more', origin: 'calibration' } };
document.querySelector('article').append(buildActions(entry));
window.qa.entry=entry; window.qa.ready=true;
</script></body></html>`;
const requests = [], held = [], moduleRequests = [];
let hold = false, fail = false;
function respond(response, body, status = 200) {
  response.writeHead(status, { "Content-Type": "application/json", "X-Aku-Sidecar-Instance-Epoch": "fixture-epoch" });
  response.end(JSON.stringify(status === 200 ? { feedback: { direction: body.direction, origin: "routine" } } : { error: "fixture_failure", message: "Synthetic feedback failure" }));
}
const server = createServer(async (req, res) => {
  try {
    if (req.url === "/") { res.setHeader("Content-Type", "text/html"); res.end(html); return; }
    if (req.url === "/styles.css" || req.url === "/timeline-feedback.js") {
      moduleRequests.push(req.url); res.setHeader("Content-Type", req.url.endsWith(".css") ? "text/css" : "text/javascript");
      res.end(await readFile(join(web, req.url.slice(1)))); return;
    }
    if (req.url === "/api/timeline/item%2Fone/feedback" && req.method === "POST") {
      let raw = ""; for await (const chunk of req) raw += chunk;
      const body = JSON.parse(raw); assert.ok(req.headers["x-aku-feedback-trace"]);
      assert.equal(body.reason, body.direction === "less" ? "not_interested" : null);
      requests.push({ body, trace: req.headers["x-aku-feedback-trace"] });
      if (hold) held.push({ res, body }); else respond(res, body, fail ? 503 : 200);
      return;
    }
    res.writeHead(404); res.end();
  } catch (error) { res.writeHead(500); res.end(String(error)); }
});
await new Promise(resolve => server.listen(0, "127.0.0.1", resolve));
let browser;
const results = [];
try {
  browser = await launchChrome({ chromePath, profilePath: join(root, "profile") });
  const page = await browser.createTemporaryPage();
  const more = ".feedback-actions > button:first-child", less = ".feedback-actions > button:last-child";
  async function until(expression) {
    for (let i=0; i<100; i++) { if (await page.evaluate(expression)) return; await new Promise(resolve => setTimeout(resolve, 25)); }
    throw Error("Fixture state timeout: " + expression);
  }
  async function waitRequests(count) {
    for (let i=0; i<100; i++) { if (requests.length === count) return; await new Promise(resolve => setTimeout(resolve, 25)); }
    throw Error("Request count mismatch: " + requests.length + " / " + count);
  }
  async function click(selector) {
    const point = await page.evaluate(`(()=>{const r=document.querySelector(${JSON.stringify(selector)}).getBoundingClientRect();return {x:r.x+r.width/2,y:r.y+r.height/2}})()`);
    await page.send("Input.dispatchMouseEvent", { type: "mousePressed", button: "left", clickCount: 1, ...point });
    await page.send("Input.dispatchMouseEvent", { type: "mouseReleased", button: "left", clickCount: 1, ...point });
  }
  async function shot(name) { await writeFile(join(artifacts, name + ".png"), Buffer.from((await page.send("Page.captureScreenshot", { format: "png" })).data, "base64")); }
  for (const [label, width, height] of [["desktop",1100,850],["mobile",390,844]]) {
    fail=false; hold=false; const count=requests.length;
    await page.send("Emulation.setDeviceMetricsOverride", { width, height, deviceScaleFactor: 1, mobile: label === "mobile" });
    await page.navigate(`http://127.0.0.1:${server.address().port}/`); await until("window.qa?.ready === true");
    assert.equal(await page.evaluate(`document.querySelector('${more}').getAttribute('aria-pressed')`), "true");
    assert.equal(await page.evaluate(`document.querySelector('${more}').title`), "Chosen during onboarding calibration");
    await shot(label + "-initial");
    await click(less); await until("window.qa.entry.feedback.direction === 'less'");
    assert.equal(await page.evaluate("window.qa.invalidations"), 1);
    fail=true; await click(more); await until("window.qa.errors.length === 1"); fail=false;
    assert.equal(await page.evaluate("window.qa.entry.feedback.direction"), "less");
    hold=true; await click(less); await click(more); await waitRequests(count+4);
    assert.equal(held.length, 2);
    assert.equal(await page.evaluate(`document.querySelector('${more}').disabled || document.querySelector('${less}').disabled`), false);
    respond(held[1].res, held[1].body); await until("window.qa.entry.feedback.direction === 'more'");
    respond(held[0].res, held[0].body); await until("window.qa.entry.feedback.direction === 'less'"); held.length=0; hold=false;
    await page.evaluate(`document.querySelector('${more}').focus()`);
    await page.send("Input.dispatchKeyEvent", { type: "keyDown", key: "Enter", code: "Enter", text: "\r", unmodifiedText: "\r", windowsVirtualKeyCode: 13 });
    await page.send("Input.dispatchKeyEvent", { type: "keyUp", key: "Enter", code: "Enter", windowsVirtualKeyCode: 13 });
    await until("window.qa.entry.feedback.direction === 'more'"); await waitRequests(count+5);
    assert.equal(await page.evaluate(`document.activeElement === document.querySelector('${more}')`), true);
    assert.equal(await page.evaluate("document.documentElement.scrollWidth <= innerWidth"), true);
    assert.equal(await page.evaluate("window.scrollY"), 0);
    assert.equal(await page.evaluate("window.qa.invalidations"), 4);
    const geometry = await page.evaluate(`(()=>{const a=document.querySelector('.timeline-primary-actions').getBoundingClientRect(), b=document.querySelector('.feedback-actions').getBoundingClientRect();return {primaryBottom:a.bottom,feedbackTop:b.top,primaryRight:a.right,feedbackLeft:b.left}})()`);
    assert.ok(label === "mobile" ? geometry.feedbackTop >= geometry.primaryBottom : geometry.feedbackLeft >= geometry.primaryRight);
    await shot(label + "-selected");
    results.push({ viewport:label,width,height,requests:5,keyboard:true,concurrency:true,error:true,focus:true,noOverflow:true,geometry });
  }
  if (extracted) assert.equal(moduleRequests.filter(x => x === "/timeline-feedback.js").length, 2);
  await page.close();
  await writeFile(join(artifacts,"result.json"),JSON.stringify({status:"passed",extracted,results,requests},null,2));
  process.stdout.write(JSON.stringify({status:"passed",extracted,results}));
} finally {
  for(const item of held) item.res.destroy();
  if(browser) await browser.close();
  await new Promise(resolve=>server.close(resolve));
}
