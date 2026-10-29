import test from 'node:test';
import assert from 'node:assert/strict';
import {runInNewContext} from 'node:vm';
import {instagramAccountExpression, readInstagramEvidence, instagramIdentityFailure, INSTAGRAM_IDENTITY_REJECTIONS, INSTAGRAM_HTTP_REJECTIONS} from './instagram-native-session.mjs';
import {nativeSourceDiagnostic} from './native-source-kernel.mjs';
import {sourceTransientClass} from './source-retry-policy.mjs';

const nonce='a'.repeat(48);
const valid={status:'ok',user:{pk:'123456',email:'PRIVATE_EMAIL',username:'PRIVATE_USERNAME'}};
function fixture(options={}) {
  let reads=0,fetches=0,canceled=0,forbiddenReads=0;
  const sandbox={location:{origin:'https://www.instagram.com',pathname:'/'},
    document:{querySelector:()=>options.login??null},URL,TextDecoder,Uint8Array,AbortController,setTimeout,clearTimeout,
    fetch:async(_url,request)=>{
      fetches++;
      if(options.timeout){await Promise.resolve();throw Error('PRIVATE_NETWORK');}
      if(options.network)throw Error('PRIVATE_NETWORK');
      if(options.move)sandbox.location.pathname='/accounts/login/';
      const bytes=options.bytes??new TextEncoder().encode(options.body??JSON.stringify(options.value??valid));
      let delivered=false;
      const response={status:options.status??200,url:options.url??'https://www.instagram.com/api/v1/accounts/current_user/?edit=true',
        headers:new Headers({'content-type':options.mime??'application/json',...(options.length?{'content-length':options.length}:{})}),
        body:options.missing?null:{getReader:()=>({read:async()=>{reads++;if(options.readFailure)throw Error('PRIVATE_BODY');if(delivered)return{done:true};delivered=true;return{done:false,value:bytes};},cancel:async()=>{canceled++;}})}};
      if(options.poison)for(const field of ['url','headers','body'])Object.defineProperty(response,field,{get(){forbiddenReads++;throw Error('PRIVATE_BODY_HEADERS_ACCOUNT');}});
      return response;
    }};
  if(options.page)sandbox.location.pathname='/other/';
  // Trigger the real AbortController state without waiting four seconds.
  if(options.timeout){sandbox.setTimeout=callback=>{queueMicrotask(callback);return 1;};sandbox.clearTimeout=()=>{};}
  return{sandbox,read:()=>runInNewContext(instagramAccountExpression(nonce),sandbox),counts:()=>({reads,fetches,canceled,forbiddenReads})};
}

test('account rejection categories are bounded, value-free and never authorize',async()=>{
  const cases=[
    [{page:true},'page_scope'],[{login:{}},'login_visible'],
    [{status:401},'http_unauthorized'],[{status:403},'http_forbidden'],[{status:429},'http_rate_limited'],
    [{status:0},'http_status'],
    [{url:'https://outside.invalid/PRIVATE_TOKEN'},'response_origin'],[{mime:'text/html'},'response_mime'],
    [{length:'65537'},'body_size'],[{body:'x'.repeat(65537)},'body_size'],[{missing:true},'body_missing'],
    [{readFailure:true},'body_read'],[{bytes:new Uint8Array([255])},'body_encoding'],[{body:'PRIVATE_TOKEN'},'body_json'],
    [{value:{status:'fail',message:'PRIVATE_TOKEN'}},'response_shape'],[{value:{status:'ok'}},'account_missing'],
    [{value:{status:'ok',user:{is_anonymous:true}}},'account_anonymous'],
    [{value:{status:'ok',user:{pk:9007199254740992}}},'account_type'],
    [{value:{status:'ok',user:{pk:'PRIVATE_ACCOUNT'}}},'account_format'],
    [{move:true},'page_changed'],[{network:true},'fetch_failed'],[{timeout:true},'fetch_timeout'],
  ];
  const seen=new Set();
  for(const [options,category] of cases){
    const f=fixture(options),result=JSON.parse(JSON.stringify(await f.read()));
    assert.deepEqual(result,{nonce,authenticated:false,accountId:'',rejection:category});
    assert.ok(!JSON.stringify(result).includes('PRIVATE'));seen.add(category);
    if(['page_scope','login_visible'].includes(category))assert.equal(f.counts().fetches,0);
    if(category.startsWith('http_')||category==='body_size'&&options.length)assert.equal(f.counts().reads,0);
  }
  assert.deepEqual([...seen].sort(),INSTAGRAM_IDENTITY_REJECTIONS.filter(reason=>!INSTAGRAM_HTTP_REJECTIONS.includes(reason)&&reason!=='http_server').sort());
});

