import {canonicalSourceURL} from './observation.mjs';

const MAX_INSTAGRAM_RETRY_CANDIDATES = 4;
const INSTAGRAM_HYDRATION_WAIT_MS = 100;
const MAX_RESOLVER_RESULT_BYTES = 256 * 1024;
const INSTAGRAM_DIAGNOSTIC_FIELDS = [
  'documentScriptCount', 'mediaScriptCount', 'inspectedScriptCount', 'parsedScriptCount',
  'rejectedScriptCount', 'inspectedBytes', 'traversedNodeCount', 'matchedMediaObjectCount', 'candidateCount',
];

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
  const instagramSummary=source==='instagram' ? createInstagramSummary(resolver,posts) : null;
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
    if (!expectedId || post.id!==expectedId) {
      if(instagramSummary) instagramSummary.additionalMediaSkippedPostCount++;
      output.push(post); continue;
    }
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
    let attemptRecords=[];
    let diagnosticOutcome='skipped';
    let skipReason=null;
    let retryOutcome='not_needed';
    let retryCount=0;
    if(source==='instagram') {
      const first=await resolveInstagramMediaAttempt({page,resolver,candidateId,deadlineAt,requestProfile:'standard'});
      status=first.status;
      additions=first.additions;
      attemptRecords.push(first.record);
      diagnosticOutcome=first.outcome;
      skipReason=first.reason;
      if(first.outcome==='attempted') instagramSummary.additionalMediaAttemptedPostCount++;
      else if(first.outcome==='error') instagramSummary.additionalMediaErrorPostCount++;
      else instagramSummary.additionalMediaSkippedPostCount++;
      recordInstagramResolverAttempt(instagramSummary,first.record);

      const expectedVideo=expectsInstagramVideoWithoutPlayback(post);
      if(status==='no_match' && first.record.returnedCandidateCount===0 && expectedVideo) {
        instagramSummary.additionalMediaRetryCandidateCount++;
        if(instagramSummary.additionalMediaRetryAttemptCount>=MAX_INSTAGRAM_RETRY_CANDIDATES) {
          retryOutcome='candidate_cap';
          instagramSummary.additionalMediaRetrySkippedCandidateCapCount++;
        } else if(Date.now()+INSTAGRAM_HYDRATION_WAIT_MS+10>=deadlineAt) {
          retryOutcome='deadline';
          instagramSummary.additionalMediaRetrySkippedDeadlineCount++;
        } else {
          try {
            await page.evaluate(`new Promise(resolve=>setTimeout(resolve,${INSTAGRAM_HYDRATION_WAIT_MS}))`,
              Math.min(150,Math.max(1,deadlineAt-Date.now())));
          } catch {
            retryOutcome='error';
            diagnosticOutcome='retry_error';
            instagramSummary.additionalMediaRetryErrorCount++;
          }
          if(retryOutcome==='not_needed' && Date.now()>=deadlineAt) {
            retryOutcome='deadline';
            instagramSummary.additionalMediaRetrySkippedDeadlineCount++;
          } else if(retryOutcome==='not_needed') {
            const retryProfile=first.record.resolverDiagnostics?.bounded===true ? 'expanded' : 'standard';
            const retry=await resolveInstagramMediaAttempt({page,resolver,candidateId,deadlineAt,requestProfile:retryProfile});
            retryCount=retry.outcome==='attempted' || retry.outcome==='error' ? 1 : 0;
            attemptRecords.push(retry.record);
            status=retry.status;
            additions=retry.additions;
            if(retryCount) instagramSummary.additionalMediaRetryAttemptCount++;
            recordInstagramResolverAttempt(instagramSummary,retry.record);
            if(retry.outcome==='error') {
              retryOutcome='error';
              diagnosticOutcome='retry_error';
              instagramSummary.additionalMediaRetryErrorCount++;
            } else if(retry.outcome==='skipped') {
              retryOutcome='deadline';
              instagramSummary.additionalMediaRetrySkippedDeadlineCount++;
            } else if(additions.length) {
              retryOutcome='recovered';
              diagnosticOutcome='retry_recovered';
              instagramSummary.additionalMediaRetryRecoveredCount++;
            } else if(retry.status==='no_match') {
              retryOutcome='no_match';
              diagnosticOutcome='retry_no_match';
              instagramSummary.additionalMediaRetryNoMatchCount++;
            } else {
              retryOutcome=retry.status==='ambiguous' ? 'ambiguous' : 'unsafe_or_no_safe_media';
              diagnosticOutcome='retried';
            }
          }
        }
        if(retryOutcome==='candidate_cap' || retryOutcome==='deadline' || retryOutcome==='error') {
          diagnosticOutcome=retryOutcome==='error' ? 'retry_error' : 'retry_skipped';
        }
      } else if(status==='no_match' && first.record.returnedCandidateCount>0) {
        retryOutcome='foreign_candidate_returned';
      } else if(status==='ambiguous') {
        retryOutcome='ambiguous';
      } else if(status==='no_safe_media') {
        retryOutcome='unsafe_or_no_safe_media';
      } else if(status==='no_match' && !expectedVideo) {
        retryOutcome='not_expected_video';
      }
    } else if (resolver?.available && typeof resolver.functionSource==='string' && resolver.functionSource.length<=128*1024 && Date.now()<deadlineAt) {
      const playerIds=(post.mediaEvidence?.playerIds || []).filter(id=>typeof id==='string' && id.length<=240).slice(0,16);
      const request={candidateIds:[candidateId],playerIds,maxCandidates:1,maxPlayers:16,maxTraversalNodes:3000};
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
    const additionalEvidence=source==='instagram'
      ? {status,runtimeRevision:safeRuntimeRevision(resolver?.runtimeRevision),ownedUrlCount:additions.length,
          outcome:diagnosticOutcome,attemptCount:attemptRecords.filter(record=>record.outcome==='attempted' || record.outcome==='error').length,
          retryCount,retryOutcome,...(skipReason ? {reason:skipReason} : {}),attempts:attemptRecords}
      : {status,runtimeRevision:resolver?.runtimeRevision || null,ownedUrlCount:additions.length};
    const evidence={...(post.mediaEvidence || {}),additionalStructuredMedia:additionalEvidence};
    if(instagramSummary) instagramSummary.additionalMediaOwnedUrlCount+=additions.length;
    if(additions.length) {
      evidence.status='owned_url_observed_partial';
      evidence.expectedWithoutUrl=(evidence.expectedWithoutUrl || []).filter(kind=>!media.some(m=>m.kind===kind && (kind!=='video' || m.playbackUrl)));
    }
    if (hasVideo) evidence.videoStreamStatus=media.some(m=>m.kind==='video' && m.playbackUrl) ? 'owned_playback_url_observed' : 'unknown';
    const ownedPlayback=additions.some(m=>m.kind==='video' && m.playbackUrl);
    const limitations=ownedPlayback ? [...new Set([...(post.limitations || []).filter(v=>v!=='video_stream_not_resolved'),'video_playback_unverified'])] : post.limitations;
    output.push({...post,media,limitations,mediaExpected:[...new Set([...(post.mediaExpected || []),...(hasVideo ? ['video'] : [])])],mediaEvidence:evidence});
  }
  if(instagramSummary) {
    instagramSummary.postCount=output.length;
    const resolverResponses=instagramSummary._resolverResponseCount;
    const observedBounded=instagramSummary._resolverBoundedObserved;
    const unknownBounded=instagramSummary._resolverBoundedUnknown;
    instagramSummary.bounded=instagramSummary.additionalMediaRetrySkippedCandidateCapCount>0 || observedBounded ? true
      : resolverResponses>0 && !unknownBounded ? false : null;
    instagramSummary.resolverBounded=observedBounded ? true : resolverResponses>0 && !unknownBounded ? false : null;
    delete instagramSummary._resolverResponseCount;
    delete instagramSummary._resolverBoundedObserved;
    delete instagramSummary._resolverBoundedUnknown;
    instagramSummary.status=instagramSummary.additionalMediaAttemptedPostCount+instagramSummary.additionalMediaRetryAttemptCount===0
      ? instagramSummary.additionalMediaSkippedPostCount+instagramSummary.additionalMediaErrorPostCount>0 ? 'unavailable' : 'not_needed'
      : instagramSummary.bounded ? 'bounded'
        : instagramSummary.additionalMediaOwnedUrlCount>0 ? 'observed' : 'unresolved';
    return {posts:output,summary:instagramSummary};
  }
  return {posts:output,summary:{mode:'additional_source_owned_url_observation',postCount:output.length}};
}

