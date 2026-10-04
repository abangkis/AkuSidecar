// Polls must not replace identical text nodes or reset classes during scrolling.
export function setSettingsText(element, value) {
  if (element.textContent !== value) element.textContent = value;
}
export function setSettingsClass(element, value) {
  if (element.className !== value) element.className = value;
}
