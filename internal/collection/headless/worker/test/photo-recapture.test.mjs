import test from 'node:test';
import assert from 'node:assert/strict';
import {photoRecaptureObservation} from '../photo-recapture.mjs';
import {capture} from '../capture.mjs';
const url='https://www.facebook.com/photo.php?fbid=123';
const fixture=()=>({status:'verified_photo_media',identityKind:'photo',photoId:'123',ownerId:'456',
  provenance:'exact_photo_metadata_and_visible_image',media:{kind:'image',url:'https://media.fbcdn.net/photo.jpg',loaded:true,width:1200,height:800},
  parent:{text:'Unrelated album caption',author:'Album author'}});
test('photo recovery returns one media evidence block without inventing caption or parent identity',()=>{
  const o=photoRecaptureObservation(fixture(),url,url,'now',{}), b=o.snapshots[0].blocks[0];
  assert.equal(b.platformId,'facebook:photo:123');assert.equal(b.text,'');assert.equal(b.author,'');assert.equal(b.media.length,1);
  assert.equal(o.coverage.photoParentResolution,undefined);
});
test('rejects wrong photo, redirects, unloaded media and untrusted image URLs',()=>{
  for(const change of [e=>e.photoId='999',e=>e.ownerId='',e=>e.media.loaded=false,e=>e.media.width=0,
    e=>e.media.url='https://fbcdn.net.evil.example/photo',e=>e.provenance='guessed']){
    const e=fixture();change(e);assert.equal(photoRecaptureObservation(e,url,url,'now',{}),null);
  }
  for(const actual of ['https://www.facebook.com/photo.php?fbid=999',url+'&fbid=999','https://www.facebook.com/'])
    assert.equal(photoRecaptureObservation(fixture(),url,actual,'now',{}),null);
});
test('actual recapture stops on verified photo without navigating to album',async()=>{
  const navigations=[];
  const page={navigate:async u=>{navigations.push(u);return {};},evaluate:async e=>{
    if(e==='location.href')return url;
    if(e.includes('XHeadlessPoC.collect()'))return JSON.stringify({posts:[],documentReady:true,photoEvidence:fixture()});
  }};
  const assets={facebook:['a','b','c'].map(relative=>({relative,execute:false,sha256:'0'.repeat(64)}))};
  const o=await capture({forSource:async()=>page},assets,'facebook',{mode:'recapture_media',reason:'missing_media',pageUrl:url,scrolls:0,sourceHydrationTimeoutMs:1000});
  assert.deepEqual(navigations,[url]);assert.equal(o.coverage.photoMediaRecapture.photoId,'123');
  for(const payload of [{},{mode:'recapture_media',reason:'playback_error'}]) {
    await assert.rejects(capture({forSource:async()=>page},assets,'facebook',{
      ...payload,pageUrl:url,scrolls:0,sourceHydrationTimeoutMs:1000}),e=>e.code==='empty_unverified');
  }
});
