// Media-only recovery is not feed-post admission. The store retains saved text/identity.
export function photoRecaptureObservation(evidence, requestedUrl, actualUrl, capturedAt, provenance) {
  try {
    const idFrom = raw => {
      const u=new URL(raw), ids=[...u.searchParams.getAll('fbid'),...u.searchParams.getAll('photo_id')];
      return u.origin==='https://www.facebook.com'&&!u.username&&!u.password
        &&/^\/photo(?:\.php|\/)?$/.test(u.pathname)&&ids.length
        &&ids.every(id=>/^[0-9]{1,32}$/.test(id)&&id===ids[0]) ? ids[0] : null;
    };
    const id=idFrom(requestedUrl), media=evidence?.media, u=new URL(media?.url);
    if(!id||idFrom(actualUrl)!==id||evidence.status!=='verified_photo_media'
      ||evidence.identityKind!=='photo'||evidence.photoId!==id
      ||evidence.provenance!=='exact_photo_metadata_and_visible_image'
      ||!/^[0-9]{1,32}$/.test(evidence.ownerId)||media.kind!=='image'||media.loaded!==true
      ||![media.width,media.height].every(n=>Number.isSafeInteger(n)&&n>0&&n<=100000)
      ||u.protocol!=='https:'||u.username||u.password||u.port||!/(^|\.)fbcdn\.net$/.test(u.hostname))return null;
    const proof={status:'verified',photoId:id,ownerId:evidence.ownerId,
      provenance:'exact_photo_metadata_and_visible_image'};
    return {source:'facebook',pageUrl:requestedUrl,pageTitle:'',capturedAt,
      snapshots:[{index:0,capturedAt,blocks:[{evidenceKey:`facebook:photo:${id}`,platformId:`facebook:photo:${id}`,
        permalink:requestedUrl,author:'',text:'',media:[{kind:'image',url:u.href,width:media.width,height:media.height}]}]}],
      coverage:{captureMode:'headless_worker',provenance,photoMediaRecapture:proof}};
  }catch{return null;}
}
