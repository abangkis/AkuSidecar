import {resolve} from 'node:path';
import {fileURLToPath} from 'node:url';
import {runDOMShadowParity} from './shared-dom-shadow-fixture.mjs';

const primary='1234567890123456789',other='9876543210987654321';
const text='<div data-testid="expandable-text-box">Private primary authored fixture text with enough source detail to exercise the existing LinkedIn capture path.</div>';
const link=id=>`<a href="/feed/update/urn:li:activity:${id}/">Post</a>`;
const ownIdentity={id:`linkedin:activity:${primary}`,url:`https://www.linkedin.com/feed/update/urn:li:activity:${primary}/`};
const embeddedRepost=`<div class="update-components-mini-update-v2">${link(other)}</div>`;
const card=({body=text,links=link(primary),id=primary,author=true,tag='article',attributes=''}={})=>
  `<${tag} class="feed-shared-update-v2" ${id?`data-urn="urn:li:activity:${id}"`:''} ${attributes}>${author?'<span data-view-name="feed-actor-name">Private Author</span>':''}${links}${body}</${tag}>`;
const post=card();
const blockedPost=card({id:null,links:'',body:text+'<button id="post-control" aria-label="Open control menu for post by Private Author">Menu</button>'});

// The live activity card has no native ID. Its unlabelled popover opens in a
// document-level portal, aligned to the control that toggles aria-expanded.
const wulfieShare='7528814955425382400';
const wulfieIdentity={id:`linkedin:share:${wulfieShare}`,url:`https://www.linkedin.com/feed/update/urn:li:share:${wulfieShare}/`};
const yosShare='7528960160740859904';
const shikharShare='7529025358638067714';
const ivoShare='7524891208469463040';
const scrollLayout=(top,post)=>`<div style="height:${top}px"></div>${post}<div style="height:950px"></div>`;
const shareIdentity=id=>({id:`linkedin:share:${id}`,url:`https://www.linkedin.com/feed/update/urn:li:share:${id}/`,
  sources:{windowed:'embed_urn',headless:'adapter_recovery'}});
const anchorGeometry=(atClick,firstGap,lastGap=firstGap,lastScroll=atClick,correctedGapY)=>[{
  snapshotIndex:0,candidateIndex:0,scrollTopAtClick:atClick,
  first:{scrollTop:atClick,gapY:firstGap,...(correctedGapY===undefined?{}:{correctedGapY})},
  last:{scrollTop:lastScroll,gapY:lastGap,...(correctedGapY===undefined?{}:{correctedGapY})},
}];

const adPromoUrl='https://www.linkedin.com/campaignmanager/claim-promo/?companyId=123&incentiveId=456';
const premiumPromoUrl='https://www.linkedin.com/premium/products/?upsellSlotId=fixture';
const sidebarMenu=(url,label,attributes='',additionalAction='')=>`<div role="menu" ${attributes} style="min-height:24px">
  <div data-display-contents="true"><a href="${url}" tabindex="0" ${url===adPromoUrl?'role="menuitem"':''}>${label}</a></div>${additionalAction}
</div>`;
const sidebar=contents=>`<aside aria-label="Sidebar">${contents}</aside>`;
const promoMenus=sidebarMenu(adPromoUrl,'Claim ad credits')+sidebarMenu(premiumPromoUrl,'Start Premium Page');

