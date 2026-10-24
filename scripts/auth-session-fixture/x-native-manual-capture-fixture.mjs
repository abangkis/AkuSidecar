// Accountless CDP/DOM fixture: no browser, sockets, profile, or network client.
import vm from 'node:vm';
import readline from 'node:readline';
import {resolve} from 'node:path';
import {pathToFileURL} from 'node:url';
import {captureNativeXTarget} from './x-native-manual-capture.mjs';
import {createXManualOwnerPort} from './x-native-manual-owner-port.mjs';

export function syntheticXDOM() {
  const permalink='https://x.com/synthetic/status/1908123456789012346';
  const location={href:'https://x.com/home',hostname:'x.com',pathname:'/home',origin:'https://x.com'};
  const empty=()=>[];
  const base={nodeType:1,tagName:'DIV',querySelectorAll:empty,querySelector:()=>null,closest:()=>null,contains:()=>false,getAttribute:()=>null,matches:()=>false,getBoundingClientRect:()=>({top:0,bottom:200,left:0,right:600,width:600,height:200})};
  const text={...base,innerText:'Synthetic native worker parser evidence.',textContent:'Synthetic native worker parser evidence.',childNodes:[{nodeType:3,nodeValue:'Synthetic native worker parser evidence.'}],matches:s=>s.includes('tweetText'),getAttribute:k=>k==='data-testid'?'tweetText':null};
  const author={...base,innerText:'Synthetic Author',textContent:'Synthetic Author'};
  const anchor={...base,tagName:'A',href:permalink,getAttribute:k=>k==='href'?permalink:null};
  const time={...base,tagName:'TIME',dateTime:'2026-10-25T00:00:00Z',closest:()=>anchor};
  const article={...base,tagName:'ARTICLE',innerText:text.innerText,childNodes:[text],matches:s=>s.includes('article'),
    querySelectorAll:s=>s==='time'?[time]:s.includes('tweetText')?[text]:s==='a[href]'||s.includes('a[href*="/status/"]')?[anchor]:[],
    querySelector:s=>s.includes('tweetText')?text:s==='[data-testid="User-Name"]'?author:null};
  const document={title:'Synthetic X DOM',readyState:'complete',visibilityState:'visible',body:{innerText:text.innerText},scrollingElement:{scrollHeight:900},
    querySelectorAll:s=>s==='article[data-testid="tweet"]'||s==='main article'?[article]:[],
    querySelector:s=>s==='main'||s==='[data-testid="SideNav_AccountSwitcher_Button"]'?base:null};
  const context=vm.createContext({URL,Date,console:{log(){}},setTimeout,clearTimeout,location,document,innerHeight:900,innerWidth:1200,scrollY:0,Node:{ELEMENT_NODE:1,TEXT_NODE:3},getComputedStyle:()=>({display:'block',visibility:'visible'}),scrollTo(){},scrollBy(){}});
  vm.runInContext('globalThis.window=globalThis;globalThis.addEventListener=()=>{};globalThis.removeEventListener=()=>{};',context);
  return {context,location};
}
export function syntheticCaptureTransport({hold,parserThrow=false,cookie=false,pendingExtra=false,lateCookie=false,websocket=false}={}) {
  const dom=syntheticXDOM(),listeners=new Set();let closed=false,registered=false;
  let targetCreatedResolve;
  const targetCreated=new Promise(resolve=>{targetCreatedResolve=resolve;});
  return {
    dom,events:[],targetCreated,
    subscribe(fn){listeners.add(fn);return()=>listeners.delete(fn);},
    get closed(){return closed;},
    async command(method,params={},session){
      this.events.push(method);
      if(method==='Target.createTarget'){targetCreatedResolve();return{targetId:'capture-target'};}
      if(method==='Target.attachToTarget')return{sessionId:'capture-session'};
      if(method==='Target.closeTarget'){closed=true;if(lateCookie)for(const listener of listeners)listener({sessionId:'capture-session',method:'Network.responseReceivedExtraInfo',params:{requestId:'feed-request',statusCode:200,headers:{'set-cookie':'ct0=late-synthetic; Path=/'}}});return{success:true};}
      if(method==='Page.getFrameTree')return{frameTree:{frame:{id:'capture-frame',loaderId:'capture-loader'}}};
      if(method==='Network.enable'){registered=true;return{};}
      if(method==='Page.addScriptToEvaluateOnNewDocument')return{identifier:'passive-hook'};
      if(method==='Page.navigate'){
        if(!registered)throw Error('observer_not_registered');
        if(websocket)for(const listener of listeners)listener({sessionId:session,method:'Network.webSocketCreated',params:{requestId:'websocket-id',url:'wss://synthetic.invalid'}});
        for(const listener of listeners){listener({sessionId:session,method:'Network.requestWillBeSent',params:{requestId:'feed-request',frameId:'capture-frame',request:{url:params.url,method:'GET'}}});
          if(cookie)listener({sessionId:session,method:'Network.responseReceivedExtraInfo',params:{requestId:'feed-request',statusCode:200,headers:{'set-cookie':'ct0=synthetic; Path=/'}}});}
        if(hold)await hold;
        for(const listener of listeners){listener({sessionId:session,method:'Network.responseReceived',params:{requestId:'feed-request',hasExtraInfo:pendingExtra,response:{status:200}}});listener({sessionId:session,method:'Network.loadingFinished',params:{requestId:'feed-request'}});}
        dom.location.href=params.url;dom.location.pathname=new URL(params.url).pathname;return{};
      }
      if(method==='Runtime.evaluate'){
        if(closed)throw Error('capture_target_closed');
        if(parserThrow&&params.expression.includes('XHeadlessPoC.collect'))throw Error('synthetic_parser_failure');
        const value=await vm.runInContext(params.expression,dom.context,{timeout:1000});
        return{result:{value}};
      }
      return{};
    },
  };
}

