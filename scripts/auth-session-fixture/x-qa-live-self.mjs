import { parseStrictJSON } from './x-evidence-ingress.mjs';
import {
  inspectBootstrapCurrentSelf,
  validateBootstrapCurrentSelf,
} from './x-bootstrap-current-self.mjs';

const TARGET_URL = 'https://x.com/home';
const MAX_BODY_BYTES = 16_384;
const MAX_REPORTED_BYTES = MAX_BODY_BYTES + 1;
const PAGE_SCHEMA = 'aku.x.qa-live-self-page.v1';
const RESULT_SCHEMA = 'aku.x.qa-live-self.v1';
const PAGE_CATEGORIES = [
  'fetched',
  'body_limit',
  'timeout',
  'fetch_failed',
  'response_rejected',
  'source_absent',
  'source_ambiguous',
];
const OUTCOMES = [
  'candidate',
  'self_unqualified',
  'body_limit',
  'timeout',
  'fetch_failed',
  'response_rejected',
  'source_absent',
  'source_ambiguous',
  'projection_invalid',
];
const RESULT_KEYS = [
  'schema',
  'outcome',
  'identity',
  'bodyBytes',
  'status',
  'projection',
  'freshness',
  'canAuthorize',
  'nativeQualified',
  'sessionTransferred',
];
const PAGE_KEYS = ['schema', 'category', 'status', 'bytesRead', 'sourceJSON'];

