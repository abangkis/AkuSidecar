import test from 'node:test';
import assert from 'node:assert/strict';
import {mkdir,mkdtemp,readFile,writeFile,rm,copyFile} from 'node:fs/promises';
import {dirname,join,resolve} from 'node:path';
import {fileURLToPath} from 'node:url';
import {digest,fileRecord,inventory,requiredRuntimeSchemas} from './x-development-pilot-package.mjs';
import {dailyPackageSchema,argumentValue,dailyLayout,dailyConfigProposal,dailyServiceProposal,verifyDailyPackage} from './x-daily-development-package.mjs';
const project=resolve(dirname(fileURLToPath(import.meta.url)),'..');
async function fixture(t){
  const parent=join(project,'build');await mkdir(parent,{recursive:true});
  const temp=await mkdtemp(join(parent,'test-daily-package-'));
  t.after(async()=>{if(dirname(temp)!==parent)throw new Error('cleanup_boundary');await rm(temp,{recursive:true,force:true});});
  const main=join(temp,'main'),root=join(temp,'package'),runtime=join(main,'runtime','dev');
  const service={cwd:main,command:join(runtime,'aku-sidecar.exe'),args:['--config',join(main,'config','sidecar.json'),
    '--dev','--app-shell','--windows-capture-split','--bridge-extension-origin','chrome-extension://synthetic/',
    '--browser-profile',join(main,'runtime','ordinary-profile'),'--chromium-path',join(main,'chrome','chrome.exe'),
    '--ui-chromium-path',join(main,'ui','bin','chrome.exe'),'--bridge-extension-path',join(main,'bridge')],
    ports:[11122],environment:{},shutdownGraceMs:5000,restartPolicy:'manual'};
  const config={version:1,deployment:{mode:'development'},server:{host:'127.0.0.1',port:11122},
    database:{path:'runtime/main.db'},reasoning:{activeProvider:'synthetic',providers:{synthetic:{modelId:'unchanged'}}},
    capture:{visibility:'quiet',behavior:{enabled:true}},preference:{mode:'guarded_live'}};
  const write=async(path,value)=>{await mkdir(dirname(path),{recursive:true});await writeFile(path,typeof value==='object'?JSON.stringify(value):value);};
  await write(service.args[1],config);
  const baseline=[];
  for(const [path,name] of [[service.args[1],'sidecar.json'],[service.command,'aku-sidecar.exe'],
    [join(runtime,'aku-reader-broker.exe'),'aku-reader-broker.exe'],[service.command+'.runtime-state.json','aku-sidecar.exe.runtime-state.json']]){
    if(name!=='sidecar.json')await write(path,'synthetic; never execute');
    const backup=join(root,'rollback',name);await mkdir(dirname(backup),{recursive:true});await copyFile(path,backup);
    baseline.push({...await fileRecord(path),backup:'rollback/'+name});
  }
  const layout=dailyLayout(service,config),configSHA256=(await fileRecord(layout.config)).sha256;
  for(const rel of ['bin/aku-sidecar.exe','bin/aku-reader-broker.exe','bin/ui-reader-broker/manifest.json',
    'bin/ui-reader-broker/content.js','bin/ui-reader-broker/service-worker.js','bin/headless-worker/worker.mjs',
    'bin/headless-worker/LICENSE-AkuSidecar','ACTIVATION.md','preflight.ps1','tools/verify-x-daily-development.mjs',
    'tools/x-daily-development-package.mjs','tools/x-development-pilot-package.mjs'])await write(join(root,rel),'synthetic');
  await write(join(root,'bin/headless-worker/node.exe'),'synthetic node');await write(join(root,'bin/headless-worker/LICENSE'),'synthetic license');
  await write(join(root,'bin/headless-worker/node.pin.json'),{protocol:1,nodeSha256:digest('synthetic node'),licenseSha256:digest('synthetic license')});
  await write(join(root,'bin/x-daily-owner/x-daily-owner.cjs'),'synthetic daily driver');
  await write(join(root,'bin/x-daily-owner/manifest.json'),{schema:'aku.x.application-owner-package.v1',dailyLifecycle:true,
    entry:'internal/collection/headless/worker/native-pilot/daily-owner.mjs',driver:{file:'x-daily-owner.cjs',sha256:digest('synthetic daily driver')},captureAssets:[]});
  await write(join(root,'bin/com.akubrowser.reader_activation.json'),{name:'com.akubrowser.reader_activation',description:'Explicit Windows AkuBrowser reader activation',
    path:join(root,'bin/aku-reader-broker.exe'),type:'stdio',allowed_origins:['chrome-extension://dlibmmlopdahibfniinemhnghlifiple/']});
  await write(join(root,'config-proposal.json'),dailyConfigProposal(layout.config,configSHA256));
  await write(join(root,'service-proposal.json'),dailyServiceProposal(service,join(root,'bin/aku-sidecar.exe')));
  await write(join(root,'rollback/service.json'),service);await write(join(root,'rollback/native-hosts.json'),{synthetic:true});
  const sourceInputs=[];
  for(const name of requiredRuntimeSchemas){const path=join(main,'schemas',name);await write(path,{synthetic:true});sourceInputs.push(await fileRecord(path));}
  const preparationInputs=[];
  for(const name of ['scripts/prepare-x-daily-development.mjs','scripts/x-daily-development-package.mjs','scripts/x-daily-preflight.ps1',
    'scripts/verify-x-daily-development.mjs','scripts/x-development-pilot-package.mjs','docs/x-daily-development-activation.md']){
    const path=join(main,name);await write(path,'synthetic preparation source');preparationInputs.push(await fileRecord(path));
  }
  const externalAssets=[];
  for(const path of [layout.chrome,layout.uiChrome,join(dirname(dirname(layout.uiChrome)),'pin.json')]){await write(path,'synthetic code');externalAssets.push(await fileRecord(path));}
  const manifest={schema:dailyPackageSchema,packageId:'synthetic',sourceCommit:'synthetic',preparationOnly:true,executionApproved:false,
    activationReady:false,processConsentGranted:false,defaultOff:true,authorization:{scope:'sidecar-process',maxBatchMS:60000,retryFailedBatch:false,cookieNames:['auth_token','ct0']},
    ordinary:layout,configSHA256,configProbe:{status:'ok',databaseSchemaVersion:30},reader:{selection:'unresolved',profileInspected:false,leaseAcquired:false},
    baseline,sourceInputs,externalAssets,preparationInputs,files:await inventory(root)};
  const path=join(root,'daily-package.json'),save=()=>writeFile(path,JSON.stringify(manifest));await save();
  const refresh=async()=>{manifest.files=(await inventory(root)).filter(row=>row.path!==path);await save();};
  return{root,main,path,service,config,manifest,save,refresh,write};
}
test('daily proposal preserves ordinary arguments/state and never grants process consent',async t=>{
  const f=await fixture(t),before=JSON.stringify(f.service),proposed=dailyServiceProposal(f.service,'synthetic-candidate');
  assert.deepEqual(proposed.args,f.service.args);assert.equal(proposed.cwd,f.service.cwd);assert.equal(JSON.stringify(f.service),before);
  assert.deepEqual(dailyConfigProposal('config','sha').changes,[{path:'/capture/xMode',value:'concurrent'},
    {path:'/capture/xReaderProfile',value:null,requiresExplicitSelection:true}]);
  const result=await verifyDailyPackage(f.path);assert.equal(result.databaseOpened,false);assert.equal(result.profileInspected,false);assert.equal(result.activationReady,false);
  await assert.rejects(readFile(f.manifest.ordinary.database),{code:'ENOENT'});
});
test('daily layout rejects QA selectors, duplicate path overrides and relocated cwd',async t=>{
  const f=await fixture(t);
  for(const arg of ['--dev-session-reuse=native-x-manual','--dev-qa-pilot-plan=other','--provider=other',
    '--discover-codex','--discover-chromium','--','positional','--dev=false','--app-shell=false','--app-shell']){
    assert.throws(()=>dailyLayout({...f.service,args:[...f.service.args,arg]},f.config),/selector_rejected/);
  }
  assert.throws(()=>argumentValue(['--config=a','--config','b'],'--config'),/duplicate/);
  assert.throws(()=>dailyLayout({...f.service,cwd:f.root},f.config),/root_changed/);
});
test('verification rejects changed bytes, missing daily payload and existing handoff receipt',async t=>{
  const f=await fixture(t);await writeFile(join(f.root,'bin/aku-sidecar.exe'),'changed');await assert.rejects(verifyDailyPackage(f.path),/payload_changed/);
  await f.refresh();f.manifest.files=f.manifest.files.filter(row=>row.file!=='bin/aku-reader-broker.exe');await f.save();
  await assert.rejects(verifyDailyPackage(f.path),/required_payload_missing/);
  await f.refresh();await writeFile(join(f.root,'activation-receipt.json'),'{}');await assert.rejects(verifyDailyPackage(f.path),/already_handed_off/);
});
test('service/config proposals cannot silently select QA or modify budgets/providers',async t=>{
  const f=await fixture(t),proposal=JSON.parse(await readFile(join(f.root,'service-proposal.json')));
  proposal.args.push('--database=qa.db');await f.write(join(f.root,'service-proposal.json'),proposal);await f.refresh();
  await assert.rejects(verifyDailyPackage(f.path),/service_proposal_changed/);
});
test('profile choice cannot be inferred and process/batch authority cannot be relaxed',async t=>{
  const f=await fixture(t);f.manifest.reader.selection='automatic';await f.save();await assert.rejects(verifyDailyPackage(f.path),/reader_prerequisite/);
  f.manifest.reader.selection='unresolved';f.manifest.authorization.maxBatchMS=60001;await f.save();await assert.rejects(verifyDailyPackage(f.path),/contract_rejected/);
});
test('baseline and code input drift invalidate a prepared candidate',async t=>{
  const f=await fixture(t);await writeFile(f.service.command,'new main binary');await assert.rejects(verifyDailyPackage(f.path),/main_baseline_stale/);
  await copyFile(join(f.root,'rollback/aku-sidecar.exe'),f.service.command);
  await writeFile(f.manifest.preparationInputs[0].path,'new preparation code');await assert.rejects(verifyDailyPackage(f.path),/input_stale/);
});
test('verification rejects database/profile paths before reading them as assets',async t=>{
  const f=await fixture(t);f.manifest.externalAssets[0]={path:f.manifest.ordinary.database,sha256:'unknown'};await f.save();
  await assert.rejects(verifyDailyPackage(f.path),/external_scope_rejected/);
  f.manifest.baseline[0].path=f.manifest.ordinary.database;await f.save();await assert.rejects(verifyDailyPackage(f.path),/baseline_scope_rejected/);
});
test('inventory traversal rejects before an outside read',async t=>{
  const f=await fixture(t);f.manifest.files.unshift({file:'../missing-private-file',sha256:'unknown'});await f.save();
  await assert.rejects(verifyDailyPackage(f.path),/path_escape/);
});
