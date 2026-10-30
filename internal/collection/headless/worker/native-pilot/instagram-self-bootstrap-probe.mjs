import {INSTAGRAM_BOOTSTRAP_BUDGET,validateInstagramBootstrapBudget} from './instagram-bootstrap-budget.mjs';
import {STRICT_JSON_PARSER_SOURCE} from '../../../../../scripts/auth-session-fixture/x-evidence-ingress.mjs';
// Candidate discovery only. This module never issues identity proof or Store
// authority. Native source qualification must establish the exact self schema.
import {bootstrapFailure,bootstrapFailureDetail,BOOTSTRAP_STAGES,BOOTSTRAP_CONDITIONS,BOOTSTRAP_CLEANUP_STAGES,BOOTSTRAP_CLEANUP_CONDITIONS} from './instagram-bootstrap-failure.mjs';
const schema = 'aku.instagram-self-bootstrap-probe.v1';
export const INSTAGRAM_BOOTSTRAP_BOUNDS = Object.freeze(['bounds_script_count','bounds_script_chars','bounds_script_bytes','bounds_total_bytes','bounds_nodes','bounds_depth','bounds_fanout','bounds_candidate_bytes','bounds_parse_bytes','bounds_parse_tokens','bounds_entries','scan_deadline','scan_clock']);
const reasons = ['candidate', 'missing', 'duplicate', 'conflict', 'shape', 'bounds', ...INSTAGRAM_BOOTSTRAP_BOUNDS, 'invalid_json', 'page_scope', 'login_visible', 'challenge', 'document_changed', 'unavailable'];
const fields = ['schema', 'reason', 'modulePresent', 'idString', 'idSafeNumber', 'usernameString', 'documentBound', 'serverFresh', 'qualified', 'scanComplete', 'budgetExpanded'];
const rejected = () => new Error('instagram_self_bootstrap_probe_rejected');

