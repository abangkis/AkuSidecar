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
      postBinding:'unverified', provenance:'exact_photo_metadata_and_visible_image'};
  }
  globalThis.FacebookHeadlessPhotoEvidence = {collect};
})();
