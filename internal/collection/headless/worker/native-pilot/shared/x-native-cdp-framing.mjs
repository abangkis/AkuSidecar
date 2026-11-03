import {probeError, isObject} from './x-native-value-validation.mjs';
import {parseStrictJSON} from './x-evidence-ingress.mjs';

// Bounded NUL framing, strict JSON decoding and byte cleanup.
const MAX_CDP_FRAME_BYTES = 262_144;

const MAX_CDP_INPUT_CHUNK_BYTES = 1_048_576;

const MAX_CDP_FRAMES_PER_CHUNK = 128;

const CDP_FRAME_LIMITS = Object.freeze({ maxJSONDepth: 20, maxJSONTokens: 8_192 });

const decoder = new TextDecoder('utf-8', { fatal: true, ignoreBOM: true });

function parseJSONBytes(bytes, maxFrameBytes = MAX_CDP_FRAME_BYTES) {
  if (!(bytes instanceof Uint8Array) || bytes.byteLength < 1 || bytes.byteLength > maxFrameBytes) {
    throw probeError('cdp_frame_size_invalid');
  }
  let text;
  try { text = decoder.decode(bytes); } catch { throw probeError('cdp_utf8_invalid'); }
  try {
    const value = parseStrictJSON(text, CDP_FRAME_LIMITS);
    if (!isObject(value)) throw probeError('cdp_json_invalid');
    return value;
  } catch (error) {
    if (error?.message?.startsWith('xnp_')) throw error;
    throw probeError('cdp_json_invalid');
  }
}

class CDPFrameDecoder {
  constructor({ maxFrameBytes = MAX_CDP_FRAME_BYTES, maxInputBytes = MAX_CDP_INPUT_CHUNK_BYTES,
    maxFramesPerChunk = MAX_CDP_FRAMES_PER_CHUNK } = {}) {
    if (!Number.isSafeInteger(maxFrameBytes) || maxFrameBytes < 1 || maxFrameBytes > MAX_CDP_FRAME_BYTES ||
        !Number.isSafeInteger(maxInputBytes) || maxInputBytes < 1 || maxInputBytes > MAX_CDP_INPUT_CHUNK_BYTES ||
        !Number.isSafeInteger(maxFramesPerChunk) || maxFramesPerChunk < 1 || maxFramesPerChunk > MAX_CDP_FRAMES_PER_CHUNK) {
      throw probeError('cdp_decoder_limits_invalid');
    }
    this.maxFrameBytes = maxFrameBytes;
    this.maxInputBytes = maxInputBytes;
    this.maxFramesPerChunk = maxFramesPerChunk;
    this.buffer = Buffer.allocUnsafe(maxFrameBytes);
    this.length = 0;
  }

  push(chunk, onFrame) {
    if (!(chunk instanceof Uint8Array) || chunk.byteLength > this.maxInputBytes || typeof onFrame !== 'function') {
      throw probeError('cdp_input_invalid');
    }
    let frames = 0;
    for (const byte of chunk) {
      if (byte === 0) {
        if (++frames > this.maxFramesPerChunk) throw probeError('cdp_frame_count_exceeded');
        if (this.length === 0) throw probeError('cdp_frame_empty');
        const frame = Buffer.from(this.buffer.subarray(0, this.length));
        this.buffer.fill(0, 0, this.length);
        this.length = 0;
        let message;
        try {
          message = parseJSONBytes(frame, this.maxFrameBytes);
          onFrame(message);
        } finally {
          frame.fill(0);
          message = null;
        }
      } else {
        if (this.length >= this.maxFrameBytes) {
          this.reset();
          throw probeError('cdp_frame_too_large');
        }
        this.buffer[this.length++] = byte;
      }
    }
  }

  finish() {
    if (this.length !== 0) throw probeError('cdp_frame_unterminated');
  }

  reset() {
    this.buffer.fill(0);
    this.length = 0;
  }
}

export {MAX_CDP_FRAME_BYTES, parseJSONBytes, CDPFrameDecoder};