async function resolveInstagramMediaAttempt({page,resolver,candidateId,deadlineAt,requestProfile}) {
  const unavailable={status:'unavailable',additions:[],outcome:'skipped',reason:null,
    record:{requestProfile,outcome:'skipped',status:'unavailable',returnedCandidateCount:null,exactCandidateCount:null,
      foreignCandidateCount:null,resolverDiagnostics:null}};
  if(!resolver?.available || typeof resolver.functionSource!=='string' || resolver.functionSource.length>128*1024) {
    unavailable.reason='resolver_unavailable';
    return unavailable;
  }
  if(Date.now()>=deadlineAt) {unavailable.reason='deadline';return unavailable;}
  const request={candidateIds:[candidateId],maxCandidates:1,maxMediaPerCandidate:20,maxScripts:48,
    maxScriptBytes:512000,maxTotalBytes:requestProfile==='expanded'?2_000_000:524288,
    maxTraversalNodes:requestProfile==='expanded'?20_000:6000,...(requestProfile==='expanded'?{maxDepth:40}:{})};
  try {
    const result=await page.evaluate(`(${resolver.functionSource})(${JSON.stringify(request)})`,Math.min(1200,Math.max(1,deadlineAt-Date.now())));
    let size;
    try {size=Buffer.byteLength(JSON.stringify(result),'utf8');} catch {size=MAX_RESOLVER_RESULT_BYTES+1;}
    if(result?.runtimeRevision!==resolver.runtimeRevision || !Array.isArray(result.candidates) || size>MAX_RESOLVER_RESULT_BYTES) {
      return instagramAttemptFailure(requestProfile,'invalid_result');
    }
    const matches=result.candidates.filter(candidate=>candidate?.candidateId===candidateId);
    const returnedCandidateCount=diagnosticCount(result.candidates.length);
    const exactCandidateCount=diagnosticCount(matches.length);
    const record={requestProfile,outcome:'attempted',status:matches.length>1?'ambiguous':matches.length?'no_safe_media':'no_match',
      returnedCandidateCount,exactCandidateCount,
      foreignCandidateCount:returnedCandidateCount===null || exactCandidateCount===null ? null : Math.max(0,returnedCandidateCount-exactCandidateCount),
      resolverDiagnostics:safeInstagramResolverDiagnostics(result.diagnostics)};
    if(matches.length>1) return {status:'ambiguous',additions:[],outcome:'attempted',reason:null,record};
    if(matches.length===0) return {status:'no_match',additions:[],outcome:'attempted',reason:null,record};
    const additions=(Array.isArray(matches[0].media)?matches[0].media:[]).slice(0,20).map(item=>safeMedia('instagram',item)).filter(Boolean);
    const status=additions.length?'observed_owned_urls':'no_safe_media';
    record.status=status;
    return {status,additions,outcome:'attempted',reason:null,record};
  } catch {
    return instagramAttemptFailure(requestProfile,'resolver_evaluation_failed');
  }
}

