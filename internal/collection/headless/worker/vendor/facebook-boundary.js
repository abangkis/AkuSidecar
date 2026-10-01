// PoC-only ownership boundary. Queries retain real DOM nodes for clicks/geometry.
(() => {
  if(globalThis.FacebookHeadlessBoundary?.runtimeRevision==='facebook-boundary-v3')return;
  const selector = 'div[aria-posinset], [role="article"]';
  const actionSelector = '[aria-label^="Actions for this post by "]';
  const contentSelector = '[data-ad-preview="message"], [data-ad-comet-preview="message"]';
  const views = new WeakMap();
  const raw = node => node?.__facebookBoundaryRoot || node;
  function isPostBoundary(element){
    if(element.matches('div[aria-posinset]'))return true;
    const postSignals=[...element.querySelectorAll(`${actionSelector}, ${contentSelector}`)]
      .some(node=>node.closest(selector)===element);
    if(postSignals)return true;
    // Facebook image/video presentation can use role=article without a post
    // header/body. Unknown article structures stay separated conservatively.
    return !element.querySelector('img,video');
  }
  function owner(element){
    let scope=element?.closest?.(selector);
    while(scope&&!isPostBoundary(scope))scope=scope.parentElement?.closest(selector);
    return scope||null;
  }
  function owns(element,container){
    const root=raw(container);let scope=element?.closest?.(selector);
    while(scope){
      if(scope===root)return true;
      if(isPostBoundary(scope))return false;
      scope=scope.parentElement?.closest(selector);
    }
    return false;
  }
  function rootFor(candidate) {
    // A position wrapper can represent a single primary article. A wrapper with
    // its own post header/body must retain its own boundary instead.
    if(candidate.matches('div[aria-posinset]') && !candidate.matches('[role="article"]')) {
      const ownAction=[...candidate.querySelectorAll(actionSelector)].some(node=>owns(node,candidate));
      const ownBody=[...candidate.querySelectorAll(contentSelector)].some(node=>owns(node,candidate));
      const primary=[...candidate.querySelectorAll('[role="article"]')]
        .filter(node=>node.parentElement?.closest(selector)===candidate&&isPostBoundary(node));
      if(!ownAction&&!ownBody&&primary.length===1)return primary[0];
    }
    return candidate;
  }
  function viewFor(candidate) {
    const root=rootFor(raw(candidate));
    if(views.has(root))return views.get(root);
    const view=new Proxy(root,{get(target,key){
      if(key==='__facebookBoundaryRoot')return root;
      if(key==='querySelectorAll')return query=>[...root.querySelectorAll(query)].filter(node=>owns(node,root))
        .filter(node=>query!=='a[href]'||Boolean(node.getAttribute('href')?.trim())&&!/^[#?]/.test(node.getAttribute('href').trim()));
      if(key==='querySelector')return query=>[...root.querySelectorAll(query)].find(node=>owns(node,root))||null;
      if(key==='innerText'||key==='textContent'){
        if(!root.querySelector(selector))return root[key];
        const copy=root.cloneNode(true);
        for(const nested of copy.querySelectorAll(selector))if(isPostBoundary(nested))nested.remove();
        for(const block of copy.querySelectorAll('div,p,section,article,h1,h2,h3,h4,li,br'))block.appendChild(document.createTextNode('\n'));
        return copy.textContent;
      }
      const value=Reflect.get(target,key,target);
      return typeof value==='function'?value.bind(target):value;
    }});
    views.set(root,view);return view;
  }
  function candidates(discovered){return [...new Set(discovered.map(viewFor))];}
  function diagnostics(discovered){
    return discovered.slice(0,12).map(candidate=>{
      const view=viewFor(candidate),root=raw(view);
      return {role:candidate.getAttribute('role'),positionWrapper:candidate.hasAttribute('aria-posinset'),
        narrowed:root!==candidate,nestedPostBoundaries:candidate.querySelectorAll(selector).length,
        allActions:candidate.querySelectorAll(actionSelector).length,ownActions:view.querySelectorAll(actionSelector).length,
        allBodies:candidate.querySelectorAll(contentSelector).length,ownBodies:view.querySelectorAll(contentSelector).length,
        allAnchors:candidate.querySelectorAll('a[href]').length,ownAnchors:view.querySelectorAll('a[href]').length,
        anchors:[...candidate.querySelectorAll('a[href]')].slice(0,20).map(anchor=>{
          const literal=anchor.getAttribute('href')?.trim()||'',rect=anchor.getBoundingClientRect(),url=new URL(anchor.href,location.href);
          return {owned:owns(anchor,root),literalKind:!literal?'empty':literal.startsWith('#')?'fragment':literal.startsWith('?')?'query_only':/^https?:/.test(literal)?'absolute':'path',
            resolvedToCurrentPath:url.pathname===location.pathname,postUrlShaped:/\/(posts|videos|reel)\//.test(url.pathname),
            targetBlank:anchor.target==='_blank',inBody:Boolean(anchor.closest(contentSelector)),media:Boolean(anchor.querySelector('img,video')),
            width:Math.round(rect.width),height:Math.round(rect.height)};
        })};
    });
  }
  globalThis.FacebookHeadlessBoundary={runtimeRevision:'facebook-boundary-v3',raw,owner,owns,candidates,diagnostics};
})();
