// Test-only strict JSON parser that preserves numeric source lexemes without
// converting them to JavaScript numbers or exposing them to the projection.
export function createQALosslessJSON() {
  const HARD_LIMITS = Object.freeze({
    maxJSONDepth: 24,
    maxJSONTokens: 16_384,
    maxInputBytes: 262_144,
  });
  const REASONS = new Set([
    'malformed_json',
    'duplicate_key',
    'invalid_unicode',
    'depth_limit',
    'value_limit',
    'initializer_limit',
  ]);
  const lexemes = new WeakMap();
  const containers = new WeakSet();
  const projectedNumber = Symbol();

  const fixedParseError = () => new SyntaxError('invalid_json');
  const fixedProjectionError = () => new TypeError('invalid_projection_input');
  const fixedNumberTokenError = () => new TypeError('invalid_number_token');

  function validUnicode(value) {
    for (let index = 0; index < value.length; index += 1) {
      const code = value.charCodeAt(index);
      if (code >= 0xd800 && code <= 0xdbff) {
        const next = value.charCodeAt(index + 1);
        if (!(next >= 0xdc00 && next <= 0xdfff)) return false;
        index += 1;
      } else if (code >= 0xdc00 && code <= 0xdfff) {
        return false;
      }
    }
    return true;
  }

  function utf8ByteLength(value, ceiling) {
    let bytes = 0;
    for (let index = 0; index < value.length; index += 1) {
      const code = value.charCodeAt(index);
      if (code <= 0x7f) bytes += 1;
      else if (code <= 0x7ff) bytes += 2;
      else if (code >= 0xd800 && code <= 0xdbff &&
          value.charCodeAt(index + 1) >= 0xdc00 && value.charCodeAt(index + 1) <= 0xdfff) {
        bytes += 4;
        index += 1;
      } else bytes += 3;
      if (bytes > ceiling) return bytes;
    }
    return bytes;
  }

  function normalizeLimits(input, fail) {
    const result = {
      maxJSONDepth: HARD_LIMITS.maxJSONDepth,
      maxJSONTokens: HARD_LIMITS.maxJSONTokens,
      maxInputBytes: HARD_LIMITS.maxInputBytes,
    };
    if (input === undefined) return result;
    if (input === null || typeof input !== 'object' || Array.isArray(input)) fail('initializer_limit');

    let prototype;
    let keys;
    try {
      prototype = Object.getPrototypeOf(input);
      keys = Reflect.ownKeys(input);
    } catch {
      fail('initializer_limit');
    }
    if ((prototype !== null && prototype !== Object.prototype) || keys.length > 3) fail('initializer_limit');

    for (const key of keys) {
      if (typeof key !== 'string' || !Object.prototype.hasOwnProperty.call(result, key)) fail('initializer_limit');
      let descriptor;
      try {
        descriptor = Object.getOwnPropertyDescriptor(input, key);
      } catch {
        fail('initializer_limit');
      }
      if (descriptor === undefined || Object.getOwnPropertyDescriptor(descriptor, 'value') === undefined) {
        fail('initializer_limit');
      }
      const value = descriptor.value;
      if (typeof value !== 'number' || !Number.isSafeInteger(value)) fail('initializer_limit');
      const minimum = key === 'maxJSONDepth' ? 0 : 1;
      if (value < minimum || value > HARD_LIMITS[key]) fail('initializer_limit');
      result[key] = value;
    }
    return result;
  }

  function parse(text, limitsInput = undefined, onReject = undefined) {
    let callback = typeof onReject === 'function' ? onReject : null;
    let rejectionSent = false;
    const fail = (reason = 'malformed_json') => {
      const safeReason = REASONS.has(reason) ? reason : 'malformed_json';
      if (!rejectionSent) {
        rejectionSent = true;
        if (callback) {
          try { callback(safeReason); } catch { /* Do not expose callback errors. */ }
        }
      }
      throw fixedParseError();
    };

    try {
      if (onReject !== undefined && typeof onReject !== 'function') fail('initializer_limit');
      if (typeof text !== 'string') fail('initializer_limit');
      const limits = normalizeLimits(limitsInput, fail);
      if (utf8ByteLength(text, limits.maxInputBytes) > limits.maxInputBytes) fail('initializer_limit');

      let index = 0;
      let values = 0;

      const whitespace = () => {
        while (index < text.length &&
            (text[index] === ' ' || text[index] === '\t' || text[index] === '\r' || text[index] === '\n')) {
          index += 1;
        }
      };

      const string = () => {
        if (text[index] !== '"') fail();
        const start = index;
        index += 1;
        while (index < text.length) {
          const code = text.charCodeAt(index);
          if (code === 0x22) {
            index += 1;
            let value;
            try {
              value = JSON.parse(text.slice(start, index));
            } catch {
              fail();
            }
            if (typeof value !== 'string' || !validUnicode(value)) fail('invalid_unicode');
            return value;
          }
          if (code < 0x20) fail();
          if (code === 0x5c) {
            index += 1;
            if (index >= text.length || !'"\\/bfnrtu'.includes(text[index])) fail();
            if (text[index] === 'u') {
              for (let offset = 1; offset <= 4; offset += 1) {
                const digit = text[index + offset];
                if (digit === undefined || !/[0-9a-f]/iu.test(digit)) fail();
              }
              index += 5;
            } else {
              index += 1;
            }
          } else {
            index += 1;
          }
        }
        fail();
      };

      const numberToken = () => {
        const start = index;
        if (text[index] === '-') index += 1;
        if (text[index] === '0') {
          index += 1;
        } else {
          if (!(text[index] >= '1' && text[index] <= '9')) fail();
          while (text[index] >= '0' && text[index] <= '9') index += 1;
        }
        if (text[index] === '.') {
          index += 1;
          if (!(text[index] >= '0' && text[index] <= '9')) fail();
          while (text[index] >= '0' && text[index] <= '9') index += 1;
        }
        if (text[index] === 'e' || text[index] === 'E') {
          index += 1;
          if (text[index] === '+' || text[index] === '-') index += 1;
          if (!(text[index] >= '0' && text[index] <= '9')) fail();
          while (text[index] >= '0' && text[index] <= '9') index += 1;
        }
        const token = Object.freeze(Object.create(null));
        lexemes.set(token, text.slice(start, index));
        return token;
      };

      const value = (depth) => {
        whitespace();
        if (depth > limits.maxJSONDepth) fail('depth_limit');
        values += 1;
        if (values > limits.maxJSONTokens) fail('value_limit');
        const first = text[index];
        if (first === '"') return string();
        if (first === '{') {
          index += 1;
          whitespace();
          const output = Object.create(null);
          containers.add(output);
          const keys = new Set();
          if (text[index] === '}') {
            index += 1;
            return output;
          }
          while (index < text.length) {
            whitespace();
            const key = string();
            if (keys.has(key)) fail('duplicate_key');
            keys.add(key);
            whitespace();
            if (text[index] !== ':') fail();
            index += 1;
            output[key] = value(depth + 1);
            whitespace();
            if (text[index] === '}') {
              index += 1;
              return output;
            }
            if (text[index] !== ',') fail();
            index += 1;
          }
          fail();
        }
        if (first === '[') {
          index += 1;
          whitespace();
          const output = [];
          containers.add(output);
          if (text[index] === ']') {
            index += 1;
            return output;
          }
          while (index < text.length) {
            output.push(value(depth + 1));
            whitespace();
            if (text[index] === ']') {
              index += 1;
              return output;
            }
            if (text[index] !== ',') fail();
            index += 1;
          }
          fail();
        }
        if (text.startsWith('true', index)) {
          index += 4;
          return true;
        }
        if (text.startsWith('false', index)) {
          index += 5;
          return false;
        }
        if (text.startsWith('null', index)) {
          index += 4;
          return null;
        }
        return numberToken();
      };

      const parsed = value(0);
      whitespace();
      if (index !== text.length) fail();
      return parsed;
    } catch {
      if (rejectionSent) {
        throw fixedParseError();
      }
      return fail('malformed_json');
    }
  }

  function isNumberToken(value) {
    return value !== null && (typeof value === 'object' || typeof value === 'function') && lexemes.has(value);
  }

  function numberLexeme(value) {
    if (!isNumberToken(value)) throw fixedNumberTokenError();
    return lexemes.get(value);
  }

  function forProjection(input) {
    const seen = new WeakSet();
    let values = 0;

    const copy = (value, depth) => {
      if (depth > HARD_LIMITS.maxJSONDepth) throw fixedProjectionError();
      values += 1;
      if (values > HARD_LIMITS.maxJSONTokens) throw fixedProjectionError();
      if (isNumberToken(value)) return projectedNumber;
      if (value === null || typeof value === 'boolean') return value;
      if (typeof value === 'string') {
        if (!validUnicode(value)) throw fixedProjectionError();
        return value;
      }
      if (value === null || typeof value !== 'object') throw fixedProjectionError();
      if (!containers.has(value)) throw fixedProjectionError();
      if (seen.has(value)) throw fixedProjectionError();
      seen.add(value);

      let prototype;
      let keys;
      try {
        prototype = Object.getPrototypeOf(value);
        keys = Reflect.ownKeys(value);
      } catch {
        throw fixedProjectionError();
      }

      if (Array.isArray(value)) {
        if (prototype !== Array.prototype) throw fixedProjectionError();
        let lengthDescriptor;
        try {
          lengthDescriptor = Object.getOwnPropertyDescriptor(value, 'length');
        } catch {
          throw fixedProjectionError();
        }
        if (lengthDescriptor === undefined ||
            Object.getOwnPropertyDescriptor(lengthDescriptor, 'value') === undefined ||
            typeof lengthDescriptor.value !== 'number' ||
            !Number.isSafeInteger(lengthDescriptor.value) ||
            lengthDescriptor.value > HARD_LIMITS.maxJSONTokens ||
            keys.length !== lengthDescriptor.value + 1 ||
            keys[lengthDescriptor.value] !== 'length') {
          throw fixedProjectionError();
        }
        const output = new Array(lengthDescriptor.value);
        for (let index = 0; index < lengthDescriptor.value; index += 1) {
          const key = String(index);
          if (keys[index] !== key) throw fixedProjectionError();
          let descriptor;
          try {
            descriptor = Object.getOwnPropertyDescriptor(value, key);
          } catch {
            throw fixedProjectionError();
          }
          if (descriptor === undefined || descriptor.enumerable !== true ||
              Object.getOwnPropertyDescriptor(descriptor, 'value') === undefined) {
            throw fixedProjectionError();
          }
          output[index] = copy(descriptor.value, depth + 1);
        }
        return Object.freeze(output);
      }

      if (prototype !== null || keys.length > HARD_LIMITS.maxJSONTokens - values) {
        throw fixedProjectionError();
      }
      const output = Object.create(null);
      for (const key of keys) {
        if (typeof key !== 'string') throw fixedProjectionError();
        let descriptor;
        try {
          descriptor = Object.getOwnPropertyDescriptor(value, key);
        } catch {
          throw fixedProjectionError();
        }
        if (descriptor === undefined || descriptor.enumerable !== true ||
            Object.getOwnPropertyDescriptor(descriptor, 'value') === undefined) {
          throw fixedProjectionError();
        }
        output[key] = copy(descriptor.value, depth + 1);
      }
      return Object.freeze(output);
    };

    try {
      return copy(input, 0);
    } catch {
      throw fixedProjectionError();
    }
  }

  return Object.freeze({ parse, isNumberToken, numberLexeme, forProjection });
}

// This expression has no imports or closure dependencies and can be evaluated
// in a private page context using only the trusted factory source.
export const QA_LOSSLESS_JSON_SOURCE = `(${createQALosslessJSON.toString()})()`;
