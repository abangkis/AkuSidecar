import test from 'node:test';
import assert from 'node:assert/strict';
import {spawnSync} from 'node:child_process';
import {renderXQAController} from './x-qa-controller-render.mjs';
import {createCaptureStartGate} from './x-qa-capture-start-gate.mjs';

const pins={id:'a'.repeat(32),commit:'b'.repeat(40),sourceSHA256:'c'.repeat(64)};
test('rendered controller embeds the exact tested gate and parses without execution',async()=>{
  const source=await renderXQAController(pins);
  assert.equal(source.includes(createCaptureStartGate.toString()),true);assert.equal(source.includes('@@'),false);
  const checked=spawnSync(process.execPath,['--input-type=module','--check'],{input:source,encoding:'utf8',timeout:5000});
  assert.equal(checked.status,0,checked.stderr);
});

test('rendered controller cannot execute without explicit one-shot arguments',async()=>{
  const source=await renderXQAController(pins);
  const result=spawnSync(process.execPath,['--input-type=module'],{input:source,encoding:'utf8',timeout:5000});
  assert.notEqual(result.status,0);assert.match(result.stderr,/Explicit one-shot authorization/);
  assert.equal(result.stdout,'');
});

test('invalid source pins cannot become controller code or grant fields',async()=>{
  for(const patch of [{id:'../PRIVATE'},{commit:'PRIVATE'},{sourceSHA256:'PRIVATE'}])
    await assert.rejects(renderXQAController({...pins,...patch}),/QA controller pins invalid/);
});
