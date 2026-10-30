// Private error identity plus closed projections; exception text never leaves.
export const BOOTSTRAP_STAGES=Object.freeze(['none','target_selection','attachment','setup','frame_before','evaluation','frame_after']);
export const BOOTSTRAP_CONDITIONS=Object.freeze(['none','inventory','missing_home','ambiguous_home','ownership','request_failed','context_missing','document_changed','invalid_operation','invalid_projection','exception']);
export const BOOTSTRAP_CLEANUP_STAGES=Object.freeze(['none','detach','attachment']);
export const BOOTSTRAP_CLEANUP_CONDITIONS=Object.freeze(['none','request_failed','attachment_pending']);
const failures=new WeakMap();
export function bootstrapFailure(stage,condition){
 const error=new Error('bootstrap_scope_rejected');
 if(!BOOTSTRAP_STAGES.includes(stage)||!BOOTSTRAP_CONDITIONS.includes(condition))throw error;
 failures.set(error,Object.freeze({stage,condition}));return error;
}
export const bootstrapFailureDetail=error=>failures.get(error)??null;
