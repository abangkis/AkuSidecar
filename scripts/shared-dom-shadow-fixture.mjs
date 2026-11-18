import assert from 'node:assert/strict';
import {spawnSync} from 'node:child_process';
import {createHash} from 'node:crypto';
import {mkdir,mkdtemp,readFile,writeFile,rm,realpath} from 'node:fs/promises';
import {join,dirname,resolve,sep} from 'node:path';
import {fileURLToPath,pathToFileURL} from 'node:url';
import {registeredScriptsForSources} from '../../AkuBridge/source-access-policy.js';
import {sourceAssets} from '../internal/collection/headless/worker/worker.mjs';
import {projectLinkedInRecoveryBlockers} from '../internal/collection/headless/worker/observation.mjs';

const workspace=fileURLToPath(new URL('../../',import.meta.url));
const bridge=join(workspace,'AkuBridge');
function browserFixture({assets,cases,mode,shadow,gate,source,canonicalUrl,probeGlobal,privacyPattern}) {
  const out=document.getElementById('result'),main=document.querySelector('main');
  const location=new URL(canonicalUrl);
  const effects={calls:0,mutations:0,clicks:0,scrolls:0,network:0};let inside=false,receive;
  let recoveryClicks=0;
  main.addEventListener('click',event=>{if(event.target.closest('button[aria-label^="Open control menu for post by"]'))recoveryClicks++;});
  const forbidden=field=>()=>{if(inside){effects[field]++;throw Error('shadow effect forbidden');}};
  const window={location,innerWidth,innerHeight,scrollX:0,scrollY:0,scrollTo:forbidden('scrolls'),scrollBy:forbidden('scrolls')};
  const pushes=[];
  const chrome={runtime:{onMessage:{addListener(fn){receive=fn;},removeListener(){}},async sendMessage(message){
    if(message.type==='AKU_BROWSER_CAPTURE_PROGRESS'){pushes.push(message);return {ok:true};}
    return {ok:false};
  }}};
  const request=message=>new Promise(ok=>receive(message,null,ok));
  const NativeDate=globalThis.Date;
  globalThis.Date=class extends NativeDate {constructor(...args){super(...(args.length?args:['2026-11-16T00:00:00Z']));}};
  globalThis.AkuHeadlessCapturePolicy={allowContentExpansion:false};
  for(const code of assets)new Function('window','chrome','location',code)(window,chrome,location);
  const observer=new MutationObserver(()=>{});observer.observe(document,{subtree:true,childList:true,attributes:true,characterData:true});
  const original=globalThis[probeGlobal];
  if(shadow)globalThis[probeGlobal]={...original,assess(sample){
    observer.takeRecords();inside=true;effects.calls++;
    try{return original.assess(sample);}finally{effects.mutations+=observer.takeRecords().length;inside=false;}
  }};
  for(const name of ['click','scrollIntoView','scrollTo']){
    const original=Element.prototype[name];if(!original)continue;
    Element.prototype[name]=function(...args){if(inside){effects[name==='click'?'clicks':'scrolls']++;throw Error('shadow mutation forbidden');}return original.apply(this,args);};
  }
  globalThis.fetch=forbidden('network');
  const ready=new Function(gate+'\nreturn isSourceCaptureReady;')();
  // Exclude only observation timing and the additive shadow field from a full
  // before/after comparison. Candidate/quality/recovery/readiness values stay.
  const stable=value=>JSON.parse(JSON.stringify(value,(key,item)=>['sharedDOMShadow','elapsedMs','waitMs'].includes(key)?undefined:item));
  return (async()=>{
    const rows=[];
    for(const item of cases){
      location.href=item.route??canonicalUrl;main.innerHTML=item.body;
      recoveryClicks=0;
      if(shadow){globalThis[probeGlobal]={...globalThis[probeGlobal],assess:item.failProbe?()=>{throw Error('private probe failure');}:function(sample){
        observer.takeRecords();inside=true;effects.calls++;
        try{return original.assess(sample);}finally{effects.mutations+=observer.takeRecords().length;inside=false;}
      }};}
      if(mode==='windowed'){
        const probe=await request({type:'AKU_BROWSER_PROBE_SOURCE_READY',source});
        if(!probe.ok)throw Error('Windowed readiness failed');
        let observation=null;
        if(ready(probe.readiness)){
          const diagnosticAttempt=`00000000-0000-4000-8000-${String(rows.length+1).padStart(12,'0')}`;
          const capture=await request({type:'AKU_BROWSER_COLLECT_VISIBLE',payload:{source,scrolls:0,restoreScroll:false,diagnosticAttempt,
            qualityRetryBudget:0,sourceFreshness:{},sourceReadiness:probe.readiness}});
          if(!capture.ok)throw Error('Windowed fixture capture failed: '+capture.message);
          observation=capture.observation;
          if(observation.snapshots.some(snapshot=>Object.hasOwn(snapshot,'linkedinRecoveryBlockers'))){
            throw Error('Windowed observation snapshots must conform to the strict Sidecar schema');
          }
          if(shadow && !item.failProbe && JSON.stringify(observation.coverage.sharedDOMShadow)!==JSON.stringify(probe.readiness.sharedDOMShadow))throw Error('Windowed projection lost shadow');
        }
        rows.push({name:item.name,diagnostic:probe.readiness.sharedDOMShadow??null,
          recoveryClicks,recoveryBlockers:observation?.coverage?.linkedinRecoveryBlockers??[],baseline:stable({readiness:probe.readiness,observation})});
      }else{
        const snapshot=await globalThis.XHeadlessPoC.collect();
        rows.push({name:item.name,diagnostic:snapshot.sharedDOMShadow??null,recoveryClicks,recoveryBlockers:snapshot.linkedinRecoveryBlockers??[],baseline:stable(snapshot)});
      }
    }
    observer.disconnect();return {rows,effects,progress:{count:pushes.length,
      attempts:new Set(pushes.map(message=>message.attemptId)).size,
      safe:pushes.every(message=>message.schemaVersion===1&&message.sequence>=1&&message.progress.source===source&&
        !new RegExp(privacyPattern).test(JSON.stringify(message.progress)))}};
  })();
}

