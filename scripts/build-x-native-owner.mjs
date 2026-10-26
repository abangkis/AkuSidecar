// Offline, dependency-free package builder. The output contains all executable
// imports and capture assets; its SHA is the Go factory's single driver pin.
import {readFile,writeFile,mkdir,lstat,realpath} from 'node:fs/promises';
import {resolve,dirname,relative,sep,isAbsolute} from 'node:path';
import {fileURLToPath,pathToFileURL} from 'node:url';
import {createHash} from 'node:crypto';
import {sourceAssets} from '../internal/collection/headless/worker/worker.mjs';

const project=resolve(dirname(fileURLToPath(import.meta.url)),'..');
const hash=value=>createHash('sha256').update(value).digest('hex');
const entry='internal/collection/headless/worker/native-pilot/application-owner.mjs';
const generatedAssets='internal/collection/headless/worker/native-pilot/packaged-assets.mjs';
const slash=value=>value.split(sep).join('/');
const imports=/\b(?:import\s+(?:[^'";]*?\s+from\s*)?|export\s+[^'";]*?\s+from\s*)['"]([^'"\n]+)['"]/gu;

export async function buildNativeOwnerPackage(outputDirectory){
  const output=resolve(outputDirectory),rel=relative(project,output);
  if(!rel||isAbsolute(rel)||rel==='..'||rel.startsWith('..'+sep))throw new Error('package_output_must_be_project_local');
  const assets=await sourceAssets(resolve(project,'..','AkuBridge'),'x');
  const timingPath='internal/engine/x-manual-timing-policy.json',timingSource=await readFile(resolve(project,timingPath),'utf8');
  const assetRows=assets.map(({relative:asset,content,execute,newDocument,sha256})=>({relative:asset,content,execute,...(newDocument?{newDocument:true}:{}),sha256}));
  const assetSource='const rows='+JSON.stringify(assetRows)+';\nconst resolver='+JSON.stringify(assets.structuredMediaResolver)+';\nexport async function loadPackagedAssets(){const value=rows.map(row=>({...row}));Object.defineProperty(value,"structuredMediaResolver",{value:{...resolver},enumerable:false});return value;}\n';
  const graph=new Map();
  async function visit(key){
    if(graph.has(key))return;
    const path=resolve(project,key),local=relative(project,path);
    if(isAbsolute(local)||local==='..'||local.startsWith('..'+sep))throw new Error('package_import_outside_project');
    const info=await lstat(path);
    if(!info.isFile()||info.isSymbolicLink()||await realpath(path)!==path)throw new Error('package_import_not_ordinary');
    let source=key===generatedAssets?assetSource:await readFile(path,'utf8');
    if(key==='scripts/auth-session-fixture/x-manual-timing-policy.mjs'){
      const expected="readFileSync(POLICY_PATH, 'utf8')";
      if(source.split(expected).length!==2)throw new Error('package_timing_recipe_changed');
      source=source.replace(expected,JSON.stringify(timingSource));
    }
    graph.set(key,source);
    if(key===generatedAssets)return;
    for(const match of source.matchAll(imports)){
      const specifier=match[1];
      if(specifier.startsWith('node:'))continue;
      if(!specifier.startsWith('./')&&!specifier.startsWith('../'))throw new Error('package_external_dependency');
      await visit(slash(relative(project,resolve(dirname(path),specifier))));
    }
  }
  await visit(entry);
  const modules=Object.fromEntries([...graph].sort(([a],[b])=>a.localeCompare(b)));
  const manifest={schema:'aku.x.application-owner-package.v1',entry,nodeMinimum:'22.15.0',capabilities:{sessionSeed:true,readerPost:true},
    dataAssets:[{path:timingPath,sha256:hash(timingSource)}],
    modules:Object.entries(modules).map(([path,source])=>({path,sha256:hash(source)})),
    captureAssets:assetRows.map(({relative:path,sha256})=>({path,sha256}))};
  // The synchronous hook reads embedded bytes only. Runtime relative imports
  // cannot consult a mutable sibling checkout, node_modules or staged files.
  const launcher=`'use strict';
const {registerHooks}=require('node:module');
const {pathToFileURL}=require('node:url');
const {join}=require('node:path');
const modules=${JSON.stringify(modules)};
const prefix=pathToFileURL(join(__dirname,'embedded')+require('node:path').sep).href;
const own=(value,key)=>Object.prototype.hasOwnProperty.call(value,key);
if(typeof registerHooks!=='function')throw new Error('application_owner_node_unsupported');
registerHooks({resolve(specifier,context,next){
 if(specifier.startsWith('node:'))return next(specifier,context);
 const url=specifier.startsWith(prefix)?specifier:context.parentURL?.startsWith(prefix)?new URL(specifier,context.parentURL).href:'';
 const key=url.startsWith(prefix)?decodeURIComponent(url.slice(prefix.length)):'';
 if(!own(modules,key))throw new Error('application_owner_import_rejected');
 return{url,shortCircuit:true};
},load(url,context,next){
 if(url.startsWith('node:'))return next(url,context);
 const key=url.startsWith(prefix)?decodeURIComponent(url.slice(prefix.length)):'';
 if(!own(modules,key))throw new Error('application_owner_import_rejected');
 return{format:'module',source:modules[key],shortCircuit:true};
}});
import(prefix+${JSON.stringify(entry)}).then(module=>module.runApplicationOwner()).catch(()=>{process.exitCode=2;process.stdin.destroy();process.stdout.end();});
`;
  await mkdir(output,{recursive:true});
  const driver=joinOutput(output,'x-application-owner.cjs');
  await writeFile(driver,launcher,{encoding:'utf8'});
  const record={...manifest,driver:{file:'x-application-owner.cjs',sha256:hash(launcher)}};
  await writeFile(joinOutput(output,'manifest.json'),JSON.stringify(record,null,2)+'\n');
  return{driver,manifest:joinOutput(output,'manifest.json'),sha256:record.driver.sha256,moduleCount:graph.size,assetCount:assets.length,capabilities:manifest.capabilities};
}
const joinOutput=(directory,name)=>resolve(directory,name);
if(process.argv[1]&&pathToFileURL(resolve(process.argv[1])).href===import.meta.url){
  const [output,...extra]=process.argv.slice(2);
  if(extra.length)throw new Error('usage: node scripts/build-x-native-owner.mjs [project-output-directory]');
  process.stdout.write(JSON.stringify(await buildNativeOwnerPackage(output??resolve(project,'build','x-native-owner')))+'\n');
}
