// Value recorder only. Callers own catalogs, clocks, resource and retry policy.
export function createDiagnosticTrace({schema,now,onProgress=()=>{},validate}) {
 const start=now();let sequence=0;
 const state={schema,stage:'none',completed:'none',disposition:'not_started',sequence:0,cause:null,cleanup:{state:'not_started',condition:'none'},complete:true,elapsedMS:0,budgetMS:null,remainingMS:null};
 const snapshot=()=>{const elapsedMS=Math.ceil(now()-start);const value={...state,sequence:++sequence,elapsedMS,remainingMS:state.budgetMS===null?null:Math.max(0,state.budgetMS-elapsedMS)};if(!validate(value))throw new Error('diagnostic_projection_invalid');return structuredClone(value);};
 const emit=()=>{try{onProgress(snapshot());}catch{state.complete=false;}};
 return Object.freeze({
  budget(ms){state.budgetMS=ms;},note(stage){state.stage=stage;emit();},done(stage){state.completed=stage;emit();},
  disposition(value){state.disposition=value;emit();},
  fail(stage,condition){state.cause??={stage,condition};emit();},
  cleanup(value,condition='none'){if(value==='uncertain'&&state.cleanup.state==='uncertain'&&state.cleanup.condition!=='none')condition=state.cleanup.condition;state.cleanup={state:value,condition};emit();},
  incomplete(){state.complete=false;emit();},snapshot,
 });
}
