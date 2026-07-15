const endpoint = location.origin;
const state = { bootstrap: null, session: null, dispatchKey: null, poller: null };
const $ = (selector) => document.querySelector(selector);

window.addEventListener("message", (event) => {
  if (event.source !== window || event.origin !== endpoint || !event.data) return;
  if (event.data.type === "AKU_BROWSER_BRIDGE_READY") {
    bridgeApi("/api/bridge/heartbeat", { method: "POST", body: { capabilities: event.data.capabilities ?? {} } })
      .then(({ bridge }) => renderBridge(bridge)).catch(showError);
  }
  if (event.data.type === "AKU_BROWSER_BRIDGE_ERROR") showError(new Error(event.data.message));
});

$("#session-form").addEventListener("submit", async (event) => {
  event.preventDefault();
  try {
    const { session } = await api("/api/sessions", { method: "POST", body: { intent: $("#intent").value } });
    state.session = session;
    renderSession();
    startPolling();
  } catch (error) { showError(error); }
});

$("#cancel-button").addEventListener("click", async () => {
  if (!state.session) return;
  const { session } = await api(`/api/sessions/${encodeURIComponent(state.session.id)}/cancel`, { method: "POST" });
  state.session = session; renderSession(); stopPolling();
});
$("#refresh-button").addEventListener("click", refreshTimeline);
$("#save-settings").addEventListener("click", saveSettings);

async function bootstrap() {
  try {
    state.bootstrap = await api("/api/bootstrap");
    state.session = state.bootstrap.activeSession;
    $("#runtime-version").textContent = `${state.bootstrap.version} · ${state.bootstrap.runtime}`;
    $("#bridge-contract").textContent = state.bootstrap.bridgeContractVersion;
    setPill("#sidecar-status", "AkuSidecar Go ready", "ok");
    setPill("#model-status", state.bootstrap.provider, "neutral");
    renderBridge(state.bootstrap.bridge);
    renderSettings(state.bootstrap.settings);
    renderTimeline(state.bootstrap.timeline ?? []);
    renderSession();
    pingBridge();
    bridgeActionLoop();
    setInterval(pingBridge, 30_000);
    if (state.session) startPolling();
  } catch (error) { showError(error); setTimeout(bootstrap, 1500); }
}

function pingBridge() { window.postMessage({ type: "AKU_BROWSER_BRIDGE_PING" }, endpoint); }

async function bridgeActionLoop() {
  while (true) {
    try {
      const response = await bridgeApi("/api/operations/bridge/actions/next?waitMs=25000");
      if (response?.action) window.postMessage({ type: "AKU_BROWSER_BRIDGE_RELOAD_SELF", actionId: response.action.id, endpoint, token: state.bootstrap.bridgeToken }, endpoint);
    } catch (error) { console.warn("Bridge action relay paused", error); await new Promise((resolve) => setTimeout(resolve, 1000)); }
  }
}
function renderBridge(bridge) {
  if (state.bootstrap) state.bootstrap.bridge = bridge;
  if (bridge?.compatible) setPill("#bridge-status", `AkuBridge ${bridge.actual?.extensionVersion} ready`, "ok");
  else if (bridge?.state === "incompatible") setPill("#bridge-status", bridge.reasons?.join(", ") || "Bridge incompatible", "danger");
  else setPill("#bridge-status", "Bridge reconnecting", "warning");
  $("#start-button").disabled = !bridge?.compatible || !!state.session;
}

function renderSettings(settings) {
  if (!settings) return;
  $("#profile").value = settings.loadProfile;
  $("#visibility").value = settings.captureVisibility;
  $("#preference-mode").value = settings.preferenceEligibilityMode;
  $("#open-missing").checked = settings.openMissingSource;
}

async function saveSettings() {
  const current = state.bootstrap.settings;
  const settings = { ...current, loadProfile: $("#profile").value, captureVisibility: $("#visibility").value, preferenceEligibilityMode: $("#preference-mode").value, openMissingSource: $("#open-missing").checked };
  try {
    const response = await api("/api/settings", { method: "PUT", body: { settings } });
    state.bootstrap.settings = response.settings; renderSettings(response.settings);
    $("#settings-note").textContent = `Saved · ${response.settings.maxScrolls} scrolls · ${response.settings.maxItemsPerSource} items/source`;
  } catch (error) { showError(error); }
}

function startPolling() { if (state.poller) return; state.poller = setInterval(pollSession, 650); pollSession(); }
function stopPolling() { clearInterval(state.poller); state.poller = null; }

