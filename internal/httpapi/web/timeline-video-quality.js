// Quality controls consume player state; source admission and streaming stay in
// the playback policy and player modules.
export function createTimelineVideoQuality(onSelect) {
  const element = document.createElement("div");
  element.className = "source-layout-video-quality";
  const label = document.createElement("label");
  const caption = document.createElement("span");
  caption.textContent = "Quality";
  const select = document.createElement("select");
  select.setAttribute("aria-label", "Video quality");
  const status = document.createElement("span");
  status.className = "source-layout-video-quality-status";
  status.setAttribute("aria-live", "polite");
  label.append(caption, select);
  element.append(label, status);
  let signature = "";
  let disposed = false;
  const change = () => { if (!disposed) onSelect(select.value); };
  select.addEventListener("change", change);

  function update(state = {}) {
    if (disposed) return;
    const qualities = Array.isArray(state.qualities) ? state.qualities : [];
    const options = [{ id: "auto", label: "Auto" }, ...qualities];
    const nextSignature = JSON.stringify(options.map(({ id, label }) => [id, label]));
    if (nextSignature !== signature) {
      signature = nextSignature;
      select.replaceChildren(...options.map(quality => {
        const option = document.createElement("option");
        option.value = String(quality.id);
        option.textContent = quality.label;
        return option;
      }));
    }
    const selected = qualities.find(quality => String(quality.id) === String(state.selected));
    const active = qualities.find(quality => String(quality.id) === String(state.active));
    select.value = selected ? String(selected.id) : "auto";
    select.disabled = qualities.length === 0;
    element.hidden = qualities.length === 1;
    if (!qualities.length) status.textContent = "Loading quality options…";
    else if (state.selected === "auto" || !selected) status.textContent = active ? `Auto · ${active.label}` : "Auto";
    else if (active && active.id !== selected.id) status.textContent = `${selected.label} selected · Playing ${active.label}`;
    else status.textContent = state.pending ? `Switching to ${selected.label}…` : selected.label;
  }

  update();
  return {
    element,
    update,
    dispose() {
      disposed = true;
      select.removeEventListener("change", change);
      element.remove();
    },
  };
}
