import test from "node:test";
import assert from "node:assert/strict";
import { createPostScreenshotUI } from "../internal/httpapi/web/post-screenshot-ui.js";
import { readFileSync } from "node:fs";
import vm from "node:vm";

class FakeElement {
  constructor(tagName, document) {
    this.tagName = tagName.toUpperCase();
    this.ownerDocument = document;
    this.children = [];
    this.attributes = new Map();
    this.listeners = new Map();
    this.style = {};
    this.hidden = false;
    this.disabled = false;
    this.open = false;
    this.isConnected = true;
    this.value = "";
    this.textContent = "";
    this.className = "";
  }

  append(...children) {
    for (const child of children) {
      child.parentNode?.removeChild(child);
      child.parentNode = this;
      child.isConnected = true;
      this.children.push(child);
    }
  }

  replaceChildren(...children) {
    for (const child of this.children) child.isConnected = false;
    this.children = [];
    this.append(...children);
  }

  removeChild(child) {
    this.children = this.children.filter((value) => value !== child);
    child.parentNode = null;
    child.isConnected = false;
  }

  remove() { this.parentNode?.removeChild(this); }

  setAttribute(name, value) { this.attributes.set(name, String(value)); }
  getAttribute(name) { return this.attributes.get(name) ?? null; }

  addEventListener(type, listener) {
    const listeners = this.listeners.get(type) || [];
    listeners.push(listener);
    this.listeners.set(type, listeners);
  }

  async dispatch(type, values = {}) {
    if (this.disabled) return [];
    const event = {
      type,
      target: this,
      defaultPrevented: false,
      preventDefault() { this.defaultPrevented = true; },
      ...values,
    };
    const results = [];
    for (const listener of this.listeners.get(type) || []) results.push(await listener(event));
    return results;
  }

  contains(target) {
    if (target === this) return true;
    return this.children.some((child) => child.contains(target));
  }

  focus() { this.ownerDocument.activeElement = this; }

  showModal() { this.open = true; }

  close() {
    this.open = false;
    this.dispatch("close");
  }
}

class FakeDocument {
  constructor() {
    this.activeElement = null;
    this.listeners = new Map();
    this.body = new FakeElement("body", this);
    this.documentElement = new FakeElement("html", this);
    this.documentElement.style.scrollbarGutter = "auto";
  }

  createElement(tagName) { return new FakeElement(tagName, this); }

  addEventListener(type, listener) {
    const listeners = this.listeners.get(type) || [];
    listeners.push(listener);
    this.listeners.set(type, listeners);
  }

  dispatch(type, values = {}) {
    const event = {
      type,
      target: values.target || this,
      defaultPrevented: false,
      preventDefault() { this.defaultPrevented = true; },
      ...values,
    };
    for (const listener of this.listeners.get(type) || []) listener(event);
    return event;
  }

}

function fixture({ request, clipboard, ClipboardItem, getOriginalUrl } = {}) {
  const document = new FakeDocument();
  const window = {
    Blob,
    atob: (value) => globalThis.atob(value),
    ClipboardItem,
    navigator: clipboard ? { clipboard } : {},
  };
  const ui = createPostScreenshotUI({
    document,
    window,
    getOriginalUrl,
    request: request || (async () => ({ parts: [{ data: "iVBORw0KGgo=", width: 10, height: 20 }], warnings: [], filename: "post.png" })),
  });
  const findByClass = (root, name) => [root, ...root.children.flatMap((child) => flatten(child))].find((element) => element.className.split(" ").includes(name));
  const flatten = (root) => [root, ...root.children.flatMap((child) => flatten(child))];
  return { ui, document, window, findByClass };
}

test("Copy original URL copies text independently without opening native reader or screenshot", async () => {
  const copied = [];
  const {ui,document,findByClass}=fixture({
    request:()=>assert.fail("URL copy must not generate a screenshot"),
    getOriginalUrl:entry=>entry.original,
    clipboard:{writeText:async value=>{copied.push(value);}},
  });
  const root=ui.createMenu({original:"https://x.com/example/status/123"});document.body.append(root);
  const trigger=findByClass(root,"post-screenshot-menu-button");
  const copy=findByClass(root,"post-original-url-copy");
  await trigger.dispatch("click");
  await copy.dispatch("click");
  assert.deepEqual(copied,["https://x.com/example/status/123"]);
  assert.equal(findByClass(root,"post-original-url-status").textContent,"Original URL copied.");
  assert.equal(ui.dialog.open,false);
  assert.equal(copy.disabled,false);
  await copy.dispatch("keydown",{key:"ArrowUp"});
  assert.equal(document.activeElement,findByClass(root,"post-screenshot-menu-item"));
  await trigger.dispatch("keydown",{key:"ArrowUp"});
  assert.equal(document.activeElement,copy);
});

