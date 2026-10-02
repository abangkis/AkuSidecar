import test from 'node:test';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import {readFile} from 'node:fs/promises';

const boundary=await readFile(new URL('../vendor/facebook-boundary.js',import.meta.url),'utf8');
const extractor=await readFile(new URL('../vendor/facebook-extract.js',import.meta.url),'utf8');
const link=id=>`https://www.facebook.com/fixture/posts/${id}/`;
const post=(id,text='Full post',author='Fixture',media=[])=>({id,text,author,media,
  getBoundingClientRect:()=>({width:600,height:400,top:100,bottom:500}),
  matches:()=>false,querySelectorAll:()=>[],querySelector:()=>null,getAttribute:()=>null,hasAttribute:()=>false});
const dialog=(members=[],style={display:'block',visibility:'visible'})=>({style,
  getBoundingClientRect:()=>({width:800,height:700,top:50,bottom:750}),contains:node=>members.includes(node)});
test('comment and reply media remain separate while unlabeled media wrappers retain ownership',()=>{
  const context=vm.createContext({});vm.runInContext(boundary,context);
  const root={};
  for(const label of ['Comment by Fixture','Reply by Fixture','Komentar oleh Fixture','Balasan oleh Fixture','']){
    const wrapper={matches:s=>s==='[role="article"]',getAttribute:()=>label,
      querySelectorAll:()=>[],querySelector:()=>({}),parentElement:{closest:()=>root}};
    const image={closest:()=>wrapper};
    assert.equal(context.FacebookHeadlessBoundary.owns(image,root),label==='');
  }
});
function fixture(posts,dialogs=[],url=link('123')) {
  const document={querySelectorAll:selector=>selector==='[role="dialog"]'?dialogs:[],querySelector:()=>null,
    visibilityState:'visible',readyState:'complete',title:'Fixture',scrollingElement:{scrollHeight:2000}};
  const adapter={version:'fixture',discoverCandidates:()=>({candidates:posts}),
    platformIdFromCandidates:values=>{const id=values[0]?.match(/\/posts\/(\d+)/)?.[1];return id?`facebook:post:${id}`:null;},
    findPermalinkDetails:p=>p.id?{url:link(p.id)}:null,findAuthor:p=>p.author,extractText:p=>p.text,
    findAvatar:()=>null,contentExpansion:{},extractPresentation:()=>({}),extractSemantics:()=>({}),
    mediaAcquisition:{extractCandidates:p=>p.media,detectExpectedKinds:()=>[]},loginRequired:()=>false,availability:()=>null};
  const context=vm.createContext({URL,document,location:new URL(url),innerWidth:1280,innerHeight:900,scrollY:0,
    getComputedStyle:node=>node.style,AkuSourceAdapters:{get:()=>adapter},setTimeout});
  vm.runInContext(boundary,context);vm.runInContext(extractor,context);
  return {collect:async()=>JSON.parse(JSON.stringify(await context.XHeadlessPoC.collect())),prepare:()=>context.XHeadlessPoC.prepareEvidenceTargets(),
    scope:()=>context.FacebookHeadlessBoundary.captureScope(posts,adapter,{})};
}

test('native dialog excludes the collapsed feed copy and preserves image/video evidence',async()=>{
  for(const media of [[{kind:'image',url:'https://media.example/one.jpg'},{kind:'image',url:'https://media.example/two.jpg'}],
    [{kind:'video_poster',url:'https://media.example/poster.jpg'}],[]]) {
    const feed=post('123','Short…'),full=post('123','Full post','Fixture',media),other=post('456');
    const result=await fixture([feed,full,other],[dialog([full,other])]).collect();
    assert.equal(result.dialogScope.status,'native_post_dialog');
    assert.equal(result.posts.length,1);assert.equal(result.posts[0].text,'Full post');
    assert.deepEqual(result.posts[0].media,media);
  }
});
test('conflicting author or text inside the selected dialog remains rejected',async()=>{
  for(const second of [post('123','Different text'),post('123','Full post','Other author')]){
    const first=post('123');const result=await fixture([first,second],[dialog([first,second])]).collect();
    assert.equal(result.posts.length,0);assert.equal(result.rejectionReasons.conflicting_post_binding,2);
  }
});
test('an unhydrated or wrong-ID dialog cannot fall back to a matching feed post',async()=>{
  const feed=post('123'),pending=post(null),surface=dialog([pending]);
  const f=fixture([feed,pending],[surface]);
  assert.equal((await f.collect()).dialogScope.status,'dialog_pending_identity');
  assert.equal((await f.collect()).posts.length,0);assert.equal(f.scope().hoverCandidates.length,1);
  pending.id='456';assert.equal((await f.collect()).posts.length,0);
  pending.id='123';assert.equal((await f.collect()).posts.length,1);
});
test('separate visible dialogs are ambiguous even when only one matches',async()=>{
  const first=post('123'),second=post('456');const f=fixture([first,second],[dialog([first]),dialog([second])]);
  const result=await f.collect();assert.equal(result.dialogScope.status,'ambiguous_dialog');assert.equal(result.posts.length,0);
  assert.equal(f.scope().hoverCandidates.length,0);
});
test('pending dialog hovers a lazy link without href without hovering the feed underlay',()=>{
  const feed=post('123'),pending=post(null);
  const anchor={tagName:'A',target:'',closest:selector=>selector==='div[aria-posinset], [role="article"]'?pending:null,
    getAttribute:()=>null,hasAttribute:()=>false,querySelector:()=>null,
    getBoundingClientRect:()=>({width:70,height:20,left:100,top:100,bottom:120}),innerText:'2h'};
  pending.querySelectorAll=selector=>selector==='a,[role="link"], abbr[data-utime], time'?[anchor]:[];
  pending.matches=()=>true;
  const targets=fixture([feed,pending],[dialog([pending])]).prepare();
  assert.equal(targets.length,1);assert.equal(targets[0].x,135);assert.equal(targets[0].y,110);
});
test('nested wrappers select the innermost dialog and hidden dialogs do not interfere',async()=>{
  const p=post('123'),inner=dialog([p]),outer=dialog([inner,p]),hidden=dialog([],{display:'none',visibility:'visible'});
  const result=await fixture([p],[outer,inner,hidden]).collect();
  assert.equal(result.dialogScope.status,'native_post_dialog');assert.equal(result.posts.length,1);
});
test('no-dialog collection and conflict rejection retain existing behavior',async()=>{
  const p=post('123');assert.equal((await fixture([p]).collect()).posts.length,1);
  const result=await fixture([p,post('123','Conflicting text')]).collect();
  assert.equal(result.dialogScope.status,'page');assert.equal(result.posts.length,0);
  assert.equal(result.rejectionReasons.conflicting_post_binding,2);
});
test('a feed route does not infer native-post dialog ownership',async()=>{
  const p=post('123');const result=await fixture([p],[dialog([])],'https://www.facebook.com/').collect();
  assert.equal(result.dialogScope.status,'page');assert.equal(result.posts.length,1);
});
