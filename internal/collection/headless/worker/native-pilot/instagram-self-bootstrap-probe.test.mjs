import {INSTAGRAM_BOOTSTRAP_BUDGET} from './instagram-bootstrap-budget.mjs';
import test from 'node:test';
import assert from 'node:assert/strict';
import {runInNewContext} from 'node:vm';
import {instagramSelfBootstrapExpression, runInstagramSelfBootstrapProbe, validateInstagramSelfBootstrapProbe, validateInstagramBootstrapDiagnostic, INSTAGRAM_BOOTSTRAP_BOUNDS, assessInstagramBootstrapDelivery} from './instagram-self-bootstrap-probe.mjs';
const tuple = (id='123456789', username='synthetic_self') => ['PolarisViewer', [], {id, username}, 1];
function read(values, options={}) {
  let forbidden=0;
  const sandbox={TextEncoder,performance:{now:options.clock??(()=>0)},location:{origin:'https://www.instagram.com',pathname:options.path??'/'},
    document:{querySelector: selector => options.login && selector.includes('password') ? {} : null,
      querySelectorAll:()=>values.map(value=>({textContent:typeof value==='string'?value:JSON.stringify(value)}))},
    fetch:()=>{forbidden++;throw Error('network forbidden');}, require:()=>{forbidden++;throw Error('module execution forbidden');}};
  const result=JSON.parse(JSON.stringify(runInNewContext(instagramSelfBootstrapExpression(options.policy),sandbox)));
  assert.equal(forbidden,0);
  return validateInstagramSelfBootstrapProbe(result);
}
test('candidate probe returns shape only and never qualifies identity or freshness',()=>{
  const result=read([{__bbox:{define:[tuple()]}}]);
  assert.equal(result.reason,'candidate');assert.equal(result.modulePresent,true);
  assert.equal(result.idString,true);assert.equal(result.usernameString,true);
  assert.equal(result.qualified,false);assert.equal(result.serverFresh,false);
  assert.doesNotMatch(JSON.stringify(result),/123456789|synthetic_self/);
});
test('post-author identity and arbitrary viewer objects cannot substitute for exact module',()=>{
  for(const value of [{user:{id:'123',username:'author'},code:'POST'}, {viewer:{id:'123',username:'self'}},['NotPolarisViewer',[],{id:'123',username:'self'},1]])assert.equal(read([value]).reason,'missing');
});
test('duplicates, conflicts and unsafe numeric identity reject',()=>{
  assert.equal(read([[tuple(),tuple()]]).reason,'duplicate');
  assert.equal(read([[tuple(),tuple('456')]]).reason,'conflict');
  assert.equal(read([tuple(Number.MAX_SAFE_INTEGER+1)]).reason,'invalid_json');
  assert.equal(read([tuple('0')]).reason,'shape');
  assert.equal(read([['PolarisViewer', ['dependency'], {id:'123',username:'self'},1]]).reason,'shape');
});
test('shape failures expose only a closed predicate category and remain incomplete',()=>{
  const data=(value={id:'123',username:'self'})=>['PolarisViewer',[],value,1];
  const cases=[
    [tuple().slice(0,3),'tuple_arity'],[tuple().concat(2),'tuple_arity'],
    [['PolarisViewer',null,{id:'123',username:'self'},1],'dependencies'],
    [['PolarisViewer',['dependency'],{id:'123',username:'self'},1],'dependencies'],
    [data(null),'data_object'],[data([]),'data_object'],
    [data({username:'self'}),'missing_id'],[data({id:'123'}),'missing_username'],
    [data({id:true,username:'self'}),'id_type'],
    ...['0','01','9'.repeat(31),0,-1,1.5].map(id=>[data({id,username:'self'}),'id_format']),
    [data({id:'123',username:7}),'username_type'],
    ...['','a'.repeat(31),'user-name','\u00e9'].map(username=>[data({id:'123',username}),'username_format']),
  ];
  for(const [candidate,shapeReason] of cases){
    const result=read([candidate]);
    assert.equal(result.reason,'shape');assert.equal(result.shapeReason,shapeReason);
    assert.equal(result.modulePresent,true);assert.equal(result.scanComplete,false);
    assert.equal(result.qualified,false);assert.equal(result.serverFresh,false);
    assert.equal(result.idString,false);assert.equal(result.idSafeNumber,false);assert.equal(result.usernameString,false);
    assert.doesNotMatch(JSON.stringify(result),/123|user-name/);
  }
});
test('identity and username formats accept their exact string boundaries',()=>{
  for(const [id,username] of [['1','a'],['9'.repeat(30),'A'.repeat(30)]]){
    const result=read([tuple(id,username)]);
    assert.equal(result.reason,'candidate');assert.equal(result.scanComplete,true);assert.equal(result.shapeReason,'none');
  }
});
test('later malformed candidates clear identity flags from earlier valid candidates',()=>{
  const later=['PolarisViewer',['dependency'],{id:'later_private_id',username:'later_private_name'},1];
  const laterScript=read([tuple(),later]);
  const nested=tuple(123,'earlier_private_name');nested[2].nested=['PolarisViewer',[],{id:'nested_private_id'},1];
  const nestedTuple=read([nested]);
  for(const [result,shapeReason] of [[laterScript,'dependencies'],[nestedTuple,'missing_username']]){
    assert.equal(result.reason,'shape');assert.equal(result.shapeReason,shapeReason);
    assert.equal(result.modulePresent,true);assert.equal(result.scanComplete,false);
    assert.equal(result.idString,false);assert.equal(result.idSafeNumber,false);assert.equal(result.usernameString,false);
    assert.equal(result.qualified,false);assert.equal(result.serverFresh,false);
    assert.doesNotMatch(JSON.stringify(result),/private/i);
  }
});
const policyFor=changes=>({...structuredClone(INSTAGRAM_BOOTSTRAP_BUDGET),...changes});
test('irrelevant scripts consume inspection but not candidate parsing budgets',()=>{
  const result=read(['x'.repeat(300000),tuple()],{policy:policyFor({candidateBytes:{initial:128,max:128},parseBytes:{initial:128,max:128}})});
  assert.equal(result.reason,'candidate');assert.equal(result.scanComplete,true);
});
test('allowances grow within the same scan and never skip a later conflicting candidate',()=>{
  const result=read([tuple(),tuple('456')],{policy:policyFor({scripts:{initial:1,max:2},containers:{initial:3,max:6}})});
  assert.equal(result.reason,'conflict');assert.equal(result.scanComplete,true);assert.equal(result.budgetExpanded,true);
  const exhausted=read([tuple(),tuple('456'),{}],{policy:policyFor({scripts:{initial:1,max:2}})});
  assert.equal(exhausted.reason,'bounds_script_count');assert.equal(exhausted.scanComplete,false);
});
test('escaped names and nested candidates participate in exhaustive ambiguity checks',()=>{
  const escaped=JSON.stringify(tuple()).replace('PolarisViewer','\\u0050olarisViewer');
  assert.equal(read([escaped]).reason,'candidate');
  assert.equal(read([tuple(),escaped]).reason,'duplicate');
  const nested=tuple();nested[2].nested=tuple('456');
  const result=read([nested]);assert.equal(result.reason,'conflict');assert.equal(result.scanComplete,true);
  assert.equal(read([tuple(),tuple(),tuple('456')]).reason,'conflict');
  const duplicateKeys='{"a":'+JSON.stringify(tuple())+',"a":'+JSON.stringify(tuple('456'))+'}';
  assert.equal(read([duplicateKeys]).reason,'invalid_json');
});
test('candidate and traversal caps return incomplete instead of accepting a prefix',()=>{
  const bytes=Buffer.byteLength(JSON.stringify(tuple()));
  const cases=[
    [[tuple()],{candidateBytes:{initial:bytes-1,max:bytes-1}},'bounds_candidate_bytes'],
    [[tuple(),tuple()],{parseBytes:{initial:bytes,max:2*bytes-1}},'bounds_parse_bytes'],
    [[tuple()],{containers:{initial:1,max:1}},'bounds_nodes'],
    [[tuple()],{entries:{initial:1,max:1}},'bounds_entries'],
    [[tuple()],{depth:1},'bounds_depth'],
    [['x'.repeat(10)],{scanBytes:{initial:9,max:9}},'bounds_total_bytes'],
    [['é'.repeat(6)],{scriptBytes:{initial:10,max:10}},'bounds_script_bytes'],
  ];
  for(const [values,changes,reason]of cases){const result=read(values,{policy:policyFor(changes)});assert.equal(result.reason,reason);assert.equal(result.scanComplete,false);}
});
test('original cooperative deadline and clock regression cannot yield a complete result',()=>{
  let ticks=0;
  const result=read([tuple(),...Array(90).fill({})],{clock:()=>5*ticks++});
  assert.equal(result.reason,'scan_deadline');assert.equal(result.scanComplete,false);assert.equal(result.modulePresent,true);
  ticks=0;const clock=read([tuple()],{clock:()=>ticks++===0?100:99});assert.equal(clock.reason,'scan_clock');
});
test('trusted policy caps and complete-result invariants cannot be forged',()=>{
  assert.throws(()=>instagramSelfBootstrapExpression(policyFor({scanBytes:{initial:1,max:Infinity}})),/budget_rejected/);
  assert.throws(()=>instagramSelfBootstrapExpression(policyFor({deadlineMS:2000})),/budget_rejected/);
  const result=read([tuple()]);
  assert.throws(()=>validateInstagramSelfBootstrapProbe({...result,scanComplete:false}));
  assert.throws(()=>validateInstagramSelfBootstrapProbe({...result,reason:'bounds_nodes',scanComplete:true}));
});
test('bounds, malformed JSON, login and foreign page return closed outcomes',()=>{
  assert.equal(read(['PolarisViewer {']).reason,'invalid_json');
  assert.equal(read(['x'.repeat(262145)]).reason,'missing');
  assert.equal(read(Array(513).fill({})).reason,'bounds_script_count');
  assert.equal(read([tuple()],{login:true}).reason,'login_visible');
  assert.equal(read([tuple()],{path:'/another/'}).reason,'page_scope');
});
test('projection rejects private fields, fabricated qualification and getters',()=>{
  const result=read([tuple()]);
  const shape=read([[ 'PolarisViewer',[],{username:'self'},1 ]]);
  assert.equal(shape.shapeReason,'missing_id');
  const diagnostic={...shape,failureStage:'none',failureCondition:'none',cleanup:'verified',cleanupStage:'none',cleanupCondition:'none'};
  assert.equal(validateInstagramBootstrapDiagnostic(diagnostic).shapeReason,'missing_id');
  for(const forged of [{...result,accountId:'PRIVATE'}, {...result,qualified:true},{...result,serverFresh:true},{...result,reason:'PRIVATE'},
    {...shape,shapeReason:'PRIVATE'}, {...shape,shapeReason:'none'}, {...result,shapeReason:'missing_id'}, {...shape,modulePresent:false},
    {...shape,idString:true}, {...shape,scanComplete:true}, Object.fromEntries(Object.entries(shape).filter(([key])=>key!=='shapeReason'))])
    assert.throws(()=>validateInstagramSelfBootstrapProbe(forged));
  for(const forged of [{...diagnostic,shapeReason:'PRIVATE'}, {...diagnostic,reason:'missing'}, {...diagnostic,identity:'PRIVATE'}])
    assert.throws(()=>validateInstagramBootstrapDiagnostic(forged));
  const getter={...result};Object.defineProperty(getter,'reason',{get(){throw Error('getter executed');}});
  assert.throws(()=>validateInstagramSelfBootstrapProbe(getter),/probe_rejected/);
});
for(const role of ['reader','collector'])test(`${role}: existing owned page is read only and document replacement rejects`,async()=>{
  for(const changed of [false,true]){
    let frames=0;const calls=[];
    const page={send:async(method)=>{calls.push(method);return{frameTree:{frame:{id:'main',loaderId:++frames===2&&changed?'replacement':'document',url:'https://www.instagram.com/'}}};},
      evaluate:async()=>{calls.push('evaluate');return read([tuple()]);}};
    const result=await runInstagramSelfBootstrapProbe(page);
    assert.equal(result.reason,changed?'document_changed':'candidate');
    assert.equal(result.documentBound,!changed);assert.equal(result.qualified,false);
    assert.deepEqual(calls,['Page.getFrameTree','evaluate','Page.getFrameTree']);
  }
});
test('fresh projection time cannot qualify old or cached delivery metadata',()=>{
  const delivery={receivedAtMS:1000,sameDocument:true,mainFrame:true,sameOrigin:true,serverDelivered:true,cacheOrServiceWorker:false,selfSemanticsQualified:false};
  assert.equal(assessInstagramBootstrapDelivery(delivery,1001),'self_schema_unqualified');
  assert.equal(assessInstagramBootstrapDelivery({...delivery,selfSemanticsQualified:true},1001),'eligible_for_native_review');
  assert.equal(assessInstagramBootstrapDelivery({...delivery,receivedAtMS:1000},7001),'stale_or_missing_delivery');
  for(const [key,value] of [['sameDocument',false],['mainFrame',false],['sameOrigin',false],['serverDelivered',false],['cacheOrServiceWorker',true]])assert.equal(assessInstagramBootstrapDelivery({...delivery,[key]:value},1001),'delivery_unproven');
});
