import {captureIsolated,validateCapture} from './capture.mjs';
import {canonicalSourceURL} from './observation.mjs';

const details=new WeakMap();
const sources=new Set(['x','instagram']);
const ref=value=>typeof value==='string'&&value.length>0&&value.length<=512&&!/[\u0000-\u001f\u007f]/u.test(value);
const fields=(value,keys)=>value&&[Object.prototype,null].includes(Object.getPrototypeOf(value))&&
  Reflect.ownKeys(value).length===keys.length&&keys.every(key=>Object.hasOwn(Object.getOwnPropertyDescriptor(value,key)??{},'value'));
const bindingKeys=['source','profileId','generation','attempt'];
const same=(left,right)=>fields(left,bindingKeys)&&bindingKeys.every(key=>left[key]===right[key]);

export const sourceConcurrentDiagnostic=error=>details.get(error)??null;

// A private collector seam, not a session importer or native login verifier.
// Native IG remains unavailable until its evidence and manifest are qualified.
export function createSourceConcurrentAdapter({source,binding,collector,observePair,loadAssets,
  now=Date.now,evidenceMaxAgeMS=6000,cleanupMS=3000,runCapture=captureIsolated}) {
  const failure=(stage,reason)=>{
    const error=new Error(`source_concurrent_${stage}_${reason}`);
    details.set(error,Object.freeze({source:sources.has(source)?source:'unknown',stage,reason,cleanup:'not_started'}));
    return error;
  };
  if(!sources.has(source)||!fields(binding,['readerProfileId','collectorProfileId','accountId','generation','attempt','deadlineMS'])||
    !['readerProfileId','collectorProfileId','accountId','attempt'].every(key=>ref(binding[key]))||binding.readerProfileId===binding.collectorProfileId||
    !Number.isSafeInteger(binding.generation)||binding.generation<1||!Number.isSafeInteger(binding.deadlineMS)||
    typeof now!=='function'||typeof observePair!=='function'||typeof loadAssets!=='function'||typeof runCapture!=='function'||
    !Number.isSafeInteger(evidenceMaxAgeMS)||evidenceMaxAgeMS<1||evidenceMaxAgeMS>6000||
    !Number.isSafeInteger(cleanupMS)||cleanupMS<1||cleanupMS>3000)throw failure('admission','binding');
  const scope=Object.freeze({...binding});
  const collectorBinding=Object.freeze({source,profileId:scope.collectorProfileId,generation:scope.generation,attempt:scope.attempt});
  if(!same(collector?.binding,collectorBinding)||typeof collector.forSource!=='function'||typeof collector.createTemporaryPage!=='function')throw failure('admission','collector');
  const admitted=now();
  if(!Number.isSafeInteger(admitted)||scope.deadlineMS<=admitted||scope.deadlineMS-admitted>60000)throw failure('admission','deadline');
  let state='ready',signal,last=admitted,cleanup='not_started',closePromise,verifiedRevision;
  const pages=new Set();
  const remaining=()=>{
    const time=now();
    if(!Number.isSafeInteger(time)||time<last)throw failure('capture','clock');
    last=time;
    if(state!=='capturing'||signal?.aborted)throw failure('capture',signal?.aborted?'canceled':'closed');
    if(time>=scope.deadlineMS)throw failure('capture','deadline');
    if(!same(collector.binding,collectorBinding))throw failure('capture','collector');
    return scope.deadlineMS-time;
  };
  const within=async(stage,operation)=>{
    let timer,onAbort;
    const ms=remaining();
    try{
      const value=await Promise.race([
        Promise.resolve().then(()=>operation(remaining())),
        new Promise((_,reject)=>{timer=setTimeout(()=>reject(failure(stage,'deadline')),ms);}),
        new Promise((_,reject)=>{onAbort=()=>reject(failure(stage,'canceled'));signal?.addEventListener('abort',onAbort,{once:true});if(signal?.aborted)onAbort();}),
      ]);
      remaining();return value;
    }finally{clearTimeout(timer);signal?.removeEventListener('abort',onAbort);}
  };
  const checkProof=async phase=>{
    const pair=await within('proof',()=>observePair(phase));
    if(!fields(pair,['domain','source','generation','attempt','reader','collector'])||pair.domain!=='synthetic-fixture'||
      pair.source!==source||pair.generation!==scope.generation||pair.attempt!==scope.attempt)throw failure('proof','scope');
    remaining();const time=last;
    for(const [role,profileId]of [['reader',scope.readerProfileId],['collector',scope.collectorProfileId]]){
      const value=pair[role];
      if(!fields(value,['profileId','accountId','revision','authenticated','observedAt'])||value.profileId!==profileId||
        value.accountId!==scope.accountId||value.authenticated!==true||!ref(value.revision)||
        !Number.isSafeInteger(value.observedAt)||value.observedAt>time||time-value.observedAt>evidenceMaxAgeMS)throw failure('proof','identity');
    }
    if(pair.reader.revision!==pair.collector.revision)throw failure('proof','revision');
    if(verifiedRevision!==undefined&&pair.reader.revision!==verifiedRevision)throw failure('proof','revision_changed');
    verifiedRevision=pair.reader.revision;
  };
  const ownedPage=page=>page&&same(page.binding,collectorBinding)&&['navigate','evaluate','send'].every(key=>typeof page[key]==='function');
  const allowedURL=url=>{
    if(canonicalSourceURL(source,url))return true;
    return url===({x:'https://x.com/home',instagram:'https://www.instagram.com/'})[source];
  };
  const wrapPage=(page,close)=>{
    if(!ownedPage(page))throw failure('target','scope');
    const invoke=(method,args)=>within('worker',ms=>{
      if(!ownedPage(page))throw failure('target','scope');
      return page[method](...args,ms);
    });
    return Object.freeze({
      navigate(url){if(!allowedURL(url))throw failure('target','url');return invoke('navigate',[url]);},
      evaluate(expression){if(typeof expression!=='string'||expression.length>1024*1024)throw failure('worker','expression');return invoke('evaluate',[expression]);},
      send(method,params={}){
        if(!['Page.navigate','Runtime.evaluate','Page.addScriptToEvaluateOnNewDocument','Input.dispatchMouseEvent'].includes(method))throw failure('worker','method');
        if(method==='Page.navigate'&&!allowedURL(params.url))throw failure('target','url');
        return invoke('send',[method,params]);
      },
      ...(close?{close}:{}),
    });
  };
  const closeRecord=record=>{
    record.closing??=record.creation.then(async page=>{
      // A malformed or reader-scoped page confers no authority to close it.
      if(!ownedPage(page)||typeof page.close!=='function')throw failure('cleanup','scope');
      const receipt=await page.close(cleanupMS);
      if(!fields(receipt,[...bindingKeys,'closed'])||!bindingKeys.every(key=>receipt[key]===collectorBinding[key])||receipt.closed!==true)throw failure('cleanup','receipt');
      record.closed=true;
    });
    return record.closing;
  };
  const boundedClose=async promise=>{
    let timer;
    try{return await Promise.race([promise,new Promise((_,reject)=>{timer=setTimeout(()=>reject(failure('cleanup','deadline')),cleanupMS);})]);}
    finally{clearTimeout(timer);}
  };
  const close=()=>{
    closePromise??=(async()=>{
      state='closing';
      try{
        const results=await boundedClose(Promise.allSettled([...pages].map(closeRecord)));
        if(results.some(result=>result.status==='rejected')||[...pages].some(page=>!page.closed))throw failure('cleanup','unverified');
        cleanup='verified';
      }catch(error){cleanup='uncertain';throw details.has(error)?error:failure('cleanup','unverified');}
      finally{state='closed';}
      return Object.freeze({closed:true,source});
    })();
    return closePromise;
  };
  const browser=Object.freeze({backend:'headless_worker',
    forSource:async selected=>{
      if(selected!==source)throw failure('capture','source');
      return wrapPage(await within('target',ms=>collector.forSource(selected,ms)));
    },
    createTemporaryPage:async()=>{
      remaining();
      const record={closed:false};
      record.creation=Promise.resolve().then(()=>{remaining();return collector.createTemporaryPage(remaining());});
      pages.add(record);
      // Keep an eventual page close obligation even if creation outlives the
      // capture/cleanup deadline. Late work cannot publish an observation.
      void record.creation.then(()=>{if(state==='closing'||state==='closed')void closeRecord(record).catch(()=>{});},()=>{});
      const page=await within('target',()=>record.creation);
      return wrapPage(page,()=>boundedClose(closeRecord(record)));
    },
  });
  return Object.freeze({
    capabilities:Object.freeze({source,proofDomain:'synthetic-fixture',nativeQualified:false}),
    async capture(payload,{signal:abortSignal}={}){
      if(state!=='ready')throw failure('capture','replay');
      state='capturing';signal=abortSignal;
      let observation,first;
      try{
        const options=validateCapture(source,payload);
        if(options.acquisitionRound!==1)throw failure('capture','round');
        await checkProof('before');
        const assets=await within('assets',()=>loadAssets(source));
        const ms=Math.floor(Math.min(options.captureTimeoutMs,remaining()));
        if(ms<1000)throw failure('capture','deadline');
        observation=await within('worker',()=>runCapture(browser,{[source]:assets},source,{...payload,captureTimeoutMs:ms}));
        if(observation?.source!==source||Buffer.byteLength(JSON.stringify(observation),'utf8')>512*1024)throw failure('capture','observation');
        await checkProof('after');
      }catch(error){
        const code=error&&(typeof error==='object'||typeof error==='function')?Object.getOwnPropertyDescriptor(error,'code')?.value:undefined;
        first=details.has(error)?error:failure('capture',['login_required','challenge_required','source_unavailable','capture_timeout'].includes(code)?code:'worker');
      }
      try{await close();}catch(error){first??=error;}
      if(first){details.set(first,Object.freeze({...details.get(first),cleanup}));throw first;}
      return observation;
    },
    close,
    status:()=>Object.freeze({source,state,cleanup,cleanupScope:'temporary_media_targets',nativeQualified:false}),
  });
}
