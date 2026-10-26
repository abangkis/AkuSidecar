// Private application pipe. This module owns framing, never browser authority.
import {parseStrictJSON} from '../../../../../scripts/auth-session-fixture/x-evidence-ingress.mjs';
import {manualOwnerOperation} from '../../../../../scripts/auth-session-fixture/x-native-manual-owner-port.mjs';
import {startupRPCCode} from '../../../../../scripts/auth-session-fixture/x-manual-startup-diagnostic.mjs';
import {manualSeedRPCCode} from '../../../../../scripts/auth-session-fixture/x-native-metadata-probe-driver.mjs';
import {manualCaptureRPCCode,manualCaptureRPCDiagnostic} from '../../../../../scripts/auth-session-fixture/x-native-manual-capture.mjs';
import {MANUAL_TIMING} from '../../../../../scripts/auth-session-fixture/x-manual-timing-policy.mjs';

export const OWNER_SCHEMA='aku.x.application-owner.v1';
const MAX_LINE=65536,MAX_PENDING=32,MAX_CALLS=512;
const failed=()=>new Error('application_owner_rejected');
function exact(value,keys){
  if(!value||![Object.prototype,null].includes(Object.getPrototypeOf(value)))throw failed();
  const descriptors=Object.getOwnPropertyDescriptors(value),names=Reflect.ownKeys(descriptors);
  if(names.length!==keys.length||names.some(k=>!keys.includes(k)||!Object.hasOwn(descriptors[k],'value')||!descriptors[k].enumerable))throw failed();
}
export function ownerBootstrap(value){
  exact(value,['op','schema','nonce','chromePath','readerProfile','collectorProfile']);
  if(value.op!=='native_owner_bootstrap'||value.schema!==OWNER_SCHEMA||typeof value.nonce!=='string'||!/^([a-f0-9]{48})$/u.test(value.nonce))throw failed();
  for(const key of ['chromePath','readerProfile','collectorProfile'])if(typeof value[key]!=='string'||value[key].length<1||value[key].length>1024||/[\u0000-\u001f\u007f]/u.test(value[key]))throw failed();
  return value;
}
export function ownerOperation(value){
  const operation=value&&Object.getOwnPropertyDescriptor(value,'op')?.value;
  if(['native_owner_initialize','native_owner_seed','manual_open_reader'].includes(operation)){
    exact(value,operation==='native_owner_initialize'?['id','op']:['id','op','owner']);
    if(!Number.isSafeInteger(value.id)||value.id<1||operation!=='native_owner_initialize'&&(!Number.isSafeInteger(value.owner)||value.owner<1))throw failed();
    return value;
  }
  return manualOwnerOperation(value);
}
export function ownerErrorReply(message,error){
  if(message.op==='manual_capture')return {id:message.id,ok:false,code:manualCaptureRPCCode(error),...(manualCaptureRPCDiagnostic(error)?{diagnostic:manualCaptureRPCDiagnostic(error)}:{})};
  const code=message.op==='native_owner_seed'?manualSeedRPCCode(error):message.op.startsWith('manual_')?startupRPCCode(error):'application_owner_failed';
  return{id:message.id,ok:false,code};
}

// Initialization and seed are separately one-use, including failed attempts.
// Manual actor operations stay concurrent so the rendezvous and cancel work.
export function serveOwnerPipe({input,output,createOwner,onClosed=()=>{},initializeBudgetMS=MANUAL_TIMING.collectorHomeReadyMS}){
  if(!Number.isSafeInteger(initializeBudgetMS)||initializeBudgetMS<1||initializeBudgetMS>MANUAL_TIMING.collectorHomeReadyMS)throw failed();
  let bytes=Buffer.alloc(0),owner,booting=false,bootstrapped=false,closed=false,initializing=false,initialized=false,seeding=false,seeded=false;
  let initializeUsed=false,seedUsed=false,pending=0,calls=0;
  const seen=new Set(),decoder=new TextDecoder('utf-8',{fatal:true,ignoreBOM:true});
  const close=()=>{
    if(closed)return;closed=true;bytes.fill(0);bytes=Buffer.alloc(0);input.pause();
    // The enclosing Go Job terminates the owned tree on EOF/pipe failure. A
    // close promise is best effort here and must never keep accepting work.
    void Promise.resolve().then(()=>owner?.close()).catch(()=>{}).finally(()=>onClosed());
  };
  const emit=value=>{
    if(closed)return;
    let wire;
    try{wire=Buffer.from(JSON.stringify(value)+'\n');if(wire.length>800*1024)throw failed();}
    catch{close();return;}
    try{output.write(wire,error=>{wire.fill(0);if(error)close();});}catch{wire.fill(0);close();}
  };
  const progress=value=>{if(value?.event==='manual_startup_progress'&&seen.has(value.id))emit(value);};
  const handle=async line=>{
    let message,result,resultWire;
    try{
      message=parseStrictJSON(decoder.decode(line),{maxJSONDepth:12,maxJSONTokens:4096});
      if(!bootstrapped){
        if(booting)throw failed();booting=true;
        ownerBootstrap(message);
        owner=await createOwner(message,progress);
        if(!owner||!['initialize','seed','execute','close'].every(k=>typeof owner[k]==='function'))throw failed();
        if(closed){void owner.close();return;}
        bootstrapped=true;emit({ready:true,schema:OWNER_SCHEMA,nonce:message.nonce});return;
      }
      ownerOperation(message);
      if(++calls>MAX_CALLS||seen.has(message.id)||pending>=MAX_PENDING)throw failed();
      seen.add(message.id);pending++;
      try{
        if(message.op==='native_owner_initialize'){
          if(initializeUsed)throw failed();initializeUsed=true;initializing=true;
          let timer;
          try{result=await Promise.race([owner.initialize(),new Promise((_,reject)=>{timer=setTimeout(()=>{close();reject(failed());},initializeBudgetMS);})]);initialized=true;}
          finally{clearTimeout(timer);initializing=false;}
        }else if(message.op==='native_owner_seed'){
          if(!initialized||initializing||seedUsed)throw failed();seedUsed=true;seeding=true;
          try{result=await owner.seed(message);seeded=true;}finally{seeding=false;}
        }else{
          if(!seeded||seeding||initializing)throw failed();
          result=await owner.execute(message);
        }
        if(Buffer.isBuffer(result)){resultWire=result;result=parseStrictJSON(result.toString('utf8'),{maxJSONDepth:4,maxJSONTokens:96});}
        emit({id:message.id,ok:true,result});
      }catch(error){emit(ownerErrorReply(message,error));}
      finally{pending--;}
    }catch{close();}
    finally{if(Buffer.isBuffer(result))result.fill(0);resultWire?.fill(0);line.fill(0);}
  };
  input.on('data',chunk=>{
    if(closed)return;
    if(!(chunk instanceof Uint8Array)||chunk.length>131072||bytes.length+chunk.length>131072){close();return;}
    const old=bytes;bytes=Buffer.concat([bytes,chunk]);old.fill(0);
    let count=0,end;
    while((end=bytes.indexOf(10))>=0){
      if(++count>8||end<1||end>MAX_LINE){close();return;}
      const line=Buffer.from(bytes.subarray(0,end)),previous=bytes;bytes=Buffer.from(previous.subarray(end+1));previous.fill(0);
      // The first bootstrap is reserved synchronously before awaiting creation.
      void handle(line);
      if(closed)return;
    }
    if(bytes.length>MAX_LINE)close();
  });
  input.on('end',close);input.on('error',close);output.on?.('error',close);
  return Object.freeze({close});
}
