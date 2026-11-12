// Isolated browser validation of the real settings renderer and styles.
// The fixture comes from TestCodexModelSettingsBrowserFixture. Saves stay in
// page memory; this server never forwards requests to the active application.
import http from "node:http";
import path from "node:path";
import { readFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";

const root = fileURLToPath(new URL("../../", import.meta.url));
const assets = path.join(root, "internal", "httpapi", "web");
const fixture = path.resolve(process.argv[2] || "");
if (!fixture.startsWith(path.join(root, ".go-build") + path.sep)) throw new Error("A project-local .go-build fixture is required.");
const fixtureBody = await readFile(fixture);
JSON.parse(fixtureBody);

const html = `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>AkuBrowser — isolated model settings</title><link rel="stylesheet" href="/styles.css"></head><body>
<main class="shell"><section class="settings-panel panel" aria-label="Isolated settings preview"><header class="section-heading"><div><p class="eyebrow">AKUBROWSER SETTINGS</p><h2>Reasoning processes</h2><p class="settings-detail">Isolated validation preview</p></div><button type="submit" form="runtime-settings-form" id="save-runtime-settings" disabled>Save settings</button></header>
<form id="runtime-settings-form"><div id="reasoning-processes"></div><div class="settings-row"><span id="runtime-settings-status" role="status">No changes</span><label>AI Detection <input id="ai-detection-enabled" type="checkbox" checked></label><select id="ai-detection-presentation" hidden><option value="side_pane">side_pane</option></select><span id="ai-hide-warning" class="hidden"></span><span id="ai-hide-status" hidden></span></div></form><pre id="saved-profile-ids" hidden></pre></section></main><script type="module" src="/preview.js"></script></body></html>`;

const script = `import { createSettingsFeature } from '/settings-feature.js';
import { createDirtyStateTracker } from '/settings-dirty-state.js';
const fixture = await (await fetch('/fixture.json')).json();
const state = { bootstrap: { settings: fixture.settings, reasoningProviders: [{name:'codex-app-server',label:'Codex App Server'}] } };
const feature = createSettingsFeature({ $: selector => document.querySelector(selector), state, RELEASE_REASONING_DEFAULTS: {} });
feature.renderReasoningProcesses(fixture.processes);
const read = () => Object.fromEntries(fixture.processes.map(process => [process.id, feature.reasoningProfileValue(process.id)]));
const save = document.querySelector('#save-runtime-settings');
const status = document.querySelector('#runtime-settings-status');
const dirty = createDirtyStateTracker({ readSnapshot: read, onChange: changed => { save.disabled = !changed; status.textContent = changed ? 'Unsaved changes' : 'No changes'; } });
dirty.setBaseline();
const form = document.querySelector('#runtime-settings-form');
form.addEventListener('change', event => { if(event.target.id==='ai-detection-enabled') feature.syncAIDetectionSettings(); dirty.refresh(); });
form.addEventListener('submit', event => { event.preventDefault(); const selected = read(); for(const process of fixture.processes) { if(!process.options.some(option=>option.id===selected[process.id])) throw new Error('Invalid profile'); } document.querySelector('#saved-profile-ids').textContent=JSON.stringify(selected); dirty.setBaseline(); status.textContent='Saved in isolated preview'; });`;

const server = http.createServer(async (request, response) => {
  if (request.method !== "GET") { response.writeHead(405); response.end("Read-only preview server"); return; }
  try {
    const pathname = new URL(request.url, "http://localhost").pathname;
    const content = pathname === "/" ? html : pathname === "/preview.js" ? script : pathname === "/fixture.json" ? fixtureBody : null;
    if (content !== null) {
      response.setHeader("Content-Type", pathname === "/" ? "text/html; charset=utf-8" : pathname === "/fixture.json" ? "application/json" : "text/javascript; charset=utf-8");
      response.end(content); return;
    }
    const asset = path.resolve(assets, "." + decodeURIComponent(pathname));
    if (!asset.startsWith(assets + path.sep) || ![".js", ".css"].includes(path.extname(asset))) { response.writeHead(404); response.end(); return; }
    response.setHeader("Content-Type", asset.endsWith(".css") ? "text/css; charset=utf-8" : "text/javascript; charset=utf-8");
    response.end(await readFile(asset));
  } catch { response.writeHead(404); response.end("Not found"); }
});
server.listen(Number(process.env.AKU_CODEX_MODEL_PREVIEW_PORT || 11127), "127.0.0.1", () => console.log(`http://127.0.0.1:${server.address().port}/`));
