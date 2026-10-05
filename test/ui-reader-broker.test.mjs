import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import vm from "node:vm";
import crypto from "node:crypto";
const root = new URL("../ui-reader-broker/", import.meta.url);

test("startup messaging failures cannot prevent trusted-click listener installation", async () => {
  for (const synchronous of [true, false]) {
    const listeners = new Map(); const messages = []; const calls = [];
    let click;
    const window = { addEventListener: (name, fn) => listeners.set(name, fn), postMessage: (m) => messages.push(m) };
    window.top = window;
    const document = { visibilityState: "visible", addEventListener: (_name, fn) => { click = fn; } };
    const context = {
      window, document, setTimeout() {}, crypto: { randomUUID: () => "a".repeat(32) },
      location: { pathname: "/", origin: "http://127.0.0.1:11122", hash: "#aku-startup=" + "b".repeat(64), href: "private-startup-url" },
      chrome: { runtime: { sendMessage(message) {
        assert.equal(typeof click, "function", "listener must exist before messaging");
        calls.push(message);
        if (!message.type) return Promise.resolve({ ok: true });
        if (synchronous) throw new Error("private sync failure");
        return Promise.reject(new Error("private async failure"));
      } } },
    };
    vm.runInNewContext(fs.readFileSync(new URL("content.js", root), "utf8"), context);
    await new Promise(setImmediate);
    assert.ok(messages.some((m) => m.phase === "broker_startup_error"));
    assert.ok(messages.some((m) => m.type === "AKU_BROWSER_READER_BROKER_READY"));
    assert.doesNotMatch(JSON.stringify(messages), /private/);
    const link = { dataset: { akuNativePost: "x" }, href: "https://x.com/a/status/1" };
    for (const reason of ["Menunggu koleksi selesai", "Membuka native post…"]) {
      link.dataset.akuNativeWait = reason;
      const before = calls.length;
      click({ isTrusted: true, button: 0, target: { closest: () => link } });
      assert.equal(calls.length, before, "blocked click must not launch another helper");
      assert.equal(link.dataset.akuReaderRequest, undefined);
    }
    delete link.dataset.akuNativeWait;
    click({ isTrusted: true, button: 0, target: { closest: () => link } });
    assert.equal(calls.at(-1).requestId, "broker_" + "a".repeat(32));
    assert.equal(link.dataset.akuReaderRequest, calls.at(-1).requestId);
  }
});

