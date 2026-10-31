import {performance} from 'node:perf_hooks';
import {createDiagnosticTrace} from './diagnostic-trace.mjs';
import {IG_DIAGNOSTIC_CATALOG as catalog} from './instagram-diagnostic-catalog.mjs';
const failures=new WeakMap(),traces=new WeakMap();
export const igAllowed=(kind,value)=>catalog[kind]?.includes(value)===true;
const exact=(v,keys)=>v&&Object.getPrototypeOf(v)===Object.prototype&&Reflect.ownKeys(v).length===keys.length&&keys.every(k=>Object.hasOwn(Object.getOwnPropertyDescriptor(v,k)??{},'value'));
export function validateIGTrace(v){
 return exact(v,['schema','stage','completed','disposition','sequence','cause','cleanup','complete','elapsedMS','budgetMS','remainingMS'])&&v.schema==='aku.ig-trace.v1'&&
 igAllowed('stages',v.stage)&&igAllowed('stages',v.completed)&&igAllowed('dispositions',v.disposition)&&Number.isSafeInteger(v.sequence)&&v.sequence>0&&v.sequence<=10000&&
 (v.cause===null||exact(v.cause,['stage','condition'])&&igAllowed('stages',v.cause.stage)&&igAllowed('conditions',v.cause.condition)&&v.cause.condition!=='none')&&
 exact(v.cleanup,['state','condition'])&&igAllowed('cleanup',v.cleanup.state)&&igAllowed('conditions',v.cleanup.condition)&&(v.cleanup.state==='uncertain'||v.cleanup.condition==='none')&&typeof v.complete==='boolean'&&
 (v.budgetMS===null&&v.remainingMS===null||Number.isSafeInteger(v.budgetMS)&&v.budgetMS>=1&&v.budgetMS<=30000&&Number.isSafeInteger(v.remainingMS)&&v.remainingMS>=0&&v.remainingMS<=v.budgetMS)&&
 (v.elapsedMS===null||Number.isSafeInteger(v.elapsedMS)&&v.elapsedMS>=0&&v.elapsedMS<=60000)&&Buffer.byteLength(JSON.stringify(v))<=512;
}
export function createIGTrace(onProgress){const trace=createDiagnosticTrace({schema:'aku.ig-trace.v1',now:()=>performance.now(),onProgress,validate:validateIGTrace,sequenceLimit:10000,elapsedLimitMS:60000});return Object.freeze({...trace,fail(stage,condition){if(!igAllowed('stages',stage)||!igAllowed('conditions',condition)||condition==='none'){trace.incomplete();return;}trace.fail(stage,condition);}});}
export function igFailure(stage,condition){const e=new Error('instagram_diagnostic_rejected');if(!igAllowed('stages',stage)||!igAllowed('conditions',condition)||condition==='none')return e;failures.set(e,{stage,condition});return e;}
export const igFailureDetail=error=>failures.get(error)??null;
export function attachIGTrace(error,trace){if(error&&typeof error==='object')try{const value=trace.snapshot();if(validateIGTrace(value))traces.set(error,value);}catch{}return error;}
export const igErrorTrace=error=>traces.get(error)??null;
export function recordIGError(trace,error,stage){const known=igFailureDetail(error);trace.fail(known?.stage??stage,known?.condition??'unclassified_exception');}
