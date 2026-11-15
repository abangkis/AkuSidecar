import test from 'node:test';
import assert from 'node:assert/strict';
import {readFile,mkdir,mkdtemp,writeFile,rm} from 'node:fs/promises';
import {fileURLToPath,pathToFileURL} from 'node:url';
import {join,resolve,sep} from 'node:path';
import {spawnSync} from 'node:child_process';
import {sourceAssets} from '../worker.mjs';

function browserFixture(spec) {
  const location=new URL('https://www.instagram.com/');
  const window={location,innerHeight,innerWidth,scrollY:0};
  for(const code of spec.assets)new Function('window','location',code)(window,location);
  const registry=globalThis.AkuSourceAdapters,adapter=registry.get('instagram'),primitives=globalThis.AkuCapturePrimitives;
  const compactText=value=>String(value?.innerText??value??'').replace(/\s+/g,' ').trim();
  const cleanExpandedText=value=>String(value).replace(/(?:\s+|^)(?:…\s*)?(?:show |see )?(?:more|less)$/i,'').trim();
  const expandWindowed=new Function('sourceAdapters','capturePrimitives','compactText','structuredText','cleanExpandedText','delay','window',spec.expand+'\nreturn expandSourceContent;')(
    registry,primitives,compactText,primitives.structuredText,cleanExpandedText,ms=>new Promise(ok=>setTimeout(ok,ms)),window);
  const short='A primary caption preview',full='A primary caption expanded with additional owned source evidence.';
  const rows=[];
  function setup(item){
    location.href='https://www.instagram.com/';
    const button='<button type="button">more</button>';
    const caption='<span dir="auto">'+short+(item.sibling?'':button+(item.ambiguous?button:''))+'</span>';
    const own=item.wrapper?'<div class="caption">'+caption+(item.sibling?button:'')+'</div>':caption;
    const other=item.comment?'<div data-comment-id="comment"><span dir="auto">A much longer comment that must never become the primary caption '+button+'</span></div>':'';
    document.querySelector('main').innerHTML='<article><a href="/fixture_author/">fixture_author</a><a class="post" href="/p/Fixture123/">Post</a>'+(item.comment?'<span dir="auto">'+short+'</span>':own)+other+'</article>';
    const article=document.querySelector('article');let clicks=0;
    for(const control of article.querySelectorAll('button'))control.onclick=()=>{
      clicks++;
      if(item.unresponsive)return;
      const span=article.querySelector('span[dir="auto"]');span.textContent=full;
      control.textContent='less';span.append(control);
      if(item.ownerChanged)article.querySelector('a.post').href='/p/Other123/';
      if(item.routeChanged)location.href='https://www.instagram.com/p/Other123/';
    };
    return {article,clicks:()=>clicks};
  }
  return (async()=>{
    for(const item of [
      {name:'direct detect only',detectOnly:true},
      {name:'wrapped detect only',wrapper:true,detectOnly:true},
      {name:'direct expansion'},
      {name:'wrapped expansion',wrapper:true},
      {name:'sibling control in exact wrapper',wrapper:true,sibling:true},
      {name:'comment control excluded',comment:true},
      {name:'ambiguous controls stay partial',ambiguous:true},
      {name:'expired deadline',expired:true},
      {name:'unresponsive control',unresponsive:true},
      {name:'changed primary identity',ownerChanged:true},
      {name:'changed route',routeChanged:true},
    ]){
      const w=setup(item),deadlineAt=Date.now()+(item.expired?-1:5000);
      const windowed=item.detectOnly ? await primitives.expandContent({container:w.article,adapter,
        readText:root=>adapter.extractText(root,{compactText}),canExpand:()=>false,route:()=>location.href,deadlineAt})
        : await expandWindowed(w.article,'instagram',deadlineAt);
      const windowedText=adapter.extractText(w.article,{compactText});
      const h=setup(item);
      globalThis.AkuHeadlessCapturePolicy={allowContentExpansion:!item.detectOnly,deadlineAt:Date.now()+(item.expired?-1:5000)};
      const snapshot=await globalThis.XHeadlessPoC.collect(),post=snapshot.posts.find(p=>p.id==='instagram:p:Fixture123');
      rows.push({name:item.name,windowed:{state:windowed.state,expanded:windowed.expanded,
        textCompleteness:primitives.textCompleteness({expansionState:windowed.state}),text:windowedText,clicks:w.clicks()},
        headless:post?{text:post.text,textCompleteness:post.textCompleteness,collapsed:post.textCollapsed,status:post.textStatus,limitations:post.limitations,clicks:h.clicks()}:{rejected:true,clicks:h.clicks()}});
    }
    return {rows,short,full};
  })();
}

