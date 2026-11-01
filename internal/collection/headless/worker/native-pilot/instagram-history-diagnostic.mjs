// Diagnostic-only: observe navigation without wrapping History methods,
// intercepting/canceling events, pausing execution, or reading history state.
export const IG_HISTORY_SOURCE_URL='aku-instagram-history-diagnostic';
export const IG_HISTORY_MARKER='aku.ig.history.v1';
export const IG_HISTORY_SOURCE=`(()=>{
 if(globalThis!==globalThis.top||typeof navigation==='undefined')return;
 const report=console.debug.bind(console);let remaining=32;
 navigation.addEventListener('navigate',event=>{
  try{
   if(event.isTrusted!==true||event.destination.sameDocument!==true||event.hashChange||!['push','replace'].includes(event.navigationType))return;
   if(remaining===0){remaining--;report('${IG_HISTORY_MARKER}','unavailable');return;}if(remaining<0)return;
   remaining--;report('${IG_HISTORY_MARKER}',event.navigationType);
  }catch{}
 });
})();
//# sourceURL=${IG_HISTORY_SOURCE_URL}`;

const exact=(v,keys)=>!!v&&Object.getPrototypeOf(v)===Object.prototype&&Reflect.ownKeys(v).length===keys.length&&keys.every(k=>Object.hasOwn(Object.getOwnPropertyDescriptor(v,k)??{},'value'));
const coordinate=v=>Number.isSafeInteger(v)&&v>=1&&v<=10000000;
const script=v=>typeof v==='string'&&v.length<=512&&(v==='https://www.instagram.com/'||/^https:\/\/static\.cdninstagram\.com\/rsrc\.php\/(?:[A-Za-z0-9_-]+\/)+[A-Za-z0-9_-]+\.js$/.test(v));
export function validIGHistoryCaller(v){
 return exact(v,['method','callerScript','line','column'])&&['pushState','replaceState'].includes(v.method)&&
  (v.callerScript===''&&v.line===null&&v.column===null||script(v.callerScript)&&coordinate(v.line)&&coordinate(v.column));
}
export function projectIGHistoryCaller(params){
 try{
  if(params?.type!=='debug'||!Array.isArray(params.args)||params.args.length!==2||params.args.some(a=>a?.type!=='string')||params.args[0].value!==IG_HISTORY_MARKER||!['push','replace'].includes(params.args[1].value))return null;
  const frames=params.stackTrace?.callFrames;
  if(!Array.isArray(frames)||frames.length>32||frames[0]?.url!==IG_HISTORY_SOURCE_URL)return null;
  const value={method:params.args[1].value==='push'?'pushState':'replaceState',callerScript:'',line:null,column:null};
  // CDP coordinates are zero-based. Export one-based locations, never the
  // stack, function names, console objects, arguments, URL queries or fragments.
  const caller=frames.find(f=>f.url!==IG_HISTORY_SOURCE_URL);
  if(caller&&script(caller.url)&&Number.isSafeInteger(caller.lineNumber)&&Number.isSafeInteger(caller.columnNumber)&&coordinate(caller.lineNumber+1)&&coordinate(caller.columnNumber+1))Object.assign(value,{callerScript:caller.url,line:caller.lineNumber+1,column:caller.columnNumber+1});
  return value;
 }catch{return null;}
}

export function createIGHistoryDiagnostic(page,onCaller=()=>{},now=Date.now){
 let binding,pending,off,identifier,disposed=false;
 return Object.freeze({
  async install(timeoutMS){
   try{
    off=page.on(event=>{
     if(disposed||!binding)return;
     try{
      const p=event.params??{};
      if(event.method==='Runtime.consoleAPICalled'&&p.executionContextId===binding.contextId){
       const value=projectIGHistoryCaller(p);if(value)pending={value,at:now()};
       else if(p.args?.[0]?.value===IG_HISTORY_MARKER)pending=undefined;
      }else if(event.method==='Page.navigatedWithinDocument'&&p.frameId===binding.id){
       const candidate=pending;pending=undefined;
       const age=candidate?now()-candidate.at:-1;
       if(p.navigationType==='historyApi'&&candidate&&age>=0&&age<=1000)onCaller({...candidate.value});
      }else if(event.method==='Runtime.executionContextsCleared'||event.method==='Runtime.executionContextDestroyed'&&(p.executionContextId===binding.contextId||p.executionContextUniqueId===binding.uniqueContextId))pending=undefined;
     }catch{pending=undefined;}
    });
    const reply=await page.send('Page.addScriptToEvaluateOnNewDocument',{source:IG_HISTORY_SOURCE},timeoutMS);
    if(typeof reply?.identifier!=='string'||!reply.identifier||reply.identifier.length>512)throw Error('history_diagnostic_unavailable');
    identifier=reply.identifier;return true;
   }catch{try{off?.();}catch{}off=undefined;return false;}
  },
  bind(value){binding={id:value.id,contextId:value.contextId,uniqueContextId:value.uniqueContextId};pending=undefined;},
  async dispose(){
   disposed=true;binding=undefined;pending=undefined;let complete=true;
   try{off?.();}catch{complete=false;}off=undefined;
   if(identifier)try{await page.send('Page.removeScriptToEvaluateOnNewDocument',{identifier},1000);}catch{complete=false;}
   identifier=undefined;return complete;
  },
 });
}
