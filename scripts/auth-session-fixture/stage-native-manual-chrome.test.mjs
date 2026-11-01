import test from 'node:test';
import assert from 'node:assert/strict';
import {execFileSync} from 'node:child_process';
import {createHash} from 'node:crypto';
import {mkdir,mkdtemp,readFile,realpath,rm} from 'node:fs/promises';
import {dirname,join,resolve} from 'node:path';
import {fileURLToPath} from 'node:url';

const project=resolve(dirname(fileURLToPath(import.meta.url)),'../..');
const parent=join(project,'runtime','qa','auth-session-sidecar');
const hash=value=>createHash('sha256').update(value).digest('hex');
for(const mode of [undefined,'x-owner-port','x-retained-manual']){
  test(`standalone ${mode??'chrome'} staging pins the real transient error without daily controller code`,async()=>{
    await mkdir(parent,{recursive:true});
    const task=await mkdtemp(join(parent,'run-staging-transient-'));
    try{
      const args=['scripts/auth-session-fixture/stage-native-manual-chrome.mjs',task];
      if(mode)args.push(mode);
      const staged=JSON.parse(execFileSync(process.execPath,args,{cwd:project,encoding:'utf8',timeout:15000,windowsHide:true}));
      const manifest=JSON.parse(await readFile(join(task,'staging-provenance.json'),'utf8'));
      if(mode){
        for(const file of ['x-evidence-ingress.mjs','x-manual-timing-policy.mjs','x-standard-chrome-args.mjs','x-qa-feasibility-collector.mjs']){
          const source='internal/collection/headless/worker/native-pilot/shared/'+file;
          assert.deepEqual(manifest.nodeSources.filter(row=>row.source===source),[{source,sha256:hash(await readFile(join(project,source)))}]);
          assert(!manifest.nodeSources.some(row=>row.source==='scripts/auth-session-fixture/'+file));
        }
      }
      const source='internal/engine/source_daily_retry.go',original=await readFile(join(project,source),'utf8');
      const start=original.indexOf('type dailyTransientFailure struct'),end=original.indexOf('\ntype dailyRetryCleanup interface',start);
      assert(start>=0&&end>start);
      const fragment=original.slice(start,end).trimEnd()+'\n';
      assert.deepEqual(manifest.extracted.filter(row=>row.name==='dailyTransientFailure'),[
        {source,name:'dailyTransientFailure',sha256:hash(fragment)},
      ]);
      const support=await readFile(join(staged.stage,'native_manual_chrome_support_windows.go'),'utf8');
      assert(support.includes(fragment));
      assert.equal(hash(support),manifest.generatedSupportSHA256);
      assert.equal(support.split('type dailyTransientFailure struct').length,2);
      assert.match(support,/func \(e \*dailyTransientFailure\) dailyFailureCode\(\) string/u);
      assert.match(support,/func \(e \*dailyTransientFailure\) Error\(\) string/u);
      assert.match(support,/func \(e \*dailyTransientFailure\) dailyRetryClass\(\) string/u);
      assert.doesNotMatch(support,/dailyController|func dailyTransientClass|finishRetryPolicy/u);
      assert(!staged.files.includes('source_daily_retry.go'));
      assert(staged.files.includes('development_native_pilot_capture_diagnostic.go'));
      if(mode==='x-owner-port'){
        assert(staged.files.includes('development_x_manual_pilot_owner_windows.go'));
        assert(!staged.files.includes('x_manual_pilot_retained_windows_test.go'));
      }
    }finally{
      assert.equal(dirname(task),parent);
      assert.equal(await realpath(task),task);
      await rm(task,{recursive:true,force:true});
    }
  });
}
