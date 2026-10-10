import test from 'node:test';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import { readFile } from 'node:fs/promises';

const extractor = await readFile(new URL('../vendor/adapter-extract.js', import.meta.url), 'utf8');

function element({ tagName = 'DIV', rect = { width: 500, height: 240, left: 0, right: 500, top: 20, bottom: 260 },
  attrs = {}, text = '', parent = null, children = [] } = {}) {
  return {
    tagName, parentElement: parent, innerText: text, textContent: text, childNodes: [], style: {},
    href: attrs.href || '', src: attrs.src || '', currentSrc: attrs.currentSrc || attrs.src || '', alt: attrs.alt || '',
    complete: attrs.complete ?? true, naturalWidth: attrs.naturalWidth ?? 800, naturalHeight: attrs.naturalHeight ?? 450,
    readyState: attrs.readyState ?? 0, paused: true, currentTime: 0, duration: NaN,
    getBoundingClientRect: () => ({ ...rect }),
    getAttribute: name => attrs[name] ?? null,
    hasAttribute: name => attrs[name] !== undefined,
    querySelectorAll: selector => {
      if (selector === 'img') return children.filter(child => child.tagName === 'IMG');
      if (selector === 'video') return children.filter(child => child.tagName === 'VIDEO');
      return [];
    },
    querySelector: () => null,
    closest: () => null,
    contains: node => node === this || children.includes(node),
  };
}

function anchor(href, text = '') {
  return element({ tagName: 'A', rect: { width: 80, height: 18, left: 10, right: 90, top: 40, bottom: 58 },
    attrs: { href }, text });
}

function candidate({ urn = null, anchors = [], images = [], times = [], body = 'Visible source post',
  rect, commentImages = [], contentLinks = anchors } = {}) {
  const imageRoots = [...images, ...commentImages];
  const root = {
    tagName: 'ARTICLE', parentElement: null, innerText: body, textContent: body, style: {},
    getAttribute: name => name === 'data-urn' ? urn : null,
    getBoundingClientRect: () => ({ width: 600, height: 300, left: 0, right: 600, top: 10, bottom: 310, ...rect }),
    contains: node => node === root || anchors.includes(node) || imageRoots.includes(node) || times.includes(node)
      || contentRoot === node || contentLinks.includes(node) || commentImages.some(image => image === node),
    querySelectorAll(selector) {
      if (selector === 'a[href]') return anchors;
      if (selector.includes('data-urn') || selector.includes('data-id')) return anchors;
      if (selector === 'time[datetime]') return times;
      if (selector === 'time') return times;
      if (selector === 'img') return imageRoots;
      if (selector === 'video') return [];
      if (selector.startsWith('button[aria-label^=')) return [];
      if (selector === '.update-components-image, .feed-shared-image') return imageRoots;
      if (selector === 'video, [data-view-name*=\'video\' i], [aria-label*=\'video\' i]') return [];
      if (selector === '[data-test-document-container], iframe[title*=\'document\' i]') return [];
      return [];
    },
    querySelector(selector) {
      if (selector === 'time') return times[0] || null;
      if (selector === '[data-testid="expandable-text-box"]') return contentRoot;
      return null;
    },
  };
  const contentRoot = { innerText: body, textContent: body, querySelectorAll: selector => selector === 'a[href]' ? contentLinks : [],
    contains: node => contentLinks.includes(node) };
  for (const image of imageRoots) image.parentElement = root;
  return root;
}

function run(source, adapter, url) {
  const context = vm.createContext({
    URL,setTimeout,clearTimeout,
    location: new URL(url),
    innerHeight: 900,
    innerWidth: 1280,
    scrollY: 0,
    getComputedStyle: element => ({ display: 'block', visibility: 'visible', opacity: '1',...element.style }),
    document: { visibilityState: 'visible', title: 'Fixture', readyState: 'complete', scrollingElement: { scrollHeight: 1400 },
      body: { innerText: '' }, querySelector: () => null,querySelectorAll:()=>[] },
    AkuSourceAdapters: { get: id => id === source ? adapter : null },
  });
  vm.runInContext(extractor, context);
  return context.XHeadlessPoC;
}

