import {canonicalSourceURL} from './observation.mjs';

export async function resolveInstagramNativeTarget({page,requestedUrl,snapshot,resolver,deadlineAt}) {
  const url=canonicalSourceURL('instagram',requestedUrl);
  if(!url || !resolver?.available || !resolver.functionSource || resolver.functionSource.length>128*1024
    || snapshot?.posts?.length || snapshot?.loginRequired || snapshot?.challengeDetected || snapshot?.sourceUnavailable) return snapshot;
  try {
    if(canonicalSourceURL('instagram',await page.evaluate('location.href',Math.max(1,deadlineAt-Date.now())))!==url) return snapshot;
    const result=await page.evaluate(`(${resolver.functionSource})(${JSON.stringify({maxCandidates:12,maxScripts:48,maxTotalBytes:524288,maxTraversalNodes:6000})})`,Math.min(1200,Math.max(1,deadlineAt-Date.now())));
    if(result?.runtimeRevision!==resolver.runtimeRevision || !Array.isArray(result.candidates) || Buffer.byteLength(JSON.stringify(result))>256*1024) return snapshot;
    const matches=result.candidates.filter(c=>canonicalSourceURL('instagram',c?.permalink)===url);
    if(matches.length!==1) return snapshot;
    const candidate=matches[0];
    const match=url.match(/\/(p|reel|tv)\/([A-Za-z0-9_-]+)\/$/);
    const id=`instagram:${match[1]}:${match[2]}`;
    if(candidate.platformId!==id || candidate.candidateId!==`instagram:post:${match[2]}`
      || typeof candidate.author!=='string' || !candidate.author || candidate.author.length>300 || typeof candidate.text!=='string'
      || Array.from(candidate.text).length>4000) return snapshot;
    const media=(Array.isArray(candidate.media) ? candidate.media : []).slice(0,20).map(m=>safeMedia('instagram',m)).filter(Boolean);
    if(!candidate.text && !media.length) return snapshot;
    return {...snapshot,posts:[{id,permalink:url,author:candidate.author,text:candidate.text,
      publishedAt:typeof candidate.publishedAt==='string' && Number.isFinite(Date.parse(candidate.publishedAt)) ? candidate.publishedAt : null,
      contentKind:media.some(m=>m.kind==='video') ? 'video' : 'post',relationshipType:'original',media,
      mediaExpected:[...new Set(media.map(m=>m.kind))],mediaEvidence:{status:'owned_url_observed_partial'},
      presentation:{permalinkSource:'explicit_target_structured_shortcode',promoted:candidate.promoted===true},
      textStatus:'bounded_structured_caption',limitations:['native_target_structured_evidence','no_production_admission'],
      evidenceMode:'headless_native_target_structured_observation'}],
      candidateDiagnostics:{...(snapshot.candidateDiagnostics || {}),nativeTargetStructuredCount:1}};
  } catch {return snapshot;}
}

