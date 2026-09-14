import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import vm from "node:vm";

const source = readFileSync(new URL("../internal/httpapi/web/startup-watchdog.js", import.meta.url), "utf8");
function fixture(hash = "") {
  const elements = Object.fromEntries(["startup-recovery", "startup-recovery-heading", "startup-recovery-detail"]
    .map((id) => [id, { hidden: false, textContent: "" }]));
  const listeners = new Map();
  let timeout;
  let cleared = false;
  const frames = [];
  const requests = [];
  const replacements = [];
  const window = {
    addEventListener: (name, handler) => listeners.set(name, handler),
    removeEventListener: (name) => listeners.delete(name),
  };
  vm.runInNewContext(source, {
    window,
    location: { hash, pathname: "/", search: "" },
    history: { state: null, replaceState: (...args) => replacements.push(args) },
    requestAnimationFrame: (callback) => frames.push(callback),
    fetch: (...args) => { requests.push(args); return Promise.resolve({ status: 204 }); },
    document: { getElementById: (id) => elements[id] },
    setTimeout: (handler, ms) => { assert.equal(ms, 60_000); timeout = handler; return 1; },
    clearTimeout: () => { cleared = true; },
  });
  return {
    elements, window, listeners, requests, replacements,
    frame: () => frames.shift()?.(),
    send: (name, event) => listeners.get(name)?.(event),
    timeout: () => timeout(),
    cleared: () => cleared,
    text: () => elements["startup-recovery-detail"].textContent,
  };
}

test("native acknowledgement waits for ready and two frames and strips fragment", () => {
  const token = "a".repeat(64);
  const app = fixture(`#aku-startup=${token}`);
  assert.equal(app.replacements[0][2], "/");
  app.send("aku-startup-stage", { detail: "restoring" });
  app.frame();
  assert.equal(app.requests.length, 0);
  app.send("aku-startup-stage", { detail: "ready" });
  assert.equal(app.requests.length, 0);
  app.frame();
  assert.equal(app.requests.length, 0);
  app.frame();
  assert.equal(app.requests.length, 1);
  assert.equal(app.requests[0][0], "/api/app-shell/startup-ready");
  assert.equal(app.requests[0][1].headers["X-Aku-Startup-Token"], token);
  assert.equal(app.requests[0][1].method, "POST");
  app.send("aku-startup-stage", { detail: "ready" });
  app.frame(); app.frame();
  assert.equal(app.requests.length, 1);
});

test("ordinary tabs and malformed fragments cannot acknowledge native startup", () => {
  for (const fragment of ["", "#other", "#aku-startup=bad"]) {
    const app = fixture(fragment);
    app.send("aku-startup-stage", { detail: "ready" });
    app.frame(); app.frame();
    assert.equal(app.requests.length, 0);
    assert.equal(app.replacements.length, 0);
  }
});

test("missing module readiness yields bounded recovery without any app code", () => {
  const app = fixture();
  app.timeout();
  assert.match(app.text(), /interface assets/);
  assert.match(app.text(), /60 seconds/);
  assert.match(app.text(), /Reload the interface/);
});

test("module load failure and runtime failure expose fixed diagnostics only", () => {
  const app = fixture();
  app.send("error", { target: { tagName: "SCRIPT", src: "secret-url" }, message: "private-token" });
  assert.match(app.text(), /script could not load/);
  assert.doesNotMatch(app.text(), /secret-url|private-token/);
  app.send("aku-startup-stage", { detail: "rendering" });
  app.send("error", { target: app.window, message: "private-token" });
  assert.match(app.text(), /rendering the interface/);
  assert.match(app.text(), /stopped unexpectedly/);
});

test("restoration timeout is distinct from module failure and ready clears listeners", () => {
  const app = fixture();
  app.send("aku-startup-stage", { detail: "restoring" });
  app.timeout();
  assert.match(app.text(), /restoring saved state/);
  app.send("aku-startup-stage", { detail: "ready" });
  assert.equal(app.elements["startup-recovery"].hidden, true);
  assert.equal(app.cleared(), true);
  assert.equal(app.listeners.size, 0);
  app.timeout();
  assert.equal(app.elements["startup-recovery"].hidden, true);
});

test("watchdog loads independently before the app module and app confirms render milestones", () => {
  const html = readFileSync(new URL("../internal/httpapi/web/index.html", import.meta.url), "utf8");
  assert.ok(html.indexOf('<script src="/startup-watchdog.js') < html.indexOf('<script type="module" src="/app.js'));
  const app = readFileSync(new URL("../internal/httpapi/web/app.js", import.meta.url), "utf8");
  for (const stage of ["restoring", "rendering", "ready"]) {
    assert.ok(app.includes(`new CustomEvent("aku-startup-stage", { detail: "${stage}" })`));
  }
});
