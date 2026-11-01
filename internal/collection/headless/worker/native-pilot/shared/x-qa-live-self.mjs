import { BOOTSTRAP_CURRENT_SELF_PROJECTION_SOURCE, validateBootstrapCurrentSelf } from './x-bootstrap-current-self.mjs';
import { QA_LOSSLESS_JSON_SOURCE } from './x-qa-lossless-json.mjs';

const MAX_BODY_BYTES = 512 * 1024;
const MAX_INITIALIZER_BYTES = 256 * 1024;
const MAX_DIAGNOSTIC_BYTES = 4_096;
const MAX_REPORTED_BODY_BYTES = MAX_BODY_BYTES + 1;
const MAX_REPORTED_INITIALIZER_BYTES = MAX_INITIALIZER_BYTES + 1;
const MAX_JSON_DEPTH = 24;
const MAX_JSON_VALUES = 16_384;
const MAX_DURATION_MS = 2_000;
const RESULT_SCHEMA = 'aku.x.qa-live-self.v4';
const RESULT_KEYS = [
  'schema',
  'outcome',
  'parserRejection',
  'identity',
  'bodyBytes',
  'initializerBytes',
  'status',
  'projection',
  'freshness',
  'canAuthorize',
  'nativeQualified',
  'sessionTransferred',
];
const OUTCOMES = [
  'candidate',
  'self_unqualified',
  'body_limit',
  'initializer_limit',
  'timeout',
  'fetch_failed',
  'response_rejected',
  'html_rejected',
  'source_absent',
  'source_ambiguous',
  'parser_rejected',
];

function strongCurrentSelf(projection) {
  const authValues = Object.values(projection.authFields);
  const entityValues = Object.values(projection.entityChecks);
  return projection.diagnostic === 'projected' &&
    projection.bootstrapState === 'object' &&
    projection.sessionState === 'object' &&
    projection.stableIdStringCandidates > 0 &&
    projection.sessionIdsAgree !== false &&
    ['absent', 'string'].includes(projection.idFields.user_id) &&
    ['absent', 'string'].includes(projection.idFields.user_id_str) &&
    authValues.every((value) => value === 'absent' || value === 'true') &&
    projection.entityChecks.normalizedUsers === 'match' &&
    !entityValues.some((value) => value === 'mismatch' || value === 'accessor' || value === 'invalid');
}

function buildQALiveSelfDiagnostic(outcome, bodyBytes, initializerBytes, status, projection, parserRejection = null) {
  const candidate = outcome === 'candidate';
  return {
    schema: 'aku.x.qa-live-self.v4',
    outcome,
    parserRejection,
    identity: candidate ? 'candidate' : 'unavailable',
    bodyBytes,
    initializerBytes,
    status,
    projection,
    freshness: candidate ? 'server-response-candidate' : 'unqualified',
    canAuthorize: false,
    nativeQualified: false,
    sessionTransferred: false,
  };
}