// Reads only JSON script nodes; never invokes page modules or executes scripts.
// PolarisViewer is a discovery candidate, not a qualified Instagram contract.
function scanInstagramBootstrap(policy,parseJSON){
 const base={schema:'aku.instagram-self-bootstrap-probe.v1',reason:'missing',modulePresent:false,idString:false,idSafeNumber:false,usernameString:false,documentBound:false,serverFresh:false,qualified:false,scanComplete:false,budgetExpanded:false};
 const out=(reason,complete=false)=>({...base,reason,scanComplete:complete});
 const started=performance.now(),deadline=started+policy.deadlineMS;let last=started;
 const check=()=>{const time=performance.now();if(!Number.isFinite(time)||time<last)throw 'scan_clock';last=time;if(time>=deadline)throw 'scan_deadline';};
 const allowance=Object.fromEntries(Object.entries(policy).filter(([,v])=>typeof v==='object').map(([k,v])=>[k,v.initial]));
 const charge=(key,amount,reason)=>{check();if(amount>allowance[key]){if(amount>policy[key].max)throw reason;allowance[key]=policy[key].max;base.budgetExpanded=true;}};
 try{
  check();
  if(location.origin!=='https://www.instagram.com'||location.pathname!=='/')return out('page_scope');
  if(document.querySelector('input[type="password"],form[action*="login"]'))return out('login_visible');
  if(document.querySelector('form[action*="challenge"],iframe[src*="captcha"]'))return out('challenge');
  // Synchronous inspection keeps this snapshot stable against page JS tasks.
  const scripts=document.querySelectorAll('script[type="application/json"]');check();
  charge('scripts',scripts.length,'bounds_script_count');
  let scanBytes=0,parseBytes=0,containers=0,entries=0,match,duplicate=false,conflict=false;
  const encoder=new TextEncoder();
  for(const script of scripts){
   check();const text=script.textContent||'';check();
   // UTF-16 length is a cheap allocation guard before UTF-8 encoding.
   if(text.length>policy.scriptBytes.max)throw 'bounds_script_chars';
   charge('scriptBytes',text.length,'bounds_script_chars');
   const size=encoder.encode(text).byteLength;check();
   charge('scriptBytes',size,'bounds_script_bytes');
   scanBytes+=size;charge('scanBytes',scanBytes,'bounds_total_bytes');
   // Escaped JSON names cannot be excluded by raw substring matching.
   if(!text.includes('PolarisViewer')&&!text.includes('\\'))continue;
   charge('candidateBytes',size,'bounds_candidate_bytes');
   parseBytes+=size;charge('parseBytes',parseBytes,'bounds_parse_bytes');
   let json,parseReason;
   try{json=parseJSON(text,{maxJSONDepth:policy.depth,maxJSONTokens:policy.entries.max+policy.containers.max},reason=>{parseReason=reason;});}
   catch{throw parseReason==='depth_limit'?'bounds_depth':parseReason==='value_limit'?'bounds_parse_tokens':'invalid_json';}
   check();
   const pending=[{value:json,depth:0}];
   while(pending.length){
    check();const {value,depth}=pending.pop();
    if(!value||typeof value!=='object')continue;
    if(depth>policy.depth)throw 'bounds_depth';
    charge('containers',++containers,'bounds_nodes');
    if(Array.isArray(value)&&value[0]==='PolarisViewer'){
     base.modulePresent=true;
     if(value.length!==4||!Array.isArray(value[1])||value[1].length!==0||!value[2]||typeof value[2]!=='object'||Array.isArray(value[2]))return out('shape');
     const data=value[2];if(!Object.hasOwn(data,'id')||!Object.hasOwn(data,'username'))return out('shape');
     const id=data.id,username=data.username;
     const idString=typeof id==='string'&&/^[1-9][0-9]{0,29}$/.test(id),idSafeNumber=Number.isSafeInteger(id)&&id>0;
     if(!(idString||idSafeNumber)||typeof username!=='string'||!/^[A-Za-z0-9._]{1,30}$/.test(username))return out('shape');
     if(match){if(String(match.id)!==String(id)||match.username!==username)conflict=true;else duplicate=true;}
     else{match={id,username};base.idString=idString;base.idSafeNumber=idSafeNumber;base.usernameString=true;}
     // Keep traversing: a nested tuple can contain a duplicate or conflict.
    }
    const keys=Object.keys(value);check();
    entries+=keys.length;charge('entries',entries,'bounds_entries');
    for(const key of keys){check();const child=value[key];if(child&&typeof child==='object')pending.push({value:child,depth:depth+1});}
   }
  }
  check();
  if(location.origin!=='https://www.instagram.com'||location.pathname!=='/')return out('document_changed');
  if(document.querySelector('input[type="password"],form[action*="login"]'))return out('login_visible');
  return out(conflict?'conflict':duplicate?'duplicate':match?'candidate':'missing',true);
 }catch(reason){
  const allowed=['bounds_script_count','bounds_script_chars','bounds_script_bytes','bounds_total_bytes','bounds_candidate_bytes','bounds_parse_bytes','bounds_parse_tokens','bounds_nodes','bounds_entries','bounds_depth','scan_deadline','scan_clock','invalid_json'];
  return out(allowed.includes(reason)?reason:'unavailable');
 }
}

export function instagramSelfBootstrapExpression(policy=INSTAGRAM_BOOTSTRAP_BUDGET){
 const checked=validateInstagramBootstrapBudget(policy);
 return `(${scanInstagramBootstrap.toString()})(${JSON.stringify(checked)},${STRICT_JSON_PARSER_SOURCE})`;
}

export function validateInstagramSelfBootstrapProbe(value) {
  if (!value || ![Object.prototype, null].includes(Object.getPrototypeOf(value)) || Reflect.ownKeys(value).length !== fields.length) throw rejected();
  for (const key of fields) if (!Object.hasOwn(Object.getOwnPropertyDescriptor(value, key) ?? {}, 'value')) throw rejected();
  if (['candidate','missing','duplicate','conflict'].includes(value.reason)!==value.scanComplete) throw rejected();
  if (value.schema !== schema || !reasons.includes(value.reason) || fields.slice(2).some(key => typeof value[key] !== 'boolean') ||
      value.serverFresh !== false || value.qualified !== false || Buffer.byteLength(JSON.stringify(value)) > 1024) throw rejected();
  return Object.freeze(Object.fromEntries(fields.map(key => [key, value[key]])));
}