function baseAdapter(source, candidates) {
  const identityFor = value => {
    if (source === 'instagram') {
      const match = String(value || '').match(/\/(p|reel|tv)\/([A-Za-z0-9_-]+)/i);
      return match ? `instagram:${match[1].toLowerCase()}:${match[2]}` : null;
    }
    const match = String(value || '').match(/urn:li:(activity|ugcPost|share):(\d{5,30})/i);
    return match ? `linkedin:${match[1].toLowerCase()}:${match[2]}` : null;
  };
  return {
    source, version: `${source}-fixture`, matchesPage: () => true,
    discoverCandidates: () => ({ candidates, strategy: 'fixture_native_permalink' }),
    platformIdFromCandidates: values => values.map(identityFor).find(Boolean) || null,
    findPermalinkDetails: source === 'instagram' ? container => ({ url: container.querySelectorAll('a[href]')[0]?.href }) : undefined,
    findDomPermalink: source === 'linkedin' ? container => `https://www.linkedin.com/feed/update/${container.getAttribute('data-urn')}/` : undefined,
    findAuthor: () => 'Fixture author', findAvatar: () => null,
    extractText: container => container.innerText,
    extractSemantics: () => ({ contentKind: 'post', relationshipType: 'original', engagement: { like: '3' } }),
    extractPresentation: () => ({ promoted: false, timestampText: '2h', originSignals: [{ kind: 'platform_ai_label' }] }),
    estimateRelativeTimestamp: (_text, capturedAt) => ({ publishedAt: capturedAt, precision: 'hour' }),
    contentRootSelector: '[data-testid="expandable-text-box"]',
    contentExpansion: { buttonSelector: 'button,[role="button"]' },
    mediaHosts: source === 'instagram' ? ['fbcdn.net', 'cdninstagram.com'] : ['licdn.com'],
    shouldSkipImage: () => false,
    loginRequired: () => false,
    mediaAcquisition: {
      detectExpectedKinds(container, helpers) {
        return [...container.querySelectorAll('.update-components-image, .feed-shared-image')]
          .filter(root => !helpers.excludeRoot.contains(root)).map(() => 'image');
      },
      extractCandidates(container, helpers) {
        const selector = source === 'instagram' ? 'img' : '.update-components-image, .feed-shared-image';
        return container.querySelectorAll(selector).flatMap(root => helpers.excludeRoot.contains(root)
          ? [] : helpers.collectRootCandidates(root, { kind: 'image', alt: 'Attached image' }));
      },
    },
    extractAttachments: source === 'linkedin' ? () => [
      { kind: 'link_preview', title: 'Owned preview', url: 'https://example.org/article' },
      { kind: 'link_preview', title: 'Comment preview', url: 'https://comment.example/article' },
    ] : undefined,
    extractDirectContext: source === 'linkedin' ? () => [{ kind: 'feed_comment', target: { availability: 'reference_only' } }] : undefined,
  };
}

test('Instagram admits visible own native posts and refuses hidden or identifier-free text', async () => {
  const shortcode = 'Dd-VoSSpoav';
  const media = element({ tagName: 'IMG', attrs: { src: 'https://scontent.cdninstagram.com/media/post.jpg',
    currentSrc: 'https://scontent.cdninstagram.com/media/post.jpg', alt: 'At the coast', naturalWidth: 1200, naturalHeight: 900 } });
  const time = element({ tagName: 'TIME', attrs: { datetime: '2026-10-02T10:00:00Z' } });
  const own = candidate({ anchors: [anchor(`https://www.instagram.com/p/${shortcode}/`)], images: [media], times: [time], body: 'Coastal morning.' });
  const hidden = candidate({ anchors: [anchor('https://www.instagram.com/reel/Hidden123/')], body: 'Hidden post', rect: { height: 0, bottom: 0 } });
  const textOnly = candidate({ body: 'Plausible caption with no native post link' });
  const api = run('instagram', baseAdapter('instagram', [own, hidden, textOnly]), 'https://www.instagram.com/');
  const result = await api.collect();
  assert.equal(result.posts.length, 1);
  assert.equal(result.posts[0].id, 'instagram:p:Dd-VoSSpoav');
  assert.equal(result.posts[0].permalink, 'https://www.instagram.com/p/Dd-VoSSpoav/');
  assert.equal(result.posts[0].text, 'Coastal morning.');
  assert.equal(result.posts[0].media[0].url, media.currentSrc);
  assert.equal(result.posts[0].presentation.originSignals[0].kind, 'platform_ai_label');
  assert.equal(result.rejectionReasons.not_visible, 1);
  assert.equal(result.rejectionReasons.missing_identity, 1);
  assert.equal(JSON.stringify(api.prepareEvidenceTargets()), '[]');
});