export async function syntheticCapturePort(pins,transport,bridgePath) {
  const roles=['reader','collector'];
  const rolePins={collector:pins,reader:{...pins,ownerId:'4101',profileId:'synthetic-reader',targetId:'proof-reader',contextId:'context-reader',frameId:'frame-reader',documentId:'document-reader'}};
  const port=createXManualOwnerPort({enabled:true,owner:1,createOwners:()=>Object.fromEntries(roles.map(role=>[role,{
    async prepare(){return{handle:role+'-handle',pins:rolePins[role]};},async acquire(){return Buffer.from('{}');},async abort(){},async cancel(){}}])),
    snapshot:async role=>({...rolePins[role],sessionId:role+'-session',alive:true,headless:role==='collector',windowCount:role==='reader'?1:0,bootstrapCount:role==='reader'?1:0,lifecycleReady:true}),retire:async()=>{},
    capture:(request,options)=>captureNativeXTarget({request,bridgePath,...options,browserContextId:'owned-context',command:transport.command.bind(transport),subscribe:transport.subscribe.bind(transport)})});
  let id=1;
  for(const role of roles)await port.execute({id:id++,op:'manual_prepare',owner:1,role});
  for(const role of roles)await port.execute({id:id++,op:'manual_acquire',owner:1,role,handle:role+'-handle',request:{challenge:'synthetic',role,...rolePins[role]}});
  return port;
}

if(process.argv[1]&&pathToFileURL(resolve(process.argv[1])).href===import.meta.url){
  if(process.env.AKU_X_NATIVE_MANUAL_CAPTURE_OFFLINE!=='1')process.exitCode=2;
  else{
    const input=readline.createInterface({input:process.stdin,crlfDelay:Infinity});let port;
    input.on('line',line=>{void(async()=>{let message;try{message=JSON.parse(line);port??=await syntheticCapturePort(message.request.pins,syntheticCaptureTransport(),resolve(process.env.AKU_NATIVE_MANUAL_PROJECT_ROOT,'..','AkuBridge'));const result=await port.execute(message);process.stdout.write(JSON.stringify({id:message.id,ok:true,result})+'\n');}catch{process.stdout.write(JSON.stringify({id:message?.id??0,ok:false,error:'synthetic_capture_rejected'})+'\n');}})();});
  }
}