async function pollSession() {
  if (!state.session) return;
  try {
    const { session } = await api(`/api/sessions/${encodeURIComponent(state.session.id)}`);
    state.session = session; renderSession();
    const activeRun = session.runs?.find((run) => ["waiting_for_bridge", "reasoning"].includes(run.status));
    if (activeRun?.status === "waiting_for_bridge") dispatch(activeRun);
    if (["completed", "partial", "failed", "cancelled"].includes(session.status)) {
      stopPolling(); state.dispatchKey = null; await refreshTimeline();
      window.postMessage({ type: "AKU_BROWSER_RELEASE_CAPTURE_SURFACE", leaseId: session.id }, endpoint);
      state.session = null; renderSession();
    }
  } catch (error) { showError(error); }
}

function dispatch(run) {
  const key = `${run.id}:${run.stage}`;
  if (state.dispatchKey === key) return;
  state.dispatchKey = key;
  window.postMessage({ type: "AKU_BROWSER_DISPATCH", endpoint, token: state.bootstrap.bridgeToken, runId: run.id }, endpoint);
}

function renderSession() {
  const session = state.session;
  $("#session-status").textContent = session ? session.status : "Idle";
  $("#start-button").disabled = !!session || !state.bootstrap?.bridge?.compatible;
  $("#cancel-button").classList.toggle("hidden", !session);
  $("#progress").classList.toggle("hidden", !session);
  if (!session) return;
  const run = session.runs?.find((value) => !["queued", "completed", "failed", "cancelled"].includes(value.status)) || session.runs?.find((value) => value.status === "queued");
  const ordinal = run?.ordinal ?? 0;
  const width = run?.status === "reasoning" ? 40 + ordinal * 45 : 15 + ordinal * 45;
  $("#progress-bar").style.width = `${Math.min(95, width)}%`;
  $("#progress-text").textContent = run ? `${run.source.toUpperCase()} · ${run.stage.replaceAll("_", " ")}` : session.status;
}

async function refreshTimeline() {
  const { items } = await api(`/api/timeline?limit=${state.bootstrap?.settings?.timelineCapacity ?? 24}&offset=0`);
  renderTimeline(items);
}

function renderTimeline(items) {
  const container = $("#timeline"); container.replaceChildren();
  if (!items?.length) { const empty = document.createElement("p"); empty.className = "empty"; empty.textContent = "No completed items yet."; container.append(empty); return; }
  for (const entry of items) {
    const fragment = $("#item-template").content.cloneNode(true);
    const card = fragment.querySelector("article"); const item = entry.item;
    card.querySelector(".source").textContent = entry.source;
    card.querySelector(".confidence").textContent = `${Math.round((item.confidence ?? 0) * 100)}% confidence`;
    card.querySelector(".what").textContent = item.whatChanged;
    card.querySelector(".why").textContent = item.whyItMatters;
    card.querySelector(".source-link").href = item.sourceUrl;
    const reason = card.querySelector(".reason");
    for (const button of card.querySelectorAll("button[data-direction]")) button.addEventListener("click", async () => {
      const direction = button.dataset.direction; reason.classList.toggle("hidden", direction !== "less");
      await sendFeedback(entry.id, direction, direction === "less" && reason.value ? reason.value : null);
    });
    reason.addEventListener("change", () => { if (reason.value) sendFeedback(entry.id, "less", reason.value); });
    container.append(fragment);
  }
}

async function sendFeedback(id, direction, reason) {
  try { await api(`/api/timeline/${encodeURIComponent(id)}/feedback`, { method: "POST", body: { direction, reason } }); }
  catch (error) { showError(error); }
}

async function api(path, options = {}) {
  const init = { method: options.method || "GET", cache: "no-store", headers: {} };
  if (options.body !== undefined) { init.headers["Content-Type"] = "application/json"; init.body = JSON.stringify(options.body); }
  const response = await fetch(path, init); const payload = response.status === 204 ? null : await response.json();
  if (!response.ok) throw new Error(payload?.message || `HTTP ${response.status}`); return payload;
}

async function bridgeApi(path, options = {}) {
  const init = { method: options.method || "GET", cache: "no-store", headers: { "X-Aku-Bridge-Token": state.bootstrap.bridgeToken, "X-Aku-Bridge-Id": "aku-browser-page", "X-Aku-Bridge-Contract": state.bootstrap.bridgeContractVersion } };
  if (options.body !== undefined) { init.headers["Content-Type"] = "application/json"; init.body = JSON.stringify(options.body); }
  const response = await fetch(path, init); if (response.status === 204) return null; const payload = await response.json(); if (!response.ok) throw new Error(payload?.message || `HTTP ${response.status}`); return payload;
}

function setPill(selector, text, tone) { const node = $(selector); node.textContent = text; node.className = `pill ${tone === "ok" ? "" : tone}`; }
function showError(error) { console.error(error); setPill("#sidecar-status", error.message || String(error), "danger"); }

bootstrap();
