// Build a sealed daily-main candidate and rollback evidence only. Never apply
// config/service/registry changes, start Chrome or open a database/profile.
import {mkdir,readFile,writeFile,copyFile,lstat} from 'node:fs/promises';
import {spawnSync} from 'node:child_process';
import {randomUUID} from 'node:crypto';
import {dirname,join,resolve} from 'node:path';
import {fileURLToPath} from 'node:url';
import {buildNativeDailyOwnerPackage} from './build-x-native-owner.mjs';
import {ordinary,digest,fileRecord,copyTree,inventory} from './x-development-pilot-package.mjs';
import {dailyPackageSchema,dailyLayout,dailyConfigProposal,dailyServiceProposal,verifyDailyPackage} from './x-daily-development-package.mjs';
import {sourceAssets} from '../internal/collection/headless/worker/worker.mjs';

const project=resolve(dirname(fileURLToPath(import.meta.url)),'..');
const args=process.argv.slice(2);
if(process.platform!=='win32'||args.length!==1||!args[0].startsWith('--supervisor-config='))
  throw new Error('usage: node scripts/prepare-x-daily-development.mjs --supervisor-config=<absolute-services.json>');
const supervisorConfig=await ordinary(args[0].slice('--supervisor-config='.length));
const supervisorBytes=await readFile(supervisorConfig),supervisor=JSON.parse(supervisorBytes);
const service=supervisor.services?.akusidecar;
if(!service?.args)throw new Error('ordinary_akusidecar_service_missing');
// Resolve the service's exact --config, without reading any account/profile.
const {argumentValue}=await import('./x-daily-development-package.mjs');
const configPath=argumentValue(service.args,'--config');
const originalBytes=await readFile(await ordinary(configPath)),original=JSON.parse(originalBytes);
const layout=dailyLayout(service,original);
if(layout.root!==project)throw new Error('daily_preparation_requires_current_main_checkout');
if(Object.keys(service.environment??{}).length!==0)throw new Error('service_environment_requires_separate_review');
const id=randomUUID().replaceAll('-',''),parent=join(project,'build','x-daily-development-prepared');
await mkdir(parent,{recursive:true});await ordinary(parent,true);
const root=join(parent,id),bin=join(root,'bin'),rollback=join(root,'rollback'),temp=join(root,'tmp');
for(const directory of [root,bin,rollback,temp])await mkdir(directory);
const env={...process.env,TEMP:temp,TMP:temp,GOTMPDIR:temp,GOCACHE:join(project,'.go-build','x-development-entry'),
  GOMODCACHE:join(project,'.go-build','mod'),GOWORK:'off',GOPROXY:'off',GOSUMDB:'off',GOTOOLCHAIN:'local',CGO_ENABLED:'0',GOFLAGS:'',
  GIT_TERMINAL_PROMPT:'0',GCM_INTERACTIVE:'0'};
