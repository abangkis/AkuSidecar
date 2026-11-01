import test from 'node:test';
import assert from 'node:assert/strict';
import {runInNewContext} from 'node:vm';
import {createIGHistoryDiagnostic,projectIGHistoryCaller,validIGHistoryCaller,IG_HISTORY_SOURCE,IG_HISTORY_SOURCE_URL,IG_HISTORY_MARKER} from './instagram-history-diagnostic.mjs';
const url='https://static.cdninstagram.com/rsrc.php/v4/test/r/caller-fixture.js';
const report=(type='replace',caller={url,lineNumber:1328,columnNumber:408})=>({type:'debug',executionContextId:7,args:[{type:'string',value:IG_HISTORY_MARKER},{type:'string',value:type}],stackTrace:{callFrames:[{url:IG_HISTORY_SOURCE_URL,lineNumber:0,columnNumber:0},caller]}});
function fixture(now=()=>100){
 const listeners=new Set(),observed=[],calls=[];
 const page={on(fn){listeners.add(fn);return()=>listeners.delete(fn);},async send(method,params){calls.push({method,params});return method==='Page.addScriptToEvaluateOnNewDocument'?{identifier:'owned-script'}:{};}};
 const diagnostic=createIGHistoryDiagnostic(page,v=>observed.push(v),now);
 return{diagnostic,listeners,observed,calls,page,emit:(method,params)=>{for(const fn of listeners)fn({method,params});}};
}
test('caller projection uses closed methods and one-based locations without retaining stack or arguments',()=>{
 const raw=report();raw.stackTrace.callFrames[1].functionName='PRIVATE';
 assert.deepEqual(projectIGHistoryCaller(raw),{method:'replaceState',callerScript:url,line:1329,column:409});
 assert.equal(projectIGHistoryCaller(report('push')).method,'pushState');
 for(const caller of [{url:url+'?PRIVATE',lineNumber:0,columnNumber:0},{url:'https://PRIVATE@static.cdninstagram.com/rsrc.php/v4/x/r/a.js',lineNumber:0,columnNumber:0},{url:'file:///PRIVATE',lineNumber:0,columnNumber:0},{url,lineNumber:-1,columnNumber:0},{url,lineNumber:0.5,columnNumber:0}]){
  assert.deepEqual(projectIGHistoryCaller(report('push',caller)),{method:'pushState',callerScript:'',line:null,column:null});
 }
 const bad=report();bad.stackTrace.callFrames.shift();assert.equal(projectIGHistoryCaller(bad),null);
 assert.equal(projectIGHistoryCaller(report('PRIVATE')),null);
 assert.equal(projectIGHistoryCaller({...report(),args:[{type:'string',value:IG_HISTORY_MARKER},{type:'object',value:'replace'}]}),null);
 const oversized=report();oversized.stackTrace.callFrames=Array(33).fill({url:IG_HISTORY_SOURCE_URL});assert.equal(projectIGHistoryCaller(oversized),null);
 assert.ok(!JSON.stringify(projectIGHistoryCaller(raw)).includes('PRIVATE'));
});
test('wire caller validation rejects unknown properties, URLs, coordinates and null objects',()=>{
 const value=projectIGHistoryCaller(report());assert.ok(validIGHistoryCaller(value));
 for(const bad of [null,{}, {...value,method:'PRIVATE'},{...value,callerScript:url+'#PRIVATE'},{...value,callerScript:'https://static.cdninstagram.com/rsrc.php/v4/../r/a.js'},{...value,line:null},{...value,line:0},{...value,column:10000001},{...value,stack:'PRIVATE'},{...value,callerScript:'',line:1}])assert.equal(validIGHistoryCaller(bad),false);
});
test('passive document listener leaves native methods intact and never intercepts navigation',()=>{
 const logs=[],handlers=[],push=()=>{},replace=()=>{},history={pushState:push,replaceState:replace};
 const sandbox={history,console:{debug:(...args)=>logs.push(args)},navigation:{addEventListener:(type,fn)=>{assert.equal(type,'navigate');handlers.push(fn);}}};sandbox.top=sandbox;
 runInNewContext(IG_HISTORY_SOURCE,sandbox);
 assert.equal(history.pushState,push);assert.equal(history.replaceState,replace);assert.equal(handlers.length,1);
 const emit=(type,trusted=true)=>handlers[0]({isTrusted:trusted,navigationType:type,hashChange:false,destination:{sameDocument:true},intercept(){assert.fail('intercept called');},preventDefault(){assert.fail('preventDefault called');}});
 emit('replace',false);emit('traverse');assert.equal(logs.length,0);
 for(let i=0;i<40;i++)emit(i%2?'push':'replace');
 assert.equal(logs.length,33);assert.deepEqual(logs.at(-1),[IG_HISTORY_MARKER,'unavailable']);
 assert.ok(logs.every(v=>v.length===2));
});
test('caller is published only for a fresh event in the bound root context and History API navigation',async()=>{
 let now=100;const f=fixture(()=>now);assert.equal(await f.diagnostic.install(1000),true);
 f.emit('Runtime.consoleAPICalled',report());
 f.diagnostic.bind({id:'root',contextId:7,uniqueContextId:'context'});
 f.emit('Page.navigatedWithinDocument',{frameId:'root',navigationType:'historyApi'});assert.equal(f.observed.length,0);
 f.emit('Runtime.consoleAPICalled',{...report(),executionContextId:8});
 f.emit('Page.navigatedWithinDocument',{frameId:'root',navigationType:'historyApi'});assert.equal(f.observed.length,0);
 f.emit('Runtime.consoleAPICalled',report());
 f.emit('Page.navigatedWithinDocument',{frameId:'foreign',navigationType:'historyApi'});assert.equal(f.observed.length,0);
 f.emit('Page.navigatedWithinDocument',{frameId:'root',navigationType:'historyApi'});assert.equal(f.observed.length,1);
 f.emit('Page.navigatedWithinDocument',{frameId:'root',navigationType:'historyApi'});assert.equal(f.observed.length,1);
 f.emit('Runtime.consoleAPICalled',report('push'));f.emit('Page.navigatedWithinDocument',{frameId:'root',navigationType:'fragment'});
 f.emit('Page.navigatedWithinDocument',{frameId:'root',navigationType:'historyApi'});assert.equal(f.observed.length,1);
 f.emit('Runtime.consoleAPICalled',report());now+=1001;f.emit('Page.navigatedWithinDocument',{frameId:'root',navigationType:'historyApi'});assert.equal(f.observed.length,1);
 assert.equal(await f.diagnostic.dispose(),true);assert.equal(f.listeners.size,0);
 assert.deepEqual(f.calls.map(c=>c.method),['Page.addScriptToEvaluateOnNewDocument','Page.removeScriptToEvaluateOnNewDocument']);
});
test('context loss, saturation and malformed reports discard pending attribution',async()=>{
 for(const event of [{method:'Runtime.executionContextsCleared',params:{}},{method:'Runtime.executionContextDestroyed',params:{executionContextId:7}},{method:'Runtime.consoleAPICalled',params:report('unavailable')}]){
  const f=fixture();await f.diagnostic.install(1000);f.diagnostic.bind({id:'root',contextId:7});
  f.emit('Runtime.consoleAPICalled',report());f.emit(event.method,event.params);f.emit('Page.navigatedWithinDocument',{frameId:'root',navigationType:'historyApi'});
  assert.equal(f.observed.length,0);await f.diagnostic.dispose();
 }
});
test('optional installation and retirement failures never escape into the shared reader',async()=>{
 const f=fixture();f.page.send=async()=>{throw Error('PRIVATE');};assert.equal(await f.diagnostic.install(1000),false);assert.equal(f.listeners.size,0);assert.equal(await f.diagnostic.dispose(),true);
 const g=fixture();await g.diagnostic.install(1000);g.page.send=async()=>{throw Error('PRIVATE');};assert.equal(await g.diagnostic.dispose(),false);assert.equal(g.listeners.size,0);
});
