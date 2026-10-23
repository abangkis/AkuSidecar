import {startupFailure} from './x-manual-startup-diagnostic.mjs';
import {ROOT_SOURCE_POLICY} from './x-manual-root-source-policy.mjs';
const sessions=new WeakMap();
const fail=()=>new Error('manual_root_control_failed');
// Called only at fresh process/target attachment, never when borrowing a tab or
// attaching the lifecycle observer. Registration never resets a used capability.
export function registerFreshStartupSession(owner,session,role){
 if(!['reader','collector'].includes(role)||!owner||typeof session!=='string'||!session)throw fail();
 const existing=sessions.get(owner);if(existing){if(existing.session!==session||existing.role!==role)throw fail();return;}
 sessions.set(owner,{session,role,target:owner.targetId,ownerId:owner.owner,used:false,expected:null,unsettled:false});
}
export function allowsStartupControl(owner,method,params,session){
 const c=sessions.get(owner),e=c?.expected;
 return !!e&&session===c.session&&method===e.method&&JSON.stringify(params)===JSON.stringify(e.params);
}
// Pre-enrollment cache disabling belongs to the same owned restoration obligation.
export function markStartupControlAttempt(owner){const c=sessions.get(owner);if(!c||owner.sessionId!==c.session||owner.targetId!==c.target||owner.owner!==c.ownerId)throw fail();c.unsettled=true;}
export function mustRetireStartupOwner(owner){return sessions.get(owner)?.unsettled===true;}
export function startupRootControls(owner,send,role){
 const c=sessions.get(owner);if(!['reader','collector'].includes(role)||!c||c.role!==role||c.used||typeof send!=='function')throw fail();c.used=true;
 let applied=false,restored=false,attemptedRestore=false;
 const command=async(method,params)=>{c.expected={method,params};try{if(owner.sessionId!==c.session||owner.targetId!==c.target||owner.owner!==c.ownerId)throw fail();const r=await send(method,params,c.session,'manual_root_control');if(r?.acknowledged!==true)throw fail();}catch{throw startupFailure(role,attemptedRestore?'cleanup':'prepare',attemptedRestore?'cleanup_unverified':'source_unavailable',false,undefined,{firstCause:'root_control',guardCondition:attemptedRestore?'root_control_restore':'root_control_apply'});}finally{c.expected=null;}};
 return Object.freeze({
  async apply(){if(applied)throw fail();applied=true;c.unsettled=true;await command('Network.setCacheDisabled',{cacheDisabled:ROOT_SOURCE_POLICY.controls.cacheDisabled});await command('Network.setBypassServiceWorker',{bypass:ROOT_SOURCE_POLICY.controls.bypassServiceWorker});},
  async restore(){if(!applied||attemptedRestore)throw fail();attemptedRestore=true;await command('Network.setBypassServiceWorker',{bypass:false});await command('Network.setCacheDisabled',{cacheDisabled:false});restored=true;c.unsettled=false;},
  readyToRelease(){return restored;},
 });
}
