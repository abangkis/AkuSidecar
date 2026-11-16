import {resolve} from 'node:path';
import {fileURLToPath} from 'node:url';
import {runDOMShadowParity} from './shared-dom-shadow-fixture.mjs';

const article=(body='<span dir="auto">Private primary authored fixture caption.</span>',links='<a href="/p/Fixture123/">Post</a>',author=true)=>
  `<article>${author?'<a href="/fixture_author/">Private Author</a>':''}${links}${body}</article>`;
const post=article();
const cases=[
  {name:'article',body:post,code:'ready'},
  {name:'permalink ancestor',body:post.replaceAll('article','div'),code:'ready'},
  {name:'missing permalink',body:article(undefined,''),code:'no_valid_candidates'},
  {name:'offscreen',body:post.replace('<article>','<article style="margin-top:2000px">'),code:'offscreen'},
  {name:'media only',body:article('<img width="320" height="180" src="https://s.cdninstagram.com/fixture.jpg">'),code:'ready'},
  {name:'video poster',body:article('<video width="320" height="180" poster="https://s.cdninstagram.com/poster.jpg"></video>'),code:'ready'},
  {name:'missing author',body:article(undefined,undefined,false),code:'no_valid_candidates'},
  {name:'missing content',body:article(''),code:'no_valid_candidates'},
  {name:'conflicting identity',rejections:{identity_conflict:1},body:article(undefined,'<a href="/p/Fixture123/">Post</a><a href="/p/Other123/">Other post</a>'),code:'no_valid_candidates'},
  {name:'nested article cannot identify parent',rejections:{missing_identity:1},body:article('<span dir="auto">Primary caption without its own link.</span>'+article(),''),code:'ready'},
  {name:'comment identity isolated',body:article('<span dir="auto">Primary caption.</span><div data-comment-id="c"><a href="/p/Other123/">Comment link</a><span dir="auto">Long private comment must not supply primary content or identity.</span></div>'),code:'ready'},
  {name:'comment cannot identify parent',rejections:{missing_identity:1},body:article('<span dir="auto">Primary caption.</span><div data-comment-id="c"><a href="/p/Other123/">Comment link</a></div>',''),code:'no_valid_candidates'},
  {name:'loading placeholder',body:'<div aria-busy="true">Loading</div>'+article(undefined,''),code:'hydrating'},
  {name:'loading valid post',body:'<div aria-busy="true">Loading</div>'+post,code:'ready'},
  {name:'empty unverified',body:'You are all caught up',code:'empty_unverified'},
  {name:'login',body:'<input name="username">',code:'login_required'},
  {name:'login with post and loader',body:'<input name="username"><div aria-busy="true">Loading</div>'+post,code:'login_required'},
  {name:'challenge route',route:'https://www.instagram.com/challenge/',body:post,code:'challenge_required'},
  {name:'challenge DOM',body:'<section id="challenge-running">Security check</section>',code:'challenge_required'},
  {name:'anchor bound',body:article(undefined,Array.from({length:101},(_,i)=>`<a href="/p/Fixture123/?q=${i}">Post</a>`).join('')),code:'no_valid_candidates'},
  {name:'claim bound',body:article(undefined,Array.from({length:9},(_,i)=>`<a href="/p/Fixture${i}/">Post</a>`).join('')),code:'no_valid_candidates'},
  {name:'candidate bound',counts:{candidates:64},body:Array.from({length:65},()=>post).join(''),code:'ready'},
  {name:'failing probe',body:post,failProbe:true},
];

export async function runInstagramShadowParity(chrome,output) {
  return runDOMShadowParity({chrome,output,source:'instagram',canonicalUrl:'https://www.instagram.com/',probeGlobal:'AkuInstagramDOMShadow',cases,privacyPattern:'Private|Fixture123|cdninstagram|instagram:|fixture_author',style:'article{width:600px;min-height:100px}span{display:block}'});
}
if(process.argv[1]&&resolve(process.argv[1])===fileURLToPath(import.meta.url)){
  const report=await runInstagramShadowParity(process.argv[2]??'C:/Program Files/Google/Chrome/Application/chrome.exe',process.argv[3]);
  console.log(JSON.stringify({fixtures:report.fixtures,engineSha256:report.engineSha256,effects:report.effects}));
}
