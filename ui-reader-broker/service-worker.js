const uiOrigins = new Set(["http://127.0.0.1:11122", "http://localhost:11122"]);
const startupURLs = new Map();
const startupReady = new Set();
const recoveryAttempted = new Set();

function uiSender(sender) {
  try {
    const url = new URL(sender.url);
    return sender.id === chrome.runtime.id && sender.frameId === 0 && Number.isInteger(sender.tab?.id)
      && uiOrigins.has(url.origin) && url.pathname === "/";
  } catch {
    return false;
  }
}

function validStartupURL(value, sender) {
  try {
    const url = new URL(value);
    const senderURL = new URL(sender.url);
    return uiOrigins.has(url.origin) && url.origin === senderURL.origin && url.pathname === "/"
      && /^#aku-startup=[a-f0-9]{64}$/.test(url.hash);
  } catch {
    return false;
  }
}

chrome.tabs.onRemoved.addListener((tabId) => {
  startupURLs.delete(tabId);
  startupReady.delete(tabId);
  recoveryAttempted.delete(tabId);
});

chrome.runtime.onMessage.addListener((message, sender, reply) => {
  void (async () => {
    if (!uiSender(sender)) throw new Error("Reader click origin rejected.");
    const tabId = sender.tab.id;
    if (message?.type === "AKU_BROWSER_UI_STARTUP_WATCH") {
      if (!validStartupURL(message.startupUrl, sender)) throw new Error("UI startup recovery origin rejected.");
      if (!startupURLs.has(tabId)) startupURLs.set(tabId, message.startupUrl);
      return { ok: true };
    }
    if (message?.type === "AKU_BROWSER_UI_STARTUP_READY") {
      startupReady.add(tabId);
      startupURLs.delete(tabId);
      return { ok: true };
    }
    if (message?.type === "AKU_BROWSER_UI_STARTUP_RECOVER") {
      if (!validStartupURL(message.startupUrl, sender) || startupURLs.get(tabId) !== message.startupUrl) {
        throw new Error("UI startup recovery capability rejected.");
      }
      if (startupReady.has(tabId) || recoveryAttempted.has(tabId)) return { ok: true, retried: false };
      recoveryAttempted.add(tabId);
      // Same tab, same window, same profile. tabs.update performs the proven
      // Ctrl+R-equivalent navigation without creating or activating a window.
      await chrome.tabs.update(tabId, { url: message.startupUrl });
      return { ok: true, retried: true };
    }
    if (!/^broker_[a-f0-9]{32}$/.test(message?.requestId ?? "")) {
      throw new Error("Reader click origin rejected.");
    }
    const tab = await chrome.tabs.get(tabId);
    const window = await chrome.windows.get(tab.windowId);
    if (!tab.active || !window.focused) throw new Error("AkuBrowser UI must be active.");
    // sendNativeMessage launches a fresh executable for each explicit click.
    return chrome.runtime.sendNativeMessage("com.akubrowser.reader_activation", {
      requestId: message.requestId, source: message.source, url: message.url,
    });
  })().then(reply, (error) => reply({ ok: false, message: String(error?.message ?? error) }));
  return true;
});
