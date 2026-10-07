import test from 'node:test';
import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import {sourceFreshnessContractFor} from '../source-freshness-contract.mjs';

const sources=['x','linkedin','instagram','facebook'];
const assets=await Promise.all(['capture-primitives.js','source-adapter-runtime.js',...sources.map(source=>`adapters/${source}-adapter.js`)].map(async name=>({
  relative:'AkuBridge/'+name,content:await readFile(new URL('../../../../../../AkuBridge/'+name,import.meta.url),'utf8')})));

test('X source contract uses actual adapter route and primary identity rules',()=>{
  const contract=sourceFreshnessContractFor(assets,'x');
  assert.equal(contract.enabled,true);
  assert.equal(contract.matchesFeedURL('https://x.com/home?tab=following#feed'),true);
  for(const url of ['https://x.com/login','https://x.com/a/status/123','https://x.com:444/home','https://user@x.com/home','https://evil.test/home']){
    assert.equal(contract.matchesFeedURL(url),false,url);
  }
  for(const id of ['12345','x:status:12345']){
    const own=contract.primaryIdentity({id,permalink:'https://x.com/author/status/12345/photo/1?foo=1',quotedPost:{id:'99999'}});
    assert.equal(own.id,'12345');assert.equal(own.permalink,'https://x.com/author/status/12345');
  }
  for(const post of [{id:'123',permalink:'https://x.com/a/status/456'},
    {id:'x:status:123',permalink:'https://evil.test/a/status/123'},
    {id:'',permalink:'https://x.com/a/status/123',quotedPost:{id:'123'}},
    {id:123,permalink:'https://x.com/a/status/123'}])assert.equal(contract.primaryIdentity(post),null);
});

test('other source adapters retain an explicit disabled headless contract',()=>{
  for(const source of sources.filter(source=>source!=='x')){
    const contract=sourceFreshnessContractFor(assets,source);
    assert.equal(contract.source,source);assert.equal(contract.enabled,false);
    assert.equal(contract.matchesFeedURL,undefined);
  }
});

test('missing, malformed or unknown adapter capabilities never enable recovery',()=>{
  assert.equal(sourceFreshnessContractFor([], 'x').enabled,false);
  assert.equal(sourceFreshnessContractFor(assets,'unknown').enabled,false);
  const broken=assets.map(asset=>asset.relative==='AkuBridge/adapters/x-adapter.js'?{...asset,content:'throw new Error("invalid adapter");'}:asset);
  assert.equal(sourceFreshnessContractFor(broken,'x').enabled,false);
  assert.equal(sourceFreshnessContractFor(assets,'x'),sourceFreshnessContractFor(assets,'x'));
});
