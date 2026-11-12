import test from "node:test";
import assert from "node:assert/strict";
import { renderExecutableSelection, readExecutableSelection } from "../internal/httpapi/web/reasoning-executable-selection.js";

test("rendering and unrelated saves keep a discovered path automatic", () => {
  const input = { value: "", dataset: {} };
  renderExecutableSelection(input, "C:\\Codex\\new\\codex.exe", "automatic");
  assert.deepEqual(readExecutableSelection(input), { reasoningExecutablePath: input.value, reasoningExecutableMode: "automatic" });
  input.value = "C:\\custom\\codex.exe";
  assert.equal(readExecutableSelection(input).reasoningExecutableMode, "explicit");
  input.value = "";
  assert.equal(readExecutableSelection(input).reasoningExecutableMode, "automatic");
});

test("Detect switches a pinned path to automatic even when the detected path is unchanged", () => {
  const input = { value: "", dataset: {} };
  renderExecutableSelection(input, "codex.exe", "explicit");
  assert.equal(readExecutableSelection(input).reasoningExecutableMode, "explicit");
  renderExecutableSelection(input, "codex.exe", "automatic");
  assert.equal(readExecutableSelection(input).reasoningExecutableMode, "automatic");
});

test("legacy provenance is left for the host and manual edits are explicit", () => {
  const input = { value: "", dataset: {} };
  renderExecutableSelection(input, "legacy.exe");
  assert.equal(readExecutableSelection(input).reasoningExecutableMode, "");
  input.value = "chosen.exe";
  assert.equal(readExecutableSelection(input).reasoningExecutableMode, "explicit");
});