// Existing exact owned page only. No allocation, navigation, cookie access or
// network request. The caller retains ownership and controls its cleanup.
export async function runInstagramSelfBootstrapProbe(page, onFailure=()=>{}) {
  const empty = reason => ({schema, reason, modulePresent: false, idString: false, idSafeNumber: false, usernameString: false, documentBound: false, serverFresh: false, qualified: false, scanComplete:false, budgetExpanded:false});
  const frame = async () => {
    const tree = await page.send('Page.getFrameTree', {}, INSTAGRAM_BOOTSTRAP_BUDGET.frameReadMS);
    const root = tree?.frameTree?.frame;
    if (!root || typeof root.id !== 'string' || !root.id || typeof root.loaderId !== 'string' || !root.loaderId || root.url !== 'https://www.instagram.com/' || root.parentId !== undefined) throw rejected();
    return {id: root.id, loaderId: root.loaderId};
  };
  let phase='frame_before';
  try {
    const before = await frame();
    phase='evaluation';
    const result = validateInstagramSelfBootstrapProbe(await page.evaluate(instagramSelfBootstrapExpression(), INSTAGRAM_BOOTSTRAP_BUDGET.evaluationTimeoutMS));
    phase='frame_after';
    const after = await frame();
    if (before.id !== after.id || before.loaderId !== after.loaderId) {onFailure(bootstrapFailure(phase,'document_changed'));return validateInstagramSelfBootstrapProbe(empty('document_changed'));}
    return validateInstagramSelfBootstrapProbe({...result, documentBound: true});
  } catch(error) { onFailure(bootstrapFailure(phase,bootstrapFailureDetail(error)?.condition??(phase==='evaluation'?'invalid_projection':'document_changed')));return validateInstagramSelfBootstrapProbe(empty('unavailable')); }
}

// Qualification planning only: a fresh read timestamp cannot qualify old
// bootstrap bytes. Kept out of the production verifier and registry entirely.
export function assessInstagramBootstrapDelivery(value, nowMS) {
  if (!value || !Number.isSafeInteger(nowMS) || !Number.isSafeInteger(value.receivedAtMS) || value.receivedAtMS > nowMS || nowMS - value.receivedAtMS > 6000) return 'stale_or_missing_delivery';
  if (value.sameDocument !== true || value.mainFrame !== true || value.sameOrigin !== true || value.serverDelivered !== true || value.cacheOrServiceWorker !== false) return 'delivery_unproven';
  if (value.selfSemanticsQualified !== true) return 'self_schema_unqualified';
  // Even structurally eligible metadata cannot mint proof; the trusted native
  // observation and independently qualified identity parser are still required.
  return 'eligible_for_native_review';
}

// The diagnostic transport chooses an existing, unambiguous Home target.
// It never creates/navigates/closes a user target; only its attachment retires.
export async function diagnoseInstagramSelfBootstrap(transport) {
  const scope=transport.createBootstrapDiagnosticScope();
  let result={schema,reason:'unavailable',modulePresent:false,idString:false,idSafeNumber:false,usernameString:false,documentBound:false,serverFresh:false,qualified:false,scanComplete:false,budgetExpanded:false};
  let first,cleanup='verified',cleanupStage='none',cleanupCondition='none';
  const remember=error=>{first??=bootstrapFailureDetail(error)??{stage:'attachment',condition:'request_failed'};};
  try {
    const page=await scope.attach();
    result=await runInstagramSelfBootstrapProbe(page,remember);
  } catch(error) {remember(error);}
  finally {
    try{await scope.close();}catch{cleanup='uncertain';const detail=scope.cleanupDetail?.();cleanupStage=detail?.cleanupStage??'detach';cleanupCondition=detail?.cleanupCondition??'request_failed';}
  }
  return validateInstagramBootstrapDiagnostic({...result,failureStage:first?.stage??'none',failureCondition:first?.condition??'none',cleanup,cleanupStage,cleanupCondition});
}

export function validateInstagramBootstrapDiagnostic(value) {
  if(!value||!Object.hasOwn(value,'cleanup')||!['verified','uncertain'].includes(value.cleanup))throw rejected();
  const {cleanup,failureStage,failureCondition,cleanupStage,cleanupCondition,...probe}=value;
  if(!BOOTSTRAP_STAGES.includes(failureStage)||!BOOTSTRAP_CONDITIONS.includes(failureCondition)||!BOOTSTRAP_CLEANUP_STAGES.includes(cleanupStage)||!BOOTSTRAP_CLEANUP_CONDITIONS.includes(cleanupCondition)||
    (failureStage==='none')!==(failureCondition==='none')||(cleanupStage==='none')!==(cleanupCondition==='none')||cleanup==='verified'&&cleanupStage!=='none'||cleanup==='uncertain'&&cleanupStage==='none')throw rejected();
  if(failureStage!=='none'&&probe.scanComplete!==false)throw rejected();
  return Object.freeze({...validateInstagramSelfBootstrapProbe(probe),failureStage,failureCondition,cleanup,cleanupStage,cleanupCondition});
}