async function qaLiveSelfPageProbe(helpers) {
  const targetURL = 'https://x.com/home';
  const bodyLimit = 512 * 1024;
  const initializerLimit = 256 * 1024;
  const diagnosticLimit = 4_096;
  const timeoutLimit = 2_000;
  const parseDepth = 24;
  const parseValues = 16_384;
  const { losslessJSON, projectBootstrapCurrentSelf, strongCurrentSelf, buildDiagnostic } = helpers;

  let bodyBytes = 0;
  let initializerBytes = 0;
  let status = null;
  let responseBody = null;
  let reader = null;
  let controller = null;
  let timer = null;
  let settled = false;
  let startedAt = null;
  let lastClock = null;
  let initialLocation = null;
  let initialDocument = null;

  return await new Promise((resolve) => {
    const sameHomeDocument = () => {
      try {
        return initialLocation !== null && initialDocument !== null &&
          globalThis.location === initialLocation && initialLocation.href === targetURL &&
          globalThis.document === initialDocument;
      } catch {
        return false;
      }
    };

    const cancelReader = () => {
      if (reader === null) return;
      try { Promise.resolve(reader.cancel()).catch(() => {}); } catch {}
    };

    const cancelBody = () => {
      if (reader !== null) {
        cancelReader();
        return;
      }
      if (responseBody !== null && typeof responseBody.cancel === 'function') {
        try { Promise.resolve(responseBody.cancel()).catch(() => {}); } catch {}
      }
    };

    const boundedDiagnostic = (outcome, projection, parserRejection) => {
      let diagnostic;
      try {
        diagnostic = buildDiagnostic(outcome, bodyBytes, initializerBytes, status, projection, parserRejection);
        const serialized = JSON.stringify(diagnostic);
        if (new TextEncoder().encode(serialized).byteLength <= diagnosticLimit) return diagnostic;
      } catch {}
      return buildDiagnostic('parser_rejected', bodyBytes, initializerBytes, status, null, 'projection_failed');
    };

    const finish = (outcome, projection = null, parserRejection = null) => {
      if (settled) return;
      let finalOutcome = outcome;
      let finalProjection = projection;
      if (initialLocation !== null && !sameHomeDocument()) {
        finalOutcome = 'response_rejected';
        finalProjection = null;
        initializerBytes = 0;
        parserRejection = null;
      }
      const diagnostic = boundedDiagnostic(finalOutcome, finalProjection, parserRejection);
      settled = true;
      if (diagnostic.outcome !== 'candidate') {
        try { controller?.abort(); } catch {}
        cancelBody();
      }
      if (timer !== null) {
        try { globalThis.clearTimeout(timer); } catch {}
      }
      resolve(diagnostic);
    };

    const withinDeadline = () => {
      let now;
      try { now = globalThis.performance.now(); } catch {
        finish('timeout');
        return false;
      }
      if (
        !Number.isFinite(now) || startedAt === null ||
        (lastClock !== null && now < lastClock) ||
        now - startedAt >= timeoutLimit
      ) {
        finish('timeout');
        return false;
      }
      lastClock = now;
      return true;
    };

    const timeout = () => finish('timeout');

    try {
      initialLocation = globalThis.location;
      initialDocument = globalThis.document;
      if (
        initialLocation === null || typeof initialLocation !== 'object' ||
        initialLocation.href !== targetURL ||
        initialDocument === null || typeof initialDocument !== 'object'
      ) {
        finish('response_rejected');
        return;
      }

      try { startedAt = globalThis.performance.now(); } catch {
        finish('timeout');
        return;
      }
      if (!Number.isFinite(startedAt) || startedAt < 0) {
        finish('timeout');
        return;
      }
      lastClock = startedAt;
      controller = new AbortController();
      timer = globalThis.setTimeout(timeout, timeoutLimit);

      const run = async () => {
        try {
          const response = await globalThis.fetch(targetURL, {
            method: 'GET',
            credentials: 'same-origin',
            cache: 'no-store',
            redirect: 'error',
            signal: controller.signal,
          });

          if (response !== null && typeof response === 'object') {
            try { responseBody = response.body ?? null; } catch {
              if (settled) return;
              finish('response_rejected');
              return;
            }
          }
          if (settled) {
            cancelBody();
            return;
          }
          if (!withinDeadline()) return;
          if (!sameHomeDocument()) {
            finish('response_rejected');
            return;
          }

          if (Number.isInteger(response?.status) && response.status >= 100 && response.status <= 599) {
            status = response.status;
          }
          if (
            response === null || typeof response !== 'object' ||
            response.type !== 'basic' || response.redirected !== false ||
            response.url !== targetURL || response.status !== 200 ||
            response.headers === null || typeof response.headers !== 'object'
          ) {
            finish('response_rejected');
            return;
          }

          let contentType;
          try { contentType = response.headers.get('content-type'); } catch {
            finish('response_rejected');
            return;
          }
          if (
            typeof contentType !== 'string' ||
            contentType.split(';', 1)[0].trim().toLowerCase() !== 'text/html'
          ) {
            finish('response_rejected');
            return;
          }

          if (responseBody === null || typeof responseBody !== 'object' || typeof responseBody.getReader !== 'function') {
            finish('response_rejected');
            return;
          }
          reader = responseBody.getReader({ mode: 'byob' });
          if (reader === null || typeof reader !== 'object' || typeof reader.read !== 'function') {
            finish('response_rejected');
            return;
          }

          const chunks = [];
          let nonemptyChunks = 0;
          let zeroChunks = 0;
          while (true) {
            const viewLength = Math.min(4_096, bodyLimit + 1 - bodyBytes);
            if (viewLength <= 0) {
              finish('body_limit');
              return;
            }
            const view = new Uint8Array(viewLength);
            const requestedLength = view.byteLength;
            const step = await reader.read(view);
            if (settled) return;
            if (step === null || typeof step !== 'object') {
              finish('fetch_failed');
              return;
            }
            const chunk = step.value;
            if (step.done === true) {
              if (chunk !== undefined && (!(chunk instanceof Uint8Array) || chunk.byteLength !== 0)) {
                finish('fetch_failed');
                return;
              }
              if (!withinDeadline()) return;
              if (!sameHomeDocument()) {
                finish('response_rejected');
                return;
              }
              break;
            }
            if (!(chunk instanceof Uint8Array) || chunk.byteLength > requestedLength) {
              finish('fetch_failed');
              return;
            }
            if (chunk.byteLength === 0) {
              zeroChunks += 1;
              if (zeroChunks > 256) finish('fetch_failed');
              if (settled) return;
              if (!withinDeadline()) return;
              if (!sameHomeDocument()) {
                finish('response_rejected');
                return;
              }
              continue;
            }
            nonemptyChunks += 1;
            const copy = new Uint8Array(chunk.byteLength);
            copy.set(chunk);
            bodyBytes += copy.byteLength;
            if (!withinDeadline()) return;
            if (!sameHomeDocument()) {
              finish('response_rejected');
              return;
            }
            if (bodyBytes > bodyLimit) {
              finish('body_limit');
              return;
            }
            if (nonemptyChunks > 256) {
              finish('fetch_failed');
              return;
            }
            chunks.push(copy);
          }

          if (!sameHomeDocument()) {
            finish('response_rejected');
            return;
          }

          const bytes = new Uint8Array(bodyBytes);
          let offset = 0;
          for (const chunk of chunks) {
            bytes.set(chunk, offset);
            offset += chunk.byteLength;
          }

          if (!withinDeadline()) return;
          let html;
          try { html = new TextDecoder('utf-8', { fatal: true }).decode(bytes); } catch {
            if (!withinDeadline()) return;
            finish('html_rejected');
            return;
          }
          if (!withinDeadline()) return;

          if (!withinDeadline()) return;
          let markerCount = 0;
          const candidateJSON = [];
          try {
            const template = initialDocument.createElement('template');
            template.innerHTML = html;
            if (!withinDeadline()) return;
            const templateContent = template?.content;
            if (templateContent === null || typeof templateContent !== 'object' || typeof templateContent.querySelectorAll !== 'function') {
              finish('html_rejected');
              return;
            }

            if (!withinDeadline()) return;
            const scripts = templateContent.querySelectorAll('script');
            if (scripts === null || typeof scripts !== 'object' || !Number.isSafeInteger(scripts.length) || scripts.length < 0) {
              finish('html_rejected');
              return;
            }

            const marker = 'window.__INITIAL_STATE__';
            for (let index = 0; index < scripts.length; index += 1) {
              const script = scripts[index];
              if (script === null || typeof script !== 'object') {
                finish('html_rejected');
                return;
              }
              const scriptText = script.textContent;
              if (typeof scriptText !== 'string') {
                finish('html_rejected');
                return;
              }
              let searchFrom = 0;
              while (true) {
                const markerIndex = scriptText.indexOf(marker, searchFrom);
                if (markerIndex < 0) break;
                markerCount += 1;
                searchFrom = markerIndex + marker.length;
                if (markerCount > 1) break;
              }
              const trimmedScript = scriptText.trim();
              if (trimmedScript.startsWith(marker)) {
                const assignment = trimmedScript.slice(marker.length).trimStart();
                if (assignment.startsWith('=')) {
                  const rightHandSide = assignment.slice(1).trim();
                  if (rightHandSide.endsWith(';')) {
                    candidateJSON.push(rightHandSide.slice(0, -1).trim());
                  }
                }
              }
              if (markerCount > 1 || candidateJSON.length > 1) break;
            }
          } catch {
            if (!withinDeadline()) return;
            finish('html_rejected');
            return;
          }
          if (!withinDeadline()) return;
          if (markerCount > 1 || candidateJSON.length > 1) {
            finish('source_ambiguous');
            return;
          }
          if (candidateJSON.length === 0) {
            finish('source_absent');
            return;
          }
          if (markerCount !== 1 || candidateJSON.length !== 1) {
            finish('source_ambiguous');
            return;
          }

          const sourceJSON = candidateJSON[0];
          let sourceLength;
          try { sourceLength = new TextEncoder().encode(sourceJSON).byteLength; } catch {
            finish('parser_rejected', null, 'parser_internal');
            return;
          }
          initializerBytes = sourceLength > initializerLimit ? initializerLimit + 1 : sourceLength;
          if (!withinDeadline()) return;
          if (sourceLength > initializerLimit) {
            finish('initializer_limit');
            return;
          }

          if (!withinDeadline()) return;
          let parsed;
          let parseFailed = false;
          let parserRejection = 'parser_internal';
          try {
            parsed = losslessJSON.parse(sourceJSON, {
              maxJSONDepth: parseDepth,
              maxJSONTokens: parseValues,
            }, (reason) => {
              if (['malformed_json', 'duplicate_key', 'invalid_unicode', 'depth_limit', 'value_limit'].includes(reason)) {
                parserRejection = reason;
              }
            });
          } catch {
            parseFailed = true;
          }
          if (!withinDeadline()) return;
          if (parseFailed) {
            finish('parser_rejected', null, parserRejection);
            return;
          }

          if (!withinDeadline()) return;
          let projection;
          let projectionFailed = false;
          try {
            projection = projectBootstrapCurrentSelf({
              location: { origin: 'https://x.com', pathname: '/home' },
              __INITIAL_STATE__: losslessJSON.forProjection(parsed),
            });
          } catch {
            projectionFailed = true;
          }
          if (!withinDeadline()) return;
          if (projectionFailed) {
            finish('parser_rejected', null, 'projection_failed');
            return;
          }

          finish(strongCurrentSelf(projection) ? 'candidate' : 'self_unqualified', projection);
        } catch {
          finish('fetch_failed');
        }
      };

      void run();
    } catch {
      finish('fetch_failed');
    }
  });
}

