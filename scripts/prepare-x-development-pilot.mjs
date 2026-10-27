// Prepare a reviewable candidate only. Never launch Chrome, invoke manual start,
// register a host, inspect an account profile or open any database.
import {mkdir,readFile,writeFile,copyFile,lstat} from 'node:fs/promises';
import {spawnSync} from 'node:child_process';
import {randomUUID} from 'node:crypto';
import {dirname,join,resolve} from 'node:path';
import {fileURLToPath} from 'node:url';
import {createServer} from 'node:net';
import {buildNativeOwnerPackage} from './build-x-native-owner.mjs';
import {ordinary,digest,fileRecord,copyTree,inventory,instanceConfig,activationArguments,verifyPackage} from './x-development-pilot-package.mjs';

const project=resolve(dirname(fileURLToPath(import.meta.url)),'..');
if(process.platform!=='win32'||process.argv.length!==2)throw new Error('Windows preparation takes no arguments');
const id=randomUUID().replaceAll('-',''),parent=join(project,'build','x-application-qa-prepared');
await mkdir(parent,{recursive:true});await ordinary(parent,true);
const root=join(parent,id),bin=join(root,'bin'),rollback=join(root,'rollback'),temp=join(root,'tmp');
for(const directory of [root,bin,rollback,temp])await mkdir(directory);
const env={...process.env,TEMP:temp,TMP:temp,GOTMPDIR:temp,GOCACHE:join(project,'.go-build','x-development-entry'),
  GOMODCACHE:join(project,'.go-build','mod'),GOWORK:'off',GOPROXY:'off',GOSUMDB:'off',GOTOOLCHAIN:'local',CGO_ENABLED:'0',GOFLAGS:'',
  GIT_TERMINAL_PROMPT:'0',GCM_INTERACTIVE:'0'};
function command(file,args){const r=spawnSync(file,args,{cwd:project,env,encoding:'utf8',windowsHide:true,maxBuffer:1024*1024});
  if(r.error||r.status!==0)throw new Error('package_preparation_command_failed: '+file+' '+args[0]+' exit='+(r.status??'unavailable')+' '+(r.stderr??'').slice(0,800));return r.stdout.trim();}
const sourceCommit=command('git',['rev-parse','HEAD']);
if(command('git',['status','--porcelain','--untracked-files=all','--','cmd','internal','ui-reader-broker','go.mod','go.sum'])!=='')throw new Error('compiled_source_must_match_reviewed_commit');
const configPath=join(project,'config','sidecar.json'),originalBytes=await readFile(await ordinary(configPath));
const original=JSON.parse(originalBytes),baseline=[];
for(const [source,name] of [[configPath,'sidecar.json'],[join(project,'runtime','dev','aku-sidecar.exe'),'aku-sidecar.exe'],
  [join(project,'runtime','dev','aku-reader-broker.exe'),'aku-reader-broker.exe'],
  [join(project,'runtime','dev','aku-sidecar.exe.runtime-state.json'),'aku-sidecar.exe.runtime-state.json']]){
  const record=await fileRecord(source);await copyFile(source,join(rollback,name));baseline.push({...record,backup:join('rollback',name)});
}
const image=join(bin,'aku-sidecar.exe');
command('go',['build','-tags','auth_session_dev','-o',image,'./cmd/akusidecar']);
command('go',['build','-ldflags','-H=windowsgui','-o',join(bin,'aku-reader-broker.exe'),'./cmd/aku-reader-broker']);
// Copy only the owned Node distribution files, not any browser profile/worker state.
const nodeRoot=join(bin,'node');await mkdir(nodeRoot);
for(const name of ['node.exe','node.pin.json','LICENSE'])await copyFile(await ordinary(join(project,'runtime','dev','headless-worker',name)),join(nodeRoot,name));
const node=join(nodeRoot,'node.exe');
const nodePin=JSON.parse(await readFile(join(nodeRoot,'node.pin.json'),'utf8'));
if((await fileRecord(node)).sha256!==nodePin.nodeSha256||
  (await fileRecord(join(nodeRoot,'LICENSE'))).sha256!==nodePin.licenseSha256)throw new Error('packaged_node_pin_rejected');
