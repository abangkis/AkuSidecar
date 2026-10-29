import test from 'node:test';
import assert from 'node:assert/strict';
import {createInstagramTrialControls,instagramTrialView} from '../internal/httpapi/web/instagram-trial-controls.js';
class Element {
  hidden=false;disabled=false;checked=false;textContent='';className='';listeners=new Map();
  addEventListener(type,fn){this.listeners.set(type,fn);} removeEventListener(type){this.listeners.delete(type);}
  emit(type){this.listeners.get(type)?.({preventDefault(){}});}
}
const status=(extra={})=>({selectedMode:'concurrent',authorizationPolicy:'profile',rememberedPermission:false,maxBatchMs:60000,state:'disabled',reason:'process-consent-required',derivationAuthorized:false,...extra});
const context={epoch:'process-a',development:true};
test('IG trial requires profile authority and describes its unqualified scope',()=>{
  assert.equal(instagramTrialView(status({authorizationPolicy:'process'}),context,true).visible,false);
  const view=instagramTrialView(status(),context,true);assert.equal(view.startEnabled,true);assert.match(view.detailText,/not yet qualified/);
  assert.equal(instagramTrialView(status({cleanup:'uncertain'}),context,true).startEnabled,false);
});

test('IG transient cooldown preserves Stop and permission without requesting restart',()=>{
  const view=instagramTrialView(status({state:'retry-wait',reason:'transient-batch-failure',cleanup:'verified',rememberedPermission:true,derivationAuthorized:true}),context,true);
  assert.equal(view.visible,true);assert.equal(view.startEnabled,false);assert.equal(view.stopEnabled,true);
  assert.equal(view.statusText,'Waiting to retry');assert.match(view.detailText,/fresh account\/session/);assert.doesNotMatch(view.detailText,/restart/);
});

test('IG maintenance release remains explicit and retains experimental acknowledgement',async()=>{
  const dom=Object.fromEntries(['panel','status','detail','consent','startButton','stopButton','feedback'].map(key=>[key,new Element()]));
  const calls=[];
  const controls=createInstagramTrialControls({...dom,readContext:()=>context,api:async(path,options)=>{calls.push({path,options});return{instagramCapture:status({state:'ready',reason:'',rememberedPermission:true,derivationAuthorized:true})};}});
  controls.reset(status({state:'disabled',reason:'maintenance-confirmation-required',maintenanceHeld:true,rememberedPermission:true}));
  assert.equal(calls.length,0);assert.equal(dom.startButton.textContent,'Resume IG');assert.equal(dom.startButton.disabled,false);
  assert.match(dom.detail.textContent,/experimental/);
  dom.startButton.emit('click');await new Promise(resolve=>setImmediate(resolve));
  assert.equal(calls[0].options.body.acknowledgeUnqualifiedRecipe,true);assert.equal(calls[0].options.instanceEpoch,'process-a');
  controls.destroy();
});
test('IG controls use independent consent, paths and status without requests on render',async()=>{
  const dom=Object.fromEntries(['panel','status','detail','consent','startButton','stopButton','feedback'].map(key=>[key,new Element()]));
  const calls=[];let currentContext=context;
  const controls=createInstagramTrialControls({...dom,readContext:()=>currentContext,api:async(path,options)=>{
    calls.push({path,options});return{instagramCapture:status({state:'ready',reason:'',rememberedPermission:true,derivationAuthorized:true})};
  }});
  controls.reset(status());assert.equal(calls.length,0);assert.equal(dom.startButton.disabled,true);
  dom.consent.checked=true;dom.consent.emit('change');assert.equal(dom.startButton.disabled,false);
  dom.startButton.emit('click');await new Promise(resolve=>setImmediate(resolve));
  assert.equal(calls[0].path,'/api/development/instagram-trial/start');
  assert.deepEqual(calls[0].options.body,{acknowledgeLimitations:true,allowProcessSessionDerivation:true,acknowledgeUnqualifiedRecipe:true});
  assert.equal(calls[0].options.instanceEpoch,'process-a');assert.equal(dom.stopButton.disabled,false);
  currentContext={...context,epoch:'process-b'};controls.beginRead();assert.equal(dom.consent.checked,false);assert.equal(dom.startButton.disabled,true);
  controls.destroy();
});

test('IG saved choice is restored only from fresh server status after an epoch change',()=>{
  const dom=Object.fromEntries(['panel','status','detail','consent','startButton','stopButton','feedback'].map(key=>[key,new Element()]));
  const calls=[];let currentContext=context;
  const controls=createInstagramTrialControls({...dom,readContext:()=>currentContext,api:(...args)=>{calls.push(args);}});
  const saved=status({state:'ready',reason:'',rememberedPermission:true,derivationAuthorized:true});
  controls.reset(saved);assert.equal(dom.consent.checked,true);
  const stale=controls.beginRead();currentContext={...context,epoch:'process-b'};
  const fresh=controls.beginRead();assert.equal(dom.consent.checked,false);
  assert.equal(controls.applyRead(stale,{instagramCapture:saved}),false);
  assert.equal(controls.applyRead(fresh,{instagramCapture:status({state:'stopped',reason:'batch-failed',rememberedPermission:true})}),true);
  assert.equal(dom.consent.checked,true);assert.equal(dom.startButton.disabled,true);assert.equal(dom.stopButton.disabled,false);
  assert.match(dom.detail.textContent,/saved/);assert.match(dom.detail.textContent,/verification/);
  assert.equal(calls.length,0);controls.destroy();
});
