import test from "node:test";
import assert from "node:assert/strict";
import { codexModelLabel, codexModelChoices, chooseCodexProfile, createCodexModelControls } from "../internal/httpapi/web/codex-model-controls.js";

const profiles = ["gpt-6.1-sol", "gpt-6.1-terra", "gpt-6-luna"].flatMap((model) =>
  ["low", "medium", "high", "xhigh", "max"].map((effort) => ({ id: `${model}:${effort}`, model, effort })));

test("only the three approved versioned models are advertised in the requested order", () => {
  const extras = ["gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna", "gpt-6-sol", "gpt-6-astra"].map((model) => ({ id: model, model, effort: "high" }));
  const choices = codexModelChoices([...profiles].reverse().concat(extras));
  assert.deepEqual(choices.map((model) => model.label), ["GPT-6.1 Sol", "GPT-6.1 Terra", "GPT-6 Luna"]);
  assert.ok(choices.every((model) => model.profiles.length === 5));
  for (const choice of choices) {
    assert.equal(codexModelLabel(choice.id), choice.label);
    assert.equal(codexModelLabel(`codex-${choice.id}`), choice.label);
  }
  assert.equal(codexModelLabel("codex-luna"), "");
  assert.equal(codexModelLabel("gpt-6-sol"), "");
});

test("a model change preserves effort and uses a real profile owned by that model", () => {
  const choices = codexModelChoices(profiles);
  for (const target of choices) {
    for (const effort of ["low", "medium", "high", "xhigh", "max"]) {
      const selected = chooseCodexProfile(choices, target.id, effort);
      assert.equal(selected.model, target.id);
      assert.equal(selected.effort, effort);
      assert.ok(profiles.includes(selected));
    }
  }
  assert.equal(chooseCodexProfile(choices, "gpt-6-astra", "high"), undefined);
});

test("unsupported effort uses high without escalating or inventing combinations", () => {
  const choices = codexModelChoices(profiles);
  assert.equal(chooseCodexProfile(choices, "gpt-6.1-sol", "ultra").effort, "high");
  const maxOnly = codexModelChoices(profiles.filter((profile) => profile.effort === "max"));
  assert.equal(chooseCodexProfile(maxOnly, "gpt-6.1-sol", "high"), undefined);
});

class Element {
  constructor(tag) { this.tagName = tag; this.children = []; this.dataset = {}; this.listeners = {}; this.attributes = {}; this.value = ""; }
  append(...children) { this.children.push(...children); }
  replaceChildren(...children) { this.children = [...children]; }
  setAttribute(name, value) { this.attributes[name] = value; }
  addEventListener(event, handler) { this.listeners[event] = handler; }
  change(value) { this.value = value; this.listeners.change?.({ target: this }); }
}
const document = { createElement: (tag) => new Element(tag) };
const formatEffort = (effort) => effort === "xhigh" ? "XHigh" : effort[0].toUpperCase() + effort.slice(1);

test("split DOM controls persist one opaque profile ID and refresh the visible model/effort", () => {
  const changes = [];
  const host = createCodexModelControls({ document, process: { id: "acquisition_planning", label: "Acquisition planning", options: profiles, profileId: "gpt-6-luna:xhigh" }, formatEffort, onChange: (...args) => changes.push(args) });
  const [hidden, modelField, effortField] = host.children;
  const model = modelField.children[1], effort = effortField.children[1];
  assert.equal(hidden.dataset.processId, "acquisition_planning");
  assert.equal(hidden.value, "gpt-6-luna:xhigh");
  assert.equal(model.value, "gpt-6-luna");
  assert.equal(effort.value, "xhigh");
  assert.equal(model.children.length, 3);
  model.change("gpt-6.1-sol");
  assert.equal(hidden.value, "gpt-6.1-sol:xhigh");
  effort.change("medium");
  assert.equal(hidden.value, "gpt-6.1-sol:medium");
  assert.deepEqual(changes, [["gpt-6.1-sol", "xhigh"], ["gpt-6.1-sol", "medium"]]);
  assert.equal(model.attributes["aria-label"], "Acquisition planning model");
  assert.equal(effort.attributes["aria-label"], "Acquisition planning reasoning effort");
});

test("unknown persisted selections remain unchanged until the user chooses a model", () => {
  const host = createCodexModelControls({ document, process: { id: "candidate_evaluation", label: "Candidate evaluation", options: profiles, profileId: "unrecognized" }, formatEffort });
  const [hidden, modelField, effortField] = host.children;
  assert.equal(hidden.value, "unrecognized");
  assert.equal(modelField.children[1].value, "");
  assert.equal(effortField.children[1].disabled, true);
  modelField.children[1].change("gpt-6.1-terra");
  assert.equal(hidden.value, "gpt-6.1-terra:high");
});
