// Preparation only. This helper has no browser, pilot execution or transfer path.
import {mkdir,readFile,writeFile,realpath,lstat} from 'node:fs/promises';
import {createHash,randomUUID} from 'node:crypto';
import {spawnSync} from 'node:child_process';
import {dirname,join,resolve} from 'node:path';
import {fileURLToPath} from 'node:url';
import {createServer} from 'node:net';
import {buildNativeOwnerPackage} from './build-x-native-owner.mjs';
import {renderXQAController} from './auth-session-fixture/x-qa-controller-render.mjs';

const project=resolve(dirname(fileURLToPath(import.meta.url)),'..');
if(process.argv.length!==2||process.platform!=='win32')throw new Error('Windows preparation takes no arguments; native execution is unavailable');
const id=randomUUID().replaceAll('-','');
const parent=join(project,'build','x-application-qa-prepared');
await mkdir(parent,{recursive:true});
if((await realpath(parent)).toLowerCase()!==parent.toLowerCase()||(await lstat(parent)).isSymbolicLink())throw new Error('Preparation output must be an ordinary project directory');
const root=join(parent,id);
await mkdir(root);
const temp=join(root,'tmp');await mkdir(temp);
const env={...process.env,TEMP:temp,TMP:temp,GOTMPDIR:temp,
  GOCACHE:join(project,'.go-build','x-qa-entry'),GOMODCACHE:join(project,'.go-build','mod'),
  GOWORK:'off',GOPROXY:'off',GOSUMDB:'off',GOTOOLCHAIN:'local',CGO_ENABLED:'0',GOFLAGS:''};
function command(file,args){
  const result=spawnSync(file,args,{cwd:project,env,encoding:'utf8',windowsHide:true,maxBuffer:1024*1024});
  if(result.error||result.status!==0)throw new Error('Preparation command failed; inspect the project-owned output before retry');
  return result.stdout.trim();
}
// Bind an uncommitted implementation to its actual source and binary hashes;
// the Git commit alone never claims to identify a dirty compiled graph.
const sourceCommit=command('git',['rev-parse','HEAD']);
const sourceDirty=command('git',['status','--porcelain','--untracked-files=normal','--','cmd','internal','ui-reader-broker','go.mod','go.sum'])!=='';
const sourcePaths=command('git',['ls-files','--cached','--others','--exclude-standard','--','cmd','internal','ui-reader-broker','go.mod','go.sum']).split(/\r?\n/u).filter(Boolean).sort();
const sourceDigest=createHash('sha256');
for(const path of sourcePaths){sourceDigest.update(path+'\0');sourceDigest.update(createHash('sha256').update(await readFile(join(project,path))).digest());}
const sourceSHA256=sourceDigest.digest('hex');
const image=join(root,'akusidecar-qa.exe');
command('go',['build','-tags','auth_session_dev','-o',image,'./cmd/akusidecar']);
const bundle=await buildNativeOwnerPackage(join(root,'owner'));
const sha256=async path=>createHash('sha256').update(await readFile(path)).digest('hex');
const node=process.execPath;
const chrome='C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe';
const assets={application:{path:image,sha256:await sha256(image)},node:{path:node,sha256:await sha256(node)},
  chrome:{path:chrome,sha256:await sha256(chrome)},driver:{path:bundle.driver,sha256:bundle.sha256}};
const port=await new Promise((accept,reject)=>{const server=createServer();server.once('error',reject);server.listen(0,'127.0.0.1',()=>{const selected=server.address().port;server.close(err=>err?reject(err):accept(selected));});});
const requestPath=join(root,'materialize-request.json');
await writeFile(requestPath,JSON.stringify({schema:'aku.x.application-qa-materialize.v1',parent:root,port,
  node:assets.node,chrome:assets.chrome,driver:assets.driver})+'\n',{flag:'wx'});
// This invokes only the early preparation exit, never the native selector,
// config, Store, owner factory or browser. The fixed retained source is deferred.
const materialized=JSON.parse(command(image,['--dev','--dev-qa-pilot-prepare='+requestPath]));
if(materialized.event!=='qa_plan_prepared'||materialized.executionApproved!==false||materialized.browserStarted!==false||materialized.retainedProfileInspected!==false)throw new Error('Preparation-only receipt rejected');
const controllerPath=join(root,'run-once.mjs');
await writeFile(controllerPath,await renderXQAController({id,commit:sourceCommit,sourceSHA256}),{flag:'wx'});
command(node,['--check',controllerPath]); // Parse only; no controller execution or claim.
const controller={path:controllerPath,sha256:await sha256(controllerPath),ordering:'capture_entered_before_open_post',
  sourceDirty:command('git',['status','--porcelain','--untracked-files=normal','--','scripts/auth-session-fixture/x-qa*','scripts/prepare-x-application-qa.mjs'])!==''};
const plan={schema:'aku.x.application-qa-preparation.v2',sourceCommit,sourceDirty,sourceSHA256,
  preparationOnly:true,executionApproved:false,executionReady:false,runtimePlanMaterialized:true,
  readiness:'offline_ready_pending_native_execution_authorization',blockers:[],
  runtimePlan:{path:materialized.plan,sha256:materialized.sha256},assets,controller,
  reader:{selection:'existing_retained_qa_in_place',
    manifest:join(project,'runtime','qa','auth-session-sidecar','x-retained-native-qa.json'),
    inspected:false,leaseAcquired:false,profileCopied:false},
  collector:{selection:'fresh_disposable_instance_profile',created:true},
  database:{selection:'fresh_instance_database',created:false},
  controls:{defaultOff:true,manualStart:true,source:'x',maxAttemptMS:60000,
    attemptClock:'seed_authorization_start',
    cookieNames:['auth_token','ct0'],transferDirection:'reader_to_collector',maxTransfers:1,retry:false},
  evidence:{startup:'strict_both_owners',reader:'exact_owned_marker_foreground_then_canonical_post',
    capture:'nonempty_observation_committed_once_to_qa_database',
    overlap:'require_observed_capture_activity_during_reader_open; active_status_alone_is_insufficient',
    cleanup:'collector_empty_then_complete_parent_owned_cleanup; retain_fence_if_uncertain'},
  forbidden:['main_login_access','user_database_access','main_runtime_change','auto_update_change',
    'profile_copy_or_move','cookie_value_logging','raw_body_logging'],
  runbook:'docs/x-application-native-qa-run-plan.md'};
const planPath=join(root,'prepared-review.json');
await writeFile(planPath,JSON.stringify(plan,null,2)+'\n',{flag:'wx'});
process.stdout.write(JSON.stringify({preparedReview:planPath,sourceCommit,sourceDirty,sourceSHA256,runtimePlan:materialized.plan,executionReady:false,
  browserStarted:false,sessionTransferred:false,retainedProfileInspected:false})+'\n');
