// Explicit loopback fixture only. Every receipt has OFFLINE provenance in Go;
// actual Chrome/Windows ownership does not qualify native X account evidence.
import readline from 'node:readline';
import { resolve } from 'node:path';
import { pathToFileURL } from 'node:url';
import { performance } from 'node:perf_hooks';
import { launchFixtureChrome } from './chrome.mjs';
import { inventoryFixtureWindows, closeFixtureWindow } from './chrome-window-lifecycle.mjs';
import { concurrentFixtureCookie } from './concurrent-engine-commands.mjs';
import { createManualStartupOwner } from './x-manual-startup-owner.mjs';
import { projectQADirectBody } from './x-qa-direct-ab.mjs';
import { parseStrictJSON } from './x-evidence-ingress.mjs';
import { captureManualFixtureDOM } from './chrome-manual-pilot-capture.mjs';

const fail = () => new Error('fixture_manual_rejected');
const ref = v => typeof v === 'string' && v.length > 0 && v.length <= 512 && !/[\u0000-\u001f\u007f]/u.test(v);
// Leave time for the Go owner to receive a closed stage category before its
// stricter joint two-role deadline expires. This never extends admission.
export async function boundedFixtureAcquire(operation, milliseconds = 1750) {
  let timer, expired=false;
  const guarded=Promise.resolve(operation).then(value=>{
    if(expired){if(Buffer.isBuffer(value))value.fill(0);throw fail();}
    return value;
  });
  try { return await Promise.race([guarded, new Promise((_, reject) => {
    timer = setTimeout(() => {expired=true;reject(fail());}, milliseconds);
  })]); } finally { clearTimeout(timer); }
}
export function manualChromeCommand(raw) {
  const fields = { fixture_bootstrap: ['op','withholdReady'], launch: ['id','op','role','nativeReaderStartup'],
    login: ['id','op','owner'], seed_fixture: ['id','op','owner','value'], prepare: ['id','op','owner'],
    acquire: ['id','op','owner','handle','request'], inspect: ['id','op','owner'], capture_fixture: ['id','op','owner'],
    inventory: ['id','op','owner'], open_independent_window: ['id','op','owner'],
    context_drift: ['id','op','owner'], close_window:['id','op','owner','index'], abort: ['id','op','owner','handle','preserveReader'], release: ['id','op','owner'] }[raw?.op];
  if (!raw || ![Object.prototype,null].includes(Object.getPrototypeOf(raw)) || !fields ||
      Reflect.ownKeys(raw).length !== fields.length || Reflect.ownKeys(raw).some(k => !fields.includes(k))) throw fail();
  if (raw.op === 'fixture_bootstrap') { if (raw.withholdReady !== false) throw fail(); return raw; }
  if (!Number.isSafeInteger(raw.id) || raw.id < 1) throw fail();
  if (raw.op === 'launch') {
    if (!['owner','collector'].includes(raw.role) || raw.nativeReaderStartup !== (raw.role === 'owner')) throw fail();
  } else if (raw.owner !== 1) throw fail();
  if (raw.op === 'seed_fixture' && (typeof raw.value!=='string' || !/^[a-f0-9]{64}$/.test(raw.value))) throw fail();
  if (raw.op === 'acquire' && (!ref(raw.handle) || !raw.request || typeof raw.request !== 'object')) throw fail();
  if (raw.op === 'abort' && (!ref(raw.handle) || typeof raw.preserveReader !== 'boolean')) throw fail();
  if (raw.op === 'close_window' && (!Number.isInteger(raw.index) || raw.index<0 || raw.index>15)) throw fail();
  return raw;
}

