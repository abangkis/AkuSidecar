import { setSettingsText } from "./settings-render.js";

export function renderOperationalBanner({ panel, title, detail, action }, notice) {
  const hidden = !notice || notice.visible === false;
  panel?.classList?.toggle("hidden", hidden);
  if (panel && panel.hidden !== hidden) panel.hidden = hidden;
  if (!notice) return;

  setSettingsText(title, notice.title);
  setSettingsText(detail, notice.detail);
  if (action) {
    action.hidden = !notice.actionLabel;
    if (notice.actionLabel) setSettingsText(action, notice.actionLabel);
    if (typeof notice.actionDisabled === "boolean") action.disabled = notice.actionDisabled;
  }
}