test('LinkedIn preserves own attachments and presentation while excluding ambiguous posts and comment media', async () => {
  const nativeId = '7512001669308555265';
  const ownImage = element({ tagName: 'IMG', attrs: { src: 'https://media.licdn.com/dms/image/own.jpg',
    currentSrc: 'https://media.licdn.com/dms/image/own.jpg', alt: 'Project cover', naturalWidth: 1200, naturalHeight: 700 } });
  const commentImage = element({ tagName: 'IMG', attrs: { src: 'https://media.licdn.com/dms/image/comment.jpg',
    currentSrc: 'https://media.licdn.com/dms/image/comment.jpg', alt: 'Comment image', naturalWidth: 900, naturalHeight: 600 } });
  const commentRoot = { contains: node => node === commentImage || node === commentLink };
  commentImage.closest = selector => selector.includes('comments-comment-item') ? commentRoot : null;
  const preview = anchor('https://example.org/article', 'Read the article');
  const commentLink = anchor('https://comment.example/article', 'Comment link');
  commentLink.closest = selector => selector.includes('comments-comment-item') ? commentRoot : null;
  const own = candidate({ urn: `urn:li:ugcPost:${nativeId}`, anchors: [
    anchor(`https://www.linkedin.com/feed/update/urn:li:ugcPost:${nativeId}/`), preview, commentLink,
  ], images: [ownImage], commentImages: [commentImage], body: 'Shipping update.', contentLinks: [preview, commentLink] });
  const ambiguous = candidate({ urn: 'urn:li:activity:7512001669308555266', anchors: [
    anchor('https://www.linkedin.com/feed/update/urn:li:activity:7512001669308555266/'),
    anchor('https://www.linkedin.com/feed/update/urn:li:share:7512001669308555267/'),
  ], body: 'Ambiguous source identity.' });
  const api = run('linkedin', baseAdapter('linkedin', [own, ambiguous]), 'https://www.linkedin.com/feed/');
  const result = await api.collect();
  assert.equal(result.posts.length, 1);
  assert.equal(result.posts[0].id, `linkedin:ugcpost:${nativeId}`);
  assert.equal(result.posts[0].media.length, 1);
  assert.equal(result.posts[0].media[0].url, ownImage.currentSrc);
  assert.equal(result.posts[0].attachments.length, 1);
  assert.equal(result.posts[0].attachments[0].url, 'https://example.org/article');
  assert.equal(JSON.stringify(result.posts[0].links.map(link => link.href)), '["https://example.org/article"]');
  assert.equal(result.posts[0].presentation.promoted, false);
  assert.equal(JSON.stringify(result.posts[0].directContext.map(item => item.kind)), '["feed_comment"]');
  assert.equal(result.rejectionReasons.ambiguous_identity, 1);
  assert.equal(JSON.stringify(api.prepareEvidenceTargets()), '[]');
});

test('Instagram explicit target binds only one article and rejects ambiguity or conflicting identity',async()=>{
  const root=candidate({body:'Native target caption'});
  const adapter=baseAdapter('instagram',[]);
  adapter.discoverCandidates=()=>({candidates:[],readinessCandidates:[root]});
  const collect=()=>run('instagram',adapter,'https://www.instagram.com/p/Target_1/').collect();
  assert.equal((await collect()).posts[0].id,'instagram:p:Target_1');
  adapter.discoverCandidates=()=>({candidates:[],readinessCandidates:[root,candidate({body:'Another post'})]});
  assert.equal((await collect()).posts.length,0);
  adapter.discoverCandidates=()=>({candidates:[],readinessCandidates:[candidate({anchors:[anchor('https://www.instagram.com/p/Foreign_1/')],body:'Another'})]});
  assert.equal((await collect()).posts.length,0);
});