test('every finite HTTP rejection is exact and precedes URL, headers and body access',async()=>{
  const seen=[];
  for(let status=100;status<=599;status++){
    if(status===200)continue;
    const category=({401:'http_unauthorized',403:'http_forbidden',429:'http_rate_limited'})[status]??`http_status_${status}`;
    const f=fixture({status,poison:true});
    assert.deepEqual(JSON.parse(JSON.stringify(await f.read())),{nonce,authenticated:false,accountId:'',rejection:category});
    assert.deepEqual(f.counts(),{reads:0,fetches:1,canceled:0,forbiddenReads:0});
    if(category.startsWith('http_status_'))seen.push(category);
  }
  assert.deepEqual(seen,INSTAGRAM_HTTP_REJECTIONS);
});

test('unsupported or injected status values cannot produce arbitrary diagnostic text',async()=>{
  for(const status of [0,99,600,-1,400.5,NaN,Infinity,'400','400_PRIVATE',{}, {valueOf(){throw Error('PRIVATE');},toString(){throw Error('PRIVATE');}}]){
    const f=fixture({status,poison:true});
    assert.equal((await f.read()).rejection,'http_status');
    assert.equal(f.counts().forbiddenReads,0);
  }
});

test('successful identity remains private, strict and bounded; cancellation retires the reader',async()=>{
  const f=fixture(),result=JSON.parse(JSON.stringify(await f.read()));
  assert.deepEqual(result,{nonce,authenticated:true,accountId:'123456',rejection:'none'});
  assert.ok(!JSON.stringify(result).includes('PRIVATE'));assert.equal(f.counts().canceled,1);
});

test('rejection role and category survive evidence validation without cookie access',async()=>{
  for(const role of ['reader','collector'])for(const category of INSTAGRAM_IDENTITY_REJECTIONS){
    let cookieReads=0;
    const projection={nonce,authenticated:false,accountId:'',rejection:category};
    const page={send:async method=>{
      if(method==='Network.getCookies')cookieReads++;
      return{frameTree:{frame:{id:'private-frame',loaderId:'private-document',url:'https://www.instagram.com/'}}};
    },evaluate:async()=>projection};
    const attempt={binding:{nonce},run:async(_stage,operation)=>operation(1000)};
    await assert.rejects(readInstagramEvidence(page,attempt,role,'PRIVATE_OWNER','PRIVATE_PROFILE','PRIVATE_SECRET'),error=>{
      assert.equal(error.message,'native_source_failed');
      assert.deepEqual(nativeSourceDiagnostic(error),{stage:'evidence',reason:`${role}_identity_${category}`,cleanup:'not_started'});
      assert.equal(sourceTransientClass(error),category==='fetch_timeout'?'timeout':'');
      assert.ok(!JSON.stringify(nativeSourceDiagnostic(error)).includes('PRIVATE'));return true;
    });
    assert.equal(cookieReads,0);
  }
});

test('projection forgery or unsupported categories cannot inject native error codes',async()=>{
  for(const patch of [{rejection:'PRIVATE_TOKEN'},...['099','600','200','401','403','429','0400','400.0','400_PRIVATE','400\n'].map(status=>({rejection:`http_status_${status}`})),{nonce:'b'.repeat(48)},{authenticated:true},{accountId:'123456'},{extra:'PRIVATE_BODY'}]){
    const page={send:async()=>({frameTree:{frame:{id:'f',loaderId:'d',url:'https://www.instagram.com/'}}}),
      evaluate:async()=>({nonce,authenticated:false,accountId:'',rejection:'http_forbidden',...patch})};
    await assert.rejects(readInstagramEvidence(page,{binding:{nonce},run:async(_stage,op)=>op(1000)},'reader','','',''),error=>nativeSourceDiagnostic(error).reason==='reader_identity_projection');
  }
  assert.equal(nativeSourceDiagnostic(instagramIdentityFailure('PRIVATE_ROLE','PRIVATE_REASON')).reason,'unknown_identity_projection');
});
