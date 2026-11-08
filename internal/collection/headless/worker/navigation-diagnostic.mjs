import { setTimeout as delay } from 'node:timers/promises';
import { captureError } from './observation.mjs';
import catalog from './navigation-catalog.json' with { type: 'json' };

const MAX_DOCUMENT_REQUESTS = 16;
const MAX_REQUEST_EXTRA_INFO = 64;
const FAILURE_EVENT_GRACE_MS = 120;
const HTTP_RESPONSE_FAILURE = 'net::ERR_HTTP_RESPONSE_CODE_FAILURE';
const validID = value => typeof value === 'string' && value.length > 0 && value.length <= 256;
const validHTTPStatus = value => Number.isInteger(value) && value >= 200 && value <= 599;

function errorCode(errorText) {
  return typeof errorText === 'string' && Object.hasOwn(catalog.errorCategories, errorText) ? errorText : undefined;
}

function requestIndicators(headers, source) {
  if (!headers || typeof headers !== 'object') return undefined;
  const indicators = {};
  for (const [name, value] of Object.entries(headers)) {
    const key = name.toLowerCase();
    if (key !== 'user-agent' && key !== 'sec-ch-ua') continue;
    if (typeof value !== 'string' || value.length > 4096 || !value.trim() || /[\r\n\0]/.test(value)) continue;
    switch (key) {
      case 'user-agent':
        indicators.userAgentHeadless = /\bHeadlessChrome\//i.test(value);
        break;
      case 'sec-ch-ua':
        indicators.clientHintsHeadless = /(?:^|,)\s*"HeadlessChrome"\s*;\s*v\s*=\s*"\d+(?:\.\d+)*"/i.test(value);
        break;
    }
  }
  return Object.keys(indicators).length ? {...indicators, requestMetadataSource: source} : undefined;
}

function observeMainDocument(page, url, collectRequestMetadata) {
  const requests = new Map();
  const requestExtras = new Map();
  let overflow = false;
  const off = page.on?.(({method, params}) => {
    if (!validID(params?.requestId)) return;
    const request = requests.get(params.requestId);
    if (collectRequestMetadata && method === 'Network.requestWillBeSentExtraInfo') {
      // ExtraInfo can arrive before the corresponding request event.
      if (requestExtras.has(params.requestId)) requestExtras.set(params.requestId, undefined);
      else if (requestExtras.size < MAX_REQUEST_EXTRA_INFO) {
        requestExtras.set(params.requestId, requestIndicators(params.headers, 'request_extra_info'));
      }
      return;
    }
    if (method === 'Network.requestWillBeSent' && params.type === 'Document') {
      if (params.redirectResponse) {
        if (request) {
          const redirectURL = params.request?.url;
          request.redirected = true;
          request.responseStatus = undefined;
          request.url = request.frameId === params.frameId && request.loaderId === params.loaderId &&
            typeof redirectURL === 'string' && redirectURL.length <= 4096 ? redirectURL : undefined;
        }
        return;
      }
      if (params.request?.url !== url || !validID(params.frameId) || !validID(params.loaderId)) return;
      if (request || requests.size >= MAX_DOCUMENT_REQUESTS) {
        overflow = true;
        return;
      }
      requests.set(params.requestId, {
        id: params.requestId, frameId: params.frameId, loaderId: params.loaderId, url, extraCount: 0,
        indicators: collectRequestMetadata
          ? requestIndicators(params.request.headers, 'request_will_be_sent') : undefined,
      });
      return;
    }
    if (!request) return;
    if (method === 'Network.responseReceivedExtraInfo') {
      // ExtraInfo has no hop or URL. A redirect reuses the request ID.
      request.extraCount = Math.min(2, request.extraCount + 1);
      request.extraStatus = validHTTPStatus(params.statusCode) ? params.statusCode : undefined;
      return;
    }
    if (params.type !== 'Document') return;
    if (method === 'Network.responseReceived' && params.frameId === request.frameId &&
        params.loaderId === request.loaderId && request.url && params.response?.url === request.url &&
        validHTTPStatus(params.response.status)) {
      request.responseStatus = params.response.status;
    } else if (method === 'Network.loadingFailed') {
      if (request.failure) return;
      request.failure = {
        state: 'matched',
        errorCode: errorCode(params.errorText),
        canceled: typeof params.canceled === 'boolean' ? params.canceled : undefined,
        blockedReason: catalog.blockedReasons.includes(params.blockedReason) ? params.blockedReason : undefined,
      };
    }
  });
  if (typeof off !== 'function') return undefined;
  return {
    off,
    documentFor({frameId, loaderId}, failed) {
      if (overflow || !validID(frameId) || !validID(loaderId)) return undefined;
      const matching = [...requests.values()].filter(request => request.frameId === frameId && request.loaderId === loaderId);
      if (matching.length !== 1 || (failed && !matching[0].failure)) return undefined;
      const request = matching[0];
      // ExtraInfo has no hop identity and the initial headers may be stale after a redirect.
      const indicators = request.redirected ? undefined : requestExtras.get(request.id) ?? request.indicators;
      const evidence = {state: 'matched', ...request.failure, ...indicators};
      if (request.responseStatus !== undefined) {
        return {...evidence, httpStatus: request.responseStatus, httpStatusSource: 'response_received'};
      }
      if (!request.redirected && request.extraCount === 1 && request.extraStatus !== undefined) {
        return {...evidence, httpStatus: request.extraStatus, httpStatusSource: 'extra_info'};
      }
      return {...evidence, httpStatusSource: 'not_observed'};
    },
  };
}

export async function navigateSourcePage(browser, page, url, timeoutMs, failureMessage, {collectRequestMetadata = false} = {}) {
  const headless = browser.backend === 'headless_worker';
  const deadline = Date.now() + timeoutMs;
  let observer;
  if (headless) {
    try { observer = observeMainDocument(page, url, collectRequestMetadata); } catch { /* Optional diagnostic. */ }
  }
  const unavailable = () => ({state: observer ? 'not_observed' : 'unavailable'});
  try {
    const navigation = await page.navigate(url, timeoutMs);
    if (!navigation.errorText) {
      if (headless && collectRequestMetadata) return observer?.documentFor(navigation, false) ?? unavailable();
      return;
    }
    const error = captureError('navigation_failed', failureMessage);
    if (headless) {
      const code = errorCode(navigation.errorText);
      let mainDocument = observer?.documentFor(navigation, true);
      const grace = Math.min(FAILURE_EVENT_GRACE_MS, deadline - Date.now());
      const needsStatus = code === HTTP_RESPONSE_FAILURE && mainDocument?.httpStatus === undefined;
      const needsRequestMetadata = collectRequestMetadata && code === HTTP_RESPONSE_FAILURE &&
        mainDocument?.requestMetadataSource !== 'request_extra_info';
      if (observer && (!mainDocument || needsStatus || needsRequestMetadata) &&
          validID(navigation.frameId) && validID(navigation.loaderId) && grace > 0) {
        // Failure and response events can trail the Page.navigate reply.
        await delay(grace);
        mainDocument = observer.documentFor(navigation, true);
      }
      error.cdpCommand = 'Page.navigate';
      error.chromeErrorCode = code;
      error.chromeErrorCategory = catalog.errorCategories[code] ?? 'other';
      error.mainDocument = mainDocument ?? unavailable();
    }
    throw error;
  } finally {
    try { observer?.off(); } catch { /* Preserve the navigation outcome. */ }
  }
}
