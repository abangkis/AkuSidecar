// Preparation and read-only verification. Never open a DB, browser profile,
// native pipe or HTTP endpoint, and never apply the proposed service/config.
import {readFile,lstat} from 'node:fs/promises';
import {dirname,join,resolve,isAbsolute,relative,sep} from 'node:path';
import {ordinary,contained,digest,fileRecord,requiredRuntimeSchemas} from './x-development-pilot-package.mjs';

const schema='aku.x.daily-development-package.v1';
const equal=(a,b)=>JSON.stringify(a)===JSON.stringify(b);
export const dailyPackageSchema=schema;
export function argumentValue(args,name){
  const values=[];
  for(let i=0;i<args.length;i++){
    if(args[i]===name){if(typeof args[i+1]!=='string'||args[i+1].startsWith('--'))throw new Error('daily_argument_value_missing');values.push(args[++i]);}
    else if(args[i].startsWith(name+'='))values.push(args[i].slice(name.length+1));
  }
  if(values.length>1)throw new Error('daily_duplicate_argument');
  return values[0];
}
export function dailyLayout(service,original){
  if(!service||!isAbsolute(service.cwd)||!isAbsolute(service.command)||!Array.isArray(service.args)||
    !service.args.every(arg=>typeof arg==='string')||!service.args.includes('--dev')||!service.args.includes('--app-shell')||
    !service.args.includes('--windows-capture-split')||service.ports?.length!==1||service.ports[0]!==11122)
    throw new Error('daily_ordinary_service_required');
  const booleans=new Set(['--dev','--app-shell','--windows-capture-split']);
  const values=new Set(['--config','--bridge-extension-origin','--chromium-path','--browser-profile',
    '--bridge-extension-path','--ui-chromium-path','--port','--database','--codex-path',
    '--app-user-model-id','--app-relaunch-command','--app-relaunch-display-name']);
  const seen=new Set();
  for(let i=0;i<service.args.length;i++){
    const arg=service.args[i],name=arg.split('=')[0];
    if(!booleans.has(name)&&!values.has(name)||seen.has(name))throw new Error('daily_service_selector_rejected');
    seen.add(name);
    if(booleans.has(name)){if(arg!==name)throw new Error('daily_service_selector_rejected');}
    else if(arg===name){if(!service.args[i+1]||service.args[i+1].startsWith('--'))throw new Error('daily_service_selector_rejected');i++;}
    else if(arg===name+'=')throw new Error('daily_service_selector_rejected');
  }
  if(original.version!==1||original.deployment?.mode!=='development'||!original.reasoning||!original.capture||
    !['exclusive',undefined,''].includes(original.capture.xMode)||original.capture.xReaderProfile)
    throw new Error('daily_baseline_config_rejected');
  const config=argumentValue(service.args,'--config');
  if(!config||!isAbsolute(config))throw new Error('daily_absolute_config_required');
  const root=dirname(dirname(config));
  if(resolve(service.cwd)!==resolve(root))throw new Error('daily_ordinary_root_changed');
  const database=resolve(root,argumentValue(service.args,'--database')??original.database?.path??'');
  const profile=argumentValue(service.args,'--browser-profile');
  const chrome=argumentValue(service.args,'--chromium-path');
  const uiChrome=argumentValue(service.args,'--ui-chromium-path');
  const bridge=argumentValue(service.args,'--bridge-extension-path');
  if(![profile,chrome,uiChrome,bridge].every(path=>typeof path==='string'&&isAbsolute(path))||
    !original.database?.path||!['127.0.0.1','localhost'].includes(original.server?.host)||
    Number(argumentValue(service.args,'--port')??original.server.port)!==11122||
    !argumentValue(service.args,'--bridge-extension-origin'))throw new Error('daily_ordinary_paths_required');
  return{root:resolve(root),config:resolve(config),database,profile:resolve(profile),chrome:resolve(chrome),uiChrome:resolve(uiChrome),bridge:resolve(bridge),port:11122};
}
export function dailyConfigProposal(configPath,configSHA256){
  return{schema:'aku.x.daily-config-proposal.v1',target:configPath,expectedSHA256:configSHA256,applyApproved:false,
    changes:[{path:'/capture/xMode',value:'concurrent'},
      {path:'/capture/xReaderProfile',value:null,requiresExplicitSelection:true}],
    readerSelection:'unresolved',restartRequired:true};
}
export function dailyServiceProposal(service,image){
  // Exact ordinary arguments retain the true config root, database, UI/capture
  // profiles, provider override policy, app identity and pinned UI executable.
  return{...structuredClone(service),command:image};
}
async function absent(path){try{await lstat(path);return false;}catch(error){if(error.code==='ENOENT')return true;throw error;}}
function reject(condition,reason){if(!condition)throw new Error(reason);}
export async function verifyDailyPackage(path){
  const root=dirname(await ordinary(path));
  const manifest=JSON.parse(await readFile(path,'utf8'));
  reject(manifest.schema===schema&&manifest.preparationOnly===true&&manifest.executionApproved===false&&
    manifest.activationReady===false&&manifest.processConsentGranted===false&&manifest.defaultOff===true&&
    manifest.authorization?.scope==='sidecar-process'&&manifest.authorization.maxBatchMS===60000&&
    manifest.authorization.retryFailedBatch===false&&equal(manifest.authorization.cookieNames,['auth_token','ct0']), 'daily_package_contract_rejected');
  const seen=new Set();
  reject(Array.isArray(manifest.files)&&manifest.files.length>0,'daily_inventory_required');
  for(const row of manifest.files){
    reject(typeof row.file==='string'&&!isAbsolute(row.file),'daily_inventory_path_rejected');
    const file=resolve(root,row.file);contained(root,file);
    reject(!seen.has(file.toLowerCase()),'daily_inventory_duplicate');seen.add(file.toLowerCase());
    reject((await fileRecord(file)).sha256===row.sha256,'daily_payload_changed');
  }
  const required=['bin/aku-sidecar.exe','bin/aku-reader-broker.exe','bin/ui-reader-broker/manifest.json',
    'bin/ui-reader-broker/content.js','bin/ui-reader-broker/service-worker.js',
    'bin/headless-worker/node.exe','bin/headless-worker/node.pin.json','bin/headless-worker/LICENSE','bin/headless-worker/LICENSE-AkuSidecar',
    'bin/headless-worker/worker.mjs','bin/x-daily-owner/manifest.json','bin/x-daily-owner/x-daily-owner.cjs',
    'bin/com.akubrowser.reader_activation.json','config-proposal.json','service-proposal.json',
    'rollback/service.json','rollback/sidecar.json','rollback/native-hosts.json','ACTIVATION.md',
    'preflight.ps1','tools/verify-x-daily-development.mjs','tools/x-daily-development-package.mjs','tools/x-development-pilot-package.mjs'];
  for(const rel of required)reject(seen.has(join(root,rel).toLowerCase()),'daily_required_payload_missing: '+rel);
  const read=async rel=>JSON.parse(await readFile(join(root,rel),'utf8'));
  const service=await read('rollback/service.json'),original=await read('rollback/sidecar.json');
  const layout=dailyLayout(service,original);
  reject(equal(layout,manifest.ordinary),'daily_layout_changed');
  const proposal=await read('config-proposal.json');
  reject(equal(proposal,dailyConfigProposal(layout.config,manifest.configSHA256)),'daily_config_proposal_changed');
  reject(equal(await read('service-proposal.json'),dailyServiceProposal(service,join(root,'bin','aku-sidecar.exe'))),'daily_service_proposal_changed');
  reject(manifest.reader?.selection==='unresolved'&&manifest.reader.profileInspected===false&&manifest.reader.leaseAcquired===false,
    'daily_reader_prerequisite_changed');
  const owner=await read('bin/x-daily-owner/manifest.json');
  reject(owner.schema==='aku.x.application-owner-package.v1'&&owner.dailyLifecycle===true&&
    owner.entry==='internal/collection/headless/worker/native-pilot/daily-owner.mjs'&&owner.driver.file==='x-daily-owner.cjs'&&
    owner.driver.sha256===digest(await readFile(join(root,'bin','x-daily-owner',owner.driver.file))), 'daily_owner_pin_rejected');
  const pin=await read('bin/headless-worker/node.pin.json');
  reject(pin.protocol===1&&pin.nodeSha256===digest(await readFile(join(root,'bin/headless-worker/node.exe')))&&
    pin.licenseSha256===digest(await readFile(join(root,'bin/headless-worker/LICENSE'))),'daily_node_pin_rejected');
  const host=await read('bin/com.akubrowser.reader_activation.json');
  reject(equal(host,{name:'com.akubrowser.reader_activation',description:'Explicit Windows AkuBrowser reader activation',
    path:join(root,'bin','aku-reader-broker.exe'),type:'stdio',allowed_origins:['chrome-extension://dlibmmlopdahibfniinemhnghlifiple/']}),'daily_native_host_changed');
  reject(Array.isArray(manifest.baseline)&&manifest.baseline.length>=4,'daily_rollback_baseline_missing');
  const baselinePaths=new Map([
    ['rollback/sidecar.json',layout.config],['rollback/aku-sidecar.exe',service.command],
    ['rollback/aku-reader-broker.exe',join(dirname(service.command),'aku-reader-broker.exe')],
    ['rollback/aku-sidecar.exe.runtime-state.json',service.command+'.runtime-state.json'],
    ['rollback/com.akubrowser.reader_activation.json',join(dirname(service.command),'com.akubrowser.reader_activation.json')],
  ]);
  for(const name of ['sidecar.json','aku-sidecar.exe','aku-reader-broker.exe','aku-sidecar.exe.runtime-state.json'])
    reject(manifest.baseline.some(row=>row.backup==='rollback/'+name),'daily_rollback_baseline_missing');
  for(const row of manifest.baseline){
    reject(baselinePaths.has(row.backup)&&resolve(row.path)===resolve(baselinePaths.get(row.backup)),'daily_baseline_scope_rejected');
    contained(root,join(root,row.backup));
    reject(seen.has(resolve(root,row.backup).toLowerCase())&&
      (await fileRecord(join(root,row.backup))).sha256===row.sha256,'daily_backup_changed');
    reject((await fileRecord(row.path)).sha256===row.sha256,'daily_main_baseline_stale');
  }
  reject((await fileRecord(layout.config)).sha256===manifest.configSHA256,'daily_main_config_stale');
  // External reads are exclusively the pinned application code/assets. The
  // database and all browser profile subtrees are never verification inputs.
  const permitted=record=>{
    const file=resolve(record.path).toLowerCase(),db=layout.database.toLowerCase(),profile=layout.profile.toLowerCase();
    reject(file!==db&&!file.startsWith(db+'-')&&file!==profile&&!file.startsWith(profile+'\\')&&!file.startsWith(profile+'/'),'daily_state_read_rejected');
    return file;
  };
  reject(Array.isArray(manifest.sourceInputs)&&manifest.sourceInputs.length>0&&Array.isArray(manifest.externalAssets)&&
    Array.isArray(manifest.preparationInputs),'daily_inputs_required');
  const sourceAllowed=path=>{
    const rel=relative(layout.root,resolve(path)).split(sep).join('/');
    const bridgeRel=relative(layout.bridge,resolve(path)).split(sep).join('/');
    return /^(?:cmd\/|internal\/|schemas\/|ui-reader-broker\/|scripts\/auth-session-fixture\/)/u.test(rel)||
      ['go.mod','go.sum','scripts/build-x-native-owner.mjs'].includes(rel)||
      bridgeRel!==''&&!bridgeRel.startsWith('../')&&!isAbsolute(bridgeRel);
  };
  const preparations=['scripts/prepare-x-daily-development.mjs','scripts/x-daily-development-package.mjs',
    'scripts/x-daily-preflight.ps1','scripts/verify-x-daily-development.mjs','scripts/x-development-pilot-package.mjs',
    'docs/x-daily-development-activation.md'];
  const external=[layout.chrome,layout.uiChrome,join(dirname(dirname(layout.uiChrome)),'pin.json')];
  reject(manifest.externalAssets.length===3&&new Set(manifest.externalAssets.map(row=>resolve(row.path))).size===3,'daily_external_scope_rejected');
  for(const row of manifest.sourceInputs)reject(sourceAllowed(row.path),'daily_source_scope_rejected');
  reject(manifest.preparationInputs.length===preparations.length,'daily_preparation_inputs_required');
  for(const name of preparations)reject(manifest.preparationInputs.some(row=>resolve(row.path)===join(layout.root,name)),'daily_preparation_inputs_required');
  for(const row of manifest.externalAssets)reject(external.includes(resolve(row.path)),'daily_external_scope_rejected');
  for(const row of [...manifest.sourceInputs,...manifest.externalAssets,...manifest.preparationInputs]){
    permitted(row);reject((await fileRecord(row.path)).sha256===row.sha256,'daily_input_stale');
  }
  for(const name of requiredRuntimeSchemas)reject(manifest.sourceInputs.some(row=>resolve(row.path)===join(layout.root,'schemas',name)),'daily_schema_missing');
  reject(manifest.configProbe?.status==='ok'&&manifest.configProbe.databaseSchemaVersion===30,'daily_schema_probe_rejected');
  reject(await absent(join(root,'activation-receipt.json')),'daily_package_already_handed_off');
  return{packageId:manifest.packageId,sourceCommit:manifest.sourceCommit,filesVerified:manifest.files.length,
    preparationOnly:true,executionApproved:false,activationReady:false,readerSelection:'unresolved',
    databaseOpened:false,profileInspected:false,sessionTransferred:false};
}