function wulfieCard({postId=wulfieShare,menuOwner='',expanded=true,offset=0,competingTrigger=false,existingMenus=false,keepOpen=false,
  author='Wulfie Bain',ownerAction='',templateID='fixture-menu',sameAuthorControl=false,embedVisible=true,
  verticalOffset=0,settleAfterMs=0,settleScrollBy=0,removeMenuAfterMs=0,
  placement='',popoverPadding=12,unfollowAction='',socialContext='',content=text}={}) {
  const menu=`<div role="menu" ${menuOwner ? `aria-labelledby="${menuOwner}"` : ''}
    ${placement?`style="height:${360-2*popoverPadding}px"`:''}>
    <button role="menuitem">Copy link to post</button>
    <a role="menuitem" ${embedVisible?'':'hidden'} href="/preload/embed-modal/?targetUrn=urn%3Ali%3Ashare%3A${postId}">Embed this post</a>
    ${ownerAction ? `<button role="menuitem">Hide posts by ${ownerAction}</button>` : ''}
    ${unfollowAction ? `<button role="menuitem">Unfollow ${unfollowAction}</button>` : ''}
  </div>`;
  const toggle=`onclick="const current = document.querySelector('[data-fixture-menu-portal]');
    if (current) {
      ${keepOpen?'':"this.setAttribute('aria-expanded','false'); current.remove();"}
    } else {
      ${expanded ? "this.setAttribute('aria-expanded','true');" : ''}
      const anchor = this.getBoundingClientRect();
      const portal = document.getElementById('${templateID}').content.firstElementChild.cloneNode(true);
      const popover = portal.firstElementChild;
      popover.style.top = (${placement==='above'?'Math.round(anchor.top) - 360':placement==='below'?'Math.round(anchor.bottom)':'anchor.bottom + 8'} + ${verticalOffset}) + 'px';
      popover.style.left = (anchor.right - 256 + ${offset}) + 'px';
      document.body.append(portal);
      popover.showPopover();
      ${settleAfterMs?`setTimeout(() => {
        ${settleScrollBy?`document.getElementById('workspace').scrollTop += ${settleScrollBy};`:''}
        const settledAnchor = this.getBoundingClientRect();
        popover.style.top = (settledAnchor.bottom + 8) + 'px';
        popover.style.left = (settledAnchor.right - 256) + 'px';
      }, ${settleAfterMs});`:''}
      ${removeMenuAfterMs?`setTimeout(() => portal.remove(), ${removeMenuAfterMs});`:''}
    }"`;
  return `<div data-testid="mainFeed" role="list"><div id="expanded-fixture-feed-entry">
    <div role="listitem" style="position:relative;width:600px;min-height:100px">
      ${socialContext ? `<div>${socialContext}</div>` : ''}
      <a href="/in/wulfie-bain/">${author}</a>
      <button aria-label="Open control menu for post by ${author}" style="position:absolute;right:5px;top:8px${placement?';width:32px;height:32px':''}" ${existingMenus?'':toggle}>Menu</button>
      ${content}
      <button>Comment</button><button>Repost</button>
    </div>
  </div></div>
  <nav><button id="foreign-control" ${competingTrigger ? 'aria-label="Open control menu for post by Another Author" aria-expanded="true"' : ''}>Private navigation menu</button></nav>
  ${sameAuthorControl ? `<button aria-label="Open control menu for post by ${author}">Private second card control</button>` : ''}
  <template id="${templateID}"><div data-floating-ui-portal data-fixture-menu-portal>
    <div popover="manual" style="position:fixed;inset:auto;margin:0;border:0;width:256px;padding:0${placement?`;box-sizing:border-box;height:360px;margin-top:8px;padding:${popoverPadding}px 0`:''}">${menu}</div>
  </div></template>
  ${existingMenus ? `<div role="menu" style="position:fixed;top:240px">${link(other)}</div>
    <div role="menu" style="position:fixed;top:290px">${link(other)}</div>` : ''}`;
}

const oraclePreview='<a href="https://blogs.oracle.com/fixture" style="display:block;width:480px;min-height:54px">'+
  '<img src="https://media.licdn.com/fixture-thumbnail.jpg" width="64" height="40" alt="">'+
  '<div>Announcing Oracle WebLogic Server and Coherence 26</div><div>blogs.oracle.com</div></a>';
const alibabaPoster='<div><img src="https://media.licdn.com/fixture-poster.jpg" style="display:block;width:470px;height:240px" alt="Conference poster"></div>';
const missingImage='<div class="update-components-image" style="width:470px;height:240px"><img width="470" height="240" alt="Image pending"></div>';

