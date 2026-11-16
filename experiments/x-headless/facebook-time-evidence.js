// Bounded already-loaded Facebook JSON, following the existing MAIN-world resolver limits.
// Accept creation_time only when its Story is explicitly bound to the native post identity.
(() => {
  const object = value => value !== null && typeof value === 'object';
  const data = (value,key) => { try { return Object.getOwnPropertyDescriptor(value,key)?.value; } catch { return undefined; } };
  globalThis.FacebookHeadlessTimeEvidence = {
    resolve(candidateId, adapter, capturedAt) {
      const times = new Set();
      const diagnostics = { scannedScripts: 0, relevantScripts: 0, inspectedScripts: 0, inspectedBytes: 0, visited: 0, matchedStories: 0, bounded: false };
      const matches = value => [data(value,'url'),data(value,'permalink_url'),data(value,'shareable_url')]
        .some(url => typeof url === 'string' && (() => {
          try { const parsed=new URL(url); return parsed.protocol==='https:' && ['facebook.com','www.facebook.com'].includes(parsed.hostname)
            && !parsed.username&&!parsed.password&&adapter.platformIdFromCandidates([parsed.href])===candidateId; } catch { return false; }
        })());
      const accept = value => {
        const raw=data(value,'creation_time');
        if (Number.isSafeInteger(raw) && raw>=1072915200 && raw*1000<=Date.parse(capturedAt)+60000) times.add(raw);
      };
      const scripts=[...document.querySelectorAll('script[type="application/json"][data-sjs]')];
      if(scripts.length>256)diagnostics.bounded=true;
      for(const script of scripts.slice(0,256)) {
        diagnostics.scannedScripts++;
        const text=script.textContent || '';
        if(!text.includes('creation_time'))continue;
        diagnostics.relevantScripts++;
        if(diagnostics.relevantScripts>48||text.length>512000||diagnostics.inspectedBytes+text.length>4000000){diagnostics.bounded=true;continue;}
        diagnostics.inspectedBytes+=text.length;diagnostics.inspectedScripts++;
        let root;try{root=JSON.parse(text);}catch{continue;}
        const queue=[{value:root,depth:0}],seen=new Set();
        while(queue.length&&diagnostics.visited<20000) {
          const {value,depth}=queue.shift();if(!object(value)||seen.has(value))continue;
          seen.add(value);diagnostics.visited++;
          if(matches(value)) {
            diagnostics.matchedStories++;accept(value);
            const timestampStory=data(data(data(value,'comet_sections'),'timestamp'),'story');
            if(object(timestampStory) && (matches(timestampStory)
              || typeof data(value,'id')==='string' && data(value,'id')===data(timestampStory,'id'))) accept(timestampStory);
          }
          if(depth>=40){diagnostics.bounded=true;continue;}
          let keys;try{keys=Object.getOwnPropertyNames(value);}catch{keys=[];}
          if(keys.length>160)diagnostics.bounded=true;
          for(const key of keys.slice(0,160)){const child=data(value,key);if(object(child))queue.push({value:child,depth:depth+1});}
        }
        if(queue.length){diagnostics.bounded=true;break;}
      }
      const conflict=times.size>1;
      return { publishedAt: times.size===1&&!diagnostics.bounded ? new Date([...times][0]*1000).toISOString() : null,
        conflict, diagnostics };
    },
  };
})();
