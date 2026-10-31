// Closed, payload-free worker lifecycle vocabulary shared with the Go reader.
const reasons=new Set(['explicit_close','command_timeout','input_end','input_error','output_error','invalid_frame','inbound_limit','reply_scope','listener_failure','sequence_limit','control_end','control_error','control_input_limit','control_output_limit','uncaught_exception','unhandled_rejection','entry_failed','unknown']);
const domains=new Set(['cdp','control','process','owner','unknown']);
const methods=new Set(['Browser.getVersion','Target.createTarget','Target.attachToTarget','Target.closeTarget','Target.getTargets','Page.enable','Runtime.enable','Network.enable','Page.getFrameTree','Page.navigate','Network.setCacheDisabled','Network.setBypassServiceWorker','Fetch.enable','Fetch.takeResponseBodyAsStream','IO.read','IO.close','Fetch.fulfillRequest','Fetch.disable','Runtime.evaluate','Network.getCookies','Network.setCookies','none','other']);
const roles=new Set(['ig_reader','ig_collector','diagnostic','ordinary','unknown']);
const stages=new Set(['command','framing','stream','shutdown','exception','unknown']);
const member=(catalog,value,fallback)=>catalog.has(value)?value:fallback;
export function mainTransportLifecycle({reason,domain,method,role,stage}={}){
  return Object.freeze({schema:'aku.main-transport-lifecycle.v1',reason:member(reasons,reason,'unknown'),domain:member(domains,domain,'unknown'),
    method:member(methods,method,method===undefined?'none':'other'),role:member(roles,role,'unknown'),stage:member(stages,stage,'unknown')});
}

// The monitor observes Node's default fatal path. It never consumes an error,
// reads exception text, or installs a rejection/exception recovery handler.
export function monitorMainTransportProcess(report,processObject=process){
  const monitor=(_error,origin)=>report({reason:origin==='unhandledRejection'?'unhandled_rejection':'uncaught_exception',domain:'process',method:'none',role:'unknown',stage:'exception'});
  processObject.on('uncaughtExceptionMonitor',monitor);
  return()=>processObject.removeListener('uncaughtExceptionMonitor',monitor);
}
