const codexModels = Object.freeze([
  { id: "gpt-6.1-sol", label: "GPT-6.1 Sol" },
  { id: "gpt-6.1-terra", label: "GPT-6.1 Terra" },
  { id: "gpt-6-luna", label: "GPT-6 Luna" },
]);

export function codexModelLabel(value) {
  const native = String(value || "").replace(/^codex-/, "");
  return codexModels.find((model) => model.id === native)?.label || "";
}

export function codexModelChoices(options = []) {
  return codexModels.map((model) => ({
    ...model,
    profiles: options.filter((option) => option.model === model.id && option.id && option.effort),
  })).filter((model) => model.profiles.length);
}

export function chooseCodexProfile(choices, modelId, effort) {
  const profiles = choices.find((model) => model.id === modelId)?.profiles || [];
  return profiles.find((profile) => profile.effort === effort)
    || profiles.find((profile) => profile.effort === "high")
    || profiles.find((profile) => ["none", "low", "medium"].includes(profile.effort));
}

// The split controls compose one provider-owned profile ID. Domain settings and
// Save keep their existing opaque profile contract; labels never route requests.
export function createCodexModelControls({ document, process, onChange, formatEffort }) {
  const choices = codexModelChoices(process.options);
  const current = choices.flatMap((model) => model.profiles).find((profile) => profile.id === process.profileId);
  const key = process.id.replaceAll("_", "-");
  const host = document.createElement("fieldset");
  host.className = "reasoning-model-controls";
  host.setAttribute("aria-label", `${process.label} model and reasoning effort`);

  const profile = document.createElement("input");
  profile.type = "hidden";
  profile.id = `reasoning-profile-${key}`;
  profile.dataset.processId = process.id;
  profile.value = process.profileId || "";

  const modelField = document.createElement("label");
  modelField.className = "reasoning-model-field";
  const modelLabel = document.createElement("span");
  modelLabel.textContent = "Model";
  const model = document.createElement("select");
  model.id = `reasoning-model-${key}`;
  model.className = "reasoning-model-select";
  model.setAttribute("aria-label", `${process.label} model`);
  modelField.append(modelLabel, model);

  const effortField = document.createElement("label");
  effortField.className = "reasoning-effort-field";
  const effortLabel = document.createElement("span");
  effortLabel.textContent = "Reasoning effort";
  const effort = document.createElement("select");
  effort.id = `reasoning-effort-${key}`;
  effort.className = "reasoning-effort-select";
  effort.setAttribute("aria-label", `${process.label} reasoning effort`);
  effortField.append(effortLabel, effort);

  function option(value, text) {
    const entry = document.createElement("option");
    entry.value = value;
    entry.textContent = text;
    return entry;
  }
  if (!current) {
    const placeholder = option("", choices.length ? "Choose model" : "No supported model");
    placeholder.disabled = true;
    model.append(placeholder);
  }
  for (const choice of choices) model.append(option(choice.id, choice.label));
  model.value = current?.model || "";
  model.disabled = !choices.length;

  function renderEfforts(selected) {
    const profiles = choices.find((choice) => choice.id === model.value)?.profiles || [];
    effort.replaceChildren(...profiles.map((entry) => option(entry.effort, formatEffort(entry.effort))));
    effort.value = selected?.effort || "";
    effort.disabled = !selected;
  }
  function select(selected) {
    if (!selected) return;
    profile.value = selected.id;
    onChange?.(selected.model, selected.effort);
  }
  renderEfforts(current);
  model.addEventListener("change", () => {
    const selected = chooseCodexProfile(choices, model.value, effort.value);
    renderEfforts(selected);
    select(selected);
  });
  effort.addEventListener("change", () => {
    const selected = choices.find((choice) => choice.id === model.value)?.profiles.find((entry) => entry.effort === effort.value);
    select(selected);
  });
  host.append(profile, modelField, effortField);
  return host;
}