test('LinkedIn adapter recovery binds the missing native ID to its own visible candidate',async()=>{
  const root=candidate({body:'Recovery fixture'});
  const adapter=baseAdapter('linkedin',[root]);
  let calls=0;
  adapter.recoverPermalinks=async(roots,deadline,helpers)=>{
    calls++;assert.equal(roots.length,1);assert.equal(roots[0],root);assert.ok(deadline>Date.now());
    assert.equal(helpers.findPermalinkDetails(root),null);
    return new WeakMap([[root,{url:'https://www.linkedin.com/feed/update/urn:li:activity:1234567890/'}]]);
  };
  const result=await run('linkedin',adapter,'https://www.linkedin.com/feed/').collect();
  assert.equal(calls,1);
  assert.equal(result.posts[0].id,'linkedin:activity:1234567890');
  assert.equal(result.posts[0].presentation.permalinkSource,'adapter_recovery');
  assert.equal(result.candidateDiagnostics.permalinkRecoveryCount,1);
});

test('source container scroll drives frontier metrics and restores the same retained root',async()=>{
  const root=candidate({urn:'urn:li:activity:1234567890',body:'Scrollable post'});
  const container={isConnected:true,clientHeight:500,scrollHeight:3000,scrollTop:0,style:{overflowY:'auto'},parentElement:null,
    scrollBy(_x,delta){this.scrollTop+=delta;},scrollTo({top}){this.scrollTop=top;}};
  root.parentElement=container;
  const api=run('linkedin',baseAdapter('linkedin',[root]),'https://www.linkedin.com/feed/');
  const initial=await api.collect();
  assert.equal(initial.scroll.context,'source_container');
  assert.equal(initial.scroll.height,3000);assert.equal(initial.scroll.viewportHeight,500);
  api.scrollSourceBy(0.75);assert.equal((await api.collect()).scroll.y,375);
  api.scrollSourceTo(1200);assert.equal((await api.collect()).scroll.y,1200);
  api.scrollSourceTo(0);assert.equal((await api.collect()).scroll.y,0);
});

test('LinkedIn initial DOM image capture is independent of legacy recovery selectors and excludes profile media',async()=>{
  const own=element({tagName:'IMG',attrs:{src:'https://media.licdn.com/dms/image/own.jpg'}});
  const profile=element({tagName:'IMG',attrs:{src:'https://media.licdn.com/dms/image/profile.jpg'}});
  profile.closest=selector=>selector.includes('/in/') ? {} : null;
  const root=candidate({urn:'urn:li:share:1234567890',images:[own,profile],body:'New layout post'});
  const adapter=baseAdapter('linkedin',[root]);
  adapter.mediaAcquisition={detectExpectedKinds:()=>[],extractCandidates:()=>[]};
  const result=await run('linkedin',adapter,'https://www.linkedin.com/feed/').collect();
  assert.equal(result.posts[0].media.length,1);
  assert.equal(result.posts[0].media[0].url,own.src);
  assert.equal(JSON.stringify(result.posts[0].mediaExpected),'["image"]');
});

test('LinkedIn UI expansion text is excluded while punctuation in the actual post remains',async()=>{
  const root=candidate({urn:'urn:li:share:1234567890',body:'Caption'});
  const body={nodeType:3,nodeValue:'Native caption…'};
  const control={nodeType:1,tagName:'BUTTON',getAttribute:name=>name==='data-testid' ? 'expandable-text-button' : null,
    childNodes:[{nodeType:3,nodeValue:'…more'}]};
  const node={nodeType:1,tagName:'DIV',getAttribute:()=>null,childNodes:[body,control]};
  const adapter=baseAdapter('linkedin',[root]);adapter.extractText=(_root,helpers)=>helpers.structuredText(node);
  const result=await run('linkedin',adapter,'https://www.linkedin.com/feed/').collect();
  assert.equal(result.posts[0].text,'Native caption…');
});

test('image resolution diagnostics retain intrinsic versus rendered sizes without extra signed URLs',async()=>{
  const own=element({tagName:'IMG',attrs:{src:'https://media.licdn.com/dms/image/large.jpg?secret=original',
    currentSrc:'https://media.licdn.com/dms/image/small.jpg?secret=selected',
    srcset:'https://media.licdn.com/dms/image/large.jpg?secret=alternative 1280w',naturalWidth:800,naturalHeight:450}});
  const foreign=element({tagName:'IMG',attrs:{src:'https://other.example/unowned.jpg'}});
  const profile=element({tagName:'IMG',attrs:{src:'https://media.licdn.com/dms/image/profile.jpg'}});
  profile.closest=selector=>selector.includes('/in/') ? {} : null;
  const root=candidate({urn:'urn:li:share:1234567890',images:[own,foreign,profile]});
  const adapter=baseAdapter('linkedin',[root]);adapter.mediaAcquisition={detectExpectedKinds:()=>[],extractCandidates:()=>[]};
  const result=await run('linkedin',adapter,'https://www.linkedin.com/feed/').collect();
  const evidence=result.posts[0].mediaEvidence.imageResolution;
  assert.equal(evidence.length,1);
  assert.equal(evidence[0].mediaIndex,0);
  assert.equal(evidence[0].renderedWidth,500);
  assert.equal(evidence[0].naturalWidth,800);
  assert.equal(evidence[0].naturalSizeMeaning,'browser_density_adjusted_intrinsic_size');
  assert.equal(evidence[0].currentSrcDiffersFromSrc,true);
  assert.equal(evidence[0].hasSrcset,true);
  assert.equal(evidence[0].devicePixelRatio,null);
  assert.equal(JSON.stringify(evidence).includes('https:'),false);
  assert.equal(JSON.stringify(evidence).includes('secret'),false);
  assert.equal(result.posts[0].media[0].url,own.currentSrc);
});

