import test from 'node:test';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import {readFile} from 'node:fs/promises';
import {emptyCaptureDiagnostics} from '../capture.mjs';
const code=await readFile(new URL('../vendor/facebook-photo-evidence.js',import.meta.url),'utf8');
const uri='https://scontent.example.fbcdn.net/photo.jpg?token=fixture';
const photo=()=>({__typename:'Photo',id:'123',owner:{id:'456'},image:{uri}});
function run({records=[photo()],url='https://www.facebook.com/photo?fbid=123',src=uri,loaded=true,style={},rect={},text}={}) {
  const image={currentSrc:src,complete:loaded,naturalWidth:2048,naturalHeight:1465,
    getBoundingClientRect:()=>({width:700,height:500,top:0,bottom:500,left:0,right:700,...rect})};
  const context=vm.createContext({URL,location:new URL(url),innerHeight:900,innerWidth:1280,
    getComputedStyle:()=>({display:'block',visibility:'visible',opacity:'1',...style}),
    document:{querySelectorAll:s=>s==='img'?[image]:[{textContent:text??JSON.stringify(records)}]}});
  vm.runInContext(code,context);
  return JSON.parse(JSON.stringify(context.FacebookHeadlessPhotoEvidence.collect()));
}
test('exact photo metadata plus loaded image admits photo evidence, never a post',()=>{
  const result=run({src:uri.replace('fixture','renewed')});
  assert.equal(result.status,'verified_photo_media');assert.equal(result.photoId,'123');
  assert.equal(result.ownerId,'456');assert.equal(result.identityKind,'photo');
  assert.equal(result.postBinding,'unverified');assert.equal(result.media.width,2048);
  assert.equal(result.author,undefined);assert.equal(result.text,undefined);assert.equal(result.platformId,undefined);
});
test('feed, post, external and conflicting photo routes are not photo evidence',()=>{
  for(const url of ['https://www.facebook.com/','https://www.facebook.com/a/posts/123',
    'https://evil.example/photo?fbid=123','https://www.facebook.com/photo?fbid=123&fbid=789',
    'https://www.facebook.com/photo?fbid=123&photo_id=789']) assert.equal(run({url}).status,'not_photo_route');
});
test('matching image alone, wrong photo ID or wrong entity type cannot bind ownership',()=>{
  for(const records of [[],[{...photo(),id:'789'}],[{...photo(),__typename:'Video'}]])
    assert.equal(run({records}).status,'photo_metadata_missing');
  assert.equal(run({records:[{...photo(),owner:null}]}).status,'photo_owner_missing');
});
test('conflicting owners or images fail closed across matching fragments',()=>{
  assert.equal(run({records:[photo(),{id:'123',owner:{id:'999'}}]}).status,'conflicting_photo_binding');
  assert.equal(run({records:[photo(),{...photo(),image:{uri:uri.replace('photo.jpg','other.jpg')}}]}).status,'conflicting_photo_binding');
});
test('offscreen, hidden, unloaded and unrelated images do not pass',()=>{
  for(const input of [{loaded:false},{src:uri.replace('photo.jpg','other.jpg')},{style:{visibility:'hidden'}},
    {style:{opacity:'0'}},{rect:{left:1500,right:2200}},{rect:{top:1000,bottom:1500}}])
    assert.equal(run(input).status,'photo_image_not_visible');
});
test('untrusted image hosts and metadata budgets fail closed',()=>{
  assert.equal(run({records:[{...photo(),image:{uri:'https://fbcdn.net.evil.example/photo.jpg'}}]}).status,'photo_metadata_missing');
  assert.equal(run({text:' '.repeat(8*1024*1024+1)}).status,'evidence_limit');
});
test('empty capture reports photo status without leaking identity, owner or media URL',()=>{
  const result=emptyCaptureDiagnostics([{photoEvidence:run()}]);
  assert.deepEqual(result.samples[0].photoEvidence,{status:'verified_photo_media',postBinding:'unverified'});
  const serialized=JSON.stringify(result);
  assert.equal(serialized.includes('fbcdn'),false);assert.equal(serialized.includes('ownerId'),false);
  assert.equal(emptyCaptureDiagnostics([{photoEvidence:{status:'untrusted page text'}}]).samples[0].photoEvidence.status,null);
});
test('parent resolution requires consistent explicit story URL, actor and body',()=>{
  const story={id:'opaque-story',post_id:'789',url:'https://www.facebook.com/fixture/posts/pfbidABC',
    actors:[{id:'456',name:'Fixture'}],message:{text:'Body'}};
  const record={...photo(),container_story:story,creation_story:{...story}};
  const parent=run({records:[record]}).parent;
  assert.equal(parent.nativeId,'facebook:post:pfbidABC');assert.equal(parent.numericPostId,'789');
  for(const patch of [{post_id:'999'},{url:'https://evil.example/fixture/posts/789'},
    {actors:[{id:'999',name:'Other'}]},{message:{text:'Other body'}},{id:'other-story'}]){
    assert.equal(run({records:[{...record,creation_story:{...story,...patch}}]}).parent,null);
  }
});
