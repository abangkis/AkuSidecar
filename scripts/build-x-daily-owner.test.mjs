import test from 'node:test';
import assert from 'node:assert/strict';
import {mkdtemp,mkdir,readFile,rm} from 'node:fs/promises';
import {resolve,join,dirname} from 'node:path';
import {fileURLToPath} from 'node:url';
import {createHash} from 'node:crypto';
import {buildNativeDailyOwnerPackage,buildMainBrowserTransportPackage} from './build-x-native-owner.mjs';
const project=resolve(dirname(fileURLToPath(import.meta.url)),'..');
test('daily owner package seals the distinct process/batch entry without execution',async t=>{
  const parent=join(project,'build');await mkdir(parent,{recursive:true});
  const root=await mkdtemp(join(parent,'test-daily-owner-'));
  t.after(async()=>{if(dirname(root)!==parent)throw new Error('invalid cleanup boundary');await rm(root,{recursive:true,force:true});});
  const result=await buildNativeDailyOwnerPackage(root);
  const manifest=JSON.parse(await readFile(result.manifest,'utf8')),bytes=await readFile(result.driver);
  assert.equal(manifest.dailyLifecycle,true);
  assert.equal(manifest.entry,'internal/collection/headless/worker/native-pilot/daily-owner.mjs');
  assert.equal(createHash('sha256').update(bytes).digest('hex'),result.sha256);
  assert.match(bytes.toString(),/module\.runDailyOwner\(\)/u);
  assert(manifest.modules.some(row=>row.path.endsWith('/daily-owner-protocol.mjs')));
  assert(manifest.modules.some(row=>row.path.endsWith('/source-retry-policy.mjs')));
  assert(manifest.modules.some(row=>row.path.endsWith('/native-source-kernel.mjs')));
  assert(manifest.captureAssets.length>0);
});

test('main transport seals per-source assets without replacing the existing X bundle',async t=>{
  const parent=join(project,'build');await mkdir(parent,{recursive:true});
  const root=await mkdtemp(join(parent,'test-source-owner-package-'));
  t.after(async()=>{assert.equal(dirname(root),parent);await rm(root,{recursive:true,force:true});});
  const result=await buildMainBrowserTransportPackage(root);
  const manifest=JSON.parse(await readFile(result.manifest,'utf8'));
  assert.equal(manifest.mainLifecycle,true);
  assert.deepEqual(Object.keys(manifest.sourceCaptureAssets),['x','instagram']);
  for(const source of ['x','instagram'])assert(manifest.sourceCaptureAssets[source].some(row=>row.path===`AkuBridge/adapters/${source}-adapter.js`));
  assert.equal(new Set(manifest.captureAssets.map(row=>row.path)).size,manifest.captureAssets.length);
  for(const name of ['instagram-native-provider.mjs','instagram-native-session.mjs','native-source-kernel.mjs','source-owner-registry.mjs','source-retry-policy.mjs'])
    assert(manifest.modules.some(row=>row.path.endsWith('/'+name)));
  assert.equal(createHash('sha256').update(await readFile(result.driver)).digest('hex'),result.sha256);
});
