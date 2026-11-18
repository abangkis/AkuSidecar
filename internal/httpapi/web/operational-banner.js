import { setSettingsText } from "./settings-render.js";

export function renderOperationalBanner({ panel, title, detail, action }, notice) {
  panel.classList.toggle("hidden", !notice);
  if (!notice) return;

  setSettingsText(title, notice.title);
  setSettingsText(detail, notice.detail);
  if (action) {
    action.hidden = !notice.actionLabel;
    if (notice.actionLabel) setSettingsText(action, notice.actionLabel);
    action.disabled = Boolean(notice.actionDisabled);
  }
}