// Reuse bounded Bridge readers. Exact native identity remains required; these
// URLs enrich observed posts, but do not certify source parity or playback.
export async function resolveAdditionalSourceMedia({page,source,posts,resolver,feedResolver,deadlineAt}) {
  const output=[];
  let captions=[];
  if(source==='instagram' && posts?.length && feedResolver?.available && typeof feedResolver.functionSource==='string'
    && feedResolver.functionSource.length<=128*1024 && Date.now()<deadlineAt) {
    try {
      const result=await page.evaluate(`(${feedResolver.functionSource})(${JSON.stringify({maxCandidates:12,maxScripts:48,maxTotalBytes:524288,maxTraversalNodes:6000})})`,Math.min(1200,deadlineAt-Date.now()));
      if(result?.runtimeRevision===feedResolver.runtimeRevision && Array.isArray(result.candidates) && Buffer.byteLength(JSON.stringify(result))<=256*1024) captions=result.candidates;
    } catch { /* Preserve observed text if native caption evidence is unavailable. */ }
  }
  for (const originalPost of Array.isArray(posts) ? posts : []) {
    let post=originalPost;
    const native=canonicalSourceURL(source,post.permalink);
    const shortcode=source==='instagram' && native?.match(/\/(?:p|reel|tv)\/([A-Za-z0-9_-]+)\/$/)?.[1];
    const nativeId=source==='linkedin' && native?.match(/urn:li:(activity|ugcPost|share):(\d+)\/$/);
    const expectedId=shortcode ? `instagram:${native.split('/')[3]}:${shortcode}`
      : nativeId ? `linkedin:${nativeId[1].toLowerCase()}:${nativeId[2]}` : null;
    if (!expectedId || post.id!==expectedId) { output.push(post); continue; }
    if(source==='instagram') {
      const matches=captions.filter(c=>c?.platformId===post.id && canonicalSourceURL(source,c.permalink)===native);
      const candidate=matches.length===1 ? matches[0] : null;
      const normalized=value=>String(value || '').replace(/\s+/g,' ').trim();
      const visiblePrefix=normalized(post.text).replace(/(?:\.{3}|…)$/,'').trim();
      if(candidate && typeof candidate.text==='string' && Array.from(candidate.text).length<=4000 && visiblePrefix
        && normalized(candidate.author).toLowerCase()===normalized(post.author).toLowerCase()
        && candidate.text.length>post.text.length && normalized(candidate.text).startsWith(visiblePrefix)) {
        post={...post,text:candidate.text,textStatus:'structured_caption_native_id_bound',
          presentation:{...(post.presentation || {}),textSource:'native_shortcode_json'},
          limitations:[...(post.limitations || []).filter(v=>v!=='text_may_be_collapsed'),'structured_caption_enrichment',
            ...(Array.from(candidate.text).length===4000 ? ['text_may_be_truncated'] : [])]};
      }
    }
    const candidateId=shortcode ? `instagram:post:${shortcode}` : expectedId;
    let status='unavailable';
    let additions=[];
    if (resolver?.available && typeof resolver.functionSource==='string' && resolver.functionSource.length<=128*1024 && Date.now()<deadlineAt) {
      const playerIds=(post.mediaEvidence?.playerIds || []).filter(id=>typeof id==='string' && id.length<=240).slice(0,16);
      const request=source==='instagram'
        ? {candidateIds:[candidateId],maxCandidates:1,maxMediaPerCandidate:20,maxScripts:48,maxTotalBytes:524288,maxTraversalNodes:6000}
        : {candidateIds:[candidateId],playerIds,maxCandidates:1,maxPlayers:16,maxTraversalNodes:3000};
      try {
        const result=await page.evaluate(`(${resolver.functionSource})(${JSON.stringify(request)})`,Math.min(1200,deadlineAt-Date.now()));
        if (result?.runtimeRevision===resolver.runtimeRevision && Array.isArray(result.candidates) && Buffer.byteLength(JSON.stringify(result))<=256*1024) {
          const matches=result.candidates.filter(c=>c?.candidateId===candidateId);
          status=matches.length>1 ? 'ambiguous' : matches.length ? 'no_safe_media' : 'no_match';
          if (matches.length===1) {
            additions=(Array.isArray(matches[0].media) ? matches[0].media : []).slice(0,20).map(item=>safeMedia(source,item)).filter(Boolean);
            if (additions.length) status='observed_owned_urls';
          }
        }
      } catch { /* Optional source state cannot erase the original observation. */ }
    }
    const media=[...(Array.isArray(post.media) ? post.media : [])];
    for (const addition of additions) {
      const index=media.findIndex(m=>m.kind===addition.kind && samePath(m.posterUrl || m.url,addition.posterUrl || addition.url));
      if (index>=0) media[index]={...media[index],...addition};
      else if(media.length<20) media.push(addition);
    }
    const hasVideo=media.some(m=>m.kind==='video');
    const evidence={...(post.mediaEvidence || {}),additionalStructuredMedia:{status,runtimeRevision:resolver?.runtimeRevision || null,ownedUrlCount:additions.length}};
    if(additions.length) {
      evidence.status='owned_url_observed_partial';
      evidence.expectedWithoutUrl=(evidence.expectedWithoutUrl || []).filter(kind=>!media.some(m=>m.kind===kind && (kind!=='video' || m.playbackUrl)));
    }
    if (hasVideo) evidence.videoStreamStatus=media.some(m=>m.kind==='video' && m.playbackUrl) ? 'owned_playback_url_observed' : 'unknown';
    const ownedPlayback=additions.some(m=>m.kind==='video' && m.playbackUrl);
    const limitations=ownedPlayback ? [...new Set([...(post.limitations || []).filter(v=>v!=='video_stream_not_resolved'),'video_playback_unverified'])] : post.limitations;
    output.push({...post,media,limitations,mediaExpected:[...new Set([...(post.mediaExpected || []),...(hasVideo ? ['video'] : [])])],mediaEvidence:evidence});
  }
  return {posts:output,summary:{mode:'additional_source_owned_url_observation',postCount:output.length}};
}

function samePath(a,b) {
  try {const x=new URL(a),y=new URL(b);return x.origin===y.origin && x.pathname===y.pathname;} catch {return false;}
}
function safeUrl(source,value,video=false) {
  try {
    const url=new URL(value);
    if (url.protocol!=='https:' || url.username || url.password || url.port) return null;
    if (source==='instagram') {
      if (!['fbcdn.net','cdninstagram.com'].some(s=>url.hostname===s || url.hostname.endsWith(`.${s}`))) return null;
      if (!(video ? /\.mp4$/i : /\.(?:avif|gif|heic|jpe?g|png|webp)$/i).test(url.pathname)) return null;
    } else if (video) {
      if(url.hostname!=='dms.licdn.com' || !/^\/playlist\//i.test(url.pathname) || !/\/mp4-\d{2,4}p(?:-|\/)/i.test(url.pathname)) return null;
    } else if (!((url.hostname==='media.licdn.com' && /^\/dms\/image\//i.test(url.pathname)) ||
      (url.hostname==='dms.licdn.com' && /^\/playlist\/vid\//i.test(url.pathname) && /\/thumbnail(?:-[a-z0-9]+)?\//i.test(url.pathname)))) return null;
    url.hash='';return url.href;
  } catch {return null;}
}
function safeMedia(source,item) {
  if (!item || !['image','video'].includes(item.kind)) return null;
  const url=safeUrl(source,item.posterUrl || item.url);
  if (!url) return null;
  const playback=item.kind==='video' ? safeUrl(source,item.playbackUrl,true) : null;
  if (item.kind==='video' && !playback) return null;
  return {kind:item.kind,url,...(playback ? {posterUrl:url,playbackUrl:playback,playbackMode:'inline'} : {}),
    width:Number.isFinite(item.width) ? Math.max(0,Math.round(item.width)) : 0,
    height:Number.isFinite(item.height) ? Math.max(0,Math.round(item.height)) : 0,
    loaded:false,loadedMeaning:'structured_url_observed',sourceKind:'owned_structured_media'};
}