const uiRoot=join(project,'runtime','chromium'),uiPin=JSON.parse(await readFile(join(uiRoot,'pin.json'),'utf8'));
if(uiPin.component!=='AkuBrowserAppShellChromium'||uiPin.platform!=='win64'||uiPin.channel!=='stable'||
  !/^\d+\.\d+\.\d+\.\d+$/.test(uiPin.version)||uiPin.sourceUrl!==`https://storage.googleapis.com/chrome-for-testing-public/${uiPin.version}/win64/chrome-win64.zip`||
  (await fileRecord(join(uiRoot,'bin','chrome.exe'))).sha256!==uiPin.executableSha256)throw new Error('pinned_ui_chromium_unavailable');
await copyTree(uiRoot,join(bin,'chromium'));
const bundle=await buildNativeOwnerPackage(join(root,'owner'));
const chrome='C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe';
const assets={node:await fileRecord(node),chrome:await fileRecord(chrome),driver:await fileRecord(bundle.driver)};
const port=await new Promise((accept,reject)=>{const server=createServer();server.once('error',reject);server.listen(0,'127.0.0.1',()=>{const port=server.address().port;server.close(error=>error?reject(error):accept(port));});});
const request=join(root,'materialize-request.json');
await writeFile(request,JSON.stringify({schema:'aku.x.application-qa-materialize.v1',parent:root,port,...assets})+'\n',{flag:'wx'});
// The preparation-only exit validates assets/layout without opening config,
// databases or the retained profile. It cannot reach the native start selector.
const receipt=JSON.parse(command(image,['--dev','--dev-qa-pilot-prepare='+request]));
if(receipt.event!=='qa_plan_prepared'||receipt.executionApproved!==false||receipt.browserStarted!==false||receipt.retainedProfileInspected!==false)throw new Error('package_preparation_receipt_rejected');
const plan=JSON.parse(await readFile(receipt.plan,'utf8'));
const configDirectory=join(plan.root,'config');await mkdir(configDirectory);
const instanceConfigPath=join(configDirectory,'sidecar.json');
await writeFile(instanceConfigPath,JSON.stringify(instanceConfig(original,plan,project),null,2)+'\n',{flag:'wx'});
// This existing early probe validates complete ordinary config and exits before
// Store, provider, app-shell or native selection. No opt-in selector is passed.
const configProbe=JSON.parse(command(image,['--dev','--config='+instanceConfigPath,'--database='+plan.database,'--port='+plan.port,
  '--runtime-candidate-probe','--runtime-candidate-probe-schema=2']));
if(configProbe.status!=='ok'||configProbe.configVersion!==1||!Number.isSafeInteger(configProbe.databaseSchemaVersion))throw new Error('package_config_probe_rejected');
const nativeManifest=join(bin,'com.akubrowser.reader_activation.json');
await writeFile(nativeManifest,JSON.stringify({name:'com.akubrowser.reader_activation',description:'Explicit Windows AkuBrowser reader activation',
  path:join(bin,'aku-reader-broker.exe'),type:'stdio',allowed_origins:['chrome-extension://dlibmmlopdahibfniinemhnghlifiple/']},null,2)+'\n',{flag:'wx'});
