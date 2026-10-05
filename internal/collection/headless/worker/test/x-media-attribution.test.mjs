import test from 'node:test';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import {readFile} from 'node:fs/promises';

const code=await readFile(new URL('../vendor/x-extract.js',import.meta.url),'utf8');
const shared=await readFile(new URL('../../../../../../AkuBridge/capture-primitives.js',import.meta.url),'utf8');
test('video-root avatar is excluded while native poster and photo are retained',async()=>{
 const image=(url,inVideo=true)=>({tagName:'IMG',currentSrc:url,alt:'',complete:true,naturalWidth:48,naturalHeight:48,closest:()=>inVideo?{}:null});
 const avatar=image('https://pbs.twimg.com/profile_images/123/avatar_normal.jpg');
 const poster=image('https://pbs.twimg.com/amplify_video_thumb/456/img/poster.jpg');
 const photo=image('https://pbs.twimg.com/media/real-photo.jpg',false);
 const time={dateTime:'2026-10-03T00:00:00Z',closest:()=>({href:'https://x.com/owner/status/456'})};
 const container={getBoundingClientRect:()=>({height:200,top:0,bottom:200}),querySelector:()=>null,
  querySelectorAll:selector=>selector==='time'?[time]:selector==='img'?[avatar,poster,photo]:[]};
 const adapter={version:'fixture',discoverCandidates:()=>({candidates:[container],strategy:'fixture'}),
  findQuotedRoot:()=>null,findAuthor:()=> 'Owner',findAvatar:()=>avatar.currentSrc,
  contentRootSelector:'text',extractText:()=> 'Video caption',extractSemantics:()=>({}),extractQuotedPost:()=>null,
  imageSelector:'img',mediaHosts:['pbs.twimg.com'],mediaRendering:{videoRootSelector:'video-root'},
  mediaAcquisition:{detectExpectedKinds:()=>['video','image']},contentExpansion:{buttonSelector:'button'},loginRequired:()=>false};
 const context=vm.createContext({URL,location:{href:'https://x.com/owner/status/456',hostname:'x.com',pathname:'/owner/status/456'},
  AkuHeadlessCapturePolicy:{allowContentExpansion:false},AkuSourceAdapters:{get:()=>adapter},
  innerHeight:900,innerWidth:1200,scrollY:0,document:{querySelector:()=>null,body:{innerText:''},readyState:'complete',visibilityState:'visible'}});
 vm.runInContext(shared,context);
 vm.runInContext(code,context);
 const result=await context.XHeadlessPoC.collect();
 assert.equal(result.posts.length,1);
 assert.equal(result.posts[0].avatar,avatar.currentSrc);
 assert.deepEqual(JSON.parse(JSON.stringify(result.posts[0].media.map(m=>({kind:m.kind,url:m.url})))),[
  {kind:'video_poster',url:poster.currentSrc},{kind:'image',url:photo.currentSrc}]);
});
