// Photo-viewer evidence is deliberately separate from feed-post admission.
(() => {
  const photoId = value => {
    try {
      const u = new URL(value);
      if (u.origin !== 'https://www.facebook.com' || !/^\/photo(?:\.php|\/)?$/.test(u.pathname)) return null;
      const ids = [...u.searchParams.getAll('fbid'), ...u.searchParams.getAll('photo_id')];
      return ids.length && ids.every(id => /^\d+$/.test(id) && id === ids[0]) ? ids[0] : null;
    } catch { return null; }
  };
  const mediaPath = value => {
    try {
      const u = new URL(value);
      return u.protocol === 'https:' && !u.username && !u.password && !u.port
        && /(^|\.)fbcdn\.net$/.test(u.hostname) ? u.origin + u.pathname : null;
    } catch { return null; }
  };
  function parentBinding(records, ownerId) {
    const stories=records.flatMap(r=>[r.container_story,r.creation_story]).filter(s=>s&&typeof s==='object');
    const unique=key=>new Set(stories.map(s=>s[key]).filter(v=>typeof v==='string'&&v));
    if (unique('id').size!==1 || unique('post_id').size!==1 || unique('url').size!==1) return null;
    const full=stories.filter(s=>s.post_id&&s.url&&typeof s.message?.text==='string'&&Array.isArray(s.actors)&&s.actors.length===1);
    if (!full.length) return null;
    const first=full[0],actor=first.actors[0],text=first.message.text;
    if (!/^\d+$/.test(first.post_id)||actor.id!==ownerId||typeof actor.name!=='string'||!actor.name.trim()
        ||actor.name.length>1200||text.length>4000) return null;
    if (full.some(s=>s.message.text!==text||s.actors[0].id!==actor.id||s.actors[0].name!==actor.name)) return null;
    // Partial fragments may corroborate identity, but cannot override a conflicting actor/body.
    if (stories.some(s=>typeof s.message?.text==='string'&&s.message.text!==text
      ||Array.isArray(s.actors)&&s.actors.some(a=>a.id!==actor.id||a.name&&a.name!==actor.name))) return null;
    try {
      const u=new URL(first.url),id=u.pathname.match(/^\/[^/]+\/posts\/(pfbid[A-Za-z0-9]+|\d+)\/?$/)?.[1];
      if(u.origin!=='https://www.facebook.com'||u.username||u.password||u.search||u.hash||!id) return null;
      return {url:u.href,nativeId:`facebook:post:${id}`,numericPostId:first.post_id,author:actor.name,text};
    }catch{return null;}
  }
  function collect() {
    const id = photoId(location.href);
    if (!id) return {status:'not_photo_route'};
    const queue = [], records = []; let bytes = 0, nodes = 0;
    for (const script of document.querySelectorAll('script[type="application/json"]')) {
      const text = script.textContent || '';
      bytes += text.length;
      if (bytes > 8 * 1024 * 1024) return {status:'evidence_limit'};
      try { queue.push(JSON.parse(text)); } catch { /* Not a JSON evidence record. */ }
    }
    while (queue.length) {
      if (++nodes > 60000) return {status:'evidence_limit'};
      const value = queue.pop();
      if (!value || typeof value !== 'object') continue;
      if (value.id === id) records.push(value);
      for (const child of Object.values(value)) if (child && typeof child === 'object') queue.push(child);
    }
    const photos = records.filter(r => r.__typename === 'Photo' && mediaPath(r.image?.uri));
    if (!photos.length) return {status:'photo_metadata_missing'};
    const paths = new Set(photos.map(r => mediaPath(r.image.uri)));
    const owners = new Set(records.map(r => r.owner?.id).filter(v => typeof v === 'string' && /^\d+$/.test(v)));
    if (paths.size !== 1 || owners.size > 1) return {status:'conflicting_photo_binding'};
    if (owners.size !== 1) return {status:'photo_owner_missing'};
    const expected = [...paths][0];
    const matching = [...document.querySelectorAll('img')].filter(img => {
      const r = img.getBoundingClientRect(), style = getComputedStyle(img);
      return r.width >= 160 && r.height >= 160 && r.bottom > 0 && r.top < innerHeight
        && r.right > 0 && r.left < innerWidth && style.display !== 'none'
        && style.visibility === 'visible' && Number(style.opacity) !== 0
        && img.complete && img.naturalWidth > 0 && img.naturalHeight > 0
        && mediaPath(img.currentSrc || img.src) === expected;
    });
    if (!matching.length) return {status:'photo_image_not_visible'};
    const img = matching[0];
    return {status:'verified_photo_media', identityKind:'photo', photoId:id, ownerId:[...owners][0],
      media:{kind:'image',url:img.currentSrc || img.src,width:img.naturalWidth,height:img.naturalHeight,loaded:true},
      // A photo owner does not establish the author or body of its parent post.
      postBinding:'unverified', parent:parentBinding(records,[...owners][0]), provenance:'exact_photo_metadata_and_visible_image'};
  }
  globalThis.FacebookHeadlessPhotoEvidence = {collect};
})();