test('real offline DOM uses one guarded Instagram primary-caption implementation in Windowed and Headless',
  {skip:!process.env.AKU_OFFLINE_CHROME&&'Set AKU_OFFLINE_CHROME for isolated Instagram DOM parity.'},async()=>{
    const workspace=fileURLToPath(new URL('../../../../../../',import.meta.url));
    const root=join(workspace,'AkuSidecar','build','instagram-caption-parity');await mkdir(root,{recursive:true});
    const scratch=await mkdtemp(join(root,'fixture-'));
    const assets=await sourceAssets(join(workspace,'AkuBridge'),'instagram');
    const content=await readFile(join(workspace,'AkuBridge','content-script.js'),'utf8');
    const expand=content.match(/^  async function expandSourceContent\([\s\S]*?^  }/m)?.[0];assert.ok(expand);
    const script='('+browserFixture.toString()+')('+JSON.stringify({assets:assets.filter(a=>a.execute).map(a=>a.content),expand})+').then(r=>document.getElementById("result").textContent=JSON.stringify(r)).catch(e=>document.getElementById("result").textContent=JSON.stringify({error:String(e.stack||e)}));';
    const file=join(scratch,'fixture.html');
    await writeFile(file,'<!doctype html><meta charset="utf-8"><meta http-equiv="Content-Security-Policy" content="default-src \'none\'; script-src \'unsafe-inline\' \'unsafe-eval\'; style-src \'unsafe-inline\'"><base href="https://www.instagram.com/"><style>article{width:600px;min-height:100px}span{display:block}</style><main></main><pre id="result"></pre><script>'+script.replaceAll('</script','<\\/script')+'</script>');
    const profile=join(scratch,'profile'),temp=join(scratch,'temp');await mkdir(temp);
    try{
      const run=spawnSync(process.env.AKU_OFFLINE_CHROME,['--headless','--dump-dom','--virtual-time-budget=20000',
        '--disable-background-networking','--disable-component-update','--no-first-run','--no-default-browser-check',
        '--host-resolver-rules=MAP * ~NOTFOUND','--window-size=1200,900','--user-data-dir='+profile,pathToFileURL(file).href],
      {windowsHide:true,timeout:30000,encoding:'utf8',maxBuffer:8*1024*1024,env:{...process.env,TEMP:temp,TMP:temp}});
      assert.equal(run.status,0,String(run.error??run.stderr).slice(-1000));
      const encoded=run.stdout.match(/<pre id="result">([\s\S]*?)<\/pre>/)?.[1];assert.ok(encoded);
      const result=JSON.parse(encoded.replaceAll('&lt;','<').replaceAll('&gt;','>').replaceAll('&amp;','&'));assert.ok(!result.error,result.error);
      await writeFile(join(scratch,'results.json'),JSON.stringify(result,null,2));
      for(const row of result.rows){
        if(['changed primary identity','changed route'].includes(row.name)){
          assert.equal(row.windowed.expanded,false,row.name);assert.equal(row.headless.rejected,true,row.name);continue;
        }
        assert.equal(row.windowed.textCompleteness,row.headless.textCompleteness,row.name);
        assert.equal(row.windowed.text,row.headless.text,row.name);
        if(['direct expansion','wrapped expansion','sibling control in exact wrapper'].includes(row.name)){
          assert.equal(row.windowed.text,result.full,row.name);assert.equal(row.headless.textCompleteness,'expanded');
          assert.equal(row.windowed.clicks,1);assert.equal(row.headless.clicks,1);
        }else if(row.name==='unresponsive control'){
          assert.equal(row.headless.textCompleteness,'partial');assert.equal(row.windowed.clicks,1);assert.equal(row.headless.clicks,1);
        }else{
          assert.equal(row.windowed.clicks,0,row.name);assert.equal(row.headless.clicks,0,row.name);
          assert.equal(row.headless.textCompleteness,row.name==='comment control excluded'?'no_collapse_observed':'partial',row.name);
        }
      }
    }finally{
      for(const directory of [profile,temp]){
        assert.ok(resolve(directory).startsWith(resolve(scratch)+sep));
        await rm(directory,{recursive:true,force:true}).catch(()=>{});
      }
    }
  });
