import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import {join,dirname,resolve,basename} from 'node:path';
import {fileURLToPath} from 'node:url';
import {randomUUID,createHash} from 'node:crypto';
import {beginDevelopmentPreparation} from './development-package-retention.mjs';

const repo=resolve(dirname(fileURLToPath(import.meta.url)),'..');
const testParent=join(repo,'build','development-package-retention-tests');
fs.mkdirSync(testParent,{recursive:true});
const suite=join(testParent,randomUUID());fs.mkdirSync(suite);
test.after(()=>{
  const full=fs.realpathSync.native(suite),parent=fs.realpathSync.native(testParent);
  assert.equal(dirname(full).toLowerCase(),parent.toLowerCase());
  assert.match(basename(full),/^[0-9a-f-]{36}$/u);
  fs.rmSync(full,{recursive:true});
});
const shapes={
 main:['main-dev-prepared','prepared-package.json','aku.main-dev-convergence-prepared.v1',['rollback/runtime/aku-sidecar.exe','rollback/runtime/aku-reader-broker.exe','rollback/runtime/headless-worker/node.exe']],
 daily:['x-daily-development-prepared','daily-package.json','aku.x.daily-development-package.v2',['rollback/aku-sidecar.exe','rollback/aku-reader-broker.exe']],
 pilot:['x-application-qa-prepared','activation-package.json','aku.x.development-activation-package.v1',['rollback/aku-sidecar.exe','rollback/aku-reader-broker.exe']],
 qa:['x-application-qa-prepared','prepared-review.json','aku.x.application-qa-preparation.v2',[]],
};
const digest=path=>createHash('sha256').update(fs.readFileSync(path)).digest('hex');
function write(path,value){fs.mkdirSync(dirname(path),{recursive:true});fs.writeFileSync(path,typeof value==='string'?value:JSON.stringify(value));}
function fixture(){const project=join(suite,randomUUID());fs.mkdirSync(project);fs.mkdirSync(join(project,'runtime','dev'),{recursive:true});return project;}
function pack(project,kind='daily'){
 const [parent,file,schema,executables]=shapes[kind],id=randomUUID().replaceAll('-',''),root=join(project,'build',parent,id);
 fs.mkdirSync(root,{recursive:true});
 const rows=[];
 for(const [i,relative]of executables.entries()){const path=join(root,relative);write(path,'owned executable '+id+' '+i);rows.push({file:relative,sha256:digest(path)});}
 const manifest={schema,packageId:id,preparationOnly:true,...(kind==='main'?{activationApproved:false}:{executionApproved:false}),files:rows};
 if(kind==='main')manifest.files=rows.map(row=>({path:row.file,sha256:row.sha256}));
 write(join(root,file),manifest);
 for(const name of ['source/local-change.go','config/sidecar.json','data/user.db','report.json','run.log','screen.png'])write(join(root,name),'preserve '+id+' '+name);
 // Database fixture lives in a payload path outside rollback. It must never be deleted.
 const preserved=['source/local-change.go','config/sidecar.json','data/user.db','report.json','run.log','screen.png'].map(name=>({path:join(root,name),sha:digest(join(root,name))}));
 return{root,kind,id,manifest,manifestPath:join(root,file),executables:executables.map(path=>join(root,path)),preserved};
}
function observer(read=()=>[]){return async()=>({snapshot:async()=>read(),checkFiles:async()=>{},close:async()=>{}});}
async function verify(project,p,guard=observer()){
 const op=beginDevelopmentPreparation(project,{guard});
 try{return await op.verified({kind:p.kind,root:p.root,verification:{preparationOnly:true,packageId:p.id}});}
 finally{op.release();}
}
function unchanged(p){for(const row of p.preserved)assert.equal(digest(row.path),row.sha);}
function present(p){for(const path of p.executables)assert.ok(fs.existsSync(path));unchanged(p);}
function retired(p){for(const path of p.executables)assert.ok(!fs.existsSync(path));assert.ok(fs.existsSync(join(p.root,'retention-retired.json')));unchanged(p);}

