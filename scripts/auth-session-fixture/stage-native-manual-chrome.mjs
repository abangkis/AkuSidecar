import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import {execFileSync} from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { sourceAssets } from '../../internal/collection/headless/worker/worker.mjs';

// Startup-only modes use a fixed allowlist. Capture mode compiles actual Engine
// production sources but excludes package TestMain and unrelated browser tests.
const project = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..', '..');
const task = path.resolve(process.argv[2] ?? '');
const parent = path.join(project, 'runtime', 'qa', 'auth-session-sidecar');
const same = (a, b) => a.toLowerCase() === b.toLowerCase();
if (!same(path.dirname(task), parent) || !path.basename(task).startsWith('run-') ||
    !fs.statSync(task).isDirectory() || !same(fs.realpathSync(task), task)) throw new Error('standalone_stage_scope_invalid');
const stage = path.join(task, 'go-src');
fs.mkdirSync(stage);
const hash = value => crypto.createHash('sha256').update(value).digest('hex');
const mode=process.argv[3];
const capture=mode==='x-retained-capture';
const retained=mode==='x-retained-manual'||capture;
const files = capture ? execFileSync('go',['list','-tags','auth_session_dev auth_session_fixture','-f','{{range .GoFiles}}{{println .}}{{end}}','./internal/engine'],{cwd:project,encoding:'utf8'}).trim().split(/\r?\n/u).concat(['x_manual_pilot_retained_windows_test.go','x_manual_capture_retained_windows_test.go']) : ['chrome_fixture_windows.go', 'development_native_pilot_adapter.go',
  'development_native_pilot_capture_diagnostic.go',
  'development_native_pilot_startup.go', 'development_native_pilot_controller.go',
  'development_native_pilot_broker.go', retained?'x_manual_pilot_retained_windows_test.go':'native_manual_pilot_chrome_windows_test.go'];
if(mode!==undefined){
  if(!['x-owner-port','x-retained-manual','x-retained-capture'].includes(mode))throw new Error('standalone_stage_mode_invalid');
  if(!capture)files.push('development_x_manual_pilot_owner.go','development_x_manual_pilot_owner_windows.go');
}
const manifest = { schema: 'aku.native-manual-chrome.staging.v1', copied: [], extracted: [] };
// Shared imported code must be pinned even for the named-file staging modes.
manifest.moduleDependencies=['go.mod','go.sum',...['internal/ownedprocess','internal/ownedrpc','internal/ownedprofile'].flatMap(directory=>
  fs.readdirSync(path.join(project,directory)).filter(file=>file.endsWith('.go')&&!file.endsWith('_test.go')).sort().map(file=>`${directory}/${file}`)
)].map(source=>({source,sha256:hash(fs.readFileSync(path.join(project,source)))}));
if(mode!==undefined){
  manifest.nodeSources=['x-native-manual-owner-port.mjs','x-native-manual-owner-port.test.mjs','x-native-manual-owner-offline.mjs','x-native-metadata-probe-driver.mjs',
    'x-native-manual-capture.mjs','x-native-manual-capture.test.mjs','x-capture-traffic-diagnostics.mjs','x-capture-traffic-diagnostics.test.mjs',
    'x-qa-feasibility-collector.mjs','x-manual-startup-owner.mjs','x-manual-startup-owner.test.mjs',
    'x-manual-startup-source.mjs','x-manual-timing-policy.mjs','x-manual-startup-limits.mjs','x-manual-startup-offline-fixture.mjs',
    'x-manual-header-safety.mjs','x-manual-root-document.mjs','x-manual-root-source-policy.mjs','x-manual-root-controls.mjs','x-manual-root-controls.test.mjs',
    'x-identity-freshness-ledger.mjs','x-identity-freshness-ledger.test.mjs',
    'x-manual-enrollment-capture.mjs','x-manual-enrollment-capture.test.mjs',
    'x-bootstrap-breakpoint-handoff.mjs','x-early-bootstrap-capture.mjs',
    'x-manual-startup-diagnostic.mjs','x-manual-startup-diagnostic.test.mjs',
    'x-manual-startup-timing.mjs','x-manual-startup-timing.test.mjs',
    'x-manual-startup-wait-budget.mjs','x-manual-startup-wait-budget.test.mjs',
    'x-settings-wait-state.mjs','x-settings-wait.test.mjs',
    'x-qa-bound-observation.mjs','x-qa-bound-observation.test.mjs','x-qa-cookie-writes.mjs',
    'x-manual-startup-cookie-guard.mjs','x-manual-startup-cookie-guard.test.mjs',
    'x-manual-diagnostic-capacity.test.mjs',
    'x-manual-unframed-traffic.test.mjs',
    'x-manual-startup-redirect.test.mjs',
    'x-manual-startup-first-cause-offline.mjs','x-manual-startup-first-cause.test.mjs',
    'x-manual-startup-root-navigation.test.mjs','x-manual-network-provenance.test.mjs'].map(file=>{
    const source='scripts/auth-session-fixture/'+file;
    return {source,sha256:hash(fs.readFileSync(path.join(project,source)))};
  });
  // These are code assets only. No browser, profile or native driver executes.
  // Pin both the shared worker and sibling AkuBridge parser files used by X.
  manifest.captureSourceAssets=(await sourceAssets(path.resolve(project,'..','AkuBridge'),'x')).map(asset=>({
    source:path.relative(project,asset.path).replaceAll('\\','/'),sha256:asset.sha256,
  }));
}
// Embed the same policy bytes that the Node reader uses; Go-only staging must
// not silently fall back to a second copy or omit the policy asset.
const timingPolicySource = 'internal/engine/x-manual-timing-policy.json';
const timingPolicyBytes = fs.readFileSync(path.join(project, timingPolicySource));
fs.writeFileSync(path.join(stage, 'x-manual-timing-policy.json'), timingPolicyBytes, {flag:'wx'});
manifest.copied.push({source:timingPolicySource,sha256:hash(timingPolicyBytes)});
for (const file of files) {
  const relative = `internal/engine/${file}`, bytes = fs.readFileSync(path.join(project, relative));
  fs.writeFileSync(path.join(stage, file), bytes, { flag: 'wx' });
  manifest.copied.push({ source: relative, sha256: hash(bytes) });
}
const original = fs.readFileSync(path.join(project, 'internal/engine/process_fixture_windows.go'), 'utf8');
const extract = name => {
  const start = original.indexOf(`func ${name}(`);
  if (start < 0 || original.indexOf(`func ${name}(`, start + 1) >= 0) throw new Error('standalone_function_unavailable');
  let end = original.indexOf('\nfunc ', start + 1);
  if(name==='processFixtureFileID'){
    const declarationEnd=original.indexOf('\nvar ',start+1);
    if(declarationEnd>=0&&declarationEnd<end)end=declarationEnd;
  }
  if (end < 0) throw new Error('standalone_function_unbounded');
  const text = original.slice(start, end).trimEnd() + '\n';
  manifest.extracted.push({ source: 'internal/engine/process_fixture_windows.go', name, sha256: hash(text) });
  return text;
};
const names = ['processFixtureProcessIdentity', 'processFixtureIdentityFromHandle', 'processFixtureDirectoryID',
  'processFixtureWindowPID', 'processFixtureWindows', 'processFixtureWindowsGone', 'processFixtureWait'];
