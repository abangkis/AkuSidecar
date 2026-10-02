import test from 'node:test';
import assert from 'node:assert/strict';
import {compareQuietHeadless} from './quiet-headless-comparison.mjs';
const block={platformId:'x:status:123',permalink:'https://x.com/fixture/status/123',author:'Fixture',text:'Content',media:[]};
const receipt=(mode,blocks=[block])=>({schema:'aku.authenticated-parity-report.v1',
  execution:{workerMode:mode,runtimeControl:{restored:true,profileReleasedBeforeRestore:true,workerExitConfirmed:true}},
  workerIdentity:{chromeVersion:{product:'Chrome/154.0.8037.93'}},captures:[{source:'x',kind:'target',ok:true,
    targetPlatformId:'x:status:123',result:{capturedAt:'2026-10-02T00:00:00Z',snapshots:[{blocks}],
      coverage:{provenance:{sources:[{path:'worker/capture.mjs',sha256:'a'.repeat(64)}]}}}}]});
test('compares exact native target across proven Quiet and headless lifecycles',()=>{
  const result=compareQuietHeadless(receipt('production_quiet_driver_packaged_worker'),receipt('packaged_worker'));
  assert.equal(result.cases[0].status,'native_identity_and_author_match');
  assert.equal(result.cases[0].textEqual,true);
  assert.equal(result.cases[0].sourceAssetHashesEqual,true);
  assert.equal(result.fullParityVerified,false);
  assert.equal(result.cases[1].status,'target_capture_unavailable');
});
test('conflicting Quiet bindings and missing lifecycle never become parity proof',()=>{
  const quiet=receipt('production_quiet_driver_packaged_worker',[block,{...block,author:'Other'}]);
  assert.equal(compareQuietHeadless(quiet,receipt('packaged_worker')).cases[0].status,'ambiguous_quiet_target');
  quiet.execution.runtimeControl.restored=false;
  assert.throws(()=>compareQuietHeadless(quiet,receipt('packaged_worker')),/lifecycle/);
});
test('different source hashes remain explicit even when text matches',()=>{
  const headless=receipt('packaged_worker');headless.captures[0].result.coverage.provenance.sources[0].sha256='b'.repeat(64);
  assert.equal(compareQuietHeadless(receipt('production_quiet_driver_packaged_worker'),headless).cases[0].sourceAssetHashesEqual,false);
});
