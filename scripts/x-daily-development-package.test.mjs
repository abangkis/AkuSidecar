import test from 'node:test';
import assert from 'node:assert/strict';
import {mkdir,mkdtemp,readFile,writeFile,rm,copyFile} from 'node:fs/promises';
import {dirname,join,resolve} from 'node:path';
import {fileURLToPath} from 'node:url';
import {buildMainBrowserTransportPackage} from './build-x-native-owner.mjs';
import {digest,fileRecord,inventory,requiredRuntimeSchemas} from './x-development-pilot-package.mjs';
import {dailyPackageSchema,dailyPreparationArguments,dailyReaderContract,dailyReaderSelection,dailyPrerequisites,
  argumentValue,dailyLayout,dailyConfigProposal,dailyMainRollbackProposal,dailyServiceProposal,dailyRollbackProvenance,verifyDailyPackage} from './x-daily-development-package.mjs';
const project=resolve(dirname(fileURLToPath(import.meta.url)),'..');
async function fixture(t,readerSource='dedicated',capture={},packaged=false){
  const reader=dailyReaderContract(readerSource);
  const parent=join(project,'build');await mkdir(parent,{recursive:true});
  const temp=await mkdtemp(join(parent,'test-daily-package-'));
  t.after(async()=>{if(dirname(temp)!==parent)throw new Error('cleanup_boundary');await rm(temp,{recursive:true,force:true});});
  const main=join(temp,'main'),root=join(temp,'package');
  const runtime=packaged?join(main,'build','x-daily-development-prepared','a'.repeat(32),'bin'):join(main,'runtime','dev');
  const service={cwd:main,command:join(runtime,'aku-sidecar.exe'),args:['--config',join(main,'config','sidecar.json'),
    '--dev','--app-shell','--windows-capture-split','--bridge-extension-origin','chrome-extension://synthetic/',
    '--browser-profile',join(main,'runtime','ordinary-profile'),'--chromium-path',join(main,'chrome','chrome.exe'),
    '--ui-chromium-path',join(main,'ui','bin','chrome.exe'),'--bridge-extension-path',join(main,'bridge')],
    ports:[11122],environment:{},shutdownGraceMs:5000,restartPolicy:'manual'};
  const config={version:1,deployment:{mode:'development'},server:{host:'127.0.0.1',port:11122},
    database:{path:'runtime/main.db'},reasoning:{activeProvider:'synthetic',providers:{synthetic:{modelId:'unchanged'}}},
    capture:{visibility:'quiet',behavior:{enabled:true},...capture},preference:{mode:'guarded_live'}};
  const write=async(path,value)=>{await mkdir(dirname(path),{recursive:true});await writeFile(path,typeof value==='object'?JSON.stringify(value):value);};
  await write(service.args[1],config);
  const baseline=[];
  for(const [path,name] of [[service.args[1],'sidecar.json'],[service.command,'aku-sidecar.exe'],
    [join(runtime,'aku-reader-broker.exe'),'aku-reader-broker.exe']]){
    if(name!=='sidecar.json')await write(path,'synthetic; never execute');
    const backup=join(root,'rollback',name);await mkdir(dirname(backup),{recursive:true});await copyFile(path,backup);
    baseline.push({...await fileRecord(path),backup:'rollback/'+name});
  }
  if(packaged){
    await write(join(runtime,'com.akubrowser.reader_activation.json'),{synthetic:true});
    const files=[];
    for(const name of ['aku-sidecar.exe','aku-reader-broker.exe','com.akubrowser.reader_activation.json'])
      files.push({file:'bin/'+name,sha256:(await fileRecord(join(runtime,name))).sha256});
    await write(join(dirname(runtime),'daily-package.json'),{schema:dailyReaderContract('main').schema,readerSource:'main',
      packageId:'a'.repeat(32),sourceCommit:'b'.repeat(40),preparationOnly:true,buildTags:['auth_session_dev'],files});
    await write(join(dirname(runtime),'activation-receipt.json'),{historical:true});
  }else await write(service.command+'.runtime-state.json','synthetic provenance');
  const provenance=await dailyRollbackProvenance(service,{allowPackaged:readerSource==='main'});
  await copyFile(provenance.path,join(root,provenance.backup));
  baseline.push({...await fileRecord(provenance.path),backup:provenance.backup});
  const layout=dailyLayout(service,config,readerSource),configSHA256=(await fileRecord(layout.config)).sha256;
  for(const rel of ['bin/aku-sidecar.exe','bin/aku-reader-broker.exe','bin/ui-reader-broker/manifest.json',
    'bin/ui-reader-broker/content.js','bin/ui-reader-broker/service-worker.js','bin/headless-worker/worker.mjs',
    'bin/headless-worker/LICENSE-AkuSidecar','ACTIVATION.md','preflight.ps1','tools/verify-x-daily-development.mjs',
    'tools/x-daily-development-package.mjs','tools/x-development-pilot-package.mjs'])await write(join(root,rel),'synthetic');
  await write(join(root,'bin/headless-worker/node.exe'),'synthetic node');await write(join(root,'bin/headless-worker/LICENSE'),'synthetic license');
  await write(join(root,'bin/headless-worker/node.pin.json'),{protocol:1,nodeSha256:digest('synthetic node'),licenseSha256:digest('synthetic license')});
  await write(join(root,'bin',reader.directory,reader.driver),'synthetic daily driver');
  await write(join(root,'bin',reader.directory,'manifest.json'),{schema:'aku.x.application-owner-package.v1',[reader.lifecycle]:true,
    entry:reader.entry,driver:{file:reader.driver,sha256:digest('synthetic daily driver')},captureAssets:[]});
  await write(join(root,'bin/com.akubrowser.reader_activation.json'),{name:'com.akubrowser.reader_activation',description:'Explicit Windows AkuBrowser reader activation',
    path:join(root,'bin/aku-reader-broker.exe'),type:'stdio',allowed_origins:['chrome-extension://dlibmmlopdahibfniinemhnghlifiple/']});
  await write(join(root,'config-proposal.json'),dailyConfigProposal(layout.config,configSHA256,readerSource,config));
  if(readerSource==='main')await write(join(root,'rollback-proposal.json'),dailyMainRollbackProposal(layout.config,configSHA256,config));
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
  const manifest={schema:reader.schema,...(readerSource==='main'?{readerSource}:{}),packageId:'synthetic',sourceCommit:'synthetic',preparationOnly:true,executionApproved:false,
    activationReady:false,processConsentGranted:false,defaultOff:true,authorization:{scope:'sidecar-process',maxBatchMS:60000,retryFailedBatch:false,cookieNames:['auth_token','ct0']},
    ordinary:layout,configSHA256,configProbe:{status:'ok',databaseSchemaVersion:30},reader:dailyReaderSelection(readerSource),prerequisites:dailyPrerequisites(readerSource),
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
test('main reader requires explicit preparation selection; legacy dedicated remains the default',()=>{
  const config=join(project,'synthetic-services.json');
  assert.deepEqual(dailyPreparationArguments(['--supervisor-config='+config]),{supervisorConfig:config,readerSource:'dedicated'});
  assert.deepEqual(dailyPreparationArguments(['--reader-source=main','--supervisor-config='+config]),{supervisorConfig:config,readerSource:'main'});
  for(const args of [[],['--supervisor-config=relative.json'],['--supervisor-config='+config,'--reader-source=auto'],
    ['--supervisor-config='+config,'--reader-source=main','--reader-source=main'],
    ['--supervisor-config='+config,'--supervisor-config='+config],['--supervisor-config='+config,'--activate'],
    ['--supervisor-config='+config,'--reader-source='],['--supervisor-config='+config,'--transfer-session']]){
    assert.throws(()=>dailyPreparationArguments(args),/daily_/);
  }
  assert.equal(dailyReaderContract().schema,dailyPackageSchema);
});
test('main package binds ordinary profile without reading it or granting activation/consent',async t=>{
  const f=await fixture(t,'main'),before=JSON.stringify(f.config);
  const proposal=JSON.parse(await readFile(join(f.root,'config-proposal.json')));
  assert.deepEqual(proposal.changes,[{path:'/capture/xMode',value:'concurrent'},{path:'/capture/xReaderSource',value:'main'}]);
  assert.equal(proposal.expectedSHA256,f.manifest.configSHA256);assert.equal(proposal.applyApproved,false);
  const result=await verifyDailyPackage(f.path);
  assert.deepEqual(result,{packageId:'synthetic',sourceCommit:'synthetic',filesVerified:f.manifest.files.length,
    preparationOnly:true,executionApproved:false,activationReady:false,readerSource:'main',readerSelection:'ordinary-main',
    databaseOpened:false,profileInspected:false,sessionTransferred:false});
  assert.equal(await readFile(f.service.args[1],'utf8'),before);
  await assert.rejects(readFile(f.manifest.ordinary.database),{code:'ENOENT'});
  await assert.rejects(readFile(f.manifest.ordinary.profile),{code:'ENOENT'});
  await assert.rejects(readFile(join(f.root,'bin/x-daily-owner/manifest.json')),{code:'ENOENT'});
});
test('main rollback restores absent fields and protects service/native-host observations',async t=>{
  const f=await fixture(t,'main'),plan=JSON.parse(await readFile(join(f.root,'rollback-proposal.json')));
  assert.deepEqual(plan,{schema:'aku.x.daily-main-rollback-proposal.v1',applyApproved:false,switchReceiptRequired:true,shutdown:'graceful-only',
    config:{target:f.service.args[1],baselineSHA256:f.manifest.configSHA256,conflictPolicy:'match-switch-receipt-and-changed-fields',fields:[
      {path:'/capture/xMode',before:{present:false},after:{present:true,value:'concurrent'}},
      {path:'/capture/xReaderSource',before:{present:false},after:{present:true,value:'main'}},
      {path:'/capture/xReaderProfile',before:{present:false},after:{present:false}}]},
    service:{backup:'rollback/service.json',conflictPolicy:'match-switch-receipt'},
    nativeHosts:{backup:'rollback/native-hosts.json',conflictPolicy:'match-switch-receipt',preserveValueKindPresenceAndView:true},
    preserved:{database:true,profiles:true,login:true,providerSettings:true,autoUpdate:true},restoreDatabaseSnapshot:false});
  plan.config.fields[0].before={present:true,value:'exclusive'};
  await f.write(join(f.root,'rollback-proposal.json'),plan);await f.refresh();
  await assert.rejects(verifyDailyPackage(f.path),/rollback_proposal_changed/);
});
test('main proposal removes an explicit empty dedicated-profile field; rollback retains exact values',async t=>{
  const f=await fixture(t,'main',{xMode:'exclusive',xReaderSource:'dedicated',xReaderProfile:''});
  const proposal=JSON.parse(await readFile(join(f.root,'config-proposal.json')));
  assert.deepEqual(proposal.changes,[{path:'/capture/xMode',value:'concurrent'},{path:'/capture/xReaderSource',value:'main'},
    {path:'/capture/xReaderProfile',operation:'remove'}]);
  const plan=JSON.parse(await readFile(join(f.root,'rollback-proposal.json')));
  assert.deepEqual(plan.config.fields.map(row=>row.before),[
    {present:true,value:'exclusive'},{present:true,value:'dedicated'},{present:true,value:''}]);
  assert.equal((await verifyDailyPackage(f.path)).readerSource,'main');
});

test('explicit main refresh preserves the existing concurrent config and exact rollback values',async t=>{
  const f=await fixture(t,'main',{xMode:'concurrent',xReaderSource:'main'}),before=await readFile(f.service.args[1],'utf8');
  assert.equal((await verifyDailyPackage(f.path)).readerSource,'main');
  const plan=JSON.parse(await readFile(join(f.root,'rollback-proposal.json')));
  assert.deepEqual(plan.config.fields.map(row=>row.before),[
    {present:true,value:'concurrent'},{present:true,value:'main'},{present:false}]);
  assert.equal(await readFile(f.service.args[1],'utf8'),before);
  assert.throws(()=>dailyLayout(f.service,f.config),/baseline_config_rejected/);
  for(const capture of [
    {xMode:'concurrent',xReaderSource:'dedicated'}, {xMode:'concurrent'},
    {xMode:'concurrent',xReaderSource:'main',xReaderProfile:'another-profile'},
    {xMode:'unknown',xReaderSource:'main'},
  ])assert.throws(()=>dailyLayout(f.service,{...f.config,capture},'main'),/baseline_config_rejected/);
});

test('sealed main refresh uses pinned previous-package provenance without state or account reads',async t=>{
  const f=await fixture(t,'main',{xMode:'concurrent',xReaderSource:'main'},true);
  const result=await verifyDailyPackage(f.path);
  assert.equal(result.preparationOnly,true);assert.equal(result.activationReady,false);assert.equal(result.sessionTransferred,false);
  const provenance=await dailyRollbackProvenance(f.service,{allowPackaged:true});
  assert.equal(provenance.backup,'rollback/previous-daily-package.json');
  assert.equal((await readFile(provenance.path)).equals(await readFile(join(f.root,provenance.backup))),true);
  await assert.rejects(readFile(f.service.command+'.runtime-state.json'),{code:'ENOENT'});
  await assert.rejects(readFile(f.manifest.ordinary.database),{code:'ENOENT'});
  await assert.rejects(readFile(f.manifest.ordinary.profile),{code:'ENOENT'});
  await assert.rejects(dailyRollbackProvenance(f.service),/rollback_provenance_missing/);
  const row=f.manifest.baseline.find(row=>row.backup===provenance.backup);
  f.manifest.baseline.push({...row});await f.save();
  await assert.rejects(verifyDailyPackage(f.path),/rollback_provenance_missing/);
});

test('previous-package provenance rejects stale or ambiguous code pins even after new inventory is sealed',async t=>{
  for(const [label,mutate,reason] of [
    ['schema',p=>{p.schema='unknown';},/previous_package_contract_rejected/],
    ['identity',p=>{p.packageId='c'.repeat(32);},/previous_package_contract_rejected/],
    ['source commit',p=>{p.sourceCommit='uncommitted';},/previous_package_contract_rejected/],
    ['helper pin',p=>{p.files.find(row=>row.file==='bin/aku-reader-broker.exe').sha256='c'.repeat(64);},/previous_package_payload_changed/],
    ['missing native host',p=>{p.files=p.files.filter(row=>row.file!=='bin/com.akubrowser.reader_activation.json');},/previous_package_inventory_rejected/],
    ['duplicate binary',p=>{p.files.push({...p.files[0]});},/previous_package_inventory_rejected/],
  ])await t.test(label,async t=>{
    const f=await fixture(t,'main',{xMode:'concurrent',xReaderSource:'main'},true);
    const row=f.manifest.baseline.find(row=>row.backup==='rollback/previous-daily-package.json');
    const previous=JSON.parse(await readFile(row.path,'utf8'));mutate(previous);
    await f.write(row.path,previous);await copyFile(row.path,join(f.root,row.backup));
    row.sha256=(await fileRecord(row.path)).sha256;await f.refresh();
    await assert.rejects(verifyDailyPackage(f.path),reason);
  });
});

test('provenance drift, missing metadata and an unsealed executable fail closed',async t=>{
  const legacy=await fixture(t);
  await rm(legacy.service.command+'.runtime-state.json');
  await assert.rejects(dailyRollbackProvenance(legacy.service),/rollback_provenance_missing/);
  const f=await fixture(t,'main',{xMode:'concurrent',xReaderSource:'main'},true);
  await f.write(f.service.command+'.runtime-state.json','unexpected new provenance');
  await assert.rejects(verifyDailyPackage(f.path),/rollback_provenance_changed/);
  await assert.rejects(dailyRollbackProvenance({...f.service,command:join(f.main,'arbitrary','aku-sidecar.exe')},{allowPackaged:true}),/path_escape/);
  await rm(f.service.command+'.runtime-state.json');
  await writeFile(f.service.command,'changed old executable');
  await assert.rejects(dailyRollbackProvenance(f.service,{allowPackaged:true}),/previous_package_payload_changed/);
});
test('main verifier rejects wrong entry, lifecycle, driver pin and missing transport even after inventory refresh',async t=>{
  for(const [label,mutate,reason] of [
    ['entry',owner=>{owner.entry='internal/collection/headless/worker/native-pilot/daily-owner.mjs';},/owner_pin_rejected/],
    ['lifecycle',owner=>{delete owner.mainLifecycle;owner.dailyLifecycle=true;},/owner_pin_rejected/],
    ['mixed',owner=>{owner.dailyLifecycle=true;},/owner_pin_rejected/],
    ['pin',owner=>{owner.driver.sha256=digest('wrong');},/owner_pin_rejected/],
    ['driver',owner=>{owner.driver.file='../x-daily-owner.cjs';},/owner_pin_rejected/],
  ])await t.test(label,async t=>{
    const f=await fixture(t,'main'),path=join(f.root,'bin/main-browser-transport/manifest.json'),owner=JSON.parse(await readFile(path));
    mutate(owner);await f.write(path,owner);await f.refresh();await assert.rejects(verifyDailyPackage(f.path),reason);
  });
  const f=await fixture(t,'main');f.manifest.files=f.manifest.files.filter(row=>row.file!=='bin/main-browser-transport/main-browser-transport.cjs');
  await f.save();await assert.rejects(verifyDailyPackage(f.path),/required_payload_missing/);
});
test('main rollback, config, prerequisites and runtime authority cannot be silently relaxed',async t=>{
  for(const field of ['executionApproved','activationReady','processConsentGranted'])await t.test(field,async t=>{
    const f=await fixture(t,'main');f.manifest[field]=true;await f.save();await assert.rejects(verifyDailyPackage(f.path),/contract_rejected/);
  });
  for(const [label,file,mutate,reason] of [
    ['reader source','config-proposal.json',p=>{p.changes[1].value='dedicated';},/config_proposal_changed/],
    ['graceful shutdown','rollback-proposal.json',p=>{p.shutdown='force-tree';},/rollback_proposal_changed/],
    ['switch receipt','rollback-proposal.json',p=>{p.switchReceiptRequired=false;},/rollback_proposal_changed/],
    ['database preservation','rollback-proposal.json',p=>{p.restoreDatabaseSnapshot=true;},/rollback_proposal_changed/],
    ['provider preservation','service-proposal.json',p=>{p.args.push('--provider=other');},/service_proposal_changed/],
  ])await t.test(label,async t=>{
    const f=await fixture(t,'main'),path=join(f.root,file),p=JSON.parse(await readFile(path));
    mutate(p);await f.write(path,p);await f.refresh();await assert.rejects(verifyDailyPackage(f.path),reason);
  });
  const f=await fixture(t,'main');f.manifest.prerequisites=f.manifest.prerequisites.filter(v=>v!=='review_non_x_headless_conflicts_and_quiet_unavailability');
  await f.save();await assert.rejects(verifyDailyPackage(f.path),/main_prerequisites_changed/);
});
test('main verification rejects source relabeling, stale config and missing rollback plan',async t=>{
  const f=await fixture(t,'main');f.manifest.readerSource='dedicated';await f.save();await assert.rejects(verifyDailyPackage(f.path),/contract_rejected/);
  f.manifest.readerSource='main';f.manifest.reader.ownershipVerified=true;await f.save();await assert.rejects(verifyDailyPackage(f.path),/reader_prerequisite/);
  f.manifest.reader.ownershipVerified=false;await f.save();
  await f.write(f.service.args[1],{...f.config,preference:{mode:'changed'}});
  await assert.rejects(verifyDailyPackage(f.path),/main_baseline_stale/);
  await copyFile(join(f.root,'rollback/sidecar.json'),f.service.args[1]);
  f.manifest.files=f.manifest.files.filter(row=>row.file!=='rollback-proposal.json');await f.save();
  await assert.rejects(verifyDailyPackage(f.path),/required_payload_missing/);
});
test('v1 dedicated cannot be relabeled as main without a new package contract',async t=>{
  const f=await fixture(t);f.manifest.readerSource='main';await f.save();
  await assert.rejects(verifyDailyPackage(f.path),/contract_rejected/);
});
test('main verifier accepts the actual sealed transport builder without launching any packaged executable',async t=>{
  const f=await fixture(t,'main'),bundle=await buildMainBrowserTransportPackage(join(f.root,'bin/main-browser-transport'));
  assert.ok(bundle.moduleCount>0);assert.ok(bundle.assetCount>0);
  await f.refresh();assert.equal((await verifyDailyPackage(f.path)).readerSource,'main');
  await writeFile(bundle.driver,'tampered sealed transport');await f.refresh();
  await assert.rejects(verifyDailyPackage(f.path),/owner_pin_rejected/);
});
