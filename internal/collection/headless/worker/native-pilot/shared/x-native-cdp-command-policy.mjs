import {MANUAL_TIMING} from './x-manual-timing-policy.mjs';
import {cdpCommandFailureDetail} from '../cdp-command-failure.mjs';

// Closed CDP vocabulary, timeout constants and value-free failure categories.
const MAX_PENDING_COMMANDS = 32;

const COMMAND_TIMEOUT_MS = MANUAL_TIMING.cdpCommandMS;

const CDP_METHODS = Object.freeze({
  'Target.getTargets': 'target_get_targets',
  'Target.attachToTarget': 'target_attach',
  'Target.detachFromTarget': 'target_detach',
  'Target.closeTarget': 'qa_ab_close_target',
  'Target.createBrowserContext': 'qa_guest_context',
  'Target.createTarget': 'qa_guest_target',
  'Target.disposeBrowserContext': 'qa_guest_dispose',
  'Page.navigate': 'qa_guest_home',
  'Page.enable': 'page_enable',
  'Page.reload': 'page_reload',
  'Page.addScriptToEvaluateOnNewDocument': 'qa_web_install',
  'Page.removeScriptToEvaluateOnNewDocument': 'qa_web_remove',
  'Runtime.evaluate': 'runtime_evaluate',
  'Debugger.evaluateOnCallFrame': 'early_evaluate_call_frame',
  'Network.enable': 'network_enable',
  'Network.disable': 'network_disable',
  'Network.getResponseBody': 'account_settings_body',
  'Network.getCookies': 'qa_cookie_metadata',
  'Network.setCacheDisabled': 'account_settings_cache',
  'Network.setBypassServiceWorker': 'account_settings_cache',
  'Browser.close': 'browser_close',
  'Browser.getWindowForTarget': 'qa_short_window',
  'Page.getFrameTree': 'early_frame_tree',
  'Runtime.enable': 'early_runtime_enable',
  'Runtime.disable': 'early_runtime_disable',
  'Debugger.enable': 'early_debugger_enable',
  'Debugger.disable': 'early_debugger_disable',
  'Debugger.setInstrumentationBreakpoint': 'early_instrumentation',
  'Debugger.setBreakpoint': 'early_normal_breakpoint',
  'Debugger.removeBreakpoint': 'early_remove_breakpoint',
  'Debugger.resume': 'early_resume',
});

// Closed, value-free startup diagnostics. Only the shared transport's private
// brand can distinguish its command failures; arbitrary exception text cannot.
function startupCDPFailureCategory(code,error){
  if(code==='borrowed_reader_command_failed'){
    switch(cdpCommandFailureDetail(error)?.kind){
      case 'command_rejected':return 'cdp_command_rejected';
      case 'request_rejected':return 'cdp_request_rejected';
      case 'reply_invalid':return 'cdp_reply_invalid';
      case 'transport_closed':return 'cdp_transport_closed';
      case 'timeout':return 'cdp_timeout';
      default:return 'cdp_borrowed_unclassified';
    }
  }
  if(code==='borrowed_reader_event_failed')return 'cdp_event_failure';
  if(['cdp_pipe_error','cdp_pipe_closed','cdp_write_failed'].includes(code))return 'cdp_pipe_failure';
  if(['chrome_process_error','chrome_exited'].includes(code))return 'cdp_process_failure';
  if(['cdp_frame_count_exceeded','cdp_frame_too_large','cdp_input_invalid','cdp_json_invalid','cdp_utf8_invalid'].includes(code))return 'cdp_frame_failure';
  if(code==='application_post_timeout'||Object.values(CDP_METHODS).some(name=>code===`${name}_timeout`))return 'cdp_timeout';
  return 'cdp_other';
}

const EARLY_CDP_METHODS = new Set(['Page.getFrameTree', 'Runtime.enable', 'Runtime.disable', 'Debugger.enable',
  'Debugger.disable', 'Debugger.setInstrumentationBreakpoint', 'Debugger.setBreakpoint', 'Debugger.removeBreakpoint', 'Debugger.resume']);

const EARLY_COMMAND_LABELS = Object.freeze({ 'Page.getFrameTree': 'frameTree', 'Runtime.enable': 'runtimeEnable',
  'Debugger.enable': 'debuggerEnable', 'Debugger.setInstrumentationBreakpoint': 'breakpointSet', 'Page.reload': 'reload',
  'Debugger.resume': 'resume', 'Debugger.removeBreakpoint': 'breakpointRemove', 'Debugger.disable': 'debuggerDisable',
  'Runtime.disable': 'runtimeDisable', 'Debugger.setBreakpoint': 'normalBreakpointSet', 'Debugger.evaluateOnCallFrame': 'pausedRead' });

const EARLY_LABELS = Object.freeze([...Object.values(EARLY_COMMAND_LABELS), 'normalBreakpointRemove']);

export {MAX_PENDING_COMMANDS, COMMAND_TIMEOUT_MS, CDP_METHODS, startupCDPFailureCategory, EARLY_CDP_METHODS, EARLY_COMMAND_LABELS, EARLY_LABELS};
