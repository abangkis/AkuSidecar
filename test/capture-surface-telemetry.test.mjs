import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import vm from "node:vm";

const source = readFileSync(new URL("../internal/httpapi/web/app.js", import.meta.url), "utf8");
const render = source.slice(source.indexOf("function buildCaptureSurfaceTelemetry("),
  source.indexOf("function buildAcquisitionIdentityTelemetry("));
function fixture() {
  const context = { document: { createElement: () => ({ textContent: "", children: [],
    append(...children) { this.children.push(...children); } }) },
  humanize: (value) => String(value ?? "").replaceAll("_", " "), formatDate: () => "test time" };
  vm.createContext(context);
  vm.runInContext(render, context);
  return (events) => context.buildCaptureSurfaceTelemetry(events);
}

test("Inbox renders verified and pending surface counts with executing policy identity", () => {
  const render = fixture();
  const view = render([{
    event: "reconciled", outcome: "cleanup_pending", detail: {
      cleanup: { schema: "aku.surface-cleanup.v1", expected: 3, closed: 1, retained: 1, pending: 1 },
      captureSurfacePolicy: { fingerprint: "a".repeat(64) },
    },
  }]);
  assert.match(view.children[0].children[1].textContent, /1 closed \/ 3 tracked.*1 lease-retained.*1 pending/);
  const copy = view.children[1].children[0].children[0];
  assert.equal(copy.children[0].textContent, "Capture cleanup receipt");
  assert.match(copy.children[1].textContent, /cleanup pending.*policy a{12}/);
});

test("Inbox does not display malformed cleanup counts as verified ownership", () => {
  const view = fixture()([{ event: "reconciled", detail: {
    cleanup: { schema: "aku.surface-cleanup.v1", expected: -1, closed: 0, retained: 0, pending: 0 },
  } }]);
  assert.doesNotMatch(view.children[0].children[1].textContent, /tracked/);
});