test('a visible native post retains rendered images below the viewport but excludes hidden and comment images',async()=>{
  const below=element({tagName:'IMG',attrs:{src:'https://media.licdn.com/dms/image/below.jpg'},
    rect:{width:320,height:320,left:0,right:320,top:1000,bottom:1320}});
  const hidden=element({tagName:'IMG',attrs:{src:'https://media.licdn.com/dms/image/hidden.jpg'}});
  hidden.style.display='none';
  const comment=element({tagName:'IMG',attrs:{src:'https://media.licdn.com/dms/image/comment.jpg'}});
  comment.closest=selector=>selector.includes('comments-comment-item') ? {} : null;
  const root=candidate({urn:'urn:li:share:1234567890',images:[below,hidden],commentImages:[comment]});
  const adapter=baseAdapter('linkedin',[root]);adapter.mediaAcquisition={detectExpectedKinds:()=>[],extractCandidates:()=>[]};
  const result=await run('linkedin',adapter,'https://www.linkedin.com/feed/').collect();
  assert.equal(result.posts[0].media.length,1);
  assert.equal(result.posts[0].media[0].url,below.src);
  assert.equal(result.posts[0].mediaEvidence.imageResolution.length,1);
  root.getBoundingClientRect=()=>({width:600,height:300,left:0,right:600,top:1000,bottom:1300});
  assert.equal((await run('linkedin',adapter,'https://www.linkedin.com/feed/').collect()).posts.length,0);
});

test('LinkedIn and Instagram video posters remain pending until playback is observed',async()=>{
  const fixtures=[
    {source:'linkedin',root:candidate({urn:'urn:li:activity:1234567890',body:'Video post'}),
      url:'https://www.linkedin.com/feed/',posterUrl:'https://media.licdn.com/dms/image/video-poster.jpg',
      playbackUrl:'https://media.licdn.com/dms/video/clip.mp4'},
    {source:'instagram',root:candidate({anchors:[anchor('https://www.instagram.com/reel/Fixture123/')],body:'Video post'}),
      url:'https://www.instagram.com/',posterUrl:'https://scontent.cdninstagram.com/media/video-poster.jpg',
      playbackUrl:'https://scontent.cdninstagram.com/media/clip.mp4'},
  ];
  for(const fixture of fixtures){
    let playback=null;
    const adapter=baseAdapter(fixture.source,[fixture.root]);
    adapter.mediaAcquisition={detectExpectedKinds:()=>['video'],extractCandidates:()=>[{
      kind:'video',url:fixture.posterUrl,posterUrl:fixture.posterUrl,playbackUrl:playback,
      width:640,height:360,loaded:true,sourceKind:'poster',
    }]};
    const api=run(fixture.source,adapter,fixture.url);

    const posterOnly=await api.collect();
    assert.equal(posterOnly.posts[0].media[0].kind,'video',fixture.source);
    assert.equal(posterOnly.posts[0].media[0].playbackUrl,null,fixture.source);
    assert.deepEqual(Array.from(posterOnly.posts[0].mediaEvidence.expectedWithoutUrl),['video'],fixture.source);

    playback=fixture.playbackUrl;
    const hydrated=await api.collect();
    assert.equal(hydrated.posts[0].media[0].playbackUrl,fixture.playbackUrl,fixture.source);
    assert.deepEqual(Array.from(hydrated.posts[0].mediaEvidence.expectedWithoutUrl),[],fixture.source);
  }
});
