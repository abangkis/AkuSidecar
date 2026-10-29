import test from 'node:test';
import assert from 'node:assert/strict';
import {createInstagramTrialControls,instagramTrialView} from '../internal/httpapi/web/instagram-trial-controls.js';
class Element {
  hidden=false;disabled=false;checked=false;textContent='';className='';listeners=new Map();
  addEventListener(type,fn){this.listeners.set(type,fn);} removeEventListener(type){this.listeners.delete(type);}
  emit(type){this.listeners.get(type)?.({preventDefault(){}});}
}
const status=(extra={})=>({selectedMode:'concurrent',authorizationPolicy:'process',maxBatchMs:60000,state:'disabled',reason:'process-consent-required',derivationAuthorized:false,...extra});
const context={epoch:'process-a',development:true};
test('IG trial hides nonprocess authority and describes its unqualified scope',()=>{
  assert.equal(instagramTrialView(status({authorizationPolicy:'profile'}),context,true).visible,false);
  const view=instagramTrialView(status(),context,true);assert.equal(view.startEnabled,true);assert.match(view.detailText,/not yet qualified/);
  assert.equal(instagramTrialView(status({cleanup:'uncertain'}),context,true).startEnabled,false);
});
test('IG controls use independent consent, paths and status without requests on render',async()=>{
  const dom=Object.fromEntries(['panel','status','detail','consent','startButton','stopButton','feedback'].map(key=>[key,new Element()]));
  const calls=[];let currentContext=context;
  const controls=createInstagramTrialControls({...dom,readContext:()=>currentContext,api:async(path,options)=>{
    calls.push({path,options});return{instagramCapture:status({state:'ready',reason:'',derivationAuthorized:true})};
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