export const QA_LIVE_SELF_EXPRESSION = `(${qaLiveSelfPageProbe.toString()})({
  losslessJSON: ${QA_LOSSLESS_JSON_SOURCE},
  projectBootstrapCurrentSelf: ${BOOTSTRAP_CURRENT_SELF_PROJECTION_SOURCE},
  strongCurrentSelf: (${strongCurrentSelf.toString()}),
  buildDiagnostic: (${buildQALiveSelfDiagnostic.toString()}),
})`;

function invalidReport() {
  throw new TypeError('qa_live_self_report_invalid');
}

function exactDataRecord(value, expectedKeys) {
  if (value === null || typeof value !== 'object' || Array.isArray(value)) invalidReport();
  const prototype = Object.getPrototypeOf(value);
  if (prototype !== null && (
    typeof prototype !== 'object' || Array.isArray(prototype) || Object.getPrototypeOf(prototype) !== null
  )) invalidReport();
  const ownKeys = Reflect.ownKeys(value);
  if (
    ownKeys.length !== expectedKeys.length ||
    ownKeys.some((key) => typeof key !== 'string' || !expectedKeys.includes(key))
  ) {
    invalidReport();
  }
  const output = Object.create(null);
  for (const key of expectedKeys) {
    const descriptor = Object.getOwnPropertyDescriptor(value, key);
    if (descriptor === undefined || !Object.hasOwn(descriptor, 'value') || descriptor.enumerable !== true) {
      invalidReport();
    }
    output[key] = descriptor.value;
  }
  return output;
}

