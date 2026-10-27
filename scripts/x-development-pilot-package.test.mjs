import test from 'node:test';
import assert from 'node:assert/strict';
import {mkdir,mkdtemp,writeFile,readFile,rm} from 'node:fs/promises';
import {dirname,join,resolve} from 'node:path';
import {fileURLToPath} from 'node:url';
import {digest,contained,ordinary,instanceConfig,activationArguments,inventory,verifyPackage} from './x-development-pilot-package.mjs';
const project=resolve(dirname(fileURLToPath(import.meta.url)),'..');
async function fixture(t){
  const parent=join(project,'build');await mkdir(parent,{recursive:true});
  const root=await mkdtemp(join(parent,'test-x-package-'));
  t.after(async()=>{if(dirname(resolve(root))!==parent)throw new Error('unsafe fixture cleanup');await rm(root,{recursive:true,force:true});});
  const configPath=join(root,'config.json'),planPath=join(root,'plan.json'),image=join(root,'main.exe');
  const config={version:1,reasoning:{activeProvider:'synthetic',providers:{synthetic:{model:'keep-this'}}},capture:{visibility:'quiet'},server:{host:'127.0.0.1',port:28173},database:{path:join(root,'fresh.db')}};
  await writeFile(configPath,JSON.stringify(config));await writeFile(planPath,'{"synthetic":true}');await writeFile(image,'synthetic executable; never run');
  const plan={path:planPath,port:28173,database:config.database.path,chrome:'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'};
  const manifest={schema:'aku.x.development-activation-package.v1',packageId:'synthetic',sourceCommit:'synthetic',preparationOnly:true,executionApproved:false,defaultOff:true,maxAttemptMS:60000,
    controls:{retry:false,maxTransfers:1,cookieNames:['auth_token','ct0']},plan,config:{path:configPath},modelPolicySHA256:digest(JSON.stringify(config.reasoning)),externalAssets:[],
    launch:{image,arguments:activationArguments(plan,configPath)},files:await inventory(root)};
  const path=join(root,'activation-package.json');
  const save=()=>writeFile(path,JSON.stringify(manifest));await save();return{root,path,config,manifest,save};
}
test('isolated config preserves models, assurance, pacing and original settings',()=>{
  const original={version:1,reasoning:{activeProvider:'same',providers:{same:{modelId:'unchanged',assurance:'provider_strict'}}},capture:{behavior:{enabled:true}},mediaProvenance:{c2paToolPath:'tools/c2pa.exe'}};
  const before=JSON.stringify(original),plan={port:28173,database:'synthetic-db'};
  const copy=instanceConfig(original,plan,project);
  assert.deepEqual(copy.reasoning,original.reasoning);assert.deepEqual(copy.capture,original.capture);
  assert.equal(copy.database.path,plan.database);assert.equal(copy.server.host,'127.0.0.1');assert.equal(JSON.stringify(original),before);
  assert.deepEqual(copy.bridge.trustedExtensionOrigins,['chrome-extension://dlibmmlopdahibfniinemhnghlifiple']);
  assert.equal(copy.mediaProvenance.c2paToolPath,resolve(project,'tools/c2pa.exe'));
});
test('isolated UI origin replaces an unrelated main-launch extension override',()=>{
  const original={version:1,reasoning:{activeProvider:'unchanged'},capture:{},bridge:{trustedExtensionOrigins:['chrome-extension://unrelated']}};
  const cfg=instanceConfig(original,{port:28173,database:'synthetic-db'},project);
  assert.deepEqual(cfg.bridge.trustedExtensionOrigins,['chrome-extension://dlibmmlopdahibfniinemhnghlifiple']);
  assert.deepEqual(original.bridge.trustedExtensionOrigins,['chrome-extension://unrelated']);
});
test('launch is ordinary development entry, not QA stdin or implicit start',()=>{
  const args=activationArguments({path:'plan',database:'db',port:28173,chrome:'chrome'},'config');
  assert(args.includes('--dev-session-reuse=native-x-manual'));assert(args.includes('--app-shell'));
  assert(!args.some(arg=>arg.includes('start')||arg.includes('transfer')||arg.includes('qa-pilot-plan')));
});
test('path containment rejects root, sibling and outside traversal',()=>{
  const root=join(project,'build','synthetic');
  for(const path of [root,join(root,'..','outside'),dirname(root)])assert.throws(()=>contained(root,path),/escape/);
});
test('verification is read-only and leaves a fresh database absent',async t=>{
  const f=await fixture(t);const before=await readFile(f.path,'utf8');
  const result=await verifyPackage(f.path);assert.equal(result.executionApproved,false);assert.equal(result.databaseOpened,false);
  assert.equal(await readFile(f.path,'utf8'),before);await assert.rejects(ordinary(f.manifest.plan.database),{code:'ENOENT'});
});
test('changed packaged bytes are rejected',async t=>{
  const f=await fixture(t);await writeFile(f.manifest.launch.image,'changed');await assert.rejects(verifyPackage(f.path),/asset_changed/);
});
test('consumed database rejects a second launch preparation',async t=>{
  const f=await fixture(t);await writeFile(f.manifest.plan.database,'synthetic consumed marker');await assert.rejects(verifyPackage(f.path),/database_already_consumed/);
});
test('escaped config is rejected before any outside read',async t=>{
  const f=await fixture(t);f.manifest.config.path=join(f.root,'..','must-not-read.json');await f.save();await assert.rejects(verifyPackage(f.path),/path_escape/);
});
test('default-off and one-use recipe cannot be silently relaxed',async t=>{
  const f=await fixture(t);f.manifest.controls.retry=true;await f.save();await assert.rejects(verifyPackage(f.path),/contract_rejected/);
});
test('unapproved external assets are rejected before read',async t=>{
  const f=await fixture(t);f.manifest.externalAssets=[{path:join(f.root,'..','unknown.db'),sha256:'unknown'}];await f.save();await assert.rejects(verifyPackage(f.path),/external_asset_scope_rejected/);
});
test('launch argument drift and missing required assets reject',async t=>{
  const f=await fixture(t);f.manifest.launch.arguments.push('--browser-profile=outside');await f.save();await assert.rejects(verifyPackage(f.path),/launch_changed/);
  f.manifest.launch.arguments.pop();f.manifest.files=f.manifest.files.filter(row=>row.path!==f.manifest.launch.image);await f.save();await assert.rejects(verifyPackage(f.path),/required_package_asset_missing/);
});
