import test from 'node:test';
import assert from 'node:assert/strict';
import {PassThrough} from 'node:stream';
import {createIGTrace,validateIGTrace,igFailure,igFailureDetail,recordIGError,igErrorTrace} from './instagram-diagnostic-trace.mjs';
import {serveMainBrowserTransport,MAIN_TRANSPORT_SCHEMA} from './main-browser-transport-protocol.mjs';
import {readInstagramEvidence} from './instagram-native-session.mjs';
const tick=()=>new Promise(r=>setImmediate(r)),nonce='a'.repeat(48);
test('closed trace keeps first cause independent, stays bounded and rejects forged metadata',()=>{
 const frames=[],trace=createIGTrace(value=>frames.push(value));trace.budget(30000);trace.note('body_read');trace.disposition('started');trace.fail('body_read','body_encoding');trace.fail('cleanup','cleanup_failed');trace.cleanup('uncertain','stream_close_failed');trace.cleanup('uncertain','cleanup_failed');
 const v=trace.snapshot();assert.equal(v.cause.condition,'body_encoding');assert.equal(v.cleanup.condition,'stream_close_failed');assert.ok(frames.every(v=>validateIGTrace(v)&&Buffer.byteLength(JSON.stringify(v))<=512));
 for(const bad of [{...v,body:'PRIVATE_MARKER'},{...v,complete:null},{...v,cause:{stage:'body_read'}},{...v,remainingMS:30001},{...v,sequence:1.5},{...v,cleanup:{state:'uncertain',condition:'PRIVATE_MARKER'}}])assert.equal(validateIGTrace(bad),false);
 assert.equal(igFailureDetail(Object.assign(Error('PRIVATE_MARKER'),{stage:'body_read',condition:'body_read'})),null);
 const fresh=createIGTrace();recordIGError(fresh,Error('PRIVATE_MARKER'),'scope');assert.equal(fresh.snapshot().cause.condition,'unclassified_exception');assert.ok(!JSON.stringify(fresh.snapshot()).includes('PRIVATE_MARKER'));
 assert.deepEqual(igFailureDetail(igFailure('scope','ownership')),{stage:'scope',condition:'ownership'});
});
test('Node guards are distinct and cannot claim adapter execution',async t=>{
 const input=new PassThrough(),output=new PassThrough(),cdpInput=new PassThrough(),cdpOutput=new PassThrough(),replies=[];let calls=0,finish;
 output.on('data',raw=>replies.push(JSON.parse(raw)));
 const server=serveMainBrowserTransport({input,output,connect:async()=>({input:cdpInput,output:cdpOutput}),diagnoseInstagramBootstrapCorroboration:()=>{calls++;return new Promise(r=>{finish=r});}});
 t.after(()=>{server.close();input.destroy();output.destroy();cdpInput.destroy();cdpOutput.destroy();});const send=q=>input.write(JSON.stringify(q)+'\n');
 send({op:'main_transport_bootstrap',schema:MAIN_TRANSPORT_SCHEMA,nonce,transport:'inherited-stdio'});await tick();
 send({id:1,op:'main_instagram_bootstrap_corroboration',nonce:'PRIVATE_MARKER'});await tick();
 const bad=replies.find(v=>v.id===1&&!v.event);assert.equal(bad.error.diagnostic.cause.condition,'nonce_invalid');assert.equal(bad.error.diagnostic.disposition,'not_started');assert.equal(calls,0);
 send({id:2,op:'main_instagram_bootstrap_corroboration',nonce,budgetMS:1000});await tick();send({id:3,op:'main_instagram_bootstrap_corroboration',nonce});await tick();assert.equal(replies.find(v=>v.id===3&&!v.event).error.diagnostic.cause.condition,'lane_busy');
 finish({schema:'aku.instagram-bootstrap-corroboration.v1',reason:'setup_failed',bodyBound:false,documentBound:false,serverFresh:false,idMatched:false,cookieMatched:false,scanComplete:false,qualified:false,cleanup:'uncertain'});await tick();
 send({id:4,op:'main_instagram_bootstrap_corroboration',nonce});await tick();assert.equal(replies.find(v=>v.id===4&&!v.event).error.diagnostic.cause.condition,'cleanup_blocked');assert.equal(calls,1);assert.ok(!JSON.stringify(replies).includes('PRIVATE_MARKER'));
});
test('production current_user HTTP400 retains a separate safe verifier observation',async()=>{
 const page={send:async method=>method==='Page.getFrameTree'?{frameTree:{frame:{id:'PRIVATE_FRAME',loaderId:'PRIVATE_LOADER',url:'https://www.instagram.com/'}}}:{},evaluate:async()=>({nonce,authenticated:false,accountId:'',rejection:'http_status_400'})};
 const attempt={binding:{nonce},run:async(_stage,fn)=>fn(1000)};
 await assert.rejects(readInstagramEvidence(page,attempt,'reader','PRIVATE_OWNER','PRIVATE_PROFILE','PRIVATE_SECRET'),error=>{const trace=igErrorTrace(error);assert.equal(trace.cause.condition,'http_400');assert.equal(trace.stage,'identity');assert.ok(!JSON.stringify(trace).includes('PRIVATE'));return true;});
});