function command(file,values,cwd=project){
  const result=spawnSync(file,values,{cwd,env,encoding:'utf8',windowsHide:true,maxBuffer:2*1024*1024});
  if(result.error||result.status!==0)throw new Error('daily_preparation_command_failed: '+file+' '+values[0]+' exit='+(result.status??'unavailable')+' '+(result.stderr??'').slice(0,600));
  return result.stdout.trim();
}
const sourceCommit=command('git',['rev-parse','HEAD']);
const inputs=['cmd','internal','ui-reader-broker','schemas','go.mod','go.sum','scripts/auth-session-fixture','scripts/build-x-native-owner.mjs'];
if(command('git',['status','--porcelain','--untracked-files=all','--',...inputs]))throw new Error('daily_runtime_source_must_match_commit');
const captureAssets=await sourceAssets(layout.bridge,'x');
const captureBridgeFiles=captureAssets.filter(row=>row.relative.startsWith('AkuBridge/')).map(row=>row.relative.slice('AkuBridge/'.length));
// The unchanged ordinary extension path is used by non-X too. Pin its complete
// tracked distribution, not only the subset embedded in the sealed X driver.
const bridgeFiles=command('git',['ls-files'],layout.bridge).split(/\r?\n/u).filter(Boolean);
if(command('git',['status','--porcelain','--untracked-files=all','--',...bridgeFiles],layout.bridge))throw new Error('daily_bridge_assets_must_match_commit');
const sourceFiles=command('git',['ls-files','--',...inputs]).split(/\r?\n/u).filter(Boolean);
const sourceInputs=await Promise.all(sourceFiles.map(file=>fileRecord(join(project,file))));
for(const file of bridgeFiles)sourceInputs.push(await fileRecord(join(layout.bridge,file)));
const bridgeCommit=command('git',['rev-parse','HEAD'],layout.bridge);
const baseline=[];
async function backup(source,name,optional=false){
  if(optional){try{await lstat(source);}catch(error){if(error.code==='ENOENT')return;throw error;}}
  const record=await fileRecord(source);await copyFile(source,join(rollback,name));baseline.push({...record,backup:'rollback/'+name});
}
await backup(configPath,'sidecar.json');
await backup(service.command,'aku-sidecar.exe');
await backup(join(dirname(service.command),'aku-reader-broker.exe'),'aku-reader-broker.exe');
await backup(service.command+'.runtime-state.json','aku-sidecar.exe.runtime-state.json');
await backup(join(dirname(service.command),'com.akubrowser.reader_activation.json'),'com.akubrowser.reader_activation.json',true);
await writeFile(join(rollback,'service.json'),JSON.stringify(service,null,2)+'\n',{flag:'wx'});
const image=join(bin,'aku-sidecar.exe');
command('go',['build','-trimpath','-tags','auth_session_dev','-o',image,'./cmd/akusidecar']);
command('go',['build','-trimpath','-ldflags','-H=windowsgui','-o',join(bin,'aku-reader-broker.exe'),'./cmd/aku-reader-broker']);
// Stage the non-X worker from reviewed source plus the already-pinned official
// Node distribution. No downloader, build-dev hook or registration helper runs.
const worker=join(bin,'headless-worker');
await copyTree(join(project,'internal','collection','headless','worker'),worker);
for(const name of ['node.exe','node.pin.json','LICENSE'])await copyFile(await ordinary(join(project,'runtime','dev','headless-worker',name)),join(worker,name));
await copyFile(join(project,'LICENSE'),join(worker,'LICENSE-AkuSidecar'));
const node=join(worker,'node.exe'),nodePin=JSON.parse(await readFile(join(worker,'node.pin.json'),'utf8'));
if((await fileRecord(node)).sha256!==nodePin.nodeSha256||
  (await fileRecord(join(worker,'LICENSE'))).sha256!==nodePin.licenseSha256)throw new Error('daily_node_pin_rejected');
await copyTree(join(project,'ui-reader-broker'),join(bin,'ui-reader-broker'));
const bundle=await buildNativeDailyOwnerPackage(join(bin,'x-daily-owner'));
const owner=JSON.parse(await readFile(bundle.manifest,'utf8'));
// Record actual sealed capture inputs, including the sibling Bridge. Generated
// modules are validated through the bundle hash rather than a source-file hash.
if(JSON.stringify(owner.captureAssets.filter(row=>row.path.startsWith('AkuBridge/')).map(row=>row.path.slice(10)))!==JSON.stringify(captureBridgeFiles))
  throw new Error('daily_capture_input_catalog_changed');
