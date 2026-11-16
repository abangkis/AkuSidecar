// Retain only verified development-package rollback executables.
// Importing this module never reads runtime state or performs cleanup.
import fs from 'node:fs';
import {createHash,randomUUID} from 'node:crypto';
import {spawn} from 'node:child_process';
import {createInterface} from 'node:readline';
import {basename,dirname,join,relative,resolve,isAbsolute} from 'node:path';
import {fileURLToPath} from 'node:url';

const contracts={
  main:{parent:'main-dev-prepared',manifest:'prepared-package.json',schemas:['aku.main-dev-convergence-prepared.v1'],
    executables:['rollback/runtime/aku-sidecar.exe','rollback/runtime/aku-reader-broker.exe','rollback/runtime/headless-worker/node.exe']},
  daily:{parent:'x-daily-development-prepared',manifest:'daily-package.json',
    schemas:['aku.x.daily-development-package.v1','aku.x.daily-development-package.v2'],
    executables:['rollback/aku-sidecar.exe','rollback/aku-reader-broker.exe']},
  pilot:{parent:'x-application-qa-prepared',manifest:'activation-package.json',schemas:['aku.x.development-activation-package.v1'],
    executables:['rollback/aku-sidecar.exe','rollback/aku-reader-broker.exe']},
  qa:{parent:'x-application-qa-prepared',manifest:'prepared-review.json',schemas:['aku.x.application-qa-preparation.v1','aku.x.application-qa-preparation.v2'],executables:[]},
};
const stateSchema='aku.development-package-retention.v1';
const normalize=value=>String(value).replaceAll('/','\\').toLowerCase();
const hash=path=>createHash('sha256').update(fs.readFileSync(path)).digest('hex');
function exists(path){try{fs.lstatSync(path);return true;}catch(error){if(error.code==='ENOENT')return false;throw error;}}
function ordinary(path,directory=false){
  const full=resolve(path);let cursor=full;
  while(true){
    const stat=fs.lstatSync(cursor);
    if(stat.isSymbolicLink()||normalize(fs.realpathSync.native(cursor))!==normalize(cursor))throw Error('retention_reparse_path');
    if(cursor===full&&(directory?!stat.isDirectory():!stat.isFile()))throw Error('retention_path_type');
    const parent=dirname(cursor);if(parent===cursor)break;cursor=parent;
  }
  return full;
}
function inside(root,path){
  const rel=relative(root,resolve(path));
  if(!rel||rel==='..'||rel.startsWith('..\\')||rel.startsWith('../')||isAbsolute(rel))throw Error('retention_outside_package');
  return resolve(path);
}
function readJSON(path){ordinary(path);return JSON.parse(fs.readFileSync(path,'utf8').replace(/^\uFEFF/u,''));}
function writeJSON(path,value){
  const pending=path+'.'+randomUUID()+'.next';
  fs.writeFileSync(pending,JSON.stringify(value,null,2)+'\n',{flag:'wx'});
  try{fs.renameSync(pending,path);}finally{if(exists(pending))fs.unlinkSync(pending);}
}
function packageRoot(project,kind,root){
  const contract=contracts[kind];if(!contract)throw Error('retention_kind_unknown');
  const parent=join(project,'build',contract.parent),full=ordinary(root,true);
  if(normalize(dirname(full))!==normalize(ordinary(parent,true))||!/^[0-9a-f]{32}$/u.test(basename(full)))throw Error('retention_package_root');
  return full;
}
function manifestRecord(project,kind,root){
  root=packageRoot(project,kind,root);const c=contracts[kind],path=join(root,c.manifest),manifest=readJSON(path);
  if(!c.schemas.includes(manifest.schema)||manifest.preparationOnly!==true||
    (kind==='main'?manifest.activationApproved!==false:manifest.executionApproved!==false)||
    (manifest.packageId!==undefined&&manifest.packageId!==basename(root)))throw Error('retention_manifest_contract');
  if(exists(join(root,'retention-retired.json')))throw Error('development_package_retired');
  const rows=manifest.files??[],executables=[];
  for(const rel of c.executables){
    const matches=rows.filter(row=>(row.file??row.path)?.replaceAll('\\','/')===rel);
    if(matches.length!==1||!/^[0-9a-f]{64}$/iu.test(matches[0].sha256))throw Error('retention_inventory_missing');
    const file=ordinary(inside(root,join(root,rel)));
    if(hash(file)!==matches[0].sha256.toLowerCase())throw Error('retention_inventory_changed');
    executables.push({file:rel,sha256:matches[0].sha256.toLowerCase(),bytes:fs.statSync(file).size});
  }
  return{kind,root,manifest:c.manifest,manifestSha256:hash(path),executables};
}
function mentions(text,root){
  const value=normalize(text),needle=normalize(root);let at=value.indexOf(needle);
  while(at>=0){
    const tail=value[at+needle.length];
    if(tail===undefined||/[\\\s"'\x60,;)\]}]/u.test(tail))return true;
    at=value.indexOf(needle,at+needle.length);
  }
  return false;
}
function qaUncertain(record){
  if(record.kind!=='qa'&&record.kind!=='pilot')return false;
  if(!exists(join(record.root,'execution.claim'))){
    const visit=dir=>fs.readdirSync(dir,{withFileTypes:true}).some(entry=>{
      const path=join(dir,entry.name);
      if(entry.isSymbolicLink())return true;
      if(entry.isDirectory())return (/profile|session|database|data|state/iu.test(entry.name)?fs.readdirSync(path).length>0:(!['bin','tools','rollback','tmp','owner'].includes(entry.name)&&visit(path)));
      return /(?:\.db(?:-wal|-shm)?$|\.sqlite(?:3)?$|lease|fence|execution\.claim)/iu.test(entry.name);
    });
    return visit(record.root);
  }
  const path=join(record.root,'execution-result.json');
  if(!exists(path))return true;
  const result=readJSON(path);
  return result.passed!==true||result.cleanup!=='verified';
}
async function runtimeGuard(project){
  if(process.platform!=='win32')throw Error('retention_native_observation_requires_windows');
  const script=join(dirname(fileURLToPath(import.meta.url)),'development-package-retention-guard.ps1');
  const child=spawn('powershell.exe',['-NoProfile','-NonInteractive','-File',script,'-Repository',project],{windowsHide:true,stdio:['pipe','pipe','pipe']});
  const lines=createInterface({input:child.stdout}),queue=[],waiting=[];let closed=false,failure='';
  const deliver=value=>waiting.length?waiting.shift().resolve(value):queue.push(value);
  lines.on('line',line=>{try{deliver(JSON.parse(line));}catch{failure='retention_observation_invalid';}});
  child.stderr.on('data',()=>{failure='retention_runtime_guard_failed';});
  child.stdin.on('error',()=>{failure='retention_runtime_guard_closed';});
  child.on('error',()=>{failure='retention_runtime_guard_unavailable';closed=true;while(waiting.length)waiting.shift().reject(Error(failure));});
  const exited=new Promise(resolveExit=>child.on('close',()=>{closed=true;while(waiting.length)waiting.shift().reject(Error(failure||'retention_runtime_guard_closed'));resolveExit();}));
  const receive=()=>queue.length?Promise.resolve(queue.shift()):closed?Promise.reject(Error(failure||'retention_runtime_guard_closed')):new Promise((resolve,reject)=>waiting.push({resolve,reject}));
  try{
    const ready=await receive();if(ready.event!=='ready')throw Error('retention_runtime_guard_not_ready');
    return{
      async snapshot(){if(closed||failure)throw Error(failure||'retention_runtime_guard_closed');child.stdin.write('snapshot\n');const snapshot=await receive();
        if(snapshot.event!=='snapshot'||snapshot.complete!==true||snapshot.processCount<50||!Array.isArray(snapshot.references))throw Error('retention_observation_incomplete');
        return snapshot.references;},
      async checkFiles(files){child.stdin.write(JSON.stringify({event:'preflight',files})+'\n');const result=await receive();if(result.event!=='preflight'||result.complete!==true)throw Error('retention_handle_preflight_failed');},
      async close(){if(!closed)child.stdin.end('release\n');await exited;},
    };
  }catch(error){child.stdin.end();await exited;throw error;}
}
function dependencies(record){
  const result=[];
  for(const name of [record.manifest,'baseline.json','rollback/service.json','rollback/previous-daily-package.json','activation-receipt.json','latest-handoff.json']){
    const path=join(record.root,name);if(exists(path)){ordinary(path);const collect=value=>{if(typeof value==='string')result.push(value);else if(value&&typeof value==='object')for(const child of Object.values(value))collect(child);};collect(readJSON(path));}
  }
  return result;
}
export function beginDevelopmentPreparation(project,{guard=runtimeGuard}={}){
  project=ordinary(project,true);
  const build=join(project,'build');if(!exists(build))fs.mkdirSync(build);ordinary(build,true);
  const directory=join(build,'.development-package-retention');if(!exists(directory))fs.mkdirSync(directory);ordinary(directory,true);
  const lock=join(directory,'operation.lock'),token=randomUUID(),fd=fs.openSync(lock,'wx');
  fs.writeFileSync(fd,JSON.stringify({pid:process.pid,token})+'\n');fs.closeSync(fd);
  const statePath=join(directory,'state.json');
  let released=false,verified=false;
  const state=()=>{const value=exists(statePath)?readJSON(statePath):{schema:stateSchema,sequence:0,packages:[]};
    if(value.schema!==stateSchema||!Number.isSafeInteger(value.sequence)||!Array.isArray(value.packages))throw Error('retention_state_invalid');return value;};
  const release=()=>{if(released)return;if(readJSON(lock).token!==token)throw Error('retention_lock_changed');fs.unlinkSync(lock);released=true;};
  // Refuse another build before copying rollback files when state needs repair.
  try{state();}catch(error){release();throw error;}
  return{
    release,
    async verified({kind,root,verification}){
      if(released||verified)throw Error('retention_operation_closed');
      if(!verification||verification.preparationOnly!==true||verification.packageId!==basename(resolve(root)))throw Error('retention_verification_missing');
      const record=manifestRecord(project,kind,root),data=state();
      if(data.packages.some(row=>normalize(row.root)===normalize(record.root)))throw Error('retention_package_already_recorded');
      const current={...record,sequence:++data.sequence,status:'verified'};
      data.packages.push(current);writeJSON(statePath,data);verified=true;
      const result={schema:'aku.development-package-retention-result.v1',verifiedRoot:current.root,removed:[],retained:[],deferred:[]};
      let native;
      try{
        native=await guard(project);const refs=await native.snapshot();
        const keep=new Set(),latest=new Map();
        for(const row of data.packages){
          if(row.status!=='verified')continue;
          packageRoot(project,row.kind,row.root);
          if(!latest.has(row.kind)||latest.get(row.kind).sequence<row.sequence)latest.set(row.kind,row);
          if(exists(join(row.root,'retention-pin.json'))||qaUncertain(row)||refs.some(value=>mentions(value,row.root)))keep.add(row.root);
        }
        for(const row of latest.values())keep.add(row.root);
        const pointer=join(build,'main-dev-prepared','latest.json');
        if(exists(pointer)){const value=readJSON(pointer);if(typeof value.root!=='string')throw Error('retention_latest_pointer_invalid');keep.add(packageRoot(project,'main',value.root));}
        let changed=true;
        while(changed){changed=false;for(const root of [...keep]){
          const row=data.packages.find(row=>normalize(row.root)===normalize(root));if(!row)continue;
          const texts=dependencies(row);
          for(const candidate of data.packages)if(candidate.status==='verified'&&!keep.has(candidate.root)&&texts.some(text=>mentions(text,candidate.root))){keep.add(candidate.root);changed=true;}
        }}
        for(const row of data.packages){
          if(row.status!=='verified'||!row.executables.length)continue;
          if(keep.has(row.root)){result.retained.push(row.root);continue;}
          try{
            const checked=manifestRecord(project,row.kind,row.root);
            if(checked.manifestSha256!==row.manifestSha256||JSON.stringify(checked.executables)!==JSON.stringify(row.executables))throw Error('retention_record_changed');
            let live=await native.snapshot();
            if(live.some(value=>mentions(value,row.root))||exists(join(row.root,'retention-pin.json'))||qaUncertain(row))throw Error('retention_package_in_use');
            if(native.checkFiles)await native.checkFiles(row.executables.map(exe=>ordinary(inside(row.root,join(row.root,exe.file)))));
            row.status='retiring';writeJSON(statePath,data);
            const marker=join(row.root,'retention-retired.json');
            fs.writeFileSync(marker,JSON.stringify({schema:'aku.development-package-retired.v1',manifestSha256:row.manifestSha256,
              replacement:current.root,reason:'superseded_verified_preparation',executables:row.executables},null,2)+'\n',{flag:'wx'});
            for(const exe of row.executables){
              live=await native.snapshot();if(live.some(value=>mentions(value,row.root)))throw Error('retention_process_started');
              const file=ordinary(inside(row.root,join(row.root,exe.file)));
              if(!contracts[row.kind].executables.includes(exe.file)||hash(file)!==exe.sha256||fs.statSync(file).size!==exe.bytes)throw Error('retention_executable_changed');
              fs.unlinkSync(file);result.removed.push({path:file,bytes:exe.bytes});
            }
            row.status='retired';writeJSON(statePath,data);
          }catch(error){result.deferred.push({root:row.root,reason:error.message});}
        }
      }catch(error){result.deferred.push({reason:error.message});}
      finally{if(native)await native.close();}
      writeJSON(join(directory,'last-result.json'),result);
      return result;
    },
  };
}