test("URL copy is disabled without a validated original URL and skips it during keyboard navigation", async () => {
  const {ui,document,findByClass}=fixture({getOriginalUrl:()=>null,clipboard:{writeText:()=>assert.fail("missing URL copied")}});
  const root=ui.createMenu({});document.body.append(root);
  const copy=findByClass(root,"post-original-url-copy");
  assert.equal(copy.disabled,true);
  assert.equal(copy.title,"Original post URL unavailable");
  await copy.dispatch("click");
  await findByClass(root,"post-screenshot-menu-button").dispatch("keydown",{key:"ArrowUp"});
  assert.equal(document.activeElement,findByClass(root,"post-screenshot-menu-item"));
});

test("URL clipboard denial or missing API stays visible and permits retry", async () => {
  for(const clipboard of [{writeText:async()=>{throw new Error("denied");}},undefined]){
    const {ui,document,findByClass}=fixture({getOriginalUrl:()=>"https://x.com/example/status/123",clipboard});
    const root=ui.createMenu({});document.body.append(root);
    await findByClass(root,"post-screenshot-menu-button").dispatch("click");
    const copy=findByClass(root,"post-original-url-copy");
    await copy.dispatch("click");
    assert.equal(findByClass(root,"post-original-url-status").hidden,false);
    assert.match(findByClass(root,"post-original-url-status").textContent,/Could not copy URL/);
    assert.equal(copy.disabled,false);
    assert.equal(copy.getAttribute("aria-busy"),"false");
    assert.equal(ui.dialog.open,false);
  }
});