export async function runManualChromeDriver(executable, root, origin) {
  if (process.env.AKU_NATIVE_MANUAL_CHROME_FIXTURE !== '1') throw fail();
  concurrentFixtureCookie('0'.repeat(64), origin);
  let browser, page, bootstrapTarget, started = false, owner = 0, login = false, seed = false, actor, handle, observation;
  let phase='idle';
  let capturing=false;
  let captureCancel;
  const emit = value => process.stdout.write(JSON.stringify(value) + '\n');
  const inventory = () => inventoryFixtureWindows(browser, origin, bootstrapTarget);
  const currentPins = () => {
    if (!observation || observation.invalid || !observation.loader || !observation.context) throw fail();
    return Object.freeze({ ownerId: String(browser.pid), profileId: browser.profile, contextId: String(observation.context),
      targetId: page.targetId, frameId: observation.frame, documentId: observation.loader });
  };
  async function startup(sink) {
    let sessionId, unsubscribe, paused = false;
    let resolvePause, rejectPause, resolveBody, rejectBody;
    const pause = new Promise((yes,no) => { resolvePause=yes; rejectPause=no; });
    const body = new Promise((yes,no) => { resolveBody=yes; rejectBody=no; });
    void pause.catch(() => {}); void body.catch(() => {});
    observation = { invalid: false, frame: null, loader: null, context: null, callFrame: null,
      request: null, settingsAt: null, fresh: false, status: 0, scripts: new Map() };
    const invalidate = () => { observation.invalid=true; rejectPause(fail()); rejectBody(fail()); };
    const onAbort = () => invalidate();
    sink.signal.addEventListener('abort', onAbort, { once:true });
    try {
      phase='setup';
      sessionId = (await browser.send('Target.attachToTarget', { targetId: page.targetId, flatten:true })).sessionId;
      unsubscribe = browser.subscribeProtocol(event => {
        if (event.method==='Target.targetDestroyed' && event.params?.targetId===page.targetId) { invalidate(); return; }
        if (event.sessionId!==sessionId) return;
        const p=event.params??{};
        if (['Inspector.detached','Inspector.targetCrashed','Page.frameDetached'].includes(event.method)) { invalidate(); return; }
        if (event.method==='Page.frameNavigated') {
          if (p.frame?.parentId) return;
          if (observation.loader || p.frame?.url!==origin+'/startup' || !ref(p.frame?.id) || !ref(p.frame?.loaderId)) { invalidate(); return; }
          observation.frame=p.frame.id; observation.loader=p.frame.loaderId;
        }
        if (event.method==='Runtime.executionContextsCleared' && observation.context) invalidate();
        if (event.method==='Runtime.executionContextDestroyed' && p.executionContextId===observation.context) invalidate();
        if (event.method==='Runtime.executionContextCreated' && observation.loader && p.context?.auxData?.isDefault && p.context.auxData.frameId===observation.frame) {
          if (observation.context || p.context.origin!==origin || !Number.isSafeInteger(p.context.id)) invalidate();
          else observation.context=p.context.id;
        }
        if (event.method==='Debugger.scriptParsed' && p.executionContextId===observation.context && p.url===origin+'/startup') {
          if (observation.scripts.size>=64) invalidate();
          else observation.scripts.set(p.scriptId,p.executionContextId);
        }
        if (event.method==='Debugger.paused') {
          const top=p.callFrames?.[0];
          if (paused || !top || p.reason==='instrumentation' || observation.scripts.get(top.location?.scriptId)!==observation.context) invalidate();
          else { paused=true; observation.callFrame=top.callFrameId; resolvePause(); }
        }
        if (event.method==='Network.requestWillBeSent' && p.request?.url===origin+'/settings') {
          if (observation.request || p.request.method!=='GET' || p.redirectResponse || p.frameId!==observation.frame || p.loaderId!==observation.loader) invalidate();
          else observation.request=p.requestId;
        }
        if (p.requestId && p.requestId===observation.request) {
          if (event.method==='Network.requestServedFromCache' || event.method==='Network.loadingFailed') invalidate();
          if (event.method==='Network.responseReceived') {
            observation.status=p.response?.status;
            observation.fresh=p.response?.fromDiskCache===false && p.response?.fromServiceWorker===false &&
              p.response?.fromPrefetchCache!==true && p.response?.mimeType==='application/json' && p.loaderId===observation.loader;
          }
          if (event.method==='Network.loadingFinished') {
            if (!Number.isSafeInteger(p.encodedDataLength) || p.encodedDataLength<1 || p.encodedDataLength>4096 || observation.invalid) invalidate();
            else { observation.settingsAt=performance.now(); resolveBody(); }
          }
        }
      },invalidate);
      await browser.send('Page.enable',{},sessionId);
      await browser.send('Runtime.enable',{},sessionId);
      await browser.send('Debugger.enable',{},sessionId);
      await browser.send('Network.enable',{ maxTotalBufferSize:65536,maxResourceBufferSize:4096,maxPostDataSize:0 },sessionId);
      await browser.send('Network.setCacheDisabled',{cacheDisabled:true},sessionId);
      await browser.send('Page.navigate',{url:origin+'/startup'},sessionId);
      await pause;
      const pins=currentPins();
      phase='ready';
      await sink.beforeIdentityRead(pins);
      if (observation.invalid || !paused || JSON.stringify(currentPins())!==JSON.stringify(pins)) throw fail();
      phase='identity';
      const evaluated=await browser.send('Debugger.evaluateOnCallFrame',{callFrameId:observation.callFrame,
        expression:'(() => { const d = Object.getOwnPropertyDescriptor(globalThis, "__aku_fixture_self"); return d && typeof d.value === "string" ? d.value : null; })()',
        returnByValue:true,throwOnSideEffect:true,timeout:1500,silent:true,includeCommandLineAPI:false,generatePreview:false},sessionId);
      if (evaluated.exceptionDetails || evaluated.result?.type!=='string' || !/^[1-9][0-9]{0,19}$/.test(evaluated.result.value)) throw fail();
      const accountId=evaluated.result.value, identityAt=performance.now();
      await browser.send('Debugger.resume',{},sessionId); paused=false;
      phase='settings_wait';
      await body;
      if (observation.invalid || observation.status!==200 || !observation.fresh) throw fail();
      phase='settings_projection';
      const projection=projectQADirectBody(await browser.send('Network.getResponseBody',{requestId:observation.request},sessionId));
      if (projection.category!=='settings_match' || !projection.settingsMatch || observation.invalid) throw fail();
      phase='document_check';
      const frame=await browser.send('Page.getFrameTree',{},sessionId);
      if (frame.frameTree?.frame?.loaderId!==pins.documentId || frame.frameTree.frame.id!==pins.frameId) throw fail();
      await browser.send('Debugger.disable',{},sessionId);
      phase='publish';
      sink.onPrivateStartup(Object.freeze({ ...pins,accountId,settingsRequestId:observation.request,identityAt,
        settingsAt:observation.settingsAt,qualification:'api_v1.cookie-auth-rejection.2026-10-20' }));
      phase='captured';
    } finally {
      sink.signal.removeEventListener('abort',onAbort);
      phase='cleanup_resume';
      if (paused) await browser.send('Debugger.resume',{},sessionId).catch(() => {});
      phase='cleanup_debugger';
      if (sessionId) await browser.send('Debugger.disable',{},sessionId).catch(() => {});
      // Page/Runtime stay enabled for fixture lifecycle invalidation until abort.
      phase='cleanup_network';
      if (sessionId) await browser.send('Network.disable',{},sessionId).catch(() => {});
      observation.unsubscribe=unsubscribe;
      phase='complete';
    }
  }
  async function execute(message) {
    if (!started) { if(message.op!=='fixture_bootstrap')throw fail();started=true;return; }
    if(message.op==='fixture_bootstrap')throw fail();
    if(message.op==='launch') {
      if(browser)throw fail();
      browser=await launchFixtureChrome(executable,root,message.role,'shared',message.nativeReaderStartup?'native_reader':'retained');
      if(message.role==='owner') {
        const targets=await browser.send('Target.getTargets');
        const blank=targets.targetInfos.filter(t=>t.type==='page'&&t.url==='about:blank');
        if(blank.length!==1)throw fail();bootstrapTarget=blank[0].targetId;page=await browser.pageForTarget(origin,bootstrapTarget);
      } else page=await browser.page(origin);
      owner=1;const windows=await inventory();
      return{owner,pid:browser.pid,nativeReaderStartup:message.nativeReaderStartup,windowCount:windows.length,bootstrapCount:windows.filter(w=>w.bootstrap).length};
    }
    if(!browser||message.owner!==owner)throw fail();
    switch(message.op) {
      case 'capture_fixture': {
        if(process.env.AKU_MANUAL_CHROME_STORE_ENABLED!=='1'||capturing||browser.role!=='collector'||phase!=='complete')throw fail();
        capturing=true;
        captureCancel=new AbortController();
        const pins=currentPins();
        try {
          const result=await captureManualFixtureDOM(browser,origin,pins,captureCancel.signal);
          if(JSON.stringify(currentPins())!==JSON.stringify(pins))throw fail();
          return result;
        } finally {capturing=false;captureCancel=null;}
      }
      case 'login':if(browser.role!=='owner'||login)throw fail();login=true;return{status:(await page.request('/login','POST')).status};
      case 'seed_fixture':if(browser.role!=='collector'||seed)throw fail();await browser.send('Storage.setCookies',{cookies:[concurrentFixtureCookie(message.value,origin)]});seed=true;return{seeded:true};
      case 'prepare':if(actor)throw fail();actor=createManualStartupOwner({run:startup});{const ready=await actor.prepare();handle=ready.handle;return ready;}
      case 'acquire':{const wire=await boundedFixtureAcquire(actor.acquire(message.handle,message.request));try{return JSON.parse(wire.toString('utf8'));}finally{wire.fill(0);}}
      case 'inspect':{
        const windows=await inventory();
        return{pins:currentPins(),windowCount:windows.length,bootstrapCount:windows.filter(w=>w.bootstrap).length,
          paused:!!observation.callFrame&&!observation.settingsAt,invalid:observation.invalid,headless:browser.role==='collector'};
      }
      case 'inventory':{const windows=await inventory();return{windowCount:windows.length,bootstrapCount:windows.filter(w=>w.bootstrap).length};}
      case 'open_independent_window':if(browser.role!=='owner')throw fail();await browser.page(origin);return execute({op:'inventory',owner});
      case 'close_window':{
        const windows=await inventory(),selected=windows[message.index];if(!selected)throw fail();
        const result=await closeFixtureWindow(browser,selected,{finalWindow:windows.length===1,inventory,
          waitRootExit:async value=>{for(let i=0;i<200&&!value.isExited();i++)await new Promise(r=>setTimeout(r,25));return value.isExited();}});
        return{...result,remaining:browser.isExited()?0:(await inventory()).length,rootExited:browser.isExited()};
      }
      case 'context_drift':await browser.send('Target.closeTarget',{targetId:page.targetId});observation.invalid=true;return{changed:true};
      case 'abort':captureCancel?.abort();await actor.abort(message.handle);observation?.unsubscribe?.();if(!message.preserveReader)await browser.close();return{closed:true};
      case 'release':captureCancel?.abort();if(handle)await actor.abort(handle);observation?.unsubscribe?.();await browser.close();return{exited:browser.isExited()};
      default:throw fail();
    }
  }
  const input=readline.createInterface({input:process.stdin,crlfDelay:Infinity});
  let chain=Promise.resolve();
  input.on('line',line=>{
    // Capture has a separate bounded lane. Owner inspection and abort must not
    // queue behind a feed response waiting on the synthetic HTTP barrier.
    let parsed;
    try { if(Buffer.byteLength(line)>16384)throw fail();parsed=manualChromeCommand(parseStrictJSON(line,{maxJSONDepth:4,maxJSONTokens:96})); }
    catch {emit({id:0,ok:false,code:'fixture_manual_rejected'});return;}
    const run=async()=>{
    let message;
    try {
      message=parsed;
      const result=await execute(message);
      emit(message.op==='fixture_bootstrap'?{ready:true}:{id:message.id,ok:true,result});
    }catch{emit({id:Number.isSafeInteger(message?.id)?message.id:0,ok:false,code:'fixture_manual_'+phase});}
    };
    if(parsed.op==='capture_fixture')void run();else chain=chain.then(run);
  });
  input.on('close',()=>{void chain.finally(async()=>{if(handle)await actor.abort(handle).catch(()=>{});if(browser)await browser.close().catch(()=>{});});});
  let exitSent=false;
  setInterval(()=>{if(browser?.isExited()&&owner&&!exitSent){emit({event:'chrome_exit',owner});exitSent=true;}},25).unref();
}

if(process.argv[1]&&pathToFileURL(resolve(process.argv[1])).href===import.meta.url){
  if(process.argv.length!==5)process.exitCode=2;
  else void runManualChromeDriver(...process.argv.slice(2)).catch(()=>{process.exitCode=2;});
}