function instagramAttemptFailure(requestProfile,reason) {
  return {status:'unavailable',additions:[],outcome:'error',reason,
    record:{requestProfile,outcome:'error',status:'unavailable',returnedCandidateCount:null,exactCandidateCount:null,
      foreignCandidateCount:null,reason,resolverDiagnostics:null}};
}

function safeInstagramResolverDiagnostics(value) {
  const diagnostics=value && typeof value==='object' && !Array.isArray(value) ? value : {};
  return {...Object.fromEntries(INSTAGRAM_DIAGNOSTIC_FIELDS.map(key=>[key,diagnosticCount(diagnostics[key],4_000_000)])),
    bounded:typeof diagnostics.bounded==='boolean'?diagnostics.bounded:null};
}

function diagnosticCount(value,maximum=1_000_000) {
  return Number.isSafeInteger(value) && value>=0 && value<=maximum ? value : null;
}

function safeRuntimeRevision(value) {
  return typeof value==='string' && value.length<=120 && /^[A-Za-z0-9._-]+$/.test(value) ? value : null;
}

function expectsInstagramVideoWithoutPlayback(post) {
  const media=Array.isArray(post.media)?post.media:[];
  const expected=(Array.isArray(post.mediaEvidence?.expectedWithoutUrl) && post.mediaEvidence.expectedWithoutUrl.includes('video'))
    || (Array.isArray(post.mediaExpected) && post.mediaExpected.includes('video'))
    || media.some(item=>item?.kind==='video_poster') || post.contentKind==='video';
  const hasPlayback=media.some(item=>item?.kind==='video' && typeof item.playbackUrl==='string' && item.playbackUrl.length>0);
  return expected && !hasPlayback;
}

