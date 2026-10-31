import test from 'node:test';
import assert from 'node:assert/strict';
import {PassThrough} from 'node:stream';
import {createMainCDP} from './main-browser-transport-protocol.mjs';
import {cdpCommandFailureDetail} from './cdp-command-failure.mjs';
import {createIGTrace} from './instagram-diagnostic-trace.mjs';
import {readInstagramBootstrapBodyStream} from './instagram-bootstrap-body-stream.mjs';

// Production CDP framing and scope, with an entirely synthetic browser peer.
function fixture(t,scenario){
  const input=new PassThrough(),output=new PassThrough(),requests=[];
  const transport=createMainCDP({input,output});t.after(()=>transport.close());
  output.on('data',wire=>{
    const q=JSON.parse(wire.toString().slice(0,-1));requests.push(q);let result={};
    if(q.method==='Target.createTarget')result={targetId:'synthetic-target'};
    if(q.method==='Target.attachToTarget')result={sessionId:'synthetic-session'};
    if(q.method==='Fetch.takeResponseBodyAsStream')result={stream:'synthetic-stream'};
    if(q.method==='IO.read'){
      if(scenario==='timeout')return;
      if(scenario==='closed'){input.end();return;}
      if(scenario==='cdp_error'){input.write(JSON.stringify({id:q.id,sessionId:q.sessionId,error:{code:-32000,message:'PRIVATE_ERROR'}})+'\0');return;}
      if(scenario==='invalid_result')result=[];
      else if(scenario==='invalid_reply')result={data:'ok'};
      else if(scenario==='empty')result={data:'',eof:false};
      else result={data:'ok',eof:true};
    }
    input.write(JSON.stringify({id:q.id,...(q.sessionId?{sessionId:q.sessionId}:{}),result})+'\0');
  });
  return {input,transport,requests};
}

async function prepare(f,trace){
  const scope=f.transport.createBootstrapCorroborationScope(trace);
  await scope.call('Target.createTarget',{url:'about:blank',background:true,forTab:false},'',1000);
  await scope.call('Target.attachToTarget',{targetId:'synthetic-target',flatten:true},'',1000);
  await scope.call('Fetch.enable',{patterns:[{urlPattern:'https://www.instagram.com/',resourceType:'Document',requestStage:'Response'}]},'synthetic-session',1000);
  f.input.write(JSON.stringify({method:'Fetch.requestPaused',sessionId:'synthetic-session',params:{requestId:'synthetic-request'}})+'\0');
  await scope.call('Fetch.takeResponseBodyAsStream',{requestId:'synthetic-request'},'synthetic-session',1000);
  return scope;
}

test('production transport preserves safe provenance for rejected command, bad reply and empty data',async t=>{
  for(const [scenario,condition] of [['cdp_error','read_cdp_rejected'],['invalid_result','read_reply_invalid'],['invalid_reply','read_reply_invalid'],['empty','read_empty_nonfinal'],['closed','read_transport_closed'],['timeout','read_command_timeout']]){
    await t.test(scenario,async t=>{
      const f=fixture(t,scenario),trace=createIGTrace(),scope=await prepare(f,trace);
      trace.note('body_read');const result=await readInstagramBootstrapBodyStream(scope,'synthetic-session','synthetic-stream',{deadlineAt:Date.now()+5000,trace});
      assert.equal(result.reason,'body_read');assert.equal(trace.snapshot().cause.condition,condition);
      assert.equal(result.cleanup,scenario==='closed'?'uncertain':'verified');
      assert.equal(f.requests.filter(q=>q.method==='IO.read').length,1);
      assert.ok(!JSON.stringify(trace.snapshot()).includes('PRIVATE'));scope.dispose();
    });
  }
});

test('local command timeout stays distinct while transport remains available',async t=>{
  const f=fixture(t,'timeout'),trace=createIGTrace(),scope=await prepare(f,trace);
  await assert.rejects(scope.call('IO.read',{handle:'synthetic-stream',size:1},'synthetic-session',5),error=>{
    assert.deepEqual(cdpCommandFailureDetail(error),{kind:'timeout'});assert.equal(error.message,'diagnostic_request_timeout');return true;
  });
  assert.equal(f.transport.alive(),true);await scope.call('IO.close',{handle:'synthetic-stream'},'synthetic-session',1000);scope.dispose();
});
