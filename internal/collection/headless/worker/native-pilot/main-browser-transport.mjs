import net from 'node:net';
import {PassThrough} from 'node:stream';
import {randomBytes} from 'node:crypto';
import {serveMainBrowserTransport,mainOwnerFailure} from './main-browser-transport-protocol.mjs';
import {serveDailyOwnerPipe,DAILY_OWNER_SCHEMA} from './daily-owner-protocol.mjs';
import {loadPackagedAssets} from './packaged-assets.mjs';
import {createApplicationDailyOwner} from '../../../../../scripts/auth-session-fixture/x-native-metadata-probe-driver.mjs';

const rejected=()=>new Error('main_transport_rejected');
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
    while((end=buffer.indexOf('\n'))>=0){const q=JSON.parse(buffer.slice(0,end));buffer=buffer.slice(end+1);
      if(q.ready){readyResolve();continue;}const p=pending.get(q.id);if(!p)continue;
      if(q.event)continue;pending.delete(q.id);q.ok?p.resolve(q.result):p.reject(mainOwnerFailure(q));
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
    async execute(operation,values){
      if(closed||!values||typeof values!=='object'||Array.isArray(values)||Object.hasOwn(values,'id')||Object.hasOwn(values,'op'))throw rejected();
      if(operation==='daily_owner_close'){if(Object.keys(values).length)throw rejected();await close();return{closed:true};}
      const id=++sequence;return new Promise((resolve,reject)=>{pending.set(id,{resolve,reject});input.write(JSON.stringify({...values,id,op:operation})+'\n');});
    },close,
  });
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
    onClosed:()=>{control.destroy();process.stdin.destroy();process.stdout.end();process.exitCode=0;},
  });
}
