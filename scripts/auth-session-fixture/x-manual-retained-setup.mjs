// One-shot private QA setup. Diagnostics contain only closed categories; the
// success DTO remains private enrollment metadata, never an evidence receipt.
import {performance} from 'node:perf_hooks';
import {MANUAL_TIMING} from './x-manual-timing-policy.mjs';
import {startupDiagnostic,startupRPCCode} from './x-manual-startup-diagnostic.mjs';
export const MANUAL_SEED_PHASES=Object.freeze(['preconditions','owner_before','frame_before','enrollment_evaluate','enrollment_project','enrollment_cleanup','frame_after','cookie_read','cookie_select','seed_write','owner_after']);
export const MANUAL_SEED_REASONS=Object.freeze(['disabled','replay','unavailable','rejected','candidate_absent','bootstrap_missing','bootstrap_invalid','wrong_page','inspection_failed','document_changed','ownership_changed','deadline','clock_regressed']);
export const MANUAL_SEED_WRITES=Object.freeze(['not_attempted','unknown','acknowledged']);
const diagnostics=new WeakMap();
const initialFailures=new WeakMap();
export function manualSeedFailure(phase,reason,write='not_attempted',cause){
  if(!MANUAL_SEED_PHASES.includes(phase)||!MANUAL_SEED_REASONS.includes(reason)||!MANUAL_SEED_WRITES.includes(write)){
    phase='preconditions';reason='unavailable';write='unknown';
  }
  const value=Object.freeze({phase,reason,write});
  const error=new Error('xnp_manual_setup_unavailable');
  diagnostics.set(error,value);
  const initial=startupDiagnostic(cause)?cause:cause!==null&&(typeof cause==='object'||typeof cause==='function')?initialFailures.get(cause):undefined;
  if(initial)initialFailures.set(error,initial);
  return error;
}
export function manualSeedDiagnostic(error){return error!==null&&(typeof error==='object'||typeof error==='function')?diagnostics.get(error)??null:null;}
export function manualSeedDiagnosticCode(error){
  const value=manualSeedDiagnostic(error);
  const initial=value?initialFailures.get(error):null;
  if(initial&&startupDiagnostic(initial)?.trace){
    const tail=startupRPCCode(initial).slice('manual_initial_'.length);
    return `manual_seed_v2_${MANUAL_SEED_PHASES.indexOf(value.phase)}_${MANUAL_SEED_REASONS.indexOf(value.reason)}_${MANUAL_SEED_WRITES.indexOf(value.write)}_${tail}`;
  }
  return value?`manual_seed_${value.phase}_${value.reason}_${value.write}`:null;
}
const keys=['readerPID','readerProfile','collectorPID','collectorProfile'];
function checkedOwnership(value){
  if(!value||Object.keys(value).sort().join(',')!==[...keys].sort().join(',')||
    !['readerPID','collectorPID'].every(k=>Number.isSafeInteger(value[k])&&value[k]>0)||value.readerPID===value.collectorPID||
    !['readerProfile','collectorProfile'].every(k=>typeof value[k]==='string'&&value[k].length>0&&Buffer.byteLength(value[k])<=512)||value.readerProfile===value.collectorProfile)throw manualSeedFailure('owner_before','rejected');
  return Object.freeze({...value});
}
export function createRetainedManualSetup({enabled=false,transferAllowed=false,ownership,readEnrollment,readAllowedCookies,beforeWrite=async()=>{},assertBeforeWrite=()=>{},writeAllowedCookies,cleanupEnrollment=async()=>{},now=()=>performance.now()}={}){
  if(enabled!==true||transferAllowed!==true||![ownership,readEnrollment,readAllowedCookies,beforeWrite,assertBeforeWrite,writeAllowedCookies,cleanupEnrollment,now].every(v=>typeof v==='function'))throw manualSeedFailure('preconditions','disabled');
  let used=false,write='not_attempted';
  return async()=>{
    if(used)throw manualSeedFailure('preconditions','replay',write);used=true;
    let phase='owner_before',start,last,selected=null,successful=false,writePending=false,firstFailure;
    const tick=()=>{const t=now();if(!Number.isFinite(start)||!Number.isFinite(t)||t<last)throw manualSeedFailure(phase,'clock_regressed',write);if(t-start>=MANUAL_TIMING.attemptAuthorizationMaxMS)throw manualSeedFailure(phase,'deadline',write);last=t;};
    try{
      start=last=now();tick();const actual=checkedOwnership(await ownership());tick();
      phase='enrollment_evaluate';const accountId=await readEnrollment();tick();
      phase='enrollment_project';if(typeof accountId!=='string'||!/^[1-9][0-9]{0,19}$/.test(accountId))throw manualSeedFailure(phase,'candidate_absent',write);
      phase='cookie_read';selected=await readAllowedCookies();tick();
      phase='cookie_select';if(!Array.isArray(selected)||selected.length!==2)throw manualSeedFailure(phase,'rejected',write);
      phase='seed_write';await beforeWrite();tick();
      // This synchronous assertion and callback dispatch share one JS turn.
      // A rejected assertion therefore proves the write was not attempted.
      if(assertBeforeWrite()!==undefined)throw manualSeedFailure(phase,'rejected',write);
      write='unknown';writePending=true;
      const acknowledgeWrite=()=>{
        if(!writePending||write!=='unknown')throw manualSeedFailure('seed_write','replay',write);
        write='acknowledged';
      };
      try{if(await writeAllowedCookies(selected,acknowledgeWrite)!==true)throw manualSeedFailure(phase,'unavailable',write);}
      finally{writePending=false;}
      write='acknowledged';tick();
      phase='owner_after';const after=checkedOwnership(await ownership());tick();
      if(keys.some(k=>actual[k]!==after[k]))throw manualSeedFailure(phase,'ownership_changed',write);
      successful=true;return Object.freeze({accountId,...actual,seeded:true,homeReady:true});
    }catch(error){
      // Only our WeakMap brand may supply a phase/reason. Arbitrary exception
      // messages, getters, properties, causes and cookie values are discarded.
      const safe=manualSeedDiagnostic(error);
      const known=safe?.phase==='enrollment_cleanup'||phase==='enrollment_evaluate'&&safe&&['frame_before','enrollment_evaluate','enrollment_project','frame_after'].includes(safe.phase)||phase==='cookie_read'&&safe&&['cookie_select','frame_after'].includes(safe.phase)||phase==='seed_write'&&safe?.phase==='frame_after';
      firstFailure=manualSeedFailure(known?safe.phase:phase,safe?.reason??(startupDiagnostic(error)?'rejected':'unavailable'),write,error);
      throw firstFailure;
    }finally{
      selected=null;
      try{await cleanupEnrollment();}catch(error){throw manualSeedFailure('enrollment_cleanup','unavailable',write,firstFailure??error);}
      if(successful){phase='enrollment_cleanup';tick();}
    }
  };
}
