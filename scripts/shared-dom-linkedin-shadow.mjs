import {resolve} from 'node:path';
import {fileURLToPath} from 'node:url';
import {runDOMShadowParity} from './shared-dom-shadow-fixture.mjs';

const primary='1234567890123456789',other='9876543210987654321';
const text='<div data-testid="expandable-text-box">Private primary authored fixture text with enough source detail to exercise the existing LinkedIn capture path.</div>';
const link=id=>`<a href="/feed/update/urn:li:activity:${id}/">Post</a>`;
const card=({body=text,links=link(primary),id=primary,author=true,tag='article',attributes=''}={})=>
  `<${tag} class="feed-shared-update-v2" ${id?`data-urn="urn:li:activity:${id}"`:''} ${attributes}>${author?'<span data-view-name="feed-actor-name">Private Author</span>':''}${links}${body}</${tag}>`;
const post=card();
const blockedPost=card({id:null,links:'',body:text+'<button id="post-control" aria-label="Open control menu for post by Private Author">Menu</button>'});
const cases=[
  {name:'native article',body:post,code:'ready'},
  {name:'native wrapper',body:card({tag:'div'}),code:'ready'},
  {name:'root URN without anchor',body:card({links:''}),code:'ready'},
  {name:'ugcPost root',body:card({id:null,links:'',attributes:`data-id="urn:li:ugcPost:${primary}"`}),code:'ready'},
  {name:'share root',body:card({id:null,links:'',attributes:`data-id="urn:li:share:${primary}"`}),code:'ready'},
  {name:'public activity anchor',body:card({id:null,links:`<a href="/posts/fixture-author_activity-${primary}-abc">Post</a>`}),code:'ready'},
  {name:'action ancestor',body:`<section><span data-view-name="feed-actor-name">Private Author</span>${link(primary)}${text}<button>Like</button><button>Comment</button></section>`,code:'ready'},
  {name:'missing native identity',body:card({id:null,links:''}),code:'no_valid_candidates',rejections:{missing_identity:1}},
  {name:'existing menu blocks recovery',body:blockedPost+'<div role="menu" style="width:120px;height:50px">Private menu</div>',code:'no_valid_candidates',recoveryBlocker:'role_menu'},
  {name:'existing dialog blocks recovery',body:blockedPost+'<div role="dialog" style="width:120px;height:50px">Private dialog</div>',code:'no_valid_candidates',recoveryBlocker:'role_dialog'},
  {name:'aria-hidden dialog remains a recovery blocker',body:blockedPost+'<div aria-hidden="true"><div role="dialog" style="width:120px;height:50px">Private hidden dialog</div></div>',code:'no_valid_candidates',recoveryBlocker:'role_dialog',ancestorFlag:'ariaHidden'},
  {name:'hidden dialog does not block recovery',body:blockedPost+'<div role="dialog" style="display:none">Private hidden dialog</div>',code:'no_valid_candidates',recoveryBlocker:null,recoveryAttempt:true},
  {name:'conflicting ARIA references stay diagnostic',body:blockedPost.replace('id="post-control"','id="post-control" aria-controls="other-surface"')+'<div role="dialog" id="actual-surface" aria-labelledby="post-control" style="width:120px;height:50px">Private dialog</div>',code:'no_valid_candidates',recoveryBlocker:'role_dialog',aria:{labelledBy:'matches',controls:'different',owns:'absent'}},
  {name:'inert ancestor remains a recovery blocker',body:blockedPost+'<div inert><div role="dialog" style="width:120px;height:50px">Private inert dialog</div></div>',code:'no_valid_candidates',recoveryBlocker:'role_dialog',ancestorFlag:'inert'},
  {name:'transparent ancestor records the existing visibility difference',body:blockedPost+'<div style="opacity:0"><div role="dialog" style="width:120px;height:50px">Private transparent dialog</div></div>',code:'no_valid_candidates',recoveryBlocker:'role_dialog',ancestorFlag:'opacityZero',recoveryDifference:'Windowed checks the surface style only; Headless also checks ancestor styles. The transparent ancestor therefore blocks only Windowed. This diagnostic increment preserves both operational predicates.',headlessRecoveryAttempt:true},
  {name:'foreign native URL',body:card({id:null,links:`<a href="https://foreign.test/posts/author_activity-${primary}-x">Post</a>`}),code:'no_valid_candidates'},
  {name:'profile URL is not a post',body:card({id:null,links:`<a href="/in/author-activity-${primary}/">Profile</a>`}),code:'no_valid_candidates'},
  {name:'conflicting root and anchor',body:card({links:link(other)}),code:'no_valid_candidates',rejections:{identity_conflict:1}},
  {name:'offscreen',body:card({attributes:'style="margin-top:2000px"'}),code:'offscreen'},
  {name:'owned image only',body:card({body:'<div class="update-components-image"><img width="320" height="180" src="https://media.licdn.com/fixture.jpg"></div>'}),code:'ready'},
  {name:'owned video poster only',body:card({body:'<video width="320" height="180" poster="https://media.licdn.com/poster.jpg"></video>'}),code:'ready'},
  {name:'foreign image is not media proof',body:card({body:'<div class="update-components-image"><img width="320" height="180" src="https://foreign.test/image.jpg"></div>'}),code:'no_valid_candidates'},
  {name:'document shell without owned preview',body:card({body:'<iframe title="Document" src="https://www.linkedin.com/embeds/document"></iframe>'}),code:'no_valid_candidates'},
  {name:'undeclared text root stays unverified',body:card({body:'Private text outside the declared authored-text root.'}),code:'no_valid_candidates'},
  {name:'video placeholder without evidence',body:card({body:'<div data-view-name="feed-video"></div>'}),code:'no_valid_candidates',rejections:{missing_content:1}},
  {name:'missing author',body:card({author:false}),code:'no_valid_candidates'},
  {name:'header and permalink are not content',body:card({body:''}),code:'no_valid_candidates'},
  {name:'nested article cannot identify parent',body:card({id:null,links:'',body:text+post}),code:'ready',rejections:{missing_identity:1}},
  {name:'comment cannot identify parent',body:card({id:null,links:'',body:text+`<div data-comment-id="comment">${link(other)}</div>`}),code:'no_valid_candidates'},
  {name:'comment data-id cannot become a primary claim',body:card({id:null,links:'',body:text+`<div data-id="urn:li:comment:(urn:li:activity:${other},123)">${link(other)}</div>`}),code:'no_valid_candidates'},
  {name:'comment cannot supply author',body:card({author:false,body:text+'<div data-comment-id="comment"><button aria-label="Open control menu for post by Comment Author"></button></div>'}),code:'no_valid_candidates'},
  {name:'repost cannot supply text',body:card({body:`<div data-testid="expandable-text-box"><div class="update-components-mini-update-v2">Private quoted content</div></div>`}),code:'no_valid_candidates'},
  {name:'repost cannot supply media',body:card({body:'<div class="update-components-mini-update-v2"><div class="update-components-image"><img width="320" height="180" src="https://media.licdn.com/quoted.jpg"></div></div>'}),code:'no_valid_candidates'},
  {name:'loading placeholder',body:'<div aria-busy="true">Loading</div>'+card({id:null,links:''}),code:'hydrating'},
  {name:'loading valid post',body:'<div aria-busy="true">Loading</div>'+post,code:'ready'},
  {name:'empty remains unverified',body:'You are all caught up',code:'empty_unverified'},
  {name:'login with post and loader',body:'<input name="session_key"><div aria-busy="true">Loading</div>'+post,code:'login_required'},
  {name:'challenge route',route:'https://www.linkedin.com/checkpoint/challenge/',body:post,code:'challenge_required'},
  {name:'challenge DOM',body:'<section id="challenge-running">Security check</section>',code:'challenge_required'},
  {name:'wrong host',route:'https://foreign.test/',body:post,code:'wrong_route',headlessCode:null,headlessProbe:false,
    difference:'Headless rejects an unsupported host before adapter selection; Windowed receives an explicit source and emits wrong_route. Both retain their existing rejection.'},
  {name:'node bound',body:card({links:Array.from({length:101},()=>link(primary)).join('')}),code:'no_valid_candidates'},
  {name:'claim bound',body:card({links:Array.from({length:9},(_,n)=>link('12345678901234567'+String(n).padStart(2,'0'))).join('')}),code:'no_valid_candidates'},
  {name:'text traversal bound',body:card({body:'<div data-testid="expandable-text-box">'+Array.from({length:101},()=>'<span>Private text</span>').join('')+'</div>'}),code:'no_valid_candidates'},
  {name:'candidate bound',body:Array.from({length:65},()=>post).join(''),code:'ready',counts:{candidates:64}},
  {name:'failing probe',body:post,failProbe:true},
];

export async function runLinkedInShadowParity(chrome,output) {
  return runDOMShadowParity({chrome,output,source:'linkedin',canonicalUrl:'https://www.linkedin.com/feed/',probeGlobal:'AkuLinkedInDOMShadow',cases,
    privacyPattern:'Private|1234567890123456789|9876543210987654321|licdn|linkedin:|fixture-author',
    style:'article,.feed-shared-update-v2,section{width:600px;min-height:100px}span{display:block}'});
}
if(process.argv[1]&&resolve(process.argv[1])===fileURLToPath(import.meta.url)){
  const report=await runLinkedInShadowParity(process.argv[2]??'C:/Program Files/Google/Chrome/Application/chrome.exe',process.argv[3]);
  console.log(JSON.stringify({fixtures:report.fixtures,engineSha256:report.engineSha256,effects:report.effects}));
}
