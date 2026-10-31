// Value recorder only. Callers own catalogs, clocks, resource and retry policy.
export function createDiagnosticTrace({schema,now,onProgress=()=>{},validate,sequenceLimit=Number.MAX_SAFE_INTEGER,elapsedLimitMS=Number.MAX_SAFE_INTEGER}) {
 if(!Number.isSafeInteger(sequenceLimit)||sequenceLimit<3||!Number.isSafeInteger(elapsedLimitMS)||elapsedLimitMS<0)throw new Error('diagnostic_policy_invalid');
 const start=now();let sequence=0,saturationReported=false,finalProgressReported=false;
 const state={schema,stage:'none',completed:'none',disposition:'not_started',sequence:0,cause:null,cleanup:{state:'not_started',condition:'none'},complete:true,elapsedMS:0,budgetMS:null,remainingMS:null};
 const snapshot=()=>{
  let elapsedMS;
  try{const elapsed=Math.ceil(now()-start);if(Number.isSafeInteger(elapsed)&&elapsed>=0&&elapsed<=elapsedLimitMS)elapsedMS=elapsed;}catch{}
  if(elapsedMS===undefined){elapsedMS=null;state.complete=false;}
  if(sequence>=sequenceLimit-2)state.complete=false;
  sequence=Math.min(sequence+1,sequenceLimit);
  const budgetMS=elapsedMS===null?null:state.budgetMS;
  const value={...state,sequence,elapsedMS,budgetMS,remainingMS:budgetMS===null?null:Math.max(0,budgetMS-elapsedMS)};
  if(!validate(value))throw new Error('diagnostic_projection_invalid');return structuredClone(value);
 };
 // Send one incomplete progress marker, then reserve the last sequence value
 // for terminal cleanup and first-cause observations.
 const emit=()=>{
  if(sequence>=sequenceLimit-2){
   state.complete=false;
   if(!saturationReported&&sequence<sequenceLimit-1){saturationReported=true;try{onProgress(snapshot());}catch{}}
   return;
  }
  try{onProgress(snapshot());}catch{state.complete=false;}
 };
 return Object.freeze({
  currentStage:()=>state.stage,
  budget(ms){state.budgetMS=ms;},note(stage){state.stage=stage;emit();},done(stage){state.completed=stage;emit();},
  disposition(value){state.disposition=value;emit();},
  fail(stage,condition){state.cause??={stage,condition};emit();},
  cleanup(value,condition='none'){if(value==='uncertain'&&state.cleanup.state==='uncertain'&&state.cleanup.condition!=='none')condition=state.cleanup.condition;state.cleanup={state:value,condition};emit();},
  flush(){if(!saturationReported||finalProgressReported)return;finalProgressReported=true;try{onProgress(snapshot());}catch{state.complete=false;}},
  incomplete(){state.complete=false;emit();},snapshot,
 });
}