await copyFile(join(project,'docs','x-development-pilot-activation.md'),join(root,'ACTIVATION.md'));
const tools=join(root,'tools');await mkdir(tools);
for(const name of ['x-development-pilot-package.mjs','verify-x-development-pilot.mjs'])await copyFile(join(project,'scripts',name),join(tools,name));
const registryScript=`$rows=@();foreach($vendor in @('Google\\Chrome','Chromium')){$key="HKCU:\\Software\\$vendor\\NativeMessagingHosts\\com.akubrowser.reader_activation";try{$present=Test-Path -LiteralPath $key;$value=if($present){[string](Get-Item -LiteralPath $key).GetValue('')}else{''};$rows+=@{vendor=$vendor;status='observed';present=$present;value=$value;view='current-user-default-64'}}catch{$rows+=@{vendor=$vendor;status='unavailable'}}};ConvertTo-Json -InputObject @($rows) -Depth 4`;
const registry=spawnSync('powershell.exe',['-NoProfile','-NonInteractive','-Command',registryScript],{encoding:'utf8',windowsHide:true});
let registration={status:'unavailable',entries:[]};
try{if(!registry.error&&registry.status===0){const entries=JSON.parse(registry.stdout);registration={status:entries.every(row=>row.status==='observed')?'observed':entries.some(row=>row.status==='observed')?'partial':'unavailable',entries};}}catch{}
await writeFile(join(rollback,'native-host-registration.json'),JSON.stringify(registration,null,2)+'\n',{flag:'wx'});
const preflight=join(root,'preflight.ps1');
await writeFile(preflight,`$ErrorActionPreference='Stop'\n# Read-only; never opens the pipe or writes registry.\n$target=Join-Path $PSScriptRoot 'bin\\com.akubrowser.reader_activation.json'\nforeach($vendor in @('Google\\Chrome','Chromium')){\n $key="HKCU:\\Software\\$vendor\\NativeMessagingHosts\\com.akubrowser.reader_activation"\n $state='missing'\n try{if(Test-Path -LiteralPath $key){$current=[string](Get-Item -LiteralPath $key).GetValue('');$state=if($current -eq $target){'matches_candidate'}else{'other_runtime_registered'}}}catch{$state='unavailable'}\n Write-Output "$vendor native_host=$state"\n}\ntry{$busy=[IO.Directory]::GetFiles('\\\\.\\pipe\\') -contains '\\\\.\\pipe\\AkuBrowser.reader-activation.11122';Write-Output "reader_pipe_present=$busy"}catch{Write-Output 'reader_pipe_present=unknown'}\nWrite-Output 'No activation or registration performed.'\n`,{flag:'wx'});
const paths=command('git',['ls-files','--','cmd','internal','ui-reader-broker','go.mod','go.sum']).split(/\r?\n/).filter(Boolean).sort();
const sourceDigest=digest(Buffer.concat(await Promise.all(paths.map(async path=>Buffer.concat([Buffer.from(path+'\0'),Buffer.from(digest(await readFile(join(project,path))),'hex')])))));
const manifest={schema:'aku.x.development-activation-package.v1',packageId:id,sourceCommit,sourceSHA256:sourceDigest,
  buildTags:['auth_session_dev'],preparationOnly:true,executionApproved:false,activationReady:false,defaultOff:true,maxAttemptMS:60000,
  configProbe,
  preparationSources:await Promise.all(['scripts/prepare-x-development-pilot.mjs','scripts/x-development-pilot-package.mjs','docs/x-development-pilot-activation.md'].map(path=>fileRecord(join(project,path)))),
  controls:{manualStart:true,source:'x',maxTransfers:1,cookieNames:['auth_token','ct0'],retry:false},
  plan:{path:receipt.plan,port:plan.port,database:plan.database,chrome:plan.chrome.path},config:await fileRecord(instanceConfigPath),
  modelPolicySHA256:digest(JSON.stringify(original.reasoning)),launch:{image,arguments:activationArguments({path:receipt.plan,port:plan.port,database:plan.database,chrome},instanceConfigPath)},
  mainBefore:{filesOnDiskOnly:true,runningProcessInspected:false,files:baseline,registrationSnapshot:'rollback/native-host-registration.json'},externalAssets:[assets.chrome],
  prerequisites:['fresh_activation_and_one_transfer_approval','candidate_native_host_registration','exclusive_reader_pipe_availability','fresh_port_and_qa_plan','physical_reader_foreground_acceptance'],
  preserved:{mainLogin:true,userDatabase:true,mainRuntime:true,autoUpdate:true,providerModels:true},
  retainedQA:{inspected:false,leaseAcquired:false,sessionRead:false},files:await inventory(root)};
for(const row of manifest.files)delete row.path;
const manifestPath=join(root,'activation-package.json');
await writeFile(manifestPath,JSON.stringify(manifest,null,2)+'\n',{flag:'wx'});
for(const row of baseline){if((await fileRecord(row.path)).sha256!==row.sha256)throw new Error('main_files_changed_during_preparation');}
try{await lstat(plan.database);throw new Error('preparation_created_database');}catch(error){if(error.code!=='ENOENT')throw error;}
const verified=await verifyPackage(manifestPath);
process.stdout.write(JSON.stringify({manifest:manifestPath,...verified,activationReady:false,mainFilesUnchanged:true,browserStarted:false,sessionTransferred:false})+'\n');
