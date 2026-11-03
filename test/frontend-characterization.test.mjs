import test from "node:test";
import assert from "node:assert/strict";
import vm from "node:vm";
import { readFrontendAppSource, frontendFunction } from "./fixtures/frontend-source.mjs";
import { buildTimelineReadLaterPath, timelineReadLaterConfirmation } from "../internal/httpapi/web/timeline-memory-state.js";
import { applyReasoningRuntimeResponse } from "../internal/httpapi/web/reasoning-runtime-state.js";
const app = readFrontendAppSource();
function fn(name, next) { return frontendFunction(app, name, next); }
class Element {
  constructor() { this.children=[];this.attributes=new Map();this.listeners=new Map();this.disabled=false;this.textContent="";this.value="";this.checked=false;const c=new Set();this.classList={toggle:(k,v)=>v?c.add(k):c.delete(k),contains:k=>c.has(k)}; }
  append(...values){this.children.push(...values);} setAttribute(k,v){this.attributes.set(k,String(v));} getAttribute(k){return this.attributes.get(k);}
  addEventListener(k,v){this.listeners.set(k,v);} click(){return this.listeners.get("click")?.();}
}
function deferred(){let resolve,reject;const promise=new Promise((a,b)=>{resolve=a;reject=b;});return{promise,resolve,reject};}
test("Read later preserves admission, confirmation, response projection and local status",async t=>{
  for(const mode of ['save','already-saved','reject-confirm','in-flight','saved','no-id','bad-response','error','no-text'])await t.test(mode,async()=>{
    const calls=[],events=[],errors=[],timers=[];const pending=deferred();const entry={id:mode==='no-id'?'':'timeline/one',evidence:{text:mode==='no-text'?'':'Text'},personalMemory:{saved:mode==='saved'}};
    const state={timelineReadLaterInFlight:new Set(mode==='in-flight'?['timeline/one']:[])};
    const context=vm.createContext({state,document:{createElement:()=>new Element()},window:{confirm:()=>{events.push('confirm');return mode!=='reject-confirm';},clearTimeout:()=>events.push('clear'),setTimeout:(callback,delay)=>{timers.push({callback,delay});return 1;}},buildTimelineReadLaterPath,timelineReadLaterConfirmation,api:async(path,options)=>{calls.push({path,options});return pending.promise;},invalidateLibraryStorage:()=>events.push('invalidate'),showError:e=>errors.push(e)});
    vm.runInContext(fn('buildTimelineReadLaterAction','function buildTimelineContentContextAnchor('),context);
    const root=context.buildTimelineReadLaterAction(entry),button=root.children[0],status=root.children[1];
    assert.equal(button.disabled,['saved','in-flight'].includes(mode));assert.equal(status.getAttribute('aria-live'),'polite');
    if(mode==='no-text')assert.match(button.title,/source reference/);
    const click=button.click();
    if(['reject-confirm','in-flight','saved','no-id'].includes(mode)){await click;assert.equal(calls.length,0);return;}
    assert.equal(calls[0].path,'/api/timeline/timeline%2Fone/read-later');assert.equal(calls[0].options.method,'POST');assert.equal(button.textContent,'Adding…');assert.equal(button.disabled,true);
    if(mode==='error')pending.reject(new Error('offline'));else pending.resolve(mode==='bad-response'?{}:{saved:true,alreadySaved:mode==='already-saved',retentionTier:'reference',permanentKeep:false});
    await click;assert.equal(state.timelineReadLaterInFlight.size,0);
    if(mode==='error'||mode==='bad-response'){assert.equal(button.disabled,false);assert.equal(button.textContent,'Read later');assert.equal(errors.length,1);assert.equal(events.includes('invalidate'),false);assert.match(status.textContent,/Could not add/);}
    else {assert.equal(entry.personalMemory.saved,true);assert.equal(button.textContent,'Saved');assert.equal(button.getAttribute('aria-pressed'),'true');assert.equal(events.includes('invalidate'),true);assert.equal(timers[0].delay,3500);assert.match(status.textContent,mode==='already-saved'?/Already Saved/:/Saved for later/);timers[0].callback();assert.equal(status.textContent,'');}
  });
});
test("Read later shares admission across a rebuilt card and keeps the completed node local",async()=>{
  const pending=deferred(),state={timelineReadLaterInFlight:new Set()},entry={id:'one'},calls=[];
  const context=vm.createContext({state,document:{createElement:()=>new Element()},window:{confirm:()=>true,clearTimeout(){},setTimeout(){return 1;}},buildTimelineReadLaterPath,timelineReadLaterConfirmation,api:async()=>{calls.push(true);return pending.promise;},invalidateLibraryStorage(){},showError:e=>assert.fail(e)});
  vm.runInContext(fn('buildTimelineReadLaterAction','function buildTimelineContentContextAnchor('),context);
  const old=context.buildTimelineReadLaterAction(entry),click=old.children[0].click();const replacement=context.buildTimelineReadLaterAction({id:'one'});await replacement.children[0].click();assert.equal(calls.length,1);pending.resolve({saved:true});await click;assert.equal(old.children[0].textContent,'Saved');assert.equal(replacement.children[0].textContent,'Adding…');
});
test("Settings admission retains empty-source rejection and explicit AI Hide confirmation",async t=>{
  for(const mode of ['empty','hide','ordinary'])await t.test(mode,async()=>{
    const calls=[],elements=new Map();const $=id=>{if(!elements.has(id))elements.set(id,new Element());return elements.get(id);};
    const state={bootstrap:{settings:{aiDetectionPresentation:'drawer',reasoningProvider:'old'}}};const draft={activeSources:mode==='empty'?[]:['x'],aiDetectionEnabled:true,aiDetectionPresentation:mode==='hide'?'hide':'drawer'};
    const context=vm.createContext({state,$,readSettingsDraft:()=>draft,openResetDialog:k=>calls.push(k),persistSettings:async v=>calls.push(v)});vm.runInContext(fn('saveSettings','function reasoningProfileValue('),context);await context.saveSettings({preventDefault(){}});
    if(mode==='empty'){assert.equal(calls.length,0);assert.match($('#runtime-settings-status').textContent,/at least one/);}else if(mode==='hide'){assert.deepEqual(calls,['ai-hide']);assert.equal(state.pendingSettings.reasoningProvider,'old');}else assert.equal(calls[0].reasoningProvider,'old');
  });
});
test("Settings persistence retains API/runtime/render/refresh ordering and failure output",async t=>{
  for(const failure of [false,true])await t.test(String(failure),async()=>{
    const events=[],elements=new Map(),$=id=>{if(!elements.has(id))elements.set(id,new Element());return elements.get(id);};const current={reasoningProvider:'old'},saved={reasoningProvider:'new',maxScrolls:2,maxItemsPerSource:5};const state={bootstrap:{settings:current}};
    const context=vm.createContext({state,$,api:async(path,options)=>{events.push('request');assert.equal(path,'/api/settings');assert.equal(options.body.confirmationPhrase,'EXACT');if(failure)throw Error('offline');return{settings:saved,provider:'new'};},applyReasoningRuntimeResponse:(...args)=>{events.push('runtime');applyReasoningRuntimeResponse(...args);},renderActiveReasoningProvider:()=>events.push('provider'),renderSettings:()=>events.push('settings'),syncOnboardingLearning:()=>events.push('learning'),shouldShowOnboardingLearning:()=>false,refreshTimeline:async()=>events.push('timeline'),settingsDirty:{isDirty:()=>true},renderSettingsDirtyState:()=>events.push('dirty'),showError:()=>events.push('error')});
    vm.runInContext(fn('persistSettings','function renderReasoningProviderControl('),context);const result=await context.persistSettings(saved,'EXACT');assert.deepEqual(events,failure?['request','error','dirty']:['request','runtime','provider','settings','learning','timeline','dirty']);assert.equal(result,failure?null:saved);
  });
});
test("Provider activation preserves unrelated dirty fields and rejects nonselectable providers",async t=>{
  for(const allowed of [true,false])await t.test(String(allowed),async()=>{
    const elements=new Map(),$=id=>{if(!elements.has(id))elements.set(id,new Element());return elements.get(id);};$('#onboarding-provider-dialog').close=()=>{};const saved={reasoningProvider:'new'},baseline={reasoningProvider:'old',timelineCapacity:24};let next,refreshes=0,requests=0;
    const context=vm.createContext({$ ,state:{bootstrap:{settings:{reasoningProvider:'old',timelineCapacity:36}}},onboardingProviderEntries:()=>[{name:'new',label:'New'}],providerCanActivate:()=>allowed,api:async()=>{requests++;return{settings:saved};},applyReasoningRuntimeResponse(){},renderActiveReasoningProvider(){},renderReasoningProviderControl(){},renderReasoningProcesses(){},settingsDirty:{getBaselineKey:()=>JSON.stringify(baseline),setBaseline:v=>{next=v;},refresh:()=>refreshes++}});vm.runInContext(fn('activateSettingsReasoningProvider','function skipOnboardingProvider('),context);await context.activateSettingsReasoningProvider('new');assert.equal(requests,allowed?1:0);if(allowed){assert.equal(next.timelineCapacity,24);assert.equal(next.reasoningProvider,'new');assert.equal(refreshes,1);}
  });
});
test("Session cancellation retains cleanup retry polling and successful cleanup order",async t=>{
  for(const fail of [true,false])await t.test(String(fail),async()=>{
    const events=[],state={session:{id:'session/one'},dispatchKey:'key',dispatchRetryAfter:new Map([['one',1]])};const context=vm.createContext({state,console:{warn(){}},api:async(path,options)=>{events.push('cancel');assert.equal(path,'/api/sessions/session%2Fone/cancel');assert.equal(options.method,'POST');return{session:{id:'session/one',status:'cancelled'}};},renderSession:()=>events.push('render'),stopPolling:()=>events.push('stop'),startPolling:()=>events.push('start'),releaseCaptureSurface:async()=>{events.push('release');if(fail)throw Error('cleanup');},showError:()=>assert.fail('unexpected error')});vm.runInContext(fn('cancelSession','function startPolling('),context);await context.cancelSession();assert.deepEqual(events,fail?['cancel','render','stop','release','render','start']:['cancel','render','stop','release','render']);assert.equal(state.session===null,!fail);assert.equal(state.captureCleanupPending,fail?'session/one':undefined);
  });
});
test("Session polling rejects stale session responses and always releases in-flight state",async()=>{
  const wait=deferred(),state={session:{id:'old'},pollInFlight:false};const context=vm.createContext({state,api:()=>wait.promise,releaseCompletedSourceSurfaces:()=>assert.fail('stale response reached cleanup'),showError:()=>assert.fail('unexpected')});vm.runInContext(fn('pollSession','async function releaseCaptureSurface('),context);const poll=context.pollSession();assert.equal(state.pollInFlight,true);state.session={id:'new'};wait.resolve({session:{id:'old',status:'completed'}});await poll;assert.equal(state.session.id,'new');assert.equal(state.pollInFlight,false);
});
test("Bootstrap retains authoritative validation, stale-attempt fencing and existing polling cadence",async t=>{
  for(const mode of ['success','invalid','stale'])await t.test(mode,async()=>{
    const requests=[],events=[],intervals=[],timers=[],elements=new Map(),$=id=>{if(!elements.has(id))elements.set(id,new Element());return elements.get(id);};
    const state={bootstrapAttempt:0,bootstrapController:null,releasedCaptureSources:new Set(),currentView:'timeline'};
    const context={$ ,state,defaultIntent:'intent',BOOTSTRAP_TIMEOUT_MS:45000,BOOTSTRAP_RETRY_MS:3000,AbortController,CustomEvent:class{constructor(type,value){this.type=type;this.detail=value.detail;}},window:{dispatchEvent:e=>events.push(e.detail)},setTimeout:(callback,delay)=>{timers.push({callback,delay});return timers.length;},clearTimeout(){},setInterval:(callback,delay)=>{intervals.push(delay);return intervals.length;},databaseHealthPoller:null,
      api:()=>{const d=deferred();requests.push(d);return d.promise;},xDailyControls:{reset(){}},instagramTrialControls:{reset(){}},installUIActivityTracking(){},recordUIActivity(){},renderSourceControls(){},renderLibrarySourceOptions(){},renderLibrary(){},renderActiveReasoningProvider(){},renderDatabaseHealth(){},renderDeployment(){},setPill(){},renderBridge(){},renderSettings(){},renderTimeline:()=>events.push('timeline'),renderSession(){},onboardingRequiresSetup:()=>false,setView(){},showOnboarding(){},showCalibration(){},pingBridge(){},bridgeActionLoop(){},pollCollectionRuntime(){},pollAutoUpdate(){},pollDatabaseHealth(){},refreshLivingTopicNotificationProjection(){},startPolling(){},clearNotice(){},syncRunButtons(){},showError:()=>events.push('error')};
    vm.createContext(context);vm.runInContext(fn('bootstrap','function renderDeployment('),context);
    const payload={onboarding:{status:'completed'},settings:{},timeline:[],activeSession:null};const first=context.bootstrap();
    if(mode==='stale'){const oldController=state.bootstrapController;const second=context.bootstrap();assert.equal(oldController.signal.aborted,true);requests[1].resolve(payload);await second;requests[0].resolve({...payload,version:'stale'});await first;assert.equal(state.bootstrap,payload);assert.deepEqual(intervals,[30000,2000,15000,60000]);}
    else {requests[0].resolve(mode==='invalid'?{onboarding:{status:'unknown'}}:payload);await first;if(mode==='invalid'){assert.equal(state.bootstrap,undefined);assert.equal(events.includes('ready'),false);assert.equal(timers.at(-1).delay,3000);}else{assert.deepEqual(events,['restoring','rendering','timeline','ready']);assert.deepEqual(intervals,[30000,2000,15000,60000]);}}
    assert.equal(timers[0].delay,45000);
  });
});
