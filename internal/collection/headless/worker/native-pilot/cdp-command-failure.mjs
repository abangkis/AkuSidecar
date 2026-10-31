// Node-local provenance only. No provider text, parameters, IDs or raw reply is
// retained. Consumers own the mapping from these categories to their domain.
const failures=new WeakMap();
const kinds=new Set(['request_rejected','command_rejected','reply_invalid','transport_closed','timeout']);

export function cdpCommandFailure(kind,localTimeout=false){
  const error=new Error(kind==='timeout'&&localTimeout===true?'diagnostic_request_timeout':'main_transport_rejected');
  if(kinds.has(kind))failures.set(error,Object.freeze({kind}));
  return error;
}

export function cdpCommandFailureDetail(error){
  return error!==null&&(typeof error==='object'||typeof error==='function')?failures.get(error)??null:null;
}