const externalAssets=await Promise.all([layout.chrome,layout.uiChrome,join(dirname(dirname(layout.uiChrome)),'pin.json')].map(fileRecord));
// This exits before Store, provider, HTTP, UI, factory or profile startup. It
// validates the actual ordinary config/arguments; it does not grant consent.
const configProbe=JSON.parse(command(image,[...service.args,'--runtime-candidate-probe','--runtime-candidate-probe-schema=2']));
if(configProbe.status!=='ok'||configProbe.databaseSchemaVersion!==30)throw new Error('daily_config_probe_rejected');
await writeFile(join(bin,'com.akubrowser.reader_activation.json'),JSON.stringify({name:'com.akubrowser.reader_activation',
  description:'Explicit Windows AkuBrowser reader activation',path:join(bin,'aku-reader-broker.exe'),type:'stdio',
  allowed_origins:['chrome-extension://dlibmmlopdahibfniinemhnghlifiple/']},null,2)+'\n',{flag:'wx'});
await writeFile(join(root,'config-proposal.json'),JSON.stringify(dailyConfigProposal(configPath,digest(originalBytes)),null,2)+'\n',{flag:'wx'});
await writeFile(join(root,'service-proposal.json'),JSON.stringify(dailyServiceProposal(service,image),null,2)+'\n',{flag:'wx'});
await copyFile(join(project,'docs','x-daily-development-activation.md'),join(root,'ACTIVATION.md'));
await copyFile(join(project,'scripts','x-daily-preflight.ps1'),join(root,'preflight.ps1'));
await writeFile(join(rollback,'native-hosts.json'),command('pwsh.exe',['-NoProfile','-NonInteractive','-File',join(root,'preflight.ps1')])+'\n',{flag:'wx'});
const tools=join(root,'tools');await mkdir(tools);
for(const file of ['verify-x-daily-development.mjs','x-daily-development-package.mjs','x-development-pilot-package.mjs'])
  await copyFile(join(project,'scripts',file),join(tools,file));
const preparationInputs=await Promise.all(['scripts/prepare-x-daily-development.mjs','scripts/x-daily-development-package.mjs',
  'scripts/x-daily-preflight.ps1','scripts/verify-x-daily-development.mjs','scripts/x-development-pilot-package.mjs',
  'docs/x-daily-development-activation.md'].map(file=>fileRecord(join(project,file))));
const files=await inventory(root);for(const row of files)delete row.path;
const manifest={schema:dailyPackageSchema,packageId:id,sourceCommit,bridgeCommit,buildTags:['auth_session_dev'],
  preparationOnly:true,executionApproved:false,activationReady:false,defaultOff:true,processConsentGranted:false,
  authorization:{scope:'sidecar-process',maxBatchMS:60000,cookieNames:['auth_token','ct0'],retryFailedBatch:false},
  ordinary:layout,configSHA256:digest(originalBytes),configProbe,baseline,baselineScope:'files-on-disk-only',runningProcessInspected:false,
  supervisor:{path:supervisorConfig,sha256:digest(supervisorBytes),service:'akusidecar',registrationChanged:false},
  reader:{selection:'unresolved',profileInspected:false,leaseAcquired:false},
  prerequisites:['explicit_dedicated_signed_in_reader_selection','fresh_operational_activation_approval',
    'refresh_supervisor_and_typed_registry_snapshot','graceful_old_tree_and_pipe_release',
    'review_service_registration_and_native_host_switch','explicit_process_derivation_consent'],
  preserved:{mainLogin:true,userDatabase:true,ordinaryConfigRoot:true,effectiveProviderSettings:true,autoUpdate:true},
  sourceInputs,externalAssets,preparationInputs,files};
const manifestPath=join(root,'daily-package.json');
await writeFile(manifestPath,JSON.stringify(manifest,null,2)+'\n',{flag:'wx'});
if((await fileRecord(supervisorConfig)).sha256!==manifest.supervisor.sha256)throw new Error('daily_supervisor_changed_during_preparation');
process.stdout.write(JSON.stringify({manifest:manifestPath,manifestSHA256:(await fileRecord(manifestPath)).sha256,...await verifyDailyPackage(manifestPath)})+'\n');
