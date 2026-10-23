// Closed private initial-evidence diagnostics. Arbitrary exception messages,
// properties and patterned xnp_* strings never become trusted categories.
export const STARTUP_ROLES=Object.freeze(['reader','collector','pair','unknown']);
export const STARTUP_PHASES=Object.freeze(['prepare','inspect_before','inspect_after','acquire_rpc','identity','rendezvous','settings','cleanup','bound_finish','wire_decode','joint']);
export const STARTUP_REASONS=Object.freeze(['unavailable','rejected','identity_unavailable','source_unavailable','stale_observation','cleanup_unverified','scope_lost','observed_cookie_write','deadline','canceled','replay','sample_missing','sample_duplicate','invalid_shape','scope_mismatch','time_invalid']);
export const STARTUP_GUARD_CAUSES=Object.freeze(['guard_request_unbound','guard_request_budget','guard_header_classification','guard_response_correlation','guard_metadata_shape','guard_completion_missing','guard_header_budget','guard_event_budget']);
// Append-only V2 catalog prefix plus V3/V4/V5/V6/V7 guard fields form the private wire contract.
export const STARTUP_TRACE=Object.freeze({
  firstCause:Object.freeze(['unknown','cookie_candidate','cookie_ambiguous','context_lost','target_lost','navigation','owner_scope','event_cap','identity_order','identity_mismatch','settings_order','response_frame','extra_shape','extra_status','extra_missing','extra_unexpected','generic_early','stale_observation','cleanup_failed',...STARTUP_GUARD_CAUSES]),
  terminal:Object.freeze(['unknown',...STARTUP_REASONS,'bound']),
  early:Object.freeze(['unknown','captured','deadline','script_limit','wrong_page','unexpected_pause','evaluation_failed','setup_failed']),
  handoff:Object.freeze(['unknown','accepted','rejected']),
  cookie:Object.freeze(['unknown','not_observed','candidate_observed','ambiguous','other_only',...STARTUP_GUARD_CAUSES]),
  extra:Object.freeze(['unknown','not_selected','not_required','required_pending','complete','missing','status_mismatch','unexpected','invalid']),
  guardTrigger:Object.freeze(['unknown','baseline','arm','metadata','request','document','response','extra','redirect','finish']),
  guardProvenance:Object.freeze(['unknown','absent','scoped','origin_scope','frame_scope','loader_missing','redirect','reused','root_scope']),
  guardCondition:Object.freeze(['unknown','predecessor_missing','redirect_shape','previous_response','chain_completed','chain_failed','chain_cache','previous_resource','current_resource','previous_origin','previous_frame','previous_loader','current_origin','current_frame','current_loader','loader_mismatch','redirect_url','redirect_status','extra_flag','headers_shape','response_cache','frame_mismatch']),
  guardEpoch:Object.freeze(['unknown','before_seed','post_seed','reload_armed','document_committed']),
  guardDiskCache:Object.freeze(['unknown','absent','false','true','malformed']),
  guardServiceWorker:Object.freeze(['unknown','absent','false','true','malformed']),
  guardPrefetchCache:Object.freeze(['unknown','absent','false','true','malformed']),
  guardRequestFrame:Object.freeze(['unknown','absent','malformed','different','matching']),
  guardHeaderReason:Object.freeze(['unknown','name_unknown','syntax','attribute_unsupported','attribute_invalid','scope_domain','scope_path','metadata_shape','capacity']),
});
const boundTraces=new WeakMap();
export function closedStartupTrace(input={}){
  const result={};
  for(const [key,values]of Object.entries(STARTUP_TRACE)){
    const d=input!==null&&typeof input==='object'?Object.getOwnPropertyDescriptor(input,key):null;
    result[key]=d&&Object.hasOwn(d,'value')&&values.includes(d.value)?d.value:'unknown';
  }
  return Object.freeze(result);
}
export function rememberBoundStartup(report,trace){boundTraces.set(report,closedStartupTrace(trace));return report;}
const brands=new WeakMap();
const messages=['manual_startup_unavailable','manual_startup_owner_unavailable','xnp_manual_owner_unavailable'];
export function startupDiagnostic(error){return error!==null&&(typeof error==='object'||typeof error==='function')?brands.get(error)??null:null;}
export function startupFailure(role,phase,reason,peer=false,message=messages[0],trace){
  if(!STARTUP_ROLES.includes(role)||!STARTUP_PHASES.includes(phase)||!STARTUP_REASONS.includes(reason)){role='unknown';phase='acquire_rpc';reason='unavailable';}
  const error=new Error(messages.includes(message)?message:messages[0]);
  brands.set(error,Object.freeze({role,phase,reason,peer:peer===true,...(trace?{trace:closedStartupTrace(trace)}:{})}));return error;
}
export function preserveStartup(error,role,phase,reason='unavailable',message=messages[0]){
  const d=startupDiagnostic(error);
  return d?startupFailure(d.role==='unknown'?role:d.role,d.phase,d.reason,d.peer,message,d.trace):startupFailure(role,phase,reason,false,message);
}
export function startupRPCCode(error){
  const d=startupDiagnostic(error)??{role:'unknown',phase:'acquire_rpc',reason:'unavailable',peer:false};
  if(d.trace){
    const head=[STARTUP_ROLES.indexOf(d.role),STARTUP_PHASES.indexOf(d.phase),STARTUP_REASONS.indexOf(d.reason),d.peer?1:0];
    const tail=Object.entries(STARTUP_TRACE).map(([key,values])=>values.indexOf(d.trace[key]));
    return 'manual_initial_v7_'+[...head,...tail].join('_');
  }
  return `manual_initial_${d.role}_${d.phase}_${d.reason}_${d.peer?'peer':'primary'}`;
}
export function startupFailureLatch(){
  let first=null;
  return Object.freeze({
    record(error,role,phase,reason='unavailable',message=messages[0]){
      const wrapped=preserveStartup(error,role,phase,reason,message),d=startupDiagnostic(wrapped);
      if(first===null)first=d;
      // Enrich only the original observation. Its cause and origin never change.
      if(first.trace&&d.trace&&first.role===d.role&&role===first.role&&!d.peer&&first.trace.firstCause===d.trace.firstCause){
        const trace={...first.trace};
        for(const key of Object.keys(STARTUP_TRACE)){
          if(['guardDiskCache','guardServiceWorker','guardPrefetchCache','guardRequestFrame','guardHeaderReason'].includes(key))continue;
          if(key==='guardCondition'&&d.trace.guardCondition==='response_cache'&&trace.guardCondition!=='response_cache')continue;
          if(trace[key]==='unknown'&&d.trace[key]!=='unknown')trace[key]=d.trace[key];
        }
        first=Object.freeze({...first,trace:closedStartupTrace(trace)});
      }
      return startupFailure(first.role,first.phase,first.reason,role!==first.role&&role!=='pair'||d.peer,message,first.trace);
    },
    current(){return first;},
  });
}

export function requireBoundStartup(observation,role,diagnostics){
  const diagnostic=observation!==null&&typeof observation==='object'?Object.getOwnPropertyDescriptor(observation,'diagnostic')?.value:undefined;
  if(diagnostic==='bound')return;
  const trace=observation!==null&&typeof observation==='object'?boundTraces.get(observation):null;
  const reason=['cookie_candidate','cookie_ambiguous',...STARTUP_GUARD_CAUSES].includes(trace?.firstCause)?'observed_cookie_write':STARTUP_REASONS.includes(diagnostic)?diagnostic:'unavailable';
  const phase=reason==='identity_unavailable'?'identity':reason==='source_unavailable'?'settings':reason==='cleanup_unverified'?'cleanup':'bound_finish';
  throw diagnostics.record(startupFailure(role,phase,reason,false,undefined,trace),role,phase,reason);
}