function createInstagramSummary(resolver,posts) {
  return {mode:'additional_source_owned_url_observation',postCount:Array.isArray(posts)?posts.length:0,
    status:'not_needed',available:Boolean(resolver?.available && typeof resolver.functionSource==='string' && resolver.functionSource.length<=128*1024),
    bounded:null,resolverBounded:null,
    additionalMediaAttemptedPostCount:0,additionalMediaSkippedPostCount:0,additionalMediaErrorPostCount:0,
    additionalMediaRetryCandidateCount:0,additionalMediaRetryAttemptCount:0,additionalMediaRetryRecoveredCount:0,
    additionalMediaRetryNoMatchCount:0,additionalMediaRetryErrorCount:0,additionalMediaRetrySkippedDeadlineCount:0,
    additionalMediaRetrySkippedCandidateCapCount:0,additionalMediaOwnedUrlCount:0,
    additionalMediaResolverBoundedAttemptCount:null,additionalMediaResolverCandidateCount:null,
    additionalMediaReturnedCandidateCount:null,additionalMediaExactCandidateCount:null,
    additionalMediaInspectedScriptCount:null,additionalMediaParsedScriptCount:null,additionalMediaRejectedScriptCount:null,
    additionalMediaInspectedBytes:null,additionalMediaTraversedNodeCount:null,additionalMediaMatchedMediaObjectCount:null,
    _resolverResponseCount:0,_resolverBoundedObserved:false,_resolverBoundedUnknown:false};
}

function recordInstagramResolverAttempt(summary,record) {
  const diagnostics=record?.resolverDiagnostics;
  if(!diagnostics) return;
  const responseIndex=summary._resolverResponseCount++;
  if(diagnostics.bounded===true) summary._resolverBoundedObserved=true;
  else if(diagnostics.bounded===null) summary._resolverBoundedUnknown=true;
  if(summary._resolverBoundedUnknown) summary.additionalMediaResolverBoundedAttemptCount=null;
  else if(diagnostics.bounded===true) summary.additionalMediaResolverBoundedAttemptCount=
    Number.isSafeInteger(summary.additionalMediaResolverBoundedAttemptCount)
      ? summary.additionalMediaResolverBoundedAttemptCount+1 : 1;
  else if(responseIndex===0) summary.additionalMediaResolverBoundedAttemptCount=0;
  const pairs=[
    ['additionalMediaResolverCandidateCount','candidateCount'],
    ['additionalMediaInspectedScriptCount','inspectedScriptCount'],
    ['additionalMediaParsedScriptCount','parsedScriptCount'],
    ['additionalMediaRejectedScriptCount','rejectedScriptCount'],
    ['additionalMediaInspectedBytes','inspectedBytes'],
    ['additionalMediaTraversedNodeCount','traversedNodeCount'],
    ['additionalMediaMatchedMediaObjectCount','matchedMediaObjectCount'],
  ];
  for(const [target,field] of pairs) summary[target]=accumulateDiagnosticCount(summary[target],diagnostics[field],responseIndex,
    field==='inspectedBytes' ? 32_000_000 : 1_000_000);
  summary.additionalMediaReturnedCandidateCount=accumulateDiagnosticCount(summary.additionalMediaReturnedCandidateCount,record.returnedCandidateCount,responseIndex);
  summary.additionalMediaExactCandidateCount=accumulateDiagnosticCount(summary.additionalMediaExactCandidateCount,record.exactCandidateCount,responseIndex);
}

function accumulateDiagnosticCount(left,right,responseIndex,maximum=1_000_000) {
  if(!Number.isSafeInteger(right)||right<0) return null;
  if(responseIndex===0) return right;
  if(!Number.isSafeInteger(left)||left+right>maximum) return null;
  return left+right;
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
