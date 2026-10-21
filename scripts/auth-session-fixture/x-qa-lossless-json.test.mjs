import assert from 'node:assert/strict';
import test from 'node:test';
import vm from 'node:vm';
import { createQALosslessJSON, QA_LOSSLESS_JSON_SOURCE } from './x-qa-lossless-json.mjs';
import { parseStrictJSON } from './x-evidence-ingress.mjs';
import { inspectBootstrapCurrentSelf } from './x-bootstrap-current-self.mjs';

const CAP = 256 * 1024;
const ID = '12345678901234567890';
const lexemes = ['0', '-0', '123', '9007199254740993', '-9007199254740993',
  '0.10000000000000001', '1.00000000000000001', '1e400', '1E+00400', '1e-999999'];

function reject(context, text, reason, limits) {
  const observed = [];
  assert.throws(() => context.parse(text, limits, (...args) => observed.push(args)),
    { name: 'SyntaxError', message: 'invalid_json' });
  assert.deepEqual(observed, [[reason]]);
}

function selfJSON() {
  return JSON.stringify({ session: { user_id_str: ID, loggedIn: true },
    entities: { users: { entities: { [ID]: { id_str: ID } } } } });
}

test('preserves every numeric lexeme privately with no conversion or coercion', () => {
  const context = createQALosslessJSON();
  const parsed = context.parse(`[${lexemes.join(',')}]`);
  parsed.forEach((token, index) => {
    assert.equal(context.isNumberToken(token), true);
    assert.equal(context.numberLexeme(token), lexemes[index]);
    assert.equal(Object.getPrototypeOf(token), null);
    assert.equal(Object.isFrozen(token), true);
    assert.deepEqual(Reflect.ownKeys(token), []);
    assert.equal(JSON.stringify(token), '{}');
    assert.throws(() => String(token), TypeError);
  });
  assert.equal(context.isNumberToken('123'), false);
  assert.equal(context.isNumberToken(123), false);
  assert.notEqual(parsed[0], parsed[1]);
});

test('huge exponent remains bounded text rather than expanded arithmetic', { timeout: 3000 }, () => {
  const context = createQALosslessJSON();
  const input = `1e${'9'.repeat(20_000)}`;
  const token = context.parse(input);
  assert.equal(context.numberLexeme(token), input);
  assert.equal(typeof context.forProjection(token), 'symbol');
});

test('preserves strings and booleans while numeric projection markers remain non-record primitives', () => {
  const context = createQALosslessJSON();
  const parsed = context.parse('{"text":"9007199254740993","truth":true,"nil":null,"values":[-0,1e400]}');
  const view = context.forProjection(parsed);
  assert.equal(view.text, '9007199254740993');
  assert.equal(view.truth, true);
  assert.equal(view.nil, null);
  assert.equal(typeof view.values[0], 'symbol');
  assert.equal(typeof view.values[1], 'symbol');
  assert.equal(Object.isFrozen(view), true);
  assert.equal(Object.isFrozen(view.values), true);
  assert.equal(context.numberLexeme(parsed.values[0]), '-0');
});

test('token-like JSON and foreign parser contexts cannot manufacture numeric provenance', () => {
  const context = createQALosslessJSON();
  const other = createQALosslessJSON();
  const fake = context.parse('{"lexeme":"123","kind":"number"}');
  assert.equal(context.isNumberToken(fake), false);
  assert.throws(() => context.numberLexeme(fake), /invalid_number_token/);
  assert.equal(context.forProjection(fake).kind, 'number');
  const foreign = other.parse('123');
  assert.equal(context.isNumberToken(foreign), false);
  assert.throws(() => context.numberLexeme(foreign), /invalid_number_token/);
  assert.throws(() => context.forProjection(foreign), /invalid_projection_input/);
  assert.throws(() => context.forProjection(other.parse('{}')), /invalid_projection_input/);
  const own = context.parse('{}');
  own.foreign = foreign;
  assert.throws(() => context.forProjection(own), /invalid_projection_input/);
});

test('rejects malformed numeric syntax, non-JSON constants and trailing code', () => {
  const context = createQALosslessJSON();
  for (const source of ['', '-', '+1', '01', '-01', '.1', '1.', '1e', '1e+',
    'NaN', 'Infinity', '-Infinity', '0x10', '[1,]', '{"a":1,}', 'true false', '1;evil()']) {
    reject(context, source, 'malformed_json');
  }
});

test('strictly rejects duplicate decoded keys and invalid Unicode', () => {
  const context = createQALosslessJSON();
  reject(context, '{"a":1,"\\u0061":2}', 'duplicate_key');
  reject(context, '{"__proto__":1,"__proto__":2}', 'duplicate_key');
  for (const source of ['"\\uD800"', '"\\uDC00"', '"\uD800"', '{"\\uD800":1}']) {
    reject(context, source, 'invalid_unicode');
  }
  reject(context, '"a\nb"', 'malformed_json');
  assert.equal(context.parse('"\\uD83D\\uDE00"'), '😀');
  const proto = context.parse('{"__proto__":1,"constructor":2}');
  assert.equal(Object.getPrototypeOf(proto), null);
  assert.equal(context.isNumberToken(proto.__proto__), true);
});

