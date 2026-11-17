// Value recorder only. Callers own catalogs, clocks, resource and retry policy.
import '../vendor/diagnostic-state-core.js';
export function createDiagnosticTrace({schema,now,onProgress=()=>{},validate,sequenceLimit=Number.MAX_SAFE_INTEGER,elapsedLimitMS=Number.MAX_SAFE_INTEGER}) {
 if(!Number.isSafeInteger(sequenceLimit)||sequenceLimit<3||!Number.isSafeInteger(elapsedLimitMS)||elapsedLimitMS<0)throw new Error('diagnostic_policy_invalid');
 const start=now();let sequence=0,saturationReported=false,finalProgressReported=false;
 const state=globalThis.AkuDiagnosticState.create();let budget=null;
 const snapshot=()=>{
  let elapsedMS;
  try{const elapsed=Math.ceil(now()-start);if(Number.isSafeInteger(elapsed)&&elapsed>=0&&elapsed<=elapsedLimitMS)elapsedMS=elapsed;}catch{}
  if(elapsedMS===undefined){elapsedMS=null;state.incomplete();}
  if(sequence>=sequenceLimit-2)state.incomplete();
  sequence=Math.min(sequence+1,sequenceLimit);
  const valueState=state.snapshot(),budgetMS=elapsedMS===null?null:budget;
  const value={schema,stage:valueState.phase,completed:valueState.completed,disposition:valueState.disposition,sequence,
    cause:valueState.cause?{stage:valueState.cause.phase,condition:valueState.cause.condition}:null,
    cleanup:valueState.cleanup,complete:valueState.complete,elapsedMS,budgetMS,remainingMS:budgetMS===null?null:Math.max(0,budgetMS-elapsedMS)};
  if(!validate(value))throw new Error('diagnostic_projection_invalid');return structuredClone(value);
 };
 // Send one incomplete progress marker, then reserve the last sequence value
 // for terminal cleanup and first-cause observations.
 const emit=()=>{
  if(sequence>=sequenceLimit-2){
   state.incomplete();
   if(!saturationReported&&sequence<sequenceLimit-1){saturationReported=true;try{onProgress(snapshot());}catch{}}
   return;
  }
  try{onProgress(snapshot());}catch{state.incomplete();}
 };
 return Object.freeze({
  currentStage:()=>state.currentPhase(),
  budget(ms){budget=ms;},note(stage){state.advance(stage);emit();},done(stage){state.completed(stage);emit();},
  disposition(value){state.disposition(value);emit();},
  fail(stage,condition){state.fail(stage,condition);emit();},
  cleanup(value,condition='none'){const previous=state.snapshot().cleanup;if(value==='uncertain'&&previous.state==='uncertain'&&previous.condition!=='none')condition=previous.condition;state.cleanup(value,condition);emit();},
  flush(){if(!saturationReported||finalProgressReported)return;finalProgressReported=true;try{onProgress(snapshot());}catch{state.incomplete();}},
  incomplete(){state.incomplete();emit();},snapshot,
 });
}