async function qaLiveSelfPageProbe() {
  const targetURL = 'https://x.com/home';
  const bodyLimit = 16_384;
  const resultSchema = 'aku.x.qa-live-self-page.v1';
  let status = null;
  let bytesRead = 0;
  let reader = null;
  let responseBody = null;
  let controller = null;
  let timer = null;
  let settled = false;

  return await new Promise((resolve) => {
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

    const finish = (category, sourceJSON = null) => {
      if (settled) return;
      settled = true;
      if (category !== 'fetched') {
        try { controller?.abort(); } catch {}
        cancelBody();
      }
      if (timer !== null) {
        try { globalThis.clearTimeout(timer); } catch {}
      }
      resolve({
        schema: resultSchema,
        category,
        status,
        bytesRead,
        sourceJSON,
      });
    };

    const timeout = () => {
      finish('timeout');
    };

    try {
      const initialLocation = globalThis.location;
      const initialDocument = globalThis.document;
      if (
        initialLocation === null || typeof initialLocation !== 'object' ||
        initialLocation.href !== targetURL ||
        initialDocument === null || typeof initialDocument !== 'object'
      ) {
        finish('response_rejected');
        return;
      }

      controller = new AbortController();
      timer = globalThis.setTimeout(timeout, 2_000);

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
            const viewLength = Math.min(4_096, bodyLimit + 1 - bytesRead);
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
              continue;
            }
            nonemptyChunks += 1;
            if (nonemptyChunks > 256) {
              finish('fetch_failed');
              return;
            }
            const copy = new Uint8Array(chunk.byteLength);
            copy.set(chunk);
            chunks.push(copy);
            bytesRead += copy.byteLength;
            if (bytesRead > bodyLimit) {
              finish('body_limit');
              return;
            }
          }

          if (
            globalThis.location !== initialLocation ||
            initialLocation.href !== targetURL ||
            globalThis.document !== initialDocument
          ) {
            finish('response_rejected');
            return;
          }

          const bytes = new Uint8Array(bytesRead);
          let offset = 0;
          for (const chunk of chunks) {
            bytes.set(chunk, offset);
            offset += chunk.byteLength;
          }

          let html;
          try { html = new TextDecoder('utf-8', { fatal: true }).decode(bytes); } catch {
            finish('source_absent');
            return;
          }

          let template;
          try {
            template = initialDocument.createElement('template');
            template.innerHTML = html;
          } catch {
            finish('source_absent');
            return;
          }
          const templateContent = template?.content;
          if (templateContent === null || typeof templateContent !== 'object' || typeof templateContent.querySelectorAll !== 'function') {
            finish('source_absent');
            return;
          }

          let scripts;
          try { scripts = templateContent.querySelectorAll('script'); } catch {
            finish('source_absent');
            return;
          }
          if (scripts === null || typeof scripts !== 'object' || !Number.isSafeInteger(scripts.length) || scripts.length < 0) {
            finish('source_absent');
            return;
          }

          const marker = 'window.__INITIAL_STATE__';
          const initializer = /^\s*window\.__INITIAL_STATE__\s*=\s*([\s\S]*?)\s*;\s*$/;
          let markerCount = 0;
          const candidateJSON = [];
          for (let index = 0; index < scripts.length; index += 1) {
            const script = scripts[index];
            if (script === null || typeof script !== 'object') {
              finish('source_absent');
              return;
            }
            const scriptText = script.textContent;
            if (typeof scriptText !== 'string') {
              finish('source_absent');
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
            const match = initializer.exec(scriptText);
            if (match !== null) candidateJSON.push(match[1]);
            if (markerCount > 1 || candidateJSON.length > 1) {
              finish('source_ambiguous');
              return;
            }
          }

          if (markerCount > 1 || candidateJSON.length > 1) {
            finish('source_ambiguous');
            return;
          }
          if (markerCount !== 1 || candidateJSON.length !== 1) {
            finish(markerCount === 0 ? 'source_absent' : 'source_ambiguous');
            return;
          }

          const sourceJSON = candidateJSON[0];
          try { JSON.parse(sourceJSON); } catch {
            finish('source_absent');
            return;
          }
          finish('fetched', sourceJSON);
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

export const QA_LIVE_SELF_EXPRESSION = `(${qaLiveSelfPageProbe.toString()})()`;

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

function utf8Length(value) {
  try { return Buffer.byteLength(value, 'utf8'); } catch { invalidReport(); }
}

export function validateQALiveSelfReport(value) {
  try {
    const report = exactDataRecord(value, PAGE_KEYS);
    if (report.schema !== PAGE_SCHEMA || !PAGE_CATEGORIES.includes(report.category)) invalidReport();
    if (!validStatus(report.status)) invalidReport();
    if (!Number.isInteger(report.bytesRead) || report.bytesRead < 0 || report.bytesRead > MAX_REPORTED_BYTES) {
      invalidReport();
    }
    if (report.sourceJSON !== null &&
      (typeof report.sourceJSON !== 'string' || report.sourceJSON.length === 0 ||
       report.sourceJSON.length > MAX_BODY_BYTES || utf8Length(report.sourceJSON) > MAX_BODY_BYTES)) {
      invalidReport();
    }

    if (report.category === 'fetched') {
      if (
        report.status !== 200 || report.bytesRead > MAX_BODY_BYTES || report.sourceJSON === null ||
        utf8Length(report.sourceJSON) > report.bytesRead
      ) invalidReport();
    } else if (report.sourceJSON !== null) {
      invalidReport();
    }
    if (report.category === 'body_limit' &&
      (report.status !== 200 || report.bytesRead !== MAX_REPORTED_BYTES)) invalidReport();
    if (['source_absent', 'source_ambiguous'].includes(report.category) &&
      (report.status !== 200 || report.bytesRead > MAX_BODY_BYTES)) invalidReport();

    return {
      schema: PAGE_SCHEMA,
      category: report.category,
      status: report.status,
      bytesRead: report.bytesRead,
      sourceJSON: report.sourceJSON,
    };
  } catch {
    return invalidReport();
  }
}

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

function diagnosticResult({ outcome, bodyBytes, status, projection = null }) {
  const candidate = outcome === 'candidate';
  return {
    schema: RESULT_SCHEMA,
    outcome,
    identity: candidate ? 'candidate' : 'unavailable',
    bodyBytes,
    status,
    projection,
    freshness: candidate ? 'server-response-candidate' : 'unqualified',
    canAuthorize: false,
    nativeQualified: false,
    sessionTransferred: false,
  };
}

export function validateQALiveSelfDiagnostic(value) {
  try {
    const result = exactDataRecord(value, RESULT_KEYS);
    if (result.schema !== RESULT_SCHEMA || !OUTCOMES.includes(result.outcome)) invalidReport();
    if (!['candidate', 'unavailable'].includes(result.identity)) invalidReport();
    if (!Number.isInteger(result.bodyBytes) || result.bodyBytes < 0 || result.bodyBytes > MAX_REPORTED_BYTES) invalidReport();
    if (!validStatus(result.status)) invalidReport();
    if (result.projection !== null) result.projection = validateBootstrapCurrentSelf(result.projection);
    if (!['server-response-candidate', 'unqualified'].includes(result.freshness)) invalidReport();
    if (result.canAuthorize !== false || result.nativeQualified !== false || result.sessionTransferred !== false) invalidReport();

    const candidate = result.outcome === 'candidate';
    if (candidate) {
      if (
        result.identity !== 'candidate' || result.freshness !== 'server-response-candidate' ||
        result.status !== 200 || result.bodyBytes > MAX_BODY_BYTES ||
        result.projection === null || !strongCurrentSelf(result.projection)
      ) invalidReport();
    } else {
      if (result.identity !== 'unavailable' || result.freshness !== 'unqualified') invalidReport();
      if (result.outcome === 'self_unqualified' &&
        (result.status !== 200 || result.bodyBytes > MAX_BODY_BYTES ||
         result.projection === null || strongCurrentSelf(result.projection))) invalidReport();
      if (result.outcome === 'projection_invalid' &&
        (result.status !== 200 || result.bodyBytes > MAX_BODY_BYTES || result.projection !== null)) invalidReport();
      if (result.outcome === 'body_limit' &&
        (result.status !== 200 || result.bodyBytes !== MAX_REPORTED_BYTES || result.projection !== null)) invalidReport();
      if (['source_absent', 'source_ambiguous'].includes(result.outcome) &&
        (result.status !== 200 || result.bodyBytes > MAX_BODY_BYTES || result.projection !== null)) invalidReport();
      if (!['self_unqualified', 'projection_invalid'].includes(result.outcome) && result.projection !== null) {
        invalidReport();
      }
    }

    const sanitized = {
      schema: RESULT_SCHEMA,
      outcome: result.outcome,
      identity: result.identity,
      bodyBytes: result.bodyBytes,
      status: result.status,
      projection: result.projection,
      freshness: result.freshness,
      canAuthorize: false,
      nativeQualified: false,
      sessionTransferred: false,
    };
    if (Buffer.byteLength(JSON.stringify(sanitized), 'utf8') > 4096) invalidReport();
    return sanitized;
  } catch {
    return invalidReport();
  }
}

export function projectQALiveSelf(privatePageResult) {
  const report = validateQALiveSelfReport(privatePageResult);
  if (report.category !== 'fetched') {
    return validateQALiveSelfDiagnostic(diagnosticResult({
      outcome: report.category,
      bodyBytes: report.bytesRead,
      status: report.status,
    }));
  }

  let projection;
  try {
    const parsed = parseStrictJSON(report.sourceJSON, { maxJSONDepth: 24, maxJSONTokens: 4_096 });
    projection = validateBootstrapCurrentSelf(inspectBootstrapCurrentSelf({
      location: { origin: 'https://x.com', pathname: '/home' },
      __INITIAL_STATE__: parsed,
    }));
  } catch {
    return validateQALiveSelfDiagnostic(diagnosticResult({
      outcome: 'projection_invalid',
      bodyBytes: report.bytesRead,
      status: report.status,
    }));
  }

  return validateQALiveSelfDiagnostic(diagnosticResult({
    outcome: strongCurrentSelf(projection) ? 'candidate' : 'self_unqualified',
    bodyBytes: report.bytesRead,
    status: report.status,
    projection,
  }));
}