names.push('processFixtureFileID');
const declaration = 'var processFixtureUser = windows.NewLazySystemDLL("user32.dll")';
if (!original.includes(declaration)) throw new Error('standalone_window_source_unavailable');
const templatePath = path.join(project, 'scripts/auth-session-fixture/native-manual-chrome-support.go.txt');
const template = fs.readFileSync(templatePath, 'utf8');
const support = `${template}\n${declaration}\n\n${names.map(extract).join('\n')}`;
if(!capture)fs.writeFileSync(path.join(stage, 'native_manual_chrome_support_windows.go'), support, { flag: 'wx' });
manifest.originalProcessSourceSHA256 = hash(original);
manifest.supportTemplateSHA256 = hash(template);
manifest.generatedSupportSHA256 = hash(support);
if(retained){
  // Preserve original function bytes and runtime.Caller source identity. Only
  // the test-only import is omitted; no retained-profile tests are staged.
  const relative='internal/engine/x_retained_qa_profile_windows_test.go';
  const source=fs.readFileSync(path.join(project,relative),'utf8');
  const end=source.indexOf('\nfunc TestXRetainedQAProfileOffline(');
  if(end<0||!source.includes('\n\t"testing"'))throw new Error('standalone_retained_source_unavailable');
  let prefix=source.slice(0,end).replace('\n\t"testing"','');
  const marker='const xRetainedQAProfileManifestName';
  const position=source.indexOf(marker), line=source.slice(0,position).split('\n').length;
  if(position<0)throw new Error('standalone_retained_source_unavailable');
  prefix=prefix.replace(marker,`//line ${path.join(project,relative).replaceAll('\\','/')}:${line}\n${marker}`);
  const twoPhaseRelative='internal/engine/x_two_phase_metadata_windows_test.go';
  const twoPhase=fs.readFileSync(path.join(project,twoPhaseRelative),'utf8');
  const pinStart=twoPhase.indexOf('func pinXTwoPhaseDirectory('), pinEnd=twoPhase.indexOf('\nfunc ',pinStart+1);
  if(pinStart<0||pinEnd<0)throw new Error('standalone_retained_pin_unavailable');
  const pin=twoPhase.slice(pinStart,pinEnd).trimEnd()+'\n';
  const generated=prefix+'\n'+pin;
  const generatedFile='native_manual_retained_lease_windows.go';
  fs.writeFileSync(path.join(stage,generatedFile),generated,{flag:'wx'});
  files.push(generatedFile);
  manifest.extracted.push({source:relative,section:'implementation-before-tests',originalSHA256:hash(source),sha256:hash(source.slice(0,end))},
    {source:twoPhaseRelative,name:'pinXTwoPhaseDirectory',sha256:hash(pin)});
  manifest.generatedRetainedSHA256=hash(generated);
}
fs.writeFileSync(path.join(task, 'staging-provenance.json'), JSON.stringify(manifest, null, 2) + '\n', { flag: 'wx' });
process.stdout.write(JSON.stringify({ stage, files: capture?files:[...files, 'native_manual_chrome_support_windows.go'] }) + '\n');
