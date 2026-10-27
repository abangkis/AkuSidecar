// Preparation/verification only. No launch, registry write, network request or
// account/profile/database reader belongs in this module.
import {createHash} from 'node:crypto';
import {lstat,realpath,readdir,readFile,copyFile,mkdir} from 'node:fs/promises';
import {resolve,relative,dirname,isAbsolute,sep,join} from 'node:path';

export const digest=bytes=>createHash('sha256').update(bytes).digest('hex');
export async function ordinary(path,directory=false){
  const full=resolve(path),info=await lstat(full);
  if(info.isSymbolicLink()||(directory?!info.isDirectory():!info.isFile()))throw new Error('package_path_not_ordinary');
  if((await realpath(full)).toLowerCase()!==full.toLowerCase())throw new Error('package_path_alias');
  return full;
}
export function contained(root,path){
  const rel=relative(resolve(root),resolve(path));
  if(!rel||isAbsolute(rel)||rel==='..'||rel.startsWith('..'+sep))throw new Error('package_path_escape');
  return rel;
}
export async function fileRecord(path){return{path:resolve(path),sha256:digest(await readFile(await ordinary(path)))}};
export async function copyTree(source,destination){
  await ordinary(source,true);
  await mkdir(destination,{recursive:true});
  for(const entry of await readdir(source,{withFileTypes:true})){
    const from=join(source,entry.name),to=join(destination,entry.name);
    if(entry.isSymbolicLink())throw new Error('package_tree_alias');
    if(entry.isDirectory())await copyTree(from,to);
    else{await ordinary(from);await copyFile(from,to);}
  }
}
export async function inventory(root){
  const rows=[];
  async function visit(directory){
    await ordinary(directory,true);
    for(const item of await readdir(directory,{withFileTypes:true})){
      const path=join(directory,item.name);
      if(item.isDirectory())await visit(path);
      else rows.push({file:contained(root,path).split(sep).join('/'),...(await fileRecord(path))});
    }
  }
  await visit(root);return rows.sort((a,b)=>a.file.localeCompare(b.file));
}
export function instanceConfig(original,plan,project){
  const cfg=structuredClone(original);
  if(cfg.version!==1||!cfg.reasoning||!cfg.capture)throw new Error('package_configuration_incomplete');
  cfg.server={host:'127.0.0.1',port:plan.port};
  cfg.database={path:plan.database};
  // This isolated UI loads the fixed packaged reader-broker identity, rather
  // than inheriting a main-launch CLI override or arbitrary extension origin.
  cfg.bridge={...cfg.bridge,trustedExtensionOrigins:['chrome-extension://dlibmmlopdahibfniinemhnghlifiple']};
  if(cfg.mediaProvenance?.c2paToolPath&&!isAbsolute(cfg.mediaProvenance.c2paToolPath))
    cfg.mediaProvenance.c2paToolPath=resolve(project,cfg.mediaProvenance.c2paToolPath);
  if(JSON.stringify(cfg.reasoning)!==JSON.stringify(original.reasoning))throw new Error('package_model_change');
  return cfg;
}
export function activationArguments(plan,configPath,appShell=true){
  const args=['--dev','--dev-session-reuse=native-x-manual','--dev-x-manual-plan='+plan.path,
    '--config='+configPath,'--database='+plan.database,'--port='+plan.port];
  if(appShell)args.push('--app-shell','--chromium-path='+plan.chrome);
  return args;
}
export async function verifyPackage(path){
  const manifest=JSON.parse(await readFile(await ordinary(path),'utf8'));
  const root=dirname(resolve(path));
  if(manifest.schema!=='aku.x.development-activation-package.v1'||manifest.preparationOnly!==true||
    manifest.executionApproved!==false||manifest.defaultOff!==true||manifest.maxAttemptMS!==60000||
    manifest.controls.retry!==false||manifest.controls.maxTransfers!==1||
    JSON.stringify(manifest.controls.cookieNames)!==JSON.stringify(['auth_token','ct0']))throw new Error('package_contract_rejected');
  if(!Array.isArray(manifest.files)||manifest.files.length===0)throw new Error('package_inventory_missing');
  for(const path of [manifest.plan.path,manifest.config.path,manifest.plan.database,manifest.launch.image])contained(root,path);
  if(JSON.stringify(manifest.launch.arguments)!==JSON.stringify(activationArguments(manifest.plan,manifest.config.path)))throw new Error('package_launch_changed');
  const approvedExternal=resolve('C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe');
  if(!Array.isArray(manifest.externalAssets))throw new Error('external_assets_missing');
  for(const row of manifest.externalAssets){if(resolve(row.path).toLowerCase()!==approvedExternal.toLowerCase())throw new Error('external_asset_scope_rejected');}
  const seen=new Set();
  for(const row of manifest.files){
    const file=resolve(root,row.file);contained(root,file);
    if(seen.has(file.toLowerCase()))throw new Error('package_inventory_duplicate');seen.add(file.toLowerCase());
    if((await fileRecord(file)).sha256!==row.sha256)throw new Error('package_asset_changed');
  }
  for(const path of [manifest.plan.path,manifest.config.path,manifest.launch.image]){
    if(!seen.has(resolve(path).toLowerCase()))throw new Error('required_package_asset_missing');
  }
  for(const row of manifest.externalAssets){if((await fileRecord(row.path)).sha256!==row.sha256)throw new Error('external_asset_changed');}
  const config=JSON.parse(await readFile(manifest.config.path,'utf8'));
  if(digest(JSON.stringify(config.reasoning))!==manifest.modelPolicySHA256)throw new Error('package_model_change');
  if(config.server.host!=='127.0.0.1'||config.server.port!==manifest.plan.port||config.database.path!==manifest.plan.database)throw new Error('package_instance_scope_changed');
  try{await lstat(manifest.plan.database);throw new Error('package_database_already_consumed');}catch(error){if(error.code!=='ENOENT')throw error;}
  return {packageId:manifest.packageId,filesVerified:manifest.files.length,sourceCommit:manifest.sourceCommit,
    preparationOnly:true,executionApproved:false,profileInspected:false,databaseOpened:false};
}
