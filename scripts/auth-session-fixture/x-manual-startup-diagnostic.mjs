// Closed private initial-evidence diagnostics. Arbitrary exception messages,
// properties and patterned xnp_* strings never become trusted categories.
export const STARTUP_ROLES=Object.freeze(['reader','collector','pair','unknown']);
export const STARTUP_PHASES=Object.freeze(['prepare','inspect_before','inspect_after','acquire_rpc','identity','rendezvous','settings','cleanup','bound_finish','wire_decode','joint','navigation','page_ready','guard_bind']);
export const STARTUP_REASONS=Object.freeze(['unavailable','rejected','identity_unavailable','source_unavailable','stale_observation','cleanup_unverified','scope_lost','observed_cookie_write','deadline','canceled','replay','sample_missing','sample_duplicate','invalid_shape','scope_mismatch','time_invalid']);
export const STARTUP_GUARD_CAUSES=Object.freeze(['guard_request_unbound','guard_request_budget','guard_header_classification','guard_response_correlation','guard_metadata_shape','guard_completion_missing','guard_header_budget','guard_event_budget']);
// Append-only catalogs: V9 adds preparation predicates without raw owner data.
export const STARTUP_TRACE=Object.freeze({
  firstCause:Object.freeze(['unknown','cookie_candidate','cookie_ambiguous','context_lost','target_lost','navigation','owner_scope','event_cap','identity_order','identity_mismatch','settings_order','response_frame','extra_shape','extra_status','extra_missing','extra_unexpected','generic_early','stale_observation','cleanup_failed',...STARTUP_GUARD_CAUSES,'root_control','inspection','settings_capture','preparation']),
  terminal:Object.freeze(['unknown',...STARTUP_REASONS,'bound']),
  early:Object.freeze(['unknown','captured','deadline','script_limit','wrong_page','unexpected_pause','evaluation_failed','setup_failed']),
  handoff:Object.freeze(['unknown','accepted','rejected']),
  cookie:Object.freeze(['unknown','not_observed','candidate_observed','ambiguous','other_only',...STARTUP_GUARD_CAUSES]),
  extra:Object.freeze(['unknown','not_selected','not_required','required_pending','complete','missing','status_mismatch','unexpected','invalid']),
  guardTrigger:Object.freeze(['unknown','baseline','arm','metadata','request','document','response','extra','redirect','finish']),
  guardProvenance:Object.freeze(['unknown','absent','scoped','origin_scope','frame_scope','loader_missing','redirect','reused','root_scope']),
  guardCondition:Object.freeze(['unknown','predecessor_missing','redirect_shape','previous_response','chain_completed','chain_failed','chain_cache','previous_resource','current_resource','previous_origin','previous_frame','previous_loader','current_origin','current_frame','current_loader','loader_mismatch','redirect_url','redirect_status','extra_flag','headers_shape','response_cache','frame_mismatch','request_shape','request_url','protected_source','resource_type','request_method','session_missing','frame_malformed','loader_malformed','ordinary','request_absent','root_arm_closed','root_arm_repeated','root_arm_before_seed','root_arm_loader','root_arm_session','root_commit_duplicate','root_commit_loader_invalid','root_commit_old_loader','root_commit_url','root_commit_parent','root_commit_loader_mismatch','root_request_duplicate','root_request_redirect','root_request_history','root_request_method','root_request_url','root_request_loader_invalid','root_request_old_loader','root_request_commit_mismatch','root_request_reused','root_response_duplicate','root_response_loader','root_response_url','root_response_status','root_response_extra_flag','root_response_disk_cache','root_response_service_worker','root_response_prefetch_cache','root_extra_duplicate','root_extra_status','root_loading_failed','root_served_from_cache','root_extra_unexpected','root_missing','root_response_missing','root_commit_missing','root_extra_missing','root_response_frame_absent','root_response_frame_malformed','root_response_frame_different','root_control_apply','root_control_restore']),
  guardEpoch:Object.freeze(['unknown','before_seed','post_seed','reload_armed','document_committed']),
  guardDiskCache:Object.freeze(['unknown','absent','false','true','malformed']),
  guardServiceWorker:Object.freeze(['unknown','absent','false','true','malformed']),
  guardPrefetchCache:Object.freeze(['unknown','absent','false','true','malformed']),
  guardRequestFrame:Object.freeze(['unknown','absent','malformed','different','matching']),
  guardHeaderReason:Object.freeze(['unknown','name_unknown','syntax','attribute_unsupported','attribute_invalid','scope_domain','scope_path','metadata_shape','capacity']),
  inspectStage:Object.freeze(['unknown','node_snapshot','node_validate','go_stored','go_core_before','go_rpc','go_decode','go_validate','go_bind','go_core_after','snapshot_attach','snapshot_page','snapshot_runtime','snapshot_frame','snapshot_inventory']),
  inspectCondition:Object.freeze(['unknown','command_failed','snapshot_shape','owner_dead','reference_invalid','headless_mismatch','window_count','bootstrap_count','reader_windows','reader_bootstrap','collector_windows','owner_mismatch','profile_mismatch','target_mismatch','session_mismatch','frame_mismatch','document_mismatch','context_mismatch','lifecycle_unready','handle_mismatch','os_snapshot','reply_invalid','invalid_flag','paused_missing','pause_required','inventory_missing','pins_mismatch','os_changed']),
  prepareStage:Object.freeze(['unknown','go_admission','go_core_before','go_request','go_rpc','go_decode','go_core_after','go_bind','node_actor','node_reply','node_snapshot','node_validate','node_distinct','actor_entry','actor_pins','actor_ready','actor_runner','driver_setup','snapshot_attach','snapshot_page','snapshot_runtime','snapshot_frame','snapshot_inventory']),
  prepareCondition:Object.freeze(['unknown','replay','os_snapshot','owner_shape','reference_invalid','profile_mismatch','parent_mismatch','attempt_mismatch','command_failed','reply_invalid','handle_invalid','pins_invalid','os_changed','owner_mismatch','state_invalid','ready_missing','roles_not_distinct','snapshot_shape','owner_dead','headless_mismatch','window_count','bootstrap_count','reader_windows','reader_bootstrap','collector_windows','target_mismatch','session_mismatch','frame_mismatch','document_mismatch','context_mismatch','lifecycle_unready','windows_scope','parent_changed','chrome_changed','image_changed','node_parent','chrome_parent','job_unavailable','job_changed','directory_changed','collector_not_headless']),
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
// Only trusted call sites select these categories; exception text is never read.
export function startupInspectionFailure(error,role,phase,inspectStage,inspectCondition){
  if(phase==='prepare')return startupPrepareFailure(error,role,inspectStage,inspectCondition);
  return startupDiagnostic(error)?preserveStartup(error,role,phase,'unavailable',messages[2]):
    startupFailure(role,phase,'unavailable',false,messages[2],{firstCause:'inspection',inspectStage,inspectCondition});
}
export function startupPrepareFailure(error,role,prepareStage,prepareCondition){
  return startupDiagnostic(error)?preserveStartup(error,role,'prepare','unavailable',messages[2]):
    startupFailure(role,'prepare','unavailable',false,messages[2],{firstCause:'preparation',prepareStage,prepareCondition});
}
export function startupRPCCode(error){
  const d=startupDiagnostic(error)??{role:'unknown',phase:'acquire_rpc',reason:'unavailable',peer:false};
  if(d.trace){
    const head=[STARTUP_ROLES.indexOf(d.role),STARTUP_PHASES.indexOf(d.phase),STARTUP_REASONS.indexOf(d.reason),d.peer?1:0];
    const tail=Object.entries(STARTUP_TRACE).map(([key,values])=>values.indexOf(d.trace[key]));
    return 'manual_initial_v9_'+[...head,...tail].join('_');
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
          if(['guardDiskCache','guardServiceWorker','guardPrefetchCache','guardRequestFrame','guardHeaderReason','inspectStage','inspectCondition','prepareStage','prepareCondition'].includes(key))continue;
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
  const reason=trace?.guardProvenance==='root_scope'&&STARTUP_GUARD_CAUSES.includes(trace?.firstCause)?'scope_lost':['cookie_candidate','cookie_ambiguous',...STARTUP_GUARD_CAUSES].includes(trace?.firstCause)?'observed_cookie_write':STARTUP_REASONS.includes(diagnostic)?diagnostic:'unavailable';
  const phase=reason==='identity_unavailable'?'identity':reason==='source_unavailable'?'settings':reason==='cleanup_unverified'?'cleanup':'bound_finish';
  throw diagnostics.record(startupFailure(role,phase,reason,false,undefined,trace),role,phase,reason);
}
