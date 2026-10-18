const RASTER_PNG_BASE64 = /^[a-z0-9+/]+={0,2}$/i;

function appendText(element, value) {
  element.textContent = String(value ?? "");
}

function createElement(document, tag, className, text = "") {
  const element = document.createElement(tag);
  if (className) element.className = className;
  if (text) appendText(element, text);
  return element;
}

function safeFilename(value) {
  let filename = String(value || "post-screenshot.png").replace(/[\\/:*?"<>|\u0000-\u001f]/g, "-").trim();
  if (!filename) filename = "post-screenshot.png";
  return filename.toLowerCase().endsWith(".png") ? filename : `${filename}.png`;
}

function base64Bytes(value, atob) {
  const encoded = String(value ?? "");
  if (!encoded || !RASTER_PNG_BASE64.test(encoded)) throw new Error("The screenshot response contained invalid PNG data.");
  const binary = atob(encoded);
  const bytes = new Uint8Array(binary.length);
  for (let index = 0; index < binary.length; index += 1) bytes[index] = binary.charCodeAt(index);
  return bytes;
}

function formatFilename(filename, index, total) {
  if (total === 1) return filename;
  const stem = filename.replace(/\.png$/i, "");
  return `${stem}-${String(index + 1).padStart(2, "0")}.png`;
}

export function createPostScreenshotUI({ request, getOriginalUrl = () => null, document = globalThis.document, window = globalThis.window } = {}) {
  if (typeof request !== "function") throw new TypeError("A request function is required.");
  if (!document || !window) throw new TypeError("A document and window are required.");

  let activeMenu = null;
  let dialogOpen = false;
  let currentEntry = null;
  let returnFocus = null;
  let activeRequest = null;
  let parts = [];
  let selectedPart = 0;
  let currentFilename = "post-screenshot.png";

  const dialog = createElement(document, "dialog", "post-screenshot-dialog");
  dialog.setAttribute("aria-labelledby", "post-screenshot-title");
  dialog.setAttribute("aria-describedby", "post-screenshot-status");
  dialog.setAttribute("aria-label", "Screenshot full post");
  dialog.tabIndex = -1;

  const panel = createElement(document, "section", "post-screenshot-panel");
  const header = createElement(document, "header", "post-screenshot-header");
  const heading = createElement(document, "div", "post-screenshot-heading");
  const title = createElement(document, "h2", "", "Screenshot full post");
  title.id = "post-screenshot-title";
  const status = createElement(document, "p", "post-screenshot-status", "Preparing screenshot…");
  status.id = "post-screenshot-status";
  heading.append(title, status);
  const closeButton = createElement(document, "button", "secondary-button post-screenshot-close", "Close");
  closeButton.type = "button";
  closeButton.setAttribute("aria-label", "Close screenshot preview");
  header.append(heading, closeButton);

  const controls = createElement(document, "div", "post-screenshot-controls");
  const themeLabel = createElement(document, "label", "post-screenshot-theme-label", "Theme");
  const themeSelect = createElement(document, "select", "post-screenshot-theme");
  themeSelect.setAttribute("aria-label", "Screenshot theme");
  for (const [value, label] of [["dark", "Dark"], ["light", "Light"]]) {
    const option = createElement(document, "option", "", label);
    option.value = value;
    themeSelect.append(option);
  }
  themeSelect.value = "dark";
  themeLabel.append(themeSelect);
  const generateButton = createElement(document, "button", "primary-button post-screenshot-generate", "Generate screenshot");
  generateButton.type = "button";
  controls.append(themeLabel, generateButton);

  const warningList = createElement(document, "ul", "post-screenshot-warnings");
  warningList.setAttribute("aria-label", "Screenshot warnings");
  const error = createElement(document, "p", "post-screenshot-error");
  error.setAttribute("role", "alert");
  error.hidden = true;

  const partNavigation = createElement(document, "div", "post-screenshot-part-navigation");
  const previousPart = createElement(document, "button", "secondary-button", "Previous part");
  previousPart.type = "button";
  previousPart.setAttribute("aria-label", "Previous screenshot part");
  const partStatus = createElement(document, "span", "post-screenshot-part-status");
  partStatus.setAttribute("aria-live", "polite");
  const nextPart = createElement(document, "button", "secondary-button", "Next part");
  nextPart.type = "button";
  nextPart.setAttribute("aria-label", "Next screenshot part");
  partNavigation.append(previousPart, partStatus, nextPart);
  partNavigation.hidden = true;

  const preview = createElement(document, "div", "post-screenshot-preview");
  preview.setAttribute("aria-live", "polite");
  const actions = createElement(document, "div", "post-screenshot-actions");
  const copyButton = createElement(document, "button", "secondary-button post-screenshot-copy", "Copy image");
  copyButton.type = "button";
  copyButton.disabled = true;
  const saveLinks = createElement(document, "div", "post-screenshot-save-links");
  actions.append(copyButton, saveLinks);
  panel.append(header, controls, error, warningList, partNavigation, preview, actions);
  dialog.append(panel);
  document.body.append(dialog);

  function setError(message) {
    appendText(error, message);
    error.hidden = !message;
  }

  function resetPreview() {
    parts = [];
    selectedPart = 0;
    preview.replaceChildren();
    saveLinks.replaceChildren();
    partNavigation.hidden = true;
    partStatus.textContent = "";
    copyButton.disabled = true;
    previousPart.disabled = true;
    nextPart.disabled = true;
    warningList.replaceChildren();
  }

  function closeMenu(menu, { focus = false } = {}) {
    if (!menu) return;
    menu.menu.hidden = true;
    menu.button.setAttribute("aria-expanded", "false");
    if (activeMenu === menu) activeMenu = null;
    if (focus) menu.button.focus();
  }

  function closeActiveMenu(options) {
    closeMenu(activeMenu, options);
  }

  function finishClose() {
    if (!dialogOpen) return;
    dialogOpen = false;
    if (activeRequest) activeRequest.abort();
    activeRequest = null;
    currentEntry = null;
    resetPreview();
    appendText(status, "Screenshot preview closed.");
    setError("");
    closeActiveMenu();
    const trigger = returnFocus;
    returnFocus = null;
    if (trigger?.isConnected !== false) trigger?.focus();
  }

  function closeDialog() {
    if (!dialogOpen) return;
    if (typeof dialog.close === "function" && dialog.open) dialog.close();
    finishClose();
  }

  function setSelectedPart(index) {
    if (!parts.length) return;
    selectedPart = Math.max(0, Math.min(parts.length - 1, index));
    preview.replaceChildren(parts[selectedPart].image);
    partNavigation.hidden = parts.length < 2;
    partStatus.textContent = `Part ${selectedPart + 1} of ${parts.length}`;
    previousPart.disabled = selectedPart === 0;
    nextPart.disabled = selectedPart === parts.length - 1;
    copyButton.disabled = false;
  }

  function displayResult(result) {
    resetPreview();
    if (!Array.isArray(result?.parts) || result.parts.length === 0) throw new Error("No screenshot images were returned.");
    const atob = window.atob?.bind(window) || globalThis.atob;
    if (typeof atob !== "function") throw new Error("This browser cannot decode the screenshot image.");
    const BlobConstructor = window.Blob || globalThis.Blob;
    if (typeof BlobConstructor !== "function") throw new Error("This browser cannot prepare screenshot images.");
    currentFilename = safeFilename(result.filename);
    parts = result.parts.map((part, index) => {
      const bytes = base64Bytes(part?.data, atob);
      const blob = new BlobConstructor([bytes], { type: "image/png" });
      const dataURL = `data:image/png;base64,${part.data}`;
      const image = createElement(document, "img", "post-screenshot-image");
      image.src = dataURL;
      image.alt = `Screenshot part ${index + 1} of ${result.parts.length}`;
      const width = Number(part?.width);
      const height = Number(part?.height);
      if (Number.isFinite(width) && width > 0) image.width = width;
      if (Number.isFinite(height) && height > 0) image.height = height;
      const link = createElement(document, "a", "post-screenshot-save-link", result.parts.length === 1 ? "Save PNG" : `Save part ${index + 1} PNG`);
      link.href = dataURL;
      link.download = formatFilename(currentFilename, index, result.parts.length);
      link.setAttribute("aria-label", result.parts.length === 1 ? "Save screenshot as PNG" : `Save screenshot part ${index + 1} as PNG`);
      saveLinks.append(link);
      return { image, blob, data: part.data };
    });
    const warnings = Array.isArray(result.warnings) ? result.warnings.filter((value) => typeof value === "string" && value.trim()) : [];
    for (const value of warnings) warningList.append(createElement(document, "li", "", value));
    appendText(status, `${parts.length} screenshot part${parts.length === 1 ? "" : "s"} ready.`);
    setSelectedPart(0);
  }

  async function generate(theme = themeSelect.value) {
    if (!dialogOpen || !currentEntry) return;
    if (activeRequest) activeRequest.abort();
    const controller = new AbortController();
    activeRequest = controller;
    resetPreview();
    setError("");
    appendText(status, "Generating full-post screenshot…");
    generateButton.disabled = true;
    try {
      const result = await request("/api/post-screenshots", {
        method: "POST",
        body: { timelineId: currentEntry.id, theme: theme === "light" ? "light" : "dark" },
        signal: controller.signal,
      });
      if (!dialogOpen || activeRequest !== controller) return;
      displayResult(result);
    } catch (caught) {
      if (!dialogOpen || activeRequest !== controller || caught?.name === "AbortError") return;
      appendText(status, "Screenshot generation failed.");
      resetPreview();
      setError(caught?.message || "Could not generate the screenshot.");
    } finally {
      if (activeRequest === controller) {
        activeRequest = null;
        generateButton.disabled = false;
      }
    }
  }

  function openDialog(entry, trigger) {
    closeActiveMenu();
    if (dialogOpen) closeDialog();
    currentEntry = entry;
    returnFocus = trigger;
    themeSelect.value = "dark";
    resetPreview();
    setError("");
    appendText(status, "Preparing screenshot…");
    dialogOpen = true;
    if (typeof dialog.showModal === "function") dialog.showModal();
    else dialog.open = true;
    generate("dark");
  }

  function createMenu(entry) {
    const root = createElement(document, "div", "post-screenshot-menu");
    const button = createElement(document, "button", "post-screenshot-menu-button", "⋮");
    button.type = "button";
    button.title = "More post actions";
    button.setAttribute("aria-label", "More post actions");
    button.setAttribute("aria-haspopup", "menu");
    button.setAttribute("aria-expanded", "false");
    const menu = createElement(document, "div", "post-screenshot-menu-list");
    menu.setAttribute("role", "menu");
    menu.hidden = true;
    const action = createElement(document, "button", "post-screenshot-menu-item", "Screenshot full post");
    action.type = "button";
    action.setAttribute("role", "menuitem");
    const copyURL = createElement(document, "button", "post-screenshot-menu-item post-original-url-copy", "Copy original URL");
    copyURL.type = "button";
    copyURL.setAttribute("role", "menuitem");
    copyURL.disabled = !getOriginalUrl(entry);
    copyURL.title = copyURL.disabled ? "Original post URL unavailable" : "Copy the source post link without opening it";
    const copyStatus = createElement(document, "p", "post-original-url-status");
    copyStatus.setAttribute("role", "status");
    copyStatus.setAttribute("aria-live", "polite");
    copyStatus.hidden = true;
    let copyingURL = false;
    const availableActions = () => [action, copyURL].filter((item) => !item.disabled);
    menu.append(action, copyURL, copyStatus);
    root.append(button, menu);
    const control = { root, button, menu, action, entry };
    button.addEventListener("click", () => {
      if (activeMenu === control) {
        closeMenu(control);
        return;
      }
      closeActiveMenu();
      activeMenu = control;
      menu.hidden = false;
      button.setAttribute("aria-expanded", "true");
      action.focus();
    });
    button.addEventListener("keydown", (event) => {
      if (event.key !== "ArrowDown" && event.key !== "ArrowUp") return;
      event.preventDefault();
      if (activeMenu !== control) {
        closeActiveMenu();
        activeMenu = control;
        menu.hidden = false;
        button.setAttribute("aria-expanded", "true");
      }
      const choices = availableActions();
      (event.key === "ArrowUp" ? choices.at(-1) : choices[0]).focus();
    });
    action.addEventListener("click", () => openDialog(entry, button));
    copyURL.addEventListener("click", async () => {
      const url = getOriginalUrl(entry);
      if (!url || copyingURL) return;
      copyingURL = true;
      copyURL.setAttribute("aria-busy", "true");
      copyStatus.hidden = false;
      appendText(copyStatus, "Copying URL…");
      try {
        if (!window.navigator?.clipboard?.writeText) throw new Error("Clipboard unavailable");
        await window.navigator.clipboard.writeText(url);
        appendText(copyStatus, "Original URL copied.");
      } catch {
        appendText(copyStatus, "Could not copy URL. Check clipboard permission.");
      } finally {
        copyingURL = false;
        copyURL.setAttribute("aria-busy", "false");
        copyURL.disabled = !getOriginalUrl(entry);
      }
    });
    for (const item of [action, copyURL]) item.addEventListener("keydown", (event) => {
      if (event.key === "Escape") {
        event.preventDefault();
        closeMenu(control, { focus: true });
      } else if (["ArrowDown", "ArrowUp", "Home", "End"].includes(event.key)) {
        event.preventDefault();
        const choices = availableActions();
        const index = choices.indexOf(item);
        const next = event.key === "Home" ? 0 : event.key === "End" ? choices.length - 1
          : (index + (event.key === "ArrowDown" ? 1 : -1) + choices.length) % choices.length;
        choices[next].focus();
      }
    });
    return root;
  }

  document.addEventListener("pointerdown", (event) => {
    if (activeMenu && !activeMenu.root.contains(event.target)) closeActiveMenu();
  });
  document.addEventListener("focusin", (event) => {
    if (activeMenu && !activeMenu.root.contains(event.target)) closeActiveMenu();
  });
  document.addEventListener("keydown", (event) => {
    if (event.key === "Escape" && activeMenu && !dialogOpen) {
      event.preventDefault();
      closeActiveMenu({ focus: true });
    }
  });
  dialog.addEventListener("cancel", (event) => {
    event.preventDefault();
    closeDialog();
  });
  dialog.addEventListener("close", finishClose);
  dialog.addEventListener("click", (event) => {
    if (event.target === dialog) closeDialog();
  });
  closeButton.addEventListener("click", closeDialog);
  generateButton.addEventListener("click", () => generate(themeSelect.value));
  previousPart.addEventListener("click", () => setSelectedPart(selectedPart - 1));
  nextPart.addEventListener("click", () => setSelectedPart(selectedPart + 1));
  copyButton.addEventListener("click", async () => {
    const part = parts[selectedPart];
    if (!part) return;
    copyButton.disabled = true;
    setError("");
    appendText(status, "Copying image to clipboard…");
    try {
      const ClipboardItem = window.ClipboardItem || globalThis.ClipboardItem;
      if (!window.navigator?.clipboard?.write || typeof ClipboardItem !== "function") {
        throw new Error("Image clipboard copying is unavailable here. Use Save PNG instead.");
      }
      await window.navigator.clipboard.write([new ClipboardItem({ "image/png": part.blob })]);
      if (dialogOpen) appendText(status, `Part ${selectedPart + 1} copied to clipboard.`);
    } catch (caught) {
      if (dialogOpen) {
        appendText(status, "Could not copy the screenshot image.");
        const detail = caught?.message && /Use Save PNG instead/i.test(caught.message)
          ? caught.message
          : "Clipboard access was denied. Use Save PNG instead.";
        setError(detail);
      }
    } finally {
      copyButton.disabled = !parts.length;
      if (dialogOpen) copyButton.focus();
    }
  });

  return {
    createMenu,
    close: closeDialog,
    get dialog() { return dialog; },
  };
}
