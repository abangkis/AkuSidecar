// Closed private initial-evidence diagnostics. Arbitrary exception messages,
// properties and patterned xnp_* strings never become trusted categories.
export const STARTUP_ROLES=Object.freeze(['reader','collector','pair','unknown']);
export const STARTUP_PHASES=Object.freeze(['prepare','inspect_before','inspect_after','acquire_rpc','identity','rendezvous','settings','cleanup','bound_finish','wire_decode','joint']);
export const STARTUP_REASONS=Object.freeze(['unavailable','rejected','identity_unavailable','source_unavailable','stale_observation','cleanup_unverified','scope_lost','observed_cookie_write','deadline','canceled','replay','sample_missing','sample_duplicate','invalid_shape','scope_mismatch','time_invalid']);
const brands=new WeakMap();
const messages=['manual_startup_unavailable','manual_startup_owner_unavailable','xnp_manual_owner_unavailable'];
export function startupDiagnostic(error){return error!==null&&(typeof error==='object'||typeof error==='function')?brands.get(error)??null:null;}
export function startupFailure(role,phase,reason,peer=false,message=messages[0]){
  if(!STARTUP_ROLES.includes(role)||!STARTUP_PHASES.includes(phase)||!STARTUP_REASONS.includes(reason)){role='unknown';phase='acquire_rpc';reason='unavailable';}
  const error=new Error(messages.includes(message)?message:messages[0]);
  brands.set(error,Object.freeze({role,phase,reason,peer:peer===true}));return error;
}
export function preserveStartup(error,role,phase,reason='unavailable',message=messages[0]){
  const d=startupDiagnostic(error);
  return d?startupFailure(d.role==='unknown'?role:d.role,d.phase,d.reason,d.peer,message):startupFailure(role,phase,reason,false,message);
}
export function startupRPCCode(error){
  const d=startupDiagnostic(error)??{role:'unknown',phase:'acquire_rpc',reason:'unavailable',peer:false};
  return `manual_initial_${d.role}_${d.phase}_${d.reason}_${d.peer?'peer':'primary'}`;
}
export function startupFailureLatch(){
  let first=null;
  return Object.freeze({
    record(error,role,phase,reason='unavailable',message=messages[0]){
      const wrapped=preserveStartup(error,role,phase,reason,message),d=startupDiagnostic(wrapped);
      if(first===null)first=d;
      return startupFailure(first.role,first.phase,first.reason,role!==first.role&&role!=='pair'||d.peer,message);
    },
    current(){return first;},
  });
}

export function requireBoundStartup(observation,role,diagnostics){
  if(observation?.diagnostic==='bound')return;
  const reason=STARTUP_REASONS.includes(observation?.diagnostic)?observation.diagnostic:'unavailable';
  const phase=reason==='identity_unavailable'?'identity':reason==='source_unavailable'?'settings':reason==='cleanup_unverified'?'cleanup':'bound_finish';
  throw diagnostics.record(startupFailure(role,phase,reason),role,phase,reason);
}
