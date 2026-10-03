import test from 'node:test';
import assert from 'node:assert/strict';
import {capture} from '../capture.mjs';
const photoUrl='https://www.facebook.com/photo?fbid=123';
const url='https://www.facebook.com/fixture/posts/pfbidABC';
const media={kind:'image',url:'https://media.fbcdn.net/image.jpg',loaded:true};
const parent={url,nativeId:'facebook:post:pfbidABC',numericPostId:'789',author:'Fixture',text:'Body'};
const evidence={status:'verified_photo_media',identityKind:'photo',photoId:'123',media,parent};
const assets={facebook:['runtime','adapter','extractor'].map((relative,index)=>({relative,execute:false,sha256:String(index).repeat(64)}))};
async function run(patch={},backend,redirect=false){
  const navigations=[];let current;
  const page={async navigate(u){navigations.push(u);current=u;return {};},async send(){return {};},async evaluate(e){
    if(e==='location.href')return redirect&&current===url?'https://www.facebook.com/':current;
    if(e.includes('prepareEvidenceTargets'))return [];
    if(e.includes('XHeadlessPoC.collect()'))return JSON.stringify({documentReady:true,scroll:{y:0,viewportHeight:900},
      photoEvidence:evidence,posts:current===photoUrl?[]:[{id:parent.nativeId,permalink:url,author:'Fixture',text:'Body',media:[media],...patch}]});
  }};
  const result=await capture({backend,forSource:async()=>page},assets,'facebook',{
    pageUrl:photoUrl,scrolls:0,sourceHydrationTimeoutMs:1000,captureTimeoutMs:3500});
  return {result,navigations};
}
test('photo follows one explicit parent and returns parent identity with separate provenance',async()=>{
  const {result,navigations}=await run();assert.deepEqual(navigations,[photoUrl,url]);
  assert.equal(result.snapshots[0].blocks[0].platformId,parent.nativeId);
  assert.deepEqual(result.coverage.photoParentResolution,{status:'verified',photoId:'123',parentPlatformId:parent.nativeId,
    provenance:'structured_photo_parent_and_matching_native_post'});
});
test('wrong author, text, media, identity or redirect cannot pass parent admission',async()=>{
  for(const [patch,field] of [[{author:'Other'},'authorMatchCount'],[{text:'Other'},'textMatchCount'],
    [{media:[]},'imagePathMatchCount'],[{id:'facebook:post:999'},'nativeIdentityCandidateCount']])
    await assert.rejects(run(patch),e=>{
      assert.equal(e.code,'photo_parent_unverified');
      assert.equal(e.diagnostics.stage,'photo_parent_corroboration');
      assert.equal(e.diagnostics.routeMatches,true);
      assert.equal(e.diagnostics[field],0);
      assert.equal(e.diagnostics.corroboratedCandidateCount,0);
      // Diagnostics expose only a stage, a boolean and counts, never social
      // content, IDs, URLs or media tokens.
      assert.ok(Object.entries(e.diagnostics).every(([key,value])=>
        key==='stage' ? value==='photo_parent_corroboration' : typeof value==='boolean'||Number.isSafeInteger(value)));
      return true;
    });
  await assert.rejects(run({},undefined,true),e=>{
    assert.equal(e.code,'photo_parent_unverified');
    assert.equal(e.diagnostics.routeMatches,false);
    assert.equal(e.diagnostics.corroboratedCandidateCount,1);
    return true;
  });
});
test('borrowed Quiet capture does not acquire photo navigation behavior',async()=>{
  await assert.rejects(run({},'browser_quiet_hidden'),e=>e.code==='empty_unverified');
});
