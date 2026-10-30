import net from 'node:net';
import {PassThrough} from 'node:stream';
import {randomBytes} from 'node:crypto';
import {serveMainBrowserTransport,mainOwnerFailure,mainStartupProgressForOwnerOperation} from './main-browser-transport-protocol.mjs';
import {serveDailyOwnerPipe,DAILY_OWNER_SCHEMA} from './daily-owner-protocol.mjs';
import {loadPackagedAssets} from './packaged-assets.mjs';
import {createApplicationDailyOwner} from '../../../../../scripts/auth-session-fixture/x-native-metadata-probe-driver.mjs';
import {parseStrictJSON} from '../../../../../scripts/auth-session-fixture/x-evidence-ingress.mjs';
import {createInstagramDailyOwner} from './instagram-native-provider.mjs';
import {INSTAGRAM_NATIVE_RECIPE} from './instagram-native-session.mjs';
import {nativeSourceFailure} from './native-source-kernel.mjs';
import {runInstagramReaderDiagnostic} from './instagram-reader-diagnostic.mjs';
import {diagnoseInstagramSelfBootstrap} from './instagram-self-bootstrap-probe.mjs';
import {runInstagramBootstrapCorroboration} from './instagram-bootstrap-corroboration.mjs';

const rejected=()=>new Error('main_transport_rejected');
const exact=(v,keys)=>{if(!v||typeof v!=='object'||Array.isArray(v)||Object.keys(v).length!==keys.length||keys.some(k=>!Object.hasOwn(v,k)))throw rejected();};
async function connectPipe(path){
  return new Promise((resolve,reject)=>{
    const socket=net.createConnection(path),timer=setTimeout(()=>{socket.destroy();reject(rejected());},10000);
    socket.once('connect',()=>{clearTimeout(timer);resolve(socket);});
    socket.once('error',()=>{clearTimeout(timer);reject(rejected());});
  });
}

// This adapter reuses the complete one-generation-at-a-time daily validator.
// Its internal pipe never crosses the Node process/secret boundary.
export async function createBorrowedDailyControl(params,transport,{createOwner=createApplicationDailyOwner,loadAssets=loadPackagedAssets}={}){
  const keys=['chromePath','readerProfile','readerPID','profileDirectory','acknowledgeLimitations','allowProcessSessionDerivation'];
  if(!params||Object.keys(params).length!==keys.length||keys.some(k=>!Object.hasOwn(params,k))||
    params.acknowledgeLimitations!==true||params.allowProcessSessionDerivation!==true||
    !Number.isSafeInteger(params.readerPID)||params.readerPID<1||typeof params.profileDirectory!=='string'||
    !params.profileDirectory||params.profileDirectory.length>128||/[\\/\u0000-\u001f]/u.test(params.profileDirectory))throw rejected();
  const input=new PassThrough(),output=new PassThrough(),pending=new Map();
  let owner,buffer='',closed=false,sequence=0,readyResolve,readyReject;
  const ready=new Promise((resolve,reject)=>{readyResolve=resolve;readyReject=reject;});
  output.on('data',chunk=>{
    buffer+=chunk.toString('utf8');let end;
    while((end=buffer.indexOf('\n'))>=0){let q;const line=buffer.slice(0,end);buffer=buffer.slice(end+1);
      try{q=parseStrictJSON(line,{maxJSONDepth:12,maxJSONTokens:4096});}catch{continue;}
      if(q?.ready){readyResolve();continue;}
      if(q&&Object.hasOwn(q,'event')){
        if(q.event!=='manual_startup_progress')continue;
        try{exact(q,['event','id','result']);}catch{continue;}
        const p=pending.get(q.id);if(!p||!Number.isSafeInteger(p.requestID)||p.requestID<1||typeof p.report!=='function'||p.progressCount>=32)continue;
        const progress=mainStartupProgressForOwnerOperation(p.operation,q.result);if(!progress)continue;
        p.progressCount++;try{p.report(progress);}catch{}
        continue;
      }
      const p=pending.get(q?.id);if(!p)continue;pending.delete(q.id);q.ok?p.resolve(q.result):p.reject(mainOwnerFailure(q));
    }
  });
  serveDailyOwnerPipe({input,output,
    createOwner:async(scope,emitProgress)=>{owner=await createOwner({...scope,loadAssets,emitProgress,
      borrowedReader:{pid:params.readerPID,profileDirectory:params.profileDirectory,transport}});return owner;},
    onClosed:()=>{closed=true;readyReject(rejected());for(const p of pending.values())p.reject(rejected());pending.clear();},
  });
  input.write(JSON.stringify({op:'daily_owner_bootstrap',schema:DAILY_OWNER_SCHEMA,nonce:randomBytes(24).toString('hex'),
    chromePath:params.chromePath,readerProfile:params.readerProfile})+'\n');
  await ready;
  const close=async()=>{if(closed)return;await owner.close();closed=true;input.end();};
  return Object.freeze({
    async execute(operation,values,requestID,report){
      if(closed||!values||typeof values!=='object'||Array.isArray(values)||Object.hasOwn(values,'id')||Object.hasOwn(values,'op'))throw rejected();
      if(requestID!==undefined&&(!Number.isSafeInteger(requestID)||requestID<1)||report!==undefined&&typeof report!=='function')throw rejected();
      if(operation==='daily_owner_close'){if(Object.keys(values).length)throw rejected();await close();return{closed:true};}
      const id=++sequence;return new Promise((resolve,reject)=>{pending.set(id,{resolve,reject,operation,requestID,report,progressCount:0});input.write(JSON.stringify({...values,id,op:operation})+'\n');});
    },close,
  });
}