export async function runDOMShadowParity({chrome,output,source,canonicalUrl,probeGlobal,cases,privacyPattern,style}) {
  const windowedFiles=registeredScriptsForSources([source]).find(s=>s.id===`aku-source-${source}-feed`).js;
  const windowed=await Promise.all(windowedFiles.map(async relative=>({relative:'AkuBridge/'+relative,content:await readFile(join(bridge,relative),'utf8')})));
  const headless=(await sourceAssets(bridge,source)).filter(a=>a.execute);
  const engine='AkuBridge/shared-dom-engine/browser-runtime.js',probe=`AkuBridge/adapters/${source}-dom-shadow.js`;
  const digest=value=>createHash('sha256').update(value).digest('hex');
  const sharedFiles=[engine,probe,...(source==='linkedin'?['AkuBridge/linkedin-permalink-policy.js']:[])];
  for(const file of sharedFiles)assert.equal(digest(windowed.find(a=>a.relative===file).content),headless.find(a=>a.relative===file).sha256);
  const service=await readFile(join(bridge,'service-worker.js'),'utf8');
  const gate=service.match(/^function isSourceCaptureReady\(readiness\) \{[\s\S]*?^\}/m)?.[0];assert.ok(gate);
  const parent=join(workspace,`AkuSidecar/build/${source}-dom-shadow`);await mkdir(parent,{recursive:true});
  const scratch=await mkdtemp(join(parent,'fixture-'));
  try{
    const results={};
    for(const [mode,assets]of [['windowed',windowed],['headless',headless]])for(const shadow of [false,true]){
      const label=mode+(shadow?'-shadow':'-baseline');
      const selected=shadow?assets:assets.filter(a=>![engine,probe].includes(a.relative));
      const script='('+browserFixture.toString()+')('+JSON.stringify({assets:selected.map(a=>a.content),cases,mode,shadow,gate,source,canonicalUrl,probeGlobal,privacyPattern})+').then(r=>document.getElementById("result").textContent=JSON.stringify(r)).catch(e=>document.getElementById("result").textContent=JSON.stringify({error:String(e.stack||e)}));';
      const file=join(scratch,label+'.html');
      await writeFile(file,'<!doctype html><meta charset="utf-8"><meta http-equiv="Content-Security-Policy" content="default-src \'none\'; script-src \'unsafe-inline\' \'unsafe-eval\'; style-src \'unsafe-inline\'"><base href="'+canonicalUrl+'"><style>'+style+'</style><main></main><pre id="result"></pre><script>'+script.replaceAll('</script','<\\/script')+'</script>');
      const temp=join(scratch,label+'-temp');await mkdir(temp);
      const run=spawnSync(chrome,['--headless','--dump-dom','--virtual-time-budget=30000','--disable-background-networking','--disable-component-update',
        '--no-first-run','--no-default-browser-check','--host-resolver-rules=MAP * ~NOTFOUND','--window-size=1200,1000',
        '--user-data-dir='+join(scratch,label+'-profile'),pathToFileURL(file).href],
      {windowsHide:true,timeout:45000,encoding:'utf8',maxBuffer:32*1024*1024,env:{...process.env,TEMP:temp,TMP:temp}});
      assert.equal(run.status,0,String(run.error??run.stderr).slice(-1000));
      const encoded=run.stdout.match(/<pre id="result">([\s\S]*?)<\/pre>/)?.[1];assert.ok(encoded);
      const result=JSON.parse(encoded.replaceAll('&lt;','<').replaceAll('&gt;','>').replaceAll('&amp;','&'));assert.ok(!result.error,result.error);
      assert.equal(result.rows.length,cases.length);results[label]=result;
    }
    const rows=cases.map((spec,index)=>{
      const w=results['windowed-shadow'].rows[index],h=results['headless-shadow'].rows[index];
      if(Object.hasOwn(spec,'headlessCode')){
        assert.equal(h.diagnostic?.code??null,spec.headlessCode,spec.name+' Headless diagnostic');
        assert.ok(spec.difference,'An intentional consumer difference requires an explanation.');
      }else assert.deepEqual(w.diagnostic,h.diagnostic,spec.name+' diagnostic');
      if(spec.code)assert.equal(w.diagnostic?.code,spec.code,spec.name);
      else assert.equal(w.diagnostic,null,spec.name);
      for(const mode of ['windowed','headless'])assert.deepEqual(results[mode+'-shadow'].rows[index].baseline,results[mode+'-baseline'].rows[index].baseline,mode+' unchanged capture: '+spec.name);
      assert.doesNotMatch(JSON.stringify(w.diagnostic),new RegExp(privacyPattern));
      for(const [key,count]of Object.entries(spec.rejections??{}))assert.equal(w.diagnostic?.rejections[key],count,spec.name+" rejection "+key);
      for(const [key,count]of Object.entries(spec.counts??{}))assert.equal(w.diagnostic?.counts[key],count,spec.name+" count "+key);
      if(spec.captureIdentity){
        const {id,url,headlessRejection,mediaKind,linkUrl,noRecovery}=spec.captureIdentity;
        const blocks=w.baseline.observation?.snapshots.flatMap(snapshot=>snapshot.blocks)??[];
        assert.equal(blocks.length,1,spec.name+' Windowed block count');
        assert.ok(blocks[0].text,spec.name+' Windowed retains authored text');
        assert.equal(blocks[0].platformId??null,id??null,spec.name+' Windowed platform ID');
        assert.equal(blocks[0].permalink??null,url??null,spec.name+' Windowed permalink');
        if(mediaKind)assert.ok(blocks[0].media.some(item=>item.kind===mediaKind),spec.name+' Windowed media');
        if(linkUrl)assert.ok(blocks[0].links.some(item=>item.href===linkUrl),spec.name+' Windowed authored link');
        if(noRecovery)assert.equal(w.recoveryClicks+h.recoveryClicks,0,spec.name+' no recovery for a conflict');
        assert.deepEqual(h.baseline.posts.map(post=>[post.id,post.permalink]),id?[[id,url]]:[],spec.name+' Headless identity');
        if(headlessRejection)assert.equal(h.baseline.rejectionReasons[headlessRejection],1,spec.name+' Headless rejection');
      }
      if(Object.hasOwn(spec,'recoveryBlocker')){
        const native=projectLinkedInRecoveryBlockers({source,linkedinRecoveryBlockers:h.recoveryBlockers},0).linkedinRecoveryBlockers??[];
        const projected=projectLinkedInRecoveryBlockers({source,linkedinRecoveryBlockers:w.recoveryBlockers},0).linkedinRecoveryBlockers??[];
        assert.deepEqual(projected,w.recoveryBlockers,spec.name+' strict projection');
        if(spec.recoveryDifference){assert.deepEqual(native,[]);assert.equal(h.recoveryClicks>0,true,spec.name+' Headless recovery attempt');}
        else assert.deepEqual(w.recoveryBlockers,native,spec.name+' recovery diagnostic parity');
        if(spec.recoveryBlocker===null)assert.deepEqual(w.recoveryBlockers,[]);
        else {
          assert.ok(w.recoveryBlockers.length>0,spec.name+' retained blocker');
          assert.equal(w.recoveryBlockers[0].samples[0].selector,spec.recoveryBlocker);
          assert.equal(w.recoveryBlockers[0].phase,'pre_recovery_guard');
          assert.equal(w.recoveryBlockers[0].snapshotIndex,0);assert.equal(w.recoveryBlockers[0].candidateIndex,0);
          if(spec.ancestorFlag)assert.equal(w.recoveryBlockers[0].samples[0].ancestors[spec.ancestorFlag],true);
          if(spec.aria)assert.deepEqual(w.recoveryBlockers[0].samples[0].aria,spec.aria);
          assert.doesNotMatch(JSON.stringify(w.recoveryBlockers),new RegExp(privacyPattern));
        }
        if(spec.recoveryAttempt){assert.ok(w.recoveryClicks>0,spec.name+' Windowed recovery reached');assert.ok(h.recoveryClicks>0,spec.name+' Headless recovery reached');}
      }
      return {name:spec.name,diagnostic:w.diagnostic,
        ...(Object.hasOwn(spec,'recoveryBlocker')?{recoveryBlockers:w.recoveryBlockers}:{}),
        ...(spec.recoveryDifference?{recoveryDifference:spec.recoveryDifference,headlessRecoveryBlockers:h.recoveryBlockers}:{}),
        ...(spec.recoveryAttempt||spec.headlessRecoveryAttempt?{recoveryClicks:{windowed:w.recoveryClicks,headless:h.recoveryClicks}}:{}),
        ...(spec.difference?{headlessDiagnostic:h.diagnostic,intentionalDifference:spec.difference}:{}),
        captureUnchanged:{windowed:true,headless:true}};
    });
    for(const mode of ['windowed','headless']){
      const effects=results[mode+'-shadow'].effects;
      assert.equal(effects.calls,cases.filter(spec=>!spec.failProbe&&!(mode==='headless'&&spec.headlessProbe===false)).length);
      for(const field of ['mutations','clicks','scrolls','network'])assert.equal(effects[field],0,mode+' '+field);
    }
    for(const label of ['windowed-baseline','windowed-shadow']){
      assert.ok(results[label].progress.count>0,'Actual Windowed capture must publish progress');
      assert.equal(results[label].progress.safe,true);
    }
    const report={scope:'offline actual DOM and in-page entry points; synthetic fixtures, no authenticated/live capture',
      baseline:'Each consumer executes the same capture/readiness entry points with shadow assets omitted; all non-diagnostic, non-timing results must match.',
      fixtures:cases.length,engineSha256:headless.find(a=>a.relative===engine).sha256,probeSha256:headless.find(a=>a.relative===probe).sha256,
      assets:Object.fromEntries([...new Map([...windowed,...headless].map(a=>[a.relative,a])).values()].map(a=>[a.relative,digest(a.content)])),
      progress:{baseline:results['windowed-baseline'].progress,shadow:results['windowed-shadow'].progress},
      effects:{windowed:results['windowed-shadow'].effects,headless:results['headless-shadow'].effects},rows};
    if(output)await writeFile(output,JSON.stringify(report,null,2)+'\n');
    return report;
  }finally{
    const actual=await realpath(scratch);assert.ok(actual.startsWith(resolve(parent)+sep));
    await rm(actual,{recursive:true,force:true,maxRetries:3,retryDelay:200});
  }
}
