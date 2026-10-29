import {parseStrictJSON} from '../../../../../scripts/auth-session-fixture/x-evidence-ingress.mjs';
import {ownerOperation,ownerErrorReply} from './owner-protocol.mjs';
import {applicationPostOperation} from '../../../../../scripts/auth-session-fixture/x-application-reader-post.mjs';
import {sourceTransientClass,SOURCE_RETRY_LIMIT} from './source-retry-policy.mjs';

export const DAILY_OWNER_SCHEMA='aku.x.daily-owner.v1';
const fail=()=>new Error('daily_owner_rejected');
const exact=(v,keys)=>{if(!v||typeof v!=='object'||Array.isArray(v)||Object.keys(v).length!==keys.length||keys.some(k=>!Object.hasOwn(v,k)))throw fail();};
// A bounded replay window permits concurrent writers to arrive out of order.
// IDs older than the window are rejected, never forgotten and reusable.
export function serveDailyOwnerPipe({input,output,createOwner,onClosed=()=>{}}){
  let bytes=Buffer.alloc(0),owner,booting=false,closed=false,lastID=0,pending=0,batchPending=0;
  let initialized=false,initializing=false,generation=0,state='idle',seedUsed=false;
  let transient='',retryCount=0,captureSucceeded=false;
  const inflight=new Set(),seen=new Set(),decoder=new TextDecoder('utf-8',{fatal:true});
  const close=()=>{if(closed)return;closed=true;bytes.fill(0);bytes=Buffer.alloc(0);input.pause();void Promise.resolve().then(()=>owner?.close()).catch(()=>{}).finally(onClosed);};
  const emit=value=>{if(closed)return;let wire;try{wire=Buffer.from(JSON.stringify(value)+'\n');if(wire.length>800*1024)throw fail();output.write(wire,error=>{wire.fill(0);if(error)close();});}catch{close();}};
  const progress=value=>{if(value?.event==='manual_startup_progress'&&inflight.has(value.id))emit(value);};
  async function handle(line){
    let q;
    try{
      q=parseStrictJSON(decoder.decode(line),{maxJSONDepth:12,maxJSONTokens:4096});
      if(!owner){
        if(booting)throw fail();booting=true;
        exact(q,['op','schema','nonce','chromePath','readerProfile']);
        if(q.op!=='daily_owner_bootstrap'||q.schema!==DAILY_OWNER_SCHEMA||!/^[a-f0-9]{48}$/.test(q.nonce))throw fail();
        for(const k of ['chromePath','readerProfile'])if(typeof q[k]!=='string'||q[k].length<1||q[k].length>1024||/[\u0000-\u001f\u007f]/.test(q[k]))throw fail();
        owner=await createOwner(q,progress);if(closed){await owner.close();return;}
        emit({ready:true,schema:DAILY_OWNER_SCHEMA,nonce:q.nonce});return;
      }
      if(!Number.isSafeInteger(q.id)||q.id<1||q.id<=lastID-1024||seen.has(q.id)||pending>=32)throw fail();
      lastID=Math.max(lastID,q.id);seen.add(q.id);for(const id of seen)if(id<=lastID-1024)seen.delete(id);
      pending++;inflight.add(q.id);
      const processOperation=['daily_reader_initialize','daily_reader_check','manual_prepare_post','manual_complete_post','manual_cancel_post'].includes(q.op);
      if(!processOperation)batchPending++;
      try{
        let result;
        if(q.op==='daily_reader_initialize'){
          exact(q,['id','op']);if(initialized||initializing||state!=='idle')throw fail();initializing=true;
          try{result=await owner.initialize();initialized=true;}finally{initializing=false;}
        }else if(q.op==='daily_batch_begin'){
          exact(q,['id','op','generation','collectorProfile']);
          if(!initialized||initializing||state!=='idle'||batchPending!==1||q.generation!==generation+1||!Number.isSafeInteger(q.generation))throw fail();
          generation=q.generation;state='starting';seedUsed=false;transient='';captureSucceeded=false;
          result=await owner.begin(q);state='prepared';
        }else if(q.op==='daily_batch_seed'){
          exact(q,['id','op','generation','owner']);
          if(state!=='prepared'||q.generation!==generation||seedUsed)throw fail();seedUsed=true;state='seeding';
          result=await owner.seed(q);state='active';
        }else if(q.op==='daily_batch_end'){
          exact(q,['id','op','generation']);
          if(q.generation!==generation||!['prepared','active','failed'].includes(state)||batchPending!==1)throw fail();
          const failed=state==='failed';state='closing';result=await owner.end(q);
          const retryReady=failed&&transient!==''&&retryCount<SOURCE_RETRY_LIMIT&&result?.closed===true;
          if(retryReady){retryCount++;result={...result,retryReady:true};}
          else if(!failed&&captureSucceeded)retryCount=0;
          state=failed&&!retryReady?'failed':'idle';
        }else if(q.op==='daily_reader_check'){
          exact(q,['id','op']);if(!initialized||initializing)throw fail();result=await owner.check();
        }else if(['manual_prepare_post','manual_complete_post','manual_cancel_post'].includes(q.op)){
          if(!initialized||initializing)throw fail();applicationPostOperation(q,{allowMainOrigin:true});result=await owner.post(q);
        }else{
          if(q.generation!==generation||!Number.isSafeInteger(q.generation))throw fail();
          const m={...q};delete m.generation;ownerOperation(m);
          if(state!=='active'&&!(['manual_abort','manual_capture_cancel'].includes(q.op)&&state==='failed'))throw fail();
          result=await owner.execute(m);
          if(q.op==='manual_capture')captureSucceeded=true;
        }
        if(Buffer.isBuffer(result)){const raw=result;try{result=parseStrictJSON(raw.toString('utf8'),{maxJSONDepth:4,maxJSONTokens:96});}finally{raw.fill(0);}}
        emit({id:q.id,ok:true,result});
      }catch(error){if(!processOperation){transient=state==='failed'||q.op!=='manual_capture'?'':sourceTransientClass(error);state='failed';}emit(ownerErrorReply(q.op==='daily_batch_seed'?{...q,op:'native_owner_seed'}:q,error));}
      finally{pending--;if(!processOperation)batchPending--;inflight.delete(q.id);}
    }catch{close();}finally{line.fill(0);}
  }
  input.on('data',chunk=>{if(closed)return;if(!(chunk instanceof Uint8Array)||chunk.length>131072||bytes.length+chunk.length>131072){close();return;}
    const old=bytes;bytes=Buffer.concat([bytes,chunk]);old.fill(0);let end,count=0;
    while((end=bytes.indexOf(10))>=0){if(++count>8||end<1||end>65536){close();return;}const line=Buffer.from(bytes.subarray(0,end)),old=bytes;bytes=Buffer.from(old.subarray(end+1));old.fill(0);void handle(line);if(closed)return;}
    if(bytes.length>65536)close();});
  input.on('end',close);input.on('error',close);output.on?.('error',close);
  return Object.freeze({close});
}
