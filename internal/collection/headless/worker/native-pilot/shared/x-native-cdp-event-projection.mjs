import {isObject, probeError, own, safeId} from './x-native-value-validation.mjs';
import {isHomeTarget} from './x-native-observation-policy.mjs';
import {projectNetworkEvent} from './x-native-network-metadata.mjs';

// Opaque event projection before owner dispatch.
function projectCDPEvent(message) {
  if (typeof message.method !== 'string' || message.method.length > 128 || !isObject(message.params)) {
    throw probeError('cdp_event_invalid');
  }
  switch (message.method) {
    case 'Page.frameNavigated': {
      const frame = message.params.frame;
      if (!isObject(frame) || own(frame, 'parentId') || !safeId(frame.id)) return null;
      return { type: 'early', sessionId: safeId(message.sessionId) ? message.sessionId : null,
        event: { type: 'frame_navigated', frameId: frame.id, isHome: isHomeTarget(frame.url) } };
    }
    case 'Runtime.executionContextCreated': {
      const context = message.params.context;
      if (!isObject(context) || !Number.isSafeInteger(context.id) || context.id <= 0 ||
          context.auxData?.isDefault !== true || !safeId(context.auxData.frameId)) return null;
      return { type: 'early', sessionId: safeId(message.sessionId) ? message.sessionId : null,
        event: { type: 'context_created', contextId: context.id, frameId: context.auxData.frameId, isDefault: true } };
    }
    case 'Runtime.executionContextsCleared':
      return { type: 'early', sessionId: safeId(message.sessionId) ? message.sessionId : null, event: { type: 'contexts_cleared' } };
    case 'Runtime.executionContextDestroyed':
      if (!Number.isSafeInteger(message.params.executionContextId) || message.params.executionContextId <= 0) return null;
      return { type: 'early', sessionId: safeId(message.sessionId) ? message.sessionId : null,
        event: { type: 'context_destroyed', contextId: message.params.executionContextId } };
    case 'Debugger.scriptParsed': {
      const params = message.params;
      if (!safeId(params.scriptId) || !Number.isSafeInteger(params.executionContextId) || params.executionContextId <= 0 ||
          params.executionContextAuxData?.isDefault !== true || !safeId(params.executionContextAuxData.frameId)) return null;
      return { type: 'early', sessionId: safeId(message.sessionId) ? message.sessionId : null,
        event: { type: 'script_parsed', scriptId: params.scriptId, contextId: params.executionContextId,
          frameId: params.executionContextAuxData.frameId, isDefault: true } };
    }
    case 'Debugger.paused': {
      // Correlate only the top frame's opaque IDs. Never project its locals, scopes, URLs or source.
      const top = Array.isArray(message.params.callFrames) ? message.params.callFrames[0] : null;
      return { type: 'early', sessionId: safeId(message.sessionId) ? message.sessionId : null,
        event: { type: 'paused', reason: message.params.reason === 'instrumentation' ? 'instrumentation' : 'other',
          callFrameId: safeId(top?.callFrameId) ? top.callFrameId : null,
          scriptId: safeId(top?.location?.scriptId) ? top.location.scriptId : null } };
    }
    case 'Debugger.resumed':
      return { type: 'early', sessionId: safeId(message.sessionId) ? message.sessionId : null, event: { type: 'resumed' } };
    case 'Page.loadEventFired':
      return { type: 'page_load', sessionId: safeId(message.sessionId) ? message.sessionId : null };
    case 'Network.requestWillBeSent':
    case 'Network.responseReceived':
    case 'Network.requestServedFromCache':
    case 'Network.loadingFinished':
    case 'Network.loadingFailed':
      return {
        type: 'network', sessionId: safeId(message.sessionId) ? message.sessionId : null,
        event: projectNetworkEvent(message.method, message.params),
      };
    default:
      return null;
  }
}

export {projectCDPEvent};
