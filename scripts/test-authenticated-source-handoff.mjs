// Operator-only real-source lifecycle QA. Default is read-only preflight.
import {execFile} from 'node:child_process';
import {promisify} from 'node:util';
import {mkdir,readFile,realpath,writeFile} from 'node:fs/promises';
import {createHash,randomUUID} from 'node:crypto';
import {dirname,isAbsolute,join,relative,resolve} from 'node:path';
import {fileURLToPath,pathToFileURL} from 'node:url';
import {loadCandidate,loadRegistration,preflight,supervisor,verifyTuple,
  waitForStopped,waitForNoProfileOwners,restoreOriginal,validateBaseline,selectTarget} from './test-authenticated-headless-parity.mjs';

const sidecar=resolve(dirname(fileURLToPath(import.meta.url)),'..');
const exec=promisify(execFile);
const fail=code=>{const error=new Error(code);error.code=code;throw error;};

export function parseJourneyArguments(values) {
  const result={allowStop:false,allowSourceWindow:false,allowBridgeReload:false,mixedUpdate:false};
  for(let i=0;i<values.length;i++) {
    const flag=values[i];
    if(['--artifact','--baseline','--source'].includes(flag)) {
      const key=flag.slice(2),value=values[++i];
      if(result[key]!==undefined || !value || value.startsWith('--')) fail('invalid_arguments');
      result[key]=value;
    } else if(['--allow-runtime-stop','--allow-source-window','--allow-bridge-reload','--mixed-update'].includes(flag)) {
      const key=flag==='--mixed-update'?'mixedUpdate':flag==='--allow-runtime-stop'?'allowStop':flag==='--allow-source-window'?'allowSourceWindow':'allowBridgeReload';
      if(result[key]) fail('invalid_arguments');
      result[key]=true;
    } else fail('invalid_arguments');
  }
  if(!isAbsolute(result.artifact || '') || (!result.mixedUpdate && !isAbsolute(result.baseline || ''))
    || (result.baseline!==undefined && !isAbsolute(result.baseline))
    || !['instagram','linkedin'].includes(result.source)) fail('invalid_arguments');
  if(result.allowStop!==result.allowSourceWindow) fail('both_runtime_and_foreground_approval_required');
  if(result.allowBridgeReload && !result.allowStop) fail('bridge_reload_requires_runtime_and_foreground_approval');
  return result;
}

export function registeredBridgeIdentity(registration,manifest) {
  const args=registration?.args;
  const values=flag=>Array.isArray(args)?args.flatMap((v,i)=>v===flag?[args[i+1]]:[]):[];
  const paths=values('--bridge-extension-path'),origins=values('--bridge-extension-origin');
  if(paths.length!==1 || origins.length!==1 || !isAbsolute(paths[0] || '')
    || typeof manifest?.key!=='string' || !manifest.key) fail('registered_bridge_identity_unavailable');
  const id=createHash('sha256').update(Buffer.from(manifest.key,'base64')).digest('hex').slice(0,32)
    .replace(/[0-9a-f]/g,v=>String.fromCharCode(97+parseInt(v,16)));
  const origin=`chrome-extension://${id}`;
  if(origins[0].replace(/\/$/,'')!==origin) fail('registered_bridge_origin_mismatch');
  return {path:paths[0],origin,id};
}

async function loadRegisteredJourneyBridge(registration) {
  const config=JSON.parse(await readFile(join(process.env.LOCALAPPDATA,'AkuSupervisor','services.json'),'utf8'));
  const service=config.services?.akusidecar;
  const expectedPath=await realpath(join(sidecar,'..','AkuBridge'));
  const manifest=JSON.parse(await readFile(join(expectedPath,'manifest.json'),'utf8'));
  const identity=registeredBridgeIdentity(service,manifest);
  const path=await realpath(identity.path);
  if(path.toLowerCase()!==expectedPath.toLowerCase()) fail('registered_bridge_path_mismatch');
  const secure=JSON.parse(await readFile(join(registration.profile,registration.profileDirectory,'Secure Preferences'),'utf8'));
  const installed=secure.extensions?.settings?.[identity.id];
  if(!installed?.path || installed.location!==4
    || (await realpath(installed.path)).toLowerCase()!==path.toLowerCase()) fail('registered_bridge_not_installed_in_profile');
  return {...identity,path};
}

async function settingsDigest() {
  const response=await fetch('http://127.0.0.1:11122/api/settings',{signal:AbortSignal.timeout(5000)});
  if(!response.ok) fail('settings_unavailable');
  const settings=(await response.json()).settings;
  if(!settings || typeof settings!=='object') fail('settings_unverifiable');
  return createHash('sha256').update(JSON.stringify(settings)).digest('hex');
}