// Qualification is a sealed implementation checkpoint. A control request cannot
// supply a manifest or turn fixture evidence into native permission.
export async function createBorrowedInstagramControl(params,transport){
  const trialKeys=['chromePath','readerProfile','readerPID','profileDirectory','acknowledgeLimitations','allowProcessSessionDerivation','acknowledgeUnqualifiedRecipe'];
  const trial=params&&Object.keys(params).length===trialKeys.length&&trialKeys.every(key=>Object.hasOwn(params,key))&&params.acknowledgeUnqualifiedRecipe===true;
  if(INSTAGRAM_NATIVE_RECIPE.nativeQualified!==true&&!trial)throw nativeSourceFailure('qualification','required');
  const values={...params};delete values.acknowledgeUnqualifiedRecipe;
  return createInstagramDailyOwner(values,transport,{loadAssets:loadPackagedAssets});
}

export async function runMainBrowserTransport(){
  const [controlPipe,nonce,...extra]=process.argv.slice(2);
  if(extra.length||typeof controlPipe!=='string'||!/^\\\\\.\\pipe\\AkuBrowser\.main-transport\.[a-f0-9]{48}$/u.test(controlPipe)||
    typeof nonce!=='string'||!/^[a-f0-9]{48}$/u.test(nonce))throw rejected();
  const control=await connectPipe(controlPipe);
  return serveMainBrowserTransport({input:control,output:control,expectedNonce:nonce,
    // Standard inherited handles become valid Node fd 0/1. Go retains its
    // duplicate endpoints without consuming bytes, preventing transport-worker
    // exit alone from sending EOF to the independently owned main Chrome.
    connect:async()=>({input:process.stdin,output:process.stdout}),
    createX:(params,transport)=>createBorrowedDailyControl(params,transport),
    sourceFactories:{instagram:createBorrowedInstagramControl},
    diagnoseInstagramReader:runInstagramReaderDiagnostic,
    diagnoseInstagramBootstrap:diagnoseInstagramSelfBootstrap,
    diagnoseInstagramBootstrapCorroboration:runInstagramBootstrapCorroboration,
    onClosed:()=>{control.destroy();process.stdin.destroy();process.stdout.end();process.exitCode=0;},
  });
}
