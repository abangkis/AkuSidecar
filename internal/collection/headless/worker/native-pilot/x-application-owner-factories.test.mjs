import test from 'node:test';
import assert from 'node:assert/strict';
import {mkdir,mkdtemp,readFile,realpath,rename,rm} from 'node:fs/promises';
import {dirname,join,relative,resolve,sep} from 'node:path';
import {fileURLToPath} from 'node:url';
import {buildNativeOwnerPackage,buildNativeDailyOwnerPackage,buildMainBrowserTransportPackage} from '../../../../../scripts/build-x-native-owner.mjs';

const project=resolve(dirname(fileURLToPath(import.meta.url)),'../../../../..');
const scratchRoot=join(project,'runtime','qa','native-owner-factory-tests');
async function withProfiles(run){
  await mkdir(scratchRoot,{recursive:true});
  const scratch=await mkdtemp(join(scratchRoot,'scope-'));
  try{
    const readerProfile=join(scratch,'reader'),collectorProfile=join(scratch,'collector');
    await mkdir(readerProfile);await mkdir(collectorProfile);
    await run({chromePath:await realpath(process.execPath),readerProfile,collectorProfile,loadAssets:async()=>({})},scratch);
  }finally{
    const rel=relative(scratchRoot,scratch);
    assert(rel&&!rel.startsWith('..')&&!rel.includes('..'+sep));
    await rm(scratch,{recursive:true,force:true});
  }
}

test('production factories share one admission scope across manual close and daily creation',async()=>{
  const factory=await import('./x-application-owner-factories.mjs?manual-close');
  assert.deepEqual(Object.keys(factory).sort(),['createApplicationDailyOwner','createApplicationManualOwner','manualSeedRPCCode']);
  await withProfiles(async options=>{
    const owner=await factory.createApplicationManualOwner(options);
    await assert.rejects(factory.createApplicationDailyOwner(options),/xnp_daily_owner_unavailable/u);
    await owner.close();
    await assert.rejects(factory.createApplicationManualOwner(options),/xnp_application_owner_unavailable/u);
    await assert.rejects(factory.createApplicationDailyOwner(options),/xnp_daily_owner_unavailable/u);
  });
});

test('failed manual initialization consumes its original scope without launching a browser',async()=>{
  const factory=await import('./x-application-owner-factories.mjs?failed-init');
  await withProfiles(async options=>{
    const owner=await factory.createApplicationManualOwner(options);
    await rename(options.readerProfile,options.readerProfile+'-moved');
    await assert.rejects(owner.initialize(),/xnp_application_owner_scope_changed/u);
    await assert.rejects(owner.initialize(),/xnp_application_owner_replay/u);
    await owner.close();
    await assert.rejects(factory.createApplicationDailyOwner(options),/xnp_daily_owner_unavailable/u);
  });
});

test('daily partial construction failure keeps admission consumed by both factories',async()=>{
  const factory=await import('./x-application-owner-factories.mjs?failed-construction');
  await withProfiles(async options=>{
    await assert.rejects(factory.createApplicationDailyOwner({...options,borrowedReader:{pid:0}}),/xnp_borrowed_reader_rejected/u);
    await assert.rejects(factory.createApplicationDailyOwner(options),/xnp_daily_owner_unavailable/u);
    await assert.rejects(factory.createApplicationManualOwner(options),/xnp_application_owner_unavailable/u);
  });
});

test('borrowed daily close unsubscribes without closing Chrome or renewing factory admission',async()=>{
  const factory=await import('./x-application-owner-factories.mjs?borrowed-close');
  await withProfiles(async options=>{
    const calls=[];let subscribed=0,unsubscribed=0;
    const transport={alive:()=>true,call:async(...args)=>{calls.push(args);throw Error('unexpected browser call');},
      subscribe:()=>{subscribed++;return()=>{unsubscribed++;};}};
    const owner=await factory.createApplicationDailyOwner({...options,borrowedReader:{pid:process.pid,profileDirectory:'Default',transport}});
    await owner.initialize();await owner.close();await owner.close();
    assert.equal(subscribed,1);assert.equal(unsubscribed,1);assert.deepEqual(calls,[]);
    await assert.rejects(factory.createApplicationManualOwner(options),/xnp_application_owner_unavailable/u);
    await assert.rejects(factory.createApplicationDailyOwner(options),/xnp_daily_owner_unavailable/u);
  });
});

test('every sealed production graph contains native factories and excludes the QA dispatcher',{timeout:30000},async()=>{
  await withProfiles(async(_options,scratch)=>{
    for(const [name,build] of [['manual',buildNativeOwnerPackage],['daily',buildNativeDailyOwnerPackage],['main',buildMainBrowserTransportPackage]]){
      const result=await build(join(scratch,name));
      const manifest=JSON.parse(await readFile(result.manifest,'utf8'));
      const paths=manifest.modules.map(row=>row.path);
      assert(paths.includes('internal/collection/headless/worker/native-pilot/x-application-owner-factories.mjs'));
      assert(paths.includes('internal/collection/headless/worker/native-pilot/x-native-owner-runtime.mjs'));
      assert(paths.includes('internal/collection/headless/worker/native-pilot/shared/x-native-cdp-result-projection.mjs'));
      for (const feature of ['x-native-value-validation', 'x-native-cdp-framing', 'x-native-network-metadata',
        'x-native-cdp-event-projection', 'x-native-observation-policy', 'x-native-owner-workflow', 'x-native-cdp-command-policy']) {
        assert(paths.includes(`internal/collection/headless/worker/native-pilot/shared/${feature}.mjs`), `${name}: missing ${feature}`);
      }
      assert(!paths.some(path=>path.endsWith('x-native-metadata-probe-driver.mjs')));
      assert(!paths.some(path=>path.startsWith('scripts/auth-session-fixture/')),`${name}: production imports a fixture module`);
      assert(paths.includes('internal/collection/headless/worker/native-pilot/shared/x-standard-chrome-args.mjs'));
      assert(paths.includes('internal/collection/headless/worker/native-pilot/shared/x-evidence-ingress.mjs'));
      const bytes=await readFile(result.driver,'utf8');
      for(const marker of ['runNativeMetadataProbe','handleControlLine','qualify_synchronized_short_concurrency'])assert(!bytes.includes(marker),`${name}: QA dispatcher marker ${marker}`);
    }
  });
});
