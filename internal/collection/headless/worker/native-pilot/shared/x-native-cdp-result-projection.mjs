import {projectAccountSettingsBody} from './x-account-settings-body.mjs';
import {validateBootstrapCurrentSelf} from './x-bootstrap-current-self.mjs';
import {projectQASourceBody, projectQACookieMetadata} from './x-qa-source-qualification.mjs';
import {projectQADirectBody} from './x-qa-direct-ab.mjs';
import {validateQAWebHook} from './x-qa-web-aba.mjs';
import {projectQA401Body} from './x-qa-401-projection.mjs';
import {projectBoundBootstrap} from './x-qa-bound-observation.mjs';
import {selectQAShortCookies} from './x-qa-short-feasibility.mjs';
import {projectQALiveSelf} from './x-qa-live-self.mjs';
import {manualSeedFailure} from './x-manual-retained-setup.mjs';

// The runtime retains state ownership, command admission, deadlines and cleanup.
export function createCDPResultProjector({
  getActive, getDriverMode, bodyMode, isHomeTarget, isObject, isTargetURL,
  own, probeError, qualificationMode, safeId,
}) {
  function projectCDPResult(method, result, projectionKind = null) {
    if(projectionKind==='application_post'){if(!isObject(result))throw probeError('application_post_unavailable');return result;}
    if(projectionKind==='manual_root_control'){if(!isObject(result)||Reflect.ownKeys(result).length!==0)throw probeError('manual_root_control_ack');return{acknowledged:true};}
    if (!isObject(result)) throw probeError('cdp_result_invalid');
    if(projectionKind==='manual_enrollment_capture')return getActive().manualEnrollment.project(method,result);
    switch (method) {
      case 'Page.addScriptToEvaluateOnNewDocument':
        if (!safeId(result.identifier)) throw probeError('qa_web_identifier_invalid');
        return { identifier: result.identifier };
      case 'Page.getFrameTree': {
        const frame = result.frameTree?.frame;
        if (!isObject(frame) || !safeId(frame.id) || !(['qa_guest', 'qa_ab'].includes(projectionKind) ? frame.url === 'about:blank' : isHomeTarget(frame.url))) throw probeError('early_frame_invalid');
        if (projectionKind === 'qa_live_self' || projectionKind === 'manual_frame') {
          if (!safeId(frame.loaderId)) throw probeError('qa_live_document_invalid');
          return { frameId: frame.id, loaderId: frame.loaderId };
        }
        return { frameId: frame.id };
      }
      case 'Target.createBrowserContext':
        if (!safeId(result.browserContextId)) throw probeError('qa_guest_context_invalid');
        return { browserContextId: result.browserContextId };
      case 'Target.createTarget':
        if (!safeId(result.targetId)) throw probeError('qa_guest_target_invalid');
        return { targetId: result.targetId };
      case 'Page.navigate':
        if (projectionKind === 'qa_ab') {
          if (!safeId(result.frameId) || result.frameId !== getActive()?.ab?.frameId ||
              own(result, 'errorText') && result.errorText !== 'net::ERR_HTTP_RESPONSE_CODE_FAILURE') throw probeError('qa_ab_navigation_failed');
          return { acknowledged: true };
        }
        if (own(result, 'errorText') || !safeId(result.frameId) || result.frameId !== getActive()?.guest?.frameId) throw probeError('qa_guest_navigation_invalid');
        return { acknowledged: true };
      case 'Debugger.setInstrumentationBreakpoint':
        if (!safeId(result.breakpointId)) throw probeError('early_breakpoint_invalid');
        getActive().earlyInstrumentationId = result.breakpointId;
        return { breakpointId: result.breakpointId };
      case 'Debugger.setBreakpoint': {
        const handoff = getActive()?.earlyCapture?.handoff;
        if (!handoff) throw probeError('early_normal_breakpoint_invalid');
        handoff.created(result.breakpointId);
        const location = result.actualLocation;
        const requested = handoff.installing;
        if (!requested || !isObject(location) || location.scriptId !== requested.scriptId ||
            !Number.isSafeInteger(location.lineNumber) || location.lineNumber < 0 || location.lineNumber > 1000000 ||
            !Number.isSafeInteger(location.columnNumber) || location.columnNumber < 0 || location.columnNumber > 1000000) throw probeError('early_normal_breakpoint_invalid');
        return { breakpointId: result.breakpointId, actualLocation: { scriptId: location.scriptId, lineNumber: location.lineNumber, columnNumber: location.columnNumber } };
      }
      case 'Network.getResponseBody':
        if (!bodyMode(getDriverMode())) throw probeError('body_mode_required');
        if (projectionKind === 'qa_web_401') {
          if (!getActive()?.webABA?.qualify401 || getActive().webABA.rejection !== null) throw probeError('qa_rejection_read_replayed');
          const projected = projectQA401Body(result);
          getActive().webABA.rejection = projected.rejection;
          return projected.body;
        }
        if (projectionKind === 'qa_bound') return projectQADirectBody(result);
        if (projectionKind === 'qa_ab' || projectionKind === 'qa_web') return projectQADirectBody(result);
        return qualificationMode(getDriverMode()) ? projectQASourceBody(result) : projectAccountSettingsBody(result);
      case 'Network.getCookies':
        if (projectionKind === 'qa_short_cookies') {
          try{return selectQAShortCookies(result);}
          catch(error){if(getActive()?.manualSetup)throw manualSeedFailure('cookie_select','rejected');throw error;}
        }
        if (!qualificationMode(getDriverMode()) || !getActive()?.qaCookieIssued) throw probeError('cookie_mode_required');
        return projectQACookieMetadata(result);
      case 'Target.getTargets': {
        if (!Array.isArray(result.targetInfos) || result.targetInfos.length > 128) throw probeError('target_inventory_invalid');
        const matches = [];
        for (const target of result.targetInfos) {
          if (isObject(target) && target.type === 'page' && (projectionKind === 'manual_inventory' || isTargetURL(target.url)) && safeId(target.targetId)) {
            matches.push(target.targetId);
          }
        }
        if(projectionKind==='manual_inventory')getActive().manualInventoryTargets=new Set(matches);
        return { targetIds: matches };
      }
      case 'Browser.getWindowForTarget':
        if (!Number.isSafeInteger(result.windowId) || result.windowId < 1) throw probeError('qa_short_window_invalid');
        return { windowId: result.windowId };
      case 'Target.attachToTarget':
        if (!safeId(result.sessionId)) throw probeError('target_session_invalid');
        return { sessionId: result.sessionId };
      case 'Target.closeTarget':
        if (result.success !== true) throw probeError('qa_ab_close_failed');
        return { acknowledged: true };
      case 'Debugger.evaluateOnCallFrame':
      case 'Runtime.evaluate': {
        if (projectionKind === 'qa_live_self') {
          if (!getActive()?.liveSelf?.running || own(result, 'exceptionDetails') || !isObject(result.result) || result.result.type !== 'object') throw probeError('qa_live_result_invalid');
          try { return projectQALiveSelf(result.result.value); } catch { throw probeError('qa_live_projection_invalid'); }
        }
        if (projectionKind === 'qa_web_hook') {
          if (own(result, 'exceptionDetails') || !isObject(result.result) || !isObject(result.result.value)) throw probeError('qa_web_hook_unavailable');
          try { return validateQAWebHook(result.result.value); } catch { throw probeError('qa_web_hook_invalid'); }
        }
        if (projectionKind === 'bootstrap_current_self') {
          if (!(getDriverMode() === 'retained_bootstrap' || getActive()?.bound?.running) || own(result, 'exceptionDetails') ||
              !isObject(result.result) || result.result.type !== 'object' || !isObject(result.result.value)) {
            throw probeError('bootstrap_runtime_result_invalid');
          }
          if (getActive()?.bound?.running) {
            return projectBoundBootstrap(result.result.value, (id, projection) => getActive().bound.binder.acceptIdentity(id, projection));
          }
          try { return validateBootstrapCurrentSelf(result.result.value); }
          catch { throw probeError('bootstrap_projection_invalid'); }
        }
        if (own(result, 'exceptionDetails') || !isObject(result.result) || result.result.type !== 'boolean' ||
            typeof result.result.value !== 'boolean') throw probeError('runtime_result_invalid');
        return { value: result.result.value };
      }
      case 'Page.enable':
      case 'Page.reload':
      case 'Page.removeScriptToEvaluateOnNewDocument':
      case 'Network.enable':
      case 'Network.disable':
      case 'Network.setBypassServiceWorker':
      case 'Network.setCacheDisabled':
      case 'Browser.close':
      case 'Target.disposeBrowserContext':
      case 'Runtime.enable':
      case 'Runtime.disable':
      case 'Debugger.enable':
      case 'Debugger.disable':
      case 'Debugger.removeBreakpoint':
      case 'Debugger.resume':
        return { acknowledged: true };
      default:
        throw probeError('cdp_method_invalid');
    }
  }

  return projectCDPResult;
}
