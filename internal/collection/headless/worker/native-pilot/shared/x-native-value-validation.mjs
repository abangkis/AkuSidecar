

// Closed value, identifier, key and control-origin validation.
function probeError(code) {
  return new Error(`xnp_${code}`);
}

function isObject(value) {
  return !!value && typeof value === 'object' && !Array.isArray(value);
}

function own(value, key) {
  return Object.prototype.hasOwnProperty.call(value, key);
}

function safeId(value, max = 256) {
  return typeof value === 'string' && value.length > 0 && value.length <= max &&
    !/[\u0000-\u001f\u007f]/u.test(value);
}

function exactKeys(value, expected, code) {
  if (!isObject(value)) throw probeError(code);
  const keys = Object.keys(value);
  if (keys.length !== expected.length || keys.some(key => !expected.includes(key))) throw probeError(code);
}

function parseControlOrigin(value) {
  if (typeof value !== 'string' || value.length > 128) throw probeError('origin_invalid');
  let parsed;
  try { parsed = new URL(value); } catch { throw probeError('origin_invalid'); }
  if (parsed.protocol !== 'http:' || parsed.hostname !== '127.0.0.1' || !parsed.port ||
      parsed.username || parsed.password || parsed.pathname !== '/' || parsed.search || parsed.hash ||
      parsed.origin !== value) throw probeError('origin_invalid');
  return parsed.origin;
}

function exactCDPKeys(message, keys) {
  const actual = Object.keys(message);
  if (actual.length !== keys.length || actual.some(key => !keys.includes(key))) throw probeError('cdp_message_invalid');
}

export {probeError, isObject, own, safeId, exactKeys, parseControlOrigin, exactCDPKeys};