const cases=[
  {name:'native article',body:post,code:'ready'},
  {name:'Lambertus link preview keeps its thumbnail and offers no media recapture',
    body:wulfieCard({author:'Lambertus Setiawan W.',socialContext:'Resaldi Ramadhan Putra likes this',content:text+oraclePreview}),
    code:'no_valid_candidates',captureIdentity:shareIdentity(wulfieShare),
    captureMedia:{attachments:['link_preview'],thumbnail:true,images:0,recapture:false,windowedTrace:'owner_unverified',
      socialContext:'Resaldi Ramadhan Putra likes this'}},
  {name:'Alibaba Cloud keeps its captured conference image and offers no media recapture',
    body:wulfieCard({author:'Alibaba Cloud',content:text+alibabaPoster}),
    code:'no_valid_candidates',captureIdentity:shareIdentity(wulfieShare),
    captureMedia:{attachments:[],images:1,recapture:false,windowedTrace:'owner_unverified',socialContext:''}},
  {name:'A LinkedIn primary image shell without a URL offers media recapture',
    body:card({body:text+missingImage}),code:'ready',captureIdentity:ownIdentity,
    captureMedia:{attachments:[],images:0,recapture:true}},
  {name:'native wrapper',body:card({tag:'div'}),code:'ready'},
  {name:'root URN without anchor',body:card({links:''}),code:'ready',captureIdentity:ownIdentity},
  {name:'ugcPost root',body:card({id:null,links:'',attributes:`data-id="urn:li:ugcPost:${primary}"`}),code:'ready'},
  {name:'share root',body:card({id:null,links:'',attributes:`data-id="urn:li:share:${primary}"`}),code:'ready'},
  {name:'public activity anchor',body:card({id:null,links:`<a href="/posts/fixture-author_activity-${primary}-abc">Post</a>`}),code:'ready',captureIdentity:ownIdentity},
  {name:'action ancestor',body:`<section><span data-view-name="feed-actor-name">Private Author</span>${link(primary)}${text}<button>Like</button><button>Comment</button></section>`,code:'ready'},
  {name:'missing native identity',body:card({id:null,links:''}),code:'no_valid_candidates',rejections:{missing_identity:1}},
  {name:'Wulfie card recovers the live share URL from its anchored popover',body:wulfieCard(),
    code:'no_valid_candidates',rejections:{missing_identity:1},
    captureIdentity:{...wulfieIdentity,sources:{windowed:'embed_urn',headless:'adapter_recovery'}},
    recoveryClickCount:2,reportCaptureIdentity:true},
  {name:'Ivo above the button recovers its permalink with the measured wrapper margin',
    body:scrollLayout(977.5,wulfieCard({author:'Ivo Setyadi',postId:ivoShare,placement:'above'})),
    batchScroll:433,code:'no_valid_candidates',captureIdentity:shareIdentity(ivoShare),recoveryClickCount:2,
    anchorPolls:[],anchorGeometry:anchorGeometry(433,-8.5,-8.5,433,-0.5),reportCaptureIdentity:true},
  {name:'Ivo centered below the button recovers through normal anchor geometry',
    body:scrollLayout(977.5,wulfieCard({author:'Ivo Setyadi',postId:ivoShare,placement:'below'})),
    batchScroll:581,code:'no_valid_candidates',captureIdentity:shareIdentity(ivoShare),recoveryClickCount:2,
    anchorGeometry:anchorGeometry(581,8.5),reportCaptureIdentity:true},
  {name:'Margin correction does not admit an actually overlapping menu body',
    body:scrollLayout(977.5,wulfieCard({author:'Ivo Setyadi',postId:ivoShare,placement:'above',popoverPadding:0})),
    batchScroll:433,code:'no_valid_candidates',captureSequence:[null],recoveryClickCount:1,
    anchorPolls:[{persistence:'repeated',vertical:true,overlap:true}],anchorGeometry:anchorGeometry(433,-8.5)},
  {name:'Margin correction cannot override a foreign ARIA reference',
    body:scrollLayout(977.5,wulfieCard({author:'Ivo Setyadi',postId:ivoShare,placement:'above',menuOwner:'foreign-control'})),
    batchScroll:433,code:'no_valid_candidates',captureSequence:[null],recoveryClickCount:1,
    anchorGeometry:[{snapshotIndex:0,candidateIndex:0,scrollTopAtClick:433,first:null,last:null}]},
  ...['Ivo Setyadi','Putra Adhiguna'].map(author=>({
    name:`Unfollow closes the owned ${author} menu after binding fails and the next card recovers`,
    body:wulfieCard({author,unfollowAction:author,offset:-175,embedVisible:false,templateID:'first-menu'})+wulfieCard(),
    code:'no_valid_candidates',captureSequence:[null,wulfieIdentity.id],recoveryClickCount:4,recoveryBlocker:null,
  })),
  {name:'Unfollow cannot override a conflicting Hide posts by author',
    body:wulfieCard({author:'Ivo Setyadi',unfollowAction:'Ivo Setyadi',ownerAction:'Other Author',offset:-175}),
    code:'no_valid_candidates',captureSequence:[null],recoveryClickCount:1},
  {name:'Unfollow cannot close a menu when two buttons have the same author',
    body:wulfieCard({author:'Ivo Setyadi',unfollowAction:'Ivo Setyadi',sameAuthorControl:true,offset:-175}),
    code:'no_valid_candidates',captureSequence:[null],recoveryClickCount:1},
  {name:'Yos layout at batch scroll recovers the verified share URL with a visible control',
    body:scrollLayout(3254,wulfieCard({author:'Yos Vincenzo',postId:yosShare,ownerAction:'Yos Vincenzo'})),
    batchScroll:3059,code:'no_valid_candidates',captureIdentity:shareIdentity(yosShare),
    recoveryClickCount:2,recoveryFailures:[],reportCaptureIdentity:true,anchorGeometry:anchorGeometry(3059,8)},
  {name:'Yos layout at batch scroll diagnoses a clipped control and a misaligned popover',
    body:scrollLayout(3043,wulfieCard({author:'Yos Vincenzo',postId:yosShare,ownerAction:'Yos Vincenzo',offset:-60})),
    batchScroll:3059,code:'no_valid_candidates',captureSequence:[null],recoveryClickCount:2,
    recoveryFailures:[{snapshotIndex:0,candidateIndex:0,stage:'ownership',reason:'anchor_mismatch',viewport:'visible',scrollRoot:'partial'}],
    anchorPolls:[{persistence:'repeated',horizontal:true}]},
  {name:'Shikhar layout at batch scroll recovers the verified share URL with a visible control',
    body:scrollLayout(3455,wulfieCard({author:'Shikhar Kwatra',postId:shikharShare,ownerAction:'Shikhar Kwatra'})),
    batchScroll:3059,code:'no_valid_candidates',captureIdentity:shareIdentity(shikharShare),
    recoveryClickCount:2,recoveryFailures:[],reportCaptureIdentity:true},
  {name:'Shikhar layout at batch scroll diagnoses a bound menu without visible permalink evidence',
    body:scrollLayout(3455,wulfieCard({author:'Shikhar Kwatra',postId:shikharShare,ownerAction:'Shikhar Kwatra',embedVisible:false})),
    batchScroll:3059,code:'no_valid_candidates',captureSequence:[null],recoveryClickCount:2,
    recoveryFailures:[{snapshotIndex:0,candidateIndex:0,stage:'menu_evidence',reason:'no_visible_urn_or_embed_action',viewport:'visible',scrollRoot:'visible'}]},
  {name:'Shikhar layout records persistent vertical mismatch across polls',
    body:scrollLayout(3455,wulfieCard({author:'Shikhar Kwatra',postId:shikharShare,ownerAction:'Shikhar Kwatra',verticalOffset:50})),
    batchScroll:3059,code:'no_valid_candidates',captureSequence:[null],recoveryClickCount:2,
    anchorPolls:[{persistence:'repeated',vertical:true,gap:true}]},
  {name:'Shikhar layout records persistent excessive vertical overlap',
    body:scrollLayout(3455,wulfieCard({author:'Shikhar Kwatra',postId:shikharShare,ownerAction:'Shikhar Kwatra',verticalOffset:-20})),
    batchScroll:3059,code:'no_valid_candidates',captureSequence:[null],recoveryClickCount:2,
    anchorPolls:[{persistence:'repeated',vertical:true,overlap:true}],anchorGeometry:anchorGeometry(3059,-12)},
  {name:'Shikhar layout records a two-axis mismatch that settles after the feed scrolls during polling',
    body:scrollLayout(3455,wulfieCard({author:'Shikhar Kwatra',postId:shikharShare,ownerAction:'Shikhar Kwatra',
      offset:-60,verticalOffset:50,settleAfterMs:160,settleScrollBy:91})),
    batchScroll:3059,code:'no_valid_candidates',captureIdentity:shareIdentity(shikharShare),recoveryClickCount:2,
    reportCaptureIdentity:true,anchorPolls:[{persistence:'resolved',horizontal:true,vertical:true,gap:true}],
    anchorGeometry:anchorGeometry(3059,58,8,3150)},
  {name:'Popover disappearing after one mismatch interrupts the polling sequence',
    body:wulfieCard({offset:-60,removeMenuAfterMs:20}),code:'no_valid_candidates',captureSequence:[null],recoveryClickCount:1,
    anchorPolls:[{persistence:'interrupted',horizontal:true}]},
  {name:'Wulfie recovers its own menu while two static sidebar promotions are visible',
    body:wulfieCard()+sidebar(promoMenus),code:'no_valid_candidates',recoveryBlocker:null,
    captureIdentity:{...wulfieIdentity,sources:{windowed:'embed_urn',headless:'adapter_recovery'}},
    recoveryClickCount:2,reportCaptureIdentity:true},
  {name:'Sidebar promotions do not exempt a visible dialog',
    body:wulfieCard()+sidebar(promoMenus)+'<div role="dialog" style="height:50px">Private dialog</div>',
    code:'no_valid_candidates',recoveryBlocker:'role_dialog',recoveryMenuCount:0,
    captureIdentity:{headlessRejection:'missing_identity'},recoveryClickCount:0},
  {name:'Sidebar promotions do not exempt a separate player menu',
    body:wulfieCard()+sidebar(promoMenus)+'<ul role="menu" class="vjs-menu-content"><li role="menuitem">Playback rate</li></ul>',
    code:'no_valid_candidates',recoveryBlocker:'role_menu',recoveryMenuCount:1,
    captureIdentity:{headlessRejection:'missing_identity'},recoveryClickCount:0},
  ...[
    ['similar path',sidebar(sidebarMenu('https://www.linkedin.com/premium/products/other','Private other product'))],
    ['foreign origin',sidebar(sidebarMenu('https://foreign.test/premium/products/','Private foreign product'))],
    ['promo link outside the sidebar',sidebarMenu(premiumPromoUrl,'Private external menu')],
    ['extra action in promo menu',sidebar(sidebarMenu(premiumPromoUrl,'Private premium','','<button>Private action</button>'))],
    ['promo destination under a named menu',sidebar(sidebarMenu(premiumPromoUrl,'Private premium','aria-labelledby="foreign-control"'))],
  ].map(([label,body])=>({name:`Recovery still blocks ${label}`,body:wulfieCard()+body,code:'no_valid_candidates',
    recoveryBlocker:'role_menu',recoveryMenuCount:1,captureIdentity:{headlessRejection:'missing_identity'},recoveryClickCount:0})),
  {name:'Wulfie card rejects a menu labelled for another control',
    body:wulfieCard({menuOwner:'foreign-control',postId:other}),
    code:'no_valid_candidates',rejections:{missing_identity:1},
    captureIdentity:{headlessRejection:'missing_identity'},recoveryClickCount:1,reportCaptureIdentity:true},
  {name:'Wulfie card leaves two pre-existing menus alone',
    body:wulfieCard({existingMenus:true}),code:'no_valid_candidates',rejections:{missing_identity:1},
    captureIdentity:{headlessRejection:'missing_identity'},recoveryBlocker:'role_menu',
    recoveryMenuCount:2,recoveryClickCount:0,reportCaptureIdentity:true},
  {name:'Wulfie card rejects a portal when its control did not expand',body:wulfieCard({expanded:false,postId:other}),
    code:'no_valid_candidates',captureIdentity:{headlessRejection:'missing_identity'},recoveryClickCount:1,reportCaptureIdentity:true},
  {name:'Wulfie card rejects a portal anchored elsewhere despite expansion',body:wulfieCard({offset:-175,postId:other}),
    code:'no_valid_candidates',captureIdentity:{headlessRejection:'missing_identity'},recoveryClickCount:1,reportCaptureIdentity:true},
  {name:'Unbound popover from an earlier recovery attempt correlates across snapshots',
    body:wulfieCard({offset:-175}),secondSnapshot:true,code:'no_valid_candidates',recoveryBlocker:'role_menu',recoveryClickCount:1,
    priorRecoveryMenus:[{slot:0,attemptSnapshotIndex:0,attemptCandidateIndex:0,sameButton:true,menuBound:false,triggerExpanded:true,cleanup:'not_attempted'}]},
  {name:'A bound menu that stays visible after cleanup correlates across snapshots',
    body:wulfieCard({keepOpen:true}),secondSnapshot:true,code:'no_valid_candidates',recoveryBlocker:'role_menu',recoveryClickCount:2,
    priorRecoveryMenus:[{slot:0,attemptSnapshotIndex:0,attemptCandidateIndex:0,sameButton:true,menuBound:true,triggerExpanded:true,cleanup:'still_visible'}]},
  {name:'Explicit foreign menu remains blocked across snapshots without self-attribution',
    body:wulfieCard({menuOwner:'foreign-control',postId:other,offset:-175,ownerAction:'Wulfie Bain'}),secondSnapshot:true,code:'no_valid_candidates',recoveryBlocker:'role_menu',
    recoveryClickCount:1,priorRecoveryMenus:null},
  {name:'Verified cleanup closes a shifted menu without accepting its URL and allows the next post',
    body:wulfieCard({offset:-175,postId:other,author:'Private Recovery Author',ownerAction:'Private Recovery Author',templateID:'first-menu'})+wulfieCard(),
    code:'no_valid_candidates',recoveryBlocker:null,recoveryClickCount:4,reportCaptureIdentity:true,
    captureSequence:[null,wulfieIdentity.id]},
  {name:'A failed cleanup of a shifted owned menu is still correlated without URL binding',
    body:wulfieCard({offset:-175,ownerAction:'Wulfie Bain',keepOpen:true}),secondSnapshot:true,code:'no_valid_candidates',
    recoveryBlocker:'role_menu',recoveryClickCount:2,
    priorRecoveryMenus:[{slot:0,attemptSnapshotIndex:0,attemptCandidateIndex:0,sameButton:true,menuBound:false,triggerExpanded:true,cleanup:'still_visible'}]},
  {name:'Cleanup does not operate a shifted menu labelled with a different author',
    body:wulfieCard({offset:-175,ownerAction:'Different Author'}),code:'no_valid_candidates',
    recoveryBlocker:null,recoveryClickCount:1,captureIdentity:{headlessRejection:'missing_identity'}},
  {name:'Cleanup does not use an author action when two post controls share the author',
    body:wulfieCard({offset:-175,ownerAction:'Wulfie Bain',sameAuthorControl:true}),code:'no_valid_candidates',
    recoveryBlocker:null,recoveryClickCount:1,captureIdentity:{headlessRejection:'missing_identity'}},
  {name:'Wulfie card rejects a portal when another post control is also expanded',body:wulfieCard({competingTrigger:true,postId:other}),
    code:'no_valid_candidates',captureIdentity:{headlessRejection:'missing_identity'},recoveryClickCount:1,reportCaptureIdentity:true},
  {name:'existing menu blocks recovery',body:blockedPost+'<div role="menu" style="width:120px;height:50px">Private menu</div>',code:'no_valid_candidates',recoveryBlocker:'role_menu'},
  {name:'surface inspection separates navigation, feed sorting and dialog without entering recovery',
    body:blockedPost.replace('id="post-control"','id="post-control" style="position:fixed;top:300px;left:30px"')+
      '<nav><button aria-controls="Private-account-menu" aria-expanded="true">Private account</button><div id="Private-account-menu" role="menu" style="position:fixed;top:300px;left:20px;width:180px;height:80px"><button role="menuitem">Settings &amp; Privacy</button></div></nav>'+
      '<section data-view-name="feed-sort-menu"><div role="menu" style="position:fixed;top:300px;left:220px;width:150px;height:80px"><button role="menuitem">Recent</button></div></section>'+
      '<div role="dialog" style="position:fixed;top:400px;left:20px;width:150px;height:50px"><button>Close</button></div>',
    code:'no_valid_candidates',recoveryBlocker:'role_menu',surfaceInspection:{dialogCount:1,complete:true,surfaces:[
      {kind:'menu',slot:0,trail:['navigation','main'],actionHints:['account'],actionCount:1,ownerRelation:'unknown',triggerBinding:'target_reference',triggerTrail:['navigation','main'],triggerExpanded:true,openState:'open',controlOcclusion:'surface_hit'},
      {kind:'menu',slot:1,trail:['feed_sort','main'],actionHints:['feed_order'],actionCount:1,ownerRelation:'unknown',triggerBinding:'unresolved',triggerTrail:[],triggerExpanded:null,openState:'unknown',controlOcclusion:'other_hit'},
      {kind:'dialog',slot:2,trail:['main'],actionHints:['dismiss'],actionCount:1,ownerRelation:'unknown',triggerBinding:'unresolved',triggerTrail:[],triggerExpanded:null,openState:'unknown',controlOcclusion:'other_hit'},
    ]}},
  {name:'two menus report distinct triggers, open signals and hit tests',
    body:blockedPost.replace('id="post-control"','id="post-control" aria-controls="Private:post-menu" aria-owns="Private:post-menu" aria-expanded="true"')+
      '<div class="artdeco-dropdown__content artdeco-dropdown__content--is-open" id="Private:post-menu" role="menu" style="position:fixed;top:260px;left:20px;width:120px;height:50px"><button role="menuitem">Private action</button></div>'+
      '<nav><button popovertarget="Private-nav-menu" aria-expanded="false">Private navigation</button><div aria-owns="Private-nav-menu" aria-expanded="true">Private owner</div></nav>'+
      '<div popover id="Private-nav-menu" role="menu" style="display:block;position:fixed;inset:260px auto auto 200px;margin:0;width:120px;height:50px">Private nav menu</div>'+
      '<div style="position:fixed;inset:250px auto auto 190px;width:170px;height:100px;z-index:10">Private overlay</div>',
    code:'no_valid_candidates',recoveryBlocker:'role_menu',menuEvidence:[
      {component:'artdeco_dropdown',scope:'feed_post',ariaOwnerRelation:'candidate',triggerBinding:'target_reference',triggerRelation:'candidate',triggerExpanded:true,popoverOpen:null,dropdownOpen:true,cssCheck:true,hitTest:{hit:5,covered:0,miss:0}},
      {component:'popover',scope:'navigation',ariaOwnerRelation:'other',triggerBinding:'target_reference',triggerRelation:'other',triggerExpanded:false,popoverOpen:false,dropdownOpen:null,cssCheck:true,hitTest:{hit:0,covered:5,miss:0}},
    ]},
  {name:'clipped menu with competing trigger references stays blocked',
    body:blockedPost.replace('id="post-control"','id="post-control" aria-controls="Private-clipped-menu"')+
      '<button id="Private-foreign-trigger" aria-expanded="true">Private trigger</button>'+
      '<div role="menu" id="Private-clipped-menu" aria-labelledby="Private-foreign-trigger" style="width:120px;height:50px;clip-path:inset(100%)">Private menu</div>',
    code:'no_valid_candidates',recoveryBlocker:'role_menu',menuEvidence:[
      {component:'unclassified',scope:'unclassified',ariaOwnerRelation:'absent',triggerBinding:'ambiguous',triggerRelation:'unknown',triggerExpanded:null,popoverOpen:null,dropdownOpen:null,cssCheck:true,hitTest:{hit:0,covered:0,miss:5}},
    ]},
  {name:'Artdeco toggle supplies a structural hint without ARIA references',
    body:card({id:null,links:'',body:text+`<div class="artdeco-dropdown">
      <button class="artdeco-dropdown__trigger" aria-label="Open control menu for post by Private Author" aria-expanded="false">Menu</button>
      <div role="menu" class="artdeco-dropdown__content" style="width:120px;height:50px">Private dropdown</div>
    </div>`}),code:'no_valid_candidates',recoveryBlocker:'role_menu',menuEvidence:[
      {component:'artdeco_dropdown',scope:'feed_post',ariaOwnerRelation:'unknown',triggerBinding:'component_toggle',triggerRelation:'candidate',triggerExpanded:false,popoverOpen:null,dropdownOpen:false,cssCheck:true,hitTest:{hit:5,covered:0,miss:0}},
    ]},
  {name:'noninteractive ARIA owner cannot supply a menu trigger or expanded state',
    body:blockedPost+'<nav><div aria-owns="Private-portal" aria-controls="Private-portal" aria-expanded="true">Private noninteractive owner</div></nav>'+
      '<div id="Private-portal" role="menu" style="width:120px;height:50px">Private menu</div>',
    code:'no_valid_candidates',recoveryBlocker:'role_menu',menuEvidence:[
      {component:'unclassified',scope:'unclassified',ariaOwnerRelation:'other',triggerBinding:'unresolved',triggerRelation:'unknown',triggerExpanded:null,popoverOpen:null,dropdownOpen:null,cssCheck:true,hitTest:{hit:5,covered:0,miss:0}},
    ]},
  {name:'existing dialog blocks recovery',body:blockedPost+'<div role="dialog" style="width:120px;height:50px">Private dialog</div>',code:'no_valid_candidates',recoveryBlocker:'role_dialog'},
  {name:'aria-hidden dialog remains a recovery blocker',body:blockedPost+'<div aria-hidden="true"><div role="dialog" style="width:120px;height:50px">Private hidden dialog</div></div>',code:'no_valid_candidates',recoveryBlocker:'role_dialog',ancestorFlag:'ariaHidden'},
  {name:'hidden dialog does not block recovery',body:blockedPost+'<div role="dialog" style="display:none">Private hidden dialog</div>',code:'no_valid_candidates',recoveryBlocker:null,recoveryAttempt:true},
  {name:'conflicting ARIA references stay diagnostic',body:blockedPost.replace('id="post-control"','id="post-control" aria-controls="other-surface"')+'<div role="dialog" id="actual-surface" aria-labelledby="post-control" style="width:120px;height:50px">Private dialog</div>',code:'no_valid_candidates',recoveryBlocker:'role_dialog',aria:{labelledBy:'matches',controls:'different',owns:'absent'}},
  {name:'inert ancestor remains a recovery blocker',body:blockedPost+'<div inert><div role="dialog" style="width:120px;height:50px">Private inert dialog</div></div>',code:'no_valid_candidates',recoveryBlocker:'role_dialog',ancestorFlag:'inert'},
  {name:'transparent ancestor records the existing visibility difference',body:blockedPost+'<div style="opacity:0"><div role="dialog" style="width:120px;height:50px">Private transparent dialog</div></div>',code:'no_valid_candidates',recoveryBlocker:'role_dialog',ancestorFlag:'opacityZero',recoveryDifference:'Windowed checks the surface style only; Headless also checks ancestor styles. The transparent ancestor therefore blocks only Windowed. This diagnostic increment preserves both operational predicates.',headlessRecoveryAttempt:true},
  {name:'foreign native URL',body:card({id:null,links:`<a href="https://foreign.test/posts/author_activity-${primary}-x">Post</a>`}),code:'no_valid_candidates',captureIdentity:{headlessRejection:'missing_identity'}},
  {name:'profile URL is not a post',body:card({id:null,links:`<a href="/in/author-activity-${primary}/">Profile</a>`}),code:'no_valid_candidates'},
  {name:'profile wrapping a timestamp does not identify post',body:card({id:null,links:'<a href="/in/fixture-author/"><time datetime="2026-11-15T00:00:00Z">1d</time></a>'}),
    code:'no_valid_candidates',captureIdentity:{headlessRejection:'missing_identity'}},
  {name:'conflicting root and anchor',body:card({links:link(other),body:text+'<button aria-label="Open control menu for post by Private Author">Menu</button>'}),
    code:'no_valid_candidates',rejections:{identity_conflict:1},captureIdentity:{headlessRejection:'ambiguous_identity',noRecovery:true}},
  {name:'root URN and only native anchor belongs to embedded repost',body:card({links:'',body:text+embeddedRepost}),code:'ready',captureIdentity:ownIdentity},
  {name:'embedded repost cannot identify parent that has text and image',body:card({id:null,links:'',body:text+embeddedRepost+
    '<div class="update-components-image"><img width="320" height="180" src="https://media.licdn.com/fixture.jpg"></div>'}),
    code:'no_valid_candidates',captureIdentity:{headlessRejection:'missing_identity',mediaKind:'image'}},
  {name:'authored native link is not the primary post identity',body:card({id:null,links:'',body:text.replace('</div>',` ${link(other)}</div>`)}),
    code:'no_valid_candidates',captureIdentity:{headlessRejection:'missing_identity',linkUrl:`https://www.linkedin.com/feed/update/urn:li:activity:${other}/`}},
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
  {name:'node bound',body:card({links:Array.from({length:101},()=>link(primary)).join('')}),code:'no_valid_candidates',captureIdentity:{headlessRejection:'identity_unverified'}},
  {name:'claim bound',body:card({links:Array.from({length:9},(_,n)=>link('12345678901234567'+String(n).padStart(2,'0'))).join('')}),code:'no_valid_candidates'},
  {name:'text traversal bound',body:card({body:'<div data-testid="expandable-text-box">'+Array.from({length:101},()=>'<span>Private text</span>').join('')+'</div>'}),code:'no_valid_candidates'},
  {name:'candidate bound',body:Array.from({length:65},()=>post).join(''),code:'ready',counts:{candidates:64}},
  {name:'failing probe',body:post,failProbe:true},
];

export async function runLinkedInShadowParity(chrome,output) {
  return runDOMShadowParity({chrome,output,source:'linkedin',canonicalUrl:'https://www.linkedin.com/feed/',probeGlobal:'AkuLinkedInDOMShadow',cases,
    privacyPattern:`Private|1234567890123456789|9876543210987654321|${wulfieShare}|${yosShare}|${shikharShare}|${ivoShare}|Wulfie Bain|Yos Vincenzo|Shikhar Kwatra|Ivo Setyadi|Putra Adhiguna|licdn|linkedin:|fixture-author`,
    style:'article,.feed-shared-update-v2,section{width:600px;min-height:100px}span{display:block}'});
}
if(process.argv[1]&&resolve(process.argv[1])===fileURLToPath(import.meta.url)){
  const report=await runLinkedInShadowParity(process.argv[2]??'C:/Program Files/Google/Chrome/Application/chrome.exe',process.argv[3]);
  console.log(JSON.stringify({fixtures:report.fixtures,engineSha256:report.engineSha256,effects:report.effects}));
}