test("Copy URL and Open native post share the actual source URL validator and permalink selection",()=>{
  const app=readFileSync(new URL("../internal/httpapi/web/app.js",import.meta.url),"utf8");
  const context=vm.createContext({URL,safeMediaUrl:value=>{try {const u=new URL(value);return u.protocol==="https:"?u.href:null;}catch{return null;}},sourceDescriptor:source=>source==="x"?{nativeHosts:["x.com","twitter.com"],nativePathTokens:["/status/"]}:null});
  vm.runInContext(app.slice(app.indexOf("function originalPostUrl("),app.indexOf("function mediaRecaptureReasonForEntry(")),context);
  assert.equal(context.originalPostUrl({source:"x",item:{sourceUrl:"https://twitter.com/author/status/123/photo/1?tracking=1"}}),"https://x.com/author/status/123");
  assert.equal(context.originalPostUrl({source:"x",evidence:{permalink:"https://x.com/author/status/456"}}),"https://x.com/author/status/456");
  for(const url of ["javascript:alert(1)","https://attacker.test/author/status/123","https://x.com/home",""]){assert.equal(context.originalPostUrl({source:"x",item:{sourceUrl:url}}),null);}
  assert.match(app,/getOriginalUrl:\s*\(entry\) => originalPostUrl\(entry\)/);
  assert.match(app,/function buildSourceLink\(entry\)\s*\{\s*const href = originalPostUrl\(entry\)/);
});

test("menu stays single-open, supports keyboard dismissal, and returns focus", () => {
  const { ui, document, findByClass } = fixture();
  const first = ui.createMenu({ id: "first" });
  const second = ui.createMenu({ id: "second" });
  document.body.append(first, second);
  const firstButton = findByClass(first, "post-screenshot-menu-button");
  const secondButton = findByClass(second, "post-screenshot-menu-button");
  const firstMenu = findByClass(first, "post-screenshot-menu-list");
  const secondMenu = findByClass(second, "post-screenshot-menu-list");

  firstButton.dispatch("click");
  assert.equal(firstMenu.hidden, false);
  assert.equal(firstButton.getAttribute("aria-expanded"), "true");
  secondButton.dispatch("click");
  assert.equal(firstMenu.hidden, true);
  assert.equal(secondMenu.hidden, false);
  assert.equal(secondButton.getAttribute("aria-expanded"), "true");

  const escaped = document.dispatch("keydown", { key: "Escape" });
  assert.equal(escaped.defaultPrevented, true);
  assert.equal(secondMenu.hidden, true);
  assert.equal(document.activeElement, secondButton);

  firstButton.dispatch("keydown", { key: "ArrowDown" });
  assert.equal(firstMenu.hidden, false);
  assert.equal(document.activeElement, findByClass(first, "post-screenshot-menu-item"));
  document.dispatch("pointerdown", { target: secondButton });
  assert.equal(firstMenu.hidden, true);
});

test("multi-part previews select the current part for image clipboard copy and numbered downloads", async () => {
  const calls = [];
  const copied = [];
  class FakeClipboardItem {
    constructor(items) { this.items = items; }
  }
  const { ui, document, findByClass } = fixture({
    request: async (...args) => {
      calls.push(args);
      return {
        parts: [
          { data: "iVBORw0KGgo=", width: 800, height: 8192 },
          { data: "iVBORw0KGgo=", width: 800, height: 420 },
        ],
        warnings: ["Long post rendered in two parts."],
        filename: "captured-post.png",
      };
    },
    ClipboardItem: FakeClipboardItem,
    clipboard: { write: async (items) => { copied.push(items); } },
  });
  const menuRoot = ui.createMenu({ id: "timeline-entry" });
  document.body.append(menuRoot);
  const trigger = findByClass(menuRoot, "post-screenshot-menu-button");
  trigger.dispatch("click");
  findByClass(menuRoot, "post-screenshot-menu-item").dispatch("click");
  await new Promise(setImmediate);

  const dialog = ui.dialog;
  assert.equal(dialog.open, true);
  assert.equal(calls.length, 1);
  assert.equal(calls[0][0], "/api/post-screenshots");
  assert.equal(calls[0][1].method, "POST");
  assert.deepEqual(calls[0][1].body, { timelineId: "timeline-entry", theme: "dark" });
  assert.ok(calls[0][1].signal instanceof AbortSignal);
  assert.equal(findByClass(dialog, "post-screenshot-status").textContent, "2 screenshot parts ready.");
  assert.match(findByClass(dialog, "post-screenshot-warnings").children[0].textContent, /two parts/);
  assert.equal(findByClass(dialog, "post-screenshot-preview").children[0].src, "data:image/png;base64,iVBORw0KGgo=");
  assert.equal(findByClass(dialog, "post-screenshot-save-links").children[0].download, "captured-post-01.png");
  assert.equal(findByClass(dialog, "post-screenshot-save-links").children[1].download, "captured-post-02.png");

  findByClass(dialog, "post-screenshot-part-navigation").children[2].dispatch("click");
  assert.equal(findByClass(dialog, "post-screenshot-preview").children[0].alt, "Screenshot part 2 of 2");
  await findByClass(dialog, "post-screenshot-copy").dispatch("click");
  assert.equal(copied.length, 1);
  assert.equal(copied[0][0].items["image/png"].type, "image/png");
  assert.match(findByClass(dialog, "post-screenshot-status").textContent, /Part 2 copied/);

  findByClass(dialog, "post-screenshot-close").dispatch("click");
  assert.equal(dialog.open, false);
  assert.equal(findByClass(dialog, "post-screenshot-preview").children.length, 0);
  assert.equal(findByClass(dialog, "post-screenshot-save-links").children.length, 0);
  assert.equal(document.activeElement, trigger);
  assert.equal(document.documentElement.style.scrollbarGutter, "auto");
});

test("clipboard denial keeps numbered Save PNG links available and explains the fallback", async () => {
  class FakeClipboardItem {
    constructor(items) { this.items = items; }
  }
  const { ui, document, findByClass } = fixture({
    ClipboardItem: FakeClipboardItem,
    clipboard: { write: async () => { throw new Error("NotAllowedError"); } },
    request: async () => ({
      parts: [{ data: "iVBORw0KGgo=", width: 800, height: 1000 }, { data: "iVBORw0KGgo=", width: 800, height: 700 }],
      warnings: [],
      filename: "capture.png",
    }),
  });
  const root = ui.createMenu({ id: "clipboard-denied" });
  document.body.append(root);
  findByClass(root, "post-screenshot-menu-button").dispatch("click");
  findByClass(root, "post-screenshot-menu-item").dispatch("click");
  await new Promise(setImmediate);
  const dialog = ui.dialog;
  await findByClass(dialog, "post-screenshot-copy").dispatch("click");

  assert.match(findByClass(dialog, "post-screenshot-error").textContent, /Clipboard access was denied\. Use Save PNG instead\./);
  const saveLinks = findByClass(dialog, "post-screenshot-save-links").children;
  assert.deepEqual(saveLinks.map((link) => link.download), ["capture-01.png", "capture-02.png"]);
  assert.equal(saveLinks.every((link) => link.href.startsWith("data:image/png;base64,")), true);
  findByClass(dialog, "post-screenshot-close").dispatch("click");
});

test("closing aborts in-flight requests, while request failures stay visible in the dialog", async () => {
  let resolveRequest;
  const requestSignals = [];
  const { ui, document, findByClass } = fixture({
    request: (path, options) => {
      requestSignals.push(options.signal);
      if (requestSignals.length === 1) return new Promise((resolve) => { resolveRequest = resolve; });
      return Promise.reject(Object.assign(new Error("Screenshot endpoint unavailable"), { code: "unavailable" }));
    },
  });
  const root = ui.createMenu({ id: "cancel-me" });
  document.body.append(root);
  const trigger = findByClass(root, "post-screenshot-menu-button");
  const action = findByClass(root, "post-screenshot-menu-item");
  trigger.dispatch("click");
  action.dispatch("click");
  const dialog = ui.dialog;
  const closeButton = findByClass(dialog, "post-screenshot-close");
  const firstSignal = requestSignals[0];
  assert.equal(firstSignal.aborted, false);
  closeButton.dispatch("click");
  assert.equal(firstSignal.aborted, true);
  resolveRequest({ parts: [{ data: "iVBORw0KGgo=", width: 1, height: 1 }], warnings: [], filename: "late.png" });
  await new Promise(setImmediate);
  assert.equal(findByClass(dialog, "post-screenshot-preview").children.length, 0);
  assert.equal(document.activeElement, trigger);

  trigger.dispatch("click");
  action.dispatch("click");
  await new Promise(setImmediate);
  assert.equal(findByClass(dialog, "post-screenshot-error").textContent, "Screenshot endpoint unavailable");
  assert.equal(findByClass(dialog, "post-screenshot-error").hidden, false);
  closeButton.dispatch("click");
});