function validStatus(value) {
  return value === null || (Number.isInteger(value) && value >= 100 && value <= 599);
}

export function validateQALiveSelfDiagnostic(value) {
  try {
    const result = exactDataRecord(value, RESULT_KEYS);
    if (result.schema !== RESULT_SCHEMA || !OUTCOMES.includes(result.outcome)) invalidReport();
    if (result.outcome === 'parser_rejected') {
      if (!['malformed_json', 'duplicate_key', 'invalid_unicode', 'depth_limit', 'value_limit', 'parser_internal', 'projection_failed'].includes(result.parserRejection)) invalidReport();
    } else if (result.parserRejection !== null) invalidReport();
    if (!['candidate', 'unavailable'].includes(result.identity)) invalidReport();
    if (!Number.isInteger(result.bodyBytes) || result.bodyBytes < 0 || result.bodyBytes > MAX_REPORTED_BODY_BYTES) invalidReport();
    if (!Number.isInteger(result.initializerBytes) || result.initializerBytes < 0 ||
      result.initializerBytes > MAX_REPORTED_INITIALIZER_BYTES) invalidReport();
    if (result.initializerBytes > result.bodyBytes) invalidReport();
    if (!validStatus(result.status)) invalidReport();
    if (result.projection !== null) result.projection = validateBootstrapCurrentSelf(result.projection);
    if (!['server-response-candidate', 'unqualified'].includes(result.freshness)) invalidReport();
    if (result.canAuthorize !== false || result.nativeQualified !== false || result.sessionTransferred !== false) invalidReport();

    const candidate = result.outcome === 'candidate';
    if (candidate) {
      if (
        result.identity !== 'candidate' || result.freshness !== 'server-response-candidate' ||
        result.status !== 200 || result.bodyBytes > MAX_BODY_BYTES ||
        result.initializerBytes < 1 || result.initializerBytes > MAX_INITIALIZER_BYTES ||
        result.projection === null || !strongCurrentSelf(result.projection)
      ) invalidReport();
    } else {
      if (result.identity !== 'unavailable' || result.freshness !== 'unqualified') invalidReport();
      if (result.outcome === 'body_limit' &&
        (result.status !== 200 || result.bodyBytes !== MAX_REPORTED_BODY_BYTES ||
         result.initializerBytes !== 0 || result.projection !== null)) invalidReport();
      if (result.outcome === 'initializer_limit' &&
        (result.status !== 200 || result.bodyBytes > MAX_BODY_BYTES ||
         result.initializerBytes !== MAX_REPORTED_INITIALIZER_BYTES || result.projection !== null)) invalidReport();
      if (['source_absent', 'source_ambiguous'].includes(result.outcome) &&
        (result.status !== 200 || result.bodyBytes > MAX_BODY_BYTES ||
         result.initializerBytes !== 0 || result.projection !== null)) invalidReport();
      if (result.outcome === 'html_rejected' &&
        (result.status !== 200 || result.bodyBytes > MAX_BODY_BYTES ||
         result.initializerBytes !== 0 || result.projection !== null)) invalidReport();
      if (result.outcome === 'self_unqualified' &&
        (result.status !== 200 || result.bodyBytes > MAX_BODY_BYTES ||
         result.initializerBytes < 1 || result.initializerBytes > MAX_INITIALIZER_BYTES || result.projection === null ||
         strongCurrentSelf(result.projection))) invalidReport();
      if (result.outcome === 'parser_rejected' &&
        (result.status !== 200 || result.bodyBytes > MAX_BODY_BYTES ||
         result.initializerBytes > MAX_INITIALIZER_BYTES || result.projection !== null)) invalidReport();
      if (result.outcome === 'response_rejected' &&
        (result.initializerBytes !== 0 || result.projection !== null)) invalidReport();
      if (['timeout', 'fetch_failed'].includes(result.outcome) && result.projection !== null) invalidReport();
      if (result.outcome === 'fetch_failed' && result.initializerBytes !== 0) invalidReport();
      if (result.outcome === 'timeout' && result.bodyBytes === MAX_REPORTED_BODY_BYTES &&
        (result.status !== 200 || result.initializerBytes !== 0)) invalidReport();
      if (result.outcome === 'timeout' && result.initializerBytes === MAX_REPORTED_INITIALIZER_BYTES &&
        (result.status !== 200 || result.bodyBytes > MAX_BODY_BYTES)) invalidReport();
      if (!['self_unqualified', 'parser_rejected'].includes(result.outcome) &&
        ['body_limit', 'initializer_limit', 'source_absent', 'source_ambiguous', 'html_rejected', 'response_rejected', 'timeout', 'fetch_failed'].includes(result.outcome) &&
        result.projection !== null) invalidReport();
    }

    const sanitized = {
      schema: RESULT_SCHEMA,
      outcome: result.outcome,
      parserRejection: result.parserRejection,
      identity: result.identity,
      bodyBytes: result.bodyBytes,
      initializerBytes: result.initializerBytes,
      status: result.status,
      projection: result.projection,
      freshness: result.freshness,
      canAuthorize: false,
      nativeQualified: false,
      sessionTransferred: false,
    };
    if (Buffer.byteLength(JSON.stringify(sanitized), 'utf8') > MAX_DIAGNOSTIC_BYTES) invalidReport();
    return sanitized;
  } catch {
    return invalidReport();
  }
}

// Only the closed v4 DTO is accepted. Raw source and historical reports fail.
export function validateQALiveSelfReport(value) {
  return validateQALiveSelfDiagnostic(value);
}

export function projectQALiveSelf(privateDiagnostic) {
  return validateQALiveSelfDiagnostic(privateDiagnostic);
}
