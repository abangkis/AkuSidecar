// Preparation and read-only verification. Never open a DB, browser profile,
// native pipe or HTTP endpoint, and never apply the proposed service/config.
import {readFile,lstat} from 'node:fs/promises';
import {basename,dirname,join,resolve,isAbsolute,relative,sep} from 'node:path';
import {ordinary,contained,digest,fileRecord,requiredRuntimeSchemas} from './x-development-pilot-package.mjs';

const schema='aku.x.daily-development-package.v1';
const mainSchema='aku.x.daily-development-package.v2';
const equal=(a,b)=>JSON.stringify(a)===JSON.stringify(b);
export const dailyPackageSchema=schema;
// Retain verification of historical X-only schema-30 packages. New source-bound
// authority uses schema 31; neither value grants runtime compatibility itself.
export const dailyDatabaseSchemas=Object.freeze([30,31]);
export function dailyReaderContract(source='dedicated'){
  if(source==='dedicated')return{source,schema,directory:'x-daily-owner',driver:'x-daily-owner.cjs',
    entry:'internal/collection/headless/worker/native-pilot/daily-owner.mjs',lifecycle:'dailyLifecycle'};
  if(source==='main')return{source,schema:mainSchema,directory:'main-browser-transport',driver:'main-browser-transport.cjs',
    entry:'internal/collection/headless/worker/native-pilot/main-browser-transport.mjs',lifecycle:'mainLifecycle'};
  throw new Error('daily_reader_source_rejected');
}
export function dailyPreparationArguments(args){
  const values=new Map();
  for(const arg of args){
    const match=/^--(supervisor-config|reader-source)=(.+)$/u.exec(arg);
    if(!match||values.has(match[1]))throw new Error('daily_preparation_argument_rejected');
    values.set(match[1],match[2]);
  }
  const supervisorConfig=values.get('supervisor-config'),readerSource=values.get('reader-source')??'dedicated';
  if(!supervisorConfig||!isAbsolute(supervisorConfig))throw new Error('daily_absolute_supervisor_config_required');
  dailyReaderContract(readerSource);
  return{supervisorConfig,readerSource};
}
export function dailyReaderSelection(source='dedicated'){
  dailyReaderContract(source);
  return source==='main'?{selection:'ordinary-main',profileInspected:false,leaseAcquired:false,ownershipVerified:false}:
    {selection:'unresolved',profileInspected:false,leaseAcquired:false};
}
export function dailyPrerequisites(source='dedicated'){
  dailyReaderContract(source);
  return[...(source==='dedicated'?['explicit_dedicated_signed_in_reader_selection']:[]),
    'fresh_operational_activation_approval','refresh_supervisor_and_typed_registry_snapshot','graceful_old_tree_and_pipe_release',
    'review_service_registration_and_native_host_switch',
    ...(source==='main'?['verify_main_profile_and_subprofile_ownership_at_startup','review_non_x_headless_conflicts_and_quiet_unavailability']:[]),
    'explicit_process_derivation_consent'];
}
export function argumentValue(args,name){
  const values=[];
  for(let i=0;i<args.length;i++){
    if(args[i]===name){if(typeof args[i+1]!=='string'||args[i+1].startsWith('--'))throw new Error('daily_argument_value_missing');values.push(args[++i]);}
    else if(args[i].startsWith(name+'='))values.push(args[i].slice(name.length+1));
  }
  if(values.length>1)throw new Error('daily_duplicate_argument');
  return values[0];
}
export function dailyLayout(service,original,readerSource='dedicated'){
  dailyReaderContract(readerSource);
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
  const capture=original.capture;
  const initial=['exclusive',undefined,''].includes(capture?.xMode)&&
    ['dedicated',undefined,''].includes(capture?.xReaderSource);
  const existingMain=readerSource==='main'&&capture?.xMode==='concurrent'&&capture.xReaderSource==='main';
  if(original.version!==1||original.deployment?.mode!=='development'||!original.reasoning||!capture||
    capture.xReaderProfile||(!initial&&!existingMain))
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
export function dailyConfigProposal(configPath,configSHA256,readerSource='dedicated',original={}){
  dailyReaderContract(readerSource);
  if(readerSource==='main')return{schema:'aku.x.daily-config-proposal.v2',target:configPath,expectedSHA256:configSHA256,applyApproved:false,
    changes:[{path:'/capture/xMode',value:'concurrent'},{path:'/capture/xReaderSource',value:'main'},
      ...(Object.hasOwn(original.capture??{},'xReaderProfile')?[{path:'/capture/xReaderProfile',operation:'remove'}]:[])],
    readerSelection:'ordinary-main',restartRequired:true};
  return{schema:'aku.x.daily-config-proposal.v1',target:configPath,expectedSHA256:configSHA256,applyApproved:false,
    changes:[{path:'/capture/xMode',value:'concurrent'},
      {path:'/capture/xReaderProfile',value:null,requiresExplicitSelection:true}],
    readerSelection:'unresolved',restartRequired:true};
}
export function dailyMainRollbackProposal(configPath,configSHA256,original){
  const capture=original.capture;
  const state=name=>Object.hasOwn(capture,name)?{present:true,value:capture[name]}:{present:false};
  return{schema:'aku.x.daily-main-rollback-proposal.v1',applyApproved:false,switchReceiptRequired:true,shutdown:'graceful-only',
    config:{target:configPath,baselineSHA256:configSHA256,conflictPolicy:'match-switch-receipt-and-changed-fields',
      fields:['xMode','xReaderSource','xReaderProfile'].map(name=>({path:'/capture/'+name,before:state(name),
        after:name==='xReaderProfile'?{present:false}:{present:true,value:name==='xMode'?'concurrent':'main'}}))},
    service:{backup:'rollback/service.json',conflictPolicy:'match-switch-receipt'},
    nativeHosts:{backup:'rollback/native-hosts.json',conflictPolicy:'match-switch-receipt',preserveValueKindPresenceAndView:true},
    preserved:{database:true,profiles:true,login:true,providerSettings:true,autoUpdate:true},restoreDatabaseSnapshot:false};
}
export function dailyServiceProposal(service,image){
  // Exact ordinary arguments retain the true config root, database, UI/capture
  // profiles, provider override policy, app identity and pinned UI executable.
  return{...structuredClone(service),command:image};
}
async function absent(path){try{await lstat(path);return false;}catch(error){if(error.code==='ENOENT')return true;throw error;}}
function reject(condition,reason){if(!condition)throw new Error(reason);}
export async function dailyRollbackProvenance(service,{allowPackaged=false}={}){
  const state=service.command+'.runtime-state.json';
  if(!await absent(state))return{path:await ordinary(state),backup:'rollback/aku-sidecar.exe.runtime-state.json'};
  reject(allowPackaged,'daily_rollback_provenance_missing');
  // A previously sealed main package has its own provenance. Validate only
  // fixed code artifacts; never follow its historical source/state paths or
  // reuse its activation approval. Its old handoff receipt is expected.
  const root=dirname(dirname(service.command));
  contained(join(resolve(service.cwd),'build','x-daily-development-prepared'),root);
  reject(resolve(service.command)===join(root,'bin','aku-sidecar.exe')&&/^[a-f0-9]{32}$/u.test(basename(root)),
    'daily_previous_package_path_rejected');
  const path=await ordinary(join(root,'daily-package.json'));
  const previous=JSON.parse(await readFile(path,'utf8'));
  reject(previous.schema===mainSchema&&previous.readerSource==='main'&&previous.packageId===basename(root)&&
    /^[a-f0-9]{40}$/u.test(previous.sourceCommit)&&previous.preparationOnly===true&&
    equal(previous.buildTags,['auth_session_dev'])&&Array.isArray(previous.files),'daily_previous_package_contract_rejected');
  for(const file of ['bin/aku-sidecar.exe','bin/aku-reader-broker.exe','bin/com.akubrowser.reader_activation.json']){
    const rows=previous.files.filter(row=>row.file===file);
    reject(rows.length===1&&/^[a-f0-9]{64}$/u.test(rows[0].sha256),'daily_previous_package_inventory_rejected');
    reject((await fileRecord(join(root,file))).sha256===rows[0].sha256,'daily_previous_package_payload_changed');
  }
  return{path,backup:'rollback/previous-daily-package.json'};
}
export async function verifyDailyPackage(path){
  const root=dirname(await ordinary(path));
  const manifest=JSON.parse(await readFile(path,'utf8'));
  reject([schema,mainSchema].includes(manifest.schema),'daily_package_contract_rejected');
  const main=manifest.schema===mainSchema,source=main?'main':'dedicated',reader=dailyReaderContract(source);
  reject((main?manifest.readerSource==='main':[undefined,'dedicated'].includes(manifest.readerSource))&&
    manifest.preparationOnly===true&&manifest.executionApproved===false&&
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
    'bin/headless-worker/worker.mjs',`bin/${reader.directory}/manifest.json`,`bin/${reader.directory}/${reader.driver}`,
    'bin/com.akubrowser.reader_activation.json','config-proposal.json','service-proposal.json',
    'rollback/service.json','rollback/sidecar.json','rollback/native-hosts.json','ACTIVATION.md',
    'preflight.ps1','tools/verify-x-daily-development.mjs','tools/x-daily-development-package.mjs','tools/x-development-pilot-package.mjs'];
  if(main)required.push('rollback-proposal.json');
  for(const rel of required)reject(seen.has(join(root,rel).toLowerCase()),'daily_required_payload_missing: '+rel);
  const read=async rel=>JSON.parse(await readFile(join(root,rel),'utf8'));
  const service=await read('rollback/service.json'),original=await read('rollback/sidecar.json');
  const layout=dailyLayout(service,original,source);
  reject(equal(layout,manifest.ordinary),'daily_layout_changed');
  const proposal=await read('config-proposal.json');
  reject(equal(proposal,dailyConfigProposal(layout.config,manifest.configSHA256,source,original)),'daily_config_proposal_changed');
  reject(equal(await read('service-proposal.json'),dailyServiceProposal(service,join(root,'bin','aku-sidecar.exe'))),'daily_service_proposal_changed');
  reject(equal(manifest.reader,dailyReaderSelection(source)),'daily_reader_prerequisite_changed');
  if(main){
    reject(equal(manifest.prerequisites,dailyPrerequisites(source)),'daily_main_prerequisites_changed');
    reject(equal(await read('rollback-proposal.json'),dailyMainRollbackProposal(layout.config,manifest.configSHA256,original)),
      'daily_rollback_proposal_changed');
  }
  const owner=await read(`bin/${reader.directory}/manifest.json`);
  reject(owner.schema==='aku.x.application-owner-package.v1'&&owner[reader.lifecycle]===true&&
    (main?owner.dailyLifecycle===undefined:owner.mainLifecycle===undefined)&&
    owner.entry===reader.entry&&owner.driver?.file===reader.driver&&
    owner.driver.sha256===digest(await readFile(join(root,'bin',reader.directory,reader.driver))), 'daily_owner_pin_rejected');
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
    ['rollback/previous-daily-package.json',join(dirname(dirname(service.command)),'daily-package.json')],
    ['rollback/com.akubrowser.reader_activation.json',join(dirname(service.command),'com.akubrowser.reader_activation.json')],
  ]);
  for(const name of ['sidecar.json','aku-sidecar.exe','aku-reader-broker.exe'])
    reject(manifest.baseline.some(row=>row.backup==='rollback/'+name),'daily_rollback_baseline_missing');
  const provenance=manifest.baseline.filter(row=>['rollback/aku-sidecar.exe.runtime-state.json','rollback/previous-daily-package.json'].includes(row.backup));
  reject(provenance.length===1,'daily_rollback_provenance_missing');
  const baselineSeen=new Set();
  for(const row of manifest.baseline){
    reject(baselinePaths.has(row.backup)&&!baselineSeen.has(row.backup)&&resolve(row.path)===resolve(baselinePaths.get(row.backup)),'daily_baseline_scope_rejected');
    baselineSeen.add(row.backup);
    contained(root,join(root,row.backup));
    reject(seen.has(resolve(root,row.backup).toLowerCase())&&
      (await fileRecord(join(root,row.backup))).sha256===row.sha256,'daily_backup_changed');
    reject((await fileRecord(row.path)).sha256===row.sha256,'daily_main_baseline_stale');
  }
  reject(equal({path:provenance[0].path,backup:provenance[0].backup},await dailyRollbackProvenance(service,{allowPackaged:main})),
    'daily_rollback_provenance_changed');
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
  reject(manifest.configProbe?.status==='ok'&&dailyDatabaseSchemas.includes(manifest.configProbe.databaseSchemaVersion),'daily_schema_probe_rejected');
  reject(await absent(join(root,'activation-receipt.json')),'daily_package_already_handed_off');
  return{packageId:manifest.packageId,sourceCommit:manifest.sourceCommit,filesVerified:manifest.files.length,
    preparationOnly:true,executionApproved:false,activationReady:false,readerSource:source,readerSelection:manifest.reader.selection,
    databaseOpened:false,profileInspected:false,sessionTransferred:false};
}