let stage='arguments';
async function main() {
  const args=parseJourneyArguments(process.argv.slice(2));
  if(process.platform!=='win32') fail('windows_required');
  let target=null;
  if(!args.mixedUpdate) {
    stage='baseline';
    const baselinePath=await realpath(args.baseline);
    const baselineRelative=relative(await realpath(join(sidecar,'build')),baselinePath);
    if(!baselineRelative || baselineRelative.startsWith('..') || isAbsolute(baselineRelative)) fail('baseline_outside_build');
    const baseline=validateBaseline(JSON.parse(await readFile(baselinePath,'utf8')));
    target=selectTarget(baseline,args.source,0);
  }
  const sources=args.mixedUpdate?['x','facebook','instagram','linkedin']:[args.source];
  stage='candidate';
  const candidate=await loadCandidate(args.artifact);
  stage='preflight';
  const registration=await loadRegistration();
  const journeyBridge=await loadRegisteredJourneyBridge(registration);
  const before=await preflight(registration,sources);
  const originalSettings=await settingsDigest();
  if(!args.allowStop) {
    console.log(JSON.stringify({status:'preflight_only',source:args.source,sources,mixedUpdate:args.mixedUpdate,before,
      requiresRuntimeStopApproval:true,requiresForegroundApproval:true,
      profile:'registered logged-in profile and selected subprofile',database:'isolated fixture only',
      bridge:'registered development Bridge; staged candidate headless worker; not full packaged Bridge parity',
      scope:args.mixedUpdate?'one mixed-source Update with deterministic local reasoning, Facebook Adaptive Fidelity, cleanup and auto-return; not media parity or reader click':'source-window API lifetime and post-close headless capture; not trusted reader-click or actual login submission'}));
    return;
  }
  stage='artifact_verification';
  await verifyTuple(candidate.artifactRoot);
  await preflight(registration,sources);
  const receiptRoot=join(sidecar,'build',`authenticated-source-handoff-${randomUUID()}`);
  await mkdir(receiptRoot,{recursive:false});
  const report={schema:'aku.authenticated-source-handoff.v1',
    scope:args.mixedUpdate?'mixed_source_update_routing_cleanup_auto_return_not_media_parity':'real_source_window_api_lifetime_not_reader_click_or_login_submission',
    startedAt:new Date().toISOString(),source:args.source,sources,mixedUpdate:args.mixedUpdate,
    bridgeReloadAuthorized:args.allowBridgeReload,stopIssued:false,testPassed:false,restored:false};
  let operationError;
  const deadline=Date.now()+(args.mixedUpdate?10:5)*60_000;
  try {
    stage='stop';report.stopIssued=true;
    await supervisor(['stop','akusidecar','--actor','codex','--reason','authorized real-source window headless journey QA','--request-id',randomUUID()]);
    if(!(await waitForStopped(registration.profile)).stopped) fail('original_owner_not_released');
    stage='fixture';
    const env={...process.env,GOCACHE:join(sidecar,'.go-build'),GOTMPDIR:join(sidecar,'build'),TEMP:receiptRoot,TMP:receiptRoot,
      AKU_AUTH_JOURNEY_RUNTIME:dirname(candidate.worker),AKU_AUTH_JOURNEY_CHROME:registration.captureExe,
      AKU_AUTH_JOURNEY_BRIDGE:journeyBridge.path,AKU_AUTH_JOURNEY_PROFILE:registration.profile,
      AKU_AUTH_JOURNEY_PROFILE_DIRECTORY:registration.profileDirectory,AKU_AUTH_JOURNEY_SOURCE:args.source,
      AKU_AUTH_JOURNEY_TARGET_URL:target?.permalink || '',AKU_AUTH_JOURNEY_ACK_PROFILE:'1',AKU_AUTH_JOURNEY_ACK_FOREGROUND:'1'};
    // Clear inherited opt-ins; only this invocation can authorize a reload.
    env.AKU_AUTH_JOURNEY_ACK_BRIDGE_RELOAD=args.allowBridgeReload?'1':'';
    env.AKU_AUTH_JOURNEY_ACK_MIXED_UPDATE=args.mixedUpdate?'1':'';
    try {
      const testName=args.mixedUpdate?'TestAuthenticatedHybridUpdateWindowsSmoke':'TestAuthenticatedSourceHeadlessHandoffWindowsSmoke';
      const result=await exec('go.exe',['test','./internal/httpapi','-run',`^${testName}$`,'-count=1',`-timeout=${args.mixedUpdate?390:180}s`,'-v'],
        {cwd:sidecar,env,windowsHide:true,timeout:args.mixedUpdate?405000:195000,maxBuffer:1024*1024});
      await writeFile(join(receiptRoot,'test-output.txt'),result.stdout+result.stderr);
      if(!result.stdout.includes(`--- PASS: ${testName} `)) fail('fixture_did_not_run');
      report.testPassed=true;
    } catch(error) {
      if(error.stdout || error.stderr) await writeFile(join(receiptRoot,'test-output.txt'),String(error.stdout || '')+String(error.stderr || ''));
      fail('fixture_failed_inspect_private_receipt');
    }
  } catch(error) {operationError=error;report.failureStage=stage;report.failureCode=error.code || 'harness_error';}
  finally {
    if(report.stopIssued) {
      const drain=await waitForNoProfileOwners(registration.profile,15000);
      report.profileReleased=drain.clear;
      const restoration=await restoreOriginal(registration,report.stopIssued,deadline);
      report.restored=restoration.restored;report.restoreBlocked=restoration.blocked;
      report.restoreReason=restoration.reason || null;
      if(report.restored) {
        try {report.originalSettingsUnchanged=originalSettings===await settingsDigest();}
        catch {report.originalSettingsUnchanged=null;}
      }
    }
    report.finishedAt=new Date().toISOString();
    await writeFile(join(receiptRoot,'receipt.json'),JSON.stringify(report,null,2));
    console.log(JSON.stringify({...report,receipt:relative(sidecar,receiptRoot)}));
  }
  if(!report.restored || !report.profileReleased) fail('runtime_restoration_unverified');
  if(report.originalSettingsUnchanged!==true) fail('original_settings_unverified');
  if(operationError) throw operationError;
}
if(process.argv[1] && import.meta.url===pathToFileURL(resolve(process.argv[1])).href) {
  void main().catch(error=>{console.error(JSON.stringify({status:'failed',stage,code:error.code || 'harness_error'}));process.exitCode=1;});
}