test("broker identity and permissions isolate it from source cookies and capture", () => {
  const manifest = JSON.parse(fs.readFileSync(new URL("manifest.json", root)));
  const id = crypto.createHash("sha256").update(Buffer.from(manifest.key, "base64")).digest("hex").slice(0,32).replace(/[0-9a-f]/g, c => String.fromCharCode(97 + parseInt(c,16)));
  assert.equal(id,"dlibmmlopdahibfniinemhnghlifiple");
  assert.equal(manifest.version,"1.0.1");
  assert.deepEqual(manifest.permissions,["nativeMessaging"]);
  assert.ok(manifest.host_permissions.every(x => /^http:\/\/(127\.0\.0\.1|localhost):11122\//.test(x)));
});

test("blank startup recovery quietly retries the same UI tab at most once", async () => {
  let listener; let removed; const updates=[];
  const chrome={
    runtime:{id:"dlibmmlopdahibfniinemhnghlifiple",onMessage:{addListener(fn){listener=fn;}}},
    tabs:{
      onRemoved:{addListener(fn){removed=fn;}},
      update:async(id,change)=>{updates.push({id,change});return {id};},
      get:async()=>({active:true,windowId:7}),
    },
    windows:{get:async()=>({focused:true})},
  };
  vm.runInNewContext(fs.readFileSync(new URL("service-worker.js",root),"utf8"),{chrome,URL,Map,Set,Number});
  const startupUrl="http://127.0.0.1:11122/#aku-startup="+"a".repeat(64);
  const sender={id:chrome.runtime.id,frameId:0,url:startupUrl,tab:{id:19}};
  const send=(message)=>new Promise(resolve=>listener(message,sender,resolve));
  assert.equal((await send({type:"AKU_BROWSER_UI_STARTUP_WATCH",startupUrl})).ok,true);
  assert.equal((await send({type:"AKU_BROWSER_UI_STARTUP_RECOVER",startupUrl})).retried,true);
  assert.equal(updates.length,1);
  assert.equal(updates[0].id,19);
  assert.equal(updates[0].change.url,startupUrl);
  assert.equal((await send({type:"AKU_BROWSER_UI_STARTUP_RECOVER",startupUrl})).retried,false);
  assert.equal(updates.length,1);
  removed(19);
});

test("startup content watchdog requests recovery only when application ready is missing", async () => {
  const startupUrl="http://127.0.0.1:11122/#aku-startup="+"b".repeat(64);
  const makeDocument=()=>{
    const messages=[],listeners={};let timer;
    const window={postMessage(){},addEventListener(type,fn){listeners[type]=fn;}};window.top=window;
    const document={visibilityState:"visible",addEventListener(){}};
    const context={
      window,document,
      location:{pathname:"/",origin:"http://127.0.0.1:11122",hash:"#aku-startup="+"b".repeat(64),href:startupUrl},
      setTimeout(fn,delay){assert.equal(delay,8_000);timer=fn;},
      crypto:{randomUUID:()=>"a".repeat(32)},
      chrome:{runtime:{sendMessage:async message=>{messages.push(message);return {ok:true};}}},
    };
    return {context,messages,listeners,fireTimer:()=>timer()};
  };
  const first=makeDocument();
  vm.runInNewContext(fs.readFileSync(new URL("content.js",root),"utf8"),first.context);
  await Promise.resolve();
  assert.equal(first.messages[0].type,"AKU_BROWSER_UI_STARTUP_WATCH");
  assert.equal(first.messages[0].startupUrl,startupUrl);
  first.fireTimer();
  await Promise.resolve();
  assert.equal(first.messages[1].type,"AKU_BROWSER_UI_STARTUP_RECOVER");

  const second=makeDocument();
  vm.runInNewContext(fs.readFileSync(new URL("content.js",root),"utf8"),second.context);
  await Promise.resolve();
  assert.equal(second.messages[0].type,"AKU_BROWSER_UI_STARTUP_WATCH");
  second.listeners["aku-startup-stage"]({detail:"ready"});
  await Promise.resolve();
  second.fireTimer();
  await Promise.resolve();
  assert.deepEqual(second.messages.map(message=>message.type),["AKU_BROWSER_UI_STARTUP_WATCH","AKU_BROWSER_UI_STARTUP_READY"]);
});

test("trusted clicks launch one helper and next-task cleanup uses exact admission IDs", async () => {
  let handler; const calls=[], messages=[], cleanup=[];
  const ids=["a","b","c"].map((value)=>value.repeat(32));
  const link={dataset:{akuNativePost:"x"},href:"https://x.com/a/status/1"};
  const window={postMessage(message){messages.push(message);},addEventListener(){}};window.top=window;
  const document={visibilityState:"visible",addEventListener(type,fn,capture){assert.equal(type,"click");assert.equal(capture,true);handler=fn;}};
  const context={window,document,location:{pathname:"/",origin:"http://127.0.0.1:11122"},
    setTimeout(fn,delay){if(delay===0)cleanup.push(fn);},
    crypto:{randomUUID:()=>ids.shift()},
    chrome:{runtime:{sendMessage:async req=>{calls.push(req);return {ok:true}}}}};
  const script=fs.readFileSync(new URL("content.js",root),"utf8");
  vm.runInNewContext(script,context);
  vm.runInNewContext(script,context);
  const event={isTrusted:false,button:0,target:{closest:()=>link}};
  handler(event);assert.equal(calls.length,0);
  handler({...event,isTrusted:true,defaultPrevented:true});assert.equal(calls.length,0);
  handler({...event,isTrusted:true,button:1});assert.equal(calls.length,0);
  document.visibilityState="hidden";handler({...event,isTrusted:true});assert.equal(calls.length,0);
  document.visibilityState="visible";
  const admitted={...event,isTrusted:true};
  handler(admitted);assert.equal(calls.length,1);
  assert.equal(link.dataset.akuReaderRequest,calls[0].requestId);
  assert.equal(calls[0].source,"x");assert.equal(calls[0].url,link.href);
  // Model the app's synchronous bubble admission and its own preventDefault.
  link.dataset.akuReaderRequestsConsumed=calls[0].requestId;
  delete link.dataset.akuReaderRequest;
  admitted.defaultPrevented=true;
  const secondAdmitted={...event,isTrusted:true};
  handler(secondAdmitted);assert.equal(calls.length,2);
  link.dataset.akuReaderRequestsConsumed += `,${calls[1].requestId}`;
  delete link.dataset.akuReaderRequest;
  cleanup.shift()();
  cleanup.shift()();
  assert.equal(messages.some((message)=>message.type==="AKU_BROWSER_CANCEL_NATIVE_POST_BROKER"),false);

  const blocked={...event,isTrusted:true};
  handler(blocked);assert.equal(calls.length,3);
  const blockedId=calls[2].requestId;
  cleanup.shift()();
  const cancellation=messages.find((message)=>message.type==="AKU_BROWSER_CANCEL_NATIVE_POST_BROKER");
  assert.equal(cancellation.requestId,blockedId);
  assert.equal(cancellation.source,"x");assert.equal(cancellation.url,link.href);
  assert.equal(link.dataset.akuReaderRequest,undefined);
});

test("reader helper diagnostics correlate the outcome without logging the native URL", async () => {
  let listener;
  let clock = 0;
  const logs = [];
  const chrome = {
    runtime: {
      id: "dlibmmlopdahibfniinemhnghlifiple",
      onMessage: { addListener(fn) { listener = fn; } },
      sendNativeMessage: async () => ({ ok: false, message: "Reader broker is unavailable or busy" }),
    },
    tabs: { onRemoved: { addListener() {} }, get: async () => ({ active: true, windowId: 7 }) },
    windows: { get: async () => ({ focused: true }) },
  };
  vm.runInNewContext(fs.readFileSync(new URL("service-worker.js", root), "utf8"), {
    chrome, URL, Map, Set, Number, Math,
    performance: { now: () => clock += 12 },
    console: { info: (...entry) => logs.push(entry) },
  });
  const requestId = "broker_" + "a".repeat(32);
  const url = "https://x.com/private/status/123";
  const sender = { id: chrome.runtime.id, frameId: 0, url: "http://127.0.0.1:11122/", tab: { id: 19 } };
  const reply = await new Promise(resolve => listener({ requestId, source: "x", url }, sender, resolve));
  assert.equal(reply.ok, false);
  assert.equal(logs.length, 2);
  assert.equal(logs[0][1].phase, "helper_start");
  assert.equal(logs[1][1].phase, "helper_result");
  assert.equal(logs[1][1].outcome, "rejected");
  assert.equal(logs[1][1].failureKind, "pipe_busy");
  assert.equal(logs[1][1].elapsedMs, 12);
  assert.ok(logs.every(([, value]) => value.requestId === requestId));
  assert.ok(!JSON.stringify(logs).includes(url));
});

test("foreground refusal diagnostics retain only fixed focus categories and visibility", async () => {
  let listener;
  const logs=[];
  const result={ok:false,message:"Reader intent expired or UI foreground changed",focusCategory:"ui",
    readerVisible:false,readerMinimized:true,title:"private window title",url:"https://x.com/private"};
  const chrome={
    runtime:{id:"dlibmmlopdahibfniinemhnghlifiple",onMessage:{addListener(fn){listener=fn;}},sendNativeMessage:async()=>result},
    tabs:{onRemoved:{addListener(){}},get:async()=>({active:true,windowId:7})},
    windows:{get:async()=>({focused:true})},
  };
  vm.runInNewContext(fs.readFileSync(new URL("service-worker.js",root),"utf8"),{
    chrome,URL,Map,Set,Number,Math,performance:{now:()=>0},console:{info:(...entry)=>logs.push(entry)},
  });
  const requestId="broker_"+"d".repeat(32);
  const sender={id:chrome.runtime.id,frameId:0,url:"http://127.0.0.1:11122/",tab:{id:19}};
  const reply=await new Promise(resolve=>listener({requestId,source:"x",url:result.url},sender,resolve));
  assert.equal(reply.message,result.message);
  const diagnostic=logs.at(-1)[1];
  assert.equal(diagnostic.failureKind,"ui_foreground_changed");
  assert.equal(diagnostic.focusCategory,"ui");
  assert.equal(diagnostic.readerVisible,false);
  assert.equal(diagnostic.readerMinimized,true);
  assert.doesNotMatch(JSON.stringify(logs),/private window title|x\.com\/private/);
});

test("development restart registers the staged broker with explicit takeover fencing", () => {
  const repository = new URL("../", import.meta.url);
  const restart = fs.readFileSync(new URL("scripts/restart-dev.ps1", repository), "utf8");
  const register = fs.readFileSync(new URL("scripts/register-reader-broker-dev.ps1", repository), "utf8");
  assert.match(restart, /register-reader-broker-dev\.ps1/);
  assert.match(restart, /-Register/);
  assert.match(restart, /-ReplaceExistingRegistration:\$ReplaceReaderBrokerRegistration/);
  assert.match(register, /Preflight both vendors before any registry write/);
  assert.match(register, /-not \$ReplaceExistingRegistration/);
  assert.match(register, /Google\\Chrome/);
  assert.match(register, /Chromium/);
});

test("development restart inspects the candidate against existing data before stopping Sidecar", () => {
  const restart = fs.readFileSync(new URL("scripts/restart-dev.ps1", new URL("../", import.meta.url)), "utf8");
  const inspect = restart.indexOf("--database-inspect");
  const register = restart.indexOf("'register-reader-broker-dev.ps1'");
  const stop = restart.indexOf("& $supervisor stop akusidecar");
  assert.ok(inspect >= 0 && register > inspect && stop > register);
  assert.match(restart, /inspection\.status -notin @\('absent', 'current', 'migratable'\)/);
});