test('enforces exact depth and value budgets with narrower limits only', () => {
  const context = createQALosslessJSON();
  context.parse(`${'['.repeat(24)}0${']'.repeat(24)}`);
  reject(context, `${'['.repeat(25)}0${']'.repeat(25)}`, 'depth_limit');
  context.forProjection(context.parse(`[${'0,'.repeat(16_382)}0]`));
  reject(context, `[${'0,'.repeat(16_383)}0]`, 'value_limit');
  context.parse('0', { maxJSONDepth: 0, maxJSONTokens: 1 });
  reject(context, '[0]', 'depth_limit', { maxJSONDepth: 0 });
  reject(context, '[0]', 'value_limit', { maxJSONTokens: 1 });
  for (const limits of [{ maxJSONDepth: 25 }, { maxJSONTokens: 16_385 },
    { maxInputBytes: CAP + 1 }, { unknown: 1 }, { maxJSONDepth: NaN }]) {
    reject(context, 'null', 'initializer_limit', limits);
  }
});

test('enforces 256 KiB input bytes and exact UTF-8 accounting before parse', () => {
  const context = createQALosslessJSON();
  assert.equal(context.parse(`"${'x'.repeat(CAP - 2)}"`).length, CAP - 2);
  reject(context, `"${'x'.repeat(CAP - 1)}"`, 'initializer_limit');
  assert.equal(context.parse('"é"', { maxInputBytes: 4 }), 'é');
  reject(context, '"é"', 'initializer_limit', { maxInputBytes: 3 });
  assert.equal(context.parse('"😀"', { maxInputBytes: 6 }), '😀');
  reject(context, '"😀"', 'initializer_limit', { maxInputBytes: 5 });
});

test('rejects getters, cycles, unsupported values and widened projection graphs', () => {
  const context = createQALosslessJSON();
  const getter = context.parse('{}');
  let reads = 0;
  Object.defineProperty(getter, 'private', { enumerable: true, get() { reads++; throw new Error('PRIVATE'); } });
  assert.throws(() => context.forProjection(getter), /invalid_projection_input/);
  assert.equal(reads, 0);
  const cyclic = context.parse('{}'); cyclic.self = cyclic;
  assert.throws(() => context.forProjection(cyclic), /invalid_projection_input/);
  for (const unsupported of [undefined, () => {}, 123, Symbol(), new Date(), {}]) {
    assert.throws(() => context.forProjection(unsupported), /invalid_projection_input/);
  }
  const large = context.parse('[]'); large.length = 16_385;
  assert.throws(() => context.forProjection(large), /invalid_projection_input/);
});

test('never inspects arbitrary exception messages or invokes limit accessors', () => {
  const context = createQALosslessJSON();
  let reads = 0;
  const error = new SyntaxError();
  Object.defineProperty(error, 'message', { get() { reads++; throw new Error('PRIVATE'); } });
  const limits = new Proxy({}, { getPrototypeOf() { throw error; } });
  reject(context, '0', 'initializer_limit', limits);
  const accessor = {};
  Object.defineProperty(accessor, 'maxJSONDepth', { enumerable: true, get() { reads++; } });
  reject(context, '0', 'initializer_limit', accessor);
  assert.throws(() => context.parse('{', undefined, () => { throw error; }), { message: 'invalid_json' });
  assert.equal(reads, 0);
});

test('serialized factory has the same private exact-number and strict projection semantics', () => {
  const context = vm.runInNewContext(QA_LOSSLESS_JSON_SOURCE, {}, { timeout: 1000 });
  const parsed = context.parse('[9007199254740993,0.10000000000000001,1e400]');
  assert.equal(context.numberLexeme(parsed[0]), '9007199254740993');
  assert.equal(context.numberLexeme(parsed[1]), '0.10000000000000001');
  assert.equal(context.numberLexeme(parsed[2]), '1e400');
  assert.equal(typeof context.forProjection(parsed)[0], 'symbol');
  assert.throws(() => context.parse('{"a":1,"a":2}'), { message: 'invalid_json' });
});

test('only original string identities survive numeric projection; shared parser stays strict', () => {
  const context = createQALosslessJSON();
  const original = selfJSON();
  const source = `${original.slice(0, -1)},"unrelated":9007199254740993}`;
  const view = context.forProjection(context.parse(source));
  const projected = inspectBootstrapCurrentSelf({ location: { origin: 'https://x.com', pathname: '/home' }, __INITIAL_STATE__: view });
  assert.equal(projected.entityChecks.normalizedUsers, 'match');
  assert.equal(projected.idFields.user_id_str, 'string');
  assert.equal(projected.authFields.loggedIn, 'true');
  for (const [before, after] of [
    [`"user_id_str":"${ID}"`, `"user_id_str":"${ID}","user_id":123`],
    [`"id_str":"${ID}"`, `"id_str":"${ID}","id":9007199254740993`],
    ['"loggedIn":true', '"loggedIn":1'],
  ]) {
    const parsed = context.parse(original.replace(before, after));
    const dto = inspectBootstrapCurrentSelf({ location: { origin: 'https://x.com', pathname: '/home' }, __INITIAL_STATE__: context.forProjection(parsed) });
    assert.ok(dto.idFields.user_id === 'invalid' || dto.entityChecks.normalizedUsers === 'invalid' || dto.authFields.loggedIn === 'invalid');
  }
  assert.throws(() => parseStrictJSON(source), { message: 'invalid_json' });
  assert.throws(() => parseStrictJSON('1e400'), { message: 'invalid_json' });
});