test('keeps newest and retires only older allowlisted executable files',async()=>{
 const project=fixture(),a=pack(project),b=pack(project);await verify(project,a);
 const original=fs.readFileSync(a.manifestPath);const result=await verify(project,b);
 assert.equal(result.removed.length,2);retired(a);present(b);assert.deepEqual(fs.readFileSync(a.manifestPath),original);
});
test('failure before verification never cleans an existing package',async()=>{
 const project=fixture(),a=pack(project);await verify(project,a);const op=beginDevelopmentPreparation(project);op.release();present(a);
});
test('requires matching successful package verification before retirement',async()=>{
 const project=fixture(),a=pack(project),b=pack(project);await verify(project,a);const op=beginDevelopmentPreparation(project,{guard:observer()});
 try{await assert.rejects(op.verified({kind:'daily',root:b.root,verification:{packageId:'wrong',preparationOnly:true}}),/verification_missing/u);}finally{op.release();}present(a);
});
test('active process and stopped-service references protect old packages',async()=>{
 for(const wrap of [root=>'"'+root+'\\bin\\aku-sidecar.exe" --dev',root=>root.replaceAll('\\','/')+'/bin/aku-sidecar.exe']){
  const project=fixture(),a=pack(project),b=pack(project);await verify(project,a);await verify(project,b,observer(()=>[wrap(a.root)]));present(a);present(b);
 }
});
test('an explicit pin preserves a superseded package',async()=>{
 const project=fixture(),a=pack(project),b=pack(project);await verify(project,a);write(join(a.root,'retention-pin.json'),{});await verify(project,b);present(a);
});
test('preserves an older package referenced by the latest prepared package',async()=>{
 const project=fixture(),a=pack(project),b=pack(project);await verify(project,a);
 write(join(b.root,'rollback/service.json'),{command:join(a.root,'bin','aku-sidecar.exe')});
 await verify(project,b);present(a);
});
test('explicit latest pointer protects a main package even when another was verified',async()=>{
 const project=fixture(),a=pack(project,'main'),b=pack(project,'main');await verify(project,a);
 write(join(project,'build','main-dev-prepared','latest.json'),{root:a.root});await verify(project,b);present(a);
});
test('separate schemas in the QA parent never replace the latest pilot',async()=>{
 const project=fixture(),a=pack(project,'pilot'),b=pack(project,'qa');await verify(project,a);await verify(project,b);present(a);
});
test('unresolved QA claim is retained',async()=>{
 const project=fixture(),a=pack(project,'pilot'),b=pack(project,'pilot');await verify(project,a);
 write(join(a.root,'execution.claim'),'claimed');await verify(project,b);present(a);
});
test('failed QA and uncertain cleanup are retained',async()=>{
 for(const result of [{passed:true,cleanup:'unknown'},{passed:false,cleanup:'verified'}]){
  const project=fixture(),a=pack(project,'pilot'),b=pack(project,'pilot');await verify(project,a);
  write(join(a.root,'execution.claim'),'claimed');write(join(a.root,'execution-result.json'),result);
  await verify(project,b);present(a);
 }
});
test('successful QA with verified cleanup can be superseded',async()=>{
 const project=fixture(),a=pack(project,'pilot'),b=pack(project,'pilot');
 // Remove the synthetic data directory so this fixture is an untouched preparation.
 const dataPath=fs.realpathSync.native(join(a.root,'data'));
 assert.ok(dataPath.toLowerCase().startsWith(fs.realpathSync.native(suite).toLowerCase()+'\\'));
 fs.rmSync(dataPath,{recursive:true});a.preserved=a.preserved.filter(row=>!row.path.includes('\\data\\'));
 await verify(project,a);write(join(a.root,'execution.claim'),'claimed');write(join(a.root,'execution-result.json'),{passed:true,cleanup:'verified'});
 await verify(project,b);retired(a);
});
test('legacy and manually pruned packages are never implicitly enrolled',async()=>{
 const project=fixture(),legacy=pack(project),a=pack(project),b=pack(project);fs.unlinkSync(legacy.executables[0]);
 await verify(project,a);await verify(project,b);assert.ok(fs.existsSync(legacy.executables[1]));retired(a);
});
test('modified manifest or executable is preserved and reported',async()=>{
 for(const mutate of [p=>fs.appendFileSync(p.executables[0],'local change'),p=>write(p.manifestPath,{...p.manifest,note:'local change'})]){
  const project=fixture(),a=pack(project),b=pack(project);await verify(project,a);mutate(a);
  const result=await verify(project,b);assert.ok(result.deferred.length);present(a);
 }
});
test('a process starting after planning prevents deletion',async()=>{
 const project=fixture(),a=pack(project),b=pack(project);await verify(project,a);let n=0;
 const result=await verify(project,b,observer(()=>++n===1?[]:[a.root]));
 assert.ok(result.deferred.length);present(a);
});
test('failed native observation defers all cleanup',async()=>{
 const project=fixture(),a=pack(project),b=pack(project);await verify(project,a);
 const result=await verify(project,b,async()=>{throw Error('CIM unavailable');});assert.ok(result.deferred.length);present(a);
});
test('concurrent preparations cannot acquire the same lock',()=>{
 const project=fixture(),op=beginDevelopmentPreparation(project);
 try{assert.throws(()=>beginDevelopmentPreparation(project),/EEXIST/u);}finally{op.release();}
 const next=beginDevelopmentPreparation(project);next.release();
});
test('malformed state refuses cleanup and preserves artifacts',async()=>{
 const project=fixture(),a=pack(project),b=pack(project);await verify(project,a);write(join(project,'build','.development-package-retention','state.json'),{schema:'unknown'});
 await assert.rejects(verify(project,b),/state_invalid/u);present(a);
});
test('outside-package and reparse destinations are rejected',async()=>{
 const project=fixture(),a=pack(project);const outside=fixture();
 const link=join(project,'build','x-daily-development-prepared',randomUUID().replaceAll('-',''));fs.symlinkSync(outside,link,'junction');
 const op=beginDevelopmentPreparation(project,{guard:observer()});
 try{await assert.rejects(op.verified({kind:'daily',root:link,verification:{preparationOnly:true,packageId:basename(link)}}),/reparse/u);}
 finally{op.release();}
 present(a);assert.ok(fs.existsSync(outside));
});
test('native guard, IPC, handle preflight and mutex work on a disposable runtime', {skip:process.platform!=='win32'||process.env.AKU_RETENTION_NATIVE_TEST!=='1'},async()=>{
 const project=fixture(),a=pack(project),b=pack(project);
 for(const p of [a,b]){
  const op=beginDevelopmentPreparation(project);
  try{const result=await op.verified({kind:p.kind,root:p.root,verification:{preparationOnly:true,packageId:p.id}});assert.deepEqual(result.deferred,[]);}
  finally{op.release();}
 }
 retired(a);present(b);
});
test('all preparation and activation hooks run only after their verification gates',()=>{
 const preparation=['convergence-release-tools','ig-bootstrap-release-tools','ig-corroboration-release-tools','ig-layered-release-tools','ig-read-release-tools','pending-diagnostics-release-tools'];
 for(const name of preparation){
  const text=fs.readFileSync(join(repo,'build',name,'prepare.mjs'),'utf8');
  assert.ok(text.indexOf('const retentionVerification=await verifyPrepared(root)')<text.indexOf('retentionOperation.verified'));
  assert.match(text,/finally \{ retentionOperation\.release\(\); \}/u);
 }
 for(const name of ['prepare-x-daily-development.mjs','prepare-x-development-pilot.mjs','prepare-x-application-qa.mjs']){
  const text=fs.readFileSync(join(repo,'scripts',name),'utf8');assert.match(text,/finally \{ retentionOperation\.release\(\); \}/u);
  const gate=name.includes('daily')?'const verified=await verifyDailyPackage':name.includes('pilot')?'const verified=await verifyPackage':'const planPath=';
  assert.ok(text.indexOf(gate)<text.indexOf('retentionOperation.verified'));
 }
 for(const name of ['convergence-handoff-tools',...preparation.slice(1)]){
  const text=fs.readFileSync(join(repo,'build',name,'activate.ps1'),'utf8');
  const at=text.indexOf('\nComplete-DevelopmentPackageHandoff');
  assert.ok(at>text.indexOf("$receipt=@{status='activated'"));assert.ok(at>text.indexOf('activation-receipt.json'));
  assert.ok(text.indexOf('Enter-DevelopmentPackageHandoff')<text.indexOf('Invoke-DevRuntimePromotion'));
  assert.match(text,/finally \{\s*try \{ \$packageOperation\.ReleaseMutex\(\)/u);
 }
});

import {verifyDailyPackage} from './x-daily-development-package.mjs';
import {verifyPackage} from './x-development-pilot-package.mjs';
import {pathToFileURL} from 'node:url';

test('retired daily, pilot and main packages are rejected before a rollback read',async()=>{
 for(const [kind,check]of [['daily',verifyDailyPackage],['pilot',verifyPackage]]){
  const project=fixture(),p=pack(project,kind);
  write(join(p.root,'retention-retired.json'),{schema:'aku.development-package-retired.v1'});
  await assert.rejects(check(p.manifestPath),/development_package_retired/u);present(p);
 }
 const project=fixture(),p=pack(project,'main');
 write(join(p.root,'retention-retired.json'),{schema:'aku.development-package-retired.v1'});
 for(const name of ['convergence-release-tools','ig-bootstrap-release-tools','ig-corroboration-release-tools','ig-layered-release-tools','ig-read-release-tools','pending-diagnostics-release-tools']){
  const {verifyPrepared}=await import(pathToFileURL(join(repo,'build',name,'verify.mjs')));
  await assert.rejects(verifyPrepared(p.root),/development_package_retired/u);
 }
 present(p);
});
test('locked executable preflight defers without deleting any rollback file',async()=>{
 const project=fixture(),a=pack(project),b=pack(project);await verify(project,a);
 const result=await verify(project,b,async()=>({snapshot:async()=>[],checkFiles:async()=>{throw Error('file in use');},close:async()=>{}}));
 assert.ok(result.deferred.some(row=>row.reason==='file in use'));present(a);present(b);
});
test('foreign manifest paths and unlisted source binaries are never selected',async()=>{
 const project=fixture(),a=pack(project),b=pack(project);
 write(join(a.root,'source','keep.exe'),'source fixture');
 await verify(project,a);await verify(project,b);retired(a);assert.equal(fs.readFileSync(join(a.root,'source','keep.exe'),'utf8'),'source fixture');
});

